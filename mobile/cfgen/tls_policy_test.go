package cfgen

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDoHCertificatePolicyPreserved(t *testing.T) {
	for _, raw := range []string{
		"https://1.1.1.1/dns-query",
		"https://[2606:4700:4700::1111]/dns-query",
		"https://dns.example.test/dns-query#DIRECT",
		"https://1.1.1.1/dns-query#skip-cert-verify=false",
		"https://1.1.1.1/dns-query#skip-cert-verify=true",
		"https://1.1.1.1/dns-query#DIRECT&skip-cert-verify=false",
		"https://1.1.1.1/dns-query#DIRECT&skip-cert-verify=true",
	} {
		t.Run(raw, func(t *testing.T) {
			got := normalizeDoHNameservers([]string{"", "  " + raw + "  "})
			if !reflect.DeepEqual(got, []string{raw}) {
				t.Fatalf("certificate policy changed: want %q, got %q", raw, got)
			}
		})
	}
}

func TestGenerateConfigPreservesProxyCertificatePolicy(t *testing.T) {
	for _, powerProfile := range []string{"balanced", "battery", "performance"} {
		for _, policy := range []string{"", "    skip-cert-verify: false\n", "    skip-cert-verify: true\n"} {
			t.Run(powerProfile+"/"+policy, func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, "config.yaml")
				input := []byte("proxies:\n  - name: fixture\n    type: vless\n    server: proxy.example.test\n    port: 443\n    tls: true\n    sni: certificate.example.test\n" + policy + "dns:\n  nameserver: [https://1.1.1.1/dns-query]\n")
				if err := os.WriteFile(path, input, 0600); err != nil {
					t.Fatal(err)
				}
				var before map[string]interface{}
				if err := yaml.Unmarshal(input, &before); err != nil {
					t.Fatal(err)
				}
				result := GenerateConfig(path, 0, dir, "fake-ip", powerProfile)
				if !result.OK {
					t.Fatal(result.Error)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var after map[string]interface{}
				if err := yaml.Unmarshal(data, &after); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(before["proxies"], after["proxies"]) {
					t.Fatal("GenerateConfig modified proxy TLS policy")
				}
			})
		}
	}
}
