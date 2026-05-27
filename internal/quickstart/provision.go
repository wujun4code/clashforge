package quickstart

import (
	"fmt"
	"strings"
)

// DetectEnv runs basic fingerprinting commands over SSH to discover the VPS environment.
func DetectEnv(c *SSHClient) (*EnvInfo, error) {
	info := &EnvInfo{}

	// OS / version
	if out, err := c.RunOutput(`cat /etc/os-release 2>/dev/null | grep -E '^(ID|VERSION_ID)=' | tr '\n' ' '`); err == nil {
		info.OS, info.OSVersion = parseOSRelease(out)
	}

	// Architecture
	if arch, err := c.RunOutput("uname -m"); err == nil {
		switch strings.TrimSpace(arch) {
		case "x86_64":
			info.Arch = "amd64"
		case "aarch64", "arm64":
			info.Arch = "arm64"
		default:
			info.Arch = strings.TrimSpace(arch)
		}
	}

	// Systemd
	if _, err := c.RunOutput("systemctl --version 2>/dev/null | head -1"); err == nil {
		info.HasSystemd = true
	}

	// Firewall
	if _, err := c.RunOutput("which ufw 2>/dev/null"); err == nil {
		info.Firewall = "ufw"
	} else if _, err := c.RunOutput("which firewall-cmd 2>/dev/null"); err == nil {
		info.Firewall = "firewalld"
	} else if _, err := c.RunOutput("which iptables 2>/dev/null"); err == nil {
		info.Firewall = "iptables"
	}

	// Port 443 in use?
	if out, err := c.RunOutput("ss -tlnp 'sport = :443' 2>/dev/null | grep -c ':443'"); err == nil {
		info.Port443In = strings.TrimSpace(out) != "0" && strings.TrimSpace(out) != ""
	}

	return info, nil
}

// InstallGost downloads and installs gost on the VPS via SSH.
func InstallGost(c *SSHClient, info *EnvInfo) error {
	arch := info.Arch
	if arch == "" {
		arch = "amd64"
	}

	// Build download URL for go-gost v3
	version := GostVersion
	tarName := fmt.Sprintf("gost_%s_linux_%s.tar.gz", version, arch)
	dlURL := fmt.Sprintf("https://github.com/go-gost/gost/releases/download/v%s/%s", version, tarName)

	// Download with curl; try ghproxy mirror on failure (GitHub may be blocked)
	downloadCmd := fmt.Sprintf(
		`curl -fsSL "%s" -o /tmp/gost.tar.gz 2>/dev/null || curl -fsSL "https://ghproxy.com/%s" -o /tmp/gost.tar.gz`,
		dlURL, dlURL,
	)
	if _, err := c.Run(downloadCmd); err != nil {
		return fmt.Errorf("download gost: %w", err)
	}

	// Extract gost binary
	if _, err := c.Run(`tar -xzf /tmp/gost.tar.gz -C /tmp/ && mv /tmp/gost /usr/local/bin/gost && chmod +x /usr/local/bin/gost`); err != nil {
		return fmt.Errorf("extract gost: %w", err)
	}

	// Create directories
	if err := c.MkdirP(GostCfgDir); err != nil {
		return fmt.Errorf("create gost config dir: %w", err)
	}
	if err := c.MkdirP(GostCertDir); err != nil {
		return fmt.Errorf("create gost cert dir: %w", err)
	}

	return nil
}

// WriteGostConfig uploads the gost config.yaml and systemd unit, then enables and starts the service.
func WriteGostConfig(c *SSHClient) error {
	// Write config.yaml
	if err := c.WriteFile(GostCfgPath, BuildGostConfig(), "0644"); err != nil {
		return fmt.Errorf("write gost config: %w", err)
	}

	// Write systemd unit
	unitPath := "/etc/systemd/system/gost.service"
	if err := c.WriteFile(unitPath, GostSystemdUnit(), "0644"); err != nil {
		return fmt.Errorf("write gost systemd unit: %w", err)
	}

	// Enable + start
	if _, err := c.Run("systemctl daemon-reload && systemctl enable gost && systemctl restart gost"); err != nil {
		return fmt.Errorf("start gost service: %w", err)
	}
	return nil
}

// AllowPort443 opens TCP port 443 in the VPS firewall.
func AllowPort443(c *SSHClient, firewall string) error {
	switch firewall {
	case "ufw":
		_, err := c.Run("ufw allow 443/tcp")
		return err
	case "firewalld":
		_, err := c.Run("firewall-cmd --permanent --add-port=443/tcp && firewall-cmd --reload")
		return err
	case "iptables":
		_, err := c.Run("iptables -I INPUT -p tcp --dport 443 -j ACCEPT")
		return err
	default:
		// No recognised firewall — assume open or unmanaged
		return nil
	}
}

// UploadCert uploads fullchain.pem and privkey.pem to the VPS cert directory.
func UploadCert(c *SSHClient, fullchainPEM, privkeyPEM string) error {
	if err := c.WriteFile(GostCertPEM, fullchainPEM, "0644"); err != nil {
		return fmt.Errorf("upload fullchain.pem: %w", err)
	}
	if err := c.WriteFile(GostKeyPEM, privkeyPEM, "0600"); err != nil {
		return fmt.Errorf("upload privkey.pem: %w", err)
	}
	return nil
}

// parseOSRelease extracts OS name and version from /etc/os-release output.
func parseOSRelease(raw string) (os, version string) {
	for _, field := range strings.Fields(raw) {
		kv := strings.SplitN(field, "=", 2)
		if len(kv) != 2 {
			continue
		}
		val := strings.Trim(kv[1], `"`)
		switch kv[0] {
		case "ID":
			os = strings.ToLower(val)
		case "VERSION_ID":
			version = val
		}
	}
	return
}
