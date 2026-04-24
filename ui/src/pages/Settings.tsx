import { useEffect, useState } from 'react'
import { getConfig, updateConfig, resetClashForge } from '../api/client'
import { Save, AlertCircle, CheckCircle2, Loader2, RotateCcw, Terminal } from 'lucide-react'

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

function TextInput({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <input
      className="w-full bg-surface-2 border border-white/10 rounded-xl px-3 py-2 text-sm text-white outline-none focus:border-brand transition-colors"
      value={value}
      onChange={e => onChange(e.target.value)}
    />
  )
}

function SelectInput({ value, onChange, options }: { value: string; onChange: (v: string) => void; options: { value: string; label: string }[] }) {
  return (
    <select
      className="w-full bg-surface-2 border border-white/10 rounded-xl px-3 py-2 text-sm text-white outline-none focus:border-brand transition-colors appearance-none"
      value={value}
      onChange={e => onChange(e.target.value)}
    >
      {options.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
    </select>
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

  useEffect(() => {
    getConfig().then(setCfg).catch(() => null)
  }, [])

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
      setTimeout(() => setSaved(false), 2000)
    } catch (e: unknown) {
      setSaveError(String(e))
    } finally { setSaving(false) }
  }

  const handleDangerAction = async (_target: DangerTarget) => {
    setDangerRunning(true)
    setDangerResult(null)
    try {
      const res = await resetClashForge()
      setDangerResult({ ok: res.ok, message: res.message })
      if (res.ok) setResetDone(true)
    } catch (e: unknown) {
      setDangerResult({ ok: false, message: e instanceof Error ? e.message : String(e) })
    } finally {
      setDangerRunning(false)
      setDangerConfirm(null)
    }
  }

  if (!cfg) return <div className="p-6 text-muted text-sm">加载中…</div>

  return (
    <div className="p-6 space-y-5 max-w-3xl mx-auto">
      <div className="flex items-center justify-between">
        <div>
          <p className="text-[11px] uppercase tracking-[0.24em] text-muted">Settings</p>
          <h1 className="text-base font-semibold text-white mt-1">高级管理</h1>
        </div>
        <div className="flex items-center gap-2">
          {saveError && <span className="flex items-center gap-1 text-xs text-danger"><AlertCircle size={12} />{saveError}</span>}
          {saved && <span className="flex items-center gap-1 text-xs text-success"><CheckCircle2 size={12} /> 已保存</span>}
        </div>
      </div>

      {/* ── General ── */}
      <div className="space-y-4">
        <div className="glass-card px-5 py-5 space-y-4">
          <h2 className="text-sm font-semibold text-slate-200 border-b border-white/5 pb-3">核心</h2>
          <Field label="mihomo 路径">
            <TextInput value={get(['core', 'binary']) as string} onChange={v => set(['core', 'binary'], v)} />
          </Field>
          <Field label="最大重启次数">
            <TextInput value={String(get(['core', 'max_restarts']) ?? 3)} onChange={v => set(['core', 'max_restarts'], parseInt(v) || 3)} />
          </Field>
          <Field label="日志级别">
            <SelectInput
              value={get(['log', 'level']) as string || 'info'}
              onChange={v => set(['log', 'level'], v)}
              options={[
                { value: 'debug', label: 'Debug' },
                { value: 'info',  label: 'Info' },
                { value: 'warn',  label: 'Warn' },
                { value: 'error', label: 'Error' },
              ]}
            />
          </Field>
        </div>

        <button className="btn-primary flex items-center gap-2" onClick={saveGeneral} disabled={saving}>
          <Save size={14} /> {saving ? '保存中…' : '保存配置'}
        </button>
      </div>

      {/* ── Danger zone ── */}
      <div className="glass-card px-5 py-5 space-y-4 border-danger/20">
        <div className="flex items-center gap-2">
          <Terminal size={15} className="text-danger" />
          <h2 className="text-sm font-semibold text-danger">危险操作</h2>
        </div>

        {dangerResult && (
          <div className={`rounded-xl border px-4 py-3 space-y-2 ${dangerResult.ok ? 'bg-success/8 border-success/25' : 'bg-danger/8 border-danger/25'}`}>
            <div className="flex items-center gap-2">
              {dangerResult.ok
                ? <CheckCircle2 size={14} className="text-success flex-shrink-0" />
                : <AlertCircle size={14} className="text-danger flex-shrink-0" />}
              <p className={`text-xs font-semibold ${dangerResult.ok ? 'text-success' : 'text-danger'}`}>
                {dangerResult.ok ? '操作完成' : '操作失败'}
              </p>
              {!resetDone && (
                <button className="ml-auto text-xs text-muted hover:text-white" onClick={() => setDangerResult(null)}>关闭</button>
              )}
            </div>
            <p className="text-xs text-slate-400 leading-5 whitespace-pre-wrap">{dangerResult.message}</p>
            {resetDone && (
              <p className="text-xs text-muted">请等待约 3 秒后刷新页面…
                <button className="ml-2 text-brand hover:underline" onClick={() => window.location.reload()}>立即刷新</button>
              </p>
            )}
          </div>
        )}

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div className="rounded-2xl border border-danger/15 bg-black/10 px-4 py-4 space-y-3">
            <div>
              <p className="text-sm font-semibold text-slate-100">重置 ClashForge</p>
              <p className="text-xs text-muted mt-1 leading-5">
                清除所有订阅、配置覆盖、生成的配置，恢复出厂默认设置，自动重启进程。
                <span className="text-danger font-medium"> 页面将短暂断开。</span>
              </p>
            </div>
            <button
              className="w-full rounded-xl px-3 py-2 text-xs font-semibold border border-danger/30 bg-danger/10 text-danger hover:bg-danger/20 transition-all flex items-center justify-center gap-2 disabled:opacity-50"
              onClick={() => { setDangerConfirm('reset'); setDangerResult(null) }}
              disabled={dangerRunning}
            >
              <RotateCcw size={12} /> 重置为出厂状态
            </button>
          </div>
        </div>
      </div>

      {/* ── Confirm dialog ── */}
      {dangerConfirm && (
        <div
          className="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50 p-4"
          onClick={() => !dangerRunning && setDangerConfirm(null)}
        >
          <div
            className="bg-surface-1 rounded-2xl border border-white/10 w-full max-w-sm p-6 space-y-4"
            onClick={e => e.stopPropagation()}
          >
            <div className="flex items-start gap-3">
              <div className="w-9 h-9 rounded-xl flex items-center justify-center flex-shrink-0 bg-danger/15">
                <RotateCcw size={16} className="text-danger" />
              </div>
              <div className="flex-1">
                <h3 className="text-base font-semibold text-white">重置 ClashForge</h3>
                <p className="text-sm text-muted mt-1.5 leading-6">
                  将清除所有订阅、配置覆盖和生成的配置，恢复出厂默认设置，并自动重启 ClashForge 进程。Web UI 会短暂断开，重启后需重新运行配置向导。
                </p>
              </div>
            </div>

            <div className="flex gap-3 pt-1">
              <button className="btn-ghost flex-1" onClick={() => setDangerConfirm(null)} disabled={dangerRunning}>
                取消
              </button>
              <button
                className="flex-1 rounded-xl px-4 py-2 text-sm font-medium transition-all disabled:opacity-50 flex items-center justify-center gap-2 bg-danger/20 text-danger border border-danger/30 hover:bg-danger/30"
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
