import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import {
  Activity,
  CheckCircle2,
  Cpu,
  HardDrive,
  Loader2,
  Play,
  RefreshCw,
  ShieldCheck,
  ShieldOff,
  Square,
  Zap,
} from 'lucide-react'

import {
  getOverviewCore,
  getOverviewProbes,
  getOverviewResources,
  reloadCore,
  restartCore,
  startCore,
  stopCore,
  takeoverOverviewModule,
} from '../api/client'
import type {
  OverviewAccessCheck,
  OverviewCoreData,
  OverviewIPCheck,
  OverviewModule,
  OverviewProbeData,
  OverviewResourceData,
} from '../api/client'
import { useSSE } from '../hooks/useSSE'
import { useStore } from '../store'
import { formatBytes, formatGB, formatMB, formatPercent, formatUptime } from '../utils/format'

type SectionKey = 'probes' | 'resources'

interface NoticeState {
  tone: 'success' | 'error' | 'info'
  message: string
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

function NoticeBanner({ notice }: { notice: NoticeState | null }) {
  if (!notice) return null
  const toneClass = {
    success: 'border-success/30 bg-success/10 text-success',
    error: 'border-danger/30 bg-danger/10 text-danger',
    info: 'border-brand/30 bg-brand/10 text-blue-200',
  }[notice.tone]
  return (
    <div className={`card border px-4 py-3 ${toneClass}`}>
      <p className="text-sm font-medium">{notice.message}</p>
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

function ModuleRow({ module, onTakeover, loading }: {
  module: OverviewModule
  onTakeover: (module: OverviewModule) => void
  loading: boolean
}) {
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
        : '可接管'

  const canTakeover = module.takeover_supported && !managed && !!module.action

  return (
    <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-4">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-slate-100">
            {managed ? <ShieldCheck size={14} className="text-success" /> : <ShieldOff size={14} className="text-warning" />}
            <p className="text-sm font-semibold truncate">{module.title}</p>
          </div>
          <p className="text-xs text-muted mt-1 leading-5">{module.current_owner}</p>
        </div>
        <Pill tone={statusTone} label={statusLabel} />
      </div>
      {canTakeover ? (
        <button
          className="btn-ghost mt-3 w-full flex items-center justify-center gap-2 py-2"
          onClick={() => onTakeover(module)}
          disabled={loading}
        >
          {loading ? <Loader2 size={14} className="animate-spin" /> : <Zap size={14} />}
          {loading ? '接管中...' : (module.action?.label || '让 ClashForge 接管')}
        </button>
      ) : null}
    </div>
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

export function Dashboard() {
  const { currentUp, currentDown, connCount, coreState, setCoreState, pushTraffic, setConnCount } = useStore()

  const [coreData, setCoreData] = useState<OverviewCoreData | null>(null)
  const [probeData, setProbeData] = useState<OverviewProbeData | null>(null)
  const [resourceData, setResourceData] = useState<OverviewResourceData | null>(null)

  const [queryingCore, setQueryingCore] = useState(true)
  const [section, setSection] = useState<SectionKey | null>(null)
  const [loadingSection, setLoadingSection] = useState<SectionKey | null>(null)
  const [loadingAction, setLoadingAction] = useState<string | null>(null)
  const [notice, setNotice] = useState<NoticeState | null>(null)

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

  const refreshProbes = async () => {
    setLoadingSection('probes')
    const next = await getOverviewProbes().catch(() => null)
    if (next) setProbeData(next)
    setLoadingSection(null)
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
    return () => {
      clearTimeout(bootstrap)
      clearInterval(timer)
    }
  }, [refreshCore])

  const openSection = async (target: SectionKey) => {
    setSection(target)
    if (target === 'probes' && !probeData) {
      await refreshProbes()
      return
    }
    if (target === 'resources' && !resourceData) {
      await refreshResources()
    }
  }

  const performAction = async (name: string, call: () => Promise<void>) => {
    setLoadingAction(name)
    setNotice(null)
    try {
      await call()
    } catch (error) {
      const message = error instanceof Error ? error.message : '操作失败'
      setNotice({ tone: 'error', message })
    } finally {
      setLoadingAction(null)
      await refreshCore(true)
    }
  }

  const allStopServices = useMemo(() => {
    const fromModules = (coreData?.modules ?? []).flatMap((item) => item.action?.stop_services ?? [])
    const fromInfluences = (coreData?.influences ?? [])
      .filter((item) => item.running && item.stoppable && item.service)
      .map((item) => item.service as string)
    return Array.from(new Set([...fromModules, ...fromInfluences]))
  }, [coreData])

  const handleTakeoverAll = async () => {
    await performAction('takeover:all', async () => {
      const result = await takeoverOverviewModule({ module: 'all', stop_services: allStopServices })
      setCoreData(result.overview)
      setCoreState(result.overview.core.state, result.overview.core.pid)
      const suffix = result.stopped?.length ? `；已停止 ${result.stopped.join('、')}` : ''
      setNotice({ tone: 'success', message: `${result.message}${suffix}` })
    })
  }

  const handleTakeoverModule = async (module: OverviewModule) => {
    if (!module.action || module.managed_by_clashforge) return
    await performAction(`takeover:${module.id}`, async () => {
      const result = await takeoverOverviewModule({
        module: module.action!.module,
        mode: module.action!.mode,
        stop_services: module.action!.stop_services,
      })
      setCoreData(result.overview)
      setCoreState(result.overview.core.state, result.overview.core.pid)
      const suffix = result.stopped?.length ? `；已停止 ${result.stopped.join('、')}` : ''
      setNotice({ tone: 'success', message: `${result.message}${suffix}` })
    })
  }

  const runCoreAction = async (name: string, action: () => Promise<unknown>, message: string) => {
    await performAction(name, async () => {
      await action()
      setNotice({ tone: 'success', message })
    })
  }

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

  const effectiveState = queryingCore && !coreData ? 'querying' : (coreData?.core.state || coreState || 'unknown')
  const coreRunning = effectiveState === 'running'

  return (
    <div className="p-6 space-y-6 max-w-7xl mx-auto">
      <div className="card px-6 py-6">
        <div className="flex items-start justify-between gap-4 flex-wrap">
          <div>
            <p className="text-[11px] uppercase tracking-[0.24em] text-muted">Overview</p>
            <h1 className="text-2xl font-semibold text-white mt-2">核心与接管控制台</h1>
            <p className="text-sm text-muted mt-2 max-w-3xl leading-6">
              首屏只展示核心状态与 DNS / NFT / 透明代理接管操作，联网探测和资源统计按需加载，避免首页卡顿。
            </p>
          </div>
          <div className="flex items-center gap-2">
            <CoreStateBadge state={effectiveState} />
            <button
              className="btn-ghost flex items-center gap-2"
              onClick={() => { void refreshCore(false) }}
              disabled={loadingAction === 'refresh:core' || queryingCore}
            >
              <RefreshCw size={14} className={queryingCore ? 'animate-spin' : ''} />
              刷新核心状态
            </button>
          </div>
        </div>

        <div className="grid grid-cols-2 xl:grid-cols-4 gap-3 mt-6">
          <MetricTile icon={<Activity size={16} />} label="上传速率" value={formatBytes(currentUp)} hint="实时上行" />
          <MetricTile icon={<Activity size={16} />} label="下载速率" value={formatBytes(currentDown)} hint="实时下行" />
          <MetricTile icon={<CheckCircle2 size={16} />} label="活跃连接" value={`${connCount}`} hint="当前连接数" />
          <MetricTile icon={<Cpu size={16} />} label="核心运行时长" value={coreData ? formatUptime(coreData.core.uptime) : '--'} hint={`PID ${coreData?.core.pid || '--'}`} />
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-5 gap-3 mt-5">
          {!coreRunning ? (
            <button
              className="btn-primary xl:col-span-2 flex items-center justify-center gap-2 py-3"
              onClick={() => { void runCoreAction('start', startCore, '核心已启动') }}
              disabled={!!loadingAction}
            >
              {loadingAction === 'start' ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} />}
              {loadingAction === 'start' ? '启动中...' : '启动核心'}
            </button>
          ) : (
            <button
              className="btn-danger xl:col-span-2 flex items-center justify-center gap-2 py-3"
              onClick={() => { void runCoreAction('stop', stopCore, '核心已停止') }}
              disabled={!!loadingAction}
            >
              {loadingAction === 'stop' ? <Loader2 size={14} className="animate-spin" /> : <Square size={14} />}
              {loadingAction === 'stop' ? '停止中...' : '停止核心'}
            </button>
          )}

          <button
            className="btn-ghost flex items-center justify-center gap-2 py-3"
            onClick={() => { void runCoreAction('restart', restartCore, '核心已重启') }}
            disabled={!!loadingAction}
          >
            {loadingAction === 'restart' ? <Loader2 size={14} className="animate-spin" /> : <RefreshCw size={14} />}
            重启
          </button>
          <button
            className="btn-ghost flex items-center justify-center gap-2 py-3"
            onClick={() => { void runCoreAction('reload', reloadCore, '配置已热重载') }}
            disabled={!!loadingAction}
          >
            {loadingAction === 'reload' ? <Loader2 size={14} className="animate-spin" /> : <Zap size={14} />}
            热重载
          </button>
          <button
            className="btn-primary flex items-center justify-center gap-2 py-3"
            onClick={() => { void handleTakeoverAll() }}
            disabled={!!loadingAction}
          >
            {loadingAction === 'takeover:all' ? <Loader2 size={14} className="animate-spin" /> : <ShieldCheck size={14} />}
            一键启动并接管全部
          </button>
        </div>
      </div>

      <NoticeBanner notice={notice} />

      <div className="card px-5 py-5">
        <div className="flex items-end justify-between gap-4 flex-wrap">
          <div>
            <p className="text-[11px] uppercase tracking-[0.24em] text-muted">Modules</p>
            <h2 className="text-xl font-semibold text-white mt-2">子模块运行与接管状态</h2>
            <p className="text-sm text-muted mt-2 leading-6">
              展示透明代理、NFT、防火墙、DNS 入口和 DNS 解析引擎的当前状态。支持逐项接管或一键接管。
            </p>
          </div>
          <p className="text-xs text-muted">{coreData?.checked_at ? `更新于 ${new Date(coreData.checked_at).toLocaleTimeString()}` : '等待查询结果'}</p>
        </div>

        <div className="grid grid-cols-1 xl:grid-cols-2 gap-3 mt-5">
          {visibleModules.map((module) => (
            <ModuleRow
              key={module.id}
              module={module}
              loading={loadingAction === `takeover:${module.id}`}
              onTakeover={handleTakeoverModule}
            />
          ))}
          {!visibleModules.length && (
            <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-5 text-sm text-muted">
              正在查询模块状态...
            </div>
          )}
        </div>
      </div>

      <div className="card px-5 py-5">
        <div className="flex items-end justify-between gap-4 flex-wrap">
          <div>
            <p className="text-[11px] uppercase tracking-[0.24em] text-muted">On-demand Data</p>
            <h2 className="text-xl font-semibold text-white mt-2">按需加载检测信息</h2>
            <p className="text-sm text-muted mt-2 leading-6">
              联网检测和资源采样不再首屏自动执行，点击后再加载，减少首页请求和等待时间。
            </p>
          </div>
          <div className="flex items-center gap-2">
            <button
              className={`btn-ghost ${section === 'probes' ? 'border-brand/40 text-white' : ''}`}
              onClick={() => { void openSection('probes') }}
            >
              出口 IP / 访问检查
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
          <div className="rounded-2xl border border-dashed border-white/15 bg-black/10 px-4 py-6 mt-5 text-sm text-muted">
            请选择上面的一个模块开始加载。
          </div>
        ) : null}

        {section === 'probes' ? (
          <div className="mt-5 space-y-4">
            <div className="flex items-center justify-between gap-3">
              <p className="text-sm text-muted">通过 mixed 端口进行出口识别和可达性探测。</p>
              <button className="btn-ghost flex items-center gap-2" onClick={() => { void refreshProbes() }} disabled={loadingSection === 'probes'}>
                <RefreshCw size={14} className={loadingSection === 'probes' ? 'animate-spin' : ''} />
                重新检测
              </button>
            </div>

            {!probeData ? (
              <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-6 text-sm text-muted">
                {loadingSection === 'probes' ? '正在进行联网检测...' : '点击“重新检测”开始加载出口 IP 与访问检查。'}
              </div>
            ) : (
              <>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                  {probeData.ip_checks.map((item) => <IPCard key={item.provider} item={item} />)}
                </div>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                  {probeData.access_checks.map((item) => <AccessCard key={item.name} item={item} />)}
                </div>
              </>
            )}
          </div>
        ) : null}

        {section === 'resources' ? (
          <div className="mt-5 space-y-4">
            <div className="flex items-center justify-between gap-3">
              <p className="text-sm text-muted">查看系统资源与 ClashForge 占用，包括规则文件空间。</p>
              <button className="btn-ghost flex items-center gap-2" onClick={() => { void refreshResources() }} disabled={loadingSection === 'resources'}>
                <RefreshCw size={14} className={loadingSection === 'resources' ? 'animate-spin' : ''} />
                刷新资源
              </button>
            </div>

            {!resourceData ? (
              <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-6 text-sm text-muted">
                {loadingSection === 'resources' ? '正在采样系统与进程资源...' : '点击“刷新资源”开始加载资源占用数据。'}
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
                    <div>
                      <p className="text-[11px] uppercase tracking-[0.16em] text-muted">运行目录</p>
                      <p className="text-slate-200 mt-1">{formatMB(resourceData.resources.app.runtime_mb)}</p>
                    </div>
                    <div>
                      <p className="text-[11px] uppercase tracking-[0.16em] text-muted">数据目录</p>
                      <p className="text-slate-200 mt-1">{formatMB(resourceData.resources.app.data_mb)}</p>
                    </div>
                    <div>
                      <p className="text-[11px] uppercase tracking-[0.16em] text-muted">程序文件</p>
                      <p className="text-slate-200 mt-1">{formatMB(resourceData.resources.app.binary_mb)}</p>
                    </div>
                    <div>
                      <p className="text-[11px] uppercase tracking-[0.16em] text-muted">规则文件</p>
                      <p className="text-slate-200 mt-1">{formatMB(resourceData.resources.app.rules_mb)}</p>
                    </div>
                    <div>
                      <p className="text-[11px] uppercase tracking-[0.16em] text-muted">总占用</p>
                      <p className="text-slate-200 mt-1">{formatMB(resourceData.resources.app.total_mb)}</p>
                    </div>
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
