package netfilter

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// IptablesBackend manages iptables TProxy rules.
type IptablesBackend struct {
	TProxyPort        int
	DNSPort           int
	EnableDNSRedirect bool
	BypassFakeIP      bool
	BypassCIDR        []string
	EnableIPv6        bool
	DropQUIC          bool
	FakeIPRange       string // actual mihomo IPv4 fake-IP pool; empty uses the default
}

// Apply sets up iptables TProxy rules.
func (i *IptablesBackend) Apply() error {
	fakeIPRange := normalizedFakeIPRange(i.FakeIPRange)
	// Even with IPv6 interception disabled we need ip6tables to block external
	// IPv6 forwarding. Refuse setup rather than silently leaving a bypass.
	if _, err := exec.LookPath("ip6tables"); err != nil {
		return fmt.Errorf("ip6tables is required for IPv6 interception or leak protection: %w", err)
	}
	if err := i.Cleanup(); err != nil {
		return fmt.Errorf("cleanup before iptables apply: %w", err)
	}

	var cmds [][]string
	for _, family := range i.families() {
		add := func(args ...string) {
			cmds = append(cmds, append([]string{family, "-t", "mangle"}, args...))
		}
		add("-N", "METACLASH")
		add("-A", "METACLASH", "-m", "mark", "--mark", fwMark, "-j", "RETURN")
		// Re-routed fake-IP packets carry fwMarkOutput; do not skip them just
		// because their source belongs to the router.
		add("-A", "METACLASH", "-m", "addrtype", "--src-type", "LOCAL", "-m", "mark", "!", "--mark", fwMarkOutput, "-j", "RETURN")
		var bypass []string
		if family == "iptables" {
			bypass = strings.Split(buildBypassIPv4Elements(i.BypassFakeIP, i.BypassCIDR, fakeIPRange), ",")
		} else {
			bypass = []string{"::1/128", "fc00::/7", "fe80::/10", "ff00::/8", "100::/64"}
		}
		for _, cidr := range bypass {
			add("-A", "METACLASH", "-d", strings.TrimSpace(cidr), "-j", "RETURN")
		}
		// DROP is conservative; TCP fallback depends on the client. When
		// disabled, QUIC follows the ordinary UDP TProxy rule, never RETURN.
		if i.DropQUIC {
			add("-A", "METACLASH", "-p", "udp", "--dport", "443", "-j", "DROP")
		}
		for _, protocol := range []string{"tcp", "udp"} {
			add("-A", "METACLASH", "-p", protocol, "-j", "TPROXY", "--tproxy-mark", fwMark, "--on-port", strconv.Itoa(i.TProxyPort))
		}
		add("-A", "PREROUTING", "-j", "METACLASH")
	}
	if !i.EnableIPv6 {
		cmds = append(cmds, []string{"ip6tables", "-t", "filter", "-N", "METACLASH_IPV6"})
		for _, cidr := range []string{"::1/128", "fc00::/7", "fe80::/10", "ff00::/8"} {
			cmds = append(cmds, []string{"ip6tables", "-t", "filter", "-A", "METACLASH_IPV6", "-d", cidr, "-j", "RETURN"})
		}
		cmds = append(cmds,
			[]string{"ip6tables", "-t", "filter", "-A", "METACLASH_IPV6", "-j", "DROP"},
			// Precede existing ACCEPT rules in the built-in FORWARD chain.
			[]string{"ip6tables", "-t", "filter", "-I", "FORWARD", "1", "-j", "METACLASH_IPV6"},
		)
	}
	// Match the nft backend's deliberately narrow router-local scope: only
	// IPv4 fake-IP destinations, not real-IP upstreams used by the proxy core.
	cmds = append(cmds,
		[]string{"iptables", "-t", "mangle", "-N", "METACLASH_OUTPUT"},
		[]string{"iptables", "-t", "mangle", "-A", "METACLASH_OUTPUT", "-m", "mark", "--mark", fwMark, "-j", "RETURN"},
		[]string{"iptables", "-t", "mangle", "-A", "METACLASH_OUTPUT", "-m", "mark", "--mark", fwMarkOutput, "-j", "RETURN"},
	)
	for _, protocol := range []string{"tcp", "udp"} {
		cmds = append(cmds, []string{"iptables", "-t", "mangle", "-A", "METACLASH_OUTPUT", "-d", fakeIPRange, "-p", protocol, "-j", "MARK", "--set-mark", fwMarkOutput})
	}
	cmds = append(cmds, []string{"iptables", "-t", "mangle", "-A", "OUTPUT", "-j", "METACLASH_OUTPUT"})

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

	// Policy routing for intercepted LAN traffic and router-local fake IPs.
	return applyTProxyPolicyRouting(i.EnableIPv6)
}

func (i *IptablesBackend) families() []string {
	if i.EnableIPv6 {
		return []string{"iptables", "ip6tables"}
	}
	return []string{"iptables"}
}

// Cleanup removes all metaclash iptables rules.
func (i *IptablesBackend) Cleanup() error {
	// Remove both families regardless of the current setting (mode switches may
	// have left chains from an earlier IPv6-enabled configuration).
	var cmds [][]string
	for _, family := range []string{"iptables", "ip6tables"} {
		cmds = append(cmds,
			[]string{family, "-t", "mangle", "-D", "PREROUTING", "-j", "METACLASH"},
			[]string{family, "-t", "mangle", "-F", "METACLASH"},
			[]string{family, "-t", "mangle", "-X", "METACLASH"},
		)
	}
	cmds = append(cmds,
		[]string{"iptables", "-t", "mangle", "-D", "OUTPUT", "-j", "METACLASH_OUTPUT"},
		[]string{"iptables", "-t", "mangle", "-F", "METACLASH_OUTPUT"},
		[]string{"iptables", "-t", "mangle", "-X", "METACLASH_OUTPUT"},
		[]string{"ip6tables", "-t", "filter", "-D", "FORWARD", "-j", "METACLASH_IPV6"},
		[]string{"ip6tables", "-t", "filter", "-F", "METACLASH_IPV6"},
		[]string{"ip6tables", "-t", "filter", "-X", "METACLASH_IPV6"},
	)
	if i.EnableDNSRedirect {
		dnsPort := strconv.Itoa(i.DNSPort)
		cmds = append(cmds,
			[]string{"iptables", "-t", "nat", "-D", "PREROUTING", "-p", "udp", "--dport", "53", "-j", "REDIRECT", "--to-port", dnsPort},
			[]string{"iptables", "-t", "nat", "-D", "PREROUTING", "-p", "tcp", "--dport", "53", "-j", "REDIRECT", "--to-port", dnsPort},
			[]string{"iptables", "-t", "nat", "-D", "OUTPUT", "-p", "udp", "--dport", "53", "-d", "127.0.0.0/8", "-j", "REDIRECT", "--to-port", dnsPort},
			[]string{"iptables", "-t", "nat", "-D", "OUTPUT", "-p", "tcp", "--dport", "53", "-d", "127.0.0.0/8", "-j", "REDIRECT", "--to-port", dnsPort},
		)
	}
	var errs []error
	for _, cmd := range cmds {
		if out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
			// Only known absent-chain/rule diagnostics are idempotent. In
			// particular missing executables, permission and busy-chain errors
			// must retain the manager's pending ownership for another attempt.
			s := string(out)
			if !iptablesResourceAbsent(s) {
				errs = append(errs, fmt.Errorf("iptables cleanup %s: %w: %s", strings.Join(cmd, " "), err, s))
			}
		}
	}
	return errors.Join(append(errs, cleanupTProxyPolicyRouting())...)
}

// IsIptablesError checks if the error is "not found" (safe to ignore during cleanup).
func IsIptablesError(err error) bool {
	return err != nil && iptablesResourceAbsent(err.Error())
}

var iptablesMissingChain = regexp.MustCompile(`Chain '[^']+' does not exist`)

func iptablesResourceAbsent(s string) bool {
	return strings.Contains(s, "No chain/target/match by that name") ||
		strings.Contains(s, "Bad rule (does a matching rule exist in that chain?).") ||
		iptablesMissingChain.MatchString(s) ||
		strings.Contains(s, "RULE_DELETE failed (No such file or directory): rule in chain ")
}
