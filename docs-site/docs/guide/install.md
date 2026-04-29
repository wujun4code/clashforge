# 安装到路由器

ClashForge 要安装在 OpenWrt 路由器上。安装完成后，你会得到一个网页管理入口，用来添加订阅、启动代理、查看状态。

## 推荐安装方式

大多数用户只需要这一种方式：**在电脑上运行安装命令，把 ClashForge 安装到路由器**。

Windows：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade
```

macOS / Linux：

```sh
./scripts/clashforgectl --router 192.168.20.1 upgrade
```

这里的 `192.168.20.1` 是示例地址，请换成你的路由器地址。

## 安装前检查

| 检查项 | 怎么确认 |
| --- | --- |
| 能登录路由器 | `ssh root@192.168.20.1` |
| 电脑能访问 GitHub | 浏览器能打开 GitHub，或使用镜像参数 |
| 已准备订阅 | 有代理订阅链接或配置文件 |

如果 SSH 端口不是默认的 `22`，可以指定端口：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 -Port 2222 upgrade
```

如果使用私钥登录：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 -Identity ~\.ssh\id_ed25519 upgrade
```

## GitHub 下载慢怎么办

可以加镜像：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Mirror https://ghproxy.com
```

## 安装成功后打开哪里

浏览器访问：

```text
http://192.168.20.1:7777
```

如果页面能打开，就可以继续 [添加代理订阅](/guide/config)。

## 开发者才需要的安装方式

如果你是在修改 ClashForge 源码，才需要用 `deploy`：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy
```

这个命令会重新构建前端和后端，再打包安装。普通用户不需要走这条路。

## 手动安装

当自动安装不可用时，可以手动下载 Releases 里的安装包，上传到路由器后安装：

```sh
opkg install --nodeps --force-downgrade /tmp/<ipk-file>.ipk
```

手动安装更容易选错架构，只建议熟悉 OpenWrt 的用户使用。
