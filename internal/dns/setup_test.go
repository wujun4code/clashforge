package dns

import "testing"

func TestUpstreamRestoresPortBeforeCommit(t *testing.T) {
	f := newDNSFixture(t, map[string]fakeDNSValue{"port": {Values: []string{"0"}}})
	t.Setenv("DNS_TEST_FAIL", "uci commit")
	if err := setupUpstreamUCI(17874); err == nil {
		t.Fatal("expected simulated commit failure")
	}
	if got := f.read()["port"]; len(got.Values) != 1 || got.Values[0] != "53" {
		t.Fatal("previous replace port=0 survives upstream setup")
	}
}
func TestUpstreamKeeps53AndSurfacesRestartFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "success"
		if fail {
			name = "restart failure"
		}
		t.Run(name, func(t *testing.T) {
			f := newDNSFixture(t, map[string]fakeDNSValue{"port": {Values: []string{"0"}}})
			if fail {
				t.Setenv("DNS_TEST_FAIL", "service dnsmasq restart")
			}
			err := Setup(ModeUpstream, 17874)
			if (err != nil) != fail {
				t.Fatalf("Setup error=%v, want failure=%t", err, fail)
			}
			if got := f.read()["port"]; len(got.Values) != 1 || got.Values[0] != "53" {
				t.Fatal("upstream loses local :53")
			}
		})
	}
}

func TestParseDnsmasqConfDir(t *testing.T) {
	tests := []struct{ name, line, want string }{
		{"plain", "conf-dir=/tmp/dnsmasq.cfg01411c.d", "/tmp/dnsmasq.cfg01411c.d"},
		{"with suffix filter", "conf-dir=/tmp/dnsmasq.d,.bak", "/tmp/dnsmasq.d"},
		{"with spaces", "  conf-dir=/tmp/dir  ", "/tmp/dir"},
		{"other line", "resolv-file=/tmp/resolv.conf", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseDnsmasqConfDir(tt.line); got != tt.want {
				t.Fatalf("parseDnsmasqConfDir(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}
