package nodes

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
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

const gostTLSConfigTemplate = `services:
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
    type: tls
    tls:
      certFile: "%s"
      keyFile: "%s"
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

const (
	gostConfigPath     = "/etc/gost/gost.yaml"
	gostServicePath    = "/etc/systemd/system/gost.service"
	gostCertDir        = "/etc/gost/certs"
	gostFullchainPath  = gostCertDir + "/fullchain.pem"
	gostPrivateKeyPath = gostCertDir + "/key.pem"
)

const (
	acmeDNSWaitPrimarySeconds  = 120
	acmeDNSWaitFallbackSeconds = 300
)

type acmeIssueAttempt struct {
	Server   string
	Label    string
	DNSSleep int
}

const defaultACMECAChain = "letsencrypt,zerossl,google,sslcom,actalis"

var acmeProviderLabels = map[string]string{
	"letsencrypt":      "Let's Encrypt",
	"letsencrypt_test": "Let's Encrypt (Staging)",
	"zerossl":          "ZeroSSL",
	"sslcom":           "SSL.com",
	"google":           "Google Public CA",
	"googletest":       "Google Public CA (Test)",
	"actalis":          "Actalis",
}

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

	gostVersion := ""
	certIssued := false
	probeResults := []ProbeResult{}

	if fullMode {
		progress("gost-check", "running", "正在检查现有 GOST 部署...", "")
		out, err = client.Run(fmt.Sprintf(`
set +e
if command -v gost >/dev/null 2>&1 || [ -x /usr/local/bin/gost ]; then
  echo "HAS_GOST_BIN"
else
  echo "MISSING_GOST_BIN"
fi
if [ -f /etc/gost/gost.yaml ] || [ -f %s ] || systemctl status gost >/dev/null 2>&1 || sudo systemctl status gost >/dev/null 2>&1; then
  echo "HAS_GOST_STATE"
else
  echo "MISSING_GOST_STATE"
fi
`, shellEscape(gostServicePath)))
		if err != nil {
			progress("gost-check", "error", "检查既有部署失败", err.Error())
			return &DeployResult{Success: false, Error: fmt.Sprintf("gost check failed: %v", err), Phase: phase}, err
		}
		if !strings.Contains(out, "HAS_GOST_BIN") || !strings.Contains(out, "HAS_GOST_STATE") {
			msg := "未检测到既有 GOST 部署，请先完成第3步基础部署"
			progress("gost-check", "error", msg, strings.TrimSpace(out))
			checkErr := fmt.Errorf(msg)
			return &DeployResult{Success: false, Error: msg, Phase: phase}, checkErr
		}

		out, _ = client.Run("gost -V 2>&1 || /usr/local/bin/gost -V 2>&1 || echo v0.0.0")
		gostVersion = strings.TrimSpace(strings.Replace(out, "gost version ", "", 1))
		progress("gost-check", "ok", fmt.Sprintf("检测到 GOST 版本: %s", gostVersion), "复用现有安装，不重复安装")
	} else {
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
		gostVersion = strings.TrimSpace(strings.Replace(out, "gost version ", "", 1))
		progress("gost-verify", "ok", fmt.Sprintf("GOST 版本: %s", gostVersion), "")

		progress("config-write", "running", "正在生成 GOST 配置文件...", "")
		gostYAML := fmt.Sprintf(gostConfigTemplate, proxyUser, proxyPass)
		out, err = client.Run(fmt.Sprintf(
			"(mkdir -p /etc/gost && printf '%%s' %s | tee %s > /dev/null) || "+
				"(sudo mkdir -p /etc/gost && printf '%%s' %s | sudo tee %s > /dev/null)",
			shellEscape(gostYAML),
			shellEscape(gostConfigPath),
			shellEscape(gostYAML),
			shellEscape(gostConfigPath),
		))
		if err != nil {
			progress("config-write", "error", "配置写入失败", err.Error())
			return &DeployResult{Success: false, Error: fmt.Sprintf("write config: %v", err), Phase: phase}, err
		}
		progress("config-write", "ok", "GOST 配置已写入", gostConfigPath)

		progress("systemd", "running", "正在注册 systemd 服务...", "")
		out, err = client.Run(fmt.Sprintf(`
set -e
cat > /tmp/gost.service << 'SYSTEMDEOF'
%s
SYSTEMDEOF
if ! command -v systemctl >/dev/null 2>&1; then
  echo "NO_SYSTEMD"
  exit 10
fi
run_root() {
  "$@" && return 0
  sudo "$@"
}
run_root mkdir -p /etc/systemd/system
run_root mv /tmp/gost.service /etc/systemd/system/gost.service
run_root chmod 644 /etc/systemd/system/gost.service
run_root systemctl daemon-reload
run_root systemctl enable gost
run_root systemctl restart gost
run_root systemctl is-active gost
`, gostServiceTemplate))
		if err != nil || !systemdOutputIsActive(out) {
			statusOut, _ := client.Run("(systemctl status gost --no-pager -l 2>&1 || sudo systemctl status gost --no-pager -l 2>&1 || true)")
			detail := strings.TrimSpace(fmt.Sprintf("%v\n%s", err, out))
			if strings.TrimSpace(statusOut) != "" {
				detail = detail + "\n--- systemctl status gost ---\n" + strings.TrimSpace(statusOut)
			}
			progress("systemd", "error", "systemd 服务启动失败", detail)
			startErr := err
			if startErr == nil {
				startErr = fmt.Errorf("gost service is not active")
			}
			return &DeployResult{Success: false, Error: "systemd service failed to start", Phase: phase}, startErr
		}
		progress("systemd", "ok", "GOST 服务已启动", "systemctl status gost")

		progress("probe", "running", "IP 直连探测中...", "通过代理访问 Google / YouTube / GitHub")
		probeResults = TestHTTPProxy(node.Host, 443, proxyUser, proxyPass, 10*time.Second, DefaultProbeTargets())
		probeOK := 0
		for _, p := range probeResults {
			if p.OK {
				probeOK++
			}
		}
		progress("probe", "ok", fmt.Sprintf("探测完成 (%d/%d 通过)", probeOK, len(probeResults)), "")
	}

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

		progress("acme-ready", "running", "检查 acme.sh 环境...", "")
		out, err = client.Run(fmt.Sprintf(`
set -e
if [ -x "$HOME/.acme.sh/acme.sh" ]; then
  echo "ACME_EXISTS"
else
  curl -s https://get.acme.sh | sh -s email=%s 2>&1
  echo "ACME_INSTALLED"
fi
`, shellEscape(node.Email)))
		if err != nil {
			progress("acme-ready", "error", "acme.sh 初始化失败", err.Error())
			return &DeployResult{Success: false, Error: fmt.Sprintf("acme.sh: %v", err), Phase: phase, ProbeResults: probeResults}, err
		}
		acmeDetail := "已复用现有 acme.sh"
		if strings.Contains(out, "ACME_INSTALLED") {
			acmeDetail = "acme.sh 已安装"
		}
		progress("acme-ready", "ok", "acme.sh 环境就绪", acmeDetail)

		caUsed, issueOut, issueErr := issueCertificateWithFallback(client, node, zoneID, progress)
		if issueErr != nil {
			progress("cert-issue", "error", "证书签发失败", issueErr.Error())
			return &DeployResult{Success: false, Error: issueErr.Error(), Phase: phase, ProbeResults: probeResults}, issueErr
		}
		progress("cert-issue", "ok", "TLS 证书签发成功", fmt.Sprintf("CA=%s · %s", caUsed, node.Domain))
		out = issueOut
		certIssued = true

		progress("cert-install", "running", "正在部署证书到 GOST 目录...", gostCertDir)
		out, err = client.Run(fmt.Sprintf(`
set -e
TMP_CERT_DIR=/tmp/clashforge-gost-certs
mkdir -p "$TMP_CERT_DIR"
~/.acme.sh/acme.sh --install-cert -d %s \
  --key-file "$TMP_CERT_DIR/key.pem" \
  --fullchain-file "$TMP_CERT_DIR/fullchain.pem" 2>&1
run_root() {
  "$@" && return 0
  sudo "$@"
}
run_root mkdir -p %s
run_root cp "$TMP_CERT_DIR/fullchain.pem" %s
run_root cp "$TMP_CERT_DIR/key.pem" %s
run_root chmod 644 %s
run_root chmod 600 %s
`, shellEscape(node.Domain), shellEscape(gostCertDir), shellEscape(gostFullchainPath), shellEscape(gostPrivateKeyPath), shellEscape(gostFullchainPath), shellEscape(gostPrivateKeyPath)))
		if err != nil {
			progress("cert-install", "error", "证书部署失败", commandErrorDetail(err, out))
			return &DeployResult{Success: false, Error: fmt.Sprintf("install cert: %v", err), Phase: phase, ProbeResults: probeResults}, err
		}
		progress("cert-install", "ok", "证书已部署", fmt.Sprintf("%s, %s", gostFullchainPath, gostPrivateKeyPath))

		progress("config-write", "running", "更新 GOST TLS 配置...", gostConfigPath)
		tlsYAML := fmt.Sprintf(gostTLSConfigTemplate, proxyUser, proxyPass, gostFullchainPath, gostPrivateKeyPath)
		out, err = client.Run(fmt.Sprintf(
			"(mkdir -p /etc/gost && printf '%%s' %s | tee %s > /dev/null) || "+
				"(sudo mkdir -p /etc/gost && printf '%%s' %s | sudo tee %s > /dev/null)",
			shellEscape(tlsYAML),
			shellEscape(gostConfigPath),
			shellEscape(tlsYAML),
			shellEscape(gostConfigPath),
		))
		if err != nil {
			progress("config-write", "error", "TLS 配置写入失败", err.Error())
			return &DeployResult{Success: false, Error: fmt.Sprintf("write tls config: %v", err), Phase: phase, ProbeResults: probeResults}, err
		}
		progress("config-write", "ok", "TLS 配置写入完成", gostConfigPath)

		progress("systemd-restart", "running", "正在重启 GOST 服务...", "")
		out, err = client.Run(fmt.Sprintf(`
set -e
run_root() {
  "$@" && return 0
  sudo "$@"
}
if [ ! -f %[1]s ] && ! run_root test -f %[1]s; then
  if [ -f /tmp/gost.service ]; then
    run_root mv /tmp/gost.service %[1]s
  fi
fi
run_root systemctl daemon-reload
run_root systemctl enable gost
run_root systemctl restart gost
run_root systemctl is-active gost
`, shellEscape(gostServicePath)))
		if err != nil || !systemdOutputIsActive(out) {
			statusOut, _ := client.Run("(systemctl status gost --no-pager -l 2>&1 || sudo systemctl status gost --no-pager -l 2>&1 || true)")
			detail := strings.TrimSpace(fmt.Sprintf("%v\n%s", err, out))
			if strings.TrimSpace(statusOut) != "" {
				detail = detail + "\n--- systemctl status gost ---\n" + strings.TrimSpace(statusOut)
			}
			progress("systemd-restart", "error", "GOST 重启失败", detail)
			startErr := err
			if startErr == nil {
				startErr = fmt.Errorf("gost service is not active")
			}
			return &DeployResult{Success: false, Error: "systemd service failed to start", Phase: phase, ProbeResults: probeResults}, startErr
		}
		progress("systemd-restart", "ok", "GOST 服务已重启", "TLS 模式已生效")

		progress("probe-domain", "running", "域名链路探测中...", "通过代理访问 Google / YouTube / GitHub")
		probeResults = TestHTTPProxyWithOptions(
			node.Domain,
			443,
			proxyUser,
			proxyPass,
			10*time.Second,
			DefaultProbeTargets(),
			ProxyProbeOptions{ProxyScheme: "https"},
		)
		probeOK := 0
		for _, p := range probeResults {
			if p.OK {
				probeOK++
			}
		}
		progress("probe-domain", "ok", fmt.Sprintf("域名探测完成 (%d/%d 通过)", probeOK, len(probeResults)), node.Domain)
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

func systemdOutputIsActive(out string) bool {
	lines := strings.Split(out, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		return line == "active"
	}
	return false
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

func issueCertificateWithFallback(
	client *SSHClient,
	node *Node,
	zoneID string,
	progress DeployProgress,
) (string, string, error) {
	domain := strings.TrimSpace(node.Domain)
	email := strings.TrimSpace(node.Email)
	cfToken := strings.TrimSpace(node.CFToken)
	cfAccountID := strings.TrimSpace(node.CFAccountID)
	zoneID = strings.TrimSpace(zoneID)

	if domain == "" {
		return "", "", fmt.Errorf("domain is required for certificate issuing")
	}
	if email == "" {
		return "", "", fmt.Errorf("acme email is required for certificate issuing")
	}
	if cfToken == "" {
		return "", "", fmt.Errorf("cloudflare token is required for certificate issuing")
	}

	attempts := buildACMEIssueAttempts()

	var mergedLogs []string

	for idx, attempt := range attempts {
		progress(
			"cert-issue",
			"running",
			fmt.Sprintf("证书签发中（%d/%d）", idx+1, len(attempts)),
			fmt.Sprintf("CA=%s · DNS 等待=%ds", attempt.Label, attempt.DNSSleep),
		)

		cmd := fmt.Sprintf(`
set -e
export CF_Token=%s
%s
%s
~/.acme.sh/acme.sh --register-account -m %s --server %s 2>&1 || true
~/.acme.sh/acme.sh --issue -d %s --dns dns_cf --server %s --dnssleep %d --keylength ec-256 2>&1
`,
			shellEscape(cfToken),
			optionalExport("CF_Account_ID", cfAccountID),
			optionalExport("CF_Zone_ID", zoneID),
			shellEscape(email),
			shellEscape(attempt.Server),
			shellEscape(domain),
			shellEscape(attempt.Server),
			attempt.DNSSleep,
		)

		out, err := client.Run(cmd)
		if err == nil {
			return attempt.Label, strings.TrimSpace(out), nil
		}

		detail := commandErrorDetail(err, out)
		mergedLogs = append(mergedLogs, fmt.Sprintf("CA=%s\n%s", attempt.Label, strings.TrimSpace(detail)))
		progress("cert-issue", "warning", fmt.Sprintf("%s 签发失败，准备回退", attempt.Label), "将尝试下一可用 CA")
	}

	if len(mergedLogs) == 0 {
		return "", "", fmt.Errorf("certificate issuing failed for unknown reason")
	}
	return "", strings.Join(mergedLogs, "\n\n---\n\n"), fmt.Errorf("all ACME providers failed:\n%s", strings.Join(mergedLogs, "\n\n---\n\n"))
}

func buildACMEIssueAttempts() []acmeIssueAttempt {
	rawChain := strings.TrimSpace(os.Getenv("CLASHFORGE_ACME_CA_CHAIN"))
	if rawChain == "" {
		rawChain = defaultACMECAChain
	}
	servers := parseACMECAServers(rawChain)
	if len(servers) == 0 {
		servers = parseACMECAServers(defaultACMECAChain)
	}
	if len(servers) == 0 {
		servers = []string{"letsencrypt"}
	}

	attempts := make([]acmeIssueAttempt, 0, len(servers)+1)
	first := servers[0]
	firstLabel := acmeProviderLabel(first)
	attempts = append(attempts,
		acmeIssueAttempt{Server: first, Label: firstLabel, DNSSleep: acmeDNSWaitPrimarySeconds},
		acmeIssueAttempt{Server: first, Label: firstLabel + " (延长等待)", DNSSleep: acmeDNSWaitFallbackSeconds},
	)
	for i := 1; i < len(servers); i++ {
		server := servers[i]
		attempts = append(attempts, acmeIssueAttempt{
			Server:   server,
			Label:    acmeProviderLabel(server),
			DNSSleep: acmeDNSWaitFallbackSeconds,
		})
	}
	return attempts
}

func parseACMECAServers(raw string) []string {
	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		server := strings.ToLower(strings.TrimSpace(part))
		if server == "" {
			continue
		}
		if !isSupportedACMEServer(server) {
			continue
		}
		if _, ok := seen[server]; ok {
			continue
		}
		seen[server] = struct{}{}
		out = append(out, server)
	}
	return out
}

func isSupportedACMEServer(server string) bool {
	if _, ok := acmeProviderLabels[server]; ok {
		return true
	}
	return strings.HasPrefix(server, "https://")
}

func acmeProviderLabel(server string) string {
	if label, ok := acmeProviderLabels[server]; ok {
		return label
	}
	return server
}

func optionalExport(name, value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	return fmt.Sprintf("export %s=%s", name, shellEscape(v))
}

func commandErrorDetail(err error, out string) string {
	var errMsg string
	if err != nil {
		errMsg = strings.TrimSpace(err.Error())
	}
	outMsg := strings.TrimSpace(out)

	if errMsg == "" {
		return outMsg
	}
	if outMsg == "" {
		return errMsg
	}
	if strings.Contains(errMsg, outMsg) {
		return errMsg
	}
	return errMsg + "\n" + outMsg
}
