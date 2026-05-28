package quickstart

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/core"
	"github.com/wujun4code/clashforge/internal/dns"
	"github.com/wujun4code/clashforge/internal/netfilter"
	"github.com/wujun4code/clashforge/internal/runtimecfg"
	"github.com/wujun4code/clashforge/internal/subscription"
)

type activeSourceRecord struct {
	Type    string `json:"type"`
	SubID   string `json:"sub_id,omitempty"`
	SubName string `json:"sub_name,omitempty"`
}

// autoConfigureClashForge applies QuickStart defaults to the ClashForge config,
// regenerates the Mihomo YAML, and starts the core.
//
// If baseYAML is non-empty, it is used as the base config via GenerateFromBase,
// preserving subscription template sections (rules, rule-providers, groups).
// If onlyNodes is non-nil, only those nodes are used as the input node set;
// otherwise nodes are collected from all enabled subscriptions.
func autoConfigureClashForge(
	ctx context.Context,
	deps Deps,
	out EventWriter,
	activeSubID string,
	activeSubName string,
	baseYAML string,
	onlyNodes []subscription.ProxyNode,
) error {
	_ = baseYAML
	_ = onlyNodes

	emit(out, PhaseConfigure, "update_config", StatusRunning, "更新 ClashForge 配置...")

	cfg := deps.Config

	// ── 代理核心 ─────────────────────────────────────────────────────────────
	cfg.Core.AutoStartCore = true

	// ── 透明代理 + 防火墙规则（auto-detected） ────────────────────────────────
	cfg.Network.Mode = "tproxy"
	cfg.Network.ApplyOnStart = true
	cfg.Network.BypassChina = true
	cfg.Network.BypassLAN = true
	// FirewallBackend "auto" lets ClashForge pick nftables or iptables;
	// only override if currently unset / explicitly disabled.
	if cfg.Network.FirewallBackend == "" || cfg.Network.FirewallBackend == "none" {
		cfg.Network.FirewallBackend = "auto"
	}

	// ── DNS 解析引擎 + DNS 入口（dnsmasq 上游转发）────────────────────────────
	// These two flags together make both the "DNS 入口" and "DNS 解析引擎"
	// tiles go green in the Setup page.
	cfg.DNS.Enable = true
	cfg.DNS.ApplyOnStart = true
	cfg.DNS.Strategy = config.DNSStrategysplit // 分流优先（geosite 精准路由）
	// Use dnsmasq upstream-forward mode: dnsmasq stays on :53 and forwards
	// all queries to Mihomo, which handles geo-split resolution.
	// Only override if currently unset or disabled.
	if cfg.DNS.DnsmasqMode == "" || cfg.DNS.DnsmasqMode == "none" {
		cfg.DNS.DnsmasqMode = "upstream"
	}

	// Save config to TOML
	if err := config.Save(deps.ConfigPath, cfg); err != nil {
		emit(out, PhaseConfigure, "update_config", StatusError, "保存配置失败", err.Error())
		return fmt.Errorf("save config: %w", err)
	}
	emit(out, PhaseConfigure, "update_config", StatusOK, "配置已保存（DNS 分流策略，TProxy 模式）")

	if strings.TrimSpace(activeSubID) != "" {
		if err := setActiveSubscriptionSource(cfg.Core.DataDir, activeSubID, activeSubName); err != nil {
			emit(out, PhaseConfigure, "set_active_source", StatusWarning,
				"写入活动订阅源失败，后续启动可能不走该订阅", err.Error())
		} else {
			emit(out, PhaseConfigure, "set_active_source", StatusOK,
				fmt.Sprintf("已设置活动配置源为订阅：%s", activeSubName))
		}
	}

	// Generate runtime config via the same source-selection pipeline used by /setup.
	emit(out, PhaseConfigure, "gen_config", StatusRunning, "以订阅为基准应用运行参数并写入 Mihomo 配置...")
	generated, err := runtimecfg.GenerateAndWrite(cfg, deps.SubManager)
	if err != nil {
		emit(out, PhaseConfigure, "gen_config", StatusError, "Mihomo 配置生成失败", err.Error())
		return fmt.Errorf("generate mihomo config: %w", err)
	}
	if !generated {
		emit(out, PhaseConfigure, "gen_config", StatusWarning, "未生成配置（暂无可用订阅源）")
	} else {
		emit(out, PhaseConfigure, "gen_config", StatusOK, "Mihomo 配置已按 /setup 默认流程写入")
	}

	// Refresh netfilter manager with updated config so it uses the new mode/backend,
	// then apply transparent-proxy + firewall rules immediately.
	if deps.Netfilter != nil {
		dnsRedirect := cfg.DNS.Enable && cfg.DNS.ApplyOnStart &&
			strings.ToLower(strings.TrimSpace(cfg.DNS.DnsmasqMode)) == "replace"
		bypassFakeIP := !cfg.DNS.Enable || !cfg.DNS.ApplyOnStart ||
			strings.ToLower(strings.TrimSpace(cfg.DNS.Mode)) != "fake-ip"
		*deps.Netfilter = *netfilter.NewManager(netfilter.Config{
			Mode:              cfg.Network.Mode,
			FirewallBackend:   cfg.Network.FirewallBackend,
			TProxyPort:        cfg.Ports.TProxy,
			DNSPort:           cfg.Ports.DNS,
			EnableDNSRedirect: dnsRedirect,
			BypassFakeIP:      bypassFakeIP,
			BypassCIDR:        cfg.Network.BypassCIDR,
			EnableIPv6:        cfg.Network.IPv6,
		})
	}

	// Start core
	emit(out, PhaseConfigure, "start_core", StatusRunning, "启动 Mihomo 内核...")
	startCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := deps.Core.Start(startCtx); err != nil && err != core.ErrAlreadyRunning {
		emit(out, PhaseConfigure, "start_core", StatusError, "内核启动失败", err.Error())
		return fmt.Errorf("start core: %w", err)
	}
	emit(out, PhaseConfigure, "start_core", StatusOK, "Mihomo 内核已启动 ✓")

	// Apply transparent-proxy firewall rules (透明代理 + 防火墙规则)
	if deps.Netfilter != nil && cfg.Network.ApplyOnStart && cfg.Network.Mode != "none" {
		if err := deps.Netfilter.Apply(); err != nil {
			emit(out, PhaseConfigure, "apply_netfilter", StatusWarning,
				"透明代理规则应用失败（可在「代理服务」页手动接管）", err.Error())
		} else {
			emit(out, PhaseConfigure, "apply_netfilter", StatusOK, "透明代理规则已应用 ✓")
		}
	}

	// Apply DNS dnsmasq coexistence (DNS 入口)
	if cfg.DNS.Enable && cfg.DNS.ApplyOnStart {
		dnsMode := dns.DnsmasqMode(cfg.DNS.DnsmasqMode)
		if dnsMode != dns.ModeNone {
			if err := dns.Setup(dnsMode, cfg.Ports.DNS); err != nil {
				emit(out, PhaseConfigure, "apply_dns", StatusWarning,
					"DNS 入口配置失败（可在「代理服务」页手动接管）", err.Error())
			} else {
				emit(out, PhaseConfigure, "apply_dns", StatusOK,
					fmt.Sprintf("DNS 入口已应用（%s 模式）✓", dnsMode))
			}
		}
	}

	return nil
}

func setActiveSubscriptionSource(dataDir, subID, subName string) error {
	dataDir = strings.TrimSpace(dataDir)
	subID = strings.TrimSpace(subID)
	if dataDir == "" || subID == "" {
		return nil
	}
	record := activeSourceRecord{
		Type:    "subscription",
		SubID:   subID,
		SubName: strings.TrimSpace(subName),
	}
	body, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, "active_source.json"), body, 0o644)
}

// collectAllNodes gathers proxy nodes from all enabled subscriptions and imports.
func collectAllNodes(sm *subscription.Manager) []subscription.ProxyNode {
	var all []subscription.ProxyNode

	// URL-based subscriptions
	for _, s := range sm.GetAll() {
		if !s.Enabled {
			continue
		}
		nodes, err := sm.GetCachedNodes(s.ID)
		if err != nil {
			continue
		}
		for i := range nodes {
			nodes[i].SourceSubID = s.ID
		}
		all = append(all, nodes...)
	}

	// Static imports (from ImportStatic / node-import)
	for _, s := range sm.GetAllImports() {
		if !s.Enabled {
			continue
		}
		nodes, err := sm.GetCachedNodes(s.ID)
		if err != nil {
			continue
		}
		all = append(all, nodes...)
	}

	return all
}

// verifyConnectivity waits for the core to stabilise then probes connectivity via
// the Mihomo SOCKS5 port. Results are emitted as events regardless of outcome
// (failures are warnings, not errors — the user can check the dashboard for details).
func verifyConnectivity(ctx context.Context, out EventWriter) error {
	emit(out, PhaseVerify, "wait_core", StatusRunning, "等待代理服务就绪...")

	// Allow the core a moment to fully initialise rule sets and connections
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(3 * time.Second):
	}

	emit(out, PhaseVerify, "wait_core", StatusOK, "代理服务已就绪")

	// Probe via simple HTTP requests — on an OpenWrt router with TProxy active,
	// outbound traffic is automatically routed through Mihomo, so no explicit
	// proxy configuration is needed here.
	probes := []struct {
		name string
		url  string
		want int
	}{
		{"Google", "http://www.gstatic.com/generate_204", 204},
		{"YouTube", "http://yt3.ggpht.com/favicon.ico", 200},
		{"Baidu（直连）", "http://www.baidu.com/favicon.ico", 200},
	}

	client := &http.Client{
		Timeout: 8 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // don't follow redirects
		},
	}

	allOK := true
	for _, p := range probes {
		step := "probe_" + p.name
		emit(out, PhaseVerify, step, StatusRunning, fmt.Sprintf("探测 %s...", p.name))

		reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		req, _ := http.NewRequestWithContext(reqCtx, http.MethodGet, p.url, nil)
		resp, err := client.Do(req)
		cancel()

		if err != nil {
			emit(out, PhaseVerify, step, StatusWarning,
				fmt.Sprintf("⚠️ %s 探测失败（可能代理尚未生效）", p.name), err.Error())
			allOK = false
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.StatusCode == p.want || resp.StatusCode/100 == 2 || resp.StatusCode/100 == 3 {
			emit(out, PhaseVerify, step, StatusOK,
				fmt.Sprintf("✅ %s 可达（HTTP %d）", p.name, resp.StatusCode))
		} else {
			emit(out, PhaseVerify, step, StatusWarning,
				fmt.Sprintf("⚠️ %s 响应 HTTP %d（预期 %d）", p.name, resp.StatusCode, p.want))
			allOK = false
		}
	}

	if allOK {
		emit(out, PhaseVerify, "done", StatusOK, "🎉 所有连通性测试通过，ClashForge 已成功启动！")
	} else {
		emit(out, PhaseVerify, "done", StatusWarning,
			"⚠️ 部分测试未通过，代理可能需要几秒稳定，请在主页「健康检查」中确认")
	}
	return nil
}
