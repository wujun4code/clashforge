# 日常怎么用

ClashForge 跑起来之后，你大多数时间只需要做 4 件事：

1. 看是否正常运行。
2. 更新订阅。
3. 切换节点。
4. 出问题时先恢复网络。

## 常用入口

浏览器打开：

```text
http://192.168.20.1:7777
```

日常操作优先在 Web 页面里完成。命令只在需要检查、更新、恢复时使用。

## 每天怎么用

| 你想做什么 | 推荐做法 |
| --- | --- |
| 看现在是否正常 | 打开 Web 页面看状态 |
| 网站突然打不开 | 先换一个节点试试 |
| 订阅节点变少 | 在页面里更新订阅 |
| 网络明显异常 | 先执行 `stop` 恢复网络 |
| 想确认是否生效 | 运行 `check` 或看出口 IP |

## 常用命令

Windows：

```powershell
# 查看状态
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 status

# 检查是否能正常访问
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 check

# 出问题时先恢复网络
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop

# 收集脱敏诊断报告
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
```

路由器本机：

```sh
clashforgectl status
clashforgectl check
clashforgectl stop
clashforgectl diag --redact
```

## 什么时候需要检查

建议在这些时候检查一下：

1. 刚安装完。
2. 刚添加或更新订阅。
3. 刚让更多设备使用代理。
4. 刚升级 ClashForge。
5. 家里设备反馈打不开网站。

## 保持稳定的小习惯

1. 不要频繁同时改很多设置。
2. 每次只调整一个订阅、一个节点或一个开关。
3. 调整后马上测试一台设备。
4. 保留一个备用节点或备用订阅。
5. 记住 `stop` 是恢复网络的第一步。

## 卸载

完全卸载：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 uninstall
```

卸载但保留配置：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 uninstall -KeepConfig
```
