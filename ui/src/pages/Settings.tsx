import { useEffect, useState } from 'react'
import { getConfig, updateConfig, resetClashForge } from '../api/client'
import { Save, AlertCircle, CheckCircle2, Loader2, RotateCcw, Terminal, SlidersHorizontal } from 'lucide-react'

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div className="flex-shrink-0 w-44">
        <label className="text-sm text-slate-300">{label}</label>
        {hint && <p className="text-xs text-muted mt-0.5 leading-4">{hint}</p>}
      </div>
      <div className="flex-1">{children}</div>
    </div>
  )
}

export function Settings() {
  const [cfg, setCfg] = useState<Record<string, unknown> | null>(null)
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [saveError, setSaveError] = useState('')

  type DangerTarget = 'reset'
  const [dangerConfirm, setDangerConfirm] = useState<DangerTarget | null>(null)
  const [dangerRunning, setDangerRunning] = useState(false)
  const [dangerResult, setDangerResult] = useState<{ ok: boolean; message: string } | null>(null)
  const [resetDone, setResetDone] = useState(false)

  useEffect(() => { getConfig().then(setCfg).catch(() => null) }, [])

  const set = (path: string[], value: unknown) => {
    if (!cfg) return
    const updated = JSON.parse(JSON.stringify(cfg))
    let cur = updated
    for (let i = 0; i < path.length - 1; i++) cur = cur[path[i]] as Record<string, unknown>
    cur[path[path.length - 1]] = value
    setCfg(updated)
  }

  const get = (path: string[]): unknown => {
    if (!cfg) return ''
    let cur = cfg as Record<string, unknown>
    for (const k of path) cur = (cur[k] ?? '') as Record<string, unknown>
    return cur
  }

  const saveGeneral = async () => {
    if (!cfg) return
    setSaving(true); setSaveError('')
    try {
      await updateConfig(cfg)
      setSaved(true)
      setTimeout(() => setSaved(false), 2500)
    } catch (e: unknown) {
      setSaveError(String(e))
    } finally { setSaving(false) }
  }

  const handleDangerAction = async (_target: DangerTarget) => {
    setDangerRunning(true); setDangerResult(null)
    try {
      const res = await resetClashForge()
      setDangerResult({ ok: res.ok, message: res.message })
      if (res.ok) setResetDone(true)
    } catch (e: unknown) {
      setDangerResult({ ok: false, message: e instanceof Error ? e.message : String(e) })
    } finally { setDangerRunning(false); setDangerConfirm(null) }
  }

  if (!cfg) return (
    <div className="p-6 flex items-center justify-center h-64">
      <div className="flex items-center gap-3 text-muted">
        <Loader2 size={16} className="animate-spin" />
        <span className="text-sm">加载配置中…</span>
      </div>
    </div>
  )

  return (
    <div className="p-6 space-y-5 max-w-3xl mx-auto">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="w-8 h-8 rounded-xl bg-brand/15 border border-brand/20 flex items-center justify-center">
            <SlidersHorizontal size={15} className="text-brand" />
          </div>
          <h1 className="text-lg font-semibold text-white">高级管理</h1>
        </div>
        <div className="flex items-center gap-2">
          {saveError && (
            <span className="flex items-center gap-1.5 text-xs text-danger bg-danger/10 border border-danger/20 px-3 py-1.5 rounded-lg">
              <AlertCircle size={12} />{saveError}
            </span>
          )}
          {saved && (
            <span className="flex items-center gap-1.5 text-xs text-success bg-success/10 border border-success/20 px-3 py-1.5 rounded-lg animate-fade-in">
              <CheckCircle2 size={12} /> 已保存
            </span>
          )}
        </div>
      </div>

      {/* Core settings */}
      <div className="space-y-4">
        <div className="card px-5 py-5 space-y-5">
          <div className="flex items-center gap-2 pb-3 border-b border-white/[0.06]">
            <div className="w-1.5 h-4 rounded-full bg-brand" style={{ boxShadow: '0 0 8px rgba(6,182,212,0.6)' }} />
            <h2 className="text-sm font-semibold text-slate-200">核心配置</h2>
          </div>
          <Field label="mihomo 路径">
            <input
              className="input font-mono text-xs"
              value={get(['core', 'binary']) as string}
              onChange={e => set(['core', 'binary'], e.target.value)}
            />
          </Field>
          <Field label="最大重启次数">
            <input
              className="input"
              value={String(get(['core', 'max_restarts']) ?? 3)}
              onChange={e => set(['core', 'max_restarts'], parseInt(e.target.value) || 3)}
            />
          </Field>
          <Field label="日志级别">
            <select
              className="select"
              value={get(['log', 'level']) as string || 'info'}
              onChange={e => set(['log', 'level'], e.target.value)}
            >
              <option value="debug">Debug</option>
              <option value="info">Info</option>
              <option value="warn">Warn</option>
              <option value="error">Error</option>
            </select>
          </Field>
        </div>

        <button className="btn-primary flex items-center gap-2" onClick={saveGeneral} disabled={saving}>
          {saving ? <><Loader2 size={14} className="animate-spin" />保存中…</> : <><Save size={14} />保存配置</>}
        </button>
      </div>

      {/* Danger zone */}
      <div className="card px-5 py-5 space-y-4 border-danger/15">
        <div className="flex items-center gap-2 pb-3 border-b border-danger/10">
          <div className="w-1.5 h-4 rounded-full bg-danger" style={{ boxShadow: '0 0 8px rgba(244,63,94,0.5)' }} />
          <Terminal size={14} className="text-danger" />
          <h2 className="text-sm font-semibold text-danger">危险操作</h2>
        </div>

        {dangerResult && (
          <div className={`rounded-xl border px-4 py-3 space-y-2 animate-slide-in ${
            dangerResult.ok ? 'bg-success/[0.06] border-success/20' : 'bg-danger/[0.06] border-danger/20'
          }`}>
            <div className="flex items-center gap-2">
              {dangerResult.ok
                ? <CheckCircle2 size={14} className="text-success flex-shrink-0" />
                : <AlertCircle size={14} className="text-danger flex-shrink-0" />}
              <p className={`text-xs font-semibold ${dangerResult.ok ? 'text-success' : 'text-danger'}`}>
                {dangerResult.ok ? '操作完成' : '操作失败'}
              </p>
              {!resetDone && (
                <button className="ml-auto text-xs text-muted hover:text-white transition-colors cursor-pointer"
                  onClick={() => setDangerResult(null)}>关闭</button>
              )}
            </div>
            <p className="text-xs text-slate-400 leading-5 whitespace-pre-wrap">{dangerResult.message}</p>
            {resetDone && (
              <p className="text-xs text-muted">请等待约 3 秒后刷新页面…
                <button className="ml-2 text-brand hover:underline cursor-pointer" onClick={() => window.location.reload()}>立即刷新</button>
              </p>
            )}
          </div>
        )}

        <div className="rounded-2xl border border-danger/12 bg-danger/[0.03] px-4 py-4 space-y-3">
          <div>
            <p className="text-sm font-semibold text-slate-100">重置 ClashForge</p>
            <p className="text-xs text-muted mt-1 leading-5">
              清除所有订阅、配置覆盖、生成的配置，恢复出厂默认设置，自动重启进程。
              <span className="text-danger/80 font-medium"> 页面将短暂断开。</span>
            </p>
          </div>
          <button
            className="btn-danger w-full flex items-center justify-center gap-2 text-xs disabled:opacity-40"
            onClick={() => { setDangerConfirm('reset'); setDangerResult(null) }}
            disabled={dangerRunning}
          >
            <RotateCcw size={13} /> 重置为出厂状态
          </button>
        </div>
      </div>

      {/* Confirm modal */}
      {dangerConfirm && (
        <div className="fixed inset-0 bg-black/70 backdrop-blur-sm flex items-center justify-center z-50 p-4 animate-fade-in"
          onClick={() => !dangerRunning && setDangerConfirm(null)}>
          <div className="bg-surface-1 rounded-2xl border border-danger/20 w-full max-w-sm p-6 space-y-4 animate-slide-in"
            style={{ boxShadow: '0 0 40px rgba(244,63,94,0.1)' }}
            onClick={e => e.stopPropagation()}>
            <div className="flex items-start gap-3">
              <div className="w-10 h-10 rounded-xl bg-danger/15 border border-danger/20 flex items-center justify-center flex-shrink-0">
                <RotateCcw size={18} className="text-danger" />
              </div>
              <div className="flex-1">
                <h3 className="text-base font-semibold text-white">重置 ClashForge</h3>
                <p className="text-sm text-muted mt-1.5 leading-6">
                  将清除所有订阅、配置覆盖和生成的配置，恢复出厂默认设置，并自动重启进程。Web UI 会短暂断开，重启后需重新运行配置向导。
                </p>
              </div>
            </div>
            <div className="flex gap-3 pt-1">
              <button className="btn-ghost flex-1" onClick={() => setDangerConfirm(null)} disabled={dangerRunning}>取消</button>
              <button
                className="btn-danger flex-1 flex items-center justify-center gap-2"
                onClick={() => handleDangerAction(dangerConfirm)}
                disabled={dangerRunning}
              >
                {dangerRunning
                  ? <><Loader2 size={13} className="animate-spin" />执行中…</>
                  : <><RotateCcw size={13} />确认重置</>}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
