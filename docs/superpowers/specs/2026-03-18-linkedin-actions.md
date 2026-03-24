# LinkedIn Action Nodes — Design Spec

**Date:** 2026-03-18
**Status:** Draft

## Goal

Add 5 LinkedIn action nodes covering read (list) and write (like, comment) operations on posts and comments, for both personal profiles and company pages.

## Decisions

| # | Decision |
|---|----------|
| 1 | `like_posts` supports all 6 LinkedIn reactions: `like\|celebrate\|support\|love\|insightful\|funny` |
| 2 | `list_post_comments` has `includeReplies: true` by default — reply expansion is attempted per comment |
| 3 | `list_user_posts` has `activityType: shares\|all` input, default `all` |
| 4 | `comment_on_posts` handles top-level comments AND replies via optional `parentCommentId` input |
| 5 | Both personal profiles (`/in/username/`) and company pages (`/company/slug/`) are supported |

---

## Actions Summary

| Action | Go method | JSON file | Loop |
|--------|-----------|-----------|------|
| `linkedin.list_user_posts` | `ListUserPosts` | `list_user_posts.json` | No |
| `linkedin.list_post_comments` | `ListPostComments` | `list_post_comments.json` | Over `selectedListItems` (post URLs) |
| `linkedin.like_posts` | `LikePost` | `like_posts.json` | Over `selectedListItems` |
| `linkedin.comment_on_posts` | `CommentOnPost` | `comment_on_posts.json` | Over `selectedListItems` |
| `linkedin.like_comments` | `LikeComment` | `like_comments.json` | Over `selectedListItems` |

---

## Go Methods — `internal/bot/linkedin/actions.go`

### 1. `ListUserPosts`

```go
func (b *LinkedInBot) ListUserPosts(
    ctx context.Context,
    page *rod.Page,
    profileURL string,   // e.g. "https://www.linkedin.com/in/username/" or "/company/slug/"
    maxCount int,
    activityType string, // "shares" | "all"
) ([]map[string]interface{}, error)
```

**URL logic:**
- Personal profile: append `recent-activity/shares/` (activityType=shares) or `recent-activity/all/` (activityType=all)
- Company page (URL contains `/company/`): use `{companyURL}posts/` (LinkedIn company posts URL)
- Normalize trailing slashes before appending

**Scroll & collect strategy:**
- Navigate to the activity URL, wait for page load + 3s
- JS: collect all post links matching `/posts/` or `/feed/update/` patterns, deduplicate by `activity_id`
- Scroll down, re-collect, repeat until `maxCount` reached or no new posts appear (3 scroll attempts with no new posts = done)

**Returned item schema:**
```json
{
  "url": "https://www.linkedin.com/posts/...",
  "activity_id": "7123456789012345678",
  "text_preview": "First 200 chars of post text",
  "author": "Full Name",
  "author_url": "https://www.linkedin.com/in/username/",
  "timestamp": "2024-01-15T10:30:00Z",
  "likes_count": 42,
  "comments_count": 7,
  "reposts_count": 3
}
```

**Activity ID extraction from URL:**
- From `/posts/slug-activity-7123456789012345678-XXXX` → match `activity-(\d+)`
- From `/feed/update/urn:li:activity:7123456789012345678` → match `activity:(\d+)`

**DB auto-save note:** The existing `node.go` auto-save block triggers on `strings.HasSuffix(nodeType, "list_user_posts")`. LinkedIn's `linkedin.list_user_posts` matches this. `savePostsToDB` uses `item.JSON["shortcode"]` and falls back to `extractPostShortcode(item.JSON["url"])`. The `extractPostShortcode` function must be extended (see node.go changes).

---

### 2. `ListPostComments`

```go
func (b *LinkedInBot) ListPostComments(
    ctx context.Context,
    page *rod.Page,
    postURL string,
    maxCount int,
    includeReplies bool,
) ([]map[string]interface{}, error)
```

**Strategy:**
- Navigate to post URL, wait for load + 3s
- Click "Load more comments" buttons until `maxCount` top-level comments collected or none left
- For each top-level comment, if `includeReplies: true` and "Load N replies" button exists, click it and collect replies with `parent_id` set to parent's `id`
- Comment identification: use `data-id` attribute (contains URN string)

**Returned item schema:**
```json
{
  "id": "urn:li:comment:(urn:li:activity:7123456789,7234567890)",
  "author": "Full Name",
  "author_url": "https://www.linkedin.com/in/username/",
  "text": "Comment text",
  "timestamp": "2024-01-15T11:00:00Z",
  "likes_count": 5,
  "reply_count": 2,
  "parent_id": null
}
```

For replies: `parent_id` = the parent comment's `id` (URN string), `reply_count` = 0.

**DB auto-save note:** `strings.HasSuffix(nodeType, "list_post_comments")` matches. `saveCommentsToDB` uses `item.JSON["author"]` and `item.JSON["timestamp"]` as dedup key. The `post_id` is resolved by `extractPostShortcode(targets[0].url)` — must be extended for LinkedIn URLs.

---

### 3. `LikePost`

```go
func (b *LinkedInBot) LikePost(
    ctx context.Context,
    page *rod.Page,
    postURL string,
    reaction string, // "like"|"celebrate"|"support"|"love"|"insightful"|"funny", default "like"
) error
```

**Strategy (JS marker pattern):**
1. Navigate to post URL, wait + 3s
2. JS: find the reaction button (aria-label contains "React Like" or similar). If the post is already reacted with the target reaction → return early ("already_reacted")
3. If reaction == "like": click the button directly
4. If reaction != "like": hover the reaction button (trigger hover event via JS or `element.Hover()`), wait 1s for popup, click the specific reaction button (aria-label matches reaction name)
5. Wait 2s, verify by checking aria-label changed to "Remove reaction" or the reaction name

**LinkedIn reaction button selectors (aria-labels):**
- Main button: `button[aria-label*="React Like"]` or `button[aria-label="Like"]`
- Already reacted: `button[aria-label*="Remove your"]` or button containing active reaction icon
- Reaction popup options: `button[aria-label="Celebrate"]`, `button[aria-label="Support"]`, etc.

**If already reacted with a different reaction:** remove first (click again to toggle off), then apply target reaction.

---

### 4. `CommentOnPost`

```go
func (b *LinkedInBot) CommentOnPost(
    ctx context.Context,
    page *rod.Page,
    postURL string,
    commentText string,
    parentCommentID string, // empty = top-level comment; URN string = reply to that comment
) error
```

**Strategy:**
- Navigate to post URL, wait + 3s
- If `parentCommentID == ""`:
  - JS: find and mark the "Add a comment" contenteditable div (aria-label contains "Add a comment")
  - Click it, type `commentText` rune-by-rune, wait for Post button, click it
- If `parentCommentID != ""`:
  - JS: find the comment element with `data-id` attribute matching `parentCommentID`, find its "Reply" button, mark it
  - Click Reply → a reply input opens within that comment's container
  - JS: find and mark the reply contenteditable that appeared within the parent comment's container
  - Type `commentText` rune-by-rune, find and click the Post/Reply submit button
- Wait 3s, clean up JS markers

**LinkedIn comment input selector:**
- Top-level: `div.comments-comment-box__form div.ql-editor[contenteditable="true"]`
- Reply: same selector but scoped to the comment's reply container

---

### 5. `LikeComment`

```go
func (b *LinkedInBot) LikeComment(
    ctx context.Context,
    page *rod.Page,
    postURL string,
    commentID string, // URN string e.g. "urn:li:comment:(urn:li:activity:123,456)"
) error
```

**Strategy:**
1. Navigate to post URL, wait + 3s
2. JS: find element with `data-id` attribute exactly matching `commentID`. If not visible, scroll it into view. Find the Like button within that element's subtree. Check if already liked (aria-label "Remove Like from comment" or similar). Mark the Like button with `data-monoes-comment-like`.
3. Rod: find `[data-monoes-comment-like='true']`, scroll into view, click
4. Wait 1s, clean up marker

---

## JSON Action Files — `data/actions/linkedin/`

### `list_user_posts.json`

```json
{
  "actionType": "list_user_posts",
  "platform": "LINKEDIN",
  "version": "1.0.0",
  "description": "List posts from a LinkedIn profile or company page",
  "metadata": { "requiresAuth": true, "supportsPagination": false, "supportsRetry": true },
  "inputs": {
    "required": [
      { "name": "targets", "type": "list", "description": "Profile/company URLs to scrape posts from" }
    ],
    "optional": [
      { "name": "maxCount",     "type": "number",  "default": 20,   "min": 1, "max": 200 },
      { "name": "activityType", "type": "select",  "default": "all", "options": ["all", "shares"] },
      { "name": "delayAfterLoad", "type": "number", "default": 3, "min": 1, "max": 30 }
    ]
  },
  "outputs": { "success": ["posts", "postsCount"], "failure": [] },
  "steps": [
    {
      "id": "t1_list_posts",
      "type": "call_bot_method",
      "methodName": "list_user_posts",
      "args": ["{{target.url}}", "{{maxCount or 20}}", "{{activityType or 'all'}}"],
      "variable_name": "posts",
      "timeout": 120,
      "onError": { "action": "skip" }
    },
    { "id": "save_posts",  "type": "save_data", "dataSource": "posts" },
    { "id": "store_count", "type": "update_progress", "set": { "postsCount": "{{posts.count}}" } },
    { "id": "log_success", "type": "log", "value": "Profile: {{target.url}}, Count: {{postsCount}}" }
  ],
  "loops": [],
  "errorHandling": { "globalRetries": 2, "retryDelay": 3000, "onFinalFailure": "log_and_continue" }
}
```

### `list_post_comments.json`

```json
{
  "actionType": "list_post_comments",
  "platform": "LINKEDIN",
  "version": "1.0.0",
  "description": "List comments (and optionally replies) on LinkedIn posts",
  "metadata": { "requiresAuth": true, "supportsPagination": false, "supportsRetry": true },
  "inputs": {
    "required": [
      { "name": "selectedListItems", "type": "list", "description": "Post URLs to list comments from" }
    ],
    "optional": [
      { "name": "maxComments",     "type": "number",  "default": 50,   "min": 1, "max": 500 },
      { "name": "includeReplies",  "type": "boolean", "default": true },
      { "name": "delayBetweenPosts", "type": "number", "default": 3, "min": 1, "max": 60 }
    ]
  },
  "outputs": { "success": ["comments", "commentsCount", "reachedIndex"], "failure": ["failedItems"] },
  "steps": [
    {
      "id": "t1_list_comments",
      "type": "call_bot_method",
      "methodName": "list_post_comments",
      "args": ["{{item.url}}", "{{maxComments or 50}}", "{{includeReplies}}"],
      "variable_name": "comments",
      "timeout": 120,
      "onError": { "action": "skip" }
    },
    { "id": "save_comments", "type": "save_data", "dataSource": "comments" },
    { "id": "store_count",   "type": "update_progress", "set": { "commentsCount": "{{comments.count}}" } },
    { "id": "wait",          "type": "wait", "duration": "{{delayBetweenPosts or 3}}", "waitFor": "time" },
    { "id": "log_success",   "type": "log", "value": "Post: {{item.url}}, Comments: {{commentsCount}}" }
  ],
  "loops": [
    {
      "id": "process_posts",
      "iterator": "selectedListItems",
      "indexVar": "reachedIndex",
      "steps": ["t1_list_comments", "save_comments", "store_count", "wait", "log_success"],
      "onComplete": "update_action_state"
    }
  ],
  "errorHandling": { "globalRetries": 2, "retryDelay": 3000, "onFinalFailure": "log_and_continue" }
}
```

### `like_posts.json`

```json
{
  "actionType": "like_posts",
  "platform": "LINKEDIN",
  "version": "1.0.0",
  "description": "React to LinkedIn posts (Like, Celebrate, Support, Love, Insightful, Funny)",
  "inputs": {
    "required": [
      { "name": "selectedListItems", "type": "list", "description": "Post URLs to react to" }
    ],
    "optional": [
      { "name": "reaction",           "type": "select", "default": "like", "options": ["like","celebrate","support","love","insightful","funny"] },
      { "name": "delayBetweenLikes",  "type": "number", "default": 3, "min": 1, "max": 60 }
    ]
  },
  "outputs": { "success": ["likesCount", "reachedIndex"], "failure": ["failedItems"] },
  "steps": [
    {
      "id": "t1_like_post",
      "type": "call_bot_method",
      "methodName": "like_post",
      "args": ["{{item.url}}", "{{reaction or 'like'}}"],
      "variable_name": "likeResult",
      "timeout": 30,
      "onError": { "action": "skip" }
    },
    { "id": "wait", "type": "wait", "duration": "{{delayBetweenLikes or 3}}", "waitFor": "time" },
    { "id": "log",  "type": "log", "value": "Liked: {{item.url}}" }
  ],
  "loops": [
    {
      "id": "process_posts",
      "iterator": "selectedListItems",
      "indexVar": "reachedIndex",
      "steps": ["t1_like_post", "wait", "log"],
      "onComplete": "update_action_state"
    }
  ],
  "errorHandling": { "globalRetries": 2, "retryDelay": 3000, "onFinalFailure": "log_and_continue" }
}
```

### `comment_on_posts.json`

```json
{
  "actionType": "comment_on_posts",
  "platform": "LINKEDIN",
  "version": "1.0.0",
  "description": "Comment on LinkedIn posts or reply to a specific comment",
  "inputs": {
    "required": [
      { "name": "selectedListItems", "type": "list", "description": "Post URLs to comment on" },
      { "name": "commentText",       "type": "string", "description": "Comment text (supports {{templates}})" }
    ],
    "optional": [
      { "name": "parentCommentId",     "type": "string",  "default": "", "description": "URN of parent comment to reply to; empty for top-level comment" },
      { "name": "delayBetweenComments","type": "number",  "default": 5, "min": 2, "max": 120 }
    ]
  },
  "outputs": { "success": ["commentsCount", "reachedIndex"], "failure": ["failedItems"] },
  "steps": [
    {
      "id": "t1_comment",
      "type": "call_bot_method",
      "methodName": "comment_on_post",
      "args": ["{{item.url}}", "{{commentText}}", "{{parentCommentId or ''}}"],
      "variable_name": "commentResult",
      "timeout": 45,
      "onError": { "action": "skip" }
    },
    { "id": "wait", "type": "wait", "duration": "{{delayBetweenComments or 5}}", "waitFor": "time" },
    { "id": "log",  "type": "log", "value": "Commented on: {{item.url}}" }
  ],
  "loops": [
    {
      "id": "process_posts",
      "iterator": "selectedListItems",
      "indexVar": "reachedIndex",
      "steps": ["t1_comment", "wait", "log"],
      "onComplete": "update_action_state"
    }
  ],
  "errorHandling": { "globalRetries": 1, "retryDelay": 5000, "onFinalFailure": "log_and_continue" }
}
```

### `like_comments.json`

```json
{
  "actionType": "like_comments",
  "platform": "LINKEDIN",
  "version": "1.0.0",
  "description": "Like comments on a LinkedIn post by comment URN ID",
  "inputs": {
    "required": [
      { "name": "postUrl",            "type": "string", "description": "URL of the post containing the comments" },
      { "name": "selectedListItems",  "type": "list",   "description": "Comment URN IDs to like (from list_post_comments output)" }
    ],
    "optional": [
      { "name": "delayBetweenLikes", "type": "number", "default": 2, "min": 1, "max": 30 }
    ]
  },
  "outputs": { "success": ["likesCount", "reachedIndex"], "failure": ["failedItems"] },
  "steps": [
    {
      "id": "t1_like_comment",
      "type": "call_bot_method",
      "methodName": "like_comment",
      "args": ["{{postUrl}}", "{{item.id}}"],
      "variable_name": "likeResult",
      "timeout": 20,
      "onError": { "action": "skip" }
    },
    { "id": "wait", "type": "wait", "duration": "{{delayBetweenLikes or 2}}", "waitFor": "time" },
    { "id": "log",  "type": "log", "value": "Liked comment: {{item.id}}" }
  ],
  "loops": [
    {
      "id": "process_comments",
      "iterator": "selectedListItems",
      "indexVar": "reachedIndex",
      "steps": ["t1_like_comment", "wait", "log"],
      "onComplete": "update_action_state"
    }
  ],
  "errorHandling": { "globalRetries": 2, "retryDelay": 2000, "onFinalFailure": "log_and_continue" }
}
```

---

## node.go Changes

### Extend `extractPostShortcode` for LinkedIn URLs

Add LinkedIn URL patterns before the final `return ""`:

```go
func extractPostShortcode(postURL string) string {
    // Instagram: /p/{shortcode}/ or /reel/{shortcode}/
    parts := strings.Split(strings.Trim(postURL, "/"), "/")
    for i, p := range parts {
        if (p == "p" || p == "reel") && i+1 < len(parts) {
            return parts[i+1]
        }
    }

    // LinkedIn: activity-NNNNNNNN in URL path
    if strings.Contains(postURL, "linkedin.com") {
        re := regexp.MustCompile(`activity[-:](\d+)`)
        if m := re.FindStringSubmatch(postURL); len(m) > 1 {
            return m[1]
        }
    }

    return ""
}
```

**Import:** `"regexp"` must be added to `node.go`'s import block if not already present.

---

## DB Integration

Both `linkedin.list_user_posts` and `linkedin.list_post_comments` reuse the existing auto-save blocks in `node.go`:

- `strings.HasSuffix("linkedin.list_user_posts", "list_user_posts")` → `true` → `savePostsToDB` called
  - `platform` derived as `"LINKEDIN"` from `strings.ToUpper(strings.SplitN(nodeType, ".", 2)[0])`
  - `person_id` resolved via `SELECT id FROM people WHERE platform_username = ? AND UPPER(platform) = 'LINKEDIN'`
  - `activity_id` field from item → used as `shortcode` value in posts table

- `strings.HasSuffix("linkedin.list_post_comments", "list_post_comments")` → `true` → `saveCommentsToDB` called
  - `post_id` resolved via `SELECT id FROM posts WHERE platform = 'LINKEDIN' AND shortcode = ?` using extracted activity ID
  - `id` (URN string) stored in `post_comments.author + timestamp` dedup key

**Note:** The `activity_id` field from `ListUserPosts` output serves as `shortcode` because `savePostsToDB` checks `item.JSON["shortcode"]` first, then falls back to `extractPostShortcode(url)`. Since `ListUserPosts` returns `activity_id` not `shortcode`, the fallback path via `extractPostShortcode` is used — which is why the LinkedIn extension to that function is critical.
