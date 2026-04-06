package tiktok

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nokhodian/mono-agent/internal/browser"
)

// ListUserVideos navigates to a TikTok profile page, scrolls to load the video
// grid, and returns up to maxCount video entries.
func (b *TikTokBot) ListUserVideos(ctx context.Context, page browser.PageInterface, profileURL string, maxCount int) ([]map[string]interface{}, error) {
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
	if tab, err := page.Element("[data-e2e='videos-tab']", 5*time.Second); err == nil && tab != nil {
		_ = tab.Click()
		time.Sleep(2 * time.Second)
	}

	// Scroll to load video grid items.
	var videos []map[string]interface{}
	prevCount := 0
	noChangeRounds := 0

	for len(videos) < maxCount && noChangeRounds < 3 {
		result, err := page.Eval(`() => {
			const items = document.querySelectorAll('[data-e2e="user-post-item"]');
			return JSON.stringify(Array.from(items).map(el => {
				const a = el.querySelector('a');
				const img = el.querySelector('img');
				return {
					url: a ? a.href : '',
					thumbnail: img ? img.src : '',
				};
			}).filter(v => v.url !== ''));
		}`)
		if err == nil && result != nil {
			var parsed []map[string]interface{}
			if jsonErr := json.Unmarshal([]byte(result.Str()), &parsed); jsonErr != nil {
				return nil, fmt.Errorf("tiktok: failed to parse video list JSON: %w", jsonErr)
			}
			videos = parsed
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

	return videos, nil
}

// FollowUser navigates to a TikTok profile page and clicks the Follow button.
// Returns nil if already following (idempotent).
func (b *TikTokBot) FollowUser(ctx context.Context, page browser.PageInterface, profileURL string) error {
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

	btn, err := page.Element("[data-e2e='follow-button']", 5*time.Second)
	if err != nil {
		return fmt.Errorf("tiktok: follow button not found on %s", profileURL)
	}

	text, _ := btn.Text()
	text = strings.TrimSpace(strings.ToLower(text))
	if text == "following" || text == "friends" {
		return nil // already following — idempotent
	}

	if err := btn.Click(); err != nil {
		return fmt.Errorf("tiktok: failed to click follow button: %w", err)
	}
	time.Sleep(1 * time.Second)
	return nil
}

// LikeVideo navigates to a TikTok video page and clicks the like button.
// Returns nil if already liked (idempotent).
func (b *TikTokBot) LikeVideo(ctx context.Context, page browser.PageInterface, videoURL string) error {
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

	var likeBtn browser.ElementHandle
	for _, sel := range likeSelectors {
		el, err := page.Element(sel, 5*time.Second)
		if err == nil && el != nil {
			likeBtn = el
			break
		}
	}
	if likeBtn == nil {
		return fmt.Errorf("tiktok: like button not found on %s", videoURL)
	}

	// Check if already liked (aria-pressed="true").
	pressed, _ := likeBtn.Attribute("aria-pressed")
	if pressed != nil && *pressed == "true" {
		return nil // already liked
	}

	if err := likeBtn.Click(); err != nil {
		return fmt.Errorf("tiktok: failed to click like button: %w", err)
	}
	time.Sleep(1 * time.Second)
	return nil
}

// CommentOnVideo navigates to a TikTok video page, opens the comment panel,
// types the comment, and submits it.
func (b *TikTokBot) CommentOnVideo(ctx context.Context, page browser.PageInterface, videoURL string, commentText string) error {
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
		el, err := page.Element(sel, 5*time.Second)
		if err == nil && el != nil {
			_ = el.Click()
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
	var commentInput browser.ElementHandle
	for _, sel := range inputSelectors {
		el, err := page.Element(sel, 5*time.Second)
		if err == nil && el != nil {
			commentInput = el
			break
		}
	}
	if commentInput == nil {
		return fmt.Errorf("tiktok: comment input not found on %s", videoURL)
	}

	if err := commentInput.Click(); err != nil {
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
		el, err := page.Element(sel, 5*time.Second)
		if err == nil && el != nil {
			if clickErr := el.Click(); clickErr == nil {
				time.Sleep(1 * time.Second)
				return nil
			}
		}
	}
	return fmt.Errorf("tiktok: could not find comment submit button on %s", videoURL)
}

// ListVideoComments navigates to a TikTok video page, opens the comment panel,
// and returns up to maxCount comment entries.
func (b *TikTokBot) ListVideoComments(ctx context.Context, page browser.PageInterface, videoURL string, maxCount int) ([]map[string]interface{}, error) {
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
	for _, sel := range []string{"[data-e2e='comment-icon']", "[data-e2e='browse-comment-button']"} {
		el, err := page.Element(sel, 5*time.Second)
		if err == nil && el != nil {
			_ = el.Click()
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
			return JSON.stringify(Array.from(items).map(el => {
				const username = el.querySelector('[data-e2e="comment-username"]');
				const content = el.querySelector('[data-e2e="comment-content"]');
				return {
					id: el.getAttribute('data-comment-id') || el.id || '',
					username: username ? username.innerText.trim() : '',
					text: content ? content.innerText.trim() : '',
				};
			}).filter(c => c.text !== ''));
		}`)
		if err == nil && result != nil {
			var parsed []map[string]interface{}
			if jsonErr := json.Unmarshal([]byte(result.Str()), &parsed); jsonErr != nil {
				return nil, fmt.Errorf("tiktok: failed to parse comment list JSON: %w", jsonErr)
			}
			comments = parsed
		}

		if len(comments) == prevCount {
			noChangeRounds++
		} else {
			noChangeRounds = 0
			prevCount = len(comments)
		}

		if len(comments) < maxCount {
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
	return comments, nil
}

// LikeComment navigates to a TikTok video page, opens the comment panel, finds
// the comment by ID, and clicks its like button.
func (b *TikTokBot) LikeComment(ctx context.Context, page browser.PageInterface, videoURL string, commentID string) error {
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
		el, err := page.Element(sel, 5*time.Second)
		if err == nil && el != nil {
			_ = el.Click()
			time.Sleep(2 * time.Second)
			break
		}
	}

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
		if !result.Bool() {
			return fmt.Errorf("tiktok: comment %s not found on page %s", commentID, videoURL)
		}
	}
	time.Sleep(1 * time.Second)
	return nil
}

// openShareModal clicks the share icon on the current page to open the share modal.
func openShareModal(page browser.PageInterface, videoURL string) error {
	shareSelectors := []string{
		"[data-e2e='share-icon']",
		"[data-e2e='share-btn']",
	}
	for _, sel := range shareSelectors {
		el, err := page.Element(sel, 5*time.Second)
		if err == nil && el != nil {
			if clickErr := el.Click(); clickErr == nil {
				time.Sleep(2 * time.Second)
				return nil
			}
		}
	}
	return fmt.Errorf("tiktok: share button not found on %s", videoURL)
}

// StitchVideo navigates to a TikTok video page, opens the share modal, and
// clicks the Stitch option to open the stitch creator.
func (b *TikTokBot) StitchVideo(ctx context.Context, page browser.PageInterface, videoURL string) error {
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
		"button[class*='stitch' i]",
	}
	for _, sel := range stitchSelectors {
		el, err := page.Element(sel, 5*time.Second)
		if err == nil && el != nil {
			if clickErr := el.Click(); clickErr == nil {
				time.Sleep(2 * time.Second)
				return nil
			}
		}
	}
	return fmt.Errorf("tiktok: stitch option not found in share modal for %s", videoURL)
}

// DuetVideo navigates to a TikTok video page, opens the share modal, and
// clicks the Duet option to open the duet creator.
func (b *TikTokBot) DuetVideo(ctx context.Context, page browser.PageInterface, videoURL string) error {
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
		"button[class*='duet' i]",
	}
	for _, sel := range duetSelectors {
		el, err := page.Element(sel, 5*time.Second)
		if err == nil && el != nil {
			if clickErr := el.Click(); clickErr == nil {
				time.Sleep(2 * time.Second)
				return nil
			}
		}
	}
	return fmt.Errorf("tiktok: duet option not found in share modal for %s", videoURL)
}

// ShareVideo navigates to a TikTok video page, opens the share modal, and
// clicks the copy link button. Returns the video URL as the share result.
func (b *TikTokBot) ShareVideo(ctx context.Context, page browser.PageInterface, videoURL string) (interface{}, error) {
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
		"[data-e2e='copy-link']",
	}
	for _, sel := range copyLinkSelectors {
		el, err := page.Element(sel, 5*time.Second)
		if err == nil && el != nil {
			if clickErr := el.Click(); clickErr == nil {
				time.Sleep(1 * time.Second)
				return map[string]interface{}{"success": true, "url": videoURL}, nil
			}
		}
	}
	return nil, fmt.Errorf("tiktok: copy link button not found in share modal for %s", videoURL)
}
