---
layout: home

hero:
  name: ClashForge
  text: OpenWrt 上的 mihomo 管理控制台
  tagline: 把订阅、节点、设备分流、DNS 接管、连通性诊断和恢复操作集中到路由器上。适合跨境电商团队、TikTok/YouTube 自媒体、AI 工具重度用户、独立开发者和多设备家庭。
  image:
    src: /logo.png
    alt: ClashForge
  actions:
    - theme: brand
      text: 30 分钟跑通
      link: /guide/quick-start
    - theme: alt
      text: 功能模块
      link: /guide/features
    - theme: alt
      text: 安装手册
      link: /guide/install

features:
  - title: 路由器统一管理
    details: ClashForge 安装在 OpenWrt 路由器上，设备不需要逐台安装客户端。订阅、配置、节点、DNS、透明代理和恢复操作都由路由器统一执行。
  - title: 设备级出口策略
    details: 可以给销售电脑、剪辑机、开发机、手机、电视、访客网络分别设置出口。关键业务设备走固定节点，娱乐设备走机场，国内业务直连。
  - title: 多来源节点并行
    details: 同时管理机场订阅、自建 VPS/GOST 节点、Cloudflare Worker 节点，并能导出或发布成 Clash 兼容配置。
  - title: 可验证的接管流程
    details: 首次配置向导会导入配置、设置 DNS、设置透明代理、启动内核、检测端口和连通性，降低“一开就全网断”的风险。
  - title: 面向运维的恢复能力
    details: stop、reset、diag、openclash 扫描、兼容性检查等命令覆盖常见故障。先恢复上网，再慢慢排查。
  - title: 技术团队可扩展
    details: Go 后端、React UI、REST API、SSE 实时状态、OpenWrt IPK/APK 打包，适合 fork 后做团队内部发行或自托管。
---

## 谁最适合使用 ClashForge

ClashForge 不是代理服务商，也不卖节点。它解决的是：你已经有机场、VPS 或 Cloudflare 账号，但缺少一个运行在路由器上的统一管理入口。

| 用户类型 | 常见问题 | ClashForge 提供的能力 |
| --- | --- | --- |
| 电商独立站团队 | 广告、支付、客服、数据分析工具对出口 IP 很敏感 | 关键岗位设备绑定固定出口，普通设备走共享订阅 |
| TikTok / YouTube 自媒体 | 多账号运营、素材上传、平台后台访问不稳定 | 为运营设备设置独立策略，避免全员共用一个高风险出口 |
| AI 资源重度用户 | OpenAI、Claude、Gemini、GitHub、npm 经常受 IP 质量影响 | 工作设备走更干净的出口，娱乐设备不挤占工作链路 |
| 独立开发者 / 工作室 | 多台设备、多个订阅、节点频繁切换，维护成本高 | Web UI 更新订阅、测速、切换节点、查看日志 |
| 小型团队网络管理员 | 没有专职 IT，却要保证全员能稳定访问海外资源 | 路由器集中配置，命令行可远程恢复、诊断和升级 |
| 家庭多设备用户 | 家人不会配置代理，电视、手机、电脑策略不同 | 设备连接路由器后自动按规则走，不需要逐台安装客户端 |

## 建议阅读路径

第一次使用时，按这个顺序走最稳：

1. [产品定位与适用人群](/guide/why)：先判断 ClashForge 是否适合你。
2. [功能模块总览](/guide/features)：理解每个核心功能解决什么问题。
3. [快速开始](/guide/quick-start)：用最短路径完成一次可用部署。
4. [安装到路由器](/guide/install)：按你的环境选择路由器本机安装、远程安装或手动安装。
5. [导入来源与配置](/guide/config)：把订阅、YAML、DNS、透明代理和设备分流配置起来。
6. [验证是否成功](/guide/verify)：确认出口 IP、目标网站、DNS 和日志都符合预期。

## 这套文档怎么分工

docs-site 是用户手册，面向安装、配置、使用和排障。它会尽量把每个按钮、每个命令、每种场景讲清楚。

GitHub 仓库的 [README](https://github.com/wujun4code/clashforge#readme) 是技术归纳，面向开发者、贡献者和想了解架构的人。README 会从组件、目录、构建、发布、API 和工程边界讲清楚项目本身。

两个入口会互相链接：你在 README 里看到架构后，可以回到这里照着使用；你在这里跑通后，也可以回到 README 继续看源码结构和二次开发方式。
