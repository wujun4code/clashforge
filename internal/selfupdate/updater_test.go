package selfupdate

import (
	"os"
	"testing"
	"time"
)

// ── ParseUpdateTime ────────────────────────────────────────────────────────

func TestParseUpdateTime_ValidMidnight(t *testing.T) {
	h, m := ParseUpdateTime("02:00")
	if h != 2 || m != 0 {
		t.Fatalf("expected 2:00, got %d:%02d", h, m)
	}
}

func TestParseUpdateTime_ValidNoon(t *testing.T) {
	h, m := ParseUpdateTime("12:30")
	if h != 12 || m != 30 {
		t.Fatalf("expected 12:30, got %d:%02d", h, m)
	}
}

func TestParseUpdateTime_LeadingZero(t *testing.T) {
	h, m := ParseUpdateTime("00:05")
	if h != 0 || m != 5 {
		t.Fatalf("expected 0:05, got %d:%02d", h, m)
	}
}

func TestParseUpdateTime_InvalidFormat_DefaultsTo2AM(t *testing.T) {
	for _, bad := range []string{"", "25:00", "12:60", "abc", "1200", "12:"} {
		h, m := ParseUpdateTime(bad)
		if h != 2 || m != 0 {
			t.Errorf("input %q: expected default 2:00, got %d:%02d", bad, h, m)
		}
	}
}

// ── NextFireDuration ───────────────────────────────────────────────────────

func TestNextFireDuration_AlwaysPositive(t *testing.T) {
	for h := range 24 {
		for m := range 60 {
			d := NextFireDuration(h, m)
			if d < time.Second {
				t.Errorf("NextFireDuration(%d,%d) = %v, want >= 1s", h, m, d)
			}
		}
	}
}

func TestNextFireDuration_AtMostOneDay(t *testing.T) {
	for h := range 24 {
		d := NextFireDuration(h, 0)
		if d > 25*time.Hour {
			t.Errorf("NextFireDuration(%d,0) = %v, want <= 25h", h, d)
		}
	}
}

func TestNextFireDuration_PastTimeScheduledTomorrow(t *testing.T) {
	// A time that has already passed today must be scheduled for tomorrow.
	now := time.Now()
	// Use an hour that is definitely in the past (midnight → always past unless it's exactly midnight).
	// To be safe: pick now.Hour - 1 (or 23 if now.Hour == 0).
	pastHour := now.Hour() - 1
	if pastHour < 0 {
		pastHour = 23
	}
	d := NextFireDuration(pastHour, 0)
	// Must be more than 22 hours away (it's tomorrow).
	if d < 22*time.Hour {
		t.Errorf("past time %02d:00: NextFireDuration = %v, expected > 22h (tomorrow)", pastHour, d)
	}
}

// ── hasVersionUpdate ───────────────────────────────────────────────────────

func TestHasVersionUpdate_NewerPatch(t *testing.T) {
	if !hasVersionUpdate("0.1.0-beta.1", "v0.1.0-beta.2") {
		t.Error("expected update: beta.1 → beta.2")
	}
}

func TestHasVersionUpdate_SameVersion(t *testing.T) {
	if hasVersionUpdate("v0.1.0-beta.2", "v0.1.0-beta.2") {
		t.Error("same version must not report update")
	}
}

func TestHasVersionUpdate_OlderLatest(t *testing.T) {
	if hasVersionUpdate("v0.1.0-beta.3", "v0.1.0-beta.2") {
		t.Error("older latest must not report update")
	}
}

func TestHasVersionUpdate_DevBuildNeverUpdates(t *testing.T) {
	if hasVersionUpdate("0.1.0-dev", "v9.9.9") {
		t.Error("dev build must never report update")
	}
}

func TestHasVersionUpdate_MajorVersionBump(t *testing.T) {
	if !hasVersionUpdate("v0.1.0-beta.5", "v1.0.0") {
		t.Error("expected update: pre-release → stable major release")
	}
}

func TestHasVersionUpdate_ReleaseToStable(t *testing.T) {
	if !hasVersionUpdate("v0.1.0-beta.1", "v0.1.0") {
		t.Error("expected update: pre-release → stable patch")
	}
}

// ── IsValidGzip ────────────────────────────────────────────────────────────

func TestIsValidGzip_ValidMagic(t *testing.T) {
	f, err := os.CreateTemp("", "test-*.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.Write([]byte{0x1f, 0x8b, 0x00, 0x00}) // gzip magic + dummy bytes
	f.Close()

	if !IsValidGzip(f.Name()) {
		t.Error("expected true for file with gzip magic bytes")
	}
}

func TestIsValidGzip_HTMLErrorPage(t *testing.T) {
	f, err := os.CreateTemp("", "test-*.html")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("<html>Error 404</html>")
	f.Close()

	if IsValidGzip(f.Name()) {
		t.Error("expected false for HTML content")
	}
}

func TestIsValidGzip_EmptyFile(t *testing.T) {
	f, err := os.CreateTemp("", "test-*.empty")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.Close()

	if IsValidGzip(f.Name()) {
		t.Error("expected false for empty file")
	}
}

func TestIsValidGzip_NonExistentFile(t *testing.T) {
	if IsValidGzip("/tmp/does-not-exist-clashforge-test.ipk") {
		t.Error("expected false for non-existent file")
	}
}

// ── buildUpgradeScript ─────────────────────────────────────────────────────

func TestBuildUpgradeScript_ContainsIPKPath(t *testing.T) {
	ipk := "/tmp/clashforge_0.1.0-beta.3_x86_64.ipk"
	script := buildUpgradeScript(ipk)
	if !containsStr(script, ipk) {
		t.Errorf("upgrade script does not reference IPK path %q", ipk)
	}
}

func TestBuildUpgradeScript_ContainsOpkgInstall(t *testing.T) {
	script := buildUpgradeScript("/tmp/test.ipk")
	if !containsStr(script, "opkg install") {
		t.Error("upgrade script must contain 'opkg install'")
	}
}

func TestBuildUpgradeScript_ContainsInitdStop(t *testing.T) {
	script := buildUpgradeScript("/tmp/test.ipk")
	if !containsStr(script, "/etc/init.d/clashforge stop") {
		t.Error("upgrade script must stop the service before installing")
	}
}

func TestBuildUpgradeScript_ContainsInitdStart(t *testing.T) {
	script := buildUpgradeScript("/tmp/test.ipk")
	if !containsStr(script, "/etc/init.d/clashforge start") {
		t.Error("upgrade script must restart the service after installing")
	}
}

func TestBuildUpgradeScript_StopBeforeInstall(t *testing.T) {
	script := buildUpgradeScript("/tmp/test.ipk")
	stopIdx := indexStr(script, "clashforge stop")
	installIdx := indexStr(script, "opkg install")
	startIdx := indexStr(script, "clashforge start")
	if stopIdx < 0 || installIdx < 0 || startIdx < 0 {
		t.Fatal("script missing stop / install / start commands")
	}
	if stopIdx > installIdx {
		t.Error("stop must appear before opkg install in upgrade script")
	}
	if installIdx > startIdx {
		t.Error("opkg install must appear before start in upgrade script")
	}
}

// ── CaptureState ──────────────────────────────────────────────────────────

func TestCaptureState_ReadsExistingFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir+"/config.toml", "test-config")
	writeFile(t, dir+"/subscriptions.json", `{"subs":[]}`)
	writeFile(t, dir+"/overrides.yaml", "mode: rule")

	snap := CaptureState(dir)
	if string(snap.ConfigToml) != "test-config" {
		t.Errorf("ConfigToml: got %q", snap.ConfigToml)
	}
	if string(snap.Subscriptions) != `{"subs":[]}` {
		t.Errorf("Subscriptions: got %q", snap.Subscriptions)
	}
	if string(snap.Overrides) != "mode: rule" {
		t.Errorf("Overrides: got %q", snap.Overrides)
	}
}

func TestCaptureState_MissingFilesAreNil(t *testing.T) {
	dir := t.TempDir()
	snap := CaptureState(dir)
	if snap.ConfigToml != nil {
		t.Error("expected nil ConfigToml for missing file")
	}
	if snap.Subscriptions != nil {
		t.Error("expected nil Subscriptions for missing file")
	}
	if snap.Overrides != nil {
		t.Error("expected nil Overrides for missing file")
	}
}

func TestCaptureState_TimestampSet(t *testing.T) {
	before := time.Now()
	snap := CaptureState(t.TempDir())
	after := time.Now()
	if snap.CapturedAt.Before(before) || snap.CapturedAt.After(after) {
		t.Errorf("CapturedAt %v not in [%v, %v]", snap.CapturedAt, before, after)
	}
}

// ── helpers ────────────────────────────────────────────────────────────────

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func containsStr(s, sub string) bool {
	return indexStr(s, sub) >= 0
}

func indexStr(s, sub string) int {
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
