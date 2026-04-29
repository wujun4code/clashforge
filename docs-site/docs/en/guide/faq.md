# FAQ

## Which installation method is recommended?

Use Release IPKs for production or long-term usage. Use `clashforgectl.ps1 deploy` for development validation because it builds, packages, uploads and installs an IPK from the current source tree.

## Why does ClashForge not automatically take over transparent proxying and DNS?

It is safer. A problem in the core, subscription, DNS or firewall layer can affect the whole LAN. Verify the core and nodes first, then enable takeover manually.

## What is the default Web UI address?

```text
http://<router-ip>:7777
```

Example:

```text
http://192.168.20.1:7777
```

## What does the Windows remote script require?

OpenSSH Client, meaning `ssh` and `scp` must be available. Windows 10/11 can install it from Optional Features.

## What is the difference between deploy and upgrade?

| Command | Purpose |
| --- | --- |
| `deploy` | Build current source locally, create an IPK and install it on the router |
| `upgrade` | Download a Release IPK, push it to the router and install it |

## How do I skip UI or Go builds?

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy -Skip ui
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy -Skip go
```

## How do I know it works?

Confirm the Web UI opens, `status` is healthy, mihomo is running, target clients use the expected egress IP, DNS follows the expected path and netfilter rules exist.

## How do I uninstall safely?

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 uninstall
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 uninstall -KeepConfig
```

## Can I paste diagnostic reports into GitHub Issues?

Prefer redacted reports:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
```

Unredacted reports may contain subscription URLs, tokens, internal addresses or other sensitive data.

## How is GitHub Pages published?

The included workflow builds and deploys whenever `docs-site/**` or the Pages workflow changes on `main`. In repository Settings → Pages, choose GitHub Actions as the source.
