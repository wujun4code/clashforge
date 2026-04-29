package publish

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	masterGroupName = "🚀 节点选择"
	autoGroupName   = "♻️ 自动选择"
)

func MergeTemplateWithNodes(templateYAML string, nodes []MergeNode) (string, error) {
	if strings.TrimSpace(templateYAML) == "" {
		return "", fmt.Errorf("template content is empty")
	}
	if len(nodes) == 0 {
		return "", fmt.Errorf("no nodes selected")
	}

	var root map[string]interface{}
	if err := yaml.Unmarshal([]byte(templateYAML), &root); err != nil {
		return "", fmt.Errorf("parse template yaml: %w", err)
	}
	if root == nil {
		root = map[string]interface{}{}
	}

	proxies, proxyNames, err := buildProxyList(nodes)
	if err != nil {
		return "", err
	}
	root["proxies"] = proxies

	filteredGroups := readProxyGroups(root["proxy-groups"])
	filteredGroups = removeInjectedGroups(filteredGroups)
	injected := buildInjectedGroups(proxyNames)
	root["proxy-groups"] = append(injected, filteredGroups...)

	merged, err := yaml.Marshal(root)
	if err != nil {
		return "", fmt.Errorf("marshal merged yaml: %w", err)
	}
	return string(merged), nil
}

func buildProxyList(nodes []MergeNode) ([]interface{}, []string, error) {
	proxies := make([]interface{}, 0, len(nodes))
	proxyNames := make([]string, 0, len(nodes))
	usedName := map[string]int{}

	for _, n := range nodes {
		baseName := strings.TrimSpace(n.Name)

		var proxy map[string]interface{}

		if n.NodeType == "worker" {
			hostname := strings.TrimSpace(n.WorkerHostname)
			if hostname == "" {
				return nil, nil, fmt.Errorf("Worker 节点 %q 缺少域名", n.Name)
			}
			if strings.TrimSpace(n.WorkerUUID) == "" {
				return nil, nil, fmt.Errorf("Worker 节点 %q 缺少 UUID", n.Name)
			}
			if baseName == "" {
				baseName = hostname
			}
			name := dedupeName(baseName, usedName)
			proxyNames = append(proxyNames, name)
			proxy = map[string]interface{}{
				"name":    name,
				"type":    "vless",
				"server":  hostname,
				"port":    443,
				"uuid":    strings.TrimSpace(n.WorkerUUID),
				"tls":     true,
				"network": "ws",
				"ws-opts": map[string]interface{}{
					"path": "/",
					"headers": map[string]interface{}{
						"Host": hostname,
					},
				},
				"udp": false,
			}
		} else {
			server := strings.TrimSpace(n.Domain)
			if server == "" {
				server = strings.TrimSpace(n.Host)
			}
			if server == "" {
				return nil, nil, fmt.Errorf("节点 %q 缺少域名/主机", n.Name)
			}
			if strings.TrimSpace(n.ProxyUser) == "" || strings.TrimSpace(n.ProxyPassword) == "" {
				return nil, nil, fmt.Errorf("节点 %q 缺少代理账号或密码，请先重新部署节点", n.Name)
			}
			if baseName == "" {
				baseName = server
			}
			name := dedupeName(baseName, usedName)
			proxyNames = append(proxyNames, name)
			proxy = map[string]interface{}{
				"name":             name,
				"type":             "http",
				"server":           server,
				"port":             443,
				"username":         strings.TrimSpace(n.ProxyUser),
				"password":         strings.TrimSpace(n.ProxyPassword),
				"tls":              true,
				"skip-cert-verify": false,
			}
		}
		proxies = append(proxies, proxy)
	}

	sort.Strings(proxyNames)
	return proxies, proxyNames, nil
}

func dedupeName(base string, used map[string]int) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "node"
	}
	if used[base] == 0 {
		used[base] = 1
		return base
	}
	used[base]++
	return fmt.Sprintf("%s-%d", base, used[base])
}

func buildInjectedGroups(proxyNames []string) []interface{} {
	autoProxies := make([]interface{}, 0, len(proxyNames))
	for _, n := range proxyNames {
		autoProxies = append(autoProxies, n)
	}
	if len(autoProxies) == 0 {
		autoProxies = append(autoProxies, "DIRECT")
	}

	masterProxies := []interface{}{autoGroupName}
	for _, n := range proxyNames {
		masterProxies = append(masterProxies, n)
	}
	masterProxies = append(masterProxies, "DIRECT")

	return []interface{}{
		map[string]interface{}{
			"name":    masterGroupName,
			"type":    "select",
			"proxies": masterProxies,
		},
		map[string]interface{}{
			"name":      autoGroupName,
			"type":      "url-test",
			"url":       "http://www.gstatic.com/generate_204",
			"interval":  300,
			"tolerance": 50,
			"proxies":   autoProxies,
		},
	}
}

func readProxyGroups(raw interface{}) []interface{} {
	switch groups := raw.(type) {
	case []interface{}:
		out := make([]interface{}, 0, len(groups))
		for _, g := range groups {
			out = append(out, g)
		}
		return out
	default:
		return []interface{}{}
	}
}

func removeInjectedGroups(groups []interface{}) []interface{} {
	out := make([]interface{}, 0, len(groups))
	for _, item := range groups {
		m, ok := item.(map[string]interface{})
		if !ok {
			out = append(out, item)
			continue
		}
		name, _ := m["name"].(string)
		name = strings.TrimSpace(name)
		if name == masterGroupName || name == autoGroupName {
			continue
		}
		out = append(out, item)
	}
	return out
}
