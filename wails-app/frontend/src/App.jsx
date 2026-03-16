import { useState, useEffect, useCallback } from 'react'
import Sidebar from './components/Sidebar.jsx'
import StatusBar from './components/StatusBar.jsx'
import Dashboard from './pages/Dashboard.jsx'
import People from './pages/People.jsx'
import Profile from './pages/Profile.jsx'
import PostDetail from './pages/PostDetail.jsx'
import Connections from './pages/Connections.jsx'
import AIProviders from './pages/AIProviders.jsx'
import Logs from './pages/Logs.jsx'
import NodeRunner from './pages/NodeRunner.jsx'
import SettingsPage from './pages/Settings.jsx'
import { api, onLogEntry, onActionComplete } from './services/api.js'

export default function App() {
  const [activePage, setActivePage] = useState('dashboard')
  const [profileId, setProfileId] = useState(null)
  const [postId, setPostId] = useState(null)
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

  const openPost = useCallback((id) => {
    setPostId(id)
    setActivePage('postDetail')
  }, [])

  const closePost = useCallback(() => {
    setPostId(null)
    setActivePage('profile')
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
    noderunner: <NodeRunner onNavigate={setActivePage} />,
    people:    <People key={peopleRefreshKey} onProfile={openProfile} />,
    profile:   <Profile id={profileId} onBack={closeProfile} onOpenURL={api.openURL} onOpenPost={openPost} />,
    postDetail: <PostDetail id={postId} onBack={closePost} onOpenURL={api.openURL} />,
    connections: <Connections onRefresh={refreshStats} />,
    ai: <AIProviders />,
    logs:      <Logs logs={logs} onClear={() => { api.clearLogs(); setLogs([]) }} />,
    settings:  <SettingsPage />,
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
