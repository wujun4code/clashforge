import { useState } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { Menu } from 'lucide-react'
import { Sidebar } from './components/Sidebar'
import { Dashboard } from './pages/Dashboard'
import { Nodes } from './pages/Nodes'
import { ConfigManagement } from './pages/ConfigManagement'
import { PerDeviceRules } from './pages/PerDeviceRules'
import { ActivityLog } from './pages/ActivityLog'
import { Setup } from './pages/Setup'
import { Settings } from './pages/Settings'
import { Publish } from './pages/Publish'

export default function App() {
  const [mobileNavOpen, setMobileNavOpen] = useState(false)

  return (
    <BrowserRouter>
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-[100] focus:rounded-xl focus:bg-brand focus:px-4 focus:py-2 focus:text-white"
      >
        跳转到主要内容
      </a>
      <div className="app-shell relative flex h-screen overflow-hidden bg-surface-0 text-white">
        <div
          className="app-backdrop pointer-events-none absolute inset-0"
          style={{ background: 'var(--app-background)' }}
        />

        {/* Mobile sidebar backdrop */}
        {mobileNavOpen && (
          <div
            className="fixed inset-0 z-40 bg-black/60 backdrop-blur-sm md:hidden"
            onClick={() => setMobileNavOpen(false)}
          />
        )}

        <Sidebar mobileOpen={mobileNavOpen} onClose={() => setMobileNavOpen(false)} />

        <main id="main-content" className="relative flex min-w-0 flex-1 flex-col overflow-hidden">
          {/* Mobile top bar — hidden on md+ */}
          <div className="flex flex-shrink-0 items-center gap-3 border-b border-white/[0.06] bg-surface-0/95 px-4 py-3 backdrop-blur-md md:hidden">
            <button
              className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-lg text-white/60 transition-colors hover:bg-white/[0.06] hover:text-white"
              onClick={() => setMobileNavOpen(true)}
              aria-label="打开菜单"
            >
              <Menu size={18} />
            </button>
            <div
              className="sidebar-logo-mark flex h-7 w-7 flex-shrink-0 items-center justify-center border border-brand/20 bg-brand/[0.09]"
              style={{ borderRadius: 'var(--radius-md)' }}
            >
              <img src="/favicon.svg" alt="" className="h-4 w-4" />
            </div>
            <span className="font-heading text-sm font-semibold text-white">ClashForge</span>
          </div>

          {/* Scrollable page content */}
          <div className="flex-1 overflow-y-auto">
            <div className="app-content mx-auto min-h-full w-full max-w-[1680px] px-4 py-4 md:px-6 md:py-6">
              <Routes>
                <Route path="/" element={<Dashboard />} />
                <Route path="/config" element={<ConfigManagement />} />
                <Route path="/device-rules" element={<PerDeviceRules />} />
                <Route path="/nodes" element={<Nodes />} />
                <Route path="/publish" element={<Publish />} />
                <Route path="/geodata" element={<Navigate to="/config?tab=geodata" replace />} />
                <Route path="/activity" element={<ActivityLog />} />
                <Route path="/setup" element={<Setup />} />
                <Route path="/settings" element={<Settings />} />
              </Routes>
            </div>
          </div>
        </main>
      </div>
    </BrowserRouter>
  )
}
