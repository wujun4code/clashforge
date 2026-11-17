[CmdletBinding()]
param(
    [ValidateRange(1, 1440)]
    [int]$Minutes = 30,

    [ValidateSet('idle', 'auto', 'download', 'custom')]
    [string]$Scenario = 'idle',

    [string]$OutputDirectory = 'build/power-reports'
)

$ErrorActionPreference = 'Stop'
$packageName = 'com.clashforge.mobile.clashforge_mobile'

if (-not (Get-Command adb -ErrorAction SilentlyContinue)) {
    throw 'adb was not found on PATH.'
}

$device = (adb get-state 2>$null).Trim()
if ($device -ne 'device') {
    throw 'No ready Android device is connected.'
}

$timestamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$reportDirectory = Join-Path $OutputDirectory "$timestamp-$Scenario"
New-Item -ItemType Directory -Force -Path $reportDirectory | Out-Null

$metadata = @(
    "package=$packageName"
    "scenario=$Scenario"
    "minutes=$Minutes"
    "started_at=$((Get-Date).ToString('o'))"
    "device=$(adb shell getprop ro.product.model)"
    "android=$(adb shell getprop ro.build.version.release)"
) -join "`n"
$metadata | Set-Content -LiteralPath (Join-Path $reportDirectory 'metadata.txt')

adb shell dumpsys batterystats --reset | Out-Null
adb shell dumpsys meminfo $packageName |
    Set-Content -LiteralPath (Join-Path $reportDirectory 'meminfo-before.txt')
adb shell dumpsys cpuinfo |
    Select-String -Pattern 'clashforge|mihomo' |
    Set-Content -LiteralPath (Join-Path $reportDirectory 'cpuinfo-before.txt')

Write-Host "Recording '$Scenario' for $Minutes minute(s). Keep test conditions stable."
Start-Sleep -Seconds ($Minutes * 60)

adb shell dumpsys batterystats $packageName |
    Set-Content -LiteralPath (Join-Path $reportDirectory 'batterystats.txt')
adb shell dumpsys meminfo $packageName |
    Set-Content -LiteralPath (Join-Path $reportDirectory 'meminfo-after.txt')
adb shell dumpsys cpuinfo |
    Select-String -Pattern 'clashforge|mihomo' |
    Set-Content -LiteralPath (Join-Path $reportDirectory 'cpuinfo-after.txt')
adb shell ps -A |
    Select-String -Pattern 'clashforge|mihomo' |
    Set-Content -LiteralPath (Join-Path $reportDirectory 'processes.txt')

Add-Content -LiteralPath (Join-Path $reportDirectory 'metadata.txt') `
    -Value "`nfinished_at=$((Get-Date).ToString('o'))"

Write-Host "Power report written to $reportDirectory"
