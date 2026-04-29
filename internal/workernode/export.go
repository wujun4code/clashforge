package workernode

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ClashProxy is a single Clash proxy entry for a Worker node.
type ClashProxy struct {
	Name    string `yaml:"name"`
	Type    string `yaml:"type"`
	Server  string `yaml:"server"`
	Port    int    `yaml:"port"`
	UUID    string `yaml:"uuid"`
	TLS     bool   `yaml:"tls,omitempty"`
	Network string `yaml:"network,omitempty"`
	WSOpts  WSOpts `yaml:"ws-opts,omitempty"`
	UDP     bool   `yaml:"udp"`
}

// WSOpts describes websocket options for Clash proxy config.
type WSOpts struct {
	Path    string            `yaml:"path,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
}

// ExportClashProxy exports a complete Clash YAML document with "proxies:" root,
// matching SSH-node export behavior.
func ExportClashProxy(node *WorkerNode) (string, error) {
	if node == nil {
		return "", fmt.Errorf("worker node is nil")
	}

	server := strings.TrimSpace(node.Hostname)
	if server == "" {
		return "", fmt.Errorf("worker 节点域名为空，无法导出配置")
	}
	uuid := strings.TrimSpace(node.WorkerUUID)
	if uuid == "" {
		return "", fmt.Errorf("worker 节点 UUID 缺失，请重新部署后再导出配置")
	}

	proxy := ClashProxy{
		Name:    node.Name,
		Type:    "vless",
		Server:  server,
		Port:    443,
		UUID:    uuid,
		TLS:     true,
		Network: "ws",
		WSOpts: WSOpts{
			Path: "/",
			Headers: map[string]string{
				"Host": server,
			},
		},
		UDP: false,
	}

	type wrapper struct {
		Proxies []ClashProxy `yaml:"proxies"`
	}

	data, err := yaml.Marshal(wrapper{Proxies: []ClashProxy{proxy}})
	if err != nil {
		return "", fmt.Errorf("marshal worker clash yaml: %w", err)
	}
	return string(data), nil
}
