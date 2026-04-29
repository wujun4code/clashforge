# 常见问题

## ClashForge 是代理服务商吗？

不是。ClashForge 不出售节点，也不提供代理账号。它是 OpenWrt 路由器上的 mihomo 管理工具，用来管理你已有的代理来源，例如机场订阅、Cloudflare Worker 节点或自建 VPS 节点。

## 我只有一台电脑需要代理，还需要 ClashForge 吗？

不一定。只在一台电脑上临时使用，桌面 Clash/mihomo 客户端更简单。

ClashForge 更适合：

| 情况 | 为什么适合 |
| --- | --- |
| 多台设备都要访问海外资源 | 路由器统一接管 |
| 团队成员不想各自配置客户端 | 管理集中在 Web UI |
| 需要按设备区分出口 | 设备分流比每台设备手动设置更稳 |
| 需要固定业务出口 | VPS/SSH 节点可和策略绑定 |
| 需要诊断和恢复能力 | `stop`、`diag`、日志和规则搜索 |

## 机场、Worker、VPS 应该怎么选？

| 类型 | 适合 | 不适合 |
| --- | --- | --- |
| 机场订阅 | 流媒体、普通浏览、备用线路 | 支付、广告、强账号稳定性业务 |
| Cloudflare Worker | 低成本轻量工作、一般 AI 访问、备用出口 | 固定 IP、独享 IP、重度流量 |
| VPS/SSH 节点 | 固定出口、支付、广告、长期 AI API | 只想零维护、只看流媒体 |

三者可以同时存在。推荐按设备和业务分流，而不是把所有流量压到一种出口。

## 为什么用了代理还是有 reCAPTCHA？

大多数时候是出口 IP 信誉问题。共享机场节点被很多人同时使用，目标平台可能把这个 IP 当作高风险来源。

处理建议：

1. 先换节点。
2. 轻量工作尝试 Cloudflare Worker。
3. 关键业务使用 VPS 固定节点。
4. 避免核心账号频繁切换国家和出口。

## Cloudflare Worker 是固定 IP 吗？

不是。Worker 出口属于 Cloudflare 网络，通常信誉较好，但 IP 不固定，也不是你独享。

适合：

| 场景 | 是否推荐 |
| --- | --- |
| 一般 AI 工具访问 | 推荐尝试 |
| 普通资料查询 | 推荐 |
| 备用线路 | 推荐 |
| Stripe/PayPal/广告账号长期操作 | 不推荐，优先 VPS |
| 强绑定单一 IP 的业务 | 不推荐 |

## VPS/SSH 节点是不是一定最好？

不是。VPS 的优势是固定和独享，但速度、线路和 IP 质量仍取决于 VPS 服务商和机房。

建议：

| 目标 | 建议 |
| --- | --- |
| 支付、广告、后台账号 | VPS 是优先选择 |
| 流媒体 | 机场专线可能更合适 |
| 免费轻量使用 | Worker 更省事 |
| 大量下载 | 选带宽和流量更充足的方案 |

## 需要在每台设备上安装客户端吗？

通常不需要。ClashForge 的目标就是让连接到路由器的设备按路由器策略生效。

但有些场景仍可能需要客户端：

| 场景 | 原因 |
| --- | --- |
| 设备离开这个路由器网络 | 路由器无法接管外部网络 |
| 单台电脑需要不同策略 | 本机客户端更灵活 |
| 移动办公 | 可使用 ClashForge 发布的订阅链接导入移动客户端 |

## Web UI 在哪里？

```text
http://<你的路由器IP>:7777
```

例如：

```text
http://192.168.20.1:7777
```

不知道路由器 IP 时，Windows 可运行 `ipconfig` 看默认网关；macOS/Linux 可运行 `ip route` 或查看当前 Wi-Fi 网关。

## `upgrade` 和 `deploy` 有什么区别？

| 命令 | 面向谁 | 做什么 |
| --- | --- | --- |
| `upgrade` | 普通用户 | 下载 Release IPK 并安装或更新 |
| `deploy` | 开发者 | 从当前源码构建前端和后端，打包本地 IPK 并安装 |

普通用户使用 `upgrade`。开发者在修改源码后才使用 `deploy`。

## 为什么文档里有远程脚本和路由器本机脚本两种命令？

因为两种场景不同。

| 命令 | 场景 |
| --- | --- |
| `.\scripts\clashforgectl.ps1` | Windows 电脑远程控制路由器 |
| `./scripts/clashforgectl` | macOS/Linux 电脑远程控制路由器 |
| `sh clashforgectl.sh` | SSH 到路由器本机后临时执行 |

当前安装包不默认把 `clashforgectl` 放进 `/usr/bin`。如果你希望在路由器本机长期使用，可以手动复制：

```sh
cp /tmp/clashforgectl.sh /usr/bin/clashforgectl
chmod +x /usr/bin/clashforgectl
```

## 打开接管后全家网络断了怎么办？

先恢复网络：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop
```

恢复后再排查。不要在断网状态下连续改 DNS、透明代理、规则和订阅。

## 设备分流为什么没生效？

最常见原因是设备 IP 变了。

处理：

1. 在路由器 DHCP 页面确认设备当前 IP。
2. 给关键设备绑定 DHCP 静态租约。
3. 回到 ClashForge 设备分流页面更新设备组。
4. 保存后重启或重新生成配置。
5. 在设备上查询出口 IP。

## 订阅发布是做什么的？

订阅发布用于把你部署的 VPS/Worker 节点、内置模板、当前运行配置或自定义模板合并，发布成一个可给客户端导入的订阅链接。

适合：

| 场景 | 价值 |
| --- | --- |
| 手机离开家里 Wi-Fi 仍要用自建节点 | 导入发布订阅 |
| 团队成员需要同一批节点 | 统一分发和更新 |
| 多客户端共享配置 | 减少手动复制节点 |

## 诊断报告能直接发出去吗？

只发脱敏版。

Windows：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
```

macOS / Linux：

```sh
./scripts/clashforgectl --router 192.168.20.1 diag --fetch --redact
```

不要公开：

| 敏感信息 | 说明 |
| --- | --- |
| 订阅链接 | 通常包含 token |
| Cloudflare Token | 可操作你的 Cloudflare 资源 |
| SSH 私钥 | 可登录服务器 |
| 账号密码 | 不应出现在任何公开 issue |

## ClashForge 和 OpenClash 能同时运行吗？

不建议。它们都可能管理 mihomo、端口、DNS、防火墙和透明代理规则，同时运行容易冲突。

安装前可检查：

```sh
sh /tmp/clashforgectl.sh compat
```

如果确认要停掉残留 OpenClash 进程：

```sh
sh /tmp/clashforgectl.sh openclash --kill
```

## README 和文档站分别看什么？

| 入口 | 适合阅读什么 |
| --- | --- |
| GitHub README | 项目定位、架构、模块、开发、发布、贡献 |
| 文档站 | 安装、配置、使用、排障、运维手册 |

README 会链接到文档站，文档站也会指向 GitHub 仓库和 README。
