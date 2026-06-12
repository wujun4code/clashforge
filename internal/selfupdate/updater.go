package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/wujun4code/clashforge/internal/config"
)

const (
	ghReleasesLatestURL = "https://api.github.com/repos/wujun4code/clashforge/releases/latest"
	ghReleasesListURL   = "https://api.github.com/repos/wujun4code/clashforge/releases"
	ghRepo              = "wujun4code/clashforge"
)

var ghProxyMirrors = []string{
	"https://ghproxy.com",
	"https://mirror.ghproxy.com",
	"https://ghfast.top",
	"https://github.moeyy.xyz",
}

// Result records the outcome of one self-update run.
type Result struct {
	RunAt      time.Time `json:"run_at"`
	Success    bool      `json:"success"`
	OldVersion string    `json:"old_version"`
	NewVersion string    `json:"new_version,omitempty"`
	Error      string    `json:"error,omitempty"`
	Skipped    bool      `json:"skipped"`
	SkipReason string    `json:"skip_reason,omitempty"`
}

// Updater checks for and applies clashforge self-updates.
type Updater struct {
	cfg     *config.MetaclashConfig
	version string
	mu      sync.Mutex
	running bool
	last    *Result

	// injectable for tests
	spawnUpgrade        func(ipkPath string) error
	downloadIPK         func(ctx context.Context, proxyURL, tag, arch string) (string, error)
	detectArch          func() (string, error)
	preRunHook          func() // called at the very start of run(); used by tests
	ghReleasesLatestURL string // overridable for tests
	ghReleasesListURL   string // overridable for tests
	ipkBaseURL          string // overridable for tests; replaces github.com download base
}

// New creates an Updater. version is the current buildVersion string.
func New(cfg *config.MetaclashConfig, version string) *Updater {
	u := &Updater{
		cfg:                 cfg,
		version:             version,
		ghReleasesLatestURL: ghReleasesLatestURL,
		ghReleasesListURL:   ghReleasesListURL,
	}
	u.spawnUpgrade = defaultSpawnUpgrade
	u.detectArch = detectArch
	u.downloadIPK = u.defaultDownloadIPK
	return u
}

// SetSpawnUpgrade replaces the upgrade spawn function (for testing).
func (u *Updater) SetSpawnUpgrade(fn func(ipkPath string) error) {
	u.spawnUpgrade = fn
}

// SetDetectArch replaces the arch detection function (for testing).
func (u *Updater) SetDetectArch(fn func() (string, error)) {
	u.detectArch = fn
}

// SetGHReleasesLatestURL overrides the GitHub releases latest API URL (for testing).
func (u *Updater) SetGHReleasesLatestURL(url string) {
	u.ghReleasesLatestURL = url
	u.ghReleasesListURL = url // for simplicity point both at mock in tests
}

// SetIPKDownloadFn replaces the IPK download function (for testing).
// The fn receives (proxyURL, tag, arch); ctx is intentionally omitted for
// test simplicity.
func (u *Updater) SetIPKDownloadFn(fn func(proxyURL, tag, arch string) (string, error)) {
	u.downloadIPK = func(ctx context.Context, proxyURL, tag, arch string) (string, error) {
		return fn(proxyURL, tag, arch)
	}
}

// SetPreRunHook sets a function called at the very beginning of run()
// before any network activity. Used to synchronise concurrency tests.
func (u *Updater) SetPreRunHook(fn func()) {
	u.preRunHook = fn
}

// UpdateConfig replaces the live configuration pointer (called after config save).
func (u *Updater) UpdateConfig(cfg *config.MetaclashConfig) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.cfg = cfg
}

// IsRunning reports whether a check/download is in progress.
func (u *Updater) IsRunning() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.running
}

// LastResult returns a copy of the most recent run result, or nil.
func (u *Updater) LastResult() *Result {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.last == nil {
		return nil
	}
	cp := *u.last
	return &cp
}

// TriggerAsync starts a self-update in the background. Returns false when
// one is already running.
func (u *Updater) TriggerAsync() bool {
	u.mu.Lock()
	if u.running {
		u.mu.Unlock()
		return false
	}
	u.running = true
	u.mu.Unlock()

	go func() {
		defer func() {
			u.mu.Lock()
			u.running = false
			u.mu.Unlock()
		}()
		u.run(context.Background())
	}()
	return true
}

// Run executes the self-update check synchronously (called by the scheduler).
func (u *Updater) Run() {
	u.mu.Lock()
	if u.running {
		u.mu.Unlock()
		log.Info().Msg("selfupdate: already running, skipping scheduled run")
		return
	}
	u.running = true
	u.mu.Unlock()

	defer func() {
		u.mu.Lock()
		u.running = false
		u.mu.Unlock()
	}()
	u.run(context.Background())
}

func (u *Updater) run(ctx context.Context) {
	if u.preRunHook != nil {
		u.preRunHook()
	}

	u.mu.Lock()
	cfg := u.cfg
	currentVersion := u.version
	u.mu.Unlock()

	res := &Result{RunAt: time.Now(), OldVersion: currentVersion}

	proxyURL := fmt.Sprintf("http://127.0.0.1:%d", cfg.Ports.Mixed)

	channel := cfg.Update.SelfUpdateChannel
	if channel != "preview" {
		channel = "stable"
	}

	// Step 1: fetch latest release tag — try via proxy first.
	// A successful proxy fetch also confirms the proxy is functional.
	tag, err := u.fetchLatestTag(ctx, proxyURL, channel)
	viaProxy := err == nil
	if err != nil {
		log.Warn().Err(err).Str("proxy", proxyURL).Msg("selfupdate: proxy unreachable, trying direct")
		tag, err = u.fetchLatestTag(ctx, "", channel)
		if err != nil {
			res.Error = fmt.Sprintf("fetch release failed (proxy + direct both failed): %v", err)
			u.saveResult(res)
			log.Error().Str("error", res.Error).Msg("selfupdate: aborted")
			return
		}
	}

	// Step 2: compare versions.
	if !hasVersionUpdate(currentVersion, tag) {
		res.Skipped = true
		res.SkipReason = fmt.Sprintf("already at latest (%s)", tag)
		u.saveResult(res)
		log.Info().Str("version", currentVersion).Str("latest", tag).Msg("selfupdate: already up to date")
		return
	}

	res.NewVersion = tag
	log.Info().Str("current", currentVersion).Str("latest", tag).Bool("via_proxy", viaProxy).Msg("selfupdate: update available, downloading")

	// Step 3: detect arch.
	arch, err := u.detectArch()
	if err != nil {
		res.Error = fmt.Sprintf("arch detection failed: %v", err)
		u.saveResult(res)
		log.Error().Err(err).Msg("selfupdate: aborted")
		return
	}

	// Step 4: download IPK.
	effectiveProxy := ""
	if viaProxy {
		effectiveProxy = proxyURL
	}
	ipkPath, err := u.downloadIPK(ctx, effectiveProxy, tag, arch)
	if err != nil {
		res.Error = fmt.Sprintf("IPK download failed: %v", err)
		u.saveResult(res)
		log.Error().Err(err).Msg("selfupdate: aborted")
		return
	}

	// Step 5: spawn detached upgrade process. This process will stop the
	// clashforge service (including this Go binary), install the new package,
	// then restart the service.
	if err := u.spawnUpgrade(ipkPath); err != nil {
		res.Error = fmt.Sprintf("spawn upgrade process failed: %v", err)
		_ = os.Remove(ipkPath)
		u.saveResult(res)
		log.Error().Err(err).Msg("selfupdate: aborted")
		return
	}

	res.Success = true
	u.saveResult(res)
	log.Info().Str("version", tag).Str("ipk", ipkPath).Msg("selfupdate: upgrade spawned, service will restart shortly")
}

func (u *Updater) saveResult(r *Result) {
	u.mu.Lock()
	u.last = r
	u.mu.Unlock()
}

// ── GitHub release fetching ────────────────────────────────────────────────

type releaseInfo struct {
	TagName string `json:"tag_name"`
}

func (u *Updater) fetchLatestTag(ctx context.Context, proxyURL, channel string) (string, error) {
	apiURL := u.ghReleasesLatestURL
	if channel == "preview" {
		apiURL = u.ghReleasesListURL
	}
	body, err := httpGet(ctx, apiURL, proxyURL, 10*time.Second)
	if err != nil {
		return "", err
	}
	if channel == "preview" {
		var list []releaseInfo
		if err := json.Unmarshal(body, &list); err != nil || len(list) == 0 {
			return "", fmt.Errorf("parse releases list: %w", err)
		}
		return list[0].TagName, nil
	}
	var info releaseInfo
	if err := json.Unmarshal(body, &info); err != nil || info.TagName == "" {
		return "", fmt.Errorf("parse release: %w", err)
	}
	return info.TagName, nil
}

// ── IPK download ───────────────────────────────────────────────────────────

func (u *Updater) defaultDownloadIPK(ctx context.Context, proxyURL, tag, arch string) (string, error) {
	pkgVer := strings.TrimPrefix(tag, "v")
	ipkName := fmt.Sprintf("clashforge_%s_%s.ipk", pkgVer, arch)

	base := fmt.Sprintf("https://github.com/%s/releases/download/%s", ghRepo, tag)
	if u.ipkBaseURL != "" {
		base = u.ipkBaseURL
	}
	ghURL := fmt.Sprintf("%s/%s", base, ipkName)

	tmp, err := os.CreateTemp("", "clashforge-*.ipk")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmp.Close()
	tmpPath := tmp.Name()

	// Build download candidates: proxy direct → github direct → ghproxy mirrors.
	type candidate struct{ proxy, downloadURL string }
	var cands []candidate
	if proxyURL != "" {
		cands = append(cands, candidate{proxyURL, ghURL})
	}
	cands = append(cands, candidate{"", ghURL})
	for _, mirror := range ghProxyMirrors {
		cands = append(cands, candidate{"", mirror + "/" + ghURL})
	}

	for _, c := range cands {
		label := c.downloadURL
		if c.proxy != "" {
			label = fmt.Sprintf("%s (via %s)", c.downloadURL, c.proxy)
		}
		if err := httpDownload(ctx, c.downloadURL, c.proxy, tmpPath, 5*time.Minute); err != nil {
			log.Warn().Str("source", label).Err(err).Msg("selfupdate: download attempt failed")
			continue
		}
		if !IsValidGzip(tmpPath) {
			log.Warn().Str("source", label).Msg("selfupdate: downloaded file is not a valid gzip (HTML error page?)")
			continue
		}
		log.Info().Str("source", label).Str("path", tmpPath).Msg("selfupdate: IPK downloaded")
		return tmpPath, nil
	}

	_ = os.Remove(tmpPath)
	return "", fmt.Errorf("all download sources failed for %s", ipkName)
}

// ── upgrade script ─────────────────────────────────────────────────────────

// buildUpgradeScript returns a self-contained POSIX shell script that:
//  1. Waits 3 s for the triggering Go process to finish logging.
//  2. Stops the clashforge service (which terminates this Go process via SIGTERM,
//     allowing it to clean up nftables / DNS / mihomo before exiting).
//  3. Installs the new IPK via opkg.
//  4. Restarts the service.
//  5. Cleans up temp files.
func buildUpgradeScript(ipkPath string) string {
	return fmt.Sprintf(`#!/bin/sh
set -e
sleep 3
/etc/init.d/clashforge stop  2>/dev/null || true
sleep 2
opkg install --nodeps --force-downgrade %s
sleep 1
/etc/init.d/clashforge start 2>/dev/null || true
rm -f %s /tmp/clashforge-autoupgrade.sh
`, ipkPath, ipkPath)
}

func defaultSpawnUpgrade(ipkPath string) error {
	script := buildUpgradeScript(ipkPath)
	scriptPath := "/tmp/clashforge-autoupgrade.sh"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		return fmt.Errorf("write upgrade script: %w", err)
	}
	// nohup ensures the child survives when this process exits.
	return exec.Command("sh", "-c",
		fmt.Sprintf("nohup sh %s >>/tmp/clashforge-autoupgrade.log 2>&1 &", scriptPath)).Run()
}

// ── architecture detection ─────────────────────────────────────────────────

func detectArch() (string, error) {
	out, err := exec.Command("uname", "-m").Output()
	if err != nil {
		return "", fmt.Errorf("uname -m: %w", err)
	}
	machine := strings.TrimSpace(string(out))
	switch machine {
	case "x86_64", "amd64":
		return "x86_64", nil
	case "aarch64", "arm64":
		part, _ := readCPUPart()
		if part == "0xd03" {
			return "aarch64_cortex-a53", nil
		}
		return "aarch64_generic", nil
	default:
		return "", fmt.Errorf("unsupported architecture: %s", machine)
	}
}

func readCPUPart() (string, error) {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`(?m)^CPU part\s*:\s*(\S+)`)
	m := re.FindSubmatch(data)
	if len(m) < 2 {
		return "", fmt.Errorf("CPU part not found")
	}
	return strings.ToLower(string(m[1])), nil
}

// ── HTTP helpers ───────────────────────────────────────────────────────────

func buildHTTPClient(proxyURL string, timeout time.Duration) *http.Client {
	transport := &http.Transport{}
	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{Transport: transport, Timeout: timeout}
}

func httpGet(ctx context.Context, reqURL, proxyURL string, timeout time.Duration) ([]byte, error) {
	client := buildHTTPClient(proxyURL, timeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "clashforge-selfupdate/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, reqURL)
	}
	return io.ReadAll(resp.Body)
}

func httpDownload(ctx context.Context, reqURL, proxyURL, destPath string, timeout time.Duration) error {
	client := buildHTTPClient(proxyURL, timeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "clashforge-selfupdate/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.OpenFile(destPath, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// IsValidGzip checks the gzip magic bytes (1f 8b) to detect HTML error pages
// that some mirrors return with HTTP 200.
func IsValidGzip(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 2)
	n, err := f.Read(buf)
	return err == nil && n == 2 && buf[0] == 0x1f && buf[1] == 0x8b
}

// ── version comparison ─────────────────────────────────────────────────────

var semVerRe = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?`)

func hasVersionUpdate(current, latest string) bool {
	cur := strings.TrimPrefix(strings.TrimSpace(current), "v")
	lat := strings.TrimPrefix(strings.TrimSpace(latest), "v")
	if cur == "0.1.0-dev" {
		return false
	}
	cv, cok := parseSemVer(cur)
	lv, lok := parseSemVer(lat)
	if !cok || !lok {
		return cur != lat
	}
	return cmpSemVer(lv, cv) > 0
}

type semVer struct {
	major, minor, patch int
	pre                 []string
}

func parseSemVer(s string) (semVer, bool) {
	m := semVerRe.FindStringSubmatch(s)
	if len(m) == 0 {
		return semVer{}, false
	}
	maj, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	pat, _ := strconv.Atoi(m[3])
	var pre []string
	if m[4] != "" {
		for _, t := range strings.Split(m[4], ".") {
			if t != "" {
				pre = append(pre, t)
			}
		}
	}
	return semVer{maj, min, pat, pre}, true
}

func cmpSemVer(a, b semVer) int {
	if d := a.major - b.major; d != 0 {
		return sign(d)
	}
	if d := a.minor - b.minor; d != 0 {
		return sign(d)
	}
	if d := a.patch - b.patch; d != 0 {
		return sign(d)
	}
	return cmpPre(a.pre, b.pre)
}

func cmpPre(a, b []string) int {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	if len(a) == 0 {
		return 1
	}
	if len(b) == 0 {
		return -1
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := range n {
		ai, bi := a[i], b[i]
		an, aerr := strconv.Atoi(ai)
		bn, berr := strconv.Atoi(bi)
		var c int
		switch {
		case aerr == nil && berr == nil:
			c = sign(an - bn)
		case aerr == nil:
			c = -1
		case berr == nil:
			c = 1
		default:
			if ai < bi {
				c = -1
			} else if ai > bi {
				c = 1
			}
		}
		if c != 0 {
			return c
		}
	}
	return sign(len(a) - len(b))
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}

// ── time scheduling helpers ────────────────────────────────────────────────

// ParseUpdateTime parses "HH:MM" into (hour, minute). Returns (2, 0) on parse error.
func ParseUpdateTime(s string) (hour, minute int) {
	parts := strings.SplitN(strings.TrimSpace(s), ":", 2)
	if len(parts) != 2 {
		return 2, 0
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 2, 0
	}
	return h, m
}

// NextFireDuration returns the duration until the next occurrence of HH:MM in
// local time. The returned value is always positive (minimum 1 s).
func NextFireDuration(hour, minute int) time.Duration {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	if d := next.Sub(now); d >= time.Second {
		return d
	}
	return time.Second
}

// ── state snapshot ─────────────────────────────────────────────────────────

// StateSnapshot captures the key persisted files before an upgrade so tests
// can assert that nothing changed after the service restarts.
type StateSnapshot struct {
	ConfigToml    []byte            `json:"config_toml"`
	Subscriptions []byte            `json:"subscriptions"`
	Overrides     []byte            `json:"overrides"`
	ExtraFiles    map[string][]byte `json:"extra_files,omitempty"`
	CapturedAt    time.Time         `json:"captured_at"`
}

// BuildUpgradeScriptForTest exposes buildUpgradeScript for e2e tests that need
// to inspect the generated script content without actually spawning it.
func BuildUpgradeScriptForTest(ipkPath string) string {
	return buildUpgradeScript(ipkPath)
}

// CaptureState reads config.toml, subscriptions.json, and overrides.yaml from
// dataDir and returns a snapshot. Missing files are silently skipped.
func CaptureState(dataDir string) StateSnapshot {
	snap := StateSnapshot{CapturedAt: time.Now(), ExtraFiles: make(map[string][]byte)}
	snap.ConfigToml, _ = os.ReadFile(filepath.Join(dataDir, "config.toml"))
	snap.Subscriptions, _ = os.ReadFile(filepath.Join(dataDir, "subscriptions.json"))
	snap.Overrides, _ = os.ReadFile(filepath.Join(dataDir, "overrides.yaml"))
	return snap
}
