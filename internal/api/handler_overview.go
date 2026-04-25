package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/core"
	"github.com/wujun4code/clashforge/internal/dns"
	"github.com/wujun4code/clashforge/internal/netfilter"
	"gopkg.in/yaml.v3"
)

type overviewData struct {
	CheckedAt    string                `json:"checked_at"`
	Summary      overviewSummary       `json:"summary"`
	Resources    overviewResources     `json:"resources"`
	IPChecks     []overviewIPCheck     `json:"ip_checks"`
	AccessChecks []overviewAccessCheck `json:"access_checks"`
	Modules      []overviewModule      `json:"modules"`
	Influences   []overviewInfluence   `json:"influences"`
}

type overviewCoreData struct {
	CheckedAt  string              `json:"checked_at"`
	Core       overviewCoreInfo    `json:"core"`
	Summary    overviewSummary     `json:"summary"`
	Modules    []overviewModule    `json:"modules"`
	Influences []overviewInfluence `json:"influences"`
}

type overviewCoreInfo struct {
	State             string `json:"state"`
	PID               int    `json:"pid"`
	Uptime            int64  `json:"uptime"`
	Running           bool   `json:"running"`
	ActiveConnections int    `json:"active_connections"`
}

type overviewProbeData struct {
	CheckedAt    string                `json:"checked_at"`
	IPChecks     []overviewIPCheck     `json:"ip_checks"`
	AccessChecks []overviewAccessCheck `json:"access_checks"`
}

type overviewResourceData struct {
	CheckedAt string            `json:"checked_at"`
	Resources overviewResources `json:"resources"`
}

type overviewSummary struct {
	CoreRunning       bool   `json:"core_running"`
	ClashforgeHealthy bool   `json:"clashforge_healthy"`
	ConflictCount     int    `json:"conflict_count"`
	TakeoverReady     int    `json:"takeover_ready"`
	Message           string `json:"message"`
}

type overviewResources struct {
	System    overviewSystemUsage    `json:"system"`
	Processes []overviewProcessUsage `json:"processes"`
	App       overviewAppStorage     `json:"app"`
}

type overviewSystemUsage struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryTotalMB float64 `json:"memory_total_mb"`
	MemoryUsedMB  float64 `json:"memory_used_mb"`
	MemoryPercent float64 `json:"memory_percent"`
	DiskTotalGB   float64 `json:"disk_total_gb"`
	DiskUsedGB    float64 `json:"disk_used_gb"`
	DiskPercent   float64 `json:"disk_percent"`
}

type overviewProcessUsage struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	PID         int     `json:"pid"`
	Running     bool    `json:"running"`
	CPUPercent  float64 `json:"cpu_percent"`
	MemoryRSSMB float64 `json:"memory_rss_mb"`
	Uptime      int64   `json:"uptime"`
	Command     string  `json:"command,omitempty"`
}

type overviewAppStorage struct {
	RuntimeMB  float64             `json:"runtime_mb"`
	DataMB     float64             `json:"data_mb"`
	BinaryMB   float64             `json:"binary_mb"`
	RulesMB    float64             `json:"rules_mb"`
	TotalMB    float64             `json:"total_mb"`
	RuleAssets []overviewRuleAsset `json:"rule_assets,omitempty"`
}

type overviewRuleAsset struct {
	Name   string  `json:"name"`
	Path   string  `json:"path"`
	SizeMB float64 `json:"size_mb"`
}

type overviewIPCheck struct {
	Provider string `json:"provider"`
	Group    string `json:"group,omitempty"`
	OK       bool   `json:"ok"`
	IP       string `json:"ip,omitempty"`
	Location string `json:"location,omitempty"`
	Error    string `json:"error,omitempty"`
}

type overviewAccessCheck struct {
	Name        string `json:"name"`
	Group       string `json:"group,omitempty"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Via         string `json:"via"`
	OK          bool   `json:"ok"`
	StatusCode  int    `json:"status_code,omitempty"`
	LatencyMS   int64  `json:"latency_ms,omitempty"`
	Error       string `json:"error,omitempty"`
	// Diagnostic fields for UI display
	Stage     string `json:"stage,omitempty"`      // failed stage: "proxy_port" | "dns" | "timeout" | "connect"
	DNSResult string `json:"dns_result,omitempty"` // resolved IP(s) for the domain, or error summary
}

type overviewProcessRef struct {
	PID     int    `json:"pid"`
	Name    string `json:"name"`
	Command string `json:"command,omitempty"`
	Service string `json:"service,omitempty"`
}

type overviewPortOwner struct {
	Port    int    `json:"port"`
	Proto   string `json:"proto"`
	Owner   string `json:"owner"`
	PID     int    `json:"pid,omitempty"`
	Command string `json:"command,omitempty"`
}

type overviewAction struct {
	Module       string   `json:"module"`
	Label        string   `json:"label"`
	Mode         string   `json:"mode,omitempty"`
	StopServices []string `json:"stop_services,omitempty"`
}

type overviewModule struct {
	ID                  string               `json:"id"`
	Title               string               `json:"title"`
	Category            string               `json:"category"`
	Status              string               `json:"status"`
	CurrentOwner        string               `json:"current_owner"`
	ManagedByClashforge bool                 `json:"managed_by_clashforge"`
	Purpose             string               `json:"purpose"`
	TakeoverEffect      string               `json:"takeover_effect"`
	CurrentMode         string               `json:"current_mode,omitempty"`
	RecommendedMode     string               `json:"recommended_mode,omitempty"`
	TakeoverSupported   bool                 `json:"takeover_supported"`
	Action              *overviewAction      `json:"action,omitempty"`
	Processes           []overviewProcessRef `json:"processes,omitempty"`
	Ports               []overviewPortOwner  `json:"ports,omitempty"`
	Notes               []string             `json:"notes,omitempty"`
}

type overviewInfluence struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Affects     []string             `json:"affects"`
	Running     bool                 `json:"running"`
	Stoppable   bool                 `json:"stoppable"`
	Service     string               `json:"service,omitempty"`
	Processes   []overviewProcessRef `json:"processes,omitempty"`
	Ports       []overviewPortOwner  `json:"ports,omitempty"`
}

type overviewTakeoverRequest struct {
	Module       string   `json:"module"`
	Mode         string   `json:"mode,omitempty"`
	StopServices []string `json:"stop_services,omitempty"`
}

type overviewTakeoverResponse struct {
	Updated      bool             `json:"updated"`
	Message      string           `json:"message"`
	Stopped      []string         `json:"stopped,omitempty"`
	NeedsRestart bool             `json:"needs_restart,omitempty"`
	Overview     overviewCoreData `json:"overview"`
}

type overviewReleaseResponse struct {
	Updated  bool             `json:"updated"`
	Message  string           `json:"message"`
	Overview overviewCoreData `json:"overview"`
}

type procMetricsSample struct {
	Ticks      uint64
	CPUPercent float64
	RSSBytes   uint64
	Uptime     int64
	Command    string
	Process    overviewProcessRef
}

type listeningPort struct {
	Port    int
	Proto   string
	PID     int
	Name    string
	Command string
}

type influenceSpec struct {
	ID          string
	Name        string
	Description string
	Affects     []string
	Match       []string
	Exclude     []string
	Ports       []int
	Services    []string
	Stoppable   bool
}

var knownInfluenceSpecs = []influenceSpec{
	{
		ID:          "openclash",
		Name:        "OpenClash",
		Description: "常见的路由透明代理套件，会占用 789x 端口、NFT 规则和 DNS 入口。",
		Affects:     []string{"transparent_proxy", "nft_firewall", "dns_entry"},
		Match:       []string{"openclash", "clash", "mihomo"},
		Exclude:     []string{"clashforge", "mihomo-clashforge"},
		Ports:       []int{7890, 7891, 7892, 7893, 7895, 9090},
		Services:    []string{"openclash"},
		Stoppable:   true,
	},
	{
		ID:          "dnsmasq",
		Name:        "dnsmasq",
		Description: "路由器默认 DNS / DHCP 服务，通常负责 53 端口入口。ClashForge 的 DNS 接管会与它协作或替换它的 DNS 监听。",
		Affects:     []string{"dns_entry"},
		Match:       []string{"dnsmasq"},
		Ports:       []int{53},
		Services:    []string{"dnsmasq"},
		Stoppable:   false,
	},
	{
		ID:          "smartdns",
		Name:        "SmartDNS",
		Description: "本地 DNS 加速 / 分流服务，会直接占用 53 端口并影响 ClashForge 的 DNS 接管。",
		Affects:     []string{"dns_entry"},
		Match:       []string{"smartdns"},
		Ports:       []int{53},
		Services:    []string{"smartdns"},
		Stoppable:   true,
	},
	{
		ID:          "mosdns",
		Name:        "mosdns",
		Description: "本地 DNS 分流服务，常和透明代理组合使用，也可能抢占 53 端口。",
		Affects:     []string{"dns_entry"},
		Match:       []string{"mosdns"},
		Ports:       []int{53},
		Services:    []string{"mosdns"},
		Stoppable:   true,
	},
	{
		ID:          "adguardhome",
		Name:        "AdGuard Home",
		Description: "本地 DNS 过滤服务，通常会占用 53 端口并替代 dnsmasq 的 DNS 入口。",
		Affects:     []string{"dns_entry"},
		Match:       []string{"adguardhome"},
		Ports:       []int{53},
		Services:    []string{"adguardhome", "AdGuardHome"},
		Stoppable:   true,
	},
	{
		ID:          "singbox",
		Name:        "sing-box",
		Description: "另一类代理核心，可能占用透明代理端口、外部控制端口或 NFT 规则。",
		Affects:     []string{"transparent_proxy", "nft_firewall"},
		Match:       []string{"sing-box", "singbox"},
		Ports:       []int{7890, 7891, 7892, 7893, 7895},
		Services:    []string{"sing-box", "singbox"},
		Stoppable:   true,
	},
	{
		ID:          "xray",
		Name:        "Xray / V2Ray",
		Description: "其他代理核心，可能复用本机代理端口或占用同类入口。",
		Affects:     []string{"transparent_proxy"},
		Match:       []string{"xray", "v2ray"},
		Ports:       []int{7890, 7891, 7892, 7893, 7895},
		Services:    []string{"xray", "v2ray"},
		Stoppable:   true,
	},
}

func handleOverview(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, buildOverviewData(deps))
	}
}

func handleOverviewCore(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, buildOverviewCoreData(deps))
	}
}

func handleOverviewProbes(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, buildOverviewProbeData(deps))
	}
}

func handleOverviewResources(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, buildOverviewResourceData(deps))
	}
}

func handleTakeoverOverviewModule(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req overviewTakeoverRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "OVERVIEW_ACTION_PARSE_FAILED", err.Error())
			return
		}
		req.Module = strings.TrimSpace(req.Module)
		if req.Module == "" {
			Err(w, http.StatusBadRequest, "OVERVIEW_ACTION_INVALID", "module is required")
			return
		}

		if err := ensureTakeoverAllowed(deps, req.Module); err != nil {
			Err(w, http.StatusConflict, "OVERVIEW_ACTION_BLOCKED", err.Error())
			return
		}

		stopped, err := stopAllowedServices(req.StopServices)
		if err != nil {
			Err(w, http.StatusInternalServerError, "OVERVIEW_ACTION_STOP_FAILED", err.Error())
			return
		}

		message := ""
		needsRestart := false
		switch req.Module {
		case "transparent_proxy", "nft_firewall":
			message, needsRestart, err = takeoverTransparentProxy(deps, req.Mode)
		case "dns_entry", "dns_resolver":
			message, needsRestart, err = takeoverDNS(deps, req.Mode)
		case "all":
			message, needsRestart, err = takeoverAll(deps)
		default:
			Err(w, http.StatusBadRequest, "OVERVIEW_ACTION_UNSUPPORTED", fmt.Sprintf("module %s does not support takeover", req.Module))
			return
		}
		if err != nil {
			Err(w, http.StatusInternalServerError, "OVERVIEW_ACTION_FAILED", err.Error())
			return
		}

		JSON(w, http.StatusOK, overviewTakeoverResponse{
			Updated:      true,
			Message:      message,
			Stopped:      stopped,
			NeedsRestart: needsRestart,
			Overview:     buildOverviewCoreData(deps),
		})
	}
}

func handleReleaseOverviewTakeover(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		message, err := releaseAllTakeover(deps)
		if err != nil {
			Err(w, http.StatusInternalServerError, "OVERVIEW_RELEASE_FAILED", err.Error())
			return
		}

		JSON(w, http.StatusOK, overviewReleaseResponse{
			Updated:  true,
			Message:  message,
			Overview: buildOverviewCoreData(deps),
		})
	}
}

func buildOverviewData(deps Dependencies) overviewData {
	listeners := listListeningPorts()
	coreStatus := deps.Core.Status()
	resources := buildOverviewResources(deps, coreStatus)
	influences := detectInfluences(listeners)
	modules := buildOverviewModules(deps, listeners, influences, coreStatus)
	ipChecks := buildOverviewIPChecks(deps)
	accessChecks := buildOverviewAccessChecks(deps)
	summary := buildOverviewSummary(coreStatus, modules, influences)

	return overviewData{
		CheckedAt:    time.Now().UTC().Format(time.RFC3339),
		Summary:      summary,
		Resources:    resources,
		IPChecks:     ipChecks,
		AccessChecks: accessChecks,
		Modules:      modules,
		Influences:   influences,
	}
}

func buildOverviewCoreData(deps Dependencies) overviewCoreData {
	listeners := listListeningPorts()
	coreStatus := deps.Core.Status()
	influences := detectInfluences(listeners)
	modules := filterCoreModules(buildOverviewModules(deps, listeners, influences, coreStatus))
	activeConnections := fetchMihomoConnectionCount(deps.Config.Ports.MihomoAPI)

	return overviewCoreData{
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
		Core: overviewCoreInfo{
			State:             string(coreStatus.State),
			PID:               coreStatus.PID,
			Uptime:            coreStatus.Uptime,
			Running:           coreStatus.Ready,
			ActiveConnections: activeConnections,
		},
		Summary:    buildOverviewSummary(coreStatus, modules, influences),
		Modules:    modules,
		Influences: influences,
	}
}

func fetchMihomoConnectionCount(port int) int {
	if port <= 0 {
		return 0
	}

	target := fmt.Sprintf("http://127.0.0.1:%d/connections", port)
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return 0
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return 0
	}

	var payload struct {
		Connections []json.RawMessage `json:"connections"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0
	}

	return len(payload.Connections)
}

func buildOverviewProbeData(deps Dependencies) overviewProbeData {
	return overviewProbeData{
		CheckedAt:    time.Now().UTC().Format(time.RFC3339),
		IPChecks:     buildOverviewIPChecks(deps),
		AccessChecks: buildOverviewAccessChecks(deps),
	}
}

func buildOverviewResourceData(deps Dependencies) overviewResourceData {
	coreStatus := deps.Core.Status()
	return overviewResourceData{
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
		Resources: buildOverviewResources(deps, coreStatus),
	}
}

func buildOverviewSummary(coreStatus core.Status, modules []overviewModule, influences []overviewInfluence) overviewSummary {
	conflicts := 0
	takeoverReady := 0
	for _, module := range modules {
		if module.TakeoverSupported && !module.ManagedByClashforge {
			takeoverReady++
		}
		if module.TakeoverSupported && strings.Contains(module.Status, "conflict") {
			conflicts++
		}
	}
	for _, influence := range influences {
		if influence.Running {
			conflicts++
		}
	}
	message := "ClashForge 已提供主要运行信息"
	if !coreStatus.Ready {
		message = "Mihomo 核心未运行，概览中的接管和访问结果会受到影响"
	} else if takeoverReady > 0 {
		message = "发现仍可由 ClashForge 接管的模块，可直接在概览页处理"
	}
	return overviewSummary{
		CoreRunning:       coreStatus.Ready,
		ClashforgeHealthy: coreStatus.Ready,
		ConflictCount:     conflicts,
		TakeoverReady:     takeoverReady,
		Message:           message,
	}
}

func filterCoreModules(modules []overviewModule) []overviewModule {
	allowed := map[string]bool{
		"proxy_core":        true,
		"transparent_proxy": true,
		"nft_firewall":      true,
		"dns_entry":         true,
		"dns_resolver":      true,
	}
	filtered := make([]overviewModule, 0, len(modules))
	for _, module := range modules {
		if allowed[module.ID] {
			filtered = append(filtered, module)
		}
	}
	return filtered
}

func buildOverviewModules(deps Dependencies, listeners []listeningPort, influences []overviewInfluence, coreStatus core.Status) []overviewModule {
	nftTables := listNFTTables()
	dnsPortOwners := selectPortOwners(listeners, 53, deps.Config.Ports.DNS)
	proxyPortOwners := selectPortOwners(listeners, 7890, 7891, 7892, 7893, 7895, deps.Config.Ports.HTTP, deps.Config.Ports.SOCKS, deps.Config.Ports.Redir, deps.Config.Ports.Mixed, deps.Config.Ports.TProxy, deps.Config.Ports.MihomoAPI)
	// IsApplied() is in-memory only; also check the actual kernel table so status
	// survives process restarts when rules were previously applied.
	clashforgeOwned := deps.Netfilter != nil && (deps.Netfilter.IsApplied() || nftTablePresent())
	// dnsManaged reflects actual runtime state: config intent AND core is currently running.
	// When mihomo stops, this becomes false so the UI correctly shows "让 ClashForge 接管".
	dnsManaged := coreStatus.Ready && deps.Config.DNS.Enable && deps.Config.DNS.ApplyOnStart && deps.Config.DNS.DnsmasqMode != "none"
	dnsListenerReady := isDNSPortListening(deps.Config.Ports.DNS)
	stopTargets := collectStopServices(influences, "transparent_proxy", "nft_firewall")
	dnsStopTargets := collectStopServices(influences, "dns_entry")
	transparentOwner := detectTransparentProxyOwner(deps, nftTables, proxyPortOwners, influences)
	nftOwner := detectNFTFirewallOwner(deps, nftTables)
	dnsOwner := detectDNSEntryOwner(deps, dnsPortOwners, influences)
	controlOwner := "ClashForge"
	if !isTCPPortListening(deps.Config.Ports.UI) {
		controlOwner = "未监听"
	}
	subscriptionOwner := "未配置代理源"
	if deps.SubManager != nil && len(deps.SubManager.GetAll()) > 0 {
		subscriptionOwner = "已配置代理源"
	}

	modules := []overviewModule{
		{
			ID:                  "proxy_core",
			Title:               "代理核心",
			Category:            "运行模块",
			Status:              map[bool]string{true: "active", false: "inactive"}[coreStatus.Ready],
			CurrentOwner:        map[bool]string{true: "ClashForge / Mihomo", false: "未运行"}[coreStatus.Ready],
			ManagedByClashforge: coreStatus.Ready,
			Purpose:             "Mihomo 是 ClashForge 的代理引擎，负责规则匹配、出站代理、延迟测试和 API 控制。",
			TakeoverEffect:      "核心运行后，HTTP / SOCKS / Mixed / TProxy 等能力才能真正提供给局域网设备和控制面板。",
			TakeoverSupported:   false,
			Processes:           []overviewProcessRef{{PID: coreStatus.PID, Name: "mihomo", Command: deps.Core.Status().Binary}},
			Notes:               []string{fmt.Sprintf("运行时长 %ds", coreStatus.Uptime)},
		},
		{
			ID:                  "transparent_proxy",
			Title:               "透明代理",
			Category:            "接管模块",
			Status:              moduleStatus(clashforgeOwned, transparentOwner != "无人接管"),
			CurrentOwner:        transparentOwner,
			ManagedByClashforge: clashforgeOwned,
			Purpose:             "透明代理会把局域网设备的普通 TCP / UDP 流量自动导入 ClashForge，不需要每台设备单独手动设置代理。",
			TakeoverEffect:      "接管后，ClashForge 会通过 TProxy / REDIR 规则自动拦截匹配流量，并交给 Mihomo 做分流和出站代理。",
			CurrentMode:         deps.Config.Network.Mode,
			RecommendedMode:     recommendedNetworkMode(deps.Config.Network.Mode),
			TakeoverSupported:   true,
			Action:              moduleAction("transparent_proxy", clashforgeOwned, recommendedNetworkMode(deps.Config.Network.Mode), stopTargets),
			Ports:               proxyPortOwners,
			Processes:           processRefsForModule(influences, "transparent_proxy"),
			Notes:               []string{fmt.Sprintf("当前启动接管=%t", deps.Config.Network.ApplyOnStart), "如果已有其他代理服务占用 789x 端口或安装了自己的 NFT 规则，接管前应先停掉它。"},
		},
		{
			ID:                  "nft_firewall",
			Title:               "NFT / 防火墙",
			Category:            "接管模块",
			Status:              moduleStatus(clashforgeOwned && nftTablePresent(), hasConflictingNFTTables(nftTables)),
			CurrentOwner:        nftOwner,
			ManagedByClashforge: clashforgeOwned && nftTablePresent(),
			Purpose:             "NFT / iptables 是透明代理真正落地的内核规则层，负责把 53、TCP、UDP 流量重定向到 ClashForge。",
			TakeoverEffect:      "接管后，ClashForge 会安装自己的 metaclash 规则表，并把透明代理 / DNS 重定向交给自己管理。",
			CurrentMode:         actualNetfilterBackend(deps),
			RecommendedMode:     actualNetfilterBackend(deps),
			TakeoverSupported:   true,
			Action:              moduleAction("nft_firewall", clashforgeOwned && nftTablePresent(), actualNetfilterBackend(deps), stopTargets),
			Processes:           processRefsForModule(influences, "nft_firewall"),
			Notes:               append([]string{fmt.Sprintf("检测到的 nftables 表: %s", strings.Join(nftTables, ", "))}, firewallNotes(nftTables)...),
		},
		{
			ID:                  "dns_entry",
			Title:               "DNS 入口",
			Category:            "接管模块",
			Status:              moduleStatus(dnsManaged && (dnsListenerReady || deps.Config.DNS.DnsmasqMode == "upstream"), dnsOwner != "无人接管"),
			CurrentOwner:        dnsOwner,
			ManagedByClashforge: dnsManaged,
			Purpose:             "DNS 入口负责接收客户端发到 53 端口的查询，是域名分流、Fake-IP 和广告过滤能否接管的关键入口。",
			TakeoverEffect:      "接管后，客户端 DNS 要么直接进入 ClashForge，要么由 dnsmasq 统一转发给 ClashForge，从而保证域名策略和代理策略一致。",
			CurrentMode:         deps.Config.DNS.DnsmasqMode,
			RecommendedMode:     recommendedDNSMode(deps.Config.DNS.DnsmasqMode),
			TakeoverSupported:   true,
			Action:              moduleAction("dns_entry", dnsManaged, recommendedDNSMode(deps.Config.DNS.DnsmasqMode), dnsStopTargets),
			Ports:               dnsPortOwners,
			Processes:           processRefsForModule(influences, "dns_entry"),
			Notes:               []string{fmt.Sprintf("当前启动接管=%t", deps.Config.DNS.ApplyOnStart), "replace 模式会让 Mihomo 直接接管 DNS；upstream 模式保留 dnsmasq，只把上游切到 ClashForge。"},
		},
		{
			ID:                  "dns_resolver",
			Title:               "DNS 解析引擎",
			Category:            "运行模块",
			Status:              moduleStatus(dnsListenerReady, deps.Config.DNS.Enable),
			CurrentOwner:        map[bool]string{true: "ClashForge / Mihomo", false: "未监听"}[dnsListenerReady],
			ManagedByClashforge: dnsListenerReady,
			Purpose:             "Mihomo 内置 DNS 解析引擎负责 Fake-IP、域名规则命中和远端 DNS 查询。",
			TakeoverEffect:      "启用后，DNS 结果会与代理规则保持一致，避免域名命中和实际流量出口不一致。",
			TakeoverSupported:   true,
			Action:              moduleAction("dns_resolver", dnsListenerReady, recommendedDNSMode(deps.Config.DNS.DnsmasqMode), dnsStopTargets),
			Ports:               selectPortOwners(listeners, deps.Config.Ports.DNS),
			Processes:           []overviewProcessRef{{PID: deps.Core.Status().PID, Name: "mihomo", Command: deps.Core.Status().Binary}},
			Notes:               []string{fmt.Sprintf("DNS 监听端口 %d", deps.Config.Ports.DNS)},
		},
		{
			ID:                  "control_panel",
			Title:               "控制面板 / API",
			Category:            "运行模块",
			Status:              moduleStatus(isTCPPortListening(deps.Config.Ports.UI), false),
			CurrentOwner:        controlOwner,
			ManagedByClashforge: isTCPPortListening(deps.Config.Ports.UI),
			Purpose:             "概览页、设置页、日志页和其他所有操作都通过 ClashForge Web UI 和本地 API 完成。",
			TakeoverEffect:      "这个模块不需要接管其他服务，它只是 ClashForge 自己的控制入口。",
			TakeoverSupported:   false,
			Ports:               selectPortOwners(listeners, deps.Config.Ports.UI, deps.Config.Ports.MihomoAPI),
			Notes:               []string{fmt.Sprintf("UI 端口 %d，Mihomo API 端口 %d", deps.Config.Ports.UI, deps.Config.Ports.MihomoAPI)},
		},
		{
			ID:                  "subscription_sources",
			Title:               "代理源 / 配置来源",
			Category:            "依赖模块",
			Status:              map[bool]string{true: "active", false: "inactive"}[subscriptionOwner != "未配置代理源"],
			CurrentOwner:        subscriptionOwner,
			ManagedByClashforge: subscriptionOwner != "未配置代理源",
			Purpose:             "订阅链接、手动 YAML 覆盖和本地节点列表会决定 Mihomo 是否真的有可用的出站线路。",
			TakeoverEffect:      "配置好代理源后，访问检查、出口 IP 和自动分流才会体现真实代理效果。",
			TakeoverSupported:   false,
			Notes:               []string{"如果这里没有可用节点，访问检查通常会失败，即使透明代理已经成功接管。"},
		},
	}

	return modules
}

func buildOverviewResources(deps Dependencies, coreStatus core.Status) overviewResources {
	systemUsage := sampleSystemUsage()
	metrics := sampleProcessMetrics([]int{os.Getpid(), coreStatus.PID})
	procIDs := []overviewProcessUsage{
		buildProcessUsage("clashforge", os.Getpid(), metrics[os.Getpid()]),
		buildProcessUsage("mihomo", coreStatus.PID, metrics[coreStatus.PID]),
	}
	app := buildAppStorage(deps)

	log.Info().
		Float64("cpu_pct", systemUsage.CPUPercent).
		Float64("mem_used_mb", systemUsage.MemoryUsedMB).
		Float64("mem_total_mb", systemUsage.MemoryTotalMB).
		Float64("mem_pct", systemUsage.MemoryPercent).
		Float64("disk_used_gb", systemUsage.DiskUsedGB).
		Float64("disk_total_gb", systemUsage.DiskTotalGB).
		Float64("disk_pct", systemUsage.DiskPercent).
		Msg("resource_sample system")

	for _, proc := range procIDs {
		e := log.Info().Str("process", proc.ID).Int("pid", proc.PID).Bool("running", proc.Running)
		if proc.Running {
			e = e.Float64("cpu_pct", proc.CPUPercent).Float64("mem_rss_mb", proc.MemoryRSSMB).Int64("uptime_s", proc.Uptime)
		}
		e.Msg("resource_sample process")
	}

	return overviewResources{System: systemUsage, Processes: procIDs, App: app}
}

func buildProcessUsage(id string, pid int, metric procMetricsSample) overviewProcessUsage {
	if pid <= 0 {
		return overviewProcessUsage{ID: id, Name: id, PID: 0, Running: false}
	}
	name := id
	if metric.Process.Name != "" {
		name = metric.Process.Name
	}
	return overviewProcessUsage{
		ID:          id,
		Name:        name,
		PID:         pid,
		Running:     metric.Process.PID > 0,
		CPUPercent:  round1(metric.CPUPercent),
		MemoryRSSMB: bytesToMB(metric.RSSBytes),
		Uptime:      metric.Uptime,
		Command:     metric.Command,
	}
}

func buildAppStorage(deps Dependencies) overviewAppStorage {
	runtimeSize := dirSize(deps.Config.Core.RuntimeDir)
	dataSize := dirSize(deps.Config.Core.DataDir)
	binarySize := fileOrDirSize("/usr/bin/clashforge") + fileOrDirSize(deps.Config.Core.Binary)
	ruleSources := []struct {
		name string
		path string
	}{
		{name: "GeoIP", path: deps.Config.Core.GeoIPPath},
		{name: "Geosite", path: deps.Config.Core.GeositePath},
		{name: "Rule Provider", path: filepath.Join(deps.Config.Core.RuntimeDir, "rule_provider")},
	}
	ruleAssets := make([]overviewRuleAsset, 0, len(ruleSources))
	rulesSize := uint64(0)
	for _, source := range ruleSources {
		size := fileOrDirSize(source.path)
		if size == 0 {
			continue
		}
		rulesSize += size
		ruleAssets = append(ruleAssets, overviewRuleAsset{Name: source.name, Path: source.path, SizeMB: bytesToMB(size)})
	}

	total := runtimeSize + dataSize + binarySize + rulesSize
	return overviewAppStorage{
		RuntimeMB:  bytesToMB(runtimeSize),
		DataMB:     bytesToMB(dataSize),
		BinaryMB:   bytesToMB(binarySize),
		RulesMB:    bytesToMB(rulesSize),
		TotalMB:    bytesToMB(total),
		RuleAssets: ruleAssets,
	}
}

func buildOverviewIPChecks(deps Dependencies) []overviewIPCheck {
	providers := []struct {
		Name  string
		Group string
		URL   string
		GBK   bool
	}{
		{Name: "太平洋", Group: "国内", URL: "https://whois.pconline.com.cn/ipJson.jsp?json=true", GBK: true},
		{Name: "UpaiYun", Group: "国内", URL: "https://pubstatic.b0.upaiyun.com/?_upnode"},
		{Name: "IP.SB", Group: "国外", URL: "https://api.ip.sb/geoip"},
		{Name: "IPInfo", Group: "国外", URL: "https://ipinfo.io/json"},
	}
	batch := shortBatchID()
	snap := captureDNSSnapshot(deps.Config.Ports.DNS)
	logDNSSnapshot(snap, batch, "router")
	result := make([]overviewIPCheck, len(providers))
	var wg sync.WaitGroup
	for index, provider := range providers {
		wg.Add(1)
		go func(i int, spec struct {
			Name, Group, URL string
			GBK              bool
		}) {
			defer wg.Done()
			log.Info().Str("batch", batch).Str("side", "router").Str("provider", spec.Name).Str("url", spec.URL).Msg("ip_check start")
			logResolveResult(resolveForDebug(spec.URL, deps.Config.Ports.DNS), batch, "router")
			start := time.Now()
			check, err := fetchIPCheck(deps, spec.Name, spec.URL, spec.GBK)
			latency := time.Since(start)
			if err != nil {
				log.Info().Str("batch", batch).Str("side", "router").Str("provider", spec.Name).Str("url", spec.URL).Dur("latency", latency).Err(err).Msg("ip_check failed")
				result[i] = overviewIPCheck{Provider: spec.Name, Group: spec.Group, OK: false, Error: err.Error()}
				return
			}
			log.Info().Str("batch", batch).Str("side", "router").Str("provider", spec.Name).Str("url", spec.URL).Str("ip", check.IP).Str("location", check.Location).Dur("latency", latency).Msg("ip_check ok")
			check.Group = spec.Group
			result[i] = check
		}(index, provider)
	}
	wg.Wait()
	return result
}

func buildOverviewAccessChecks(deps Dependencies) []overviewAccessCheck {
	targets := []struct {
		Name        string
		Group       string
		URL         string
		Description string
	}{
		{Name: "淘宝", Group: "国内", URL: "https://www.taobao.com", Description: "用于验证国内主要电商平台的直连可达性。"},
		{Name: "网易云音乐", Group: "国内", URL: "https://music.163.com", Description: "用于验证国内常见内容站点延迟。"},
		{Name: "GitHub", Group: "国外", URL: "https://github.com", Description: "用于验证国际开发站点的代理访问效果。"},
		{Name: "Google", Group: "国外", URL: "https://www.google.com", Description: "用于验证 Google 搜索是否可通过代理访问。"},
		{Name: "OpenAI", Group: "AI", URL: "https://chat.openai.com", Description: "用于验证 ChatGPT / OpenAI 是否可通过代理访问。"},
		{Name: "Claude", Group: "AI", URL: "https://api.anthropic.com", Description: "用于验证 Claude AI 是否可通过代理访问。"},
		{Name: "Gemini", Group: "AI", URL: "https://gemini.google.com", Description: "用于验证 Google Gemini 是否可通过代理访问。"},
	}
	batch := shortBatchID()
	snap := captureDNSSnapshot(deps.Config.Ports.DNS)
	logDNSSnapshot(snap, batch, "router")
	checks := make([]overviewAccessCheck, 0, len(targets))
	for _, target := range targets {
		log.Info().Str("batch", batch).Str("side", "router").Str("name", target.Name).Str("group", target.Group).Str("url", target.URL).Int("proxy_port", deps.Config.Ports.Mixed).Msg("access_check start")

		// Resolve DNS first for diagnostic info
		dnsResult := resolveForDebug(target.URL, deps.Config.Ports.DNS)
		logResolveResult(dnsResult, batch, "router")
		dnsStr := ""
		if len(dnsResult.Addrs) > 0 {
			dnsStr = strings.Join(dnsResult.Addrs, ", ")
			if dnsResult.IsFakeIP {
				dnsStr += " (fake-ip)"
			}
		} else if dnsResult.Err != "" {
			dnsStr = "解析失败: " + dnsResult.Err
		}

		start := time.Now()
		probe := testHTTPProxyEndpoint("mixed", deps.Config.Ports.Mixed, target.URL, deps.Config.Core.RuntimeDir)
		latency := time.Since(start)

		// Determine which stage failed for UI diagnostics
		stage := ""
		if !probe.OK {
			errLower := strings.ToLower(probe.Error)
			if !probe.Listening {
				stage = "proxy_port"
			} else if strings.Contains(errLower, "no such host") || strings.Contains(errLower, "name resolution") || strings.Contains(errLower, "dns") || (dnsResult.Err != "" && !dnsResult.IsFakeIP) {
				stage = "dns"
			} else if strings.Contains(errLower, "deadline exceeded") || strings.Contains(errLower, "timeout") || strings.Contains(errLower, "i/o timeout") {
				stage = "timeout"
			} else {
				stage = "connect"
			}
		}

		if probe.OK {
			log.Info().Str("batch", batch).Str("side", "router").Str("name", target.Name).Str("url", target.URL).Bool("ok", probe.OK).Int("status_code", probe.StatusCode).Int64("latency_ms", latency.Milliseconds()).Str("dns", dnsStr).Msg("access_check ok")
		} else {
			log.Info().Str("batch", batch).Str("side", "router").Str("name", target.Name).Str("url", target.URL).Bool("ok", false).Int("status_code", probe.StatusCode).Int64("latency_ms", latency.Milliseconds()).Str("error", probe.Error).Str("stage", stage).Str("dns", dnsStr).Msg("access_check failed")
		}
		checks = append(checks, overviewAccessCheck{
			Name:        target.Name,
			Group:       target.Group,
			URL:         target.URL,
			Description: target.Description,
			Via:         fmt.Sprintf("通过 ClashForge mixed 端口 %d 检查", deps.Config.Ports.Mixed),
			OK:          probe.OK,
			StatusCode:  probe.StatusCode,
			LatencyMS:   probe.DurationMS,
			Error:       probe.Error,
			Stage:       stage,
			DNSResult:   dnsStr,
		})
	}
	return checks
}

func fetchIPCheck(deps Dependencies, provider, rawURL string, gbk bool) (overviewIPCheck, error) {
	client := overviewProxyClient(deps.Config.Ports.Mixed, deps.Config.Core.RuntimeDir, 6*time.Second)
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return overviewIPCheck{}, err
	}
	req.Header.Set("User-Agent", "clashforge-overview/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return overviewIPCheck{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return overviewIPCheck{}, err
	}
	// Decode GBK to UTF-8 if needed (e.g. pconline)
	if gbk {
		decoded, _, decErr := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), body)
		if decErr == nil {
			body = decoded
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err == nil {
		ip, location := extractIPLocation(payload)
		if ip == "" {
			return overviewIPCheck{}, fmt.Errorf("provider returned no ip")
		}
		return overviewIPCheck{Provider: provider, OK: true, IP: ip, Location: location}, nil
	}

	// Plain-text fallback — handles IPIP.NET format:
	// "当前 IP：1.2.3.4 来自于：中国 北京 ..."
	text := strings.TrimSpace(string(body))
	if strings.Contains(text, "当前 IP：") {
		parts := strings.SplitN(text, "当前 IP：", 2)
		if len(parts) == 2 {
			rest := parts[1]
			location := ""
			if idx := strings.Index(rest, " 来自于："); idx != -1 {
				location = strings.TrimSpace(rest[idx+len(" 来自于："):])
				rest = strings.TrimSpace(rest[:idx])
			}
			if rest != "" {
				return overviewIPCheck{Provider: provider, OK: true, IP: rest, Location: location}, nil
			}
		}
	}
	// Generic single-line plain IP
	if text == "" {
		return overviewIPCheck{}, fmt.Errorf("provider returned empty response")
	}
	return overviewIPCheck{Provider: provider, OK: true, IP: text}, nil
}

func extractIPLocation(payload map[string]any) (string, string) {
	// Handle UpaiYun format: {"remote_addr":"...","remote_addr_location":{"country":"中国","province":"北京","city":"北京","isp":"电信"}}
	if ra, ok := payload["remote_addr"].(string); ok && strings.TrimSpace(ra) != "" {
		location := ""
		if loc, ok := payload["remote_addr_location"].(map[string]any); ok {
			parts := make([]string, 0, 4)
			for _, key := range []string{"country", "province", "city", "isp"} {
				if v, ok := loc[key].(string); ok && strings.TrimSpace(v) != "" {
					parts = append(parts, strings.TrimSpace(v))
				}
			}
			location = strings.Join(parts, " · ")
		}
		return ra, location
	}

	stringValue := func(keys ...string) string {
		for _, key := range keys {
			if value, ok := payload[key]; ok {
				if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
					return strings.TrimSpace(text)
				}
			}
		}
		return ""
	}
	// pconline: {"ip":"...","pro":"北京市","city":"北京市","addr":"北京市 联通","err":""}
	// ip.sb:    {"ip":"...","country":"China","city":"Beijing","isp":"..."}
	// ipinfo.io: {"ip":"...","city":"...","region":"...","org":"..."}
	ip := stringValue("ip", "query")
	// pconline provides a pre-formatted "addr" field — use it directly
	if addr := stringValue("addr"); addr != "" && stringValue("pro") != "" {
		return ip, addr
	}
	parts := []string{
		stringValue("city"),
		stringValue("pro", "region", "regionName", "region_name"),
		stringValue("country_name", "country"),
		stringValue("isp", "organization", "org"),
	}
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	return ip, strings.Join(filtered, " · ")
}

// readMihomoProxyCreds reads the first authentication entry from the generated
// mihomo config (format "user:pass") so that internal probe clients can
// authenticate against the mixed/http ports when authentication is configured.
func readMihomoProxyCreds(runtimeDir string) (user, pass string) {
	path := runtimeDir + "/mihomo-config.yaml"
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var parsed struct {
		Authentication []string `yaml:"authentication"`
	}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return
	}
	if len(parsed.Authentication) == 0 {
		return
	}
	parts := strings.SplitN(parsed.Authentication[0], ":", 2)
	if len(parts) == 2 {
		user, pass = parts[0], parts[1]
	}
	return
}

// mihomoProxyURL builds http://[user:pass@]127.0.0.1:port — credentials are
// injected only when the generated mihomo config has authentication set.
func mihomoProxyURL(port int, runtimeDir string) *url.URL {
	u := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("127.0.0.1:%d", port),
	}
	if runtimeDir != "" {
		user, pass := readMihomoProxyCreds(runtimeDir)
		if user != "" {
			u.User = url.UserPassword(user, pass)
		}
	}
	return u
}

func overviewProxyClient(port int, runtimeDir string, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(mihomoProxyURL(port, runtimeDir)),
		},
	}
}

func detectInfluences(listeners []listeningPort) []overviewInfluence {
	processes := listSystemProcesses()
	result := make([]overviewInfluence, 0)
	for _, spec := range knownInfluenceSpecs {
		processRefs := matchInfluenceProcesses(spec, processes)
		portOwners := matchInfluencePorts(spec, listeners)
		running := len(processRefs) > 0 || len(portOwners) > 0
		service := firstExistingService(spec.Services)
		if !running && service == "" {
			continue
		}
		result = append(result, overviewInfluence{
			ID:          spec.ID,
			Name:        spec.Name,
			Description: spec.Description,
			Affects:     append([]string(nil), spec.Affects...),
			Running:     running,
			Stoppable:   spec.Stoppable && service != "",
			Service:     service,
			Processes:   processRefs,
			Ports:       portOwners,
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Running == result[j].Running {
			return result[i].Name < result[j].Name
		}
		return result[i].Running
	})
	return result
}

func matchInfluenceProcesses(spec influenceSpec, processes []overviewProcessRef) []overviewProcessRef {
	matches := make([]overviewProcessRef, 0)
	for _, process := range processes {
		text := strings.ToLower(process.Name + " " + process.Command)
		if !containsAny(text, spec.Match) || containsAny(text, spec.Exclude) {
			continue
		}
		matches = append(matches, process)
	}
	return dedupeProcessRefs(matches)
}

func matchInfluencePorts(spec influenceSpec, listeners []listeningPort) []overviewPortOwner {
	ports := make([]overviewPortOwner, 0)
	for _, listener := range listeners {
		if !intInSlice(listener.Port, spec.Ports) {
			continue
		}
		text := strings.ToLower(listener.Name + " " + listener.Command)
		if containsAny(text, spec.Match) && !containsAny(text, spec.Exclude) {
			ports = append(ports, toPortOwner(listener))
		}
	}
	return dedupePortOwners(ports)
}

func collectStopServices(influences []overviewInfluence, modules ...string) []string {
	services := make([]string, 0)
	for _, influence := range influences {
		if !influence.Running || !influence.Stoppable || influence.Service == "" {
			continue
		}
		for _, affect := range influence.Affects {
			if stringInSlice(affect, modules) {
				services = append(services, influence.Service)
				break
			}
		}
	}
	return dedupeStrings(services)
}

func detectTransparentProxyOwner(deps Dependencies, nftTables []string, owners []overviewPortOwner, influences []overviewInfluence) string {
	// Check in-memory flag first, then fall back to kernel table presence.
	if deps.Netfilter != nil && (deps.Netfilter.IsApplied() || nftTablePresent()) {
		return "ClashForge"
	}
	for _, influence := range influences {
		if influence.Running && stringInSlice("transparent_proxy", influence.Affects) {
			return influence.Name
		}
	}
	// Mihomo is ClashForge's own subprocess – listening on proxy ports is expected
	// and should not be reported as a conflicting owner of transparent proxy.
	for _, owner := range owners {
		lower := strings.ToLower(owner.Owner)
		if strings.Contains(lower, "mihomo") || strings.Contains(lower, "clashforg") {
			continue
		}
		return owner.Owner
	}
	if hasConflictingNFTTables(nftTables) {
		return "系统防火墙 / 其他 nft 规则"
	}
	return "无人接管"
}

func detectNFTFirewallOwner(deps Dependencies, nftTables []string) string {
	// Also check kernel table so ownership is detected after process restarts.
	if nftTablePresent() {
		return "ClashForge"
	}
	if len(nftTables) == 0 {
		return "未检测到 nftables 表"
	}
	owners := make([]string, 0)
	for _, table := range nftTables {
		lower := strings.ToLower(table)
		switch {
		case strings.Contains(lower, "metaclash"):
			owners = append(owners, "ClashForge")
		case strings.Contains(lower, "openclash"):
			owners = append(owners, "OpenClash")
		case strings.Contains(lower, "fw4"), strings.Contains(lower, "dnsmasq"):
			// System-default OpenWrt tables – not a conflict with ClashForge.
			continue
		default:
			owners = append(owners, table)
		}
	}
	if len(owners) == 0 {
		return "系统防火墙（正常）"
	}
	return strings.Join(dedupeStrings(owners), " / ")
}

// hasConflictingNFTTables returns true only if there are nftables tables that
// would genuinely compete with ClashForge. System-default OpenWrt tables
// (fw4, dnsmasq) and ClashForge's own metaclash table are excluded.
func hasConflictingNFTTables(tables []string) bool {
	for _, t := range tables {
		lower := strings.ToLower(t)
		if strings.Contains(lower, "fw4") ||
			strings.Contains(lower, "dnsmasq") ||
			strings.Contains(lower, "metaclash") {
			continue
		}
		return true
	}
	return false
}

func detectDNSEntryOwner(deps Dependencies, owners []overviewPortOwner, influences []overviewInfluence) string {
	if deps.Config.DNS.ApplyOnStart && deps.Config.DNS.DnsmasqMode != "none" {
		if deps.Config.DNS.DnsmasqMode == "replace" {
			return "ClashForge（通过 dnsmasq 释放 53 端口）"
		}
		return "ClashForge（通过 dnsmasq 上游转发）"
	}
	for _, influence := range influences {
		if influence.Running && stringInSlice("dns_entry", influence.Affects) {
			return influence.Name
		}
	}
	if len(owners) > 0 {
		return owners[0].Owner
	}
	return "无人接管"
}

func firewallNotes(tables []string) []string {
	notes := make([]string, 0)
	for _, table := range tables {
		lower := strings.ToLower(table)
		switch {
		case strings.Contains(lower, "fw4"):
			notes = append(notes, "系统自带的 fw4 规则仍会保留，这是正常的基础防火墙行为。")
		case strings.Contains(lower, "openclash"):
			notes = append(notes, "检测到 OpenClash 的规则表，如果要完全交给 ClashForge，应先停止 OpenClash。")
		}
	}
	return dedupeStrings(notes)
}

func moduleStatus(managedByClashforge bool, occupied bool) string {
	if managedByClashforge {
		return "active"
	}
	if occupied {
		return "conflict"
	}
	return "available"
}

func recommendedNetworkMode(current string) string {
	if current == "redir" || current == "tproxy" {
		return current
	}
	return "tproxy"
}

func recommendedDNSMode(current string) string {
	if current == "replace" || current == "upstream" {
		return current
	}
	return "replace"
}

func moduleAction(module string, managed bool, mode string, stopServices []string) *overviewAction {
	if managed {
		return &overviewAction{Module: module, Label: "已由 ClashForge 接管", Mode: mode, StopServices: stopServices}
	}
	label := "让 ClashForge 接管"
	if len(stopServices) > 0 {
		label = "停止冲突服务并接管"
	}
	return &overviewAction{Module: module, Label: label, Mode: mode, StopServices: stopServices}
}

func processRefsForModule(influences []overviewInfluence, module string) []overviewProcessRef {
	refs := make([]overviewProcessRef, 0)
	for _, influence := range influences {
		if !stringInSlice(module, influence.Affects) {
			continue
		}
		refs = append(refs, influence.Processes...)
	}
	return dedupeProcessRefs(refs)
}

func ensureTakeoverAllowed(deps Dependencies, module string) error {
	switch module {
	case "transparent_proxy", "nft_firewall", "dns_entry", "dns_resolver", "all":
	default:
		return nil
	}

	if !deps.Core.Status().Ready {
		return fmt.Errorf("核心尚未健康运行，无法接管系统服务；请先启动核心")
	}

	if (module == "dns_entry" || module == "dns_resolver") && !deps.Config.DNS.Enable {
		return fmt.Errorf("当前 dns.enable=false，未启用 DNS 引擎；请先在配置中启用 DNS 再执行 DNS 接管")
	}

	return nil
}

func anyIPCheckOK(checks []overviewIPCheck) bool {
	for _, check := range checks {
		if check.OK {
			return true
		}
	}
	return false
}

func failedAccessCheckNames(checks []overviewAccessCheck) []string {
	failed := make([]string, 0)
	for _, check := range checks {
		if !check.OK {
			failed = append(failed, check.Name)
		}
	}
	return failed
}

func takeoverTransparentProxy(deps Dependencies, mode string) (string, bool, error) {
	if mode == "" {
		mode = recommendedNetworkMode(deps.Config.Network.Mode)
	}
	deps.Config.Network.Mode = mode
	deps.Config.Network.ApplyOnStart = true
	if deps.Config.Network.FirewallBackend == "none" || strings.TrimSpace(deps.Config.Network.FirewallBackend) == "" {
		deps.Config.Network.FirewallBackend = "auto"
	}
	if err := saveRuntimeConfig(deps); err != nil {
		return "", false, err
	}
	refreshNetfilterManager(deps)
	if deps.Core.Status().Ready {
		if err := deps.Netfilter.Apply(); err != nil {
			return "", false, err
		}
		return fmt.Sprintf("ClashForge 已开始以 %s 模式接管透明代理和防火墙规则", mode), false, nil
	}
	return fmt.Sprintf("已保存透明代理接管配置，启动 Mihomo 核心后会按 %s 模式生效", mode), true, nil
}

func takeoverDNS(deps Dependencies, mode string) (string, bool, error) {
	if !deps.Config.DNS.Enable {
		return "", false, fmt.Errorf("dns.enable=false，当前配置未启用 DNS；请先在配置中启用 DNS")
	}

	oldMode := dns.DnsmasqMode(deps.Config.DNS.DnsmasqMode)
	if mode == "" {
		mode = recommendedDNSMode(deps.Config.DNS.DnsmasqMode)
	}
	deps.Config.DNS.ApplyOnStart = true
	deps.Config.DNS.DnsmasqMode = mode
	if err := saveRuntimeConfig(deps); err != nil {
		return "", false, err
	}
	if _, err := generateMihomoConfig(deps); err != nil {
		return "", false, err
	}
	if deps.Core.Status().Ready {
		if oldMode != dns.ModeNone {
			_ = dns.Restore(oldMode)
		}
		if deps.Config.DNS.Enable {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := deps.Core.Restart(ctx); err != nil {
				return "", false, err
			}
		}
		if err := dns.Setup(dns.DnsmasqMode(mode), deps.Config.Ports.DNS); err != nil {
			return "", false, err
		}
		return fmt.Sprintf("ClashForge 已开始以 %s 模式接管 DNS 入口", mode), false, nil
	}
	return fmt.Sprintf("已保存 DNS 接管配置，启动 Mihomo 核心后会以 %s 模式生效", mode), true, nil
}

func takeoverAll(deps Dependencies) (string, bool, error) {
	networkMode := recommendedNetworkMode(deps.Config.Network.Mode)
	dnsMode := recommendedDNSMode(deps.Config.DNS.DnsmasqMode)
	oldDNSMode := dns.DnsmasqMode(deps.Config.DNS.DnsmasqMode)
	dnsEnabled := deps.Config.DNS.Enable

	deps.Config.Network.Mode = networkMode
	deps.Config.Network.ApplyOnStart = true
	if deps.Config.Network.FirewallBackend == "none" || strings.TrimSpace(deps.Config.Network.FirewallBackend) == "" {
		deps.Config.Network.FirewallBackend = "auto"
	}
	if dnsEnabled {
		deps.Config.DNS.ApplyOnStart = true
		deps.Config.DNS.DnsmasqMode = dnsMode
	}

	if err := saveRuntimeConfig(deps); err != nil {
		return "", false, err
	}
	if _, err := generateMihomoConfig(deps); err != nil {
		return "", false, err
	}

	refreshNetfilterManager(deps)
	coreWasRunning := deps.Core.Status().Ready
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if coreWasRunning {
		if dnsEnabled && oldDNSMode != dns.ModeNone {
			_ = dns.Restore(oldDNSMode)
		}
		if err := deps.Core.Restart(ctx); err != nil {
			return "", false, err
		}
	} else if err := deps.Core.Start(ctx); err != nil {
		if err != core.ErrAlreadyRunning {
			return "", false, err
		}
	}

	if dnsEnabled {
		if err := dns.Setup(dns.DnsmasqMode(dnsMode), deps.Config.Ports.DNS); err != nil {
			return "", false, err
		}
	}
	if deps.Netfilter != nil {
		if err := deps.Netfilter.Apply(); err != nil {
			return "", false, err
		}
	}

	if !dnsEnabled {
		return "核心已启动，并已尝试接管透明代理与 NFT 防火墙；当前 dns.enable=false，已跳过 DNS 接管", false, nil
	}

	return "核心已启动，并已尝试接管透明代理、NFT 防火墙与 DNS 子模块", false, nil
}

func releaseAllTakeover(deps Dependencies) (string, error) {
	// Release system-level takeover modules.
	// Do NOT touch ApplyOnStart flags — they represent the user's intent and
	// must remain sticky so that restarting the core re-applies the same setup.

	if deps.Netfilter != nil {
		_ = deps.Netfilter.Cleanup()
	}

	// Force cleanup even if manager state is out of sync.
	_ = (&netfilter.NftablesBackend{}).Cleanup()
	for _, dnsPort := range dedupeInts([]int{deps.Config.Ports.DNS, 17874, 7874}) {
		if dnsPort <= 0 {
			continue
		}
		_ = (&netfilter.IptablesBackend{DNSPort: dnsPort}).Cleanup()
	}

	for _, mode := range []dns.DnsmasqMode{dns.ModeReplace, dns.ModeUpstream} {
		if err := dns.Restore(mode); err != nil {
			return "", err
		}
	}

	if deps.Core.Status().Ready {
		return "已停止 ClashForge 对系统模块的接管（DNS / NFT / 透明代理），核心保持运行", nil
	}
	return "已停止 ClashForge 对系统模块的接管，核心当前未运行", nil
}

func saveRuntimeConfig(deps Dependencies) error {
	if err := config.Save(deps.ConfigPath, deps.Config); err != nil {
		return err
	}
	return nil
}

func refreshNetfilterManager(deps Dependencies) {
	if deps.Netfilter == nil {
		return
	}
	*deps.Netfilter = *netfilter.NewManager(netfilter.Config{
		Mode:              deps.Config.Network.Mode,
		FirewallBackend:   deps.Config.Network.FirewallBackend,
		TProxyPort:        deps.Config.Ports.TProxy,
		DNSPort:           deps.Config.Ports.DNS,
		EnableDNSRedirect: shouldRedirectDNS(deps.Config),
		BypassFakeIP:      shouldBypassFakeIP(deps.Config),
		BypassCIDR:        deps.Config.Network.BypassCIDR,
		EnableIPv6:        deps.Config.Network.IPv6,
	})
}

func shouldRedirectDNS(cfg *config.MetaclashConfig) bool {
	if cfg == nil {
		return false
	}
	if !cfg.DNS.Enable || !cfg.DNS.ApplyOnStart {
		return false
	}
	// Only redirect port-53 via nftables/iptables in replace mode.
	// In upstream mode dnsmasq stays on port 53 and forwards to mihomo itself;
	// adding an NFT redirect would bypass dnsmasq's local-hostname resolution.
	mode := strings.ToLower(strings.TrimSpace(cfg.DNS.DnsmasqMode))
	return mode == "replace"
}

func shouldBypassFakeIP(cfg *config.MetaclashConfig) bool {
	if cfg == nil {
		return true
	}
	if !cfg.DNS.Enable || !cfg.DNS.ApplyOnStart {
		return true
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.DNS.Mode))
	return mode != "fake-ip"
}

func stopAllowedServices(requested []string) ([]string, error) {
	allowed := map[string]bool{
		"openclash":   true,
		"smartdns":    true,
		"mosdns":      true,
		"adguardhome": true,
		"AdGuardHome": true,
		"sing-box":    true,
		"singbox":     true,
		"xray":        true,
		"v2ray":       true,
	}
	stopped := make([]string, 0)
	for _, service := range dedupeStrings(requested) {
		if !allowed[service] {
			continue
		}
		script := filepath.Clean("/etc/init.d/" + service)
		if !strings.HasPrefix(script, "/etc/init.d/") {
			continue
		}
		if _, err := os.Stat(script); err != nil {
			continue
		}
		if out, err := exec.Command(script, "stop").CombinedOutput(); err != nil {
			return stopped, fmt.Errorf("stop %s: %w: %s", service, err, string(out))
		}
		stopped = append(stopped, service)
	}
	return stopped, nil
}

func listListeningPorts() []listeningPort {
	out, err := exec.Command("netstat", "-lntup").CombinedOutput()
	if err != nil {
		return nil
	}
	listeners := make([]listeningPort, 0)
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 6 {
			continue
		}
		proto := strings.ToLower(fields[0])
		if !strings.HasPrefix(proto, "tcp") && !strings.HasPrefix(proto, "udp") {
			continue
		}
		localAddress := fields[3]
		processField := fields[len(fields)-1]
		if proto == "tcp" && len(fields) >= 7 {
			processField = fields[6]
		}
		port := parseAddressPort(localAddress)
		if port <= 0 {
			continue
		}
		pid, name := parseProcessField(processField)
		command := processCommand(pid)
		listeners = append(listeners, listeningPort{Port: port, Proto: proto[:3], PID: pid, Name: name, Command: command})
	}
	return listeners
}

func parseAddressPort(address string) int {
	idx := strings.LastIndex(address, ":")
	if idx < 0 || idx+1 >= len(address) {
		return 0
	}
	port, _ := strconv.Atoi(address[idx+1:])
	return port
}

func parseProcessField(field string) (int, string) {
	parts := strings.SplitN(strings.TrimSpace(field), "/", 2)
	if len(parts) != 2 {
		return 0, strings.TrimSpace(field)
	}
	pid, _ := strconv.Atoi(parts[0])
	return pid, strings.TrimSpace(parts[1])
}

func processCommand(pid int) string {
	if pid <= 0 {
		return ""
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return ""
	}
	command := strings.ReplaceAll(string(data), "\x00", " ")
	return strings.TrimSpace(command)
}

func listSystemProcesses() []overviewProcessRef {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	processes := make([]overviewProcessRef, 0)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
		if err != nil {
			continue
		}
		command := processCommand(pid)
		processes = append(processes, overviewProcessRef{PID: pid, Name: strings.TrimSpace(string(comm)), Command: command, Service: guessServiceName(command, strings.TrimSpace(string(comm)))})
	}
	return processes
}

func guessServiceName(command string, name string) string {
	joined := strings.ToLower(name + " " + command)
	for _, spec := range knownInfluenceSpecs {
		if containsAny(joined, spec.Match) && !containsAny(joined, spec.Exclude) {
			return firstExistingService(spec.Services)
		}
	}
	return ""
}

func firstExistingService(candidates []string) string {
	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Clean("/etc/init.d/" + candidate)); err == nil {
			return candidate
		}
	}
	return ""
}

func listNFTTables() []string {
	out, err := exec.Command("nft", "list", "tables").CombinedOutput()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	tables := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			tables = append(tables, line)
		}
	}
	return dedupeStrings(tables)
}

func sampleSystemUsage() overviewSystemUsage {
	startIdle, startTotal, _ := readCPUTicks()
	time.Sleep(120 * time.Millisecond)
	endIdle, endTotal, _ := readCPUTicks()
	busyPercent := 0.0
	if endTotal > startTotal {
		busyPercent = 100 * (1 - float64(endIdle-startIdle)/float64(endTotal-startTotal))
	}
	totalMem, availableMem := readMemoryInfo()
	usedMem := uint64(0)
	if totalMem > availableMem {
		usedMem = totalMem - availableMem
	}
	totalDisk, usedDisk := readDiskUsage("/")
	memoryPercent := 0.0
	diskPercent := 0.0
	if totalMem > 0 {
		memoryPercent = float64(usedMem) / float64(totalMem) * 100
	}
	if totalDisk > 0 {
		diskPercent = float64(usedDisk) / float64(totalDisk) * 100
	}
	return overviewSystemUsage{
		CPUPercent:    round1(busyPercent),
		MemoryTotalMB: bytesToMB(totalMem),
		MemoryUsedMB:  bytesToMB(usedMem),
		MemoryPercent: round1(memoryPercent),
		DiskTotalGB:   bytesToGB(totalDisk),
		DiskUsedGB:    bytesToGB(usedDisk),
		DiskPercent:   round1(diskPercent),
	}
}

func readCPUTicks() (uint64, uint64, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, err
	}
	line := strings.SplitN(string(data), "\n", 2)[0]
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return 0, 0, fmt.Errorf("unexpected /proc/stat format")
	}
	var total uint64
	for _, field := range fields[1:] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			continue
		}
		total += value
	}
	idle, _ := strconv.ParseUint(fields[4], 10, 64)
	return idle, total, nil
}

func readMemoryInfo() (uint64, uint64) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	var total uint64
	var available uint64
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		value, _ := strconv.ParseUint(parts[1], 10, 64)
		switch strings.TrimSuffix(parts[0], ":") {
		case "MemTotal":
			total = value * 1024
		case "MemAvailable":
			available = value * 1024
		case "MemFree":
			if available == 0 {
				available = value * 1024
			}
		}
	}
	return total, available
}

func readDiskUsage(path string) (uint64, uint64) {
	out, err := exec.Command("df", "-k", path).CombinedOutput()
	if err != nil {
		return 0, 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return 0, 0
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 3 {
		return 0, 0
	}
	total, _ := strconv.ParseUint(fields[1], 10, 64)
	used, _ := strconv.ParseUint(fields[2], 10, 64)
	return total * 1024, used * 1024
}

func sampleProcessMetrics(pids []int) map[int]procMetricsSample {
	_, startTotalTicks, _ := readCPUTicks()
	start := make(map[int]procMetricsSample)
	for _, pid := range pids {
		start[pid] = readProcMetrics(pid)
	}
	time.Sleep(120 * time.Millisecond)
	_, endTotalTicks, _ := readCPUTicks()
	totalDelta := endTotalTicks - startTotalTicks
	end := make(map[int]procMetricsSample)
	for _, pid := range pids {
		endMetrics := readProcMetrics(pid)
		if totalDelta > 0 && endMetrics.Process.PID > 0 && start[pid].Process.PID > 0 {
			procDelta := endMetrics.Ticks - start[pid].Ticks
			endMetrics.CPUPercent = float64(procDelta) / float64(totalDelta) * 100
		}
		end[pid] = endMetrics
	}
	return end
}

func readProcMetrics(pid int) procMetricsSample {
	metrics := procMetricsSample{}
	if pid <= 0 {
		return metrics
	}
	statData, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return metrics
	}
	text := string(statData)
	lParen := strings.Index(text, "(")
	rParen := strings.LastIndex(text, ")")
	if lParen < 0 || rParen <= lParen || rParen+2 >= len(text) {
		return metrics
	}
	name := text[lParen+1 : rParen]
	fields := strings.Fields(text[rParen+2:])
	if len(fields) < 22 {
		return metrics
	}
	utime, _ := strconv.ParseUint(fields[11], 10, 64)
	stime, _ := strconv.ParseUint(fields[12], 10, 64)
	rssPages, _ := strconv.ParseInt(fields[21], 10, 64)
	uptime := processUptimeSeconds(fields[19])
	command := processCommand(pid)
	metrics.Ticks = utime + stime
	metrics.RSSBytes = uint64(maxInt64(rssPages, 0)) * uint64(os.Getpagesize())
	metrics.Uptime = uptime
	metrics.Command = command
	metrics.Process = overviewProcessRef{PID: pid, Name: name, Command: command, Service: guessServiceName(command, name)}
	return metrics
}

func processUptimeSeconds(startTimeField string) int64 {
	startTicks, err := strconv.ParseUint(startTimeField, 10, 64)
	if err != nil {
		return 0
	}
	uptimeData, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	parts := strings.Fields(string(uptimeData))
	if len(parts) == 0 {
		return 0
	}
	systemUptime, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0
	}
	clockTicks := float64(systemClockTicks())
	if clockTicks <= 0 {
		clockTicks = 100
	}
	seconds := systemUptime - float64(startTicks)/clockTicks
	if seconds < 0 {
		return 0
	}
	return int64(seconds)
}

var clockTicksCache sync.Once
var clockTicksValue uint64 = 100

func systemClockTicks() uint64 {
	clockTicksCache.Do(func() {
		out, err := exec.Command("getconf", "CLK_TCK").CombinedOutput()
		if err != nil {
			return
		}
		if value, parseErr := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64); parseErr == nil && value > 0 {
			clockTicksValue = value
		}
	})
	return clockTicksValue
}

func selectPortOwners(listeners []listeningPort, ports ...int) []overviewPortOwner {
	selected := make([]overviewPortOwner, 0)
	for _, listener := range listeners {
		if intInSlice(listener.Port, ports) {
			selected = append(selected, toPortOwner(listener))
		}
	}
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].Port == selected[j].Port {
			return selected[i].Proto < selected[j].Proto
		}
		return selected[i].Port < selected[j].Port
	})
	return dedupePortOwners(selected)
}

func toPortOwner(listener listeningPort) overviewPortOwner {
	owner := listener.Name
	if owner == "" {
		owner = "unknown"
	}
	return overviewPortOwner{Port: listener.Port, Proto: listener.Proto, Owner: owner, PID: listener.PID, Command: listener.Command}
}

func dirSize(path string) uint64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	if !info.IsDir() {
		return uint64(info.Size())
	}
	var total uint64
	_ = filepath.Walk(path, func(_ string, fileInfo os.FileInfo, walkErr error) error {
		if walkErr != nil || fileInfo == nil || fileInfo.IsDir() {
			return nil
		}
		total += uint64(fileInfo.Size())
		return nil
	})
	return total
}

func fileOrDirSize(path string) uint64 {
	return dirSize(path)
}

func dedupeStrings(items []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}

func dedupePortOwners(items []overviewPortOwner) []overviewPortOwner {
	seen := make(map[string]bool)
	result := make([]overviewPortOwner, 0, len(items))
	for _, item := range items {
		key := fmt.Sprintf("%s-%d-%d-%s", item.Proto, item.Port, item.PID, item.Owner)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, item)
	}
	return result
}

func dedupeProcessRefs(items []overviewProcessRef) []overviewProcessRef {
	seen := make(map[int]bool)
	result := make([]overviewProcessRef, 0, len(items))
	for _, item := range items {
		if item.PID <= 0 || seen[item.PID] {
			continue
		}
		seen[item.PID] = true
		result = append(result, item)
	}
	return result
}

func containsAny(text string, patterns []string) bool {
	text = strings.ToLower(text)
	for _, pattern := range patterns {
		if pattern != "" && strings.Contains(text, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

func intInSlice(value int, list []int) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func stringInSlice(value string, list []string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func bytesToMB(value uint64) float64 {
	return round1(float64(value) / 1024.0 / 1024.0)
}

func bytesToGB(value uint64) float64 {
	return round1(float64(value) / 1024.0 / 1024.0 / 1024.0)
}

func round1(value float64) float64 {
	return mathRound(value*10) / 10
}

func mathRound(value float64) float64 {
	if value < 0 {
		return float64(int64(value - 0.5))
	}
	return float64(int64(value + 0.5))
}

func maxInt64(value int64, fallback int64) int64 {
	if value < fallback {
		return fallback
	}
	return value
}

// ─── DNS diagnostic helpers ──────────────────────────────────────────────────

type dnsSnapshot struct {
	Port53UDP         bool
	Port53TCP         bool
	MihomoPort        int
	MihomoListening   bool
	ManagedFilePresent bool
	UCIDnsmasqPort    string
}

type dnsResolveResult struct {
	Host      string
	Addrs     []string
	IsFakeIP  bool
	LatencyMS int64
	Err       string
}

// captureDNSSnapshot collects the current DNS stack state. Reuses helpers from
// handler_health.go which live in the same package.
func captureDNSSnapshot(mihomoPort int) dnsSnapshot {
	snap := dnsSnapshot{
		Port53UDP:  isUDPPortListening(53),
		Port53TCP:  isTCPPortListening(53),
		MihomoPort: mihomoPort,
	}
	if mihomoPort > 0 {
		snap.MihomoListening = isDNSPortListening(mihomoPort)
	}
	snap.ManagedFilePresent = fileExists("/etc/dnsmasq.d/clashforge.conf")
	if out, err := exec.Command("uci", "get", "dhcp.@dnsmasq[0].port").Output(); err == nil {
		snap.UCIDnsmasqPort = strings.TrimSpace(string(out))
	}
	return snap
}

// logDNSSnapshot emits a single structured log line for the current DNS state.
func logDNSSnapshot(snap dnsSnapshot, batch, side string) {
	log.Info().
		Str("batch", batch).
		Str("side", side).
		Bool("port53_udp", snap.Port53UDP).
		Bool("port53_tcp", snap.Port53TCP).
		Bool("mihomo_dns_listening", snap.MihomoListening).
		Int("mihomo_dns_port", snap.MihomoPort).
		Bool("clashforge_dnsmasq_conf", snap.ManagedFilePresent).
		Str("uci_dnsmasq_port", snap.UCIDnsmasqPort).
		Msg("dns_snapshot")
}

// resolveForDebug resolves the hostname extracted from rawURL by querying
// mihomo's DNS port directly (127.0.0.1:dnsPort). This avoids false "connection
// refused" errors in replace mode where the system resolver at [::1]:53 has
// nothing listening (dnsmasq disabled, nft redirect only covers non-loopback).
// Checks whether any returned address falls in the mihomo fake-IP range 198.18.0.0/15.
func resolveForDebug(rawURL string, dnsPort int) dnsResolveResult {
	u, err := url.Parse(rawURL)
	if err != nil {
		return dnsResolveResult{Host: rawURL, Err: "url parse: " + err.Error()}
	}
	host := u.Hostname()
	if host == "" {
		return dnsResolveResult{Host: rawURL, Err: "no host in url"}
	}
	// Skip IP literals — nothing to resolve.
	if net.ParseIP(host) != nil {
		return dnsResolveResult{Host: host, Addrs: []string{host}}
	}
	var resolver *net.Resolver
	if dnsPort > 0 {
		dnsAddr := fmt.Sprintf("127.0.0.1:%d", dnsPort)
		resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "udp", dnsAddr)
			},
		}
	} else {
		resolver = net.DefaultResolver
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	start := time.Now()
	addrs, err := resolver.LookupHost(ctx, host)
	latency := time.Since(start)
	if err != nil {
		return dnsResolveResult{Host: host, LatencyMS: latency.Milliseconds(), Err: err.Error()}
	}
	fakeIP := false
	for _, a := range addrs {
		if isMihomoFakeIP(a) {
			fakeIP = true
			break
		}
	}
	return dnsResolveResult{Host: host, Addrs: addrs, IsFakeIP: fakeIP, LatencyMS: latency.Milliseconds()}
}

// logResolveResult emits a structured log line for one DNS resolution attempt.
func logResolveResult(res dnsResolveResult, batch, side string) {
	e := log.Info().
		Str("batch", batch).
		Str("side", side).
		Str("host", res.Host).
		Strs("addrs", res.Addrs).
		Bool("fake_ip", res.IsFakeIP).
		Int64("dns_latency_ms", res.LatencyMS)
	if res.Err != "" {
		e = e.Str("error", res.Err)
	}
	e.Msg("dns_resolve")
}

// shortBatchID returns a compact 6-char hex identifier for grouping probe log lines.
func shortBatchID() string {
	return fmt.Sprintf("%06x", time.Now().UnixNano()&0xFFFFFF)
}

// isMihomoFakeIP reports whether addr falls in the default mihomo fake-IP
// range 198.18.0.0/15 (covers 198.18.x.x and 198.19.x.x).
func isMihomoFakeIP(addr string) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19)
	}
	return false
}

func dedupeInts(items []int) []int {
	seen := make(map[int]bool)
	result := make([]int, 0, len(items))
	for _, item := range items {
		if seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}
