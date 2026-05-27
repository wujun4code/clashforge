package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// dnsLeakEntry is one record returned by the bash.ws DNS leak test API.
// type is "your_ip" for the detected public exit IP, or "dns" for each DNS resolver.
type dnsLeakEntry struct {
	IP          string `json:"ip"`
	Type        string `json:"type"` // "your_ip" | "dns"
	Country     string `json:"country_name"`
	CountryCode string `json:"country_code,omitempty"`
	ISP         string `json:"isp"`
}

// dnsLeakTestResult is the payload returned by GET /api/v1/health/dns-leak.
type dnsLeakTestResult struct {
	TestID   string         `json:"test_id"`
	Entries  []dnsLeakEntry `json:"entries"`
	HasLeak  bool           `json:"has_leak"`
	Summary  string         `json:"summary"`
	TestedAt string         `json:"tested_at"`
	Err      string         `json:"error,omitempty"`
}

// handleDNSLeakTest performs a DNS leak test using the bash.ws service.
//
// It works in three steps:
//  1. Generate a random 8-hex probe ID.
//  2. Fire 10 concurrent HEAD requests to unique subdomains of bash.ws
//     (e.g. "ab12cd34-1.bash.ws … ab12cd34-10.bash.ws") so that the router's
//     configured DNS resolver(s) have to look up those subdomains, making
//     themselves visible to the bash.ws authoritative DNS server.
//  3. Fetch https://bash.ws/dnsleak/test/{id}?lang=en — bash.ws returns a
//     JSON array of the public IP and every DNS resolver that made those
//     lookups, enriched with GeoIP / ISP data.
//
// GET /api/v1/health/dns-leak
func handleDNSLeakTest(_ Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
		defer cancel()

		now := time.Now().UTC().Format(time.RFC3339)

		// Step 1: generate random 8-hex probe ID.
		b := make([]byte, 4)
		if _, err := rand.Read(b); err != nil {
			JSON(w, http.StatusOK, dnsLeakTestResult{
				TestedAt: now,
				Err:      "随机数生成失败: " + err.Error(),
			})
			return
		}
		testID := hex.EncodeToString(b)

		// Step 2: trigger 10 DNS lookups for unique subdomains.
		probeClient := &http.Client{
			Timeout: 8 * time.Second,
			// Do not follow redirects — we only need the DNS lookup to happen.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}

		var wg sync.WaitGroup
		for i := 1; i <= 10; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				url := fmt.Sprintf("https://%s-%d.bash.ws", testID, n)
				req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
				if err != nil {
					return
				}
				resp, err := probeClient.Do(req)
				if err == nil {
					resp.Body.Close()
				}
			}(i)
		}
		wg.Wait()

		// Brief pause so bash.ws DNS server has time to record all resolver hits.
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
		}

		// Step 3: fetch results from bash.ws.
		resultsURL := fmt.Sprintf("https://bash.ws/dnsleak/test/%s?lang=en", testID)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, resultsURL, nil)
		if err != nil {
			JSON(w, http.StatusOK, dnsLeakTestResult{
				TestID:   testID,
				TestedAt: now,
				Err:      "无法构建请求: " + err.Error(),
			})
			return
		}

		fetchClient := &http.Client{Timeout: 10 * time.Second}
		resp, err := fetchClient.Do(req)
		if err != nil {
			JSON(w, http.StatusOK, dnsLeakTestResult{
				TestID:   testID,
				TestedAt: now,
				Err:      "无法连接检测服务 (bash.ws): " + err.Error(),
			})
			return
		}
		defer resp.Body.Close()

		var entries []dnsLeakEntry
		if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
			JSON(w, http.StatusOK, dnsLeakTestResult{
				TestID:   testID,
				TestedAt: now,
				Err:      "解析检测结果失败: " + err.Error(),
			})
			return
		}

		// Analyse results.
		// Collect distinct ISPs among DNS-type entries.  When multiple ISPs
		// appear it means some DNS queries bypassed the proxy and went directly
		// to a different resolver — a typical DNS-leak signature.
		var dnsEntries []dnsLeakEntry
		ispSet := make(map[string]bool)
		for _, e := range entries {
			if e.Type == "dns" {
				dnsEntries = append(dnsEntries, e)
				if e.ISP != "" {
					ispSet[e.ISP] = true
				}
			}
		}

		hasLeak := len(ispSet) > 1
		var summary string
		switch {
		case len(dnsEntries) == 0:
			summary = "未检测到 DNS 解析器，请确保代理已正常运行并重试"
			hasLeak = false
		case hasLeak:
			summary = fmt.Sprintf(
				"检测到 %d 个 DNS 解析器，来自 %d 家不同服务商，DNS 查询可能未完全通过代理隧道",
				len(dnsEntries), len(ispSet),
			)
		default:
			summary = fmt.Sprintf(
				"检测到 %d 个 DNS 解析器，均来自同一服务商，未发现明显泄露",
				len(dnsEntries),
			)
		}

		JSON(w, http.StatusOK, dnsLeakTestResult{
			TestID:   testID,
			Entries:  entries,
			HasLeak:  hasLeak,
			Summary:  summary,
			TestedAt: now,
		})
	}
}
