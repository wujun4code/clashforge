package integration_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/subscription"
	"gopkg.in/yaml.v3"
)

// Synthetic v11-shaped fixture, NOT a copy of private subscription bytes.
// The reserved hostname and password below cannot connect to a real service.
const singleNodeV11Fixture = `
proxies:
  - {name: Tokyo-AnyTLS, type: anytls, server: fixture.example.invalid, port: 443, password: synthetic-only, udp: true}
proxy-groups:
  - {name: 🚀 节点选择, type: select, proxies: [Tokyo-AnyTLS, ♻️ 自动选择, DIRECT]}
  - {name: ♻️ 自动选择, type: url-test, url: https://example.invalid/generate_204, interval: 300, proxies: [Tokyo-AnyTLS]}
  - {name: 🎯 全球直连, type: select, proxies: [DIRECT, 🚀 节点选择]}
  - {name: 🐟 漏网之鱼, type: select, proxies: [🚀 节点选择, 🎯 全球直连]}
rules:
  - DOMAIN,example.invalid,🚀 节点选择
  - MATCH,🐟 漏网之鱼
`

func TestSingleNodeV11_ParseGenerateDeviceGroups(t *testing.T) {
	// Generation diagnostics can contain private policy/server data. Suppress
	// production logs for this test, including the opt-in real-input invocation.
	previousLogger := log.Logger
	log.Logger = zerolog.Nop()
	t.Cleanup(func() { log.Logger = previousLogger })
	data := []byte(singleNodeV11Fixture)
	// Explicit local validation of the real downloaded input is opt-in. If a
	// path is supplied but cannot be read, fail (never silently skip/fallback).
	if path := os.Getenv("CLASHFORGE_E2E_SUBSCRIPTION_FILE"); path != "" {
		var err error
		data, err = os.ReadFile(path)
		if err != nil {
			t.Fatalf("read supplied subscription failed (%T)", err)
		}
	}
	nodes, err := subscription.Parse(data)
	if err != nil {
		t.Fatalf("subscription parse failed (%T)", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("v11 fixture must have exactly one node, got %d", len(nodes))
	}
	node := nodes[0]
	if node.Name != "Tokyo-AnyTLS" || node.Type != "anytls" || node.Extra["udp"] != true {
		t.Fatal("v11 node name/type/UDP contract not preserved")
	}
	if node.Server == "" || node.Port <= 0 || node.Extra["password"] == "" {
		t.Fatal("required AnyTLS transport fields missing")
	}
	generated, err := config.GenerateFromBase(config.Default(), data, nodes)
	if err != nil {
		t.Fatalf("generation failed (%T)", err)
	}
	encoded, err := config.MarshalYAML(generated)
	if err != nil {
		t.Fatalf("marshal failed (%T)", err)
	}
	var normalized map[string]interface{}
	if err := yaml.Unmarshal(encoded, &normalized); err != nil {
		t.Fatalf("generated YAML invalid (%T)", err)
	}
	// Compare transport privately, without formatting credential-bearing nodes.
	reparsed, err := subscription.Parse(encoded)
	if err != nil || len(reparsed) != 1 {
		t.Fatal("generated single node cannot be reparsed")
	}
	if reparsed[0].Server != node.Server || reparsed[0].Port != node.Port || reparsed[0].Extra["password"] != node.Extra["password"] || reparsed[0].Extra["udp"] != true {
		t.Fatal("AnyTLS transport changed during generation")
	}
	wantGroups := []string{"🚀 节点选择", "♻️ 自动选择", "🎯 全球直连", "🐟 漏网之鱼"}
	groups, ok := normalized["proxy-groups"].([]interface{})
	if !ok {
		t.Fatal("generated proxy-groups missing")
	}
	found := make(map[string]map[string]interface{})
	for _, raw := range groups {
		group, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatal("invalid group structure")
		}
		name, _ := group["name"].(string)
		found[name] = group
	}
	for _, name := range wantGroups {
		if found[name] == nil {
			t.Fatalf("v11 group not preserved: %s", name)
		}
	}
	for _, name := range wantGroups[:2] {
		members, _ := found[name]["proxies"].([]interface{})
		present := false
		for _, member := range members {
			if member == "Tokyo-AnyTLS" {
				present = true
			}
		}
		if !present {
			t.Fatalf("real node unavailable in %s", name)
		}
	}
	// Persist, reload and expand a source-bound per-device override using only
	// RFC5737 synthetic device identity. No real device data enters fixtures.
	deviceGroups := []config.DeviceGroup{{
		ID: "synthetic-device", Name: "Synthetic device",
		Devices:   []config.Device{{IP: "192.0.2.10", Prefix: 32}},
		Overrides: []config.ProxyGroupOverride{{OriginalGroup: "🚀 节点选择", Proxies: []string{"Tokyo-AnyTLS"}}},
	}}
	path := filepath.Join(t.TempDir(), "device-groups.json")
	if err := config.SaveDeviceGroupsForSource(path, deviceGroups, "subscription:synthetic-v11"); err != nil {
		t.Fatalf("device save failed (%T)", err)
	}
	loaded, err := config.LoadDeviceGroupsForSource(path, "subscription:synthetic-v11")
	if err != nil || !reflect.DeepEqual(loaded, deviceGroups) {
		t.Fatal("source-scoped device group did not round trip")
	}
	applied, specs := config.ApplyPerDeviceSubRulesWithProviders(normalized, loaded)
	if len(specs) != 1 || !reflect.DeepEqual(specs[0].Payload, []string{"192.0.2.10/32"}) {
		t.Fatal("device source matcher missing")
	}
	shadowFound := false
	for _, raw := range applied["proxy-groups"].([]interface{}) {
		group := raw.(map[string]interface{})
		if group["name"] == "Synthetic device - 🚀 节点选择" {
			shadowFound = reflect.DeepEqual(group["proxies"], []interface{}{"Tokyo-AnyTLS"})
		}
	}
	if !shadowFound {
		t.Fatal("device shadow must select the only real node, not DIRECT")
	}
	bound := false
	for _, raw := range applied["rules"].([]interface{}) {
		rule, _ := raw.(string)
		if strings.Contains(rule, "cf-device-group-synthetic-device") && strings.HasSuffix(rule, ",Synthetic device - 🚀 节点选择") {
			bound = true
		}
	}
	if !bound {
		t.Fatal("device rule not bound to source and expected node group")
	}
	t.Log("validated 1 anytls node, UDP, 4 subscription groups and source-bound device routing")
}
