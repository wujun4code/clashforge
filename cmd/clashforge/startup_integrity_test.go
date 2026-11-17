package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/subscription"
)

// Exercise the actual daemon's generation wrapper, not only runtimecfg's API.
// No core process or network takeover is started by this test.
func TestDaemonGenerationRejectsIncompletePersistedSource(t *testing.T) {
	for _, mode := range []string{"missing-raw", "missing-active", "corrupt-active"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			cfg := config.Default()
			cfg.Core.DataDir = dir
			cfg.Core.RuntimeDir = filepath.Join(dir, "runtime")
			cfg.Core.AutoStartCore = true
			mgr := subscription.NewManager(dir)
			id, err := mgr.Add(subscription.Subscription{Name: "daemon-fixture", Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			cache := filepath.Join(dir, "cache")
			if err := os.MkdirAll(cache, 0700); err != nil {
				t.Fatal(err)
			}
			write := func(path string, data []byte) {
				t.Helper()
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			nodes, err := json.Marshal([]subscription.ProxyNode{{Name: "fixture-node", Type: "ss", Server: "192.0.2.1", Port: 443}})
			if err != nil {
				t.Fatal(err)
			}
			write(filepath.Join(cache, id+".json"), nodes)
			if mode != "missing-raw" {
				write(filepath.Join(cache, id+".raw.yaml"), []byte("proxies:\n - {name: fixture-node, type: ss, server: 192.0.2.1, port: 443}\nrules: [MATCH,DIRECT]\n"))
			}
			if mode != "missing-active" {
				active, err := json.Marshal(map[string]string{"type": "subscription", "sub_id": id})
				if err != nil {
					t.Fatal(err)
				}
				if mode == "corrupt-active" {
					active = []byte("{broken")
				}
				write(filepath.Join(dir, "active_source.json"), active)
			}
			if err := os.MkdirAll(cfg.Core.RuntimeDir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(cfg.Core.RuntimeDir, "mihomo-config.yaml")
			previous := []byte("# existing runtime must not be replaced by a template\n")
			write(path, previous)
			if err := writeRuntimeMihomoConfig(cfg, mgr); err == nil {
				t.Fatal("daemon must receive a generation error for incomplete persisted input")
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != string(previous) {
				t.Fatalf("daemon generation changed the previous runtime: %v", err)
			}
		})
	}
}
