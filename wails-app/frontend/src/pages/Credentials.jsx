import { useState, useEffect, useCallback, useRef } from 'react'
import { Key, RefreshCw, Terminal, Trash2, CheckCircle, Loader } from 'lucide-react'
import { api } from '../services/api.js'

// Map raw category strings to display group names
const CATEGORY_LABELS = {
  service:       'Services & APIs',
  communication: 'Communication',
  database:      'Databases',
}

// Preferred display order for categories
const CATEGORY_ORDER = ['service', 'communication', 'database']

function groupPlatformsByCategory(platforms) {
  const groups = {}
  for (const p of platforms) {
    const cat = (p.Category || p.category || 'service').toLowerCase()
    if (!groups[cat]) groups[cat] = []
    groups[cat].push(p)
  }
  return groups
}

// Resolve a connection for a given platform id from the connections list
function findConnection(connections, platformId) {
  return connections.find(c => c.platform === platformId || c.Platform === platformId) || null
}

function ConnectPanel({ platform, onDismiss }) {
  const platformId = platform.ID || platform.id || platform.Name || platform.name || ''
  const methods = platform.Methods || platform.methods || []

  return (
    <div style={{
      marginTop: 10,
      padding: '14px 16px',
      background: 'var(--elevated)',
      border: '1px solid var(--border-bright)',
      borderRadius: 'var(--radius-lg)',
      animation: 'slide-up 160ms ease',
    }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 10 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 7 }}>
          <Terminal size={13} style={{ color: 'var(--cyan)' }} />
          <span style={{
            fontFamily: 'var(--font-mono)',
            fontSize: 10,
            color: 'var(--text-muted)',
            textTransform: 'uppercase',
            letterSpacing: 1.5,
          }}>
            Connect via CLI
          </span>
        </div>
        <button className="btn btn-ghost btn-sm" onClick={onDismiss} style={{ padding: '2px 8px', fontSize: 11 }}>
          Dismiss
        </button>
      </div>

      {methods.length > 0 && (
        <div style={{ marginBottom: 10, display: 'flex', gap: 6, flexWrap: 'wrap' }}>
          {methods.map(m => (
            <span key={m} style={{
              fontFamily: 'var(--font-mono)',
              fontSize: 10,
              padding: '2px 8px',
              borderRadius: 4,
              background: 'var(--surface)',
              border: '1px solid var(--border)',
              color: 'var(--cyan-dim)',
            }}>
              {m}
            </span>
          ))}
        </div>
      )}

      <div style={{
        fontFamily: 'var(--font-mono)',
        fontSize: 12,
        color: 'var(--cyan-dim)',
        lineHeight: 1.8,
      }}>
        <div style={{ color: 'var(--text-muted)', fontSize: 10, marginBottom: 4 }}>Run in terminal:</div>
        <div style={{ color: 'var(--text-secondary)' }}>
          monoes connect <span style={{ color: 'var(--cyan)' }}>{platformId.toLowerCase()}</span>
        </div>
      </div>
    </div>
  )
}

function PlatformRow({ platform, connection, onTest, onRemove, onConnectOpen, isConnectOpen, onConnectDismiss, testing, removing }) {
  const platformId  = platform.ID   || platform.id   || ''
  const displayName = platform.Name || platform.name || platformId
  const authMethod  = connection
    ? (connection.auth_type || connection.AuthType || connection.method || '—')
    : '—'
  const username = connection
    ? (connection.username || connection.Username || connection.account || '—')
    : '—'
  const isConnected = !!connection
  const isActive = connection && (
    ['active', 'connected', 'ok'].includes(connection.status) ||
    ['active', 'connected', 'ok'].includes(connection.Status)
  )

  return (
    <div>
      <div style={{
        display: 'grid',
        gridTemplateColumns: '18px 1fr 140px 100px auto',
        alignItems: 'center',
        gap: 12,
        padding: '10px 14px',
        background: 'var(--surface)',
        border: '1px solid var(--border)',
        borderRadius: isConnectOpen ? 'var(--radius) var(--radius) 0 0' : 'var(--radius)',
        borderBottom: isConnectOpen ? '1px solid var(--border-dim)' : undefined,
        transition: 'all var(--transition)',
      }}>
        {/* Status dot */}
        <span className={`status-dot ${isActive ? 'connected' : ''}`} />

        {/* Platform name */}
        <span style={{
          fontFamily: 'var(--font-mono)',
          fontSize: 13,
          fontWeight: 600,
          color: 'var(--text)',
        }}>
          {displayName}
        </span>

        {/* Username / account */}
        <span style={{
          fontFamily: 'var(--font-mono)',
          fontSize: 11,
          color: connection ? 'var(--text-secondary)' : 'var(--text-muted)',
        }}>
          {username}
        </span>

        {/* Auth method */}
        <span style={{
          fontFamily: 'var(--font-mono)',
          fontSize: 11,
          color: connection ? 'var(--cyan-dim)' : 'var(--text-muted)',
        }}>
          {authMethod}
        </span>

        {/* Actions */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          {connection ? (
            <>
              <button
                className="btn btn-secondary btn-sm"
                onClick={() => onTest(connection.id || connection.ID)}
                disabled={testing}
                title="Re-validate connection"
              >
                {testing
                  ? <Loader size={11} style={{ animation: 'spin 0.7s linear infinite' }} />
                  : <CheckCircle size={11} />
                }
                {' '}Test
              </button>
              <button
                className="btn btn-danger btn-sm btn-icon"
                onClick={() => onRemove(connection.id || connection.ID, displayName)}
                disabled={removing}
                title={`Remove ${displayName} connection`}
                aria-label={`Remove ${displayName} connection`}
              >
                {removing
                  ? <Loader size={11} style={{ animation: 'spin 0.7s linear infinite' }} />
                  : <Trash2 size={11} />
                }
              </button>
            </>
          ) : (
            <button
              className="btn btn-primary btn-sm"
              onClick={onConnectOpen}
              title={`Connect ${displayName}`}
            >
              + Connect
            </button>
          )}
        </div>
      </div>

      {isConnectOpen && (
        <ConnectPanel platform={platform} onDismiss={onConnectDismiss} />
      )}
    </div>
  )
}

export default function Credentials({ onRefresh }) {
  const [platforms, setPlatforms]     = useState([])
  const [connections, setConnections] = useState([])
  const [loading, setLoading]         = useState(true)
  const [openPanel, setOpenPanel]     = useState(null)   // platform id with open connect panel
  const [testing, setTesting]         = useState({})     // { [connectionId]: bool }
  const [removing, setRemoving]       = useState({})     // { [connectionId]: bool }

  const pollRef = useRef(null)

  const loadConnections = useCallback(async () => {
    const data = await api.listConnections('')
    setConnections(data || [])
  }, [])

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [plats, conns] = await Promise.all([
        api.listPlatforms('API'),
        api.listConnections(''),
      ])
      setPlatforms(plats || [])
      setConnections(conns || [])
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load() }, [load])

  // Poll for new connections every 10s while a connect panel is open
  useEffect(() => {
    if (openPanel) {
      pollRef.current = setInterval(loadConnections, 10000)
    } else {
      clearInterval(pollRef.current)
    }
    return () => clearInterval(pollRef.current)
  }, [openPanel, loadConnections])

  const handleTest = useCallback(async (connectionId) => {
    setTesting(prev => ({ ...prev, [connectionId]: true }))
    try {
      await api.testConnection(connectionId)
      await loadConnections()
    } finally {
      setTesting(prev => ({ ...prev, [connectionId]: false }))
    }
  }, [loadConnections])

  const handleRemove = useCallback(async (connectionId, displayName) => {
    if (!confirm(`Remove connection for ${displayName}?`)) return
    setRemoving(prev => ({ ...prev, [connectionId]: true }))
    try {
      const result = await api.removeConnection(connectionId)
      if (result && result.startsWith('error:')) return
      setConnections(prev => prev.filter(c => (c.id || c.ID) !== connectionId))
      onRefresh?.()
    } finally {
      setRemoving(prev => ({ ...prev, [connectionId]: false }))
    }
  }, [onRefresh, loadConnections])

  const groups = groupPlatformsByCategory(platforms)

  return (
    <>
      <div className="page-header">
        <div className="page-header-left">
          <div className="page-title">Credentials</div>
          <div className="page-subtitle">API Connections</div>
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
        ) : platforms.length === 0 ? (
          <div className="empty-state">
            <div className="empty-state-icon"><Key size={40} /></div>
            <div className="empty-state-title">No API Platforms</div>
            <div className="empty-state-desc">
              No platforms with API connection type were found.
            </div>
          </div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 28 }}>
            {CATEGORY_ORDER.filter(cat => groups[cat] && groups[cat].length > 0).map(cat => (
              <div key={cat}>
                {/* Section header */}
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 10 }}>
                  <div style={{
                    width: 16,
                    height: 1,
                    background: 'var(--cyan)',
                    opacity: 0.7,
                  }} />
                  <span style={{
                    fontFamily: 'var(--font-mono)',
                    fontSize: 10.5,
                    fontWeight: 600,
                    color: 'var(--text-muted)',
                    textTransform: 'uppercase',
                    letterSpacing: 2,
                  }}>
                    {CATEGORY_LABELS[cat] || cat}
                  </span>
                  <span style={{
                    fontFamily: 'var(--font-mono)',
                    fontSize: 10,
                    color: 'var(--text-muted)',
                  }}>
                    {groups[cat].filter(p => findConnection(connections, p.ID || p.id)).length}
                    /{groups[cat].length} connected
                  </span>
                </div>

                {/* Table-like header row */}
                <div style={{
                  display: 'grid',
                  gridTemplateColumns: '18px 1fr 140px 100px auto',
                  gap: 12,
                  padding: '5px 14px 7px',
                  borderBottom: '1px solid var(--border-dim)',
                  marginBottom: 6,
                }}>
                  {['', 'Platform', 'Account', 'Auth', ''].map((h, i) => (
                    <span key={i} style={{
                      fontFamily: 'var(--font-mono)',
                      fontSize: 9.5,
                      fontWeight: 600,
                      color: 'var(--text-muted)',
                      textTransform: 'uppercase',
                      letterSpacing: 1.5,
                    }}>{h}</span>
                  ))}
                </div>

                {/* Platform rows */}
                <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                  {groups[cat].map(platform => {
                    const pid = platform.ID || platform.id || ''
                    const conn = findConnection(connections, pid)
                    const connId = conn ? (conn.id || conn.ID) : null
                    return (
                      <PlatformRow
                        key={pid}
                        platform={platform}
                        connection={conn}
                        onTest={handleTest}
                        onRemove={handleRemove}
                        onConnectOpen={() => setOpenPanel(pid)}
                        isConnectOpen={openPanel === pid}
                        onConnectDismiss={() => setOpenPanel(null)}
                        testing={connId ? !!testing[connId] : false}
                        removing={connId ? !!removing[connId] : false}
                      />
                    )
                  })}
                </div>
              </div>
            ))}

            {/* Any extra categories not in the preferred order */}
            {Object.keys(groups)
              .filter(cat => !CATEGORY_ORDER.includes(cat))
              .map(cat => (
                <div key={cat}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 10 }}>
                    <div style={{ width: 16, height: 1, background: 'var(--cyan)', opacity: 0.7 }} />
                    <span style={{
                      fontFamily: 'var(--font-mono)',
                      fontSize: 10.5,
                      fontWeight: 600,
                      color: 'var(--text-muted)',
                      textTransform: 'uppercase',
                      letterSpacing: 2,
                    }}>
                      {CATEGORY_LABELS[cat] || cat}
                    </span>
                  </div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                    {groups[cat].map(platform => {
                      const pid = platform.ID || platform.id || ''
                      const conn = findConnection(connections, pid)
                      const connId = conn ? (conn.id || conn.ID) : null
                      return (
                        <PlatformRow
                          key={pid}
                          platform={platform}
                          connection={conn}
                          onTest={handleTest}
                          onRemove={handleRemove}
                          onConnectOpen={() => setOpenPanel(pid)}
                          isConnectOpen={openPanel === pid}
                          onConnectDismiss={() => setOpenPanel(null)}
                          testing={connId ? !!testing[connId] : false}
                          removing={connId ? !!removing[connId] : false}
                        />
                      )
                    })}
                  </div>
                </div>
              ))
            }
          </div>
        )}
      </div>
    </>
  )
}
