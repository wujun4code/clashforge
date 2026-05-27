package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
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
	// This means the query was intercepted by Mihomo's internal DNS engine.
	IsFakeIP bool `json:"is_fake_ip"`
	// Error holds any resolution failure message.
	Error string `json:"error,omitempty"`
}

// dnsLeakTestResult is the payload returned by GET /api/v1/health/dns-leak.
type dnsLeakTestResult struct {
	// TestDomain is the domain used for all probes (e.g. "google.com").
	TestDomain string `json:"test_domain"`
	// Paths is one entry per DNS path probed, in display order.
	Paths []dnsPathResult `json:"paths"`
	// MihomoIntercepting is true when Mihomo's DNS port returned fake-ip —
	// indicating Mihomo is actively intercepting DNS queries.
	MihomoIntercepting bool `json:"mihomo_intercepting"`
	// HasLeak is true when Mihomo is running/intercepting but the system resolver
	// appears to return real IPs, meaning DNS queries bypass Mihomo.
	HasLeak  bool   `json:"has_leak"`
	Summary  string `json:"summary"`
	TestedAt string `json:"tested_at"`
	Err      string `json:"error,omitempty"`
}

// dnsLeakProbeHost is the canonical foreign domain used for all DNS-leak probes.
// It should reliably return real IPs when queried via a direct DNS server, and
// Mihomo's fake-ip when queried through Mihomo's intercepting DNS.
const dnsLeakProbeHost = "google.com"

// handleDNSLeakTest compares DNS resolution across multiple paths to determine
// whether the router's DNS traffic is properly intercepted by Mihomo or leaking
// to ISP nameservers.
//
// Probed paths (concurrently):
//  1. Mihomo DNS port (if DNS enabled in config) — should return fake-ip when running.
//  2. System default resolver (net.DefaultResolver / /etc/resolv.conf).
//  3. Each configured upstream nameserver (direct UDP query).
//  4. Cloudflare DoH (1.1.1.1) — independent reference baseline.
//
// Leak logic:
//   - Mihomo intercepting (fake-ip on its DNS port) AND system DNS also fake-ip → OK, no leak.
//   - Mihomo intercepting AND system DNS returns real IPs → DNS bypasses Mihomo → LEAK.
//   - Mihomo NOT intercepting (not running or DNS disabled) → informational only.
//
// GET /api/v1/health/dns-leak
func handleDNSLeakTest(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()

		now := time.Now().UTC().Format(time.RFC3339)

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
			// grow slice to fit index
			for len(results) <= idx {
				results = append(results, dnsPathResult{})
			}
			results[idx] = p
		}

		pathIdx := 0

		// ── Path 1: Mihomo DNS port ────────────────────────────────────────
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

		// ── Path 2: System default resolver (/etc/resolv.conf) ────────────
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

		// ── Path 3: Configured upstream nameservers (direct UDP/DoH) ──────
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

		// ── Path 4: Cloudflare DoH — independent reference ────────────────
		{
			idx := pathIdx
			pathIdx++
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

		// Trim trailing zero-value entries that were never written.
		for len(results) > 0 && results[len(results)-1].Name == "" {
			results = results[:len(results)-1]
		}

		// ── Analyse results ────────────────────────────────────────────────
		mihomoIntercepting := false
		systemFakeIP := false
		systemError := false
		systemHasData := false

		for _, p := range results {
			switch {
			case p.Name != "" && len(p.Name) >= 6 && p.Name[:6] == "Mihomo":
				if p.IsFakeIP {
					mihomoIntercepting = true
				}
			case p.Name == "系统 DNS":
				systemHasData = true
				if p.IsFakeIP {
					systemFakeIP = true
				}
				if p.Error != "" {
					systemError = true
				}
			}
		}

		var hasLeak bool
		var summary string

		switch {
		case !deps.Config.DNS.Enable || deps.Config.Ports.DNS == 0:
			// Mihomo DNS is disabled in config — can't determine leak state.
			summary = "ClashForge 未启用 DNS 功能，无法判断 DNS 泄露状态。请在配置中启用 DNS 并重新检测。"
			hasLeak = false

		case mihomoIntercepting && systemFakeIP:
			summary = fmt.Sprintf(
				"Mihomo DNS 正在拦截 DNS 查询（返回 Fake-IP），系统 DNS 解析结果与 Mihomo 一致，未检测到泄露。",
			)
			hasLeak = false

		case mihomoIntercepting && !systemFakeIP && systemHasData && !systemError:
			summary = "Mihomo DNS 正在拦截（返回 Fake-IP），但系统 DNS 返回真实 IP，" +
				"DNS 查询可能绕过了 Mihomo 直接访问上游服务器，存在泄露风险。"
			hasLeak = true

		case mihomoIntercepting && systemError:
			summary = "Mihomo DNS 正在拦截（返回 Fake-IP），系统 DNS 查询失败，可能是 dnsmasq 配置问题，请检查 DNS 设置。"
			hasLeak = false

		case !mihomoIntercepting && systemHasData:
			if deps.Config.DNS.Enable {
				summary = "未检测到 Mihomo DNS 拦截（未返回 Fake-IP）。可能原因：Mihomo 核心未运行、DNS 端口未就绪，或 fake-ip 模式未启用。"
			} else {
				summary = "Mihomo DNS 功能未启用，当前使用系统 DNS 直接解析。"
			}
			hasLeak = false

		default:
			summary = "检测完成，请参考下方各路径结果手动判断。"
			hasLeak = false
		}

		JSON(w, http.StatusOK, dnsLeakTestResult{
			TestDomain:         dnsLeakProbeHost,
			Paths:              results,
			MihomoIntercepting: mihomoIntercepting,
			HasLeak:            hasLeak,
			Summary:            summary,
			TestedAt:           now,
		})
	}
}
