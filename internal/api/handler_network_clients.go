package api

import (
	"bytes"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

type networkClient struct {
	IP        string `json:"ip"`
	MAC       string `json:"mac,omitempty"`
	Hostname  string `json:"hostname,omitempty"`
	Interface string `json:"interface,omitempty"`
	Source    string `json:"source,omitempty"`
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

	merged := map[string]networkClient{}
	merge := func(items []networkClient) {
		for _, item := range items {
			ip := strings.TrimSpace(item.IP)
			if ip == "" {
				continue
			}
			if net.ParseIP(ip) == nil {
				continue
			}

			current := merged[ip]
			if current.IP == "" {
				current.IP = ip
			}
			if current.MAC == "" {
				current.MAC = item.MAC
			}
			if current.Hostname == "" {
				current.Hostname = item.Hostname
			}
			if current.Interface == "" {
				current.Interface = item.Interface
			}
			if current.Source == "" {
				current.Source = item.Source
			}
			merged[ip] = current
		}
	}

	// OpenWrt DHCP leases: best source for hostname + IP.
	if leaseData, err := os.ReadFile("/tmp/dhcp.leases"); err == nil {
		merge(parseDHCPLeases(leaseData))
	}

	// Runtime neighbor table: active LAN devices.
	if out, err := exec.Command("ip", "neigh", "show").Output(); err == nil {
		merge(parseIPNeigh(out))
	}

	// Fallback for systems lacking iproute2.
	if arpData, err := os.ReadFile("/proc/net/arp"); err == nil {
		merge(parseProcARP(arpData))
	}

	result := make([]networkClient, 0, len(merged))
	for _, c := range merged {
		if strings.TrimSpace(c.IP) == "" {
			continue
		}
		// Skip devices in the WAN subnet — they are upstream of this router.
		if isWANIP(c.IP) {
			continue
		}
		result = append(result, c)
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
