# DNS 分流策略 — PRD v0.1

> 状态：草稿 | 作者：Wu Jun | 日期：2026-05-27

---

## 一、背景与动机

### 现状问题

ClashForge 当前的 DNS 方案是**事后补救型**：

```
所有域名 → nameserver（ISP DNS / dhcp://eth1）先查
         → 看返回 IP 的 GeoIP
         → GeoIP ≠ CN → 触发 fallback（DoH 重查）
```

这套 `fallback-filter` 方案有一个无法规避的漏洞：**如果 ISP DNS 对 `google.com` 返回一个 GeoIP = CN 的污染 IP，Mihomo 会认为它是国内域名而直接使用**，既造成 DNS 泄露（查询明文发给了 ISP），又导致连接失败。

### 目标

引入**查询前分流**机制：在把域名发给任何 DNS 服务器之前，先用 geosite 数据判断它属于哪一类，再决定问哪个 DNS。ISP 对国际域名永远没有机会插手。

---

## 二、用户故事

| # | 用户类型 | 需求 | 验收标准 |
|---|---|---|---|
| U1 | 普通用户 | 开箱即用，国内外都能正常访问 | 默认配置下百度/Google/YouTube 均可访问 |
| U2 | 隐私敏感用户 | 不想让 ISP 看到我访问了哪些国际网站 | DNS 泄露检测显示上游为 Google/Cloudflare，无 ISP DNS |
| U3 | 国内 CDN 优化用户 | 国内网站要用最近的 CDN 节点 | taobao.com / bilibili.com 解析到国内 CDN IP |
| U4 | 现有用户 | 升级后原有功能不受影响 | 升级后默认行为等价于当前方案 A，可手动切换到方案 C |

---

## 三、产品方案

### 3.1 DNS 策略枚举

新增 `dns_strategy` 字段，取值：

| 值 | 名称 | 描述 |
|---|---|---|
| `"legacy"` | 事后补救（当前默认）| 保持现有 fallback-filter 行为，不加 nameserver-policy |
| `"split"` | **分流优先（推荐）** | 加 nameserver-policy：国内走 CN DNS，国际走 DoH，未知走 fallback-filter 兜底 |
| `"privacy"` | 全链路加密 | nameserver 也换成 CN DoH，全面规避 ISP，牺牲少量国内 CDN 最优路由 |

### 3.2 各策略生成的 Mihomo DNS 配置对比

#### `"legacy"`（现状，不改动）

```yaml
nameserver:
  - dhcp://eth1           # 或 PPP DNS
fallback:
  - tls://8.8.4.4
  - tls://1.1.1.1
  - https://dns.google/dns-query
  - https://cloudflare-dns.com/dns-query
fallback-filter:
  geoip: true
  geoip-code: CN
  ipcidr: [240.0.0.0/4]
```

#### `"split"`（新增，推荐默认）

```yaml
# nameserver 处理 nameserver-policy 未覆盖的剩余域名
nameserver:
  - dhcp://eth1           # 或 PPP DNS

# 国内域名 → ISP DNS / CN 纯 IP（最优 CDN）
# 国际域名 → 国际 DoH（ISP 无法截获）
nameserver-policy:
  "geosite:cn":
    - 223.5.5.5
    - 119.29.29.29
  "geosite:geolocation-!cn":
    - https://dns.google/dns-query
    - https://cloudflare-dns.com/dns-query

# 安全网：兜底未分类域名中的污染情况
fallback:
  - tls://8.8.4.4
  - tls://1.1.1.1
  - https://dns.google/dns-query
  - https://cloudflare-dns.com/dns-query
fallback-filter:
  geoip: true
  geoip-code: CN
  ipcidr: [240.0.0.0/4, 0.0.0.0/8]
```

#### `"privacy"`（新增，隐私最大化）

```yaml
# nameserver 本身也换成 CN DoH，ISP 全程看不到任何 DNS 查询
nameserver:
  - https://dns.alidns.com/dns-query
  - https://doh.pub/dns-query

nameserver-policy:
  "geosite:cn":
    - https://dns.alidns.com/dns-query
    - https://doh.pub/dns-query
  "geosite:geolocation-!cn":
    - https://dns.google/dns-query
    - https://cloudflare-dns.com/dns-query

fallback:
  - https://dns.google/dns-query
  - https://cloudflare-dns.com/dns-query
fallback-filter:
  geoip: true
  geoip-code: CN
  ipcidr: [240.0.0.0/4, 0.0.0.0/8]
```

### 3.3 策略切换前提条件

- `"split"` 和 `"privacy"` 都需要 geosite.dat 已下载（`/usr/share/metaclash/geosite.dat` 存在）。
- 若 geosite.dat 不存在，`nameserver-policy` 不写入，降级到 `"legacy"` 行为，并在 UI 提示。

---

## 四、技术设计

### 4.1 后端

#### 4.1.1 `internal/config/types.go` — 新增字段

```go
type DNSConfig struct {
    // 现有字段保持不变
    Enable       bool     `toml:"enable"         json:"enable"`
    Mode         string   `toml:"mode"           json:"mode"`
    IPv6         bool     `toml:"ipv6"           json:"ipv6"`
    Nameservers  []string `toml:"nameservers"    json:"nameservers"`
    Fallback     []string `toml:"fallback"       json:"fallback"`
    DoH          []string `toml:"doh"            json:"doh"`
    FakeIPFilter []string `toml:"fake_ip_filter" json:"fake_ip_filter"`
    DnsmasqMode  string   `toml:"dnsmasq_mode"   json:"dnsmasq_mode"`
    ApplyOnStart bool     `toml:"apply_on_start" json:"apply_on_start"`

    // 新增
    // Strategy 控制 nameserver-policy 的生成行为。
    // 合法值: "legacy" | "split" | "privacy"
    // 空值等价于 "legacy"（向后兼容）。
    Strategy string `toml:"strategy" json:"strategy"`
}
```

默认值（`Default()` 函数）：`Strategy: "split"`（新安装用最优方案，存量用户升级不变）

> **升级策略**：已有 TOML 配置文件中没有 `strategy` 字段 → 读取后为空字符串 → 代码中 `""` 等价于 `"legacy"` → 存量用户行为不变。

#### 4.1.2 `internal/config/generator.go` — `buildDNSMap` 改动

在现有 fallback 逻辑之后，追加 nameserver-policy 生成逻辑：

```go
// buildNameserverPolicy returns the nameserver-policy map for "split" / "privacy"
// strategies, or nil if the strategy is "legacy" / empty.
// geositeAvailable must be checked by the caller before calling this.
func buildNameserverPolicy(cfg *MetaclashConfig, bootstrapIPs []string) map[string]interface{} {
    strategy := strings.ToLower(strings.TrimSpace(cfg.DNS.Strategy))
    if strategy != "split" && strategy != "privacy" {
        return nil
    }

    intlDNS := []string{
        "https://dns.google/dns-query",
        "https://cloudflare-dns.com/dns-query",
    }
    // User-configured fallback takes precedence if it contains DoH entries.
    var userDoH []string
    for _, f := range cfg.DNS.Fallback {
        if strings.HasPrefix(f, "https://") || strings.HasPrefix(f, "tls://") {
            userDoH = append(userDoH, f)
        }
    }
    if len(userDoH) > 0 {
        intlDNS = userDoH
    }

    var cnDNS []string
    if strategy == "privacy" {
        // CN DoH — ISP cannot observe any query
        cnDNS = []string{
            "https://dns.alidns.com/dns-query",
            "https://doh.pub/dns-query",
        }
    } else {
        // "split" — CN domains get ISP-optimal CDN IPs via pure-IP resolvers
        cnDNS = bootstrapIPs // 223.5.5.5, 119.29.29.29 等
        if len(cnDNS) == 0 {
            cnDNS = []string{"223.5.5.5", "119.29.29.29"}
        }
    }

    return map[string]interface{}{
        "geosite:cn":                cnDNS,
        "geosite:geolocation-!cn":   intlDNS,
    }
}
```

在 `buildDNSMap` 末尾调用：

```go
// nameserver-policy（仅 split / privacy 策略）
if policy := buildNameserverPolicy(cfg, bootstrapIPs); policy != nil {
    if geositeExists(cfg.Core.GeositePath) {
        dnsMap["nameserver-policy"] = policy
        // privacy 模式下 nameserver 也换成 CN DoH
        if strings.ToLower(cfg.DNS.Strategy) == "privacy" {
            dnsMap["nameserver"] = []string{
                "https://dns.alidns.com/dns-query",
                "https://doh.pub/dns-query",
            }
        }
    } else {
        log.Warn().
            Str("geosite_path", cfg.Core.GeositePath).
            Str("strategy", cfg.DNS.Strategy).
            Msg("config: geosite.dat 不存在，nameserver-policy 降级到 legacy 模式")
    }
}
```

`geositeExists` 是一个简单的文件存在检查：

```go
func geositeExists(path string) bool {
    _, err := os.Stat(path)
    return err == nil
}
```

#### 4.1.3 API 层

`GET /config` 和 `POST /config` 已经透传整个 `DNSConfig` 结构体，无需改动 handler。
新字段 `strategy` 自动包含在 JSON 响应里。

#### 4.1.4 新增辅助日志

在 `buildDNSMap` 末尾补充日志：

```go
log.Info().
    Str("strategy", cfg.DNS.Strategy).
    Bool("nameserver_policy_active", policyApplied).
    Msg("config: DNS 策略")
```

### 4.2 前端

#### 4.2.1 `ui/src/api/client.ts`

在 `ClashForgeConfig.dns` 类型里新增：

```ts
dns: {
  enable: boolean; mode: string; dnsmasq_mode: string; apply_on_start: boolean
  listen: string; ipv6: boolean
  nameservers: string[]; fallback: string[]; doh: string[]; fake_ip_filter: string[]
  strategy: 'legacy' | 'split' | 'privacy'   // ← 新增
}
```

#### 4.2.2 Setup 向导 DNS 步骤（`ui/src/pages/Setup.tsx`）

在现有的 DNS 设置 UI 中，`mode`（fake-ip / redir-host）选择器下方，增加 **策略选择器**：

```
DNS 分流策略
┌─────────────────────────────────────────────────────────────┐
│ ○ 分流优先（推荐）                                           │
│   国内域名走 ISP DNS，国际域名走加密 DoH，防 DNS 污染和泄露  │
│                                                             │
│ ○ 全链路加密                                                │
│   所有查询走 DoH，ISP 完全不可见，国内 CDN 路由略有损耗      │
│                                                             │
│ ○ 传统模式                                                  │
│   保持原有 fallback-filter 行为（兼容旧配置）                │
└─────────────────────────────────────────────────────────────┘
```

UI 规则：
- 默认选中「分流优先」
- 若 geosite 状态 API 返回未下载，「分流优先」和「全链路加密」显示 ⚠️ 提示：*需要 GeoData · 请先在 GeoData 页面下载*，但不禁用（允许选择，保存后若 geosite 存在则生效）
- 选择「分流优先」或「全链路加密」时，在预览框里高亮 `nameserver-policy` 段落

#### 4.2.3 DNS 字段标签（`DNS_FIELD_LABELS`）

```ts
const DNS_FIELD_LABELS: Record<string, string> = {
  // 现有...
  'nameserver-policy': '分流策略：国内/国际域名分别问不同 DNS',
}
```

#### 4.2.4 设置页面（如有独立 DNS 设置页）

同向导，增加相同的策略三选一控件。

---

## 五、数据模型变更总结

| 层 | 文件 | 变更类型 | 描述 |
|---|---|---|---|
| 配置结构 | `internal/config/types.go` | 新增字段 | `DNSConfig.Strategy string` |
| 默认值 | `internal/config/types.go` | 修改 | 新安装默认 `Strategy: "split"` |
| 生成逻辑 | `internal/config/generator.go` | 新增函数 + 修改 | `buildNameserverPolicy()` + `buildDNSMap()` 末尾调用 |
| API 类型 | `ui/src/api/client.ts` | 新增字段 | `dns.strategy` |
| 向导 UI | `ui/src/pages/Setup.tsx` | 新增控件 | 策略三选一 radio group |
| 标签映射 | `ui/src/pages/Setup.tsx` | 新增 | `DNS_FIELD_LABELS['nameserver-policy']` |

---

## 六、升级兼容性

| 场景 | 行为 |
|---|---|
| 全新安装 | `Strategy = "split"`，nameserver-policy 自动生成（geosite 存在时）|
| 现有用户升级 | TOML 无 `strategy` 字段 → 读到空字符串 → 等价 `"legacy"` → 行为与升级前完全一致 |
| 现有用户手动切换 | 在向导或设置页选择「分流优先」→ 保存 → 重新生成配置 → 重启 Mihomo |
| geosite 未下载 | 选了 `split`/`privacy` 但文件不存在 → 降级 `legacy` + 日志警告 + UI 提示 |

---

## 七、测试要求

### 7.1 单元测试（Go）

新增 `TestBuildDNSMap_SplitStrategy` 和 `TestBuildDNSMap_PrivacyStrategy`：

```go
// 验证 split 策略生成的 nameserver-policy 包含 geosite:cn 和 geosite:geolocation-!cn
// 验证 split 策略的 nameserver 仍是 dhcp://eth1（不替换）
// 验证 privacy 策略的 nameserver 被替换为 CN DoH
// 验证 legacy / 空字符串策略不生成 nameserver-policy
// 验证 geosite 文件不存在时不生成 nameserver-policy（mock os.Stat）
```

### 7.2 集成测试

| 场景 | 验证方法 |
|---|---|
| `split` 策略下 `google.com` 走国际 DoH | DNS 泄露检测工具显示上游为 8.8.8.8 / 1.1.1.1 |
| `split` 策略下 `baidu.com` 走 ISP DNS | 解析到国内 CDN IP（GeoIP = CN）|
| `privacy` 策略下 ISP 看不到任何 DNS | 抓包验证路由器对外无 UDP/53 流量 |
| 升级后存量用户行为不变 | 对比升级前后 Mihomo 配置 diff |

### 7.3 手动验收

1. 全新安装 → 向导 DNS 步骤默认选中「分流优先」✓
2. 选「分流优先」→ 配置预览出现 `nameserver-policy` 段落 ✓
3. geosite 未下载 → 预览显示降级警告 ✓
4. 存量用户升级 → DNS 步骤显示当前为「传统模式」✓

---

## 八、不在本期范围内

- 自定义 `nameserver-policy` 规则（用户自己填 domain → DNS 映射）
- 每个策略的 nameserver 列表 UI 编辑（沿用现有 Nameservers / Fallback 字段）
- `respect-rules: true` 联动（让 DNS 解析也走代理规则）— 另行评估

---

## 九、里程碑

| 阶段 | 内容 | 估时 |
|---|---|---|
| M1 | 后端：`types.go` 新增字段 + `buildDNSMap` 逻辑 + 单元测试 | 0.5 天 |
| M2 | 前端：API 类型 + Setup 向导策略控件 + 预览高亮 | 0.5 天 |
| M3 | 集成测试 + 文档更新（README DNS 章节）| 0.5 天 |
| **合计** | | **1.5 天** |
