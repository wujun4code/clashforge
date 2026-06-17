import { useState } from 'react'
import {
  Monitor, Server, Globe, Shield, Cpu, Network, Wifi,
} from 'lucide-react'

// ─── animated flow diagram ──────────────────────────────────────────────────

const FLOW_CSS = `
@keyframes cfFlowR {
  0%   { left: -8px;          opacity: 0; }
  8%   { opacity: 1; }
  92%  { opacity: 1; }
  100% { left: calc(100% + 8px); opacity: 0; }
}
@keyframes cfFlowL {
  0%   { left: calc(100% + 8px); opacity: 0; }
  8%   { opacity: 1; }
  92%  { opacity: 1; }
  100% { left: -8px;          opacity: 0; }
}
`

type Dir = 'r' | 'l'

function Dot({ dir, color, delay, dur }: { dir: Dir; color: string; delay: string; dur: string }) {
  return (
    <span
      style={{
        position: 'absolute',
        top: '50%',
        transform: 'translateY(-50%)',
        width: 7,
        height: 7,
        borderRadius: '50%',
        background: color,
        boxShadow: `0 0 8px ${color}`,
        animation: `${dir === 'r' ? 'cfFlowR' : 'cfFlowL'} ${dur} ${delay} infinite linear`,
      }}
    />
  )
}

function Arrow({
  label, color, dir = 'r', dur = '1.6s', flex = 1,
}: {
  label?: string; color: string; dir?: Dir; dur?: string; flex?: number
}) {
  const d = parseFloat(dur)
  const delays = ['0s', `${(d / 3).toFixed(2)}s`, `${(2 * d / 3).toFixed(2)}s`]
  return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', flex, minWidth: 48, padding: '0 2px' }}>
      {label && (
        <p style={{ fontSize: 9, color: 'rgba(255,255,255,0.35)', marginBottom: 5, textAlign: 'center', whiteSpace: 'pre', lineHeight: 1.4 }}>
          {label}
        </p>
      )}
      <div style={{ position: 'relative', width: '100%', height: 12 }}>
        {/* track */}
        <span style={{ position: 'absolute', inset: '50% 0 auto', height: 1, background: `${color}28` }} />
        {/* arrowhead */}
        {dir === 'r' ? (
          <span style={{ position: 'absolute', right: 0, top: '50%', transform: 'translateY(-50%)',
            width: 0, height: 0,
            borderTop: '4px solid transparent', borderBottom: '4px solid transparent',
            borderLeft: `6px solid ${color}50` }} />
        ) : (
          <span style={{ position: 'absolute', left: 0, top: '50%', transform: 'translateY(-50%)',
            width: 0, height: 0,
            borderTop: '4px solid transparent', borderBottom: '4px solid transparent',
            borderRight: `6px solid ${color}50` }} />
        )}
        {delays.map((delay, i) => <Dot key={i} dir={dir} color={color} delay={delay} dur={dur} />)}
      </div>
    </div>
  )
}

function Node({
  icon, label, sub, bg, glow,
}: {
  icon: React.ReactNode; label: string; sub?: string; bg: string; glow: string
}) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 6, minWidth: 68, maxWidth: 90 }}>
      <div style={{
        width: 44, height: 44, borderRadius: 12, background: bg,
        boxShadow: `0 0 16px ${glow}`, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0,
      }}>
        {icon}
      </div>
      <div style={{ textAlign: 'center' }}>
        <p style={{ fontSize: 10, fontWeight: 600, color: 'rgba(255,255,255,0.85)', lineHeight: 1.3, whiteSpace: 'pre' }}>{label}</p>
        {sub && <p style={{ fontSize: 9, color: 'rgba(255,255,255,0.35)', marginTop: 2 }}>{sub}</p>}
      </div>
    </div>
  )
}

function PhaseRow({ n, label }: { n: number; label: string }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 10 }}>
      <span style={{
        width: 20, height: 20, borderRadius: '50%', background: 'rgba(255,255,255,0.08)',
        fontSize: 10, fontWeight: 700, color: 'rgba(255,255,255,0.5)',
        display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0,
      }}>{n}</span>
      <span style={{ fontSize: 11, fontWeight: 600, color: 'rgba(255,255,255,0.4)', textTransform: 'uppercase', letterSpacing: '0.1em' }}>
        {label}
      </span>
      <span style={{ flex: 1, height: 1, background: 'rgba(255,255,255,0.06)' }} />
    </div>
  )
}

function SplitOutput() {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8, flex: 1, minWidth: 150 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
        <span style={{ fontSize: 9, fontWeight: 600, color: '#34d399', background: 'rgba(52,211,153,0.12)', borderRadius: 4, padding: '2px 6px', flexShrink: 0 }}>CN 直连</span>
        <span style={{ flex: 1, height: 1, background: 'rgba(52,211,153,0.2)', position: 'relative' }}>
          <span style={{ position: 'absolute', right: -4, top: '50%', transform: 'translateY(-50%)', fontSize: 8, color: 'rgba(52,211,153,0.5)' }}>▶</span>
        </span>
        <span style={{ fontSize: 9, color: 'rgba(255,255,255,0.55)', background: 'rgba(255,255,255,0.06)', borderRadius: 4, padding: '2px 6px', flexShrink: 0 }}>国内服务器</span>
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
        <span style={{ fontSize: 9, fontWeight: 600, color: '#a78bfa', background: 'rgba(167,139,250,0.12)', borderRadius: 4, padding: '2px 6px', flexShrink: 0 }}>非CN 代理</span>
        <span style={{ flex: 1, height: 1, background: 'rgba(167,139,250,0.2)', position: 'relative' }}>
          <span style={{ position: 'absolute', right: -4, top: '50%', transform: 'translateY(-50%)', fontSize: 8, color: 'rgba(167,139,250,0.5)' }}>▶</span>
        </span>
        <span style={{ fontSize: 9, color: 'rgba(255,255,255,0.55)', background: 'rgba(255,255,255,0.06)', borderRadius: 4, padding: '2px 6px', flexShrink: 0 }}>代理出口</span>
      </div>
    </div>
  )
}

function DNSPhase() {
  return (
    <div style={{ marginBottom: 20 }}>
      <PhaseRow n={1} label="DNS 解析阶段 — 两种模式完全相同" />
      <div style={{ display: 'flex', alignItems: 'center', gap: 0 }}>
        <Node
          icon={<Monitor size={17} color="white" />}
          label="你的设备" sub="发起 DNS 查询"
          bg="rgba(59,130,246,0.55)" glow="rgba(59,130,246,0.3)"
        />
        <Arrow label="DNS 查询" color="#818cf8" />
        <Node
          icon={<Network size={17} color="white" />}
          label="dnsmasq" sub=":53 DNS入口"
          bg="rgba(99,102,241,0.5)" glow="rgba(99,102,241,0.25)"
        />
        <Arrow label="upstream\n转发" color="#a78bfa" />
        <Node
          icon={<Server size={17} color="white" />}
          label={'Mihomo DNS'} sub=":17874  fake-ip"
          bg="rgba(124,58,237,0.6)" glow="rgba(124,58,237,0.3)"
        />
        <Arrow label={'返回\nFake IP'} color="#f59e0b" dir="l" dur="1.8s" />
        <Node
          icon={<Wifi size={17} color="white" />}
          label={'设备得到\n198.18.x.x'} sub="假 IP 地址"
          bg="rgba(217,119,6,0.5)" glow="rgba(245,158,11,0.25)"
        />
      </div>
      <p style={{ marginTop: 10, fontSize: 10, color: 'rgba(255,255,255,0.3)', lineHeight: 1.7, paddingLeft: 4 }}>
        Fake-IP 的核心：Mihomo 立刻返回一个虚构 IP（198.18.x.x），设备拿到这个假 IP 后马上发起 TCP 连接，
        Mihomo 凭借这个假 IP 映射回真实域名，再决定走直连还是代理——<strong style={{ color: 'rgba(255,255,255,0.5)' }}>域名不会被提前泄漏给 DNS</strong>。
      </p>
    </div>
  )
}

function TProxyDataPhase() {
  return (
    <div>
      <PhaseRow n={2} label="流量拦截阶段 — TProxy（透明代理）模式" />
      <div style={{ display: 'flex', alignItems: 'center', gap: 0 }}>
        <Node
          icon={<Monitor size={17} color="white" />}
          label="你的设备" sub={'TCP 连接\n198.18.x.x'}
          bg="rgba(59,130,246,0.55)" glow="rgba(59,130,246,0.3)"
        />
        <Arrow label={'TCP 包\n发出'} color="#60a5fa" />
        <Node
          icon={<Shield size={17} color="white" />}
          label={'nftables\nTProxy'} sub="内核层拦截"
          bg="rgba(239,68,68,0.55)" glow="rgba(239,68,68,0.3)"
        />
        <Arrow label={'识别真实\n域名'} color="#f87171" />
        <Node
          icon={<Cpu size={17} color="white" />}
          label={'Mihomo\n规则引擎'} sub="匹配 GeoSite/IP"
          bg="rgba(16,185,129,0.6)" glow="rgba(16,185,129,0.3)"
        />
        <Arrow label="路由决策" color="#34d399" flex={0.6} />
        <SplitOutput />
      </div>
      <div style={{
        marginTop: 14, padding: '10px 12px', borderRadius: 10,
        background: 'rgba(239,68,68,0.07)', border: '1px solid rgba(239,68,68,0.15)',
      }}>
        <p style={{ fontSize: 10, fontWeight: 700, color: 'rgba(239,100,100,0.8)', marginBottom: 6, letterSpacing: '0.08em', textTransform: 'uppercase' }}>
          为什么 TProxy 模式需要接管 5 项服务
        </p>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '4px 16px' }}>
          {[
            ['代理核心', 'Mihomo 进程本身，提供代理隧道'],
            ['透明代理规则', 'nftables TPROXY target，把所有 TCP/UDP 重定向到 Mihomo :7895'],
            ['防火墙规则', 'nftables 标记流量、放行直连、拦截代理，需深度修改内核防火墙'],
            ['DNS 入口', 'dnsmasq upstream → Mihomo DNS :17874'],
            ['DNS 解析引擎', 'Mihomo 内置 fake-ip 解析器'],
          ].map(([t, d]) => (
            <div key={t} style={{ display: 'flex', gap: 6, alignItems: 'flex-start' }}>
              <span style={{ width: 14, height: 14, borderRadius: '50%', background: 'rgba(239,68,68,0.25)', flexShrink: 0, marginTop: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                <span style={{ width: 5, height: 5, borderRadius: '50%', background: '#f87171' }} />
              </span>
              <div>
                <p style={{ fontSize: 10, fontWeight: 600, color: 'rgba(255,255,255,0.7)' }}>{t}</p>
                <p style={{ fontSize: 9, color: 'rgba(255,255,255,0.35)', lineHeight: 1.5 }}>{d}</p>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

function TUNDataPhase() {
  return (
    <div>
      <PhaseRow n={2} label="流量拦截阶段 — TUN（虚拟网卡）模式" />
      <div style={{ display: 'flex', alignItems: 'center', gap: 0 }}>
        <Node
          icon={<Monitor size={17} color="white" />}
          label="你的设备" sub={'TCP 连接\n198.18.x.x'}
          bg="rgba(59,130,246,0.55)" glow="rgba(59,130,246,0.3)"
        />
        <Arrow label={'TCP 包\n发出'} color="#60a5fa" />
        <Node
          icon={<Network size={17} color="white" />}
          label={'系统路由表'} sub="auto-route"
          bg="rgba(245,158,11,0.45)" glow="rgba(245,158,11,0.2)"
        />
        <Arrow label={'所有 IP\n流量导入'} color="#fbbf24" />
        <Node
          icon={<Globe size={17} color="white" />}
          label={'TUN 网卡'} sub="虚拟NIC (Meta)"
          bg="rgba(249,115,22,0.6)" glow="rgba(249,115,22,0.3)"
        />
        <Arrow label={'用户空间\n解包'} color="#fb923c" />
        <Node
          icon={<Cpu size={17} color="white" />}
          label={'Mihomo\n规则引擎'} sub="匹配 GeoSite/IP"
          bg="rgba(16,185,129,0.6)" glow="rgba(16,185,129,0.3)"
        />
        <Arrow label="路由决策" color="#34d399" flex={0.6} />
        <SplitOutput />
      </div>
      <div style={{
        marginTop: 14, padding: '10px 12px', borderRadius: 10,
        background: 'rgba(249,115,22,0.07)', border: '1px solid rgba(249,115,22,0.15)',
      }}>
        <p style={{ fontSize: 10, fontWeight: 700, color: 'rgba(251,146,60,0.85)', marginBottom: 6, letterSpacing: '0.08em', textTransform: 'uppercase' }}>
          为什么 TUN 模式只需接管 3 项服务
        </p>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '4px 16px' }}>
          {[
            ['代理核心 ✓', 'Mihomo 进程，与 TProxy 相同'],
            ['DNS 入口 ✓', 'dnsmasq upstream → Mihomo DNS :17874，与 TProxy 相同'],
            ['DNS 解析引擎 ✓', 'Mihomo fake-ip 解析器，与 TProxy 相同'],
            ['透明代理规则 ✗ 不需要', 'TUN 网卡自动捕获 IP 层所有流量，无需 TPROXY 内核目标'],
            ['防火墙规则 ✗ 不需要', '路由表把流量导入 TUN，不需要修改 nftables/iptables 规则'],
          ].map(([t, d]) => (
            <div key={t} style={{ display: 'flex', gap: 6, alignItems: 'flex-start' }}>
              <span style={{
                width: 14, height: 14, borderRadius: '50%', flexShrink: 0, marginTop: 1,
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                background: t.includes('✗') ? 'rgba(255,255,255,0.06)' : 'rgba(249,115,22,0.2)',
              }}>
                <span style={{ width: 5, height: 5, borderRadius: '50%', background: t.includes('✗') ? 'rgba(255,255,255,0.15)' : '#fb923c' }} />
              </span>
              <div>
                <p style={{ fontSize: 10, fontWeight: 600, color: t.includes('✗') ? 'rgba(255,255,255,0.3)' : 'rgba(255,255,255,0.7)' }}>{t}</p>
                <p style={{ fontSize: 9, color: 'rgba(255,255,255,0.35)', lineHeight: 1.5 }}>{d}</p>
              </div>
            </div>
          ))}
        </div>
        <p style={{ marginTop: 8, fontSize: 9, color: 'rgba(255,255,255,0.3)', lineHeight: 1.6 }}>
          TUN 工作在 <strong style={{ color: 'rgba(255,255,255,0.45)' }}>IP 层（第三层）</strong>，虚拟网卡天然能捕获包括 ICMP 在内的所有协议；
          TProxy 工作在 <strong style={{ color: 'rgba(255,255,255,0.45)' }}>传输层（第四层）</strong>，只能拦截 TCP/UDP，且必须借助 nftables 内核规则。
        </p>
      </div>
    </div>
  )
}

function RedirHostPhase() {
  return (
    <div style={{ marginBottom: 20 }}>
      <PhaseRow n={1} label="DNS 解析阶段 — redir-host 模式" />
      <div style={{ display: 'flex', alignItems: 'center', gap: 0 }}>
        <Node
          icon={<Monitor size={17} color="white" />}
          label="你的设备" sub="发起 DNS 查询"
          bg="rgba(59,130,246,0.55)" glow="rgba(59,130,246,0.3)"
        />
        <Arrow label="DNS 查询" color="#818cf8" />
        <Node
          icon={<Network size={17} color="white" />}
          label="dnsmasq" sub=":53 DNS入口"
          bg="rgba(99,102,241,0.5)" glow="rgba(99,102,241,0.25)"
        />
        <Arrow label={'upstream\n转发'} color="#22d3ee" />
        <Node
          icon={<Server size={17} color="white" />}
          label={'Mihomo DNS'} sub=":17874  redir-host"
          bg="rgba(8,145,178,0.6)" glow="rgba(34,211,238,0.3)"
        />
        <Arrow label={'返回\n真实 IP'} color="#22d3ee" dir="l" dur="1.8s" />
        <Node
          icon={<Wifi size={17} color="white" />}
          label={'设备得到\n真实 IP'} sub="如 20.205.x.x"
          bg="rgba(14,116,144,0.5)" glow="rgba(34,211,238,0.25)"
        />
      </div>
      <p style={{ marginTop: 10, fontSize: 10, color: 'rgba(255,255,255,0.3)', lineHeight: 1.7, paddingLeft: 4 }}>
        Redir-host 的核心：Mihomo 把查询原样转发给上游，返回域名<strong style={{ color: 'rgba(255,255,255,0.5)' }}>真实的公网 IP</strong>。
        设备拿到的就是这个真实 IP 并直接发起连接——此刻 Mihomo 还<strong style={{ color: 'rgba(255,255,255,0.5)' }}>不知道这个 IP 对应哪个域名</strong>。
      </p>
    </div>
  )
}

function FakeIPMatchPhase() {
  return (
    <div>
      <PhaseRow n={2} label="域名识别阶段 — 反查映射表，无需嗅探" />
      <div style={{ display: 'flex', alignItems: 'center', gap: 0 }}>
        <Node
          icon={<Monitor size={17} color="white" />}
          label="你的设备" sub={'TCP 连接\n198.18.x.x'}
          bg="rgba(59,130,246,0.55)" glow="rgba(59,130,246,0.3)"
        />
        <Arrow label={'TCP 包\n发出'} color="#60a5fa" />
        <Node
          icon={<Shield size={17} color="white" />}
          label={'TProxy / TUN'} sub="按 IP+端口拦截"
          bg="rgba(124,58,237,0.55)" glow="rgba(124,58,237,0.3)"
        />
        <Arrow label={'目的 IP\n198.18.x.x'} color="#a78bfa" />
        <Node
          icon={<Cpu size={17} color="white" />}
          label={'Mihomo\n反查映射表'} sub="fake-ip → 域名"
          bg="rgba(124,58,237,0.6)" glow="rgba(167,139,250,0.3)"
        />
        <Arrow label="规则匹配" color="#34d399" flex={0.6} />
        <SplitOutput />
      </div>
      <div style={{
        marginTop: 14, padding: '10px 12px', borderRadius: 10,
        background: 'rgba(124,58,237,0.07)', border: '1px solid rgba(124,58,237,0.15)',
      }}>
        <p style={{ fontSize: 10, fontWeight: 700, color: 'rgba(167,139,250,0.85)', marginBottom: 6, letterSpacing: '0.08em', textTransform: 'uppercase' }}>
          Fake-IP 的取舍
        </p>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '4px 16px' }}>
          {[
            ['优点：域名 100% 精确', 'IP 是 Mihomo 自己生成的，反查映射表必然命中，不依赖嗅探，规则匹配最准确'],
            ['缺点：需要例外清单', '某些域名必须拿到真实 IP（按 IP 选边缘节点的 CDN、局域网服务等），需加入 fake_ip_filter，否则会异常'],
            ['ClashForge 已自动处理', '自动把 geosite:cn 加入过滤列表，保证国内域名总是拿到真实 IP'],
            ['典型场景', '希望按域名精确分流（默认的 split / privacy 策略）的场景'],
          ].map(([t, d]) => (
            <div key={t} style={{ display: 'flex', gap: 6, alignItems: 'flex-start' }}>
              <span style={{ width: 14, height: 14, borderRadius: '50%', background: 'rgba(124,58,237,0.25)', flexShrink: 0, marginTop: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                <span style={{ width: 5, height: 5, borderRadius: '50%', background: '#a78bfa' }} />
              </span>
              <div>
                <p style={{ fontSize: 10, fontWeight: 600, color: 'rgba(255,255,255,0.7)' }}>{t}</p>
                <p style={{ fontSize: 9, color: 'rgba(255,255,255,0.35)', lineHeight: 1.5 }}>{d}</p>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

function RedirHostMatchPhase() {
  return (
    <div>
      <PhaseRow n={2} label="域名识别阶段 — 需要实时嗅探" />
      <div style={{ display: 'flex', alignItems: 'center', gap: 0 }}>
        <Node
          icon={<Monitor size={17} color="white" />}
          label="你的设备" sub={'TCP 连接\n真实 IP'}
          bg="rgba(59,130,246,0.55)" glow="rgba(59,130,246,0.3)"
        />
        <Arrow label={'TCP/TLS\n握手'} color="#60a5fa" />
        <Node
          icon={<Shield size={17} color="white" />}
          label={'TProxy / TUN'} sub="按 IP+端口拦截"
          bg="rgba(8,145,178,0.55)" glow="rgba(34,211,238,0.3)"
        />
        <Arrow label={'读取首包\nSNI/Host'} color="#22d3ee" />
        <Node
          icon={<Cpu size={17} color="white" />}
          label={'Mihomo\n嗅探域名'} sub="TLS SNI / HTTP Host"
          bg="rgba(13,148,136,0.6)" glow="rgba(20,184,166,0.3)"
        />
        <Arrow label="规则匹配" color="#34d399" flex={0.6} />
        <SplitOutput />
      </div>
      <div style={{
        marginTop: 14, padding: '10px 12px', borderRadius: 10,
        background: 'rgba(34,211,238,0.07)', border: '1px solid rgba(34,211,238,0.15)',
      }}>
        <p style={{ fontSize: 10, fontWeight: 700, color: 'rgba(34,211,238,0.85)', marginBottom: 6, letterSpacing: '0.08em', textTransform: 'uppercase' }}>
          Redir-Host 的取舍
        </p>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '4px 16px' }}>
          {[
            ['优点：IP 始终真实', '即使嗅探失败或规则未命中，连接仍能直连成功，不会因为假 IP 而中断'],
            ['缺点：依赖嗅探', 'TLS SNI / HTTP Host 嗅探失败时（非标准协议、加密 SNI 等）规则可能退化为按 GeoIP 匹配，不够精确'],
            ['无需维护例外清单', '不存在 fake_ip_filter 这类"哪些域名不能走假 IP"的清单需要维护'],
            ['典型场景', '希望未代理流量 100% 稳定直连、对嗅探精度要求不高的场景'],
          ].map(([t, d]) => (
            <div key={t} style={{ display: 'flex', gap: 6, alignItems: 'flex-start' }}>
              <span style={{ width: 14, height: 14, borderRadius: '50%', background: 'rgba(34,211,238,0.25)', flexShrink: 0, marginTop: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                <span style={{ width: 5, height: 5, borderRadius: '50%', background: '#22d3ee' }} />
              </span>
              <div>
                <p style={{ fontSize: 10, fontWeight: 600, color: 'rgba(255,255,255,0.7)' }}>{t}</p>
                <p style={{ fontSize: 9, color: 'rgba(255,255,255,0.35)', lineHeight: 1.5 }}>{d}</p>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

type DNSDiagramTab = 'fake-ip' | 'redir-host'

export function DNSModeDiagram({ activeMode, running = false }: { activeMode: string; running?: boolean }) {
  const [tab, setTab] = useState<DNSDiagramTab>(activeMode === 'redir-host' ? 'redir-host' : 'fake-ip')

  const tabs: { id: DNSDiagramTab; label: string }[] = [
    { id: 'fake-ip',    label: 'Fake-IP（假 IP）' },
    { id: 'redir-host', label: 'Redir-Host（真实 IP）' },
  ]

  return (
    <div className="glass-card" style={{ padding: '20px 24px' }}>
      <style>{FLOW_CSS}</style>

      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: 16, gap: 12 }}>
        <div>
          <h2 style={{ fontSize: 13, fontWeight: 700, color: 'rgba(255,255,255,0.9)', margin: 0 }}>DNS 解析模式</h2>
          <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.35)', marginTop: 3 }}>Mihomo 如何把连接的目的 IP 映射回域名以匹配规则 — 点击 Tab 切换模式</p>
        </div>
        {running && activeMode && (
          <span style={{
            fontSize: 10, fontWeight: 700, padding: '4px 10px', borderRadius: 20, flexShrink: 0,
            background: activeMode === 'redir-host' ? 'rgba(34,211,238,0.2)' : 'rgba(124,58,237,0.2)',
            color: activeMode === 'redir-host' ? '#22d3ee' : '#a78bfa',
            border: `1px solid ${activeMode === 'redir-host' ? 'rgba(34,211,238,0.3)' : 'rgba(124,58,237,0.3)'}`,
          }}>
            当前运行：{activeMode === 'redir-host' ? 'Redir-Host 模式' : activeMode === 'fake-ip' ? 'Fake-IP 模式' : activeMode}
          </span>
        )}
      </div>

      {/* Tabs */}
      <div style={{ display: 'flex', gap: 4, background: 'rgba(0,0,0,0.2)', padding: 4, borderRadius: 10, marginBottom: 20 }}>
        {tabs.map(t => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            style={{
              flex: 1, padding: '7px 12px', borderRadius: 8, border: 'none', cursor: 'pointer',
              fontSize: 11, fontWeight: 600, transition: 'all 0.15s',
              background: tab === t.id ? (t.id === 'redir-host' ? 'rgba(34,211,238,0.22)' : 'rgba(124,58,237,0.25)') : 'transparent',
              color: tab === t.id ? (t.id === 'redir-host' ? '#22d3ee' : '#a78bfa') : 'rgba(255,255,255,0.35)',
              display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6,
            }}
          >
            {running && activeMode === t.id && (
              <span style={{ width: 6, height: 6, borderRadius: '50%', background: 'currentColor', animation: 'pulse 2s infinite' }} />
            )}
            {t.label}
          </button>
        ))}
      </div>

      {/* Diagram */}
      <div style={{ overflowX: 'auto', paddingBottom: 4 }}>
        <div style={{ minWidth: 600 }}>
          {tab === 'fake-ip' ? <DNSPhase /> : <RedirHostPhase />}
          <div style={{ height: 1, background: 'rgba(255,255,255,0.05)', margin: '20px 0' }} />
          {tab === 'fake-ip' ? <FakeIPMatchPhase /> : <RedirHostMatchPhase />}
        </div>
      </div>

      {/* Bottom comparison */}
      <div style={{
        marginTop: 20, padding: '12px 14px', borderRadius: 10,
        background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.06)',
        display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12,
      }}>
        <div>
          <p style={{ fontSize: 10, fontWeight: 700, color: 'rgba(167,139,250,0.8)', marginBottom: 6, textTransform: 'uppercase', letterSpacing: '0.08em' }}>
            Fake-IP 模式适合
          </p>
          {['希望按域名精确分流（默认 split / privacy 策略）', '不想为了规则精度而依赖嗅探兜底', '能接受维护 fake_ip_filter 例外列表'].map(s => (
            <p key={s} style={{ fontSize: 10, color: 'rgba(255,255,255,0.4)', lineHeight: 1.6 }}>· {s}</p>
          ))}
        </div>
        <div>
          <p style={{ fontSize: 10, fontWeight: 700, color: 'rgba(34,211,238,0.8)', marginBottom: 6, textTransform: 'uppercase', letterSpacing: '0.08em' }}>
            Redir-Host 模式适合
          </p>
          {['希望未代理流量始终是真实 IP，零例外', '不想维护 fake_ip_filter 这类例外清单', '可以接受嗅探失败时规则退化为 GeoIP 匹配'].map(s => (
            <p key={s} style={{ fontSize: 10, color: 'rgba(255,255,255,0.4)', lineHeight: 1.6 }}>· {s}</p>
          ))}
        </div>
      </div>
    </div>
  )
}

type DiagramTab = 'tproxy' | 'tun'

export function WorkflowDiagram({ activeMode, running = false }: { activeMode: string; running?: boolean }) {
  const [tab, setTab] = useState<DiagramTab>(activeMode === 'tun' ? 'tun' : 'tproxy')

  const tabs: { id: DiagramTab; label: string }[] = [
    { id: 'tproxy', label: 'TProxy · Fake-IP（透明代理）' },
    { id: 'tun',    label: 'TUN · 虚拟网卡' },
  ]

  return (
    <div className="glass-card" style={{ padding: '20px 24px' }}>
      <style>{FLOW_CSS}</style>

      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: 16, gap: 12 }}>
        <div>
          <h2 style={{ fontSize: 13, fontWeight: 700, color: 'rgba(255,255,255,0.9)', margin: 0 }}>工作原理</h2>
          <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.35)', marginTop: 3 }}>数据包如何在 ClashForge 中流转 — 点击 Tab 切换模式</p>
        </div>
        {running && activeMode && (
          <span style={{
            fontSize: 10, fontWeight: 700, padding: '4px 10px', borderRadius: 20, flexShrink: 0,
            background: activeMode === 'tun' ? 'rgba(249,115,22,0.2)' : 'rgba(239,68,68,0.2)',
            color: activeMode === 'tun' ? '#fb923c' : '#f87171',
            border: `1px solid ${activeMode === 'tun' ? 'rgba(249,115,22,0.3)' : 'rgba(239,68,68,0.3)'}`,
          }}>
            当前运行：{activeMode === 'tun' ? 'TUN 模式' : activeMode === 'tproxy' ? 'TProxy 模式' : activeMode}
          </span>
        )}
      </div>

      {/* Tabs */}
      <div style={{ display: 'flex', gap: 4, background: 'rgba(0,0,0,0.2)', padding: 4, borderRadius: 10, marginBottom: 20 }}>
        {tabs.map(t => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            style={{
              flex: 1, padding: '7px 12px', borderRadius: 8, border: 'none', cursor: 'pointer',
              fontSize: 11, fontWeight: 600, transition: 'all 0.15s',
              background: tab === t.id ? (t.id === 'tun' ? 'rgba(249,115,22,0.25)' : 'rgba(239,68,68,0.22)') : 'transparent',
              color: tab === t.id ? (t.id === 'tun' ? '#fb923c' : '#f87171') : 'rgba(255,255,255,0.35)',
              display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6,
            }}
          >
            {running && activeMode === t.id && (
              <span style={{ width: 6, height: 6, borderRadius: '50%', background: 'currentColor', animation: 'pulse 2s infinite' }} />
            )}
            {t.label}
          </button>
        ))}
      </div>

      {/* Diagram */}
      <div style={{ overflowX: 'auto', paddingBottom: 4 }}>
        <div style={{ minWidth: 600 }}>
          <DNSPhase />
          <div style={{ height: 1, background: 'rgba(255,255,255,0.05)', margin: '20px 0' }} />
          {tab === 'tproxy' ? <TProxyDataPhase /> : <TUNDataPhase />}
        </div>
      </div>

      {/* Bottom comparison */}
      <div style={{
        marginTop: 20, padding: '12px 14px', borderRadius: 10,
        background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.06)',
        display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12,
      }}>
        <div>
          <p style={{ fontSize: 10, fontWeight: 700, color: 'rgba(248,113,113,0.8)', marginBottom: 6, textTransform: 'uppercase', letterSpacing: '0.08em' }}>
            TProxy 模式适合
          </p>
          {['路由器内核支持 TPROXY（OpenWrt 默认支持）', '需要精细控制哪些流量走代理', '对 nftables 规则有定制需求'].map(s => (
            <p key={s} style={{ fontSize: 10, color: 'rgba(255,255,255,0.4)', lineHeight: 1.6 }}>· {s}</p>
          ))}
        </div>
        <div>
          <p style={{ fontSize: 10, fontWeight: 700, color: 'rgba(251,146,60,0.8)', marginBottom: 6, textTransform: 'uppercase', letterSpacing: '0.08em' }}>
            TUN 模式适合
          </p>
          {['不想碰 nftables/iptables，部署更简单', '需要代理 ICMP、原始套接字等非 TCP/UDP 协议', '虚拟机、容器等不支持 TPROXY 的环境'].map(s => (
            <p key={s} style={{ fontSize: 10, color: 'rgba(255,255,255,0.4)', lineHeight: 1.6 }}>· {s}</p>
          ))}
        </div>
      </div>
    </div>
  )
}
