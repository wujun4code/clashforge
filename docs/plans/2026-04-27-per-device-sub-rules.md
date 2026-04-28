# Per-Device Sub Rules

## 背景

用户从订阅导入的 Clash/Mihomo 配置通常包含一套"顶层规则"（top rules），对路由器上所有设备一视同仁：

```yaml
rules:
  - RULE-SET,reject,REJECT
  - RULE-SET,google,🚀 节点选择
  - RULE-SET,chatGPT,🚀 节点选择
  - GEOIP,CN,🏯 全球直连
  - MATCH,🐟 漏网之鱼
```

实际场景中，用户需要为不同设备选择不同的代理节点：
- iPhone 走新加坡节点
- 工作电脑走美国节点
- IoT 设备全部直连

ClashForge 提供 **Per-Device Sub Rules**：用户只需为每个设备指定"用哪些节点"，系统自动生成正确的 Mihomo 配置。

---

## 核心设计原则

**只对"真正走代理"的规则做 per-device 拆分，REJECT / DIRECT 类规则全局共享。**

原因：
- `RULE-SET,reject,REJECT` — 广告、恶意域名，所有设备都应拒绝，无需拆分
- `GEOIP,CN,🏯 全球直连` — 国内流量，所有设备都直连，无需拆分
- `RULE-SET,google,🚀 节点选择` — 需要走代理，这里才需要 per-device 区分用哪个节点

---

## 工作原理

### Step 1：用户配置设备分组及其节点偏好

用户在 portal 上：
1. 创建设备分组（如 `iPhone`），指定设备 IP
2. 为该设备选择节点覆盖：`🚀 节点选择` → 只用 `[wujun-sg]`

### Step 2：ClashForge 创建影子 proxy group

为每个设备分组 × 每个被覆盖的 proxy group，生成一个影子 proxy group：

```yaml
proxy-groups:
  # 原有（来自订阅，保留）
  - name: 🚀 节点选择
    type: select
    proxies: [https-clearance.dota2gaming.win, wujun-sg, nocix-us-01]

  # ClashForge 生成的影子 proxy group
  - name: "iPhone - 🚀 节点选择"
    type: select
    proxies: [wujun-sg]                  # 用户为 iPhone 指定的节点

  - name: "Windows - 🚀 节点选择"
    type: select
    proxies: [https-clearance.dota2gaming.win]
```

### Step 3：ClashForge 扫描 top rules，自动展开 AND 规则

扫描所有 top rules，找出 policy 指向被覆盖 proxy group 的条目，为每个设备生成 AND 版本：

**原 top rule：**
```
RULE-SET,google,🚀 节点选择
```

**自动展开为：**
```
AND,((SRC-IP-CIDR,192.168.1.100/32),(RULE-SET,google)),iPhone - 🚀 节点选择
AND,((SRC-IP-CIDR,192.168.1.200/32),(RULE-SET,google)),Windows - 🚀 节点选择
```

### 最终生成的完整配置

```yaml
# ── 来自订阅，原样保留 ────────────────────────
proxies:
  - { name: wujun-sg, ... }
  - { name: https-clearance.dota2gaming.win, ... }
  - { name: nocix-us-01, ... }

proxy-groups:
  - name: 🚀 节点选择
    type: select
    proxies: [https-clearance.dota2gaming.win, wujun-sg, nocix-us-01]
  - name: 🏯 全球直连
    type: select
    proxies: [DIRECT, 🚀 节点选择]
  - name: 🐟 漏网之鱼
    type: select
    proxies: [🚀 节点选择, 🏯 全球直连]

  # ── ClashForge 生成的影子 proxy groups ──────
  - name: "iPhone - 🚀 节点选择"
    type: select
    proxies: [wujun-sg]
  - name: "Windows - 🚀 节点选择"
    type: select
    proxies: [https-clearance.dota2gaming.win]

rule-providers:                          # 原样保留
  reject: { ... }
  google: { ... }
  chatGPT: { ... }

rules:
  # ── 全局共享规则（REJECT 类，不拆分）────────
  - RULE-SET,reject,REJECT

  # ── ClashForge 生成的 per-device AND 规则 ──
  # iPhone (192.168.1.100)
  - AND,((SRC-IP-CIDR,192.168.1.100/32),(RULE-SET,google)),iPhone - 🚀 节点选择
  - AND,((SRC-IP-CIDR,192.168.1.100/32),(RULE-SET,chatGPT)),iPhone - 🚀 节点选择

  # Windows (192.168.1.200)
  - AND,((SRC-IP-CIDR,192.168.1.200/32),(RULE-SET,google)),Windows - 🚀 节点选择
  - AND,((SRC-IP-CIDR,192.168.1.200/32),(RULE-SET,chatGPT)),Windows - 🚀 节点选择

  # ── 原 top rules（原样保留，兜底）────────────
  - RULE-SET,google,🚀 节点选择
  - RULE-SET,chatGPT,🚀 节点选择
  - GEOIP,CN,🏯 全球直连
  - MATCH,🐟 漏网之鱼
```

---

## AND 规则展开逻辑

ClashForge 在生成配置时执行如下扫描：

1. 收集所有设备分组中"有节点覆盖"的 proxy group 名称集合，记为 `overridden_groups`
2. 遍历 top rules：
   - 若某条规则的 policy **在 `overridden_groups` 中** → 为每个设备展开 AND 规则
   - 否则（REJECT、DIRECT、未被覆盖的 proxy group）→ 原样保留，不拆分
3. AND 规则块插在第一条非全局规则之前（即 REJECT 之后）
4. 原 top rules 全部保留在末尾作为兜底

---

## Portal UI 设计

用户操作路径：

```
设备管理
  └─ 添加设备：IP、名称、归属分组

分组路由
  └─ 选择设备分组（如 iPhone）
  └─ 节点覆盖列表（来自订阅的 proxy groups）
     ├─ 🚀 节点选择  [覆盖 ✓]  → 可用节点: [wujun-sg ✓, nocix-us-01, ...]
     └─ 🏯 全球直连  [覆盖 ✗]  → 不覆盖，走全局
```

用户**不需要手动写 AND 规则**，ClashForge 自动根据 top rules 展开。

---

## 数据模型

```go
type DeviceGroup struct {
    ID       string             `json:"id"`
    Name     string             `json:"name"`
    Devices  []Device           `json:"devices"`
    Overrides []ProxyGroupOverride `json:"overrides"` // 节点覆盖配置
    Order    int                `json:"order"`
}

type Device struct {
    IP       string `json:"ip"`
    Prefix   int    `json:"prefix"` // CIDR prefix，单 IP 填 32
    Hostname string `json:"hostname,omitempty"`
}

type ProxyGroupOverride struct {
    OriginalGroup string   `json:"original_group"` // 订阅中的 proxy group 名
    Proxies       []string `json:"proxies"`         // 该设备使用的节点子集
}
```

### 持久化

存储在独立的 `device-groups.json`，与订阅数据分离，订阅更新时不覆盖。

---

## 约束与边界

1. **AND 规则仅 Mihomo 支持**：原版 Clash 不支持 `AND` 复合规则。

2. **引用校验**：`ProxyGroupOverride.OriginalGroup` 和 `Proxies` 中的节点名均来自订阅。订阅更新后若名称变化，需提示用户修复失效引用。

3. **MATCH 兜底不拆分**：`MATCH,🐟 漏网之鱼` 是全局兜底，保持共享。若某设备的所有代理流量都通过影子 proxy group 匹配，MATCH 行为一致不影响结果。

4. **设备 IP 稳定性**：依赖 IP 识别设备，建议路由器为关键设备绑定静态 IP，portal 在设备页面提示。

---

## 实现拆分

| 阶段 | 内容 |
|------|------|
| Phase 1 | 设备发现（读 ARP 表），设备命名与分组管理 API + UI |
| Phase 2 | 节点覆盖配置 API（DeviceGroup.Overrides 的增删改）+ UI |
| Phase 3 | 配置生成逻辑：影子 proxy group 生成 + AND 规则展开算法 |
| Phase 4 | 订阅更新后的引用校验与修复提示 |
