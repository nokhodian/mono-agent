import { useState, useEffect, useRef } from 'react'
import {
  X, ZoomIn, ZoomOut, Trash2, Database, Zap,
  ChevronLeft, ChevronRight, Play, RotateCcw, Save, Settings2
} from 'lucide-react'

// ── Layout constants ────────────────────────────────────────────────
const NODE_W    = 236
const HEAD_H    = 48
const PORT_H    = 30
const PORT_PAD  = 10
const PORT_R    = 6.5

// ── Platform accent colors ──────────────────────────────────────────
const PCOL = {
  instagram: '#e1306c',
  linkedin:  '#0a66c2',
  x:         '#8899aa',
  tiktok:    '#ff0050',
  default:   '#00b4d8',
}

// ── Node templates ──────────────────────────────────────────────────
const DATA_TEMPLATES = [
  {
    subtype: 'keywords',
    label: 'Keywords',
    color: '#d97706',
    outputs: [{ id: 'items', label: 'items' }],
    configFields: [{ key: 'items', label: 'Keywords (one per line)', type: 'textarea' }],
  },
  {
    subtype: 'profile_urls',
    label: 'Profile URLs',
    color: '#d97706',
    outputs: [{ id: 'items', label: 'items' }],
    configFields: [{ key: 'items', label: 'Profile URLs (one per line)', type: 'textarea' }],
  },
  {
    subtype: 'post_urls',
    label: 'Post URLs',
    color: '#d97706',
    outputs: [{ id: 'items', label: 'items' }],
    configFields: [{ key: 'items', label: 'Post URLs (one per line)', type: 'textarea' }],
  },
  {
    subtype: 'text_value',
    label: 'Text Value',
    color: '#d97706',
    outputs: [{ id: 'value', label: 'value' }],
    configFields: [{ key: 'value', label: 'Value', type: 'text' }],
  },
]

const ACTION_TEMPLATES = [
  {
    subtype: 'FIND_BY_KEYWORD', label: 'Find by Keyword', platform: 'instagram',
    inputs:  [{ id: 'keywords', label: 'keywords' }],
    outputs: [{ id: 'profiles', label: 'profiles' }, { id: 'errors', label: 'errors' }],
    configFields: [{ key: 'maxCount', label: 'Max Count', type: 'number', default: '50' }],
  },
  {
    subtype: 'POST_LIKING', label: 'Post Liking', platform: 'instagram',
    inputs:  [{ id: 'items', label: 'items' }],
    outputs: [{ id: 'liked', label: 'liked' }, { id: 'errors', label: 'errors' }],
    configFields: [{ key: 'maxCount', label: 'Max Count', type: 'number', default: '100' }],
  },
  {
    subtype: 'POST_COMMENTING', label: 'Post Commenting', platform: 'instagram',
    inputs:  [{ id: 'items', label: 'items' }, { id: 'text', label: 'comment text' }],
    outputs: [{ id: 'commented', label: 'commented' }, { id: 'errors', label: 'errors' }],
    configFields: [{ key: 'commentText', label: 'Comment Text', type: 'text', default: '' }],
  },
  {
    subtype: 'BULK_MESSAGING', label: 'Bulk Messaging', platform: 'instagram',
    inputs:  [{ id: 'profiles', label: 'profiles' }, { id: 'message', label: 'message' }],
    outputs: [{ id: 'sent', label: 'sent' }, { id: 'errors', label: 'errors' }],
    configFields: [{ key: 'messageText', label: 'Message Text', type: 'text', default: '' }],
  },
  {
    subtype: 'BULK_FOLLOWING', label: 'Bulk Following', platform: 'instagram',
    inputs:  [{ id: 'profiles', label: 'profiles' }],
    outputs: [{ id: 'followed', label: 'followed' }, { id: 'errors', label: 'errors' }],
    configFields: [],
  },
  {
    subtype: 'BULK_UNFOLLOWING', label: 'Bulk Unfollowing', platform: 'instagram',
    inputs:  [{ id: 'profiles', label: 'profiles' }],
    outputs: [{ id: 'unfollowed', label: 'unfollowed' }, { id: 'errors', label: 'errors' }],
    configFields: [],
  },
  {
    subtype: 'STORY_VIEWING', label: 'Story Viewing', platform: 'instagram',
    inputs:  [{ id: 'profiles', label: 'profiles' }],
    outputs: [{ id: 'viewed', label: 'viewed' }, { id: 'errors', label: 'errors' }],
    configFields: [],
  },
  {
    subtype: 'POST_SCRAPING', label: 'Post Scraping', platform: 'instagram',
    inputs:  [{ id: 'posts', label: 'posts' }],
    outputs: [{ id: 'data', label: 'data' }, { id: 'errors', label: 'errors' }],
    configFields: [],
  },
  {
    subtype: 'COMMENT_LIKING', label: 'Comment Liking', platform: 'instagram',
    inputs:  [{ id: 'posts', label: 'posts' }],
    outputs: [{ id: 'liked', label: 'liked' }, { id: 'errors', label: 'errors' }],
    configFields: [],
  },
]

// ── Geometry helpers ────────────────────────────────────────────────
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
  return `M${sx},${sy} C${sx + dx},${sy} ${tx - dx},${ty} ${tx},${ty}`
}

// ── ID generator ────────────────────────────────────────────────────
let _seq = 1
const uid = () => `wf${_seq++}`

// ── Load/save from localStorage ─────────────────────────────────────
const LS_KEY = 'monoes-workflow-v1'

function loadWorkflow() {
  try {
    const s = localStorage.getItem(LS_KEY)
    if (!s) return { nodes: [], edges: [] }
    return JSON.parse(s)
  } catch { return { nodes: [], edges: [] } }
}

function saveWorkflow(nodes, edges) {
  try { localStorage.setItem(LS_KEY, JSON.stringify({ nodes, edges })) } catch {}
}

// ══════════════════════════════════════════════════════════════════
// Sub-component: WorkflowNode
// ══════════════════════════════════════════════════════════════════
function WorkflowNode({
  node, selected, zoom,
  onHeaderMouseDown,
  onOutputPortMouseDown,
  onInputPortMouseUp,
  onClick,
  onDelete,
}) {
  const h     = nodeH(node)
  const rows  = Math.max(node.inputs.length, node.outputs.length, 1)
  const color = node.color || PCOL.default

  return (
    <div
      style={{
        position: 'absolute',
        left: node.x,
        top: node.y,
        width: NODE_W,
        height: h,
        background: 'linear-gradient(160deg, #0d1a28 0%, #091220 100%)',
        border: `1px solid ${selected ? color : 'rgba(0,180,216,0.12)'}`,
        borderRadius: 10,
        boxShadow: selected
          ? `0 0 0 1.5px ${color}55, 0 12px 32px rgba(0,0,0,0.7), 0 0 28px ${color}18`
          : '0 6px 20px rgba(0,0,0,0.5)',
        userSelect: 'none',
        overflow: 'visible',
        transition: 'border-color 140ms, box-shadow 140ms',
      }}
      onMouseDown={(e) => { e.stopPropagation(); onClick?.() }}
    >
      {/* ── Header ── */}
      <div
        style={{
          height: HEAD_H,
          background: `linear-gradient(110deg, ${color}1a 0%, ${color}0a 100%)`,
          borderBottom: `1px solid ${color}22`,
          borderRadius: '10px 10px 0 0',
          display: 'flex',
          alignItems: 'center',
          padding: '0 12px 0 10px',
          cursor: 'grab',
          gap: 7,
        }}
        onMouseDown={(e) => { e.stopPropagation(); onHeaderMouseDown(e) }}
      >
        {/* color dot */}
        <div style={{
          width: 7, height: 7, borderRadius: '50%',
          background: color, boxShadow: `0 0 8px ${color}`,
          flexShrink: 0,
        }} />

        {/* title */}
        <span style={{
          flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
          fontFamily: 'var(--font-mono)',
          fontSize: 11, fontWeight: 700,
          color: '#dde6f0',
          letterSpacing: 0.5,
          textTransform: 'uppercase',
        }}>
          {node.label}
        </span>

        {/* platform badge */}
        {node.platform && (
          <span style={{
            fontSize: 9, fontFamily: 'var(--font-mono)',
            padding: '2px 5px', borderRadius: 3,
            background: `${color}22`, color,
            textTransform: 'uppercase', letterSpacing: 1,
          }}>
            {node.platform.slice(0, 2).toUpperCase()}
          </span>
        )}

        {/* type badge */}
        <span style={{
          fontSize: 9, fontFamily: 'var(--font-mono)',
          padding: '2px 5px', borderRadius: 3,
          background: node.type === 'data' ? 'rgba(217,119,6,0.15)' : 'rgba(0,180,216,0.1)',
          color: node.type === 'data' ? '#d97706' : '#00b4d8',
          textTransform: 'uppercase', letterSpacing: 1,
        }}>
          {node.type}
        </span>

        {/* delete button (only when selected) */}
        {selected && (
          <button
            style={{
              background: 'none', border: 'none', cursor: 'pointer',
              color: 'rgba(100,120,140,0.6)', padding: '2px 1px',
              display: 'flex', borderRadius: 3, flexShrink: 0,
            }}
            onMouseDown={(e) => e.stopPropagation()}
            onClick={(e) => { e.stopPropagation(); onDelete() }}
            title="Delete node"
          >
            <X size={12} />
          </button>
        )}
      </div>

      {/* ── Port rows ── */}
      <div style={{ padding: `${PORT_PAD}px 0` }}>
        {Array.from({ length: rows }).map((_, i) => {
          const inp = node.inputs[i]
          const out = node.outputs[i]
          return (
            <div key={i} style={{
              height: PORT_H,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
            }}>
              {/* Input port (left side) */}
              <div style={{
                display: 'flex', alignItems: 'center',
                minWidth: '45%', paddingLeft: 0,
              }}>
                {inp ? (
                  <>
                    <div
                      className="wf-port-in"
                      style={{
                        width: PORT_R * 2, height: PORT_R * 2,
                        borderRadius: '50%',
                        background: '#0d1a28',
                        border: '2px solid rgba(0,180,216,0.35)',
                        marginLeft: -PORT_R,
                        cursor: 'crosshair',
                        flexShrink: 0,
                        boxSizing: 'border-box',
                        transition: 'border-color 120ms, box-shadow 120ms',
                        zIndex: 3,
                        position: 'relative',
                      }}
                      onMouseUp={(e) => { e.stopPropagation(); onInputPortMouseUp?.(e, i) }}
                    />
                    <span style={{
                      marginLeft: 8, fontSize: 10,
                      fontFamily: 'var(--font-mono)',
                      color: 'var(--text-muted)',
                      letterSpacing: 0.2,
                    }}>
                      {inp.label}
                    </span>
                  </>
                ) : null}
              </div>

              {/* Output port (right side) */}
              <div style={{
                display: 'flex', alignItems: 'center', justifyContent: 'flex-end',
                minWidth: '45%', paddingRight: 0,
              }}>
                {out ? (
                  <>
                    <span style={{
                      marginRight: 8, fontSize: 10,
                      fontFamily: 'var(--font-mono)',
                      color: 'var(--text-muted)',
                      letterSpacing: 0.2,
                    }}>
                      {out.label}
                    </span>
                    <div
                      className="wf-port-out"
                      style={{
                        width: PORT_R * 2, height: PORT_R * 2,
                        borderRadius: '50%',
                        background: color,
                        marginRight: -PORT_R,
                        cursor: 'crosshair',
                        flexShrink: 0,
                        boxShadow: `0 0 7px ${color}99`,
                        boxSizing: 'border-box',
                        zIndex: 3,
                        position: 'relative',
                        transition: 'box-shadow 120ms',
                      }}
                      onMouseDown={(e) => { e.stopPropagation(); onOutputPortMouseDown?.(e, i) }}
                    />
                  </>
                ) : null}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

// ══════════════════════════════════════════════════════════════════
// Sub-component: NodePalette
// ══════════════════════════════════════════════════════════════════
function NodePalette({ onAdd }) {
  return (
    <div style={{
      width: 210,
      flexShrink: 0,
      background: '#080d16',
      borderRight: '1px solid rgba(0,180,216,0.1)',
      display: 'flex',
      flexDirection: 'column',
      overflow: 'hidden',
    }}>
      {/* Header */}
      <div style={{
        padding: '14px 14px 10px',
        borderBottom: '1px solid rgba(0,180,216,0.08)',
        flexShrink: 0,
      }}>
        <div style={{
          fontFamily: 'var(--font-mono)',
          fontSize: 9,
          color: 'var(--text-muted)',
          letterSpacing: 2,
          textTransform: 'uppercase',
        }}>
          NODE PALETTE
        </div>
      </div>

      <div style={{ overflowY: 'auto', flex: 1, padding: '8px 10px 12px' }}>
        {/* DATA section */}
        <div style={{
          fontFamily: 'var(--font-mono)',
          fontSize: 9, letterSpacing: 2,
          color: '#d97706',
          textTransform: 'uppercase',
          padding: '10px 4px 6px',
          display: 'flex', alignItems: 'center', gap: 6,
        }}>
          <Database size={10} />
          DATA SOURCES
        </div>
        {DATA_TEMPLATES.map(t => (
          <PaletteItem key={t.subtype} template={t} type="data" onAdd={onAdd} />
        ))}

        {/* ACTION section */}
        <div style={{
          fontFamily: 'var(--font-mono)',
          fontSize: 9, letterSpacing: 2,
          color: '#00b4d8',
          textTransform: 'uppercase',
          padding: '14px 4px 6px',
          display: 'flex', alignItems: 'center', gap: 6,
        }}>
          <Zap size={10} />
          ACTIONS
        </div>
        {ACTION_TEMPLATES.map(t => (
          <PaletteItem key={t.subtype} template={t} type="action" onAdd={onAdd} />
        ))}
      </div>
    </div>
  )
}

function PaletteItem({ template, type, onAdd }) {
  const [hov, setHov] = useState(false)
  const color = template.color || (template.platform ? PCOL[template.platform] : PCOL.default)
  return (
    <div
      style={{
        padding: '7px 10px',
        borderRadius: 6,
        marginBottom: 2,
        cursor: 'pointer',
        background: hov ? `${color}12` : 'transparent',
        border: `1px solid ${hov ? `${color}30` : 'transparent'}`,
        transition: 'all 120ms',
        display: 'flex',
        alignItems: 'center',
        gap: 8,
      }}
      onMouseEnter={() => setHov(true)}
      onMouseLeave={() => setHov(false)}
      onClick={() => onAdd(type, template)}
    >
      {/* color swatch */}
      <div style={{
        width: 6, height: 6, borderRadius: '50%',
        background: color, flexShrink: 0,
        boxShadow: hov ? `0 0 6px ${color}` : 'none',
        transition: 'box-shadow 120ms',
      }} />
      <div>
        <div style={{
          fontFamily: 'var(--font-mono)',
          fontSize: 11,
          color: hov ? '#dde6f0' : 'var(--text-secondary)',
          transition: 'color 120ms',
          letterSpacing: 0.3,
        }}>
          {template.label}
        </div>
        {template.platform && (
          <div style={{
            fontSize: 9,
            fontFamily: 'var(--font-mono)',
            color: color,
            opacity: 0.7,
            letterSpacing: 0.5,
            textTransform: 'uppercase',
          }}>
            {template.platform}
          </div>
        )}
      </div>
    </div>
  )
}

// ══════════════════════════════════════════════════════════════════
// Sub-component: NodeInspector
// ══════════════════════════════════════════════════════════════════
function NodeInspector({ node, onUpdateLabel, onUpdateConfig, onDelete, onClose }) {
  const color = node.color || PCOL.default

  return (
    <div style={{
      width: 260,
      flexShrink: 0,
      background: '#080d16',
      borderLeft: '1px solid rgba(0,180,216,0.1)',
      display: 'flex',
      flexDirection: 'column',
      overflow: 'hidden',
    }}>
      {/* Inspector header */}
      <div style={{
        padding: '12px 14px',
        borderBottom: `1px solid ${color}18`,
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        flexShrink: 0,
        background: `${color}08`,
      }}>
        <div style={{
          width: 8, height: 8, borderRadius: '50%',
          background: color, boxShadow: `0 0 8px ${color}`,
          flexShrink: 0,
        }} />
        <div style={{ flex: 1 }}>
          <div style={{
            fontFamily: 'var(--font-mono)',
            fontSize: 9, color: 'var(--text-muted)',
            textTransform: 'uppercase', letterSpacing: 2,
            marginBottom: 2,
          }}>
            {node.type} NODE
          </div>
          <div style={{
            fontFamily: 'var(--font-mono)',
            fontSize: 11, color: '#dde6f0',
            fontWeight: 600,
          }}>
            {node.subtype}
          </div>
        </div>
        <button
          style={{
            background: 'none', border: 'none',
            cursor: 'pointer', color: 'var(--text-muted)',
            padding: 4, display: 'flex', borderRadius: 4,
          }}
          onClick={onClose}
          title="Close inspector"
        >
          <ChevronRight size={14} />
        </button>
      </div>

      {/* Inspector body */}
      <div style={{ overflowY: 'auto', flex: 1, padding: '14px' }}>
        {/* Label field */}
        <InspectorField label="LABEL">
          <input
            style={{
              width: '100%',
              background: '#0d1a28',
              border: '1px solid rgba(0,180,216,0.2)',
              borderRadius: 5,
              padding: '6px 9px',
              color: '#dde6f0',
              fontFamily: 'var(--font-mono)',
              fontSize: 11,
              outline: 'none',
            }}
            value={node.label}
            onChange={(e) => onUpdateLabel(node.id, e.target.value)}
            onFocus={(e) => e.currentTarget.style.borderColor = `${color}60`}
            onBlur={(e) => e.currentTarget.style.borderColor = 'rgba(0,180,216,0.2)'}
          />
        </InspectorField>

        {/* Platform info */}
        {node.platform && (
          <InspectorField label="PLATFORM">
            <div style={{
              display: 'flex', alignItems: 'center', gap: 7,
              padding: '5px 9px',
              background: '#0d1a28',
              borderRadius: 5,
              border: `1px solid ${color}22`,
            }}>
              <div style={{
                width: 6, height: 6, borderRadius: '50%',
                background: color, boxShadow: `0 0 6px ${color}`,
              }} />
              <span style={{
                fontFamily: 'var(--font-mono)', fontSize: 11, color,
                textTransform: 'capitalize',
              }}>
                {node.platform}
              </span>
            </div>
          </InspectorField>
        )}

        {/* Ports overview */}
        {(node.inputs.length > 0 || node.outputs.length > 0) && (
          <InspectorField label="PORTS">
            <div style={{
              background: '#0d1a28',
              border: '1px solid rgba(0,180,216,0.1)',
              borderRadius: 5,
              padding: '8px 9px',
              display: 'flex',
              flexDirection: 'column',
              gap: 5,
            }}>
              {node.inputs.map(p => (
                <div key={p.id} style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  <div style={{
                    width: 5, height: 5, borderRadius: '50%',
                    border: '1.5px solid rgba(0,180,216,0.5)',
                    flexShrink: 0,
                  }} />
                  <span style={{ fontSize: 10, fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
                    ← {p.label}
                  </span>
                </div>
              ))}
              {node.outputs.map(p => (
                <div key={p.id} style={{ display: 'flex', alignItems: 'center', gap: 6, justifyContent: 'flex-end' }}>
                  <span style={{ fontSize: 10, fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
                    {p.label} →
                  </span>
                  <div style={{
                    width: 5, height: 5, borderRadius: '50%',
                    background: color,
                    boxShadow: `0 0 5px ${color}80`,
                    flexShrink: 0,
                  }} />
                </div>
              ))}
            </div>
          </InspectorField>
        )}

        {/* Config fields */}
        {node.configFields?.length > 0 && (
          <>
            <div style={{
              fontFamily: 'var(--font-mono)',
              fontSize: 9, color: 'var(--text-muted)',
              letterSpacing: 2, textTransform: 'uppercase',
              margin: '14px 0 8px',
              paddingTop: 12,
              borderTop: '1px solid rgba(0,180,216,0.07)',
            }}>
              CONFIGURATION
            </div>
            {node.configFields.map(f => (
              <InspectorField key={f.key} label={f.label}>
                {f.type === 'textarea' ? (
                  <textarea
                    rows={5}
                    style={{
                      width: '100%', resize: 'vertical',
                      background: '#0d1a28',
                      border: '1px solid rgba(0,180,216,0.18)',
                      borderRadius: 5,
                      padding: '6px 9px',
                      color: '#dde6f0',
                      fontFamily: 'var(--font-mono)',
                      fontSize: 10,
                      outline: 'none',
                      lineHeight: 1.6,
                    }}
                    value={node.config?.[f.key] ?? ''}
                    onChange={(e) => onUpdateConfig(node.id, f.key, e.target.value)}
                    placeholder={`Enter ${f.label.toLowerCase()}...`}
                    onFocus={(e) => e.currentTarget.style.borderColor = `${color}50`}
                    onBlur={(e) => e.currentTarget.style.borderColor = 'rgba(0,180,216,0.18)'}
                  />
                ) : (
                  <input
                    type={f.type || 'text'}
                    style={{
                      width: '100%',
                      background: '#0d1a28',
                      border: '1px solid rgba(0,180,216,0.18)',
                      borderRadius: 5,
                      padding: '6px 9px',
                      color: '#dde6f0',
                      fontFamily: 'var(--font-mono)',
                      fontSize: 11,
                      outline: 'none',
                    }}
                    value={node.config?.[f.key] ?? ''}
                    onChange={(e) => onUpdateConfig(node.id, f.key, e.target.value)}
                    placeholder={f.default ?? ''}
                    onFocus={(e) => e.currentTarget.style.borderColor = `${color}50`}
                    onBlur={(e) => e.currentTarget.style.borderColor = 'rgba(0,180,216,0.18)'}
                  />
                )}
              </InspectorField>
            ))}
          </>
        )}
      </div>

      {/* Inspector footer: delete */}
      <div style={{
        padding: '10px 14px',
        borderTop: '1px solid rgba(0,180,216,0.07)',
        flexShrink: 0,
      }}>
        <button
          style={{
            width: '100%',
            background: 'rgba(239,68,68,0.08)',
            border: '1px solid rgba(239,68,68,0.2)',
            borderRadius: 6,
            padding: '7px 0',
            color: '#ef4444',
            fontFamily: 'var(--font-mono)',
            fontSize: 11,
            cursor: 'pointer',
            display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6,
            transition: 'background 120ms',
          }}
          onClick={onDelete}
          onMouseEnter={(e) => e.currentTarget.style.background = 'rgba(239,68,68,0.16)'}
          onMouseLeave={(e) => e.currentTarget.style.background = 'rgba(239,68,68,0.08)'}
        >
          <Trash2 size={12} /> Delete Node
        </button>
      </div>
    </div>
  )
}

function InspectorField({ label, children }) {
  return (
    <div style={{ marginBottom: 10 }}>
      <div style={{
        fontFamily: 'var(--font-mono)',
        fontSize: 9, color: 'var(--text-muted)',
        letterSpacing: 1.5, textTransform: 'uppercase',
        marginBottom: 5,
      }}>
        {label}
      </div>
      {children}
    </div>
  )
}

// ══════════════════════════════════════════════════════════════════
// Main: Workflow page
// ══════════════════════════════════════════════════════════════════
export default function Workflow() {
  const initial = loadWorkflow()
  const [nodes, setNodes]     = useState(initial.nodes)
  const [edges, setEdges]     = useState(initial.edges)
  const [selectedId, setSelectedId] = useState(null)
  const [camera, setCamera]   = useState({ x: 80, y: 80, zoom: 1 })
  const [paletteOpen, setPaletteOpen] = useState(true)
  const [pendingEdge, setPendingEdge] = useState(null) // { sx, sy, tx, ty }
  const [saved, setSaved]     = useState(false)

  const wrapperRef  = useRef(null)
  const dragRef     = useRef(null)
  const nodesRef    = useRef(nodes)
  const cameraRef   = useRef(camera)

  useEffect(() => { nodesRef.current = nodes }, [nodes])
  useEffect(() => { cameraRef.current = camera }, [camera])

  const selectedNode = nodes.find(n => n.id === selectedId) || null

  // ── Screen ↔ world coordinate conversion ─────────────────────
  const toWorld = (clientX, clientY) => {
    const rect = wrapperRef.current?.getBoundingClientRect() || { left: 0, top: 0 }
    const cam  = cameraRef.current
    return {
      x: (clientX - rect.left - cam.x) / cam.zoom,
      y: (clientY - rect.top  - cam.y) / cam.zoom,
    }
  }

  // ── Global mouse handlers (document-level to handle mouse-leave) ──
  useEffect(() => {
    const onMove = (e) => {
      const d = dragRef.current
      if (!d) return

      if (d.type === 'canvas') {
        const dx = e.clientX - d.startX
        const dy = e.clientY - d.startY
        setCamera(c => ({ ...c, x: d.camX + dx, y: d.camY + dy }))

      } else if (d.type === 'node') {
        const cam = cameraRef.current
        const dx  = (e.clientX - d.startX) / cam.zoom
        const dy  = (e.clientY - d.startY) / cam.zoom
        setNodes(prev => prev.map(n =>
          n.id === d.nodeId
            ? { ...n, x: d.nodeStartX + dx, y: d.nodeStartY + dy }
            : n
        ))

      } else if (d.type === 'edge') {
        const w = toWorld(e.clientX, e.clientY)
        setPendingEdge(pe => pe ? { ...pe, tx: w.x, ty: w.y } : null)
      }
    }

    const onUp = () => {
      if (dragRef.current?.type === 'edge') {
        setPendingEdge(null)
      }
      dragRef.current = null
    }

    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup',   onUp)
    return () => {
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseup',   onUp)
    }
  }, []) // eslint-disable-line

  // ── Scroll to zoom ────────────────────────────────────────────
  useEffect(() => {
    const el = wrapperRef.current
    if (!el) return
    const onWheel = (e) => {
      e.preventDefault()
      const factor = e.deltaY < 0 ? 1.1 : 0.9
      setCamera(c => {
        const newZoom = Math.max(0.25, Math.min(2.5, c.zoom * factor))
        const rect    = el.getBoundingClientRect()
        const mx = e.clientX - rect.left
        const my = e.clientY - rect.top
        return {
          x: mx - (mx - c.x) * (newZoom / c.zoom),
          y: my - (my - c.y) * (newZoom / c.zoom),
          zoom: newZoom,
        }
      })
    }
    el.addEventListener('wheel', onWheel, { passive: false })
    return () => el.removeEventListener('wheel', onWheel)
  }, [])

  // ── Delete key to remove selected node ───────────────────────
  useEffect(() => {
    const onKey = (e) => {
      if ((e.key === 'Delete' || e.key === 'Backspace') &&
          !['INPUT', 'TEXTAREA'].includes(e.target.tagName)) {
        if (selectedId) deleteNode(selectedId)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [selectedId]) // eslint-disable-line

  // ── Canvas mousedown (panning) ────────────────────────────────
  const handleCanvasMouseDown = (e) => {
    if (e.target !== wrapperRef.current && !e.target.dataset.canvasBg) return
    setSelectedId(null)
    dragRef.current = {
      type: 'canvas',
      startX: e.clientX, startY: e.clientY,
      camX: cameraRef.current.x, camY: cameraRef.current.y,
    }
  }

  // ── Node drag ─────────────────────────────────────────────────
  const startNodeDrag = (e, nodeId) => {
    const node = nodesRef.current.find(n => n.id === nodeId)
    if (!node) return
    dragRef.current = {
      type: 'node', nodeId,
      startX: e.clientX, startY: e.clientY,
      nodeStartX: node.x, nodeStartY: node.y,
    }
  }

  // ── Start drawing an edge from output port ────────────────────
  const startEdge = (e, nodeId, portIdx) => {
    const node = nodesRef.current.find(n => n.id === nodeId)
    if (!node) return
    const pos = outPortPos(node, portIdx)
    dragRef.current = { type: 'edge', sourceNodeId: nodeId, sourcePortIdx: portIdx }
    setPendingEdge({ sx: pos.x, sy: pos.y, tx: pos.x, ty: pos.y })
  }

  // ── Complete edge on input port ───────────────────────────────
  const completeEdge = (e, targetNodeId, targetPortIdx) => {
    if (!dragRef.current || dragRef.current.type !== 'edge') return
    const { sourceNodeId, sourcePortIdx } = dragRef.current
    if (sourceNodeId === targetNodeId) { dragRef.current = null; setPendingEdge(null); return }

    const sNode = nodesRef.current.find(n => n.id === sourceNodeId)
    const tNode = nodesRef.current.find(n => n.id === targetNodeId)
    if (!sNode || !tNode) { dragRef.current = null; setPendingEdge(null); return }

    setEdges(prev => {
      const exists = prev.some(
        e => e.source === sourceNodeId && e.sourcePortIdx === sourcePortIdx &&
             e.target === targetNodeId && e.targetPortIdx === targetPortIdx
      )
      if (exists) return prev
      return [...prev, {
        id: uid(),
        source: sourceNodeId, sourcePortIdx,
        sourcePortId: sNode.outputs[sourcePortIdx]?.id,
        target: targetNodeId, targetPortIdx,
        targetPortId: tNode.inputs[targetPortIdx]?.id,
      }]
    })
    dragRef.current = null
    setPendingEdge(null)
  }

  // ── Add node from palette ─────────────────────────────────────
  const addNode = (type, template) => {
    const id     = uid()
    const rect   = wrapperRef.current?.getBoundingClientRect() || { width: 800, height: 600 }
    const cam    = cameraRef.current
    const cx     = (rect.width  / 2 - cam.x) / cam.zoom
    const cy     = (rect.height / 2 - cam.y) / cam.zoom
    const jitter = () => (Math.random() - 0.5) * 120

    const defaults = {}
    template.configFields?.forEach(f => { defaults[f.key] = f.default ?? '' })

    setNodes(prev => [...prev, {
      id, type,
      subtype:     template.subtype,
      label:       template.label,
      platform:    template.platform || null,
      color:       template.color || (template.platform ? PCOL[template.platform] : PCOL.default),
      inputs:      template.inputs  || [],
      outputs:     template.outputs || [],
      configFields: template.configFields || [],
      config:      defaults,
      x: cx - NODE_W / 2 + jitter(),
      y: cy - 60 + jitter(),
    }])
    setSelectedId(id)
  }

  // ── Delete node ───────────────────────────────────────────────
  const deleteNode = (id) => {
    setNodes(prev => prev.filter(n => n.id !== id))
    setEdges(prev => prev.filter(e => e.source !== id && e.target !== id))
    setSelectedId(s => s === id ? null : s)
  }

  // ── Update node config / label ────────────────────────────────
  const updateConfig = (nodeId, key, val) =>
    setNodes(prev => prev.map(n =>
      n.id === nodeId ? { ...n, config: { ...n.config, [key]: val } } : n
    ))

  const updateLabel = (nodeId, label) =>
    setNodes(prev => prev.map(n => n.id === nodeId ? { ...n, label } : n))

  // ── Save to localStorage ──────────────────────────────────────
  const handleSave = () => {
    saveWorkflow(nodes, edges)
    setSaved(true)
    setTimeout(() => setSaved(false), 2000)
  }

  // ── Build edge paths ──────────────────────────────────────────
  const edgePaths = edges.map(edge => {
    const sNode = nodes.find(n => n.id === edge.source)
    const tNode = nodes.find(n => n.id === edge.target)
    if (!sNode || !tNode) return null
    const sp = outPortPos(sNode, edge.sourcePortIdx)
    const tp = inPortPos(tNode, edge.targetPortIdx)
    return { ...edge, path: edgePath(sp.x, sp.y, tp.x, tp.y), color: sNode.color || PCOL.default }
  }).filter(Boolean)

  const pendingPath = pendingEdge
    ? edgePath(pendingEdge.sx, pendingEdge.sy, pendingEdge.tx, pendingEdge.ty)
    : null

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', background: '#04060a', overflow: 'hidden' }}>

      {/* ── TOP TOOLBAR ── */}
      <div style={{
        height: 44,
        flexShrink: 0,
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        padding: '0 14px',
        background: '#080d16',
        borderBottom: '1px solid rgba(0,180,216,0.1)',
        zIndex: 10,
      }}>
        {/* Palette toggle */}
        <button
          style={{
            background: paletteOpen ? 'rgba(0,180,216,0.1)' : 'transparent',
            border: '1px solid rgba(0,180,216,0.2)',
            borderRadius: 6,
            padding: '4px 10px',
            color: paletteOpen ? '#00b4d8' : 'var(--text-muted)',
            fontFamily: 'var(--font-mono)',
            fontSize: 10,
            cursor: 'pointer',
            display: 'flex', alignItems: 'center', gap: 5,
            letterSpacing: 0.5,
            transition: 'all 120ms',
          }}
          onClick={() => setPaletteOpen(p => !p)}
          title={paletteOpen ? 'Hide palette' : 'Show palette'}
        >
          {paletteOpen ? <ChevronLeft size={12} /> : <ChevronRight size={12} />}
          PALETTE
        </button>

        {/* Title */}
        <div style={{
          flex: 1,
          textAlign: 'center',
          fontFamily: 'var(--font-mono)',
          fontSize: 11, fontWeight: 700,
          color: 'var(--text-secondary)',
          letterSpacing: 3,
          textTransform: 'uppercase',
        }}>
          ◈ WORKFLOW BUILDER
        </div>

        {/* Stats */}
        <span style={{
          fontFamily: 'var(--font-mono)',
          fontSize: 10, color: 'var(--text-muted)',
        }}>
          {nodes.length} nodes · {edges.length} edges
        </span>

        {/* Zoom controls */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
          <button
            style={toolbarBtnStyle}
            onClick={() => setCamera(c => ({ ...c, zoom: Math.max(0.25, c.zoom / 1.2) }))}
            title="Zoom out"
          >
            <ZoomOut size={13} />
          </button>
          <span style={{
            fontFamily: 'var(--font-mono)', fontSize: 10,
            color: 'var(--text-muted)', minWidth: 36, textAlign: 'center',
          }}>
            {Math.round(camera.zoom * 100)}%
          </span>
          <button
            style={toolbarBtnStyle}
            onClick={() => setCamera(c => ({ ...c, zoom: Math.min(2.5, c.zoom * 1.2) }))}
            title="Zoom in"
          >
            <ZoomIn size={13} />
          </button>
          <button
            style={toolbarBtnStyle}
            onClick={() => setCamera({ x: 80, y: 80, zoom: 1 })}
            title="Reset view"
          >
            <RotateCcw size={13} />
          </button>
        </div>

        {/* Save */}
        <button
          style={{
            ...toolbarBtnStyle,
            background: saved ? 'rgba(16,185,129,0.15)' : 'rgba(0,180,216,0.08)',
            border: `1px solid ${saved ? 'rgba(16,185,129,0.4)' : 'rgba(0,180,216,0.2)'}`,
            color: saved ? '#10b981' : '#00b4d8',
            padding: '4px 12px',
            gap: 5,
          }}
          onClick={handleSave}
        >
          <Save size={12} />
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: 0.5 }}>
            {saved ? 'SAVED' : 'SAVE'}
          </span>
        </button>
      </div>

      {/* ── MAIN AREA ── */}
      <div style={{ display: 'flex', flex: 1, overflow: 'hidden', position: 'relative' }}>

        {/* Palette */}
        {paletteOpen && <NodePalette onAdd={addNode} />}

        {/* Canvas wrapper */}
        <div
          ref={wrapperRef}
          style={{
            flex: 1,
            position: 'relative',
            overflow: 'hidden',
            cursor: dragRef.current?.type === 'canvas' ? 'grabbing' : 'default',
          }}
          onMouseDown={handleCanvasMouseDown}
        >
          {/* Dot grid */}
          <div
            data-canvas-bg="1"
            style={{
              position: 'absolute', inset: 0,
              backgroundImage: 'radial-gradient(circle, rgba(0,180,216,0.2) 1.2px, transparent 1.2px)',
              backgroundSize: '28px 28px',
              backgroundPosition: `${camera.x % 28}px ${camera.y % 28}px`,
              pointerEvents: 'none',
            }}
          />

          {/* SVG — connections layer (z-index 1, behind nodes) */}
          <svg
            style={{
              position: 'absolute', inset: 0,
              width: '100%', height: '100%',
              overflow: 'visible',
              zIndex: 1,
              pointerEvents: 'none',
            }}
          >
            <defs>
              <filter id="wf-glow">
                <feGaussianBlur stdDeviation="3" result="blur" />
                <feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
              </filter>
            </defs>
            <g transform={`translate(${camera.x} ${camera.y}) scale(${camera.zoom})`}>
              {edgePaths.map(ep => (
                <g key={ep.id}>
                  {/* glow halo */}
                  <path
                    d={ep.path} fill="none"
                    stroke={ep.color} strokeOpacity={0.12}
                    strokeWidth={10} vectorEffect="non-scaling-stroke"
                  />
                  {/* solid line */}
                  <path
                    d={ep.path} fill="none"
                    stroke={ep.color} strokeOpacity={0.7}
                    strokeWidth={1.8} vectorEffect="non-scaling-stroke"
                    style={{ filter: 'url(#wf-glow)' }}
                  />
                  {/* animated flow */}
                  <path
                    d={ep.path} fill="none"
                    stroke={ep.color} strokeOpacity={0.9}
                    strokeWidth={1.2}
                    strokeDasharray="7 12"
                    vectorEffect="non-scaling-stroke"
                    style={{ animation: 'wfFlow 1s linear infinite' }}
                  />
                  {/* invisible thick hit-area for deletion */}
                  <path
                    d={ep.path} fill="none"
                    stroke="transparent" strokeWidth={14}
                    vectorEffect="non-scaling-stroke"
                    style={{ cursor: 'pointer', pointerEvents: 'stroke' }}
                    onClick={(e) => { e.stopPropagation(); setEdges(prev => prev.filter(e2 => e2.id !== ep.id)) }}
                    title="Click to delete connection"
                  />
                </g>
              ))}

              {/* Pending edge while dragging */}
              {pendingPath && (
                <path
                  d={pendingPath} fill="none"
                  stroke="rgba(0,180,216,0.45)" strokeWidth={2}
                  strokeDasharray="5 8"
                  vectorEffect="non-scaling-stroke"
                />
              )}
            </g>
          </svg>

          {/* Nodes layer (z-index 2, above SVG) */}
          <div style={{
            position: 'absolute', inset: 0,
            overflow: 'visible',
            transform: `translate(${camera.x}px, ${camera.y}px) scale(${camera.zoom})`,
            transformOrigin: '0 0',
            zIndex: 2,
          }}>
            {nodes.map(node => (
              <WorkflowNode
                key={node.id}
                node={node}
                selected={node.id === selectedId}
                zoom={camera.zoom}
                onClick={() => setSelectedId(node.id)}
                onHeaderMouseDown={(e) => { setSelectedId(node.id); startNodeDrag(e, node.id) }}
                onOutputPortMouseDown={(e, portIdx) => startEdge(e, node.id, portIdx)}
                onInputPortMouseUp={(e, portIdx) => completeEdge(e, node.id, portIdx)}
                onDelete={() => deleteNode(node.id)}
              />
            ))}
          </div>

          {/* Empty state */}
          {nodes.length === 0 && (
            <div style={{
              position: 'absolute', inset: 0, display: 'flex',
              flexDirection: 'column', alignItems: 'center', justifyContent: 'center',
              pointerEvents: 'none', gap: 14,
            }}>
              <div style={{
                fontSize: 56, opacity: 0.06,
                fontFamily: 'var(--font-mono)',
                color: '#00b4d8',
              }}>⬡⬡⬡</div>
              <div style={{
                fontFamily: 'var(--font-mono)',
                fontSize: 11, color: 'var(--text-muted)',
                letterSpacing: 3, textTransform: 'uppercase',
              }}>
                Select nodes from the palette to build a workflow
              </div>
              <div style={{
                fontSize: 10, color: 'var(--text-dim)',
                fontFamily: 'var(--font-mono)',
                letterSpacing: 1,
              }}>
                Drag • Connect • Configure
              </div>
            </div>
          )}
        </div>

        {/* Inspector panel */}
        {selectedNode && (
          <NodeInspector
            node={selectedNode}
            onUpdateLabel={updateLabel}
            onUpdateConfig={updateConfig}
            onDelete={() => deleteNode(selectedNode.id)}
            onClose={() => setSelectedId(null)}
          />
        )}
      </div>
    </div>
  )
}

const toolbarBtnStyle = {
  background: 'transparent',
  border: '1px solid rgba(0,180,216,0.15)',
  borderRadius: 5,
  padding: '4px 7px',
  color: 'var(--text-muted)',
  cursor: 'pointer',
  display: 'flex', alignItems: 'center', gap: 4,
  transition: 'all 120ms',
}
