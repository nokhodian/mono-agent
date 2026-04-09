import { useEffect, useRef, useState } from 'react'
import { X, Copy, Trash2, Edit3 } from 'lucide-react'
import * as WailsApp from '../wailsjs/wailsjs/go/main/App'

export default function ImageDetailModal({ image, onClose, onDelete, onRename }) {
  const overlayRef = useRef(null)
  const [imgError, setImgError] = useState(false)

  useEffect(() => {
    const handler = (e) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [onClose])

  if (!image) return null

  const handleDelete = async () => {
    if (!window.confirm(`Delete ${image.id}? This cannot be undone.`)) return
    try {
      await WailsApp.DeleteVaultImage(image.id)
      onDelete(image.id)
      onClose()
    } catch (e) {
      alert('Delete failed: ' + e)
    }
  }

  const copyRef = () => {
    navigator.clipboard.writeText('@' + image.id).catch(() => {})
  }

  const fmtBytes = (b) => {
    if (b < 1024) return b + ' B'
    if (b < 1024 * 1024) return (b / 1024).toFixed(1) + ' KB'
    return (b / 1024 / 1024).toFixed(1) + ' MB'
  }

  const fmtDate = (s) => {
    if (!s) return '—'
    const d = new Date(s.includes('T') ? s : s.replace(' ', 'T'))
    return isNaN(d) ? s : d.toLocaleDateString()
  }

  return (
    <div
      ref={overlayRef}
      onClick={(e) => { if (e.target === overlayRef.current) onClose() }}
      style={{
        position: 'fixed', inset: 0, zIndex: 1000,
        background: 'rgba(0,0,0,0.75)',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
      }}
    >
      <div style={{
        background: '#0d1a26', border: '1px solid #1e3a4f', borderRadius: 12,
        padding: 20, width: 420, maxWidth: '90vw',
        display: 'flex', flexDirection: 'column', gap: 14,
        boxShadow: '0 20px 60px rgba(0,0,0,0.6)',
      }}>
        {/* Header */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 13, color: '#00b4d8' }}>
            {image.id}{image.label ? ` · ${image.label}` : ''}
          </span>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#475569', padding: 2 }}>
            <X size={16} />
          </button>
        </div>

        {/* Image preview */}
        <div style={{
          background: '#060b11', borderRadius: 8, overflow: 'hidden',
          border: '1px solid #1e3a4f', minHeight: 120, maxHeight: 280,
          display: 'flex', alignItems: 'center', justifyContent: 'center',
        }}>
          {imgError ? (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 6, color: '#334155', padding: 20 }}>
              <span style={{ fontSize: 28 }}>🖼</span>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10 }}>Image unavailable</span>
            </div>
          ) : (
            <img
              src={image.url}
              alt={image.label || image.id}
              style={{ maxWidth: '100%', maxHeight: 280, objectFit: 'contain', display: 'block' }}
              onError={() => setImgError(true)}
            />
          )}
        </div>

        {/* Metadata */}
        <div style={{ display: 'flex', gap: 16, fontFamily: 'var(--font-mono)', fontSize: 10, color: '#475569', flexWrap: 'wrap' }}>
          <span style={{ color: '#a78bfa' }}>{image.source}</span>
          <span>{fmtBytes(image.size_bytes)}</span>
          <span>{fmtDate(image.created_at)}</span>
          {image.workflow_id && <span style={{ color: '#64748b' }}>{image.workflow_id}</span>}
        </div>

        {/* Actions */}
        <div style={{ display: 'flex', gap: 8 }}>
          <button
            onClick={copyRef}
            style={{
              flex: 1, background: '#0a1829', border: '1px solid rgba(0,180,216,0.3)',
              borderRadius: 6, padding: '7px 12px', color: '#00b4d8',
              fontFamily: 'var(--font-mono)', fontSize: 11, cursor: 'pointer',
              display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 5,
            }}
          >
            <Copy size={12} /> Copy @{image.id}
          </button>
          {onRename && (
            <button
              onClick={() => onRename(image)}
              style={{
                background: '#0a1829', border: '1px solid #1e3a4f',
                borderRadius: 6, padding: '7px 10px', color: '#475569',
                cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 4,
                fontFamily: 'var(--font-mono)', fontSize: 11,
              }}
            >
              <Edit3 size={12} /> Rename
            </button>
          )}
          <button
            onClick={handleDelete}
            style={{
              background: 'rgba(239,68,68,0.08)', border: '1px solid rgba(239,68,68,0.3)',
              borderRadius: 6, padding: '7px 10px', color: '#ef4444',
              cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 4,
              fontFamily: 'var(--font-mono)', fontSize: 11,
            }}
          >
            <Trash2 size={12} /> Delete
          </button>
        </div>
      </div>
    </div>
  )
}
