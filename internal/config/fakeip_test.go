package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
)

// TestSelectFakeIPRange_ColdBoot verifies that with no rule-provider cache on
// disk yet (first boot), the historical default 198.18.0.1/16 is rejected
// because it collides with the IANA baseline's 198.18.0.0/15 entry, and the
// second candidate is chosen instead.
func TestSelectFakeIPRange_ColdBoot(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "no-cache-yet")

	got, warning := config.SelectFakeIPRange(dir)

	if warning != "" {
		t.Fatalf("expected no warning, got %q", warning)
	}
	if got == "198.18.0.1/16" {
		t.Fatalf("expected the 198.18.0.0/15-colliding default to be rejected, got %q", got)
	}
}

// TestSelectFakeIPRange_RejectsWhateverLancidrActuallyContains proves the
// check is driven by the real cached rule-provider payload, not just the
// hardcoded IANA list: a lancidr.yaml that (hypothetically) also reserves the
// second candidate's block must push selection to the third one.
func TestSelectFakeIPRange_RejectsWhateverLancidrActuallyContains(t *testing.T) {
	dir := t.TempDir()
	ruleDir := filepath.Join(dir, "rule_provider")
	if err := os.MkdirAll(ruleDir, 0o755); err != nil {
		t.Fatalf("mkdir rule_provider: %v", err)
	}
	// Pretend this rule-set also reserves 28.0.0.0/8 (the second candidate's block).
	lancidr := "payload:\n  - '28.0.0.0/8'\n"
	if err := os.WriteFile(filepath.Join(ruleDir, "lancidr.yaml"), []byte(lancidr), 0o644); err != nil {
		t.Fatalf("write lancidr.yaml: %v", err)
	}

	got, warning := config.SelectFakeIPRange(dir)

	if warning != "" {
		t.Fatalf("expected no warning, got %q", warning)
	}
	if got == "198.18.0.1/16" || got == "28.18.0.1/16" {
		t.Fatalf("expected both the default and the 28.0.0.0/8-colliding candidate to be rejected, got %q", got)
	}
}

// TestSelectFakeIPRange_AllCandidatesCollide exercises the extremely-unlikely
// "every built-in candidate collides" path and checks it still returns a
// usable CIDR (never an empty string) plus a non-empty warning.
func TestSelectFakeIPRange_AllCandidatesCollide(t *testing.T) {
	dir := t.TempDir()
	ruleDir := filepath.Join(dir, "rule_provider")
	if err := os.MkdirAll(ruleDir, 0o755); err != nil {
		t.Fatalf("mkdir rule_provider: %v", err)
	}
	// Reserve the whole IPv4 space so no candidate can possibly clear the check.
	lancidr := "payload:\n  - '0.0.0.0/0'\n"
	if err := os.WriteFile(filepath.Join(ruleDir, "lancidr.yaml"), []byte(lancidr), 0o644); err != nil {
		t.Fatalf("write lancidr.yaml: %v", err)
	}

	got, warning := config.SelectFakeIPRange(dir)

	if got == "" {
		t.Fatal("expected a non-empty fallback CIDR even when every candidate collides")
	}
	if warning == "" {
		t.Fatal("expected a warning when every candidate collides")
	}
}
