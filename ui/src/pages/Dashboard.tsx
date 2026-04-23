import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import {
  Activity,
  CheckCircle2,
  Cpu,
  ExternalLink,
  HardDrive,
  Loader2,
  RefreshCw,
  X,
  Zap,
} from 'lucide-react'

import {
  getOverviewCore,
  getOverviewProbes,
  getOverviewResources,
  getClashforgeVersion,
  restartCore,
  getProxies,
  selectProxy,
  testLatency,
} from '../api/client'
import type { ClashforgeVersionData } from '../api/client'
import type {
  OverviewAccessCheck,
  OverviewCoreData,
  OverviewIPCheck,
  OverviewModule,
  OverviewProbeData,
  OverviewResourceData,
} from '../api/client'
import type { ProxyNode } from '../api/client'
import { useSSE } from '../hooks/useSSE'
import { useStore } from '../store'
import { formatBytes, formatGB, formatMB, formatPercent, formatUptime, latencyColor, latencyBarColor, latencyBarWidth } from '../utils/format'

type SectionKey = 'probes' | 'resources'

type ProxyMap = Record<string, ProxyNode>

const TYPE_ORDER = ['Selector', 'Fallback', 'URLTest', 'LoadBalance']
const IGNORED = ['DIRECT', 'REJECT', 'GLOBAL', 'PASS', 'REJECT-DROP', 'Compatible']

interface BrowserIPCheck {
  provider: string
  group: string
  ok: boolean
  ip?: string
  location?: string
  error?: string
}

interface BrowserAccessCheck {
  name: string
  group?: string
  url: string
  description: string
  via: string
  ok: boolean
  latency_ms?: number
  error?: string
}

interface BrowserProbeData {
  checked_at: string
  ip_checks: BrowserIPCheck[]
  access_checks: BrowserAccessCheck[]
}

interface ProbeSnapshot {
  router: OverviewProbeData | null
  browser: BrowserProbeData | null
}

interface ProbeHealth {
  hasData: boolean
  ipOK: boolean
  failedAccess: string[]
  healthy: boolean
}

function evaluateRouterProbeHealth(data: OverviewProbeData | null): ProbeHealth {
  if (!data) return { hasData: false, ipOK: false, failedAccess: [], healthy: false }
  const ipOK = data.ip_checks.some((item) => item.ok)
  const failedAccess = data.access_checks.filter((item) => !item.ok).map((item) => item.name)
  return {
    hasData: true,
    ipOK,
    failedAccess,
    healthy: ipOK && failedAccess.length === 0,
  }
}

function evaluateBrowserProbeHealth(data: BrowserProbeData | null): ProbeHealth {
  if (!data) return { hasData: false, ipOK: false, failedAccess: [], healthy: false }
  const ipOK = data.ip_checks.some((c) => c.ok)
  const failedAccess = data.access_checks.filter((item) => !item.ok).map((item) => item.name)
  return {
    hasData: true,
    ipOK,
    failedAccess,
    healthy: ipOK && failedAccess.length === 0,
  }
}

async function fetchWithTimeout(input: RequestInfo | URL, init: RequestInit, timeoutMs: number) {
  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort(), timeoutMs)
  try {
    return await fetch(input, { ...init, signal: controller.signal })
  } finally {
    window.clearTimeout(timer)
  }
}

async function runBrowserProbeData(targets: OverviewAccessCheck[]): Promise<BrowserProbeData> {
  const ipProviders: { provider: string; group: string; fetch: () => Promise<{ ip: string; location: string }> }[] = [
    {
      provider: 'UpaiYun',
      group: '国内',
      fetch: async () => {
        const res = await fetchWithTimeout('https://pubstatic.b0.upaiyun.com/?_upnode', { cache: 'no-store' }, 7000)
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const payload = await res.json() as { remote_addr?: string; remote_addr_location?: { country?: string; province?: string; city?: string; isp?: string } }
        if (!payload.remote_addr) throw new Error('empty')
        const loc = payload.remote_addr_location
        const location = loc ? [loc.country, loc.province, loc.city, loc.isp].filter(Boolean).join(' · ') : ''
        return { ip: payload.remote_addr, location }
      },
    },
    {
      provider: 'IP.SB',
      group: '国外',
      fetch: async () => {
        const res = await fetchWithTimeout('https://api.ip.sb/geoip', { cache: 'no-store' }, 7000)
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const payload = await res.json() as { ip?: string; country?: string; city?: string; region?: string; isp?: string }
        if (!payload.ip) throw new Error('empty')
        const location = [payload.city, payload.region, payload.country].filter(Boolean).join(' · ')
        return { ip: payload.ip, location }
      },
    },
    {
      provider: 'IPInfo',
      group: '国外',
      fetch: async () => {
        const res = await fetchWithTimeout('https://ipinfo.io/json', { cache: 'no-store' }, 7000)
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const payload = await res.json() as { ip?: string; city?: string; region?: string; country?: string; org?: string }
        if (!payload.ip) throw new Error('empty')
        const location = [payload.city, payload.region, payload.country, payload.org].filter(Boolean).join(' · ')
        return { ip: payload.ip, location }
      },
    },
  ]

  const ip_checks: BrowserIPCheck[] = await Promise.all(
    ipProviders.map(async (p) => {
      try {
        const { ip, location } = await p.fetch()
        return { provider: p.provider, group: p.group, ok: true, ip, location }
      } catch (error) {
        return { provider: p.provider, group: p.group, ok: false, error: error instanceof Error ? error.message : '获取失败' }
      }
    })
  )

  const accessChecks = await Promise.all(targets.map(async (target) => {
    const started = performance.now()
    try {
      await fetchWithTimeout(target.url, { method: 'GET', mode: 'no-cors', cache: 'no-store', redirect: 'follow' }, 8000)
      return {
        name: target.name,
        group: target.group,
        url: target.url,
        description: target.description,
        via: '由当前浏览器客户端直连发起检测',
        ok: true,
        latency_ms: Math.max(1, Math.round(performance.now() - started)),
      } satisfies BrowserAccessCheck
    } catch (error) {
      return {
        name: target.name,
        group: target.group,
        url: target.url,
        description: target.description,
        via: '由当前浏览器客户端直连发起检测',
        ok: false,
        error: error instanceof Error ? error.message : '浏览器访问失败',
      } satisfies BrowserAccessCheck
    }
  }))

  return {
    checked_at: new Date().toISOString(),
    ip_checks,
    access_checks: accessChecks,
  }
}

function Pill({ tone, label }: { tone: 'success' | 'warning' | 'danger' | 'muted'; label: string }) {
  const className = {
    success: 'border-success/25 bg-success/10 text-success',
    warning: 'border-warning/25 bg-warning/10 text-warning',
    danger: 'border-danger/25 bg-danger/10 text-danger',
    muted: 'border-white/10 bg-white/5 text-slate-300',
  }[tone]
  return <span className={`inline-flex items-center rounded-full border px-3 py-1 text-xs font-medium ${className}`}>{label}</span>
}

function ModuleRow({ module }: { module: OverviewModule }) {
  const managed = module.managed_by_clashforge
  const statusTone: 'success' | 'warning' | 'danger' | 'muted' = managed
    ? 'success'
    : module.status === 'conflict'
      ? 'warning'
      : module.status === 'inactive'
        ? 'danger'
        : 'muted'

  const statusLabel = managed
    ? '已接管'
    : module.status === 'conflict'
      ? '有占用'
      : module.status === 'inactive'
        ? '未运行'
        : '待接管'

  return (
    <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-4">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-semibold text-slate-100 truncate">{module.title}</p>
          <p className="text-xs text-muted mt-1 leading-5">{module.current_owner}</p>
        </div>
        <Pill tone={statusTone} label={statusLabel} />
      </div>
    </div>
  )
}

// ── Proxy switcher helpers ──────────────────────────────────────────────────

function LatencyBar({ ms }: { ms: number }) {
  if (!ms || ms <= 0) return <span className="text-muted text-xs">—</span>
  return (
    <div className="flex items-center gap-2">
      <div className="w-12 h-1.5 bg-surface-3 rounded-full overflow-hidden">
        <div className={`h-full rounded-full ${latencyBarColor(ms)}`} style={{ width: latencyBarWidth(ms) }} />
      </div>
      <span className={`text-xs tabular-nums font-mono ${latencyColor(ms)}`}>{ms}ms</span>
    </div>
  )
}

function ProxyGroup({ name, group, allProxies, onSelect }: {
  name: string; group: ProxyNode; allProxies: ProxyMap
  onSelect: (group: string, proxy: string) => void
}) {
  const [open, setOpen] = useState(true)
  const members = (group.all ?? []).filter(n => !IGNORED.includes(n))

  return (
    <div className="rounded-2xl border border-white/8 bg-black/10 overflow-hidden">
      <button
        onClick={() => setOpen(o => !o)}
        className="w-full flex items-center justify-between px-4 py-3.5 hover:bg-white/5 transition-all"
      >
        <div className="flex items-center gap-3">
          <span className="text-sm font-semibold text-white">{name}</span>
          <span className="text-[10px] px-1.5 py-0.5 rounded-full border border-white/15 text-muted">{group.type}</span>
          {group.now && (
            <span className="text-xs text-muted">
              → <span className="text-slate-300 font-medium">{group.now}</span>
            </span>
          )}
        </div>
        <span className="text-muted text-xs">{open ? '▲' : '▼'}</span>
      </button>
      {open && (
        <div className="px-3 pb-3 space-y-0.5 max-h-64 overflow-y-auto border-t border-white/5">
          {members.map(m => {
            const node = allProxies[m]
            if (!node) return <div key={m} className="px-4 py-2 text-sm text-muted">{m}</div>
            const isSelected = group.now === m
            const lastDelay = node.history?.at(-1)?.delay ?? -1
            return (
              <button
                key={m}
                onClick={() => { if (group.type === 'Selector') onSelect(name, m) }}
                className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-xl transition-all text-left ${isSelected ? 'bg-brand/15 ring-1 ring-brand/40' : 'hover:bg-white/5'}`}
              >
                <div className={`w-2 h-2 rounded-full flex-shrink-0 ${isSelected ? 'bg-brand' : 'bg-surface-3'}`} />
                <div className="flex-1 min-w-0">
                  <p className={`text-sm font-medium truncate ${isSelected ? 'text-brand' : 'text-slate-200'}`}>{node.name}</p>
                  <p className="text-xs text-muted">{node.type}</p>
                </div>
                <LatencyBar ms={lastDelay} />
              </button>
            )
          })}
          {group.type === 'Selector' && ['DIRECT', 'REJECT'].filter(n => group.all?.includes(n)).map(n => {
            const isSelected = group.now === n
            return (
              <button
                key={n}
                onClick={() => onSelect(name, n)}
                className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-xl transition-all text-left ${isSelected ? 'bg-brand/15 ring-1 ring-brand/40' : 'hover:bg-white/5'}`}
              >
                <div className={`w-2 h-2 rounded-full flex-shrink-0 ${isSelected ? 'bg-brand' : 'bg-surface-3'}`} />
                <span className={`text-sm font-medium ${isSelected ? 'text-brand' : 'text-slate-200'}`}>{n}</span>
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}

function CoreStateBadge({ state }: { state: string }) {
  const tone = {
    running: 'success',
    stopped: 'danger',
    error: 'danger',
    starting: 'warning',
    stopping: 'warning',
    querying: 'warning',
    unknown: 'warning',
  }[state] as 'success' | 'warning' | 'danger' | undefined

  const label = {
    running: '运行中',
    stopped: '已停止',
    error: '异常',
    starting: '启动中',
    stopping: '停止中',
    querying: '查询中...',
    unknown: '查询中...',
  }[state] ?? state

  if (state === 'querying' || state === 'unknown') {
    return (
      <span className="inline-flex items-center gap-2 rounded-full border border-warning/25 bg-warning/10 px-3 py-1 text-xs font-medium text-warning">
        <Loader2 size={12} className="animate-spin" />
        {label}
      </span>
    )
  }

  return <Pill tone={tone ?? 'muted'} label={label} />
}

function MetricTile({ icon, label, value, hint }: { icon: ReactNode; label: string; value: string; hint?: string }) {
  return (
    <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-4">
      <div className="flex items-center gap-2 text-slate-300">
        {icon}
        <p className="text-xs uppercase tracking-[0.16em] text-muted">{label}</p>
      </div>
      <p className="text-lg font-semibold text-white mt-3">{value}</p>
      {hint ? <p className="text-xs text-muted mt-2">{hint}</p> : null}
    </div>
  )
}

function StatusDot({ online }: { online: boolean }) {
  return (
    <span
      className={`inline-flex h-2 w-2 rounded-full ${online ? 'bg-success animate-pulse' : 'bg-danger'}`}
      aria-hidden
    />
  )
}

function groupBy<T extends { group?: string }>(items: T[]): [string, T[]][] {
  const map = new Map<string, T[]>()
  for (const item of items) {
    const g = item.group ?? '其他'
    if (!map.has(g)) map.set(g, [])
    map.get(g)!.push(item)
  }
  return Array.from(map.entries())
}

function GroupTag({ label }: { label: string }) {
  const cls = label === '国内'
    ? 'bg-sky-500/10 border-sky-500/25 text-sky-400'
    : label === '国外'
      ? 'bg-violet-500/10 border-violet-500/25 text-violet-400'
      : label === 'AI'
        ? 'bg-amber-500/10 border-amber-500/25 text-amber-400'
        : 'bg-white/5 border-white/15 text-slate-400'
  return (
    <span className={`inline-flex items-center rounded-full border px-2.5 py-0.5 text-[10px] font-semibold tracking-wider ${cls}`}>
      {label}
    </span>
  )
}

function IPCard({ item }: { item: OverviewIPCheck }) {
  return (
    <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-4">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm font-semibold text-slate-100">{item.provider}</p>
        <Pill tone={item.ok ? 'success' : 'danger'} label={item.ok ? '已解析' : '失败'} />
      </div>
      {item.ok ? (
        <>
          <p className="text-lg font-semibold text-white mt-3">{item.ip || '--'}</p>
          <p className="text-xs text-muted mt-2 leading-5">{item.location || '未返回位置信息'}</p>
        </>
      ) : (
        <p className="text-xs text-danger mt-3 leading-5">{item.error || '无法获取出口 IP'}</p>
      )}
    </div>
  )
}

function AccessCard({ item }: { item: OverviewAccessCheck }) {
  return (
    <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <p className="text-sm font-semibold text-slate-100">{item.name}</p>
          <p className="text-xs text-muted mt-1">{item.description}</p>
        </div>
        <Pill tone={item.ok ? 'success' : 'danger'} label={item.ok ? '正常' : '失败'} />
      </div>
      <p className="text-sm text-slate-200 mt-3">{item.ok ? `${item.latency_ms ?? 0} ms` : (item.error || '请求失败')}</p>
      <p className="text-xs text-muted mt-2 break-all">{item.url}</p>
    </div>
  )
}

function ProcessCard({ name, pid, cpu, memory, uptime, running, command }: {
  name: string
  pid: number
  cpu: number
  memory: number
  uptime: number
  running: boolean
  command?: string
}) {
  return (
    <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-4">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <StatusDot online={running} />
          <p className="text-sm font-semibold text-slate-100">{name}</p>
        </div>
        <p className="text-xs text-muted">PID {pid || '--'}</p>
      </div>
      <div className="grid grid-cols-2 gap-3 mt-3 text-sm">
        <div>
          <p className="text-[11px] uppercase tracking-[0.16em] text-muted">CPU</p>
          <p className="text-slate-200 mt-1">{formatPercent(cpu)}</p>
        </div>
        <div>
          <p className="text-[11px] uppercase tracking-[0.16em] text-muted">内存</p>
          <p className="text-slate-200 mt-1">{formatMB(memory)}</p>
        </div>
        <div>
          <p className="text-[11px] uppercase tracking-[0.16em] text-muted">运行时长</p>
          <p className="text-slate-200 mt-1">{formatUptime(uptime)}</p>
        </div>
        <div>
          <p className="text-[11px] uppercase tracking-[0.16em] text-muted">状态</p>
          <p className="text-slate-200 mt-1">{running ? '在线' : '离线'}</p>
        </div>
      </div>
      {command ? <p className="text-xs text-muted mt-3 break-all">{command}</p> : null}
    </div>
  )
}

// ── Probe pane ─────────────────────────────────────────────────────────────

type PaneIPCheck = { provider: string; group?: string; ok: boolean; ip?: string; location?: string; error?: string }
type PaneAccessCheck = { name: string; group?: string; url: string; description: string; via: string; ok: boolean; latency_ms?: number; error?: string }

function ProbePane({ title, subtitle, health, ipChecks, accessChecks, loading }: {
  title: string
  subtitle: string
  health: ProbeHealth
  ipChecks: PaneIPCheck[]
  accessChecks: PaneAccessCheck[]
  loading?: boolean
}) {
  const ipGroups = groupBy(ipChecks)
  const accessGroups = groupBy(accessChecks)
  const hasContent = ipChecks.length > 0 || accessChecks.length > 0
  return (
    <div className="rounded-2xl border border-white/10 bg-black/15 overflow-hidden">
      {/* Panel header */}
      <div className="flex items-center justify-between gap-3 px-4 py-3.5 border-b border-white/8 bg-white/[0.02]">
        <div>
          <div className="flex items-center gap-2">
            <p className="text-sm font-semibold text-white">{title}</p>
            {loading && <Loader2 size={12} className="animate-spin text-muted" />}
          </div>
          <p className="text-xs text-muted mt-0.5">{subtitle}</p>
        </div>
        {health.hasData
          ? <Pill tone={health.healthy ? 'success' : 'warning'} label={health.healthy ? '通过' : '待修复'} />
          : <Pill tone="muted" label={loading ? '检测中' : '等待'} />
        }
      </div>

      {/* Body */}
      <div className="px-4 py-4 space-y-5">
        {!hasContent ? (
          <p className="text-xs text-muted py-2 text-center">{loading ? '正在检测...' : '暂无数据'}</p>
        ) : (
          <>
            {/* 出口 IP sub-section */}
            {ipGroups.length > 0 && (
              <div className="space-y-3">
                <div className="flex items-center gap-2">
                  <span className="text-[10px] font-semibold uppercase tracking-[0.2em] text-muted whitespace-nowrap">出口 IP</span>
                  <div className="flex-1 h-px bg-white/8" />
                </div>
                {ipGroups.map(([group, items]) => (
                  <div key={group} className="space-y-2">
                    <GroupTag label={group} />
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                      {items.map((item) => <IPCard key={item.provider} item={item as OverviewIPCheck} />)}
                    </div>
                  </div>
                ))}
              </div>
            )}

            {/* 访问检查 sub-section */}
            {accessGroups.length > 0 && (
              <div className="space-y-3">
                <div className="flex items-center gap-2">
                  <span className="text-[10px] font-semibold uppercase tracking-[0.2em] text-muted whitespace-nowrap">访问检查</span>
                  <div className="flex-1 h-px bg-white/8" />
                </div>
                {accessGroups.map(([group, items]) => (
                  <div key={group} className="space-y-2">
                    <GroupTag label={group} />
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                      {items.map((item) => <AccessCard key={item.name} item={item as OverviewAccessCheck} />)}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}

const SKIP_VERSION_KEY = 'cf_skip_version'

function UpdateBanner({ data, onSkip }: { data: ClashforgeVersionData; onSkip: () => void }) {
  return (
    <div className="rounded-2xl border border-warning/30 bg-warning/8 px-5 py-4 flex items-center justify-between gap-4 flex-wrap">
      <div className="flex items-center gap-3 min-w-0">
        <span className="text-warning text-lg">🎉</span>
        <div className="min-w-0">
          <p className="text-sm font-semibold text-warning">发现新版本 {data.latest}</p>
          <p className="text-xs text-muted mt-0.5">当前版本 {data.current}，前往 GitHub Releases 下载安装包</p>
        </div>
      </div>
      <div className="flex items-center gap-2 shrink-0">
        <a
          href={data.release_url}
          target="_blank"
          rel="noopener noreferrer"
          className="btn-ghost flex items-center gap-1.5 text-warning border-warning/30 hover:bg-warning/10"
        >
          <ExternalLink size={13} />
          查看发布页
        </a>
        <button
          className="btn-ghost flex items-center gap-1.5 text-muted"
          onClick={onSkip}
          title="跳过此版本，不再提醒"
        >
          <X size={13} />
          跳过
        </button>
      </div>
    </div>
  )
}

export function Dashboard() {
  const { currentUp, currentDown, connCount, coreState, setCoreState, pushTraffic, setConnCount } = useStore()

  const [coreData, setCoreData] = useState<OverviewCoreData | null>(null)
  const [probeData, setProbeData] = useState<OverviewProbeData | null>(null)
  const [browserProbeData, setBrowserProbeData] = useState<BrowserProbeData | null>(null)
  const [resourceData, setResourceData] = useState<OverviewResourceData | null>(null)

  const [queryingCore, setQueryingCore] = useState(true)
  const [section, setSection] = useState<SectionKey | null>(null)
  const [loadingSection, setLoadingSection] = useState<SectionKey | null>(null)
  const [loadingBrowserProbe, setLoadingBrowserProbe] = useState(false)
  const [loadingAction, setLoadingAction] = useState<string | null>(null)

  const [versionData, setVersionData] = useState<ClashforgeVersionData | null>(null)
  const [showBanner, setShowBanner] = useState(false)

  // proxy switcher state
  const [proxies, setProxies] = useState<ProxyMap>({})
  const [testingLatency, setTestingLatency] = useState(false)

  useSSE({
    onCoreState: (data) => setCoreState(data.state, data.pid),
    onTraffic: (data) => pushTraffic(data),
    onConnCount: (data) => setConnCount(data.total),
  })

  const refreshCore = useCallback(async (silent = false) => {
    if (!silent) setQueryingCore(true)
    const next = await getOverviewCore().catch(() => null)
    if (next) {
      setCoreData(next)
      setCoreState(next.core.state, next.core.pid)
      if (typeof next.core.active_connections === 'number') {
        setConnCount(next.core.active_connections)
      }
    }
    if (!silent) setQueryingCore(false)
  }, [setConnCount, setCoreState])

  const refreshProxies = useCallback(async () => {
    const data = await getProxies().catch(() => null)
    if (data) setProxies(data.proxies ?? {})
  }, [])

  const refreshProbes = async (): Promise<ProbeSnapshot> => {
    setLoadingSection('probes')
    try {
      const next = await getOverviewProbes().catch(() => null)
      if (!next) {
        setProbeData(null)
        setBrowserProbeData(null)
        return { router: null, browser: null }
      }
      setProbeData(next)
      setLoadingBrowserProbe(true)
      const browser = await runBrowserProbeData(next.access_checks).catch(() => null)
      setBrowserProbeData(browser)
      return { router: next, browser }
    } finally {
      setLoadingBrowserProbe(false)
      setLoadingSection(null)
    }
  }

  const refreshResources = async () => {
    setLoadingSection('resources')
    const next = await getOverviewResources().catch(() => null)
    if (next) setResourceData(next)
    setLoadingSection(null)
  }

  useEffect(() => {
    const bootstrap = setTimeout(() => { void refreshCore(false) }, 0)
    const timer = setInterval(() => { void refreshCore(true) }, 8000)
    return () => { clearTimeout(bootstrap); clearInterval(timer) }
  }, [refreshCore])

  // Only fetch proxies when we know the core is running — avoids 502 spam when Mihomo is stopped
  useEffect(() => {
    if (coreData?.core.state !== 'running') return
    const t = setTimeout(() => { void refreshProxies() }, 0)
    return () => clearTimeout(t)
  }, [coreData, refreshProxies])

  // Version check once on mount
  useEffect(() => {
    const t = setTimeout(async () => {
      const data = await getClashforgeVersion().catch(() => null)
      if (!data?.has_update) return
      const skipped = localStorage.getItem(SKIP_VERSION_KEY)
      if (skipped === data.latest) return
      setVersionData(data)
      setShowBanner(true)
    }, 0)
    return () => clearTimeout(t)
  }, [])

  const openSection = async (target: SectionKey) => {
    setSection(target)
    if (target === 'probes' && !probeData) { await refreshProbes(); return }
    if (target === 'resources' && !resourceData) { await refreshResources() }
  }

  const handleSelectProxy = async (group: string, proxy: string) => {
    await selectProxy(group, proxy).catch(() => null)
    await refreshProxies()
    // auto-run probes to verify the switch
    setSection('probes')
    await refreshProbes()
  }

  const handleTestLatency = async () => {
    setTestingLatency(true)
    const nodeNames = Object.entries(proxies)
      .filter(([, v]) => !TYPE_ORDER.includes(v.type) && !IGNORED.includes(v.name))
      .map(([k]) => k)
    await testLatency(nodeNames).catch(() => null)
    await refreshProxies()
    setTestingLatency(false)
  }

  const effectiveState = queryingCore && !coreData ? 'querying' : (coreData?.core.state || coreState || 'unknown')
  const coreRunning = effectiveState === 'running'

  const routerProbeHealth = useMemo(() => evaluateRouterProbeHealth(probeData), [probeData])
  const browserProbeHealth = useMemo(() => evaluateBrowserProbeHealth(browserProbeData), [browserProbeData])

  const visibleModules = useMemo(() => {
    const preferredOrder = ['proxy_core', 'transparent_proxy', 'nft_firewall', 'dns_entry', 'dns_resolver']
    const byID = new Map((coreData?.modules ?? []).map((item) => [item.id, item]))
    const ordered: OverviewModule[] = []
    for (const id of preferredOrder) {
      const found = byID.get(id)
      if (found) ordered.push(found)
    }
    return ordered
  }, [coreData])

  const proxyGroups = useMemo(() =>
    Object.entries(proxies)
      .filter(([, v]) => TYPE_ORDER.includes(v.type) && !IGNORED.includes(v.name) && v.name !== 'GLOBAL')
      .sort(([, a], [, b]) => TYPE_ORDER.indexOf(a.type) - TYPE_ORDER.indexOf(b.type)),
    [proxies]
  )

  return (
    <div className="p-6 space-y-6 max-w-7xl mx-auto">

      {/* ── Update banner ── */}
      {showBanner && versionData && (
        <UpdateBanner
          data={versionData}
          onSkip={() => {
            localStorage.setItem(SKIP_VERSION_KEY, versionData.latest)
            setShowBanner(false)
          }}
        />
      )}

      {/* ── Core status + restart ── */}
      <div className="card px-6 py-5">
        <div className="flex items-center justify-between gap-4 flex-wrap">
          <div className="flex items-center gap-3">
            <CoreStateBadge state={effectiveState} />
            <div>
              <p className="text-sm font-semibold text-white">{coreRunning ? '内核运行中' : '内核未运行'}</p>
              <p className="text-xs text-muted mt-0.5">
                {coreData ? `PID ${coreData.core.pid} · 运行 ${formatUptime(coreData.core.uptime)}` : '等待状态'}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <button
              className="btn-ghost flex items-center gap-2"
              onClick={() => { void refreshCore(false) }}
              disabled={queryingCore}
            >
              <RefreshCw size={14} className={queryingCore ? 'animate-spin' : ''} />
              刷新
            </button>
            <button
              className="btn-ghost flex items-center gap-2"
              onClick={async () => {
                setLoadingAction('restart')
                await restartCore().catch(() => null)
                setLoadingAction(null)
                await refreshCore(true)
              }}
              disabled={!!loadingAction}
            >
              {loadingAction === 'restart' ? <Loader2 size={14} className="animate-spin" /> : <RefreshCw size={14} />}
              重启
            </button>
          </div>
        </div>

        <div className="grid grid-cols-2 xl:grid-cols-4 gap-3 mt-5">
          <MetricTile icon={<Activity size={16} />} label="上传速率" value={formatBytes(currentUp)} hint="实时上行" />
          <MetricTile icon={<Activity size={16} />} label="下载速率" value={formatBytes(currentDown)} hint="实时下行" />
          <MetricTile icon={<CheckCircle2 size={16} />} label="活跃连接" value={`${connCount}`} hint="当前连接数" />
          <MetricTile icon={<Cpu size={16} />} label="核心运行时长" value={coreData ? formatUptime(coreData.core.uptime) : '--'} hint={`PID ${coreData?.core.pid || '--'}`} />
        </div>
      </div>

      {/* ── Modules status ── */}
      <div className="card px-5 py-5">
        <div className="flex items-center justify-between gap-4 flex-wrap">
          <div>
            <p className="text-[11px] uppercase tracking-[0.24em] text-muted">Modules</p>
            <h2 className="text-base font-semibold text-white mt-1">子模块状态</h2>
          </div>
          <p className="text-xs text-muted">{coreData?.checked_at ? `更新于 ${new Date(coreData.checked_at).toLocaleTimeString()}` : '等待查询'}</p>
        </div>
        <div className="grid grid-cols-1 xl:grid-cols-3 gap-3 mt-4">
          {visibleModules.map((module) => (
            <ModuleRow key={module.id} module={module} />
          ))}
          {!visibleModules.length && (
            <div className="col-span-3 rounded-2xl border border-white/8 bg-black/10 px-4 py-4 text-sm text-muted">
              正在查询模块状态...
            </div>
          )}
        </div>
      </div>

      {/* ── Proxy switcher ── */}
      <div className="card px-5 py-5">
        <div className="flex items-center justify-between gap-4 flex-wrap">
          <div>
            <p className="text-[11px] uppercase tracking-[0.24em] text-muted">Proxies</p>
            <h2 className="text-base font-semibold text-white mt-1">节点切换</h2>
          </div>
          <div className="flex items-center gap-2">
            <button className="btn-ghost flex items-center gap-2" onClick={refreshProxies}>
              <RefreshCw size={14} /> 刷新
            </button>
            <button className="btn-ghost flex items-center gap-2" onClick={handleTestLatency} disabled={testingLatency}>
              <Zap size={14} className={testingLatency ? 'animate-pulse' : ''} />
              {testingLatency ? '测试中…' : '测速'}
            </button>
          </div>
        </div>

        {proxyGroups.length === 0 ? (
          <div className="rounded-2xl border border-dashed border-white/15 bg-black/10 px-4 py-5 mt-4 text-sm text-muted">
            {coreRunning ? '未找到代理组，请先添加订阅并更新节点。' : '内核未运行，无法获取节点列表。'}
          </div>
        ) : (
          <div className="space-y-3 mt-4">
            {proxyGroups.map(([name, group]) => (
              <ProxyGroup
                key={name}
                name={name}
                group={group}
                allProxies={proxies}
                onSelect={handleSelectProxy}
              />
            ))}
          </div>
        )}
      </div>

      {/* ── On-demand probes + resources ── */}
      <div className="card px-5 py-5">
        <div className="flex items-end justify-between gap-4 flex-wrap">
          <div>
            <p className="text-[11px] uppercase tracking-[0.24em] text-muted">Connectivity</p>
            <h2 className="text-base font-semibold text-white mt-1">出口 IP / 访问检查</h2>
          </div>
          <div className="flex items-center gap-2">
            <button
              className={`btn-ghost ${section === 'probes' ? 'border-brand/40 text-white' : ''}`}
              onClick={() => { void openSection('probes') }}
            >
              IP 检查
            </button>
            <button
              className={`btn-ghost ${section === 'resources' ? 'border-brand/40 text-white' : ''}`}
              onClick={() => { void openSection('resources') }}
            >
              资源占用
            </button>
          </div>
        </div>

        {!section ? (
          <div className="rounded-2xl border border-dashed border-white/15 bg-black/10 px-4 py-5 mt-4 text-sm text-muted">
            切换节点后会自动执行 IP 检查，也可手动点击上方按钮。
          </div>
        ) : null}

        {section === 'probes' ? (
          <div className="mt-4">
            <div className="flex items-center justify-between gap-3 mb-4">
              <p className="text-sm text-muted">路由器侧经代理转发，浏览器侧由客户端直连。对比两侧可快速判断代理出口是否工作正常。</p>
              <button className="btn-ghost flex items-center gap-2" onClick={() => { void refreshProbes() }} disabled={loadingSection === 'probes'}>
                <RefreshCw size={14} className={loadingSection === 'probes' ? 'animate-spin' : ''} />
                重新检测
              </button>
            </div>

            {!probeData ? (
              <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-5 text-sm text-muted">
                {loadingSection === 'probes' ? '正在进行联网检测…' : '点击"重新检测"开始。'}
              </div>
            ) : (
              <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
                <ProbePane
                  title="路由器侧"
                  subtitle="经 ClashForge mixed 端口转发"
                  health={routerProbeHealth}
                  ipChecks={probeData.ip_checks}
                  accessChecks={probeData.access_checks}
                  loading={loadingSection === 'probes'}
                />
                <ProbePane
                  title="浏览器侧"
                  subtitle="由当前浏览器客户端直连，不经过代理"
                  health={browserProbeHealth}
                  ipChecks={browserProbeData?.ip_checks ?? []}
                  accessChecks={browserProbeData?.access_checks ?? []}
                  loading={loadingBrowserProbe}
                />
              </div>
            )}
          </div>
        ) : null}

        {section === 'resources' ? (
          <div className="mt-4 space-y-4">
            <div className="flex items-center justify-between gap-3">
              <p className="text-sm text-muted">系统资源与 ClashForge 占用。</p>
              <button className="btn-ghost flex items-center gap-2" onClick={() => { void refreshResources() }} disabled={loadingSection === 'resources'}>
                <RefreshCw size={14} className={loadingSection === 'resources' ? 'animate-spin' : ''} />
                刷新资源
              </button>
            </div>
            {!resourceData ? (
              <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-5 text-sm text-muted">
                {loadingSection === 'resources' ? '正在采样…' : '点击"刷新资源"开始加载。'}
              </div>
            ) : (
              <>
                <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                  <MetricTile icon={<Cpu size={16} />} label="系统 CPU" value={formatPercent(resourceData.resources.system.cpu_percent)} />
                  <MetricTile icon={<Activity size={16} />} label="系统内存" value={`${formatMB(resourceData.resources.system.memory_used_mb)} / ${formatMB(resourceData.resources.system.memory_total_mb)}`} hint={`已用 ${formatPercent(resourceData.resources.system.memory_percent)}`} />
                  <MetricTile icon={<HardDrive size={16} />} label="系统磁盘" value={`${formatGB(resourceData.resources.system.disk_used_gb)} / ${formatGB(resourceData.resources.system.disk_total_gb)}`} hint={`已用 ${formatPercent(resourceData.resources.system.disk_percent)}`} />
                </div>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                  {resourceData.resources.processes.map((item) => (
                    <ProcessCard
                      key={item.id}
                      name={item.name}
                      pid={item.pid}
                      cpu={item.cpu_percent}
                      memory={item.memory_rss_mb}
                      uptime={item.uptime}
                      running={item.running}
                      command={item.command}
                    />
                  ))}
                </div>
                <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-4">
                  <p className="text-sm font-semibold text-slate-100">ClashForge 磁盘占用</p>
                  <div className="grid grid-cols-2 md:grid-cols-5 gap-3 mt-3 text-sm">
                    {[
                      { label: '运行目录', val: formatMB(resourceData.resources.app.runtime_mb) },
                      { label: '数据目录', val: formatMB(resourceData.resources.app.data_mb) },
                      { label: '程序文件', val: formatMB(resourceData.resources.app.binary_mb) },
                      { label: '规则文件', val: formatMB(resourceData.resources.app.rules_mb) },
                      { label: '总占用',   val: formatMB(resourceData.resources.app.total_mb) },
                    ].map(({ label, val }) => (
                      <div key={label}>
                        <p className="text-[11px] uppercase tracking-[0.16em] text-muted">{label}</p>
                        <p className="text-slate-200 mt-1">{val}</p>
                      </div>
                    ))}
                  </div>
                  {!!resourceData.resources.app.rule_assets?.length && (
                    <div className="mt-4 border-t border-white/10 pt-3 space-y-2">
                      <p className="text-xs uppercase tracking-[0.16em] text-muted">规则文件明细</p>
                      {resourceData.resources.app.rule_assets.map((asset) => (
                        <div key={`${asset.name}-${asset.path}`} className="flex items-center justify-between gap-3 text-xs">
                          <span className="text-slate-200">{asset.name}</span>
                          <span className="text-muted">{formatMB(asset.size_mb)} · {asset.path}</span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              </>
            )}
          </div>
        ) : null}
      </div>
    </div>
  )
}
