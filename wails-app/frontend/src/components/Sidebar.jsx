import {
  LayoutDashboard, Zap, Users, Database,
  Terminal, Shield, ChevronRight, GitBranch
} from 'lucide-react'

const NAV_ITEMS = [
  { id: 'dashboard', label: 'Dashboard',  icon: LayoutDashboard, section: 'MAIN' },
  { id: 'actions',   label: 'Actions',    icon: Zap,             section: 'MAIN' },
  { id: 'workflow',  label: 'Workflows',  icon: GitBranch,       section: 'MAIN' },
  { id: 'people',    label: 'People',     icon: Users,           section: 'DATA' },
  { id: 'sessions',  label: 'Sessions',   icon: Shield,          section: 'DATA' },
  { id: 'logs',      label: 'Live Logs',  icon: Terminal,        section: 'DEBUG' },
]

export default function Sidebar({ activePage, onNavigate, stats, dbConnected }) {
  const actionCount = stats
    ? Object.values(stats.actions_by_state || {}).reduce((a, b) => a + b, 0)
    : null

  const getBadge = (id) => {
    if (!stats) return null
    if (id === 'actions' && actionCount > 0) return actionCount
    if (id === 'people' && stats.total_people > 0) return stats.total_people
    if (id === 'sessions' && stats.active_sessions > 0) return stats.active_sessions
    return null
  }

  const sections = [...new Set(NAV_ITEMS.map(i => i.section))]

  return (
    <aside className="sidebar">
      <div className="sidebar-titlebar">
        <div className="sidebar-logo">
          <div className="logo-mark">MN</div>
          <div>
            <div className="logo-text">Monoes</div>
            <div className="logo-sub">Agent v1.0</div>
          </div>
        </div>
      </div>

      <nav className="sidebar-nav" aria-label="Main navigation">
        {sections.map(section => (
          <div key={section}>
            <div className="nav-section-label">{section}</div>
            {NAV_ITEMS.filter(i => i.section === section).map(item => {
              const Icon = item.icon
              const badge = getBadge(item.id)
              const isActive = activePage === item.id
              return (
                <div
                  key={item.id}
                  className={`nav-item ${isActive ? 'active' : ''}`}
                  onClick={() => onNavigate(item.id)}
                  onKeyDown={(e) => (e.key === 'Enter' || e.key === ' ') && onNavigate(item.id)}
                  role="button"
                  tabIndex={0}
                  aria-current={isActive ? 'page' : undefined}
                  aria-label={item.label}
                >
                  <Icon className="nav-icon" size={15} />
                  <span>{item.label}</span>
                  {badge != null && (
                    <span className="nav-badge" aria-label={`${badge} items`}>{badge > 999 ? '999+' : badge}</span>
                  )}
                </div>
              )
            })}
          </div>
        ))}
      </nav>

      <div className="sidebar-footer">
        {/* Platform session indicators */}
        {stats?.sessions?.length > 0 && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
            {['INSTAGRAM', 'LINKEDIN', 'X', 'TIKTOK'].map(p => {
              const session = stats.sessions.find(s => s.platform === p && s.active)
              if (!session) return null
              return (
                <div key={p} style={{
                  display: 'flex', alignItems: 'center', gap: 6,
                  padding: '4px 8px', borderRadius: 4,
                  fontSize: 11, fontFamily: 'var(--font-mono)',
                  color: 'var(--text-secondary)',
                }}>
                  <span className="status-dot connected" />
                  <span style={{ color: 'var(--text-muted)', fontSize: 9, textTransform: 'uppercase', letterSpacing: 1 }}>
                    {p.slice(0, 2)}
                  </span>
                  <span style={{ marginLeft: 2 }}>{session.username}</span>
                </div>
              )
            })}
          </div>
        )}

        <div className="db-status">
          <span className={`status-dot ${dbConnected ? 'connected pulse' : 'disconnected'}`} />
          <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {dbConnected ? 'DB connected' : 'DB offline'}
          </span>
        </div>
      </div>
    </aside>
  )
}
