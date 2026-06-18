package cfgen

import (
	"net"
	"os"
	"path/filepath"
)

// fakeIPFilterBase is the baseline fake-ip-filter applied in Android TUN mode.
// geosite:cn is prepended when GeoDataDir/geosite.dat is present.
var fakeIPFilterBase = []string{
	"*.lan", "*.local", "*.localhost", "*.localdomain",
	"+.stun.*.*", "+.stun.*.*.*",
	"msftconnecttest.com", "*.msftconnecttest.com",
	"time.*.com", "ntp.*.com", "*.pool.ntp.org",
}

// defaultDNSHijack intercepts all DNS traffic entering the TUN device.
var defaultDNSHijack = []interface{}{
	"any:53",
	"tcp://any:53",
	"tls://any:853",
}

// knownFakeNets lists CIDR ranges that indicate GFW DNS poisoning or
// the mihomo fake-ip pool itself. Used in the upstream DNS hijack probe.
var knownFakeNets []*net.IPNet

// knownFakeIPs lists well-documented GFW DNS poison IPs not covered by CIDR ranges.
var knownFakeIPs = map[string]bool{
	"4.36.66.178":    true,
	"8.7.198.45":     true,
	"37.61.54.158":   true,
	"46.82.174.68":   true,
	"59.24.3.173":    true,
	"64.33.88.161":   true,
	"64.33.99.47":    true,
	"64.66.163.251":  true,
	"65.104.202.252": true,
	"65.160.219.113": true,
	"66.45.252.237":  true,
	"72.14.205.99":   true,
	"72.14.205.104":  true,
	"78.16.49.15":    true,
	"93.46.8.89":     true,
	"108.160.166.92": true,
	"113.11.194.190": true,
	"159.106.121.75": true,
	"169.132.13.103": true,
	"192.67.198.6":   true,
	"202.106.1.2":    true,
	"202.181.7.85":   true,
	"211.94.66.147":  true,
	"213.169.251.35": true,
}

func init() {
	for _, cidr := range []string{
		"198.18.0.0/15", // mihomo/clash default fake-ip range
		"28.0.0.0/8",    // historically used by GFW
		"1.2.4.0/22",    // IANA unallocated; used in GFW DNS poisoning
	} {
		_, n, err := net.ParseCIDR(cidr)
		if err == nil && n != nil {
			knownFakeNets = append(knownFakeNets, n)
		}
	}
}

// IsKnownFakeIP reports whether ip is a known GFW-poison or mihomo fake-ip.
func IsKnownFakeIP(ip string) bool {
	if knownFakeIPs[ip] {
		return true
	}
	addr := net.ParseIP(ip)
	if addr == nil {
		return false
	}
	for _, n := range knownFakeNets {
		if n.Contains(addr) {
			return true
		}
	}
	return false
}

// GeoDataAvailable reports whether geosite.dat is present in dir.
func GeoDataAvailable(dir string) bool {
	candidates := []string{
		filepath.Join(dir, "geosite.dat"),
		filepath.Join(dir, "GeoSite.dat"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

func fakeIPFilter(geoDataDir string) []interface{} {
	filter := make([]interface{}, 0, len(fakeIPFilterBase)+1)
	if geoDataDir != "" && GeoDataAvailable(geoDataDir) {
		filter = append(filter, "geosite:cn")
	}
	for _, f := range fakeIPFilterBase {
		filter = append(filter, f)
	}
	return filter
}
