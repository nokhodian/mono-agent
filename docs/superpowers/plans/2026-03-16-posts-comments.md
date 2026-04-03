# Posts & Comments Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add scraped posts and comments as first-class entities — stored in SQLite, auto-saved by the CLI, exposed via Go API, and displayed in a collapsible section on the Profile page with a dedicated PostDetail page for comments.

**Architecture:** Two new SQLite tables (`posts`, `post_comments`) are created at startup via both a migration file (for CLI) and the `safeMigrations` slice (for Wails app). The CLI auto-saves after `list_user_posts` and `list_post_comments` the same way it does for `scrape_profile_info → people`. Three new Wails-bound Go functions serve the frontend. The Profile page gets a collapsible PostsSection above interaction history; clicking a post navigates to a new PostDetail page.

**Tech Stack:** Go 1.21, SQLite (modernc.org/sqlite), Wails v2, React 18 JSX, lucide-react icons, github.com/google/uuid

**Spec:** `docs/superpowers/specs/2026-03-16-posts-comments-design.md`

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `data/migrations/007_posts_comments.sql` | Create | DDL for `posts` and `post_comments` tables |
| `wails-app/app.go` | Modify | safeMigrations entries + Go types + 3 Wails API functions |
| `cmd/monoes/node.go` | Modify | Auto-save posts after `list_user_posts`; auto-save comments after `list_post_comments` |
| `wails-app/frontend/src/services/api.js` | Modify | Add `getPersonPosts`, `getPostDetail`, `getPostComments` |
| `wails-app/frontend/src/App.jsx` | Modify | Add `postId` state, `postDetail` page, `onOpenPost` prop to Profile |
| `wails-app/frontend/src/pages/Profile.jsx` | Modify | Add `PostsSection` collapsible component above interaction history |
| `wails-app/frontend/src/pages/PostDetail.jsx` | Create | New full-page post detail + comments list |

---

## Chunk 1: Database + CLI Auto-Save

### Task 1: Create migration file

**Files:**
- Create: `data/migrations/007_posts_comments.sql`

- [ ] **Step 1.1: Create the migration file**

```sql
-- data/migrations/007_posts_comments.sql

CREATE TABLE IF NOT EXISTS posts (
  id            TEXT PRIMARY KEY,
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
  id          TEXT PRIMARY KEY,
  post_id     TEXT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  author      TEXT NOT NULL,
  text        TEXT,
  timestamp   TEXT,
  likes_count INTEGER DEFAULT 0,
  reply_count INTEGER DEFAULT 0,
  scraped_at  TEXT NOT NULL,
  UNIQUE(post_id, author, timestamp)
);
```

- [ ] **Step 1.2: Apply the migration manually to verify SQL is valid**

```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes
sqlite3 ~/.monoes/monoes.db < data/migrations/007_posts_comments.sql
sqlite3 ~/.monoes/monoes.db ".tables" | grep -E "posts|post_comments"
```

Expected output:
```
post_comments  posts
```

- [ ] **Step 1.3: Commit**

```bash
git add data/migrations/007_posts_comments.sql
git commit -m "feat: add posts and post_comments migration"
```

---

### Task 2: Add safeMigrations entries in app.go

**Files:**
- Modify: `wails-app/app.go` (around line 162, after the last entry in `safeMigrations`)

- [ ] **Step 2.1: Add the two CREATE TABLE IF NOT EXISTS blocks to the safeMigrations slice**

In `wails-app/app.go`, find the `safeMigrations` slice (line ~120). Add these two entries at the end, before the closing `}`:

```go
		`CREATE TABLE IF NOT EXISTS posts (
			id            TEXT PRIMARY KEY,
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
		)`,
		`CREATE TABLE IF NOT EXISTS post_comments (
			id          TEXT PRIMARY KEY,
			post_id     TEXT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
			author      TEXT NOT NULL,
			text        TEXT,
			timestamp   TEXT,
			likes_count INTEGER DEFAULT 0,
			reply_count INTEGER DEFAULT 0,
			scraped_at  TEXT NOT NULL,
			UNIQUE(post_id, author, timestamp)
		)`,
```

- [ ] **Step 2.2: Build to verify no compile errors**

```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes/wails-app
go build ./... 2>&1
echo "EXIT:$?"
```

Expected: `EXIT:0`

- [ ] **Step 2.3: Commit**

```bash
git add wails-app/app.go
git commit -m "feat: add posts/post_comments to wails safeMigrations"
```

---

### Task 3: CLI auto-save for list_user_posts

**Files:**
- Modify: `cmd/monoes/node.go` (after the existing `scrape_profile_info` auto-save block, around line 395)

The items from `list_user_posts` have these fields (from the bot method in `internal/bot/instagram/actions.go`):
- `url` — post URL e.g. `https://www.instagram.com/p/CD61bhxKOQh/`
- `shortcode` — e.g. `CD61bhxKOQh`
- `thumbnail_src` — thumbnail image URL
- `alt_text` — caption/alt text

The `username` and `platform` come from the action config.

- [ ] **Step 3.1: Add the auto-save block for list_user_posts in node.go**

In `cmd/monoes/node.go`, locate the existing `scrape_profile_info` auto-save block (around line 375). Add this block immediately after it:

```go
// Auto-save posts to posts table after list_user_posts.
if strings.HasSuffix(nodeType, "list_user_posts") && rawDB != nil {
    var allItems []workflow.Item
    for _, o := range outputs {
        allItems = append(allItems, o.Items...)
    }
    if len(allItems) > 0 {
        saved, skipped := savePostsToDB(ctx, rawDB, allItems, nodeType, config)
        fmt.Fprintf(os.Stderr, "  Saved %d post(s) to posts table (%d skipped — no shortcode)\n", saved, skipped)
    }
}
```

- [ ] **Step 3.2: Add the savePostsToDB helper function in node.go**

Add this function at the bottom of `cmd/monoes/node.go`, after the last function:

```go
// savePostsToDB upserts scraped post items into the posts table.
// Returns (saved, skipped) counts.
func savePostsToDB(ctx context.Context, db *sql.DB, items []workflow.Item, nodeType string, config map[string]interface{}) (int, int) {
    // Derive platform from nodeType prefix e.g. "instagram.list_user_posts" → "INSTAGRAM"
    platform := strings.ToUpper(strings.SplitN(nodeType, ".", 2)[0])

    // Resolve person_id: find username from config targets, look up people table.
    personID := ""
    if targets, ok := config["targets"].([]interface{}); ok && len(targets) > 0 {
        if t, ok := targets[0].(map[string]interface{}); ok {
            profileURL, _ := t["url"].(string)
            username := ""
            if factory, ok := bot.PlatformRegistry[platform]; ok {
                username = factory().ExtractUsername(profileURL)
            }
            if username != "" {
                _ = db.QueryRowContext(ctx,
                    "SELECT id FROM people WHERE platform_username = ? AND UPPER(platform) = ?",
                    username, platform,
                ).Scan(&personID)
            }
        }
    }

    now := time.Now().UTC().Format(time.RFC3339)
    saved, skipped := 0, 0

    for _, item := range items {
        data := item.JSON
        shortcode, _ := data["shortcode"].(string)
        postURL, _ := data["url"].(string)

        // Fallback: extract shortcode from URL if not present as a field.
        if shortcode == "" && postURL != "" {
            shortcode = extractPostShortcode(postURL)
        }
        if shortcode == "" {
            skipped++
            continue
        }
        if postURL == "" {
            postURL = "https://www.instagram.com/p/" + shortcode + "/"
        }

        thumbnail, _ := data["thumbnail_src"].(string)
        caption, _ := data["alt_text"].(string)

        var personIDArg interface{}
        if personID != "" {
            personIDArg = personID
        }

        // Note: like_count and comment_count are NOT available from list_user_posts output
        // (the bot method returns URLs/thumbnails, not engagement counts). They remain NULL
        // until updated by a future enrichment step. The spec's mention of updating them on
        // conflict applies only when a source provides them.
        _, err := db.ExecContext(ctx,
            `INSERT INTO posts (id, person_id, platform, shortcode, url, thumbnail_url, caption, scraped_at)
             VALUES (?, ?, ?, ?, ?, ?, ?, ?)
             ON CONFLICT(platform, shortcode)
             DO UPDATE SET
               thumbnail_url = COALESCE(excluded.thumbnail_url, posts.thumbnail_url),
               caption       = COALESCE(excluded.caption,       posts.caption),
               person_id     = COALESCE(excluded.person_id,     posts.person_id),
               scraped_at    = excluded.scraped_at`,
            uuid.New().String(), personIDArg, platform, shortcode, postURL,
            nullableStrCLI(thumbnail), nullableStrCLI(caption), now,
        )
        if err != nil {
            fmt.Fprintf(os.Stderr, "  Warning: failed to save post %s: %v\n", shortcode, err)
            skipped++
        } else {
            saved++
        }
    }
    return saved, skipped
}

// extractPostShortcode extracts the shortcode from an Instagram post or reel URL.
// e.g. https://www.instagram.com/p/CD61bhxKOQh/ → "CD61bhxKOQh"
func extractPostShortcode(postURL string) string {
    parts := strings.Split(strings.Trim(postURL, "/"), "/")
    for i, p := range parts {
        if (p == "p" || p == "reel") && i+1 < len(parts) {
            return parts[i+1]
        }
    }
    return ""
}

func nullableStrCLI(s string) interface{} {
    if s == "" {
        return nil
    }
    return s
}
```

- [ ] **Step 3.3: Add required imports to node.go**

`uuid` and `time` are used in other files in this package but are NOT in `node.go`'s own import block — they must be added explicitly. `database/sql` and `context` are already present.

Check what's currently imported:
```bash
grep -n '"github.com/google/uuid"\|"time"\|"database/sql"\|"context"' \
  /Users/morteza/Desktop/monoes/mono-agent/newmonoes/cmd/monoes/node.go
```

Add the missing ones to the import block in `node.go`. After your additions the block must include at minimum:
```go
import (
    // ... existing ...
    "context"
    "database/sql"
    "time"
    "github.com/google/uuid"
    // ...
)
```

> **Note:** `people.go` in the same package already defines a `nullableStr` function that returns `sql.NullString`. The helper added here is named `nullableStrCLI` (returns `interface{}`) to avoid a compile-time redeclaration error. Do NOT rename it to `nullableStr`.

- [ ] **Step 3.4: Build CLI**

```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes
go build ./cmd/monoes/... 2>&1
echo "EXIT:$?"
```

Expected: `EXIT:0`

- [ ] **Step 3.5: Test — run list_user_posts and verify DB**

```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes
go run ./cmd/monoes node run instagram.list_user_posts \
  --config '{"username":"onetap","targets":[{"url":"https://www.instagram.com/mortezanoes/","username":"mortezanoes"}],"maxCount":10}' 2>&1

# Verify posts saved
sqlite3 ~/.monoes/monoes.db \
  "SELECT shortcode, like_count, comment_count, person_id FROM posts WHERE platform='INSTAGRAM' ORDER BY scraped_at DESC LIMIT 10;"
```

Expected: `Saved N post(s) to posts table` and 10 rows in DB.

- [ ] **Step 3.6: Commit**

```bash
git add cmd/monoes/node.go
git commit -m "feat: auto-save posts to DB after list_user_posts"
```

---

### Task 4: CLI auto-save for list_post_comments

**Files:**
- Modify: `cmd/monoes/node.go`

The items from `list_post_comments` have: `author`, `text`, `timestamp`, `likes_count`, `reply_count`.
The post URL comes from `config["targets"][0]["url"]`.

- [ ] **Step 4.1: Add the auto-save block for list_post_comments in node.go**

After the `list_user_posts` block (from Task 3), add:

```go
// Auto-save comments to post_comments table after list_post_comments.
if strings.HasSuffix(nodeType, "list_post_comments") && rawDB != nil {
    var allItems []workflow.Item
    for _, o := range outputs {
        allItems = append(allItems, o.Items...)
    }
    if len(allItems) > 0 {
        // Resolve post_id from config targets[0].url
        postID := ""
        platform := strings.ToUpper(strings.SplitN(nodeType, ".", 2)[0])
        if targets, ok := config["targets"].([]interface{}); ok && len(targets) > 0 {
            if t, ok := targets[0].(map[string]interface{}); ok {
                postURL, _ := t["url"].(string)
                shortcode := extractPostShortcode(postURL)
                if shortcode != "" {
                    _ = rawDB.QueryRowContext(ctx,
                        "SELECT id FROM posts WHERE platform = ? AND shortcode = ?",
                        platform, shortcode,
                    ).Scan(&postID)
                }
            }
        }
        if postID == "" {
            fmt.Fprintf(os.Stderr, "  Warning: post not found in DB — run list_user_posts first\n")
        } else {
            saved, skipped := saveCommentsToDB(ctx, rawDB, allItems, postID)
            fmt.Fprintf(os.Stderr, "  Saved %d comment(s) to post_comments table (%d skipped)\n", saved, skipped)
        }
    }
}
```

- [ ] **Step 4.2: Add the saveCommentsToDB helper function**

Add after `savePostsToDB` in `cmd/monoes/node.go`:

```go
// saveCommentsToDB upserts scraped comment items into the post_comments table.
func saveCommentsToDB(ctx context.Context, db *sql.DB, items []workflow.Item, postID string) (int, int) {
    now := time.Now().UTC().Format(time.RFC3339)
    saved, skipped := 0, 0

    for _, item := range items {
        data := item.JSON
        author, _ := data["author"].(string)
        if author == "" {
            skipped++
            continue
        }
        text, _ := data["text"].(string)
        timestamp, _ := data["timestamp"].(string)
        if timestamp == "" {
            timestamp = now
        }

        likesCount := int64(0)
        switch v := data["likes_count"].(type) {
        case float64:
            likesCount = int64(v)
        case int64:
            likesCount = v
        }
        replyCount := int64(0)
        switch v := data["reply_count"].(type) {
        case float64:
            replyCount = int64(v)
        case int64:
            replyCount = v
        }

        _, err := db.ExecContext(ctx,
            `INSERT INTO post_comments (id, post_id, author, text, timestamp, likes_count, reply_count, scraped_at)
             VALUES (?, ?, ?, ?, ?, ?, ?, ?)
             ON CONFLICT(post_id, author, timestamp)
             DO UPDATE SET
               text        = COALESCE(excluded.text,        post_comments.text),
               likes_count = excluded.likes_count,
               reply_count = excluded.reply_count,
               scraped_at  = excluded.scraped_at`,
            uuid.New().String(), postID, author,
            nullableStrCLI(text), timestamp, likesCount, replyCount, now,
        )
        if err != nil {
            fmt.Fprintf(os.Stderr, "  Warning: failed to save comment by %s: %v\n", author, err)
            skipped++
        } else {
            saved++
        }
    }
    return saved, skipped
}
```

- [ ] **Step 4.3: Build CLI**

```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes
go build ./cmd/monoes/... 2>&1
echo "EXIT:$?"
```

Expected: `EXIT:0`

- [ ] **Step 4.4: Test — pick a shortcode from Task 3.5, scrape 10 comments**

```bash
# Replace <shortcode> with one from `sqlite3 ~/.monoes/monoes.db "SELECT shortcode FROM posts LIMIT 1;"`
SHORTCODE=$(sqlite3 ~/.monoes/monoes.db "SELECT shortcode FROM posts WHERE platform='INSTAGRAM' LIMIT 1;")
echo "Using shortcode: $SHORTCODE"

go run ./cmd/monoes node run instagram.list_post_comments \
  --config "{\"username\":\"onetap\",\"targets\":[{\"url\":\"https://www.instagram.com/p/$SHORTCODE/\"}],\"maxComments\":10}" 2>&1

sqlite3 ~/.monoes/monoes.db \
  "SELECT author, text, likes_count FROM post_comments LIMIT 10;"
```

Expected: `Saved N comment(s) to post_comments table` and rows visible in DB.

- [ ] **Step 4.5: Commit**

```bash
git add cmd/monoes/node.go
git commit -m "feat: auto-save comments to DB after list_post_comments"
```

---

## Chunk 2: Go API (Wails Backend)

### Task 5: Add Go types and API functions to app.go

**Files:**
- Modify: `wails-app/app.go`

- [ ] **Step 5.1: Add the three response types**

Find the `PersonInfo` struct in `wails-app/app.go` (around line 520). Add these three new types nearby, grouped together:

```go
// PostSummary is returned by GetPersonPosts.
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

// PostDetail is returned by GetPostDetail.
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

// PostComment is returned by GetPostComments.
type PostComment struct {
	ID         string `json:"id"`
	Author     string `json:"author"`
	Text       string `json:"text"`
	Timestamp  string `json:"timestamp"`
	LikesCount int    `json:"likes_count"`
	ReplyCount int    `json:"reply_count"`
}
```

- [ ] **Step 5.2: Add GetPersonPosts function**

Add after `GetPersonInteractions` in `wails-app/app.go`:

```go
// GetPersonPosts returns all scraped posts for a person, with we_liked/we_commented flags.
func (a *App) GetPersonPosts(personID string) []PostSummary {
	if a.db == nil {
		return []PostSummary{}
	}
	rows, err := a.db.Query(`
		SELECT
			p.id,
			p.shortcode,
			p.url,
			COALESCE(p.thumbnail_url, ''),
			COALESCE(p.like_count, 0),
			COALESCE(p.comment_count, 0),
			COALESCE(p.caption, ''),
			COALESCE(p.posted_at, ''),
			p.scraped_at,
			EXISTS(
				SELECT 1 FROM action_targets at2
				JOIN actions a2 ON at2.action_id = a2.id
				WHERE rtrim(at2.link, '/') = rtrim(p.url, '/')
				  AND a2.type = 'like_posts'
				  AND at2.status = 'COMPLETED'
			) AS we_liked,
			EXISTS(
				SELECT 1 FROM action_targets at3
				JOIN actions a3 ON at3.action_id = a3.id
				WHERE rtrim(at3.link, '/') = rtrim(p.url, '/')
				  AND a3.type = 'comment_on_posts'
				  AND at3.status = 'COMPLETED'
			) AS we_commented
		FROM posts p
		WHERE p.person_id = ?
		ORDER BY p.scraped_at DESC`,
		personID,
	)
	if err != nil {
		return []PostSummary{}
	}
	defer rows.Close()

	var posts []PostSummary
	for rows.Next() {
		var p PostSummary
		var weLiked, weCommented int
		if err := rows.Scan(
			&p.ID, &p.Shortcode, &p.URL, &p.ThumbnailURL,
			&p.LikeCount, &p.CommentCount, &p.Caption,
			&p.PostedAt, &p.ScrapedAt,
			&weLiked, &weCommented,
		); err != nil {
			continue
		}
		p.WeLiked = weLiked != 0
		p.WeCommented = weCommented != 0
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return []PostSummary{}
	}
	if posts == nil {
		return []PostSummary{}
	}
	return posts
}
```

- [ ] **Step 5.3: Add GetPostDetail function**

```go
// GetPostDetail returns full metadata for a single post by ID.
func (a *App) GetPostDetail(postID string) *PostDetail {
	if a.db == nil {
		return nil
	}
	var p PostDetail
	err := a.db.QueryRow(`
		SELECT id, shortcode, url,
		       COALESCE(thumbnail_url, ''),
		       COALESCE(like_count, 0),
		       COALESCE(comment_count, 0),
		       COALESCE(caption, ''),
		       COALESCE(posted_at, ''),
		       scraped_at
		FROM posts WHERE id = ?`,
		postID,
	).Scan(
		&p.ID, &p.Shortcode, &p.URL, &p.ThumbnailURL,
		&p.LikeCount, &p.CommentCount, &p.Caption,
		&p.PostedAt, &p.ScrapedAt,
	)
	if err != nil {
		return nil
	}
	return &p
}
```

- [ ] **Step 5.4: Add GetPostComments function**

```go
// GetPostComments returns all scraped comments for a post, ordered by timestamp.
func (a *App) GetPostComments(postID string) []PostComment {
	if a.db == nil {
		return []PostComment{}
	}
	rows, err := a.db.Query(`
		SELECT id, COALESCE(author, ''), COALESCE(text, ''),
		       COALESCE(timestamp, ''),
		       COALESCE(likes_count, 0),
		       COALESCE(reply_count, 0)
		FROM post_comments
		WHERE post_id = ?
		ORDER BY timestamp ASC`,
		postID,
	)
	if err != nil {
		return []PostComment{}
	}
	defer rows.Close()

	var comments []PostComment
	for rows.Next() {
		var c PostComment
		if err := rows.Scan(
			&c.ID, &c.Author, &c.Text,
			&c.Timestamp, &c.LikesCount, &c.ReplyCount,
		); err != nil {
			continue
		}
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		return []PostComment{}
	}
	if comments == nil {
		return []PostComment{}
	}
	return comments
}
```

- [ ] **Step 5.5: Build Wails app to confirm no compile errors**

```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes/wails-app
go build ./... 2>&1
echo "EXIT:$?"
```

Expected: `EXIT:0`

- [ ] **Step 5.6: Commit**

```bash
git add wails-app/app.go
git commit -m "feat: add GetPersonPosts, GetPostDetail, GetPostComments Wails API"
```

---

## Chunk 3: Frontend

### Task 6: Add API bindings

**Files:**
- Modify: `wails-app/frontend/src/services/api.js`

- [ ] **Step 6.1: Add three new entries inside the `api` object**

In `api.js`, find the line `getPeopleTagsMap: ...`. Add the following three entries right after `getPersonInteractions`:

```js
  getPersonPosts:   (personId) => GoApp.GetPersonPosts(personId).catch(() => []),
  getPostDetail:    (postId)   => GoApp.GetPostDetail(postId).catch(() => null),
  getPostComments:  (postId)   => GoApp.GetPostComments(postId).catch(() => []),
```

- [ ] **Step 6.2: Commit**

```bash
git add wails-app/frontend/src/services/api.js
git commit -m "feat: add getPersonPosts, getPostDetail, getPostComments to api.js"
```

---

### Task 7: Update App.jsx navigation

**Files:**
- Modify: `wails-app/frontend/src/App.jsx`

- [ ] **Step 7.1: Add postId state and import PostDetail**

At the top of `App.jsx`, add the import:
```jsx
import PostDetail from './pages/PostDetail.jsx'
```

Inside the `App` component, after `const [profileId, setProfileId] = useState(null)`, add:
```jsx
const [postId, setPostId] = useState(null)
```

- [ ] **Step 7.2: Add openPost / closePost callbacks**

After the `closeProfile` useCallback, add:
```jsx
const openPost = useCallback((id) => {
  setPostId(id)
  setActivePage('postDetail')
}, [])

const closePost = useCallback(() => {
  setPostId(null)
  setActivePage('profile')
}, [])
```

- [ ] **Step 7.3: Add postDetail to pages and pass onOpenPost to Profile**

In the `pages` object, update the `profile` entry and add `postDetail`:

```jsx
profile: <Profile
  id={profileId}
  onBack={closeProfile}
  onOpenURL={api.openURL}
  onOpenPost={openPost}
/>,
postDetail: <PostDetail
  id={postId}
  onBack={closePost}
  onOpenURL={api.openURL}
/>,
```

- [ ] **Step 7.4: Verify the file renders without error**

```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes/wails-app/frontend
# If npm/node available, run linter or just check syntax
node --input-type=module < /dev/null 2>&1 || true
grep -n "postDetail\|openPost\|closePost\|PostDetail" src/App.jsx
```

Expected: all four identifiers appear in the file.

- [ ] **Step 7.5: Commit**

```bash
git add wails-app/frontend/src/App.jsx
git commit -m "feat: add postDetail page and navigation to App.jsx"
```

---

### Task 8: Add PostsSection to Profile.jsx

**Files:**
- Modify: `wails-app/frontend/src/pages/Profile.jsx`

- [ ] **Step 8.1: Add needed imports**

The existing `Profile.jsx` import from `lucide-react` already includes `Heart` and `ExternalLink`. It does NOT include `MessageCircle`, `ChevronDown`, or `ChevronRight` — these three must be added.

> **Important:** The file imports `MessageSquare`, NOT `MessageCircle`. These are different icons. The `PostsSection` component uses `MessageCircle` (speech bubble with tail). Add `MessageCircle`, `ChevronDown`, `ChevronRight` to the existing lucide-react import line — do not replace `MessageSquare`.

Verify and add:
```bash
grep "MessageCircle\|ChevronDown\|ChevronRight" \
  /Users/morteza/Desktop/monoes/mono-agent/newmonoes/wails-app/frontend/src/pages/Profile.jsx
```

Expected: no output (they are not there yet). Add them to the import.

- [ ] **Step 8.2: Add the PostsSection component**

Add this component function in `Profile.jsx`, before the main `export default function Profile` declaration:

```jsx
function PostsSection({ personId, onOpenPost, onOpenURL }) {
  const [posts, setPosts]       = useState([])
  const [open, setOpen]         = useState(false)
  const [loading, setLoading]   = useState(true)

  useEffect(() => {
    api.getPersonPosts(personId).then(data => {
      const rows = data || []
      setPosts(rows)
      setOpen(rows.length > 0)
      setLoading(false)
    })
  }, [personId])

  return (
    <div className="profile-section">
      {/* Header row — always visible */}
      <button
        onClick={() => setOpen(o => !o)}
        style={{
          display: 'flex', alignItems: 'center', gap: 6,
          background: 'none', border: 'none', cursor: 'pointer',
          width: '100%', padding: 0, textAlign: 'left',
        }}
      >
        {open
          ? <ChevronDown size={13} style={{ color: 'var(--text-muted)' }} />
          : <ChevronRight size={13} style={{ color: 'var(--text-muted)' }} />
        }
        <span className="profile-section-title" style={{ margin: 0 }}>
          Posts
          <span style={{ color: 'var(--text-muted)', fontWeight: 400, marginLeft: 8, fontSize: 11 }}>
            {loading ? '…' : posts.length}
          </span>
        </span>
      </button>

      {/* Collapsible body */}
      {open && (
        <div style={{ marginTop: 10 }}>
          {loading ? (
            <div style={{ padding: '12px 0', textAlign: 'center' }}>
              <div className="spinner" style={{ width: 14, height: 14, margin: '0 auto' }} />
            </div>
          ) : posts.length === 0 ? (
            <div style={{
              padding: '16px 0', textAlign: 'center',
              color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', fontSize: 11,
            }}>
              No posts scraped yet — run list_user_posts to populate
            </div>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              {posts.map(post => (
                <div key={post.id} style={{
                  display: 'flex', alignItems: 'center', gap: 10,
                  padding: '6px 8px', borderRadius: 6,
                  background: 'var(--elevated)',
                  border: '1px solid var(--border)',
                }}>
                  {/* Shortcode link → PostDetail */}
                  <button
                    onClick={() => onOpenPost(post.id)}
                    style={{
                      fontFamily: 'var(--font-mono)', fontSize: 11,
                      color: 'var(--cyan)', background: 'none', border: 'none',
                      cursor: 'pointer', padding: 0, flexShrink: 0,
                    }}
                  >
                    {post.shortcode}
                  </button>

                  {/* External link */}
                  <button
                    onClick={() => onOpenURL(post.url)}
                    style={{
                      background: 'none', border: 'none', cursor: 'pointer',
                      color: 'var(--text-muted)', padding: 0, display: 'flex',
                      opacity: 0.5, flexShrink: 0,
                    }}
                  >
                    <ExternalLink size={10} />
                  </button>

                  {/* Stats */}
                  <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginLeft: 4 }}>
                    <span style={{
                      display: 'flex', alignItems: 'center', gap: 3,
                      fontFamily: 'var(--font-mono)', fontSize: 10,
                      color: 'var(--text-muted)',
                    }}>
                      <Heart size={10} /> {post.like_count ?? '—'}
                    </span>
                    <span style={{
                      display: 'flex', alignItems: 'center', gap: 3,
                      fontFamily: 'var(--font-mono)', fontSize: 10,
                      color: 'var(--text-muted)',
                    }}>
                      <MessageCircle size={10} /> {post.comment_count ?? '—'}
                    </span>
                  </div>

                  {/* We interacted badges */}
                  {post.we_liked && (
                    <span style={{
                      padding: '1px 6px', borderRadius: 4,
                      background: 'rgba(0,180,216,0.12)',
                      border: '1px solid rgba(0,180,216,0.3)',
                      color: '#00b4d8', fontSize: 9,
                      fontFamily: 'var(--font-mono)',
                    }}>
                      ♥ liked
                    </span>
                  )}
                  {post.we_commented && (
                    <span style={{
                      padding: '1px 6px', borderRadius: 4,
                      background: 'rgba(124,58,237,0.12)',
                      border: '1px solid rgba(124,58,237,0.3)',
                      color: '#a855f7', fontSize: 9,
                      fontFamily: 'var(--font-mono)',
                    }}>
                      💬 commented
                    </span>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
```

- [ ] **Step 8.3: Update Profile export to accept onOpenPost prop and insert PostsSection**

In the main `Profile` component function signature (find `export default function Profile`), add `onOpenPost` to props:

```jsx
export default function Profile({ id, onBack, onOpenURL, onOpenPost }) {
```

Then find the `{/* ── Interaction history ── */}` comment (around line 283). Insert the `<PostsSection>` component immediately before it:

```jsx
{/* ── Posts section ── */}
<PostsSection
  personId={id}
  onOpenPost={onOpenPost}
  onOpenURL={onOpenURL}
/>

{/* ── Interaction history ── */}
```

- [ ] **Step 8.4: Commit**

```bash
git add wails-app/frontend/src/pages/Profile.jsx
git commit -m "feat: add PostsSection to Profile page"
```

---

### Task 9: Create PostDetail.jsx

**Files:**
- Create: `wails-app/frontend/src/pages/PostDetail.jsx`

- [ ] **Step 9.1: Create the PostDetail page**

```jsx
import { useState, useEffect } from 'react'
import { ArrowLeft, Heart, MessageCircle, ExternalLink } from 'lucide-react'
import { api } from '../services/api.js'

export default function PostDetail({ id, onBack, onOpenURL }) {
  const [post, setPost]         = useState(null)
  const [comments, setComments] = useState([])
  const [loading, setLoading]   = useState(true)

  useEffect(() => {
    if (!id) { setLoading(false); return }
    Promise.all([
      api.getPostDetail(id),
      api.getPostComments(id),
    ]).then(([p, c]) => {
      setPost(p)
      setComments(c || [])
      setLoading(false)
    })
  }, [id])

  if (loading) {
    return (
      <div className="page-body" style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: 200 }}>
        <div className="spinner" />
      </div>
    )
  }

  if (!post) {
    return (
      <div className="page-body">
        <button className="btn btn-ghost btn-sm" onClick={onBack} style={{ gap: 5, marginBottom: 16 }}>
          <ArrowLeft size={13} /> Back
        </button>
        <div className="empty-state">Post not found.</div>
      </div>
    )
  }

  return (
    <div className="page-scroll">
      <div className="page-header">
        <div className="page-header-left" style={{ gap: 10 }}>
          <button className="btn btn-ghost btn-sm" onClick={onBack} style={{ gap: 5 }}>
            <ArrowLeft size={13} /> Back
          </button>
          <div className="page-title" style={{ fontFamily: 'var(--font-mono)', fontSize: 16 }}>
            {post.shortcode}
          </div>
        </div>
        <div className="page-header-right">
          <button
            className="btn btn-ghost btn-sm"
            onClick={() => onOpenURL(post.url)}
            style={{ gap: 5 }}
          >
            <ExternalLink size={12} /> Open Post
          </button>
        </div>
      </div>

      <div className="page-body">
        {/* Meta row */}
        <div style={{
          display: 'flex', alignItems: 'center', gap: 12,
          padding: '10px 0 16px',
          borderBottom: '1px solid var(--border)',
          marginBottom: 16,
          flexWrap: 'wrap',
        }}>
          <span style={{
            display: 'flex', alignItems: 'center', gap: 4,
            fontFamily: 'var(--font-mono)', fontSize: 12,
            color: 'var(--text-muted)',
          }}>
            <Heart size={12} /> {post.like_count ?? '—'} likes
          </span>
          <span style={{
            display: 'flex', alignItems: 'center', gap: 4,
            fontFamily: 'var(--font-mono)', fontSize: 12,
            color: 'var(--text-muted)',
          }}>
            <MessageCircle size={12} /> {post.comment_count ?? '—'} comments
          </span>
          {post.scraped_at && (
            <span style={{
              fontFamily: 'var(--font-mono)', fontSize: 10,
              color: 'var(--text-dim)',
              marginLeft: 'auto',
            }}>
              scraped {post.scraped_at.slice(0, 10)}
            </span>
          )}
        </div>

        {/* Caption */}
        {post.caption && (
          <div style={{
            fontFamily: 'var(--font-mono)', fontSize: 12,
            color: 'var(--text-secondary)',
            padding: '0 0 16px',
            borderBottom: '1px solid var(--border)',
            marginBottom: 16,
            lineHeight: 1.6,
          }}>
            {post.caption}
          </div>
        )}

        {/* Comments section */}
        <div className="profile-section-title" style={{ marginBottom: 12 }}>
          Comments
          <span style={{ color: 'var(--text-muted)', fontWeight: 400, marginLeft: 8, fontSize: 11 }}>
            {comments.length}
          </span>
        </div>

        {comments.length === 0 ? (
          <div style={{
            padding: '24px 0', textAlign: 'center',
            color: 'var(--text-muted)', fontFamily: 'var(--font-mono)', fontSize: 12,
          }}>
            No comments scraped yet — run list_post_comments to populate
          </div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
            {comments.map((c, i) => (
              <div key={c.id || i} style={{
                display: 'grid',
                gridTemplateColumns: '130px 1fr auto',
                gap: 10,
                padding: '8px 10px',
                borderRadius: 5,
                background: i % 2 === 0 ? 'var(--elevated)' : 'transparent',
                alignItems: 'start',
              }}>
                {/* Author */}
                <span style={{
                  fontFamily: 'var(--font-mono)', fontSize: 11,
                  color: '#00b4d8', flexShrink: 0,
                  overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                }}>
                  @{c.author}
                </span>

                {/* Text */}
                <span style={{
                  fontSize: 12, color: 'var(--text)',
                  lineHeight: 1.5, wordBreak: 'break-word',
                }}>
                  {c.text || <span style={{ color: 'var(--text-muted)', fontStyle: 'italic' }}>—</span>}
                </span>

                {/* Right side: timestamp + likes */}
                <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 3, flexShrink: 0 }}>
                  {c.timestamp && (
                    <span style={{
                      fontFamily: 'var(--font-mono)', fontSize: 9,
                      color: 'var(--text-dim)', whiteSpace: 'nowrap',
                    }}>
                      {c.timestamp.slice(0, 10)}
                    </span>
                  )}
                  {c.likes_count > 0 && (
                    <span style={{
                      display: 'flex', alignItems: 'center', gap: 2,
                      fontFamily: 'var(--font-mono)', fontSize: 9,
                      color: 'var(--text-muted)',
                    }}>
                      <Heart size={9} /> {c.likes_count}
                    </span>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
```

- [ ] **Step 9.2: Commit**

```bash
git add wails-app/frontend/src/pages/PostDetail.jsx
git commit -m "feat: add PostDetail page with comments list"
```

---

### Task 10: End-to-end CLI verification

- [ ] **Step 10.1: Confirm posts in DB**

```bash
sqlite3 ~/.monoes/monoes.db \
  "SELECT shortcode, like_count, comment_count, person_id FROM posts WHERE platform='INSTAGRAM' LIMIT 5;"
```

Expected: rows with shortcodes and a non-null person_id for mortezanoes.

- [ ] **Step 10.2: Confirm comments in DB**

```bash
sqlite3 ~/.monoes/monoes.db \
  "SELECT pc.author, pc.text, pc.likes_count FROM post_comments pc JOIN posts p ON pc.post_id = p.id LIMIT 5;"
```

Expected: rows with author names and comment text.

- [ ] **Step 10.3: Confirm people list still works**

```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes
go run ./cmd/monoes people list 2>&1 | head -5
```

Expected: mortezanoes still at top of list, no errors.

- [ ] **Step 10.4: Final build check (both CLI and Wails)**

```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes
go build ./cmd/monoes/... 2>&1 && echo "CLI OK"

cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes/wails-app
go build ./... 2>&1 && echo "WAILS OK"
```

Expected: both print OK. Note: Wails app has its own `go.mod` so it must be built from within its directory.

- [ ] **Step 10.5: Commit**

```bash
git add -A
git commit -m "feat: posts and comments feature complete"
```
