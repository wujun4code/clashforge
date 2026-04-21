# ClashForge

ClashForge 是一个面向 **OpenWrt 路由器** 的新一代代理管理项目，目标是基于 **mihomo** 的核心能力，重新实现和重构现有 OpenClash 这一类产品在路由器上的管理层、控制层和交付体验。

它不是另一个代理协议实现，也不是要替代 mihomo 本身；相反，它的定位是：

> **围绕 mihomo 构建一个更轻、更稳、更现代、更易维护的 OpenWrt 路由器管理层。**

---

## 项目背景

目前 OpenWrt 路由器场景里，用户广泛依赖以下两类上游能力：

- **[vernesong/openclash](https://github.com/vernesong/openclash)**  
  OpenWrt 上非常成熟的 Clash/Mihomo 客户端与管理方案，具备很强的生态基础和用户基础。

- **[MetaCubeX/mihomo](https://github.com/MetaCubeX/mihomo)**  
  负责实际代理协议、规则匹配、DNS、分流、透明代理与控制 API 的核心引擎。

这两个项目都很重要，但在 OpenWrt 实际使用场景里，传统方案也存在一些长期痛点，例如：

- 管理层依赖较多，整体偏重
- OpenWrt 集成方式历史包袱较大
- 配置、订阅、透明代理、防火墙规则、UI、状态管理等能力耦合较深
- 对低配路由器的资源占用与维护成本并不总是理想
- 对现代 Web UI、原子配置更新、可观测性和工程化演进不够友好

ClashForge 的目标，就是在继承上游成熟能力的前提下，针对 **OpenWrt 路由器场景** 重新设计一层更现代的管理系统。

---

## 设计目标

ClashForge 当前设定的方向包括：

- 基于 **mihomo**，而不是重复造代理内核
- 聚焦 **OpenWrt 路由器部署**
- 尽量做成 **单一静态二进制**
- 尽量减少运行时依赖
- 更清晰地拆分：
  - 核心进程管理
  - 配置生成
  - 订阅管理
  - 防火墙规则管理
  - Web UI / REST API
- 强调：
  - 原子配置更新
  - 优雅退出
  - 单实例约束
  - 可观测性
  - 更现代的前端与控制体验

一句话说：

> **让 OpenWrt 上的 Clash/Mihomo 管理层，从“能用”进化到“工程上更干净、更稳、更长期可维护”。**

---

## 当前状态

当前仓库还处于 **设计与规格定义阶段**，尚未正式开始完整实现。

现阶段最重要的产物是这份实施落地文档：

- [`docs/CLASH_REPLACEMENT_DESIGN.md`](./docs/CLASH_REPLACEMENT_DESIGN.md)

这份文档是基于项目目标生成和整理出的 **工程规格文档**，用于指导后续编码实现。它覆盖了：

- 目录结构
- Go 模块与依赖约束
- 配置文件设计
- HTTP API 规格
- CoreManager 生命周期
- 配置生成引擎
- 订阅管理
- nftables / iptables 透明代理规则
- Web UI 结构
- OpenWrt 集成
- 错误处理与测试策略
- Phase 开发计划

也就是说，这份文档不是随手记的想法，而是：

> **后续真正实现 ClashForge 的第一份工程蓝图。**

---

## 项目边界

ClashForge **不会**：

- 自己实现代理协议栈
- 替代 mihomo 的流量处理能力
- 偏离 OpenWrt 路由器场景去做桌面端客户端

ClashForge **会重点负责**：

- mihomo 进程生命周期管理
- mihomo 配置生成与热重载
- 订阅拉取、过滤、缓存与合并
- nftables / iptables 规则应用与回滚
- Web UI 与 REST API
- OpenWrt 包集成与部署体验

---

## 为什么叫 ClashForge

这个名字表达的是一个很直接的意图：

- `Clash`：源于 Clash / Mihomo 生态背景
- `Forge`：强调“锻造”、“重构”、“重新打磨工程实现”

它不是简单包一层 UI，而是希望把 OpenWrt 路由器上的代理管理体验重新锻造一遍。

---

## 近期计划

当前的短期方向：

1. 固化工程规格文档
2. 初始化项目骨架
3. 搭建最小可运行原型：
   - 静态二进制入口
   - CoreManager
   - 基础 API
   - 配置生成
   - 简单 Web UI
4. 逐步接入：
   - 订阅管理
   - 透明代理规则
   - OpenWrt init / package 集成

---

## 参考上游项目

- [vernesong/openclash](https://github.com/vernesong/openclash)
- [MetaCubeX/mihomo](https://github.com/MetaCubeX/mihomo)

ClashForge 的方向建立在对这些优秀上游项目的学习和继承之上。

---

## License

暂未确定。
