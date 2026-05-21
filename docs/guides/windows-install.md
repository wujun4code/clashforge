# 在 Windows 上使用 clashforgectl.ps1 管理 OpenWrt 路由器

本文适用于在 **Windows 10/11** 上通过 `clashforgectl.ps1` 一键脚本对 OpenWrt 路由器进行 ClashForge 的安装、升级与日常管理。

---

## 前置条件

### 1. 开启 OpenSSH 客户端

Windows 10 (1809+) 和 Windows 11 已内置 OpenSSH，默认可能需要手动启用：

**Settings → Apps → Optional Features → Add a feature → OpenSSH Client**

或在 PowerShell（管理员）中执行：

```powershell
Add-WindowsCapability -Online -Name OpenSSH.Client~~~~0.0.1.0
```

验证安装：

```powershell
ssh -V   # 输出类似 OpenSSH_for_Windows_9.x
```

### 2. 获取脚本文件

从 GitHub 仓库获取两个必需文件，放在同一目录下：

```
scripts\
  clashforgectl.ps1   ← Windows 控制入口
  clashforgectl.sh    ← 上传到路由器执行的 shell 脚本
```

克隆仓库（推荐）：

```powershell
git clone https://github.com/wujun4code/clashforge.git
cd clashforge
```

或只下载脚本：

```powershell
$base = "https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts"
Invoke-WebRequest "$base/clashforgectl.ps1" -OutFile clashforgectl.ps1
Invoke-WebRequest "$base/clashforgectl.sh"  -OutFile clashforgectl.sh
```

### 3. 配置免密 SSH 登录（推荐）

```powershell
# 生成密钥（已有则跳过）
ssh-keygen -t ed25519 -f "$env:USERPROFILE\.ssh\id_clashforge" -N ""

# 将公钥追加到路由器
$pub = Get-Content "$env:USERPROFILE\.ssh\id_clashforge.pub"
ssh root@192.168.1.1 "mkdir -p ~/.ssh && echo '$pub' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys"
```

之后可通过 `-Identity` 参数指定私钥，或省略（自动使用 `~/.ssh/id_ed25519` 等默认密钥）。

---

## 快速开始：首次安装

```powershell
# 进入仓库目录（两个脚本文件所在处）
cd clashforge\scripts

# 自动检测架构、下载最新版 IPK、上传到路由器并安装
.\clashforgectl.ps1 -Router 192.168.1.1 upgrade
```

脚本会依次执行：
1. SSH 到路由器检测 CPU 架构（x86_64 / aarch64）
2. 从 GitHub Releases 解析最新版本号
3. 将 IPK 下载到本机临时目录
4. SCP 上传 IPK 到路由器 `/tmp/`
5. 远程执行 `opkg install` 完成安装

> **为什么要本机下载再推送？**  
> 如果让路由器自行下载，停止旧版 ClashForge 会切断路由器的出口，导致下载失败。本机代理下载规避了此鸡蛋问题。

---

## 升级到最新版

```powershell
.\clashforgectl.ps1 -Router 192.168.1.1 upgrade
```

升级流程与首次安装相同，幂等可重复执行。

## 指定版本

```powershell
.\clashforgectl.ps1 -Router 192.168.1.1 upgrade -Version v0.1.0-rc.50
```

## 使用镜像加速（GitHub 访问受限时）

```powershell
# 使用 ghproxy.com 加速
.\clashforgectl.ps1 -Router 192.168.1.1 upgrade -Mirror https://ghproxy.com

# 备选镜像
.\clashforgectl.ps1 -Router 192.168.1.1 upgrade -Mirror https://ghfast.top
.\clashforgectl.ps1 -Router 192.168.1.1 upgrade -Mirror https://github.moeyy.xyz
```

## 使用自定义下载源（企业内网 / 私有 CDN）

```powershell
.\clashforgectl.ps1 -Router 192.168.1.1 upgrade -BaseUrl https://cdn.example.com
```

脚本会拼接为 `$BaseUrl/releases/$Tag/$IpkName` 进行下载。

---

## 参数说明

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-Router` | *必填* | 路由器 IP 或主机名 |
| `-User` | `root` | SSH 用户名 |
| `-Port` | `22` | SSH 端口 |
| `-Identity` | *系统默认* | SSH 私钥路径，如 `~\.ssh\id_ed25519` |
| `-Version` | `latest` | 目标版本，如 `v0.1.0-rc.52` |
| `-Mirror` | *无* | GitHub 下载镜像前缀 |
| `-BaseUrl` | *无* | 完全自定义的 IPK 下载根 URL |
| `-Purge` | `false` | 覆盖安装前先卸载旧版（保留配置） |

---

## 日常管理命令

### 查看服务状态

```powershell
.\clashforgectl.ps1 -Router 192.168.1.1 status
```

### 重启服务

```powershell
.\clashforgectl.ps1 -Router 192.168.1.1 reset
```

带 `-Start` 则在停止后立即启动：

```powershell
.\clashforgectl.ps1 -Router 192.168.1.1 reset -Start
```

### 停止服务

```powershell
.\clashforgectl.ps1 -Router 192.168.1.1 stop
```

### 验证配置文件

```powershell
.\clashforgectl.ps1 -Router 192.168.1.1 check
```

### 刷新 DNS 缓存

```powershell
.\clashforgectl.ps1 -Router 192.168.1.1 flush-dns
```

---

## 卸载

```powershell
# 卸载，保留配置文件
.\clashforgectl.ps1 -Router 192.168.1.1 uninstall -KeepConfig

# 完全卸载（删除配置、密钥、GeoData）
.\clashforgectl.ps1 -Router 192.168.1.1 uninstall -PurgeAll
```

---

## 故障诊断

### 收集并查看诊断报告（路由器本地）

```powershell
.\clashforgectl.ps1 -Router 192.168.1.1 diag
```

### 下载诊断报告到本机

```powershell
.\clashforgectl.ps1 -Router 192.168.1.1 diag -Fetch -LocalPath .\cf-diag.txt
```

### 下载并脱敏（适合共享给他人排查问题）

```powershell
.\clashforgectl.ps1 -Router 192.168.1.1 diag -Fetch -LocalPath .\cf-diag-redacted.txt -Redact
```

---

## 使用非标准端口 / 密钥

```powershell
.\clashforgectl.ps1 -Router 192.168.1.1 -Port 2222 -Identity "~\.ssh\id_router" upgrade
```

---

## 常见问题

**Q: 提示 `ssh not found`**  
A: 按前置条件第 1 步安装 OpenSSH Client，或重启 PowerShell 后重试。

**Q: 提示 `clashforgectl.sh not found`**  
A: 确认 `clashforgectl.sh` 与 `clashforgectl.ps1` 在同一目录下，或切换到 `scripts/` 目录后执行。

**Q: GitHub 下载超时**  
A: 添加 `-Mirror https://ghproxy.com` 参数，或检查本地网络能否访问 `github.com`。

**Q: 安装后路由器无法上网**  
A: 运行 `.\clashforgectl.ps1 -Router 192.168.1.1 check` 验证配置，确认订阅 URL 填写正确；或运行 `diag -Fetch` 下载日志排查。
