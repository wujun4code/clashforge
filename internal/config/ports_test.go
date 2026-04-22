package config_test

import (
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
)

func TestSelectCompatiblePorts_FallbackToCoexistWhenCommunityOccupied(t *testing.T) {
	cfg := config.Default()
	cfg.Ports.HTTP = 7890
	cfg.Ports.SOCKS = 7891
	cfg.Ports.Mixed = 7893
	cfg.Ports.Redir = 7892
	cfg.Ports.TProxy = 7895
	cfg.Ports.DNS = 7874
	cfg.Ports.MihomoAPI = 9090

	occupied := map[int]bool{7890: true, 7891: true, 7892: true, 7893: true, 7895: true, 7874: true, 9090: true}
	adjustments := config.SelectCompatiblePorts(cfg, config.PortSelectionOptions{
		OccupiedChecker: func(port int, _ bool) bool { return occupied[port] },
	})
	if len(adjustments) != 7 {
		t.Fatalf("expected 7 port adjustments, got %d", len(adjustments))
	}
	if cfg.Ports.HTTP != 17890 || cfg.Ports.SOCKS != 17891 || cfg.Ports.Mixed != 17893 || cfg.Ports.Redir != 17892 || cfg.Ports.TProxy != 17895 || cfg.Ports.DNS != 17874 || cfg.Ports.MihomoAPI != 19090 {
		t.Fatalf("legacy ports were not remapped to coexist ports: %+v", cfg.Ports)
	}
}

func TestSelectCompatiblePorts_PreferCommunityDefaultsWhenAvailable(t *testing.T) {
	cfg := config.Default()
	cfg.Ports.HTTP = 17890
	cfg.Ports.SOCKS = 17891
	cfg.Ports.Mixed = 17893
	cfg.Ports.Redir = 17892
	cfg.Ports.TProxy = 17895
	cfg.Ports.DNS = 17874
	cfg.Ports.MihomoAPI = 19090

	adjustments := config.SelectCompatiblePorts(cfg, config.PortSelectionOptions{
		PreferCommunityDefaults: true,
		OccupiedChecker:         func(_ int, _ bool) bool { return false },
	})

	if len(adjustments) != 7 {
		t.Fatalf("expected 7 port adjustments, got %d", len(adjustments))
	}
	if cfg.Ports.HTTP != 7890 || cfg.Ports.SOCKS != 7891 || cfg.Ports.Mixed != 7893 || cfg.Ports.Redir != 7892 || cfg.Ports.TProxy != 7895 || cfg.Ports.DNS != 7874 || cfg.Ports.MihomoAPI != 9090 {
		t.Fatalf("coexist ports were not restored to community defaults: %+v", cfg.Ports)
	}
}

func TestApplyManagedRuntimeSettingsOverridesLegacyPorts(t *testing.T) {
	cfg := config.Default()
	merged := map[string]interface{}{
		"port":                7890,
		"socks-port":          7891,
		"mixed-port":          7893,
		"redir-port":          7892,
		"tproxy-port":         7895,
		"external-controller": "0.0.0.0:9090",
		"dns": map[string]interface{}{
			"enable": true,
			"listen": "0.0.0.0:7874",
		},
	}

	result := config.ApplyManagedRuntimeSettings(cfg, merged)
	if result["port"] != 17890 || result["socks-port"] != 17891 || result["mixed-port"] != 17893 || result["redir-port"] != 17892 || result["tproxy-port"] != 17895 {
		t.Fatalf("managed runtime ports were not enforced: %+v", result)
	}
	if result["external-controller"] != "127.0.0.1:19090" {
		t.Fatalf("expected external-controller to be normalized, got %v", result["external-controller"])
	}
	dnsMap, ok := result["dns"].(map[string]interface{})
	if !ok {
		t.Fatal("expected dns map to remain present")
	}
	if dnsMap["listen"] != "0.0.0.0:17874" {
		t.Fatalf("expected dns.listen to be normalized, got %v", dnsMap["listen"])
	}
}
