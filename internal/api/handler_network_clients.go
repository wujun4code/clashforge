package api

import (
	"bytes"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
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

func discoverNetworkClients() []networkClient {
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

func parseIPNeigh(data []byte) []networkClient {
	lines := bytes.Split(data, []byte{'\n'})
	out := make([]networkClient, 0, len(lines))
	for _, line := range lines {
		text := strings.TrimSpace(string(line))
		if text == "" {
			continue
		}
		fields := strings.Fields(text)
		// Examples:
		// 192.168.1.10 dev br-lan lladdr aa:bb:cc:dd:ee:ff REACHABLE
		// 192.168.1.9 dev br-lan INCOMPLETE
		if len(fields) < 4 {
			continue
		}
		ip := fields[0]
		if net.ParseIP(ip) == nil {
			continue
		}
		if strings.Contains(strings.ToUpper(text), "FAILED") || strings.Contains(strings.ToUpper(text), "INCOMPLETE") {
			continue
		}

		var dev string
		var mac string
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
