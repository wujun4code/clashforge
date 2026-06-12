// Package e2e contains end-to-end simulation tests for the self-update flow.
//
// Tests run entirely in-process using mock HTTP servers and stubbed OS calls.
// No real router, opkg, or internet access is required.
package e2e_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/selfupdate"
)

// ── test doubles ───────────────────────────────────────────────────────────

// newGzipBytes returns minimal valid gzip content (so IsValidGzip passes).
func newGzipBytes() []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write([]byte("fake ipk content"))
	_ = w.Close()
	return buf.Bytes()
}

// fakeTempIPK creates a temp file containing valid gzip bytes and registers
// cleanup on t. Returns the file path.
func fakeTempIPK(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp("", "fake-*.ipk")
	if err != nil {
		t.Fatalf("create temp ipk: %v", err)
	}
	_, _ = f.Write(newGzipBytes())
	_ = f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}

// releaseServer starts an httptest.Server that returns a GitHub-style
// releases/latest JSON with the given tag_name.
func releaseServer(tag string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": tag})
	}))
}

// proxyForwardServer starts an httptest.Server that acts as a simple HTTP
// forward proxy, rewriting the request host to targetBase.
func proxyForwardServer(targetBase string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := targetBase + r.URL.RequestURI()
		req, err := http.NewRequest(r.Method, target, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		for k, vs := range r.Header {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		buf := make([]byte, 32*1024)
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				_, _ = w.Write(buf[:n])
			}
			if readErr != nil {
				break
			}
		}
	}))
}

// portFromURL extracts the port number from "http://127.0.0.1:PORT".
func portFromURL(rawURL string) int {
	var port int
	fmt.Sscanf(rawURL, "http://127.0.0.1:%d", &port)
	return port
}

// populateDataDir writes realistic config artefacts into dir.
func populateDataDir(t *testing.T, dir string) {
	t.Helper()
	files := map[string]string{
		"config.toml": fmt.Sprintf(`[core]
binary = "/usr/bin/mihomo"
data_dir = %q

[update]
auto_self_update = true
self_update_time = "02:00"
self_update_channel = "stable"

[security]
api_secret = ""
`, dir),
		"subscriptions.json": `{"subscriptions":[{"id":"s1","name":"My Sub","url":"https://example.com/sub","enabled":true}]}`,
		"overrides.yaml":     "mode: rule\nallow-lan: true\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

// snapshotDiff returns a description of the first file that changed between two
// snapshots, or "" if they are identical.
func snapshotDiff(before, after selfupdate.StateSnapshot) string {
	type check struct {
		name   string
		b, a   []byte
	}
	for _, c := range []check{
		{"config.toml", before.ConfigToml, after.ConfigToml},
		{"subscriptions.json", before.Subscriptions, after.Subscriptions},
		{"overrides.yaml", before.Overrides, after.Overrides},
	} {
		if !bytes.Equal(c.b, c.a) {
			return fmt.Sprintf("%s changed:\n  before: %q\n  after:  %q", c.name, c.b, c.a)
		}
	}
	return ""
}

// noOpSpawn is a SpawnUpgrade stub that does nothing (simulates spawning
// without actually starting a shell process).
func noOpSpawn(_ string) error { return nil }

// ── scenario 1: proxy unreachable → skip, config untouched ────────────────

func TestAutoUpdate_SkipWhenProxyAndDirectBothFail(t *testing.T) {
	dir := t.TempDir()
	populateDataDir(t, dir)

	cfg := config.Default()
	cfg.Core.DataDir = dir
	cfg.Ports.Mixed = 19999 // nothing listening

	u := selfupdate.New(cfg, "v0.1.0-beta.1")
	u.SetSpawnUpgrade(noOpSpawn)
	u.SetDetectArch(func() (string, error) { return "x86_64", nil })
	// Point at a definitely-unreachable URL so both proxy and direct fail fast.
	u.SetGHReleasesLatestURL("http://127.0.0.1:19999/releases/latest")

	before := selfupdate.CaptureState(dir)
	u.Run()
	after := selfupdate.CaptureState(dir)

	res := u.LastResult()
	if res == nil {
		t.Fatal("expected a result")
	}
	if res.Success {
		t.Error("Success must be false when all sources fail")
	}
	if res.Error == "" {
		t.Error("Error must be non-empty when all sources fail")
	}
	if diff := snapshotDiff(before, after); diff != "" {
		t.Errorf("config changed despite skipped upgrade:\n%s", diff)
	}
}

// ── scenario 2: already at latest → skip, config untouched ───────────────

func TestAutoUpdate_SkipWhenAlreadyLatest(t *testing.T) {
	dir := t.TempDir()
	populateDataDir(t, dir)

	currentTag := "v0.1.0-beta.5"
	ghSrv := releaseServer(currentTag)
	defer ghSrv.Close()
	px := proxyForwardServer(ghSrv.URL)
	defer px.Close()

	cfg := config.Default()
	cfg.Core.DataDir = dir
	cfg.Ports.Mixed = portFromURL(px.URL)
	cfg.Update.SelfUpdateChannel = "stable"

	u := selfupdate.New(cfg, currentTag)
	u.SetSpawnUpgrade(noOpSpawn)
	u.SetDetectArch(func() (string, error) { return "x86_64", nil })
	u.SetGHReleasesLatestURL(ghSrv.URL + "/latest")

	var spawnCalled bool
	u.SetSpawnUpgrade(func(_ string) error { spawnCalled = true; return nil })

	before := selfupdate.CaptureState(dir)
	u.Run()
	after := selfupdate.CaptureState(dir)

	if spawnCalled {
		t.Error("upgrade must not be spawned when already at latest")
	}
	res := u.LastResult()
	if res == nil || !res.Skipped {
		t.Errorf("expected Skipped=true, got %+v", res)
	}
	if diff := snapshotDiff(before, after); diff != "" {
		t.Errorf("config changed despite no-op check:\n%s", diff)
	}
}

// ── scenario 3: update available via proxy → upgrade spawned ──────────────

func TestAutoUpdate_UpgradeSpawnedViaProxy(t *testing.T) {
	dir := t.TempDir()
	populateDataDir(t, dir)

	newTag := "v0.1.0-beta.6"
	ghSrv := releaseServer(newTag)
	defer ghSrv.Close()
	px := proxyForwardServer(ghSrv.URL)
	defer px.Close()

	cfg := config.Default()
	cfg.Core.DataDir = dir
	cfg.Ports.Mixed = portFromURL(px.URL)
	cfg.Update.SelfUpdateChannel = "stable"

	u := selfupdate.New(cfg, "v0.1.0-beta.5")
	u.SetDetectArch(func() (string, error) { return "x86_64", nil })
	u.SetGHReleasesLatestURL(ghSrv.URL + "/latest")
	u.SetIPKDownloadFn(func(_, _, _ string) (string, error) {
		return fakeTempIPK(t), nil
	})

	var spawnedIPK string
	u.SetSpawnUpgrade(func(p string) error { spawnedIPK = p; return nil })

	before := selfupdate.CaptureState(dir)
	u.Run()
	after := selfupdate.CaptureState(dir)

	if spawnedIPK == "" {
		t.Fatal("expected upgrade to be spawned")
	}
	res := u.LastResult()
	if res == nil || !res.Success {
		t.Fatalf("expected Success=true, got %+v", res)
	}
	if res.NewVersion != newTag {
		t.Errorf("NewVersion: want %q, got %q", newTag, res.NewVersion)
	}

	// KEY ASSERTION: all config files must be byte-for-byte identical.
	// The upgrade only replaces the binary; user data must be untouched.
	if diff := snapshotDiff(before, after); diff != "" {
		t.Errorf("SEAMLESS UPDATE FAILED — config changed:\n%s", diff)
	}
}

// ── scenario 4: proxy fails, direct succeeds → upgrade spawned ────────────

func TestAutoUpdate_FallsBackToDirectWhenProxyFails(t *testing.T) {
	dir := t.TempDir()
	populateDataDir(t, dir)

	newTag := "v0.1.0-beta.7"
	ghSrv := releaseServer(newTag)
	defer ghSrv.Close()

	cfg := config.Default()
	cfg.Core.DataDir = dir
	cfg.Ports.Mixed = 19998 // nothing listening — proxy will fail

	u := selfupdate.New(cfg, "v0.1.0-beta.5")
	u.SetDetectArch(func() (string, error) { return "x86_64", nil })
	// Proxy fetch fails (port 19998 not listening); direct fetch hits mock.
	u.SetGHReleasesLatestURL(ghSrv.URL + "/latest")
	u.SetIPKDownloadFn(func(_, _, _ string) (string, error) {
		return fakeTempIPK(t), nil
	})

	var spawnedIPK string
	u.SetSpawnUpgrade(func(p string) error { spawnedIPK = p; return nil })

	before := selfupdate.CaptureState(dir)
	u.Run()
	after := selfupdate.CaptureState(dir)

	if spawnedIPK == "" {
		t.Fatal("expected direct fallback to succeed and spawn upgrade")
	}
	if diff := snapshotDiff(before, after); diff != "" {
		t.Errorf("config changed during direct-fallback upgrade:\n%s", diff)
	}
}

// ── scenario 5: TriggerAsync rejects concurrent runs ─────────────────────

func TestAutoUpdate_TriggerAsyncRejectsConcurrentRun(t *testing.T) {
	cfg := config.Default()
	cfg.Ports.Mixed = 19997 // nothing listening

	u := selfupdate.New(cfg, "v0.1.0-beta.1")
	u.SetSpawnUpgrade(noOpSpawn)
	u.SetDetectArch(func() (string, error) { return "x86_64", nil })
	u.SetGHReleasesLatestURL("http://127.0.0.1:19997/releases/latest") // will fail fast

	gate := make(chan struct{})
	release := make(chan struct{})
	var hookOnce sync.Once

	u.SetPreRunHook(func() {
		hookOnce.Do(func() { close(gate) }) // signal that run() has started
		<-release                           // block until test releases it
	})

	// Start first run in background.
	go u.Run()
	<-gate // wait until run() is in progress

	// Second async trigger must be rejected.
	started := u.TriggerAsync()
	if started {
		t.Error("TriggerAsync must return false when a run is already in progress")
	}

	close(release) // unblock the first run
}

// ── scenario 6: detailed config preservation with rich data ───────────────

func TestAutoUpdate_ConfigPreservationDetailed(t *testing.T) {
	dir := t.TempDir()

	// Rich, realistic config artefacts with unicode and multi-line content.
	files := map[string]string{
		"config.toml": `[core]
binary = "/usr/bin/mihomo"
data_dir = "/etc/metaclash"
runtime_dir = "/var/run/metaclash"

[update]
auto_self_update = true
self_update_time = "03:30"
self_update_channel = "preview"

[security]
api_secret = "hunter2"
allow_lan = true
`,
		"subscriptions.json": `{"subscriptions":[
  {"id":"s1","name":"Primary 🚀","url":"https://sub.example.com/clash","enabled":true},
  {"id":"s2","name":"Backup  ⚡","url":"https://bak.example.com/clash","enabled":false}
]}`,
		"overrides.yaml": `mode: rule
allow-lan: true
proxy-groups:
  - name: "🚀 节点选择"
    type: select
    proxies: [DIRECT]
  - name: "♻️ 自动选择"
    type: url-test
    url: "http://www.gstatic.com/generate_204"
    interval: 300
    proxies: [DIRECT]
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	newTag := "v0.1.0-beta.9"
	ghSrv := releaseServer(newTag)
	defer ghSrv.Close()
	px := proxyForwardServer(ghSrv.URL)
	defer px.Close()

	cfg := config.Default()
	cfg.Core.DataDir = dir
	cfg.Ports.Mixed = portFromURL(px.URL)

	u := selfupdate.New(cfg, "v0.1.0-beta.8")
	u.SetDetectArch(func() (string, error) { return "x86_64", nil })
	u.SetGHReleasesLatestURL(ghSrv.URL + "/latest")
	u.SetIPKDownloadFn(func(_, _, _ string) (string, error) {
		return fakeTempIPK(t), nil
	})
	u.SetSpawnUpgrade(noOpSpawn)

	before := selfupdate.CaptureState(dir)
	u.Run()
	after := selfupdate.CaptureState(dir)

	if diff := snapshotDiff(before, after); diff != "" {
		t.Errorf("SEAMLESS UPDATE FAILED — config changed:\n%s", diff)
	}
	if after.CapturedAt.Before(before.CapturedAt) {
		t.Error("after snapshot captured before before snapshot")
	}
}

// ── scenario 7: upgrade script has correct stop→install→start order ───────

func TestAutoUpdate_UpgradeScriptCommandOrder(t *testing.T) {
	newTag := "v0.2.0"
	ghSrv := releaseServer(newTag)
	defer ghSrv.Close()
	px := proxyForwardServer(ghSrv.URL)
	defer px.Close()

	cfg := config.Default()
	cfg.Core.DataDir = t.TempDir()
	cfg.Ports.Mixed = portFromURL(px.URL)

	u := selfupdate.New(cfg, "v0.1.0")
	u.SetDetectArch(func() (string, error) { return "x86_64", nil })
	u.SetGHReleasesLatestURL(ghSrv.URL + "/latest")
	u.SetIPKDownloadFn(func(_, _, _ string) (string, error) {
		return fakeTempIPK(t), nil
	})

	// Capture the IPK path so we can reconstruct what the script would look like.
	var capturedIPK string
	u.SetSpawnUpgrade(func(ipkPath string) error {
		capturedIPK = ipkPath
		return nil
	})

	u.Run()

	if capturedIPK == "" {
		t.Fatal("no upgrade was spawned")
	}

	script := selfupdate.BuildUpgradeScriptForTest(capturedIPK)

	stopIdx := indexInStr(script, "clashforge stop")
	installIdx := indexInStr(script, "opkg install")
	startIdx := indexInStr(script, "clashforge start")

	if stopIdx < 0 || installIdx < 0 || startIdx < 0 {
		t.Fatalf("upgrade script missing required commands:\n%s", script)
	}
	if stopIdx > installIdx {
		t.Errorf("stop (%d) must appear before opkg install (%d)", stopIdx, installIdx)
	}
	if installIdx > startIdx {
		t.Errorf("opkg install (%d) must appear before start (%d)", installIdx, startIdx)
	}
}

// ── scenario 8: Result fields are populated correctly ─────────────────────

func TestAutoUpdate_ResultFieldsOnSuccess(t *testing.T) {
	newTag := "v0.1.0-beta.10"
	ghSrv := releaseServer(newTag)
	defer ghSrv.Close()
	px := proxyForwardServer(ghSrv.URL)
	defer px.Close()

	cfg := config.Default()
	cfg.Core.DataDir = t.TempDir()
	cfg.Ports.Mixed = portFromURL(px.URL)

	currentVersion := "v0.1.0-beta.9"
	u := selfupdate.New(cfg, currentVersion)
	u.SetDetectArch(func() (string, error) { return "x86_64", nil })
	u.SetGHReleasesLatestURL(ghSrv.URL + "/latest")
	u.SetIPKDownloadFn(func(_, _, _ string) (string, error) {
		return fakeTempIPK(t), nil
	})
	u.SetSpawnUpgrade(noOpSpawn)

	t0 := time.Now()
	u.Run()
	t1 := time.Now()

	res := u.LastResult()
	if res == nil {
		t.Fatal("no result recorded")
	}
	if !res.Success {
		t.Errorf("expected Success=true, got error=%q", res.Error)
	}
	if res.OldVersion != currentVersion {
		t.Errorf("OldVersion: want %q, got %q", currentVersion, res.OldVersion)
	}
	if res.NewVersion != newTag {
		t.Errorf("NewVersion: want %q, got %q", newTag, res.NewVersion)
	}
	if res.RunAt.Before(t0) || res.RunAt.After(t1) {
		t.Errorf("RunAt %v out of range [%v, %v]", res.RunAt, t0, t1)
	}
	if res.Skipped {
		t.Error("Skipped must be false on successful upgrade")
	}
	if res.Error != "" {
		t.Errorf("Error must be empty on success, got %q", res.Error)
	}
}

// ── helpers ────────────────────────────────────────────────────────────────

func indexInStr(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	for i := range len(s) - len(sub) + 1 {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
