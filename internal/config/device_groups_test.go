package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
)

func TestApplyPerDeviceSubRules_GeneratesShadowGroupsAndANDRules(t *testing.T) {
	base := map[string]interface{}{
		"proxies": []interface{}{
			map[string]interface{}{"name": "node-sg"},
			map[string]interface{}{"name": "node-us"},
		},
		"proxy-groups": []interface{}{
			map[string]interface{}{
				"name":    "Proxy",
				"type":    "select",
				"proxies": []interface{}{"node-sg", "node-us"},
			},
			map[string]interface{}{
				"name":    "Final",
				"type":    "select",
				"proxies": []interface{}{"Proxy", "DIRECT"},
			},
		},
		"rules": []interface{}{
			"RULE-SET,reject,REJECT",
			"RULE-SET,google,Proxy",
			"GEOIP,CN,DIRECT,no-resolve",
			"MATCH,Final",
		},
	}

	deviceGroups := []config.DeviceGroup{
		{
			ID:    "iphone",
			Name:  "iPhone",
			Order: 1,
			Devices: []config.Device{
				{IP: "192.168.1.100", Prefix: 32},
			},
			Overrides: []config.ProxyGroupOverride{
				{OriginalGroup: "Proxy", Proxies: []string{"node-sg"}},
			},
		},
		{
			ID:    "windows",
			Name:  "Windows",
			Order: 2,
			Devices: []config.Device{
				{IP: "192.168.1.200", Prefix: 32},
			},
			Overrides: []config.ProxyGroupOverride{
				{OriginalGroup: "Proxy", Proxies: []string{"node-us"}},
			},
		},
	}

	result := config.ApplyPerDeviceSubRules(base, deviceGroups)

	groups, ok := result["proxy-groups"].([]interface{})
	if !ok {
		t.Fatalf("proxy-groups should be []interface{}, got %T", result["proxy-groups"])
	}
	hasIPhoneShadow := false
	hasWindowsShadow := false
	for _, g := range groups {
		gm, _ := g.(map[string]interface{})
		name, _ := gm["name"].(string)
		if name == "iPhone - Proxy" {
			hasIPhoneShadow = true
		}
		if name == "Windows - Proxy" {
			hasWindowsShadow = true
		}
	}
	if !hasIPhoneShadow || !hasWindowsShadow {
		t.Fatalf("expected both shadow groups, got iPhone=%v windows=%v", hasIPhoneShadow, hasWindowsShadow)
	}

	rules, ok := result["rules"].([]interface{})
	if !ok {
		t.Fatalf("rules should be []interface{}, got %T", result["rules"])
	}
	got := make([]string, 0, len(rules))
	for _, r := range rules {
		rs, _ := r.(string)
		got = append(got, rs)
	}

	if len(got) != 6 {
		t.Fatalf("expected 6 rules, got %d: %#v", len(got), got)
	}
	if got[0] != "RULE-SET,reject,REJECT" {
		t.Fatalf("expected first rule to remain global reject, got %q", got[0])
	}
	if got[1] != "AND,((RULE-SET,cf-device-group-iphone,src,no-resolve),(RULE-SET,google)),iPhone - Proxy" {
		t.Fatalf("unexpected first AND rule: %q", got[1])
	}
	if got[2] != "AND,((RULE-SET,cf-device-group-windows,src,no-resolve),(RULE-SET,google)),Windows - Proxy" {
		t.Fatalf("unexpected second AND rule: %q", got[2])
	}
	if got[3] != "RULE-SET,google,Proxy" || got[4] != "GEOIP,CN,DIRECT,no-resolve" || got[5] != "MATCH,Final" {
		t.Fatalf("unexpected fallback rules: %#v", got[3:])
	}

	providers, ok := result["rule-providers"].(map[string]interface{})
	if !ok {
		t.Fatalf("rule-providers should be map[string]interface{}, got %T", result["rule-providers"])
	}
	if _, ok := providers["cf-device-group-iphone"]; !ok {
		t.Fatalf("missing managed provider cf-device-group-iphone: %#v", providers)
	}
	if _, ok := providers["cf-device-group-windows"]; !ok {
		t.Fatalf("missing managed provider cf-device-group-windows: %#v", providers)
	}
}

func TestApplyPerDeviceSubRules_IgnoresInvalidOverrides(t *testing.T) {
	base := map[string]interface{}{
		"proxies": []interface{}{
			map[string]interface{}{"name": "node-sg"},
		},
		"proxy-groups": []interface{}{
			map[string]interface{}{
				"name":    "Proxy",
				"type":    "select",
				"proxies": []interface{}{"node-sg"},
			},
		},
		"rules": []interface{}{
			"RULE-SET,google,Proxy",
			"MATCH,Proxy",
		},
	}

	deviceGroups := []config.DeviceGroup{
		{
			Name: "Broken",
			Devices: []config.Device{
				{IP: "192.168.1.10"},
			},
			Overrides: []config.ProxyGroupOverride{
				{OriginalGroup: "NotFound", Proxies: []string{"node-sg"}},
				{OriginalGroup: "Proxy", Proxies: []string{"not-exist"}},
			},
		},
	}

	result := config.ApplyPerDeviceSubRules(base, deviceGroups)
	rules, _ := result["rules"].([]interface{})
	if len(rules) != 2 {
		t.Fatalf("rules should stay unchanged, got %d", len(rules))
	}
	groups, _ := result["proxy-groups"].([]interface{})
	if len(groups) != 1 {
		t.Fatalf("proxy-groups should stay unchanged, got %d", len(groups))
	}
}

func TestApplyPerDeviceSubRules_ExpandsMatchAsPerDeviceFallback(t *testing.T) {
	base := map[string]interface{}{
		"proxies": []interface{}{
			map[string]interface{}{"name": "node-sg"},
			map[string]interface{}{"name": "node-us"},
		},
		"proxy-groups": []interface{}{
			map[string]interface{}{
				"name":    "Proxy",
				"type":    "select",
				"proxies": []interface{}{"node-sg", "node-us"},
			},
		},
		"rules": []interface{}{
			"MATCH,Proxy",
		},
	}

	deviceGroups := []config.DeviceGroup{
		{
			Name: "iPhone",
			Devices: []config.Device{
				{IP: "192.168.1.100", Prefix: 32},
			},
			Overrides: []config.ProxyGroupOverride{
				{OriginalGroup: "Proxy", Proxies: []string{"node-sg"}},
			},
		},
		{
			Name: "Windows",
			Devices: []config.Device{
				{IP: "192.168.1.200", Prefix: 32},
			},
			Overrides: []config.ProxyGroupOverride{
				{OriginalGroup: "Proxy", Proxies: []string{"node-us"}},
			},
		},
	}

	result := config.ApplyPerDeviceSubRules(base, deviceGroups)
	rules, ok := result["rules"].([]interface{})
	if !ok {
		t.Fatalf("rules should be []interface{}, got %T", result["rules"])
	}
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d: %#v", len(rules), rules)
	}

	got := make([]string, 0, len(rules))
	for _, r := range rules {
		rs, _ := r.(string)
		got = append(got, rs)
	}

	if got[0] != "RULE-SET,cf-device-group-iphone,iPhone - Proxy,src,no-resolve" {
		t.Fatalf("unexpected first per-device MATCH fallback: %q", got[0])
	}
	if got[1] != "RULE-SET,cf-device-group-windows,Windows - Proxy,src,no-resolve" {
		t.Fatalf("unexpected second per-device MATCH fallback: %q", got[1])
	}
	if got[2] != "MATCH,Proxy" {
		t.Fatalf("MATCH should stay as final global fallback, got %q", got[2])
	}
}

func TestApplyPerDeviceSubRules_InsertsAndRulesBeforeTheirSourceRules(t *testing.T) {
	base := map[string]interface{}{
		"proxies": []interface{}{
			map[string]interface{}{"name": "node-sg"},
			map[string]interface{}{"name": "node-us"},
		},
		"proxy-groups": []interface{}{
			map[string]interface{}{
				"name":    "🚀 节点选择",
				"type":    "select",
				"proxies": []interface{}{"node-sg", "node-us"},
			},
			map[string]interface{}{
				"name":    "🎯 全球直连",
				"type":    "select",
				"proxies": []interface{}{"DIRECT"},
			},
		},
		"rules": []interface{}{
			"RULE-SET,private,🎯 全球直连",
			"RULE-SET,google,🚀 节点选择",
			"RULE-SET,cncidr,🎯 全球直连,no-resolve",
			"MATCH,🚀 节点选择",
		},
	}

	deviceGroups := []config.DeviceGroup{
		{
			Name: "WindowsPC",
			Devices: []config.Device{
				{IP: "192.168.20.231", Prefix: 32},
			},
			Overrides: []config.ProxyGroupOverride{
				{OriginalGroup: "🚀 节点选择", Proxies: []string{"node-us"}},
			},
		},
	}

	result := config.ApplyPerDeviceSubRules(base, deviceGroups)
	rules, ok := result["rules"].([]interface{})
	if !ok {
		t.Fatalf("rules should be []interface{}, got %T", result["rules"])
	}

	got := make([]string, 0, len(rules))
	for _, r := range rules {
		rs, _ := r.(string)
		got = append(got, rs)
	}

	// Direct rule remains at the top and is not pushed behind expanded AND rules.
	if got[0] != "RULE-SET,private,🎯 全球直连" {
		t.Fatalf("expected direct rule to remain first, got %q", got[0])
	}
	if got[1] != "AND,((RULE-SET,cf-device-group-windowspc,src,no-resolve),(RULE-SET,google)),WindowsPC - 🚀 节点选择" {
		t.Fatalf("unexpected AND insertion around proxy rule: %#v", got)
	}
	if got[2] != "RULE-SET,google,🚀 节点选择" {
		t.Fatalf("expected source proxy rule right after its AND expansion, got %q", got[2])
	}
	if got[3] != "RULE-SET,cncidr,🎯 全球直连,no-resolve" {
		t.Fatalf("expected later direct rule unchanged, got %q", got[3])
	}
	if got[4] != "RULE-SET,cf-device-group-windowspc,WindowsPC - 🚀 节点选择,src,no-resolve" {
		t.Fatalf("expected per-device MATCH fallback before MATCH, got %q", got[4])
	}
	if got[5] != "MATCH,🚀 节点选择" {
		t.Fatalf("expected global MATCH retained at end, got %q", got[5])
	}
}

func TestApplyPerDeviceSubRules_AggregatesDevicesByGroupMatcher(t *testing.T) {
	base := map[string]interface{}{
		"proxies": []interface{}{
			map[string]interface{}{"name": "node-sg"},
			map[string]interface{}{"name": "node-us"},
		},
		"proxy-groups": []interface{}{
			map[string]interface{}{
				"name":    "🚀 节点选择",
				"type":    "select",
				"proxies": []interface{}{"node-sg", "node-us"},
			},
		},
		"rules": []interface{}{
			"RULE-SET,google,🚀 节点选择",
			"MATCH,🚀 节点选择",
		},
	}

	deviceGroups := []config.DeviceGroup{
		{
			Name: "WindowsPC",
			Devices: []config.Device{
				{IP: "192.168.20.231", Prefix: 32},
				{IP: "192.168.20.232", Prefix: 32},
			},
			Overrides: []config.ProxyGroupOverride{
				{OriginalGroup: "🚀 节点选择", Proxies: []string{"node-us"}},
			},
		},
	}

	result := config.ApplyPerDeviceSubRules(base, deviceGroups)
	rules, ok := result["rules"].([]interface{})
	if !ok {
		t.Fatalf("rules should be []interface{}, got %T", result["rules"])
	}

	got := make([]string, 0, len(rules))
	for _, r := range rules {
		rs, _ := r.(string)
		got = append(got, rs)
	}

	if len(got) != 4 {
		t.Fatalf("expected 4 rules with grouped matcher, got %d: %#v", len(got), got)
	}
	if got[0] != "AND,((RULE-SET,cf-device-group-windowspc,src,no-resolve),(RULE-SET,google)),WindowsPC - 🚀 节点选择" {
		t.Fatalf("unexpected grouped AND rule: %q", got[0])
	}
	if got[1] != "RULE-SET,google,🚀 节点选择" {
		t.Fatalf("expected original source rule to remain, got %q", got[1])
	}
	if got[2] != "RULE-SET,cf-device-group-windowspc,WindowsPC - 🚀 节点选择,src,no-resolve" {
		t.Fatalf("unexpected grouped MATCH fallback: %q", got[2])
	}
	if got[3] != "MATCH,🚀 节点选择" {
		t.Fatalf("expected global MATCH to remain last, got %q", got[3])
	}
}

func TestApplyPerDeviceSubRulesWithProviders_ReturnsDeviceProviderSpecs(t *testing.T) {
	base := map[string]interface{}{
		"proxies": []interface{}{
			map[string]interface{}{"name": "node-us"},
		},
		"proxy-groups": []interface{}{
			map[string]interface{}{
				"name":    "Proxy",
				"type":    "select",
				"proxies": []interface{}{"node-us"},
			},
		},
		"rules": []interface{}{
			"RULE-SET,google,Proxy",
		},
	}

	deviceGroups := []config.DeviceGroup{
		{
			ID:   "work-laptops",
			Name: "Work Laptops",
			Devices: []config.Device{
				{IP: "192.168.1.100", Prefix: 32},
				{IP: "192.168.1.101", Prefix: 32},
			},
			Overrides: []config.ProxyGroupOverride{
				{OriginalGroup: "Proxy", Proxies: []string{"node-us"}},
			},
		},
	}

	_, specs := config.ApplyPerDeviceSubRulesWithProviders(base, deviceGroups)
	if len(specs) != 1 {
		t.Fatalf("expected 1 provider spec, got %d: %#v", len(specs), specs)
	}
	if specs[0].Name != "cf-device-group-work-laptops" {
		t.Fatalf("unexpected provider name: %q", specs[0].Name)
	}
	if specs[0].FileName != "cf-device-group-work-laptops.yaml" {
		t.Fatalf("unexpected provider filename: %q", specs[0].FileName)
	}
	if len(specs[0].Payload) != 2 || specs[0].Payload[0] != "192.168.1.100/32" || specs[0].Payload[1] != "192.168.1.101/32" {
		t.Fatalf("unexpected provider payload: %#v", specs[0].Payload)
	}
}

func TestSyncDeviceRuleProviderFiles_RewritesManagedFilesOnly(t *testing.T) {
	runtimeDir := t.TempDir()
	ruleDir := filepath.Join(runtimeDir, "rule_provider")
	if err := os.MkdirAll(ruleDir, 0o755); err != nil {
		t.Fatalf("mkdir rule_provider: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ruleDir, "cf-device-group-old.yaml"), []byte("payload:\n  - 10.0.0.1/32\n"), 0o644); err != nil {
		t.Fatalf("seed old managed file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ruleDir, "upstream-provider.yaml"), []byte("payload:\n  - DOMAIN,example.com\n"), 0o644); err != nil {
		t.Fatalf("seed upstream file: %v", err)
	}

	specs := []config.DeviceRuleProviderSpec{
		{
			Name:     "cf-device-group-work-laptops",
			FileName: "cf-device-group-work-laptops.yaml",
			Payload:  []string{"192.168.1.100/32", "192.168.1.101/32"},
		},
	}
	if err := config.SyncDeviceRuleProviderFiles(runtimeDir, specs); err != nil {
		t.Fatalf("SyncDeviceRuleProviderFiles: %v", err)
	}

	if _, err := os.Stat(filepath.Join(ruleDir, "cf-device-group-old.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected stale managed file to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(ruleDir, "upstream-provider.yaml")); err != nil {
		t.Fatalf("expected upstream provider file to be kept: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(ruleDir, "cf-device-group-work-laptops.yaml"))
	if err != nil {
		t.Fatalf("read new managed provider: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "192.168.1.100/32") || !strings.Contains(text, "192.168.1.101/32") {
		t.Fatalf("unexpected managed provider content: %q", text)
	}
}

func TestDeviceGroupsSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "device-groups.json")

	input := []config.DeviceGroup{
		{
			ID:    "g2",
			Name:  "Windows",
			Order: 2,
			Devices: []config.Device{
				{IP: "192.168.1.200", Prefix: 32, Hostname: "work-pc"},
			},
			Overrides: []config.ProxyGroupOverride{
				{OriginalGroup: "Proxy", Proxies: []string{"node-us"}},
			},
		},
		{
			ID:    "g1",
			Name:  "iPhone",
			Order: 1,
			Devices: []config.Device{
				{IP: "192.168.1.100", Prefix: 32, Hostname: "iphone"},
			},
			Overrides: []config.ProxyGroupOverride{
				{OriginalGroup: "Proxy", Proxies: []string{"node-sg"}},
			},
		},
	}

	if err := config.SaveDeviceGroups(path, input); err != nil {
		t.Fatalf("SaveDeviceGroups: %v", err)
	}

	loaded, err := config.LoadDeviceGroups(path)
	if err != nil {
		t.Fatalf("LoadDeviceGroups: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(loaded))
	}
	if loaded[0].Name != "iPhone" || loaded[1].Name != "Windows" {
		t.Fatalf("groups should be ordered by Order, got %#v", loaded)
	}
}

func TestLoadDeviceGroups_LegacyArrayFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "device-groups.json")
	content := []byte(`[
  {
    "id": "g1",
    "name": "iPhone",
    "devices": [{"ip":"192.168.1.100","prefix":32}],
    "overrides": [{"original_group":"Proxy","proxies":["node-sg"]}],
    "order": 1
  }
]`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	loaded, err := config.LoadDeviceGroups(path)
	if err != nil {
		t.Fatalf("LoadDeviceGroups: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Name != "iPhone" {
		t.Fatalf("unexpected loaded groups: %#v", loaded)
	}
}
