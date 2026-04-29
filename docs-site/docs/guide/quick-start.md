# 快速开始

这页适合第一次安装。目标是在 30 到 60 分钟内完成四件事：安装 ClashForge、导入一个可用来源、启动 mihomo、验证一台设备能按预期访问。

示例路由器地址使用 `192.168.20.1`，请替换成你的实际地址。

::: tip 安全默认
ClashForge 安装后不会强行让全家设备立刻走代理。先导入来源、启动内核、验证节点，再逐步开启透明代理和 DNS 接管。
:::

## 0. 先判断你属于哪条路径

| 你的目标 | 推荐第一步 |
| --- | --- |
| 日常浏览、流媒体、家里多设备共用机场 | 先导入机场订阅 |
| TikTok/YouTube/独立站运营，需要较干净的轻量出口 | 先部署 Cloudflare Worker 节点 |
| Stripe/PayPal/广告后台/长期 AI API，需要固定出口 | 准备 VPS/SSH 节点 |
| 还不确定 | 先用机场或已有 YAML 跑通，再逐步增加 Worker/VPS |

如果你还没理解出口差异，先读 [产品定位与适用人群](/guide/why)。

## 1. 准备环境

| 项目 | 要求 |
| --- | --- |
| 路由器 | OpenWrt，能 SSH 登录 |
| 电脑 | Windows 10/11、macOS 或 Linux |
| 网络 | 电脑能访问 GitHub，或准备镜像参数 |
| 代理来源 | 机场订阅、Clash YAML、Cloudflare 账号或 VPS 任选其一 |

确认能登录路由器：

```powershell
ssh root@192.168.20.1
```

Windows 如果没有 `ssh` 命令，先在系统“可选功能”里安装 OpenSSH Client。

## 2. 下载仓库

```powershell
git clone https://github.com/wujun4code/clashforge.git
cd clashforge
```

GitHub 克隆慢时，也可以在浏览器下载 ZIP 解压后进入目录。

## 3. 安装 ClashForge

推荐从电脑远程安装。这样 IPK 先下载到电脑，再上传到路由器，通常比让路由器自己访问 GitHub 更稳。

Windows：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade
```

macOS / Linux：

```sh
./scripts/clashforgectl --router 192.168.20.1 upgrade
```

GitHub 下载慢时加镜像：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Mirror https://ghproxy.com
```

安装完成后打开：

```text
http://192.168.20.1:7777
```

如果页面能打开，就进入下一步。

## 4. 第一次进入 Web UI

打开 Web UI 后，优先进入 Setup 或配置管理。

建议顺序：

1. 导入一个最容易成功的来源。
2. 保持 DNS 和透明代理为保守设置。
3. 启动 mihomo 内核。
4. 在概览页测试节点和出口 IP。
5. 只让一台设备先验证。

## 5. 路径 A：导入机场订阅

适合先跑通基础能力。

准备：

| 需要 | 说明 |
| --- | --- |
| Clash 订阅链接 | 从机场后台复制 Clash/YAML 格式订阅 |
| 备用订阅 | 可选，但推荐 |

操作：

1. 进入“配置管理”或 Setup。
2. 添加订阅，粘贴 Clash 订阅 URL。
3. 填写名称，例如“主机场”。
4. 如机场要求特殊客户端，填写 User-Agent。
5. 保存后点击更新订阅。
6. 选择该配置或来源，启动 mihomo。

验证：

| 检查项 | 期望结果 |
| --- | --- |
| 节点列表 | 能看到订阅里的节点 |
| 概览状态 | mihomo 显示运行中 |
| 节点测速 | 至少一个节点延迟正常 |
| 出口 IP | 与所选节点地区一致 |

## 6. 路径 B：部署 Cloudflare Worker 节点

适合低成本补充一个 Cloudflare 网络出口。

准备：

| 需要 | 说明 |
| --- | --- |
| Cloudflare 账号 | 免费账号即可 |
| API Token | 用于部署 Worker 和读取 Zone |
| 托管在 Cloudflare 的域名 | 用于绑定 Worker 自定义域名，推荐 |

操作：

1. 进入“出口节点”。
2. 点击“Worker 节点”或“新建 Worker 节点”。
3. 填入 Cloudflare API Token 和 Account ID。
4. 选择 Zone 或填写绑定域名。
5. 点击部署，等待 Worker 创建和域名绑定。
6. 部署完成后导出 Clash 配置，或把节点用于订阅发布。

效果：

| 能带来的价值 | 说明 |
| --- | --- |
| 成本低 | Cloudflare 免费额度适合轻量使用 |
| IP 信誉通常优于普通共享机场 | 适合一般 AI、资料查询、备用线路 |
| 无需 VPS | 不需要维护服务器 |

注意：

Worker IP 不固定，也不是独享。支付、广告账号、长期固定业务出口仍建议使用 VPS。

## 7. 路径 C：部署 VPS/SSH 节点

适合需要固定独享出口的业务。

准备：

| 需要 | 说明 |
| --- | --- |
| 海外 VPS | 选择业务需要的国家或地区 |
| SSH 登录权限 | root 或有 sudo 权限的用户 |
| 域名和 Cloudflare DNS | 用于自动绑定和证书签发 |
| 邮箱 | 用于 Let's Encrypt 证书申请 |

操作：

1. 进入“出口节点”。
2. 点击新增 VPS/SSH 节点。
3. 填写服务器 IP、SSH 端口、用户、域名和邮箱。
4. 按页面提示授权 ClashForge 的 SSH 公钥。
5. 点击校验 SSH。
6. 执行完整部署，等待 GOST、TLS、Cloudflare DNS 和探测完成。
7. 导出 Clash 配置，或把节点加入订阅发布。

效果：

| 能带来的价值 | 说明 |
| --- | --- |
| 固定 IP | 适合账号后台、广告、支付、长期 AI API |
| 独享出口 | 不受机场其他用户行为影响 |
| 可纳入设备分流 | 关键设备可长期绑定此出口 |

## 8. 让一台设备先使用

不要第一次就让全家设备全部接管。建议先选一台测试设备。

检查顺序：

1. 设备连接到这台 OpenWrt 路由器。
2. 在 ClashForge 中确认 mihomo 正在运行。
3. 打开透明代理或对应设备策略。
4. 在设备上访问一个海外网站。
5. 再访问一个国内网站。
6. 查询出口 IP。

期望结果：

| 项目 | 成功表现 |
| --- | --- |
| 海外网站 | 能打开 |
| 国内网站 | 正常且不明显变慢 |
| 出口 IP | 与所选节点一致 |
| Web UI | 概览页能看到连接和流量 |

## 9. 出问题先恢复网络

如果开启接管后无法上网，先恢复，不要继续乱改设置。

Windows：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop
```

macOS / Linux：

```sh
./scripts/clashforgectl --router 192.168.20.1 stop
```

路由器本机临时执行：

```sh
cd /tmp
wget -O clashforgectl.sh https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/clashforgectl.sh
sh clashforgectl.sh stop
```

恢复后看 [故障排查](/guide/troubleshooting)。

## 10. 下一步

已经能访问后，按你的目标继续：

| 目标 | 下一页 |
| --- | --- |
| 想把安装细节看清楚 | [安装到路由器](/guide/install) |
| 想添加多个来源 | [导入来源与配置](/guide/config) |
| 想按设备区分出口 | [启动接管与设备生效](/guide/run) |
| 想确认是否真的成功 | [验证是否成功](/guide/verify) |
