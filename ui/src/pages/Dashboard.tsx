import { useCallback, useEffect, useId, useMemo, useRef, useState, type ReactNode } from 'react'
import {
  AlertTriangle,
  Activity,
  Clock3,
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
  getHealthIncidents,
  getHealthSummary,
  getProxies,
  selectProxy,
  testLatency,
  probeDomain,
  reportHealthBrowser,
} from '../api/client'
import type { ClashforgeVersionData, DomainProbeResult } from '../api/client'
import type {
  HealthIncident,
  HealthSummaryData,
  OverviewAccessCheck,
  OverviewCoreData,
  OverviewIPCheck,
  OverviewModule,
  OverviewProbeData,
  OverviewResourceData,
} from '../api/client'
import type { ProxyNode } from '../api/client'
import { PageHeader, SectionCard } from '../components/ui'
import { BROWSER_IP_PROVIDERS, DEFAULT_DOMAIN_PROBE_INPUT, DOMAIN_PROBE_PRESETS } from '../constants/probeTargets'
import { useSSE } from '../hooks/useSSE'
import { useStore } from '../store'
import { formatBytes, formatGB, formatMB, formatPercent, formatUptime, latencyColor, latencyBarColor, latencyBarWidth } from '../utils/format'

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
  stage?: string // 'timeout' | 'connect' | 'dns'
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

type TimelineState = 'healthy' | 'degraded' | 'unhealthy' | 'unknown'

interface HealthTimelineSegment {
  state: TimelineState
  startMs: number
  endMs: number
  reason?: string
  incidentId?: string
}

const TIMELINE_WINDOW_HOURS = 24

function parseISOToMS(raw?: string): number | null {
  if (!raw) return null
  const ts = Date.parse(raw)
  return Number.isFinite(ts) ? ts : null
}

function normalizeTimelineState(raw?: string): TimelineState {
  const s = (raw || '').toLowerCase()
  if (s === 'healthy' || s === 'degraded' || s === 'unhealthy' || s === 'unknown') return s
  return 'unknown'
}

function stateLabel(state: TimelineState): string {
  switch (state) {
    case 'healthy': return '健康'
    case 'degraded': return '部分异常'
    case 'unhealthy': return '严重异常'
    case 'unknown': return '未知'
  }
}

function stateBarClass(state: TimelineState): string {
  switch (state) {
    case 'healthy': return 'bg-success/80'
    case 'degraded': return 'bg-warning/85'
    case 'unhealthy': return 'bg-danger/85'
    case 'unknown': return 'bg-white/25'
  }
}

function stateBadgeClass(state: TimelineState): string {
  switch (state) {
    case 'healthy': return 'border-success/25 bg-success/10 text-success'
    case 'degraded': return 'border-warning/25 bg-warning/10 text-warning'
    case 'unhealthy': return 'border-danger/25 bg-danger/10 text-danger'
    case 'unknown': return 'border-white/15 bg-white/5 text-slate-300'
  }
}

function formatTimelinePoint(tsMs: number): string {
  return new Date(tsMs).toLocaleString()
}

function formatTimelineDuration(startMs: number, endMs: number): string {
  const deltaSec = Math.max(0, Math.floor((endMs - startMs) / 1000))
  if (deltaSec < 60) return `${deltaSec}s`
  if (deltaSec < 3600) return `${Math.floor(deltaSec / 60)}m`
  const h = Math.floor(deltaSec / 3600)
  const m = Math.floor((deltaSec % 3600) / 60)
  return `${h}h ${m}m`
}

function buildTimelineSegments(
  summary: HealthSummaryData | null,
  incidents: HealthIncident[],
  nowMs: number,
  windowHours: number,
): HealthTimelineSegment[] {
  const windowStart = nowMs - windowHours * 3600 * 1000
  const normalized = [...incidents]
    .map((item) => {
      const opened = parseISOToMS(item.opened_at)
      if (opened === null) return null
      const resolved = parseISOToMS(item.resolved_at) ?? nowMs
      return {
        id: item.id,
        state: normalizeTimelineState(item.state),
        reason: item.reason || '',
        start: opened,
        end: resolved,
      }
    })
    .filter((item): item is { id: string; state: TimelineState; reason: string; start: number; end: number } => !!item)
    .filter((item) => item.end > windowStart && item.start < nowMs)
    .sort((a, b) => a.start - b.start)

  const segments: HealthTimelineSegment[] = []
  let cursor = windowStart

  for (const item of normalized) {
    const segStart = Math.max(item.start, windowStart)
    const segEnd = Math.min(item.end, nowMs)
    if (segEnd <= segStart) continue
    if (segStart > cursor) {
      segments.push({ state: 'healthy', startMs: cursor, endMs: segStart })
    }
    const adjustedStart = Math.max(segStart, cursor)
    if (segEnd > adjustedStart) {
      segments.push({
        state: item.state,
        startMs: adjustedStart,
        endMs: segEnd,
        reason: item.reason,
        incidentId: item.id,
      })
      cursor = segEnd
    }
  }

  if (cursor < nowMs) {
    segments.push({ state: 'healthy', startMs: cursor, endMs: nowMs })
  }

  if (summary) {
    const currentState = normalizeTimelineState(summary.current.state)
    if (currentState === 'unknown') {
      const sinceMs = parseISOToMS(summary.current.since) ?? windowStart
      const unknownStart = Math.max(windowStart, sinceMs)
      if (unknownStart < nowMs) {
        const kept: HealthTimelineSegment[] = []
        for (const seg of segments) {
          if (seg.endMs <= unknownStart) {
            kept.push(seg)
            continue
          }
          if (seg.startMs < unknownStart) {
            kept.push({ ...seg, endMs: unknownStart })
          }
        }
        kept.push({ state: 'unknown', startMs: unknownStart, endMs: nowMs, reason: summary.current.last_reason || '' })
        return kept.filter((seg) => seg.endMs > seg.startMs)
      }
    }
  }

  return segments.filter((seg) => seg.endMs > seg.startMs)
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

function categorizeBrowserFetchError(error: unknown, elapsedMs: number): { stage: string; message: string } {
  if (error instanceof Error) {
    const name = error.name
    const msg = error.message.toLowerCase()
    if (name === 'AbortError' || msg.includes('abort')) {
      return {
        stage: 'timeout',
        message: `请求超时（超过 ${Math.round(elapsedMs / 1000)}s 无响应）— 可能是 DNS 解析失败或代理服务未响应`,
      }
    }
    if (msg.includes('failed to fetch') || msg.includes('load failed') || msg.includes('networkerror') || msg.includes('network error')) {
      return { stage: 'connect', message: 'DNS 解析失败或网络连接被拒绝，请确认 DNS 入口和透明代理已接管' }
    }
    return { stage: 'connect', message: error.message }
  }
  return { stage: 'connect', message: '访问失败' }
}

async function runBrowserProbeData(targets: OverviewAccessCheck[]): Promise<BrowserProbeData> {
  const ip_checks: BrowserIPCheck[] = await Promise.all(
    BROWSER_IP_PROVIDERS.map(async (p) => {
      try {
        const res = await fetchWithTimeout(p.url, { cache: 'no-store' }, 7000)
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        let ip = ''
        let location = ''
        if (p.parse === 'upaiyun') {
          const payload = await res.json() as { remote_addr?: string; remote_addr_location?: { country?: string; province?: string; city?: string; isp?: string } }
          ip = payload.remote_addr || ''
          const loc = payload.remote_addr_location
          location = loc ? [loc.country, loc.province, loc.city, loc.isp].filter(Boolean).join(' · ') : ''
        } else if (p.parse === 'ipsb') {
          const payload = await res.json() as { ip?: string; country?: string; city?: string; region?: string }
          ip = payload.ip || ''
          location = [payload.city, payload.region, payload.country].filter(Boolean).join(' · ')
        } else {
          const payload = await res.json() as { ip?: string; city?: string; region?: string; country?: string; org?: string }
          ip = payload.ip || ''
          location = [payload.city, payload.region, payload.country, payload.org].filter(Boolean).join(' · ')
        }
        if (!ip) throw new Error('empty')
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
      const elapsed = Math.max(1, Math.round(performance.now() - started))
      const { stage, message } = categorizeBrowserFetchError(error, elapsed)
      return {
        name: target.name,
        group: target.group,
        url: target.url,
        description: target.description,
        via: '由当前浏览器客户端直连发起检测',
        ok: false,
        error: message,
        stage,
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
  return <span className={`inline-flex items-center rounded-full border px-3 py-1 text-xs font-medium backdrop-blur-sm ${className}`}>{label}</span>
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
    <div className="glass-card overflow-hidden">
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

function MetricTile({ icon, label, value, hint }: { icon: ReactNode; label: string; value: string; hint?: string }) {
  return (
    <div className="dashboard-card">
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
    <div className="dashboard-card">
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

const STAGE_LABELS: Record<string, string> = {
  proxy_port: '代理端口未就绪',
  dns:        'DNS 解析失败',
  timeout:    '连接超时',
  connect:    '连接失败',
}

function AccessCard({ item }: { item: PaneAccessCheck }) {
  const stageLabel = item.stage ? (STAGE_LABELS[item.stage] ?? item.stage) : null
  return (
    <div className="dashboard-card">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-semibold text-slate-100 truncate">{item.name}</p>
          <p className="text-xs text-muted mt-0.5 truncate">{item.description}</p>
        </div>
        <Pill tone={item.ok ? 'success' : 'danger'} label={item.ok ? '正常' : '失败'} />
      </div>
      {item.ok ? (
        <div className="mt-3 flex items-center gap-2">
          <span className="text-sm font-mono text-slate-200">{item.latency_ms ?? 0} ms</span>
          {item.dns_result && <span className="text-xs text-muted truncate">DNS → {item.dns_result}</span>}
        </div>
      ) : (
        <div className="mt-3 space-y-1">
          {stageLabel && <p className="text-xs font-semibold text-danger">↳ {stageLabel}</p>}
          <p className="text-xs text-muted break-all leading-5">{item.error || '请求失败'}</p>
          {item.dns_result && <p className="text-xs text-muted/70">DNS → {item.dns_result}</p>}
        </div>
      )}
      <p className="text-[11px] text-muted/50 mt-2 break-all">{item.url}</p>
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
    <div className="dashboard-card">
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
type PaneAccessCheck = { name: string; group?: string; url: string; description: string; via: string; ok: boolean; latency_ms?: number; error?: string; stage?: string; dns_result?: string }

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
    <div className="glass-card overflow-hidden">
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

// ── Domain connectivity checker ────────────────────────────────────────────

interface DomainProbeState {
  domain: string
  router: DomainProbeResult | null
  browser: { ok: boolean; latency_ms?: number; error?: string } | null
}

const PRESET_DOMAINS = DOMAIN_PROBE_PRESETS

function DomainProbePanel({ domain, onDomainChange, loading, result, onRun }: {
  domain: string
  onDomainChange: (v: string) => void
  loading: boolean
  result: DomainProbeState | null
  onRun: () => void
}) {
  return (
    <div className="glass-card overflow-hidden">
      <div className="px-4 py-3.5 space-y-3">
        {/* Input row */}
        <div className="flex items-center gap-2">
          <input
            type="text"
            value={domain}
            onChange={e => onDomainChange(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter') onRun() }}
            placeholder="输入域名，如 google.com"
            className="glass-input h-8 flex-1 min-w-0 text-xs font-mono"
          />
          <button
            onClick={onRun}
            disabled={loading || !domain.trim()}
            className="btn-primary h-8 px-3 flex items-center gap-1.5 text-xs shrink-0"
          >
            {loading && <Loader2 size={11} className="animate-spin" />}
            {loading ? '检测中' : '探测'}
          </button>
        </div>

        {/* Preset chips */}
        <div className="flex flex-wrap gap-1.5">
          {PRESET_DOMAINS.map(d => (
            <button
              key={d}
              onClick={() => onDomainChange(d)}
              className={`px-2.5 py-0.5 rounded-full border text-[10px] font-mono transition-colors ${domain === d ? 'border-brand/40 bg-brand/10 text-brand' : 'border-white/10 bg-white/[0.03] text-muted hover:text-slate-300 hover:border-white/20'}`}
            >
              {d}
            </button>
          ))}
        </div>
      </div>

      {/* Results */}
      {result && (
        <div className="border-t border-white/8 grid grid-cols-2">
          {/* Router */}
          <div className="px-4 py-3 space-y-1 border-r border-white/8">
            <div className="flex items-center justify-between gap-2">
              <p className="text-[10px] font-semibold uppercase tracking-[0.16em] text-muted">路由器侧</p>
              <Pill tone={result.router?.ok ? 'success' : 'danger'} label={result.router?.ok ? '通' : '不通'} />
            </div>
            <p className="text-[10px] text-muted/60">经 Mihomo mixed 端口转发</p>
            {result.router?.ok
              ? <p className="text-sm font-mono font-semibold text-white">{result.router.latency_ms} ms</p>
              : <p className="text-xs text-danger/80 leading-5 break-all">{result.router?.error || '连接失败'}</p>}
            {result.router?.dns_ips?.[0] && (
              <p className="text-[10px] text-muted/50 font-mono">DNS → {result.router.dns_ips[0]}</p>
            )}
            {result.router?.dns_error && (
              <p className="text-[10px] text-warning/60 break-all">DNS 失败: {result.router.dns_error}</p>
            )}
          </div>
          {/* Browser */}
          <div className="px-4 py-3 space-y-1">
            <div className="flex items-center justify-between gap-2">
              <p className="text-[10px] font-semibold uppercase tracking-[0.16em] text-muted">浏览器侧</p>
              <Pill tone={result.browser?.ok ? 'success' : 'danger'} label={result.browser?.ok ? '通' : '不通'} />
            </div>
            <p className="text-[10px] text-muted/60">由浏览器直连，不经过代理</p>
            {result.browser?.ok
              ? <p className="text-sm font-mono font-semibold text-white">{result.browser.latency_ms} ms</p>
              : <p className="text-xs text-danger/80 leading-5 break-all">{result.browser?.error || '连接失败'}</p>}
          </div>
        </div>
      )}

      {loading && !result && (
        <div className="border-t border-white/8 px-4 py-4 flex items-center gap-2 text-xs text-muted">
          <Loader2 size={12} className="animate-spin" />
          正在同时从路由器和浏览器发起探测…
        </div>
      )}
    </div>
  )
}

function HealthTimelinePanel({
  summary,
  incidents,
  loading,
}: {
  summary: HealthSummaryData | null
  incidents: HealthIncident[]
  loading: boolean
}) {
  const nowMs = Date.now()
  const windowMs = TIMELINE_WINDOW_HOURS * 3600 * 1000
  const timeline = useMemo(
    () => buildTimelineSegments(summary, incidents, nowMs, TIMELINE_WINDOW_HOURS),
    [summary, incidents, nowMs]
  )

  const currentState = summary ? normalizeTimelineState(summary.current.state) : 'unknown'
  const changes = timeline.filter((seg) => seg.state !== 'healthy').slice(-6).reverse()
  const windowStart = nowMs - windowMs
  const intervalText = summary?.router_interval_sec ? `每 ${summary.router_interval_sec}s 探测` : '定时探测'

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className={`inline-flex items-center rounded-full border px-2.5 py-1 text-[11px] font-medium ${stateBadgeClass(currentState)}`}>
          当前状态：{stateLabel(currentState)}
        </span>
        <span className="inline-flex items-center gap-1 rounded-full border border-white/12 bg-white/[0.03] px-2.5 py-1 text-[11px] text-muted">
          <Clock3 size={11} />
          最近 {TIMELINE_WINDOW_HOURS} 小时
        </span>
        <span className="inline-flex items-center rounded-full border border-white/12 bg-white/[0.03] px-2.5 py-1 text-[11px] text-muted">
          {intervalText}
        </span>
      </div>

      {!summary && incidents.length === 0 && !loading ? (
        <div className="rounded-xl border border-dashed border-white/12 bg-black/15 px-4 py-6 text-sm text-muted">
          暂无健康时间轴数据，等待定时 probe 采样。
        </div>
      ) : (
        <>
          <div className="overflow-hidden rounded-xl border border-white/10 bg-black/20 p-3">
            <div className="h-5 overflow-hidden rounded-lg border border-white/8 bg-white/[0.04]">
              <div className="flex h-full w-full">
                {timeline.map((seg, idx) => {
                  const widthPct = Math.max(0.5, ((seg.endMs - seg.startMs) / windowMs) * 100)
                  const title = `${stateLabel(seg.state)} | ${formatTimelinePoint(seg.startMs)} - ${formatTimelinePoint(seg.endMs)} | 持续 ${formatTimelineDuration(seg.startMs, seg.endMs)}${seg.reason ? ` | ${seg.reason}` : ''}`
                  return (
                    <div
                      key={`${seg.state}-${seg.startMs}-${seg.endMs}-${idx}`}
                      className={`${stateBarClass(seg.state)} ${idx > 0 ? 'border-l border-black/25' : ''}`}
                      style={{ width: `${widthPct}%` }}
                      title={title}
                    />
                  )
                })}
              </div>
            </div>
            <div className="mt-2 flex items-center justify-between text-[10px] text-muted/70">
              <span>{new Date(windowStart).toLocaleTimeString()}</span>
              <span>{new Date(windowStart + windowMs / 2).toLocaleTimeString()}</span>
              <span>{new Date(nowMs).toLocaleTimeString()}</span>
            </div>
          </div>

          <div className="rounded-xl border border-white/10 bg-black/15">
            <div className="border-b border-white/8 px-3 py-2 text-[11px] uppercase tracking-[0.16em] text-muted">状态变化记录</div>
            {changes.length === 0 ? (
              <div className="px-3 py-3 text-sm text-muted">最近 {TIMELINE_WINDOW_HOURS} 小时未出现异常状态切换。</div>
            ) : (
              <div className="divide-y divide-white/6">
                {changes.map((item, idx) => (
                  <div key={`${item.startMs}-${item.endMs}-${idx}`} className="px-3 py-2.5 text-xs">
                    <div className="flex items-center gap-2">
                      <span className={`h-2 w-2 rounded-full ${stateBarClass(item.state)}`} />
                      <span className={`rounded-full border px-2 py-0.5 text-[10px] ${stateBadgeClass(item.state)}`}>{stateLabel(item.state)}</span>
                      <span className="text-muted">{formatTimelineDuration(item.startMs, item.endMs)}</span>
                    </div>
                    <div className="mt-1 text-muted">
                      {new Date(item.startMs).toLocaleString()} - {new Date(item.endMs).toLocaleString()}
                    </div>
                    {item.reason && <div className="mt-1 text-slate-300 break-all">{item.reason}</div>}
                  </div>
                ))}
              </div>
            )}
          </div>
        </>
      )}
    </div>
  )
}

const SKIP_VERSION_KEY = 'cf_skip_version'

function ResourceDrawer({
  data,
  loading,
  onRefresh,
  onClose,
}: {
  data: OverviewResourceData | null
  loading: boolean
  onRefresh: () => void
  onClose: () => void
}) {
  return (
    <>
      <div className="fixed inset-0 z-40 bg-black/60 backdrop-blur-sm" onClick={onClose} />
      <div
        className="fixed right-0 top-0 z-50 flex h-full w-full max-w-[480px] flex-col overflow-hidden shadow-2xl"
        style={{ background: 'linear-gradient(to bottom, #0d0d1f, #090914)', borderLeft: '1px solid rgba(255,255,255,0.08)' }}
      >
        <div className="flex shrink-0 items-center justify-between border-b border-white/8 px-5 py-4">
          <div className="flex items-center gap-2">
            <HardDrive size={15} className="text-brand-light" />
            <h2 className="text-sm font-semibold text-white">系统资源</h2>
          </div>
          <div className="flex items-center gap-2">
            <button className="btn-ghost flex items-center gap-1.5 text-xs" onClick={onRefresh} disabled={loading}>
              <RefreshCw size={12} className={loading ? 'animate-spin' : ''} />
              {loading ? '采样中…' : '刷新'}
            </button>
            <button className="btn-ghost p-1.5" onClick={onClose}><X size={14} /></button>
          </div>
        </div>
        <div className="flex-1 space-y-4 overflow-y-auto p-4">
          {!data ? (
            <div className="flex flex-col items-center justify-center gap-3 py-16 text-sm text-muted">
              {loading
                ? <><Loader2 size={22} className="animate-spin text-brand/50" /><span>正在采样系统资源…</span></>
                : <><Activity size={22} className="text-muted/30" /><span>点击"刷新"开始加载资源信息</span></>}
            </div>
          ) : (
            <>
              <div className="grid grid-cols-3 gap-2">
                <MetricTile icon={<Cpu size={14} />} label="CPU" value={formatPercent(data.resources.system.cpu_percent)} />
                <MetricTile icon={<Activity size={14} />} label="内存" value={formatMB(data.resources.system.memory_used_mb)} hint={formatPercent(data.resources.system.memory_percent)} />
                <MetricTile icon={<HardDrive size={14} />} label="磁盘" value={formatGB(data.resources.system.disk_used_gb)} hint={formatPercent(data.resources.system.disk_percent)} />
              </div>
              {data.resources.processes.length > 0 && (
                <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                  {data.resources.processes.map((item) => (
                    <ProcessCard key={item.id} name={item.name} pid={item.pid} cpu={item.cpu_percent}
                      memory={item.memory_rss_mb} uptime={item.uptime} running={item.running} command={item.command} />
                  ))}
                </div>
              )}
              <div className="dashboard-card">
                <p className="text-sm font-semibold text-slate-100">ClashForge 磁盘占用</p>
                <div className="mt-3 grid grid-cols-2 gap-3 text-sm sm:grid-cols-3">
                  {[
                    { label: '运行目录', val: formatMB(data.resources.app.runtime_mb) },
                    { label: '数据目录', val: formatMB(data.resources.app.data_mb) },
                    { label: '程序文件', val: formatMB(data.resources.app.binary_mb) },
                    { label: '规则文件', val: formatMB(data.resources.app.rules_mb) },
                    { label: '总占用',   val: formatMB(data.resources.app.total_mb) },
                  ].map(({ label, val }) => (
                    <div key={label}>
                      <p className="text-[11px] uppercase tracking-[0.16em] text-muted">{label}</p>
                      <p className="mt-1 text-slate-200">{val}</p>
                    </div>
                  ))}
                </div>
                {!!data.resources.app.rule_assets?.length && (
                  <div className="mt-4 space-y-2 border-t border-white/10 pt-3">
                    <p className="text-xs uppercase tracking-[0.16em] text-muted">规则文件明细</p>
                    {data.resources.app.rule_assets.map((asset) => (
                      <div key={`${asset.name}-${asset.path}`} className="flex items-center justify-between gap-3 text-xs">
                        <span className="truncate text-slate-200">{asset.name}</span>
                        <span className="shrink-0 text-muted">{formatMB(asset.size_mb)}</span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </>
          )}
        </div>
      </div>
    </>
  )
}

function UpdateBanner({ data, onSkip }: { data: ClashforgeVersionData; onSkip: () => void }) {
  return (
    <div className="hero-panel border-warning/20 bg-[linear-gradient(145deg,rgba(245,158,11,0.16),rgba(255,255,255,0.03))]">
      <div className="relative flex items-center justify-between gap-4 flex-wrap">
        <div className="min-w-0">
          <div className="inline-flex items-center gap-2 rounded-full border border-warning/25 bg-warning/10 px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.2em] text-warning">
            <Zap size={12} /> 新版本提醒
          </div>
          <p className="mt-3 text-lg font-semibold text-white">发现新版本 {data.latest}</p>
          <p className="mt-1 text-sm leading-6 text-warning/80">当前版本 {data.current}，可前往 GitHub Releases 下载最新安装包。</p>
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
    </div>
  )
}

export function Dashboard() {
  const { currentUp, currentDown, connCount, coreState, setCoreState, pushTraffic, setConnCount } = useStore()

  const [coreData, setCoreData] = useState<OverviewCoreData | null>(null)
  const [probeData, setProbeData] = useState<OverviewProbeData | null>(null)
  const [browserProbeData, setBrowserProbeData] = useState<BrowserProbeData | null>(null)
  const [resourceData, setResourceData] = useState<OverviewResourceData | null>(null)
  const [healthSummary, setHealthSummary] = useState<HealthSummaryData | null>(null)
  const [healthIncidents, setHealthIncidents] = useState<HealthIncident[]>([])

  const [queryingCore, setQueryingCore] = useState(true)
  const [loadingProbes, setLoadingProbes] = useState(false)
  const [loadingBrowserProbe, setLoadingBrowserProbe] = useState(false)
  const [loadingResources, setLoadingResources] = useState(false)
  const [loadingHealthTimeline, setLoadingHealthTimeline] = useState(false)

  const [versionData, setVersionData] = useState<ClashforgeVersionData | null>(null)
  const [showBanner, setShowBanner] = useState(false)
  const [liveHealthAlert, setLiveHealthAlert] = useState<{ state: string; reason: string; checkedAt?: string } | null>(null)

  // proxy switcher state
  const [proxies, setProxies] = useState<ProxyMap>({})
  const [testingLatency, setTestingLatency] = useState(false)

  // resource drawer + probe tab + last-switched-proxy tracking
  const [resourceDrawerOpen, setResourceDrawerOpen] = useState(false)
  const [probeTab, setProbeTab] = useState<'router' | 'browser' | 'domain'>('router')
  const [lastSwitchedProxy, setLastSwitchedProxy] = useState<{ group: string; proxy: string } | null>(null)

  // domain probe state (tab 3)
  const [domainInput, setDomainInput] = useState<string>(DEFAULT_DOMAIN_PROBE_INPUT)
  const [domainLoading, setDomainLoading] = useState(false)
  const [domainResult, setDomainResult] = useState<DomainProbeState | null>(null)

  // prevents auto-running probes more than once per mount
  const probesAutoRanRef = useRef(false)
  const browserSessionID = useId()

  useSSE({
    onCoreState: (data) => setCoreState(data.state, data.pid),
    onTraffic: (data) => pushTraffic(data),
    onConnCount: (data) => setConnCount(data.total),
    onHealthState: (data) => {
      const next = { state: data.state, reason: data.reason ?? '', checkedAt: data.checked_at }
      setLiveHealthAlert(next)
      if (data.state === 'healthy') {
        window.setTimeout(() => setLiveHealthAlert((prev) => (prev?.state === 'healthy' ? null : prev)), 5000)
      }
    },
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

  const refreshProbes = useCallback(async (): Promise<ProbeSnapshot> => {
    setLoadingProbes(true)
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
      if (browser) {
        void reportHealthBrowser({
          session_id: `dashboard-${browserSessionID}`,
          checked_at: browser.checked_at,
          user_agent: navigator.userAgent,
          ip_checks: browser.ip_checks.map((item) => ({
            provider: item.provider,
            group: item.group,
            ok: item.ok,
            ip: item.ip,
            location: item.location,
            error: item.error,
          })),
          access_checks: browser.access_checks.map((item) => ({
            name: item.name,
            group: item.group,
            url: item.url,
            ok: item.ok,
            latency_ms: item.latency_ms,
            error: item.error,
            stage: item.stage,
          })),
        }).catch(() => null)
      }
      return { router: next, browser }
    } finally {
      setLoadingBrowserProbe(false)
      setLoadingProbes(false)
    }
  }, [browserSessionID])

  const refreshResources = useCallback(async () => {
    setLoadingResources(true)
    const next = await getOverviewResources().catch(() => null)
    if (next) setResourceData(next)
    setLoadingResources(false)
  }, [])

  const refreshHealthTimeline = useCallback(async (silent = false) => {
    if (!silent) setLoadingHealthTimeline(true)
    try {
      const [summaryRes, incidentsRes] = await Promise.all([
        getHealthSummary().catch(() => null),
        getHealthIncidents(120).catch(() => null),
      ])
      if (summaryRes) setHealthSummary(summaryRes)
      if (incidentsRes) setHealthIncidents(incidentsRes.incidents ?? [])
    } finally {
      if (!silent) setLoadingHealthTimeline(false)
    }
  }, [])

  useEffect(() => {
    const bootstrap = setTimeout(() => { void refreshCore(false) }, 0)
    const timer = setInterval(() => { void refreshCore(true) }, 8000)
    return () => { clearTimeout(bootstrap); clearInterval(timer) }
  }, [refreshCore])

  useEffect(() => {
    const bootstrap = setTimeout(() => { void refreshHealthTimeline(false) }, 0)
    const timer = setInterval(() => { void refreshHealthTimeline(true) }, 15000)
    return () => { clearTimeout(bootstrap); clearInterval(timer) }
  }, [refreshHealthTimeline])

  // Only fetch proxies when we know the core is running — avoids 502 spam when Mihomo is stopped
  useEffect(() => {
    if (coreData?.core.state !== 'running') return
    const t = setTimeout(() => { void refreshProxies() }, 0)
    return () => clearTimeout(t)
  }, [coreData, refreshProxies])

  // Auto-run connectivity probes once when core is confirmed running
  useEffect(() => {
    if (!coreData || coreData.core.state !== 'running') return
    if (probesAutoRanRef.current) return
    probesAutoRanRef.current = true
    void refreshProbes()
  }, [coreData, refreshProbes])

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

  const handleSelectProxy = async (group: string, proxy: string) => {
    setLastSwitchedProxy({ group, proxy })
    await selectProxy(group, proxy).catch(() => null)
    await refreshProxies()
    // auto-run probes to verify the switch
    void refreshProbes()
  }

  const handleDomainProbe = async (d: string) => {
    const clean = d.trim().replace(/^https?:\/\//, '').replace(/\/.*$/, '')
    if (!clean) return
    setDomainLoading(true)
    setDomainResult(null)
    const [routerRes, browserRes] = await Promise.all([
      probeDomain(clean).catch((e: unknown) => ({
        domain: clean, checked_at: new Date().toISOString(),
        ok: false, error: e instanceof Error ? e.message : '请求失败',
      }) as DomainProbeResult),
      (async () => {
        const t0 = performance.now()
        try {
          await fetchWithTimeout(`https://${clean}`, { method: 'GET', mode: 'no-cors', cache: 'no-store' }, 8000)
          return { ok: true, latency_ms: Math.max(1, Math.round(performance.now() - t0)) }
        } catch (e) {
          const { message } = categorizeBrowserFetchError(e, Math.round(performance.now() - t0))
          return { ok: false, error: message }
        }
      })(),
    ])
    setDomainResult({ domain: clean, router: routerRes, browser: browserRes })
    setDomainLoading(false)
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
    <div className="space-y-4">

      {/* ── Hero ──────────────────────────────────────────────────────────── */}
      <PageHeader
        eyebrow="Dashboard"
        title="ClashForge 控制总台"
        description="核心状态、连通性诊断与代理节点切换一览。"
        actions={
          <div className="flex items-center gap-2">
            <button
              className="btn-ghost flex items-center gap-2"
              onClick={() => {
                setResourceDrawerOpen(true)
                if (!resourceData) void refreshResources()
              }}
            >
              <HardDrive size={14} />
              系统资源
            </button>
          </div>
        }
        metrics={[
          { label: '活跃连接', value: `${connCount}` },
          { label: '运行时长', value: coreData ? formatUptime(coreData.core.uptime) : '--' },
          { label: '上行速率', value: formatBytes(currentUp) },
          { label: '下行速率', value: formatBytes(currentDown) },
        ]}
      />
      {liveHealthAlert && liveHealthAlert.state !== 'healthy' && (
        <div className="hero-panel border-danger/25 bg-[linear-gradient(145deg,rgba(239,68,68,0.18),rgba(255,255,255,0.03))]">
          <div className="flex items-start gap-3 text-sm">
            <AlertTriangle size={16} className="mt-0.5 text-danger" />
            <div className="min-w-0">
              <p className="font-semibold text-white">健康检查告警：{liveHealthAlert.state === 'unhealthy' ? '严重异常' : '部分异常'}</p>
              <p className="mt-1 text-danger/90 leading-6 break-all">{liveHealthAlert.reason || '检测到链路异常，请查看概览与日志。'}</p>
            </div>
          </div>
        </div>
      )}

      {/* ── 子模块状态 (compact chips strip) ─────────────────────────────── */}
      {visibleModules.length > 0 && (
        <div className="flex flex-wrap items-center gap-x-5 gap-y-2 rounded-xl border border-white/[0.06] bg-white/[0.02] px-4 py-2.5">
          <span className="shrink-0 text-[10px] font-semibold uppercase tracking-[0.2em] text-muted/60">子模块</span>
          {visibleModules.map((m) => {
            const managed = m.managed_by_clashforge
            const dot = managed ? 'bg-success' : m.status === 'conflict' ? 'bg-warning' : m.status === 'inactive' ? 'bg-danger' : 'bg-white/20'
            const labelColor = managed ? 'text-success' : m.status === 'conflict' ? 'text-warning' : m.status === 'inactive' ? 'text-danger/80' : 'text-muted'
            const label = managed ? '已接管' : m.status === 'conflict' ? '占用' : m.status === 'inactive' ? '未运行' : '--'
            return (
              <div key={m.id} className="flex items-center gap-1.5">
                <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${dot}${managed ? ' animate-pulse' : ''}`} />
                <span className="text-xs text-slate-300">{m.title}</span>
                <span className={`text-[11px] ${labelColor}`}>{label}</span>
              </div>
            )
          })}
          {coreData?.checked_at && (
            <span className="ml-auto text-[10px] text-muted/50">
              {new Date(coreData.checked_at).toLocaleTimeString()}
            </span>
          )}
        </div>
      )}

      {/* ── UpdateBanner ──────────────────────────────────────────────────── */}
      {showBanner && versionData && (
        <UpdateBanner
          data={versionData}
          onSkip={() => {
            localStorage.setItem(SKIP_VERSION_KEY, versionData.latest)
            setShowBanner(false)
          }}
        />
      )}

      {/* ── 连通性 & 节点切换 (combined) ──────────────────────────────────── */}
      <SectionCard
        title="连通性 & 节点切换"
        description="切换节点后将自动重新执行连通性检测"
        actions={
          <div className="flex items-center gap-2">
            <button className="btn-ghost flex items-center gap-2" onClick={refreshProxies}>
              <RefreshCw size={14} /> 刷新节点
            </button>
            <button className="btn-ghost flex items-center gap-2" onClick={handleTestLatency} disabled={testingLatency}>
              <Zap size={14} className={testingLatency ? 'animate-pulse' : ''} />
              {testingLatency ? '测速中…' : '测速'}
            </button>
            <button
              className="btn-ghost flex items-center gap-2"
              onClick={() => { setLastSwitchedProxy(null); void refreshProbes() }}
              disabled={loadingProbes}
            >
              <RefreshCw size={14} className={loadingProbes ? 'animate-spin' : ''} />
              重新检测
            </button>
          </div>
        }
      >
        {/* Node-switch trigger status */}
        {lastSwitchedProxy && (
          <div className="mb-4 flex items-center gap-2 rounded-xl border border-brand/20 bg-brand/[0.06] px-3 py-2 text-xs text-brand/90">
            <Zap size={11} className="shrink-0" />
            <span>已切换至 <span className="font-semibold">{lastSwitchedProxy.group}</span> → <span className="font-semibold">{lastSwitchedProxy.proxy}</span></span>
            {loadingProbes
              ? <><span className="ml-1 text-muted">· 正在重新检测…</span><Loader2 size={11} className="ml-auto animate-spin text-muted" /></>
              : <span className="ml-1 text-success">· 检测已更新</span>}
          </div>
        )}

        <div className="grid grid-cols-1 gap-4 xl:grid-cols-5">
          {/* Left (3/5): connectivity probe */}
          <div className="space-y-0 xl:col-span-3">
            {/* Tab bar */}
            <div className="-mx-px flex items-center border-b border-white/8 pb-0">
              {(['router', 'browser'] as const).map((tab) => {
                const isRouter = tab === 'router'
                const health = isRouter ? routerProbeHealth : browserProbeHealth
                const isLoading = isRouter ? loadingProbes : loadingBrowserProbe
                const isActive = probeTab === tab
                return (
                  <button
                    key={tab}
                    onClick={() => setProbeTab(tab)}
                    className={`-mb-px flex items-center gap-2 border-b-2 px-3 py-2.5 text-xs font-medium transition-colors ${
                      isActive ? 'border-brand text-white' : 'border-transparent text-muted hover:text-slate-300'
                    }`}
                  >
                    {isRouter ? '路由器侧' : '浏览器侧'}
                    {isLoading
                      ? <Loader2 size={10} className="animate-spin text-muted" />
                      : health.hasData
                        ? <span className={`h-1.5 w-1.5 rounded-full ${health.healthy ? 'bg-success' : 'bg-warning'}`} />
                        : null}
                  </button>
                )
              })}
              {/* Domain probe tab */}
              <button
                onClick={() => setProbeTab('domain')}
                className={`-mb-px flex items-center gap-2 border-b-2 px-3 py-2.5 text-xs font-medium transition-colors ${
                  probeTab === 'domain' ? 'border-brand text-white' : 'border-transparent text-muted hover:text-slate-300'
                }`}
              >
                域名测试
                {domainLoading
                  ? <Loader2 size={10} className="animate-spin text-muted" />
                  : domainResult
                    ? <span className={`h-1.5 w-1.5 rounded-full ${domainResult.router?.ok && domainResult.browser?.ok ? 'bg-success' : 'bg-warning'}`} />
                    : null}
              </button>
            </div>

            <div className="pt-3">
              {probeTab === 'domain' ? (
                <DomainProbePanel
                  domain={domainInput}
                  onDomainChange={setDomainInput}
                  loading={domainLoading}
                  result={domainResult}
                  onRun={() => void handleDomainProbe(domainInput)}
                />
              ) : !probeData && !loadingProbes ? (
                <div className="rounded-2xl border border-dashed border-white/15 bg-black/10 px-4 py-5 text-sm text-muted">
                  {coreRunning ? '正在准备首次检测…' : '内核未运行，启动服务后将自动检测。'}
                </div>
              ) : probeTab === 'router' ? (
                <ProbePane
                  title="路由器侧"
                  subtitle="经 ClashForge mixed 端口转发"
                  health={routerProbeHealth}
                  ipChecks={probeData?.ip_checks ?? []}
                  accessChecks={probeData?.access_checks ?? []}
                  loading={loadingProbes}
                />
              ) : (
                <ProbePane
                  title="浏览器侧"
                  subtitle="由当前浏览器客户端直连，不经过代理"
                  health={browserProbeHealth}
                  ipChecks={browserProbeData?.ip_checks ?? []}
                  accessChecks={browserProbeData?.access_checks ?? []}
                  loading={loadingBrowserProbe}
                />
              )}
            </div>
          </div>

          {/* Right (2/5): proxy switcher */}
          <div className="space-y-3 xl:col-span-2">
            <p className="-mb-1 border-b border-white/8 pb-2.5 text-[10px] font-semibold uppercase tracking-[0.2em] text-muted">节点切换</p>
            {proxyGroups.length === 0 ? (
              <div className="rounded-2xl border border-dashed border-white/15 bg-black/10 px-4 py-5 text-sm text-muted">
                {coreRunning ? '未找到代理组，请先添加订阅并更新节点。' : '内核未运行，无法获取节点列表。'}
              </div>
            ) : (
              proxyGroups.map(([name, group]) => (
                <ProxyGroup key={name} name={name} group={group} allProxies={proxies} onSelect={handleSelectProxy} />
              ))
            )}
          </div>
        </div>
      </SectionCard>

      <SectionCard
        title="网络状态时间轴"
        description="基于定时 probe 任务，展示最近 24 小时网络状态的开始、结束与切换。"
        actions={(
          <button
            className="btn-ghost flex items-center gap-2"
            onClick={() => { void refreshHealthTimeline(false) }}
            disabled={loadingHealthTimeline}
          >
            <RefreshCw size={14} className={loadingHealthTimeline ? 'animate-spin' : ''} />
            {loadingHealthTimeline ? '刷新中…' : '刷新'}
          </button>
        )}
      >
        <HealthTimelinePanel
          summary={healthSummary}
          incidents={healthIncidents}
          loading={loadingHealthTimeline}
        />
      </SectionCard>

      {/* ── Resource Drawer ───────────────────────────────────────────────── */}
      {resourceDrawerOpen && (
        <ResourceDrawer
          data={resourceData}
          loading={loadingResources}
          onRefresh={() => { void refreshResources() }}
          onClose={() => setResourceDrawerOpen(false)}
        />
      )}
    </div>
  )
}

