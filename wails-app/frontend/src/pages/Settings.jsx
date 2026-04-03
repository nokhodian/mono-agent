import { useState, useEffect } from 'react'
import { Link2, Brain, ExternalLink } from 'lucide-react'
import { api } from '../services/api.js'
import { GetVersion, CheckForUpdate, SelfUpdate } from '../wailsjs/go/main/App'

// ── VersionRow ──────────────────────────────────────────────────────────────

function VersionRow() {
  const [ver, setVer] = useState(null)
  const [update, setUpdate] = useState(null)

  useEffect(() => { GetVersion().then(setVer).catch(() => {}) }, [])

  function check() {
    setUpdate({ checking: true })
    CheckForUpdate().then(info => {
      if (info.error) setUpdate({ error: info.error })
      else if (info.update_available) setUpdate({ available: true, latest: info.latest_version })
      else { setUpdate({ upToDate: true }); setTimeout(() => setUpdate(null), 3000) }
    }).catch(e => setUpdate({ error: String(e) }))
  }

  function doUpdate() {
    setUpdate(u => ({ ...u, updating: true }))
    SelfUpdate().then(r => {
      if (r.success) setUpdate({ done: true, latest: r.new_version })
      else setUpdate({ error: r.error })
    }).catch(e => setUpdate({ error: String(e) }))
  }

  const vText = ver ? `v${ver.version.replace(/^v/, '')}` : '...'

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: 1 }}>
          Version
        </span>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-secondary)' }}>
            Mono Agent {vText}
          </span>
          <button
            onClick={check}
            disabled={update?.checking || update?.updating}
            style={{
              background: 'rgba(0,180,216,.15)',
              color: '#00b4d8',
              border: '1px solid rgba(0,180,216,.2)',
              borderRadius: 4,
              padding: '2px 10px',
              cursor: 'pointer',
              fontFamily: 'var(--font-mono)',
              fontSize: 9,
              opacity: (update?.checking || update?.updating) ? 0.5 : 1,
            }}
          >
            {update?.checking ? 'Checking...' : 'Check for updates'}
          </button>
        </div>
      </div>
      {update?.upToDate && (
        <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: '#00f5d4', marginTop: 6 }}>
          You're on the latest version.
        </div>
      )}
      {update?.available && !update.updating && !update.done && (
        <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, marginTop: 6, display: 'flex', alignItems: 'center', gap: 8 }}>
          <span style={{ color: '#fbbf24' }}>{update.latest} is available</span>
          <button onClick={doUpdate} style={{
            background: '#00b4d8', color: '#fff', border: 'none', borderRadius: 4,
            padding: '3px 12px', cursor: 'pointer', fontFamily: 'var(--font-mono)', fontSize: 10,
          }}>Install Update</button>
        </div>
      )}
      {update?.updating && (
        <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: '#00b4d8', marginTop: 6 }}>
          Updating...
        </div>
      )}
      {update?.done && (
        <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: '#00f5d4', marginTop: 6 }}>
          Updated to {update.latest} — restart the app to apply.
        </div>
      )}
      {update?.error && (
        <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--red)', marginTop: 6 }}>
          {update.error}
        </div>
      )}
    </div>
  )
}

// ── QuickAccessCard ─────────────────────────────────────────────────────────

function QuickAccessCard({ icon: Icon, title, description, stats, onClick }) {
  const [hov, setHov] = useState(false)
  return (
    <div
      onClick={onClick}
      onMouseEnter={() => setHov(true)}
      onMouseLeave={() => setHov(false)}
      style={{
        flex: 1,
        background: hov ? 'var(--elevated)' : 'var(--surface)',
        border: hov ? '1px solid var(--border-bright)' : '1px solid var(--border)',
        borderRadius: 'var(--radius-lg)',
        padding: '20px 18px',
        cursor: 'pointer',
        transition: 'all var(--transition)',
        display: 'flex',
        flexDirection: 'column',
        gap: 10,
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
        <div style={{
          width: 34, height: 34, borderRadius: 'var(--radius)',
          background: 'rgba(0,180,216,.1)', border: '1px solid rgba(0,180,216,.15)',
          display: 'flex', alignItems: 'center', justifyContent: 'center',
        }}>
          <Icon size={16} style={{ color: '#00b4d8' }} />
        </div>
        <div>
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: 12, fontWeight: 600, color: 'var(--text)' }}>
            {title}
          </div>
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)', marginTop: 2 }}>
            {description}
          </div>
        </div>
      </div>
      {stats && (
        <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-dim)', paddingLeft: 44 }}>
          {stats}
        </div>
      )}
    </div>
  )
}

// ── Main ─────────────────────────────────────────────────────────────────────

export default function Settings({ onNavigate }) {
  const [dbPath, setDbPath] = useState('')
  const [dbConnected, setDbConnected] = useState(false)
  const [connCount, setConnCount] = useState(null)

  useEffect(() => {
    api.getDBPath().then(p => setDbPath(p || ''))
    api.isDBConnected().then(c => setDbConnected(!!c))
    // Count active connections
    api.listConnections('').then(conns => {
      if (Array.isArray(conns)) {
        const active = conns.filter(c => (c.Status || c.status) === 'active').length
        setConnCount(`${active} active connection${active !== 1 ? 's' : ''}`)
      }
    }).catch(() => {})
  }, [])

  return (
    <>
      <div className="page-header">
        <div className="page-header-left">
          <div className="page-title">Settings</div>
          <div className="page-subtitle">Application configuration and quick access</div>
        </div>
      </div>

      <div className="page-body">
        {/* Quick access cards */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 14 }}>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, fontWeight: 700, color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: 2 }}>
            Quick Access
          </span>
          <div style={{ flex: 1, height: 1, background: 'var(--border)' }} />
        </div>

        <div style={{ display: 'flex', gap: 12, marginBottom: 28 }}>
          <QuickAccessCard
            icon={Link2}
            title="Connections"
            description="Manage accounts, OAuth, API keys"
            stats={connCount}
            onClick={() => onNavigate?.('connections')}
          />
          <QuickAccessCard
            icon={Brain}
            title="AI Providers"
            description="Configure AI models and keys"
            onClick={() => onNavigate?.('ai')}
          />
        </div>

        {/* Application Info */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 14 }}>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, fontWeight: 700, color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: 2 }}>
            Application Info
          </span>
          <div style={{ flex: 1, height: 1, background: 'var(--border)' }} />
        </div>

        <div style={{
          background: 'var(--surface)',
          border: '1px solid var(--border)',
          borderRadius: 'var(--radius-lg)',
          padding: '16px 20px',
          display: 'flex', flexDirection: 'column', gap: 14,
          marginBottom: 16,
        }}>
          {/* DB path */}
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 16 }}>
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: 1, flexShrink: 0, paddingTop: 1 }}>
              Database Path
            </span>
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-secondary)', wordBreak: 'break-all', textAlign: 'right' }}>
              {dbPath || '—'}
            </span>
          </div>

          {/* DB status */}
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: 1 }}>
              Database Status
            </span>
            <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
              <span style={{
                width: 7, height: 7, borderRadius: '50%',
                background: dbConnected ? 'var(--green-neon)' : 'var(--red)',
                boxShadow: dbConnected ? '0 0 5px var(--green-neon)' : 'none',
              }} />
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: dbConnected ? 'var(--green-neon)' : 'var(--red)' }}>
                {dbConnected ? 'Connected' : 'Disconnected'}
              </span>
            </div>
          </div>

          {/* App version + update */}
          <VersionRow />
        </div>

        {/* Note */}
        <div style={{
          fontFamily: 'var(--font-body)',
          fontSize: 11,
          color: 'var(--text-muted)',
          lineHeight: 1.6,
          padding: '10px 14px',
          background: 'rgba(0,245,212,.04)',
          border: '1px solid rgba(0,245,212,.12)',
          borderRadius: 'var(--radius)',
        }}>
          All authentication is managed in <strong style={{ color: 'var(--text-secondary)', cursor: 'pointer' }} onClick={() => onNavigate?.('connections')}>Connections</strong>.
          OAuth credentials, API keys, and browser sessions are configured there.
        </div>
      </div>
    </>
  )
}
