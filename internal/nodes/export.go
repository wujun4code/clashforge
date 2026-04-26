package nodes

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ClashProxy is a single proxy entry in Clash format.
type ClashProxy struct {
	Name           string `yaml:"name"`
	Type           string `yaml:"type"`
	Server         string `yaml:"server"`
	Port           int    `yaml:"port"`
	Username       string `yaml:"username,omitempty"`
	Password       string `yaml:"password,omitempty"`
	TLS            bool   `yaml:"tls,omitempty"`
	SkipCertVerify bool   `yaml:"skip-cert-verify,omitempty"`
}

// ExportClashProxy generates a Clash proxy YAML for the node.
func ExportClashProxy(node *Node) (string, error) {
	proxy := ClashProxy{
		Name:     node.Name,
		Type:     "http",
		Server:   node.Domain,
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
