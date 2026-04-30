import { useEffect, useRef } from 'react'

type Handlers = {
  onTraffic?:    (d: { up: number; down: number; ts: number }) => void
  onCoreState?:  (d: { state: string; pid: number }) => void
  onSubUpdate?:  (d: { id: string; status: string; node_count?: number; error?: string }) => void
  onLog?:        (d: { level: string; msg: string; ts: number; fields?: Record<string, unknown> }) => void
  onConnCount?:  (d: { total: number; active: number }) => void
  onHealthState?: (d: { state: string; from?: string; reason?: string; trigger?: string; checked_at?: string }) => void
  onHealthIncidentOpened?: (d: { id: string; state: string; reason: string; opened_at: string }) => void
  onHealthIncidentResolved?: (d: { id: string; state: string; reason: string; resolved_at?: string }) => void
}

export function useSSE(handlers: Handlers) {
  const ref = useRef(handlers)
  ref.current = handlers

  useEffect(() => {
    let es: EventSource
    let retryTimer: ReturnType<typeof setTimeout>
    let dead = false

    const connect = () => {
      if (dead) return
      const secret = localStorage.getItem('cf_secret') || ''
      es = new EventSource(`/api/v1/events${secret ? `?secret=${secret}` : ''}`)

      const on = (ev: string, fn: (d: unknown) => void) =>
        es.addEventListener(ev, (e: MessageEvent) => fn(JSON.parse(e.data)))

      on('traffic',          d => ref.current.onTraffic?.(d as never))
      on('core_state',       d => ref.current.onCoreState?.(d as never))
      on('subscription_update', d => ref.current.onSubUpdate?.(d as never))
      on('log',              d => ref.current.onLog?.(d as never))
      on('connections_count',d => ref.current.onConnCount?.(d as never))
      on('health_state',     d => ref.current.onHealthState?.(d as never))
      on('health_incident_opened', d => ref.current.onHealthIncidentOpened?.(d as never))
      on('health_incident_resolved', d => ref.current.onHealthIncidentResolved?.(d as never))

      es.onerror = () => {
        es.close()
        retryTimer = setTimeout(connect, 3000)
      }
    }

    connect()
    return () => {
      dead = true
      clearTimeout(retryTimer)
      es?.close()
    }
  }, [])
}
