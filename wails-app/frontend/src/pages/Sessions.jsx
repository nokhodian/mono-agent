import { useState, useEffect, useCallback } from 'react'
import { Shield, Trash2, RefreshCw, Terminal } from 'lucide-react'
import { api, PLATFORM_COLORS } from '../services/api.js'

const PLATFORM_ABBR = {
  INSTAGRAM: 'IG',
  LINKEDIN:  'LI',
  X:         'X',
  TIKTOK:    'TK',
  EMAIL:     'EM',
  TELEGRAM:  'TG',
}

function formatExpiry(expiryStr) {
  if (!expiryStr) return { text: 'Unknown', expired: true }
  try {
    const exp = new Date(expiryStr)
    const now = new Date()
    if (exp < now) return { text: 'Expired', expired: true }
    const diffMs = exp - now
    const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))
    const diffHours = Math.floor((diffMs % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60))
    if (diffDays > 0) return { text: `${diffDays}d ${diffHours}h left`, expired: false }
    return { text: `${diffHours}h left`, expired: false }
  } catch {
    return { text: expiryStr.slice(0, 10), expired: false }
  }
}

export default function Sessions({ onRefresh }) {
  const [sessions, setSessions] = useState([])
  const [loading, setLoading] = useState(true)

  const load = useCallback(async () => {
    setLoading(true)
    const data = await api.getSessions()
    setSessions(data || [])
    setLoading(false)
  }, [])

  useEffect(() => { load() }, [load])

  const handleDelete = async (id, username, platform) => {
    if (!confirm(`Remove session for @${username} on ${platform}?`)) return
    await api.deleteSession(id)
    setSessions(prev => prev.filter(s => s.id !== id))
    onRefresh?.()
  }

  // Group by platform
  const grouped = {}
  sessions.forEach(s => {
    if (!grouped[s.platform]) grouped[s.platform] = []
    grouped[s.platform].push(s)
  })

  const platforms = Object.keys(grouped).sort()

  return (
    <>
      <div className="page-header">
        <div className="page-header-left">
          <div className="page-title">Sessions</div>
          <div className="page-subtitle">Authentication</div>
        </div>
        <div className="page-header-right">
          <button className="btn btn-ghost btn-sm" onClick={load} style={{ gap: 5 }}>
            <RefreshCw size={12} /> Refresh
          </button>
        </div>
      </div>

      <div className="page-body">
        {loading ? (
          <div className="empty-state"><div className="spinner" /></div>
        ) : sessions.length === 0 ? (
          <div className="empty-state">
            <div className="empty-state-icon"><Shield size={40} /></div>
            <div className="empty-state-title">No Sessions</div>
            <div className="empty-state-desc">
              Authenticate to a platform first.
            </div>
            <div style={{
              marginTop: 16,
              padding: '12px 16px',
              background: 'var(--elevated)',
              borderRadius: 'var(--radius)',
              border: '1px solid var(--border)',
              fontFamily: 'var(--font-mono)',
              fontSize: 12,
              color: 'var(--cyan-dim)',
            }}>
              <div style={{ color: 'var(--text-muted)', fontSize: 10, marginBottom: 6, letterSpacing: 1, textTransform: 'uppercase' }}>Terminal</div>
              <div>monoes login instagram</div>
              <div>monoes login linkedin</div>
              <div>monoes login x</div>
            </div>
          </div>
        ) : (
          <div>
            {platforms.map(platform => {
              const platformSessions = grouped[platform]
              const color = PLATFORM_COLORS[platform] || 'var(--cyan)'
              const abbr = PLATFORM_ABBR[platform.toUpperCase()] || platform.slice(0, 2).toUpperCase()
              const activeCount = platformSessions.filter(s => s.active).length

              return (
                <div key={platform} style={{ marginBottom: 20 }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 10 }}>
                    <div style={{
                      width: 6, height: 20, background: color,
                      borderRadius: 3, opacity: 0.8,
                      boxShadow: `0 0 8px ${color}`,
                    }} />
                    <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, fontWeight: 600, color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: 1.5 }}>
                      {platform}
                    </span>
                    <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)' }}>
                      {activeCount}/{platformSessions.length} active
                    </span>
                  </div>

                  <div className="session-grid">
                    {platformSessions.map(s => {
                      const { text: expiryText, expired } = formatExpiry(s.expiry)
                      return (
                        <div
                          key={s.id}
                          className={`session-card ${s.active ? 'active' : 'expired'}`}
                          style={{ '--platform-color': color }}
                        >
                          <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between' }}>
                            <div className="platform-icon-lg" style={{
                              background: `${color}18`,
                              color: color,
                              border: `1px solid ${color}30`,
                            }}>
                              {abbr}
                            </div>
                            <button
                              className="btn btn-danger btn-sm btn-icon"
                              onClick={() => handleDelete(s.id, s.username, s.platform)}
                              title={`Remove @${s.username} session`}
                              aria-label={`Remove @${s.username} session`}
                            >
                              <Trash2 size={12} />
                            </button>
                          </div>

                          <div style={{ fontFamily: 'var(--font-mono)', fontSize: 14, fontWeight: 600, color: 'var(--text)', marginBottom: 4 }}>
                            @{s.username}
                          </div>

                          <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 10 }}>
                            <span className={`status-dot ${s.active ? 'connected' : 'disconnected'}`} />
                            <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: s.active ? 'var(--green-neon)' : 'var(--red)' }}>
                              {s.active ? 'Active' : 'Expired'}
                            </span>
                          </div>

                          <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)', display: 'flex', flexDirection: 'column', gap: 3 }}>
                            <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                              <span>Expires</span>
                              <span style={{ color: expired ? 'var(--red)' : 'var(--text-secondary)' }}>{expiryText}</span>
                            </div>
                            {s.added_at && (
                              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                                <span>Added</span>
                                <span style={{ color: 'var(--text-secondary)' }}>{s.added_at.slice(0, 10)}</span>
                              </div>
                            )}
                          </div>
                        </div>
                      )
                    })}
                  </div>
                </div>
              )
            })}

            <div style={{
              marginTop: 24,
              padding: '14px 16px',
              background: 'var(--surface)',
              borderRadius: 'var(--radius-lg)',
              border: '1px solid var(--border)',
            }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 10 }}>
                <Terminal size={13} style={{ color: 'var(--cyan)' }} />
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: 1.5 }}>
                  Add Account
                </span>
              </div>
              <div style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--cyan-dim)', lineHeight: 1.9 }}>
                {['instagram', 'linkedin', 'x', 'tiktok', 'telegram'].map(p => (
                  <div key={p}>monoes login {p}</div>
                ))}
              </div>
            </div>
          </div>
        )}
      </div>
    </>
  )
}
