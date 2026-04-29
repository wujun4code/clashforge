# 检查清单

每次安装、升级、修改配置或开启接管后，都建议按本页顺序检查。

## 1. 服务状态

Windows 远程：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 status
```

路由器本机：

```sh
/etc/init.d/clashforge status
ps | grep clashforge
```

期望结果：

| 项目 | 正常表现 |
| --- | --- |
| ClashForge 服务 | 正在运行 |
| Web UI | `http://<router-ip>:7777` 可打开 |
| mihomo 内核 | 配置完成后可启动 |
| 日志 | 没有持续 crash loop 或端口冲突 |

## 2. Web UI 与 API

浏览器访问：

```text
http://192.168.20.1:7777
```

如需在路由器上用 curl 检查，可按实际 API 路径验证健康、状态和版本接口。

```sh
curl -s http://127.0.0.1:7777/api/v1/health
curl -s http://127.0.0.1:7777/api/v1/status
curl -s http://127.0.0.1:7777/api/v1/version
```

如果接口路径随版本变化，请以 Web UI 网络请求或 API 路由为准。

## 3. 轻量连通性检查

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 check
```

路由器本机：

```sh
clashforgectl check
```

重点看：

1. 目标网站是否可达。
2. 路由器侧出口 IP 是否符合预期。
3. DNS 是否能解析。
4. 代理开启后客户端出口是否发生变化。

## 4. DNS 检查

```sh
nslookup example.com 127.0.0.1
nslookup github.com 127.0.0.1
logread | grep -i dns
```

如果使用 fake-ip，还要确认客户端不会直接访问 fake-ip 段导致异常。

## 5. netfilter 检查

nftables：

```sh
nft list ruleset | grep -i clashforge
```

iptables：

```sh
iptables-save | grep -i clashforge
```

期望结果：规则存在、链顺序正确、没有重复堆叠的旧规则。

## 6. 进程与端口检查

```sh
ps | grep -E 'clashforge|mihomo'
netstat -lntup | grep -E '7777|7890|7891|7892|7893|7895|7874|9090'
```

如果默认 Clash 端口被占用，ClashForge 会优先使用社区默认端口，冲突时再回退到共存端口。

## 7. 诊断报告

收集脱敏报告并下载到本机：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
```

指定保存路径：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -LocalPath .\cf-diag.txt -Redact
```

::: warning 未脱敏报告
不加 `-Redact` 的诊断报告可能包含订阅 URL、Token 或其他敏感信息。
:::
