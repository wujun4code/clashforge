package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	githubReleasesLatestURL = "https://api.github.com/repos/wujun4code/clashforge/releases/latest"
	githubReleasesListURL   = "https://api.github.com/repos/wujun4code/clashforge/releases"
)

type releaseInfo struct {
	TagName    string `json:"tag_name"`
	Body       string `json:"body"`
	Prerelease bool   `json:"prerelease"`
}

// perChannelCache stores one cache entry per channel ("stable" | "preview").
var releaseCache struct {
	mu      sync.Mutex
	entries map[string]releaseCacheEntry
}

type releaseCacheEntry struct {
	info      releaseInfo
	fetchedAt time.Time
}

const versionCacheTTL = 10 * time.Minute

func init() {
	releaseCache.entries = make(map[string]releaseCacheEntry)
}

func fetchRelease(ctx context.Context, channel string) (info releaseInfo, err error) {
	releaseCache.mu.Lock()
	if e, ok := releaseCache.entries[channel]; ok && time.Since(e.fetchedAt) < versionCacheTTL {
		info = e.info
		releaseCache.mu.Unlock()
		return
	}
	releaseCache.mu.Unlock()

	var url string
	if channel == "preview" {
		url = githubReleasesListURL
	} else {
		url = githubReleasesLatestURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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

	if channel == "preview" {
		// List API returns an array; pick the first entry (newest, including pre-releases).
		var releases []releaseInfo
		if err = json.NewDecoder(resp.Body).Decode(&releases); err != nil {
			return
		}
		if len(releases) > 0 {
			info = releases[0]
		}
	} else {
		if err = json.NewDecoder(resp.Body).Decode(&info); err != nil {
			return
		}
	}

	releaseCache.mu.Lock()
	releaseCache.entries[channel] = releaseCacheEntry{info: info, fetchedAt: time.Now()}
	releaseCache.mu.Unlock()
	return
}

// stripV removes a leading "v" for semver comparison ("v1.2.3" → "1.2.3").
func stripV(s string) string { return strings.TrimPrefix(s, "v") }

func handleClashforgeVersion(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		current := deps.Version

		channel := r.URL.Query().Get("channel")
		if channel != "preview" {
			channel = "stable"
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		info, err := fetchRelease(ctx, channel)
		if err != nil || info.TagName == "" {
			JSON(w, http.StatusOK, map[string]any{
				"current":       current,
				"latest":        "",
				"has_update":    false,
				"download_url":  "",
				"release_url":   "",
				"release_notes": "",
				"channel":       channel,
			})
			return
		}

		hasUpdate := stripV(info.TagName) != stripV(current) && stripV(current) != "0.1.0-dev"
		JSON(w, http.StatusOK, map[string]any{
			"current":       current,
			"latest":        info.TagName,
			"has_update":    hasUpdate,
			"download_url":  "https://github.com/wujun4code/clashforge/releases/download/" + info.TagName + "/install.sh",
			"release_url":   "https://github.com/wujun4code/clashforge/releases/tag/" + info.TagName,
			"release_notes": info.Body,
			"channel":       channel,
		})
	}
}
