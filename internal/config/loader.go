package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

func Load(path string) (*MetaclashConfig, error) {
	cfg := Default()
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("stat config: %w", err)
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	if err := Validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

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
