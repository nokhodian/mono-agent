//go:build integration

package action

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

// TestInstagramCommentDebug is a diagnostic test that walks through the
// commenting flow step by step with screenshots to figure out why comments
// don't actually get posted.
func TestInstagramCommentDebug(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	_, page, cleanup := launchTestBrowser(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	_ = ctx

	postURL := "https://www.instagram.com/p/BGKpW0mHnMt/"
	t.Logf("Navigating to %s", postURL)

	err := rod.Try(func() {
		page.MustNavigate(postURL).MustWaitLoad()
	})
	if err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}
	time.Sleep(3 * time.Second)

	// Screenshot 1: Post page loaded.
	saveScreenshot(t, page, "/tmp/comment_debug_1_loaded.png")

	// Step 1: Find comment input.
	t.Log("Step 1: Finding comment input...")
	commentSelectors := []string{
		"textarea[aria-label='Add a comment…']",
		"textarea[placeholder='Add a comment…']",
		"div[aria-label='Add a comment…'][role='textbox']",
		"div[contenteditable='true'][role='textbox']",
		"form textarea",
		"form div[contenteditable='true']",
	}

	var commentInput *rod.Element
	var foundVia string
	for _, sel := range commentSelectors {
		el, findErr := page.Timeout(5 * time.Second).Element(sel)
		if findErr == nil && el != nil {
			commentInput = el
			foundVia = sel
			break
		}
	}

	if commentInput == nil {
		// Check if we need to click a "comment" icon first to reveal the input.
		t.Log("No comment input found directly. Looking for comment icon to click...")
		commentIconXPaths := []string{
			"//svg[@aria-label='Comment']/..",
			"//*[@aria-label='Comment']",
		}
		for _, xpath := range commentIconXPaths {
			var icon *rod.Element
			tryErr := rod.Try(func() {
				icon = page.Timeout(3 * time.Second).MustElementX(xpath)
			})
			if tryErr == nil && icon != nil {
				t.Logf("Found comment icon, clicking...")
				_ = icon.Click(proto.InputMouseButtonLeft, 1)
				time.Sleep(2 * time.Second)
				// Retry finding the input.
				for _, sel := range commentSelectors {
					el, findErr := page.Timeout(3 * time.Second).Element(sel)
					if findErr == nil && el != nil {
						commentInput = el
						foundVia = sel
						break
					}
				}
				break
			}
		}
	}

	if commentInput == nil {
		saveScreenshot(t, page, "/tmp/comment_debug_2_no_input.png")
		t.Fatal("Could not find comment input")
	}
	t.Logf("Found comment input via %q", foundVia)

	// Dump the element's tag, attributes.
	attrs, _ := commentInput.Eval(`() => {
		return this.tagName + ' | contenteditable=' + this.getAttribute('contenteditable') +
			' | role=' + this.getAttribute('role') +
			' | aria-label=' + this.getAttribute('aria-label') +
			' | placeholder=' + this.getAttribute('placeholder')
	}`)
	if attrs != nil {
		t.Logf("Comment input attrs: %s", attrs.Value.Str())
	}

	// Step 2: Click to focus.
	t.Log("Step 2: Clicking comment input to focus...")
	_ = commentInput.Click(proto.InputMouseButtonLeft, 1)
	time.Sleep(1 * time.Second)
	saveScreenshot(t, page, "/tmp/comment_debug_2_focused.png")

	// Step 3: Type comment using page.Keyboard (avoid element timeout issue).
	commentText := fmt.Sprintf("test comment %d", time.Now().Unix())
	t.Logf("Step 3: Typing %q via page.Keyboard...", commentText)
	for _, ch := range commentText {
		if err := page.Keyboard.Type(input.Key(ch)); err != nil {
			t.Logf("WARNING: Keyboard.Type failed for %c: %v", ch, err)
		}
		time.Sleep(80 * time.Millisecond)
	}
	time.Sleep(1 * time.Second)

	// Check what text is in the input now.
	textContent, _ := commentInput.Eval(`() => {
		if (this.tagName === 'TEXTAREA') return this.value;
		return this.textContent || this.innerText || '';
	}`)
	if textContent != nil {
		t.Logf("Text in comment input: %q", textContent.Value.Str())
	}

	saveScreenshot(t, page, "/tmp/comment_debug_3_typed.png")

	// Step 4: Find the Post button.
	t.Log("Step 4: Finding Post button...")
	postBtnXPaths := []string{
		"//div[@role='button'][normalize-space(.)='Post']",
		"//button[normalize-space(.)='Post']",
		"//div[@role='button'][text()='Post']",
		"//button[text()='Post']",
		"//form//div[@role='button']",
		"//form//button[@type='submit']",
	}

	var postBtn *rod.Element
	var btnFoundVia string
	for _, xpath := range postBtnXPaths {
		var el *rod.Element
		tryErr := rod.Try(func() {
			el = page.Timeout(5 * time.Second).MustElementX(xpath)
		})
		if tryErr == nil && el != nil {
			text, _ := el.Text()
			t.Logf("Found Post button via %q, text=%q", xpath, text)
			postBtn = el
			btnFoundVia = xpath
			break
		}
	}

	if postBtn == nil {
		// Dump all buttons on the page.
		t.Log("No Post button found. Dumping all buttons...")
		var buttons []*rod.Element
		tryErr := rod.Try(func() {
			buttons = page.MustElements("div[role='button'], button")
		})
		if tryErr == nil {
			for i, btn := range buttons {
				text, _ := btn.Text()
				if len(text) > 60 {
					text = text[:60]
				}
				t.Logf("  Button[%d]: %q", i, text)
				if i > 15 {
					break
				}
			}
		}
		saveScreenshot(t, page, "/tmp/comment_debug_4_no_button.png")
		t.Fatal("Could not find Post button")
	}

	// Check if button is disabled/aria-disabled.
	btnAttrs, _ := postBtn.Eval(`() => {
		return 'disabled=' + this.getAttribute('disabled') +
			' aria-disabled=' + this.getAttribute('aria-disabled') +
			' class=' + (this.className||'').toString().substring(0,60) +
			' opacity=' + window.getComputedStyle(this).opacity +
			' pointer-events=' + window.getComputedStyle(this).pointerEvents
	}`)
	if btnAttrs != nil {
		t.Logf("Post button attrs: %s (found via %q)", btnAttrs.Value.Str(), btnFoundVia)
	}

	saveScreenshot(t, page, "/tmp/comment_debug_4_before_click.png")

	// Step 5: Click the Post button.
	t.Log("Step 5: Clicking Post button...")
	if err := postBtn.Click(proto.InputMouseButtonLeft, 1); err != nil {
		t.Logf("Click error: %v", err)
	}
	time.Sleep(5 * time.Second)

	saveScreenshot(t, page, "/tmp/comment_debug_5_after_click.png")

	// Step 6: Check if the comment was posted by looking for it in the comments section.
	t.Log("Step 6: Checking if comment appears in comments...")
	checkRes, _ := page.Eval(fmt.Sprintf(`() => {
		const spans = document.querySelectorAll('span');
		for (const s of spans) {
			if (s.textContent.includes('%s')) {
				return 'FOUND: ' + s.textContent.substring(0, 80);
			}
		}
		return 'NOT FOUND';
	}`, commentText))
	if checkRes != nil {
		t.Logf("Comment check: %s", checkRes.Value.Str())
	}
}

func saveScreenshot(t *testing.T, page *rod.Page, path string) {
	t.Helper()
	data, err := page.Screenshot(true, nil)
	if err == nil {
		os.WriteFile(path, data, 0644)
		t.Logf("Screenshot saved: %s", path)
	}
}
