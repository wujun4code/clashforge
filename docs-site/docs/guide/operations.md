# 日常运维（让它一直稳定，而不是偶尔能用）

这页面向已经跑起来的用户。目标是两件事：

1. 日常有节奏地做小检查，提前发现问题。
2. 变更时可控、可回退。

## 高频命令速查

| 目标 | Windows | 路由器本机 |
| --- | --- | --- |
| 看状态 | `.\scripts\clashforgectl.ps1 -Router 192.168.20.1 status` | `clashforgectl status` |
| 看连通性 | `.\scripts\clashforgectl.ps1 -Router 192.168.20.1 check` | `clashforgectl check` |
| 快速回退 | `.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop` | `clashforgectl stop` |
| 升级 | `.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade` | `clashforgectl upgrade` |
| 收集诊断 | `.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact` | `clashforgectl diag --redact` |
| 重置 | `.\scripts\clashforgectl.ps1 -Router 192.168.20.1 reset` | `clashforgectl reset` |

## 推荐运维节奏

| 频率 | 建议动作 |
| --- | --- |
| 每天 | 看一次 `status` / `check`，确认出口与连通性 |
| 每周 | 检查日志里是否有重复报错、频繁重启 |
| 每次改配置后 | 跑完整 [检查清单](/guide/verify) |
| 每次升级后 | 观察 10-30 分钟稳定性再结束 |

## 变更流程（非常实用）

每次改订阅、规则、DNS 或接管策略，按这个顺序：

1. 记录当前可用状态（版本、订阅、关键设置）。
2. 一次只改一个变量。
3. 改完立刻执行 `status` + `check`。
4. 观察日志 1 到 3 分钟。
5. 异常时先 `stop` 回退，再做下一步。

## 连接参数模板

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 -User root -Port 22 -Identity ~\.ssh\id_ed25519 status
```

常用参数：

| 参数 | 作用 |
| --- | --- |
| `-Router` | 路由器地址（必填） |
| `-User` | SSH 用户（默认 `root`） |
| `-Port` | SSH 端口（默认 `22`） |
| `-Identity` | 私钥路径 |
| `-Yes` | 跳过确认 |
| `-DryRun` | 只看计划不执行 |

## 诊断报告规范

建议默认使用脱敏拉取：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
```

自定义文件路径：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -RemoteOutput /tmp/cf-diag.txt -LocalPath .\cf-diag.txt -Redact
```

## 卸载策略

完全卸载：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 uninstall
```

卸载但保留配置：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 uninstall -KeepConfig
```
