import { useEffect, useState } from 'react'
import { NavLink } from 'react-router-dom'
import {
  Activity,
  ChevronRight,
  FolderCog,
  LayoutDashboard,
  Settings,
  Sparkles,
  Wifi,
  WifiOff,
  Loader,
  PawPrint,
  WandSparkles,
} from 'lucide-react'
import { getOverviewCore } from '../api/client'
import { CatMascot } from './CatMascot'

type CoreState = 'running' | 'stopped' | 'checking'

const navLinks = [
  {
    to: '/',
    icon: LayoutDashboard,
    label: '猫猫控制台',
    caption: '状态 · 速率 · 资源 · 节点切换',
    accent: 'from-violet-500/15 to-fuchsia-500/10',
  },
  {
    to: '/config',
    icon: FolderCog,
    label: '配置工坊',
    caption: '订阅 · 规则 · 配置来源 · 预览',
    accent: 'from-cyan-500/15 to-indigo-500/10',
  },
  {
    to: '/activity',
    icon: Activity,
    label: '流量侦查室',
    caption: '连接链路 · 日志流 · 故障线索',
    accent: 'from-amber-500/15 to-orange-500/10',
  },
  {
    to: '/settings',
    icon: Settings,
    label: '系统猫箱',
    caption: '核心参数 · 安全操作 · 重置',
    accent: 'from-rose-500/15 to-fuchsia-500/10',
  },
]

const statusConfig: Record<CoreState, {
  label: string
  hint: string
  color: string
  borderColor: string
  bgColor: string
  iconColor: string
  StatusIcon: typeof Wifi
  mood: 'default' | 'sleepy' | 'excited'
}> = {
  running: {
    label: '猫猫在线巡航',
    hint: '核心正在处理连接与分流规则',
    color: 'text-emerald-300',
    borderColor: 'border-emerald-400/35',
    bgColor: 'bg-emerald-400/10',
    iconColor: 'text-emerald-300',
    StatusIcon: Wifi,
    mood: 'excited',
  },
  stopped: {
    label: '猫猫暂停值守',
    hint: '核心未运行，可前往向导重新启动',
    color: 'text-rose-300',
    borderColor: 'border-rose-400/35',
    bgColor: 'bg-rose-400/10',
    iconColor: 'text-rose-300',
    StatusIcon: WifiOff,
    mood: 'sleepy',
  },
  checking: {
    label: '猫猫确认状态中',
    hint: '正在同步核心运行信息',
    color: 'text-amber-200',
    borderColor: 'border-amber-300/35',
    bgColor: 'bg-amber-300/10',
    iconColor: 'text-amber-200',
    StatusIcon: Loader,
    mood: 'default',
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
    <aside className="cat-sidebar-shell flex h-screen w-[308px] flex-shrink-0 flex-col overflow-hidden">
      <div className="cat-sidebar-grid pointer-events-none absolute inset-y-0 left-0 w-[308px]" />

      <div className="relative px-4 pb-4 pt-5">
        <div className="cat-brand-card">
          <div className="flex items-start gap-3">
            <div className="cat-brand-mascot">
              <CatMascot size={54} mood={ss.mood} />
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <p className="cat-brand-title">ClashForge</p>
                <span className="cat-mini-chip">
                  <Sparkles size={10} />
                  Alpha UI
                </span>
              </div>
              <p className="cat-brand-subtitle">极客网络工坊 × 萌系猫猫终端</p>
              <p className="cat-brand-version">build {__APP_VERSION__}</p>
            </div>
          </div>

          <div className="cat-brand-copy">
            <div className="cat-copy-line">
              <PawPrint size={11} />
              <span>更亮眼的状态观察、更顺手的导航层级、更可爱的控制语气。</span>
            </div>
            <div className="cat-copy-line">
              <WandSparkles size={11} />
              <span>保留工程师质感，同时把 Clash 的猫猫气质做成品牌体验。</span>
            </div>
          </div>
        </div>
      </div>

      <div className="relative px-4 pb-4">
        <NavLink to="/setup" className="group block cursor-pointer">
          <div className={`cat-status-card ${ss.bgColor} ${ss.borderColor}`}>
            <div className="flex items-center gap-3">
              <div className={`cat-status-icon ${ss.bgColor} ${ss.borderColor} ${ss.iconColor}`}>
                <ss.StatusIcon size={18} className={coreState === 'checking' ? 'animate-spin' : coreState === 'running' ? 'animate-pulse-soft' : ''} />
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="cat-status-eyebrow">猫猫值守状态</span>
                  {coreState === 'running' && <span className="cat-status-pulse" />}
                </div>
                <p className={`cat-status-title ${ss.color}`}>{ss.label}</p>
                <p className="cat-status-hint">{ss.hint}</p>
              </div>
              <ChevronRight size={15} className="text-slate-300/60 transition-transform duration-200 group-hover:translate-x-0.5 group-hover:text-white" />
            </div>
          </div>
        </NavLink>
      </div>

      <nav className="relative flex-1 overflow-y-auto px-3 pb-4 pt-1">
        <div className="cat-nav-section-label">
          <span className="cat-divider" />
          <span>主导航</span>
          <span className="cat-divider" />
        </div>

        <div className="space-y-2">
          {navLinks.map(({ to, icon: Icon, label, caption, accent }) => (
            <NavLink key={to} to={to} end={to === '/'}>
              {({ isActive }) => (
                <div className={`cat-nav-card ${isActive ? 'cat-nav-card-active' : ''}`}>
                  <div className={`cat-nav-accent bg-gradient-to-r ${accent}`} />
                  <div className={`cat-nav-icon ${isActive ? 'cat-nav-icon-active' : ''}`}>
                    <Icon size={17} />
                  </div>
                  <div className="min-w-0 flex-1">
                    <p className={`cat-nav-title ${isActive ? 'cat-nav-title-active' : ''}`}>{label}</p>
                    <p className="cat-nav-caption">{caption}</p>
                  </div>
                  <ChevronRight size={13} className={`transition-all duration-200 ${isActive ? 'translate-x-0 text-violet-200' : 'text-slate-400/50 group-hover:text-slate-200'}`} />
                </div>
              )}
            </NavLink>
          ))}
        </div>
      </nav>

      <div className="relative px-4 pb-5 pt-2">
        <div className="cat-sidebar-footer">
          <p className="cat-footer-title">猫猫提示</p>
          <p className="cat-footer-copy">
            推荐先从「猫猫控制台」查看整体状态，再去「配置工坊」或「流量侦查室」做细节操作。
          </p>
        </div>
      </div>
    </aside>
  )
}
