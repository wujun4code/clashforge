# 快速开始

本页用于从零完成一次可验证的 ClashForge 部署。示例路由器地址使用 `192.168.20.1`，请替换为你的 OpenWrt 管理地址。

## 前置条件

| 项目 | 要求 |
| --- | --- |
| 路由器 | OpenWrt / Kwrt，允许 SSH 登录 |
| 本机 | Windows 10/11、macOS 或 Linux |
| 工具 | Git、ssh、scp |
| 开发部署 | Go、Node.js、npm、Python |
| 推荐权限 | 路由器 `root` 用户或具备等价权限的用户 |

::: tip 安全默认
ClashForge 启动时默认不自动接管透明代理和 DNS。先完成配置和节点验证，再手动开启接管，能降低首次部署风险。
:::

## 1. 获取项目

```powershell
 git clone https://github.com/wujun4code/clashforge.git
 cd clashforge
```

## 2. 从 Windows 一键部署

此命令会在本机完成 UI 构建、Go 交叉编译、IPK 打包、上传到路由器并安装。

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy
```

常用变体：

```powershell
# 指定 SSH 用户、端口和私钥
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 -User root -Port 22 -Identity ~\.ssh\id_ed25519 deploy

# 跳过 UI 构建，只重新打包后端和 IPK
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy -Skip ui

# 跳过 Go 编译，只重新打包已有二进制和 UI
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy -Skip go

# 清理旧配置后安装，谨慎使用
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy -Purge
```

## 3. 打开 Web UI

部署完成后访问：

```text
http://192.168.20.1:7777
```

## 4. 完成首次配置

进入 Setup/配置向导，至少完成一个配置来源：

1. 上传或粘贴 Clash 兼容 YAML。
2. 添加订阅链接。
3. 保存并激活配置。
4. 启动 mihomo 内核。
5. 确认节点可用后，再开启透明代理或 DNS 接管。

## 5. 快速检查

```powershell
# 查看路由器侧 ClashForge 状态
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 status

# 轻量检查连通性和出口 IP
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 check

# 收集脱敏诊断报告到本机
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
```

如果上述步骤都正常，继续阅读 [启动与接管](/guide/run) 和 [检查清单](/guide/verify)。
