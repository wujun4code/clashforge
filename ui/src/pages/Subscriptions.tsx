import { useEffect, useState } from 'react'
import { getSubscriptions, addSubscription, deleteSubscription, triggerSubUpdate, triggerUpdateAll } from '../api/client'
import type { Subscription } from '../api/client'
import { Plus, Trash2, RefreshCw, MoreVertical } from 'lucide-react'

function SubCard({ sub, onDelete, onUpdate }: { sub: Subscription; onDelete: () => void; onUpdate: () => void }) {
  const [menu, setMenu] = useState(false)
  const lastUpdated = sub.last_updated
    ? new Date(sub.last_updated).toLocaleString('zh-CN', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
    : '从未更新'

  return (
    <div className="glass-card px-5 py-4 flex items-start gap-4">
      <div className={`w-2 h-2 rounded-full mt-2 flex-shrink-0 ${sub.enabled ? 'bg-success' : 'bg-surface-3'}`} />
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 flex-wrap">
          <p className="font-semibold text-white text-sm">{sub.name}</p>
          {sub.enabled ? <span className="badge badge-success">启用</span> : <span className="badge badge-muted">禁用</span>}
        </div>
        <p className="text-xs text-muted mt-1">
          {sub.node_count ? `${sub.node_count} 节点` : '—'} · 上次更新: {lastUpdated}
        </p>
        {sub.url && <p className="text-xs text-muted truncate mt-0.5">{sub.url}</p>}
      </div>
      <div className="relative flex-shrink-0">
        <div className="flex gap-2">
          <button className="btn-ghost flex items-center gap-1.5 text-xs py-1.5 px-3" onClick={onUpdate}>
            <RefreshCw size={12}/> 更新
          </button>
          <button className="btn-ghost p-1.5" onClick={() => setMenu(m => !m)}>
            <MoreVertical size={15}/>
          </button>
        </div>
        {menu && (
          <div className="absolute right-0 top-9 z-10 bg-surface-2 border border-white/10 rounded-xl shadow-xl overflow-hidden w-36">
            <button className="w-full flex items-center gap-2 px-4 py-2.5 text-sm text-danger hover:bg-white/5 transition-all" onClick={() => { setMenu(false); onDelete() }}>
              <Trash2 size={13}/> 删除
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
              value={(form as unknown as Record<string,string>)[key] ?? ''}
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

export function Subscriptions() {
  const [subs, setSubs] = useState<Subscription[]>([])
  const [showAdd, setShowAdd] = useState(false)
  const [loading, setLoading] = useState(true)

  const refresh = () => getSubscriptions().then(d => { setSubs(d.subscriptions ?? []); setLoading(false) }).catch(() => setLoading(false))
  useEffect(() => { refresh() }, [])

  const handleDelete = async (id: string) => {
    await deleteSubscription(id).catch(() => null)
    refresh()
  }

  const handleUpdate = async (id: string) => {
    await triggerSubUpdate(id).catch(() => null)
  }

  return (
    <div className="p-6 space-y-5 max-w-3xl mx-auto">
      <div className="flex items-center justify-between">
        <div>
          <p className="text-[11px] uppercase tracking-[0.24em] text-muted">Subscriptions</p>
          <h1 className="text-base font-semibold text-white mt-1">订阅管理</h1>
        </div>
        <div className="flex gap-2">
          <button className="btn-ghost flex items-center gap-2" onClick={() => triggerUpdateAll().then(refresh)}>
            <RefreshCw size={14}/> 全部更新
          </button>
          <button className="btn-primary flex items-center gap-2" onClick={() => setShowAdd(true)}>
            <Plus size={14}/> 添加订阅
          </button>
        </div>
      </div>

      {loading && <div className="glass-card px-5 py-8 text-center text-muted text-sm">加载中…</div>}
      {!loading && subs.length === 0 && (
        <div className="glass-card px-5 py-8 text-center text-muted text-sm">
          还没有订阅。点击「添加订阅」开始配置。
        </div>
      )}
      {subs.map(sub => (
        <SubCard key={sub.id} sub={sub} onDelete={() => handleDelete(sub.id)} onUpdate={() => handleUpdate(sub.id)} />
      ))}

      {showAdd && <AddModal onClose={() => setShowAdd(false)} onAdded={refresh} />}
    </div>
  )
}
