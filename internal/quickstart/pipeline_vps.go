package quickstart

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/wujun4code/clashforge/internal/publish"
	"github.com/wujun4code/clashforge/internal/subscription"
)

// VPSPipeline provisions a gost SOCKS5+TLS node on a VPS.
type VPSPipeline struct {
	deps Deps
}

func (p *VPSPipeline) Run(ctx context.Context, req *DeployRequest, out EventWriter) error {
	// ── Compute node host once ────────────────────────────────────────────────
	nodeHost := req.NodePrefix + "." + req.Cloudflare.ZoneName
	if req.NodePrefix == "" {
		nodeHost = req.Cloudflare.ZoneName
	}
	nodeName := req.NodeName
	if nodeName == "" {
		nodeName = nodeHost
	}

	// ── Phase 0: Skip to import if caller already verified connectivity ──────
	if req.ForceImport {
		emit(out, PhaseExistingCheck, "skip_deploy", StatusOK, "代理连通性已验证，跳过部署，直接生成订阅")
		return p.importAndConfigure(ctx, req, out, nodeName, nodeHost)
	}

	if req.VPS == nil {
		return fmt.Errorf("vps credentials required for vps deploy type")
	}

	// ── Phase 0b: Reconfigure auth only (no reinstall/cert/dns) ──────────────
	if req.EnsureProxyAuth {
		emit(out, PhaseProvision, "ensure_proxy_auth", StatusRunning, "仅更新代理认证（用户名/密码）...")

		sshClient, err := DialSSH(req.VPS)
		if err != nil {
			emit(out, PhaseSSHTest, "connect", StatusError, "SSH 连接失败", err.Error())
			return fmt.Errorf("ssh connect: %w", err)
		}
		defer sshClient.Close()
		emit(out, PhaseSSHTest, "connect", StatusOK, fmt.Sprintf("SSH 已连接到 %s", req.VPS.Host))

		proxyUser := "proxy"
		proxyPass := generatePass(20)
		req.ProxyUser = proxyUser
		req.ProxyPassword = proxyPass

		if err := WriteGostConfig(sshClient, proxyUser, proxyPass); err != nil {
			emit(out, PhaseProvision, "ensure_proxy_auth", StatusError, "代理认证更新失败", err.Error())
			return fmt.Errorf("ensure proxy auth: %w", err)
		}
		emit(out, PhaseProvision, "ensure_proxy_auth", StatusOK, "代理认证已更新并重启 gost")
		return p.importAndConfigure(ctx, req, out, nodeName, nodeHost)
	}

	// ── Phase 0: Check for existing healthy deployment ────────────────────────
	result := CheckExistingDeployment(ctx, nodeHost, req.VPS.Host, out)
	// Reuse only when the node already has saved credentials from a prior QuickStart deploy.
	// Without saved creds we proceed with a full deploy to generate and store them.
	if result.Reusable && req.ProxyUser != "" {
		return p.importAndConfigure(ctx, req, out, nodeName, nodeHost)
	}

	// ── Phase 1: SSH connect ─────────────────────────────────────────────────
	emit(out, PhaseSSHTest, "connect", StatusRunning, "正在连接 VPS...")
	sshClient, err := DialSSH(req.VPS)
	if err != nil {
		emit(out, PhaseSSHTest, "connect", StatusError, "SSH 连接失败", err.Error())
		return fmt.Errorf("ssh connect: %w", err)
	}
	defer sshClient.Close()
	emit(out, PhaseSSHTest, "connect", StatusOK, fmt.Sprintf("SSH 已连接到 %s", req.VPS.Host))

	// ── Phase 2: Detect VPS environment ──────────────────────────────────────
	emit(out, PhaseEnvDetect, "detect_os", StatusRunning, "检测 VPS 环境...")
	env, err := DetectEnv(sshClient)
	if err != nil {
		emit(out, PhaseEnvDetect, "detect_os", StatusWarning, "环境检测部分失败，继续部署", err.Error())
		env = &EnvInfo{Arch: "amd64", HasSystemd: true}
	}
	emit(out, PhaseEnvDetect, "detect_os", StatusOK,
		fmt.Sprintf("OS：%s %s (%s)", env.OS, env.OSVersion, env.Arch))

	if env.Port443In {
		emit(out, PhaseEnvDetect, "port_check", StatusInfo,
			"443 端口当前已被占用，将在下一步自动清理")
	} else {
		emit(out, PhaseEnvDetect, "port_check", StatusOK, "443 端口未被占用")
	}

	// ── Phase 3: Clean up any existing gost / port-443 occupant ─────────────
	emit(out, PhaseProvision, "cleanup_gost", StatusRunning, "清理旧 gost 服务及占用 443 端口的进程...")
	CleanupGost(sshClient)
	emit(out, PhaseProvision, "cleanup_gost", StatusOK, "旧服务已停止，443 端口已释放")

	// ── Phase 4: Install gost ────────────────────────────────────────────────
	emit(out, PhaseProvision, "install_gost", StatusRunning, "下载并安装 gost...")
	if err := InstallGost(sshClient, env); err != nil {
		emit(out, PhaseProvision, "install_gost", StatusError, "gost 安装失败", err.Error())
		return fmt.Errorf("install gost: %w", err)
	}
	emit(out, PhaseProvision, "install_gost", StatusOK,
		fmt.Sprintf("gost v%s 安装完成", GostVersion))

	// ── Phase 5: Open firewall port 443 ─────────────────────────────────────
	if env.Firewall != "none" && env.Firewall != "" {
		emit(out, PhaseProvision, "firewall", StatusRunning,
			fmt.Sprintf("开放 443 端口（%s）...", env.Firewall))
		if err := AllowPort443(sshClient, env.Firewall); err != nil {
			emit(out, PhaseProvision, "firewall", StatusWarning,
				"防火墙规则设置失败（可能需要手动开放 443）", err.Error())
		} else {
			emit(out, PhaseProvision, "firewall", StatusOK, "443 端口已开放")
		}
	}

	// ── Phase 6: CF DNS A record ─────────────────────────────────────────────
	emit(out, PhaseCertDNS, "create_a_record", StatusRunning,
		fmt.Sprintf("在 Cloudflare 创建 A 记录：%s → %s...", nodeHost, req.VPS.Host))

	cf, err := publish.NewCloudflareClient(req.Cloudflare.Token)
	if err != nil {
		emit(out, PhaseCertDNS, "create_a_record", StatusError, "Cloudflare 客户端初始化失败", err.Error())
		return fmt.Errorf("new cf client: %w", err)
	}

	aRecordID, err := cf.CreateDNSARecord(ctx, req.Cloudflare.ZoneID, nodeHost, req.VPS.Host)
	if err != nil {
		emit(out, PhaseCertDNS, "create_a_record", StatusError, "创建 A 记录失败", err.Error())
		return fmt.Errorf("create dns a record: %w", err)
	}
	_ = aRecordID // kept for potential cleanup
	emit(out, PhaseCertDNS, "create_a_record", StatusOK,
		fmt.Sprintf("A 记录已创建：%s → %s", nodeHost, req.VPS.Host))

	// ── Phase 7: Issue Let's Encrypt certificate ──────────────────────────────
	emit(out, PhaseCertDNS, "issue_cert", StatusRunning, "申请 Let's Encrypt 证书（DNS-01）...")

	certCtx, certCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer certCancel()

	certPair, err := IssueCert(certCtx, nodeHost, req.Cloudflare.Token, req.Cloudflare.ZoneID,
		func(msg string) {
			emit(out, PhaseCertDNS, "issue_cert", StatusInfo, msg)
		},
	)
	if err != nil {
		emit(out, PhaseCertDNS, "issue_cert", StatusError, "证书申请失败", err.Error())
		return fmt.Errorf("issue cert: %w", err)
	}
	emit(out, PhaseCertDNS, "issue_cert", StatusOK, "TLS 证书申请成功（有效期 90 天）")

	// ── Phase 8: Upload cert + write gost config + start service ─────────────
	emit(out, PhaseProvision, "upload_cert", StatusRunning, "上传证书到 VPS...")
	if err := UploadCert(sshClient, certPair.FullChainPEM, certPair.PrivKeyPEM); err != nil {
		emit(out, PhaseProvision, "upload_cert", StatusError, "证书上传失败", err.Error())
		return fmt.Errorf("upload cert: %w", err)
	}
	emit(out, PhaseProvision, "upload_cert", StatusOK, "证书已上传至 /etc/gost/cert/")

	// Generate proxy credentials and store in req so the handler can persist them.
	proxyUser := "proxy"
	proxyPass := generatePass(20)
	req.ProxyUser = proxyUser
	req.ProxyPassword = proxyPass

	emit(out, PhaseProvision, "start_gost", StatusRunning, "写入 gost 配置（含认证）并启动服务...")
	if err := WriteGostConfig(sshClient, proxyUser, proxyPass); err != nil {
		emit(out, PhaseProvision, "start_gost", StatusError, "gost 服务启动失败", err.Error())
		return fmt.Errorf("write gost config: %w", err)
	}
	emit(out, PhaseProvision, "start_gost", StatusOK, "gost.service 已启动（HTTP CONNECT+TLS，已设置认证）")

	return p.importAndConfigure(ctx, req, out, nodeName, nodeHost)
}

// importAndConfigure deploys a subscription publish Worker, uploads the
// loyalSoldier subscription, imports it as a URL subscription, then
// auto-configures ClashForge and verifies connectivity.
// This mirrors the Workers pipeline's phases 2–6 exactly; only the proxy
// entry differs (HTTP CONNECT+TLS instead of VLESS+WS+TLS).
func (p *VPSPipeline) importAndConfigure(ctx context.Context, req *DeployRequest, out EventWriter, nodeName, nodeHost string) error {
	cf := req.Cloudflare

	// ── Phase: Deploy subscription publish Worker ─────────────────────────────
	emit(out, PhasePublish, "create_pub_worker", StatusRunning, "正在部署订阅发布 Worker...")

	cfClient, err := publish.NewCloudflareClient(cf.Token)
	if err != nil {
		emit(out, PhasePublish, "create_pub_worker", StatusError, "初始化 CF 客户端失败", err.Error())
		return fmt.Errorf("init cf client: %w", err)
	}

	pubWorkerName := randName("cf-sub")
	pubBaseDomain := ResolvePublishBaseDomain(nodeHost, cf.ZoneName)
	pubHostname := BuildPublishHostname(pubWorkerName, nodeHost, cf.ZoneName)
	if pubBaseDomain != "" {
		emit(out, PhasePublish, "create_pub_worker", StatusInfo,
			fmt.Sprintf("发布订阅将绑定随机二级域名：*.%s（与节点域名分离）", pubBaseDomain))
	}

	nsResult, err := cfClient.CreateOrReuseNamespace(ctx, cf.AccountID, pubWorkerName)
	if err != nil {
		emit(out, PhasePublish, "create_pub_worker", StatusError, "创建 KV 命名空间失败", err.Error())
		return fmt.Errorf("create kv namespace: %w", err)
	}

	pubAccessToken := randHex(32)

	deployResult, err := cfClient.DeployWorkerScript(ctx, cf.AccountID, pubWorkerName, nsResult.NamespaceID, pubAccessToken)
	if err != nil {
		emit(out, PhasePublish, "create_pub_worker", StatusError, "部署发布 Worker 脚本失败", err.Error())
		return fmt.Errorf("deploy publish worker script: %w", err)
	}

	// Publish worker must bind to the same base domain as the node domain.
	if strings.TrimSpace(cf.ZoneID) == "" {
		emit(out, PhasePublish, "create_pub_worker", StatusError,
			"缺少 Cloudflare ZoneID，无法绑定与节点同一级域名的订阅域名")
		return fmt.Errorf("publish worker bind requires zone_id")
	}
	bindResult, bindErr := cfClient.BindWorkerDomain(ctx, cf.AccountID, cf.ZoneID, pubWorkerName, pubHostname)
	if bindErr != nil {
		emit(out, PhasePublish, "create_pub_worker", StatusError,
			"绑定发布 Worker 域名失败（不会回退 workers.dev）", bindErr.Error())
		return fmt.Errorf("bind publish worker domain: %w", bindErr)
	}
	workerURL := bindResult.WorkerURL

	permanentBase := publish.PickWorkerBaseURL(workerURL, deployResult.WorkerDevURL)

	verifyResult, _ := publish.VerifyWorkerEndpoint(ctx, workerURL, deployResult.WorkerDevURL, pubAccessToken)
	if verifyResult.OK {
		emit(out, PhasePublish, "create_pub_worker", StatusOK,
			fmt.Sprintf("发布 Worker HTTP 端点已就绪：%s", verifyResult.UsedURL))
	} else {
		emit(out, PhasePublish, "create_pub_worker", StatusInfo,
			"发布 Worker HTTP 端点暂不可达（DNS 可能尚未传播），将直接写入 KV")
	}

	// ── Phase: Generate loyalSoldier subscription YAML ────────────────────────
	emit(out, PhasePublish, "gen_sub", StatusRunning, "正在生成 loyalSoldier 订阅...")

	templateYAML, err := publish.TemplateByID("loyalsoldier_standard")
	if err != nil {
		emit(out, PhasePublish, "gen_sub", StatusError, "加载订阅模板失败", err.Error())
		return fmt.Errorf("load loyalsoldier template: %w", err)
	}
	emit(out, PhasePublish, "gen_sub", StatusInfo, "已固定使用 loyalSoldier 模板（不使用 mihomo 运行时模板）")

	// HTTP CONNECT+TLS proxy with basic auth credentials.
	proxyMap := map[string]interface{}{
		"name":             nodeName,
		"type":             "http",
		"server":           nodeHost,
		"port":             443,
		"tls":              true,
		"skip-cert-verify": false,
	}
	if req.ProxyUser != "" {
		proxyMap["username"] = req.ProxyUser
		proxyMap["password"] = req.ProxyPassword
	}
	subYAML, err := publish.MergeTemplateWithNodes(templateYAML, []publish.MergeNode{
		{ID: nodeName, Name: nodeName, NodeType: "imported", ImportedProxy: proxyMap},
	})
	if err != nil {
		emit(out, PhasePublish, "gen_sub", StatusError, "生成订阅 YAML 失败", err.Error())
		return fmt.Errorf("merge template with nodes: %w", err)
	}
	emit(out, PhasePublish, "gen_sub", StatusOK, "订阅配置生成完成")

	// ── Phase: Upload subscription to publish Worker ──────────────────────────
	emit(out, PhasePublish, "upload_sub", StatusRunning, "正在上传订阅到发布 Worker...")

	baseName := publish.SanitizeBaseName(nodeName)
	fileName := publish.VersionedFileName(baseName, 1, time.Now())
	accessURL := fmt.Sprintf("%s/%s?token=%s",
		permanentBase, url.PathEscape(fileName), url.QueryEscape(pubAccessToken))

	uploadErr := cfClient.WriteKVValue(ctx, cf.AccountID, nsResult.NamespaceID, fileName, subYAML)
	if uploadErr != nil {
		if verifyResult.OK && verifyResult.UsedURL != "" {
			uploadErr = publish.UploadContentViaWorker(ctx, verifyResult.UsedURL, fileName, pubAccessToken, subYAML)
		}
	}
	if uploadErr != nil {
		emit(out, PhasePublish, "upload_sub", StatusError, "上传订阅失败", uploadErr.Error())
		return fmt.Errorf("upload subscription: %w", uploadErr)
	}
	emit(out, PhasePublish, "upload_sub", StatusOK,
		fmt.Sprintf("订阅已发布：%s", accessURL))

	syncQuickStartPublishArtifacts(p.deps.PublishStore, out, quickStartPublishSyncInput{
		WorkerName:   pubWorkerName,
		WorkerURL:    workerURL,
		WorkerDevURL: deployResult.WorkerDevURL,
		Hostname:     pubHostname,
		AccountID:    cf.AccountID,
		NamespaceID:  nsResult.NamespaceID,
		ZoneID:       cf.ZoneID,
		AccessToken:  pubAccessToken,
		BaseName:     baseName,
		Version:      1,
		FileName:     fileName,
		AccessURL:    accessURL,
	})

	// ── Phase: Import subscription URL into ClashForge ────────────────────────
	emit(out, PhaseImport, "add_sub", StatusRunning, "正在导入订阅到 ClashForge...")

	subID, err := p.deps.SubManager.Add(subscription.Subscription{
		Name:    nodeName + "（VPS）",
		Type:    "url",
		URL:     accessURL,
		Enabled: true,
	})
	if err != nil {
		emit(out, PhaseImport, "add_sub", StatusError, "添加订阅失败", err.Error())
		return fmt.Errorf("add subscription: %w", err)
	}

	activeSubID := subID
	activeSubName := nodeName + "（VPS）"
	if syncErr := p.deps.SubManager.SyncUpdate(subID); syncErr != nil {
		emit(out, PhaseImport, "add_sub", StatusWarning,
			"URL 同步失败（路由器尚未走代理），已从本地 YAML 缓存节点", syncErr.Error())
		staticID, _, _, importErr := p.deps.SubManager.ImportStatic(subYAML)
		if importErr != nil {
			emit(out, PhaseImport, "add_sub", StatusError, "本地缓存也失败", importErr.Error())
			return fmt.Errorf("import static fallback: %w", importErr)
		}
		activeSubID = staticID
		activeSubName = nodeName + "（VPS-本地缓存）"
	} else {
		emit(out, PhaseImport, "add_sub", StatusOK,
			fmt.Sprintf("订阅已导入并同步（ID: %s）", subID))
	}

	// ── Auto-configure ClashForge + verify ────────────────────────────────────
	newNodes, parseErr := subscription.Parse([]byte(subYAML))
	if parseErr != nil || len(newNodes) == 0 {
		newNodes, _ = p.deps.SubManager.GetCachedNodes(subID)
	}
	if err := autoConfigureClashForge(ctx, p.deps, out, activeSubID, activeSubName, subYAML, newNodes); err != nil {
		return err
	}
	return verifyConnectivity(ctx, out)
}
