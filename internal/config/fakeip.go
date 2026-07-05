package config

import (
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
	"github.com/wujun4code/clashforge/internal/netreserved"
	"gopkg.in/yaml.v3"
)

// fakeIPRangeCandidates lists fake-ip-range values to try, in preference order.
// 198.18.0.1/16 is the historical Clash/Mihomo default and is tried first for
// compatibility, but it sits entirely inside 198.18.0.0/15 — the IANA
// "benchmark testing" block (RFC 2544) that generic LAN/reserved rule-sets
// (e.g. Loyalsoldier's lancidr.txt) legitimately treat as "not really
// Internet-routable, route DIRECT". Any RULE-SET/GEOIP rule matching on raw
// destination IP will then swallow every fake-ip address before domain-based
// rules ever run, forcing foreign proxy-bound traffic to DIRECT and tripping
// Mihomo's own anti-loop guard ("reject loopback connection to: 198.18.x.x").
// 28.18.0.1/16 is a real, allocated-but-obscure block (part of 28.0.0.0/8,
// long assigned to the US DoD network and not announced on the consumer
// Internet) that no mainstream LAN/reserved rule-set bundles — used by some
// Clash forks specifically to dodge this collision. It's the second
// candidate, so it's tried whenever the first one collides with whatever
// reserved-range data we can see (see SelectFakeIPRange).
var fakeIPRangeCandidates = []string{
	"198.18.0.1/16",
	"28.18.0.1/16",
	"29.18.0.1/16",
}

// ExtractFakeIPRange reads dns.fake-ip-range out of a full mihomo-config.yaml
// document. Returns "" if the file can't be parsed or the field is absent
// (e.g. DNS is disabled, or enhanced-mode is redir-host).
func ExtractFakeIPRange(mihomoConfigYAML []byte) string {
	return ExtractDNSTopology(mihomoConfigYAML).FakeIPRange
}

// DNSTopology is the non-sensitive subset of DNS settings ClashForge exposes
// to a downstream ClashForge instance via GET /api/discovery/dns-topology —
// deliberately just enough to let it avoid picking a colliding fake-ip pool,
// nothing about proxies, subscriptions, or credentials.
type DNSTopology struct {
	EnhancedMode string `json:"enhanced_mode" yaml:"enhanced-mode"`
	FakeIPRange  string `json:"fake_ip_range" yaml:"fake-ip-range"`
}

// ExtractDNSTopology reads dns.enhanced-mode and dns.fake-ip-range out of a
// full mihomo-config.yaml document. Returns a zero-value DNSTopology if the
// file can't be parsed.
func ExtractDNSTopology(mihomoConfigYAML []byte) DNSTopology {
	var doc struct {
		DNS DNSTopology `yaml:"dns"`
	}
	if err := yaml.Unmarshal(mihomoConfigYAML, &doc); err != nil {
		return DNSTopology{}
	}
	return doc.DNS
}

// upstreamTopologyFile is the on-disk shape written by the apply flow's
// upstream-discovery step (see internal/api) when a directly-connected
// upstream router answers GET /api/discovery/dns-topology as another
// ClashForge instance running fake-ip.
type upstreamTopologyFile struct {
	FakeIPRange string `json:"fake_ip_range"`
}

// UpstreamTopologyPath returns where the upstream-discovery step persists a
// confirmed upstream fake-ip-range for a given DataDir.
func UpstreamTopologyPath(dataDir string) string {
	return filepath.Join(dataDir, "upstream_dns_topology.json")
}

// ruleProviderCIDRFile is the on-disk shape of a Mihomo ipcidr rule-provider
// payload, e.g. <DataDir>/rule_provider/lancidr.yaml.
type ruleProviderCIDRFile struct {
	Payload []string `yaml:"payload"`
}

// reservedRangesForDataDir returns the IANA baseline plus, best-effort, two
// kinds of on-disk data that only ever narrow the set of safe fake-ip-range
// candidates (never widen it, so a cold boot with nothing cached yet still
// gets the IANA baseline protection):
//
//   - the actual CIDR entries currently cached for the ipcidr rule-providers
//     the generated rule list matches as DIRECT ahead of any domain-based
//     rule (lancidr, cncidr, telegramcidr — see internal/publish/template.go)
//   - a directly-connected upstream router's own fake-ip-range, if it
//     answered GET /api/discovery/dns-topology as another ClashForge
//     instance (see internal/api's upstream-discovery step and
//     UpstreamTopologyPath). This is the precise, authoritative fix for the
//     case a rule-set/IANA-only check can't catch: two chained ClashForge
//     routers whose fake-ip pools happen to collide on a range that is
//     neither reserved nor a known non-reserved trick — we don't need to
//     guess or infer it, we can just ask.
func reservedRangesForDataDir(dataDir string) []netip.Prefix {
	prefixes := netreserved.ParsePrefixes(netreserved.IANARanges)
	for _, provider := range []string{"lancidr", "cncidr", "telegramcidr"} {
		path := filepath.Join(dataDir, "rule_provider", provider+".yaml")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var file ruleProviderCIDRFile
		if err := yaml.Unmarshal(data, &file); err != nil {
			continue
		}
		prefixes = append(prefixes, netreserved.ParsePrefixes(file.Payload)...)
	}
	if data, err := os.ReadFile(UpstreamTopologyPath(dataDir)); err == nil {
		var topo upstreamTopologyFile
		if err := json.Unmarshal(data, &topo); err == nil && topo.FakeIPRange != "" {
			prefixes = append(prefixes, netreserved.ParsePrefixes([]string{topo.FakeIPRange})...)
		}
	}
	return prefixes
}

// SelectFakeIPRange picks the first entry from fakeIPRangeCandidates that does
// not overlap any known-reserved range (IANA baseline + whatever lancidr/
// cncidr/telegramcidr currently have cached on disk for dataDir). This
// replaces a single hardcoded fake-ip-range constant with a validated choice,
// so a future change to the bundled rule-sets (or a chained upstream router
// running its own fake-ip pool on the historical 198.18.0.0/16 default)
// cannot silently reintroduce the same DIRECT-routes-fake-ip collision.
//
// Returns the chosen CIDR and, only when every candidate collided (should
// not happen with the built-in list — a config problem, not a runtime one),
// a non-empty warning to surface to the caller.
func SelectFakeIPRange(dataDir string) (chosen string, warning string) {
	reserved := reservedRangesForDataDir(dataDir)
	for _, cand := range fakeIPRangeCandidates {
		p, err := netip.ParsePrefix(cand)
		if err != nil {
			continue
		}
		collides := false
		for _, r := range reserved {
			if p.Overlaps(r) {
				collides = true
				break
			}
		}
		if !collides {
			return cand, ""
		}
	}
	log.Warn().
		Strs("candidates", fakeIPRangeCandidates).
		Msg("config: 所有内置 fake-ip-range 候选均与已知保留网段冲突，回退到默认值——请检查 lancidr/cncidr 规则集是否异常")
	return fakeIPRangeCandidates[0],
		"所有内置 fake-ip-range 候选均与已知保留网段冲突，已回退到默认值"
}
