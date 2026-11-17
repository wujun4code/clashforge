package config_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
)

// This is a synthetic subscription policy, not a vendor policy supplied by the
// generic generator. Keep the dedicated cloud group and device binding together.
const claudeSubscriptionFixture = `
proxies:
  - {name: node-tokyo, type: anytls, server: tokyo.example.invalid, port: 443, password: test-only}
  - {name: node-osaka, type: anytls, server: osaka.example.invalid, port: 443, password: test-only}
proxy-groups:
  - {name: Proxy, type: select, proxies: [node-tokyo, node-osaka]}
  - {name: Domestic, type: select, proxies: [DIRECT, Proxy]}
  - {name: Final, type: select, proxies: [Proxy, Domestic]}
rules:
  - RULE-SET,reject,REJECT
  - RULE-SET,private,Domestic
  - GEOSITE,CN,Domestic
  - GEOIP,CN,Domestic,no-resolve
  - MATCH,Final
`

const claudeOverridesFixture = `
proxy-groups:
  - {name: Claude, type: select, proxies: [node-tokyo, node-osaka]}
rules:
  - DOMAIN,api.anthropic.com,Claude
  - DOMAIN,anthropic.com,Claude
  - DOMAIN,claude.ai,Claude
  - DOMAIN,claude.com,Claude
  - DOMAIN,platform.claude.com,Claude
  - DOMAIN,sso.claude.com,Claude
  - DOMAIN,a-api.anthropic.com,Claude
  - DOMAIN,a-cdn.anthropic.com,Claude
  - DOMAIN,s-cdn.anthropic.com,Claude
  - DOMAIN,assets-proxy.anthropic.com,Claude
  - DOMAIN,a.claude.ai,Claude
  - DOMAIN,assets.claude.ai,Claude
  - DOMAIN,downloads.claude.ai,Claude
  - DOMAIN,mcp-proxy.anthropic.com,Claude
  - DOMAIN-SUFFIX,livepreview.claude.ai,Claude
  - DOMAIN-SUFFIX,livepreview.claude.app,Claude
  - DOMAIN-SUFFIX,claudeusercontent.com,Claude
  - DOMAIN-SUFFIX,claudemcpcontent.com,Claude
  - DOMAIN-SUFFIX,douyin.com,DIRECT
  - DOMAIN-SUFFIX,douyinvod.com,DIRECT
`

func claudeDeviceFixture() []config.DeviceGroup {
	return []config.DeviceGroup{{
		ID: "workstation", Name: "Workstation",
		Devices: []config.Device{{IP: "192.0.2.10", Prefix: 32}},
		Overrides: []config.ProxyGroupOverride{
			{OriginalGroup: "Proxy", Proxies: []string{"node-osaka"}},
			{OriginalGroup: "Final", Proxies: []string{"node-osaka"}},
		},
	}}
}

func generateClaudeFixture(t *testing.T, groups []config.DeviceGroup) (map[string]interface{}, []config.DeviceRuleProviderSpec) {
	t.Helper()
	base, err := config.GenerateFromBase(config.Default(), []byte(claudeSubscriptionFixture), nil)
	if err != nil {
		t.Fatal(err)
	}
	base, err = config.MergeWithOverrides(base, []byte(claudeOverridesFixture))
	if err != nil {
		t.Fatal(err)
	}
	return config.ApplyPerDeviceSubRulesWithProviders(base, groups)
}

func claudeFixtureRules(t *testing.T, base map[string]interface{}) []string {
	t.Helper()
	raw, ok := base["rules"].([]interface{})
	if !ok {
		t.Fatalf("unexpected rules type %T", base["rules"])
	}
	rules := make([]string, len(raw))
	for i, r := range raw {
		rules[i] = r.(string)
	}
	return rules
}

func TestClaudeCompatibilityRules_ExplicitDeviceBinding(t *testing.T) {
	groups := claudeDeviceFixture()
	// Configuration repair: the dedicated cloud group needs its own override;
	// overriding Proxy/Final alone cannot constrain a different policy group.
	groups[0].Overrides = append(groups[0].Overrides, config.ProxyGroupOverride{
		OriginalGroup: "Claude", Proxies: []string{"node-osaka"},
	})
	base, specs := generateClaudeFixture(t, groups)
	rules := claudeFixtureRules(t, base)
	wantFirst := "AND,((RULE-SET,cf-device-group-workstation,src,no-resolve),(DOMAIN,api.anthropic.com)),Workstation - Claude"
	if rules[0] != wantFirst {
		t.Fatalf("new cloud group must have an explicit device binding; first rule = %q, want %q", rules[0], wantFirst)
	}
	if len(specs) != 1 || !reflect.DeepEqual(specs[0].Payload, []string{"192.0.2.10/32"}) {
		t.Fatalf("unexpected device provider: %#v", specs)
	}
	var shadow map[string]interface{}
	for _, raw := range base["proxy-groups"].([]interface{}) {
		g := raw.(map[string]interface{})
		if g["name"] == "Workstation - Claude" {
			shadow = g
		}
	}
	if shadow == nil || shadow["type"] != "select" || !reflect.DeepEqual(shadow["proxies"], []interface{}{"node-osaka"}) {
		t.Fatalf("cloud shadow must keep the device's allowed node: %#v", shadow)
	}
	// Each precise cloud matcher remains immediately after its device-specific
	// version, ahead of reject and CN; an unbound device uses the global group.
	cloudRuleCount := strings.Count(claudeOverridesFixture, ",Claude\n")
	for i := 0; i < cloudRuleCount; i++ {
		if !strings.HasPrefix(rules[2*i], "AND,((RULE-SET,cf-device-group-workstation,src,no-resolve),(") ||
			!strings.HasSuffix(rules[2*i], ")),Workstation - Claude") || !strings.HasSuffix(rules[2*i+1], ",Claude") {
			t.Fatalf("cloud rule ordering broken at %d: %#v", i, rules[2*i:2*i+2])
		}
	}
	wantTail := []string{
		"DOMAIN-SUFFIX,douyin.com,DIRECT",
		"DOMAIN-SUFFIX,douyinvod.com,DIRECT",
		"RULE-SET,reject,REJECT",
		"RULE-SET,private,Domestic",
		"GEOSITE,CN,Domestic",
		"GEOIP,CN,Domestic,no-resolve",
		"RULE-SET,cf-device-group-workstation,Workstation - Final,src,no-resolve",
		"MATCH,Final",
	}
	if !reflect.DeepEqual(rules[2*cloudRuleCount:], wantTail) {
		t.Fatalf("domestic/LAN rules must precede device fallback: got %#v, want %#v", rules[2*cloudRuleCount:], wantTail)
	}
	for _, rule := range rules {
		if strings.Contains(rule, "PROCESS-") || strings.Contains(rule, "DOMAIN-KEYWORD") || strings.Contains(rule, "mcp.example.invalid") {
			t.Fatalf("must not infer processes or force arbitrary MCP endpoints: %q", rule)
		}
	}
}

func TestClaudeCompatibilityRules_NoImplicitGroupInheritance(t *testing.T) {
	groups := claudeDeviceFixture()
	base, _ := generateClaudeFixture(t, groups)
	for _, raw := range base["proxy-groups"].([]interface{}) {
		if raw.(map[string]interface{})["name"] == "Workstation - Claude" {
			t.Fatal("an unrelated group override must not silently change Claude policy")
		}
	}
	if got := claudeFixtureRules(t, base)[0]; got != "DOMAIN,api.anthropic.com,Claude" {
		t.Fatalf("unbound cloud policy should remain global, got %q", got)
	}
}

func TestClaudeCompatibilityRules_SourceScopedDeviceBinding(t *testing.T) {
	groups := claudeDeviceFixture()
	groups[0].Overrides = append(groups[0].Overrides, config.ProxyGroupOverride{
		OriginalGroup: "Claude", Proxies: []string{"node-osaka"},
	})
	path := filepath.Join(t.TempDir(), "device-groups.json")
	if err := config.SaveDeviceGroupsForSource(path, groups, "subscription:synthetic"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		source    string
		wantBound bool
	}{
		{"subscription:synthetic", true}, {"subscription:other", false},
	} {
		t.Run(tc.source, func(t *testing.T) {
			loaded, err := config.LoadDeviceGroupsForSource(path, tc.source)
			if err != nil {
				t.Fatal(err)
			}
			base, _ := generateClaudeFixture(t, loaded)
			bound := strings.HasSuffix(claudeFixtureRules(t, base)[0], ")),Workstation - Claude")
			if bound != tc.wantBound {
				t.Fatalf("source-specific cloud binding = %v, want %v", bound, tc.wantBound)
			}
		})
	}
}

func TestClaudeCompatibilityRules_DoesNotPromoteSubscriptionRules(t *testing.T) {
	// Device expansion must not reorder a subscription's security policy. The
	// operator, not generic device code, must put cloud exceptions before reject.
	base := map[string]interface{}{
		"proxies":      []interface{}{map[string]interface{}{"name": "node-osaka"}},
		"proxy-groups": []interface{}{map[string]interface{}{"name": "Claude", "type": "select", "proxies": []interface{}{"node-osaka"}}},
		"rules":        []interface{}{"RULE-SET,reject,REJECT", "GEOSITE,CN,DIRECT", "DOMAIN,api.anthropic.com,Claude", "MATCH,DIRECT"},
	}
	groups := claudeDeviceFixture()
	groups[0].Overrides = []config.ProxyGroupOverride{{OriginalGroup: "Claude", Proxies: []string{"node-osaka"}}}
	base = config.ApplyPerDeviceSubRules(base, groups)
	rules := claudeFixtureRules(t, base)
	if rules[0] != "RULE-SET,reject,REJECT" || rules[1] != "GEOSITE,CN,DIRECT" || !strings.HasSuffix(rules[2], ")),Workstation - Claude") {
		t.Fatalf("device expansion changed policy priority: %#v", rules)
	}
}
