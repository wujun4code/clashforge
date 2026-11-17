package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
)

// Reuse the exact registered closure after each early-return boundary.
func TestQualitySSECandidateRetryUpdatesAuthoritativeConfig(t *testing.T) {
	for _, which := range []string{"setup", "apply"} {
		for _, failure := range []string{"validation", "save"} {
			t.Run(which+"/"+failure, func(t *testing.T) {
				record := mockLifecycleCommands(t)
				deps := lifecycleDeps(t)
				deps.Config.DNS.Enable = true
				deps.Config.DNS.DnsmasqMode = "upstream"
				deps.Config.DNS.Strategy = config.DNSStrategyAntiHijack
				original := *deps.Config
				if err := config.Save(deps.ConfigPath, deps.Config); err != nil {
					t.Fatal(err)
				}
				baseline, err := os.ReadFile(deps.ConfigPath)
				if err != nil {
					t.Fatal(err)
				}
				h := handleSetupLaunch(deps)
				if which == "apply" {
					h = handleCoreApply(deps)
				}
				bad := `{"dns":{"enable":true,"dnsmasq_mode":"replace","apply_on_start":true},"network":{"mode":"tun","firewall_backend":"nftables","apply_on_start":true}}`
				if failure == "save" {
					// A directory at the destination reliably fails Save even when run as root.
					if err := os.Remove(deps.ConfigPath); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(deps.ConfigPath, 0700); err != nil {
						t.Fatal(err)
					}
					bad = `{"dns":{"enable":false,"dnsmasq_mode":"none","apply_on_start":false},"network":{"mode":"tun","firewall_backend":"nftables","apply_on_start":true}}`
				}
				w := callLifecycle(t, h, bad)
				if sseDone(t, w).Success || !strings.Contains(w.Body.String(), `"step":"config-save","status":"error"`) {
					t.Fatalf("expected config rejection: %s", w.Body.String())
				}
				if !reflect.DeepEqual(*deps.Config, original) {
					t.Fatal("rejection mutated authoritative configuration")
				}
				if deps.Core.Status().Ready || deps.Netfilter.IsApplied() {
					t.Fatal("rejection activated runtime")
				}
				if failure == "save" {
					if err := os.Remove(deps.ConfigPath); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(deps.ConfigPath, baseline, 0600); err != nil {
						t.Fatal(err)
					}
				}
				unchanged, err := os.ReadFile(deps.ConfigPath)
				if err != nil {
					t.Fatal(err)
				}
				if string(unchanged) != string(baseline) {
					t.Fatal("rejection changed saved TOML")
				}
				good := `{"dns":{"enable":false,"dnsmasq_mode":"none","apply_on_start":false},"network":{"mode":"none","firewall_backend":"nftables","apply_on_start":false}}`
				w = callLifecycle(t, h, good)
				if !sseDone(t, w).Success {
					t.Fatalf("valid retry: %s", w.Body.String())
				}
				saved, err := config.Load(deps.ConfigPath)
				if err != nil {
					t.Fatal(err)
				}
				if saved.Network.Mode != "none" || saved.DNS.Enable || saved.Network.ApplyOnStart || saved.DNS.ApplyOnStart {
					t.Fatalf("retry persisted wrong policy: network=%+v DNS enable=%t", saved.Network, saved.DNS.Enable)
				}
				// This diagnostic field is deliberately excluded from TOML.
				saved.Network.WANInterfaceAutoDetected = deps.Config.Network.WANInterfaceAutoDetected
				if !reflect.DeepEqual(*deps.Config, *saved) {
					t.Fatalf("retry detached authoritative Config: shared mode=%s DNS=%t; saved mode=%s DNS=%t", deps.Config.Network.Mode, deps.Config.DNS.Enable, saved.Network.Mode, saved.DNS.Enable)
				}
				if deps.Config.DNS.Strategy != original.DNS.Strategy {
					t.Fatal("retry changed anti-hijack preference")
				}
				if !deps.Core.Status().Ready || deps.Netfilter.IsApplied() {
					t.Fatal("retry activation does not match saved proxy-only policy")
				}
				data, _ := os.ReadFile(record)
				if strings.Contains(string(data), "uci ") || strings.Contains(string(data), "nft -f") {
					t.Fatalf("proxy-only retry acquired takeover: %s", data)
				}
			})
		}
	}
}

func qualityStopHandler(deps Dependencies, which string) http.HandlerFunc {
	if which == "sse" {
		return handleSetupStop(deps)
	}
	return handleCoreStop(deps)
}

func qualityStopSucceeded(t *testing.T, w *httptest.ResponseRecorder, which string) bool {
	t.Helper()
	if which == "sse" {
		return sseDone(t, w).Success
	}
	return w.Code == http.StatusOK
}

func qualityStartOwnedCore(t *testing.T, deps Dependencies) {
	t.Helper()
	if err := applyNetworkTakeover(deps); err != nil {
		t.Fatal(err)
	}
	if _, err := generateMihomoConfig(deps); err != nil {
		t.Fatal(err)
	}
	if err := deps.Core.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func qualityAssertTUNClean(t *testing.T) {
	t.Helper()
	for _, name := range []string{"forward", "9020", "9021"} {
		data, err := os.ReadFile(filepath.Join(os.Getenv("CF_MOCK_STATE_DIR"), name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(data)) != "0" {
			t.Fatalf("owned %s remains: %s", name, data)
		}
	}
}

// Missing never-owned tools must not be confused with failure of the owner.
func TestQualityStopSkipsNeverOwnedMissingBackend(t *testing.T) {
	for _, backend := range []string{"nftables", "iptables", "none"} {
		for _, which := range []string{"http", "sse"} {
			t.Run(backend+"/"+which, func(t *testing.T) {
				record := mockLifecycleCommands(t)
				deps := lifecycleDeps(t)
				missing := []string{"iptables", "ip6tables"}
				if backend == "iptables" {
					deps.Config.Network.Mode = "tproxy"
					deps.Config.Network.FirewallBackend = "iptables"
					missing = []string{"nft"}
				}
				if backend == "none" {
					deps.Config.Network.Mode = "none"
					deps.Config.Network.FirewallBackend = "none"
					deps.Config.Network.ApplyOnStart = false
					missing = []string{"nft", "iptables", "ip6tables", "ip"}
				}
				// Install a single-backend PATH before acquiring any resources.
				for _, name := range missing {
					if err := os.Remove(filepath.Join(os.Getenv("CF_MOCK_STATE_DIR"), name)); err != nil {
						t.Fatal(err)
					}
				}
				qualityStartOwnedCore(t, deps)
				if err := os.WriteFile(record, nil, 0600); err != nil {
					t.Fatal(err)
				}
				w := callLifecycle(t, qualityStopHandler(deps, which), "{}")
				if !qualityStopSucceeded(t, w, which) {
					t.Fatalf("never-owned absent backend blocked stop: %s", w.Body.String())
				}
				if deps.Core.Status().Ready || deps.Netfilter.IsApplied() {
					t.Fatal("stop left core or takeover active")
				}
				if backend == "nftables" {
					qualityAssertTUNClean(t)
				}
				data, _ := os.ReadFile(record)
				if strings.Contains(string(data), "uci ") || strings.Contains(string(data), "service dnsmasq") {
					t.Fatalf("unowned DNS mutated: %s", data)
				}
				if backend == "none" && len(data) != 0 {
					t.Fatalf("proxy-only stop swept foreign resources: %s", data)
				}
			})
		}
	}
}

func TestQualityStopOwnerFailureRemainsFatalAndRetryable(t *testing.T) {
	for _, backend := range []string{"nftables", "iptables"} {
		for _, which := range []string{"http", "sse"} {
			for _, failure := range []string{"missing", "permission", "busy"} {
				t.Run(backend+"/"+which+"/"+failure, func(t *testing.T) {
					record := mockLifecycleCommands(t)
					deps := lifecycleDeps(t)
					owner := "nft"
					if backend == "iptables" {
						owner = "iptables"
						deps.Config.Network.Mode = "tproxy"
						deps.Config.Network.FirewallBackend = "iptables"
					}
					qualityStartOwnedCore(t, deps)
					// Acquire real typed DNS ownership before expecting any Restore commands.
					mockLifecycleOwnedDNS(t, deps.Config.Ports.DNS)
					ownerPath := filepath.Join(os.Getenv("CF_MOCK_STATE_DIR"), owner)
					original, err := os.ReadFile(ownerPath)
					if err != nil {
						t.Fatal(err)
					}
					if failure == "missing" {
						if err := os.Remove(ownerPath); err != nil {
							t.Fatal(err)
						}
					} else {
						diagnostic := "Permission denied"
						if failure == "busy" {
							diagnostic = "Device or resource busy"
						}
						script := "#!/bin/sh\nprintf '%s\\n' '" + diagnostic + "' >&2\nexit 1\n"
						if err := os.WriteFile(ownerPath, []byte(script), 0700); err != nil {
							t.Fatal(err)
						}
					}
					h := qualityStopHandler(deps, which)
					w := callLifecycle(t, h, "{}")
					if qualityStopSucceeded(t, w, which) || !deps.Core.Status().Ready {
						t.Fatalf("owner failure incorrectly stopped core: %s", w.Body.String())
					}
					if which == "http" && w.Code != http.StatusInternalServerError {
						t.Fatalf("owner failure status %d", w.Code)
					}
					if err := os.WriteFile(ownerPath, original, 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(record, nil, 0600); err != nil {
						t.Fatal(err)
					}
					w = callLifecycle(t, h, "{}")
					if !qualityStopSucceeded(t, w, which) || deps.Core.Status().Ready || deps.Netfilter.IsApplied() {
						t.Fatalf("owner cleanup retry failed: %s", w.Body.String())
					}
					data, _ := os.ReadFile(record)
					if backend == "nftables" {
						qualityAssertTUNClean(t)
						if !strings.Contains(string(data), "delete rule inet fw4") {
							t.Fatalf("lost nft owner on failed cleanup: %s", data)
						}
					} else if !strings.Contains(string(data), "iptables -t mangle -F METACLASH") {
						t.Fatalf("lost iptables owner on failed cleanup: %s", data)
					}
					state, err := os.ReadFile(filepath.Join(os.Getenv("CF_MOCK_STATE_DIR"), "uci-state.json"))
					if err != nil {
						t.Fatal(err)
					}
					var restored map[string]any
					if err := json.Unmarshal(state, &restored); err != nil {
						t.Fatal(err)
					}
					server, _ := json.Marshal(restored["server"])
					if string(server) != `["list",["192.0.2.53","/test.invalid/192.0.2.54"]]` {
						t.Fatalf("DNS original not restored: %s", server)
					}
				})
			}
		}
	}
}
