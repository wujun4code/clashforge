# ClashForge Unified Control Script Plan

Date: 2026-04-27
Status: Planning
Scope: scripts for local OpenWrt control and remote router control from Windows/macOS/Linux

## Background

ClashForge currently has several operational scripts with overlapping responsibilities:

- `scripts/install.sh` installs or upgrades ClashForge on OpenWrt and already contains robust pre-upgrade cleanup logic.
- `scripts/push-install.ps1` copies a local IPK to a router and embeds a smaller copy of the cleanup/install logic.
- `scripts/uninstall.ps1` remotely removes ClashForge and restores dnsmasq, nftables, policy routes, processes, and data directories.
- `openwrt/files/usr/bin/clashforge-diag` collects detailed field diagnostics into `/tmp/cf-diag.txt`. This file will be removed and its logic fully merged into the unified control script.

These scripts solve real operational problems, but the control logic is duplicated. The next step is to introduce a unified control surface that can fully manage ClashForge in a consistent, auditable, and idempotent way.

## Goals

Build a unified ClashForge control script system that supports the following operations:

1. Upgrade ClashForge to latest or a specific version.
2. Reset the currently installed ClashForge version to first-install state.
3. Stop all ClashForge services and fully exit takeover mode.
4. Uninstall ClashForge completely.
5. Collect full on-device diagnostics for support and troubleshooting.

The same operational semantics must work in both environments:

1. Inside OpenWrt directly.
2. From an external machine, such as Windows/macOS/Linux, using `--router` to target a router over SSH.

## Non-Goals

This planning phase does not implement the control scripts yet.

This plan also does not change ClashForge runtime behavior, UI behavior, subscription logic, rule generation logic, or package build logic unless later implementation discovers a required integration point.

## Design Principles

### Single Source of Truth

The network restore and takeover-exit logic must live in one canonical shell implementation. Other wrappers should call or embed that canonical logic instead of maintaining separate copies.

### Safety First

The stop/restore path is the most important part of the design. It must avoid DNS blackout and must be safe to run repeatedly.

### Idempotency

Every operation should tolerate already-stopped services, missing nft tables, missing policy rules, absent config files, and already-removed packages.

### Predictable Ordering

The restore order must remain deliberate:

1. Restore dnsmasq configuration first.
2. Restart dnsmasq before tearing down forwarding tables.
3. Remove ClashForge nftables takeover tables.
4. Remove policy routing rules and route tables.
5. Stop services and kill remaining processes last.

This order reduces the risk of DNS interruption during takeover exit.

### Local and Remote Parity

A local OpenWrt command and a remote `--router` command should perform the same action with the same meaning.

## Proposed Files

### `scripts/clashforgectl.sh`

Primary OpenWrt-side control script.

Supported subcommands:

```sh
sh clashforgectl.sh status
sh clashforgectl.sh stop
sh clashforgectl.sh reset
sh clashforgectl.sh upgrade --version latest
sh clashforgectl.sh upgrade --version v0.1.0
sh clashforgectl.sh uninstall
sh clashforgectl.sh diag
```

Primary responsibilities:

- Parse subcommands and options.
- Provide canonical restore/stop logic.
- Download and install IPK files for upgrades.
- Reset runtime/config/data state.
- Uninstall the opkg package and remove all data.
- Provide `diag` subcommand with all diagnostic logic ported from the deleted `clashforge-diag` script.
- Print verification summaries.

### `scripts/clashforgectl.ps1`

Windows remote wrapper.

Example usage:

```powershell
.\scripts\clashforgectl.ps1 -Router 192.168.1.1 stop
.\scripts\clashforgectl.ps1 -Router 192.168.1.1 reset
.\scripts\clashforgectl.ps1 -Router 192.168.1.1 upgrade -Version latest
.\scripts\clashforgectl.ps1 -Router 192.168.1.1 uninstall
.\scripts\clashforgectl.ps1 -Router 192.168.1.1 diag -Fetch
```

Primary responsibilities:

- Accept router connection parameters.
- Use SSH to execute the OpenWrt-side control logic.
- Optionally upload the shell control script if it is not already present.
- Support remote diagnostic collection and optional report download.
- Surface clear local error messages.

### `scripts/clashforgectl`

Optional macOS/Linux remote wrapper.

Example usage:

```sh
./scripts/clashforgectl --router 192.168.1.1 stop
./scripts/clashforgectl --router 192.168.1.1 reset
./scripts/clashforgectl --router 192.168.1.1 upgrade --version latest
./scripts/clashforgectl --router 192.168.1.1 uninstall
./scripts/clashforgectl --router 192.168.1.1 diag --fetch
```

This can be added after the PowerShell wrapper if needed.

## Command Semantics

## `status`

Purpose: show the current ClashForge state without mutating the router.

Checks:

- Whether `clashforge` opkg package is installed.
- Whether `/etc/init.d/clashforge` exists.
- Whether `clashforge` or `mihomo-clashforge` processes are running.
- Whether nftables table `inet metaclash` exists.
- Whether nftables table `inet dnsmasq` exists.
- Whether policy routing fwmark rules exist.
- Whether dnsmasq appears to listen on port 53.
- Whether Web UI port 7777 appears to listen.

Expected behavior:

- Never exits non-zero simply because ClashForge is stopped.
- Exits non-zero only for script/runtime errors.

## `diag`

Purpose: collect a complete ClashForge field diagnostic report for support and troubleshooting.

This command directly incorporates all logic from the existing `openwrt/files/usr/bin/clashforge-diag` script. That file will be removed. There is no separate diagnostic script; `clashforgectl diag` is the single entry point.

Default output:

```text
/tmp/cf-diag.txt
```

Required options:

- `--output <path>` writes the diagnostic report to a custom path on the router.
- `--stdout` prints the report to stdout instead of only writing the file.
- `--redact` masks sensitive values where feasible before writing or printing the report.

Remote wrapper options:

- `-Fetch` or `--fetch` downloads the generated report to the local machine after remote collection.
- `-Output` or `--output` controls the remote output file path.
- `-LocalPath` or `--local-path` controls where the fetched report is saved locally.

Required diagnostic sections, ported directly from the deleted `clashforge-diag` script:

1. Collection timestamp and OpenWrt base information.
2. Process state for `clashforge`, `mihomo-clashforge`, `mihomo`, and `dnsmasq`.
3. Mihomo process metadata, including `/proc/<pid>/status`, cmdline, and starttime ticks when available.
4. TCP listen sockets from `/proc/net/tcp` and `/proc/net/tcp6`.
5. UDP bindings from `/proc/net/udp` and `/proc/net/udp6`.
6. `ss -tlnpu` or `netstat -tlnpu` snapshot.
7. ClashForge API probes against `127.0.0.1:7777`, including `/api/v1/status`, `/api/v1/config`, `/api/v1/setup/port-check`, `/api/v1/core/version`, and `/api/v1/config/mihomo`.
8. ClashForge config files such as `/etc/metaclash/config.toml` and `/etc/clashforge/config.toml`.
9. Generated Mihomo config files from `/etc/metaclash`, `/etc/clashforge`, `/tmp/metaclash`, and `/var/run/metaclash`.
10. DNS state, including `/etc/resolv.conf`, active dnsmasq configuration, dnsmasq.d contents, and `nslookup` probes.
11. Full nftables ruleset and `table inet metaclash`.
12. Policy routing state, including `ip rule list`, route table `100`, and default routing table snapshots.
13. Network interfaces from `ip addr show` and `ip link show`.
14. System resources from `free`, `df -h`, and `uptime`.
15. OpenWrt service registration state for ClashForge, MetaClash, and Mihomo init scripts.
16. Recent system logs from `logread`, `/var/log/messages`, or `dmesg` fallback.
17. ClashForge and MetaClash log files, including `/var/log/clashforge.log`, `/var/log/metaclash.log`, `/tmp/clashforge.log`, and `/tmp/metaclash.log`.
18. Mihomo binary discovery and version output.
19. ClashForge binary discovery and version output.
20. `/etc/metaclash` and `/etc/clashforge` directory structure.
21. Connectivity probes, including direct `api.ipify.org`, proxy through `127.0.0.1:7893`, and local TCP port checks for `7777`, `7874`, `7893`, and `53`.
22. OOM, crash, segfault, Mihomo, and ClashForge kernel log snippets.
23. ESTABLISHED TCP connection count as a rough Mihomo active connection indicator.

Behavior requirements:

- The diagnostic command must be non-destructive.
- Individual probe failures must not abort the whole report.
- Every command section should show the command being run and its output.
- Missing tools such as `curl`, `ss`, `netstat`, `logread`, or `nslookup` should be reported as unavailable instead of causing failure.
- The report should end with a clear path to the saved file.

Redaction requirements:

- `--redact` should mask likely subscription URLs, tokens, passwords, authorization headers, proxy credentials, and API secrets.
- Redaction should be best-effort and documented as not a cryptographic guarantee.
- Default behavior can initially preserve the current full-fidelity report for debugging, but remote wrappers should warn before fetching an unredacted report.

Remote flow:

1. Connect to the router over SSH.
2. Run the OpenWrt-side `diag` command.
3. If `--fetch` is set, copy the report back with `scp`.
4. Print both the remote report path and local report path.

## `stop`

Purpose: stop ClashForge and fully exit takeover mode while restoring router networking to the pre-ClashForge state.

Required actions:

1. Restore dnsmasq UCI state:
   - Delete `dhcp.@dnsmasq[0].port`.
   - Delete `dhcp.@dnsmasq[0].server`.
   - Delete `dhcp.@dnsmasq[0].noresolv`.
   - Commit `dhcp`.
2. Remove dnsmasq fallback config files:
   - `/etc/dnsmasq.d/clashforge.conf`.
   - `/var/etc/dnsmasq.d/clashforge.conf` if used.
3. Restart dnsmasq with `/etc/init.d/dnsmasq restart`.
4. Remove nftables takeover tables:
   - `table inet metaclash`.
   - `table inet dnsmasq` when it acts as the dnsmasq hijack table.
5. Remove policy routing:
   - IPv4 fwmark `0x1a3` table `100`.
   - IPv6 fwmark `0x1a3` table `100`.
   - IPv4 output tproxy fwmark `0x1a4` table `101`.
6. Stop init service:
   - `/etc/init.d/clashforge stop`.
7. Kill leftover processes:
   - `/usr/bin/clashforge`.
   - `/usr/bin/mihomo-clashforge`.
   - Use TERM first, then KILL if needed.
8. Print verification summary.

Important ordering note:

DNS restore should happen before nftables teardown and process killing. This follows the intent already documented in `scripts/install.sh`: dnsmasq should be back on port 53 before takeover redirect tables are removed.

## `reset`

Purpose: keep the installed ClashForge package version, but reset all ClashForge state to first-install state.

Required actions:

1. Run `stop` first.
2. Clear runtime and generated state:
   - `/var/run/metaclash`.
   - generated mihomo config.
   - cache database.
   - generated rule providers.
3. Clear user-facing configuration and data:
   - `/etc/metaclash`.
   - subscriptions.
   - overrides.
   - rule configuration.
   - cached subscription output.
4. Clear bundled or downloaded support data as needed:
   - `/usr/share/metaclash`.
5. Clear logs:
   - `/var/log/clashforge.log`.
6. Recreate required directories with expected ownership and permissions if the package expects them to exist.
7. Optionally start service when `--start` is provided.

Default behavior:

- Do not automatically restart ClashForge unless `--start` is explicitly passed.

## `upgrade`

Purpose: install latest or selected ClashForge IPK while preserving user config by default.

Required options:

- `--version <tag|latest>` defaults to `latest`.
- `--mirror <url>` forces a GitHub proxy.
- `--base-url <url>` downloads from a custom release base URL.
- `--purge` performs full cleanup before install.

Required actions:

1. Resolve version tag when `latest` is requested.
2. Detect OpenWrt IPK architecture.
3. Build IPK file name.
4. Download IPK.
5. If `--purge` is set:
   - Run purge/reset-level cleanup.
   - Remove existing package if installed.
6. Otherwise:
   - Run `stop` while preserving `/etc/metaclash`.
7. Install with:

```sh
opkg install --nodeps --force-downgrade <ipk>
```

8. Remove temporary IPK.
9. Print version and Web UI URL.

## `uninstall`

Purpose: completely remove ClashForge from the router.

Required actions:

1. Run `stop` first.
2. Remove package:

```sh
opkg remove clashforge
```

3. Delete all ClashForge data:
   - `/etc/metaclash` unless `--keep-config` is provided.
   - `/usr/share/metaclash`.
   - `/var/run/metaclash`.
   - `/var/log/clashforge.log`.
4. Disable service if init script still exists.
5. Print final verification summary:
   - no ClashForge processes.
   - no `inet metaclash` table.
   - no ClashForge fwmark policy routing rules.
   - dnsmasq listening on port 53 if detectable.

## CLI Option Plan

### Common OpenWrt-side options

```text
--yes             Skip confirmation prompts.
--verbose         Print more detail.
--dry-run         Print planned actions without mutating state where feasible.
--output <path>   Output path for commands that write reports, such as diag.
--stdout          Print report output to stdout when supported.
--redact          Mask sensitive values in diagnostic output where feasible.
--help            Show help.
```

### Remote wrapper options

```text
--router <host>   Router IP or hostname.
--user <name>     SSH user, default root.
--port <port>     SSH port, default 22.
--identity <path> SSH identity key path.
--fetch           Download generated diagnostic output after remote diag.
--local-path <p>  Local path for fetched diagnostic output.
```

PowerShell equivalents should use idiomatic parameter names while keeping command semantics recognizable:

```powershell
-Router <host>
-User root
-Port 22
-Identity <path>
-Fetch
-LocalPath <path>
-Yes
-Verbose
```

## Integration Strategy for Existing Scripts

## `scripts/install.sh`

Keep this script as the public one-shot installer.

Recommended evolution:

1. Continue supporting current install command forms.
2. Replace internal duplicated cleanup logic with calls to shared control functions when practical.
3. Preserve existing mirror and base-url behavior.

## `scripts/push-install.ps1`

Keep this as a compatibility helper for local IPK upload.

Recommended evolution:

1. Continue supporting `-Ipk` workflows.
2. Remove or reduce embedded shell cleanup duplication.
3. Call the same remote stop/restore logic before installing uploaded IPK.

## `scripts/uninstall.ps1`

Keep this as a compatibility helper.

Recommended evolution:

1. Keep current user-facing parameters.
2. Delegate remote logic to the unified control script.
3. Preserve `-KeepConfig` semantics.

## `openwrt/files/usr/bin/clashforge-diag`

This file will be removed. Its full diagnostic logic will be moved directly into `scripts/clashforgectl.sh` as the `diag` subcommand.

There will be no wrapper or compatibility shim. Users who previously ran `clashforge-diag` should use `clashforgectl diag` instead. Documentation and README references to `clashforge-diag` should be updated accordingly.

## Implementation Phases

## Phase 1: OpenWrt Control Script

Deliver `scripts/clashforgectl.sh` with:

- argument parser.
- `status`.
- `diag` incorporating the full logic from the deleted `clashforge-diag` script.
- canonical `stop`/restore logic.
- `reset`.
- `uninstall`.
- `upgrade` using the existing install logic as reference.

Validation:

- Run `stop` twice and confirm idempotency.
- Run `status` before and after stop.
- Run `diag` and confirm `/tmp/cf-diag.txt` is generated even when some probes fail.
- Confirm DNS recovers to port 53.
- Confirm nftables and policy rules are removed.

## Phase 2: Windows Remote Wrapper

Deliver `scripts/clashforgectl.ps1` with:

- `-Router`, `-User`, `-Port`, optional key path.
- subcommand forwarding.
- SSH execution.
- clear failure messages.

Validation:

- Remote `status`.
- Remote `diag` with optional report fetch.
- Remote `stop`.
- Remote `reset`.
- Remote `uninstall` dry run if supported.

## Phase 3: macOS/Linux Remote Wrapper

Deliver optional `scripts/clashforgectl` bash wrapper.

Validation:

- Same subcommands as PowerShell wrapper.
- Compatible with standard OpenSSH.

## Phase 4: Existing Script Convergence

Update existing scripts to reduce duplication:

- `scripts/install.sh`.
- `scripts/push-install.ps1`.
- `scripts/uninstall.ps1`.
- `openwrt/files/usr/bin/clashforge-diag` (to be deleted, its logic fully moved into `clashforgectl.sh`).

Validation:

- Existing documented install commands still work.
- Existing documented push-install command still works.
- Existing documented uninstall command still works.
- `clashforge-diag` is removed; `clashforgectl diag` produces the equivalent report.

## Phase 5: Documentation and Release

Update documentation:

- README operational section.
- OpenWrt install/upgrade instructions.
- Remote management instructions.
- Troubleshooting section for DNS, nftables, policy routing, opkg failures.

## Acceptance Criteria

The work is complete when:

1. A single OpenWrt-side script can perform `status`, `diag`, `stop`, `reset`, `upgrade`, and `uninstall`.
2. External control via router SSH supports the same actions.
3. `stop` restores dnsmasq, nftables, policy routing, and process state without DNS blackout in normal operation.
4. `reset` clears subscriptions, rules, overrides, generated config, caches, runtime data, and logs while keeping the installed package.
5. `uninstall` first runs stop/restore and then removes package plus all ClashForge data.
6. `diag` generates a comprehensive report containing all sections previously covered by the deleted `clashforge-diag` script and can optionally be fetched by remote wrappers.
7. Existing scripts either delegate to or remain behaviorally consistent with the unified control implementation.
8. Each destructive action is idempotent and can be safely retried.

## Risks and Mitigations

### DNS behavior differs across OpenWrt versions

Mitigation:

- Use UCI cleanup as the primary path.
- Remove known fallback config files.
- Restart dnsmasq.
- Treat missing keys/files as successful skips.

### nftables table ownership can be ambiguous

Mitigation:

- Only delete known ClashForge-owned table `inet metaclash`.
- Delete `inet dnsmasq` only with clear documentation that it is the dnsmasq HIJACK table involved in the takeover path.

### Remote wrapper quoting issues

Mitigation:

- Prefer uploading or streaming the canonical shell script instead of dynamically constructing large shell fragments.
- Keep remote command construction minimal.

### Reset semantics may need product decisions

Mitigation:

- Define reset as first-install state.
- Clear all subscriptions, rule providers, overrides, caches, generated configs, runtime state, and logs.
- Add `--keep-config` only to uninstall, not reset, unless a later requirement asks for partial reset.

### Diagnostic reports may contain sensitive data

Mitigation:

- Add `--redact` support for common sensitive values.
- Warn before fetching unredacted reports from a router.
- Document that redaction is best-effort and support should still handle diagnostic files as sensitive data.

## Open Questions

1. Should `reset` automatically start ClashForge after cleanup, or remain stopped by default?
2. Should remote wrappers upload `clashforgectl.sh` to `/tmp` for every run, or install it as `/usr/bin/clashforgectl` as part of the IPK?
3. Should `upgrade --purge` be equivalent to `uninstall` followed by install, or should it preserve package-managed directories?
4. Should `inet dnsmasq` deletion be conditional on detecting ClashForge takeover mode to avoid interfering with custom dnsmasq nft behavior?
5. Should `diag` redact by default, or should it preserve full-fidelity output by default and require explicit `--redact`?
6. Should remote `diag --fetch` automatically name local files with router host and timestamp?

## Recommended Next Step

Implement Phase 1 first: create `scripts/clashforgectl.sh`, move the known-good cleanup behavior from the current installer/uninstaller scripts into a canonical, idempotent command implementation, and port all logic from `clashforge-diag` directly into the `diag` subcommand. Delete `clashforge-diag` as part of this phase.
