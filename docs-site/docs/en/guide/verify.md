# Verification

A successful deployment means the target devices can access required overseas resources, domestic and LAN access still works, and egress IPs match expectations.

## Verification Matrix

| Layer | How to verify | Expected result |
| --- | --- | --- |
| Web UI | Open `http://<router-ip>:7777` | UI loads |
| Core | Dashboard or `status` | PID and uptime are present |
| Source | Configuration page | Nodes and subscriptions are visible |
| Node | Latency/probe | At least one node works |
| Transparent proxy | Test device browser | Overseas sites work without local proxy client |
| DNS | Domestic/overseas domains | Routing matches rules |
| Per-device policy | Egress IP checks | Different devices use expected exits |

## Command Checks

Windows:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 status
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 check
```

macOS / Linux:

```sh
./scripts/clashforgectl --router 192.168.20.1 status
./scripts/clashforgectl --router 192.168.20.1 check
```

Router-local:

```sh
cd /tmp
wget -O clashforgectl.sh https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/clashforgectl.sh
sh clashforgectl.sh status
sh clashforgectl.sh check
```

## Browser Checks

From a client connected to the router:

| Test | Expected |
| --- | --- |
| Overseas site | Opens normally |
| Domestic site | Opens without obvious detour |
| Egress IP | Matches selected node or device policy |
| LAN service | Router/NAS/printer remains reachable |

## Dashboard Probes

| Probe | Meaning |
| --- | --- |
| Router-side | Request originates from the OpenWrt router |
| Browser-side | Request originates from the current browser/client |

If router-side succeeds but browser-side fails, the client may not be under takeover. If both fail, inspect node, DNS and router connectivity.

## Per-device Routing Checks

1. Confirm each critical device has a stable IP.
2. Query egress IP from each device.
3. Check active connections in the Activity page.
4. Search rules for important domains.

## Diagnostics

Windows:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
```

macOS / Linux:

```sh
./scripts/clashforgectl --router 192.168.20.1 diag --fetch --redact
```
