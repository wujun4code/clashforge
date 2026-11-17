package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wujun4code/clashforge/internal/dns"
)

// A typed, stateful UCI fixture: ownership must be acquired through real Setup.
// The interpreter path is explicit because host networking programs are absent
// from the test PATH. Every value/address below is synthetic documentation data.
func mockLifecycleOwnedDNS(t *testing.T, port int) {
	t.Helper()
	dir := os.Getenv("CF_MOCK_STATE_DIR")
	if dir == "" {
		t.Fatal("networking command sandbox must be installed first")
	}
	script := `#!/usr/bin/python3
import json, os, shlex, sys
from pathlib import Path
a = sys.argv[1:]
with open(os.environ['CF_COMMAND_LOG'], 'a') as log:
    log.write('uci ' + ' '.join(a) + '\n')
if os.environ.get('CF_FAIL_COMMAND') and os.environ['CF_FAIL_COMMAND'] in 'uci:' + ' '.join(a):
    sys.exit(1)
p = Path(os.environ['CF_MOCK_STATE_DIR']) / 'uci-state.json'
s = json.loads(p.read_text()) if p.exists() else {'port': ['option', ['1053']], 'server': ['list', ['192.0.2.53', '/test.invalid/192.0.2.54']], 'noresolv': ['option', ['0']]}
if a == ['export', 'dhcp']:
    print('package dhcp\n\nconfig dnsmasq ' + shlex.quote('cfgmock'))
    for key, (kind, values) in s.items():
        for value in values:
            print('\t' + kind + ' ' + key + ' ' + shlex.quote(value))
elif a == ['-X', '-q', 'show', 'dhcp.@dnsmasq[0]']:
    print('dhcp.cfgmock=dnsmasq')
elif a == ['commit', 'dhcp']:
    pass
elif len(a) == 2 and a[0] in ['set', 'delete', 'add_list']:
    target, sep, value = a[1].partition('=')
    if not target.startswith('dhcp.cfgmock.'):
        sys.exit(2)
    key = target[len('dhcp.cfgmock.'):]
    if a[0] == 'delete':
        if key not in s:
            sys.exit(1)
        del s[key]
    elif a[0] == 'set' and sep:
        s[key] = ['option', [value]]
    elif a[0] == 'add_list' and sep:
        if key not in s:
            s[key] = ['list', []]
        if s[key][0] != 'list':
            sys.exit(2)
        s[key][1].append(value)
    else:
        sys.exit(2)
    p.write_text(json.dumps(s))
else:
    sys.exit(2)
`
	if err := os.WriteFile(filepath.Join(dir, "uci"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if err := dns.Setup(dns.ModeUpstream, port); err != nil {
		t.Fatal(err)
	}
}

func TestSetupStopWithoutOwnedDNSDoesNotAlterUserDNS(t *testing.T) {
	record := mockLifecycleCommands(t)
	deps := lifecycleDeps(t)
	if err := applyNetworkTakeover(deps); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record, nil, 0600); err != nil {
		t.Fatal(err)
	}
	w := callLifecycle(t, handleSetupStop(deps), "{}")
	if !sseDone(t, w).Success {
		t.Fatal(w.Body.String())
	}
	b, _ := os.ReadFile(record)
	if strings.Contains(string(b), "uci ") || strings.Contains(string(b), "service dnsmasq") {
		t.Fatalf("unowned DNS was modified: %s", b)
	}
	if deps.Netfilter.IsApplied() {
		t.Fatal("network cleanup was skipped")
	}
}

func TestSetupStopDNSFailureRetainsNetworkOwnerForRetry(t *testing.T) {
	mockLifecycleCommands(t)
	deps := lifecycleDeps(t)
	if err := applyNetworkTakeover(deps); err != nil {
		t.Fatal(err)
	}
	mockLifecycleOwnedDNS(t, deps.Config.Ports.DNS)
	t.Setenv("CF_FAIL_COMMAND", "service:dnsmasq restart")
	w := callLifecycle(t, handleSetupStop(deps), "{}")
	if sseDone(t, w).Success || !deps.Netfilter.IsApplied() {
		t.Fatalf("DNS restore failure lost the network owner or reported success: %s", w.Body.String())
	}
	t.Setenv("CF_FAIL_COMMAND", "")
	w = callLifecycle(t, handleSetupStop(deps), "{}")
	if !sseDone(t, w).Success || deps.Netfilter.IsApplied() {
		t.Fatalf("retry did not finish cleanup: %s", w.Body.String())
	}
}
