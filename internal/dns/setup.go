package dns

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	dnsmasqConf             = "/etc/dnsmasq.conf"
	dnsmasqGeneratedConfGlob = "/var/etc/dnsmasq.conf*"
	dnsmasqDefaultDir       = "/etc/dnsmasq.d"
	clashforgeConfName      = "clashforge.conf"
	dnsmasqService          = "dnsmasq"
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

// setupReplace disables dnsmasq's DNS listener so mihomo can bind to port 53.
// On OpenWrt, uses UCI to set port=0 — writing port= to a conf-dir file would
// cause "illegal repeated keyword" because UCI already sets port=53 in the
// generated config. Falls back to a conf-dir file on non-OpenWrt systems.
func setupReplace() error {
	if _, err := exec.LookPath("uci"); err == nil {
		return setupReplaceUCI()
	}
	content := "# clashforge: disable dnsmasq DNS port so mihomo can take over\nport=0\n"
	if err := writeManagedDNSMasqConfig(content); err != nil {
		return err
	}
	return reloadDnsmasq()
}

func setupReplaceUCI() error {
	if out, err := exec.Command("uci", "set", "dhcp.@dnsmasq[0].port=0").CombinedOutput(); err != nil {
		return fmt.Errorf("uci set dnsmasq port=0: %w: %s", err, out)
	}
	if out, err := exec.Command("uci", "commit", "dhcp").CombinedOutput(); err != nil {
		return fmt.Errorf("uci commit dhcp: %w: %s", err, out)
	}
	log.Info().Msg("dns: dnsmasq port=0 set via UCI (replace mode)")
	return reloadDnsmasq()
}

func restoreReplace() error {
	if _, err := exec.LookPath("uci"); err == nil {
		return restoreReplaceUCI()
	}
	if err := removeManagedDNSMasqConfig(); err != nil {
		return err
	}
	return reloadDnsmasq()
}

func restoreReplaceUCI() error {
	// Remove the port override so dnsmasq returns to its default (53).
	exec.Command("uci", "delete", "dhcp.@dnsmasq[0].port").Run() //nolint:errcheck
	if out, err := exec.Command("uci", "commit", "dhcp").CombinedOutput(); err != nil {
		return fmt.Errorf("uci commit dhcp: %w: %s", err, out)
	}
	log.Info().Msg("dns: dnsmasq port restored via UCI (replace mode)")
	return reloadDnsmasq()
}

// setupUpstream configures dnsmasq to forward DNS queries to mihomo's DNS port.
func setupUpstream(port int) error {
	content := fmt.Sprintf(
		"# clashforge: forward all DNS to mihomo\nserver=127.0.0.1#%d\nno-resolv\n",
		port,
	)
	if err := writeManagedDNSMasqConfig(content); err != nil {
		return err
	}
	log.Info().Int("port", port).Msg("dns: dnsmasq upstream set to mihomo")
	return reloadDnsmasq()
}

func restoreUpstream() error {
	if err := removeManagedDNSMasqConfig(); err != nil {
		return err
	}
	return reloadDnsmasq()
}

func writeManagedDNSMasqConfig(content string) error {
	paths := dnsmasqManagedConfigPaths()
	written := 0
	var lastErr error

	for _, target := range paths {
		dir := filepath.Dir(target)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			lastErr = err
			continue
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			lastErr = err
			continue
		}
		written++
		log.Info().Str("file", target).Msg("dns: wrote managed dnsmasq config")
	}

	if written > 0 {
		return nil
	}
	if lastErr != nil {
		return fmt.Errorf("write managed dnsmasq config: %w", lastErr)
	}
	return fmt.Errorf("write managed dnsmasq config: no writable config path")
}

func removeManagedDNSMasqConfig() error {
	var lastErr error
	for _, target := range dnsmasqManagedConfigPaths() {
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			lastErr = err
		}
	}
	return lastErr
}

func dnsmasqManagedConfigPaths() []string {
	dirs := []string{dnsmasqDefaultDir}
	if confFiles, err := filepath.Glob(dnsmasqGeneratedConfGlob); err == nil {
		for _, confPath := range confFiles {
			f, err := os.Open(confPath)
			if err != nil {
				continue
			}
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				if dir := parseDnsmasqConfDir(scanner.Text()); dir != "" {
					// Skip transient OpenWrt working directories under /tmp —
					// they are recreated on every dnsmasq restart and must not
					// be written to directly (causes conf conflicts on reload).
					if strings.HasPrefix(filepath.ToSlash(dir), "/tmp/") {
						continue
					}
					dirs = append(dirs, dir)
				}
			}
			_ = f.Close()
		}
	}

	uniq := make(map[string]bool)
	paths := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		target := filepath.Join(dir, clashforgeConfName)
		if uniq[target] {
			continue
		}
		uniq[target] = true
		paths = append(paths, target)
	}
	return paths
}

func parseDnsmasqConfDir(line string) string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "conf-dir=") {
		return ""
	}
	dir := strings.TrimSpace(strings.TrimPrefix(line, "conf-dir="))
	if comma := strings.IndexByte(dir, ','); comma >= 0 {
		dir = dir[:comma]
	}
	return strings.TrimSpace(dir)
}

// reloadDnsmasq sends SIGHUP to dnsmasq (OpenWrt procd style).
// Falls back to service restart if SIGHUP fails.
func reloadDnsmasq() error {
	// Try SIGHUP via known pidfiles (including OpenWrt instance pidfiles).
	reloaded := false
	for _, pid := range dnsmasqPIDs() {
		cmd := exec.Command("kill", "-HUP", pid)
		if err := cmd.Run(); err == nil {
			reloaded = true
			log.Info().Str("pid", pid).Msg("dns: dnsmasq reloaded via SIGHUP")
		}
	}
	if reloaded {
		return nil
	}

	// Fallback: restart service to ensure both config and cache are refreshed.
	out, err := exec.Command("/etc/init.d/dnsmasq", "restart").CombinedOutput()
	if err != nil {
		// Not fatal – dnsmasq may not be running or on non-OpenWrt systems
		log.Warn().Err(err).Str("output", string(out)).Msg("dns: dnsmasq restart failed (not fatal)")
	} else {
		log.Info().Msg("dns: dnsmasq restarted via init.d")
	}
	return nil
}

func dnsmasqPIDs() []string {
	var pids []string
	seen := make(map[string]struct{})
	addPID := func(pid string) {
		pid = strings.TrimSpace(pid)
		if pid == "" {
			return
		}
		if _, ok := seen[pid]; ok {
			return
		}
		seen[pid] = struct{}{}
		pids = append(pids, pid)
	}

	candidates := []string{
		"/var/run/dnsmasq/dnsmasq.pid",
		"/var/run/dnsmasq.pid",
		"/run/dnsmasq.pid",
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil {
			addPID(string(data))
		}
	}

	if instancePIDFiles, err := filepath.Glob("/var/run/dnsmasq/dnsmasq.*.pid"); err == nil {
		for _, path := range instancePIDFiles {
			if data, readErr := os.ReadFile(path); readErr == nil {
				addPID(string(data))
			}
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
					addPID(string(data))
				}
			}
		}
	}

	return pids
}
