package core

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleDeathIgnoresStaleRun(t *testing.T) {
	m := NewManager(CoreManagerConfig{MaxRestarts: 3})
	m.state = StateRunning
	m.pid = 2222
	m.runID = 2
	m.stopCh = make(chan struct{})

	m.handleDeath(errors.New("stale exit"), 1, 1111)

	if m.state != StateRunning {
		t.Fatalf("expected state to stay running, got %s", m.state)
	}
	if m.pid != 2222 {
		t.Fatalf("expected pid to stay 2222, got %d", m.pid)
	}
}

func TestHandleDeathCurrentRunMarksError(t *testing.T) {
	m := NewManager(CoreManagerConfig{MaxRestarts: 3})
	m.state = StateRunning
	m.pid = 3333
	m.runID = 4
	m.stopCh = make(chan struct{})
	close(m.stopCh)

	m.handleDeath(errors.New("current exit"), 4, 3333)

	if m.state != StateError {
		t.Fatalf("expected state error, got %s", m.state)
	}
	if m.pid != 0 {
		t.Fatalf("expected pid to reset to 0, got %d", m.pid)
	}
}

func TestStageConfigForReload_CopiesWhenOutsideHomeDir(t *testing.T) {
	homeDir := t.TempDir()
	runtimeDir := t.TempDir()
	configFile := filepath.Join(runtimeDir, "mihomo-config.yaml")
	want := []byte("dns:\n  enable: true\n")
	if err := os.WriteFile(configFile, want, 0o644); err != nil {
		t.Fatalf("write fixture config: %v", err)
	}

	got, err := stageConfigForReload(homeDir, configFile)
	if err != nil {
		t.Fatalf("stageConfigForReload: %v", err)
	}
	if filepath.Dir(got) != homeDir {
		t.Fatalf("expected staged path under %q, got %q", homeDir, got)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("read staged config: %v", err)
	}
	if string(data) != string(want) {
		t.Fatalf("staged content = %q, want %q", data, want)
	}
}

func TestStageConfigForReload_NoopWhenAlreadyUnderHomeDir(t *testing.T) {
	homeDir := t.TempDir()
	configFile := filepath.Join(homeDir, "mihomo-config.yaml")
	if err := os.WriteFile(configFile, []byte("dns: {}\n"), 0o644); err != nil {
		t.Fatalf("write fixture config: %v", err)
	}

	got, err := stageConfigForReload(homeDir, configFile)
	if err != nil {
		t.Fatalf("stageConfigForReload: %v", err)
	}
	if got != configFile {
		t.Fatalf("expected no-op returning original path %q, got %q", configFile, got)
	}
}

func TestStageConfigForReload_NoopWhenHomeDirEmpty(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "mihomo-config.yaml")
	if err := os.WriteFile(configFile, []byte("dns: {}\n"), 0o644); err != nil {
		t.Fatalf("write fixture config: %v", err)
	}

	got, err := stageConfigForReload("", configFile)
	if err != nil {
		t.Fatalf("stageConfigForReload: %v", err)
	}
	if got != configFile {
		t.Fatalf("expected no-op returning original path %q, got %q", configFile, got)
	}
}
