package netfilter

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/rs/zerolog/log"
)

// mihomoRouteTable is mihomo's fixed Linux policy-routing table ID for TUN
// auto-route (see mihomo's tun/route_linux.go). Once auto-route succeeds it
// always contains a default route pointing into the TUN device.
const mihomoRouteTable = "2022"

// tunRoutePriority1/2 sit below mihomo's own auto-route rules (observed at
// priorities 9000-9010) but above the kernel's default rule (32766: from all
// lookup main), so they only catch traffic mihomo's own rules didn't already
// claim.
const tunRoutePriority1 = "9020"
const tunRoutePriority2 = "9021"

// EnsureTunRouteRule adds the ip-rule pair that routes LAN-client (forwarded)
// traffic into mihomo's TUN routing table.
//
// mihomo's "auto-route" on Linux only installs ip rules that capture traffic
// originated by the router itself (loopback) and traffic destined to the
// TUN's own point-to-point subnet (observed: "9002: from <lo> lookup 2022"
// and "9000: from all to <tun>/30 lookup 2022"). It has no concept of "this
// box is also forwarding for a downstream LAN" (e.g. br-lan clients), so
// packets forwarded from the LAN never match any of mihomo's own rules and
// fall straight through to the kernel's literal default rule
// (32766: from all lookup main) — which still points at the real upstream
// gateway, not the TUN device. Symptom observed: LAN clients get TCP RST
// (or, if an upstream router happens to also run fake-ip in the same CIDR,
// confusing partial responses) for destinations mihomo should have
// intercepted, while the router's own (loopback) traffic through mihomo
// works fine — because only loopback traffic was ever actually captured.
//
// The fix mirrors the "suppress_prefixlength" trick mihomo itself uses for
// its own rules: first try the main table but ignore a bare default-route
// match (so genuinely specific routes — other LAN subnets, the TUN's own
// subnet — still resolve normally via main); only traffic with no more
// specific route in main falls through to mihomo's table 2022, which always
// carries a real default route into the TUN device once auto-route is up.
func EnsureTunRouteRule() error {
	existing, _ := exec.Command("ip", "rule", "show").CombinedOutput()
	rules := string(existing)

	if !hasRulePriority(rules, tunRoutePriority1) {
		cmd := exec.Command("ip", "rule", "add", "pref", tunRoutePriority1, "lookup", "main", "suppress_prefixlength", "0")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("ip rule add (suppress main default): %w: %s", err, string(out))
		}
	}
	if !hasRulePriority(rules, tunRoutePriority2) {
		cmd := exec.Command("ip", "rule", "add", "pref", tunRoutePriority2, "lookup", mihomoRouteTable)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("ip rule add (tun forward fallback): %w: %s", err, string(out))
		}
	}
	log.Info().Msg("netfilter: 已确保 LAN 转发流量可进入 TUN 路由表 (table 2022) ✓")
	return nil
}

// RemoveTunRouteRule deletes the ip rules added by EnsureTunRouteRule.
// Best-effort: missing rules are not an error.
func RemoveTunRouteRule() error {
	_ = exec.Command("ip", "rule", "del", "pref", tunRoutePriority2, "lookup", mihomoRouteTable).Run()
	_ = exec.Command("ip", "rule", "del", "pref", tunRoutePriority1, "lookup", "main", "suppress_prefixlength", "0").Run()
	return nil
}

func hasRulePriority(rulesOutput, pref string) bool {
	prefix := pref + ":"
	for _, line := range strings.Split(rulesOutput, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			return true
		}
	}
	return false
}
