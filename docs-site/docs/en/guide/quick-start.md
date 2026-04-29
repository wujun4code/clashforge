# Quick Start

This guide gets you to a verifiable ClashForge deployment. Examples use `192.168.20.1`; replace it with your OpenWrt router address.

::: tip Safe default
ClashForge does not force transparent proxy or DNS takeover on first boot. Start the core, verify nodes, then enable takeover gradually.
:::

## 0. Choose a Path

| Goal | Start with |
| --- | --- |
| Daily browsing and streaming | Airport subscription |
| Low-cost work exit | Cloudflare Worker node |
| Payment, ads, account backends or stable AI API | VPS/SSH node |
| Unsure | Import an existing YAML or subscription first |

## 1. Prerequisites

| Item | Requirement |
| --- | --- |
| Router | OpenWrt with SSH access |
| Local machine | Windows, macOS or Linux |
| Tools | Git, ssh, scp |
| Proxy source | Subscription, Clash YAML, Cloudflare account or VPS |

Check SSH:

```powershell
ssh root@192.168.20.1
```

## 2. Clone the Repository

```powershell
git clone https://github.com/wujun4code/clashforge.git
cd clashforge
```

## 3. Install

Windows:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade
```

macOS / Linux:

```sh
./scripts/clashforgectl --router 192.168.20.1 upgrade
```

Use a mirror when GitHub is slow:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Mirror https://ghproxy.com
```

Open the Web UI:

```text
http://192.168.20.1:7777
```

## 4. Import a Source

In the Web UI:

1. Open Setup or Configuration.
2. Add a Clash subscription URL, upload YAML, deploy a Worker node or deploy a VPS/SSH node.
3. Save and update the source.
4. Start the mihomo core.
5. Run dashboard probes.

## 5. Verify One Device First

Do not take over the entire LAN immediately.

1. Connect one test device to the router.
2. Enable the required takeover or device policy.
3. Open an overseas site.
4. Open a domestic site.
5. Check the egress IP.

## 6. Recover If Needed

Windows:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop
```

macOS / Linux:

```sh
./scripts/clashforgectl --router 192.168.20.1 stop
```

Router-local temporary use:

```sh
cd /tmp
wget -O clashforgectl.sh https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/clashforgectl.sh
sh clashforgectl.sh stop
```
