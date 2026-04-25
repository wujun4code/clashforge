package netfilter

import (
	"bytes"
	"strings"
	"testing"
)

func renderNFTTemplate(t *testing.T, enableDNSRedirect, bypassFakeIP bool) string {
	t.Helper()

	bypassCIDR := []string{"1.1.1.0/24"}
	vars := struct {
		FWMark             string
		TProxyPort         int
		DNSPort            int
		EnableDNSRedirect  bool
		EnableIPv6         bool
		BypassIPv4Elements string
	}{
		FWMark:             fwMark,
		TProxyPort:         7895,
		DNSPort:            7874,
		EnableDNSRedirect:  enableDNSRedirect,
		EnableIPv6:         false,
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
	if strings.Contains(rendered, "198.18.0.0/15") {
		t.Fatalf("expected fake-ip range to be omitted when bypass is disabled")
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

func TestNFTTemplate_NoBlankLineBeforeBypassSetClose(t *testing.T) {
	rendered := renderNFTTemplate(t, false, false)
	if strings.Contains(rendered, "240.0.0.0/4\n\n        }") {
		t.Fatalf("expected no blank line before bypass set closing brace")
	}
}
