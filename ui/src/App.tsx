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
        className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-[100] focus:rounded-2xl focus:bg-violet-600 focus:px-4 focus:py-2 focus:text-white"
      >
        跳转到主要内容
      </a>
      <div className="cat-app-shell relative flex h-screen overflow-hidden bg-[#070b17] text-slate-100">
        <div className="cat-app-aurora pointer-events-none absolute inset-0" />
        <div className="cat-app-stars pointer-events-none absolute inset-0 opacity-70" />
        <Sidebar />
        <main id="main-content" className="relative z-10 flex-1 overflow-y-auto">
          <div className="mx-auto min-h-full w-full max-w-[1720px] px-4 py-4 md:px-6 md:py-6 xl:px-8">
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
