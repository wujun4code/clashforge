package config_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wujun4code/clashforge/internal/config"
	"gopkg.in/yaml.v3"
)

func TestDNSYAMLResolversSurviveFullMergePipeline(t *testing.T) {
	cfg := defaultTestCfg()
	defer withGeosite(t, cfg)()
	base, err := config.GenerateFromBase(cfg, []byte(`
dns:
  default-nameserver: [tcp://192.0.2.1]
  proxy-server-nameserver: [https://nodes.example/dns-query#DIRECT]
  nameserver: [https://old.example/dns-query]
  fallback: [tls://192.0.2.2]
  nameserver-policy:
    '+.example.com': https://trusted.example/dns-query#DIRECT
rules: [MATCH,DIRECT]
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !stringsValueContains(generatedDNS(t, base)["default-nameserver"], "tcp://192.0.2.1") {
		t.Fatal("source YAML bootstrap resolver was discarded")
	}
	merged, err := config.MergeWithOverrides(base, []byte(`
dns:
  enable: false
  listen: 127.0.0.1:53
  enhanced-mode: redir-host
  fake-ip-range: 198.18.0.1/16
  default-nameserver: [tcp://192.0.2.3]
  nameserver: [https://new.example/dns-query]
  fallback: []
`))
	if err != nil {
		t.Fatal(err)
	}
	final := config.ApplyManagedRuntimeSettings(cfg, merged)
	data, err := config.MarshalYAML(final)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	dns := generatedDNS(t, decoded)
	for key, want := range map[string][]interface{}{
		"default-nameserver":      {"tcp://192.0.2.3"},
		"proxy-server-nameserver": {"https://nodes.example/dns-query#DIRECT"},
		"nameserver":              {"https://new.example/dns-query"},
		"fallback":                {},
	} {
		if !reflect.DeepEqual(dns[key], want) {
			t.Errorf("%s: want %v, got %v", key, want, dns[key])
		}
	}
	policy := nameserverPolicy(dns)
	if policy["+.example.com"] != "https://trusted.example/dns-query#DIRECT" || policy["geosite:cn"] == nil {
		t.Errorf("custom domain policy and domestic split must survive: %v", policy)
	}
	if dns["enable"] != true || dns["listen"] != "0.0.0.0:17874" || dns["enhanced-mode"] != "fake-ip" || dns["fake-ip-range"] == "198.18.0.1/16" {
		t.Errorf("runtime fields must remain managed: %v", dns)
	}
}

func TestDNSBootstrapSupportsIPTransportEndpoints(t *testing.T) {
	cfg := defaultTestCfg()
	withoutGeosite(cfg)
	cfg.DNS.Nameservers = []string{"tcp://192.0.2.1", "https://192.0.2.2/dns-query", "tls://[2001:db8::1]:853", "https://resolver.example/dns-query"}
	generated, err := config.Generate(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := generatedDNS(t, generated)["default-nameserver"]
	if !reflect.DeepEqual(got, cfg.DNS.Nameservers[:3]) {
		t.Errorf("bootstrap must preserve IP endpoints and exclude hostname endpoints: %v", got)
	}
}

func TestDNSStrategyAntiHijackNeverDegradesToPlaintext(t *testing.T) {
	for _, geosite := range []bool{true, false} {
		t.Run(map[bool]string{true: "with-geosite", false: "without-geosite"}[geosite], func(t *testing.T) {
			cfg := defaultTestCfg()
			cfg.DNS.Strategy = config.DNSStrategyAntiHijack
			// A provider selected by the old automatic probe is not an explicit
			// trusted-provider choice for the new strategy.
			cfg.DNS.DoH = []string{"https://doh.pub/dns-query"}
			if geosite {
				defer withGeosite(t, cfg)()
			} else {
				withoutGeosite(cfg)
			}
			generated, err := config.Generate(cfg, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := config.ValidateRuntimeDNS(cfg, generated); err != nil {
				t.Fatal(err)
			}
			dns := generatedDNS(t, generated)
			if stringsValueContains(dns["proxy-server-nameserver"], "https://doh.pub/dns-query#DIRECT") {
				t.Error("strict preset must not adopt the legacy probe's provider")
			}
			if dns["fallback"] != nil || dns["fallback-filter"] != nil || dns["use-system-hosts"] != false {
				t.Errorf("strict preset must not inherit legacy fallback/system hosts: %v", dns)
			}
			policy := nameserverPolicy(dns)
			if geosite && policy["geosite:cn"] == nil {
				t.Error("domestic DNS split missing")
			}
			if !geosite && policy != nil {
				t.Error("must not reference missing GeoSite")
			}
		})
	}
}

func generatedDNS(t *testing.T, generated map[string]interface{}) map[string]interface{} {
	t.Helper()
	dns, ok := generated["dns"].(map[string]interface{})
	if !ok {
		t.Fatal("generated DNS missing")
	}
	return dns
}

func TestDNSStrategyAntiHijackRejectsUnsafeMergedYAML(t *testing.T) {
	cases := []struct{ name, yaml string }{
		{"plaintext-bootstrap", "default-nameserver: [223.5.5.5]"},
		{"tcp-is-not-encryption", "default-nameserver: [tcp://192.0.2.1]"},
		{"hostname-bootstrap", "default-nameserver: [https://dns.example/dns-query]"},
		{"plaintext-node", "proxy-server-nameserver: [119.29.29.29]"},
		{"certificate-bypass", "nameserver: ['https://192.0.2.1/dns-query#skip-cert-verify=true']"},
		{"plaintext-fallback", "fallback: [192.0.2.1]"},
		{"plaintext-direct", "direct-nameserver: [system]"},
		{"plaintext-policy", "nameserver-policy: {'+.example': 223.5.5.5}"},
		{"node-policy-cycle", "proxy-server-nameserver-policy: {'node.example': 'https://192.0.2.1/dns-query#Proxy'}"},
		{"node-cycle", "proxy-server-nameserver: ['https://192.0.2.1/dns-query#RULES']"},
		{"bootstrap-cycle", "default-nameserver: ['https://192.0.2.1/dns-query#Proxy']"},
		{"empty-main", "nameserver: []"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultTestCfg()
			cfg.DNS.Strategy = config.DNSStrategyAntiHijack
			withoutGeosite(cfg)
			base, err := config.Generate(cfg, nil)
			if err != nil {
				t.Fatal(err)
			}
			merged, err := config.MergeWithOverrides(base, []byte("dns:\n  "+tc.yaml+"\n"))
			if err != nil {
				t.Fatal(err)
			}
			merged = config.ApplyManagedRuntimeSettings(cfg, merged)
			if err := config.ValidateRuntimeDNS(cfg, merged); err == nil || !strings.Contains(err.Error(), "防劫持") {
				t.Fatalf("unsafe YAML must produce an actionable strict-mode error: %v", err)
			}
		})
	}
}
