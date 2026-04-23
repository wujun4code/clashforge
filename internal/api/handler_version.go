package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

const githubReleasesURL = "https://api.github.com/repos/wujun4code/clashforge/releases/latest"

// versionCache caches the latest GitHub release to avoid hammering the API.
var versionCache struct {
	mu        sync.Mutex
	latest    string
	fetchedAt time.Time
}

const versionCacheTTL = 10 * time.Minute

func fetchLatestRelease(ctx context.Context) (tag string, err error) {
	versionCache.mu.Lock()
	if time.Since(versionCache.fetchedAt) < versionCacheTTL && versionCache.latest != "" {
		tag = versionCache.latest
		versionCache.mu.Unlock()
		return
	}
	versionCache.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubReleasesURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "clashforge-update-check/1.0")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return
	}
	tag = payload.TagName

	versionCache.mu.Lock()
	versionCache.latest = tag
	versionCache.fetchedAt = time.Now()
	versionCache.mu.Unlock()
	return
}

// stripV removes a leading "v" for semver comparison ("v1.2.3" → "1.2.3").
func stripV(s string) string { return strings.TrimPrefix(s, "v") }

func handleClashforgeVersion(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		current := deps.Version // injected via ldflags as buildVersion

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		latest, err := fetchLatestRelease(ctx)
		if err != nil || latest == "" {
			// Network unreachable or GitHub rate-limit — return current only, no error.
			JSON(w, http.StatusOK, map[string]any{
				"current":      current,
				"latest":       "",
				"has_update":   false,
				"download_url": "",
				"release_url":  "",
			})
			return
		}

		hasUpdate := stripV(latest) != stripV(current) && latest != "" && stripV(current) != "0.1.0-dev"
		JSON(w, http.StatusOK, map[string]any{
			"current":      current,
			"latest":       latest,
			"has_update":   hasUpdate,
			"download_url": "https://github.com/wujun4code/clashforge/releases/download/" + latest + "/install.sh",
			"release_url":  "https://github.com/wujun4code/clashforge/releases/tag/" + latest,
		})
	}
}
