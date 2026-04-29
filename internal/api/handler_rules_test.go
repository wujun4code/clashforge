package api

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
)

func TestReadMihomoRuleProviderPaths_PrefersDataDirForRelativePath(t *testing.T) {
	runtimeDir := t.TempDir()
	dataDir := t.TempDir()

	configYAML := "" +
		"rule-providers:\n" +
		"  direct:\n" +
		"    path: ./rule_provider/direct.yaml\n"
	if err := os.WriteFile(filepath.Join(runtimeDir, "mihomo-config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write mihomo config: %v", err)
	}

	expected := filepath.Join(dataDir, "rule_provider", "direct.yaml")
	if err := os.MkdirAll(filepath.Dir(expected), 0o755); err != nil {
		t.Fatalf("mkdir data rule_provider: %v", err)
	}
	if err := os.WriteFile(expected, []byte("payload:\n  - DOMAIN,example.com\n"), 0o644); err != nil {
		t.Fatalf("write data provider: %v", err)
	}

	got := readMihomoRuleProviderPaths(runtimeDir, dataDir)
	if got["direct"] != expected {
		t.Fatalf("provider path mismatch, want %q, got %q", expected, got["direct"])
	}
}

func TestReadMihomoRuleProviderPaths_FallsBackToRuntimeDirWhenNeeded(t *testing.T) {
	runtimeDir := t.TempDir()
	dataDir := t.TempDir()

	configYAML := "" +
		"rule-providers:\n" +
		"  direct:\n" +
		"    path: ./rule_provider/direct.yaml\n"
	if err := os.WriteFile(filepath.Join(runtimeDir, "mihomo-config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write mihomo config: %v", err)
	}

	expected := filepath.Join(runtimeDir, "rule_provider", "direct.yaml")
	if err := os.MkdirAll(filepath.Dir(expected), 0o755); err != nil {
		t.Fatalf("mkdir runtime rule_provider: %v", err)
	}
	if err := os.WriteFile(expected, []byte("payload:\n  - DOMAIN,example.com\n"), 0o644); err != nil {
		t.Fatalf("write runtime provider: %v", err)
	}

	got := readMihomoRuleProviderPaths(runtimeDir, dataDir)
	if got["direct"] != expected {
		t.Fatalf("provider path mismatch, want %q, got %q", expected, got["direct"])
	}
}

func TestSearchRuleFile_FallbackPlainTextSearch(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "direct.list")
	content := "" +
		"# comment\n" +
		"DOMAIN-SUFFIX,google.com\n" +
		"DOMAIN-KEYWORD,youtube\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write rule file: %v", err)
	}

	matches, behavior := searchRuleFile(path, "google.com")
	if behavior != "" {
		t.Fatalf("expected empty behavior for plain text fallback, got %q", behavior)
	}
	if len(matches) != 1 || matches[0] != "DOMAIN-SUFFIX,google.com" {
		t.Fatalf("unexpected matches: %#v", matches)
	}
}

func TestHandleGetRuleProviders_UsesResolvedDataDirPath(t *testing.T) {
	runtimeDir := t.TempDir()
	dataDir := t.TempDir()

	configYAML := "" +
		"rule-providers:\n" +
		"  direct:\n" +
		"    path: ./rule_provider/direct.yaml\n"
	if err := os.WriteFile(filepath.Join(runtimeDir, "mihomo-config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write mihomo config: %v", err)
	}
	providerPath := filepath.Join(dataDir, "rule_provider", "direct.yaml")
	if err := os.MkdirAll(filepath.Dir(providerPath), 0o755); err != nil {
		t.Fatalf("mkdir data provider dir: %v", err)
	}
	if err := os.WriteFile(providerPath, []byte("payload:\n  - DOMAIN,example.com\n"), 0o644); err != nil {
		t.Fatalf("write data provider: %v", err)
	}

	mihomoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/providers/rules" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"providers": map[string]any{
				"direct": map[string]any{
					"type":        "Rule",
					"vehicleType": "HTTP",
					"behavior":    "domain",
					"format":      "yaml",
					"ruleCount":   1,
				},
			},
		})
	}))
	defer mihomoServer.Close()

	port := mustServerPort(t, mihomoServer.URL)
	deps := Dependencies{
		Config: &config.MetaclashConfig{
			Core:  config.CoreConfig{RuntimeDir: runtimeDir, DataDir: dataDir},
			Ports: config.PortsConfig{MihomoAPI: port},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/rules/providers", nil)
	rr := httptest.NewRecorder()
	handleGetRuleProviders(deps).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status code: want %d, got %d, body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var resp struct {
		OK   bool `json:"ok"`
		Data struct {
			Providers []ruleProviderInfo `json:"providers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !resp.OK {
		t.Fatalf("response not ok: %s", rr.Body.String())
	}
	if len(resp.Data.Providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(resp.Data.Providers))
	}
	if resp.Data.Providers[0].FilePath != providerPath {
		t.Fatalf("expected file_path %q, got %q", providerPath, resp.Data.Providers[0].FilePath)
	}
}

func TestHandleSearchRules_SearchesResolvedDataDirProvider(t *testing.T) {
	runtimeDir := t.TempDir()
	dataDir := t.TempDir()

	configYAML := "" +
		"rule-providers:\n" +
		"  direct:\n" +
		"    path: ./rule_provider/direct.yaml\n"
	if err := os.WriteFile(filepath.Join(runtimeDir, "mihomo-config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write mihomo config: %v", err)
	}
	providerPath := filepath.Join(dataDir, "rule_provider", "direct.yaml")
	if err := os.MkdirAll(filepath.Dir(providerPath), 0o755); err != nil {
		t.Fatalf("mkdir data provider dir: %v", err)
	}
	content := "" +
		"behavior: domain\n" +
		"payload:\n" +
		"  - DOMAIN-SUFFIX,google.com\n" +
		"  - DOMAIN-SUFFIX,github.com\n"
	if err := os.WriteFile(providerPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write data provider: %v", err)
	}

	mihomoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/providers/rules" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"providers": map[string]any{
				"direct": map[string]any{
					"behavior": "domain",
					"format":   "yaml",
				},
			},
		})
	}))
	defer mihomoServer.Close()

	port := mustServerPort(t, mihomoServer.URL)
	deps := Dependencies{
		Config: &config.MetaclashConfig{
			Core:  config.CoreConfig{RuntimeDir: runtimeDir, DataDir: dataDir},
			Ports: config.PortsConfig{MihomoAPI: port},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/rules/search?q=google.com", nil)
	rr := httptest.NewRecorder()
	handleSearchRules(deps).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status code: want %d, got %d, body=%s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var resp struct {
		OK   bool `json:"ok"`
		Data struct {
			Results []ruleSearchResult `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !resp.OK {
		t.Fatalf("response not ok: %s", rr.Body.String())
	}
	if len(resp.Data.Results) != 1 {
		t.Fatalf("expected 1 search result, got %d", len(resp.Data.Results))
	}
	if resp.Data.Results[0].Provider != "direct" {
		t.Fatalf("unexpected provider: %q", resp.Data.Results[0].Provider)
	}
	if len(resp.Data.Results[0].Matches) == 0 {
		t.Fatalf("expected at least one match, got none")
	}
}

func mustServerPort(t *testing.T, rawURL string) int {
	t.Helper()
	addr := strings.TrimPrefix(rawURL, "http://")
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("parse server host/port from %q: %v", rawURL, err)
	}
	if host == "" {
		t.Fatalf("empty host in test server url: %q", rawURL)
	}
	value, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("parse server port %q: %v", port, err)
	}
	return value
}
