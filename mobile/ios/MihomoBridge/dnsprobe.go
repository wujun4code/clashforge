package mihomobridge

// Pre-flight upstream-DNS hijack probe, ported from
// ClashVpnService.patchConfigForUpstreamFakeIP.  Must run BEFORE the tunnel
// is up (the Swift side calls it before setTunnelNetworkSettings) so the
// probe sockets travel the physical network and don't produce false
// negatives through the tunnel.
//
// If every sampled proxy hostname resolves to a known fake-ip range
// (198.18.0.0/15 / 28.0.0.0/8 — GFW-style poisoning) or UDP DNS is unusable,
// the config's resolver set is rewritten to verified DoH endpoints, matching
// the OpenWrt startup self-healing behaviour.

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

var defaultDoHProbeCandidates = []string{
	"https://1.1.1.1/dns-query",
	"https://8.8.8.8/dns-query",
	"https://doh.pub/dns-query",
	"https://dns.alidns.com/dns-query",
}

var defaultBootstrapNameservers = []string{"1.1.1.1", "8.8.8.8"}

var ipv4Pattern = regexp.MustCompile(`^\d{1,3}(\.\d{1,3}){3}$`)

// ProbeAndPatchDNS probes the configured UDP nameservers against sampled
// proxy hostnames and, on detected fake-ip hijack or total UDP failure,
// rewrites the DNS section to a DoH-only resolver set.  Returns a summary
// line for the extension's log stream; never fails the connection attempt.
func ProbeAndPatchDNS(configPath string) string {
	raw, err := readFileString(configPath)
	if err != nil {
		return "dns-probe: skip (read config failed: " + err.Error() + ")"
	}
	original := raw

	sampleHosts := distinct(extractProxyHostnames(original))
	if len(sampleHosts) > 3 {
		sampleHosts = sampleHosts[:3]
	}
	if len(sampleHosts) == 0 {
		return "dns-probe: skip (no proxy hostnames)"
	}

	var udpNameservers []string
	for _, ns := range extractDNSList(original, "nameserver") {
		ns = strings.TrimSpace(ns)
		if isUDPNameserver(ns) {
			udpNameservers = append(udpNameservers, ns)
		}
	}
	udpNameservers = distinct(udpNameservers)
	if len(udpNameservers) == 0 {
		return "dns-probe: skip (no UDP nameserver configured)"
	}

	hijacked := []string{}
	working := []string{}
	unresolved := []string{}
	hostHasUsableAnswer := map[string]bool{}
	for _, h := range sampleHosts {
		hostHasUsableAnswer[h] = false
	}

	for _, ns := range udpNameservers {
		nsHijacked := false
		nsHasFailure := false
		for _, hostname := range sampleHosts {
			ips := queryUDPA(ns, hostname)
			if len(ips) == 0 {
				nsHasFailure = true
				continue
			}
			if allInKnownFakeRanges(ips) {
				nsHijacked = true
				break
			}
			hostHasUsableAnswer[hostname] = true
		}
		switch {
		case nsHijacked:
			hijacked = append(hijacked, ns)
		case nsHasFailure:
			unresolved = append(unresolved, ns)
		default:
			working = append(working, ns)
		}
	}

	var unresolvedHosts []string
	for _, h := range sampleHosts {
		if !hostHasUsableAnswer[h] {
			unresolvedHosts = append(unresolvedHosts, h)
		}
	}
	hasUnusableHost := len(unresolvedHosts) > 0

	if len(hijacked) == 0 && !hasUnusableHost {
		return fmt.Sprintf("dns-probe: passed (working=%s unresolved=%s)",
			strings.Join(working, ","), strings.Join(unresolved, ","))
	}

	probeHost := sampleHosts[0]
	var suggestedDoH []string
	for _, doh := range defaultDoHProbeCandidates {
		ips := queryDoHA(doh, probeHost)
		if len(ips) > 0 && !allInKnownFakeRanges(ips) {
			suggestedDoH = append(suggestedDoH, doh)
		}
	}

	var existingDoh []string
	for _, ns := range extractDNSList(original, "proxy-server-nameserver") {
		ns = strings.TrimSpace(ns)
		if isDoHNameserver(ns) {
			existingDoh = append(existingDoh, withSkipCertVerifyParam(ns))
		}
	}
	existingDoh = distinct(existingDoh)
	existingIPLiteralDoh := filterStrings(existingDoh, isIPLiteralDoH)

	var verifiedDoH []string
	for _, doh := range suggestedDoH {
		v := strings.TrimSpace(withSkipCertVerifyParam(doh))
		if v != "" {
			verifiedDoH = append(verifiedDoH, v)
		}
	}
	verifiedDoH = distinct(verifiedDoH)
	verifiedIPLiteralDoh := filterStrings(verifiedDoH, isIPLiteralDoH)

	var builtInDoH []string
	for _, doh := range defaultDoHProbeCandidates {
		builtInDoH = append(builtInDoH, withSkipCertVerifyParam(doh))
	}
	builtInDoH = distinct(builtInDoH)
	builtInIPLiteralDoh := filterStrings(builtInDoH, isIPLiteralDoH)

	var finalDoH []string
	switch {
	case len(verifiedIPLiteralDoh) > 0:
		finalDoH = distinct(append(append([]string{}, existingIPLiteralDoh...), verifiedIPLiteralDoh...))
	case len(verifiedDoH) > 0:
		finalDoH = distinct(append(append([]string{}, existingDoh...), verifiedDoH...))
	case len(existingIPLiteralDoh) > 0:
		finalDoH = existingIPLiteralDoh
	case len(builtInIPLiteralDoh) > 0:
		finalDoH = builtInIPLiteralDoh
	default:
		finalDoH = distinct(append(append([]string{}, existingDoh...), builtInDoH...))
	}

	if len(finalDoH) == 0 {
		return fmt.Sprintf("dns-probe: hijack detected but no DoH candidates (hijacked=%s)",
			strings.Join(hijacked, ","))
	}

	finalNameserver := finalDoH
	finalProxyServerNS := finalDoH
	finalFallback := filterStrings(finalDoH, isIPLiteralDoH)
	if len(finalFallback) == 0 {
		finalFallback = finalDoH
	}

	var finalDefaultNS []string
	for _, doh := range finalDoH {
		if h := extractIPLiteralHostFromDoH(doh); h != "" {
			finalDefaultNS = append(finalDefaultNS, h)
		}
	}
	for _, ns := range extractDNSList(original, "default-nameserver") {
		if h := extractIPLiteralFromPlainNameserver(ns); h != "" {
			finalDefaultNS = append(finalDefaultNS, h)
		}
	}
	for _, ns := range working {
		if h := extractIPLiteralFromPlainNameserver(ns); h != "" {
			finalDefaultNS = append(finalDefaultNS, h)
		}
	}
	finalDefaultNS = append(finalDefaultNS, defaultBootstrapNameservers...)
	finalDefaultNS = distinct(trimNonEmpty(finalDefaultNS))

	patched := upsertDNSList(original, "nameserver", finalNameserver)
	patched = upsertDNSList(patched, "proxy-server-nameserver", finalProxyServerNS)
	patched = upsertDNSList(patched, "fallback", finalFallback)
	if len(finalDefaultNS) > 0 {
		patched = upsertDNSList(patched, "default-nameserver", finalDefaultNS)
	}

	if patched == original {
		return fmt.Sprintf("dns-probe: hijack detected but patch was a no-op (hijacked=%s)",
			strings.Join(hijacked, ","))
	}
	if err := writeFileString(configPath, patched); err != nil {
		return "dns-probe: patch write failed: " + err.Error()
	}
	return fmt.Sprintf(
		"dns-probe: hijack detected, switched to DoH-only (hijacked=%s working=%s unresolved=%s doh=%s)",
		strings.Join(hijacked, ","), strings.Join(working, ","),
		strings.Join(unresolved, ","), strings.Join(finalDoH, ","))
}

// ── config scanning ────────────────────────────────────────────────────────

func extractProxyHostnames(config string) []string {
	var out []string
	inProxies := false
	for _, line := range normalizeLines(config) {
		trimmed := strings.TrimSpace(line)
		topLevel := line != "" && !strings.HasPrefix(line, " ")
		if topLevel {
			inProxies = trimmed == "proxies:"
			continue
		}
		if !inProxies || trimmed == "" || !strings.HasPrefix(trimmed, "server:") {
			continue
		}
		value := parseYAMLScalar(trimmed[len("server:"):])
		if isLikelyHostname(value) {
			out = append(out, value)
		}
	}
	return out
}

func extractDNSList(config, key string) []string {
	lines := normalizeLines(config)
	dnsStart, dnsEnd := dnsBlockRange(lines)
	if dnsStart < 0 {
		return nil
	}

	keyLine := -1
	for i := dnsStart + 1; i < dnsEnd; i++ {
		if strings.TrimSpace(lines[i]) == key+":" {
			keyLine = i
			break
		}
	}
	if keyLine < 0 {
		return nil
	}

	var out []string
	for i := keyLine + 1; i < dnsEnd; i++ {
		ln := lines[i]
		trimmed := strings.TrimSpace(ln)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(ln, "    ") {
			break
		}
		if !strings.HasPrefix(trimmed, "-") {
			continue
		}
		value := parseYAMLScalar(strings.TrimSpace(strings.TrimPrefix(trimmed, "-")))
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

// ── nameserver classification ──────────────────────────────────────────────

func isDoHNameserver(ns string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(ns)), "https://")
}

func isUDPNameserver(ns string) bool {
	if strings.TrimSpace(ns) == "" {
		return false
	}
	lower := strings.ToLower(ns)
	return !strings.HasPrefix(lower, "https://") &&
		!strings.HasPrefix(lower, "tls://") &&
		!strings.HasPrefix(lower, "tcp://") &&
		!strings.HasPrefix(lower, "dhcp://")
}

func isIPLiteralDoH(ns string) bool {
	return extractIPLiteralHostFromDoH(ns) != ""
}

func extractIPLiteralHostFromDoH(doh string) string {
	if !isDoHNameserver(doh) {
		return ""
	}
	u, err := url.Parse(strings.SplitN(doh, "#", 2)[0])
	if err != nil {
		return ""
	}
	host := u.Hostname()
	if isIPLiteralHost(host) {
		return host
	}
	return ""
}

func extractIPLiteralFromPlainNameserver(nameserver string) string {
	ns := strings.TrimSpace(nameserver)
	if ns == "" || isDoHNameserver(ns) {
		return ""
	}
	lower := strings.ToLower(ns)
	if strings.HasPrefix(lower, "dhcp://") {
		return ""
	}
	for _, scheme := range []string{"udp://", "tcp://", "tls://"} {
		if strings.HasPrefix(lower, scheme) {
			ns = ns[len(scheme):]
			break
		}
	}

	host := ns
	switch {
	case strings.HasPrefix(ns, "["):
		if end := strings.Index(ns, "]"); end > 0 {
			host = ns[1:end]
		}
	case strings.Count(ns, ":") > 1:
		host = ns
	case strings.Contains(ns, ":"):
		host = ns[:strings.LastIndex(ns, ":")]
	}
	host = strings.TrimSpace(host)
	if isIPLiteralHost(host) {
		return host
	}
	return ""
}

func isIPLiteralHost(host string) bool {
	normalized := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(host), "["), "]")
	if normalized == "" {
		return false
	}
	return ipv4Pattern.MatchString(normalized) || strings.Contains(normalized, ":")
}

func withSkipCertVerifyParam(doh string) string {
	raw := strings.TrimSpace(doh)
	if raw == "" {
		return raw
	}
	if strings.Contains(strings.ToLower(raw), "skip-cert-verify=") {
		return raw
	}
	if strings.Contains(raw, "#") {
		return raw + "&skip-cert-verify=true"
	}
	return raw + "#skip-cert-verify=true"
}

func isLikelyHostname(host string) bool {
	if strings.TrimSpace(host) == "" || strings.HasPrefix(host, "[") {
		return false
	}
	return !ipv4Pattern.MatchString(host)
}

// ── DNS queries ────────────────────────────────────────────────────────────

func queryUDPA(nameserver, hostname string) []string {
	serverPart := strings.TrimPrefix(nameserver, "udp://")
	host := serverPart
	port := "53"
	switch {
	case strings.HasPrefix(serverPart, "["):
		if end := strings.Index(serverPart, "]"); end > 0 {
			host = serverPart[1:end]
			if end+1 < len(serverPart) && serverPart[end+1] == ':' {
				port = serverPart[end+2:]
			}
		}
	case strings.Count(serverPart, ":") > 1:
		host = serverPart
	case strings.Contains(serverPart, ":"):
		idx := strings.LastIndex(serverPart, ":")
		host = serverPart[:idx]
		port = serverPart[idx+1:]
	}

	conn, err := net.DialTimeout("udp", net.JoinHostPort(host, port), 5*time.Second)
	if err != nil {
		return nil
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	queryID := uint16(rand.Intn(0x10000))
	if _, err := conn.Write(buildDNSQuery(hostname, queryID)); err != nil {
		return nil
	}
	buf := make([]byte, 1500)
	n, err := conn.Read(buf)
	if err != nil {
		return nil
	}
	return parseDNSAAnswers(buf[:n], queryID)
}

func queryDoHA(dohURL, hostname string) []string {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", dohURL+"?name="+url.QueryEscape(hostname)+"&type=A", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Accept", "application/dns-json")
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var parsed struct {
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil
	}
	var ips []string
	for _, ans := range parsed.Answer {
		if ans.Type == 1 && strings.TrimSpace(ans.Data) != "" {
			ips = append(ips, ans.Data)
		}
	}
	return ips
}

func buildDNSQuery(hostname string, queryID uint16) []byte {
	out := make([]byte, 0, 12+len(hostname)+6)
	out = binary.BigEndian.AppendUint16(out, queryID)
	out = append(out,
		0x01, 0x00, // RD=1 standard query
		0x00, 0x01, // QDCOUNT
		0x00, 0x00, // ANCOUNT
		0x00, 0x00, // NSCOUNT
		0x00, 0x00, // ARCOUNT
	)
	for _, label := range strings.Split(strings.Trim(hostname, "."), ".") {
		if label == "" {
			continue
		}
		out = append(out, byte(len(label)))
		out = append(out, label...)
	}
	out = append(out, 0x00)             // QNAME terminator
	out = append(out, 0x00, 0x01)       // QTYPE A
	return append(out, 0x00, 0x01)      // QCLASS IN
}

func parseDNSAAnswers(msg []byte, queryID uint16) []string {
	if len(msg) < 12 {
		return nil
	}
	if binary.BigEndian.Uint16(msg[0:2]) != queryID {
		return nil
	}
	qdCount := int(binary.BigEndian.Uint16(msg[4:6]))
	anCount := int(binary.BigEndian.Uint16(msg[6:8]))

	offset := 12
	for i := 0; i < qdCount; i++ {
		offset = skipDNSName(msg, offset)
		if offset+4 > len(msg) {
			return nil
		}
		offset += 4
	}

	var ips []string
	for i := 0; i < anCount; i++ {
		offset = skipDNSName(msg, offset)
		if offset+10 > len(msg) {
			break
		}
		rType := binary.BigEndian.Uint16(msg[offset : offset+2])
		rClass := binary.BigEndian.Uint16(msg[offset+2 : offset+4])
		rdLen := int(binary.BigEndian.Uint16(msg[offset+8 : offset+10]))
		offset += 10
		if offset+rdLen > len(msg) {
			break
		}
		if rType == 1 && rClass == 1 && rdLen == 4 {
			ips = append(ips, net.IP(msg[offset:offset+4]).String())
		}
		offset += rdLen
	}
	return ips
}

func skipDNSName(msg []byte, start int) int {
	p := start
	for p < len(msg) {
		length := int(msg[p])
		if length == 0 {
			return p + 1
		}
		if length&0xC0 == 0xC0 {
			return p + 2
		}
		p += length + 1
	}
	return p
}

func allInKnownFakeRanges(ips []string) bool {
	if len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if !isKnownFakeIP(ip) {
			return false
		}
	}
	return true
}

func isKnownFakeIP(ip string) bool {
	addr := net.ParseIP(ip)
	if addr == nil {
		return false
	}
	v4 := addr.To4()
	if v4 == nil {
		return false
	}
	// 198.18.0.0/15 and 28.0.0.0/8
	return (v4[0] == 198 && (v4[1] == 18 || v4[1] == 19)) || v4[0] == 28
}

// ── small helpers ──────────────────────────────────────────────────────────

func distinct(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func filterStrings(in []string, keep func(string) bool) []string {
	var out []string
	for _, v := range in {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}

func trimNonEmpty(in []string) []string {
	var out []string
	for _, v := range in {
		if t := strings.TrimSpace(v); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func readFileString(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func writeFileString(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
