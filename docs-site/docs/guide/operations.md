# 日常运维

本页整理安装后的常用操作。Windows 用户优先使用 `scripts\clashforgectl.ps1`，路由器 SSH 内优先使用 `clashforgectl`。

## 常用命令速查

| 目标 | Windows 远程命令 | 路由器本机命令 |
| --- | --- | --- |
| 查看状态 | `.\scripts\clashforgectl.ps1 -Router 192.168.20.1 status` | `clashforgectl status` |
| 轻量检查 | `.\scripts\clashforgectl.ps1 -Router 192.168.20.1 check` | `clashforgectl check` |
| 停止接管 | `.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop` | `clashforgectl stop` |
| 重置 | `.\scripts\clashforgectl.ps1 -Router 192.168.20.1 reset` | `clashforgectl reset` |
| 重置并启动 | `.\scripts\clashforgectl.ps1 -Router 192.168.20.1 reset -Start` | `clashforgectl reset --start` |
| 升级 | `.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade` | `clashforgectl upgrade` |
| 诊断 | `.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact` | `clashforgectl diag --redact` |
| 卸载 | `.\scripts\clashforgectl.ps1 -Router 192.168.20.1 uninstall` | `clashforgectl uninstall` |

## 指定连接参数

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 -User root -Port 22 -Identity ~\.ssh\id_ed25519 status
```

参数说明：

| 参数 | 说明 |
| --- | --- |
| `-Router` | 路由器 IP 或主机名，必填 |
| `-User` | SSH 用户，默认 `root` |
| `-Port` | SSH 端口，默认 `22` |
| `-Identity` | SSH 私钥路径 |
| `-Yes` | 跳过确认提示 |
| `-DryRun` | 只打印计划，不执行变更 |

## 诊断报告

常用方式：

```powershell
# 只在路由器生成报告
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag

# 生成并下载到当前目录
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch

# 下载前脱敏
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact

# 指定远端和本地路径
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -RemoteOutput /tmp/cf-diag.txt -LocalPath .\cf-diag.txt -Redact
```

## 配置变更流程

建议每次配置变更按这个流程：

1. 保存当前可用配置或导出订阅信息。
2. 修改 YAML、订阅或 Overrides。
3. 启动内核或重新加载配置。
4. 执行 `status` 和 `check`。
5. 观察日志 1 到 3 分钟。
6. 再开启或恢复透明代理/DNS 接管。

## 日志入口

路由器系统日志：

```sh
logread | grep -i clashforge
logread | grep -i mihomo
```

Web UI 也提供实时日志和活动视图，适合观察节点切换、规则更新和连接变化。

## 卸载

完全卸载：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 uninstall
```

卸载但保留配置：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 uninstall -KeepConfig
```
