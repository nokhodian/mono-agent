package instagram

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

// jsonUnmarshal wraps encoding/json.Unmarshal for use in helpers.
var jsonUnmarshal = json.Unmarshal

// ---------------------------------------------------------------------------
// CommentPost — navigate to a post and leave a comment.
// ---------------------------------------------------------------------------

// CommentPost navigates to the given post URL, finds the comment textarea
// using JavaScript-based section identification (same pattern as LikePost),
// types the comment text using page.Keyboard.Type(), then finds and clicks
// the Post button with a native CDP mouse event.
func (b *InstagramBot) CommentPost(ctx context.Context, page *rod.Page, postURL, commentText string) error {
	if postURL == "" {
		return fmt.Errorf("instagram: post URL is required")
	}
	if commentText == "" {
		return fmt.Errorf("instagram: comment text is required")
	}

	// Navigate to the post.
	err := rod.Try(func() {
		page.MustNavigate(postURL).MustWaitLoad()
	})
	if err != nil {
		return fmt.Errorf("instagram: failed to navigate to post %s: %w", postURL, err)
	}
	time.Sleep(3 * time.Second)

	b.dismissNotificationDialog(page)

	// Step 1: Find and click the comment textarea.
	// Instagram's comment input can be a textarea or a contenteditable div.
	// We use JS to find it in the action section context.
	res, err := page.Timeout(10 * time.Second).Eval(`() => {
		// Clean up any previous marker.
		const prev = document.querySelector('[data-monoes-comment-input]');
		if (prev) prev.removeAttribute('data-monoes-comment-input');

		// Strategy 1: textarea with aria-label containing "comment"
		const textareas = document.querySelectorAll(
			'textarea[aria-label*="comment" i], textarea[aria-label*="Comment" i]'
		);
		if (textareas.length > 0) {
			textareas[textareas.length - 1].setAttribute('data-monoes-comment-input', 'true');
			return 'marked_textarea';
		}

		// Strategy 2: contenteditable div with comment-related aria-label
		const editables = document.querySelectorAll(
			'div[aria-label*="comment" i][role="textbox"], ' +
			'div[aria-label*="comment" i][contenteditable="true"]'
		);
		if (editables.length > 0) {
			editables[editables.length - 1].setAttribute('data-monoes-comment-input', 'true');
			return 'marked_editable';
		}

		// Strategy 3: any form textarea or contenteditable in the post context
		const formTextareas = document.querySelectorAll('form textarea');
		if (formTextareas.length > 0) {
			formTextareas[formTextareas.length - 1].setAttribute('data-monoes-comment-input', 'true');
			return 'marked_form_textarea';
		}

		return 'not_found';
	}`)
	if err != nil {
		return fmt.Errorf("instagram: failed to evaluate comment input script on %s: %w", postURL, err)
	}

	state := res.Value.Str()
	if state == "not_found" {
		return fmt.Errorf("instagram: could not find comment input on %s", postURL)
	}

	// Find the marked element with rod and click to focus it.
	commentInput, err := page.Timeout(5 * time.Second).Element("[data-monoes-comment-input='true']")
	if err != nil {
		return fmt.Errorf("instagram: marked comment input not found on %s: %w", postURL, err)
	}

	commentInput.MustScrollIntoView()
	time.Sleep(500 * time.Millisecond)

	if err := commentInput.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("instagram: failed to click comment input: %w", err)
	}
	time.Sleep(1 * time.Second)

	// Type the comment using page.Keyboard.Type() for React event compatibility.
	for _, ch := range commentText {
		if err := page.Keyboard.Type(input.Key(ch)); err != nil {
			return fmt.Errorf("instagram: failed to type character %c: %w", ch, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(1 * time.Second)

	// Step 2: Find and click the Post button.
	postRes, err := page.Timeout(10 * time.Second).Eval(`() => {
		const prev = document.querySelector('[data-monoes-post-btn]');
		if (prev) prev.removeAttribute('data-monoes-post-btn');

		// Strategy 1: div[role=button] or button with text "Post"
		const allBtns = document.querySelectorAll(
			'div[role="button"], button'
		);
		for (const btn of allBtns) {
			const text = (btn.textContent || '').trim();
			if (text === 'Post') {
				btn.setAttribute('data-monoes-post-btn', 'true');
				return 'marked';
			}
		}

		// Strategy 2: submit button inside a form
		const submitBtn = document.querySelector('form button[type="submit"]');
		if (submitBtn) {
			submitBtn.setAttribute('data-monoes-post-btn', 'true');
			return 'marked';
		}

		return 'not_found';
	}`)
	if err != nil {
		return fmt.Errorf("instagram: failed to evaluate post button script: %w", err)
	}

	if postRes.Value.Str() != "marked" {
		return fmt.Errorf("instagram: could not find Post button on %s", postURL)
	}

	postBtn, err := page.Timeout(5 * time.Second).Element("[data-monoes-post-btn='true']")
	if err != nil {
		return fmt.Errorf("instagram: marked Post button not found: %w", err)
	}

	postBtn.MustScrollIntoView()
	time.Sleep(500 * time.Millisecond)

	if err := postBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("instagram: failed to click Post button: %w", err)
	}

	time.Sleep(3 * time.Second)

	// Clean up markers.
	page.Eval(`() => {
		const el1 = document.querySelector('[data-monoes-comment-input]');
		if (el1) el1.removeAttribute('data-monoes-comment-input');
		const el2 = document.querySelector('[data-monoes-post-btn]');
		if (el2) el2.removeAttribute('data-monoes-post-btn');
	}`)

	return nil
}

// ---------------------------------------------------------------------------
// ReplyToConversation — navigate to DM inbox and reply to a conversation.
// ---------------------------------------------------------------------------

// ReplyToConversation navigates to the DM inbox, finds the conversation
// matching the given URL or username, opens it, and sends a reply.
func (b *InstagramBot) ReplyToConversation(ctx context.Context, page *rod.Page, conversationURL, replyText string) error {
	if replyText == "" {
		return fmt.Errorf("instagram: reply text is required")
	}

	// Navigate to the conversation directly if a full URL is provided.
	if strings.Contains(conversationURL, "/direct/t/") {
		err := rod.Try(func() {
			page.MustNavigate(conversationURL).MustWaitLoad()
		})
		if err != nil {
			return fmt.Errorf("instagram: failed to navigate to conversation %s: %w", conversationURL, err)
		}
		time.Sleep(3 * time.Second)
		b.dismissNotificationDialog(page)
		return b.typeAndSendMessage(page, replyText)
	}

	// Navigate to inbox first.
	err := rod.Try(func() {
		page.MustNavigate("https://www.instagram.com/direct/inbox/").MustWaitLoad()
	})
	if err != nil {
		return fmt.Errorf("instagram: failed to navigate to inbox: %w", err)
	}
	time.Sleep(3 * time.Second)
	b.dismissNotificationDialog(page)

	// Try to find and click the conversation in the inbox list.
	username := conversationURL
	if strings.Contains(conversationURL, "instagram.com") {
		username = b.ExtractUsername(conversationURL)
	}

	// Search for the conversation by clicking on a user entry.
	convXPaths := []string{
		fmt.Sprintf("//a[contains(@href, '/direct/t/')][.//span[contains(text(), '%s')]]", username),
		fmt.Sprintf("//div[@role='listitem'][.//span[contains(text(), '%s')]]", username),
		fmt.Sprintf("//div[@role='button'][.//span[contains(text(), '%s')]]", username),
	}

	clicked := false
	for _, xpath := range convXPaths {
		var el *rod.Element
		tryErr := rod.Try(func() {
			el = page.Timeout(5 * time.Second).MustElementX(xpath)
		})
		if tryErr == nil && el != nil {
			if clickErr := el.Click(proto.InputMouseButtonLeft, 1); clickErr == nil {
				clicked = true
				break
			}
		}
	}

	if !clicked {
		return fmt.Errorf("instagram: could not find conversation for %q in inbox", username)
	}

	time.Sleep(3 * time.Second)
	return b.typeAndSendMessage(page, replyText)
}

// ---------------------------------------------------------------------------
// FetchFollowersList — extract followers or following list from a profile.
// ---------------------------------------------------------------------------

// FetchFollowersList navigates to the given profile, clicks the followers or
// following link (based on sourceType), and extracts user entries from the
// dialog via scroll-and-extract loop.
func (b *InstagramBot) FetchFollowersList(ctx context.Context, page *rod.Page, profileURL, sourceType string, maxCount int) ([]map[string]interface{}, error) {
	if profileURL == "" {
		return nil, fmt.Errorf("instagram: profile URL is required")
	}

	// Navigate to the profile.
	err := rod.Try(func() {
		page.MustNavigate(profileURL).MustWaitLoad()
	})
	if err != nil {
		return nil, fmt.Errorf("instagram: failed to navigate to profile %s: %w", profileURL, err)
	}
	time.Sleep(3 * time.Second)
	b.dismissNotificationDialog(page)

	// Click followers or following link.
	var linkXPaths []string
	if strings.EqualFold(sourceType, "FOLLOWERS_FETCH") || strings.EqualFold(sourceType, "followers") {
		linkXPaths = []string{
			"//a[contains(@href, '/followers')]",
			"//button[contains(text(), 'follower')]",
			"//span[contains(text(), 'follower')]/ancestor::a",
		}
	} else {
		linkXPaths = []string{
			"//a[contains(@href, '/following')]",
			"//button[contains(text(), 'following')]",
			"//span[contains(text(), 'following')]/ancestor::a",
		}
	}

	clicked := false
	for _, xpath := range linkXPaths {
		var el *rod.Element
		tryErr := rod.Try(func() {
			el = page.Timeout(5 * time.Second).MustElementX(xpath)
		})
		if tryErr == nil && el != nil {
			if clickErr := el.Click(proto.InputMouseButtonLeft, 1); clickErr == nil {
				clicked = true
				break
			}
		}
	}

	if !clicked {
		return nil, fmt.Errorf("instagram: could not find %s link on profile", sourceType)
	}

	time.Sleep(3 * time.Second)

	// Wait for the dialog to appear.
	dialogSelectors := []string{
		"div[role='dialog']",
		"div[class*='dialog']",
	}

	var dialog *rod.Element
	for _, sel := range dialogSelectors {
		el, findErr := page.Timeout(10 * time.Second).Element(sel)
		if findErr == nil && el != nil {
			dialog = el
			break
		}
	}
	if dialog == nil {
		return nil, fmt.Errorf("instagram: followers/following dialog did not appear")
	}

	// Extract users via scroll-and-extract loop.
	var results []map[string]interface{}
	seen := make(map[string]bool)
	maxScrollAttempts := 20

	if maxCount <= 0 {
		maxCount = 100
	}

	for attempt := 0; attempt < maxScrollAttempts; attempt++ {
		// Extract user links from the dialog.
		res, evalErr := page.Eval(`() => {
			const dialog = document.querySelector('div[role="dialog"]');
			if (!dialog) return JSON.stringify([]);

			const links = dialog.querySelectorAll('a[href*="/"]');
			const users = [];
			for (const link of links) {
				const href = link.getAttribute('href') || '';
				if (href && href !== '/' && !href.includes('/p/') && !href.includes('/explore/')) {
					const parts = href.replace(/^\/|\/$/g, '').split('/');
					if (parts.length === 1 && parts[0].length > 0) {
						users.push({
							username: parts[0],
							profile_url: 'https://www.instagram.com/' + parts[0] + '/',
							platform: 'INSTAGRAM'
						});
					}
				}
			}
			return JSON.stringify(users);
		}`)
		if evalErr != nil {
			break
		}

		var batch []map[string]interface{}
		if jsonStr := res.Value.Str(); jsonStr != "" {
			// Parse manually since rod returns a gjson value.
			_ = parseJSONArray(jsonStr, &batch)
		}

		for _, user := range batch {
			username, _ := user["username"].(string)
			if username == "" || seen[username] {
				continue
			}
			seen[username] = true
			results = append(results, user)
		}

		if len(results) >= maxCount {
			results = results[:maxCount]
			break
		}

		// Scroll the dialog to load more.
		_, _ = page.Eval(`() => {
			const dialog = document.querySelector('div[role="dialog"]');
			if (!dialog) return;
			const scrollable = dialog.querySelector('div[style*="overflow"]') || dialog;
			scrollable.scrollTop = scrollable.scrollHeight;
		}`)
		time.Sleep(2 * time.Second)

		// Check if we got new results.
		if attempt > 3 && len(results) == len(seen) {
			// No new results after scrolling — we've hit the bottom.
			break
		}
	}

	return results, nil
}

// parseJSONArray is a helper to unmarshal a JSON array string.
func parseJSONArray(jsonStr string, target *[]map[string]interface{}) error {
	return json.Unmarshal([]byte(jsonStr), target)
}

// ---------------------------------------------------------------------------
// InteractWithPosts — search by keyword and interact with found posts.
// ---------------------------------------------------------------------------

// InteractWithPosts navigates to the explore/tags page for the given keyword,
// extracts post links, then iterates through them calling LikePost and
// optionally CommentPost.
func (b *InstagramBot) InteractWithPosts(ctx context.Context, page *rod.Page, keyword string, maxCount int, commentText string) (map[string]interface{}, error) {
	if keyword == "" {
		return nil, fmt.Errorf("instagram: keyword is required")
	}

	searchURL := b.SearchURL(keyword)

	err := rod.Try(func() {
		page.MustNavigate(searchURL).MustWaitLoad()
	})
	if err != nil {
		return nil, fmt.Errorf("instagram: failed to navigate to search page: %w", err)
	}
	time.Sleep(3 * time.Second)
	b.dismissNotificationDialog(page)

	// Extract post links from the page.
	postLinks, err := b.extractPostLinks(page, maxCount)
	if err != nil {
		return nil, fmt.Errorf("instagram: failed to extract post links: %w", err)
	}

	if len(postLinks) == 0 {
		return map[string]interface{}{
			"success":     true,
			"liked_count": 0,
			"total_found": 0,
		}, nil
	}

	// Interact with each post.
	likedCount := 0
	commentedCount := 0
	for i, postURL := range postLinks {
		if i >= maxCount {
			break
		}

		if err := b.interactWithSinglePost(ctx, page, postURL, commentText); err != nil {
			continue
		}

		likedCount++
		if commentText != "" {
			commentedCount++
		}

		time.Sleep(2 * time.Second)
	}

	return map[string]interface{}{
		"success":         true,
		"liked_count":     likedCount,
		"commented_count": commentedCount,
		"total_found":     len(postLinks),
	}, nil
}

// ---------------------------------------------------------------------------
// InteractWithUserPosts — visit a user's profile and interact with their posts.
// ---------------------------------------------------------------------------

// InteractWithUserPosts navigates to the given user's profile, extracts post
// links from their grid, then iterates through them calling LikePost and
// optionally CommentPost.
func (b *InstagramBot) InteractWithUserPosts(ctx context.Context, page *rod.Page, username string, maxCount int, commentText string) (map[string]interface{}, error) {
	if username == "" {
		return nil, fmt.Errorf("instagram: username is required")
	}

	profileURL := fmt.Sprintf("https://www.instagram.com/%s/", url.PathEscape(username))

	err := rod.Try(func() {
		page.MustNavigate(profileURL).MustWaitLoad()
	})
	if err != nil {
		return nil, fmt.Errorf("instagram: failed to navigate to profile %s: %w", profileURL, err)
	}
	time.Sleep(3 * time.Second)
	b.dismissNotificationDialog(page)

	// Extract post links from the profile grid.
	postLinks, err := b.extractPostLinks(page, maxCount)
	if err != nil {
		return nil, fmt.Errorf("instagram: failed to extract post links from profile: %w", err)
	}

	if len(postLinks) == 0 {
		return map[string]interface{}{
			"success":     true,
			"liked_count": 0,
			"total_found": 0,
		}, nil
	}

	// Interact with each post.
	likedCount := 0
	commentedCount := 0
	for i, postURL := range postLinks {
		if i >= maxCount {
			break
		}

		if err := b.interactWithSinglePost(ctx, page, postURL, commentText); err != nil {
			continue
		}

		likedCount++
		if commentText != "" {
			commentedCount++
		}

		time.Sleep(2 * time.Second)
	}

	return map[string]interface{}{
		"success":         true,
		"liked_count":     likedCount,
		"commented_count": commentedCount,
		"total_found":     len(postLinks),
	}, nil
}

// ---------------------------------------------------------------------------
// PublishContent — create and publish a new Instagram post.
// ---------------------------------------------------------------------------

// PublishContent handles the Instagram post creation flow: finds the "New post"
// button, uploads media via the file input, advances through screens, types
// a caption, optionally adds a location, and clicks Share.
func (b *InstagramBot) PublishContent(ctx context.Context, page *rod.Page, mediaPath, caption, locationTag string) error {
	if mediaPath == "" {
		return fmt.Errorf("instagram: media path is required")
	}

	// Navigate to Instagram home to access the create button.
	err := rod.Try(func() {
		page.MustNavigate("https://www.instagram.com/").MustWaitLoad()
	})
	if err != nil {
		return fmt.Errorf("instagram: failed to navigate to home: %w", err)
	}
	time.Sleep(3 * time.Second)
	b.dismissNotificationDialog(page)

	// Step 1: Find and click the "New post" button.
	createRes, err := page.Timeout(10 * time.Second).Eval(`() => {
		const prev = document.querySelector('[data-monoes-create-btn]');
		if (prev) prev.removeAttribute('data-monoes-create-btn');

		// Strategy 1: SVG with aria-label "New post"
		const svg = document.querySelector('svg[aria-label="New post"]');
		if (svg) {
			const btn = svg.closest('a') || svg.closest('div[role="button"]') || svg.closest('button') || svg.parentElement;
			if (btn) {
				btn.setAttribute('data-monoes-create-btn', 'true');
				return 'marked';
			}
		}

		// Strategy 2: link to /create/
		const createLink = document.querySelector('a[href*="/create"]');
		if (createLink) {
			createLink.setAttribute('data-monoes-create-btn', 'true');
			return 'marked';
		}

		return 'not_found';
	}`)
	if err != nil {
		return fmt.Errorf("instagram: failed to find create button: %w", err)
	}

	if createRes.Value.Str() != "marked" {
		return fmt.Errorf("instagram: could not find 'New post' button")
	}

	createBtn, err := page.Timeout(5 * time.Second).Element("[data-monoes-create-btn='true']")
	if err != nil {
		return fmt.Errorf("instagram: marked create button not found: %w", err)
	}

	if err := createBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("instagram: failed to click create button: %w", err)
	}
	time.Sleep(3 * time.Second)

	// Step 2: Set the file input for media upload.
	fileInput, err := page.Timeout(10 * time.Second).Element("input[type='file']")
	if err != nil {
		return fmt.Errorf("instagram: could not find file input: %w", err)
	}

	if err := fileInput.SetFiles([]string{mediaPath}); err != nil {
		return fmt.Errorf("instagram: failed to set file input: %w", err)
	}
	time.Sleep(5 * time.Second)

	// Step 3: Click through "Next" buttons (crop screen → filter screen → caption screen).
	for i := 0; i < 3; i++ {
		nextBtnXPaths := []string{
			"//button[normalize-space(.)='Next']",
			"//div[@role='button'][normalize-space(.)='Next']",
		}
		for _, xpath := range nextBtnXPaths {
			var btn *rod.Element
			tryErr := rod.Try(func() {
				btn = page.Timeout(5 * time.Second).MustElementX(xpath)
			})
			if tryErr == nil && btn != nil {
				_ = btn.Click(proto.InputMouseButtonLeft, 1)
				time.Sleep(2 * time.Second)
				break
			}
		}
	}

	// Step 4: Type the caption.
	if caption != "" {
		captionSelectors := []string{
			"textarea[aria-label*='caption' i]",
			"textarea[aria-label*='Caption' i]",
			"div[aria-label*='caption' i][contenteditable='true']",
			"div[aria-label*='Caption' i][contenteditable='true']",
			"div[role='textbox'][contenteditable='true']",
			"textarea",
		}

		var captionInput *rod.Element
		for _, sel := range captionSelectors {
			el, findErr := page.Timeout(5 * time.Second).Element(sel)
			if findErr == nil && el != nil {
				captionInput = el
				break
			}
		}

		if captionInput != nil {
			if clickErr := captionInput.Click(proto.InputMouseButtonLeft, 1); clickErr == nil {
				time.Sleep(500 * time.Millisecond)
				for _, ch := range caption {
					_ = page.Keyboard.Type(input.Key(ch))
					time.Sleep(30 * time.Millisecond)
				}
				time.Sleep(1 * time.Second)
			}
		}
	}

	// Step 5: Optionally add location.
	if locationTag != "" {
		locSelectors := []string{
			"input[placeholder*='location' i]",
			"input[aria-label*='location' i]",
			"input[placeholder*='Location' i]",
		}
		for _, sel := range locSelectors {
			el, findErr := page.Timeout(3 * time.Second).Element(sel)
			if findErr == nil && el != nil {
				if clickErr := el.Click(proto.InputMouseButtonLeft, 1); clickErr == nil {
					time.Sleep(500 * time.Millisecond)
					for _, ch := range locationTag {
						_ = page.Keyboard.Type(input.Key(ch))
						time.Sleep(50 * time.Millisecond)
					}
					time.Sleep(2 * time.Second)
					// Click first location suggestion.
					var suggestion *rod.Element
					tryErr := rod.Try(func() {
						suggestion = page.Timeout(3*time.Second).MustElementX("//div[@role='listitem'][1]")
					})
					if tryErr == nil && suggestion != nil {
						_ = suggestion.Click(proto.InputMouseButtonLeft, 1)
						time.Sleep(1 * time.Second)
					}
				}
				break
			}
		}
	}

	// Step 6: Click "Share" button.
	shareBtnXPaths := []string{
		"//button[normalize-space(.)='Share']",
		"//div[@role='button'][normalize-space(.)='Share']",
	}
	shared := false
	for _, xpath := range shareBtnXPaths {
		var btn *rod.Element
		tryErr := rod.Try(func() {
			btn = page.Timeout(5 * time.Second).MustElementX(xpath)
		})
		if tryErr == nil && btn != nil {
			if err := btn.Click(proto.InputMouseButtonLeft, 1); err == nil {
				shared = true
				break
			}
		}
	}
	if !shared {
		return fmt.Errorf("instagram: could not find or click Share button")
	}

	time.Sleep(5 * time.Second)

	// Clean up markers.
	page.Eval(`() => {
		const el = document.querySelector('[data-monoes-create-btn]');
		if (el) el.removeAttribute('data-monoes-create-btn');
	}`)

	return nil
}

// ---------------------------------------------------------------------------
// FollowUser — navigate to a profile and click the Follow button.
// ---------------------------------------------------------------------------

// FollowUser navigates to the given profile URL and clicks the Follow button.
// If the user is already followed (button shows "Following" or "Requested"),
// it returns nil (no-op).
func (b *InstagramBot) FollowUser(ctx context.Context, page *rod.Page, profileURL string) error {
	if profileURL == "" {
		return fmt.Errorf("instagram: profile URL is required")
	}

	err := rod.Try(func() {
		page.MustNavigate(profileURL).MustWaitLoad()
	})
	if err != nil {
		return fmt.Errorf("instagram: failed to navigate to profile %s: %w", profileURL, err)
	}
	time.Sleep(3 * time.Second)
	b.dismissNotificationDialog(page)

	// Use JS to identify the Follow button state and mark it.
	res, err := page.Timeout(10 * time.Second).Eval(`() => {
		// Clean up previous markers.
		const prev = document.querySelector('[data-monoes-follow-btn]');
		if (prev) prev.removeAttribute('data-monoes-follow-btn');

		// Scan all buttons for follow-related text.
		const buttons = document.querySelectorAll('button, div[role="button"]');
		for (const btn of buttons) {
			const text = (btn.textContent || '').trim();
			// Already following — no-op.
			if (text === 'Following' || text === 'Requested') {
				return 'already_following';
			}
		}
		// Find the "Follow" button (not "Follow Back" exclusion — Follow Back is OK).
		for (const btn of buttons) {
			const text = (btn.textContent || '').trim();
			if (text === 'Follow' || text === 'Follow Back') {
				btn.setAttribute('data-monoes-follow-btn', 'true');
				return 'marked';
			}
		}
		return 'not_found';
	}`)
	if err != nil {
		return fmt.Errorf("instagram: failed to evaluate follow button script on %s: %w", profileURL, err)
	}

	state := res.Value.Str()
	if state == "already_following" || state == "not_found" {
		// already_following: user is already followed, no action needed.
		// not_found: no Follow button exists (own profile, restricted, etc.) — treat as no-op.
		return nil
	}

	// Click the marked Follow button.
	followBtn, err := page.Timeout(5 * time.Second).Element("[data-monoes-follow-btn='true']")
	if err != nil {
		return fmt.Errorf("instagram: marked Follow button not found on %s: %w", profileURL, err)
	}

	followBtn.MustScrollIntoView()
	time.Sleep(500 * time.Millisecond)

	if err := followBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("instagram: failed to click Follow button: %w", err)
	}
	time.Sleep(3 * time.Second)

	// Verify: check that button text changed.
	verifyRes, _ := page.Eval(`() => {
		const buttons = document.querySelectorAll('button, div[role="button"]');
		for (const btn of buttons) {
			const text = (btn.textContent || '').trim();
			if (text === 'Following' || text === 'Requested') {
				return 'confirmed';
			}
		}
		return 'unconfirmed';
	}`)
	if verifyRes != nil && verifyRes.Value.Str() == "confirmed" {
		// Success.
	}

	// Clean up marker.
	page.Eval(`() => {
		const el = document.querySelector('[data-monoes-follow-btn]');
		if (el) el.removeAttribute('data-monoes-follow-btn');
	}`)

	return nil
}

// ---------------------------------------------------------------------------
// UnfollowUser — navigate to a profile and unfollow the user.
// ---------------------------------------------------------------------------

// UnfollowUser navigates to the given profile URL, clicks the "Following"
// button, then confirms the unfollow in the dialog. If the user is not
// followed (button shows "Follow"), it returns nil (no-op).
func (b *InstagramBot) UnfollowUser(ctx context.Context, page *rod.Page, profileURL string) error {
	if profileURL == "" {
		return fmt.Errorf("instagram: profile URL is required")
	}

	err := rod.Try(func() {
		page.MustNavigate(profileURL).MustWaitLoad()
	})
	if err != nil {
		return fmt.Errorf("instagram: failed to navigate to profile %s: %w", profileURL, err)
	}
	time.Sleep(3 * time.Second)
	b.dismissNotificationDialog(page)

	// Find and mark the "Following" button.
	res, err := page.Timeout(10 * time.Second).Eval(`() => {
		const prev = document.querySelector('[data-monoes-following-btn]');
		if (prev) prev.removeAttribute('data-monoes-following-btn');

		const buttons = document.querySelectorAll('button, div[role="button"]');
		for (const btn of buttons) {
			const text = (btn.textContent || '').trim();
			if (text === 'Follow' || text === 'Follow Back') {
				return 'not_following';
			}
		}
		for (const btn of buttons) {
			const text = (btn.textContent || '').trim();
			if (text === 'Following' || text === 'Requested') {
				btn.setAttribute('data-monoes-following-btn', 'true');
				return 'marked';
			}
		}
		return 'not_found';
	}`)
	if err != nil {
		return fmt.Errorf("instagram: failed to evaluate following button script on %s: %w", profileURL, err)
	}

	state := res.Value.Str()
	if state == "not_following" || state == "not_found" {
		// not_following: user is not followed, nothing to unfollow.
		// not_found: no Following button exists (own profile, restricted, etc.) — treat as no-op.
		return nil
	}

	// Click the "Following" button to open unfollow confirmation dialog.
	followingBtn, err := page.Timeout(5 * time.Second).Element("[data-monoes-following-btn='true']")
	if err != nil {
		return fmt.Errorf("instagram: marked Following button not found on %s: %w", profileURL, err)
	}

	followingBtn.MustScrollIntoView()
	time.Sleep(500 * time.Millisecond)

	if err := followingBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("instagram: failed to click Following button: %w", err)
	}
	time.Sleep(2 * time.Second)

	// Find and click the "Unfollow" confirmation button in the dialog.
	confirmRes, err := page.Timeout(10 * time.Second).Eval(`() => {
		const prev = document.querySelector('[data-monoes-unfollow-confirm]');
		if (prev) prev.removeAttribute('data-monoes-unfollow-confirm');

		// Look for "Unfollow" button inside a dialog.
		const dialog = document.querySelector('div[role="dialog"]');
		if (!dialog) return 'no_dialog';

		const buttons = dialog.querySelectorAll('button, div[role="button"]');
		for (const btn of buttons) {
			const text = (btn.textContent || '').trim();
			if (text === 'Unfollow') {
				btn.setAttribute('data-monoes-unfollow-confirm', 'true');
				return 'marked';
			}
		}
		return 'not_found';
	}`)
	if err != nil {
		return fmt.Errorf("instagram: failed to evaluate unfollow dialog script: %w", err)
	}

	confirmState := confirmRes.Value.Str()
	if confirmState != "marked" {
		return fmt.Errorf("instagram: could not find Unfollow confirmation button (state: %s)", confirmState)
	}

	confirmBtn, err := page.Timeout(5 * time.Second).Element("[data-monoes-unfollow-confirm='true']")
	if err != nil {
		return fmt.Errorf("instagram: marked Unfollow confirm button not found: %w", err)
	}

	if err := confirmBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("instagram: failed to click Unfollow confirm: %w", err)
	}
	time.Sleep(3 * time.Second)

	// Verify: button should now show "Follow".
	page.Eval(`() => {
		const el = document.querySelector('[data-monoes-following-btn]');
		if (el) el.removeAttribute('data-monoes-following-btn');
		const el2 = document.querySelector('[data-monoes-unfollow-confirm]');
		if (el2) el2.removeAttribute('data-monoes-unfollow-confirm');
	}`)

	return nil
}

// ---------------------------------------------------------------------------
// ViewStories — navigate to a profile and watch their stories.
// ---------------------------------------------------------------------------

// ViewStories navigates to the given profile URL, clicks the story ring to
// open the story viewer, then advances through all stories.
func (b *InstagramBot) ViewStories(ctx context.Context, page *rod.Page, profileURL string) error {
	if profileURL == "" {
		return fmt.Errorf("instagram: profile URL is required")
	}

	err := rod.Try(func() {
		page.MustNavigate(profileURL).MustWaitLoad()
	})
	if err != nil {
		return fmt.Errorf("instagram: failed to navigate to profile %s: %w", profileURL, err)
	}
	time.Sleep(3 * time.Second)
	b.dismissNotificationDialog(page)

	// Find the story ring (gradient-bordered profile picture in header).
	storyRes, err := page.Timeout(10 * time.Second).Eval(`() => {
		const prev = document.querySelector('[data-monoes-story-ring]');
		if (prev) prev.removeAttribute('data-monoes-story-ring');

		// Strategy 1: Find a canvas element near the profile picture (story ring indicator).
		const canvases = document.querySelectorAll('header canvas');
		if (canvases.length > 0) {
			// The canvas parent or grandparent is clickable.
			let clickable = canvases[0].closest('div[role="button"]') ||
				canvases[0].closest('a') ||
				canvases[0].parentElement;
			if (clickable) {
				clickable.setAttribute('data-monoes-story-ring', 'true');
				return 'marked_canvas';
			}
		}

		// Strategy 2: Profile image in header that is wrapped in a clickable role=button with a gradient.
		const headerImgs = document.querySelectorAll('header img[alt]');
		for (const img of headerImgs) {
			// The story ring wraps the profile image in a role="button" ancestor.
			const btn = img.closest('div[role="button"]') || img.closest('a[role="link"]');
			if (btn) {
				btn.setAttribute('data-monoes-story-ring', 'true');
				return 'marked_img';
			}
		}

		// Strategy 3: Any clickable element in header containing a profile-size image.
		const headerLinks = document.querySelectorAll('header a, header div[role="button"]');
		for (const el of headerLinks) {
			const imgs = el.querySelectorAll('img');
			if (imgs.length > 0) {
				const img = imgs[0];
				const rect = img.getBoundingClientRect();
				// Profile pics are typically 77-150px.
				if (rect.width >= 50 && rect.width <= 200) {
					el.setAttribute('data-monoes-story-ring', 'true');
					return 'marked_header_link';
				}
			}
		}

		return 'not_found';
	}`)
	if err != nil {
		return fmt.Errorf("instagram: failed to evaluate story ring script on %s: %w", profileURL, err)
	}

	state := storyRes.Value.Str()
	if state == "not_found" {
		// No active stories — treat as no-op rather than error.
		return nil
	}

	// Click the story ring to open stories.
	storyRing, err := page.Timeout(5 * time.Second).Element("[data-monoes-story-ring='true']")
	if err != nil {
		return fmt.Errorf("instagram: marked story ring not found on %s: %w", profileURL, err)
	}

	// Story ring is in the header — skip ScrollIntoView (can hang on invisible elements)
	// and click directly.
	time.Sleep(500 * time.Millisecond)

	if err := storyRing.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("instagram: failed to click story ring: %w", err)
	}
	time.Sleep(3 * time.Second)

	// Advance through stories by clicking the right side or the next button.
	// Stories auto-advance but we help by clicking. We loop until we detect
	// the story viewer has closed or we exhaust attempts.
	maxStories := 10
	for i := 0; i < maxStories; i++ {
		// Check if we're still in the story viewer.
		stillViewing, evalErr := page.Timeout(5 * time.Second).Eval(`() => {
			const closeBtn = document.querySelector('svg[aria-label="Close"]');
			const storyImg = document.querySelector('img[decoding="sync"]');
			const storyVideo = document.querySelector('video');
			return (closeBtn || storyImg || storyVideo) ? 'viewing' : 'done';
		}`)
		if evalErr != nil || stillViewing == nil || stillViewing.Value.Str() == "done" {
			break
		}

		// Wait for the story to display.
		time.Sleep(5 * time.Second)

		// Try to advance to next story by clicking the right side of the viewport.
		page.Timeout(3 * time.Second).Eval(`() => {
			const prev = document.querySelector('[data-monoes-story-next]');
			if (prev) prev.removeAttribute('data-monoes-story-next');

			const nextBtn = document.querySelector('button[aria-label="Next"]') ||
				document.querySelector('div[role="button"][aria-label="Next"]');
			if (nextBtn) {
				nextBtn.setAttribute('data-monoes-story-next', 'true');
				return 'marked_next';
			}
			return 'not_found';
		}`)

		nextBtn, findErr := page.Timeout(2 * time.Second).Element("[data-monoes-story-next='true']")
		if findErr == nil && nextBtn != nil {
			_ = nextBtn.Click(proto.InputMouseButtonLeft, 1)
		} else {
			// Fallback: click the right side of the screen using keyboard right arrow.
			_ = page.Keyboard.Press(input.ArrowRight)
		}
		time.Sleep(1 * time.Second)
	}

	// Clean up markers.
	page.Eval(`() => {
		const el1 = document.querySelector('[data-monoes-story-ring]');
		if (el1) el1.removeAttribute('data-monoes-story-ring');
		const el2 = document.querySelector('[data-monoes-story-next]');
		if (el2) el2.removeAttribute('data-monoes-story-next');
	}`)

	return nil
}

// ---------------------------------------------------------------------------
// ScrapePostData — navigate to a post and extract structured data.
// ---------------------------------------------------------------------------

// ScrapePostData navigates to the given post URL and extracts structured
// information including author, caption, likes, comments count, date, and
// media URLs.
func (b *InstagramBot) ScrapePostData(ctx context.Context, page *rod.Page, postURL string) (map[string]interface{}, error) {
	if postURL == "" {
		return nil, fmt.Errorf("instagram: post URL is required")
	}

	err := rod.Try(func() {
		page.MustNavigate(postURL).MustWaitLoad()
	})
	if err != nil {
		return nil, fmt.Errorf("instagram: failed to navigate to post %s: %w", postURL, err)
	}
	time.Sleep(3 * time.Second)
	b.dismissNotificationDialog(page)

	// Extract all post data via a single JS evaluation.
	res, err := page.Timeout(15 * time.Second).Eval(`() => {
		const data = {};

		// Author username — from header link or meta tags.
		const authorLink = document.querySelector('header a[href*="/"]');
		if (authorLink) {
			const href = authorLink.getAttribute('href') || '';
			const parts = href.replace(/^\/|\/$/g, '').split('/');
			if (parts.length >= 1 && parts[0].length > 0) {
				data.author_username = parts[0];
			}
		}
		// Fallback: OG meta.
		if (!data.author_username) {
			const ogTitle = document.querySelector('meta[property="og:title"]');
			if (ogTitle) {
				const match = (ogTitle.content || '').match(/@(\w+)/);
				if (match) data.author_username = match[1];
			}
		}

		// Caption — from meta description or visible text.
		const ogDesc = document.querySelector('meta[property="og:description"]');
		if (ogDesc) {
			data.caption = ogDesc.content || '';
		}
		// Also try visible caption text.
		if (!data.caption || data.caption.length < 5) {
			const spans = document.querySelectorAll('span');
			let longest = '';
			for (const span of spans) {
				const text = (span.textContent || '').trim();
				if (text.length > longest.length && text.length > 20 && text.length < 2000) {
					// Skip if it looks like a comment (has a username prefix pattern).
					longest = text;
				}
			}
			if (longest) data.caption = longest;
		}

		// Likes count.
		const likeSections = document.querySelectorAll('section');
		for (const section of likeSections) {
			const text = section.textContent || '';
			const likeMatch = text.match(/([\d,]+)\s*like/i);
			if (likeMatch) {
				data.likes_count = likeMatch[1].replace(/,/g, '');
				break;
			}
		}
		// Fallback: button with "others" or "like" text.
		if (!data.likes_count) {
			const btns = document.querySelectorAll('button, a, span');
			for (const btn of btns) {
				const text = (btn.textContent || '').trim();
				const match = text.match(/([\d,]+)\s*like/i) || text.match(/([\d,]+)\s*other/i);
				if (match) {
					data.likes_count = match[1].replace(/,/g, '');
					break;
				}
			}
		}

		// Comments count — from visible elements.
		const allText = document.body.textContent || '';
		const commentMatch = allText.match(/View all ([\d,]+) comments/i);
		if (commentMatch) {
			data.comments_count = commentMatch[1].replace(/,/g, '');
		}

		// Post date — from time element.
		const timeEl = document.querySelector('time[datetime]');
		if (timeEl) {
			data.post_date = timeEl.getAttribute('datetime');
		}

		// Media URLs.
		const mediaUrls = [];
		// Images in post content (not profile pics, not tiny icons).
		const imgs = document.querySelectorAll('img[src]');
		for (const img of imgs) {
			const src = img.getAttribute('src') || '';
			const rect = img.getBoundingClientRect();
			if (rect.width > 200 && src.includes('instagram') && !src.includes('150x150')) {
				mediaUrls.push(src);
			}
		}
		// Videos.
		const videos = document.querySelectorAll('video[src], video source[src]');
		for (const vid of videos) {
			const src = vid.getAttribute('src') || '';
			if (src) mediaUrls.push(src);
		}
		data.media_urls = mediaUrls;

		// Post type.
		const hasVideo = document.querySelector('video') !== null;
		const carouselBtn = document.querySelector('button[aria-label="Next"]') ||
			document.querySelector('div[role="button"][aria-label="Next"]');
		if (carouselBtn) {
			data.post_type = 'carousel';
		} else if (hasVideo) {
			data.post_type = 'video';
		} else {
			data.post_type = 'image';
		}

		data.post_url = window.location.href;

		return JSON.stringify(data);
	}`)
	if err != nil {
		return nil, fmt.Errorf("instagram: failed to evaluate scrape script on %s: %w", postURL, err)
	}

	var data map[string]interface{}
	jsonStr := res.Value.Str()
	if jsonStr == "" {
		return nil, fmt.Errorf("instagram: empty scrape result from %s", postURL)
	}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, fmt.Errorf("instagram: failed to parse scraped data: %w", err)
	}

	return data, nil
}

// ---------------------------------------------------------------------------
// LikeComment — navigate to a post and like a specific comment.
// ---------------------------------------------------------------------------

// LikeComment navigates to the given post URL, finds a comment by the
// specified author, and clicks the like heart on that comment. If no
// matching comment is found, it likes the most recent comment as fallback.
func (b *InstagramBot) LikeComment(ctx context.Context, page *rod.Page, postURL, commentAuthor string) error {
	if postURL == "" {
		return fmt.Errorf("instagram: post URL is required")
	}

	err := rod.Try(func() {
		page.MustNavigate(postURL).MustWaitLoad()
	})
	if err != nil {
		return fmt.Errorf("instagram: failed to navigate to post %s: %w", postURL, err)
	}
	time.Sleep(3 * time.Second)
	b.dismissNotificationDialog(page)

	// Find the comment by author and mark the like button.
	res, err := page.Timeout(10 * time.Second).Eval(fmt.Sprintf(`() => {
		const prev = document.querySelector('[data-monoes-comment-like]');
		if (prev) prev.removeAttribute('data-monoes-comment-like');

		const targetAuthor = '%s'.toLowerCase();

		// Find all comment containers — they contain a username link and a like heart.
		// Comments are NOT inside <section> elements (unlike the post action bar).
		// They appear as list items or divs with a username link and a standalone like SVG.

		// Strategy 1: Find comment by author username.
		const allLinks = document.querySelectorAll('a[href*="/"]');
		let targetContainer = null;
		let fallbackContainer = null;

		for (const link of allLinks) {
			const href = (link.getAttribute('href') || '').replace(/^\/|\/$/g, '');
			const parts = href.split('/');
			if (parts.length !== 1 || parts[0].length === 0) continue;

			const username = parts[0].toLowerCase();
			// Find the parent comment container.
			let container = link.closest('li') || link.closest('div[role="button"]') || link.parentElement?.parentElement;
			if (!container) continue;

			// Check if this container has a like SVG (not inside a section — that would be the post like).
			const likeSvg = container.querySelector('svg[aria-label="Like"]');
			if (!likeSvg) continue;

			// Check if it is NOT inside the post action bar section.
			const parentSection = likeSvg.closest('section');
			if (parentSection) continue;  // This is a post-level like, skip.

			if (!fallbackContainer) {
				fallbackContainer = likeSvg.closest('div[role="button"]') || likeSvg.parentElement;
			}

			if (targetAuthor && username === targetAuthor) {
				targetContainer = likeSvg.closest('div[role="button"]') || likeSvg.parentElement;
				break;
			}
		}

		// Strategy 2: If no exact match, try finding comments with matching text.
		if (!targetContainer && targetAuthor) {
			const spans = document.querySelectorAll('span');
			for (const span of spans) {
				if ((span.textContent || '').toLowerCase().includes(targetAuthor)) {
					const container = span.closest('li') || span.closest('div')?.parentElement;
					if (container) {
						const likeSvg = container.querySelector('svg[aria-label="Like"]');
						if (likeSvg) {
							const parentSection = likeSvg.closest('section');
							if (!parentSection) {
								targetContainer = likeSvg.closest('div[role="button"]') || likeSvg.parentElement;
								break;
							}
						}
					}
				}
			}
		}

		const toMark = targetContainer || fallbackContainer;
		if (toMark) {
			// Check if already liked.
			const unlikeSvg = toMark.querySelector('svg[aria-label="Unlike"]');
			if (unlikeSvg) return 'already_liked';

			toMark.setAttribute('data-monoes-comment-like', 'true');
			return 'marked';
		}

		return 'not_found';
	}`, strings.ReplaceAll(commentAuthor, "'", "\\'")))
	if err != nil {
		return fmt.Errorf("instagram: failed to evaluate comment like script on %s: %w", postURL, err)
	}

	state := res.Value.Str()
	if state == "already_liked" || state == "not_found" {
		// already_liked: comment is already liked, no action needed.
		// not_found: no comments found on the post — treat as no-op.
		return nil
	}

	// Click the marked comment like button.
	likeBtn, err := page.Timeout(5 * time.Second).Element("[data-monoes-comment-like='true']")
	if err != nil {
		return fmt.Errorf("instagram: marked comment like button not found: %w", err)
	}

	likeBtn.MustScrollIntoView()
	time.Sleep(500 * time.Millisecond)

	if err := likeBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("instagram: failed to click comment like button: %w", err)
	}
	time.Sleep(2 * time.Second)

	// Clean up marker.
	page.Eval(`() => {
		const el = document.querySelector('[data-monoes-comment-like]');
		if (el) el.removeAttribute('data-monoes-comment-like');
	}`)

	return nil
}

// ---------------------------------------------------------------------------
// Shared private helpers
// ---------------------------------------------------------------------------

// interactWithSinglePost navigates to a post, likes it, and optionally
// comments on it. Used by InteractWithPosts and InteractWithUserPosts.
func (b *InstagramBot) interactWithSinglePost(ctx context.Context, page *rod.Page, postURL, commentText string) error {
	// Like the post (handles navigation internally).
	if err := b.LikePost(ctx, page, postURL); err != nil {
		return fmt.Errorf("like failed on %s: %w", postURL, err)
	}

	// Comment if text is provided.
	if commentText != "" {
		// The page is already on the post from LikePost, but CommentPost
		// navigates again to be safe.
		if err := b.CommentPost(ctx, page, postURL, commentText); err != nil {
			// Comment failure is not fatal for the interaction.
			return nil
		}
	}

	return nil
}

// extractPostLinks extracts post URLs from the current page (search results
// or profile grid) using JavaScript evaluation.
func (b *InstagramBot) extractPostLinks(page *rod.Page, maxCount int) ([]string, error) {
	if maxCount <= 0 {
		maxCount = 20
	}

	res, err := page.Timeout(10 * time.Second).Eval(fmt.Sprintf(`() => {
		const links = document.querySelectorAll('a[href*="/p/"], a[href*="/reel/"]');
		const urls = [];
		const seen = new Set();
		for (const link of links) {
			const href = link.getAttribute('href');
			if (href && !seen.has(href)) {
				seen.add(href);
				// Build absolute URL.
				const abs = href.startsWith('/') ? 'https://www.instagram.com' + href : href;
				urls.push(abs);
				if (urls.length >= %d) break;
			}
		}
		return JSON.stringify(urls);
	}`, maxCount))
	if err != nil {
		return nil, fmt.Errorf("failed to extract post links: %w", err)
	}

	var urls []string
	jsonStr := res.Value.Str()
	if jsonStr != "" {
		_ = jsonUnmarshal([]byte(jsonStr), &urls)
	}

	return urls, nil
}
