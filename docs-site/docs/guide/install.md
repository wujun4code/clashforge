# 安装方式选择

这页只解决一个问题：**你该用哪条安装路径最稳**。  
在 ClashForge 里，安装、升级、回滚本质都围绕 IPK 包完成。

## 先选路径，再执行命令

| 你的场景 | 推荐路径 | 为什么 |
| --- | --- | --- |
| 普通用户首次安装 | `upgrade`（从电脑推送到路由器） | 电脑端下载 IPK 更稳定，路由器不容易“边升级边掉网” |
| 想安装指定版本 | `upgrade -Version` | 版本明确、可控、便于回滚 |
| 开发者验证本地代码 | `deploy` | 本地构建 UI/Go/IPK 后直接推送安装 |
| 路由器 SSH 本机运维 | `clashforgectl` | 不依赖控制端，适合维护现场 |

## 路径 A：普通用户推荐（最稳）

Windows：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade
```

macOS / Linux：

```sh
./scripts/clashforgectl --router 192.168.20.1 upgrade
```

安装指定版本：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Version v0.1.0
```

网络受限时：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Mirror https://ghproxy.com
```

## 路径 B：开发者部署（本地源码）

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy
```

`deploy` 会做这些事：

1. 自动递增 `ipk/CONTROL/control` 里的 patch 版本。
2. 构建前端 UI。
3. 交叉编译 Go 二进制。
4. 生成 IPK 并上传安装。

::: warning `deploy` 会改版本文件
如果你工作区有未提交改动，先确认你确实希望把版本号变化纳入本次提交。
:::

可选参数：

```powershell
# 跳过 UI 构建
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy -Skip ui

# 跳过 Go 构建
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy -Skip go

# 清理旧数据后部署（高风险）
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy -Purge
```

## 路径 C：路由器本机维护

安装完成后，在路由器 SSH 里可直接执行：

```sh
clashforgectl status
clashforgectl upgrade
clashforgectl upgrade --version v0.1.0
clashforgectl upgrade --mirror https://ghproxy.com
```

## 手动安装（兜底）

当你需要完全手动控制时：

1. 在 Releases 下载匹配架构的 IPK。
2. 上传到路由器 `/tmp`。
3. 执行 `opkg install --nodeps --force-downgrade /tmp/<ipk-file>.ipk`。

常见架构判断：

```sh
uname -m
```

## 安装后第一件事

先确认 UI 能打开，再继续配置：

```text
http://192.168.20.1:7777
```

如果打不开，先看 [排障](/guide/troubleshooting) 的“Web UI 无法访问”章节。
