package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/wujun4code/clashforge/internal/subscription"
	"gopkg.in/yaml.v3"
)

// Generate 从 MetaclashConfig + 节点列表生成 mihomo YAML 配置 map
func Generate(cfg *MetaclashConfig, nodes []subscription.ProxyNode) (map[string]interface{}, error) {
	out := map[string]interface{}{}

	// 基础配置
	out["port"] = cfg.Ports.HTTP
	out["socks-port"] = cfg.Ports.SOCKS
	out["mixed-port"] = cfg.Ports.Mixed
	out["redir-port"] = cfg.Ports.Redir
	out["tproxy-port"] = cfg.Ports.TProxy
	out["allow-lan"] = cfg.Security.AllowLAN
	out["bind-address"] = "*"
	out["mode"] = "rule"
	out["log-level"] = normalizeMihomoLogLevel(cfg.Log.Level)
	out["external-controller"] = fmt.Sprintf("127.0.0.1:%d", cfg.Ports.MihomoAPI)
	out["unified-delay"] = true
	out["tcp-concurrent"] = true
	out["geodata-mode"] = true
	out["geox-url"] = map[string]string{
		"mmdb": filepath.ToSlash(cfg.Core.GeoIPPath),
	}

	// DNS 配置
	if cfg.DNS.Enable {
		out["dns"] = buildDNSMap(cfg)
	}

	// Proxies
	var proxies []map[string]interface{}
	var proxyNames []string
	for _, node := range nodes {
		p := map[string]interface{}{
			"name":   node.Name,
			"type":   node.Type,
			"server": node.Server,
			"port":   node.Port,
		}
		for k, v := range node.Extra {
			p[k] = v
		}
		proxies = append(proxies, p)
		proxyNames = append(proxyNames, node.Name)
	}
	out["proxies"] = proxies

	// Proxy Groups
	autoProxies := proxyNames
	if len(autoProxies) == 0 {
		autoProxies = []string{"DIRECT"}
	}
	selectProxies := append([]string{"Auto", "DIRECT"}, proxyNames...)

	out["proxy-groups"] = []map[string]interface{}{
		{
			"name":    "Proxy",
			"type":    "select",
			"proxies": selectProxies,
		},
		{
			"name":      "Auto",
			"type":      "url-test",
			"url":       "http://www.gstatic.com/generate_204",
			"interval":  300,
			"tolerance": 50,
			"proxies":   autoProxies,
		},
		{
			"name":    "Final",
			"type":    "select",
			"proxies": []string{"Proxy", "DIRECT"},
		},
	}

	// Rules
	rules := []string{
		"DOMAIN-SUFFIX,local,DIRECT",
		"IP-CIDR,127.0.0.0/8,DIRECT,no-resolve",
		"IP-CIDR,192.168.0.0/16,DIRECT,no-resolve",
		"IP-CIDR,10.0.0.0/8,DIRECT,no-resolve",
		"IP-CIDR,172.16.0.0/12,DIRECT,no-resolve",
		"IP-CIDR,100.64.0.0/10,DIRECT,no-resolve",
	}
	for _, cidr := range cfg.Network.BypassCIDR {
		rules = append(rules, fmt.Sprintf("IP-CIDR,%s,DIRECT,no-resolve", cidr))
	}
	if cfg.Network.BypassChina {
		rules = append(rules, "GEOSITE,cn,DIRECT")
		rules = append(rules, "GEOIP,CN,DIRECT,no-resolve")
	}
	rules = append(rules, "MATCH,Final")
	out["rules"] = rules

	return out, nil
}

// buildDNSMap constructs the Mihomo DNS config map from ClashForge's DNS settings.
func buildDNSMap(cfg *MetaclashConfig) map[string]interface{} {
	dnsMap := map[string]interface{}{
		"enable":     true,
		"listen":     fmt.Sprintf("0.0.0.0:%d", cfg.Ports.DNS),
		"ipv6":       cfg.Network.IPv6,
		"use-hosts":  true,
		"nameserver": cfg.DNS.Nameservers,
	}
	if cfg.DNS.Mode == "fake-ip" {
		dnsMap["enhanced-mode"] = "fake-ip"
		dnsMap["fake-ip-range"] = "198.18.0.1/16"
		if len(cfg.DNS.FakeIPFilter) > 0 {
			dnsMap["fake-ip-filter"] = cfg.DNS.FakeIPFilter
		}
	} else {
		dnsMap["enhanced-mode"] = "redir-host"
	}
	if len(cfg.DNS.Fallback) > 0 {
		dnsMap["fallback"] = cfg.DNS.Fallback
	}
	// default-nameserver must be pure IPs (Mihomo requirement).
	var bootstrapIPs []string
	for _, ns := range cfg.DNS.Nameservers {
		if !strings.HasPrefix(ns, "https://") && !strings.HasPrefix(ns, "tls://") && !strings.HasPrefix(ns, "tcp://") {
			bootstrapIPs = append(bootstrapIPs, ns)
		}
	}
	if len(bootstrapIPs) > 0 {
		dnsMap["default-nameserver"] = bootstrapIPs
	}
	if len(cfg.DNS.DoH) > 0 {
		if existing, ok := dnsMap["nameserver"].([]string); ok {
			dnsMap["nameserver"] = append(existing, cfg.DNS.DoH...)
		} else {
			dnsMap["nameserver"] = cfg.DNS.DoH
		}
	}
	return dnsMap
}

// GenerateFromBase uses rawYAML as the base Mihomo config, rewriting only the
// DNS section from ClashForge settings.  All other sections (proxies,
// proxy-groups, rules, rule-providers, …) are preserved as-is from the
// subscription.  extraNodes are proxy nodes from additional subscriptions
// (those without a raw YAML); they are appended, skipping duplicates by name.
// Falls back to Generate if rawYAML is empty or unparseable.
func GenerateFromBase(cfg *MetaclashConfig, rawYAML []byte, extraNodes []subscription.ProxyNode) (map[string]interface{}, error) {
	if len(rawYAML) == 0 {
		return Generate(cfg, extraNodes)
	}
	var base map[string]interface{}
	if err := yaml.Unmarshal(rawYAML, &base); err != nil || base == nil {
		return Generate(cfg, extraNodes)
	}

	// Rewrite DNS from ClashForge config
	if cfg.DNS.Enable {
		base["dns"] = buildDNSMap(cfg)
	} else {
		delete(base, "dns")
	}

	// Enforce local geodata so ClashForge-managed files are used
	base["geodata-mode"] = true
	base["geox-url"] = map[string]string{
		"mmdb": filepath.ToSlash(cfg.Core.GeoIPPath),
	}

	// Append proxy nodes from other (nodes-only) subscriptions, deduplicating by name
	if len(extraNodes) > 0 {
		existingNames := map[string]bool{}
		if proxies, ok := base["proxies"].([]interface{}); ok {
			for _, p := range proxies {
				if pm, ok2 := p.(map[string]interface{}); ok2 {
					if name, ok3 := pm["name"].(string); ok3 {
						existingNames[name] = true
					}
				}
			}
		}
		var added []interface{}
		for _, node := range extraNodes {
			if existingNames[node.Name] {
				continue
			}
			p := map[string]interface{}{
				"name": node.Name, "type": node.Type,
				"server": node.Server, "port": node.Port,
			}
			for k, v := range node.Extra {
				p[k] = v
			}
			added = append(added, p)
			existingNames[node.Name] = true
		}
		if len(added) > 0 {
			if proxies, ok := base["proxies"].([]interface{}); ok {
				base["proxies"] = append(proxies, added...)
			} else {
				base["proxies"] = added
			}
		}
	}

	return base, nil
}

func normalizeMihomoLogLevel(level string) string {
	level = strings.ToLower(strings.TrimSpace(level))
	switch level {
	case "debug", "info", "warning", "error", "silent":
		return level
	case "warn":
		return "warning"
	default:
		return "info"
	}
}

// ApplyManagedRuntimeSettings rewrites runtime-owned fields after overrides merge.
// This keeps legacy overrides from stealing managed ports or re-enabling DNS with stale values.
func ApplyManagedRuntimeSettings(cfg *MetaclashConfig, merged map[string]interface{}) map[string]interface{} {
	if merged == nil {
		merged = map[string]interface{}{}
	}

	// Prevent legacy/OpenClash UI overrides from injecting unsafe local paths
	// (for example /usr/share/openclash/ui) that violate Mihomo SAFE_PATHS.
	delete(merged, "external-ui")
	delete(merged, "external-ui-name")
	delete(merged, "external-ui-url")

	merged["port"] = cfg.Ports.HTTP
	merged["socks-port"] = cfg.Ports.SOCKS
	merged["mixed-port"] = cfg.Ports.Mixed
	merged["redir-port"] = cfg.Ports.Redir
	merged["tproxy-port"] = cfg.Ports.TProxy
	merged["external-controller"] = fmt.Sprintf("127.0.0.1:%d", cfg.Ports.MihomoAPI)
	// ClashForge owns the mihomo API endpoint (localhost-only); strip any secret
	// the user may have imported so our proxy handler can always reach it unauthenticated.
	delete(merged, "secret")

	if !cfg.DNS.Enable {
		delete(merged, "dns")
		return merged
	}

	dnsMap, ok := merged["dns"].(map[string]interface{})
	if !ok || dnsMap == nil {
		dnsMap = map[string]interface{}{}
	}
	dnsMap["enable"] = true
	dnsMap["listen"] = fmt.Sprintf("0.0.0.0:%d", cfg.Ports.DNS)
	merged["dns"] = dnsMap

	return merged
}

// MarshalYAML 将配置 map 序列化为 YAML 字节
func MarshalYAML(cfg map[string]interface{}) ([]byte, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal mihomo config: %w", err)
	}
	return data, nil
}
