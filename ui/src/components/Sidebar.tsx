import { useEffect, useState } from 'react'
import { NavLink } from 'react-router-dom'
import {
  Activity,
  ChevronRight,
  FolderCog,
  LayoutDashboard,
  Loader2,
  Rocket,
  Server,
  Settings,
} from 'lucide-react'
import { getOverviewCore } from '../api/client'
import { ThemeSwitcher } from '../theme/ThemeSwitcher'

type CoreState = 'running' | 'stopped' | 'checking'

const navLinks = [
  { to: '/',         icon: LayoutDashboard, label: '概览',   caption: '运行状态 · 核心监控' },
  { to: '/config',   icon: FolderCog,       label: '配置管理', caption: '订阅 · 规则 · 运行配置' },
  { to: '/nodes',    icon: Server,          label: '节点管理', caption: '远程部署 · 证书' },
  { to: '/activity', icon: Activity,        label: '活动日志', caption: '连接轨迹 · 实时日志' },
  { to: '/settings', icon: Settings,        label: '高级管理', caption: '系统参数 · 重置' },
]

const coreStatus = {
  running: {
    dot:       'bg-emerald-400 shadow-[0_0_7px_rgba(52,211,153,0.80)]',
    pulseDot:  true,
    iconBg:    'bg-emerald-500/12 text-emerald-300',
    badge:     'bg-emerald-500/12 text-emerald-300 ring-1 ring-inset ring-emerald-500/22',
    badgeText: '运行中',
    border:    'border-emerald-500/18',
    bg:        'bg-emerald-500/[0.03]',
  },
  stopped: {
    dot:       'bg-rose-500 shadow-[0_0_6px_rgba(244,63,94,0.70)]',
    pulseDot:  false,
    iconBg:    'bg-rose-500/10 text-rose-300',
    badge:     'bg-rose-500/10 text-rose-300 ring-1 ring-inset ring-rose-500/20',
    badgeText: '未启动',
    border:    'border-rose-500/14',
    bg:        'bg-rose-500/[0.025]',
  },
  checking: {
    dot:       'bg-amber-400/60',
    pulseDot:  false,
    iconBg:    'bg-white/[0.05] text-white/35',
    badge:     'bg-amber-500/10 text-amber-300 ring-1 ring-inset ring-amber-500/20',
    badgeText: '检测中',
    border:    'border-white/[0.07]',
    bg:        '',
  },
} as const

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

  const sc = coreStatus[coreState]

  return (
    <aside
      className="flex h-screen flex-shrink-0 flex-col border-r border-white/[0.06]"
      style={{
        width: 'var(--sidebar-width)',
        background: 'linear-gradient(180deg, rgb(var(--surface-1)) 0%, rgb(var(--surface-0)) 100%)',
      }}
    >
      {/* ── Logo ──────────────────────────────────────────── */}
      <div className="flex items-center gap-3 border-b border-white/[0.06] px-4 py-4">
        <div
          className="flex h-9 w-9 flex-shrink-0 items-center justify-center border border-brand/20 bg-brand/[0.09]"
          style={{
            borderRadius: 'var(--radius-md)',
            boxShadow: '0 0 16px rgba(139,92,246,0.22)',
          }}
        >
          <img src="/favicon.svg" alt="ClashForge" className="h-[18px] w-[18px]" />
        </div>
        <div className="min-w-0">
          <p
            className="font-heading text-[15px] font-semibold leading-none tracking-tight text-white"
          >
            ClashForge
          </p>
          <p className="mt-1.5 text-[9.5px] font-medium uppercase tracking-[0.22em] text-brand-light/45">
            Control Studio
          </p>
        </div>
      </div>

      {/* ── Proxy Service Card ────────────────────────────── */}
      <div className="px-3 pt-3">
        <NavLink to="/setup" className="group block cursor-pointer">
          {({ isActive }) => (
            <div
              className={[
                'relative flex items-center gap-3 border px-3 py-2.5 transition-all duration-200',
                isActive ? 'border-brand/28 bg-brand/[0.07]' : `${sc.border} ${sc.bg}`,
              ].join(' ')}
              style={{ borderRadius: 'var(--radius-lg)' }}
            >
              {/* Icon + status dot */}
              <div className="relative flex-shrink-0">
                <div
                  className={`flex h-10 w-10 items-center justify-center transition-colors ${sc.iconBg}`}
                  style={{ borderRadius: 'var(--radius-md)' }}
                >
                  {coreState === 'checking' ? (
                    <Loader2 size={17} className="animate-spin" />
                  ) : (
                    <Rocket size={17} />
                  )}
                </div>
                {/* Live dot */}
                <span
                  className={`absolute -right-0.5 -top-0.5 h-2.5 w-2.5 rounded-full ${sc.dot}`}
                />
                {sc.pulseDot && (
                  <span className="absolute -right-0.5 -top-0.5 h-2.5 w-2.5 animate-ping rounded-full bg-emerald-400/40" />
                )}
              </div>

              {/* Label + badge */}
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-1.5">
                  <span className="text-[13px] font-semibold text-white/90 leading-none">
                    代理服务
                  </span>
                  <span
                    className={`flex-shrink-0 rounded-full px-2 py-0.5 text-[10px] font-semibold leading-none whitespace-nowrap ${sc.badge}`}
                  >
                    {sc.badgeText}
                  </span>
                </div>
                <p className="mt-1.5 text-[10.5px] leading-none text-white/30">
                  启动 · 停止 · 实时监控
                </p>
              </div>

              <ChevronRight
                size={13}
                className="flex-shrink-0 text-white/22 transition-colors group-hover:text-white/50"
              />
            </div>
          )}
        </NavLink>
      </div>

      {/* ── Nav section label ─────────────────────────────── */}
      <div className="mx-4 mt-3 flex items-center gap-2">
        <div className="h-px flex-1 bg-white/[0.05]" />
        <span className="text-[9px] font-semibold uppercase tracking-[0.28em] text-white/20">
          功能导航
        </span>
        <div className="h-px flex-1 bg-white/[0.05]" />
      </div>

      {/* ── Nav links ─────────────────────────────────────── */}
      <nav className="flex flex-1 flex-col gap-0.5 overflow-y-auto px-2 py-1.5">
        {navLinks.map(({ to, icon: Icon, label, caption }) => (
          <NavLink
            key={to}
            to={to}
            end={to === '/'}
          >
            {({ isActive }) => (
              <div
                className={[
                  'group relative flex items-center gap-2.5 px-2.5 py-2 transition-all duration-150 cursor-pointer',
                  isActive
                    ? 'text-white bg-brand/[0.07]'
                    : 'text-white/50 hover:bg-white/[0.04] hover:text-white/80',
                ].join(' ')}
                style={{ borderRadius: 'var(--radius-md)' }}
              >
                {/* Active left accent bar */}
                {isActive && (
                  <span
                    className="absolute left-0 top-1.5 bottom-1.5 w-[2px] rounded-r-full bg-brand"
                    style={{ boxShadow: '0 0 6px rgba(139,92,246,0.60)' }}
                  />
                )}

                {/* Icon box */}
                <div
                  className={`flex h-[34px] w-[34px] flex-shrink-0 items-center justify-center transition-colors ${
                    isActive
                      ? 'bg-brand/14 text-brand-light'
                      : 'bg-white/[0.045] text-white/38 group-hover:bg-white/[0.07] group-hover:text-white/65'
                  }`}
                  style={{ borderRadius: '6px' }}
                >
                  <Icon size={15} />
                </div>

                {/* Text */}
                <div className="min-w-0 flex-1">
                  <p className="text-[13px] font-semibold leading-none">
                    {label}
                  </p>
                  <p className="mt-[5px] text-[10.5px] leading-none text-white/28 truncate group-hover:text-white/40">
                    {caption}
                  </p>
                </div>

                <ChevronRight
                  size={12}
                  className={`flex-shrink-0 transition-colors ${
                    isActive ? 'text-brand-light/55' : 'text-white/18 group-hover:text-white/38'
                  }`}
                />
              </div>
            )}
          </NavLink>
        ))}
      </nav>

      {/* ── Footer ────────────────────────────────────────── */}
      <div className="border-t border-white/[0.06] px-3 pb-4 pt-3 space-y-2">
        <ThemeSwitcher />
        <div className="flex items-center justify-between px-1">
          <span className="text-[9.5px] font-medium uppercase tracking-[0.20em] text-white/22">
            Version
          </span>
          <span className="font-mono text-[11px] text-white/40">
            {__APP_VERSION__}
          </span>
        </div>
      </div>
    </aside>
  )
}
