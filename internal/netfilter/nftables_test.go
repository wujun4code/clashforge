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
	}{
		FWMark:             fwMark,
		FWMarkOutput:       fwMarkOutput,
		TProxyPort:         7895,
		DNSPort:            7874,
		EnableDNSRedirect:  enableDNSRedirect,
		EnableIPv6:         enableIPv6,
		DropQUIC:           dropQUIC,
		BypassIPv4Elements: buildBypassIPv4Elements(bypassFakeIP, bypassCIDR),
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
	rendered := buildBypassIPv4Elements(true, []string{"", "  ", "invalid", "1.2.3.0/24"})

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

func TestNFTTemplate_QUICReturnWhenDropQUICDisabled(t *testing.T) {
	rendered := renderNFTTemplateOpts(t, false, false, false, false)
	if strings.Contains(rendered, "udp dport 443 drop") {
		t.Fatalf("did not expect 'udp dport 443 drop' when DropQUIC=false")
	}
	if !strings.Contains(rendered, "udp dport 443 return") {
		t.Fatalf("expected 'udp dport 443 return' when DropQUIC=false")
	}
}

func TestNFTTemplate_IPv6ForwardBlockWhenIPv6Disabled(t *testing.T) {
	rendered := renderNFTTemplateOpts(t, false, false, false, false)
	if !strings.Contains(rendered, "chain ipv6_forward_block") {
		t.Fatalf("expected ipv6_forward_block chain when EnableIPv6=false")
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
