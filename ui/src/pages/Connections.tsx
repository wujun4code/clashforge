import { useEffect, useState } from 'react'
import { getConnections, closeAllConns } from '../api/client'
import type { Connection } from '../api/client'
import { formatBytes } from '../utils/format'
import { Trash2, RefreshCw } from 'lucide-react'

export function Connections() {
  const [conns, setConns] = useState<Connection[]>([])
  const [loading, setLoading] = useState(true)

  const refresh = () => getConnections().then(d => { setConns(d.connections ?? []); setLoading(false) }).catch(() => setLoading(false))
  useEffect(() => { refresh(); const t = setInterval(refresh, 2000); return () => clearInterval(t) }, [])

  const handleCloseAll = async () => { await closeAllConns().catch(() => null); refresh() }

  return (
    <div className="p-6 space-y-5 max-w-6xl mx-auto">
      <div className="flex items-center justify-between">
        <div>
          <p className="text-[11px] uppercase tracking-[0.24em] text-muted">Connections</p>
          <h1 className="text-base font-semibold text-white mt-1">连接管理</h1>
        </div>
        <div className="flex gap-2">
          <button className="btn-ghost flex items-center gap-2" onClick={refresh}><RefreshCw size={14}/> 刷新</button>
          <button className="btn-danger flex items-center gap-2" onClick={handleCloseAll}><Trash2 size={14}/> 清理全部</button>
        </div>
      </div>

      <div className="glass-card overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-white/5 text-xs text-muted uppercase tracking-wider">
                <th className="px-4 py-3 text-left">目标</th>
                <th className="px-4 py-3 text-left">协议</th>
                <th className="px-4 py-3 text-left">代理链</th>
                <th className="px-4 py-3 text-right">上传</th>
                <th className="px-4 py-3 text-right">下载</th>
              </tr>
            </thead>
            <tbody>
              {loading && (
                <tr><td colSpan={5} className="px-4 py-8 text-center text-muted">加载中…</td></tr>
              )}
              {!loading && conns.length === 0 && (
                <tr><td colSpan={5} className="px-4 py-8 text-center text-muted">暂无活跃连接</td></tr>
              )}
              {conns.map(c => (
                <tr key={c.id} className="border-b border-white/5 hover:bg-white/3 transition-colors">
                  <td className="px-4 py-3 font-mono text-xs text-slate-200 max-w-xs truncate">
                    {c.metadata.host || c.metadata.sourceIP}:{c.metadata.destinationPort}
                  </td>
                  <td className="px-4 py-3">
                    <span className="badge badge-muted">{c.metadata.network || c.metadata.type}</span>
                  </td>
                  <td className="px-4 py-3 text-xs text-muted">{c.chains?.join(' → ')}</td>
                  <td className="px-4 py-3 text-right font-mono text-xs text-brand">{formatBytes(c.upload, '')}</td>
                  <td className="px-4 py-3 text-right font-mono text-xs text-success">{formatBytes(c.download, '')}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}
