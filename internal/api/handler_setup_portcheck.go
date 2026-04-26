package api

import (
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

type portCheckItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Port        int    `json:"port"`
	Required    bool   `json:"required"`
	OK          bool   `json:"ok"`
	LatencyMs   int64  `json:"latency_ms,omitempty"`
	Error       string `json:"error,omitempty"`
}

type portCheckResp struct {
	Checks []portCheckItem `json:"checks"`
}

// handleSetupPortCheck checks each ClashForge-managed port and returns pass/fail per port.
// Called from the Setup UI after launch to give the user explicit per-port verification.
func handleSetupPortCheck(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := deps.Config

		type spec struct {
			name     string
			desc     string
			port     int
			required bool
			mode     string // "tcp" | "dns_udp" | "nft_chain"
		}

		specs := []spec{
			{
				name:     "Mihomo API",
				desc:     fmt.Sprintf("Mihomo 内核 REST API — :%d", cfg.Ports.MihomoAPI),
				port:     cfg.Ports.MihomoAPI,
				required: true,
				mode:     "tcp",
			},
		}

		if cfg.Ports.HTTP > 0 {
			specs = append(specs, spec{
				name: "HTTP 代理",
				desc: fmt.Sprintf("HTTP 代理服务 — :%d", cfg.Ports.HTTP),
				port: cfg.Ports.HTTP,
				mode: "tcp",
			})
		}
		if cfg.Ports.Mixed > 0 {
			specs = append(specs, spec{
				name: "Mixed 代理",
				desc: fmt.Sprintf("HTTP+SOCKS5 混合代理 — :%d", cfg.Ports.Mixed),
				port: cfg.Ports.Mixed,
				mode: "tcp",
			})
		}
		if cfg.Ports.SOCKS > 0 {
			specs = append(specs, spec{
				name: "SOCKS5 代理",
				desc: fmt.Sprintf("SOCKS5 代理服务 — :%d", cfg.Ports.SOCKS),
				port: cfg.Ports.SOCKS,
				mode: "tcp",
			})
		}
		if cfg.Network.Mode == "tproxy" && cfg.Ports.TProxy > 0 {
			specs = append(specs, spec{
				name: "TProxy 透明代理",
				desc: fmt.Sprintf("透明代理入口 — :%d", cfg.Ports.TProxy),
				port: cfg.Ports.TProxy,
				mode: "tcp",
			})
		}
		if cfg.Network.Mode == "redir" && cfg.Ports.Redir > 0 {
			specs = append(specs, spec{
				name: "Redir 透明代理",
				desc: fmt.Sprintf("TCP 透明代理入口 — :%d", cfg.Ports.Redir),
				port: cfg.Ports.Redir,
				mode: "tcp",
			})
		}

		if cfg.DNS.Enable && cfg.Ports.DNS > 0 {
			specs = append(specs, spec{
				name: "Mihomo DNS",
				desc: fmt.Sprintf("Mihomo DNS 解析服务 — :%d (UDP)", cfg.Ports.DNS),
				port: cfg.Ports.DNS,
				mode: "dns_udp",
			})
		}

		// DNS :53 takeover check — strategy depends on dnsmasq_mode
		if cfg.DNS.Enable && cfg.DNS.ApplyOnStart && cfg.DNS.DnsmasqMode != "none" {
			switch cfg.DNS.DnsmasqMode {
			case "replace":
				// nftables redirects LAN :53 → mihomo DNS port.
				// Localhost traffic bypasses the redirect (fib saddr type local return),
				// so we verify the nftables chain directly.
				specs = append(specs, spec{
					name: "DNS :53 接管 (nftables)",
					desc: fmt.Sprintf("nftables dns_redirect 链：:53 → :%d (replace 模式)", cfg.Ports.DNS),
					port: 53,
					mode: "nft_chain",
				})
			case "upstream":
				// dnsmasq still owns :53 but forwards to mihomo.
				// Send a real DNS query to 127.0.0.1:53 to verify dnsmasq is answering.
				specs = append(specs, spec{
					name: "DNS :53 接管 (dnsmasq upstream)",
					desc: fmt.Sprintf("dnsmasq :53 → mihomo :%d (upstream 模式)", cfg.Ports.DNS),
					port: 53,
					mode: "dns_udp",
				})
			}
		}

		results := make([]portCheckItem, 0, len(specs))
		for _, s := range specs {
			item := portCheckItem{
				Name:        s.name,
				Description: s.desc,
				Port:        s.port,
				Required:    s.required,
			}
			start := time.Now()
			switch s.mode {
			case "tcp":
				conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", s.port), 2*time.Second)
				if err != nil {
					item.Error = err.Error()
				} else {
					conn.Close()
					item.OK = true
					item.LatencyMs = time.Since(start).Milliseconds()
				}
			case "dns_udp":
				if err := probeDNSPort(fmt.Sprintf("127.0.0.1:%d", s.port)); err != nil {
					item.Error = err.Error()
				} else {
					item.OK = true
					item.LatencyMs = time.Since(start).Milliseconds()
				}
			case "nft_chain":
				if err := checkNftDNSRedirect(cfg.Ports.DNS); err != nil {
					item.Error = err.Error()
				} else {
					item.OK = true
					item.LatencyMs = time.Since(start).Milliseconds()
				}
			}
			results = append(results, item)
		}

		JSON(w, http.StatusOK, portCheckResp{Checks: results})
	}
}

// probeDNSPort sends a minimal UDP DNS query and waits for any response.
func probeDNSPort(addr string) error {
	conn, err := net.DialTimeout("udp", addr, 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Minimal DNS query: TxID=0x1234, RD set, 1 question: "." A IN
	query := []byte{
		0x12, 0x34, // Transaction ID
		0x01, 0x00, // Flags: standard query, recursion desired
		0x00, 0x01, // QDCOUNT: 1
		0x00, 0x00, // ANCOUNT: 0
		0x00, 0x00, // NSCOUNT: 0
		0x00, 0x00, // ARCOUNT: 0
		0x00,       // QNAME: root label (end of name)
		0x00, 0x01, // QTYPE: A
		0x00, 0x01, // QCLASS: IN
	}

	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write(query); err != nil {
		return err
	}
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		return fmt.Errorf("无响应: %w", err)
	}
	if n < 2 {
		return fmt.Errorf("响应过短 (%d 字节)", n)
	}
	return nil
}

// checkNftDNSRedirect verifies the nftables dns_redirect chain redirects :53 → dnsPort.
func checkNftDNSRedirect(dnsPort int) error {
	out, err := exec.Command("nft", "list", "chain", "inet", "metaclash", "dns_redirect").CombinedOutput()
	if err != nil {
		s := string(out)
		if strings.Contains(s, "No such file") || strings.Contains(s, "table not found") ||
			strings.Contains(s, "does not exist") || strings.Contains(s, "no such chain") {
			return fmt.Errorf("nftables dns_redirect 链不存在，DNS :53 接管可能未生效")
		}
		return fmt.Errorf("nft list chain: %w", err)
	}
	expected := fmt.Sprintf("redirect to :%d", dnsPort)
	if !strings.Contains(string(out), expected) {
		return fmt.Errorf("规则存在但重定向目标不正确 (期望 %s)", expected)
	}
	return nil
}
