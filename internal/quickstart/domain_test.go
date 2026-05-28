package quickstart

import "testing"

func TestDomainMatchesZone(t *testing.T) {
	tests := []struct {
		domain string
		zone   string
		ok     bool
	}{
		{domain: "a.example.com", zone: "example.com", ok: true},
		{domain: "example.com", zone: "example.com", ok: true},
		{domain: "example.net", zone: "example.com", ok: false},
		{domain: "", zone: "example.com", ok: false},
	}
	for _, tt := range tests {
		if got := DomainMatchesZone(tt.domain, tt.zone); got != tt.ok {
			t.Fatalf("DomainMatchesZone(%q,%q)=%v want %v", tt.domain, tt.zone, got, tt.ok)
		}
	}
}

func TestResolvePublishBaseDomain(t *testing.T) {
	tests := []struct {
		name     string
		nodeHost string
		zoneName string
		want     string
	}{
		{
			name:     "prefer normalized zone",
			nodeHost: "xxx.top-domain.com",
			zoneName: "top-domain.com",
			want:     "top-domain.com",
		},
		{
			name:     "fallback to remove left-most label",
			nodeHost: "xxx.top-domain.com",
			zoneName: "",
			want:     "top-domain.com",
		},
		{
			name:     "apex host keeps itself",
			nodeHost: "top-domain.com",
			zoneName: "",
			want:     "top-domain.com",
		},
	}

	for _, tt := range tests {
		if got := ResolvePublishBaseDomain(tt.nodeHost, tt.zoneName); got != tt.want {
			t.Fatalf("%s: ResolvePublishBaseDomain(%q,%q)=%q want %q", tt.name, tt.nodeHost, tt.zoneName, got, tt.want)
		}
	}
}

func TestBuildPublishHostname(t *testing.T) {
	got := BuildPublishHostname("cf-sub-abcd", "xxx.top-domain.com", "top-domain.com")
	if got != "cf-sub-abcd.top-domain.com" {
		t.Fatalf("BuildPublishHostname()=%q", got)
	}
}
