# Upgrade & Rollback

ClashForge upgrades and rollbacks should be IPK based for predictable version boundaries.

## Upgrade to Latest

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade
```

Router-side:

```sh
clashforgectl upgrade
```

The Windows path downloads the IPK locally and pushes it to the router by default, avoiding router-side download failures after proxy services stop.

## Upgrade to a Specific Version

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Version v0.1.0-rc.1
```

Router-side:

```sh
clashforgectl upgrade --version v0.1.0-rc.1
```

## Mirrors and Custom Sources

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Mirror https://ghproxy.com
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -BaseUrl https://releases.example.com
```

## Purge Upgrade

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Purge
```

::: danger Be careful with purge
`-Purge` may remove existing configuration, subscriptions and runtime data. Use it only for clean reinstalls or when old state is suspected.
:::

## Rollback

Keep the last two or three known-good IPK packages. Rollback flow:

```sh
clashforgectl stop
opkg remove clashforge
opkg install /tmp/clashforge_0.1.0-rc.1_x86_64.ipk
/etc/init.d/clashforge enable
/etc/init.d/clashforge start
```

## Post-upgrade Checks

1. Web UI opens.
2. `status` is healthy.
3. Only one mihomo instance is active.
4. No port conflicts.
5. DNS and transparent proxy takeover match expectations.
6. Subscriptions, rule sets and overrides still work.
