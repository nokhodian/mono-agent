import { useEffect, useState } from 'react'
import { RefreshCw, Zap, Users, Shield, List, TrendingUp, ChevronRight } from 'lucide-react'
import { STATE_COLORS, PLATFORM_COLORS } from '../services/api.js'

function StatCard({ icon: Icon, label, value, accentColor, iconBg, iconColor }) {
  return (
    <div className="stat-card" style={{ '--accent-color': accentColor, '--icon-bg': iconBg, '--icon-color': iconColor }}>
      <div className="stat-icon">
        <Icon size={16} />
      </div>
      <div className="stat-value">{value ?? '—'}</div>
      <div className="stat-label">{label}</div>
    </div>
  )
}

function ActionBadge({ state }) {
  const cls = `badge badge-state-${state.toLowerCase()}`
  return <span className={cls}>{state}</span>
}

function PlatformBadge({ platform }) {
  const cls = `badge badge-platform-${(platform || 'unknown').toLowerCase()}`
  return <span className={cls}>{platform}</span>
}

export default function Dashboard({ stats, onRefresh, onNavigate }) {
  const [refreshing, setRefreshing] = useState(false)

  const handleRefresh = async () => {
    setRefreshing(true)
    await onRefresh()
    setTimeout(() => setRefreshing(false), 400)
  }

  const actionsByState = stats?.actions_by_state || {}
  const sessions = stats?.sessions || []
  const recentActions = stats?.recent_actions || []
  const totalActions = stats?.total_actions || 0

  // Group sessions by platform
  const platformSessions = {}
  sessions.forEach(s => {
    if (!platformSessions[s.platform]) platformSessions[s.platform] = []
    platformSessions[s.platform].push(s)
  })

  const stateDots = Object.entries(actionsByState).map(([state, count]) => ({
    state, count,
    color: STATE_COLORS[state] || '#94a3b8',
    pct: totalActions > 0 ? Math.round((count / totalActions) * 100) : 0,
  }))

  return (
    <>
      <div className="page-header">
        <div className="page-header-left">
          <div className="page-title">Dashboard</div>
          <div className="page-subtitle">System Overview</div>
        </div>
        <div className="page-header-right">
          <button
            className="btn btn-ghost btn-sm"
            onClick={handleRefresh}
            style={{ gap: 5 }}
          >
            <RefreshCw size={13} style={{ animation: refreshing ? 'spin 0.7s linear infinite' : 'none' }} />
            Refresh
          </button>
        </div>
      </div>

      <div className="page-body">
        {/* Stat Cards */}
        <div className="stat-grid">
          <StatCard
            icon={Shield}
            label="Active Sessions"
            value={stats?.active_sessions}
            accentColor="var(--cyan)"
            iconBg="var(--cyan-glow)"
            iconColor="var(--cyan)"
          />
          <StatCard
            icon={Zap}
            label="Total Actions"
            value={stats?.total_actions}
            accentColor="var(--purple-light)"
            iconBg="rgba(124,58,237,0.1)"
            iconColor="var(--purple-light)"
          />
          <StatCard
            icon={Users}
            label="People Found"
            value={stats?.total_people}
            accentColor="var(--green-neon)"
            iconBg="rgba(16,185,129,0.1)"
            iconColor="var(--green-neon)"
          />
          <StatCard
            icon={List}
            label="Social Lists"
            value={stats?.total_lists}
            accentColor="var(--orange)"
            iconBg="rgba(249,115,22,0.1)"
            iconColor="var(--orange)"
          />
        </div>

        <div className="dashboard-grid">
          {/* Left: Recent Actions + Action State breakdown */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            {/* Action State Breakdown */}
            {stateDots.length > 0 && (
              <div className="card">
                <div className="section-header">
                  <div className="section-title">
                    <TrendingUp size={12} /> Action States
                  </div>
                  <button
                    className="btn btn-ghost btn-sm"
                    onClick={() => onNavigate('actions')}
                    style={{ fontSize: 11, gap: 3 }}
                  >
                    View All <ChevronRight size={11} />
                  </button>
                </div>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                  {stateDots.map(({ state, count, color, pct }) => (
                    <div key={state} style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                      <span style={{
                        fontFamily: 'var(--font-mono)',
                        fontSize: 10,
                        color: color,
                        width: 72,
                        flexShrink: 0,
                        textTransform: 'uppercase',
                        letterSpacing: 0.5,
                      }}>{state}</span>
                      <div className="platform-bar-track">
                        <div className="platform-bar-fill"
                             style={{ width: `${pct}%`, background: color }} />
                      </div>
                      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)', width: 28, textAlign: 'right' }}>
                        {count}
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Recent Actions */}
            <div className="card">
              <div className="section-header">
                <div className="section-title">
                  <Zap size={12} /> Recent Actions
                </div>
                <button
                  className="btn btn-ghost btn-sm"
                  onClick={() => onNavigate('actions')}
                  style={{ fontSize: 11, gap: 3 }}
                >
                  View All <ChevronRight size={11} />
                </button>
              </div>

              {recentActions.length === 0 ? (
                <div className="empty-state" style={{ padding: '24px 0' }}>
                  <div className="empty-state-desc">No actions yet. Create one to get started.</div>
                </div>
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                  {recentActions.map(action => (
                    <div key={action.id} style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 10,
                      padding: '10px 12px',
                      background: 'var(--elevated)',
                      borderRadius: 'var(--radius)',
                      border: '1px solid var(--border-dim)',
                    }}>
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div style={{ fontFamily: 'var(--font-display)', fontSize: 13, fontWeight: 600, color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                          {action.title}
                        </div>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 3 }}>
                          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)' }}>
                            {action.type}
                          </span>
                        </div>
                      </div>
                      <PlatformBadge platform={action.platform} />
                      <ActionBadge state={action.state} />
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>

          {/* Right: Active Sessions Panel */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            <div className="card" style={{ flex: 1 }}>
              <div className="section-header">
                <div className="section-title">
                  <Shield size={12} /> Connected Accounts
                </div>
                <button
                  className="btn btn-ghost btn-sm"
                  onClick={() => onNavigate('sessions')}
                  style={{ fontSize: 11, gap: 3 }}
                >
                  Manage <ChevronRight size={11} />
                </button>
              </div>

              {sessions.length === 0 ? (
                <div className="empty-state" style={{ padding: '24px 0' }}>
                  <div className="empty-state-desc">No active sessions. Run <code style={{ fontFamily: 'var(--font-mono)', color: 'var(--cyan)', fontSize: 11 }}>monoes login &lt;platform&gt;</code> to authenticate.</div>
                </div>
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                  {sessions.map((s, i) => (
                    <div key={i} style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 10,
                      padding: '8px 10px',
                      background: 'var(--elevated)',
                      borderRadius: 'var(--radius)',
                      border: `1px solid ${s.active ? 'var(--border)' : 'var(--border-dim)'}`,
                      opacity: s.active ? 1 : 0.5,
                    }}>
                      <span className={`status-dot ${s.active ? 'connected' : 'disconnected'}`} />
                      <div style={{ minWidth: 0, flex: 1 }}>
                        <div style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--text)' }}>{s.username}</div>
                        <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)' }}>{s.platform}</div>
                      </div>
                      <span className={`badge badge-platform-${s.platform.toLowerCase()}`}>{s.platform}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* DB info */}
            {stats?.db_path && (
              <div className="card" style={{ padding: 12 }}>
                <div className="section-title" style={{ marginBottom: 8 }}>Database</div>
                <div style={{
                  fontFamily: 'var(--font-mono)',
                  fontSize: 10,
                  color: 'var(--text-muted)',
                  wordBreak: 'break-all',
                  lineHeight: 1.6,
                }}>
                  {stats.db_path}
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </>
  )
}
