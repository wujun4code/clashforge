package api

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type healthProcess struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	PID     int    `json:"pid,omitempty"`
	State   string `json:"state,omitempty"`
	Uptime  int64  `json:"uptime,omitempty"`
}

type healthPort struct {
	Name      string `json:"name"`
	Port      int    `json:"port"`
	Proto     string `json:"proto"`
	Required  bool   `json:"required"`
	Listening bool   `json:"listening"`
	Message   string `json:"message"`
}

type healthTakeover struct {
	Configured   bool   `json:"configured"`
	ApplyOnStart bool   `json:"apply_on_start"`
	Active       bool   `json:"active"`
	Mode         string `json:"mode,omitempty"`
	Backend      string `json:"backend,omitempty"`
	RulesApplied bool   `json:"rules_applied,omitempty"`
	TablePresent bool   `json:"table_present,omitempty"`
	Message      string `json:"message"`
}

type healthDNS struct {
	Enabled            bool   `json:"enabled"`
	ApplyOnStart       bool   `json:"apply_on_start"`
	DnsmasqMode        string `json:"dnsmasq_mode"`
	Active             bool   `json:"active"`
	ManagedFilePresent bool   `json:"managed_file_present"`
	ListenerReady      bool   `json:"listener_ready"`
	Message            string `json:"message"`
}

type healthProxyTest struct {
	Name       string `json:"name"`
	Port       int    `json:"port"`
	Listening  bool   `json:"listening"`
	OK         bool   `json:"ok"`
	StatusCode int    `json:"status_code,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
	Error      string `json:"error,omitempty"`
}

func handleHealthCheck(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		targetURL := strings.TrimSpace(r.URL.Query().Get("target"))
		if targetURL == "" {
			targetURL = "https://www.google.com"
		}

		coreStatus := deps.Core.Status()
		mihomoAPIReady := testMihomoAPI(deps.Config.Ports.MihomoAPI)
		ports := buildHealthPorts(deps)
		httpTest := testHTTPProxyEndpoint("http", deps.Config.Ports.HTTP, targetURL)
		mixedTest := testHTTPProxyEndpoint("mixed", deps.Config.Ports.Mixed, targetURL)
		socksTest := testSOCKS5Endpoint("socks", deps.Config.Ports.SOCKS, targetURL)
		apiTest := testMihomoAPIEndpoint(deps.Config.Ports.MihomoAPI)

		failures := 0
		warnings := 0
		if coreStatus.State != "running" || !mihomoAPIReady {
			failures++
		}
		for _, port := range ports {
			if port.Required && !port.Listening {
				failures++
			}
		}
		for _, test := range []healthProxyTest{httpTest, mixedTest, socksTest, apiTest} {
			if !test.OK {
				failures++
			}
		}
		if deps.Config.Network.Mode != "none" && !deps.Config.Network.ApplyOnStart {
			warnings++
		}
		if deps.Config.DNS.DnsmasqMode != "none" && !deps.Config.DNS.ApplyOnStart {
			warnings++
		}

		JSON(w, http.StatusOK, map[string]any{
			"checked_at": time.Now().UTC().Format(time.RFC3339),
			"summary": map[string]any{
				"healthy":  failures == 0,
				"failures": failures,
				"warnings": warnings,
			},
			"process": map[string]any{
				"clashforge": healthProcess{
					OK:      true,
					Message: "clashforge API responding normally",
					PID:     os.Getpid(),
					State:   "running",
					Uptime:  int64(time.Since(deps.StartedAt).Seconds()),
				},
				"mihomo": healthProcess{
					OK:      coreStatus.State == "running" && mihomoAPIReady,
					Message: buildMihomoHealthMessage(string(coreStatus.State), mihomoAPIReady),
					PID:     coreStatus.PID,
					State:   string(coreStatus.State),
					Uptime:  coreStatus.Uptime,
				},
			},
			"ports": ports,
			"transparent_proxy": buildTransparentProxyHealth(deps),
			"nft":                buildNFTHealth(deps),
			"dns":                buildDNSHealth(deps),
			"proxy_tests": map[string]any{
				"target_url": targetURL,
				"http":       httpTest,
				"mixed":      mixedTest,
				"socks":      socksTest,
				"mihomo_api": apiTest,
			},
		})
	}
}

func buildHealthPorts(deps Dependencies) []healthPort {
	ports := []healthPort{
		makeHealthPort("http", deps.Config.Ports.HTTP, "tcp", true),
		makeHealthPort("socks", deps.Config.Ports.SOCKS, "tcp", true),
		makeHealthPort("mixed", deps.Config.Ports.Mixed, "tcp", true),
		makeHealthPort("mihomo_api", deps.Config.Ports.MihomoAPI, "tcp", true),
	}
	if deps.Config.Network.Mode != "none" {
		ports = append(ports,
			makeHealthPort("redir", deps.Config.Ports.Redir, "tcp", false),
			makeHealthPort("tproxy", deps.Config.Ports.TProxy, "tcp", false),
		)
	}
	if deps.Config.DNS.Enable {
		ports = append(ports, makeHealthPort("dns", deps.Config.Ports.DNS, "tcp", false))
	}
	return ports
}

func makeHealthPort(name string, port int, proto string, required bool) healthPort {
	listening := isTCPPortListening(port)
	message := "listening"
	if !listening {
		message = "not listening"
	}
	return healthPort{Name: name, Port: port, Proto: proto, Required: required, Listening: listening, Message: message}
}

func buildMihomoHealthMessage(state string, apiReady bool) string {
	if state != "running" {
		return "mihomo process is not in running state"
	}
	if !apiReady {
		return "mihomo process is running but API is unreachable"
	}
	return "mihomo process and API are healthy"
}

func buildTransparentProxyHealth(deps Dependencies) healthTakeover {
	configured := deps.Config.Network.Mode != "none"
	active := deps.Netfilter != nil && deps.Netfilter.IsApplied()
	backend := actualNetfilterBackend(deps)
	message := "transparent proxy mode disabled in config"
	if configured && !deps.Config.Network.ApplyOnStart {
		message = "transparent proxy configured but disabled on startup"
	} else if configured && active {
		message = "transparent proxy rules are active"
	} else if configured {
		message = "transparent proxy configured but rules are not active"
	}
	return healthTakeover{
		Configured:   configured,
		ApplyOnStart: deps.Config.Network.ApplyOnStart,
		Active:       active,
		Mode:         deps.Config.Network.Mode,
		Backend:      backend,
		RulesApplied: active,
		Message:      message,
	}
}

func buildNFTHealth(deps Dependencies) healthTakeover {
	tablePresent := nftTablePresent()
	active := tablePresent && deps.Netfilter != nil && deps.Netfilter.IsApplied()
	backend := actualNetfilterBackend(deps)
	message := "nft takeover is inactive"
	if active {
		message = "nft rules are present and active"
	} else if tablePresent {
		message = "nft table exists but clashforge does not think rules are active"
	}
	return healthTakeover{
		Configured:   deps.Config.Network.Mode != "none",
		ApplyOnStart: deps.Config.Network.ApplyOnStart,
		Active:       active,
		Mode:         deps.Config.Network.Mode,
		Backend:      backend,
		RulesApplied: deps.Netfilter != nil && deps.Netfilter.IsApplied(),
		TablePresent: tablePresent,
		Message:      message,
	}
}

func buildDNSHealth(deps Dependencies) healthDNS {
	managedFilePresent := fileExists("/etc/dnsmasq.d/clashforge.conf")
	listenerReady := isDNSPortListening(deps.Config.Ports.DNS)
	active := deps.Config.DNS.Enable && deps.Config.DNS.ApplyOnStart && deps.Config.DNS.DnsmasqMode != "none" && managedFilePresent && listenerReady
	message := "dns feature is disabled"
	if deps.Config.DNS.Enable && deps.Config.DNS.DnsmasqMode != "none" && !deps.Config.DNS.ApplyOnStart {
		if listenerReady {
			message = "mihomo dns listener is available without dnsmasq takeover"
		} else {
			message = "dns takeover is configured but disabled on startup; mihomo dns listener is not ready"
		}
	} else if active {
		message = "dns takeover is active"
	} else if deps.Config.DNS.Enable && listenerReady {
		message = "mihomo dns listener is available without dnsmasq takeover"
	} else if deps.Config.DNS.Enable {
		message = "mihomo dns listener is not ready"
	}
	return healthDNS{
		Enabled:            deps.Config.DNS.Enable,
		ApplyOnStart:       deps.Config.DNS.ApplyOnStart,
		DnsmasqMode:        deps.Config.DNS.DnsmasqMode,
		Active:             active,
		ManagedFilePresent: managedFilePresent,
		ListenerReady:      listenerReady,
		Message:            message,
	}
}

func actualNetfilterBackend(deps Dependencies) string {
	if deps.Netfilter != nil {
		if backend := deps.Netfilter.BackendName(); backend != "" {
			return backend
		}
	}
	return deps.Config.Network.FirewallBackend
}

func testMihomoAPI(port int) bool {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/version", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 500
}

func testMihomoAPIEndpoint(port int) healthProxyTest {
	start := time.Now()
	client := http.Client{Timeout: 4 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/version", port))
	if err != nil {
		return healthProxyTest{Name: "mihomo_api", Port: port, Listening: isTCPPortListening(port), OK: false, Error: err.Error()}
	}
	defer resp.Body.Close()
	return healthProxyTest{Name: "mihomo_api", Port: port, Listening: true, OK: resp.StatusCode >= 200 && resp.StatusCode < 500, StatusCode: resp.StatusCode, DurationMS: time.Since(start).Milliseconds()}
}

func testHTTPProxyEndpoint(name string, port int, targetURL string) healthProxyTest {
	listening := isTCPPortListening(port)
	if !listening {
		return healthProxyTest{Name: name, Port: port, Listening: false, OK: false, Error: "port is not listening"}
	}
	proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	client := http.Client{
		Timeout: 8 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}
	req, err := http.NewRequest(http.MethodHead, targetURL, nil)
	if err != nil {
		return healthProxyTest{Name: name, Port: port, Listening: true, OK: false, Error: err.Error()}
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return healthProxyTest{Name: name, Port: port, Listening: true, OK: false, Error: err.Error()}
	}
	defer resp.Body.Close()
	return healthProxyTest{Name: name, Port: port, Listening: true, OK: resp.StatusCode >= 200 && resp.StatusCode < 500, StatusCode: resp.StatusCode, DurationMS: time.Since(start).Milliseconds()}
}

func testSOCKS5Endpoint(name string, port int, targetURL string) healthProxyTest {
	listening := isTCPPortListening(port)
	if !listening {
		return healthProxyTest{Name: name, Port: port, Listening: false, OK: false, Error: "port is not listening"}
	}
	start := time.Now()
	statusCode, err := issueSOCKS5Head(port, targetURL)
	if err != nil {
		return healthProxyTest{Name: name, Port: port, Listening: true, OK: false, Error: err.Error()}
	}
	return healthProxyTest{Name: name, Port: port, Listening: true, OK: statusCode >= 200 && statusCode < 500, StatusCode: statusCode, DurationMS: time.Since(start).Milliseconds()}
}

func issueSOCKS5Head(port int, targetURL string) (int, error) {
	target, err := url.Parse(targetURL)
	if err != nil {
		return 0, err
	}
	host := target.Hostname()
	targetPort := target.Port()
	if targetPort == "" {
		if strings.EqualFold(target.Scheme, "https") {
			targetPort = "443"
		} else {
			targetPort = "80"
		}
	}
	conn, err := dialSOCKS5(port, host, targetPort)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))

	var rw net.Conn = conn
	if strings.EqualFold(target.Scheme, "https") {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err := tlsConn.Handshake(); err != nil {
			return 0, err
		}
		rw = tlsConn
	}
	path := target.RequestURI()
	if path == "" {
		path = "/"
	}
	request := fmt.Sprintf("HEAD %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: clashforge-health/1.0\r\nConnection: close\r\n\r\n", path, host)
	if _, err := rw.Write([]byte(request)); err != nil {
		return 0, err
	}
	statusLine, err := bufio.NewReader(rw).ReadString('\n')
	if err != nil {
		return 0, err
	}
	parts := strings.Fields(statusLine)
	if len(parts) < 2 {
		return 0, fmt.Errorf("unexpected response: %s", strings.TrimSpace(statusLine))
	}
	statusCode, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}
	return statusCode, nil
}

func dialSOCKS5(port int, host, targetPort string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 3*time.Second)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		conn.Close()
		return nil, err
	}
	buf := make([]byte, 2)
	if _, err := conn.Read(buf); err != nil {
		conn.Close()
		return nil, err
	}
	if buf[0] != 0x05 || buf[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("SOCKS5 auth negotiation failed")
	}
	portNum, err := strconv.Atoi(targetPort)
	if err != nil {
		conn.Close()
		return nil, err
	}
	req := []byte{0x05, 0x01, 0x00, 0x03, byte(len(host))}
	req = append(req, []byte(host)...)
	req = append(req, byte(portNum>>8), byte(portNum))
	if _, err := conn.Write(req); err != nil {
		conn.Close()
		return nil, err
	}
	resp := make([]byte, 4)
	if _, err := conn.Read(resp); err != nil {
		conn.Close()
		return nil, err
	}
	if resp[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("SOCKS5 connect failed with code %d", resp[1])
	}
	if err := discardSOCKS5Address(conn, resp[3]); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func discardSOCKS5Address(conn net.Conn, atyp byte) error {
	var skip int
	switch atyp {
	case 0x01:
		skip = 4
	case 0x04:
		skip = 16
	case 0x03:
		lenBuf := make([]byte, 1)
		if _, err := conn.Read(lenBuf); err != nil {
			return err
		}
		skip = int(lenBuf[0])
	default:
		return fmt.Errorf("unsupported SOCKS5 address type %d", atyp)
	}
	buf := make([]byte, skip+2)
	_, err := conn.Read(buf)
	return err
}

func nftTablePresent() bool {
	out, err := exec.Command("nft", "list", "table", "inet", "metaclash").CombinedOutput()
	if err != nil {
		return false
	}
	return len(out) > 0
}

func isTCPPortListening(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 800*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func isDNSPortListening(port int) bool {
	return isTCPPortListening(port) || isUDPPortListening(port)
}

func isUDPPortListening(port int) bool {
	return procNetHasLocalPort("/proc/net/udp", port) || procNetHasLocalPort("/proc/net/udp6", port)
}

func procNetHasLocalPort(path string, port int) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	wantPort := strings.ToUpper(fmt.Sprintf("%04X", port))
	scanner := bufio.NewScanner(f)
	firstLine := true
	for scanner.Scan() {
		if firstLine {
			firstLine = false
			continue
		}
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		localAddr := strings.Split(fields[1], ":")
		if len(localAddr) != 2 {
			continue
		}
		if strings.EqualFold(localAddr[1], wantPort) {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}