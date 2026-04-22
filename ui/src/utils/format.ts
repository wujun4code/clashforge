export function formatBytes(bytes: number, suffix = '/s'): string {
  if (bytes < 1024) return `${bytes} B${suffix}`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB${suffix}`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(2)} MB${suffix}`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB${suffix}`
}

export function formatUptime(secs: number): string {
  if (secs < 60) return `${secs}s`
  if (secs < 3600) return `${Math.floor(secs / 60)}m ${secs % 60}s`
  const h = Math.floor(secs / 3600)
  const m = Math.floor((secs % 3600) / 60)
  return `${h}h ${m}m`
}

export function latencyColor(ms: number): string {
  if (ms <= 0) return 'text-muted'
  if (ms < 100) return 'text-success'
  if (ms < 300) return 'text-warning'
  return 'text-danger'
}

export function latencyBarWidth(ms: number): string {
  if (ms <= 0) return '0%'
  const pct = Math.min(100, (ms / 500) * 100)
  return `${pct}%`
}

export function latencyBarColor(ms: number): string {
  if (ms <= 0) return 'bg-surface-3'
  if (ms < 100) return 'bg-success'
  if (ms < 300) return 'bg-warning'
  return 'bg-danger'
}
