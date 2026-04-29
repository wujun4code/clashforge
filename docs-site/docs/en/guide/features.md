# Feature Modules

This page explains the main ClashForge modules: what each module does, what problem it solves, how to use it and what value it provides.

## 1. Router Management Console

**What it does**
ClashForge runs on OpenWrt and manages the mihomo core, runtime configuration, subscriptions, firewall takeover, DNS takeover, diagnostics and the embedded Web UI.

**Problem solved**
Without a router management console, every device needs its own proxy client and configuration. That creates inconsistent exits, stale subscriptions and hard-to-debug failures.

**How to use**
Install ClashForge on the router, then open `http://<router-ip>:7777`. Use the Web UI for daily work and the remote scripts for recovery, upgrade and diagnostics.

**Value**
Your network policy becomes centralized and observable. You know which config is active, which node is selected and whether DNS/firewall takeover is enabled.

## 2. Setup Wizard

**What it does**
The setup flow guides first-run configuration: import source, DNS settings, network takeover, core start and connectivity checks.

**Problem solved**
Router-level proxying can break LAN access when ports, DNS or firewall rules are wrong. A staged wizard reduces first-deploy risk.

**How to use**
Open Setup, import a Clash-compatible YAML or subscription, review generated config, start the core and validate before enabling takeover.

**Value**
You can see where startup fails instead of only seeing “the network is down”.

## 3. Configuration and Subscriptions

**What it does**
ClashForge stores YAML configs, fetches subscriptions and parses Clash/SS/Trojan/VLESS/VMess sources into mihomo-compatible config.

**Problem solved**
Teams often have primary subscriptions, backup subscriptions, temporary nodes and self-built nodes scattered across devices.

**How to use**
Go to Configuration, add subscription URLs, set name/User-Agent/update interval, update sources and activate the desired config.

**Value**
Source state, node count, update time and runtime config are visible in one place.

## 4. Per-device Routing

**What it does**
Create device groups by source IP/CIDR and override allowed proxies for specific policy groups.

**Problem solved**
Business devices, creator devices, phones, TVs and guests should not always share the same exit.

**How to use**
Assign stable DHCP leases, create device groups, add IPs/CIDRs, configure policy overrides and restart the generated config.

**Value**
Critical devices can stay on stable exits while entertainment and guest devices use cheaper shared nodes or direct access.

## 5. Egress Node Management

**What it does**
Manage airport subscription nodes, Cloudflare Worker VLESS-WS nodes and VPS/SSH nodes. VPS nodes can deploy GOST, bind Cloudflare DNS, issue TLS certificates and export Clash config.

**Problem solved**
Shared subscription IPs are not ideal for payment, ads, account backends or long-running AI workflows.

**How to use**
Open Nodes. Deploy Worker nodes with Cloudflare credentials or deploy VPS/SSH nodes through the guided SSH workflow.

**Value**
Self-built exits become part of the same node pool and can be routed, exported or published.

## 6. Subscription Publishing

**What it does**
Publish selected nodes and templates to Cloudflare Worker + KV as a versioned Clash-compatible subscription link.

**Problem solved**
Teams often need the same self-built nodes on laptops, phones and temporary clients outside the router network.

**How to use**
Open Publish, configure Cloudflare Worker/KV, choose built-in/runtime/custom template, select nodes, preview YAML and publish.

**Value**
You can distribute one managed subscription instead of manually copying node definitions.

## 7. Transparent Proxy and DNS Takeover

**What it does**
Apply tproxy/redir/tun/none modes and coordinate nftables/iptables with mihomo DNS and dnsmasq.

**Problem solved**
Router proxy failures are often caused by inconsistent firewall, policy routing and DNS behavior.

**How to use**
Start the core first, verify nodes, then enable transparent proxying and DNS takeover step by step.

**Value**
Devices can use proxy routing without installing local clients, while DNS and routing decisions stay aligned.

## 8. Rule Sets and GeoData

**What it does**
View rule-providers, sync rule files, search domain/IP matches and manage GeoIP/GeoSite data files.

**Problem solved**
Stale rules or missing GeoData cause incorrect direct/proxy routing.

**How to use**
Use Configuration for rule-providers and GeoData for `GeoIP.dat`/`GeoSite.dat` updates. Choose a proxy server for downloads when needed.

**Value**
Routing decisions become inspectable instead of guessed.

## 9. Connectivity Diagnostics

**What it does**
The dashboard compares router-side and browser-side probes, shows egress IPs, proxy groups, latency, connections and resources.

**Problem solved**
You need to know whether a failure is caused by the router, the browser, DNS, the selected node or the target service.

**How to use**
Run dashboard probes, switch selector nodes, run domain tests and inspect logs/connections.

**Value**
Troubleshooting has clear boundaries and evidence.

## 10. Operations and Recovery

**What it does**
The control script supports status, compat, check, stop, reset, upgrade, diag, uninstall and openclash checks.

**Problem solved**
When router takeover breaks networking, recovery must be fast and predictable.

**How to use**
Use `scripts\clashforgectl.ps1` on Windows, `scripts/clashforgectl` on macOS/Linux, or download `clashforgectl.sh` on the router and run it with `sh`.

**Value**
You can restore networking first, then collect redacted diagnostics and troubleshoot safely.
