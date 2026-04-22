import { create } from 'zustand'

interface TrafficPoint { ts: number; up: number; down: number }

interface AppState {
  coreState: string
  corePid: number
  trafficHistory: TrafficPoint[]
  currentUp: number
  currentDown: number
  connCount: number
  setCoreState: (s: string, pid: number) => void
  pushTraffic: (p: TrafficPoint) => void
  setConnCount: (n: number) => void
}

export const useStore = create<AppState>((set) => ({
  coreState: 'unknown',
  corePid: 0,
  trafficHistory: [],
  currentUp: 0,
  currentDown: 0,
  connCount: 0,
  setCoreState: (state, pid) => set({ coreState: state, corePid: pid }),
  pushTraffic: (p) => set(s => {
    const history = [...s.trafficHistory, p].slice(-60)
    return { trafficHistory: history, currentUp: p.up, currentDown: p.down }
  }),
  setConnCount: (n) => set({ connCount: n }),
}))
