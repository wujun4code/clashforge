// Command repair-config-preview renders a candidate offline using production
// generation/device-rule functions. It never talks to a router or starts Mihomo.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wujun4code/clashforge/internal/config"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func read(path string) []byte { b, err := os.ReadFile(path); must(err); return b }

func main() {
	conf := flag.String("config", "", "private router TOML input")
	sub := flag.String("subscription", "", "private subscription YAML input")
	overrides := flag.String("overrides", "", "existing private overrides YAML input")
	overlay := flag.String("overlay", "", "optional public merge fragment")
	device := flag.String("devices", "", "private source-scoped device state JSON")
	active := flag.String("active-source", "", "private active source JSON")
	output := flag.String("out", "", "private local output directory (must not exist)")
	flag.Parse()
	if *conf == "" || *sub == "" || *device == "" || *active == "" || *output == "" {
		panic("all required input flags must be set")
	}
	abs, err := filepath.Abs(*output)
	must(err)
	if _, err := os.Stat(abs); !os.IsNotExist(err) {
		panic("output must be a new directory")
	}
	must(os.MkdirAll(abs, 0700))
	cfg, err := config.Load(*conf)
	must(err)
	cfg.Core.DataDir = abs
	cfg.Core.RuntimeDir = abs
	base, err := config.GenerateFromBase(cfg, read(*sub), nil)
	must(err)
	for _, path := range []string{*overrides, *overlay} {
		if path != "" {
			base, err = config.MergeWithOverrides(base, read(path))
			must(err)
		}
	}
	base = config.ApplyManagedRuntimeSettings(cfg, base)
	must(config.ValidateRuntimeDNS(cfg, base))
	var source struct {
		Type     string `json:"type"`
		SubID    string `json:"sub_id"`
		Filename string `json:"filename"`
	}
	must(json.Unmarshal(read(*active), &source))
	var key string
	switch strings.ToLower(source.Type) {
	case "subscription":
		key = "subscription:" + strings.TrimSpace(source.SubID)
	case "file":
		key = "file:" + strings.TrimSpace(source.Filename)
	default:
		panic("unsupported active source")
	}
	groups, err := config.LoadDeviceGroupsForSource(*device, key)
	must(err)
	generated, specs := config.ApplyPerDeviceSubRulesWithProviders(base, groups)
	must(config.SyncDeviceRuleProviderFiles(abs, specs))
	data, err := config.MarshalYAML(generated)
	must(err)
	must(os.WriteFile(filepath.Join(abs, "mihomo-config.private.yaml"), data, 0600))
	// Managed files contain LAN source CIDRs; keep every generated file private.
	must(filepath.Walk(abs, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return os.Chmod(path, 0700)
		}
		return os.Chmod(path, 0600)
	}))
	rules, _ := generated["rules"].([]interface{})
	bound := 0
	for _, r := range rules {
		s, _ := r.(string)
		if strings.HasPrefix(s, "AND,") && strings.Contains(s, " - 🤖 Claude") {
			bound++
		}
	}
	summary := map[string]interface{}{"rules": len(rules), "device_groups": len(groups), "managed_device_providers": len(specs), "device_scoped_claude_rules": bound, "router_modified": false}
	b, err := json.Marshal(summary)
	must(err)
	fmt.Println(string(b))
}
