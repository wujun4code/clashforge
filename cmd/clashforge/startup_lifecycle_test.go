package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/dns"
)

func TestStartupRejectsMissingGenerationInsteadOfStaleRuntime(t *testing.T) {
	cfg := config.Default()
	cfg.Core.RuntimeDir = t.TempDir()
	cfg.Core.DataDir = t.TempDir()
	stale := filepath.Join(cfg.Core.RuntimeDir, "mihomo-config.yaml")
	if err := os.WriteFile(stale, []byte("old-runtime"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeRuntimeMihomoConfig(cfg, nil); err == nil {
		t.Fatal("startup accepted absent generation and overwrote/used stale runtime")
	}
	data, err := os.ReadFile(stale)
	if err != nil || string(data) != "old-runtime" {
		t.Fatalf("stale runtime changed: %s, %v", data, err)
	}
}

func TestCrashRestoreStopsOnFailure(t *testing.T) {
	cfg := config.Default()
	cfg.DNS.Strategy = config.DNSStrategysplit
	calls := 0
	err := restoreDNSAfterCrash(cfg, func(dns.DnsmasqMode) error {
		calls++
		return os.ErrPermission
	})
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("restore failure was not propagated: %v", err)
	}
	if calls != 1 {
		t.Fatalf("DNS restore ignored failure and continued: %d calls", calls)
	}
}
