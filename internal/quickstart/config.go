package quickstart

import "fmt"

const (
	GostBin     = "/usr/local/bin/gost"
	GostCfgDir  = "/etc/gost"
	GostCfgPath = "/etc/gost/config.yaml"
	GostCertDir = "/etc/gost/cert"
	GostCertPEM = "/etc/gost/cert/fullchain.pem"
	GostKeyPEM  = "/etc/gost/cert/privkey.pem"
	GostService = "gost"
)

// GostVersion is the version of gost to install on the VPS.
// Updated alongside ClashForge releases.
const GostVersion = "3.0.0"

// BuildGostConfig returns the content of /etc/gost/config.yaml.
// Uses HTTP CONNECT+TLS with basic auth, knock protection, and probe resistance —
// mirroring the security posture of the /nodes deployment pipeline.
func BuildGostConfig(user, pass string) string {
	return fmt.Sprintf(`services:
  - name: clashforge-node
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
        certFile: %s
        keyFile: %s
`, user, pass, GostCertPEM, GostKeyPEM)
}

// GostSystemdUnit returns the content of /etc/systemd/system/gost.service.
func GostSystemdUnit() string {
	return fmt.Sprintf(`[Unit]
Description=Gost Proxy Service (ClashForge)
After=network.target

[Service]
Type=simple
ExecStart=%s -C %s
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`, GostBin, GostCfgPath)
}
