package netfilter

import (
	"fmt"
	"os/exec"
	"strings"
	"text/template"
	"bytes"
)

const fwMark = "0x1a3"
const routeTable = "100"

// NftablesBackend manages nftables TProxy rules.
type NftablesBackend struct {
	TProxyPort int
	DNSPort    int
	BypassCIDR []string
}

var nftTableTemplate = template.Must(template.New("nft").Parse(`
table inet metaclash {
    set bypass_ipv4 {
        type ipv4_addr
        flags interval
        elements = {
            0.0.0.0/8,
            10.0.0.0/8,
            100.64.0.0/10,
            127.0.0.0/8,
            169.254.0.0/16,
            172.16.0.0/12,
            192.0.0.0/24,
            192.168.0.0/16,
            198.18.0.0/15,
            198.51.100.0/24,
            203.0.113.0/24,
            224.0.0.0/4,
            240.0.0.0/4,
            255.255.255.255/32{{ range .BypassCIDR }},
            {{ . }}{{ end }}
        }
    }

    chain dns_redirect {
        type nat hook prerouting priority dstnat; policy accept;
        meta mark {{ .FWMark }} return
        fib saddr type local return
        ip daddr @bypass_ipv4 return
        udp dport 53 redirect to :{{ .DNSPort }}
        tcp dport 53 redirect to :{{ .DNSPort }}
    }

    chain tproxy_prerouting {
        type filter hook prerouting priority mangle; policy accept;
        meta mark {{ .FWMark }} return
        fib saddr type local return
        ip daddr @bypass_ipv4 return
        meta l4proto { tcp, udp } tproxy ip to 127.0.0.1:{{ .TProxyPort }} meta mark set {{ .FWMark }}
    }
}
`))

// Apply writes and applies the nftables ruleset.
func (n *NftablesBackend) Apply() error {
	vars := struct {
		FWMark     string
		TProxyPort int
		DNSPort    int
		BypassCIDR []string
	}{
		FWMark:     fwMark,
		TProxyPort: n.TProxyPort,
		DNSPort:    n.DNSPort,
		BypassCIDR: n.BypassCIDR,
	}
	var buf bytes.Buffer
	if err := nftTableTemplate.Execute(&buf, vars); err != nil {
		return fmt.Errorf("render nft template: %w", err)
	}

	// First cleanup any existing rules
	_ = n.Cleanup()

	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = &buf
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nft apply: %w: %s", err, string(out))
	}

	// Setup policy routing
	_ = exec.Command("ip", "rule", "add", "fwmark", fwMark, "table", routeTable).Run()
	_ = exec.Command("ip", "route", "add", "local", "default", "dev", "lo", "table", routeTable).Run()
	return nil
}

// Cleanup removes all metaclash nftables rules.
func (n *NftablesBackend) Cleanup() error {
	out, err := exec.Command("nft", "delete", "table", "inet", "metaclash").CombinedOutput()
	if err != nil {
		s := string(out)
		if strings.Contains(s, "No such file") || strings.Contains(s, "table not found") || strings.Contains(s, "does not exist") {
			return nil
		}
		return fmt.Errorf("nft cleanup: %w: %s", err, s)
	}
	_ = exec.Command("ip", "rule", "del", "fwmark", fwMark, "table", routeTable).Run()
	_ = exec.Command("ip", "route", "flush", "table", routeTable).Run()
	return nil
}
