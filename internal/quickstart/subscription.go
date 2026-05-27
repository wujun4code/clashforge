package quickstart

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// socks5Proxy is the Clash YAML proxy entry for a gost SOCKS5+TLS VPS node.
type socks5Proxy struct {
	Name           string `yaml:"name"`
	Type           string `yaml:"type"`
	Server         string `yaml:"server"`
	Port           int    `yaml:"port"`
	TLS            bool   `yaml:"tls"`
	SkipCertVerify bool   `yaml:"skip-cert-verify"`
	UDP            bool   `yaml:"udp"`
}

// BuildSocks5ClashYAML generates a Clash YAML subscription for a gost SOCKS5+TLS node.
func BuildSocks5ClashYAML(name, host string, port int) (string, error) {
	if name == "" || host == "" {
		return "", fmt.Errorf("node name and host are required")
	}
	if port == 0 {
		port = 443
	}
	proxy := socks5Proxy{
		Name:           name,
		Type:           "socks5",
		Server:         host,
		Port:           port,
		TLS:            true,
		SkipCertVerify: false,
		UDP:            false,
	}
	type wrapper struct {
		Proxies []socks5Proxy `yaml:"proxies"`
	}
	data, err := yaml.Marshal(wrapper{Proxies: []socks5Proxy{proxy}})
	if err != nil {
		return "", fmt.Errorf("marshal socks5 clash yaml: %w", err)
	}
	return string(data), nil
}
