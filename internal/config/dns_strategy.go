package config

import (
	"fmt"
	"net/url"
	"strings"
)

// Bootstrap and node lookups must work before a proxy can be connected. Use
// independent encrypted IP endpoints, with Mihomo's native name-cert-verify
// parameter retaining hostname verification. These DNS4EU endpoints were
// validated on a WAN where Cloudflare and Google IPs cannot be reached directly.
// Reachability remains network-dependent; users can replace them in their YAML.
func applyAntiHijackDefaults(cfg *MetaclashConfig, dns map[string]interface{}, bootstrap []string) {
	secureBootstrap := []string{
		"https://86.54.11.100/dns-query#DIRECT&name-cert-verify=unfiltered.joindns4.eu",
		"https://86.54.11.200/dns-query#DIRECT&name-cert-verify=unfiltered.joindns4.eu",
	}
	var encryptedBootstrap []string
	for _, ns := range bootstrap {
		if encryptedNameserver(ns) {
			encryptedBootstrap = append(encryptedBootstrap, ns)
		}
	}
	if len(encryptedBootstrap) > 0 {
		secureBootstrap = encryptedBootstrap
	}
	nodes := secureBootstrap
	// Domestic website DNS is separate from node DNS: retain local CDN answers
	// without trusting a domestic provider to resolve the proxy's hostname.
	domestic := []string{"https://doh.pub/dns-query#DIRECT"}
	// DoH may have been populated by the legacy automatic probe. Strict mode
	// must not adopt that provider as trusted without an explicit YAML choice.
	foreign := cfg.DNS.Fallback
	if len(foreign) == 0 {
		foreign = []string{"https://1.1.1.1/dns-query", "https://8.8.8.8/dns-query"}
	}
	foreign = rulesAwareNameservers(foreign)
	dns["default-nameserver"] = secureBootstrap
	dns["proxy-server-nameserver"] = nodes
	dns["nameserver"] = foreign
	dns["direct-nameserver"] = domestic
	dns["direct-nameserver-follow-policy"] = true
	dns["respect-rules"] = true
	dns["use-hosts"] = true
	dns["use-system-hosts"] = false
	// Route queries by domain instead of racing a plaintext primary against
	// an encrypted fallback. Missing GeoSite never downgrades to ISP DNS.
	delete(dns, "fallback")
	delete(dns, "fallback-filter")
	delete(dns, "nameserver-policy")
	if geositeExists(cfg.Core.DataDir) {
		dns["nameserver-policy"] = map[string]interface{}{
			"geosite:cn":              domestic,
			"geosite:geolocation-!cn": foreign,
		}
	}
}

// #RULES makes the DNS transport follow the user's routing rules. A resolver
// already pinned to a proxy/interface keeps that explicit choice. No proxy
// selector is invented, and DIRECT routing remains a user-controlled decision.
func rulesAwareNameservers(servers []string) []string {
	result := make([]string, len(servers))
	for i, ns := range servers {
		if strings.Contains(ns, "#") {
			result[i] = ns
		} else {
			result[i] = ns + "#RULES"
		}
	}
	return result
}

func encryptedNameserver(ns string) bool {
	u, err := url.Parse(strings.TrimSpace(ns))
	return err == nil && u.Hostname() != "" &&
		(u.Scheme == "https" || u.Scheme == "tls" || u.Scheme == "quic")
}

// ValidateRuntimeDNS runs after ALL YAML merges, before writing the runtime
// config. Strict mode must never silently inherit plaintext/insecure resolvers.
func ValidateRuntimeDNS(cfg *MetaclashConfig, generated map[string]interface{}) error {
	if !cfg.DNS.Enable || !strings.EqualFold(strings.TrimSpace(cfg.DNS.Strategy), DNSStrategyAntiHijack) {
		return nil
	}
	dns, ok := generated["dns"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("防劫持策略需要启用 Mihomo DNS")
	}
	for _, field := range []string{"default-nameserver", "proxy-server-nameserver", "nameserver", "fallback", "direct-nameserver"} {
		required := field == "default-nameserver" || field == "proxy-server-nameserver" || field == "nameserver"
		servers, err := dnsServers(dns[field])
		if err != nil || (required && len(servers) == 0) {
			return fmt.Errorf("防劫持策略：dns.%s 必须配置有效的加密 DNS 列表", field)
		}
		for _, ns := range servers {
			if err := validateEncryptedDNS(field, ns); err != nil {
				return err
			}
		}
	}
	for _, field := range []string{"nameserver-policy", "proxy-server-nameserver-policy"} {
		if dns[field] == nil {
			continue
		}
		policy, ok := dns[field].(map[string]interface{})
		if !ok {
			return fmt.Errorf("防劫持策略：dns.%s 必须是域名到加密 DNS 的映射", field)
		}
		for domain, value := range policy {
			servers, err := dnsServers(value)
			if err != nil || len(servers) == 0 {
				return fmt.Errorf("防劫持策略：dns.%s[%s] 缺少加密 DNS", field, domain)
			}
			for _, ns := range servers {
				if err := validateEncryptedDNS(field, ns); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func dnsServers(value interface{}) ([]string, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case string:
		return []string{v}, nil
	case []string:
		return v, nil
	case []interface{}:
		servers := make([]string, len(v))
		for i, item := range v {
			var ok bool
			servers[i], ok = item.(string)
			if !ok {
				return nil, fmt.Errorf("invalid DNS entry")
			}
		}
		return servers, nil
	default:
		return nil, fmt.Errorf("invalid DNS list")
	}
}

func validateEncryptedDNS(field, ns string) error {
	if !encryptedNameserver(ns) {
		return fmt.Errorf("防劫持策略：dns.%s 含明文或无效上游 %q，请使用 DoH/DoT/DoQ", field, ns)
	}
	u, _ := url.Parse(strings.TrimSpace(ns))
	if field == "default-nameserver" && !isIPNameserver(ns) {
		return fmt.Errorf("防劫持策略：default-nameserver 必须使用 IP 地址的加密 DNS")
	}
	for _, part := range strings.Split(u.Fragment, "&") {
		key, value, param := strings.Cut(part, "=")
		if key == "skip-cert-verify" && value != "false" {
			return fmt.Errorf("防劫持策略：dns.%s 禁止跳过 TLS 证书验证", field)
		}
		independent := field == "default-nameserver" || strings.HasPrefix(field, "proxy-server-nameserver")
		if independent && !param && key != "" && key != "DIRECT" {
			return fmt.Errorf("防劫持策略：dns.%s 必须直连解析，不能依赖待解析的代理；请使用 #DIRECT", field)
		}
	}
	return nil
}
