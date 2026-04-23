import { useEffect, useState } from 'react'
import { NavLink } from 'react-router-dom'
import {
  LayoutDashboard, FolderCog, Activity, Sparkles
} from 'lucide-react'
import { getOverviewCore } from '../api/client'

const links = [
  { to: '/',         icon: LayoutDashboard, label: '概览' },
  { to: '/config',   icon: FolderCog,       label: '配置管理' },
  { to: '/activity', icon: Activity,        label: '活动' },
  { to: '/setup',    icon: Sparkles,        label: '配置向导' },
]

function CoreStatusBadge() {
  const [state, setState] = useState<'running' | 'stopped' | 'checking'>('checking')

  useEffect(() => {
    const check = () => {
      getOverviewCore()
        .then(d => setState(d.core.state === 'running' ? 'running' : 'stopped'))
        .catch(() => setState('stopped'))
    }
    check()
    const id = setInterval(check, 5000)
    return () => clearInterval(id)
  }, [])

  return (
    <div className="mx-3 mb-3 flex items-center gap-2.5 px-3 py-2.5 rounded-xl bg-white/[0.04] border border-white/[0.07]">
      <span className={`w-2 h-2 rounded-full flex-shrink-0 transition-colors ${
        state === 'running' ? 'bg-success animate-pulse' :
        state === 'stopped' ? 'bg-white/20' :
        'bg-warning/60'
      }`} />
      <div className="min-w-0">
        <p className="text-[10px] text-muted leading-none">内核状态</p>
        <p className={`text-xs font-semibold leading-none mt-1 ${
          state === 'running' ? 'text-success' :
          state === 'stopped' ? 'text-slate-400' :
          'text-warning'
        }`}>
          {state === 'running' ? '运行中' : state === 'stopped' ? '未启动' : '检测中…'}
        </p>
      </div>
    </div>
  )
}

export function Sidebar() {
  return (
    <aside className="w-56 flex-shrink-0 bg-surface-1 border-r border-white/5 flex flex-col">
      <div className="px-5 py-5 border-b border-white/5">
        <div className="flex items-center gap-2.5">
          <img src="/favicon.svg" alt="ClashForge" className="w-6 h-6" />
          <span className="font-bold text-base tracking-wide text-white">ClashForge</span>
        </div>
      </div>
      <nav className="flex-1 px-3 py-4 space-y-1">
        {links.map(({ to, icon: Icon, label }) => (
          <NavLink
            key={to}
            to={to}
            end={to === '/'}
            className={({ isActive }) =>
              `flex items-center gap-3 px-3 py-2.5 rounded-xl text-sm font-medium transition-all ${
                isActive
                  ? 'bg-brand/15 text-brand'
                  : 'text-muted hover:text-slate-200 hover:bg-white/5'
              }`
            }
          >
            <Icon size={17} />
            {label}
          </NavLink>
        ))}
      </nav>
      <CoreStatusBadge />
      <div className="px-5 py-4 border-t border-white/5">
        <p className="text-xs text-surface-3 font-mono">{__APP_VERSION__}</p>
      </div>
    </aside>
  )
}
