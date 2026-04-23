import { useEffect, useState } from 'react'
import {
  getOverrides,
  updateOverrides,
  generateConfig,
  getSubscriptions,
  addSubscription,
  deleteSubscription,
  triggerSubUpdate,
  triggerUpdateAll,
  getMihomoConfig,
} from '../api/client'
import type { Subscription } from '../api/client'
import { FolderCog, List, RefreshCw, Save, Plus, Trash2, MoreVertical, Zap, Eye } from 'lucide-react'

// ── Subscriptions sub-section ────────────────────────────────────────────────

function SubCard({ sub, onDelete, onUpdate }: { sub: Subscription; onDelete: () => void; onUpdate: () => void }) {
  const [menu, setMenu] = useState(false)
  const lastUpdated = sub.last_updated
    ? new Date(sub.last_updated).toLocaleString('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
    : '从未更新'

  return (
    <div className="card px-5 py-4 flex items-start gap-4">
      <div className={`w-2 h-2 rounded-full mt-2 flex-shrink-0 ${sub.enabled ? 'bg-success' : 'bg-surface-3'}`} />
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 flex-wrap">
          <p className="font-semibold text-white text-sm">{sub.name}</p>
          {sub.enabled
            ? <span className="badge badge-success">启用</span>
            : <span className="badge badge-muted">禁用</span>}
        </div>
        <p className="text-xs text-muted mt-1">
          {sub.node_count ? `${sub.node_count} 节点` : '—'} · 上次更新: {lastUpdated}
        </p>
        {sub.url && <p className="text-xs text-muted truncate mt-0.5">{sub.url}</p>}
      </div>
      <div className="relative flex-shrink-0">
        <div className="flex gap-2">
          <button className="btn-ghost flex items-center gap-1.5 text-xs py-1.5 px-3" onClick={onUpdate}>
            <RefreshCw size={12} /> 更新
          </button>
          <button className="btn-ghost p-1.5" onClick={() => setMenu(m => !m)}>
            <MoreVertical size={15} />
          </button>
        </div>
        {menu && (
          <div className="absolute right-0 top-9 z-10 bg-surface-2 border border-white/10 rounded-xl shadow-xl overflow-hidden w-36">
            <button
              className="w-full flex items-center gap-2 px-4 py-2.5 text-sm text-danger hover:bg-white/5 transition-all"
              onClick={() => { setMenu(false); onDelete() }}
            >
              <Trash2 size={13} /> 删除
            </button>
          </div>
        )}
      </div>
    </div>
  )
}

function AddModal({ onClose, onAdded }: { onClose: () => void; onAdded: () => void }) {
  const [form, setForm] = useState({ name: '', url: '', user_agent: 'clash-meta', interval: '6h', enabled: true })
  const [saving, setSaving] = useState(false)

  const save = async () => {
    if (!form.name || !form.url) return
    setSaving(true)
    await addSubscription({ ...form, type: 'url' }).catch(() => null)
    setSaving(false)
    onAdded()
    onClose()
  }

  return (
    <div className="fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50 p-4" onClick={onClose}>
      <div className="bg-surface-1 rounded-2xl border border-white/10 w-full max-w-md p-6 space-y-4" onClick={e => e.stopPropagation()}>
        <h2 className="text-base font-semibold text-white">添加订阅</h2>
        {[
          { key: 'name', label: '名称', placeholder: '我的机场' },
          { key: 'url', label: '订阅 URL', placeholder: 'https://...' },
          { key: 'user_agent', label: 'User-Agent', placeholder: 'clash-meta' },
          { key: 'interval', label: '更新间隔', placeholder: '6h' },
        ].map(({ key, label, placeholder }) => (
          <div key={key}>
            <label className="text-xs text-muted font-medium block mb-1.5">{label}</label>
            <input
              className="w-full bg-surface-2 border border-white/10 rounded-xl px-3 py-2.5 text-sm text-white outline-none focus:border-brand transition-colors"
              placeholder={placeholder}
              value={(form as unknown as Record<string, string>)[key] ?? ''}
              onChange={e => setForm(f => ({ ...f, [key]: e.target.value }))}
            />
          </div>
        ))}
        <div className="flex gap-3 pt-1">
          <button className="btn-ghost flex-1" onClick={onClose}>取消</button>
          <button className="btn-primary flex-1" onClick={save} disabled={saving}>
            {saving ? '保存中…' : '添加'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Config overrides sub-section ──────────────────────────────────────────────

function OverridesEditor() {
  const [content, setContent] = useState('')
  const [original, setOriginal] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [generating, setGenerating] = useState(false)
  const [status, setStatus] = useState('')

  const load = async () => {
    setLoading(true)
    const data = await getOverrides().catch(() => null)
    if (data) { setContent(data.content); setOriginal(data.content) }
    setLoading(false)
  }

  useEffect(() => { void load() }, [])

  const handleSave = async () => {
    setSaving(true)
    setStatus('')
    await updateOverrides(content).catch(() => null)
    setOriginal(content)
    setSaving(false)
    setStatus('已保存')
    setTimeout(() => setStatus(''), 3000)
  }

  const handleGenerate = async () => {
    setGenerating(true)
    setStatus('')
    const res = await generateConfig().catch(() => null)
    setGenerating(false)
    setStatus(res?.generated ? '配置已重新生成' : '生成失败')
    setTimeout(() => setStatus(''), 4000)
  }

  const dirty = content !== original

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <div>
          <p className="text-sm text-muted">编辑覆写规则（YAML），保存后点击「重新生成配置」使其生效。</p>
        </div>
        <div className="flex items-center gap-2">
          {status && <span className="text-xs text-success">{status}</span>}
          <button className="btn-ghost flex items-center gap-2" onClick={handleSave} disabled={saving || !dirty}>
            <Save size={14} />
            {saving ? '保存中…' : '保存'}
          </button>
          <button className="btn-primary flex items-center gap-2" onClick={handleGenerate} disabled={generating}>
            <Zap size={14} className={generating ? 'animate-pulse' : ''} />
            {generating ? '生成中…' : '重新生成配置'}
          </button>
        </div>
      </div>

      {loading ? (
        <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-8 text-sm text-muted text-center">
          加载中…
        </div>
      ) : (
        <textarea
          className="w-full h-96 bg-surface-2 border border-white/10 rounded-2xl px-4 py-3 text-sm font-mono text-slate-200 outline-none focus:border-brand transition-colors resize-none"
          spellCheck={false}
          value={content}
          onChange={e => setContent(e.target.value)}
        />
      )}
    </div>
  )
}

// ── Subscriptions sub-section ─────────────────────────────────────────────────

function SubscriptionsPanel() {
  const [subs, setSubs] = useState<Subscription[]>([])
  const [showAdd, setShowAdd] = useState(false)
  const [loading, setLoading] = useState(true)

  const refresh = () => getSubscriptions()
    .then(d => { setSubs(d.subscriptions ?? []); setLoading(false) })
    .catch(() => setLoading(false))

  useEffect(() => { refresh() }, [])

  const handleDelete = async (id: string) => {
    await deleteSubscription(id).catch(() => null)
    refresh()
  }

  const handleUpdate = async (id: string) => {
    await triggerSubUpdate(id).catch(() => null)
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <p className="text-sm text-muted">管理节点订阅源，支持 Clash / SS / Trojan / VLESS / VMess 格式。</p>
        <div className="flex gap-2">
          <button className="btn-ghost flex items-center gap-2" onClick={() => triggerUpdateAll().then(refresh)}>
            <RefreshCw size={14} /> 全部更新
          </button>
          <button className="btn-primary flex items-center gap-2" onClick={() => setShowAdd(true)}>
            <Plus size={14} /> 添加订阅
          </button>
        </div>
      </div>

      {loading && (
        <div className="card px-5 py-8 text-center text-muted text-sm">加载中…</div>
      )}
      {!loading && subs.length === 0 && (
        <div className="card px-5 py-8 text-center text-muted text-sm">
          还没有订阅。点击「添加订阅」开始配置。
        </div>
      )}
      {subs.map(sub => (
        <SubCard
          key={sub.id}
          sub={sub}
          onDelete={() => handleDelete(sub.id)}
          onUpdate={() => handleUpdate(sub.id)}
        />
      ))}

      {showAdd && <AddModal onClose={() => setShowAdd(false)} onAdded={refresh} />}
    </div>
  )
}

// ── Running config view ───────────────────────────────────────────────────────

function RunningConfigPanel() {
  const [content, setContent] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  const load = () => {
    setLoading(true)
    getMihomoConfig()
      .then(d => setContent(d.content))
      .catch(() => setContent(''))
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [])

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <p className="text-sm text-muted">由 ClashForge 生成并写入 /var/run/metaclash/mihomo-config.yaml 的当前运行配置（只读）。</p>
        <button className="btn-ghost flex items-center gap-2" onClick={load} disabled={loading}>
          <RefreshCw size={14} className={loading ? 'animate-spin' : ''} /> 刷新
        </button>
      </div>
      {loading ? (
        <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-8 text-sm text-muted text-center">加载中…</div>
      ) : content === '' ? (
        <div className="rounded-2xl border border-white/8 bg-black/10 px-4 py-8 text-sm text-muted text-center">
          配置文件不存在（核心尚未启动或尚未生成配置）
        </div>
      ) : (
        <textarea
          className="w-full h-[32rem] bg-surface-2 border border-white/10 rounded-2xl px-4 py-3 text-sm font-mono text-slate-300 outline-none resize-none"
          value={content ?? ''}
          readOnly
          spellCheck={false}
        />
      )}
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────────

type Tab = 'config' | 'subscriptions' | 'running'

export function ConfigManagement() {
  const [tab, setTab] = useState<Tab>('running')

  return (
    <div className="p-6 space-y-5 max-w-4xl mx-auto">
      <div className="flex items-center gap-3">
        <FolderCog size={20} className="text-brand" />
        <h1 className="text-lg font-semibold text-white">配置管理</h1>
      </div>

      {/* Tab switcher */}
      <div className="flex gap-2">
        <button
          className={`btn-ghost flex items-center gap-2 ${tab === 'running' ? 'border-brand/40 text-white' : ''}`}
          onClick={() => setTab('running')}
        >
          <Eye size={14} /> 运行中配置
        </button>
        <button
          className={`btn-ghost flex items-center gap-2 ${tab === 'subscriptions' ? 'border-brand/40 text-white' : ''}`}
          onClick={() => setTab('subscriptions')}
        >
          <List size={14} /> 订阅
        </button>
        <button
          className={`btn-ghost flex items-center gap-2 ${tab === 'config' ? 'border-brand/40 text-white' : ''}`}
          onClick={() => setTab('config')}
        >
          <FolderCog size={14} /> 覆写配置
        </button>
      </div>

      {tab === 'running' && <RunningConfigPanel />}
      {tab === 'subscriptions' && <SubscriptionsPanel />}
      {tab === 'config' && <OverridesEditor />}
    </div>
  )
}
