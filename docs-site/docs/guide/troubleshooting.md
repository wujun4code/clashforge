# 排障（先恢复网络，再找根因）

这页按“症状”组织，不按模块组织。  
你只要先识别症状，就能快速找到第一步动作。

## 0. 任何问题先做这一步

如果已经影响上网体验，先执行：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop
```

或：

```sh
clashforgectl stop
```

`stop` 的目标是先退出接管、恢复基础网络，再继续定位。

## 1. 症状：开启接管后全网异常

常见表现：

1. 客户端全部断网或大量超时。
2. 路由器后台也变慢或不可达。

处理顺序：

1. 先 `stop`。
2. 确认 OpenWrt 原生网络是否恢复。
3. 回到“只开内核，不开接管”的状态重新验证。

## 2. 症状：Web UI 打不开

检查：

```sh
/etc/init.d/clashforge status
ps | grep -E 'clashforge|mihomo'
logread | grep -i clashforge
```

再看端口：

```sh
netstat -lntup | grep -E '7777|7890|7891|7892|7893|7895|7874|9090'
```

如果服务在线但端口冲突，先停掉冲突进程再重启。

## 3. 症状：服务在线，但代理效果不对

先做快速验收：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 check
```

重点看：

1. 出口 IP 是否变化。
2. 目标站点是否可达。
3. 失败集中在代理层还是 DNS 层。

## 4. 症状：DNS 解析异常

检查命令：

```sh
nslookup github.com 127.0.0.1
logread | grep -i dnsmasq
logread | grep -i clashforge
```

建议动作：

1. 先关闭 DNS 接管。
2. 确认路由器原生 dnsmasq 正常。
3. 再逐步恢复 ClashForge DNS 能力。

## 5. 症状：透明代理规则没生效

```sh
nft list ruleset | grep -i clashforge
iptables-save | grep -i clashforge
```

如果看不到规则或重复堆叠：

1. `stop` 清理现场。
2. 重新开启透明代理。
3. 立即复验一次 `check`。

## 6. 症状：订阅更新失败

优先排查：

```sh
date
nslookup github.com
logread | grep -i subscription
```

常见根因：

1. 系统时间不准（TLS 失败）。
2. DNS 不通。
3. 供应商限制 User-Agent 或请求频率。

## 7. 一键收集证据

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
```

拿到报告后再提交 Issue，效率会高很多。

## 8. 提交 Issue 最少信息

1. ClashForge 版本。
2. OpenWrt 版本与架构。
3. 安装方式（`upgrade` / `deploy` / 手动 IPK）。
4. 是否开启透明代理和 DNS 接管。
5. 脱敏后的 `diag` 报告。
6. 可稳定复现的步骤。
