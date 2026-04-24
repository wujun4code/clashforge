import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { Sidebar } from './components/Sidebar'
import { Dashboard } from './pages/Dashboard'
import { ConfigManagement } from './pages/ConfigManagement'
import { ActivityLog } from './pages/ActivityLog'
import { Setup } from './pages/Setup'
import { Settings } from './pages/Settings'

export default function App() {
  return (
    <BrowserRouter>
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-[100] focus:rounded-xl focus:bg-brand focus:px-4 focus:py-2 focus:text-white"
      >
        跳转到主要内容
      </a>
      <div className="relative flex h-screen overflow-hidden bg-surface-0 text-[color:var(--text-primary)]">
        {/* Cozy ambient wash — soft cat-blue + paw-pink halos, no neon */}
        <div className="pointer-events-none absolute inset-0">
          <div
            className="absolute inset-0 opacity-90"
            style={{
              background:
                'radial-gradient(circle at 15% 12%, rgba(106,168,224,0.18), transparent 45%),' +
                'radial-gradient(circle at 85% 18%, rgba(200,181,232,0.14), transparent 45%),' +
                'radial-gradient(circle at 82% 88%, rgba(244,166,181,0.13), transparent 42%),' +
                'radial-gradient(circle at 18% 92%, rgba(143,212,168,0.10), transparent 40%)',
            }}
          />
          {/* Soft vignette for depth */}
          <div
            className="absolute inset-0"
            style={{
              background:
                'radial-gradient(ellipse at center, transparent 55%, rgba(12,18,32,0.35) 100%)',
            }}
          />
        </div>
        <Sidebar />
        <main id="main-content" className="relative z-10 flex-1 overflow-y-auto">
          <div className="mx-auto min-h-full w-full max-w-[1680px] px-4 py-4 md:px-6 md:py-6">
            <Routes>
              <Route path="/" element={<Dashboard />} />
              <Route path="/config" element={<ConfigManagement />} />
              <Route path="/activity" element={<ActivityLog />} />
              <Route path="/setup" element={<Setup />} />
              <Route path="/settings" element={<Settings />} />
            </Routes>
          </div>
        </main>
      </div>
    </BrowserRouter>
  )
}
