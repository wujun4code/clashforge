package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	cfDNS "github.com/wujun4code/clashforge/internal/dns"
)

// dnsPathResult is the resolution outcome for one DNS path (server).
type dnsPathResult struct {
	// Name is a human-readable label shown in the UI.
	Name string `json:"name"`
	// Server is the address that was actually queried.
	Server string `json:"server"`
	// IPs contains the A-record addresses returned (empty on error or fake-ip).
	IPs []string `json:"ips,omitempty"`
	// IsFakeIP is true when ANY returned IP is in Mihomo's fake-ip range (198.18/15 or 28/8).
	IsFakeIP bool `json:"is_fake_ip"`
	// Error holds any resolution failure message.
	Error string `json:"error,omitempty"`
}

// externalResolver is an actual DNS resolver observed from the internet side
// (i.e., a resolver IP that queried an authoritative DNS server for our test domain).
type externalResolver struct {
	// IP is the resolver's address as seen by the authoritative DNS server.
	IP string `json:"ip"`
	// CountryName is the full country name from GeoIP.
	CountryName string `json:"country_name"`
	// CountryCode is the ISO 3166-1 alpha-2 country code.
	CountryCode string `json:"country_code"`
	// ISP is the internet service provider or org name.
	ISP string `json:"isp"`
	// IsLeak is true when the resolver is in a country that indicates a DNS leak
	// (currently: China-based resolvers, which mean queries are visible to Chinese entities).
	IsLeak bool `json:"is_leak"`
	// UpstreamIntercepted is true when this nameserver returned Fake-IP in the internal
	// path test, meaning an upstream Mihomo is transparently intercepting its traffic.
	// In this case IsLeak is forced to false — the server never actually received the query.
	UpstreamIntercepted bool `json:"upstream_intercepted,omitempty"`
}

// dnsLeakTestResult is the payload returned by GET /api/v1/health/dns-leak.
type dnsLeakTestResult struct {
	// TestDomain is the domain used for internal-path probes.
	TestDomain string `json:"test_domain"`
	// Paths is one entry per internal DNS path probed (Mihomo, system, upstream, DoH ref).
	Paths []dnsPathResult `json:"paths"`
	// ExternalResolvers holds resolver IPs observed from the internet perspective.
	// Populated from bash.ws probe or GeoIP-enriched nameserver list as fallback.
	ExternalResolvers []externalResolver `json:"external_resolvers,omitempty"`
	// ExternalMethod describes how external_resolvers were obtained.
	ExternalMethod string `json:"external_method,omitempty"`
	// MihomoIntercepting is true when Mihomo's DNS port returned fake-ip.
	MihomoIntercepting bool `json:"mihomo_intercepting"`
	// HasLeak is true when any DNS resolver visible from the internet is in a
	// jurisdiction that indicates the user's DNS traffic is exposed.
	HasLeak  bool   `json:"has_leak"`
	Summary  string `json:"summary"`
	TestedAt string `json:"tested_at"`
	Err      string `json:"error,omitempty"`
}

// dnsLeakProbeHost is the domain used for internal multi-path fake-ip comparison.
const dnsLeakProbeHost = "google.com"

// ── HTTP client that bypasses Mihomo fake-ip DNS ──────────────────────────────
//
// The ClashForge Go process is typically exempt from its own iptables redirect
// rules.  When Mihomo is running in fake-ip mode, a plain net.DefaultResolver
// call resolves hostnames to 198.18.x.x fake addresses; connecting to those
// from the service process fails.  This transport instead resolves hostnames
// via Cloudflare DoH at 1.1.1.1 (a literal IP — no DNS lookup required) so
// external HTTP calls (bash.ws, ip-api.com) always reach the real server.
func newDNSBypassClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout: 6 * time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			// If addr is already an IP literal, connect straight away.
			if net.ParseIP(host) != nil {
				return (&net.Dialer{Timeout: 6 * time.Second}).DialContext(ctx, network, addr)
			}
			// Resolve via Cloudflare DoH (1.1.1.1 is an IP address — zero DNS
			// dependency even when Mihomo fake-ip is active).
			dohCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			ips, dohErr := cfDNS.QueryDoHDirect(dohCtx, "https://1.1.1.1/dns-query", host)
			cancel()
			if dohErr != nil || len(ips) == 0 {
				// Best-effort fallback to system resolver.
				return (&net.Dialer{Timeout: 6 * time.Second}).DialContext(ctx, network, addr)
			}
			return (&net.Dialer{Timeout: 6 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ips[0], port))
		},
	}
	return &http.Client{Timeout: timeout, Transport: transport}
}

// ── bash.ws external resolver probe ──────────────────────────────────────────
//
// Bash.ws hosts an authoritative DNS zone for *.bash.ws.  When a resolver
// performs a recursive lookup for a unique subdomain (e.g. 123456-1.bash.ws),
// bash.ws logs the resolver IP.  We trigger those lookups by querying each
// configured nameserver directly (bypassing Mihomo fake-ip cache), then fetch
// the logged resolver list from bash.ws's REST endpoint.

type bashWSEntry struct {
	IP          string `json:"ip"`
	CountryName string `json:"country_name"`
	CountryCode string `json:"country_code"`
	ISP         string `json:"isp"`
}

func probeBashWS(ctx context.Context, nameservers []string, httpClient *http.Client) ([]externalResolver, bool) {
	testID := fmt.Sprintf("%d", rand.Intn(900000)+100000)

	// Register the test so bash.ws expects our probe subdomains.
	startURL := fmt.Sprintf("https://bash.ws/dnsleak/test/start/%s", testID)
	if req, err := http.NewRequestWithContext(ctx, "GET", startURL, nil); err == nil {
		resp, err := httpClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}

	// Trigger DNS lookups for the test subdomains via each nameserver.
	// QueryUDPDirect bypasses Mihomo's DNS interception and hits each upstream
	// nameserver directly; that server then performs recursive resolution and
	// hits bash.ws's authoritative NS, which logs the resolver IP.
	var wg sync.WaitGroup
	for n := 1; n <= 10; n++ {
		domain := fmt.Sprintf("%s-%d.bash.ws", testID, n)

		// System resolver (may route through Mihomo → upstream NS → bash.ws).
		wg.Add(1)
		go func(d string) {
			defer wg.Done()
			pCtx, c := context.WithTimeout(ctx, 5*time.Second)
			defer c()
			net.DefaultResolver.LookupHost(pCtx, d) //nolint:errcheck
		}(domain)

		// Each configured UDP nameserver.
		for _, ns := range nameservers {
			if strings.HasPrefix(ns, "https://") {
				continue
			}
			wg.Add(1)
			go func(server, d string) {
				defer wg.Done()
				pCtx, c := context.WithTimeout(ctx, 5*time.Second)
				defer c()
				cfDNS.QueryUDPDirect(pCtx, server, d) //nolint:errcheck
			}(ns, domain)
		}
	}
	wg.Wait()

	// Give bash.ws a moment to index the incoming queries.
	select {
	case <-time.After(2 * time.Second):
	case <-ctx.Done():
		return nil, false
	}

	// Retrieve the logged resolver list.
	resultsURL := fmt.Sprintf("https://bash.ws/dnsleak/test/%s?lang=en", testID)
	req, err := http.NewRequestWithContext(ctx, "GET", resultsURL, nil)
	if err != nil {
		return nil, false
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()

	// bash.ws returns text/html on unknown test IDs or when no queries arrived.
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/html") {
		return nil, false
	}

	var entries []bashWSEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil || len(entries) == 0 {
		return nil, false
	}

	resolvers := make([]externalResolver, 0, len(entries))
	for _, e := range entries {
		resolvers = append(resolvers, externalResolver{
			IP:          e.IP,
			CountryName: e.CountryName,
			CountryCode: e.CountryCode,
			ISP:         e.ISP,
			IsLeak:      e.CountryCode == "CN",
		})
	}
	return resolvers, true
}

// ── GeoIP fallback: enrich configured nameservers via ip-api.com ─────────────
//
// When bash.ws is unreachable or returns no results, we fall back to querying
// ip-api.com for each configured UDP nameserver IP.  This gives the user the
// same country/ISP view without requiring external DNS-probe infrastructure.

type ipAPIEntry struct {
	Status      string `json:"status"`
	Country     string `json:"country"`
	CountryCode string `json:"countryCode"`
	ISP         string `json:"isp"`
	Query       string `json:"query"`
}

func enrichNameserversGeoIP(ctx context.Context, nameservers []string, httpClient *http.Client) []externalResolver {
	var (
		mu      sync.Mutex
		results []externalResolver
		wg      sync.WaitGroup
	)

	for _, ns := range nameservers {
		if strings.HasPrefix(ns, "https://") {
			// DoH URL: no IP to geo-locate; mark it as inherently safe.
			mu.Lock()
			results = append(results, externalResolver{
				IP:          ns,
				CountryName: "DNS over HTTPS",
				ISP:         "加密 DoH",
				IsLeak:      false,
			})
			mu.Unlock()
			continue
		}

		wg.Add(1)
		go func(nsAddr string) {
			defer wg.Done()
			host, _, err := net.SplitHostPort(nsAddr)
			if err != nil {
				host = nsAddr
			}

			pCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
			defer cancel()

			reqURL := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,country,countryCode,isp,query", host)
			req, err := http.NewRequestWithContext(pCtx, "GET", reqURL, nil)
			if err != nil {
				mu.Lock()
				results = append(results, externalResolver{IP: host, ISP: "GeoIP 查询失败"})
				mu.Unlock()
				return
			}
			resp, err := httpClient.Do(req)
			if err != nil {
				mu.Lock()
				results = append(results, externalResolver{IP: host, ISP: "GeoIP 查询失败"})
				mu.Unlock()
				return
			}
			defer resp.Body.Close()

			var geo ipAPIEntry
			if err := json.NewDecoder(resp.Body).Decode(&geo); err != nil || geo.Status != "success" {
				mu.Lock()
				results = append(results, externalResolver{IP: host, ISP: "GeoIP 解析失败"})
				mu.Unlock()
				return
			}

			mu.Lock()
			results = append(results, externalResolver{
				IP:          host,
				CountryName: geo.Country,
				CountryCode: geo.CountryCode,
				ISP:         geo.ISP,
				// All CN-based resolvers are marked as potential leaks because DNS
				// queries are visible to Chinese entities, regardless of whether
				// the provider is an ISP or a commercial tech company.
				IsLeak: geo.CountryCode == "CN",
			})
			mu.Unlock()
		}(ns)
	}
	wg.Wait()
	return results
}

// handleDNSLeakTest runs two probes concurrently:
//
//  1. Internal multi-path fake-ip comparison (Mihomo health check).
//  2. External resolver detection via bash.ws (or GeoIP fallback), which shows
//     the actual DNS resolver IPs observed from the internet — the same view
//     that services like ip.net.coffee provide.
//
// GET /api/v1/health/dns-leak
func handleDNSLeakTest(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		now := time.Now().UTC().Format(time.RFC3339)

		httpClient := newDNSBypassClient(10 * time.Second)

		// ── Run internal and external probes concurrently ─────────────────────

		var (
			internalPaths      []dnsPathResult
			mihomoIntercepting bool
			externalResolvers  []externalResolver
			externalMethod     string
			wgOuter            sync.WaitGroup
		)

		// ── Probe 1: internal fake-ip multi-path comparison ───────────────────
		wgOuter.Add(1)
		go func() {
			defer wgOuter.Done()

			type indexedPath struct {
				idx  int
				path dnsPathResult
			}

			var (
				mu      sync.Mutex
				results = make([]dnsPathResult, 0, 8)
				wg      sync.WaitGroup
			)

			addPath := func(idx int, p dnsPathResult) {
				mu.Lock()
				defer mu.Unlock()
				for len(results) <= idx {
					results = append(results, dnsPathResult{})
				}
				results[idx] = p
			}

			pathIdx := 0

			// Mihomo DNS port
			if deps.Config.DNS.Enable && deps.Config.Ports.DNS > 0 {
				idx := pathIdx
				pathIdx++
				server := fmt.Sprintf("127.0.0.1:%d", deps.Config.Ports.DNS)
				label := fmt.Sprintf("Mihomo DNS (:%d)", deps.Config.Ports.DNS)
				wg.Add(1)
				go func() {
					defer wg.Done()
					pCtx, c := context.WithTimeout(ctx, 5*time.Second)
					defer c()
					ips, err := cfDNS.QueryUDPDirect(pCtx, server, dnsLeakProbeHost)
					p := dnsPathResult{Name: label, Server: server, IPs: ips}
					if err != nil {
						p.Error = err.Error()
					} else {
						p.IsFakeIP = cfDNS.AnyInFakeIPRanges(ips)
					}
					addPath(idx, p)
				}()
			}

			// System default resolver
			{
				idx := pathIdx
				pathIdx++
				wg.Add(1)
				go func() {
					defer wg.Done()
					pCtx, c := context.WithTimeout(ctx, 5*time.Second)
					defer c()
					ips, err := net.DefaultResolver.LookupHost(pCtx, dnsLeakProbeHost)
					p := dnsPathResult{
						Name:   "系统 DNS",
						Server: "系统默认 (/etc/resolv.conf)",
						IPs:    ips,
					}
					if err != nil {
						p.Error = err.Error()
					} else {
						p.IsFakeIP = cfDNS.AnyInFakeIPRanges(ips)
					}
					addPath(idx, p)
				}()
			}

			// Configured upstream nameservers
			for _, ns := range deps.Config.DNS.Nameservers {
				idx := pathIdx
				pathIdx++
				nsAddr := ns
				wg.Add(1)
				go func() {
					defer wg.Done()
					pCtx, c := context.WithTimeout(ctx, 6*time.Second)
					defer c()
					var (
						ips []string
						err error
					)
					if len(nsAddr) > 8 && nsAddr[:8] == "https://" {
						ips, err = cfDNS.QueryDoHDirect(pCtx, nsAddr, dnsLeakProbeHost)
					} else {
						ips, err = cfDNS.QueryUDPDirect(pCtx, nsAddr, dnsLeakProbeHost)
					}
					p := dnsPathResult{
						Name:   "上游 DNS: " + nsAddr,
						Server: nsAddr,
						IPs:    ips,
					}
					if err != nil {
						p.Error = err.Error()
					} else {
						p.IsFakeIP = cfDNS.AnyInFakeIPRanges(ips)
					}
					addPath(idx, p)
				}()
			}

			// Cloudflare DoH reference
			{
				idx := pathIdx
				wg.Add(1)
				go func() {
					defer wg.Done()
					pCtx, c := context.WithTimeout(ctx, 8*time.Second)
					defer c()
					ips, err := cfDNS.QueryDoHDirect(pCtx, "https://1.1.1.1/dns-query", dnsLeakProbeHost)
					p := dnsPathResult{
						Name:   "Cloudflare DoH (参考)",
						Server: "https://1.1.1.1/dns-query",
						IPs:    ips,
					}
					if err != nil {
						p.Error = err.Error()
					} else {
						p.IsFakeIP = cfDNS.AnyInFakeIPRanges(ips)
					}
					addPath(idx, p)
				}()
			}

			wg.Wait()

			// Trim empty trailing slots.
			for len(results) > 0 && results[len(results)-1].Name == "" {
				results = results[:len(results)-1]
			}

			// Determine Mihomo-intercepting status.
			intercepting := false
			for _, p := range results {
				if strings.HasPrefix(p.Name, "Mihomo") && p.IsFakeIP {
					intercepting = true
					break
				}
			}

			mu.Lock()
			internalPaths = results
			mihomoIntercepting = intercepting
			mu.Unlock()
		}()

		// ── Probe 2: external resolver detection ──────────────────────────────
		wgOuter.Add(1)
		go func() {
			defer wgOuter.Done()

			// Try bash.ws first — gives real external perspective.
			resolvers, ok := probeBashWS(ctx, deps.Config.DNS.Nameservers, httpClient)
			if ok && len(resolvers) > 0 {
				externalResolvers = resolvers
				externalMethod = "bash.ws"
				return
			}

			// Fallback: GeoIP-enrich the configured nameservers.
			if len(deps.Config.DNS.Nameservers) > 0 {
				resolvers = enrichNameserversGeoIP(ctx, deps.Config.DNS.Nameservers, httpClient)
				if len(resolvers) > 0 {
					externalResolvers = resolvers
					externalMethod = "geoip-nameservers"
				}
			}
		}()

		wgOuter.Wait()

		// ── Analyse results ───────────────────────────────────────────────────

		// Internal analysis: check system DNS state and build a set of upstream
		// nameserver IPs that returned fake-ip (meaning an upstream Mihomo is
		// transparently intercepting their traffic — they never received the query).
		systemFakeIP := false
		systemError := false
		systemHasData := false
		interceptedNS := make(map[string]bool) // nameserver IPs intercepted by upstream Mihomo
		allUpstreamIntercepted := true          // true when EVERY configured NS returned fake-ip
		anyUpstreamConfigured := false

		for _, p := range internalPaths {
			switch {
			case p.Name == "系统 DNS":
				systemHasData = true
				if p.IsFakeIP {
					systemFakeIP = true
				}
				if p.Error != "" {
					systemError = true
				}
			case strings.HasPrefix(p.Name, "上游 DNS:"):
				anyUpstreamConfigured = true
				host, _, err := net.SplitHostPort(p.Server)
				if err != nil {
					host = p.Server
				}
				if p.IsFakeIP {
					interceptedNS[host] = true
					interceptedNS[p.Server] = true
				} else {
					// At least one upstream got a real response → not all intercepted.
					allUpstreamIntercepted = false
				}
			}
		}
		if !anyUpstreamConfigured {
			allUpstreamIntercepted = false
		}

		// Cross-reference: when using the GeoIP fallback (not a real external probe),
		// a CN nameserver that returned fake-ip was intercepted by an upstream Mihomo
		// and never actually handled the query — mark it accordingly.
		if externalMethod == "geoip-nameservers" {
			for i := range externalResolvers {
				r := &externalResolvers[i]
				host, _, err := net.SplitHostPort(r.IP)
				if err != nil {
					host = r.IP
				}
				if r.IsLeak && (interceptedNS[host] || interceptedNS[r.IP]) {
					r.IsLeak = false
					r.UpstreamIntercepted = true
				}
			}
		}

		// External analysis: any leak-tagged resolver means DNS is exposed.
		externalHasLeak := false
		interceptedLeakCount := 0
		for _, r := range externalResolvers {
			if r.IsLeak {
				externalHasLeak = true
			}
			if r.UpstreamIntercepted {
				interceptedLeakCount++
			}
		}

		hasLeak := externalHasLeak

		// Build summary
		var summary string
		switch {
		case !deps.Config.DNS.Enable || deps.Config.Ports.DNS == 0:
			summary = "ClashForge 未启用 DNS 功能，无法判断 DNS 泄露状态。请在配置中启用 DNS 并重新检测。"
			hasLeak = false

		case externalHasLeak && len(externalResolvers) > 0:
			leakCount := 0
			for _, r := range externalResolvers {
				if r.IsLeak {
					leakCount++
				}
			}
			summary = fmt.Sprintf(
				"检测到 DNS 泄露！%d 个 DNS 解析器位于中国境内，这意味着您的 DNS 查询对中国实体可见。"+
					"建议在 Mihomo 配置中将上游 DNS 改为 DoH（如 https://1.1.1.1/dns-query）并经代理转发。",
				leakCount,
			)

		case externalMethod == "geoip-nameservers" && allUpstreamIntercepted && interceptedLeakCount > 0 && systemFakeIP:
			// All configured nameservers were intercepted by an upstream Mihomo.
			// The CN nameservers in config never received queries — no actual leak.
			summary = "您的 DNS 查询已被上游 Mihomo 完全拦截（所有上游服务器均返回 Fake-IP），" +
				"配置中的中国 DNS 服务器实际上并未收到任何查询。DNS 隐私取决于上游路由器的 DNS 配置。"
			hasLeak = false

		case len(externalResolvers) > 0 && !externalHasLeak:
			summary = "未检测到 DNS 泄露。所有 DNS 解析器均在中国境外，您的 DNS 查询通过代理或境外 DNS 处理。"

		case mihomoIntercepting && systemFakeIP:
			summary = "Mihomo DNS 正在拦截 DNS 查询（返回 Fake-IP），系统 DNS 解析结果一致，未检测到泄露。"

		case mihomoIntercepting && !systemFakeIP && systemHasData && !systemError:
			summary = "Mihomo DNS 正在拦截（返回 Fake-IP），但系统 DNS 返回真实 IP，DNS 查询可能绕过了 Mihomo，存在泄露风险。"
			hasLeak = true

		case mihomoIntercepting && systemError:
			summary = "Mihomo DNS 正在拦截（返回 Fake-IP），系统 DNS 查询失败，请检查 DNS 设置。"

		case !mihomoIntercepting && systemHasData:
			if deps.Config.DNS.Enable {
				summary = "未检测到 Mihomo DNS 拦截（未返回 Fake-IP）。可能原因：Mihomo 核心未运行、DNS 端口未就绪，或 fake-ip 模式未启用。"
			} else {
				summary = "Mihomo DNS 功能未启用，当前使用系统 DNS 直接解析。"
			}

		default:
			summary = "检测完成，请参考下方各路径结果手动判断。"
		}

		JSON(w, http.StatusOK, dnsLeakTestResult{
			TestDomain:         dnsLeakProbeHost,
			Paths:              internalPaths,
			ExternalResolvers:  externalResolvers,
			ExternalMethod:     externalMethod,
			MihomoIntercepting: mihomoIntercepting,
			HasLeak:            hasLeak,
			Summary:            summary,
			TestedAt:           now,
		})
	}
}
