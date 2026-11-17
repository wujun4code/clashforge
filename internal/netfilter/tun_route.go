package netfilter

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const mihomoRouteTable = "2022"
const tunRoutePriority1 = "9020"
const tunRoutePriority2 = "9021"

// Numeric protocol is an ownership marker, not just a priority reservation.
// Untagged legacy rules are deliberately NOT adopted or deleted automatically.
// Requires iproute2 with rule protocol support; unsupported tools fail explicitly.
const tunRuleProtocol = "242"

type tunRouteSpec struct {
	priority, table string
	suppress        bool
}

// Prefer specific main-table routes, suppress its bare default, then fall back
// to mihomo's IPv4 TUN table. This preserves the established routing policy;
// fw4 forward_lan is the only firewall zone this integration supports.
var tunRouteSpecs = []tunRouteSpec{
	{tunRoutePriority1, "main", true},
	{tunRoutePriority2, mihomoRouteTable, false},
}

func (r tunRouteSpec) args(action string) []string {
	a := []string{"-4", "rule", action, "pref", r.priority, "from", "all", "lookup", r.table}
	if r.suppress {
		a = append(a, "suppress_prefixlength", "0")
	}
	return append(a, "protocol", tunRuleProtocol)
}

// matches requires the entire selector/action/ownership tuple. Extra selectors,
// duplicate priorities, and same-priority rules owned by others are conflicts.
func (r tunRouteSpec) matches(line string) bool {
	f := strings.Fields(line)
	if len(f) == 0 || f[0] != r.priority+":" {
		return false
	}
	want := []string{"from", "all", "lookup", r.table}
	if r.suppress {
		want = append(want, "suppress_prefixlength", "0")
	}
	// iproute2 renders the protocol as "proto" (some versions use "protocol").
	tail := f[1:]
	normalized := make([]string, 0, len(tail))
	for i := 0; i < len(tail); i++ {
		if tail[i] == "proto" || tail[i] == "protocol" {
			if i+1 >= len(tail) || tail[i+1] != tunRuleProtocol {
				return false
			}
			i++
			normalized = append(normalized, "protocol", tunRuleProtocol)
		} else {
			normalized = append(normalized, tail[i])
		}
	}
	// Accept protocol before or after suppress_prefixlength, but nothing else.
	withoutProto := []string{}
	protocols := 0
	for i := 0; i < len(normalized); i++ {
		if normalized[i] == "protocol" {
			protocols++
			i++
			continue
		}
		withoutProto = append(withoutProto, normalized[i])
	}
	return protocols == 1 && strings.Join(withoutProto, " ") == strings.Join(want, " ")
}

func readTunRules() ([]string, error) {
	out, err := exec.Command("ip", "-4", "rule", "show").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("inspect IPv4 TUN rules: %w: %s", err, out)
	}
	return strings.Split(string(out), "\n"), nil
}

// ensureTunRouteRules returns only resources created by this invocation, even
// on error. The caller must roll those back without removing preexisting rules.
func ensureTunRouteRules() ([]tunRouteSpec, error) {
	lines, err := readTunRules()
	if err != nil {
		return nil, err
	}
	missing := []tunRouteSpec{}
	// Preflight BOTH priorities before making any changes.
	for _, r := range tunRouteSpecs {
		count := 0
		for _, line := range lines {
			f := strings.Fields(line)
			if len(f) == 0 || f[0] != r.priority+":" {
				continue
			}
			count++
			if !r.matches(line) {
				return nil, fmt.Errorf("TUN IPv4 rule priority %s occupied by foreign or incompatible rule: %s (legacy untagged rules require manual review)", r.priority, strings.TrimSpace(line))
			}
		}
		if count > 1 {
			return nil, fmt.Errorf("TUN IPv4 rule priority %s has duplicate occupants", r.priority)
		}
		if count == 0 {
			missing = append(missing, r)
		}
	}
	added := []tunRouteSpec{}
	for _, r := range missing {
		out, err := exec.Command("ip", r.args("add")...).CombinedOutput()
		if err != nil {
			return added, fmt.Errorf("add TUN IPv4 rule %s: %w: %s", r.priority, err, out)
		}
		added = append(added, r)
	}
	// A zero command status isn't enough if the expected state never appeared.
	lines, err = readTunRules()
	if err != nil {
		return added, err
	}
	for _, r := range tunRouteSpecs {
		count := 0
		for _, line := range lines {
			f := strings.Fields(line)
			if len(f) == 0 || f[0] != r.priority+":" {
				continue
			}
			if !r.matches(line) {
				return added, fmt.Errorf("TUN IPv4 priority %s changed during apply", r.priority)
			}
			count++
		}
		if count != 1 {
			return added, fmt.Errorf("TUN IPv4 rule %s verification failed: got %d", r.priority, count)
		}
	}
	return added, nil
}

// EnsureTunRouteRule installs the IPv4 supplemental pair transactionally.
// It does not certify IPv6, a live TUN device, routing-table contents or LAN data.
func EnsureTunRouteRule() error {
	added, err := ensureTunRouteRules()
	if err != nil {
		return errors.Join(err, removeTunRouteRules(added))
	}
	return nil
}

func removeTunRouteRules(specs []tunRouteSpec) error {
	if len(specs) == 0 {
		return nil
	}
	lines, err := readTunRules()
	if err != nil {
		return err
	}
	var errs []error
	for i := len(specs) - 1; i >= 0; i-- {
		r := specs[i]
		ambiguous := false
		for _, line := range lines {
			f := strings.Fields(line)
			if len(f) > 0 && f[0] == r.priority+":" && !r.matches(line) {
				ambiguous = true
			}
		}
		if ambiguous {
			// A partial delete selector may match a more-specific foreign rule.
			// Never risk it when the priority has changed ownership/semantics.
			for _, line := range lines {
				if r.matches(line) {
					errs = append(errs, fmt.Errorf("refusing ambiguous cleanup at TUN IPv4 priority %s: foreign occupant present", r.priority))
					break
				}
			}
			continue
		}
		for _, line := range lines {
			if !r.matches(line) {
				continue
			}
			out, err := exec.Command("ip", r.args("del")...).CombinedOutput()
			if err != nil {
				errs = append(errs, fmt.Errorf("delete owned TUN IPv4 rule %s: %w: %s", r.priority, err, out))
			}
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	remaining, err := readTunRules()
	if err != nil {
		return err
	}
	for _, r := range specs {
		for _, line := range remaining {
			if r.matches(line) {
				errs = append(errs, fmt.Errorf("owned TUN IPv4 rule %s remains after delete", r.priority))
			}
		}
	}
	return errors.Join(errs...)
}

// RemoveTunRouteRule only removes the fully matching, protocol-tagged rules.
// Missing rules are harmless; inspection and deletion failures are NOT ignored.
func RemoveTunRouteRule() error { return removeTunRouteRules(tunRouteSpecs) }

func checkTunRoute(device string) error {
	link, err := exec.Command("ip", "-4", "link", "show", "dev", device).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect TUN device %s: %w: %s", device, err, link)
	}
	start, end := strings.Index(string(link), "<"), strings.Index(string(link), ">")
	up := false
	if start >= 0 && end > start {
		for _, flag := range strings.Split(string(link)[start+1:end], ",") {
			if flag == "UP" {
				up = true
			}
		}
	}
	if !up {
		return fmt.Errorf("TUN device %s is not UP", device)
	}
	out, err := exec.Command("ip", "-4", "route", "show", "table", mihomoRouteTable).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect TUN IPv4 table %s: %w: %s", mihomoRouteTable, err, out)
	}
	defaults := 0
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || f[0] != "default" {
			continue
		}
		defaults++
		devs := 0
		for i := 1; i < len(f); i++ {
			if f[i] == "linkdown" || f[i] == "nexthop" {
				return fmt.Errorf("unsupported or down TUN IPv4 default: %s", line)
			}
			if f[i] == "dev" {
				devs++
				if i+1 >= len(f) || f[i+1] != device {
					return fmt.Errorf("TUN IPv4 table %s has conflicting default: %s", mihomoRouteTable, line)
				}
			}
		}
		if devs != 1 {
			return fmt.Errorf("TUN IPv4 default does not uniquely use %s: %s", device, line)
		}
	}
	if defaults == 0 {
		return fmt.Errorf("TUN IPv4 table %s has no default route via %s; mihomo auto-route must be ready", mihomoRouteTable, device)
	}
	return nil
}
