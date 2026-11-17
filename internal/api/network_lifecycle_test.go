package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/core"
	"github.com/wujun4code/clashforge/internal/netfilter"
)

// All networking executables are recording stubs; never inherit host PATH.
func mockLifecycleCommands(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	record := filepath.Join(dir, "commands")
	t.Setenv("CF_COMMAND_LOG", record)
	t.Setenv("CF_MOCK_STATE_DIR", dir)
	t.Setenv("CLASHFORGE_DNS_STATE_DIR", filepath.Join(dir, "dns-owned"))
	for _, state := range []string{"forward", "9020", "9021"} {
		if err := os.WriteFile(filepath.Join(dir, state), []byte("1\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	script := `#!/bin/sh
name="${0##*/}"
printf '%s %s\n' "$name" "$*" >> "$CF_COMMAND_LOG"
full="$name:$*"
if [ -n "$CF_FAIL_COMMAND" ]; then case "$full" in *"$CF_FAIL_COMMAND"*) exit 1;; esac; fi
case "$full" in
 ip:'-4 link show dev Meta') printf '9: Meta: <POINTOPOINT,UP,LOWER_UP> mtu 9000\n';;
 ip:'-4 route show table 2022') printf 'default dev Meta\n';;
 ip:'-4 -d rule show')
  printf '0: from all lookup local\n'
  IFS= read -r flag < "$CF_MOCK_STATE_DIR/9020"
  [ "$flag" = 1 ] && printf '9020: from all lookup main suppress_prefixlength 0 proto 242\n'
  IFS= read -r flag < "$CF_MOCK_STATE_DIR/9021"
  [ "$flag" = 1 ] && printf '9021: from all lookup 2022 proto 242\n';;
 ip:'-4 rule del pref 9020 '*) printf '0\n' > "$CF_MOCK_STATE_DIR/9020";;
 ip:'-4 rule del pref 9021 '*) printf '0\n' > "$CF_MOCK_STATE_DIR/9021";;
 ip:'-4 rule add pref 9020 '*) printf '1\n' > "$CF_MOCK_STATE_DIR/9020";;
 ip:'-4 rule add pref 9021 '*) printf '1\n' > "$CF_MOCK_STATE_DIR/9021";;
 nft:'-a list chain inet fw4 forward_lan')
  IFS= read -r flag < "$CF_MOCK_STATE_DIR/forward"
  [ "$flag" = 1 ] && printf 'meta nfproto ipv4 oifname "Meta" counter accept comment "clashforge: allow forwarding into TUN device" # handle 77\n';;
 nft:'delete rule inet fw4 forward_lan handle 77') printf '0\n' > "$CF_MOCK_STATE_DIR/forward";;
 nft:'insert rule inet fw4 forward_lan '*) printf '1\n' > "$CF_MOCK_STATE_DIR/forward";;
 nft:'-f -') /bin/cat > "$CF_MOCK_STATE_DIR/nft-input";;
esac
exit 0
`
	for _, name := range []string{"ip", "nft", "iptables", "ip6tables", "modprobe", "uci", "kill", "pgrep", "service"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	return record
}

func TestRefreshNetfilterCleansOldTUN(t *testing.T) {
	record := mockLifecycleCommands(t)
	old := netfilter.NewManager(netfilter.Config{Mode: "tun", FirewallBackend: "nftables", TunDevice: "Meta"})
	if err := old.Apply(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record, nil, 0644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Network.Mode = "none"
	cfg.Network.FirewallBackend = "none"
	if err := refreshNetfilterManager(Dependencies{Config: cfg, Netfilter: old}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(record)
	if !strings.Contains(string(data), "rule del") || !strings.Contains(string(data), "fw4 forward_lan handle 77") {
		t.Fatalf("old TUN ownership lost without cleanup: %s", data)
	}
}

func TestStartupTakeoverFailureStopsBeforeDNS(t *testing.T) {
	record := mockLifecycleCommands(t)
	dir := filepath.Dir(record)
	if err := os.WriteFile(filepath.Join(dir, "ip"), []byte("#!/bin/sh\nprintf 'ip %s\\n' \"$*\" >> \"$CF_COMMAND_LOG\"\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Network.Mode = "tun"
	cfg.Network.FirewallBackend = "nftables"
	cfg.Network.ApplyOnStart = true
	cfg.DNS.Enable = false
	nf := netfilter.NewManager(netfilter.Config{Mode: "none", FirewallBackend: "none"})
	_, warnings := applyConfiguredStartupTakeover(Dependencies{Config: cfg, Netfilter: nf, Core: core.NewManager(core.CoreManagerConfig{})})
	if len(warnings) == 0 {
		t.Fatal("failed TUN Apply was reported as success")
	}
	if nf.IsApplied() {
		t.Fatal("failed takeover marked applied")
	}
	data, _ := os.ReadFile(record)
	if strings.Contains(string(data), "uci ") {
		t.Fatalf("DNS mutated after takeover failure: %s", data)
	}
}
