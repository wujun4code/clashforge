import { NavLink } from 'react-router-dom'
import {
  LayoutDashboard, FolderCog, Activity, Sparkles
} from 'lucide-react'

const links = [
  { to: '/',         icon: LayoutDashboard, label: '概览' },
  { to: '/config',   icon: FolderCog,       label: '配置管理' },
  { to: '/activity', icon: Activity,        label: '活动' },
  { to: '/setup',    icon: Sparkles,        label: '配置向导' },
]

export function Sidebar() {
  return (
    <aside className="w-56 flex-shrink-0 bg-surface-1 border-r border-white/5 flex flex-col">
      <div className="px-5 py-5 border-b border-white/5">
        <div className="flex items-center gap-2.5">
          <span className="text-xl">⚡</span>
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
      <div className="px-5 py-4 border-t border-white/5">
        <p className="text-xs text-surface-3 font-mono">v0.1.0-dev</p>
      </div>
    </aside>
  )
}
