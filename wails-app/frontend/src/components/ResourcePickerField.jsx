import { useState, useEffect } from 'react'
import { ListResources, CreateResource } from '../wailsjs/go/main/App'

/**
 * ResourcePickerField — searchable dropdown + expand button for external resources.
 *
 * Props:
 *   field: schema field object (field.resource = { type, create_label, param_field })
 *   value: current selected resource ID
 *   onChange: (newId) => void
 *   credentialId: string
 *   platform: string (e.g. "google_sheets")
 *   nodeConfig: object (full node config, for param_field resolution)
 */
export default function ResourcePickerField({ field, value, onChange, credentialId, platform, nodeConfig }) {
  const [items, setItems] = useState([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)
  const [query, setQuery] = useState('')
  const [expanded, setExpanded] = useState(false)
  const [creating, setCreating] = useState(false)
  const [newName, setNewName] = useState('')
  const [selectedLabel, setSelectedLabel] = useState('')

  const resourceType = field.resource?.type || ''

  // Load items when expanded
  useEffect(() => {
    if (!expanded) return
    if (!credentialId || !platform || !resourceType) return
    setLoading(true)
    setError(null)
    ListResources(platform, resourceType, credentialId, query)
      .then(result => {
        if (result.error) setError(result.error)
        else setItems(result.items || [])
      })
      .catch(e => setError(String(e)))
      .finally(() => setLoading(false))
  }, [expanded, credentialId, platform, resourceType])

  // Re-load when query changes (debounced)
  useEffect(() => {
    if (!expanded || !credentialId || !platform || !resourceType) return
    const timer = setTimeout(() => {
      setLoading(true)
      setError(null)
      ListResources(platform, resourceType, credentialId, query)
        .then(result => {
          if (result.error) setError(result.error)
          else setItems(result.items || [])
        })
        .catch(e => setError(String(e)))
        .finally(() => setLoading(false))
    }, 300)
    return () => clearTimeout(timer)
  }, [query])

  // Resolve selected label from items
  useEffect(() => {
    if (value && items.length > 0) {
      const found = items.find(i => i.id === value)
      if (found) setSelectedLabel(found.name)
    }
  }, [value, items])

  function selectItem(item) {
    onChange(item.id)
    setSelectedLabel(item.name)
    setExpanded(false)
    setQuery('')
  }

  async function handleCreate() {
    if (!newName.trim()) return
    setLoading(true)
    try {
      const result = await CreateResource(platform, resourceType, credentialId, newName.trim())
      if (result.error) { setError(result.error); return }
      if (result.item) {
        setItems(prev => [result.item, ...prev])
        selectItem(result.item)
        setCreating(false)
        setNewName('')
      }
    } catch(e) {
      setError(String(e))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="resource-picker">
      <div className="resource-picker-compact">
        <input
          type="text"
          className="resource-picker-input"
          placeholder={`Search ${resourceType}...`}
          value={query}
          onChange={e => { setQuery(e.target.value); if (!expanded) setExpanded(true) }}
          onFocus={() => setExpanded(true)}
        />
        {selectedLabel && !expanded && (
          <span className="resource-picker-selected" title={value}>{selectedLabel}</span>
        )}
        {!selectedLabel && value && !expanded && (
          <span className="resource-picker-selected" title={value}>{value}</span>
        )}
        <button
          type="button"
          className="resource-picker-expand-btn"
          title="Browse all"
          onClick={() => setExpanded(e => !e)}
        >⊞</button>
      </div>

      {expanded && (
        <div className="resource-browser">
          <div className="resource-browser-header">
            <span className="resource-browser-title">Select {field.label}</span>
            {field.resource?.create_label && (
              <button
                type="button"
                className="resource-create-btn"
                onClick={() => setCreating(c => !c)}
              >
                + {field.resource.create_label}
              </button>
            )}
            <button
              type="button"
              className="resource-browser-close"
              onClick={() => setExpanded(false)}
            >✕</button>
          </div>

          {creating && (
            <div className="resource-create-row">
              <input
                type="text"
                placeholder="Name..."
                value={newName}
                onChange={e => setNewName(e.target.value)}
                onKeyDown={e => { if (e.key === 'Enter') handleCreate() }}
                autoFocus
              />
              <button type="button" onClick={handleCreate} disabled={loading}>Create</button>
              <button type="button" onClick={() => { setCreating(false); setNewName('') }}>Cancel</button>
            </div>
          )}

          {error && <div className="resource-error">{error}</div>}
          {loading && <div className="resource-loading">Loading...</div>}

          {!loading && (
            <ul className="resource-list">
              {items.map(item => (
                <li
                  key={item.id}
                  className={`resource-list-item${item.id === value ? ' selected' : ''}`}
                  onClick={() => selectItem(item)}
                >
                  <span className="resource-item-name">{item.name}</span>
                  {item.metadata?.modified_time && (
                    <span className="resource-item-meta">{item.metadata.modified_time}</span>
                  )}
                </li>
              ))}
              {items.length === 0 && !error && (
                <li className="resource-empty">
                  {credentialId ? 'No results found' : 'No credential selected'}
                </li>
              )}
            </ul>
          )}
        </div>
      )}
    </div>
  )
}
