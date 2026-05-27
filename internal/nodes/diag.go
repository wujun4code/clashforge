// Package nodes — SSH-based remote node diagnostics.
package nodes

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// NodeDiagCheck is one check item in a node diagnostic run.
type NodeDiagCheck struct {
	ID       string `json:"id"`
	Category string `json:"category"` // "network" | "process" | "system" | "cert"
	Name     string `json:"name"`
	Status   string `json:"status"` // "ok" | "warn" | "error" | "skip"
	Value    string `json:"value,omitempty"`
	Detail   string `json:"detail,omitempty"` // multi-line raw output (collapsible in UI)
	Message  string `json:"message"`
}

// NodeDiagSummary aggregates results from a diagnostic run.
type NodeDiagSummary struct {
	Total int `json:"total"`
	OK    int `json:"ok"`
	Warn  int `json:"warn"`
	Error int `json:"error"`
	Skip  int `json:"skip"`
}

// RunNodeDiag connects to the node via SSH and runs a suite of diagnostic
// checks, calling onCheck for each result as soon as it completes.
// The returned summary covers all checks including the local TCP probe.
func RunNodeDiag(ctx context.Context, node *Node, kp *KeyPair, onCheck func(NodeDiagCheck)) (NodeDiagSummary, error) {
	var checks []NodeDiagCheck
	emit := func(c NodeDiagCheck) {
		checks = append(checks, c)
		onCheck(c)
	}

	// ── 1. Local TCP probe (port 443) — no SSH required ──────────────────
	emit(checkTCP(ctx, node.Host, 443))

	// ── 2. SSH connect ────────────────────────────────────────────────────
	sshClient, sshCheck := connectSSH(ctx, node, kp)
	emit(sshCheck)
	if sshClient == nil {
		// Remote checks are impossible without SSH — stop here.
		return diagSummarize(checks), nil
	}
	defer sshClient.Close()

	// ── 3. Detect root / sudo availability ───────────────────────────────
	sudo := ""
	if uidOut, _ := sshClient.Run("id -u 2>/dev/null"); strings.TrimSpace(uidOut) != "0" {
		sudo = "sudo "
	}

	// ── 4–N. Remote checks (sequential, single SSH connection) ────────────
	for _, rc := range buildRemoteChecks(sudo) {
		if ctx.Err() != nil {
			break
		}
		out, err := sshClient.Run(rc.cmd)
		emit(rc.parse(out, err))
	}

	return diagSummarize(checks), nil
}

// ── Local checks ──────────────────────────────────────────────────────────────

func checkTCP(ctx context.Context, host string, port int) NodeDiagCheck {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	start := time.Now()
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", addr)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return NodeDiagCheck{
			ID: "tcp_connect", Category: "network", Name: "TCP 端口 443",
			Status:  "error",
			Value:   addr,
			Message: fmt.Sprintf("端口 443 不可达: %v", err),
		}
	}
	conn.Close()
	return NodeDiagCheck{
		ID: "tcp_connect", Category: "network", Name: "TCP 端口 443",
		Status:  "ok",
		Value:   fmt.Sprintf("%dms", latency),
		Message: fmt.Sprintf("端口 443 可达，延迟 %dms", latency),
	}
}

func connectSSH(ctx context.Context, node *Node, kp *KeyPair) (*SSHClient, NodeDiagCheck) {
	_ = ctx // ssh.Dial does not yet accept a context; timeout is set on the dialer
	label := fmt.Sprintf("%s@%s:%d", node.Username, node.Host, node.Port)
	client, err := NewSSHClient(node.Host, node.Port, node.Username,
		BuildAuthMethods(node.Password, kp), 15*time.Second)
	if err != nil {
		return nil, NodeDiagCheck{
			ID: "ssh_connect", Category: "network", Name: "SSH 连接",
			Status:  "error",
			Value:   label,
			Message: fmt.Sprintf("SSH 连接失败: %v", err),
		}
	}
	return client, NodeDiagCheck{
		ID: "ssh_connect", Category: "network", Name: "SSH 连接",
		Status:  "ok",
		Value:   label,
		Message: "SSH 连接成功",
	}
}

// ── Remote check definitions ──────────────────────────────────────────────────

type remoteCheck struct {
	cmd   string
	parse func(out string, cmdErr error) NodeDiagCheck
}

func buildRemoteChecks(sudo string) []remoteCheck {
	return []remoteCheck{
		// ── gost 进程 ────────────────────────────────────────────────────────
		{
			cmd: `pgrep -la gost 2>/dev/null || echo "__not_found__"`,
			parse: func(out string, _ error) NodeDiagCheck {
				out = strings.TrimSpace(out)
				if out == "" || strings.Contains(out, "__not_found__") {
					return NodeDiagCheck{
						ID: "gost_process", Category: "process", Name: "gost 进程",
						Status:  "error",
						Message: "gost 进程未运行",
					}
				}
				return NodeDiagCheck{
					ID: "gost_process", Category: "process", Name: "gost 进程",
					Status:  "ok",
					Value:   firstLine(out),
					Message: "gost 进程正在运行",
				}
			},
		},

		// ── systemd 服务状态 ──────────────────────────────────────────────
		{
			cmd: `systemctl is-active gost 2>/dev/null || echo "__unknown__"`,
			parse: func(out string, _ error) NodeDiagCheck {
				out = strings.TrimSpace(out)
				switch out {
				case "active":
					return NodeDiagCheck{
						ID: "gost_service", Category: "process", Name: "systemd 服务状态",
						Status: "ok", Value: "active", Message: "gost 服务运行中",
					}
				case "inactive":
					return NodeDiagCheck{
						ID: "gost_service", Category: "process", Name: "systemd 服务状态",
						Status: "error", Value: "inactive", Message: "gost 服务已停止（inactive）",
					}
				case "failed":
					return NodeDiagCheck{
						ID: "gost_service", Category: "process", Name: "systemd 服务状态",
						Status: "error", Value: "failed", Message: "gost 服务处于 failed 状态",
					}
				default:
					return NodeDiagCheck{
						ID: "gost_service", Category: "process", Name: "systemd 服务状态",
						Status: "skip", Value: out, Message: "非 systemd 管理或服务名不同，已跳过",
					}
				}
			},
		},

		// ── 服务重启次数 ───────────────────────────────────────────────────
		{
			cmd: `systemctl show gost --property=NRestarts --value 2>/dev/null`,
			parse: func(out string, _ error) NodeDiagCheck {
				out = strings.TrimSpace(out)
				if out == "" {
					return NodeDiagCheck{
						ID: "gost_restarts", Category: "process", Name: "服务重启次数",
						Status: "skip", Message: "无法获取重启计数（非 systemd）",
					}
				}
				n, err := strconv.Atoi(out)
				if err != nil {
					return NodeDiagCheck{
						ID: "gost_restarts", Category: "process", Name: "服务重启次数",
						Status: "skip", Value: out, Message: "重启次数解析失败",
					}
				}
				if n == 0 {
					return NodeDiagCheck{
						ID: "gost_restarts", Category: "process", Name: "服务重启次数",
						Status: "ok", Value: "0 次",
						Message: "自服务启动以来未发生意外重启",
					}
				}
				status := "warn"
				if n >= 5 {
					status = "error"
				}
				return NodeDiagCheck{
					ID: "gost_restarts", Category: "process", Name: "服务重启次数",
					Status: status, Value: fmt.Sprintf("%d 次", n),
					Message: fmt.Sprintf("gost 已意外重启 %d 次（可能是崩溃或被 OOM Kill）", n),
				}
			},
		},

		// ── 443 端口本地监听 ──────────────────────────────────────────────
		{
			cmd: `ss -tlnp 2>/dev/null | grep ':443 ' | head -3`,
			parse: func(out string, _ error) NodeDiagCheck {
				out = strings.TrimSpace(out)
				if out == "" {
					return NodeDiagCheck{
						ID: "port_listen", Category: "network", Name: "443 端口监听",
						Status:  "error",
						Message: "服务器本地未监听 443 端口，gost 可能未启动或端口已变更",
					}
				}
				return NodeDiagCheck{
					ID: "port_listen", Category: "network", Name: "443 端口监听",
					Status:  "ok",
					Value:   firstLine(out),
					Message: "端口 443 正在监听",
				}
			},
		},

		// ── TLS 证书有效期 ────────────────────────────────────────────────
		{
			cmd: `echo | timeout 5 openssl s_client -connect 127.0.0.1:443 -servername localhost 2>/dev/null | openssl x509 -noout -enddate 2>/dev/null`,
			parse: func(out string, _ error) NodeDiagCheck {
				out = strings.TrimSpace(out)
				if out == "" {
					return NodeDiagCheck{
						ID: "cert_expiry", Category: "cert", Name: "TLS 证书有效期",
						Status:  "skip",
						Message: "无法读取证书（可能为 IP 直连模式，无 TLS）",
					}
				}
				remaining, err := parseCertExpiry(out)
				if err != nil {
					return NodeDiagCheck{
						ID: "cert_expiry", Category: "cert", Name: "TLS 证书有效期",
						Status: "warn", Value: out, Message: "证书日期解析失败",
					}
				}
				days := int(remaining.Hours() / 24)
				switch {
				case days <= 0:
					return NodeDiagCheck{
						ID: "cert_expiry", Category: "cert", Name: "TLS 证书有效期",
						Status: "error", Value: "已过期",
						Message: "TLS 证书已过期，客户端将无法建立 TLS 连接",
					}
				case days <= 14:
					return NodeDiagCheck{
						ID: "cert_expiry", Category: "cert", Name: "TLS 证书有效期",
						Status: "warn", Value: fmt.Sprintf("剩余 %d 天", days),
						Message: fmt.Sprintf("证书即将到期（剩余 %d 天），请尽快续签", days),
					}
				default:
					return NodeDiagCheck{
						ID: "cert_expiry", Category: "cert", Name: "TLS 证书有效期",
						Status: "ok", Value: fmt.Sprintf("剩余 %d 天", days),
						Message: fmt.Sprintf("证书有效，剩余 %d 天", days),
					}
				}
			},
		},

		// ── OOM 日志（近 2 小时）─────────────────────────────────────────
		{
			cmd: sudo + `journalctl -k --since "2 hours ago" 2>/dev/null | grep -iE "oom|killed process|out of memory" | tail -5`,
			parse: func(out string, _ error) NodeDiagCheck {
				out = strings.TrimSpace(out)
				if out == "" {
					return NodeDiagCheck{
						ID: "oom_log", Category: "system", Name: "OOM 日志",
						Status: "ok", Message: "过去 2 小时无 OOM Kill 记录",
					}
				}
				lines := strings.Count(out, "\n") + 1
				return NodeDiagCheck{
					ID: "oom_log", Category: "system", Name: "OOM 日志",
					Status: "warn", Value: fmt.Sprintf("%d 条", lines),
					Detail:  out,
					Message: "发现 OOM Kill 记录，可能导致 gost 进程被终止",
				}
			},
		},

		// ── gost 错误日志（近 1 小时）────────────────────────────────────
		{
			cmd: sudo + `journalctl -u gost --since "1 hour ago" -p err --no-pager 2>/dev/null | grep -v "^--" | tail -20`,
			parse: func(out string, _ error) NodeDiagCheck {
				out = strings.TrimSpace(out)
				if out == "" {
					return NodeDiagCheck{
						ID: "gost_errors", Category: "process", Name: "gost 错误日志",
						Status: "ok", Message: "过去 1 小时无错误日志",
					}
				}
				lines := strings.Count(out, "\n") + 1
				return NodeDiagCheck{
					ID: "gost_errors", Category: "process", Name: "gost 错误日志",
					Status: "warn", Value: fmt.Sprintf("%d 条", lines),
					Detail:  out,
					Message: fmt.Sprintf("过去 1 小时有 %d 条错误日志", lines),
				}
			},
		},

		// ── 磁盘空间 ──────────────────────────────────────────────────────
		{
			cmd: `df -h / 2>/dev/null | tail -1`,
			parse: func(out string, _ error) NodeDiagCheck {
				out = strings.TrimSpace(out)
				if out == "" {
					return NodeDiagCheck{
						ID: "disk_space", Category: "system", Name: "磁盘空间",
						Status: "skip", Message: "无法获取磁盘信息",
					}
				}
				pct := parseDiskPercent(out)
				switch {
				case pct >= 90:
					return NodeDiagCheck{
						ID: "disk_space", Category: "system", Name: "磁盘空间",
						Status: "error", Value: fmt.Sprintf("%d%%", pct),
						Detail:  out,
						Message: fmt.Sprintf("磁盘使用率 %d%%，空间严重不足", pct),
					}
				case pct >= 80:
					return NodeDiagCheck{
						ID: "disk_space", Category: "system", Name: "磁盘空间",
						Status: "warn", Value: fmt.Sprintf("%d%%", pct),
						Detail:  out,
						Message: fmt.Sprintf("磁盘使用率 %d%%，建议清理", pct),
					}
				default:
					return NodeDiagCheck{
						ID: "disk_space", Category: "system", Name: "磁盘空间",
						Status: "ok", Value: fmt.Sprintf("%d%%", pct),
						Message: fmt.Sprintf("磁盘空间充足（使用率 %d%%）", pct),
					}
				}
			},
		},

		// ── 内存使用 ──────────────────────────────────────────────────────
		{
			cmd: `free -m 2>/dev/null | grep '^Mem'`,
			parse: func(out string, _ error) NodeDiagCheck {
				out = strings.TrimSpace(out)
				if out == "" {
					return NodeDiagCheck{
						ID: "mem_usage", Category: "system", Name: "内存使用",
						Status: "skip", Message: "无法获取内存信息",
					}
				}
				pct, total, used := parseMemUsage(out)
				switch {
				case pct >= 90:
					return NodeDiagCheck{
						ID: "mem_usage", Category: "system", Name: "内存使用",
						Status: "error", Value: fmt.Sprintf("%d/%dMB", used, total),
						Message: fmt.Sprintf("内存使用率 %d%%，可能触发 OOM Kill", pct),
					}
				case pct >= 75:
					return NodeDiagCheck{
						ID: "mem_usage", Category: "system", Name: "内存使用",
						Status: "warn", Value: fmt.Sprintf("%d/%dMB", used, total),
						Message: fmt.Sprintf("内存使用率较高（%d%%）", pct),
					}
				default:
					return NodeDiagCheck{
						ID: "mem_usage", Category: "system", Name: "内存使用",
						Status: "ok", Value: fmt.Sprintf("%d/%dMB", used, total),
						Message: fmt.Sprintf("内存正常（使用率 %d%%）", pct),
					}
				}
			},
		},

		// ── 节点出站网络 ──────────────────────────────────────────────────
		{
			cmd: `curl -s --max-time 5 -o /dev/null -w "%{http_code}" https://1.1.1.1/ 2>/dev/null || echo "timeout"`,
			parse: func(out string, _ error) NodeDiagCheck {
				out = strings.TrimSpace(out)
				switch out {
				case "200", "301", "302", "204":
					return NodeDiagCheck{
						ID: "outbound_net", Category: "network", Name: "节点出站网络",
						Status: "ok", Value: fmt.Sprintf("HTTP %s", out),
						Message: "节点可正常访问外部网络",
					}
				case "timeout", "":
					return NodeDiagCheck{
						ID: "outbound_net", Category: "network", Name: "节点出站网络",
						Status: "error", Value: "超时",
						Message: "节点无法访问外部网络（curl 超时）",
					}
				default:
					return NodeDiagCheck{
						ID: "outbound_net", Category: "network", Name: "节点出站网络",
						Status: "warn", Value: fmt.Sprintf("HTTP %s", out),
						Message: fmt.Sprintf("出站网络异常（状态码: %s）", out),
					}
				}
			},
		},
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

func diagSummarize(checks []NodeDiagCheck) NodeDiagSummary {
	s := NodeDiagSummary{Total: len(checks)}
	for _, c := range checks {
		switch c.Status {
		case "ok":
			s.OK++
		case "warn":
			s.Warn++
		case "error":
			s.Error++
		case "skip":
			s.Skip++
		}
	}
	return s
}

// parseCertExpiry parses openssl's "notAfter=..." line and returns time until expiry.
func parseCertExpiry(notAfterLine string) (time.Duration, error) {
	// "notAfter=May 27 12:00:00 2026 GMT"
	parts := strings.SplitN(notAfterLine, "=", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("unexpected format: %q", notAfterLine)
	}
	// Try common openssl date formats
	for _, layout := range []string{
		"Jan _2 15:04:05 2006 MST",
		"Jan  2 15:04:05 2006 MST",
	} {
		t, err := time.Parse(layout, strings.TrimSpace(parts[1]))
		if err == nil {
			return time.Until(t), nil
		}
	}
	return 0, fmt.Errorf("cannot parse date: %q", parts[1])
}

// parseDiskPercent extracts the usage percentage from one line of `df -h` output.
func parseDiskPercent(dfLine string) int {
	for _, f := range strings.Fields(dfLine) {
		if strings.HasSuffix(f, "%") {
			pct, _ := strconv.Atoi(strings.TrimSuffix(f, "%"))
			return pct
		}
	}
	return 0
}

// parseMemUsage parses a `free -m` "Mem:" line and returns (pct, totalMB, usedMB).
func parseMemUsage(freeLine string) (pct, total, used int) {
	// "Mem:   1024   512   256   0   128   384"
	// fields[1]=total, fields[2]=used
	fields := strings.Fields(freeLine)
	if len(fields) >= 3 {
		total, _ = strconv.Atoi(fields[1])
		used, _ = strconv.Atoi(fields[2])
		if total > 0 {
			pct = used * 100 / total
		}
	}
	return
}
