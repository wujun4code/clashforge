# 升级与回滚

ClashForge 推荐使用 IPK 进行升级和回滚。这样可以保留清晰的版本边界，也方便从故障版本回退。

## 升级到最新版

Windows 远程：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade
```

路由器本机：

```sh
clashforgectl upgrade
```

默认 Windows 流程会由本机下载 IPK，再上传到路由器安装。这样可以避免路由器在停止代理后失去网络，导致无法下载自己的升级包。

## 升级到指定版本

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Version v0.1.0
```

路由器本机：

```sh
clashforgectl upgrade --version v0.1.0
```

## 使用镜像或自定义源

```powershell
# GitHub 代理镜像
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Mirror https://ghproxy.com

# 自定义 release base URL
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -BaseUrl https://releases.example.com
```

## 清理升级

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Purge
```

::: danger 谨慎使用 Purge
`-Purge` 会触发更彻底的清理流程，可能删除已有配置、订阅和运行数据。只有在需要干净重装或排除历史配置污染时使用。
:::

## 回滚策略

建议保留最近 2 到 3 个可用 IPK 包。回滚时：

1. 先停止接管。
2. 安装旧版本 IPK。
3. 启动服务。
4. 执行完整检查清单。

示例：

```sh
clashforgectl stop
opkg remove clashforge
opkg install /tmp/clashforge_0.1.0_x86_64.ipk
/etc/init.d/clashforge enable
/etc/init.d/clashforge start
```

## 升级后必查

升级完成后至少检查：

1. Web UI 能否打开。
2. `status` 是否正常。
3. mihomo 是否只有一个有效进程。
4. 端口是否无冲突。
5. DNS 和透明代理是否按预期接管或保持关闭。
6. 订阅、规则集和 Overrides 是否仍然有效。
