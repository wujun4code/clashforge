// Package diag provides comprehensive domain diagnostic utilities used by the
// ClashForge health/probe-domain API endpoint.
package diag

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/geoip2-golang"
)

// DNSSourceResult holds the DNS resolution result from one upstream source.
type DNSSourceResult struct {
	Source string        `json:"source"` // "mihomo" | "cn" | "isp" | "cloudflare" | "google"
	Label  string        `json:"label"`
	Server string        `json:"server"`
	IPs    []string      `json:"ips,omitempty"`
	GeoIPs []GeoIPResult `json:"geo_ips,omitempty"`
	Error  string        `json:"error,omitempty"`
}

// GeoIPResult holds country information for a single IP.
type GeoIPResult struct {
	IP      string `json:"ip"`
	Country string `json:"country"` // ISO 3166-1 alpha-2, e.g. "CN", "SG"
	IsCN    bool   `json:"is_cn"`
}

// RoutingInfo holds the routing decision observed from active mihomo connections.
type RoutingInfo struct {
	HasActive   bool     `json:"has_active"`
	Rule        string   `json:"rule,omitempty"`
	RulePayload string   `json:"rule_payload,omitempty"`
	Chains      []string `json:"chains,omitempty"`
	Network     string   `json:"network,omitempty"`
}

// DiagNote is a single diagnosis conclusion entry.
type DiagNote struct {
	Level   string `json:"level"` // "ok" | "warn" | "error"
	Code    string `json:"code"`
	Message string `json:"message"`
}

// DomainDiag is the full diagnostic result for a domain.
type DomainDiag struct {
	DNSSources []DNSSourceResult `json:"dns_sources"`
	Routing    RoutingInfo       `json:"routing"`
	FakeIP     string            `json:"fake_ip,omitempty"`
	Diagnoses  []DiagNote        `json:"diagnoses"`
}

// Config holds runtime dependencies for the diagnostic.
type Config struct {
	MihomoAPIPort int
	GeoIPPath     string // path to Country.mmdb
}

// Run executes the full domain diagnostic and returns structured results.
func Run(ctx context.Context, domain string, cfg Config) DomainDiag {
	var result DomainDiag

	// Open GeoIP reader once; shared across all lookups.
	var geo *geoip2.Reader
	if cfg.GeoIPPath != "" {
		geo, _ = geoip2.Open(cfg.GeoIPPath)
		if geo != nil {
			defer geo.Close()
		}
	}

	// Build DNS source list.
	sources := buildDNSSources(cfg.MihomoAPIPort)

	// Query all sources in parallel.
	results := make([]DNSSourceResult, len(sources))
	var wg sync.WaitGroup
	for i, s := range sources {
		wg.Add(1)
		go func(idx int, src dnsSource) {
			defer wg.Done()
			r := querySource(ctx, src, domain, geo)
			results[idx] = r
		}(i, s)
	}
	wg.Wait()
	result.DNSSources = results

	// Routing: scan active mihomo connections for this domain.
	result.Routing = queryRouting(ctx, cfg.MihomoAPIPort, domain)

	// Fake-IP: what fake address was handed to LAN clients for this domain.
	result.FakeIP = queryFakeIP(domain)

	// Generate diagnoses.
	result.Diagnoses = diagnose(results, result.Routing)

	return result
}

// ── DNS sources ───────────────────────────────────────────────────────────────

type dnsSource struct {
	source string
	label  string
	server string // empty means use mihomo internal API
}

func buildDNSSources(mihomoPort int) []dnsSource {
	sources := []dnsSource{
		{"mihomo", "Mihomo 内部（实际使用）", fmt.Sprintf("127.0.0.1:%d", mihomoPort)},
	}

	// ISP DNS from DHCP (best-effort).
	if isp := detectISPDNS(); isp != "" {
		sources = append(sources, dnsSource{"isp", fmt.Sprintf("ISP DNS (%s)", isp), isp})
	}

	sources = append(sources,
		dnsSource{"cn_1", "阿里 DNS (223.5.5.5)", "223.5.5.5"},
		dnsSource{"cn_2", "腾讯 DNS (119.29.29.29)", "119.29.29.29"},
		dnsSource{"cloudflare", "Cloudflare (1.1.1.1)", "1.1.1.1"},
		dnsSource{"google", "Google (8.8.8.8)", "8.8.8.8"},
	)
	return sources
}

func querySource(ctx context.Context, src dnsSource, domain string, geo *geoip2.Reader) DNSSourceResult {
	r := DNSSourceResult{Source: src.source, Label: src.label, Server: src.server}

	qctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var ips []string
	var err error

	if src.source == "mihomo" {
		// Query mihomo's internal DNS resolver via its REST API.
		port := parsePort(src.server)
		ips, err = queryMihomoDNS(qctx, port, domain)
	} else {
		ips, err = queryUDP(qctx, src.server, domain)
	}

	if err != nil {
		r.Error = err.Error()
		return r
	}
	r.IPs = ips
	if geo != nil {
		for _, ip := range ips {
			r.GeoIPs = append(r.GeoIPs, lookupGeoIP(geo, ip))
		}
	}
	return r
}

// queryMihomoDNS calls Mihomo's /dns/query REST endpoint.
func queryMihomoDNS(ctx context.Context, port int, domain string) ([]string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/dns/query?name=%s&type=A", port, domain)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var body struct {
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	var ips []string
	for _, a := range body.Answer {
		if a.Type == 1 { // A record
			ips = append(ips, a.Data)
		}
	}
	return ips, nil
}

// queryUDP resolves domain against a plain UDP DNS server.
func queryUDP(ctx context.Context, server, domain string) ([]string, error) {
	host, port, err := net.SplitHostPort(server)
	if err != nil {
		host, port = server, "53"
	}
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "udp", net.JoinHostPort(host, port))
		},
	}
	addrs, err := r.LookupHost(ctx, domain)
	if err != nil {
		return nil, err
	}
	// Filter to IPv4 only to match A records.
	var out []string
	for _, a := range addrs {
		if ip := net.ParseIP(a); ip != nil && ip.To4() != nil {
			out = append(out, a)
		}
	}
	return out, nil
}

// ── GeoIP ─────────────────────────────────────────────────────────────────────

func lookupGeoIP(geo *geoip2.Reader, ipStr string) GeoIPResult {
	r := GeoIPResult{IP: ipStr}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return r
	}
	record, err := geo.Country(ip)
	if err != nil {
		return r
	}
	r.Country = record.Country.IsoCode
	if r.Country == "" {
		r.Country = record.RegisteredCountry.IsoCode
	}
	r.IsCN = r.Country == "CN"
	return r
}

// ── Routing ───────────────────────────────────────────────────────────────────

func queryRouting(ctx context.Context, mihomoPort int, domain string) RoutingInfo {
	url := fmt.Sprintf("http://127.0.0.1:%d/connections", mihomoPort)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return RoutingInfo{}
	}
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return RoutingInfo{}
	}
	defer resp.Body.Close()

	var body struct {
		Connections []struct {
			Metadata struct {
				Host    string `json:"host"`
				Network string `json:"network"`
			} `json:"metadata"`
			Rule        string   `json:"rule"`
			RulePayload string   `json:"rulePayload"`
			Chains      []string `json:"chains"`
		} `json:"connections"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return RoutingInfo{}
	}

	// Find the most recent connection for this domain.
	for _, c := range body.Connections {
		if strings.EqualFold(c.Metadata.Host, domain) ||
			strings.HasSuffix(c.Metadata.Host, "."+domain) {
			return RoutingInfo{
				HasActive:   true,
				Rule:        c.Rule,
				RulePayload: c.RulePayload,
				Chains:      c.Chains,
				Network:     c.Metadata.Network,
			}
		}
	}
	return RoutingInfo{}
}

// ── Fake-IP ───────────────────────────────────────────────────────────────────

// queryFakeIP resolves domain against the local DNS port (127.0.0.1:53) to
// retrieve the fake-IP assigned by Mihomo to LAN clients.
func queryFakeIP(domain string) string {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "udp", "127.0.0.1:53")
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	addrs, err := r.LookupHost(ctx, domain)
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		if isFakeIP(a) {
			return a
		}
	}
	return ""
}

func isFakeIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	// Mihomo default fake-ip range: 198.18.0.0/15
	fakeStart := net.ParseIP("198.18.0.0")
	fakeEnd := net.ParseIP("198.19.255.255")
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	return bytesGTE(ip4, fakeStart.To4()) && bytesLTE(ip4, fakeEnd.To4())
}

func bytesGTE(a, b []byte) bool {
	for i := range a {
		if a[i] > b[i] {
			return true
		}
		if a[i] < b[i] {
			return false
		}
	}
	return true
}

func bytesLTE(a, b []byte) bool {
	for i := range a {
		if a[i] < b[i] {
			return true
		}
		if a[i] > b[i] {
			return false
		}
	}
	return true
}

// ── ISP DNS detection ─────────────────────────────────────────────────────────

// detectISPDNS reads the DHCP-assigned DNS server from resolv.conf files,
// skipping loopback addresses (which point to Mihomo itself).
func detectISPDNS() string {
	for _, path := range []string{
		"/tmp/resolv.conf.d/resolv.conf.auto",
		"/var/run/resolv.conf.auto",
		"/etc/resolv.conf",
	} {
		if ns := readFirstNonLoopbackNS(path); ns != "" {
			return ns
		}
	}
	return ""
}

func readFirstNonLoopbackNS(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "nameserver") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		ns := fields[1]
		if strings.HasPrefix(ns, "127.") || ns == "::1" {
			continue
		}
		return ns
	}
	return ""
}

// ── Diagnoses ─────────────────────────────────────────────────────────────────

func diagnose(sources []DNSSourceResult, routing RoutingInfo) []DiagNote {
	var notes []DiagNote

	// Find mihomo's resolved IPs and CN DNS IPs.
	var mihomoIPs, cnIPs []string
	var mihomoGeo []GeoIPResult
	for _, s := range sources {
		switch s.Source {
		case "mihomo":
			mihomoIPs = s.IPs
			mihomoGeo = s.GeoIPs
		case "cn_1", "cn_2":
			cnIPs = append(cnIPs, s.IPs...)
		}
	}

	// Check if mihomo resolved to a non-CN IP while CN DNS returns CN IPs.
	mihomoHasCN := anyIsCN(mihomoGeo)
	cnIPsExist := len(cnIPs) > 0

	if !mihomoHasCN && cnIPsExist && len(mihomoIPs) > 0 {
		notes = append(notes, DiagNote{
			Level:   "warn",
			Code:    "dns_mismatch",
			Message: fmt.Sprintf("Mihomo 实际解析到境外 IP（%s），但国内 DNS 返回大陆 IP。Fallback DNS 可能覆盖了正确结果，建议为此域名添加 nameserver-policy 强制使用国内 DNS。", firstIP(mihomoIPs)),
		})
	}

	// Check if domain going DIRECT to overseas.
	if routing.HasActive && isDirect(routing.Chains) && !mihomoHasCN && len(mihomoIPs) > 0 {
		notes = append(notes, DiagNote{
			Level:   "warn",
			Code:    "direct_overseas",
			Message: fmt.Sprintf("当前连接走直连（%s），目标 IP 为境外节点，可能导致延迟偏高。", strings.Join(routing.Chains, " → ")),
		})
	}

	// Check Cloudflare vs Google disagreement (Cloudflare pollution).
	cfIPs, googleIPs := ipsFor(sources, "cloudflare"), ipsFor(sources, "google")
	cfGeo := geoFor(sources, "cloudflare")
	if len(cfIPs) > 0 && len(googleIPs) > 0 && !setsOverlap(cfIPs, googleIPs) && !anyIsCN(cfGeo) {
		notes = append(notes, DiagNote{
			Level:   "warn",
			Code:    "cf_google_disagree",
			Message: fmt.Sprintf("Cloudflare（%s）与 Google DNS（%s）返回的 IP 不同，Cloudflare 返回境外 IP，可能导致 fallback 命中次优 CDN 节点。", firstIP(cfIPs), firstIP(googleIPs)),
		})
	}

	// All good.
	if len(notes) == 0 {
		msg := "DNS 解析路径正常"
		if mihomoHasCN {
			msg = "Mihomo 解析到大陆 IP，DNS 路径正常"
		}
		if routing.HasActive {
			msg += fmt.Sprintf("，当前走 %s", strings.Join(routing.Chains, " → "))
		}
		notes = append(notes, DiagNote{Level: "ok", Code: "ok", Message: msg})
	}

	return notes
}

// ── helpers ───────────────────────────────────────────────────────────────────

func parsePort(addr string) int {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 9090
	}
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	return port
}

func anyIsCN(geos []GeoIPResult) bool {
	for _, g := range geos {
		if g.IsCN {
			return true
		}
	}
	return false
}

func isDirect(chains []string) bool {
	for _, c := range chains {
		if strings.ToUpper(c) == "DIRECT" {
			return true
		}
	}
	return false
}

func firstIP(ips []string) string {
	if len(ips) == 0 {
		return ""
	}
	return ips[0]
}

func ipsFor(sources []DNSSourceResult, source string) []string {
	for _, s := range sources {
		if s.Source == source {
			return s.IPs
		}
	}
	return nil
}

func geoFor(sources []DNSSourceResult, source string) []GeoIPResult {
	for _, s := range sources {
		if s.Source == source {
			return s.GeoIPs
		}
	}
	return nil
}

func setsOverlap(a, b []string) bool {
	set := make(map[string]bool, len(a))
	for _, v := range a {
		set[v] = true
	}
	for _, v := range b {
		if set[v] {
			return true
		}
	}
	return false
}
