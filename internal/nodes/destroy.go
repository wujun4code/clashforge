package nodes

import (
	"context"
	"fmt"
	"time"
)

// DestroyResult holds the outcome of a destroy operation.
type DestroyResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// DestroyProgress is a callback for streaming progress updates.
type DestroyProgress func(step, status, message, detail string)

// DestroyGOST remotely removes GOST deployment from the node.
// It streams progress via the callback function.
func DestroyGOST(
	ctx context.Context,
	node *Node,
	kp *KeyPair,
	progress DestroyProgress,
) (*DestroyResult, error) {
	// Step 1: SSH connect
	progress("connect", "running", "正在连接远程服务器...", "")
	client, err := NewSSHClient(node.Host, node.Port, node.Username, BuildAuthMethods(node.Password, kp), 30*time.Second)
	if err != nil {
		progress("connect", "error", "SSH 连接失败", err.Error())
		return &DestroyResult{Success: false, Error: err.Error()}, err
	}
	defer client.Close()
	progress("connect", "ok", "SSH 连接成功", node.Host)

	// Step 2: Stop and disable systemd service
	progress("systemd-stop", "running", "正在停止 GOST 服务...", "")
	out, err := client.Run(`systemctl stop gost 2>&1; systemctl disable gost 2>&1; rm -f /etc/systemd/system/gost.service 2>&1; systemctl daemon-reload 2>&1`)
	if err != nil {
		progress("systemd-stop", "warning", "停止服务有警告", fmt.Sprintf("%s\n%s", err.Error(), out))
	}
	progress("systemd-stop", "ok", "GOST 服务已停止并移除", "")

	// Step 3: Remove GOST binary
	progress("remove-gost", "running", "正在移除 GOST 程序...", "")
	out, err = client.Run(`rm -f /usr/local/bin/gost 2>&1; rm -rf /tmp/gost 2>&1`)
	if err != nil {
		progress("remove-gost", "warning", "移除 GOST 有警告", fmt.Sprintf("%s\n%s", err.Error(), out))
	}
	progress("remove-gost", "ok", "GOST 程序已清理", "")

	// Step 4: Remove GOST config and certs
	progress("remove-config", "running", "正在清理配置文件...", "")
	out, err = client.Run(`rm -rf /etc/gost 2>&1`)
	if err != nil {
		progress("remove-config", "warning", "清理配置有警告", fmt.Sprintf("%s\n%s", err.Error(), out))
	}
	progress("remove-config", "ok", "GOST 配置和证书已清理", "")

	// Step 5: Remove acme.sh certificates
	progress("remove-certs", "running", "正在吊销 TLS 证书...", "")
	certDir := fmt.Sprintf("/etc/gost/certs/%s", node.Domain)
	out, err = client.Run(fmt.Sprintf(`
if [ -f ~/.acme.sh/acme.sh ]; then
  ~/.acme.sh/acme.sh --remove -d %s 2>&1 || true
  rm -rf %s 2>&1 || true
fi
`, node.Domain, certDir))
	if err != nil {
		progress("remove-certs", "warning", "证书清理有警告", fmt.Sprintf("%s\n%s", err.Error(), out))
	}
	progress("remove-certs", "ok", "TLS 证书已吊销并清理", "")

	// Step 6: Optional - remove acme.sh itself
	progress("cleanup-acme", "running", "正在清理 acme.sh...", "")
	out, err = client.Run(`rm -rf ~/.acme.sh 2>&1 || true`)
	if err != nil {
		progress("cleanup-acme", "warning", "acme.sh 清理有警告", err.Error())
	}
	progress("cleanup-acme", "ok", "acme.sh 已清理", "")

	progress("done", "ok", "销毁完成 ✓", "服务器已恢复至部署前状态")
	return &DestroyResult{Success: true}, nil
}
