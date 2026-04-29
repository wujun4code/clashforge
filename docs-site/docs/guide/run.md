# 启动与接管（按风险分阶段）

把“服务启动”“内核启动”“透明代理”“DNS”拆开做，是为了让你每一步都可验证、可回退。  
最稳的顺序是：**先启动，再验证，再接管**。

## 阶段 1：确认服务可用

路由器本机：

```sh
/etc/init.d/clashforge enable
/etc/init.d/clashforge start
/etc/init.d/clashforge status
```

远程检查：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 status
```

通过标准：

1. 服务进程存在。
2. Web UI `http://192.168.20.1:7777` 可打开。

## 阶段 2：仅启动内核（先不接管）

在 UI 中完成：

1. 选择有效配置源。
2. 保存并激活。
3. 启动 mihomo 内核。

通过标准：

1. 内核启动成功，无持续重启。
2. `check` 能拿到连通性结果。

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 check
```

## 阶段 3：开启透明代理接管

只在阶段 2 稳定后再开透明代理。  
开启后立刻做一次验收（见 [检查清单](/guide/verify)）。

重点观察：

1. 客户端出口 IP 是否按预期变化。
2. 路由器管理地址是否仍可访问。
3. 日志里是否出现持续报错或规则重复注入。

## 阶段 4：开启 DNS 接管

DNS 是最容易影响全网体验的一层，建议最后开启。  
开启后至少验证：

```sh
nslookup github.com 127.0.0.1
logread | grep -i clashforge
```

## 一键回退（最重要）

如果任何阶段出现异常，先恢复网络再排障：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop
```

或在路由器上：

```sh
clashforgectl stop
```

`stop` 会尝试退出接管并恢复 DNS / netfilter / 策略路由基线。

## 重置到初始状态

当配置已经混乱、难以定位时再用 `reset`：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 reset
```

重置后自动启动：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 reset -Start
```

它会清理运行数据、订阅和缓存，但保留当前已安装包版本。
