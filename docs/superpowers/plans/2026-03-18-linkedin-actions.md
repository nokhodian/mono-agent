# LinkedIn Action Nodes — Implementation Plan

> **For agentic workers:** Use superpowers:subagent-driven-development to execute this plan.

**Goal:** Add 5 LinkedIn action nodes — `list_user_posts`, `list_post_comments`, `like_posts`, `comment_on_posts`, `like_comments` — with Go bot methods, JSON action definitions, and DB auto-save integration.

**Spec:** `docs/superpowers/specs/2026-03-18-linkedin-actions.md`

**Tech stack:** Go 1.21, go-rod browser automation, SQLite, JSON action files

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/bot/linkedin/actions.go` | Create | 5 `*LinkedInBot` methods |
| `data/actions/linkedin/list_user_posts.json` | Create | Action definition |
| `data/actions/linkedin/list_post_comments.json` | Create | Action definition |
| `data/actions/linkedin/like_posts.json` | Create | Action definition |
| `data/actions/linkedin/comment_on_posts.json` | Create | Action definition |
| `data/actions/linkedin/like_comments.json` | Create | Action definition |
| `cmd/monoes/node.go` | Modify | Extend `extractPostShortcode` for LinkedIn URLs |

---

## Chunk 1: Go Bot Methods

### Task 1: ListUserPosts + ListPostComments

**Files:**
- Create: `internal/bot/linkedin/actions.go`

> **Read first:** `internal/bot/linkedin/bot.go` (to understand existing `LinkedInBot` struct and imports), `internal/bot/instagram/actions.go` (pattern reference).

- [ ] **Step 1.1: Create `internal/bot/linkedin/actions.go`** with package declaration and imports:

```go
package linkedin

import (
    "context"
    "fmt"
    "regexp"
    "strings"
    "time"

    "github.com/go-rod/rod"
    "github.com/go-rod/rod/lib/proto"
)
```

- [ ] **Step 1.2: Add `ListUserPosts` method**

```go
// ListUserPosts scrapes posts from a LinkedIn personal profile or company page.
// profileURL: full URL to the profile (e.g. https://www.linkedin.com/in/username/ or /company/slug/)
// activityType: "shares" | "all"
func (b *LinkedInBot) ListUserPosts(ctx context.Context, page *rod.Page, profileURL string, maxCount int, activityType string) ([]map[string]interface{}, error) {
    if profileURL == "" {
        return nil, fmt.Errorf("linkedin: profileURL is required")
    }
    if maxCount <= 0 {
        maxCount = 20
    }
    if activityType == "" {
        activityType = "all"
    }

    // Build the activity feed URL.
    base := strings.TrimRight(profileURL, "/") + "/"
    var feedURL string
    if strings.Contains(base, "/company/") {
        feedURL = base + "posts/"
    } else {
        if activityType == "shares" {
            feedURL = base + "recent-activity/shares/"
        } else {
            feedURL = base + "recent-activity/all/"
        }
    }

    err := page.Navigate(feedURL)
    if err != nil {
        return nil, fmt.Errorf("linkedin: failed to navigate to activity feed: %w", err)
    }
    if err := page.WaitLoad(); err != nil {
        return nil, fmt.Errorf("linkedin: activity feed did not load: %w", err)
    }
    time.Sleep(3 * time.Second)

    activityRe := regexp.MustCompile(`activity[-:](\d+)`)

    var allPosts []map[string]interface{}
    seen := map[string]bool{}
    noNewCount := 0

    for len(allPosts) < maxCount && noNewCount < 3 {
        res, err := page.Eval(`() => {
            const posts = [];
            // Collect all elements that look like post links.
            const links = Array.from(document.querySelectorAll('a[href*="/posts/"], a[href*="/feed/update/"]'));
            for (const a of links) {
                const href = a.href || '';
                if (!href) continue;
                // Extract author name from nearby context.
                const article = a.closest('div[data-id], article, li');
                let author = '', authorUrl = '', textPreview = '', timestamp = '', likesCount = 0, commentsCount = 0, repostsCount = 0;
                if (article) {
                    const nameEl = article.querySelector('span.feed-shared-actor__name, span.update-components-actor__name, a.update-components-actor__meta-link span[aria-hidden="true"]');
                    if (nameEl) author = nameEl.innerText.trim();
                    const authorLink = article.querySelector('a[href*="/in/"], a[href*="/company/"]');
                    if (authorLink) authorUrl = authorLink.href;
                    const textEl = article.querySelector('span.break-words, div.feed-shared-update-v2__description, div.update-components-text');
                    if (textEl) textPreview = textEl.innerText.trim().slice(0, 200);
                    const timeEl = article.querySelector('time, span[class*="time"]');
                    if (timeEl) timestamp = timeEl.getAttribute('datetime') || timeEl.innerText.trim();
                    const socialEl = article.querySelector('span.social-details-social-counts__reactions-count, button[aria-label*="reaction"]');
                    if (socialEl) likesCount = parseInt(socialEl.innerText.replace(/[^0-9]/g,'')) || 0;
                }
                posts.push({ url: href, author, authorUrl, textPreview, timestamp, likesCount, commentsCount, repostsCount });
            }
            return JSON.stringify(posts);
        }`)
        if err != nil {
            break
        }

        var rawPosts []map[string]interface{}
        if jsonErr := jsonUnmarshal(res.Value.Str(), &rawPosts); jsonErr != nil {
            break
        }

        newThisScroll := 0
        for _, p := range rawPosts {
            rawURL, _ := p["url"].(string)
            if rawURL == "" {
                continue
            }
            m := activityRe.FindStringSubmatch(rawURL)
            if len(m) < 2 {
                continue
            }
            activityID := m[1]
            if seen[activityID] {
                continue
            }
            seen[activityID] = true
            newThisScroll++
            post := map[string]interface{}{
                "url":            rawURL,
                "activity_id":    activityID,
                "shortcode":      activityID, // so savePostsToDB finds it via item.JSON["shortcode"]
                "text_preview":   p["textPreview"],
                "author":         p["author"],
                "author_url":     p["authorUrl"],
                "timestamp":      p["timestamp"],
                "likes_count":    p["likesCount"],
                "comments_count": p["commentsCount"],
                "reposts_count":  p["repostsCount"],
            }
            allPosts = append(allPosts, post)
            if len(allPosts) >= maxCount {
                break
            }
        }

        if newThisScroll == 0 {
            noNewCount++
        } else {
            noNewCount = 0
        }

        if len(allPosts) < maxCount {
            page.Eval(`() => window.scrollBy(0, window.innerHeight * 2)`)
            time.Sleep(2 * time.Second)
        }
    }

    return allPosts, nil
}
```

**Note:** The `jsonUnmarshal` helper is already defined in `internal/bot/instagram/actions.go` but is package-private. Implement it locally in `actions.go`:

```go
func jsonUnmarshal(s string, v interface{}) error {
    return json.Unmarshal([]byte(s), v)
}
```

Add `"encoding/json"` to the imports.

- [ ] **Step 1.3: Add `ListPostComments` method**

```go
// ListPostComments scrapes comments (and optionally replies) from a LinkedIn post.
func (b *LinkedInBot) ListPostComments(ctx context.Context, page *rod.Page, postURL string, maxCount int, includeReplies bool) ([]map[string]interface{}, error) {
    if postURL == "" {
        return nil, fmt.Errorf("linkedin: postURL is required")
    }
    if maxCount <= 0 {
        maxCount = 50
    }

    err := page.Navigate(postURL)
    if err != nil {
        return nil, fmt.Errorf("linkedin: failed to navigate to post: %w", err)
    }
    if err := page.WaitLoad(); err != nil {
        return nil, fmt.Errorf("linkedin: post page did not load: %w", err)
    }
    time.Sleep(3 * time.Second)

    // Click "Load more comments" until maxCount reached or no more.
    for i := 0; i < 10; i++ {
        loadMoreRes, _ := page.Eval(`() => {
            const btns = Array.from(document.querySelectorAll('button'));
            const loadMore = btns.find(b => b.innerText.match(/load more comments/i) || b.innerText.match(/show more/i));
            if (loadMore) { loadMore.click(); return true; }
            return false;
        }`)
        if loadMoreRes == nil || !loadMoreRes.Value.Bool() {
            break
        }
        time.Sleep(2 * time.Second)
    }

    // If includeReplies, click "Load X replies" on each comment.
    if includeReplies {
        for i := 0; i < 20; i++ {
            repliesRes, _ := page.Eval(`() => {
                const btns = Array.from(document.querySelectorAll('button'));
                const loadReplies = btns.find(b => b.innerText.match(/load \d+ repl/i) || b.innerText.match(/view repl/i));
                if (loadReplies) { loadReplies.click(); return true; }
                return false;
            }`)
            if repliesRes == nil || !repliesRes.Value.Bool() {
                break
            }
            time.Sleep(1500 * time.Millisecond)
        }
    }

    // Extract all comments.
    res, err := page.Eval(`() => {
        const comments = [];
        const commentEls = document.querySelectorAll('article.comments-comment-item, div[class*="comment-item"]');
        for (const el of commentEls) {
            const id = el.getAttribute('data-id') || el.getAttribute('id') || '';
            const authorEl = el.querySelector('span.comments-post-meta__name, a.comments-post-meta__name span');
            const author = authorEl ? authorEl.innerText.trim() : '';
            const authorLinkEl = el.querySelector('a[href*="/in/"]');
            const authorUrl = authorLinkEl ? authorLinkEl.href : '';
            const textEl = el.querySelector('span.comments-comment-item__main-content, div.update-components-text');
            const text = textEl ? textEl.innerText.trim() : '';
            const timeEl = el.querySelector('time');
            const timestamp = timeEl ? (timeEl.getAttribute('datetime') || timeEl.innerText.trim()) : '';
            const likeEl = el.querySelector('button[aria-label*="Like"][aria-label*="comment"], span.social-details-social-counts__reactions-count');
            const likesCount = likeEl ? (parseInt(likeEl.getAttribute('aria-label')) || 0) : 0;
            // Detect if this is a reply (nested inside another comment).
            const isReply = !!el.closest('div.comments-comment-item__nested-items, div[class*="nested"]');
            const parentEl = isReply ? el.closest('article.comments-comment-item, div[class*="comment-item"]:not(article)') : null;
            const parentId = parentEl ? (parentEl.getAttribute('data-id') || '') : '';
            comments.push({ id, author, authorUrl, text, timestamp, likesCount, replyCount: 0, parentId: parentId || null });
        }
        return JSON.stringify(comments);
    }`)
    if err != nil {
        return nil, fmt.Errorf("linkedin: failed to extract comments: %w", err)
    }

    var rawComments []map[string]interface{}
    if err := jsonUnmarshal(res.Value.Str(), &rawComments); err != nil {
        return nil, fmt.Errorf("linkedin: failed to parse comments JSON: %w", err)
    }

    // Remap keys to match saveCommentsToDB expected field names.
    var result []map[string]interface{}
    for i, c := range rawComments {
        if i >= maxCount {
            break
        }
        result = append(result, map[string]interface{}{
            "id":          c["id"],
            "author":      c["author"],
            "author_url":  c["authorUrl"],
            "text":        c["text"],
            "timestamp":   c["timestamp"],
            "likes_count": c["likesCount"],
            "reply_count": c["replyCount"],
            "parent_id":   c["parentId"],
        })
    }

    return result, nil
}
```

- [ ] **Step 1.4: Build to verify**

```bash
cd /Users/morteza/Desktop/monoes/monoes-agent/newmonoes
go build ./internal/bot/linkedin/... 2>&1
echo "EXIT:$?"
```

Expected: `EXIT:0`

- [ ] **Step 1.5: Commit**

```bash
git add internal/bot/linkedin/actions.go
git commit -m "feat: add ListUserPosts and ListPostComments to LinkedIn bot"
```

---

### Task 2: LikePost + CommentOnPost + LikeComment

**Files:**
- Modify: `internal/bot/linkedin/actions.go` (append to existing file)

> **Pattern reference:** read `internal/bot/instagram/bot.go` `LikePost` method and `internal/bot/instagram/actions.go` `CommentPost` method for the JS marker pattern and rod interaction style.

- [ ] **Step 2.1: Add `LikePost` method** (append to `actions.go`)

```go
// LikePost reacts to a LinkedIn post with the specified reaction.
// reaction: "like"|"celebrate"|"support"|"love"|"insightful"|"funny" (default: "like")
func (b *LinkedInBot) LikePost(ctx context.Context, page *rod.Page, postURL string, reaction string) error {
    if postURL == "" {
        return fmt.Errorf("linkedin: postURL is required")
    }
    if reaction == "" {
        reaction = "like"
    }

    if err := page.Navigate(postURL); err != nil {
        return fmt.Errorf("linkedin: failed to navigate to post: %w", err)
    }
    if err := page.WaitLoad(); err != nil {
        return fmt.Errorf("linkedin: post page did not load: %w", err)
    }
    time.Sleep(3 * time.Second)

    // JS: find the main reaction button, check current state, mark it.
    res, err := page.Eval(`() => {
        // Clean up previous markers.
        const prev = document.querySelector('[data-monoes-reaction-btn]');
        if (prev) prev.removeAttribute('data-monoes-reaction-btn');

        // Find the reaction button for the post (not for comments).
        // LinkedIn uses buttons with aria-label like "React Like" or "Like" in the action bar.
        const actionBars = document.querySelectorAll('div.feed-shared-social-action-bar, div.social-actions-button, div[class*="social-actions"]');
        let reactionBtn = null;
        for (const bar of actionBars) {
            const btn = bar.querySelector('button[aria-label*="Like"], button[aria-label*="React"]');
            if (btn) { reactionBtn = btn; break; }
        }
        // Fallback: any Like/React button not inside a comment.
        if (!reactionBtn) {
            const allBtns = document.querySelectorAll('button[aria-label*="React Like"], button[aria-label*="Like"]');
            for (const btn of allBtns) {
                if (!btn.closest('article.comments-comment-item') && !btn.closest('div[class*="comment-item"]')) {
                    reactionBtn = btn;
                    break;
                }
            }
        }
        if (!reactionBtn) return 'not_found';

        // Check if already reacted with this reaction.
        const label = (reactionBtn.getAttribute('aria-label') || '').toLowerCase();
        if (label.includes('remove') || label.includes('unlike')) return 'already_reacted';

        reactionBtn.setAttribute('data-monoes-reaction-btn', 'true');
        return 'marked';
    }`)
    if err != nil {
        return fmt.Errorf("linkedin: failed to evaluate reaction script: %w", err)
    }

    state := res.Value.Str()
    if state == "already_reacted" {
        return nil
    }
    if state != "marked" {
        return fmt.Errorf("linkedin: could not find reaction button on %s (%s)", postURL, state)
    }

    reactionBtn, err := page.Timeout(5 * time.Second).Element("[data-monoes-reaction-btn='true']")
    if err != nil {
        return fmt.Errorf("linkedin: marked reaction button not found: %w", err)
    }
    reactionBtn.MustScrollIntoView()
    time.Sleep(300 * time.Millisecond)

    if reaction == "like" {
        // Simple click for Like.
        if err := reactionBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
            return fmt.Errorf("linkedin: failed to click Like: %w", err)
        }
    } else {
        // Hover to open reaction popup, then click the specific reaction.
        if err := reactionBtn.Hover(); err != nil {
            return fmt.Errorf("linkedin: failed to hover reaction button: %w", err)
        }
        time.Sleep(1 * time.Second)

        // Map reaction name to aria-label (LinkedIn capitalizes them).
        reactionLabel := strings.Title(reaction) // "celebrate" → "Celebrate"
        popupBtn, popupErr := page.Timeout(5 * time.Second).Element(
            fmt.Sprintf("button[aria-label='%s']", reactionLabel),
        )
        if popupErr != nil {
            // Fallback: click Like if popup not found.
            if err := reactionBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
                return fmt.Errorf("linkedin: fallback Like click failed: %w", err)
            }
        } else {
            if err := popupBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
                return fmt.Errorf("linkedin: failed to click %s reaction: %w", reaction, err)
            }
        }
    }

    time.Sleep(2 * time.Second)

    // Cleanup.
    page.Eval(`() => {
        const el = document.querySelector('[data-monoes-reaction-btn]');
        if (el) el.removeAttribute('data-monoes-reaction-btn');
    }`)

    return nil
}
```

**Note:** `strings.Title` is deprecated in newer Go — use `strings.ToUpper(reaction[:1]) + reaction[1:]` instead.

- [ ] **Step 2.2: Add `CommentOnPost` method** (append to `actions.go`)

```go
// CommentOnPost posts a comment on a LinkedIn post, or replies to a specific comment.
// parentCommentID: empty for top-level comment; URN string to reply to that comment.
func (b *LinkedInBot) CommentOnPost(ctx context.Context, page *rod.Page, postURL, commentText, parentCommentID string) error {
    if postURL == "" {
        return fmt.Errorf("linkedin: postURL is required")
    }
    if commentText == "" {
        return fmt.Errorf("linkedin: commentText is required")
    }

    if err := page.Navigate(postURL); err != nil {
        return fmt.Errorf("linkedin: failed to navigate to post: %w", err)
    }
    if err := page.WaitLoad(); err != nil {
        return fmt.Errorf("linkedin: post page did not load: %w", err)
    }
    time.Sleep(3 * time.Second)

    if parentCommentID != "" {
        // Find the parent comment's Reply button and click it first.
        escapedID := strings.ReplaceAll(parentCommentID, `"`, `\"`)
        replyRes, err := page.Eval(fmt.Sprintf(`() => {
            const prev = document.querySelector('[data-monoes-reply-btn]');
            if (prev) prev.removeAttribute('data-monoes-reply-btn');
            const commentEl = document.querySelector('[data-id="%s"]');
            if (!commentEl) return 'not_found';
            const replyBtn = commentEl.querySelector('button[aria-label*="Reply"], button:has-text("Reply")');
            if (!replyBtn) return 'no_reply_btn';
            replyBtn.setAttribute('data-monoes-reply-btn', 'true');
            return 'marked';
        }`, escapedID))
        if err != nil || replyRes.Value.Str() != "marked" {
            return fmt.Errorf("linkedin: could not find Reply button for comment %s", parentCommentID)
        }
        replyBtn, err := page.Timeout(5 * time.Second).Element("[data-monoes-reply-btn='true']")
        if err != nil {
            return fmt.Errorf("linkedin: marked Reply button not found: %w", err)
        }
        replyBtn.MustScrollIntoView()
        time.Sleep(300 * time.Millisecond)
        if err := replyBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
            return fmt.Errorf("linkedin: failed to click Reply: %w", err)
        }
        time.Sleep(1 * time.Second)
        page.Eval(`() => {
            const el = document.querySelector('[data-monoes-reply-btn]');
            if (el) el.removeAttribute('data-monoes-reply-btn');
        }`)
    }

    // Find and mark the comment/reply input.
    inputRes, err := page.Eval(`() => {
        const prev = document.querySelector('[data-monoes-comment-input]');
        if (prev) prev.removeAttribute('data-monoes-comment-input');

        // Primary: contenteditable comment box (LinkedIn uses Quill editor).
        const editors = document.querySelectorAll('div.ql-editor[contenteditable="true"], div[role="textbox"][contenteditable="true"]');
        if (editors.length > 0) {
            editors[editors.length - 1].setAttribute('data-monoes-comment-input', 'true');
            return 'marked';
        }
        // Fallback: textarea with comment in label.
        const textareas = document.querySelectorAll('textarea[aria-label*="comment" i]');
        if (textareas.length > 0) {
            textareas[textareas.length - 1].setAttribute('data-monoes-comment-input', 'true');
            return 'marked';
        }
        return 'not_found';
    }`)
    if err != nil || inputRes.Value.Str() != "marked" {
        return fmt.Errorf("linkedin: could not find comment input")
    }

    commentInput, err := page.Timeout(5 * time.Second).Element("[data-monoes-comment-input='true']")
    if err != nil {
        return fmt.Errorf("linkedin: marked comment input not found: %w", err)
    }
    commentInput.MustScrollIntoView()
    time.Sleep(300 * time.Millisecond)
    if err := commentInput.Click(proto.InputMouseButtonLeft, 1); err != nil {
        return fmt.Errorf("linkedin: failed to click comment input: %w", err)
    }
    time.Sleep(500 * time.Millisecond)

    // Type comment text rune-by-rune for React/Quill compatibility.
    for _, ch := range commentText {
        if err := page.Keyboard.Type(input.Key(ch)); err != nil {
            return fmt.Errorf("linkedin: failed to type character: %w", err)
        }
        time.Sleep(40 * time.Millisecond)
    }
    time.Sleep(800 * time.Millisecond)

    // Find and click the Post/Submit button.
    submitRes, err := page.Eval(`() => {
        const prev = document.querySelector('[data-monoes-submit-btn]');
        if (prev) prev.removeAttribute('data-monoes-submit-btn');
        const allBtns = document.querySelectorAll('button');
        for (const btn of allBtns) {
            const text = btn.innerText.trim();
            if (text === 'Post' || text === 'Done' || text === 'Reply') {
                btn.setAttribute('data-monoes-submit-btn', 'true');
                return 'marked';
            }
        }
        return 'not_found';
    }`)
    if err != nil || submitRes.Value.Str() != "marked" {
        // Fallback: press Enter.
        page.Keyboard.Press(input.Enter)
    } else {
        submitBtn, err := page.Timeout(5 * time.Second).Element("[data-monoes-submit-btn='true']")
        if err != nil {
            page.Keyboard.Press(input.Enter)
        } else {
            submitBtn.MustScrollIntoView()
            time.Sleep(200 * time.Millisecond)
            if err := submitBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
                return fmt.Errorf("linkedin: failed to click submit: %w", err)
            }
        }
    }

    time.Sleep(3 * time.Second)

    page.Eval(`() => {
        ['[data-monoes-comment-input]','[data-monoes-submit-btn]','[data-monoes-reply-btn]'].forEach(sel => {
            const el = document.querySelector(sel);
            if (el) el.removeAttribute(sel.slice(1, -2).replace('data-monoes-', 'data-monoes-'));
        });
    }`)

    return nil
}
```

**Important import:** `"github.com/go-rod/rod/lib/input"` must be added to the import block in `actions.go` (for `input.Key(ch)` and `input.Enter`).

- [ ] **Step 2.3: Add `LikeComment` method** (append to `actions.go`)

```go
// LikeComment likes a specific comment on a LinkedIn post.
// commentID: the URN string from list_post_comments output (e.g. "urn:li:comment:(urn:li:activity:123,456)")
func (b *LinkedInBot) LikeComment(ctx context.Context, page *rod.Page, postURL, commentID string) error {
    if postURL == "" {
        return fmt.Errorf("linkedin: postURL is required")
    }
    if commentID == "" {
        return fmt.Errorf("linkedin: commentID is required")
    }

    if err := page.Navigate(postURL); err != nil {
        return fmt.Errorf("linkedin: failed to navigate to post: %w", err)
    }
    if err := page.WaitLoad(); err != nil {
        return fmt.Errorf("linkedin: post page did not load: %w", err)
    }
    time.Sleep(3 * time.Second)

    escapedID := strings.ReplaceAll(commentID, `"`, `\"`)
    res, err := page.Eval(fmt.Sprintf(`() => {
        const prev = document.querySelector('[data-monoes-comment-like]');
        if (prev) prev.removeAttribute('data-monoes-comment-like');

        const commentEl = document.querySelector('[data-id="%s"]');
        if (!commentEl) return 'not_found';
        commentEl.scrollIntoView({ behavior: 'smooth', block: 'center' });

        // Find the Like button within this comment (not a nested reply's like button).
        const likeBtn = commentEl.querySelector('button[aria-label*="Like"], button[aria-label*="React"]');
        if (!likeBtn) return 'no_like_btn';

        const label = (likeBtn.getAttribute('aria-label') || '').toLowerCase();
        if (label.includes('remove') || label.includes('unlike')) return 'already_liked';

        likeBtn.setAttribute('data-monoes-comment-like', 'true');
        return 'marked';
    }`, escapedID))
    if err != nil {
        return fmt.Errorf("linkedin: failed to evaluate like comment script: %w", err)
    }

    state := res.Value.Str()
    if state == "already_liked" {
        return nil
    }
    if state != "marked" {
        return fmt.Errorf("linkedin: could not find Like button for comment %s (%s)", commentID, state)
    }

    likeBtn, err := page.Timeout(5 * time.Second).Element("[data-monoes-comment-like='true']")
    if err != nil {
        return fmt.Errorf("linkedin: marked comment like button not found: %w", err)
    }
    likeBtn.MustScrollIntoView()
    time.Sleep(300 * time.Millisecond)

    if err := likeBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
        return fmt.Errorf("linkedin: failed to click comment Like: %w", err)
    }

    time.Sleep(1 * time.Second)
    page.Eval(`() => {
        const el = document.querySelector('[data-monoes-comment-like]');
        if (el) el.removeAttribute('data-monoes-comment-like');
    }`)

    return nil
}
```

- [ ] **Step 2.4: Fix import block in `actions.go`**

The `actions.go` file must import `"github.com/go-rod/rod/lib/input"` (used for `input.Key` and `input.Enter` in `CommentOnPost`). Check the final import block and add if missing:

```go
import (
    "context"
    "encoding/json"
    "fmt"
    "regexp"
    "strings"
    "time"

    "github.com/go-rod/rod"
    "github.com/go-rod/rod/lib/input"
    "github.com/go-rod/rod/lib/proto"
)
```

- [ ] **Step 2.5: Fix `strings.Title` deprecation**

In `LikePost`, replace `strings.Title(reaction)` with:
```go
reactionLabel := strings.ToUpper(reaction[:1]) + reaction[1:]
```

- [ ] **Step 2.6: Build to verify**

```bash
cd /Users/morteza/Desktop/monoes/monoes-agent/newmonoes
go build ./internal/bot/linkedin/... 2>&1
echo "EXIT:$?"
```

Expected: `EXIT:0`

- [ ] **Step 2.7: Commit**

```bash
git add internal/bot/linkedin/actions.go
git commit -m "feat: add LikePost, CommentOnPost, LikeComment to LinkedIn bot"
```

---

## Chunk 2: JSON Action Files

### Task 3: All 5 JSON action files

**Files to create:**
- `data/actions/linkedin/list_user_posts.json`
- `data/actions/linkedin/list_post_comments.json`
- `data/actions/linkedin/like_posts.json`
- `data/actions/linkedin/comment_on_posts.json`
- `data/actions/linkedin/like_comments.json`

> **Read first:** `data/actions/linkedin/find_by_keyword.json` for platform/structure reference.
> **Pattern reference:** `data/actions/instagram/list_user_posts.json`, `data/actions/instagram/like_posts.json`.

Use the exact JSON content from the spec (`docs/superpowers/specs/2026-03-18-linkedin-actions.md`, section "JSON Action Files").

- [ ] **Step 3.1: Create `data/actions/linkedin/list_user_posts.json`** — content from spec
- [ ] **Step 3.2: Create `data/actions/linkedin/list_post_comments.json`** — content from spec
- [ ] **Step 3.3: Create `data/actions/linkedin/like_posts.json`** — content from spec
- [ ] **Step 3.4: Create `data/actions/linkedin/comment_on_posts.json`** — content from spec
- [ ] **Step 3.5: Create `data/actions/linkedin/like_comments.json`** — content from spec

- [ ] **Step 3.6: Validate JSON syntax**

```bash
for f in list_user_posts list_post_comments like_posts comment_on_posts like_comments; do
  python3 -c "import json,sys; json.load(open('data/actions/linkedin/$f.json'))" && echo "$f: OK" || echo "$f: INVALID"
done
```

Expected: all 5 print `OK`.

- [ ] **Step 3.7: Verify CLI can load the action types**

```bash
cd /Users/morteza/Desktop/monoes/monoes-agent/newmonoes
go run ./cmd/monoes node types 2>&1 | grep linkedin
```

Expected: `linkedin.list_user_posts`, `linkedin.list_post_comments`, `linkedin.like_posts`, `linkedin.comment_on_posts`, `linkedin.like_comments` all appear.

- [ ] **Step 3.8: Commit**

```bash
git add data/actions/linkedin/list_user_posts.json \
        data/actions/linkedin/list_post_comments.json \
        data/actions/linkedin/like_posts.json \
        data/actions/linkedin/comment_on_posts.json \
        data/actions/linkedin/like_comments.json
git commit -m "feat: add LinkedIn action JSON definitions"
```

---

## Chunk 3: node.go + DB Integration

### Task 4: Extend extractPostShortcode for LinkedIn URLs

**Files:**
- Modify: `cmd/monoes/node.go`

- [ ] **Step 4.1: Check if `"regexp"` is already imported in `node.go`**

```bash
grep '"regexp"' /Users/morteza/Desktop/monoes/monoes-agent/newmonoes/cmd/monoes/node.go
```

If not present, add it to the import block.

- [ ] **Step 4.2: Update `extractPostShortcode`**

Find the function (currently around line 569). Replace the entire function body:

```go
// extractPostShortcode extracts the platform shortcode from a post URL.
// Instagram: https://www.instagram.com/p/CD61bhxKOQh/ → "CD61bhxKOQh"
// LinkedIn:  https://www.linkedin.com/posts/user-activity-7123456789/ → "7123456789"
func extractPostShortcode(postURL string) string {
    // Instagram: /p/{shortcode}/ or /reel/{shortcode}/
    parts := strings.Split(strings.Trim(postURL, "/"), "/")
    for i, p := range parts {
        if (p == "p" || p == "reel") && i+1 < len(parts) {
            return parts[i+1]
        }
    }

    // LinkedIn: activity-NNNNNNNN (posts URL) or activity:NNNNNNNN (feed/update URL)
    if strings.Contains(postURL, "linkedin.com") {
        re := regexp.MustCompile(`activity[-:](\d+)`)
        if m := re.FindStringSubmatch(postURL); len(m) > 1 {
            return m[1]
        }
    }

    return ""
}
```

- [ ] **Step 4.3: Build CLI**

```bash
cd /Users/morteza/Desktop/monoes/monoes-agent/newmonoes
go build ./cmd/monoes/... 2>&1
echo "EXIT:$?"
```

Expected: `EXIT:0`

- [ ] **Step 4.4: Commit**

```bash
git add cmd/monoes/node.go
git commit -m "feat: extend extractPostShortcode for LinkedIn activity URLs"
```

---

### Task 5: End-to-end CLI test

- [ ] **Step 5.1: Full build check**

```bash
cd /Users/morteza/Desktop/monoes/monoes-agent/newmonoes
go build ./... 2>&1 && echo "ALL OK"
```

Expected: `ALL OK`

- [ ] **Step 5.2: Verify action types are registered**

```bash
go run ./cmd/monoes node types 2>&1 | grep -E "linkedin\.(list_user_posts|list_post_comments|like_posts|comment_on_posts|like_comments)"
```

Expected: all 5 lines appear.

- [ ] **Step 5.3: Test list_user_posts (dry run — verify no panic)**

```bash
go run ./cmd/monoes node run linkedin.list_user_posts \
  --config '{"username":"onetap","targets":[{"url":"https://www.linkedin.com/in/mortezanoes/"}],"maxCount":5,"activityType":"all"}' \
  --dry-run 2>&1
```

If `--dry-run` is not supported, skip this step.

- [ ] **Step 5.4: Verify DB auto-save wiring for LinkedIn posts**

The `savePostsToDB` function in `node.go` already handles `platform = "LINKEDIN"` via:
- `strings.HasSuffix("linkedin.list_user_posts", "list_user_posts")` → `true`
- `strings.ToUpper(strings.SplitN("linkedin.list_user_posts", ".", 2)[0])` → `"LINKEDIN"`

Verify this by running a quick sanity check:

```bash
go run ./cmd/monoes -h 2>&1 | head -5
echo "CLI loads OK"
```

- [ ] **Step 5.5: Commit**

```bash
git add -A
git commit -m "feat: LinkedIn action nodes complete"
```
