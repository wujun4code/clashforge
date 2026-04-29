# FAQ

## 推荐哪种安装方式？

生产或长期使用优先 Release IPK。开发验证优先 `clashforgectl.ps1 deploy`，因为它会自动构建、打包、上传并安装 IPK。

## 为什么首次启动不自动接管透明代理和 DNS？

这是安全默认。代理内核、订阅、DNS 和防火墙任何一层有问题，都可能影响全网。先验证内核和节点，再手动接管，更容易回退。

## Web UI 默认地址是什么？

```text
http://<router-ip>:7777
```

例如：

```text
http://192.168.20.1:7777
```

## Windows 远程脚本需要什么？

需要 OpenSSH Client，也就是 `ssh` 和 `scp` 命令可用。Windows 10/11 通常可以在可选功能中安装。

## `deploy` 和 `upgrade` 有什么区别？

| 命令 | 用途 |
| --- | --- |
| `deploy` | 本地构建当前源码，生成 IPK 并安装到路由器 |
| `upgrade` | 从 Release 下载 IPK，上传到路由器并安装 |

## 如何跳过 UI 或 Go 构建？

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy -Skip ui
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy -Skip go
```

## 如何确认真的生效了？

至少确认：

1. Web UI 可打开。
2. `status` 正常。
3. mihomo 内核正在运行。
4. 目标客户端出口 IP 符合预期。
5. DNS 查询路径符合预期。
6. nftables/iptables 中存在 ClashForge 规则。

## 如何安全卸载？

完全卸载：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 uninstall
```

保留配置：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 uninstall -KeepConfig
```

## 诊断报告可以直接发到 GitHub Issue 吗？

建议使用脱敏报告：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
```

未脱敏报告可能包含订阅链接、Token、内网地址或其他敏感数据。

## GitHub Pages 如何发布？

本仓库新增的工作流会在 `main` 分支推送 `docs-site/**` 或 Pages 工作流变更时自动构建并部署。仓库 Settings → Pages 的 Source 选择 GitHub Actions。
