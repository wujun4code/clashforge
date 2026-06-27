package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/core"
	"github.com/wujun4code/clashforge/internal/dns"
	"gopkg.in/yaml.v3"
)

// applySource describes where the mihomo source YAML comes from.
// Only one of the typed variants is used per call; Type selects which.
type applySource struct {
	// "yaml"     — inline YAML content is saved as a new source file
	// "sub_id"   — use an existing subscription; optionally sync from URL first
	// "filename" — use a previously saved source file by name
	// "current"  — keep the active source unchanged (default when Type is empty)
	Type     string `json:"type"`
	YAML     string `json:"yaml"`     // type=yaml: raw Clash/Mihomo YAML
	SubID    string `json:"sub_id"`   // type=sub_id
	SubName  string `json:"sub_name"` // type=sub_id: display name (optional)
	Filename string `json:"filename"` // type=filename
	Sync     bool   `json:"sync"`     // type=sub_id: fetch from URL before applying
}

// applyDNS holds DNS settings. All fields are optional; nil/empty uses ClashForge defaults.
type applyDNS struct {
	Enable       *bool    `json:"enable"`
	Mode         string   `json:"mode"`          // "fake-ip" | "redir-host"
	Strategy     string   `json:"strategy"`      // "split" | "privacy" | "legacy"
	DnsmasqMode  string   `json:"dnsmasq_mode"`  // "upstream" | "replace" | "none"
	Listen       string   `json:"listen"`        // "0.0.0.0:17874"
	IPv6         *bool    `json:"ipv6"`
	ApplyOnStart *bool    `json:"apply_on_start"`
	Nameservers  []string `json:"nameservers"`
	Fallback     []string `json:"fallback"`
	DoH          []string `json:"doh"`
	FakeIPFilter []string `json:"fake_ip_filter"`
}

// applyNetwork holds network/routing settings. All fields are optional; nil/empty uses defaults.
type applyNetwork struct {
	Mode            string `json:"mode"`             // "tproxy" | "tun" | "redir" | "none"
	FirewallBackend string `json:"firewall_backend"` // "auto" | "nftables" | "iptables" | "none"
	BypassLAN       *bool  `json:"bypass_lan"`
	BypassChina     *bool  `json:"bypass_china"`
	ApplyOnStart    *bool  `json:"apply_on_start"`
	IPv6            *bool  `json:"ipv6"`
	WANInterface    string `json:"wan_interface"` // "auto" or explicit interface name
	DropQUIC        *bool  `json:"drop_quic"`
}

// coreApplyRequest is the POST body for POST /api/v1/core/apply.
// All fields have ClashForge defaults and can be omitted for a zero-config call.
type coreApplyRequest struct {
	Source  applySource  `json:"source"`
	DNS     applyDNS     `json:"dns"`
	Network applyNetwork `json:"network"`
}

// boolOr returns *b if non-nil, otherwise def.
func boolOr(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}

// applyBaselineOverridesYAML is the Go equivalent of SETUP_BASELINE_OVERRIDES in Setup.tsx.
// Merged into the user YAML (user values win) or written standalone for subscriptions.
const applyBaselineOverridesYAML = `log-level: info
bind-address: "*"
keep-alive-interval: 15
keep-alive-idle: 30
ipv6: false
tcp-concurrent: true
unified-delay: true
experimental:
  quic-go-disable-gso: true
sniffer:
  enable: true
  override-destination: true
  sniff:
    QUIC:
      ports: [443]
    TLS:
      ports: [443, 8443]
    HTTP:
      ports: [80, "8080-8880"]
      override-destination: true
  parse-pure-ip: true
profile:
  store-selected: true
  store-fake-ip: true
ntp:
  enable: true
  server: time.apple.com
  port: 123
  interval: 30
  write-to-system: true
`

// mergeBaselineIntoUserYAML merges applyBaselineOverridesYAML into the user's YAML.
// User's existing keys take precedence (baseline only fills missing keys).
func mergeBaselineIntoUserYAML(userYAML string) (string, error) {
	var base map[string]interface{}
	if err := yaml.Unmarshal([]byte(applyBaselineOverridesYAML), &base); err != nil {
		base = map[string]interface{}{} // non-fatal
	}

	var user map[string]interface{}
	if err := yaml.Unmarshal([]byte(userYAML), &user); err != nil {
		return "", fmt.Errorf("invalid YAML: %w", err)
	}
	if user == nil {
		user = map[string]interface{}{}
	}

	// DeepMerge(dst=baseline, src=user) → user wins for conflicts
	merged := config.DeepMerge(base, user)
	out, err := yaml.Marshal(merged)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// saveApplyYAMLSource saves YAML content as a source file, deduplicating by SHA-256.
// Returns the filename.
func saveApplyYAMLSource(deps Dependencies, content string) (string, error) {
	dir := sourcesDirPath(deps.Config.Core.DataDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir sources: %w", err)
	}
	hash := sha256.Sum256([]byte(content))
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasSuffix(n, ".yaml") && !strings.HasSuffix(n, ".yml") {
			continue
		}
		existing, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			continue
		}
		if sha256.Sum256(existing) == hash {
			return n, nil
		}
	}
	filename := nextPastedFilename(dir)
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write source: %w", err)
	}
	return filename, nil
}

// handleCoreApply is the unified "apply all settings and start" endpoint.
//
// It is idempotent: calling it repeatedly with the same parameters produces
// the same result. It streams step-by-step progress over SSE in the same
// format as POST /api/v1/setup/launch so the frontend and netdiag can share
// the same rendering logic.
//
// Step sequence:
//
//	0. source         — save/activate the source YAML (skipped for type=current)
//	1. conflicts      — detect conflicting proxy services
//	2. config-save    — apply DNS + network defaults and persist to config.toml
//	2.5 dns-probe     — detect upstream fake-ip hijacking; auto-inject DoH before config-gen
//	3. config-gen     — generate the final mihomo-config.yaml (DoH already set if needed)
//	4. geodata-*      — verify / download GeoData files
//	5. core-start     — start or restart the mihomo core
//	6. proxy-takeover — apply nftables / iptables transparent-proxy rules
//	7. dns-takeover   — configure dnsmasq redirect
func handleCoreApply(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req coreApplyRequest
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

		// ── emit helpers ──────────────────────────────────────────────────────

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
			ev := log.WithLevel(lev).Str("side", "core_apply").Str("step", step).Str("status", status)
			if detail != "" {
				ev = ev.Str("detail", detail)
			}
			ev.Msg(message)
		}

		emitInfo := func(step, message string) {
			sendSSE(launchEvent{Type: "info", Step: step, Message: message})
			log.Info().Str("side", "core_apply").Str("step", step).Msg(message)
		}

		emitDone := func(success bool, errMsg string) {
			ev := launchEvent{Type: "done", Success: success}
			if !success {
				ev.Error = errMsg
			}
			sendSSE(ev)
			if success {
				log.Info().Str("side", "core_apply").Msg("apply 流程全部完成 ✓")
			} else {
				log.Error().Str("side", "core_apply").Str("error", errMsg).Msg("apply 流程失败")
			}
		}

		// ── Step 0: source ────────────────────────────────────────────────────

		srcType := strings.ToLower(strings.TrimSpace(req.Source.Type))
		if srcType == "" {
			srcType = "current"
		}

		overridesPath := filepath.Join(deps.Config.Core.DataDir, "overrides.yaml")

		switch srcType {
		case "yaml":
			if strings.TrimSpace(req.Source.YAML) == "" {
				emitDone(false, "source.type=yaml 但 source.yaml 为空")
				return
			}
			emitStep("source", "running", "正在保存 YAML 配置内容…", "")
			merged, err := mergeBaselineIntoUserYAML(req.Source.YAML)
			if err != nil {
				emitStep("source", "error", "YAML 内容无效", err.Error())
				emitDone(false, err.Error())
				return
			}
			filename, err := saveApplyYAMLSource(deps, req.Source.YAML)
			if err != nil {
				emitStep("source", "error", "保存来源文件失败", err.Error())
				emitDone(false, err.Error())
				return
			}
			if err := writeActiveSource(deps.Config.Core.DataDir, ActiveSource{Type: "file", Filename: filename}); err != nil {
				emitStep("source", "error", "设置活跃来源失败", err.Error())
				emitDone(false, err.Error())
				return
			}
			if err := os.WriteFile(overridesPath, []byte(merged), 0o644); err != nil {
				emitStep("source", "error", "写入 overrides.yaml 失败", err.Error())
				emitDone(false, err.Error())
				return
			}
			emitStep("source", "ok", fmt.Sprintf("YAML 来源已保存 → %s", filename), "")

		case "sub_id":
			if req.Source.SubID == "" {
				emitDone(false, "source.type=sub_id 但 source.sub_id 为空")
				return
			}
			if deps.SubManager != nil {
				if _, ok := deps.SubManager.GetByID(req.Source.SubID); !ok {
					emitStep("source", "error",
						fmt.Sprintf("订阅 %s 不存在（可能已被删除）", req.Source.SubID), "")
					emitDone(false, fmt.Sprintf("subscription %s not found", req.Source.SubID))
					return
				}
			}
			// Force sync if explicitly requested OR if there is no cached content yet
			// (new subscription that has never been fetched must be pulled before
			// generateMihomoConfig runs, otherwise the config has zero proxies and
			// all traffic falls through to the upstream gateway unproxied).
			shouldSync := req.Source.Sync || (deps.SubManager != nil && !deps.SubManager.HasCache(req.Source.SubID))
			if shouldSync && deps.SubManager != nil {
				emitStep("source", "running", fmt.Sprintf("正在同步订阅 %s…", req.Source.SubID), "")
				if err := deps.SubManager.SyncUpdate(req.Source.SubID); err != nil {
					emitStep("source", "error", "订阅同步失败", err.Error())
					emitDone(false, err.Error())
					return
				}
			}
			subName := req.Source.SubName
			if subName == "" {
				subName = req.Source.SubID
			}
			if err := writeActiveSource(deps.Config.Core.DataDir, ActiveSource{
				Type: "subscription", SubID: req.Source.SubID, SubName: subName,
			}); err != nil {
				emitStep("source", "error", "设置活跃订阅失败", err.Error())
				emitDone(false, err.Error())
				return
			}
			// Subscription content is managed by SubManager; write baseline-only overrides.
			if err := os.WriteFile(overridesPath, []byte(applyBaselineOverridesYAML), 0o644); err != nil {
				emitStep("source", "error", "写入 overrides.yaml 失败", err.Error())
				emitDone(false, err.Error())
				return
			}
			emitStep("source", "ok", fmt.Sprintf("活跃来源已设置为订阅 %s", subName), "")

		case "filename":
			name := req.Source.Filename
			if name == "" || strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
				emitDone(false, "source.type=filename 但 source.filename 无效")
				return
			}
			filePath := filepath.Join(sourcesDirPath(deps.Config.Core.DataDir), name)
			content, err := os.ReadFile(filepath.Clean(filePath))
			if err != nil {
				emitStep("source", "error", fmt.Sprintf("读取来源文件 %s 失败", name), err.Error())
				emitDone(false, err.Error())
				return
			}
			merged, err := mergeBaselineIntoUserYAML(string(content))
			if err != nil {
				emitStep("source", "error", "来源文件 YAML 解析失败", err.Error())
				emitDone(false, err.Error())
				return
			}
			if err := writeActiveSource(deps.Config.Core.DataDir, ActiveSource{Type: "file", Filename: name}); err != nil {
				emitStep("source", "error", "设置活跃来源失败", err.Error())
				emitDone(false, err.Error())
				return
			}
			if err := os.WriteFile(overridesPath, []byte(merged), 0o644); err != nil {
				emitStep("source", "error", "写入 overrides.yaml 失败", err.Error())
				emitDone(false, err.Error())
				return
			}
			emitStep("source", "ok", fmt.Sprintf("来源文件 %s 已激活", name), "")

		default: // "current"
			emitStep("source", "skip", "使用现有活跃来源（未变更）", "")
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

		// ── Step 2: fill defaults and save config ─────────────────────────────

		emitStep("config-save", "running", "保存 DNS / 网络配置到 config.toml…", "")

		// DNS — only override if the request provides a non-zero value; otherwise
		// keep the existing config value, or fall back to the ClashForge default.
		def := config.Default()

		deps.Config.DNS.Enable = boolOr(req.DNS.Enable, true)
		if req.DNS.Mode != "" {
			deps.Config.DNS.Mode = req.DNS.Mode
		} else if deps.Config.DNS.Mode == "" {
			deps.Config.DNS.Mode = def.DNS.Mode
		}
		if req.DNS.Strategy != "" {
			deps.Config.DNS.Strategy = req.DNS.Strategy
		} else if deps.Config.DNS.Strategy == "" {
			deps.Config.DNS.Strategy = def.DNS.Strategy
		}
		if req.DNS.DnsmasqMode != "" {
			deps.Config.DNS.DnsmasqMode = req.DNS.DnsmasqMode
		} else if deps.Config.DNS.DnsmasqMode == "" {
			deps.Config.DNS.DnsmasqMode = "upstream" // recommended default
		}
		if req.DNS.Listen != "" {
			if port := parseDNSListenPort(req.DNS.Listen); port > 0 {
				deps.Config.Ports.DNS = port
			}
		}
		deps.Config.DNS.IPv6 = boolOr(req.DNS.IPv6, false)
		deps.Config.DNS.ApplyOnStart = boolOr(req.DNS.ApplyOnStart, true)
		if len(req.DNS.Nameservers) > 0 {
			deps.Config.DNS.Nameservers = req.DNS.Nameservers
		} else if len(deps.Config.DNS.Nameservers) == 0 {
			deps.Config.DNS.Nameservers = def.DNS.Nameservers
		}
		if len(req.DNS.Fallback) > 0 {
			deps.Config.DNS.Fallback = req.DNS.Fallback
		} else if len(deps.Config.DNS.Fallback) == 0 {
			deps.Config.DNS.Fallback = def.DNS.Fallback
		}
		if req.DNS.DoH != nil {
			deps.Config.DNS.DoH = req.DNS.DoH
		}
		if len(req.DNS.FakeIPFilter) > 0 {
			deps.Config.DNS.FakeIPFilter = req.DNS.FakeIPFilter
		} else if len(deps.Config.DNS.FakeIPFilter) == 0 {
			deps.Config.DNS.FakeIPFilter = def.DNS.FakeIPFilter
		}

		// Network
		if req.Network.Mode != "" {
			deps.Config.Network.Mode = req.Network.Mode
		} else if deps.Config.Network.Mode == "" {
			deps.Config.Network.Mode = def.Network.Mode
		}
		if req.Network.FirewallBackend != "" {
			deps.Config.Network.FirewallBackend = req.Network.FirewallBackend
		} else if deps.Config.Network.FirewallBackend == "" {
			deps.Config.Network.FirewallBackend = def.Network.FirewallBackend
		}
		deps.Config.Network.BypassLAN = boolOr(req.Network.BypassLAN, def.Network.BypassLAN)
		deps.Config.Network.BypassChina = boolOr(req.Network.BypassChina, def.Network.BypassChina)
		deps.Config.Network.ApplyOnStart = boolOr(req.Network.ApplyOnStart, def.Network.ApplyOnStart)
		deps.Config.Network.IPv6 = boolOr(req.Network.IPv6, false)
		deps.Config.Network.DropQUIC = boolOr(req.Network.DropQUIC, def.Network.DropQUIC)

		// WAN interface: "auto" or empty triggers auto-detection.
		wanReq := strings.TrimSpace(req.Network.WANInterface)
		if wanReq == "" || wanReq == "auto" {
			if detected, _ := config.DetectWANInterface(""); detected != "" {
				deps.Config.Network.WANInterface = detected
				deps.Config.Network.WANInterfaceAutoDetected = true
				emitInfo("config-save", fmt.Sprintf("WAN 接口自动检测: %s", detected))
			}
			// If detection fails, keep the existing configured value.
		} else {
			iface, autoDetected := config.DetectWANInterface(wanReq)
			deps.Config.Network.WANInterface = iface
			deps.Config.Network.WANInterfaceAutoDetected = autoDetected
			if autoDetected {
				emitInfo("config-save", fmt.Sprintf("WAN 接口 %q 不存在，已自动切换至 %s", wanReq, iface))
			}
		}

		deps.Config.Core.AutoStartCore = true

		if err := saveRuntimeConfig(deps); err != nil {
			emitStep("config-save", "error", "配置保存失败", err.Error())
			emitDone(false, err.Error())
			return
		}
		emitStep("config-save", "ok",
			fmt.Sprintf("配置已保存 — DNS: %s/%s | 代理: %s/%s | WAN: %s",
				deps.Config.DNS.Mode, deps.Config.DNS.DnsmasqMode,
				deps.Config.Network.Mode, deps.Config.Network.FirewallBackend,
				deps.Config.Network.WANInterface), "")

		// ── Step 2.5: dns-probe ───────────────────────────────────────────────
		// Runs BEFORE config-gen so that any auto-injected DoH entries are
		// already in deps.Config.DNS.DoH when generateMihomoConfig is called,
		// avoiding a second generate pass.

		emitStep("dns-probe", "running", "正在检测 proxy-server-nameserver 是否被上游路由劫持…", "")
		nodeHostnames := extractProxyHostnames(deps)
		udpNameservers := deps.Config.DNS.Nameservers

		switch {
		case len(nodeHostnames) == 0:
			emitStep("dns-probe", "skip", "暂无已缓存的代理节点，跳过 DNS 劫持检测", "")
		case len(udpNameservers) == 0:
			emitStep("dns-probe", "skip", "未配置 UDP nameserver，跳过检测", "")
		default:
			probeCtx, probeCancel := context.WithTimeout(context.Background(), 20*time.Second)
			report := dns.ProbeNameservers(probeCtx, udpNameservers, nodeHostnames)
			probeCancel()

			if report.AllClear {
				emitStep("dns-probe", "ok",
					fmt.Sprintf("proxy-server-nameserver 正常 ✓（测试了 %d 个节点域名）", len(report.Results)), "")
			} else {
				probeData := &dnsProbeEventData{
					HijackedNameservers: report.HijackedNameservers,
					WorkingNameservers:  report.WorkingNameservers,
					SuggestedFallbacks:  report.SuggestedFallbacks,
				}
				hijackedList := strings.Join(report.HijackedNameservers, ", ")
				if len(report.SuggestedFallbacks) > 0 {
					deps.Config.DNS.DoH = mergeDNSList(deps.Config.DNS.DoH, report.SuggestedFallbacks)
					probeData.AutoApplied = report.SuggestedFallbacks
					_ = saveRuntimeConfig(deps) // persist DoH; config-gen (step 3) will pick it up
					sendSSE(launchEvent{
						Type: "step", Step: "dns-probe", Status: "warn",
						Message: fmt.Sprintf(
							"⚠️ 检测到 %d 个 nameserver 被上游路由劫持，已自动追加 DoH 到 proxy-server-nameserver",
							len(report.HijackedNameservers)),
						Detail:   fmt.Sprintf("被劫持: %s → 已追加: %s", hijackedList, strings.Join(report.SuggestedFallbacks, ", ")),
						DNSProbe: probeData,
					})
				} else {
					sendSSE(launchEvent{
						Type: "step", Step: "dns-probe", Status: "warn",
						Message: fmt.Sprintf(
							"⚠️ 检测到 %d 个 nameserver 被上游劫持，且所有备用 DoH 也无法访问",
							len(report.HijackedNameservers)),
						Detail:   fmt.Sprintf("被劫持: %s", hijackedList),
						DNSProbe: probeData,
					})
				}
			}
		}

		// ── Step 3: generate mihomo config ────────────────────────────────────
		// deps.Config.DNS.DoH is already populated if hijacking was detected above.

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
			info, statErr := os.Stat(path)
			if statErr == nil {
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
					continue
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
						emitStep(dlStep, "ok",
							fmt.Sprintf("%s 下载完成 (%.1f MB, %.1fs)", gf.Name, float64(size)/1024/1024, time.Since(start).Seconds()),
							fmt.Sprintf("来源: %s", host))
						lastErr = nil
						break
					}
					emitInfo(dlStep, fmt.Sprintf("镜像 %s 下载失败: %v", host, err))
					lastErr = err
				}
				if lastErr != nil {
					emitStep(dlStep, "error", fmt.Sprintf("%s 下载失败，所有镜像均不可用", gf.Name), lastErr.Error())
					emitDone(false, fmt.Sprintf("下载 %s 失败: %v", gf.Name, lastErr))
					return
				}
			}
			emitStep("geodata-check", "ok", "所有 GeoData 文件下载完成 ✓", "")
		}

		// ── Step 5: start or restart core ─────────────────────────────────────

		emitStep("core-start", "running", "正在启动 mihomo 内核…", "")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		startAction := "启动"
		if err := deps.Core.Start(ctx); err != nil {
			if errors.Is(err, core.ErrAlreadyRunning) {
				// Restart to pick up new config
				startAction = "重启"
				if restartErr := deps.Core.Restart(ctx); restartErr != nil {
					emitStep("core-start", "error", "内核重启失败", restartErr.Error())
					emitDone(false, restartErr.Error())
					return
				}
			} else {
				emitStep("core-start", "error", "内核启动失败", err.Error())
				emitDone(false, err.Error())
				return
			}
		}
		st := deps.Core.Status()
		emitStep("core-start", "ok",
			fmt.Sprintf("内核已%s ✓ (PID %d)", startAction, st.PID),
			fmt.Sprintf("binary: %s | api: :%d", deps.Config.Core.Binary, deps.Config.Ports.MihomoAPI))

		// ── Step 6: transparent proxy takeover ────────────────────────────────

		netMode := deps.Config.Network.Mode
		switch {
		case netMode == "tun" && deps.Config.Network.ApplyOnStart && deps.Netfilter != nil:
			// TUN mode still needs two post-start fixups that mihomo's own auto-route
			// does NOT handle for a router (forwarding) setup:
			//   1. EnsureTunForwardAccept: insert oifname "Meta" accept into fw4's
			//      forward_lan chain — without it nftables rejects (TCP RST) all LAN
			//      client packets routed to the Meta TUN device.
			//   2. EnsureTunRouteRule: add ip rules 9020/9021 so forwarded LAN traffic
			//      falls into mihomo's table 2022 (default → Meta) rather than leaking
			//      via the kernel's bare default route to the upstream gateway.
			emitStep("proxy-takeover", "running",
				"正在为 TUN 模式补充 LAN 转发规则 (fw4 forward_lan 放行 Meta + ip rule 9020/9021)…", "")
			refreshNetfilterManager(deps)
			_ = deps.Netfilter.Apply() // always returns nil for TUN; sub-steps log.Warn on failure
			emitStep("proxy-takeover", "ok",
				"TUN 模式已接管 ✓ — fw4 forward_lan 放行 Meta 设备 | ip rule 9020/9021 补全 LAN 转发路由",
				"auto-route=true")
		case netMode == "tun":
			emitStep("proxy-takeover", "ok",
				"TUN 模式已启用（apply_on_start=false 或 netfilter 未初始化，跳过路由补充）", "")
		case netMode != "none" && deps.Config.Network.ApplyOnStart && deps.Netfilter != nil:
			emitStep("proxy-takeover", "running",
				fmt.Sprintf("正在应用透明代理规则 (%s / %s)…", netMode, deps.Config.Network.FirewallBackend), "")
			refreshNetfilterManager(deps)
			if err := deps.Netfilter.Apply(); err != nil {
				emitStep("proxy-takeover", "error", "透明代理规则应用失败", err.Error())
				emitDone(false, err.Error())
				return
			}
			emitStep("proxy-takeover", "ok",
				fmt.Sprintf("透明代理已接管 ✓ — %s 模式 / %s 后端", netMode, deps.Netfilter.BackendName()), "")
		default:
			reason := "mode=none"
			if !deps.Config.Network.ApplyOnStart {
				reason = "apply_on_start=false"
			}
			emitStep("proxy-takeover", "skip", fmt.Sprintf("透明代理接管已跳过 (%s)", reason), "")
		}

		// ── Step 7: DNS takeover ──────────────────────────────────────────────

		if deps.Config.DNS.Enable && deps.Config.DNS.ApplyOnStart && deps.Config.DNS.DnsmasqMode != "none" {
			dnsMode := deps.Config.DNS.DnsmasqMode
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
			if deps.Config.DNS.Enable && !deps.Config.DNS.ApplyOnStart {
				reason = "apply_on_start=false"
			} else if deps.Config.DNS.Enable && deps.Config.DNS.DnsmasqMode == "none" {
				reason = "dnsmasq_mode=none"
			}
			emitStep("dns-takeover", "skip", fmt.Sprintf("DNS 接管已跳过 (%s)", reason), "")
		}

		emitDone(true, "")
	}
}
