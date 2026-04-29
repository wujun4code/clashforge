# FAQ

## 我只想稳定用代理，不想折腾源码，怎么装最合适？

用 `upgrade` 路径最稳：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade
```

它会由你的电脑下载 IPK 再推送到路由器，通常比路由器本机下载更稳。

## 为什么第一次不建议直接开透明代理和 DNS？

因为这是影响面最大的两层。  
先验证内核与节点，再逐步接管，问题会更容易定位和回退。

## `deploy` 和 `upgrade` 到底怎么选？

| 命令 | 适合谁 |
| --- | --- |
| `upgrade` | 普通用户、稳定运维 |
| `deploy` | 开发者，本地源码验证 |

## Web UI 默认地址是什么？

```text
http://<router-ip>:7777
```

例如：

```text
http://192.168.20.1:7777
```

## Windows 侧最低要求是什么？

需要 `ssh` 和 `scp` 命令可用（OpenSSH Client）。

## 开了接管后网络异常，第一步该做什么？

先回退：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop
```

先恢复可用网络，再继续排障。

## 如何确认“真的生效”而不是“看起来正常”？

至少确认：

1. `status` 正常。
2. `check` 返回正常连通性和出口信息。
3. 你的实际客户端网络体验符合预期（速度、可达性、DNS）。

## 想升级到固定版本怎么做？

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Version v0.1.0
```

同理也可用这个方式回滚到历史版本。

## 如何安全卸载？

完全卸载：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 uninstall
```

卸载但保留配置：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 uninstall -KeepConfig
```

## 诊断报告可以直接发到 Issue 吗？

建议只发脱敏报告：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
```

因为原始报告可能包含订阅 URL、Token、内网地址等敏感信息。
