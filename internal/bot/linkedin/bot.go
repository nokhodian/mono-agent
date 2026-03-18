package linkedin

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	botpkg "github.com/monoes/monoes-agent/internal/bot"
)

// LinkedInBot implements botpkg.BotAdapter for LinkedIn.
type LinkedInBot struct{}

func init() {
	botpkg.PlatformRegistry["LINKEDIN"] = func() botpkg.BotAdapter {
		return &LinkedInBot{}
	}
}

// Platform returns the canonical platform name.
func (b *LinkedInBot) Platform() string {
	return "LINKEDIN"
}

// LoginURL returns the LinkedIn login page URL.
func (b *LinkedInBot) LoginURL() string {
	return "https://www.linkedin.com/login"
}

// IsLoggedIn checks whether the user is authenticated on LinkedIn by looking
// for elements that are only rendered for logged-in users.
func (b *LinkedInBot) IsLoggedIn(page *rod.Page) (bool, error) {
	selectors := []string{
		// Global navigation bar present on all authenticated pages.
		"div.global-nav",
		"nav[aria-label='Primary']",
		// Feed container.
		"div.feed-identity-module",
		// The "Me" profile dropdown in the navbar.
		"div.feed-identity-module__actor-meta",
		"img.global-nav__me-photo",
		// Messaging icon.
		"a[href*='/messaging/']",
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

	// Check for the login form — if present, we are NOT logged in.
	loginSelectors := []string{
		"input#username",
		"form.login__form",
		"input[name='session_key']",
	}
	for _, sel := range loginSelectors {
		has, _, err := page.Has(sel)
		if err != nil {
			continue
		}
		if has {
			return false, nil
		}
	}

	return false, nil
}

// ResolveURL converts a relative LinkedIn URL to an absolute URL. If the URL
// is already absolute it is returned unchanged.
func (b *LinkedInBot) ResolveURL(rawURL string) string {
	if strings.HasPrefix(rawURL, "/") {
		return "https://www.linkedin.com" + rawURL
	}
	return rawURL
}

// ExtractUsername parses a LinkedIn profile URL and returns the username from
// the /in/{username} path segment.
func (b *LinkedInBot) ExtractUsername(pageURL string) string {
	parsed, err := url.Parse(pageURL)
	if err != nil {
		return ""
	}

	trimmed := strings.Trim(parsed.Path, "/")
	if trimmed == "" {
		return ""
	}

	segments := strings.Split(trimmed, "/")

	// LinkedIn profile URLs follow the pattern /in/{username}/
	for i, seg := range segments {
		if seg == "in" && i+1 < len(segments) {
			return strings.TrimSpace(segments[i+1])
		}
	}

	return ""
}

// SearchURL returns the LinkedIn people search URL for the given keyword.
func (b *LinkedInBot) SearchURL(keyword string) string {
	encoded := url.QueryEscape(strings.TrimSpace(keyword))
	return fmt.Sprintf("https://www.linkedin.com/search/results/people/?keywords=%s", encoded)
}

// SendMessage navigates to the LinkedIn messaging interface and sends a direct
// message to the specified user.
func (b *LinkedInBot) SendMessage(ctx context.Context, page *rod.Page, username, message string) error {
	if username == "" {
		return fmt.Errorf("linkedin: username is required")
	}
	if message == "" {
		return fmt.Errorf("linkedin: message is required")
	}

	// Navigate to the user's profile first.
	profileURL := fmt.Sprintf("https://www.linkedin.com/in/%s/", url.PathEscape(username))
	err := page.Navigate(profileURL)
	if err != nil {
		return fmt.Errorf("linkedin: failed to navigate to profile: %w", err)
	}
	err = page.WaitLoad()
	if err != nil {
		return fmt.Errorf("linkedin: profile page did not load: %w", err)
	}
	time.Sleep(3 * time.Second)

	// Look for and click the "Message" button on the profile.
	msgBtnSelectors := []string{
		"button.message-anywhere-button",
		"a.message-anywhere-button",
		"button:has-text('Message')",
		"button[aria-label*='Message']",
		"div.pvs-profile-actions button:has-text('Message')",
	}

	clicked := false
	for _, sel := range msgBtnSelectors {
		btn, findErr := page.Timeout(5 * time.Second).Element(sel)
		if findErr == nil && btn != nil {
			if clickErr := btn.Click(proto.InputMouseButtonLeft, 1); clickErr == nil {
				clicked = true
				break
			}
		}
	}

	if !clicked {
		// Fallback: navigate directly to messaging with the user.
		msgURL := fmt.Sprintf("https://www.linkedin.com/messaging/compose/?recipient=%s", url.QueryEscape(username))
		err = page.Navigate(msgURL)
		if err != nil {
			return fmt.Errorf("linkedin: failed to navigate to messaging compose: %w", err)
		}
		err = page.WaitLoad()
		if err != nil {
			return fmt.Errorf("linkedin: messaging compose page did not load: %w", err)
		}
	}

	time.Sleep(3 * time.Second)

	// Find the message input field.
	inputSelectors := []string{
		"div.msg-form__contenteditable[contenteditable='true']",
		"div[role='textbox'][contenteditable='true']",
		"div.msg-form__msg-content-container div[contenteditable='true']",
		"form.msg-form div[contenteditable='true']",
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
		return fmt.Errorf("linkedin: could not find message input field")
	}

	// Focus and type the message.
	err = msgInput.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("linkedin: failed to focus message input: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	err = msgInput.Input(message)
	if err != nil {
		return fmt.Errorf("linkedin: failed to type message: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Click the Send button.
	sendBtnSelectors := []string{
		"button.msg-form__send-button",
		"button[type='submit'].msg-form__send-button",
		"button:has-text('Send')",
		"button[aria-label='Send']",
	}

	sent := false
	for _, sel := range sendBtnSelectors {
		sendBtn, sErr := page.Timeout(5 * time.Second).Element(sel)
		if sErr == nil && sendBtn != nil {
			if clickErr := sendBtn.Click(proto.InputMouseButtonLeft, 1); clickErr == nil {
				sent = true
				break
			}
		}
	}

	if !sent {
		// Fallback: press Enter.
		err = page.Keyboard.Press(input.Enter)
		if err != nil {
			return fmt.Errorf("linkedin: failed to send message: %w", err)
		}
	}

	time.Sleep(1 * time.Second)
	return nil
}

// GetProfileData scrapes the currently loaded LinkedIn profile page and
// returns structured profile information.
func (b *LinkedInBot) GetProfileData(ctx context.Context, page *rod.Page) (map[string]interface{}, error) {
	data := make(map[string]interface{})

	err := page.WaitLoad()
	if err != nil {
		return data, fmt.Errorf("linkedin: page did not finish loading: %w", err)
	}
	time.Sleep(3 * time.Second)

	pageURL := page.MustInfo().URL
	data["username"] = b.ExtractUsername(pageURL)
	data["profile_url"] = pageURL

	// Full name.
	nameSelectors := []string{
		"h1.text-heading-xlarge",
		"h1.top-card-layout__title",
		"li.inline.t-24.t-black.t-normal.break-words",
		"div.ph5 h1",
	}
	for _, sel := range nameSelectors {
		el, findErr := page.Timeout(3 * time.Second).Element(sel)
		if findErr == nil && el != nil {
			text, tErr := el.Text()
			if tErr == nil && strings.TrimSpace(text) != "" {
				data["full_name"] = strings.TrimSpace(text)
				break
			}
		}
	}

	// Headline (job title / tagline).
	headlineSelectors := []string{
		"div.text-body-medium.break-words",
		"h2.top-card-layout__headline",
		"div.ph5 div.text-body-medium",
	}
	for _, sel := range headlineSelectors {
		el, findErr := page.Timeout(3 * time.Second).Element(sel)
		if findErr == nil && el != nil {
			text, tErr := el.Text()
			if tErr == nil && strings.TrimSpace(text) != "" {
				data["headline"] = strings.TrimSpace(text)
				break
			}
		}
	}

	// Location.
	locationSelectors := []string{
		"span.text-body-small.inline.t-black--light.break-words",
		"div.pb2.pv-text-details__left-panel span.text-body-small",
		"span.top-card-layout__first-subline",
	}
	for _, sel := range locationSelectors {
		el, findErr := page.Timeout(3 * time.Second).Element(sel)
		if findErr == nil && el != nil {
			text, tErr := el.Text()
			if tErr == nil && strings.TrimSpace(text) != "" {
				data["location"] = strings.TrimSpace(text)
				break
			}
		}
	}

	// Connection count.
	connectionSelectors := []string{
		"span.t-bold:has-text('connections')",
		"li.text-body-small span.t-bold",
		"span.pv-top-card--list-bullet span.t-bold",
	}
	for _, sel := range connectionSelectors {
		el, findErr := page.Timeout(3 * time.Second).Element(sel)
		if findErr == nil && el != nil {
			text, tErr := el.Text()
			if tErr == nil && strings.TrimSpace(text) != "" {
				data["connection_count"] = strings.TrimSpace(text)
				break
			}
		}
	}

	// Follower count.
	followerSelectors := []string{
		"span:has-text('followers')",
		"p.pvs-header-actions__subtitle span",
	}
	for _, sel := range followerSelectors {
		el, findErr := page.Timeout(3 * time.Second).Element(sel)
		if findErr == nil && el != nil {
			text, tErr := el.Text()
			if tErr == nil && strings.TrimSpace(text) != "" {
				data["follower_count"] = strings.TrimSpace(text)
				break
			}
		}
	}

	// About / summary section.
	aboutSelectors := []string{
		"div#about ~ div.display-flex div.inline-show-more-text span[aria-hidden='true']",
		"section.pv-about-section div.inline-show-more-text",
		"div.pv-shared-text-with-see-more span.visually-hidden",
	}
	for _, sel := range aboutSelectors {
		el, findErr := page.Timeout(3 * time.Second).Element(sel)
		if findErr == nil && el != nil {
			text, tErr := el.Text()
			if tErr == nil && strings.TrimSpace(text) != "" {
				data["about"] = strings.TrimSpace(text)
				break
			}
		}
	}

	// Profile picture URL.
	imgSelectors := []string{
		"img.pv-top-card-profile-picture__image",
		"img.profile-photo-edit__preview",
		"div.pv-top-card__photo-wrapper img",
		"img.top-card-layout__entity-image",
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

	// Current company / experience.
	experienceSelectors := []string{
		"div#experience ~ div.pvs-list__outer-container li.artdeco-list__item:first-child",
		"section.pv-experience-section li:first-child",
	}
	for _, sel := range experienceSelectors {
		el, findErr := page.Timeout(3 * time.Second).Element(sel)
		if findErr == nil && el != nil {
			text, tErr := el.Text()
			if tErr == nil && strings.TrimSpace(text) != "" {
				data["current_experience"] = strings.TrimSpace(text)
				break
			}
		}
	}

	// Education.
	educationSelectors := []string{
		"div#education ~ div.pvs-list__outer-container li.artdeco-list__item:first-child",
		"section.pv-education-section li:first-child",
	}
	for _, sel := range educationSelectors {
		el, findErr := page.Timeout(3 * time.Second).Element(sel)
		if findErr == nil && el != nil {
			text, tErr := el.Text()
			if tErr == nil && strings.TrimSpace(text) != "" {
				data["education"] = strings.TrimSpace(text)
				break
			}
		}
	}

	// Website / contact info link.
	websiteSelectors := []string{
		"section.ci-websites a",
		"a[href*='contact-info']",
	}
	for _, sel := range websiteSelectors {
		el, findErr := page.Timeout(2 * time.Second).Element(sel)
		if findErr == nil && el != nil {
			href, aErr := el.Attribute("href")
			if aErr == nil && href != nil && *href != "" {
				data["contact_info_url"] = *href
				break
			}
		}
	}

	return data, nil
}

// GetMethodByName returns a dispatchable wrapper for the named LinkedIn action method.
// This satisfies the action.BotAdapter interface so call_bot_method steps can resolve
// LinkedIn methods at runtime.
func (b *LinkedInBot) GetMethodByName(name string) (func(ctx context.Context, args ...interface{}) (interface{}, error), bool) {
	switch name {
	case "list_user_posts":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 4 {
				return nil, fmt.Errorf("list_user_posts requires (page, profileURL, maxCount, activityType)")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("list_user_posts: first arg must be *rod.Page")
			}
			profileURL, _ := args[1].(string)
			maxCount := 20
			if v, ok := args[2].(float64); ok {
				maxCount = int(v)
			}
			activityType, _ := args[3].(string)
			if activityType == "" {
				activityType = "all"
			}
			return b.ListUserPosts(ctx, page, profileURL, maxCount, activityType)
		}, true

	case "list_post_comments":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 4 {
				return nil, fmt.Errorf("list_post_comments requires (page, postURL, maxCount, includeReplies)")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("list_post_comments: first arg must be *rod.Page")
			}
			postURL, _ := args[1].(string)
			maxCount := 50
			if v, ok := args[2].(float64); ok {
				maxCount = int(v)
			}
			includeReplies := true
			if v, ok := args[3].(bool); ok {
				includeReplies = v
			}
			return b.ListPostComments(ctx, page, postURL, maxCount, includeReplies)
		}, true

	case "like_post":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 3 {
				return nil, fmt.Errorf("like_post requires (page, postURL, reaction)")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("like_post: first arg must be *rod.Page")
			}
			postURL, _ := args[1].(string)
			reaction, _ := args[2].(string)
			if reaction == "" {
				reaction = "like"
			}
			if err := b.LikePost(ctx, page, postURL, reaction); err != nil {
				return nil, err
			}
			return map[string]interface{}{"success": true, "postURL": postURL, "reaction": reaction}, nil
		}, true

	case "comment_on_post":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 4 {
				return nil, fmt.Errorf("comment_on_post requires (page, postURL, commentText, parentCommentID)")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("comment_on_post: first arg must be *rod.Page")
			}
			postURL, _ := args[1].(string)
			commentText, _ := args[2].(string)
			parentCommentID, _ := args[3].(string)
			if err := b.CommentOnPost(ctx, page, postURL, commentText, parentCommentID); err != nil {
				return nil, err
			}
			return map[string]interface{}{"success": true, "postURL": postURL}, nil
		}, true

	case "like_comment":
		return func(ctx context.Context, args ...interface{}) (interface{}, error) {
			if len(args) < 3 {
				return nil, fmt.Errorf("like_comment requires (page, postURL, commentID)")
			}
			page, ok := args[0].(*rod.Page)
			if !ok {
				return nil, fmt.Errorf("like_comment: first arg must be *rod.Page")
			}
			postURL, _ := args[1].(string)
			commentID, _ := args[2].(string)
			if err := b.LikeComment(ctx, page, postURL, commentID); err != nil {
				return nil, err
			}
			return map[string]interface{}{"success": true, "postURL": postURL, "commentID": commentID}, nil
		}, true
	}
	return nil, false
}
