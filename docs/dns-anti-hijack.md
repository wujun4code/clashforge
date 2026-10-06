# 防劫持 DNS 策略

先升级到包含本次改动的 ClashForge，再在设置向导选择“防劫持”，或在 ClashForge 的 `config.toml` 中设置：

```toml
[dns]
strategy = "anti-hijack"
```

策略字段属于 ClashForge；将 `strategy` 写进 Mihomo YAML 的 `dns` 部分不会选择此策略。

## 用户 YAML 的生效规则

DNS 解析配置的优先级为：`overrides.yaml` > 来源 YAML > ClashForge 生成的默认值。
可以自定义 `default-nameserver`、`proxy-server-nameserver`、`nameserver`、`fallback`、各类 DNS policy 和 Fake-IP 过滤列表。服务器列表替换原列表，避免已删除的上游继续参与查询；policy 按域名合并，已有国内分流默认保留。可以用 `nameserver-policy: null` 清除整个默认 policy。

ClashForge 继续管理 DNS 开关、监听端口、IPv6 开关、解析模式和 Fake-IP 地址池，因为它们必须与 DNS 接管和透明代理一致。TUN 仍强制 Fake-IP。防劫持模式还强制遵守路由规则、启用配置 hosts、关闭系统 hosts。

请修改来源配置或覆盖配置，再重新生成并应用。运行时 `mihomo-config.yaml` 会自动重新生成，直接编辑它不会持久生效。`/service` 展示生成结果，不能修复旧版本的覆盖逻辑。

## 防劫持模式的行为

- 引导和节点解析默认使用 DNS4EU 的 HTTPS IP 端点，通过 Mihomo 原生 `name-cert-verify` 校验证书域名。它们直连且不依赖代理；可通过 YAML 替换。Cloudflare / Google 在此线路无法直连，不再用于默认引导。
- 节点解析独立直连，不依赖尚未连接的代理。旧的自动探测 `dns.doh` 不会被当作新策略的可信节点上游；请在 YAML 显式选择。
- 网站默认使用配置中的加密 `fallback` 上游作为主解析，未指定出口的上游补充 Mihomo 原生 `#RULES`，已有代理组或 `#DIRECT` 选择保持不变。有 GeoSite 时保留国内、境外解析策略，允许用户分别指定可信服务；直连网站解析也遵循 policy。
- 国内网站和默认直连网站使用已验证可达的腾讯 DoH 保留国内 CDN 选择；它只负责普通网站，节点解析始终使用独立的 DNS4EU。可以在 YAML 替换国内网站的解析器。
- 不使用 ISP/DHCP DNS，不生成明文 primary 与加密 fallback 的竞速。没有 GeoSite 时仍用加密主解析。
- 所有最终合并后的 DNS 上游都必须是 DoH/DoT/DoQ，并验证证书。UDP DNS、`tcp://`、`system`、`dhcp://`、跳过证书验证会导致配置生成失败，保留原运行时文件；启动/重启不会应用这份错误配置。
- `default-nameserver` 必须是 IP 端点。引导和节点 DNS 不能使用 `#RULES` 或代理组，避免启动循环。网站 DNS 可以使用 `#RULES` 或显式代理组。
- 不再进行旧的明文节点 DNS 自动探测，也不自动加入新的解析商。DNS 上游不可达时不回退到明文。
- 已接管 dnsmasq 时，内核意外崩溃不会恢复运营商 DNS；恢复前 DNS 会暂时不可用。错误的严格模式配置也不会在程序启动时触发旧运行时配置的自动启动。

## 当前线路的配置

此前在这条 WAN 上测试到：Cloudflare / Google 常见 DNS IP 直连超时；DNS4EU 的 `unfiltered.joindns4.eu` 通过 `86.54.11.100`、`86.54.11.200` 直连 HTTPS 可用，证书验证正常，节点域名结果正确。

可参考 [DNS4EU 覆盖配置](../examples/dns-anti-hijack-dns4eu.yaml)。引导和节点直接连接解析器 IP，通过 `name-cert-verify=unfiltered.joindns4.eu` 保留证书域名校验；不跳过证书验证，也无需明文 DNS。普通域名的 HTTPS 解析器另通过配置 hosts 固定入口 IP，保留其域名与 SNI。网站境外 DNS 使用 `#RULES`，需要现有路由规则将对应 DNS IP 送入可用代理；`#RULES` 不会强制覆盖用户选择的 DIRECT。

2026-10-06 在目标路由器的独立 Mihomo 测试实例中，旧 Cloudflare / Google 直连预设复现解析超时和 Tokyo 连接失败；两台 DNS4EU 的 HTTPS 查询均返回 `20.18.48.221`，使用原生证书域名校验的独立节点 DNS 后，Tokyo AnyTLS 直连测试成功（首次测试 1210 ms，包含解析和建立连接）。这项测试没有修改原服务配置；它验证此线路的节点引导路径，不代表所有线路或所有网站都可达。

最终预设还以当前订阅和规则进行完整实例验证：Tokyo 直连延迟 87 ms，Google DNS 查询成功，境外 HTTPS 返回 204，百度通过国内 DoH 获得 `180.101.51.73` / `180.101.49.44` 并返回 HTTPS 200。测试实例使用独立端口和数据目录，随后已停止。

示例保留国内网站的流量分流，国内域名使用国内加密解析器，节点域名使用独立境外解析器。可以将 `geosite:cn` 和 `direct-nameserver` 替换为自己信任且验证可达的国内加密解析服务；该修改不会改变节点专用 DNS。入口 IP 固定后需要在服务商换地址时维护，不能保证永久可达。没有 GeoSite 时，应删除示例中的两条 geosite policy。

普通模式也支持此前验证的 TCP 引导配置：

```yaml
dns:
  default-nameserver:
    - tcp://86.54.11.100
    - tcp://86.54.11.200
  proxy-server-nameserver:
    - https://unfiltered.joindns4.eu/dns-query#DIRECT
```

TCP DNS 没有加密，不能用于严格防劫持模式。改成 TCP 并不构成对链路篡改的完整防护。

## 安全边界与社区实践

加密传输和证书验证保护客户端到解析器之间的链路，不能证明解析器返回的记录是真实记录。DoH 服务商仍然是信任对象；Fake-IP 也不验证真实 DNS 记录。

常见做法是可信加密上游、必要时通过代理访问、独立节点引导和禁止明文降级。如果不信任公共解析商，可在可信海外网络自建递归解析器，通过经过验证的 DoH/DoT 接口访问。签署了 DNSSEC 的域名还可以使用本地验证器核验签名；依赖公共服务商报告的验证结果不能防止该服务商自身作恶，未签名域名也不受此保护。本次实现没有增加本地 DNSSEC 验证器。

这项策略约束的是 ClashForge 管理的 DNS 路径；客户端自行访问外部 DNS、浏览器独立 DoH、显式停止接管等不属于自动保障范围。它不能承诺“彻底消灭所有 DNS 劫持”。

参考：

- [Mihomo DNS 配置与指定代理](https://wiki.metacubex.one/config/dns/)
- [Mihomo hosts 配置](https://wiki.metacubex.one/config/dns/hosts/)
- [RFC 8484 §9：DoH 的安全与 DNSSEC 边界](https://www.rfc-editor.org/rfc/rfc8484.html#section-9)
- [DNSCrypt FAQ：加密与 DNSSEC 的作用](https://dnscrypt.info/faq/)
- [DNS4EU 公共端点](https://www.joindns4.eu/for-public)
