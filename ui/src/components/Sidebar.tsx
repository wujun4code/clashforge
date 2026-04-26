import { useEffect, useState } from 'react'
import { NavLink } from 'react-router-dom'
import {
  Activity,
  ChevronRight,
  FolderCog,
  LayoutDashboard,
  Rocket,
  Server,
  Settings,
} from 'lucide-react'
import { getOverviewCore } from '../api/client'

type CoreState = 'running' | 'stopped' | 'checking'

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
    to: '/nodes',
    icon: Server,
    label: '节点管理',
    caption: '远程服务器 · GOST 部署 · 销毁清理 · 证书管理',
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

const statusStyle: Record<CoreState, {
  border: string
  bg: string
  iconBorder: string
  iconBg: string
  iconText: string
  iconGlow: string
  outerGlow: string
  textColor: string
  badge: string
  badgeText: string
  dot: string
  dotPulse?: string
}> = {
  running: {
    border: 'from-emerald-400/70 via-green-400/50 to-teal-400/60',
    outerGlow: 'shadow-[0_0_22px_rgba(52,211,153,0.30)]',
    bg: 'bg-[linear-gradient(135deg,rgba(16,185,129,0.22),rgba(20,184,166,0.15))]',
    iconBorder: 'border-emerald-400/40',
    iconBg: 'bg-[linear-gradient(135deg,rgba(16,185,129,0.45),rgba(20,184,166,0.35))]',
    iconText: 'text-white',
    iconGlow: 'shadow-[0_0_18px_rgba(52,211,153,0.45)]',
    textColor: 'text-emerald-100',
    badge: 'bg-emerald-500/20 ring-emerald-400/30 text-emerald-300',
    badgeText: '运行中',
    dot: 'bg-emerald-400',
    dotPulse: 'bg-emerald-400',
  },
  stopped: {
    border: 'from-rose-500/50 via-red-500/35 to-orange-500/30',
    outerGlow: 'shadow-[0_0_14px_rgba(244,63,94,0.18)]',
    bg: 'bg-[linear-gradient(135deg,rgba(244,63,94,0.14),rgba(239,68,68,0.08))]',
    iconBorder: 'border-rose-500/30',
    iconBg: 'bg-[linear-gradient(135deg,rgba(244,63,94,0.28),rgba(239,68,68,0.18))]',
    iconText: 'text-rose-300',
    iconGlow: '',
    textColor: 'text-rose-200/80',
    badge: 'bg-rose-500/20 ring-rose-400/30 text-rose-300',
    badgeText: '未启动',
    dot: 'bg-rose-400',
  },
  checking: {
    border: 'from-amber-500/40 via-yellow-500/25 to-orange-500/30',
    outerGlow: 'shadow-[0_0_12px_rgba(245,158,11,0.15)]',
    bg: 'bg-[linear-gradient(135deg,rgba(245,158,11,0.12),rgba(234,179,8,0.07))]',
    iconBorder: 'border-amber-500/30',
    iconBg: 'bg-[linear-gradient(135deg,rgba(245,158,11,0.25),rgba(234,179,8,0.15))]',
    iconText: 'text-amber-300',
    iconGlow: '',
    textColor: 'text-amber-200/70',
    badge: 'bg-amber-500/20 ring-amber-400/30 text-amber-300',
    badgeText: '检测中',
    dot: 'bg-amber-400',
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

  const ss = statusStyle[coreState]

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
        {/* Proxy Service — primary entry + live status */}
        <NavLink to="/setup" className="group block">
          {({ isActive }) => (
            <div
              className={[
                'relative rounded-[22px] p-[1.5px] transition-all duration-200',
                `bg-gradient-to-br ${ss.border}`,
                isActive
                  ? 'shadow-[0_0_28px_rgba(167,139,250,0.35)]'
                  : `${ss.outerGlow} group-hover:shadow-[0_0_22px_rgba(167,139,250,0.22)]`,
              ].join(' ')}
            >
              <div className={['flex items-center gap-4 rounded-[21px] px-4 py-4 transition-all duration-200', ss.bg].join(' ')}>
                {/* Icon with status-driven color */}
                <div
                  className={[
                    'relative flex h-14 w-14 flex-shrink-0 items-center justify-center rounded-2xl border transition-all duration-200',
                    ss.iconBorder, ss.iconBg, ss.iconText, ss.iconGlow,
                  ].join(' ')}
                >
                  <Rocket size={26} className="relative z-10" />
                  {/* Pulsing ring when running */}
                  {coreState === 'running' && (
                    <span className="absolute -inset-[3px] animate-ping rounded-2xl bg-emerald-400/20" />
                  )}
                </div>

                {/* Label + status badge */}
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className={['text-base font-bold', isActive ? 'text-white' : ss.textColor, 'group-hover:text-white transition-colors'].join(' ')}>
                      代理服务
                    </span>
                    <span className={['rounded-full px-2 py-0.5 text-[10px] font-semibold ring-1', ss.badge].join(' ')}>
                      {ss.badgeText}
                    </span>
                  </div>
                </div>

                <ChevronRight
                  size={15}
                  className={isActive ? 'text-white/70' : 'text-white/25 group-hover:text-white/55'}
                />
              </div>
            </div>
          )}
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

      <div className="border-t border-white/[0.05] px-5 py-4">
        <div className="rounded-2xl border border-white/[0.08] bg-white/[0.03] px-4 py-3">
          <p className="text-[10px] uppercase tracking-[0.24em] text-muted">Version</p>
          <p className="mt-1 font-mono text-xs text-[#D9D7EA]">{__APP_VERSION__}</p>
        </div>
      </div>
    </aside>
  )
}
