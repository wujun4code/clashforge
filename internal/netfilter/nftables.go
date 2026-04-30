package netfilter

import (
	"bytes"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"text/template"

	"github.com/rs/zerolog/log"
)

const fwMark = "0x1a3"
const routeTable = "100"

// fwMarkOutput marks packets from router-originated processes so they can be
// re-routed through loopback and intercepted by tproxy_prerouting.
const fwMarkOutput = "0x1a4"
const routeTableOutput = "101"

// Exported for use in API stop-flow SSE messages.
const FWMark = fwMark
const RouteTable = routeTable
const FWMarkOutput = fwMarkOutput
const RouteTableOutput = routeTableOutput

// NftablesBackend manages nftables TProxy rules.
type NftablesBackend struct {
	TProxyPort        int
	DNSPort           int
	EnableDNSRedirect bool
	BypassFakeIP      bool
	BypassCIDR        []string
	EnableIPv6        bool
}

var nftTableTemplate = template.Must(template.New("nft").Parse(`
table inet metaclash {
    set bypass_ipv4 {
        type ipv4_addr
        flags interval
        elements = {
{{ .BypassIPv4Elements }}
        }
    }
{{ if .EnableIPv6 }}
    set bypass_ipv6 {
        type ipv6_addr
        flags interval
        elements = {
            ::1/128,
            fc00::/7,
            fe80::/10,
            ff00::/8,
            100::/64
        }
    }
{{ end }}
{{ if .EnableDNSRedirect }}
    chain dns_redirect {
        type nat hook prerouting priority dstnat; policy accept;
        meta mark {{ .FWMark }} return
        fib saddr type local return
        udp dport 53 redirect to :{{ .DNSPort }}
        tcp dport 53 redirect to :{{ .DNSPort }}
    }

    # Redirect DNS originating from the router itself (resolv.conf → 127.0.0.1:53).
    # The prerouting chain only handles forwarded traffic; locally-generated packets
    # go through the output hook and bypass prerouting entirely.
    # Only loopback destinations are matched so mihomo's own upstream DNS queries
    # (to real IPs like 119.29.29.29:53) are left untouched and do not loop back.
    chain dns_output_redirect {
        type nat hook output priority -100; policy accept;
        ip daddr 127.0.0.0/8 udp dport 53 redirect to :{{ .DNSPort }}
        ip daddr 127.0.0.0/8 tcp dport 53 redirect to :{{ .DNSPort }}
{{ if .EnableIPv6 }}
        ip6 daddr ::1/128 udp dport 53 redirect to :{{ .DNSPort }}
        ip6 daddr ::1/128 tcp dport 53 redirect to :{{ .DNSPort }}
{{ end }}
    }
{{ end }}

    chain tproxy_prerouting {
        type filter hook prerouting priority mangle; policy accept;
        meta mark {{ .FWMark }} return
        # Skip router-originated traffic UNLESS it was re-routed by tproxy_output
        # (those packets carry {{ .FWMarkOutput }} and must be tproxied, not skipped).
        fib saddr type local meta mark != {{ .FWMarkOutput }} return
        ip daddr @bypass_ipv4 return
        # Block QUIC (HTTP/3 over UDP 443) — mihomo cannot SNI-sniff QUIC packets,
        # dropping forces the client to retry over TCP where SNI matching works correctly.
        udp dport 443 drop
        meta l4proto { tcp, udp } tproxy ip to 127.0.0.1:{{ .TProxyPort }} meta mark set {{ .FWMark }}
{{ if .EnableIPv6 }}
        ip6 daddr @bypass_ipv6 return
        meta l4proto { tcp, udp } tproxy ip6 to [::1]:{{ .TProxyPort }} meta mark set {{ .FWMark }}
{{ end }}
    }

    # Intercept traffic originating from the router itself (curl, wget, opkg, etc.).
    # Router-originated packets skip PREROUTING entirely, so without this chain they
    # would attempt direct connections to mihomo fake-IPs (198.18.0.x) which do not
    # exist as real destinations.
    # Only fake-IP destinations are matched — mihomo's own upstream connections always
    # target real IPs, so there is no routing loop.
    chain tproxy_output {
        type route hook output priority mangle; policy accept;
        meta mark {{ .FWMark }} return
        meta mark {{ .FWMarkOutput }} return
        ip daddr 198.18.0.0/15 meta l4proto { tcp, udp } meta mark set {{ .FWMarkOutput }}
    }
}
`))

// Apply writes and applies the nftables ruleset.
func (n *NftablesBackend) Apply() error {
	vars := struct {
		FWMark             string
		FWMarkOutput       string
		TProxyPort         int
		DNSPort            int
		EnableDNSRedirect  bool
		EnableIPv6         bool
		BypassIPv4Elements string
	}{
		FWMark:             fwMark,
		FWMarkOutput:       fwMarkOutput,
		TProxyPort:         n.TProxyPort,
		DNSPort:            n.DNSPort,
		EnableDNSRedirect:  n.EnableDNSRedirect,
		EnableIPv6:         n.EnableIPv6,
		BypassIPv4Elements: buildBypassIPv4Elements(n.BypassFakeIP, n.BypassCIDR),
	}

	log.Info().
		Int("tproxy_port", n.TProxyPort).
		Int("dns_port", n.DNSPort).
		Bool("dns_redirect", n.EnableDNSRedirect).
		Bool("bypass_fakeip", n.BypassFakeIP).
		Bool("enable_ipv6", n.EnableIPv6).
		Strs("bypass_cidr", n.BypassCIDR).
		Msg("netfilter: 开始应用 nftables 规则")
	var buf bytes.Buffer
	if err := nftTableTemplate.Execute(&buf, vars); err != nil {
		return fmt.Errorf("render nft template: %w", err)
	}

	// DNS redirect chains use nat table hooks which require nf_nat. Probe-load
	// the module so the nft apply below doesn't fail on minimal OpenWrt images
	// that haven't loaded it yet (e.g. no firewall3/firewall4 running).
	// nft_redir provides the "redirect to :port" expression evaluator; it is a
	// separate module from nf_nat on kernels that compile CONFIG_NFT_REDIR=m.
	if n.EnableDNSRedirect {
		for _, mod := range []string{"nf_nat", "nft_redir"} {
			out, err := exec.Command("modprobe", mod).CombinedOutput()
			if err != nil {
				log.Debug().Err(err).Str("module", mod).Str("output", string(out)).
					Msg("netfilter: modprobe (may be built-in or unavailable)")
			}
		}
	}

	// First cleanup any existing rules
	_ = n.Cleanup()

	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = &buf
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Error().Err(err).Str("output", string(out)).Msg("netfilter: ⚠️ nft apply 失败！透明代理规则未生效，所有流量将绕过代理")
		return fmt.Errorf("nft apply: %w: %s", err, string(out))
	}
	log.Info().Msg("netfilter: nftables 规则已成功应用 ✓")

	// Setup policy routing (IPv4)
	_ = exec.Command("ip", "rule", "add", "fwmark", fwMark, "table", routeTable).Run()
	_ = exec.Command("ip", "route", "add", "local", "default", "dev", "lo", "table", routeTable).Run()

	// Setup policy routing (IPv6) — only when IPv6 tproxy is enabled
	if n.EnableIPv6 {
		_ = exec.Command("ip", "-6", "rule", "add", "fwmark", fwMark, "table", routeTable).Run()
		_ = exec.Command("ip", "-6", "route", "add", "local", "default", "dev", "lo", "table", routeTable).Run()
	}

	// Setup policy routing for tproxy_output (router-originated traffic).
	// tproxy_output only marks IPv4 fake-IP destinations, so only IPv4 rules needed.
	_ = exec.Command("ip", "rule", "add", "fwmark", fwMarkOutput, "table", routeTableOutput).Run()
	_ = exec.Command("ip", "route", "add", "local", "default", "dev", "lo", "table", routeTableOutput).Run()

	log.Info().
		Str("fwmark", fwMark).
		Str("route_table", routeTable).
		Str("fwmark_output", fwMarkOutput).
		Str("route_table_output", routeTableOutput).
		Msg("netfilter: 策略路由已设置 ✓ — fwmark 匹配的流量将通过 TProxy 端口转发")

	return nil
}

// Cleanup removes all metaclash nftables rules.
func (n *NftablesBackend) Cleanup() error {
	out, err := exec.Command("nft", "delete", "table", "inet", "metaclash").CombinedOutput()
	if err != nil {
		s := string(out)
		if strings.Contains(s, "No such file") || strings.Contains(s, "table not found") || strings.Contains(s, "does not exist") {
			return nil
		}
		return fmt.Errorf("nft cleanup: %w: %s", err, s)
	}
	_ = exec.Command("ip", "rule", "del", "fwmark", fwMark, "table", routeTable).Run()
	_ = exec.Command("ip", "route", "flush", "table", routeTable).Run()
	_ = exec.Command("ip", "-6", "rule", "del", "fwmark", fwMark, "table", routeTable).Run()
	_ = exec.Command("ip", "-6", "route", "flush", "table", routeTable).Run()
	_ = exec.Command("ip", "rule", "del", "fwmark", fwMarkOutput, "table", routeTableOutput).Run()
	_ = exec.Command("ip", "route", "flush", "table", routeTableOutput).Run()
	return nil
}

func buildBypassIPv4Elements(bypassFakeIP bool, bypassCIDR []string) string {
	elements := []string{
		"0.0.0.0/8",
		"10.0.0.0/8",
		"100.64.0.0/10",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"172.16.0.0/12",
		"192.0.0.0/24",
		"192.168.0.0/16",
	}

	if bypassFakeIP {
		elements = append(elements, "198.18.0.0/15")
	}

	elements = append(elements,
		"198.51.100.0/24",
		"203.0.113.0/24",
		"224.0.0.0/4",
		"240.0.0.0/4",
	)

	for _, raw := range bypassCIDR {
		cidr := strings.TrimSpace(raw)
		if cidr == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			log.Warn().Str("cidr", cidr).Err(err).Msg("netfilter: skip invalid bypass_cidr entry")
			continue
		}
		elements = append(elements, cidr)
	}

	var b strings.Builder
	for i, cidr := range elements {
		b.WriteString("            ")
		b.WriteString(cidr)
		if i < len(elements)-1 {
			b.WriteString(",")
			b.WriteString("\n")
		}
	}

	return b.String()
}
