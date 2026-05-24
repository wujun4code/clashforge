package api

import "testing"

func TestHasVersionUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{
			name:    "newer stable patch",
			current: "v0.1.0",
			latest:  "v0.1.1",
			want:    true,
		},
		{
			name:    "same version no update",
			current: "0.1.0-beta.1",
			latest:  "v0.1.0-beta.1",
			want:    false,
		},
		{
			name:    "beta prerelease bump",
			current: "0.1.0-beta.1",
			latest:  "v0.1.0-beta.2",
			want:    true,
		},
		{
			name:    "downgrade beta",
			current: "0.1.0-beta.3",
			latest:  "v0.1.0-beta.2",
			want:    false,
		},
		{
			name:    "historical rc to beta migration",
			current: "0.1.0-rc.84",
			latest:  "v0.1.0-beta.1",
			want:    true,
		},
		{
			name:    "rc to stable upgrade",
			current: "0.1.0-rc.84",
			latest:  "v0.1.0",
			want:    true,
		},
		{
			name:    "dev build keeps update disabled",
			current: "0.1.0-dev",
			latest:  "v0.1.0-beta.1",
			want:    false,
		},
		{
			name:    "non semver fallback compares inequality",
			current: "dev-build-42",
			latest:  "v0.1.0-beta.1",
			want:    true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := hasVersionUpdate(tt.current, tt.latest)
			if got != tt.want {
				t.Fatalf("hasVersionUpdate(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}
