package config_test

import (
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
)

func TestDeepMerge_ScalarOverride(t *testing.T) {
	dst := map[string]interface{}{
		"mode":      "rule",
		"log-level": "info",
	}
	src := map[string]interface{}{
		"log-level": "debug",
	}
	result := config.DeepMerge(dst, src)
	if result["log-level"] != "debug" {
		t.Errorf("expected log-level=debug, got %v", result["log-level"])
	}
	if result["mode"] != "rule" {
		t.Errorf("expected mode=rule to be preserved, got %v", result["mode"])
	}
}

func TestDeepMerge_MapRecursive(t *testing.T) {
	dst := map[string]interface{}{
		"dns": map[string]interface{}{
			"enable": true,
			"mode":   "fake-ip",
		},
	}
	src := map[string]interface{}{
		"dns": map[string]interface{}{
			"mode": "redir-host",
		},
	}
	result := config.DeepMerge(dst, src)
	dns, _ := result["dns"].(map[string]interface{})
	if dns == nil {
		t.Fatal("dns key missing in result")
	}
	if dns["mode"] != "redir-host" {
		t.Errorf("expected dns.mode=redir-host, got %v", dns["mode"])
	}
	if dns["enable"] != true {
		t.Errorf("expected dns.enable=true to be preserved, got %v", dns["enable"])
	}
}

func TestDeepMerge_ProxiesAppend(t *testing.T) {
	dst := map[string]interface{}{
		"proxies": []interface{}{
			map[string]interface{}{"name": "node-a", "type": "ss"},
		},
	}
	src := map[string]interface{}{
		"proxies": []interface{}{
			map[string]interface{}{"name": "node-b", "type": "vmess"},
		},
	}
	result := config.DeepMerge(dst, src)
	proxies, _ := result["proxies"].([]interface{})
	if len(proxies) != 2 {
		t.Errorf("expected 2 proxies after append, got %d", len(proxies))
	}
}

func TestDeepMerge_RulesPrepend(t *testing.T) {
	dst := map[string]interface{}{
		"rules": []interface{}{"MATCH,Final"},
	}
	src := map[string]interface{}{
		"rules": []interface{}{"DOMAIN,example.com,DIRECT"},
	}
	result := config.DeepMerge(dst, src)
	rules, _ := result["rules"].([]interface{})
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0] != "DOMAIN,example.com,DIRECT" {
		t.Errorf("override rule should be first, got %v", rules[0])
	}
	if rules[1] != "MATCH,Final" {
		t.Errorf("base rule should be last, got %v", rules[1])
	}
}

func TestDeepMerge_ProxyGroupsMergeByName(t *testing.T) {
	dst := map[string]interface{}{
		"proxy-groups": []interface{}{
			map[string]interface{}{"name": "Proxy", "type": "select", "proxies": []interface{}{"DIRECT"}},
			map[string]interface{}{"name": "Auto", "type": "url-test"},
		},
	}
	src := map[string]interface{}{
		"proxy-groups": []interface{}{
			map[string]interface{}{"name": "Proxy", "type": "select", "proxies": []interface{}{"NodeA", "DIRECT"}},
		},
	}
	result := config.DeepMerge(dst, src)
	groups, _ := result["proxy-groups"].([]interface{})
	if len(groups) != 2 {
		t.Fatalf("expected 2 proxy-groups (no duplication), got %d", len(groups))
	}
	// The "Proxy" group should be updated
	proxy, _ := groups[0].(map[string]interface{})
	proxies, _ := proxy["proxies"].([]interface{})
	if len(proxies) != 2 {
		t.Errorf("expected Proxy group to have 2 proxies from override, got %d", len(proxies))
	}
}

func TestDeepMerge_EmptyOverride(t *testing.T) {
	dst := map[string]interface{}{"key": "value"}
	result := config.DeepMerge(dst, nil)
	if result["key"] != "value" {
		t.Errorf("nil src should not change dst")
	}
}

func TestMergeWithOverrides_RealYAML(t *testing.T) {
	generated := map[string]interface{}{
		"port":      7890,
		"log-level": "info",
		"proxies":   []interface{}{},
		"rules":     []interface{}{"MATCH,Final"},
	}
	overrides := []byte(`
log-level: debug
rules:
  - "DOMAIN,override.example.com,DIRECT"
`)
	result, err := config.MergeWithOverrides(generated, overrides)
	if err != nil {
		t.Fatalf("MergeWithOverrides: %v", err)
	}
	if result["log-level"] != "debug" {
		t.Errorf("expected log-level=debug after override, got %v", result["log-level"])
	}
	rules, _ := result["rules"].([]interface{})
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0] != "DOMAIN,override.example.com,DIRECT" {
		t.Errorf("override rule should be first, got %v", rules[0])
	}
}
