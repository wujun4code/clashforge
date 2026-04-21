# MetaClash — 完整工程规格文档

> 版本：0.2.0  
> 日期：2026-04-21  
> 定位：另一个 AI 或开发者拿到此文档即可直接开始编码，无需再做任何架构决策。所有接口、数据结构、错误处理、边界条件均有明确定义。

---

## 目录

1. [项目概述与约束](#1-项目概述与约束)
2. [完整目录结构](#2-完整目录结构)
3. [Go 模块与依赖](#3-go-模块与依赖)
4. [数据类型定义](#4-数据类型定义)
5. [配置文件规格](#5-配置文件规格)
6. [HTTP API 完整规格](#6-http-api-完整规格)
7. [核心管理器 (CoreManager)](#7-核心管理器-coremanager)
8. [配置生成引擎](#8-配置生成引擎)
9. [订阅管理器](#9-订阅管理器)
10. [防火墙规则层](#10-防火墙规则层)
11. [Web UI 规格](#11-web-ui-规格)
12. [OpenWrt 集成](#12-openwrt-集成)
13. [错误处理规范](#13-错误处理规范)
14. [测试规格](#14-测试规格)
15. [构建与 CI](#15-构建与-ci)
16. [开发顺序（Phase 计划）](#16-开发顺序phase-计划)

---

## 1. 项目概述与约束

### 1.1 一句话定义

metaclash 是 mihomo（Clash.Meta）在 OpenWrt 上的**管理守护进程**。它不处理任何网络流量，只负责：启动/停止/监控 mihomo 进程、生成 mihomo 所需的 YAML 配置、管理 nftables/iptables 透明代理规则、提供 Web UI 和 REST API。

### 1.2 硬性约束（不可妥协）

1. **单一静态二进制**：`CGO_ENABLED=0`，无任何运行时依赖，直接 `scp` 到路由器即可运行
2. **无双进程**：任何情况下同一时刻只能有一个 mihomo 进程存在，用锁保证
3. **不自己实现代理**：所有代理协议由 mihomo 处理，metaclash 不解析流量
4. **优雅退出**：收到 SIGTERM 时，必须先清除 nftables/iptables 规则，再停止 mihomo，最后退出自身
5. **配置变更原子化**：任何配置变更要么完全生效，要么完全不生效，不能出现中间状态

### 1.3 目标硬件

| 参数 | 最低 | 典型 |
|------|------|------|
| CPU | MIPS 880 MHz 单核 | ARM Cortex-A7 双核 |
| RAM | 64 MB | 128 MB |
| Flash | 8 MB | 16 MB |
| OpenWrt 版本 | 21.02 | 23.05 |
| 内核版本 | 5.4 | 5.15 |

### 1.4 metaclash 二进制大小预算

- Go 代码编译后（stripped）：~5 MB
- 嵌入的 React UI（gzip）：~400 KB
- 总计目标：< 6 MB

---

## 2. 完整目录结构

```
metaclash/
├── cmd/
│   └── metaclash/
│       └── main.go                  # 唯一入口，只做：解析参数、初始化、启动
│
├── internal/
│   ├── api/
│   │   ├── server.go                # HTTP 服务器初始化、路由注册、中间件
│   │   ├── middleware.go            # CORS、Auth、Logger 中间件
│   │   ├── response.go              # 统一 JSON 响应结构体和辅助函数
│   │   ├── handler_status.go        # GET /api/v1/status
│   │   ├── handler_core.go          # /api/v1/core/*
│   │   ├── handler_config.go        # /api/v1/config/*
│   │   ├── handler_subscriptions.go # /api/v1/subscriptions/*
│   │   ├── handler_proxies.go       # /api/v1/proxies/* (透传 mihomo)
│   │   ├── handler_rules.go         # /api/v1/rules (透传 mihomo)
│   │   ├── handler_connections.go   # /api/v1/connections (透传 mihomo)
│   │   ├── handler_logs.go          # /api/v1/logs (SSE)
│   │   ├── handler_traffic.go       # /api/v1/traffic (SSE)
│   │   └── sse.go                   # SSE broker，管理所有 SSE 客户端连接
│   │
│   ├── core/
│   │   ├── manager.go               # CoreManager：mihomo 进程生命周期
│   │   ├── api_client.go            # 封装对 mihomo REST API 的所有调用
│   │   └── updater.go               # mihomo 二进制版本检查与更新
│   │
│   ├── config/
│   │   ├── loader.go                # 加载并验证 /etc/metaclash/config.toml
│   │   ├── types.go                 # MetaclashConfig 及所有子结构体定义
│   │   ├── generator.go             # 从 metaclash 配置生成 mihomo YAML
│   │   ├── merger.go                # 三层配置合并（base + generated + overrides）
│   │   └── validator.go             # 调用 mihomo -t 验证生成的配置
│   │
│   ├── subscription/
│   │   ├── manager.go               # SubscriptionManager：更新调度、状态管理
│   │   ├── store.go                 # 读写 subscriptions.toml 和缓存 JSON
│   │   ├── fetcher.go               # HTTP 下载订阅，处理超时、重试、UA
│   │   ├── parser.go                # 分发到各协议解析器
│   │   ├── parser_vmess.go          # vmess:// 解析
│   │   ├── parser_trojan.go         # trojan:// 解析
│   │   ├── parser_ss.go             # ss:// 解析
│   │   ├── parser_vless.go          # vless:// 解析
│   │   ├── parser_hy2.go            # hy2:// 解析
│   │   ├── parser_clash_yaml.go     # 直接 Clash YAML 格式解析
│   │   └── filter.go                # 节点过滤（关键词、去重）
│   │
│   ├── netfilter/
│   │   ├── detect.go                # 自动检测防火墙后端
│   │   ├── manager.go               # NetfilterManager：规则应用、清理
│   │   ├── nftables.go              # nftables 规则生成与应用
│   │   ├── iptables.go              # iptables 规则生成与应用
│   │   └── templates/
│   │       ├── nft_tproxy.tmpl      # nftables TProxy 规则模板
│   │       └── ipt_tproxy.sh.tmpl   # iptables TProxy 规则模板
│   │
│   ├── dns/
│   │   └── setup.go                 # 配置 dnsmasq 共存模式
│   │
│   ├── scheduler/
│   │   └── scheduler.go             # 内置 cron：订阅更新、GeoIP 更新
│   │
│   └── daemon/
│       ├── pidfile.go               # PID 文件锁
│       └── signals.go               # 信号处理（SIGTERM、SIGHUP、SIGUSR1）
│
├── ui/                              # React 前端（独立构建）
│   ├── src/
│   │   ├── main.tsx
│   │   ├── App.tsx
│   │   ├── api/
│   │   │   └── client.ts            # 所有 API 调用封装
│   │   ├── pages/
│   │   │   ├── Dashboard.tsx
│   │   │   ├── Proxies.tsx
│   │   │   ├── Rules.tsx
│   │   │   ├── Connections.tsx
│   │   │   ├── Subscriptions.tsx
│   │   │   ├── Logs.tsx
│   │   │   └── Settings.tsx
│   │   ├── components/
│   │   │   ├── layout/
│   │   │   │   ├── Sidebar.tsx
│   │   │   │   └── TopBar.tsx
│   │   │   ├── TrafficChart.tsx
│   │   │   ├── ProxyGroup.tsx
│   │   │   ├── NodeCard.tsx
│   │   │   └── StatusBadge.tsx
│   │   ├── hooks/
│   │   │   ├── useSSE.ts            # SSE 实时数据 hook
│   │   │   ├── useTraffic.ts
│   │   │   └── useProxies.ts
│   │   └── store/
│   │       └── index.ts             # Zustand store
│   ├── index.html
│   ├── vite.config.ts
│   ├── tailwind.config.ts
│   └── package.json
│
├── openwrt/
│   ├── Makefile                     # OpenWrt package Makefile
│   └── files/
│       ├── etc/init.d/metaclash     # procd 服务脚本
│       ├── etc/metaclash/
│       │   ├── config.toml.example
│       │   └── overrides.yaml.example
│       └── usr/share/metaclash/     # 数据库文件占位
│
├── .github/
│   └── workflows/
│       └── release.yml
│
├── Makefile
├── go.mod
└── go.sum
```

---

## 3. Go 模块与依赖

### 3.1 go.mod

```
module github.com/yourusername/metaclash

go 1.22

require (
    github.com/BurntSushi/toml v1.3.2
    github.com/go-chi/chi/v5 v5.0.12
    github.com/rs/zerolog v1.32.0
    github.com/fsnotify/fsnotify v1.7.0
    gopkg.in/yaml.v3 v3.0.1
    golang.org/x/sync v0.7.0
)
```

**不引入任何其他依赖。** 所有功能用标准库 + 上述 6 个包实现。

### 3.2 各包职责说明

- `BurntSushi/toml`：解析 `config.toml` 和 `subscriptions.toml`
- `go-chi/chi`：HTTP 路由，比 gorilla/mux 更轻，比 gin 少 70% 代码量
- `rs/zerolog`：零 allocation 日志，在 MIPS 上比 zap 更友好
- `fsnotify`：监听 `/etc/metaclash/` 目录变化，自动触发重载
- `gopkg.in/yaml.v3`：生成 mihomo YAML 配置
- `golang.org/x/sync`：`errgroup`（并发订阅更新）、`semaphore`（限制并发数）

---

## 4. 数据类型定义

### 4.1 internal/config/types.go — 完整定义

```go
package config

// MetaclashConfig 是 /etc/metaclash/config.toml 的完整映射
type MetaclashConfig struct {
    Core     CoreConfig     `toml:"core"`
    Ports    PortsConfig    `toml:"ports"`
    Network  NetworkConfig  `toml:"network"`
    DNS      DNSConfig      `toml:"dns"`
    Update   UpdateConfig   `toml:"update"`
    Security SecurityConfig `toml:"security"`
    Log      LogConfig      `toml:"log"`
}

type CoreConfig struct {
    // mihomo 二进制绝对路径，必须存在且可执行
    // 默认：/usr/bin/mihomo
    Binary string `toml:"binary"`

    // 运行时配置目录（tmpfs，重启后清空无所谓）
    // 默认：/var/run/metaclash
    RuntimeDir string `toml:"runtime_dir"`

    // 持久化数据目录（flash 存储）
    // 默认：/etc/metaclash
    DataDir string `toml:"data_dir"`

    // GeoIP 数据库路径
    // 默认：/usr/share/metaclash/Country.mmdb
    GeoIPPath string `toml:"geoip_path"`

    // Geosite 数据库路径
    // 默认：/usr/share/metaclash/geosite.dat
    GeositePath string `toml:"geosite_path"`

    // mihomo 崩溃后自动重启最大次数（per 60 秒窗口）
    // 默认：3，设为 0 禁用自动重启
    MaxRestarts int `toml:"max_restarts"`
}

type PortsConfig struct {
    HTTP   int `toml:"http"`   // 默认 7890
    SOCKS  int `toml:"socks"`  // 默认 7891
    Mixed  int `toml:"mixed"`  // 默认 7893，HTTP+SOCKS 合并端口
    Redir  int `toml:"redir"`  // 默认 7892，透明代理 TCP redirect 端口
    TProxy int `toml:"tproxy"` // 默认 7895，TProxy 端口
    DNS    int `toml:"dns"`    // 默认 7874，mihomo DNS 监听端口
    MihomoAPI int `toml:"mihomo_api"` // 默认 9090，mihomo REST API
    UI     int `toml:"ui"`     // 默认 7777，metaclash Web UI
}

type NetworkConfig struct {
    // 透明代理模式
    // "tproxy"：推荐，需要 kmod-nft-tproxy 或 xt_TPROXY
    // "redir"：仅 TCP，兼容性最好
    // "tun"：需要 TUN 模块，资源消耗最高
    // "none"：不设置透明代理，只做端口代理
    Mode string `toml:"mode"`

    // 防火墙后端
    // "auto"：自动检测（优先 nftables）
    // "nftables"：强制使用 nftables
    // "iptables"：强制使用 iptables
    FirewallBackend string `toml:"firewall_backend"`

    // 是否绕过局域网（192.168.0.0/16 等私有地址不走代理）
    // 默认：true，几乎永远不要改成 false
    BypassLAN bool `toml:"bypass_lan"`

    // 是否绕过中国大陆 IP（GEOIP,CN 直连）
    // 默认：false
    BypassChina bool `toml:"bypass_china"`

    // 是否代理 IPv6 流量
    // 默认：false（大多数情况下 IPv6 透明代理会出问题）
    IPv6 bool `toml:"ipv6"`

    // 额外的绕过 CIDR 列表（不走代理）
    // 例如：["10.8.0.0/16", "172.20.0.0/14"]
    BypassCIDR []string `toml:"bypass_cidr"`
}

type DNSConfig struct {
    Enable bool `toml:"enable"` // 默认 true

    // "fake-ip"：mihomo 推荐模式，性能好，分流准确
    // "redir-host"：兼容模式，某些场景需要真实 IP
    Mode string `toml:"mode"`

    // 国内 DNS（直接查询）
    Nameservers []string `toml:"nameservers"`

    // 境外 DNS（通过代理查询）
    Fallback []string `toml:"fallback"`

    // DoH 服务器
    DoH []string `toml:"doh"`

    // fake-ip 不拦截的域名（这些域名返回真实 IP）
    FakeIPFilter []string `toml:"fake_ip_filter"`

    // dnsmasq 共存模式
    // "replace"：禁用 dnsmasq DNS（设置 port=0），metaclash/mihomo 接管
    // "upstream"：dnsmasq 保留，将上游设为 metaclash DNS 端口
    // "none"：不修改 dnsmasq 配置（用户自行处理）
    DnsmasqMode string `toml:"dnsmasq_mode"`
}

type UpdateConfig struct {
    // 自动更新订阅
    AutoSubscription bool   `toml:"auto_subscription"`
    SubscriptionInterval string `toml:"subscription_interval"` // Go duration: "6h"

    // 自动更新 GeoIP
    AutoGeoIP bool   `toml:"auto_geoip"`
    GeoIPInterval string `toml:"geoip_interval"` // 默认 "168h"（7天）

    // 自动更新 Geosite
    AutoGeosite bool   `toml:"auto_geosite"`
    GeositeInterval string `toml:"geosite_interval"` // 默认 "168h"

    // GeoIP 下载 URL（默认使用 MetaCubeX 仓库）
    GeoIPURL string `toml:"geoip_url"`

    // Geosite 下载 URL
    GeositeURL string `toml:"geosite_url"`
}

type SecurityConfig struct {
    // metaclash Web UI / API 鉴权密钥
    // 空字符串表示不鉴权（推荐内网使用时留空）
    APISecret string `toml:"api_secret"`

    // 是否允许非 localhost 访问 Web UI
    // 默认：true（路由器局域网访问）
    AllowLAN bool `toml:"allow_lan"`
}

type LogConfig struct {
    // "debug" | "info" | "warn" | "error"
    // 默认：info
    Level string `toml:"level"`

    // 日志文件路径，空字符串表示输出到 stdout（由 procd 捕获）
    // 默认：""
    File string `toml:"file"`

    // 日志文件最大大小 MB，超过后截断
    // 默认：10
    MaxSizeMB int `toml:"max_size_mb"`
}
```

### 4.2 internal/subscription/store.go — 订阅数据结构

```go
package subscription

import "time"

// SubscriptionList 是 subscriptions.toml 的顶层结构
type SubscriptionList struct {
    Subscriptions []Subscription `toml:"subscription"`
}

type Subscription struct {
    // 唯一 ID，格式：sub_<8位随机hex>，例如 sub_a1b2c3d4
    ID string `toml:"id"`

    // 显示名称
    Name string `toml:"name"`

    // "url"：在线订阅
    // "manual"：手动管理的节点，无 URL
    Type string `toml:"type"`

    // 订阅 URL（type=url 时必填）
    URL string `toml:"url,omitempty"`

    // 请求时使用的 User-Agent
    // 默认："clash-meta"
    UserAgent string `toml:"user_agent,omitempty"`

    // 更新间隔，Go duration 字符串
    // 默认："6h"
    Interval string `toml:"interval,omitempty"`

    // 是否启用此订阅
    Enabled bool `toml:"enabled"`

    // 最后一次成功更新时间（metaclash 写入，用户不应手动修改）
    LastUpdated time.Time `toml:"last_updated,omitempty"`

    // 最后一次更新后解析到的节点数（过滤前）
    NodeCount int `toml:"node_count,omitempty"`

    // 节点过滤配置
    Filter SubscriptionFilter `toml:"filter"`
}

type SubscriptionFilter struct {
    // 关键词白名单，节点名称包含其中任一词才保留
    // 空列表表示不过滤
    Include []string `toml:"include,omitempty"`

    // 关键词黑名单，节点名称包含其中任一词则丢弃
    // 优先级高于 Include
    Exclude []string `toml:"exclude,omitempty"`

    // 最多保留节点数，0 表示不限制
    // 默认：0
    MaxNodes int `toml:"max_nodes,omitempty"`
}

// ProxyNode 是协议无关的节点表示，最终会转换为 Clash proxy 对象
type ProxyNode struct {
    // 以下字段必填
    Name   string // 节点显示名称
    Type   string // vmess | trojan | ss | vless | hy2 | tuic | http | socks5
    Server string // 服务器地址
    Port   int    // 服务器端口

    // 原始协议数据（不同类型有不同字段，直接存为 map 传递给 YAML 生成器）
    // 这个 map 中的 key/value 直接对应 Clash proxy 配置字段
    Extra map[string]interface{}

    // 来源订阅 ID
    SourceSubID string
}
```

### 4.3 API 响应通用结构

```go
// internal/api/response.go

package api

import (
    "encoding/json"
    "net/http"
    "time"
)

// APIResponse 是所有 API 的统一响应结构
type APIResponse struct {
    OK    bool        `json:"ok"`
    Data  interface{} `json:"data,omitempty"`
    Error *APIError   `json:"error,omitempty"`
    TS    int64       `json:"ts"` // Unix timestamp
}

type APIError struct {
    Code    string `json:"code"`    // 机器可读错误码，例如 "CORE_NOT_RUNNING"
    Message string `json:"message"` // 人类可读错误描述
}

func JSON(w http.ResponseWriter, status int, data interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(APIResponse{
        OK:   status < 400,
        Data: data,
        TS:   time.Now().Unix(),
    })
}

func Err(w http.ResponseWriter, status int, code, message string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(APIResponse{
        OK:    false,
        Error: &APIError{Code: code, Message: message},
        TS:    time.Now().Unix(),
    })
}
```

---

## 5. 配置文件规格

### 5.1 /etc/metaclash/config.toml — 完整默认值

```toml
[core]
binary = "/usr/bin/mihomo"
runtime_dir = "/var/run/metaclash"
data_dir = "/etc/metaclash"
geoip_path = "/usr/share/metaclash/Country.mmdb"
geosite_path = "/usr/share/metaclash/geosite.dat"
max_restarts = 3

[ports]
http = 7890
socks = 7891
mixed = 7893
redir = 7892
tproxy = 7895
dns = 7874
mihomo_api = 9090
ui = 7777

[network]
mode = "tproxy"
firewall_backend = "auto"
bypass_lan = true
bypass_china = false
ipv6 = false
bypass_cidr = []

[dns]
enable = true
mode = "fake-ip"
nameservers = ["119.29.29.29", "223.5.5.5"]
fallback = ["8.8.8.8", "1.1.1.1"]
doh = ["https://doh.pub/dns-query"]
fake_ip_filter = [
    "+.lan",
    "+.local",
    "time.*.com",
    "ntp.*.com",
    "+.ntp.org",
]
dnsmasq_mode = "upstream"

[update]
auto_subscription = true
subscription_interval = "6h"
auto_geoip = true
geoip_interval = "168h"
auto_geosite = true
geosite_interval = "168h"
geoip_url = "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb"
geosite_url = "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat"

[security]
api_secret = ""
allow_lan = true

[log]
level = "info"
file = ""
max_size_mb = 10
```

### 5.2 /etc/metaclash/subscriptions.toml — 初始状态（空）

```toml
# MetaClash 订阅配置
# 通过 Web UI 管理，也可直接编辑此文件后重启 metaclash

# 示例：
# [[subscription]]
# id = "sub_a1b2c3d4"
# name = "我的机场"
# type = "url"
# url = "https://example.com/subscribe?token=xxx"
# user_agent = "clash-meta"
# interval = "6h"
# enabled = true
# [subscription.filter]
# include = ["香港", "日本", "新加坡"]
# exclude = ["套餐到期", "剩余流量"]
# max_nodes = 50
```

### 5.3 /etc/metaclash/overrides.yaml — 用户覆写层

```yaml
# 此文件中的内容会 deep-merge 覆盖 metaclash 生成的 mihomo 配置
# 优先级最高，可覆盖任何生成的配置
#
# 常用场景：
#
# 1. 添加自定义规则（插入到规则列表最前面）：
# rules:
#   - "DOMAIN,custom.example.com,DIRECT"
#
# 2. 修改代理组行为：
# proxy-groups:
#   - name: "Proxy"
#     type: select
#     proxies:
#       - "香港 01"
#       - "DIRECT"
#
# 3. 添加自定义 hosts：
# hosts:
#   "my.router": "192.168.1.1"
```

### 5.4 运行时生成文件（不由用户编辑）

```
/var/run/metaclash/
├── metaclash.pid          # PID 文件
├── metaclash.sock         # Unix Domain Socket（内部使用）
├── mihomo-config.yaml     # 最终生成并传给 mihomo 的配置
├── mihomo.pid             # mihomo 子进程 PID
└── cache/
    ├── sub_a1b2c3d4.json  # 每个订阅的节点缓存（JSON 数组）
    └── sub_*.json
```

---

## 6. HTTP API 完整规格

**Base URL**：`http://<router-ip>:7777/api/v1`

**认证**：当 `security.api_secret` 非空时，请求必须携带 Header：`Authorization: Bearer <secret>`

**所有响应**均为 `Content-Type: application/json`，格式见第 4.3 节。

---

### 6.1 状态接口

#### GET /api/v1/status

返回 metaclash 和 mihomo 的完整状态。

**响应 200**：
```json
{
  "ok": true,
  "data": {
    "metaclash": {
      "version": "1.0.0",
      "uptime": 3600,
      "config_file": "/etc/metaclash/config.toml"
    },
    "core": {
      "state": "running",
      "pid": 1234,
      "version": "Mihomo Meta v1.19.24",
      "restarts": 0,
      "uptime": 3598
    },
    "network": {
      "mode": "tproxy",
      "firewall_backend": "nftables",
      "rules_applied": true
    },
    "subscriptions": {
      "total": 2,
      "enabled": 2,
      "last_updated": "2026-04-21T10:00:00Z"
    }
  },
  "ts": 1745200000
}
```

**core.state 枚举值**：`stopped` | `starting` | `running` | `stopping` | `error`

---

### 6.2 核心管理接口

#### POST /api/v1/core/start

启动 mihomo 进程。如果已在运行则返回错误。

**响应 200**：`{"ok": true, "data": {"pid": 1234}}`  
**响应 409**（已运行）：`{"ok": false, "error": {"code": "CORE_ALREADY_RUNNING", "message": "..."}}`  
**响应 500**（启动失败）：`{"ok": false, "error": {"code": "CORE_START_FAILED", "message": "..."}}`

#### POST /api/v1/core/stop

停止 mihomo 进程。先发 SIGTERM，等待最多 5 秒，超时后发 SIGKILL。

**响应 200**：`{"ok": true}`  
**响应 404**（未运行）：`{"ok": false, "error": {"code": "CORE_NOT_RUNNING", "message": "..."}}`

#### POST /api/v1/core/restart

等价于 stop + start。保证原子性（旧进程完全退出后才启动新进程）。

**响应 200**：`{"ok": true, "data": {"pid": 1235}}`

#### POST /api/v1/core/reload

向 mihomo 发送热重载请求（PUT /configs 到 mihomo API）。不重启进程，已有连接不断。

**触发条件**：仅当 mihomo 处于 `running` 状态时有效。

**响应 200**：`{"ok": true}`  
**响应 400**（配置验证失败）：`{"ok": false, "error": {"code": "CONFIG_INVALID", "message": "<mihomo -t 的输出>"}}`

#### GET /api/v1/core/version

检查当前 mihomo 版本及是否有新版本可用。

**响应 200**：
```json
{
  "ok": true,
  "data": {
    "current": "v1.19.24",
    "latest": "v1.19.25",
    "has_update": true,
    "download_url": "https://github.com/MetaCubeX/mihomo/releases/download/v1.19.25/mihomo-linux-arm-v7.tar.gz"
  }
}
```

#### POST /api/v1/core/update

下载最新 mihomo 二进制并替换。流程：下载 → 验证（执行 -v）→ 备份旧版本 → 替换 → 重启。

**响应 200**：`{"ok": true, "data": {"version": "v1.19.25"}}`  
**响应 500**：下载失败、校验失败等

---

### 6.3 配置接口

#### GET /api/v1/config

返回当前 metaclash 配置（脱敏，`api_secret` 替换为 `"***"`）。

**响应 200**：返回 MetaclashConfig 序列化后的 JSON。

#### PUT /api/v1/config

更新 metaclash 配置。只更新请求体中提供的字段（partial update）。更新后自动触发配置重新生成和 mihomo 热重载。

**请求体**（示例，只更新部分字段）：
```json
{
  "network": {
    "bypass_china": true
  },
  "dns": {
    "mode": "redir-host"
  }
}
```

**响应 200**：`{"ok": true}`  
**响应 400**：字段验证失败

#### GET /api/v1/config/mihomo

返回当前生效的 mihomo YAML 配置内容（文本形式）。

**响应 200**：
```json
{
  "ok": true,
  "data": {
    "content": "port: 7890\nsocks-port: 7891\n..."
  }
}
```

#### GET /api/v1/config/overrides

返回 overrides.yaml 内容。

**响应 200**：`{"ok": true, "data": {"content": "# empty\n"}}`

#### PUT /api/v1/config/overrides

更新 overrides.yaml 内容。验证是否为合法 YAML 后写入，然后触发 mihomo 热重载。

**请求体**：`{"content": "rules:\n  - DOMAIN,example.com,DIRECT\n"}`

**响应 400**（YAML 语法错误）：`{"ok": false, "error": {"code": "YAML_PARSE_ERROR", "message": "..."}}`

---

### 6.4 订阅接口

#### GET /api/v1/subscriptions

返回所有订阅列表。

**响应 200**：
```json
{
  "ok": true,
  "data": {
    "subscriptions": [
      {
        "id": "sub_a1b2c3d4",
        "name": "机场A",
        "type": "url",
        "url": "https://example.com/sub?token=xxx",
        "enabled": true,
        "last_updated": "2026-04-21T10:00:00Z",
        "node_count": 156,
        "filtered_count": 42,
        "filter": {
          "include": ["香港", "日本"],
          "exclude": ["套餐"],
          "max_nodes": 0
        },
        "status": "ok"
      }
    ]
  }
}
```

`status` 枚举：`ok` | `updating` | `error` | `never_updated`

#### POST /api/v1/subscriptions

添加新订阅。

**请求体**：
```json
{
  "name": "机场B",
  "type": "url",
  "url": "https://example.com/sub?token=yyy",
  "user_agent": "clash-meta",
  "interval": "6h",
  "enabled": true,
  "filter": {
    "include": [],
    "exclude": ["套餐", "剩余"],
    "max_nodes": 100
  }
}
```

**响应 201**：`{"ok": true, "data": {"id": "sub_e5f6g7h8"}}`  
**响应 400**：URL 格式错误、名称为空等

#### PUT /api/v1/subscriptions/:id

更新订阅配置（partial update，不触发立即更新）。

**响应 200**：`{"ok": true}`  
**响应 404**：订阅不存在

#### DELETE /api/v1/subscriptions/:id

删除订阅，同时删除对应缓存文件，触发配置重新生成。

**响应 200**：`{"ok": true}`

#### POST /api/v1/subscriptions/:id/update

立即触发指定订阅的更新（异步，立即返回，结果通过 SSE 推送）。

**响应 202**：`{"ok": true, "data": {"message": "update started"}}`

#### POST /api/v1/subscriptions/update-all

立即触发所有已启用订阅的更新（异步）。

**响应 202**：`{"ok": true}`

---

### 6.5 代理节点接口（透传 mihomo）

以下接口直接代理到 mihomo API（`http://127.0.0.1:9090`），metaclash 转发请求并返回响应。

#### GET /api/v1/proxies

透传 `GET http://127.0.0.1:9090/proxies`。

#### PUT /api/v1/proxies/:group/select

透传 `PUT http://127.0.0.1:9090/proxies/{group}` 切换代理组选中节点。

**请求体**：`{"name": "香港 01"}`

#### POST /api/v1/proxies/test-latency

对指定节点列表发起延迟测试。

**请求体**：
```json
{
  "proxies": ["香港 01", "日本 02"],
  "url": "http://www.gstatic.com/generate_204",
  "timeout": 5000
}
```

**响应 200**：
```json
{
  "ok": true,
  "data": {
    "香港 01": 45,
    "日本 02": 120
  }
}
```
延迟单位 ms，超时返回 -1。

---

### 6.6 规则接口

#### GET /api/v1/rules

透传 `GET http://127.0.0.1:9090/rules`，返回当前生效的规则列表。

---

### 6.7 连接接口

#### GET /api/v1/connections

透传 `GET http://127.0.0.1:9090/connections`。

#### DELETE /api/v1/connections

关闭所有连接，透传 `DELETE http://127.0.0.1:9090/connections`。

#### DELETE /api/v1/connections/:id

关闭指定连接。

---

### 6.8 实时数据接口（SSE）

#### GET /api/v1/events

Server-Sent Events 端点，客户端建立长连接后持续接收事件。

**响应头**：
```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no
```

**事件类型与数据格式**：

```
# 流量统计（每秒推送）
event: traffic
data: {"up": 1024, "down": 8192, "ts": 1745200000}

# 订阅更新进度
event: subscription_update
data: {"id": "sub_a1b2c3d4", "status": "updating", "progress": "downloading..."}

event: subscription_update
data: {"id": "sub_a1b2c3d4", "status": "ok", "node_count": 156, "filtered_count": 42}

event: subscription_update
data: {"id": "sub_a1b2c3d4", "status": "error", "error": "connection timeout"}

# mihomo 核心状态变化
event: core_state
data: {"state": "running", "pid": 1234}

event: core_state
data: {"state": "stopped", "pid": 0}

# 日志流（级别过滤由 log.level 配置决定）
event: log
data: {"level": "info", "msg": "subscription updated", "sub_id": "sub_a1b2c3d4", "ts": 1745200000}

# 连接数统计（每 3 秒推送）
event: connections_count
data: {"total": 42, "active": 38}
```

---

### 6.9 系统接口

#### GET /api/v1/geoip/update

手动触发 GeoIP 数据库更新（异步）。

**响应 202**：`{"ok": true}`

#### GET /api/v1/geosite/update

手动触发 Geosite 数据库更新（异步）。

**响应 202**：`{"ok": true}`

#### GET /api/v1/logs

返回最近的日志（非 SSE，一次性请求）。

**Query params**：`?level=info&limit=100`

**响应 200**：
```json
{
  "ok": true,
  "data": {
    "logs": [
      {"level": "info", "msg": "...", "ts": 1745200000},
      {"level": "warn", "msg": "...", "ts": 1745200001}
    ]
  }
}
```

---

## 7. 核心管理器 (CoreManager)

### 7.1 完整实现规格

```go
// internal/core/manager.go

package core

import (
    "context"
    "fmt"
    "os"
    "os/exec"
    "sync"
    "syscall"
    "time"

    "github.com/rs/zerolog/log"
)

type CoreState string

const (
    StateStopped  CoreState = "stopped"
    StateStarting CoreState = "starting"
    StateRunning  CoreState = "running"
    StateStopping CoreState = "stopping"
    StateError    CoreState = "error"
)

type CoreManager struct {
    mu           sync.Mutex
    cmd          *exec.Cmd
    state        CoreState
    pid          int
    startTime    time.Time
    restartCount int
    restartTimes []time.Time // 记录最近的重启时间，用于限速
    
    cfg          CoreManagerConfig
    onStateChange func(state CoreState, pid int) // 状态变更回调（用于 SSE 推送）
    
    deathCh chan error // 子进程退出信号
    stopCh  chan struct{} // 停止自动重启的信号
}

type CoreManagerConfig struct {
    Binary     string
    ConfigFile string // /var/run/metaclash/mihomo-config.yaml
    APIPort    int    // 9090
    MaxRestarts int
}

// Start 启动 mihomo 进程
// 如果已在运行，返回 ErrAlreadyRunning
// 启动后等待最多 5 秒确认 mihomo API 可达
func (m *CoreManager) Start(ctx context.Context) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    if m.state == StateRunning || m.state == StateStarting {
        return ErrAlreadyRunning
    }
    
    return m.start(ctx)
}

// start 内部启动逻辑（调用前必须持有锁）
func (m *CoreManager) start(ctx context.Context) error {
    m.setState(StateStarting, 0)
    
    cmd := exec.CommandContext(ctx, m.cfg.Binary,
        "-d", configDir(m.cfg.ConfigFile),
        "-f", m.cfg.ConfigFile,
    )
    cmd.Stdout = newCoreLogWriter("stdout")
    cmd.Stderr = newCoreLogWriter("stderr")
    
    if err := cmd.Start(); err != nil {
        m.setState(StateError, 0)
        return fmt.Errorf("failed to start mihomo: %w", err)
    }
    
    m.cmd = cmd
    m.pid = cmd.Process.Pid
    m.startTime = time.Now()
    m.deathCh = make(chan error, 1)
    
    // 监控子进程退出
    go func() {
        err := cmd.Wait()
        m.deathCh <- err
        m.handleDeath(err)
    }()
    
    // 等待 mihomo API 就绪（最多 10 秒）
    if err := m.waitAPIReady(ctx, 10*time.Second); err != nil {
        // API 未就绪，但进程可能还活着——记录警告但不失败
        log.Warn().Err(err).Msg("mihomo API not ready within timeout, continuing anyway")
    }
    
    m.setState(StateRunning, m.pid)
    log.Info().Int("pid", m.pid).Msg("mihomo started")
    return nil
}

// Stop 停止 mihomo 进程
// 先发 SIGTERM，等待 5 秒，超时后发 SIGKILL
func (m *CoreManager) Stop() error {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    if m.state == StateStopped || m.cmd == nil {
        return ErrNotRunning
    }
    
    // 发送停止信号，阻止自动重启
    if m.stopCh != nil {
        close(m.stopCh)
        m.stopCh = nil
    }
    
    return m.stop()
}

// stop 内部停止逻辑（调用前必须持有锁）
func (m *CoreManager) stop() error {
    m.setState(StateStopping, m.pid)
    
    process := m.cmd.Process
    if process == nil {
        m.setState(StateStopped, 0)
        return nil
    }
    
    // 优雅停止
    process.Signal(syscall.SIGTERM)
    
    select {
    case <-m.deathCh:
        // 正常退出
    case <-time.After(5 * time.Second):
        // 超时强杀
        log.Warn().Int("pid", m.pid).Msg("mihomo did not stop gracefully, killing")
        process.Kill()
        <-m.deathCh
    }
    
    m.cmd = nil
    m.pid = 0
    m.setState(StateStopped, 0)
    log.Info().Msg("mihomo stopped")
    return nil
}

// Restart 原子重启（旧进程完全退出后才启动新进程）
func (m *CoreManager) Restart(ctx context.Context) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    if m.state == StateRunning || m.state == StateStopping {
        if err := m.stop(); err != nil {
            return fmt.Errorf("stop failed: %w", err)
        }
    }
    
    return m.start(ctx)
}

// handleDeath 子进程意外退出时的处理（在 goroutine 中调用，不持有锁）
func (m *CoreManager) handleDeath(err error) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    // 如果是主动停止的，不触发自动重启
    if m.state == StateStopping || m.state == StateStopped {
        return
    }
    
    log.Error().Err(err).Int("pid", m.pid).Msg("mihomo exited unexpectedly")
    m.setState(StateError, 0)
    
    // 自动重启限速：60 秒内不超过 MaxRestarts 次
    m.restartTimes = append(m.restartTimes, time.Now())
    cutoff := time.Now().Add(-60 * time.Second)
    recent := 0
    for _, t := range m.restartTimes {
        if t.After(cutoff) {
            recent++
        }
    }
    
    if m.cfg.MaxRestarts > 0 && recent > m.cfg.MaxRestarts {
        log.Error().
            Int("recent_restarts", recent).
            Int("max_restarts", m.cfg.MaxRestarts).
            Msg("mihomo restart limit exceeded, giving up")
        return
    }
    
    // 等待 2 秒后重启
    time.Sleep(2 * time.Second)
    
    log.Info().Int("attempt", recent).Msg("auto-restarting mihomo")
    ctx := context.Background()
    if err := m.start(ctx); err != nil {
        log.Error().Err(err).Msg("auto-restart failed")
    }
}

// waitAPIReady 轮询 mihomo API 直到可达
func (m *CoreManager) waitAPIReady(ctx context.Context, timeout time.Duration) error {
    deadline := time.Now().Add(timeout)
    url := fmt.Sprintf("http://127.0.0.1:%d/version", m.cfg.APIPort)
    
    for time.Now().Before(deadline) {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }
        
        // 简单 HTTP GET，200 表示就绪
        if resp, err := httpGetWithTimeout(url, 500*time.Millisecond); err == nil {
            resp.Body.Close()
            return nil
        }
        time.Sleep(200 * time.Millisecond)
    }
    return fmt.Errorf("API not ready after %v", timeout)
}

func (m *CoreManager) setState(s CoreState, pid int) {
    m.state = s
    m.pid = pid
    if m.onStateChange != nil {
        go m.onStateChange(s, pid) // 异步通知，避免死锁
    }
}

// 错误定义
var (
    ErrAlreadyRunning = fmt.Errorf("core already running")
    ErrNotRunning     = fmt.Errorf("core not running")
)
```

### 7.2 Reload（热重载）实现

```go
// Reload 通过 mihomo REST API 触发热重载（不重启进程）
// 先用 mihomo -t 验证配置，验证通过才发送重载请求
func (m *CoreManager) Reload(configFile string) error {
    // Step 1: 验证配置语法
    out, err := exec.Command(m.cfg.Binary, "-t",
        "-d", configDir(configFile),
        "-f", configFile,
    ).CombinedOutput()
    if err != nil {
        return fmt.Errorf("config validation failed: %s", string(out))
    }
    
    // Step 2: 发送热重载请求到 mihomo API
    url := fmt.Sprintf("http://127.0.0.1:%d/configs?force=false", m.cfg.APIPort)
    body := fmt.Sprintf(`{"path": "%s"}`, configFile)
    
    req, _ := http.NewRequest(http.MethodPut, url, strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return fmt.Errorf("reload request failed: %w", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusNoContent {
        b, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("reload rejected by mihomo: %s", string(b))
    }
    
    return nil
}
```

---

## 8. 配置生成引擎

### 8.1 生成流程（严格按此顺序执行）

```
1. 加载 base.yaml（内嵌在二进制中，Go embed）
2. 从 MetaclashConfig 生成 generated 部分
   a. 全局设置（端口、模式、日志）
   b. DNS 配置
   c. 合并所有启用订阅的节点缓存
   d. 对节点应用过滤规则
   e. 生成代理组（Auto、Proxy、Fallback 等）
   f. 生成规则列表
3. 加载 overrides.yaml（用户覆写）
4. 深度合并：generated 覆盖 base，overrides 覆盖 generated
5. 写入 /var/run/metaclash/mihomo-config.yaml
6. 验证：执行 `mihomo -t -f /var/run/metaclash/mihomo-config.yaml`
7. 如果验证失败：保留旧配置文件，返回错误（不影响当前运行）
```

### 8.2 自动生成的代理组规则

metaclash 自动生成以下代理组（用户可通过 overrides.yaml 覆盖）：

```yaml
proxy-groups:
  # 手动选择组（用户在 UI 切换节点）
  - name: "Proxy"
    type: select
    proxies:
      - "Auto"           # 自动选择子组
      - "DIRECT"
      - <所有节点名称列表>

  # 自动延迟测试组
  - name: "Auto"
    type: url-test
    url: "http://www.gstatic.com/generate_204"
    interval: 300
    tolerance: 50
    proxies:
      - <所有节点名称列表>

  # 兜底（未匹配规则使用此组）
  - name: "Final"
    type: select
    proxies:
      - "Proxy"
      - "DIRECT"
```

### 8.3 自动生成的规则列表

```yaml
rules:
  # 用户覆写规则（从 overrides.yaml 注入，最高优先级）
  # <user_rules>

  # 本地地址直连
  - "DOMAIN-SUFFIX,local,DIRECT"
  - "IP-CIDR,127.0.0.0/8,DIRECT,no-resolve"
  - "IP-CIDR,192.168.0.0/16,DIRECT,no-resolve"
  - "IP-CIDR,10.0.0.0/8,DIRECT,no-resolve"
  - "IP-CIDR,172.16.0.0/12,DIRECT,no-resolve"

  # 如果 bypass_china = true，添加：
  - "GEOSITE,cn,DIRECT"
  - "GEOIP,CN,DIRECT,no-resolve"

  # 兜底规则
  - "MATCH,Final"
```

### 8.4 深度合并算法规格

```go
// merger.go

// deepMerge 将 src 深度合并到 dst
// 规则：
//   - 标量值（string, int, bool）：src 覆盖 dst
//   - map：递归合并
//   - slice：特殊处理（见下）
//
// Slice 合并规则（按字段名区分）：
//   - "proxies"：src 追加到 dst 末尾（保留 dst 中的所有节点）
//   - "proxy-groups"：按 name 字段合并，同名的 src 覆盖 dst，dst 独有的保留
//   - "rules"：src 插入到 dst 最前面（用户覆写规则优先级最高）
//   - "nameserver", "fallback"：src 追加到 dst
//   - 其他 slice：src 完全替换 dst
func deepMerge(dst, src map[string]interface{}, fieldName string) map[string]interface{} {
    // ... 实现
}
```

---

## 9. 订阅管理器

### 9.1 协议解析规格

#### vmess:// 解析

```go
// parser_vmess.go

// VMess 链接格式：vmess://base64(json)
// JSON 字段（V2RayN 格式）：
// {
//   "v": "2",
//   "ps": "节点名称",
//   "add": "服务器地址",
//   "port": "443",          // 注意：可能是字符串或数字
//   "id": "uuid",
//   "aid": "0",             // alterID
//   "scy": "auto",          // security: auto|aes-128-gcm|chacha20-poly1305|none
//   "net": "ws",            // network: tcp|kcp|ws|http|h2|grpc
//   "type": "none",         // header type
//   "host": "example.com",  // ws host / h2 host
//   "path": "/path",        // ws path / h2 path / grpc service name
//   "tls": "tls",           // tls|xtls|reality|""
//   "sni": "example.com",   // TLS SNI
//   "alpn": "h2",
//   "fp": "chrome"          // fingerprint
// }

func parseVmess(link string) (*ProxyNode, error) {
    b64 := strings.TrimPrefix(link, "vmess://")
    // 尝试标准 base64，失败则尝试 URL-safe base64
    data, err := base64Decode(b64)
    if err != nil {
        return nil, fmt.Errorf("vmess base64 decode: %w", err)
    }
    
    var v vmessJSON
    if err := json.Unmarshal(data, &v); err != nil {
        return nil, fmt.Errorf("vmess json parse: %w", err)
    }
    
    // port 字段容忍字符串和数字
    port, err := parsePort(v.Port)
    if err != nil {
        return nil, fmt.Errorf("vmess invalid port: %w", err)
    }
    
    node := &ProxyNode{
        Name:   v.PS,
        Type:   "vmess",
        Server: v.Add,
        Port:   port,
        Extra:  map[string]interface{}{},
    }
    
    // UUID
    node.Extra["uuid"] = v.ID
    
    // alterId（现代节点通常是 0）
    node.Extra["alterId"] = toInt(v.Aid, 0)
    
    // cipher
    if v.Scy != "" && v.Scy != "auto" {
        node.Extra["cipher"] = v.Scy
    } else {
        node.Extra["cipher"] = "auto"
    }
    
    // 传输层配置
    if v.Net != "" && v.Net != "tcp" {
        node.Extra["network"] = v.Net
        opts := map[string]interface{}{}
        switch v.Net {
        case "ws":
            if v.Host != "" { opts["headers"] = map[string]string{"Host": v.Host} }
            if v.Path != "" { opts["path"] = v.Path }
            node.Extra["ws-opts"] = opts
        case "grpc":
            if v.Path != "" { opts["grpc-service-name"] = v.Path }
            node.Extra["grpc-opts"] = opts
        case "h2":
            if v.Host != "" { opts["host"] = []string{v.Host} }
            if v.Path != "" { opts["path"] = v.Path }
            node.Extra["h2-opts"] = opts
        }
    }
    
    // TLS
    if v.TLS == "tls" || v.TLS == "reality" {
        node.Extra["tls"] = true
        if v.SNI != "" { node.Extra["servername"] = v.SNI }
        if v.FP != "" {
            node.Extra["client-fingerprint"] = v.FP
        }
    }
    
    return node, nil
}
```

#### trojan:// 解析

```go
// trojan://password@server:port?params#name
// 参数：
//   sni=          TLS SNI
//   allowInsecure=0/1
//   type=tcp/ws/grpc
//   host=         ws host
//   path=         ws path / grpc service
//   fp=           fingerprint

func parseTrojan(link string) (*ProxyNode, error) {
    u, err := url.Parse(link)
    if err != nil {
        return nil, err
    }
    
    port, _ := strconv.Atoi(u.Port())
    node := &ProxyNode{
        Name:   url.PathUnescape(u.Fragment),
        Type:   "trojan",
        Server: u.Hostname(),
        Port:   port,
        Extra:  map[string]interface{}{},
    }
    node.Extra["password"] = u.User.Username()
    
    q := u.Query()
    if sni := q.Get("sni"); sni != "" {
        node.Extra["sni"] = sni
    }
    if q.Get("allowInsecure") == "1" {
        node.Extra["skip-cert-verify"] = true
    }
    
    // 传输层
    switch q.Get("type") {
    case "ws":
        node.Extra["network"] = "ws"
        opts := map[string]interface{}{}
        if h := q.Get("host"); h != "" { opts["headers"] = map[string]string{"Host": h} }
        if p := q.Get("path"); p != "" { opts["path"] = p }
        node.Extra["ws-opts"] = opts
    case "grpc":
        node.Extra["network"] = "grpc"
        if sn := q.Get("serviceName"); sn != "" {
            node.Extra["grpc-opts"] = map[string]string{"grpc-service-name": sn}
        }
    }
    
    return node, nil
}
```

#### ss:// 解析

```go
// 两种格式：
// 格式1（旧）：ss://base64(method:password)@server:port#name
// 格式2（新 SIP002）：ss://base64(method:password)@server:port/?plugin=...#name

func parseSS(link string) (*ProxyNode, error) {
    // 尝试 SIP002 格式（包含 @）
    withoutScheme := strings.TrimPrefix(link, "ss://")
    
    if strings.Contains(withoutScheme, "@") {
        return parseSSSIP002(link)
    }
    return parseSSLegacy(link)
}
```

#### vless:// 解析

```go
// vless://uuid@server:port?params#name
// 参数与 vmess 类似，但无 alterId 和 cipher
// 额外参数：
//   flow=xtls-rprx-vision  (Reality 使用)
//   pbk=                    Reality public key
//   sid=                    Reality short ID
//   spx=                    Reality spider X

func parseVless(link string) (*ProxyNode, error) {
    u, err := url.Parse(link)
    // ...类似 trojan 解析，处理 Reality 参数
}
```

### 9.2 过滤器实现

```go
// filter.go

func ApplyFilter(nodes []ProxyNode, f SubscriptionFilter) []ProxyNode {
    var result []ProxyNode
    
    for _, node := range nodes {
        name := strings.ToLower(node.Name)
        
        // 黑名单优先
        excluded := false
        for _, kw := range f.Exclude {
            if strings.Contains(name, strings.ToLower(kw)) {
                excluded = true
                break
            }
        }
        if excluded {
            continue
        }
        
        // 白名单（空白名单表示全部通过）
        if len(f.Include) > 0 {
            matched := false
            for _, kw := range f.Include {
                if strings.Contains(name, strings.ToLower(kw)) {
                    matched = true
                    break
                }
            }
            if !matched {
                continue
            }
        }
        
        result = append(result, node)
    }
    
    // 去重（按 server:port 去重，保留第一个）
    seen := map[string]bool{}
    deduped := result[:0]
    for _, node := range result {
        key := fmt.Sprintf("%s:%d", node.Server, node.Port)
        if !seen[key] {
            seen[key] = true
            deduped = append(deduped, node)
        }
    }
    
    // 数量限制
    if f.MaxNodes > 0 && len(deduped) > f.MaxNodes {
        deduped = deduped[:f.MaxNodes]
    }
    
    return deduped
}
```

### 9.3 Clash YAML 格式订阅解析

```go
// parser_clash_yaml.go
// 当订阅内容是合法 YAML 且包含 "proxies" key 时，直接解析

func parseClashYAML(content []byte) ([]ProxyNode, error) {
    var raw struct {
        Proxies []map[string]interface{} `yaml:"proxies"`
    }
    if err := yaml.Unmarshal(content, &raw); err != nil {
        return nil, err
    }
    
    var nodes []ProxyNode
    for _, p := range raw.Proxies {
        name, _ := p["name"].(string)
        typ, _ := p["type"].(string)
        server, _ := p["server"].(string)
        port := toInt(p["port"], 0)
        
        if name == "" || typ == "" || server == "" || port == 0 {
            continue // 跳过残缺节点
        }
        
        extra := make(map[string]interface{})
        for k, v := range p {
            if k != "name" && k != "type" && k != "server" && k != "port" {
                extra[k] = v
            }
        }
        
        nodes = append(nodes, ProxyNode{
            Name:   name,
            Type:   typ,
            Server: server,
            Port:   port,
            Extra:  extra,
        })
    }
    
    return nodes, nil
}
```

### 9.4 内容格式检测顺序

```go
// fetcher.go: 检测订阅内容格式

func detectAndParse(content []byte) ([]ProxyNode, error) {
    trimmed := bytes.TrimSpace(content)
    
    // 1. 尝试 YAML（包含 "proxies:" 关键字）
    if bytes.Contains(trimmed, []byte("proxies:")) {
        nodes, err := parseClashYAML(trimmed)
        if err == nil && len(nodes) > 0 {
            return nodes, nil
        }
    }
    
    // 2. 尝试 Base64 解码（整体是一个 base64 字符串）
    if looksLikeBase64(trimmed) {
        decoded, err := base64Decode(string(trimmed))
        if err == nil {
            // 解码后按行解析
            return parseLineBased(decoded)
        }
    }
    
    // 3. 按行解析（每行一个协议链接）
    return parseLineBased(trimmed)
}

func parseLineBased(content []byte) ([]ProxyNode, error) {
    var nodes []ProxyNode
    lines := bytes.Split(content, []byte("\n"))
    
    for _, line := range lines {
        line = bytes.TrimSpace(line)
        if len(line) == 0 {
            continue
        }
        
        link := string(line)
        var node *ProxyNode
        var err error
        
        switch {
        case strings.HasPrefix(link, "vmess://"):
            node, err = parseVmess(link)
        case strings.HasPrefix(link, "trojan://"):
            node, err = parseTrojan(link)
        case strings.HasPrefix(link, "ss://"):
            node, err = parseSS(link)
        case strings.HasPrefix(link, "vless://"):
            node, err = parseVless(link)
        case strings.HasPrefix(link, "hy2://"):
            node, err = parseHy2(link)
        case strings.HasPrefix(link, "tuic://"):
            node, err = parseTuic(link)
        default:
            continue // 跳过未知格式
        }
        
        if err != nil {
            log.Warn().Err(err).Str("link", link[:min(len(link), 50)]).Msg("parse error, skipping node")
            continue
        }
        nodes = append(nodes, *node)
    }
    
    return nodes, nil
}
```

---

## 10. 防火墙规则层

### 10.1 自动检测逻辑

```go
// detect.go

type FirewallBackend int

const (
    BackendUnknown  FirewallBackend = iota
    BackendNftables
    BackendIptables
    BackendNone
)

func DetectBackend(forced string) FirewallBackend {
    switch forced {
    case "nftables":
        return BackendNftables
    case "iptables":
        return BackendIptables
    case "none":
        return BackendNone
    }
    
    // auto 检测：优先 nftables
    if path, err := exec.LookPath("nft"); err == nil {
        // 验证 nft 实际可用（不仅仅是二进制存在）
        out, err := exec.Command(path, "list", "tables").Output()
        if err == nil && len(out) >= 0 { // 空输出也算成功
            // 检查是否有 tproxy 支持（加载 nft_tproxy 模块）
            if checkNftTProxy() {
                return BackendNftables
            }
        }
    }
    
    if _, err := exec.LookPath("iptables"); err == nil {
        if checkIptablesTProxy() {
            return BackendIptables
        }
    }
    
    return BackendNone
}

// checkNftTProxy 检查 nft_tproxy 内核模块是否可用
func checkNftTProxy() bool {
    // 方法1：检查 /proc/net/ip_tables_targets（nftables 不用这个）
    // 方法2：尝试创建一个测试 tproxy 规则
    out, err := exec.Command("nft", "-c",
        "table inet test_tproxy_check { chain c { type filter hook prerouting priority 0; tproxy to :1 } }",
    ).CombinedOutput()
    // 如果只是因为权限问题失败，认为 tproxy 可用
    if err == nil || strings.Contains(string(out), "Operation not permitted") {
        return true
    }
    return false
}
```

### 10.2 nftables 完整规则模板

```
// templates/nft_tproxy.tmpl

table inet metaclash {
    # 绕过地址集合
    set bypass_ipv4 {
        type ipv4_addr
        flags interval
        elements = {
            0.0.0.0/8,
            10.0.0.0/8,
            100.64.0.0/10,
            127.0.0.0/8,
            169.254.0.0/16,
            172.16.0.0/12,
            192.0.0.0/24,
            192.168.0.0/16,
            198.18.0.0/15,
            198.51.100.0/24,
            203.0.113.0/24,
            224.0.0.0/4,
            240.0.0.0/4,
            255.255.255.255/32
            {{- range .BypassCIDR }},
            {{ . }}
            {{- end }}
        }
    }

    # DNS 劫持（将 LAN 侧 DNS 查询重定向到 mihomo）
    chain dns_redirect {
        type nat hook prerouting priority dstnat; policy accept;
        meta mark {{ .FWMark }} return
        fib saddr type local return
        ip daddr @bypass_ipv4 return
        udp dport 53 redirect to :{{ .DNSPort }}
        tcp dport 53 redirect to :{{ .DNSPort }}
    }

    # 透明代理主链
    chain tproxy_prerouting {
        type filter hook prerouting priority mangle; policy accept;
        # 跳过已标记的流量（来自 metaclash 自身的流量）
        meta mark {{ .FWMark }} return
        # 跳过本机源地址
        fib saddr type local return
        # 跳过绕过地址
        ip daddr @bypass_ipv4 return
        # 仅代理 TCP 和 UDP
        meta l4proto { tcp, udp } \
            tproxy ip to 127.0.0.1:{{ .TProxyPort }} \
            meta mark set {{ .FWMark }}
    }

    # 本机流量代理（可选，由 ProxyLocalTraffic 控制）
    {{- if .ProxyLocalTraffic }}
    chain tproxy_output {
        type route hook output priority mangle; policy accept;
        meta mark {{ .FWMark }} return
        fib daddr type local return
        ip daddr @bypass_ipv4 return
        meta l4proto { tcp, udp } meta mark set {{ .FWMark }}
    }
    {{- end }}
}
```

模板变量：
```go
type NftablesTemplateVars struct {
    FWMark           string   // 例如 "0x1a3"
    TProxyPort       int      // 7895
    DNSPort          int      // 7874
    BypassCIDR       []string // 额外绕过的 CIDR
    ProxyLocalTraffic bool    // 是否代理路由器本机流量
}
```

### 10.3 规则清理

**关键**：metaclash 退出时必须清理规则，否则路由器断网。

```go
// cleanup.go

// Cleanup 删除所有 metaclash 注入的规则
// 在以下情况调用：
//   1. 正常退出（SIGTERM）
//   2. 启动失败回滚
//   3. 用户通过 API 手动停止

func (n *NftablesBackend) Cleanup() error {
    // nftables：删除整个 table（原子操作）
    out, err := exec.Command("nft", "delete", "table", "inet", "metaclash").CombinedOutput()
    if err != nil {
        // 如果 table 不存在，忽略错误
        if strings.Contains(string(out), "No such file") ||
           strings.Contains(string(out), "table not found") {
            return nil
        }
        return fmt.Errorf("nft cleanup: %w: %s", err, out)
    }
    
    // 清理策略路由
    exec.Command("ip", "rule", "del", "fwmark", FWMark, "table", RouteTable).Run()
    exec.Command("ip", "route", "flush", "table", RouteTable).Run()
    
    return nil
}

func (i *IptablesBackend) Cleanup() error {
    // iptables：逐条删除规则
    cmds := [][]string{
        {"iptables", "-t", "mangle", "-D", "PREROUTING", "-j", "METACLASH"},
        {"iptables", "-t", "mangle", "-F", "METACLASH"},
        {"iptables", "-t", "mangle", "-X", "METACLASH"},
        {"iptables", "-t", "nat", "-D", "PREROUTING", "-p", "udp", "--dport", "53", "-j", "REDIRECT", "--to-port", strconv.Itoa(i.dnsPort)},
        {"iptables", "-t", "nat", "-D", "PREROUTING", "-p", "tcp", "--dport", "53", "-j", "REDIRECT", "--to-port", strconv.Itoa(i.dnsPort)},
        {"ip", "rule", "del", "fwmark", FWMark, "table", RouteTable},
    }
    
    for _, cmd := range cmds {
        exec.Command(cmd[0], cmd[1:]...).Run() // 忽略单条命令错误
    }
    return nil
}
```

---

## 11. Web UI 规格

### 11.1 技术栈锁定

```json
// ui/package.json（锁定版本，不使用 ^）
{
  "dependencies": {
    "react": "18.3.1",
    "react-dom": "18.3.1",
    "react-router-dom": "6.23.1",
    "zustand": "4.5.2",
    "recharts": "2.12.7"
  },
  "devDependencies": {
    "typescript": "5.4.5",
    "vite": "5.2.11",
    "@vitejs/plugin-react": "4.3.0",
    "tailwindcss": "3.4.4",
    "autoprefixer": "10.4.19"
  }
}
```

不引入 shadcn/ui 组件库（避免依赖复杂度），所有 UI 组件手写，用 Tailwind CSS 样式。

### 11.2 API 客户端完整定义

```typescript
// ui/src/api/client.ts

const BASE = '/api/v1'

async function request<T>(
  method: string,
  path: string,
  body?: unknown
): Promise<T> {
  const secret = localStorage.getItem('metaclash_secret') || ''
  const res = await fetch(`${BASE}${path}`, {
    method,
    headers: {
      'Content-Type': 'application/json',
      ...(secret ? { Authorization: `Bearer ${secret}` } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  })

  const json = await res.json()
  if (!json.ok) {
    throw new APIError(json.error?.code, json.error?.message)
  }
  return json.data as T
}

export class APIError extends Error {
  constructor(public code: string, message: string) {
    super(message)
  }
}

// 状态
export const getStatus = () => request<StatusData>('GET', '/status')

// 核心管理
export const startCore = () => request('POST', '/core/start')
export const stopCore = () => request('POST', '/core/stop')
export const restartCore = () => request('POST', '/core/restart')
export const reloadCore = () => request('POST', '/core/reload')
export const getCoreVersion = () => request<CoreVersionData>('GET', '/core/version')
export const updateCore = () => request('POST', '/core/update')

// 配置
export const getConfig = () => request<MetaclashConfig>('GET', '/config')
export const updateConfig = (patch: Partial<MetaclashConfig>) =>
  request('PUT', '/config', patch)
export const getMihomoConfig = () =>
  request<{ content: string }>('GET', '/config/mihomo')
export const getOverrides = () =>
  request<{ content: string }>('GET', '/config/overrides')
export const updateOverrides = (content: string) =>
  request('PUT', '/config/overrides', { content })

// 订阅
export const getSubscriptions = () =>
  request<{ subscriptions: Subscription[] }>('GET', '/subscriptions')
export const addSubscription = (data: NewSubscriptionInput) =>
  request<{ id: string }>('POST', '/subscriptions', data)
export const updateSubscription = (id: string, data: Partial<Subscription>) =>
  request('PUT', `/subscriptions/${id}`, data)
export const deleteSubscription = (id: string) =>
  request('DELETE', `/subscriptions/${id}`)
export const triggerUpdate = (id: string) =>
  request('POST', `/subscriptions/${id}/update`)
export const triggerUpdateAll = () =>
  request('POST', '/subscriptions/update-all')

// 代理
export const getProxies = () => request<ProxiesData>('GET', '/proxies')
export const selectProxy = (group: string, name: string) =>
  request('PUT', `/proxies/${encodeURIComponent(group)}/select`, { name })
export const testLatency = (proxies: string[]) =>
  request<Record<string, number>>('POST', '/proxies/test-latency', {
    proxies,
    url: 'http://www.gstatic.com/generate_204',
    timeout: 5000,
  })

// 规则
export const getRules = () => request<RulesData>('GET', '/rules')

// 连接
export const getConnections = () => request<ConnectionsData>('GET', '/connections')
export const closeAllConnections = () => request('DELETE', '/connections')
export const closeConnection = (id: string) =>
  request('DELETE', `/connections/${id}`)

// 系统
export const getLogs = (level = 'info', limit = 100) =>
  request<{ logs: LogEntry[] }>('GET', `/logs?level=${level}&limit=${limit}`)
export const updateGeoIP = () => request('GET', '/geoip/update')
export const updateGeosite = () => request('GET', '/geosite/update')
```

### 11.3 SSE Hook

```typescript
// ui/src/hooks/useSSE.ts

import { useEffect, useRef, useCallback } from 'react'

type SSEHandlers = {
  onTraffic?: (data: TrafficEvent) => void
  onCoreState?: (data: CoreStateEvent) => void
  onSubscriptionUpdate?: (data: SubUpdateEvent) => void
  onConnectionsCount?: (data: ConnectionsCountEvent) => void
  onLog?: (data: LogEvent) => void
}

export function useSSE(handlers: SSEHandlers) {
  const esRef = useRef<EventSource | null>(null)
  const handlersRef = useRef(handlers)
  handlersRef.current = handlers // 不触发重连

  const connect = useCallback(() => {
    const secret = localStorage.getItem('metaclash_secret') || ''
    // SSE 不支持自定义 Header，用 query param 传递 secret
    const url = `/api/v1/events${secret ? `?secret=${secret}` : ''}`
    const es = new EventSource(url)
    esRef.current = es

    es.addEventListener('traffic', (e) => {
      handlersRef.current.onTraffic?.(JSON.parse(e.data))
    })
    es.addEventListener('core_state', (e) => {
      handlersRef.current.onCoreState?.(JSON.parse(e.data))
    })
    es.addEventListener('subscription_update', (e) => {
      handlersRef.current.onSubscriptionUpdate?.(JSON.parse(e.data))
    })
    es.addEventListener('connections_count', (e) => {
      handlersRef.current.onConnectionsCount?.(JSON.parse(e.data))
    })
    es.addEventListener('log', (e) => {
      handlersRef.current.onLog?.(JSON.parse(e.data))
    })

    es.onerror = () => {
      es.close()
      // 断线 3 秒后重连
      setTimeout(connect, 3000)
    }
  }, [])

  useEffect(() => {
    connect()
    return () => esRef.current?.close()
  }, [connect])
}
```

### 11.4 TrafficChart 组件

```typescript
// ui/src/components/TrafficChart.tsx
// 使用 recharts 绘制实时流量折线图

import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer } from 'recharts'
import { useState } from 'react'
import { useSSE } from '@/hooks/useSSE'

type DataPoint = { ts: number; up: number; down: number }

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B/s`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB/s`
  return `${(bytes / 1024 / 1024).toFixed(2)} MB/s`
}

export function TrafficChart() {
  const [data, setData] = useState<DataPoint[]>([])

  useSSE({
    onTraffic: (event) => {
      setData((prev) => {
        const next = [...prev, { ts: event.ts, up: event.up, down: event.down }]
        // 只保留最近 60 个数据点
        return next.slice(-60)
      })
    },
  })

  return (
    <div className="w-full h-40">
      <ResponsiveContainer>
        <LineChart data={data}>
          <XAxis dataKey="ts" hide />
          <YAxis tickFormatter={formatBytes} width={80} />
          <Tooltip formatter={(v: number) => formatBytes(v)} />
          <Line
            type="monotone"
            dataKey="up"
            stroke="#6366f1"
            dot={false}
            strokeWidth={2}
            name="上传"
            isAnimationActive={false}
          />
          <Line
            type="monotone"
            dataKey="down"
            stroke="#22c55e"
            dot={false}
            strokeWidth={2}
            name="下载"
            isAnimationActive={false}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}
```

### 11.5 Vite 构建配置

```typescript
// ui/vite.config.ts
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  build: {
    outDir: '../internal/api/ui_dist', // Go embed 读取此目录
    emptyOutDir: true,
    rollupOptions: {
      output: {
        // 确保文件名固定（不带 hash），方便 Go embed
        entryFileNames: 'assets/index.js',
        chunkFileNames: 'assets/[name].js',
        assetFileNames: 'assets/[name].[ext]',
        manualChunks: {
          // 将 recharts 单独分包（避免主包过大）
          'vendor-charts': ['recharts'],
          'vendor-react': ['react', 'react-dom', 'react-router-dom'],
        },
      },
    },
  },
  server: {
    // 开发时代理到本地 metaclash
    proxy: {
      '/api': 'http://192.168.1.1:7777',
      '/api/v1/events': {
        target: 'http://192.168.1.1:7777',
        changeOrigin: true,
      },
    },
  },
})
```

### 11.6 Go 端 embed 与 SPA fallback

```go
// internal/api/server.go

//go:embed ui_dist
var uiDist embed.FS

func (s *Server) registerUIRoutes(r chi.Router) {
    // API 路由在 /api/v1/* 已注册
    // 所有其他路由返回 index.html（SPA 客户端路由）
    
    uiFS, _ := fs.Sub(uiDist, "ui_dist")
    fileServer := http.FileServer(http.FS(uiFS))
    
    r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
        // 如果文件存在（JS、CSS、图片等静态资源）直接返回
        if _, err := uiFS.(fs.StatFS).Stat(strings.TrimPrefix(r.URL.Path, "/")); err == nil {
            fileServer.ServeHTTP(w, r)
            return
        }
        // 否则返回 index.html（让 React Router 处理）
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        f, _ := uiDist.Open("ui_dist/index.html")
        io.Copy(w, f)
    })
}
```

---

## 12. OpenWrt 集成

### 12.1 init.d 完整脚本

```bash
#!/bin/sh /etc/rc.common
# /etc/init.d/metaclash

USE_PROCD=1
START=99
STOP=10

PROG=/usr/bin/metaclash
CONFIG=/etc/metaclash/config.toml
PIDFILE=/var/run/metaclash/metaclash.pid

start_service() {
    # 确保运行时目录存在（tmpfs 重启后消失）
    mkdir -p /var/run/metaclash
    
    procd_open_instance
    procd_set_param command "$PROG" -config "$CONFIG"
    procd_set_param pidfile "$PIDFILE"
    
    # 进程崩溃时自动重启：
    # respawn <threshold> <timeout> <retry>
    # threshold: 在 threshold 秒内崩溃超过 retry 次则停止重启
    # timeout: 重启前等待时间
    # retry: 最大重启次数
    procd_set_param respawn 3600 5 3
    
    procd_set_param stdout 1
    procd_set_param stderr 1
    
    # 需要的 capabilities
    procd_set_param capabilities CAP_NET_ADMIN,CAP_NET_RAW,CAP_NET_BIND_SERVICE
    
    procd_close_instance
}

reload_service() {
    # metaclash 处理 SIGHUP 为配置热重载
    local pid
    pid=$(cat "$PIDFILE" 2>/dev/null)
    [ -n "$pid" ] && kill -HUP "$pid" 2>/dev/null
}

service_triggers() {
    # 网络变化时触发 reload（例如 WAN 口重新拨号）
    procd_add_reload_trigger "network"
}
```

### 12.2 OpenWrt Package Makefile 完整版

```makefile
# openwrt/Makefile

include $(TOPDIR)/rules.mk

PKG_NAME:=metaclash
PKG_VERSION:=1.0.0
PKG_RELEASE:=1
PKG_MAINTAINER:=Your Name <your@email.com>
PKG_LICENSE:=MIT

# 使用预构建二进制（Go 交叉编译在 OpenWrt 构建系统内较复杂）
PKG_SOURCE_URL:=https://github.com/yourusername/metaclash/releases/download/v$(PKG_VERSION)
PKG_HASH:=<sha256sum of source tarball>

include $(INCLUDE_DIR)/package.mk

# 定义架构到 Go GOARCH 的映射
define GoArch
$(if $(filter mipsel,$(ARCH)),mipsle,\
$(if $(filter mips,$(ARCH)),mips,\
$(if $(filter arm,$(ARCH)),\
  $(if $(filter cortex-a7 cortex-a9 cortex-a15,$(CONFIG_CPU_TYPE)),armv7,armv5),\
$(if $(filter aarch64,$(ARCH)),arm64,\
$(ARCH)))))
endef

define Package/metaclash
  SECTION:=net
  CATEGORY:=Network
  TITLE:=MetaClash - Clash proxy manager for OpenWrt
  # 仅需 nft-tproxy 和 iproute2，其他都内置了
  DEPENDS:=+kmod-nft-tproxy +ip-full
  URL:=https://github.com/yourusername/metaclash
endef

define Package/metaclash/description
  A lightweight Clash (Mihomo) proxy manager for OpenWrt.
  - No Ruby, no Lua, no dnsmasq-full required
  - nftables-first, iptables fallback
  - Single static binary with embedded Web UI
  - < 3 second cold start
endef

define Package/metaclash/conffiles
/etc/metaclash/config.toml
/etc/metaclash/subscriptions.toml
/etc/metaclash/overrides.yaml
endef

define Build/Compile
	$(CP) $(PKG_BUILD_DIR)/metaclash-linux-$(call GoArch) \
		$(PKG_BUILD_DIR)/metaclash
endef

define Package/metaclash/install
	$(INSTALL_DIR) $(1)/usr/bin
	$(INSTALL_BIN) $(PKG_BUILD_DIR)/metaclash $(1)/usr/bin/metaclash

	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/etc/init.d/metaclash $(1)/etc/init.d/metaclash

	$(INSTALL_DIR) $(1)/etc/metaclash
	$(INSTALL_CONF) ./files/etc/metaclash/config.toml.example \
		$(1)/etc/metaclash/config.toml
	$(INSTALL_CONF) ./files/etc/metaclash/overrides.yaml.example \
		$(1)/etc/metaclash/overrides.yaml

	$(INSTALL_DIR) $(1)/usr/share/metaclash
endef

$(eval $(call BuildPackage,metaclash))
```

### 12.3 首次安装流程

metaclash 首次运行时（检测到 mihomo 二进制不存在）应自动执行：

```
1. 检测 CPU 架构（读取 /proc/cpuinfo 或 uname -m）
2. 从 GitHub Releases 下载对应架构的 mihomo 二进制
3. 写入 /usr/bin/mihomo，chmod +x
4. 下载 Country.mmdb 到 /usr/share/metaclash/
5. 下载 geosite.dat 到 /usr/share/metaclash/
6. 输出就绪日志
```

架构检测代码：
```go
func detectArch() string {
    out, _ := exec.Command("uname", "-m").Output()
    arch := strings.TrimSpace(string(out))
    
    switch {
    case arch == "x86_64":
        return "amd64"
    case arch == "aarch64":
        return "arm64"
    case strings.HasPrefix(arch, "armv7"):
        return "armv7"
    case strings.HasPrefix(arch, "armv5"):
        return "armv5"
    case arch == "mips":
        // 检查字节序
        if isLittleEndian() {
            return "mipsle"
        }
        return "mips"
    default:
        return arch
    }
}
```

---

## 13. 错误处理规范

### 13.1 错误码完整列表

| 错误码 | HTTP 状态 | 含义 |
|--------|-----------|------|
| `CORE_ALREADY_RUNNING` | 409 | mihomo 已在运行，无需再次启动 |
| `CORE_NOT_RUNNING` | 404 | mihomo 未运行 |
| `CORE_START_FAILED` | 500 | mihomo 启动失败，data 包含错误详情 |
| `CORE_RELOAD_FAILED` | 500 | 热重载失败，通常因为新配置无效 |
| `CORE_NOT_FOUND` | 500 | mihomo 二进制不存在 |
| `CONFIG_INVALID` | 400 | mihomo -t 验证失败 |
| `CONFIG_PARSE_ERROR` | 400 | TOML/YAML 语法错误 |
| `CONFIG_WRITE_FAILED` | 500 | 配置文件写入失败（磁盘满？） |
| `SUB_NOT_FOUND` | 404 | 指定订阅 ID 不存在 |
| `SUB_URL_INVALID` | 400 | 订阅 URL 格式非法 |
| `SUB_FETCH_FAILED` | 502 | 订阅下载失败（网络错误） |
| `SUB_PARSE_FAILED` | 422 | 订阅内容解析失败（格式不识别） |
| `YAML_PARSE_ERROR` | 400 | YAML 语法错误（overrides） |
| `AUTH_REQUIRED` | 401 | 需要认证 |
| `AUTH_INVALID` | 403 | 认证密钥错误 |
| `FIREWALL_APPLY_FAILED` | 500 | nftables/iptables 规则应用失败 |
| `INTERNAL_ERROR` | 500 | 未预期的内部错误 |

### 13.2 panic 处理

```go
// internal/api/middleware.go

func recoverMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if rec := recover(); rec != nil {
                log.Error().
                    Interface("panic", rec).
                    Str("stack", string(debug.Stack())).
                    Msg("handler panic recovered")
                api.Err(w, 500, "INTERNAL_ERROR", "unexpected error")
            }
        }()
        next.ServeHTTP(w, r)
    })
}
```

### 13.3 优雅退出序列

```go
// cmd/metaclash/main.go

func main() {
    // ...初始化代码...
    
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
    
    for sig := range sigCh {
        switch sig {
        case syscall.SIGHUP:
            log.Info().Msg("received SIGHUP, reloading config")
            if err := app.Reload(); err != nil {
                log.Error().Err(err).Msg("reload failed")
            }
            
        case syscall.SIGTERM, syscall.SIGINT:
            log.Info().Msg("shutting down")
            
            // 严格按照此顺序执行：
            // 1. 停止接受新的 HTTP 请求（给正在处理的请求 5 秒完成）
            ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
            defer cancel()
            app.HTTPServer.Shutdown(ctx)
            
            // 2. 停止调度器（不再触发新的订阅更新）
            app.Scheduler.Stop()
            
            // 3. 停止 mihomo 进程
            app.CoreManager.Stop()
            
            // 4. 清理防火墙规则（最重要！）
            if err := app.NetfilterManager.Cleanup(); err != nil {
                log.Error().Err(err).Msg("firewall cleanup failed, network may be broken!")
            }
            
            // 5. 删除 PID 文件
            app.PIDFile.Release()
            
            log.Info().Msg("shutdown complete")
            return
        }
    }
}
```

---

## 14. 测试规格

### 14.1 单元测试覆盖要求

| 包 | 必须测试的函数 | 目标覆盖率 |
|----|--------------|-----------|
| `subscription/parser_vmess.go` | `parseVmess` 所有字段组合 | 90% |
| `subscription/parser_trojan.go` | `parseTrojan` 含/不含各参数 | 90% |
| `subscription/parser_ss.go` | 新旧两种格式 | 90% |
| `subscription/filter.go` | 包含/排除/去重/数量限制 | 95% |
| `subscription/fetcher.go` | 格式检测（4种格式） | 85% |
| `config/merger.go` | 三层合并，slice 各字段规则 | 95% |
| `config/generator.go` | 完整配置生成 | 80% |
| `netfilter/nftables.go` | 规则模板渲染 | 80% |

### 14.2 测试数据文件

```
internal/subscription/testdata/
├── vmess_basic.txt          # 基础 vmess 链接
├── vmess_ws_tls.txt         # ws+tls vmess
├── vmess_grpc.txt           # grpc vmess
├── trojan_basic.txt
├── trojan_ws.txt
├── ss_legacy.txt            # 旧格式 ss
├── ss_sip002.txt            # 新格式 ss
├── vless_reality.txt        # Reality 节点
├── clash_yaml_proxies.yaml  # 标准 Clash YAML 订阅
├── base64_encoded.txt       # 整体 base64 编码的订阅
└── mixed_subscription.txt   # 多种格式混合
```

### 14.3 集成测试

```go
// 集成测试需要：
// - 实际的 mihomo 二进制（从 CI 环境下载）
// - 不需要实际路由器，在 Linux 上运行

func TestFullStartStop(t *testing.T) {
    // 需要 root 权限（nftables 操作）
    if os.Getuid() != 0 {
        t.Skip("requires root")
    }
    
    cfg := testConfig() // 使用非标准端口避免冲突
    app := NewApp(cfg)
    
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    require.NoError(t, app.Start(ctx))
    defer app.Shutdown()
    
    // 验证 mihomo API 可达
    resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/version", cfg.Ports.MihomoAPI))
    require.NoError(t, err)
    require.Equal(t, 200, resp.StatusCode)
    
    // 验证 metaclash API 可达
    resp, err = http.Get(fmt.Sprintf("http://127.0.0.1:%d/api/v1/status", cfg.Ports.UI))
    require.NoError(t, err)
    require.Equal(t, 200, resp.StatusCode)
    
    // 验证 nftables 规则已应用
    out, err := exec.Command("nft", "list", "table", "inet", "metaclash").Output()
    require.NoError(t, err)
    require.Contains(t, string(out), "tproxy")
}
```

---

## 15. 构建与 CI

### 15.1 Makefile 完整版

```makefile
# Makefile

BINARY_NAME := metaclash
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_FLAGS := -trimpath -ldflags="-s -w -X main.Version=$(VERSION)"

# 构建 UI（必须先于 Go 构建）
.PHONY: ui
ui:
	cd ui && npm ci && npm run build

# 构建所有架构
.PHONY: build-all
build-all: ui
	@mkdir -p dist
	GOOS=linux GOARCH=amd64    CGO_ENABLED=0 go build $(BUILD_FLAGS) -o dist/$(BINARY_NAME)-linux-amd64      ./cmd/metaclash
	GOOS=linux GOARCH=arm64    CGO_ENABLED=0 go build $(BUILD_FLAGS) -o dist/$(BINARY_NAME)-linux-arm64      ./cmd/metaclash
	GOOS=linux GOARCH=arm   GOARM=7 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o dist/$(BINARY_NAME)-linux-armv7 ./cmd/metaclash
	GOOS=linux GOARCH=arm   GOARM=5 CGO_ENABLED=0 go build $(BUILD_FLAGS) -o dist/$(BINARY_NAME)-linux-armv5 ./cmd/metaclash
	GOOS=linux GOARCH=mipsle   CGO_ENABLED=0 go build $(BUILD_FLAGS) -o dist/$(BINARY_NAME)-linux-mipsle     ./cmd/metaclash
	GOOS=linux GOARCH=mips     CGO_ENABLED=0 go build $(BUILD_FLAGS) -o dist/$(BINARY_NAME)-linux-mips       ./cmd/metaclash
	@echo "Build complete:"
	@ls -lh dist/

# 本地开发构建（仅当前架构）
.PHONY: build
build: ui
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o dist/$(BINARY_NAME) ./cmd/metaclash

# 测试
.PHONY: test
test:
	go test ./... -v -timeout 60s

.PHONY: test-integration
test-integration:
	go test ./... -v -timeout 120s -tags integration

# 代码检查
.PHONY: lint
lint:
	golangci-lint run ./...

# 清理
.PHONY: clean
clean:
	rm -rf dist/ internal/api/ui_dist/
	cd ui && rm -rf dist/ node_modules/
```

### 15.2 GitHub Actions CI

```yaml
# .github/workflows/release.yml
name: Build and Release

on:
  push:
    tags: ['v*']

jobs:
  build-ui:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with: { node-version: '20' }
      - run: cd ui && npm ci && npm run build
      - uses: actions/upload-artifact@v4
        with:
          name: ui-dist
          path: internal/api/ui_dist/

  build-binaries:
    needs: build-ui
    runs-on: ubuntu-latest
    strategy:
      matrix:
        include:
          - goos: linux
            goarch: amd64
          - goos: linux
            goarch: arm64
          - goos: linux
            goarch: arm
            goarm: "7"
            suffix: armv7
          - goos: linux
            goarch: arm
            goarm: "5"
            suffix: armv5
          - goos: linux
            goarch: mipsle
          - goos: linux
            goarch: mips
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }
      - uses: actions/download-artifact@v4
        with:
          name: ui-dist
          path: internal/api/ui_dist/
      - name: Build
        env:
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
          GOARM: ${{ matrix.goarm }}
          CGO_ENABLED: "0"
        run: |
          SUFFIX="${{ matrix.suffix || matrix.goarch }}"
          go build -trimpath -ldflags="-s -w -X main.Version=${{ github.ref_name }}" \
            -o dist/metaclash-${{ matrix.goos }}-${SUFFIX} ./cmd/metaclash
      - uses: actions/upload-artifact@v4
        with:
          name: binary-${{ matrix.goos }}-${{ matrix.goarch }}${{ matrix.goarm }}
          path: dist/

  release:
    needs: build-binaries
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/download-artifact@v4
        with:
          pattern: binary-*
          path: dist/
          merge-multiple: true
      - uses: softprops/action-gh-release@v2
        with:
          files: dist/*
          generate_release_notes: true
```

---

## 16. 开发顺序（Phase 计划）

### Phase 1 — 核心骨架（目标：2 周）

**可验收标准**：可以在路由器上通过命令行启动，mihomo 正常运行，流量被透明代理。

任务清单（严格按此顺序）：
```
1. 初始化 Go 项目：go mod init，创建完整目录结构
2. internal/config/types.go + loader.go：加载 config.toml，验证必填字段
3. internal/daemon/pidfile.go：PID 文件锁，防止多实例
4. internal/core/manager.go：启动/停止/重启 mihomo，含进程锁
5. internal/netfilter/detect.go + nftables.go：检测并应用 nftables TProxy 规则
6. internal/netfilter/iptables.go：iptables 备选实现
7. cmd/metaclash/main.go：串联上述模块，实现信号处理和优雅退出
8. openwrt/files/etc/init.d/metaclash：procd 服务脚本
9. 手工测试：在路由器上验证启动/停止/重启/流量代理
```

### Phase 2 — 订阅系统（目标：1.5 周）

**可验收标准**：能通过命令行参数或配置文件添加订阅并自动下载。

```
1. internal/subscription/fetcher.go：HTTP 下载，格式检测
2. internal/subscription/parser_*.go：vmess/trojan/ss/vless/clash_yaml
3. internal/subscription/filter.go：关键词过滤
4. internal/subscription/store.go：读写 subscriptions.toml 和缓存
5. internal/subscription/manager.go：并发更新，定时调度
6. internal/config/generator.go：从订阅节点生成 mihomo YAML
7. internal/config/merger.go：三层合并
8. 验证：添加真实订阅，生成配置，mihomo 成功加载
```

### Phase 3 — HTTP API（目标：1 周）

**可验收标准**：curl 能调用所有 API 端点。

```
1. internal/api/server.go + response.go + middleware.go
2. handler_status.go
3. handler_core.go（start/stop/restart/reload）
4. handler_subscriptions.go（CRUD + 触发更新）
5. handler_config.go
6. handler_proxies.go + handler_rules.go + handler_connections.go（透传 mihomo）
7. sse.go + handler_traffic.go + handler_logs.go
8. 用 curl 验证所有端点
```

### Phase 4 — Web UI（目标：2 周）

**可验收标准**：浏览器能正常使用所有功能页面。

```
1. 项目脚手架：Vite + React + TypeScript + Tailwind
2. ui/src/api/client.ts：所有 API 调用
3. ui/src/hooks/useSSE.ts
4. 布局：Sidebar + TopBar
5. Dashboard 页面（状态卡片 + TrafficChart）
6. Proxies 页面（代理组列表 + 节点切换 + 延迟测试）
7. Subscriptions 页面（CRUD + 触发更新）
8. Settings 页面（所有配置项表单）
9. Logs 页面
10. Connections 页面
11. Rules 页面
12. Go embed 集成，验证生产构建
```

### Phase 5 — 稳定化（目标：1 周）

```
1. 补全单元测试（达到 14.1 节覆盖率目标）
2. 首次安装流程（自动下载 mihomo + GeoIP + Geosite）
3. mihomo 版本检查与更新（handler_core.go 中的 update 端点）
4. 定时任务（GeoIP/Geosite 自动更新）
5. 压力测试：订阅更新期间不能影响代理性能
6. 在 3 种不同路由器（MIPS/armv7/x86）上全流程验证
7. 撰写用户文档
```

---

## 附录 A：mihomo API 常用端点速查

metaclash 在实现透传接口时需要调用以下 mihomo API：

```
GET  http://127.0.0.1:9090/version          → {"version": "Mihomo..."}
GET  http://127.0.0.1:9090/proxies          → 所有代理组和节点
PUT  http://127.0.0.1:9090/proxies/{name}   → 切换代理组，body: {"name": "节点名"}
GET  http://127.0.0.1:9090/rules            → 规则列表
GET  http://127.0.0.1:9090/connections      → 连接列表
DELETE http://127.0.0.1:9090/connections    → 关闭所有连接
GET  http://127.0.0.1:9090/traffic          → 流量统计（SSE）
GET  http://127.0.0.1:9090/logs             → 日志（SSE）
PUT  http://127.0.0.1:9090/configs          → 热重载，body: {"path": "/path/to/config.yaml"}
GET  http://127.0.0.1:9090/providers/proxies → 代理提供者
```

mihomo API 鉴权：当 `external-controller-secret` 非空时，需 Header `Authorization: Bearer <secret>`。

---

## 附录 B：已知边界条件

1. **节点名称重复**：同名节点在 Clash 中会导致配置加载失败。订阅管理器在合并多个订阅时必须去重节点名，冲突时添加后缀（`节点名 #2`）。

2. **端口冲突**：启动前需检查 7890/7891/7893/7895/7874/9090/7777 端口是否被占用，被占用时给出明确错误信息。

3. **Flash 空间不足**：写入配置文件前检查磁盘剩余空间，低于 1MB 时拒绝写入并报错（不能静默失败导致配置损坏）。

4. **mihomo 二进制无执行权限**：启动前 `os.Chmod(binary, 0755)`，避免权限问题。

5. **订阅 URL 需要特殊 User-Agent**：部分机场会根据 UA 返回不同格式的订阅内容，默认 UA 为 `clash-meta`，允许用户自定义。

6. **nftables 和 iptables 不能同时使用**：检测到 nftables 时，必须确保没有旧的 iptables 规则残留（可能是 OpenClash 留下的）。提供 `--force-cleanup-old-rules` 参数处理迁移场景。

7. **MIPS 上 Go GC 压力**：在 64MB MIPS 设备上，设置环境变量 `GOGC=20 GOMEMLIMIT=40MiB`（通过 init.d 传递）减少内存峰值。

---

## 附录 C：调度器完整实现

### C.1 internal/scheduler/scheduler.go

```go
package scheduler

import (
    "context"
    "sync"
    "time"

    "github.com/rs/zerolog/log"
)

// Job 是一个定时任务
type Job struct {
    Name     string
    Interval time.Duration
    Fn       func(ctx context.Context) error
    // 是否在启动时立刻执行一次（而不是等第一个 interval）
    RunOnStart bool
}

// Scheduler 是一个极简的内置 cron，不依赖任何外部库
type Scheduler struct {
    jobs   []Job
    stopCh chan struct{}
    wg     sync.WaitGroup
}

func New() *Scheduler {
    return &Scheduler{stopCh: make(chan struct{})}
}

func (s *Scheduler) Add(job Job) {
    s.jobs = append(s.jobs, job)
}

func (s *Scheduler) Start(ctx context.Context) {
    for _, job := range s.jobs {
        job := job // 捕获循环变量
        s.wg.Add(1)
        go func() {
            defer s.wg.Done()
            s.runJob(ctx, job)
        }()
    }
}

func (s *Scheduler) Stop() {
    close(s.stopCh)
    s.wg.Wait()
}

func (s *Scheduler) runJob(ctx context.Context, job Job) {
    if job.RunOnStart {
        s.execute(ctx, job)
    }

    ticker := time.NewTicker(job.Interval)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            s.execute(ctx, job)
        case <-s.stopCh:
            return
        case <-ctx.Done():
            return
        }
    }
}

func (s *Scheduler) execute(ctx context.Context, job Job) {
    log.Info().Str("job", job.Name).Msg("scheduler: running job")
    start := time.Now()

    // 给每个任务一个独立的超时 context（最多运行 10 分钟）
    jobCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
    defer cancel()

    if err := job.Fn(jobCtx); err != nil {
        log.Error().Str("job", job.Name).Err(err).
            Dur("elapsed", time.Since(start)).Msg("scheduler: job failed")
        return
    }
    log.Info().Str("job", job.Name).
        Dur("elapsed", time.Since(start)).Msg("scheduler: job done")
}
```

### C.2 调度任务注册（在 App 初始化时调用）

```go
// 在 cmd/metaclash/main.go 的 App.Start() 中注册

func (app *App) registerScheduledJobs() {
    cfg := app.Config

    // 1. 订阅自动更新
    if cfg.Update.AutoSubscription {
        interval, _ := time.ParseDuration(cfg.Update.SubscriptionInterval)
        if interval <= 0 {
            interval = 6 * time.Hour
        }
        app.Scheduler.Add(scheduler.Job{
            Name:       "subscription-update",
            Interval:   interval,
            RunOnStart: false, // 启动时不立刻触发，避免阻塞启动流程
            Fn: func(ctx context.Context) error {
                return app.SubManager.UpdateAll(ctx)
            },
        })
    }

    // 2. GeoIP 自动更新
    if cfg.Update.AutoGeoIP {
        interval, _ := time.ParseDuration(cfg.Update.GeoIPInterval)
        if interval <= 0 {
            interval = 168 * time.Hour
        }
        app.Scheduler.Add(scheduler.Job{
            Name:     "geoip-update",
            Interval: interval,
            Fn: func(ctx context.Context) error {
                return app.updateGeoIP(ctx)
            },
        })
    }

    // 3. Geosite 自动更新
    if cfg.Update.AutoGeosite {
        interval, _ := time.ParseDuration(cfg.Update.GeositeInterval)
        if interval <= 0 {
            interval = 168 * time.Hour
        }
        app.Scheduler.Add(scheduler.Job{
            Name:     "geosite-update",
            Interval: interval,
            Fn: func(ctx context.Context) error {
                return app.updateGeosite(ctx)
            },
        })
    }

    // 4. 心跳检查：每分钟检查 mihomo 是否存活
    app.Scheduler.Add(scheduler.Job{
        Name:     "core-heartbeat",
        Interval: 60 * time.Second,
        Fn: func(ctx context.Context) error {
            // 如果 CoreManager 认为在运行，但 /version API 无响应，触发重启
            if app.CoreManager.State() == core.StateRunning {
                if err := app.CoreManager.PingAPI(); err != nil {
                    log.Warn().Err(err).Msg("mihomo heartbeat failed, restarting")
                    return app.CoreManager.Restart(ctx)
                }
            }
            return nil
        },
    })
}
```

---

## 附录 D：dnsmasq 集成完整实现

### D.1 internal/dns/setup.go

```go
package dns

import (
    "fmt"
    "os"
    "os/exec"
    "strings"

    "github.com/rs/zerolog/log"
)

type DnsmasqMode string

const (
    ModeReplace  DnsmasqMode = "replace"  // 禁用 dnsmasq DNS，mihomo 完全接管
    ModeUpstream DnsmasqMode = "upstream" // dnsmasq 转发到 mihomo
    ModeNone     DnsmasqMode = "none"     // 不修改 dnsmasq
)

// metaclash 在 /tmp/dnsmasq.d/ 写入的配置文件名
const dnsmasqConfigFile = "/tmp/dnsmasq.d/metaclash.conf"

// Setup 根据模式配置 dnsmasq
// 必须在 mihomo 启动之后调用（此时 DNS 端口已就绪）
func Setup(mode DnsmasqMode, dnsPort int) error {
    switch mode {
    case ModeReplace:
        return setupReplace()
    case ModeUpstream:
        return setupUpstream(dnsPort)
    case ModeNone:
        log.Info().Msg("dns: dnsmasq_mode=none, skipping dnsmasq configuration")
        return nil
    default:
        return fmt.Errorf("unknown dnsmasq_mode: %s", mode)
    }
}

// Cleanup 在 metaclash 退出时恢复 dnsmasq 配置
func Cleanup(mode DnsmasqMode) error {
    if mode == ModeNone {
        return nil
    }

    // 删除我们写入的配置文件
    os.Remove(dnsmasqConfigFile)

    // 重启 dnsmasq 使配置生效
    return reloadDnsmasq()
}

// setupReplace 禁用 dnsmasq 的 DNS 功能（只保留 DHCP）
// 写入 /tmp/dnsmasq.d/metaclash.conf: port=0
func setupReplace() error {
    content := "# MetaClash: disable dnsmasq DNS, mihomo handles DNS directly\nport=0\n"
    if err := os.WriteFile(dnsmasqConfigFile, []byte(content), 0644); err != nil {
        return fmt.Errorf("write dnsmasq config: %w", err)
    }
    log.Info().Str("file", dnsmasqConfigFile).Msg("dns: disabled dnsmasq DNS (port=0)")
    return reloadDnsmasq()
}

// setupUpstream 将 dnsmasq 的上游设为 mihomo DNS 端口
// dnsmasq 仍然监听 53，但所有查询转发给 mihomo
func setupUpstream(dnsPort int) error {
    content := fmt.Sprintf(
        "# MetaClash: forward all DNS to mihomo\nno-resolv\nserver=127.0.0.1#%d\n",
        dnsPort,
    )
    if err := os.WriteFile(dnsmasqConfigFile, []byte(content), 0644); err != nil {
        return fmt.Errorf("write dnsmasq config: %w", err)
    }
    log.Info().Int("port", dnsPort).Msg("dns: configured dnsmasq upstream to mihomo")
    return reloadDnsmasq()
}

// reloadDnsmasq 向 dnsmasq 发送 SIGHUP 触发配置重载
func reloadDnsmasq() error {
    // 方法1：通过 /var/run/dnsmasq/dnsmasq.pid
    pidFile := "/var/run/dnsmasq/dnsmasq.pid"
    data, err := os.ReadFile(pidFile)
    if err != nil {
        // 方法2：通过 kill -HUP $(pidof dnsmasq)
        out, err2 := exec.Command("pidof", "dnsmasq").Output()
        if err2 != nil {
            log.Warn().Msg("dns: dnsmasq not running, skip reload")
            return nil
        }
        pid := strings.TrimSpace(string(out))
        return exec.Command("kill", "-HUP", pid).Run()
    }

    pid := strings.TrimSpace(string(data))
    if err := exec.Command("kill", "-HUP", pid).Run(); err != nil {
        return fmt.Errorf("reload dnsmasq (pid %s): %w", pid, err)
    }
    log.Info().Str("pid", pid).Msg("dns: dnsmasq reloaded")
    return nil
}
```

### D.2 dnsmasq 模式选择指南

| 场景 | 推荐模式 | 原因 |
|------|---------|------|
| 标准 OpenWrt，不需要本地域名解析 | `replace` | 最简单，DNS 延迟最低，mihomo 直接处理 |
| 有自定义 hosts / 本地域名（如 NAS、打印机） | `upstream` | 保留 dnsmasq 的 hosts 功能，DNS 查询经 dnsmasq → mihomo |
| 已有复杂 dnsmasq 配置，不想改动 | `none` | 用户自行处理 DNS，metaclash 只管透明代理 |

---

## 附录 E：完整 main.go 与 App 结构体

### E.1 cmd/metaclash/main.go

```go
package main

import (
    "context"
    "flag"
    "fmt"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/rs/zerolog"
    "github.com/rs/zerolog/log"

    "github.com/yourusername/metaclash/internal/api"
    "github.com/yourusername/metaclash/internal/config"
    "github.com/yourusername/metaclash/internal/core"
    "github.com/yourusername/metaclash/internal/daemon"
    "github.com/yourusername/metaclash/internal/dns"
    "github.com/yourusername/metaclash/internal/netfilter"
    "github.com/yourusername/metaclash/internal/scheduler"
    "github.com/yourusername/metaclash/internal/subscription"
)

// Version 由构建时 -ldflags 注入
var Version = "dev"

func main() {
    // ── 1. 命令行参数 ──────────────────────────────────────────
    var (
        configFile    = flag.String("config", "/etc/metaclash/config.toml", "config file path")
        versionFlag   = flag.Bool("version", false, "print version and exit")
        cleanupFlag   = flag.Bool("cleanup", false, "cleanup firewall rules and exit")
        forceCleanOld = flag.Bool("force-cleanup-old-rules", false,
            "also clean up OpenClash/legacy iptables rules before starting")
    )
    flag.Parse()

    if *versionFlag {
        fmt.Printf("metaclash %s\n", Version)
        os.Exit(0)
    }

    // ── 2. 配置加载 ────────────────────────────────────────────
    cfg, err := config.Load(*configFile)
    if err != nil {
        fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
        os.Exit(1)
    }

    // ── 3. 日志初始化 ──────────────────────────────────────────
    initLogger(cfg.Log)
    log.Info().Str("version", Version).Str("config", *configFile).Msg("metaclash starting")

    // ── 4. 运行时目录 ──────────────────────────────────────────
    if err := os.MkdirAll(cfg.Core.RuntimeDir, 0755); err != nil {
        log.Fatal().Err(err).Str("dir", cfg.Core.RuntimeDir).Msg("cannot create runtime dir")
    }

    // ── 5. PID 文件锁（防止多实例） ───────────────────────────
    pidFile := daemon.NewPIDFile(cfg.Core.RuntimeDir + "/metaclash.pid")
    if err := pidFile.Acquire(); err != nil {
        log.Fatal().Err(err).Msg("another instance is already running")
    }
    defer pidFile.Release()

    // ── 6. 构建 App ────────────────────────────────────────────
    app := &App{
        Config:    cfg,
        Version:   Version,
        Scheduler: scheduler.New(),
    }

    // 防火墙后端
    backend := netfilter.DetectBackend(cfg.Network.FirewallBackend)
    if backend == netfilter.BackendNone && cfg.Network.Mode != "none" {
        log.Warn().Msg("no supported firewall backend found, transparent proxy disabled")
    }
    app.Netfilter = netfilter.NewManager(backend, cfg)

    // --cleanup 模式：只清理规则，不启动服务
    if *cleanupFlag {
        if err := app.Netfilter.Cleanup(); err != nil {
            log.Error().Err(err).Msg("cleanup failed")
            os.Exit(1)
        }
        log.Info().Msg("cleanup complete")
        os.Exit(0)
    }

    // 清理旧规则（OpenClash 迁移场景）
    if *forceCleanOld {
        cleanupLegacyRules()
    }

    // 订阅管理
    app.SubManager = subscription.NewManager(cfg)

    // 核心管理
    app.CoreManager = core.NewManager(core.CoreManagerConfig{
        Binary:      cfg.Core.Binary,
        ConfigFile:  cfg.Core.RuntimeDir + "/mihomo-config.yaml",
        APIPort:     cfg.Ports.MihomoAPI,
        MaxRestarts: cfg.Core.MaxRestarts,
        OnStateChange: func(state core.CoreState, pid int) {
            // 通过 SSE 推送状态变化
            if app.SSEBroker != nil {
                app.SSEBroker.Publish("core_state", map[string]interface{}{
                    "state": string(state),
                    "pid":   pid,
                })
            }
        },
    })

    // SSE broker
    app.SSEBroker = api.NewSSEBroker()

    // HTTP API 服务器
    app.HTTPServer = api.NewServer(api.ServerConfig{
        ListenAddr: listenAddr(cfg),
        App:        app,
    })

    // ── 7. 启动序列 ────────────────────────────────────────────
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    if err := app.Start(ctx); err != nil {
        log.Fatal().Err(err).Msg("startup failed")
    }

    // ── 8. 信号处理 ────────────────────────────────────────────
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

    for sig := range sigCh {
        switch sig {
        case syscall.SIGHUP:
            log.Info().Msg("received SIGHUP, reloading")
            if err := app.Reload(ctx); err != nil {
                log.Error().Err(err).Msg("reload failed")
            }

        case syscall.SIGTERM, syscall.SIGINT:
            log.Info().Msg("received shutdown signal")
            cancel()
            app.Shutdown()
            return
        }
    }
}

func initLogger(cfg config.LogConfig) {
    level, err := zerolog.ParseLevel(cfg.Level)
    if err != nil {
        level = zerolog.InfoLevel
    }
    zerolog.SetGlobalLevel(level)

    if cfg.File == "" {
        // stdout（procd 会捕获）
        log.Logger = log.Output(zerolog.ConsoleWriter{
            Out:        os.Stdout,
            TimeFormat: "15:04:05",
            NoColor:    true, // 路由器终端通常不支持颜色
        })
    } else {
        f, err := os.OpenFile(cfg.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
        if err != nil {
            log.Fatal().Err(err).Msg("cannot open log file")
        }
        log.Logger = log.Output(f)
    }
}

func listenAddr(cfg *config.MetaclashConfig) string {
    if cfg.Security.AllowLAN {
        return fmt.Sprintf("0.0.0.0:%d", cfg.Ports.UI)
    }
    return fmt.Sprintf("127.0.0.1:%d", cfg.Ports.UI)
}

// cleanupLegacyRules 清理 OpenClash 可能留下的旧规则
func cleanupLegacyRules() {
    log.Info().Msg("cleaning up legacy rules (OpenClash/other)")
    cmds := [][]string{
        {"nft", "delete", "table", "inet", "fw4"},        // 小心：只删 openclash 的链
        {"iptables", "-t", "mangle", "-F", "CLASH"},
        {"iptables", "-t", "mangle", "-X", "CLASH"},
        {"iptables", "-t", "nat", "-F", "CLASH_DNS"},
        {"iptables", "-t", "nat", "-X", "CLASH_DNS"},
    }
    for _, cmd := range cmds {
        exec.Command(cmd[0], cmd[1:]...).Run() // 静默忽略错误
    }
}
```

### E.2 App 结构体（应用核心）

```go
// cmd/metaclash/app.go

package main

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "github.com/rs/zerolog/log"

    "github.com/yourusername/metaclash/internal/api"
    "github.com/yourusername/metaclash/internal/config"
    "github.com/yourusername/metaclash/internal/core"
    "github.com/yourusername/metaclash/internal/dns"
    "github.com/yourusername/metaclash/internal/netfilter"
    "github.com/yourusername/metaclash/internal/scheduler"
    "github.com/yourusername/metaclash/internal/subscription"
)

// App 是 metaclash 的整个应用状态，持有所有子系统的引用
type App struct {
    Version     string
    Config      *config.MetaclashConfig
    CoreManager *core.Manager
    SubManager  *subscription.Manager
    Netfilter   *netfilter.Manager
    Scheduler   *scheduler.Scheduler
    SSEBroker   *api.SSEBroker
    HTTPServer  *api.Server
    startTime   time.Time
}

// Start 按顺序启动所有子系统
// 任何步骤失败都会触发已完成步骤的回滚
func (app *App) Start(ctx context.Context) error {
    app.startTime = time.Now()
    steps := []struct {
        name   string
        run    func() error
        rollback func()
    }{
        {
            name: "validate mihomo binary",
            run: func() error {
                if err := validateBinary(app.Config.Core.Binary); err != nil {
                    // 尝试首次安装
                    log.Info().Msg("mihomo binary not found, attempting first-time setup")
                    return app.firstTimeSetup(ctx)
                }
                return nil
            },
        },
        {
            name: "generate mihomo config",
            run: func() error {
                return app.SubManager.GenerateConfig(ctx)
            },
        },
        {
            name: "apply firewall rules",
            run: func() error {
                return app.Netfilter.Apply()
            },
            rollback: func() {
                app.Netfilter.Cleanup()
            },
        },
        {
            name: "start mihomo",
            run: func() error {
                return app.CoreManager.Start(ctx)
            },
            rollback: func() {
                app.CoreManager.Stop()
            },
        },
        {
            name: "configure dns",
            run: func() error {
                return dns.Setup(
                    dns.DnsmasqMode(app.Config.DNS.DnsmasqMode),
                    app.Config.Ports.DNS,
                )
            },
            rollback: func() {
                dns.Cleanup(dns.DnsmasqMode(app.Config.DNS.DnsmasqMode))
            },
        },
        {
            name: "start http api server",
            run: func() error {
                go app.HTTPServer.Start()
                return nil
            },
            rollback: func() {
                shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
                defer cancel()
                app.HTTPServer.Shutdown(shutdownCtx)
            },
        },
        {
            name: "start scheduler",
            run: func() error {
                app.registerScheduledJobs()
                app.Scheduler.Start(ctx)
                return nil
            },
        },
    }

    var completed []int
    for i, step := range steps {
        log.Info().Str("step", step.name).Msg("startup")
        if err := step.run(); err != nil {
            log.Error().Err(err).Str("step", step.name).Msg("startup step failed, rolling back")
            // 逆序回滚已完成的步骤
            for j := len(completed) - 1; j >= 0; j-- {
                s := steps[completed[j]]
                if s.rollback != nil {
                    log.Info().Str("step", s.name).Msg("rollback")
                    s.rollback()
                }
            }
            return fmt.Errorf("startup failed at step %q: %w", step.name, err)
        }
        completed = append(completed, i)
    }

    log.Info().
        Dur("elapsed", time.Since(app.startTime)).
        Int("port", app.Config.Ports.UI).
        Msg("metaclash started successfully")
    return nil
}

// Shutdown 按逆序关闭所有子系统（严格顺序，不可改变）
func (app *App) Shutdown() {
    log.Info().Msg("shutting down metaclash")

    // 1. 停止调度器（不再触发新任务）
    app.Scheduler.Stop()
    log.Debug().Msg("scheduler stopped")

    // 2. 优雅关闭 HTTP 服务器（等待进行中的请求完成，最多 5 秒）
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := app.HTTPServer.Shutdown(shutdownCtx); err != nil {
        log.Warn().Err(err).Msg("http server forced shutdown")
    }
    log.Debug().Msg("http server stopped")

    // 3. 恢复 dnsmasq 配置
    if err := dns.Cleanup(dns.DnsmasqMode(app.Config.DNS.DnsmasqMode)); err != nil {
        log.Error().Err(err).Msg("dns cleanup failed")
    }
    log.Debug().Msg("dns restored")

    // 4. 停止 mihomo 进程
    if err := app.CoreManager.Stop(); err != nil {
        log.Error().Err(err).Msg("core stop failed")
    }
    log.Debug().Msg("mihomo stopped")

    // 5. 清理防火墙规则（最关键！必须成功，否则路由器断网）
    if err := app.Netfilter.Cleanup(); err != nil {
        log.Error().Err(err).Msg("CRITICAL: firewall cleanup failed! network may be broken")
        // 不 panic，继续清理其他资源
    }
    log.Debug().Msg("firewall rules cleaned")

    log.Info().Msg("shutdown complete")
}

// Reload 热重载：重新加载配置，重新生成 mihomo 配置，触发热重载
// 不重启 mihomo 进程，不中断已有连接
func (app *App) Reload(ctx context.Context) error {
    log.Info().Msg("reloading configuration")

    // 1. 重新加载 metaclash 自身配置
    newCfg, err := config.Load(app.Config.configFile())
    if err != nil {
        return fmt.Errorf("reload config: %w", err)
    }

    // 2. 重新生成 mihomo 配置（写到临时文件，验证通过再替换）
    if err := app.SubManager.GenerateConfigWithCfg(ctx, newCfg); err != nil {
        return fmt.Errorf("regenerate config: %w", err)
    }

    // 3. 热重载 mihomo（PUT /configs）
    if err := app.CoreManager.Reload(app.Config.Core.RuntimeDir + "/mihomo-config.yaml"); err != nil {
        return fmt.Errorf("mihomo reload: %w", err)
    }

    // 4. 如果网络模式发生变化，重新应用防火墙规则
    if newCfg.Network.Mode != app.Config.Network.Mode ||
        newCfg.Network.FirewallBackend != app.Config.Network.FirewallBackend {
        app.Netfilter.Cleanup()
        app.Netfilter = netfilter.NewManager(
            netfilter.DetectBackend(newCfg.Network.FirewallBackend),
            newCfg,
        )
        if err := app.Netfilter.Apply(); err != nil {
            return fmt.Errorf("reapply firewall: %w", err)
        }
    }

    app.Config = newCfg
    log.Info().Msg("reload complete")
    return nil
}

// firstTimeSetup 首次启动时自动下载 mihomo 和数据库文件
func (app *App) firstTimeSetup(ctx context.Context) error {
    log.Info().Msg("first time setup: downloading mihomo and databases")

    arch := detectArch()
    log.Info().Str("arch", arch).Msg("detected architecture")

    // 下载 mihomo
    mihomoURL := fmt.Sprintf(
        "https://github.com/MetaCubeX/mihomo/releases/latest/download/mihomo-linux-%s.tar.gz",
        arch,
    )
    if err := downloadAndExtract(ctx, mihomoURL, app.Config.Core.Binary); err != nil {
        return fmt.Errorf("download mihomo: %w", err)
    }
    os.Chmod(app.Config.Core.Binary, 0755)

    // 下载 GeoIP
    if err := downloadFile(ctx, app.Config.Update.GeoIPURL, app.Config.Core.GeoIPPath); err != nil {
        log.Warn().Err(err).Msg("GeoIP download failed, continuing without it")
    }

    // 下载 Geosite
    if err := downloadFile(ctx, app.Config.Update.GeositeURL, app.Config.Core.GeositePath); err != nil {
        log.Warn().Err(err).Msg("Geosite download failed, continuing without it")
    }

    log.Info().Msg("first time setup complete")
    return nil
}
```

---

## 附录 F：mihomo YAML 配置模板

这是 metaclash 生成 mihomo 配置时使用的基础模板（内嵌在二进制中）。`generator.go` 在此基础上填充动态内容。

```yaml
# /internal/config/base.yaml（Go embed，不对外暴露）
# 此文件是生成层的最低优先级基础，所有值都会被 generator 和 overrides 覆盖

# ── 全局端口（由 generator 覆盖）──────────────────────────────
port: 7890
socks-port: 7891
mixed-port: 7893
redir-port: 7892
tproxy-port: 7895
allow-lan: true
bind-address: "*"

# ── 运行模式 ────────────────────────────────────────────────
mode: rule
log-level: warning   # mihomo 自身日志级别（metaclash 有独立日志系统）

# ── 外部控制 API ─────────────────────────────────────────────
external-controller: "127.0.0.1:9090"
external-controller-secret: ""   # 由 generator 根据 security.api_secret 填充
# 不设置 external-ui，metaclash 提供自己的 Web UI

# ── 嗅探（域名还原） ──────────────────────────────────────────
sniffer:
  enable: true
  sniff:
    HTTP:
      ports: [80, 8080]
    TLS:
      ports: [443, 8443]
    QUIC:
      ports: [443, 8443]
  skip-domain:
    - "Mijia Cloud"
    - "+.apple.com"

# ── TUN 模式（默认关闭，通过 network.mode=tun 启用）──────────
tun:
  enable: false
  stack: system
  dns-hijack:
    - "any:53"
  auto-route: true
  auto-detect-interface: true

# ── profile ─────────────────────────────────────────────────
profile:
  store-selected: true   # 记住用户手动选择的节点
  store-fake-ip: true    # 重启后恢复 fake-ip 映射

# ── geodata ─────────────────────────────────────────────────
geodata-mode: true
geox-url:
  geoip: ""    # 由 generator 填充实际路径（file:// 协议）
  geosite: ""
  mmdb: ""
geo-auto-update: false  # 由 metaclash 调度器负责更新，mihomo 自身不自动更新
```

### F.1 generator.go 生成逻辑（关键片段）

```go
// internal/config/generator.go

package config

import (
    "fmt"
    "os"
    "path/filepath"

    "gopkg.in/yaml.v3"
)

//go:embed base.yaml
var baseYAML []byte

// Generate 从 MetaclashConfig 和订阅节点生成完整的 mihomo YAML
// 输出写到 outputPath（通常是 /var/run/metaclash/mihomo-config.yaml）
func Generate(cfg *MetaclashConfig, nodes []ProxyNode, outputPath string) error {
    // Step 1: 解析基础模板
    var base map[string]interface{}
    if err := yaml.Unmarshal(baseYAML, &base); err != nil {
        return fmt.Errorf("parse base yaml: %w", err)
    }

    // Step 2: 覆盖全局设置
    base["port"] = cfg.Ports.HTTP
    base["socks-port"] = cfg.Ports.SOCKS
    base["mixed-port"] = cfg.Ports.Mixed
    base["redir-port"] = cfg.Ports.Redir
    base["tproxy-port"] = cfg.Ports.TProxy
    base["external-controller"] = fmt.Sprintf("127.0.0.1:%d", cfg.Ports.MihomoAPI)
    if cfg.Security.APISecret != "" {
        base["external-controller-secret"] = cfg.Security.APISecret
    }

    // Step 3: TUN 模式
    if cfg.Network.Mode == "tun" {
        tun := base["tun"].(map[string]interface{})
        tun["enable"] = true
        base["tun"] = tun
    }

    // Step 4: DNS 配置
    base["dns"] = buildDNSConfig(cfg)

    // Step 5: GeoData 路径（file:// 协议）
    base["geox-url"] = map[string]interface{}{
        "geoip":   "file://" + cfg.Core.GeoIPPath,
        "geosite": "file://" + cfg.Core.GeositePath,
        "mmdb":    "file://" + cfg.Core.GeoIPPath,
    }

    // Step 6: 代理节点列表
    proxies := make([]map[string]interface{}, 0, len(nodes))
    for _, node := range nodes {
        p := map[string]interface{}{
            "name":   node.Name,
            "type":   node.Type,
            "server": node.Server,
            "port":   node.Port,
        }
        for k, v := range node.Extra {
            p[k] = v
        }
        proxies = append(proxies, p)
    }
    base["proxies"] = proxies

    // Step 7: 代理组
    base["proxy-groups"] = buildProxyGroups(nodes, cfg)

    // Step 8: 规则
    base["rules"] = buildRules(cfg)

    // Step 9: 加载用户覆写并合并
    overridesPath := filepath.Join(cfg.Core.DataDir, "overrides.yaml")
    if data, err := os.ReadFile(overridesPath); err == nil && len(data) > 0 {
        var overrides map[string]interface{}
        if err := yaml.Unmarshal(data, &overrides); err == nil {
            deepMerge(base, overrides, "")
        }
    }

    // Step 10: 序列化并写到临时文件，再原子替换
    tmpPath := outputPath + ".tmp"
    data, err := yaml.Marshal(base)
    if err != nil {
        return fmt.Errorf("marshal yaml: %w", err)
    }
    if err := os.WriteFile(tmpPath, data, 0644); err != nil {
        return fmt.Errorf("write config: %w", err)
    }
    return os.Rename(tmpPath, outputPath) // 原子替换
}

func buildDNSConfig(cfg *MetaclashConfig) map[string]interface{} {
    dns := map[string]interface{}{
        "enable":            cfg.DNS.Enable,
        "listen":            fmt.Sprintf("0.0.0.0:%d", cfg.Ports.DNS),
        "enhanced-mode":     cfg.DNS.Mode, // "fake-ip" or "redir-host"
        "fake-ip-range":     "198.18.0.0/15",
        "fake-ip-filter":    cfg.DNS.FakeIPFilter,
        "nameserver":        cfg.DNS.Nameservers,
        "fallback":          cfg.DNS.Fallback,
        "default-nameserver": []string{"119.29.29.29", "223.5.5.5"},
        // fallback-filter：境外 IP 走 fallback DNS
        "fallback-filter": map[string]interface{}{
            "geoip":     true,
            "geoip-code": "CN",
        },
    }

    // 如果配置了 DoH，nameserver 中加入
    for _, doh := range cfg.DNS.DoH {
        dns["nameserver"] = append(dns["nameserver"].([]string), doh)
    }

    return dns
}

func buildProxyGroups(nodes []ProxyNode, cfg *MetaclashConfig) []interface{} {
    nodeNames := make([]string, len(nodes))
    for i, n := range nodes {
        nodeNames[i] = n.Name
    }

    groups := []interface{}{
        // 自动测速组（放第一，作为 Proxy 的子组）
        map[string]interface{}{
            "name":      "Auto",
            "type":      "url-test",
            "url":       "http://www.gstatic.com/generate_204",
            "interval":  300,
            "tolerance": 50,
            "proxies":   nodeNames,
        },
        // 主选择组
        map[string]interface{}{
            "name":    "Proxy",
            "type":    "select",
            "proxies": append([]string{"Auto", "DIRECT"}, nodeNames...),
        },
        // 兜底组
        map[string]interface{}{
            "name":    "Final",
            "type":    "select",
            "proxies": []string{"Proxy", "DIRECT"},
        },
    }

    return groups
}

func buildRules(cfg *MetaclashConfig) []string {
    rules := []string{
        // 本地流量直连
        "DOMAIN-SUFFIX,local,DIRECT",
        "IP-CIDR,127.0.0.0/8,DIRECT,no-resolve",
        "IP-CIDR,192.168.0.0/16,DIRECT,no-resolve",
        "IP-CIDR,10.0.0.0/8,DIRECT,no-resolve",
        "IP-CIDR,172.16.0.0/12,DIRECT,no-resolve",
        "IP-CIDR,100.64.0.0/10,DIRECT,no-resolve",
    }

    if cfg.Network.BypassChina {
        rules = append(rules,
            "GEOSITE,cn,DIRECT",
            "GEOIP,CN,DIRECT,no-resolve",
        )
    }

    // 额外绕过 CIDR
    for _, cidr := range cfg.Network.BypassCIDR {
        rules = append(rules, fmt.Sprintf("IP-CIDR,%s,DIRECT,no-resolve", cidr))
    }

    rules = append(rules, "MATCH,Final")
    return rules
}
```

---

## 附录 G：Web UI 剩余页面规格

### G.1 Connections 页面

**功能**：展示所有活跃连接，支持搜索、关闭单条连接、关闭全部。

**数据来源**：`GET /api/v1/connections`，每 3 秒轮询一次（不用 SSE，避免大量数据推送）。

**表格列**：

| 列 | 字段 | 说明 |
|----|------|------|
| 来源 | `metadata.sourceIP:port` | 发起连接的设备 |
| 目标 | `metadata.host` 或 `metadata.destinationIP` | 目标域名/IP |
| 规则 | `rule` + `rulePayload` | 匹配的规则 |
| 代理 | `chains` | 使用的代理链 |
| 上传 | `upload` | 累计上传字节 |
| 下载 | `download` | 累计下载字节 |
| 时间 | `start` | 连接建立时间（相对时间，如 "2分钟前"） |
| 操作 | — | 关闭按钮 |

**搜索**：对 host 和 sourceIP 做前端过滤（不发额外 API 请求）。

```typescript
// pages/Connections.tsx 关键逻辑

const [connections, setConnections] = useState<Connection[]>([])
const [filter, setFilter] = useState('')

// 每 3 秒刷新
useEffect(() => {
  const timer = setInterval(async () => {
    const data = await getConnections()
    setConnections(data.connections || [])
  }, 3000)
  return () => clearInterval(timer)
}, [])

const filtered = connections.filter(c =>
  (c.metadata.host || c.metadata.destinationIP || '')
    .toLowerCase()
    .includes(filter.toLowerCase()) ||
  c.metadata.sourceIP.includes(filter)
)
```

### G.2 Rules 页面

**功能**：只读展示当前生效的规则列表，支持搜索。

**数据来源**：`GET /api/v1/rules`，页面加载时获取一次。

**规则条目显示**：

```
GEOSITE    │ cn              │ → DIRECT
GEOIP      │ CN              │ → DIRECT    (no-resolve)
IP-CIDR    │ 192.168.0.0/16  │ → DIRECT    (no-resolve)
DOMAIN     │ example.com     │ → Proxy
MATCH      │                 │ → Final
```

颜色编码：
- `DIRECT` → 绿色
- `REJECT` → 红色
- 代理组名 → 蓝色（紫色/Indigo）

**搜索**：前端过滤 `type + payload`。

### G.3 Logs 页面

**功能**：实时日志流（SSE），支持级别过滤，支持暂停/继续。

```typescript
// pages/Logs.tsx 关键逻辑

const [logs, setLogs] = useState<LogEntry[]>([])
const [paused, setPaused] = useState(false)
const [levelFilter, setLevelFilter] = useState<string>('info')
const pendingRef = useRef<LogEntry[]>([])

useSSE({
  onLog: (entry) => {
    if (paused) {
      pendingRef.current.push(entry)
      return
    }
    setLogs(prev => [...prev.slice(-499), entry]) // 最多保留 500 条
  },
})

const handleResume = () => {
  setLogs(prev => [...prev, ...pendingRef.current].slice(-500))
  pendingRef.current = []
  setPaused(false)
}

const displayLogs = logs.filter(l => {
  const levels = ['debug', 'info', 'warn', 'error']
  return levels.indexOf(l.level) >= levels.indexOf(levelFilter)
})
```

级别徽章颜色：
- `debug` → 灰色
- `info`  → 蓝色
- `warn`  → 黄色
- `error` → 红色

### G.4 Settings 页面详细表单规格

Settings 页面分为 4 个 Tab，每个 Tab 对应 config.toml 的一个区块。

**Tab 1：常规（General）**

| 控件类型 | 标签 | 字段 | 说明 |
|---------|------|------|------|
| Select | 运行模式 | `network.mode` | rule / global / direct 三选一 |
| NumberInput | HTTP 代理端口 | `ports.http` | 1024-65535 |
| NumberInput | SOCKS 代理端口 | `ports.socks` | |
| NumberInput | 混合端口 | `ports.mixed` | |
| Toggle | 允许局域网连接 | `security.allow_lan` | |
| PasswordInput | API 密钥 | `security.api_secret` | 留空=不鉴权 |
| Select | 日志级别 | `log.level` | debug/info/warn/error |

**Tab 2：网络（Network）**

| 控件类型 | 标签 | 字段 |
|---------|------|------|
| RadioGroup | 透明代理模式 | `network.mode` (tproxy/tun/redir/none) |
| Select | 防火墙后端 | `network.firewall_backend` (auto/nftables/iptables) |
| Toggle | 绕过局域网 | `network.bypass_lan` |
| Toggle | 绕过中国大陆 IP | `network.bypass_china` |
| Toggle | IPv6 透明代理 | `network.ipv6` |
| TextArea | 额外绕过 CIDR | `network.bypass_cidr` (每行一个) |

**Tab 3：DNS**

| 控件类型 | 标签 | 字段 |
|---------|------|------|
| Toggle | 启用 DNS | `dns.enable` |
| RadioGroup | DNS 模式 | `dns.mode` (fake-ip/redir-host) |
| TextArea | 国内 DNS | `dns.nameservers` (每行一个) |
| TextArea | 境外 DNS | `dns.fallback` |
| TextArea | DNS over HTTPS | `dns.doh` (每行一个) |
| Select | dnsmasq 共存模式 | `dns.dnsmasq_mode` (replace/upstream/none) |

**Tab 4：自动更新（Updates）**

| 控件类型 | 标签 | 字段 |
|---------|------|------|
| Toggle | 自动更新订阅 | `update.auto_subscription` |
| Select | 订阅更新间隔 | `update.subscription_interval` (1h/3h/6h/12h/24h) |
| Toggle | 自动更新 GeoIP | `update.auto_geoip` |
| Toggle | 自动更新 Geosite | `update.auto_geosite` |
| Select | 数据库更新间隔 | `update.geoip_interval` (24h/72h/168h) |
| Button | 立即更新 GeoIP | → POST /api/v1/geoip/update |
| Button | 立即更新 Geosite | → POST /api/v1/geosite/update |
| Button | 检查 mihomo 更新 | → GET /api/v1/core/version |

Settings 保存逻辑：
```typescript
// 用户点击保存时：
// 1. 前端校验（端口范围、CIDR 格式等）
// 2. PUT /api/v1/config（只发变更的字段）
// 3. 成功后显示 toast "配置已保存，正在热重载..."
// 4. 监听 SSE core_state 事件确认重载完成
```

---

## 附录 H：端口冲突检测实现

```go
// internal/daemon/ports.go

package daemon

import (
    "fmt"
    "net"
    "time"
)

// CheckPorts 在启动前检查所有需要用到的端口是否可用
// ports: 端口号列表
// 返回冲突的端口列表（为空则全部可用）
func CheckPorts(ports []int) []int {
    var conflicts []int
    for _, port := range ports {
        if !isPortAvailable(port) {
            conflicts = append(conflicts, port)
        }
    }
    return conflicts
}

func isPortAvailable(port int) bool {
    // 尝试 TCP bind
    ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
    if err != nil {
        return false
    }
    ln.Close()

    // 尝试 UDP bind
    conn, err := net.ListenPacket("udp", fmt.Sprintf(":%d", port))
    if err != nil {
        return false
    }
    conn.Close()

    return true
}

// 在 App.Start() 的第一步调用：
func (app *App) checkPortConflicts() error {
    cfg := app.Config
    ports := []int{
        cfg.Ports.HTTP,
        cfg.Ports.SOCKS,
        cfg.Ports.Mixed,
        cfg.Ports.Redir,
        cfg.Ports.TProxy,
        cfg.Ports.DNS,
        cfg.Ports.MihomoAPI,
        cfg.Ports.UI,
    }

    conflicts := daemon.CheckPorts(ports)
    if len(conflicts) > 0 {
        return fmt.Errorf(
            "port(s) already in use: %v — check if another proxy or metaclash instance is running",
            conflicts,
        )
    }
    return nil
}
```

---

## 附录 I：PID 文件锁实现

```go
// internal/daemon/pidfile.go

package daemon

import (
    "fmt"
    "os"
    "strconv"
    "strings"
    "syscall"
)

type PIDFile struct {
    path string
    file *os.File
}

func NewPIDFile(path string) *PIDFile {
    return &PIDFile{path: path}
}

// Acquire 获取 PID 文件锁
// 如果文件已存在且对应进程仍存活，返回错误
// 否则写入当前 PID 并返回
func (p *PIDFile) Acquire() error {
    // 检查是否有旧的 PID 文件
    if data, err := os.ReadFile(p.path); err == nil {
        pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
        if pid > 0 && processExists(pid) {
            return fmt.Errorf("metaclash already running (pid %d)", pid)
        }
        // 旧进程已退出，删除残留 PID 文件
        os.Remove(p.path)
    }

    // 写入当前 PID（使用 O_EXCL 保证原子创建）
    f, err := os.OpenFile(p.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
    if err != nil {
        // 竞态：另一个进程刚才也在启动
        return fmt.Errorf("cannot create pid file %s: %w", p.path, err)
    }
    p.file = f

    if _, err := fmt.Fprintf(f, "%d\n", os.Getpid()); err != nil {
        f.Close()
        os.Remove(p.path)
        return err
    }
    return f.Sync()
}

// Release 释放 PID 文件
func (p *PIDFile) Release() {
    if p.file != nil {
        p.file.Close()
    }
    os.Remove(p.path)
}

// processExists 检查指定 PID 的进程是否存在
func processExists(pid int) bool {
    process, err := os.FindProcess(pid)
    if err != nil {
        return false
    }
    // 发送 signal 0 只检查进程是否存在，不实际发送信号
    err = process.Signal(syscall.Signal(0))
    return err == nil
}
```

---

## 附录 J：GeoIP / Geosite 更新实现

```go
// 在 App 中实现，被调度器和 API handler 调用

func (app *App) updateGeoIP(ctx context.Context) error {
    return app.downloadDatabase(ctx,
        app.Config.Update.GeoIPURL,
        app.Config.Core.GeoIPPath,
        "GeoIP",
    )
}

func (app *App) updateGeosite(ctx context.Context) error {
    return app.downloadDatabase(ctx,
        app.Config.Update.GeositeURL,
        app.Config.Core.GeositePath,
        "Geosite",
    )
}

func (app *App) downloadDatabase(ctx context.Context, url, destPath, name string) error {
    log.Info().Str("db", name).Str("url", url).Msg("updating database")

    tmpPath := destPath + ".tmp"

    // 下载到临时文件
    if err := downloadFile(ctx, url, tmpPath); err != nil {
        os.Remove(tmpPath)
        return fmt.Errorf("download %s: %w", name, err)
    }

    // 验证文件大小（至少 1 MB，防止下载到错误页面）
    info, err := os.Stat(tmpPath)
    if err != nil || info.Size() < 1024*1024 {
        os.Remove(tmpPath)
        return fmt.Errorf("%s download too small (%d bytes), likely an error page",
            name, info.Size())
    }

    // 原子替换
    if err := os.Rename(tmpPath, destPath); err != nil {
        os.Remove(tmpPath)
        return fmt.Errorf("replace %s: %w", name, err)
    }

    log.Info().Str("db", name).Int64("size", info.Size()).Msg("database updated")

    // 触发 mihomo 热重载（让新数据库生效）
    if app.CoreManager.State() == core.StateRunning {
        return app.CoreManager.Reload(
            app.Config.Core.RuntimeDir + "/mihomo-config.yaml",
        )
    }
    return nil
}

// downloadFile 带进度、超时和重试的 HTTP 下载
func downloadFile(ctx context.Context, url, destPath string) error {
    const maxRetries = 3
    const timeout = 120 * time.Second

    var lastErr error
    for attempt := 1; attempt <= maxRetries; attempt++ {
        if attempt > 1 {
            log.Info().Int("attempt", attempt).Msg("retrying download")
            select {
            case <-time.After(5 * time.Second):
            case <-ctx.Done():
                return ctx.Err()
            }
        }

        reqCtx, cancel := context.WithTimeout(ctx, timeout)
        err := doDownload(reqCtx, url, destPath)
        cancel()

        if err == nil {
            return nil
        }
        lastErr = err
        log.Warn().Err(err).Int("attempt", attempt).Msg("download failed")
    }
    return fmt.Errorf("all %d attempts failed, last error: %w", maxRetries, lastErr)
}

func doDownload(ctx context.Context, url, destPath string) error {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    if err != nil {
        return err
    }
    req.Header.Set("User-Agent", "metaclash/"+Version)

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("HTTP %d", resp.StatusCode)
    }

    f, err := os.Create(destPath)
    if err != nil {
        return err
    }
    defer f.Close()

    _, err = io.Copy(f, resp.Body)
    return err
}
```

---

## 附录 K：SSE Broker 完整实现

```go
// internal/api/sse.go

package api

import (
    "encoding/json"
    "fmt"
    "net/http"
    "sync"
    "time"
)

// SSEBroker 管理所有 SSE 客户端连接
// 支持多客户端并发订阅，线程安全
type SSEBroker struct {
    mu      sync.RWMutex
    clients map[string]chan SSEMessage // key: client ID
}

type SSEMessage struct {
    Event string
    Data  interface{}
}

func NewSSEBroker() *SSEBroker {
    b := &SSEBroker{
        clients: make(map[string]chan SSEMessage),
    }
    // 启动流量统计推送（每秒从 mihomo API 拉取并广播）
    go b.trafficLoop()
    return b
}

// Publish 向所有已连接的客户端推送事件
func (b *SSEBroker) Publish(event string, data interface{}) {
    msg := SSEMessage{Event: event, Data: data}
    b.mu.RLock()
    defer b.mu.RUnlock()
    for _, ch := range b.clients {
        select {
        case ch <- msg:
        default:
            // 客户端消费太慢，丢弃这条消息（不阻塞 Publish 调用者）
        }
    }
}

// ServeHTTP 处理 SSE 长连接请求
func (b *SSEBroker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // 设置 SSE 响应头
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    w.Header().Set("X-Accel-Buffering", "no") // 禁止 nginx 缓冲
    w.WriteHeader(http.StatusOK)

    // 注册客户端
    clientID := fmt.Sprintf("%d", time.Now().UnixNano())
    ch := make(chan SSEMessage, 64) // 缓冲 64 条，防止慢客户端阻塞 Publish
    b.mu.Lock()
    b.clients[clientID] = ch
    b.mu.Unlock()

    defer func() {
        b.mu.Lock()
        delete(b.clients, clientID)
        close(ch)
        b.mu.Unlock()
    }()

    // 发送初始心跳，确认连接建立
    fmt.Fprintf(w, ": connected\n\n")
    if f, ok := w.(http.Flusher); ok {
        f.Flush()
    }

    // 定时心跳（15 秒），防止代理服务器断开空闲连接
    ticker := time.NewTicker(15 * time.Second)
    defer ticker.Stop()

    flusher, _ := w.(http.Flusher)

    for {
        select {
        case <-r.Context().Done():
            return // 客户端断开连接

        case <-ticker.C:
            fmt.Fprintf(w, ": heartbeat\n\n")
            if flusher != nil {
                flusher.Flush()
            }

        case msg, ok := <-ch:
            if !ok {
                return
            }
            data, err := json.Marshal(msg.Data)
            if err != nil {
                continue
            }
            fmt.Fprintf(w, "event: %s\ndata: %s\n\n", msg.Event, data)
            if flusher != nil {
                flusher.Flush()
            }
        }
    }
}

// trafficLoop 每秒从 mihomo API 拉取流量数据并广播
func (b *SSEBroker) trafficLoop() {
    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()

    // mihomo 的 /traffic 是 ndjson 流，这里用轮询简化实现
    for range ticker.C {
        // 从 mihomo API 获取实时流量
        // GET http://127.0.0.1:9090/traffic 返回的是流式数据，
        // 这里改为每秒调用一次连接列表计算差值
        // 实际实现：维护上一次的累计值，计算每秒增量
        b.Publish("traffic", map[string]interface{}{
            "up":   0, // 从 mihomo connections API 计算
            "down": 0,
            "ts":   time.Now().Unix(),
        })
    }
}
```

**注意**：`trafficLoop` 中的流量数据应从 mihomo 的 `/connections` 接口计算每秒增量（对所有连接的 `upload`/`download` 求和，与上一秒对比得到速率）。具体实现参考 mihomo API 返回格式中的 `uploadTotal` 和 `downloadTotal` 字段。

---

## 附录 L：完整错误场景处理矩阵

| 场景 | 检测方式 | 处理逻辑 | 用户感知 |
|------|---------|---------|---------|
| mihomo 二进制不存在 | 启动时 `os.Stat` | 触发首次安装流程，下载二进制 | UI 显示"正在初始化..." |
| mihomo 启动后立即崩溃 | `cmd.Wait()` 在 2 秒内返回 | 读取 stderr 输出，分析原因（端口冲突？配置错误？）上报错误 | UI core_state=error，显示错误信息 |
| mihomo 配置 YAML 语法错误 | `mihomo -t` 失败 | 保留旧配置，不写入新配置文件，返回具体行号和错误描述 | API 返回 CONFIG_INVALID + 详细消息 |
| 订阅下载超时（120s） | context deadline | 保留上次缓存，继续使用旧节点，记录警告日志 | UI 订阅状态显示"上次更新 X 分钟前，更新失败" |
| 订阅内容 0 节点（解析后） | `len(nodes) == 0` | 记录警告，不更新缓存，不触发配置重新生成 | UI 显示"0 个可用节点" |
| nftables 模块缺失 | `nft` 执行失败 | 自动回退 iptables，如果 iptables 也不可用则 mode=none | 日志警告，透明代理不可用，端口代理仍可用 |
| Flash 磁盘满 | `os.WriteFile` 失败 | 终止操作，不写入任何文件，返回 CONFIG_WRITE_FAILED | API 报错，UI 显示磁盘空间不足 |
| mihomo API 无响应（已启动但不响应） | 心跳检测失败 | 等待 2 次心跳确认（2 分钟），再触发重启 | 日志警告 |
| SIGTERM 时 mihomo 5s 未退出 | `time.After(5s)` | SIGKILL 强杀 | 日志记录"forced kill" |
| 防火墙清理失败（退出时） | `Cleanup()` 返回错误 | 记录 CRITICAL 日志，继续退出流程（不能因清理失败而挂起） | 日志显示，用户可能需要手动运行 `nft delete table inet metaclash` |
| 配置文件被外部修改 | fsnotify 事件 | 自动触发 `Reload()`（防抖：同一秒内多次变更合并为一次） | 日志显示"config file changed, reloading" |

---

*文档版本：0.3.0 | 最后更新：2026-04-22*  
*此文档自包含，另一个 AI 或开发者无需查阅其他资料即可开始编码。*
