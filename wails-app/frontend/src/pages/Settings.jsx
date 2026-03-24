import { useState, useEffect, useCallback } from 'react'
import { Copy, CheckCircle, ExternalLink, ChevronDown, ChevronRight } from 'lucide-react'
import { api } from '../services/api.js'

const OAUTH_PLATFORMS = [
  { id: 'gmail.oauth', name: 'Gmail', emoji: '📧', docsUrl: 'https://console.cloud.google.com/apis/credentials' },
  { id: 'slack.oauth', name: 'Slack', emoji: '💬', docsUrl: 'https://api.slack.com/apps' },
  { id: 'github.oauth', name: 'GitHub', emoji: '🐙', docsUrl: 'https://github.com/settings/developers' },
  { id: 'notion.oauth', name: 'Notion', emoji: '📝', docsUrl: 'https://www.notion.so/my-integrations' },
  { id: 'airtable.oauth', name: 'Airtable', emoji: '📊', docsUrl: 'https://airtable.com/create/oauth' },
  { id: 'jira.oauth', name: 'Jira', emoji: '🎫', docsUrl: 'https://developer.atlassian.com/console/myapps/' },
  { id: 'linear.oauth', name: 'Linear', emoji: '📐', docsUrl: 'https://linear.app/settings/api' },
  { id: 'asana.oauth', name: 'Asana', emoji: '✅', docsUrl: 'https://app.asana.com/0/developer-console' },
  { id: 'hubspot.oauth', name: 'HubSpot', emoji: '🧲', docsUrl: 'https://developers.hubspot.com/docs/api/private-apps' },
  { id: 'salesforce.oauth', name: 'Salesforce', emoji: '☁️', docsUrl: 'https://login.salesforce.com/lightning/setup/NavigationMenus/home' },
]

const CALLBACK_URL = 'http://localhost:9876/callback'

const TABS = [
  { id: 'oauth', label: 'OAuth Credentials' },
  { id: 'general', label: 'General' },
]

// ── OAuthCard ────────────────────────────────────────────────────────────────

function OAuthCard({ platform, configured, onSaved }) {
  const [expanded, setExpanded] = useState(false)
  const [clientID, setClientID] = useState('')
  const [clientSecret, setClientSecret] = useState('')
  const [saving, setSaving] = useState(false)
  const [saveOk, setSaveOk] = useState(false)
  const [saveErr, setSaveErr] = useState(null)
  const [copied, setCopied] = useState(false)
  const [loaded, setLoaded] = useState(false)

  // Load credentials when expanded
  useEffect(() => {
    if (!expanded || loaded) return
    api.getOAuthCredentials(platform.id).then(json => {
      if (json) {
        try {
          const c = JSON.parse(json)
          setClientID(c.clientID || c.client_id || '')
          setClientSecret(c.clientSecret || c.client_secret || '')
        } catch { /* ignore parse errors */ }
      }
      setLoaded(true)
    })
  }, [expanded, loaded, platform.id])

  const save = async () => {
    setSaving(true); setSaveErr(null); setSaveOk(false)
    try {
      const r = await api.setOAuthCredentials(platform.id, clientID, clientSecret)
      if (r === 'ok') {
        setSaveOk(true)
        onSaved(platform.id, !!(clientID && clientSecret))
        setTimeout(() => setSaveOk(false), 2000)
      } else {
        setSaveErr(typeof r === 'string' ? r.replace('error:', '').trim() : 'Failed to save')
      }
    } catch (e) {
      setSaveErr(e.message || 'Failed to save')
    } finally {
      setSaving(false)
    }
  }

  const copyCallback = () => {
    navigator.clipboard.writeText(CALLBACK_URL)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  const Arrow = expanded ? ChevronDown : ChevronRight

  return (
    <div style={{
      background: 'var(--surface)',
      border: configured ? '1px solid var(--border-active)' : '1px solid var(--border)',
      borderRadius: 'var(--radius-lg)',
      overflow: 'hidden',
      transition: 'all var(--transition)',
    }}>
      {/* Header row */}
      <div
        onClick={() => setExpanded(!expanded)}
        style={{
          display: 'flex', alignItems: 'center', gap: 12,
          padding: '12px 16px',
          cursor: 'pointer',
          userSelect: 'none',
        }}
      >
        <span style={{ fontSize: 20, lineHeight: 1 }}>{platform.emoji}</span>
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, fontWeight: 600, color: 'var(--text)', flex: 1 }}>
          {platform.name}
        </span>
        <span style={{
          width: 7, height: 7, borderRadius: '50%',
          background: configured ? 'var(--green-neon)' : 'var(--text-muted)',
          boxShadow: configured ? '0 0 5px var(--green-neon)' : 'none',
          flexShrink: 0,
        }} />
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: configured ? 'var(--green-neon)' : 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: 1, minWidth: 70 }}>
          {configured ? 'Configured' : 'Not Set'}
        </span>
        <Arrow size={14} style={{ color: 'var(--text-muted)', flexShrink: 0 }} />
      </div>

      {/* Expanded form */}
      {expanded && (
        <div style={{
          padding: '0 16px 16px',
          display: 'flex', flexDirection: 'column', gap: 10,
          borderTop: '1px solid var(--border)',
          paddingTop: 14,
        }}>
          <div>
            <label style={{ display: 'block', fontFamily: 'var(--font-mono)', fontSize: 9.5, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: 1.5, marginBottom: 4 }}>
              Client ID
            </label>
            <input
              type="text"
              value={clientID}
              onChange={e => setClientID(e.target.value)}
              placeholder="Enter client ID"
              style={{ width: '100%', fontFamily: 'var(--font-mono)', fontSize: 11, background: 'var(--elevated)', border: '1px solid var(--border-bright)', borderRadius: 'var(--radius)', color: 'var(--text)', padding: '7px 10px', outline: 'none', boxSizing: 'border-box' }}
            />
          </div>

          <div>
            <label style={{ display: 'block', fontFamily: 'var(--font-mono)', fontSize: 9.5, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: 1.5, marginBottom: 4 }}>
              Client Secret
            </label>
            <input
              type="password"
              value={clientSecret}
              onChange={e => setClientSecret(e.target.value)}
              placeholder="Enter client secret"
              autoComplete="new-password"
              style={{ width: '100%', fontFamily: 'var(--font-mono)', fontSize: 11, background: 'var(--elevated)', border: '1px solid var(--border-bright)', borderRadius: 'var(--radius)', color: 'var(--text)', padding: '7px 10px', outline: 'none', boxSizing: 'border-box' }}
            />
          </div>

          {/* Callback URL */}
          <div>
            <label style={{ display: 'block', fontFamily: 'var(--font-mono)', fontSize: 9.5, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: 1.5, marginBottom: 4 }}>
              Callback URL
            </label>
            <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
              <input
                type="text"
                value={CALLBACK_URL}
                readOnly
                style={{ flex: 1, fontFamily: 'var(--font-mono)', fontSize: 11, background: 'var(--elevated)', border: '1px solid var(--border)', borderRadius: 'var(--radius)', color: 'var(--text-secondary)', padding: '7px 10px', outline: 'none', boxSizing: 'border-box', cursor: 'default' }}
              />
              <button
                onClick={copyCallback}
                style={{
                  display: 'flex', alignItems: 'center', gap: 4,
                  background: 'var(--elevated)', border: '1px solid var(--border-bright)',
                  borderRadius: 'var(--radius)', color: copied ? 'var(--green-neon)' : 'var(--text-muted)',
                  padding: '6px 10px', cursor: 'pointer',
                  fontFamily: 'var(--font-mono)', fontSize: 10,
                  transition: 'all var(--transition)',
                }}
              >
                {copied ? <CheckCircle size={11} /> : <Copy size={11} />}
                {copied ? 'Copied' : 'Copy'}
              </button>
            </div>
          </div>

          {/* Help link */}
          <button
            onClick={() => api.openURL(platform.docsUrl)}
            style={{ display: 'inline-flex', alignItems: 'center', gap: 4, background: 'none', border: 'none', cursor: 'pointer', color: 'var(--cyan)', fontFamily: 'var(--font-mono)', fontSize: 10, padding: 0, alignSelf: 'flex-start' }}
          >
            <ExternalLink size={11} /> How to get these credentials
          </button>

          {/* Save + feedback */}
          {saveErr && (
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: 11, padding: '7px 12px', borderRadius: 'var(--radius)', background: 'rgba(239,68,68,.08)', border: '1px solid rgba(239,68,68,.25)', color: 'var(--red)' }}>
              {saveErr}
            </div>
          )}

          <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
            <button
              className="btn btn-primary btn-sm"
              disabled={saving || !clientID || !clientSecret}
              onClick={save}
              style={{ gap: 5 }}
            >
              {saving ? 'Saving...' : <><CheckCircle size={11} /> Save</>}
            </button>
            {saveOk && <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--green-neon)' }}>Saved ✓</span>}
          </div>
        </div>
      )}
    </div>
  )
}

// ── Main ─────────────────────────────────────────────────────────────────────

export default function Settings() {
  const [activeTab, setActiveTab] = useState('oauth')
  const [credStatus, setCredStatus] = useState({}) // { platformID: boolean }
  const [loadingCreds, setLoadingCreds] = useState(true)
  const [dbPath, setDbPath] = useState('')
  const [dbConnected, setDbConnected] = useState(false)

  // Load OAuth credential status for all platforms on mount
  useEffect(() => {
    const load = async () => {
      setLoadingCreds(true)
      const status = {}
      await Promise.all(
        OAUTH_PLATFORMS.map(async (p) => {
          try {
            const json = await api.getOAuthCredentials(p.id)
            if (json) {
              const c = JSON.parse(json)
              status[p.id] = !!(c.clientID || c.client_id) && !!(c.clientSecret || c.client_secret)
            } else {
              status[p.id] = false
            }
          } catch {
            status[p.id] = false
          }
        })
      )
      setCredStatus(status)
      setLoadingCreds(false)
    }
    load()
  }, [])

  // Load general info
  useEffect(() => {
    api.getDBPath().then(p => setDbPath(p || ''))
    api.isDBConnected().then(c => setDbConnected(!!c))
  }, [])

  const handleCredSaved = useCallback((platformID, isConfigured) => {
    setCredStatus(prev => ({ ...prev, [platformID]: isConfigured }))
  }, [])

  const configuredCount = Object.values(credStatus).filter(Boolean).length

  return (
    <>
      <div className="page-header">
        <div className="page-header-left">
          <div className="page-title">Settings</div>
          <div className="page-subtitle">Application configuration</div>
        </div>
      </div>

      <div className="page-body">
        {/* Tab bar */}
        <div style={{
          display: 'flex', gap: 0,
          borderBottom: '1px solid var(--border)',
          marginBottom: 20,
        }}>
          {TABS.map(tab => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              style={{
                background: 'none',
                border: 'none',
                borderBottom: activeTab === tab.id ? '2px solid var(--cyan)' : '2px solid transparent',
                padding: '10px 20px',
                fontFamily: 'var(--font-mono)',
                fontSize: 11,
                fontWeight: 600,
                color: activeTab === tab.id ? 'var(--text)' : 'var(--text-muted)',
                cursor: 'pointer',
                transition: 'all var(--transition)',
                textTransform: 'uppercase',
                letterSpacing: 1,
              }}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {/* Tab content */}
        {activeTab === 'oauth' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 20, paddingBottom: 24 }}>
            {/* Section header */}
            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, fontWeight: 700, color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: 2 }}>
                OAuth Platforms
              </span>
              <div style={{ flex: 1, height: 1, background: 'var(--border)' }} />
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9.5, color: 'var(--text-muted)' }}>
                {loadingCreds ? '...' : `${configuredCount}/${OAUTH_PLATFORMS.length} configured`}
              </span>
            </div>

            {loadingCreds ? (
              <div className="empty-state"><div className="spinner" /></div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                {OAUTH_PLATFORMS.map(platform => (
                  <OAuthCard
                    key={platform.id}
                    platform={platform}
                    configured={!!credStatus[platform.id]}
                    onSaved={handleCredSaved}
                  />
                ))}
              </div>
            )}
          </div>
        )}

        {activeTab === 'general' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 20, paddingBottom: 24 }}>
            {/* Section header */}
            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, fontWeight: 700, color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: 2 }}>
                Application Info
              </span>
              <div style={{ flex: 1, height: 1, background: 'var(--border)' }} />
            </div>

            {/* Info card */}
            <div style={{
              background: 'var(--surface)',
              border: '1px solid var(--border)',
              borderRadius: 'var(--radius-lg)',
              padding: '16px 20px',
              display: 'flex', flexDirection: 'column', gap: 14,
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

              {/* App version */}
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: 1 }}>
                  Version
                </span>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-secondary)' }}>
                  Monoes Agent v1.0
                </span>
              </div>
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
              Settings are stored in the SQLite database at the path above.
            </div>
          </div>
        )}
      </div>
    </>
  )
}
