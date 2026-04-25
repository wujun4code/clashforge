#!/usr/bin/env node
/**
 * browser-probe.mjs — 浏览器端连通性测试（在宿主机运行）
 *
 * 模拟 Dashboard.tsx 里的 runBrowserProbeData()：
 *   - IP 检查：UpaiYun / IP.SB / IPInfo（直连，不走代理）
 *   - Access 检查：百度 / 网易云 / GitHub / YouTube
 *     - 直连模式：不走代理（模拟普通用户浏览器直连）
 *     - 代理模式：通过 OpenWrt VM 的 mixed 端口（模拟用户配了网关代理）
 *
 * 用法：
 *   node browser-probe.mjs [--proxy http://user:pass@host:port] [--timeout 8000]
 *
 * 环境变量：
 *   PROXY_URL        代理地址，e.g. http://Clash:SN1pj7Wo@127.0.0.1:7890
 *   PROXY_AUTH       代理认证 user:pass（会自动构造 PROXY_URL）
 *   PROBE_TIMEOUT    超时毫秒（default: 8000）
 */

import { createRequire } from 'module'
import { promisify } from 'util'
import { exec as execCb } from 'child_process'
import * as http from 'http'
import * as https from 'https'
import * as url from 'url'

const exec = promisify(execCb)

// ── 颜色 ──────────────────────────────────────────────────────────────────────
const R = '\x1b[0;31m', G = '\x1b[0;32m', Y = '\x1b[1;33m', C = '\x1b[0;36m', B = '\x1b[1m', X = '\x1b[0m'
const pass  = (msg) => console.log(`${G}✅ PASS${X} ${msg}`)
const fail  = (msg) => { console.log(`${R}❌ FAIL${X} ${msg}`); FAILED++ }
const info  = (msg) => console.log(`${C}ℹ️   ${X} ${msg}`)
const warn  = (msg) => console.log(`${Y}⚠️  WARN${X} ${msg}`)
const section = (msg) => console.log(`\n${B}${Y}=== ${msg} ===${X}`)

let FAILED = 0

// ── GitHub Actions IP mask helper ────────────────────────────────────────────
function maskIP(ip) {
  if (ip && process.env.GITHUB_ACTIONS) {
    process.stdout.write(`::add-mask::${ip}\n`)
  }
  return ip
}
function redact(ip) {
  return process.env.GITHUB_ACTIONS && ip ? '***MASKED***' : ip
}


// ── 参数解析 ──────────────────────────────────────────────────────────────────
const args = process.argv.slice(2)
let PROXY_URL = process.env.PROXY_URL || ''
const TIMEOUT = parseInt(process.env.PROBE_TIMEOUT || '8000', 10)

for (let i = 0; i < args.length; i++) {
  if (args[i] === '--proxy' && args[i+1]) { PROXY_URL = args[++i] }
  if (args[i] === '--timeout' && args[i+1]) { /* already parsed */ }
}

// 如果有 PROXY_AUTH 但没有 PROXY_URL，自动构造
if (!PROXY_URL && process.env.PROXY_AUTH) {
  PROXY_URL = `http://${process.env.PROXY_AUTH}@127.0.0.1:7890`
}

// ── HTTP fetch 实现（node 内建，支持代理）────────────────────────────────────
function fetchVia(targetUrl, proxyUrl, timeoutMs) {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error(`timeout after ${timeoutMs}ms`)), timeoutMs)

    const parsed = new url.URL(targetUrl)
    const isHttps = parsed.protocol === 'https:'

    let reqOptions, transport

    if (proxyUrl) {
      // CONNECT tunnel for HTTPS, plain HTTP proxy for HTTP
      const proxy = new url.URL(proxyUrl)
      const proxyAuth = (proxy.username || proxy.password)
        ? `${decodeURIComponent(proxy.username)}:${decodeURIComponent(proxy.password)}`
        : null

      if (isHttps) {
        // CONNECT tunnel
        const connectOpts = {
          host: proxy.hostname,
          port: parseInt(proxy.port || '8080'),
          method: 'CONNECT',
          path: `${parsed.hostname}:${parsed.port || 443}`,
          headers: {},
        }
        if (proxyAuth) {
          connectOpts.headers['Proxy-Authorization'] = 'Basic ' + Buffer.from(proxyAuth).toString('base64')
        }

        const connectReq = http.request(connectOpts)
        connectReq.on('connect', (res, socket) => {
          clearTimeout(timer)
          if (res.statusCode !== 200) {
            socket.destroy()
            return reject(new Error(`CONNECT failed: ${res.statusCode}`))
          }
          const tlsSocket = require('tls').connect({
            host: parsed.hostname,
            socket,
            rejectUnauthorized: false,
          }, () => {
            const getReq = http.request({
              method: 'GET',
              path: parsed.pathname + (parsed.search || ''),
              headers: { Host: parsed.hostname, 'User-Agent': 'Mozilla/5.0 (ClashForge-E2E-Test)' },
              createConnection: () => tlsSocket,
            })
            getReq.on('response', (r) => {
              let body = ''
              r.on('data', d => body += d)
              r.on('end', () => resolve({ status: r.statusCode, body, headers: r.headers }))
            })
            getReq.on('error', reject)
            getReq.end()
          })
          tlsSocket.on('error', reject)
        })
        connectReq.on('error', reject)
        connectReq.setTimeout(timeoutMs, () => {
          connectReq.destroy()
          reject(new Error(`proxy connect timeout`))
        })
        connectReq.end()
        return
      } else {
        // Plain HTTP through proxy
        reqOptions = {
          host: proxy.hostname,
          port: parseInt(proxy.port || '8080'),
          method: 'GET',
          path: targetUrl,
          headers: {
            Host: parsed.hostname,
            'User-Agent': 'Mozilla/5.0 (ClashForge-E2E-Test)',
          },
        }
        if (proxyAuth) {
          reqOptions.headers['Proxy-Authorization'] = 'Basic ' + Buffer.from(proxyAuth).toString('base64')
        }
        transport = http
      }
    } else {
      // Direct connection
      reqOptions = {
        host: parsed.hostname,
        port: parseInt(parsed.port || (isHttps ? '443' : '80')),
        method: 'GET',
        path: parsed.pathname + (parsed.search || ''),
        headers: { Host: parsed.hostname, 'User-Agent': 'Mozilla/5.0 (ClashForge-E2E-Test)' },
        rejectUnauthorized: false,
      }
      transport = isHttps ? https : http
    }

    const req = transport.request(reqOptions, (res) => {
      clearTimeout(timer)
      let body = ''
      res.on('data', d => body += d)
      res.on('end', () => resolve({ status: res.statusCode, body, headers: res.headers }))
    })
    req.on('error', (e) => { clearTimeout(timer); reject(e) })
    req.setTimeout(timeoutMs, () => { req.destroy(); reject(new Error(`timeout after ${timeoutMs}ms`)) })
    req.end()
  })
}

// tls は dynamic require
import { createRequire as cr } from 'module'
const require = cr(import.meta.url)

// ── IP 检查目标 ───────────────────────────────────────────────────────────────
const IP_PROVIDERS = [
  {
    name: 'IP.SB',
    group: '国外',
    url: 'https://api.ip.sb/geoip',
    parse: (body) => {
      const d = JSON.parse(body)
      return { ip: d.ip, location: [d.city, d.region, d.country].filter(Boolean).join(' · ') }
    },
  },
  {
    name: 'IPInfo',
    group: '国外',
    url: 'https://ipinfo.io/json',
    parse: (body) => {
      const d = JSON.parse(body)
      return { ip: d.ip, location: [d.city, d.region, d.country, d.org].filter(Boolean).join(' · ') }
    },
  },
  {
    name: 'IPIFY',
    group: '国外',
    url: 'https://api.ipify.org?format=json',
    parse: (body) => {
      const d = JSON.parse(body)
      return { ip: d.ip, location: '' }
    },
  },
]

// ── Access 检查目标（与 probes.json 一致）─────────────────────────────────────
const ACCESS_TARGETS = [
  { name: '百度搜索',    url: 'https://www.baidu.com',     group: '国内' },
  { name: '网易云音乐',  url: 'https://music.163.com',     group: '国内' },
  { name: 'GitHub',      url: 'https://github.com',         group: '国际' },
  { name: 'YouTube',     url: 'https://www.youtube.com',    group: '国际' },
]

// ── 主逻辑 ─────────────────────────────────────────────────────────────────────
async function runIPChecks(label, proxyUrl) {
  section(`IP 检查 (${label})`)
  const results = []
  for (const p of IP_PROVIDERS) {
    const started = Date.now()
    try {
      const res = await fetchVia(p.url, proxyUrl, TIMEOUT)
      const latency = Date.now() - started
      if (res.status < 200 || res.status >= 300) throw new Error(`HTTP ${res.status}`)
      const { ip, location } = p.parse(res.body)
      pass(`${p.name} [${p.group}] — IP: ${ip}${location ? ' · ' + location : ''} (${latency}ms)`)
      results.push({ provider: p.name, ok: true, ip, location })
    } catch (e) {
      warn(`${p.name} [${p.group}] — 失败: ${e.message}`)
      results.push({ provider: p.name, ok: false, error: e.message })
    }
  }
  return results
}

async function runAccessChecks(label, proxyUrl) {
  section(`可访问性检查 (${label})`)
  const results = []
  for (const t of ACCESS_TARGETS) {
    const started = Date.now()
    try {
      const res = await fetchVia(t.url, proxyUrl, TIMEOUT)
      const latency = Date.now() - started
      // 2xx 或 3xx 都算成功（no-cors 模式下 redirect 也算可达）
      if (res.status >= 200 && res.status < 400) {
        pass(`${t.name} [${t.group}] — HTTP ${res.status} (${latency}ms)`)
        results.push({ name: t.name, ok: true, status: res.status, latency_ms: latency })
      } else {
        fail(`${t.name} [${t.group}] — HTTP ${res.status} (${latency}ms)`)
        results.push({ name: t.name, ok: false, status: res.status })
      }
    } catch (e) {
      const latency = Date.now() - started
      fail(`${t.name} [${t.group}] — 失败: ${e.message} (${latency}ms)`)
      results.push({ name: t.name, ok: false, error: e.message })
    }
  }
  return results
}

// ── GitHub Actions summary helper ──────────────────────────────────────────
import * as fs from 'fs'
const SUMMARY_FILE = process.env.GITHUB_STEP_SUMMARY || ''
const summaryLines = []
function summary(line) {
  summaryLines.push(line)
}
function flushSummary() {
  if (SUMMARY_FILE) {
    fs.appendFileSync(SUMMARY_FILE, summaryLines.join('\n') + '\n')
  }
}

// 测试结果记录
const tcResults = []
function recordTC(status, tc, name, op, expected, actual) {
  tcResults.push({ status, tc, name, op, expected, actual })
  const icon = status === 'PASS' ? `${G}✅ PASS${X}` : status === 'FAIL' ? `${R}❌ FAIL${X}` : `${Y}⚠️  WARN${X}`
  console.log(`${icon} [${tc}] ${name} — ${actual}`)
  if (status === 'FAIL') FAILED++
}

async function main() {
  console.log(`${B}ClashForge Browser-Side Probe Test${X}`)
  console.log(`代理: ${PROXY_URL || '无（直连模式）'}`)
  console.log(`超时: ${TIMEOUT}ms`)

  // ── 1. 直连 IP 检查（模拟浏览器直连，不走代理）
  const directIP = await runIPChecks('浏览器直连', null)
  const directIPOK = directIP.filter(r => r.ok)
  if (directIPOK.length > 0) {
    recordTC('PASS', 'BC-01', '直连 IP 检查', '不走代理，请求 IP.SB / IPInfo / IPIFY', '至少 1 个服务返回有效 IP', `${directIPOK.length}/${IP_PROVIDERS.length} 成功，IP: ${redact(directIPOK[0]?.ip)}`)
  } else {
    recordTC('FAIL', 'BC-01', '直连 IP 检查', '不走代理，请求 IP.SB / IPInfo / IPIFY', '至少 1 个服务返回有效 IP', '全部失败')
  }

  // ── 2. 直连可访问性（模拟浏览器直连）
  const directAccess = await runAccessChecks('浏览器直连', null)
  const directOK = directAccess.filter(r => r.ok).length
  const directTotal = directAccess.length
  const directDetail = directAccess.map(r => `${r.ok ? '✓' : '✗'} ${r.name} HTTP ${r.status || 'ERR'} ${r.latency_ms || '?'}ms`).join(' | ')
  if (directOK === directTotal) {
    recordTC('PASS', 'BC-02', '直连可访问性检查', '不走代理，访问百度/网易云/GitHub/YouTube', '所有站点 HTTP 2xx/3xx', `${directOK}/${directTotal} 成功: ${directDetail}`)
  } else if (directOK > 0) {
    recordTC('WARN', 'BC-02', '直连可访问性检查', '不走代理，访问百度/网易云/GitHub/YouTube', '所有站点 HTTP 2xx/3xx', `${directOK}/${directTotal} 成功: ${directDetail}`)
  } else {
    recordTC('FAIL', 'BC-02', '直连可访问性检查', '不走代理，访问百度/网易云/GitHub/YouTube', '所有站点 HTTP 2xx/3xx', `全部失败: ${directDetail}`)
  }

  // ── 3. 代理模式检查（如果提供了 PROXY_URL）
  let proxyIP = [], proxyAccess = []
  if (PROXY_URL) {
    proxyIP = await runIPChecks('通过代理', PROXY_URL)
    proxyAccess = await runAccessChecks('通过代理', PROXY_URL)

    const intlViaProxy = proxyAccess.filter(r => {
      const t = ACCESS_TARGETS.find(t => t.name === r.name)
      return t && t.group === '国际' && r.ok
    })
    const intlTargets = ACCESS_TARGETS.filter(t => t.group === '国际')
    const domTargets = ACCESS_TARGETS.filter(t => t.group === '国内')
    const domViaProxy = proxyAccess.filter(r => {
      const t = ACCESS_TARGETS.find(t => t.name === r.name)
      return t && t.group === '国内' && r.ok
    })

    section('对比分析')
    const directIPStr2 = directIPOK[0]?.ip || 'N/A'
    const proxyIPStr2 = proxyIP.find(r => r.ok)?.ip || 'N/A'
    const proxyLoc = proxyIP.find(r => r.ok)?.location || ''
    info(`直连 IP:   ${directIPStr2}`)
    info(`代理出口:  ${proxyIPStr2} ${proxyLoc}`)

    // BC-03 代理 IP 检查
    if (proxyIPStr2 !== 'N/A') {
      recordTC('PASS', 'BC-03', '代理模式 IP 检查', '通过代理请求 IP 检查服务，获取代理出口 IP', '返回有效出口 IP', `出口 IP: ${proxyIPStr2} (${proxyLoc})`)
    } else {
      recordTC('FAIL', 'BC-03', '代理模式 IP 检查', '通过代理请求 IP 检查服务', '返回有效出口 IP', '代理模式下 IP 检查全部失败')
    }

    // BC-04 出口 IP 变化 + 节点匹配验证
    // 通过 Google DoH 解析代理服务器真实 IP（绕过 fake-ip）
    let proxyServerIP = 'N/A'
    const proxyServerHost = process.env.PROXY_SERVER_HOST || ''
    const proxyNodeName = process.env.PROXY_NODE_NAME || ''
    if (proxyServerHost) {
      try {
        const dohRes = await fetchVia(
          `https://dns.google/resolve?name=${proxyServerHost}&type=A`,
          null, 8000
        )
        const dohData = JSON.parse(dohRes.body)
        proxyServerIP = dohData?.Answer?.[0]?.data || 'N/A'
        info(`DoH 解析 ${proxyServerHost} → ${proxyServerIP}`)
      } catch (e) { warn(`DoH 解析失败: ${e.message}`) }
    }

    if (directIPStr2 !== proxyIPStr2 && proxyIPStr2 !== 'N/A') {
      const ipMatchesNode = proxyServerIP !== 'N/A' && proxyIPStr2 === proxyServerIP
      const nodeNote = proxyServerIP !== 'N/A'
        ? `节点[${proxyNodeName}] ${proxyServerHost}→${proxyServerIP}`
        : '节点服务器 IP 未提供'
      if (ipMatchesNode) {
        recordTC('PASS', 'BC-04', '代理出口 IP 变化 + 节点匹配验证',
          '对比直连 IP、代理出口 IP、DoH 解析代理服务器 IP',
          '三者匹配：出口 IP 已变且等于节点服务器真实 IP',
          `${redact(directIPStr2)} → ${redact(proxyIPStr2)} = ${nodeNote} ✓`)
      } else {
        recordTC('WARN', 'BC-04', '代理出口 IP 变化 + 节点匹配验证',
          '对比直连 IP、代理出口 IP、DoH 解析代理服务器 IP',
          '三者匹配',
          `IP 已变 (${directIPStr2} → ${proxyIPStr2})，${nodeNote}`)
      }
    } else if (proxyIPStr2 === 'N/A') {
      recordTC('FAIL', 'BC-04', '代理出口 IP 变化 + 节点匹配验证',
        '对比直连 IP、代理出口 IP', 'IP 已变化，且与节点匹配', '代理 IP 不可用')
    } else {
      recordTC('WARN', 'BC-04', '代理出口 IP 变化 + 节点匹配验证',
        '对比直连 IP、代理出口 IP', 'IP 已变化，且与节点匹配',
        `IP 未变化（可能同出口）: ${proxyIPStr2}`)
    }

    // BC-05 国内站点通过代理
    const domDetail = proxyAccess.filter(r => domTargets.find(t => t.name === r.name))
      .map(r => `${r.ok ? '✓' : '✗'} ${r.name} HTTP ${r.status || 'ERR'} ${r.latency_ms || '?'}ms`).join(' | ')
    if (domViaProxy.length === domTargets.length) {
      recordTC('PASS', 'BC-05', '代理模式国内站点可访问性', '通过代理访问百度/网易云', '国内站点正常回落直连，HTTP 2xx/3xx', `${domViaProxy.length}/${domTargets.length} 成功: ${domDetail}`)
    } else {
      recordTC('WARN', 'BC-05', '代理模式国内站点可访问性', '通过代理访问百度/网易云', '国内站点正常回落直连，HTTP 2xx/3xx', `${domViaProxy.length}/${domTargets.length} 成功: ${domDetail}`)
    }

    // BC-06 国际站点通过代理
    const intlDetail = proxyAccess.filter(r => intlTargets.find(t => t.name === r.name))
      .map(r => `${r.ok ? '✓' : '✗'} ${r.name} HTTP ${r.status || 'ERR'} ${r.latency_ms || '?'}ms`).join(' | ')
    if (intlViaProxy.length === intlTargets.length) {
      recordTC('PASS', 'BC-06', '代理模式国际站点可访问性', '通过代理访问 GitHub / YouTube', '国际站点通过代理节点可达，HTTP 2xx/3xx', `${intlViaProxy.length}/${intlTargets.length} 成功: ${intlDetail}`)
    } else if (intlViaProxy.length > 0) {
      recordTC('WARN', 'BC-06', '代理模式国际站点可访问性', '通过代理访问 GitHub / YouTube', '国际站点通过代理节点可达，HTTP 2xx/3xx', `${intlViaProxy.length}/${intlTargets.length} 成功: ${intlDetail}`)
    } else {
      recordTC('FAIL', 'BC-06', '代理模式国际站点可访问性', '通过代理访问 GitHub / YouTube', '国际站点通过代理节点可达，HTTP 2xx/3xx', `全部失败: ${intlDetail}`)
    }
  }

  // ── 输出 GitHub Actions Job Summary
  const passCount = tcResults.filter(r => r.status === 'PASS').length
  const failCount = tcResults.filter(r => r.status === 'FAIL').length
  const warnCount = tcResults.filter(r => r.status === 'WARN').length
  const directIPStr = directIP.find(r => r.ok)?.ip || 'N/A'
  const proxyIPStr = proxyIP?.find(r => r.ok)?.ip || 'N/A'

  summary('## 🔍 浏览器端探测测试报告')
  summary('')
  summary('| 项目 | 值 |')
  summary('|------|----|')
  summary(`| **直连 IP** | ${directIPStr} |`)
  summary(`| **代理出口 IP** | ${proxyIPStr} |`)
  summary(`| **代理地址** | ${PROXY_URL || '无'} |`)
  summary('')
  if (failCount === 0) {
    summary('### ✅ 浏览器端探测全部通过')
  } else {
    summary(`### ❌ 存在失败用例 (${failCount} 个)`)
  }
  summary('')
  summary('| 结果 | 数量 |')
  summary('|------|------|')
  summary(`| ✅ 通过 | ${passCount} |`)
  summary(`| ❌ 失败 | ${failCount} |`)
  summary(`| ⚠️ 警告 | ${warnCount} |`)
  summary(`| **合计** | **${tcResults.length}** |`)
  summary('')
  summary('---')
  summary('')
  summary('### 详细用例结果')
  summary('')
  summary('| 编号 | 用例名称 | 操作 | 预期结果 | 实际结果 | 状态 |')
  summary('|------|----------|------|----------|----------|------|')
  for (const r of tcResults) {
    const icon = r.status === 'PASS' ? '✅' : r.status === 'FAIL' ? '❌' : '⚠️'
    summary(`| ${r.tc} | ${r.name} | ${r.op} | ${r.expected} | ${r.actual} | ${icon} ${r.status} |`)
  }
  flushSummary()

  // 控制台总结
  section('测试结果')
  const totalChecks = tcResults.length
  console.log(`\n${B}总计:${X} ${totalChecks} 项检查，${FAILED} 个失败\n`)

  if (FAILED === 0) {
    console.log(`${G}${B}ALL BROWSER PROBE TESTS PASSED ✅${X}`)
    process.exit(0)
  } else {
    console.log(`${R}${B}${FAILED} 个测试失败 ❌${X}`)
    process.exit(1)
  }
}

main().catch(e => { console.error(e); process.exit(1) })
