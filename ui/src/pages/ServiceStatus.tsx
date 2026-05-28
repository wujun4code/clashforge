import { useCallback, useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Activity, AlertCircle, Loader2, PowerOff, RefreshCw, ShieldOff,
} from 'lucide-react'
import {
  getOverviewCore, getMihomoConfig, stopCore, releaseOverviewTakeover,
} from '../api/client'
import type { OverviewModule } from '../api/client'

export function ServiceStatus() {
  const navigate = useNavigate()
  const [modules, setModules] = useState<OverviewModule[]>([])
  const [stopping, setStopping] = useState(false)
  const [stopError, setStopError] = useState('')
  const [configYaml, setConfigYaml] = useState('')
  const [configLoading, setConfigLoading] = useState(false)
  const [configError, setConfigError] = useState('')

  const loadConfig = useCallback(async () => {
    setConfigLoading(true)
    setConfigError('')
    try {
      const { content } = await getMihomoConfig()
      setConfigYaml(content)
    } catch (e) {
      setConfigError(e instanceof Error ? e.message : '读取配置失败')
    } finally {
      setConfigLoading(false)
    }
  }, [])

  useEffect(() => {
    getOverviewCore()
      .then(data => setModules(data.modules ?? []))
      .catch(() => {})
    void loadConfig()
  }, [loadConfig])

  const handleStop = async () => {
    setStopping(true)
    setStopError('')
    try {
      await stopCore().catch(() => null)
      await releaseOverviewTakeover()
      navigate('/setup')
    } catch (e) {
      setStopError(e instanceof Error ? e.message : '停止失败')
    } finally {
      setStopping(false)
    }
  }

  const managed = modules.filter(m => m.managed_by_clashforge)

  return (
    <div className="min-h-full bg-gradient-to-b from-surface-0 to-surface-1 px-6 py-8">
      <div className="mx-auto max-w-6xl space-y-6">

        {/* Header */}
        <div className="flex items-center gap-3">
          <div className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl bg-emerald-500/20">
            <Activity size={18} className="text-emerald-400" />
          </div>
          <div>
            <p className="text-[11px] uppercase tracking-[0.24em] text-white/35">Proxy · Service</p>
            <h1 className="mt-0.5 text-base font-bold text-white">代理服务</h1>
            <p className="text-xs text-white/40">服务正在运行，停止后可重新配置</p>
          </div>
        </div>

        <div className="grid gap-6 xl:grid-cols-[420px_minmax(0,1fr)]">

          {/* Left: status card + stop */}
          <div className="glass-card px-5 py-5 space-y-4">
            <div className="flex items-center gap-3">
              <span className="inline-flex h-2.5 w-2.5 rounded-full bg-emerald-400 animate-pulse" />
              <p className="text-sm font-semibold text-white">内核正在运行</p>
            </div>
            <p className="text-sm text-white/50 leading-6">
              ClashForge 内核当前处于运行状态，并已接管以下系统服务。
              要重新配置，请先停止内核并退出所有接管，然后再继续。
            </p>

            {managed.length > 0 && (
              <div className="rounded-xl border border-white/[0.08] bg-black/10 px-4 py-3 space-y-2">
                <p className="text-xs text-white/30 uppercase tracking-wider font-semibold">当前已接管</p>
                {managed.map(m => (
                  <div key={m.id} className="flex items-center gap-2 text-xs">
                    <ShieldOff size={12} className="text-amber-400" />
                    <span className="text-white/80">{m.title}</span>
                    <span className="text-white/35">— {m.current_owner}</span>
                  </div>
                ))}
              </div>
            )}

            {stopError && (
              <div className="flex items-center gap-2 text-xs text-rose-400">
                <AlertCircle size={13} />{stopError}
              </div>
            )}

            <button
              className="w-full flex items-center justify-center gap-2.5 py-4 text-sm font-bold rounded-xl
                         bg-red-500/90 hover:bg-red-500 text-white border border-red-500/60 hover:border-red-500
                         shadow-lg shadow-red-500/20 transition-all active:scale-[0.98] disabled:opacity-60"
              onClick={handleStop}
              disabled={stopping}
            >
              {stopping
                ? <><Loader2 size={16} className="animate-spin" />停止中…</>
                : <><PowerOff size={16} />停止内核 + 退出所有接管</>}
            </button>
          </div>

          {/* Right: running config preview */}
          <div className="glass-card px-5 py-5 space-y-4">
            <div className="flex items-start justify-between gap-3">
              <div>
                <h2 className="text-sm font-semibold text-white/90">正在运行配置</h2>
                <p className="mt-1 text-xs text-white/40">当前完整运行配置（mihomo-config.yaml）</p>
              </div>
              <button
                className="btn-ghost text-xs flex items-center gap-1.5"
                onClick={loadConfig}
                disabled={configLoading}
              >
                <RefreshCw size={12} className={configLoading ? 'animate-spin' : ''} />
                {configLoading ? '刷新中…' : '刷新'}
              </button>
            </div>

            {configLoading && (
              <div className="flex items-center gap-2 text-sm text-white/40 py-2">
                <Loader2 size={14} className="animate-spin text-brand" />
                正在读取运行配置…
              </div>
            )}
            {!configLoading && configError && (
              <div className="rounded-xl border border-rose-500/30 bg-rose-500/10 px-3 py-2 text-xs text-rose-300">
                {configError}
              </div>
            )}
            {!configLoading && !configError && (
              <div className="rounded-xl bg-black/30 border border-white/[0.08] overflow-auto max-h-[72rem] text-xs font-mono select-text">
                {configYaml
                  ? configYaml.split('\n').map((line, i) => (
                      <div key={i} className="flex items-start gap-2 px-2 py-px leading-5">
                        <span className="select-none text-white/20 w-7 flex-shrink-0 text-right tabular-nums">{i + 1}</span>
                        <span className="flex-1 text-white/70 whitespace-pre">{line || ' '}</span>
                      </div>
                    ))
                  : <p className="px-3 py-3 text-white/30">暂无运行配置</p>
                }
              </div>
            )}
          </div>

        </div>
      </div>
    </div>
  )
}
