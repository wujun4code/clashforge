# 我该怎么开始

这页给第一次使用 ClashForge 的用户。目标很简单：**把代理服务放到路由器上，并确认它真的能用**。

文档里的路由器地址都用 `192.168.20.1` 举例。你需要替换成自己的路由器地址。

## 先看你是否适合用 ClashForge

ClashForge 适合你，如果：

1. 你已经有代理订阅链接，或已有 Clash 兼容配置文件。
2. 你有一台 OpenWrt 路由器。
3. 你希望手机、电脑、电视等设备少做单独配置。
4. 你愿意先用浏览器管理代理，再慢慢调整高级设置。

ClashForge 不适合你，如果：

1. 你还没有任何代理服务或订阅。
2. 你不能登录自己的路由器。
3. 你只是想在一台电脑上临时使用代理。

## 你会经历 4 步

| 步骤 | 你要做什么 | 完成后是什么样 |
| --- | --- | --- |
| 1 | 安装 ClashForge 到路由器 | 浏览器能打开管理页面 |
| 2 | 添加你的订阅或配置 | 页面里能看到可用节点 |
| 3 | 启动代理服务 | 路由器上开始运行代理 |
| 4 | 让设备使用它 | 你的设备能访问需要代理的资源 |

## 1. 准备电脑和路由器

你需要：

| 项目 | 说明 |
| --- | --- |
| 路由器 | OpenWrt / Kwrt，能 SSH 登录 |
| 电脑 | Windows、macOS、Linux 都可以 |
| 订阅 | 代理服务商给你的订阅链接，或 Clash YAML 文件 |

Windows 用户先确认电脑有 SSH：

```powershell
ssh -V
```

## 2. 下载 ClashForge 项目

```powershell
git clone https://github.com/wujun4code/clashforge.git
cd clashforge
```

## 3. 安装到路由器

Windows：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade
```

macOS / Linux：

```sh
./scripts/clashforgectl --router 192.168.20.1 upgrade
```

如果 GitHub 下载慢，可以加镜像：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Mirror https://ghproxy.com
```

## 4. 打开管理页面

安装成功后，在浏览器打开：

```text
http://192.168.20.1:7777
```

你会看到 ClashForge 的 Web 管理页面。

## 5. 添加订阅并启动

在页面里按这个顺序做：

1. 进入 Setup 或配置页面。
2. 添加订阅链接，或上传已有配置文件。
3. 保存并启用这个配置。
4. 点击启动代理服务。

先不要急着让所有设备都使用代理。先确认服务本身能正常跑起来。

## 6. 确认它真的能用

Windows：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 check
```

你也可以直接用浏览器试试那些平时需要代理才能访问的网站。

## 7. 让设备开始使用

确认代理可用后，再到 Web 页面里打开“让设备使用代理”的相关开关。  
建议先让一台手机或一台电脑测试，没问题后再给更多设备使用。

## 不对劲时先恢复网络

如果打开开关后家里设备不好上网，先运行：

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop
```

这会尽量关闭代理相关设置，让网络先回到普通状态。

下一步可以看：[不好用怎么办](/guide/troubleshooting)。
