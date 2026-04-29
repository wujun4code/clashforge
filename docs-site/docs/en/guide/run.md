# Run & Takeover

ClashForge separates the management service, the mihomo core and transparent/DNS takeover. This makes first deployment safer.

## Start the Service

```sh
/etc/init.d/clashforge enable
/etc/init.d/clashforge start
/etc/init.d/clashforge status
```

Remote status from Windows:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 status
```

## Start the Core from Web UI

Open:

```text
http://192.168.20.1:7777
```

Then:

1. Import or activate a configuration.
2. Start the mihomo core.
3. Confirm PID, uptime, CPU, memory and connection count.

## Enable Transparent Proxying

Enable takeover from the overview or settings page only after node validation.

Check:

| Item | Expected result |
| --- | --- |
| nftables/iptables | Rules applied without errors |
| Policy routing | Related rules and tables exist |
| LAN bypass | Router management stays reachable |
| Egress IP | Target clients use expected exit |

## Enable DNS Takeover

```sh
nslookup example.com 127.0.0.1
logread | grep -i clashforge
```

::: tip Step-by-step takeover
Start the core first, then transparent proxying, then DNS. Run verification after each step.
:::

## Stop and Exit Takeover

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop
```

Router-side:

```sh
clashforgectl stop
```

## Reset

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 reset
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 reset -Start
```
