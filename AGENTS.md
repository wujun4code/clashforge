# AGENTS.md

## Purpose

This file provides concise, actionable instructions and key links to help AI coding agents be immediately productive in the ClashForge codebase. It summarizes build, test, deployment, and architecture conventions, and links to detailed documentation where needed. 

---

## Key Documentation

- [User Docs (EN/中文)](https://wujun4code.github.io/clashforge/)
- [Feature Modules](https://wujun4code.github.io/clashforge/guide/features)
- [Quick Start (EN)](https://wujun4code.github.io/clashforge/en/guide/quick-start)
- [Install Guide (EN)](https://wujun4code.github.io/clashforge/en/guide/install)
- [Fork R2 Asset Hosting](docs/guides/fork-r2-setup.md)

---

## Build & Test

- **Go backend:**
  - Test: `go test ./...`
  - Build: `go build ./cmd/clashforge`
- **Web UI:**
  - Build: `cd ui && npm ci && npm run build`
- **Docs site:**
  - Build: `cd docs-site && npm ci && npm run docs:build`

---

## Deploy & Release

- **Normal upgrade:**
  - Windows: `.\scripts\clashforgectl.ps1 -Router <router-ip> upgrade`
  - macOS/Linux: `./scripts/clashforgectl --router <router-ip> upgrade`
- **Developer deploy (local source):**
  - Windows: `.\scripts\clashforgectl.ps1 -Router <router-ip> deploy`
- **Router-local install:**
  - `sh /tmp/clashforgectl.sh upgrade`
- **Release:**
  - Tag `v*` or run `.github/workflows/release.yml` to build UI, cross-compile Go, package IPK/APK, and upload assets.

---

## Project Structure

- `cmd/clashforge` — Main service entry
- `cmd/genconfig` — Config generator
- `internal/api` — HTTP API, SSE, embedded UI
- `internal/config` — Config generation, merging, device routing
- `internal/core` — mihomo process management
- `internal/dns` — DNS takeover, dnsmasq
- `internal/netfilter` — nftables/iptables, policy routing
- `internal/subscription` — Subscription management
- `internal/nodes` — Node deployment, certs
- `internal/workernode` — Cloudflare Worker nodes
- `internal/publish` — Subscription publishing
- `internal/geodata` — GeoIP/GeoSite
- `internal/scheduler` — Scheduled tasks
- `ui` — React + Vite Web UI
- `openwrt` — OpenWrt integration scripts
- `ipk` — IPK packaging tree
- `scripts` — Packaging, remote control scripts
- `docs-site` — VitePress user docs
- `docs` — Design docs, guides

---

## Conventions & Notes

- **Deployment:** Always use IPK packaging and install for production. Do not deploy by copying binaries unless explicitly requested.
- **Web UI:** Runs on port 7777 by default.
- **Runtime data:** `/etc/metaclash`, `/var/run/metaclash`
- **Security:** Never expose subscription links, Cloudflare tokens, or SSH keys. Use `diag -Redact` for public diagnostics.
- **OpenClash:** Not recommended to run alongside ClashForge.

---

## For More

- See [README.md](README.md) for full details and architecture diagrams.
- See [docs-site/README.md](docs-site/README.md) for docs site structure and build.
- See [docs/guides/fork-r2-setup.md](docs/guides/fork-r2-setup.md) for self-hosted release assets.

---

This file is maintained to help AI agents and contributors quickly understand and work with the ClashForge project. Update as conventions or structure change.
