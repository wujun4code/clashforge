# 故障排查

排障时先记住一句话：先恢复上网，再找原因。不要在全网异常时连续修改多个设置。

## 第一步：恢复网络

Windows：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop
```

macOS / Linux：

```sh
./scripts/clashforgectl --router 192.168.20.1 stop
```

路由器本机：

```sh
cd /tmp
wget -O clashforgectl.sh https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/clashforgectl.sh
sh clashforgectl.sh stop
```

等 10 到 20 秒，再测试国内网站和路由器管理页面。

## 问题一：管理页面打不开

先确认地址：

```text
http://192.168.20.1:7777
```

检查服务：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 status
```

路由器上：

```sh
/etc/init.d/clashforge status
/etc/init.d/clashforge restart
logread | grep -i clashforge
```

| 现象 | 可能原因 | 处理 |
| --- | --- | --- |
| 浏览器无法访问 | 路由器 IP 错误 | 查看默认网关，替换示例 IP |
| 连接超时 | 服务未启动 | 执行 `/etc/init.d/clashforge restart` |
| 端口冲突 | 7777 被占用 | 查找占用进程，调整配置 |
| 能 SSH 但 UI 不开 | 防火墙或服务异常 | 查看 `logread` 和诊断报告 |

## 问题二：订阅添加后没有节点

常见原因：

| 原因 | 处理 |
| --- | --- |
| URL 复制不完整 | 从机场后台重新复制 |
| URL 带空格或换行 | 清理后重试 |
| 订阅需要 User-Agent | 在订阅设置里填写 |
| 账号过期或欠费 | 登录机场后台确认 |
| 路由器无法访问订阅地址 | 用镜像、代理或换网络测试 |

排查步骤：

1. 在电脑浏览器打开订阅 URL，确认能看到 YAML 或节点内容。
2. 在 ClashForge 中更新单个订阅。
3. 查看活动日志是否有 HTTP、TLS、DNS 错误。
4. 如果电脑能打开但路由器不能打开，重点排查路由器 DNS 和出站网络。

## 问题三：节点有，但海外网站打不开

按概率排查。

### 3.1 节点本身不可用

表现：

| 现象 | 判断 |
| --- | --- |
| 换节点后恢复 | 原节点不可用 |
| 所有节点都失败 | 订阅、DNS 或网络整体有问题 |
| 只有某个国家节点失败 | 节点 IP 或线路被目标服务拒绝 |

处理：

1. 在概览页测速。
2. 换一个节点。
3. 更新订阅。
4. 查看机场公告。

### 3.2 出口 IP 信誉差

表现：

| 平台 | 常见表现 |
| --- | --- |
| Google | reCAPTCHA 频繁出现 |
| OpenAI/Claude | 登录验证、异常提示、访问不稳定 |
| Stripe/PayPal | 额外验证或拒绝登录 |
| 广告后台 | 风控提示或频繁重新验证 |

处理：

1. 先换节点。
2. 轻量工作可尝试 Cloudflare Worker 节点。
3. 关键业务使用 VPS/SSH 固定节点。
4. 避免支付、广告、核心账号频繁切换出口。

### 3.3 DNS 与规则不一致

表现：

| 现象 | 可能原因 |
| --- | --- |
| 国外网站解析到奇怪 IP | DNS 未接管或被污染 |
| 国内网站走代理 | 规则或 GeoData 过期 |
| 某个域名总是走错 | rule-provider 未更新或自定义规则冲突 |

处理：

1. 在“路由数据”更新 GeoIP/GeoSite。
2. 在“配置管理 -> 规则集”同步 rule-provider。
3. 使用规则搜索检查域名命中。
4. 查看 DNS 设置是否与接管模式一致。

## 问题四：打开接管后全家网络异常

立刻执行 `stop`。恢复后按这个顺序重新来：

1. 只启动 mihomo 内核，不开透明代理。
2. 在概览页测试节点。
3. 只让一台设备进入测试组。
4. 开启透明代理，测试这台设备。
5. 再开启 DNS 接管。
6. 最后再扩大设备范围。

常见原因：

| 原因 | 说明 |
| --- | --- |
| DNS 接管冲突 | dnsmasq 和 mihomo 端口或模式冲突 |
| 防火墙规则冲突 | OpenClash 或旧规则残留 |
| 策略路由残留 | 流量被错误标记 |
| 设备 IP 变化 | 设备分流命中错误 |
| 路由器资源不足 | mihomo 或 ClashForge 被杀掉 |

## 问题五：设备分流不生效

检查：

| 检查项 | 说明 |
| --- | --- |
| 设备 IP 是否正确 | 手机/电脑可能重新获取了 IP |
| DHCP 静态租约 | 关键设备应固定 IP |
| 设备组来源 | 分流规则绑定到对应配置来源 |
| 策略覆盖 | 目标 proxy-group 是否被覆盖 |
| 最终配置 | 运行中配置里是否有生成的规则 |

处理：

1. 在路由器 DHCP 页面确认设备 IP。
2. 在“设备分流”更新设备组。
3. 保存后重新生成配置。
4. 重启 mihomo。
5. 查询该设备出口 IP。

## 问题六：Cloudflare Worker 节点不可用

检查：

| 检查项 | 说明 |
| --- | --- |
| Worker 是否存在 | 登录 Cloudflare Dashboard 查看 |
| 自定义域名是否绑定 | DNS 和 Worker Routes/Custom Domain 是否正确 |
| API Token 权限 | Token 是否有 Worker 和 Zone 相关权限 |
| 请求额度 | 免费额度是否耗尽 |
| UUID/导出配置 | 客户端配置是否来自最新导出 |

处理：

1. 在 ClashForge 中重新部署 Worker。
2. 重新导出 Clash 配置。
3. 检查 Cloudflare 域名解析。
4. 必要时删除后重新创建节点。

## 问题七：VPS/SSH 节点部署失败

检查：

| 阶段 | 常见问题 |
| --- | --- |
| SSH 校验 | 端口不通、用户错误、密钥未授权 |
| GOST 部署 | 服务器系统不兼容、权限不足 |
| Cloudflare DNS | Token 权限不足、Zone 选择错误 |
| 证书签发 | 域名未解析、80/443 被占用 |
| 探测失败 | 防火墙未放行、域名未生效 |

处理：

1. 先在本地 `ssh user@server` 确认能登录。
2. 按页面提示重新授权 ClashForge 公钥。
3. 确认 Cloudflare Token 权限。
4. 确认域名已经指向 VPS。
5. 查看节点部署事件日志。

## 问题八：之前好用，突然不行

| 可能原因 | 判断 | 处理 |
| --- | --- | --- |
| 机场节点变更 | 更新订阅后节点变化 | 更新订阅或换节点 |
| 节点 IP 被目标平台拒绝 | 换节点后恢复 | 使用 Worker/VPS |
| GeoData 过期 | 规则命中异常 | 更新路由数据 |
| 路由器重启 | 服务未启动或未接管 | 检查服务和开机接管 |
| Cloudflare 额度或绑定变化 | Worker 请求失败 | 查看 CF Dashboard |
| VPS 服务异常 | 只有 VPS 节点失败 | 重新探测或登录服务器排查 |

## 手动检查命令

路由器上常用：

```sh
logread | grep -i clashforge
logread | grep -i mihomo
netstat -lntup | grep -E '7777|19090|1789|17874'
nft list tables
ip rule list
df -h /tmp /overlay
```

这些命令只用于观察。清理动作优先交给远程脚本的 `stop`，或路由器本机的 `sh clashforgectl.sh stop`。

## 求助前准备

生成脱敏诊断：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
```

描述问题时建议包含：

1. 你想访问什么服务。
2. 哪台设备受影响。
3. 是否所有网站都失败，还是只有特定网站失败。
4. 出口 IP 是否符合预期。
5. 路由器型号和 OpenWrt 版本。
6. ClashForge 版本。
7. 脱敏诊断报告。

不要公开订阅链接、Cloudflare Token、SSH 私钥、账号密码。
