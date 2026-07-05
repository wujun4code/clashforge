package mihomobridge

// Line-oriented YAML patching, ported 1:1 from the Android side
// (ClashVpnService.kt) so both platforms apply identical DNS/TUN migrations
// to configs written by any app version.  A YAML library would re-serialise
// the whole document and destroy comments/ordering; surgical line edits
// match what the Kotlin code does.

import (
	"fmt"
	"os"
	"strings"
)

// iosFakeIPFilter mirrors the list in ClashVpnService.patchConfigWithTun.
var iosFakeIPFilter = []string{
	"geosite:cn",
	"*.lan", "*.local", "*.localhost", "*.localdomain",
	"+.stun.*.*", "+.stun.*.*.*",
	"msftconnecttest.com", "*.msftconnecttest.com",
	"time.*.com", "ntp.*.com", "*.pool.ntp.org",
}

// PatchConfigWithTun rewrites config.yaml for in-process TUN operation:
// forces fake-ip DNS mode, replaces any tun/sniffer blocks with the
// canonical iOS stanza, and records the utun fd handed over by the
// NEPacketTunnelProvider.
//
// Differences from the Android stanza, both deliberate:
//   - stack: system — gvisor costs 10–20 MB the 50 MB extension budget
//     can't spare; sing-tun's native darwin path reads the fd directly.
//   - auto-detect-interface: true — iOS has no addDisallowedApplication,
//     so mihomo must bind upstream sockets to the physical interface to
//     avoid routing its own traffic back into the tunnel.
//
// mtu: 1500 must match PacketTunnelProvider's NEPacketTunnelNetworkSettings
// mtu (also 1500) exactly. Confirmed on Android as a real bug, not just a
// theoretical mismatch: when this field is omitted, mihomo's TUN stack
// defaults to 9000 (listener/sing_tun/server.go), while the real OS-level
// TUN interface only accepts 1500-byte packets — small requests work fine,
// but any transfer that grows past 1500 bytes gets silently dropped, which
// is exactly what made large downloads (Play Store app installs, video CDN)
// hang indefinitely a few KB in on Android. Setting it explicitly here so
// iOS doesn't ship with the same latent bug.
//
// sniffer.override-destination: false (global) is likewise a confirmed-on-
// Android fix, not precautionary — see docs/clashforge-wechat-video-bug-report.md
// for the first time this exact bug shipped (WeChat's video CDN) and
// buildSniffer's doc comment on the Android side for the second (Google
// Play's download redirector). With it true, mihomo re-resolves every
// sniffed TLS/QUIC hostname via its own DNS and redials that fresh IP
// instead of preserving the client's original connection, which breaks any
// session/token-bound CDN redirector. HTTP keeps its own true — plain HTTP
// has no SNI, so override-destination there is what makes Host-header
// transparent proxying work, and no CDN-session-token concept to break.
func PatchConfigWithTun(configPath string, tunFd int) error {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	original := string(raw)

	// fake-ip migration applies on every start so configs written by older
	// app versions (redir-host era, or the 198.18.0.0/15 default) are
	// upgraded too. Not 198.18.0.0/15: Loyalsoldier's lancidr.txt (used by
	// LoyalsoldierTemplate/LiteTemplate rule-providers) bundles that exact
	// IANA-reserved block as a "route DIRECT" range, so a fake-ip literal
	// for a domain not covered by a more specific rule-provider falls
	// through to lancidr and gets force-routed DIRECT instead of through
	// the proxy. 28.18.0.1/16 is a real, allocated-but-obscure block (part
	// of 28.0.0.0/8, long assigned to the US DoD network) that lancidr does
	// not bundle — mirrors internal/config/fakeip.go's SelectFakeIPRange on
	// the router side.
	// respect-rules: true — see mobile/cfgen/generator.go's matching comment
	// (Android debug logging caught domestic DNS injection returning a
	// poisoned CN IP for a Google download-CDN redirector hostname; routing
	// that resolution through the proxy, which respect-rules does for any
	// hostname matching a proxy rule, avoids the domestic network path
	// where the injection happens).
	patched := upsertDNSScalar(original, "respect-rules", "true")
	patched = upsertDNSScalar(patched, "enhanced-mode", "fake-ip")
	patched = upsertDNSScalar(patched, "fake-ip-range", "28.18.0.1/16")
	patched = upsertDNSList(patched, "fake-ip-filter", iosFakeIPFilter)

	// memconservative trades geosite lookup speed for a drastically smaller
	// resident matcher — required to fit full geosite.dat under jetsam.
	patched = upsertTopLevelScalar(patched, "geodata-loader", "memconservative")

	patched = removeTopLevelSection(patched, "tun")
	patched = strings.TrimRight(removeTopLevelSection(patched, "sniffer"), " \t\n")

	stanza := fmt.Sprintf(`

tun:
  enable: true
  stack: system
  file-descriptor: %d
  auto-route: false
  auto-detect-interface: true
  mtu: 1500
  dns-hijack:
    - "any:53"
    - "tcp://any:53"
    - "tls://any:853"

sniffer:
  enable: true
  override-destination: false
  parse-pure-ip: true
  sniff:
    TLS:
      ports: [443, 8443]
    HTTP:
      ports: [80, 8080-8880]
      override-destination: true
    QUIC:
      ports: [443]
`, tunFd)

	if err := os.WriteFile(configPath, []byte(patched+stanza), 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func normalizeLines(config string) []string {
	return strings.Split(strings.ReplaceAll(config, "\r\n", "\n"), "\n")
}

// dnsBlockRange returns the line index of the top-level "dns:" key and the
// exclusive end of its block, or (-1, -1) when absent.
func dnsBlockRange(lines []string) (int, int) {
	start := -1
	for i, ln := range lines {
		if strings.TrimSpace(ln) == "dns:" && !strings.HasPrefix(ln, " ") {
			start = i
			break
		}
	}
	if start < 0 {
		return -1, -1
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if lines[i] != "" && !strings.HasPrefix(lines[i], " ") {
			end = i
			break
		}
	}
	return start, end
}

func removeTopLevelSection(config, key string) string {
	lines := normalizeLines(config)
	out := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		ln := lines[i]
		if strings.TrimSpace(ln) == key+":" && !strings.HasPrefix(ln, " ") {
			i++
			for i < len(lines) && (lines[i] == "" || strings.HasPrefix(lines[i], " ")) {
				i++
			}
			continue
		}
		out = append(out, ln)
		i++
	}
	return strings.Join(out, "\n")
}

func upsertDNSScalar(config, key, value string) string {
	lines := normalizeLines(config)
	dnsStart, dnsEnd := dnsBlockRange(lines)
	if dnsStart < 0 {
		return config
	}

	scalarLine := "  " + key + ": " + value
	keyStart := -1
	for i := dnsStart + 1; i < dnsEnd; i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), key+":") {
			keyStart = i
			break
		}
	}

	if keyStart >= 0 {
		keyEnd := keyStart + 1
		for keyEnd < dnsEnd {
			if lines[keyEnd] != "" && !strings.HasPrefix(lines[keyEnd], "    ") {
				break
			}
			keyEnd++
		}
		lines = append(lines[:keyStart], append([]string{scalarLine}, lines[keyEnd:]...)...)
	} else {
		lines = append(lines[:dnsStart+1], append([]string{scalarLine}, lines[dnsStart+1:]...)...)
	}
	return strings.Join(lines, "\n")
}

func upsertDNSList(config, key string, values []string) string {
	lines := normalizeLines(config)
	dnsStart, dnsEnd := dnsBlockRange(lines)
	if dnsStart < 0 {
		return config
	}

	block := []string{"  " + key + ":"}
	for _, v := range values {
		block = append(block, "    - "+renderYAMLScalar(v))
	}

	keyStart := -1
	for i := dnsStart + 1; i < dnsEnd; i++ {
		if strings.TrimSpace(lines[i]) == key+":" {
			keyStart = i
			break
		}
	}

	if keyStart >= 0 {
		keyEnd := keyStart + 1
		for keyEnd < dnsEnd {
			if lines[keyEnd] != "" && !strings.HasPrefix(lines[keyEnd], "    ") {
				break
			}
			keyEnd++
		}
		lines = append(lines[:keyStart], append(block, lines[keyEnd:]...)...)
	} else {
		lines = append(lines[:dnsEnd], append(block, lines[dnsEnd:]...)...)
	}
	return strings.Join(lines, "\n")
}

func upsertTopLevelScalar(config, key, value string) string {
	lines := normalizeLines(config)
	scalarLine := key + ": " + value
	for i, ln := range lines {
		if !strings.HasPrefix(ln, " ") && strings.HasPrefix(strings.TrimSpace(ln), key+":") {
			lines[i] = scalarLine
			return strings.Join(lines, "\n")
		}
	}
	return strings.Join(append(lines, scalarLine), "\n")
}

// renderYAMLScalar quotes values containing YAML-special leading chars
// ('*' alias, '+' merge) or separators, same rules as the Kotlin side.
func renderYAMLScalar(v string) string {
	if strings.ContainsAny(v, " \t:#\"'*+") {
		escaped := strings.ReplaceAll(v, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return v
}

func parseYAMLScalar(raw string) string {
	v := strings.TrimSpace(raw)
	if len(v) >= 2 && strings.HasPrefix(v, `"`) && strings.HasSuffix(v, `"`) {
		v = v[1 : len(v)-1]
		v = strings.ReplaceAll(v, `\"`, `"`)
		v = strings.ReplaceAll(v, `\\`, `\`)
	}
	return v
}
