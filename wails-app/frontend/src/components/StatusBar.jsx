export default function StatusBar({ stats, dbConnected }) {
  const running = stats?.actions_by_state?.RUNNING || 0
  const total   = stats?.total_actions || 0
  const people  = stats?.total_people || 0
  const sessions = stats?.active_sessions || 0

  return (
    <div className="status-bar">
      <div className="status-bar-item">
        <span className={`status-dot ${dbConnected ? 'connected' : 'disconnected'}`}
              style={{ width: 5, height: 5 }} />
        <span>{dbConnected ? 'Connected' : 'Offline'}</span>
      </div>
      <div className="status-bar-item">
        Actions: <span>{total}</span>
      </div>
      {running > 0 && (
        <div className="status-bar-item" style={{ color: 'var(--teal)' }}>
          <span className="live-dot" style={{ width: 5, height: 5 }} />
          <span style={{ color: 'inherit' }}>{running} running</span>
        </div>
      )}
      <div className="status-bar-item">
        People: <span>{people}</span>
      </div>
      <div className="status-bar-item">
        Sessions: <span>{sessions}</span>
      </div>
      <div style={{ marginLeft: 'auto', color: 'var(--text-dim)', fontSize: 10 }}>
        Monoes Agent UI · v1.0.0
      </div>
    </div>
  )
}
