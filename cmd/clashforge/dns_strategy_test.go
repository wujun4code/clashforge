package main

import (
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/dns"
)

func TestCrashDNSPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, strategy string
		enable, apply  bool
		mode           string
		want           int
	}{
		{"strict", config.DNSStrategyAntiHijack, true, true, "upstream", 0},
		{"normalized-strict", " ANTI-HIJACK ", true, true, "replace", 0},
		{"split", config.DNSStrategysplit, true, true, "upstream", 2},
		{"disabled", config.DNSStrategyAntiHijack, false, true, "upstream", 2},
		{"not-managed", config.DNSStrategyAntiHijack, true, false, "upstream", 2},
		{"mode-none", config.DNSStrategyAntiHijack, true, true, "none", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.DNS.Strategy, cfg.DNS.Enable, cfg.DNS.ApplyOnStart, cfg.DNS.DnsmasqMode = tc.strategy, tc.enable, tc.apply, tc.mode
			var restored []dns.DnsmasqMode
			if err := restoreDNSAfterCrash(cfg, func(mode dns.DnsmasqMode) error { restored = append(restored, mode); return nil }); err != nil {
				t.Fatal(err)
			}
			if len(restored) != tc.want {
				t.Fatalf("crash DNS policy: got %v, want %d restore calls", restored, tc.want)
			}
			if tc.want == 2 && (restored[0] != dns.ModeReplace || restored[1] != dns.ModeUpstream) {
				t.Fatalf("wrong release order: %v", restored)
			}
		})
	}
}
