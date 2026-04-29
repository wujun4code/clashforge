---
layout: home

hero:
  name: ClashForge
  text: OpenWrt mihomo Management Console
  tagline: Manage subscriptions, egress nodes, per-device routing, DNS takeover, diagnostics and recovery from your router.
  image:
    src: /logo.svg
    alt: ClashForge
  actions:
    - theme: brand
      text: Quick Start
      link: /en/guide/quick-start
    - theme: alt
      text: Feature Modules
      link: /en/guide/features
    - theme: alt
      text: Install Guide
      link: /en/guide/install

features:
  - title: Router-level control
    details: Run the management service, embedded Web UI, subscriptions, DNS and transparent proxy orchestration from OpenWrt.
  - title: Per-device egress policy
    details: Route workstations, creator devices, phones, TVs and guest clients through different exits.
  - title: Multiple node sources
    details: Combine airport subscriptions, Cloudflare Worker nodes and VPS/SSH nodes in one operating surface.
  - title: Safer rollout
    details: Start the core, verify nodes, then enable transparent proxying and DNS takeover gradually.
  - title: Operational recovery
    details: Remote scripts provide status, check, stop, reset, upgrade, diagnostics and uninstall flows.
  - title: Developer-friendly
    details: Go backend, React UI, REST API, SSE updates, OpenWrt IPK/APK release packaging and self-hosted distribution options.
---

## Who It Is For

ClashForge is not a proxy provider. It is useful when you already have proxy sources but need a reliable router-side management console.

| User | Typical pain | ClashForge value |
| --- | --- | --- |
| Cross-border e-commerce teams | Ads, payment and analytics tools are sensitive to egress identity | Bind business devices to stable exits |
| TikTok/YouTube creators | Creator tools, uploads and platform backends need predictable access | Separate creator devices from entertainment traffic |
| Heavy AI users | OpenAI, Claude, GitHub, npm and API workflows suffer from unstable shared IPs | Route work devices through cleaner or dedicated exits |
| Developers and studios | Many devices, many subscriptions, frequent troubleshooting | Centralized Web UI, logs, rules and diagnostics |
| Small network admins | No full-time IT, but the team needs stable overseas access | Router-managed policy and remote recovery commands |
| Multi-device homes | Phones, TVs and computers need different behavior | Per-device routing without installing clients everywhere |

## Recommended Reading Path

1. [Feature Modules](/en/guide/features)
2. [Quick Start](/en/guide/quick-start)
3. [Install & Deploy](/en/guide/install)
4. [First Configuration](/en/guide/config)
5. [Run & Takeover](/en/guide/run)
6. [Verification](/en/guide/verify)

## README vs Docs Site

The GitHub [README](https://github.com/wujun4code/clashforge#readme) is the technical project overview: architecture, components, source tree, build and release flow.

This docs site is the user manual: installation, configuration, routing strategy, operations and troubleshooting.
