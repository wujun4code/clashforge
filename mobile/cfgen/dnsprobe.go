package cfgen

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var defaultDoHCandidates = []string{
	"https://1.1.1.1/dns-query",
	"https://8.8.8.8/dns-query",
	"https://doh.pub/dns-query",
	"https://dns.alidns.com/dns-query",
}

var defaultBootstrapNS = []string{"1.1.1.1", "8.8.8.8"}

// ProbeResult is returned by ProbeAndPatchDNS as JSON.
type ProbeResult struct {
	Summary    string `json:"summary"`
	WasPatched bool   `json:"was_patched"`
}

// ProbeAndPatchDNS probes configured UDP nameservers against sampled proxy
// hostnames.  If hijack is detected or all UDP resolvers fail, it replaces
// the dns section's nameserver/fallback/default-nameserver/proxy-server-nameserver
// fields with DoH-only entries and rewrites configPath.
// Must be called BEFORE the VPN interface is established so probe sockets
// travel the physical network, not the tunnel.
func ProbeAndPatchDNS(configPath string) ProbeResult {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return ProbeResult{Summary: "dns-probe: skip (read failed: " + err.Error() + ")"}
	}

	var cfg map[string]interface{}
	if err := yaml.Unmarshal(raw, &cfg); err != nil || cfg == nil {
		return ProbeResult{Summary: "dns-probe: skip (yaml parse failed)"}
	}

	sampleHosts := distinct(extractProxyHostnames(cfg))
	if len(sampleHosts) > 3 {
		sampleHosts = sampleHosts[:3]
	}
	if len(sampleHosts) == 0 {
		return ProbeResult{Summary: "dns-probe: skip (no proxy hostnames)"}
	}

	udpNS := distinct(filterUDPNameservers(extractDNSStringList(cfg, "nameserver")))
	if len(udpNS) == 0 {
		return ProbeResult{Summary: "dns-probe: skip (no UDP nameserver configured)"}
	}

	var hijacked, working, unresolved []string
	hostUsable := make(map[string]bool, len(sampleHosts))
	for _, h := range sampleHosts {
		hostUsable[h] = false
	}

	for _, ns := range udpNS {
		nsHijacked := false
		nsHasFailure := false
		for _, hostname := range sampleHosts {
			ips := queryUDPA(ns, hostname)
			if len(ips) == 0 {
				nsHasFailure = true
				continue
			}
			if allKnownFake(ips) {
				nsHijacked = true
				break
			}
			hostUsable[hostname] = true
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
		if !hostUsable[h] {
			unresolvedHosts = append(unresolvedHosts, h)
		}
	}
	hasUnusableHost := len(unresolvedHosts) > 0

	if len(hijacked) == 0 && !hasUnusableHost {
		return ProbeResult{
			Summary: fmt.Sprintf("dns-probe: passed (working=%s unresolved=%s)",
				strings.Join(working, ","), strings.Join(unresolved, ",")),
		}
	}

	// DNS is polluted or completely unreachable: find working DoH candidates.
	probeHost := sampleHosts[0]
	var suggestedDoH []string
	for _, doh := range defaultDoHCandidates {
		ips := queryDoHA(doh, probeHost)
		if len(ips) > 0 && !allKnownFake(ips) {
			suggestedDoH = append(suggestedDoH, doh)
		}
	}

	existingDoH := filterDoHNameservers(extractDNSStringList(cfg, "proxy-server-nameserver"))
	existingIPDoH := filterIPLiteralDoH(existingDoH)

	verifiedDoH := distinct(addSkipCertParam(suggestedDoH))
	verifiedIPDoH := filterIPLiteralDoH(verifiedDoH)

	builtInDoH := distinct(addSkipCertParam(defaultDoHCandidates))
	builtInIPDoH := filterIPLiteralDoH(builtInDoH)

	var finalDoH []string
	switch {
	case len(verifiedIPDoH) > 0:
		finalDoH = distinct(append(existingIPDoH, verifiedIPDoH...))
	case len(verifiedDoH) > 0:
		finalDoH = distinct(append(existingDoH, verifiedDoH...))
	case len(existingIPDoH) > 0:
		finalDoH = existingIPDoH
	case len(builtInIPDoH) > 0:
		finalDoH = builtInIPDoH
	default:
		finalDoH = distinct(append(existingDoH, builtInDoH...))
	}

	if len(finalDoH) == 0 {
		return ProbeResult{
			Summary: fmt.Sprintf("dns-probe: hijack detected but no DoH candidates (hijacked=%s)",
				strings.Join(hijacked, ",")),
		}
	}

	finalFallback := filterIPLiteralDoH(finalDoH)
	if len(finalFallback) == 0 {
		finalFallback = finalDoH
	}

	var finalDefaultNS []string
	for _, doh := range finalDoH {
		if h := ipLiteralHostFromDoH(doh); h != "" {
			finalDefaultNS = append(finalDefaultNS, h)
		}
	}
	for _, ns := range extractDNSStringList(cfg, "default-nameserver") {
		if h := ipLiteralFromPlainNS(ns); h != "" {
			finalDefaultNS = append(finalDefaultNS, h)
		}
	}
	for _, ns := range working {
		if h := ipLiteralFromPlainNS(ns); h != "" {
			finalDefaultNS = append(finalDefaultNS, h)
		}
	}
	finalDefaultNS = distinct(nonEmpty(append(finalDefaultNS, defaultBootstrapNS...)))

	// Patch the dns section in-place using yaml.v3 structured edit.
	dns := extractStringMap(cfg, "dns")
	dns["nameserver"] = toIface(finalDoH)
	dns["proxy-server-nameserver"] = toIface(finalDoH)
	dns["fallback"] = toIface(finalFallback)
	if len(finalDefaultNS) > 0 {
		dns["default-nameserver"] = toIface(finalDefaultNS)
	}
	cfg["dns"] = dns

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return ProbeResult{Summary: "dns-probe: marshal failed: " + err.Error()}
	}
	if err := os.WriteFile(configPath, out, 0o644); err != nil {
		return ProbeResult{Summary: "dns-probe: write failed: " + err.Error()}
	}

	return ProbeResult{
		Summary: fmt.Sprintf(
			"dns-probe: hijack detected, switched to DoH-only (hijacked=%s working=%s unresolved=%s doh=%s)",
			strings.Join(hijacked, ","), strings.Join(working, ","),
			strings.Join(unresolved, ","), strings.Join(finalDoH, ",")),
		WasPatched: true,
	}
}

// ── DNS queries ─────────────────────────────────────────────────────────────

func queryUDPA(nameserver, hostname string) []string {
	serverPart := strings.TrimPrefix(nameserver, "udp://")
	host, port := serverPart, "53"
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
		host, port = serverPart[:idx], serverPart[idx+1:]
	}

	conn, err := net.DialTimeout("udp", net.JoinHostPort(host, port), 5*time.Second)
	if err != nil {
		return nil
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	qid := uint16(rand.Intn(0x10000)) //nolint:gosec
	if _, err := conn.Write(buildDNSQuery(hostname, qid)); err != nil {
		return nil
	}
	buf := make([]byte, 1500)
	n, err := conn.Read(buf)
	if err != nil {
		return nil
	}
	return parseDNSAAnswers(buf[:n], qid)
}

func queryDoHA(dohURL, hostname string) []string {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet,
		dohURL+"?name="+url.QueryEscape(hostname)+"&type=A", nil)
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
			ips = append(ips, strings.TrimSpace(ans.Data))
		}
	}
	return ips
}

func buildDNSQuery(hostname string, qid uint16) []byte {
	out := make([]byte, 0, 12+len(hostname)+6)
	out = binary.BigEndian.AppendUint16(out, qid)
	out = append(out, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00)
	for _, label := range strings.Split(strings.Trim(hostname, "."), ".") {
		if label == "" {
			continue
		}
		out = append(out, byte(len(label)))
		out = append(out, label...)
	}
	out = append(out, 0x00, 0x00, 0x01, 0x00, 0x01)
	return out
}

func parseDNSAAnswers(msg []byte, qid uint16) []string {
	if len(msg) < 12 || binary.BigEndian.Uint16(msg[0:2]) != qid {
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
		l := int(msg[p])
		if l == 0 {
			return p + 1
		}
		if l&0xC0 == 0xC0 {
			return p + 2
		}
		p += l + 1
	}
	return p
}

// ── config scanning ──────────────────────────────────────────────────────────

func extractProxyHostnames(cfg map[string]interface{}) []string {
	proxiesRaw, _ := cfg["proxies"].([]interface{})
	var out []string
	for _, p := range proxiesRaw {
		pm, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		server, _ := pm["server"].(string)
		server = strings.TrimSpace(server)
		if server != "" && !isIPLiteralHost(server) {
			out = append(out, server)
		}
	}
	return out
}

func extractDNSStringList(cfg map[string]interface{}, key string) []string {
	dns := extractStringMap(cfg, "dns")
	raw, _ := dns[key].([]interface{})
	var out []string
	for _, v := range raw {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

// ── nameserver classification ────────────────────────────────────────────────

func filterUDPNameservers(nss []string) []string {
	var out []string
	for _, ns := range nss {
		lower := strings.ToLower(strings.TrimSpace(ns))
		if lower == "" {
			continue
		}
		if strings.HasPrefix(lower, "https://") ||
			strings.HasPrefix(lower, "tls://") ||
			strings.HasPrefix(lower, "tcp://") ||
			strings.HasPrefix(lower, "dhcp://") {
			continue
		}
		out = append(out, ns)
	}
	return out
}

func filterDoHNameservers(nss []string) []string {
	var out []string
	for _, ns := range nss {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(ns)), "https://") {
			out = append(out, ns)
		}
	}
	return out
}

func filterIPLiteralDoH(dohList []string) []string {
	var out []string
	for _, doh := range dohList {
		if ipLiteralHostFromDoH(doh) != "" {
			out = append(out, doh)
		}
	}
	return out
}

func ipLiteralHostFromDoH(doh string) string {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(doh)), "https://") {
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

func ipLiteralFromPlainNS(ns string) string {
	ns = strings.TrimSpace(ns)
	if ns == "" || strings.HasPrefix(strings.ToLower(ns), "https://") {
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
		// bare IPv6
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
	h := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(host), "["), "]")
	if h == "" {
		return false
	}
	return net.ParseIP(h) != nil
}

func addSkipCertParam(list []string) []string {
	out := make([]string, 0, len(list))
	for _, doh := range list {
		doh = strings.TrimSpace(doh)
		if doh == "" {
			continue
		}
		if strings.Contains(strings.ToLower(doh), "skip-cert-verify=") {
			out = append(out, doh)
			continue
		}
		if strings.Contains(doh, "#") {
			out = append(out, doh+"&skip-cert-verify=true")
		} else {
			out = append(out, doh+"#skip-cert-verify=true")
		}
	}
	return out
}

// ── helpers ──────────────────────────────────────────────────────────────────

func allKnownFake(ips []string) bool {
	if len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if !IsKnownFakeIP(ip) {
			return false
		}
	}
	return true
}

func extractStringMap(cfg map[string]interface{}, key string) map[string]interface{} {
	if m, ok := cfg[key].(map[string]interface{}); ok && m != nil {
		// Return a copy so callers can mutate freely.
		out := make(map[string]interface{}, len(m))
		for k, v := range m {
			out[k] = v
		}
		return out
	}
	return make(map[string]interface{})
}

func toIface(ss []string) []interface{} {
	out := make([]interface{}, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

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

func nonEmpty(in []string) []string {
	var out []string
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}
