# ClashForge CLI Installer
#
# Usage:
#   irm dl.wei1xuan.com/install.ps1 | iex
#
# After install, use from any terminal:
#   clashforgectl help
#   clashforgectl hyperv -SubscriptionURL 'https://...'
#
# To also run Hyper-V setup immediately after install:
#   $env:CF_SUB='https://your-sub-url'; irm dl.wei1xuan.com/install.ps1 | iex
#
# Compatible with PowerShell 5.1 and 7+.

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

# ── 3. Write .cmd shim ────────────────────────────────────────────────────────
# The shim lets `clashforgectl` be called from cmd.exe, PowerShell, and
# Windows Terminal without typing the full path or .ps1 extension.
@'
@echo off
where pwsh >nul 2>&1 && (
    pwsh -NoLogo -NoProfile -File "%~dp0clashforgectl.ps1" %*
) || (
    powershell -NoLogo -NoProfile -File "%~dp0clashforgectl.ps1" %*
)
exit /b %ERRORLEVEL%
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
Write-Host '  Installation complete.' -ForegroundColor Green
Write-Host ''
Write-Host '  Run from any terminal (new window required for PATH to take effect):' -ForegroundColor Yellow
Write-Host '    clashforgectl help' -ForegroundColor White
Write-Host '    clashforgectl hyperv -SubscriptionURL "https://..."' -ForegroundColor White
Write-Host ''
Write-Host '  In the current session, you can run it right now:' -ForegroundColor Yellow
Write-Host "    & `"$ps1Path`" help" -ForegroundColor White
Write-Host ''

# ── 6. Optional: run Hyper-V setup immediately ────────────────────────────────
$subUrl = if ($env:CF_SUB) { $env:CF_SUB.Trim() } else { '' }
if ($subUrl) {
    Write-Host '  CF_SUB is set — launching Hyper-V setup now...' -ForegroundColor Cyan
    Write-Host '  (A UAC prompt will appear to request Administrator privileges)' -ForegroundColor DarkGray
    Write-Host ''
    & $ps1Path hyperv -SubscriptionURL $subUrl
}
