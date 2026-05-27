package quickstart

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/core"
	"github.com/wujun4code/clashforge/internal/subscription"
)

// autoConfigureClashForge applies QuickStart defaults to the ClashForge config,
// regenerates the Mihomo YAML, and starts the core.
func autoConfigureClashForge(ctx context.Context, deps Deps, out EventWriter) error {
	emit(out, PhaseConfigure, "update_config", StatusRunning, "更新 ClashForge 配置...")

	cfg := deps.Config
	cfg.DNS.Strategy = config.DNSStrategysplit // 分流优先
	cfg.Network.Mode = "tproxy"
	cfg.Network.BypassChina = true
	cfg.DNS.ApplyOnStart = true
	cfg.Network.ApplyOnStart = true
	cfg.Core.AutoStartCore = true

	// Save config to TOML
	if err := config.Save(deps.ConfigPath, cfg); err != nil {
		emit(out, PhaseConfigure, "update_config", StatusError, "保存配置失败", err.Error())
		return fmt.Errorf("save config: %w", err)
	}
	emit(out, PhaseConfigure, "update_config", StatusOK, "配置已保存（DNS 分流策略，TProxy 模式）")

	// Collect proxy nodes from all enabled imports
	emit(out, PhaseConfigure, "gen_config", StatusRunning, "重新生成 Mihomo 配置...")
	nodes := collectAllNodes(deps.SubManager)

	generated, err := config.Generate(cfg, nodes)
	if err != nil {
		emit(out, PhaseConfigure, "gen_config", StatusError, "Mihomo 配置生成失败", err.Error())
		return fmt.Errorf("generate mihomo config: %w", err)
	}
	generated = config.ApplyManagedRuntimeSettings(cfg, generated)

	data, err := config.MarshalYAML(generated)
	if err != nil {
		emit(out, PhaseConfigure, "gen_config", StatusError, "Mihomo 配置序列化失败", err.Error())
		return fmt.Errorf("marshal mihomo config: %w", err)
	}

	outPath := cfg.Core.RuntimeDir + "/mihomo-config.yaml"
	if err := os.MkdirAll(cfg.Core.RuntimeDir, 0o755); err != nil {
		emit(out, PhaseConfigure, "gen_config", StatusError, "创建运行时目录失败", err.Error())
		return fmt.Errorf("mkdir runtime dir: %w", err)
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		emit(out, PhaseConfigure, "gen_config", StatusError, "写入 Mihomo 配置失败", err.Error())
		return fmt.Errorf("write mihomo config: %w", err)
	}
	emit(out, PhaseConfigure, "gen_config", StatusOK,
		fmt.Sprintf("Mihomo 配置已写入（%d 个代理节点）", len(nodes)))

	// Start core
	emit(out, PhaseConfigure, "start_core", StatusRunning, "启动 Mihomo 内核...")
	startCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := deps.Core.Start(startCtx); err != nil && err != core.ErrAlreadyRunning {
		emit(out, PhaseConfigure, "start_core", StatusError, "内核启动失败", err.Error())
		return fmt.Errorf("start core: %w", err)
	}
	emit(out, PhaseConfigure, "start_core", StatusOK, "Mihomo 内核已启动 ✓")

	return nil
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
