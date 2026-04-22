import { useEffect, useState } from 'react'
import { getConfig, updateConfig, getOverrides, updateOverrides } from '../api/client'
import { Save } from 'lucide-react'

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4">
      <label className="text-sm text-muted flex-shrink-0 w-40">{label}</label>
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

function SelectInput({ value, onChange, options }: { value: string; onChange: (v: string) => void; options: string[] }) {
  return (
    <select
      className="w-full bg-surface-2 border border-white/10 rounded-xl px-3 py-2 text-sm text-white outline-none focus:border-brand transition-colors appearance-none"
      value={value}
      onChange={e => onChange(e.target.value)}
    >
      {options.map(o => <option key={o} value={o}>{o}</option>)}
    </select>
  )
}

function Toggle({ checked, onChange }: { checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <button
      onClick={() => onChange(!checked)}
      className={`w-10 h-5 rounded-full transition-colors ${checked ? 'bg-brand' : 'bg-surface-3'} relative`}
    >
      <span className={`absolute top-0.5 w-4 h-4 bg-white rounded-full transition-transform ${checked ? 'left-5' : 'left-0.5'}`}/>
    </button>
  )
}

export function Settings() {
  const [cfg, setCfg] = useState<Record<string,unknown> | null>(null)
  const [overrides, setOverrides] = useState('')
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [tab, setTab] = useState<'general' | 'overrides'>('general')

  useEffect(() => {
    getConfig().then(setCfg).catch(() => null)
    getOverrides().then(d => setOverrides(d.content)).catch(() => null)
  }, [])

  const set = (path: string[], value: unknown) => {
    if (!cfg) return
    const updated = JSON.parse(JSON.stringify(cfg))
    let cur = updated
    for (let i = 0; i < path.length - 1; i++) cur = cur[path[i]] as Record<string,unknown>
    cur[path[path.length - 1]] = value
    setCfg(updated)
  }

  const get = (path: string[]): unknown => {
    if (!cfg) return ''
    let cur = cfg as Record<string,unknown>
    for (const k of path) cur = (cur[k] ?? '') as Record<string,unknown>
    return cur
  }

  const saveGeneral = async () => {
    if (!cfg) return
    setSaving(true)
    await updateConfig(cfg).catch(() => null)
    setSaving(false)
    setSaved(true)
    setTimeout(() => setSaved(false), 2000)
  }

  const saveOverrides = async () => {
    setSaving(true)
    await updateOverrides(overrides).catch(() => null)
    setSaving(false)
    setSaved(true)
    setTimeout(() => setSaved(false), 2000)
  }

  if (!cfg) return <div className="p-6 text-muted text-sm">加载中…</div>

  return (
    <div className="p-6 space-y-5 max-w-3xl mx-auto">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold text-white">设置</h1>
        {saved && <span className="badge badge-success">已保存</span>}
      </div>

      <div className="flex gap-2">
        {(['general','overrides'] as const).map(t => (
          <button key={t} onClick={() => setTab(t)} className={`btn text-xs py-1.5 ${tab === t ? 'btn-primary' : 'btn-ghost'}`}>
            {t === 'general' ? '常规设置' : 'Overrides YAML'}
          </button>
        ))}
      </div>

      {tab === 'general' && (
        <div className="space-y-4">
          <div className="card px-5 py-5 space-y-4">
            <h2 className="text-sm font-semibold text-slate-200 border-b border-white/5 pb-3">核心</h2>
            <Field label="mihomo 路径">
              <TextInput value={get(['core','binary']) as string} onChange={v => set(['core','binary'],v)} />
            </Field>
            <Field label="最大重启次数">
              <TextInput value={String(get(['core','max_restarts']) ?? 3)} onChange={v => set(['core','max_restarts'],parseInt(v)||3)} />
            </Field>
            <Field label="日志级别">
              <SelectInput value={get(['log','level']) as string || 'info'} onChange={v => set(['log','level'],v)} options={['debug','info','warn','error']} />
            </Field>
          </div>

          <div className="card px-5 py-5 space-y-4">
            <h2 className="text-sm font-semibold text-slate-200 border-b border-white/5 pb-3">网络</h2>
            <Field label="透明代理模式">
              <SelectInput value={get(['network','mode']) as string || 'tproxy'} onChange={v => set(['network','mode'],v)} options={['tproxy','redir','tun','none']} />
            </Field>
            <Field label="防火墙后端">
              <SelectInput value={get(['network','firewall_backend']) as string || 'auto'} onChange={v => set(['network','firewall_backend'],v)} options={['auto','nftables','iptables','none']} />
            </Field>
            <Field label="绕过局域网">
              <Toggle checked={!!get(['network','bypass_lan'])} onChange={v => set(['network','bypass_lan'],v)} />
            </Field>
            <Field label="绕过中国大陆 IP">
              <Toggle checked={!!get(['network','bypass_china'])} onChange={v => set(['network','bypass_china'],v)} />
            </Field>
          </div>

          <div className="card px-5 py-5 space-y-4">
            <h2 className="text-sm font-semibold text-slate-200 border-b border-white/5 pb-3">DNS</h2>
            <Field label="启用 DNS">
              <Toggle checked={!!get(['dns','enable'])} onChange={v => set(['dns','enable'],v)} />
            </Field>
            <Field label="DNS 模式">
              <SelectInput value={get(['dns','mode']) as string || 'fake-ip'} onChange={v => set(['dns','mode'],v)} options={['fake-ip','redir-host']} />
            </Field>
            <Field label="dnsmasq 共存">
              <SelectInput value={get(['dns','dnsmasq_mode']) as string || 'upstream'} onChange={v => set(['dns','dnsmasq_mode'],v)} options={['upstream','replace','none']} />
            </Field>
          </div>

          <button className="btn-primary flex items-center gap-2" onClick={saveGeneral} disabled={saving}>
            <Save size={14}/> {saving ? '保存中…' : '保存并重载'}
          </button>
        </div>
      )}

      {tab === 'overrides' && (
        <div className="space-y-4">
          <div className="card px-5 py-5">
            <p className="text-xs text-muted mb-3">此处内容会 deep-merge 覆盖生成的 mihomo 配置，优先级最高。</p>
            <textarea
              className="w-full bg-surface-2 border border-white/10 rounded-xl px-3 py-3 text-sm text-white font-mono outline-none focus:border-brand transition-colors resize-none"
              rows={20}
              value={overrides}
              onChange={e => setOverrides(e.target.value)}
              spellCheck={false}
            />
          </div>
          <button className="btn-primary flex items-center gap-2" onClick={saveOverrides} disabled={saving}>
            <Save size={14}/> {saving ? '保存中…' : '保存 Overrides'}
          </button>
        </div>
      )}
    </div>
  )
}
