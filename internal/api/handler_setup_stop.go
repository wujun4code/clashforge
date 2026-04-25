package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/wujun4code/clashforge/internal/core"
	"github.com/wujun4code/clashforge/internal/dns"
	"github.com/wujun4code/clashforge/internal/netfilter"
)

// handleSetupStop streams a step-by-step teardown over SSE, mirroring the
// structure of handleSetupLaunch.  Every operation is reported individually
// so the /setup page can show exactly what was (and wasn't) restored.
//
// Stop sequence (designed for zero-DNS-blackout teardown):
//  1. core-stop        — SIGTERM → wait → SIGKILL mihomo
//  2. dns-restore      — UCI delete port=0/server=/noresolv=, commit, dnsmasq restart
//  3. nft-metaclash    — delete table inet metaclash (tproxy + dns_redirect chains)
//  4. nft-dnsmasq-hijack — delete table inet dnsmasq (the HIJACK table dnsmasq injects)
//  5. route-cleanup    — ip rule del fwmark 0x1a3 + ip route flush table 100 (v4+v6)
func handleSetupStop(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			Err(w, http.StatusInternalServerError, "SSE_UNSUPPORTED", "SSE not supported")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		sendSSE := func(ev launchEvent) {
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
		emitStep := func(step, status, message, detail string) {
			sendSSE(launchEvent{Type: "step", Step: step, Status: status, Message: message, Detail: detail})
			lev := zerolog.InfoLevel
			if status == "error" {
				lev = zerolog.ErrorLevel
			}
			ev := log.WithLevel(lev).
				Str("side", "setup_stop").
				Str("step", step).
				Str("status", status)
			if detail != "" {
				ev = ev.Str("detail", detail)
			}
			ev.Msg(message)
		}
		emitInfo := func(step, message string) {
			sendSSE(launchEvent{Type: "info", Step: step, Message: message})
			log.Info().Str("side", "setup_stop").Str("step", step).Msg(message)
		}
		emitDone := func(success bool, errMsg string) {
			ev := launchEvent{Type: "done", Success: success}
			if !success {
				ev.Error = errMsg
			}
			sendSSE(ev)
			if success {
				log.Info().Str("side", "setup_stop").Msg("停止流程全部完成 ✓")
			} else {
				log.Error().Str("side", "setup_stop").Str("error", errMsg).Msg("停止流程异常结束")
			}
		}

		// ── Step 1: stop mihomo core ──────────────────────────────────────────

		emitStep("core-stop", "running", "正在停止 mihomo 内核…",
			"发送 SIGTERM，等待最多 5 秒优雅退出，超时后 SIGKILL")

		coreWasRunning := deps.Core.Status().Ready
		if coreWasRunning {
			pid := deps.Core.Status().PID
			emitInfo("core-stop", fmt.Sprintf("mihomo PID %d — 发送 SIGTERM…", pid))
		}

		if err := deps.Core.Stop(); err != nil && !errors.Is(err, core.ErrNotRunning) {
			emitStep("core-stop", "error", "内核停止失败，继续执行后续清理", err.Error())
		} else if coreWasRunning {
			emitStep("core-stop", "ok", "mihomo 内核已停止 ✓",
				fmt.Sprintf("mihomo API 端口 :%d 已释放", deps.Config.Ports.MihomoAPI))
		} else {
			emitStep("core-stop", "skip", "mihomo 内核本已停止，跳过", "")
		}

		deps.Config.Core.AutoStartCore = false
		_ = saveRuntimeConfig(deps)

		// ── Step 2: restore dnsmasq DNS config ───────────────────────────────
		// DNS restore BEFORE nftables cleanup: while dnsmasq restarts the
		// metaclash dns_redirect chain (:53→mihomo) is still active so LAN
		// clients are never left without a working DNS resolver.

		dnsMode := dns.DnsmasqMode(deps.Config.DNS.DnsmasqMode)
		emitStep("dns-restore", "running", "正在恢复 dnsmasq DNS 配置…",
			fmt.Sprintf("当前模式: %s", dnsMode))

		switch dnsMode {
		case dns.ModeReplace:
			emitInfo("dns-restore", "replace 模式: 删除 UCI dhcp.@dnsmasq[0].port=0 覆盖")
			emitInfo("dns-restore", "执行: uci delete dhcp.@dnsmasq[0].port && uci commit dhcp")
		case dns.ModeUpstream:
			emitInfo("dns-restore", "upstream 模式: 删除 UCI dhcp.@dnsmasq[0].server / noresolv 覆盖")
			emitInfo("dns-restore", "执行: uci delete dhcp.@dnsmasq[0].server && uci delete dhcp.@dnsmasq[0].noresolv && uci commit dhcp")
		default:
			emitInfo("dns-restore", fmt.Sprintf("dnsmasq_mode=%s，尝试两种模式的还原（belt-and-suspenders）", dnsMode))
		}

		dnsRestoreErr := false
		for _, mode := range []dns.DnsmasqMode{dns.ModeReplace, dns.ModeUpstream} {
			if err := dns.Restore(mode); err != nil {
				emitInfo("dns-restore", fmt.Sprintf("⚠ Restore(%s) 返回错误: %v", mode, err))
				dnsRestoreErr = true
			}
		}
		emitInfo("dns-restore", "执行: /etc/init.d/dnsmasq restart（full restart，让 UCI 变更生效）")

		if dnsRestoreErr {
			emitStep("dns-restore", "error", "dnsmasq 恢复过程中存在错误，请检查 UCI 配置",
				"可手动执行: uci delete dhcp.@dnsmasq[0].port && uci commit dhcp && /etc/init.d/dnsmasq restart")
		} else {
			emitStep("dns-restore", "ok", "dnsmasq 已恢复监听 :53 ✓",
				"UCI port=0 覆盖已删除，dnsmasq 已完整重启")
		}

		// ── Step 3: delete table inet metaclash ──────────────────────────────

		emitStep("nft-metaclash", "running", "正在清理 nftables table inet metaclash…",
			"包含: chain dns_redirect (nat prerouting, :53→mihomo DNS) + chain tproxy_prerouting (mangle, TProxy) + set bypass_ipv4")

		if err := (&netfilter.NftablesBackend{}).Cleanup(); err != nil {
			emitStep("nft-metaclash", "error", "nftables 清理失败", err.Error())
		} else {
			emitInfo("nft-metaclash", "执行: nft delete table inet metaclash")
			emitInfo("nft-metaclash", fmt.Sprintf("执行: ip rule del fwmark %s table %s", netfilter.FWMark, netfilter.RouteTable))
			emitInfo("nft-metaclash", fmt.Sprintf("执行: ip route flush table %s", netfilter.RouteTable))
			emitInfo("nft-metaclash", fmt.Sprintf("执行: ip -6 rule del fwmark %s table %s", netfilter.FWMark, netfilter.RouteTable))
			emitInfo("nft-metaclash", fmt.Sprintf("执行: ip -6 route flush table %s", netfilter.RouteTable))
			emitStep("nft-metaclash", "ok", "table inet metaclash 已删除，策略路由已清除 ✓",
				"透明代理 TProxy 规则、DNS 劫持链、fwmark 路由全部还原")
		}

		// Also cleanup iptables backends (for mixed-backend environments)
		for _, dnsPort := range dedupeInts([]int{deps.Config.Ports.DNS, 17874, 7874}) {
			if dnsPort <= 0 {
				continue
			}
			_ = (&netfilter.IptablesBackend{DNSPort: dnsPort}).Cleanup()
		}

		// ── Step 4: delete table inet dnsmasq HIJACK table ───────────────────
		// dnsmasq auto-injects this table (priority dstnat-5) on every restart.
		// In replace mode (port=0) the redirect :53→:53 hits nothing and breaks
		// DNS for LAN clients.  After our dns-restore step dnsmasq is back on
		// :53 so this table is harmless if it re-appears, but we clean it up
		// explicitly to leave the nftables ruleset pristine.

		emitStep("nft-dnsmasq-hijack", "running", "正在检查并清理 table inet dnsmasq (HIJACK 表)…",
			"dnsmasq 在每次重启时自动注入此表 (priority dstnat-5)，replace 模式下 port=0 时会导致 DNS 全挂")

		out, err := exec.Command("nft", "delete", "table", "inet", "dnsmasq").CombinedOutput()
		if err != nil {
			s := string(out)
			if strings.Contains(s, "No such file") || strings.Contains(s, "table not found") || strings.Contains(s, "does not exist") {
				emitStep("nft-dnsmasq-hijack", "skip", "table inet dnsmasq 不存在，跳过 ✓", "dnsmasq 未注入 HIJACK 规则")
			} else {
				emitStep("nft-dnsmasq-hijack", "error", "删除 table inet dnsmasq 失败（非致命）", s)
			}
		} else {
			emitInfo("nft-dnsmasq-hijack", "执行: nft delete table inet dnsmasq")
			emitStep("nft-dnsmasq-hijack", "ok", "table inet dnsmasq (HIJACK) 已删除 ✓",
				"dnsmasq 将在下次重启时以正确的 port=53 重新注入该表")
		}

		// ── Step 5: verify final state ────────────────────────────────────────

		emitStep("verify", "running", "正在验证最终网络状态…", "")

		var issues []string

		// Check port 53
		port53 := exec.Command("sh", "-c", "netstat -lnup 2>/dev/null | grep ':53 '")
		if out53, err53 := port53.Output(); err53 != nil || len(out53) == 0 {
			issues = append(issues, "⚠ dnsmasq 尚未在 :53 监听（可能仍在重启中，稍等片刻即可）")
		} else {
			emitInfo("verify", fmt.Sprintf("✓ DNS :53 正在监听: %s", strings.TrimSpace(string(out53))))
		}

		// Check metaclash table gone
		if chk := exec.Command("nft", "list", "table", "inet", "metaclash"); chk.Run() == nil {
			issues = append(issues, "⚠ table inet metaclash 仍然存在，请手动执行: nft delete table inet metaclash")
		} else {
			emitInfo("verify", "✓ table inet metaclash 已清除")
		}

		// Check ip rules
		if ipRule, _ := exec.Command("sh", "-c", "ip rule show 2>/dev/null | grep 'fwmark 0x1a3'").Output(); len(ipRule) > 0 {
			issues = append(issues, "⚠ ip rule fwmark 0x1a3 仍然存在")
		} else {
			emitInfo("verify", "✓ 策略路由规则 fwmark 0x1a3 已清除")
		}

		if len(issues) > 0 {
			emitStep("verify", "error",
				fmt.Sprintf("验证发现 %d 个问题", len(issues)),
				strings.Join(issues, "\n"))
		} else {
			emitStep("verify", "ok", "所有系统模块已完整还原 ✓",
				"nftables 规则已清除 | dnsmasq 监听 :53 | 策略路由已移除 | 内核已停止")
		}

		emitDone(len(issues) == 0, strings.Join(issues, "; "))
	}
}
