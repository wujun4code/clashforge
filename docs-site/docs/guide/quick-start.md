# 快速开始（15 分钟先把网络跑通）

这份快速开始只追求一件事：**先让你的路由器代理能力可用，并且随时能回退**。  
示例路由器地址用 `192.168.20.1`，请替换成你的实际地址。

## 先明确今天的目标

本页的目标是：

1. ClashForge 安装成功。
2. Web UI 可打开。
3. mihomo 内核能启动。
4. 你知道出问题时如何一键退出接管。

本页不追求：

1. 一次性把所有高级规则调到最优。
2. 第一次就全网接管所有设备。

## 前置条件

| 项目 | 要求 |
| --- | --- |
| 路由器 | OpenWrt / Kwrt，允许 SSH 登录 |
| 控制端 | Windows 10/11、macOS 或 Linux |
| 工具 | `ssh`、`scp` 可用 |
| 权限 | 推荐 `root` 或等价权限 |
| 配置来源 | 至少一个可用的 YAML 或订阅链接 |

::: tip 为什么先保守
ClashForge 默认不自动接管透明代理和 DNS，这是为了避免第一次部署时把全网带离线。
:::

## 1. 准备控制脚本

```powershell
git clone https://github.com/wujun4code/clashforge.git
cd clashforge
```

Windows 可用性检查：

```powershell
ssh -V
scp -V
```

## 2. 安装到路由器（推荐）

推荐用 `upgrade` 路径：由你的电脑下载 IPK，再推送到路由器安装，稳定性更好。

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade
```

网络受限时可加镜像：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Mirror https://ghproxy.com
```

macOS / Linux：

```sh
./scripts/clashforgectl --router 192.168.20.1 upgrade
```

## 3. 打开 Web UI 并导入配置

访问：

```text
http://192.168.20.1:7777
```

在 Setup 页面完成最小配置：

1. 上传 YAML 或添加订阅。
2. 保存并激活配置。
3. 启动 mihomo 内核。

## 4. 先验证，再接管

先看运行状态：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 status
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 check
```

确认核心流程可用后，再在 UI 中按顺序开启：

1. 透明代理接管。
2. DNS 接管。

每开一步都执行一次 `check`，不要两步同时开。

## 5. 出问题时的立即回退

如果开启接管后体验变差或断网，先执行：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop
```

这会尝试退出接管并恢复系统网络基线，再继续排障。

## 完成标志

满足以下 4 条，就算首次上手成功：

1. `status` 显示服务状态正常。
2. Web UI 能稳定访问。
3. `check` 能返回有效连通性与出口信息。
4. 你已经验证过 `stop` 可以作为回退手段。

下一步建议阅读 [启动与接管](/guide/run) 和 [检查清单](/guide/verify)。
