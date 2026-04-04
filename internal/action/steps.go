package action

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	bot "github.com/nokhodian/mono-agent/internal/bot"
	"github.com/nokhodian/mono-agent/internal/util"
)

// xpathAttrPattern matches XPath expressions ending with /@attribute, used
// to extract an attribute directly from an XPath result.
var xpathAttrPattern = regexp.MustCompile(`^(.+)/@([a-zA-Z_][\w-]*)$`)

// toFloat64Ok extracts a float64 from an interface{} value (as produced by JSON
// unmarshaling or variable resolution). Returns (0, false) when the value is
// nil or cannot be converted.
func toFloat64Ok(v interface{}) (float64, bool) {
	if v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

// ---------------------------------------------------------------------------
// Element resolution helper
// ---------------------------------------------------------------------------

// resolveElement attempts to locate an element for the given step using
// several strategies in order of priority:
//  1. ElementRef — retrieve a previously stored element from the execution context.
//  2. Selector — find by CSS selector (with alternatives via FindElementWithAlternatives).
//  3. XPath — find by XPath expression.
//  4. ConfigKey — dynamically obtain a selector from the config manager.
func (ae *ActionExecutor) resolveElement(step StepDef) *rod.Element {
	timeout := stepTimeout(step, 10)

	// 1. ElementRef from context.
	if step.ElementRef != "" {
		elem := ae.execCtx.GetElement(step.ElementRef)
		if elem != nil {
			return elem
		}
		// Also check step results for elements.
		sr := ae.execCtx.GetStepResult(step.ElementRef)
		if sr != nil && sr.Element != nil {
			return sr.Element
		}
	}

	// 2. CSS Selector (with alternatives via bot.FindElementWithAlternatives).
	if step.Selector != "" {
		el, err := bot.FindElementWithAlternatives(ae.page, step.Selector, step.Alternatives, timeout)
		if err == nil {
			return el
		}
		return nil
	}

	// 3. XPath.
	if step.XPath != "" {
		var elem *rod.Element
		rod.Try(func() {
			elem = ae.page.Timeout(timeout).MustElementX(step.XPath)
		})
		return elem
	}

	// 4. ConfigKey — ask the config manager for a selector.
	if step.ConfigKey != "" {
		configSelector := ae.resolveConfigSelector(step.ConfigKey)
		if configSelector != "" {
			var elem *rod.Element
			rod.Try(func() {
				if isXPath(configSelector) {
					elem = ae.page.Timeout(timeout).MustElementX(configSelector)
				} else {
					elem = ae.page.Timeout(timeout).MustElement(configSelector)
				}
			})
			return elem
		}
	}

	return nil
}

// stepTimeout returns the step's explicit timeout or the given default, in
// seconds, converted to a time.Duration.
func stepTimeout(step StepDef, defaultSeconds float64) time.Duration {
	if step.Timeout > 0 {
		return time.Duration(step.Timeout * float64(time.Second))
	}
	return time.Duration(defaultSeconds * float64(time.Second))
}

// ---------------------------------------------------------------------------
// 1. stepNavigate — Navigate to a URL
// ---------------------------------------------------------------------------

func (ae *ActionExecutor) stepNavigate(ctx context.Context, step StepDef) (*StepResult, error) {
	targetURL := step.URL
	if targetURL == "" {
		return &StepResult{Success: false, StepID: step.ID, Error: fmt.Errorf("navigate step %s: empty URL", step.ID)}, nil
	}

	// Resolve relative URLs against the current page URL.
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		if ae.execCtx.CurrentURL != "" {
			base, err := url.Parse(ae.execCtx.CurrentURL)
			if err == nil {
				ref, err := url.Parse(targetURL)
				if err == nil {
					targetURL = base.ResolveReference(ref).String()
				}
			}
		}
		// If still relative, prepend https://
		if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
			targetURL = "https://" + strings.TrimPrefix(targetURL, "/")
		}
	}

	timeout := stepTimeout(step, 30)

	ae.logger.Debug().
		Str("stepID", step.ID).
		Str("url", targetURL).
		Dur("timeout", timeout).
		Msg("navigating")

	err := rod.Try(func() {
		ae.page.Timeout(timeout).MustNavigate(targetURL)

		// Wait strategy.
		switch step.WaitFor {
		case "dom_ready":
			ae.page.MustWaitDOMStable()
		case "network_idle":
			ae.page.MustWaitIdle()
		default: // "page_load" or empty
			ae.page.MustWaitLoad()
		}
	})

	if err != nil {
		return &StepResult{
			Success: false,
			StepID:  step.ID,
			Error:   fmt.Errorf("navigate to %s: %w", targetURL, err),
		}, nil
	}

	// Update current URL in context.
	info, infoErr := ae.page.Info()
	if infoErr == nil && info != nil {
		ae.execCtx.mu.Lock()
		ae.execCtx.CurrentURL = info.URL
		ae.execCtx.mu.Unlock()
		ae.execCtx.SetVariable("current_url", info.URL)
	}

	return &StepResult{Success: true, StepID: step.ID}, nil
}

// ---------------------------------------------------------------------------
// 2. stepWait — Wait for element or time duration
// ---------------------------------------------------------------------------

func (ae *ActionExecutor) stepWait(ctx context.Context, step StepDef) (*StepResult, error) {
	timeout := stepTimeout(step, 10)

	// If ConfigKey is set, obtain the selector from the config manager.
	selector := step.Selector
	if step.ConfigKey != "" && ae.configMgr != nil {
		configSelector := ae.resolveConfigSelector(step.ConfigKey)
		if configSelector != "" {
			selector = configSelector
		}
	}

	// If we have a selector or XPath, wait for the element.
	if selector != "" {
		ae.logger.Debug().
			Str("stepID", step.ID).
			Str("selector", selector).
			Dur("timeout", timeout).
			Msg("waiting for element")

		var elem *rod.Element
		var findErr error

		if isXPath(selector) {
			elem, findErr = ae.page.Timeout(timeout).ElementX(selector)
		} else {
			elem, findErr = ae.page.Timeout(timeout).Element(selector)
		}

		if findErr != nil {
			return &StepResult{
				Success: false,
				StepID:  step.ID,
				Error:   fmt.Errorf("wait for element %s: %w", selector, findErr),
			}, nil
		}

		// Store element if a variable name is provided.
		if step.Variable != "" && elem != nil {
			ae.execCtx.SetElement(step.Variable, elem)
		}

		return &StepResult{Success: true, Element: elem, StepID: step.ID}, nil
	}

	if step.XPath != "" {
		ae.logger.Debug().
			Str("stepID", step.ID).
			Str("xpath", step.XPath).
			Dur("timeout", timeout).
			Msg("waiting for element (xpath)")

		elem, findErr := ae.page.Timeout(timeout).ElementX(step.XPath)
		if findErr != nil {
			return &StepResult{
				Success: false,
				StepID:  step.ID,
				Error:   fmt.Errorf("wait for xpath element %s: %w", step.XPath, findErr),
			}, nil
		}

		if step.Variable != "" && elem != nil {
			ae.execCtx.SetElement(step.Variable, elem)
		}

		return &StepResult{Success: true, Element: elem, StepID: step.ID}, nil
	}

	// No selector — just wait for the specified duration.
	duration := timeout
	if d, ok := toFloat64Ok(step.Duration); ok && d > 0 {
		duration = time.Duration(d * float64(time.Second))
	}

	ae.logger.Debug().
		Str("stepID", step.ID).
		Dur("duration", duration).
		Msg("waiting (time)")

	select {
	case <-ctx.Done():
		return &StepResult{Success: false, StepID: step.ID, Error: ctx.Err()}, nil
	case <-time.After(duration):
	}

	return &StepResult{Success: true, StepID: step.ID}, nil
}

// ---------------------------------------------------------------------------
// 3. stepRefresh — Reload the current page
// ---------------------------------------------------------------------------

func (ae *ActionExecutor) stepRefresh(ctx context.Context, step StepDef) (*StepResult, error) {
	timeout := stepTimeout(step, 30)

	ae.logger.Debug().Str("stepID", step.ID).Msg("refreshing page")

	err := rod.Try(func() {
		ae.page.Timeout(timeout).MustReload()
		ae.page.Timeout(timeout).MustWaitLoad()
	})

	if err != nil {
		return &StepResult{
			Success: false,
			StepID:  step.ID,
			Error:   fmt.Errorf("refresh: %w", err),
		}, nil
	}

	return &StepResult{Success: true, StepID: step.ID}, nil
}

// ---------------------------------------------------------------------------
// 4. stepFindElement — Find an element using CSS selector, XPath, or config
// ---------------------------------------------------------------------------

func (ae *ActionExecutor) stepFindElement(ctx context.Context, step StepDef) (*StepResult, error) {
	timeout := stepTimeout(step, 10)

	// Use bot.FindElementWithAlternatives when we have a selector with alternatives.
	if step.Selector != "" {
		elem, err := bot.FindElementWithAlternatives(ae.page, step.Selector, step.Alternatives, timeout)
		if err != nil {
			return &StepResult{
				Success: false,
				StepID:  step.ID,
				Error:   fmt.Errorf("find_element %s: %w", step.ID, err),
			}, nil
		}

		// Store the element for later reference.
		ae.execCtx.SetElement(step.ID, elem)
		if step.Variable != "" {
			ae.execCtx.SetElement(step.Variable, elem)
		}
		if step.VariableName != "" {
			ae.execCtx.SetElement(step.VariableName, elem)
		}

		ae.logger.Debug().Str("stepID", step.ID).Msg("element found via selector")
		return &StepResult{Success: true, Element: elem, StepID: step.ID}, nil
	}

	// Build selector list from XPath/ConfigKey/Alternatives.
	selectors := ae.buildSelectorList(step)
	if len(selectors) == 0 {
		return &StepResult{
			Success: false,
			StepID:  step.ID,
			Error:   fmt.Errorf("find_element step %s: no selectors configured", step.ID),
		}, nil
	}

	var elem *rod.Element
	var lastErr error

	for _, sel := range selectors {
		err := rod.Try(func() {
			if isXPath(sel) {
				elem = ae.page.Timeout(timeout).MustElementX(sel)
			} else {
				elem = ae.page.Timeout(timeout).MustElement(sel)
			}
		})
		if err == nil && elem != nil {
			break
		}
		lastErr = err
		elem = nil
	}

	if elem == nil {
		return &StepResult{
			Success: false,
			StepID:  step.ID,
			Error:   fmt.Errorf("find_element %s: no matching element found (tried %d selectors): %v", step.ID, len(selectors), lastErr),
		}, nil
	}

	// Store the element for later reference.
	ae.execCtx.SetElement(step.ID, elem)
	if step.Variable != "" {
		ae.execCtx.SetElement(step.Variable, elem)
	}
	if step.VariableName != "" {
		ae.execCtx.SetElement(step.VariableName, elem)
		// Also set a variable so {{variable_name}} resolves to "true" in templates.
		ae.execCtx.SetVariable(step.VariableName, true)
	}

	ae.logger.Debug().Str("stepID", step.ID).Msg("element found")
	return &StepResult{Success: true, Element: elem, StepID: step.ID}, nil
}

// ---------------------------------------------------------------------------
// 5. stepClick — Click on an element
// ---------------------------------------------------------------------------

func (ae *ActionExecutor) stepClick(ctx context.Context, step StepDef) (*StepResult, error) {
	elem := ae.resolveElement(step)
	if elem == nil {
		return &StepResult{
			Success: false,
			StepID:  step.ID,
			Error:   fmt.Errorf("click step %s: no element to click", step.ID),
		}, nil
	}

	ae.logger.Debug().
		Str("stepID", step.ID).
		Str("waitFor", step.WaitFor).
		Str("waitAfter", step.WaitAfter).
		Msg("clicking element")

	// Only use ClickAndWaitNavigation for explicit page-level navigation waits.
	if step.WaitFor == "page_load" || step.WaitFor == "navigation" {
		if err := bot.ClickAndWaitNavigation(ae.page, elem); err != nil {
			return &StepResult{
				Success: false,
				StepID:  step.ID,
				Error:   fmt.Errorf("click %s (with navigation wait): %w", step.ID, err),
			}, nil
		}
		return &StepResult{Success: true, Element: elem, StepID: step.ID}, nil
	}

	// Standard click.
	if err := elem.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return &StepResult{
			Success: false,
			StepID:  step.ID,
			Error:   fmt.Errorf("click %s: %w", step.ID, err),
		}, nil
	}

	// Post-click wait strategy — uses WaitFor if WaitAfter is empty.
	waitStep := step
	if step.WaitAfter == "" && step.WaitFor != "" {
		waitStep.WaitAfter = step.WaitFor
	}
	ae.waitAfterClick(waitStep)

	return &StepResult{Success: true, Element: elem, StepID: step.ID}, nil
}

// waitAfterClick handles different wait strategies after a click action.
func (ae *ActionExecutor) waitAfterClick(step StepDef) {
	waitStrategy := step.WaitAfter

	switch waitStrategy {
	case "navigation", "page_load":
		rod.Try(func() {
			ae.page.MustWaitLoad()
		})

	case "ajax", "network_idle":
		rod.Try(func() {
			ae.page.MustWaitIdle()
		})

	case "race":
		// Wait for any of the race selectors to appear.
		if len(step.RaceSelectors) > 0 {
			timeout := stepTimeout(step, 5)
			_, _, _ = bot.WaitForOutcome(ae.page, step.RaceSelectors, timeout)
		}

	case "":
		// Default: short wait for UI to settle.
		util.SleepRandom(300*time.Millisecond, 700*time.Millisecond)

	default:
		// Treat as a CSS selector to wait for.
		rod.Try(func() {
			el := ae.page.Timeout(5 * time.Second).MustElement(waitStrategy)
			el.WaitStable(500 * time.Millisecond)
		})
	}
}

// ---------------------------------------------------------------------------
// 6. stepType — Type text into an input element
// ---------------------------------------------------------------------------

func (ae *ActionExecutor) stepType(ctx context.Context, step StepDef) (*StepResult, error) {
	elem := ae.resolveElement(step)
	if elem == nil {
		return &StepResult{
			Success: false,
			StepID:  step.ID,
			Error:   fmt.Errorf("type step %s: no element to type into", step.ID),
		}, nil
	}

	// Determine the text to type. Prefer step.Value (string), fall back to step.Text.
	text := step.Text
	if text == "" {
		if s, ok := step.Value.(string); ok {
			text = s
		} else if step.Value != nil {
			text = fmt.Sprintf("%v", step.Value)
		}
	}

	ae.logger.Debug().
		Str("stepID", step.ID).
		Bool("humanLike", step.HumanLike).
		Int("textLen", len(text)).
		Msg("typing text")

	if step.HumanLike {
		// Use bot.WriteHumanLike with 0.05 (5%) typo probability.
		// Pass ae.page so keystrokes use page.Keyboard (not el.Type) — this
		// ensures text reaches the focused element even if the DOM node was
		// swapped on focus (e.g. Instagram replaces textarea with contenteditable).
		if err := bot.WriteHumanLike(ae.page, elem, text, 0.05); err != nil {
			return &StepResult{
				Success: false,
				StepID:  step.ID,
				Error:   fmt.Errorf("type (human-like) %s: %w", step.ID, err),
			}, nil
		}
	} else {
		if err := elem.Input(text); err != nil {
			return &StepResult{
				Success: false,
				StepID:  step.ID,
				Error:   fmt.Errorf("type %s: %w", step.ID, err),
			}, nil
		}
	}

	return &StepResult{Success: true, Element: elem, StepID: step.ID}, nil
}

// ---------------------------------------------------------------------------
// 6b. stepUpload — Set files on a file input element
// ---------------------------------------------------------------------------

func (ae *ActionExecutor) stepUpload(ctx context.Context, step StepDef) (*StepResult, error) {
	elem := ae.resolveElement(step)
	if elem == nil {
		return &StepResult{
			Success: false,
			StepID:  step.ID,
			Error:   fmt.Errorf("upload step %s: no file input element found", step.ID),
		}, nil
	}

	// Resolve the file path(s) from step.Text or step.Value.
	filePath := step.Text
	if filePath == "" {
		if s, ok := step.Value.(string); ok {
			filePath = s
		}
	}
	if filePath == "" {
		return &StepResult{
			Success: false,
			StepID:  step.ID,
			Error:   fmt.Errorf("upload step %s: no file path provided", step.ID),
		}, nil
	}

	// Support multiple files separated by commas.
	var files []string
	for _, f := range strings.Split(filePath, ",") {
		f = strings.TrimSpace(f)
		if f != "" {
			files = append(files, f)
		}
	}

	ae.logger.Debug().
		Str("stepID", step.ID).
		Strs("files", files).
		Msg("uploading files")

	// Rod's SetFiles sets file paths on an <input type="file"> element.
	err := elem.SetFiles(files)
	if err != nil {
		return &StepResult{
			Success: false,
			StepID:  step.ID,
			Error:   fmt.Errorf("upload %s: %w", step.ID, err),
		}, nil
	}

	return &StepResult{Success: true, Element: elem, StepID: step.ID}, nil
}

// ---------------------------------------------------------------------------
// 7. stepScroll — Scroll page or element
// ---------------------------------------------------------------------------

func (ae *ActionExecutor) stepScroll(ctx context.Context, step StepDef) (*StepResult, error) {
	ae.logger.Debug().
		Str("stepID", step.ID).
		Str("direction", step.Direction).
		Interface("duration", step.Duration).
		Msg("scrolling")

	// If an element reference is given, scroll it into view.
	if step.ElementRef != "" {
		elem := ae.execCtx.GetElement(step.ElementRef)
		if elem != nil {
			err := rod.Try(func() {
				elem.MustScrollIntoView()
			})
			if err != nil {
				return &StepResult{
					Success: false,
					StepID:  step.ID,
					Error:   fmt.Errorf("scroll element into view %s: %w", step.ID, err),
				}, nil
			}

			// Additional scroll within the element for list containers.
			rod.Try(func() {
				ae.page.Mouse.MustScroll(0, 300)
			})

			waitDuration := time.Duration(1 * time.Second)
			if d, ok := toFloat64Ok(step.Duration); ok && d > 0 {
				waitDuration = time.Duration(d * float64(time.Second))
			}
			util.SleepRandom(waitDuration, waitDuration+500*time.Millisecond)

			return &StepResult{Success: true, StepID: step.ID}, nil
		}
	}

	// Page-level scroll.
	direction := strings.ToLower(step.Direction)
	if direction == "" {
		direction = "down"
	}

	scrollY := 300.0
	if direction == "up" {
		scrollY = -300.0
	}

	duration, ok := toFloat64Ok(step.Duration)
	if !ok || duration <= 0 {
		duration = 1.0
	}

	steps := int(duration * 3) // approximately 3 scroll increments per second
	if steps < 1 {
		steps = 1
	}

	for i := 0; i < steps; i++ {
		select {
		case <-ctx.Done():
			return &StepResult{Success: false, StepID: step.ID, Error: ctx.Err()}, nil
		default:
		}

		err := rod.Try(func() {
			ae.page.Mouse.MustScroll(0, float64(scrollY)/float64(steps))
		})
		if err != nil {
			return &StepResult{
				Success: false,
				StepID:  step.ID,
				Error:   fmt.Errorf("scroll %s: %w", step.ID, err),
			}, nil
		}

		util.SleepRandom(200*time.Millisecond, 400*time.Millisecond)
	}

	return &StepResult{Success: true, StepID: step.ID}, nil
}

// ---------------------------------------------------------------------------
// 8. stepHover — Hover over an element
// ---------------------------------------------------------------------------

func (ae *ActionExecutor) stepHover(ctx context.Context, step StepDef) (*StepResult, error) {
	elem := ae.resolveElement(step)
	if elem == nil {
		return &StepResult{
			Success: false,
			StepID:  step.ID,
			Error:   fmt.Errorf("hover step %s: no element to hover", step.ID),
		}, nil
	}

	ae.logger.Debug().Str("stepID", step.ID).Msg("hovering over element")

	if err := elem.Hover(); err != nil {
		return &StepResult{
			Success: false,
			StepID:  step.ID,
			Error:   fmt.Errorf("hover %s: %w", step.ID, err),
		}, nil
	}

	return &StepResult{Success: true, Element: elem, StepID: step.ID}, nil
}

// ---------------------------------------------------------------------------
// 9. stepExtractText — Extract text content from an element
// ---------------------------------------------------------------------------

func (ae *ActionExecutor) stepExtractText(ctx context.Context, step StepDef) (*StepResult, error) {
	timeout := stepTimeout(step, 10)

	selectors := ae.buildSelectorList(step)
	if len(selectors) == 0 {
		return &StepResult{
			Success: false,
			StepID:  step.ID,
			Error:   fmt.Errorf("extract_text step %s: no selectors configured", step.ID),
		}, nil
	}

	var text string
	var elem *rod.Element
	var lastErr error

	for _, sel := range selectors {
		// Check for XPath attribute pattern: //path/@attribute
		basePath, attrName, isAttr := resolveXPathAttribute(sel)
		if isAttr {
			err := rod.Try(func() {
				elem = ae.page.Timeout(timeout).MustElementX(basePath)
				attrVal, _ := elem.Attribute(attrName)
				if attrVal != nil {
					text = *attrVal
				}
			})
			if err == nil && text != "" {
				break
			}
			lastErr = err
			continue
		}

		err := rod.Try(func() {
			if isXPath(sel) {
				elem = ae.page.Timeout(timeout).MustElementX(sel)
			} else {
				elem = ae.page.Timeout(timeout).MustElement(sel)
			}
			text = elem.MustText()
		})
		if err == nil {
			break
		}
		lastErr = err
	}

	if text == "" && lastErr != nil {
		return &StepResult{
			Success: false,
			StepID:  step.ID,
			Error:   fmt.Errorf("extract_text %s: failed to extract text: %v", step.ID, lastErr),
		}, nil
	}

	text = strings.TrimSpace(text)

	// Store in variable.
	varName := step.Variable
	if varName == "" {
		varName = step.VariableName
	}
	if varName != "" {
		ae.execCtx.SetVariable(varName, text)
	}
	ae.execCtx.SetVariable(step.ID, text)

	ae.logger.Debug().
		Str("stepID", step.ID).
		Str("text", truncateStr(text, 80)).
		Msg("text extracted")

	return &StepResult{
		Success: true,
		Data:    text,
		Element: elem,
		StepID:  step.ID,
	}, nil
}

// ---------------------------------------------------------------------------
// 10. stepExtractAttribute — Extract an attribute value from an element
// ---------------------------------------------------------------------------

func (ae *ActionExecutor) stepExtractAttribute(ctx context.Context, step StepDef) (*StepResult, error) {
	timeout := stepTimeout(step, 10)

	attrName := step.Attribute
	if attrName == "" {
		attrName = "href"
	}

	selectors := ae.buildSelectorList(step)
	if len(selectors) == 0 {
		return &StepResult{
			Success: false,
			StepID:  step.ID,
			Error:   fmt.Errorf("extract_attribute step %s: no selectors configured", step.ID),
		}, nil
	}

	var attrValue string
	var elem *rod.Element
	var lastErr error

	for _, sel := range selectors {
		err := rod.Try(func() {
			if isXPath(sel) {
				elem = ae.page.Timeout(timeout).MustElementX(sel)
			} else {
				elem = ae.page.Timeout(timeout).MustElement(sel)
			}
			val, _ := elem.Attribute(attrName)
			if val != nil {
				attrValue = *val
			}
		})
		if err == nil && attrValue != "" {
			break
		}
		lastErr = err
		attrValue = ""
	}

	if attrValue == "" && lastErr != nil {
		return &StepResult{
			Success: false,
			StepID:  step.ID,
			Error:   fmt.Errorf("extract_attribute %s: attribute %q not found: %v", step.ID, attrName, lastErr),
		}, nil
	}

	// Resolve relative URLs for href/src attributes.
	if (attrName == "href" || attrName == "src") && attrValue != "" {
		attrValue = ae.resolveRelativeURL(attrValue)
	}

	// Store result.
	varName := step.Variable
	if varName == "" {
		varName = step.VariableName
	}
	if varName != "" {
		ae.execCtx.SetVariable(varName, attrValue)
	}
	ae.execCtx.SetVariable(step.ID, attrValue)

	ae.logger.Debug().
		Str("stepID", step.ID).
		Str("attribute", attrName).
		Str("value", truncateStr(attrValue, 100)).
		Msg("attribute extracted")

	return &StepResult{
		Success: true,
		Data:    attrValue,
		Element: elem,
		StepID:  step.ID,
	}, nil
}

// ---------------------------------------------------------------------------
// 11. stepExtractMultiple — Extract data from multiple matching elements
// ---------------------------------------------------------------------------

func (ae *ActionExecutor) stepExtractMultiple(ctx context.Context, step StepDef) (*StepResult, error) {
	timeout := stepTimeout(step, 15)

	selectors := ae.buildSelectorList(step)
	if len(selectors) == 0 {
		return &StepResult{
			Success: false,
			StepID:  step.ID,
			Error:   fmt.Errorf("extract_multiple step %s: no selectors configured", step.ID),
		}, nil
	}

	var elements rod.Elements
	var lastErr error

	for _, sel := range selectors {
		err := rod.Try(func() {
			if isXPath(sel) {
				elements = ae.page.Timeout(timeout).MustElementsX(sel)
			} else {
				elements = ae.page.Timeout(timeout).MustElements(sel)
			}
		})
		if err == nil && len(elements) > 0 {
			break
		}
		lastErr = err
		elements = nil
	}

	if len(elements) == 0 && lastErr != nil {
		return &StepResult{
			Success: false,
			StepID:  step.ID,
			Error:   fmt.Errorf("extract_multiple %s: no elements found: %v", step.ID, lastErr),
		}, nil
	}

	// Extract values from elements as a slice of maps.
	attrName := step.Attribute
	seen := make(map[string]bool)
	var extracted []map[string]interface{}

	for _, elem := range elements {
		item := make(map[string]interface{})

		// Extract text content.
		var text string
		rod.Try(func() {
			text = strings.TrimSpace(elem.MustText())
		})
		if text != "" {
			item["text"] = text
		}

		// Extract specified attribute.
		if attrName != "" {
			attrVal, attrErr := elem.Attribute(attrName)
			if attrErr == nil && attrVal != nil {
				val := *attrVal
				// Resolve relative URLs.
				if attrName == "href" || attrName == "src" {
					val = ae.resolveRelativeURL(val)
				}
				item[attrName] = val
			}
		} else {
			// Extract href by default if it exists on this element.
			href, hrefErr := elem.Attribute("href")
			if hrefErr == nil && href != nil && *href != "" {
				item["href"] = ae.resolveRelativeURL(*href)
			} else {
				// Fallback: look for first child <a> with an href (profile link in card containers).
				var childHref *string
				rod.Try(func() {
					a := elem.MustElement("a[href]")
					childHref, _ = a.Attribute("href")
				})
				if childHref != nil && *childHref != "" {
					item["href"] = ae.resolveRelativeURL(*childHref)
				}
			}
		}

		// Deduplicate based on the primary extracted value.
		dedupeKey := fmt.Sprintf("%v", item)
		if seen[dedupeKey] {
			continue
		}
		seen[dedupeKey] = true

		extracted = append(extracted, item)
	}

	count := len(extracted)

	ae.logger.Debug().
		Str("stepID", step.ID).
		Int("count", count).
		Msg("multiple elements extracted")

	// Store results.
	varName := step.Variable
	if varName == "" {
		varName = step.ID
	}
	ae.execCtx.SetVariable(varName, extracted)

	// Also add as extracted items for later saving.
	for _, item := range extracted {
		ae.execCtx.AddExtractedItem(item)
	}

	result := &StepResult{
		Success: true,
		Data:    extracted,
		StepID:  step.ID,
	}
	ae.execCtx.SetStepResult(step.ID, result)
	ae.execCtx.SetVariable(step.ID+".count", count)

	return result, nil
}

// ---------------------------------------------------------------------------
// 12. stepCondition — Evaluate a condition and execute then/else branches
// ---------------------------------------------------------------------------

func (ae *ActionExecutor) stepCondition(ctx context.Context, step StepDef) (*StepResult, error) {
	condResult := ae.evaluateCondition(step)

	ae.logger.Debug().
		Str("stepID", step.ID).
		Bool("result", condResult).
		Msg("condition evaluated")

	// Execute the appropriate branch.
	var branchIDs []string
	if condResult {
		branchIDs = step.Then
	} else {
		branchIDs = step.Else
	}

	if len(branchIDs) == 0 {
		return &StepResult{Success: true, Data: condResult, StepID: step.ID}, nil
	}

	// Prevent infinite recursion.
	recursionKey := fmt.Sprintf("condition_%s", step.ID)
	count := ae.execCtx.IncrementRecursion(recursionKey)
	const maxRecursion = 100
	if count > maxRecursion {
		ae.logger.Warn().
			Str("stepID", step.ID).
			Int("count", count).
			Msg("max recursion depth reached for condition")
		return &StepResult{Success: true, Data: condResult, StepID: step.ID}, nil
	}

	// Resolve branch step IDs to StepDef and execute.
	branchSteps := ae.getStepsByIDs(ae.actionDef.Steps, branchIDs)
	if err := ae.executeSteps(ctx, branchSteps); err != nil {
		return &StepResult{
			Success: false,
			StepID:  step.ID,
			Error:   fmt.Errorf("condition %s branch execution: %w", step.ID, err),
		}, nil
	}

	return &StepResult{Success: true, Data: condResult, StepID: step.ID}, nil
}

// evaluateCondition parses and evaluates the condition expression from a step.
// It supports both string expressions (e.g. "variable == 'value'") and
// ConditionDef objects with operators: "exists", "equals", "not_equals",
// "greater_than", "contains".
func (ae *ActionExecutor) evaluateCondition(step StepDef) bool {
	if step.Condition == nil {
		return false
	}

	// Try to parse as ConditionDef struct first.
	switch v := step.Condition.(type) {
	case map[string]interface{}:
		cond := parseConditionDef(v)
		if cond != nil {
			return ae.evalConditionDef(cond)
		}
	case *ConditionDef:
		return ae.evalConditionDef(v)
	case ConditionDef:
		return ae.evalConditionDef(&v)
	}

	// Fall back to string expression evaluation.
	cond, ok := step.Condition.(string)
	if !ok {
		// Try JSON round-trip for other types.
		data, err := json.Marshal(step.Condition)
		if err != nil {
			return false
		}
		var condDef ConditionDef
		if err := json.Unmarshal(data, &condDef); err == nil && condDef.Variable != "" {
			return ae.evalConditionDef(&condDef)
		}
		return false
	}

	// Handle compound conditions with "and" / "or".
	if strings.Contains(cond, " and ") {
		parts := strings.Split(cond, " and ")
		for _, part := range parts {
			if !ae.evalSimpleCondition(strings.TrimSpace(part)) {
				return false
			}
		}
		return true
	}
	if strings.Contains(cond, " or ") {
		parts := strings.Split(cond, " or ")
		for _, part := range parts {
			if ae.evalSimpleCondition(strings.TrimSpace(part)) {
				return true
			}
		}
		return false
	}

	return ae.evalSimpleCondition(cond)
}

// parseConditionDef converts a map[string]interface{} into a ConditionDef.
func parseConditionDef(m map[string]interface{}) *ConditionDef {
	cond := &ConditionDef{}
	if v, ok := m["variable"].(string); ok {
		cond.Variable = v
	}
	if v, ok := m["operator"].(string); ok {
		cond.Operator = v
	}
	cond.Value = m["value"]

	if cond.Variable == "" {
		return nil
	}
	return cond
}

// evalConditionDef evaluates a structured ConditionDef against the execution context.
func (ae *ActionExecutor) evalConditionDef(cond *ConditionDef) bool {
	varValue, exists := ae.execCtx.GetVariable(cond.Variable)

	switch cond.Operator {
	case "exists":
		return exists && varValue != nil

	case "not_exists":
		return !exists || varValue == nil

	case "equals":
		if !exists {
			return false
		}
		return fmt.Sprintf("%v", varValue) == fmt.Sprintf("%v", cond.Value)

	case "not_equals":
		if !exists {
			return true
		}
		return fmt.Sprintf("%v", varValue) != fmt.Sprintf("%v", cond.Value)

	case "greater_than":
		if !exists {
			return false
		}
		return toFloat64(varValue) > toFloat64(cond.Value)

	case "contains":
		if !exists {
			return false
		}
		return strings.Contains(fmt.Sprintf("%v", varValue), fmt.Sprintf("%v", cond.Value))

	default:
		ae.logger.Warn().
			Str("operator", cond.Operator).
			Msg("unknown condition operator, defaulting to false")
		return false
	}
}

// toFloat64 converts a value to float64 for numeric comparisons.
func toFloat64(val interface{}) float64 {
	switch v := val.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case int32:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	default:
		return 0
	}
}

// evalSimpleCondition evaluates a single condition expression like
// "variable == 'value'" or "variable != ''".
func (ae *ActionExecutor) evalSimpleCondition(expr string) bool {
	expr = strings.TrimSpace(expr)

	// Parse operator.
	operators := []struct {
		op   string
		eval func(left, right string) bool
	}{
		{"!=", func(l, r string) bool { return l != unquote(r) }},
		{"==", func(l, r string) bool { return l == unquote(r) }},
		{">=", func(l, r string) bool { return toFloat(l) >= toFloat(r) }},
		{"<=", func(l, r string) bool { return toFloat(l) <= toFloat(r) }},
		{">", func(l, r string) bool { return toFloat(l) > toFloat(r) }},
		{"<", func(l, r string) bool { return toFloat(l) < toFloat(r) }},
	}

	for _, op := range operators {
		if idx := strings.Index(expr, op.op); idx > 0 {
			left := strings.TrimSpace(expr[:idx])
			right := strings.TrimSpace(expr[idx+len(op.op):])

			// Resolve the left side variable.
			leftVal := ae.resolveConditionValue(left)
			rightVal := right

			return op.eval(leftVal, rightVal)
		}
	}

	// No operator found — treat as a truthiness check.
	val := ae.resolveConditionValue(expr)
	return val != "" && val != "0" && val != "false" && val != "None" && val != "nil"
}

// resolveConditionValue resolves a variable name to its string value for
// condition evaluation.
func (ae *ActionExecutor) resolveConditionValue(name string) string {
	val := ae.resolver.ResolvePath(name)
	if val == nil {
		return ""
	}
	return fmt.Sprintf("%v", val)
}

// ---------------------------------------------------------------------------
// 13. stepUpdateProgress — Increment a variable, update reached_index in db
// ---------------------------------------------------------------------------

func (ae *ActionExecutor) stepUpdateProgress(ctx context.Context, step StepDef) (*StepResult, error) {
	// Apply variable assignments from the Set map.
	if step.Set != nil {
		for key, value := range step.Set {
			resolved := ae.resolver.ResolveValue(value)
			ae.execCtx.SetVariable(key, resolved)
			ae.logger.Debug().
				Str("stepID", step.ID).
				Str("key", key).
				Interface("value", resolved).
				Msg("variable set")
		}
	}

	// Handle increment.
	if step.Increment != "" {
		val, _ := ae.execCtx.GetVariable(step.Increment)
		var newVal int
		switch v := val.(type) {
		case int:
			newVal = v + 1
		case float64:
			newVal = int(v) + 1
		default:
			newVal = 1
		}
		ae.execCtx.SetVariable(step.Increment, newVal)

		// Persist the reached index to storage.
		if ae.db != nil && ae.action != nil {
			if err := ae.db.UpdateActionReachedIndex(ae.action.ID, newVal); err != nil {
				ae.logger.Warn().Err(err).Int("index", newVal).Msg("failed to update reached index")
			}
		}

		ae.logger.Debug().
			Str("stepID", step.ID).
			Str("variable", step.Increment).
			Int("value", newVal).
			Msg("progress incremented")
	}

	return &StepResult{Success: true, StepID: step.ID}, nil
}

// ---------------------------------------------------------------------------
// 14. stepSaveData — Flush extracted data to storage
// ---------------------------------------------------------------------------

func (ae *ActionExecutor) stepSaveData(ctx context.Context, step StepDef) (*StepResult, error) {
	ae.logger.Debug().
		Str("stepID", step.ID).
		Str("dataSource", step.DataSource).
		Msg("saving data")

	// Collect data from the data source variable or from the step result.
	var dataToSave []map[string]interface{}

	if step.DataSource != "" {
		val, ok := ae.execCtx.GetVariable(step.DataSource)
		if !ok {
			// Try step result.
			sr := ae.execCtx.GetStepResult(step.DataSource)
			if sr != nil {
				val = sr.Data
			}
		}

		if val != nil {
			switch d := val.(type) {
			case map[string]interface{}:
				dataToSave = append(dataToSave, d)
			case []map[string]interface{}:
				dataToSave = append(dataToSave, d...)
			case []interface{}:
				for _, item := range d {
					if m, ok := item.(map[string]interface{}); ok {
						dataToSave = append(dataToSave, m)
					}
				}
			}
		}

	}

	// If no specific data source, flush all accumulated extracted items.
	if len(dataToSave) == 0 {
		ae.execCtx.mu.Lock()
		dataToSave = make([]map[string]interface{}, len(ae.execCtx.ExtractedItems))
		copy(dataToSave, ae.execCtx.ExtractedItems)
		ae.execCtx.mu.Unlock()
	}

	// Add items to the extracted list.
	for _, item := range dataToSave {
		ae.execCtx.AddExtractedItem(item)
	}

	// Persist to storage.
	if ae.db != nil && ae.action != nil && len(dataToSave) > 0 {
		if err := ae.db.SaveExtractedData(ae.action.ID, dataToSave); err != nil {
			ae.logger.Warn().Err(err).Msg("failed to save extracted data")
			return &StepResult{
				Success: false,
				StepID:  step.ID,
				Error:   fmt.Errorf("save_data: %w", err),
			}, nil
		}
	}

	ae.logger.Info().
		Str("stepID", step.ID).
		Int("itemCount", len(dataToSave)).
		Msg("data saved")

	return &StepResult{
		Success: true,
		Data:    len(dataToSave),
		StepID:  step.ID,
	}, nil
}

// ---------------------------------------------------------------------------
// 15. stepMarkFailed — Record a failure for the current loop item
// ---------------------------------------------------------------------------

func (ae *ActionExecutor) stepMarkFailed(ctx context.Context, step StepDef) (*StepResult, error) {
	idx := 0
	if v, ok := ae.execCtx.GetVariable("loopIndex"); ok {
		switch i := v.(type) {
		case int:
			idx = i
		case float64:
			idx = int(i)
		}
	}

	errMsg := "step marked as failed"
	if step.Text != "" {
		errMsg = ae.resolver.Resolve(step.Text)
	} else if step.Description != "" {
		errMsg = ae.resolver.Resolve(step.Description)
	}

	ae.execCtx.AddFailedItem(FailedItem{
		StepID:    step.ID,
		Error:     fmt.Errorf("%s", errMsg),
		Timestamp: time.Now(),
		Index:     idx,
	})

	ae.logger.Warn().
		Str("stepID", step.ID).
		Int("index", idx).
		Str("reason", errMsg).
		Msg("item marked as failed")

	return &StepResult{Success: true, StepID: step.ID}, nil
}

// ---------------------------------------------------------------------------
// 16. stepSetVariable — Set an execution variable directly
// ---------------------------------------------------------------------------

func (ae *ActionExecutor) stepSetVariable(_ context.Context, step StepDef) (*StepResult, error) {
	varName := step.VariableName
	if varName == "" {
		varName = step.Variable
	}
	if varName == "" {
		return &StepResult{Success: false, StepID: step.ID, Error: fmt.Errorf("set_variable step %s: missing variable_name", step.ID)}, nil
	}
	ae.execCtx.SetVariable(varName, step.Value)
	ae.logger.Debug().
		Str("stepID", step.ID).
		Str("type", "set_variable").
		Msg("variable set")
	return &StepResult{Success: true, StepID: step.ID}, nil
}

// ---------------------------------------------------------------------------
// 17. stepLog — Log a message with variable resolution
// ---------------------------------------------------------------------------

func (ae *ActionExecutor) stepLog(ctx context.Context, step StepDef) (*StepResult, error) {
	msg := step.Text
	if msg == "" {
		msg = step.Description
	}
	if msg == "" {
		if s, ok := step.Value.(string); ok {
			msg = s
		}
	}

	resolved := ae.resolver.Resolve(msg)

	ae.logger.Info().
		Str("stepID", step.ID).
		Str("message", resolved).
		Msg("action log")

	return &StepResult{Success: true, Data: resolved, StepID: step.ID}, nil
}

// ---------------------------------------------------------------------------
// 17. stepCallBotMethod — Call a method on the BotAdapter
// ---------------------------------------------------------------------------

func (ae *ActionExecutor) stepCallBotMethod(ctx context.Context, step StepDef) (*StepResult, error) {
	if ae.botAdapter == nil {
		return &StepResult{
			Success: false,
			StepID:  step.ID,
			Error:   fmt.Errorf("call_bot_method step %s: no bot adapter configured", step.ID),
		}, nil
	}

	methodName := step.MethodName
	if methodName == "" {
		methodName = step.Method
	}
	if methodName == "" {
		return &StepResult{
			Success: false,
			StepID:  step.ID,
			Error:   fmt.Errorf("call_bot_method step %s: no method name specified", step.ID),
		}, nil
	}

	fn, ok := ae.botAdapter.GetMethodByName(methodName)
	if !ok {
		return &StepResult{
			Success: false,
			StepID:  step.ID,
			Error:   fmt.Errorf("call_bot_method step %s: method %q not found on bot adapter", step.ID, methodName),
		}, nil
	}

	ae.logger.Debug().
		Str("stepID", step.ID).
		Str("method", methodName).
		Int("argCount", len(step.Args)).
		Msg("calling bot method")

	// Resolve arguments. The page is always prepended as the first argument
	// so bot methods have access to the browser page.
	resolvedArgs := []interface{}{ae.page}
	for _, arg := range step.Args {
		resolvedArgs = append(resolvedArgs, ae.resolver.ResolveValue(arg))
	}

	result, err := fn(ctx, resolvedArgs...)
	if err != nil {
		return &StepResult{
			Success: false,
			StepID:  step.ID,
			Error:   fmt.Errorf("call_bot_method %s/%s: %w", step.ID, methodName, err),
		}, nil
	}

	// Store the result in a variable if specified.
	varName := step.VariableName
	if varName == "" {
		varName = step.Variable
	}
	if varName != "" {
		ae.execCtx.SetVariable(varName, result)
	}

	// If the bot method returned a map, also add it as an extracted item
	// so it appears in the node output.
	if m, ok := result.(map[string]interface{}); ok && len(m) > 0 {
		ae.execCtx.AddExtractedItem(m)
	}

	ae.logger.Debug().
		Str("stepID", step.ID).
		Str("method", methodName).
		Msg("bot method executed successfully")

	return &StepResult{
		Success: true,
		Data:    result,
		StepID:  step.ID,
	}, nil
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// buildSelectorList assembles the list of selectors to try for element lookup,
// starting with the primary selector from xpath/selector/configKey, then
// alternatives.
func (ae *ActionExecutor) buildSelectorList(step StepDef) []string {
	var selectors []string

	// Primary selector.
	if step.XPath != "" {
		selectors = append(selectors, step.XPath)
	} else if step.Selector != "" {
		selectors = append(selectors, step.Selector)
	} else if step.ConfigKey != "" {
		// ConfigKey means we should look up the selector from the config manager.
		configSelector := ae.resolveConfigSelector(step.ConfigKey)
		if configSelector != "" {
			selectors = append(selectors, configSelector)
		}
	}

	// Alternatives.
	for _, alt := range step.Alternatives {
		if alt != "" {
			// If the alternative is a configKey (no / or [ prefix), resolve it.
			if !isXPath(alt) && !strings.HasPrefix(alt, ".") && !strings.HasPrefix(alt, "#") {
				resolved := ae.resolveConfigSelector(alt)
				if resolved != "" {
					selectors = append(selectors, resolved)
					continue
				}
			}
			selectors = append(selectors, alt)
		}
	}

	return selectors
}

// resolveConfigSelector looks up a config key through the config manager to
// get an XPath or CSS selector.
func (ae *ActionExecutor) resolveConfigSelector(configKey string) string {
	if ae.configMgr == nil {
		return ""
	}

	platform := ""
	if ae.action != nil {
		platform = ae.action.TargetPlatform
	}

	configContext := ""
	if v, ok := ae.execCtx.GetVariable("configContext"); ok {
		if s, ok := v.(string); ok {
			configContext = s
		}
	}

	// Get page HTML for config resolution.
	var html string
	rod.Try(func() {
		html = ae.page.MustHTML()
	})

	result, err := ae.configMgr.GetConfig(
		platform,
		configKey,
		configContext,
		html,
		fmt.Sprintf("Find selector for %s", configKey),
		nil,
	)
	if err != nil {
		ae.logger.Debug().
			Err(err).
			Str("configKey", configKey).
			Msg("config selector resolution failed")
		return ""
	}

	switch v := result.(type) {
	case string:
		return v
	case map[string]interface{}:
		if xpath, ok := v["xpath"].(string); ok {
			return xpath
		}
	}
	return ""
}

// resolveRelativeURL converts a relative URL to an absolute one using the
// current page URL as the base.
func (ae *ActionExecutor) resolveRelativeURL(rawURL string) string {
	if rawURL == "" {
		return rawURL
	}
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		return rawURL
	}

	baseURL := ae.execCtx.CurrentURL
	if baseURL == "" {
		var info *proto.TargetTargetInfo
		rod.Try(func() {
			info, _ = ae.page.Info()
		})
		if info != nil {
			baseURL = info.URL
		}
	}

	if baseURL == "" {
		return rawURL
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return rawURL
	}

	ref, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	return base.ResolveReference(ref).String()
}

// resolveXPathAttribute splits an XPath like "//div/@class" into the base
// XPath "//div" and the attribute name "class".
func resolveXPathAttribute(xpath string) (basePath, attrName string, isAttr bool) {
	matches := xpathAttrPattern.FindStringSubmatch(xpath)
	if len(matches) == 3 {
		return matches[1], matches[2], true
	}
	return xpath, "", false
}

// isXPath returns true if the selector string looks like an XPath expression.
func isXPath(selector string) bool {
	return strings.HasPrefix(selector, "/") ||
		strings.HasPrefix(selector, "(") ||
		strings.HasPrefix(selector, ".//")
}

// unquote removes surrounding single or double quotes from a string.
func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// toFloat converts a string to a float64, returning 0 on failure.
func toFloat(s string) float64 {
	s = strings.TrimSpace(s)
	s = unquote(s)
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

// truncateStr shortens a string for display purposes.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
