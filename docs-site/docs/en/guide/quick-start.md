# Quick Start

This page gets you from zero to a verifiable ClashForge deployment. The examples use `192.168.20.1`; replace it with your OpenWrt router address.

## Prerequisites

| Item | Requirement |
| --- | --- |
| Router | OpenWrt / Kwrt with SSH access |
| Local machine | Windows 10/11, macOS or Linux |
| Tools | Git, ssh, scp |
| Development deploy | Go, Node.js, npm, Python |
| Recommended user | Router `root` or equivalent privileges |

::: tip Safe default
ClashForge does not automatically take over transparent proxying or DNS on first boot. Configure and verify nodes first, then enable takeover manually.
:::

## 1. Clone the Project

```powershell
git clone https://github.com/wujun4code/clashforge.git
cd clashforge
```

## 2. Deploy from Windows

This command builds the UI, cross-compiles Go, creates an IPK package, uploads it to the router and installs it.

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy
```

Useful variants:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 -User root -Port 22 -Identity ~\.ssh\id_ed25519 deploy
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy -Skip ui
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy -Skip go
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy -Purge
```

## 3. Open the Web UI

```text
http://192.168.20.1:7777
```

## 4. First Setup

1. Open Setup in the Web UI.
2. Upload or paste a Clash-compatible YAML file, or add a subscription URL.
3. Save and activate the configuration.
4. Start the mihomo core.
5. Verify node connectivity before enabling transparent proxying or DNS takeover.

## 5. Quick Checks

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 status
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 check
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
```
