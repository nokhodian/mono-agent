import { useState, useEffect } from 'react'
import { GetVersion, CheckForUpdate, SelfUpdate } from '../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime'

export default function StatusBar({ stats, dbConnected }) {
  const running = stats?.actions_by_state?.RUNNING || 0
  const total   = stats?.total_actions || 0
  const people  = stats?.total_people || 0
  const sessions = stats?.active_sessions || 0

  const [ver, setVer] = useState(null)
  const [update, setUpdate] = useState(null)   // { checking, available, latest, error, updating, progress }

  useEffect(() => {
    GetVersion().then(v => setVer(v)).catch(() => {})
  }, [])

  // Listen for update progress events
  useEffect(() => {
    const off = EventsOn('update:progress', msg => {
      setUpdate(u => ({ ...u, progress: msg }))
    })
    return () => { if (typeof off === 'function') off(); else EventsOff('update:progress') }
  }, [])

  function handleVersionClick() {
    if (update?.updating) return
    setUpdate({ checking: true })
    CheckForUpdate()
      .then(info => {
        if (info.error) {
          setUpdate({ error: info.error })
        } else if (info.update_available) {
          setUpdate({ available: true, latest: info.latest_version, url: info.release_url })
        } else {
          setUpdate({ upToDate: true })
          setTimeout(() => setUpdate(null), 3000)
        }
      })
      .catch(e => setUpdate({ error: String(e) }))
  }

  function handleUpdate() {
    setUpdate(u => ({ ...u, updating: true, progress: 'Starting update...' }))
    SelfUpdate()
      .then(result => {
        if (result.success) {
          setUpdate({ done: true, latest: result.new_version })
        } else {
          setUpdate({ error: result.error })
        }
      })
      .catch(e => setUpdate({ error: String(e) }))
  }

  const versionText = ver ? `v${ver.version.replace(/^v/, '')}` : 'v…'

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

      {/* Version + update indicator */}
      <div style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 8, fontSize: 10 }}>
        {/* Update status popover */}
        {update && (
          <div style={{
            fontFamily: 'var(--font-mono)',
            fontSize: 10,
            display: 'flex',
            alignItems: 'center',
            gap: 6,
          }}>
            {update.checking && (
              <span style={{ color: 'var(--text-muted)' }}>Checking...</span>
            )}
            {update.upToDate && (
              <span style={{ color: '#00f5d4' }}>Up to date</span>
            )}
            {update.error && (
              <span style={{ color: 'var(--red)', maxWidth: 200, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                    title={update.error}>
                Update failed
              </span>
            )}
            {update.available && !update.updating && !update.done && (
              <>
                <span style={{ color: '#fbbf24' }}>{update.latest} available</span>
                <button
                  onClick={handleUpdate}
                  style={{
                    background: '#00b4d8',
                    color: '#fff',
                    border: 'none',
                    borderRadius: 3,
                    padding: '2px 8px',
                    cursor: 'pointer',
                    fontFamily: 'var(--font-mono)',
                    fontSize: 9,
                  }}
                >
                  Update
                </button>
                <button
                  onClick={() => setUpdate(null)}
                  style={{
                    background: 'transparent',
                    color: 'var(--text-muted)',
                    border: 'none',
                    cursor: 'pointer',
                    fontSize: 10,
                    padding: '0 2px',
                  }}
                >✕</button>
              </>
            )}
            {update.updating && (
              <span style={{ color: '#00b4d8' }}>{update.progress || 'Updating...'}</span>
            )}
            {update.done && (
              <span style={{ color: '#00f5d4' }}>Updated to {update.latest} — restart to apply</span>
            )}
          </div>
        )}

        <span
          onClick={handleVersionClick}
          title="Click to check for updates"
          style={{
            color: 'var(--text-dim)',
            cursor: 'pointer',
            userSelect: 'none',
            transition: 'color .15s',
          }}
          onMouseEnter={e => e.currentTarget.style.color = '#00b4d8'}
          onMouseLeave={e => e.currentTarget.style.color = 'var(--text-dim)'}
        >
          Mono Agent · {versionText}
        </span>
      </div>
    </div>
  )
}
