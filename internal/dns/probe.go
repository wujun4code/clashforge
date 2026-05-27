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
)

// KnownFakeIPRanges contains CIDR ranges used by Clash/Mihomo fake-ip.
// These cover the default 198.18.0.0/15 range (198.18.x.x and 198.19.x.x)
// and a few variant ranges seen in the wild.
var KnownFakeIPRanges = []string{
	"198.18.0.0/15", // Mihomo / OpenClash default (covers 198.18.0.0–198.19.255.255)
	"28.0.0.0/8",    // rare variant used by some Clash forks
}

// DefaultFallbackDNS is the built-in list of DoH servers offered as replacement
// when user-configured UDP nameservers are found to be hijacked.
var DefaultFallbackDNS = []string{
	"https://dns.alidns.com/dns-query", // Alibaba DNS (China-friendly)
	"https://doh.pub/dns-query",        // DNSPod / Tencent (China-friendly)
	"https://1.1.1.1/dns-query",        // Cloudflare
	"https://8.8.8.8/dns-query",        // Google
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
	// Results contains the raw per-probe detail.
	Results []ProbeResult `json:"results"`
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
	if len(hijacked) > 0 && len(sample) > 0 {
		for _, fb := range DefaultFallbackDNS {
			fbCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
			r := probeOne(fbCtx, fb, sample[0], fakeRanges)
			cancel()
			if r.Err == "" && !r.Hijacked {
				report.SuggestedFallbacks = append(report.SuggestedFallbacks, fb)
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
