package quickstart

import (
	"context"
	"fmt"
	"time"

	"github.com/wujun4code/clashforge/internal/publish"
)

// VPSPipeline provisions a gost SOCKS5+TLS node on a VPS.
type VPSPipeline struct {
	deps Deps
}

func (p *VPSPipeline) Run(ctx context.Context, req *DeployRequest, out EventWriter) error {
	if req.VPS == nil {
		return fmt.Errorf("vps credentials required for vps deploy type")
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
	nodeHost := req.NodePrefix + "." + req.Cloudflare.ZoneName
	if req.NodePrefix == "" {
		nodeHost = req.Cloudflare.ZoneName
	}
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

	emit(out, PhaseProvision, "start_gost", StatusRunning, "写入 gost 配置并启动服务...")
	if err := WriteGostConfig(sshClient); err != nil {
		emit(out, PhaseProvision, "start_gost", StatusError, "gost 服务启动失败", err.Error())
		return fmt.Errorf("write gost config: %w", err)
	}
	emit(out, PhaseProvision, "start_gost", StatusOK, "gost.service 已启动")

	// ── Phase 9: Import subscription ─────────────────────────────────────────
	emit(out, PhaseImport, "build_yaml", StatusRunning, "生成订阅配置...")
	nodeName := req.NodeName
	if nodeName == "" {
		nodeName = nodeHost
	}
	clashYAML, err := BuildSocks5ClashYAML(nodeName, nodeHost, 443)
	if err != nil {
		emit(out, PhaseImport, "build_yaml", StatusError, "Clash YAML 生成失败", err.Error())
		return fmt.Errorf("build socks5 yaml: %w", err)
	}

	emit(out, PhaseImport, "import_sub", StatusRunning, "导入订阅到 ClashForge...")
	subID, nodeCount, _, err := p.deps.SubManager.ImportStatic(clashYAML)
	if err != nil {
		emit(out, PhaseImport, "import_sub", StatusError, "订阅导入失败", err.Error())
		return fmt.Errorf("import subscription: %w", err)
	}
	emit(out, PhaseImport, "import_sub", StatusOK,
		fmt.Sprintf("订阅已导入（%d 个节点，ID: %s）", nodeCount, subID))

	// ── Phase 10: Auto-configure ClashForge ──────────────────────────────────
	if err := autoConfigureClashForge(ctx, p.deps, out, nil); err != nil {
		return err
	}

	// ── Phase 11: Verify connectivity ────────────────────────────────────────
	return verifyConnectivity(ctx, out)
}
