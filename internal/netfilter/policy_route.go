package netfilter

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// applyTProxyPolicyRouting installs the routes needed by both TProxy backends.
// A rendered firewall is not a working interception path until these commands
// succeed. Report failures (including stderr), rather than claiming Apply worked.
// This is not transactional rollback or a global fail-closed guarantee: earlier
// commands and firewall rules may already be installed when an error is returned.
func applyTProxyPolicyRouting(enableIPv6 bool) error {
	commands := [][]string{
		{"rule", "add", "fwmark", fwMark, "table", routeTable},
		{"route", "add", "local", "default", "dev", "lo", "table", routeTable},
	}
	if enableIPv6 {
		commands = append(commands,
			[]string{"-6", "rule", "add", "fwmark", fwMark, "table", routeTable},
			[]string{"-6", "route", "add", "local", "default", "dev", "lo", "table", routeTable},
		)
	}
	// OUTPUT only marks IPv4 fake-IP destinations, so no IPv6 OUTPUT route.
	commands = append(commands,
		[]string{"rule", "add", "fwmark", fwMarkOutput, "table", routeTableOutput},
		[]string{"route", "add", "local", "default", "dev", "lo", "table", routeTableOutput},
	)
	for _, args := range commands {
		if out, err := exec.Command("ip", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("policy routing ip %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// The existing TProxy selectors (unlike TUN) have no protocol ownership tag.
// Preserve that compatibility boundary, but do not broaden it to a shared-table
// flush or deletion of rules with extra source/interface/mask selectors. Inspect
// complete tuples and use their priority only in conjunction with that tuple.
func cleanupTProxyPolicyRouting() error {
	var errs []error
	for _, spec := range []struct{ family, mark, table string }{
		{"", fwMark, routeTable}, {"-6", fwMark, routeTable}, {"", fwMarkOutput, routeTableOutput},
	} {
		prefix := []string{}
		if spec.family != "" {
			prefix = append(prefix, spec.family)
		}
		run := func(args ...string) ([]byte, error) {
			args = append(append([]string{}, prefix...), args...)
			out, err := exec.Command("ip", args...).CombinedOutput()
			if err != nil {
				return out, fmt.Errorf("policy cleanup ip %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
			}
			return out, nil
		}
		// Numeric output avoids aliases from /etc/iproute2/rt_tables masking
		// our table numbers during exact tuple matching.
		out, err := run("-N", "-o", "rule", "show")
		if err != nil {
			errs = append(errs, err)
		} else {
			for _, line := range strings.Split(string(out), "\n") {
				fields := strings.Fields(line)
				// Default ip-rule protocol is boot, normally omitted by iproute2.
				if len(fields) == 9 && fields[7] == "proto" && (fields[8] == "boot" || fields[8] == "3") {
					fields = fields[:7]
				}
				if len(fields) != 7 || strings.Join(fields[1:], " ") != "from all fwmark "+spec.mark+" lookup "+spec.table {
					continue
				}
				priority := strings.TrimSuffix(fields[0], ":")
				if _, err := strconv.ParseUint(priority, 10, 32); err != nil || !strings.HasSuffix(fields[0], ":") {
					errs = append(errs, fmt.Errorf("invalid TProxy rule priority: %s", line))
					continue
				}
				out, err := run("rule", "del", "fwmark", spec.mark, "table", spec.table, "from", "all", "priority", priority, "protocol", "boot")
				if err != nil && !policyResourceAbsent(out) {
					errs = append(errs, err)
				}
			}
		}
		out, err = run("-o", "route", "show", "table", spec.table)
		if err != nil {
			// An uncreated table is a normal partial-Apply cleanup case.
			if !strings.Contains(string(out), "FIB table does not exist") {
				errs = append(errs, err)
			}
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Fields(line)
			if !defaultTProxyRoute(fields, spec.family == "-6") {
				continue
			}
			args := append([]string{"route", "del"}, fields...)
			args = append(args, "table", spec.table)
			out, err := run(args...)
			if err != nil && !policyResourceAbsent(out) {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func policyResourceAbsent(out []byte) bool {
	s := strings.TrimSpace(string(out))
	return s == "RTNETLINK answers: No such file or directory" || s == "RTNETLINK answers: No such process"
}

// Only the local-loopback default created by Apply; unrelated destinations,
// devices, protocols and nondefault metrics in tables 100/101 remain untouched.
func defaultTProxyRoute(fields []string, ipv6 bool) bool {
	if len(fields) < 4 || strings.Join(fields[:4], " ") != "local default dev lo" || (len(fields)-4)%2 != 0 {
		return false
	}
	seen := map[string]bool{}
	for j := 4; j < len(fields); j += 2 {
		key, value := fields[j], fields[j+1]
		if seen[key] {
			return false
		}
		seen[key] = true
		switch key {
		case "proto":
			if value != "boot" && value != "3" {
				return false
			}
		case "scope":
			if value != "host" {
				return false
			}
		case "metric":
			if (!ipv6 && value != "0") || (ipv6 && value != "1024") {
				return false
			}
		case "pref":
			if !ipv6 || value != "medium" {
				return false
			}
		default:
			return false
		}
	}
	return true
}
