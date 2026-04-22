import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { Sidebar } from './components/Sidebar'
import { Dashboard } from './pages/Dashboard'
import { Proxies } from './pages/Proxies'
import { Subscriptions } from './pages/Subscriptions'
import { Connections } from './pages/Connections'
import { Logs } from './pages/Logs'
import { Settings } from './pages/Settings'

export default function App() {
  return (
    <BrowserRouter>
      <div className="flex h-screen overflow-hidden">
        <Sidebar />
        <main className="flex-1 overflow-y-auto bg-surface-0">
          <Routes>
            <Route path="/"              element={<Dashboard />} />
            <Route path="/proxies"       element={<Proxies />} />
            <Route path="/subscriptions" element={<Subscriptions />} />
            <Route path="/connections"   element={<Connections />} />
            <Route path="/logs"          element={<Logs />} />
            <Route path="/settings"      element={<Settings />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  )
}
