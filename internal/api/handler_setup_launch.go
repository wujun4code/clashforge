package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/wujun4code/clashforge/internal/core"
	"github.com/wujun4code/clashforge/internal/dns"
)

// setupLaunchRequest is the POST body for the streaming launch endpoint.
type setupLaunchRequest struct {
	DNS     setupLaunchDNS     `json:"dns"`
	Network setupLaunchNetwork `json:"network"`
}

type setupLaunchDNS struct {
	Enable       bool   `json:"enable"`
	Mode         string `json:"mode"`
	DnsmasqMode  string `json:"dnsmasq_mode"`
	ApplyOnStart bool   `json:"apply_on_start"`
}

type setupLaunchNetwork struct {
	Mode            string `json:"mode"`
	FirewallBackend string `json:"firewall_backend"`
	BypassLAN       bool   `json:"bypass_lan"`
	BypassChina     bool   `json:"bypass_china"`
	ApplyOnStart    bool   `json:"apply_on_start"`
	IPv6            bool   `json:"ipv6"`
}

// launchEvent is streamed to the client as an SSE data payload.
type launchEvent struct {
	Type    string `json:"type"`             // step | info | done
	Step    string `json:"step,omitempty"`   // step identifier
	Status  string `json:"status,omitempty"` // running | ok | error | skip
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
	// done-only fields
	Success bool   `json:"success,omitempty"`
	Error   string `json:"error,omitempty"`
}

// geodataSpec describes a single geodata file to check/download.
type geodataSpec struct {
	Name     string   // display name
	Filename string   // filename in DataDir
	URLs     []string // download URLs in priority order
}

// handleSetupFinalConfigPreview returns the final mihomo config preview that would
// be generated with the provided DNS/network setup values.
func handleSetupFinalConfigPreview(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req setupLaunchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}

		if deps.Config == nil {
			Err(w, http.StatusInternalServerError, "CONFIG_NOT_READY", "runtime config is not ready")
			return
		}

		cfgCopy := *deps.Config
		cfgCopy.DNS.Enable = req.DNS.Enable
		cfgCopy.DNS.Mode = req.DNS.Mode
		cfgCopy.DNS.DnsmasqMode = req.DNS.DnsmasqMode
		cfgCopy.DNS.ApplyOnStart = req.DNS.ApplyOnStart
		cfgCopy.Network.Mode = req.Network.Mode
		cfgCopy.Network.FirewallBackend = req.Network.FirewallBackend
		cfgCopy.Network.BypassLAN = req.Network.BypassLAN
		cfgCopy.Network.BypassChina = req.Network.BypassChina
		cfgCopy.Network.ApplyOnStart = req.Network.ApplyOnStart
		cfgCopy.Network.IPv6 = req.Network.IPv6
		cfgCopy.Core.AutoStartCore = true

		previewDeps := deps
		previewDeps.Config = &cfgCopy
		if _, err := generateMihomoConfig(previewDeps); err != nil {
			Err(w, http.StatusInternalServerError, "CONFIG_PREVIEW_FAILED", err.Error())
			return
		}

		outPath := filepath.Join(cfgCopy.Core.RuntimeDir, "mihomo-config.yaml")
		data, err := os.ReadFile(outPath)
		if err != nil {
			Err(w, http.StatusInternalServerError, "CONFIG_READ_FAILED", err.Error())
			return
		}

		JSON(w, http.StatusOK, map[string]interface{}{
			"config_file": outPath,
			"content":     string(data),
		})
	}
}

// defaultGeodataSpecs returns the list of geodata files mihomo needs.
func defaultGeodataSpecs() []geodataSpec {
	return []geodataSpec{
		{
			Name:     "GeoIP.dat",
			Filename: "GeoIP.dat",
			URLs: []string{
				// CDN mirror (faster in mainland China)
				"https://cdn.jsdmirror.com/gh/MetaCubeX/meta-rules-dat@release/geoip.dat",
				// Official GitHub release
				"https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.dat",
			},
		},
		{
			Name:     "GeoSite.dat",
			Filename: "GeoSite.dat",
			URLs: []string{
				"https://cdn.jsdmirror.com/gh/MetaCubeX/meta-rules-dat@release/geosite.dat",
				"https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat",
			},
		},
	}
}

// handleSetupLaunch streams step-by-step launch progress over SSE (POST body).
// Every step is also written to the structured logger so it appears in /activity.
//
// Step sequence:
//  1. conflicts      — detect conflicting services
//  2. config-save    — persist DNS + network settings
//  3. config-gen     — generate mihomo config YAML
//  4. geodata-check  — verify GeoIP.dat / GeoSite.dat exist
//  5. geodata-dl-*   — download each missing geodata file
//  6. core-start     — start mihomo and wait for ready
//  7. proxy-takeover — apply nft / iptables transparent-proxy rules
//  8. dns-takeover   — configure dnsmasq redirect
func handleSetupLaunch(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req setupLaunchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			Err(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			Err(w, http.StatusInternalServerError, "SSE_UNSUPPORTED", "SSE not supported")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		// ── emit helpers ─────────────────────────────────────────────────────

		sendSSE := func(ev launchEvent) {
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}

		// emitStep sends a step event and writes a structured log entry.
		emitStep := func(step, status, message, detail string) {
			sendSSE(launchEvent{Type: "step", Step: step, Status: status, Message: message, Detail: detail})

			lev := zerolog.InfoLevel
			if status == "error" {
				lev = zerolog.ErrorLevel
			}
			ev := log.WithLevel(lev).
				Str("side", "setup_launch").
				Str("step", step).
				Str("status", status)
			if detail != "" {
				ev = ev.Str("detail", detail)
			}
			ev.Msg(message)
		}

		// emitInfo sends an informational sub-message within a step (no status change).
		emitInfo := func(step, message string) {
			sendSSE(launchEvent{Type: "info", Step: step, Message: message})
			log.Info().Str("side", "setup_launch").Str("step", step).Msg(message)
		}

		emitDone := func(success bool, errMsg string) {
			ev := launchEvent{Type: "done", Success: success}
			if !success {
				ev.Error = errMsg
			}
			sendSSE(ev)
			if success {
				log.Info().Str("side", "setup_launch").Str("step", "done").Msg("启动流程全部完成 ✓")
			} else {
				log.Error().Str("side", "setup_launch").Str("step", "done").Str("error", errMsg).Msg("启动流程失败")
			}
		}

		// ── Step 1: detect conflicts ──────────────────────────────────────────

		emitStep("conflicts", "running", "正在检测冲突服务…", "")
		conflictSvcs := []ConflictService{
			detectConflict("openclash", "OpenClash", []string{"/etc/openclash/clash", "openclash_watchdog"}),
			detectConflict("mihomo", "系统 mihomo（非 ClashForge 管理）", []string{"/usr/bin/mihomo"}),
			detectConflict("clash", "Clash（原版）", []string{"/usr/bin/clash"}),
		}
		var runningConflicts []string
		for _, c := range conflictSvcs {
			if c.Running {
				runningConflicts = append(runningConflicts, c.Label)
			}
		}
		if len(runningConflicts) > 0 {
			emitStep("conflicts", "error",
				fmt.Sprintf("检测到 %d 个冲突服务: %s", len(runningConflicts), strings.Join(runningConflicts, ", ")),
				"请先在 UI 中停止冲突服务后再启动")
			emitDone(false, "存在冲突服务，请先停止")
			return
		}
		emitStep("conflicts", "ok", "未检测到冲突服务", "")

		// ── Step 2: save config ───────────────────────────────────────────────

		emitStep("config-save", "running", "保存 DNS / 网络配置到 config.toml…", "")

		deps.Config.DNS.Enable = req.DNS.Enable
		deps.Config.DNS.Mode = req.DNS.Mode
		deps.Config.DNS.DnsmasqMode = req.DNS.DnsmasqMode
		deps.Config.DNS.ApplyOnStart = req.DNS.ApplyOnStart
		deps.Config.Network.Mode = req.Network.Mode
		deps.Config.Network.FirewallBackend = req.Network.FirewallBackend
		deps.Config.Network.BypassLAN = req.Network.BypassLAN
		deps.Config.Network.BypassChina = req.Network.BypassChina
		deps.Config.Network.ApplyOnStart = req.Network.ApplyOnStart
		deps.Config.Network.IPv6 = req.Network.IPv6
		deps.Config.Core.AutoStartCore = true

		if err := saveRuntimeConfig(deps); err != nil {
			emitStep("config-save", "error", "配置保存失败", err.Error())
			emitDone(false, err.Error())
			return
		}
		emitStep("config-save", "ok",
			fmt.Sprintf("配置已保存 — DNS: %s/%s | 代理: %s/%s",
				req.DNS.Mode, req.DNS.DnsmasqMode,
				req.Network.Mode, req.Network.FirewallBackend), "")

		// ── Step 3: generate mihomo config ────────────────────────────────────

		emitStep("config-gen", "running", "正在生成 mihomo 配置文件…", "")
		if _, err := generateMihomoConfig(deps); err != nil {
			emitStep("config-gen", "error", "配置文件生成失败", err.Error())
			emitDone(false, err.Error())
			return
		}
		configPath := filepath.Join(deps.Config.Core.RuntimeDir, "mihomo-config.yaml")
		emitStep("config-gen", "ok", fmt.Sprintf("配置文件已写入 %s", configPath), "")

		// ── Step 4: geodata check + download ──────────────────────────────────

		emitStep("geodata-check", "running", "正在检查 GeoData 文件完整性…", "")

		dataDir := deps.Config.Core.DataDir
		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			emitStep("geodata-check", "error", "无法创建数据目录", err.Error())
			emitDone(false, err.Error())
			return
		}

		allPresent := true
		for _, gf := range defaultGeodataSpecs() {
			path := filepath.Join(dataDir, gf.Filename)
			info, err := os.Stat(path)
			if err == nil {
				emitInfo("geodata-check", fmt.Sprintf("✓ %s 已存在 (%.1f MB)", gf.Name, float64(info.Size())/1024/1024))
			} else {
				emitInfo("geodata-check", fmt.Sprintf("✗ %s 不存在，需要下载", gf.Name))
				allPresent = false
			}
		}

		if allPresent {
			emitStep("geodata-check", "ok", "所有 GeoData 文件完整，无需下载 ✓", "")
		} else {
			emitStep("geodata-check", "info", "部分 GeoData 文件缺失，开始下载…", "")

			for _, gf := range defaultGeodataSpecs() {
				path := filepath.Join(dataDir, gf.Filename)
				if _, err := os.Stat(path); err == nil {
					continue // already exists
				}

				dlStep := "geodata-dl-" + gf.Filename
				emitStep(dlStep, "running", fmt.Sprintf("正在下载 %s…", gf.Name), "")

				var lastErr error
				for i, dlURL := range gf.URLs {
					host := urlHost(dlURL)
					emitInfo(dlStep, fmt.Sprintf("尝试镜像 %d/%d: %s", i+1, len(gf.URLs), host))

					start := time.Now()
					size, err := downloadGeodata(dlURL, path, 120*time.Second)
					if err == nil {
						elapsed := time.Since(start)
						emitStep(dlStep, "ok",
							fmt.Sprintf("%s 下载完成 (%.1f MB, %.1fs)", gf.Name, float64(size)/1024/1024, elapsed.Seconds()),
							fmt.Sprintf("来源: %s", host))
						lastErr = nil
						break
					}
					emitInfo(dlStep, fmt.Sprintf("镜像 %s 下载失败: %v", host, err))
					lastErr = err
				}

				if lastErr != nil {
					emitStep(dlStep, "error",
						fmt.Sprintf("%s 下载失败，所有镜像均不可用", gf.Name),
						lastErr.Error())
					emitDone(false, fmt.Sprintf("下载 %s 失败: %v", gf.Name, lastErr))
					return
				}
			}

			emitStep("geodata-check", "ok", "所有 GeoData 文件下载完成 ✓", "")
		}

		// ── Step 5: start core ────────────────────────────────────────────────

		emitStep("core-start", "running", "正在启动 mihomo 内核…", "")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err := deps.Core.Start(ctx); err != nil && err != core.ErrAlreadyRunning {
			emitStep("core-start", "error", "内核启动失败", err.Error())
			emitDone(false, err.Error())
			return
		}
		st := deps.Core.Status()
		emitStep("core-start", "ok",
			fmt.Sprintf("内核已启动 ✓ (PID %d)", st.PID),
			fmt.Sprintf("binary: %s | api: :%d", deps.Config.Core.Binary, deps.Config.Ports.MihomoAPI))

		// ── Step 6: transparent proxy takeover ────────────────────────────────

		if req.Network.Mode != "none" && req.Network.ApplyOnStart && deps.Netfilter != nil {
			emitStep("proxy-takeover", "running",
				fmt.Sprintf("正在应用透明代理规则 (%s / %s)…", req.Network.Mode, req.Network.FirewallBackend), "")
			refreshNetfilterManager(deps)
			if err := deps.Netfilter.Apply(); err != nil {
				emitStep("proxy-takeover", "error", "透明代理规则应用失败", err.Error())
				emitDone(false, err.Error())
				return
			}
			emitStep("proxy-takeover", "ok",
				fmt.Sprintf("透明代理已接管 ✓ — %s 模式 / %s 后端", req.Network.Mode, deps.Netfilter.BackendName()), "")
		} else {
			reason := "mode=none"
			if !req.Network.ApplyOnStart {
				reason = "apply_on_start=false"
			}
			emitStep("proxy-takeover", "skip",
				fmt.Sprintf("透明代理接管已跳过 (%s)", reason), "")
		}

		// ── Step 7: DNS takeover ──────────────────────────────────────────────

		if req.DNS.Enable && req.DNS.ApplyOnStart && req.DNS.DnsmasqMode != "none" {
			dnsMode := req.DNS.DnsmasqMode
			dnsPort := deps.Config.Ports.DNS
			var dnsDetail string
			switch dnsMode {
			case "replace":
				dnsDetail = fmt.Sprintf("dnsmasq port=0 → nft redirect :53→:%d → mihomo", dnsPort)
			case "upstream":
				dnsDetail = fmt.Sprintf("dnsmasq server=127.0.0.1#%d (UCI), LAN :53 → dnsmasq → mihomo :%d", dnsPort, dnsPort)
			}
			emitStep("dns-takeover", "running",
				fmt.Sprintf("正在接管 DNS 入口 (dnsmasq 模式: %s, DNS 端口: %d)…", dnsMode, dnsPort), dnsDetail)
			if err := dns.Setup(dns.DnsmasqMode(dnsMode), dnsPort); err != nil {
				emitStep("dns-takeover", "error", "DNS 接管失败", err.Error())
				emitDone(false, err.Error())
				return
			}
			emitStep("dns-takeover", "ok",
				fmt.Sprintf("DNS 已接管 ✓ — dnsmasq %s 模式，流量 :53 → mihomo :%d", dnsMode, dnsPort), dnsDetail)
		} else {
			reason := "dns.enable=false"
			if req.DNS.Enable && !req.DNS.ApplyOnStart {
				reason = "apply_on_start=false"
			} else if req.DNS.Enable && req.DNS.DnsmasqMode == "none" {
				reason = "dnsmasq_mode=none"
			}
			emitStep("dns-takeover", "skip",
				fmt.Sprintf("DNS 接管已跳过 (%s)", reason), "")
		}

		// ── Done ──────────────────────────────────────────────────────────────

		emitDone(true, "")
	}
}

// downloadGeodata downloads url to destPath atomically (write to tmp then rename).
// Returns the number of bytes written.
func downloadGeodata(rawURL, destPath string, timeout time.Duration) (int64, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(rawURL)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d from %s", resp.StatusCode, rawURL)
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return 0, fmt.Errorf("mkdir: %w", err)
	}

	f, err := os.CreateTemp(filepath.Dir(destPath), ".geodata-*.tmp")
	if err != nil {
		return 0, err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // no-op if rename succeeds

	n, err := io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		return 0, fmt.Errorf("write: %w", err)
	}
	if n == 0 {
		return 0, fmt.Errorf("empty response body from %s", rawURL)
	}
	if err := os.Rename(tmp, destPath); err != nil {
		return 0, fmt.Errorf("rename: %w", err)
	}
	return n, nil
}

// urlHost extracts just the hostname from a URL for display purposes.
func urlHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Host
}
