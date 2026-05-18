package config

import (
	"bufio"
	"context"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// DetectWANInterface validates the configured WAN interface against the
// current system. If the configured interface does not exist, it auto-detects
// the WAN interface from the kernel routing table or UCI config.
//
// Returns (iface, autoDetected):
//   - autoDetected=false → configured value is valid, returned as-is
//   - autoDetected=true  → configured value was invalid; iface is the detected replacement
func DetectWANInterface(configured string) (string, bool) {
	if ifaceExists(configured) {
		return configured, false
	}

	// Primary: parse /proc/net/route for the default-route interface.
	if iface := defaultRouteIface(); iface != "" {
		return iface, true
	}

	// Fallback: ask UCI (OpenWrt-specific).
	if iface := uciWANIface(); iface != "" {
		return iface, true
	}

	// Give up — return configured value unchanged.
	return configured, false
}

// ifaceExists checks whether a named network interface exists on the system.
func ifaceExists(name string) bool {
	if name == "" {
		return false
	}
	_, err := os.Stat("/sys/class/net/" + name)
	return err == nil
}

// defaultRouteIface reads /proc/net/route and returns the interface that
// carries the default route (Destination=0.0.0.0, RTF_UP|RTF_GATEWAY set).
func defaultRouteIface() string {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Scan() // skip header line
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		// Column layout: Iface Destination Gateway Flags ...
		if fields[1] != "00000000" {
			continue
		}
		flags, err := strconv.ParseUint(fields[3], 16, 32)
		if err != nil {
			continue
		}
		// RTF_UP (0x1) | RTF_GATEWAY (0x2) both must be set.
		if flags&0x0003 == 0x0003 {
			return fields[0]
		}
	}
	return ""
}

// PPPNameservers reads ISP DNS servers from PPP-provided resolv.conf files.
// PPPoE WAN connections receive DNS from the PPP server and pppd writes them
// to well-known paths rather than a DHCP lease, so dhcp://iface won't work.
func PPPNameservers() []string {
	candidates := []string{
		"/tmp/resolv.conf.ppp",
		"/etc/ppp/resolv.conf",
		"/tmp/ppp/resolv.conf",
	}
	for _, path := range candidates {
		if servers := readNameserversFromFile(path); len(servers) > 0 {
			return servers
		}
	}
	return nil
}

func readNameserversFromFile(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var servers []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "nameserver ") {
			continue
		}
		ip := strings.TrimSpace(strings.TrimPrefix(line, "nameserver "))
		if net.ParseIP(ip) != nil {
			servers = append(servers, ip)
		}
	}
	return servers
}

// uciWANIface queries OpenWrt's UCI for the WAN device name.
func uciWANIface() string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, key := range []string{"network.wan.device", "network.wan.ifname"} {
		out, err := exec.CommandContext(ctx, "uci", "get", key).Output()
		if err != nil {
			continue
		}
		iface := strings.TrimSpace(string(out))
		if iface != "" && ifaceExists(iface) {
			return iface
		}
	}
	return ""
}
