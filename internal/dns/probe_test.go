package dns

import (
	"strings"
	"testing"
)

// TestSummarizeFallbackFailures_Empty verifies no summary is produced when
// there's nothing to report (e.g. hijack detection never ran the fallback
// search, or every candidate actually succeeded).
func TestSummarizeFallbackFailures_Empty(t *testing.T) {
	r := NSProbeReport{}
	if got := r.SummarizeFallbackFailures(); got != "" {
		t.Fatalf("expected empty summary, got %q", got)
	}
}

// TestSummarizeFallbackFailures_DistinguishesNetworkFromPoisoning is the case
// from the sg-02 incident: some candidates timed out (network path blocked,
// unrelated to the domain), others resolved but to a fake-ip (this resolver
// itself poisons the domain). The summary must call out both, separately.
func TestSummarizeFallbackFailures_DistinguishesNetworkFromPoisoning(t *testing.T) {
	r := NSProbeReport{
		FallbackResults: []ProbeResult{
			{Nameserver: "https://1.1.1.1/dns-query", Err: "context deadline exceeded"},
			{Nameserver: "https://8.8.8.8/dns-query", Err: "context deadline exceeded"},
			{Nameserver: "https://dns.alidns.com/dns-query", Hijacked: true, IPs: []string{"198.18.45.34"}},
			{Nameserver: "https://doh.pub/dns-query", Hijacked: true, IPs: []string{"198.18.45.34"}},
		},
	}

	got := r.SummarizeFallbackFailures()

	wantSubstrings := []string{
		"2 个连接超时/不可达",
		"https://1.1.1.1/dns-query",
		"https://8.8.8.8/dns-query",
		"2 个返回了假 IP",
		"https://dns.alidns.com/dns-query",
		"https://doh.pub/dns-query",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(got, want) {
			t.Errorf("summary %q missing expected substring %q", got, want)
		}
	}
}
