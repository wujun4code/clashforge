# Bug Report & Fix：微信发送视频失败（发送中断）

**日期：** 2026-06-09
**环境：** ImmortalWrt 6.6.133 / ClashForge / mihomo v1.19.25 / tproxy + fake-ip 模式
**设备：** 路由器 192.168.6.1，问题设备 192.168.6.151（小米手机）
**状态：** 已修复 ✅

---

## 一、问题描述

用户反映微信发送视频消息时，界面显示「发送中断」，上传失败。其他请求（访问 Google、刷微信朋友圈图片等）均正常，问题仅复现于微信视频发送场景。

---

## 二、调查过程

### 2.1 规则层核查

通过 Clash API (`GET /rules`, `GET /proxies`) 确认路由规则配置正常：

- 微信视频 CDN 域名 `c2c.cdn.weixin.qq.com` 命中 `GEOSITE,CN → 全球直连[DIRECT]` ✅
- `全球直连` 代理组当前选中 `DIRECT` ✅
- 无任何腾讯系域名出现在 `proxy` 或 `gfw` 规则集中 ✅

**结论：路由规则没有问题，流量走向正确。**

### 2.2 实时日志抓取

开启 info 级别日志，在用户发送视频期间抓取，观察到以下异常现象：

```
[TCP] 192.168.6.151:52680 --> c2c.cdn.weixin.qq.com:443  DIRECT
[TCP] 192.168.6.151:52676 --> c2c.cdn.weixin.qq.com:443  DIRECT
[TCP] 192.168.6.151:46340 --> c2c.cdn.weixin.qq.com:443  DIRECT
[TCP] 192.168.6.151:46326 --> c2c.cdn.weixin.qq.com:443  DIRECT
... （共 8 对，约 4 秒一次）
```

规则命中正确，但微信在 30 秒内对同一 CDN 域名发起了 **16 次连接（8 对），呈典型的上传失败重试风暴**。

### 2.3 确认服务器可达性

从路由器直接 `curl https://c2c.cdn.weixin.qq.com`，TLS 握手完全正常，排除服务器侧不可达。

### 2.4 DNS 层核查

```bash
nslookup c2c.cdn.weixin.qq.com 127.0.0.1
# 返回：198.18.3.45（fake-ip 假 IP）
```

确认该域名在 fake-ip 模式下被分配假 IP，而非真实 CDN IP。

### 2.5 nftables 规则核查

```
chain tproxy_prerouting {
    ip daddr @bypass_ipv4 return    ← 仅包含私有/保留 IP 段，不含国内 IP
    udp dport 443 return            ← 所有 QUIC 流量绕过 tproxy
    meta l4proto { tcp, udp } tproxy ip to 127.0.0.1:7895
}
```

发现 `udp dport 443 return` 规则：所有 UDP 443（QUIC）流量完全绕过 tproxy，与 fake-ip 机制存在结构性冲突。

---

## 三、根因分析

本次问题由**两个独立根因叠加**导致。

---

### 根因 1：fake-ip + QUIC bypass 导致初始连接失败

**触发链：**

```
手机 DNS 查询 c2c.cdn.weixin.qq.com
    ↓
Clash fake-ip DNS 返回 198.18.3.45（假 IP）
    ↓
微信尝试 QUIC（UDP 443）连接 198.18.3.45
    ↓
nftables: udp dport 443 return → 绕过 tproxy
    ↓
数据包以 dst=198.18.3.45 转发至上游路由器 → 公网 → 丢包
    ↓
微信 QUIC 探测失败 → 触发重试风暴
```

`bypass_ipv4` 集合仅包含私有/保留 IP 段，不含国内 IP，故腾讯系域名全部经过 fake-ip 机制。当 QUIC 包目标为假 IP 时，绕过 tproxy 后无处可达。

**影响范围：** 所有使用 fake-ip 解析、且应用层会尝试 QUIC 的国内域名均可能触发。

---

### 根因 2：`override-destination: true` 导致上传鉴权失败

`overrides.yaml` 中 sniffer 配置了 `override-destination: true`。

**触发链：**

```
微信通过 dns.weixin.qq.com.cn（微信自有 HTTP DNS）解析，
得到指定 CDN 节点 A：43.175.129.34
    ↓
手机连接 43.175.129.34:443
    ↓
tproxy → Clash sniffer 读取 TLS SNI: c2c.cdn.weixin.qq.com
    ↓
override-destination: true → Clash 重新解析该域名
    ↓
Clash 自身 DNS 因轮询返回不同节点 B：43.175.129.40
    ↓
Clash 出站连接发往 43.175.129.40（而非手机预期的 .34）
    ↓
微信上传 token 绑定 .34 节点 → .40 节点鉴权失败 → 发送中断
```

**关键背景——rule provider 与 fake-ip 是两个独立系统：**

| 系统 | 职责 |
|------|------|
| Rule Provider | 流量进入 Clash 后走哪条路（DIRECT / 代理节点） |
| fake-ip DNS | 客户端 DNS 查询时返回什么 IP（用于将流量引入 tproxy） |

即使 `qq.com` 在规则里正确命中 `GEOSITE,CN → DIRECT`，DNS 查询照样会返回假 IP——规则层和 DNS 层各自独立，问题出在 Clash 建立出站连接时的「目标改写」环节：

```
规则命中：正确（→ DIRECT）
DNS 机制：正确（返回假 IP，引流进 Clash）
出站连接：错误（override-destination 改写了 CDN 目标节点）
```

**`override-destination` 的设计意图 vs 副作用：**

- **设计意图：** 解决 fake-ip 模式下出站连接需要真实 IP 的问题（将假 IP 映射回域名再解析）
- **副作用：** 对已持有真实 IP 的连接（如经微信自有 HTTP DNS 解析的连接），重新解析会因 DNS 轮询选到不同 CDN 节点，破坏节点亲和性

---

## 四、修复方案

修改文件：`/etc/metaclash/overrides.yaml`

### Fix 1：腾讯系域名加入 fake-ip-filter

让这些域名的 DNS 查询直接返回真实 IP，使 QUIC 流量目标变为真实可达的 IP。`udp dport 443 return` 绕过 tproxy 后，流量可正常直连，根因 1 消除。

```yaml
dns:
  fake-ip-filter:
    - +.lan
    - +.local
    - time.*.com
    - ntp.*.com
    - +.ntp.org
    # 腾讯系（新增）
    - +.qq.com
    - +.qpic.cn
    - +.qlogo.cn
    - +.myqcloud.com
    - +.qcloud.com
    - +.tencent.com
    - +.wechat.com
    - +.weixin.com
    - +.tencentcs.com
    - +.gtimg.com
    - +.tim.qq.com
    - +.weiyun.com
```

### Fix 2：关闭 TLS/QUIC 的 override-destination

Clash 读取 SNI 仅用于规则匹配，出站连接保留手机原始选定的 CDN 节点 IP，不再重新解析，根因 2 消除。

```yaml
sniffer:
  enable: true
  override-destination: false        # 全局关闭（mihomo 官方默认值即为 false）
  sniff:
    QUIC:
      ports:
        - 443
    TLS:
      ports:
        - 443
        - "8443"
    HTTP:
      ports:
        - 80
        - 8080-8880
      override-destination: true     # HTTP 明文仍保留，用于 Host 透明代理
  parse-pure-ip: true
```

---

## 五、产品默认配置建议

### 5.1 需要调整的默认项

| 项目 | 当前默认 | 建议 | 原因 |
|------|---------|------|------|
| `sniffer.override-destination` | `true` | `false` | mihomo 官方默认值即为 false；设为 true 在国内 CDN 场景下破坏节点亲和性 |
| `dns.fake-ip-filter` 腾讯系域名 | 未包含 | 补充（见上） | 微信/QQ/腾讯云是国内最高频 app，应纳入默认白名单 |
| `dns.fake-ip-filter` 其他高频国内 app | 未评估 | 建议补充 | 参考：`+.taobao.com`、`+.alipay.com`、`+.baidu.com`、`+.bytedance.com` 等 |

### 5.2 关于 `udp dport 443 return` 规则

该 nftables 规则本身设计合理（避免 UDP tproxy 的状态跟踪复杂性），但与 fake-ip 存在结构性张力：

- fake-ip 模式下，客户端拿到假 IP，QUIC 包目标是假 IP，bypass 后无处可达
- 若 fake-ip-filter 完整覆盖了所有应用域名，客户端持有真实 IP，bypass 后可直连

**结论：该规则本身无需修改，正确解法是确保 fake-ip-filter 完整覆盖国内主流 app 域名。**

---

## 六、验证结果

| 验证项 | 结果 |
|--------|------|
| 微信发送视频 | ✅ 成功 |
| 微信浏览图片/文字 | ✅ 正常 |
| Google 等代理流量 | ✅ 正常 |
| 日志中不再出现重试风暴 | ✅ 确认 |
| `c2c.cdn.weixin.qq.com` DNS 返回真实 IP | ✅ 已验证 |

---

## 七、附录

### 完整修复后的 overrides.yaml

```yaml
log-level: info
bind-address: "*"
keep-alive-interval: 15
keep-alive-idle: 600
ipv6: false
tcp-concurrent: true
unified-delay: true
experimental:
  quic-go-disable-gso: true
sniffer:
  enable: true
  override-destination: false
  sniff:
    QUIC:
      ports:
        - 443
    TLS:
      ports:
        - 443
        - "8443"
    HTTP:
      ports:
        - 80
        - 8080-8880
      override-destination: true
  parse-pure-ip: true
profile:
  store-selected: true
  store-fake-ip: true
ntp:
  enable: true
  server: time.apple.com
  port: 123
  interval: 30
  write-to-system: true
dns:
  fake-ip-filter:
    - +.lan
    - +.local
    - time.*.com
    - ntp.*.com
    - +.ntp.org
    - +.qq.com
    - +.qpic.cn
    - +.qlogo.cn
    - +.myqcloud.com
    - +.qcloud.com
    - +.tencent.com
    - +.wechat.com
    - +.weixin.com
    - +.tencentcs.com
    - +.gtimg.com
    - +.tim.qq.com
    - +.weiyun.com
```

### 关键域名说明

| 域名 | 用途 |
|------|------|
| `+.qq.com` | 微信/QQ 核心服务及所有子域名 |
| `+.qpic.cn` | 微信图片 CDN（mmbiz.qpic.cn 等） |
| `+.qlogo.cn` | 微信头像 CDN（wx.qlogo.cn 等） |
| `+.myqcloud.com` | 腾讯云对象存储 COS，视频上传主要端点 |
| `+.qcloud.com` | 腾讯云控制面域名 |
| `+.tencent.com` | 腾讯主域 |
| `+.gtimg.com` | 腾讯图片/静态资源 CDN |
| `+.weiyun.com` | 微云存储 |
