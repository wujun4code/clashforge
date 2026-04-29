# Run & Takeover

ClashForge separates the management service, the mihomo core and LAN takeover. This staged model keeps first deployment safer.

## Three Layers

| Layer | Purpose | Recommendation |
| --- | --- | --- |
| Management service | Web UI and API on port `7777` | Installed and started automatically |
| mihomo core | Loads runtime YAML and provides proxy/API ports | Start after importing a valid source |
| Transparent/DNS takeover | Routes LAN clients through mihomo | Enable after verification |

## Check the Management Service

```text
http://192.168.20.1:7777
```

Remote status:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 status
```

Router service:

```sh
/etc/init.d/clashforge status
/etc/init.d/clashforge restart
```

## Start the Core

In the Web UI:

1. Open Setup or Configuration.
2. Select a config source.
3. Preview the generated runtime config.
4. Start mihomo.
5. Confirm PID, uptime, CPU, memory, proxy groups and logs.

## Verify Before LAN Takeover

Use the dashboard:

1. Run router-side probe.
2. Run browser-side probe.
3. Compare egress IPs.
4. Switch a Selector node.
5. Run a domain test.

If probes fail here, do not enable full takeover yet.

## Enable Transparent Proxying

**What it does**
Routes client traffic into mihomo without configuring every device manually.

**How to use**
Enable takeover from Setup or Advanced Management after the core is healthy. Keep LAN bypass enabled and test one device first.

**Value**
Phones, TVs and other devices can follow router policy without local clients.

## Enable DNS Takeover

**What it does**
Keeps DNS resolution aligned with mihomo routing rules.

**How to use**
Enable DNS after transparent proxying is stable. Choose fake-ip or redir-host and the dnsmasq coexistence mode from Advanced Management.

**Value**
Reduces DNS leaks and incorrect rule matches.

## Stop and Recover

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
