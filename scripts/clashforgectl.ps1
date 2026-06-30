# clashforgectl.ps1 — ClashForge Remote Control Script (Windows)
#
# Uploads clashforgectl.sh to the target router and executes the requested
# subcommand over SSH, providing a unified control surface from Windows.
#
# Usage:
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 status
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 stop
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 reset
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 reset -Start
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 upgrade
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 upgrade -Version v0.1.0-beta.1
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 upgrade -Mirror https://ghproxy.com
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 upgrade -BaseUrl https://releases.example.com
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 upgrade -Purge
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 upgrade -RemoteDownload   # legacy: let router download
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 upgrade -LocalIpkFile C:\Downloads\clashforge.ipk
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 deploy
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 deploy -Skip ui
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 deploy -Skip go
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 deploy -Purge
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 check
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 compat
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 uninstall
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 uninstall -KeepConfig
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 uninstall -PurgeAll
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 diag
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 diag -Fetch
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 diag -Fetch -LocalPath .\cf-diag.txt -Redact
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 openclash
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 openclash -Kill
#   .\scripts\clashforgectl.ps1 -Router 192.168.1.1 flush-dns
#   .\scripts\clashforgectl.ps1 -Router 192.168.20.1 netdiag
#   .\scripts\clashforgectl.ps1 -Router 192.168.20.1 netdiag -Monitor
#   .\scripts\clashforgectl.ps1 -Router 192.168.20.1 netdiag -ApiPort 7777 -LogDir .\diag-logs
#
# Hyper-V one-click deploy (no -Router needed, requires Administrator):
#   .\scripts\clashforgectl.ps1 hyperv
#   .\scripts\clashforgectl.ps1 hyperv -SubscriptionURL 'https://your-sub-url/...'
#   .\scripts\clashforgectl.ps1 hyperv -VHDXPath C:\VMs\clashforge.vhdx -SubscriptionURL '...'
#   .\scripts\clashforgectl.ps1 hyperv -VHDXUrl 'https://dl.wei1xuan.com/clashforge-openwrt-hyperv.vhdx'
#   .\scripts\clashforgectl.ps1 hyperv -GithubMirror 'https://ghproxy.net/' -SubscriptionURL '...'
#
# Hyper-V lifecycle (requires Administrator):
#   .\scripts\clashforgectl.ps1 hyperv-stop                 # stop VM + disable ClashForge-LAN adapter (restores original network)
#   .\scripts\clashforgectl.ps1 hyperv-start                # start VM + enable adapter (metric 9000, won't preempt original NIC)
#   .\scripts\clashforgectl.ps1 hyperv-route                # show current routing priority + auto-toggle
#   .\scripts\clashforgectl.ps1 hyperv-route -HvMode clashforge   # set ClashForge-LAN as preferred gateway
#   .\scripts\clashforgectl.ps1 hyperv-route -HvMode direct       # set original NIC as preferred gateway
#
# Hyper-V cleanup (remove VM, switch, VHDX, proxy; requires Administrator):
#   .\scripts\clashforgectl.ps1 hyperv-remove
#   .\scripts\clashforgectl.ps1 hyperv-remove -Yes          # skip all confirmations
#
# Requirements:
#   - ssh and scp must be available for SSH-based actions (OpenSSH bundled with Windows 10/11)
#   - clashforgectl.sh must exist alongside this script (or in scripts/) for SSH-based actions
#   - Password-less SSH key auth is recommended
#   - The 'hyperv' action requires Administrator privileges and Windows Hyper-V

[CmdletBinding()]
param(
    # Subcommand — positional argument 0
    [Parameter(Mandatory, Position = 0)]
    [ValidateSet("status", "stop", "reset", "upgrade", "deploy", "check", "compat", "uninstall", "diag", "openclash", "flush-dns", "netdiag", "hyperv", "hyperv-remove", "hyperv-stop", "hyperv-start", "hyperv-route", "help")]
    [string]$Action,

    # Connection parameters (required for all SSH-based actions; not needed for 'netdiag' or 'hyperv')
    [string]$Router = '',                 # Router IP or hostname

    [string]$User     = "root",
    [int]$Port        = 22,
    [string]$Identity = "",               # Path to SSH private key (optional)

    # deploy options (local build + package + push)
    [string]$Skip     = "",              # Comma-separated steps to skip: "ui", "go"
    [string]$RepoRoot = "",              # Repo root path (default: auto-detect from script location)

    # upgrade options
    [string]$Version        = "latest",
    [string]$Mirror         = "",
    [string]$BaseUrl        = "",
    [switch]$Purge,
    [switch]$RemoteDownload,               # Have the router download the IPK itself (legacy fallback)
    [string]$LocalIpkFile = "",            # Pre-downloaded IPK path; skips arch detect, version resolve, and download

    # reset options
    [switch]$Start,

    # uninstall options
    [switch]$KeepConfig,
    [switch]$PurgeAll,                    # Delete EVERYTHING: config, keys, tokens, GeoData, binaries

    # diag options
    [switch]$Fetch,                       # Download report to local machine after collection
    [string]$RemoteOutput = "/tmp/cf-diag.txt",  # Remote report path
    [string]$LocalPath    = "",           # Local path for fetched report
    [switch]$Redact,                      # Best-effort masking of sensitive values

    # openclash options
    [switch]$Kill,                        # Stop OpenClash service and kill detected OpenClash processes

    # netdiag options (talks directly to the ClashForge HTTP API, no SSH needed)
    [int]$ApiPort   = 7777,               # ClashForge API port
    [string]$LogDir = ".\diag-logs",      # Local directory for diagnostic run logs
    [switch]$Monitor,                     # Continuous post-probe monitoring (Ctrl+C to stop)

    # common options
    [switch]$Yes,
    [switch]$DryRun,

    # hyperv options (hyperv / hyperv-stop / hyperv-start / hyperv-route / hyperv-remove; requires Administrator)
    [string]$VMName          = 'ClashForge-Router',
    [string]$VHDXDir         = (Join-Path $env:USERPROFILE 'VMs\ClashForge'),
    [string]$VHDXPath        = '',
    [string]$LanSwitchName   = 'ClashForge-LAN',
    [string]$LanIP           = '192.168.77.1',
    [int]   $MemoryMB        = 512,
    [int]   $CPUCount        = 1,
    [string]$GitHubRepo      = 'wujun4code/clashforge',
    [string]$VHDXUrl         = '',
    [string]$GithubMirror    = '',
    [string]$SubscriptionURL = '',
    [string]$ApiSecret       = '',

    # hyperv-route: routing mode — '' = auto-toggle, 'clashforge' = ClashForge takes priority, 'direct' = original NIC takes priority
    [ValidateSet('', 'clashforge', 'direct')]
    [string]$HvMode          = ''
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# ── helpers ───────────────────────────────────────────────────────────────────
function Log  { param($Msg) Write-Host "[clashforgectl] $Msg"       -ForegroundColor Cyan }
function Ok   { param($Msg) Write-Host "[clashforgectl] OK  $Msg"   -ForegroundColor Green }
function Warn { param($Msg) Write-Host "[clashforgectl] WARN $Msg"  -ForegroundColor Yellow }
function Die  { param($Msg) Write-Host "[clashforgectl] ERROR $Msg" -ForegroundColor Red; exit 1 }

# ══════════════════════════════════════════════════════════════════════════════
# NETDIAG — local network diagnostic suite (talks to ClashForge HTTP API directly,
# no SSH/SCP needed). Ported from scripts/diagnose-net.ps1.
# ══════════════════════════════════════════════════════════════════════════════
if ($Action -eq "netdiag") {
    # diagnose-net.ps1 relies on permissive null handling (bare $obj.prop on a
    # possibly-$null $obj) throughout; scope these off for this block only —
    # we exit before falling through to the SSH-based actions below.
    Set-StrictMode -Off
    $ErrorActionPreference = 'SilentlyContinue'
    $ProgressPreference    = 'SilentlyContinue'

    $stamp  = Get-Date -Format 'yyyyMMdd_HHmmss'
    $runDir = Join-Path $LogDir "run_${stamp}"
    $null   = New-Item -ItemType Directory -Force $runDir

    # ── 探测目标（与 internal/probetargets/targets.go 完全对应） ─────────────────

    $httpTargets = @(
        @{ name = '淘宝';      group = '国内'; url = 'https://www.taobao.com'    },
        @{ name = '网易云音乐'; group = '国内'; url = 'https://music.163.com'    },
        @{ name = 'GitHub';    group = '国外'; url = 'https://github.com'        },
        @{ name = 'Google';    group = '国外'; url = 'https://www.google.com'    },
        @{ name = 'OpenAI';    group = 'AI';   url = 'https://chat.openai.com'   },
        @{ name = 'Gemini';    group = 'AI';   url = 'https://gemini.google.com' }
    )

    $ipTargets = @(
        @{ name = '太平洋';  group = '国内'; url = 'https://whois.pconline.com.cn/ipJson.jsp?json=true'; gbk = $true  },
        @{ name = 'UpaiYun'; group = '国内'; url = 'https://pubstatic.b0.upaiyun.com/?_upnode';          gbk = $false },
        @{ name = 'IP.SB';   group = '国外'; url = 'https://api.ip.sb/geoip';                           gbk = $false },
        @{ name = 'IPInfo';  group = '国外'; url = 'https://ipinfo.io/json';                            gbk = $false }
    )

    $dnsTargets = @(
        @{ domain = 'google.com';    cat = '国外-GFW' },
        @{ domain = 'github.com';    cat = '国外-GFW' },
        @{ domain = 'openai.com';    cat = '国外-GFW' },
        @{ domain = 'taobao.com';    cat = '国内直连' },
        @{ domain = 'music.163.com'; cat = '国内直连' },
        @{ domain = 'baidu.com';     cat = '国内直连' }
    )

    # ── 基础工具 ─────────────────────────────────────────────────────────────────

    $script:CurrentLogFile = $null

    function Write-L {
        param([string]$msg, [string]$color = 'White')
        $ts   = Get-Date -Format 'HH:mm:ss.fff'
        $line = "[$ts] $msg"
        Write-Host $line -ForegroundColor $color
        if ($script:CurrentLogFile) { Add-Content -Path $script:CurrentLogFile -Value $line }
    }

    function Sep { Write-L ('─' * 72) -color DarkGray }
    function Hdr([string]$t) {
        $pad = '─' * [Math]::Max(0, 66 - $t.Length)
        Write-L "── $t $pad" -color DarkGray
    }

    function Sym([bool]$v)       { if ($v) { '✓' } else { '✗' } }
    function Color-Ok([bool]$v)  { if ($v) { 'Green' } else { 'Red' } }
    function Format-Ok([bool]$v) { if ($v) { 'OK  ' } else { 'FAIL' } }

    function Ts2Local([string]$iso) {
        try { ([datetime]::Parse($iso).ToLocalTime()).ToString('HH:mm:ss') } catch { $iso }
    }
    function Unix2Local([long]$ts) {
        try { ([System.DateTimeOffset]::FromUnixTimeSeconds($ts).LocalDateTime).ToString('HH:mm:ss') } catch { "$ts" }
    }

    # ── 表格渲染（支持中文双宽字符对齐） ─────────────────────────────────────────

    function Get-DisplayWidth([string]$s) {
        $w = 0
        foreach ($c in $s.ToCharArray()) {
            $cp = [int][char]$c
            if (($cp -ge 0x1100 -and $cp -le 0x115F) -or
                ($cp -ge 0x2E80 -and $cp -le 0x9FFF) -or
                ($cp -ge 0xAC00 -and $cp -le 0xD7A3) -or
                ($cp -ge 0xF900 -and $cp -le 0xFAFF) -or
                ($cp -ge 0xFF01 -and $cp -le 0xFF60)) {
                $w += 2
            } else { $w += 1 }
        }
        return $w
    }

    function Pad-ToWidth([string]$s, [int]$width) {
        $dw = Get-DisplayWidth $s
        return $s + (' ' * [Math]::Max(0, $width - $dw))
    }

    # Write-Table: 渲染带 Unicode 边框的对齐表格
    # $Headers   : string[]      列标题
    # $Rows      : object[][]    数据行
    # $RowColors : string[]      每行颜色（与 Rows 等长）
    # $Widths    : int[]         列宽最小值（0=自动）
    function Write-Table {
        param(
            [string[]]$Headers,
            [object[][]]$Rows,
            [string[]]$RowColors,
            [int[]]$Widths
        )

        if (-not $Rows -or $Rows.Count -eq 0) { return }
        $cols = $Headers.Count

        $w = @(0) * $cols
        for ($c = 0; $c -lt $cols; $c++) {
            $w[$c] = Get-DisplayWidth $Headers[$c]
            if ($Widths -and $c -lt $Widths.Count -and $Widths[$c] -gt $w[$c]) { $w[$c] = $Widths[$c] }
        }
        foreach ($row in $Rows) {
            for ($c = 0; $c -lt [Math]::Min($cols, $row.Count); $c++) {
                $dw = Get-DisplayWidth ([string]$row[$c])
                if ($dw -gt $w[$c]) { $w[$c] = $dw }
            }
        }

        function Make-Sep([string]$l, [string]$m, [string]$r) {
            $parts = @(); for ($c = 0; $c -lt $cols; $c++) { $parts += '─' * ($w[$c] + 2) }
            return $l + ($parts -join $m) + $r
        }

        $top = Make-Sep '┌' '┬' '┐'
        $mid = Make-Sep '├' '┼' '┤'
        $bot = Make-Sep '└' '┴' '┘'

        $hdrLine = '│'
        for ($c = 0; $c -lt $cols; $c++) { $hdrLine += ' ' + (Pad-ToWidth $Headers[$c] $w[$c]) + ' │' }

        function Out-Ln([string]$line, [string]$color = 'DarkGray') {
            Write-Host "    $line" -ForegroundColor $color
            if ($script:CurrentLogFile) { Add-Content $script:CurrentLogFile "    $line" }
        }

        Out-Ln $top; Out-Ln $hdrLine; Out-Ln $mid

        for ($i = 0; $i -lt $Rows.Count; $i++) {
            $row   = $Rows[$i]
            $color = if ($RowColors -and $i -lt $RowColors.Count -and $RowColors[$i]) { $RowColors[$i] } else { 'White' }
            $line  = '│'
            for ($c = 0; $c -lt $cols; $c++) {
                $val  = if ($c -lt $row.Count) { [string]$row[$c] } else { '' }
                $line += ' ' + (Pad-ToWidth $val $w[$c]) + ' │'
            }
            Out-Ln $line $color
        }
        Out-Ln $bot
    }

    # Write-GroupedTable: 按 group 字段分组后依次渲染表格（对应浏览器分组卡片）
    function Write-GroupedTable {
        param(
            [string[]]$Headers,
            [object[]]$Items,
            [string]$GroupOrder = '国内,国外,AI'
        )
        $groups = $GroupOrder -split ','
        $anyPrinted = $false
        foreach ($g in $groups) {
            $gi = @($Items | Where-Object { $_.group -eq $g })
            if ($gi.Count -eq 0) { continue }
            $anyPrinted = $true
            $label = "  [$g]"
            Write-Host $label -ForegroundColor DarkCyan
            if ($script:CurrentLogFile) { Add-Content $script:CurrentLogFile $label }
            Write-Table $Headers @($gi | ForEach-Object { ,[object[]]$_.row }) ($gi | ForEach-Object { $_.color })
        }
        # 未匹配任何分组的项目
        $ungrouped = @($Items | Where-Object { $groups -notcontains $_.group })
        if ($ungrouped.Count -gt 0) {
            Write-Table $Headers @($ungrouped | ForEach-Object { ,[object[]]$_.row }) ($ungrouped | ForEach-Object { $_.color })
        }
    }

    # ── 探测函数 ─────────────────────────────────────────────────────────────────

    function Ping-T([string]$target) {
        [bool](Test-Connection $target -Count 1 -TimeoutSeconds 2 -Quiet)
    }

    function Dns-Resolve {
        param([string]$name, [string]$server = '')
        try {
            $a = @{ Name = $name; Type = 'A'; ErrorAction = 'Stop' }
            if ($server) { $a.Server = $server }
            $r  = Resolve-DnsName @a
            $ip = ($r | Where-Object Type -eq 'A' | Select-Object -First 1).IPAddress
            return @{ ok = $true; ip = ($ip ?? '?'); fakeip = ($ip -match '^198\.1[89]\.')}
        } catch {
            return @{ ok = $false; ip = ''; fakeip = $false }
        }
    }

    function Http-Check {
        param([string]$url, [int]$timeout = 8)
        $sw = [System.Diagnostics.Stopwatch]::StartNew()
        try {
            $r = Invoke-WebRequest $url -TimeoutSec $timeout -UseBasicParsing -ErrorAction Stop
            $sw.Stop()
            return @{ ok = $true; status = [int]$r.StatusCode; ms = $sw.ElapsedMilliseconds }
        } catch [System.Net.WebException] {
            $sw.Stop()
            $sc = [int]($_.Exception.Response.StatusCode ?? 0)
            # 即便是 WebException，也可能根本没拿到响应（DNS/连接层失败），把底层异常也带出来。
            $inner = $_.Exception.InnerException
            $detail = "$($_.Exception.Status): $($_.Exception.Message)"
            if ($inner) { $detail += " <- $($inner.GetType().Name): $($inner.Message)" }
            return @{ ok = ($sc -gt 0 -and $sc -lt 500); status = $sc; ms = $sw.ElapsedMilliseconds; detail = $detail }
        } catch {
            $sw.Stop()
            # 捕获完整异常链（类型 + 消息），区分"DNS 解析失败/连接被拒/连接超时/TLS 握手失败"等
            # 完全不同的失败阶段——这是诊断"为什么连 conntrack 都没有记录这次尝试"的关键线索。
            $chain = @()
            $e = $_.Exception
            while ($e) { $chain += "$($e.GetType().Name): $($e.Message)"; $e = $e.InnerException }
            return @{ ok = $false; status = 0; ms = $sw.ElapsedMilliseconds; detail = ($chain -join ' <- ') }
        }
    }

    function Http-Json {
        param([string]$url, [int]$timeout = 8)
        try { return Invoke-RestMethod $url -TimeoutSec $timeout -ErrorAction Stop }
        catch { return $null }
    }

    function Api-Get([string]$path) {
        $r = Http-Json "http://${Router}:${ApiPort}${path}"
        if ($r -and $r.ok) { return $r.data }
        return $null
    }

    function Api-Post([string]$path, [object]$body = $null) {
        try {
            $params = @{ Uri = "http://${Router}:${ApiPort}${path}"; Method = 'POST'; TimeoutSec = 20; ErrorAction = 'Stop' }
            if ($body) { $params.Body = ($body | ConvertTo-Json -Depth 5); $params.ContentType = 'application/json' }
            $r = Invoke-RestMethod @params
            if ($r -and $r.ok) { return $r.data }
            return $r
        } catch { return $null }
    }

    function Api-Delete([string]$path) {
        try {
            $r = Invoke-RestMethod "http://${Router}:${ApiPort}${path}" -Method Delete -TimeoutSec 15 -ErrorAction Stop
            return $r
        } catch { return $null }
    }

    function Api-Put([string]$path, [object]$body) {
        try {
            $params = @{ Uri = "http://${Router}:${ApiPort}${path}"; Method = 'PUT'; TimeoutSec = 20; ErrorAction = 'Stop' }
            $params.Body = ($body | ConvertTo-Json -Depth 5); $params.ContentType = 'application/json'
            $r = Invoke-RestMethod @params
            return $r
        } catch { return $null }
    }

    function Ensure-TransparentTakeover {
        param([string]$reason = '')
        $cfg = Api-Get '/api/v1/config'
        $hc  = Api-Get '/api/v1/health/check'
        if (-not $cfg -or -not $hc) { return }

        $net = $cfg.network
        $tp  = $hc.transparent_proxy
        $nft = $hc.nft
        $shouldApply = $net -and $net.apply_on_start -and $net.mode -and $net.mode -ne 'none'
        $missing = $shouldApply -and ((-not $tp.active) -or (-not $nft.active))
        if (-not $missing) { return }

        $mode = [string]$net.mode
        $why = if ($reason) { " ($reason)" } else { '' }
        Write-L "  → 按配置补齐透明代理/防火墙接管${why}: mode=$mode" -color DarkGray
        $r = Api-Post '/api/v1/overview/takeover' @{
            module = 'transparent_proxy'
            mode = $mode
            stop_services = @()
        }
        if ($r) {
            Write-L "  ✓ 透明代理/防火墙已接管: $($r.message ?? 'ok')" -color Green
        } else {
            Write-L '  ⚠ 自动补齐透明代理/防火墙接管失败，请在「代理服务」页手动接管' -color Yellow
        }
    }

    # Fetch-IPProvider: 与路由器 fetchIPCheck/extractIPLocation 逻辑对应
    function Fetch-IPProvider {
        param([string]$url, [bool]$gbk = $false, [int]$timeout = 8)
        try {
            $wr = Invoke-WebRequest $url -TimeoutSec $timeout -UseBasicParsing -ErrorAction Stop
            if ($gbk) {
                $enc  = [System.Text.Encoding]::GetEncoding('GB18030')
                $body = $enc.GetString($wr.RawContentStream.ToArray())
            } else {
                $body = $wr.Content
            }
            $j = $body | ConvertFrom-Json -ErrorAction Stop

            # UpaiYun: { remote_addr, remote_addr_location: { country, province, city, isp } }
            if ($j.remote_addr) {
                $loc = $j.remote_addr_location
                $parts = @('country','province','city','isp') | ForEach-Object { $loc.$_ } | Where-Object { $_ }
                return @{ ok = $true; ip = $j.remote_addr; loc = ($parts -join ' · ') }
            }
            # pconline (太平洋): { ip, addr, pro, ... }
            if ($j.ip -and $j.addr) {
                return @{ ok = $true; ip = $j.ip; loc = $j.addr }
            }
            # IP.SB / IPInfo: { ip, country/org/city/... }
            if ($j.ip) {
                $parts = @($j.city, ($j.pro ?? $j.region), ($j.country_name ?? $j.country), ($j.isp ?? $j.org)) | Where-Object { $_ }
                return @{ ok = $true; ip = $j.ip; loc = ($parts -join ' · ') }
            }
            return @{ ok = $false; ip = ''; loc = '无法解析响应' }
        } catch {
            return @{ ok = $false; ip = ''; loc = "请求失败" }
        }
    }

    # ── 路由器配置快照 ────────────────────────────────────────────────────────────

    function Get-RouterConfigSnapshot {
        param([string]$saveDir = '')
        $snap = [ordered]@{ Config = $null; ActiveSource = $null; Subscriptions = @(); MihomoYaml = '' }

        $cfg = Api-Get '/api/v1/config'
        $snap.Config = $cfg
        if ($cfg -and $saveDir) { $cfg | ConvertTo-Json -Depth 10 | Set-Content (Join-Path $saveDir 'cf_config.json') -Encoding UTF8 }

        $as = Api-Get '/api/v1/config/active-source'
        $snap.ActiveSource = $as.active_source
        if ($as -and $saveDir) { $as | ConvertTo-Json -Depth 5 | Set-Content (Join-Path $saveDir 'active_source.json') -Encoding UTF8 }

        $subs = Api-Get '/api/v1/subscriptions'
        $snap.Subscriptions = $subs.subscriptions ?? @()
        if ($subs -and $saveDir) { $subs | ConvertTo-Json -Depth 10 | Set-Content (Join-Path $saveDir 'subscriptions.json') -Encoding UTF8 }

        $mc = Api-Get '/api/v1/config/mihomo'
        $snap.MihomoYaml = $mc.content ?? ''
        return $snap
    }

    function Show-RouterConfigSnapshot {
        param($snap, [string]$yamlFile = '', [string]$indent = '  ')

        $cfg = $snap.Config
        if ($cfg) {
            $mode     = [string]($cfg.network.mode ?? '?')
            $dnsMode  = [string]($cfg.dns.mode     ?? '?')
            $dnsStrat = [string]($cfg.dns.strategy ?? 'legacy')
            $tun      = $cfg.network.tun
            Write-L "${indent}网络模式 : $mode$(if($mode-eq'tun'){" stack=$($tun.stack) auto_route=$($tun.auto_route)"})" -color White
            Write-L "${indent}DNS 模式 : $dnsMode  策略=$dnsStrat  端口=$($cfg.ports.dns)" -color White
            Write-L "${indent}端  口   : http=$($cfg.ports.http)  socks=$($cfg.ports.socks)  mixed=$($cfg.ports.mixed)  tproxy=$($cfg.ports.tproxy)  mihomo_api=$($cfg.ports.mihomo_api)" -color DarkGray
            Write-L "${indent}auto_start_core=$($cfg.core.auto_start_core)" -color DarkGray
            if ($mode -eq 'tun') {
                $dh = ($tun.dns_hijack -join ',')
                Write-L "${indent}TUN      : stack=$($tun.stack)  dns_hijack=$dh  auto_route=$($tun.auto_route)  auto_detect_interface=$($tun.auto_detect_interface)" -color Cyan
            }
        } else {
            Write-L "${indent}(无法获取 ClashForge 配置)" -color Red
        }

        $as = $snap.ActiveSource
        if ($as) {
            $subInfo = ($as.type -eq 'subscription') ? "订阅: $($as.sub_name) (id=$($as.sub_id))" : "文件: $($as.filename)"
            Write-L "${indent}活跃来源 : $subInfo" -color White
        } else {
            Write-L "${indent}活跃来源 : (未配置)" -color Yellow
        }

        $subs = @($snap.Subscriptions)
        foreach ($s in $subs) {
            $flag = ($s.enabled ? '✓' : '○')
            $updated = try { [datetime]$s.last_updated | Get-Date -Format 'yyyy-MM-dd HH:mm' } catch { [string]($s.last_updated ?? '') }
            Write-L "${indent}订阅 $flag    : $($s.name) — $($s.node_count) 节点  更新=$updated" -color DarkGray
        }

        $yaml = $snap.MihomoYaml
        if ($yaml -and $yaml.Length -gt 10) {
            if ($yamlFile) { $yaml | Set-Content $yamlFile -Encoding UTF8 }
            $yl = $yaml -split "`n"
            $proxyCnt = ($yl | Where-Object { $_ -match '^\s*- name:' }).Count
            $ruleCnt  = ($yl | Where-Object { $_ -match '^\s*- [A-Z][A-Z]' }).Count
            $hasTun   = ($yaml -match '\btun:\b')
            $dnsSect  = ($yaml -match '\bdns:\b')
            Write-L "${indent}mihomo YAML: $($yl.Count) 行  代理节点≈$proxyCnt  规则≈$ruleCnt  tun段=$hasTun  dns段=$dnsSect" -color DarkGray
            if ($yamlFile) { Write-L "${indent}  → 完整 YAML 已保存: $yamlFile" -color DarkGray }

            # 打印关键顶层键
            $topKeys = @('mode','log-level','allow-lan','mixed-port','port','socks-port','tproxy-port','redir-port','external-controller','ipv6','unified-delay')
            $keyLines = @()
            foreach ($k in $topKeys) {
                $hit = $yl | Where-Object { $_ -match "^${k}:" } | Select-Object -First 1
                if ($hit) { $keyLines += $hit.Trim() }
            }
            if ($keyLines.Count -gt 0) {
                Write-L "${indent}  [顶层] $($keyLines -join '  ')" -color DarkCyan
            }

            # dns 段关键行
            $inDns = $false; $dnsLines = @()
            foreach ($line in $yl) {
                if ($line -match '^dns:') { $inDns = $true; continue }
                if ($inDns) {
                    if ($line -match '^[a-zA-Z]' -and $line -notmatch '^\s') { break }
                    if ($line -match 'mode:|enable:|listen:|nameserver:|fallback:|fake-ip-range:|enhanced-mode:') {
                        $dnsLines += $line.Trim()
                    }
                }
            }
            if ($dnsLines.Count -gt 0) {
                Write-L "${indent}  [dns]  $($dnsLines -join '  ')" -color DarkCyan
            }

            # tun 段
            $inTun = $false; $tunLines = @()
            foreach ($line in $yl) {
                if ($line -match '^tun:') { $inTun = $true; continue }
                if ($inTun) {
                    if ($line -match '^[a-zA-Z]' -and $line -notmatch '^\s') { break }
                    if ($line.Trim()) { $tunLines += $line.Trim() }
                }
            }
            if ($tunLines.Count -gt 0) {
                Write-L "${indent}  [tun]  $($tunLines -join '  ')" -color Cyan
            }
        } else {
            Write-L "${indent}mihomo YAML: (未生成或为空)" -color Yellow
        }
    }

    # 打印路由器当前运行状态（ClashForge 服务 + mihomo 进程）
    function Show-RouterStatus {
        param($snap)
        $hc = Api-Get '/api/v1/health/check'
        $cf = $hc.process.clashforge
        $mh = $hc.process.mihomo
        $dns = $hc.dns
        $tp  = $hc.transparent_proxy

        $cfOk = [bool]($cf.ok)
        $mhOk = [bool]($mh.ok)

        Write-L "  ClashForge : state=$($cf.state ?? '?')  ok=$cfOk  uptime=$($cf.uptime ?? '?')s" -color ($cfOk ? 'Green' : 'Yellow')
        Write-L "  mihomo     : state=$($mh.state ?? '?')  ok=$mhOk" -color ($mhOk ? 'Green' : 'Red')
        if ($dns)  { Write-L "  dns        : active=$($dns.active)  mode=$($dns.dnsmasq_mode)" -color ($dns.active ? 'DarkGreen' : 'DarkGray') }
        if ($tp)   { Write-L "  tproxy     : active=$($tp.active)  mode=$($tp.mode)" -color ($tp.active ? 'DarkGreen' : 'DarkGray') }

        if ($snap) {
            $cfg = $snap.Config
            $as  = $snap.ActiveSource
            $mode = [string]($cfg.network.mode ?? '?')
            $dnsMode = [string]($cfg.dns.mode ?? '?')
            $subLabel = if ($as -and $as.type -eq 'subscription') { $as.sub_name } elseif ($as -and $as.type -eq 'file') { $as.filename } else { '(未配置)' }
            Write-L "  配置       : 网络=$mode  DNS=$dnsMode  订阅=$subLabel" -color White
        }
        return $mhOk
    }

    # Get-TakeoverState: 抓取一份用于探测前后对比的"接管状态"快照
    # （mihomo 进程状态 + transparent_proxy/nft 的 active 标志），
    # 用于侦测探测过程中是否发生了意外的接管重触发/重启（典型表现：
    # 探测窗口内出现一次短暂的全网不可访问，但健康检查前后看起来都"正常"——
    # 这正是 Ensure-TransparentTakeover 在 tun 模式下被反复触发时的样子，因为旧版
    # 后端 tun 模式下 transparent_proxy.active/nft.active 永远报 false，导致
    # Ensure-TransparentTakeover 每次都误判"接管缺失"并重新 POST /overview/takeover）。
    function Get-TakeoverState {
        $hc = Api-Get '/api/v1/health/check'
        if (-not $hc) { return $null }
        return [pscustomobject]@{
            mihomoState = [string]($hc.process.mihomo.state ?? '?')
            mihomoOk    = [bool]($hc.process.mihomo.ok)
            tpActive    = [bool]($hc.transparent_proxy.active)
            nftActive   = [bool]($hc.nft.active)
            dnsActive   = [bool]($hc.dns.active)
        }
    }

    function Compare-TakeoverState {
        param($before, $after, [string]$label = '探测窗口')
        if (-not $before -or -not $after) { return }
        $changed = @()
        if ($before.mihomoState -ne $after.mihomoState) { $changed += "mihomo.state: $($before.mihomoState) → $($after.mihomoState)" }
        if ($before.tpActive    -ne $after.tpActive)    { $changed += "transparent_proxy.active: $($before.tpActive) → $($after.tpActive)" }
        if ($before.nftActive   -ne $after.nftActive)   { $changed += "nft.active: $($before.nftActive) → $($after.nftActive)" }
        if ($before.dnsActive   -ne $after.dnsActive)   { $changed += "dns.active: $($before.dnsActive) → $($after.dnsActive)" }

        if ($changed.Count -gt 0) {
            Write-L "  ⚠ ${label}内接管状态发生变化（可能是外部触发了重新接管/重启，正是导致测试期间断网的嫌疑点）：" -color Yellow
            foreach ($c in $changed) { Write-L "    - $c" -color Yellow }
        } else {
            Write-L "  ✓ ${label}内接管状态未发生变化" -color DarkGreen
        }
    }

    # ── Fake-IP 劫持检测 ─────────────────────────────────────────────────────────
    # 对应 Setup 向导第 2 步的 DNS 劫持探测（POST /setup/dns-probe）。
    # 检测上游 nameserver 是否对代理节点域名返回 198.18.x.x 虚拟地址——
    # 这正是本次排查的核心嫌疑：.10 路由器的 fake-ip TUN 拦截了 .20 发往
    # nameserver 的查询，让节点域名解析回虚假地址，导致 mihomo 连不上节点。
    function Invoke-DnsProbe {
        param([string[]]$Nameservers = @(), [string]$Indent = '  ')

        Write-L "${Indent}→ POST /api/v1/setup/dns-probe" -color DarkGray
        $pr = Api-Post '/api/v1/setup/dns-probe' @{ nameservers = $Nameservers }
        if (-not $pr) {
            Write-L "${Indent}⚠ DNS 劫持检测请求失败（端点可能不可用或路由器版本较旧）" -color Yellow
            return $null
        }

        $report   = $pr.report
        $nodeCount = [int]($pr.node_count       ?? 0)
        $nsCount   = [int]($pr.nameserver_count ?? 0)
        Write-L "${Indent}扫描范围: $nsCount 个 nameserver  ×  $nodeCount 个节点域名" -color DarkGray

        if ($report.all_clear) {
            Write-L "${Indent}✓ 未发现 fake-ip 劫持 — 所有 nameserver 均正常" -color Green
        } else {
            Write-L "${Indent}⚠ 检测到 fake-ip 劫持！上游 nameserver 对代理节点域名返回了 198.18.x.x 地址" -color Yellow
            Write-L "${Indent}  原因：上游路由器（如 .10）开启了 fake-ip 模式，DNS 查询被其 TUN 拦截" -color Yellow
            Write-L "${Indent}  后果：mihomo 拿到虚假 IP → 连不上节点 → 所有代理请求失败" -color Yellow

            $hijacked  = @($report.hijacked_nameservers ?? @())
            $working   = @($report.working_nameservers  ?? @())
            $fallbacks = @($report.suggested_fallbacks  ?? @())

            if ($hijacked.Count -gt 0) {
                Write-L "${Indent}  被劫持的 Nameserver:" -color Red
                foreach ($ns in $hijacked) { Write-L "${Indent}    ✗ $ns" -color Red }
            }
            if ($working.Count -gt 0) {
                Write-L "${Indent}  正常的 Nameserver:" -color Green
                foreach ($ns in $working) { Write-L "${Indent}    ✓ $ns" -color Green }
            }
            if ($fallbacks.Count -gt 0) {
                Write-L "${Indent}  建议切换到以下 DoH（绕开劫持，节点域名将走 DoH 解析）:" -color Cyan
                foreach ($fb in $fallbacks) { Write-L "${Indent}    → $fb" -color Cyan }
            }

            # 把探测到的被劫持/正常 nameserver 详情打到日志
            $results = @($report.results ?? @())
            if ($results.Count -gt 0) {
                $rRows = @(); $rColors = @()
                foreach ($res in $results) {
                    $ips = ($res.ips ?? @()) -join ', '
                    $status = if ($res.error) { "err: $($res.error)" } elseif ($res.hijacked) { "⚠ 劫持 → $ips" } else { "✓ 正常 → $ips" }
                    $rRows   += ,@($res.nameserver, $res.hostname, $status)
                    $rColors += if ($res.hijacked) { 'Yellow' } elseif ($res.error) { 'Red' } else { 'DarkGray' }
                }
                Write-Table @('Nameserver', '节点域名', '结果') $rRows $rColors
            }
        }

        $pr | ConvertTo-Json -Depth 5 | Set-Content (Join-Path $runDir 'dns_probe.json') -Encoding UTF8 -ErrorAction SilentlyContinue
        return $pr
    }

    # ── 自动启动 ClashForge mihomo 核心 ──────────────────────────────────────────

    function Start-ClashForge {
        param([int]$MaxWaitSec = 40, [string]$saveDir = '')

        $svcBefore = (Api-Get '/api/v1/service-log?lines=5000').lines ?? @()
        $svcLastTs = if ($svcBefore.Count -gt 0) { try { ($svcBefore[-1] | ConvertFrom-Json).time } catch { 0 } } else { 0 }

        Write-L "  → POST /api/v1/core/start  $(Get-Date -Format 'HH:mm:ss')" -color DarkGray
        $sr = Api-Post '/api/v1/core/start'
        if ($null -eq $sr) {
            $hc0 = Api-Get '/api/v1/health/check'
            if ($hc0.process.mihomo.ok) {
                Write-L '  mihomo 已在运行 (ALREADY_RUNNING)' -color Yellow
            } else {
                Write-L '  ✗ /core/start 返回 null，启动可能失败' -color Red
                return $false
            }
        } else {
            Write-L "  ✓ core/start 接受  pid=$($sr.pid ?? '?')" -color Green
            if ($sr.takeover_applied) {
                Write-L "  ✓ 自动接管: $($sr.takeover_applied -join ', ')" -color Green
            }
            if ($sr.takeover_warnings) {
                foreach ($w in @($sr.takeover_warnings)) {
                    Write-L "  ⚠ $w" -color Yellow
                }
            }
        }

        Write-L "  等待 mihomo 就绪 (最多 ${MaxWaitSec}s)..." -color DarkGray
        $deadline = (Get-Date).AddSeconds($MaxWaitSec)
        $ready    = $false
        while ((Get-Date) -lt $deadline) {
            Start-Sleep -Seconds 2
            $hc = Api-Get '/api/v1/health/check'
            $mh = $hc.process.mihomo
            if ($mh.ok) {
                Write-L "  ✓ mihomo 已就绪  state=$($mh.state)  $(Get-Date -Format 'HH:mm:ss')" -color Green
                $ready = $true; break
            }
            Write-L "  ... mihomo.state=$($mh.state ?? '?')  等待..." -color DarkGray
        }
        if (-not $ready) { Write-L "  ⚠ ${MaxWaitSec}s 超时，mihomo 未就绪" -color Yellow }

        if ($ready) {
            Ensure-TransparentTakeover 'core/start 后检查'
        }

        # 收集启动窗口内的路由器服务日志
        Start-Sleep -Milliseconds 600
        $svcAfter = (Api-Get '/api/v1/service-log?lines=5000').lines ?? @()
        $newLines  = @($svcAfter | Where-Object {
            try { ($_ | ConvertFrom-Json).time -gt $svcLastTs } catch { $false }
        })
        if ($saveDir) { $newLines | Set-Content (Join-Path $saveDir 'startup_service_log.json') -Encoding UTF8 }
        Write-L "  路由器启动日志: $($newLines.Count) 行$(if($saveDir){ "  → $(Join-Path $saveDir 'startup_service_log.json')"})" -color DarkGray
        foreach ($line in $newLines) {
            try {
                $obj = $line | ConvertFrom-Json
                $lv  = [string]($obj.level ?? ''); $ts = [long]($obj.time ?? 0)
                $msg = [string]($obj.message ?? $obj.msg ?? '')
                $extra = @()
                if ($obj.error)  { $extra += "err=$($obj.error)" }
                if ($obj.stage)  { $extra += "stage=$($obj.stage)" }
                if ($obj.phase)  { $extra += "phase=$($obj.phase)" }
                $det = if ($extra.Count) { "  {$($extra -join ', ')}" } else { '' }
                $lc  = switch ($lv) { 'error' { 'Red' } { $_ -in @('warn','warning') } { 'Yellow' } default { 'DarkCyan' } }
                Write-L "  [RTR $(Unix2Local $ts)] [$lv] $msg$det" -color $lc
            } catch { Write-L "  [RTR] $line" -color DarkGray }
        }
        return $ready
    }

    # ── core/apply — SSE 流式生成配置 + 启动 mihomo ────────────────────────────────
    function Invoke-CoreApply {
        param(
            [hashtable]$Payload,
            [int]$TimeoutSec = 120,
            [string]$saveDir = ''
        )

        $uri  = "http://${Router}:${ApiPort}/api/v1/core/apply"
        $body = $Payload | ConvertTo-Json -Depth 10 -Compress

        try {
            $req = [System.Net.HttpWebRequest]::Create($uri)
            $req.Method           = 'POST'
            $req.ContentType      = 'application/json'
            $req.Accept           = 'text/event-stream'
            $req.Timeout          = $TimeoutSec * 1000
            $req.ReadWriteTimeout = $TimeoutSec * 1000

            $bytes = [System.Text.Encoding]::UTF8.GetBytes($body)
            $req.ContentLength = $bytes.Length
            $ws = $req.GetRequestStream()
            $ws.Write($bytes, 0, $bytes.Length)
            $ws.Close()

            $resp     = $req.GetResponse()
            $reader   = [System.IO.StreamReader]::new($resp.GetResponseStream())
            $logLines = [System.Collections.Generic.List[string]]::new()
            $success  = $false

            while (-not $reader.EndOfStream) {
                $line = $reader.ReadLine()
                $logLines.Add($line)
                if (-not $line.StartsWith('data:')) { continue }
                $json = $line.Substring(5).TrimStart()
                try {
                    $ev = $json | ConvertFrom-Json
                    switch ([string]$ev.type) {
                        'step' {
                            $color = switch ([string]$ev.status) {
                                'ok'      { 'Green'    }
                                'error'   { 'Red'      }
                                'warn'    { 'Yellow'   }
                                'skip'    { 'DarkGray' }
                                'running' { 'Cyan'     }
                                default   { 'White'    }
                            }
                            $icon = switch ([string]$ev.status) {
                                'ok'    { [char]0x2713 }
                                'error' { [char]0x2717 }
                                'warn'  { [char]0x26A0 }
                                'skip'  { [char]0x2192 }
                                default { '...' }
                            }
                            Write-L "  [$($ev.step)] $icon $($ev.message)" -color $color
                            if ($ev.detail) { Write-L "      $($ev.detail)" -color DarkGray }
                        }
                        'info' {
                            Write-L "  [info] $($ev.message)" -color DarkGray
                        }
                        'done' {
                            $success = [bool]$ev.success
                            if (-not $success -and $ev.error) {
                                Write-L "  $([char]0x2717) 失败: $($ev.error)" -color Red
                            }
                        }
                    }
                } catch { }
            }
            $reader.Close()
            $resp.Close()

            if ($saveDir) { $logLines | Set-Content (Join-Path $saveDir 'apply_sse.log') -Encoding UTF8 }
            return $success

        } catch {
            Write-L "  $([char]0x2717) core/apply 请求失败: $_" -color Red
            return $false
        }
    }

    # ── 日志管理：路由器存储宝贵，本地日志超量需提醒清理 ─────────────────────────
    # 每次 Invoke-Probes 产出的 run_* 目录视为「一份」配对记录（本机日志 + 路由器日志）

    function Clear-RouterLogs {
        Write-L '  → DELETE /api/v1/service-log' -color DarkGray
        $d1 = Api-Delete '/api/v1/service-log'
        Write-L "    $($d1 -ne $null ? '✓ 已清理' : '⚠ 清理失败/无响应')" -color ($d1 -ne $null ? 'Green' : 'Yellow')

        Write-L '  → DELETE /api/v1/logs' -color DarkGray
        $d2 = Api-Delete '/api/v1/logs'
        Write-L "    $($d2 -ne $null ? '✓ 已清理' : '⚠ 清理失败/无响应')" -color ($d2 -ne $null ? 'Green' : 'Yellow')
    }

    # 启动诊断前扫描路由器现存日志，避免旧日志干扰本轮归因
    function Show-PreScanRouterLogs {
        Hdr '路由器现存日志扫描（诊断开始前）'
        $svc = Api-Get '/api/v1/service-log?lines=5000'
        $req = Api-Get '/api/v1/logs?limit=5000'
        $svcCount = [int]($svc.lines.Count ?? 0)
        $reqCount = [int]($req.logs.Count ?? 0)
        Write-L "  当前路由器服务日志: $svcCount 行" -color White
        Write-L "  当前路由器请求日志: $reqCount 条" -color White

        if ($svcCount -eq 0 -and $reqCount -eq 0) {
            Write-L '  ✓ 路由器当前无历史日志，无需清理' -color Green
            Sep
            return
        }

        Write-L '  ⚠ 路由器上存在历史日志，可能干扰本轮诊断的归因判断' -color Yellow
        Write-L '  [1] 清理现有日志后再开始诊断（推荐，获得干净基线）' -color White
        Write-L '  [2] 保留现有日志，直接开始诊断' -color White
        Write-L ''
        $choice = Read-Host '  请输入 1 或 2'
        if ($choice -eq '1') {
            Clear-RouterLogs
            Write-L '  ✓ 已清理，开始诊断...' -color Green
        } else {
            Write-L '  保留现有日志，继续...' -color DarkGray
        }
        Sep
    }

    function Show-LogManagement {
        param([string]$logDir, [string]$currentRunDir)

        Write-L ''
        Hdr '本地日志管理'

        $runDirs = @(Get-ChildItem -Path $logDir -Directory -Filter 'run_*' -ErrorAction SilentlyContinue | Sort-Object Name)
        if ($runDirs.Count -eq 0) { Write-L '  (无历史记录)' -color DarkGray; return }

        $entries = @()
        $totalBytes = [long]0
        foreach ($d in $runDirs) {
            $sz = (Get-ChildItem $d.FullName -Recurse -File -ErrorAction SilentlyContinue | Measure-Object -Property Length -Sum).Sum
            if (-not $sz) { $sz = 0 }
            $entries    += [pscustomobject]@{ Dir = $d; Bytes = [long]$sz }
            $totalBytes += $sz
        }

        $totalMB = [Math]::Round($totalBytes / 1MB, 2)
        Write-L "  共 $($entries.Count) 份测试记录（每份 = 本机日志 + 路由器日志配对）  总大小: ${totalMB} MB" -color Cyan

        $threshold = 5MB
        if ($totalBytes -le $threshold) {
            Write-L "  总大小未超过 5MB 阈值，无需清理" -color DarkGray
            return
        }

        Write-L "  ⚠ 总大小超过 5MB 阈值，按时间从早到晚列出：" -color Yellow
        $sorted = $entries | Sort-Object { $_.Dir.Name }
        foreach ($e in $sorted) {
            $mb  = [Math]::Round($e.Bytes / 1MB, 3)
            $cur = if ($e.Dir.FullName -eq $currentRunDir) { '  (本次)' } else { '' }
            Write-L "    $($e.Dir.Name)  ${mb} MB$cur" -color DarkGray
        }
        Write-L ''
        $delChoice = Read-Host '  是否删除最早的记录直至总量低于 5MB？（本次运行的记录不会被删除）[y/N]'
        if ($delChoice -eq 'y' -or $delChoice -eq 'Y') {
            $running = $totalBytes
            foreach ($e in $sorted) {
                if ($running -le $threshold) { break }
                if ($e.Dir.FullName -eq $currentRunDir) { continue }
                Write-L "    删除 $($e.Dir.Name) ..." -color DarkGray
                Remove-Item $e.Dir.FullName -Recurse -Force -ErrorAction SilentlyContinue
                $running -= $e.Bytes
            }
            Write-L "  ✓ 清理完成，剩余约 $([Math]::Round($running/1MB,2)) MB" -color Green
        } else {
            Write-L '  保留所有记录' -color DarkGray
        }
    }

    # ── 核心：单次探测套件 ────────────────────────────────────────────────────────

    function Invoke-Probes {
        $logFile = Join-Path $runDir 'windows.log'
        $script:CurrentLogFile = $logFile
        $startTime = Get-Date

        Write-L ''
        Write-L '╔══════════════════════════════════════════════════════════════════════╗' -color Cyan
        Write-L "║  ClashForge 网络诊断  $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')" -color Cyan
        Write-L "║  Router : ${Router}:${ApiPort}" -color Cyan
        Write-L "║  LogDir : $runDir" -color Cyan
        Write-L '╚══════════════════════════════════════════════════════════════════════╝' -color Cyan
        Write-L ''

        # 快照路由器日志基准（在探测前记录，之后做 diff）
        $svcBefore = (Api-Get '/api/v1/service-log?lines=5000').lines ?? @()
        $reqBefore = (Api-Get '/api/v1/logs?limit=2000').logs ?? @()
        $svcLastTs = if ($svcBefore.Count -gt 0) { try { ($svcBefore[-1] | ConvertFrom-Json).time } catch { 0 } } else { 0 }
        $reqLastTs = if ($reqBefore.Count -gt 0) { $reqBefore[-1].ts } else { 0 }
        Write-L "路由器日志基准: svc=$($svcBefore.Count)行 ts=$(Unix2Local $svcLastTs)  req=$($reqBefore.Count)条" -color DarkGray
        Write-L ''

        # 接管状态基线（用于侦测探测期间是否被外部触发了重新接管/重启——
        # 这正是过去导致探测窗口内"网络全都不可访问"的根因之一）
        $takeoverBefore = Get-TakeoverState
        if ($takeoverBefore) {
            Write-L "接管状态基线: mihomo=$($takeoverBefore.mihomoState)  tp.active=$($takeoverBefore.tpActive)  nft.active=$($takeoverBefore.nftActive)  dns.active=$($takeoverBefore.dnsActive)" -color DarkGray
            Write-L ''
        }

        # ── 配置快照 ─────────────────────────────────────────────────────────────
        Hdr '配置快照'
        $cfgSnap = Get-RouterConfigSnapshot -saveDir $runDir
        Show-RouterConfigSnapshot $cfgSnap (Join-Path $runDir 'mihomo_config.yaml')
        Write-L ''

        # ── 1. Ping ───────────────────────────────────────────────────────────────
        Hdr '1. Ping'
        $pingDefs = @(
            @{ key = 'router-20';       target = $Router          }
            @{ key = '8.8.8.8';         target = '8.8.8.8'        }
            @{ key = '114.114.114.114'; target = '114.114.114.114' }
        )
        $pRows = @(); $pColors = @()
        $pingRouterOk = $false
        foreach ($pd in $pingDefs) {
            $v = Ping-T $pd.target
            if ($pd.key -eq 'router-20') { $pingRouterOk = $v }
            $pRows   += ,@($pd.key, ($v ? '✓ OK  ' : '✗ FAIL'))
            $pColors += Color-Ok $v
        }
        Write-Table @('目标', '结果') $pRows $pColors
        Write-L ''

        # ── 2. DNS ────────────────────────────────────────────────────────────────
        Hdr '2. DNS 解析'
        Write-L '  正在解析...' -color DarkGray
        $dRows = @(); $dColors = @()
        foreach ($t in $dnsTargets) {
            $r  = Dns-Resolve $t.domain
            $fi = if ($r.fakeip) { ' [fake-ip ✓]' } elseif ($t.cat -match '国外' -and $r.ok -and -not $r.fakeip) { ' [直连]' } else { '' }
            $ipd = ($r.ok ? $r.ip : '—') + $fi
            $dRows   += ,@($t.domain, $t.cat, (Sym $r.ok), $ipd)
            $dColors += if ($r.ok) { 'White' } else { 'Red' }
        }
        foreach ($d in @('google.com', 'github.com')) {
            $r = Dns-Resolve $d '8.8.8.8'
            $dRows   += ,@("$d (8.8.8.8)", '直连外部', (Sym $r.ok), ($r.ip ?? '—'))
            $dColors += 'DarkGray'
        }
        Write-Table @('域名', '类别', '✓✗', '解析结果') $dRows $dColors
        Write-L ''

        # ── 2b. Fake-IP 劫持检测 ──────────────────────────────────────────────────
        Hdr '2b. Fake-IP 劫持检测'
        Write-L '  检测上游 nameserver 是否对代理节点域名返回 198.18.x.x — 正在扫描...' -color DarkGray
        $script:DnsProbeResult = Invoke-DnsProbe -Indent '  '
        Write-L ''

        # ── 3. 访问检查 ───────────────────────────────────────────────────────────
        Hdr '3. 访问检查'
        Write-L '  正在测试 (每项最多 8s)...' -color DarkGray

        $h204 = Http-Check 'http://connectivitycheck.gstatic.com/generate_204' 5
        $c204c = if ($h204.ok) { 'Green' } else { 'Red' }
        Write-L "  connectivitycheck/204  $(if ($h204.ok){'✓ 正常'}else{'✗ 失败'})  $($h204.ms)ms" -color $c204c
        if (-not $h204.ok -and $h204.detail) { Write-L "    → $($h204.detail)" -color Yellow }
        Write-L ''

        $httpItems = @()
        $httpFailDetails = @()
        foreach ($t in $httpTargets) {
            $r = Http-Check $t.url
            $statusLabel = if ($r.ok) { '✓ 正常' } else { "✗ 失败 $($r.status)" }
            if (-not $r.ok -and $r.detail) { $httpFailDetails += "  [$($t.name)] → $($r.detail)" }
            $httpItems += [pscustomobject]@{
                group = $t.group
                color = Color-Ok $r.ok
                row   = [object[]]@($t.name, $statusLabel, "$($r.ms) ms", $t.url)
            }
        }
        Write-GroupedTable @('目标', '状态', '延迟', 'URL') $httpItems
        foreach ($d in $httpFailDetails) { Write-L $d -color Yellow }
        Write-L ''

        # 访问检查刚发生的 TCP 连接尝试（如果被内核 conntrack 跟踪到，即使
        # mihomo 从未记录任何转发日志）此刻仍最新鲜——conntrack 条目通常在
        # SYN_SENT/UNREPLIED 状态下数十秒后就会过期，所以这里立即抓取一份
        # 快照，而不是等到本次诊断流程的最后（7. TUN 转发底层诊断）才抓取。
        $earlyConntrack = Api-Get '/api/v1/health/net-debug'
        if ($earlyConntrack -and $earlyConntrack.conntrack) {
            $earlyConntrack.conntrack | Set-Content (Join-Path $runDir 'conntrack_after_access_check.txt') -Encoding UTF8
            $ctLines = ($earlyConntrack.conntrack -split "`n") | Where-Object { $_ -match '192\.168\.20\.' }
            if ($ctLines) {
                Write-L "  [访问检查后立即抓取的 conntrack 快照] 发现 $($ctLines.Count) 条与 LAN 客户端相关的连接跟踪记录:" -color DarkGray
                $ctLines | Select-Object -First 20 | ForEach-Object { Write-L "    $_" -color White }
            } else {
                Write-L '  [访问检查后立即抓取的 conntrack 快照] ⚠ 未发现任何与 LAN 客户端 IP 相关的连接跟踪记录 — 内核甚至从未跟踪到这些 TCP 尝试，说明丢弃发生在 conntrack 之前（更早的内核路径），而不是 mihomo 内部' -color Yellow
            }
            Write-L ''
        }

        # ── 4. 出口 IP ────────────────────────────────────────────────────────────
        Hdr '4. 出口 IP'
        Write-L '  正在检测...' -color DarkGray

        $ipItems = @()
        foreach ($t in $ipTargets) {
            $r = Fetch-IPProvider $t.url $t.gbk 8
            $ipItems += [pscustomobject]@{
                group = $t.group
                color = if ($r.ok) { 'Green' } else { 'Red' }
                row   = [object[]]@($t.name, ($r.ok ? $r.ip : '—'), $r.loc)
            }
        }
        Write-GroupedTable @('来源', 'IP', '位置') $ipItems '国内,国外'
        Write-L ''

        # ── 5. 路由器健康检查 ─────────────────────────────────────────────────────
        if ($pingRouterOk) {
            Hdr '5. 路由器健康检查'
            $hc = Api-Get '/api/v1/health/check'
            if ($hc) {
                $ok = [bool]$hc.summary.healthy
                Write-L "  $(Sym $ok) healthy=$($hc.summary.healthy)  failures=$($hc.summary.failures)" -color (Color-Ok $ok)
                $cf = $hc.process.clashforge; $mh = $hc.process.mihomo
                if ($cf) { Write-L "    clashforge : state=$($cf.state)  uptime=$($cf.uptime)s"  -color ($cf.ok ? 'DarkGreen' : 'Red') }
                if ($mh) { Write-L "    mihomo     : state=$($mh.state)  ok=$($mh.ok)"           -color ($mh.ok ? 'DarkGreen' : 'Red') }
                if ($hc.dns) { Write-L "    dns        : active=$($hc.dns.active)  mode=$($hc.dns.dnsmasq_mode)  — $($hc.dns.message)" -color ($hc.dns.active ? 'DarkGreen' : 'Yellow') }
                if ($hc.transparent_proxy) { Write-L "    tproxy     : active=$($hc.transparent_proxy.active)  mode=$($hc.transparent_proxy.mode)" -color ($hc.transparent_proxy.active ? 'DarkGreen' : 'Yellow') }
                $hc.ports | Where-Object required | ForEach-Object {
                    Write-L "    port $($_.name.PadRight(12)) :$($_.port)  $(Sym $_.listening)  $($_.message)" -color (Color-Ok $_.listening)
                }
            } else { Write-L '  ✗ /health/check 请求失败' -color Red }
            Write-L ''

            # ── 6. 路由器侧探测 ───────────────────────────────────────────────────
            Hdr '6. 路由器侧探测 (/overview/probes)'
            Write-L '  (触发路由器完整探测，对应浏览器"路由器侧"面板)' -color DarkGray
            $op = Api-Get '/api/v1/overview/probes'
            if ($op) {
                Write-L "  路由器探测时间: $($op.checked_at)  本地: $(Ts2Local $op.checked_at)" -color DarkGray
                Write-L ''

                Write-L '  [路由器侧] 出口 IP:' -color DarkGray
                $rtrIpItems = @()
                foreach ($ic in $op.ip_checks) {
                    $prov = [string]($ic.provider ?? $ic.name ?? '?')
                    $rtrIpItems += [pscustomobject]@{
                        group = [string]($ic.group ?? '国内')
                        color = if ($ic.ok) { 'Green' } else { 'Red' }
                        row   = [object[]]@($prov, ($ic.ip ?? '—'), ($ic.location ?? ''))
                    }
                }
                Write-GroupedTable @('来源', 'IP', '位置') $rtrIpItems '国内,国外'
                Write-L ''

                Write-L '  [路由器侧] 访问检查:' -color DarkGray
                $rtrAccItems = @()
                foreach ($ac in $op.access_checks) {
                    if ($ac.ok) { $detail = "✓ 正常  HTTP $($ac.status_code)  $($ac.latency_ms)ms" }
                    else {
                        $s = if ($ac.stage) { " [$($ac.stage)]" } else { '' }
                        $detail = "✗ 失败$s  $($ac.error)"
                    }
                    $rtrAccItems += [pscustomobject]@{
                        group = [string]($ac.group ?? '国内')
                        color = if ($ac.ok) { 'Green' } else { 'Red' }
                        row   = [object[]]@($ac.name, $detail, $ac.url)
                    }
                }
                Write-GroupedTable @('目标', '状态', 'URL') $rtrAccItems
            } else { Write-L '  ✗ /overview/probes 失败（mihomo 可能未启动）' -color Yellow }
            Write-L ''
        } else {
            Write-L '  ⚠ 路由器 Ping 不通，跳过健康检查和路由器侧探测' -color Yellow
            Write-L ''
        }

        # ── 7. TUN 转发底层诊断 ───────────────────────────────────────────────────
        # 仅在 tun 模式下运行：mihomo 的 auto-route 只保证"路由器自身发出的连接"
        # 被正确接管（这也是 5/6 两步健康检查/路由器侧探测能通过的原因——它们都
        # 是路由器自身进程直连本地代理端口或被 DNS-hijack 在本地应答，从未真正
        # 走"转发"路径）。本节直接读取内核路由/防火墙状态，验证 LAN 客户端
        # （如 Windows）真正需要被转发的流量是否也进入了 TUN 设备——如果不是，
        # 这正好解释"路由器侧探测全部正常，但 Windows 完全无法上网"的矛盾现象。
        if ($pingRouterOk -and $hc -and $hc.transparent_proxy -and $hc.transparent_proxy.mode -eq 'tun') {
            Hdr '7. TUN 转发底层诊断 (ip route / ip rule / nft)'

            # ── DNS 模式 / 网络模式一致性检查 ──────────────────────────────────
            # TUN 模式下 mihomo 必须能凭目的 IP 本身判断"这是不是该由我接管的连接"。
            # fake-ip 下这是免费的（目的 IP 只可能来自 mihomo 自己的地址池）；
            # redir-host 下目的 IP 是真实公网地址，mihomo 需要反查 DNS 劫持缓存才能
            # 认领这个连接——这条反查路径对"转发给 LAN 客户端"的连接不可靠，实测
            # 表现为 TCP SYN 进入 TUN 后卡在 SYN_SENT/UNREPLIED，mihomo 从未处理。
            # 后端生成配置时已会自动改用 fake-ip（network 模式是基准），这里只是
            # 让用户也能在已保存的设置里看到并修正这个不一致，而不是被自动覆盖
            # 却毫无感知。
            $cfgNow = Api-Get '/api/v1/config'
            if ($cfgNow -and $cfgNow.dns -and $cfgNow.dns.mode -and $cfgNow.dns.mode -ne 'fake-ip') {
                Write-L "  ⚠ 检测到设置不一致：网络模式=tun，但 DNS 模式=$($cfgNow.dns.mode)" -color Yellow
                Write-L '     TUN 模式下 redir-host 无法可靠接管转发给 LAN 客户端的 TCP 连接（已通过 conntrack 实测验证）。' -color Yellow
                Write-L '     ClashForge 生成 mihomo 配置时已自动按 tun 模式改用 fake-ip，但已保存的设置仍是 redir-host。' -color Yellow
                Write-L ''
                Write-L '  [1] 现在切换已保存的 DNS 模式为 fake-ip 并重启内核（推荐）' -color White
                Write-L '  [2] 暂不修改，仅记录此次提醒' -color White
                Write-L ''
                $dnsModeChoice = Read-Host '  请输入 1 或 2'
                if ($dnsModeChoice -eq '1') {
                    $patchResult = Api-Put '/api/v1/config' @{ dns = @{ mode = 'fake-ip' } }
                    if ($patchResult -and $patchResult.updated) {
                        Write-L '  ✓ 已将 DNS 模式切换为 fake-ip，正在重启内核使其生效...' -color Green
                        $null = Api-Post '/api/v1/core/restart' '{}'
                        Start-Sleep -Seconds 2
                        Write-L '  ✓ 内核已重启' -color Green
                    } else {
                        Write-L '  ✗ 切换失败，请到「设置」页面手动修改 DNS 模式为 fake-ip' -color Red
                    }
                } else {
                    Write-L '  保留当前设置，继续诊断...' -color DarkGray
                }
                Write-L ''
            }

            $nd = Api-Get '/api/v1/health/net-debug'
            if ($nd) {
                $nd | ConvertTo-Json -Depth 5 | Set-Content (Join-Path $runDir 'net_debug.json') -Encoding UTF8
                Write-L "  ip_forward=$($nd.ip_forward)  rp_filter(all/default)=$($nd.rp_filter_all)/$($nd.rp_filter_default)" -color DarkGray
                Write-L ''

                # ── main 路由表（路由器自身出口路由，TUN 模式下 default 仍指向上游网关） ──
                Write-L '  [main 路由表]' -color DarkGray
                ($nd.routes_main -split "`n") | Where-Object { $_ } | ForEach-Object { Write-L "    $_" -color White }
                Write-L ''

                # ── ip rule 完整列表 —— 关键：9020/9021 是否由 core/apply 补全 ──────────
                Write-L '  [ip rule]' -color DarkGray
                ($nd.ip_rules -split "`n") | Where-Object { $_ } | ForEach-Object { Write-L "    $_" -color White }
                $hasRule9020 = [bool]($nd.ip_rules -match '9020:')
                $hasRule9021 = [bool]($nd.ip_rules -match '9021:')
                Write-L ''

                # ── table 2022（mihomo 的 LAN 转发路由表）—— default dev Meta 必须存在 ───
                Write-L '  [table 2022 路由]' -color DarkGray
                if ($nd.routes_table_2022) {
                    ($nd.routes_table_2022 -split "`n") | Where-Object { $_ } | ForEach-Object { Write-L "    $_" -color White }
                    $tbl2022HasMeta = [bool]($nd.routes_table_2022 -match '\bMeta\b')
                } else {
                    Write-L '    (空 — table 2022 尚无路由，mihomo auto-route 可能未启动)' -color Yellow
                    $tbl2022HasMeta = $false
                }
                Write-L ''

                # ── TUN 设备链路状态 ────────────────────────────────────────────────────
                Write-L '  [TUN 设备链路状态]' -color DarkGray
                ($nd.tun_links -split "`n") | Where-Object { $_ } | ForEach-Object { Write-L "    $_" -color White }
                $tunUp = [bool]($nd.tun_links -match '(?i)Meta[^\n]*UP|UP[^\n]*Meta')
                Write-L ''

                # ── nftables forward_lan —— oifname "Meta" accept 必须存在 ─────────────
                $fwdLanMetaAccept = $false
                if ($nd.nft_ruleset) {
                    $nd.nft_ruleset | Set-Content (Join-Path $runDir 'net_debug_nft.txt') -Encoding UTF8
                    Write-L "  完整 nft ruleset 已保存: $(Join-Path $runDir 'net_debug_nft.txt')" -color DarkGray
                    $fwdLanSection = ([regex]::Match($nd.nft_ruleset, '(?s)chain forward_lan\s*\{[^}]*\}')).Value
                    $fwdLanMetaAccept = [bool]($fwdLanSection -match 'oifname\s+"?Meta"?')
                    if ($fwdLanSection) {
                        Write-L '  [forward_lan 链]' -color DarkGray
                        ($fwdLanSection -split "`n") | Where-Object { $_ } | ForEach-Object { Write-L "    $_" -color DarkGray }
                        Write-L ''
                    }
                }

                # ── TUN 转发健康检查汇总 ─────────────────────────────────────────────────
                # mihomo 的 auto-route 只覆盖路由器自身的出口流量（9000–9010 规则）。
                # LAN 客户端的"转发"流量还需要两个额外补丁（由 core/apply 写入）：
                #   1. ip rule 9020/9021：把转发流量引入 table 2022（default dev Meta）
                #   2. fw4 forward_lan 放行 Meta：否则 nftables policy drop + tcp reject
                #      会在流量到达 Meta 前就把它 RST 掉（即"目标计算机积极拒绝"的根源）
                Write-L '  ── TUN 转发健康检查 ──────────────────────────────────────────────' -color DarkCyan
                $tunChecks = @(
                    [pscustomobject]@{ name = 'TUN 设备 Meta 已启动 (UP)';                 ok = $tunUp }
                    [pscustomobject]@{ name = 'ip rule 9020 — suppress main default route'; ok = $hasRule9020 }
                    [pscustomobject]@{ name = 'ip rule 9021 — lookup table 2022';           ok = $hasRule9021 }
                    [pscustomobject]@{ name = 'table 2022 包含 Meta 出口路由';               ok = $tbl2022HasMeta }
                    [pscustomobject]@{ name = 'fw4 forward_lan 放行 Meta (oifname accept)'; ok = $fwdLanMetaAccept }
                )
                $allTunOk = $true
                foreach ($c in $tunChecks) {
                    if (-not $c.ok) { $allTunOk = $false }
                    $icon  = if ($c.ok) { [char]0x2713 } else { [char]0x2717 }
                    $color = if ($c.ok) { 'Green' } else { 'Red' }
                    Write-L "  $icon $($c.name)" -color $color
                }
                Write-L ''
                if ($allTunOk) {
                    Write-L '  ✓ TUN 转发链路完整 — LAN 客户端流量应正常进入 mihomo' -color Green
                } else {
                    Write-L '  ✗ TUN 转发链路不完整，以上红色项缺失会导致 LAN 客户端无法上网:' -color Red
                    if (-not $hasRule9020 -or -not $hasRule9021) {
                        Write-L '    → ip rule 9020/9021 缺失：重新运行 core/apply（需部署新版 clashforge）' -color Yellow
                    }
                    if (-not $fwdLanMetaAccept) {
                        Write-L '    → fw4 forward_lan 未放行 Meta：重新运行 core/apply（需部署新版 clashforge）' -color Yellow
                    }
                    if (-not $tbl2022HasMeta) {
                        Write-L '    → table 2022 无路由：mihomo auto-route 应在启动后自动写入，若缺失请检查 mihomo 是否正常运行' -color Yellow
                    }
                }
                Write-L ''

                # ── loopback RST 检测 ─────────────────────────────────────────────────
                # "reject loopback connection to: 127.0.0.1:PORT" = TUN 捕获了到代理端口
                # 自身的请求并形成循环，是 TUN 模式下配置了 tproxy/redirect 端口时才出现的冲突。
                $svcLogRaw = (Api-Get '/api/v1/service-log?lines=200').lines ?? @()
                $loopbackRst = @($svcLogRaw | Where-Object { $_ -match 'reject.*loopback|loopback.*reject' })
                if ($loopbackRst.Count -gt 0) {
                    Write-L '  ⚠ 检测到 loopback 拒绝连接日志（TUN 循环捕获代理端口迹象）:' -color Yellow
                    $loopbackRst | Select-Object -First 5 | ForEach-Object { Write-L "    $_" -color DarkGray }
                    Write-L ''
                }

                if ($nd.errors) {
                    foreach ($e in $nd.errors) { Write-L "  ⚠ $e" -color Yellow }
                }
            } else {
                Write-L '  ✗ /health/net-debug 请求失败（路由器后端可能尚未升级到包含该诊断接口的版本）' -color Yellow
            }
            Write-L ''
        }

        $endTime = Get-Date
        Write-L "探测完成  $(Get-Date -Format 'HH:mm:ss')  耗时 $([int]($endTime - $startTime).TotalSeconds)s" -color Cyan
        Write-L "客户端日志: $logFile" -color DarkGray
        Sep

        # 收集路由器日志 diff
        Start-Sleep -Milliseconds 800
        $svcAfter = (Api-Get '/api/v1/service-log?lines=5000').lines ?? @()
        $reqAfter = (Api-Get '/api/v1/logs?limit=2000').logs ?? @()

        $svcNew = @($svcAfter | Where-Object {
            try { ($_ | ConvertFrom-Json).time -gt $svcLastTs } catch { $false }
        })
        $reqNew = @($reqAfter | Where-Object { $_.ts -gt $reqLastTs })

        $routerSvcFile = Join-Path $runDir 'router_svc.log'
        $routerReqFile = Join-Path $runDir 'router_req.log'
        $svcNew | Set-Content $routerSvcFile -Encoding UTF8
        $reqNew | ForEach-Object {
            "$($_.method.PadRight(7)) $($_.path.PadRight(40)) → $($_.status)  $($_.latency_ms)ms  ts=$(Unix2Local $_.ts)"
        } | Set-Content $routerReqFile -Encoding UTF8

        Write-L "路由器服务日志新增: $($svcNew.Count) 行  → $routerSvcFile" -color DarkGray
        Write-L "路由器请求日志新增: $($reqNew.Count) 条  → $routerReqFile" -color DarkGray

        # 接管状态收尾对比：如果探测期间被意外重新接管/重启，这里会标红提示
        $takeoverAfter = Get-TakeoverState
        if ($takeoverBefore -and $takeoverAfter) {
            Write-L "接管状态收尾: mihomo=$($takeoverAfter.mihomoState)  tp.active=$($takeoverAfter.tpActive)  nft.active=$($takeoverAfter.nftActive)  dns.active=$($takeoverAfter.dnsActive)" -color DarkGray
            Compare-TakeoverState $takeoverBefore $takeoverAfter '本次探测'
        }

        if ($svcNew.Count -gt 0) {
            Write-L ''
            Hdr "本次路由器服务日志（新增 $($svcNew.Count) 行）"
            foreach ($line in $svcNew) {
                try {
                    $obj   = $line | ConvertFrom-Json
                    $lv    = [string]($obj.level   ?? '')
                    $msg   = [string]($obj.message ?? $obj.msg ?? '')
                    $ts    = [long]  ($obj.time    ?? 0)
                    $extra = @()
                    if ($obj.ip)       { $extra += "ip=$($obj.ip)" }
                    if ($obj.location) { $extra += "loc=$($obj.location)" }
                    if ($obj.error)    { $extra += "err=$($obj.error)" }
                    if ($obj.stage)    { $extra += "stage=$($obj.stage)" }
                    $detail = if ($extra.Count) { "  {$($extra -join ', ')}" } else { '' }
                    $lc = switch ($lv) { 'error' { 'Red' } { $_ -in @('warn','warning') } { 'Yellow' } default { 'DarkCyan' } }
                    Write-L "  [RTR $(Unix2Local $ts)] [$lv] $msg$detail" -color $lc
                } catch { Write-L "  [RTR] $line" -color DarkGray }
            }
        }

        $script:CurrentLogFile = $null
        return $svcNew
    }

    # ══ netdiag 主流程 ════════════════════════════════════════════════════════

    Write-Host ''
    Write-Host '╔══════════════════════════════════════════════════════════════════════╗' -ForegroundColor Cyan
    Write-Host '║      ClashForge 网络诊断脚本                                        ║' -ForegroundColor Cyan
    Write-Host "║      Router : ${Router}:${ApiPort}" -ForegroundColor Cyan
    Write-Host "║      LogDir : $runDir" -ForegroundColor Cyan
    Write-Host '╚══════════════════════════════════════════════════════════════════════╝' -ForegroundColor Cyan
    Write-Host ''

    $script:CurrentLogFile = Join-Path $runDir 'windows.log'

    # ── 检查路由器连通性 ─────────────────────────────────────────────────────────
    Write-L "正在连接路由器 ${Router}:${ApiPort}..." -color DarkGray
    $routerPing = Ping-T $Router
    if (-not $routerPing) {
        Write-L "✗ 无法 Ping 通路由器 $Router，请检查网络连接后重试" -color Red
        exit 1
    }
    Write-L "✓ 路由器 $Router 可达" -color Green

    # ── 诊断开始前：扫描并提示清理路由器现存日志 ─────────────────────────────────
    Write-L ''
    Show-PreScanRouterLogs

    # ── 检查 mihomo 运行状态 ──────────────────────────────────────────────────────
    Write-L ''
Hdr '路由器当前状态'
$preSnap = Get-RouterConfigSnapshot
$mihomoBefore = Show-RouterStatus $preSnap
if ($mihomoBefore) {
    $takeoverPreEnsure = Get-TakeoverState
    Ensure-TransparentTakeover '当前核心已运行'
    $takeoverPostEnsure = Get-TakeoverState
    Compare-TakeoverState $takeoverPreEnsure $takeoverPostEnsure 'Ensure-TransparentTakeover 调用'
}
Sep

$scriptStartedMihomo = $false

    if (-not $mihomoBefore) {
        Write-L ''
        Write-L '  mihomo 当前未运行。请选择操作：' -color Yellow
        Write-L '  [1] 选择配置参数后启动（推荐：用于测试不同模式搭配）' -color White
        Write-L '  [2] 直接用上次配置启动，然后执行诊断' -color White
        Write-L '  [3] 不启动，直接执行诊断（部分路由器侧探测可能为空）' -color White
        Write-L ''
        $choice = Read-Host '  请输入 1、2 或 3'

        if ($choice -eq '1') {
            # ── 配置向导 ─────────────────────────────────────────────────────────
            Write-L ''
            Hdr '配置向导：选择启动参数'

            $curCfg = Api-Get '/api/v1/config'
            $curNet = $curCfg.network
            $curDns = $curCfg.dns

            # 网络模式（必选）
            Write-L ''
            Write-L '  ── 网络模式（必选）──────────────────────────────────────────────────' -color DarkCyan
            Write-L "     当前配置: $([string]($curNet.mode ?? '?'))" -color DarkGray
            Write-L '  [1] tun    — TUN 透明代理（mihomo 全接管，LAN 客户端最兼容）' -color White
            Write-L '  [2] tproxy — TProxy 模式（nftables/iptables TPROXY，需内核支持）' -color White
            Write-L '  [3] redir  — Redir 模式（iptables REDIRECT，仅 TCP）' -color White
            Write-L '  [4] none   — 不启用透明代理（仅手动指定代理）' -color White
            Write-L ''
            $netModeInput = Read-Host '  请输入 1-4（直接回车保持当前值）'
            $netMode = switch ($netModeInput) {
                '1' { 'tun' }
                '2' { 'tproxy' }
                '3' { 'redir' }
                '4' { 'none' }
                default { [string]($curNet.mode ?? 'tun') }
            }
            Write-L "  → 网络模式: $netMode" -color Green

            # DNS 模式（必选）
            Write-L ''
            Write-L '  ── DNS 模式（必选）──────────────────────────────────────────────────' -color DarkCyan
            Write-L "     当前配置: $([string]($curDns.mode ?? '?'))" -color DarkGray
            Write-L '  [1] fake-ip    — 虚假 IP（推荐；TUN 模式下必须使用此模式）' -color White
            Write-L '  [2] redir-host — 真实 IP（tproxy/redir 可用；TUN 下后端会自动切回 fake-ip）' -color White
            Write-L ''
            $dnsModeInput = Read-Host '  请输入 1 或 2（直接回车保持当前值）'
            $dnsMode = switch ($dnsModeInput) {
                '1' { 'fake-ip' }
                '2' { 'redir-host' }
                default { [string]($curDns.mode ?? 'fake-ip') }
            }
            Write-L "  → DNS 模式: $dnsMode" -color Green

            # DNS 分流策略
            Write-L ''
            Write-L '  ── DNS 分流策略 ──────────────────────────────────────────────────────' -color DarkCyan
            Write-L "     当前配置: $([string]($curDns.strategy ?? 'split'))" -color DarkGray
            Write-L '  [1] split   — 按域名分流：国内域名 → ISP DNS，国外域名 → DoH（推荐）' -color White
            Write-L '  [2] privacy — 全加密：国内 + 国外都走 DoH，ISP 完全看不到 DNS 查询' -color White
            Write-L '  [3] legacy  — 兼容模式：不生成 nameserver-policy，依赖 fallback-filter' -color White
            Write-L ''
            $dnsStrategyInput = Read-Host '  请输入 1-3（直接回车保持当前值）'
            $dnsStrategy = switch ($dnsStrategyInput) {
                '1' { 'split' }
                '2' { 'privacy' }
                '3' { 'legacy' }
                default { [string]($curDns.strategy ?? 'split') }
            }
            Write-L "  → DNS 分流策略: $dnsStrategy" -color Green

            # Fake-IP 劫持检测
            Write-L ''
            Write-L '  ── Fake-IP 劫持检测 ──────────────────────────────────────────────────' -color DarkCyan
            Write-L '  正在检测上游 nameserver 是否对代理节点域名返回 198.18.x.x (最多 20s)...' -color DarkGray
            $wizardProbe = Invoke-DnsProbe -Indent '  '
            if ($wizardProbe -and $wizardProbe.report -and -not $wizardProbe.report.all_clear) {
                if ($dnsMode -eq 'redir-host') {
                    Write-L ''
                    Write-L '  ⚠ 检测到劫持且 DNS 模式为 redir-host — 强烈建议改为 fake-ip：' -color Yellow
                    Write-L '     fake-ip 下 mihomo 完全接管 DNS，不依赖上游 nameserver，可绕过劫持' -color Yellow
                    $switchInput = Read-Host '  自动将 DNS 模式改为 fake-ip？[Y/n]'
                    if ($switchInput -ne 'n' -and $switchInput -ne 'N') {
                        $dnsMode = 'fake-ip'
                        Write-L '  → DNS 模式已改为: fake-ip' -color Green
                    }
                }
            }

            # 其他选项（直接回车 = 保持当前值）
            Write-L ''
            Write-L '  ── 其他选项（直接回车 = 保持当前值）───────────────────────────────' -color DarkCyan

            $curFwBackend = [string]($curNet.firewall_backend ?? 'auto')
            $fwInput = Read-Host "  防火墙后端 [auto/nftables/iptables/none] (当前=$curFwBackend)"
            $fwBackend = if ($fwInput -and $fwInput -in @('auto','nftables','iptables','none')) { $fwInput } else { $curFwBackend }

            $curWan = [string]($curNet.wan_interface ?? 'eth1')
            $wanInput = Read-Host "  WAN 接口 (当前=$curWan, 常见: eth1/pppoe-wan)"
            $wanIface = if ($wanInput) { $wanInput } else { $curWan }

            $curBypassLan = [string]($curNet.bypass_lan ?? 'true')
            $bypassLanInput = Read-Host "  绕过 LAN [true/false] (当前=$curBypassLan)"
            $bypassLan = if ($bypassLanInput -in @('true','false')) { [bool]($bypassLanInput -eq 'true') } else { [bool]($curNet.bypass_lan -ne $false) }

            $curBypassChina = [string]($curNet.bypass_china ?? 'true')
            $bypassChinaInput = Read-Host "  绕过中国 [true/false] (当前=$curBypassChina)"
            $bypassChina = if ($bypassChinaInput -in @('true','false')) { [bool]($bypassChinaInput -eq 'true') } else { [bool]($curNet.bypass_china -ne $false) }

            $curDnsmasqMode = [string]($curDns.dnsmasq_mode ?? 'upstream')
            $dnsmasqInput = Read-Host "  dnsmasq 共存模式 [upstream/replace/none] (当前=$curDnsmasqMode)"
            $dnsmasqMode = if ($dnsmasqInput -and $dnsmasqInput -in @('upstream','replace','none')) { $dnsmasqInput } else { $curDnsmasqMode }

            # 配置来源（从实时订阅列表选择，避免使用已删除订阅的缓存）
            Write-L ''
            Write-L '  ── 配置来源 ──────────────────────────────────────────────────────────' -color DarkCyan
            $liveSubs    = (Api-Get '/api/v1/subscriptions').subscriptions ?? @()
            $enabledSubs = @($liveSubs | Where-Object { $_.enabled })
            $sourcePayload = @{ type = 'current' }
            $sourceDesc    = '当前活跃来源（不变）'

            if ($enabledSubs.Count -gt 0) {
                Write-L '  [0] 保持当前活跃来源（不变）' -color White
                for ($i = 0; $i -lt $enabledSubs.Count; $i++) {
                    $s   = $enabledSubs[$i]
                    $tag = if ($s.has_cache) { '✓缓存' } else { '无缓存' }
                    $nc  = if ($s.node_count -gt 0) { "$($s.node_count)节点" } else { '?' }
                    Write-L "  [$($i+1)] $($s.name)  ($tag, $nc)" -color White
                }
                $srcInput = Read-Host '  选择订阅来源编号（直接回车 = 保持当前）'
                $srcIdx   = 0
                if ($srcInput -match '^\d+$') { $srcIdx = [int]$srcInput }
                if ($srcIdx -ge 1 -and $srcIdx -le $enabledSubs.Count) {
                    $sel           = $enabledSubs[$srcIdx - 1]
                    $sourcePayload = @{ type = 'sub_id'; sub_id = $sel.id; sub_name = $sel.name; sync = (-not $sel.has_cache) }
                    $sourceDesc    = "订阅: $($sel.name)"
                }
            } else {
                Write-L '  暂无已启用订阅，将使用当前活跃来源' -color DarkGray
            }
            Write-L "  → 来源: $sourceDesc" -color Green

            # 汇总确认
            Write-L ''
            Write-L '  ── 即将应用以下配置 ─────────────────────────────────────────────────' -color DarkCyan
            Write-L "     来源         : $sourceDesc" -color Cyan
            Write-L "     网络模式     : $netMode" -color Cyan
            Write-L "     DNS 模式     : $dnsMode" -color Cyan
            Write-L "     DNS 分流策略 : $dnsStrategy" -color Cyan
            Write-L "     防火墙后端   : $fwBackend" -color Cyan
            Write-L "     WAN 接口     : $wanIface" -color Cyan
            Write-L "     绕过 LAN     : $bypassLan" -color Cyan
            Write-L "     绕过中国     : $bypassChina" -color Cyan
            Write-L "     dnsmasq 模式 : $dnsmasqMode" -color Cyan
            Write-L ''
            $confirm = Read-Host '  确认应用并启动 mihomo？[Y/n]'

            if ($confirm -ne 'n' -and $confirm -ne 'N') {
                $applyPayload = @{
                    source  = $sourcePayload
                    network = @{
                        mode             = $netMode
                        firewall_backend = $fwBackend
                        wan_interface    = $wanIface
                        bypass_lan       = $bypassLan
                        bypass_china     = $bypassChina
                        apply_on_start   = $true
                    }
                    dns = @{
                        mode           = $dnsMode
                        strategy       = $dnsStrategy
                        dnsmasq_mode   = $dnsmasqMode
                        apply_on_start = $true
                    }
                }

                Write-L ''
                Hdr 'core/apply — 生成配置并启动 mihomo'
                $script:CurrentLogFile = Join-Path $runDir 'windows.log'
                $started = Invoke-CoreApply -Payload $applyPayload -TimeoutSec 120 -saveDir $runDir
                if ($started) {
                    $scriptStartedMihomo = $true
                    Write-L ''
                    Hdr '启动后配置快照'
                    $postSnap = Get-RouterConfigSnapshot -saveDir $runDir
                    Show-RouterConfigSnapshot $postSnap (Join-Path $runDir 'mihomo_config.yaml')
                    Write-L ''
                    Write-L '  等待 3s 让路由表稳定...' -color DarkGray
                    Start-Sleep -Seconds 3
                } else {
                    Write-L '  ⚠ mihomo 启动失败，继续执行诊断...' -color Yellow
                }
            } else {
                Write-L '  已取消，跳过启动...' -color DarkGray
            }
            Sep

        } elseif ($choice -eq '2') {
            Write-L ''
            Hdr '启动 mihomo'
            $script:CurrentLogFile = Join-Path $runDir 'windows.log'
            $started = Start-ClashForge -MaxWaitSec 40 -saveDir $runDir
            if ($started) {
                $scriptStartedMihomo = $true
                Write-L ''
                Hdr '启动后配置快照'
                $postSnap = Get-RouterConfigSnapshot -saveDir $runDir
                Show-RouterConfigSnapshot $postSnap (Join-Path $runDir 'mihomo_config.yaml')
                Write-L ''
                Write-L '  等待 3s 让路由表稳定...' -color DarkGray
                Start-Sleep -Seconds 3
            } else {
                Write-L '  ⚠ mihomo 启动失败，继续执行诊断...' -color Yellow
            }
            Sep

        } else {
            Write-L '  跳过启动，继续执行诊断...' -color DarkGray
            Sep
        }
    }

    # ── 执行诊断探测 ──────────────────────────────────────────────────────────────
    $svcNew = Invoke-Probes

    # ── 本轮配对日志汇总：路由器侧日志已拷贝到本地，与客户端日志配对成一组 ───────
    Write-L ''
    Hdr '本轮配对日志（已从路由器拷贝到本地）'
    $pairFiles = @(
        @{ name = 'windows.log';        desc = '客户端探测日志' }
        @{ name = 'router_svc.log';     desc = '路由器服务日志（本轮新增，已拷贝）' }
        @{ name = 'router_req.log';     desc = '路由器请求日志（本轮新增，已拷贝）' }
        @{ name = 'cf_config.json';     desc = 'ClashForge 配置快照' }
        @{ name = 'active_source.json'; desc = '活跃订阅来源' }
        @{ name = 'subscriptions.json'; desc = '订阅列表' }
        @{ name = 'mihomo_config.yaml'; desc = 'mihomo 运行配置 YAML' }
        @{ name = 'dns_probe.json';     desc = 'Fake-IP 劫持检测结果' }
    )
    foreach ($pf in $pairFiles) {
        $fp = Join-Path $runDir $pf.name
        if (Test-Path $fp) {
            $sz = [Math]::Round((Get-Item $fp).Length / 1KB, 1)
            Write-L "  ✓ $($pf.name.PadRight(20)) ${sz} KB   $($pf.desc)" -color Green
        } else {
            Write-L "  · $($pf.name.PadRight(20)) (未生成)   $($pf.desc)" -color DarkGray
        }
    }
    Write-L "  这一组日志均位于同一目录，可一并用于回溯分析：$runDir" -color Cyan
    Sep

    # ── 收尾：如脚本启动了 mihomo，询问是否还原 ──────────────────────────────────
    if ($scriptStartedMihomo) {
        Write-L ''
        Write-L '  诊断完成。脚本本次启动了 mihomo。' -color Yellow
        Write-L '  [1] 还原（停止 mihomo）' -color White
        Write-L '  [2] 保持运行' -color White
        Write-L ''
        $restoreChoice = Read-Host '  请输入 1 或 2'
        if ($restoreChoice -eq '1') {
            $script:CurrentLogFile = Join-Path $runDir 'windows.log'
            Write-L '  → POST /api/v1/core/stop' -color DarkGray
            $stopResult = Api-Post '/api/v1/core/stop'
            if ($stopResult -ne $null) {
                Write-L '  ✓ mihomo 已停止' -color Green
            } else {
                Write-L '  ⚠ /core/stop 无响应，请手动确认状态' -color Yellow
            }
            $script:CurrentLogFile = $null
        } else {
            Write-L '  mihomo 保持运行' -color DarkGray
        }
    }

    $script:CurrentLogFile = Join-Path $runDir 'windows.log'

    # ── 路由器侧日志清理：确认本地已留存副本后自动清理（路由器存储宝贵） ─────────
    Write-L ''
    Hdr '路由器日志清理'
    $svcCopied = Test-Path (Join-Path $runDir 'router_svc.log')
    $reqCopied = Test-Path (Join-Path $runDir 'router_req.log')
    if ($svcCopied -and $reqCopied) {
        Write-L '  ✓ 已确认本次路由器服务/请求日志副本已落盘本地（router_svc.log / router_req.log）' -color Green
        Write-L '  路由器存储空间宝贵，自动清理路由器侧日志缓冲...' -color DarkGray
        Clear-RouterLogs
    } else {
        Write-L '  ⚠ 未能确认本地已保存路由器日志副本，为防止数据丢失，跳过本次清理' -color Yellow
    }
    Sep

    # ── 本地日志管理：配对计数 + 超量提醒 ─────────────────────────────────────────
    Show-LogManagement -logDir $LogDir -currentRunDir $runDir

    Write-L ''
    Write-L "完整日志目录: $runDir" -color Cyan

    # ── 可选：持续监控模式 ────────────────────────────────────────────────────────
    if ($Monitor) {
        $monLog = Join-Path $runDir 'monitor.log'
        $script:CurrentLogFile = $monLog
        Write-L ''
        Write-L '▶ 进入持续监控模式（Ctrl+C 停止）…' -color Cyan
        Write-L ''

        $monStart   = Get-Date
        $wasLost    = $false; $lossStart = $null; $lossCount = 0
        $knownSvcTs = 0
        if ($svcNew -and $svcNew.Count -gt 0) {
            try { $knownSvcTs = ($svcNew[-1] | ConvertFrom-Json).time } catch {}
        }

        try {
            while ($true) {
                $elapsed  = [int]((Get-Date) - $monStart).TotalSeconds
                $pingRtr  = Ping-T $Router
                $pingWan  = Ping-T '8.8.8.8'
                $dnsLocal = Dns-Resolve 'google.com'
                $http204  = Http-Check 'http://connectivitycheck.gstatic.com/generate_204' 4
                $internet = $pingWan -and $http204.ok

                if (-not $internet -and -not $wasLost) {
                    $lossStart = Get-Date; $wasLost = $true; $lossCount++
                    Write-L "⚡ [#${lossCount}] 断网 +${elapsed}s" -color Red
                    if ($pingRtr -and -not $pingWan) { Write-L '   rtr=OK wan=FAIL → nftables SNAT/路由问题' -color Yellow }
                    if (-not $dnsLocal.ok)            { Write-L '   dns=FAIL → dnsmasq/mihomo DNS 链路断裂'   -color Yellow }
                    if ($dnsLocal.fakeip)             { Write-L "   dns=fake-ip($($dnsLocal.ip)) mihomo DNS 工作中" -color DarkCyan }
                } elseif ($internet -and $wasLost) {
                    $dur = [int]((Get-Date) - $lossStart).TotalSeconds; $wasLost = $false
                    Write-L "✅ [#${lossCount}] 恢复 断了 ${dur}s，总 +${elapsed}s" -color Green
                }

                $fi  = if ($dnsLocal.fakeip) { '(fakeip)' } else { '' }
                $col = if (-not $internet) { 'Red' } elseif (-not $dnsLocal.ok) { 'Yellow' } else { 'DarkGreen' }
                Write-L "+${elapsed}s  rtr=$(Format-Ok $pingRtr)  wan=$(Format-Ok $pingWan)  dns=$(Format-Ok $dnsLocal.ok)$fi  http=$(Format-Ok $http204.ok)" -color $col

                if ($pingRtr) {
                    $svc = (Api-Get '/api/v1/service-log?lines=500').lines
                    if ($svc) {
                        $svc | Where-Object {
                            try { ($_ | ConvertFrom-Json).time -gt $knownSvcTs } catch { $false }
                        } | ForEach-Object {
                            try {
                                $obj = $_ | ConvertFrom-Json
                                $lv  = [string]($obj.level ?? ''); $nts = [long]($obj.time ?? 0)
                                if ($lv -in @('warn','warning','error') -or $obj.phase) {
                                    $rc = switch ($lv) { 'error' { 'Red' } { $_ -in @('warn','warning') } { 'Yellow' } default { 'DarkCyan' } }
                                    Write-L "  [RTR $(Unix2Local $nts)] [$lv] $([string]($obj.message ?? $obj.msg ?? ''))" -color $rc
                                }
                                if ($nts -gt $knownSvcTs) { $knownSvcTs = $nts }
                            } catch {}
                        }
                    }
                }
                Start-Sleep -Seconds 2
            }
        } finally {
            Write-L "监控结束 断网 $lossCount 次  日志: $monLog" -color Cyan
            $script:CurrentLogFile = $null
        }
    }

    exit 0
}

# ══════════════════════════════════════════════════════════════════════════════
# HYPERV — create OpenWrt + ClashForge VM on Windows Hyper-V
# Requires Administrator privileges.
# ══════════════════════════════════════════════════════════════════════════════
if ($Action -eq "hyperv") {
    $isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    if (-not $isAdmin) {
        Die "The 'hyperv' action requires Administrator privileges.`nRight-click PowerShell -> 'Run as administrator', then retry."
    }

    function Write-Step([string]$msg) { Write-Host "`n==> $msg" -ForegroundColor Cyan }
    function Write-OK  ([string]$msg) { Write-Host "  [OK] $msg" -ForegroundColor Green }
    function Write-Warn([string]$msg) { Write-Host "  [!!] $msg" -ForegroundColor Yellow }
    function Write-Fail([string]$msg) { Write-Host "  [XX] $msg" -ForegroundColor Red }

    function Invoke-DownloadFile([string]$Url, [string]$Dest) {
        Write-Host "  Downloading: $Url"
        Write-Host "  Destination: $Dest"
        $sw = [System.Diagnostics.Stopwatch]::StartNew()
        Invoke-WebRequest -Uri $Url -OutFile $Dest -UseBasicParsing
        $sw.Stop()
        Write-OK "Downloaded in $($sw.Elapsed.TotalSeconds.ToString('F1'))s  ($('{0:N1}' -f ((Get-Item $Dest).Length / 1MB)) MB)"
    }

    function Invoke-CF {
        param([string]$Method, [string]$Path, [object]$Body = $null, [switch]$SSE)
        $uri     = "http://${LanIP}:7777${Path}"
        $headers = @{ 'Content-Type' = 'application/json' }
        if ($ApiSecret -ne '') { $headers['Authorization'] = "Bearer $ApiSecret" }
        if ($SSE) {
            $client  = [System.Net.Http.HttpClient]::new()
            $client.Timeout = [System.TimeSpan]::FromSeconds(120)
            foreach ($k in $headers.Keys) { $client.DefaultRequestHeaders.Add($k, $headers[$k]) }
            $json   = if ($Body) { [System.Net.Http.StringContent]::new(($Body | ConvertTo-Json -Depth 10 -Compress), [System.Text.Encoding]::UTF8, 'application/json') } else { $null }
            $req    = [System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::new($Method), $uri)
            if ($json) { $req.Content = $json }
            $resp   = $client.SendAsync($req, [System.Net.Http.HttpCompletionOption]::ResponseHeadersRead).Result
            $stream = $resp.Content.ReadAsStreamAsync().Result
            $reader = [System.IO.StreamReader]::new($stream)
            $result = $null
            while (-not $reader.EndOfStream) {
                $line = $reader.ReadLine()
                if ($line -match '^data:\s*(\{.+\})$') {
                    $evt = $Matches[1] | ConvertFrom-Json
                    if ($evt.type -eq 'step') {
                        $icon = if ($evt.status -eq 'ok') { '  [OK]' } elseif ($evt.status -eq 'error') { '  [XX]' } else { '  [ ]' }
                        Write-Host "$icon $($evt.step): $($evt.message)"
                    }
                    if ($evt.type -eq 'done') { $result = $evt; break }
                }
            }
            $client.Dispose()
            return $result
        } else {
            $bodyJson = if ($Body) { $Body | ConvertTo-Json -Depth 10 -Compress } else { $null }
            return Invoke-RestMethod -Uri $uri -Method $Method -Headers $headers `
                   -Body $bodyJson -UseBasicParsing -ErrorAction Stop
        }
    }

    # ── 0. Pre-flight checks ─────────────────────────────────────────────────────
    Write-Step "Pre-flight checks"

    $hvEnabled = $false
    try {
        $hvFeature = Get-WindowsOptionalFeature -Online -FeatureName 'Microsoft-Hyper-V-All' -ErrorAction Stop
        $hvEnabled = $hvFeature.State -eq 'Enabled'
    } catch {
        $vmms = Get-Service -Name vmms -ErrorAction SilentlyContinue
        $hvEnabled = ($vmms -and $vmms.Status -eq 'Running')
    }
    if (-not $hvEnabled) {
        Write-Fail "Hyper-V is not enabled on this machine."
        Write-Host @"

  To enable Hyper-V, run this in an elevated PowerShell and reboot:
    Enable-WindowsOptionalFeature -Online -FeatureName Microsoft-Hyper-V-All

  Or via Settings -> Apps -> Optional features -> More Windows features -> Hyper-V
"@
        exit 1
    }
    Write-OK "Hyper-V is enabled"

    if (Get-VM -Name $VMName -ErrorAction SilentlyContinue) {
        Write-Warn "VM '$VMName' already exists."
        $choice = Read-Host "  Delete and recreate? [y/N]"
        if ($choice -match '^[Yy]') {
            $existing = Get-VM -Name $VMName
            if ($existing.State -ne 'Off') { Stop-VM -Name $VMName -Force }
            Remove-VM -Name $VMName -Force
            Write-OK "Removed existing VM"
        } else {
            Write-Host "Aborted." -ForegroundColor Yellow
            exit 0
        }
    }

    # ── 1. Download / locate VHDX ────────────────────────────────────────────────
    if ($VHDXPath -ne '' -and (Test-Path $VHDXPath)) {
        Write-Step "Using provided VHDX: $VHDXPath"
        Write-OK "$('{0:N0}' -f ((Get-Item $VHDXPath).Length / 1MB)) MB"

    } elseif ($VHDXUrl -ne '') {
        Write-Step "Downloading VHDX from custom URL"
        New-Item -ItemType Directory -Path $VHDXDir -Force | Out-Null
        $vhdxName = [System.IO.Path]::GetFileName(([uri]$VHDXUrl).LocalPath)
        if (-not $vhdxName -or $vhdxName -notlike '*.vhdx') { $vhdxName = 'clashforge-hyperv.vhdx' }
        $VHDXPath = Join-Path $VHDXDir $vhdxName
        if (Test-Path $VHDXPath) {
            Write-Warn "File already exists at $VHDXPath, skipping download."
        } else {
            Invoke-DownloadFile -Url $VHDXUrl -Dest $VHDXPath
        }

    } else {
        Write-Step "Downloading latest ClashForge Hyper-V image from GitHub"
        $mirrorPrefix = if ($GithubMirror -ne '') { $GithubMirror.TrimEnd('/') + '/' } else { '' }
        if ($mirrorPrefix -ne '') { Write-Warn "GitHub download mirror: $mirrorPrefix" }

        $releaseApi = "https://api.github.com/repos/$GitHubRepo/releases/latest"
        Write-Host "  Querying $releaseApi ..."
        try {
            $release = Invoke-RestMethod -Uri $releaseApi -UseBasicParsing `
                -Headers @{ 'Accept' = 'application/vnd.github+json' }
        } catch {
            Write-Fail "Failed to query GitHub API: $_"
            Write-Host ""
            Write-Host "  If GitHub is unreachable, try one of these alternatives:" -ForegroundColor Yellow
            Write-Host "    1. Mirror  : .\clashforgectl.ps1 hyperv -GithubMirror 'https://ghproxy.net/'" -ForegroundColor Yellow
            Write-Host "    2. CDN URL : .\clashforgectl.ps1 hyperv -VHDXUrl 'https://dl.wei1xuan.com/clashforge-openwrt-hyperv.vhdx'" -ForegroundColor Yellow
            Write-Host "    3. Local   : .\clashforgectl.ps1 hyperv -VHDXPath C:\path\to\clashforge.vhdx" -ForegroundColor Yellow
            exit 1
        }

        $vhdxAsset = $release.assets | Where-Object { $_.name -like '*.vhdx' } | Select-Object -First 1
        if (-not $vhdxAsset) {
            Write-Fail "No .vhdx asset found in release '$($release.tag_name)'."
            Write-Host "  Has the 'Build OpenWrt + ClashForge Hyper-V Image' workflow run for this release?"
            exit 1
        }

        New-Item -ItemType Directory -Path $VHDXDir -Force | Out-Null
        $VHDXPath = Join-Path $VHDXDir $vhdxAsset.name

        if (Test-Path $VHDXPath) {
            Write-Warn "File already exists at $VHDXPath, skipping download."
        } else {
            $dlUrl = if ($mirrorPrefix) { "${mirrorPrefix}$($vhdxAsset.browser_download_url)" } `
                     else               { $vhdxAsset.browser_download_url }
            Invoke-DownloadFile -Url $dlUrl -Dest $VHDXPath
        }
    }

    # ── 2. Create internal LAN switch ────────────────────────────────────────────
    Write-Step "Configuring Hyper-V internal switch: $LanSwitchName"

    $lanSwitch = Get-VMSwitch -Name $LanSwitchName -ErrorAction SilentlyContinue
    if (-not $lanSwitch) {
        New-VMSwitch -Name $LanSwitchName -SwitchType Internal | Out-Null
        Write-OK "Created internal switch '$LanSwitchName'"
    } else {
        Write-OK "Switch '$LanSwitchName' already exists"
    }

    $vEth = Get-NetAdapter | Where-Object { $_.Name -like "*$LanSwitchName*" } | Select-Object -First 1
    if ($vEth) {
        Get-NetIPAddress -InterfaceIndex $vEth.InterfaceIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue |
            Where-Object { $_.PrefixOrigin -ne 'WellKnown' } |
            Remove-NetIPAddress -Confirm:$false -ErrorAction SilentlyContinue
        Set-NetIPInterface -InterfaceIndex $vEth.InterfaceIndex -Dhcp Enabled -ErrorAction SilentlyContinue
        Write-OK "vEthernet ($LanSwitchName) set to DHCP — OpenWrt will assign an IP after VM boots"
    } else {
        Write-Warn "Could not find vEthernet adapter for '$LanSwitchName'"
    }

    # ── 3. Check Default Switch (for WAN) ────────────────────────────────────────
    Write-Step "Verifying WAN switch"
    $defaultSwitch = Get-VMSwitch | Where-Object { $_.Name -eq 'Default Switch' } | Select-Object -First 1
    if (-not $defaultSwitch) {
        Write-Warn "'Default Switch' not found. Using first external/NAT switch as WAN."
        $defaultSwitch = Get-VMSwitch | Where-Object { $_.SwitchType -in 'External','Internal' } | Select-Object -First 1
        if (-not $defaultSwitch) {
            Write-Fail "No suitable WAN switch found. Connect the VM to a switch manually after creation."
        }
    }
    $wanSwitchName = if ($defaultSwitch) { $defaultSwitch.Name } else { '' }
    Write-OK "WAN switch: $wanSwitchName"

    # ── 4. Create VM ─────────────────────────────────────────────────────────────
    Write-Step "Creating VM: $VMName"

    $null = New-VM -Name $VMName -Generation 1 -MemoryStartupBytes ($MemoryMB * 1MB) -NoVHD
    Set-VMProcessor -VMName $VMName -Count $CPUCount
    Set-VMMemory    -VMName $VMName -DynamicMemoryEnabled $false -StartupBytes ($MemoryMB * 1MB)
    Set-VMDvdDrive  -VMName $VMName -Path $null -ErrorAction SilentlyContinue
    Add-VMHardDiskDrive -VMName $VMName -Path $VHDXPath -ControllerType IDE -ControllerNumber 0 -ControllerLocation 0
    Set-VMBios          -VMName $VMName -StartupOrder @('IDE', 'CD', 'LegacyNetworkAdapter', 'Floppy')
    Write-OK "VM created (Gen1, ${MemoryMB}MB RAM, ${CPUCount} vCPU)"

    # ── 5. Configure NICs ────────────────────────────────────────────────────────
    Write-Step "Configuring network adapters"

    Get-VMNetworkAdapter -VMName $VMName | Remove-VMNetworkAdapter

    if ($wanSwitchName) {
        Add-VMNetworkAdapter -VMName $VMName -SwitchName $wanSwitchName -Name 'WAN'
        Write-OK "WAN NIC -> $wanSwitchName"
    } else {
        Add-VMNetworkAdapter -VMName $VMName -Name 'WAN'
        Write-Warn "WAN NIC created but not connected (no switch found)"
    }
    Add-VMNetworkAdapter -VMName $VMName -SwitchName $LanSwitchName -Name 'LAN'
    Write-OK "LAN NIC -> $LanSwitchName (OpenWrt LAN: $LanIP)"

    # ── 6. Start VM ──────────────────────────────────────────────────────────────
    Write-Step "Starting VM"
    Start-VM -Name $VMName
    Write-OK "VM started"

    # ── 7. Wait for web UI ───────────────────────────────────────────────────────
    Write-Step "Waiting for ClashForge to come online (up to 90s)..."
    $deadline = (Get-Date).AddSeconds(90)
    $online   = $false
    while ((Get-Date) -lt $deadline) {
        try {
            $r = Invoke-WebRequest -Uri "http://${LanIP}:7777" -UseBasicParsing -TimeoutSec 2 -ErrorAction Stop
            if ($r.StatusCode -lt 500) { $online = $true; break }
        } catch { }
        Write-Host '.' -NoNewline
        Start-Sleep 3
    }
    Write-Host ''

    if ($online) {
        Write-OK "ClashForge is online!"
    } else {
        Write-Warn "Could not reach http://${LanIP}:7777 within 90s. VM may still be booting."
    }

    # ── 8. Configure subscription + apply TUN mode ───────────────────────────────
    $serviceStarted = $false

    if ($SubscriptionURL -eq '') {
        Write-Host ''
        Write-Host '  No -SubscriptionURL provided.' -ForegroundColor Yellow
        Write-Host '  VM is up with empty config — ClashForge service has NOT been started.' -ForegroundColor Yellow
        Write-Host ''
        Write-Host '  To start with a subscription, re-run:' -ForegroundColor DarkGray
        Write-Host "    .\clashforgectl.ps1 hyperv -SubscriptionURL 'https://your-sub-url/...'" -ForegroundColor DarkGray
        Write-Host "  Or open the web UI to configure manually: http://${LanIP}:7777" -ForegroundColor DarkGray
    } elseif (-not $online) {
        Write-Warn "VM not reachable — cannot configure subscription. Open http://${LanIP}:7777 when it finishes booting, then re-run with -SubscriptionURL."
    } else {
        Write-Step "Configuring subscription"
        $sub   = Invoke-CF -Method POST -Path '/api/v1/subscriptions' -Body @{
            name = 'auto'; type = 'url'; url = $SubscriptionURL; enabled = $true
        }
        $subId = $sub.id
        Write-OK "Subscription added (id=$subId)"

        Write-Host "  Fetching nodes from subscription URL..."
        try {
            Invoke-CF -Method POST -Path "/api/v1/subscriptions/$subId/sync-update" | Out-Null
            Write-OK "Nodes fetched"
        } catch {
            Write-Warn "sync-update failed ($_) — will proceed anyway"
        }

        Write-Step "Applying TUN mode (transparent proxy)"
        $applyResult = Invoke-CF -Method POST -Path '/api/v1/core/apply' -SSE -Body @{
            source  = @{ type = 'sub_id'; sub_id = $subId; sync = $false }
            network = @{ mode = 'tun'; firewall_backend = 'auto'; bypass_lan = $true; bypass_china = $false; apply_on_start = $true; ipv6 = $false }
            dns     = @{ enable = $true; mode = 'fake-ip'; dnsmasq_mode = 'upstream'; apply_on_start = $true; nameservers = @('119.29.29.29', '223.5.5.5'); fallback = @('8.8.8.8', '1.1.1.1') }
        }
        if ($applyResult -and $applyResult.success) {
            Write-OK "TUN mode active — proxy is running"
            $serviceStarted = $true
        } else {
            $errMsg = if ($applyResult) { $applyResult.error } else { 'no response' }
            Write-Warn "Apply finished with issues: $errMsg"
            Write-Host "  Open http://${LanIP}:7777 to check status and configure manually." -ForegroundColor DarkGray
        }
    }

    # ── Summary ───────────────────────────────────────────────────────────────────
    Write-Host ''
    Write-Host '------------------------------------------------' -ForegroundColor DarkGray
    if ($serviceStarted) {
        Write-Host " ClashForge VM is running  — proxy ACTIVE" -ForegroundColor Green
        Write-Host '------------------------------------------------' -ForegroundColor DarkGray
        Write-Host "  VM Name   : $VMName"
        Write-Host "  Web UI    : http://${LanIP}:7777"
        Write-Host "  HTTP Proxy: ${LanIP}:17890"
        Write-Host "  SOCKS5    : ${LanIP}:17891"
        Write-Host "  Mixed     : ${LanIP}:17893"
        Write-Host ''
        Write-Host "  Run 'hyperv-route' to control which traffic goes through ClashForge." -ForegroundColor DarkGray
    } else {
        Write-Host " ClashForge VM is booted — proxy NOT started" -ForegroundColor Yellow
        Write-Host '------------------------------------------------' -ForegroundColor DarkGray
        Write-Host "  VM Name   : $VMName"
        Write-Host "  Web UI    : http://${LanIP}:7777"
        Write-Host ''
        Write-Host "  No proxy service is running. To start it:" -ForegroundColor DarkGray
        Write-Host "    .\clashforgectl.ps1 hyperv -SubscriptionURL 'https://your-sub-url/...'" -ForegroundColor DarkGray
    }
    Write-Host "  To stop VM : .\clashforgectl.ps1 hyperv-stop" -ForegroundColor DarkGray
    Write-Host '------------------------------------------------' -ForegroundColor DarkGray

    try { if ($online) { Start-Process "http://${LanIP}:7777" } } catch { }

    # ── Auto netdiag — only when service is confirmed running ────────────────────
    if ($serviceStarted) {
        Write-Host ''
        Write-Host '════════════════════════════════════════════════' -ForegroundColor DarkCyan
        Write-Host '  Network Diagnostics (netdiag)' -ForegroundColor Cyan
        Write-Host '  Verifying ClashForge + Hyper-V is working correctly...' -ForegroundColor DarkGray
        Write-Host '════════════════════════════════════════════════' -ForegroundColor DarkCyan
        Write-Host ''
        & $PSCommandPath netdiag -Router $LanIP -ApiPort $ApiPort
    }

    exit 0
}

# ══════════════════════════════════════════════════════════════════════════════
# HYPERV-REMOVE — remove VM, switch, VHDX and proxy created by 'hyperv'
# Requires Administrator privileges.
# ══════════════════════════════════════════════════════════════════════════════
if ($Action -eq "hyperv-remove") {
    $isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    if (-not $isAdmin) {
        Die "The 'hyperv-remove' action requires Administrator privileges.`nRight-click PowerShell -> 'Run as administrator', then retry."
    }

    function Write-Step([string]$msg) { Write-Host "`n==> $msg" -ForegroundColor Cyan }
    function Write-OK  ([string]$msg) { Write-Host "  [OK] $msg" -ForegroundColor Green }
    function Write-Warn([string]$msg) { Write-Host "  [!!] $msg" -ForegroundColor Yellow }

    $removed = [System.Collections.Generic.List[string]]::new()
    $skipped = [System.Collections.Generic.List[string]]::new()

    function Confirm-Remove([string]$prompt) {
        if ($Yes) { return $true }
        return (Read-Host "  $prompt [y/N]") -match '^[Yy]'
    }

    # ── 1. Stop and remove VM ────────────────────────────────────────────────────
    Write-Step "VM: $VMName"
    $vm = Get-VM -Name $VMName -ErrorAction SilentlyContinue
    if ($vm) {
        Write-Host "  State: $($vm.State)  Generation: $($vm.Generation)"

        # Collect VHDX paths before VM is removed
        $vhdxPaths = @(Get-VMHardDiskDrive -VMName $VMName -ErrorAction SilentlyContinue |
            Select-Object -ExpandProperty Path | Where-Object { $_ -and (Test-Path $_) })

        if (Confirm-Remove "Remove VM '$VMName'?") {
            if ($vm.State -ne 'Off') {
                Write-Host "  Stopping VM (force power-off)..."
                Stop-VM -Name $VMName -TurnOff -Confirm:$false
                Write-OK "VM stopped"
            }
            Remove-VM -Name $VMName -Force -Confirm:$false
            Write-OK "VM '$VMName' removed"
            $removed.Add("VM '$VMName'")

            # ── 1b. Optionally delete VHDX files ─────────────────────────────────
            foreach ($vhdx in $vhdxPaths) {
                $sizeMB = '{0:N0}' -f ((Get-Item $vhdx).Length / 1MB)
                Write-Host "  VHDX: $vhdx  (${sizeMB} MB)" -ForegroundColor DarkGray
                if (Confirm-Remove "Delete this VHDX?") {
                    Remove-Item $vhdx -Force
                    Write-OK "Deleted VHDX: $(Split-Path $vhdx -Leaf)"
                    $removed.Add("VHDX: $vhdx")
                    # Remove parent dir if now empty
                    $parent = Split-Path $vhdx -Parent
                    if ($parent -and (Test-Path $parent) -and @(Get-ChildItem $parent -ErrorAction SilentlyContinue).Count -eq 0) {
                        Remove-Item $parent -Force
                        Write-OK "Removed empty directory: $parent"
                    }
                } else {
                    Write-Warn "VHDX kept: $vhdx"
                    $skipped.Add("VHDX: $vhdx")
                }
            }
        } else {
            Write-Warn "Skipped VM removal"
            $skipped.Add("VM '$VMName'")
        }
    } else {
        Write-Host "  VM '$VMName' not found — already removed or never created" -ForegroundColor DarkGray
        $skipped.Add("VM '$VMName' (not found)")

        # Fallback: scan $VHDXDir for orphaned .vhdx / .avhdx files
        if ($VHDXDir -and (Test-Path $VHDXDir)) {
            $orphans = @(Get-ChildItem $VHDXDir -Recurse -Include '*.vhdx','*.avhdx','*.sha256' -ErrorAction SilentlyContinue)
            if ($orphans.Count -gt 0) {
                Write-Step "Orphaned VHDX files in $VHDXDir"
                foreach ($vhdx in $orphans) {
                    $sizeMB = '{0:N0}' -f ($vhdx.Length / 1MB)
                    Write-Host "  $($vhdx.FullName)  (${sizeMB} MB)" -ForegroundColor DarkGray
                    if (Confirm-Remove "Delete this file?") {
                        Remove-Item $vhdx.FullName -Force
                        Write-OK "Deleted: $($vhdx.Name)"
                        $removed.Add("VHDX: $($vhdx.FullName)")
                    } else {
                        Write-Warn "Kept: $($vhdx.Name)"
                        $skipped.Add("VHDX: $($vhdx.FullName)")
                    }
                }
                # Remove directory if now empty
                Get-ChildItem $VHDXDir -Recurse -Directory -ErrorAction SilentlyContinue |
                    Sort-Object FullName -Descending |
                    Where-Object { @(Get-ChildItem $_.FullName -ErrorAction SilentlyContinue).Count -eq 0 } |
                    ForEach-Object {
                        Remove-Item $_.FullName -Force
                        Write-OK "Removed empty directory: $($_.FullName)"
                    }
                if ((Test-Path $VHDXDir) -and @(Get-ChildItem $VHDXDir -ErrorAction SilentlyContinue).Count -eq 0) {
                    Remove-Item $VHDXDir -Force
                    Write-OK "Removed empty directory: $VHDXDir"
                }
            }
        }
    }

    # ── 2. Remove internal LAN switch ────────────────────────────────────────────
    Write-Step "Hyper-V switch: $LanSwitchName"
    $lanSwitch = Get-VMSwitch -Name $LanSwitchName -ErrorAction SilentlyContinue
    if ($lanSwitch) {
        # Safety: check if any OTHER VM (not $VMName) still uses this switch
        $otherVMsOnSwitch = @(
            Get-VM | Where-Object { $_.Name -ne $VMName } | ForEach-Object {
                Get-VMNetworkAdapter -VM $_ -ErrorAction SilentlyContinue |
                    Where-Object { $_.SwitchName -eq $LanSwitchName }
            }
        )
        if ($otherVMsOnSwitch.Count -gt 0) {
            Write-Warn "Switch '$LanSwitchName' is still used by $($otherVMsOnSwitch.Count) other VM(s) — skipping"
            $skipped.Add("Switch '$LanSwitchName' (used by other VMs)")
        } elseif (Confirm-Remove "Remove switch '$LanSwitchName' (and its host vEthernet adapter)?") {
            Remove-VMSwitch -Name $LanSwitchName -Confirm:$false
            Write-OK "Switch '$LanSwitchName' removed"
            $removed.Add("Switch '$LanSwitchName'")
        } else {
            Write-Warn "Skipped switch removal"
            $skipped.Add("Switch '$LanSwitchName'")
        }
    } else {
        Write-Host "  Switch '$LanSwitchName' not found — already removed or never created" -ForegroundColor DarkGray
        $skipped.Add("Switch '$LanSwitchName' (not found)")
    }

    # ── Summary ───────────────────────────────────────────────────────────────────
    Write-Host ''
    Write-Host '------------------------------------------------' -ForegroundColor DarkGray
    if ($removed.Count -gt 0) {
        Write-Host " Removed:" -ForegroundColor Green
        foreach ($r in $removed) { Write-Host "  [OK] $r" -ForegroundColor Green }
    }
    if ($skipped.Count -gt 0) {
        Write-Host " Skipped / not found:" -ForegroundColor Yellow
        foreach ($s in $skipped) { Write-Host "  [ ] $s" -ForegroundColor DarkGray }
    }
    if ($removed.Count -eq 0 -and $skipped.Count -gt 0) {
        Write-Host " Nothing was removed." -ForegroundColor Yellow
    }
    Write-Host '------------------------------------------------' -ForegroundColor DarkGray
    exit 0
}

# ══════════════════════════════════════════════════════════════════════════════
# HYPERV-STOP — stop VM + disable ClashForge-LAN adapter (restore original network)
# ══════════════════════════════════════════════════════════════════════════════
if ($Action -eq "hyperv-stop") {
    $isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    if (-not $isAdmin) { Die "The 'hyperv-stop' action requires Administrator privileges." }

    function Write-Step([string]$msg) { Write-Host "`n==> $msg" -ForegroundColor Cyan }
    function Write-OK  ([string]$msg) { Write-Host "  [OK] $msg" -ForegroundColor Green }
    function Write-Warn([string]$msg) { Write-Host "  [!!] $msg" -ForegroundColor Yellow }

    # ── 1. Stop VM ───────────────────────────────────────────────────────────────
    Write-Step "Stopping VM: $VMName"
    $vm = Get-VM -Name $VMName -ErrorAction SilentlyContinue
    if ($vm) {
        if ($vm.State -eq 'Off') {
            Write-Warn "VM '$VMName' is already stopped"
        } else {
            Stop-VM -Name $VMName -Confirm:$false
            Write-OK "VM '$VMName' stopped"
        }
    } else {
        Write-Warn "VM '$VMName' not found"
    }

    # ── 2. Disable ClashForge-LAN adapter ────────────────────────────────────────
    Write-Step "Disabling adapter: vEthernet ($LanSwitchName)"
    $adapter = Get-NetAdapter | Where-Object { $_.Name -like "*$LanSwitchName*" } | Select-Object -First 1
    if ($adapter) {
        if ($adapter.Status -eq 'Disabled') {
            Write-Warn "Adapter already disabled"
        } else {
            Disable-NetAdapter -Name $adapter.Name -Confirm:$false
            Write-OK "vEthernet ($LanSwitchName) disabled"
        }
    } else {
        Write-Warn "Adapter 'vEthernet ($LanSwitchName)' not found"
    }

    # ── 3. Show active network ────────────────────────────────────────────────────
    Write-Step "Active network adapters"
    Get-NetIPInterface -AddressFamily IPv4 | Where-Object { $_.ConnectionState -eq 'Connected' } |
        Sort-Object InterfaceMetric |
        ForEach-Object {
            $ip = (Get-NetIPAddress -InterfaceIndex $_.InterfaceIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue |
                   Where-Object { $_.PrefixOrigin -ne 'WellKnown' } | Select-Object -First 1).IPAddress
            Write-Host ("  metric {0,5}  {1,-35}  {2}" -f $_.InterfaceMetric, $_.InterfaceAlias, ($ip ?? '')) -ForegroundColor White
        }

    Write-Host ''
    Write-OK "Original network is active. Run 'hyperv-start' to bring ClashForge back."
    exit 0
}

# ══════════════════════════════════════════════════════════════════════════════
# HYPERV-START — enable ClashForge-LAN adapter + start VM
# ══════════════════════════════════════════════════════════════════════════════
if ($Action -eq "hyperv-start") {
    $isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    if (-not $isAdmin) { Die "The 'hyperv-start' action requires Administrator privileges." }

    function Write-Step([string]$msg) { Write-Host "`n==> $msg" -ForegroundColor Cyan }
    function Write-OK  ([string]$msg) { Write-Host "  [OK] $msg" -ForegroundColor Green }
    function Write-Warn([string]$msg) { Write-Host "  [!!] $msg" -ForegroundColor Yellow }

    # ── 1. Enable adapter ────────────────────────────────────────────────────────
    Write-Step "Enabling adapter: vEthernet ($LanSwitchName)"
    $adapter = Get-NetAdapter | Where-Object { $_.Name -like "*$LanSwitchName*" } | Select-Object -First 1
    if ($adapter) {
        if ($adapter.Status -ne 'Disabled') {
            Write-Warn "Adapter already up (Status=$($adapter.Status))"
        } else {
            Enable-NetAdapter -Name $adapter.Name -Confirm:$false
            Start-Sleep -Seconds 1
            Write-OK "vEthernet ($LanSwitchName) enabled"
        }
        # Set metric high so it doesn't preempt the original NIC until user runs hyperv-route
        Set-NetIPInterface -InterfaceAlias $adapter.Name -AddressFamily IPv4 -InterfaceMetric 9000 -ErrorAction SilentlyContinue
        Set-NetIPInterface -InterfaceAlias $adapter.Name -AddressFamily IPv6 -InterfaceMetric 9000 -ErrorAction SilentlyContinue
        Write-OK "Metric set to 9000 — original NIC still preferred (run 'hyperv-route' to switch)"
    } else {
        Write-Warn "Adapter 'vEthernet ($LanSwitchName)' not found — was the switch removed?"
    }

    # ── 2. Start VM ──────────────────────────────────────────────────────────────
    Write-Step "Starting VM: $VMName"
    $vm = Get-VM -Name $VMName -ErrorAction SilentlyContinue
    if (-not $vm) {
        Write-Warn "VM '$VMName' not found. Run 'hyperv' to create it."
        exit 1
    }
    if ($vm.State -eq 'Running') {
        Write-Warn "VM '$VMName' is already running"
    } else {
        Start-VM -Name $VMName
        Write-OK "VM '$VMName' started"
    }

    # ── 3. Wait for web UI ───────────────────────────────────────────────────────
    Write-Step "Waiting for ClashForge to come online (up to 60s)..."
    $deadline = (Get-Date).AddSeconds(60)
    $online   = $false
    while ((Get-Date) -lt $deadline) {
        try {
            $r = Invoke-WebRequest -Uri "http://${LanIP}:7777" -UseBasicParsing -TimeoutSec 2 -ErrorAction Stop
            if ($r.StatusCode -lt 500) { $online = $true; break }
        } catch { }
        Write-Host '.' -NoNewline
        Start-Sleep 3
    }
    Write-Host ''
    if ($online) {
        Write-OK "ClashForge is online at http://${LanIP}:7777"
    } else {
        Write-Warn "VM started but web UI not yet reachable — it may still be booting"
    }
    Write-Host "  Run 'hyperv-route -HvMode clashforge' to route traffic through ClashForge." -ForegroundColor DarkGray
    exit 0
}

# ══════════════════════════════════════════════════════════════════════════════
# HYPERV-ROUTE — show + switch routing priority between ClashForge and original NIC
# Metrics: ClashForge mode = 5 (takes default gateway), Direct mode = 9000 (lowest priority)
# ══════════════════════════════════════════════════════════════════════════════
if ($Action -eq "hyperv-route") {
    $isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    if (-not $isAdmin) { Die "The 'hyperv-route' action requires Administrator privileges." }

    function Write-Step([string]$msg) { Write-Host "`n==> $msg" -ForegroundColor Cyan }
    function Write-OK  ([string]$msg) { Write-Host "  [OK] $msg" -ForegroundColor Green }
    function Write-Warn([string]$msg) { Write-Host "  [!!] $msg" -ForegroundColor Yellow }

    $CF_METRIC_HIGH  = 5     # ClashForge-LAN takes priority (becomes default gateway)
    $CF_METRIC_LOW   = 9000  # ClashForge-LAN is lowest priority

    # ── Collect adapter state ────────────────────────────────────────────────────
    $lanAlias   = "vEthernet ($LanSwitchName)"
    $lanIface   = Get-NetIPInterface -InterfaceAlias $lanAlias -AddressFamily IPv4 -ErrorAction SilentlyContinue
    $lanAdapter = Get-NetAdapter | Where-Object { $_.Name -eq $lanAlias } | Select-Object -First 1

    $allIfaces = Get-NetIPInterface -AddressFamily IPv4 |
        Where-Object { $_.InterfaceAlias -notlike '*Loopback*' } |
        Sort-Object InterfaceMetric

    Write-Step "Current network priority (IPv4)"
    Write-Host ""
    foreach ($iface in $allIfaces) {
        $adp = Get-NetAdapter -InterfaceIndex $iface.InterfaceIndex -ErrorAction SilentlyContinue
        $ip  = (Get-NetIPAddress -InterfaceIndex $iface.InterfaceIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue |
                Where-Object { $_.PrefixOrigin -ne 'WellKnown' } | Select-Object -First 1).IPAddress

        $isCF     = ($iface.InterfaceAlias -eq $lanAlias)
        $isUp     = ($adp -and $adp.Status -eq 'Up')
        $connStr  = if ($iface.ConnectionState -eq 'Connected') { 'Connected  ' } else { 'Disconnected' }
        $statusStr = if (-not $isUp) { '[Disabled]' } elseif ($iface.ConnectionState -eq 'Connected') { '[ Active ]' } else { '[No Link ]' }
        $color     = if ($isCF -and $isUp -and $iface.InterfaceMetric -lt 100)  { 'Green'  } `
                     elseif ($isCF)                                               { 'Yellow' } `
                     elseif ($isUp -and $iface.ConnectionState -eq 'Connected')  { 'Cyan'   } `
                     else                                                          { 'DarkGray' }
        $tag = if ($isCF) { '<-- ClashForge-LAN' } else { '' }
        Write-Host ("  metric {0,5}  {1}  {2,-35}  {3,-18}  {4}" -f `
            $iface.InterfaceMetric, $statusStr, $iface.InterfaceAlias, ($ip ?? ''), $tag) -ForegroundColor $color
    }
    Write-Host ""

    # ── Determine current mode ───────────────────────────────────────────────────
    $lanMetric         = if ($lanIface) { $lanIface.InterfaceMetric } else { 99999 }
    $isClashForgeMode  = ($lanMetric -le 100)  # metric ≤ 100 means ClashForge is preferred
    $currentModeLabel  = if ($isClashForgeMode) { 'clashforge (ClashForge-LAN is preferred gateway)' } `
                         else                    { 'direct (original NIC is preferred gateway)' }
    Write-Host "  Current mode: $currentModeLabel" -ForegroundColor White

    # ── Determine target mode ────────────────────────────────────────────────────
    $targetMode = if ($HvMode -ne '') { $HvMode } `
                  elseif ($isClashForgeMode) { 'direct' } `
                  else                        { 'clashforge' }

    if ($HvMode -eq '') {
        Write-Host "  Auto-toggle -> switching to: $targetMode" -ForegroundColor DarkGray
    }

    # ── Guard: adapter must exist and be Up ──────────────────────────────────────
    if (-not $lanAdapter) {
        Write-Warn "Adapter '$lanAlias' not found. Run 'hyperv-start' first."
        exit 1
    }
    if ($lanAdapter.Status -eq 'Disabled' -and $targetMode -eq 'clashforge') {
        Write-Warn "Adapter '$lanAlias' is disabled. Run 'hyperv-start' to enable it first."
        exit 1
    }

    # ── Apply ────────────────────────────────────────────────────────────────────
    Write-Step "Applying: $targetMode mode"

    if ($targetMode -eq 'clashforge') {
        Set-NetIPInterface -InterfaceAlias $lanAlias -AddressFamily IPv4 -InterfaceMetric $CF_METRIC_HIGH -ErrorAction Stop
        Set-NetIPInterface -InterfaceAlias $lanAlias -AddressFamily IPv6 -InterfaceMetric $CF_METRIC_HIGH -ErrorAction SilentlyContinue
        Write-OK "ClashForge-LAN metric -> $CF_METRIC_HIGH (now preferred gateway)"
        Write-Host "  All traffic will route through ClashForge ($LanIP)." -ForegroundColor DarkGray
        Write-Host "  Proxy ports: HTTP=$LanIP`:17890  SOCKS5=$LanIP`:17891  Mixed=$LanIP`:17893" -ForegroundColor DarkGray
    } else {
        Set-NetIPInterface -InterfaceAlias $lanAlias -AddressFamily IPv4 -InterfaceMetric $CF_METRIC_LOW -ErrorAction Stop
        Set-NetIPInterface -InterfaceAlias $lanAlias -AddressFamily IPv6 -InterfaceMetric $CF_METRIC_LOW -ErrorAction SilentlyContinue
        Write-OK "ClashForge-LAN metric -> $CF_METRIC_LOW (original NIC now preferred)"
        Write-Host "  Traffic bypasses ClashForge — using your original network." -ForegroundColor DarkGray
    }

    # ── Show updated table ────────────────────────────────────────────────────────
    Write-Step "Updated network priority"
    Write-Host ""
    Get-NetIPInterface -AddressFamily IPv4 |
        Where-Object { $_.InterfaceAlias -notlike '*Loopback*' } |
        Sort-Object InterfaceMetric |
        ForEach-Object {
            $adp  = Get-NetAdapter -InterfaceIndex $_.InterfaceIndex -ErrorAction SilentlyContinue
            $ip   = (Get-NetIPAddress -InterfaceIndex $_.InterfaceIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue |
                     Where-Object { $_.PrefixOrigin -ne 'WellKnown' } | Select-Object -First 1).IPAddress
            $isCF = ($_.InterfaceAlias -eq $lanAlias)
            $isUp = ($adp -and $adp.Status -eq 'Up')
            $statusStr = if (-not $isUp) { '[Disabled]' } elseif ($_.ConnectionState -eq 'Connected') { '[ Active ]' } else { '[No Link ]' }
            $color = if ($isCF -and $isUp -and $_.InterfaceMetric -lt 100) { 'Green' } `
                     elseif ($isCF)                                          { 'Yellow' } `
                     elseif ($isUp -and $_.ConnectionState -eq 'Connected') { 'Cyan' }   `
                     else                                                     { 'DarkGray' }
            $tag = if ($isCF) { '<-- ClashForge-LAN' } else { '' }
            Write-Host ("  metric {0,5}  {1}  {2,-35}  {3,-18}  {4}" -f `
                $_.InterfaceMetric, $statusStr, $_.InterfaceAlias, ($ip ?? ''), $tag) -ForegroundColor $color
        }
    Write-Host ""
    exit 0
}

function Download-WithFallback {
    param(
        [Parameter(Mandatory)][string]$Name,
        [Parameter(Mandatory)][string]$OutFile,
        [Parameter(Mandatory)][string[]]$Urls
    )

    foreach ($Url in $Urls) {
        try {
            Log "Downloading $Name from $Url"
            Invoke-WebRequest -Uri $Url -OutFile $OutFile -TimeoutSec 180 -UseBasicParsing
            if ((Test-Path $OutFile) -and ((Get-Item $OutFile).Length -gt 0)) {
                Ok "$Name downloaded ($((Get-Item $OutFile).Length) bytes)"
                return $true
            }
            Warn "$Name download produced an empty file"
        } catch {
            Warn "$Name download failed: $_"
        }
        Remove-Item $OutFile -Force -ErrorAction SilentlyContinue
    }
    return $false
}

function Sync-GeoDataToRouter {
    if ($DryRun) {
        Warn "Skipping GeoData preload because -DryRun is set"
        return
    }

    $RemoteGeoDir = "/etc/metaclash"

    # Primary: GitHub releases; fallback: jsdmirror CDN (mirrors clashforgectl bash)
    $GeoSpecs = @(
        @{
            Name = "country.mmdb"
            File = "country.mmdb"
            Urls = @(
                "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb",
                "https://cdn.jsdmirror.com/gh/MetaCubeX/meta-rules-dat@release/country.mmdb"
            )
        },
        @{
            Name = "GeoIP.dat"
            File = "GeoIP.dat"
            Urls = @(
                "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.dat",
                "https://cdn.jsdmirror.com/gh/MetaCubeX/meta-rules-dat@release/geoip.dat"
            )
        },
        @{
            Name = "GeoSite.dat"
            File = "GeoSite.dat"
            Urls = @(
                "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat",
                "https://cdn.jsdmirror.com/gh/MetaCubeX/meta-rules-dat@release/geosite.dat"
            )
        }
    )

    # Check which files are already present on the router
    $NeedUpload = @{ "country.mmdb" = $true; "GeoIP.dat" = $true; "GeoSite.dat" = $true }
    try {
        $SshCheckCmd = '[ -s /etc/metaclash/country.mmdb ] && echo mmdb=1   || echo mmdb=0; ' +
                       '[ -s /etc/metaclash/GeoIP.dat    ] && echo GeoIP=1  || echo GeoIP=0; ' +
                       '[ -s /etc/metaclash/GeoSite.dat  ] && echo GeoSite=1|| echo GeoSite=0'
        $SshCheckArgs = $SshBase + @($Target, $SshCheckCmd)
        $RemoteState = ssh @SshCheckArgs 2>$null
        if ($LASTEXITCODE -eq 0 -and $RemoteState) {
            foreach ($Line in $RemoteState) {
                switch -Regex ($Line.Trim()) {
                    '^mmdb=1$'    { $NeedUpload["country.mmdb"] = $false }
                    '^GeoIP=1$'   { $NeedUpload["GeoIP.dat"]    = $false }
                    '^GeoSite=1$' { $NeedUpload["GeoSite.dat"]  = $false }
                }
            }
        } else {
            Warn "Unable to check existing GeoData on router, falling back to local preload"
        }
    } catch {
        Warn "Unable to check existing GeoData on router, falling back to local preload"
    }

    if (-not $NeedUpload["country.mmdb"] -and -not $NeedUpload["GeoIP.dat"] -and -not $NeedUpload["GeoSite.dat"]) {
        Ok "All GeoData files already exist on router, skipping preload"
        return
    }

    Log "── GeoData: preloading missing files only"
    $GeoTempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("clashforge-geodata-" + [Guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Force -Path $GeoTempDir | Out-Null

    try {
        $DownloadedFiles = @()
        foreach ($Spec in $GeoSpecs) {
            if (-not $NeedUpload[$Spec.File]) {
                Ok "$($Spec.File) already exists on router, skip download/upload"
                continue
            }
            $LocalFilePath = Join-Path $GeoTempDir $Spec.File
            if (Download-WithFallback -Name $Spec.Name -OutFile $LocalFilePath -Urls $Spec.Urls) {
                $DownloadedFiles += $Spec.File
            }
        }

        if ($DownloadedFiles.Count -eq 0) {
            Warn "GeoData preload skipped: no GeoData files could be downloaded locally"
            return
        }

        $SshMkdirArgs = $SshBase + @($Target, "mkdir -p $RemoteGeoDir")
        ssh @SshMkdirArgs
        if ($LASTEXITCODE -ne 0) {
            Warn "GeoData preload skipped: cannot create $RemoteGeoDir on router"
            return
        }

        $UploadedPaths = @()
        Push-Location $GeoTempDir
        try {
            foreach ($File in $DownloadedFiles) {
                $ScpGeoArgs = $ScpBase + @(".\$File", "${Target}:${RemoteGeoDir}/$File")
                scp @ScpGeoArgs
                if ($LASTEXITCODE -ne 0) {
                    Warn "Failed to upload $File to router"
                    continue
                }
                Ok "$File uploaded to ${RemoteGeoDir}/$File"
                $UploadedPaths += "$RemoteGeoDir/$File"
            }
        } finally {
            Pop-Location
        }

        if ($UploadedPaths.Count -gt 0) {
            $ChmodTargets = $UploadedPaths -join " "
            $SshChmodArgs = $SshBase + @($Target, "chmod 644 $ChmodTargets 2>/dev/null || true")
            ssh @SshChmodArgs | Out-Null
        }
    } catch {
        Warn "GeoData preload failed: $_"
    } finally {
        Remove-Item $GeoTempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# ── locate clashforgectl.sh ───────────────────────────────────────────────────
# Look next to this PS1 file first, then in the scripts/ directory.
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ShScript  = Join-Path $ScriptDir "clashforgectl.sh"

if (-not (Test-Path $ShScript)) {
    # Try workspace root scripts/ directory
    $ShScript = Join-Path (Split-Path -Parent $ScriptDir) "scripts\clashforgectl.sh"
}

if (-not (Test-Path $ShScript)) {
    Die "clashforgectl.sh not found next to this script. Expected: $(Join-Path $ScriptDir 'clashforgectl.sh')"
}

# ── validate tools ────────────────────────────────────────────────────────────
foreach ($Tool in @("ssh", "scp")) {
    if (-not (Get-Command $Tool -ErrorAction SilentlyContinue)) {
        Die "$Tool not found. Install OpenSSH: Settings → Apps → Optional Features → OpenSSH Client"
    }
}

# ── show help ─────────────────────────────────────────────────────────────────
if ($Action -eq "help") {
    Get-Help $MyInvocation.MyCommand.Path -Detailed
    exit 0
}

# ── resolve repo root (for deploy) ───────────────────────────────────────────
if ($RepoRoot -eq "") {
    # Walk up from the script directory to find the repo root (contains go.mod)
    $RepoRoot = $ScriptDir
    if (-not (Test-Path (Join-Path $RepoRoot "go.mod"))) {
        $RepoRoot = Split-Path -Parent $ScriptDir
    }
    if (-not (Test-Path (Join-Path $RepoRoot "go.mod"))) {
        Die "Cannot auto-detect repo root (no go.mod found). Pass -RepoRoot explicitly."
    }
}
$RepoRoot = (Resolve-Path $RepoRoot).Path

# ── build SSH / SCP base args ─────────────────────────────────────────────────
$Target  = "${User}@${Router}"
$SshBase = [System.Collections.Generic.List[string]]@(
    "-p", $Port,
    "-o", "StrictHostKeyChecking=no",
    "-o", "ConnectTimeout=15"
)
if ($Identity -ne "") {
    $SshBase.Add("-i")
    $SshBase.Add($Identity)
}

$ScpBase = [System.Collections.Generic.List[string]]@(
    "-P", $Port,
    "-o", "StrictHostKeyChecking=no",
    "-o", "ConnectTimeout=15"
)
if ($Identity -ne "") {
    $ScpBase.Add("-i")
    $ScpBase.Add($Identity)
}

$RemoteScript = "/tmp/clashforgectl.sh"

if ($Router -eq '') {
    Die "-Router is required for '$Action'. Example: .\clashforgectl.ps1 -Router 192.168.1.1 $Action"
}

Log "Router  : $Target  (port $Port)"
Log "Script  : $ShScript"
Log "Action  : $Action"

# ══════════════════════════════════════════════════════════════════════════════
# DEPLOY — local build + package + push to router
# ══════════════════════════════════════════════════════════════════════════════
if ($Action -eq "deploy") {
    Log "Repo root: $RepoRoot"
    Log ""

    # ── 1. Bump patch version ─────────────────────────────────────────────────
    Log "── Step 1: Bumping version"
    $ControlPath = Join-Path $RepoRoot "ipk\CONTROL\control"
    if (-not (Test-Path $ControlPath)) { Die "control file not found: $ControlPath" }
    $ctrl = Get-Content $ControlPath -Raw
    if ($ctrl -match 'Version:\s*([\d\.\-a-z]+)') {
        $OldVer = $Matches[1]
        if ($OldVer -match '^(.*\.)(\d+)$') {
            $NewVer = "$($Matches[1])$([int]$Matches[2] + 1)"
        } else {
            $NewVer = "$OldVer.1"
        }
        $ctrl = $ctrl -replace "Version: $([regex]::Escape($OldVer))", "Version: $NewVer"
        Set-Content $ControlPath $ctrl -NoNewline
        Ok "Version $OldVer  →  $NewVer"
    } else {
        Die "Cannot parse version from control file: $ControlPath"
    }
    $IpkName = "clashforge_${NewVer}_x86_64.ipk"

    # ── 2. Build UI ───────────────────────────────────────────────────────────
    if ($Skip -notmatch '\bui\b') {
        Log "── Step 2: Building React UI"
        Push-Location (Join-Path $RepoRoot "ui")
        try {
            $env:VITE_APP_VERSION = $NewVer
            npm run build | Select-Object -Last 10
            if ($LASTEXITCODE -ne 0) { Die "npm run build failed" }
        } finally {
            $env:VITE_APP_VERSION = ''
            Pop-Location
        }
        Log "Syncing UI dist → internal/api/ui_dist"
        $UiDist = Join-Path $RepoRoot "internal\api\ui_dist"
        Remove-Item -Recurse -Force "$UiDist\*" -ErrorAction SilentlyContinue
        Copy-Item -Recurse (Join-Path $RepoRoot "ui\dist\*") "$UiDist\"
        Ok "UI build complete"
    } else {
        Warn "Skipping UI build (-Skip contains 'ui')"
    }

    # ── 3. Cross-compile Go ───────────────────────────────────────────────────
    if ($Skip -notmatch '\bgo\b') {
        Log "── Step 3: Cross-compiling Go (linux/amd64)"
        $env:GOOS = 'linux'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
        try {
            $BinOut = Join-Path $RepoRoot "ipk\usr\bin\clashforge"
            go build -trimpath -ldflags='-s -w' -o $BinOut (Join-Path $RepoRoot "cmd\clashforge")
            if ($LASTEXITCODE -ne 0) { Die "go build failed" }
        } finally {
            $env:GOOS = ''; $env:GOARCH = ''; $env:CGO_ENABLED = ''
        }
        Ok "Binary written to ipk/usr/bin/clashforge"
    } else {
        Warn "Skipping Go build (-Skip contains 'go')"
    }

    # ── 4. Sync openwrt/files helpers → ipk/ ─────────────────────────────────
    Log "── Step 4: Syncing openwrt/files helpers → ipk/"
    # Remove deprecated helper that was replaced by clashforgectl diag.
    Remove-Item -Force (Join-Path $RepoRoot "ipk\usr\bin\clashforge-diag") -ErrorAction SilentlyContinue
    $HelperMap = @{
        "openwrt\files\etc\init.d\metaclash"                                = "ipk\etc\init.d\clashforge"
        "openwrt\files\usr\share\luci\menu.d\luci-app-clashforge.json"      = "ipk\usr\share\luci\menu.d\luci-app-clashforge.json"
        "openwrt\files\www\luci-static\resources\view\clashforge\main.js"   = "ipk\www\luci-static\resources\view\clashforge\main.js"
        "openwrt\files\usr\share\rpcd\acl.d\luci-app-clashforge.json"       = "ipk\usr\share\rpcd\acl.d\luci-app-clashforge.json"
        "openwrt\files\etc\metaclash\postinst.sh"                           = "ipk\CONTROL\postinst"
        "openwrt\files\etc\metaclash\prerm.sh"                              = "ipk\CONTROL\prerm"
        "openwrt\files\etc\metaclash\postrm.sh"                             = "ipk\CONTROL\postrm"
    }
    foreach ($RelSrc in $HelperMap.Keys) {
        $Src = Join-Path $RepoRoot $RelSrc
        $Dst = Join-Path $RepoRoot $HelperMap[$RelSrc]
        if (Test-Path $Src) {
            $DstDir = Split-Path $Dst
            if (-not (Test-Path $DstDir)) { New-Item -ItemType Directory -Force $DstDir | Out-Null }
            Copy-Item -Force $Src $Dst
            Ok (Split-Path $Src -Leaf)
        } else {
            Warn "Not found (skipped): $RelSrc"
        }
    }

    # ── 4.5. Pre-seed GeoData into IPK staging (best-effort, mirrors CI) ────────
    Log "── Step 4.5: Pre-seeding GeoData into ipk\usr\share\metaclash\"
    $GeoStagingDir = Join-Path $RepoRoot "ipk\usr\share\metaclash"
    New-Item -ItemType Directory -Force -Path $GeoStagingDir | Out-Null
    $GeoStagingSpecs = @(
        @{
            File = "country.mmdb"
            Urls = @(
                "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb",
                "https://cdn.jsdmirror.com/gh/MetaCubeX/meta-rules-dat@release/country.mmdb"
            )
        },
        @{
            File = "GeoIP.dat"
            Urls = @(
                "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.dat",
                "https://cdn.jsdmirror.com/gh/MetaCubeX/meta-rules-dat@release/geoip.dat"
            )
        },
        @{
            File = "GeoSite.dat"
            Urls = @(
                "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat",
                "https://cdn.jsdmirror.com/gh/MetaCubeX/meta-rules-dat@release/geosite.dat"
            )
        }
    )
    $GeoTmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ("cf-geodata-" + [Guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Force -Path $GeoTmpDir | Out-Null
    $GeoSeeded = 0
    try {
        foreach ($Spec in $GeoStagingSpecs) {
            $StagedPath = Join-Path $GeoStagingDir $Spec.File
            if ((Test-Path $StagedPath) -and ((Get-Item $StagedPath).Length -gt 0)) {
                Ok "$($Spec.File) already in staging, skip download"
                $GeoSeeded++
                continue
            }
            $TmpPath = Join-Path $GeoTmpDir $Spec.File
            if (Download-WithFallback -Name $Spec.File -OutFile $TmpPath -Urls $Spec.Urls) {
                Copy-Item -Force $TmpPath $StagedPath
                Ok "$($Spec.File) staged → ipk\usr\share\metaclash\"
                $GeoSeeded++
            }
        }
    } catch {
        Warn "GeoData pre-seed encountered an error: $_"
    } finally {
        Remove-Item $GeoTmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }
    if ($GeoSeeded -eq 3) {
        Ok "All 3 GeoData files bundled into IPK staging"
    } else {
        Warn "GeoData pre-seed partial ($GeoSeeded/3 files) — Sync-GeoDataToRouter will fill gaps after install"
    }

    # ── 5. Build IPK ─────────────────────────────────────────────────────────
    Log "── Step 5: Building IPK ($IpkName)"
    $BuildScript = Join-Path $RepoRoot "scripts\build_ipk.py"
    if (-not (Test-Path $BuildScript)) { Die "build_ipk.py not found: $BuildScript" }
    Push-Location $RepoRoot
    try {
        $env:PKG_NAME = $IpkName
        python $BuildScript
        if ($LASTEXITCODE -ne 0) { Die "build_ipk.py failed" }
        $env:PKG_NAME = ''
    } finally {
        Pop-Location
    }
    $LocalIpk = Join-Path $RepoRoot $IpkName
    Ok "IPK ready: $LocalIpk"

    # ── 6. Upload IPK to router ───────────────────────────────────────────────
    $RemoteIpk = "/tmp/cf_deploy.ipk"
    Log "── Step 6: Uploading $IpkName → ${Target}:${RemoteIpk}"
    # Push to RepoRoot and use relative path: Windows scp misreads 'C:' as hostname
    Push-Location $RepoRoot
    try {
        $ScpIpkArgs = $ScpBase + @(".\$IpkName", "${Target}:${RemoteIpk}")
        scp @ScpIpkArgs
        if ($LASTEXITCODE -ne 0) { Die "scp upload of IPK failed" }
    } finally {
        Pop-Location
    }
    Ok "IPK uploaded"

    # ── 7. Upload clashforgectl.sh ────────────────────────────────────────────
    Log "── Step 7: Uploading clashforgectl.sh → ${Target}:${RemoteScript}"
    $ScpCtlArgs = $ScpBase + @($ShScript, "${Target}:${RemoteScript}")
    scp @ScpCtlArgs
    if ($LASTEXITCODE -ne 0) { Die "scp upload of clashforgectl.sh failed" }
    Ok "clashforgectl.sh uploaded"

    # ── 8. Install on router via clashforgectl upgrade --local-ipk ───────────
    $PurgeFlag = if ($Purge) { " --purge" } else { "" }
    $RemoteInstall = "sh $RemoteScript upgrade --local-ipk $RemoteIpk$PurgeFlag"
    Log "── Step 8: Installing on router"
    Log "Remote  : $RemoteInstall"
    Log ""
    $SshInstallArgs = $SshBase + @($Target, $RemoteInstall)
    ssh @SshInstallArgs
    if ($LASTEXITCODE -ne 0) { Die "Remote install failed (exit $LASTEXITCODE)" }

    Sync-GeoDataToRouter

    Log ""
    Ok "Deploy complete: $NewVer  →  http://${Router}:7777"
    exit 0
}


# ══════════════════════════════════════════════════════════════════════════════
# UPGRADE — download IPK locally, push to router, install via --local-ipk
#
# Default path: the local machine downloads the IPK and SCPs it to the router.
# This avoids the "chicken-and-egg" problem where ClashForge stopping breaks the
# router's internet access before it can download its own replacement.
# Use -RemoteDownload to revert to the old behaviour (router downloads itself).
# ══════════════════════════════════════════════════════════════════════════════
if ($Action -eq "upgrade" -and -not $RemoteDownload) {

    $CleanupLocalIpk = $false   # true only when we create a temp download

    if ($LocalIpkFile -ne "") {
        # ── User-supplied local IPK — skip arch detect, version resolve, download ─
        if (-not (Test-Path $LocalIpkFile)) { Die "Local IPK not found: $LocalIpkFile" }
        $IpkItem = Get-Item $LocalIpkFile
        if ($IpkItem.Length -eq 0) { Die "Local IPK is empty: $LocalIpkFile" }
        $IpkName  = $IpkItem.Name
        $LocalIpk = $LocalIpkFile
        Ok "Local IPK   : $LocalIpkFile ($($IpkItem.Length) bytes)"
    } else {
        # ── 1. Detect router architecture via SSH ─────────────────────────────────
        Log "── Step 1: Detecting router architecture..."
        $SshMachineArgs = $SshBase + @($Target, 'uname -m')
        $RouterMachine  = (ssh @SshMachineArgs).Trim()

        $IpkArch = $null
        if ($RouterMachine -in @("x86_64", "amd64")) {
            $IpkArch = "x86_64"
        } elseif ($RouterMachine -in @("aarch64", "arm64")) {
            $SshCpuArgs = $SshBase + @($Target, "grep -m1 'CPU part' /proc/cpuinfo 2>/dev/null | awk '{print tolower(`$NF)}'")
            $CpuPart    = (ssh @SshCpuArgs).Trim()
            $IpkArch    = if ($CpuPart -eq "0xd03") { "aarch64_cortex-a53" } else { "aarch64_generic" }
        } else {
            Die "Unsupported router architecture: $RouterMachine (supported: x86_64, aarch64)"
        }
        Ok "Router arch : $RouterMachine → $IpkArch"

        # ── 2. Resolve version ─────────────────────────────────────────────────
        # When version is "latest", walk releases newest-first and pick the first
        # one that actually contains the IPK asset for this architecture.
        # This avoids installing a half-published release that has no IPK yet.
        $Tag = $Version
        if ($Tag -eq "latest") {
            Log "── Step 2: Resolving latest release with IPK for $IpkArch..."
            try {
                $Releases = Invoke-RestMethod `
                    -Uri "https://api.github.com/repos/wujun4code/clashforge/releases?per_page=50" `
                    -Headers @{ "Accept" = "application/vnd.github+json" } `
                    -TimeoutSec 15
                # GitHub returns releases newest-first; walk until we find one
                # that has clashforge_<ver>_<arch>.ipk as a published asset.
                foreach ($rel in $Releases) {
                    $tagCandidate = $rel.tag_name
                    if (-not $tagCandidate) { continue }
                    $pkgVerCandidate = $tagCandidate.TrimStart("v")
                    $ipkAssetName    = "clashforge_${pkgVerCandidate}_${IpkArch}.ipk"
                    $hasIpk = $rel.assets | Where-Object { $_.name -eq $ipkAssetName }
                    if ($hasIpk) {
                        $Tag = $tagCandidate
                        break
                    }
                }
            } catch {
                Die "Failed to resolve latest version from GitHub API: $_"
            }
        }
        if (-not $Tag) { Die "Could not find a published release with an IPK for $IpkArch. Check https://github.com/wujun4code/clashforge/releases" }
        $PkgVer  = $Tag.TrimStart("v")
        $IpkName = "clashforge_${PkgVer}_${IpkArch}.ipk"
        Ok "Version     : $Tag"
        Ok "Package     : $IpkName"

        # ── 3. Download IPK to local machine ──────────────────────────────────
        Log "── Step 3: Downloading IPK locally..."
        $LocalIpk  = Join-Path ([System.IO.Path]::GetTempPath()) $IpkName
        $GhUrl     = "https://github.com/wujun4code/clashforge/releases/download/$Tag/$IpkName"
        $GhProxies = @(
            "https://ghproxy.com",
            "https://mirror.ghproxy.com",
            "https://ghfast.top",
            "https://github.moeyy.xyz"
        )
        $Downloaded = $false

        if ($BaseUrl -ne "") {
            $Url = "$($BaseUrl.TrimEnd('/'))/releases/$Tag/$IpkName"
            Log "Downloading from custom base URL: $BaseUrl"
            try {
                Invoke-WebRequest -Uri $Url -OutFile $LocalIpk -TimeoutSec 120 -UseBasicParsing
                $Downloaded = $true
            } catch {
                Die "Download failed from base URL: $Url`n$_"
            }
        } elseif ($Mirror -ne "") {
            $Url = "$($Mirror.TrimEnd('/'))/$GhUrl"
            Log "Downloading via mirror: $Mirror"
            try {
                Invoke-WebRequest -Uri $Url -OutFile $LocalIpk -TimeoutSec 120 -UseBasicParsing
                $Downloaded = $true
            } catch {
                Die "Download failed from mirror ${Mirror}: $_"
            }
        } else {
            try {
                Log "Downloading from GitHub..."
                Invoke-WebRequest -Uri $GhUrl -OutFile $LocalIpk -TimeoutSec 120 -UseBasicParsing
                $Downloaded = $true
                Ok "Downloaded from GitHub"
            } catch {
                Warn "Direct GitHub download failed, trying mirrors..."
                Remove-Item $LocalIpk -Force -ErrorAction SilentlyContinue
            }

            if (-not $Downloaded) {
                foreach ($Proxy in $GhProxies) {
                    try {
                        Log "Trying mirror: $Proxy"
                        Invoke-WebRequest -Uri "$Proxy/$GhUrl" -OutFile $LocalIpk -TimeoutSec 120 -UseBasicParsing
                        $Downloaded = $true
                        Ok "Downloaded via $Proxy"
                        break
                    } catch {
                        Warn "Mirror $Proxy failed"
                        Remove-Item $LocalIpk -Force -ErrorAction SilentlyContinue
                    }
                }
            }
        }

        if (-not $Downloaded) {
            Die "Failed to download IPK from all sources.`nTry: -Mirror https://ghproxy.com  or  -BaseUrl <your-cdn>"
        }
        $IpkBytes = (Get-Item $LocalIpk).Length
        # Validate the file is a gzip archive (magic bytes 1f 8b).
        # Some mirrors return an HTML error page with HTTP 200, which would
        # silently produce a "Malformed package file" error on the router.
        $magic = [System.IO.File]::ReadAllBytes($LocalIpk) | Select-Object -First 2
        if ($magic.Count -lt 2 -or $magic[0] -ne 0x1f -or $magic[1] -ne 0x8b) {
            Remove-Item $LocalIpk -Force -ErrorAction SilentlyContinue
            Die "Downloaded file is not a valid gzip archive (got HTML error page?).`nSource: $GhUrl"
        }
        Ok "Local IPK   : $LocalIpk ($IpkBytes bytes)"
        $CleanupLocalIpk = $true
    }

    # ── 4. Upload clashforgectl.sh ─────────────────────────────────────────────
    Log "── Step 4: Uploading clashforgectl.sh → ${Target}:${RemoteScript}"
    $ScpCtlArgs = $ScpBase + @($ShScript, "${Target}:${RemoteScript}")
    scp @ScpCtlArgs
    if ($LASTEXITCODE -ne 0) { Die "scp upload of clashforgectl.sh failed" }
    Ok "clashforgectl.sh uploaded"

    # ── 5. Push IPK to router ──────────────────────────────────────────────────
    $RemoteIpk  = "/tmp/$IpkName"
    Log "── Step 5: Pushing $IpkName → ${Target}:${RemoteIpk}"
    $ScpIpkArgs = $ScpBase + @($LocalIpk, "${Target}:${RemoteIpk}")
    scp @ScpIpkArgs
    if ($LASTEXITCODE -ne 0) { Die "scp upload of IPK failed" }
    Ok "IPK pushed to router"
    if ($CleanupLocalIpk) { Remove-Item $LocalIpk -Force -ErrorAction SilentlyContinue }

    # ── 6. Install on router via --local-ipk (no download needed on router) ──
    $PurgeFlag  = if ($Purge)  { " --purge"   } else { "" }
    $YesFlag    = if ($Yes)    { " --yes"     } else { "" }
    $DryRunFlag = if ($DryRun) { " --dry-run" } else { "" }
    $RemoteCmd  = "sh $RemoteScript upgrade --local-ipk $RemoteIpk$PurgeFlag$YesFlag$DryRunFlag"
    Log "── Step 6: Installing on router"
    Log "Remote  : $RemoteCmd"
    Log ""
    $SshInstallArgs = $SshBase + @($Target, $RemoteCmd)
    ssh @SshInstallArgs
    if ($LASTEXITCODE -ne 0) { Die "Remote install failed (exit $LASTEXITCODE)" }

    Sync-GeoDataToRouter

    Log ""
    Ok "Upgrade complete: $Tag  →  http://${Router}:7777"
    exit 0
}


# ── upload clashforgectl.sh to router ─────────────────────────────────────────
Log "Uploading clashforgectl.sh to router..."
$ScpUploadArgs = $ScpBase + @($ShScript, "${Target}:${RemoteScript}")
scp @ScpUploadArgs
if ($LASTEXITCODE -ne 0) {
    Die "scp upload failed (exit $LASTEXITCODE). Check router SSH access and disk space."
}
Ok "Upload complete."

# ── build remote command string ───────────────────────────────────────────────
# Values containing slashes or special characters are single-quoted for sh.
function ShQuote { param([string]$s) "'$($s -replace "'", "'\''")'" }

$RemoteCmd = "sh $RemoteScript $Action"

switch ($Action) {
    "upgrade" {
        if ($Version -ne "latest") { $RemoteCmd += " --version $(ShQuote $Version)" }
        if ($Mirror  -ne "")       { $RemoteCmd += " --mirror $(ShQuote $Mirror)" }
        if ($BaseUrl -ne "")       { $RemoteCmd += " --base-url $(ShQuote $BaseUrl)" }
        if ($Purge)                { $RemoteCmd += " --purge" }
    }
    "reset" {
        if ($Start) { $RemoteCmd += " --start" }
    }
    "uninstall" {
        if ($KeepConfig) { $RemoteCmd += " --keep-config" }
        if ($PurgeAll)   { $RemoteCmd += " --purge-all" }
    }
    "diag" {
        $RemoteCmd += " --output $(ShQuote $RemoteOutput)"
        if ($Redact) { $RemoteCmd += " --redact" }
        # Always capture to file; --stdout only needed when not fetching
        if (-not $Fetch) { $RemoteCmd += " --stdout" }
    }
    "openclash" {
        if ($Kill) { $RemoteCmd += " --kill" }
    }
}

if ($Yes)    { $RemoteCmd += " --yes" }
if ($DryRun) { $RemoteCmd += " --dry-run" }

Log "Remote  : $RemoteCmd"
Log ""

# ── execute on router ─────────────────────────────────────────────────────────
$SshExecArgs = $SshBase + @($Target, $RemoteCmd)
ssh @SshExecArgs
if ($LASTEXITCODE -ne 0) {
    Die "Remote command failed (exit $LASTEXITCODE)."
}

# ── diag: fetch report to local machine ───────────────────────────────────────
if ($Action -eq "diag" -and $Fetch) {
    # Build local output path: default to cf-diag_<router>_<timestamp>.txt
    if ($LocalPath -eq "") {
        $Timestamp = Get-Date -Format "yyyyMMdd_HHmmss"
        $LocalPath = "cf-diag_${Router}_${Timestamp}.txt"
    }

    Log ""
    Log "Fetching diagnostic report from router..."
    if ($Redact) {
        Warn "Fetching report with redaction applied on router side."
    } else {
        Warn "Report is UNREDACTED — handle with care (may contain subscription URLs, tokens)."
    }

    $ScpFetchArgs = $ScpBase + @("${Target}:${RemoteOutput}", $LocalPath)
    scp @ScpFetchArgs
    if ($LASTEXITCODE -ne 0) {
        Die "scp fetch failed (exit $LASTEXITCODE). Remote path: $RemoteOutput"
    }

    $FullLocalPath = (Resolve-Path $LocalPath).Path
    Ok ""
    Ok "Diagnostic report fetched:"
    Ok "  Remote : ${RemoteOutput}"
    Ok "  Local  : ${FullLocalPath}"
}

Ok ""
Ok "Done."
