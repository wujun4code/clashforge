package cfgen

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func generateTestConfig(t *testing.T, dnsMode, powerProfile string) map[string]interface{} {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	input := []byte("mode: rule\ndns:\n  nameserver: [223.5.5.5]\n")
	if err := os.WriteFile(path, input, 0o644); err != nil {
		t.Fatal(err)
	}
	if result := GenerateConfig(path, 0, filepath.Dir(path), dnsMode, powerProfile); !result.OK {
		t.Fatalf("GenerateConfig failed: %s", result.Error)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]interface{}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestGenerateConfigBatteryProfile(t *testing.T) {
	cfg := generateTestConfig(t, "fake-ip", "battery")
	if got := cfg["log-level"]; got != "warning" {
		t.Fatalf("log-level = %v, want warning", got)
	}
	if got := cfg["unified-delay"]; got != false {
		t.Fatalf("unified-delay = %v, want false", got)
	}
	if got := cfg["tcp-concurrent"]; got != false {
		t.Fatalf("tcp-concurrent = %v, want false", got)
	}
	sniffer, ok := cfg["sniffer"].(map[string]interface{})
	if !ok || sniffer["enable"] != false {
		t.Fatalf("sniffer = %#v, want disabled", cfg["sniffer"])
	}
}

func TestGenerateConfigBalancedFallback(t *testing.T) {
	cfg := generateTestConfig(t, "fake-ip", "unknown")
	if got := cfg["unified-delay"]; got != false {
		t.Fatalf("unified-delay = %v, want false", got)
	}
	if got := cfg["tcp-concurrent"]; got != true {
		t.Fatalf("tcp-concurrent = %v, want true", got)
	}
	sniffer := cfg["sniffer"].(map[string]interface{})
	if sniffer["enable"] != true {
		t.Fatalf("sniffer = %#v, want enabled", sniffer)
	}
}

func TestGenerateConfigBatteryRedirHostKeepsSniffer(t *testing.T) {
	cfg := generateTestConfig(t, "redir-host", "battery")
	sniffer := cfg["sniffer"].(map[string]interface{})
	if sniffer["enable"] != true {
		t.Fatalf("sniffer = %#v, want enabled for redir-host", sniffer)
	}
}
