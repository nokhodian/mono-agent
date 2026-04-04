package gemini

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	botpkg "github.com/nokhodian/mono-agent/internal/bot"
)

// GeminiBot implements botpkg.BotAdapter for Google Gemini.
type GeminiBot struct{}

func init() {
	botpkg.PlatformRegistry["GEMINI"] = func() botpkg.BotAdapter {
		return &GeminiBot{}
	}
}

// Platform returns the canonical platform name.
func (b *GeminiBot) Platform() string { return "GEMINI" }

// LoginURL returns the Gemini login page URL.
func (b *GeminiBot) LoginURL() string { return "https://gemini.google.com" }

// IsLoggedIn checks whether the user is authenticated on Gemini by looking
// for sign-in buttons (NOT logged in) and prompt input (IS logged in).
func (b *GeminiBot) IsLoggedIn(page *rod.Page) (bool, error) {
	// Check for sign-in button — if present, NOT logged in.
	signInSelectors := []string{
		"a[href*='accounts.google.com/ServiceLogin']",
		"a[href*='accounts.google.com/signin']",
		"button[data-signin]",
	}
	for _, sel := range signInSelectors {
		has, _, err := page.Has(sel)
		if err != nil {
			continue
		}
		if has {
			return false, nil
		}
	}

	// Check for prompt input — only appears when logged in.
	promptSelectors := []string{
		"div.ql-editor[contenteditable='true']",
		"rich-textarea",
		"input-area-v2",
	}
	for _, sel := range promptSelectors {
		has, _, err := page.Has(sel)
		if err != nil {
			continue
		}
		if has {
			return true, nil
		}
	}

	return false, nil
}

// ResolveURL returns the Gemini base URL for any input.
func (b *GeminiBot) ResolveURL(_ string) string {
	return "https://gemini.google.com"
}

// ExtractUsername returns a placeholder — Gemini has no user profiles.
func (b *GeminiBot) ExtractUsername(_ string) string {
	return "gemini-user"
}

// SearchURL is not applicable for Gemini.
func (b *GeminiBot) SearchURL(_ string) string {
	return ""
}

// SendMessage is not supported for Gemini.
func (b *GeminiBot) SendMessage(_ context.Context, _ *rod.Page, _, _ string) error {
	return fmt.Errorf("gemini: SendMessage not supported")
}

// GetProfileData is not supported for Gemini.
func (b *GeminiBot) GetProfileData(_ context.Context, _ *rod.Page) (map[string]interface{}, error) {
	return nil, fmt.Errorf("gemini: GetProfileData not supported")
}

// GetMethodByName returns a dispatchable wrapper for the named Gemini action
// method. The executor calls this to resolve call_bot_method steps.
func (b *GeminiBot) GetMethodByName(name string) (func(ctx context.Context, args ...interface{}) (interface{}, error), bool) {
	switch name {
	case "find_prompt_input":
		return b.methodFindPromptInput, true
	case "type_prompt":
		return b.methodTypePrompt, true
	case "click_send":
		return b.methodClickSend, true
	case "wait_for_response":
		return b.methodWaitForResponse, true
	case "wait_for_image_response":
		return b.methodWaitForImageResponse, true
	case "extract_text_response":
		return b.methodExtractTextResponse, true
	case "extract_image_response":
		return b.methodExtractImageResponse, true
	case "download_images":
		return b.methodDownloadImages, true
	default:
		return nil, false
	}
}

// ---------------------------------------------------------------------------
// Bot methods
// ---------------------------------------------------------------------------

func (b *GeminiBot) methodFindPromptInput(_ context.Context, args ...interface{}) (interface{}, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("find_prompt_input: requires (page)")
	}
	page, ok := args[0].(*rod.Page)
	if !ok {
		return nil, fmt.Errorf("find_prompt_input: first arg must be *rod.Page")
	}
	selectors := []string{
		"div.ql-editor[contenteditable='true']",
		"[role='textbox'][aria-label*='prompt' i]",
		"rich-textarea .ql-editor",
	}
	for _, sel := range selectors {
		el, err := page.Timeout(5 * time.Second).Element(sel)
		if err == nil && el != nil {
			return map[string]interface{}{"success": true, "selector": sel}, nil
		}
	}
	return nil, fmt.Errorf("find_prompt_input: could not find prompt input")
}

func (b *GeminiBot) methodTypePrompt(_ context.Context, args ...interface{}) (interface{}, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("type_prompt: requires (page, promptText)")
	}
	page, ok := args[0].(*rod.Page)
	if !ok {
		return nil, fmt.Errorf("type_prompt: first arg must be *rod.Page")
	}
	prompt, ok := args[1].(string)
	if !ok {
		return nil, fmt.Errorf("type_prompt: second arg must be string")
	}

	// Find and focus the input.
	selectors := []string{
		"div.ql-editor[contenteditable='true']",
		"[role='textbox'][aria-label*='prompt' i]",
		"rich-textarea .ql-editor",
	}
	for _, sel := range selectors {
		el, err := page.Timeout(5 * time.Second).Element(sel)
		if err == nil && el != nil {
			_ = el.Click(proto.InputMouseButtonLeft, 1)
			time.Sleep(300 * time.Millisecond)
			// Type with small delays to mimic human input.
			for _, ch := range prompt {
				_ = page.Keyboard.Type(input.Key(ch))
				time.Sleep(30 * time.Millisecond)
			}
			return map[string]interface{}{"success": true, "typed": len(prompt)}, nil
		}
	}
	return nil, fmt.Errorf("type_prompt: could not find input to type into")
}

func (b *GeminiBot) methodClickSend(_ context.Context, args ...interface{}) (interface{}, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("click_send: requires (page)")
	}
	page, ok := args[0].(*rod.Page)
	if !ok {
		return nil, fmt.Errorf("click_send: first arg must be *rod.Page")
	}
	selectors := []string{
		"button.send-button",
		"button[aria-label='Send message']",
	}
	for _, sel := range selectors {
		el, err := page.Timeout(5 * time.Second).Element(sel)
		if err == nil && el != nil {
			_ = el.Click(proto.InputMouseButtonLeft, 1)
			return map[string]interface{}{"success": true}, nil
		}
	}
	// Fallback: press Enter.
	_ = page.Keyboard.Press(input.Enter)
	return map[string]interface{}{"success": true, "method": "enter_key"}, nil
}

func (b *GeminiBot) methodWaitForResponse(_ context.Context, args ...interface{}) (interface{}, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("wait_for_response: requires (page)")
	}
	page, ok := args[0].(*rod.Page)
	if !ok {
		return nil, fmt.Errorf("wait_for_response: first arg must be *rod.Page")
	}
	maxWait := 60
	if len(args) >= 2 {
		switch v := args[1].(type) {
		case string:
			fmt.Sscanf(v, "%d", &maxWait)
		case float64:
			maxWait = int(v)
		case int:
			maxWait = v
		}
	}

	deadline := time.Now().Add(time.Duration(maxWait) * time.Second)

	// Count existing message-content elements BEFORE this response arrives.
	beforeCount := 0
	if existing, err := page.Elements("message-content"); err == nil {
		beforeCount = len(existing)
	}

	// Wait for a NEW message-content element with stable text (stops changing).
	prevText := ""
	stableCount := 0
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		els, err := page.Elements("message-content")
		if err != nil || len(els) <= beforeCount {
			continue
		}
		last := els[len(els)-1]
		text, _ := last.Text()
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if text == prevText {
			stableCount++
			if stableCount >= 2 {
				return map[string]interface{}{"success": true, "ready": true}, nil
			}
		} else {
			stableCount = 0
		}
		prevText = text
	}
	return nil, fmt.Errorf("wait_for_response: timed out after %ds", maxWait)
}

func (b *GeminiBot) methodWaitForImageResponse(_ context.Context, args ...interface{}) (interface{}, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("wait_for_image_response: requires (page)")
	}
	page, ok := args[0].(*rod.Page)
	if !ok {
		return nil, fmt.Errorf("wait_for_image_response: first arg must be *rod.Page")
	}
	maxWait := 120
	if len(args) >= 2 {
		switch v := args[1].(type) {
		case string:
			fmt.Sscanf(v, "%d", &maxWait)
		case float64:
			maxWait = int(v)
		case int:
			maxWait = v
		}
	}

	deadline := time.Now().Add(time.Duration(maxWait) * time.Second)
	imageSelectors := []string{
		"model-response img:not([width='24'])",
		"message-content img[src*='blob:']",
		".response-container img",
	}

	for time.Now().Before(deadline) {
		for _, sel := range imageSelectors {
			has, _, _ := page.Has(sel)
			if has {
				// Wait a bit for all images to render.
				time.Sleep(3 * time.Second)
				return map[string]interface{}{"success": true, "ready": true}, nil
			}
		}
		time.Sleep(3 * time.Second)
	}
	return nil, fmt.Errorf("wait_for_image_response: timed out after %ds", maxWait)
}

func (b *GeminiBot) methodExtractTextResponse(_ context.Context, args ...interface{}) (interface{}, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("extract_text_response: requires (page)")
	}
	page, ok := args[0].(*rod.Page)
	if !ok {
		return nil, fmt.Errorf("extract_text_response: first arg must be *rod.Page")
	}
	selectors := []string{
		"message-content div.markdown.markdown-main-panel",
		"message-content",
		"structured-content-container.model-response-text",
	}
	for _, sel := range selectors {
		els, err := page.Elements(sel)
		if err != nil || len(els) == 0 {
			continue
		}
		// Get the last response (most recent).
		el := els[len(els)-1]
		text, _ := el.Text()
		text = strings.TrimSpace(text)
		if text != "" {
			return map[string]interface{}{"success": true, "response_text": text}, nil
		}
	}
	return nil, fmt.Errorf("extract_text_response: no response text found")
}

func (b *GeminiBot) methodExtractImageResponse(_ context.Context, args ...interface{}) (interface{}, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("extract_image_response: requires (page)")
	}
	page, ok := args[0].(*rod.Page)
	if !ok {
		return nil, fmt.Errorf("extract_image_response: first arg must be *rod.Page")
	}
	selectors := []string{
		"model-response img:not([width='24']):not([alt=''])",
		"message-content img[src*='blob:']",
		".response-container img:not([width='24'])",
	}
	var urls []string
	for _, sel := range selectors {
		els, err := page.Elements(sel)
		if err != nil || len(els) == 0 {
			continue
		}
		for _, el := range els {
			src, err := el.Attribute("src")
			if err == nil && src != nil && *src != "" {
				urls = append(urls, *src)
			}
		}
		if len(urls) > 0 {
			break
		}
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("extract_image_response: no images found")
	}
	return map[string]interface{}{"success": true, "image_urls": urls}, nil
}

func (b *GeminiBot) methodDownloadImages(_ context.Context, args ...interface{}) (interface{}, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("download_images: requires (page, imageUrls)")
	}
	page, ok := args[0].(*rod.Page)
	if !ok {
		return nil, fmt.Errorf("download_images: first arg must be *rod.Page")
	}

	// Parse image URLs from args.
	var imageURLs []string
	switch v := args[1].(type) {
	case []string:
		imageURLs = v
	case []interface{}:
		for _, u := range v {
			if s, ok := u.(string); ok {
				imageURLs = append(imageURLs, s)
			}
		}
	case map[string]interface{}:
		if urls, ok := v["image_urls"].([]interface{}); ok {
			for _, u := range urls {
				if s, ok := u.(string); ok {
					imageURLs = append(imageURLs, s)
				}
			}
		}
	}

	downloadDir := filepath.Join(os.Getenv("HOME"), ".monoes", "downloads")
	if len(args) >= 3 {
		if dir, ok := args[2].(string); ok && dir != "" {
			downloadDir = dir
		}
	}
	_ = os.MkdirAll(downloadDir, 0700)

	timestamp := time.Now().Unix()
	var downloaded []map[string]interface{}

	for i, imgURL := range imageURLs {
		filename := fmt.Sprintf("gemini_%d_%d.png", timestamp, i)
		filePath := filepath.Join(downloadDir, filename)

		// Use page.Eval to fetch image as base64 (works for both blob: and
		// regular URLs).
		b64, err := page.Eval(`(url) => {
			return fetch(url)
				.then(r => r.blob())
				.then(b => new Promise((resolve, reject) => {
					const reader = new FileReader();
					reader.onloadend = () => resolve(reader.result.split(',')[1]);
					reader.onerror = reject;
					reader.readAsDataURL(b);
				}));
		}`, imgURL)
		if err != nil {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(b64.Value.Str())
		if err != nil {
			continue
		}

		if err := os.WriteFile(filePath, decoded, 0600); err != nil {
			continue
		}

		downloaded = append(downloaded, map[string]interface{}{
			"path":       filePath,
			"filename":   filename,
			"size_bytes": len(decoded),
		})
	}

	if len(downloaded) == 0 {
		return nil, fmt.Errorf("download_images: failed to download any images")
	}

	return map[string]interface{}{
		"success":     true,
		"images":      downloaded,
		"image_count": len(downloaded),
	}, nil
}
