# 快速开始

这页帮你在 30 分钟内完成 ClashForge 的基础安装和配置。

## 先确认你的情况

**你的需求是什么？**

| 需求 | 推荐方案 |
| --- | --- |
| 只是想看 YouTube、一般浏览，有机场就行 | [路径 A：机场订阅](#路径-a-机场订阅) |
| 想要比机场更好的 IP 信誉，且预算为零 | [路径 B：Cloudflare Worker 节点](#路径-b-cloudflare-worker-节点) |
| 外贸、支付、账号管理，需要固定独享 IP | [路径 C：VPS 节点](#路径-c-vps-节点) |
| 以上都想要 | 先走路径 A 跑通基础，再按需叠加 B 或 C |

不清楚区别？先看 [为什么需要管好自己的网络出口](/guide/why)，5 分钟理解清楚比之后反复折腾省时间。

**环境准备：**

| 需要 | 说明 |
| --- | --- |
| OpenWrt 路由器 | 能 SSH 登录，通常是 `ssh root@192.168.20.1` |
| 电脑 | Windows / macOS / Linux 都可以 |

Windows 用户先确认 SSH 可用：

```powershell
ssh -V
```

如果提示"命令不存在"，在系统设置里打开"可选功能 → OpenSSH 客户端"安装后重试。

---

## 第一步：下载 ClashForge

```powershell
git clone https://github.com/wujun4code/clashforge.git
cd clashforge
```

GitHub 克隆慢的话，在浏览器下载 zip 解压也一样。

---

## 第二步：安装到路由器

把 `192.168.20.1` 替换成你路由器的实际地址。

Windows：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade
```

macOS / Linux：

```sh
./scripts/clashforgectl --router 192.168.20.1 upgrade
```

下载慢时加镜像：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Mirror https://ghproxy.com
```

安装完成后，浏览器访问 `http://192.168.20.1:7777`，能看到管理页面就成了。

---

## 路径 A：机场订阅

**1. 复制机场订阅链接**

登录机场后台，复制 Clash 格式的订阅 URL。

**2. 在管理页面添加**

1. 进入 Setup / 配置页面
2. 添加订阅链接，给它起个名字（例如"主机场"）
3. 保存，点击"更新订阅"
4. 选中这个配置，点击启动代理服务

**3. 快速验证**

在已连接到路由器的一台设备上，打开之前打不开的网站。能访问即成功。

**如果遇到 reCAPTCHA 频繁弹出或 OpenAI 账号风控**，参考 [为什么需要管好自己的网络出口](/guide/why)——这是机场共享 IP 的结构性问题，考虑叠加路径 B 或 C。

---

## 路径 B：Cloudflare Worker 节点

**优势**：免费，Cloudflare IP 信誉比机场 IP 好，不在 VPN 黑名单里。  
**局限**：IP 不固定、不独享，不适合需要固定 IP 的业务场景。

**前置条件**：一个 Cloudflare 账号（免费注册 cloudflare.com，不需要信用卡）。

**操作步骤**：

1. 打开管理页面 `http://192.168.20.1:7777`
2. 找到"Cloudflare Worker 节点"管理入口
3. 按引导完成 Cloudflare 授权
4. 点击"部署 Worker 节点"，约 30–60 秒完成
5. 节点列表里出现新节点，测试连通性

**验证**：访问出口 IP 查询网站（如 `https://ifconfig.me`），确认 IP 属于 Cloudflare 段。

---

## 路径 C：VPS 节点

**优势**：固定独享 IP，只有你在用，适合外贸、支付、账号管理等对 IP 稳定性要求高的场景。  
**费用**：约 $3–10/月。

**步骤概览**：

1. 购买海外 VPS（推荐：Vultr、DigitalOcean、搬瓦工），选择你业务对应的地区
2. 在 VPS 上搭建代理服务（Shadowsocks / V2Ray / Trojan 等）
3. 在 ClashForge 管理页面"手动添加节点"，填入 VPS 的代理配置参数
4. 将工作设备的流量路由到这个节点

详细的 VPS 节点配置步骤参考 [添加代理来源 → 方式三](/guide/config#方式三添加-vps--ssh-节点)。

---

## 第三步：让更多设备使用

确认一台设备没问题后，在 Web 管理页面打开"让设备使用代理"的相关开关，逐步扩大。

建议顺序：工作电脑 → 手机 → 平板 → 其他设备。

---

## 出问题的第一反应

任何时候网络异常，先执行：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop
```

或路由器本机：

```sh
clashforgectl stop
```

网络恢复后再参考 [不好用怎么办](/guide/troubleshooting) 排查原因。
