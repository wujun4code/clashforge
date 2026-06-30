# clashforgectl.ps1 — PS 5.1-compatible launcher
# Relaunches with pwsh (PowerShell 7) when running under Windows PowerShell 5.x.
# All implementation code lives in clashforgectl_impl.ps1 (requires PS 7+).
#
# Single-file bootstrap: if clashforgectl_impl.ps1 is missing from the same
# directory, this launcher downloads it automatically from dl.wei1xuan.com.

$impl = Join-Path $PSScriptRoot 'clashforgectl_impl.ps1'

if (-not (Test-Path $impl)) {
    $implUrl = 'https://dl.wei1xuan.com/clashforgectl_impl.ps1'
    Write-Host ''
    Write-Host '[clashforgectl] clashforgectl_impl.ps1 not found — downloading from CDN...' -ForegroundColor Yellow
    Write-Host "  URL: $implUrl" -ForegroundColor DarkGray
    try {
        Invoke-WebRequest -Uri $implUrl -OutFile $impl -UseBasicParsing -ErrorAction Stop
        Write-Host '  [OK] Downloaded clashforgectl_impl.ps1' -ForegroundColor Green
    } catch {
        Write-Host ''
        Write-Host "  [XX] Failed to download clashforgectl_impl.ps1: $_" -ForegroundColor Red
        Write-Host ''
        Write-Host '  Download manually from:' -ForegroundColor Yellow
        Write-Host "    $implUrl" -ForegroundColor White
        Write-Host "  Save it next to clashforgectl.ps1 ($PSScriptRoot)" -ForegroundColor White
        Write-Host ''
        exit 1
    }
}

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
