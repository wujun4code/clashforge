---
layout: home

hero:
  name: ClashForge
  text: OpenWrt mihomo 管理与接管指南
  tagline: 从安装、配置、启动到检查、排障和升级，给路由器代理运维一套清晰闭环。
  image:
    src: /logo.svg
    alt: ClashForge
  actions:
    - theme: brand
      text: 10 分钟快速开始
      link: /guide/quick-start
    - theme: alt
      text: 安装与部署
      link: /guide/install
    - theme: alt
      text: GitHub
      link: https://github.com/wujun4code/clashforge

features:
  - title: 安全默认
    details: 首次启动默认不自动接管透明代理和 DNS，先确认节点与内核运行正常，再手动开启接管。
  - title: IPK 优先
    details: 面向 OpenWrt 的部署、升级和回滚都围绕 IPK 包展开，适合长期维护与自动化发布。
  - title: 远程控制
    details: Windows、macOS、Linux 都可以通过 clashforgectl 远程查看状态、部署、升级、诊断和卸载。
  - title: 可验证
    details: 提供服务、API、DNS、netfilter、出口 IP、日志与诊断报告的完整检查清单。
  - title: 双语结构
    details: 中文与英文文档并行，适合个人使用、团队交接和公开发布。
  - title: GitHub Pages
    details: 使用 VitePress 生成静态站点，支持本地搜索、代码高亮、响应式导航和自动部署。
---

## 推荐阅读路径

如果你是第一次安装，按这个顺序阅读：

1. [快速开始](/guide/quick-start)
2. [安装与部署](/guide/install)
3. [首次配置](/guide/config)
4. [启动与接管](/guide/run)
5. [检查清单](/guide/verify)

如果你已经部署过，直接进入 [日常运维](/guide/operations) 或 [排障](/guide/troubleshooting)。
