import { useEffect, useState } from 'react'
import { NavLink } from 'react-router-dom'
import {
  Activity,
  ChevronRight,
  FolderCog,
  LayoutDashboard,
  Rocket,
  Settings,
} from 'lucide-react'
import { getOverviewCore } from '../api/client'

const setupLink = {
  to: '/setup',
  icon: Rocket,
  label: '代理服务',
}

const navLinks = [
  {
    to: '/',
    icon: LayoutDashboard,
    label: '概览',
    caption: '运行状态 · 核心监控 · 资源总览',
  },
  {
    to: '/config',
    icon: FolderCog,
    label: '配置管理',
    caption: '订阅、规则、运行配置与文件资产',
  },
  {
    to: '/activity',
    icon: Activity,
    label: '活动日志',
    caption: '连接轨迹、实时日志与问题定位',
  },
  {
    to: '/settings',
    icon: Settings,
    label: '高级管理',
    caption: '核心参数、系统级行为与重置操作',
  },
]

function CoreStatusBadge() {
  const [state, setState] = useState<'running' | 'stopped' | 'checking'>('checking')

  useEffect(() => {
    const check = () => {
      getOverviewCore()
        .then((d) => setState(d.core.state === 'running' ? 'running' : 'stopped'))
        .catch(() => setState('stopped'))
    }
    check()
    const id = setInterval(check, 5000)
    return () => clearInterval(id)
  }, [])

  const config = {
    running: {
      dot: 'status-orb status-orb-online',
      title: '系统在线',
      subtitle: 'ClashForge 与内核当前可交互',
      label: '运行中',
      labelClass: 'badge-success',
    },
    stopped: {
      dot: 'status-orb status-orb-offline',
      title: '等待启动',
      subtitle: '可通过配置向导重新拉起内核',
      label: '未启动',
      labelClass: 'badge-danger',
    },
    checking: {
      dot: 'status-orb status-orb-idle',
      title: '状态检测中',
      subtitle: '正在同步运行状态与进程信息',
      label: '检测中',
      labelClass: 'badge-warning',
    },
  }[state]

  return (
    <div className="mx-3 mt-3 rounded-[24px] border border-white/[0.08] bg-[linear-gradient(180deg,rgba(255,255,255,0.06),rgba(255,255,255,0.03))] p-4 shadow-glass backdrop-blur-xl">
      <div className="flex items-start gap-3">
        <span className={config.dot} aria-hidden>
          {state === 'running' ? <span className="absolute inset-0 animate-ping rounded-full bg-success opacity-35" /> : null}
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex items-center justify-between gap-2">
            <p className="text-sm font-semibold text-white">{config.title}</p>
            <span className={`badge ${config.labelClass}`}>{config.label}</span>
          </div>
          <p className="mt-1 text-xs leading-5 text-muted">{config.subtitle}</p>
        </div>
      </div>
    </div>
  )
}

export function Sidebar() {
  return (
    <aside className="flex h-screen w-[290px] flex-shrink-0 flex-col border-r border-white/[0.06] bg-[linear-gradient(180deg,rgba(11,10,26,0.98),rgba(15,12,36,0.96))] backdrop-blur-2xl">
      <div className="border-b border-white/[0.05] px-5 py-5">
        <div className="rounded-[26px] border border-white/[0.08] bg-white/[0.03] p-4 shadow-glass">
          <div className="flex items-center gap-3">
            <div className="relative flex h-12 w-12 items-center justify-center rounded-2xl border border-brand/20 bg-brand-subtle shadow-glow">
              <img src="/favicon.svg" alt="ClashForge" className="relative z-10 h-7 w-7" />
              <div className="absolute inset-0 rounded-2xl bg-[radial-gradient(circle,rgba(167,139,250,0.28),transparent_62%)]" />
            </div>
            <div className="min-w-0">
              <p className="font-heading text-lg font-semibold tracking-tight text-white">ClashForge</p>
              <p className="mt-1 text-[11px] uppercase tracking-[0.24em] text-brand-light">Network Control Studio</p>
            </div>
          </div>
          <p className="mt-4 text-xs leading-6 text-[#B9B6D3]">
            更亮眼、更流畅的代理控制面板，把配置、检测与运行态统一放进一条工作流里。
          </p>
        </div>
      </div>

      <nav className="flex-1 space-y-2 overflow-y-auto px-3 py-4">
        {/* Proxy Service — primary entry, always visually prominent */}
        <NavLink to={setupLink.to} className="group block">
          {({ isActive }) => {
            const Icon = setupLink.icon
            return (
              <div
                className={[
                  'relative rounded-[22px] p-[1.5px] transition-all duration-200',
                  isActive
                    ? 'bg-gradient-to-br from-white/90 via-violet-200/80 to-purple-300/70 shadow-[0_0_28px_rgba(167,139,250,0.45)]'
                    : 'bg-gradient-to-br from-violet-500/40 via-purple-500/25 to-indigo-500/30 shadow-[0_0_10px_rgba(139,92,246,0.12)] group-hover:from-violet-400/60 group-hover:to-indigo-500/45 group-hover:shadow-[0_0_20px_rgba(139,92,246,0.25)]',
                ].join(' ')}
              >
                <div
                  className={[
                    'flex items-center gap-4 rounded-[21px] px-4 py-4 transition-all duration-200',
                    isActive
                      ? 'bg-[linear-gradient(135deg,rgba(139,92,246,0.55),rgba(99,102,241,0.40))]'
                      : 'bg-[linear-gradient(135deg,rgba(109,40,217,0.10),rgba(79,70,229,0.06))] group-hover:bg-[linear-gradient(135deg,rgba(109,40,217,0.18),rgba(79,70,229,0.13))]',
                  ].join(' ')}
                >
                  <div
                    className={[
                      'relative flex h-14 w-14 flex-shrink-0 items-center justify-center rounded-2xl border transition-all duration-200',
                      isActive
                        ? 'border-white/40 bg-white/20 text-white shadow-[0_0_20px_rgba(255,255,255,0.25)]'
                        : 'border-violet-500/25 bg-[linear-gradient(135deg,rgba(139,92,246,0.16),rgba(99,102,241,0.10))] text-violet-400/70 group-hover:border-violet-400/40 group-hover:text-violet-200',
                    ].join(' ')}
                  >
                    <Icon size={26} className="relative z-10" />
                    {isActive && <span className="absolute inset-0 rounded-2xl bg-[radial-gradient(circle,rgba(255,255,255,0.20),transparent_65%)]" />}
                  </div>
                  <div className="min-w-0 flex-1">
                    <span className={['text-base font-bold', isActive ? 'text-white' : 'text-violet-300/70 group-hover:text-violet-100'].join(' ')}>
                      {setupLink.label}
                    </span>
                  </div>
                  <ChevronRight
                    size={15}
                    className={isActive ? 'text-white/70' : 'text-violet-500/40 group-hover:text-violet-300/70'}
                  />
                </div>
              </div>
            )
          }}
        </NavLink>

        {/* Divider */}
        <div className="flex items-center gap-2 px-1 py-1">
          <div className="h-px flex-1 bg-white/[0.06]" />
          <span className="text-[10px] uppercase tracking-[0.22em] text-white/20">功能导航</span>
          <div className="h-px flex-1 bg-white/[0.06]" />
        </div>

        {/* Regular nav links */}
        {navLinks.map(({ to, icon: Icon, label, caption }) => (
          <NavLink
            key={to}
            to={to}
            end={to === '/'}
            className={({ isActive }) =>
              [
                'group relative flex items-center gap-3 rounded-[22px] border px-4 py-3.5 transition-all duration-200',
                isActive
                  ? 'border-brand/25 bg-brand-subtle text-white shadow-glow'
                  : 'border-transparent text-[#B6B2D2] hover:border-white/[0.08] hover:bg-white/[0.04] hover:text-white',
              ].join(' ')
            }
          >
            {({ isActive }) => (
              <>
                <div
                  className={[
                    'relative flex h-11 w-11 items-center justify-center rounded-2xl border transition-all duration-200',
                    isActive
                      ? 'border-brand/25 bg-brand-subtle text-brand-light shadow-glow'
                      : 'border-white/[0.08] bg-white/[0.03] text-muted group-hover:border-white/[0.14] group-hover:text-white',
                  ].join(' ')}
                >
                  <Icon size={18} className="relative z-10" />
                  {isActive ? <span className="absolute inset-0 rounded-2xl bg-[radial-gradient(circle,rgba(167,139,250,0.25),transparent_68%)]" /> : null}
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex items-center justify-between gap-2">
                    <span className="text-sm font-semibold">{label}</span>
                    <ChevronRight size={14} className={isActive ? 'text-brand-light' : 'text-white/25 group-hover:text-white/55'} />
                  </div>
                  <p className="mt-1 line-clamp-2 text-xs leading-5 text-muted group-hover:text-[#CAC7E6]">{caption}</p>
                </div>
              </>
            )}
          </NavLink>
        ))}
      </nav>

      <CoreStatusBadge />

      <div className="border-t border-white/[0.05] px-5 py-4">
        <div className="rounded-2xl border border-white/[0.08] bg-white/[0.03] px-4 py-3">
          <p className="text-[10px] uppercase tracking-[0.24em] text-muted">Version</p>
          <p className="mt-1 font-mono text-xs text-[#D9D7EA]">{__APP_VERSION__}</p>
        </div>
      </div>
    </aside>
  )
}
