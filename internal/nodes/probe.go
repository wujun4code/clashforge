package nodes

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wujun4code/clashforge/internal/probetargets"
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
	targets := probetargets.NodeConnectivityTargets()
	out := make([]struct {
		Name string
		URL  string
	}, 0, len(targets))
	for _, target := range targets {
		out = append(out, struct {
			Name string
			URL  string
		}{
			Name: target.Name,
			URL:  target.URL,
		})
	}
	return out
}

type ProxyProbeOptions struct {
	ProxyScheme        string
	InsecureSkipVerify bool
}

// TestHTTPProxy probes the target URLs through a HTTP proxy endpoint.
func TestHTTPProxy(proxyHost string, proxyPort int, username, password string, timeout time.Duration, targets []struct {
	Name string
	URL  string
}) []ProbeResult {
	return TestHTTPProxyWithOptions(proxyHost, proxyPort, username, password, timeout, targets, ProxyProbeOptions{ProxyScheme: "http"})
}

// TestHTTPProxyWithOptions probes target URLs through proxy endpoint with explicit proxy transport settings.
func TestHTTPProxyWithOptions(proxyHost string, proxyPort int, username, password string, timeout time.Duration, targets []struct {
	Name string
	URL  string
}, options ProxyProbeOptions) []ProbeResult {
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

	scheme := strings.ToLower(strings.TrimSpace(options.ProxyScheme))
	if scheme == "" {
		scheme = "http"
	}
	if scheme != "http" && scheme != "https" {
		scheme = "http"
	}

	proxyURL := &url.URL{
		Scheme: scheme,
		Host:   proxyAddr,
	}
	if username != "" {
		proxyURL.User = url.UserPassword(username, password)
	}

	transport := &http.Transport{
		Proxy:               http.ProxyURL(proxyURL),
		TLSHandshakeTimeout: timeout,
	}
	if options.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true, // only for probe diagnostics; required when probing TLS proxy by IP
		}
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
