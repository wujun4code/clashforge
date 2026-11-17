package runtimecfg

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/subscription"
)

func TestColdBootRejectsIncompletePersistentInputsBeforeWrites(t *testing.T) {
	for _, damage := range []string{"active-missing", "active-corrupt", "active-invalid", "raw-missing", "raw-corrupt", "cache-missing", "nodes-corrupt", "overrides-corrupt", "overrides-unreadable", "device-corrupt", "file-missing", "file-corrupt", "file-empty", "file-comment", "file-sequence", "file-null", "overrides-sequence", "overrides-null", "unselected-custom", "unselected-device"} {
		t.Run(damage, func(t *testing.T) {
			dir := t.TempDir()
			cfg := config.Default()
			cfg.Core.DataDir = dir
			cfg.Core.RuntimeDir = filepath.Join(dir, "runtime")
			m := subscription.NewManager(dir)
			id, err := m.Add(subscription.Subscription{Name: "fixture", Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			writeSubCache(t, dir, id, []subscription.ProxyNode{{Name: "node", Type: "ss", Server: "192.0.2.1", Port: 443}}, "proxies:\n - {name: node, type: ss, server: 192.0.2.1, port: 443}\nproxy-groups:\n - {name: original, type: select, proxies: [node]}\nrules: [MATCH,original]\n")
			writeActiveSourceJSON(t, dir, activeSource{Type: "subscription", SubID: id})
			if err := config.SaveDeviceGroupsForSource(config.DeviceGroupsPath(dir), []config.DeviceGroup{{ID: "device", Name: "device", Devices: []config.Device{{IP: "192.0.2.2", Prefix: 32}}, Overrides: []config.ProxyGroupOverride{{OriginalGroup: "original", Proxies: []string{"node"}}}}}, "subscription:"+id); err != nil {
				t.Fatal(err)
			}
			runtime := filepath.Join(cfg.Core.RuntimeDir, "mihomo-config.yaml")
			provider := filepath.Join(dir, "rule_provider", "cf-device-group-device.yaml")
			cache := filepath.Join(dir, "cache.db")
			for _, p := range []string{runtime, provider, cache} {
				if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte("sentinel"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			switch damage {
			case "active-missing":
				os.Remove(activeSourceFilePath(dir))
			case "active-corrupt":
				os.WriteFile(activeSourceFilePath(dir), []byte("{"), 0644)
			case "active-invalid":
				os.WriteFile(activeSourceFilePath(dir), []byte(`{"type":"subscription"}`), 0644)
			case "raw-missing":
				os.Remove(filepath.Join(dir, "cache", id+".raw.yaml"))
			case "raw-corrupt":
				os.WriteFile(filepath.Join(dir, "cache", id+".raw.yaml"), []byte("secret-marker: ["), 0644)
			case "cache-missing":
				os.RemoveAll(filepath.Join(dir, "cache"))
			case "nodes-corrupt":
				os.WriteFile(filepath.Join(dir, "cache", id+".json"), []byte("{"), 0644)
			case "overrides-corrupt":
				os.WriteFile(filepath.Join(dir, "overrides.yaml"), []byte("secret-marker: ["), 0644)
			case "overrides-unreadable":
				os.Mkdir(filepath.Join(dir, "overrides.yaml"), 0755)
			case "device-corrupt":
				os.WriteFile(config.DeviceGroupsPath(dir), []byte("{"), 0644)
			case "overrides-sequence", "overrides-null":
				value := "- invalid\n"
				if damage == "overrides-null" {
					value = "null\n"
				}
				os.WriteFile(filepath.Join(dir, "overrides.yaml"), []byte(value), 0644)
			case "unselected-custom", "unselected-device":
				os.Remove(activeSourceFilePath(dir))
				if err := m.Delete(id); err != nil {
					t.Fatal(err)
				}
				if damage == "unselected-custom" {
					os.Remove(config.DeviceGroupsPath(dir))
					os.WriteFile(filepath.Join(dir, "overrides.yaml"), []byte("rules: [MATCH,DIRECT]\n"), 0644)
				}
			case "file-missing", "file-corrupt", "file-empty", "file-comment", "file-sequence", "file-null":
				writeActiveSourceJSON(t, dir, activeSource{Type: "file", Filename: "fixture.yaml"})
				values := map[string]string{"file-corrupt": "secret-marker: [", "file-empty": "", "file-comment": "# overrides\n", "file-sequence": "- invalid\n", "file-null": "null\n"}
				if value, ok := values[damage]; ok {
					os.WriteFile(filepath.Join(dir, "overrides.yaml"), []byte(value), 0644)
				}
			}
			wrote, err := GenerateAndWrite(cfg, m)
			if err == nil || wrote {
				t.Errorf("incomplete persistent input accepted: wrote=%v err=%v", wrote, err)
			}
			if err != nil && strings.Contains(err.Error(), "secret-marker") {
				t.Error("input leaked in diagnostic")
			}
			for _, p := range []string{runtime, provider, cache} {
				got, err := os.ReadFile(p)
				if err != nil || !bytes.Equal(got, []byte("sentinel")) {
					t.Errorf("persistent input failure changed %s", filepath.Base(p))
				}
			}
		})
	}
}

func TestColdBootLegacyFullRestoresDeviceProfile(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Core.DataDir = dir
	cfg.Core.RuntimeDir = filepath.Join(dir, "runtime")
	m := subscription.NewManager(dir)
	id, err := m.Add(subscription.Subscription{Name: "fixture", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	writeSubCache(t, dir, id, []subscription.ProxyNode{{Name: "node", Type: "ss", Server: "192.0.2.1", Port: 443}}, "proxies:\n - {name: node, type: ss, server: 192.0.2.1, port: 443}\nproxy-groups:\n - {name: original, type: select, proxies: [node]}\nrules: [\"MATCH,original\"]\n")
	writeActiveSourceJSON(t, dir, activeSource{Type: "subscription", SubID: id})
	if err := config.SaveDeviceGroupsForSource(config.DeviceGroupsPath(dir), []config.DeviceGroup{{ID: "device", Name: "device", Devices: []config.Device{{IP: "192.0.2.2", Prefix: 32}}, Overrides: []config.ProxyGroupOverride{{OriginalGroup: "original", Proxies: []string{"node"}}}}}, "subscription:"+id); err != nil {
		t.Fatal(err)
	}
	// Reconstruct twice with distinct Manager instances and no tmpfs runtime.
	for i := 0; i < 2; i++ {
		if err := os.RemoveAll(cfg.Core.RuntimeDir); err != nil {
			t.Fatal(err)
		}
		reboot := subscription.NewManager(dir)
		if err := reboot.Load(); err != nil {
			t.Fatal(err)
		}
		if wrote, err := GenerateAndWrite(cfg, reboot); err != nil || !wrote {
			t.Fatalf("cold boot: %v %v", wrote, err)
		}
		data, err := os.ReadFile(filepath.Join(cfg.Core.RuntimeDir, "mihomo-config.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range []string{"original", "device - original", "RULE-SET,cf-device-group-device,device - original,src,no-resolve"} {
			if !strings.Contains(string(data), s) {
				t.Errorf("lost %s", s)
			}
		}
		provider, err := os.ReadFile(filepath.Join(dir, "rule_provider", "cf-device-group-device.yaml"))
		if err != nil || !strings.Contains(string(provider), "192.0.2.2/32") {
			t.Fatalf("provider not restored: %v", err)
		}
	}
}

// The transport is entirely in-process: the sync path must publish a real nodes
// marker, then a new Manager must reconstruct runtime without a raw YAML base.
type coldBootTransport func(*http.Request) (*http.Response, error)

func (f coldBootTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestColdBootOptionalOverridesCompatibility(t *testing.T) {
	for _, source := range []string{"first", "full", "nodes"} {
		for _, shape := range []string{"missing", "empty", "comment"} {
			t.Run(source+"-"+shape, func(t *testing.T) {
				dir := t.TempDir()
				cfg := config.Default()
				cfg.Core.DataDir = dir
				cfg.Core.RuntimeDir = filepath.Join(dir, "runtime")
				m := subscription.NewManager(dir)
				if source != "first" {
					id, err := m.Add(subscription.Subscription{Name: "fixture", URL: "https://offline.invalid/sub", Enabled: true})
					if err != nil {
						t.Fatal(err)
					}
					if source == "full" {
						writeSubCache(t, dir, id, []subscription.ProxyNode{{Name: "full-node", Type: "ss", Server: "192.0.2.1", Port: 443}}, "proxies:\n - {name: full-node, type: ss, server: 192.0.2.1, port: 443}\nrules: [\"MATCH,DIRECT\"]\n")
					} else {
						old := http.DefaultTransport
						t.Cleanup(func() { http.DefaultTransport = old })
						http.DefaultTransport = coldBootTransport(func(*http.Request) (*http.Response, error) {
							return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("- {name: list-node, type: ss, server: 192.0.2.1, port: 443, cipher: aes-128-gcm, password: fixture}\n"))}, nil
						})
						if err := m.SyncUpdate(id); err != nil {
							t.Fatal(err)
						}
						if _, raw, err := m.GenerationInput(id); err != nil || len(raw) != 0 {
							t.Fatalf("node marker: raw=%q err=%v", raw, err)
						}
					}
					writeActiveSourceJSON(t, dir, activeSource{Type: "subscription", SubID: id})
				}
				if shape != "missing" {
					var b []byte
					if shape == "comment" {
						b = []byte("# ClashForge overrides\n")
					}
					if err := os.WriteFile(filepath.Join(dir, "overrides.yaml"), b, 0644); err != nil {
						t.Fatal(err)
					}
				}
				// Exercise persistent cold boot, not just the warm manager's memory.
				for i := 0; i < 2; i++ {
					if err := os.RemoveAll(cfg.Core.RuntimeDir); err != nil {
						t.Fatal(err)
					}
					reboot := subscription.NewManager(dir)
					if err := reboot.Load(); err != nil {
						t.Fatal(err)
					}
					if wrote, err := GenerateAndWrite(cfg, reboot); err != nil || !wrote {
						t.Fatalf("valid optional overrides rejected: wrote=%v err=%v", wrote, err)
					}
					data, err := os.ReadFile(filepath.Join(cfg.Core.RuntimeDir, "mihomo-config.yaml"))
					if err != nil {
						t.Fatal(err)
					}
					if source == "nodes" && !strings.Contains(string(data), "list-node") {
						t.Fatal("node-list lost during runtime cold boot")
					}
					if source == "full" && !strings.Contains(string(data), "full-node") {
						t.Fatal("full source lost during runtime cold boot")
					}
				}
			})
		}
	}
}

func TestColdBootFileIgnoresUnrelatedBrokenCacheAndKeepsExtras(t *testing.T) {
	for _, damage := range []string{"missing", "corrupt"} {
		t.Run(damage, func(t *testing.T) {
			dir := t.TempDir()
			cfg := config.Default()
			cfg.Core.DataDir = dir
			cfg.Core.RuntimeDir = filepath.Join(dir, "runtime")
			m := subscription.NewManager(dir)
			extra, err := m.Add(subscription.Subscription{Name: "extra", URL: "https://offline.invalid/extra", Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			writeSubCache(t, dir, extra, []subscription.ProxyNode{{Name: "extra-node", Type: "ss", Server: "192.0.2.2", Port: 443}}, "")
			bad, err := m.Add(subscription.Subscription{Name: "unrelated", URL: "https://offline.invalid/bad", Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			if damage == "corrupt" {
				if err := os.WriteFile(filepath.Join(dir, "cache", bad+".json"), []byte("{"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			writeActiveSourceJSON(t, dir, activeSource{Type: "file", Filename: "fixture.yaml"})
			if err := os.WriteFile(filepath.Join(dir, "overrides.yaml"), []byte("proxies:\n - {name: file-node, type: ss, server: 192.0.2.1, port: 443}\nproxy-groups:\n - {name: FileGroup, type: select, proxies: [file-node, extra-node]}\nrules: [\"MATCH,FileGroup\"]\n"), 0644); err != nil {
				t.Fatal(err)
			}
			reboot := subscription.NewManager(dir)
			if err := reboot.Load(); err != nil {
				t.Fatal(err)
			}
			if wrote, err := GenerateAndWrite(cfg, reboot); err != nil || !wrote {
				t.Fatalf("independent file rejected: wrote=%v err=%v", wrote, err)
			}
			data, err := os.ReadFile(filepath.Join(cfg.Core.RuntimeDir, "mihomo-config.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range []string{"file-node", "extra-node", "MATCH,FileGroup"} {
				if !strings.Contains(string(data), s) {
					t.Errorf("file generation lost %s", s)
				}
			}
		})
	}
}

func TestColdBootDuplicateRawRejected(t *testing.T) {
	for _, mode := range []string{"legacy", "full"} {
		for _, raw := range []string{"secret-marker: a\nsecret-marker: b\n", "outer:\n  secret-marker: a\n  secret-marker: b\n"} {
			t.Run(mode+"/"+strings.Split(raw, ":")[0], func(t *testing.T) {
				dir := t.TempDir()
				cfg := config.Default()
				cfg.Core.DataDir = dir
				cfg.Core.RuntimeDir = filepath.Join(dir, "runtime")
				m := subscription.NewManager(dir)
				id, err := m.Add(subscription.Subscription{Name: "fixture", Enabled: true})
				if err != nil {
					t.Fatal(err)
				}
				writeSubCache(t, dir, id, []subscription.ProxyNode{{Name: "node", Type: "ss", Server: "192.0.2.1", Port: 443}}, raw)
				writeActiveSourceJSON(t, dir, activeSource{Type: "subscription", SubID: id})
				if mode == "full" {
					nodes, err := os.ReadFile(filepath.Join(dir, "cache", id+".json"))
					if err != nil {
						t.Fatal(err)
					}
					hash := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
					marker, err := json.Marshal(map[string]interface{}{"version": 1, "kind": "full", "nodes_sha256": hash(nodes), "raw_sha256": hash([]byte(raw))})
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(dir, "cache", id+".commit.json"), marker, 0644); err != nil {
						t.Fatal(err)
					}
				}
				paths := []string{filepath.Join(cfg.Core.RuntimeDir, "mihomo-config.yaml"), filepath.Join(dir, "rule_provider", "cf-device-group-device.yaml"), filepath.Join(dir, "cache.db")}
				for _, p := range paths {
					if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(p, []byte("sentinel"), 0644); err != nil {
						t.Fatal(err)
					}
				}
				var logs bytes.Buffer
				old := log.Logger
				log.Logger = zerolog.New(&logs)
				defer func() { log.Logger = old }()
				wrote, err := GenerateAndWrite(cfg, m)
				if wrote || err == nil {
					t.Errorf("duplicate mapping accepted: wrote=%v err=%v", wrote, err)
				}
				if strings.Contains(logs.String(), "secret-marker") || (err != nil && strings.Contains(err.Error(), "secret-marker")) {
					t.Error("raw key leaked")
				}
				for _, p := range paths {
					got, err := os.ReadFile(p)
					if err != nil || string(got) != "sentinel" {
						t.Errorf("changed %s", filepath.Base(p))
					}
				}
			})
		}
	}
}

func TestColdBootFirstInstallAllowed(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Core.DataDir = dir
	cfg.Core.RuntimeDir = filepath.Join(dir, "runtime")
	if wrote, err := GenerateAndWrite(cfg, subscription.NewManager(dir)); err != nil || !wrote {
		t.Fatalf("first install: %v %v", wrote, err)
	}
}
