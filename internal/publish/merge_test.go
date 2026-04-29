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
