import { useEffect, useState } from 'react'
import { NavLink } from 'react-router-dom'
import {
  LayoutDashboard, FolderCog, Activity, Sparkles, Settings
} from 'lucide-react'
import { getOverviewCore } from '../api/client'

const links = [
  { to: '/',         icon: LayoutDashboard, label: '概览' },
  { to: '/config',   icon: FolderCog,       label: '配置管理' },
  { to: '/activity', icon: Activity,        label: '活动' },
  { to: '/setup',    icon: Sparkles,        label: '配置向导' },
  { to: '/settings', icon: Settings,        label: '高级管理' },
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
    <div className="mx-3 mb-3 flex items-center gap-2.5 px-3 py-2.5 rounded-xl bg-white/[0.03] border border-white/[0.06] backdrop-blur-sm">
      <span className={`relative w-2 h-2 rounded-full flex-shrink-0 transition-all duration-300 ${
        state === 'running'
          ? 'bg-success'
          : state === 'stopped'
            ? 'bg-white/15'
            : 'bg-warning/50'
      }`}>
        {state === 'running' && (
          <span className="absolute inset-0 rounded-full bg-success animate-ping opacity-40" />
        )}
      </span>
      <div className="min-w-0">
        <p className="text-[10px] text-muted leading-none tracking-wide uppercase">内核状态</p>
        <p className={`text-xs font-semibold leading-none mt-1 transition-colors duration-300 ${
          state === 'running' ? 'text-success' :
          state === 'stopped' ? 'text-slate-500' :
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
    <aside className="w-56 flex-shrink-0 bg-surface-0/95 backdrop-blur-xl border-r border-white/[0.05] flex flex-col h-screen">
      {/* Logo */}
      <div className="px-5 py-5 border-b border-white/[0.05]">
        <div className="flex items-center gap-2.5 group">
          <div className="relative">
            <img src="/favicon.svg" alt="ClashForge" className="w-6 h-6 relative z-10" />
            <div className="absolute inset-0 bg-brand/20 blur-lg rounded-full scale-150 group-hover:bg-brand/30 transition-all duration-500" />
          </div>
          <div>
            <span className="font-bold text-base tracking-tight text-white font-heading">ClashForge</span>
            <p className="text-[10px] text-muted/50 leading-none mt-0.5">Dashboard</p>
          </div>
        </div>
      </div>

      {/* Nav */}
      <nav className="flex-1 px-3 py-4 space-y-0.5">
        {links.map(({ to, icon: Icon, label }) => (
          <NavLink
            key={to}
            to={to}
            end={to === '/'}
            className={({ isActive }) =>
              `group flex items-center gap-3 px-3 py-2.5 rounded-xl text-sm font-medium transition-all duration-200 ${
                isActive
                  ? 'bg-brand-subtle text-brand-light border border-brand/20 shadow-glow/30'
                  : 'text-muted hover:text-[#E0DFF0] hover:bg-white/[0.04] border border-transparent'
              }`
            }
          >
            {({ isActive }) => (
              <>
                <div className={`relative flex items-center justify-center w-5 h-5 transition-all duration-200 ${
                  isActive ? 'text-brand-light' : 'text-muted group-hover:text-[#E0DFF0]'
                }`}>
                  <Icon size={17} className="relative z-10" />
                  {isActive && (
                    <span className="absolute inset-0 bg-brand/20 blur-md rounded-full scale-150" />
                  )}
                </div>
                <span>{label}</span>
              </>
            )}
          </NavLink>
        ))}
      </nav>

      {/* Bottom */}
      <CoreStatusBadge />
      <div className="px-5 py-4 border-t border-white/[0.05]">
        <p className="text-[11px] text-surface-4/50 font-mono select-none">{__APP_VERSION__}</p>
      </div>
    </aside>
  )
}
