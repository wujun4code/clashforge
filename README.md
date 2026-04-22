# ClashForge

ClashForge is a modern **OpenWrt router** management layer built around **mihomo**.

ClashForge 是一个围绕 **mihomo** 构建、面向 **OpenWrt 路由器** 的现代代理管理层。

It does not replace mihomo and does not reimplement proxy protocols. It focuses on the parts around mihomo that are painful on routers: lifecycle management, subscription handling, config generation, transparent proxy orchestration, DNS takeover, observability, and delivery.

它不会替代 mihomo，也不会重复实现代理协议；它重点解决的是路由器场景里围绕 mihomo 的管理层问题，例如：进程生命周期、订阅管理、配置生成、透明代理接管、DNS 接管、可观测性，以及 OpenWrt 的交付体验。

---

## 中文说明

### 项目定位

ClashForge 的目标不是再造一个代理内核，而是把 OpenWrt 上的 Mihomo 管理体验重新做一遍，让它更轻、更稳、更现代、更容易长期维护。

当前项目重点包括：

- mihomo 核心进程生命周期管理
- 配置生成、保存、热重载
- 订阅拉取、过滤、缓存与合并
- nftables / iptables 透明代理规则管理
- DNS 接管与 dnsmasq 协作
- Web UI、REST API、SSE 实时状态流
- OpenWrt init.d / IPK 打包与发布流程

### 当前能力

当前仓库已经不是“只有设计文档”的状态，而是进入了**可运行的早期预发布阶段**。已经具备的核心能力包括：

- ClashForge 服务启动时自动生成运行时 Mihomo 配置，并拉起 Mihomo
- 默认采用更安全的启动模型：**启动后不自动接管透明代理和 DNS**，由用户手动开启
- 支持订阅管理、YAML 覆盖、配置保存与运行时重生成
- 提供健康检查接口与概览页
- 概览页支持：
  - 出口 IP 显示
  - 访问检查
  - ClashForge / Mihomo CPU、内存、磁盘占用
  - 透明代理、NFT、防火墙、DNS 等模块的“当前由谁接管”展示
  - 冲突服务检测
  - 直接从概览页触发接管操作
- 提供 OpenWrt 安装包构建流程，并自动打包内置 `mihomo-clashforge`

### 快速开始

#### 1. 从 Releases 安装 OpenWrt 包

仓库的 tag push 会自动构建并发布：

- `clashforge_*_x86_64.ipk`
- `clashforge_*_aarch64_generic.ipk`
- `clashforge_*_aarch64_cortex-a53.ipk`

在路由器上安装后：

```sh
opkg install clashforge_*.ipk
```

#### 2. 启动服务

```sh
/etc/init.d/clashforge enable
/etc/init.d/clashforge start
```

默认 Web UI 地址：

```text
http://<router-ip>:7777
```

#### 3. 添加代理源

进入 Web UI 后，可以通过两种方式准备代理配置：

- 添加订阅链接
- 上传 / 粘贴 YAML

#### 4. 按需开启接管

为了降低首次启动风险，ClashForge 当前默认：

- 不在启动时自动接管透明代理
- 不在启动时自动接管 DNS

确认节点和核心运行正常后，再从设置页或概览页手动开启这些能力。

### 本地开发

#### 构建前端

```sh
cd ui
npm ci
npm run build
```

#### 构建服务端

```sh
go build ./cmd/clashforge
```

#### 生成发布版本

推送符合 `v*` 格式的 tag 会自动触发 GitHub Actions，完成：

- React UI 构建
- Linux 二进制交叉编译
- OpenWrt IPK 打包
- GitHub Release 发布

### 项目文档

详细设计和架构文档仍然保留在：

- [`docs/CLASH_REPLACEMENT_DESIGN.md`](./docs/CLASH_REPLACEMENT_DESIGN.md)

这份文档仍然是理解整体架构和后续演进方向的重要参考。

### 上游项目

- [vernesong/openclash](https://github.com/vernesong/openclash)
- [MetaCubeX/mihomo](https://github.com/MetaCubeX/mihomo)

ClashForge 建立在对这些优秀上游项目的学习与继承之上，但目标是给 OpenWrt 提供一套更现代、更工程化的管理体验。

---

## English

### What ClashForge Is

ClashForge is not another proxy core. It is a router-focused management and control plane for mihomo on OpenWrt.

It is designed to make the OpenWrt experience cleaner and more maintainable by handling:

- mihomo lifecycle management
- config generation, persistence, and reload
- subscription fetch / filter / merge workflows
- nftables / iptables transparent proxy orchestration
- DNS takeover and dnsmasq integration
- Web UI, REST API, and SSE-based live status updates
- OpenWrt init scripts and IPK packaging

### Current Status

The repository is no longer at a design-only stage. It is now in an **early runnable prerelease state** with a working backend, OpenWrt packaging pipeline, and a functional web UI.

Available features currently include:

- automatic runtime mihomo config generation on startup
- managed mihomo startup through ClashForge
- safer default startup behavior: **no transparent proxy or DNS takeover by default**
- subscription management and YAML override support
- health diagnostics and overview APIs
- an overview dashboard with:
  - egress IP checks
  - access probes
  - ClashForge / Mihomo CPU, memory, and disk usage
  - ownership and conflict detection for transparent proxy, firewall, and DNS modules
  - one-click takeover actions from the dashboard
- automated OpenWrt IPK release packaging with bundled `mihomo-clashforge`

### Quick Start

#### 1. Install from Releases

Each `v*` tag triggers release artifacts for:

- `x86_64`
- `aarch64_generic`
- `aarch64_cortex-a53`

Install on OpenWrt with:

```sh
opkg install clashforge_*.ipk
```

#### 2. Enable and start the service

```sh
/etc/init.d/clashforge enable
/etc/init.d/clashforge start
```

Open the UI at:

```text
http://<router-ip>:7777
```

#### 3. Add your proxy source

You can configure ClashForge by either:

- adding a subscription URL
- uploading or pasting a Clash-compatible YAML config

#### 4. Enable takeover when ready

To reduce startup risk on routers, ClashForge currently defaults to:

- not enabling transparent proxy takeover at startup
- not enabling DNS takeover at startup

Once nodes and core status are healthy, you can enable takeover manually from the settings page or the overview dashboard.

### Local Development

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

Publishing a tag matching `v*` triggers the GitHub Actions release pipeline, which builds the UI, cross-compiles binaries, packages OpenWrt IPKs, and creates a GitHub Release.

### Documentation

The architecture and implementation design document is still available here:

- [`docs/CLASH_REPLACEMENT_DESIGN.md`](./docs/CLASH_REPLACEMENT_DESIGN.md)

It remains the best reference for the overall structure and intended evolution of the project.

### Upstream Projects

- [vernesong/openclash](https://github.com/vernesong/openclash)
- [MetaCubeX/mihomo](https://github.com/MetaCubeX/mihomo)

ClashForge builds on lessons from these upstream projects while aiming for a cleaner, more maintainable OpenWrt control plane.

---

## License

This project is licensed under the MIT License. See [LICENSE](./LICENSE).
