package netfilter

import (
	"bytes"
	"strings"
	"testing"
)

func renderNFTTemplate(t *testing.T, enableDNSRedirect, bypassFakeIP bool) string {
	t.Helper()
	return renderNFTTemplateOpts(t, enableDNSRedirect, bypassFakeIP, false, false)
}

func renderNFTTemplateOpts(t *testing.T, enableDNSRedirect, bypassFakeIP, enableIPv6, dropQUIC bool) string {
	t.Helper()

	bypassCIDR := []string{"1.1.1.0/24"}
	vars := struct {
		FWMark             string
		FWMarkOutput       string
		TProxyPort         int
		DNSPort            int
		EnableDNSRedirect  bool
		EnableIPv6         bool
		DropQUIC           bool
		BypassIPv4Elements string
		FakeIPRange        string
	}{
		FWMark:             fwMark,
		FWMarkOutput:       fwMarkOutput,
		TProxyPort:         7895,
		DNSPort:            7874,
		EnableDNSRedirect:  enableDNSRedirect,
		EnableIPv6:         enableIPv6,
		DropQUIC:           dropQUIC,
		BypassIPv4Elements: buildBypassIPv4Elements(bypassFakeIP, bypassCIDR, defaultFakeIPRange),
		FakeIPRange:        defaultFakeIPRange,
	}

	var buf bytes.Buffer
	if err := nftTableTemplate.Execute(&buf, vars); err != nil {
		t.Fatalf("render nft template: %v", err)
	}
	return buf.String()
}

func TestNFTTemplate_OmitsFakeIPBypassWhenDisabled(t *testing.T) {
	rendered := renderNFTTemplate(t, true, false)
	// 198.18.0.0/15 must NOT appear inside the bypass_ipv4 set when BypassFakeIP=false.
	// It will always appear in tproxy_output (hardcoded fake-IP range), so we check the
	// set block specifically.
	bypassSetEnd := strings.Index(rendered, "set bypass_ipv4")
	if bypassSetEnd == -1 {
		t.Fatalf("bypass_ipv4 set not found in rendered template")
	}
	setClose := strings.Index(rendered[bypassSetEnd:], "}")
	bypassBlock := rendered[bypassSetEnd : bypassSetEnd+setClose+1]
	if strings.Contains(bypassBlock, "198.18.0.0/15") {
		t.Fatalf("expected fake-ip range to be omitted from bypass_ipv4 set when bypass is disabled")
	}
}

func TestNFTTemplate_IncludesFakeIPBypassWhenEnabled(t *testing.T) {
	rendered := renderNFTTemplate(t, true, true)
	if !strings.Contains(rendered, "198.18.0.0/15") {
		t.Fatalf("expected fake-ip range to be present when bypass is enabled")
	}
}

func TestNFTTemplate_DNSRedirectChainToggle(t *testing.T) {
	withDNSRedirect := renderNFTTemplate(t, true, true)
	withoutDNSRedirect := renderNFTTemplate(t, false, true)

	if !strings.Contains(withDNSRedirect, "chain dns_redirect") {
		t.Fatalf("expected dns_redirect chain when dns redirect is enabled")
	}
	if strings.Contains(withoutDNSRedirect, "chain dns_redirect") {
		t.Fatalf("did not expect dns_redirect chain when dns redirect is disabled")
	}
	if !strings.Contains(withDNSRedirect, "chain dns_output_redirect") {
		t.Fatalf("expected dns_output_redirect chain when dns redirect is enabled")
	}
	if strings.Contains(withoutDNSRedirect, "chain dns_output_redirect") {
		t.Fatalf("did not expect dns_output_redirect chain when dns redirect is disabled")
	}
}

func TestBuildBypassIPv4Elements_SkipsInvalidAndEmptyCIDR(t *testing.T) {
	rendered := buildBypassIPv4Elements(true, []string{"", "  ", "invalid", "1.2.3.0/24"}, defaultFakeIPRange)

	if strings.Contains(rendered, "invalid") {
		t.Fatalf("expected invalid cidr to be skipped")
	}
	if !strings.Contains(rendered, "1.2.3.0/24") {
		t.Fatalf("expected valid custom cidr to be included")
	}
}

func TestNFTTemplate_TproxyOutputChainPresent(t *testing.T) {
	rendered := renderNFTTemplate(t, false, false)
	if !strings.Contains(rendered, "chain tproxy_output") {
		t.Fatalf("expected tproxy_output chain to be present")
	}
	if !strings.Contains(rendered, "198.18.0.0/15") {
		t.Fatalf("expected fake-IP range 198.18.0.0/15 in tproxy_output")
	}
	if !strings.Contains(rendered, fwMarkOutput) {
		t.Fatalf("expected output fwmark %s in tproxy_output", fwMarkOutput)
	}
}

func TestNFTTemplate_TproxyPreRoutingAllowsReRoutedLocalTraffic(t *testing.T) {
	rendered := renderNFTTemplate(t, false, false)
	if !strings.Contains(rendered, "fib saddr type local meta mark !=") {
		t.Fatalf("expected tproxy_prerouting to conditionally skip local traffic based on output mark")
	}
}

func TestNFTTemplate_QUICDropWhenDropQUICEnabled(t *testing.T) {
	rendered := renderNFTTemplateOpts(t, false, false, false, true)
	if !strings.Contains(rendered, "udp dport 443 drop") {
		t.Fatalf("expected 'udp dport 443 drop' when DropQUIC=true")
	}
	if strings.Contains(rendered, "udp dport 443 return") {
		t.Fatalf("did not expect 'udp dport 443 return' when DropQUIC=true")
	}
}

func TestNFTTemplate_QUICTProxyWhenDropQUICDisabled(t *testing.T) {
	for _, ipv6 := range []bool{false, true} {
		rendered := renderNFTTemplateOpts(t, false, false, ipv6, false)
		if strings.Contains(rendered, "udp dport 443 drop") || strings.Contains(rendered, "udp dport 443 return") {
			t.Errorf("EnableIPv6=%v: allowed QUIC must use ordinary UDP TProxy, not drop or return", ipv6)
		}
		if !strings.Contains(rendered, "meta nfproto ipv4 meta l4proto { tcp, udp } tproxy ip to 127.0.0.1:7895 meta mark set "+fwMark) {
			t.Errorf("EnableIPv6=%v: missing family-scoped IPv4 TCP/UDP interception", ipv6)
		}
		if ipv6 && !strings.Contains(rendered, "meta nfproto ipv6 meta l4proto { tcp, udp } tproxy ip6 to [::1]:7895 meta mark set "+fwMark) {
			t.Error("missing family-scoped IPv6 TCP/UDP interception")
		}
	}
}

func TestNFTTemplate_IPv6ForwardBlockWhenIPv6Disabled(t *testing.T) {
	rendered := renderNFTTemplateOpts(t, false, false, false, false)
	if !strings.Contains(rendered, "chain ipv6_forward_block") ||
		!strings.Contains(rendered, "meta nfproto ipv6 ip6 daddr != { ::1/128, fc00::/7, fe80::/10, ff00::/8 } drop") {
		t.Fatalf("expected explicit global IPv6 forward block when EnableIPv6=false")
	}
	if strings.Contains(rendered, "tproxy ip6 to") {
		t.Fatal("disabled IPv6 must be blocked, not intercepted without policy routing")
	}
}

func TestNFTTemplate_NoIPv6ForwardBlockWhenIPv6Enabled(t *testing.T) {
	rendered := renderNFTTemplateOpts(t, false, false, true, false)
	if strings.Contains(rendered, "chain ipv6_forward_block") {
		t.Fatalf("did not expect ipv6_forward_block chain when EnableIPv6=true")
	}
}

func TestNFTTemplate_NoBlankLineBeforeBypassSetClose(t *testing.T) {
	rendered := renderNFTTemplate(t, false, false)
	if strings.Contains(rendered, "240.0.0.0/4\n\n        }") {
		t.Fatalf("expected no blank line before bypass set closing brace")
	}
}

// TestNFTTemplate_CustomFakeIPRangeAppliedToOutputChainAndBypassSet guards
// against regressing back to a hardcoded 198.18.0.0/15 in tproxy_output: a
// chained upstream ClashForge can force this instance onto a different
// fake-ip pool (config.SelectFakeIPRange), and if tproxy_output doesn't
// track that, router-originated traffic (curl/wget/opkg, subscription
// updates, ...) to fake-ip destinations silently stops being redirected into
// the transparent proxy and times out instead.
func TestNFTTemplate_CustomFakeIPRangeAppliedToOutputChainAndBypassSet(t *testing.T) {
	const fakeIPRange = "28.18.0.0/16"
	vars := struct {
		FWMark             string
		FWMarkOutput       string
		TProxyPort         int
		DNSPort            int
		EnableDNSRedirect  bool
		EnableIPv6         bool
		DropQUIC           bool
		BypassIPv4Elements string
		FakeIPRange        string
	}{
		FWMark:             fwMark,
		FWMarkOutput:       fwMarkOutput,
		TProxyPort:         7895,
		DNSPort:            7874,
		BypassIPv4Elements: buildBypassIPv4Elements(true, nil, fakeIPRange),
		FakeIPRange:        fakeIPRange,
	}
	var buf bytes.Buffer
	if err := nftTableTemplate.Execute(&buf, vars); err != nil {
		t.Fatalf("render nft template: %v", err)
	}
	rendered := buf.String()
	if !strings.Contains(rendered, "ip daddr 28.18.0.0/16 meta l4proto { tcp, udp } meta mark set") {
		t.Fatalf("expected tproxy_output to match the configured fake-ip range, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "198.18.0.0/15") {
		t.Fatalf("did not expect the historical default fake-ip range to appear when a custom one is configured")
	}
}

func TestNormalizedFakeIPRange(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty falls back to default", "", defaultFakeIPRange},
		{"invalid falls back to default", "not-a-cidr", defaultFakeIPRange},
		{"host address is masked down to its network", "28.18.0.1/16", "28.18.0.0/16"},
		{"already network-aligned is unchanged", "198.18.0.0/15", "198.18.0.0/15"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := normalizedFakeIPRange(c.in); got != c.want {
				t.Fatalf("normalizedFakeIPRange(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
