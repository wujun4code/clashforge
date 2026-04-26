package netfilter

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// IptablesBackend manages iptables TProxy rules.
type IptablesBackend struct {
	TProxyPort        int
	DNSPort           int
	EnableDNSRedirect bool
}

// Apply sets up iptables TProxy rules.
func (i *IptablesBackend) Apply() error {
	_ = i.Cleanup()

	cmds := [][]string{
		// Create METACLASH chain
		{"iptables", "-t", "mangle", "-N", "METACLASH"},
		// Skip local/private addresses
		{"iptables", "-t", "mangle", "-A", "METACLASH", "-d", "0.0.0.0/8", "-j", "RETURN"},
		{"iptables", "-t", "mangle", "-A", "METACLASH", "-d", "10.0.0.0/8", "-j", "RETURN"},
		{"iptables", "-t", "mangle", "-A", "METACLASH", "-d", "127.0.0.0/8", "-j", "RETURN"},
		{"iptables", "-t", "mangle", "-A", "METACLASH", "-d", "172.16.0.0/12", "-j", "RETURN"},
		{"iptables", "-t", "mangle", "-A", "METACLASH", "-d", "192.168.0.0/16", "-j", "RETURN"},
		{"iptables", "-t", "mangle", "-A", "METACLASH", "-d", "224.0.0.0/4", "-j", "RETURN"},
		{"iptables", "-t", "mangle", "-A", "METACLASH", "-d", "240.0.0.0/4", "-j", "RETURN"},
		// TProxy TCP and UDP
		{"iptables", "-t", "mangle", "-A", "METACLASH", "-p", "tcp", "-j", "TPROXY",
			"--tproxy-mark", fwMark, "--on-port", strconv.Itoa(i.TProxyPort)},
		{"iptables", "-t", "mangle", "-A", "METACLASH", "-p", "udp", "-j", "TPROXY",
			"--tproxy-mark", fwMark, "--on-port", strconv.Itoa(i.TProxyPort)},
		// Hook into PREROUTING
		{"iptables", "-t", "mangle", "-A", "PREROUTING", "-j", "METACLASH"},
	}

	if i.EnableDNSRedirect {
		dnsPort := strconv.Itoa(i.DNSPort)
		cmds = append(cmds,
			// Redirect LAN client DNS (forwarded traffic via PREROUTING).
			[]string{"iptables", "-t", "nat", "-A", "PREROUTING", "-p", "udp", "--dport", "53", "-j", "REDIRECT", "--to-port", dnsPort},
			[]string{"iptables", "-t", "nat", "-A", "PREROUTING", "-p", "tcp", "--dport", "53", "-j", "REDIRECT", "--to-port", dnsPort},
			// Redirect router-local DNS (OUTPUT). Only loopback destinations are matched
			// so mihomo's own upstream queries to real IPs are left untouched.
			[]string{"iptables", "-t", "nat", "-A", "OUTPUT", "-p", "udp", "--dport", "53", "-d", "127.0.0.0/8", "-j", "REDIRECT", "--to-port", dnsPort},
			[]string{"iptables", "-t", "nat", "-A", "OUTPUT", "-p", "tcp", "--dport", "53", "-d", "127.0.0.0/8", "-j", "REDIRECT", "--to-port", dnsPort},
		)
	}

	for _, cmd := range cmds {
		if out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("iptables apply %v: %w: %s", cmd, err, string(out))
		}
	}

	// Policy routing
	_ = exec.Command("ip", "rule", "add", "fwmark", fwMark, "table", routeTable).Run()
	_ = exec.Command("ip", "route", "add", "local", "default", "dev", "lo", "table", routeTable).Run()
	return nil
}

// Cleanup removes all metaclash iptables rules.
func (i *IptablesBackend) Cleanup() error {
	cmds := [][]string{
		{"iptables", "-t", "mangle", "-D", "PREROUTING", "-j", "METACLASH"},
		{"iptables", "-t", "mangle", "-F", "METACLASH"},
		{"iptables", "-t", "mangle", "-X", "METACLASH"},
	}
	if i.EnableDNSRedirect {
		dnsPort := strconv.Itoa(i.DNSPort)
		cmds = append(cmds,
			[]string{"iptables", "-t", "nat", "-D", "PREROUTING", "-p", "udp", "--dport", "53", "-j", "REDIRECT", "--to-port", dnsPort},
			[]string{"iptables", "-t", "nat", "-D", "PREROUTING", "-p", "tcp", "--dport", "53", "-j", "REDIRECT", "--to-port", dnsPort},
			[]string{"iptables", "-t", "nat", "-D", "OUTPUT", "-p", "udp", "--dport", "53", "-d", "127.0.0.0/8", "-j", "REDIRECT", "--to-port", dnsPort},
			[]string{"iptables", "-t", "nat", "-D", "OUTPUT", "-p", "tcp", "--dport", "53", "-d", "127.0.0.0/8", "-j", "REDIRECT", "--to-port", dnsPort},
		)
	}
	for _, cmd := range cmds {
		_ = exec.Command(cmd[0], cmd[1:]...).Run()
	}
	_ = exec.Command("ip", "rule", "del", "fwmark", fwMark, "table", routeTable).Run()
	_ = exec.Command("ip", "route", "flush", "table", routeTable).Run()
	return nil
}

// IsIptablesError checks if the error is "not found" (safe to ignore during cleanup).
func IsIptablesError(err error) bool {
	return strings.Contains(err.Error(), "No chain/target/match by that name")
}
