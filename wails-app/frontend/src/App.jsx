import { useState, useEffect, useCallback } from 'react'
import Sidebar from './components/Sidebar.jsx'
import StatusBar from './components/StatusBar.jsx'
import Dashboard from './pages/Dashboard.jsx'
import Actions from './pages/Actions.jsx'
import People from './pages/People.jsx'
import Profile from './pages/Profile.jsx'
import Sessions from './pages/Sessions.jsx'
import Logs from './pages/Logs.jsx'
import Workflow from './pages/Workflow.jsx'
import { api, onLogEntry, onActionComplete } from './services/api.js'

export default function App() {
  const [activePage, setActivePage] = useState('dashboard')
  const [profileId, setProfileId] = useState(null)
  const [dbConnected, setDbConnected] = useState(false)
  const [stats, setStats] = useState(null)
  const [logs, setLogs] = useState([])
  const [peopleRefreshKey, setPeopleRefreshKey] = useState(0)

  const openProfile = useCallback((id) => {
    setProfileId(id)
    setActivePage('profile')
  }, [])

  const closeProfile = useCallback(() => {
    setActivePage('people')
    setProfileId(null)
  }, [])

  // Initial data load
  useEffect(() => {
    const checkDB = async () => {
      const connected = await api.isDBConnected()
      setDbConnected(!!connected)
    }
    const loadStats = async () => {
      const s = await api.getDashboardStats()
      if (s) setStats(s)
    }
    const loadLogs = async () => {
      const l = await api.getLogs()
      if (l) setLogs(l)
    }
    checkDB()
    loadStats()
    loadLogs()
  }, [])

  // Live log streaming
  useEffect(() => {
    const off = onLogEntry((entry) => {
      setLogs(prev => {
        const next = [...prev, entry]
        return next.length > 500 ? next.slice(-500) : next
      })
    })
    return off
  }, [])

  // Action completion refresh
  useEffect(() => {
    const off = onActionComplete(async () => {
      const s = await api.getDashboardStats()
      if (s) setStats(s)
      setPeopleRefreshKey(k => k + 1)
    })
    return off
  }, [])

  const refreshStats = useCallback(async () => {
    const s = await api.getDashboardStats()
    if (s) setStats(s)
  }, [])

  const pages = {
    dashboard: <Dashboard stats={stats} onRefresh={refreshStats} onNavigate={setActivePage} />,
    actions:   <Actions onRefresh={refreshStats} />,
    workflow:  <Workflow />,
    people:    <People key={peopleRefreshKey} onProfile={openProfile} />,
    profile:   <Profile id={profileId} onBack={closeProfile} onOpenURL={api.openURL} />,
    sessions:  <Sessions onRefresh={refreshStats} />,
    logs:      <Logs logs={logs} onClear={() => { api.clearLogs(); setLogs([]) }} />,
  }

  return (
    <div className="app-layout">
      <Sidebar
        activePage={activePage}
        onNavigate={setActivePage}
        stats={stats}
        dbConnected={dbConnected}
      />
      <main className="main-content">
        {pages[activePage] || pages.dashboard}
      </main>
      <StatusBar stats={stats} dbConnected={dbConnected} />
    </div>
  )
}
