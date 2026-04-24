import { useEffect, useState } from 'react'
import { getConfig, updateConfig, resetClashForge } from '../api/client'
import { Save, Loader2, RotateCcw, ShieldAlert } from 'lucide-react'
import { InlineNotice, ModalShell, PageHeader, SectionCard } from '../components/ui'

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
    <div className="space-y-6">
      <PageHeader
        eyebrow="System Cat Box"
        title="高级管理与系统猫箱"
        description="调整 ClashForge 的核心路径、日志策略与系统级动作。保留工程师掌控感，同时让危险操作与系统反馈拥有更清晰、更友好的猫系表达。"
        actions={
          <button className="btn-primary flex items-center gap-2" onClick={saveGeneral} disabled={saving}>
            <Save size={14} /> {saving ? '保存中…' : '保存配置'}
          </button>
        }
        metrics={[
          { label: '保存状态', value: saved ? '已保存' : saving ? '进行中' : '待提交' },
          { label: '危险区', value: '受保护' },
        ]}
      />

      {saveError && (
        <InlineNotice tone="danger" title="保存失败">
          {saveError}
        </InlineNotice>
      )}
      {saved && (
        <InlineNotice tone="success" title="保存成功">
          配置已写入 ClashForge，可继续进行其他调整。
        </InlineNotice>
      )}

      <SectionCard title="核心参数" description="用于管理 mihomo 启动路径、重启策略与日志级别。">
        <div className="space-y-4">
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
                { value: 'info', label: 'Info' },
                { value: 'warn', label: 'Warn' },
                { value: 'error', label: 'Error' },
              ]}
            />
          </Field>
        </div>
      </SectionCard>

      <SectionCard title="危险操作" description="以下行为会影响配置资产与运行态，请谨慎处理。" className="border-danger/20">
        <div className="space-y-4">
          {dangerResult && (
            <InlineNotice tone={dangerResult.ok ? 'success' : 'danger'} title={dangerResult.ok ? '操作完成' : '操作失败'}>
              <div className="space-y-2">
                <p className="whitespace-pre-wrap text-sm leading-6">{dangerResult.message}</p>
                {resetDone ? (
                  <p className="text-xs text-muted">
                    请等待约 3 秒后刷新页面。
                    <button className="ml-2 text-brand hover:underline" onClick={() => window.location.reload()}>立即刷新</button>
                  </p>
                ) : null}
              </div>
            </InlineNotice>
          )}

          <div className="rounded-[24px] border border-danger/15 bg-[linear-gradient(180deg,rgba(239,68,68,0.12),rgba(239,68,68,0.05))] p-5">
            <div className="flex items-start gap-3">
              <div className="rounded-2xl border border-danger/20 bg-danger/10 p-2.5 text-danger">
                <ShieldAlert size={18} />
              </div>
              <div className="flex-1">
                <p className="text-base font-semibold text-white">重置 ClashForge</p>
                <p className="mt-2 text-sm leading-6 text-[#E8C5C5]">
                  清除所有订阅、配置覆盖与生成文件，恢复为出厂默认设置，并自动重启进程。页面会短暂断开连接。
                </p>
              </div>
            </div>
            <div className="mt-4 flex flex-wrap gap-3">
              <button
                className="btn-danger flex items-center gap-2"
                onClick={() => { setDangerConfirm('reset'); setDangerResult(null) }}
                disabled={dangerRunning}
              >
                <RotateCcw size={13} /> 重置为出厂状态
              </button>
            </div>
          </div>
        </div>
      </SectionCard>

      {dangerConfirm && (
        <ModalShell
          title="确认重置 ClashForge"
          description="该操作将清除所有订阅、配置覆盖与生成配置，并自动重启 ClashForge 进程。"
          icon={<RotateCcw size={16} />}
          onClose={() => !dangerRunning && setDangerConfirm(null)}
          size="sm"
          dismissible={!dangerRunning}
        >
          <div className="space-y-5">
            <p className="text-sm leading-6 text-muted">
              重启后你需要重新运行配置向导。若当前正在排查问题，建议先备份现有配置再继续。
            </p>
            <div className="flex gap-3 pt-1">
              <button className="btn-ghost flex-1" onClick={() => setDangerConfirm(null)} disabled={dangerRunning}>
                取消
              </button>
              <button
                className="btn-danger flex-1 justify-center"
                onClick={() => handleDangerAction(dangerConfirm)}
                disabled={dangerRunning}
              >
                {dangerRunning
                  ? <><Loader2 size={13} className="animate-spin" />执行中…</>
                  : <><RotateCcw size={13} />确认重置</>}
              </button>
            </div>
          </div>
        </ModalShell>
      )}
    </div>
  )
}
