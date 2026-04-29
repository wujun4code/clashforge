# 不好用怎么办

排查问题时先记住一句话：**先恢复上网，再找原因**。

如果已经影响家里设备上网，不要先改更多设置，先执行恢复命令。

## 先恢复网络

Windows：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop
```

路由器本机：

```sh
clashforgectl stop
```

恢复后再慢慢判断是哪一环出问题。

## 管理页面打不开

先确认地址是否正确：

```text
http://192.168.20.1:7777
```

再检查服务：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 status
```

常见原因：

1. 路由器地址写错。
2. ClashForge 没有启动。
3. 电脑和路由器不在同一个网络。
4. 端口被其他程序占用。

## 添加订阅后没有节点

可能原因：

1. 订阅链接复制错了。
2. 订阅需要特殊 User-Agent。
3. 路由器无法访问订阅地址。
4. 订阅本身已经过期或不可用。

建议：

1. 在浏览器里确认订阅服务仍然可用。
2. 重新复制订阅链接。
3. 在 ClashForge 里重新更新订阅。

## 节点有，但网站打不开

先换一个节点试试。  
如果换节点后正常，说明原节点可能不可用。

如果所有节点都不行：

1. 确认订阅没有过期。
2. 确认路由器时间正确。
3. 执行一次检查：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 check
```

## 打开开关后全家网络异常

先恢复：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop
```

恢复后按这个顺序重新尝试：

1. 只启动代理服务。
2. 只用一台设备测试。
3. 确认没问题后，再让更多设备使用。

## 有些网站走代理，有些不走

这通常和你的订阅规则有关。  
你可以先切换节点、更新订阅，确认是否是服务商规则变化。

如果你熟悉 Clash 规则，再考虑修改高级规则。

## 需要发 Issue 求助

先生成脱敏诊断报告：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
```

提交 Issue 时建议提供：

1. 你想做什么。
2. 实际发生了什么。
3. 路由器型号和 OpenWrt 版本。
4. ClashForge 版本。
5. 脱敏诊断报告。

不要公开订阅链接、Token、账号密码或私钥。
