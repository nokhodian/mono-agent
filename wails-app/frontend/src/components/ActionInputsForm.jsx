// ActionInputsForm.jsx — renders typed input fields from action input schemas.
// Required fields are always visible. Optional fields are in a collapsible
// "Advanced options" section.

import { useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'

// ─── Input schema registry ────────────────────────────────────────────────────

const SCHEMAS = {
  INSTAGRAM: {
    find_by_keyword: {
      required: [
        { name: 'keyword', type: 'string', description: 'Search keyword or hashtag' },
        { name: 'maxResultsCount', type: 'number', description: 'Maximum results to collect' },
      ],
      optional: [
        { name: 'mediaType', type: 'select', default: 'all', options: ['all', 'image', 'video', 'reels'], description: 'Media type filter' },
        { name: 'minLikes', type: 'number', default: 0, description: 'Minimum likes filter' },
        { name: 'sortBy', type: 'select', default: 'recent', options: ['recent', 'top'], description: 'Sort order' },
        { name: 'followFound', type: 'boolean', default: false, description: 'Follow discovered accounts' },
        { name: 'saveToListId', type: 'string', default: '', description: 'Save results to list ID' },
      ],
    },
    like_posts: {
      required: [],
      optional: [
        { name: 'maxLikesPerSession', type: 'number', default: 50, description: 'Max likes per session' },
        { name: 'delayBetweenLikes', type: 'number', default: 3, description: 'Seconds between likes' },
        { name: 'skipAlreadyLiked', type: 'boolean', default: true, description: 'Skip already-liked posts' },
        { name: 'randomizeOrder', type: 'boolean', default: false, description: 'Randomize processing order' },
      ],
    },
    comment_on_posts: {
      required: [
        { name: 'commentText', type: 'textarea', description: 'Comment to post' },
      ],
      optional: [
        { name: 'maxCommentsPerSession', type: 'number', default: 20, description: 'Max comments per session' },
        { name: 'delayBetweenComments', type: 'number', default: 10, description: 'Seconds between comments' },
        { name: 'skipAlreadyCommented', type: 'boolean', default: true, description: 'Skip already-commented posts' },
      ],
    },
    send_dms: {
      required: [
        { name: 'messageText', type: 'textarea', description: 'Message to send' },
      ],
      optional: [
        { name: 'maxMessagesPerSession', type: 'number', default: 30, description: 'Max DMs per session' },
        { name: 'delayBetweenMessages', type: 'number', default: 15, description: 'Seconds between messages' },
        { name: 'skipAlreadyMessaged', type: 'boolean', default: true, description: 'Skip already-messaged users' },
      ],
    },
    auto_reply_dms: {
      required: [
        { name: 'replyText', type: 'textarea', description: 'Auto-reply message' },
        { name: 'startDate', type: 'string', description: 'Start date (YYYY-MM-DD)' },
        { name: 'endDate', type: 'string', description: 'End date (YYYY-MM-DD)' },
        { name: 'pollInterval', type: 'number', description: 'Polling interval (seconds)' },
      ],
      optional: [
        { name: 'maxRepliesPerDay', type: 'number', default: 100, description: 'Max replies per day' },
        { name: 'delayBeforeReply', type: 'number', default: 5, description: 'Seconds before replying' },
        { name: 'filterByKeyword', type: 'string', default: '', description: 'Only reply to messages with this keyword' },
        { name: 'stopAfterReplies', type: 'number', default: 0, description: 'Stop after N replies (0=unlimited)' },
      ],
    },
    follow_users: {
      required: [],
      optional: [
        { name: 'maxFollowsPerSession', type: 'number', default: 50, description: 'Max follows per session' },
        { name: 'maxFollowsPerDay', type: 'number', default: 150, description: 'Max follows per day' },
        { name: 'delayBetweenFollows', type: 'number', default: 5, description: 'Seconds between follows' },
        { name: 'skipAlreadyFollowing', type: 'boolean', default: true, description: 'Skip already-followed accounts' },
        { name: 'skipPrivateAccounts', type: 'boolean', default: false, description: 'Skip private accounts' },
      ],
    },
    unfollow_users: {
      required: [],
      optional: [
        { name: 'maxUnfollowsPerSession', type: 'number', default: 50, description: 'Max unfollows per session' },
        { name: 'maxUnfollowsPerDay', type: 'number', default: 150, description: 'Max unfollows per day' },
        { name: 'delayBetweenUnfollows', type: 'number', default: 5, description: 'Seconds between unfollows' },
        { name: 'skipMutualFollows', type: 'boolean', default: true, description: 'Skip mutual follows' },
        { name: 'onlyNonFollowers', type: 'boolean', default: false, description: 'Only unfollow non-followers' },
      ],
    },
    watch_stories: {
      required: [],
      optional: [
        { name: 'maxStoriesPerUser', type: 'number', default: 0, description: 'Max stories per user (0=all)' },
        { name: 'delayBetweenStories', type: 'number', default: 3, description: 'Seconds between stories' },
        { name: 'delayBetweenProfiles', type: 'number', default: 5, description: 'Seconds between profiles' },
        { name: 'likeStory', type: 'boolean', default: false, description: 'Like each story' },
        { name: 'replyToStory', type: 'boolean', default: false, description: 'Reply to each story' },
        { name: 'replyText', type: 'string', default: '', description: 'Reply text for stories' },
      ],
    },
    scrape_profile_info: {
      required: [],
      optional: [
        { name: 'fieldsToScrape', type: 'select', default: 'all', options: ['all', 'basic', 'followers', 'posts'], description: 'Fields to scrape' },
        { name: 'includeRecentPosts', type: 'boolean', default: false, description: 'Include recent posts' },
        { name: 'maxRecentPosts', type: 'number', default: 12, description: 'Max recent posts' },
        { name: 'saveToListId', type: 'string', default: '', description: 'Save to list ID' },
        { name: 'updateExisting', type: 'boolean', default: true, description: 'Update existing records' },
      ],
    },
    export_followers: {
      required: [
        { name: 'sourceType', type: 'string', description: 'Source type: followers, following, or profile URL' },
      ],
      optional: [
        { name: 'maxResultsCount', type: 'number', default: 1000, description: 'Max followers to export' },
        { name: 'listId', type: 'string', default: '', description: 'Save to list ID' },
        { name: 'filterMinFollowers', type: 'number', default: 0, description: 'Min follower count' },
        { name: 'filterMaxFollowers', type: 'number', default: 0, description: 'Max follower count (0=unlimited)' },
        { name: 'filterVerified', type: 'boolean', default: false, description: 'Only verified accounts' },
        { name: 'sortBy', type: 'select', default: 'recent', options: ['recent', 'followers'], description: 'Sort order' },
      ],
    },
    engage_with_posts: {
      required: [
        { name: 'maxContentCount', type: 'number', description: 'Max posts to engage with' },
        { name: 'searches', type: 'string', description: 'Hashtags/keywords (comma-separated)' },
      ],
      optional: [
        { name: 'target', type: 'select', default: 'posts', options: ['posts', 'reels'], description: 'Content type' },
        { name: 'commentText', type: 'string', default: '', description: 'Comment text' },
        { name: 'minLikes', type: 'number', default: 0, description: 'Min likes filter' },
        { name: 'maxLikes', type: 'number', default: 0, description: 'Max likes filter (0=unlimited)' },
        { name: 'sessionDelay', type: 'number', default: 30, description: 'Seconds between sessions' },
      ],
    },
    engage_user_posts: {
      required: [
        { name: 'username', type: 'string', description: 'Instagram username' },
        { name: 'maxContentCount', type: 'number', description: 'Max posts to engage' },
      ],
      optional: [
        { name: 'commentText', type: 'string', default: '', description: 'Comment text' },
        { name: 'startFromPost', type: 'number', default: 0, description: 'Start from post index' },
        { name: 'filterRecentOnly', type: 'boolean', default: false, description: 'Only recent posts' },
        { name: 'reverseOrder', type: 'boolean', default: false, description: 'Oldest to newest' },
        { name: 'filterMinLikes', type: 'number', default: 0, description: 'Min likes filter' },
        { name: 'delayBetweenPosts', type: 'number', default: 5, description: 'Seconds between posts' },
      ],
    },
    extract_post_data: {
      required: [],
      optional: [
        { name: 'fieldsToExtract', type: 'select', default: 'all', options: ['all', 'basic', 'comments'], description: 'Fields to extract' },
        { name: 'includeComments', type: 'boolean', default: false, description: 'Include comments' },
        { name: 'maxComments', type: 'number', default: 0, description: 'Max comments (0=all)' },
        { name: 'includeReplies', type: 'boolean', default: false, description: 'Include replies' },
        { name: 'downloadMedia', type: 'boolean', default: false, description: 'Download media files' },
        { name: 'delayBetweenPosts', type: 'number', default: 2, description: 'Seconds between posts' },
      ],
    },
    publish_post: {
      required: [
        { name: 'text', type: 'textarea', description: 'Post caption' },
        { name: 'media', type: 'string', description: 'Media file path or URL' },
      ],
      optional: [
        { name: 'locationTag', type: 'string', default: '', description: 'Location tag' },
        { name: 'altText', type: 'string', default: '', description: 'Accessibility alt text' },
        { name: 'enableComments', type: 'boolean', default: true, description: 'Allow comments' },
        { name: 'hideLikeCount', type: 'boolean', default: false, description: 'Hide like count' },
        { name: 'scheduledTime', type: 'string', default: '', description: 'Schedule time (ISO 8601)' },
        { name: 'tags', type: 'string', default: '', description: 'Hashtags (comma-separated)' },
        { name: 'mentionUsers', type: 'string', default: '', description: 'Mention users (comma-separated)' },
      ],
    },
    like_comments_on_posts: {
      required: [],
      optional: [
        { name: 'commentAuthor', type: 'string', default: '', description: 'Only like comments by this author' },
        { name: 'maxCommentsToLike', type: 'number', default: 5, description: 'Max comments to like per post' },
        { name: 'maxLikesPerSession', type: 'number', default: 100, description: 'Max comment likes per session' },
        { name: 'delayBetweenLikes', type: 'number', default: 2, description: 'Seconds between likes' },
        { name: 'likeReplies', type: 'boolean', default: false, description: 'Also like replies' },
        { name: 'skipAlreadyLiked', type: 'boolean', default: true, description: 'Skip already-liked comments' },
        { name: 'sortByNew', type: 'boolean', default: false, description: 'Sort comments by newest' },
      ],
    },
  },
}

// Fall back to empty schema if action type is not in registry
function getSchema(platform, actionType) {
  return SCHEMAS[platform]?.[actionType] || { required: [], optional: [] }
}

// ─── Single field renderer ────────────────────────────────────────────────────

function FieldInput({ field, value, onChange }) {
  const label = (
    <label
      htmlFor={`param-${field.name}`}
      className="form-label"
      title={field.description}
      style={{ cursor: 'help' }}
    >
      {field.name}
      {field.description && (
        <span style={{ color: 'var(--text-dim)', fontWeight: 400, marginLeft: 4, fontSize: 10 }}>
          — {field.description}
        </span>
      )}
    </label>
  )

  if (field.type === 'boolean') {
    const checked = value === undefined ? field.default ?? false : Boolean(value)
    return (
      <div className="form-group" style={{ marginBottom: 10 }}>
        <label
          htmlFor={`param-${field.name}`}
          style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}
          title={field.description}
        >
          <input
            id={`param-${field.name}`}
            type="checkbox"
            checked={checked}
            onChange={e => onChange(field.name, e.target.checked)}
            style={{ accentColor: 'var(--accent)', width: 14, height: 14 }}
          />
          <span className="form-label" style={{ margin: 0, cursor: 'pointer' }}>
            {field.name}
            {field.description && (
              <span style={{ color: 'var(--text-dim)', fontWeight: 400, marginLeft: 4, fontSize: 10 }}>
                — {field.description}
              </span>
            )}
          </span>
        </label>
      </div>
    )
  }

  if (field.type === 'select') {
    const current = value === undefined ? (field.default ?? '') : value
    return (
      <div className="form-group" style={{ marginBottom: 10 }}>
        {label}
        <select
          id={`param-${field.name}`}
          className="form-select"
          value={current}
          onChange={e => onChange(field.name, e.target.value)}
        >
          {(field.options || []).map(opt => (
            <option key={opt} value={opt}>{opt}</option>
          ))}
        </select>
      </div>
    )
  }

  if (field.type === 'number') {
    const current = value === undefined ? (field.default ?? 0) : value
    return (
      <div className="form-group" style={{ marginBottom: 10 }}>
        {label}
        <input
          id={`param-${field.name}`}
          type="number"
          className="form-input"
          value={current}
          min={field.min}
          max={field.max}
          onChange={e => onChange(field.name, parseFloat(e.target.value) || 0)}
        />
      </div>
    )
  }

  if (field.type === 'textarea') {
    const current = value === undefined ? (field.default ?? '') : value
    return (
      <div className="form-group" style={{ marginBottom: 10 }}>
        {label}
        <textarea
          id={`param-${field.name}`}
          className="form-textarea"
          value={current}
          placeholder={field.description}
          onChange={e => onChange(field.name, e.target.value)}
          rows={3}
        />
      </div>
    )
  }

  // Default: string input
  const current = value === undefined ? (field.default ?? '') : value
  return (
    <div className="form-group" style={{ marginBottom: 10 }}>
      {label}
      <input
        id={`param-${field.name}`}
        type="text"
        className="form-input"
        value={current}
        placeholder={field.description}
        onChange={e => onChange(field.name, e.target.value)}
      />
    </div>
  )
}

// ─── Main exported component ──────────────────────────────────────────────────

/**
 * ActionInputsForm renders required + optional (collapsible) input fields for
 * the given platform/actionType combination. `params` is the current key-value
 * map; `onChange(newParams)` is called whenever any field changes.
 */
export default function ActionInputsForm({ platform, actionType, params = {}, onChange }) {
  const [showAdvanced, setShowAdvanced] = useState(false)
  const schema = getSchema(platform, actionType)

  if (!actionType) return null
  if (schema.required.length === 0 && schema.optional.length === 0) return null

  const setField = (name, value) => {
    onChange({ ...params, [name]: value })
  }

  return (
    <div style={{ marginTop: 4 }}>
      {/* Required fields */}
      {schema.required.length > 0 && (
        <div style={{
          padding: '10px 12px',
          background: 'rgba(255,255,255,0.02)',
          border: '1px solid var(--border)',
          borderRadius: 'var(--radius)',
          marginBottom: 8,
        }}>
          <div style={{ fontSize: 10, fontFamily: 'var(--font-mono)', color: 'var(--text-dim)', marginBottom: 8, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
            Required
          </div>
          {schema.required.map(field => (
            <FieldInput
              key={field.name}
              field={field}
              value={params[field.name]}
              onChange={setField}
            />
          ))}
        </div>
      )}

      {/* Optional / advanced fields */}
      {schema.optional.length > 0 && (
        <div style={{
          border: '1px solid var(--border)',
          borderRadius: 'var(--radius)',
          overflow: 'hidden',
        }}>
          <button
            type="button"
            onClick={() => setShowAdvanced(v => !v)}
            style={{
              width: '100%',
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              padding: '7px 12px',
              background: 'rgba(255,255,255,0.02)',
              border: 'none',
              cursor: 'pointer',
              color: 'var(--text-muted)',
              fontSize: 11,
              fontFamily: 'var(--font-mono)',
              textAlign: 'left',
            }}
          >
            {showAdvanced ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
            Advanced options ({schema.optional.length})
          </button>
          {showAdvanced && (
            <div style={{ padding: '8px 12px 4px', borderTop: '1px solid var(--border)' }}>
              {schema.optional.map(field => (
                <FieldInput
                  key={field.name}
                  field={field}
                  value={params[field.name]}
                  onChange={setField}
                />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
