package nodes

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// FixKind identifies the type of remediation to apply.
type FixKind string

const (
	FixKindAddSwap     FixKind = "add_swap"
	FixKindRestartGost FixKind = "restart_gost"
)

// FixProgress is called for each step of a fix operation.
// status is one of: "running" | "ok" | "warning" | "error".
type FixProgress func(step, status, message, detail string)

// RunNodeFix connects to the node via SSH and applies the requested fix,
// calling progress for each step.  It reuses the same BuildAuthMethods /
// NewSSHClient path as deploy/diag so auth is always consistent.
func RunNodeFix(ctx context.Context, node *Node, kp *KeyPair, kind FixKind, params map[string]string, progress FixProgress) error {
	_ = ctx
	client, err := NewSSHClient(node.Host, node.Port, node.Username,
		BuildAuthMethods(node.Password, kp), 30*time.Second)
	if err != nil {
		return fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer client.Close()

	switch kind {
	case FixKindAddSwap:
		return fixAddSwap(client, params, progress)
	case FixKindRestartGost:
		return fixRestartGost(client, progress)
	default:
		return fmt.Errorf("未知修复类型: %s", kind)
	}
}

// ── sudoPrefix ────────────────────────────────────────────────────────────────
// Detects whether the SSH session is running as root (uid 0).
// Returns "sudo " for non-root users (who are expected to have passwordless
// sudo, as is standard on cloud VMs like Azure azureuser / AWS ubuntu).
// Returns "" when already root so no redundant prefix is added.
func sudoPrefix(client *SSHClient) string {
	out, _ := client.Run("id -u 2>/dev/null")
	if strings.TrimSpace(out) == "0" {
		return ""
	}
	return "sudo "
}

// ── add_swap ──────────────────────────────────────────────────────────────────
// Creates a 1 GB swap file at /swapfile, activates it, persists it in
// /etc/fstab, and tunes vm.swappiness to 10.

func fixAddSwap(client *SSHClient, _ map[string]string, progress FixProgress) error {
	const (
		sizeMB   = 1024
		swapPath = "/swapfile"
	)

	sudo := sudoPrefix(client)

	// 1. Check for existing swap
	progress("check", "running", "检查现有交换分区…", "")
	out, _ := client.Run(sudo + "swapon --show 2>/dev/null")
	if strings.TrimSpace(out) != "" {
		progress("check", "ok", "服务器已有交换分区，无需重复创建", strings.TrimSpace(out))
		return nil
	}
	progress("check", "ok", "未发现交换分区，准备新建 1 GB Swap", "")

	// 2. Check free disk space (need sizeMB + 200 MB buffer)
	progress("disk", "running", "检查磁盘可用空间…", "")
	diskOut, _ := client.Run("df -m / | awk 'NR==2{print $4}'")
	var avail int
	fmt.Sscanf(strings.TrimSpace(diskOut), "%d", &avail)
	if avail > 0 && avail < sizeMB+200 {
		progress("disk", "error",
			fmt.Sprintf("磁盘空闲仅 %d MB，至少需要 %d MB，无法创建交换文件", avail, sizeMB+200), "")
		return fmt.Errorf("磁盘空间不足")
	}
	if avail > 0 {
		progress("disk", "ok", fmt.Sprintf("磁盘空闲 %d MB，空间充足", avail), "")
	}

	// 3. Create swap file (fallocate preferred, dd as fallback for file-systems
	//    that don't support preallocation, e.g. btrfs)
	progress("create", "running", fmt.Sprintf("创建交换文件 %s (1 GB)…", swapPath), "")
	createCmd := fmt.Sprintf(
		"%sfallocate -l %dM %s 2>/dev/null || %sdd if=/dev/zero of=%s bs=1M count=%d status=none 2>&1",
		sudo, sizeMB, swapPath, sudo, swapPath, sizeMB,
	)
	if out, err := client.Run(createCmd); err != nil {
		progress("create", "error", "创建交换文件失败", strings.TrimSpace(out))
		return fmt.Errorf("创建交换文件失败: %w", err)
	}
	progress("create", "ok", fmt.Sprintf("交换文件已创建: %s", swapPath), "")

	// 4. Permissions
	progress("chmod", "running", "设置文件权限 600…", "")
	if out, err := client.Run(fmt.Sprintf("%schmod 600 %s", sudo, swapPath)); err != nil {
		progress("chmod", "error", "设置权限失败", strings.TrimSpace(out))
		return fmt.Errorf("chmod 失败: %w", err)
	}
	progress("chmod", "ok", "权限已设置为 600", "")

	// 5. Format
	progress("mkswap", "running", "格式化交换分区…", "")
	if out, err := client.Run(fmt.Sprintf("%smkswap %s 2>&1", sudo, swapPath)); err != nil {
		progress("mkswap", "error", "格式化失败", strings.TrimSpace(out))
		return fmt.Errorf("mkswap 失败: %w", err)
	}
	progress("mkswap", "ok", "格式化完成", "")

	// 6. Activate
	progress("swapon", "running", "启用交换分区…", "")
	if out, err := client.Run(fmt.Sprintf("%sswapon %s 2>&1", sudo, swapPath)); err != nil {
		progress("swapon", "error", "启用失败", strings.TrimSpace(out))
		return fmt.Errorf("swapon 失败: %w", err)
	}
	progress("swapon", "ok", "交换分区已激活", "")

	// 7. Persist across reboots
	progress("fstab", "running", "写入 /etc/fstab（开机自动挂载）…", "")
	// Use tee to append — avoids shell redirection privilege issues with sudo.
	fstabCmd := fmt.Sprintf(
		"grep -q '%s' /etc/fstab 2>/dev/null || echo '%s none swap sw 0 0' | %stee -a /etc/fstab > /dev/null",
		swapPath, swapPath, sudo,
	)
	if out, err := client.Run(fstabCmd); err != nil {
		progress("fstab", "warning", "写入 fstab 失败，重启后 Swap 不会自动挂载", strings.TrimSpace(out))
	} else {
		progress("fstab", "ok", "已写入 /etc/fstab，开机自动挂载", "")
	}

	// 8. Tune swappiness (10 = use swap only under real pressure)
	progress("swappiness", "running", "优化 Swap 使用策略 (vm.swappiness=10)…", "")
	client.Run(sudo + "sysctl -w vm.swappiness=10 2>/dev/null")
	// Append via tee to avoid redirection privilege issues with sudo.
	client.Run("grep -q 'vm.swappiness' /etc/sysctl.conf 2>/dev/null || echo 'vm.swappiness=10' | " + sudo + "tee -a /etc/sysctl.conf > /dev/null 2>&1")
	progress("swappiness", "ok", "vm.swappiness 已设为 10（仅在必要时换页）", "")

	// 9. Report final state
	freeOut, _ := client.Run("free -h 2>/dev/null | grep -i swap")
	progress("finish", "ok", "Swap 配置完成，内存压力已降低", strings.TrimSpace(freeOut))
	return nil
}

// ── restart_gost ──────────────────────────────────────────────────────────────
// Clears the systemd restart counter, restarts the gost service, and
// verifies it comes up active.

func fixRestartGost(client *SSHClient, progress FixProgress) error {
	sudo := sudoPrefix(client)

	// 1. Reset fail counter so systemd doesn't refuse to start
	progress("reset", "running", "重置 gost 故障计数…", "")
	client.Run(sudo + "systemctl reset-failed gost 2>/dev/null")
	progress("reset", "ok", "故障计数已清零", "")

	// 2. Restart
	progress("restart", "running", "重启 gost 服务…", "")
	if out, err := client.Run(sudo + "systemctl restart gost 2>&1"); err != nil {
		progress("restart", "error", "重启失败", strings.TrimSpace(out))
		return fmt.Errorf("重启 gost 失败: %w", err)
	}
	progress("restart", "ok", "gost 服务重启指令已发出", "")

	// 3. Wait briefly then verify
	progress("verify", "running", "等待服务启动（2 s）…", "")
	time.Sleep(2 * time.Second)
	out, err := client.Run(sudo + "systemctl is-active gost 2>&1")
	state := strings.TrimSpace(out)
	if err != nil || state != "active" {
		progress("verify", "warning",
			fmt.Sprintf("服务状态: %s（可能仍在启动，可重新诊断确认）", state), "")
	} else {
		progress("verify", "ok", "gost 服务运行正常 (active)", "")
	}
	return nil
}
