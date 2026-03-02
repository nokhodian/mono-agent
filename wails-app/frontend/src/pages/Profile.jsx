import { useState, useEffect } from 'react'
import {
  ArrowLeft, ExternalLink, CheckCircle, Globe, Mail,
  Users, FileText, Heart, MessageSquare, Send, Eye,
  UserPlus, UserMinus, Search, RefreshCw, Zap
} from 'lucide-react'
import { api, PLATFORM_COLORS, STATE_COLORS } from '../services/api.js'

// Map action types to icons + labels
const ACTION_META = {
  like_posts:            { icon: Heart,         label: 'Liked Posts' },
  comment_on_posts:      { icon: MessageSquare, label: 'Commented' },
  like_comments_on_posts:{ icon: Heart,         label: 'Liked Comment' },
  send_dms:              { icon: Send,          label: 'Sent DM' },
  auto_reply_dms:        { icon: Send,          label: 'Replied to DM' },
  follow_users:          { icon: UserPlus,      label: 'Followed' },
  unfollow_users:        { icon: UserMinus,     label: 'Unfollowed' },
  watch_stories:         { icon: Eye,           label: 'Watched Stories' },
  scrape_profile_info:   { icon: Search,        label: 'Scraped Profile' },
  export_followers:      { icon: Users,         label: 'Exported Followers' },
  engage_with_posts:     { icon: Zap,           label: 'Engaged with Posts' },
  engage_user_posts:     { icon: Zap,           label: 'Engaged User Posts' },
  find_by_keyword:       { icon: Search,        label: 'Found via Keyword' },
  extract_post_data:     { icon: FileText,      label: 'Extracted Post Data' },
  publish_post:          { icon: Send,          label: 'Published Post' },
}

const PLATFORM_PROFILE_URL = {
  INSTAGRAM: (u) => `https://www.instagram.com/${u}/`,
  LINKEDIN:  (u) => `https://www.linkedin.com/in/${u}/`,
  X:         (u) => `https://x.com/${u}`,
  TIKTOK:    (u) => `https://www.tiktok.com/@${u}`,
}

function StatPill({ label, value, icon: Icon }) {
  if (!value && value !== 0) return null
  return (
    <div className="profile-stat">
      {Icon && <Icon size={14} style={{ color: 'var(--cyan-dim)', flexShrink: 0 }} />}
      <span className="profile-stat-value">{value || '—'}</span>
      <span className="profile-stat-label">{label}</span>
    </div>
  )
}

function InteractionRow({ item }) {
  const meta = ACTION_META[item.action_type] || { icon: Zap, label: item.action_type }
  const Icon = meta.icon
  const stateColor = STATE_COLORS[item.status] || '#94a3b8'
  const ts = item.last_interacted_at || item.created_at
  const date = ts ? ts.slice(0, 10) : '—'
  const time = ts && ts.length > 10 ? ts.slice(11, 16) : ''

  return (
    <div className="interaction-row">
      <div className="interaction-icon" style={{ color: 'var(--cyan-dim)' }}>
        <Icon size={13} />
      </div>
      <div className="interaction-body">
        <div className="interaction-top">
          <span className="interaction-type">{meta.label}</span>
          {item.action_title && (
            <span className="interaction-action-title">via "{item.action_title}"</span>
          )}
          <span
            className="interaction-status"
            style={{ color: stateColor, borderColor: stateColor + '40', background: stateColor + '12' }}
          >
            {item.status}
          </span>
        </div>
        {item.comment_text && (
          <div className="interaction-comment">"{item.comment_text}"</div>
        )}
        {item.link && (
          <div className="interaction-link">{item.link}</div>
        )}
      </div>
      <div className="interaction-time">
        <span>{date}</span>
        {time && <span style={{ color: 'var(--text-dim)', fontSize: 10 }}>{time}</span>}
      </div>
    </div>
  )
}

function Avatar({ username, imageUrl, size = 72 }) {
  const [failed, setFailed] = useState(false)
  const initials = (username || '?').slice(0, 2).toUpperCase()
  const colors = [
    ['#7c3aed','#00b4d8'], ['#00b4d8','#00f5d4'], ['#e1306c','#7c3aed'],
    ['#f97316','#eab308'], ['#10b981','#00b4d8'],
  ]
  const pair = colors[(username?.charCodeAt(0) || 0) % colors.length]
  const style = {
    width: size, height: size, borderRadius: '50%', flexShrink: 0,
    display: 'flex', alignItems: 'center', justifyContent: 'center',
    fontFamily: 'var(--font-mono)', fontWeight: 700,
    fontSize: size * 0.26, color: 'white', overflow: 'hidden',
    border: '2px solid var(--border-bright)',
  }

  if (imageUrl && !failed) {
    return (
      <div style={{ ...style, background: 'var(--elevated)', padding: 0 }}>
        <img src={imageUrl} alt={username} onError={() => setFailed(true)}
          style={{ width: '100%', height: '100%', objectFit: 'cover' }} />
      </div>
    )
  }
  return (
    <div style={{ ...style, background: `linear-gradient(135deg, ${pair[0]}, ${pair[1]})` }}>
      {initials}
    </div>
  )
}

export default function Profile({ id, onBack, onOpenURL }) {
  const [person, setPerson] = useState(null)
  const [interactions, setInteractions] = useState([])
  const [loading, setLoading] = useState(true)
  const [filterType, setFilterType] = useState('')

  const load = async () => {
    if (!id) return
    setLoading(true)
    const [p, i] = await Promise.all([
      api.getPersonDetail(id),
      api.getPersonInteractions(id),
    ])
    setPerson(p)
    setInteractions(i || [])
    setLoading(false)
  }

  useEffect(() => { load() }, [id])

  if (loading) {
    return (
      <div className="empty-state" style={{ height: '100%' }}>
        <div className="spinner" />
      </div>
    )
  }

  if (!person) {
    return (
      <div className="empty-state" style={{ height: '100%' }}>
        <div className="empty-state-title">Profile not found</div>
        <button className="btn btn-secondary btn-sm" onClick={onBack} style={{ marginTop: 12 }}>
          <ArrowLeft size={13} /> Back
        </button>
      </div>
    )
  }

  const platformColor = PLATFORM_COLORS[person.platform?.toUpperCase()] || 'var(--cyan)'
  const profileUrl = PLATFORM_PROFILE_URL[person.platform?.toUpperCase()]?.(person.username)

  const actionTypes = [...new Set(interactions.map(i => i.action_type).filter(Boolean))]
  const filtered = filterType ? interactions.filter(i => i.action_type === filterType) : interactions

  // Summarise interaction counts
  const summary = {}
  interactions.forEach(i => {
    if (i.action_type) summary[i.action_type] = (summary[i.action_type] || 0) + 1
  })

  return (
    <div className="profile-page">
      {/* ── Header bar ── */}
      <div className="page-header">
        <div className="page-header-left">
          <button className="btn btn-ghost btn-sm" onClick={onBack} style={{ gap: 6 }}>
            <ArrowLeft size={13} /> People
          </button>
          <div className="page-subtitle" style={{ margin: '0 4px', color: 'var(--text-dim)' }}>/</div>
          <div className="page-subtitle">@{person.username}</div>
        </div>
        <div className="page-header-right">
          <button className="btn btn-ghost btn-sm" onClick={load} style={{ gap: 5 }}>
            <RefreshCw size={12} /> Refresh
          </button>
          {profileUrl && (
            <button className="btn btn-secondary btn-sm" onClick={() => onOpenURL(profileUrl)} style={{ gap: 6 }}>
              <ExternalLink size={12} /> Open on {person.platform}
            </button>
          )}
        </div>
      </div>

      <div className="page-body profile-body">
        {/* ── Hero card ── */}
        <div className="profile-hero" style={{ '--platform-color': platformColor }}>
          {/* Background blurred photo */}
          {person.image_url && (
            <div className="profile-hero-bg" style={{ backgroundImage: `url(${person.image_url})` }} />
          )}
          <div className="profile-hero-content">
            <Avatar username={person.username} imageUrl={person.image_url} size={80} />
            <div className="profile-hero-info">
              <div className="profile-name-row">
                <h1 className="profile-name">{person.full_name || person.username}</h1>
                {person.is_verified && (
                  <CheckCircle size={18} style={{ color: 'var(--cyan)', flexShrink: 0 }} />
                )}
                <span
                  className="badge"
                  style={{ background: platformColor + '20', color: platformColor, borderColor: platformColor + '40' }}
                >
                  {person.platform}
                </span>
              </div>
              <div className="profile-username">@{person.username}</div>
              {(person.job_title || person.category) && (
                <div className="profile-role">{person.job_title || person.category}</div>
              )}
              {person.introduction && (
                <p className="profile-bio">{person.introduction}</p>
              )}
              <div className="profile-links">
                {person.website && (
                  <button className="profile-meta-link" onClick={() => onOpenURL(person.website)}>
                    <Globe size={12} /> {person.website.replace(/^https?:\/\//, '')}
                  </button>
                )}
                {person.contact_details && (
                  <span className="profile-meta-link">
                    <Mail size={12} /> {person.contact_details}
                  </span>
                )}
              </div>
            </div>
          </div>

          {/* Stats row */}
          <div className="profile-stats">
            <StatPill label="Followers"  value={person.follower_count}  icon={Users} />
            <StatPill label="Following"  value={person.following_count || null} icon={Users} />
            <StatPill label="Posts"      value={person.content_count || null}   icon={FileText} />
            <StatPill label="Interactions" value={interactions.length || null}  icon={Zap} />
          </div>
        </div>

        {/* ── Interaction summary chips ── */}
        {Object.keys(summary).length > 0 && (
          <div className="profile-summary-chips">
            <button
              className={`summary-chip ${!filterType ? 'active' : ''}`}
              onClick={() => setFilterType('')}
            >
              All <span>{interactions.length}</span>
            </button>
            {Object.entries(summary).sort((a,b) => b[1]-a[1]).map(([type, count]) => {
              const meta = ACTION_META[type] || { icon: Zap, label: type }
              const Icon = meta.icon
              return (
                <button
                  key={type}
                  className={`summary-chip ${filterType === type ? 'active' : ''}`}
                  onClick={() => setFilterType(filterType === type ? '' : type)}
                >
                  <Icon size={11} />
                  {meta.label} <span>{count}</span>
                </button>
              )
            })}
          </div>
        )}

        {/* ── Interaction history ── */}
        <div className="profile-section">
          <div className="profile-section-title">
            Interaction History
            <span style={{ color: 'var(--text-muted)', fontWeight: 400, marginLeft: 8, fontSize: 11 }}>
              {filtered.length} event{filtered.length !== 1 ? 's' : ''}
            </span>
          </div>

          {filtered.length === 0 ? (
            <div style={{ padding: '24px 0', textAlign: 'center', color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', fontSize: 12 }}>
              No interactions recorded yet
            </div>
          ) : (
            <div className="interaction-list">
              {filtered.map((item, i) => (
                <InteractionRow key={`${item.action_id}-${i}`} item={item} />
              ))}
            </div>
          )}
        </div>

        {/* ── Meta footer ── */}
        <div style={{ display: 'flex', gap: 20, padding: '4px 0 8px', fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-dim)' }}>
          <span>Added {person.created_at?.slice(0, 10) || '—'}</span>
          {person.updated_at && person.updated_at !== person.created_at && (
            <span>Updated {person.updated_at.slice(0, 10)}</span>
          )}
          <span style={{ marginLeft: 'auto' }}>ID {person.id}</span>
        </div>
      </div>
    </div>
  )
}
