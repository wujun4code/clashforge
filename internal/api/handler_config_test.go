package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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
