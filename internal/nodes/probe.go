package nodes

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// ProbeResult is a single proxy connectivity check result.
type ProbeResult struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	OK         bool   `json:"ok"`
	StatusCode int    `json:"status_code,omitempty"`
	LatencyMS  int64  `json:"latency_ms,omitempty"`
	Error      string `json:"error,omitempty"`
}

func DefaultProbeTargets() []struct {
	Name string
	URL  string
} {
	return []struct {
		Name string
		URL  string
	}{
		{Name: "Google", URL: "https://www.google.com"},
		{Name: "YouTube", URL: "https://www.youtube.com"},
		{Name: "GitHub", URL: "https://github.com"},
	}
}

// TestHTTPProxy probes the target URLs through a HTTP proxy endpoint.
func TestHTTPProxy(proxyHost string, proxyPort int, username, password string, timeout time.Duration, targets []struct {
	Name string
	URL  string
}) []ProbeResult {
	results := make([]ProbeResult, 0, len(targets))
	if proxyHost == "" || proxyPort <= 0 {
		for _, t := range targets {
			results = append(results, ProbeResult{Name: t.Name, URL: t.URL, OK: false, Error: "invalid proxy host/port"})
		}
		return results
	}

	proxyAddr := net.JoinHostPort(proxyHost, fmt.Sprintf("%d", proxyPort))
	if conn, err := net.DialTimeout("tcp", proxyAddr, timeout); err != nil {
		for _, t := range targets {
			results = append(results, ProbeResult{Name: t.Name, URL: t.URL, OK: false, Error: fmt.Sprintf("proxy port unreachable: %v", err)})
		}
		return results
	} else {
		_ = conn.Close()
	}

	proxyURL := &url.URL{
		Scheme: "http",
		Host:   proxyAddr,
	}
	if username != "" {
		proxyURL.User = url.UserPassword(username, password)
	}

	transport := &http.Transport{
		Proxy:               http.ProxyURL(proxyURL),
		TLSHandshakeTimeout: timeout,
	}
	client := &http.Client{Timeout: timeout, Transport: transport}

	for _, t := range targets {
		start := time.Now()
		res := ProbeResult{Name: t.Name, URL: t.URL}

		req, err := http.NewRequest(http.MethodGet, t.URL, nil)
		if err != nil {
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		req.Header.Set("User-Agent", "clashforge-node-probe/1.0")

		resp, err := client.Do(req)
		res.LatencyMS = time.Since(start).Milliseconds()
		if err != nil {
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		res.StatusCode = resp.StatusCode
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		res.OK = resp.StatusCode >= 200 && resp.StatusCode < 400
		if !res.OK {
			res.Error = fmt.Sprintf("unexpected status: %d", resp.StatusCode)
		}
		results = append(results, res)
	}

	return results
}
