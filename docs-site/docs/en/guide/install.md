# Install & Deploy

This page describes production installation, router-local installation, manual packages and developer deployment.

## What Gets Installed

| Component | Purpose |
| --- | --- |
| `clashforge` | Backend service with embedded Web UI and API |
| `mihomo-clashforge` | Bundled mihomo core |
| `/etc/init.d/clashforge` | OpenWrt service entry |
| `/etc/metaclash` | Config, subscriptions, rules and runtime data |
| Web UI | `http://<router-ip>:7777` |
| `clashforgectl.sh` | Release asset used by remote scripts and router-local maintenance |

::: warning
Current IPK/APK packages do not install `clashforgectl` into `/usr/bin` by default. Remote wrappers upload `clashforgectl.sh` temporarily. For router-local long-term usage, copy it manually.
:::

## Supported Packages

| Package | Target | Architectures |
| --- | --- | --- |
| IPK | Mainstream OpenWrt | `x86_64`, `aarch64_generic`, `aarch64_cortex-a53` |
| APK | OpenWrt 25.12+ | `x86_64`, `aarch64_generic`, `aarch64_cortex-a53` |

ARMv7, MIPS and older devices are not covered by the default release packages.

## Windows Remote Install

```powershell
git clone https://github.com/wujun4code/clashforge.git
cd clashforge
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade
```

Variants:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 -Port 2222 upgrade
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 -Identity ~\.ssh\id_ed25519 upgrade
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Version v0.1.0-rc.1
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Mirror https://ghproxy.com
```

## macOS / Linux Remote Install

```sh
git clone https://github.com/wujun4code/clashforge.git
cd clashforge
./scripts/clashforgectl --router 192.168.20.1 upgrade
```

Variants:

```sh
./scripts/clashforgectl --router 192.168.20.1 --port 2222 upgrade
./scripts/clashforgectl --router 192.168.20.1 --identity ~/.ssh/id_ed25519 upgrade
./scripts/clashforgectl --router 192.168.20.1 upgrade --version v0.1.0-rc.1
./scripts/clashforgectl --router 192.168.20.1 upgrade --mirror https://ghproxy.com
```

## Router-local Install

```sh
cd /tmp
wget -O clashforgectl.sh https://raw.githubusercontent.com/wujun4code/clashforge/main/scripts/clashforgectl.sh
sh clashforgectl.sh compat
sh clashforgectl.sh upgrade
```

Mirror:

```sh
sh clashforgectl.sh upgrade --mirror https://ghproxy.com
```

Optional persistent command:

```sh
cp /tmp/clashforgectl.sh /usr/bin/clashforgectl
chmod +x /usr/bin/clashforgectl
clashforgectl status
```

## Manual Package Install

IPK:

```sh
opkg install --nodeps --force-downgrade /tmp/clashforge_<version>_<arch>.ipk
```

APK for OpenWrt 25.12+:

```sh
apk add --allow-untrusted /tmp/clashforge-<version>_<arch>.apk
```

Manual install is more error-prone because you must choose the correct architecture.

## Developer Deploy

Use `deploy` only when validating local source changes:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy
```

It builds the UI, compiles Go, creates a local IPK, uploads it and installs it. Normal users should use `upgrade`.
