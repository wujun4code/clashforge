# Install & Deploy

ClashForge is designed for IPK-based deployment on OpenWrt. The development deploy script also builds and installs an IPK under the hood.

## Choose a Deployment Path

| Scenario | Recommended path | Notes |
| --- | --- | --- |
| Development validation | Windows `clashforgectl.ps1 deploy` | Fast local build, package and push loop |
| Production install | Release IPK | Install a stable package from GitHub Releases |
| Router-side maintenance | `clashforgectl` | Use after the package has been installed |
| Recovery | IPK rollback | Keep previous packages for quick rollback |

## Windows Remote Deploy

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 deploy
```

The deploy flow:

1. Bumps the patch version in the IPK control file.
2. Builds the React Web UI.
3. Cross-compiles the Linux amd64 Go binary.
4. Syncs OpenWrt helper files into the IPK tree.
5. Builds the IPK package.
6. Uploads the IPK and `clashforgectl.sh` to the router.
7. Installs with `upgrade --local-ipk` on the router.

::: warning Version bump
`deploy` modifies the version in `ipk/CONTROL/control`. Check your working tree before running it if you have unrelated changes.
:::

## Upgrade or Install from Releases

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Version v0.1.0-rc.1
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Mirror https://ghproxy.com
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -BaseUrl https://releases.example.com
.\scripts\clashforgectl.ps1 -Router 192.168.20.1 upgrade -Purge
```

## Router-side Maintenance

```sh
clashforgectl status
clashforgectl upgrade
clashforgectl upgrade --version v0.1.0-rc.1
clashforgectl upgrade --mirror https://ghproxy.com
clashforgectl upgrade --purge
```

## Service Entry Points

```sh
/etc/init.d/clashforge enable
/etc/init.d/clashforge start
/etc/init.d/clashforge status
/etc/init.d/clashforge restart
/etc/init.d/clashforge stop
```

## Web UI

```text
http://<router-ip>:7777
```
