package subscription

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

func parseClashYAML(content []byte) ([]ProxyNode, error) {
	var raw struct {
		Proxies []map[string]interface{} `yaml:"proxies"`
	}
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return nil, fmt.Errorf("clash yaml parse: %w", err)
	}
	var nodes []ProxyNode
	for _, p := range raw.Proxies {
		name, _ := p["name"].(string)
		typ, _ := p["type"].(string)
		server, _ := p["server"].(string)
		port := toIntVal(p["port"], 0)
		if name == "" || typ == "" || server == "" || port == 0 {
			continue
		}
		extra := make(map[string]interface{})
		for k, v := range p {
			if k != "name" && k != "type" && k != "server" && k != "port" {
				extra[k] = v
			}
		}
		nodes = append(nodes, ProxyNode{
			Name:   name,
			Type:   typ,
			Server: server,
			Port:   port,
			Extra:  extra,
		})
	}
	return nodes, nil
}
