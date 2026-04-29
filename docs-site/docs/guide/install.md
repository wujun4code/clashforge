# 安装到路由器

这页是完整安装手册。普通用户建议用“电脑远程安装”；熟悉 OpenWrt 的用户可以在路由器本机安装或手动安装 Release 包。

## 安装后会得到什么

| 组件 | 作用 |
| --- | --- |
| `clashforge` | 后端管理服务，内嵌 Web UI 和 API |
| `mihomo-clashforge` | 随包内置的 mihomo 内核 |
| `/etc/init.d/clashforge` | OpenWrt 服务入口 |
| `/etc/metaclash` | ClashForge 配置、订阅、规则和运行数据 |
| Web UI | 默认 `http://<路由器IP>:7777` |
| `clashforgectl.sh` | 维护脚本，作为 Release 资产和远程脚本使用 |

::: warning 注意
当前 IPK/APK 包不默认把 `clashforgectl` 安装到 `/usr/bin`。Windows/macOS/Linux 远程脚本会自动把 `clashforgectl.sh` 临时上传到路由器执行。需要在路由器本机长期使用时，可以手动复制为 `/usr/bin/clashforgectl`。
:::

## 支持的包和架构

Release 会构建以下包：

| 包类型 | 适用系统 | 架构 |
| --- | --- | --- |
| IPK | 当前主流 OpenWrt | `x86_64`、`aarch64_generic`、`aarch64_cortex-a53` |
| APK | OpenWrt 25.12+ | `x86_64`、`aarch64_generic`、`aarch64_cortex-a53` |

如果你的设备是 ARMv7、MIPS 或非常老的 OpenWrt，目前不在默认 Release 支持范围内。

## 安装前检查

在电脑上确认能登录路由器：

```powershell
ssh root@192.168.20.1
```

在路由器本机可先运行兼容性检查：

```sh
cd /tmp
wget -O clashforgectl.sh https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/clashforgectl.sh
sh clashforgectl.sh compat
```

检查项包括：

| 检查项 | 为什么重要 |
| --- | --- |
| CPU 架构 | 需要匹配 Release 包 |
| OpenWrt 包管理器 | IPK 用 `opkg`，OpenWrt 25.12+ 可手动用 `apk` |
| `/tmp` 和 `/overlay` 空间 | 包下载和安装需要空间 |
| GitHub 访问 | 影响自动下载 |
| OpenClash 残留 | 可能与端口、进程和防火墙规则冲突 |

如检测到 OpenClash 进程残留，可按提示处理：

```sh
sh clashforgectl.sh openclash --kill
```

## 方式一：Windows 远程安装

这是多数用户最稳的方式。

```powershell
git clone https://github.com/wujun4code/clashforge.git
cd clashforge
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade
```

指定 SSH 端口：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 -Port 2222 upgrade
```

使用私钥：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 -Identity ~\.ssh\id_ed25519 upgrade
```

GitHub 下载慢时使用镜像：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Mirror https://ghproxy.com
```

指定版本：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Version v0.1.0-rc.1
```

这条路径会自动完成：

1. 连接路由器并识别 CPU 架构。
2. 在本机下载对应 IPK。
3. 通过 SCP 上传到路由器。
4. 上传 `clashforgectl.sh`。
5. 在路由器上执行 `upgrade --local-ipk`。
6. 安装后启动 `/etc/init.d/clashforge`。

## 方式二：macOS / Linux 远程安装

```sh
git clone https://github.com/wujun4code/clashforge.git
cd clashforge
./scripts/clashforgectl --router 192.168.20.1 upgrade
```

指定端口、用户和私钥：

```sh
./scripts/clashforgectl --router 192.168.20.1 --user root --port 2222 --identity ~/.ssh/id_ed25519 upgrade
```

使用镜像：

```sh
./scripts/clashforgectl --router 192.168.20.1 upgrade --mirror https://ghproxy.com
```

## 方式三：路由器本机安装

如果你只方便 SSH 到路由器，也可以直接在路由器上执行。

```sh
cd /tmp
wget -O clashforgectl.sh https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/clashforgectl.sh
sh clashforgectl.sh compat
sh clashforgectl.sh upgrade
```

指定镜像：

```sh
sh clashforgectl.sh upgrade --mirror https://ghproxy.com
```

指定版本：

```sh
sh clashforgectl.sh upgrade --version v0.1.0-rc.1
```

如果想以后直接输入 `clashforgectl`，可手动保留一份：

```sh
cp /tmp/clashforgectl.sh /usr/bin/clashforgectl
chmod +x /usr/bin/clashforgectl
clashforgectl status
```

## 方式四：手动安装 Release 包

当自动脚本不可用时，可以手动从 GitHub Releases 下载对应架构包。

IPK 示例：

```sh
opkg install --nodeps --force-downgrade /tmp/clashforge_<version>_<arch>.ipk
```

APK 示例，适用于 OpenWrt 25.12+：

```sh
apk add --allow-untrusted /tmp/clashforge-<version>_<arch>.apk
```

手动安装容易选错架构。除非你熟悉 OpenWrt 包管理，否则优先使用远程安装脚本。

## 安装完成后

浏览器打开：

```text
http://192.168.20.1:7777
```

服务命令：

```sh
/etc/init.d/clashforge status
/etc/init.d/clashforge restart
/etc/init.d/clashforge stop
```

远程检查：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 status
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 check
```

## 开发者部署

如果你正在修改源码，才需要 `deploy`。它会构建前端、编译 Go、生成本地 IPK 并安装到路由器。

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy
```

可跳过部分构建步骤：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy -Skip ui
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy -Skip go
```

::: warning
`deploy` 面向开发验证，不是普通用户安装路径。它可能修改本地打包版本号，请在干净工作区使用。
:::

## 下一步

安装成功后继续 [导入来源与配置](/guide/config)。
