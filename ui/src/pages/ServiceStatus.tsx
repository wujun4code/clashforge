import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import yaml from 'js-yaml'
import {
  Activity, AlertCircle, EyeOff, Loader2, Network, PowerOff, RefreshCw,
  Route, Server, ShieldOff, Wifi,
} from 'lucide-react'
import {
  getOverviewCore, getMihomoConfig, getConfig, stopCore, releaseOverviewTakeover,
} from '../api/client'
import type { OverviewModule } from '../api/client'

// ─── "current effective modes" summary ───────────────────────────────────────
// Everything here used to only be readable by scrolling the raw YAML on the
// right. These tiles translate the handful of settings that actually change
// how traffic/DNS behave into plain-language cards, with the ground-truth
// value read from the *generated* mihomo-config.yaml (not just the user's
// config.toml choice) wherever the two can diverge — e.g. TUN network mode
// silently forces DNS enhanced-mode to fake-ip regardless of what was picked.

type Tone = 'good' | 'neutral' | 'warn'

const TONE_CLASSES: Record<Tone, string> = {
  good: 'text-emerald-300 bg-emerald-500/15 border-emerald-500/30',
  neutral: 'text-sky-300 bg-sky-500/15 border-sky-500/30',
  warn: 'text-amber-300 bg-amber-500/15 border-amber-500/30',
}

interface ModeInfo {
  label: string
  tone: Tone
  description: string
}

const DNS_ENHANCED_MODE_INFO: Record<string, ModeInfo> = {
  'fake-ip': {
    label: 'Fake-IP',
    tone: 'good',
    description: '给域名分配虚拟 IP 把流量引入内核，按域名精确分流；个别应用直连虚拟 IP（如 QUIC 探测）时可能受影响。',
  },
  'redir-host': {
    label: 'Redir-Host',
    tone: 'neutral',
    description: '客户端拿到真实 IP，内核靠嗅探 SNI/Host 判断域名后分流；兼容性更好，但依赖流量嗅探，精度略低于 Fake-IP。',
  },
}

const DNS_STRATEGY_INFO: Record<string, ModeInfo> = {
  legacy: {
    label: '传统模式',
    tone: 'warn',
    description: '不生成 nameserver-policy，查询统一发给同一组 DNS，靠应答后的 GeoIP 兜底判断——配置简单，但分流精度较低。',
  },
  split: {
    label: '分流模式',
    tone: 'good',
    description: '按域名归属在查询时就分流：国内域名走国内 DNS 拿最优 CDN 节点，境外域名走加密 DoH，未知域名兜底保护。',
  },
  privacy: {
    label: '隐私模式',
    tone: 'good',
    description: '在分流模式基础上，所有 DNS 查询都强制走国内加密 DoH，运营商在链路上完全看不到任何查询内容。',
  },
}

const NETWORK_MODE_INFO: Record<string, ModeInfo> = {
  tproxy: {
    label: 'TProxy 透明代理',
    tone: 'good',
    description: '基于 Linux TPROXY 内核特性透明拦截流量，性能开销最小，是路由器场景下的推荐方式。',
  },
  tun: {
    label: 'TUN 虚拟网卡',
    tone: 'neutral',
    description: '创建虚拟网卡接管全部流量，兼容性最强（包括部分 tproxy 处理不好的场景），但会强制 DNS 使用 Fake-IP。',
  },
  redir: {
    label: 'Redirect 重定向',
    tone: 'warn',
    description: '基于 iptables/nftables REDIRECT 规则拦截 TCP 流量；不支持透明代理 UDP（含 QUIC）。',
  },
  none: {
    label: '不接管',
    tone: 'warn',
    description: '不做透明代理，只提供 HTTP/SOCKS 端口，LAN 内设备需要手动配置代理才能生效。',
  },
}

const DNSMASQ_MODE_INFO: Record<string, ModeInfo> = {
  upstream: {
    label: '上游模式',
    tone: 'good',
    description: '保留路由器原有 dnsmasq，把 ClashForge 设为它的上游解析器——对其他依赖 dnsmasq 的功能（DHCP 主机名等）影响最小。',
  },
  replace: {
    label: '替换模式',
    tone: 'neutral',
    description: 'ClashForge 直接顶替 dnsmasq 监听 53 端口——更直接，但会短暂中断依赖原 dnsmasq 的功能。',
  },
  none: {
    label: '不接管',
    tone: 'warn',
    description: '不劫持 DNS 入口，LAN 内设备需要手动把 DNS 服务器指向 ClashForge 才能走分流。',
  },
}

function readStr(obj: unknown, ...path: string[]): string | undefined {
  let cur: unknown = obj
  for (const key of path) {
    if (cur == null || typeof cur !== 'object') return undefined
    cur = (cur as Record<string, unknown>)[key]
  }
  return typeof cur === 'string' ? cur : undefined
}

function readBool(obj: unknown, ...path: string[]): boolean | undefined {
  let cur: unknown = obj
  for (const key of path) {
    if (cur == null || typeof cur !== 'object') return undefined
    cur = (cur as Record<string, unknown>)[key]
  }
  return typeof cur === 'boolean' ? cur : undefined
}

function ModeTile({ icon, label, info, value, note }: {
  icon: React.ReactNode
  label: string
  info?: ModeInfo
  value: string
  note?: string
}) {
  const tone = info?.tone ?? 'neutral'
  return (
    <div className="rounded-xl border border-white/[0.08] bg-black/15 px-4 py-3.5 space-y-2">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2 text-white/50">
          {icon}
          <span className="text-xs font-semibold uppercase tracking-wider">{label}</span>
        </div>
        <span className={`text-[11px] font-bold px-2 py-0.5 rounded-full border ${TONE_CLASSES[tone]}`}>
          {info?.label ?? value}
        </span>
      </div>
      <p className="text-[11px] leading-relaxed text-white/45">
        {info?.description ?? '未知取值，可能来自自定义配置。'}
      </p>
      {note && (
        <p className="text-[11px] leading-relaxed text-amber-300/80 flex items-start gap-1">
          <AlertCircle size={11} className="mt-0.5 flex-shrink-0" />
          {note}
        </p>
      )}
    </div>
  )
}

// ─── main page ───────────────────────────────────────────────────────────────

export function ServiceStatus() {
  const navigate = useNavigate()
  const [modules, setModules] = useState<OverviewModule[]>([])
  const [stopping, setStopping] = useState(false)
  const [stopError, setStopError] = useState('')
  const [configYaml, setConfigYaml] = useState('')
  const [configLoading, setConfigLoading] = useState(false)
  const [configError, setConfigError] = useState('')
  const [appConfig, setAppConfig] = useState<unknown>(null)

  const loadConfig = useCallback(async () => {
    setConfigLoading(true)
    setConfigError('')
    try {
      const { content } = await getMihomoConfig()
      setConfigYaml(content)
    } catch (e) {
      setConfigError(e instanceof Error ? e.message : '读取配置失败')
    } finally {
      setConfigLoading(false)
    }
  }, [])

  useEffect(() => {
    getOverviewCore()
      .then(data => setModules(data.modules ?? []))
      .catch(() => {})
    getConfig().then(setAppConfig).catch(() => {})
    void loadConfig()
  }, [loadConfig])

  const handleStop = async () => {
    setStopping(true)
    setStopError('')
    try {
      await stopCore().catch(() => null)
      await releaseOverviewTakeover()
      navigate('/setup')
    } catch (e) {
      setStopError(e instanceof Error ? e.message : '停止失败')
    } finally {
      setStopping(false)
    }
  }

  const managed = modules.filter(m => m.managed_by_clashforge)

  // Parsed mihomo-config.yaml — ground truth for anything the network mode
  // can silently override (e.g. dns.enhanced-mode).
  const parsedYaml = useMemo(() => {
    if (!configYaml) return null
    try {
      return yaml.load(configYaml) as Record<string, unknown>
    } catch {
      return null
    }
  }, [configYaml])

  const configuredDNSMode = readStr(appConfig, 'dns', 'mode')
  const effectiveDNSMode = readStr(parsedYaml, 'dns', 'enhanced-mode') ?? configuredDNSMode
  const dnsModeOverridden = !!configuredDNSMode && !!effectiveDNSMode && configuredDNSMode !== effectiveDNSMode
  const fakeIPRange = readStr(parsedYaml, 'dns', 'fake-ip-range')

  const dnsStrategy = readStr(appConfig, 'dns', 'strategy') || 'legacy'
  const networkMode = readStr(appConfig, 'network', 'mode') || 'none'
  const dnsmasqMode = readStr(appConfig, 'dns', 'dnsmasq_mode') || 'none'
  const firewallBackend = readStr(appConfig, 'network', 'firewall_backend')
  const bypassLAN = readBool(appConfig, 'network', 'bypass_lan')
  const bypassChina = readBool(appConfig, 'network', 'bypass_china')
  const wanInterface = readStr(appConfig, 'network', 'wan_interface')

  return (
    <div className="min-h-full bg-gradient-to-b from-surface-0 to-surface-1 px-6 py-8">
      <div className="mx-auto max-w-6xl space-y-6">

        {/* Header */}
        <div className="flex items-center gap-3">
          <div className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl bg-emerald-500/20">
            <Activity size={18} className="text-emerald-400" />
          </div>
          <div>
            <p className="text-[11px] uppercase tracking-[0.24em] text-white/35">Proxy · Service</p>
            <h1 className="mt-0.5 text-base font-bold text-white">代理服务</h1>
            <p className="text-xs text-white/40">服务正在运行，停止后可重新配置</p>
          </div>
        </div>

        {/* Current effective modes */}
        <div className="glass-card px-5 py-5 space-y-4">
          <div>
            <h2 className="text-sm font-semibold text-white/90">当前生效模式</h2>
            <p className="mt-1 text-xs text-white/40">
              这些设置决定了分流的精确度和兼容性；DNS 增强模式 / Fake-IP 网段读取的是内核实际生效的配置，
              而不是你手动选的那个值——两者不一致时会特别标出。
            </p>
          </div>

          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            <ModeTile
              icon={<Route size={13} />}
              label="DNS 增强模式"
              info={effectiveDNSMode ? DNS_ENHANCED_MODE_INFO[effectiveDNSMode] : undefined}
              value={effectiveDNSMode ?? '未知'}
              note={dnsModeOverridden
                ? `你选择的是「${configuredDNSMode}」，但网络模式（${networkMode}）要求必须用 Fake-IP，已自动调整。`
                : undefined}
            />
            {effectiveDNSMode === 'fake-ip' && fakeIPRange && (
              <ModeTile
                icon={<Route size={13} />}
                label="Fake-IP 网段"
                value={fakeIPRange}
                info={{
                  label: fakeIPRange,
                  tone: fakeIPRange === '198.18.0.1/16' ? 'warn' : 'good',
                  description: fakeIPRange === '198.18.0.1/16'
                    ? '使用的是历史默认网段，与部分公共规则集（如 lancidr）收录的保留网段重叠，理论上存在误判为「直连」的风险。'
                    : '已自动避开 IANA 保留网段与当前规则集实际收录的网段，不会与「局域网/保留地址直连」类规则冲突。',
                }}
              />
            )}
            <ModeTile
              icon={<Network size={13} />}
              label="DNS 分流策略"
              info={DNS_STRATEGY_INFO[dnsStrategy]}
              value={dnsStrategy}
            />
            <ModeTile
              icon={<Wifi size={13} />}
              label="网络代理模式"
              info={NETWORK_MODE_INFO[networkMode]}
              value={networkMode}
            />
            <ModeTile
              icon={<Server size={13} />}
              label="DNS 接管方式"
              info={DNSMASQ_MODE_INFO[dnsmasqMode]}
              value={dnsmasqMode}
            />
            <ModeTile
              icon={<EyeOff size={13} />}
              label="防火墙后端"
              value={firewallBackend ?? '未知'}
              info={firewallBackend ? {
                label: firewallBackend,
                tone: 'neutral',
                description: firewallBackend === 'nftables'
                  ? '使用 nftables 管理透明代理与 DNS 接管规则——新版 OpenWrt/ImmortalWrt 的默认防火墙后端。'
                  : firewallBackend === 'iptables'
                  ? '使用 iptables（legacy）管理透明代理与 DNS 接管规则，用于不支持 nftables 的旧系统。'
                  : '由 ClashForge 自动探测选择。',
              } : undefined}
            />
          </div>

          <div className="flex flex-wrap items-center gap-2 pt-1">
            <span className={`text-[11px] font-medium px-2.5 py-1 rounded-full border ${bypassLAN ? TONE_CLASSES.good : 'text-white/40 bg-white/5 border-white/10'}`}>
              {bypassLAN ? '✓ 已绕过局域网地址' : '✕ 未绕过局域网地址'}
            </span>
            <span className={`text-[11px] font-medium px-2.5 py-1 rounded-full border ${bypassChina ? TONE_CLASSES.good : 'text-white/40 bg-white/5 border-white/10'}`}>
              {bypassChina ? '✓ 已绕过中国大陆 IP' : '✕ 未绕过中国大陆 IP'}
            </span>
            {wanInterface && (
              <span className="text-[11px] font-medium px-2.5 py-1 rounded-full border text-white/50 bg-white/5 border-white/10">
                WAN 接口：{wanInterface}
              </span>
            )}
          </div>
        </div>

        <div className="grid gap-6 xl:grid-cols-[420px_minmax(0,1fr)]">

          {/* Left: status card + stop */}
          <div className="glass-card px-5 py-5 space-y-4">
            <div className="flex items-center gap-3">
              <span className="inline-flex h-2.5 w-2.5 rounded-full bg-emerald-400 animate-pulse" />
              <p className="text-sm font-semibold text-white">内核正在运行</p>
            </div>
            <p className="text-sm text-white/50 leading-6">
              ClashForge 内核当前处于运行状态，并已接管以下系统服务。
              要重新配置，请先停止内核并退出所有接管，然后再继续。
            </p>

            {managed.length > 0 && (
              <div className="rounded-xl border border-white/[0.08] bg-black/10 px-4 py-3 space-y-2">
                <p className="text-xs text-white/30 uppercase tracking-wider font-semibold">当前已接管</p>
                {managed.map(m => (
                  <div key={m.id} className="flex items-center gap-2 text-xs">
                    <ShieldOff size={12} className="text-amber-400" />
                    <span className="text-white/80">{m.title}</span>
                    <span className="text-white/35">— {m.current_owner}</span>
                  </div>
                ))}
              </div>
            )}

            {stopError && (
              <div className="flex items-center gap-2 text-xs text-rose-400">
                <AlertCircle size={13} />{stopError}
              </div>
            )}

            <button
              className="w-full flex items-center justify-center gap-2.5 py-4 text-sm font-bold rounded-xl
                         bg-red-500/90 hover:bg-red-500 text-white border border-red-500/60 hover:border-red-500
                         shadow-lg shadow-red-500/20 transition-all active:scale-[0.98] disabled:opacity-60"
              onClick={handleStop}
              disabled={stopping}
            >
              {stopping
                ? <><Loader2 size={16} className="animate-spin" />停止中…</>
                : <><PowerOff size={16} />停止内核 + 退出所有接管</>}
            </button>
          </div>

          {/* Right: running config preview */}
          <div className="glass-card px-5 py-5 space-y-4">
            <div className="flex items-start justify-between gap-3">
              <div>
                <h2 className="text-sm font-semibold text-white/90">正在运行配置</h2>
                <p className="mt-1 text-xs text-white/40">当前完整运行配置（mihomo-config.yaml）</p>
              </div>
              <button
                className="btn-ghost text-xs flex items-center gap-1.5"
                onClick={loadConfig}
                disabled={configLoading}
              >
                <RefreshCw size={12} className={configLoading ? 'animate-spin' : ''} />
                {configLoading ? '刷新中…' : '刷新'}
              </button>
            </div>

            {configLoading && (
              <div className="flex items-center gap-2 text-sm text-white/40 py-2">
                <Loader2 size={14} className="animate-spin text-brand" />
                正在读取运行配置…
              </div>
            )}
            {!configLoading && configError && (
              <div className="rounded-xl border border-rose-500/30 bg-rose-500/10 px-3 py-2 text-xs text-rose-300">
                {configError}
              </div>
            )}
            {!configLoading && !configError && (
              <div className="rounded-xl bg-black/30 border border-white/[0.08] overflow-auto max-h-[72rem] text-xs font-mono select-text">
                {configYaml
                  ? configYaml.split('\n').map((line, i) => (
                      <div key={i} className="flex items-start gap-2 px-2 py-px leading-5">
                        <span className="select-none text-white/20 w-7 flex-shrink-0 text-right tabular-nums">{i + 1}</span>
                        <span className="flex-1 text-white/70 whitespace-pre">{line || ' '}</span>
                      </div>
                    ))
                  : <p className="px-3 py-3 text-white/30">暂无运行配置</p>
                }
              </div>
            )}
          </div>

        </div>

      </div>
    </div>
  )
}
