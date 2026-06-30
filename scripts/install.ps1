# ClashForge Quick Install
# Usage:
#   irm dl.wei1xuan.com/install.ps1 | iex
#
# With subscription URL (fully automated):
#   $env:CF_SUB='https://your-sub-url'; irm dl.wei1xuan.com/install.ps1 | iex
#
# This script:
#   1. Downloads clashforgectl.ps1 to $HOME\.clashforge\
#   2. Prompts for subscription URL if not set via $env:CF_SUB
#   3. Launches: clashforgectl.ps1 hyperv [-SubscriptionURL ...]
#
# Compatible with PowerShell 5.1 and 7+.

$ErrorActionPreference = 'Stop'

$cdnBase    = 'https://dl.wei1xuan.com'
$installDir = Join-Path $env:USERPROFILE '.clashforge'
$scriptPath = Join-Path $installDir 'clashforgectl.ps1'

Write-Host ''
Write-Host '  ClashForge Setup' -ForegroundColor Cyan
Write-Host '  ----------------' -ForegroundColor DarkGray
Write-Host ''

# ── 1. Create install dir ─────────────────────────────────────────────────────
if (-not (Test-Path $installDir)) {
    New-Item -ItemType Directory -Path $installDir -Force | Out-Null
}

# ── 2. Download clashforgectl.ps1 ─────────────────────────────────────────────
Write-Host '  Downloading clashforgectl.ps1 from CDN...' -ForegroundColor DarkGray
try {
    Invoke-WebRequest -Uri "$cdnBase/clashforgectl.ps1" -OutFile $scriptPath -UseBasicParsing -ErrorAction Stop
    Write-Host "  [OK] Saved to $scriptPath" -ForegroundColor Green
} catch {
    Write-Host "  [XX] Download failed: $_" -ForegroundColor Red
    Write-Host ''
    Write-Host '  Check your internet connection and try again.' -ForegroundColor Yellow
    Write-Host "  Manual download: $cdnBase/clashforgectl.ps1" -ForegroundColor White
    Write-Host ''
    exit 1
}

# ── 3. Subscription URL ───────────────────────────────────────────────────────
$subUrl = $env:CF_SUB
if (-not $subUrl) {
    Write-Host ''
    Write-Host '  Subscription URL (optional — press Enter to skip):' -ForegroundColor Yellow
    Write-Host '  Example: https://your-provider.com/clash.yaml?token=xxx' -ForegroundColor DarkGray
    $subUrl = (Read-Host '  URL').Trim()
}

# ── 4. Launch hyperv setup ────────────────────────────────────────────────────
Write-Host ''
Write-Host '  Launching Hyper-V setup...' -ForegroundColor Cyan
Write-Host '  (A UAC prompt will appear to request Administrator privileges)' -ForegroundColor DarkGray
Write-Host ''

if ($subUrl) {
    & $scriptPath hyperv -SubscriptionURL $subUrl
} else {
    & $scriptPath hyperv
}
