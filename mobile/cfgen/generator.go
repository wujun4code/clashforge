package cfgen

import (
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// GenerateResult is returned by GenerateConfig as JSON.
type GenerateResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

// GenerateConfig reads the subscription YAML at configPath (which may already
// have been patched by ProbeAndPatchDNS), applies the mobile-specific DNS/TUN/
// sniffer settings for Android, and writes the final config back to configPath.
//
// tunFd is the file-descriptor number mihomo should use for the TUN device.
// On Android we always pass 0 here: the parent process dup2s the real TUN fd
// onto fd 0 (stdin) before forking mihomo, so file-descriptor: 0 is correct.
//
// geoDataDir is the directory containing geosite.dat / country.mmdb.
//
// dnsMode controls dns.enhanced-mode: "fake-ip" (default) or "redir-host".
// On single-device TUN both modes work; fake-ip is more robust against DNS
// pollution.  redir-host gives better compatibility with NTP/STUN/games.
// powerProfile controls optional work that affects mobile CPU/radio usage.
func GenerateConfig(configPath string, tunFd int, geoDataDir string, dnsMode string, powerProfile string) GenerateResult {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return GenerateResult{Error: "read config: " + err.Error()}
	}

	var cfg map[string]interface{}
	if err := yaml.Unmarshal(raw, &cfg); err != nil || cfg == nil {
		// Subscription YAML unreadable — start with an empty map; proxies/rules
		// will be absent but DNS/TUN will still be correct.
		cfg = make(map[string]interface{})
	}

	if dnsMode == "" {
		dnsMode = "fake-ip"
	}
	powerProfile = normalizePowerProfile(powerProfile)

	// Rebuild DNS section, preserving probe-patched nameservers.
	cfg["dns"] = buildDNSSection(cfg, geoDataDir, dnsMode)

	// Remove existing tun and sniffer; yaml.v3 structural delete handles any
	// inline comments, indentation style, or duplicate keys cleanly.
	delete(cfg, "tun")
	delete(cfg, "sniffer")

	// Append canonical Android TUN and sniffer blocks. fake-ip already carries
	// domain context, so battery mode can avoid parsing every TLS/QUIC flow.
	cfg["tun"] = buildAndroidTUN(tunFd)
	cfg["sniffer"] = buildSniffer(powerProfile != "battery" || dnsMode == "redir-host")

	// Runtime-owned fields that must not come from the subscription config.
	cfg["external-controller"] = "127.0.0.1:9090"
	cfg["allow-lan"] = false
	cfg["mode"] = "rule"
	cfg["geodata-mode"] = false
	cfg["geodata-path"] = geoDataDir
	cfg["geodata-loader"] = "memconservative"
	cfg["log-level"] = "warning"
	cfg["unified-delay"] = powerProfile == "performance"
	cfg["tcp-concurrent"] = powerProfile != "battery"
	delete(cfg, "secret") // keep API endpoint reachable without auth

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return GenerateResult{Error: "marshal: " + err.Error()}
	}
	if err := os.WriteFile(configPath, out, 0o644); err != nil {
		return GenerateResult{Error: "write: " + err.Error()}
	}
	return GenerateResult{OK: true}
}

// buildDNSSection builds the final dns map. It reads the existing dns section
// (which ProbeAndPatchDNS may have already updated) and overlays the
// Android-mandatory settings: enhanced-mode, filters, and respect-rules.
//
// dnsMode: "fake-ip" (default) — returns 198.18.x.x synthetic IPs, most robust
//
//	"redir-host"        — returns real upstream IPs, better NTP/STUN compat
func buildDNSSection(cfg map[string]interface{}, geoDataDir string, dnsMode string) map[string]interface{} {
	existing := extractStringMap(cfg, "dns")

	dns := make(map[string]interface{})

	// Preserve probe-patched nameservers; fall back to subscription defaults.
	for _, key := range []string{
		"nameserver", "fallback", "default-nameserver", "proxy-server-nameserver",
	} {
		if v, ok := existing[key]; ok {
			dns[key] = v
		}
	}

	// Add fallback-filter when fallback is present (useful in both modes).
	if _, ok := dns["fallback"]; ok {
		dns["fallback-filter"] = map[string]interface{}{
			"geoip":      true,
			"geoip-code": "CN",
			"ipcidr":     []interface{}{"240.0.0.0/4", "0.0.0.0/8"},
		}
	}

	// Android mandatory overrides.
	dns["enable"] = true
	dns["listen"] = "0.0.0.0:1053"
	// respect-rules: true — strongly indicated by live debug logging as the
	// fix for Google Play downloads hanging indefinitely (a redirector
	// hostname like rr2---sn-xxxx.googlevideo.com resolved through the
	// plain-UDP domestic nameservers to a *poisoned* domestic IP, e.g.
	// 180.163.150.131 for a Google CDN host — mainland ISP/GFW-style DNS
	// injection targeting this class of hostname). With this false, that
	// resolution happened directly over the phone's real network path,
	// subject to that injection. With it true, when the hostname being
	// resolved matches a proxy rule, mihomo's own DNS dial for it is routed
	// through that same proxy — the query leaves from the proxy's location
	// (unaffected by domestic injection) instead of the phone's. The
	// previous rationale here ("safe because addDisallowedApplication
	// excludes us from the tunnel") was true but answered a different
	// question (why this doesn't self-capture/loop) — it didn't actually
	// justify false over true for poisoning resistance.
	dns["respect-rules"] = true

	if dnsMode == "redir-host" {
		dns["enhanced-mode"] = "redir-host"
		// In redir-host mode mihomo must resolve real upstream IPs.
		// The subscription's fallback (1.1.1.1/8.8.8.8 DoH) is often
		// unreachable from mainland China; replace it with China-accessible
		// DoH endpoints so foreign domains resolve without timeout.
		dns["fallback"] = []interface{}{
			"https://doh.pub/dns-query",
			"https://dns.alidns.com/dns-query",
		}
		dns["fallback-filter"] = map[string]interface{}{
			"geoip":      true,
			"geoip-code": "CN",
			"ipcidr":     []interface{}{"240.0.0.0/4", "0.0.0.0/8"},
		}
		// fake-ip-* keys have no meaning in redir-host mode; omit them so
		// mihomo doesn't allocate the fake-ip pool unnecessarily.
		delete(dns, "fake-ip-range")
		delete(dns, "fake-ip-filter")
		delete(dns, "fake-ip-filter-mode")
	} else {
		dns["enhanced-mode"] = "fake-ip"
		// Not 198.18.0.0/15: Loyalsoldier's lancidr.txt (used by
		// LoyalsoldierTemplate/LiteTemplate rule-providers) bundles that exact
		// IANA-reserved block as a "route DIRECT" range. Any fake-ip literal
		// for a domain not covered by a more specific rule-provider falls
		// through to lancidr and gets force-routed DIRECT instead of through
		// the proxy — mirrors the router-side fix in
		// internal/config/fakeip.go (SelectFakeIPRange). 28.18.0.1/16 is a
		// real, allocated-but-obscure block (part of 28.0.0.0/8, long
		// assigned to the US DoD network) that lancidr does not bundle.
		//
		// (Was briefly reverted to 198.18.0.1/16 to diagnose a Google Play
		// download hang; confirmed unrelated — the actual cause was a TUN
		// MTU mismatch, see the "mtu" field in buildAndroidTUN below.)
		dns["fake-ip-range"] = "28.18.0.1/16"
		dns["fake-ip-filter-mode"] = "blacklist"
		dns["fake-ip-filter"] = fakeIPFilter(geoDataDir)
	}

	return dns
}

func buildAndroidTUN(tunFd int) map[string]interface{} {
	return map[string]interface{}{
		"enable": true,
		// Tried "system" (kernel TCP via local NAT listener) to test whether
		// a gvisor bug explained Google Play downloads stalling at an
		// identical byte count right after the TLS handshake. Reverted:
		// mihomo's system-stack TUN reader does blocking read() with no
		// EAGAIN/readiness handling, but ClashVpnService.kt's
		// Builder.setBlocking(false) hands it a non-blocking fd — the result
		// is a tight busy-loop logging "read packet: read tun: resource
		// temporarily unavailable" with no packets ever processed, not a
		// partial improvement. gvisor is epoll-driven and handles the
		// non-blocking fd correctly, which is presumably why it was chosen
		// originally.
		"stack":                 "gvisor",
		"file-descriptor":       tunFd,
		"auto-route":            false,
		"auto-detect-interface": false,
		"dns-hijack":            defaultDNSHijack,
		// Must match ClashVpnService's VpnService.Builder.setMtu(1500)
		// exactly. When omitted, mihomo's gvisor TUN stack defaults to 9000
		// (listener/sing_tun/server.go: "if tunMTU == 0 { tunMTU = 9000 }"),
		// while the real Android TUN interface backing this fd only accepts
		// 1500-byte packets. Small requests never approach either limit, so
		// they work fine; any transfer that grows past 1500 bytes has gvisor
		// negotiate/produce packets the real interface can't carry, and they
		// get silently dropped — the exact symptom that made large CDN
		// downloads (Google Play app installs, video CDN edges) stall
		// indefinitely a few KB in while ordinary API calls kept working.
		"mtu": 1500,
	}
}

func buildSniffer(enabled bool) map[string]interface{} {
	if !enabled {
		return map[string]interface{}{"enable": false}
	}
	return map[string]interface{}{
		"enable": true,
		// Global override-destination must be false (mihomo's own official
		// default — we previously diverged from it and paid for it once
		// already, see docs/clashforge-wechat-video-bug-report.md). With it
		// true, mihomo re-resolves every sniffed TLS/QUIC hostname via its
		// own DNS and redials that fresh IP instead of preserving the
		// client's original connection — which breaks any session/token-
		// bound CDN redirector (WeChat's video CDN in that report; Google
		// Play's "rr---sn-*.googlevideo.com"-style download redirector here)
		// where the destination IP the client resolved is only valid for
		// that specific request. The symptom is identical both times: the
		// TLS handshake to the re-resolved IP succeeds, but the actual
		// payload transfer fails because that edge wasn't the one the
		// session was authorized for, so the client retries indefinitely
		// against a fresh redirect target.
		"override-destination": false,
		"parse-pure-ip":        true,
		"sniff": map[string]interface{}{
			"TLS": map[string]interface{}{
				"ports": []interface{}{443, 8443},
			},
			"HTTP": map[string]interface{}{
				"ports": []interface{}{"80", "8080-8880"},
				// Plain HTTP has no SNI; override-destination here is what
				// makes Host-header-based transparent proxying work at all,
				// and plain HTTP has no CDN-session-token concept to break.
				"override-destination": true,
			},
			"QUIC": map[string]interface{}{
				"ports": []interface{}{443},
			},
		},
	}
}

func normalizePowerProfile(value string) string {
	switch value {
	case "battery", "performance":
		return value
	default:
		return "balanced"
	}
}

// ── JSON helpers for JNI boundary ────────────────────────────────────────────

func MarshalProbeResult(r ProbeResult) string {
	b, _ := json.Marshal(r)
	return string(b)
}

func MarshalGenerateResult(r GenerateResult) string {
	b, err := json.Marshal(r)
	if err != nil {
		return fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error())
	}
	return string(b)
}
