package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/subscription"
)

// withGeosite creates a temporary geosite.dat placeholder and sets
// cfg.Core.GeositePath to it. It returns a cleanup function.
func withGeosite(t *testing.T, cfg *config.MetaclashConfig) func() {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "geosite-*.dat")
	if err != nil {
		t.Fatalf("create temp geosite: %v", err)
	}
	_ = f.Close()
	cfg.Core.GeositePath = f.Name()
	return func() { os.Remove(f.Name()) }
}

// withoutGeosite sets cfg.Core.GeositePath to a path that does not exist.
func withoutGeosite(cfg *config.MetaclashConfig) {
	cfg.Core.GeositePath = filepath.Join(os.TempDir(), "nonexistent-geosite-XXXXX.dat")
}

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

// ── DNS strategy tests ────────────────────────────────────────────────────────

func dnsSection(t *testing.T, cfg *config.MetaclashConfig) map[string]interface{} {
	t.Helper()
	result, err := config.Generate(cfg, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	dns, ok := result["dns"].(map[string]interface{})
	if !ok {
		t.Fatal("dns section missing or wrong type")
	}
	return dns
}

func nameserverPolicy(dns map[string]interface{}) map[string]interface{} {
	p, _ := dns["nameserver-policy"].(map[string]interface{})
	return p
}

func TestDNSStrategy_Legacy_NoPolicy(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.DNS.Strategy = config.DNSStrategyLegacy
	withoutGeosite(cfg)

	dns := dnsSection(t, cfg)
	if _, ok := dns["nameserver-policy"]; ok {
		t.Error("legacy strategy must not produce nameserver-policy")
	}
}

func TestDNSStrategy_EmptyString_TreatedAsLegacy(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.DNS.Strategy = "" // blank → legacy behaviour
	withoutGeosite(cfg)

	dns := dnsSection(t, cfg)
	if _, ok := dns["nameserver-policy"]; ok {
		t.Error("empty strategy must not produce nameserver-policy")
	}
}

func TestDNSStrategy_Split_WithGeosite(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.DNS.Strategy = config.DNSStrategysplit
	cleanup := withGeosite(t, cfg)
	defer cleanup()

	dns := dnsSection(t, cfg)

	policy := nameserverPolicy(dns)
	if policy == nil {
		t.Fatal("split strategy with geosite must produce nameserver-policy")
	}
	if _, ok := policy["geosite:cn"]; !ok {
		t.Error("nameserver-policy missing geosite:cn")
	}
	if _, ok := policy["geosite:geolocation-!cn"]; !ok {
		t.Error("nameserver-policy missing geosite:geolocation-!cn")
	}

	// split must NOT replace nameserver with CN DoH
	ns, _ := dns["nameserver"].([]string)
	for _, s := range ns {
		if strings.HasPrefix(s, "https://dns.alidns") || strings.HasPrefix(s, "https://doh.pub") {
			t.Errorf("split strategy must keep ISP nameserver, got CN DoH: %s", s)
		}
	}
}

func TestDNSStrategy_Split_WithoutGeosite_Degrades(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.DNS.Strategy = config.DNSStrategysplit
	withoutGeosite(cfg)

	dns := dnsSection(t, cfg)
	if _, ok := dns["nameserver-policy"]; ok {
		t.Error("split without geosite.dat must degrade to legacy (no nameserver-policy)")
	}
}

func TestDNSStrategy_Privacy_WithGeosite(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.DNS.Strategy = config.DNSStrategyPrivacy
	cleanup := withGeosite(t, cfg)
	defer cleanup()

	dns := dnsSection(t, cfg)

	policy := nameserverPolicy(dns)
	if policy == nil {
		t.Fatal("privacy strategy with geosite must produce nameserver-policy")
	}

	// CN side must use DoH, not plain IPs
	cnDNS, _ := policy["geosite:cn"].([]interface{})
	for _, v := range cnDNS {
		s, _ := v.(string)
		if s != "" && !strings.HasPrefix(s, "https://") {
			t.Errorf("privacy CN DNS must be DoH, got: %s", s)
		}
	}

	// nameserver must be replaced with CN DoH
	ns, _ := dns["nameserver"].([]string)
	if len(ns) == 0 {
		t.Fatal("nameserver must not be empty after privacy strategy")
	}
	for _, s := range ns {
		if !strings.HasPrefix(s, "https://") {
			t.Errorf("privacy strategy must replace nameserver with DoH, got: %s", s)
		}
	}
}

func TestDNSStrategy_Privacy_WithoutGeosite_Degrades(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.DNS.Strategy = config.DNSStrategyPrivacy
	withoutGeosite(cfg)

	dns := dnsSection(t, cfg)
	if _, ok := dns["nameserver-policy"]; ok {
		t.Error("privacy without geosite.dat must degrade to legacy (no nameserver-policy)")
	}
	// nameserver must remain ISP-based (not replaced with CN DoH)
	ns, _ := dns["nameserver"].([]string)
	for _, s := range ns {
		if strings.HasPrefix(s, "https://dns.alidns") || strings.HasPrefix(s, "https://doh.pub") {
			t.Errorf("degrade path must not replace nameserver with CN DoH, got: %s", s)
		}
	}
}

func TestDNSStrategy_Split_FallbackFilter_ExtraIPCIDR(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.DNS.Strategy = config.DNSStrategysplit
	cleanup := withGeosite(t, cfg)
	defer cleanup()

	dns := dnsSection(t, cfg)
	ff, ok := dns["fallback-filter"].(map[string]interface{})
	if !ok {
		t.Fatal("fallback-filter missing")
	}
	cidrs, _ := ff["ipcidr"].([]string)
	found240, found0 := false, false
	for _, c := range cidrs {
		if c == "240.0.0.0/4" {
			found240 = true
		}
		if c == "0.0.0.0/8" {
			found0 = true
		}
	}
	if !found240 {
		t.Error("fallback-filter ipcidr must include 240.0.0.0/4")
	}
	if !found0 {
		t.Error("fallback-filter ipcidr must include 0.0.0.0/8")
	}
}

func TestDNSStrategy_DefaultIsSplit(t *testing.T) {
	cfg := config.Default()
	if cfg.DNS.Strategy != config.DNSStrategysplit {
		t.Errorf("Default() strategy must be %q, got %q", config.DNSStrategysplit, cfg.DNS.Strategy)
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

func TestGenerate_GeodataModeFollowsConfig(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.Core.GeoDataMode = false
	result, err := config.Generate(cfg, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result["geodata-mode"] != false {
		t.Errorf("expected geodata-mode=false, got %v", result["geodata-mode"])
	}

	cfg.Core.GeoDataMode = true
	result, err = config.Generate(cfg, nil)
	if err != nil {
		t.Fatalf("Generate (true): %v", err)
	}
	if result["geodata-mode"] != true {
		t.Errorf("expected geodata-mode=true, got %v", result["geodata-mode"])
	}
}

func TestGenerate_GeodataPathPointsToDataDir(t *testing.T) {
	cfg := defaultTestCfg()
	result, err := config.Generate(cfg, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if _, hasGeox := result["geox-url"]; hasGeox {
		t.Error("geox-url must not be set (local paths are not valid URLs; use geodata-path instead)")
	}

	pathRaw, ok := result["geodata-path"]
	if !ok {
		t.Fatal("geodata-path key missing from generated config")
	}
	gotPath, ok := pathRaw.(string)
	if !ok {
		t.Fatalf("geodata-path is not a string, got %T", pathRaw)
	}

	dataDir := strings.ReplaceAll(cfg.Core.DataDir, `\`, "/")
	if gotPath != dataDir {
		t.Errorf("geodata-path %q does not equal DataDir %q", gotPath, dataDir)
	}
}

func TestGenerateFromBase_GeodataModeFollowsConfig(t *testing.T) {
	rawYAML := []byte(`
proxies: []
proxy-groups:
  - name: Proxy
    type: select
    proxies: [DIRECT]
rules:
  - MATCH,Proxy
`)
	cfg := config.Default()
	cfg.Core.GeoDataMode = false
	result, err := config.GenerateFromBase(cfg, rawYAML, nil)
	if err != nil {
		t.Fatalf("GenerateFromBase: %v", err)
	}

	if result["geodata-mode"] != false {
		t.Errorf("expected geodata-mode=false in GenerateFromBase output, got %v", result["geodata-mode"])
	}

	if _, hasGeox := result["geox-url"]; hasGeox {
		t.Error("geox-url must not be set in GenerateFromBase output (use geodata-path instead)")
	}

	pathRaw, ok := result["geodata-path"]
	if !ok {
		t.Fatal("geodata-path key missing from GenerateFromBase output")
	}
	if _, ok := pathRaw.(string); !ok {
		t.Fatalf("geodata-path is not a string, got %T", pathRaw)
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

// ── TUN mode tests ────────────────────────────────────────────────────────────

func TestGenerate_TUNMode_EmitsTUNBlock(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.Network.Mode = "tun"
	cfg.Network.TUN = config.TUNConfig{
		Stack:               "mixed",
		DNSHijack:           []string{"any:53"},
		AutoRoute:           true,
		AutoDetectInterface: true,
	}

	result, err := config.Generate(cfg, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	tunRaw, ok := result["tun"]
	if !ok {
		t.Fatal("expected 'tun' key in generated config when mode=tun")
	}
	tun, ok := tunRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("expected tun to be map[string]interface{}, got %T", tunRaw)
	}
	if tun["enable"] != true {
		t.Errorf("tun.enable: expected true, got %v", tun["enable"])
	}
	if tun["stack"] != "mixed" {
		t.Errorf("tun.stack: expected mixed, got %v", tun["stack"])
	}
	if tun["auto-route"] != true {
		t.Errorf("tun.auto-route: expected true, got %v", tun["auto-route"])
	}
	if tun["auto-detect-interface"] != true {
		t.Errorf("tun.auto-detect-interface: expected true, got %v", tun["auto-detect-interface"])
	}
	hijack, _ := tun["dns-hijack"].([]string)
	if len(hijack) == 0 || hijack[0] != "any:53" {
		t.Errorf("tun.dns-hijack: expected [any:53], got %v", tun["dns-hijack"])
	}
}

func TestGenerate_NonTUNMode_NoTUNBlock(t *testing.T) {
	for _, mode := range []string{"tproxy", "redir", "none"} {
		t.Run("mode="+mode, func(t *testing.T) {
			cfg := defaultTestCfg()
			cfg.Network.Mode = mode
			result, err := config.Generate(cfg, nil)
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			if _, ok := result["tun"]; ok {
				t.Errorf("mode=%s: unexpected 'tun' key in generated config", mode)
			}
		})
	}
}

func TestApplyManagedRuntimeSettings_TUNMode_InjectsTUN(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.Network.Mode = "tun"
	cfg.Network.TUN = config.TUNConfig{
		Stack:               "gvisor",
		DNSHijack:           []string{"any:53"},
		AutoRoute:           true,
		AutoDetectInterface: true,
	}

	merged := map[string]interface{}{"mode": "rule"}
	result := config.ApplyManagedRuntimeSettings(cfg, merged)

	tunRaw, ok := result["tun"]
	if !ok {
		t.Fatal("ApplyManagedRuntimeSettings: expected 'tun' key when mode=tun")
	}
	tun, ok := tunRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("expected tun map, got %T", tunRaw)
	}
	if tun["stack"] != "gvisor" {
		t.Errorf("expected stack=gvisor, got %v", tun["stack"])
	}
}

func TestApplyManagedRuntimeSettings_NonTUN_StripsTUN(t *testing.T) {
	// Even if overrides injected a tun block, non-TUN modes must strip it.
	cfg := defaultTestCfg()
	cfg.Network.Mode = "tproxy"

	merged := map[string]interface{}{
		"mode": "rule",
		"tun":  map[string]interface{}{"enable": true, "stack": "mixed"},
	}
	result := config.ApplyManagedRuntimeSettings(cfg, merged)
	if _, ok := result["tun"]; ok {
		t.Error("ApplyManagedRuntimeSettings: 'tun' key must be removed for non-TUN mode")
	}
}

func TestApplyManagedRuntimeSettings_DNSEnabled_ReplacesImportedDNS(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.DNS.Enable = true
	cfg.DNS.Mode = "fake-ip"
	cfg.DNS.Nameservers = []string{"223.5.5.5"}
	cfg.DNS.Fallback = []string{"tls://1.1.1.1"}
	withoutGeosite(cfg)

	merged := map[string]interface{}{
		"dns": map[string]interface{}{
			"enable":             false,
			"listen":             "127.0.0.1:53",
			"enhanced-mode":      "redir-host",
			"nameserver":         []interface{}{"1.2.3.4"},
			"fallback":           []interface{}{"tls://9.9.9.9"},
			"default-nameserver": []interface{}{"8.8.8.8"},
		},
	}

	result := config.ApplyManagedRuntimeSettings(cfg, merged)
	dns, ok := result["dns"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected managed dns map, got %T", result["dns"])
	}
	if dns["enable"] != true {
		t.Errorf("expected dns.enable=true, got %v", dns["enable"])
	}
	if dns["listen"] != "0.0.0.0:17874" {
		t.Errorf("expected managed DNS listen port, got %v", dns["listen"])
	}
	if dns["enhanced-mode"] != "fake-ip" {
		t.Errorf("expected enhanced-mode=fake-ip from ClashForge mode, got %v", dns["enhanced-mode"])
	}
	if stringsValueContains(dns["nameserver"], "1.2.3.4") {
		t.Errorf("imported nameserver leaked into managed DNS: %v", dns["nameserver"])
	}
	if stringsValueContains(dns["fallback"], "tls://9.9.9.9") {
		t.Errorf("imported fallback leaked into managed DNS: %v", dns["fallback"])
	}
	if stringsValueContains(dns["default-nameserver"], "8.8.8.8") {
		t.Errorf("imported default-nameserver leaked into managed DNS: %v", dns["default-nameserver"])
	}
}

func TestApplyManagedRuntimeSettings_DNSDisabled_StripsImportedDNS(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.DNS.Enable = false

	result := config.ApplyManagedRuntimeSettings(cfg, map[string]interface{}{
		"dns": map[string]interface{}{"enable": true, "nameserver": []interface{}{"1.2.3.4"}},
	})
	if _, ok := result["dns"]; ok {
		t.Error("ApplyManagedRuntimeSettings: dns must be removed when ClashForge DNS is disabled")
	}
}

func TestApplyManagedRuntimeSettings_TUNMode_ForcesFakeIPDNS(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.Network.Mode = "tun"
	cfg.DNS.Enable = true
	cfg.DNS.Mode = "redir-host"
	withoutGeosite(cfg)

	result := config.ApplyManagedRuntimeSettings(cfg, map[string]interface{}{
		"dns": map[string]interface{}{"enhanced-mode": "redir-host"},
	})
	dns, ok := result["dns"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected managed dns map, got %T", result["dns"])
	}
	if dns["enhanced-mode"] != "fake-ip" {
		t.Errorf("TUN runtime DNS must force fake-ip, got %v", dns["enhanced-mode"])
	}
}

func stringsValueContains(v interface{}, want string) bool {
	switch xs := v.(type) {
	case []string:
		for _, s := range xs {
			if s == want {
				return true
			}
		}
	case []interface{}:
		for _, x := range xs {
			if s, ok := x.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}

func TestApplyManagedRuntimeSettings_TUNMode_DeviceField(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.Network.Mode = "tun"
	cfg.Network.TUN = config.TUNConfig{
		Stack:               "mixed",
		DNSHijack:           []string{"any:53"},
		AutoRoute:           true,
		AutoDetectInterface: true,
		Device:              "Meta",
	}

	result := config.ApplyManagedRuntimeSettings(cfg, map[string]interface{}{})
	tun, _ := result["tun"].(map[string]interface{})
	if tun == nil {
		t.Fatal("expected tun block")
	}
	if tun["device"] != "Meta" {
		t.Errorf("expected device=Meta, got %v", tun["device"])
	}
}

func TestGenerate_TUNMode_NoDeviceField_WhenEmpty(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.Network.Mode = "tun"
	cfg.Network.TUN = config.TUNConfig{
		Stack:               "mixed",
		DNSHijack:           []string{"any:53"},
		AutoRoute:           true,
		AutoDetectInterface: true,
		Device:              "", // empty → omit
	}

	result, err := config.Generate(cfg, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	tun, _ := result["tun"].(map[string]interface{})
	if tun == nil {
		t.Fatal("expected tun block")
	}
	if _, exists := tun["device"]; exists {
		t.Error("expected 'device' key to be omitted when Device is empty")
	}
}
