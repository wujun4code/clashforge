package main

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
	"time"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/core"
	"github.com/wujun4code/clashforge/internal/dns"
	"github.com/wujun4code/clashforge/internal/netfilter"
	"github.com/wujun4code/clashforge/internal/subscription"
)

// All networking executables are recording stubs; never inherit host PATH.
func mockStartupCommands(t *testing.T) string {
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
	installStartupDNSFixture(t, dir)
	t.Setenv("PATH", dir)
	return record
}

func startupLifecycleDeps(t *testing.T) (*config.MetaclashConfig, *subscription.Manager, *core.CoreManager, *netfilter.Manager, string) {
	t.Helper()
	record := mockStartupCommands(t)
	cfg := config.Default()
	cfg.Core.DataDir = t.TempDir()
	cfg.Core.RuntimeDir = t.TempDir()
	cfg.Network.Mode = "tun"
	cfg.Network.TUN.Device = "Meta"
	cfg.Network.IPv6 = false
	cfg.Network.ApplyOnStart = true
	cfg.Network.FirewallBackend = "nftables"
	cfg.DNS.Enable = false
	cfg.DNS.Strategy = config.DNSStrategysplit
	cfg.DNS.DnsmasqMode = "upstream"
	cfg.DNS.ApplyOnStart = true
	healthMarker := filepath.Join(os.Getenv("CF_MOCK_STATE_DIR"), "api-unhealthy")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := os.Stat(healthMarker); err == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	apiPort, _ := strconv.Atoi(port)
	binary := filepath.Join(t.TempDir(), "test-core")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf 'core-start\\n' >> \"$CF_COMMAND_LOG\"\nexec /bin/sleep 600\n"), 0755); err != nil {
		t.Fatal(err)
	}
	cm := core.NewManager(core.CoreManagerConfig{Binary: binary, ConfigFile: filepath.Join(cfg.Core.RuntimeDir, "mihomo-config.yaml"), APIPort: apiPort})
	t.Cleanup(func() { _ = cm.Stop() })
	return cfg, subscription.NewManager(cfg.Core.DataDir), cm, netfilter.NewManager(startupNetfilterConfig(cfg)), record
}

// Exercise the registered production crash callback, not only the strategy helper.
// Removing only our harmless child executable also forces the real watchdog's
// automatic restart to fail, without touching any host process/network state.
func TestStrictActualCrashRetainsTakeoverAfterRestartFailure(t *testing.T) {
	cfg, sm, cm, nf, record := startupLifecycleDeps(t)
	cfg.DNS.Enable = true
	cfg.DNS.Strategy = " ANTI-HIJACK "
	originalDNS := startupDNSState(t)
	if err := dns.Setup(dns.ModeUpstream, cfg.Ports.DNS); err != nil {
		t.Fatal(err)
	}
	managedDNS := startupDNSState(t)
	if err := nf.Apply(); err != nil {
		t.Fatal(err)
	}
	mu := installStartupWatchdog(cfg, sm, cm, nf)
	if err := cm.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	// The independent HTTP fixture may answer before /bin/sh has opened its
	// script. Wait for the test child's acknowledgement before unlinking it;
	// otherwise the child can exit on ENOENT before the intended crash signal.
	started := false
	startDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(startDeadline) {
		data, err := os.ReadFile(record)
		if err == nil && strings.Contains(string(data), "core-start\n") {
			started = true
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !started {
		t.Fatal("test core did not acknowledge script startup")
	}
	if err := os.Remove(cm.Status().Binary); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record, nil, 0600); err != nil {
		t.Fatal(err)
	}
	proc, err := os.FindProcess(cm.Status().PID)
	if err != nil {
		t.Fatal(err)
	}
	if err := proc.Kill(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		status := cm.Status()
		if status.State == core.StateError && strings.Contains(status.LastError, "no such file or directory") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	status := cm.Status()
	if status.State != core.StateError || !strings.Contains(status.LastError, "no such file or directory") {
		t.Fatalf("watchdog failure not exercised: %+v", status)
	}
	if !nf.IsApplied() {
		t.Fatal("strict crash released required netfilter owner")
	}
	assertStartupDNSEqual(t, managedDNS)
	data, _ := os.ReadFile(record)
	if strings.Contains(string(data), "uci ") || strings.Contains(string(data), "rule del") || strings.Contains(string(data), "delete rule") {
		t.Fatalf("strict failure released takeover: %s", data)
	}
	// Explicit stop must restore the original resolver even after restart fails.
	if err := releaseStartupTakeover(nf); err != nil {
		t.Fatal(err)
	}
	assertStartupDNSEqual(t, originalDNS)
	if nf.IsApplied() {
		t.Fatal("explicit release retained netfilter readiness")
	}
}

func TestStrictRecoveryFailureRetainsTakeover(t *testing.T) {
	for _, failure := range []string{"generation", "restart"} {
		t.Run(failure, func(t *testing.T) {
			cfg, sm, cm, nf, record := startupLifecycleDeps(t)
			cfg.DNS.Enable = true
			cfg.DNS.Strategy = config.DNSStrategyAntiHijack
			if err := nf.Apply(); err != nil {
				t.Fatal(err)
			}
			if failure == "generation" {
				sm = nil
			} else if err := os.Remove(cm.Status().Binary); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(record, nil, 0600); err != nil {
				t.Fatal(err)
			}
			ready, err := recoverStartupTakeover(context.Background(), cfg, sm, cm, nf)
			if err == nil || ready {
				t.Fatalf("strict recovery accepted %s failure: %t %v", failure, ready, err)
			}
			if !nf.IsApplied() {
				t.Fatal("strict failed recovery lost old owner")
			}
			data, _ := os.ReadFile(record)
			if strings.Contains(string(data), "uci ") || strings.Contains(string(data), "rule del") {
				t.Fatalf("strict failed recovery restored untrusted DNS/removed rules: %s", data)
			}
		})
	}
}

func TestStrictRecoveryRequiresHealthyCoreWithoutISPDetour(t *testing.T) {
	for _, unhealthy := range []bool{false, true} {
		t.Run(strconv.FormatBool(unhealthy), func(t *testing.T) {
			cfg, sm, cm, nf, record := startupLifecycleDeps(t)
			cfg.DNS.Enable = true
			cfg.DNS.Strategy = config.DNSStrategyAntiHijack
			if err := nf.Apply(); err != nil {
				t.Fatal(err)
			}
			if err := dns.Setup(dns.ModeUpstream, cfg.Ports.DNS); err != nil {
				t.Fatal(err)
			}
			managed := startupDNSState(t)
			if unhealthy {
				if err := os.WriteFile(filepath.Join(os.Getenv("CF_MOCK_STATE_DIR"), "api-unhealthy"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(record, nil, 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			ready, err := recoverStartupTakeover(ctx, cfg, sm, cm, nf)
			if unhealthy {
				if ready || err == nil || !strings.Contains(err.Error(), "not healthy") {
					t.Fatalf("unhealthy core accepted: %t %v", ready, err)
				}
			} else if !ready || err != nil {
				t.Fatalf("healthy recovery rejected: %t %v", ready, err)
			}
			assertStartupDNSEqual(t, managed)
			if !nf.IsApplied() {
				t.Fatal("strict recovery lost required owner")
			}
			data, _ := os.ReadFile(record)
			trace := string(data)
			for _, forbidden := range []string{"203.0.113.53", "port=5353", "noresolv=0", "rule del", "delete rule"} {
				if strings.Contains(trace, forbidden) {
					t.Fatalf("strict recovery restored untrusted DNS or released owner: %s", trace)
				}
			}
		})
	}
}

func TestStrictActualWatchdogRecoveryNeverRestoresISP(t *testing.T) {
	for _, unhealthy := range []bool{false, true} {
		t.Run(strconv.FormatBool(unhealthy), func(t *testing.T) {
			cfg, sm, cm, nf, record := startupLifecycleDeps(t)
			cfg.DNS.Enable = true
			cfg.DNS.Strategy = config.DNSStrategyAntiHijack
			if err := nf.Apply(); err != nil {
				t.Fatal(err)
			}
			if err := dns.Setup(dns.ModeUpstream, cfg.Ports.DNS); err != nil {
				t.Fatal(err)
			}
			managed := startupDNSState(t)
			mu := installStartupWatchdog(cfg, sm, cm, nf)
			if err := cm.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			if unhealthy {
				if err := os.WriteFile(filepath.Join(os.Getenv("CF_MOCK_STATE_DIR"), "api-unhealthy"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(record, nil, 0600); err != nil {
				t.Fatal(err)
			}
			proc, err := os.FindProcess(cm.Status().PID)
			if err != nil {
				t.Fatal(err)
			}
			if err := proc.Kill(); err != nil {
				t.Fatal(err)
			}
			complete := false
			deadline := time.Now().Add(12 * time.Second)
			for time.Now().Before(deadline) {
				data, _ := os.ReadFile(record)
				trace := string(data)
				if strings.Count(trace, "core-start") >= 2 {
					mu.Lock()
					status := cm.Status()
					if unhealthy {
						complete = status.State == core.StateStopped && !status.Ready
					} else {
						complete = status.Ready && strings.Contains(trace, "uci commit dhcp")
					}
					mu.Unlock()
					if complete {
						break
					}
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !complete {
				t.Fatalf("strict real restart callback did not complete (unhealthy=%t): %+v", unhealthy, cm.Status())
			}
			mu.Lock()
			defer mu.Unlock()
			assertStartupDNSEqual(t, managed)
			if !nf.IsApplied() {
				t.Fatal("strict callback discarded required owner")
			}
			data, _ := os.ReadFile(record)
			for _, forbidden := range []string{"203.0.113.53", "port=5353", "noresolv=0", "rule del", "delete rule"} {
				if strings.Contains(string(data), forbidden) {
					t.Fatalf("real strict callback released takeover: %s", data)
				}
			}
		})
	}
}

func TestStrictExplicitStopRestoreFailureRetainsOwnerForRetry(t *testing.T) {
	cfg, _, _, nf, record := startupLifecycleDeps(t)
	cfg.DNS.Enable = true
	cfg.DNS.Strategy = config.DNSStrategyAntiHijack
	original := startupDNSState(t)
	if err := nf.Apply(); err != nil {
		t.Fatal(err)
	}
	if err := dns.Setup(dns.ModeUpstream, cfg.Ports.DNS); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CF_FAIL_COMMAND", "uci:commit dhcp")
	if err := releaseStartupTakeover(nf); err == nil {
		t.Fatal("restore failure reported success")
	}
	if !nf.IsApplied() {
		t.Fatal("restore failure retired required owner")
	}
	data, _ := os.ReadFile(record)
	if strings.Contains(string(data), "rule del") {
		t.Fatal("netfilter cleanup preceded successful restore")
	}
	t.Setenv("CF_FAIL_COMMAND", "")
	if err := releaseStartupTakeover(nf); err != nil {
		t.Fatal(err)
	}
	assertStartupDNSEqual(t, original)
	if nf.IsApplied() {
		t.Fatal("explicit stop retry did not release owner")
	}
}

func TestDaemonActualWatchdogCallbacks(t *testing.T) {
	for _, failDNS := range []bool{false, true} {
		name := "dns-disabled"
		if failDNS {
			name = "dns-setup-fails"
		}
		t.Run(name, func(t *testing.T) {
			cfg, sm, cm, nf, record := startupLifecycleDeps(t)
			cfg.DNS.Enable = failDNS
			// Acquire DNS even for the disabled case: release follows ownership,
			// not current feature flags.
			if err := dns.Setup(dns.ModeUpstream, cfg.Ports.DNS); err != nil {
				t.Fatal(err)
			}
			if err := nf.Apply(); err != nil {
				t.Fatal(err)
			}
			mu := installStartupWatchdog(cfg, sm, cm, nf)
			if err := cm.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(record, nil, 0600); err != nil {
				t.Fatal(err)
			}
			if failDNS {
				t.Setenv("CF_FAIL_COMMAND", "uci:add_list dhcp.cfgtest.server=127.0.0.1#")
			}
			proc, err := os.FindProcess(cm.Status().PID)
			if err != nil {
				t.Fatal(err)
			}
			if err := proc.Kill(); err != nil {
				t.Fatal(err)
			} // only our test's sleep child
			deadline := time.Now().Add(10 * time.Second)
			completed := false
			for time.Now().Before(deadline) {
				data, _ := os.ReadFile(record)
				trace := string(data)
				if strings.Count(trace, "core-start") >= 2 && strings.Contains(trace, "ip -4 rule add pref 9021") {
					mu.Lock()
					status := cm.Status()
					applied := nf.IsApplied()
					mu.Unlock()
					if failDNS {
						completed = status.State == core.StateStopped && !status.Ready
					} else {
						completed = status.Ready && applied
					}
					if completed {
						break
					}
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !completed {
				data, _ := os.ReadFile(record)
				t.Fatalf("actual watchdog did not complete expected recovery/failure: %+v\n%s", cm.Status(), data)
			}
			data, _ := os.ReadFile(record)
			trace := string(data)
			restore, cleanup := strings.Index(trace, "uci set dhcp.cfgtest.port=5353"), strings.Index(trace, "ip -4 rule del")
			if restore < 0 || cleanup <= restore {
				t.Fatalf("crash callback bypassed DNS-first manager cleanup: %s", trace)
			}
			if !failDNS && strings.Contains(trace, "server=127.0.0.1#") {
				t.Fatalf("DNS-disabled callback set up DNS: %s", trace)
			}
		})
	}
}

func TestDaemonNetfilterConfigPreservesPolicy(t *testing.T) {
	cfg, _, _, _, _ := startupLifecycleDeps(t)
	cfg.Network.IPv6 = true
	cfg.Network.DropQUIC = true
	cfg.Network.WANInterface = "test-wan"
	if err := os.WriteFile(filepath.Join(cfg.Core.RuntimeDir, "mihomo-config.yaml"), []byte("dns:\n  fake-ip-range: 198.20.0.0/16\n"), 0600); err != nil {
		t.Fatal(err)
	}
	nfCfg := startupNetfilterConfig(cfg)
	if !nfCfg.EnableIPv6 || !nfCfg.DropQUIC || nfCfg.WANInterface != "test-wan" || nfCfg.FakeIPRange != "198.20.0.0/16" {
		t.Fatalf("lost netfilter policy: %+v", nfCfg)
	}
	// Passing IPv6 through must trigger the netfilter layer's explicit refusal.
	if err := netfilter.NewManager(nfCfg).Apply(); err == nil || !strings.Contains(err.Error(), "IPv6") {
		t.Fatalf("TUN IPv6 refusal bypassed: %v", err)
	}
}

func TestDaemonTakeoverFailureBlocksDNS(t *testing.T) {
	cfg, _, _, _, record := startupLifecycleDeps(t)
	cfg.DNS.Enable = true
	t.Setenv("CF_FAIL_COMMAND", "ip:-4 link show")
	ready, err := applyStartupTakeover(cfg, netfilter.NewManager(startupNetfilterConfig(cfg)))
	if err == nil || ready {
		t.Fatalf("failed takeover reported ready: %t, %v", ready, err)
	}
	data, _ := os.ReadFile(record)
	if strings.Contains(string(data), "uci ") {
		t.Fatalf("DNS touched after Apply failure: %s", data)
	}
}

func TestDaemonRejectsTUNReplaceBeforeMutation(t *testing.T) {
	cfg, _, _, _, record := startupLifecycleDeps(t)
	cfg.DNS.Enable = true
	cfg.DNS.DnsmasqMode = "replace"
	ready, err := applyStartupTakeover(cfg, netfilter.NewManager(startupNetfilterConfig(cfg)))
	if err == nil || ready {
		t.Fatalf("TUN+replace accepted: %t %v", ready, err)
	}
	data, _ := os.ReadFile(record)
	if len(data) != 0 {
		t.Fatalf("unsupported DNS coexistence mutated networking: %s", data)
	}
}

func TestDaemonDNSFailureIsNotReady(t *testing.T) {
	cfg, _, _, _, _ := startupLifecycleDeps(t)
	cfg.Network.Mode = "none"
	cfg.DNS.Enable = true
	t.Setenv("CF_FAIL_COMMAND", "uci:add_list dhcp.cfgtest.server=127.0.0.1#")
	ready, err := applyStartupTakeover(cfg, nil)
	if err == nil || ready {
		t.Fatalf("DNS failure reported ready: %t %v", ready, err)
	}
}

func TestWatchdogRecoversNetworkWhenDNSDisabled(t *testing.T) {
	cfg, sm, cm, nf, record := startupLifecycleDeps(t)
	if err := dns.Setup(dns.ModeUpstream, cfg.Ports.DNS); err != nil {
		t.Fatal(err)
	}
	if err := nf.Apply(); err != nil {
		t.Fatal(err)
	}
	if err := cm.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	oldPID := cm.Status().PID
	if err := os.WriteFile(record, nil, 0600); err != nil {
		t.Fatal(err)
	}
	ready, err := recoverStartupTakeover(context.Background(), cfg, sm, cm, nf)
	if err != nil || ready || !nf.IsApplied() {
		t.Fatalf("DNS-disabled network recovery: DNS=%t applied=%t error=%v", ready, nf.IsApplied(), err)
	}
	if cm.Status().PID == oldPID {
		t.Fatal("watchdog did not load generated runtime via Restart")
	}
	if data, err := os.ReadFile(filepath.Join(cfg.Core.RuntimeDir, "mihomo-config.yaml")); err != nil || !strings.Contains(string(data), "tun:") {
		t.Fatalf("fresh runtime not generated: %s %v", data, err)
	}
	data, _ := os.ReadFile(record)
	trace := string(data)
	restore, cleanup, apply := strings.Index(trace, "uci set dhcp.cfgtest.port=5353"), strings.Index(trace, "ip -4 rule del"), strings.LastIndex(trace, "ip -4 rule add")
	if restore < 0 || cleanup <= restore || apply <= cleanup {
		t.Fatalf("not DNS restore -> manager cleanup -> Apply: %s", trace)
	}
	if strings.Contains(trace, "server=127.0.0.1#") {
		t.Fatalf("DNS enabled during DNS-disabled recovery: %s", trace)
	}
}

func TestWatchdogCleanupFailureRetainsOwnerForRetry(t *testing.T) {
	cfg, sm, cm, nf, record := startupLifecycleDeps(t)
	if err := nf.Apply(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CF_FAIL_COMMAND", "ip:-4 rule del")
	if ready, err := recoverStartupTakeover(context.Background(), cfg, sm, cm, nf); err == nil || ready {
		t.Fatalf("cleanup failure accepted: %t %v", ready, err)
	}
	if nf.IsApplied() {
		t.Fatal("failed Cleanup remained applied")
	}
	// IsApplied is now false, but the same manager must still own cleanup retry.
	t.Setenv("CF_FAIL_COMMAND", "")
	if _, err := recoverStartupTakeover(context.Background(), cfg, sm, cm, nf); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(record)
	if strings.Count(string(data), "ip -4 rule del pref 9020") < 2 {
		t.Fatalf("cleanup was not retried on old owner: %s", data)
	}
}

func TestWatchdogFailureNeverSetsUpDNS(t *testing.T) {
	for _, failure := range []string{"generation", "missing-generation", "apply", "restore"} {
		t.Run(failure, func(t *testing.T) {
			cfg, sm, cm, nf, record := startupLifecycleDeps(t)
			cfg.DNS.Enable = true
			switch failure {
			case "generation":
				cfg.Core.RuntimeDir = filepath.Join(t.TempDir(), "not-a-directory")
				if err := os.WriteFile(cfg.Core.RuntimeDir, []byte("occupied"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing-generation":
				sm = nil
			case "apply":
				t.Setenv("CF_FAIL_COMMAND", "ip:-4 link show")
			case "restore":
				if err := dns.Setup(dns.ModeUpstream, cfg.Ports.DNS); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(record, nil, 0600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("CF_FAIL_COMMAND", "uci:commit dhcp")
			}
			ready, err := recoverStartupTakeover(context.Background(), cfg, sm, cm, nf)
			if err == nil || ready {
				t.Fatalf("watchdog failure reported ready: %t %v", ready, err)
			}
			data, _ := os.ReadFile(record)
			if strings.Contains(string(data), "server=127.0.0.1#") {
				t.Fatalf("DNS setup after recovery failure: %s", data)
			}
		})
	}
}

func TestWatchdogAppliesBeforeDNS(t *testing.T) {
	cfg, sm, cm, _, record := startupLifecycleDeps(t)
	cfg.DNS.Enable = true
	nf := netfilter.NewManager(startupNetfilterConfig(cfg))
	ready, err := recoverStartupTakeover(context.Background(), cfg, sm, cm, nf)
	if err != nil || !ready {
		t.Fatalf("valid recovery: DNS=%t %v", ready, err)
	}
	data, _ := os.ReadFile(record)
	trace := string(data)
	apply, setup := strings.LastIndex(trace, "ip -4 rule add"), strings.Index(trace, "uci add_list dhcp.cfgtest.server=")
	// The fixture starts with owned routes already present, so verify the Apply
	// path's final route readback when idempotent insertion was unnecessary.
	if apply < 0 {
		apply = strings.LastIndex(trace, "ip -4 -d rule show")
	}
	if apply < 0 || setup <= apply {
		t.Fatalf("DNS setup did not follow verified Apply: %s", trace)
	}
}
