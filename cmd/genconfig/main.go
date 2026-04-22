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
		// Remove heavy features for smoke test on this server
		delete(merged, "rule-providers")
		delete(merged, "geodata-mode")
		delete(merged, "geox-url")
		// Replace rules with simple ones
		merged["rules"] = []string{
			"IP-CIDR,127.0.0.0/8,DIRECT,no-resolve",
			"IP-CIDR,192.168.0.0/16,DIRECT,no-resolve",
			"IP-CIDR,10.0.0.0/8,DIRECT,no-resolve",
			"MATCH,🚀 节点选择",
		}
		// Simplify proxy groups - remove references to non-existent rule-set groups
		if groups, ok := merged["proxy-groups"].([]interface{}); ok {
			var simple []interface{}
			for _, g := range groups {
				m, _ := g.(map[string]interface{})
				if m == nil { continue }
				name, _ := m["name"].(string)
				// Keep only the main groups
				if name == "🚀 节点选择" || name == "♻️ 自动选择" || name == "🚀 手动切换" ||
					name == "🌏 新加坡" || name == "🌏 日本" {
					simple = append(simple, g)
				}
			}
			if len(simple) > 0 {
				merged["proxy-groups"] = simple
			}
		}
		// Disable DNS to avoid fake-ip issues on server
		merged["dns"] = map[string]interface{}{"enable": false}
		fmt.Fprintln(os.Stderr, "stripped: removed rule-providers, simplified rules and dns")
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
