package netfilter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// BusyBox 1.36.1 does not expose rule/route protocols and cannot select a
// rule protocol on deletion. Removing -N alone cannot preserve ownership.
func TestPolicyBusyBoxRequiresFullIPBeforeMutation(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "commands")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$POLICY_IP_LOG\"\nprintf '%s\\n' 'BusyBox v1.36.1 multi-call binary.' 'Usage: ip [OPTIONS] address|route|link|neigh|rule [ARGS]' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "ip"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("POLICY_IP_LOG", log)
	for _, call := range []func() error{func() error { return applyTProxyPolicyRouting(true) }, cleanupTProxyPolicyRouting, func() error { _, err := readTunRules(); return err }} {
		err := call()
		if err == nil || !strings.Contains(err.Error(), "ip-full") || !strings.Contains(err.Error(), "protocol") {
			t.Errorf("missing actionable safe dependency diagnostic: %v", err)
		}
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), " add ") || strings.Contains(string(data), " del ") {
		t.Fatalf("unsupported tool used for mutation: %s", data)
	}
}
