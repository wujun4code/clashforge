package nodes

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ClashProxy is a single proxy entry in Clash format.
type ClashProxy struct {
	Name           string `yaml:"name"`
	Type           string `yaml:"type"`
	Server         string `yaml:"server"`
	Port           int    `yaml:"port"`
	Username       string `yaml:"username"`
	Password       string `yaml:"password"`
	TLS            bool   `yaml:"tls,omitempty"`
	SkipCertVerify bool   `yaml:"skip-cert-verify,omitempty"`
}

// ExportClashProxy generates a Clash proxy YAML for the node.
func ExportClashProxy(node *Node) (string, error) {
	if node == nil {
		return "", fmt.Errorf("node is nil")
	}
	if strings.TrimSpace(node.ProxyUser) == "" || strings.TrimSpace(node.ProxyPassword) == "" {
		return "", fmt.Errorf("节点代理账号或密码缺失，请先重新部署节点后再导出配置")
	}
	server := strings.TrimSpace(node.Domain)
	if server == "" {
		server = strings.TrimSpace(node.Host)
	}
	if server == "" {
		return "", fmt.Errorf("节点域名/主机为空，无法导出配置")
	}

	proxy := ClashProxy{
		Name:     node.Name,
		Type:     "http",
		Server:   server,
		Port:     443,
		Username: node.ProxyUser,
		Password: node.ProxyPassword,
		TLS:      true,
	}

	type wrapper struct {
		Proxies []ClashProxy `yaml:"proxies"`
	}

	w := wrapper{Proxies: []ClashProxy{proxy}}
	data, err := yaml.Marshal(w)
	if err != nil {
		return "", fmt.Errorf("marshal yaml: %w", err)
	}
	return string(data), nil
}
