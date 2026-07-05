# ClashForge CLI Installer
#
# Install only:
#   irm dl.wei1xuan.com/install.ps1 | iex
#
# After install, use from any terminal:
#   clashforgectl help
#   clashforgectl hyperv -SubscriptionURL 'https://...'
#
# To also run Hyper-V setup immediately after install:
#   $env:CF_SUB='https://your-sub-url'; irm dl.wei1xuan.com/install.ps1 | iex
#
# Install (or upgrade to latest) and immediately run any clashforgectl
# command in one line — `iex` alone can't forward arguments, so wrap the
# downloaded text in a scriptblock and invoke it with `&` instead:
#   &([scriptblock]::Create((irm dl.wei1xuan.com/install.ps1))) -Router 192.168.20.1 upgrade
#
# To uninstall:
#   irm dl.wei1xuan.com/uninstall.ps1 | iex
#
# Compatible with PowerShell 5.1 and 7+.

param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$RestArgs
)

$ErrorActionPreference = 'Stop'

$cdnBase    = 'https://dl.wei1xuan.com'
$installDir = Join-Path $env:USERPROFILE '.clashforge'
$ps1Path    = Join-Path $installDir 'clashforgectl.ps1'
$cmdPath    = Join-Path $installDir 'clashforgectl.cmd'

Write-Host ''
Write-Host '  ClashForge CLI Installer' -ForegroundColor Cyan
Write-Host '  ------------------------' -ForegroundColor DarkGray
Write-Host ''

# ── 1. Create install directory ───────────────────────────────────────────────
if (-not (Test-Path $installDir)) {
    New-Item -ItemType Directory -Path $installDir -Force | Out-Null
}
Write-Host "  Install directory: $installDir" -ForegroundColor DarkGray

# ── 2. Download clashforgectl.ps1 ─────────────────────────────────────────────
Write-Host '  Downloading clashforgectl.ps1...' -ForegroundColor DarkGray
try {
    Invoke-WebRequest -Uri "$cdnBase/clashforgectl.ps1" -OutFile $ps1Path -UseBasicParsing -ErrorAction Stop
    Write-Host '  [OK] clashforgectl.ps1' -ForegroundColor Green
} catch {
    Write-Host "  [XX] Download failed: $_" -ForegroundColor Red
    exit 1
}

# Fetch the version marker so the printed banner reflects exactly what was
# just downloaded — the quickest way to tell "did I actually get the latest
# build" apart from "my local copy is stale", instead of comparing file dates.
$version = '(unknown)'
try {
    $version = (Invoke-WebRequest -Uri "$cdnBase/version.txt" -UseBasicParsing -ErrorAction Stop).Content.Trim()
} catch {
    # version.txt may not exist yet on older R2 mirrors — non-fatal.
}

# ── 3. Write .cmd shim ────────────────────────────────────────────────────────
# The shim lets `clashforgectl` be called from cmd.exe, PowerShell, and
# Windows Terminal without typing the full path or .ps1 extension.
#
# clashforgectl.ps1 requires pwsh (PS7+) — it uses `??` throughout, which
# Windows PowerShell 5.1 cannot even parse, so there is no working fallback
# to `powershell.exe`. Deliberately NOT using `where pwsh && (a) || (b)`
# here: if block (a) itself exits non-zero (e.g. a diagnostic subcommand
# reporting a problem it found), cmd's `||` re-evaluates against block (a)'s
# exit code and ALSO runs block (b) — silently double-invoking the script,
# with the second (5.1) run always failing on `??`. `if/else` on `where`'s
# own captured errorlevel avoids that.
@'
@echo off
where pwsh >nul 2>&1
if %ERRORLEVEL% EQU 0 (
    pwsh -NoLogo -NoProfile -File "%~dp0clashforgectl.ps1" %*
    exit /b %ERRORLEVEL%
)
echo.
echo   [clashforgectl] PowerShell 7 (pwsh) is required but not installed.
echo.
echo   Install it:
echo     winget install Microsoft.PowerShell
echo   Or download: https://github.com/PowerShell/PowerShell/releases/latest
echo.
exit /b 1
'@ | Set-Content -Path $cmdPath -Encoding ASCII
Write-Host '  [OK] clashforgectl.cmd' -ForegroundColor Green

# ── 4. Add install directory to user PATH ─────────────────────────────────────
$userPath = [Environment]::GetEnvironmentVariable('PATH', 'User')
if ($userPath -notlike "*$installDir*") {
    $newPath = "$installDir;$userPath"
    [Environment]::SetEnvironmentVariable('PATH', $newPath, 'User')
    # Also update the current session so the user can run it immediately
    $env:PATH = "$installDir;$env:PATH"
    Write-Host "  [OK] Added to PATH" -ForegroundColor Green
} else {
    Write-Host '  [OK] PATH already contains install directory' -ForegroundColor DarkGray
}

# ── 5. Summary ────────────────────────────────────────────────────────────────
Write-Host ''
Write-Host "  Installation complete — clashforgectl $version" -ForegroundColor Green
Write-Host ''
Write-Host '  Run from any terminal (new window required for PATH to take effect):' -ForegroundColor Yellow
Write-Host '    clashforgectl help' -ForegroundColor White
Write-Host '    clashforgectl hyperv -SubscriptionURL "https://..."' -ForegroundColor White
Write-Host ''
Write-Host '  In the current session, you can run it right now:' -ForegroundColor Yellow
Write-Host "    & `"$ps1Path`" help" -ForegroundColor White
Write-Host ''
Write-Host '  To uninstall:' -ForegroundColor Yellow
Write-Host '    irm https://dl.wei1xuan.com/uninstall.ps1 | iex' -ForegroundColor White
Write-Host ''

# ── 6. Optional: run Hyper-V setup immediately ────────────────────────────────
$subUrl = if ($env:CF_SUB) { $env:CF_SUB.Trim() } else { '' }
if ($subUrl) {
    Write-Host '  CF_SUB is set — launching Hyper-V setup now...' -ForegroundColor Cyan
    Write-Host '  (A UAC prompt will appear to request Administrator privileges)' -ForegroundColor DarkGray
    Write-Host ''
    & $ps1Path hyperv -SubscriptionURL $subUrl
}

# ── 7. Optional: forward any extra arguments straight to clashforgectl ────────
# Lets a single scriptblock-wrapped invocation both install (or upgrade to the
# latest ctl scripts) and run a command in one line, e.g.:
#   &([scriptblock]::Create((irm dl.wei1xuan.com/install.ps1))) -Router 192.168.20.1 upgrade
# Plain `irm ... | iex` never reaches here with arguments — iex can't forward
# them — so this only fires on the scriptblock-invocation form above.
if ($RestArgs.Count -gt 0) {
    Write-Host ''
    Write-Host "  Running: clashforgectl $($RestArgs -join ' ')" -ForegroundColor Cyan
    & $cmdPath @RestArgs
}
