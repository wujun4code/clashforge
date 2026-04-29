# 首次配置

ClashForge 的配置目标是让 mihomo 可以稳定启动，并让透明代理、DNS 接管、订阅和规则都可控。

## 推荐顺序

1. 先准备 Clash 兼容 YAML 或订阅链接。
2. 在 Web UI 的 Setup/配置向导中导入来源。
3. 保存配置并激活。
4. 启动 mihomo 内核。
5. 验证节点连通性和出口 IP。
6. 最后按需开启透明代理或 DNS 接管。

## 配置来源

常见来源包括：

| 来源 | 用途 | 建议 |
| --- | --- | --- |
| 上传 YAML | 快速迁移已有 Clash 配置 | 适合首次验证 |
| 粘贴 YAML | 手动调试配置 | 注意缩进和格式 |
| 订阅 URL | 长期维护节点 | 建议配置至少一个备用源 |
| YAML Overrides | 覆盖生成配置 | 只放必须覆盖的最小片段 |

## 订阅配置建议

订阅源建议记录以下信息：

| 项目 | 说明 |
| --- | --- |
| 名称 | 用于区分供应商或用途 |
| URL | 订阅地址，注意不要公开 |
| User-Agent | 某些订阅服务需要指定 |
| 更新间隔 | 根据供应商限制设置，不要过于频繁 |

::: warning 保护敏感信息
订阅 URL、Token、SSH 私钥路径和诊断报告都可能包含敏感信息。公开 Issue 或截图前先脱敏。
:::

## 网络配置

透明代理相关配置建议从保守策略开始：

| 配置 | 推荐初始值 | 说明 |
| --- | --- | --- |
| 透明代理模式 | `none` 或手动开启 | 首次部署先不全网接管 |
| 防火墙后端 | `auto` | 让 ClashForge 自动选择 nftables/iptables |
| 启动时接管 | 关闭 | 确认内核稳定后再开启 |
| LAN 绕过 | 开启 | 避免管理流量绕路 |
| 中国大陆 IP 绕过 | 按需 | 取决于你的策略和规则集 |

## DNS 配置

DNS 是最容易影响全网体验的部分，建议逐步打开：

1. 先只启动 mihomo 内核。
2. 检查代理节点、规则和出口 IP。
3. 再开启 DNS 解析器。
4. 最后开启 dnsmasq 协作或入口接管。

常见模式：

| 模式 | 适合场景 |
| --- | --- |
| `fake-ip` | 透明代理、域名规则命中、复杂分流 |
| `redir-host` | 更接近传统解析行为，排障相对直观 |
| dnsmasq upstream | 与 OpenWrt 现有 DNS 体系共存 |
| dnsmasq replace | ClashForge 接管更多 DNS 流程，需谨慎验证 |

## 配置文件位置

代码层面的相关模块：

| 路径 | 作用 |
| --- | --- |
| `internal/config/loader.go` | 配置加载 |
| `internal/config/merger.go` | 配置合并 |
| `internal/config/generator.go` | mihomo 配置生成 |
| `internal/config/validator.go` | 配置校验 |
| `internal/config/device_groups.go` | 设备分组 |

## 配置完成标准

完成首次配置后，应满足：

1. Web UI 可以正常打开。
2. 至少有一个可用节点。
3. mihomo 内核可以启动。
4. `/status` 或状态页面显示正常。
5. 浏览器侧和路由器侧连通性探测符合预期。
