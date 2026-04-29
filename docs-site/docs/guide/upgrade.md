# 升级与回滚（稳定优先）

升级不是“追新版本”，而是“在可控风险下获取修复和能力”。  
建议把升级看成一个标准流程：**准备 -> 升级 -> 验收 -> 必要时回滚**。

## 升级前 3 分钟准备

升级前先确认：

1. 你知道当前稳定版本号。
2. 你知道一键回退命令（`stop`）。
3. 你可以在异常时快速收集 `diag` 报告。

## 标准升级（推荐）

Windows：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade
```

路由器本机：

```sh
clashforgectl upgrade
```

为什么推荐 Windows 远程升级：  
IPK 在本机下载后再推送，能降低“路由器升级中失去外网”的风险。

## 升级到指定版本

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Version v0.1.0
```

路由器本机：

```sh
clashforgectl upgrade --version v0.1.0
```

## 网络受限时

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Mirror https://ghproxy.com
```

自定义发布源：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -BaseUrl https://releases.example.com
```

## `-Purge` 什么时候用

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Purge
```

::: danger `-Purge` 只用于“历史状态已污染”
它会触发更彻底清理，可能删除现有配置和运行数据。正常升级不建议使用。
:::

## 回滚策略（建议默认掌握）

最简单的回滚方式是直接升回旧版本：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Version v0.1.0
```

回滚动作建议顺序：

1. 先 `stop`，确保退出接管。
2. 安装目标旧版本。
3. 启动并执行 [检查清单](/guide/verify)。

## 升级后验收必做

至少确认这 6 项：

1. Web UI 可访问。
2. `status` 正常。
3. `check` 结果与预期一致。
4. 内核进程稳定、无重复重启。
5. 接管状态符合你的配置（没有“意外全开/全关”）。
6. 订阅与规则仍有效。
