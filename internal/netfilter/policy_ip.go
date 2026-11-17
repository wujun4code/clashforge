package netfilter

import (
	"fmt"
	"os/exec"
	"strings"
)

// BusyBox 1.36.1 hides route/rule protocols (and some rule selectors), and
// cannot constrain rule deletion by protocol. A text-only fallback is unsafe:
// it would mistake foreign resources for ours. OpenWrt must install ip-full;
// its alternatives priority selects iproute2 over BusyBox and ip-tiny.
// This read-only probe also keeps rt_tables aliases from masking our numeric
// ownership tuple. Never reinterpret permission/busy errors as absent state.
func inspectPolicyRules() ([]byte, error) {
	out, err := exec.Command("ip", "-N", "-d", "-o", "rule", "show").CombinedOutput()
	if err != nil {
		return out, policyIPInspectionError("inspect policy routing ip -N -d -o rule show", out, err)
	}
	return out, nil
}

func policyIPInspectionError(operation string, out []byte, err error) error {
	detail := strings.TrimSpace(string(out))
	if strings.Contains(detail, "BusyBox") || strings.Contains(detail, "invalid option") || strings.Contains(detail, "Option \"-N\" is unknown") {
		return fmt.Errorf("%s: policy routing requires iproute2 protocol ownership support; install ip-full (opkg update && opkg install ip-full), not BusyBox ip: %w: %s", operation, err, detail)
	}
	return fmt.Errorf("%s: %w: %s", operation, err, detail)
}
