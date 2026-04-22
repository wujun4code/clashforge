package dns

import "testing"

func TestParseDnsmasqConfDir(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "plain", line: "conf-dir=/tmp/dnsmasq.cfg01411c.d", want: "/tmp/dnsmasq.cfg01411c.d"},
		{name: "with suffix filter", line: "conf-dir=/tmp/dnsmasq.d,.bak", want: "/tmp/dnsmasq.d"},
		{name: "with spaces", line: "  conf-dir=/tmp/dir  ", want: "/tmp/dir"},
		{name: "other line", line: "resolv-file=/tmp/resolv.conf", want: ""},
		{name: "empty", line: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDnsmasqConfDir(tt.line)
			if got != tt.want {
				t.Fatalf("parseDnsmasqConfDir(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}
