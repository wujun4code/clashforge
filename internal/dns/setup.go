package dns

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/rs/zerolog/log"
)

// DnsmasqMode controls how clashforge coexists with dnsmasq.
type DnsmasqMode string

const (
	// ModeReplace: disable dnsmasq's DNS port (port=0), mihomo takes over.
	ModeReplace DnsmasqMode = "replace"
	// ModeUpstream: keep dnsmasq, set its upstream to mihomo DNS port.
	ModeUpstream DnsmasqMode = "upstream"
	// ModeNone: do nothing to dnsmasq.
	ModeNone DnsmasqMode = "none"
)

const (
	dnsmasqConf         = "/etc/dnsmasq.conf"
	dnsmasqDir          = "/etc/dnsmasq.d"
	clashforgeConf      = "/etc/dnsmasq.d/clashforge.conf"
	dnsmasqService      = "dnsmasq"
)

// Setup configures dnsmasq coexistence according to mode.
func Setup(mode DnsmasqMode, mihmoDNSPort int) error {
	switch mode {
	case ModeReplace:
		return setupReplace()
	case ModeUpstream:
		return setupUpstream(mihmoDNSPort)
	case ModeNone:
		log.Info().Msg("dns: dnsmasq_mode=none, skipping dnsmasq configuration")
		return nil
	default:
		return fmt.Errorf("unknown dnsmasq_mode: %s", mode)
	}
}

// Restore undoes changes made by Setup.
func Restore(mode DnsmasqMode) error {
	switch mode {
	case ModeReplace:
		return restoreReplace()
	case ModeUpstream:
		return restoreUpstream()
	default:
		return nil
	}
}

// setupReplace sets port=0 in dnsmasq to disable its DNS listener.
func setupReplace() error {
	if err := os.MkdirAll(dnsmasqDir, 0o755); err != nil {
		return err
	}
	content := "# clashforge: disable dnsmasq DNS port so mihomo can take over\nport=0\n"
	if err := os.WriteFile(clashforgeConf, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", clashforgeConf, err)
	}
	log.Info().Str("file", clashforgeConf).Msg("dns: dnsmasq port=0 written")
	return reloadDnsmasq()
}

func restoreReplace() error {
	if err := os.Remove(clashforgeConf); err != nil && !os.IsNotExist(err) {
		return err
	}
	return reloadDnsmasq()
}

// setupUpstream configures dnsmasq to forward DNS queries to mihomo's DNS port.
func setupUpstream(port int) error {
	if err := os.MkdirAll(dnsmasqDir, 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf(
		"# clashforge: forward all DNS to mihomo\nserver=127.0.0.1#%d\nno-resolv\n",
		port,
	)
	if err := os.WriteFile(clashforgeConf, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", clashforgeConf, err)
	}
	log.Info().Int("port", port).Str("file", clashforgeConf).Msg("dns: dnsmasq upstream set to mihomo")
	return reloadDnsmasq()
}

func restoreUpstream() error {
	if err := os.Remove(clashforgeConf); err != nil && !os.IsNotExist(err) {
		return err
	}
	return reloadDnsmasq()
}

// reloadDnsmasq sends SIGHUP to dnsmasq (OpenWrt procd style).
// Falls back to service restart if SIGHUP fails.
func reloadDnsmasq() error {
	// Try SIGHUP via pidfile
	if pid := dnsmasqPID(); pid != "" {
		cmd := exec.Command("kill", "-HUP", pid)
		if err := cmd.Run(); err == nil {
			log.Info().Str("pid", pid).Msg("dns: dnsmasq reloaded via SIGHUP")
			return nil
		}
	}
	// Fallback: /etc/init.d/dnsmasq reload
	out, err := exec.Command("/etc/init.d/dnsmasq", "reload").CombinedOutput()
	if err != nil {
		// Not fatal – dnsmasq may not be running or on non-OpenWrt systems
		log.Warn().Err(err).Str("output", string(out)).Msg("dns: dnsmasq reload failed (not fatal)")
	} else {
		log.Info().Msg("dns: dnsmasq reloaded via init.d")
	}
	return nil
}

func dnsmasqPID() string {
	candidates := []string{
		"/var/run/dnsmasq/dnsmasq.pid",
		"/var/run/dnsmasq.pid",
		"/run/dnsmasq.pid",
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	// Try parsing /etc/dnsmasq.conf for pid-file= directive
	if f, err := os.Open(dnsmasqConf); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "pid-file=") {
				pidPath := strings.TrimPrefix(line, "pid-file=")
				if data, err := os.ReadFile(pidPath); err == nil {
					return strings.TrimSpace(string(data))
				}
			}
		}
	}
	return ""
}
