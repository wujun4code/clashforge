# Troubleshooting

Recover networking first, then investigate. Avoid changing many settings while the LAN is already broken.

## First Step: Stop Takeover

Windows:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop
```

macOS / Linux:

```sh
./scripts/clashforgectl --router 192.168.20.1 stop
```

Router-local:

```sh
cd /tmp
wget -O clashforgectl.sh https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/clashforgectl.sh
sh clashforgectl.sh stop
```

Wait 10 to 20 seconds and test domestic/LAN access again.

## Web UI Does Not Open

Check:

```sh
/etc/init.d/clashforge status
/etc/init.d/clashforge restart
logread | grep -i clashforge
```

| Symptom | Possible cause | Action |
| --- | --- | --- |
| Wrong address | Router IP changed | Check default gateway |
| Timeout | Service not running | Restart init.d service |
| Port conflict | Port `7777` occupied | Inspect listening ports |
| SSH works but UI fails | Service/firewall issue | Collect diagnostics |

## Subscription Has No Nodes

Check URL completeness, trailing spaces, required User-Agent, account status and router outbound connectivity. Try opening the subscription URL from your computer first.

## Nodes Exist but Sites Fail

Most common causes:

| Cause | Signal | Action |
| --- | --- | --- |
| Bad node | Another node works | Switch node |
| Low IP reputation | reCAPTCHA or account verification | Use Worker or VPS for work devices |
| DNS/rule mismatch | Domestic/overseas routes look wrong | Update GeoData and rule-providers |
| Provider outage | All provider nodes fail | Check provider status |

## Full LAN Breaks After Takeover

Run `stop`, then restart in stages:

1. Start only the mihomo core.
2. Verify dashboard probes.
3. Test one device.
4. Enable transparent proxying.
5. Enable DNS takeover.
6. Expand to more devices.

## Per-device Routing Does Not Work

Check whether the client IP changed. Bind DHCP static leases for critical devices, update device groups, save, restart the generated config and verify egress IP again.

## Worker Node Fails

Check Cloudflare Worker existence, custom domain binding, API token permissions, request quota and whether the client config was exported after the latest deployment.

## VPS/SSH Deployment Fails

| Stage | Check |
| --- | --- |
| SSH | Host, port, user, key authorization |
| GOST | Server OS and permissions |
| Cloudflare DNS | Token permissions and Zone |
| TLS | Domain points to VPS, ports are reachable |
| Probe | Server firewall and DNS propagation |

## Collect Diagnostics

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
```

Share the redacted report only. Do not publish subscription URLs, Cloudflare tokens, SSH keys or passwords.
