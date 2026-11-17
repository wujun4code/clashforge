package netfilter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
)

// All external programs are replaced with local recording stubs. These tests
// never execute a firewall command or alter host/router networking.
func recordFirewallCommands(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	output := filepath.Join(dir, "commands")
	for _, name := range []string{"iptables", "ip6tables", "ip", "nft", "modprobe"} {
		script := "#!/bin/sh\nprintf '%s' '" + name + "' >> \"$QUIC_TEST_COMMANDS\"\nprintf ' %s' \"$@\" >> \"$QUIC_TEST_COMMANDS\"\nprintf '\\n' >> \"$QUIC_TEST_COMMANDS\"\nif [ \"$1\" = '-f' ]; then /bin/cat > \"$QUIC_TEST_NFT\"; fi\n"
		script += "if [ -n \"$QUIC_TEST_FAIL_COMMAND\" ] && [ \"" + name + " $*\" = \"$QUIC_TEST_FAIL_COMMAND\" ]; then printf 'injected route failure\\n' >&2; exit 42; fi\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	t.Setenv("QUIC_TEST_COMMANDS", output)
	t.Setenv("QUIC_TEST_NFT", filepath.Join(dir, "rules.nft"))
	return output
}

func TestQUICIptablesRefusesMissingIPv6Protection(t *testing.T) {
	for _, ipv6 := range []bool{false, true} {
		t.Run(fmt.Sprint(ipv6), func(t *testing.T) {
			path := recordFirewallCommands(t)
			if err := os.Remove(filepath.Join(os.Getenv("PATH"), "ip6tables")); err != nil {
				t.Fatal(err)
			}
			m := NewManager(Config{Mode: "tproxy", FirewallBackend: "iptables", EnableIPv6: ipv6})
			if err := m.Apply(); err == nil || !strings.Contains(err.Error(), "ip6tables") {
				t.Fatalf("missing IPv6 protection must be an explicit error, got %v", err)
			}
			if m.IsApplied() {
				t.Fatal("unsafe backend must not report successfully applied")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("prerequisite rejection must leave existing firewall untouched")
			}
		})
	}
}

func readCommands(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestQUICDefaultRemainsConservative(t *testing.T) {
	if !config.Default().Network.DropQUIC {
		t.Fatal("default must keep UDP443 blocked; enabling UDP requires an explicit choice")
	}
	// DROP can encourage client TCP fallback, but neither its timing nor success
	// is guaranteed. This test asserts only the firewall policy.
	rendered := renderNFTTemplateOpts(t, false, false, true, config.Default().Network.DropQUIC)
	drop := strings.Index(rendered, "udp dport 443 drop")
	tcpUDP := strings.Index(rendered, "tproxy ip to")
	if drop < 0 || drop > tcpUDP {
		t.Fatal("default QUIC DROP must precede TCP/UDP interception")
	}
}

func TestQUICNftOutputPolicyRouting(t *testing.T) {
	path := recordFirewallCommands(t)
	b := NftablesBackend{TProxyPort: 7895, EnableIPv6: true, DropQUIC: false}
	if err := b.Apply(); err != nil {
		t.Fatal(err)
	}
	commands := readCommands(t, path)
	for _, want := range []string{
		"ip rule add fwmark " + fwMark + " table " + routeTable,
		"ip -6 rule add fwmark " + fwMark + " table " + routeTable,
		"ip -6 route add local default dev lo table " + routeTable,
		"ip rule add fwmark " + fwMarkOutput + " table " + routeTableOutput,
		"ip route add local default dev lo table " + routeTableOutput,
	} {
		if !strings.Contains(commands, want+"\n") {
			t.Errorf("missing policy route: %s", want)
		}
	}
	rendered := readCommands(t, os.Getenv("QUIC_TEST_NFT"))
	for _, want := range []string{
		"type route hook output priority mangle",
		"ip daddr 198.18.0.0/15 meta l4proto { tcp, udp } meta mark set " + fwMarkOutput,
		"fib saddr type local meta mark != " + fwMarkOutput + " return",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("missing OUTPUT -> PREROUTING path: %s", want)
		}
	}
	if strings.Contains(rendered, "udp dport 443 return") {
		t.Error("rerouted local QUIC must not escape PREROUTING via return")
	}
}

func TestQUICIptablesCustomFakeIPRange(t *testing.T) {
	for _, bypass := range []bool{false, true} {
		t.Run(fmt.Sprintf("bypass=%v", bypass), func(t *testing.T) {
			path := recordFirewallCommands(t)
			m := NewManager(Config{Mode: "tproxy", FirewallBackend: "iptables", TProxyPort: 7895,
				FakeIPRange: "28.18.0.1/16", BypassFakeIP: bypass, BypassCIDR: []string{"1.1.1.0/24"}})
			if err := m.Apply(); err != nil {
				t.Fatal(err)
			}
			commands := readCommands(t, path)
			for _, proto := range []string{"tcp", "udp"} {
				want := "iptables -t mangle -A METACLASH_OUTPUT -d 28.18.0.0/16 -p " + proto + " -j MARK --set-mark " + fwMarkOutput + "\n"
				if !strings.Contains(commands, want) {
					t.Errorf("missing configured fake-IP OUTPUT: %s", want)
				}
			}
			if strings.Contains(commands, "198.18.0.0/15") || strings.Contains(commands, "28.18.0.1/16") {
				t.Error("must normalize the configured range, not use the default or host address")
			}
			want := "iptables -t mangle -A METACLASH -d 28.18.0.0/16 -j RETURN\n"
			if strings.Contains(commands, want) != bypass {
				t.Error("custom fake-IP bypass must follow BypassFakeIP")
			}
			for _, want := range []string{
				"iptables -t mangle -A METACLASH -d 1.1.1.0/24 -j RETURN\n",
				"iptables -t mangle -A METACLASH -m mark --mark " + fwMark + " -j RETURN\n",
				"iptables -t mangle -A METACLASH -m addrtype --src-type LOCAL -m mark ! --mark " + fwMarkOutput + " -j RETURN\n",
				"iptables -t mangle -A METACLASH_OUTPUT -m mark --mark " + fwMarkOutput + " -j RETURN\n",
			} {
				if !strings.Contains(commands, want) {
					t.Errorf("missing bypass or mark exception: %s", want)
				}
			}
		})
	}
}

func TestQUICPolicyRoutingFailureIsReported(t *testing.T) {
	commands := []string{
		"ip rule add fwmark " + fwMark + " table " + routeTable,
		"ip route add local default dev lo table " + routeTable,
		"ip -6 rule add fwmark " + fwMark + " table " + routeTable,
		"ip -6 route add local default dev lo table " + routeTable,
		"ip rule add fwmark " + fwMarkOutput + " table " + routeTableOutput,
		"ip route add local default dev lo table " + routeTableOutput,
	}
	for _, backend := range []string{"nftables", "iptables"} {
		for _, command := range commands {
			t.Run(backend+"/"+command, func(t *testing.T) {
				recordFirewallCommands(t)
				m := NewManager(Config{Mode: "tproxy", FirewallBackend: backend, TProxyPort: 7895, EnableIPv6: true})
				// Check both an initial failure and a failed re-apply after success.
				for _, reapply := range []bool{false, true} {
					t.Setenv("QUIC_TEST_FAIL_COMMAND", "")
					if reapply {
						if err := m.Apply(); err != nil {
							t.Fatal(err)
						}
					}
					t.Setenv("QUIC_TEST_FAIL_COMMAND", command)
					err := m.Apply()
					if err == nil || !strings.Contains(err.Error(), "policy routing") || !strings.Contains(err.Error(), strings.TrimPrefix(command, "ip ")) || !strings.Contains(err.Error(), "injected route failure") {
						t.Errorf("reapply=%v: must report critical route command and stderr, got %v", reapply, err)
					}
					if m.IsApplied() {
						t.Errorf("reapply=%v: failed setup must not report applied", reapply)
					}
				}
			})
		}
	}
}

func TestQUICIptablesBackendParity(t *testing.T) {
	for _, ipv6 := range []bool{false, true} {
		for _, drop := range []bool{false, true} {
			t.Run(fmt.Sprintf("ipv6=%v/drop=%v", ipv6, drop), func(t *testing.T) {
				path := recordFirewallCommands(t)
				m := NewManager(Config{Mode: "tproxy", FirewallBackend: "iptables", TProxyPort: 7895, EnableIPv6: ipv6, DropQUIC: drop})
				if err := m.Apply(); err != nil {
					t.Fatal(err)
				}
				commands := readCommands(t, path)
				families := []string{"iptables"}
				if ipv6 {
					families = append(families, "ip6tables")
				}
				for _, family := range families {
					prefix := family + " -t mangle -A METACLASH "
					udp := prefix + "-p udp -j TPROXY --tproxy-mark " + fwMark + " --on-port 7895"
					tcp := prefix + "-p tcp -j TPROXY --tproxy-mark " + fwMark + " --on-port 7895"
					for _, want := range []string{udp, tcp, family + " -t mangle -A PREROUTING -j METACLASH"} {
						if !strings.Contains(commands, want+"\n") {
							t.Errorf("missing normal interception: %s", want)
						}
					}
					dropRule := prefix + "-p udp --dport 443 -j DROP"
					if strings.Contains(commands, dropRule+"\n") != drop {
						t.Errorf("%s: QUIC DROP does not follow configuration", family)
					}
					if drop && strings.Index(commands, dropRule) > strings.Index(commands, udp) {
						t.Error("DROP must precede UDP interception")
					}
					if strings.Contains(commands, "--dport 443 -j RETURN") {
						t.Error("QUIC allow must not bypass TProxy")
					}
				}
				for _, want := range []string{
					"iptables -t mangle -A OUTPUT -j METACLASH_OUTPUT",
					"iptables -t mangle -A METACLASH_OUTPUT -d 198.18.0.0/15 -p udp -j MARK --set-mark " + fwMarkOutput,
					"iptables -t mangle -A METACLASH_OUTPUT -d 198.18.0.0/15 -p tcp -j MARK --set-mark " + fwMarkOutput,
					"ip rule add fwmark " + fwMarkOutput + " table " + routeTableOutput,
				} {
					if !strings.Contains(commands, want+"\n") {
						t.Errorf("missing router-local fake-IP routing: %s", want)
					}
				}
				for _, want := range []string{
					"ip6tables -t filter -A METACLASH_IPV6 -j DROP\n",
					"ip6tables -t filter -I FORWARD 1 -j METACLASH_IPV6\n",
				} {
					if !ipv6 && !strings.Contains(commands, want) {
						t.Errorf("disabled IPv6 must explicitly block global forwarding: %s", want)
					}
				}
				if ipv6 && !strings.Contains(commands, "ip -6 rule add fwmark "+fwMark+" table "+routeTable+"\n") {
					t.Error("IPv6 interception needs IPv6 policy routing")
				}
				if err := m.Cleanup(); err != nil {
					t.Fatal(err)
				}
				commands = readCommands(t, path)
				if !strings.Contains(commands, "iptables -t mangle -D OUTPUT -j METACLASH_OUTPUT\n") {
					t.Error("cleanup must remove OUTPUT hook")
				}
			})
		}
	}
}
