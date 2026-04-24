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
      <div className="relative flex h-screen overflow-hidden bg-surface-0 text-[#F0EFF8]">
        {/* Ambient cyber-glow background — cyan → violet → amber wash */}
        <div className="pointer-events-none absolute inset-0 opacity-80">
          <div
            className="absolute inset-0"
            style={{
              background:
                'radial-gradient(circle at 12% 8%, rgba(0,245,255,0.14), transparent 42%),' +
                'radial-gradient(circle at 88% 18%, rgba(167,139,250,0.14), transparent 45%),' +
                'radial-gradient(circle at 82% 96%, rgba(249,115,22,0.12), transparent 40%),' +
                'radial-gradient(circle at 18% 92%, rgba(255,0,170,0.08), transparent 38%)',
            }}
          />
          {/* vignette */}
          <div
            className="absolute inset-0"
            style={{
              background:
                'radial-gradient(ellipse at center, transparent 40%, rgba(2,4,8,0.55) 100%)',
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
