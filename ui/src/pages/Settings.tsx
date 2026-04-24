import { useEffect, useState } from 'react'
import { getConfig, updateConfig, resetClashForge } from '../api/client'
import { Save, Loader2, RotateCcw, ShieldAlert } from 'lucide-react'
import { InlineNotice, ModalShell, PageHeader, SectionCard } from '../components/ui'

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div className="flex-shrink-0 w-44">
        <label className="font-mono text-[10px] uppercase tracking-[0.15em] text-muted">{label}</label>
        {hint && <p className="font-mono text-[9px] text-muted mt-0.5 leading-4">{hint}</p>}
      </div>
      <div className="flex-1">{children}</div>
    </div>
  )
}

function TextInput({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <input
      className="glass-input"
      value={value}
      onChange={e => onChange(e.target.value)}
    />
  )
}

function SelectInput({ value, onChange, options }: { value: string; onChange: (v: string) => void; options: { value: string; label: string }[] }) {
  return (
    <select
      className="glass-input"
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

  if (!cfg) return (
    <div className="p-6 font-mono text-[10px] text-muted uppercase tracking-[0.15em]">
      LOADING_CONFIG...
    </div>
  )

  return (
    <div className="space-y-5">
      <PageHeader
        eyebrow="SYS_ADMIN"
        title="高级管理与系统行为"
        description="CORE_PARAMS · LOG_POLICY · SYSTEM_ACTIONS · FACTORY_RESET"
        actions={
          <button className="btn-primary btn-sm flex items-center gap-1.5" onClick={saveGeneral} disabled={saving}>
            <Save size={12} /> {saving ? 'SAVING...' : 'SAVE_CFG'}
          </button>
        }
        metrics={[
          { label: 'SAVE_STATUS', value: saved ? 'COMMITTED' : saving ? 'WRITING' : 'PENDING', color: saved ? 'green' : saving ? 'yellow' : 'cyan' },
          { label: 'DANGER_ZONE', value: 'PROTECTED', color: 'red' },
        ]}
      />

      {saveError && (
        <InlineNotice tone="danger" title="SAVE_FAILED">{saveError}</InlineNotice>
      )}
      {saved && (
        <InlineNotice tone="success" title="COMMITTED">
          CONFIG_WRITTEN — changes applied to ClashForge
        </InlineNotice>
      )}

      <SectionCard title="CORE_PARAMS" description="MIHOMO_PATH · RESTART_POLICY · LOG_LEVEL">
        <div className="space-y-4">
          <Field label="MIHOMO_PATH">
            <TextInput value={get(['core', 'binary']) as string} onChange={v => set(['core', 'binary'], v)} />
          </Field>
          <Field label="MAX_RESTARTS">
            <TextInput value={String(get(['core', 'max_restarts']) ?? 3)} onChange={v => set(['core', 'max_restarts'], parseInt(v) || 3)} />
          </Field>
          <Field label="LOG_LEVEL">
            <SelectInput
              value={get(['log', 'level']) as string || 'info'}
              onChange={v => set(['log', 'level'], v)}
              options={[
                { value: 'debug', label: 'DEBUG' },
                { value: 'info', label: 'INFO' },
                { value: 'warn', label: 'WARN' },
                { value: 'error', label: 'ERROR' },
              ]}
            />
          </Field>
        </div>
      </SectionCard>

      <SectionCard title="DANGER_ZONE" description="DESTRUCTIVE_OPERATIONS — handle with care" accent="red">
        <div className="space-y-4">
          {dangerResult && (
            <InlineNotice tone={dangerResult.ok ? 'success' : 'danger'} title={dangerResult.ok ? 'OP_COMPLETE' : 'OP_FAILED'}>
              <div className="space-y-2">
                <p className="font-mono text-[10px] whitespace-pre-wrap leading-5">{dangerResult.message}</p>
                {resetDone ? (
                  <p className="font-mono text-[10px] text-muted">
                    WAIT ~3s then refresh.{' '}
                    <button
                      className="cursor-pointer"
                      style={{ color: '#00F5FF', textShadow: '0 0 6px rgba(0,245,255,0.5)' }}
                      onClick={() => window.location.reload()}
                    >
                      RELOAD_NOW
                    </button>
                  </p>
                ) : null}
              </div>
            </InlineNotice>
          )}

          <div
            className="p-5"
            style={{
              border: '1px solid rgba(255,34,85,0.2)',
              background: 'linear-gradient(135deg, rgba(255,34,85,0.06), rgba(6,12,18,0.8))',
              clipPath: 'polygon(0 0, calc(100% - 10px) 0, 100% 10px, 100% 100%, 10px 100%, 0 calc(100% - 10px))',
            }}
          >
            <div className="flex items-start gap-3">
              <div
                className="p-2.5 flex-shrink-0"
                style={{ border: '1px solid rgba(255,34,85,0.3)', background: 'rgba(255,34,85,0.08)', color: '#FF2255' }}
              >
                <ShieldAlert size={16} />
              </div>
              <div className="flex-1">
                <p className="font-mono text-sm font-bold uppercase tracking-[0.06em]" style={{ color: '#FF2255', textShadow: '0 0 8px rgba(255,34,85,0.6)' }}>
                  FACTORY_RESET
                </p>
                <p className="font-mono text-[10px] text-muted mt-1.5 leading-5">
                  Clears all subscriptions, config overrides, and generated files. Restores defaults and auto-restarts process. Brief disconnect expected.
                </p>
              </div>
            </div>
            <div className="mt-4 flex flex-wrap gap-3">
              <button
                className="btn-danger flex items-center gap-2"
                onClick={() => { setDangerConfirm('reset'); setDangerResult(null) }}
                disabled={dangerRunning}
              >
                <RotateCcw size={12} /> FACTORY_RESET
              </button>
            </div>
          </div>
        </div>
      </SectionCard>

      {dangerConfirm && (
        <ModalShell
          title="CONFIRM_FACTORY_RESET"
          description="All subscriptions, config overrides, and generated configs will be wiped. ClashForge will auto-restart."
          icon={<RotateCcw size={14} />}
          onClose={() => !dangerRunning && setDangerConfirm(null)}
          size="sm"
          dismissible={!dangerRunning}
        >
          <div className="space-y-5">
            <p className="font-mono text-[10px] leading-5 text-muted">
              // You will need to re-run the setup wizard after reset. Backup your config if troubleshooting.
            </p>
            <div className="flex gap-3 pt-1">
              <button className="btn-secondary btn-sm flex-1" onClick={() => setDangerConfirm(null)} disabled={dangerRunning}>
                CANCEL
              </button>
              <button
                className="btn-danger btn-sm flex-1 justify-center"
                onClick={() => handleDangerAction(dangerConfirm)}
                disabled={dangerRunning}
              >
                {dangerRunning
                  ? <><Loader2 size={12} className="animate-spin" />EXECUTING...</>
                  : <><RotateCcw size={12} />CONFIRM_RESET</>}
              </button>
            </div>
          </div>
        </ModalShell>
      )}
    </div>
  )
}
