package api

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

type networkClient struct {
	IP        string   `json:"ip"`
	MAC       string   `json:"mac,omitempty"`
	Hostname  string   `json:"hostname,omitempty"`
	IPs       []string `json:"ips,omitempty"` // all addresses: IPv4 first, then IPv6
	Interface string   `json:"interface,omitempty"`
	Source    string   `json:"source,omitempty"`
}

func handleGetNetworkClients(_ Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, map[string]interface{}{
			"clients": discoverNetworkClients(),
		})
	}
}

// getWANSubnets returns the IP subnets assigned to WAN interfaces (those carrying the default route).
// Devices in these subnets are upstream of the router and cannot be managed by it.
func getWANSubnets() []*net.IPNet {
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return nil
	}
	wanIfaces := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		for i, f := range fields {
			if f == "dev" && i+1 < len(fields) {
				wanIfaces[fields[i+1]] = true
			}
		}
	}
	if len(wanIfaces) == 0 {
		return nil
	}

	addrOut, err := exec.Command("ip", "addr", "show").Output()
	if err != nil {
		return nil
	}

	var subnets []*net.IPNet
	var currentIface string
	for _, line := range strings.Split(string(addrOut), "\n") {
		trimmed := strings.TrimSpace(line)
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 {
				currentIface = strings.TrimSuffix(fields[1], ":")
			}
		} else if wanIfaces[currentIface] && strings.HasPrefix(trimmed, "inet ") {
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 {
				_, ipNet, err := net.ParseCIDR(fields[1])
				if err == nil {
					subnets = append(subnets, ipNet)
				}
			}
		}
	}
	return subnets
}

// mergedDevice accumulates all addresses and metadata for one physical device.
type mergedDevice struct {
	mac      string
	hostname string
	iface    string
	source   string
	v4s      []string // IPv4 addresses, deduped
	v6s      []string // IPv6 addresses, deduped
}

func (d *mergedDevice) addIP(rawIP string) {
	ip := net.ParseIP(rawIP)
	if ip == nil {
		return
	}
	if ip.To4() != nil {
		for _, x := range d.v4s {
			if x == rawIP {
				return
			}
		}
		d.v4s = append(d.v4s, rawIP)
	} else {
		for _, x := range d.v6s {
			if x == rawIP {
				return
			}
		}
		d.v6s = append(d.v6s, rawIP)
	}
}

func (d *mergedDevice) absorb(item networkClient) {
	d.addIP(item.IP)
	if d.mac == "" && item.MAC != "" {
		d.mac = item.MAC
	}
	if d.hostname == "" && item.Hostname != "" {
		d.hostname = item.Hostname
	}
	if d.iface == "" && item.Interface != "" {
		d.iface = item.Interface
	}
	if d.source == "" && item.Source != "" {
		d.source = item.Source
	}
}

func discoverNetworkClients() []networkClient {
	wanSubnets := getWANSubnets()
	isWANIP := func(ipStr string) bool {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			return false
		}
		for _, subnet := range wanSubnets {
			if subnet.Contains(ip) {
				return true
			}
		}
		return false
	}

	// byMAC groups all addresses for one physical device by its MAC address.
	// ipToMAC is a reverse index so MAC-less neighbor entries can be attached to
	// an existing device once its MAC is known from a later source.
	byMAC   := map[string]*mergedDevice{}
	ipToMAC := map[string]string{}
	// noMAC holds entries whose MAC address is not yet known.
	noMAC := map[string]*mergedDevice{}

	ingest := func(items []networkClient) {
		for _, item := range items {
			rawIP := strings.TrimSpace(item.IP)
			if net.ParseIP(rawIP) == nil {
				continue
			}

			if item.MAC != "" {
				// Find or create the MAC-keyed device.
				dev := byMAC[item.MAC]
				if dev == nil {
					dev = &mergedDevice{mac: item.MAC}
					byMAC[item.MAC] = dev
				}
				dev.absorb(item)
				ipToMAC[rawIP] = item.MAC

				// Absorb any previously MAC-less entry for the same IP.
				if prev, ok := noMAC[rawIP]; ok {
					dev.absorb(networkClient{
						IP:        rawIP,
						Hostname:  prev.hostname,
						Interface: prev.iface,
					})
					delete(noMAC, rawIP)
				}
			} else if mac, ok := ipToMAC[rawIP]; ok {
				// This IP is already owned by a known MAC device.
				byMAC[mac].absorb(item)
			} else {
				// No MAC known yet — store under IP for later merging.
				dev := noMAC[rawIP]
				if dev == nil {
					dev = &mergedDevice{}
					noMAC[rawIP] = dev
				}
				dev.absorb(item)
			}
		}
	}

	// Sources in priority order: DHCP gives the best hostname + IPv4 mapping.
	// Both the lease file and ubus are tried because some OpenWrt variants store
	// leases at a non-standard path while ubus works across all firmware forks.
	// The MAC-merge logic deduplicates entries that appear in both.
	if leaseData, err := os.ReadFile("/tmp/dhcp.leases"); err == nil {
		ingest(parseDHCPLeases(leaseData))
	}
	if out, err := exec.Command("ubus", "call", "dhcp", "ipv4leases").Output(); err == nil {
		ingest(parseUbusIPv4Leases(out))
	}
	if out, err := exec.Command("ip", "neigh", "show").Output(); err == nil {
		ingest(parseIPNeigh(out))
	}
	if arpData, err := os.ReadFile("/proc/net/arp"); err == nil {
		ingest(parseProcARP(arpData))
	}

	toClient := func(dev *mergedDevice) (networkClient, bool) {
		// Primary address: first IPv4 (most readable), or first IPv6 as fallback.
		var primary string
		if len(dev.v4s) > 0 {
			primary = dev.v4s[0]
		} else if len(dev.v6s) > 0 {
			primary = dev.v6s[0]
		}
		if primary == "" || isWANIP(primary) {
			return networkClient{}, false
		}
		allIPs := append(append([]string{}, dev.v4s...), dev.v6s...)
		c := networkClient{
			IP:        primary,
			MAC:       dev.mac,
			Hostname:  dev.hostname,
			Interface: dev.iface,
			Source:    dev.source,
		}
		if len(allIPs) > 1 {
			c.IPs = allIPs
		}
		return c, true
	}

	var result []networkClient
	for _, dev := range byMAC {
		if c, ok := toClient(dev); ok {
			result = append(result, c)
		}
	}
	for _, dev := range noMAC {
		if c, ok := toClient(dev); ok {
			result = append(result, c)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		ip1 := net.ParseIP(result[i].IP)
		ip2 := net.ParseIP(result[j].IP)
		v41 := ip1 != nil && ip1.To4() != nil
		v42 := ip2 != nil && ip2.To4() != nil
		if v41 && v42 {
			b1 := ip1.To4()
			b2 := ip2.To4()
			for idx := 0; idx < 4; idx++ {
				if b1[idx] == b2[idx] {
					continue
				}
				return b1[idx] < b2[idx]
			}
		}
		h1 := strings.ToLower(strings.TrimSpace(result[i].Hostname))
		h2 := strings.ToLower(strings.TrimSpace(result[j].Hostname))
		if h1 != h2 {
			if h1 == "" {
				return false
			}
			if h2 == "" {
				return true
			}
			return h1 < h2
		}
		return result[i].IP < result[j].IP
	})

	return result
}

func parseDHCPLeases(data []byte) []networkClient {
	lines := bytes.Split(data, []byte{'\n'})
	out := make([]networkClient, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(string(line)))
		// OpenWrt format: expires mac ip hostname clientid
		if len(fields) < 4 {
			continue
		}
		ip := fields[2]
		if net.ParseIP(ip) == nil {
			continue
		}
		hostname := fields[3]
		if hostname == "*" {
			hostname = ""
		}
		out = append(out, networkClient{
			IP:       ip,
			MAC:      normalizeMAC(fields[1]),
			Hostname: hostname,
			Source:   "dhcp",
		})
	}
	return out
}

// activeNeighStates are NUD states treated as usable client hints.
// STALE is included because many OpenWrt routers keep active LAN devices in STALE
// between traffic bursts; we only exclude clearly incomplete/failed entries.
var activeNeighStates = map[string]bool{
	"REACHABLE": true,
	"STALE":     true,
	"DELAY":     true,
	"PROBE":     true,
	"PERMANENT": true,
}

func parseIPNeigh(data []byte) []networkClient {
	lines := bytes.Split(data, []byte{'\n'})
	out := make([]networkClient, 0, len(lines))
	for _, line := range lines {
		text := strings.TrimSpace(string(line))
		if text == "" {
			continue
		}
		fields := strings.Fields(text)
		// Format: <ip> dev <iface> [lladdr <mac>] <STATE>
		if len(fields) < 4 {
			continue
		}
		ip := fields[0]
		if net.ParseIP(ip) == nil {
			continue
		}
		// State is always the last token; skip anything not in our allowlist.
		if !activeNeighStates[strings.ToUpper(fields[len(fields)-1])] {
			continue
		}

		var dev, mac string
		for i := 1; i < len(fields)-1; i++ {
			if fields[i] == "dev" && i+1 < len(fields) {
				dev = fields[i+1]
			}
			if fields[i] == "lladdr" && i+1 < len(fields) {
				mac = normalizeMAC(fields[i+1])
			}
		}

		out = append(out, networkClient{
			IP:        ip,
			MAC:       mac,
			Interface: dev,
			Source:    "neigh",
		})
	}
	return out
}

func parseProcARP(data []byte) []networkClient {
	lines := bytes.Split(data, []byte{'\n'})
	out := make([]networkClient, 0, len(lines))
	for idx, line := range lines {
		// Header: IP address HW type Flags HW address Mask Device
		if idx == 0 {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(string(line)))
		if len(fields) < 6 {
			continue
		}
		ip := fields[0]
		if net.ParseIP(ip) == nil {
			continue
		}
		// fields[2] is the flags column (hex). ATF_COM (0x02) must be set,
		// meaning the entry is complete — the MAC was confirmed via ARP reply.
		flagVal, err := strconv.ParseUint(strings.TrimPrefix(fields[2], "0x"), 16, 32)
		if err != nil || flagVal&0x02 == 0 {
			continue
		}
		mac := normalizeMAC(fields[3])
		if mac == "" {
			continue
		}
		out = append(out, networkClient{
			IP:        ip,
			MAC:       mac,
			Interface: fields[5],
			Source:    "arp",
		})
	}
	return out
}

func normalizeMAC(mac string) string {
	mac = strings.TrimSpace(strings.ToLower(mac))
	if mac == "" || mac == "00:00:00:00:00:00" {
		return ""
	}
	if _, err := net.ParseMAC(mac); err != nil {
		return ""
	}
	return mac
}

// parseUbusIPv4Leases parses the JSON output of `ubus call dhcp ipv4leases`.
// This works on all OpenWrt firmware variants regardless of DHCP backend or
// lease file path, so it is used as both a complement and a fallback to the
// dnsmasq /tmp/dhcp.leases file.
func parseUbusIPv4Leases(data []byte) []networkClient {
	var resp struct {
		Device map[string]struct {
			Leases []struct {
				MAC      string `json:"mac"`
				IP       string `json:"ip"`
				Hostname string `json:"hostname"`
			} `json:"leases"`
		} `json:"device"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil
	}
	var out []networkClient
	for iface, dev := range resp.Device {
		for _, lease := range dev.Leases {
			if net.ParseIP(lease.IP) == nil {
				continue
			}
			hostname := lease.Hostname
			if hostname == "*" {
				hostname = ""
			}
			out = append(out, networkClient{
				IP:        lease.IP,
				MAC:       normalizeMAC(lease.MAC),
				Hostname:  hostname,
				Interface: iface,
				Source:    "dhcp",
			})
		}
	}
	return out
}
