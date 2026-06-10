# ClashForge iOS 版本 — 架构与验收说明

> 分支 `feature/ios-app`。在 Windows 上开发，iOS 构建/出包全部由 GitHub Actions
> 的 macOS runner 完成（`.github/workflows/ios-ci.yml`），无需本地 Mac。

## 架构总览

Android 与 iOS 的 Dart/Flutter UI 层 100% 共享（`mobile/lib/` 零分叉，仅
`_fetchUrlWithFallback` 的平台判断从 Android-only 放宽到 Android+iOS）。
差异全部在原生层：

| 维度 | Android | iOS |
|---|---|---|
| VPN 载体 | `ClashVpnService`（同进程 VpnService） | PacketTunnel 网络扩展（独立进程，~50 MB jetsam 内存上限） |
| mihomo 运行方式 | fork `libmihomo.so` 子进程，TUN fd 走 stdin | **进程内嵌入**：gomobile 编译成 `Mihomobridge.xcframework`（iOS 禁止 fork/exec） |
| TUN fd 获取 | `Builder.establish()` 直接返回 | `setTunnelNetworkSettings` 后扫描进程 fd 表找 utun 控制 socket（WireGuardKit 同款技巧） |
| sing-tun fd 哨兵 | 需 patch（fd=0） | 不需要（darwin 实现原生接受 fd>0） |
| TUN 栈 | gvisor | **system**（省 10–20 MB 内存；gvisor 已编入可一行配置切回） |
| 防自环 | `addDisallowedApplication` | `auto-detect-interface: true`（绑定物理网卡） |
| 配置 patch / DNS 污染探测 | Kotlin（ClashVpnService） | **Go 移植**（`MihomoBridge/configpatch.go`、`dnsprobe.go`，带单测） |
| App↔核心通信 | 同进程 | App Group 容器共享 `config.yaml`/geodata；日志走 `logs/extension.jsonl`（扩展写、App 0.5s tail）；REST 9090 跨进程 loopback 直通 |
| 订阅原生抓取 | Cronet（Chrome JA3） | URLSession（Safari 指纹） |

Dart 的 MethodChannel 契约完全不变：`com.clashforge.mobile/vpn`（5 个方法，
含 `permission_needed` 语义 → iOS 映射为用户拒绝"添加 VPN 配置"系统弹窗）、
`com.clashforge.mobile/logs`（同 JSON schema）、`com.clashforge.mobile/http`。

## 新增文件

```
mobile/ios/
├── MihomoBridge/          Go 模块（gomobile bind 源）
│   ├── bridge.go          Start/Stop/IsRunning/ForceGC + 日志回调 + GC 调优
│   ├── configpatch.go     fake-ip 迁移 + tun/sniffer 注入（对齐 Kotlin patchConfigWithTun）
│   ├── dnsprobe.go        上游 DNS fake-ip 劫持探测 → DoH-only 自愈（对齐 Kotlin）
│   └── configpatch_test.go
├── Runner/                App：AppDelegate + VpnChannelHandler + LogEventBridge
│                          + HttpChannelHandler + Runner.entitlements
├── PacketTunnel/          扩展：PacketTunnelProvider + ExtensionLogger
│                          + Info.plist + entitlements
├── Shared/                双 target 共享：SharedPaths（App Group 路径）+ LogLine
└── Podfile                iOS 15.0
mobile/scripts/build-mihomo-ios.sh   gomobile 构建脚本（CI 调用）
.github/workflows/ios-ci.yml         桥接层 go test（Linux）+ 无签名全量构建（macOS）
```

## 连接时序（扩展进程）

1. App：写 `config.yaml` 到 App Group 容器，解出 geodata，`startVPNTunnel()`
2. 扩展 `startTunnel`：**先** `ProbeAndPatchDNS`（隧道未起，探测走物理网络）
3. `setTunnelNetworkSettings`：172.19.0.1/30、DNS 172.19.0.2、默认路由、MTU 1500
   （与 Android Builder 逐项一致，含 DNS 必须用 /30 对端地址的坑）
4. 扫 fd 表找到 utun fd → `PatchConfigWithTun(config, fd)`
5. `MihomobridgeStart` 进程内启动 mihomo（GOGC=30 / GOMEMLIMIT=40MiB /
   60s FreeOSMemory；配置注入 `geodata-loader: memconservative`）

## 明天接上开发者账号后要做的事

1. **App Store Connect / 开发者后台**（账号下来后）：
   - 注册 Bundle ID `com.clashforge.clashforgeMobile` 与
     `com.clashforge.clashforgeMobile.PacketTunnel`，都勾选
     **Network Extensions** capability 和 **App Groups**
   - 创建 App Group `group.com.clashforge.clashforgeMobile`
     （想换名字的话，同步改 `Shared/SharedPaths.swift`、两个 .entitlements、
     pbxproj 里的 `PRODUCT_BUNDLE_IDENTIFIER`）
2. **签名进 CI**：导出发布证书 .p12 + 两个 provisioning profile，存入 GitHub
   Secrets，再加一个签名出 ipa + 上传 TestFlight 的 workflow（账号就绪后我来补）
3. **真机验证清单**：
   - [ ] TestFlight 安装，首次连接弹"添加 VPN 配置"授权
   - [ ] 连接后国内外分流、DNS 不泄漏（用 App 内自带的探测页）
   - [ ] 扩展内存：设置 → 隐私 → 分析数据里看有无 jetsam 报告；超限则把
     geodata 换 Loyalsoldier cn-only 裁剪版（geosite 只留 cn/geolocation-!cn）
   - [ ] 锁屏 30 分钟后隧道保持
4. **已知限制**：
   - VPN 类 App 不能上中国区 App Store（TestFlight / 外区分发）
   - 扩展崩溃不会带崩 App，UI 会显示"未连接"；日志看 App 内日志页
     （扩展日志经 JSONL 桥接，组件名 `mihomo`/`vpn`/`dns`）
   - Android 的 Quick Settings 磁贴无 iOS 对应物（可后续加 Widget/快捷指令）
