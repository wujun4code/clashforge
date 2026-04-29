# FAQ

## Is ClashForge a proxy provider?

No. ClashForge does not sell nodes or accounts. It manages proxy sources you already own: subscriptions, Cloudflare Worker nodes or VPS/SSH nodes.

## Do I need it for one computer?

Not necessarily. A desktop Clash/mihomo client is simpler for one temporary machine. ClashForge is more useful for many devices, router-level policy, per-device exits and operational recovery.

## Which exit type should I use?

| Type | Good for | Not good for |
| --- | --- | --- |
| Airport subscription | Browsing, streaming, backup lines | Payment, ads, stable account operations |
| Cloudflare Worker | Low-cost work access, general AI access, backup | Fixed IP or heavy traffic |
| VPS/SSH node | Stable dedicated egress | Zero-maintenance streaming-only use |

## Why do I still see reCAPTCHA?

Usually because of egress IP reputation. Shared subscription IPs are used by many people and can be flagged by target platforms. Switch nodes, try Worker for lightweight work or use VPS for critical workflows.

## Is Cloudflare Worker a fixed IP?

No. Worker exits belong to Cloudflare's network and are not fixed or dedicated. Use VPS/SSH nodes when you need a stable dedicated IP.

## Do all devices need local clients?

Usually no. Router-level takeover lets connected devices follow router policy. You may still use client subscriptions for mobile devices outside your home/office network.

## Where is the Web UI?

```text
http://<router-ip>:7777
```

Example:

```text
http://192.168.20.1:7777
```

## What is the difference between `upgrade` and `deploy`?

| Command | Audience | Behavior |
| --- | --- | --- |
| `upgrade` | Normal users | Install a Release IPK |
| `deploy` | Developers | Build local source, package an IPK and install it |

Normal users should use `upgrade`.

## Why are there remote scripts and router-local scripts?

| Command | Scenario |
| --- | --- |
| `.\scripts\clashforgectl.ps1` | Windows remote control |
| `./scripts/clashforgectl` | macOS/Linux remote control |
| `sh clashforgectl.sh` | Temporary router-local execution |

Current packages do not install `clashforgectl` into `/usr/bin` by default. You can copy it manually if you want a persistent router-local command.

## What if takeover breaks the LAN?

Run:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop
```

Recover first, then investigate DNS, transparent proxy, rules and device policies.

## What is subscription publishing?

It publishes selected VPS/Worker nodes and templates to Cloudflare Worker + KV as a Clash-compatible subscription link. This is useful for phones, laptops and team devices outside the router network.

## Can I share diagnostic reports?

Share only redacted reports:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
```

Never publish subscription URLs, Cloudflare tokens, SSH keys or passwords.

## Can ClashForge and OpenClash run together?

It is not recommended. Both may manage mihomo, ports, DNS, firewall and transparent proxy rules. Use `compat` and `openclash --kill` only when you intentionally want to remove OpenClash leftovers.
