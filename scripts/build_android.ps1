<#
.SYNOPSIS
  Build a ClashForge Android APK with an embedded free-node subscription URL.

.DESCRIPTION
  Reads FREE_NODE_URL and FREE_NODE_AES_KEY from .env.dev.mobile at the repo root
  (gitignored). CLI parameters override the env file if supplied.

  Encrypts the subscription URL with AES-256-CBC at build time and bakes it into
  the APK via --dart-define. The app decrypts it at runtime and auto-imports the
  subscription on first launch (when no subscriptions are saved).

.PARAMETER SubUrl
  Subscription URL to embed. Overrides FREE_NODE_URL from .env.dev.mobile.

.PARAMETER AesKey
  AES-256 encryption key (max 32 chars). Overrides FREE_NODE_AES_KEY.

.PARAMETER Install
  Install the freshly-built APK on the connected ADB device.

.PARAMETER Debug
  Build debug variant (default: release).

.PARAMETER OutputDir
  Copy the APK to this folder after building (optional).

.EXAMPLE
  # Typical usage — just fill in .env.dev.mobile once, then:
  .\scripts\build_android.ps1 -Install

.EXAMPLE
  # One-off override without touching the env file:
  .\scripts\build_android.ps1 -SubUrl "https://example.com/sub" -AesKey "key" -Install
#>
param(
    [string]$SubUrl    = '',
    [string]$AesKey    = '',
    [switch]$Install,
    [switch]$Debug,
    [string]$OutputDir = ''
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$RepoRoot   = Join-Path $PSScriptRoot '..'
$ProjectDir = Join-Path $RepoRoot 'mobile'
$EnvFile    = Join-Path $RepoRoot '.env.dev.mobile'

# ── helpers ──────────────────────────────────────────────────────────────────

function Write-Step([string]$msg) { Write-Host "`n[build] $msg" -ForegroundColor Cyan }
function Write-Ok([string]$msg)   { Write-Host "[build] OK  $msg" -ForegroundColor Green }
function Write-Err([string]$msg)  { Write-Host "[build] ERR $msg" -ForegroundColor Red }

# ── load .env.dev.mobile ─────────────────────────────────────────────────────

function Read-EnvFile([string]$Path) {
    $vars = @{}
    if (-not (Test-Path $Path)) { return $vars }
    foreach ($line in Get-Content $Path) {
        $line = $line.Trim()
        if ($line -eq '' -or $line.StartsWith('#')) { continue }
        $idx = $line.IndexOf('=')
        if ($idx -lt 1) { continue }
        $key = $line.Substring(0, $idx).Trim()
        $val = $line.Substring($idx + 1).Trim()
        $vars[$key] = $val
    }
    return $vars
}

Write-Step "Loading env from $EnvFile ..."
$env = Read-EnvFile $EnvFile

if ($SubUrl -eq '' -and $env['FREE_NODE_URL']) {
    $SubUrl = $env['FREE_NODE_URL']
    Write-Host "         FREE_NODE_URL   = (from .env.dev.mobile)" -ForegroundColor Gray
} elseif ($SubUrl -ne '') {
    Write-Host "         FREE_NODE_URL   = (from -SubUrl parameter)" -ForegroundColor Gray
}

if ($AesKey -eq '' -and $env['FREE_NODE_AES_KEY']) {
    $AesKey = $env['FREE_NODE_AES_KEY']
    Write-Host "         FREE_NODE_AES_KEY = (from .env.dev.mobile)" -ForegroundColor Gray
} elseif ($AesKey -ne '') {
    Write-Host "         FREE_NODE_AES_KEY = (from -AesKey parameter)" -ForegroundColor Gray
}

if ($SubUrl -eq '' -or $AesKey -eq '') {
    Write-Err "FREE_NODE_URL and FREE_NODE_AES_KEY are required."
    Write-Host @"

  Fill in ${EnvFile}:

    FREE_NODE_URL=https://example.com/sub?token=xxx
    FREE_NODE_AES_KEY=your32charkey

  Or pass them directly:
    .\scripts\build_android.ps1 -SubUrl "..." -AesKey "..." -Install
"@ -ForegroundColor Yellow
    exit 1
}

# ── AES-256-CBC encrypt ───────────────────────────────────────────────────────
# Returns: base64( IV(16 bytes) || PKCS7-padded-ciphertext )

function Invoke-AesEncrypt {
    param([string]$PlainText, [string]$Key)

    $aes = [System.Security.Cryptography.Aes]::Create()
    $aes.Mode    = [System.Security.Cryptography.CipherMode]::CBC
    $aes.Padding = [System.Security.Cryptography.PaddingMode]::PKCS7
    $aes.KeySize = 256

    $keyBytes = [System.Text.Encoding]::UTF8.GetBytes($Key)
    $key32 = [byte[]]::new(32)
    [Array]::Copy($keyBytes, $key32, [Math]::Min($keyBytes.Length, 32))
    $aes.Key = $key32
    $aes.GenerateIV()

    $enc      = $aes.CreateEncryptor()
    $urlBytes = [System.Text.Encoding]::UTF8.GetBytes($PlainText)
    $cipher   = $enc.TransformFinalBlock($urlBytes, 0, $urlBytes.Length)
    return [Convert]::ToBase64String($aes.IV + $cipher)
}

# ── main ─────────────────────────────────────────────────────────────────────

Write-Step 'Encrypting subscription URL...'
$cipher  = Invoke-AesEncrypt -PlainText $SubUrl -Key $AesKey
$preview = $cipher.Substring(0, [Math]::Min(24, $cipher.Length))
Write-Host "         cipher (preview): $preview..." -ForegroundColor Gray
Write-Host "         key length:       $($AesKey.Length) chars" -ForegroundColor Gray

$mode       = if ($Debug) { 'debug' } else { 'release' }
$apkRelPath = if ($Debug) {
    'build\app\outputs\flutter-apk\app-debug.apk'
} else {
    'build\app\outputs\flutter-apk\app-release.apk'
}
$apkPath = Join-Path $ProjectDir $apkRelPath

Write-Step "Building Flutter APK ($mode)..."

$flutter = 'flutter'
if (-not (Get-Command flutter -ErrorAction SilentlyContinue)) {
    $flutter = 'C:\Users\admin\flutter\bin\flutter.bat'
}

Push-Location $ProjectDir
try {
    & $flutter build apk "--$mode" `
        "--dart-define=FREE_NODE_CIPHER=$cipher" `
        "--dart-define=FREE_NODE_KEY=$AesKey"
    if ($LASTEXITCODE -ne 0) { throw "flutter build apk exited with $LASTEXITCODE" }
} finally {
    Pop-Location
}

Write-Ok "APK: $apkPath"
Write-Host "     size: $([Math]::Round((Get-Item $apkPath).Length / 1MB, 1)) MB" -ForegroundColor Gray

if ($OutputDir -ne '') {
    $null = New-Item -ItemType Directory -Force $OutputDir
    Copy-Item $apkPath $OutputDir -Force
    Write-Ok "Copied to: $(Join-Path $OutputDir (Split-Path $apkPath -Leaf))"
}

if ($Install) {
    $adb = if (Get-Command adb -ErrorAction SilentlyContinue) { 'adb' } `
           else { 'C:\Users\admin\AppData\Local\Android\Sdk\platform-tools\adb.exe' }

    Write-Step 'Detecting connected devices...'

    # Parse "adb devices" — skip header, keep lines whose status column is "device"
    $rawLines = & $adb devices 2>&1 | Select-Object -Skip 1
    $devices  = @($rawLines |
        Where-Object { $_ -match '^\S+\s+device$' } |
        ForEach-Object { ($_ -split '\s+')[0] })

    if ($devices.Count -eq 0) {
        Write-Err 'No devices connected. Plug in a device (USB debugging on) and retry.'
        exit 1
    }

    $target = ''

    if ($devices.Count -eq 1) {
        $target = $devices[0]
        # Fetch model name for a friendlier display
        $model = (& $adb -s $target shell getprop ro.product.model 2>$null).Trim()
        Write-Host "         Device: $target  $model" -ForegroundColor Gray
    } else {
        Write-Host ''
        Write-Host '  Multiple devices found:' -ForegroundColor Yellow
        for ($i = 0; $i -lt $devices.Count; $i++) {
            $model = (& $adb -s $devices[$i] shell getprop ro.product.model 2>$null).Trim()
            Write-Host "    [$($i + 1)]  $($devices[$i])  $model"
        }
        Write-Host ''
        $raw = Read-Host "  Select device [1-$($devices.Count)]"
        $idx = 0
        if (-not [int]::TryParse($raw.Trim(), [ref]$idx) -or $idx -lt 1 -or $idx -gt $devices.Count) {
            Write-Err "Invalid selection '$raw'."
            exit 1
        }
        $target = $devices[$idx - 1]
    }

    Write-Step "Installing to $target..."
    & $adb -s $target install -r $apkPath
    if ($LASTEXITCODE -ne 0) { throw "adb install failed on $target" }
    Write-Ok "Installed on $target."
}

Write-Host ''
Write-Host 'Done.' -ForegroundColor Green
