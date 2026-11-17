package api

import (
	"context"
	"encoding/json"
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
	"github.com/wujun4code/clashforge/internal/netfilter"
	"github.com/wujun4code/clashforge/internal/subscription"
)

func lifecycleDeps(t *testing.T) Dependencies {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Core.DataDir = dir
	cfg.Core.RuntimeDir = filepath.Join(dir, "run")
	cfg.DNS.Enable = false
	cfg.Network.Mode = "tun"
	cfg.Network.FirewallBackend = "nftables"
	cfg.Network.IPv6 = false
	cfg.Network.ApplyOnStart = true
	// A harmless child, scoped to this test; never run a real proxy binary.
	cfg.Core.Binary = filepath.Join(dir, "fake-core")
	if err := os.WriteFile(cfg.Core.Binary, []byte("#!/bin/sh\nexec /bin/sleep 60\n"), 0755); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"version":"test"}`)) }))
	t.Cleanup(srv.Close)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	cfg.Ports.MihomoAPI, _ = strconv.Atoi(port)
	cm := core.NewManager(core.CoreManagerConfig{Binary: cfg.Core.Binary, ConfigFile: filepath.Join(cfg.Core.RuntimeDir, "mihomo-config.yaml"), APIPort: cfg.Ports.MihomoAPI})
	t.Cleanup(func() { _ = cm.Stop() })
	for _, gf := range defaultGeodataSpecs() {
		if err := os.WriteFile(filepath.Join(dir, gf.Filename), []byte("fixture"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return Dependencies{Config: cfg, ConfigPath: filepath.Join(dir, "config.toml"), Core: cm, Netfilter: netfilter.NewManager(netfilter.Config{Mode: "none", FirewallBackend: "none"}), SubManager: subscription.NewManager(dir)}
}

func callLifecycle(t *testing.T, h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
	return w
}

func sseDone(t *testing.T, w *httptest.ResponseRecorder) launchEvent {
	t.Helper()
	var done launchEvent
	found := false
	for _, line := range strings.Split(w.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev launchEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			t.Fatal(err)
		}
		if ev.Type == "done" {
			done = ev
			found = true
		}
		if ev.Step == "proxy-takeover" && ev.Status == "ok" && strings.Contains(w.Body.String(), `"step":"proxy-takeover","status":"error"`) {
			t.Fatalf("false successful takeover event: %s", w.Body.String())
		}
	}
	if !found {
		t.Fatalf("missing done: %s", w.Body.String())
	}
	return done
}

func TestCoreStartHTTPRejectsTakeoverError(t *testing.T) {
	record := mockLifecycleCommands(t)
	deps := lifecycleDeps(t)
	t.Setenv("CF_FAIL_COMMAND", "ip:-4 link")
	w := callLifecycle(t, handleCoreStart(deps), "{}")
	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "TAKEOVER_FAILED") {
		t.Fatalf("status=%d: %s", w.Code, w.Body.String())
	}
	data, _ := os.ReadFile(record)
	if strings.Contains(string(data), "uci ") {
		t.Fatalf("DNS changed after failed apply: %s", data)
	}
}

func TestLaunchAndApplyRejectTUNFailure(t *testing.T) {
	for _, which := range []string{"setup", "apply"} {
		t.Run(which, func(t *testing.T) {
			record := mockLifecycleCommands(t)
			deps := lifecycleDeps(t)
			t.Setenv("CF_FAIL_COMMAND", "ip:-4 link")
			body := `{"dns":{"enable":false,"dnsmasq_mode":"none","apply_on_start":false},"network":{"mode":"tun","firewall_backend":"nftables","apply_on_start":true}}`
			h := handleSetupLaunch(deps)
			if which == "apply" {
				h = handleCoreApply(deps)
			}
			w := callLifecycle(t, h, body)
			done := sseDone(t, w)
			if done.Success || !strings.Contains(w.Body.String(), `"step":"proxy-takeover","status":"error"`) {
				t.Fatalf("takeover failure not surfaced: %s", w.Body.String())
			}
			data, _ := os.ReadFile(record)
			if strings.Contains(string(data), "uci ") {
				t.Fatalf("DNS changed after failed apply: %s", data)
			}
		})
	}
}

func TestRefreshCleanupFailurePreservesOwnerForRetry(t *testing.T) {
	record := mockLifecycleCommands(t)
	deps := lifecycleDeps(t)
	if err := applyNetworkTakeover(deps); err != nil {
		t.Fatal(err)
	}
	deps.Config.Network.Mode = "none"
	t.Setenv("CF_FAIL_COMMAND", "nft:delete rule inet fw4")
	if err := refreshNetfilterManager(deps); err == nil {
		t.Fatal("cleanup failure ignored")
	}
	t.Setenv("CF_FAIL_COMMAND", "")
	if err := os.WriteFile(record, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := refreshNetfilterManager(deps); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(record)
	if !strings.Contains(string(data), "delete rule inet fw4 forward_lan handle 77") {
		t.Fatalf("old owner lost on failure: %s", data)
	}
}

func TestSetupStopUsesDNSFirstTUNCleanup(t *testing.T) {
	record := mockLifecycleCommands(t)
	deps := lifecycleDeps(t)
	if err := applyNetworkTakeover(deps); err != nil {
		t.Fatal(err)
	}
	mockLifecycleOwnedDNS(t, deps.Config.Ports.DNS)
	if err := os.WriteFile(record, nil, 0644); err != nil {
		t.Fatal(err)
	}
	w := callLifecycle(t, handleSetupStop(deps), "{}")
	if !sseDone(t, w).Success {
		t.Fatal(w.Body.String())
	}
	data, _ := os.ReadFile(record)
	commands := string(data)
	dnsIndex := strings.Index(commands, "service dnsmasq restart")
	cleanupIndex := strings.Index(commands, "rule del pref 9021")
	if dnsIndex < 0 || cleanupIndex < dnsIndex || !strings.Contains(commands, "delete rule inet fw4 forward_lan handle 77") {
		t.Fatalf("DNS-first TUN cleanup missing: %s", commands)
	}
	if deps.Netfilter.IsApplied() {
		t.Fatal("stopped manager still applied")
	}
}

func TestSetupStopCleanupFailureIsNotSuccess(t *testing.T) {
	mockLifecycleCommands(t)
	deps := lifecycleDeps(t)
	if err := applyNetworkTakeover(deps); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CF_FAIL_COMMAND", "nft:delete rule inet fw4")
	w := callLifecycle(t, handleSetupStop(deps), "{}")
	if sseDone(t, w).Success {
		t.Fatal("cleanup failure reported as success")
	}
}

func TestModeTransitionsCleanTUNAndDisableTakeover(t *testing.T) {
	mockLifecycleCommands(t)
	deps := lifecycleDeps(t)
	for _, mode := range []string{"tun", "tproxy", "tun", "none", "tun"} {
		deps.Config.Network.Mode = mode
		if err := applyNetworkTakeover(deps); err != nil {
			t.Fatalf("mode %s: %v", mode, err)
		}
	}
	deps.Config.Network.ApplyOnStart = false
	if err := applyNetworkTakeover(deps); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"forward", "9020", "9021"} {
		data, _ := os.ReadFile(filepath.Join(os.Getenv("CF_MOCK_STATE_DIR"), state))
		if strings.TrimSpace(string(data)) != "0" {
			t.Fatalf("%s remains after disabled takeover", state)
		}
	}
}

func TestRepeatedStartAndLaunchRestartPreparedCore(t *testing.T) {
	for _, which := range []string{"start", "restart", "setup", "apply"} {
		t.Run(which, func(t *testing.T) {
			mockLifecycleCommands(t)
			deps := lifecycleDeps(t)
			if _, err := generateMihomoConfig(deps); err != nil {
				t.Fatal(err)
			}
			if err := deps.Core.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			oldPID := deps.Core.Status().PID
			body := `{"dns":{"enable":false,"dnsmasq_mode":"none","apply_on_start":false},"network":{"mode":"tun","firewall_backend":"nftables","apply_on_start":true}}`
			var h http.HandlerFunc
			switch which {
			case "start":
				h = handleCoreStart(deps)
			case "restart":
				h = handleCoreRestart(deps)
			case "setup":
				h = handleSetupLaunch(deps)
			default:
				h = handleCoreApply(deps)
			}
			w := callLifecycle(t, h, body)
			if which == "setup" || which == "apply" {
				if !sseDone(t, w).Success {
					t.Fatal(w.Body.String())
				}
			} else if w.Code != http.StatusOK {
				t.Fatalf("%d: %s", w.Code, w.Body.String())
			}
			if deps.Core.Status().PID == oldPID {
				t.Fatal("already-running core did not load prepared config")
			}
		})
	}
}

func TestUnsupportedTUNReplaceDoesNotMutateActiveConfig(t *testing.T) {
	for _, which := range []string{"setup", "apply", "update", "dns-takeover"} {
		t.Run(which, func(t *testing.T) {
			record := mockLifecycleCommands(t)
			deps := lifecycleDeps(t)
			deps.Config.DNS.Enable = true
			deps.Config.DNS.DnsmasqMode = "upstream"
			body := `{"dns":{"enable":true,"dnsmasq_mode":"replace","apply_on_start":true},"network":{"mode":"tun","firewall_backend":"nftables","apply_on_start":true}}`
			switch which {
			case "dns-takeover":
				if _, _, err := takeoverDNS(deps, "replace"); err == nil || !strings.Contains(err.Error(), "upstream") {
					t.Fatalf("unsupported error: %v", err)
				}
			case "update":
				w := callLifecycle(t, handleUpdateConfig(deps), body)
				if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "upstream") {
					t.Fatalf("%d: %s", w.Code, w.Body.String())
				}
			default:
				h := handleSetupLaunch(deps)
				if which == "apply" {
					h = handleCoreApply(deps)
				}
				w := callLifecycle(t, h, body)
				if sseDone(t, w).Success || !strings.Contains(w.Body.String(), "upstream") {
					t.Fatal(w.Body.String())
				}
			}
			if deps.Config.DNS.DnsmasqMode != "upstream" {
				t.Fatal("invalid request changed active config")
			}
			data, _ := os.ReadFile(record)
			if strings.Contains(string(data), "uci ") || strings.Contains(string(data), "delete rule") {
				t.Fatalf("invalid request mutated network: %s", data)
			}
		})
	}
}

func TestGenerationFailureKeepsActiveTakeover(t *testing.T) {
	for _, which := range []string{"write failure", "not generated"} {
		t.Run(which, func(t *testing.T) {
			record := mockLifecycleCommands(t)
			deps := lifecycleDeps(t)
			if err := applyNetworkTakeover(deps); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(record, nil, 0644); err != nil {
				t.Fatal(err)
			}
			if which == "not generated" {
				deps.SubManager = nil
			} else {
				block := filepath.Join(deps.Config.Core.DataDir, "block")
				if err := os.WriteFile(block, []byte("file"), 0644); err != nil {
					t.Fatal(err)
				}
				deps.Config.Core.RuntimeDir = filepath.Join(block, "child")
			}
			w := callLifecycle(t, handleCoreStart(deps), "{}")
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("%d: %s", w.Code, w.Body.String())
			}
			if !deps.Netfilter.IsApplied() {
				t.Fatal("unprepared config retired active manager")
			}
			data, _ := os.ReadFile(record)
			if len(data) > 0 {
				t.Fatalf("generation failure mutated network: %s", data)
			}
		})
	}
}

func TestConfigSaveRetainsActiveManager(t *testing.T) {
	record := mockLifecycleCommands(t)
	deps := lifecycleDeps(t)
	if err := applyNetworkTakeover(deps); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record, nil, 0644); err != nil {
		t.Fatal(err)
	}
	w := callLifecycle(t, handleUpdateConfig(deps), `{"network":{"mode":"tproxy"}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	if !deps.Netfilter.IsApplied() {
		t.Fatal("config-save discarded active manager")
	}
	data, _ := os.ReadFile(record)
	if strings.Contains(string(data), "delete rule") || strings.Contains(string(data), "rule del") {
		t.Fatalf("config-save destroyed active takeover: %s", data)
	}
	if err := refreshNetfilterManager(deps); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(record)
	if !strings.Contains(string(data), "delete rule inet fw4 forward_lan handle 77") {
		t.Fatalf("old TUN ownership lost: %s", data)
	}
}
