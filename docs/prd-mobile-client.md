# ClashForge Mobile — PRD v0.1

> 状态：草稿 | 作者：Wu Jun | 日期：2026-05-16

---

## 一、产品背景

ClashForge 现有版本运行在 OpenWrt 路由器上，通过透明代理为局域网内所有设备服务。移动端用户目前没有独立客户端，只能依赖路由器中转。

本文档定义 **ClashForge Mobile** 的产品需求：一款运行在 Android / iOS 上的独立 VPN 客户端，内嵌 mihomo 代理核心，完全不依赖路由器。

---

## 二、核心产品原则

> **"订阅导入 → 一键启动"是唯一必经路径。**

1. **默认配置即最优配置** — 用户不需要了解 Fake-IP、GeoData、Rule Provider 等概念，App 开箱即用
2. **渐进式披露** — 高级设置存在但不在主流程中出现，90% 用户不需要进入设置页
3. **零网络依赖启动** — GeoData 全部预置于安装包内，首次连接不需要下载任何文件
4. **尊重订阅结构** — 直接展示机场提供的 proxy-group 分组，不做破坏性重组

---

## 三、目标用户

| 用户类型 | 描述 |
|---|---|
| **主力用户** | 有 Clash 订阅，希望在手机上直接使用，不想折腾配置 |
| **ClashForge 路由器用户** | 路由器已有 ClashForge，手机外出时需要独立客户端 |
| **不在范围** | 需要路由器+手机联动管控的企业/家庭网络管理员（下一阶段 Companion 功能） |

---

## 四、技术栈决策

| 维度 | 决策 | 理由 |
|---|---|---|
| 跨平台框架 | **Flutter** | 共享 UI 和业务逻辑层，原生层仅处理 VPN 隧道 |
| 代理核心 | **mihomo 嵌入二进制** | 复用 ClashForge 现有核心，兼容所有 Clash 格式订阅 |
| Android VPN | **VpnService + tun 接口** | 标准 Android VPN 方案，无需 root |
| iOS VPN | **NEPacketTunnelProvider** | Network Extension，需要 Apple 特殊授权 |
| 平台优先级 | **Android 先行**，iOS 复用 Flutter 层跟进 | Android 无审核，APK 旁载发布更灵活 |
| 路由器同步 | **不支持**（极简模式）| MVP 阶段完全独立，订阅手动导入 |

---

## 五、开箱即用默认配置

App 内置以下基准配置，用户启动后自动生效，无需任何手动设置：

```yaml
# 代理模式
mode: rule                        # 规则分流（非全局，非直连）

# 节点选择策略（顶层代理组）
proxy-groups:
  - name: 🚀 Proxy
    type: url-test                # 自动选延迟最低节点
    url: http://www.gstatic.com/generate_204
    interval: 300
    proxies: <订阅解析出的节点或机场分组>

# DNS
dns:
  enable: true
  enhanced-mode: fake-ip          # 防 DNS 泄漏，性能最佳
  nameserver:
    - 223.5.5.5                   # 阿里 DNS（CN 直连）
    - 8.8.8.8                     # Google DNS（走代理）
  fake-ip-filter:                 # 以下域名不走 fake-ip（防止部分 App 异常）
    - "*.lan"
    - "localhost.ptlogin2.qq.com"

# 分流规则
rules:
  - GEOIP,private,DIRECT,no-resolve
  - GEOSITE,cn,DIRECT
  - GEOIP,CN,DIRECT
  - MATCH,🚀 Proxy

# GeoData
geodata-mode: false               # mmdb 格式（country.mmdb），内存占用最低
geodata-path: <App 内置路径>

# 其他
ipv6: false
tcp-concurrent: true
unified-delay: true
```

**代理组生成策略**：
- 若订阅包含 proxy-group 定义（绝大多数机场订阅）→ 保留原有分组结构，顶层 `🚀 Proxy` 指向订阅的第一个 select/url-test 组
- 若订阅只有裸节点（无分组）→ 生成单个 url-test 组包含所有节点

---

## 六、用户旅程

### 6.1 首次使用（新用户）

```
打开 App
    │
    ▼
欢迎页：「还没有订阅」
    │
    ▼ 点击「+ 导入订阅」
    │
    ├── 扫二维码
    ├── 粘贴 URL（检测剪贴板）
    └── 手动输入 URL
    │
    ▼ 拉取成功
显示：「订阅名称 · 共 XX 个节点」
    │
    ▼ 点击「连接」
    │
系统弹出 VPN 权限请求（Android 首次）
    │
    ▼ 用户允许
VPN 已连接 ✅（预计耗时 ≤ 3 秒）
```

**全程最少 3 步：导入 → 允许权限 → 连接完成。**

### 6.2 日常使用（已有订阅）

```
打开 App → 点击「连接」→ 已连接（< 2 秒）
```

### 6.3 切换节点

```
首页点击当前节点行
    │
    ▼
弹出代理组/节点列表（订阅原有分组结构）
    │
    ▼ 点击目标节点
切换完成，无需重启 VPN
```

---

## 七、功能模块详述

### M1 — 代理核心引擎（Core Engine）

**目标**：在 Android/iOS 上运行 mihomo，建立 VPN 隧道，用户无感知。

| 需求 | 优先级 | 说明 |
|---|---|---|
| mihomo 二进制嵌入 | P0 | Android: `.so` via JNI；iOS: `.xcframework`；按 ABI 分包 |
| Android VpnService 隧道 | P0 | tun 接口，`protect()` 防回环，`FOREGROUND_SERVICE` 保活 |
| iOS NEPacketTunnelProvider | P1 | Network Extension，App Group 共享容器 |
| 配置生成 | P0 | Dart 移植 ClashForge `generator.go` 核心逻辑，输出 mihomo config.yaml |
| 核心生命周期管理 | P0 | 启动 / 停止 / 崩溃自动重启（最多 3 次），复用 CoreManager 逻辑 |
| mihomo API 内部代理 | P0 | App 内 HTTP 转发到 mihomo `127.0.0.1:9090`，用于节点切换和流量监控 |
| 断线自动重连 | P0 | 网络切换（WiFi ↔ 移动数据）后自动恢复 VPN |
| IPv6 支持 | P2 | 可配置开关，默认关闭 |

**约束**：
- iOS Network Extension 内存上限约 15 MB，需精简 mihomo 编译选项（关闭不必要的 provider）
- Android Doze 模式：前台服务 + 持续通知必须存在

---

### M2 — 订阅管理（Subscriptions）

**目标**：导入 Clash 格式订阅，自动维护节点列表。

| 需求 | 优先级 | 说明 |
|---|---|---|
| URL 订阅导入 | P0 | Clash YAML 格式 |
| 单节点链接导入 | P0 | ss:// / trojan:// / vless:// / vmess:// |
| 二维码扫描导入 | P0 | 调用系统相机，识别 URI |
| 剪贴板自动检测 | P1 | App 进入前台时检测剪贴板，含订阅 URL 时提示导入 |
| 手动刷新 | P0 | 单条刷新 + 全量刷新 |
| 自动定时更新 | P1 | 默认 6h；Android: WorkManager；iOS: BGAppRefreshTask |
| 订阅状态展示 | P0 | 节点数量、上次更新时间、流量信息（解析订阅 header `Subscription-Userinfo`）|
| 禁用 / 删除订阅 | P0 | 禁用后节点不参与代理组 |
| 本地文件导入 | P1 | 从文件管理器选择 .yaml / .yml |
| User-Agent 配置 | P2 | 自定义请求头，兼容部分机场验证 |

**复用**：`internal/subscription/parser*.go` 逻辑移植为 Dart 类。

---

### M3 — 节点 & 代理组（Nodes & Proxy Groups）

**目标**：展示订阅的原有分组结构，支持手动切换和延迟测速。

| 需求 | 优先级 | 说明 |
|---|---|---|
| 代理组列表展示 | P0 | 保留订阅原有 proxy-group 层级结构 |
| 节点列表（组内展示）| P0 | 展示节点名称、延迟、协议类型 |
| 手动切换节点/组 | P0 | PUT `/proxies/{group}` 到 mihomo API，无需重启 VPN |
| 单节点延迟测速 | P0 | 调用 mihomo `/proxies/{name}/delay` |
| 全量测速 | P1 | 并发 ping 所有节点，结果实时更新 |
| 延迟状态色阶 | P1 | 绿色（<150ms）/ 黄色（150-300ms）/ 红色（>300ms）/ 灰色（超时）|
| 收藏节点 | P2 | 本地持久化，快捷访问 |

---

### M4 — 分流规则（Traffic Rules）

**目标**：内置最优默认规则，高级用户可自定义。

| 需求 | 优先级 | 说明 |
|---|---|---|
| 预设模式一键切换 | P0 | 全局代理 / 规则分流（默认）/ 全部直连，首页直接操作 |
| GeoIP 规则（默认启用）| P0 | country.mmdb，CN → DIRECT |
| GeoSite 规则（默认启用）| P0 | GeoSite.dat，cn / geolocation-!cn |
| 私有地址直连（默认启用）| P0 | RFC1918，不可关闭 |
| **按 App 分流**（Android）| P1 | 勾选 App 排除出 VPN；展示已安装 App 列表，搜索过滤 |
| 自定义域名规则 | P2 | DOMAIN / DOMAIN-SUFFIX / DOMAIN-KEYWORD |
| 自定义 IP 规则 | P2 | IP-CIDR 直连或代理 |
| Rule Provider 订阅 | P2 | 订阅外部 .yaml 规则集 |

---

### M5 — DNS 配置（DNS）

默认配置覆盖 95% 用例，以下为可选调整项：

| 需求 | 优先级 | 说明 |
|---|---|---|
| Fake-IP 模式（默认开启）| P0 | 防 DNS 泄漏，自动处理国内外域名分流 |
| Real-IP 模式切换 | P2 | 兼容少数需要真实 IP 的 App |
| 自定义上游 DNS | P1 | 可替换默认的 223.5.5.5 / 8.8.8.8 |
| DoH 支持 | P2 | `https://dns.google/dns-query` |
| Fallback DNS | P1 | 国外域名走 8.8.8.8（经代理），国内走 223.5.5.5（直连）|

---

### M6 — GeoData 管理

| 需求 | 优先级 | 说明 |
|---|---|---|
| 预置 GeoData（安装包内置）| P0 | country.mmdb (~4MB) + GeoSite.dat (~7MB)，复用 CI bundling 流程 |
| 手动检查更新 | P1 | 从 MetaCubeX releases 拉取最新版 |
| 自动更新 | P2 | 可配置，默认关闭，避免意外流量消耗 |
| 当前版本信息展示 | P1 | 文件大小、更新时间戳 |

---

### M7 — 流量监控 & 连接

| 需求 | 优先级 | 说明 |
|---|---|---|
| 实时速率（首页展示）| P0 | 上行 / 下行 bps |
| 今日 / 本月用量 | P1 | 累计统计，本地持久化 |
| 实时连接列表 | P1 | 复用 mihomo `/connections`，展示目标域名 / IP / 命中规则 |
| 关闭指定连接 | P1 | DELETE `/connections/{id}` |
| 关闭全部连接 | P1 | |

---

### M8 — 日志

| 需求 | 优先级 | 说明 |
|---|---|---|
| 实时日志流 | P1 | WebSocket 订阅 mihomo log stream |
| 日志级别过滤 | P1 | Info / Warning / Error |
| 日志导出 / 分享 | P2 | 系统分享菜单 |
| 清空日志 | P1 | |

---

### M9 — 系统集成 & 设置

| 需求 | 平台 | 优先级 | 说明 |
|---|---|---|---|
| 开机自启 | Android | P1 | `BOOT_COMPLETED` broadcast，自动连接上次配置 |
| Always-on VPN | Android/iOS | P1 | 引导用户在系统设置中开启 |
| 前台通知（VPN 状态）| Android | P0 | 系统要求，展示连接状态 + 速率 |
| 断线通知 | 双端 | P1 | VPN 意外断开时推送通知 |
| 快捷磁贴 | Android | P2 | `TileService` 一键开关 VPN |
| WidgetKit 小组件 | iOS | P2 | 主屏显示连接状态 / 一键切换 |
| 深色 / 浅色主题 | 双端 | P1 | 跟随系统 |
| 语言（中文 / 英文）| 双端 | P1 | 跟随系统语言 |
| 配置备份 / 导出 | 双端 | P2 | 导出加密 zip（含订阅 + 规则 + 设置）|

---

## 八、信息架构

```
底部导航（4 Tab）
│
├── 首页（Home）                        ← 主操作页
│   ├── VPN 开关（大按钮，状态即操作）
│   ├── 当前代理模式快捷切换（全局/规则/直连）
│   ├── 当前节点 + 延迟（点击展开切换）
│   └── 实时速率卡片
│
├── 代理（Proxy）                        ← 节点管理
│   ├── 代理组列表（订阅原有分组结构）
│   ├── 组内节点列表（延迟色阶）
│   └── 测速按钮
│
├── 订阅（Subscriptions）               ← 数据来源
│   ├── 订阅列表（名称、节点数、更新状态）
│   ├── 新增订阅（URL / 扫码 / 文件）
│   └── 订阅详情（刷新、禁用、删除）
│
└── 设置（Settings）                     ← 进阶配置（不强迫访问）
    ├── 按 App 分流（Android）
    ├── DNS 配置
    ├── GeoData 管理
    ├── 连接监控
    ├── 日志
    └── 通用（主题、语言、开机自启、备份）
```

---

## 九、技术架构

```
Flutter App
│
├── Dart 业务层
│   ├── subscription/        移植 ClashForge parser*.go → 解析 Clash YAML / SS / VLESS / VMess
│   ├── config/              移植 generator.go → 生成 mihomo config.yaml（含开箱即用默认值）
│   ├── geodata/             管理内置 GeoData 文件，处理更新逻辑
│   ├── api/                 HTTP client → 内部 mihomo API（节点切换、连接、流量）
│   └── store/               本地持久化（订阅列表、设置、流量统计）
│
├── Platform Channel（原生 VPN 层）
│   ├── Android（Kotlin）
│   │   ├── ClashVpnService extends VpnService
│   │   ├── tun 接口 + mihomo 子进程管理
│   │   ├── protect() 防回环
│   │   └── ForegroundService 保活通知
│   └── iOS（Swift）
│       ├── NEPacketTunnelProvider（Network Extension）
│       ├── App Group 共享容器（config.yaml / GeoData）
│       └── XPC 与主 App 通信
│
└── 嵌入 mihomo 二进制
    ├── Android libmihomo.so（arm64-v8a / armeabi-v7a / x86_64）
    └── iOS mihomo.xcframework（arm64 / x86_64-simulator）
```

**包体积估算**：
| 组成 | 大小 |
|---|---|
| Flutter 框架 | ~8 MB |
| mihomo 二进制（单 ABI）| ~12 MB |
| country.mmdb | ~4 MB |
| GeoSite.dat | ~7 MB |
| App 代码 + 资源 | ~3 MB |
| **合计（单 ABI APK）** | **~34 MB** |

---

## 十、MVP 交付标准

### Alpha（内测，Android）

全部满足才算 Alpha：

- [ ] 订阅 URL 导入 → 节点拉取成功，全程 ≤ 3 步
- [ ] 首次连接耗时 ≤ 3 秒（含配置生成 + mihomo 启动）
- [ ] 默认规则分流正常工作：国内域名直连，国外走代理
- [ ] WiFi ↔ 移动数据切换后 VPN 自动恢复，无需用户干预
- [ ] App 进后台 / 锁屏后 VPN 不断开
- [ ] 崩溃后自动重启，重启失败（3 次后）展示错误通知

Alpha 可缺失：延迟测速 UI、流量统计、日志页、GeoData 更新、按 App 分流

### Beta（公测，Android）

在 Alpha 基础上补齐：

- [ ] 延迟测速（单节点 + 全量）
- [ ] 实时连接列表
- [ ] 按 App 分流
- [ ] 自动订阅更新
- [ ] 开机自启
- [ ] 流量统计（今日/本月）

### v1.0（双端正式版）

- [ ] iOS 版本达到 Android Beta 同等功能
- [ ] App Store / Google Play 上架
- [ ] iOS Network Extension 内存优化通过审核

---

## 十一、不在本期范围

| 功能 | 原因 |
|---|---|
| 与路由器 ClashForge 同步订阅/节点 | 下一阶段 Companion 功能 |
| Cloudflare Worker 节点创建向导 | 导入已有配置可做（P2），创建流程复杂度高 |
| Hysteria2 / Reality 等新协议 | mihomo 支持，但 UI 和测试成本高，排期后续 |
| 付费 / 账户体系 | 产品定位为工具类，不依赖账户 |
| 流量审计 / 企业管控 | 面向个人用户 |
| macOS / Windows 客户端 | 有 ClashForge 路由器版覆盖桌面场景 |
