package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/subscription"
	"gopkg.in/yaml.v3"
)

func main() {
	overridesPath := flag.String("overrides", "", "path to overrides YAML")
	outPath := flag.String("out", "/tmp/clashforge-runtime/mihomo-config.yaml", "output path")
	strip := flag.Bool("strip", false, "strip rule-providers, geodata, and complex rules for smoke test")
	flag.Parse()

	var nodes []subscription.ProxyNode
	var overrides []byte

	if *overridesPath != "" {
		data, err := os.ReadFile(*overridesPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read overrides: %v\n", err)
			os.Exit(1)
		}
		overrides = data
		n, err := subscription.Parse(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse nodes: %v\n", err)
			os.Exit(1)
		}
		nodes = n
		fmt.Fprintf(os.Stderr, "parsed %d nodes from overrides\n", len(nodes))
	}

	cfg := config.Default()
	generated, err := config.Generate(cfg, nodes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate: %v\n", err)
		os.Exit(1)
	}

	merged, err := config.MergeWithOverrides(generated, overrides)
	if err != nil {
		fmt.Fprintf(os.Stderr, "merge: %v\n", err)
		os.Exit(1)
	}

	if *strip {
		// Remove heavy features that require internet download during -t validation
		delete(merged, "rule-providers")
		delete(merged, "geodata-mode")
		delete(merged, "geox-url")

		// Collect actual proxy node names from the proxies list
		// After yaml.Marshal/Unmarshal the type is []interface{} not []map[string]interface{}
		var nodeNames []string
		switch px := merged["proxies"].(type) {
		case []map[string]interface{}:
			for _, p := range px {
				if name, ok := p["name"].(string); ok {
					nodeNames = append(nodeNames, name)
				}
			}
		case []interface{}:
			for _, item := range px {
				if m, ok := item.(map[string]interface{}); ok {
					if name, ok := m["name"].(string); ok {
						nodeNames = append(nodeNames, name)
					}
				}
			}
		}
		if len(nodeNames) == 0 {
			nodeNames = []string{"DIRECT"}
		}

		// Replace proxy-groups with clean minimal set referencing only real nodes
		// Put wujun-sg first so Proxy group defaults to it on start
		selectProxies := append(toIface(nodeNames), []interface{}{"Auto", "DIRECT"}...)
		merged["proxy-groups"] = []interface{}{
			map[string]interface{}{
				"name":    "Proxy",
				"type":    "select",
				"proxies": selectProxies,
			},
			map[string]interface{}{
				"name":      "Auto",
				"type":      "url-test",
				"url":       "http://www.gstatic.com/generate_204",
				"interval":  300,
				"tolerance": 50,
				"proxies":   toIface(nodeNames),
			},
		}

		// Simple rules that don't reference GEOIP/GEOSITE/RULE-SET
		merged["rules"] = []interface{}{
			"IP-CIDR,127.0.0.0/8,DIRECT,no-resolve",
			"IP-CIDR,192.168.0.0/16,DIRECT,no-resolve",
			"IP-CIDR,10.0.0.0/8,DIRECT,no-resolve",
			"MATCH,Proxy",
		}

		// Disable DNS — avoids fake-ip issues outside OpenWrt
		merged["dns"] = map[string]interface{}{"enable": false}

		fmt.Fprintln(os.Stderr, "stripped: rule-providers/geodata removed, proxy-groups rebuilt from node list")
	}

	data, err := yaml.Marshal(merged)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(*outPath, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("config written: %s (%d bytes)\n", *outPath, len(data))
}

func toIface(ss []string) []interface{} {
	out := make([]interface{}, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
