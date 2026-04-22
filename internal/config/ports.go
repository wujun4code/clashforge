package config

import (
	"fmt"
	"net"
)

type PortAdjustment struct {
	Name string `json:"name"`
	From int    `json:"from"`
	To   int    `json:"to"`
}

type PortSelectionOptions struct {
	PreferCommunityDefaults bool
	IgnoreOccupiedPorts     map[int]bool
	OccupiedChecker         func(port int, checkUDP bool) bool
}

// NormalizeLegacyCoexistPorts keeps legacy default ports when available,
// and falls back to coexistence ports only when conflicts are detected.
func NormalizeLegacyCoexistPorts(cfg *MetaclashConfig) []PortAdjustment {
	return SelectCompatiblePorts(cfg, PortSelectionOptions{})
}

// SelectCompatiblePorts selects Clash community default ports when available,
// otherwise falls back to ClashForge coexist ports.
func SelectCompatiblePorts(cfg *MetaclashConfig, options PortSelectionOptions) []PortAdjustment {
	if cfg == nil {
		return nil
	}

	type rule struct {
		name      string
		current   *int
		community int
		coexist   int
		checkUDP  bool
	}

	rules := []rule{
		{name: "http", current: &cfg.Ports.HTTP, community: 7890, coexist: 17890},
		{name: "socks", current: &cfg.Ports.SOCKS, community: 7891, coexist: 17891},
		{name: "mixed", current: &cfg.Ports.Mixed, community: 7893, coexist: 17893},
		{name: "redir", current: &cfg.Ports.Redir, community: 7892, coexist: 17892},
		{name: "tproxy", current: &cfg.Ports.TProxy, community: 7895, coexist: 17895},
		{name: "dns", current: &cfg.Ports.DNS, community: 7874, coexist: 17874, checkUDP: true},
		{name: "mihomo_api", current: &cfg.Ports.MihomoAPI, community: 9090, coexist: 19090},
	}

	adjustments := make([]PortAdjustment, 0, 7)
	for _, rule := range rules {
		if rule.current == nil {
			continue
		}
		communityOccupied := isPortOccupied(rule.community, rule.checkUDP, options)
		coexistOccupied := isPortOccupied(rule.coexist, rule.checkUDP, options)

		switch *rule.current {
		case rule.community:
			if communityOccupied && !coexistOccupied {
				adjustments = append(adjustments, PortAdjustment{Name: rule.name, From: rule.community, To: rule.coexist})
				*rule.current = rule.coexist
			}
		case rule.coexist:
			if options.PreferCommunityDefaults && !communityOccupied {
				adjustments = append(adjustments, PortAdjustment{Name: rule.name, From: rule.coexist, To: rule.community})
				*rule.current = rule.community
			} else if coexistOccupied && !communityOccupied {
				adjustments = append(adjustments, PortAdjustment{Name: rule.name, From: rule.coexist, To: rule.community})
				*rule.current = rule.community
			}
		}
	}

	return adjustments
}

func isPortOccupied(port int, checkUDP bool, options PortSelectionOptions) bool {
	if options.IgnoreOccupiedPorts != nil && options.IgnoreOccupiedPorts[port] {
		return false
	}
	if options.OccupiedChecker != nil {
		return options.OccupiedChecker(port, checkUDP)
	}
	if !canListen("tcp", port) {
		return true
	}
	if checkUDP && !canListen("udp", port) {
		return true
	}
	return false
}

func canListen(network string, port int) bool {
	address := fmt.Sprintf(":%d", port)
	if network == "udp" {
		conn, err := net.ListenPacket(network, address)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}
	listener, err := net.Listen(network, address)
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}
