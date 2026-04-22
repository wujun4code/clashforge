package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ValidateBinary runs `mihomo -t -f configFile` to check validity.
// If the binary does not exist, validation is skipped (returns nil).
func ValidateBinary(binary, configFile string) error {
	if _, err := os.Stat(binary); os.IsNotExist(err) {
		return nil
	}
	out, err := exec.Command(binary, "-t",
		"-d", filepath.Dir(configFile),
		"-f", configFile,
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("config validation failed: %s", string(out))
	}
	return nil
}

// Validate validates the MetaclashConfig struct fields.
func Validate(cfg *MetaclashConfig) error {
	if strings.TrimSpace(cfg.Core.Binary) == "" {
		return fmt.Errorf("core.binary is required")
	}
	if cfg.Ports.UI <= 0 || cfg.Ports.MihomoAPI <= 0 {
		return fmt.Errorf("invalid port configuration")
	}
	switch cfg.Network.Mode {
	case "tproxy", "redir", "tun", "none":
	default:
		return fmt.Errorf("invalid network.mode: %s", cfg.Network.Mode)
	}
	switch cfg.Network.FirewallBackend {
	case "auto", "nftables", "iptables", "none":
	default:
		return fmt.Errorf("invalid network.firewall_backend: %s", cfg.Network.FirewallBackend)
	}
	return nil
}

// ValidateStruct is an alias for Validate.
func ValidateStruct(cfg *MetaclashConfig) error {
	return Validate(cfg)
}

// ValidateYAML checks that content is valid YAML.
func ValidateYAML(content []byte) error {
	var v interface{}
	return yaml.Unmarshal(content, &v)
}
