package nodes

import (
	"strings"
	"testing"
)

// TestGeneratePass_Length verifies the generated password has exactly the requested length.
func TestGeneratePass_Length(t *testing.T) {
	for _, length := range []int{8, 16, 24, 32} {
		got := generatePass(length)
		if len(got) != length {
			t.Errorf("generatePass(%d) returned length %d, want %d", length, len(got), length)
		}
	}
}

// TestGeneratePass_OnlyValidChars ensures every character is from the declared charset.
func TestGeneratePass_OnlyValidChars(t *testing.T) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*-_=+"
	pass := generatePass(256)
	for i, c := range pass {
		if !strings.ContainsRune(charset, c) {
			t.Errorf("generatePass char at index %d (%q) is not in charset", i, c)
		}
	}
}

// TestGeneratePass_NonDeterministic runs the generator multiple times and expects at least
// two distinct values (probability of collision with 16-char password is negligible).
func TestGeneratePass_NonDeterministic(t *testing.T) {
	seen := make(map[string]struct{}, 5)
	for i := 0; i < 5; i++ {
		seen[generatePass(16)] = struct{}{}
	}
	if len(seen) < 2 {
		t.Error("generatePass produced identical values across 5 calls — suspected non-random source")
	}
}

// TestFullModeDetection exercises the same condition used inside DeployGOST to choose
// between bootstrap and full deployment phases.
func TestFullModeDetection(t *testing.T) {
	isFullMode := func(n *Node) bool {
		return strings.TrimSpace(n.Domain) != "" &&
			strings.TrimSpace(n.Email) != "" &&
			strings.TrimSpace(n.CFToken) != ""
	}

	cases := []struct {
		name     string
		node     Node
		wantFull bool
	}{
		{
			name:     "all fields set → full mode",
			node:     Node{Domain: "edge.example.com", Email: "ops@example.com", CFToken: "tok"},
			wantFull: true,
		},
		{
			name:     "domain only → bootstrap",
			node:     Node{Domain: "edge.example.com"},
			wantFull: false,
		},
		{
			name:     "email only → bootstrap",
			node:     Node{Email: "ops@example.com"},
			wantFull: false,
		},
		{
			name:     "token only → bootstrap",
			node:     Node{CFToken: "tok"},
			wantFull: false,
		},
		{
			name:     "domain+email, no token → bootstrap",
			node:     Node{Domain: "edge.example.com", Email: "ops@example.com"},
			wantFull: false,
		},
		{
			name:     "whitespace-only domain → bootstrap",
			node:     Node{Domain: "  ", Email: "ops@example.com", CFToken: "tok"},
			wantFull: false,
		},
		{
			name:     "empty node → bootstrap",
			node:     Node{},
			wantFull: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isFullMode(&tc.node)
			if got != tc.wantFull {
				t.Errorf("isFullMode = %v, want %v", got, tc.wantFull)
			}
		})
	}
}

// TestDomainAssembly mirrors the handleSaveDomain logic from the frontend:
// fullDomain = prefix + "." + zone when zone and prefix are non-empty.
func TestDomainAssembly(t *testing.T) {
	assembleDomain := func(zone, prefix, fallback string) string {
		zone = strings.TrimSpace(zone)
		prefix = strings.TrimSpace(prefix)
		if zone != "" && prefix != "" {
			return prefix + "." + zone
		}
		return strings.TrimSpace(fallback)
	}

	cases := []struct {
		zone, prefix, fallback string
		want                   string
	}{
		{"example.com", "edge-01", "", "edge-01.example.com"},
		{"example.com", "market-99", "", "market-99.example.com"},
		{"", "edge-01", "edge-01.example.com", "edge-01.example.com"},       // no zone → fallback
		{"example.com", "", "manual.example.com", "manual.example.com"},      // no prefix → fallback
		{"example.com", "  edge  ", "", "edge.example.com"},                  // trims prefix
		{" example.com ", "edge", "", "edge.example.com"},                     // trims zone
	}

	for _, tc := range cases {
		got := assembleDomain(tc.zone, tc.prefix, tc.fallback)
		if got != tc.want {
			t.Errorf("assembleDomain(%q, %q, %q) = %q, want %q",
				tc.zone, tc.prefix, tc.fallback, got, tc.want)
		}
	}
}

// TestPrefixExtraction mirrors the prefix-extraction logic that runs after zones are fetched:
// if the stored domain ends with ".<zone>", strip that suffix to recover the prefix.
func TestPrefixExtraction(t *testing.T) {
	extractPrefix := func(domain, zone string) string {
		domain = strings.TrimSpace(domain)
		zone = strings.ToLower(strings.TrimSpace(zone))
		lower := strings.ToLower(domain)
		suffix := "." + zone
		if strings.HasSuffix(lower, suffix) {
			return domain[:len(domain)-len(suffix)]
		}
		return ""
	}

	cases := []struct {
		domain, zone string
		want         string
	}{
		{"edge-01.example.com", "example.com", "edge-01"},
		{"market-99.example.com", "example.com", "market-99"},
		{"EDGE-01.Example.COM", "example.com", "EDGE-01"},    // case-insensitive suffix match
		{"example.com", "example.com", ""},                   // domain IS the zone, no prefix
		{"other.io", "example.com", ""},                      // different zone → empty
		{"", "example.com", ""},                              // empty domain → empty
	}

	for _, tc := range cases {
		got := extractPrefix(tc.domain, tc.zone)
		if got != tc.want {
			t.Errorf("extractPrefix(%q, %q) = %q, want %q", tc.domain, tc.zone, got, tc.want)
		}
	}
}
