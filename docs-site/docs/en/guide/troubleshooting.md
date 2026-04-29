# Troubleshooting

Protect network availability first, then collect evidence. If takeover breaks connectivity, stop takeover before deeper debugging.

## Restore Network Quickly

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop
```

Router-side:

```sh
clashforgectl stop
```

## Startup Failure

| Symptom | Possible cause | Action |
| --- | --- | --- |
| Web UI does not open | Service down or port unreachable | Check init status and `logread` |
| Repeated restart | Invalid config, PID lock or procd issue | Collect diagnostics and inspect logs |
| mihomo fails | Invalid YAML, port conflict or wrong binary path | Inspect generated config and mihomo logs |

```sh
/etc/init.d/clashforge status
logread | grep -i clashforge
logread | grep -i mihomo
ps | grep -E 'clashforge|mihomo'
```

## Port Conflicts

```sh
netstat -lntup | grep -E '7777|7890|7891|7892|7893|7895|7874|9090|17890|17891|17892|17893|17895|17874|19090'
```

## DNS Problems

```sh
nslookup example.com 127.0.0.1
nslookup github.com 127.0.0.1
logread | grep -i dnsmasq
logread | grep -i clashforge
```

Recommended approach:

1. Disable DNS takeover.
2. Confirm native OpenWrt dnsmasq works.
3. Enable mihomo DNS.
4. Re-enable dnsmasq cooperation.

## Transparent Proxy Not Working

```sh
nft list ruleset | grep -i clashforge
iptables-save | grep -i clashforge
```

Check firewall backend, LAN bypass rules, client gateway/DNS and stale rules.

## Subscription Update Failure

Common causes:

1. Router time is wrong, breaking TLS.
2. DNS cannot resolve the subscription host.
3. The provider requires a specific User-Agent.
4. Router direct access cannot reach the subscription endpoint.

```sh
date
nslookup github.com
logread | grep -i subscription
```

## Diagnostic Report

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
```

## Before Opening an Issue

Provide version, OpenWrt/Kwrt version, architecture, install method, redacted diagnostics, reproduction steps and whether transparent proxy or DNS takeover was enabled.
