import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { Sidebar } from './components/Sidebar'
import { Dashboard } from './pages/Dashboard'
import { Nodes } from './pages/Nodes'
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
      <div className="app-shell relative flex h-screen overflow-hidden bg-surface-0 text-white">
        <div
          className="app-backdrop pointer-events-none absolute inset-0"
          style={{
            background: 'var(--app-background)',
          }}
        />
        <Sidebar />
        <main id="main-content" className="relative z-10 flex-1 overflow-y-auto">
          <div className="app-content mx-auto min-h-full w-full max-w-[1680px] px-4 py-4 md:px-6 md:py-6">
            <Routes>
              <Route path="/" element={<Dashboard />} />
              <Route path="/config" element={<ConfigManagement />} />
              <Route path="/nodes" element={<Nodes />} />
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
