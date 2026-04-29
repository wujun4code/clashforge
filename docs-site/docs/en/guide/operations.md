# Operations

Daily operations should be boring: observe, update, switch, verify and recover when necessary.

## Daily Map

| Goal | Recommended entry |
| --- | --- |
| Check health | Dashboard |
| Update subscription | Configuration -> Subscriptions |
| Switch node | Dashboard proxy groups |
| Inspect connections | Activity -> Connections |
| Inspect logs | Activity -> Logs |
| Update GeoData | GeoData |
| Adjust device exits | Per-device Rules |
| Publish team subscription | Publish |
| Recover network | `stop` command |
| Collect diagnostics | redacted `diag` command |

## Common Commands

Windows:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 status
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 check
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
```

macOS / Linux:

```sh
./scripts/clashforgectl --router 192.168.20.1 status
./scripts/clashforgectl --router 192.168.20.1 check
./scripts/clashforgectl --router 192.168.20.1 stop
./scripts/clashforgectl --router 192.168.20.1 diag --fetch --redact
```

Router-local temporary use:

```sh
cd /tmp
wget -O clashforgectl.sh https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/clashforgectl.sh
sh clashforgectl.sh status
sh clashforgectl.sh check
sh clashforgectl.sh stop
sh clashforgectl.sh diag --redact
```

Optional persistent command:

```sh
cp /tmp/clashforgectl.sh /usr/bin/clashforgectl
chmod +x /usr/bin/clashforgectl
clashforgectl status
```

## Subscription Maintenance

Update subscriptions when node count changes, a node stops working or the provider announces updates. If update fails, check URL, User-Agent, account status and router outbound connectivity.

## Node Switching

Use dashboard proxy groups to run latency checks and switch Selector nodes. After switching, run router-side and browser-side probes to verify the egress IP changed as expected.

## Device Policy Maintenance

Keep critical devices on DHCP static leases. When adding a new workstation or creator device, place it in a test group first, verify egress, then move it into the production group.

## GeoData and Rule Maintenance

Update `GeoIP.dat` and `GeoSite.dat` regularly, for example weekly. Use a proxy server for downloads if the router cannot reach GitHub directly.

## Uninstall

Keep config:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 uninstall -KeepConfig
```

Full uninstall:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 uninstall
```
