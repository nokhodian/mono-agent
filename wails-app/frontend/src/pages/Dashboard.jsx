import { useEffect, useState, useCallback } from 'react'
import {
  RefreshCw, Play, Users, Shield, GitBranch,
  CheckCircle, XCircle, Clock, Loader, ChevronRight,
  ToggleLeft, ToggleRight, Layers, Zap,
} from 'lucide-react'
import { api, STATE_COLORS, PLATFORM_COLORS } from '../services/api.js'

// ── Status dot for execution ──────────────────────────────────────────────────
function ExecStatusDot({ status }) {
  const s = (status || '').toUpperCase()
  const color =
    s === 'COMPLETED' ? 'var(--green-neon)' :
    s === 'RUNNING'   ? 'var(--cyan)' :
    s === 'FAILED'    ? '#ef4444' :
    s === 'PENDING'   ? '#eab308' : 'var(--text-muted)'
  const pulse = s === 'RUNNING'
  return (
    <span style={{
      display: 'inline-block',
      width: 7, height: 7, borderRadius: '50%',
      background: color, flexShrink: 0,
      boxShadow: pulse ? `0 0 6px ${color}` : 'none',
      animation: pulse ? 'pulse 1.4s ease-in-out infinite' : 'none',
    }} />
  )
}

// ── Duration label ────────────────────────────────────────────────────────────
function duration(startedAt, finishedAt) {
  if (!startedAt) return null
  const t0 = new Date(startedAt)
  const t1 = finishedAt ? new Date(finishedAt) : new Date()
  const ms = t1 - t0
  if (isNaN(ms) || ms < 0) return null
  const sec = Math.round(ms / 1000)
  if (sec < 60) return `${sec}s`
  return `${Math.floor(sec / 60)}m ${sec % 60}s`
}

// ── Relative time ─────────────────────────────────────────────────────────────
function relTime(ts) {
  if (!ts) return '—'
  const diff = Date.now() - new Date(ts)
  if (diff < 60000) return 'just now'
  if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`
  return new Date(ts).toLocaleDateString()
}

// ── Stat card ─────────────────────────────────────────────────────────────────
function StatCard({ icon: Icon, label, value, color }) {
  return (
    <div className="stat-card" style={{ '--accent-color': color, '--icon-bg': color + '18', '--icon-color': color }}>
      <div className="stat-icon"><Icon size={16} /></div>
      <div className="stat-value">{value ?? '—'}</div>
      <div className="stat-label">{label}</div>
    </div>
  )
}

// ── Workflow row ──────────────────────────────────────────────────────────────
function WorkflowRow({ wf, execMap, onRun, onToggle, onNavigate }) {
  const execs = execMap[wf.id] || []
  const last = execs[0]
  const [running, setRunning] = useState(false)
  const [toggling, setToggling] = useState(false)

  const handleRun = async () => {
    setRunning(true)
    await onRun(wf.id)
    setTimeout(() => setRunning(false), 2000)
  }

  const handleToggle = async () => {
    setToggling(true)
    await onToggle(wf.id, !wf.is_active)
    setToggling(false)
  }

  const lastStatus = last?.status?.toUpperCase() || null
  const statusColor =
    lastStatus === 'COMPLETED' ? 'var(--green-neon)' :
    lastStatus === 'RUNNING'   ? 'var(--cyan)' :
    lastStatus === 'FAILED'    ? '#ef4444' :
    lastStatus === 'PENDING'   ? '#eab308' : 'var(--text-dim)'

  return (
    <div className="wf-row" style={{ opacity: wf.is_active ? 1 : 0.55 }}>
      {/* Active toggle */}
      <button
        className="btn btn-ghost btn-icon"
        onClick={handleToggle}
        disabled={toggling}
        title={wf.is_active ? 'Deactivate' : 'Activate'}
        style={{ color: wf.is_active ? 'var(--cyan)' : 'var(--text-dim)', padding: 2 }}
      >
        {wf.is_active
          ? <ToggleRight size={18} />
          : <ToggleLeft size={18} />}
      </button>

      {/* Name + description */}
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ fontWeight: 600, fontSize: 13, color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {wf.name}
        </div>
        {wf.description && (
          <div style={{ fontSize: 11, color: 'var(--text-muted)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', marginTop: 1 }}>
            {wf.description}
          </div>
        )}
      </div>

      {/* Last run */}
      <div style={{ textAlign: 'right', minWidth: 80 }}>
        {last ? (
          <>
            <div style={{ display: 'flex', alignItems: 'center', gap: 5, justifyContent: 'flex-end' }}>
              <ExecStatusDot status={last.status} />
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: statusColor, textTransform: 'uppercase' }}>
                {last.status}
              </span>
            </div>
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-dim)', marginTop: 2 }}>
              {relTime(last.created_at)}
            </div>
          </>
        ) : (
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-dim)' }}>never run</span>
        )}
      </div>

      {/* Run button */}
      <button
        className="btn btn-secondary btn-sm"
        onClick={handleRun}
        disabled={running || lastStatus === 'RUNNING'}
        style={{ gap: 4, minWidth: 60, flexShrink: 0 }}
      >
        {running || lastStatus === 'RUNNING'
          ? <Loader size={11} style={{ animation: 'spin 1s linear infinite' }} />
          : <Play size={11} />}
        {running || lastStatus === 'RUNNING' ? 'Running' : 'Run'}
      </button>

      {/* Arrow to workflow editor */}
      <button
        className="btn btn-ghost btn-icon"
        onClick={() => onNavigate('noderunner')}
        style={{ padding: 3, color: 'var(--text-dim)' }}
        title="Open in editor"
      >
        <ChevronRight size={14} />
      </button>
    </div>
  )
}

// ── Execution timeline row ────────────────────────────────────────────────────
function ExecRow({ exec }) {
  const dur = duration(exec.started_at, exec.finished_at)
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '7px 0', borderBottom: '1px solid var(--border-dim)' }}>
      <ExecStatusDot status={exec.status} />
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {exec.workflow_name || exec.workflow_id.slice(0, 8)}
        </div>
        {exec.error && (
          <div style={{ fontSize: 10, color: '#ef4444', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', marginTop: 1 }}>
            {exec.error}
          </div>
        )}
      </div>
      <div style={{ textAlign: 'right', flexShrink: 0 }}>
        {dur && <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--cyan-dim)' }}>{dur}</div>}
        <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-dim)' }}>{relTime(exec.created_at)}</div>
      </div>
    </div>
  )
}

// ── Main dashboard ────────────────────────────────────────────────────────────
export default function Dashboard({ stats, onRefresh, onNavigate }) {
  const [refreshing, setRefreshing]     = useState(false)
  const [workflows, setWorkflows]       = useState([])
  const [executions, setExecutions]     = useState([])
  const [execMap, setExecMap]           = useState({})   // workflowID → last execution[]

  const load = useCallback(async () => {
    const [wfs, execs] = await Promise.all([
      api.listWorkflows(),
      api.getRecentExecutions(30),
    ])
    const wfList = wfs || []
    const execList = execs || []
    setWorkflows(wfList)
    setExecutions(execList)

    // build a per-workflow map of recent executions (last 1 needed for status badge)
    const map = {}
    execList.forEach(e => {
      if (!map[e.workflow_id]) map[e.workflow_id] = []
      map[e.workflow_id].push(e)
    })
    setExecMap(map)
  }, [])

  useEffect(() => { load() }, [load])

  const handleRefresh = async () => {
    setRefreshing(true)
    await Promise.all([onRefresh(), load()])
    setTimeout(() => setRefreshing(false), 400)
  }

  const handleRun = async (id) => {
    await api.runWorkflow(id)
    setTimeout(load, 1500)
  }

  const handleToggle = async (id, active) => {
    await api.setWorkflowActive(id, active)
    await load()
  }

  const sessions    = stats?.sessions || []
  const totalPeople = stats?.total_people || 0
  const activeWFs   = workflows.filter(w => w.is_active).length
  const totalExecs  = executions.length

  return (
    <>
      <div className="page-header">
        <div className="page-header-left">
          <div className="page-title">Dashboard</div>
          <div className="page-subtitle">Workflows &amp; Executions</div>
        </div>
        <div className="page-header-right">
          <button className="btn btn-ghost btn-sm" onClick={handleRefresh} style={{ gap: 5 }}>
            <RefreshCw size={13} style={{ animation: refreshing ? 'spin 0.7s linear infinite' : 'none' }} />
            Refresh
          </button>
          <button className="btn btn-secondary btn-sm" onClick={() => onNavigate('noderunner')} style={{ gap: 5 }}>
            <GitBranch size={13} /> Workflow Editor
          </button>
        </div>
      </div>

      <div className="page-body">
        {/* ── Stat cards ── */}
        <div className="stat-grid">
          <StatCard icon={Layers}     label="Workflows"        value={workflows.length} color="var(--cyan)" />
          <StatCard icon={GitBranch}  label="Active"           value={activeWFs}        color="var(--purple-light)" />
          <StatCard icon={Zap}        label="Recent Runs"      value={totalExecs}       color="#eab308" />
          <StatCard icon={Users}      label="People Found"     value={totalPeople}      color="var(--green-neon)" />
        </div>

        <div className="dashboard-grid">
          {/* ── Left: Workflows ── */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: 0 }}>
            <div className="card" style={{ flex: 1 }}>
              <div className="section-header">
                <div className="section-title"><GitBranch size={12} /> Workflows</div>
                <button
                  className="btn btn-ghost btn-sm"
                  onClick={() => onNavigate('noderunner')}
                  style={{ fontSize: 11, gap: 3 }}
                >
                  Open Editor <ChevronRight size={11} />
                </button>
              </div>

              {workflows.length === 0 ? (
                <div className="empty-state" style={{ padding: '32px 0' }}>
                  <GitBranch size={28} style={{ color: 'var(--text-dim)', marginBottom: 8 }} />
                  <div className="empty-state-title" style={{ fontSize: 13 }}>No workflows yet</div>
                  <div className="empty-state-desc" style={{ marginBottom: 12 }}>Build your first workflow in the editor.</div>
                  <button className="btn btn-secondary btn-sm" onClick={() => onNavigate('noderunner')} style={{ gap: 5 }}>
                    <Play size={12} /> Open Editor
                  </button>
                </div>
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                  {workflows.map(wf => (
                    <WorkflowRow
                      key={wf.id}
                      wf={wf}
                      execMap={execMap}
                      onRun={handleRun}
                      onToggle={handleToggle}
                      onNavigate={onNavigate}
                    />
                  ))}
                </div>
              )}
            </div>
          </div>

          {/* ── Right: Recent executions + sessions ── */}
          <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            {/* Execution timeline */}
            <div className="card">
              <div className="section-header">
                <div className="section-title"><Clock size={12} /> Recent Runs</div>
              </div>
              {executions.length === 0 ? (
                <div style={{ padding: '20px 0', textAlign: 'center', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', fontSize: 11 }}>
                  No runs yet
                </div>
              ) : (
                <div>
                  {executions.slice(0, 15).map(e => <ExecRow key={e.id} exec={e} />)}
                </div>
              )}
            </div>

            {/* Connected accounts */}
            <div className="card">
              <div className="section-header">
                <div className="section-title"><Shield size={12} /> Connected Accounts</div>
                <button
                  className="btn btn-ghost btn-sm"
                  onClick={() => onNavigate('connections')}
                  style={{ fontSize: 11, gap: 3 }}
                >
                  Manage <ChevronRight size={11} />
                </button>
              </div>

              {sessions.length === 0 ? (
                <div style={{ padding: '16px 0', textAlign: 'center', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', fontSize: 11 }}>
                  No active sessions
                </div>
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 5 }}>
                  {sessions.map((s, i) => (
                    <div key={i} style={{
                      display: 'flex', alignItems: 'center', gap: 8,
                      padding: '7px 10px',
                      background: 'var(--elevated)',
                      borderRadius: 'var(--radius)',
                      border: `1px solid ${s.active ? 'var(--border)' : 'var(--border-dim)'}`,
                      opacity: s.active ? 1 : 0.5,
                    }}>
                      <span className={`status-dot ${s.active ? 'connected' : 'disconnected'}`} />
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--text)' }}>{s.username}</div>
                        <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)' }}>{s.platform}</div>
                      </div>
                      <span
                        className="badge"
                        style={{
                          background: (PLATFORM_COLORS[s.platform?.toUpperCase()] || 'var(--cyan)') + '20',
                          color:      (PLATFORM_COLORS[s.platform?.toUpperCase()] || 'var(--cyan)'),
                          borderColor:(PLATFORM_COLORS[s.platform?.toUpperCase()] || 'var(--cyan)') + '40',
                        }}
                      >
                        {s.platform}
                      </span>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* DB path */}
            {stats?.db_path && (
              <div className="card" style={{ padding: 12 }}>
                <div className="section-title" style={{ marginBottom: 6, fontSize: 11 }}>Database</div>
                <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)', wordBreak: 'break-all', lineHeight: 1.6 }}>
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
