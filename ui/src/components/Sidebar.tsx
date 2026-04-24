import { useEffect, useState } from 'react'
import { NavLink } from 'react-router-dom'
import {
  Activity,
  ChevronRight,
  FolderCog,
  LayoutDashboard,
  Settings,
  Sparkles,
} from 'lucide-react'
import { getOverviewCore } from '../api/client'

const links = [
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
    to: '/setup',
    icon: Sparkles,
    label: '配置向导',
    caption: '导入配置、调优网络并快速上线',
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
        {links.map(({ to, icon: Icon, label, caption }) => (
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
