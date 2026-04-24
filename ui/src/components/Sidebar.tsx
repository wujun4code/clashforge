import { useEffect, useState } from 'react'
import { NavLink } from 'react-router-dom'
import { LayoutDashboard, FolderCog, Activity, Sparkles, Settings, Cpu } from 'lucide-react'
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

  const dotClass = state === 'running'
    ? 'status-dot-online'
    : state === 'stopped'
      ? 'status-dot-offline'
      : 'status-dot-warn'

  const labelClass = state === 'running'
    ? 'text-success'
    : state === 'stopped'
      ? 'text-slate-500'
      : 'text-warning'

  const label = state === 'running' ? '运行中' : state === 'stopped' ? '未启动' : '检测中…'

  return (
    <div className="mx-3 mb-3">
      <div className="flex items-center gap-2.5 px-3 py-2.5 rounded-xl bg-surface-2/60 border border-white/[0.05]">
        <Cpu size={13} className="text-muted flex-shrink-0" />
        <div className="flex-1 min-w-0">
          <p className="text-[10px] text-muted leading-none mb-1">内核状态</p>
          <div className="flex items-center gap-1.5">
            <span className={dotClass} />
            <p className={`text-xs font-semibold leading-none ${labelClass}`}>{label}</p>
          </div>
        </div>
      </div>
    </div>
  )
}

export function Sidebar() {
  return (
    <aside className="w-56 flex-shrink-0 flex flex-col relative"
      style={{
        background: 'linear-gradient(180deg, #0b1120 0%, #080e1c 100%)',
        borderRight: '1px solid rgba(6,182,212,0.08)',
      }}
    >
      {/* subtle top glow line */}
      <div className="absolute top-0 left-0 right-0 h-px" style={{ background: 'linear-gradient(90deg, transparent, rgba(6,182,212,0.4), transparent)' }} />

      {/* Logo */}
      <div className="px-5 py-5 border-b border-white/[0.04]">
        <div className="flex items-center gap-2.5">
          <div className="relative">
            <img src="/favicon.svg" alt="ClashForge" className="w-6 h-6 relative z-10" />
            <div className="absolute inset-0 blur-sm opacity-60" style={{ background: 'rgba(6,182,212,0.4)', borderRadius: '50%' }} />
          </div>
          <span className="font-bold text-base tracking-wide text-white">
            Clash<span className="text-brand">Forge</span>
          </span>
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
              `flex items-center gap-3 px-3 py-2.5 rounded-xl text-sm font-medium transition-all duration-200 cursor-pointer group relative overflow-hidden ${
                isActive
                  ? 'text-brand'
                  : 'text-muted hover:text-slate-200'
              }`
            }
          >
            {({ isActive }) => (
              <>
                {isActive && (
                  <>
                    <div className="absolute inset-0 rounded-xl bg-brand/10" />
                    <div className="absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-4 rounded-r-full bg-brand"
                      style={{ boxShadow: '0 0 8px rgba(6,182,212,0.8)' }} />
                  </>
                )}
                {!isActive && (
                  <div className="absolute inset-0 rounded-xl opacity-0 group-hover:opacity-100 transition-opacity bg-white/[0.04]" />
                )}
                <Icon size={16} className="relative z-10 flex-shrink-0" />
                <span className="relative z-10">{label}</span>
              </>
            )}
          </NavLink>
        ))}
      </nav>

      <CoreStatusBadge />

      {/* Version */}
      <div className="px-5 py-3 border-t border-white/[0.04]">
        <p className="text-[10px] text-muted/60 font-mono">{__APP_VERSION__}</p>
      </div>
    </aside>
  )
}
