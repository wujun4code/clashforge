package runtimecfg

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
)

func TestInvalidateStaleFakeIPCache(t *testing.T) {
	newConfigDoc := func(fakeIPRange string) []byte {
		return []byte("dns:\n  fake-ip-range: " + fakeIPRange + "\n")
	}

	t.Run("first run — no previous config, cache untouched", func(t *testing.T) {
		dir := t.TempDir()
		cfg := &config.MetaclashConfig{Core: config.CoreConfig{DataDir: dir}}
		cachePath := filepath.Join(dir, "cache.db")
		if err := os.WriteFile(cachePath, []byte("stub"), 0o644); err != nil {
			t.Fatalf("seed cache.db: %v", err)
		}
		outPath := filepath.Join(dir, "mihomo-config.yaml") // does not exist yet

		invalidateStaleFakeIPCache(cfg, outPath, "28.18.0.1/16")

		if _, err := os.Stat(cachePath); err != nil {
			t.Fatalf("cache.db should survive a first run, got: %v", err)
		}
	})

	t.Run("range unchanged — cache untouched", func(t *testing.T) {
		dir := t.TempDir()
		cfg := &config.MetaclashConfig{Core: config.CoreConfig{DataDir: dir}}
		cachePath := filepath.Join(dir, "cache.db")
		if err := os.WriteFile(cachePath, []byte("stub"), 0o644); err != nil {
			t.Fatalf("seed cache.db: %v", err)
		}
		outPath := filepath.Join(dir, "mihomo-config.yaml")
		if err := os.WriteFile(outPath, newConfigDoc("28.18.0.1/16"), 0o644); err != nil {
			t.Fatalf("seed old config: %v", err)
		}

		invalidateStaleFakeIPCache(cfg, outPath, "28.18.0.1/16")

		if _, err := os.Stat(cachePath); err != nil {
			t.Fatalf("cache.db should survive an unchanged range, got: %v", err)
		}
	})

	t.Run("range changed — cache deleted", func(t *testing.T) {
		dir := t.TempDir()
		cfg := &config.MetaclashConfig{Core: config.CoreConfig{DataDir: dir}}
		cachePath := filepath.Join(dir, "cache.db")
		if err := os.WriteFile(cachePath, []byte("stub"), 0o644); err != nil {
			t.Fatalf("seed cache.db: %v", err)
		}
		outPath := filepath.Join(dir, "mihomo-config.yaml")
		if err := os.WriteFile(outPath, newConfigDoc("198.18.0.1/16"), 0o644); err != nil {
			t.Fatalf("seed old config: %v", err)
		}

		invalidateStaleFakeIPCache(cfg, outPath, "28.18.0.1/16")

		if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
			t.Fatalf("expected cache.db to be removed after a range change, stat err: %v", err)
		}
	})

	t.Run("cache.db already absent — no error", func(t *testing.T) {
		dir := t.TempDir()
		cfg := &config.MetaclashConfig{Core: config.CoreConfig{DataDir: dir}}
		outPath := filepath.Join(dir, "mihomo-config.yaml")
		if err := os.WriteFile(outPath, newConfigDoc("198.18.0.1/16"), 0o644); err != nil {
			t.Fatalf("seed old config: %v", err)
		}

		// Must not panic or otherwise fail when cache.db was never created.
		invalidateStaleFakeIPCache(cfg, outPath, "28.18.0.1/16")
	})
}
