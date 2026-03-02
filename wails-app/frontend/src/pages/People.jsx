import { useState, useEffect, useCallback, useRef } from 'react'
import { Users, Search, RefreshCw, CheckCircle, ExternalLink, Plus, X, Tag } from 'lucide-react'
import { api, PLATFORMS } from '../services/api.js'

// ── Tag colour palette ────────────────────────────────────────
const TAG_COLORS = [
  '#e1306c', '#f97316', '#eab308', '#10b981', '#00b4d8',
  '#7c3aed', '#a855f7', '#0a66c2', '#00f5d4', '#ef4444',
]

const PLATFORM_PROFILE_URL = {
  INSTAGRAM: (u) => `https://www.instagram.com/${u}/`,
  LINKEDIN:  (u) => `https://www.linkedin.com/in/${u}/`,
  X:         (u) => `https://x.com/${u}`,
  TIKTOK:    (u) => `https://www.tiktok.com/@${u}`,
}

// ── Avatar ────────────────────────────────────────────────────
function Avatar({ username, imageUrl }) {
  const [imgFailed, setImgFailed] = useState(false)
  const initials = (username || '?').slice(0, 2).toUpperCase()
  const pairs = [
    ['#7c3aed','#00b4d8'], ['#00b4d8','#00f5d4'], ['#e1306c','#7c3aed'],
    ['#f97316','#eab308'], ['#10b981','#00b4d8'],
  ]
  const pair = pairs[(username?.charCodeAt(0) || 0) % pairs.length]

  if (imageUrl && !imgFailed) {
    return (
      <div className="person-avatar" style={{ padding: 0, overflow: 'hidden', background: 'var(--elevated)' }}>
        <img src={imageUrl} alt={username} onError={() => setImgFailed(true)}
          style={{ width: '100%', height: '100%', objectFit: 'cover', borderRadius: 'inherit' }} />
      </div>
    )
  }
  return (
    <div className="person-avatar" style={{ background: `linear-gradient(135deg, ${pair[0]}, ${pair[1]})` }}>
      {initials}
    </div>
  )
}

function PlatformBadge({ platform }) {
  return <span className={`badge badge-platform-${(platform || '').toLowerCase()}`}>{platform}</span>
}

// ── Single tag chip (read-only) ───────────────────────────────
function TagChip({ tag }) {
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center',
      padding: '2px 8px', borderRadius: 4,
      background: `${tag.color}1a`,
      border: `1px solid ${tag.color}55`,
      color: tag.color,
      fontSize: 10, fontFamily: 'var(--font-mono)',
      whiteSpace: 'nowrap', maxWidth: 90,
      overflow: 'hidden', textOverflow: 'ellipsis',
      lineHeight: 1.6,
    }}>
      {tag.name}
    </span>
  )
}

// ── Tag Editor popup ──────────────────────────────────────────
function TagEditor({ personId, onClose }) {
  const [tags, setTags]           = useState([])
  const [allTags, setAllTags]     = useState([])
  const [input, setInput]         = useState('')
  const [selColor, setSelColor]   = useState(TAG_COLORS[4])
  const [loading, setLoading]     = useState(true)
  const [suggestions, setSuggestions] = useState([])
  const inputRef = useRef(null)
  const rootRef  = useRef(null)

  // Load person's current tags + all global tags
  useEffect(() => {
    Promise.all([
      api.getPersonTags(personId),
      api.getAllTags(),
    ]).then(([pt, at]) => {
      setTags(pt || [])
      setAllTags(at || [])
      setLoading(false)
    })
  }, [personId])

  // Update suggestions when input changes
  useEffect(() => {
    const q = input.trim().toLowerCase()
    if (!q) { setSuggestions([]); return }
    const already = new Set(tags.map(t => t.id))
    setSuggestions(
      allTags.filter(t =>
        t.name.toLowerCase().includes(q) && !already.has(t.id)
      ).slice(0, 6)
    )
  }, [input, allTags, tags])

  // Close on outside click
  useEffect(() => {
    const handler = (e) => {
      if (rootRef.current && !rootRef.current.contains(e.target)) onClose()
    }
    setTimeout(() => document.addEventListener('mousedown', handler), 0)
    return () => document.removeEventListener('mousedown', handler)
  }, [onClose])

  useEffect(() => { inputRef.current?.focus() }, [])

  const addExisting = async (tag) => {
    if (tags.length >= 10) return
    const result = await api.addPersonTag(personId, tag.name, tag.color)
    if (result) {
      setTags(prev => [...prev.filter(t => t.id !== result.id), result])
    }
    setInput('')
    setSuggestions([])
  }

  const addNew = async () => {
    const name = input.trim()
    if (!name || tags.length >= 10) return
    // Check if exact match exists in allTags
    const exact = allTags.find(t => t.name.toLowerCase() === name.toLowerCase())
    const color = exact ? exact.color : selColor
    const result = await api.addPersonTag(personId, name, color)
    if (result) {
      setTags(prev => [...prev.filter(t => t.id !== result.id), result])
      setAllTags(prev => prev.some(t => t.id === result.id) ? prev : [...prev, result])
    }
    setInput('')
    setSuggestions([])
  }

  const remove = async (tagId) => {
    await api.removePersonTag(personId, tagId)
    setTags(prev => prev.filter(t => t.id !== tagId))
  }

  const isNewTag = input.trim() && !allTags.some(t =>
    t.name.toLowerCase() === input.trim().toLowerCase()
  )

  return (
    <div
      ref={rootRef}
      style={{
        position: 'absolute', zIndex: 1000,
        top: '100%', left: 0,
        marginTop: 4,
        width: 300,
        background: '#0d1520',
        border: '1px solid rgba(0,180,216,0.22)',
        borderRadius: 8,
        boxShadow: '0 12px 40px rgba(0,0,0,0.7), 0 0 0 1px rgba(0,180,216,0.08)',
        overflow: 'hidden',
      }}
      onMouseDown={(e) => e.stopPropagation()}
    >
      {/* Header */}
      <div style={{
        padding: '10px 12px 8px',
        borderBottom: '1px solid rgba(0,180,216,0.08)',
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <Tag size={11} style={{ color: 'var(--cyan)' }} />
          <span style={{
            fontFamily: 'var(--font-mono)', fontSize: 10,
            color: 'var(--text-secondary)', letterSpacing: 1.5, textTransform: 'uppercase',
          }}>
            Tags {tags.length > 0 && <span style={{ color: 'var(--text-muted)' }}>({tags.length}/10)</span>}
          </span>
        </div>
        <button onClick={onClose} style={{
          background: 'none', border: 'none', cursor: 'pointer',
          color: 'var(--text-muted)', padding: 2, display: 'flex',
        }}>
          <X size={12} />
        </button>
      </div>

      <div style={{ padding: '10px 12px' }}>
        {/* Current tags */}
        {loading ? (
          <div style={{ height: 28, display: 'flex', alignItems: 'center' }}>
            <div className="spinner" style={{ width: 14, height: 14 }} />
          </div>
        ) : tags.length > 0 ? (
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 5, marginBottom: 10 }}>
            {tags.map(tag => (
              <span key={tag.id} style={{
                display: 'inline-flex', alignItems: 'center', gap: 4,
                padding: '3px 7px 3px 8px', borderRadius: 4,
                background: `${tag.color}1a`,
                border: `1px solid ${tag.color}55`,
                color: tag.color,
                fontSize: 10, fontFamily: 'var(--font-mono)',
              }}>
                {tag.name}
                <button onClick={() => remove(tag.id)} style={{
                  background: 'none', border: 'none', cursor: 'pointer',
                  color: `${tag.color}99`, padding: 0, lineHeight: 1,
                  display: 'flex', alignItems: 'center',
                }}>
                  <X size={10} />
                </button>
              </span>
            ))}
          </div>
        ) : (
          <div style={{
            fontSize: 11, color: 'var(--text-muted)', marginBottom: 10,
            fontFamily: 'var(--font-mono)',
          }}>
            No tags yet
          </div>
        )}

        {/* Input */}
        {tags.length < 10 && (
          <div style={{ position: 'relative' }}>
            <div style={{
              display: 'flex', gap: 6, alignItems: 'center',
              background: '#080c14',
              border: '1px solid rgba(0,180,216,0.2)',
              borderRadius: 5, padding: '5px 8px',
            }}>
              <input
                ref={inputRef}
                value={input}
                onChange={e => setInput(e.target.value)}
                onKeyDown={e => {
                  if (e.key === 'Enter') { e.preventDefault(); addNew() }
                  if (e.key === 'Escape') onClose()
                }}
                placeholder="Type a tag name…"
                style={{
                  flex: 1, background: 'none', border: 'none', outline: 'none',
                  color: '#dde6f0', fontFamily: 'var(--font-mono)', fontSize: 11,
                }}
              />
              {input.trim() && (
                <button
                  onClick={addNew}
                  style={{
                    background: 'rgba(0,180,216,0.15)',
                    border: '1px solid rgba(0,180,216,0.3)',
                    borderRadius: 3, padding: '2px 7px',
                    color: '#00b4d8', fontFamily: 'var(--font-mono)',
                    fontSize: 10, cursor: 'pointer', whiteSpace: 'nowrap',
                  }}
                >
                  {isNewTag ? 'Create' : 'Add'}
                </button>
              )}
            </div>

            {/* Autocomplete suggestions */}
            {suggestions.length > 0 && (
              <div style={{
                position: 'absolute', top: '100%', left: 0, right: 0,
                background: '#0d1520',
                border: '1px solid rgba(0,180,216,0.2)',
                borderRadius: '0 0 5px 5px', marginTop: -1,
                zIndex: 10, overflow: 'hidden',
              }}>
                {suggestions.map(s => (
                  <div
                    key={s.id}
                    onClick={() => addExisting(s)}
                    style={{
                      display: 'flex', alignItems: 'center', gap: 8,
                      padding: '6px 10px', cursor: 'pointer',
                      transition: 'background 100ms',
                    }}
                    onMouseEnter={e => e.currentTarget.style.background = 'rgba(0,180,216,0.06)'}
                    onMouseLeave={e => e.currentTarget.style.background = 'transparent'}
                  >
                    <div style={{
                      width: 8, height: 8, borderRadius: '50%',
                      background: s.color, boxShadow: `0 0 5px ${s.color}`,
                      flexShrink: 0,
                    }} />
                    <span style={{
                      fontFamily: 'var(--font-mono)', fontSize: 11,
                      color: 'var(--text)', flex: 1,
                    }}>
                      {s.name}
                    </span>
                    <span style={{ fontSize: 9, color: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}>
                      existing
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {/* Color picker — only for new tags */}
        {isNewTag && (
          <div style={{ marginTop: 10 }}>
            <div style={{
              fontFamily: 'var(--font-mono)', fontSize: 9,
              color: 'var(--text-muted)', letterSpacing: 1.5,
              textTransform: 'uppercase', marginBottom: 7,
            }}>
              PICK A COLOR
            </div>
            <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
              {TAG_COLORS.map(c => (
                <button
                  key={c}
                  onClick={() => setSelColor(c)}
                  title={c}
                  style={{
                    width: 22, height: 22, borderRadius: '50%',
                    background: c,
                    border: selColor === c
                      ? `2px solid #fff`
                      : '2px solid transparent',
                    boxShadow: selColor === c
                      ? `0 0 0 1.5px ${c}, 0 0 8px ${c}88`
                      : `0 0 5px ${c}66`,
                    cursor: 'pointer',
                    padding: 0, transition: 'box-shadow 120ms, border 120ms',
                  }}
                />
              ))}
            </div>
            {/* Preview */}
            <div style={{ marginTop: 8, display: 'flex', alignItems: 'center', gap: 6 }}>
              <span style={{ fontSize: 9, fontFamily: 'var(--font-mono)', color: 'var(--text-muted)' }}>
                PREVIEW
              </span>
              <span style={{
                padding: '2px 8px', borderRadius: 4,
                background: `${selColor}1a`,
                border: `1px solid ${selColor}55`,
                color: selColor,
                fontSize: 10, fontFamily: 'var(--font-mono)',
              }}>
                {input.trim() || 'tag name'}
              </span>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

// ── Tags cell in the table ────────────────────────────────────
function TagsCell({ personId, initialTags }) {
  const [open, setOpen]       = useState(false)
  const [tags, setTags]       = useState(initialTags || [])
  const containerRef          = useRef(null)

  // Reload tags when editor closes so the table row refreshes
  const handleClose = async () => {
    setOpen(false)
    const fresh = await api.getPersonTags(personId)
    if (fresh) setTags(fresh)
  }

  const visibleTags = tags.slice(0, 3)
  const extra       = tags.length - visibleTags.length

  return (
    <div ref={containerRef} style={{ position: 'relative', display: 'flex', alignItems: 'center', gap: 4, flexWrap: 'nowrap' }}>
      {visibleTags.map(t => <TagChip key={t.id} tag={t} />)}
      {extra > 0 && (
        <span style={{
          fontSize: 9, fontFamily: 'var(--font-mono)',
          color: 'var(--text-muted)', whiteSpace: 'nowrap',
        }}>
          +{extra}
        </span>
      )}
      <button
        onClick={(e) => { e.stopPropagation(); setOpen(o => !o) }}
        title="Edit tags"
        style={{
          width: 18, height: 18, borderRadius: 4,
          background: open ? 'rgba(0,180,216,0.15)' : 'rgba(0,180,216,0.06)',
          border: '1px solid rgba(0,180,216,0.2)',
          color: open ? '#00b4d8' : 'var(--text-muted)',
          cursor: 'pointer', display: 'flex', alignItems: 'center', justifyContent: 'center',
          flexShrink: 0, transition: 'all 120ms', padding: 0,
        }}
      >
        <Plus size={10} />
      </button>

      {open && (
        <TagEditor personId={personId} onClose={handleClose} />
      )}
    </div>
  )
}

// ── Main People page ──────────────────────────────────────────
export default function People({ onProfile }) {
  const [people, setPeople]   = useState([])
  const [tagsMap, setTagsMap] = useState({})
  const [count, setCount]     = useState(0)
  const [loading, setLoading] = useState(true)
  const [platform, setPlatform] = useState('')
  const [search, setSearch]   = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [offset, setOffset]   = useState(0)
  const LIMIT = 50

  const debounceRef = useRef(null)

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      setDebouncedSearch(search)
      setOffset(0)
    }, 300)
  }, [search])

  const load = useCallback(async () => {
    setLoading(true)
    const [data, total] = await Promise.all([
      api.getPeople(platform, debouncedSearch, LIMIT, offset),
      api.getPeopleCount(platform, debouncedSearch),
    ])
    const rows = data || []
    setPeople(rows)
    setCount(total || 0)

    // Bulk-load tags for all visible people
    if (rows.length > 0) {
      const ids = rows.map(p => p.id)
      const tm  = await api.getPeopleTagsMap(ids)
      setTagsMap(tm || {})
    } else {
      setTagsMap({})
    }
    setLoading(false)
  }, [platform, debouncedSearch, offset])

  useEffect(() => { load() }, [load])

  const handlePlatformChange = (p) => {
    setPlatform(p)
    setOffset(0)
    setSearch('')
    setDebouncedSearch('')
  }

  return (
    <>
      <div className="page-header">
        <div className="page-header-left">
          <div className="page-title">People</div>
          <div className="page-subtitle">Discovered Profiles</div>
        </div>
        <div className="page-header-right">
          <button className="btn btn-ghost btn-sm" onClick={() => setOffset(0)} style={{ gap: 5 }}>
            <RefreshCw size={12} /> Refresh
          </button>
        </div>
      </div>

      <div className="page-body">
        <div className="filters-bar">
          <div style={{ position: 'relative', display: 'flex', alignItems: 'center' }}>
            <Search size={13} style={{ position: 'absolute', left: 10, color: 'var(--text-muted)', pointerEvents: 'none' }} />
            <input
              className="search-input"
              style={{ paddingLeft: 30 }}
              placeholder="Search username or name..."
              value={search}
              onChange={e => setSearch(e.target.value)}
            />
          </div>
          <select className="filter-select" value={platform} onChange={e => handlePlatformChange(e.target.value)}>
            <option value="">All Platforms</option>
            {PLATFORMS.map(p => <option key={p} value={p}>{p}</option>)}
          </select>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)', marginLeft: 'auto' }}>
            {count.toLocaleString()} profile{count !== 1 ? 's' : ''}
          </span>
        </div>

        {loading ? (
          <div className="empty-state"><div className="spinner" /></div>
        ) : people.length === 0 ? (
          <div className="empty-state">
            <div className="empty-state-icon"><Users size={40} /></div>
            <div className="empty-state-title">No profiles found</div>
            <div className="empty-state-desc">
              {debouncedSearch || platform
                ? 'Try adjusting your filters.'
                : 'Run a KEYWORD_SEARCH or PROFILE_SEARCH action to discover profiles.'}
            </div>
          </div>
        ) : (
          <>
            <div style={{ overflowX: 'auto' }}>
              <table className="data-table">
                <thead>
                  <tr>
                    <th>Profile</th>
                    <th>Platform</th>
                    <th>Full Name</th>
                    <th>Tags</th>
                    <th>Followers</th>
                    <th>Job / Category</th>
                    <th>Verified</th>
                    <th>Added</th>
                  </tr>
                </thead>
                <tbody>
                  {people.map(p => {
                    const profileUrl = PLATFORM_PROFILE_URL[p.platform?.toUpperCase()]?.(p.username)
                    return (
                      <tr key={p.id}>
                        <td>
                          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                            <Avatar username={p.username} imageUrl={p.image_url} />
                            <button
                              className="mono profile-link"
                              onClick={() => onProfile?.(p.id)}
                              aria-label={`View profile of @${p.username}`}
                            >
                              @{p.username}
                            </button>
                            {profileUrl && (
                              <button
                                className="btn btn-ghost btn-icon"
                                onClick={() => api.openURL(profileUrl)}
                                title={`Open on ${p.platform}`}
                                style={{ padding: 3, minWidth: 'unset', minHeight: 'unset', opacity: 0.4 }}
                              >
                                <ExternalLink size={11} />
                              </button>
                            )}
                          </div>
                        </td>
                        <td><PlatformBadge platform={p.platform} /></td>
                        <td style={{ color: 'var(--text)' }}>{p.full_name || '—'}</td>

                        {/* Tags column */}
                        <td style={{ minWidth: 140 }}>
                          <TagsCell
                            personId={p.id}
                            initialTags={tagsMap[p.id] || []}
                          />
                        </td>

                        <td className="mono">{p.follower_count || '—'}</td>
                        <td style={{ maxWidth: 160 }} className="truncate">{p.job_title || p.category || '—'}</td>
                        <td>
                          {p.is_verified && (
                            <CheckCircle size={13} style={{ color: 'var(--cyan)' }} />
                          )}
                        </td>
                        <td className="mono text-xs" style={{ color: 'var(--text-muted)' }}>
                          {p.created_at ? p.created_at.slice(0, 10) : '—'}
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>

            {/* Pagination */}
            {count > LIMIT && (
              <div style={{ display: 'flex', justifyContent: 'center', gap: 8, marginTop: 16 }}>
                <button
                  className="btn btn-secondary btn-sm"
                  onClick={() => setOffset(Math.max(0, offset - LIMIT))}
                  disabled={offset === 0}
                >
                  ← Prev
                </button>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)', alignSelf: 'center' }}>
                  {offset + 1}–{Math.min(offset + LIMIT, count)} of {count}
                </span>
                <button
                  className="btn btn-secondary btn-sm"
                  onClick={() => setOffset(offset + LIMIT)}
                  disabled={offset + LIMIT >= count}
                >
                  Next →
                </button>
              </div>
            )}
          </>
        )}
      </div>
    </>
  )
}
