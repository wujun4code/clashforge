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
func GenerateConfig(configPath string, tunFd int, geoDataDir string, dnsMode string) GenerateResult {
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

	// Rebuild DNS section, preserving probe-patched nameservers.
	cfg["dns"] = buildDNSSection(cfg, geoDataDir, dnsMode)

	// Remove existing tun and sniffer; yaml.v3 structural delete handles any
	// inline comments, indentation style, or duplicate keys cleanly.
	delete(cfg, "tun")
	delete(cfg, "sniffer")

	// Append canonical Android TUN and sniffer blocks.
	cfg["tun"] = buildAndroidTUN(tunFd)
	cfg["sniffer"] = buildSniffer()

	// Runtime-owned fields that must not come from the subscription config.
	cfg["external-controller"] = "127.0.0.1:9090"
	cfg["allow-lan"] = false
	cfg["mode"] = "rule"
	cfg["geodata-mode"] = false
	cfg["geodata-path"] = geoDataDir
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
//          "redir-host"        — returns real upstream IPs, better NTP/STUN compat
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
	// respect-rules: false keeps mihomo's own DNS queries out of proxy rules,
	// which is safe because addDisallowedApplication excludes ourselves from the
	// VPN tunnel so our outbound sockets travel the physical network directly.
	dns["respect-rules"] = false

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
		dns["fake-ip-range"] = "198.18.0.0/15"
		dns["fake-ip-filter-mode"] = "blacklist"
		dns["fake-ip-filter"] = fakeIPFilter(geoDataDir)
	}

	return dns
}

func buildAndroidTUN(tunFd int) map[string]interface{} {
	return map[string]interface{}{
		"enable":                true,
		"stack":                 "gvisor",
		"file-descriptor":       tunFd,
		"auto-route":            false,
		"auto-detect-interface": false,
		"dns-hijack":            defaultDNSHijack,
	}
}

func buildSniffer() map[string]interface{} {
	return map[string]interface{}{
		"enable":               true,
		"override-destination": true,
		"parse-pure-ip":        true,
		"sniff": map[string]interface{}{
			"TLS": map[string]interface{}{
				"ports": []interface{}{443, 8443},
			},
			"HTTP": map[string]interface{}{
				"ports": []interface{}{"80", "8080-8880"},
			},
			"QUIC": map[string]interface{}{
				"ports": []interface{}{443},
			},
		},
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
