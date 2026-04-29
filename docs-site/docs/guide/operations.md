# 日常维护

ClashForge 跑起来后，日常维护的核心不是频繁折腾，而是稳定、可观测、可恢复。

## 日常操作地图

| 你想做什么 | 推荐入口 |
| --- | --- |
| 看现在是否正常 | Web UI 概览 |
| 更新机场订阅 | 配置管理 -> 订阅 |
| 切换节点 | 概览页代理组 |
| 查看连接 | 活动日志 -> 连接 |
| 查看错误 | 活动日志 -> 日志 |
| 更新 GeoData | 路由数据 |
| 调整设备出口 | 设备分流 |
| 发布团队订阅 | 订阅定制 |
| 恢复网络 | 远程脚本或本机 `sh clashforgectl.sh stop` |
| 生成诊断 | `diag -Redact` 或 `diag --redact` |

## 每天怎么用

建议形成一个轻量流程：

1. 打开 Web UI 概览。
2. 看 mihomo 是否运行。
3. 看流量和连接是否正常。
4. 若网站异常，先切换节点。
5. 若节点列表异常，更新订阅。
6. 若全网异常，先执行 `stop` 恢复。

## 订阅维护

**这个功能是干嘛的？**
定期拉取机场或节点服务提供的最新配置。

**解决什么问题？**
节点 IP、端口、加密方式和规则可能变化。订阅不更新，就会出现节点失效或规则过期。

**如何使用？**

1. 进入“配置管理 -> 订阅”。
2. 查看上次更新时间和节点数量。
3. 点击单个订阅更新，或执行全部更新。
4. 如果失败，检查订阅 URL、User-Agent、账号状态。

**实现效果和价值**

| 效果 | 价值 |
| --- | --- |
| 节点保持最新 | 减少“昨天能用今天不能用” |
| 多来源统一维护 | 主备订阅切换更简单 |
| 更新失败可见 | 更快判断是订阅服务问题还是本地问题 |

## 节点切换和测速

**这个功能是干嘛的？**
在代理组里切换 Selector 节点，并查看延迟。

**解决什么问题？**
某个节点可能临时不可用、速度变慢或被目标平台拒绝。切换节点是最快的第一步。

**如何使用？**

1. 进入概览页。
2. 找到对应代理组。
3. 点击测速。
4. 选择延迟更低或业务更匹配的节点。
5. 切换后重新做连通检测。

**实现效果和价值**

| 效果 | 价值 |
| --- | --- |
| 不用编辑 YAML | 直接在 UI 里切换 |
| 切换后可检测 | 立刻知道出口是否变化 |
| 保留策略组语义 | 仍沿用原订阅的 Selector/URLTest/Fallback |

## 设备策略维护

**这个功能是干嘛的？**
维护不同设备组的出口策略。

**解决什么问题？**
设备 IP 变化、人员变动、节点更新后，设备分流可能需要调整。

**如何使用？**

1. 定期检查关键设备是否仍使用固定 IP。
2. 新增设备时先放入测试组。
3. 工作设备只允许使用业务节点。
4. 娱乐设备使用机场或流媒体节点。
5. 保存后验证每台关键设备出口 IP。

**实现效果和价值**

| 效果 | 价值 |
| --- | --- |
| 关键设备稳定 | 不被娱乐流量和随机节点影响 |
| 团队策略统一 | 用户不用自己配置客户端 |
| 排障范围清晰 | 设备问题和节点问题更容易区分 |

## GeoData 与规则维护

建议：

| 项目 | 频率 |
| --- | --- |
| GeoIP/GeoSite | 每周或按需更新 |
| rule-provider | 订阅更新后检查 |
| 域名/IP 规则搜索 | 网站走错路线时使用 |

操作：

1. 进入“路由数据”。
2. 检查 `GeoIP.dat` 和 `GeoSite.dat` 是否存在。
3. 选择下载代理。
4. 点击立即更新。
5. 开启定时更新时，设置合理间隔，例如 `168h`。

## 常用远程命令

Windows：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 status
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 check
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
```

macOS / Linux：

```sh
./scripts/clashforgectl --router 192.168.20.1 status
./scripts/clashforgectl --router 192.168.20.1 check
./scripts/clashforgectl --router 192.168.20.1 stop
./scripts/clashforgectl --router 192.168.20.1 diag --fetch --redact
```

路由器本机临时执行：

```sh
cd /tmp
wget -O clashforgectl.sh https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/clashforgectl.sh
sh clashforgectl.sh status
sh clashforgectl.sh check
sh clashforgectl.sh stop
sh clashforgectl.sh diag --redact
```

如需本机长期使用：

```sh
cp /tmp/clashforgectl.sh /usr/bin/clashforgectl
chmod +x /usr/bin/clashforgectl
clashforgectl status
```

## 诊断报告

需要公开求助时，优先使用脱敏报告。

Windows：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
```

macOS / Linux：

```sh
./scripts/clashforgectl --router 192.168.20.1 diag --fetch --redact
```

报告通常包含：

| 内容 | 用途 |
| --- | --- |
| 系统版本和架构 | 判断包和平台是否匹配 |
| 服务状态 | 判断 clashforge/mihomo 是否运行 |
| 端口和进程 | 判断端口冲突 |
| nftables/iptables | 判断接管规则是否残留 |
| DNS 配置 | 判断 dnsmasq 是否恢复 |
| 日志片段 | 判断启动和运行错误 |

## 卸载

保留配置卸载：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 uninstall -KeepConfig
```

完全卸载：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 uninstall
```

路由器本机：

```sh
sh /tmp/clashforgectl.sh uninstall --keep-config
sh /tmp/clashforgectl.sh uninstall
```

## 稳定使用建议

1. 不要一次改很多设置。
2. 改动后立刻验证一台设备。
3. 关键设备绑定 DHCP 静态租约。
4. 保留至少一个备用来源。
5. 开机自动接管只在稳定后开启。
6. 遇到全网异常先 `stop`，恢复后再排查。
