package main

import (
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/dns"
)

func TestCrashDoesNotRestorePlaintextDNSInAntiHijackMode(t *testing.T) {
	for _, strategy := range []string{config.DNSStrategyAntiHijack, " ANTI-HIJACK ", config.DNSStrategysplit} {
		t.Run(strategy, func(t *testing.T) {
			cfg := config.Default()
			cfg.DNS.Strategy = strategy
			var restored []dns.DnsmasqMode
			restoreDNSAfterCrash(cfg, func(mode dns.DnsmasqMode) error {
				restored = append(restored, mode)
				return nil
			})
			if strategy == config.DNSStrategysplit {
				if len(restored) != 2 {
					t.Errorf("ordinary mode must keep existing recovery: %v", restored)
				}
			} else if len(restored) != 0 {
				t.Errorf("strict mode must not restore ISP DNS after crash: %v", restored)
			}
		})
	}
}
