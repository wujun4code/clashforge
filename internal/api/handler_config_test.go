package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/subscription"
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

	if err := writeActiveSource(dataDir, ActiveSource{Type: "subscription", SubID: idB, SubName: "test-01"}); err != nil {
		t.Fatalf("write active source: %v", err)
	}

	deps := Dependencies{
		Config: &config.MetaclashConfig{
			Core: config.CoreConfig{DataDir: dataDir},
		},
		SubManager: subMgr,
	}

	nodes, rawYAMLs := resolveGenerationInputs(deps)
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

	if err := writeActiveSource(dataDir, ActiveSource{Type: "file", Filename: "20260429_v1.yaml"}); err != nil {
		t.Fatalf("write active source: %v", err)
	}

	deps := Dependencies{
		Config: &config.MetaclashConfig{
			Core: config.CoreConfig{DataDir: dataDir},
		},
		SubManager: subMgr,
	}

	nodes, rawYAMLs := resolveGenerationInputs(deps)
	if len(nodes) != 2 {
		t.Fatalf("expected nodes from enabled subscriptions to remain available, got %d", len(nodes))
	}
	if len(rawYAMLs) != 0 {
		t.Fatalf("expected subscription raw YAML base to be disabled for file source, got %d", len(rawYAMLs))
	}
}
