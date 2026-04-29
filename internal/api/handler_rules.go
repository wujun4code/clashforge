package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"
)

// ── Types ─────────────────────────────────────────────────────────────────────

type ruleProviderInfo struct {
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	VehicleType string    `json:"vehicleType"`
	Behavior    string    `json:"behavior"`
	Format      string    `json:"format"`
	RuleCount   int       `json:"ruleCount"`
	UpdatedAt   time.Time `json:"updatedAt"`
	// augmented from disk
	FilePath string  `json:"file_path,omitempty"`
	SizeMB   float64 `json:"size_mb"`
}

type ruleSearchResult struct {
	Provider string   `json:"provider"`
	Behavior string   `json:"behavior"`
	Matches  []string `json:"matches"`
	Total    int      `json:"total"`
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// handleGetRuleProviders proxies GET /providers/rules from mihomo and augments
// with disk file sizes.
func handleGetRuleProviders(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mihomoData, err := mihomoGet(deps.Config.Ports.MihomoAPI, "/providers/rules")
		if err != nil {
			Err(w, http.StatusBadGateway, "MIHOMO_UNAVAILABLE", err.Error())
			return
		}

		// Parse mihomo response: {"providers": {"name": {...}, ...}}
		var raw struct {
			Providers map[string]json.RawMessage `json:"providers"`
		}
		if err := json.Unmarshal(mihomoData, &raw); err != nil {
			Err(w, http.StatusBadGateway, "MIHOMO_PARSE_ERROR", err.Error())
			return
		}

		// Read actual file paths from the generated mihomo config
		providerPaths := readMihomoRuleProviderPaths(deps.Config.Core.RuntimeDir, deps.Config.Core.DataDir)

		providers := make([]ruleProviderInfo, 0, len(raw.Providers))

		for name, rawVal := range raw.Providers {
			var p ruleProviderInfo
			if err := json.Unmarshal(rawVal, &p); err != nil {
				continue
			}
			p.Name = name
			candidate := resolveRuleProviderFile(
				name,
				p.Format,
				providerPaths,
				deps.Config.Core.RuntimeDir,
				deps.Config.Core.DataDir,
			)
			if candidate != "" {
				p.FilePath = candidate
			}
			if info, err := os.Stat(candidate); err == nil {
				p.SizeMB = bytesToMB(uint64(info.Size()))
			}
			providers = append(providers, p)
		}

		JSON(w, http.StatusOK, map[string]any{"providers": providers})
	}
}

// handleSyncRuleProvider forces mihomo to re-fetch a single rule provider by name.
// It also sends cache-busting headers by calling the PUT endpoint.
func handleSyncRuleProvider(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		if name == "" {
			Err(w, http.StatusBadRequest, "MISSING_PARAM", "provider name required")
			return
		}
		if err := mihomoPut(deps.Config.Ports.MihomoAPI, "/providers/rules/"+name); err != nil {
			Err(w, http.StatusBadGateway, "SYNC_FAILED", err.Error())
			return
		}
		JSON(w, http.StatusOK, map[string]any{"ok": true, "name": name})
	}
}

// handleSyncAllRuleProviders forces re-fetch of every HTTP-type rule provider in
// parallel and reports per-provider results.
func handleSyncAllRuleProviders(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mihomoData, err := mihomoGet(deps.Config.Ports.MihomoAPI, "/providers/rules")
		if err != nil {
			Err(w, http.StatusBadGateway, "MIHOMO_UNAVAILABLE", err.Error())
			return
		}

		var raw struct {
			Providers map[string]struct {
				VehicleType string `json:"vehicleType"`
			} `json:"providers"`
		}
		if err := json.Unmarshal(mihomoData, &raw); err != nil {
			Err(w, http.StatusBadGateway, "MIHOMO_PARSE_ERROR", err.Error())
			return
		}

		type syncResult struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
			Err  string `json:"error,omitempty"`
		}

		var mu sync.Mutex
		var wg sync.WaitGroup
		results := make([]syncResult, 0, len(raw.Providers))

		for name, p := range raw.Providers {
			// Only HTTP providers actually download from a URL
			if !strings.EqualFold(p.VehicleType, "HTTP") {
				continue
			}
			wg.Add(1)
			go func(n string) {
				defer wg.Done()
				err := mihomoPut(deps.Config.Ports.MihomoAPI, "/providers/rules/"+n)
				mu.Lock()
				if err != nil {
					results = append(results, syncResult{Name: n, OK: false, Err: err.Error()})
				} else {
					results = append(results, syncResult{Name: n, OK: true})
				}
				mu.Unlock()
			}(name)
		}
		wg.Wait()

		JSON(w, http.StatusOK, map[string]any{"ok": true, "results": results})
	}
}

// handleSearchRules searches across all rule provider files from mihomo config.
// Query param: q (required), provider (optional, filter to single provider).
func handleSearchRules(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q == "" {
			Err(w, http.StatusBadRequest, "MISSING_PARAM", "q is required")
			return
		}
		filterProvider := r.URL.Query().Get("provider")
		qLower := strings.ToLower(q)

		mihomoData, err := mihomoGet(deps.Config.Ports.MihomoAPI, "/providers/rules")
		if err != nil {
			Err(w, http.StatusBadGateway, "MIHOMO_UNAVAILABLE", err.Error())
			return
		}
		var raw struct {
			Providers map[string]struct {
				Behavior string `json:"behavior"`
				Format   string `json:"format"`
			} `json:"providers"`
		}
		if err := json.Unmarshal(mihomoData, &raw); err != nil {
			Err(w, http.StatusBadGateway, "MIHOMO_PARSE_ERROR", err.Error())
			return
		}

		providerPaths := readMihomoRuleProviderPaths(deps.Config.Core.RuntimeDir, deps.Config.Core.DataDir)

		var resultsMu sync.Mutex
		var wg sync.WaitGroup
		allResults := make([]ruleSearchResult, 0)

		for providerName, spec := range raw.Providers {
			if filterProvider != "" && !strings.EqualFold(providerName, filterProvider) {
				continue
			}
			path := resolveRuleProviderFile(
				providerName,
				spec.Format,
				providerPaths,
				deps.Config.Core.RuntimeDir,
				deps.Config.Core.DataDir,
			)
			if path == "" {
				continue
			}

			wg.Add(1)
			go func(name, path, defaultBehavior string) {
				defer wg.Done()
				matches, behavior := searchRuleFile(path, qLower)
				if len(matches) == 0 {
					return
				}
				if behavior == "" {
					behavior = defaultBehavior
				}
				resultsMu.Lock()
				allResults = append(allResults, ruleSearchResult{
					Provider: name,
					Behavior: behavior,
					Matches:  matches,
					Total:    len(matches),
				})
				resultsMu.Unlock()
			}(providerName, path, spec.Behavior)
		}
		wg.Wait()

		JSON(w, http.StatusOK, map[string]any{
			"query":   q,
			"results": allResults,
		})
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// mihomoGet calls GET on the mihomo API and returns the raw response body.
func mihomoGet(port int, path string) ([]byte, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", port, path)
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// mihomoPut calls PUT on the mihomo API with no body (used for force-update).
func mihomoPut(port int, path string) error {
	url := fmt.Sprintf("http://127.0.0.1:%d%s", port, path)
	req, err := http.NewRequest(http.MethodPut, url, nil)
	if err != nil {
		return err
	}
	// Ask the CDN to bypass its cache
	req.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	req.Header.Set("Pragma", "no-cache")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("mihomo returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// searchRuleFile parses a rule provider YAML file and returns entries that
// contain the query string (case-insensitive). Also returns the inferred behavior.
func searchRuleFile(path, qLower string) (matches []string, behavior string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, ""
	}

	var doc struct {
		Behavior string   `yaml:"behavior"`
		Payload  []string `yaml:"payload"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		// Fallback to plain-text rule list (one entry per line)
		return searchRuleLines(data, qLower), ""
	}

	behavior = doc.Behavior
	if len(doc.Payload) > 0 {
		for _, entry := range doc.Payload {
			if strings.Contains(strings.ToLower(entry), qLower) {
				matches = append(matches, entry)
				if len(matches) >= 200 { // cap results per provider
					break
				}
			}
		}
		return matches, behavior
	}

	return searchRuleLines(data, qLower), behavior
}

// readMihomoRuleProviderPaths reads the generated mihomo config and returns a
// map of provider name → absolute file path on disk.
func readMihomoRuleProviderPaths(runtimeDir, dataDir string) map[string]string {
	result := make(map[string]string)
	configPath := filepath.Join(runtimeDir, "mihomo-config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return result
	}
	var doc struct {
		RuleProviders map[string]struct {
			Path string `yaml:"path"`
		} `yaml:"rule-providers"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return result
	}
	for name, rp := range doc.RuleProviders {
		if rp.Path == "" {
			continue
		}
		p := rp.Path
		if filepath.IsAbs(p) {
			result[name] = filepath.Clean(p)
			continue
		}

		// Mihomo runtime uses -d <dataDir>, so relative path should be resolved
		// against dataDir first. Fall back to runtimeDir for legacy layouts.
		rel := filepath.Clean(p)
		if dataDir != "" {
			dataCandidate := filepath.Clean(filepath.Join(dataDir, rel))
			if pathExists(dataCandidate) {
				result[name] = dataCandidate
				continue
			}
			runtimeCandidate := filepath.Clean(filepath.Join(runtimeDir, rel))
			if pathExists(runtimeCandidate) {
				result[name] = runtimeCandidate
				continue
			}
			result[name] = dataCandidate
			continue
		}
		result[name] = filepath.Clean(filepath.Join(runtimeDir, rel))
	}
	return result
}

func resolveRuleProviderFile(name, format string, configuredPaths map[string]string, runtimeDir, dataDir string) string {
	if p := strings.TrimSpace(configuredPaths[name]); p != "" {
		return p
	}
	candidates := buildProviderFallbackCandidates(name, format, runtimeDir, dataDir)
	for _, candidate := range candidates {
		if pathExists(candidate) {
			return candidate
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

func buildProviderFallbackCandidates(name, format, runtimeDir, dataDir string) []string {
	exts := []string{".yaml", ".yml", ".mrs", ".txt"}
	f := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(format), "."))
	if f != "" {
		exts = append([]string{"." + f}, exts...)
	}

	dirs := []string{
		filepath.Join(dataDir, "rule_provider"),
		filepath.Join(runtimeDir, "rule_provider"),
		filepath.Join(dataDir, "ruleset"),
		filepath.Join(dataDir, "rule-set"),
		filepath.Join(runtimeDir, "ruleset"),
		filepath.Join(runtimeDir, "rule-set"),
	}

	seen := map[string]bool{}
	out := make([]string, 0, len(dirs)*len(exts))
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		for _, ext := range exts {
			p := filepath.Clean(filepath.Join(dir, name+ext))
			if seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func searchRuleLines(data []byte, qLower string) []string {
	// Skip binary-like files (e.g. mrs) that are not safely searchable as text.
	if bytes.IndexByte(data, 0) >= 0 {
		return nil
	}

	lines := strings.Split(string(data), "\n")
	matches := make([]string, 0, 16)
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.EqualFold(line, "payload:") {
			continue
		}
		line = strings.TrimPrefix(line, "-")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(strings.ToLower(line), qLower) {
			matches = append(matches, line)
			if len(matches) >= 200 {
				break
			}
		}
	}
	return matches
}

func pathExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}
