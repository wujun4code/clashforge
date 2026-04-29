# 更新软件

ClashForge 可以像普通软件一样更新。  
更新前先确认当前网络能正常使用，并知道如何恢复。

## 什么时候需要更新

建议更新的情况：

1. 新版本修复了你遇到的问题。
2. 新版本增加了你需要的功能。
3. 当前版本长期稳定，但你准备做一次维护。

不建议在网络已经异常时直接更新。先恢复网络，再更新。

## 更新到最新版

Windows：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade
```

路由器本机：

```sh
clashforgectl upgrade
```

Windows 方式会先在电脑上下载安装包，再传到路由器，通常更稳。

## GitHub 下载慢

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Mirror https://ghproxy.com
```

## 更新到指定版本

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Version v0.1.0
```

这个命令也可以用来回到旧版本。

## 更新后检查

更新完成后做三件事：

1. 打开 `http://192.168.20.1:7777`。
2. 确认代理服务能启动。
3. 用一台设备测试需要代理的网站。

也可以运行：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 check
```

## 出问题时

先恢复网络：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop
```

再回到旧版本：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Version v0.1.0
```

把 `v0.1.0` 换成你想回到的版本。

## 谨慎使用清理更新

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Purge
```

`-Purge` 会清理更多旧数据，可能影响已有配置。只有在配置已经混乱、需要干净重装时再用。
