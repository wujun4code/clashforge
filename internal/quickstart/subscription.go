package quickstart

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// httpsProxy is the Clash YAML proxy entry for a gost HTTP CONNECT+TLS VPS node.
type httpsProxy struct {
	Name           string `yaml:"name"`
	Type           string `yaml:"type"`
	Server         string `yaml:"server"`
	Port           int    `yaml:"port"`
	TLS            bool   `yaml:"tls"`
	SkipCertVerify bool   `yaml:"skip-cert-verify"`
}

// BuildHTTPTLSClashYAML generates a minimal Clash YAML for a gost HTTP CONNECT+TLS node.
// For full loyalsoldier rule sets, use publish.MergeTemplateWithNodes instead.
func BuildHTTPTLSClashYAML(name, host string, port int) (string, error) {
	if name == "" || host == "" {
		return "", fmt.Errorf("node name and host are required")
	}
	if port == 0 {
		port = 443
	}
	proxy := httpsProxy{
		Name:           name,
		Type:           "http",
		Server:         host,
		Port:           port,
		TLS:            true,
		SkipCertVerify: false,
	}
	type wrapper struct {
		Proxies []httpsProxy `yaml:"proxies"`
	}
	data, err := yaml.Marshal(wrapper{Proxies: []httpsProxy{proxy}})
	if err != nil {
		return "", fmt.Errorf("marshal http-tls clash yaml: %w", err)
	}
	return string(data), nil
}
