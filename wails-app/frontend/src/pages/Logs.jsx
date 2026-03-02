import { useEffect, useRef, useState } from 'react'
import { Terminal, Trash2, ArrowDown } from 'lucide-react'

const LEVEL_COLORS = {
  INFO:   { level: 'log-level-info',   msg: 'log-msg-info' },
  WARN:   { level: 'log-level-warn',   msg: 'log-msg-warn' },
  WARNING:{ level: 'log-level-warn',   msg: 'log-msg-warn' },
  ERROR:  { level: 'log-level-error',  msg: 'log-msg-error' },
  SYSTEM: { level: 'log-level-system', msg: 'log-msg-system' },
}

export default function Logs({ logs, onClear }) {
  const endRef = useRef(null)
  const containerRef = useRef(null)
  const [autoScroll, setAutoScroll] = useState(true)
  const [filter, setFilter] = useState('')

  useEffect(() => {
    if (autoScroll && endRef.current) {
      endRef.current.scrollIntoView({ behavior: 'smooth' })
    }
  }, [logs, autoScroll])

  const handleScroll = () => {
    if (!containerRef.current) return
    const { scrollTop, scrollHeight, clientHeight } = containerRef.current
    const nearBottom = scrollHeight - scrollTop - clientHeight < 80
    setAutoScroll(nearBottom)
  }

  const filtered = filter
    ? logs.filter(l =>
        l.message?.toLowerCase().includes(filter.toLowerCase()) ||
        l.source?.toLowerCase().includes(filter.toLowerCase()) ||
        l.level?.toLowerCase().includes(filter.toLowerCase())
      )
    : logs

  return (
    <>
      <div className="page-header">
        <div className="page-header-left">
          <div className="page-title">Live Logs</div>
          <div className="page-subtitle">Real-time Execution</div>
        </div>
        <div className="page-header-right">
          {!autoScroll && (
            <button
              className="btn btn-secondary btn-sm"
              onClick={() => {
                setAutoScroll(true)
                endRef.current?.scrollIntoView({ behavior: 'smooth' })
              }}
              style={{ gap: 5, animation: 'pulse-dot 1.5s infinite' }}
            >
              <ArrowDown size={12} /> Scroll to Bottom
            </button>
          )}
          <button className="btn btn-danger btn-sm" onClick={onClear} style={{ gap: 5 }}>
            <Trash2 size={12} /> Clear
          </button>
        </div>
      </div>

      <div className="page-body" style={{ display: 'flex', flexDirection: 'column', gap: 12, overflow: 'hidden' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <input
            className="search-input"
            placeholder="Filter logs..."
            value={filter}
            onChange={e => setFilter(e.target.value)}
            style={{ maxWidth: 300 }}
          />
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)' }}>
            {filtered.length} {filtered.length !== 1 ? 'entries' : 'entry'}
          </span>
          {autoScroll && (
            <div style={{ display: 'flex', alignItems: 'center', gap: 5, marginLeft: 'auto' }}>
              <span className="live-dot" />
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--teal)' }}>LIVE</span>
            </div>
          )}
        </div>

        {filtered.length === 0 ? (
          <div className="empty-state">
            <div className="empty-state-icon"><Terminal size={40} /></div>
            <div className="empty-state-title">No Logs</div>
            <div className="empty-state-desc">
              Logs will appear here when actions are running.
            </div>
          </div>
        ) : (
          <div
            ref={containerRef}
            onScroll={handleScroll}
            className="log-terminal"
            style={{ flex: 1, minHeight: 0 }}
          >
            {filtered.map((entry, i) => {
              const colors = LEVEL_COLORS[entry.level?.toUpperCase()] || LEVEL_COLORS.INFO
              const key = `${entry.time}-${entry.source}-${i}`
              return (
                <div key={key} className="log-line">
                  <span className="log-time">{entry.time}</span>
                  <span className={`log-source ${colors.level}`}>[{(entry.source || 'SYS').slice(0, 6)}]</span>
                  <span className={colors.msg}>{entry.message}</span>
                </div>
              )
            })}
            <div ref={endRef} />
          </div>
        )}
      </div>
    </>
  )
}
