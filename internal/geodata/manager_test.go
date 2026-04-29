package geodata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildProxyURLDirect(t *testing.T) {
	t.Parallel()

	if got := buildProxyURL("", 17893, ""); got != "" {
		t.Fatalf("expected empty proxy URL for empty proxy server, got %q", got)
	}
	if got := buildProxyURL("DIRECT", 17893, ""); got != "" {
		t.Fatalf("expected empty proxy URL for DIRECT, got %q", got)
	}
	if got := buildProxyURL(" direct ", 17893, ""); got != "" {
		t.Fatalf("expected empty proxy URL for case-insensitive DIRECT, got %q", got)
	}
}

func TestBuildProxyURLWithMihomoAuthentication(t *testing.T) {
	t.Parallel()

	runtimeDir := t.TempDir()
	content := "mixed-port: 17893\nauthentication:\n  - test-user:test-pass\n"
	if err := os.WriteFile(filepath.Join(runtimeDir, "mihomo-config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write mihomo config: %v", err)
	}

	got := buildProxyURL("proxy", 17893, runtimeDir)
	want := "http://test-user:test-pass@127.0.0.1:17893"
	if got != want {
		t.Fatalf("unexpected proxy URL: got %q want %q", got, want)
	}
}

func TestBuildProxyURLWithoutMihomoAuthentication(t *testing.T) {
	t.Parallel()

	runtimeDir := t.TempDir()
	content := "mixed-port: 17893\nmode: rule\n"
	if err := os.WriteFile(filepath.Join(runtimeDir, "mihomo-config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write mihomo config: %v", err)
	}

	got := buildProxyURL("proxy", 17893, runtimeDir)
	want := "http://127.0.0.1:17893"
	if got != want {
		t.Fatalf("unexpected proxy URL: got %q want %q", got, want)
	}
}

