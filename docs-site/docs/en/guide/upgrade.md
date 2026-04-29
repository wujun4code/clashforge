# Upgrade & Rollback

Use Release packages for normal upgrades. Remote upgrade from your computer is recommended because the package is downloaded locally and then uploaded to the router.

## Before Upgrading

| Check | Reason |
| --- | --- |
| Current network is healthy | Recover first, upgrade later |
| You know the `stop` command | Recovery path must be clear |
| Important config is backed up | Protect subscriptions and overrides |
| Router has enough space | Package install needs `/tmp` and overlay space |
| Target version is known | Required for rollback |

## Upgrade

Windows:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade
```

macOS / Linux:

```sh
./scripts/clashforgectl --router 192.168.20.1 upgrade
```

Mirror:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Mirror https://ghproxy.com
```

Specific version:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Version v0.1.0-rc.1
```

## Router-local Upgrade

```sh
cd /tmp
wget -O clashforgectl.sh https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/clashforgectl.sh
sh clashforgectl.sh upgrade
```

## Verify After Upgrade

1. Open `http://192.168.20.1:7777`.
2. Confirm service and core status.
3. Test one overseas site and one domestic site.
4. Check egress IP.
5. Inspect Activity logs for repeated errors.

## Rollback

Recover first:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop
```

Then install a previous version:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Version v0.1.0-rc.1
```

## Purge Upgrade

Use only when you intentionally want a clean rebuild:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Purge
```

`Purge` can remove local data. It is not a normal upgrade path.

## APK Note

Release assets include APK packages for OpenWrt 25.12+, but the automation currently focuses on IPK/opkg. If your system uses APK, download the matching asset and install manually:

```sh
apk add --allow-untrusted /tmp/clashforge-<version>_<arch>.apk
```
