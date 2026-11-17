package quickstart

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/core"
	"github.com/wujun4code/clashforge/internal/dns"
	"github.com/wujun4code/clashforge/internal/netfilter"
	"github.com/wujun4code/clashforge/internal/subscription"
)

// All networking executables are recording stubs; never inherit host PATH.
func mockQuickstartCommands(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	record := filepath.Join(dir, "commands")
	t.Setenv("CF_COMMAND_LOG", record)
	t.Setenv("CF_MOCK_STATE_DIR", dir)
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
 ip:'-4 rule show')
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
	installStartupDNSFixture(t, dir)
	t.Setenv("PATH", dir)
	return record
}

func quickstartLifecycleDeps(t *testing.T) (Deps, string) {
	t.Helper()
	record := mockQuickstartCommands(t)
	cfg := config.Default()
	cfg.Core.DataDir = t.TempDir()
	cfg.Core.RuntimeDir = t.TempDir()
	cfg.Network.IPv6 = false
	cfg.Network.FirewallBackend = "nftables"
	cfg.DNS.DnsmasqMode = "upstream"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	t.Cleanup(server.Close)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	apiPort, _ := strconv.Atoi(port)
	binary := filepath.Join(t.TempDir(), "test-core")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexec /bin/sleep 600\n"), 0755); err != nil {
		t.Fatal(err)
	}
	cm := core.NewManager(core.CoreManagerConfig{Binary: binary, ConfigFile: filepath.Join(cfg.Core.RuntimeDir, "mihomo-config.yaml"), APIPort: apiPort})
	t.Cleanup(func() { _ = cm.Stop() })
	return Deps{Config: cfg, ConfigPath: filepath.Join(t.TempDir(), "config.toml"), Core: cm, SubManager: subscription.NewManager(cfg.Core.DataDir), Netfilter: netfilter.NewManager(netfilter.Config{Mode: "none", FirewallBackend: "none"})}, record
}

func TestQuickstartRejectsMissingGeneration(t *testing.T) {
	deps, _ := quickstartLifecycleDeps(t)
	deps.SubManager = nil
	err := autoConfigureClashForge(context.Background(), deps, func(Event) {}, "", "", "", nil)
	if err == nil {
		t.Fatal("quickstart accepted generated=false and started stale runtime")
	}
	if deps.Core.Status().Ready {
		t.Fatal("core started despite missing generation")
	}
}

func TestQuickstartPropagatesApplyFailureBeforeDNS(t *testing.T) {
	deps, record := quickstartLifecycleDeps(t)
	t.Setenv("CF_FAIL_COMMAND", "nft:-f -")
	err := autoConfigureClashForge(context.Background(), deps, func(Event) {}, "", "", "", nil)
	if err == nil {
		t.Error("Apply failure reported as quickstart success")
	}
	if err != nil && !strings.Contains(err.Error(), "apply netfilter") {
		t.Fatalf("did not reach expected Apply failure: %v", err)
	}
	data, _ := os.ReadFile(record)
	if strings.Contains(string(data), "server=127.0.0.1#") {
		t.Fatalf("DNS setup after failed Apply: %s", data)
	}
}

func TestQuickstartRetainsOldTUNOwnerOnCleanupFailure(t *testing.T) {
	deps, record := quickstartLifecycleDeps(t)
	old := netfilter.NewManager(netfilter.Config{Mode: "tun", FirewallBackend: "nftables", TunDevice: "Meta"})
	if err := old.Apply(); err != nil {
		t.Fatal(err)
	}
	deps.Netfilter = old
	t.Setenv("CF_FAIL_COMMAND", "ip:-4 rule del")
	err := autoConfigureClashForge(context.Background(), deps, func(Event) {}, "", "", "", nil)
	if err == nil {
		t.Error("cleanup failure discarded old TUN owner")
	}
	if deps.Core.Status().Ready {
		t.Error("started before old owner cleaned")
	}
	t.Setenv("CF_FAIL_COMMAND", "")
	if err := old.Cleanup(); err != nil {
		t.Fatalf("cleanup retry: %v", err)
	}
	data, _ := os.ReadFile(record)
	if !strings.Contains(string(data), "ip -4 rule del") {
		t.Fatalf("TUN owner lost without cleanup/retry: %s", data)
	}
}

func TestQuickstartRestartsRunningCore(t *testing.T) {
	deps, _ := quickstartLifecycleDeps(t)
	if err := deps.Core.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	oldPID := deps.Core.Status().PID
	if err := autoConfigureClashForge(context.Background(), deps, func(Event) {}, "", "", "", nil); err != nil {
		t.Fatal(err)
	}
	if deps.Core.Status().PID == oldPID {
		t.Fatal("running core kept old config instead of Restart")
	}
}

func TestQuickstartGenerationErrorKeepsOldOwner(t *testing.T) {
	deps, record := quickstartLifecycleDeps(t)
	old := netfilter.NewManager(netfilter.Config{Mode: "tun", FirewallBackend: "nftables", TunDevice: "Meta"})
	if err := old.Apply(); err != nil {
		t.Fatal(err)
	}
	deps.Netfilter = old
	deps.Config.Core.RuntimeDir = filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(deps.Config.Core.RuntimeDir, []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := autoConfigureClashForge(context.Background(), deps, func(Event) {}, "", "", "", nil); err == nil {
		t.Fatal("generation error accepted")
	}
	if !old.IsApplied() || deps.Core.Status().Ready {
		t.Fatal("generation failure changed old owner/core")
	}
	data, _ := os.ReadFile(record)
	if strings.Contains(string(data), "rule del") || strings.Contains(string(data), "uci ") {
		t.Fatalf("generation failure released old takeover: %s", data)
	}
}

func TestQuickstartDNSRestoreFailureKeepsOldOwner(t *testing.T) {
	deps, record := quickstartLifecycleDeps(t)
	if err := dns.Setup(dns.ModeUpstream, deps.Config.Ports.DNS); err != nil {
		t.Fatal(err)
	}
	old := netfilter.NewManager(netfilter.Config{Mode: "tun", FirewallBackend: "nftables", TunDevice: "Meta"})
	if err := old.Apply(); err != nil {
		t.Fatal(err)
	}
	deps.Netfilter = old
	if err := os.WriteFile(record, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CF_FAIL_COMMAND", "uci:commit dhcp")
	if err := autoConfigureClashForge(context.Background(), deps, func(Event) {}, "", "", "", nil); err == nil {
		t.Fatal("DNS restore failure accepted")
	}
	if !old.IsApplied() || deps.Core.Status().Ready {
		t.Fatal("failed restore retired manager or started core")
	}
	data, _ := os.ReadFile(record)
	if strings.Contains(string(data), "rule del") {
		t.Fatalf("cleanup ran after failed DNS restore: %s", data)
	}
}

func TestQuickstartPropagatesDNSSetupFailure(t *testing.T) {
	deps, _ := quickstartLifecycleDeps(t)
	t.Setenv("CF_FAIL_COMMAND", "uci:add_list")
	if err := autoConfigureClashForge(context.Background(), deps, func(Event) {}, "", "", "", nil); err == nil {
		t.Fatal("DNS Setup failure reported as success")
	} else if !strings.Contains(err.Error(), "setup DNS") || !strings.Contains(err.Error(), "add_list") {
		t.Fatalf("did not reach DNS mutation failure: %v", err)
	}
}
