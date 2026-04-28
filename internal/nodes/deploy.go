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
	Success      bool          `json:"success"`
	Error        string        `json:"error,omitempty"`
	GOSTVersion  string        `json:"gost_version,omitempty"`
	CertIssued   bool          `json:"cert_issued"`
	CertExpiry   string        `json:"cert_expiry,omitempty"`
	ProxyUser    string        `json:"proxy_user,omitempty"`
	ProxyPass    string        `json:"proxy_pass,omitempty"`
	ServicePort  int           `json:"service_port"`
	Phase        string        `json:"phase"`
	ProbeResults []ProbeResult `json:"probe_results,omitempty"`
}

// DeployProgress is a callback for streaming progress updates.
type DeployProgress func(step, status, message, detail string)

// DeployGOST runs the deployment pipeline in two modes:
// - bootstrap mode: install/start GOST and run IP direct probe.
// - full mode: additionally bind domain on Cloudflare and issue ACME certificate.
func DeployGOST(
	ctx context.Context,
	node *Node,
	kp *KeyPair,
	progress DeployProgress,
) (*DeployResult, error) {
	_ = ctx
	phase := "bootstrap"
	fullMode := strings.TrimSpace(node.Domain) != "" && strings.TrimSpace(node.Email) != "" && strings.TrimSpace(node.CFToken) != ""
	if fullMode {
		phase = "full"
	}

	proxyUser := strings.TrimSpace(node.ProxyUser)
	if proxyUser == "" {
		proxyUser = "proxy"
	}
	proxyPass := strings.TrimSpace(node.ProxyPassword)
	if proxyPass == "" {
		proxyPass = generatePass(16)
	}

	progress("connect", "running", "正在连接远程服务器...", "")
	client, err := NewSSHClient(node.Host, node.Port, node.Username, BuildAuthMethods(node.Password, kp), 30*time.Second)
	if err != nil {
		progress("connect", "error", "SSH 连接失败", err.Error())
		return &DeployResult{Success: false, Error: err.Error(), Phase: phase}, err
	}
	defer client.Close()
	progress("connect", "ok", "SSH 连接成功", node.Host)

	progress("prereqs", "running", "正在检查系统环境...", "")
	out, err := client.Run("which curl || echo MISSING_CURL")
	if err != nil || strings.Contains(out, "MISSING_CURL") {
		progress("prereqs", "running", "安装 curl...", "")
		_, _ = client.Run("(apt-get update -qq && apt-get install -y -qq curl 2>&1) || (sudo apt-get update -qq && sudo apt-get install -y -qq curl 2>&1)")
	}
	progress("prereqs", "ok", "系统环境就绪", "")

	progress("gost-install", "running", "正在安装 GOST...", "下载预编译二进制")
	out, err = client.Run(`
set -e
ARCH=$(uname -m)
case $ARCH in
  x86_64)        GOARCH="amd64" ;;
  aarch64|arm64) GOARCH="arm64" ;;
  armv7l|armhf)  GOARCH="armv7" ;;
  *)             GOARCH="amd64" ;;
esac
GOST_VER=$(curl -sf "https://api.github.com/repos/go-gost/gost/releases/latest" 2>/dev/null \
  | grep '"tag_name"' | sed 's/.*"tag_name": *"v\{0,1\}\([^"]*\)".*/\1/' | head -1)
[ -z "$GOST_VER" ] && GOST_VER="3.2.6"
echo "Installing gost v${GOST_VER} for ${GOARCH}..."
GOST_URL="https://github.com/go-gost/gost/releases/download/v${GOST_VER}/gost_${GOST_VER}_linux_${GOARCH}.tar.gz"
curl -sSfL "$GOST_URL" -o /tmp/gost.tar.gz 2>&1
rm -rf /tmp/gost
tar -xzf /tmp/gost.tar.gz -C /tmp/ gost 2>&1
mkdir -p /usr/local/bin
sudo install -m 755 /tmp/gost /usr/local/bin/gost 2>/dev/null || install -m 755 /tmp/gost /usr/local/bin/gost
rm -f /tmp/gost.tar.gz /tmp/gost
echo "OK"
`)
	if err != nil {
		progress("gost-install", "error", "GOST 安装失败", fmt.Sprintf("%s\n%s", err.Error(), out))
		return &DeployResult{Success: false, Error: fmt.Sprintf("gost install: %v", err), Phase: phase}, err
	}
	progress("gost-install", "ok", "GOST 安装完成", "")

	progress("gost-verify", "running", "验证 GOST 服务...", "")
	out, _ = client.Run("gost -V 2>&1 || /usr/local/bin/gost -V 2>&1 || echo v0.0.0")
	gostVersion := strings.TrimSpace(strings.Replace(out, "gost version ", "", 1))
	progress("gost-verify", "ok", fmt.Sprintf("GOST 版本: %s", gostVersion), "")

	progress("config-write", "running", "正在生成 GOST 配置文件...", "")
	gostYAML := fmt.Sprintf(gostConfigTemplate, proxyUser, proxyPass)
	// Use printf + tee to write the config and gracefully handle privilege models:
	// 1) direct write (root user), 2) sudo write (non-root with sudo).
	out, err = client.Run(fmt.Sprintf(
		"(mkdir -p /etc/gost && printf '%%s' %s | tee /etc/gost/gost.yaml > /dev/null) || "+
			"(sudo mkdir -p /etc/gost && printf '%%s' %s | sudo tee /etc/gost/gost.yaml > /dev/null)",
		shellEscape(gostYAML),
		shellEscape(gostYAML),
	))
	if err != nil {
		progress("config-write", "error", "配置写入失败", err.Error())
		return &DeployResult{Success: false, Error: fmt.Sprintf("write config: %v", err), Phase: phase}, err
	}
	progress("config-write", "ok", "GOST 配置已写入", "/etc/gost/gost.yaml")

	progress("systemd", "running", "正在注册 systemd 服务...", "")
	out, err = client.Run(fmt.Sprintf(`
(
cat > /etc/systemd/system/gost.service << 'SYSTEMDEOF'
%s
SYSTEMDEOF
systemctl daemon-reload
systemctl enable gost
systemctl restart gost
systemctl is-active gost
) || (
cat > /tmp/gost.service << 'SYSTEMDEOF'
%s
SYSTEMDEOF
sudo mv /tmp/gost.service /etc/systemd/system/gost.service
sudo systemctl daemon-reload
sudo systemctl enable gost
sudo systemctl restart gost
sudo systemctl is-active gost
)
`, gostServiceTemplate, gostServiceTemplate))
	if err != nil || strings.TrimSpace(out) != "active" {
		progress("systemd", "error", "systemd 服务启动失败", fmt.Sprintf("%s\n%s", err, out))
		return &DeployResult{Success: false, Error: "systemd service failed to start", Phase: phase}, err
	}
	progress("systemd", "ok", "GOST 服务已启动", "systemctl status gost")

	progress("probe", "running", "IP 直连探测中...", "通过代理访问 Google / YouTube / GitHub")
	probeResults := TestHTTPProxy(node.Host, 443, proxyUser, proxyPass, 10*time.Second, DefaultProbeTargets())
	probeOK := 0
	for _, p := range probeResults {
		if p.OK {
			probeOK++
		}
	}
	progress("probe", "ok", fmt.Sprintf("探测完成 (%d/%d 通过)", probeOK, len(probeResults)), "")

	certIssued := false
	if fullMode {
		zoneID := strings.TrimSpace(node.CFZoneID)
		if zoneID == "" {
			progress("cf-zone", "running", "自动识别 Cloudflare Zone...", node.Domain)
			zoneID, err = findZoneIDByDomain(node.CFToken, node.CFAccountID, node.Domain)
			if err != nil {
				progress("cf-zone", "error", "Zone 识别失败", err.Error())
				return &DeployResult{Success: false, Error: err.Error(), Phase: phase, ProbeResults: probeResults}, err
			}
			progress("cf-zone", "ok", "Zone 识别成功", zoneID)
			node.CFZoneID = zoneID
		}

		progress("dns-bind", "running", "绑定域名到节点 IP...", fmt.Sprintf("%s -> %s", node.Domain, node.Host))
		if err := CloudflareUpsertARecord(node.CFToken, zoneID, node.Domain, node.Host); err != nil {
			progress("dns-bind", "error", "Cloudflare DNS 绑定失败", err.Error())
			return &DeployResult{Success: false, Error: err.Error(), Phase: phase, ProbeResults: probeResults}, err
		}
		progress("dns-bind", "ok", "Cloudflare DNS 绑定成功", node.Domain)

		progress("acme-install", "running", "正在安装 acme.sh...", "")
		out, err = client.Run(fmt.Sprintf("curl -s https://get.acme.sh | sh -s email=%s 2>&1", shellEscape(node.Email)))
		if err != nil {
			progress("acme-install", "error", "acme.sh 安装失败", err.Error())
			return &DeployResult{Success: false, Error: fmt.Sprintf("acme.sh: %v", err), Phase: phase, ProbeResults: probeResults}, err
		}
		progress("acme-install", "ok", "acme.sh 安装完成", "")

		progress("cert-issue", "running", fmt.Sprintf("正在为 %s 签发 TLS 证书...", node.Domain), "通过 Cloudflare DNS API")
		out, err = client.Run(fmt.Sprintf(`
export CF_Token=%s
export CF_Account_ID=%s
export CF_Zone_ID=%s
~/.acme.sh/acme.sh --issue -d %s --dns dns_cf --server letsencrypt 2>&1
`, shellEscape(node.CFToken), shellEscape(node.CFAccountID), shellEscape(zoneID), shellEscape(node.Domain)))
		if err != nil {
			progress("cert-issue", "error", "证书签发失败", fmt.Sprintf("%s\n%s", err.Error(), out))
			return &DeployResult{Success: false, Error: fmt.Sprintf("cert issue: %v", err), Phase: phase, ProbeResults: probeResults}, err
		}
		progress("cert-issue", "ok", "TLS 证书签发成功", node.Domain)
		certIssued = true

		progress("cert-verify", "running", "验证证书文件...", "")
		out, err = client.Run(fmt.Sprintf("ls -la ~/.acme.sh/%s* 2>/dev/null | head -n 5", shellEscape(node.Domain)))
		if err != nil || strings.TrimSpace(out) == "" {
			progress("cert-verify", "warning", "未找到证书文件清单", strings.TrimSpace(out))
		} else {
			progress("cert-verify", "ok", "证书文件已生成", strings.TrimSpace(out))
		}
	}

	return &DeployResult{
		Success:      true,
		GOSTVersion:  gostVersion,
		CertIssued:   certIssued,
		ProxyUser:    proxyUser,
		ProxyPass:    proxyPass,
		ServicePort:  443,
		Phase:        phase,
		ProbeResults: probeResults,
	}, nil
}

func shellEscape(v string) string {
	replacer := strings.NewReplacer("'", "'\\''")
	return "'" + replacer.Replace(v) + "'"
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
