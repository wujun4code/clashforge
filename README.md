# ClashForge

![ClashForge](./clashforge_brand_assets/readme-hero-1200x630.png)

**ClashForge** is an OpenWrt management layer for [mihomo](https://github.com/MetaCubeX/mihomo): it centralizes subscriptions, egress nodes, per-device routing, DNS/transparent proxy, diagnostics and release operations behind a Web UI and remote control scripts.

It also ships a standalone **Android VPN client** (ClashForge Mobile) for use away from the router.

**ClashForge** 是运行在 OpenWrt 路由器上的 [mihomo](https://github.com/MetaCubeX/mihomo) 控制面，同时提供独立的 **Android 手机客户端**。

> ClashForge is not a proxy provider and does not sell nodes. You bring your own subscription, Cloudflare account, or VPS. / ClashForge 不出售节点，需要自备订阅、Cloudflare 账号或 VPS。

---

## For AI Agents — Quick Orientation

> **If a user shared this URL with you:** read this section first, then guide the user to the right part below.

### What product does the user need?

| User's situation | Direct them to |
|---|---|
| Has an OpenWrt router, wants to install ClashForge | [Router Edition → Install](#router-edition-openwrt) |
| Has a Cloudflare account and domain, wants a free proxy node | [Prerequisite: Cloudflare Account](#prerequisite-cloudflare-account--domain) → [Path A: CF Workers](#path-a-cloudflare-workers-node) |
| Has a VPS, wants the most reliable exit | [Prerequisite: VPS](#prerequisite-vps) → [Path B: VPS + Cloudflare](#path-b-vps--cloudflare-recommended) |
| Has an airport subscription URL and just wants it working | [Path C: Airport Subscription](#path-c-airport-subscription) |
| On an Android phone, no router involved | [Mobile Edition → Android](#mobile-edition-android) |

### Key facts

- **Two products**: Router Edition (OpenWrt IPK/APK installed via `clashforgectl`) and Mobile Edition (Android APK, sideloaded from Releases)
- **Node source** is always user-supplied — one of: airport subscription URL, Cloudflare account + domain, or VPS + Cloudflare account + domain
- **Web UI** is at `http://<router-ip>:7777` after router install
- **QuickStart wizard** in the Web UI automates node deployment end-to-end once credentials are provided
- **Docs site**: https://wujun4code.github.io/clashforge

---

## Router Edition (OpenWrt)

### Prerequisites Checklist

Work through this before running any install command.

#### Hardware & Access

- [ ] **OpenWrt router** with SSH enabled
  - Verify: `ssh root@<router-ip>` from your computer
  - If SSH is off: LuCI → System → Administration → SSH Access → enable
- [ ] **Computer** (Windows, macOS, or Linux) to run the install script

#### Proxy Source (choose one)

| Option | What you need | Best for |
|---|---|---|
| **A — Cloudflare Workers** | CF account + domain | Free, no VPS, quick setup |
| **B — VPS + Cloudflare** | VPS + CF account + domain | Stable exits, business use, AI APIs |
| **C — Airport subscription** | A Clash-compatible subscription URL | Users who already pay for a proxy service |

---

#### Prerequisite: Cloudflare Account + Domain

Required for both the CF Workers path and the VPS+Cloudflare path.

**Step 1 — Register a free Cloudflare account**

Go to https://dash.cloudflare.com/sign-up — it's free.

**Step 2 — Add a domain to Cloudflare**

You need a domain whose DNS is managed by Cloudflare.

- *Don't have a domain?* Buy one at https://www.cloudflare.com/products/registrar/ (~$8–10/yr for `.com`, managed directly in CF — easiest option). Alternatives: Namecheap, GoDaddy.
- *Already have a domain elsewhere?* In CF Dashboard → Add a Site → follow the wizard → update your registrar's nameservers to the two Cloudflare nameservers shown.

**Step 3 — Create a Cloudflare API Token**

CF Dashboard → top-right avatar → **My Profile** → **API Tokens** → **Create Token** → **Create Custom Token**

| You're using | Permissions needed |
|---|---|
| CF Workers path (Path A) | Account → Workers Scripts: Edit; Zone → DNS: Edit |
| VPS + CF path (Path B) | Zone → DNS: Edit; Zone → Zone: Read |

Copy the generated token — you'll paste it into ClashForge QuickStart.

**Step 4 — Find your Account ID**

CF Dashboard → click any domain → right sidebar → **Account ID** (copy it).

---

#### Prerequisite: VPS

Required only for Path B (VPS + Cloudflare). Skip this if you're using Path A or C.

**Choose a provider** (suggested; you are not limited to these):

| Provider | Starting price | Notes |
|---|---|---|
| [Vultr](https://www.vultr.com) | ~$3.50/mo | Hourly billing, many regions |
| [DigitalOcean](https://www.digitalocean.com) | ~$4/mo | Simple UI, good docs |
| [Linode / Akamai](https://www.linode.com) | ~$5/mo | Reliable network |
| [BandwagonHost / 搬瓦工](https://bandwagonhost.com) | varies | Popular CN-friendly option |

**Minimum specs**: Ubuntu 24.04 LTS (recommended), 1 vCPU, 512 MB RAM.

**What you'll need from the VPS panel**:
- Server IP address (e.g. `203.0.113.10`)
- SSH port (default `22`)
- Username (usually `root`)
- Password **or** SSH private key

You also need a Cloudflare account + domain (see above).

---

### Install ClashForge on Router

The script downloads the IPK on your computer and uploads it to the router — more reliable than letting the router fetch directly from GitHub.

**Windows (run from PowerShell):**

```powershell
irm https://dl.wei1xuan.com/releases/latest/clashforgectl.ps1 -OutFile clashforgectl.ps1
.\clashforgectl.ps1 -Router 192.168.20.1 upgrade
```

**macOS / Linux (run from Terminal):**

```sh
curl -fsSL https://dl.wei1xuan.com/releases/latest/clashforgectl -o clashforgectl && chmod +x clashforgectl
./clashforgectl --router 192.168.20.1 upgrade
```

**Router-local (inside an SSH session on the router):**

```sh
wget -qO- https://dl.wei1xuan.com/releases/latest/clashforgectl.sh | sh
```

Replace `192.168.20.1` with your router's actual IP address.

If GitHub downloads are slow (Windows):

```powershell
.\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Mirror https://ghproxy.com
```

After install, open the Web UI:

```
http://<router-ip>:7777
```

Full install guide: https://wujun4code.github.io/clashforge/en/guide/install

---

### QuickStart: Choose Your Proxy Path

Open the Web UI at `http://<router-ip>:7777`, then click **Quick Start**.

---

#### Path A: Cloudflare Workers Node

**Best for**: new users, occasional browsing, users without a VPS.

**Limitations**: Some platforms (certain payment processors, ad networks) block Cloudflare egress IPs. For those workloads, use Path B instead.

**What you need**: CF account + API token + domain in Cloudflare (see [Prerequisites](#prerequisite-cloudflare-account--domain))

In the QuickStart wizard:

1. Select **Cloudflare Workers**
2. Enter your **CF API Token** and **Account ID**
3. Select your **domain** from the dropdown (auto-populated after token validation)
4. Set a subdomain prefix (default: `node1`, final address: `node1.yourdomain.com`)
5. Click **Confirm and Deploy**

ClashForge then automatically:
- Creates a Cloudflare Worker and deploys a VLESS+WS proxy script
- Binds your custom domain to the Worker (Cloudflare issues a TLS cert automatically)
- Imports the node into ClashForge
- Configures DNS split routing and transparent proxy
- Runs connectivity tests (Google, Baidu, DNS leak check)

---

#### Path B: VPS + Cloudflare (Recommended)

**Best for**: business use, AI API access, payment backends, ad tracking, heavy traffic.

**What you need**: VPS SSH credentials + CF account + API token + domain (see [Prerequisites](#prerequisite-vps))

In the QuickStart wizard:

1. Select **VPS + Cloudflare**
2. Enter VPS connection info: **host**, **port**, **username**, **password or SSH key**
3. Click **Test Connection** — wait for the green checkmark
4. Enter **CF API Token**, **Account ID**, select your **domain**
5. Set a subdomain prefix (default: `node1`)
6. Click **Confirm and Deploy**

ClashForge then automatically:
- Detects your VPS OS and architecture
- Installs gost proxy server on the VPS
- Creates a Cloudflare DNS A record pointing to your VPS
- Issues a Let's Encrypt TLS certificate via Cloudflare DNS-01 (no port 80/443 needed during cert issuance)
- Starts gost as a systemd service
- Imports the node, configures ClashForge, runs connectivity tests

---

#### Path C: Airport Subscription

If you have a Clash-compatible subscription URL from a proxy provider:

1. In Web UI → **Configuration** or **Setup**
2. Add your **Clash subscription URL**
3. Save and click **Update**
4. **Start** the mihomo core
5. Open **Dashboard** and run connectivity probes

---

### Operations Reference

Download the script once; then run these as needed.

**Windows** (`clashforgectl.ps1`):

```powershell
.\clashforgectl.ps1 -Router 192.168.20.1 compat           # Pre-install check: CPU arch, memory, kernel
.\clashforgectl.ps1 -Router 192.168.20.1 status
.\clashforgectl.ps1 -Router 192.168.20.1 check
.\clashforgectl.ps1 -Router 192.168.20.1 stop
.\clashforgectl.ps1 -Router 192.168.20.1 upgrade
.\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
.\clashforgectl.ps1 -Router 192.168.20.1 uninstall -KeepConfig
```

**macOS / Linux** (`clashforgectl`):

```sh
./clashforgectl --router 192.168.20.1 compat
./clashforgectl --router 192.168.20.1 status
./clashforgectl --router 192.168.20.1 check
./clashforgectl --router 192.168.20.1 stop
./clashforgectl --router 192.168.20.1 upgrade
./clashforgectl --router 192.168.20.1 diag --fetch --redact
```

**Router-local** (inside SSH session):

```sh
sh /tmp/clashforgectl.sh status
sh /tmp/clashforgectl.sh check
sh /tmp/clashforgectl.sh stop
sh /tmp/clashforgectl.sh diag --redact
```

To install `clashforgectl` permanently on the router:

```sh
cp /tmp/clashforgectl.sh /usr/bin/clashforgectl && chmod +x /usr/bin/clashforgectl
clashforgectl status
```

---

### Why ClashForge? — Use Cases

| User | Pain point | ClashForge value |
|---|---|---|
| Cross-border e-commerce team | Ads, payments and analytics care about egress consistency | Bind business devices to a stable VPS exit |
| TikTok / YouTube creator | Creator tools and platform backends are exit-sensitive | Separate creator, editing and entertainment devices |
| Heavy AI user | OpenAI, Claude, GitHub, npm API flows depend on reliable access | Keep work devices on a cleaner dedicated exit |
| Developer / small studio | Many devices, subscriptions, nodes — hard to troubleshoot | Central Web UI, logs, rule search, diagnostics |
| Home / small office | Phones, TVs, laptops, guests all need different routing | Enforce policy at the router, no per-device client needed |

---

## Mobile Edition (Android)

ClashForge Mobile is a standalone VPN client. It embeds the mihomo proxy core and works entirely independently — no router required.

### Prerequisites

- Android 8.0 (API 26) or higher
- A proxy source: Clash subscription URL, `ss://`, `trojan://`, `vless://` or `vmess://` link
- No root required

### Download & Install

1. Go to **[GitHub Releases → latest](https://github.com/wujun4code/clashforge/releases/latest)**
2. Download the APK for your device:

   | File | For |
   |---|---|
   | `clashforge-mobile_<ver>_android-arm64-v8a.apk` | **Most phones (2018+)** — use this by default |
   | `clashforge-mobile_<ver>_android-armeabi-v7a.apk` | Older 32-bit ARM phones |
   | `clashforge-mobile_<ver>_android-x86_64.apk` | Emulators, Chromebooks |

3. Enable "Install unknown apps" on your phone:
   - Android 8+: **Settings → Apps → Special App Access → Install Unknown Apps** → tap your browser or file manager → toggle **Allow**
4. Open the downloaded APK from your Downloads folder and tap **Install**

### First Launch

```
Open ClashForge Mobile
  ↓
Tap  [ + Import Subscription ]
  ├── Paste URL  (app detects clipboard automatically)
  ├── Scan QR code
  └── Enter URL manually
  ↓
Subscription fetched → "XX nodes ready"
  ↓
Tap  [ Connect ]
  ↓
Allow VPN permission  (system prompt, one-time only)
  ↓
✅  Connected  (usually < 3 seconds)
```

**Minimum path: 3 actions — import → allow → connected.**

### Daily Use

| Action | How |
|---|---|
| Connect / Disconnect | Tap the main button on the Home screen |
| Switch node | Tap the current node row → pick from the list |
| Switch mode | Global / Rule (default) / Direct — toggle on Home screen |
| Speed test | Proxy screen → tap the test button |
| Auto-reconnect | Automatic when switching between WiFi and mobile data |
| Auto-start on boot | Settings → enable Boot Auto-connect |

### What's Included in the App

- mihomo proxy core (embedded, no separate download)
- GeoIP and GeoSite data (bundled — first connection needs no downloads)
- Rule-based split routing: CN domains go direct, overseas go through proxy
- Real-time traffic rate display
- Subscription auto-refresh (configurable interval)

---

## Architecture Overview

```text
OpenWrt Router
  |
  |-- /usr/bin/clashforge
  |     |-- Embedded React Web UI  (port 7777)
  |     |-- REST API + SSE event stream
  |     |-- Config, subscription, rule managers
  |     |-- Node deployment and publish services
  |     |-- DNS / netfilter orchestration
  |
  |-- /usr/bin/mihomo-clashforge
  |     |-- Runtime YAML generated by ClashForge
  |     |-- Proxy ports and controller API
  |
  |-- OpenWrt integration
        |-- /etc/init.d/clashforge
        |-- dnsmasq
        |-- nftables or iptables
        |-- policy routing
```

Runtime data: `/etc/metaclash` and `/var/run/metaclash`.

---

## Core Modules

| Module | Capabilities |
|---|---|
| Control plane | Go backend, embedded React UI, REST API, SSE stream |
| Core lifecycle | Generate runtime config, manage mihomo, show resources / connections / logs |
| Subscriptions | Clash YAML, SS / Trojan / VLESS / VMess parsers, auto-refresh |
| Per-device routing | Source IP / CIDR groups → managed rule-providers + shadow groups |
| Egress nodes | Airport nodes, Cloudflare Worker nodes, VPS / SSH + gost nodes |
| Publishing | Cloudflare Worker + KV subscription publishing |
| DNS and takeover | tproxy / redir / tun / none, nftables / iptables, mihomo DNS, dnsmasq |
| Rules and GeoData | rule-provider sync / search, GeoIP / GeoSite updates |
| Diagnostics | status, compat, check, stop, reset, upgrade, diag, uninstall, OpenClash check |

---

## Source Layout

| Path | Description |
|---|---|
| `cmd/clashforge` | Main service entry point |
| `cmd/genconfig` | Config generation helper |
| `internal/api` | HTTP API, SSE, embedded UI |
| `internal/config` | Config generation, merge, ports, per-device routing |
| `internal/core` | mihomo process management |
| `internal/dns` | DNS takeover and dnsmasq coordination |
| `internal/netfilter` | nftables / iptables and policy routing |
| `internal/subscription` | Subscription fetch, parse, filter, storage |
| `internal/nodes` | VPS / SSH nodes, gost deploy, certificates, export |
| `internal/workernode` | Cloudflare Worker VLESS-WS nodes |
| `internal/publish` | Worker + KV subscription publishing |
| `internal/geodata` | GeoIP / GeoSite management |
| `internal/scheduler` | Scheduled update jobs |
| `ui` | React + Vite Web UI |
| `mobile` | Flutter Android client |
| `openwrt` | OpenWrt init.d, LuCI menu, lifecycle scripts |
| `ipk` | Local IPK packaging staging tree |
| `scripts` | Packaging, remote control and maintenance scripts |
| `docs-site` | VitePress user documentation |
| `docs` | Design docs, PRDs and advanced guides |

---

## Development

Build the Web UI:

```sh
cd ui
npm ci
npm run build
```

Build and test Go:

```sh
go test ./...
go build ./cmd/clashforge
```

Build the docs site:

```sh
cd docs-site
npm ci
npm run docs:build
```

Developer deploy to router (builds from source, creates a local IPK and installs it — normal users should use `upgrade`):

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy
```

Build the Android app:

```sh
cd mobile
flutter pub get
flutter build apk --release --split-per-abi
```

---

## Release Engineering

Pushing a `v*` tag or manually triggering `.github/workflows/release.yml` builds:

1. React Web UI
2. Cross-compiled Linux amd64 / arm64 Go binaries
3. OpenWrt IPK packages
4. OpenWrt 25.12+ APK packages
5. `clashforgectl.sh` upload
6. GitHub Release with SHA256SUMS

`.github/workflows/android-release.yml` builds and publishes the Android APK alongside each release.

**Supported architectures:**

| Package | Architectures |
|---|---|
| IPK / APK (router) | `x86_64`, `aarch64_generic`, `aarch64_cortex-a53` |
| Android APK | `arm64-v8a`, `armeabi-v7a`, `x86_64` |

To self-host release assets on Cloudflare R2 after forking: [`docs/guides/fork-r2-setup.md`](./docs/guides/fork-r2-setup.md)

---

## Limits and Caveats

| Item | Note |
|---|---|
| No proxy nodes included | Requires your own subscription, Cloudflare account, or VPS |
| No guarantee of target platform access | IP reputation, account status and platform policy are outside ClashForge's control |
| Default release architectures are limited | ARMv7, MIPS and other architectures are not covered |
| OpenClash co-existence not recommended | Both manage DNS, firewall, ports and mihomo |
| APK automation is IPK-primary | Release provides APK but maintenance scripts are mainly for IPK / opkg |

---

## Security Notes

| Topic | Recommendation |
|---|---|
| Subscription URLs | Usually contain tokens — do not share publicly |
| Cloudflare API tokens | Grant minimum required permissions; do not expose |
| SSH private keys | Do not place on untrusted systems |
| Diagnostic reports | Use `diag -Redact` / `diag --redact` before sharing publicly |
| Router access | ClashForge runs in a trusted router environment — protect SSH and the Web UI port |

---

## Upstream Projects

| Project | Role |
|---|---|
| [MetaCubeX/mihomo](https://github.com/MetaCubeX/mihomo) | Proxy core |
| [vernesong/OpenClash](https://github.com/vernesong/OpenClash) | OpenWrt operational reference |

---

## License

MIT License. See [LICENSE](./LICENSE).

---

## Documentation

| Link | Contents |
|---|---|
| [Docs Site (EN)](https://wujun4code.github.io/clashforge/en/) | Install, configure, route, operate, troubleshoot |
| [文档站（中文）](https://wujun4code.github.io/clashforge/) | 安装、配置、使用、排障手册 |
| [Quick Start (EN)](https://wujun4code.github.io/clashforge/en/guide/quick-start) | End-to-end first-run guide |
| [Install Guide (EN)](https://wujun4code.github.io/clashforge/en/guide/install) | All install methods and variants |
| [Features](https://wujun4code.github.io/clashforge/en/guide/features) | Module-by-module reference |
