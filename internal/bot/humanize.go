package bot

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	"github.com/nokhodian/mono-agent/internal/util"
)

// --------------------------------------------------------------------------
// Character-level human typing
// --------------------------------------------------------------------------

// WriteHumanLike types text into an element character-by-character with random
// inter-key delays (50-250 ms) and an optional typo simulation. When a typo
// occurs the function types a wrong character, pauses, backspaces 1-3 times,
// then retypes the correct character. After the full text has been entered a
// post-typing pause of 1-3 seconds is applied.
//
// The page parameter is used for keyboard events (page.Keyboard.Type) instead
// of el.Type. This is critical because some sites (e.g. Instagram) replace the
// initial element (textarea) with a different one (contenteditable div) when
// focused. Using page-level keyboard events ensures keystrokes reach whatever
// element currently has focus, not the potentially-detached original element.
//
// mistakeProbability should be in [0,1]; a value of 0.05 means 5 % of
// keystrokes will produce a simulated typo.
func WriteHumanLike(page *rod.Page, el *rod.Element, text string, mistakeProbability float64) error {
	// Click and focus the element to ensure it's active. Some sites swap the
	// underlying DOM node on focus (e.g. textarea → contenteditable div), so
	// after this point we use page.Keyboard for all keystroke delivery.
	_ = el.Click(proto.InputMouseButtonLeft, 1)
	util.SleepRandom(200*time.Millisecond, 400*time.Millisecond)
	_ = el.Focus()
	util.SleepRandom(100*time.Millisecond, 200*time.Millisecond)

	for i, ch := range text {
		// Decide whether to simulate a typo for this character.
		if mistakeProbability > 0 && rand.Float64() < mistakeProbability {
			if err := simulateTypo(page, ch); err != nil {
				return fmt.Errorf("typo simulation failed at index %d: %w", i, err)
			}
			continue
		}

		if err := typeCharacter(page, ch); err != nil {
			return fmt.Errorf("typing char at index %d failed: %w", i, err)
		}

		// Human-like inter-keystroke delay.
		util.SleepRandom(50*time.Millisecond, 250*time.Millisecond)
	}

	// Post-typing pause to mimic the user reviewing what they typed.
	util.SleepRandom(1*time.Second, 3*time.Second)
	return nil
}

// simulateTypo types a random wrong character, waits briefly, then backspaces
// 1-3 times and retypes the correct character.
func simulateTypo(page *rod.Page, correct rune) error {
	// Pick a random printable ASCII character that is NOT the correct one.
	wrong := randomWrongChar(correct)

	if err := typeCharacter(page, wrong); err != nil {
		return err
	}

	// Short pause before realising the mistake.
	util.SleepRandom(200*time.Millisecond, 500*time.Millisecond)

	// Backspace 1-3 times (sometimes people over-delete).
	backspaces := 1 + rand.Intn(3)
	for j := 0; j < backspaces; j++ {
		if err := page.Keyboard.Type(input.Backspace); err != nil {
			return fmt.Errorf("backspace failed: %w", err)
		}
		util.SleepRandom(50*time.Millisecond, 150*time.Millisecond)
	}

	// Retype the correct character (plus any that were over-deleted).
	if err := typeCharacter(page, correct); err != nil {
		return err
	}

	util.SleepRandom(100*time.Millisecond, 300*time.Millisecond)
	return nil
}

// typeCharacter types a single rune via the page keyboard. For standard BMP
// characters it uses page.Keyboard.Type() with the rune converted to an
// input.Key. For non-BMP characters (emoji, etc.) it falls back to
// page.InsertText() which uses CDP Input.insertText.
func typeCharacter(page *rod.Page, ch rune) error {
	// Use InsertText for any character outside the printable ASCII range that
	// Rod's keyboard map may not support (e.g. newlines, emoji, accented chars).
	if ch > 0x7E || ch < 0x20 {
		return page.InsertText(string(ch))
	}
	err := rod.Try(func() {
		if typeErr := page.Keyboard.Type(input.Key(ch)); typeErr != nil {
			// Fallback for supported-range chars with no key mapping.
			_ = page.InsertText(string(ch))
		}
	})
	if err != nil {
		// Panic recovered by rod.Try — use InsertText as fallback.
		return page.InsertText(string(ch))
	}
	return nil
}

// randomWrongChar returns a random printable ASCII character that differs from
// the given rune.
func randomWrongChar(correct rune) rune {
	const printable = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	for {
		candidate := rune(printable[rand.Intn(len(printable))])
		if candidate != correct {
			return candidate
		}
	}
}

// --------------------------------------------------------------------------
// Multi-line / emoji input
// --------------------------------------------------------------------------

// InputWithNewlines enters text that may contain newlines. When
// removeNewlines is true, newline characters are stripped from the text
// before input. Otherwise each '\n' is sent as Shift+Enter (soft return)
// which is the standard way to insert a line break inside messaging fields
// without submitting the form.
//
// Emoji and other non-BMP characters are handled correctly via el.Input().
func InputWithNewlines(el *rod.Element, text string, removeNewlines bool) error {
	if removeNewlines {
		clean := make([]rune, 0, len(text))
		for _, ch := range text {
			if ch != '\n' && ch != '\r' {
				clean = append(clean, ch)
			}
		}
		return el.Input(string(clean))
	}

	// Process the text segment by segment, splitting on newlines.
	segment := make([]rune, 0, 128)
	for _, ch := range text {
		if ch == '\n' {
			// Flush the accumulated segment.
			if len(segment) > 0 {
				if err := el.Input(string(segment)); err != nil {
					return fmt.Errorf("input segment failed: %w", err)
				}
				segment = segment[:0]
			}
			// Send Shift+Enter for a soft line break.
			if err := el.Type(input.ShiftLeft, input.Enter); err != nil {
				return fmt.Errorf("shift+enter failed: %w", err)
			}
			util.SleepRandom(100*time.Millisecond, 300*time.Millisecond)
			continue
		}
		// Skip bare carriage returns.
		if ch == '\r' {
			continue
		}
		segment = append(segment, ch)
	}

	// Flush any remaining text.
	if len(segment) > 0 {
		if err := el.Input(string(segment)); err != nil {
			return fmt.Errorf("input final segment failed: %w", err)
		}
	}
	return nil
}

// --------------------------------------------------------------------------
// Click helpers
// --------------------------------------------------------------------------

// ClickAndWaitNavigation clicks an element and waits for the resulting page
// navigation to complete. The navigation listener is set up BEFORE the click
// so that fast navigations are never missed.
func ClickAndWaitNavigation(page *rod.Page, el *rod.Element) error {
	// Subscribe to the navigation event before triggering the click.
	wait := page.MustWaitNavigation()

	if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("click failed: %w", err)
	}

	wait()
	return nil
}

// ClickAndWaitForContent clicks an element and then waits for dynamic content
// identified by contentSelector to appear and stabilise within the given
// timeout. The returned element is the first match of contentSelector.
func ClickAndWaitForContent(page *rod.Page, el *rod.Element, contentSelector string, timeout time.Duration) (*rod.Element, error) {
	if err := el.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return nil, fmt.Errorf("click failed: %w", err)
	}

	content, err := page.Timeout(timeout).Element(contentSelector)
	if err != nil {
		return nil, fmt.Errorf("content %q did not appear within %v: %w", contentSelector, timeout, err)
	}

	// Wait for the element to become visually stable (no layout shifts).
	if err := content.WaitStable(500 * time.Millisecond); err != nil {
		return nil, fmt.Errorf("content %q did not stabilise: %w", contentSelector, err)
	}

	return content, nil
}

// --------------------------------------------------------------------------
// Element location helpers
// --------------------------------------------------------------------------

// FindElementWithAlternatives tries to locate an element using the primary
// CSS selector. If that fails within a short initial probe, each alternative
// selector is tried in order. The first successful match is returned.
// If no selector matches within timeout the function returns an error.
func FindElementWithAlternatives(page *rod.Page, primary string, alternatives []string, timeout time.Duration) (*rod.Element, error) {
	deadline := time.Now().Add(timeout)

	// Try the primary selector with a short initial timeout.
	probeTimeout := timeout / time.Duration(1+len(alternatives))
	if probeTimeout < 500*time.Millisecond {
		probeTimeout = 500 * time.Millisecond
	}

	el, err := page.Timeout(probeTimeout).Element(primary)
	if err == nil {
		return el, nil
	}

	// Fall through to alternatives.
	for _, selector := range alternatives {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		if remaining > probeTimeout {
			remaining = probeTimeout
		}

		el, err = page.Timeout(remaining).Element(selector)
		if err == nil {
			return el, nil
		}
	}

	return nil, fmt.Errorf("no element found for primary %q or %d alternatives within %v", primary, len(alternatives), timeout)
}

// --------------------------------------------------------------------------
// Race / outcome waiting
// --------------------------------------------------------------------------

// WaitForOutcome waits for one of several possible page outcomes. Each entry
// in outcomes maps a human-readable label (e.g. "success", "error") to a CSS
// selector. The function races all selectors and returns the label, the
// matched element, and nil error for the first selector that appears.
// If no outcome is observed within timeout, an error is returned.
func WaitForOutcome(page *rod.Page, outcomes map[string]string, timeout time.Duration) (string, *rod.Element, error) {
	if len(outcomes) == 0 {
		return "", nil, fmt.Errorf("no outcomes provided")
	}

	timedPage := page.Timeout(timeout)
	race := timedPage.Race()

	// Build an ordered list so we can map the result back to a label.
	type entry struct {
		label    string
		selector string
	}
	ordered := make([]entry, 0, len(outcomes))

	for label, selector := range outcomes {
		ordered = append(ordered, entry{label: label, selector: selector})
		race = race.Element(selector)
	}

	el, err := race.Do()
	if err != nil {
		return "", nil, fmt.Errorf("none of %d outcomes appeared within %v: %w", len(outcomes), timeout, err)
	}

	// Determine which selector won by checking Matches on the returned element.
	for _, e := range ordered {
		matched, mErr := el.Matches(e.selector)
		if mErr == nil && matched {
			return e.label, el, nil
		}
	}

	// Fallback: should not happen, but return the element with unknown label.
	return "unknown", el, nil
}

// --------------------------------------------------------------------------
// Scrolling / infinite-load collection
// --------------------------------------------------------------------------

// ScrollAndCollect scrolls the page incrementally and collects elements that
// match itemSelector. Scrolling continues until maxItems are collected or no
// new items appear after several consecutive scroll attempts (lazy-load
// exhaustion). The function returns all collected elements.
func ScrollAndCollect(page *rod.Page, itemSelector string, maxItems int) ([]*rod.Element, error) {
	const (
		scrollStep       = 600.0  // pixels per scroll
		scrollSteps      = 3      // intermediate steps for smooth scroll
		maxStaleAttempts = 5      // stop after this many scrolls with no new items
		stabiliseWait    = 800    // ms to wait for lazy-loaded content
	)

	var collected []*rod.Element
	staleCount := 0
	previousCount := 0

	for {
		// Query all currently visible items.
		elements, err := page.Elements(itemSelector)
		if err != nil {
			return collected, fmt.Errorf("failed to query items: %w", err)
		}

		collected = elements

		// Check completion conditions.
		if len(collected) >= maxItems {
			collected = collected[:maxItems]
			return collected, nil
		}

		if len(collected) == previousCount {
			staleCount++
			if staleCount >= maxStaleAttempts {
				// No new items after several scrolls; lazy load exhausted.
				return collected, nil
			}
		} else {
			staleCount = 0
		}
		previousCount = len(collected)

		// Scroll down.
		if err := page.Mouse.Scroll(0, scrollStep, scrollSteps); err != nil {
			return collected, fmt.Errorf("scroll failed: %w", err)
		}

		// Wait for potential lazy-loaded content to render.
		util.SleepRandom(
			time.Duration(stabiliseWait)*time.Millisecond,
			time.Duration(stabiliseWait+500)*time.Millisecond,
		)
	}
}

// --------------------------------------------------------------------------
// File upload
// --------------------------------------------------------------------------

// UploadFile sets file paths on a file-input element using the CDP protocol.
// This works even when the <input type="file"> is hidden or not directly
// interactable.
func UploadFile(page *rod.Page, fileInputSelector string, filePaths []string) error {
	el, err := page.Element(fileInputSelector)
	if err != nil {
		return fmt.Errorf("file input %q not found: %w", fileInputSelector, err)
	}

	if err := el.SetFiles(filePaths); err != nil {
		return fmt.Errorf("failed to set files on %q: %w", fileInputSelector, err)
	}

	return nil
}

// --------------------------------------------------------------------------
// Resource blocking
// --------------------------------------------------------------------------

// BlockUnnecessaryResources sets up a request hijacker on the page that
// silently aborts requests for images, fonts, media, and stylesheets. This
// significantly reduces bandwidth and speeds up page loads for automation
// tasks where visual fidelity is not required.
//
// The hijack router runs in a background goroutine; the caller does not need
// to manage its lifecycle beyond the page's own lifetime.
func BlockUnnecessaryResources(page *rod.Page) {
	router := page.HijackRequests()

	router.MustAdd("*", func(ctx *rod.Hijack) {
		resourceType := ctx.Request.Type()
		switch resourceType {
		case proto.NetworkResourceTypeImage,
			proto.NetworkResourceTypeFont,
			proto.NetworkResourceTypeMedia,
			proto.NetworkResourceTypeStylesheet:
			ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
			return
		}
		ctx.ContinueRequest(&proto.FetchContinueRequest{})
	})

	go router.Run()
}
