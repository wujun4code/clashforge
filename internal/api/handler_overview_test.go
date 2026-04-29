package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
)

func writeSizedFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	buf := make([]byte, size)
	for i := range buf {
		buf[i] = 'x'
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestBuildAppStorageCountsRulesOutsideRuntimeRuleProvider(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "runtime")
	dataDir := filepath.Join(root, "data")
	binaryPath := filepath.Join(root, "bin", "clashforge")
	geoIPPath := filepath.Join(root, "assets", "Country.mmdb")
	geositePath := filepath.Join(root, "assets", "geosite.dat")

	writeSizedFile(t, filepath.Join(runtimeDir, "state", "runtime.db"), 1024)
	writeSizedFile(t, filepath.Join(dataDir, "device-groups.json"), 2048)
	writeSizedFile(t, binaryPath, 4096)
	writeSizedFile(t, geoIPPath, 3072)
	writeSizedFile(t, geositePath, 5120)

	// Simulate rule assets produced outside runtime/rule_provider.
	dataRulesetFile := filepath.Join(dataDir, "ruleset", "private", "streaming.mrs")
	writeSizedFile(t, dataRulesetFile, 256*1024)

	deps := Dependencies{
		Config: &config.MetaclashConfig{Core: config.CoreConfig{
			RuntimeDir:  runtimeDir,
			DataDir:     dataDir,
			Binary:      binaryPath,
			GeoIPPath:   geoIPPath,
			GeositePath: geositePath,
		}},
	}

	app := buildAppStorage(deps)
	if app.RulesMB <= 0 {
		t.Fatalf("expected rules size > 0, got %.2f MB", app.RulesMB)
	}

	foundRuleset := false
	for _, asset := range app.RuleAssets {
		if asset.Name == "Data Ruleset" {
			foundRuleset = true
			if asset.SizeMB <= 0 {
				t.Fatalf("expected Data Ruleset size > 0, got %.2f MB", asset.SizeMB)
			}
		}
	}
	if !foundRuleset {
		t.Fatalf("expected Data Ruleset asset to be present")
	}
}
