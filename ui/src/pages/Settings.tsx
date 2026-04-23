import { useEffect, useRef, useState } from 'react'
import { getConfig, updateConfig, getOverrides, updateOverrides, generateConfig, getMihomoConfig } from '../api/client'
import { Save, Upload, FileText, CheckCircle2, AlertCircle, ArrowRight, RefreshCw } from 'lucide-react'
import { useNavigate } from 'react-router-dom'

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
  const [runningConfig, setRunningConfig] = useState<string | null>(null)
  const [runningConfigLoading, setRunningConfigLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [saveError, setSaveError] = useState('')
  const [importDone, setImportDone] = useState(false)
  const [tab, setTab] = useState<'import' | 'general' | 'overrides' | 'running'>('import')
  const fileRef = useRef<HTMLInputElement>(null)
  const navigate = useNavigate()

  const loadRunningConfig = () => {
    setRunningConfigLoading(true)
    getMihomoConfig().then(d => setRunningConfig(d.content)).catch(() => setRunningConfig('')).finally(() => setRunningConfigLoading(false))
  }

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
    setSaving(true); setSaveError('')
    try {
      await updateConfig(cfg)
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    } catch(e: unknown) {
      setSaveError(String(e))
    } finally { setSaving(false) }
  }

  const saveOverrides = async () => {
    setSaving(true); setSaveError('')
    try {
      await updateOverrides(overrides)
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    } catch(e: unknown) {
      setSaveError(String(e))
    } finally { setSaving(false) }
  }

  // Handle file upload or paste of full Clash YAML config
  const handleImport = async (content: string) => {
    setSaving(true); setSaveError(''); setImportDone(false)
    try {
      if (!content.trim()) { setSaveError('内容为空'); setSaving(false); return }
      await updateOverrides(content)
      setOverrides(content)
      // Auto-generate mihomo config
      await generateConfig().catch(() => null)
      setImportDone(true)
      setSaved(true)
      setTimeout(() => setSaved(false), 5000)
    } catch(e: unknown) {
      setSaveError('YAML 格式不正确：' + String(e))
    } finally { setSaving(false) }
  }

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = (ev) => handleImport(ev.target?.result as string)
    reader.readAsText(file)
  }

  if (!cfg) return <div className="p-6 text-muted text-sm">加载中…</div>

  return (
    <div className="p-6 space-y-5 max-w-3xl mx-auto">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold text-white">设置</h1>
        <div className="flex items-center gap-2">
          {saveError && <span className="flex items-center gap-1 text-xs text-danger"><AlertCircle size={12}/>{saveError}</span>}
          {saved && <span className="flex items-center gap-1 text-xs text-success"><CheckCircle2 size={12}/> 已保存</span>}
        </div>
      </div>

      <div className="flex gap-2 flex-wrap">
        {(['import','general','overrides','running'] as const).map(t => (
          <button key={t} onClick={() => { setTab(t); if (t === 'running' && runningConfig === null) loadRunningConfig() }} className={`btn text-xs py-1.5 ${tab === t ? 'btn-primary' : 'btn-ghost'}`}>
            {t === 'import' ? '📋 导入配置' : t === 'general' ? '常规设置' : t === 'overrides' ? 'Overrides YAML' : '🔍 运行中配置'}
          </button>
        ))}
      </div>

      {tab === 'import' && (
        <div className="space-y-4">
          {/* Import method 1: File upload */}
          <div className="card px-5 py-5 space-y-4">
            <div className="flex items-center gap-2 mb-1">
              <FileText size={16} className="text-brand"/>
              <h2 className="text-sm font-semibold text-slate-200">从文件导入</h2>
            </div>
            <p className="text-xs text-muted leading-5">
              直接上传你现有的 Clash / OpenClash YAML 配置文件（config.yaml 或订阅下载的文件）。
              导入后会作为 Overrides 覆盖生成的配置，优先级最高。
            </p>
            <div
              className="border-2 border-dashed border-white/15 rounded-2xl px-6 py-10 flex flex-col items-center gap-3 hover:border-brand/40 hover:bg-brand/5 transition-all cursor-pointer"
              onClick={() => fileRef.current?.click()}
              onDragOver={e => e.preventDefault()}
              onDrop={e => {
                e.preventDefault()
                const file = e.dataTransfer.files[0]
                if (!file) return
                const reader = new FileReader()
                reader.onload = ev => handleImport(ev.target?.result as string)
                reader.readAsText(file)
              }}
            >
              <Upload size={28} className="text-muted"/>
              <div className="text-center">
                <p className="text-sm text-slate-300 font-medium">点击上传或拖放文件</p>
                <p className="text-xs text-muted mt-1">.yaml / .yml 格式</p>
              </div>
              <input ref={fileRef} type="file" accept=".yaml,.yml,.txt" className="hidden" onChange={handleFileUpload} />
            </div>
          </div>

          {/* Import method 2: Paste */}
          <div className="card px-5 py-5 space-y-3">
            <div className="flex items-center gap-2">
              <FileText size={16} className="text-brand"/>
              <h2 className="text-sm font-semibold text-slate-200">直接粘贴 YAML</h2>
            </div>
            <p className="text-xs text-muted">把配置内容粘贴到下面，然后点“导入并应用”。</p>
            <textarea
              className="w-full bg-surface-2 border border-white/10 rounded-xl px-3 py-3 text-sm text-white font-mono outline-none focus:border-brand transition-colors resize-none"
              rows={14}
              placeholder="port: 7890&#10;proxies:&#10;  - name: my-node&#10;    type: ss&#10;    ...&#10;&#10;或者直接粘贴整份订阅下载的 YAML…"
              value={overrides}
              onChange={e => setOverrides(e.target.value)}
              spellCheck={false}
            />
            <button
              className="btn-primary w-full flex items-center justify-center gap-2"
              onClick={() => handleImport(overrides)}
              disabled={saving || !overrides.trim()}
            >
              <Save size={14}/> {saving ? '导入中…' : '导入并应用'}
            </button>

            {importDone && (
              <div className="rounded-xl bg-success/10 border border-success/20 px-4 py-3 flex items-start gap-3">
                <CheckCircle2 size={16} className="text-success flex-shrink-0 mt-0.5"/>
                <div className="flex-1">
                  <p className="text-sm font-semibold text-success">配置已导入并生成</p>
                  <p className="text-xs text-muted mt-1">现在可以回到首页启动核心，节点信息会在「节点」页面显示。</p>
                  <button
                    className="mt-2 flex items-center gap-1.5 text-xs text-brand hover:text-blue-300 transition-colors"
                    onClick={() => navigate('/')}
                  >
                    <ArrowRight size={12}/> 去首页启动核心
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>
      )}

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
              <SelectInput value={get(['network','mode']) as string || 'none'} onChange={v => set(['network','mode'],v)} options={['none','tproxy','redir','tun']} />
            </Field>
            <Field label="防火墙后端">
              <SelectInput value={get(['network','firewall_backend']) as string || 'auto'} onChange={v => set(['network','firewall_backend'],v)} options={['auto','nftables','iptables','none']} />
            </Field>
            <Field label="启动时接管透明代理">
              <Toggle checked={!!get(['network','apply_on_start'])} onChange={v => set(['network','apply_on_start'],v)} />
            </Field>
            <Field label="绕过局域网">
              <Toggle checked={!!get(['network','bypass_lan'])} onChange={v => set(['network','bypass_lan'],v)} />
            </Field>
            <Field label="绕过中国大陆 IP">
              <Toggle checked={!!get(['network','bypass_china'])} onChange={v => set(['network','bypass_china'],v)} />
            </Field>
            <p className="text-xs text-muted leading-5 border-t border-white/5 pt-3">
              默认不在启动时接管透明代理。开启后需要重启 clashforge 服务，启动阶段才会应用 nft / TProxy 规则。
            </p>
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
              <SelectInput value={get(['dns','dnsmasq_mode']) as string || 'none'} onChange={v => set(['dns','dnsmasq_mode'],v)} options={['none','upstream','replace']} />
            </Field>
            <Field label="启动时接管 DNS">
              <Toggle checked={!!get(['dns','apply_on_start'])} onChange={v => set(['dns','apply_on_start'],v)} />
            </Field>
            <p className="text-xs text-muted leading-5 border-t border-white/5 pt-3">
              默认只启动 mihomo 自己的 DNS 能力，不改写 dnsmasq。开启后需要重启 clashforge 服务，启动阶段才会接管 DNS。
            </p>
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

      {tab === 'running' && (
        <div className="space-y-4">
          <div className="card px-5 py-5 space-y-3">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-semibold text-slate-200">当前运行的 mihomo 配置</p>
                <p className="text-xs text-muted mt-0.5">由 ClashForge 实时生成并写入 /var/run/metaclash/mihomo-config.yaml</p>
              </div>
              <button className="btn-ghost flex items-center gap-1.5 text-xs py-1.5" onClick={loadRunningConfig} disabled={runningConfigLoading}>
                <RefreshCw size={13} className={runningConfigLoading ? 'animate-spin' : ''}/> 刷新
              </button>
            </div>
            {runningConfigLoading && (
              <div className="py-8 text-center text-muted text-sm">加载中…</div>
            )}
            {!runningConfigLoading && runningConfig === '' && (
              <div className="py-8 text-center text-muted text-sm">配置文件不存在（核心尚未启动或尚未生成配置）</div>
            )}
            {!runningConfigLoading && runningConfig !== null && runningConfig !== '' && (
              <textarea
                className="w-full bg-surface-2 border border-white/10 rounded-xl px-3 py-3 text-sm text-slate-300 font-mono outline-none resize-none"
                rows={28}
                value={runningConfig}
                readOnly
                spellCheck={false}
              />
            )}
          </div>
        </div>
      )}
    </div>
  )
}
