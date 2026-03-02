package email

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	botpkg "github.com/monoes/monoes-agent/internal/bot"
)

// EmailBot implements botpkg.BotAdapter for email-based outreach via the
// Gmail web interface. Since email is fundamentally different from social
// platforms, several methods provide minimal or stub behaviour while still
// satisfying the BotAdapter contract.
type EmailBot struct{}

func init() {
	botpkg.PlatformRegistry["EMAIL"] = func() botpkg.BotAdapter {
		return &EmailBot{}
	}
}

// Platform returns the canonical platform name.
func (b *EmailBot) Platform() string {
	return "EMAIL"
}

// LoginURL returns the Gmail login page URL as the default email provider.
func (b *EmailBot) LoginURL() string {
	return "https://mail.google.com/"
}

// IsLoggedIn checks whether the user is authenticated on Gmail by looking for
// elements that appear only after successful login.
func (b *EmailBot) IsLoggedIn(page *rod.Page) (bool, error) {
	selectors := []string{
		// Main content area present on all authenticated Gmail views.
		"div[role='main']",
		// Gmail's outer wrapper class.
		"div.AO",
		// Thread list container.
		"div[gh='tl']",
		// Compose button area.
		"div[gh='cm']",
		// Navigation / folder list.
		"div[role='navigation']",
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

	// Check for the Google sign-in form — if present, we are NOT logged in.
	hasLogin, _, err := page.Has("input[type='email']")
	if err != nil {
		return false, fmt.Errorf("email: failed to check login state: %w", err)
	}
	if hasLogin {
		return false, nil
	}

	return false, nil
}

// ResolveURL returns the raw URL unchanged. Email addresses do not need URL
// resolution against a base domain.
func (b *EmailBot) ResolveURL(rawURL string) string {
	return rawURL
}

// ExtractUsername extracts an email address from a mailto: URI or returns the
// input as-is if it already looks like a plain email address.
func (b *EmailBot) ExtractUsername(pageURL string) string {
	if pageURL == "" {
		return ""
	}

	// Handle mailto: URIs.
	if strings.HasPrefix(strings.ToLower(pageURL), "mailto:") {
		parsed, err := url.Parse(pageURL)
		if err != nil {
			// Fallback: strip the "mailto:" prefix manually.
			addr := strings.TrimPrefix(pageURL, "mailto:")
			addr = strings.TrimPrefix(addr, "MAILTO:")
			addr = strings.TrimPrefix(addr, "Mailto:")
			// Remove any query parameters (?subject=...&body=...).
			if idx := strings.IndexByte(addr, '?'); idx >= 0 {
				addr = addr[:idx]
			}
			return strings.TrimSpace(addr)
		}

		// url.Parse treats mailto:user@host as Opaque.
		opaque := parsed.Opaque
		if opaque != "" {
			if idx := strings.IndexByte(opaque, '?'); idx >= 0 {
				opaque = opaque[:idx]
			}
			return strings.TrimSpace(opaque)
		}

		// Some implementations put the address in the path.
		path := strings.Trim(parsed.Path, "/")
		if path != "" {
			if idx := strings.IndexByte(path, '?'); idx >= 0 {
				path = path[:idx]
			}
			return strings.TrimSpace(path)
		}

		return ""
	}

	// If the input contains an @ sign, treat it as an email address.
	if strings.Contains(pageURL, "@") {
		// Strip any surrounding angle brackets: <user@host.com>
		addr := strings.TrimPrefix(pageURL, "<")
		addr = strings.TrimSuffix(addr, ">")
		return strings.TrimSpace(addr)
	}

	return ""
}

// SearchURL returns the Gmail search URL for the given keyword.
func (b *EmailBot) SearchURL(keyword string) string {
	safeKeyword := url.QueryEscape(strings.TrimSpace(keyword))
	return fmt.Sprintf("https://mail.google.com/mail/u/0/#search/%s", safeKeyword)
}

// SendMessage navigates to Gmail's compose interface and sends an email to the
// specified recipient.
func (b *EmailBot) SendMessage(ctx context.Context, page *rod.Page, username, message string) error {
	if username == "" {
		return fmt.Errorf("email: recipient address is required")
	}
	if message == "" {
		return fmt.Errorf("email: message body is required")
	}

	// Validate that the username looks like an email address.
	if !strings.Contains(username, "@") {
		return fmt.Errorf("email: recipient %q does not appear to be a valid email address", username)
	}

	// Navigate to the Gmail compose view with the recipient pre-filled.
	composeURL := fmt.Sprintf("https://mail.google.com/mail/u/0/?view=cm&to=%s", url.QueryEscape(username))
	err := page.Navigate(composeURL)
	if err != nil {
		return fmt.Errorf("email: failed to navigate to compose page: %w", err)
	}

	err = page.WaitLoad()
	if err != nil {
		return fmt.Errorf("email: compose page did not load: %w", err)
	}

	// Allow time for Gmail's JavaScript to render the compose form.
	time.Sleep(3 * time.Second)

	// Fill in the "To" field if not already populated via the URL parameter.
	toSelectors := []string{
		"textarea[name='to']",
		"input[name='to']",
		"div[name='to'] input",
		"input[aria-label='To']",
	}

	for _, sel := range toSelectors {
		el, findErr := page.Timeout(5 * time.Second).Element(sel)
		if findErr == nil && el != nil {
			val, _ := el.Attribute("value")
			if val == nil || *val == "" {
				_ = el.Click(proto.InputMouseButtonLeft, 1)
				time.Sleep(300 * time.Millisecond)
				_ = el.Input(username)
				time.Sleep(500 * time.Millisecond)
			}
			break
		}
	}

	// Fill in a default subject line.
	subjectSelectors := []string{
		"input[name='subjectbox']",
		"input[aria-label='Subject']",
		"input[placeholder='Subject']",
	}

	for _, sel := range subjectSelectors {
		el, findErr := page.Timeout(5 * time.Second).Element(sel)
		if findErr == nil && el != nil {
			err = el.Click(proto.InputMouseButtonLeft, 1)
			if err == nil {
				time.Sleep(300 * time.Millisecond)
				_ = el.Input("Message")
			}
			break
		}
	}

	// Fill in the message body.
	bodySelectors := []string{
		"div[aria-label='Message Body'][contenteditable='true']",
		"div[role='textbox'][contenteditable='true']",
		"div.Am.Al.editable",
		"textarea[name='body']",
	}

	var bodyInput *rod.Element
	for _, sel := range bodySelectors {
		el, findErr := page.Timeout(5 * time.Second).Element(sel)
		if findErr == nil && el != nil {
			bodyInput = el
			break
		}
	}

	if bodyInput == nil {
		return fmt.Errorf("email: could not find message body input field")
	}

	err = bodyInput.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("email: failed to focus message body: %w", err)
	}
	time.Sleep(300 * time.Millisecond)

	err = bodyInput.Input(message)
	if err != nil {
		return fmt.Errorf("email: failed to type message body: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Click the Send button.
	sendSelectors := []string{
		"div[aria-label='Send'][role='button']",
		"div[data-tooltip='Send']",
		"div.T-I.J-J5-Ji[role='button']",
	}

	sent := false
	for _, sel := range sendSelectors {
		sendBtn, sErr := page.Timeout(5 * time.Second).Element(sel)
		if sErr == nil && sendBtn != nil {
			if clickErr := sendBtn.Click(proto.InputMouseButtonLeft, 1); clickErr == nil {
				sent = true
				break
			}
		}
	}

	if !sent {
		return fmt.Errorf("email: could not find or click the Send button")
	}

	time.Sleep(2 * time.Second)
	return nil
}

// GetProfileData returns basic data for the email address. Email does not have
// rich profile pages like social media platforms, so this returns a minimal map
// containing the email address itself.
func (b *EmailBot) GetProfileData(ctx context.Context, page *rod.Page) (map[string]interface{}, error) {
	data := make(map[string]interface{})

	err := page.WaitLoad()
	if err != nil {
		return data, fmt.Errorf("email: page did not finish loading: %w", err)
	}
	time.Sleep(1 * time.Second)

	pageURL := page.MustInfo().URL
	data["profile_url"] = pageURL

	// Try to extract an email address from the current URL or page content.
	email := b.ExtractUsername(pageURL)
	if email != "" {
		data["email"] = email
	}

	// Try to extract the signed-in user's email from the Gmail interface.
	accountSelectors := []string{
		"a[aria-label*='Google Account']",
		"a[href*='SignOutOptions']",
		"header a[aria-label*='@']",
	}
	for _, sel := range accountSelectors {
		el, findErr := page.Timeout(3 * time.Second).Element(sel)
		if findErr == nil && el != nil {
			ariaLabel, aErr := el.Attribute("aria-label")
			if aErr == nil && ariaLabel != nil && strings.Contains(*ariaLabel, "@") {
				data["account_email"] = strings.TrimSpace(*ariaLabel)
				break
			}
		}
	}

	data["platform"] = "EMAIL"

	return data, nil
}
