# 启动与接管

ClashForge 将“启动管理层”“启动 mihomo 内核”“接管透明代理/DNS”拆开处理。这样做的目的，是让首次部署更安全。

## 启动服务

在路由器上执行：

```sh
/etc/init.d/clashforge enable
/etc/init.d/clashforge start
/etc/init.d/clashforge status
```

从 Windows 远程查看状态：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 status
```

## Web UI 启动内核

打开：

```text
http://192.168.20.1:7777
```

然后按顺序操作：

1. 进入 Setup 或配置页面。
2. 导入配置或订阅。
3. 保存并激活配置。
4. 启动 mihomo 内核。
5. 在概览页确认 PID、运行时长、CPU、内存和连接数。

## 开启透明代理

确认节点可用后，再从 Web UI 的概览或设置页面开启透明代理接管。

建议检查：

| 检查项 | 期望结果 |
| --- | --- |
| nftables/iptables | 规则已应用，无报错 |
| 策略路由 | 相关 table/rule 存在 |
| LAN 绕过 | 路由器管理地址不受影响 |
| 出口 IP | 需要代理的客户端出口发生变化 |

## 开启 DNS 接管

DNS 接管应晚于节点验证。

建议检查：

```sh
nslookup example.com 127.0.0.1
logread | grep -i clashforge
```

::: tip 逐步接管
推荐先开启内核，再开启透明代理，最后开启 DNS。每一步都做一次 [检查清单](/guide/verify)，问题更容易定位。
:::

## 停止服务并退出接管

远程执行：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop
```

路由器本机执行：

```sh
clashforgectl stop
```

`stop` 会尝试停止服务并退出透明代理接管模式，包括恢复 dnsmasq、清理 netfilter 和策略路由。

## 重置为初始状态

保留已安装包版本，但清除运行数据、订阅、生成配置、缓存和日志：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 reset
```

重置后自动启动：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 reset -Start
```
