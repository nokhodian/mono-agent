# Posts & Comments Feature Design

**Date:** 2026-03-16
**Project:** mono-agent
**Status:** Approved

---

## Overview

Add first-class support for scraped social media posts and their comments. Posts are displayed as a collapsible section on a person's Profile page. Clicking a post opens a full PostDetail page showing post metadata and a scrollable comments list. The CLI auto-saves post and comment data to SQLite after running `list_user_posts` and `list_post_comments` node types.

---

## Data Layer

### Migration

Two new tables are added via **two mechanisms**:

1. A new migration file `data/migrations/007_posts_comments.sql` applied by `database.ApplyMigrations()` when the CLI runs.
2. The same DDL blocks added to the `safeMigrations` slice in `wails-app/app.go` so the Wails desktop app (which does not call `ApplyMigrations`) also creates the tables at startup.

### Schema

```sql
-- data/migrations/007_posts_comments.sql

CREATE TABLE IF NOT EXISTS posts (
  id            TEXT PRIMARY KEY,          -- UUID generated at insert time; preserved on upsert conflict
  person_id     TEXT REFERENCES people(id),
  platform      TEXT NOT NULL,
  shortcode     TEXT NOT NULL,
  url           TEXT NOT NULL,
  thumbnail_url TEXT,
  like_count    INTEGER,
  comment_count INTEGER,
  caption       TEXT,
  posted_at     TEXT,
  scraped_at    TEXT NOT NULL,
  UNIQUE(platform, shortcode)
);

CREATE TABLE IF NOT EXISTS post_comments (
  id          TEXT PRIMARY KEY,            -- UUID generated at insert time
  post_id     TEXT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  author      TEXT NOT NULL,
  text        TEXT,
  timestamp   TEXT,
  likes_count INTEGER DEFAULT 0,
  reply_count INTEGER DEFAULT 0,
  scraped_at  TEXT NOT NULL,
  UNIQUE(post_id, author, timestamp)       -- natural deduplication key
);
```

**`posts.id`:** A new `uuid.New().String()` generated at insert time. On `CONFLICT(platform, shortcode)` the existing `id` is preserved (`id` is excluded from the `DO UPDATE SET` clause).

**`post_comments.id`:** A new `uuid.New().String()` generated at insert time. On `CONFLICT(post_id, author, timestamp)` the row is updated in place (`text`, `likes_count`, `reply_count`, `scraped_at` are overwritten; `id` is preserved).

### CLI Auto-Save Wiring

Same pattern as `scrape_profile_info → people` in `cmd/monoes/node.go`:

**After `list_user_posts` completes:**
- Each output item contains `url`, `shortcode`, `thumbnail_src`, `alt_text`, `platform`.
- `person_id` is resolved via: `SELECT id FROM people WHERE platform_username = ? AND platform = ?` using the username extracted from the post URL via `bot.PlatformRegistry[platformUpper].ExtractUsername(postURL)`.
- Upsert into `posts` — `ON CONFLICT(platform, shortcode) DO UPDATE SET like_count, comment_count, thumbnail_url, caption, scraped_at`. The `id` column is **not** in the update set.

**After `list_post_comments` completes:**
- Each output item contains `author`, `text`, `timestamp`, `likes_count`, `reply_count`.
- The parent `post_id` is resolved by: `SELECT id FROM posts WHERE platform = ? AND shortcode = ?` using the shortcode extracted from the post URL in the config.
- Upsert into `post_comments` — `ON CONFLICT(post_id, author, timestamp) DO UPDATE SET text, likes_count, reply_count, scraped_at`.

### we_liked / we_commented — URL Normalization

`GetPersonPosts` derives boolean flags via LEFT JOIN on `action_targets WHERE link = posts.url`. To avoid false negatives from URL formatting differences, both `posts.url` and `action_targets.link` are normalized at query time using SQLite's `TRIM` + `RTRIM(url, '/')` before comparison:

```sql
LEFT JOIN actions a ON at.action_id = a.id
WHERE rtrim(at.link, '/') = rtrim(p.url, '/')
  AND a.type IN ('like_posts', 'comment_on_posts')
  AND at.status = 'COMPLETED'
```

`action_targets` has no `action_type` column — the type lives on the `actions` table. This mirrors the pattern used by `GetPersonInteractions` in `app.go`.

---

## Backend API

Three new Wails-bound functions on `App` in `wails-app/app.go`:

```go
func (a *App) GetPersonPosts(personId string) []PostSummary
func (a *App) GetPostDetail(postId string) *PostDetail
func (a *App) GetPostComments(postId string) []PostComment
```

All three return empty slices (never nil) on DB error to keep the frontend safe.

### Response Types

```go
type PostSummary struct {
    ID           string `json:"id"`
    Shortcode    string `json:"shortcode"`
    URL          string `json:"url"`
    ThumbnailURL string `json:"thumbnail_url"`
    LikeCount    int    `json:"like_count"`
    CommentCount int    `json:"comment_count"`
    Caption      string `json:"caption"`
    PostedAt     string `json:"posted_at"`
    ScrapedAt    string `json:"scraped_at"`
    WeLiked      bool   `json:"we_liked"`
    WeCommented  bool   `json:"we_commented"`
}

type PostDetail struct {
    ID           string `json:"id"`
    Shortcode    string `json:"shortcode"`
    URL          string `json:"url"`
    ThumbnailURL string `json:"thumbnail_url"`
    LikeCount    int    `json:"like_count"`
    CommentCount int    `json:"comment_count"`
    Caption      string `json:"caption"`
    PostedAt     string `json:"posted_at"`
    ScrapedAt    string `json:"scraped_at"`
}

type PostComment struct {
    ID         string `json:"id"`
    Author     string `json:"author"`
    Text       string `json:"text"`
    Timestamp  string `json:"timestamp"`
    LikesCount int    `json:"likes_count"`
    ReplyCount int    `json:"reply_count"`
}
```

---

## Frontend

### Navigation (App.jsx)

Add `postId` state alongside `profileId`. Navigation follows the same `activePage` switching pattern:

- `activePage = 'people'` → click person → `activePage = 'profile'`, `profileId = p.id`
- `activePage = 'profile'` → click post → `activePage = 'postDetail'`, `postId = post.id` (**`profileId` is preserved**)
- `activePage = 'postDetail'` → Back → `activePage = 'profile'` (postId cleared, profileId still set)

The `pages` object in `App.jsx` gains a `postDetail` entry:

```jsx
postDetail: <PostDetail
  id={postId}
  profileId={profileId}
  onBack={() => { setPostId(null); setActivePage('profile') }}
  onOpenURL={api.openURL}
/>
```

`Profile` receives a new `onOpenPost` prop:

```jsx
profile: <Profile
  id={profileId}
  onBack={closeProfile}
  onOpenURL={api.openURL}
  onOpenPost={(pid) => { setPostId(pid); setActivePage('postDetail') }}
/>
```

### Profile.jsx — Posts Section

A collapsible `<PostsSection>` component inserted **above** the interaction history section:

- Header: `Posts · {count}` with a chevron toggle icon
- Collapsed by default when `posts.length === 0`; expanded by default when posts exist
- Fetches via `api.getPersonPosts(personId)` on mount
- Each list row shows:
  - Shortcode in monospace, clickable → calls `onOpenPost(post.id)`
  - External link icon → calls `onOpenURL(post.url)`
  - Heart icon + like count
  - Speech bubble icon + comment count
  - `♥ liked` badge (cyan, only when `we_liked = true`)
  - `💬 commented` badge (purple, only when `we_commented = true`)
- Empty state: *"No posts scraped yet — run list_user_posts to populate"*

### PostDetail.jsx — New Page

New top-level page component. Props: `id`, `profileId`, `onBack`, `onOpenURL`.

**Header:**
- Back button → calls `onBack()`
- Post shortcode as page title (monospace)
- External link icon → calls `onOpenURL(post.url)`

**Meta row:**
- Platform badge
- Like count with heart icon
- Comment count with speech bubble icon
- Scraped date (muted)

**Comments section:**
- Label: `Comments ({n})`
- Scrollable list; each row:
  - `@author` in cyan monospace
  - Comment text
  - Timestamp right-aligned (muted)
  - Heart icon + like count (only shown if `likes_count > 0`)
- Empty state: *"No comments scraped yet — run list_post_comments to populate"*

Fetches `api.getPostDetail(id)` and `api.getPostComments(id)` on mount.

### api.js Additions

Inside the existing `api` object literal:

```javascript
const api = {
  // ... existing entries ...
  getPersonPosts:  (personId) => GoApp.GetPersonPosts(personId).catch(() => []),
  getPostDetail:   (postId)   => GoApp.GetPostDetail(postId).catch(() => null),
  getPostComments: (postId)   => GoApp.GetPostComments(postId).catch(() => []),
}
```

---

## Testing

Use the CLI to verify the full data pipeline:

```bash
# 1. Scrape 10 posts for mortezanoes
go run ./cmd/monoes node run instagram.list_user_posts \
  --config '{"username":"onetap","targets":[{"url":"https://www.instagram.com/mortezanoes/","username":"mortezanoes"}],"maxCount":10}'

# 2. Verify posts saved to DB
sqlite3 ~/.monoes/monoes.db \
  "SELECT shortcode, like_count, comment_count, person_id FROM posts WHERE platform='INSTAGRAM' LIMIT 10;"

# 3. Pick a shortcode from step 2, then scrape 10 comments from that post
go run ./cmd/monoes node run instagram.list_post_comments \
  --config '{"username":"onetap","targets":[{"url":"https://www.instagram.com/p/<shortcode>/"}],"maxComments":10}'

# 4. Verify comments saved
sqlite3 ~/.monoes/monoes.db \
  "SELECT author, text, likes_count FROM post_comments LIMIT 10;"
```

---

## Error Handling

- **Missing `person_id`:** If the person has not yet been scraped into `people`, `person_id` is saved as `NULL`. The post is still stored and can be queried by shortcode; it will not appear on any profile until the person is scraped.
- **Duplicate posts:** `ON CONFLICT(platform, shortcode)` upsert — later scrape updates counts, preserves `id`.
- **Duplicate comments:** `ON CONFLICT(post_id, author, timestamp)` upsert — later scrape updates text and counts, preserves `id`.
- **API errors:** `GetPersonPosts`, `GetPostDetail`, `GetPostComments` return empty slice / nil on any DB error; frontend empty-states handle gracefully.
- **URL normalization:** Both sides of the `we_liked`/`we_commented` join use `RTRIM(url, '/')` to handle trailing slash differences.
