package nodes

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
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

// TestSocks5TLSProxy probes target URLs through a no-auth SOCKS5-over-TLS endpoint
// (e.g. gost v3 deployed by QuickStart with no auther configured).
func TestSocks5TLSProxy(proxyHost string, proxyPort int, timeout time.Duration, targets []struct {
	Name string
	URL  string
}, insecureSkipVerify bool) []ProbeResult {
	results := make([]ProbeResult, 0, len(targets))
	proxyAddr := net.JoinHostPort(proxyHost, strconv.Itoa(proxyPort))
	if tc, err := net.DialTimeout("tcp", proxyAddr, timeout); err != nil {
		for _, t := range targets {
			results = append(results, ProbeResult{Name: t.Name, URL: t.URL, OK: false, Error: fmt.Sprintf("proxy port unreachable: %v", err)})
		}
		return results
	} else {
		tc.Close()
	}
	for _, t := range targets {
		start := time.Now()
		r := probeSocks5TLS(proxyHost, proxyPort, t.URL, timeout, insecureSkipVerify)
		r.Name = t.Name
		r.LatencyMS = time.Since(start).Milliseconds()
		results = append(results, r)
	}
	return results
}

func probeSocks5TLS(proxyHost string, proxyPort int, targetURL string, timeout time.Duration, insecureSkipVerify bool) ProbeResult {
	res := ProbeResult{URL: targetURL}

	u, err := url.Parse(targetURL)
	if err != nil {
		res.Error = "invalid target URL: " + err.Error()
		return res
	}
	targetHost := u.Hostname()
	targetPort := u.Port()
	if targetPort == "" {
		if u.Scheme == "https" {
			targetPort = "443"
		} else {
			targetPort = "80"
		}
	}
	portNum, _ := strconv.Atoi(targetPort)

	proxyAddr := net.JoinHostPort(proxyHost, strconv.Itoa(proxyPort))
	raw, err := net.DialTimeout("tcp", proxyAddr, timeout)
	if err != nil {
		res.Error = "connect: " + err.Error()
		return res
	}
	defer raw.Close()
	_ = raw.SetDeadline(time.Now().Add(timeout))

	tlsConn := tls.Client(raw, &tls.Config{
		ServerName:         proxyHost,
		InsecureSkipVerify: insecureSkipVerify, //nolint:gosec
	})
	if err := tlsConn.Handshake(); err != nil {
		res.Error = "TLS handshake: " + err.Error()
		return res
	}

	// SOCKS5 greeting: ver=5, nmethods=1, method=0x00 (no auth)
	if _, err := tlsConn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		res.Error = "socks5 greeting: " + err.Error()
		return res
	}
	choice := make([]byte, 2)
	if _, err := io.ReadFull(tlsConn, choice); err != nil {
		res.Error = "socks5 method select: " + err.Error()
		return res
	}
	if choice[0] != 0x05 || choice[1] != 0x00 {
		res.Error = fmt.Sprintf("socks5: server chose method 0x%02x (expected no-auth 0x00)", choice[1])
		return res
	}

	// SOCKS5 CONNECT: ver=5, cmd=1(CONNECT), rsv=0, atyp=3(domain), host, port
	hostBytes := []byte(targetHost)
	pkt := []byte{0x05, 0x01, 0x00, 0x03, byte(len(hostBytes))}
	pkt = append(pkt, hostBytes...)
	pkt = append(pkt, byte(portNum>>8), byte(portNum&0xff))
	if _, err := tlsConn.Write(pkt); err != nil {
		res.Error = "socks5 connect request: " + err.Error()
		return res
	}

	// Read SOCKS5 reply header (4 bytes)
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(tlsConn, hdr); err != nil {
		res.Error = "socks5 reply: " + err.Error()
		return res
	}
	if hdr[1] != 0x00 {
		codes := map[byte]string{1: "general failure", 2: "not allowed", 3: "network unreachable", 4: "host unreachable", 5: "connection refused"}
		msg := codes[hdr[1]]
		if msg == "" {
			msg = fmt.Sprintf("code %d", hdr[1])
		}
		res.Error = "socks5 connect rejected: " + msg
		return res
	}
	// Skip bound address
	switch hdr[3] {
	case 0x01:
		skip := make([]byte, 4+2)
		_, _ = io.ReadFull(tlsConn, skip)
	case 0x03:
		lb := make([]byte, 1)
		_, _ = io.ReadFull(tlsConn, lb)
		skip := make([]byte, int(lb[0])+2)
		_, _ = io.ReadFull(tlsConn, skip)
	case 0x04:
		skip := make([]byte, 16+2)
		_, _ = io.ReadFull(tlsConn, skip)
	}

	// For HTTPS targets, wrap the tunnel with an inner TLS handshake.
	var tunnelRW io.ReadWriter = tlsConn
	if u.Scheme == "https" {
		innerTLS := tls.Client(tlsConn, &tls.Config{
			ServerName:         targetHost,
			InsecureSkipVerify: false, //nolint:gosec — inner cert is the target's real cert
		})
		if err := innerTLS.Handshake(); err != nil {
			res.Error = "inner TLS handshake to target: " + err.Error()
			return res
		}
		tunnelRW = innerTLS
	}

	reqLine := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\nUser-Agent: clashforge-probe/1.0\r\n\r\n", u.RequestURI(), targetHost)
	if _, err := io.WriteString(tunnelRW, reqLine); err != nil {
		res.Error = "http request: " + err.Error()
		return res
	}

	// Read status line only
	br := bufio.NewReader(tunnelRW)
	statusLine, err := br.ReadString('\n')
	if err != nil && statusLine == "" {
		res.Error = "http response: " + err.Error()
		return res
	}
	parts := strings.Fields(strings.TrimSpace(statusLine))
	if len(parts) < 2 {
		res.Error = fmt.Sprintf("unexpected HTTP response: %q", statusLine)
		return res
	}
	code, err := strconv.Atoi(parts[1])
	if err != nil {
		res.Error = fmt.Sprintf("bad status code %q", parts[1])
		return res
	}
	res.StatusCode = code
	res.OK = code >= 200 && code < 400
	if !res.OK {
		res.Error = fmt.Sprintf("unexpected status: %d", code)
	}
	return res
}
