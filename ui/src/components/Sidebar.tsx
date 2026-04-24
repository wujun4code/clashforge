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
import { CatMascot } from './CatMascot'
import { ThemeToggle } from './ThemeToggle'
import { useTheme } from '../contexts/ThemeContext'

type CoreState = 'running' | 'stopped' | 'checking'

const navLinks = [
  {
    to: '/',
    icon: LayoutDashboard,
    label: '概览',
    caption: '状态 · 核心 · 资源',
  },
  {
    to: '/config',
    icon: FolderCog,
    label: '配置',
    caption: '订阅 · 规则 · 运行配置',
  },
  {
    to: '/activity',
    icon: Activity,
    label: '活动',
    caption: '连接 · 日志 · 诊断',
  },
  {
    to: '/settings',
    icon: Settings,
    label: '设置',
    caption: '参数 · 系统 · 重置',
  },
]

const statusConfig: Record<CoreState, {
  label: string
  color: string
  bgColor: string
  borderColor: string
  StatusIcon: typeof Wifi
}> = {
  running: {
    label: '在线',
    color: 'text-success',
    bgColor: 'bg-success/15',
    borderColor: 'border-success/40',
    StatusIcon: Wifi,
  },
  stopped: {
    label: '离线',
    color: 'text-danger',
    bgColor: 'bg-danger/15',
    borderColor: 'border-danger/40',
    StatusIcon: WifiOff,
  },
  checking: {
    label: '检查中',
    color: 'text-warning',
    bgColor: 'bg-warning/15',
    borderColor: 'border-warning/40',
    StatusIcon: Loader,
  },
}

export function Sidebar() {
  const [coreState, setCoreState] = useState<CoreState>('checking')
  const { theme } = useTheme()

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
    <aside className="flex h-screen w-[272px] flex-shrink-0 flex-col bg-surface-0/95 backdrop-blur-2xl border-r border-white/5">
      {/* ── Brand Header ─────────────────────────────── */}
      <div className="px-4 py-5 border-b border-white/5">
        {/* Logo row */}
        <div className="flex items-center gap-3 mb-4">
          <div className="relative flex h-11 w-11 items-center justify-center flex-shrink-0 rounded-2xl bg-gradient-to-br from-brand/25 via-accent-violet/15 to-paw/20 shadow-soft">
            {theme === 'cyberpunk' ? (
              <span
                aria-hidden
                className="font-display text-xl font-black text-brand"
                style={{ textShadow: '0 0 10px rgba(0,245,255,0.6)' }}
              >
                ▲
              </span>
            ) : (
              <CatMascot size={28} />
            )}
          </div>
          <div className="min-w-0">
            <p className="font-display text-lg font-extrabold tracking-tight text-display-gradient leading-none">
              ClashForge
            </p>
            <p className="text-[10px] tracking-[0.18em] text-[color:var(--text-muted)] mt-1 font-mono uppercase">
              v{__APP_VERSION__}
            </p>
          </div>
        </div>

        {/* Status panel */}
        <NavLink to="/setup" className="group block cursor-pointer">
          <div
            className={`relative flex items-center gap-3 px-3 py-2.5 rounded-xl border transition-all duration-200 ${ss.bgColor} ${ss.borderColor} hover:-translate-y-[1px] hover:shadow-soft`}
          >
            <div className={`flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg ${ss.bgColor} border ${ss.borderColor} ${ss.color}`}>
              <ss.StatusIcon
                size={16}
                className={coreState === 'checking' ? 'animate-spin' : coreState === 'running' ? 'animate-pulse-soft' : ''}
              />
            </div>

            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <span className="text-[10px] tracking-wider text-[color:var(--text-muted)]">代理服务</span>
              </div>
              <div className="flex items-center gap-2 mt-0.5">
                <span className={`font-display text-sm font-bold ${ss.color}`}>
                  {ss.label}
                </span>
                {coreState === 'running' && <span className="status-orb status-orb-online" />}
              </div>
            </div>

            <ChevronRight size={14} className={`${ss.color} opacity-60 group-hover:opacity-100 group-hover:translate-x-0.5 transition-all`} />
          </div>
        </NavLink>
      </div>

      {/* ── Navigation ───────────────────────────────── */}
      <nav className="flex-1 overflow-y-auto px-3 py-4 space-y-1">
        <div className="px-2 pb-2">
          <span className="text-[10px] uppercase tracking-[0.18em] text-[color:var(--text-muted)] font-semibold">导航</span>
        </div>

        {navLinks.map(({ to, icon: Icon, label, caption }) => (
          <NavLink key={to} to={to} end={to === '/'}>
            {({ isActive }) => (
              <div
                className={[
                  'group relative flex items-center gap-3 px-3 py-2.5 rounded-xl transition-all duration-200 cursor-pointer border',
                  isActive
                    ? 'bg-brand/10 border-brand/30 shadow-soft'
                    : 'border-transparent hover:bg-surface-2/60 hover:border-white/5 hover:-translate-y-[1px]',
                ].join(' ')}
              >
                {/* Active indicator */}
                {isActive && (
                  <span className="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-6 rounded-r-full bg-brand" />
                )}

                {/* Icon */}
                <div
                  className={[
                    'flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg transition-all duration-200',
                    isActive
                      ? 'bg-brand/15 text-brand border border-brand/30'
                      : 'bg-surface-2/40 text-[color:var(--text-muted)] border border-white/5 group-hover:text-brand/80 group-hover:border-brand/20',
                  ].join(' ')}
                >
                  <Icon size={16} />
                </div>

                <div className="min-w-0 flex-1">
                  <p
                    className={[
                      'font-display text-sm font-bold transition-colors leading-tight',
                      isActive ? 'text-brand' : 'text-[color:var(--text-primary)] group-hover:text-brand/90',
                    ].join(' ')}
                  >
                    {label}
                  </p>
                  <p className="text-[10px] text-[color:var(--text-muted)] mt-0.5 truncate">
                    {caption}
                  </p>
                </div>

                <ChevronRight
                  size={12}
                  className={isActive ? 'text-brand/70' : 'text-[color:var(--text-muted)]/40 group-hover:text-[color:var(--text-muted)]/70 group-hover:translate-x-0.5 transition-all'}
                />
              </div>
            )}
          </NavLink>
        ))}
      </nav>

      {/* ── Footer ───────────────────────────────────── */}
      <div className="px-4 py-3 border-t border-white/5 space-y-2">
        <div className="flex items-center justify-between gap-2">
          <span className="text-[10px] uppercase tracking-[0.18em] text-[color:var(--text-muted)] font-semibold">
            皮肤
          </span>
          <ThemeToggle />
        </div>
        <div className="flex items-center justify-between px-3 py-2 rounded-lg border border-white/5 bg-surface-1/50">
          <span className="font-mono text-[10px] uppercase tracking-wider text-[color:var(--text-muted)]">build</span>
          <span className="font-mono text-[11px] text-brand/80">
            {__APP_VERSION__}
          </span>
        </div>
      </div>
    </aside>
  )
}
