# 在 Hyper-V OpenWrt 虚拟机中安装 ClashForge

本文适用于你已经有一台 **Hyper-V 里的 OpenWrt x86_64 虚拟机**，想在里面安装 ClashForge。

如果你使用的是 ClashForge 预制 Hyper-V VHDX，则镜像已经内置 ClashForge，不需要再安装；本文主要给“自己装的 OpenWrt x86_64 VM”使用。

---

## 0. 前置条件

- OpenWrt 虚拟机架构是 `x86_64`。
- Windows 主机可以 SSH 到 OpenWrt，或 OpenWrt 自己可以访问下载地址。
- OpenWrt 使用 `opkg`（OpenWrt 24.10/23.05 常见）。如果是 OpenWrt 25.12+，可能需要改用 APK 包。

检查架构：

```sh
uname -m
opkg print-architecture
```

常见输出应包含：

```text
x86_64
arch x86_64 ...
```

---

## 1. 找到 OpenWrt 的 IP

在 Hyper-V 控制台登录 OpenWrt 后执行：

```sh
ip addr show br-lan
ip route
```

常见 IP：

- 普通 OpenWrt 默认：`192.168.1.1`
- ClashForge 预制 Hyper-V 网络规划：`192.168.77.1`

下面示例用 `<OPENWRT_IP>` 代表你的 OpenWrt IP。

---

## 2. 推荐方式：在 Windows 上用 ctl 脚本远程安装

在 Windows PowerShell 中执行：

```powershell
Invoke-WebRequest 'https://dl.wei1xuan.com/clashforgectl.ps1' -OutFile .\clashforgectl.ps1
Invoke-WebRequest 'https://dl.wei1xuan.com/clashforgectl_impl.ps1' -OutFile .\clashforgectl_impl.ps1
```

先做兼容性检查：

```powershell
.\clashforgectl.ps1 -Router <OPENWRT_IP> compat
```

安装/升级 ClashForge：

```powershell
.\clashforgectl.ps1 -Router <OPENWRT_IP> upgrade
```

如果 GitHub 访问慢，可以改成“本机下载 R2 最新 x86_64 IPK，再上传到 OpenWrt 安装”：

```powershell
Invoke-WebRequest 'https://dl.wei1xuan.com/clashforge-latest-x86_64.ipk' -OutFile .\clashforge-latest-x86_64.ipk
.\clashforgectl.ps1 -Router <OPENWRT_IP> upgrade -LocalIpkFile .\clashforge-latest-x86_64.ipk
```

> 这个方式最稳：下载发生在 Windows 主机，安装发生在 OpenWrt，不依赖 OpenWrt 自己访问外网。

---

## 3. 方式二：直接在 OpenWrt 里安装 R2 最新 IPK

SSH 进入 OpenWrt：

```powershell
ssh root@<OPENWRT_IP>
```

在 OpenWrt 中执行：

```sh
cd /tmp
wget -O clashforge-latest-x86_64.ipk https://dl.wei1xuan.com/clashforge-latest-x86_64.ipk
opkg install --nodeps ./clashforge-latest-x86_64.ipk
/etc/init.d/clashforge enable
/etc/init.d/clashforge restart
```

检查状态：

```sh
/etc/init.d/clashforge status
logread -e clashforge | tail -80
```

打开 Web UI：

```text
http://<OPENWRT_IP>:7777
```

---

## 4. 方式三：在 OpenWrt 里运行 ctl 安装脚本

如果 OpenWrt 能访问 R2/GitHub，也可以直接执行：

```sh
wget -qO- https://dl.wei1xuan.com/clashforgectl.sh | sh
```

指定用 R2 版本目录安装某个版本时：

```sh
wget -qO- https://dl.wei1xuan.com/clashforgectl.sh | \
  sh -s -- --version v0.1.0-rc.1 --base-url https://dl.wei1xuan.com
```

把 `v0.1.0-rc.1` 换成你要安装的 release tag。

---

## 5. 安装完成后的常用地址和端口

- ClashForge Web UI：`http://<OPENWRT_IP>:7777`
- HTTP 代理：`<OPENWRT_IP>:17890`
- SOCKS5 代理：`<OPENWRT_IP>:17891`
- Mixed 代理：`<OPENWRT_IP>:17893`

首次打开 Web UI 后，先添加/导入订阅，再启动代理。

---

## 6. 常见问题

### `wget: bad address` 或下载失败

OpenWrt DNS 或出口还没通。优先用 Windows 下载 IPK 后通过 ctl 脚本上传安装：

```powershell
Invoke-WebRequest 'https://dl.wei1xuan.com/clashforge-latest-x86_64.ipk' -OutFile .\clashforge-latest-x86_64.ipk
.\clashforgectl.ps1 -Router <OPENWRT_IP> upgrade -LocalIpkFile .\clashforge-latest-x86_64.ipk
```

### `opkg` 提示依赖问题

ClashForge IPK 已内置 mihomo，安装时可以使用：

```sh
opkg install --nodeps /tmp/clashforge-latest-x86_64.ipk
```

### 打不开 `:7777`

在 OpenWrt 中检查服务和监听端口：

```sh
/etc/init.d/clashforge status
netstat -lntp | grep 7777 || ss -lntp | grep 7777
logread -e clashforge | tail -80
```

Hyper-V 如果是多网卡，请确认 Windows 主机和 OpenWrt LAN 口在同一个虚拟交换机里。

### 想卸载

```sh
opkg remove clashforge
# 完全删除配置/数据：
uninstall-clashforge
```
