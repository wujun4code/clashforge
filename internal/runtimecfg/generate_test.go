package runtimecfg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/subscription"
	"gopkg.in/yaml.v3"
)

func writeSubCache(t *testing.T, dataDir, subID string, nodes []subscription.ProxyNode, rawYAML string) {
	t.Helper()
	cacheDir := filepath.Join(dataDir, "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	nodeData, err := json.Marshal(nodes)
	if err != nil {
		t.Fatalf("marshal nodes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, subID+".json"), nodeData, 0o644); err != nil {
		t.Fatalf("write node cache: %v", err)
	}
	if rawYAML != "" {
		if err := os.WriteFile(filepath.Join(cacheDir, subID+".raw.yaml"), []byte(rawYAML), 0o644); err != nil {
			t.Fatalf("write raw cache: %v", err)
		}
	}
}

func TestGenerateAndWritePreservesUserDNSAndRejectsPlaintextInStrictMode(t *testing.T) {
	dataDir := t.TempDir()
	cfg := config.Default()
	cfg.Core.DataDir = dataDir
	cfg.Core.RuntimeDir = filepath.Join(dataDir, "runtime")
	subMgr := subscription.NewManager(dataDir)
	subID, err := subMgr.Add(subscription.Subscription{Name: "dns-test", URL: "https://example.com/sub", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	writeSubCache(t, dataDir, subID, nil, "dns:\n  default-nameserver: [tcp://192.0.2.1]\n  proxy-server-nameserver: [https://nodes.example/dns-query#DIRECT]\n")
	writeActiveSourceJSON(t, dataDir, activeSource{Type: "subscription", SubID: subID})
	overridesPath := filepath.Join(dataDir, "overrides.yaml")
	if err := os.WriteFile(overridesPath, []byte("dns:\n  default-nameserver: [tcp://192.0.2.2]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if wrote, err := GenerateAndWrite(cfg, subMgr); err != nil || !wrote {
		t.Fatalf("generate: wrote=%v, err=%v", wrote, err)
	}
	runtimePath := filepath.Join(cfg.Core.RuntimeDir, "mihomo-config.yaml")
	before, err := os.ReadFile(runtimePath)
	if err != nil {
		t.Fatal(err)
	}
	var generated map[string]interface{}
	if err := yaml.Unmarshal(before, &generated); err != nil {
		t.Fatal(err)
	}
	dns := generated["dns"].(map[string]interface{})
	bootstrap := dns["default-nameserver"].([]interface{})
	if len(bootstrap) != 1 || bootstrap[0] != "tcp://192.0.2.2" {
		t.Fatalf("overrides must replace subscription and managed bootstrap: %v", bootstrap)
	}
	if !strings.Contains(string(before), "https://nodes.example/dns-query#DIRECT") {
		t.Fatal("subscription's node resolver was discarded")
	}
	cfg.DNS.Strategy = config.DNSStrategyAntiHijack
	if wrote, err := GenerateAndWrite(cfg, subMgr); err == nil || wrote {
		t.Fatalf("strict mode must reject plaintext before writing: wrote=%v, err=%v", wrote, err)
	}
	after, err := os.ReadFile(runtimePath)
	if err != nil || string(after) != string(before) {
		t.Fatalf("invalid strict configuration overwrote working runtime: %v", err)
	}
	if err := os.WriteFile(overridesPath, []byte("dns:\n  default-nameserver: [https://192.0.2.2/dns-query#DIRECT]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if wrote, err := GenerateAndWrite(cfg, subMgr); err != nil || !wrote {
		t.Fatalf("valid encrypted overrides must replace insecure subscription DNS: wrote=%v, err=%v", wrote, err)
	}
}

func writeActiveSourceJSON(t *testing.T, dataDir string, as activeSource) {
	t.Helper()
	data, err := json.Marshal(as)
	if err != nil {
		t.Fatalf("marshal active source: %v", err)
	}
	if err := os.WriteFile(activeSourceFilePath(dataDir), data, 0o644); err != nil {
		t.Fatalf("write active source: %v", err)
	}
}

func TestResolveGenerationInputsUsesActiveSubscriptionOnly(t *testing.T) {
	dataDir := t.TempDir()
	subMgr := subscription.NewManager(dataDir)

	idA, err := subMgr.Add(subscription.Subscription{Name: "store", URL: "https://store.example/sub", Enabled: true})
	if err != nil {
		t.Fatalf("add sub A: %v", err)
	}
	idB, err := subMgr.Add(subscription.Subscription{Name: "test-01", URL: "https://test.example/sub", Enabled: true})
	if err != nil {
		t.Fatalf("add sub B: %v", err)
	}

	writeSubCache(t, dataDir, idA, []subscription.ProxyNode{
		{Name: "store-node", Type: "ss", Server: "store.youzhuzhu.com", Port: 443},
	}, "proxies:\n  - name: store-node\n")
	writeSubCache(t, dataDir, idB, []subscription.ProxyNode{
		{Name: "test-node", Type: "ss", Server: "test-01.example.com", Port: 443},
	}, "proxies:\n  - name: test-node\n")

	writeActiveSourceJSON(t, dataDir, activeSource{Type: "subscription", SubID: idB})

	cfg := &config.MetaclashConfig{Core: config.CoreConfig{DataDir: dataDir}}
	nodes, rawYAMLs := resolveGenerationInputs(cfg, subMgr)

	if len(nodes) != 1 {
		t.Fatalf("expected 1 active-sub node, got %d", len(nodes))
	}
	if nodes[0].Name != "test-node" {
		t.Fatalf("expected active sub node 'test-node', got %q", nodes[0].Name)
	}
	if nodes[0].SourceSubID != idB {
		t.Fatalf("expected SourceSubID=%s, got %s", idB, nodes[0].SourceSubID)
	}
	if len(rawYAMLs) != 1 {
		t.Fatalf("expected 1 raw YAML from active sub, got %d", len(rawYAMLs))
	}
	if !strings.Contains(string(rawYAMLs[0]), "test-node") {
		t.Fatalf("expected active sub raw YAML content, got: %s", string(rawYAMLs[0]))
	}
}

func TestResolveGenerationInputsFileSourceDisablesSubscriptionRawBase(t *testing.T) {
	dataDir := t.TempDir()
	subMgr := subscription.NewManager(dataDir)

	idA, err := subMgr.Add(subscription.Subscription{Name: "store", URL: "https://store.example/sub", Enabled: true})
	if err != nil {
		t.Fatalf("add sub A: %v", err)
	}
	idB, err := subMgr.Add(subscription.Subscription{Name: "test-01", URL: "https://test.example/sub", Enabled: true})
	if err != nil {
		t.Fatalf("add sub B: %v", err)
	}

	writeSubCache(t, dataDir, idA, []subscription.ProxyNode{
		{Name: "store-node", Type: "ss", Server: "store.youzhuzhu.com", Port: 443},
	}, "proxies:\n  - name: store-node\n")
	writeSubCache(t, dataDir, idB, []subscription.ProxyNode{
		{Name: "test-node", Type: "ss", Server: "test-01.example.com", Port: 443},
	}, "proxies:\n  - name: test-node\n")

	writeActiveSourceJSON(t, dataDir, activeSource{Type: "file", Filename: "20260429_v1.yaml"})

	cfg := &config.MetaclashConfig{Core: config.CoreConfig{DataDir: dataDir}}
	nodes, rawYAMLs := resolveGenerationInputs(cfg, subMgr)

	if len(nodes) != 2 {
		t.Fatalf("expected nodes from enabled subscriptions to remain available, got %d", len(nodes))
	}
	if len(rawYAMLs) != 0 {
		t.Fatalf("expected subscription raw YAML base to be disabled for file source, got %d", len(rawYAMLs))
	}
}
