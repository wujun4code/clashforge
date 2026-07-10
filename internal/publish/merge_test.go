package publish

import (
	"strings"
	"testing"
)

func TestMergeTemplateWithNodes_IncludesCredentials(t *testing.T) {
	template := `mode: rule
proxy-groups:
  - name: "🚀 节点选择"
    type: select
    proxies:
      - DIRECT
rules:
  - MATCH,🚀 节点选择
`

	out, err := MergeTemplateWithNodes(template, []MergeNode{
		{
			ID:            "n1",
			Name:          "Tokyo",
			Domain:        "tokyo.example.com",
			ProxyUser:     "user-a",
			ProxyPassword: "pass-a",
		},
	})
	if err != nil {
		t.Fatalf("MergeTemplateWithNodes() error = %v", err)
	}

	for _, needle := range []string{
		"name: Tokyo",
		"server: tokyo.example.com",
		"username: user-a",
		"password: pass-a",
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("merged yaml missing %q\noutput:\n%s", needle, out)
		}
	}
}

func TestInjectRuleProviders_UsesYAMLFormatMatchingBuildRulePayload(t *testing.T) {
	template := `mode: rule
rules:
  - MATCH,🚀 节点选择
`
	out, err := InjectRuleProviders(template, []RuleSet{
		{Name: "pikaqiu", AccessURL: "https://example.workers.dev/abc?token=xyz"},
	})
	if err != nil {
		t.Fatalf("InjectRuleProviders() error = %v", err)
	}

	// buildRulePayload (internal/api/handler_publish_rulesets.go) always uploads
	// rules wrapped in a "payload:" YAML list. The provider's declared format
	// must be "yaml" so mihomo parses that body instead of treating it as raw
	// text lines (which silently yields zero rules).
	if !strings.Contains(out, "format: yaml") {
		t.Fatalf("expected rule-provider format to be yaml, got:\n%s", out)
	}
	if strings.Contains(out, "format: text") {
		t.Fatalf("rule-provider format must not be text (mismatches buildRulePayload's YAML output):\n%s", out)
	}
	if !strings.Contains(out, "RULE-SET,pikaqiu,") {
		t.Fatalf("expected injected RULE-SET rule for pikaqiu, got:\n%s", out)
	}
}

func TestMergeTemplateWithNodes_RejectsMissingCredentials(t *testing.T) {
	_, err := MergeTemplateWithNodes("mode: rule\n", []MergeNode{
		{
			ID:     "n1",
			Name:   "Tokyo",
			Domain: "tokyo.example.com",
		},
	})
	if err == nil {
		t.Fatal("expected error when proxy credentials are missing")
	}
}
