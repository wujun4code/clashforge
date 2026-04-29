# Verification

Run this checklist after every install, upgrade, configuration change or takeover change.

## 1. Service State

Windows remote:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 status
```

Router-side:

```sh
/etc/init.d/clashforge status
ps | grep clashforge
```

Expected:

| Item | Healthy state |
| --- | --- |
| ClashForge service | Running |
| Web UI | `http://<router-ip>:7777` opens |
| mihomo core | Starts after configuration |
| Logs | No continuous crash loop or port conflict |

## 2. Web UI and API

```text
http://192.168.20.1:7777
```

If checking with curl, use the actual API paths exposed by your version:

```sh
curl -s http://127.0.0.1:7777/api/v1/health
curl -s http://127.0.0.1:7777/api/v1/status
curl -s http://127.0.0.1:7777/api/v1/version
```

## 3. Connectivity Check

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 check
```

Router-side:

```sh
clashforgectl check
```

Check reachability, router egress IP, DNS resolution and client egress after takeover.

## 4. DNS

```sh
nslookup example.com 127.0.0.1
nslookup github.com 127.0.0.1
logread | grep -i dns
```

## 5. netfilter

```sh
nft list ruleset | grep -i clashforge
iptables-save | grep -i clashforge
```

## 6. Processes and Ports

```sh
ps | grep -E 'clashforge|mihomo'
netstat -lntup | grep -E '7777|7890|7891|7892|7893|7895|7874|9090'
```

## 7. Diagnostics

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -LocalPath .\cf-diag.txt -Redact
```

::: warning Unredacted reports
Reports without `-Redact` may contain subscription URLs, tokens or other sensitive data.
:::
