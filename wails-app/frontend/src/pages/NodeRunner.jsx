import { useState, useEffect, useRef, useCallback } from 'react'
import {
  Play, RotateCcw, ZoomIn, ZoomOut, Trash2, Search,
  ChevronDown, ChevronRight, X, Settings2, Copy,
  AlertCircle, CheckCircle, Clock, Loader, Plus,
  Save, FolderOpen, ToggleLeft, ToggleRight, List,
  MessageSquare, Braces,
} from 'lucide-react'
import * as WailsApp from '../wailsjs/go/main/App'
import { api } from '../services/api.js'
import AIChatPanel from '../components/AIChatPanel.jsx'
import ResourcePickerField from '../components/ResourcePickerField.jsx'

// ── Wails bindings with mock fallback ────────────────────────────────────────
const RunNode               = WailsApp.RunNode               ?? (async (req) => ({ outputs: [{ handle: 'main', items: [{ mock: true, node_type: req.node_type }] }], duration_ms: 42 }))
const GetWorkflowNodeTypes  = WailsApp.GetWorkflowNodeTypes  ?? (async () => ({}))
const _LS = 'monoes-wf-mock-v2'
const _ms = () => { try { return JSON.parse(localStorage.getItem(_LS) || '{}') } catch { return {} } }
const _mp = s  => { try { localStorage.setItem(_LS, JSON.stringify(s)) } catch {} }
const ListWorkflows    = WailsApp.ListWorkflows    ?? (async () => Object.values(_ms()))
const GetWorkflow      = WailsApp.GetWorkflow      ?? (async (id) => _ms()[id] || null)
const SaveWorkflow     = WailsApp.SaveWorkflow     ?? (async (req) => { const s=_ms(); const id=req.id||`wf_${Date.now()}`; const now=new Date().toISOString(); const ex=s[id]||{}; const n={...ex,...req,id,updated_at:now,created_at:ex.created_at||now,version:(ex.version||0)+1}; s[id]=n; _mp(s); return n })
const DeleteWorkflow   = WailsApp.DeleteWorkflow   ?? (async (id) => { const s=_ms(); delete s[id]; _mp(s) })
const SetWorkflowActive= WailsApp.SetWorkflowActive?? (async (id,a) => { const s=_ms(); if(s[id]){s[id].active=a;_mp(s)} })
const GetWorkflowExecutions = WailsApp.GetWorkflowExecutions ?? (async () => [])

// ── Canvas geometry ───────────────────────────────────────────────────────────
const NODE_W   = 220
const HEAD_H   = 44
const PORT_H   = 28
const PORT_PAD = 8
const PORT_R   = 6

const CAT_COLOR = {
  triggers:       '#7c3aed',
  control:        '#0891b2',
  data:           '#d97706',
  http:           '#d97706',
  system:         '#64748b',
  database:       '#1d4ed8',
  communication:  '#9333ea',
  services:       '#0f766e',
  instagram:      '#e1306c',
  linkedin:       '#0a66c2',
  x:              '#8899aa',
  tiktok:         '#ff0050',
}
const catColor = (cat) => CAT_COLOR[cat] || '#00b4d8'

function nodeH(n) {
  return HEAD_H + PORT_PAD + Math.max(n.inputs.length, n.outputs.length, 1) * PORT_H + PORT_PAD
}
function inPortPos(n, i) {
  return { x: n.x, y: n.y + HEAD_H + PORT_PAD + i * PORT_H + PORT_H / 2 }
}
function outPortPos(n, i) {
  return { x: n.x + NODE_W, y: n.y + HEAD_H + PORT_PAD + i * PORT_H + PORT_H / 2 }
}
function edgePath(sx, sy, tx, ty) {
  const dx = Math.max(60, Math.abs(tx - sx) * 0.5)
  return `M${sx},${sy} C${sx+dx},${sy} ${tx-dx},${ty} ${tx},${ty}`
}

let _seq = 1
const uid = () => `nr${_seq++}`

// ── Status badge on node ──────────────────────────────────────────────────────
function NodeStatusBadge({ status, itemCount, durationMs }) {
  if (!status) return null
  const color = status === 'ok' ? '#10b981' : status === 'error' ? '#ef4444' : '#00b4d8'
  const icon = status === 'ok' ? '✓' : status === 'error' ? '✕' : '…'
  return (
    <div style={{
      position: 'absolute', top: -10, right: -6,
      background: color,
      color: '#fff',
      fontFamily: 'var(--font-mono)',
      fontSize: 9,
      borderRadius: 10,
      padding: '2px 6px',
      display: 'flex', alignItems: 'center', gap: 3,
      boxShadow: `0 0 8px ${color}66`,
      whiteSpace: 'nowrap',
    }}>
      {icon} {status === 'ok' ? `${itemCount} item${itemCount !== 1 ? 's' : ''}` : status === 'running' ? 'running' : 'error'}
      {durationMs != null && status === 'ok' && <span style={{ opacity: 0.7 }}> · {durationMs}ms</span>}
    </div>
  )
}

// ── Single canvas node ────────────────────────────────────────────────────────
function CanvasNode({ node, selected, zoom, onHeaderMouseDown, onOutputPortMouseDown, onInputPortMouseUp, onClick, onDelete, onConfigure }) {
  const h = nodeH(node)
  const color = node.color || catColor(node.category)
  const status = node.runStatus // 'running' | 'ok' | 'error' | null
  const rows = Math.max(node.inputs.length, node.outputs.length, 1)

  return (
    <div
      style={{
        position: 'absolute', left: node.x, top: node.y,
        width: NODE_W, height: h,
        background: 'linear-gradient(160deg,#0d1a28 0%,#091220 100%)',
        border: `1.5px solid ${selected ? color : status === 'error' ? '#ef444444' : status === 'ok' ? '#10b98133' : 'rgba(0,180,216,0.12)'}`,
        borderRadius: 10,
        boxShadow: selected
          ? `0 0 0 1.5px ${color}55, 0 12px 32px rgba(0,0,0,.7)`
          : '0 6px 20px rgba(0,0,0,.5)',
        userSelect: 'none',
        overflow: 'visible',
        transition: 'border-color 140ms, box-shadow 140ms',
      }}
      onMouseDown={(e) => { e.stopPropagation(); onClick?.() }}
    >
      <NodeStatusBadge
        status={status}
        itemCount={node.runOutputItems}
        durationMs={node.runDuration}
      />

      {/* Header */}
      <div
        style={{
          height: HEAD_H,
          background: `linear-gradient(110deg,${color}1a 0%,${color}0a 100%)`,
          borderBottom: `1px solid ${color}22`,
          borderRadius: '9px 9px 0 0',
          display: 'flex', alignItems: 'center',
          padding: '0 8px 0 10px',
          cursor: 'grab', gap: 6,
        }}
        onMouseDown={(e) => { e.stopPropagation(); onHeaderMouseDown(e) }}
      >
        <div style={{ width: 8, height: 8, borderRadius: '50%', background: color, flexShrink: 0 }} />
        <span style={{
          flex: 1,
          fontFamily: 'var(--font-mono)', fontSize: 11, fontWeight: 600,
          color: '#e2e8f0', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
        }}>{node.label}</span>
        <button
          onMouseDown={e => { e.stopPropagation(); onConfigure() }}
          title="Configure"
          style={{ background: 'transparent', border: 'none', cursor: 'pointer', color: 'rgba(148,163,184,.5)', padding: 2, display: 'flex', alignItems: 'center', transition: 'color 100ms' }}
          onMouseEnter={e => e.currentTarget.style.color = '#00b4d8'}
          onMouseLeave={e => e.currentTarget.style.color = 'rgba(148,163,184,.5)'}
        ><Settings2 size={12} /></button>
        <button
          onMouseDown={e => { e.stopPropagation(); onDelete() }}
          title="Delete node"
          style={{ background: 'transparent', border: 'none', cursor: 'pointer', color: 'rgba(148,163,184,.3)', padding: 2, display: 'flex', alignItems: 'center', transition: 'color 100ms' }}
          onMouseEnter={e => e.currentTarget.style.color = '#ef4444'}
          onMouseLeave={e => e.currentTarget.style.color = 'rgba(148,163,184,.3)'}
        ><X size={11} /></button>
      </div>

      {/* Ports */}
      <div style={{ position: 'relative', padding: `${PORT_PAD}px 0` }}>
        {Array.from({ length: rows }).map((_, i) => (
          <div key={i} style={{ height: PORT_H, display: 'flex', alignItems: 'center', justifyContent: 'space-between', position: 'relative' }}>
            {/* Input port */}
            {node.inputs[i] ? (
              <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
                <div
                  onMouseUp={e => { e.stopPropagation(); onInputPortMouseUp(i) }}
                  style={{
                    width: PORT_R * 2, height: PORT_R * 2,
                    borderRadius: '50%',
                    background: '#1e293b',
                    border: `1.5px solid ${color}66`,
                    cursor: 'crosshair',
                    marginLeft: -PORT_R,
                    transition: 'all 100ms',
                    flexShrink: 0,
                  }}
                  onMouseEnter={e => { e.currentTarget.style.background = color; e.currentTarget.style.borderColor = color }}
                  onMouseLeave={e => { e.currentTarget.style.background = '#1e293b'; e.currentTarget.style.borderColor = `${color}66` }}
                />
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)' }}>{node.inputs[i].label}</span>
              </div>
            ) : <div />}

            {/* Output port */}
            {node.outputs[i] ? (
              <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)' }}>{node.outputs[i].label}</span>
                <div
                  onMouseDown={e => { e.stopPropagation(); onOutputPortMouseDown(e, i) }}
                  style={{
                    width: PORT_R * 2, height: PORT_R * 2,
                    borderRadius: '50%',
                    background: '#1e293b',
                    border: `1.5px solid ${color}66`,
                    cursor: 'crosshair',
                    marginRight: -PORT_R,
                    transition: 'all 100ms',
                    flexShrink: 0,
                  }}
                  onMouseEnter={e => { e.currentTarget.style.background = color; e.currentTarget.style.borderColor = color }}
                  onMouseLeave={e => { e.currentTarget.style.background = '#1e293b'; e.currentTarget.style.borderColor = `${color}66` }}
                />
              </div>
            ) : <div />}
          </div>
        ))}
      </div>

      {/* Running pulse */}
      {status === 'running' && (
        <div style={{
          position: 'absolute', inset: 0, borderRadius: 10,
          border: `1.5px solid ${color}`,
          animation: 'nodePulse 1s ease-in-out infinite',
          pointerEvents: 'none',
        }} />
      )}
    </div>
  )
}

// ── Save workflow modal ───────────────────────────────────────────────────────
function SaveModal({ initialName, onConfirm, onClose }) {
  const [name, setName] = useState(initialName || '')
  const inputRef = useRef(null)
  useEffect(() => { inputRef.current?.focus(); inputRef.current?.select() }, [])

  const submit = () => { if (name.trim()) onConfirm(name.trim()) }

  return (
    <div style={{
      position: 'absolute', inset: 0, zIndex: 200,
      background: 'rgba(2,5,9,0.8)',
      display: 'flex', alignItems: 'center', justifyContent: 'center',
    }} onMouseDown={e => { if (e.target === e.currentTarget) onClose() }}>
      <div style={{
        width: 360,
        background: '#080d16',
        border: '1px solid rgba(0,180,216,0.25)',
        borderRadius: 12,
        padding: '20px 20px 16px',
        boxShadow: '0 24px 60px rgba(0,0,0,.85)',
        display: 'flex', flexDirection: 'column', gap: 14,
      }}>
        <div style={{ fontFamily: 'var(--font-mono)', fontSize: 12, fontWeight: 700, color: '#e2e8f0', letterSpacing: 1 }}>
          SAVE WORKFLOW
        </div>

        <div>
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', letterSpacing: 1.5, textTransform: 'uppercase', marginBottom: 6 }}>
            Workflow Name
          </div>
          <input
            ref={inputRef}
            value={name}
            onChange={e => setName(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter') submit(); if (e.key === 'Escape') onClose() }}
            placeholder="e.g. Instagram Outreach"
            style={{
              width: '100%', boxSizing: 'border-box',
              background: '#020509',
              border: '1px solid rgba(0,180,216,0.2)',
              borderRadius: 6, padding: '8px 10px',
              color: '#e2e8f0', fontFamily: 'var(--font-mono)', fontSize: 12,
              outline: 'none',
            }}
          />
        </div>

        <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
          <button
            onMouseDown={onClose}
            style={{ background: 'transparent', border: '1px solid rgba(0,180,216,0.15)', borderRadius: 6, padding: '6px 16px', cursor: 'pointer', fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)' }}
          >Cancel</button>
          <button
            onMouseDown={() => { if (name.trim()) submit() }}
            style={{ background: 'rgba(0,180,216,0.15)', border: '1px solid rgba(0,180,216,0.3)', borderRadius: 6, padding: '6px 20px', cursor: 'pointer', fontFamily: 'var(--font-mono)', fontSize: 11, color: '#00b4d8' }}
          >Save</button>
        </div>
      </div>
    </div>
  )
}

// ── Workflows modal ───────────────────────────────────────────────────────────
function WorkflowsModal({ currentId, onLoad, onDelete, onClose }) {
  const [list, setList]       = useState([])
  const [loading, setLoading] = useState(true)
  const [execsFor, setExecsFor] = useState(null) // workflowId to show executions for
  const [execs, setExecs]     = useState([])

  useEffect(() => {
    ListWorkflows().then(d => { setList(d || []); setLoading(false) }).catch(() => setLoading(false))
  }, [])

  const showExecs = async (id) => {
    setExecsFor(id)
    try { setExecs(await GetWorkflowExecutions(id)) } catch { setExecs([]) }
  }

  return (
    <div style={{
      position: 'absolute', inset: 0, zIndex: 100,
      background: 'rgba(2,5,9,0.85)',
      display: 'flex', alignItems: 'center', justifyContent: 'center',
    }} onMouseDown={e => { if (e.target === e.currentTarget) onClose() }}>
      <div style={{
        width: 520, maxHeight: '70vh',
        background: '#080d16',
        border: '1px solid rgba(0,180,216,0.2)',
        borderRadius: 12,
        display: 'flex', flexDirection: 'column',
        overflow: 'hidden',
        boxShadow: '0 24px 60px rgba(0,0,0,.8)',
      }}>
        {/* Header */}
        <div style={{ padding: '12px 16px', borderBottom: '1px solid rgba(0,180,216,0.1)', display: 'flex', alignItems: 'center', gap: 8, flexShrink: 0 }}>
          {execsFor ? (
            <>
              <button onMouseDown={() => { setExecsFor(null); setExecs([]) }} style={{ background:'transparent',border:'none',cursor:'pointer',color:'var(--text-muted)',display:'flex',alignItems:'center',gap:4,fontFamily:'var(--font-mono)',fontSize:10 }}>
                ← BACK
              </button>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: '#e2e8f0' }}>EXECUTIONS</span>
            </>
          ) : (
            <>
              <List size={13} color="#00b4d8" />
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, fontWeight: 700, color: '#e2e8f0', flex: 1 }}>SAVED WORKFLOWS</span>
            </>
          )}
          <button onMouseDown={onClose} style={{ background:'transparent',border:'none',cursor:'pointer',color:'var(--text-muted)',display:'flex' }}><X size={14} /></button>
        </div>

        {/* Body */}
        <div style={{ flex: 1, overflowY: 'auto', padding: '8px 0' }}>
          {execsFor ? (
            execs.length === 0 ? (
              <div style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)', padding: '24px', textAlign: 'center' }}>No executions found</div>
            ) : execs.map(ex => (
              <div key={ex.id} style={{ padding: '8px 16px', display: 'flex', alignItems: 'center', gap: 10, borderBottom: '1px solid rgba(0,180,216,0.05)' }}>
                <div style={{ width: 7, height: 7, borderRadius: '50%', background: ex.status === 'success' ? '#10b981' : ex.status === 'running' ? '#00b4d8' : '#ef4444', flexShrink: 0 }} />
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)', flex: 1 }}>{ex.id}</span>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)' }}>{ex.status}</span>
                {ex.started_at && <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)' }}>{new Date(ex.started_at).toLocaleString()}</span>}
              </div>
            ))
          ) : loading ? (
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)', padding: '24px', textAlign: 'center' }}>Loading…</div>
          ) : list.length === 0 ? (
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)', padding: '24px', textAlign: 'center' }}>No saved workflows yet</div>
          ) : list.map(wf => (
            <div key={wf.id} style={{
              padding: '10px 16px', display: 'flex', alignItems: 'center', gap: 10,
              borderBottom: '1px solid rgba(0,180,216,0.05)',
              background: wf.id === currentId ? 'rgba(0,180,216,0.05)' : 'transparent',
              cursor: 'pointer',
            }}
              onMouseEnter={e => { if (wf.id !== currentId) e.currentTarget.style.background = 'rgba(255,255,255,0.03)' }}
              onMouseLeave={e => { if (wf.id !== currentId) e.currentTarget.style.background = 'transparent' }}
            >
              <div style={{ width: 7, height: 7, borderRadius: '50%', background: wf.is_active ? '#10b981' : '#334155', flexShrink: 0 }} title={wf.is_active ? 'Active' : 'Inactive'} />
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: '#e2e8f0', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{wf.name || 'Untitled'}</div>
                <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', marginTop: 2 }}>
                  {(wf.nodes || []).length} nodes · v{wf.version || 1} · {wf.updated_at ? new Date(wf.updated_at).toLocaleString() : ''}
                </div>
              </div>
              <button
                onMouseDown={e => { e.stopPropagation(); showExecs(wf.id) }}
                title="View executions"
                style={{ background:'transparent',border:'none',cursor:'pointer',color:'var(--text-muted)',padding:4,display:'flex',alignItems:'center' }}
                onMouseEnter={e => e.currentTarget.style.color='#00b4d8'}
                onMouseLeave={e => e.currentTarget.style.color='var(--text-muted)'}
              ><List size={11} /></button>
              <button
                onMouseDown={e => { e.stopPropagation(); onLoad(wf.id) }}
                style={{ background:'rgba(0,180,216,0.08)',border:'1px solid rgba(0,180,216,0.2)',borderRadius:5,cursor:'pointer',color:'#00b4d8',padding:'3px 10px',fontFamily:'var(--font-mono)',fontSize:10 }}
                onMouseEnter={e => e.currentTarget.style.background='rgba(0,180,216,0.18)'}
                onMouseLeave={e => e.currentTarget.style.background='rgba(0,180,216,0.08)'}
              >Open</button>
              <button
                onMouseDown={e => { e.stopPropagation(); onDelete(wf.id).then(() => setList(l => l.filter(w => w.id !== wf.id))) }}
                title="Delete"
                style={{ background:'transparent',border:'none',cursor:'pointer',color:'rgba(239,68,68,0.4)',padding:4,display:'flex',alignItems:'center' }}
                onMouseEnter={e => e.currentTarget.style.color='#ef4444'}
                onMouseLeave={e => e.currentTarget.style.color='rgba(239,68,68,0.4)'}
              ><Trash2 size={11} /></button>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

// ── Node palette sidebar ──────────────────────────────────────────────────────
function Palette({ categories, onAdd, onNodeMouseDown }) {
  const [search, setSearch] = useState('')
  const [open, setOpen] = useState(() => {
    try { return JSON.parse(localStorage.getItem('nr2-palette-open') || '{}') } catch { return {} }
  })

  const toggle = (id) => setOpen(prev => {
    const next = { ...prev, [id]: !prev[id] }
    try { localStorage.setItem('nr2-palette-open', JSON.stringify(next)) } catch {}
    return next
  })

  const q = search.toLowerCase()
  const filtered = categories.map(cat => ({
    ...cat,
    nodes: q ? cat.nodes.filter(n => n.label.toLowerCase().includes(q) || n.subtype.toLowerCase().includes(q)) : cat.nodes,
  })).filter(cat => cat.nodes.length > 0)

  return (
    <div style={{
      width: 200, flexShrink: 0,
      background: '#080d16',
      borderRight: '1px solid rgba(0,180,216,0.1)',
      display: 'flex', flexDirection: 'column',
      overflow: 'hidden',
    }}>
      <div style={{ padding: '10px 10px 8px', borderBottom: '1px solid rgba(0,180,216,0.08)', flexShrink: 0 }}>
        <div style={{ position: 'relative' }}>
          <Search size={11} style={{ position: 'absolute', left: 8, top: '50%', transform: 'translateY(-50%)', color: 'var(--text-muted)', pointerEvents: 'none' }} />
          <input
            value={search}
            onChange={e => setSearch(e.target.value)}
            placeholder="Search nodes…"
            style={{
              width: '100%', background: '#020509',
              border: '1px solid rgba(0,180,216,0.15)', borderRadius: 6,
              padding: '5px 8px 5px 26px', color: '#e2e8f0',
              fontFamily: 'var(--font-mono)', fontSize: 11, outline: 'none', boxSizing: 'border-box',
            }}
          />
        </div>
      </div>

      <div style={{ flex: 1, overflowY: 'auto', padding: '4px 0 12px' }}>
        {filtered.map(cat => {
          const isOpen = search ? true : (open[cat.id] !== false)
          const color = catColor(cat.id)
          return (
            <div key={cat.id}>
              <div
                onClick={() => !search && toggle(cat.id)}
                style={{ display: 'flex', alignItems: 'center', gap: 5, padding: '7px 10px 5px', cursor: search ? 'default' : 'pointer', userSelect: 'none' }}
              >
                {!search && (isOpen ? <ChevronDown size={9} color={color} /> : <ChevronRight size={9} color={color} />)}
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color, letterSpacing: 1.5, textTransform: 'uppercase' }}>
                  {cat.label}
                </span>
              </div>
              {isOpen && cat.nodes.map(n => (
                <div
                  key={n.subtype}
                  onMouseDown={e => { e.preventDefault(); onNodeMouseDown(e, { ...n, category: cat.id }) }}
                  onClick={() => onAdd({ ...n, category: cat.id })}
                  title={`Click or drag to add ${n.label}`}
                  style={{
                    padding: '5px 14px 5px 20px',
                    cursor: 'grab',
                    fontFamily: 'var(--font-mono)', fontSize: 11,
                    color: 'var(--text-secondary)',
                    borderLeft: '2px solid transparent',
                    transition: 'all 80ms',
                    whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis',
                    display: 'flex', alignItems: 'center', gap: 6,
                  }}
                  onMouseEnter={e => { e.currentTarget.style.background = 'rgba(255,255,255,0.04)'; e.currentTarget.style.borderLeftColor = color; e.currentTarget.style.color = '#e2e8f0' }}
                  onMouseLeave={e => { e.currentTarget.style.background = 'transparent'; e.currentTarget.style.borderLeftColor = 'transparent'; e.currentTarget.style.color = 'var(--text-secondary)' }}
                >
                  <div style={{ width: 5, height: 5, borderRadius: '50%', background: color, flexShrink: 0 }} />
                  {n.label}
                </div>
              ))}
            </div>
          )
        })}
      </div>
    </div>
  )
}

// ── Platforms that require a credential selection ─────────────────────────────
const CREDENTIAL_PLATFORMS = {
  'service.github': 'github',
  'service.notion': 'notion',
  'service.airtable': 'airtable',
  'service.jira': 'jira',
  'service.linear': 'linear',
  'service.asana': 'asana',
  'service.stripe': 'stripe',
  'service.shopify': 'shopify',
  'service.salesforce': 'salesforce',
  'service.hubspot': 'hubspot',
  'service.google_sheets': 'google_sheets',
  'service.gmail': 'gmail',
  'service.google_drive': 'google_drive',
  'comm.slack': 'slack',
  'comm.discord': 'discord',
  'comm.twilio': 'twilio',
  'comm.whatsapp': 'whatsapp',
  'db.postgres': 'postgresql',
  'db.mysql': 'mysql',
  'db.mongodb': 'mongodb',
  'db.redis': 'redis',
}

// ── Field visibility check (depends_on support) ───────────────────────────────
function fieldIsVisible(field, config) {
  if (!field.depends_on) return true
  const depValue = String(config?.[field.depends_on.key] ?? config?.[field.depends_on.field] ?? '')
  return (field.depends_on.values || []).includes(depValue)
}

// ── Inspector panel (right side) ──────────────────────────────────────────────
function Inspector({ node, onConfigChange, onClose, onNavigate }) {
  const [copied, setCopied] = useState(false)
  const [connections, setConnections] = useState([])
  const [loadingCreds, setLoadingCreds] = useState(false)

  const platformId = node ? CREDENTIAL_PLATFORMS[node.subtype] : null

  useEffect(() => {
    if (!platformId) { setConnections([]); return }
    setLoadingCreds(true)
    api.getConnectionsForPlatform(platformId)
      .then(list => setConnections(Array.isArray(list) ? list : []))
      .catch(() => setConnections([]))
      .finally(() => setLoadingCreds(false))
  }, [platformId, node?.id])

  if (!node) return null

  const color = node.color || catColor(node.category)
  const outputItems = node.runOutputs?.flatMap(o => o.items) ?? []

  const copyOutput = () => {
    navigator.clipboard.writeText(JSON.stringify(node.runOutputs, null, 2))
    setCopied(true); setTimeout(() => setCopied(false), 1500)
  }

  return (
    <div style={{
      width: 280, flexShrink: 0,
      background: '#060b13', borderLeft: '1px solid rgba(0,180,216,0.08)',
      display: 'flex', flexDirection: 'column', overflow: 'hidden',
    }}>
      {/* Header */}
      <div style={{
        padding: '10px 12px', flexShrink: 0,
        borderBottom: '1px solid rgba(0,180,216,0.08)',
        display: 'flex', alignItems: 'center', gap: 8,
      }}>
        <div style={{ width: 8, height: 8, borderRadius: '50%', background: color }} />
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: '#e2e8f0', flex: 1, overflow: 'hidden', textOverflow: 'ellipsis' }}>{node.label}</span>
        <button onMouseDown={onClose} style={{ background: 'transparent', border: 'none', cursor: 'pointer', color: 'var(--text-muted)', display: 'flex', padding: 2 }}
          onMouseEnter={e => e.currentTarget.style.color = '#fff'}
          onMouseLeave={e => e.currentTarget.style.color = 'var(--text-muted)'}
        ><X size={12} /></button>
      </div>

      <div style={{ flex: 1, overflowY: 'auto', padding: '12px' }}>
        {/* Credential dropdown — shown for platforms that require auth */}
        {platformId && (
          <>
            <Label>CREDENTIAL</Label>
            <div style={{ marginBottom: 10 }}>
              <select
                value={node.config?.credential_id ?? ''}
                onChange={e => onConfigChange(node.id, 'credential_id', String(e.target.value))}
                style={inputStyle}
                disabled={loadingCreds}
              >
                <option value="">— None —</option>
                {connections.map(c => {
                  const id = c.id || c.ID || ''
                  const label = c.label || c.Label || c.AccountID || c.account_id || c.platform || id
                  return <option key={id} value={String(id)}>{label}</option>
                })}
              </select>
              {loadingCreds && (
                <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', marginTop: 4 }}>
                  Loading…
                </div>
              )}
              <button
                onClick={() => onNavigate?.('credentials')}
                style={{
                  background: 'transparent', border: 'none', cursor: 'pointer',
                  fontFamily: 'var(--font-mono)', fontSize: 9,
                  color: '#00b4d8', padding: '4px 0', display: 'block', marginTop: 4,
                }}
                onMouseEnter={e => e.currentTarget.style.opacity = '0.7'}
                onMouseLeave={e => e.currentTarget.style.opacity = '1'}
              >
                Manage credentials →
              </button>
            </div>
          </>
        )}

        {/* Config fields */}
        {(() => {
          const fields = node.schema?.fields || node.configFields || []
          if (fields.length === 0) {
            return (
              <div style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)', marginBottom: 12 }}>
                No config fields — this node runs with defaults.
              </div>
            )
          }
          return (
            <>
              <Label>CONFIG</Label>
              {fields.map(f => {
                if (!fieldIsVisible(f, node.config)) return null
                const val = node.config?.[f.key] ?? f.default ?? ''
                const onChange = e => onConfigChange(node.id, f.key, e.target.value)
                let inputEl
                if (f.type === 'boolean') {
                  const checked = Boolean(node.config?.[f.key] ?? f.default ?? false)
                  inputEl = (
                    <input
                      type="checkbox"
                      checked={checked}
                      onChange={e => onConfigChange(node.id, f.key, e.target.checked)}
                      style={{ accentColor: '#00b4d8', width: 14, height: 14 }}
                    />
                  )
                } else if (f.type === 'textarea') {
                  inputEl = (
                    <textarea
                      value={val}
                      onChange={onChange}
                      rows={f.rows || 3}
                      style={inputStyle}
                    />
                  )
                } else if (f.type === 'code') {
                  return (
                    <div key={f.key} className="config-field" style={{ marginBottom: 10 }}>
                      <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', letterSpacing: 1.2, textTransform: 'uppercase', marginBottom: 4 }}>
                        {f.label}{f.required ? ' *' : ''}
                        {f.language && <span className="code-lang-badge">{f.language}</span>}
                      </div>
                      <textarea
                        className="field-code"
                        rows={f.rows || 10}
                        value={node.config?.[f.key] || ''}
                        onChange={e => onConfigChange(node.id, f.key, e.target.value)}
                        placeholder={f.placeholder || `// Enter ${f.language || ''} code here`}
                        spellCheck={false}
                        autoComplete="off"
                        autoCorrect="off"
                        style={{ ...inputStyle, fontFamily: 'var(--font-mono)', fontSize: 11 }}
                      />
                      {f.help && <p className="field-help">{f.help}</p>}
                    </div>
                  )
                } else if (f.type === 'select') {
                  inputEl = (
                    <select
                      value={val}
                      onChange={onChange}
                      style={inputStyle}
                    >
                      {(f.options || []).map(o => <option key={o} value={o}>{o}</option>)}
                    </select>
                  )
                } else if (f.type === 'number') {
                  inputEl = (
                    <input
                      type="number"
                      min={f.min}
                      max={f.max}
                      value={val}
                      onChange={onChange}
                      style={inputStyle}
                    />
                  )
                } else if (f.type === 'password') {
                  inputEl = (
                    <input
                      type="password"
                      value={val}
                      onChange={onChange}
                      style={inputStyle}
                    />
                  )
                } else if (f.type === 'array') {
                  const rawVal = node.config?.[f.key]
                  const arrValue = Array.isArray(rawVal) ? rawVal :
                    (rawVal ? String(rawVal).split(',').map(s => s.trim()).filter(Boolean) : [])
                  return (
                    <div key={f.key} className="config-field" style={{ marginBottom: 10 }}>
                      <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', letterSpacing: 1.2, textTransform: 'uppercase', marginBottom: 4 }}>
                        {f.label}{f.required ? ' *' : ''}
                      </div>
                      <input
                        type="text"
                        className="array-tag-input"
                        placeholder={f.placeholder || 'Type and press Enter...'}
                        style={inputStyle}
                        onKeyDown={e => {
                          if (e.key === 'Enter' && e.target.value.trim()) {
                            e.preventDefault()
                            const newArr = [...arrValue, e.target.value.trim()]
                            onConfigChange(node.id, f.key, newArr)
                            e.target.value = ''
                          }
                        }}
                      />
                      {arrValue.length > 0 && (
                        <div className="field-tags">
                          {arrValue.map((tag, i) => (
                            <span key={i} className="field-tag">
                              <span>{tag}</span>
                              <button
                                type="button"
                                className="field-tag-remove"
                                onClick={() => {
                                  const newArr = arrValue.filter((_, idx) => idx !== i)
                                  onConfigChange(node.id, f.key, newArr)
                                }}
                              >×</button>
                            </span>
                          ))}
                        </div>
                      )}
                      {f.help && <p className="field-help">{f.help}</p>}
                    </div>
                  )
                } else if (f.type === 'resource_picker') {
                  inputEl = (
                    <ResourcePickerField
                      field={f}
                      value={node.config?.[f.key] || ''}
                      onChange={v => onConfigChange(node.id, f.key, v)}
                      credentialId={node.config?.credential_id || ''}
                      platform={node.schema?.credential_platform || ''}
                      nodeConfig={node.config}
                    />
                  )
                } else {
                  // 'text' and any unknown types
                  inputEl = (
                    <input
                      type="text"
                      value={val}
                      onChange={onChange}
                      style={inputStyle}
                    />
                  )
                }
                return (
                  <div key={f.key} style={{ marginBottom: 10 }}>
                    <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', letterSpacing: 1.2, textTransform: 'uppercase', marginBottom: 4 }}>
                      {f.label}{f.required ? ' *' : ''}
                    </div>
                    {inputEl}
                    {f.help && <p className="field-help">{f.help}</p>}
                  </div>
                )
              })}
            </>
          )
        })()}

        {/* Output results */}
        {node.runStatus && (
          <>
            <div style={{ height: 1, background: 'rgba(0,180,216,0.08)', margin: '12px 0' }} />
            <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 8 }}>
              <Label style={{ marginBottom: 0 }}>OUTPUT</Label>
              <div style={{ flex: 1 }} />
              {node.runStatus === 'ok' && (
                <button
                  onClick={copyOutput}
                  style={{ background: 'transparent', border: 'none', cursor: 'pointer', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', fontSize: 9, display: 'flex', alignItems: 'center', gap: 3 }}
                  onMouseEnter={e => e.currentTarget.style.color = '#00b4d8'}
                  onMouseLeave={e => e.currentTarget.style.color = 'var(--text-muted)'}
                >
                  <Copy size={9} /> {copied ? 'COPIED' : 'COPY'}
                </button>
              )}
            </div>

            {node.runStatus === 'running' && (
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, color: '#00b4d8', fontFamily: 'var(--font-mono)', fontSize: 11 }}>
                <Loader size={11} style={{ animation: 'spin 0.7s linear infinite' }} /> Running…
              </div>
            )}
            {node.runStatus === 'error' && (
              <div style={{ background: 'rgba(239,68,68,0.08)', border: '1px solid rgba(239,68,68,0.2)', borderRadius: 6, padding: '8px 10px' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 5, marginBottom: 4 }}>
                  <AlertCircle size={11} color="#ef4444" />
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: '#ef4444' }}>ERROR</span>
                </div>
                <pre style={{ margin: 0, fontFamily: 'var(--font-mono)', fontSize: 11, color: '#fca5a5', whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                  {node.runError}
                </pre>
              </div>
            )}
            {node.runStatus === 'ok' && node.runOutputs?.map((out, oi) => (
              <div key={oi} style={{ marginBottom: 8 }}>
                {node.runOutputs.length > 1 && (
                  <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: '#00b4d8', letterSpacing: 1.5, textTransform: 'uppercase', marginBottom: 4, display: 'flex', alignItems: 'center', gap: 4 }}>
                    <div style={{ width: 5, height: 5, borderRadius: '50%', background: '#00b4d8' }} />
                    {out.handle} · {out.items.length} item{out.items.length !== 1 ? 's' : ''}
                  </div>
                )}
                {out.items.slice(0, 5).map((item, ii) => (
                  <div key={ii} style={{ background: '#020509', border: '1px solid rgba(0,180,216,0.1)', borderRadius: 6, padding: '7px 9px', marginBottom: 5, position: 'relative' }}>
                    {out.items.length > 1 && (
                      <span style={{ position: 'absolute', top: 4, right: 6, fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)' }}>[{ii}]</span>
                    )}
                    <pre style={{ margin: 0, fontFamily: 'var(--font-mono)', fontSize: 10, color: '#e2e8f0', whiteSpace: 'pre-wrap', wordBreak: 'break-word', maxHeight: 120, overflow: 'auto' }}>
                      {JSON.stringify(item, null, 2)}
                    </pre>
                  </div>
                ))}
                {out.items.length > 5 && (
                  <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)', textAlign: 'center', padding: '4px 0' }}>
                    + {out.items.length - 5} more items
                  </div>
                )}
              </div>
            ))}
            {node.runStatus === 'ok' && node.runDuration != null && (
              <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)', display: 'flex', alignItems: 'center', gap: 4, marginTop: 6 }}>
                <Clock size={10} /> {node.runDuration}ms
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}

const inputStyle = {
  width: '100%', background: '#020509',
  border: '1px solid rgba(0,180,216,0.15)', borderRadius: 6,
  padding: '6px 8px', color: '#e2e8f0',
  fontFamily: 'var(--font-mono)', fontSize: 11, outline: 'none',
  boxSizing: 'border-box', resize: 'vertical',
}

function Label({ children, style }) {
  return (
    <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', letterSpacing: 2, textTransform: 'uppercase', marginBottom: 8, ...style }}>
      {children}
    </div>
  )
}

// ── Topological sort for execution order ──────────────────────────────────────
function topoSort(nodes, edges) {
  const inDeg = {}
  const adj = {}
  nodes.forEach(n => { inDeg[n.id] = 0; adj[n.id] = [] })
  edges.forEach(e => {
    if (adj[e.source] && inDeg[e.target] !== undefined) {
      adj[e.source].push(e.target)
      inDeg[e.target]++
    }
  })
  const queue = nodes.filter(n => inDeg[n.id] === 0).map(n => n.id)
  const order = []
  while (queue.length) {
    const id = queue.shift()
    order.push(id)
    adj[id].forEach(next => { if (--inDeg[next] === 0) queue.push(next) })
  }
  return order
}

// ── Main page ─────────────────────────────────────────────────────────────────
export default function NodeRunner({ onNavigate }) {
  const [categories, setCategories] = useState([])
  const [nodes, setNodes]           = useState([])
  const [edges, setEdges]           = useState([])
  const [selectedId, setSelectedId] = useState(null)
  const [inspectorOpen, setInspectorOpen] = useState(false)
  const [camera, setCamera]         = useState({ x: 60, y: 60, zoom: 1 })
  const [pendingEdge, setPendingEdge] = useState(null)
  const [running, setRunning]       = useState(false)
  const [globalStatus, setGlobalStatus] = useState(null) // null | 'ok' | 'error'

  // ── Workflow persistence state ────────────────────────────────────────────
  const [wfId,     setWfId]     = useState(null)
  const [wfName,   setWfName]   = useState('Untitled Workflow')
  const [wfActive, setWfActive] = useState(false)
  const [saving,        setSaving]       = useState(false)
  const [saveMsg,       setSaveMsg]       = useState(null) // { ok: bool, text: string }
  const [showWfModal,   setShowWfModal]   = useState(false)
  const [showSaveModal, setShowSaveModal] = useState(false)

  const [chatOpen, setChatOpen] = useState(false)
  const [jsonView, setJsonView] = useState(false)

  const [ghost, setGhost] = useState(null) // { template, x, y }
  const ghostRef   = useRef(null)          // same data, for mouseup handler

  const wrapperRef = useRef(null)
  const dragRef    = useRef(null)
  const nodesRef   = useRef(nodes)
  const cameraRef  = useRef(camera)
  useEffect(() => { nodesRef.current = nodes }, [nodes])
  useEffect(() => { cameraRef.current = camera }, [camera])

  // Load node types from backend
  useEffect(() => {
    GetWorkflowNodeTypes().then(data => {
      const cats = Object.entries(data).map(([id, nodes]) => ({
        id,
        label: id.toUpperCase(),
        nodes: Array.isArray(nodes) ? nodes.map(n => ({
          subtype: n.type,
          label: n.label,
          category: id,
          color: catColor(id),
          inputs:  deriveInputs(n.type),
          outputs: deriveOutputs(n.type),
          schema: n.schema || { credential_platform: null, fields: [] },
          configFields: n.schema?.fields ? n.schema.fields : getConfigFields(n.type),
        })) : [],
      })).filter(c => c.nodes.length > 0)
      setCategories(cats)
    }).catch(() => {})
  }, [])

  // ── Coordinate helpers ────────────────────────────────────────────────────
  const toWorld = useCallback((cx, cy) => {
    const rect = wrapperRef.current?.getBoundingClientRect() || { left: 0, top: 0 }
    const cam  = cameraRef.current
    return { x: (cx - rect.left - cam.x) / cam.zoom, y: (cy - rect.top - cam.y) / cam.zoom }
  }, [])

  // ── Global mouse handlers ─────────────────────────────────────────────────
  useEffect(() => {
    const onMove = (e) => {
      // Ghost drag from palette
      if (ghostRef.current) {
        setGhost(g => g ? { ...g, x: e.clientX, y: e.clientY } : null)
        return
      }
      const d = dragRef.current; if (!d) return
      if (d.type === 'canvas') {
        setCamera(c => ({ ...c, x: d.camX + (e.clientX - d.startX), y: d.camY + (e.clientY - d.startY) }))
      } else if (d.type === 'node') {
        const cam = cameraRef.current
        const dx = (e.clientX - d.startX) / cam.zoom, dy = (e.clientY - d.startY) / cam.zoom
        setNodes(prev => prev.map(n => n.id === d.nodeId ? { ...n, x: d.nx + dx, y: d.ny + dy } : n))
      } else if (d.type === 'edge') {
        const w = toWorld(e.clientX, e.clientY)
        setPendingEdge(pe => pe ? { ...pe, tx: w.x, ty: w.y } : null)
      }
    }
    const onUp = (e) => {
      // Ghost drop
      if (ghostRef.current) {
        const template = ghostRef.current.template
        ghostRef.current = null
        setGhost(null)
        // Only drop if mouse released over canvas (not over palette or inspector)
        const canvasEl = wrapperRef.current
        if (canvasEl) {
          const rect = canvasEl.getBoundingClientRect()
          if (e.clientX >= rect.left && e.clientX <= rect.right &&
              e.clientY >= rect.top  && e.clientY <= rect.bottom) {
            const cam = cameraRef.current
            const wx = (e.clientX - rect.left - cam.x) / cam.zoom
            const wy = (e.clientY - rect.top  - cam.y) / cam.zoom
            // Use functional addNode by calling it after render via timeout
            addNodeRef.current?.(template, wx, wy)
          }
        }
        return
      }
      if (dragRef.current?.type === 'edge') setPendingEdge(null)
      dragRef.current = null
    }
    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onUp)
    return () => { document.removeEventListener('mousemove', onMove); document.removeEventListener('mouseup', onUp) }
  }, [toWorld])

  // ── Scroll to zoom ────────────────────────────────────────────────────────
  useEffect(() => {
    const el = wrapperRef.current; if (!el) return
    const onWheel = (e) => {
      e.preventDefault()
      const factor = e.deltaY < 0 ? 1.1 : 0.9
      setCamera(c => {
        const z = Math.max(0.25, Math.min(2.5, c.zoom * factor))
        const rect = el.getBoundingClientRect()
        const mx = e.clientX - rect.left, my = e.clientY - rect.top
        return { x: mx - (mx - c.x) * (z / c.zoom), y: my - (my - c.y) * (z / c.zoom), zoom: z }
      })
    }
    el.addEventListener('wheel', onWheel, { passive: false })
    return () => el.removeEventListener('wheel', onWheel)
  }, [])

  // ── Delete selected node ──────────────────────────────────────────────────
  useEffect(() => {
    const onKey = (e) => {
      if ((e.key === 'Delete' || e.key === 'Backspace') && !['INPUT','TEXTAREA','SELECT'].includes(e.target.tagName)) {
        if (selectedId) deleteNode(selectedId)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [selectedId]) // eslint-disable-line

  // ── Add node ──────────────────────────────────────────────────────────────
  const addNode = useCallback((template, worldX, worldY) => {
    const cam = cameraRef.current
    const rect = wrapperRef.current?.getBoundingClientRect() || { width: 900, height: 600 }
    const x = worldX ?? (rect.width / 2 - cam.x) / cam.zoom - NODE_W / 2 + (Math.random() - .5) * 100
    const y = worldY ?? (rect.height / 2 - cam.y) / cam.zoom - 60 + (Math.random() - .5) * 80
    const defaults = {}
    template.configFields?.forEach(f => { defaults[f.key] = f.default ?? '' })
    const id = uid()
    setNodes(prev => [...prev, {
      id, label: template.label, subtype: template.subtype,
      category: template.category,
      color: template.color || catColor(template.category || template.id),
      inputs:  template.inputs  || [],
      outputs: template.outputs || [],
      schema: template.schema || { credential_platform: null, fields: [] },
      configFields: template.configFields || [],
      config: defaults,
      x, y,
      runStatus: null, runOutputs: null, runOutputItems: 0, runDuration: null, runError: null,
    }])
    setSelectedId(id)
  }, [])

  // Keep addNode accessible from mouseup closure via ref
  const addNodeRef = useRef(null)
  useEffect(() => { addNodeRef.current = addNode }, [addNode])

  // ── Palette ghost drag start ──────────────────────────────────────────────
  const onPaletteNodeMouseDown = useCallback((e, template) => {
    e.preventDefault()
    e.stopPropagation()
    ghostRef.current = { template }
    setGhost({ template, x: e.clientX, y: e.clientY })
  }, [])

  const deleteNode = (id) => {
    setNodes(prev => prev.filter(n => n.id !== id))
    setEdges(prev => prev.filter(e => e.source !== id && e.target !== id))
    if (id === selectedId) { setSelectedId(null); setInspectorOpen(false) }
  }

  const updateConfig = (nodeId, key, val) =>
    setNodes(prev => prev.map(n => n.id === nodeId ? { ...n, config: { ...n.config, [key]: val } } : n))

  // ── Edge drawing ──────────────────────────────────────────────────────────
  const startEdge = (e, nodeId, portIdx) => {
    const node = nodesRef.current.find(n => n.id === nodeId); if (!node) return
    const pos = outPortPos(node, portIdx)
    dragRef.current = { type: 'edge', sourceNodeId: nodeId, sourcePortIdx: portIdx }
    setPendingEdge({ sx: pos.x, sy: pos.y, tx: pos.x, ty: pos.y })
  }

  const completeEdge = (targetNodeId, targetPortIdx) => {
    if (!dragRef.current || dragRef.current.type !== 'edge') return
    const { sourceNodeId, sourcePortIdx } = dragRef.current
    if (sourceNodeId === targetNodeId) { dragRef.current = null; setPendingEdge(null); return }
    const sNode = nodesRef.current.find(n => n.id === sourceNodeId)
    const tNode = nodesRef.current.find(n => n.id === targetNodeId)
    if (!sNode || !tNode) { dragRef.current = null; setPendingEdge(null); return }
    setEdges(prev => {
      if (prev.some(e => e.source === sourceNodeId && e.sourcePortIdx === sourcePortIdx && e.target === targetNodeId && e.targetPortIdx === targetPortIdx)) return prev
      return [...prev, {
        id: uid(), source: sourceNodeId, sourcePortIdx,
        sourcePortId: sNode.outputs[sourcePortIdx]?.id,
        target: targetNodeId, targetPortIdx,
        targetPortId: tNode.inputs[targetPortIdx]?.id,
      }]
    })
    dragRef.current = null; setPendingEdge(null)
  }

  // ── RUN ──────────────────────────────────────────────────────────────────
  const handleRun = async () => {
    if (running || nodes.length === 0) return
    setRunning(true)
    setGlobalStatus(null)

    // Reset all node statuses
    setNodes(prev => prev.map(n => ({ ...n, runStatus: null, runOutputs: null, runOutputItems: 0, runDuration: null, runError: null })))

    const order = topoSort(nodes, edges)
    const nodeOutputsMap = {} // nodeId → items from "main" handle
    let hadError = false

    for (const nodeId of order) {
      const node = nodesRef.current.find(n => n.id === nodeId)
      if (!node) continue

      // Skip trigger nodes — they have no executor; they are metadata-only
      if (node.subtype?.startsWith('trigger.')) {
        setNodes(prev => prev.map(n => n.id === nodeId ? { ...n, runStatus: 'ok', runOutputItems: 0, runDuration: 0 } : n))
        nodeOutputsMap[nodeId] = [{}]
        continue
      }

      // Collect input items from upstream nodes
      const incomingEdges = edges.filter(e => e.target === nodeId)
      let inputItems = incomingEdges.length > 0
        ? incomingEdges.flatMap(e => nodeOutputsMap[e.source] || [])
        : [{}]

      // Mark as running
      setNodes(prev => prev.map(n => n.id === nodeId ? { ...n, runStatus: 'running' } : n))

      try {
        const result = await RunNode({ node_type: node.subtype, config: node.config || {}, items: inputItems })
        if (result.error) {
          setNodes(prev => prev.map(n => n.id === nodeId ? { ...n, runStatus: 'error', runError: result.error, runDuration: result.duration_ms } : n))
          hadError = true
          nodeOutputsMap[nodeId] = []
        } else {
          const mainOut = result.outputs?.find(o => o.handle === 'main') || result.outputs?.[0]
          nodeOutputsMap[nodeId] = mainOut?.items || []
          const totalItems = result.outputs?.reduce((s, o) => s + o.items.length, 0) || 0
          setNodes(prev => prev.map(n => n.id === nodeId ? {
            ...n,
            runStatus: 'ok',
            runOutputs: result.outputs,
            runOutputItems: totalItems,
            runDuration: result.duration_ms,
          } : n))
        }
      } catch (err) {
        setNodes(prev => prev.map(n => n.id === nodeId ? { ...n, runStatus: 'error', runError: String(err) } : n))
        hadError = true
        nodeOutputsMap[nodeId] = []
      }
    }

    setGlobalStatus(hadError ? 'error' : 'ok')
    setRunning(false)
  }

  // ── Save workflow ─────────────────────────────────────────────────────────
  const handleSave = useCallback(async (nameOverride) => {
    if (saving) return
    setSaving(true)
    setShowSaveModal(false)
    const finalName = nameOverride || wfName || 'Untitled Workflow'
    if (nameOverride) setWfName(nameOverride)
    try {
      const req = {
        id: wfId || '',
        name: finalName,
        description: '',
        nodes: nodes.map(n => ({
          id: n.id,
          node_type: n.subtype,
          name: n.label,
          config: n.config || {},
          position_x: n.x,
          position_y: n.y,
          disabled: false,
        })),
        connections: edges.map((e, i) => ({
          id: e.id,
          source_node_id: e.source,
          source_handle: e.sourcePortId || String(e.sourcePortIdx ?? 0),
          target_node_id: e.target,
          target_handle: e.targetPortId || String(e.targetPortIdx ?? 0),
          position: i,
        })),
      }
      const saved = await SaveWorkflow(req)
      if (saved?.id) {
        setWfId(saved.id)
        setSaveMsg({ ok: true, text: 'Saved' })
      } else {
        setSaveMsg({ ok: false, text: 'Save returned no ID' })
      }
    } catch (e) {
      setSaveMsg({ ok: false, text: String(e) })
    } finally {
      setSaving(false)
      setTimeout(() => setSaveMsg(null), 3000)
    }
  }, [saving, wfId, wfName, nodes, edges, setShowSaveModal])

  // ── Load workflow ─────────────────────────────────────────────────────────
  const handleLoad = useCallback(async (id) => {
    try {
      const wf = await GetWorkflow(id)
      if (!wf) return
      setWfId(wf.id)
      setWfName(wf.name || 'Untitled Workflow')
      setWfActive(!!wf.is_active)
      // Map backend WorkflowNodeData → canvas node shape
      const prefixToCat = { core: 'control', db: 'database', comm: 'communication', service: 'services', data: 'data', http: 'http', system: 'system', trigger: 'triggers', ai: 'ai', instagram: 'instagram', linkedin: 'linkedin', x: 'x', tiktok: 'tiktok' }
      const loadedNodes = (wf.nodes || []).map(n => {
        const nt = normalizeNodeType(n.node_type || n.type || '')
        const prefix = nt.split('.')[0]
        const cat = prefixToCat[prefix] || prefix
        return {
        id:       n.id,
        label:    n.name,
        subtype:  nt,
        category: cat,
        x: n.position_x,
        y: n.position_y,
        config: n.config || {},
        color: catColor(cat),
        schema: n.schema || null,
        configFields: n.schema?.fields ? n.schema.fields : getConfigFields(nt),
        inputs:  deriveInputs(nt),
        outputs: deriveOutputs(nt),
        runStatus: null, runOutputs: null, runOutputItems: 0, runDuration: null, runError: null,
      }})
      // Map backend WorkflowConnectionData → canvas edge shape
      const loadedEdges = (wf.connections || []).map(c => ({
        id:           c.id,
        source:       c.source_node_id,
        sourcePortId: c.source_handle,
        sourcePortIdx: parseInt(c.source_handle) || 0,
        target:       c.target_node_id,
        targetPortId: c.target_handle,
        targetPortIdx: parseInt(c.target_handle) || 0,
      }))
      setNodes(loadedNodes)
      setEdges(loadedEdges)
      setSelectedId(null)
      setGlobalStatus(null)
      setCamera({ x: 60, y: 60, zoom: 1 })
      setShowWfModal(false)
    } catch (e) {
      console.error('Load failed', e)
    }
  }, [])

  // ── Toggle active ─────────────────────────────────────────────────────────
  const handleToggleActive = useCallback(async () => {
    const next = !wfActive
    setWfActive(next)
    if (wfId) { try { await SetWorkflowActive(wfId, next) } catch {} }
  }, [wfActive, wfId])

  // ── New canvas ────────────────────────────────────────────────────────────
  const handleNew = useCallback(() => {
    setWfId(null); setWfName('Untitled Workflow'); setWfActive(false)
    setNodes([]); setEdges([]); setSelectedId(null); setGlobalStatus(null)
    setCamera({ x: 60, y: 60, zoom: 1 })
  }, [nodes.length])

  // ── Edge paths ────────────────────────────────────────────────────────────
  const edgePaths = edges.map(edge => {
    const sNode = nodes.find(n => n.id === edge.source)
    const tNode = nodes.find(n => n.id === edge.target)
    if (!sNode || !tNode) return null
    const sp = outPortPos(sNode, edge.sourcePortIdx)
    const tp = inPortPos(tNode, edge.targetPortIdx)
    return { ...edge, path: edgePath(sp.x, sp.y, tp.x, tp.y), color: sNode.color || catColor(sNode.category) }
  }).filter(Boolean)

  const pendingPath = pendingEdge ? edgePath(pendingEdge.sx, pendingEdge.sy, pendingEdge.tx, pendingEdge.ty) : null
  const selectedNode = nodes.find(n => n.id === selectedId) || null
  const canRun = nodes.length > 0 && !running

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', background: '#04060a', overflow: 'hidden' }}>
      {/* ── TOOLBAR ── */}
      <div style={{
        height: 44, flexShrink: 0,
        display: 'flex', alignItems: 'center', gap: 6,
        padding: '0 12px',
        background: '#080d16',
        borderBottom: '1px solid rgba(0,180,216,0.1)',
        zIndex: 10,
      }}>
        {/* Workflow name */}
        <input
          value={wfName}
          onChange={e => setWfName(e.target.value)}
          style={{
            background: 'transparent', border: 'none', outline: 'none',
            fontFamily: 'var(--font-mono)', fontSize: 12, fontWeight: 700,
            color: 'var(--text-secondary)', letterSpacing: 1,
            width: 200, minWidth: 80,
          }}
        />

        {/* Active toggle */}
        <button
          onMouseDown={handleToggleActive}
          title={wfActive ? 'Deactivate' : 'Activate'}
          style={{ background: 'transparent', border: 'none', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 4, color: wfActive ? '#10b981' : 'var(--text-muted)', padding: '2px 4px' }}
        >
          {wfActive ? <ToggleRight size={16} /> : <ToggleLeft size={16} />}
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9, letterSpacing: 1 }}>{wfActive ? 'ACTIVE' : 'OFF'}</span>
        </button>

        <div style={{ width: 1, height: 16, background: 'rgba(0,180,216,0.15)' }} />

        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)' }}>
          {nodes.length}n · {edges.length}e
        </span>

        {saveMsg && (
          <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
            {saveMsg.ok
              ? <CheckCircle size={11} color="#10b981" />
              : <AlertCircle size={11} color="#ef4444" />}
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: saveMsg.ok ? '#10b981' : '#ef4444', maxWidth: 200, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {saveMsg.text}
            </span>
          </div>
        )}

        {globalStatus && !saveMsg && (
          <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
            {globalStatus === 'ok'
              ? <CheckCircle size={11} color="#10b981" />
              : <AlertCircle size={11} color="#ef4444" />}
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: globalStatus === 'ok' ? '#10b981' : '#ef4444' }}>
              {globalStatus === 'ok' ? 'done' : 'failed'}
            </span>
          </div>
        )}

        <div style={{ flex: 1 }} />

        {/* New */}
        <button style={tbBtn} onMouseDown={handleNew} title="New workflow"><Plus size={13} /></button>

        {/* Load */}
        <button style={tbBtn} onMouseDown={() => setShowWfModal(true)} title="Open saved workflow"><FolderOpen size={13} /></button>

        {/* Save */}
        <button
          style={{ ...tbBtn, color: saving ? 'var(--text-muted)' : '#00b4d8', borderColor: 'rgba(0,180,216,0.3)' }}
          onMouseDown={() => { if (!saving) setShowSaveModal(true) }}
          title="Save workflow"
        >
          {saving ? <Loader size={12} style={{ animation: 'spin 0.7s linear infinite' }} /> : <Save size={13} />}
        </button>

        <div style={{ width: 1, height: 16, background: 'rgba(0,180,216,0.15)' }} />

        {/* Zoom */}
        <button style={tbBtn} onMouseDown={() => setCamera(c => ({ ...c, zoom: Math.max(0.25, c.zoom / 1.2) }))} title="Zoom out"><ZoomOut size={13} /></button>
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)', minWidth: 32, textAlign: 'center' }}>
          {Math.round(camera.zoom * 100)}%
        </span>
        <button style={tbBtn} onMouseDown={() => setCamera(c => ({ ...c, zoom: Math.min(2.5, c.zoom * 1.2) }))} title="Zoom in"><ZoomIn size={13} /></button>
        <button style={tbBtn} onMouseDown={() => setCamera({ x: 60, y: 60, zoom: 1 })} title="Reset view"><RotateCcw size={13} /></button>

        {/* Clear */}
        <button
          style={{ ...tbBtn, color: nodes.length ? 'rgba(239,68,68,0.6)' : 'var(--text-muted)' }}
          onMouseDown={() => { setNodes([]); setEdges([]); setSelectedId(null); setGlobalStatus(null) }}
          title="Clear canvas"
        >
          <Trash2 size={13} />
        </button>

        {/* JSON view toggle */}
        <button
          style={{ ...tbBtn, color: jsonView ? '#00b4d8' : 'var(--text-muted)', borderColor: jsonView ? 'rgba(0,180,216,0.3)' : 'rgba(0,180,216,0.15)', background: jsonView ? 'rgba(0,180,216,0.08)' : 'transparent' }}
          onMouseDown={() => setJsonView(v => !v)}
          title={jsonView ? 'Switch to visual canvas' : 'Switch to JSON view'}
        >
          <Braces size={13} />
        </button>

        {/* AI Chat toggle */}
        <button
          style={{ ...tbBtn, color: chatOpen ? '#00b4d8' : 'var(--text-muted)', borderColor: chatOpen ? 'rgba(0,180,216,0.3)' : 'rgba(0,180,216,0.15)', background: chatOpen ? 'rgba(0,180,216,0.08)' : 'transparent' }}
          onMouseDown={() => setChatOpen(o => !o)}
          title="AI Assistant"
        >
          <MessageSquare size={13} />
        </button>

        {/* Run */}
        <button
          onMouseDown={handleRun}
          disabled={!canRun}
          style={{
            ...tbBtn,
            background: canRun ? 'rgba(16,185,129,0.12)' : 'rgba(100,116,139,0.06)',
            border: `1px solid ${canRun ? 'rgba(16,185,129,0.3)' : 'rgba(100,116,139,0.1)'}`,
            color: canRun ? '#10b981' : 'var(--text-muted)',
            padding: '5px 14px', gap: 5,
            opacity: canRun ? 1 : 0.5,
          }}
          onMouseEnter={e => { if (canRun) e.currentTarget.style.background = 'rgba(16,185,129,0.2)' }}
          onMouseLeave={e => { if (canRun) e.currentTarget.style.background = 'rgba(16,185,129,0.12)' }}
          title="Run all nodes"
        >
          {running ? <Loader size={12} style={{ animation: 'spin 0.7s linear infinite' }} /> : <Play size={12} />}
          {running ? 'RUNNING…' : 'RUN'}
        </button>
      </div>

      {/* ── SAVE MODAL ── */}
      {showSaveModal && (
        <SaveModal
          initialName={wfName}
          onConfirm={handleSave}
          onClose={() => setShowSaveModal(false)}
        />
      )}

      {/* ── WORKFLOWS MODAL ── */}
      {showWfModal && (
        <WorkflowsModal
          currentId={wfId}
          onLoad={handleLoad}
          onDelete={async (id) => { await DeleteWorkflow(id); if (id === wfId) { setWfId(null); setWfName('Untitled Workflow') } }}
          onClose={() => setShowWfModal(false)}
        />
      )}

      {/* ── MAIN LAYOUT ── */}
      <div style={{ display: 'flex', flex: 1, overflow: 'hidden' }}>

        {/* Palette */}
        <Palette categories={categories} onAdd={addNode} onNodeMouseDown={onPaletteNodeMouseDown} />

        {/* Canvas / JSON view */}
        {jsonView ? (
          <div style={{
            flex: 1, position: 'relative', overflow: 'auto',
            background: '#04060a',
            padding: 16,
          }}>
            <div style={{
              fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)',
              letterSpacing: 1.5, textTransform: 'uppercase', marginBottom: 10,
              display: 'flex', alignItems: 'center', gap: 6,
            }}>
              <Braces size={11} color="#00b4d8" />
              WORKFLOW JSON
              {wfId && <span style={{ opacity: 0.5 }}>· {wfId}</span>}
            </div>
            <pre style={{
              margin: 0,
              fontFamily: 'var(--font-mono)', fontSize: 11,
              color: '#e2e8f0', lineHeight: 1.6,
              whiteSpace: 'pre-wrap', wordBreak: 'break-word',
              background: '#020509',
              border: '1px solid rgba(0,180,216,0.1)',
              borderRadius: 8,
              padding: 16,
              minHeight: 200,
            }}>
              {JSON.stringify({
                id: wfId || null,
                name: wfName,
                nodes: nodes.map(n => ({
                  id: n.id,
                  node_type: n.subtype,
                  name: n.label,
                  config: n.config || {},
                  position_x: Math.round(n.x),
                  position_y: Math.round(n.y),
                })),
                connections: edges.map((e, i) => ({
                  id: e.id,
                  source_node_id: e.source,
                  source_handle: e.sourcePortId || String(e.sourcePortIdx ?? 0),
                  target_node_id: e.target,
                  target_handle: e.targetPortId || String(e.targetPortIdx ?? 0),
                })),
              }, null, 2)}
            </pre>
          </div>
        ) : (
          <div
            ref={wrapperRef}
            style={{ flex: 1, position: 'relative', overflow: 'hidden', cursor: 'default' }}
            onMouseDown={(e) => {
              if (e.target !== wrapperRef.current && !e.target.dataset.bg) return
              setSelectedId(null)
              dragRef.current = { type: 'canvas', startX: e.clientX, startY: e.clientY, camX: cameraRef.current.x, camY: cameraRef.current.y }
            }}
          >
            {/* Dot grid */}
            <div data-bg="1" style={{
              position: 'absolute', inset: 0,
              backgroundImage: 'radial-gradient(circle,rgba(0,180,216,0.18) 1.2px,transparent 1.2px)',
              backgroundSize: '28px 28px',
              backgroundPosition: `${camera.x % 28}px ${camera.y % 28}px`,
              pointerEvents: 'none',
            }} />

            {/* Empty state */}
            {nodes.length === 0 && (
              <div style={{
                position: 'absolute', inset: 0, display: 'flex', flexDirection: 'column',
                alignItems: 'center', justifyContent: 'center', gap: 12,
                pointerEvents: 'none',
              }}>
                <Plus size={32} style={{ opacity: 0.1 }} />
                <div style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--text-muted)', textAlign: 'center', lineHeight: 1.8 }}>
                  Click or drag nodes from the left panel<br />
                  Connect output → input ports to chain them<br />
                  Press <kbd style={{ background: 'rgba(0,180,216,0.1)', border: '1px solid rgba(0,180,216,0.2)', borderRadius: 3, padding: '1px 5px' }}>RUN</kbd> to execute
                </div>
              </div>
            )}

            {/* SVG edges */}
            <svg style={{ position: 'absolute', inset: 0, width: '100%', height: '100%', overflow: 'visible', zIndex: 1, pointerEvents: 'none' }}>
              <g transform={`translate(${camera.x} ${camera.y}) scale(${camera.zoom})`}>
                {edgePaths.map(ep => (
                  <g key={ep.id}>
                    <path d={ep.path} stroke={ep.color} strokeWidth={1.5} fill="none" strokeOpacity={0.5} />
                    <path d={ep.path} stroke={ep.color} strokeWidth={4} fill="none" strokeOpacity={0}
                      style={{ pointerEvents: 'stroke', cursor: 'pointer' }}
                      onClick={() => setEdges(prev => prev.filter(e => e.id !== ep.id))}
                    />
                  </g>
                ))}
                {pendingPath && (
                  <path d={pendingPath} stroke="#00b4d8" strokeWidth={1.5} fill="none" strokeDasharray="5 4" strokeOpacity={0.6} />
                )}
              </g>
            </svg>

            {/* Nodes */}
            <div style={{ position: 'absolute', inset: 0, zIndex: 2, transformOrigin: '0 0', transform: `translate(${camera.x}px,${camera.y}px) scale(${camera.zoom})` }}>
              {nodes.map(node => (
                <CanvasNode
                  key={node.id}
                  node={node}
                  selected={selectedId === node.id}
                  zoom={camera.zoom}
                  onClick={() => setSelectedId(node.id)}
                  onDelete={() => deleteNode(node.id)}
                  onConfigure={() => { setSelectedId(node.id); setInspectorOpen(true) }}
                  onHeaderMouseDown={(e) => {
                    dragRef.current = { type: 'node', nodeId: node.id, startX: e.clientX, startY: e.clientY, nx: node.x, ny: node.y }
                  }}
                  onOutputPortMouseDown={(e, portIdx) => startEdge(e, node.id, portIdx)}
                  onInputPortMouseUp={(portIdx) => completeEdge(node.id, portIdx)}
                />
              ))}
            </div>
          </div>
        )}

        {/* Inspector — only shown when user clicks Settings on a node */}
        {inspectorOpen && selectedNode && (
          <Inspector
            node={selectedNode}
            onConfigChange={updateConfig}
            onClose={() => setInspectorOpen(false)}
            onNavigate={onNavigate}
          />
        )}

        {/* AI Chat Panel */}
        <AIChatPanel
          workflowID={wfId || 'draft'}
          isOpen={chatOpen}
          onClose={() => setChatOpen(false)}
          onWorkflowCreated={handleLoad}
        />
      </div>

      {/* Ghost node following cursor during palette drag */}
      {ghost && (
        <div style={{
          position: 'fixed',
          left: ghost.x + 12, top: ghost.y - 16,
          pointerEvents: 'none',
          zIndex: 9999,
          background: 'linear-gradient(160deg,#0d1a28 0%,#091220 100%)',
          border: `1.5px solid ${ghost.template.color || catColor(ghost.template.category || '')}66`,
          borderRadius: 10,
          padding: '8px 14px',
          fontFamily: 'var(--font-mono)', fontSize: 11, color: '#e2e8f0',
          opacity: 0.85,
          boxShadow: '0 8px 24px rgba(0,0,0,.6)',
          display: 'flex', alignItems: 'center', gap: 7,
          userSelect: 'none',
          whiteSpace: 'nowrap',
        }}>
          <div style={{ width: 7, height: 7, borderRadius: '50%', background: ghost.template.color || catColor(ghost.template.category || ''), flexShrink: 0 }} />
          {ghost.template.label}
        </div>
      )}

      <style>{`
        @keyframes spin { to { transform: rotate(360deg); } }
        @keyframes nodePulse { 0%,100% { opacity:.4; } 50% { opacity:1; } }
      `}</style>
    </div>
  )
}

// ── Config field definitions per node type ────────────────────────────────────
// field: { key, label, type: 'text'|'textarea'|'select'|'number'|'password', options?, default? }
// Legacy (unprefixed) → new prefixed node type names.
// Mirrors legacyNodeTypes map in app.go.
const LEGACY_NODE_TYPES = {
  'google_sheets': 'service.google_sheets', 'gmail': 'service.gmail', 'google_drive': 'service.google_drive',
  'github': 'service.github', 'notion': 'service.notion', 'airtable': 'service.airtable',
  'jira': 'service.jira', 'linear': 'service.linear', 'asana': 'service.asana',
  'stripe': 'service.stripe', 'shopify': 'service.shopify', 'salesforce': 'service.salesforce',
  'hubspot': 'service.hubspot',
  'slack': 'comm.slack', 'discord': 'comm.discord', 'telegram': 'comm.telegram',
  'twilio': 'comm.twilio', 'whatsapp': 'comm.whatsapp',
  'email_send': 'comm.email_send', 'email_read': 'comm.email_read',
  'mysql': 'db.mysql', 'postgres': 'db.postgres', 'mongodb': 'db.mongodb', 'redis': 'db.redis',
  'datetime': 'data.datetime', 'crypto': 'data.crypto', 'html': 'data.html',
  'xml': 'data.xml', 'markdown': 'data.markdown', 'spreadsheet': 'data.spreadsheet',
  'compression': 'data.compression', 'write_binary_file': 'data.write_binary_file',
  'if': 'core.if', 'switch': 'core.switch', 'merge': 'core.merge', 'set': 'core.set',
  'code': 'core.code', 'filter': 'core.filter', 'sort': 'core.sort', 'limit': 'core.limit',
  'aggregate': 'core.aggregate', 'wait': 'core.wait',
  'http_request': 'http.request', 'http_response': 'http.response',
  'execute_command': 'system.execute_command', 'rss_read': 'system.rss_read',
  'read_write_file': 'system.read_write_file',
}
function normalizeNodeType(t) { return LEGACY_NODE_TYPES[t] || t }

const NODE_CONFIG_FIELDS = {
  // ── Triggers ──────────────────────────────────────────────────────────────
  'trigger.schedule': [
    { key: 'cron', label: 'Cron Expression', type: 'text', default: '0 * * * *' },
    { key: 'timezone', label: 'Timezone', type: 'text', default: 'UTC' },
  ],
  'trigger.webhook': [
    { key: 'path', label: 'URL Path', type: 'text', default: '/webhook' },
    { key: 'method', label: 'HTTP Method', type: 'select', options: ['GET','POST','PUT','PATCH','DELETE'], default: 'POST' },
    { key: 'auth_type', label: 'Auth Type', type: 'select', options: ['none','header_token','basic'], default: 'none' },
    { key: 'auth_token', label: 'Auth Token', type: 'password', default: '' },
  ],

  // ── Control ───────────────────────────────────────────────────────────────
  'core.if': [
    { key: 'condition', label: 'Condition', type: 'text', default: '{{$json.value}} == true' },
    { key: 'mode', label: 'Mode', type: 'select', options: ['expression','regex'], default: 'expression' },
  ],
  'core.filter': [
    { key: 'condition', label: 'Condition', type: 'text', default: '{{$json.value}} != ""' },
    { key: 'mode', label: 'Mode', type: 'select', options: ['expression','regex'], default: 'expression' },
  ],
  'core.switch': [
    { key: 'expression', label: 'Expression', type: 'text', default: '{{$json.status}}' },
    { key: 'cases', label: 'Cases (JSON array)', type: 'textarea', default: '[{"value":"active"},{"value":"inactive"}]' },
    { key: 'default_handle', label: 'Default Handle', type: 'text', default: 'default' },
  ],
  'core.set': [
    { key: 'assignments', label: 'Assignments (JSON)', type: 'textarea', default: '[{"name":"output","value":"{{$json.input}}"}]' },
    { key: 'include_input', label: 'Include Input Fields', type: 'select', options: ['true','false'], default: 'true' },
  ],
  'core.code': [
    { key: 'code', label: 'JavaScript Code', type: 'textarea', default: '// return array of items\nreturn items.map(item => ({ ...item.json }))' },
  ],
  'core.wait': [
    { key: 'duration', label: 'Duration (e.g. 5s, 2m, 1h)', type: 'text', default: '5s' },
  ],
  'core.limit': [
    { key: 'max_items', label: 'Max Items', type: 'number', default: '10' },
  ],
  'core.split_in_batches': [
    { key: 'batch_size', label: 'Batch Size', type: 'number', default: '10' },
  ],
  'core.sort': [
    { key: 'field', label: 'Field', type: 'text', default: 'name' },
    { key: 'order', label: 'Order', type: 'select', options: ['asc','desc'], default: 'asc' },
    { key: 'type', label: 'Sort Type', type: 'select', options: ['string','number','date'], default: 'string' },
  ],
  'core.remove_duplicates': [
    { key: 'field', label: 'Field Key', type: 'text', default: 'id' },
    { key: 'keep', label: 'Keep', type: 'select', options: ['first','last'], default: 'first' },
  ],
  'core.stop_error': [
    { key: 'message', label: 'Error Message', type: 'text', default: 'Workflow stopped with error' },
  ],
  'core.merge': [
    { key: 'mode', label: 'Mode', type: 'select', options: ['append','first'], default: 'append' },
  ],
  'core.compare_datasets': [
    { key: 'key_field', label: 'Key Field', type: 'text', default: 'id' },
    { key: 'split_at', label: 'Split At Index', type: 'number', default: '' },
  ],
  'core.aggregate': [
    { key: 'group_by', label: 'Group By Field', type: 'text', default: '' },
    { key: 'operations', label: 'Operations (JSON)', type: 'textarea', default: '[{"field":"amount","operation":"sum","output_field":"total"}]' },
  ],

  // ── HTTP ──────────────────────────────────────────────────────────────────
  'http.request': [
    { key: 'method', label: 'Method', type: 'select', options: ['GET','POST','PUT','PATCH','DELETE','HEAD'], default: 'GET' },
    { key: 'url', label: 'URL', type: 'text', default: 'https://api.example.com/endpoint' },
    { key: 'body_type', label: 'Body Type', type: 'select', options: ['none','json','form','raw'], default: 'none' },
    { key: 'body', label: 'Body (JSON)', type: 'textarea', default: '' },
    { key: 'auth_type', label: 'Auth', type: 'select', options: ['none','bearer','basic','api_key'], default: 'none' },
    { key: 'auth_api_key_value', label: 'API Key / Bearer Token', type: 'password', default: '' },
    { key: 'response_format', label: 'Response Format', type: 'select', options: ['json','text','binary'], default: 'json' },
  ],
  'http.ftp': [
    { key: 'host', label: 'Host', type: 'text', default: '' },
    { key: 'port', label: 'Port', type: 'number', default: '21' },
    { key: 'username', label: 'Username', type: 'text', default: '' },
    { key: 'password', label: 'Password', type: 'password', default: '' },
    { key: 'remote_path', label: 'Remote Path', type: 'text', default: '/' },
    { key: 'operation', label: 'Operation', type: 'select', options: ['list','download','upload','delete'], default: 'list' },
  ],
  'http.ssh': [
    { key: 'host', label: 'Host', type: 'text', default: '' },
    { key: 'port', label: 'Port', type: 'number', default: '22' },
    { key: 'username', label: 'Username', type: 'text', default: '' },
    { key: 'password', label: 'Password', type: 'password', default: '' },
    { key: 'command', label: 'Command', type: 'textarea', default: 'echo hello' },
  ],

  // ── System ────────────────────────────────────────────────────────────────
  'system.execute_command': [
    { key: 'command', label: 'Command', type: 'text', default: 'echo' },
    { key: 'args', label: 'Arguments (JSON array)', type: 'text', default: '["hello world"]' },
    { key: 'working_dir', label: 'Working Directory', type: 'text', default: '' },
    { key: 'timeout_seconds', label: 'Timeout (seconds)', type: 'number', default: '30' },
  ],
  'system.rss_read': [
    { key: 'url', label: 'Feed URL', type: 'text', default: 'https://feeds.example.com/rss' },
    { key: 'limit', label: 'Max Items', type: 'number', default: '20' },
  ],

  // ── Data ───────────────────────────────────────────────────────────────────
  'data.datetime': [
    { key: 'operation', label: 'Operation', type: 'select', options: ['format','parse','add','subtract','diff','now'], default: 'now' },
    { key: 'field', label: 'Source Field', type: 'text', default: 'date' },
    { key: 'input_format', label: 'Input Format', type: 'text', default: '' },
    { key: 'output_format', label: 'Output Format', type: 'text', default: '2006-01-02T15:04:05Z07:00' },
    { key: 'duration', label: 'Duration (e.g. 24h, 30m)', type: 'text', default: '' },
    { key: 'output_field', label: 'Output Field', type: 'text', default: '' },
  ],
  'data.crypto': [
    { key: 'operation', label: 'Operation', type: 'select', options: ['md5','sha256','sha512','hmac_sha256','uuid','random_bytes','base64_encode','base64_decode'], default: 'sha256' },
    { key: 'field', label: 'Source Field', type: 'text', default: '' },
    { key: 'key', label: 'HMAC Key', type: 'password', default: '' },
    { key: 'encoding', label: 'Output Encoding', type: 'select', options: ['hex','base64'], default: 'hex' },
  ],
  'data.html': [
    { key: 'operation', label: 'Operation', type: 'select', options: ['extract','extract_all','text','generate'], default: 'extract' },
    { key: 'field', label: 'Source Field', type: 'text', default: '' },
    { key: 'selector', label: 'CSS Selector', type: 'text', default: '' },
    { key: 'attribute', label: 'Attribute', type: 'text', default: '' },
    { key: 'template', label: 'HTML Template', type: 'textarea', default: '' },
  ],
  'data.xml': [
    { key: 'operation', label: 'Operation', type: 'select', options: ['parse','generate'], default: 'parse' },
    { key: 'field', label: 'Source Field', type: 'text', default: '' },
    { key: 'root_element', label: 'Root Element', type: 'text', default: 'root' },
  ],
  'data.markdown': [
    { key: 'field', label: 'Source Field', type: 'text', default: '' },
    { key: 'output_field', label: 'Output Field', type: 'text', default: '' },
  ],
  'data.spreadsheet': [
    { key: 'operation', label: 'Operation', type: 'select', options: ['read_csv','write_csv','read_xlsx','write_xlsx'], default: 'read_csv' },
    { key: 'file_path', label: 'File Path', type: 'text', default: '' },
    { key: 'sheet', label: 'Sheet Name', type: 'text', default: 'Sheet1' },
    { key: 'has_header', label: 'Has Header Row', type: 'select', options: ['true','false'], default: 'true' },
  ],
  'data.compression': [
    { key: 'operation', label: 'Operation', type: 'select', options: ['gzip_compress','gzip_decompress','zip_compress','zip_decompress'], default: 'gzip_compress' },
    { key: 'field', label: 'Source Field (base64)', type: 'text', default: '' },
    { key: 'filename', label: 'Filename (for zip)', type: 'text', default: 'data' },
  ],
  'data.write_binary_file': [
    { key: 'file_path', label: 'Output File Path', type: 'text', default: '' },
    { key: 'field', label: 'Source Field (base64)', type: 'text', default: '' },
  ],

  // ── Database ──────────────────────────────────────────────────────────────
  'db.mysql': [
    { key: 'host', label: 'Host', type: 'text', default: 'localhost' },
    { key: 'port', label: 'Port', type: 'number', default: '3306' },
    { key: 'database', label: 'Database', type: 'text', default: '' },
    { key: 'username', label: 'Username', type: 'text', default: 'root' },
    { key: 'password', label: 'Password', type: 'password', default: '' },
    { key: 'operation', label: 'Operation', type: 'select', options: ['query','insert','update','delete'], default: 'query' },
    { key: 'query', label: 'SQL Query', type: 'textarea', default: 'SELECT * FROM users LIMIT 10' },
  ],
  'db.postgres': [
    { key: 'host', label: 'Host', type: 'text', default: 'localhost' },
    { key: 'port', label: 'Port', type: 'number', default: '5432' },
    { key: 'database', label: 'Database', type: 'text', default: '' },
    { key: 'username', label: 'Username', type: 'text', default: 'postgres' },
    { key: 'password', label: 'Password', type: 'password', default: '' },
    { key: 'operation', label: 'Operation', type: 'select', options: ['query','insert','update','delete'], default: 'query' },
    { key: 'query', label: 'SQL Query', type: 'textarea', default: 'SELECT * FROM users LIMIT 10' },
  ],
  'db.mongodb': [
    { key: 'connection_string', label: 'Connection String', type: 'text', default: 'mongodb://localhost:27017' },
    { key: 'database', label: 'Database', type: 'text', default: '' },
    { key: 'collection', label: 'Collection', type: 'text', default: '' },
    { key: 'operation', label: 'Operation', type: 'select', options: ['find','insertOne','insertMany','updateOne','updateMany','deleteOne','deleteMany','aggregate'], default: 'find' },
    { key: 'filter', label: 'Filter (JSON)', type: 'textarea', default: '{}' },
  ],
  'db.redis': [
    { key: 'addr', label: 'Address', type: 'text', default: 'localhost:6379' },
    { key: 'password', label: 'Password', type: 'password', default: '' },
    { key: 'db', label: 'DB Index', type: 'number', default: '0' },
    { key: 'operation', label: 'Operation', type: 'select', options: ['get','set','del','exists','keys','lpush','rpush','lrange','hset','hget','hgetall'], default: 'get' },
    { key: 'key', label: 'Key', type: 'text', default: '' },
    { key: 'value', label: 'Value', type: 'text', default: '' },
    { key: 'ttl_seconds', label: 'TTL (seconds)', type: 'number', default: '0' },
  ],

  // ── Communication ─────────────────────────────────────────────────────────
  'comm.email_send': [
    { key: 'smtp_host', label: 'SMTP Host', type: 'text', default: 'smtp.gmail.com' },
    { key: 'smtp_port', label: 'SMTP Port', type: 'number', default: '587' },
    { key: 'username', label: 'Username', type: 'text', default: '' },
    { key: 'password', label: 'Password', type: 'password', default: '' },
    { key: 'from', label: 'From', type: 'text', default: '' },
    { key: 'to', label: 'To (comma-separated)', type: 'text', default: '' },
    { key: 'subject', label: 'Subject', type: 'text', default: '' },
    { key: 'body', label: 'Body', type: 'textarea', default: '' },
    { key: 'body_type', label: 'Body Type', type: 'select', options: ['text','html'], default: 'text' },
  ],
  'comm.slack': [
    { key: 'token', label: 'Bot Token', type: 'password', default: '' },
    { key: 'operation', label: 'Operation', type: 'select', options: ['post_message','upload_file','get_user_info'], default: 'post_message' },
    { key: 'channel', label: 'Channel', type: 'text', default: '#general' },
    { key: 'text', label: 'Message Text', type: 'textarea', default: '' },
  ],
  'comm.telegram': [
    { key: 'token', label: 'Bot Token', type: 'password', default: '' },
    { key: 'chat_id', label: 'Chat ID', type: 'text', default: '' },
    { key: 'operation', label: 'Operation', type: 'select', options: ['send_message','send_photo','send_document'], default: 'send_message' },
    { key: 'text', label: 'Message Text', type: 'textarea', default: '' },
    { key: 'parse_mode', label: 'Parse Mode', type: 'select', options: ['','Markdown','HTML'], default: '' },
  ],
  'comm.discord': [
    { key: 'webhook_url', label: 'Webhook URL', type: 'text', default: '' },
    { key: 'content', label: 'Message', type: 'textarea', default: '' },
    { key: 'username', label: 'Username Override', type: 'text', default: '' },
  ],
  'comm.twilio': [
    { key: 'account_sid', label: 'Account SID', type: 'text', default: '' },
    { key: 'auth_token', label: 'Auth Token', type: 'password', default: '' },
    { key: 'from', label: 'From Number', type: 'text', default: '' },
    { key: 'to', label: 'To Number', type: 'text', default: '' },
    { key: 'body', label: 'Message Body', type: 'textarea', default: '' },
  ],
  'comm.email_read': [
    { key: 'imap_host', label: 'IMAP Host', type: 'text', default: 'imap.gmail.com' },
    { key: 'imap_port', label: 'IMAP Port', type: 'number', default: '993' },
    { key: 'username', label: 'Username', type: 'text', default: '' },
    { key: 'password', label: 'Password', type: 'password', default: '' },
    { key: 'mailbox', label: 'Mailbox', type: 'text', default: 'INBOX' },
    { key: 'limit', label: 'Max Messages', type: 'number', default: '10' },
    { key: 'unread_only', label: 'Unread Only', type: 'select', options: ['true','false'], default: 'false' },
  ],
  'comm.whatsapp': [
    { key: 'access_token', label: 'WhatsApp API Token', type: 'password', default: '' },
    { key: 'phone_number_id', label: 'Phone Number ID', type: 'text', default: '' },
    { key: 'to', label: 'To (E.164 number)', type: 'text', default: '' },
    { key: 'operation', label: 'Operation', type: 'select', options: ['send_message','send_template','send_media'], default: 'send_message' },
    { key: 'text', label: 'Message Text', type: 'textarea', default: '' },
    { key: 'template_name', label: 'Template Name', type: 'text', default: '' },
    { key: 'template_language', label: 'Template Language', type: 'text', default: 'en_US' },
    { key: 'media_url', label: 'Media URL', type: 'text', default: '' },
  ],

  // ── Services ──────────────────────────────────────────────────────────────
  'service.github': [
    { key: 'token', label: 'Personal Access Token', type: 'password', default: '' },
    { key: 'owner', label: 'Owner (user/org)', type: 'text', default: '' },
    { key: 'repo', label: 'Repository', type: 'text', default: '' },
    { key: 'operation', label: 'Operation', type: 'select', options: ['list_issues','get_issue','create_issue','update_issue','list_prs','list_releases','create_release'], default: 'list_issues' },
    { key: 'number', label: 'Issue / PR Number', type: 'number', default: '' },
    { key: 'title', label: 'Title', type: 'text', default: '' },
    { key: 'body', label: 'Body', type: 'textarea', default: '' },
    { key: 'state', label: 'State Filter', type: 'select', options: ['','open','closed','all'], default: '' },
  ],
  'service.notion': [
    { key: 'token', label: 'Integration Token', type: 'password', default: '' },
    { key: 'operation', label: 'Operation', type: 'select', options: ['get_page','create_page','update_page','query_database','create_database','append_blocks'], default: 'get_page' },
    { key: 'page_id', label: 'Page ID', type: 'text', default: '' },
    { key: 'database_id', label: 'Database ID', type: 'text', default: '' },
    { key: 'parent_id', label: 'Parent ID', type: 'text', default: '' },
  ],
  'service.airtable': [
    { key: 'token', label: 'Personal Access Token', type: 'password', default: '' },
    { key: 'base_id', label: 'Base ID', type: 'text', default: '' },
    { key: 'table', label: 'Table Name', type: 'text', default: '' },
    { key: 'operation', label: 'Operation', type: 'select', options: ['list','get','create','update','delete'], default: 'list' },
    { key: 'record_id', label: 'Record ID', type: 'text', default: '' },
    { key: 'filter_formula', label: 'Filter Formula', type: 'text', default: '' },
    { key: 'max_records', label: 'Max Records', type: 'number', default: '100' },
    { key: 'view', label: 'View Name', type: 'text', default: '' },
  ],
  'service.google_sheets': [
    { key: 'access_token', label: 'OAuth Access Token', type: 'password', default: '' },
    { key: 'spreadsheet_id', label: 'Spreadsheet ID', type: 'text', default: '' },
    { key: 'sheet', label: 'Sheet Name', type: 'text', default: 'Sheet1' },
    { key: 'operation', label: 'Operation', type: 'select', options: ['read','append','update','clear'], default: 'read' },
    { key: 'range', label: 'Range (e.g. A1:Z)', type: 'text', default: 'A1:Z' },
    { key: 'use_header_row', label: 'Use Header Row', type: 'select', options: ['true','false'], default: 'true' },
    { key: 'value_input_option', label: 'Value Input', type: 'select', options: ['RAW','USER_ENTERED'], default: 'USER_ENTERED' },
  ],
  'service.gmail': [
    { key: 'access_token', label: 'OAuth Access Token', type: 'password', default: '' },
    { key: 'operation', label: 'Operation', type: 'select', options: ['send','list','read','trash'], default: 'send' },
    { key: 'to', label: 'To', type: 'text', default: '' },
    { key: 'subject', label: 'Subject', type: 'text', default: '' },
    { key: 'body', label: 'Body', type: 'textarea', default: '' },
    { key: 'body_type', label: 'Body Type', type: 'select', options: ['text','html'], default: 'text' },
    { key: 'query', label: 'Search Query', type: 'text', default: '' },
    { key: 'max_results', label: 'Max Results', type: 'number', default: '20' },
  ],
  'service.google_drive': [
    { key: 'access_token', label: 'OAuth Access Token', type: 'password', default: '' },
    { key: 'operation', label: 'Operation', type: 'select', options: ['list','download','upload','delete','create_folder'], default: 'list' },
    { key: 'file_id', label: 'File ID', type: 'text', default: '' },
    { key: 'folder_id', label: 'Folder ID', type: 'text', default: '' },
    { key: 'query', label: 'Search Query', type: 'text', default: '' },
  ],
  'service.jira': [
    { key: 'base_url', label: 'Jira Base URL', type: 'text', default: 'https://yourorg.atlassian.net' },
    { key: 'email', label: 'Email', type: 'text', default: '' },
    { key: 'api_token', label: 'API Token', type: 'password', default: '' },
    { key: 'operation', label: 'Operation', type: 'select', options: ['list_issues','get_issue','create_issue','update_issue','add_comment'], default: 'list_issues' },
    { key: 'project_key', label: 'Project Key', type: 'text', default: '' },
    { key: 'issue_key', label: 'Issue Key', type: 'text', default: '' },
    { key: 'issue_type', label: 'Issue Type', type: 'text', default: 'Task' },
    { key: 'summary', label: 'Summary', type: 'text', default: '' },
    { key: 'description', label: 'Description', type: 'textarea', default: '' },
    { key: 'jql', label: 'JQL Query', type: 'text', default: '' },
    { key: 'comment', label: 'Comment', type: 'textarea', default: '' },
  ],
  'service.linear': [
    { key: 'token', label: 'API Key', type: 'password', default: '' },
    { key: 'operation', label: 'Operation', type: 'select', options: ['list_issues','get_issue','create_issue','update_issue','list_teams','list_cycles'], default: 'list_issues' },
    { key: 'team_id', label: 'Team ID', type: 'text', default: '' },
    { key: 'issue_id', label: 'Issue ID', type: 'text', default: '' },
    { key: 'title', label: 'Title', type: 'text', default: '' },
    { key: 'description', label: 'Description', type: 'textarea', default: '' },
  ],
  'service.asana': [
    { key: 'token', label: 'Personal Access Token', type: 'password', default: '' },
    { key: 'operation', label: 'Operation', type: 'select', options: ['list_tasks','get_task','create_task','update_task','list_projects'], default: 'list_tasks' },
    { key: 'project_id', label: 'Project ID', type: 'text', default: '' },
    { key: 'task_id', label: 'Task ID', type: 'text', default: '' },
    { key: 'name', label: 'Task Name', type: 'text', default: '' },
    { key: 'notes', label: 'Notes', type: 'textarea', default: '' },
  ],
  'service.stripe': [
    { key: 'api_key', label: 'Secret Key', type: 'password', default: '' },
    { key: 'operation', label: 'Operation', type: 'select', options: ['list_customers','get_customer','create_customer','list_charges','create_charge','list_subscriptions'], default: 'list_customers' },
    { key: 'customer_id', label: 'Customer ID', type: 'text', default: '' },
    { key: 'limit', label: 'Limit', type: 'number', default: '20' },
  ],
  'service.shopify': [
    { key: 'shop_domain', label: 'Shop Domain', type: 'text', default: 'yourshop.myshopify.com' },
    { key: 'access_token', label: 'Admin API Token', type: 'password', default: '' },
    { key: 'operation', label: 'Operation', type: 'select', options: ['list_orders','get_order','list_products','get_product','list_customers'], default: 'list_orders' },
    { key: 'limit', label: 'Limit', type: 'number', default: '50' },
    { key: 'status', label: 'Status Filter', type: 'text', default: '' },
  ],
  'service.salesforce': [
    { key: 'instance_url', label: 'Instance URL', type: 'text', default: 'https://yourorg.salesforce.com' },
    { key: 'access_token', label: 'Access Token', type: 'password', default: '' },
    { key: 'operation', label: 'Operation', type: 'select', options: ['query','get','create','update','delete'], default: 'query' },
    { key: 'object_type', label: 'Object Type', type: 'text', default: 'Contact' },
    { key: 'soql', label: 'SOQL Query', type: 'textarea', default: 'SELECT Id, Name FROM Contact LIMIT 10' },
    { key: 'record_id', label: 'Record ID', type: 'text', default: '' },
  ],
  'service.hubspot': [
    { key: 'access_token', label: 'Private App Token', type: 'password', default: '' },
    { key: 'operation', label: 'Operation', type: 'select', options: ['list_contacts','get_contact','create_contact','update_contact','list_deals','create_deal'], default: 'list_contacts' },
    { key: 'contact_id', label: 'Contact ID', type: 'text', default: '' },
    { key: 'limit', label: 'Limit', type: 'number', default: '50' },
  ],

  // ── AI ──────────────────────────────────────────────────────────────────
  'ai.chat': [
    { key: 'provider_id', label: 'AI Provider', type: 'text', default: '' },
    { key: 'model', label: 'Model', type: 'text', default: '' },
    { key: 'system_prompt', label: 'System Prompt', type: 'textarea', default: '' },
    { key: 'prompt', label: 'Prompt', type: 'textarea', default: '{{$json.text}}' },
    { key: 'temperature', label: 'Temperature', type: 'text', default: '0.7' },
    { key: 'max_tokens', label: 'Max Tokens', type: 'number', default: '1024' },
    { key: 'output_key', label: 'Output Key', type: 'text', default: 'ai_response' },
  ],
  'ai.extract': [
    { key: 'provider_id', label: 'AI Provider', type: 'text', default: '' },
    { key: 'model', label: 'Model', type: 'text', default: '' },
    { key: 'prompt', label: 'Extraction Prompt', type: 'textarea', default: '' },
    { key: 'output_schema', label: 'Output Schema (JSON)', type: 'textarea', default: '' },
    { key: 'temperature', label: 'Temperature', type: 'text', default: '0.2' },
    { key: 'max_tokens', label: 'Max Tokens', type: 'number', default: '1024' },
    { key: 'output_key', label: 'Output Key', type: 'text', default: 'extracted' },
  ],
  'ai.classify': [
    { key: 'provider_id', label: 'AI Provider', type: 'text', default: '' },
    { key: 'model', label: 'Model', type: 'text', default: '' },
    { key: 'categories', label: 'Categories (comma-separated)', type: 'text', default: '' },
    { key: 'prompt_template', label: 'Custom Prompt', type: 'textarea', default: '' },
    { key: 'temperature', label: 'Temperature', type: 'text', default: '0.3' },
    { key: 'max_tokens', label: 'Max Tokens', type: 'number', default: '256' },
  ],
  'ai.transform': [
    { key: 'provider_id', label: 'AI Provider', type: 'text', default: '' },
    { key: 'model', label: 'Model', type: 'text', default: '' },
    { key: 'instruction', label: 'Instruction', type: 'textarea', default: '' },
    { key: 'input_field', label: 'Input Field', type: 'text', default: '' },
    { key: 'temperature', label: 'Temperature', type: 'text', default: '0.5' },
    { key: 'max_tokens', label: 'Max Tokens', type: 'number', default: '1024' },
    { key: 'output_key', label: 'Output Key', type: 'text', default: 'transformed' },
  ],
  'ai.embed': [
    { key: 'provider_id', label: 'AI Provider', type: 'text', default: '' },
    { key: 'model', label: 'Model', type: 'text', default: '' },
    { key: 'input_field', label: 'Input Field', type: 'text', default: '' },
    { key: 'output_key', label: 'Output Key', type: 'text', default: 'embedding' },
  ],
  'ai.agent': [
    { key: 'provider_id', label: 'AI Provider', type: 'text', default: '' },
    { key: 'model', label: 'Model', type: 'text', default: '' },
    { key: 'goal', label: 'Goal', type: 'textarea', default: '' },
    { key: 'max_steps', label: 'Max Steps', type: 'number', default: '5' },
    { key: 'temperature', label: 'Temperature', type: 'text', default: '0.7' },
    { key: 'max_tokens', label: 'Max Tokens', type: 'number', default: '2048' },
  ],

  // ── Browser (social) ──────────────────────────────────────────────────────
  'instagram.find_by_keyword': [
    { key: 'keywords', label: 'Keywords (comma-separated)', type: 'text', default: '' },
    { key: 'limit', label: 'Max Results', type: 'number', default: '50' },
    { key: 'username', label: 'Account Username', type: 'text', default: '' },
    { key: 'message', label: 'DM Template', type: 'textarea', default: '' },
  ],
  'instagram.send_dms': [
    { key: 'username', label: 'Account Username', type: 'text', default: '' },
    { key: 'message', label: 'Message Template', type: 'textarea', default: 'Hi {{name}},' },
    { key: 'targets', label: 'Target Usernames (JSON array)', type: 'textarea', default: '["user1","user2"]' },
  ],
  'instagram.follow_users': [
    { key: 'username', label: 'Account Username', type: 'text', default: '' },
    { key: 'targets', label: 'Target Usernames (JSON array)', type: 'textarea', default: '["user1","user2"]' },
    { key: 'limit', label: 'Max Follows', type: 'number', default: '20' },
  ],
  'instagram.publish_post': [
    { key: 'username', label: 'Account Username', type: 'text', default: '' },
    { key: 'caption', label: 'Caption', type: 'textarea', default: '' },
    { key: 'image_path', label: 'Image Path / URL', type: 'text', default: '' },
  ],
  'linkedin.find_by_keyword': [
    { key: 'keywords', label: 'Keywords', type: 'text', default: '' },
    { key: 'limit', label: 'Max Results', type: 'number', default: '50' },
    { key: 'username', label: 'Account Username', type: 'text', default: '' },
  ],
  'linkedin.send_dms': [
    { key: 'username', label: 'Account Username', type: 'text', default: '' },
    { key: 'message', label: 'Message Template', type: 'textarea', default: 'Hi {{name}},' },
    { key: 'targets', label: 'Target Profiles (JSON array)', type: 'textarea', default: '[]' },
  ],
  'x.find_by_keyword': [
    { key: 'keywords', label: 'Keywords / Hashtags', type: 'text', default: '' },
    { key: 'limit', label: 'Max Results', type: 'number', default: '50' },
    { key: 'username', label: 'Account Username', type: 'text', default: '' },
  ],
  'x.publish_post': [
    { key: 'username', label: 'Account Username', type: 'text', default: '' },
    { key: 'text', label: 'Post Text', type: 'textarea', default: '' },
  ],
  'tiktok.find_by_keyword': [
    { key: 'keywords', label: 'Keywords / Hashtags', type: 'text', default: '' },
    { key: 'limit', label: 'Max Results', type: 'number', default: '50' },
    { key: 'username', label: 'Account Username', type: 'text', default: '' },
  ],
}

// Fallback: generic fields for browser nodes not explicitly listed
const BROWSER_NODE_GENERIC = [
  { key: 'username', label: 'Account Username', type: 'text', default: '' },
  { key: 'targets', label: 'Targets (JSON array)', type: 'textarea', default: '[]' },
  { key: 'limit', label: 'Max Items', type: 'number', default: '20' },
  { key: 'message', label: 'Message / Caption', type: 'textarea', default: '' },
]

function getConfigFields(nodeType) {
  const nt = normalizeNodeType(nodeType)
  if (NODE_CONFIG_FIELDS[nt]) return NODE_CONFIG_FIELDS[nt]
  // Generic fallback for browser/social nodes
  if (nt.startsWith('instagram.') || nt.startsWith('linkedin.') ||
      nt.startsWith('x.') || nt.startsWith('tiktok.')) {
    return BROWSER_NODE_GENERIC
  }
  return []
}

// ── Derive input/output ports from node type string ───────────────────────────
function deriveInputs(type) {
  if (type.startsWith('trigger.')) return []
  return [{ id: 'in', label: 'in' }]
}
function deriveOutputs(type) {
  if (type === 'core.if')                return [{ id: 'true', label: 'true' }, { id: 'false', label: 'false' }]
  if (type === 'core.switch')            return [{ id: 'case0', label: 'case0' }, { id: 'default', label: 'default' }]
  if (type === 'core.split_in_batches')  return [{ id: 'batch', label: 'batch' }, { id: 'done', label: 'done' }]
  if (type === 'core.filter')            return [{ id: 'pass', label: 'pass' }, { id: 'fail', label: 'fail' }]
  if (type === 'core.merge')             return [{ id: 'out', label: 'out' }]
  if (type === 'core.stop_error')        return []
  if (type === 'trigger.webhook')        return [{ id: 'body', label: 'body' }, { id: 'headers', label: 'headers' }]
  if (type === 'system.execute_command') return [{ id: 'stdout', label: 'stdout' }, { id: 'stderr', label: 'stderr' }]
  if (type.startsWith('db.'))            return [{ id: 'rows', label: 'rows' }, { id: 'error', label: 'error' }]
  if (type.startsWith('http.'))          return [{ id: 'out', label: 'out' }, { id: 'error', label: 'error' }]
  return [{ id: 'main', label: 'main' }]
}

const tbBtn = {
  background: 'transparent',
  border: '1px solid rgba(0,180,216,0.15)',
  borderRadius: 6, padding: '4px 8px',
  color: 'var(--text-muted)', cursor: 'pointer',
  display: 'flex', alignItems: 'center',
  transition: 'all 100ms',
}
