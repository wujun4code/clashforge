package config_test

import (
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/subscription"
)

func defaultTestCfg() *config.MetaclashConfig {
	cfg := config.Default()
	cfg.Network.BypassChina = true
	return cfg
}

func TestGenerate_BasicStructure(t *testing.T) {
	cfg := defaultTestCfg()
	result, err := config.Generate(cfg, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Check required top-level keys
	requiredKeys := []string{"port", "socks-port", "allow-lan", "mode", "log-level",
		"external-controller", "proxies", "proxy-groups", "rules"}
	for _, k := range requiredKeys {
		if _, ok := result[k]; !ok {
			t.Errorf("missing key: %s", k)
		}
	}

	if result["port"] != cfg.Ports.HTTP {
		t.Errorf("expected port=%d, got %v", cfg.Ports.HTTP, result["port"])
	}
	if result["mode"] != "rule" {
		t.Errorf("expected mode=rule, got %v", result["mode"])
	}
}

func TestGenerate_DNSConfig(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.DNS.Enable = true
	cfg.DNS.Mode = "fake-ip"

	result, err := config.Generate(cfg, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	dns, ok := result["dns"].(map[string]interface{})
	if !ok {
		t.Fatal("dns section not found or wrong type")
	}
	if dns["enhanced-mode"] != "fake-ip" {
		t.Errorf("expected enhanced-mode=fake-ip, got %v", dns["enhanced-mode"])
	}
	if _, ok := dns["fake-ip-range"]; !ok {
		t.Error("fake-ip-range missing")
	}
}

func TestGenerate_BypassChinaRules(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.Network.BypassChina = true

	result, err := config.Generate(cfg, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	rules, _ := result["rules"].([]string)
	foundGeosite := false
	foundGeoIP := false
	for _, r := range rules {
		if r == "GEOSITE,cn,DIRECT" {
			foundGeosite = true
		}
		if r == "GEOIP,CN,DIRECT,no-resolve" {
			foundGeoIP = true
		}
	}
	if !foundGeosite {
		t.Error("expected GEOSITE,cn,DIRECT rule when bypass_china=true")
	}
	if !foundGeoIP {
		t.Error("expected GEOIP,CN,DIRECT,no-resolve rule when bypass_china=true")
	}
}

func TestGenerate_WithNodes(t *testing.T) {
	cfg := defaultTestCfg()
	nodes := []subscription.ProxyNode{
		{Name: "SG-01", Type: "ss", Server: "sg.example.com", Port: 8443,
			Extra: map[string]interface{}{"cipher": "aes-256-gcm", "password": "test"}},
		{Name: "JP-01", Type: "vmess", Server: "jp.example.com", Port: 443,
			Extra: map[string]interface{}{"uuid": "deadbeef-1234", "alterId": 0}},
	}

	result, err := config.Generate(cfg, nodes)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	proxies, _ := result["proxies"].([]map[string]interface{})
	if len(proxies) != 2 {
		t.Fatalf("expected 2 proxies, got %d", len(proxies))
	}

	// First proxy should have correct fields
	p := proxies[0]
	if p["name"] != "SG-01" {
		t.Errorf("expected name=SG-01, got %v", p["name"])
	}
	if p["cipher"] != "aes-256-gcm" {
		t.Errorf("expected cipher from Extra, got %v", p["cipher"])
	}

	// Proxy groups should contain node names
	groups, _ := result["proxy-groups"].([]map[string]interface{})
	if len(groups) == 0 {
		t.Fatal("proxy-groups is empty")
	}
	// Auto group should contain both node names
	var autoGroup map[string]interface{}
	for _, g := range groups {
		if g["name"] == "Auto" {
			autoGroup = g
		}
	}
	if autoGroup == nil {
		t.Fatal("Auto proxy group not found")
	}
	autoPx, _ := autoGroup["proxies"].([]string)
	if len(autoPx) != 2 {
		t.Errorf("expected 2 proxies in Auto group, got %d", len(autoPx))
	}
}

func TestGenerate_MarshalYAML(t *testing.T) {
	cfg := defaultTestCfg()
	result, err := config.Generate(cfg, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	data, err := config.MarshalYAML(result)
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}
	if len(data) < 100 {
		t.Errorf("marshaled YAML seems too short: %d bytes", len(data))
	}
}

func TestGenerate_NoBypassChina(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.Network.BypassChina = false

	result, err := config.Generate(cfg, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	rules, _ := result["rules"].([]string)
	for _, r := range rules {
		if r == "GEOSITE,cn,DIRECT" {
			t.Error("GEOSITE,cn,DIRECT should NOT be present when bypass_china=false")
		}
	}
}

func TestGenerate_LogLevelWarnAlias(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.Log.Level = "warn"

	result, err := config.Generate(cfg, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if result["log-level"] != "warning" {
		t.Fatalf("expected log-level=warning, got %v", result["log-level"])
	}
}

func TestGenerate_LogLevelFallback(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.Log.Level = "nope"

	result, err := config.Generate(cfg, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if result["log-level"] != "info" {
		t.Fatalf("expected log-level=info, got %v", result["log-level"])
	}
}

func TestGenerateFromBase_PatchesIPCIDRNoResolve(t *testing.T) {
	// A minimal subscription YAML with an ipcidr rule-provider ("lancidr") referenced
	// by a RULE-SET rule without no-resolve.  GenerateFromBase must add no-resolve.
	rawYAML := []byte(`
proxies: []
proxy-groups:
  - name: Proxy
    type: select
    proxies: [DIRECT]
rule-providers:
  lancidr:
    type: http
    behavior: ipcidr
    url: "https://example.com/lancidr.txt"
    path: ./lancidr.yaml
    interval: 86400
  gfw:
    type: http
    behavior: domain
    url: "https://example.com/gfw.txt"
    path: ./gfw.yaml
    interval: 86400
rules:
  - RULE-SET,gfw,Proxy
  - RULE-SET,lancidr,DIRECT
  - MATCH,DIRECT
`)

	cfg := config.Default()
	result, err := config.GenerateFromBase(cfg, rawYAML, nil)
	if err != nil {
		t.Fatalf("GenerateFromBase: %v", err)
	}

	rules, _ := result["rules"].([]interface{})
	foundLancidrNoResolve := false
	gfwHasNoResolve := false
	for _, r := range rules {
		rStr, _ := r.(string)
		if rStr == "RULE-SET,lancidr,DIRECT,no-resolve" {
			foundLancidrNoResolve = true
		}
		if rStr == "RULE-SET,gfw,Proxy,no-resolve" {
			gfwHasNoResolve = true
		}
	}
	if !foundLancidrNoResolve {
		t.Error("expected RULE-SET,lancidr,DIRECT,no-resolve — ipcidr provider must have no-resolve")
	}
	if gfwHasNoResolve {
		t.Error("domain-type provider rule should NOT have no-resolve appended")
	}
}
