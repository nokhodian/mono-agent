import { useState, useEffect, useCallback, useRef } from 'react'
import { Plus, Play, Trash2, Pause, RefreshCw, Target, Zap } from 'lucide-react'
import { api, PLATFORMS, STATES, onActionComplete } from '../services/api.js'
import ActionInputsForm from '../components/ActionInputsForm.jsx'

function ActionBadge({ state }) {
  return <span className={`badge badge-state-${state.toLowerCase()}`}>{state}</span>
}

function PlatformBadge({ platform }) {
  return <span className={`badge badge-platform-${(platform || '').toLowerCase()}`}>{platform}</span>
}

function CreateModal({ availableTypes, onClose, onCreated }) {
  const [form, setForm] = useState({
    title: '',
    type: '',
    platform: 'INSTAGRAM',
    keywords: '',
    content_message: '',
    params: {},
  })
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const types = availableTypes?.[form.platform] || []

  // Close on Escape
  useEffect(() => {
    const handler = (e) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [onClose])

  const submit = async (e) => {
    e.preventDefault()
    if (!form.title || !form.type) {
      setError('Title and type are required')
      return
    }
    setLoading(true)
    setError('')
    try {
      const created = await api.createAction(form)
      if (created) {
        onCreated(created)
        onClose()
      }
    } catch (err) {
      setError(String(err))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="modal-overlay" onClick={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal">
        <div className="modal-title">
          <span>New Action</span>
          <button className="btn btn-ghost btn-icon" onClick={onClose} style={{ fontSize: 18, color: 'var(--text-muted)' }}>×</button>
        </div>
        <form onSubmit={submit}>
          <div className="form-group">
            <label className="form-label">Title</label>
            <input
              className="form-input"
              placeholder="My Instagram Campaign"
              value={form.title}
              onChange={e => setForm(f => ({ ...f, title: e.target.value }))}
              autoFocus
            />
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
            <div className="form-group">
              <label className="form-label">Platform</label>
              <select
                className="form-select"
                value={form.platform}
                onChange={e => setForm(f => ({ ...f, platform: e.target.value, type: '' }))}
              >
                {PLATFORMS.map(p => <option key={p} value={p}>{p}</option>)}
              </select>
            </div>
            <div className="form-group">
              <label className="form-label">Action Type</label>
              <select
                className="form-select"
                value={form.type}
                onChange={e => setForm(f => ({ ...f, type: e.target.value }))}
              >
                <option value="">— Select —</option>
                {types.map(t => <option key={t} value={t}>{t}</option>)}
              </select>
            </div>
          </div>
          <div className="form-group">
            <label className="form-label">Keywords <span style={{ color: 'var(--text-dim)', fontSize: 10 }}>(optional)</span></label>
            <input
              className="form-input"
              placeholder="golang, tech, startup..."
              value={form.keywords}
              onChange={e => setForm(f => ({ ...f, keywords: e.target.value }))}
            />
          </div>
          <div className="form-group">
            <label className="form-label">Message <span style={{ color: 'var(--text-dim)', fontSize: 10 }}>(optional)</span></label>
            <textarea
              className="form-textarea"
              placeholder="Your message template..."
              value={form.content_message}
              onChange={e => setForm(f => ({ ...f, content_message: e.target.value }))}
            />
          </div>
          {form.type && (
            <ActionInputsForm
              platform={form.platform}
              actionType={form.type}
              params={form.params}
              onChange={params => setForm(f => ({ ...f, params }))}
            />
          )}
          {error && (
            <div style={{ color: 'var(--red)', fontSize: 12, fontFamily: 'var(--font-mono)', marginBottom: 12, padding: '8px 10px', background: 'rgba(239,68,68,0.08)', borderRadius: 'var(--radius)', border: '1px solid rgba(239,68,68,0.2)' }}>
              {error}
            </div>
          )}
          <div className="modal-actions">
            <button type="button" className="btn btn-secondary" onClick={onClose}>Cancel</button>
            <button type="submit" className="btn btn-primary" disabled={loading}>
              {loading ? <span className="spinner" style={{ width: 14, height: 14 }} /> : <Plus size={14} />}
              Create Action
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

export default function Actions({ onRefresh }) {
  const [actions, setActions] = useState([])
  const [loading, setLoading] = useState(true)
  const [filterPlatform, setFilterPlatform] = useState('')
  const [filterState, setFilterState] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [availableTypes, setAvailableTypes] = useState({})
  const [runningIds, setRunningIds] = useState(new Set())
  const [runError, setRunError] = useState('')

  const load = useCallback(async () => {
    const data = await api.getActions(filterPlatform, filterState, 0)
    setActions(data || [])
    setLoading(false)
  }, [filterPlatform, filterState])

  useEffect(() => {
    setLoading(true)
    load()
  }, [load])

  useEffect(() => {
    api.getAvailableActionTypes().then(t => t && setAvailableTypes(t))
  }, [])

  useEffect(() => {
    const off = onActionComplete(({ action_id }) => {
      setRunningIds(prev => {
        const next = new Set(prev)
        next.delete(action_id)
        return next
      })
      load()
    })
    return off
  }, [load])

  const handleRun = async (id) => {
    setRunError('')
    setRunningIds(prev => new Set([...prev, id]))
    try {
      await api.executeAction(id)
    } catch (err) {
      setRunningIds(prev => {
        const next = new Set(prev)
        next.delete(id)
        return next
      })
      setRunError('Failed to start: ' + err)
    }
  }

  const handleDelete = async (id) => {
    if (!confirm('Delete this action?')) return
    await api.deleteAction(id)
    setActions(prev => prev.filter(a => a.id !== id))
    onRefresh?.()
  }

  const handleStateChange = async (id, newState) => {
    await api.updateActionState(id, newState)
    setActions(prev => prev.map(a => a.id === id ? { ...a, state: newState } : a))
  }

  return (
    <>
      <div className="page-header">
        <div className="page-header-left">
          <div className="page-title">Actions</div>
          <div className="page-subtitle">Automation Tasks</div>
        </div>
        <div className="page-header-right">
          <button className="btn btn-ghost btn-sm" onClick={load} style={{ gap: 5 }}>
            <RefreshCw size={12} /> Refresh
          </button>
          <button className="btn btn-primary btn-sm" onClick={() => setShowCreate(true)}>
            <Plus size={13} /> New Action
          </button>
        </div>
      </div>

      <div className="page-body">
        <div className="filters-bar">
          <select
            className="filter-select"
            value={filterPlatform}
            onChange={e => setFilterPlatform(e.target.value)}
          >
            <option value="">All Platforms</option>
            {PLATFORMS.map(p => <option key={p} value={p}>{p}</option>)}
          </select>
          <select
            className="filter-select"
            value={filterState}
            onChange={e => setFilterState(e.target.value)}
          >
            <option value="">All States</option>
            {STATES.map(s => <option key={s} value={s}>{s}</option>)}
          </select>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)', marginLeft: 'auto' }}>
            {actions.length} action{actions.length !== 1 ? 's' : ''}
          </span>
        </div>

        {runError && (
          <div style={{ color: 'var(--red)', fontSize: 12, fontFamily: 'var(--font-mono)', marginBottom: 12, padding: '8px 10px', background: 'rgba(239,68,68,0.08)', borderRadius: 'var(--radius)', border: '1px solid rgba(239,68,68,0.2)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span>{runError}</span>
            <button onClick={() => setRunError('')} style={{ background: 'none', border: 'none', color: 'var(--red)', cursor: 'pointer', fontSize: 16, lineHeight: 1 }} aria-label="Dismiss error">×</button>
          </div>
        )}

        {loading ? (
          <div className="empty-state">
            <div className="spinner" />
          </div>
        ) : actions.length === 0 ? (
          <div className="empty-state">
            <div className="empty-state-icon"><Zap size={40} /></div>
            <div className="empty-state-title">No Actions</div>
            <div className="empty-state-desc">Create your first automation action to get started.</div>
            <button className="btn btn-primary" onClick={() => setShowCreate(true)} style={{ marginTop: 8 }}>
              <Plus size={14} /> New Action
            </button>
          </div>
        ) : (
          <div className="action-list">
            {actions.map(action => {
              const isRunning = runningIds.has(action.id) || action.state === 'RUNNING'
              const progress = action.target_count > 0
                ? Math.min(100, Math.round((action.reached_index / action.target_count) * 100))
                : 0

              return (
                <div key={action.id} className={`action-item ${isRunning ? 'running' : ''}`}>
                  <div style={{ minWidth: 0 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
                      {isRunning && <span className="live-dot" />}
                      <div className="action-title truncate">{action.title}</div>
                    </div>
                    <div className="action-meta">
                      <PlatformBadge platform={action.platform} />
                      <ActionBadge state={action.state} />
                      <span className="action-type">{action.type}</span>
                      {action.target_count > 0 && (
                        <span className="action-stat">
                          <Target size={10} />
                          {action.reached_index}/{action.target_count}
                        </span>
                      )}
                      {action.exec_count > 0 && (
                        <span className="action-stat">
                          ran {action.exec_count}×
                        </span>
                      )}
                    </div>
                    {isRunning && action.target_count > 0 && (
                      <div className="progress-bar">
                        <div className="progress-fill running" style={{ width: `${progress}%` }} />
                      </div>
                    )}
                  </div>

                  <div className="action-actions">
                    {!isRunning && (action.state === 'PENDING' || action.state === 'PAUSED' || action.state === 'FAILED') && (
                      <button className="btn btn-success btn-sm" onClick={() => handleRun(action.id)} aria-label={`Run ${action.title}`}>
                        <Play size={11} /> Run
                      </button>
                    )}
                    {isRunning && (
                      <button className="btn btn-secondary btn-sm" onClick={() => handleStateChange(action.id, 'PAUSED')} aria-label={`Pause ${action.title}`}>
                        <Pause size={11} /> Pause
                      </button>
                    )}
                    <button className="btn btn-danger btn-sm btn-icon" onClick={() => handleDelete(action.id)} aria-label={`Delete ${action.title}`} title="Delete action">
                      <Trash2 size={12} />
                    </button>
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>

      {showCreate && (
        <CreateModal
          availableTypes={availableTypes}
          onClose={() => setShowCreate(false)}
          onCreated={(created) => {
            setActions(prev => [created, ...prev])
            onRefresh?.()
          }}
        />
      )}
    </>
  )
}

