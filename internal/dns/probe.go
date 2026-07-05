package dns

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/wujun4code/clashforge/internal/netreserved"
)

// KnownFakeIPRanges is the full IANA reserved/special-purpose registry plus a
// handful of known non-reserved tricks (see netreserved.KnownNonReservedFakeIPRanges).
// A hijacked/substituted DNS answer for a domain that should have a real
// public IP is detected by checking membership here — NOT by matching one or
// two specific "known fake-ip pool" CIDRs. This matters because virtually
// every fake-ip implementation (Mihomo, OpenClash, Surge, ...) is
// constrained to pick its pool from non-globally-routable space (or it would
// collide with real destinations), so checking the full reserved registry
// catches an upstream hijack regardless of which specific pool it happens to
// use — including one we've never seen before — not just the historical
// 198.18.0.0/16 default.
var KnownFakeIPRanges = append(
	append([]string{}, netreserved.IANARanges...),
	netreserved.KnownNonReservedFakeIPRanges...,
)

// DefaultFallbackDNS is the built-in list of DoH servers offered as replacement
// when user-configured UDP nameservers are found to be hijacked.
// Domestic (AliDNS/DNSPod) entries only clear a *network-path* hijack (e.g. an
// upstream router transparently redirecting UDP:53) — if the resolver itself
// applies a *content-level* block to the queried domain (a domestic recursive
// resolver refusing/poisoning a known proxy-seller domain, independent of any
// network interception), asking the same operator over DoH doesn't help,
// since it's still that operator's own resolution policy answering. Foreign
// entries (Cloudflare/Google/Quad9) are the only candidates with a chance of
// clearing that case — they're kept diverse (three separate anycast networks/
// operators) because GFW's blocking of specific foreign IPs is inconsistent
// and can vary between attempts; having more than two hedges against whichever
// one or two happen to be blocked at probe time.
var DefaultFallbackDNS = []string{
	"https://dns.alidns.com/dns-query", // Alibaba DNS (China-friendly)
	"https://doh.pub/dns-query",        // DNSPod / Tencent (China-friendly)
	"https://1.1.1.1/dns-query",        // Cloudflare
	"https://8.8.8.8/dns-query",        // Google
	"https://9.9.9.9/dns-query",        // Quad9 — separate operator/network from the two above
}

// ProbeResult is the result of a single DNS lookup via one nameserver.
type ProbeResult struct {
	Nameserver string   `json:"nameserver"`
	Hostname   string   `json:"hostname"`
	IPs        []string `json:"ips,omitempty"`
	Hijacked   bool     `json:"hijacked"`
	Err        string   `json:"error,omitempty"`
}

// NSProbeReport summarises the outcome of probing all nameservers.
type NSProbeReport struct {
	// AllClear is true when no nameserver returned a fake-ip.
	AllClear bool `json:"all_clear"`
	// HijackedNameservers lists nameservers that returned fake-ip for ≥1 hostname.
	HijackedNameservers []string `json:"hijacked_nameservers,omitempty"`
	// WorkingNameservers lists nameservers that returned real IPs for all tested hostnames.
	WorkingNameservers []string `json:"working_nameservers,omitempty"`
	// SuggestedFallbacks holds DoH servers from DefaultFallbackDNS that are reachable
	// and returned real IPs. Only populated when hijacking was detected.
	SuggestedFallbacks []string `json:"suggested_fallbacks,omitempty"`
	// Results contains the raw per-probe detail for the original nameservers.
	Results []ProbeResult `json:"results"`
	// FallbackResults contains the raw per-probe detail for each DoH candidate
	// tried after hijacking was detected (one entry per candidate — only the
	// first sample hostname is used per candidate to keep the probe fast).
	// Only populated when hijacking was detected. Kept separate from Results
	// so callers can tell "why did the fallback search fail" apart from the
	// original-nameserver hijack detection itself.
	FallbackResults []ProbeResult `json:"fallback_results,omitempty"`
}

// SummarizeFallbackFailures groups FallbackResults by failure kind — network
// unreachable/timeout vs. reachable-but-still-returned-a-fake-ip — so a "all
// fallbacks failed" message can say *why* instead of just "all failed".
// Returns "" when there's nothing to summarize (no fallback was attempted, or
// every candidate is missing a result — should not normally happen).
func (r NSProbeReport) SummarizeFallbackFailures() string {
	var unreachable, poisoned []string
	for _, res := range r.FallbackResults {
		switch {
		case res.Err != "":
			unreachable = append(unreachable, res.Nameserver)
		case res.Hijacked:
			poisoned = append(poisoned, res.Nameserver)
		}
	}
	var parts []string
	if len(unreachable) > 0 {
		parts = append(parts, fmt.Sprintf(
			"%d 个连接超时/不可达，疑似网络层被拦截（与域名污染无关）: %s",
			len(unreachable), strings.Join(unreachable, ", ")))
	}
	if len(poisoned) > 0 {
		parts = append(parts, fmt.Sprintf(
			"%d 个返回了假 IP，疑似该服务商对此域名做了污染: %s",
			len(poisoned), strings.Join(poisoned, ", ")))
	}
	return strings.Join(parts, "；")
}

// ProbeNameservers tests each nameserver against a sample of hostnames and
// returns a report indicating which nameservers are returning fake-ip responses.
//
// nameservers may be bare IPs ("223.5.5.5"), IP:port ("223.5.5.5:53"), or
// DoH URLs ("https://dns.alidns.com/dns-query").
// hostnames should be the proxy-server domain names extracted from subscriptions.
func ProbeNameservers(ctx context.Context, nameservers []string, hostnames []string) NSProbeReport {
	fakeRanges := parsePrefixes(KnownFakeIPRanges)

	// Take a sample of up to 3 hostnames to keep the probe fast.
	sample := hostnames
	if len(sample) > 3 {
		sample = sample[:3]
	}

	hijackedSet := make(map[string]bool)
	workingSet := make(map[string]bool)
	var results []ProbeResult

	for _, ns := range nameservers {
		if len(sample) == 0 {
			break
		}
		nsHijacked := false
		for _, h := range sample {
			r := probeOne(ctx, ns, h, fakeRanges)
			results = append(results, r)
			if r.Hijacked {
				nsHijacked = true
			}
		}
		if nsHijacked {
			hijackedSet[ns] = true
		} else {
			workingSet[ns] = true
		}
	}

	var hijacked, working []string
	for ns := range hijackedSet {
		hijacked = append(hijacked, ns)
	}
	for ns := range workingSet {
		if !hijackedSet[ns] {
			working = append(working, ns)
		}
	}

	report := NSProbeReport{
		HijackedNameservers: hijacked,
		WorkingNameservers:  working,
		Results:             results,
		AllClear:            len(hijacked) == 0,
	}

	// If any UDP nameservers are hijacked, find working DoH fallbacks.
	// Try DoH on the user's own hijacked nameserver IPs first — the upstream
	// hijack rule matches on udp dport 53 only, so re-asking the exact same
	// resolver over DoH (443) keeps the user's chosen provider and sidesteps
	// the hijack without switching to an unrelated public resolver. Only fall
	// back to DefaultFallbackDNS if that doesn't clear the hijack.
	if len(hijacked) > 0 && len(sample) > 0 {
		var candidates []string
		seen := make(map[string]bool)
		addCandidate := func(c string) {
			if c != "" && !seen[c] {
				seen[c] = true
				candidates = append(candidates, c)
			}
		}
		for _, ns := range hijacked {
			addCandidate(dohCandidateForIP(ns))
		}
		for _, fb := range DefaultFallbackDNS {
			addCandidate(fb)
		}
		for _, cand := range candidates {
			fbCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
			r := probeOne(fbCtx, cand, sample[0], fakeRanges)
			cancel()
			report.FallbackResults = append(report.FallbackResults, r)
			if r.Err == "" && !r.Hijacked {
				report.SuggestedFallbacks = append(report.SuggestedFallbacks, cand)
			}
		}
	}

	return report
}

// probeOne performs a single DNS A-record lookup for hostname using nameserver.
// nameserver may be a bare IP, IP:port, or https:// DoH URL.
func probeOne(ctx context.Context, nameserver, hostname string, fakeRanges []netip.Prefix) ProbeResult {
	r := ProbeResult{Nameserver: nameserver, Hostname: hostname}

	var ips []string
	var err error

	if strings.HasPrefix(nameserver, "https://") {
		ips, err = queryDoH(ctx, nameserver, hostname)
	} else {
		ips, err = queryUDP(ctx, nameserver, hostname)
	}

	if err != nil {
		r.Err = err.Error()
		return r
	}
	r.IPs = ips
	r.Hijacked = allInFakeRanges(ips, fakeRanges)
	return r
}

// queryUDP resolves hostname against the specified nameserver using UDP/53.
func queryUDP(ctx context.Context, nameserver, hostname string) ([]string, error) {
	host, port, err := net.SplitHostPort(nameserver)
	if err != nil {
		// nameserver has no port — default to 53.
		host = nameserver
		port = "53"
	}
	addr := net.JoinHostPort(host, port)

	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 3 * time.Second}
			return d.DialContext(ctx, "udp", addr)
		},
	}
	qctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return r.LookupHost(qctx, hostname)
}

type dohJSONResponse struct {
	Answer []struct {
		Type int    `json:"type"`
		Data string `json:"data"`
	} `json:"Answer"`
}

// queryDoH resolves hostname using a DoH server (HTTPS GET, application/dns-json).
func queryDoH(ctx context.Context, dohURL, hostname string) ([]string, error) {
	reqURL := fmt.Sprintf("%s?name=%s&type=A", dohURL, hostname)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/dns-json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DoH HTTP %d from %s", resp.StatusCode, dohURL)
	}

	var dnsResp dohJSONResponse
	if err := json.NewDecoder(resp.Body).Decode(&dnsResp); err != nil {
		return nil, fmt.Errorf("DoH JSON decode: %w", err)
	}

	var ips []string
	for _, ans := range dnsResp.Answer {
		if ans.Type == 1 { // A record
			ips = append(ips, ans.Data)
		}
	}
	return ips, nil
}

// dohCandidateForIP converts a bare-IP (or IP:port) nameserver into a DoH URL
// pointing at the same address, e.g. "223.5.5.5" -> "https://223.5.5.5/dns-query".
// Returns "" if ns is not a bare IP (already a DoH/DoT URL, or a hostname).
func dohCandidateForIP(ns string) string {
	host, _, err := net.SplitHostPort(ns)
	if err != nil {
		host = ns
	}
	if net.ParseIP(host) == nil {
		return ""
	}
	return fmt.Sprintf("https://%s/dns-query", host)
}

func parsePrefixes(cidrs []string) []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(cidrs))
	for _, cidr := range cidrs {
		if p, err := netip.ParsePrefix(cidr); err == nil {
			prefixes = append(prefixes, p)
		}
	}
	return prefixes
}

// allInFakeRanges returns true if every IP in ips falls inside at least one fakeRanges entry.
func allInFakeRanges(ips []string, fakeRanges []netip.Prefix) bool {
	if len(ips) == 0 {
		return false
	}
	for _, ipStr := range ips {
		addr, err := netip.ParseAddr(ipStr)
		if err != nil {
			continue
		}
		inFake := false
		for _, pfx := range fakeRanges {
			if pfx.Contains(addr) {
				inFake = true
				break
			}
		}
		if !inFake {
			return false
		}
	}
	return true
}

// AnyInFakeIPRanges is the exported counterpart of allInFakeRanges that checks
// whether ANY of the given IPs falls in Mihomo's known fake-ip ranges.
// Unlike allInFakeRanges it returns true as soon as one IP matches.
func AnyInFakeIPRanges(ips []string) bool {
	ranges := parsePrefixes(KnownFakeIPRanges)
	for _, ipStr := range ips {
		addr, err := netip.ParseAddr(ipStr)
		if err != nil {
			continue
		}
		for _, pfx := range ranges {
			if pfx.Contains(addr) {
				return true
			}
		}
	}
	return false
}

// QueryUDPDirect resolves hostname against a bare-IP (or IP:port) nameserver
// using UDP and returns the A-record IPs.  Exported for use by the DNS-leak handler.
func QueryUDPDirect(ctx context.Context, nameserver, hostname string) ([]string, error) {
	return queryUDP(ctx, nameserver, hostname)
}

// QueryDoHDirect resolves hostname using a DoH URL and returns the A-record IPs.
// Exported for use by the DNS-leak handler.
func QueryDoHDirect(ctx context.Context, dohURL, hostname string) ([]string, error) {
	return queryDoH(ctx, dohURL, hostname)
}
