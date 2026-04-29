# 更新软件

ClashForge 的普通升级以 Release 包为边界。推荐从电脑远程执行升级，因为电脑下载 Release 包通常比路由器直接访问 GitHub 更稳定。

## 更新前检查

| 检查项 | 为什么 |
| --- | --- |
| 当前网络是否正常 | 网络已经异常时先恢复，不要直接升级 |
| 是否知道恢复命令 | 升级异常时能执行 `stop` |
| 是否有重要配置 | 重要订阅、Overrides、节点信息建议先备份 |
| 路由器空间是否足够 | IPK/APK 下载和安装需要空间 |
| 是否需要指定版本 | 回滚时必须知道目标版本号 |

## 推荐升级方式

Windows：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade
```

macOS / Linux：

```sh
./scripts/clashforgectl --router 192.168.20.1 upgrade
```

使用镜像：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Mirror https://ghproxy.com
```

指定版本：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Version v0.1.0-rc.1
```

远程升级会：

1. 检测路由器架构。
2. 下载对应 Release IPK 到电脑。
3. 上传到路由器 `/tmp`。
4. 执行安装。
5. 尽量保留 `/etc/metaclash` 中的配置。
6. 重启 ClashForge 服务。

## 路由器本机升级

```sh
cd /tmp
wget -O clashforgectl.sh https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/clashforgectl.sh
sh clashforgectl.sh upgrade
```

镜像：

```sh
sh clashforgectl.sh upgrade --mirror https://ghproxy.com
```

指定版本：

```sh
sh clashforgectl.sh upgrade --version v0.1.0-rc.1
```

## 更新后验证

升级完成后做这些检查：

1. 打开 `http://192.168.20.1:7777`。
2. 查看概览页版本和内核状态。
3. 启动或确认 mihomo 正常运行。
4. 测试一个海外网站和一个国内网站。
5. 查询出口 IP。
6. 查看活动日志是否有持续错误。

命令检查：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 status
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 check
```

## 回滚到指定版本

如果新版本出现问题，先恢复网络：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop
```

再安装旧版本：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Version v0.1.0-rc.1
```

把 `v0.1.0-rc.1` 换成你想回到的版本。

## 清理升级

默认升级会尽量保留配置。只有在配置已经混乱、需要干净重装时才使用清理升级。

Windows：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Purge
```

路由器本机：

```sh
sh /tmp/clashforgectl.sh upgrade --purge
```

::: danger
`Purge` 会清理更多本地数据，可能影响订阅、规则、缓存和配置。不要把它当成普通升级方式。
:::

## APK 包说明

OpenWrt 25.12+ 开始引入 APK 包管理。Release 会提供 APK 资产，但当前自动升级脚本主要围绕 IPK/opkg 流程设计。

如果你的系统明确使用 APK，请从 Release 下载对应架构 APK 后手动安装：

```sh
apk add --allow-untrusted /tmp/clashforge-<version>_<arch>.apk
```

## 升级失败时

| 现象 | 处理 |
| --- | --- |
| 下载失败 | 加 `-Mirror https://ghproxy.com` |
| 架构不支持 | 运行 `compat` 确认 CPU 架构 |
| 安装后 UI 打不开 | 检查 `/etc/init.d/clashforge status` |
| 升级后全网异常 | 先执行 `stop` |
| 配置异常 | 查看运行中配置和 Overrides |
| 无法判断 | 生成脱敏诊断报告 |

诊断：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
```
