// Package netreserved is the single source of truth for "address space a
// legitimate public-internet destination should never resolve to". It backs
// two otherwise-unrelated-looking checks that are really the same question
// asked from opposite directions:
//
//   - internal/config.SelectFakeIPRange: picking OUR OWN fake-ip pool, which
//     must live in address space no public-website-direct rule-set (like
//     Loyalsoldier's lancidr.txt) will ever claim as "LAN/reserved, route
//     DIRECT".
//   - internal/dns.ProbeNameservers: detecting whether an upstream resolver
//     has substituted a fake-ip answer for a domain that should have a real
//     public IP — regardless of which specific fake-ip pool the upstream
//     happens to use.
//
// Both only work because virtually every fake-ip implementation (Mihomo,
// OpenClash, Surge, Shadowrocket, ...) is constrained to pick its pool from
// non-globally-routable space, or it would collide with real destinations.
// Keeping one canonical list means a future addition (or a newly-discovered
// non-reserved trick some fork uses, see KnownNonReservedFakeIPRanges) only
// has to be made once.
package netreserved

import "net/netip"

// IANARanges mirrors the IANA IPv4 Special-Purpose Address Registry (RFC
// 6890 and updates) — every block that is not meant to appear as a real
// destination on the public Internet.
var IANARanges = []string{
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

// KnownNonReservedFakeIPRanges lists real, allocated public blocks that are
// nonetheless used as a fake-ip pool by some Clash/Mihomo forks specifically
// because they are NOT in the IANA reserved registry (chosen to dodge
// naive "is this reserved" collision checks against rule-sets like lancidr).
// These can't be derived from any general rule — they're documented tricks,
// added here as they're discovered.
var KnownNonReservedFakeIPRanges = []string{
	"28.0.0.0/8", // historically DoD-assigned, effectively unused on the public Internet
	"29.0.0.0/8", // same rationale as 28.0.0.0/8 — config.fakeIPRangeCandidates' third fallback
}

// ParsePrefixes converts a list of CIDR strings to netip.Prefix, silently
// skipping any that fail to parse.
func ParsePrefixes(cidrs []string) []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		if p, err := netip.ParsePrefix(c); err == nil {
			prefixes = append(prefixes, p)
		}
	}
	return prefixes
}
