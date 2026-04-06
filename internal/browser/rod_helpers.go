package browser

import (
	"fmt"
	"time"
)

// FindElementWithAlternatives tries to locate an element using the primary CSS
// selector. If that fails within a short initial probe, each alternative
// selector is tried in order. The first successful match is returned. If no
// selector matches within timeout the function returns an error.
//
// This is the PageInterface-based equivalent of bot.FindElementWithAlternatives.
func FindElementWithAlternatives(page PageInterface, primary string, alternatives []string, timeout time.Duration) (ElementHandle, error) {
	deadline := time.Now().Add(timeout)

	// Give each selector a proportional share of the total timeout.
	probeTimeout := timeout / time.Duration(1+len(alternatives))
	if probeTimeout < 500*time.Millisecond {
		probeTimeout = 500 * time.Millisecond
	}

	el, err := page.Element(primary, probeTimeout)
	if err == nil {
		return el, nil
	}

	for _, selector := range alternatives {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		if remaining > probeTimeout {
			remaining = probeTimeout
		}

		el, err = page.Element(selector, remaining)
		if err == nil {
			return el, nil
		}
	}

	return nil, fmt.Errorf("no element found for primary %q or %d alternatives within %v", primary, len(alternatives), timeout)
}

// ClickAndWaitForContent clicks an element and then waits for dynamic content
// identified by contentSelector to appear and stabilise within the given
// timeout. The returned element is the first match of contentSelector.
//
// This is the PageInterface-based equivalent of bot.ClickAndWaitForContent.
func ClickAndWaitForContent(page PageInterface, el ElementHandle, contentSelector string, timeout time.Duration) (ElementHandle, error) {
	if err := el.Click(); err != nil {
		return nil, fmt.Errorf("click failed: %w", err)
	}

	content, err := page.Element(contentSelector, timeout)
	if err != nil {
		return nil, fmt.Errorf("content %q did not appear within %v: %w", contentSelector, timeout, err)
	}

	if err := content.WaitStable(500 * time.Millisecond); err != nil {
		return nil, fmt.Errorf("content %q did not stabilise: %w", contentSelector, err)
	}

	return content, nil
}

// WaitForOutcome waits for one of several possible page outcomes. Each entry
// in outcomes maps a human-readable label (e.g. "success", "error") to a CSS
// selector. The function races all selectors and returns the label, the
// matched element, and nil error for the first selector that appears. If no
// outcome is observed within timeout, an error is returned.
//
// This is the PageInterface-based equivalent of bot.WaitForOutcome.
func WaitForOutcome(page PageInterface, outcomes map[string]string, timeout time.Duration) (string, ElementHandle, error) {
	if len(outcomes) == 0 {
		return "", nil, fmt.Errorf("no outcomes provided")
	}

	// Build ordered lists so we can map the Race index back to a label.
	labels := make([]string, 0, len(outcomes))
	selectors := make([]string, 0, len(outcomes))
	for label, sel := range outcomes {
		labels = append(labels, label)
		selectors = append(selectors, sel)
	}

	idx, el, err := page.Race(selectors, timeout)
	if err != nil {
		return "", nil, fmt.Errorf("none of %d outcomes appeared within %v: %w", len(outcomes), timeout, err)
	}

	if idx >= 0 && idx < len(labels) {
		return labels[idx], el, nil
	}

	return "unknown", el, nil
}

// ScrollAndCollect scrolls the page incrementally and collects elements that
// match itemSelector. Scrolling continues until maxItems are collected or no
// new items appear after several consecutive scroll attempts (lazy-load
// exhaustion). The function returns all collected element handles.
//
// This is the PageInterface-based equivalent of bot.ScrollAndCollect.
func ScrollAndCollect(page PageInterface, itemSelector string, maxItems int) ([]ElementHandle, error) {
	const (
		scrollStepY      = 600.0
		scrollSteps      = 3
		maxStaleAttempts = 5
		stabiliseWaitMs  = 800
	)

	var collected []ElementHandle
	staleCount := 0
	previousCount := 0

	for {
		elements, err := page.Elements(itemSelector)
		if err != nil {
			return collected, fmt.Errorf("failed to query items: %w", err)
		}

		collected = elements

		if len(collected) >= maxItems {
			collected = collected[:maxItems]
			return collected, nil
		}

		if len(collected) == previousCount {
			staleCount++
			if staleCount >= maxStaleAttempts {
				return collected, nil
			}
		} else {
			staleCount = 0
		}
		previousCount = len(collected)

		if err := page.MouseScroll(0, scrollStepY, scrollSteps); err != nil {
			return collected, fmt.Errorf("scroll failed: %w", err)
		}

		// Wait for potential lazy-loaded content to render. We use a simple
		// sleep here; callers that need precision should use WaitDOMStable.
		time.Sleep(time.Duration(stabiliseWaitMs) * time.Millisecond)
	}
}

// UploadFile sets file paths on a file-input element identified by selector.
//
// This is the PageInterface-based equivalent of bot.UploadFile.
func UploadFile(page PageInterface, fileInputSelector string, filePaths []string, timeout time.Duration) error {
	el, err := page.Element(fileInputSelector, timeout)
	if err != nil {
		return fmt.Errorf("file input %q not found: %w", fileInputSelector, err)
	}

	if err := el.SetFiles(filePaths); err != nil {
		return fmt.Errorf("failed to set files on %q: %w", fileInputSelector, err)
	}

	return nil
}
