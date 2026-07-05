package config

import (
	"net/netip"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
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

// ianaReservedRanges mirrors the IANA IPv4 Special-Purpose Address Registry —
// the same "not really Internet-routable" blocks that public LAN/reserved
// rule-sets (lancidr.txt and equivalents) bundle alongside genuine private
// ranges. This is the baseline fallback used before any rule-provider has
// been downloaded (first boot); once a lancidr-style provider is cached on
// disk, its actual payload is unioned in on top of this list (see
// reservedRangesForDataDir) so the check stays in sync with whatever the
// active template really ships, instead of a hand-maintained copy drifting
// out of date with it.
var ianaReservedRanges = []string{
	"0.0.0.0/8",
	"10.0.0.0/8",
	"100.64.0.0/10",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.0.0.0/24",
	"192.0.2.0/24",
	"192.88.99.0/24",
	"192.168.0.0/16",
	"198.18.0.0/15",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"224.0.0.0/4",
	"240.0.0.0/4",
	"255.255.255.255/32",
}

// ExtractFakeIPRange reads dns.fake-ip-range out of a full mihomo-config.yaml
// document. Returns "" if the file can't be parsed or the field is absent
// (e.g. DNS is disabled, or enhanced-mode is redir-host).
func ExtractFakeIPRange(mihomoConfigYAML []byte) string {
	var doc struct {
		DNS struct {
			FakeIPRange string `yaml:"fake-ip-range"`
		} `yaml:"dns"`
	}
	if err := yaml.Unmarshal(mihomoConfigYAML, &doc); err != nil {
		return ""
	}
	return doc.DNS.FakeIPRange
}

// ruleProviderCIDRFile is the on-disk shape of a Mihomo ipcidr rule-provider
// payload, e.g. <DataDir>/rule_provider/lancidr.yaml.
type ruleProviderCIDRFile struct {
	Payload []string `yaml:"payload"`
}

// reservedRangesForDataDir returns ianaReservedRanges plus, best-effort, the
// actual CIDR entries currently cached for the ipcidr rule-providers that the
// generated rule list matches as DIRECT ahead of any domain-based rule
// (lancidr, cncidr, telegramcidr — see internal/publish/template.go). Missing
// or unparsable files are skipped silently: this only ever narrows the set of
// safe candidates, it never widens fake-ip-range's own space, so a cold boot
// with no cache yet still gets the IANA baseline protection.
func reservedRangesForDataDir(dataDir string) []netip.Prefix {
	prefixes := parseCIDRList(ianaReservedRanges)
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
		prefixes = append(prefixes, parseCIDRList(file.Payload)...)
	}
	return prefixes
}

func parseCIDRList(cidrs []string) []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		if p, err := netip.ParsePrefix(c); err == nil {
			prefixes = append(prefixes, p)
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
