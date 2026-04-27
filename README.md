# ClashForge

**ClashForge** is a modern management and control plane for [mihomo](https://github.com/MetaCubeX/mihomo) on OpenWrt routers.

> **ClashForge** 是一个为 OpenWrt 路由器上的 [mihomo](https://github.com/MetaCubeX/mihomo) 构建的现代管理与控制层。

---

<details open>
<summary><b>中文文档</b></summary>

## 项目定位

ClashForge 的目标不是再造代理内核，而是重新做一遍 OpenWrt 上的 Mihomo 管理体验，让它更轻、更稳、更现代、更容易长期维护。

它解决的是路由器场景里围绕 mihomo 的管理层问题：

- mihomo 进程生命周期管理
- 订阅拉取、缓存与多源合并
- 配置生成、保存与运行时热重载
- nftables / iptables 透明代理规则管理
- DNS 接管与 dnsmasq 协作
- 规则集同步与域名/IP 规则搜索
- Web UI、REST API、SSE 实时状态流
- OpenWrt init.d / IPK 打包与发布

## 当前能力

项目已进入**可运行的早期预发布阶段**，具备完整的前后端和打包流程。

### 概览仪表盘

- mihomo 内核状态实时监控（PID、运行时长、CPU/内存）
- 实时流量速率（上传/下载）与活跃连接数
- 子模块状态一览：透明代理、nftables 防火墙、DNS 入口、DNS 解析器
- **双侧连通性探测**：路由器侧（经代理转发）与浏览器侧（客户端直连）对比检测，出口 IP 显示、访问可达性检查
- 系统与进程资源占用（CPU、内存、磁盘）
- 内嵌节点切换器：支持 Selector 手动切换、一键测速（latency test）

### 配置管理

- **配置向导**（Setup）：引导式首次配置流程，支持上传或粘贴 YAML、添加订阅
- **配置文件管理**：保存多份配置文件，支持一键切换激活配置（运行中自动提示停止服务）
- **订阅管理**：添加/删除/更新订阅，支持自定义 User-Agent 与更新间隔，按需触发单源或全量更新
- **规则集管理**（需内核运行）：查看所有 rule-provider，显示规则条数与文件大小，支持单个或全量强制同步
- **规则搜索**：输入域名或 IP，实时搜索匹配的规则集与规则条目
- **运行中配置预览**：查看 ClashForge 生成并写入的实际 mihomo 配置（只读）
- YAML Overrides：deep-merge 覆盖机制，优先级最高，支持直接编辑

### 节点与连接

- 节点分组展示（Selector / Fallback / URLTest / LoadBalance），一键切换，支持延迟可视化进度条
- 切换节点后自动触发连通性探测，验证出口是否正常
- 活跃连接实时列表（每 2 秒刷新）：目标地址、协议、代理链、上传/下载量
- 一键清理全部连接

### 日志与活动

- mihomo 实时日志：SSE 推送 + 3 秒轮询，按级别过滤（ALL / INFO / WARNING / ERROR），支持自动滚动
- 连接与日志合并在「活动」页面的 Tab 中

### 系统设置

- 常规配置：mihomo 二进制路径、最大重启次数、日志级别
- 网络配置：透明代理模式（tproxy / redir / tun / none）、防火墙后端（nftables / iptables / auto）、启动时自动接管开关、LAN 与中国大陆 IP 绕过
- DNS 配置：启用/禁用、fake-ip / redir-host 模式、dnsmasq 共存模式（none / upstream / replace）、启动时自动接管开关

### 安全默认

启动时默认**不自动接管**透明代理和 DNS，降低首次部署风险，待节点与内核确认正常后再手动开启。

## 快速开始

### 1. 从 Releases 安装 OpenWrt 包

推送 `v*` tag 会自动构建并发布以下架构的 IPK 包：

- `x86_64`
- `aarch64_generic`
- `aarch64_cortex-a53`

在路由器上直接安装最新版本：

```sh
wget -qO- https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/install.sh | sh
```

国内加速（通过 ghproxy）：

```sh
wget -qO- https://ghproxy.com/https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/install.sh | sh
```

安装指定版本：

```sh
wget -qO- .../install.sh | sh -s -- --version v0.1.0
```

### 2. 启动服务

```sh
/etc/init.d/clashforge enable
/etc/init.d/clashforge start
```

Web UI 默认地址：

```
http://<router-ip>:7777
```

### 3. 添加配置源

通过配置向导（Setup 页面）完成首次配置：

- 上传或粘贴 Clash 兼容的 YAML 配置文件
- 添加订阅链接（支持 Clash / SS / Trojan / VLESS / VMess 格式）

### 4. 按需开启接管

确认节点和内核运行正常后，再从**设置页面**或**概览仪表盘**手动开启透明代理和 DNS 接管。

## 路由器管理命令（clashforgectl）

`clashforgectl` 是 ClashForge 的统一运维入口，安装包会自动部署到路由器上。

### 本机操作（在路由器 SSH 终端中执行）

```sh
# 查看当前运行状态（只读，不修改任何设置）
clashforgectl status

# 停止 ClashForge，完全退出透明代理接管模式
# 自动恢复 dnsmasq 配置、清理 nftables、策略路由
clashforgectl stop

# 重置为初始安装状态（保留已安装的包版本）
# 清除订阅、规则集、生成配置、缓存、运行时数据与日志
clashforgectl reset

# 重置并自动重启服务
clashforgectl reset --start

# 升级到最新版本
clashforgectl upgrade

# 升级到指定版本
clashforgectl upgrade --version v0.1.0

# 使用国内镜像升级
clashforgectl upgrade --mirror https://ghproxy.com

# 完全重置后升级（--purge）
clashforgectl upgrade --purge

# 收集诊断报告（输出至 /tmp/cf-diag.txt）
clashforgectl diag

# 收集诊断报告并打印到终端
clashforgectl diag --stdout

# 收集诊断报告并对敏感信息做脱敏处理
clashforgectl diag --redact

# 完全卸载 ClashForge
clashforgectl uninstall

# 完全卸载，但保留 /etc/metaclash 配置数据
clashforgectl uninstall --keep-config
```

### 远程操作（从 Windows 主机控制路由器）

```powershell
# 查看状态
.\scripts\clashforgectl.ps1 -Router 192.168.1.1 status

# 停止 ClashForge
.\scripts\clashforgectl.ps1 -Router 192.168.1.1 stop

# 重置为初始状态
.\scripts\clashforgectl.ps1 -Router 192.168.1.1 reset

# 升级到最新版本
.\scripts\clashforgectl.ps1 -Router 192.168.1.1 upgrade

# 收集诊断报告并下载到本地
.\scripts\clashforgectl.ps1 -Router 192.168.1.1 diag -Fetch

# 收集脱敏报告并下载
.\scripts\clashforgectl.ps1 -Router 192.168.1.1 diag -Fetch -Redact

# 卸载（保留配置）
.\scripts\clashforgectl.ps1 -Router 192.168.1.1 uninstall -KeepConfig
```

指定 SSH 用户、端口或密钥：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.1.1 -User root -Port 22 -Identity ~\.ssh\id_ed25519 status
```

### 远程操作（从 macOS / Linux 主机控制路由器）

```sh
# 查看状态
./scripts/clashforgectl --router 192.168.1.1 status

# 停止 ClashForge
./scripts/clashforgectl --router 192.168.1.1 stop

# 升级
./scripts/clashforgectl --router 192.168.1.1 upgrade --version latest

# 收集脱敏诊断报告并下载
./scripts/clashforgectl --router 192.168.1.1 diag --fetch --redact

# 卸载
./scripts/clashforgectl --router 192.168.1.1 uninstall
```

## 故障排查

### DNS 断连 / 无法上网

ClashForge 退出接管后 DNS 未恢复：

```sh
# 强制恢复 dnsmasq 配置并重启
clashforgectl stop

# 验证 dnsmasq 是否监听 53 端口
netstat -lnup | grep :53

# 如仍有问题，手动恢复 UCI 设置
uci delete dhcp.@dnsmasq[0].port
uci delete dhcp.@dnsmasq[0].server
uci delete dhcp.@dnsmasq[0].noresolv
uci commit dhcp
/etc/init.d/dnsmasq restart
```

### nftables 规则残留

```sh
# 查看是否有残留表
nft list tables

# 手动清除（clashforgectl stop 会自动处理）
nft delete table inet metaclash
nft delete table inet dnsmasq
```

### 策略路由规则残留

```sh
# 查看
ip rule list | grep 0x1a3

# 手动清除
while ip rule del fwmark 0x1a3 table 100 2>/dev/null; do :; done
ip route flush table 100
while ip -6 rule del fwmark 0x1a3 table 100 2>/dev/null; do :; done
ip -6 route flush table 100
```

### opkg 安装 / 升级失败

```sh
# 更新包列表后重试
opkg update
opkg install --nodeps --force-downgrade clashforge_*.ipk

# 检查磁盘空间
df -h /tmp /overlay
```

### 收集完整诊断报告

```sh
# 在路由器上收集（含所有探测信息，输出至 /tmp/cf-diag.txt）
clashforgectl diag --redact

# 从 Windows 远程拉取报告到本地
.\scripts\clashforgectl.ps1 -Router 192.168.1.1 diag -Fetch -Redact
```

## 本地开发

构建前端：

```sh
cd ui
npm ci
npm run build
```

构建服务端：

```sh
go build ./cmd/clashforge
```

推送 `v*` 格式的 tag 会自动触发 GitHub Actions，完成 UI 构建、二进制交叉编译、IPK 打包和 GitHub Release 发布。

## 项目文档

架构与设计文档：[`docs/CLASH_REPLACEMENT_DESIGN.md`](./docs/CLASH_REPLACEMENT_DESIGN.md)

**Fork 后自托管分发：** 如果你 fork 了本仓库并希望将 IPK 文件同步到自己的 Cloudflare R2，请参考 [`docs/guides/fork-r2-setup.md`](./docs/guides/fork-r2-setup.md)。

## 上游项目

- [vernesong/openclash](https://github.com/vernesong/openclash)
- [MetaCubeX/mihomo](https://github.com/MetaCubeX/mihomo)

ClashForge 建立在对这些优秀上游项目的学习与继承之上，目标是为 OpenWrt 提供一套更现代、更工程化的管理体验。

</details>

---

<details>
<summary><b>English Documentation</b></summary>

## What ClashForge Is

ClashForge is not another proxy core. It is a router-focused management and control plane for mihomo on OpenWrt — designed to make the OpenWrt experience cleaner and more maintainable.

It handles everything *around* mihomo:

- mihomo lifecycle management
- subscription fetch, cache, and multi-source merge
- config generation, persistence, and hot reload
- nftables / iptables transparent proxy orchestration
- DNS takeover and dnsmasq integration
- rule-set sync and domain/IP rule search
- Web UI, REST API, and SSE-based live status updates
- OpenWrt init scripts and IPK packaging

## Current Status

The repository is in an **early runnable prerelease state** with a working backend, a full web UI, and an automated OpenWrt packaging pipeline.

### Overview Dashboard

- Real-time mihomo core status (PID, uptime, CPU/memory)
- Live traffic rates (upload/download) and active connection count
- Module status overview: transparent proxy, nftables firewall, DNS entry, DNS resolver
- **Dual-side connectivity probing**: router-side (via proxy) vs. browser-side (direct) — compare egress IPs and access reachability side by side
- System and process resource usage (CPU, memory, disk)
- Embedded proxy switcher: manual Selector switching with one-click latency testing

### Config Management

- **Setup wizard**: guided first-run flow — upload or paste a YAML config, or add a subscription URL
- **Config file management**: store multiple configs, switch active config with one click (prompts to stop the running service if needed)
- **Subscription management**: add/delete/update subscriptions, custom User-Agent and update interval, per-source or batch update
- **Rule-set management** (requires core running): list all rule-providers with rule counts and file sizes, force-sync individual or all providers
- **Rule search**: type a domain or IP to instantly search for matching rule-sets and entries
- **Running config viewer**: read-only view of the actual mihomo config generated and written by ClashForge
- YAML Overrides: deep-merge override mechanism with highest priority, editable directly in the UI

### Proxies and Connections

- Proxy groups (Selector / Fallback / URLTest / LoadBalance) with inline node switcher and latency bar visualization
- Auto-trigger connectivity probe after switching nodes to verify egress health
- Live connection list with auto-refresh (every 2 s): destination, protocol, proxy chain, upload/download
- One-click close-all connections

### Logs and Activity

- Real-time mihomo logs: SSE push + 3-second polling, filterable by level (ALL / INFO / WARNING / ERROR), with auto-scroll
- Connections and logs combined under the Activity page with tab switching

### System Settings

- General: mihomo binary path, max restart count, log level
- Network: transparent proxy mode (tproxy / redir / tun / none), firewall backend (nftables / iptables / auto), apply-on-start toggle, LAN and China-IP bypass
- DNS: enable/disable, fake-ip / redir-host mode, dnsmasq coexistence (none / upstream / replace), apply-on-start toggle

### Safe Defaults

Transparent proxy and DNS takeover are **disabled by default at startup** to reduce risk on first deployment. Enable them manually from the settings page or dashboard once nodes and the core are confirmed healthy.

## Quick Start

### 1. Install from Releases

Each `v*` tag triggers release builds for:

- `x86_64`
- `aarch64_generic`
- `aarch64_cortex-a53`

Install the latest release on OpenWrt:

```sh
wget -qO- https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/install.sh | sh
```

China mirror (via ghproxy):

```sh
wget -qO- https://ghproxy.com/https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/install.sh | sh
```

Install a specific version:

```sh
wget -qO- .../install.sh | sh -s -- --version v0.1.0
```

### 2. Enable and start the service

```sh
/etc/init.d/clashforge enable
/etc/init.d/clashforge start
```

Open the UI at:

```
http://<router-ip>:7777
```

### 3. Add your proxy source

Use the Setup wizard to complete first-run configuration:

- Upload or paste a Clash-compatible YAML config
- Add a subscription URL (Clash / SS / Trojan / VLESS / VMess)

### 4. Enable takeover when ready

Once nodes and core status are confirmed healthy, enable transparent proxy and DNS takeover manually from the **Settings page** or the **Overview dashboard**.

## Router Management (`clashforgectl`)

`clashforgectl` is the unified operations entry point for ClashForge. The IPK package deploys it automatically.

### On the router (over SSH)

```sh
# Read-only status check
clashforgectl status

# Stop ClashForge and exit takeover mode
# Restores dnsmasq, removes nftables tables and policy routing rules
clashforgectl stop

# Reset to first-install state (keeps installed package version)
# Clears subscriptions, rule-sets, generated configs, caches, runtime data, logs
clashforgectl reset

# Reset and auto-start the service
clashforgectl reset --start

# Upgrade to the latest version
clashforgectl upgrade

# Upgrade to a specific version
clashforgectl upgrade --version v0.1.0

# Upgrade via China mirror
clashforgectl upgrade --mirror https://ghproxy.com

# Full reset before upgrading
clashforgectl upgrade --purge

# Collect a diagnostic report (saved to /tmp/cf-diag.txt)
clashforgectl diag

# Print diagnostic report to terminal
clashforgectl diag --stdout

# Collect report with sensitive values redacted
clashforgectl diag --redact

# Fully uninstall ClashForge
clashforgectl uninstall

# Uninstall but keep /etc/metaclash config data
clashforgectl uninstall --keep-config
```

### Remote control from Windows

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.1.1 status
.\scripts\clashforgectl.ps1 -Router 192.168.1.1 stop
.\scripts\clashforgectl.ps1 -Router 192.168.1.1 reset
.\scripts\clashforgectl.ps1 -Router 192.168.1.1 upgrade
.\scripts\clashforgectl.ps1 -Router 192.168.1.1 diag -Fetch
.\scripts\clashforgectl.ps1 -Router 192.168.1.1 diag -Fetch -Redact
.\scripts\clashforgectl.ps1 -Router 192.168.1.1 uninstall -KeepConfig
```

Specify SSH user, port, or key:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.1.1 -User root -Port 22 -Identity ~\.ssh\id_ed25519 status
```

### Remote control from macOS / Linux

```sh
./scripts/clashforgectl --router 192.168.1.1 status
./scripts/clashforgectl --router 192.168.1.1 stop
./scripts/clashforgectl --router 192.168.1.1 upgrade --version latest
./scripts/clashforgectl --router 192.168.1.1 diag --fetch --redact
./scripts/clashforgectl --router 192.168.1.1 uninstall
```

## Troubleshooting

### DNS drops after ClashForge stops

```sh
# Force dnsmasq config restore and restart
clashforgectl stop

# Verify dnsmasq is listening on port 53
netstat -lnup | grep :53

# Manual UCI restore if needed
uci delete dhcp.@dnsmasq[0].port
uci delete dhcp.@dnsmasq[0].server
uci delete dhcp.@dnsmasq[0].noresolv
uci commit dhcp
/etc/init.d/dnsmasq restart
```

### Leftover nftables rules

```sh
# Check for leftover tables
nft list tables

# Remove manually (clashforgectl stop handles this automatically)
nft delete table inet metaclash
nft delete table inet dnsmasq
```

### Leftover policy routing rules

```sh
# Check
ip rule list | grep 0x1a3

# Remove manually
while ip rule del fwmark 0x1a3 table 100 2>/dev/null; do :; done
ip route flush table 100
while ip -6 rule del fwmark 0x1a3 table 100 2>/dev/null; do :; done
ip -6 route flush table 100
```

### opkg install / upgrade failure

```sh
# Update package list and retry
opkg update
opkg install --nodeps --force-downgrade clashforge_*.ipk

# Check disk space
df -h /tmp /overlay
```

### Collect a full diagnostic report

```sh
# On the router
clashforgectl diag --redact

# Pull report to local machine from Windows
.\scripts\clashforgectl.ps1 -Router 192.168.1.1 diag -Fetch -Redact
```

## Local Development

Build the UI:

```sh
cd ui
npm ci
npm run build
```

Build the backend:

```sh
go build ./cmd/clashforge
```

Pushing a tag matching `v*` triggers the GitHub Actions release pipeline: UI build, cross-compiled binaries, OpenWrt IPK packaging, and GitHub Release creation.

## Documentation

Architecture and design document: [`docs/CLASH_REPLACEMENT_DESIGN.md`](./docs/CLASH_REPLACEMENT_DESIGN.md)

**Fork & self-hosted distribution:** If you have forked this repo and want to sync IPK releases to your own Cloudflare R2 bucket, see [`docs/guides/fork-r2-setup.md`](./docs/guides/fork-r2-setup.md).

## Upstream Projects

- [vernesong/openclash](https://github.com/vernesong/openclash)
- [MetaCubeX/mihomo](https://github.com/MetaCubeX/mihomo)

ClashForge builds on lessons from these upstream projects while aiming for a cleaner, more maintainable OpenWrt control plane.

</details>

---

## License

MIT License — see [LICENSE](./LICENSE).
