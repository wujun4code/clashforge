import { useEffect, useState } from 'react'
import { NavLink } from 'react-router-dom'
import {
  Activity,
  ChevronRight,
  LayoutDashboard,
  Loader2,
  Network,
  Rss,
  Rocket,
  Server,
  Settings,
  SlidersHorizontal,
  X,
} from 'lucide-react'
import { getOverviewCore } from '../api/client'
import { ThemeSwitcher } from '../theme/ThemeSwitcher'

// Parses versions like "0.1.0-local.20260423.297" or "1.2.3"
function parseVersion(v: string): { base: string; channel: string | null; date: string | null; build: string | null } {
  const m = v.match(/^([^-]+)-([^.]+)\.(\d{8})\.(\d+)$/)
  if (m) return { base: m[1], channel: m[2], date: m[3], build: m[4] }
  return { base: v, channel: null, date: null, build: null }
}

function VersionBadge({ version }: { version: string }) {
  const p = parseVersion(version)
  const isLocal = p.channel === 'local'
  const isPreview = p.channel && p.channel !== 'local' && p.channel !== 'stable'

  return (
    <div className="sidebar-version-row px-1" title={version}>
      <div className="flex items-center justify-between gap-1.5">
        <span className="text-[9px] font-semibold uppercase tracking-[0.18em] text-white/30">Version</span>
        <span className="font-mono text-[11px] font-medium text-white/60">{p.base}</span>
      </div>
      {(isLocal || isPreview) && p.build && (
        <div className="mt-1 flex items-center justify-end gap-1.5">
          {isLocal && (
            <span className="rounded px-1 py-px text-[9px] font-bold uppercase tracking-wider bg-amber-400/15 text-amber-300/80">
              local
            </span>
          )}
          {isPreview && (
            <span className="rounded px-1 py-px text-[9px] font-bold uppercase tracking-wider bg-sky-400/15 text-sky-300/80">
              {p.channel}
            </span>
          )}
          {p.date && (
            <span className="font-mono text-[10px] text-white/35">
              {p.date.slice(0, 4)}-{p.date.slice(4, 6)}-{p.date.slice(6, 8)}
            </span>
          )}
          <span className="font-mono text-[11px] font-semibold text-white/55">
            #{p.build}
          </span>
        </div>
      )}
    </div>
  )
}

type CoreState = 'running' | 'stopped' | 'checking'

type NavItem = { to: string; icon: React.ElementType; label: string; caption: string }

const navGroups: { label: string; items: NavItem[] }[] = [
  {
    label: '路由引擎',
    items: [
      { to: '/',           icon: LayoutDashboard,  label: '概览',   caption: '运行状态 · 核心监控' },
      { to: '/config',     icon: SlidersHorizontal, label: '配置管理', caption: '来源导入 · 运行配置 · 路由数据' },
      { to: '/device-rules', icon: Network,         label: '设备分流', caption: '设备出口 · 策略覆盖' },
    ],
  },
  {
    label: '代理资源',
    items: [
      { to: '/nodes',   icon: Server,    label: '出口节点', caption: '服务器部署 · 代理管理' },
      { to: '/publish', icon: Rss,       label: '订阅定制', caption: '模板编排 · 链接生成' },
    ],
  },
  {
    label: '系统',
    items: [
      { to: '/activity', icon: Activity,  label: '活动日志', caption: '连接轨迹 · 实时日志' },
      { to: '/settings', icon: Settings,  label: '高级管理', caption: '系统参数 · 重置' },
    ],
  },
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

export function Sidebar({ mobileOpen = false, onClose }: { mobileOpen?: boolean; onClose?: () => void }) {
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
      className={[
        'app-sidebar flex h-screen flex-shrink-0 flex-col border-r border-white/[0.06]',
        'fixed inset-y-0 left-0 z-50 transition-transform duration-300 ease-in-out',
        mobileOpen ? 'translate-x-0' : '-translate-x-full',
        'md:static md:translate-x-0',
      ].join(' ')}
      style={{
        width: 'var(--sidebar-width)',
        background: 'linear-gradient(180deg, rgb(var(--surface-1)) 0%, rgb(var(--surface-0)) 100%)',
      }}
    >
      {/* ── Logo ──────────────────────────────────────────── */}
      <div className="sidebar-brand flex items-center gap-3 border-b border-white/[0.06] px-4 py-4">
        <div
          className="sidebar-logo-mark flex h-9 w-9 flex-shrink-0 items-center justify-center border border-brand/20 bg-brand/[0.09]"
          style={{
            borderRadius: 'var(--radius-md)',
            boxShadow: 'var(--shadow-glow-brand-sm)',
          }}
        >
          <img src="/favicon.svg" alt="ClashForge" className="h-[18px] w-[18px]" />
        </div>
        <div className="min-w-0 flex-1">
          <p
            className="font-heading text-[15px] font-semibold leading-none tracking-tight text-white"
          >
            ClashForge
          </p>
          <p className="mt-1.5 text-[9.5px] font-medium uppercase tracking-[0.22em] text-brand-light/45">
            Control Studio
          </p>
        </div>
        {/* Mobile close button */}
        <button
          className="md:hidden ml-2 flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-lg text-white/40 transition-colors hover:bg-white/[0.06] hover:text-white"
          onClick={onClose}
          aria-label="关闭菜单"
        >
          <X size={16} />
        </button>
      </div>

      {/* ── Proxy Service Card ────────────────────────────── */}
      <div className="px-3 pt-3">
        <NavLink to="/setup" className="group block cursor-pointer" onClick={onClose}>
          {({ isActive }) => (
            <div
              className={[
                'sidebar-service-card relative flex items-center gap-3 border px-3 py-2.5 transition-all duration-200',
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

      {/* ── Nav groups ────────────────────────────────────── */}
      <nav className="sidebar-nav flex flex-1 flex-col overflow-y-auto px-2 pb-2">
        {navGroups.map((group, gi) => (
          <div key={group.label}>
            {/* Section label */}
            <div className={`sidebar-nav-label mx-2 flex items-center gap-2 ${gi === 0 ? 'mt-3' : 'mt-2'} mb-0.5`}>
              <div className="h-px flex-1 bg-white/[0.05]" />
              <span className="text-[9px] font-semibold uppercase tracking-[0.28em] text-white/20">
                {group.label}
              </span>
              <div className="h-px flex-1 bg-white/[0.05]" />
            </div>

            {/* Items */}
            <div className="flex flex-col gap-0.5 py-1">
              {group.items.map(({ to, icon: Icon, label, caption }) => (
                <NavLink
                  key={to}
                  to={to}
                  end={to === '/'}
                  onClick={onClose}
                  className={({ isActive }) =>
                    [
                      'sidebar-nav-item sidebar-nav-link group relative flex items-center gap-2.5 px-2.5 py-2 transition-all duration-150 cursor-pointer',
                      isActive
                        ? 'sidebar-nav-link-active text-white bg-brand/[0.07]'
                        : 'text-white/50 hover:bg-white/[0.04] hover:text-white/80',
                    ].join(' ')
                  }
                  style={{ borderRadius: 'var(--radius-md)' }}
                >
                  {({ isActive }) => (
                    <>
                      {isActive && (
                        <span
                          className="sidebar-nav-active-bar absolute left-0 top-1.5 bottom-1.5 w-[2px] rounded-r-full bg-brand"
                          style={{ boxShadow: 'var(--shadow-glow-brand-sm)' }}
                        />
                      )}

                      <div
                        className={`sidebar-nav-icon flex h-[34px] w-[34px] flex-shrink-0 items-center justify-center transition-colors ${
                          isActive
                            ? 'bg-brand/14 text-brand-light'
                            : 'bg-white/[0.045] text-white/38 group-hover:bg-white/[0.07] group-hover:text-white/65'
                        }`}
                        style={{ borderRadius: '6px' }}
                      >
                        <Icon size={15} />
                      </div>

                      <div className="min-w-0 flex-1 py-[1px]">
                        <p className="text-[13px] font-semibold leading-[1.24]">{label}</p>
                        <p className="mt-1 text-[10.5px] leading-[1.32] text-white/30 truncate group-hover:text-white/42">
                          {caption}
                        </p>
                      </div>

                      <ChevronRight
                        size={12}
                        className={`flex-shrink-0 transition-colors ${
                          isActive ? 'text-brand-light/55' : 'text-white/18 group-hover:text-white/38'
                        }`}
                      />
                    </>
                  )}
                </NavLink>
              ))}
            </div>
          </div>
        ))}
      </nav>

      {/* ── Footer ────────────────────────────────────────── */}
      <div className="sidebar-footer border-t border-white/[0.06] px-3 pb-4 pt-3 space-y-2">
        <ThemeSwitcher />
        <VersionBadge version={__APP_VERSION__} />
      </div>
    </aside>
  )
}
