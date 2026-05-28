import { useEffect, useState } from 'react'
import { NavLink } from 'react-router-dom'
import {
  Activity,
  ChevronRight,
  CloudCog,
  LayoutDashboard,
  Network,
  Rss,
  Rocket,
  Server,
  Settings,
  SlidersHorizontal,
  Sparkles,
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

type NavItem = { to: string; icon: React.ElementType; label: string; caption: string }

const navGroups: { label: string; items: NavItem[] }[] = [
  {
    label: '路由引擎',
    items: [
      { to: '/',           icon: LayoutDashboard,  label: '概览',   caption: '运行状态 · 核心监控' },
      { to: '/setup',      icon: Rocket,           label: '设置向导', caption: '启动停止 · 透明代理参数' },
      { to: '/config',     icon: SlidersHorizontal, label: '配置管理', caption: '来源导入 · 运行配置 · 路由数据' },
      { to: '/device-rules', icon: Network,         label: '设备分流', caption: '设备出口 · 策略覆盖' },
    ],
  },
  {
    label: '代理资源',
    items: [
      { to: '/nodes',   icon: Server,    label: '出口节点', caption: '服务器部署 · 代理管理' },
      { to: '/publish', icon: Rss,       label: '订阅定制', caption: '模板编排 · 链接生成' },
      { to: '/cloudflare-resources', icon: CloudCog, label: 'CF 资源管理', caption: 'Worker / KV 清理工具' },
    ],
  },
  {
    label: '系统',
    items: [
      { to: '/activity',    icon: Activity,   label: '活动日志', caption: '连接轨迹 · 实时日志' },
      { to: '/settings',    icon: Settings,   label: '高级管理', caption: '系统参数 · 重置' },
    ],
  },
]

export function Sidebar({ mobileOpen = false, onClose }: { mobileOpen?: boolean; onClose?: () => void }) {
  const [coreRunning, setCoreRunning] = useState<boolean | null>(null)

  useEffect(() => {
    const check = () => {
      getOverviewCore()
        .then((d) => setCoreRunning(Boolean(d.core.running)))
        .catch(() => setCoreRunning(false))
    }

    check()
    const id = setInterval(check, 6000)
    return () => clearInterval(id)
  }, [])

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

      {/* ── Context banner: mutually exclusive ────────────── */}
      {coreRunning === false && (
        <div className="px-3 pt-3">
          <NavLink to="/quickstart" className="group block cursor-pointer" onClick={onClose}>
            {({ isActive }) => (
              <div
                className={[
                  'relative overflow-hidden border px-3.5 py-3 transition-all duration-200',
                  isActive
                    ? 'border-brand/55 bg-brand/25'
                    : 'border-brand/45 bg-[linear-gradient(135deg,rgba(74,179,255,0.25)_0%,rgba(74,179,255,0.12)_100%)] hover:border-brand/60 hover:bg-[linear-gradient(135deg,rgba(74,179,255,0.30)_0%,rgba(74,179,255,0.14)_100%)]',
                ].join(' ')}
                style={{ borderRadius: 'var(--radius-lg)', boxShadow: 'var(--shadow-glow-brand-sm)' }}
              >
                <div className="flex items-center gap-3">
                  <div className="flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-lg bg-brand/25 text-brand-light">
                    <Sparkles size={18} />
                  </div>
                  <div className="min-w-0 flex-1">
                    <p className="text-[15px] font-bold leading-none tracking-tight text-white">
                      快速启动
                    </p>
                    <p className="mt-1.5 text-[11px] leading-none text-white/80">
                      服务未启动，推荐先执行一键部署向导
                    </p>
                  </div>
                  <ChevronRight
                    size={14}
                    className="flex-shrink-0 text-brand-light/75 transition-colors group-hover:text-brand-light"
                  />
                </div>
              </div>
            )}
          </NavLink>
        </div>
      )}

      {coreRunning === true && (
        <div className="px-3 pt-3">
          <NavLink to="/service" className="group block cursor-pointer" onClick={onClose}>
            {({ isActive }) => (
              <div
                className={[
                  'relative overflow-hidden border px-3.5 py-3 transition-all duration-200',
                  isActive
                    ? 'border-emerald-500/55 bg-emerald-500/[0.22]'
                    : 'border-emerald-500/40 bg-[linear-gradient(135deg,rgba(16,185,129,0.22)_0%,rgba(16,185,129,0.10)_100%)] hover:border-emerald-500/55 hover:bg-[linear-gradient(135deg,rgba(16,185,129,0.28)_0%,rgba(16,185,129,0.13)_100%)]',
                ].join(' ')}
                style={{ borderRadius: 'var(--radius-lg)', boxShadow: '0 0 18px rgba(16,185,129,0.12)' }}
              >
                <div className="flex items-center gap-3">
                  <div className="relative flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-lg bg-emerald-500/20 text-emerald-400">
                    <Activity size={18} />
                    <span className="absolute -top-0.5 -right-0.5 h-2 w-2 rounded-full bg-emerald-400 shadow-[0_0_6px_rgba(52,211,153,0.8)]" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <p className="text-[15px] font-bold leading-none tracking-tight text-white">
                      代理运行中
                    </p>
                    <p className="mt-1.5 text-[11px] leading-none text-emerald-300/80">
                      内核已启动，点击查看运行状态
                    </p>
                  </div>
                  <ChevronRight
                    size={14}
                    className="flex-shrink-0 text-emerald-400/70 transition-colors group-hover:text-emerald-400"
                  />
                </div>
              </div>
            )}
          </NavLink>
        </div>
      )}

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
