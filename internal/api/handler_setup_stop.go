package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/rs/zerolog/log"
	"github.com/wujun4code/clashforge/internal/core"
)

// Keep every stop入口 on the same DNS-first teardown. In particular, do not
// bypass Manager.Cleanup: it owns the TUN fw4 exception and policy rules too.
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
		emit := func(ev launchEvent) {
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			log.Info().Str("side", "setup_stop").Str("step", ev.Step).Str("status", ev.Status).Str("error", ev.Error).Msg(ev.Message)
		}
		fail := func(step string, err error) {
			emit(launchEvent{Type: "step", Step: step, Status: "error", Message: "停止/清理失败", Detail: err.Error()})
			emit(launchEvent{Type: "done", Success: false, Error: err.Error()})
		}
		emit(launchEvent{Type: "step", Step: "dns-restore", Status: "running", Message: "先恢复 dnsmasq，再清理透明代理及 TUN 专用规则"})
		if _, err := releaseAllTakeover(deps); err != nil {
			fail("dns-restore", err)
			return
		}
		emit(launchEvent{Type: "step", Step: "dns-restore", Status: "ok", Message: "DNS 配置恢复命令已完成"})
		emit(launchEvent{Type: "step", Step: "nft-metaclash", Status: "ok", Message: "透明代理 / TUN 接管清理已完成（含 fw4 forward 与专用策略路由）"})
		emit(launchEvent{Type: "step", Step: "core-stop", Status: "running", Message: "正在停止 mihomo 内核…"})
		if err := deps.Core.Stop(); err != nil && !errors.Is(err, core.ErrNotRunning) {
			fail("core-stop", err)
			return
		}
		deps.Config.Core.AutoStartCore = false
		if err := saveRuntimeConfig(deps); err != nil {
			fail("config-save", err)
			return
		}
		emit(launchEvent{Type: "step", Step: "core-stop", Status: "ok", Message: "mihomo 内核已停止"})
		// These are command-level acknowledgements, not a claim that kernel
		// forwarding or a real DNS :53 query has been tested successfully.
		emit(launchEvent{Type: "done", Success: true, Message: "停止与接管清理命令已完成"})
	}
}
