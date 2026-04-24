import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import {
  Activity, Cpu, ExternalLink, HardDrive,
  Loader2, RefreshCw, X, Zap, ChevronDown, ChevronUp,
  Globe, Shield, Wifi, Server, ArrowUp, ArrowDown,
} from 'lucide-react'

import {
  getOverviewCore, getOverviewProbes, getOverviewResources,
  getClashforgeVersion, restartCore, getProxies, selectProxy, testLatency,
} from '../api/client'
import type { ClashforgeVersionData } from '../api/client'
import type {
  OverviewAccessCheck, OverviewCoreData, OverviewIPCheck,
  OverviewModule, OverviewProbeData, OverviewResourceData,
} from '../api/client'
import type { ProxyNode } from '../api/client'
import { useSSE } from '../hooks/useSSE'
import { useStore } from '../store'
import { formatBytes, formatGB, formatMB, formatPercent, formatUptime, latencyColor, latencyBarColor, latencyBarWidth } from '../utils/format'

type SectionKey = 'probes' | 'resources'
type ProxyMap = Record<string, ProxyNode>

const TYPE_ORDER = ['Selector', 'Fallback', 'URLTest', 'LoadBalance']
const IGNORED = ['DIRECT', 'REJECT', 'GLOBAL', 'PASS', 'REJECT-DROP', 'Compatible']

interface BrowserIPCheck { provider: string; group: string; ok: boolean; ip?: string; location?: string; error?: string }
interface BrowserAccessCheck { name: string; group?: string; url: string; description: string; via: string; ok: boolean; latency_ms?: number; error?: string }
interface BrowserProbeData { checked_at: string; ip_checks: BrowserIPCheck[]; access_checks: BrowserAccessCheck[] }
interface ProbeSnapshot { router: OverviewProbeData | null; browser: BrowserProbeData | null }
interface ProbeHealth { hasData: boolean; ipOK: boolean; failedAccess: string[]; healthy: boolean }

function evaluateRouterProbeHealth(data: OverviewProbeData | null): ProbeHealth {
  if (!data) return { hasData: false, ipOK: false, failedAccess: [], healthy: false }
  const ipOK = data.ip_checks.some((item) => item.ok)
  const failedAccess = data.access_checks.filter((item) => !item.ok).map((item) => item.name)
  return { hasData: true, ipOK, failedAccess, healthy: ipOK && failedAccess.length === 0 }
}

function evaluateBrowserProbeHealth(data: BrowserProbeData | null): ProbeHealth {
  if (!data) return { hasData: false, ipOK: false, failedAccess: [], healthy: false }
  const ipOK = data.ip_checks.some((c) => c.ok)
  const failedAccess = data.access_checks.filter((item) => !item.ok).map((item) => item.name)
  return { hasData: true, ipOK, failedAccess, healthy: ipOK && failedAccess.length === 0 }
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
      provider: 'UpaiYun', group: '国内',
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
      provider: 'IP.SB', group: '国外',
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
      provider: 'IPInfo', group: '国外',
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
        name: target.name, group: target.group, url: target.url, description: target.description,
        via: '由当前浏览器客户端直连发起检测', ok: true,
        latency_ms: Math.max(1, Math.round(performance.now() - started)),
      } satisfies BrowserAccessCheck
    } catch (error) {
      return {
        name: target.name, group: target.group, url: target.url, description: target.description,
        via: '由当前浏览器客户端直连发起检测', ok: false,
        error: error instanceof Error ? error.message : '浏览器访问失败',
      } satisfies BrowserAccessCheck
    }
  }))

  return { checked_at: new Date().toISOString(), ip_checks, access_checks: accessChecks }
}

// ── Shared UI primitives ────────────────────────────────────────────────────

function StatusPill({ tone, label }: { tone: 'success' | 'warning' | 'danger' | 'muted' | 'brand'; label: string }) {
  const cls = {
    success: 'badge-success',
    warning: 'badge-warning',
    danger:  'badge-danger',
    muted:   'badge-muted',
    brand:   'badge-brand',
  }[tone]
  return <span className={cls}>{label}</span>
}

function SectionHeader({ eyebrow, title, right }: { eyebrow: string; title: string; right?: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 flex-wrap mb-4">
      <div>
        <p className="section-label">{eyebrow}</p>
        <h2 className="section-title">{title}</h2>
      </div>
      {right && <div className="flex items-center gap-2">{right}</div>}
    </div>
  )
}

// ── Module row ──────────────────────────────────────────────────────────────

function ModuleRow({ module }: { module: OverviewModule }) {
  const managed = module.managed_by_clashforge
  const tone: 'success' | 'warning' | 'danger' | 'muted' = managed
    ? 'success'
    : module.status === 'conflict' ? 'warning'
    : module.status === 'inactive' ? 'danger'
    : 'muted'

  const label = managed ? '已接管'
    : module.status === 'conflict' ? '有占用'
    : module.status === 'inactive' ? '未运行'
    : '待接管'

  const icons: Record<string, ReactNode> = {
    proxy_core: <Cpu size={14} className="text-brand" />,
    transparent_proxy: <Shield size={14} className="text-brand" />,
    nft_firewall: <Shield size={14} className="text-accent" />,
    dns_entry: <Globe size={14} className="text-brand" />,
    dns_resolver: <Globe size={14} className="text-accent" />,
  }

  return (
    <div className={`rounded-2xl border px-4 py-4 transition-all duration-200 ${
      managed
        ? 'border-success/15 bg-success/[0.04]'
        : module.status === 'conflict'
          ? 'border-warning/15 bg-warning/[0.04]'
          : 'border-white/[0.06] bg-surface-2/30'
    }`}>
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-start gap-2.5 min-w-0">
          <div className="mt-0.5 flex-shrink-0">{icons[module.id] ?? <Server size={14} className="text-muted" />}</div>
          <div className="min-w-0">
            <p className="text-sm font-semibold text-slate-100 truncate">{module.title}</p>
            <p className="text-xs text-muted mt-0.5 leading-5">{module.current_owner}</p>
          </div>
        </div>
        <StatusPill tone={tone} label={label} />
      </div>
    </div>
  )
}

// ── Latency bar ─────────────────────────────────────────────────────────────

function LatencyBar({ ms }: { ms: number }) {
  if (!ms || ms <= 0) return <span className="text-muted text-xs font-mono">—</span>
  return (
    <div className="flex items-center gap-2">
      <div className="w-10 h-1 bg-surface-3 rounded-full overflow-hidden">
        <div className={`h-full rounded-full transition-all ${latencyBarColor(ms)}`} style={{ width: latencyBarWidth(ms) }} />
      </div>
      <span className={`text-xs tabular-nums font-mono ${latencyColor(ms)}`}>{ms}<span className="text-muted/60">ms</span></span>
    </div>
  )
}

// ── Proxy group ─────────────────────────────────────────────────────────────

function ProxyGroup({ name, group, allProxies, onSelect }: {
  name: string; group: ProxyNode; allProxies: ProxyMap
  onSelect: (group: string, proxy: string) => void
}) {
  const [open, setOpen] = useState(true)
  const members = (group.all ?? []).filter(n => !IGNORED.includes(n))

  return (
    <div className="rounded-2xl border border-white/[0.07] bg-surface-2/20 overflow-hidden">
      <button
        onClick={() => setOpen(o => !o)}
        className="w-full flex items-center justify-between px-4 py-3.5 hover:bg-white/[0.03] transition-all cursor-pointer"
      >
        <div className="flex items-center gap-3">
          <span className="text-sm font-semibold text-white">{name}</span>
          <span className="text-[10px] px-2 py-0.5 rounded-full bg-surface-3/60 border border-white/[0.08] text-muted font-mono">{group.type}</span>
          {group.now && (
            <span className="text-xs text-muted">
              → <span className="text-brand font-medium">{group.now}</span>
            </span>
          )}
        </div>
        {open
          ? <ChevronUp size={14} className="text-muted" />
          : <ChevronDown size={14} className="text-muted" />
        }
      </button>
      {open && (
        <div className="px-3 pb-3 space-y-0.5 max-h-64 overflow-y-auto border-t border-white/[0.05]">
          {members.map(m => {
            const node = allProxies[m]
            if (!node) return <div key={m} className="px-4 py-2 text-sm text-muted">{m}</div>
            const isSelected = group.now === m
            const lastDelay = node.history?.at(-1)?.delay ?? -1
            return (
              <button
                key={m}
                onClick={() => { if (group.type === 'Selector') onSelect(name, m) }}
                className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-xl transition-all text-left cursor-pointer ${
                  isSelected
                    ? 'bg-brand/10 border border-brand/25'
                    : 'hover:bg-white/[0.04] border border-transparent'
                }`}
              >
                <div className={`w-1.5 h-1.5 rounded-full flex-shrink-0 transition-all ${
                  isSelected ? 'bg-brand' : 'bg-surface-3'
                }`}
                  style={isSelected ? { boxShadow: '0 0 6px rgba(6,182,212,0.8)' } : {}}
                />
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
                className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-xl transition-all text-left cursor-pointer ${
                  isSelected
                    ? 'bg-brand/10 border border-brand/25'
                    : 'hover:bg-white/[0.04] border border-transparent'
                }`}
              >
                <div className={`w-1.5 h-1.5 rounded-full flex-shrink-0 ${isSelected ? 'bg-brand' : 'bg-surface-3'}`}
                  style={isSelected ? { boxShadow: '0 0 6px rgba(6,182,212,0.8)' } : {}}
                />
                <span className={`text-sm font-medium ${isSelected ? 'text-brand' : 'text-slate-200'}`}>{n}</span>
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}

// ── Core state badge ─────────────────────────────────────────────────────────

function CoreStateBadge({ state }: { state: string }) {
  const tone = ({
    running: 'success', stopped: 'danger', error: 'danger',
    starting: 'warning', stopping: 'warning', querying: 'warning', unknown: 'warning',
  } as Record<string, 'success'|'warning'|'danger'>)[state]

  const label = ({
    running: '运行中', stopped: '已停止', error: '异常',
    starting: '启动中…', stopping: '停止中…', querying: '查询中…', unknown: '查询中…',
  })[state] ?? state

  if (state === 'querying' || state === 'unknown') {
    return (
      <span className="badge-warning inline-flex items-center gap-1.5">
        <Loader2 size={11} className="animate-spin" />{label}
      </span>
    )
  }
  return <StatusPill tone={tone ?? 'muted'} label={label} />
}

// ── Metric tile ──────────────────────────────────────────────────────────────

function MetricTile({ icon, label, value, hint, accent }: {
  icon: ReactNode; label: string; value: string; hint?: string; accent?: boolean
}) {
  return (
    <div className={`rounded-2xl px-4 py-4 transition-all ${
      accent
        ? 'border border-brand/20 bg-brand/[0.04]'
        : 'border border-white/[0.06] bg-surface-2/30'
    }`}>
      <div className="flex items-center gap-2 mb-3">
        <div className={accent ? 'text-brand' : 'text-muted'}>{icon}</div>
        <p className="text-[10px] uppercase tracking-[0.18em] text-muted">{label}</p>
      </div>
      <p className={`text-xl font-bold font-mono tabular-nums ${accent ? 'text-brand' : 'text-white'}`}>
        {value}
      </p>
      {hint && <p className="text-xs text-muted mt-1.5">{hint}</p>}
    </div>
  )
}

// ── Traffic tiles ────────────────────────────────────────────────────────────

function TrafficTile({ direction, value }: { direction: 'up' | 'down'; value: string }) {
  const isUp = direction === 'up'
  return (
    <div className={`rounded-2xl px-4 py-4 border transition-all ${
      isUp
        ? 'border-warning/15 bg-warning/[0.04]'
        : 'border-brand/15 bg-brand/[0.04]'
    }`}>
      <div className="flex items-center gap-2 mb-3">
        {isUp
          ? <ArrowUp size={14} className="text-warning" />
          : <ArrowDown size={14} className="text-brand" />
        }
        <p className="text-[10px] uppercase tracking-[0.18em] text-muted">{isUp ? '上行速率' : '下行速率'}</p>
      </div>
      <p className={`text-xl font-bold font-mono tabular-nums ${isUp ? 'text-warning' : 'text-brand'}`}>
        {value}
      </p>
    </div>
  )
}

// ── Status dot ───────────────────────────────────────────────────────────────

function StatusDot({ online }: { online: boolean }) {
  return (
    <span className={online ? 'status-dot-online' : 'status-dot-offline'} aria-hidden />
  )
}

// ── Grouping helpers ─────────────────────────────────────────────────────────

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
    ? 'bg-sky-500/10 border-sky-400/25 text-sky-400'
    : label === '国外'
      ? 'bg-violet-500/10 border-violet-400/25 text-violet-400'
      : label === 'AI'
        ? 'bg-amber-500/10 border-amber-400/25 text-amber-400'
        : 'bg-white/[0.05] border-white/[0.1] text-slate-400'
  return (
    <span className={`inline-flex items-center rounded-full border px-2.5 py-0.5 text-[10px] font-semibold tracking-wider ${cls}`}>
      {label}
    </span>
  )
}

// ── IP card ──────────────────────────────────────────────────────────────────

function IPCard({ item }: { item: OverviewIPCheck }) {
  return (
    <div className={`rounded-2xl border px-4 py-4 transition-all ${
      item.ok
        ? 'border-success/15 bg-success/[0.04]'
        : 'border-danger/15 bg-danger/[0.04]'
    }`}>
      <div className="flex items-center justify-between gap-3 mb-3">
        <p className="text-sm font-semibold text-slate-100">{item.provider}</p>
        <StatusPill tone={item.ok ? 'success' : 'danger'} label={item.ok ? '已解析' : '失败'} />
      </div>
      {item.ok ? (
        <>
          <p className="text-base font-bold font-mono text-white">{item.ip || '--'}</p>
          <p className="text-xs text-muted mt-1.5 leading-5">{item.location || '未返回位置信息'}</p>
        </>
      ) : (
        <p className="text-xs text-danger/80 leading-5">{item.error || '无法获取出口 IP'}</p>
      )}
    </div>
  )
}

// ── Access card ──────────────────────────────────────────────────────────────

function AccessCard({ item }: { item: OverviewAccessCheck }) {
  return (
    <div className={`rounded-2xl border px-4 py-4 transition-all ${
      item.ok
        ? 'border-success/15 bg-success/[0.04]'
        : 'border-danger/15 bg-danger/[0.04]'
    }`}>
      <div className="flex items-start justify-between gap-3 mb-3">
        <div>
          <p className="text-sm font-semibold text-slate-100">{item.name}</p>
          <p className="text-xs text-muted mt-0.5">{item.description}</p>
        </div>
        <StatusPill tone={item.ok ? 'success' : 'danger'} label={item.ok ? '正常' : '失败'} />
      </div>
      <p className={`text-sm font-mono font-semibold ${item.ok ? 'text-success' : 'text-danger/80'}`}>
        {item.ok ? `${item.latency_ms ?? 0} ms` : (item.error || '请求失败')}
      </p>
      <p className="text-xs text-muted mt-1.5 break-all opacity-60">{item.url}</p>
    </div>
  )
}

// ── Process card ─────────────────────────────────────────────────────────────

function ProcessCard({ name, pid, cpu, memory, uptime, running, command }: {
  name: string; pid: number; cpu: number; memory: number
  uptime: number; running: boolean; command?: string
}) {
  return (
    <div className="rounded-2xl border border-white/[0.07] bg-surface-2/30 px-4 py-4">
      <div className="flex items-center justify-between gap-3 mb-4">
        <div className="flex items-center gap-2.5">
          <StatusDot online={running} />
          <p className="text-sm font-semibold text-slate-100">{name}</p>
        </div>
        <span className="text-[10px] font-mono text-muted bg-surface-3/60 px-2 py-0.5 rounded-full">PID {pid || '--'}</span>
      </div>
      <div className="grid grid-cols-2 gap-3 text-sm">
        {[
          { l: 'CPU', v: formatPercent(cpu) },
          { l: '内存', v: formatMB(memory) },
          { l: '运行时长', v: formatUptime(uptime) },
          { l: '状态', v: running ? '在线' : '离线' },
        ].map(({ l, v }) => (
          <div key={l}>
            <p className="text-[10px] uppercase tracking-[0.18em] text-muted mb-1">{l}</p>
            <p className="text-slate-200 font-mono text-sm">{v}</p>
          </div>
        ))}
      </div>
      {command && <p className="text-[10px] text-muted/60 mt-3 break-all font-mono leading-4">{command}</p>}
    </div>
  )
}

// ── Probe pane ───────────────────────────────────────────────────────────────

type PaneIPCheck = { provider: string; group?: string; ok: boolean; ip?: string; location?: string; error?: string }
type PaneAccessCheck = { name: string; group?: string; url: string; description: string; via: string; ok: boolean; latency_ms?: number; error?: string }

function ProbePane({ title, subtitle, health, ipChecks, accessChecks, loading }: {
  title: string; subtitle: string; health: ProbeHealth
  ipChecks: PaneIPCheck[]; accessChecks: PaneAccessCheck[]; loading?: boolean
}) {
  const ipGroups = groupBy(ipChecks)
  const accessGroups = groupBy(accessChecks)
  const hasContent = ipChecks.length > 0 || accessChecks.length > 0

  return (
    <div className="rounded-2xl border border-white/[0.08] bg-surface-1/60 overflow-hidden">
      <div className="flex items-center justify-between gap-3 px-4 py-3.5 border-b border-white/[0.06] bg-white/[0.01]">
        <div>
          <div className="flex items-center gap-2">
            <p className="text-sm font-semibold text-white">{title}</p>
            {loading && <Loader2 size={11} className="animate-spin text-muted" />}
          </div>
          <p className="text-xs text-muted mt-0.5">{subtitle}</p>
        </div>
        {health.hasData
          ? <StatusPill tone={health.healthy ? 'success' : 'warning'} label={health.healthy ? '通过' : '待修复'} />
          : <StatusPill tone="muted" label={loading ? '检测中' : '等待'} />
        }
      </div>
      <div className="px-4 py-4 space-y-5">
        {!hasContent ? (
          <p className="text-xs text-muted py-2 text-center">{loading ? '正在检测…' : '暂无数据'}</p>
        ) : (
          <>
            {ipGroups.length > 0 && (
              <div className="space-y-3">
                <div className="divider"><span className="section-label">出口 IP</span></div>
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
            {accessGroups.length > 0 && (
              <div className="space-y-3">
                <div className="divider"><span className="section-label">访问检查</span></div>
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

// ── Update banner ─────────────────────────────────────────────────────────────

const SKIP_VERSION_KEY = 'cf_skip_version'

function UpdateBanner({ data, onSkip }: { data: ClashforgeVersionData; onSkip: () => void }) {
  return (
    <div className="rounded-2xl border border-brand/25 bg-brand/[0.06] px-5 py-4 flex items-center justify-between gap-4 flex-wrap animate-slide-in"
      style={{ boxShadow: '0 0 30px rgba(6,182,212,0.08)' }}
    >
      <div className="flex items-center gap-3 min-w-0">
        <div className="w-8 h-8 rounded-xl bg-brand/15 border border-brand/25 flex items-center justify-center flex-shrink-0">
          <Zap size={15} className="text-brand" />
        </div>
        <div className="min-w-0">
          <p className="text-sm font-semibold text-brand">发现新版本 {data.latest}</p>
          <p className="text-xs text-muted mt-0.5">当前版本 {data.current}，前往 GitHub Releases 下载安装包</p>
        </div>
      </div>
      <div className="flex items-center gap-2 shrink-0">
        <a href={data.release_url} target="_blank" rel="noopener noreferrer"
          className="btn-ghost flex items-center gap-1.5 text-brand border-brand/25 hover:bg-brand/10 text-xs">
          <ExternalLink size={13} /> 查看发布页
        </a>
        <button className="btn-icon" onClick={onSkip} title="跳过此版本">
          <X size={14} />
        </button>
      </div>
    </div>
  )
}

// ── Dashboard ─────────────────────────────────────────────────────────────────

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
      if (typeof next.core.active_connections === 'number') setConnCount(next.core.active_connections)
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
      if (!next) { setProbeData(null); setBrowserProbeData(null); return { router: null, browser: null } }
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

  useEffect(() => {
    if (coreData?.core.state !== 'running') return
    const t = setTimeout(() => { void refreshProxies() }, 0)
    return () => clearTimeout(t)
  }, [coreData, refreshProxies])

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
    for (const id of preferredOrder) { const found = byID.get(id); if (found) ordered.push(found) }
    return ordered
  }, [coreData])

  const proxyGroups = useMemo(() =>
    Object.entries(proxies)
      .filter(([, v]) => TYPE_ORDER.includes(v.type) && !IGNORED.includes(v.name) && v.name !== 'GLOBAL')
      .sort(([, a], [, b]) => TYPE_ORDER.indexOf(a.type) - TYPE_ORDER.indexOf(b.type)),
    [proxies]
  )

  return (
    <div className="p-6 space-y-5 max-w-7xl mx-auto">

      {/* Update banner */}
      {showBanner && versionData && (
        <UpdateBanner data={versionData} onSkip={() => {
          localStorage.setItem(SKIP_VERSION_KEY, versionData.latest)
          setShowBanner(false)
        }} />
      )}

      {/* ── Core status ── */}
      <div className="card px-5 py-5">
        <div className="flex items-center justify-between gap-4 flex-wrap mb-5">
          <div className="flex items-center gap-3">
            <div className={`w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0 transition-all ${
              coreRunning ? 'bg-success/15 border border-success/25' : 'bg-surface-3/60 border border-white/[0.07]'
            }`}
              style={coreRunning ? { boxShadow: '0 0 16px rgba(16,185,129,0.2)' } : {}}
            >
              <Cpu size={16} className={coreRunning ? 'text-success' : 'text-muted'} />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <CoreStateBadge state={effectiveState} />
                <p className="text-sm font-semibold text-white">{coreRunning ? '内核运行中' : '内核未运行'}</p>
              </div>
              <p className="text-xs text-muted mt-0.5">
                {coreData ? `PID ${coreData.core.pid} · 运行 ${formatUptime(coreData.core.uptime)}` : '等待状态'}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <button className="btn-ghost flex items-center gap-2 text-xs"
              onClick={() => { void refreshCore(false) }} disabled={queryingCore}>
              <RefreshCw size={13} className={queryingCore ? 'animate-spin' : ''} /> 刷新
            </button>
            <button className="btn-ghost flex items-center gap-2 text-xs"
              onClick={async () => {
                setLoadingAction('restart')
                await restartCore().catch(() => null)
                setLoadingAction(null)
                await refreshCore(true)
              }}
              disabled={!!loadingAction}>
              {loadingAction === 'restart' ? <Loader2 size={13} className="animate-spin" /> : <RefreshCw size={13} />}
              重启
            </button>
          </div>
        </div>

        <div className="grid grid-cols-2 xl:grid-cols-4 gap-3">
          <TrafficTile direction="up" value={formatBytes(currentUp)} />
          <TrafficTile direction="down" value={formatBytes(currentDown)} />
          <MetricTile icon={<Wifi size={15} />} label="活跃连接" value={`${connCount}`} hint="当前连接数" accent />
          <MetricTile icon={<Activity size={15} />} label="运行时长" value={coreData ? formatUptime(coreData.core.uptime) : '--'} hint={`PID ${coreData?.core.pid || '--'}`} />
        </div>
      </div>

      {/* ── Modules ── */}
      <div className="card px-5 py-5">
        <SectionHeader
          eyebrow="Modules"
          title="子模块状态"
          right={<p className="text-xs text-muted">{coreData?.checked_at ? `更新于 ${new Date(coreData.checked_at).toLocaleTimeString()}` : '等待查询'}</p>}
        />
        <div className="grid grid-cols-1 xl:grid-cols-3 gap-3">
          {visibleModules.map((module) => (
            <ModuleRow key={module.id} module={module} />
          ))}
          {!visibleModules.length && (
            <div className="col-span-3 rounded-2xl border border-white/[0.06] bg-surface-2/20 px-4 py-5 text-sm text-muted text-center">
              正在查询模块状态…
            </div>
          )}
        </div>
      </div>

      {/* ── Proxy switcher ── */}
      <div className="card px-5 py-5">
        <SectionHeader
          eyebrow="Proxies"
          title="节点切换"
          right={<>
            <button className="btn-ghost flex items-center gap-2 text-xs" onClick={refreshProxies}>
              <RefreshCw size={13} /> 刷新
            </button>
            <button className="btn-ghost flex items-center gap-2 text-xs" onClick={handleTestLatency} disabled={testingLatency}>
              <Zap size={13} className={testingLatency ? 'text-brand animate-pulse' : ''} />
              {testingLatency ? '测速中…' : '测速'}
            </button>
          </>}
        />
        {proxyGroups.length === 0 ? (
          <div className="rounded-2xl border border-dashed border-white/[0.1] bg-surface-2/20 px-4 py-6 text-sm text-muted text-center">
            {coreRunning ? '未找到代理组，请先添加订阅并更新节点。' : '内核未运行，无法获取节点列表。'}
          </div>
        ) : (
          <div className="space-y-2">
            {proxyGroups.map(([name, group]) => (
              <ProxyGroup key={name} name={name} group={group} allProxies={proxies} onSelect={handleSelectProxy} />
            ))}
          </div>
        )}
      </div>

      {/* ── Connectivity ── */}
      <div className="card px-5 py-5">
        <SectionHeader
          eyebrow="Connectivity"
          title="出口 IP / 访问检查"
          right={<>
            <button
              className={`btn-ghost text-xs flex items-center gap-1.5 ${section === 'probes' ? 'border-brand/30 text-brand bg-brand/[0.06]' : ''}`}
              onClick={() => { void openSection('probes') }}>
              <Globe size={13} /> IP 检查
            </button>
            <button
              className={`btn-ghost text-xs flex items-center gap-1.5 ${section === 'resources' ? 'border-brand/30 text-brand bg-brand/[0.06]' : ''}`}
              onClick={() => { void openSection('resources') }}>
              <HardDrive size={13} /> 资源占用
            </button>
          </>}
        />

        {!section && (
          <div className="rounded-2xl border border-dashed border-white/[0.1] bg-surface-2/20 px-4 py-6 text-sm text-muted text-center">
            切换节点后会自动执行 IP 检查，也可手动点击上方按钮。
          </div>
        )}

        {section === 'probes' && (
          <div className="animate-slide-in">
            <div className="flex items-center justify-between gap-3 mb-4">
              <p className="text-xs text-muted leading-5">路由器侧经代理转发，浏览器侧由客户端直连。对比两侧可快速判断代理出口是否工作正常。</p>
              <button className="btn-ghost flex items-center gap-2 text-xs flex-shrink-0"
                onClick={() => { void refreshProbes() }} disabled={loadingSection === 'probes'}>
                <RefreshCw size={13} className={loadingSection === 'probes' ? 'animate-spin' : ''} />
                重新检测
              </button>
            </div>
            {!probeData ? (
              <div className="rounded-2xl border border-white/[0.06] bg-surface-2/20 px-4 py-6 text-sm text-muted text-center">
                {loadingSection === 'probes' ? '正在进行联网检测…' : '点击"重新检测"开始。'}
              </div>
            ) : (
              <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
                <ProbePane title="路由器侧" subtitle="经 ClashForge mixed 端口转发"
                  health={routerProbeHealth} ipChecks={probeData.ip_checks}
                  accessChecks={probeData.access_checks} loading={loadingSection === 'probes'} />
                <ProbePane title="浏览器侧" subtitle="由当前浏览器客户端直连，不经过代理"
                  health={browserProbeHealth} ipChecks={browserProbeData?.ip_checks ?? []}
                  accessChecks={browserProbeData?.access_checks ?? []} loading={loadingBrowserProbe} />
              </div>
            )}
          </div>
        )}

        {section === 'resources' && (
          <div className="space-y-4 animate-slide-in">
            <div className="flex items-center justify-between gap-3">
              <p className="text-xs text-muted">系统资源与 ClashForge 进程占用。</p>
              <button className="btn-ghost flex items-center gap-2 text-xs"
                onClick={() => { void refreshResources() }} disabled={loadingSection === 'resources'}>
                <RefreshCw size={13} className={loadingSection === 'resources' ? 'animate-spin' : ''} />
                刷新
              </button>
            </div>
            {!resourceData ? (
              <div className="rounded-2xl border border-white/[0.06] bg-surface-2/20 px-4 py-6 text-sm text-muted text-center">
                {loadingSection === 'resources' ? '正在采样…' : '点击"刷新资源"开始加载。'}
              </div>
            ) : (
              <>
                <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                  <MetricTile icon={<Cpu size={15} />} label="系统 CPU" value={formatPercent(resourceData.resources.system.cpu_percent)} />
                  <MetricTile icon={<Activity size={15} />} label="系统内存"
                    value={`${formatMB(resourceData.resources.system.memory_used_mb)} / ${formatMB(resourceData.resources.system.memory_total_mb)}`}
                    hint={`已用 ${formatPercent(resourceData.resources.system.memory_percent)}`} />
                  <MetricTile icon={<HardDrive size={15} />} label="系统磁盘"
                    value={`${formatGB(resourceData.resources.system.disk_used_gb)} / ${formatGB(resourceData.resources.system.disk_total_gb)}`}
                    hint={`已用 ${formatPercent(resourceData.resources.system.disk_percent)}`} />
                </div>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                  {resourceData.resources.processes.map((item) => (
                    <ProcessCard key={item.id} name={item.name} pid={item.pid}
                      cpu={item.cpu_percent} memory={item.memory_rss_mb}
                      uptime={item.uptime} running={item.running} command={item.command} />
                  ))}
                </div>
                <div className="rounded-2xl border border-white/[0.07] bg-surface-2/30 px-4 py-4">
                  <p className="text-sm font-semibold text-slate-100 mb-3">ClashForge 磁盘占用</p>
                  <div className="grid grid-cols-2 md:grid-cols-5 gap-3">
                    {[
                      { label: '运行目录', val: formatMB(resourceData.resources.app.runtime_mb) },
                      { label: '数据目录', val: formatMB(resourceData.resources.app.data_mb) },
                      { label: '程序文件', val: formatMB(resourceData.resources.app.binary_mb) },
                      { label: '规则文件', val: formatMB(resourceData.resources.app.rules_mb) },
                      { label: '总占用',   val: formatMB(resourceData.resources.app.total_mb) },
                    ].map(({ label, val }) => (
                      <div key={label}>
                        <p className="text-[10px] uppercase tracking-[0.18em] text-muted mb-1">{label}</p>
                        <p className="text-slate-200 font-mono text-sm">{val}</p>
                      </div>
                    ))}
                  </div>
                  {!!resourceData.resources.app.rule_assets?.length && (
                    <div className="mt-4 border-t border-white/[0.07] pt-3 space-y-2">
                      <p className="section-label">规则文件明细</p>
                      {resourceData.resources.app.rule_assets.map((asset) => (
                        <div key={`${asset.name}-${asset.path}`} className="flex items-center justify-between gap-3">
                          <span className="text-xs text-slate-300">{asset.name}</span>
                          <span className="text-xs text-muted font-mono">{formatMB(asset.size_mb)} · {asset.path}</span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              </>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
