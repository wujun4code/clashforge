# clashforgectl.ps1 — PS 5.1-compatible launcher
# Relaunches with pwsh (PowerShell 7) when running under Windows PowerShell 5.x.
# All implementation code lives in clashforgectl_impl.ps1 (requires PS 7+).

$impl = Join-Path $PSScriptRoot 'clashforgectl_impl.ps1'

if ($PSVersionTable.PSVersion.Major -ge 7) {
    & $impl @args
    exit $LASTEXITCODE
}

$pwshCmd = Get-Command pwsh -ErrorAction SilentlyContinue
if ($pwshCmd) {
    & $pwshCmd.Source -NoLogo -NoProfile -File $impl @args
    exit $LASTEXITCODE
}

Write-Host ''
Write-Host '  [clashforgectl] PowerShell 7 (pwsh) is required but not installed.' -ForegroundColor Red
Write-Host ''
Write-Host '  Install it:' -ForegroundColor Yellow
Write-Host '    winget install Microsoft.PowerShell' -ForegroundColor White
Write-Host '  Or download: https://github.com/PowerShell/PowerShell/releases/latest' -ForegroundColor White
Write-Host ''
exit 1
