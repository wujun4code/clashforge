# 安装与部署

ClashForge 推荐使用 IPK 包部署到 OpenWrt。脚本部署本质上也是本机构建 IPK、上传 IPK、再在路由器安装。

## 部署方式选择

| 场景 | 推荐方式 | 说明 |
| --- | --- | --- |
| 日常开发验证 | Windows `clashforgectl.ps1 deploy` | 本地构建并推送 IPK，最快闭环 |
| 正式安装 | Release IPK | 从 GitHub Releases 安装稳定包 |
| 路由器上维护 | `clashforgectl` | 已安装后在路由器 SSH 内直接操作 |
| 故障恢复 | IPK 回滚 | 保留旧包，直接卸载后安装旧版本 |

## Windows 远程部署

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy
```

部署流程包括：

1. 自动提升 IPK control 文件中的 patch 版本。
2. 构建 React Web UI。
3. 交叉编译 Linux amd64 Go 二进制。
4. 同步 OpenWrt helper 文件到 IPK 目录。
5. 生成 IPK 包。
6. 上传 IPK 和 `clashforgectl.sh` 到路由器。
7. 通过 `upgrade --local-ipk` 在路由器安装。

::: warning 版本会自动递增
`deploy` 会修改 `ipk/CONTROL/control` 中的版本号。执行前如果工作区已有未提交改动，请确认这是你希望保留的构建行为。
:::

## 从 Releases 升级或安装

```powershell
# 升级到最新版本，默认由本机下载 IPK 后推送到路由器
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade

# 安装指定版本
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Version v0.1.0

# 通过 GitHub 镜像下载
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Mirror https://ghproxy.com

# 使用自定义发布源
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -BaseUrl https://releases.example.com

# 完全清理旧数据后升级，谨慎使用
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Purge
```

## 路由器本机安装与维护

SSH 到路由器后可以直接使用：

```sh
clashforgectl status
clashforgectl upgrade
clashforgectl upgrade --version v0.1.0
clashforgectl upgrade --mirror https://ghproxy.com
clashforgectl upgrade --purge
```

## 服务入口

安装后常用 OpenWrt init 命令：

```sh
/etc/init.d/clashforge enable
/etc/init.d/clashforge start
/etc/init.d/clashforge status
/etc/init.d/clashforge restart
/etc/init.d/clashforge stop
```

## 安装后地址

默认 Web UI：

```text
http://<router-ip>:7777
```

例如：

```text
http://192.168.20.1:7777
```
