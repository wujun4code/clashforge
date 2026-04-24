import { useEffect, useState } from 'react'
import { NavLink } from 'react-router-dom'
import {
  Activity,
  ChevronRight,
  FolderCog,
  LayoutDashboard,
  Settings,
  Wifi,
  WifiOff,
  Loader,
} from 'lucide-react'
import { getOverviewCore } from '../api/client'

type CoreState = 'running' | 'stopped' | 'checking'

const navLinks = [
  {
    to: '/',
    icon: LayoutDashboard,
    label: '// OVERVIEW',
    caption: 'SYS_STATUS · CORE_MON · RESOURCE',
  },
  {
    to: '/config',
    icon: FolderCog,
    label: '// CONFIG',
    caption: 'SUBS · RULES · RUNTIME_CFG',
  },
  {
    to: '/activity',
    icon: Activity,
    label: '// ACTIVITY',
    caption: 'CONN_TRACE · LIVE_LOG · DIAG',
  },
  {
    to: '/settings',
    icon: Settings,
    label: '// ADMIN',
    caption: 'CORE_PARAMS · SYSTEM · RESET',
  },
]

const statusConfig: Record<CoreState, {
  label: string
  color: string
  borderColor: string
  bgColor: string
  iconColor: string
  shadowColor: string
  StatusIcon: typeof Wifi
}> = {
  running: {
    label: 'ONLINE',
    color: 'text-neon-green',
    borderColor: 'border-neon-green/40',
    bgColor: 'bg-neon-green/[0.06]',
    iconColor: 'text-neon-green',
    shadowColor: 'shadow-neon-green',
    StatusIcon: Wifi,
  },
  stopped: {
    label: 'OFFLINE',
    color: 'text-neon-red',
    borderColor: 'border-neon-red/40',
    bgColor: 'bg-neon-red/[0.06]',
    iconColor: 'text-neon-red',
    shadowColor: 'shadow-neon-red',
    StatusIcon: WifiOff,
  },
  checking: {
    label: 'INIT...',
    color: 'text-neon-yellow',
    borderColor: 'border-neon-yellow/40',
    bgColor: 'bg-neon-yellow/[0.06]',
    iconColor: 'text-neon-yellow',
    shadowColor: 'shadow-neon-yellow',
    StatusIcon: Loader,
  },
}

export function Sidebar() {
  const [coreState, setCoreState] = useState<CoreState>('checking')

  useEffect(() => {
    const check = () => {
      getOverviewCore()
        .then((d) => setCoreState(d.core.state === 'running' ? 'running' : 'stopped'))
        .catch(() => setCoreState('stopped'))
    }
    check()
    const id = setInterval(check, 5000)
    return () => clearInterval(id)
  }, [])

  const ss = statusConfig[coreState]

  return (
    <aside
      className="flex h-screen w-[280px] flex-shrink-0 flex-col bg-surface-0/98 backdrop-blur-2xl"
      style={{ borderRight: '1px solid rgba(0,245,255,0.10)' }}
    >
      {/* ── Brand Header ─────────────────────────────── */}
      <div className="px-4 py-5" style={{ borderBottom: '1px solid rgba(0,245,255,0.06)' }}>
        {/* Logo row */}
        <div className="flex items-center gap-3 mb-4">
          <div
            className="relative flex h-10 w-10 items-center justify-center flex-shrink-0"
            style={{
              border: '1px solid rgba(0,245,255,0.3)',
              boxShadow:
                '0 0 12px rgba(0,245,255,0.25), inset 0 0 16px rgba(167,139,250,0.18)',
              background:
                'linear-gradient(135deg, rgba(0,245,255,0.08) 0%, rgba(167,139,250,0.10) 55%, rgba(249,115,22,0.08) 100%)',
            }}
          >
            <img src="/favicon.svg" alt="ClashForge" className="h-6 w-6" />
          </div>
          <div className="min-w-0">
            <p
              className="font-mono text-base font-semibold tracking-[0.08em] text-display-gradient animate-glitch"
            >
              CLASHFORGE
            </p>
            <p className="text-[9px] uppercase tracking-[0.3em] text-muted mt-0.5 font-mono">
              NETWORK_CONTROL_v{__APP_VERSION__}
            </p>
          </div>
        </div>

        {/* Status panel */}
        <NavLink to="/setup" className="group block cursor-pointer">
          <div
            className={`relative flex items-center gap-3 px-3 py-3 transition-all duration-200 ${ss.bgColor} ${ss.borderColor}`}
            style={{
              border: '1px solid',
              borderColor: 'inherit',
              clipPath: 'polygon(0 0, calc(100% - 8px) 0, 100% 8px, 100% 100%, 8px 100%, 0 calc(100% - 8px))',
            }}
          >
            {/* Status icon */}
            <div className={`flex h-10 w-10 flex-shrink-0 items-center justify-center ${ss.bgColor} ${ss.borderColor} ${ss.iconColor}`}
              style={{ border: '1px solid' }}>
              <ss.StatusIcon
                size={18}
                className={coreState === 'checking' ? 'animate-spin' : coreState === 'running' ? 'animate-pulse-soft' : ''}
              />
            </div>

            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <span className="text-[9px] uppercase tracking-[0.2em] text-muted">PROXY_SVC</span>
              </div>
              <div className="flex items-center gap-2 mt-0.5">
                <span
                  className={`font-mono text-sm font-bold ${ss.color}`}
                  style={{ textShadow: coreState === 'running' ? '0 0 8px rgba(0,255,136,0.8)' : coreState === 'stopped' ? '0 0 8px rgba(255,34,85,0.8)' : '0 0 8px rgba(255,230,0,0.8)' }}
                >
                  {ss.label}
                </span>
                {coreState === 'running' && (
                  <span className="inline-block w-1.5 h-1.5 bg-neon-green animate-pulse-soft"
                    style={{ boxShadow: '0 0 6px #00FF88' }} />
                )}
              </div>
            </div>

            <ChevronRight size={13} className={`${ss.color} opacity-60 group-hover:opacity-100 transition-opacity`} />

            {/* Corner accent */}
            <span className="absolute top-0 right-0 pointer-events-none"
              style={{ width: 8, height: 8, background: `linear-gradient(225deg, ${coreState === 'running' ? '#00FF88' : coreState === 'stopped' ? '#FF2255' : '#FFE600'} 0%, transparent 60%)` }} />
          </div>
        </NavLink>
      </div>

      {/* ── Navigation ───────────────────────────────── */}
      <nav className="flex-1 overflow-y-auto px-3 py-4 space-y-1">
        {/* Section label */}
        <div className="flex items-center gap-2 px-2 pb-2">
          <div className="h-px flex-1" style={{ background: 'linear-gradient(90deg, transparent, rgba(0,245,255,0.15))' }} />
          <span className="text-[9px] uppercase tracking-[0.25em] text-muted">NAV_MATRIX</span>
          <div className="h-px flex-1" style={{ background: 'linear-gradient(270deg, transparent, rgba(0,245,255,0.15))' }} />
        </div>

        {navLinks.map(({ to, icon: Icon, label, caption }) => (
          <NavLink
            key={to}
            to={to}
            end={to === '/'}
          >
            {({ isActive }) => (
              <div
                className={[
                  'group relative flex items-center gap-3 px-3 py-3 transition-all duration-200 cursor-pointer',
                  isActive
                    ? 'bg-neon-cyan/[0.08]'
                    : 'hover:bg-surface-2/60',
                ].join(' ')}
                style={{
                  border: '1px solid',
                  borderColor: isActive ? 'rgba(0,245,255,0.3)' : 'transparent',
                  clipPath: isActive
                    ? 'polygon(0 0, calc(100% - 6px) 0, 100% 6px, 100% 100%, 6px 100%, 0 calc(100% - 6px))'
                    : 'none',
                }}
                onMouseEnter={(e) => {
                  if (!isActive) (e.currentTarget as HTMLElement).style.borderColor = 'rgba(0,245,255,0.12)'
                }}
                onMouseLeave={(e) => {
                  if (!isActive) (e.currentTarget as HTMLElement).style.borderColor = 'transparent'
                }}
              >
                {/* Active indicator bar */}
                {isActive && (
                  <span
                    className="absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-6"
                    style={{ background: '#00F5FF', boxShadow: '0 0 6px #00F5FF, 0 0 12px rgba(0,245,255,0.4)' }}
                  />
                )}

                {/* Icon box */}
                <div
                  className={[
                    'flex h-9 w-9 flex-shrink-0 items-center justify-center transition-all duration-200',
                    isActive ? 'text-neon-cyan' : 'text-muted group-hover:text-neon-cyan/70',
                  ].join(' ')}
                  style={{
                    border: '1px solid',
                    borderColor: isActive ? 'rgba(0,245,255,0.3)' : 'rgba(255,255,255,0.06)',
                    background: isActive ? 'rgba(0,245,255,0.08)' : 'rgba(255,255,255,0.02)',
                    ...(isActive ? { boxShadow: '0 0 10px rgba(0,245,255,0.2)' } : {}),
                  }}
                >
                  <Icon size={16} />
                </div>

                <div className="min-w-0 flex-1">
                  <p
                    className={[
                      'font-mono text-xs font-semibold tracking-[0.06em] transition-colors',
                      isActive ? 'text-neon-cyan' : 'text-[#8AABB8] group-hover:text-[#C8E8F0]',
                    ].join(' ')}
                    style={isActive ? { textShadow: '0 0 8px rgba(0,245,255,0.6)' } : undefined}
                  >
                    {label}
                  </p>
                  <p className="font-mono text-[9px] tracking-[0.05em] text-muted mt-0.5 truncate">
                    {caption}
                  </p>
                </div>

                <ChevronRight
                  size={11}
                  className={isActive ? 'text-neon-cyan/60' : 'text-muted/30 group-hover:text-muted/60'}
                />
              </div>
            )}
          </NavLink>
        ))}
      </nav>

      {/* ── Footer ───────────────────────────────────── */}
      <div className="px-4 py-3" style={{ borderTop: '1px solid rgba(0,245,255,0.06)' }}>
        <div
          className="flex items-center justify-between px-3 py-2"
          style={{ border: '1px solid rgba(0,245,255,0.08)', background: 'rgba(0,245,255,0.02)' }}
        >
          <span className="font-mono text-[9px] uppercase tracking-[0.2em] text-muted">BUILD</span>
          <span
            className="font-mono text-[10px] text-neon-cyan/70"
            style={{ textShadow: '0 0 6px rgba(0,245,255,0.4)' }}
          >
            {__APP_VERSION__}
          </span>
        </div>
      </div>
    </aside>
  )
}
