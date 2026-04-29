# Operations

Use `scripts\clashforgectl.ps1` from Windows and `clashforgectl` inside router SSH.

## Command Reference

| Goal | Windows remote | Router-side |
| --- | --- | --- |
| Status | `.\scripts\clashforgectl.ps1 -Router 192.168.20.1 status` | `clashforgectl status` |
| Check | `.\scripts\clashforgectl.ps1 -Router 192.168.20.1 check` | `clashforgectl check` |
| Stop takeover | `.\scripts\clashforgectl.ps1 -Router 192.168.20.1 stop` | `clashforgectl stop` |
| Reset | `.\scripts\clashforgectl.ps1 -Router 192.168.20.1 reset` | `clashforgectl reset` |
| Reset and start | `.\scripts\clashforgectl.ps1 -Router 192.168.20.1 reset -Start` | `clashforgectl reset --start` |
| Upgrade | `.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade` | `clashforgectl upgrade` |
| Diagnostics | `.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact` | `clashforgectl diag --redact` |
| Uninstall | `.\scripts\clashforgectl.ps1 -Router 192.168.20.1 uninstall` | `clashforgectl uninstall` |

## Connection Parameters

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 -User root -Port 22 -Identity ~\.ssh\id_ed25519 status
```

## Diagnostics

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -Redact
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 diag -Fetch -RemoteOutput /tmp/cf-diag.txt -LocalPath .\cf-diag.txt -Redact
```

## Configuration Change Flow

1. Back up the current working configuration or subscription metadata.
2. Change YAML, subscriptions or overrides.
3. Start the core or reload configuration.
4. Run `status` and `check`.
5. Watch logs for one to three minutes.
6. Re-enable transparent proxying or DNS takeover only after validation.

## Logs

```sh
logread | grep -i clashforge
logread | grep -i mihomo
```

## Uninstall

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 uninstall
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 uninstall -KeepConfig
```
