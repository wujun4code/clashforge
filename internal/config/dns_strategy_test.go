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

func TestAntiHijackSeparatesIndependentBootstrapAndRuleAwareWebsiteDNS(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.DNS.Strategy = config.DNSStrategyAntiHijack
	withoutGeosite(cfg)
	cfg.DNS.Fallback = []string{
		"https://dns.google/dns-query",
		"tls://8.8.4.4",
		"https://chosen.example/dns-query#MyProxy",
		"https://local.example/dns-query#DIRECT",
	}
	generated, err := config.Generate(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.ValidateRuntimeDNS(cfg, generated); err != nil {
		t.Fatal(err)
	}
	dns := generatedDNS(t, generated)
	bootstrap := dns["default-nameserver"].([]string)
	if len(bootstrap) < 2 {
		t.Fatal("independent bootstrap must have multiple encrypted endpoints")
	}
	for _, ns := range bootstrap {
		if !strings.Contains(ns, "#DIRECT&name-cert-verify=unfiltered.joindns4.eu") {
			t.Errorf("bootstrap must use independent DNS4EU with certificate verification: %s", ns)
		}
		if strings.Contains(ns, "1.1.1.1/") || strings.Contains(ns, "8.8.8.8/") {
			t.Errorf("bootstrap must not rely on the directly unreachable website resolvers: %s", ns)
		}
	}
	if !reflect.DeepEqual(dns["proxy-server-nameserver"], bootstrap) {
		t.Errorf("node lookups must stay independent of website proxy DNS: %v", dns)
	}
	if reflect.DeepEqual(dns["direct-nameserver"], bootstrap) || !stringsValueContains(dns["direct-nameserver"], "https://doh.pub/dns-query#DIRECT") {
		t.Errorf("direct websites need domestic encrypted DNS separate from node DNS: %v", dns["direct-nameserver"])
	}
	wantForeign := []string{
		"https://dns.google/dns-query#RULES",
		"tls://8.8.4.4#RULES",
		"https://chosen.example/dns-query#MyProxy",
		"https://local.example/dns-query#DIRECT",
	}
	if !reflect.DeepEqual(dns["nameserver"], wantForeign) {
		t.Errorf("website lookups must obey routing without replacing explicit transports: %v", dns["nameserver"])
	}
	if !reflect.DeepEqual(cfg.DNS.Fallback, []string{"https://dns.google/dns-query", "tls://8.8.4.4", "https://chosen.example/dns-query#MyProxy", "https://local.example/dns-query#DIRECT"}) {
		t.Error("generation modified the saved user resolver list")
	}
	if dns["direct-nameserver-follow-policy"] != true {
		t.Error("direct website lookups must honor domestic/custom DNS policy")
	}
}

func TestAntiHijackDomesticPolicyNeverOverridesNodeDNS(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.DNS.Strategy = config.DNSStrategyAntiHijack
	defer withGeosite(t, cfg)()
	generated, err := config.Generate(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	dns := generatedDNS(t, generated)
	policy := nameserverPolicy(dns)
	if !reflect.DeepEqual(policy["geosite:cn"], dns["direct-nameserver"]) {
		t.Error("domestic website policy must use the domestic direct resolver")
	}
	if !reflect.DeepEqual(policy["geosite:geolocation-!cn"], dns["nameserver"]) {
		t.Error("foreign website policy must use rule-routed encrypted resolvers")
	}
	if stringsValueContains(dns["proxy-server-nameserver"], "https://doh.pub/dns-query#DIRECT") {
		t.Error("domestic website provider leaked into independent node DNS")
	}
	if err := config.ValidateRuntimeDNS(cfg, generated); err != nil {
		t.Fatal(err)
	}
}

func TestAntiHijackKeepsExplicitEncryptedIPBootstrap(t *testing.T) {
	cfg := defaultTestCfg()
	cfg.DNS.Strategy = config.DNSStrategyAntiHijack
	withoutGeosite(cfg)
	cfg.DNS.Nameservers = []string{"https://192.0.2.1/dns-query#DIRECT&name-cert-verify=trusted.example"}
	generated, err := config.Generate(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.ValidateRuntimeDNS(cfg, generated); err != nil {
		t.Fatal(err)
	}
	dns := generatedDNS(t, generated)
	for _, field := range []string{"default-nameserver", "proxy-server-nameserver"} {
		if !reflect.DeepEqual(dns[field], cfg.DNS.Nameservers) {
			t.Errorf("%s replaced the explicitly selected encrypted bootstrap: %v", field, dns[field])
		}
	}
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
