package netfilter

import (
	"bytes"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strings"
	"text/template"

	"github.com/rs/zerolog/log"
)

// tunForwardComment marks the forward-accept rule ClashForge inserts into the
// live fw4 ruleset for TUN mode, so it can find and remove its own rule later
// without touching anything fw4 itself manages.
const tunForwardComment = "clashforge: allow forwarding into TUN device"

var nftHandleRe = regexp.MustCompile(`# handle (\d+)`)

// EnsureTunForwardAccept inserts a forward-accept rule for the TUN device into
// the live "inet fw4" ruleset. mihomo's TUN auto-route correctly diverts
// LAN-client traffic into the TUN device at the IP routing layer (ip rule/ip
// route), but stock OpenWrt fw4 only allowlists known zone devices (lan/wan)
// in its forward chain — without this rule, that traffic is silently
// dropped/rejected by fw4's handle_reject fallback before mihomo's TUN reader
// ever sees it. This is a live edit to fw4's own table (not a separate
// ClashForge-owned table) because nftables verdicts from a different table at
// the same hook do not override fw4's own drop policy; it does not persist
// across an OpenWrt firewall reload (`fw4 reload`/`service firewall restart`),
// since fw4 regenerates from /etc/config/firewall — Apply() re-adds it on
// every (re)start so this is re-asserted whenever ClashForge re-applies rules.
func EnsureTunForwardAccept(device string) error {
	if device == "" {
		device = "Meta"
	}
	out, _ := exec.Command("nft", "list", "chain", "inet", "fw4", "forward_lan").CombinedOutput()
	if strings.Contains(string(out), `oifname "`+device+`"`) {
		return nil
	}
	cmd := exec.Command("nft", "insert", "rule", "inet", "fw4", "forward_lan",
		"oifname", device, "counter", "accept", "comment", `"`+tunForwardComment+`"`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nft insert tun forward rule: %w: %s", err, string(out))
	}
	log.Info().Str("device", device).Msg("netfilter: 已在 fw4 forward_lan 中放行 TUN 设备转发流量 ✓")
	return nil
}

// RemoveTunForwardAccept deletes the rule added by EnsureTunForwardAccept.
// Best-effort: if fw4 already reloaded and dropped it, this is a no-op.
func RemoveTunForwardAccept(device string) error {
	out, err := exec.Command("nft", "-a", "list", "chain", "inet", "fw4", "forward_lan").CombinedOutput()
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, tunForwardComment) {
			continue
		}
		m := nftHandleRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		_ = exec.Command("nft", "delete", "rule", "inet", "fw4", "forward_lan", "handle", m[1]).Run()
	}
	return nil
}

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
	DropQUIC          bool
	WANInterface      string
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
        # QUIC (HTTP/3, UDP 443) handling.
        # drop:   forces browsers to immediately fall back to TCP, which HTTP proxy
        #         nodes can tunnel. Correct for all HTTP-proxy setups. Domestic apps
        #         lose QUIC speed but still work via TCP.
        # return: bypasses tproxy entirely (legacy). Only safe if proxy nodes support
        #         UDP. With HTTP proxies, GFW-blocked domains (e.g. OpenAI) may not
        #         be rejected fast enough, so the browser sends the real Chinese IP
        #         before falling back to TCP — causing geo-detection failures.
{{ if .DropQUIC }}        udp dport 443 drop
{{ else }}        udp dport 443 return
{{ end -}}
        meta l4proto { tcp, udp } tproxy ip to 127.0.0.1:{{ .TProxyPort }} meta mark set {{ .FWMark }}
{{ if .EnableIPv6 }}
        ip6 daddr @bypass_ipv6 return
        meta l4proto { tcp, udp } tproxy ip6 to [::1]:{{ .TProxyPort }} meta mark set {{ .FWMark }}
{{ end }}
    }

{{ if not .EnableIPv6 }}
    # IPv6 proxy is disabled. Drop forwarded IPv6 to global addresses so LAN
    # clients cannot reach IPv6-capable external sites with their real (Chinese)
    # IPv6 address, bypassing the proxy entirely.
    # ULA (fc00::/7) and link-local (fe80::/10) are preserved for LAN services.
    chain ipv6_forward_block {
        type filter hook forward priority filter; policy accept;
        meta nfproto ipv6 ip6 daddr != { ::1/128, fc00::/7, fe80::/10, ff00::/8 } drop
    }
{{ end }}
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
		DropQUIC           bool
		BypassIPv4Elements string
	}{
		FWMark:             fwMark,
		FWMarkOutput:       fwMarkOutput,
		TProxyPort:         n.TProxyPort,
		DNSPort:            n.DNSPort,
		EnableDNSRedirect:  n.EnableDNSRedirect,
		EnableIPv6:         n.EnableIPv6,
		DropQUIC:           n.DropQUIC,
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
