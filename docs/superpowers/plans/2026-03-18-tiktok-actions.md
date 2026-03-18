# TikTok Action Suite Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add 9 TikTok bot actions (list_user_videos, like_video, comment_on_video, list_video_comments, like_comment, follow_user, stitch_video, duet_video, share_video) following the existing LinkedIn/Instagram three-layer pattern.

**Architecture:** `TikTokBot` in `internal/bot/tiktok/` already implements `bot.BotAdapter`. This plan adds `GetMethodByName` to `bot.go` (satisfying `action.BotAdapter`) and the 9 DOM automation methods to `actions.go`. Nine JSON action definition files go in `data/actions/tiktok/`.

**Tech Stack:** Go, go-rod browser automation, TikTok `data-e2e` DOM selectors, JSON action definitions.

---

## File Map

| File | Status | Purpose |
|---|---|---|
| `internal/bot/tiktok/bot.go` | **Modify** | Add `GetMethodByName` 9-case dispatch |
| `internal/bot/tiktok/actions.go` | **Create** | 9 DOM automation methods on `*TikTokBot` |
| `data/actions/tiktok/list_user_videos.json` | **Create** | List videos from profile |
| `data/actions/tiktok/like_video.json` | **Create** | Like a video |
| `data/actions/tiktok/comment_on_video.json` | **Create** | Comment on a video |
| `data/actions/tiktok/list_video_comments.json` | **Create** | List comments on a video |
| `data/actions/tiktok/like_comment.json` | **Create** | Like a comment |
| `data/actions/tiktok/follow_user.json` | **Create** | Follow a user |
| `data/actions/tiktok/stitch_video.json` | **Create** | Open stitch creator |
| `data/actions/tiktok/duet_video.json` | **Create** | Open duet creator |
| `data/actions/tiktok/share_video.json` | **Create** | Copy video share link |

**Reference files** (read before implementing):
- `internal/bot/linkedin/bot.go` — `GetMethodByName` pattern (lines 428–531)
- `internal/bot/linkedin/actions.go` — DOM automation method style
- `data/actions/linkedin/list_user_posts.json` — list action JSON pattern (uses `"name": "targets"`)
- `data/actions/linkedin/list_post_comments.json` — list action using `"name": "selectedListItems"`
- `data/actions/linkedin/like_posts.json` — interaction action JSON pattern
- `data/actions/linkedin/comment_on_posts.json` — comment action JSON pattern

**Method return type convention (authoritative):**
List methods (`ListUserVideos`, `ListVideoComments`) return `(interface{}, error)`.
Interaction methods (`LikeVideo`, `FollowUser`, `StitchVideo`, `DuetVideo`) return `error` — the `GetMethodByName` wrapper wraps them in a `map[string]interface{}{"success": true}` response.
`ShareVideo` returns `(interface{}, error)` because it returns data (the share URL).
This matches the LinkedIn pattern exactly: `ListUserPosts`/`ListPostComments` → `(interface{}, error)`; `LikePost`/`CommentOnPost`/`LikeComment` → `error` wrapped by closure.

**Registration note:** `cmd/monoes/node.go` already imports `_ ".../internal/bot/tiktok"` and handles `tiktok.` prefixes — no registration changes needed.

**Compile note:** Tasks 1–4 will produce "undefined method" compile errors until Task 5 is complete. Each task adds stub methods to keep the build green. Do NOT commit broken code. See Task 1 Step 2 for stubs.

**`tiktok.go` note:** `internal/bot/tiktok/tiktok.go` is a stub (package declaration only). Do not add any functions to it — put everything in `bot.go` or `actions.go`.

---

## Task 1: Add `GetMethodByName` to `bot.go`

**Files:**
- Modify: `internal/bot/tiktok/bot.go`

Add the `GetMethodByName` method after the existing `GetProfileData` function. Follow the LinkedIn pattern exactly: each `case` checks arg count, asserts `args[0].(*rod.Page)`, converts `float64` to `int` for counts, then calls the corresponding method on `b`.

- [ ] **Step 1: Add the import for `action` package at top of `bot.go`**

The file currently imports `github.com/go-rod/rod`. Add `"context"` if not already present (it is — check imports). No new imports needed since `GetMethodByName` only uses `fmt`, `context`, and `*rod.Page` which are already imported.

- [ ] **Step 2: Add `GetMethodByName` at the bottom of `bot.go`**

```go
// GetMethodByName returns a dispatchable wrapper for the named TikTok action method.
// This satisfies the action.BotAdapter interface so call_bot_method steps can resolve
// TikTok methods at runtime.
func (b *TikTokBot) GetMethodByName(name string) (func(ctx context.Context, args ...interface{}) (interface{}, error), bool) {
	switch name {
	case "list_user_videos":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("list_user_videos requires (page, profileURL)")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("list_user_videos: first arg must be *rod.Page")
			}
			profileURL, _ := args[1].(string)
			maxCount := 20
			if len(args) >= 3 {
				if v, ok := args[2].(float64); ok {
					maxCount = int(v)
				}
			}
			return b.ListUserVideos(ctx, page, profileURL, maxCount)
		}, true

	case "like_video":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("like_video requires (page, videoURL)")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("like_video: first arg must be *rod.Page")
			}
			videoURL, _ := args[1].(string)
			if err := b.LikeVideo(ctx, page, videoURL); err != nil {
				return nil, err
			}
			return map[string]interface{}{"success": true, "videoURL": videoURL}, nil
		}, true

	case "comment_on_video":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 3 {
				return nil, fmt.Errorf("comment_on_video requires (page, videoURL, commentText)")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("comment_on_video: first arg must be *rod.Page")
			}
			videoURL, _ := args[1].(string)
			commentText, _ := args[2].(string)
			if err := b.CommentOnVideo(ctx, page, videoURL, commentText); err != nil {
				return nil, err
			}
			return map[string]interface{}{"success": true, "videoURL": videoURL}, nil
		}, true

	case "list_video_comments":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("list_video_comments requires (page, videoURL)")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("list_video_comments: first arg must be *rod.Page")
			}
			videoURL, _ := args[1].(string)
			maxCount := 50
			if len(args) >= 3 {
				if v, ok := args[2].(float64); ok {
					maxCount = int(v)
				}
			}
			return b.ListVideoComments(ctx, page, videoURL, maxCount)
		}, true

	case "like_comment":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 3 {
				return nil, fmt.Errorf("like_comment requires (page, videoURL, commentID)")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("like_comment: first arg must be *rod.Page")
			}
			videoURL, _ := args[1].(string)
			commentID, _ := args[2].(string)
			if err := b.LikeComment(ctx, page, videoURL, commentID); err != nil {
				return nil, err
			}
			return map[string]interface{}{"success": true, "videoURL": videoURL, "commentID": commentID}, nil
		}, true

	case "follow_user":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("follow_user requires (page, profileURL)")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("follow_user: first arg must be *rod.Page")
			}
			profileURL, _ := args[1].(string)
			if err := b.FollowUser(ctx, page, profileURL); err != nil {
				return nil, err
			}
			return map[string]interface{}{"success": true, "profileURL": profileURL}, nil
		}, true

	case "stitch_video":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("stitch_video requires (page, videoURL)")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("stitch_video: first arg must be *rod.Page")
			}
			videoURL, _ := args[1].(string)
			if err := b.StitchVideo(ctx, page, videoURL); err != nil {
				return nil, err
			}
			return map[string]interface{}{"success": true, "videoURL": videoURL}, nil
		}, true

	case "duet_video":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("duet_video requires (page, videoURL)")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("duet_video: first arg must be *rod.Page")
			}
			videoURL, _ := args[1].(string)
			if err := b.DuetVideo(ctx, page, videoURL); err != nil {
				return nil, err
			}
			return map[string]interface{}{"success": true, "videoURL": videoURL}, nil
		}, true

	case "share_video":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("share_video requires (page, videoURL)")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("share_video: first arg must be *rod.Page")
			}
			videoURL, _ := args[1].(string)
			return b.ShareVideo(ctx, page, videoURL)
		}, true
	}
	return nil, false
}
```

- [ ] **Step 3: Create `actions.go` with method stubs so the build is green**

Create `internal/bot/tiktok/actions.go` with stubs for all 9 methods. This prevents compile errors. Tasks 2–5 will replace each stub with the real implementation.

```go
package tiktok

import (
	"context"
	"fmt"

	"github.com/go-rod/rod"
)

func (b *TikTokBot) ListUserVideos(ctx context.Context, page *rod.Page, profileURL string, maxCount int) (interface{}, error) {
	return nil, fmt.Errorf("tiktok: ListUserVideos not yet implemented")
}
func (b *TikTokBot) FollowUser(ctx context.Context, page *rod.Page, profileURL string) error {
	return fmt.Errorf("tiktok: FollowUser not yet implemented")
}
func (b *TikTokBot) LikeVideo(ctx context.Context, page *rod.Page, videoURL string) error {
	return fmt.Errorf("tiktok: LikeVideo not yet implemented")
}
func (b *TikTokBot) CommentOnVideo(ctx context.Context, page *rod.Page, videoURL string, commentText string) error {
	return fmt.Errorf("tiktok: CommentOnVideo not yet implemented")
}
func (b *TikTokBot) ListVideoComments(ctx context.Context, page *rod.Page, videoURL string, maxCount int) (interface{}, error) {
	return nil, fmt.Errorf("tiktok: ListVideoComments not yet implemented")
}
func (b *TikTokBot) LikeComment(ctx context.Context, page *rod.Page, videoURL string, commentID string) error {
	return fmt.Errorf("tiktok: LikeComment not yet implemented")
}
func (b *TikTokBot) StitchVideo(ctx context.Context, page *rod.Page, videoURL string) error {
	return fmt.Errorf("tiktok: StitchVideo not yet implemented")
}
func (b *TikTokBot) DuetVideo(ctx context.Context, page *rod.Page, videoURL string) error {
	return fmt.Errorf("tiktok: DuetVideo not yet implemented")
}
func (b *TikTokBot) ShareVideo(ctx context.Context, page *rod.Page, videoURL string) (interface{}, error) {
	return nil, fmt.Errorf("tiktok: ShareVideo not yet implemented")
}
```

- [ ] **Step 4: Verify it compiles cleanly**

```bash
cd /path/to/newmonoes && go build ./internal/bot/tiktok/...
```
Expected: PASS — no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/bot/tiktok/bot.go internal/bot/tiktok/actions.go
git commit -m "feat(tiktok): add GetMethodByName dispatch + method stubs"
```

---

## Task 2: Implement list and follow actions in `actions.go`

**Files:**
- Create: `internal/bot/tiktok/actions.go`

This task implements: `ListUserVideos`, `FollowUser`. These are the two "navigate to profile" methods.

Reference: `internal/bot/linkedin/actions.go` for code style and go-rod patterns.

Key patterns to follow:
- `page.Navigate(url)` + `page.WaitLoad()` + `time.Sleep(3 * time.Second)` for navigation
- `page.Timeout(N * time.Second).Element(selector)` for element lookup with timeout
- `page.Eval(script, args...)` for complex JS (returns `*proto.RuntimeRemoteObject`)
- Scroll with `page.Eval("() => window.scrollBy(0, 500)")` repeated in a loop
- Return `[]map[string]interface{}` for list methods

- [ ] **Step 1: Create `actions.go` with `ListUserVideos`**

```go
package tiktok

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// ListUserVideos navigates to a TikTok profile page, scrolls to load the video
// grid, and returns up to maxCount video entries.
func (b *TikTokBot) ListUserVideos(ctx context.Context, page *rod.Page, profileURL string, maxCount int) (interface{}, error) {
	if profileURL == "" {
		return nil, fmt.Errorf("tiktok: profileURL is required")
	}
	if maxCount <= 0 {
		maxCount = 20
	}

	if err := page.Navigate(profileURL); err != nil {
		return nil, fmt.Errorf("tiktok: navigate to %s: %w", profileURL, err)
	}
	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("tiktok: page load failed: %w", err)
	}
	time.Sleep(3 * time.Second)

	// Click videos tab to ensure the grid is active.
	if tab, err := page.Timeout(5 * time.Second).Element("[data-e2e='videos-tab']"); err == nil && tab != nil {
		_ = tab.Click(proto.InputMouseButtonLeft, 1)
		time.Sleep(2 * time.Second)
	}

	// Scroll to load video grid items.
	var videos []map[string]interface{}
	prevCount := 0
	noChangeRounds := 0

	for len(videos) < maxCount && noChangeRounds < 3 {
		result, err := page.Eval(`() => {
			const items = document.querySelectorAll('[data-e2e="user-post-item"]');
			return Array.from(items).map(el => {
				const a = el.querySelector('a');
				const img = el.querySelector('img');
				return {
					url: a ? a.href : '',
					thumbnail: img ? img.src : '',
				};
			}).filter(v => v.url !== '');
		}`)
		if err == nil && result != nil {
			if arr, ok := result.Value.Value().([]interface{}); ok {
				videos = videos[:0]
				for _, item := range arr {
					if m, ok := item.(map[string]interface{}); ok {
						videos = append(videos, m)
					}
				}
			}
		}

		if len(videos) == prevCount {
			noChangeRounds++
		} else {
			noChangeRounds = 0
			prevCount = len(videos)
		}

		if len(videos) < maxCount {
			_, _ = page.Eval("() => window.scrollBy(0, 800)")
			time.Sleep(1500 * time.Millisecond)
		}
	}

	if len(videos) > maxCount {
		videos = videos[:maxCount]
	}

	result := make([]interface{}, len(videos))
	for i, v := range videos {
		result[i] = v
	}
	return result, nil
}

// FollowUser navigates to a TikTok profile page and clicks the Follow button.
// Returns nil if already following (idempotent).
func (b *TikTokBot) FollowUser(ctx context.Context, page *rod.Page, profileURL string) error {
	if profileURL == "" {
		return fmt.Errorf("tiktok: profileURL is required")
	}

	if err := page.Navigate(profileURL); err != nil {
		return fmt.Errorf("tiktok: navigate to %s: %w", profileURL, err)
	}
	if err := page.WaitLoad(); err != nil {
		return fmt.Errorf("tiktok: page load failed: %w", err)
	}
	time.Sleep(3 * time.Second)

	btn, err := page.Timeout(5 * time.Second).Element("[data-e2e='follow-button']")
	if err != nil {
		return fmt.Errorf("tiktok: follow button not found on %s", profileURL)
	}

	text, _ := btn.Text()
	text = strings.TrimSpace(strings.ToLower(text))
	if text == "following" || text == "friends" {
		// Already following — idempotent, not an error.
		return nil
	}

	if err := btn.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("tiktok: failed to click follow button: %w", err)
	}
	time.Sleep(1 * time.Second)
	return nil
}
```

- [ ] **Step 2: Build to confirm no compile errors**

```bash
go build ./internal/bot/tiktok/...
```
Expected: PASS — the stubs from Task 1 are being replaced one by one. This replaces `ListUserVideos` and `FollowUser` stubs. If you get import errors, check that `proto` import is `github.com/go-rod/rod/lib/proto`.

- [ ] **Step 3: Commit**

```bash
git add internal/bot/tiktok/actions.go
git commit -m "feat(tiktok): add ListUserVideos and FollowUser"
```

---

## Task 3: Implement video interaction actions in `actions.go`

**Files:**
- Modify: `internal/bot/tiktok/actions.go`

Add: `LikeVideo`, `CommentOnVideo`.

- [ ] **Step 1: Add `LikeVideo`**

Append to `actions.go`:

```go
// LikeVideo navigates to a TikTok video page and clicks the like button.
// Returns nil if already liked (idempotent).
func (b *TikTokBot) LikeVideo(ctx context.Context, page *rod.Page, videoURL string) error {
	if videoURL == "" {
		return fmt.Errorf("tiktok: videoURL is required")
	}

	if err := page.Navigate(videoURL); err != nil {
		return fmt.Errorf("tiktok: navigate to %s: %w", videoURL, err)
	}
	if err := page.WaitLoad(); err != nil {
		return fmt.Errorf("tiktok: page load failed: %w", err)
	}
	time.Sleep(3 * time.Second)

	likeSelectors := []string{
		"[data-e2e='like-icon']",
		"[data-e2e='browse-like-button']",
	}

	var likeBtn *rod.Element
	for _, sel := range likeSelectors {
		el, err := page.Timeout(5 * time.Second).Element(sel)
		if err == nil && el != nil {
			likeBtn = el
			break
		}
	}
	if likeBtn == nil {
		return fmt.Errorf("tiktok: like button not found on %s", videoURL)
	}

	// Check if already liked (aria-pressed="true" or similar).
	pressed, _ := likeBtn.Attribute("aria-pressed")
	if pressed != nil && *pressed == "true" {
		return nil // already liked
	}

	if err := likeBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("tiktok: failed to click like button: %w", err)
	}
	time.Sleep(1 * time.Second)
	return nil
}

// CommentOnVideo navigates to a TikTok video page, opens the comment panel,
// types the comment, and submits it.
func (b *TikTokBot) CommentOnVideo(ctx context.Context, page *rod.Page, videoURL string, commentText string) error {
	if videoURL == "" {
		return fmt.Errorf("tiktok: videoURL is required")
	}
	if commentText == "" {
		return fmt.Errorf("tiktok: commentText is required")
	}

	if err := page.Navigate(videoURL); err != nil {
		return fmt.Errorf("tiktok: navigate to %s: %w", videoURL, err)
	}
	if err := page.WaitLoad(); err != nil {
		return fmt.Errorf("tiktok: page load failed: %w", err)
	}
	time.Sleep(3 * time.Second)

	// Open comment panel.
	commentIconSelectors := []string{
		"[data-e2e='comment-icon']",
		"[data-e2e='browse-comment-button']",
	}
	for _, sel := range commentIconSelectors {
		el, err := page.Timeout(5 * time.Second).Element(sel)
		if err == nil && el != nil {
			_ = el.Click(proto.InputMouseButtonLeft, 1)
			time.Sleep(2 * time.Second)
			break
		}
	}

	// Find comment input.
	inputSelectors := []string{
		"[data-e2e='comment-input']",
		"div[contenteditable='true'][class*='comment']",
		"div[contenteditable='true']",
	}
	var commentInput *rod.Element
	for _, sel := range inputSelectors {
		el, err := page.Timeout(5 * time.Second).Element(sel)
		if err == nil && el != nil {
			commentInput = el
			break
		}
	}
	if commentInput == nil {
		return fmt.Errorf("tiktok: comment input not found on %s", videoURL)
	}

	if err := commentInput.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("tiktok: failed to focus comment input: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	if err := commentInput.Input(commentText); err != nil {
		return fmt.Errorf("tiktok: failed to type comment: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Submit comment.
	submitSelectors := []string{
		"[data-e2e='comment-send-btn']",
		"[data-e2e='comment-post-btn']",
		"button[type='submit']",
	}
	for _, sel := range submitSelectors {
		el, err := page.Timeout(5 * time.Second).Element(sel)
		if err == nil && el != nil {
			if clickErr := el.Click(proto.InputMouseButtonLeft, 1); clickErr == nil {
				time.Sleep(1 * time.Second)
				return nil
			}
		}
	}
	return fmt.Errorf("tiktok: could not find comment submit button on %s", videoURL)
}
```

- [ ] **Step 2: Build**

```bash
go build ./internal/bot/tiktok/...
```
Expected: PASS — `LikeVideo` and `CommentOnVideo` stubs are now replaced.

- [ ] **Step 3: Commit**

```bash
git add internal/bot/tiktok/actions.go
git commit -m "feat(tiktok): add LikeVideo and CommentOnVideo"
```

---

## Task 4: Implement comment listing and liking in `actions.go`

**Files:**
- Modify: `internal/bot/tiktok/actions.go`

Add: `ListVideoComments`, `LikeComment`.

- [ ] **Step 1: Add `ListVideoComments`**

Append to `actions.go`:

```go
// ListVideoComments navigates to a TikTok video page, opens the comment panel,
// and returns up to maxCount comment entries.
func (b *TikTokBot) ListVideoComments(ctx context.Context, page *rod.Page, videoURL string, maxCount int) (interface{}, error) {
	if videoURL == "" {
		return nil, fmt.Errorf("tiktok: videoURL is required")
	}
	if maxCount <= 0 {
		maxCount = 50
	}

	if err := page.Navigate(videoURL); err != nil {
		return nil, fmt.Errorf("tiktok: navigate to %s: %w", videoURL, err)
	}
	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("tiktok: page load failed: %w", err)
	}
	time.Sleep(3 * time.Second)

	// Open comment panel.
	commentIconSelectors := []string{
		"[data-e2e='comment-icon']",
		"[data-e2e='browse-comment-button']",
	}
	for _, sel := range commentIconSelectors {
		el, err := page.Timeout(5 * time.Second).Element(sel)
		if err == nil && el != nil {
			_ = el.Click(proto.InputMouseButtonLeft, 1)
			time.Sleep(2 * time.Second)
			break
		}
	}

	var comments []map[string]interface{}
	prevCount := 0
	noChangeRounds := 0

	for len(comments) < maxCount && noChangeRounds < 3 {
		result, err := page.Eval(`() => {
			const items = document.querySelectorAll('[data-e2e="comment-item"]');
			return Array.from(items).map(el => {
				const username = el.querySelector('[data-e2e="comment-username"]');
				const content = el.querySelector('[data-e2e="comment-content"]');
				const likeEl = el.querySelector('[data-e2e="comment-like-btn"]');
				return {
					id: el.getAttribute('data-comment-id') || el.id || '',
					username: username ? username.innerText.trim() : '',
					text: content ? content.innerText.trim() : '',
					likes: likeEl ? (likeEl.getAttribute('aria-label') || '') : '',
				};
			}).filter(c => c.text !== '');
		}`)
		if err == nil && result != nil {
			if arr, ok := result.Value.Value().([]interface{}); ok {
				comments = comments[:0]
				for _, item := range arr {
					if m, ok := item.(map[string]interface{}); ok {
						comments = append(comments, m)
					}
				}
			}
		}

		if len(comments) == prevCount {
			noChangeRounds++
		} else {
			noChangeRounds = 0
			prevCount = len(comments)
		}

		if len(comments) < maxCount {
			// Scroll comment panel.
			_, _ = page.Eval(`() => {
				const panel = document.querySelector('[data-e2e="comment-list"]') ||
				              document.querySelector('[class*="CommentList"]');
				if (panel) panel.scrollBy(0, 500);
				else window.scrollBy(0, 500);
			}`)
			time.Sleep(1500 * time.Millisecond)
		}
	}

	if len(comments) > maxCount {
		comments = comments[:maxCount]
	}

	result := make([]interface{}, len(comments))
	for i, c := range comments {
		result[i] = c
	}
	return result, nil
}

// LikeComment navigates to a TikTok video page, finds the comment by ID, and
// clicks its like button.
func (b *TikTokBot) LikeComment(ctx context.Context, page *rod.Page, videoURL string, commentID string) error {
	if videoURL == "" {
		return fmt.Errorf("tiktok: videoURL is required")
	}
	if commentID == "" {
		return fmt.Errorf("tiktok: commentID is required")
	}

	if err := page.Navigate(videoURL); err != nil {
		return fmt.Errorf("tiktok: navigate to %s: %w", videoURL, err)
	}
	if err := page.WaitLoad(); err != nil {
		return fmt.Errorf("tiktok: page load failed: %w", err)
	}
	time.Sleep(3 * time.Second)

	// Open comment panel.
	for _, sel := range []string{"[data-e2e='comment-icon']", "[data-e2e='browse-comment-button']"} {
		el, err := page.Timeout(5 * time.Second).Element(sel)
		if err == nil && el != nil {
			_ = el.Click(proto.InputMouseButtonLeft, 1)
			time.Sleep(2 * time.Second)
			break
		}
	}

	// Find comment by ID and click its like button using JS.
	result, err := page.Eval(fmt.Sprintf(`() => {
		const id = %q;
		const items = document.querySelectorAll('[data-e2e="comment-item"]');
		for (const el of items) {
			if (el.getAttribute('data-comment-id') === id || el.id === id) {
				const likeBtn = el.querySelector('[data-e2e="comment-like-btn"]');
				if (likeBtn) { likeBtn.click(); return true; }
			}
		}
		return false;
	}`, commentID))
	if err != nil {
		return fmt.Errorf("tiktok: failed to like comment %s: %w", commentID, err)
	}
	if result != nil {
		if ok, _ := result.Value.Value().(bool); !ok {
			return fmt.Errorf("tiktok: comment %s not found on page %s", commentID, videoURL)
		}
	}
	time.Sleep(1 * time.Second)
	return nil
}
```

- [ ] **Step 2: Build**

```bash
go build ./internal/bot/tiktok/...
```
Expected: PASS — `ListVideoComments` and `LikeComment` stubs are now replaced.

- [ ] **Step 3: Commit**

```bash
git add internal/bot/tiktok/actions.go
git commit -m "feat(tiktok): add ListVideoComments and LikeComment"
```

---

## Task 5: Implement TikTok-exclusive actions in `actions.go`

**Files:**
- Modify: `internal/bot/tiktok/actions.go`

Add: `StitchVideo`, `DuetVideo`, `ShareVideo`.

- [ ] **Step 1: Add the three methods**

Append to `actions.go`:

```go
// StitchVideo navigates to a TikTok video page, opens the share modal, and
// clicks the Stitch option to open the stitch creator.
func (b *TikTokBot) StitchVideo(ctx context.Context, page *rod.Page, videoURL string) error {
	if videoURL == "" {
		return fmt.Errorf("tiktok: videoURL is required")
	}
	if err := page.Navigate(videoURL); err != nil {
		return fmt.Errorf("tiktok: navigate to %s: %w", videoURL, err)
	}
	if err := page.WaitLoad(); err != nil {
		return fmt.Errorf("tiktok: page load failed: %w", err)
	}
	time.Sleep(3 * time.Second)

	if err := openShareModal(page, videoURL); err != nil {
		return err
	}

	stitchSelectors := []string{
		"[data-e2e='share-stitch']",
		"button[class*='stitch']",
		"div[class*='Stitch']",
	}
	for _, sel := range stitchSelectors {
		el, err := page.Timeout(5 * time.Second).Element(sel)
		if err == nil && el != nil {
			if clickErr := el.Click(proto.InputMouseButtonLeft, 1); clickErr == nil {
				time.Sleep(2 * time.Second)
				return nil
			}
		}
	}
	return fmt.Errorf("tiktok: stitch option not found in share modal for %s", videoURL)
}

// DuetVideo navigates to a TikTok video page, opens the share modal, and
// clicks the Duet option to open the duet creator.
func (b *TikTokBot) DuetVideo(ctx context.Context, page *rod.Page, videoURL string) error {
	if videoURL == "" {
		return fmt.Errorf("tiktok: videoURL is required")
	}
	if err := page.Navigate(videoURL); err != nil {
		return fmt.Errorf("tiktok: navigate to %s: %w", videoURL, err)
	}
	if err := page.WaitLoad(); err != nil {
		return fmt.Errorf("tiktok: page load failed: %w", err)
	}
	time.Sleep(3 * time.Second)

	if err := openShareModal(page, videoURL); err != nil {
		return err
	}

	duetSelectors := []string{
		"[data-e2e='share-duet']",
		"button[class*='duet']",
		"div[class*='Duet']",
	}
	for _, sel := range duetSelectors {
		el, err := page.Timeout(5 * time.Second).Element(sel)
		if err == nil && el != nil {
			if clickErr := el.Click(proto.InputMouseButtonLeft, 1); clickErr == nil {
				time.Sleep(2 * time.Second)
				return nil
			}
		}
	}
	return fmt.Errorf("tiktok: duet option not found in share modal for %s", videoURL)
}

// ShareVideo navigates to a TikTok video page, opens the share modal, and
// clicks "Copy link". Returns the video URL (TikTok copies the URL to clipboard).
func (b *TikTokBot) ShareVideo(ctx context.Context, page *rod.Page, videoURL string) (interface{}, error) {
	if videoURL == "" {
		return nil, fmt.Errorf("tiktok: videoURL is required")
	}
	if err := page.Navigate(videoURL); err != nil {
		return nil, fmt.Errorf("tiktok: navigate to %s: %w", videoURL, err)
	}
	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("tiktok: page load failed: %w", err)
	}
	time.Sleep(3 * time.Second)

	if err := openShareModal(page, videoURL); err != nil {
		return nil, err
	}

	copyLinkSelectors := []string{
		"[data-e2e='copy-link-icon']",
		"button[class*='CopyLink']",
		"div[class*='copy-link']",
	}
	for _, sel := range copyLinkSelectors {
		el, err := page.Timeout(5 * time.Second).Element(sel)
		if err == nil && el != nil {
			if clickErr := el.Click(proto.InputMouseButtonLeft, 1); clickErr == nil {
				time.Sleep(1 * time.Second)
				return map[string]interface{}{"success": true, "url": videoURL}, nil
			}
		}
	}
	return nil, fmt.Errorf("tiktok: copy link button not found in share modal for %s", videoURL)
}

// openShareModal clicks the share icon on the current page to open the share modal.
func openShareModal(page *rod.Page, videoURL string) error {
	shareSelectors := []string{
		"[data-e2e='share-icon']",
		"[data-e2e='share-btn']",
		"button[class*='ShareButton']",
	}
	for _, sel := range shareSelectors {
		el, err := page.Timeout(5 * time.Second).Element(sel)
		if err == nil && el != nil {
			if clickErr := el.Click(proto.InputMouseButtonLeft, 1); clickErr == nil {
				time.Sleep(2 * time.Second)
				return nil
			}
		}
	}
	return fmt.Errorf("tiktok: share button not found on %s", videoURL)
}
```

- [ ] **Step 2: Build — should compile cleanly now**

```bash
go build ./internal/bot/tiktok/...
```
Expected: PASS with no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/bot/tiktok/actions.go
git commit -m "feat(tiktok): add StitchVideo, DuetVideo, ShareVideo"
```

---

## Task 6: JSON action files — list actions

**Files:**
- Create: `data/actions/tiktok/list_user_videos.json`
- Create: `data/actions/tiktok/list_video_comments.json`

Reference: `data/actions/linkedin/list_user_posts.json` and `data/actions/linkedin/list_post_comments.json`.

- [ ] **Step 1: Create `list_user_videos.json`**

```json
{
  "actionType": "list_user_videos",
  "platform": "TIKTOK",
  "version": "1.0.0",
  "description": "List videos from a TikTok profile",
  "metadata": {
    "requiresAuth": true,
    "supportsPagination": false,
    "supportsRetry": true
  },
  "inputs": {
    "required": [
      {
        "name": "targets",
        "type": "list",
        "description": "TikTok profile URLs to collect videos from"
      }
    ],
    "optional": [
      {
        "name": "maxCount",
        "type": "number",
        "default": 20,
        "min": 1,
        "max": 200
      }
    ]
  },
  "outputs": {
    "success": ["videos", "videosCount"],
    "failure": []
  },
  "steps": [
    {
      "id": "t1_list_videos",
      "type": "call_bot_method",
      "methodName": "list_user_videos",
      "args": ["{{item.url or item}}", "{{maxCount or 20}}"],
      "variable_name": "videos",
      "timeout": 120,
      "onError": {
        "action": "skip"
      }
    },
    {
      "id": "save_videos",
      "type": "save_data",
      "dataSource": "videos"
    },
    {
      "id": "store_count",
      "type": "update_progress",
      "set": {
        "videosCount": "{{videos.count}}"
      }
    },
    {
      "id": "log_success",
      "type": "log",
      "value": "Profile: {{item.url or item}}, Videos: {{videosCount}}"
    }
  ],
  "loops": [
    {
      "id": "process_profiles",
      "iterator": "selectedListItems",
      "indexVar": "loopIndex",
      "steps": ["t1_list_videos", "save_videos", "store_count", "log_success"],
      "onComplete": "update_action_state"
    }
  ],
  "errorHandling": {
    "globalRetries": 2,
    "retryDelay": 3000,
    "onFinalFailure": "log_and_continue"
  }
}
```

- [ ] **Step 2: Create `list_video_comments.json`**

```json
{
  "actionType": "list_video_comments",
  "platform": "TIKTOK",
  "version": "1.0.0",
  "description": "List comments on a TikTok video",
  "metadata": {
    "requiresAuth": true,
    "supportsPagination": false,
    "supportsRetry": true
  },
  "inputs": {
    "required": [
      {
        "name": "selectedListItems",
        "type": "list",
        "description": "TikTok video URLs to collect comments from"
      }
    ],
    "optional": [
      {
        "name": "maxCount",
        "type": "number",
        "default": 50,
        "min": 1,
        "max": 500
      }
    ]
  },
  "outputs": {
    "success": ["comments", "commentsCount"],
    "failure": []
  },
  "steps": [
    {
      "id": "t1_list_comments",
      "type": "call_bot_method",
      "methodName": "list_video_comments",
      "args": ["{{item.url or item}}", "{{maxCount or 50}}"],
      "variable_name": "comments",
      "timeout": 120,
      "onError": {
        "action": "skip"
      }
    },
    {
      "id": "save_comments",
      "type": "save_data",
      "dataSource": "comments"
    },
    {
      "id": "store_count",
      "type": "update_progress",
      "set": {
        "commentsCount": "{{comments.count}}"
      }
    },
    {
      "id": "log_success",
      "type": "log",
      "value": "Video: {{item.url or item}}, Comments: {{commentsCount}}"
    }
  ],
  "loops": [
    {
      "id": "process_videos",
      "iterator": "selectedListItems",
      "indexVar": "loopIndex",
      "steps": ["t1_list_comments", "save_comments", "store_count", "log_success"],
      "onComplete": "update_action_state"
    }
  ],
  "errorHandling": {
    "globalRetries": 2,
    "retryDelay": 3000,
    "onFinalFailure": "log_and_continue"
  }
}
```

- [ ] **Step 3: Commit**

```bash
git add data/actions/tiktok/list_user_videos.json data/actions/tiktok/list_video_comments.json
git commit -m "feat(tiktok): add list_user_videos and list_video_comments JSON actions"
```

---

## Task 7: JSON action files — interaction actions

**Files:**
- Create: `data/actions/tiktok/like_video.json`
- Create: `data/actions/tiktok/comment_on_video.json`
- Create: `data/actions/tiktok/like_comment.json`
- Create: `data/actions/tiktok/follow_user.json`

Reference: `data/actions/linkedin/like_posts.json` and `data/actions/linkedin/comment_on_posts.json`.

- [ ] **Step 1: Create `like_video.json`**

```json
{
  "actionType": "like_video",
  "platform": "TIKTOK",
  "version": "1.0.0",
  "description": "Like TikTok videos",
  "metadata": {
    "requiresAuth": true,
    "supportsPagination": false,
    "supportsRetry": true
  },
  "inputs": {
    "required": [
      {
        "name": "selectedListItems",
        "type": "list",
        "description": "TikTok video URLs to like"
      }
    ],
    "optional": [
      {
        "name": "delayBetweenLikes",
        "type": "number",
        "default": 3,
        "min": 1,
        "max": 60
      }
    ]
  },
  "outputs": {
    "success": ["likesCount", "reachedIndex"],
    "failure": ["failedItems"]
  },
  "steps": [
    {
      "id": "t1_like_video",
      "type": "call_bot_method",
      "methodName": "like_video",
      "args": ["{{item.url or item}}"],
      "variable_name": "likeResult",
      "timeout": 30,
      "onError": {
        "action": "skip"
      }
    },
    {
      "id": "wait",
      "type": "wait",
      "duration": "{{delayBetweenLikes or 3}}",
      "waitFor": "time"
    },
    {
      "id": "log",
      "type": "log",
      "value": "Liked: {{item.url or item}}"
    }
  ],
  "loops": [
    {
      "id": "process_videos",
      "iterator": "selectedListItems",
      "indexVar": "reachedIndex",
      "steps": ["t1_like_video", "wait", "log"],
      "onComplete": "update_action_state"
    }
  ],
  "errorHandling": {
    "globalRetries": 2,
    "retryDelay": 3000,
    "onFinalFailure": "log_and_continue"
  }
}
```

- [ ] **Step 2: Create `comment_on_video.json`**

```json
{
  "actionType": "comment_on_video",
  "platform": "TIKTOK",
  "version": "1.0.0",
  "description": "Comment on TikTok videos",
  "metadata": {
    "requiresAuth": true,
    "supportsPagination": false,
    "supportsRetry": true
  },
  "inputs": {
    "required": [
      {
        "name": "selectedListItems",
        "type": "list",
        "description": "TikTok video URLs to comment on"
      },
      {
        "name": "commentText",
        "type": "string",
        "description": "Comment text (supports {{templates}})"
      }
    ],
    "optional": [
      {
        "name": "delayBetweenComments",
        "type": "number",
        "default": 5,
        "min": 2,
        "max": 120
      }
    ]
  },
  "outputs": {
    "success": ["commentsCount", "reachedIndex"],
    "failure": ["failedItems"]
  },
  "steps": [
    {
      "id": "t1_comment",
      "type": "call_bot_method",
      "methodName": "comment_on_video",
      "args": ["{{item.url or item}}", "{{commentText}}"],
      "variable_name": "commentResult",
      "timeout": 45,
      "onError": {
        "action": "skip"
      }
    },
    {
      "id": "wait",
      "type": "wait",
      "duration": "{{delayBetweenComments or 5}}",
      "waitFor": "time"
    },
    {
      "id": "log",
      "type": "log",
      "value": "Commented on: {{item.url or item}}"
    }
  ],
  "loops": [
    {
      "id": "process_videos",
      "iterator": "selectedListItems",
      "indexVar": "reachedIndex",
      "steps": ["t1_comment", "wait", "log"],
      "onComplete": "update_action_state"
    }
  ],
  "errorHandling": {
    "globalRetries": 1,
    "retryDelay": 5000,
    "onFinalFailure": "log_and_continue"
  }
}
```

- [ ] **Step 3: Create `like_comment.json`**

```json
{
  "actionType": "like_comment",
  "platform": "TIKTOK",
  "version": "1.0.0",
  "description": "Like a specific comment on a TikTok video",
  "metadata": {
    "requiresAuth": true,
    "supportsPagination": false,
    "supportsRetry": true
  },
  "inputs": {
    "required": [
      {
        "name": "selectedListItems",
        "type": "list",
        "description": "Items with videoURL and commentID fields, or plain comment IDs"
      }
    ]
  },
  "outputs": {
    "success": ["reachedIndex"],
    "failure": ["failedItems"]
  },
  "steps": [
    {
      "id": "t1_like_comment",
      "type": "call_bot_method",
      "methodName": "like_comment",
      "args": ["{{item.videoURL or item.url or item}}", "{{item.id or item}}"],
      "variable_name": "likeResult",
      "timeout": 30,
      "onError": {
        "action": "skip"
      }
    },
    {
      "id": "log",
      "type": "log",
      "value": "Liked comment: {{item.id or item}}"
    }
  ],
  "loops": [
    {
      "id": "process_comments",
      "iterator": "selectedListItems",
      "indexVar": "reachedIndex",
      "steps": ["t1_like_comment", "log"],
      "onComplete": "update_action_state"
    }
  ],
  "errorHandling": {
    "globalRetries": 2,
    "retryDelay": 3000,
    "onFinalFailure": "log_and_continue"
  }
}
```

- [ ] **Step 4: Create `follow_user.json`**

```json
{
  "actionType": "follow_user",
  "platform": "TIKTOK",
  "version": "1.0.0",
  "description": "Follow TikTok users",
  "metadata": {
    "requiresAuth": true,
    "supportsPagination": false,
    "supportsRetry": true
  },
  "inputs": {
    "required": [
      {
        "name": "selectedListItems",
        "type": "list",
        "description": "TikTok profile URLs to follow"
      }
    ],
    "optional": [
      {
        "name": "delayBetweenFollows",
        "type": "number",
        "default": 5,
        "min": 2,
        "max": 120
      }
    ]
  },
  "outputs": {
    "success": ["followsCount", "reachedIndex"],
    "failure": ["failedItems"]
  },
  "steps": [
    {
      "id": "t1_follow",
      "type": "call_bot_method",
      "methodName": "follow_user",
      "args": ["{{item.url or item}}"],
      "variable_name": "followResult",
      "timeout": 30,
      "onError": {
        "action": "skip"
      }
    },
    {
      "id": "wait",
      "type": "wait",
      "duration": "{{delayBetweenFollows or 5}}",
      "waitFor": "time"
    },
    {
      "id": "log",
      "type": "log",
      "value": "Followed: {{item.url or item}}"
    }
  ],
  "loops": [
    {
      "id": "process_profiles",
      "iterator": "selectedListItems",
      "indexVar": "reachedIndex",
      "steps": ["t1_follow", "wait", "log"],
      "onComplete": "update_action_state"
    }
  ],
  "errorHandling": {
    "globalRetries": 2,
    "retryDelay": 3000,
    "onFinalFailure": "log_and_continue"
  }
}
```

- [ ] **Step 5: Commit**

```bash
git add data/actions/tiktok/like_video.json data/actions/tiktok/comment_on_video.json \
        data/actions/tiktok/like_comment.json data/actions/tiktok/follow_user.json
git commit -m "feat(tiktok): add like_video, comment_on_video, like_comment, follow_user JSON actions"
```

---

## Task 8: JSON action files — TikTok-exclusive actions

**Files:**
- Create: `data/actions/tiktok/stitch_video.json`
- Create: `data/actions/tiktok/duet_video.json`
- Create: `data/actions/tiktok/share_video.json`

- [ ] **Step 1: Create `stitch_video.json`**

```json
{
  "actionType": "stitch_video",
  "platform": "TIKTOK",
  "version": "1.0.0",
  "description": "Open the Stitch creator for TikTok videos",
  "metadata": {
    "requiresAuth": true,
    "supportsPagination": false,
    "supportsRetry": false
  },
  "inputs": {
    "required": [
      {
        "name": "selectedListItems",
        "type": "list",
        "description": "TikTok video URLs to stitch"
      }
    ]
  },
  "outputs": {
    "success": ["reachedIndex"],
    "failure": ["failedItems"]
  },
  "steps": [
    {
      "id": "t1_stitch",
      "type": "call_bot_method",
      "methodName": "stitch_video",
      "args": ["{{item.url or item}}"],
      "variable_name": "stitchResult",
      "timeout": 30,
      "onError": {
        "action": "skip"
      }
    },
    {
      "id": "log",
      "type": "log",
      "value": "Stitch opened for: {{item.url or item}}"
    }
  ],
  "loops": [
    {
      "id": "process_videos",
      "iterator": "selectedListItems",
      "indexVar": "reachedIndex",
      "steps": ["t1_stitch", "log"],
      "onComplete": "update_action_state"
    }
  ],
  "errorHandling": {
    "globalRetries": 1,
    "retryDelay": 3000,
    "onFinalFailure": "log_and_continue"
  }
}
```

- [ ] **Step 2: Create `duet_video.json`**

```json
{
  "actionType": "duet_video",
  "platform": "TIKTOK",
  "version": "1.0.0",
  "description": "Open the Duet creator for TikTok videos",
  "metadata": {
    "requiresAuth": true,
    "supportsPagination": false,
    "supportsRetry": false
  },
  "inputs": {
    "required": [
      {
        "name": "selectedListItems",
        "type": "list",
        "description": "TikTok video URLs to duet"
      }
    ]
  },
  "outputs": {
    "success": ["reachedIndex"],
    "failure": ["failedItems"]
  },
  "steps": [
    {
      "id": "t1_duet",
      "type": "call_bot_method",
      "methodName": "duet_video",
      "args": ["{{item.url or item}}"],
      "variable_name": "duetResult",
      "timeout": 30,
      "onError": {
        "action": "skip"
      }
    },
    {
      "id": "log",
      "type": "log",
      "value": "Duet opened for: {{item.url or item}}"
    }
  ],
  "loops": [
    {
      "id": "process_videos",
      "iterator": "selectedListItems",
      "indexVar": "reachedIndex",
      "steps": ["t1_duet", "log"],
      "onComplete": "update_action_state"
    }
  ],
  "errorHandling": {
    "globalRetries": 1,
    "retryDelay": 3000,
    "onFinalFailure": "log_and_continue"
  }
}
```

- [ ] **Step 3: Create `share_video.json`**

```json
{
  "actionType": "share_video",
  "platform": "TIKTOK",
  "version": "1.0.0",
  "description": "Copy the share link for TikTok videos",
  "metadata": {
    "requiresAuth": true,
    "supportsPagination": false,
    "supportsRetry": true
  },
  "inputs": {
    "required": [
      {
        "name": "selectedListItems",
        "type": "list",
        "description": "TikTok video URLs to share"
      }
    ]
  },
  "outputs": {
    "success": ["shareLinks", "reachedIndex"],
    "failure": ["failedItems"]
  },
  "steps": [
    {
      "id": "t1_share",
      "type": "call_bot_method",
      "methodName": "share_video",
      "args": ["{{item.url or item}}"],
      "variable_name": "shareResult",
      "timeout": 30,
      "onError": {
        "action": "skip"
      }
    },
    {
      "id": "save_share",
      "type": "save_data",
      "dataSource": "shareResult"
    },
    {
      "id": "log",
      "type": "log",
      "value": "Shared: {{item.url or item}}"
    }
  ],
  "loops": [
    {
      "id": "process_videos",
      "iterator": "selectedListItems",
      "indexVar": "reachedIndex",
      "steps": ["t1_share", "save_share", "log"],
      "onComplete": "update_action_state"
    }
  ],
  "errorHandling": {
    "globalRetries": 2,
    "retryDelay": 3000,
    "onFinalFailure": "log_and_continue"
  }
}
```

- [ ] **Step 4: Commit**

```bash
git add data/actions/tiktok/stitch_video.json data/actions/tiktok/duet_video.json \
        data/actions/tiktok/share_video.json
git commit -m "feat(tiktok): add stitch_video, duet_video, share_video JSON actions"
```

---

## Task 9: Final build and smoke test

- [ ] **Step 1: Full build**

```bash
cd /path/to/newmonoes && go build ./...
```
Expected: PASS — no errors.

- [ ] **Step 2: Smoke test `list_user_videos` via CLI**

```bash
monoes run tiktok.list_user_videos \
  --targets "https://www.tiktok.com/@pissfool" \
  --maxCount 5
```
Expected: JSON output with 5 video URLs, each matching `https://www.tiktok.com/@pissfool/video/...`

- [ ] **Step 3: Smoke test `follow_user` via CLI**

```bash
monoes run tiktok.follow_user \
  --targets "https://www.tiktok.com/@pissfool"
```
Expected: success log "Followed: https://www.tiktok.com/@pissfool" (or "already following" if already followed).

- [ ] **Step 4: Smoke test `like_video` via CLI**

Use one of the video URLs returned from step 2:
```bash
monoes run tiktok.like_video \
  --targets "<video-url-from-step-2>"
```
Expected: success log "Liked: <url>"

- [ ] **Step 5: Smoke test `comment_on_video` via CLI**

```bash
monoes run tiktok.comment_on_video \
  --targets "<video-url-from-step-2>" \
  --commentText "Great video!"
```
Expected: success log "Commented on: <url>"

- [ ] **Step 6: Smoke test `list_video_comments` via CLI**

```bash
monoes run tiktok.list_video_comments \
  --targets "<video-url-from-step-2>" \
  --maxCount 5
```
Expected: JSON output with up to 5 comment objects, each with `username` and `text` fields.

- [ ] **Step 7: Smoke test `like_comment` via CLI**

Use a comment ID from step 6:
```bash
monoes run tiktok.like_comment \
  --targets '{"videoURL":"<video-url>","id":"<comment-id>"}'
```
Expected: success log "Liked comment: <id>"

- [ ] **Step 8: Smoke test `share_video` via CLI**

```bash
monoes run tiktok.share_video \
  --targets "<video-url-from-step-2>"
```
Expected: JSON output `{"success": true, "url": "<video-url>"}` and success log.

- [ ] **Step 9: Smoke test `stitch_video` and `duet_video` via CLI**

```bash
monoes run tiktok.stitch_video \
  --targets "<video-url-from-step-2>"
```
Expected: TikTok creator opens in browser (returns success), or descriptive error if stitch is disabled on that video.

- [ ] **Step 10: Commit if any fixes were needed**

```bash
git add -A
git commit -m "fix(tiktok): smoke test fixes"
```
