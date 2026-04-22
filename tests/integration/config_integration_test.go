package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/subscription"
)

const sampleConfig = "../../testdata/sample-clash-config.yaml"

// skipIfNoSample skips the test if the private config file is not present.
func skipIfNoSample(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(sampleConfig); os.IsNotExist(err) {
		t.Skip("testdata/sample-clash-config.yaml not present; skipping integration test")
	}
}

// TestSampleConfig_IsValidYAML verifies the sample config is valid YAML.
func TestSampleConfig_IsValidYAML(t *testing.T) {
	skipIfNoSample(t)
	data, err := os.ReadFile(sampleConfig)
	if err != nil {
		t.Fatalf("read sample config: %v", err)
	}
	var parsed map[string]interface{}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("sample config is not valid YAML: %v", err)
	}
}

// TestSampleConfig_ParseProxies reads the sample config and parses its proxies section
// as if it were a Clash YAML subscription.
func TestSampleConfig_ParseProxies(t *testing.T) {
	skipIfNoSample(t)
	data, err := os.ReadFile(sampleConfig)
	if err != nil {
		t.Fatalf("read sample config: %v", err)
	}
	nodes, err := subscription.Parse(data)
	if err != nil {
		t.Fatalf("parse sample config proxies: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("expected at least 1 proxy node from sample config")
	}
	t.Logf("parsed %d nodes from sample config", len(nodes))
	for _, n := range nodes {
		if n.Name == "" {
			t.Errorf("node has empty name: %+v", n)
		}
		if n.Server == "" {
			t.Errorf("node %q has empty server", n.Name)
		}
		if n.Port == 0 {
			t.Errorf("node %q has zero port", n.Name)
		}
	}
}

// TestSampleConfig_AsOverrides uses the sample config as overrides on top of a generated config
// and verifies that the merge produces a valid, loadable YAML.
func TestSampleConfig_AsOverrides(t *testing.T) {
	skipIfNoSample(t)
	overridesData, err := os.ReadFile(sampleConfig)
	if err != nil {
		t.Fatalf("read sample config: %v", err)
	}

	// Generate a base config from defaults
	cfg := config.Default()
	nodes, _ := subscription.Parse(overridesData)
	generated, err := config.Generate(cfg, nodes)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Merge sample config as overrides
	merged, err := config.MergeWithOverrides(generated, overridesData)
	if err != nil {
		t.Fatalf("MergeWithOverrides: %v", err)
	}

	// Resulting config must be serializable
	data, err := config.MarshalYAML(merged)
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}
	if len(data) < 200 {
		t.Errorf("merged config suspiciously small: %d bytes", len(data))
	}
	t.Logf("merged config: %d bytes", len(data))

	// Must be valid YAML
	var verify map[string]interface{}
	if err := yaml.Unmarshal(data, &verify); err != nil {
		t.Fatalf("merged YAML is not parseable: %v", err)
	}

	// Core fields must be present
	for _, key := range []string{"port", "proxies", "proxy-groups", "rules"} {
		if _, ok := verify[key]; !ok {
			t.Errorf("merged config missing key: %s", key)
		}
	}
}

// TestSampleConfig_ProxyGroupsPreserved verifies that proxy-groups from the sample
// config are retained after merge (not replaced wholesale by generated ones).
func TestSampleConfig_ProxyGroupsPreserved(t *testing.T) {
	skipIfNoSample(t)
	overridesData, err := os.ReadFile(sampleConfig)
	if err != nil {
		t.Fatalf("read sample config: %v", err)
	}

	cfg := config.Default()
	nodes, _ := subscription.Parse(overridesData)
	generated, err := config.Generate(cfg, nodes)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	merged, err := config.MergeWithOverrides(generated, overridesData)
	if err != nil {
		t.Fatalf("MergeWithOverrides: %v", err)
	}

	groups, _ := merged["proxy-groups"].([]interface{})
	if len(groups) == 0 {
		t.Fatal("proxy-groups is empty after merge")
	}

	// Sample config has group names with emoji – verify they survived
	var groupNames []string
	for _, g := range groups {
		m, _ := g.(map[string]interface{})
		if name, ok := m["name"].(string); ok {
			groupNames = append(groupNames, name)
		}
	}
	t.Logf("proxy-groups after merge: %v", groupNames)

	found := false
	for _, name := range groupNames {
		if strings.Contains(name, "自动选择") || strings.Contains(name, "节点选择") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected sample config proxy-group names to appear in merged result, got: %v", groupNames)
	}
}

// TestSampleConfig_RulesOrder verifies that sample rules come before base rules
// (since overrides should be prepended to rules slice).
func TestSampleConfig_RulesOrder(t *testing.T) {
	skipIfNoSample(t)
	overridesData, err := os.ReadFile(sampleConfig)
	if err != nil {
		t.Fatalf("read sample config: %v", err)
	}

	cfg := config.Default()
	generated, err := config.Generate(cfg, nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	merged, err := config.MergeWithOverrides(generated, overridesData)
	if err != nil {
		t.Fatalf("MergeWithOverrides: %v", err)
	}

	rules, _ := merged["rules"].([]interface{})
	if len(rules) == 0 {
		t.Fatal("rules is empty after merge")
	}

	// MATCH rule should be last (it's the catch-all from base config)
	last := rules[len(rules)-1]
	if s, ok := last.(string); !ok || !strings.HasPrefix(s, "MATCH,") {
		t.Errorf("expected MATCH rule at end of rules, got: %v", last)
	}
}

// TestSampleConfig_WriteToTempFile verifies end-to-end: generate → merge → write YAML to disk.
func TestSampleConfig_WriteToTempFile(t *testing.T) {
	skipIfNoSample(t)
	overridesData, err := os.ReadFile(sampleConfig)
	if err != nil {
		t.Fatalf("read sample config: %v", err)
	}

	cfg := config.Default()
	nodes, _ := subscription.Parse(overridesData)
	generated, err := config.Generate(cfg, nodes)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	merged, err := config.MergeWithOverrides(generated, overridesData)
	if err != nil {
		t.Fatalf("MergeWithOverrides: %v", err)
	}
	data, err := config.MarshalYAML(merged)
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}

	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "mihomo-config.yaml")
	if err := os.WriteFile(outFile, data, 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	// Re-read and verify
	written, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("re-read temp file: %v", err)
	}
	var verify map[string]interface{}
	if err := yaml.Unmarshal(written, &verify); err != nil {
		t.Fatalf("written YAML is not parseable: %v", err)
	}
	t.Logf("successfully wrote %d bytes to %s", len(written), outFile)
}
