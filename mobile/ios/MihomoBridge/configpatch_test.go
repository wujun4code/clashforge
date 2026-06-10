package mihomobridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleConfig = `port: 7890
mode: rule
external-controller: 127.0.0.1:9090

dns:
  enable: true
  listen: 0.0.0.0:1053
  enhanced-mode: redir-host
  nameserver:
    - 223.5.5.5
    - 119.29.29.29
  proxy-server-nameserver:
    - 223.5.5.5
    - "https://doh.pub/dns-query"

proxies:
  - name: node-a
    type: vless
    server: example-node.example.com
    port: 443
  - name: node-b
    type: ss
    server: 1.2.3.4
    port: 8388

tun:
  enable: true
  stack: gvisor
  file-descriptor: 0

sniffer:
  enable: false

rules:
  - MATCH,DIRECT
`

func patchSample(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(sampleConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := PatchConfigWithTun(path, 7); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestPatchConfigWithTun(t *testing.T) {
	got := patchSample(t)

	for _, want := range []string{
		"file-descriptor: 7",
		"stack: system",
		"auto-detect-interface: true",
		"  enhanced-mode: fake-ip",
		"  fake-ip-range: 198.18.0.0/15",
		"  respect-rules: false",
		"geodata-loader: memconservative",
		`- "geosite:cn"`,
		"override-destination: true",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("patched config missing %q\n---\n%s", want, got)
		}
	}

	if strings.Contains(got, "stack: gvisor") {
		t.Error("old tun block should have been removed")
	}
	if strings.Contains(got, "enhanced-mode: redir-host") {
		t.Error("redir-host should have been migrated to fake-ip")
	}
	if strings.Count(got, "\ntun:") != 1 {
		t.Errorf("expected exactly one tun block:\n%s", got)
	}
	if strings.Count(got, "\nsniffer:") != 1 {
		t.Errorf("expected exactly one sniffer block:\n%s", got)
	}
	// rules block must survive section surgery
	if !strings.Contains(got, "MATCH,DIRECT") {
		t.Error("rules section was lost")
	}
}

func TestPatchIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(sampleConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := PatchConfigWithTun(path, 7); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(path)
	if err := PatchConfigWithTun(path, 7); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if string(first) != string(second) {
		t.Errorf("patch is not idempotent:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

func TestExtractProxyHostnames(t *testing.T) {
	hosts := extractProxyHostnames(sampleConfig)
	if len(hosts) != 1 || hosts[0] != "example-node.example.com" {
		t.Errorf("want [example-node.example.com], got %v", hosts)
	}
}

func TestExtractDNSList(t *testing.T) {
	ns := extractDNSList(sampleConfig, "nameserver")
	if len(ns) != 2 || ns[0] != "223.5.5.5" || ns[1] != "119.29.29.29" {
		t.Errorf("unexpected nameservers: %v", ns)
	}
	psn := extractDNSList(sampleConfig, "proxy-server-nameserver")
	if len(psn) != 2 || psn[1] != "https://doh.pub/dns-query" {
		t.Errorf("unexpected proxy-server-nameserver: %v", psn)
	}
}

func TestUpsertDNSListReplacesExisting(t *testing.T) {
	out := upsertDNSList(sampleConfig, "nameserver", []string{"https://1.1.1.1/dns-query"})
	if strings.Contains(out, "223.5.5.5\n") && strings.Contains(strings.SplitN(out, "proxies:", 2)[0], "- 223.5.5.5\n    - 119.29.29.29") {
		t.Error("old nameserver entries should be replaced")
	}
	got := extractDNSList(out, "nameserver")
	if len(got) != 1 || got[0] != "https://1.1.1.1/dns-query" {
		t.Errorf("unexpected nameservers after upsert: %v", got)
	}
}

func TestFakeIPDetection(t *testing.T) {
	if !isKnownFakeIP("198.18.0.5") || !isKnownFakeIP("198.19.255.1") || !isKnownFakeIP("28.1.2.3") {
		t.Error("known fake ranges not detected")
	}
	if isKnownFakeIP("104.16.1.1") || isKnownFakeIP("198.20.0.1") {
		t.Error("real IPs misclassified as fake")
	}
	if !allInKnownFakeRanges([]string{"198.18.0.5", "28.0.0.1"}) {
		t.Error("all-fake list not detected")
	}
	if allInKnownFakeRanges([]string{"198.18.0.5", "104.16.1.1"}) {
		t.Error("mixed list must not count as hijack")
	}
	if allInKnownFakeRanges(nil) {
		t.Error("empty list must not count as hijack")
	}
}

func TestNameserverClassification(t *testing.T) {
	if !isUDPNameserver("223.5.5.5") || !isUDPNameserver("udp://1.1.1.1:53") {
		t.Error("udp nameservers misclassified")
	}
	for _, ns := range []string{"https://doh.pub/dns-query", "tls://1.1.1.1", "tcp://1.1.1.1", "dhcp://en0"} {
		if isUDPNameserver(ns) {
			t.Errorf("%s should not be udp", ns)
		}
	}
	if !isIPLiteralDoH("https://1.1.1.1/dns-query#skip-cert-verify=true") {
		t.Error("ip-literal DoH not detected")
	}
	if isIPLiteralDoH("https://doh.pub/dns-query") {
		t.Error("domain DoH misclassified as ip-literal")
	}
	if got := withSkipCertVerifyParam("https://1.1.1.1/dns-query"); got != "https://1.1.1.1/dns-query#skip-cert-verify=true" {
		t.Errorf("unexpected skip-cert-verify rendering: %s", got)
	}
	if got := withSkipCertVerifyParam("https://1.1.1.1/dns-query#foo"); got != "https://1.1.1.1/dns-query#foo&skip-cert-verify=true" {
		t.Errorf("unexpected skip-cert-verify append: %s", got)
	}
	if h := extractIPLiteralFromPlainNameserver("udp://223.5.5.5:53"); h != "223.5.5.5" {
		t.Errorf("plain nameserver host extraction failed: %s", h)
	}
}

func TestDNSQueryRoundTrip(t *testing.T) {
	q := buildDNSQuery("example.com", 0xBEEF)
	if len(q) != 12+13+4 {
		t.Errorf("unexpected query length %d", len(q))
	}
	// craft a minimal response: header + question + one A answer (1.2.3.4)
	resp := append([]byte{}, q...)
	resp[2] = 0x81 // QR=1
	resp[7] = 0x01 // ANCOUNT=1
	resp = append(resp,
		0xC0, 0x0C, // name pointer to offset 12
		0x00, 0x01, 0x00, 0x01, // TYPE A, CLASS IN
		0x00, 0x00, 0x00, 0x3C, // TTL
		0x00, 0x04, // RDLENGTH
		1, 2, 3, 4,
	)
	ips := parseDNSAAnswers(resp, 0xBEEF)
	if len(ips) != 1 || ips[0] != "1.2.3.4" {
		t.Errorf("unexpected answers: %v", ips)
	}
	if got := parseDNSAAnswers(resp, 0x1234); got != nil {
		t.Error("mismatched query id must be rejected")
	}
}
