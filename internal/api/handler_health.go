package api

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/wujun4code/clashforge/internal/probetargets"
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
			targetURL = probetargets.DefaultHealthCheckTargetURL()
		}

		coreStatus := deps.Core.Status()
		mihomoAPIReady := testMihomoAPI(deps.Config.Ports.MihomoAPI)
		ports := buildHealthPorts(deps)
		httpTest := testHTTPProxyEndpoint("http", deps.Config.Ports.HTTP, targetURL, deps.Config.Core.RuntimeDir)
		mixedTest := testHTTPProxyEndpoint("mixed", deps.Config.Ports.Mixed, targetURL, deps.Config.Core.RuntimeDir)
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
			"ports":             ports,
			"transparent_proxy": buildTransparentProxyHealth(deps),
			"nft":               buildNFTHealth(deps),
			"dns":               buildDNSHealth(deps),
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

func testHTTPProxyEndpoint(name string, port int, targetURL string, runtimeDir string) healthProxyTest {
	listening := isTCPPortListening(port)
	if !listening {
		return healthProxyTest{Name: name, Port: port, Listening: false, OK: false, Error: "port is not listening"}
	}
	client := http.Client{
		Timeout:   8 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyURL(mihomoProxyURL(port, runtimeDir))},
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

type domainProbeReq struct {
	Domain string `json:"domain"`
}

type domainProbeResult struct {
	Domain     string   `json:"domain"`
	CheckedAt  string   `json:"checked_at"`
	DNSIPs     []string `json:"dns_ips,omitempty"`
	DNSError   string   `json:"dns_error,omitempty"`
	OK         bool     `json:"ok"`
	LatencyMS  int64    `json:"latency_ms,omitempty"`
	StatusCode int      `json:"status_code,omitempty"`
	Error      string   `json:"error,omitempty"`
}

// proxyDiagResult holds comprehensive proxy diagnostic result.
type proxyDiagResult struct {
	CheckedAt         string                      `json:"checked_at"`
	OverallOK         bool                        `json:"overall_ok"`
	Issues            []string                    `json:"issues,omitempty"`
	CoreStatus        string                      `json:"core_status"`
	CoreReady         bool                        `json:"core_ready"`
	ProxyCount        int                         `json:"proxy_count"`
	ProxyGroups       []proxyDiagGroup            `json:"proxy_groups"`
	DNSTest           proxyDiagDNS                `json:"dns_test"`
	NetfilterOK       bool                        `json:"netfilter_ok"`
	ConnectivityTests []proxyDiagConnectivityTest `json:"connectivity_tests,omitempty"`
	RuleSummary       []string                    `json:"rule_summary,omitempty"`
}

type proxyDiagConnectivityTest struct {
	Name  string          `json:"name"`
	Group string          `json:"group,omitempty"`
	URL   string          `json:"url"`
	Test  healthProxyTest `json:"test"`
}

type proxyDiagGroup struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Now        string   `json:"now,omitempty"`
	ProxyCount int      `json:"proxy_count"`
	Proxies    []string `json:"proxies,omitempty"`
}

type proxyDiagDNS struct {
	MihomoDNSListening bool     `json:"mihomo_dns_listening"`
	Port53Listening    bool     `json:"port_53_listening"`
	Nameservers        []string `json:"nameservers,omitempty"`
	Fallback           []string `json:"fallback,omitempty"`
	EnhancedMode       string   `json:"enhanced_mode,omitempty"`
}

// handleProxyDiag runs a comprehensive proxy diagnostic based on shared probe targets.
func handleProxyDiag(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var issues []string

		// 1. Core status
		coreStatus := deps.Core.Status()
		if coreStatus.State != "running" {
			issues = append(issues, fmt.Sprintf("mihomo 内核未运行 (状态: %s)", coreStatus.State))
		}

		// 2. Proxy count and groups via mihomo API
		var groups []proxyDiagGroup
		proxyCount := 0
		if coreStatus.State == "running" {
			proxyData, err := queryMihomoAPI(deps.Config.Ports.MihomoAPI, "/proxies")
			if err != nil {
				issues = append(issues, fmt.Sprintf("无法查询 mihomo 代理 API: %v", err))
			} else {
				if proxies, ok := proxyData["proxies"].(map[string]interface{}); ok {
					proxyCount = len(proxies)
					for name, v := range proxies {
						if pm, ok2 := v.(map[string]interface{}); ok2 {
							ptype, _ := pm["type"].(string)
							// Only log Selector and URLTest groups (not individual proxies)
							if ptype == "Selector" || ptype == "URLTest" || ptype == "Fallback" || ptype == "LoadBalance" {
								now, _ := pm["now"].(string)
								var proxyNames []string
								if all, ok3 := pm["all"].([]interface{}); ok3 {
									for _, p := range all {
										if s, ok4 := p.(string); ok4 {
											proxyNames = append(proxyNames, s)
										}
									}
								}
								groups = append(groups, proxyDiagGroup{
									Name:       name,
									Type:       ptype,
									Now:        now,
									ProxyCount: len(proxyNames),
									Proxies:    firstNString(proxyNames, 10),
								})
								// Check if the active proxy is DIRECT
								if now == "DIRECT" && (name == "Proxy" || name == "Final" || name == "🚀 节点选择") {
									issues = append(issues, fmt.Sprintf("代理组 %q 当前选择了 DIRECT，国际流量将无法代理", name))
								}
							}
						}
					}
				}
				if proxyCount <= 2 { // DIRECT + REJECT are always present
					issues = append(issues, "代理节点数量为 0！所有国际流量将走 DIRECT，谷歌等境外网站将无法访问")
				}
			}
		}

		// 3. DNS test
		dnsTest := proxyDiagDNS{
			MihomoDNSListening: isDNSPortListening(deps.Config.Ports.DNS),
			Port53Listening:    isTCPPortListening(53),
		}
		if coreStatus.State == "running" {
			dnsData, err := queryMihomoAPI(deps.Config.Ports.MihomoAPI, "/configs")
			if err == nil {
				if dns, ok := dnsData["dns"].(map[string]interface{}); ok {
					dnsTest.EnhancedMode, _ = dns["enhanced-mode"].(string)
					if ns, ok2 := dns["nameserver"].([]interface{}); ok2 {
						for _, n := range ns {
							if s, ok3 := n.(string); ok3 {
								dnsTest.Nameservers = append(dnsTest.Nameservers, s)
							}
						}
					}
					if fb, ok2 := dns["fallback"].([]interface{}); ok2 {
						for _, n := range fb {
							if s, ok3 := n.(string); ok3 {
								dnsTest.Fallback = append(dnsTest.Fallback, s)
							}
						}
					}
					if len(dnsTest.Fallback) == 0 && dnsTest.EnhancedMode == "fake-ip" {
						issues = append(issues, "DNS fallback 为空且使用 fake-ip 模式，境外域名可能无法正确解析")
					}
				}
			}
		}
		if !dnsTest.MihomoDNSListening && deps.Config.DNS.Enable {
			issues = append(issues, fmt.Sprintf("mihomo DNS 端口 %d 未监听，DNS 解析可能失败", deps.Config.Ports.DNS))
		}

		// 4. Netfilter
		netfilterOK := deps.Netfilter != nil && deps.Netfilter.IsApplied()
		if deps.Config.Network.Mode != "none" && deps.Config.Network.ApplyOnStart && !netfilterOK {
			issues = append(issues, "透明代理规则未生效，LAN 客户端流量可能绕过代理")
		}

		// 5. Proxy connectivity tests (shared probe target catalog)
		targets := probetargets.ConnectivityTargets()
		connectivityTests := make([]proxyDiagConnectivityTest, 0, len(targets))
		failedTargets := make([]string, 0, len(targets))
		for _, target := range targets {
			test := testHTTPProxyEndpoint(target.Name, deps.Config.Ports.Mixed, target.URL, deps.Config.Core.RuntimeDir)
			connectivityTests = append(connectivityTests, proxyDiagConnectivityTest{
				Name:  target.Name,
				Group: target.Group,
				URL:   target.URL,
				Test:  test,
			})
			if !test.OK {
				failedTargets = append(failedTargets, target.Name)
			}
		}
		if len(failedTargets) > 0 {
			issues = append(issues, "以下连通性目标探测失败: "+strings.Join(failedTargets, ", "))
		}
		if len(failedTargets) == len(targets) && len(targets) > 0 {
			issues = append(issues, "所有连通性探测目标均失败，代理链路可能整体不可用")
		}

		// 6. Rule summary from mihomo API
		var ruleSummary []string
		if coreStatus.State == "running" {
			rulesData, err := queryMihomoAPI(deps.Config.Ports.MihomoAPI, "/rules")
			if err == nil {
				if rules, ok := rulesData["rules"].([]interface{}); ok {
					total := len(rules)
					ruleSummary = append(ruleSummary, fmt.Sprintf("总规则数: %d", total))
					// Find MATCH rule
					for _, r := range rules {
						if rm, ok2 := r.(map[string]interface{}); ok2 {
							rtype, _ := rm["type"].(string)
							if rtype == "MATCH" {
								payload, _ := rm["payload"].(string)
								proxy, _ := rm["proxy"].(string)
								ruleSummary = append(ruleSummary, fmt.Sprintf("MATCH 规则: payload=%s proxy=%s", payload, proxy))
							}
						}
					}
				}
			}
		}

		overallOK := len(issues) == 0

		JSON(w, http.StatusOK, proxyDiagResult{
			CheckedAt:         time.Now().UTC().Format(time.RFC3339),
			OverallOK:         overallOK,
			Issues:            issues,
			CoreStatus:        string(coreStatus.State),
			CoreReady:         coreStatus.Ready,
			ProxyCount:        proxyCount,
			ProxyGroups:       groups,
			DNSTest:           dnsTest,
			NetfilterOK:       netfilterOK,
			ConnectivityTests: connectivityTests,
			RuleSummary:       ruleSummary,
		})
	}
}

func queryMihomoAPI(port int, path string) (map[string]interface{}, error) {
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d%s", port, path))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func firstNString(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func handleProbeDomain(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req domainProbeReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body")
			return
		}
		domain := strings.TrimSpace(req.Domain)
		domain = strings.TrimPrefix(domain, "https://")
		domain = strings.TrimPrefix(domain, "http://")
		if idx := strings.IndexByte(domain, '/'); idx != -1 {
			domain = domain[:idx]
		}
		if domain == "" {
			Err(w, http.StatusBadRequest, "INVALID_REQUEST", "domain is required")
			return
		}

		res := domainProbeResult{Domain: domain, CheckedAt: time.Now().UTC().Format(time.RFC3339)}

		ips, dnsErr := net.LookupHost(domain)
		if dnsErr != nil {
			res.DNSError = dnsErr.Error()
		} else {
			res.DNSIPs = ips
		}

		test := testHTTPProxyEndpoint("mixed", deps.Config.Ports.Mixed, "https://"+domain, deps.Config.Core.RuntimeDir)
		res.OK = test.OK
		res.LatencyMS = test.DurationMS
		res.StatusCode = test.StatusCode
		if test.Error != "" {
			res.Error = test.Error
		}

		JSON(w, http.StatusOK, res)
	}
}
