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
	botpkg "github.com/nokhodian/mono-agent/internal/bot"
	"github.com/nokhodian/mono-agent/internal/browser"
)

// reservedPaths contains Instagram URL path segments that do not represent
// user profiles and should be skipped when extracting a username from a URL.
var reservedPaths = map[string]bool{
	"home":     true,
	"explore":  true,
	"reels":    true,
	"direct":   true,
	"accounts": true,
	"p":        true,
	"stories":  true,
	"tv":       true,
}

// InstagramBot implements botpkg.BotAdapter for Instagram.
type InstagramBot struct{}

func init() {
	botpkg.PlatformRegistry["INSTAGRAM"] = func() botpkg.BotAdapter {
		return &InstagramBot{}
	}
}

// Platform returns the canonical platform name.
func (b *InstagramBot) Platform() string {
	return "INSTAGRAM"
}

// LoginURL returns the Instagram login page URL.
func (b *InstagramBot) LoginURL() string {
	return "https://www.instagram.com/accounts/login/"
}

// IsLoggedIn checks whether the user is authenticated on Instagram by looking
// for navigation elements that only appear when logged in.
func (b *InstagramBot) IsLoggedIn(p browser.PageInterface) (bool, error) {
	page := p.(*browser.RodPage).UnwrapRodPage()
	// Instagram renders a navigation bar with specific selectors when logged in.
	// We check for the presence of the navigation element or profile avatar icon.
	selectors := []string{
		// Main navigation bar present on all authenticated pages.
		"nav[role='navigation']",
		// Profile link / avatar in the sidebar or bottom nav.
		"svg[aria-label='Profile']",
		"a[href*='/direct/inbox/']",
		// Alternate: the "New post" icon only appears when logged in.
		"svg[aria-label='New post']",
	}

	for _, sel := range selectors {
		has, _, err := page.Has(sel)
		if err != nil {
			continue
		}
		if has {
			return true, nil
		}
	}

	// As a fallback, check for the login form — if it is present, we are NOT
	// logged in.
	hasLogin, _, err := page.Has("input[name='username']")
	if err != nil {
		return false, fmt.Errorf("instagram: failed to check login state: %w", err)
	}
	if hasLogin {
		return false, nil
	}

	return false, nil
}

// ResolveURL converts a relative Instagram URL to an absolute URL. If the URL
// is already absolute it is returned unchanged.
func (b *InstagramBot) ResolveURL(rawURL string) string {
	if strings.HasPrefix(rawURL, "/") {
		return "https://www.instagram.com" + rawURL
	}
	return rawURL
}

// ExtractUsername parses a profile URL and returns the Instagram username.
// It skips reserved path segments that do not represent user profiles.
func (b *InstagramBot) ExtractUsername(pageURL string) string {
	parsed, err := url.Parse(pageURL)
	if err != nil {
		return ""
	}

	// Trim leading/trailing slashes and split the path.
	trimmed := strings.Trim(parsed.Path, "/")
	if trimmed == "" {
		return ""
	}

	segments := strings.Split(trimmed, "/")
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		if reservedPaths[seg] {
			continue
		}
		return seg
	}

	return ""
}

// SearchURL returns the Instagram explore/tags search URL for the given keyword.
func (b *InstagramBot) SearchURL(keyword string) string {
	// Instagram's tag-based explore URL for discovering content/users.
	safeKeyword := url.PathEscape(strings.TrimSpace(keyword))
	return fmt.Sprintf("https://www.instagram.com/explore/tags/%s/", safeKeyword)
}

// SendMessage sends a direct message to the specified Instagram user.
//
// Strategy 1: Navigate to profile → click "Message" button → type → send.
// Strategy 2 (fallback): Use /direct/new/ compose flow to search for user.
func (b *InstagramBot) SendMessage(ctx context.Context, p browser.PageInterface, username, message string) error {
	page := p.(*browser.RodPage).UnwrapRodPage()
	if username == "" {
		return fmt.Errorf("instagram: username is required")
	}
	if message == "" {
		return fmt.Errorf("instagram: message is required")
	}

	// Strategy 1: Try the profile "Message" button.
	if err := b.sendViaProfileButton(page, username, message); err == nil {
		return nil
	}

	// Strategy 2: Use the /direct/new/ compose flow.
	return b.sendViaComposeFlow(page, username, message)
}

// sendViaProfileButton navigates to the user's profile and clicks the
// "Message" button to open a DM thread. This only works if the "Message"
// button is visible (i.e. you follow the user or they accept DMs from everyone).
func (b *InstagramBot) sendViaProfileButton(page *rod.Page, username, message string) error {
	profileURL := fmt.Sprintf("https://www.instagram.com/%s/", url.PathEscape(username))
	err := rod.Try(func() {
		page.MustNavigate(profileURL).MustWaitLoad()
	})
	if err != nil {
		return fmt.Errorf("navigate to profile: %w", err)
	}
	time.Sleep(3 * time.Second)

	// Dismiss any blocking dialogs.
	b.dismissNotificationDialog(page)

	// Look for a "Message" button (exact text, not "Messages").
	msgBtnXPaths := []string{
		"//div[@role='button'][normalize-space(.)='Message']",
		"//button[normalize-space(.)='Message']",
	}

	var msgBtn *rod.Element
	for _, xpath := range msgBtnXPaths {
		tryErr := rod.Try(func() {
			msgBtn = page.Timeout(3 * time.Second).MustElementX(xpath)
		})
		if tryErr == nil && msgBtn != nil {
			break
		}
		msgBtn = nil
	}

	if msgBtn == nil {
		return fmt.Errorf("no Message button on profile (user may not allow DMs or not followed)")
	}

	if err := msgBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("click Message button: %w", err)
	}
	_ = page.WaitLoad()
	time.Sleep(3 * time.Second)

	return b.typeAndSendMessage(page, message)
}

// sendViaComposeFlow uses Instagram's /direct/new/ page to start a new
// conversation by searching for the username.
func (b *InstagramBot) sendViaComposeFlow(page *rod.Page, username, message string) error {
	err := rod.Try(func() {
		page.MustNavigate("https://www.instagram.com/direct/new/").MustWaitLoad()
	})
	if err != nil {
		return fmt.Errorf("instagram: failed to navigate to new DM page: %w", err)
	}
	time.Sleep(3 * time.Second)

	// Dismiss any blocking dialogs (e.g. "Turn on Notifications").
	b.dismissNotificationDialog(page)
	time.Sleep(1 * time.Second)

	// Find the search/recipient input field.
	searchSelectors := []string{
		"input[name='searchInput']",
		"input[placeholder='Search']",
		"input[placeholder='Search...']",
		"input[type='text']",
	}

	var searchInput *rod.Element
	for _, sel := range searchSelectors {
		el, findErr := page.Timeout(5 * time.Second).Element(sel)
		if findErr == nil && el != nil {
			searchInput = el
			break
		}
	}
	if searchInput == nil {
		return fmt.Errorf("instagram: could not find recipient search input on /direct/new/")
	}

	// Click to focus the search input.
	if err := searchInput.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("instagram: failed to click search input: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Type the username character-by-character using page.Keyboard (NOT el.Type).
	// el.Type inherits the timeout context from page.Timeout() used during Element()
	// lookup, which would be expired by now. page.Keyboard has no such limitation.
	// Keyboard events trigger React's synthetic event system which powers the search.
	for _, ch := range username {
		if err := page.Keyboard.Type(input.Key(ch)); err != nil {
			return fmt.Errorf("instagram: failed to type character %c: %w", ch, err)
		}
		// Small delay between keystrokes for realism and to let React process.
		time.Sleep(100 * time.Millisecond)
	}
	// Wait for search results to appear (Instagram API call + render).
	time.Sleep(3 * time.Second)

	// Click the search result for the target user.
	// Instagram renders results as div[role='button'][tabindex='0'] containing
	// a span with the username text. The results are NOT inside a dialog or
	// listbox — they render directly on the /direct/new/ page.
	resultXPaths := []string{
		// Primary: find the clickable button ancestor of the username span.
		fmt.Sprintf("//span[contains(text(), '%s')]/ancestor::div[@role='button'][@tabindex='0']", username),
		// Fallback: any role=button containing the username anywhere in text.
		fmt.Sprintf("//div[@role='button'][@tabindex='0'][.//span[contains(text(), '%s')]]", username),
		// Broader: div[role=button] inside a presentation wrapper.
		fmt.Sprintf("//div[@role='presentation']//div[@role='button'][.//span[contains(text(), '%s')]]", username),
	}

	clicked := false
	for _, xpath := range resultXPaths {
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
		// Debug: dump what elements are visible for diagnostics.
		b.dumpSearchResultsDebug(page, username)
		return fmt.Errorf("instagram: could not select user %q from search results", username)
	}
	time.Sleep(2 * time.Second)

	// Click "Next" or "Chat" button to proceed to the message thread.
	nextBtnXPaths := []string{
		"//div[@role='button'][normalize-space(.)='Chat']",
		"//button[normalize-space(.)='Chat']",
		"//div[@role='button'][normalize-space(.)='Next']",
		"//button[normalize-space(.)='Next']",
	}
	for _, xpath := range nextBtnXPaths {
		var nextBtn *rod.Element
		tryErr := rod.Try(func() {
			nextBtn = page.Timeout(3 * time.Second).MustElementX(xpath)
		})
		if tryErr == nil && nextBtn != nil {
			_ = nextBtn.Click(proto.InputMouseButtonLeft, 1)
			break
		}
	}
	_ = page.WaitLoad()
	time.Sleep(3 * time.Second)

	return b.typeAndSendMessage(page, message)
}

// dumpSearchResultsDebug logs the DOM structure inside the dialog for debugging
// when search results can't be found. Output goes to stderr via fmt.Fprintf.
func (b *InstagramBot) dumpSearchResultsDebug(page *rod.Page, username string) {
	res, err := page.Eval(`() => {
		const dialogs = document.querySelectorAll('div[role="dialog"]');
		let output = "Dialogs found: " + dialogs.length + "\n";
		dialogs.forEach((d, i) => {
			const children = d.querySelectorAll('*');
			output += "Dialog " + i + " has " + children.length + " descendants\n";
			children.forEach(c => {
				const text = (c.textContent || "").trim();
				if (text.length > 0 && text.length < 100) {
					const tag = c.tagName.toLowerCase();
					const role = c.getAttribute('role') || '';
					const type = c.getAttribute('type') || '';
					if (role || type || tag === 'input' || tag === 'button' || tag === 'span') {
						output += "  <" + tag + " role=" + role + " type=" + type + "> " + text.substring(0, 60) + "\n";
					}
				}
			});
		});
		return output;
	}`)
	if err == nil && res != nil {
		fmt.Printf("[DM DEBUG] %s\n", res.Value.Str())
	}
}

// typeAndSendMessage finds the message input field, types the message, and
// sends it (via Enter key or Send button).
func (b *InstagramBot) typeAndSendMessage(page *rod.Page, message string) error {
	inputSelectors := []string{
		"div[aria-label='Message'][role='textbox']",
		"div[contenteditable='true'][role='textbox']",
		"textarea[placeholder='Message...']",
		"textarea[aria-label='Message']",
		"div[aria-label='Message'][contenteditable='true']",
		"div[aria-label='Message…'][role='textbox']",
		"p[contenteditable='true']",
	}
	inputXPaths := []string{
		"//div[@role='textbox'][@contenteditable='true']",
		"//div[contains(@aria-label, 'Message')][@role='textbox']",
		"//div[contains(@aria-label, 'Message')][@contenteditable='true']",
		"//textarea[contains(@placeholder, 'Message')]",
	}

	var msgInput *rod.Element
	for _, sel := range inputSelectors {
		el, findErr := page.Timeout(5 * time.Second).Element(sel)
		if findErr == nil && el != nil {
			msgInput = el
			break
		}
	}
	if msgInput == nil {
		for _, xpath := range inputXPaths {
			tryErr := rod.Try(func() {
				msgInput = page.Timeout(3 * time.Second).MustElementX(xpath)
			})
			if tryErr == nil && msgInput != nil {
				break
			}
			msgInput = nil
		}
	}
	if msgInput == nil {
		return fmt.Errorf("instagram: could not find message input field")
	}

	// Focus and type the message.
	if err := msgInput.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("instagram: failed to focus message input: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	if err := msgInput.Input(message); err != nil {
		return fmt.Errorf("instagram: failed to type message: %w", err)
	}
	// Dispatch input event for React state sync.
	_, _ = msgInput.Eval(`() => {
		this.dispatchEvent(new Event('input', { bubbles: true }));
	}`)
	time.Sleep(500 * time.Millisecond)

	// Press Enter to send.
	if err := page.Keyboard.Press(input.Enter); err != nil {
		// Fallback: try clicking a send button.
		sendBtnXPaths := []string{
			"//div[@role='button'][normalize-space(.)='Send']",
			"//button[normalize-space(.)='Send']",
		}
		sent := false
		for _, xpath := range sendBtnXPaths {
			var sendBtn *rod.Element
			tryErr := rod.Try(func() {
				sendBtn = page.Timeout(3 * time.Second).MustElementX(xpath)
			})
			if tryErr == nil && sendBtn != nil {
				if clickErr := sendBtn.Click(proto.InputMouseButtonLeft, 1); clickErr == nil {
					sent = true
					break
				}
			}
		}
		if !sent {
			return fmt.Errorf("instagram: failed to send message (Enter key failed: %w)", err)
		}
	}

	time.Sleep(2 * time.Second)
	return nil
}

// GetProfileData scrapes the currently loaded Instagram profile page and
// returns structured profile information.
func (b *InstagramBot) GetProfileData(ctx context.Context, p browser.PageInterface) (map[string]interface{}, error) {
	page := p.(*browser.RodPage).UnwrapRodPage()
	data := make(map[string]interface{})

	// Wait for profile content to be present.
	err := page.WaitLoad()
	if err != nil {
		return data, fmt.Errorf("instagram: page did not finish loading: %w", err)
	}
	time.Sleep(2 * time.Second)

	// Extract the username from the current URL as a fallback identifier.
	pageURL := page.MustInfo().URL
	data["username"] = b.ExtractUsername(pageURL)
	data["profile_url"] = pageURL

	// Full name — Instagram typically shows it as a heading or specific element.
	fullNameSelectors := []string{
		"header section span[class*='full']",
		"header section h1",
		"span[class*='_7UhW9']",
	}
	for _, sel := range fullNameSelectors {
		el, findErr := page.Timeout(3 * time.Second).Element(sel)
		if findErr == nil && el != nil {
			text, tErr := el.Text()
			if tErr == nil && strings.TrimSpace(text) != "" {
				data["full_name"] = strings.TrimSpace(text)
				break
			}
		}
	}

	// Bio section.
	bioSelectors := []string{
		"div[class*='biography'] span",
		"header section > div:nth-child(3) span",
		"-bio span",
	}
	for _, sel := range bioSelectors {
		el, findErr := page.Timeout(3 * time.Second).Element(sel)
		if findErr == nil && el != nil {
			text, tErr := el.Text()
			if tErr == nil && strings.TrimSpace(text) != "" {
				data["bio"] = strings.TrimSpace(text)
				break
			}
		}
	}

	// Follower, following, and post counts.
	// Instagram uses <a> or <span> elements inside the header stats section.
	// Typical structure: <li><a><span title="1,234">1,234</span> followers</a></li>
	statSelectors := []string{
		"header section ul li",
		"header ul li",
	}

	for _, sel := range statSelectors {
		elements, findErr := page.Timeout(3 * time.Second).Elements(sel)
		if findErr != nil || len(elements) == 0 {
			continue
		}
		for i, el := range elements {
			text, tErr := el.Text()
			if tErr != nil {
				continue
			}
			text = strings.TrimSpace(text)
			lowerText := strings.ToLower(text)

			switch {
			case i == 0 || strings.Contains(lowerText, "post"):
				data["post_count"] = text
			case i == 1 || strings.Contains(lowerText, "follower"):
				data["follower_count"] = text
			case i == 2 || strings.Contains(lowerText, "following"):
				data["following_count"] = text
			}
		}
		break
	}

	// Profile picture URL.
	imgSelectors := []string{
		"header img[alt*='profile']",
		"header img[data-testid='user-avatar']",
		"header canvas + img",
		"header img",
	}
	for _, sel := range imgSelectors {
		el, findErr := page.Timeout(3 * time.Second).Element(sel)
		if findErr == nil && el != nil {
			src, aErr := el.Attribute("src")
			if aErr == nil && src != nil && *src != "" {
				data["profile_picture_url"] = *src
				break
			}
		}
	}

	// External link / website.
	linkSelectors := []string{
		"header a[rel='me nofollow noopener noreferrer']",
		"header section a[target='_blank']",
	}
	for _, sel := range linkSelectors {
		el, findErr := page.Timeout(2 * time.Second).Element(sel)
		if findErr == nil && el != nil {
			href, aErr := el.Attribute("href")
			if aErr == nil && href != nil && *href != "" {
				data["website"] = *href
				break
			}
			text, tErr := el.Text()
			if tErr == nil && strings.TrimSpace(text) != "" {
				data["website"] = strings.TrimSpace(text)
				break
			}
		}
	}

	// Verified badge.
	verifiedSelectors := []string{
		"svg[aria-label='Verified']",
		"span[title='Verified']",
	}
	data["is_verified"] = false
	for _, sel := range verifiedSelectors {
		has, _, err := page.Has(sel)
		if err == nil && has {
			data["is_verified"] = true
			break
		}
	}

	// Category label (e.g. "Musician/Band", "Public figure").
	categorySelectors := []string{
		"header div[class*='category']",
		"header section div[class*='_9bJtn']",
	}
	for _, sel := range categorySelectors {
		el, findErr := page.Timeout(2 * time.Second).Element(sel)
		if findErr == nil && el != nil {
			text, tErr := el.Text()
			if tErr == nil && strings.TrimSpace(text) != "" {
				data["category"] = strings.TrimSpace(text)
				break
			}
		}
	}

	return data, nil
}

// GetUserInfo fetches Instagram profile data using the browser's own session.
// Strategy 1: Use in-browser fetch() to call Instagram's API from the JS context.
//   This is indistinguishable from Instagram's own frontend making the call —
//   same cookies, same User-Agent, same origin, same CSRF token.
// Strategy 2: Navigate to profile page and extract from embedded page data.
func (b *InstagramBot) GetUserInfo(ctx context.Context, page *rod.Page, username string) (map[string]interface{}, error) {
	if username == "" {
		return nil, fmt.Errorf("instagram: username is required for get_user_info")
	}

	// Strategy 1: In-browser fetch — fast, no navigation needed.
	result, err := b.fetchProfileViaJS(page, username)
	if err == nil && result != nil {
		return result, nil
	}

	// Strategy 2: Navigate to profile page and extract from rendered content.
	profileURL := fmt.Sprintf("https://www.instagram.com/%s/", username)
	if navErr := page.Navigate(profileURL); navErr != nil {
		return nil, fmt.Errorf("instagram: failed to navigate to profile: %w", navErr)
	}
	if loadErr := page.WaitLoad(); loadErr != nil {
		return nil, fmt.Errorf("instagram: profile page did not load: %w", loadErr)
	}
	time.Sleep(2 * time.Second)

	result = b.extractProfileFromPageSource(page, username)
	if result != nil {
		return result, nil
	}

	return nil, fmt.Errorf("instagram: could not capture profile data for %s", username)
}

// fetchProfileViaJS uses the browser's JavaScript context to call Instagram's
// internal API. The fetch() runs inside the page with real session cookies,
// real CSRF token, and the browser's own User-Agent — exactly like Instagram's
// own frontend JavaScript would make the call.
func (b *InstagramBot) fetchProfileViaJS(page *rod.Page, username string) (map[string]interface{}, error) {
	// The JS script:
	// 1. Reads the CSRF token from cookies (Instagram requires it).
	// 2. Calls /api/v1/users/web_profile_info/ as a same-origin relative URL.
	// 3. Includes the x-ig-app-id header that Instagram's frontend always sends.
	// 4. Returns the JSON response as a string.
	script := fmt.Sprintf(`() => {
		const csrfToken = (document.cookie.match(/csrftoken=([^;]+)/) || [])[1] || '';
		return fetch("/api/v1/users/web_profile_info/?username=%s", {
			headers: {
				"x-ig-app-id": "936619743392459",
				"x-requested-with": "XMLHttpRequest",
				"x-csrftoken": csrfToken
			},
			credentials: "include"
		})
		.then(function(r) {
			if (!r.ok) throw new Error("HTTP " + r.status);
			return r.json();
		})
		.then(function(data) { return JSON.stringify(data); })
		.catch(function(err) { return JSON.stringify({error: err.message}); });
	}`, username)

	res, err := page.Timeout(10 * time.Second).Eval(script)
	if err != nil {
		return nil, fmt.Errorf("js eval failed: %w", err)
	}

	jsonStr := res.Value.Str()
	if jsonStr == "" {
		return nil, fmt.Errorf("empty response from JS fetch")
	}

	var raw map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(jsonStr), &raw); jsonErr != nil {
		return nil, fmt.Errorf("json parse error: %w", jsonErr)
	}

	if errMsg, hasErr := raw["error"]; hasErr {
		return nil, fmt.Errorf("fetch error: %v", errMsg)
	}

	user := extractUserFromResponse(raw)
	if user == nil {
		return nil, fmt.Errorf("no user data in API response")
	}

	return buildProfileResult(username, user), nil
}

// extractUserFromResponse navigates the JSON response structure to find the
// user object. Instagram uses different response formats:
//   - web_profile_info: {"data": {"user": {...}}}
//   - graphql: {"data": {"user": {...}}} or {"graphql": {"user": {...}}}
func extractUserFromResponse(raw map[string]interface{}) map[string]interface{} {
	// Try data.user (web_profile_info and modern graphql).
	if data, ok := raw["data"].(map[string]interface{}); ok {
		if user, ok := data["user"].(map[string]interface{}); ok {
			// Verify it has expected fields.
			if _, hasID := user["id"]; hasID {
				return user
			}
			if _, hasUsername := user["username"]; hasUsername {
				return user
			}
		}
	}
	// Try graphql.user (older format).
	if gql, ok := raw["graphql"].(map[string]interface{}); ok {
		if user, ok := gql["user"].(map[string]interface{}); ok {
			return user
		}
	}
	return nil
}

// buildProfileResult converts an Instagram user object to our standard format.
func buildProfileResult(username string, user map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{
		"platform":     "INSTAGRAM",
		"url":          fmt.Sprintf("https://www.instagram.com/%s/", username),
		"full_name":    getString(user, "full_name"),
		"introduction": getString(user, "biography"),
		"is_verified":  getBool(user, "is_verified"),
		"image_url":    getString(user, "profile_pic_url"),
		"website":      getString(user, "external_url"),
	}

	// Follower/following counts from edge_ fields.
	if edgeFollowedBy, ok := user["edge_followed_by"].(map[string]interface{}); ok {
		if count, ok := edgeFollowedBy["count"].(float64); ok {
			result["follower_count"] = fmt.Sprintf("%d", int64(count))
		}
	}
	if edgeFollow, ok := user["edge_follow"].(map[string]interface{}); ok {
		if count, ok := edgeFollow["count"].(float64); ok {
			result["following_count"] = fmt.Sprintf("%d", int64(count))
		}
	}
	// Post count from edge_owner_to_timeline_media.
	if edgePosts, ok := user["edge_owner_to_timeline_media"].(map[string]interface{}); ok {
		if count, ok := edgePosts["count"].(float64); ok {
			result["content_count"] = fmt.Sprintf("%d", int64(count))
		}
	}

	return result
}

// extractProfileFromPageSource is a fallback that parses the embedded JSON
// from the page source when network interception doesn't capture the API call
// (e.g., when data was server-rendered into the initial HTML).
func (b *InstagramBot) extractProfileFromPageSource(page *rod.Page, username string) map[string]interface{} {
	// Instagram often embeds profile data in script tags as JSON.
	var scripts []string
	rod.Try(func() {
		elements := page.MustElements("script[type='application/ld+json']")
		for _, el := range elements {
			scripts = append(scripts, el.MustText())
		}
	})

	for _, script := range scripts {
		var ld map[string]interface{}
		if json.Unmarshal([]byte(script), &ld) != nil {
			continue
		}
		// Check for name and other profile indicators.
		if name, ok := ld["name"].(string); ok && name != "" {
			result := map[string]interface{}{
				"platform":  "INSTAGRAM",
				"url":       fmt.Sprintf("https://www.instagram.com/%s/", username),
				"full_name": name,
			}
			if desc, ok := ld["description"].(string); ok {
				result["introduction"] = desc
			}
			return result
		}
	}

	// Try to extract from __initialData or shared_data script.
	var pageHTML string
	rod.Try(func() {
		el := page.MustElement("html")
		pageHTML = el.MustHTML()
	})
	if pageHTML != "" {
		// Look for window._sharedData or window.__additionalDataLoaded patterns.
		for _, marker := range []string{
			`"user":{"id":"`,
			`"username":"` + username + `"`,
		} {
			idx := strings.Index(pageHTML, marker)
			if idx < 0 {
				continue
			}
			// Found embedded data — try to extract the surrounding JSON object.
			// Find the opening brace before the marker.
			start := strings.LastIndex(pageHTML[:idx], `{"user"`)
			if start < 0 {
				start = strings.LastIndex(pageHTML[:idx], `"user":{`)
				if start > 0 {
					start-- // include the opening brace of parent
				}
			}
			if start >= 0 {
				// Try to parse a JSON object starting from here.
				remaining := pageHTML[start:]
				depth := 0
				end := -1
				for i, ch := range remaining {
					if ch == '{' {
						depth++
					} else if ch == '}' {
						depth--
						if depth == 0 {
							end = i + 1
							break
						}
					}
				}
				if end > 0 {
					var embedded map[string]interface{}
					if json.Unmarshal([]byte(remaining[:end]), &embedded) == nil {
						if user := extractUserFromResponse(embedded); user != nil {
							return buildProfileResult(username, user)
						}
						// Maybe the root is the user object itself.
						if _, hasID := embedded["id"]; hasID {
							return buildProfileResult(username, embedded)
						}
					}
				}
			}
		}
	}

	return nil
}

// LikePost navigates to a post URL and clicks the Like button. It checks
// whether the post is already liked (to avoid toggling it off) and verifies
// the like was applied by checking for the Unlike indicator afterwards.
//
// The main challenge is distinguishing the POST's like button from COMMENT
// like buttons. Instagram renders both as svg[aria-label='Like']. The post's
// like button lives in the action bar section alongside Comment, Share, and
// Save icons. We use JavaScript to find the correct section first, then
// locate the Like/Unlike button within it.
func (b *InstagramBot) LikePost(ctx context.Context, page *rod.Page, postURL string) error {
	if postURL == "" {
		return fmt.Errorf("instagram: post URL is required")
	}

	// Navigate to the post.
	err := rod.Try(func() {
		page.MustNavigate(postURL).MustWaitLoad()
	})
	if err != nil {
		return fmt.Errorf("instagram: failed to navigate to post %s: %w", postURL, err)
	}
	time.Sleep(3 * time.Second)

	// Dismiss any blocking dialogs.
	b.dismissNotificationDialog(page)

	// Instagram's post page DOM (observed 2026):
	//   - No <article> element on individual post pages.
	//   - The post action bar (Like, Comment, Share, Save) is inside a <section>.
	//   - Comment like buttons are NOT inside any <section> — they're standalone
	//     div[role=button] > svg elements.
	//
	// Strategy: use JS to find the correct <section> (the one with both
	// Like/Unlike AND Comment/Share icons), then mark the like button with a
	// temporary data attribute. Rod then locates that marked element and clicks
	// it with a real CDP mouse event — JS .click() does NOT trigger Instagram's
	// React event handlers.

	// Step 1: Use JS to identify the post action bar and mark the like button.
	// Returns: "already_liked" | "marked" | "not_found:<reason>"
	//
	// Instagram nests sections: the outer section wraps comments + action bar,
	// so it inherits comment like icons. We want the INNERMOST section that has
	// Like/Unlike + Comment/Share — that is the post's own action bar.
	res, err := page.Timeout(10 * time.Second).Eval(`() => {
		// Clean up any previous marker.
		const prev = document.querySelector('[data-monoes-post-like]');
		if (prev) prev.removeAttribute('data-monoes-post-like');

		const sections = document.querySelectorAll('section');
		if (sections.length === 0) return 'not_found:no_sections';

		// Collect ALL matching sections, then pick the innermost (last) one.
		// The outermost section wraps comments too (inheriting their Like/Unlike).
		let actionSection = null;
		for (const section of sections) {
			const hasLikeOrUnlike = section.querySelector(
				'svg[aria-label="Like"], svg[aria-label="Unlike"], ' +
				'span[aria-label="Like"], span[aria-label="Unlike"]'
			);
			const hasCommentOrShare = section.querySelector(
				'svg[aria-label="Comment"], svg[aria-label="Share Post"], ' +
				'svg[aria-label="Share"], span[aria-label="Comment"]'
			);
			if (hasLikeOrUnlike && hasCommentOrShare) {
				// Keep overwriting — last match is the innermost.
				actionSection = section;
			}
		}

		if (!actionSection) return 'not_found:no_action_section';

		// Check if already liked (Unlike icon that is a DIRECT child of this
		// section's subtree, not from a nested section).
		const unlikeIcon = actionSection.querySelector(
			'svg[aria-label="Unlike"], span[aria-label="Unlike"]'
		);
		if (unlikeIcon) return 'already_liked';

		// Find the Like icon and its clickable parent.
		const likeIcon = actionSection.querySelector(
			'svg[aria-label="Like"], span[aria-label="Like"]'
		);
		if (!likeIcon) return 'not_found:no_like_icon';

		const btn = likeIcon.closest('button') || likeIcon.closest('div[role="button"]') || likeIcon.parentElement;
		if (!btn) return 'not_found:no_button';

		// Mark it so rod can find it with a CSS selector.
		btn.setAttribute('data-monoes-post-like', 'true');
		return 'marked';
	}`)
	if err != nil {
		return fmt.Errorf("instagram: failed to evaluate like script on %s: %w", postURL, err)
	}

	state := res.Value.Str()

	if state == "already_liked" {
		return nil
	}
	if state != "marked" {
		return fmt.Errorf("instagram: could not find post Like button on %s (%s)", postURL, state)
	}

	// Step 2: Find the marked element with rod and click with native CDP event.
	likeBtn, err := page.Timeout(5 * time.Second).Element("[data-monoes-post-like='true']")
	if err != nil {
		return fmt.Errorf("instagram: marked like button not found on %s: %w", postURL, err)
	}

	// Scroll into view and small human-like pause.
	likeBtn.MustScrollIntoView()
	time.Sleep(500 * time.Millisecond)

	if err := likeBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("instagram: failed to click Like button: %w", err)
	}

	// Wait for the like to register.
	time.Sleep(2 * time.Second)

	// Step 3: Verify the like succeeded — check for Unlike in the innermost
	// action section (same logic as step 1).
	verifyRes, verifyErr := page.Timeout(5 * time.Second).Eval(`() => {
		const sections = document.querySelectorAll('section');
		let actionSection = null;
		for (const section of sections) {
			const hasLikeOrUnlike = section.querySelector(
				'svg[aria-label="Like"], svg[aria-label="Unlike"], ' +
				'span[aria-label="Like"], span[aria-label="Unlike"]'
			);
			const hasCommentOrShare = section.querySelector(
				'svg[aria-label="Comment"], svg[aria-label="Share Post"], ' +
				'svg[aria-label="Share"], span[aria-label="Comment"]'
			);
			if (hasLikeOrUnlike && hasCommentOrShare) {
				actionSection = section;
			}
		}
		if (!actionSection) return false;
		const unlike = actionSection.querySelector(
			'svg[aria-label="Unlike"], span[aria-label="Unlike"]'
		);
		return !!unlike;
	}`)
	if verifyErr != nil || !verifyRes.Value.Bool() {
		return fmt.Errorf("instagram: like not confirmed on %s", postURL)
	}

	// Clean up the marker attribute.
	page.Eval(`() => {
		const el = document.querySelector('[data-monoes-post-like]');
		if (el) el.removeAttribute('data-monoes-post-like');
	}`)

	return nil
}

// dismissNotificationDialog clicks "Not Now" on Instagram's "Turn on
// Notifications" dialog if it is present. This dialog commonly appears on the
// first visit to the DM pages and blocks interaction with the underlying page.
func (b *InstagramBot) dismissNotificationDialog(page *rod.Page) {
	dismissXPaths := []string{
		"//button[normalize-space(.)='Not Now']",
		"//button[contains(., 'Not Now')]",
		"//div[@role='button'][normalize-space(.)='Not Now']",
	}
	for _, xpath := range dismissXPaths {
		var btn *rod.Element
		tryErr := rod.Try(func() {
			btn = page.Timeout(2 * time.Second).MustElementX(xpath)
		})
		if tryErr == nil && btn != nil {
			_ = btn.Click(proto.InputMouseButtonLeft, 1)
			time.Sleep(1 * time.Second)
			return
		}
	}
}

// getString safely extracts a string value from a map.
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// getBool safely extracts a boolean value from a map.
func getBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// GetMethodByName implements action.BotAdapter — it exposes bot methods that
// can be called from JSON action definitions via the "call_bot_method" step type.
func (b *InstagramBot) GetMethodByName(name string) (func(ctx context.Context, args ...interface{}) (interface{}, error), bool) {
	switch name {
	case "get_user_info":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("get_user_info requires (page, usernameOrURL) args")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("get_user_info: first arg must be *rod.Page")
			}
			input, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("get_user_info: second arg must be string")
			}
			// Accept either a username or a full profile URL.
			username := input
			if strings.Contains(input, "instagram.com") {
				username = b.ExtractUsername(input)
			}
			if username == "" {
				return nil, fmt.Errorf("get_user_info: could not determine username from %q", input)
			}
			return b.GetUserInfo(ctx, page, username)
		}, true

	case "extract_username_from_metadata":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 1 {
				return nil, fmt.Errorf("extract_username_from_metadata requires (page) arg")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("extract_username_from_metadata: first arg must be *rod.Page")
			}
			return b.ExtractUsernameFromMetadata(page)
		}, true

	case "send_message":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 3 {
				return nil, fmt.Errorf("send_message requires (page, usernameOrURL, message) args")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("send_message: first arg must be *rod.Page")
			}
			usernameOrURL, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("send_message: second arg must be string")
			}
			message, ok := args[2].(string)
			if !ok {
				return nil, fmt.Errorf("send_message: third arg must be string")
			}
			// Accept either a username or a full profile URL.
			username := usernameOrURL
			if strings.Contains(usernameOrURL, "instagram.com") {
				username = b.ExtractUsername(usernameOrURL)
			}
			if username == "" {
				return nil, fmt.Errorf("send_message: could not determine username from %q", usernameOrURL)
			}
			err := b.SendMessage(ctx, browser.NewRodPage(page), username, message)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"success":  true,
				"username": username,
			}, nil
		}, true

	case "like_post":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("like_post requires (page, postURL) args")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("like_post: first arg must be *rod.Page")
			}
			postURL, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("like_post: second arg must be string")
			}
			err := b.LikePost(ctx, page, postURL)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"success": true,
				"url":     postURL,
			}, nil
		}, true

	case "comment_post":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 3 {
				return nil, fmt.Errorf("comment_post requires (page, postURL, commentText) args")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("comment_post: first arg must be *rod.Page")
			}
			postURL, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("comment_post: second arg must be string")
			}
			commentText, ok := args[2].(string)
			if !ok {
				return nil, fmt.Errorf("comment_post: third arg must be string")
			}
			err := b.CommentPost(ctx, page, postURL, commentText)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"success": true,
				"url":     postURL,
			}, nil
		}, true

	case "reply_to_conversation":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 3 {
				return nil, fmt.Errorf("reply_to_conversation requires (page, conversationURL, replyText) args")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("reply_to_conversation: first arg must be *rod.Page")
			}
			conversationURL, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("reply_to_conversation: second arg must be string")
			}
			replyText, ok := args[2].(string)
			if !ok {
				return nil, fmt.Errorf("reply_to_conversation: third arg must be string")
			}
			err := b.ReplyToConversation(ctx, page, conversationURL, replyText)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"success": true,
			}, nil
		}, true

	case "fetch_followers_list":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 4 {
				return nil, fmt.Errorf("fetch_followers_list requires (page, profileURL, sourceType, maxCount) args")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("fetch_followers_list: first arg must be *rod.Page")
			}
			profileURL, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("fetch_followers_list: second arg must be string")
			}
			sourceType, ok := args[2].(string)
			if !ok {
				return nil, fmt.Errorf("fetch_followers_list: third arg must be string")
			}
			// maxCount may come as float64 from JSON unmarshaling.
			maxCount := 100
			switch v := args[3].(type) {
			case float64:
				maxCount = int(v)
			case int:
				maxCount = v
			case string:
				fmt.Sscanf(v, "%d", &maxCount)
			}
			users, err := b.FetchFollowersList(ctx, page, profileURL, sourceType, maxCount)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"success": true,
				"users":   users,
				"count":   len(users),
			}, nil
		}, true

	case "interact_with_posts":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 3 {
				return nil, fmt.Errorf("interact_with_posts requires (page, keyword, maxCount) args, optional commentText")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("interact_with_posts: first arg must be *rod.Page")
			}
			keyword, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("interact_with_posts: second arg must be string")
			}
			maxCount := 10
			switch v := args[2].(type) {
			case float64:
				maxCount = int(v)
			case int:
				maxCount = v
			case string:
				fmt.Sscanf(v, "%d", &maxCount)
			}
			commentText := ""
			if len(args) > 3 {
				if ct, ok := args[3].(string); ok {
					commentText = ct
				}
			}
			return b.InteractWithPosts(ctx, page, keyword, maxCount, commentText)
		}, true

	case "interact_with_user_posts":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 3 {
				return nil, fmt.Errorf("interact_with_user_posts requires (page, username, maxCount) args, optional commentText")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("interact_with_user_posts: first arg must be *rod.Page")
			}
			username, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("interact_with_user_posts: second arg must be string")
			}
			maxCount := 10
			switch v := args[2].(type) {
			case float64:
				maxCount = int(v)
			case int:
				maxCount = v
			case string:
				fmt.Sscanf(v, "%d", &maxCount)
			}
			commentText := ""
			if len(args) > 3 {
				if ct, ok := args[3].(string); ok {
					commentText = ct
				}
			}
			return b.InteractWithUserPosts(ctx, page, username, maxCount, commentText)
		}, true

	case "publish_content":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 3 {
				return nil, fmt.Errorf("publish_content requires (page, mediaPath, caption) args, optional locationTag")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("publish_content: first arg must be *rod.Page")
			}
			mediaPath, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("publish_content: second arg must be string")
			}
			caption, ok := args[2].(string)
			if !ok {
				return nil, fmt.Errorf("publish_content: third arg must be string")
			}
			locationTag := ""
			if len(args) > 3 {
				if lt, ok := args[3].(string); ok {
					locationTag = lt
				}
			}
			err := b.PublishContent(ctx, page, mediaPath, caption, locationTag)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"success": true,
			}, nil
		}, true

	case "follow_user":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("follow_user requires (page, profileURL) args")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("follow_user: first arg must be *rod.Page")
			}
			profileURL, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("follow_user: second arg must be string")
			}
			err := b.FollowUser(ctx, page, profileURL)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"success": true,
				"url":     profileURL,
			}, nil
		}, true

	case "unfollow_user":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("unfollow_user requires (page, profileURL) args")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("unfollow_user: first arg must be *rod.Page")
			}
			profileURL, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("unfollow_user: second arg must be string")
			}
			err := b.UnfollowUser(ctx, page, profileURL)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"success": true,
				"url":     profileURL,
			}, nil
		}, true

	case "view_stories":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("view_stories requires (page, profileURL) args")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("view_stories: first arg must be *rod.Page")
			}
			profileURL, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("view_stories: second arg must be string")
			}
			err := b.ViewStories(ctx, page, profileURL)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"success": true,
				"url":     profileURL,
			}, nil
		}, true

	case "scrape_post_data":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("scrape_post_data requires (page, postURL) args")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("scrape_post_data: first arg must be *rod.Page")
			}
			postURL, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("scrape_post_data: second arg must be string")
			}
			return b.ScrapePostData(ctx, page, postURL)
		}, true

	case "like_comment":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 3 {
				return nil, fmt.Errorf("like_comment requires (page, postURL, commentAuthor) args")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("like_comment: first arg must be *rod.Page")
			}
			postURL, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("like_comment: second arg must be string")
			}
			commentAuthor, ok := args[2].(string)
			if !ok {
				return nil, fmt.Errorf("like_comment: third arg must be string")
			}
			err := b.LikeComment(ctx, page, postURL, commentAuthor)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"success": true,
				"url":     postURL,
			}, nil
		}, true

	case "list_user_posts":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 3 {
				return nil, fmt.Errorf("list_user_posts requires (page, username, maxCount) args")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("list_user_posts: first arg must be *rod.Page")
			}
			username, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("list_user_posts: second arg must be string")
			}
			maxCount := 20
			switch v := args[2].(type) {
			case int:
				maxCount = v
			case float64:
				maxCount = int(v)
			}
			posts, err := b.ListUserPosts(ctx, page, username, maxCount)
			if err != nil {
				return nil, err
			}
			result := make([]interface{}, len(posts))
			for i, p := range posts {
				result[i] = p
			}
			return result, nil
		}, true

	case "list_post_comments":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 3 {
				return nil, fmt.Errorf("list_post_comments requires (page, postURL, maxCount) args")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("list_post_comments: first arg must be *rod.Page")
			}
			postURL, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("list_post_comments: second arg must be string")
			}
			maxCount := 50
			switch v := args[2].(type) {
			case int:
				maxCount = v
			case float64:
				maxCount = int(v)
			}
			comments, err := b.ListPostComments(ctx, page, postURL, maxCount)
			if err != nil {
				return nil, err
			}
			result := make([]interface{}, len(comments))
			for i, c := range comments {
				result[i] = c
			}
			return result, nil
		}, true

	case "reply_comment":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 4 {
				return nil, fmt.Errorf("reply_comment requires (page, postURL, commentAuthor, replyText) args")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("reply_comment: first arg must be *rod.Page")
			}
			postURL, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("reply_comment: second arg must be string")
			}
			commentAuthor, ok := args[2].(string)
			if !ok {
				return nil, fmt.Errorf("reply_comment: third arg must be string")
			}
			replyText, ok := args[3].(string)
			if !ok {
				return nil, fmt.Errorf("reply_comment: fourth arg must be string")
			}
			err := b.ReplyToComment(ctx, page, postURL, commentAuthor, replyText)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"success": true,
				"url":     postURL,
			}, nil
		}, true

	default:
		return nil, false
	}
}

// ExtractUsernameFromMetadata attempts to extract the Instagram username from
// the page's JSON-LD or Open Graph metadata.
func (b *InstagramBot) ExtractUsernameFromMetadata(page *rod.Page) (string, error) {
	// Try JSON-LD structured data.
	var jsonLD string
	err := rod.Try(func() {
		el := page.Timeout(3 * time.Second).MustElement("script[type='application/ld+json']")
		jsonLD = el.MustText()
	})
	if err == nil && jsonLD != "" {
		var ld map[string]interface{}
		if json.Unmarshal([]byte(jsonLD), &ld) == nil {
			if alt, ok := ld["alternateName"].(string); ok && strings.HasPrefix(alt, "@") {
				return strings.TrimPrefix(alt, "@"), nil
			}
			if mainEntity, ok := ld["mainEntityofPage"].(map[string]interface{}); ok {
				if id, ok := mainEntity["@id"].(string); ok {
					username := b.ExtractUsername(id)
					if username != "" {
						return username, nil
					}
				}
			}
		}
	}

	// Try Open Graph meta tag.
	var ogURL string
	err = rod.Try(func() {
		el := page.Timeout(2 * time.Second).MustElement("meta[property='og:url']")
		content, _ := el.Attribute("content")
		if content != nil {
			ogURL = *content
		}
	})
	if err == nil && ogURL != "" {
		username := b.ExtractUsername(ogURL)
		if username != "" {
			return username, nil
		}
	}

	// Fallback: try canonical link.
	var canonical string
	err = rod.Try(func() {
		el := page.Timeout(2 * time.Second).MustElement("link[rel='canonical']")
		href, _ := el.Attribute("href")
		if href != nil {
			canonical = *href
		}
	})
	if err == nil && canonical != "" {
		username := b.ExtractUsername(canonical)
		if username != "" {
			return username, nil
		}
	}

	return "", fmt.Errorf("could not extract username from page metadata")
}
