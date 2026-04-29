# 排障

排障时先保护网络可用性，再收集证据。建议先执行 `stop` 退出接管，然后再逐项定位。

## 快速恢复网络

如果开启透明代理或 DNS 后客户端无法联网：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop
```

路由器本机：

```sh
clashforgectl stop
```

然后检查 OpenWrt 自身网络、dnsmasq 和防火墙是否恢复。

## 启动失败

常见原因：

| 现象 | 可能原因 | 处理 |
| --- | --- | --- |
| Web UI 打不开 | 服务未启动或端口不可达 | 查看 `/etc/init.d/clashforge status` 和 `logread` |
| 反复重启 | 配置错误、PID 锁冲突、procd 参数问题 | 收集诊断报告并检查日志 |
| mihomo 启动失败 | YAML 无效、端口冲突、二进制路径错误 | 查看运行中配置和 mihomo 日志 |

命令：

```sh
/etc/init.d/clashforge status
logread | grep -i clashforge
logread | grep -i mihomo
ps | grep -E 'clashforge|mihomo'
```

## 端口冲突

检查监听端口：

```sh
netstat -lntup | grep -E '7777|7890|7891|7892|7893|7895|7874|9090|17890|17891|17892|17893|17895|17874|19090'
```

如果旧进程残留，先停止服务并确认只有一个 mihomo 实例。

## DNS 异常

表现：网页打不开、域名无法解析、部分客户端正常部分异常。

检查：

```sh
nslookup example.com 127.0.0.1
nslookup github.com 127.0.0.1
logread | grep -i dnsmasq
logread | grep -i clashforge
```

处理建议：

1. 先关闭 DNS 接管。
2. 确认 OpenWrt 原生 dnsmasq 可用。
3. 再开启 mihomo DNS。
4. 最后恢复 dnsmasq 协作模式。

## 透明代理不生效

检查 nftables：

```sh
nft list ruleset | grep -i clashforge
```

检查 iptables：

```sh
iptables-save | grep -i clashforge
```

处理建议：

1. 确认防火墙后端选择正确。
2. 检查 LAN 绕过规则是否过宽。
3. 检查客户端网关和 DNS 是否指向路由器。
4. 停止再重新开启接管，避免旧规则残留。

## 订阅更新失败

常见原因：

1. 路由器时间不正确导致 TLS 失败。
2. DNS 无法解析订阅域名。
3. 订阅服务限制 User-Agent。
4. 代理尚未可用，路由器直连无法访问。

处理：

```sh
date
nslookup github.com
logread | grep -i subscription
```

如供应商要求 User-Agent，请在订阅设置中配置。

## 生成诊断报告

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
```

报告应包含：

1. ClashForge 状态。
2. 服务和进程信息。
3. 网络、DNS、防火墙摘要。
4. 最近日志。
5. 配置和订阅的脱敏信息。

## 提交 Issue 前准备

请提供：

1. ClashForge 版本。
2. OpenWrt/Kwrt 版本和架构。
3. 安装方式：deploy、upgrade、Release IPK 或手动安装。
4. 脱敏诊断报告。
5. 复现步骤。
6. 是否开启透明代理或 DNS 接管。
