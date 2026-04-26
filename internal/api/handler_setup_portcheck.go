package api

import (
	"fmt"
	"net"
	"net/http"
	"os"
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
func handleSetupPortCheck(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := deps.Config

		type spec struct {
			name     string
			desc     string
			port     int
			required bool
			// "tcp_listen"     — /proc/net/tcp LISTEN state only (proxy/TProxy ports: no direct connect)
			// "tcp_connect"    — /proc/net/tcp LISTEN + actual TCP connect (plain TCP services)
			// "socks5"         — /proc/net/tcp LISTEN + SOCKS5 greeting exchange
			// "http_api"       — /proc/net/tcp LISTEN + HTTP GET response
			// "dns_udp"        — /proc/net/udp bind + actual UDP DNS response (mihomo DNS port)
			// "udp_listen"     — /proc/net/udp bind only (dnsmasq :53 upstream: may not bind lo)
			// "nft_chain"      — nftables dns_redirect prerouting chain inspection (LAN client redirect)
			// "dns_lo_redirect"— UDP DNS query to 127.0.0.1:53 via OUTPUT nat redirect (replace mode, router itself)
			mode string
		}

		specs := []spec{
			{
				name:     "Mihomo API",
				desc:     fmt.Sprintf("Mihomo 内核 REST API — TCP :%d", cfg.Ports.MihomoAPI),
				port:     cfg.Ports.MihomoAPI,
				required: true,
				mode:     "http_api",
			},
		}

		if cfg.Ports.HTTP > 0 {
			specs = append(specs, spec{
				name: "HTTP 代理",
				desc: fmt.Sprintf("HTTP 代理 — TCP :%d", cfg.Ports.HTTP),
				port: cfg.Ports.HTTP,
				mode: "tcp_connect",
			})
		}
		if cfg.Ports.Mixed > 0 {
			specs = append(specs, spec{
				name: "Mixed 代理",
				desc: fmt.Sprintf("HTTP+SOCKS5 混合代理 — TCP :%d", cfg.Ports.Mixed),
				port: cfg.Ports.Mixed,
				mode: "tcp_connect",
			})
		}
		if cfg.Ports.SOCKS > 0 {
			specs = append(specs, spec{
				name: "SOCKS5 代理",
				desc: fmt.Sprintf("SOCKS5 代理 — TCP :%d", cfg.Ports.SOCKS),
				port: cfg.Ports.SOCKS,
				mode: "socks5",
			})
		}
		if cfg.Network.Mode == "tproxy" && cfg.Ports.TProxy > 0 {
			// TProxy uses IP_TRANSPARENT socket — accepts only kernel-redirected traffic,
			// not ordinary loopback connections. Check LISTEN state only.
			specs = append(specs, spec{
				name: "TProxy 透明代理",
				desc: fmt.Sprintf("透明代理入口 — TCP :%d (IP_TRANSPARENT, 仅检测 LISTEN 状态)", cfg.Ports.TProxy),
				port: cfg.Ports.TProxy,
				mode: "tcp_listen",
			})
		}
		if cfg.Network.Mode == "redir" && cfg.Ports.Redir > 0 {
			specs = append(specs, spec{
				name: "Redir 透明代理",
				desc: fmt.Sprintf("透明代理入口 — TCP :%d (仅检测 LISTEN 状态)", cfg.Ports.Redir),
				port: cfg.Ports.Redir,
				mode: "tcp_listen",
			})
		}

		if cfg.DNS.Enable && cfg.Ports.DNS > 0 {
			// Mihomo DNS listens on UDP (and TCP), send a real UDP DNS query to confirm response.
			specs = append(specs, spec{
				name: "Mihomo DNS",
				desc: fmt.Sprintf("Mihomo DNS 解析 — UDP :%d", cfg.Ports.DNS),
				port: cfg.Ports.DNS,
				mode: "dns_udp",
			})
		}

		if cfg.DNS.Enable && cfg.DNS.ApplyOnStart && cfg.DNS.DnsmasqMode != "none" {
			switch cfg.DNS.DnsmasqMode {
			case "replace":
			// LAN clients: verify the prerouting nftables chain redirects :53 → dnsPort.
			specs = append(specs, spec{
				name: "DNS :53 接管 — LAN 客户端 (replace)",
				desc: fmt.Sprintf("nftables dns_redirect prerouting：:53 → :%d", cfg.Ports.DNS),
				port: 53,
				mode: "nft_chain",
			})
			// Router itself: send a real DNS query to 127.0.0.1:53 via the OUTPUT nat hook.
			// dnsmasq port=0 means nothing binds :53; the OUTPUT chain redirects loopback
			// DNS transparently to mihomo so the router's own DNS resolution works.
			specs = append(specs, spec{
				name: "DNS :53 接管 — 本机 DNS (replace)",
				desc: fmt.Sprintf("nftables dns_output_redirect output：:53 → :%d (路由器本机 DNS)", cfg.Ports.DNS),
				port: 53,
				mode: "dns_lo_redirect",
			})
			case "upstream":
				// dnsmasq keeps port 53 and forwards queries to mihomo DNS port.
				// dnsmasq in OpenWrt may only bind to the LAN interface, not 127.0.0.1,
				// so a DNS query to 127.0.0.1:53 is unreliable. Use /proc/net/udp instead.
				specs = append(specs, spec{
					name: "DNS :53 接管 (upstream)",
					desc: fmt.Sprintf("dnsmasq :53 绑定 → upstream mihomo :%d", cfg.Ports.DNS),
					port: 53,
					mode: "udp_listen",
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
			var err error
			switch s.mode {
			case "tcp_listen":
				err = checkTCPListen(s.port)
			case "tcp_connect":
				err = checkTCPConnect(s.port)
			case "socks5":
				err = checkSOCKS5(s.port)
			case "http_api":
				err = checkHTTPAPI(s.port)
			case "dns_udp":
				err = checkDNSUDP(s.port)
			case "udp_listen":
				err = checkUDPListen(s.port)
			case "nft_chain":
				err = checkNftDNSRedirect(cfg.Ports.DNS)
			case "dns_lo_redirect":
				err = checkDNSLoopbackRedirect()
			}
			if err != nil {
				item.Error = err.Error()
			} else {
				item.OK = true
				item.LatencyMs = time.Since(start).Milliseconds()
			}
			results = append(results, item)
		}

		JSON(w, http.StatusOK, portCheckResp{Checks: results})
	}
}

// procNetListening returns true if the given port has a listening/bound entry in
// /proc/net/{tcp,tcp6} (state=LISTEN/0A) or /proc/net/{udp,udp6} (any entry).
func procNetListening(port int, proto string) bool {
	hexPort := fmt.Sprintf("%04X", port)
	var paths []string
	switch proto {
	case "tcp":
		paths = []string{"/proc/net/tcp", "/proc/net/tcp6"}
	case "udp":
		paths = []string{"/proc/net/udp", "/proc/net/udp6"}
	default:
		return false
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for i, line := range strings.Split(string(data), "\n") {
			if i == 0 {
				continue // skip header line
			}
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			// fields[1] = local_address "HEXIP:HEXPORT"
			colon := strings.LastIndex(fields[1], ":")
			if colon < 0 || !strings.EqualFold(fields[1][colon+1:], hexPort) {
				continue
			}
			// TCP: must be LISTEN state (0A). UDP: any bound entry is sufficient.
			if proto == "tcp" && fields[3] != "0A" {
				continue
			}
			return true
		}
	}
	return false
}

// checkTCPListen verifies a TCP port is in LISTEN state via /proc/net/tcp.
// Used for TProxy/Redir ports that cannot be directly connected to from localhost.
func checkTCPListen(port int) error {
	if !procNetListening(port, "tcp") {
		return fmt.Errorf("TCP 未监听 (/proc/net/tcp 中无 LISTEN 条目)")
	}
	return nil
}

// checkTCPConnect verifies LISTEN state and performs an actual TCP connect.
// Used for plain proxy ports (HTTP, Mixed) where a connection is meaningful.
func checkTCPConnect(port int) error {
	if !procNetListening(port, "tcp") {
		return fmt.Errorf("TCP 未监听 (/proc/net/tcp 中无 LISTEN 条目)")
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		return fmt.Errorf("TCP 连接失败: %w", err)
	}
	conn.Close()
	return nil
}

// checkSOCKS5 verifies LISTEN state and exchanges the SOCKS5 method-selection greeting.
func checkSOCKS5(port int) error {
	if !procNetListening(port, "tcp") {
		return fmt.Errorf("TCP 未监听 (/proc/net/tcp 中无 LISTEN 条目)")
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		return fmt.Errorf("TCP 连接失败: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	// ClientHello: VER=5, NMETHODS=1, METHOD=NO_AUTH(0x00)
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return fmt.Errorf("SOCKS5 握手发送失败: %w", err)
	}
	resp := make([]byte, 2)
	if _, err := conn.Read(resp); err != nil {
		return fmt.Errorf("SOCKS5 无握手响应: %w", err)
	}
	if resp[0] != 0x05 {
		return fmt.Errorf("非 SOCKS5 响应 (首字节 0x%02x, 期望 0x05)", resp[0])
	}
	return nil
}

// checkHTTPAPI verifies LISTEN state and checks the Mihomo REST API returns an HTTP response.
func checkHTTPAPI(port int) error {
	if !procNetListening(port, "tcp") {
		return fmt.Errorf("TCP 未监听 (/proc/net/tcp 中无 LISTEN 条目)")
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/version", port))
	if err != nil {
		return fmt.Errorf("HTTP 响应失败: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("HTTP 状态码异常: %d", resp.StatusCode)
	}
	return nil
}

// checkDNSUDP verifies UDP bind state via /proc/net/udp and sends a real DNS query.
// Used for the Mihomo DNS port which genuinely listens on UDP and should respond.
func checkDNSUDP(port int) error {
	if !procNetListening(port, "udp") {
		return fmt.Errorf("UDP 未绑定 (/proc/net/udp 中无条目)")
	}
	conn, err := net.DialTimeout("udp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		return fmt.Errorf("UDP 连接失败: %w", err)
	}
	defer conn.Close()
	// Minimal DNS query: root (.) A IN
	query := []byte{
		0x12, 0x34, 0x01, 0x00, // TxID=0x1234, flags: standard query, RD
		0x00, 0x01, 0x00, 0x00, // QDCOUNT=1
		0x00, 0x00, 0x00, 0x00, // NSCOUNT=0, ARCOUNT=0
		0x00,       // QNAME: root label
		0x00, 0x01, // QTYPE: A
		0x00, 0x01, // QCLASS: IN
	}
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write(query); err != nil {
		return fmt.Errorf("DNS 查询发送失败: %w", err)
	}
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		return fmt.Errorf("DNS 无响应: %w", err)
	}
	if n < 4 {
		return fmt.Errorf("DNS 响应过短 (%d 字节)", n)
	}
	return nil
}

// checkUDPListen verifies a UDP port is bound via /proc/net/udp.
// Used for dnsmasq :53 in upstream mode: dnsmasq may only bind to the LAN
// interface (not 127.0.0.1), so sending a DNS query to localhost is unreliable.
func checkUDPListen(port int) error {
	if !procNetListening(port, "udp") {
		return fmt.Errorf("UDP 未绑定 (/proc/net/udp 中无条目，dnsmasq 可能未运行)")
	}
	return nil
}

// checkDNSLoopbackRedirect sends a real DNS query to 127.0.0.1:53 without any
// procNet pre-check. In replace mode the kernel's OUTPUT nat hook (dns_output_redirect
// chain) transparently rewrites the destination to mihomo's DNS port, so the router's
// own DNS resolution works even though nothing actually binds :53.
func checkDNSLoopbackRedirect() error {
	conn, err := net.DialTimeout("udp", "127.0.0.1:53", 2*time.Second)
	if err != nil {
		return fmt.Errorf("UDP 连接 127.0.0.1:53 失败: %w", err)
	}
	defer conn.Close()
	// Minimal DNS query: root (.) A IN
	query := []byte{
		0x12, 0x34, 0x01, 0x00, // TxID=0x1234, flags: standard query, RD
		0x00, 0x01, 0x00, 0x00, // QDCOUNT=1
		0x00, 0x00, 0x00, 0x00, // NSCOUNT=0, ARCOUNT=0
		0x00,       // QNAME: root label
		0x00, 0x01, // QTYPE: A
		0x00, 0x01, // QCLASS: IN
	}
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write(query); err != nil {
		return fmt.Errorf("DNS 查询发送失败: %w", err)
	}
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		return fmt.Errorf("DNS 无响应 — OUTPUT hook 重定向未生效或 mihomo DNS 未运行: %w", err)
	}
	if n < 4 {
		return fmt.Errorf("DNS 响应过短 (%d 字节)", n)
	}
	return nil
}
// Used for replace mode to confirm LAN client DNS redirect is in place.
func checkNftDNSRedirect(dnsPort int) error {
	out, err := exec.Command("nft", "list", "chain", "inet", "metaclash", "dns_redirect").CombinedOutput()
	if err != nil {
		s := string(out)
		if strings.Contains(s, "No such file") || strings.Contains(s, "table not found") ||
			strings.Contains(s, "does not exist") || strings.Contains(s, "no such chain") {
			return fmt.Errorf("nftables dns_redirect 链不存在，:53 接管未生效")
		}
		return fmt.Errorf("nft list chain: %w", err)
	}
	expected := fmt.Sprintf("redirect to :%d", dnsPort)
	if !strings.Contains(string(out), expected) {
		return fmt.Errorf("规则存在但重定向目标不正确 (期望 %s)", expected)
	}
	return nil
}
