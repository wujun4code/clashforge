---
layout: home

hero:
  name: ClashForge
  text: 面向真实代理用户的 OpenWrt 上手文档
  tagline: 从“先能用”到“长期稳定”，按用户旅程一步步完成安装、配置、接管、验收和排障。
  image:
    src: /logo.svg
    alt: ClashForge
  actions:
    - theme: brand
      text: 15 分钟快速开始
      link: /guide/quick-start
    - theme: alt
      text: 选择安装方式
      link: /guide/install
    - theme: alt
      text: GitHub
      link: https://github.com/wujun4code/clashforge

features:
  - title: 用户目标优先
    details: 每一页都围绕“我要先恢复上网、再稳定使用”来写，而不是按代码模块拆文档。
  - title: 安全默认
    details: 默认不自动接管透明代理和 DNS，先验证内核与节点，避免一次改动影响全网。
  - title: 可回退
    details: 每个关键步骤都附带回退命令，出现问题先恢复网络，再继续定位。
  - title: 验收导向
    details: 不是“命令执行成功”就算完成，而是用可感知的联网结果和检查清单做验收。
  - title: 运维闭环
    details: 覆盖安装、配置、升级、回滚、排障与诊断报告，适合长期维护家庭或小团队网络。
  - title: 多端控制
    details: 可以在 Windows、macOS、Linux 远程控制路由器，也能在路由器本机执行运维命令。
---

## 你现在处在哪个阶段

| 你的情况 | 先看哪里 |
| --- | --- |
| 第一次部署，目标是尽快能用 | [快速开始](/guide/quick-start) |
| 不确定该用哪种安装方式 | [安装方式选择](/guide/install) |
| 已安装但不知道怎么配置更稳 | [首次配置](/guide/config) |
| 刚开了接管，担心影响全网 | [启动与接管](/guide/run) + [检查清单](/guide/verify) |
| 现在已经能用，想长期稳定 | [日常运维](/guide/operations) + [升级与回滚](/guide/upgrade) |
| 突然断网或体验异常 | [排障](/guide/troubleshooting) |

## 推荐阅读路线

### 路线 A：首次上手（推荐）

1. [快速开始](/guide/quick-start)
2. [安装方式选择](/guide/install)
3. [首次配置](/guide/config)
4. [启动与接管](/guide/run)
5. [检查清单](/guide/verify)

### 路线 B：已有基础，追求稳定

1. [检查清单](/guide/verify)
2. [日常运维](/guide/operations)
3. [升级与回滚](/guide/upgrade)
4. [排障](/guide/troubleshooting)

## 三个使用原则

1. **先通，再快，再优雅**：先保证核心网络可用，再做分流和细节优化。
2. **一次只改一件事**：每次只改一个变量（订阅、DNS、接管、规则），定位会快很多。
3. **先留回退手段**：改动前先确认 `stop`、`reset`、`diag` 怎么用，出问题就不会慌。
