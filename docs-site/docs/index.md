---
layout: home

hero:
  name: ClashForge
  text: 把代理服务变成路由器上的一个简单开关
  tagline: 你已经有代理订阅或配置，ClashForge 帮你把它放到 OpenWrt 路由器上，让手机、电脑、电视等设备更容易访问需要代理的资源。
  image:
    src: /logo.svg
    alt: ClashForge
  actions:
    - theme: brand
      text: 我该怎么开始
      link: /guide/quick-start
    - theme: alt
      text: 添加代理订阅
      link: /guide/config
    - theme: alt
      text: 不好用怎么办
      link: /guide/troubleshooting

features:
  - title: 少折腾每台设备
    details: 把代理配置放在路由器上，家里的设备接入这个网络后，就可以按你的规则使用代理。
  - title: 用网页管理
    details: 不需要一直 SSH 到路由器，打开浏览器就能添加订阅、启动服务、查看连接状态。
  - title: 先试用再全家使用
    details: 第一次不会直接改变全网流量，先确认代理可用，再让更多设备使用。
  - title: 出问题能恢复
    details: 网络异常时可以一键退出代理相关设置，先恢复正常上网，再慢慢排查。
  - title: 适合长期使用
    details: 支持更新订阅、切换节点、查看状态、升级软件和收集诊断信息。
  - title: 面向 OpenWrt
    details: 适合把 OpenWrt 路由器当作家庭或小办公室的代理入口。
---

## ClashForge 是什么？

ClashForge 是一个安装在 OpenWrt 路由器上的代理管理工具。

你可以把它理解成：

1. 你手里有一个代理订阅或 Clash 配置。
2. ClashForge 把这个配置放到路由器上运行。
3. 你用网页控制它启动、停止、切换节点、查看是否生效。
4. 你的手机、电脑、电视等设备可以更方便地使用这个代理能力。

它不是代理服务商，也不会提供节点。你仍然需要自己准备可用的订阅链接或配置文件。

## 它解决什么问题？

如果没有 ClashForge，很多人会遇到这些麻烦：

| 常见麻烦 | ClashForge 的目标 |
| --- | --- |
| 每台设备都要单独安装代理软件 | 尽量把配置集中到路由器上 |
| 路由器上配置代理太复杂 | 用网页向导完成主要操作 |
| 不知道代理到底有没有生效 | 提供状态、连接检查和出口信息 |
| 改错设置后全家断网 | 保留快速恢复网络的办法 |
| 订阅、规则、节点长期维护麻烦 | 在一个地方更新和管理 |

## 我怎么用？

第一次使用建议按这个顺序：

1. [安装到路由器](/guide/install)
2. [添加代理订阅](/guide/config)
3. [让设备开始使用](/guide/run)
4. [确认是否成功](/guide/verify)
5. [不好用时恢复网络](/guide/troubleshooting)

## 你需要准备什么？

| 需要准备 | 说明 |
| --- | --- |
| OpenWrt 路由器 | 需要能 SSH 登录，通常使用 root 用户 |
| 一台电脑 | Windows、macOS、Linux 都可以，用来安装和管理 |
| 代理订阅或配置 | 订阅链接、Clash YAML 配置都可以 |
| 路由器地址 | 例如 `192.168.20.1`，文档里都用这个作为示例 |

## 最重要的使用原则

第一次不要急着让所有设备都走代理。

更稳的做法是：

1. 先把 ClashForge 安装好。
2. 再添加订阅并启动代理。
3. 确认网页能打开、节点能用、出口符合预期。
4. 最后再让家里的设备开始使用它。

这样即使哪里不对，也更容易恢复。
