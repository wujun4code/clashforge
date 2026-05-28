package quickstart

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/wujun4code/clashforge/internal/publish"
	"github.com/wujun4code/clashforge/internal/subscription"
	"github.com/wujun4code/clashforge/internal/workernode"
)

// WorkersPipeline deploys a Cloudflare Worker VLESS+WS+TLS proxy node,
// then publishes a loyalSoldier subscription via a separate publish Worker,
// imports the subscription URL into ClashForge, auto-configures and verifies.
type WorkersPipeline struct {
	deps Deps
}

func (p *WorkersPipeline) Run(ctx context.Context, req *DeployRequest, out EventWriter) error {
	cf := req.Cloudflare

	// ── Phase 1: Deploy proxy Worker ─────────────────────────────────────────
	emit(out, PhaseWorkerDeploy, "create_proxy_worker", StatusRunning, "正在部署代理 Worker...")

	// Auto-generate readable random names for both workers
	proxyWorkerName := randName("cf-proxy")
	proxyHostname := proxyWorkerName + "." + cf.ZoneName
	nodeName := req.NodeName
	if nodeName == "" {
		nodeName = proxyHostname
	}

	deployCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	proxyNode, err := workernode.Deploy(deployCtx, &workernode.CreateRequest{
		Name:        nodeName,
		WorkerName:  proxyWorkerName,
		CFToken:     cf.Token,
		CFAccountID: cf.AccountID,
		CFZoneID:    cf.ZoneID,
		Hostname:    proxyHostname,
	})
	if err != nil {
		emit(out, PhaseWorkerDeploy, "create_proxy_worker", StatusError, "代理 Worker 部署失败", err.Error())
		return fmt.Errorf("deploy proxy worker: %w", err)
	}

	if err := p.deps.WorkerStore.Create(proxyNode); err != nil {
		emit(out, PhaseWorkerDeploy, "create_proxy_worker", StatusError, "保存节点失败", err.Error())
		return fmt.Errorf("store proxy worker node: %w", err)
	}

	emit(out, PhaseWorkerDeploy, "create_proxy_worker", StatusOK,
		fmt.Sprintf("代理 Worker 已上线：%s", proxyNode.Hostname))

	// ── Phase 2: Deploy subscription publish Worker ───────────────────────────
	emit(out, PhasePublish, "create_pub_worker", StatusRunning, "正在部署订阅发布 Worker...")

	cfClient, err := publish.NewCloudflareClient(cf.Token)
	if err != nil {
		emit(out, PhasePublish, "create_pub_worker", StatusError, "初始化 CF 客户端失败", err.Error())
		return fmt.Errorf("init cf client: %w", err)
	}

	pubWorkerName := randName("cf-sub")
	pubBaseDomain := ResolvePublishBaseDomain(proxyHostname, cf.ZoneName)
	pubHostname := BuildPublishHostname(pubWorkerName, proxyHostname, cf.ZoneName)
	if pubBaseDomain != "" {
		emit(out, PhasePublish, "create_pub_worker", StatusInfo,
			fmt.Sprintf("发布订阅将绑定随机二级域名：*.%s（与节点域名分离）", pubBaseDomain))
	}

	// Create KV namespace for the publish worker
	nsResult, err := cfClient.CreateOrReuseNamespace(ctx, cf.AccountID, pubWorkerName)
	if err != nil {
		emit(out, PhasePublish, "create_pub_worker", StatusError, "创建 KV 命名空间失败", err.Error())
		return fmt.Errorf("create kv namespace: %w", err)
	}

	// Generate an access token for KV reads/writes
	pubAccessToken := randHex(32)

	// Deploy the publish worker script
	deployResult, err := cfClient.DeployWorkerScript(ctx, cf.AccountID, pubWorkerName, nsResult.NamespaceID, pubAccessToken)
	if err != nil {
		emit(out, PhasePublish, "create_pub_worker", StatusError, "部署发布 Worker 脚本失败", err.Error())
		return fmt.Errorf("deploy publish worker script: %w", err)
	}

	// Bind custom domain
	bindResult, err := cfClient.BindWorkerDomain(ctx, cf.AccountID, cf.ZoneID, pubWorkerName, pubHostname)
	if err != nil {
		emit(out, PhasePublish, "create_pub_worker", StatusError, "绑定发布 Worker 域名失败", err.Error())
		return fmt.Errorf("bind publish worker domain: %w", err)
	}

	// permanentBase: stored as the subscription access URL.
	// Custom domain is preferred so Clash clients use a proper hostname once DNS propagates.
	permanentBase := publish.PickWorkerBaseURL(bindResult.WorkerURL, deployResult.WorkerDevURL)

	// Quick reachability check — informational only; we don't use the HTTP endpoint for uploading.
	verifyResult, _ := publish.VerifyWorkerEndpoint(ctx, bindResult.WorkerURL, deployResult.WorkerDevURL, pubAccessToken)
	if verifyResult.OK {
		emit(out, PhasePublish, "create_pub_worker", StatusOK,
			fmt.Sprintf("发布 Worker HTTP 端点已就绪：%s", verifyResult.UsedURL))
	} else {
		emit(out, PhasePublish, "create_pub_worker", StatusInfo,
			"发布 Worker HTTP 端点暂不可达（DNS 可能尚未传播），将直接写入 KV")
	}

	// ── Phase 3: Generate subscription YAML ──────────────────────────────────
	emit(out, PhasePublish, "gen_sub", StatusRunning, "正在生成 loyalSoldier 订阅...")

	templateYAML, err := publish.TemplateByID("loyalsoldier_standard")
	if err != nil {
		emit(out, PhasePublish, "gen_sub", StatusError, "加载订阅模板失败", err.Error())
		return fmt.Errorf("load loyalsoldier template: %w", err)
	}
	emit(out, PhasePublish, "gen_sub", StatusInfo, "已固定使用 loyalSoldier 模板（不使用 mihomo 运行时模板）")

	mergeNodes := []publish.MergeNode{
		{
			ID:             proxyNode.ID,
			Name:           proxyNode.Name,
			NodeType:       "worker",
			WorkerUUID:     proxyNode.WorkerUUID,
			WorkerHostname: proxyNode.Hostname,
		},
	}

	subYAML, err := publish.MergeTemplateWithNodes(templateYAML, mergeNodes)
	if err != nil {
		emit(out, PhasePublish, "gen_sub", StatusError, "生成订阅 YAML 失败", err.Error())
		return fmt.Errorf("merge template with nodes: %w", err)
	}

	emit(out, PhasePublish, "gen_sub", StatusOK, "订阅配置生成完成")

	// ── Phase 4: Upload subscription to publish Worker ────────────────────────
	emit(out, PhasePublish, "upload_sub", StatusRunning, "正在上传订阅到发布 Worker...")

	baseName := publish.SanitizeBaseName(nodeName)
	now := time.Now()
	fileName := publish.VersionedFileName(baseName, 1, now)
	// accessURL uses the permanent custom-domain base so Clash clients can reach it
	// after DNS propagates. The token is embedded so clients can fetch without extra headers.
	accessURL := fmt.Sprintf("%s/%s?token=%s", permanentBase, url.PathEscape(fileName), url.QueryEscape(pubAccessToken))

	// Write directly to the KV namespace via api.cloudflare.com — this bypasses the
	// Worker HTTP endpoint entirely and works even when workers.dev is blocked or the
	// custom domain DNS hasn't propagated yet.
	uploadErr := cfClient.WriteKVValue(ctx, cf.AccountID, nsResult.NamespaceID, fileName, subYAML)
	if uploadErr != nil {
		// Fallback: try the Worker HTTP endpoint if it was reachable during verify.
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
		WorkerURL:    bindResult.WorkerURL,
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

	// ── Phase 5: Import subscription URL into ClashForge ─────────────────────
	emit(out, PhaseImport, "add_sub", StatusRunning, "正在导入订阅到 ClashForge...")

	subID, err := p.deps.SubManager.Add(subscription.Subscription{
		Name:    nodeName + "（CF Workers）",
		Type:    "url",
		URL:     accessURL,
		Enabled: true,
	})
	if err != nil {
		emit(out, PhaseImport, "add_sub", StatusError, "添加订阅失败", err.Error())
		return fmt.Errorf("add subscription: %w", err)
	}

	// Attempt to sync — may fail if router can't reach the publish worker
	// before proxy is active. Fall back to importing from generated YAML directly.
	activeSubID := subID
	activeSubName := nodeName + "（CF Workers）"
	if syncErr := p.deps.SubManager.SyncUpdate(subID); syncErr != nil {
		emit(out, PhaseImport, "add_sub", StatusWarning,
			"URL 同步失败（路由器尚未走代理），已从本地 YAML 缓存节点", syncErr.Error())
		staticID, _, _, importErr := p.deps.SubManager.ImportStatic(subYAML)
		if importErr != nil {
			emit(out, PhaseImport, "add_sub", StatusError, "本地缓存也失败", importErr.Error())
			return fmt.Errorf("import static fallback: %w", importErr)
		}
		activeSubID = staticID
		activeSubName = nodeName + "（CF Workers-本地缓存）"
	} else {
		emit(out, PhaseImport, "add_sub", StatusOK,
			fmt.Sprintf("订阅已导入并同步（ID: %s）", subID))
	}

	// ── Phase 6: Auto-configure ClashForge + verify ───────────────────────────
	// Only configure with the nodes from THIS quickstart deployment — do not pull
	// in existing subscriptions from the system, which would cause duplicate proxy
	// names if the user already has nodes with the same display names.
	newNodes, parseErr := subscription.Parse([]byte(subYAML))
	if parseErr != nil || len(newNodes) == 0 {
		// If parsing the YAML fails, fall back to the sub we just imported
		newNodes, _ = p.deps.SubManager.GetCachedNodes(subID)
	}
	if err := autoConfigureClashForge(ctx, p.deps, out, activeSubID, activeSubName, subYAML, newNodes); err != nil {
		return err
	}

	return verifyConnectivity(ctx, out)
}
