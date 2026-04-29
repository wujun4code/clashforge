# 验证是否成功

成功不是“安装命令没报错”，而是：目标设备能稳定访问需要代理的资源，国内和内网访问不受影响，并且出口符合预期。

## 验证总表

| 层级 | 怎么验证 | 成功表现 |
| --- | --- | --- |
| 管理服务 | 打开 Web UI | `http://<路由器IP>:7777` 正常显示 |
| mihomo 内核 | 概览页或 `status` | PID、运行时长、资源信息正常 |
| 配置来源 | 配置管理页 | 节点、规则、订阅更新记录可见 |
| 节点可用性 | 节点测速和连通检测 | 至少一个节点延迟正常 |
| 透明代理 | 设备访问海外网站 | 无需客户端即可访问 |
| DNS | 国内外域名解析 | 国内直连，海外按规则走代理 |
| 设备分流 | 不同设备查出口 IP | 工作设备和娱乐设备出口不同 |

## 1. Web UI 验证

打开：

```text
http://192.168.20.1:7777
```

成功表现：

| 页面 | 应看到什么 |
| --- | --- |
| 概览 | 核心状态、流量、连接、代理组 |
| 配置管理 | 配置文件、订阅、规则集 |
| 出口节点 | VPS/Worker 节点状态 |
| 设备分流 | 设备组和策略覆盖 |
| 活动日志 | mihomo 日志和连接记录 |

如果 Web UI 打不开，先看 [故障排查：管理页面打不开](/guide/troubleshooting#问题一管理页面打不开)。

## 2. 命令验证

Windows：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 status
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 check
```

macOS / Linux：

```sh
./scripts/clashforgectl --router 192.168.20.1 status
./scripts/clashforgectl --router 192.168.20.1 check
```

路由器本机临时执行：

```sh
cd /tmp
wget -O clashforgectl.sh https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/clashforgectl.sh
sh clashforgectl.sh status
sh clashforgectl.sh check
```

`check` 是轻量检查，适合确认服务、访问和出口是否大体正常。

## 3. 浏览器验证

在已经接入该路由器的设备上测试：

| 测试 | 期望 |
| --- | --- |
| 海外网站 | 能打开，速度合理 |
| 国内网站 | 能打开，不明显绕路 |
| 出口 IP 查询 | 国家/地区符合所选节点 |
| OpenAI/Claude/GitHub | 能正常访问或登录 |
| 内网地址 | 路由器、NAS、打印机等仍可访问 |

如果海外网站打不开，但国内正常，优先排查节点、规则和 DNS。
如果所有网站都打不开，先执行 `stop` 恢复网络。

## 4. 概览页双侧检测

概览页有两类检测：

| 检测 | 含义 |
| --- | --- |
| 路由器侧检测 | 从 OpenWrt 路由器发起请求，判断路由器到外网是否通 |
| 浏览器侧检测 | 从当前浏览器发起请求，判断当前设备实际出口 |

常见判断：

| 现象 | 可能原因 |
| --- | --- |
| 路由器侧成功，浏览器侧失败 | 设备还没有被接管，或浏览器网络不走路由器 |
| 路由器侧失败，浏览器侧也失败 | 节点、DNS 或路由器外网有问题 |
| 两侧出口 IP 不同 | 当前设备没有按预期进入同一策略 |
| 切换节点后浏览器侧没变 | 设备分流或透明代理未生效 |

## 5. 设备分流验证

如果你配置了设备分流，请至少测试两台设备。

示例：

| 设备 | 期望出口 |
| --- | --- |
| 运营电脑 | 美国 VPS |
| 手机 | 机场节点 |
| 电视 | 流媒体节点 |
| 国内办公电脑 | DIRECT |

验证方法：

1. 在每台设备上查询出口 IP。
2. 在 ClashForge 活动连接页查看目标连接。
3. 在规则搜索里检查关键域名命中规则。
4. 切换某个策略组节点，观察该设备出口是否变化。

如果设备出口不符合预期，优先确认设备 IP 是否变化。设备分流以源 IP/CIDR 命中，关键设备建议绑定 DHCP 静态租约。

## 6. DNS 验证

DNS 问题通常表现为网站打不开、打开很慢、国内外分流异常。

路由器上可检查：

```sh
nslookup example.com 127.0.0.1
logread | grep -i clashforge
logread | grep -i dns
```

Web UI 中可检查：

| 页面 | 检查 |
| --- | --- |
| 高级管理 | DNS 是否启用、模式是否正确 |
| 活动日志 | 是否持续出现 DNS 错误 |
| 规则集 | 目标域名是否命中预期规则 |

## 7. 什么情况算不成功

| 现象 | 优先处理 |
| --- | --- |
| Web UI 打不开 | 检查服务、端口、路由器 IP |
| UI 能开但节点为空 | 检查订阅链接、User-Agent、路由器网络 |
| 节点有但海外网站打不开 | 换节点、检查规则、检查 DNS |
| 国内网站变慢 | 检查直连规则和 GeoData |
| 某台设备出口不对 | 检查设备 IP、分流规则、策略覆盖 |
| 全家无法上网 | 立刻执行 `stop` |

## 8. 生成诊断报告

需要求助时，生成脱敏报告。

Windows：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
```

macOS / Linux：

```sh
./scripts/clashforgectl --router 192.168.20.1 diag --fetch --redact
```

脱敏报告会尽量隐藏订阅 token、密钥、内网敏感信息。公开求助时不要发送原始订阅链接、Cloudflare Token、SSH 私钥或账号密码。
