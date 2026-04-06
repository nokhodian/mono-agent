package gemini

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nokhodian/mono-agent/internal/browser"
	"github.com/nokhodian/mono-agent/internal/extension"

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
func (b *GeminiBot) IsLoggedIn(page browser.PageInterface) (bool, error) {
	// Check for sign-in button — if present, NOT logged in.
	signInSelectors := []string{
		"a[href*='accounts.google.com/ServiceLogin']",
		"a[href*='accounts.google.com/signin']",
		"button[data-signin]",
	}
	for _, sel := range signInSelectors {
		has, err := page.Has(sel)
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
		has, err := page.Has(sel)
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
func (b *GeminiBot) SendMessage(_ context.Context, _ browser.PageInterface, _, _ string) error {
	return fmt.Errorf("gemini: SendMessage not supported")
}

// GetProfileData is not supported for Gemini.
func (b *GeminiBot) GetProfileData(_ context.Context, _ browser.PageInterface) (map[string]interface{}, error) {
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
	case "extract_and_download_images":
		return b.methodExtractAndDownloadImages, true
	default:
		return nil, false
	}
}

// ---------------------------------------------------------------------------
// Bot methods — all use browser.PageInterface for extension compatibility
// ---------------------------------------------------------------------------

// extractPage gets browser.PageInterface from args[0], accepting both
// PageInterface directly and *rod.Page (wrapped automatically).
func extractPage(args []interface{}, methodName string) (browser.PageInterface, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("%s: requires (page)", methodName)
	}
	if p, ok := args[0].(browser.PageInterface); ok {
		return p, nil
	}
	return nil, fmt.Errorf("%s: first arg must be browser.PageInterface, got %T", methodName, args[0])
}

func (b *GeminiBot) methodFindPromptInput(_ context.Context, args ...interface{}) (interface{}, error) {
	page, err := extractPage(args, "find_prompt_input")
	if err != nil {
		return nil, err
	}
	selectors := []string{
		"div.ql-editor[contenteditable='true']",
		"[role='textbox'][aria-label*='prompt' i]",
		"rich-textarea .ql-editor",
		"rich-textarea",
		"[contenteditable='true'][data-placeholder]",
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		for _, sel := range selectors {
			el, err := page.Element(sel, 2*time.Second)
			if err == nil && el != nil {
				return map[string]interface{}{"success": true, "selector": sel}, nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("find_prompt_input: could not find prompt input after 20s")
}

func (b *GeminiBot) methodTypePrompt(_ context.Context, args ...interface{}) (interface{}, error) {
	page, err := extractPage(args, "type_prompt")
	if err != nil {
		return nil, err
	}
	if len(args) < 2 {
		return nil, fmt.Errorf("type_prompt: requires (page, promptText)")
	}
	prompt, ok := args[1].(string)
	if !ok {
		return nil, fmt.Errorf("type_prompt: second arg must be string")
	}

	selectors := []string{
		"div.ql-editor[contenteditable='true']",
		"[role='textbox'][aria-label*='prompt' i]",
		"rich-textarea .ql-editor",
	}
	for _, sel := range selectors {
		el, err := page.Element(sel, 5*time.Second)
		if err == nil && el != nil {
			_ = el.Click()
			time.Sleep(300 * time.Millisecond)

			// Use InsertText for contenteditable (works with Quill/Gemini).
			// Falls back to per-character KeyboardType for regular inputs.
			err := page.InsertText(prompt)
			if err != nil {
				// Fallback: type character by character
				for _, ch := range prompt {
					_ = page.KeyboardType(ch)
					time.Sleep(30 * time.Millisecond)
				}
			}
			return map[string]interface{}{"success": true, "typed": len(prompt)}, nil
		}
	}
	return nil, fmt.Errorf("type_prompt: could not find input to type into")
}

func (b *GeminiBot) methodClickSend(_ context.Context, args ...interface{}) (interface{}, error) {
	page, err := extractPage(args, "click_send")
	if err != nil {
		return nil, err
	}
	selectors := []string{
		"button.send-button",
		"button[aria-label='Send message']",
		"button[aria-label*='Send' i]",
		"button[data-testid='send-button']",
		".send-button-container button",
	}
	for _, sel := range selectors {
		el, err := page.Element(sel, 5*time.Second)
		if err == nil && el != nil {
			_ = el.Click()
			return map[string]interface{}{"success": true}, nil
		}
	}
	// Fallback: press Enter.
	_ = page.KeyboardPress('\n')
	return map[string]interface{}{"success": true, "method": "enter_key"}, nil
}

func (b *GeminiBot) methodWaitForResponse(_ context.Context, args ...interface{}) (interface{}, error) {
	page, err := extractPage(args, "wait_for_response")
	if err != nil {
		return nil, err
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

	// Use DOM queries to count existing responses and detect new ones with stable text.
	// Works with both ExtensionPage (content script queries) and RodPage (Eval).
	prevText := ""
	stableCount := 0
	beforeCount := 0

	// Get initial count.
	if ep, ok := page.(*extension.ExtensionPage); ok {
		beforeCount, _ = ep.QueryCount("message-content")
	} else {
		initResult, err := page.Eval(`() => document.querySelectorAll('message-content').length`)
		if err == nil && !initResult.Nil() {
			beforeCount = initResult.Int()
		}
	}

	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)

		var ready bool
		var text string

		if ep, ok := page.(*extension.ExtensionPage); ok {
			// Extension path: use content script DOM queries
			count, _ := ep.QueryCount("message-content")
			if count <= beforeCount {
				continue
			}
			// Get last message-content text (index -1 = last)
			text, _ = ep.QueryText("message-content")
			ready = text != ""
		} else {
			// Rod path: use Eval
			result, err := page.Eval(`(beforeCount) => {
				const els = document.querySelectorAll('message-content');
				if (els.length <= beforeCount) return {ready: false, text: ''};
				const last = els[els.length - 1];
				return {ready: true, text: (last.textContent || '').trim()};
			}`, beforeCount)
			if err != nil {
				continue
			}
			ready = result.Get("ready").Bool()
			text = result.Get("text").Str()
		}
		if !ready || text == "" {
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
	page, err := extractPage(args, "wait_for_image_response")
	if err != nil {
		return nil, err
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

	if ep, ok := page.(*extension.ExtensionPage); ok {
		// Extension path: use content script queries
		prevText := ""
		stableTextCount := 0
		for time.Now().Before(deadline) {
			time.Sleep(3 * time.Second)
			// Check for images via content script
			imgCount, _ := ep.QueryCount("model-response img, message-content img, .response-container img")
			if imgCount > 0 {
				time.Sleep(3 * time.Second)
				return map[string]interface{}{"success": true, "ready": true}, nil
			}
			// Check for text-only refusal
			text, _ := ep.QueryText("message-content")
			if text != "" && text == prevText {
				stableTextCount++
				lower := strings.ToLower(text)
				if strings.Contains(lower, "can't create") || strings.Contains(lower, "image creation isn't available") {
					return nil, fmt.Errorf("wait_for_image_response: Gemini refused: %s", text[:min(len(text), 200)])
				}
				if stableTextCount >= 5 {
					return nil, fmt.Errorf("wait_for_image_response: text-only response (no images): %s", text[:min(len(text), 200)])
				}
			} else {
				stableTextCount = 0
			}
			prevText = text
		}
		return nil, fmt.Errorf("wait_for_image_response: timed out after %ds", maxWait)
	}

	// Rod path: use Eval
	for time.Now().Before(deadline) {
		time.Sleep(3 * time.Second)
		result, err := page.Eval(`() => {
			const containers = document.querySelectorAll('model-response, message-content, .response-container');
			if (containers.length === 0) return {found: false, text: ''};
			const last = containers[containers.length - 1];
			const imgs = last.querySelectorAll('img');
			let valid = 0;
			for (const img of imgs) {
				const w = img.width || img.naturalWidth || 0;
				if (w > 0 && w < 48) continue;
				const src = img.src || '';
				if (src.startsWith('blob:') || src.startsWith('data:image') || (src.startsWith('https://') && w >= 100)) valid++;
			}
			return {found: valid > 0, text: (last.textContent || '').trim().substring(0, 300)};
		}`)
		if err != nil {
			continue
		}
		found := result.Get("found").Bool()
		text := result.Get("text").Str()
		if found {
			time.Sleep(3 * time.Second)
			return map[string]interface{}{"success": true, "ready": true}, nil
		}
		if text != "" {
			lower := strings.ToLower(text)
			if strings.Contains(lower, "can't create") || strings.Contains(lower, "image creation isn't available") {
				return nil, fmt.Errorf("wait_for_image_response: Gemini refused: %s", text[:min(len(text), 200)])
			}
		}
	}
	return nil, fmt.Errorf("wait_for_image_response: timed out after %ds", maxWait)
}

func (b *GeminiBot) methodExtractTextResponse(_ context.Context, args ...interface{}) (interface{}, error) {
	page, err := extractPage(args, "extract_text_response")
	if err != nil {
		return nil, err
	}

	selectors := []string{
		"message-content div.markdown.markdown-main-panel",
		"message-content",
		"structured-content-container.model-response-text",
	}

	if ep, ok := page.(*extension.ExtensionPage); ok {
		// Extension path: use content script DOM queries
		for _, sel := range selectors {
			text, err := ep.QueryText(sel)
			if err == nil && text != "" {
				return map[string]interface{}{"success": true, "response_text": text}, nil
			}
		}
		return nil, fmt.Errorf("extract_text_response: no response text found")
	}

	// Rod path: use Eval
	result, err := page.Eval(`() => {
		const sels = ['message-content div.markdown.markdown-main-panel', 'message-content', 'structured-content-container.model-response-text'];
		for (const sel of sels) {
			const els = document.querySelectorAll(sel);
			if (els.length === 0) continue;
			const text = (els[els.length - 1].textContent || '').trim();
			if (text) return text;
		}
		return '';
	}`)
	if err != nil {
		return nil, fmt.Errorf("extract_text_response: eval failed: %w", err)
	}
	text := result.Str()
	if text == "" {
		return nil, fmt.Errorf("extract_text_response: no response text found")
	}
	return map[string]interface{}{"success": true, "response_text": text}, nil
}

func (b *GeminiBot) methodExtractImageResponse(_ context.Context, args ...interface{}) (interface{}, error) {
	page, err := extractPage(args, "extract_image_response")
	if err != nil {
		return nil, err
	}
	result, err := page.Eval(`() => {
		const containers = document.querySelectorAll('model-response, message-content, .response-container');
		if (containers.length === 0) return [];
		const last = containers[containers.length - 1];
		const urls = [];
		for (const img of last.querySelectorAll('img')) {
			const src = img.src || '';
			const w = img.width || img.naturalWidth || 0;
			if (w > 0 && w < 48) continue;
			if (src.startsWith('blob:') || src.startsWith('data:image') || (src.startsWith('https://') && w >= 100)) urls.push(src);
		}
		return urls;
	}`)
	if err != nil {
		return nil, fmt.Errorf("extract_image_response: eval failed: %w", err)
	}
	var urls []string
	for _, item := range result.Arr() {
		if s := item.Str(); s != "" {
			urls = append(urls, s)
		}
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("extract_image_response: no images found")
	}
	return map[string]interface{}{"success": true, "image_urls": urls}, nil
}

func (b *GeminiBot) methodDownloadImages(_ context.Context, args ...interface{}) (interface{}, error) {
	page, err := extractPage(args, "download_images")
	if err != nil {
		return nil, err
	}
	if len(args) < 2 {
		return nil, fmt.Errorf("download_images: requires (page, imageUrls)")
	}

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
			if strings.HasPrefix(dir, "~/") {
				dir = filepath.Join(os.Getenv("HOME"), dir[2:])
			}
			downloadDir = dir
		}
	}
	_ = os.MkdirAll(downloadDir, 0700)

	timestamp := time.Now().Unix()
	var downloaded []map[string]interface{}

	for i, imgURL := range imageURLs {
		filename := fmt.Sprintf("gemini_%d_%d.png", timestamp, i)
		filePath := filepath.Join(downloadDir, filename)

		var b64str string
		if strings.HasPrefix(imgURL, "data:image") {
			parts := strings.SplitN(imgURL, ",", 2)
			if len(parts) == 2 {
				b64str = parts[1]
			}
		} else {
			result, err := page.Eval(`(url) => {
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
			b64str = result.Str()
		}

		if b64str == "" {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(b64str)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(b64str)
			if err != nil {
				continue
			}
		}
		if len(decoded) < 100 {
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

	// Create latest symlink.
	if first, ok := downloaded[0]["path"].(string); ok {
		latestLink := filepath.Join(downloadDir, "latest_gemini.png")
		_ = os.Remove(latestLink)
		_ = os.Symlink(first, latestLink)
	}

	return map[string]interface{}{
		"success":     true,
		"images":      downloaded,
		"image_count": len(downloaded),
	}, nil
}

// methodExtractAndDownloadImages combines image extraction + download in one step.
// Uses the content script FetchImageBase64 on extension path, Eval on Rod path.
func (b *GeminiBot) methodExtractAndDownloadImages(_ context.Context, args ...interface{}) (interface{}, error) {
	page, err := extractPage(args, "extract_and_download_images")
	if err != nil {
		return nil, err
	}

	downloadDir := filepath.Join(os.Getenv("HOME"), ".monoes", "downloads")
	if len(args) >= 2 {
		if dir, ok := args[1].(string); ok && dir != "" {
			if strings.HasPrefix(dir, "~/") {
				dir = filepath.Join(os.Getenv("HOME"), dir[2:])
			}
			downloadDir = dir
		}
	}
	_ = os.MkdirAll(downloadDir, 0700)

	timestamp := time.Now().Unix()
	var downloaded []map[string]interface{}

	if ep, ok := page.(*extension.ExtensionPage); ok {
		// Extension path: use content script to fetch images as base64
		imgSelector := "model-response img, message-content img, .response-container img"
		images, err := ep.FetchImageBase64(imgSelector)
		if err != nil || len(images) == 0 {
			return nil, fmt.Errorf("extract_and_download_images: no images found via extension")
		}
		for i, img := range images {
			b64, _ := img["data"].(string)
			if b64 == "" {
				continue
			}
			decoded, err := base64.StdEncoding.DecodeString(b64)
			if err != nil {
				continue
			}
			if len(decoded) < 100 {
				continue
			}
			filename := fmt.Sprintf("gemini_%d_%d.png", timestamp, i)
			filePath := filepath.Join(downloadDir, filename)
			if err := os.WriteFile(filePath, decoded, 0600); err != nil {
				continue
			}
			downloaded = append(downloaded, map[string]interface{}{
				"path":       filePath,
				"filename":   filename,
				"size_bytes": len(decoded),
			})
		}
	} else {
		// Rod path: use Eval to extract and download
		result, err := page.Eval(`() => {
			const containers = document.querySelectorAll('model-response, message-content, .response-container');
			if (containers.length === 0) return [];
			const last = containers[containers.length - 1];
			const imgs = last.querySelectorAll('img');
			const data = [];
			for (const img of imgs) {
				const w = img.naturalWidth || img.width || 0;
				if (w < 48) continue;
				try {
					const canvas = document.createElement('canvas');
					canvas.width = img.naturalWidth;
					canvas.height = img.naturalHeight;
					canvas.getContext('2d').drawImage(img, 0, 0);
					const b64 = canvas.toDataURL('image/png').split(',')[1];
					if (b64 && b64.length > 200) data.push(b64);
				} catch(e) {}
			}
			return data;
		}`)
		if err == nil {
			for i, item := range result.Arr() {
				b64 := item.Str()
				if b64 == "" {
					continue
				}
				decoded, err := base64.StdEncoding.DecodeString(b64)
				if err != nil || len(decoded) < 100 {
					continue
				}
				filename := fmt.Sprintf("gemini_%d_%d.png", timestamp, i)
				filePath := filepath.Join(downloadDir, filename)
				if err := os.WriteFile(filePath, decoded, 0600); err != nil {
					continue
				}
				downloaded = append(downloaded, map[string]interface{}{
					"path":       filePath,
					"filename":   filename,
					"size_bytes": len(decoded),
				})
			}
		}
	}

	if len(downloaded) == 0 {
		return nil, fmt.Errorf("extract_and_download_images: no images downloaded")
	}

	if first, ok := downloaded[0]["path"].(string); ok {
		latestLink := filepath.Join(downloadDir, "latest_gemini.png")
		_ = os.Remove(latestLink)
		_ = os.Symlink(first, latestLink)
	}

	return map[string]interface{}{
		"success":     true,
		"images":      downloaded,
		"image_count": len(downloaded),
	}, nil
}
