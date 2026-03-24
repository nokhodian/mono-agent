import { useState, useEffect, useCallback } from 'react'
import { X, CheckCircle, Loader, Trash2, RefreshCw, ExternalLink, Search, Brain, MessageSquare } from 'lucide-react'
import { api } from '../services/api.js'
import AIChatPanel from '../components/AIChatPanel.jsx'

const CATEGORY_ORDER = ['frontier', 'cloud', 'inference', 'gateway']
const CATEGORY_LABELS = { frontier: 'Frontier', cloud: 'Cloud', inference: 'Inference', gateway: 'Gateway' }

function fmtDate(s) {
  if (!s) return '—'
  try { return new Date(s).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' }) }
  catch { return s.slice(0, 16) }
}

function statusColor(status) {
  if (status === 'active') return 'var(--green-neon)'
  if (status === 'error') return 'var(--red)'
  return 'var(--text-muted)'
}

function statusGlow(status) {
  if (status === 'active') return '0 0 5px var(--green-neon)'
  return 'none'
}

// ── Tile ──────────────────────────────────────────────────────────────────────

function Tile({ regProvider, connProvider, onClick }) {
  const [hov, setHov] = useState(false)
  const connected = !!connProvider
  return (
    <div
      onClick={onClick}
      onMouseEnter={() => setHov(true)}
      onMouseLeave={() => setHov(false)}
      style={{
        background: connected ? 'linear-gradient(145deg,var(--elevated),var(--surface))' : 'var(--surface)',
        border: connected ? '1px solid var(--border-active)' : hov ? '1px solid var(--border-bright)' : '1px solid var(--border)',
        borderRadius: 'var(--radius-lg)',
        padding: '18px 10px 12px',
        cursor: 'pointer',
        display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 6,
        transition: 'all var(--transition)',
        boxShadow: connected ? 'var(--shadow-glow)' : 'none',
        userSelect: 'none',
        minWidth: 0,
      }}
    >
      <span style={{ fontSize: 24, lineHeight: 1, filter: connected ? 'none' : 'grayscale(40%) opacity(0.7)' }}>
        {regProvider.icon_emoji || '🤖'}
      </span>
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, fontWeight: 600, color: connected ? 'var(--text)' : 'var(--text-secondary)', textAlign: 'center', lineHeight: 1.3 }}>
        {regProvider.name}
      </span>
      <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
        <span style={{ width: 6, height: 6, borderRadius: '50%', background: connected ? 'var(--green-neon)' : 'var(--text-muted)', boxShadow: connected ? '0 0 5px var(--green-neon)' : 'none', flexShrink: 0 }} />
        {connected && <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', maxWidth: 72, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{connProvider.name}</span>}
      </div>
    </div>
  )
}

// ── Add Provider Modal ────────────────────────────────────────────────────────

function AddModal({ onClose, onSaved, registry, preselected }) {
  const [step, setStep] = useState(preselected ? 2 : 1)
  const [search, setSearch] = useState('')
  const [selectedReg, setSelectedReg] = useState(preselected || null)
  const [form, setForm] = useState(preselected ? {
    name: preselected.name,
    api_key: '',
    base_url: preselected.default_base_url || '',
    default_model: (preselected.models && preselected.models.length > 0) ? preselected.models[0].id || preselected.models[0].name || '' : '',
    extra_headers: '',
  } : { name: '', api_key: '', base_url: '', default_model: '', extra_headers: '' })
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState(null)
  const [error, setError] = useState(null)

  const filtered = registry.filter(r =>
    r.name.toLowerCase().includes(search.toLowerCase()) ||
    (r.category || '').toLowerCase().includes(search.toLowerCase())
  )

  const groups = {}
  for (const r of filtered) {
    const cat = (r.tier || r.category || 'other').toLowerCase()
    if (!groups[cat]) groups[cat] = []
    groups[cat].push(r)
  }

  const pickProvider = (reg) => {
    setSelectedReg(reg)
    setForm({
      name: reg.name,
      api_key: '',
      base_url: reg.default_base_url || '',
      default_model: (reg.models && reg.models.length > 0) ? reg.models[0].id || reg.models[0].name || '' : '',
      extra_headers: '',
    })
    setStep(2)
  }

  const saveAndTest = async () => {
    setSaving(true); setError(null); setTestResult(null)
    try {
      const payload = {
        name: form.name,
        provider_id: selectedReg.id,
        tier: selectedReg.tier || '',
        api_key: form.api_key,
        base_url: form.base_url,
        default_model: form.default_model,
        extra_headers: form.extra_headers,
      }
      const saved = await api.saveAIProvider(payload)
      if (!saved || saved.error) { setError(saved?.error || 'Failed to save'); setSaving(false); return }
      setSaving(false); setTesting(true)
      const result = await api.testAIProvider(saved.id)
      setTestResult(result)
      setTesting(false)
      setStep(3)
      onSaved()
    } catch (e) {
      setError(e.message || 'Failed to save'); setSaving(false); setTesting(false)
    }
  }

  const saveWithoutTest = async () => {
    setSaving(true); setError(null)
    try {
      const payload = {
        name: form.name,
        provider_id: selectedReg.id,
        tier: selectedReg.tier || '',
        api_key: form.api_key,
        base_url: form.base_url,
        default_model: form.default_model,
        extra_headers: form.extra_headers,
      }
      const saved = await api.saveAIProvider(payload)
      if (!saved || saved.error) { setError(saved?.error || 'Failed to save'); setSaving(false); return }
      setSaving(false)
      onSaved()
      onClose()
    } catch (e) {
      setError(e.message || 'Failed to save'); setSaving(false)
    }
  }

  return (
    <div
      onClick={e => e.target === e.currentTarget && onClose()}
      style={{ position: 'fixed', inset: 0, background: 'var(--overlay)', zIndex: 1000, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 24 }}
    >
      <div style={{ width: '100%', maxWidth: 500, maxHeight: '80vh', background: 'var(--elevated)', border: '1px solid var(--border-bright)', borderRadius: 'var(--radius-xl)', boxShadow: 'var(--shadow-glow)', overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>

        {/* Header */}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '16px 20px 12px', borderBottom: '1px solid var(--border)', flexShrink: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <span style={{ fontSize: 26 }}>{step === 1 ? '🤖' : (selectedReg?.icon_emoji || '🤖')}</span>
            <div>
              <div style={{ fontFamily: 'var(--font-mono)', fontSize: 13, fontWeight: 700, color: 'var(--text)' }}>
                {step === 1 ? 'Add AI Provider' : step === 2 ? `Configure ${form.name}` : 'Connection Result'}
              </div>
              <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9.5, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: 1, marginTop: 1 }}>
                Step {step} of 3
              </div>
            </div>
          </div>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)', padding: 4, display: 'flex' }}>
            <X size={16} />
          </button>
        </div>

        {/* Body */}
        <div style={{ padding: '16px 20px 20px', display: 'flex', flexDirection: 'column', gap: 14, overflowY: 'auto', flex: 1 }}>

          {step === 1 && (
            <>
              <div style={{ position: 'relative' }}>
                <Search size={13} style={{ position: 'absolute', left: 10, top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)' }} />
                <input
                  type="text"
                  placeholder="Search providers…"
                  value={search}
                  onChange={e => setSearch(e.target.value)}
                  style={{ width: '100%', fontFamily: 'var(--font-mono)', fontSize: 11, background: 'var(--surface)', border: '1px solid var(--border-bright)', borderRadius: 'var(--radius)', color: 'var(--text)', padding: '8px 10px 8px 30px', outline: 'none', boxSizing: 'border-box' }}
                />
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 20 }}>
                {[...CATEGORY_ORDER, ...Object.keys(groups).filter(c => !CATEGORY_ORDER.includes(c))]
                  .filter(cat => groups[cat] && groups[cat].length > 0)
                  .map(cat => (
                    <div key={cat}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 10 }}>
                        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, fontWeight: 700, color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: 2 }}>
                          {CATEGORY_LABELS[cat] || cat}
                        </span>
                        <div style={{ flex: 1, height: 1, background: 'var(--border)' }} />
                      </div>
                      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(100px, 1fr))', gap: 8 }}>
                        {groups[cat].map(r => (
                          <div
                            key={r.id}
                            onClick={() => pickProvider(r)}
                            style={{
                              background: 'var(--surface)',
                              border: '1px solid var(--border)',
                              borderRadius: 'var(--radius-lg)',
                              padding: '14px 8px 10px',
                              cursor: 'pointer',
                              display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 5,
                              transition: 'all var(--transition)',
                            }}
                            onMouseEnter={e => { e.currentTarget.style.borderColor = 'var(--border-bright)'; e.currentTarget.style.background = 'var(--elevated)' }}
                            onMouseLeave={e => { e.currentTarget.style.borderColor = 'var(--border)'; e.currentTarget.style.background = 'var(--surface)' }}
                          >
                            <span style={{ fontSize: 22, lineHeight: 1 }}>{r.icon_emoji || '🤖'}</span>
                            <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, fontWeight: 600, color: 'var(--text)', textAlign: 'center', lineHeight: 1.3 }}>{r.name}</span>
                          </div>
                        ))}
                      </div>
                    </div>
                  ))}
                {filtered.length === 0 && (
                  <div style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)', textAlign: 'center', padding: 20 }}>No providers found.</div>
                )}
              </div>
            </>
          )}

          {step === 2 && (
            <>
              <div>
                <label style={{ display: 'block', fontFamily: 'var(--font-mono)', fontSize: 9.5, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: 1.5, marginBottom: 4 }}>Name</label>
                <input
                  type="text"
                  value={form.name}
                  onChange={e => setForm(p => ({ ...p, name: e.target.value }))}
                  style={{ width: '100%', fontFamily: 'var(--font-mono)', fontSize: 11, background: 'var(--surface)', border: '1px solid var(--border-bright)', borderRadius: 'var(--radius)', color: 'var(--text)', padding: '7px 10px', outline: 'none', boxSizing: 'border-box' }}
                />
              </div>

              <div>
                <label style={{ display: 'block', fontFamily: 'var(--font-mono)', fontSize: 9.5, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: 1.5, marginBottom: 4 }}>
                  API Key <span style={{ color: 'var(--red)' }}>*</span>
                </label>
                <input
                  type="password"
                  value={form.api_key}
                  onChange={e => setForm(p => ({ ...p, api_key: e.target.value }))}
                  autoComplete="new-password"
                  placeholder={selectedReg?.auth_label || 'Enter API key'}
                  style={{ width: '100%', fontFamily: 'var(--font-mono)', fontSize: 11, background: 'var(--surface)', border: '1px solid var(--border-bright)', borderRadius: 'var(--radius)', color: 'var(--text)', padding: '7px 10px', outline: 'none', boxSizing: 'border-box' }}
                />
              </div>

              <div>
                <label style={{ display: 'block', fontFamily: 'var(--font-mono)', fontSize: 9.5, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: 1.5, marginBottom: 4 }}>
                  Base URL {selectedReg?.tier === 'gateway' && <span style={{ color: 'var(--cyan)', fontSize: 8, marginLeft: 4 }}>REQUIRED FOR GATEWAY</span>}
                </label>
                <input
                  type="text"
                  value={form.base_url}
                  onChange={e => setForm(p => ({ ...p, base_url: e.target.value }))}
                  placeholder="https://api.example.com/v1"
                  style={{ width: '100%', fontFamily: 'var(--font-mono)', fontSize: 11, background: 'var(--surface)', border: selectedReg?.tier === 'gateway' ? '1px solid var(--cyan)' : '1px solid var(--border-bright)', borderRadius: 'var(--radius)', color: 'var(--text)', padding: '7px 10px', outline: 'none', boxSizing: 'border-box' }}
                />
              </div>

              <div>
                <label style={{ display: 'block', fontFamily: 'var(--font-mono)', fontSize: 9.5, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: 1.5, marginBottom: 4 }}>Default Model</label>
                <input
                  list="models-list"
                  value={form.default_model}
                  onChange={e => setForm(p => ({ ...p, default_model: e.target.value }))}
                  placeholder="Type or select a model"
                  style={{ width: '100%', fontFamily: 'var(--font-mono)', fontSize: 11, background: 'var(--surface)', border: '1px solid var(--border-bright)', borderRadius: 'var(--radius)', color: 'var(--text)', padding: '7px 10px', outline: 'none', boxSizing: 'border-box' }}
                />
                <datalist id="models-list">
                  {(selectedReg?.models || []).map(m => (
                    <option key={m.id || m.name} value={m.id || m.name}>{m.name || m.id}</option>
                  ))}
                </datalist>
              </div>

              {selectedReg?.docs_url && (
                <button
                  onClick={() => api.openURL(selectedReg.docs_url)}
                  style={{ display: 'inline-flex', alignItems: 'center', gap: 4, background: 'none', border: 'none', cursor: 'pointer', color: 'var(--cyan)', fontFamily: 'var(--font-mono)', fontSize: 10, padding: 0, alignSelf: 'flex-start' }}
                >
                  <ExternalLink size={11} /> View documentation
                </button>
              )}

              {error && <div style={{ fontFamily: 'var(--font-mono)', fontSize: 11, padding: '7px 12px', borderRadius: 'var(--radius)', background: 'rgba(239,68,68,.08)', border: '1px solid rgba(239,68,68,.25)', color: 'var(--red)' }}>{error}</div>}

              <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'center' }}>
                <button className="btn btn-primary btn-sm" onClick={saveAndTest} disabled={saving || testing || !form.api_key} style={{ gap: 5 }}>
                  {(saving || testing) ? <Loader size={11} style={{ animation: 'spin .7s linear infinite' }} /> : <CheckCircle size={11} />}
                  {saving ? 'Saving…' : testing ? 'Testing…' : 'Save & Test'}
                </button>
                <button className="btn btn-ghost btn-sm" onClick={saveWithoutTest} disabled={saving || testing || !form.api_key} style={{ gap: 5 }}>
                  Save without testing
                </button>
                <button className="btn btn-ghost btn-sm" onClick={() => { setStep(1); setError(null); setTestResult(null) }}>Back</button>
              </div>
            </>
          )}

          {step === 3 && (
            <>
              {testResult && testResult.status === 'active' ? (
                <div style={{ fontFamily: 'var(--font-mono)', fontSize: 11, padding: '10px 14px', borderRadius: 'var(--radius)', background: 'rgba(74,222,128,.08)', border: '1px solid rgba(74,222,128,.25)', color: 'var(--green-neon)', display: 'flex', alignItems: 'center', gap: 8 }}>
                  <CheckCircle size={14} />
                  Connection successful! Provider is active.
                </div>
              ) : (
                <div style={{ fontFamily: 'var(--font-mono)', fontSize: 11, padding: '10px 14px', borderRadius: 'var(--radius)', background: 'rgba(239,68,68,.08)', border: '1px solid rgba(239,68,68,.25)', color: 'var(--red)' }}>
                  ✗ Test failed{testResult?.error ? `: ${testResult.error}` : ''}
                </div>
              )}
              <div style={{ display: 'flex', gap: 8 }}>
                <button className="btn btn-primary btn-sm" onClick={onClose} style={{ gap: 5 }}>Done</button>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  )
}

// ── Manage Modal ──────────────────────────────────────────────────────────────

function ManageModal({ provider, onClose, onRefresh, onDeleted, registry }) {
  const [form, setForm] = useState({
    name: provider.name || '',
    api_key: provider.api_key || '',
    base_url: provider.base_url || '',
    default_model: provider.default_model || '',
    extra_headers: provider.extra_headers || '',
  })
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState(null)
  const [error, setError] = useState(null)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [deleting, setDeleting] = useState(false)

  const reg = registry.find(r => r.id === provider.provider_id)

  const save = async () => {
    setSaving(true); setError(null)
    try {
      const payload = { ...provider, ...form }
      const saved = await api.saveAIProvider(payload)
      if (!saved || saved.error) { setError(saved?.error || 'Failed to save'); setSaving(false); return }
      setSaving(false)
      onRefresh()
    } catch (e) {
      setError(e.message || 'Failed to save'); setSaving(false)
    }
  }

  const test = async () => {
    setTesting(true); setTestResult(null)
    try {
      const result = await api.testAIProvider(provider.id)
      setTestResult(result)
      onRefresh()
    } finally { setTesting(false) }
  }

  const remove = async () => {
    setDeleting(true)
    try {
      await api.deleteAIProvider(provider.id)
      onDeleted()
    } finally { setDeleting(false) }
  }

  return (
    <div
      onClick={e => e.target === e.currentTarget && onClose()}
      style={{ position: 'fixed', inset: 0, background: 'var(--overlay)', zIndex: 1000, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 24 }}
    >
      <div style={{ width: '100%', maxWidth: 440, background: 'var(--elevated)', border: '1px solid var(--border-bright)', borderRadius: 'var(--radius-xl)', boxShadow: 'var(--shadow-glow)', overflow: 'hidden' }}>

        {/* Header */}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '16px 20px 12px', borderBottom: '1px solid var(--border)' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <span style={{ fontSize: 26 }}>{provider.icon_emoji || reg?.icon_emoji || '🤖'}</span>
            <div>
              <div style={{ fontFamily: 'var(--font-mono)', fontSize: 13, fontWeight: 700, color: 'var(--text)' }}>{provider.name}</div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 5, marginTop: 2 }}>
                <span style={{ width: 6, height: 6, borderRadius: '50%', background: statusColor(provider.status), boxShadow: statusGlow(provider.status) }} />
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: statusColor(provider.status), textTransform: 'uppercase', letterSpacing: 1 }}>
                  {provider.status || 'untested'}
                </span>
              </div>
            </div>
          </div>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)', padding: 4, display: 'flex' }}>
            <X size={16} />
          </button>
        </div>

        {/* Body */}
        <div style={{ padding: '16px 20px 20px', display: 'flex', flexDirection: 'column', gap: 14 }}>

          {/* Info row */}
          <div style={{ background: 'var(--surface)', border: '1px solid var(--border)', borderRadius: 'var(--radius-lg)', padding: '10px 14px', display: 'flex', flexDirection: 'column', gap: 8 }}>
            {[['Provider', provider.provider_id], ['Status', provider.status || 'untested'], ['Last Tested', fmtDate(provider.last_tested)]].map(([lbl, val]) => (
              <div key={lbl} style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: 1 }}>{lbl}</span>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: lbl === 'Status' ? statusColor(val) : 'var(--text-secondary)' }}>{val}</span>
              </div>
            ))}
          </div>

          {/* Editable fields */}
          <div>
            <label style={{ display: 'block', fontFamily: 'var(--font-mono)', fontSize: 9.5, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: 1.5, marginBottom: 4 }}>Name</label>
            <input
              type="text"
              value={form.name}
              onChange={e => setForm(p => ({ ...p, name: e.target.value }))}
              style={{ width: '100%', fontFamily: 'var(--font-mono)', fontSize: 11, background: 'var(--surface)', border: '1px solid var(--border-bright)', borderRadius: 'var(--radius)', color: 'var(--text)', padding: '7px 10px', outline: 'none', boxSizing: 'border-box' }}
            />
          </div>

          <div>
            <label style={{ display: 'block', fontFamily: 'var(--font-mono)', fontSize: 9.5, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: 1.5, marginBottom: 4 }}>API Key</label>
            <input
              type="password"
              value={form.api_key}
              onChange={e => setForm(p => ({ ...p, api_key: e.target.value }))}
              autoComplete="new-password"
              style={{ width: '100%', fontFamily: 'var(--font-mono)', fontSize: 11, background: 'var(--surface)', border: '1px solid var(--border-bright)', borderRadius: 'var(--radius)', color: 'var(--text)', padding: '7px 10px', outline: 'none', boxSizing: 'border-box' }}
            />
          </div>

          <div>
            <label style={{ display: 'block', fontFamily: 'var(--font-mono)', fontSize: 9.5, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: 1.5, marginBottom: 4 }}>Base URL</label>
            <input
              type="text"
              value={form.base_url}
              onChange={e => setForm(p => ({ ...p, base_url: e.target.value }))}
              style={{ width: '100%', fontFamily: 'var(--font-mono)', fontSize: 11, background: 'var(--surface)', border: '1px solid var(--border-bright)', borderRadius: 'var(--radius)', color: 'var(--text)', padding: '7px 10px', outline: 'none', boxSizing: 'border-box' }}
            />
          </div>

          <div>
            <label style={{ display: 'block', fontFamily: 'var(--font-mono)', fontSize: 9.5, color: 'var(--text-muted)', textTransform: 'uppercase', letterSpacing: 1.5, marginBottom: 4 }}>Default Model</label>
            <input
              list="manage-models-list"
              value={form.default_model}
              onChange={e => setForm(p => ({ ...p, default_model: e.target.value }))}
              placeholder="Type or select a model"
              style={{ width: '100%', fontFamily: 'var(--font-mono)', fontSize: 11, background: 'var(--surface)', border: '1px solid var(--border-bright)', borderRadius: 'var(--radius)', color: 'var(--text)', padding: '7px 10px', outline: 'none', boxSizing: 'border-box' }}
            />
            <datalist id="manage-models-list">
              {(reg?.models || []).map(m => (
                <option key={m.id || m.name} value={m.id || m.name}>{m.name || m.id}</option>
              ))}
            </datalist>
          </div>

          {/* Test result */}
          {testResult && (
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: 11, padding: '7px 12px', borderRadius: 'var(--radius)', background: testResult.status === 'active' ? 'rgba(74,222,128,.08)' : 'rgba(239,68,68,.08)', border: `1px solid ${testResult.status === 'active' ? 'rgba(74,222,128,.25)' : 'rgba(239,68,68,.25)'}`, color: testResult.status === 'active' ? 'var(--green-neon)' : 'var(--red)' }}>
              {testResult.status === 'active' ? '✓ Connection OK' : `✗ Test failed${testResult.error ? `: ${testResult.error}` : ''}`}
            </div>
          )}

          {error && <div style={{ fontFamily: 'var(--font-mono)', fontSize: 11, padding: '7px 12px', borderRadius: 'var(--radius)', background: 'rgba(239,68,68,.08)', border: '1px solid rgba(239,68,68,.25)', color: 'var(--red)' }}>{error}</div>}

          {/* Actions */}
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            <button className="btn btn-primary btn-sm" onClick={save} disabled={saving} style={{ gap: 5 }}>
              {saving ? <Loader size={11} style={{ animation: 'spin .7s linear infinite' }} /> : <CheckCircle size={11} />}
              {saving ? 'Saving…' : 'Save'}
            </button>
            <button className="btn btn-secondary btn-sm" onClick={test} disabled={testing} style={{ gap: 5 }}>
              {testing ? <Loader size={11} style={{ animation: 'spin .7s linear infinite' }} /> : <RefreshCw size={11} />}
              {testing ? 'Testing…' : 'Test Connection'}
            </button>
            {!confirmDelete ? (
              <button className="btn btn-danger btn-sm" onClick={() => setConfirmDelete(true)} style={{ gap: 5 }}>
                <Trash2 size={11} /> Delete
              </button>
            ) : (
              <button className="btn btn-danger btn-sm" onClick={remove} disabled={deleting} style={{ gap: 5 }}>
                {deleting ? <Loader size={11} style={{ animation: 'spin .7s linear infinite' }} /> : <Trash2 size={11} />}
                {deleting ? 'Deleting…' : 'Confirm Delete'}
              </button>
            )}
            <button className="btn btn-ghost btn-sm" onClick={onClose}>Close</button>
          </div>
        </div>
      </div>
    </div>
  )
}

// ── Main ──────────────────────────────────────────────────────────────────────

export default function AIProviders() {
  const [providers, setProviders] = useState([])
  const [registry, setRegistry]   = useState([])
  const [loading, setLoading]     = useState(true)
  const [showAdd, setShowAdd]     = useState(false)
  const [addPreselect, setAddPreselect] = useState(null)
  const [selected, setSelected]   = useState(null)
  const [chatOpen, setChatOpen]   = useState(false)

  const loadAll = useCallback(async (silent = false) => {
    if (!silent) setLoading(true)
    try {
      const [provs, reg] = await Promise.all([
        api.listAIProviders(),
        api.getAIRegistry(),
      ])
      setProviders(Array.isArray(provs) ? provs : [])
      setRegistry(Array.isArray(reg) ? reg : [])
    } finally {
      if (!silent) setLoading(false)
    }
  }, [])

  useEffect(() => { loadAll() }, [loadAll])

  const handleRefresh = useCallback(() => loadAll(true), [loadAll])

  const handleDeleted = useCallback(async () => {
    await loadAll(true)
    setSelected(null)
  }, [loadAll])

  // Build a map of provider_id → connected provider for quick lookup
  const connMap = {}
  for (const p of providers) {
    if (p.provider_id) connMap[p.provider_id] = p
  }

  // Group registry by category
  const groups = {}
  for (const r of registry) {
    const cat = (r.tier || r.category || 'other').toLowerCase()
    if (!groups[cat]) groups[cat] = []
    groups[cat].push(r)
  }

  const totalConnected = providers.length

  const handleTileClick = (regProv) => {
    const conn = connMap[regProv.id]
    if (conn) {
      setSelected(conn)
    } else {
      setAddPreselect(regProv)
      setShowAdd(true)
    }
  }

  const hasActiveProvider = providers.some(p => p.status === 'active')

  return (
    <div style={{ display: 'flex', height: '100%', overflow: 'hidden' }}>
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
        <div className="page-header">
          <div className="page-header-left">
            <div className="page-title">AI Providers</div>
            <div className="page-subtitle">{loading ? 'Loading…' : `${totalConnected} / ${registry.length} connected`}</div>
          </div>
          <div className="page-header-right" style={{ display: 'flex', gap: 6 }}>
            {hasActiveProvider && (
              <button
                className="btn btn-sm"
                onClick={() => setChatOpen(o => !o)}
                title="Chat with AI"
                style={{
                  gap: 5,
                  color: chatOpen ? '#00b4d8' : 'var(--text-muted)',
                  borderColor: chatOpen ? 'rgba(0,180,216,0.3)' : 'var(--border)',
                  background: chatOpen ? 'rgba(0,180,216,0.08)' : 'transparent',
                }}
              >
                <MessageSquare size={12} /> Chat
              </button>
            )}
            <button className="btn btn-ghost btn-sm" onClick={() => loadAll()} style={{ gap: 5 }}><RefreshCw size={12} /> Refresh</button>
          </div>
        </div>

        <div className="page-body" style={{ flex: 1, overflow: 'auto' }}>
          {loading ? (
            <div className="empty-state"><div className="spinner" /></div>
          ) : registry.length === 0 ? (
            <div className="empty-state">
              <div className="empty-state-icon"><Brain size={36} /></div>
              <div className="empty-state-title">No Providers Found</div>
              <div className="empty-state-desc">Could not load AI provider registry.</div>
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 28, paddingBottom: 24 }}>
              {[...CATEGORY_ORDER, ...Object.keys(groups).filter(c => !CATEGORY_ORDER.includes(c))]
                .filter(cat => groups[cat] && groups[cat].length > 0)
                .map(cat => {
                  const list = groups[cat]
                  const catConn = list.filter(r => connMap[r.id]).length
                  return (
                    <div key={cat}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 12 }}>
                        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, fontWeight: 700, color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: 2 }}>
                          {CATEGORY_LABELS[cat] || cat}
                        </span>
                        <div style={{ flex: 1, height: 1, background: 'var(--border)' }} />
                        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9.5, color: 'var(--text-muted)' }}>{catConn}/{list.length}</span>
                      </div>
                      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(100px, 1fr))', gap: 8 }}>
                        {list.map(r => (
                          <Tile
                            key={r.id}
                            regProvider={r}
                            connProvider={connMap[r.id] || null}
                            onClick={() => handleTileClick(r)}
                          />
                        ))}
                      </div>
                    </div>
                  )
                })}
            </div>
          )}
        </div>
      </div>

      <AIChatPanel
        workflowID="general"
        isOpen={chatOpen}
        onClose={() => setChatOpen(false)}
      />

      {showAdd && (
        <AddModal
          registry={registry}
          preselected={addPreselect}
          onClose={() => { setShowAdd(false); setAddPreselect(null) }}
          onSaved={handleRefresh}
        />
      )}

      {selected && (
        <ManageModal
          provider={selected}
          registry={registry}
          onClose={() => setSelected(null)}
          onRefresh={handleRefresh}
          onDeleted={handleDeleted}
        />
      )}
    </div>
  )
}
