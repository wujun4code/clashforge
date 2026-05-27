package quickstart

import (
	"context"
	"fmt"
	"time"

	"github.com/wujun4code/clashforge/internal/workernode"
)

// WorkersPipeline deploys a Cloudflare Worker VLESS+WS+TLS node.
// It reuses the existing workernode.Deploy() function end-to-end.
type WorkersPipeline struct {
	deps Deps
}

func (p *WorkersPipeline) Run(ctx context.Context, req *DeployRequest, out EventWriter) error {
	// ── Phase 1: Create Cloudflare Worker + bind custom domain ───────────────
	emit(out, PhaseWorkerDeploy, "create_worker", StatusRunning, "正在创建 Cloudflare Worker...")

	wd := req.WorkersDomain
	if wd.WorkerName == "" {
		wd.WorkerName = "clashforge-node"
	}
	nodeName := req.NodeName
	if nodeName == "" {
		nodeName = wd.CustomDomain
	}

	deployReq := &workernode.CreateRequest{
		Name:        nodeName,
		WorkerName:  wd.WorkerName,
		CFToken:     req.Cloudflare.Token,
		CFAccountID: req.Cloudflare.AccountID,
		CFZoneID:    wd.ZoneID,
		Hostname:    wd.CustomDomain,
	}

	deployCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	node, err := workernode.Deploy(deployCtx, deployReq)
	if err != nil {
		emit(out, PhaseWorkerDeploy, "create_worker", StatusError, "Worker 部署失败", err.Error())
		return fmt.Errorf("deploy worker: %w", err)
	}

	// Persist node to store
	if err := p.deps.WorkerStore.Create(node); err != nil {
		emit(out, PhaseWorkerDeploy, "create_worker", StatusError, "保存节点失败", err.Error())
		return fmt.Errorf("store worker node: %w", err)
	}

	emit(out, PhaseWorkerDeploy, "create_worker", StatusOK,
		fmt.Sprintf("Worker 已上线：%s", node.Hostname))

	// ── Phase 2: Export Clash YAML and import as subscription ────────────────
	emit(out, PhaseImport, "export_yaml", StatusRunning, "生成订阅配置...")

	clashYAML, err := workernode.ExportClashProxy(node)
	if err != nil {
		emit(out, PhaseImport, "export_yaml", StatusError, "导出 Clash 配置失败", err.Error())
		return fmt.Errorf("export clash proxy: %w", err)
	}

	emit(out, PhaseImport, "import_sub", StatusRunning, "导入订阅到 ClashForge...")
	subID, nodeCount, _, err := p.deps.SubManager.ImportStatic(clashYAML)
	if err != nil {
		emit(out, PhaseImport, "import_sub", StatusError, "订阅导入失败", err.Error())
		return fmt.Errorf("import subscription: %w", err)
	}
	emit(out, PhaseImport, "import_sub", StatusOK,
		fmt.Sprintf("订阅已导入（%d 个节点，ID: %s）", nodeCount, subID))

	// ── Phase 3: Auto-configure ClashForge ───────────────────────────────────
	if err := autoConfigureClashForge(ctx, p.deps, out); err != nil {
		return err
	}

	// ── Phase 4: Verify connectivity ─────────────────────────────────────────
	return verifyConnectivity(ctx, out)
}
