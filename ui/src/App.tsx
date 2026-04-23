import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { Sidebar } from './components/Sidebar'
import { Dashboard } from './pages/Dashboard'
import { ConfigManagement } from './pages/ConfigManagement'
import { ActivityLog } from './pages/ActivityLog'
import { Setup } from './pages/Setup'

export default function App() {
  return (
    <BrowserRouter>
      <div className="flex h-screen overflow-hidden">
        <Sidebar />
        <main className="flex-1 overflow-y-auto bg-surface-0">
          <Routes>
            <Route path="/"        element={<Dashboard />} />
            <Route path="/config"  element={<ConfigManagement />} />
            <Route path="/activity" element={<ActivityLog />} />
            <Route path="/setup"   element={<Setup />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  )
}
