export interface BrowserIPProvider {
  provider: string
  group: string
  url: string
  parse: 'upaiyun' | 'ipsb' | 'ipinfo'
}

export const BROWSER_IP_PROVIDERS: BrowserIPProvider[] = [
  { provider: 'UpaiYun', group: '国内', url: 'https://pubstatic.b0.upaiyun.com/?_upnode', parse: 'upaiyun' },
  { provider: 'IP.SB', group: '国外', url: 'https://api.ip.sb/geoip', parse: 'ipsb' },
  { provider: 'IPInfo', group: '国外', url: 'https://ipinfo.io/json', parse: 'ipinfo' },
]

export const DOMAIN_PROBE_PRESETS = ['google.com', 'youtube.com', 'github.com', 'openai.com'] as const
export const DEFAULT_DOMAIN_PROBE_INPUT = DOMAIN_PROBE_PRESETS[0]

