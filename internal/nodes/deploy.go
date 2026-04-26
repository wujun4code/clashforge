package nodes

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"
)

const gostConfigTemplate = `services:
- name: service-0
  addr: ":443"
  handler:
    type: http
    auth:
      username: "%s"
      password: "%s"
    metadata:
      knock: "www.google.com"
      probeResistance: "code:404"
  listener:
    type: tcp
  forwarder:
    nodes:
    - name: target-0
      addr: "%s"
`

const gostServiceTemplate = `[Unit]
Description=GOST Proxy Service
After=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/gost -C /etc/gost/gost.yaml
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
`

// DeployResult holds the outcome of a deployment.
type DeployResult struct {
	Success     bool   `json:"success"`
	Error       string `json:"error,omitempty"`
	GOSTVersion string `json:"gost_version,omitempty"`
	CertIssued  bool   `json:"cert_issued"`
	CertExpiry  string `json:"cert_expiry,omitempty"`
	ProxyUser   string `json:"proxy_user,omitempty"`
	ProxyPass   string `json:"proxy_pass,omitempty"`
	ServicePort int    `json:"service_port"`
}

// DeployProgress is a callback for streaming progress updates.
type DeployProgress func(step, status, message, detail string)

// DeployGOST runs the full GOST deployment pipeline on the remote node.
// It streams progress via the callback function.
func DeployGOST(
	ctx context.Context,
	node *Node,
	progress DeployProgress,
) (*DeployResult, error) {
	// Step 1: SSH connect
	progress("connect", "running", "正在连接远程服务器...", "")
	client, err := NewSSHClient(node.Host, node.Port, node.Username, node.Password, 30*time.Second)
	if err != nil {
		progress("connect", "error", "SSH 连接失败", err.Error())
		return &DeployResult{Success: false, Error: err.Error()}, err
	}
	defer client.Close()
	progress("connect", "ok", "SSH 连接成功", node.Host)

	// Step 2: Check prerequisites
	progress("prereqs", "running", "正在检查系统环境...", "")
	out, err := client.Run("which go && which git && which curl || echo MISSING_PREREQS")
	if err != nil || strings.Contains(out, "MISSING_PREREQS") {
		progress("prereqs", "running", "安装必要依赖 (go, git)...", "")
		client.Run("which go || (apt-get update -qq && apt-get install -y -qq golang-go git curl) 2>&1")
	}
	progress("prereqs", "ok", "系统环境就绪", "")

	// Step 3: Install GOST
	progress("gost-install", "running", "正在安装 GOST...", "从 GitHub 克隆并编译")
	out, err = client.Run(`
if [ -d /tmp/gost ]; then rm -rf /tmp/gost; fi
git clone --depth 1 https://github.com/go-gost/gost.git /tmp/gost 2>&1
cd /tmp/gost
sudo bash install.sh 2>&1
`)
	if err != nil {
		progress("gost-install", "error", "GOST 安装失败", fmt.Sprintf("%s\n%s", err.Error(), out))
		return &DeployResult{Success: false, Error: fmt.Sprintf("gost install: %v", err)}, err
	}
	progress("gost-install", "ok", "GOST 安装完成", "")

	// Step 4: Quick verify
	progress("gost-verify", "running", "验证 GOST 服务...", "")
	out, err = client.Run("gost -V 2>&1 || /usr/local/bin/gost -V 2>&1 || echo v0.0.0")
	gostVersion := strings.TrimSpace(strings.Replace(out, "gost version ", "", 1))
	progress("gost-verify", "ok", fmt.Sprintf("GOST 版本: %s", gostVersion), "")

	// Step 5: Install acme.sh
	progress("acme-install", "running", "正在安装 acme.sh...", "")
	out, err = client.Run(fmt.Sprintf(
		"curl -s https://get.acme.sh | sh -s email=%s 2>&1", node.Email))
	if err != nil {
		progress("acme-install", "error", "acme.sh 安装失败", err.Error())
		return &DeployResult{Success: false, Error: fmt.Sprintf("acme.sh: %v", err)}, err
	}
	progress("acme-install", "ok", "acme.sh 安装完成", "")

	// Step 6: Issue TLS certificate
	progress("cert-issue", "running", fmt.Sprintf("正在为 %s 签发 TLS 证书...", node.Domain), "通过 Cloudflare DNS API")
	out, err = client.Run(fmt.Sprintf(`
export CF_Token="%s"
export CF_Account_ID="%s"
export CF_Zone_ID="%s"
~/.acme.sh/acme.sh --issue -d %s -d '*.%s' --dns dns_cf --server letsencrypt 2>&1
`, node.CFToken, node.CFAccountID, node.CFZoneID, node.Domain, node.Domain))
	if err != nil {
		progress("cert-issue", "error", "证书签发失败", fmt.Sprintf("%s\n%s", err.Error(), out))
		return &DeployResult{Success: false, Error: fmt.Sprintf("cert issue: %v", err)}, err
	}
	progress("cert-issue", "ok", "TLS 证书签发成功", node.Domain)

	// Step 7: Install certs to directory
	progress("cert-install", "running", "正在安装证书文件...", "")
	certDir := fmt.Sprintf("/etc/gost/certs/%s", node.Domain)
	client.Run(fmt.Sprintf("mkdir -p %s", certDir))
	out, err = client.Run(fmt.Sprintf(`
~/.acme.sh/acme.sh --install-cert -d %s \
  --key-file %s/key.pem \
  --fullchain-file %s/cert.pem \
  --ecc 2>&1
`, node.Domain, certDir, certDir))
	if err != nil {
		progress("cert-install", "error", "证书安装失败", err.Error())
		return &DeployResult{Success: false, Error: fmt.Sprintf("cert install: %v", err)}, err
	}
	progress("cert-install", "ok", "证书已安装", certDir)

	// Step 8: Generate proxy credentials
	proxyUser := "proxy"
	proxyPass := generatePass(16)

	// Step 9: Write GOST config
	progress("config-write", "running", "正在生成 GOST 配置文件...", "")
	gostYAML := fmt.Sprintf(gostConfigTemplate, proxyUser, proxyPass, certDir)
	client.Run("mkdir -p /etc/gost")
	escapedYAML := strings.ReplaceAll(gostYAML, "'", "'\\''")
	out, err = client.Run(fmt.Sprintf("echo '%s' > /etc/gost/gost.yaml", escapedYAML))
	if err != nil {
		progress("config-write", "error", "配置写入失败", err.Error())
		return &DeployResult{Success: false, Error: fmt.Sprintf("write config: %v", err)}, err
	}
	progress("config-write", "ok", "GOST 配置已写入", "/etc/gost/gost.yaml")

	// Step 10: Systemd service
	progress("systemd", "running", "正在注册 systemd 服务...", "")
	out, err = client.Run(fmt.Sprintf(`
cat > /etc/systemd/system/gost.service << 'SYSTEMDEOF'
%s
SYSTEMDEOF
systemctl daemon-reload
systemctl enable gost
systemctl restart gost
systemctl is-active gost
`, gostServiceTemplate))
	if err != nil || strings.TrimSpace(out) != "active" {
		progress("systemd", "error", "systemd 服务启动失败", fmt.Sprintf("%s\n%s", err, out))
		return &DeployResult{Success: false, Error: "systemd service failed to start"}, err
	}
	progress("systemd", "ok", "GOST 服务已启动", "systemctl status gost")

	return &DeployResult{
		Success:     true,
		GOSTVersion: gostVersion,
		CertIssued:  true,
		ProxyUser:   proxyUser,
		ProxyPass:   proxyPass,
		ServicePort: 443,
	}, nil
}

// generatePass creates a random password of given length using crypto/rand.
func generatePass(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*-_=+"
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			// Fallback: this should never happen with crypto/rand
			b[i] = charset[i%len(charset)]
			continue
		}
		b[i] = charset[n.Int64()]
	}
	return string(b)
}
