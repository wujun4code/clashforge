package geodata

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
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

func TestBuildSpecs_ContainsBothGeoIPFormats(t *testing.T) {
	t.Parallel()

	cfg := defaultTestConfig()
	specs := buildSpecs(cfg)

	byName := make(map[string]FileSpec, len(specs))
	for _, s := range specs {
		byName[s.Filename] = s
	}

	for _, required := range []string{"country.mmdb", "GeoIP.dat", "GeoSite.dat"} {
		if _, ok := byName[required]; !ok {
			t.Errorf("buildSpecs missing expected file: %s", required)
		}
	}

	// country.mmdb must have at least one mmdb CDN mirror URL
	mmdb := byName["country.mmdb"]
	foundMMDB := false
	for _, u := range mmdb.URLs {
		if contains(u, "country.mmdb") {
			foundMMDB = true
			break
		}
	}
	if !foundMMDB {
		t.Errorf("country.mmdb spec has no URL referencing country.mmdb: %v", mmdb.URLs)
	}

	// GeoIP.dat must have at least one dat-format CDN URL
	dat := byName["GeoIP.dat"]
	foundDAT := false
	for _, u := range dat.URLs {
		if contains(u, "geoip.dat") {
			foundDAT = true
			break
		}
	}
	if !foundDAT {
		t.Errorf("GeoIP.dat spec has no URL referencing geoip.dat: %v", dat.URLs)
	}
}

func TestBuildSpecs_CustomGeoIPURLPrependedToMMDB(t *testing.T) {
	t.Parallel()

	cfg := defaultTestConfig()
	cfg.Update.GeoIPURL = "https://example.com/custom.mmdb"
	specs := buildSpecs(cfg)

	for _, s := range specs {
		if s.Filename == "country.mmdb" {
			if len(s.URLs) == 0 || s.URLs[0] != "https://example.com/custom.mmdb" {
				t.Errorf("custom GeoIPURL not prepended to country.mmdb URLs: %v", s.URLs)
			}
			return
		}
	}
	t.Error("country.mmdb spec not found")
}

func defaultTestConfig() *config.MetaclashConfig {
	return config.Default()
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

