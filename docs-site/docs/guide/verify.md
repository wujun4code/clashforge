# 检查清单（以用户体验为验收标准）

安装、升级、改配置、开启接管后，都建议跑一遍这份清单。  
目标不是“命令执行成功”，而是“网络体验符合预期”。

## 一次验收看这 5 项

| 检查项 | 命令/动作 | 通过标准 | 失败先做什么 |
| --- | --- | --- | --- |
| 服务状态 | `status` | 服务在线，UI 可访问 | 执行 `stop`，再看日志 |
| 连通性 | `check` | 常用站点可达、出口信息正常 | 暂停接管，回到内核验证 |
| DNS | `nslookup` | 常用域名解析稳定 | 先关闭 DNS 接管 |
| 防火墙接管 | `nft`/`iptables` | 规则存在且不重复堆叠 | `stop` 后重新接管 |
| 诊断留档 | `diag -Fetch -Redact` | 报告可生成并下载 | 用报告做后续定位 |

## 1. 服务与 UI

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 status
```

手动访问：

```text
http://192.168.20.1:7777
```

如果 status 正常但 UI 打不开，优先排查端口占用和防火墙策略。

## 2. 连通性与出口

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 check
```

看这三件事：

1. 常见站点探测是否成功。
2. 出口 IP 是否符合你当前策略。
3. 失败是否集中在 DNS、代理端口或目标站点。

## 3. DNS 验收

```sh
nslookup github.com 127.0.0.1
nslookup example.com 127.0.0.1
logread | grep -i dns
```

现象判断：

1. 全部解析慢或失败：先关 DNS 接管。
2. 部分域名异常：检查规则或上游 DNS。
3. 开关接管后表现差异明显：重点看 dnsmasq 协作模式。

## 4. 接管规则验收

nftables：

```sh
nft list ruleset | grep -i clashforge
```

iptables：

```sh
iptables-save | grep -i clashforge
```

预期：规则链存在、顺序合理、没有多次重复注入。

## 5. 诊断报告（用于复盘与求助）

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
```

指定保存路径：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -LocalPath .\cf-diag.txt -Redact
```

::: warning 别上传未脱敏报告
不带 `-Redact` 的报告可能包含订阅 URL、Token 或内网信息。
:::
