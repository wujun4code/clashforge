# ClashForge CLI Uninstaller
#
# Removes what install.ps1 installed: the ~/.clashforge directory and its
# entry on your user PATH. Does not touch anything on a router — this only
# affects the clashforgectl command on this machine.
#
# Usage:
#   irm https://dl.wei1xuan.com/uninstall.ps1 | iex

$ErrorActionPreference = 'Stop'

$installDir = Join-Path $env:USERPROFILE '.clashforge'

Write-Host ''
Write-Host '  ClashForge CLI Uninstaller' -ForegroundColor Cyan
Write-Host '  --------------------------' -ForegroundColor DarkGray
Write-Host ''

# ── 1. Remove install directory ───────────────────────────────────────────────
if (Test-Path $installDir) {
    Remove-Item -Recurse -Force $installDir
    Write-Host "  [OK] Removed $installDir" -ForegroundColor Green
} else {
    Write-Host "  [OK] $installDir does not exist (already uninstalled)" -ForegroundColor DarkGray
}

# ── 2. Remove install directory from user PATH ────────────────────────────────
$userPath = [Environment]::GetEnvironmentVariable('PATH', 'User')
if ($userPath) {
    $entries = $userPath -split ';' | Where-Object { $_ -ne '' -and $_ -ne $installDir }
    $newUserPath = $entries -join ';'
    if ($newUserPath -ne $userPath) {
        [Environment]::SetEnvironmentVariable('PATH', $newUserPath, 'User')
        Write-Host '  [OK] Removed from user PATH' -ForegroundColor Green
    } else {
        Write-Host '  [OK] User PATH did not contain the install directory' -ForegroundColor DarkGray
    }
}
# Also clean the current session so a re-check right now reflects reality,
# even though this process's own PATH change can't affect already-open shells.
$env:PATH = ($env:PATH -split ';' | Where-Object { $_ -ne '' -and $_ -ne $installDir }) -join ';'

Write-Host ''
Write-Host '  Uninstall complete. Open a new terminal for the PATH change to take effect.' -ForegroundColor Green
Write-Host ''
