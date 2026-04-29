---
layout: home

hero:
  name: ClashForge
  text: 在路由器上定义规则，让每台设备走该走的线
  tagline: 家庭、工作室、小型跨境团队都适用。ClashForge 安装在 OpenWrt 路由器上，统一管理所有设备的网络出口——哪台设备走哪个节点、哪些域名直连、谁完全隔离，一次配置，自动执行。
  image:
    src: /logo.png
    alt: ClashForge
  actions:
    - theme: brand
      text: 快速开始
      link: /guide/quick-start
    - theme: alt
      text: 为什么 IP 质量很重要
      link: /guide/why
    - theme: alt
      text: 出问题怎么办
      link: /guide/troubleshooting

features:
  - title: 按设备精细分流
    details: 以设备为单位定义出口规则。销售电脑走 VPS 固定节点用于广告账号管理，开发机走另一条线访问 GitHub 和 AI 工具，家人的手机走机场看视频，访客设备完全隔离——规则在路由器上，每台设备自动执行，无需单独配置。
  - title: 多出口并行，按场景自动切换
    details: 机场订阅、Cloudflare Worker 节点、VPS/SSH 节点可以同时接入，在不同设备或不同域名规则上分别使用。不同的业务需求对出口 IP 有不同要求，ClashForge 让你在一个地方统一管理和切换。
  - title: 共享 IP 信誉差？用干净出口替代
    details: 机场共享 IP 被数百人同时使用，容易被 Google、OpenAI、支付平台拉黑。Cloudflare Worker 节点免费且 IP 信誉好；需要固定独享 IP 时接入 VPS 节点。给对 IP 质量敏感的设备单独配置干净出口。
  - title: 网页管理，无需每次 SSH
    details: 更新订阅、切换节点、查看出口 IP、调整设备规则，浏览器打开管理页面就能完成。只有第一次安装需要用到命令行。
  - title: 网络异常一键恢复
    details: 配置出错或节点故障时，一条命令先恢复全网正常上网，不会因为调代理让整个办公室或全屋断网。恢复网络之后再慢慢排查原因。
  - title: 适合家庭和小型团队
    details: 几个人的外贸团队、独立工作室、自媒体团队，或家里有多台需要不同网络策略的设备——只要有一台 OpenWrt 路由器，ClashForge 就能统一管控，不需要专职 IT。
---

## 典型使用场景

### 跨境团队 / 小型外贸公司

**网络现状：** 办公室共用同一个路由器，不同岗位需要不同的网络策略，但没有专职 IT 去维护每台电脑。

| 岗位 / 设备 | 网络需求 | ClashForge 的配置 |
| --- | --- | --- |
| 销售 / 运营电脑 | Google Ads、Facebook Ads、收款平台，需要固定 IP | 走 VPS 固定节点 |
| 开发 / 设计电脑 | GitHub、Claude Code、Adobe CC | 走 VPS 或 CF Worker 节点 |
| 财务电脑 | 只访问国内系统，不需要代理 | 直连，不走任何代理 |
| 访客 Wi-Fi | 完全隔离，不影响业务设备 | 单独规则，限速或直连 |

**效果：** IT 负责人（或最懂技术的那个人）在路由器上配置一次，此后每台设备自动按规则走，员工不需要自己折腾代理设置。

---

### 独立开发者 / 自由职业者

你每天需要：GitHub、npm、Claude Code、OpenAI API、Google Search。

**痛点：** 机场 IP 经常触发 Google reCAPTCHA，OpenAI 偶尔提示账号异常，Claude Code 连接不稳定。根源是你在用一个被数百人共用、已被标记为高风险的 IP。

**解法：** 工作电脑走 VPS 固定节点，AI 工具和开发服务走干净的独享 IP；预算为零时先用 Cloudflare Worker 节点过渡；手机娱乐保留机场订阅。家里不同设备互不干扰。

---

### 自媒体团队 / 创意工作室

你每天需要：YouTube Studio（上传/分析）、Twitter/X、TikTok、Google Analytics、Facebook Ads。

**痛点：** 多账号管理时平台检测到"异常 IP"，账号被限流或封禁。机场共享 IP 上可能早已有人在滥用这些平台。

**解法：** 负责账号运营的设备走 VPS 固定节点，同一个 IP 长期出现在平台日志里，不会因为 IP 变化触发风控。视频渲染机、个人手机走另一套规则，互不影响。

---

### 家庭多设备管理

你家里有：工作用的 MacBook、孩子的 iPad、父母的手机、智能电视、NAS。

**痛点：** 每台设备都要单独装代理软件和配置，更新节点时要挨个改，家里人也不会用。

**解法：** 路由器统一管控，工作电脑走指定节点，娱乐设备走机场，老人手机直连不走代理，一次配置永久生效。

---

## ClashForge 是什么，不是什么

| ClashForge 是 | ClashForge 不是 |
| --- | --- |
| 安装在 OpenWrt 路由器上的代理管控工具 | 代理服务商或节点提供商 |
| 按设备定义分流规则的管理平台 | 免费的代理服务 |
| 支持机场订阅 + CF Worker + VPS 多出口并行 | 一键解决所有网络问题的魔法工具 |
| 适合家庭和小型团队的轻量 IT 方案 | 企业级 SD-WAN 或专业网络设备的替代品 |

你仍然需要自己准备节点来源：机场订阅、Cloudflare 免费账号，或一台海外 VPS。

---

## 你需要准备什么

| 需要准备 | 说明 |
| --- | --- |
| OpenWrt 路由器 | 能 SSH 登录。Kwrt、ImmortalWrt 等也兼容 |
| 一台电脑 | Windows、macOS、Linux 都可以，用来安装和管理 |
| 代理来源（至少一种） | 机场订阅、Cloudflare 免费账号（CF Worker 节点），或海外 VPS |
| 路由器地址 | 例如 `192.168.20.1`，文档里统一用这个作为示例 |

---

## 第一次使用，建议这个顺序

1. [IP 质量为什么重要](/guide/why) — 5 分钟理解清楚，之后少走弯路
2. [快速开始](/guide/quick-start) — 选择适合你的路径
3. [安装到路由器](/guide/install) — 5 分钟完成
4. [添加代理来源](/guide/config) — 机场订阅、CF Worker 或 VPS 节点
5. [让设备开始使用](/guide/run) — 先一台，确认后再扩大
6. [确认是否成功](/guide/verify) — 检查出口 IP 和可访问性
