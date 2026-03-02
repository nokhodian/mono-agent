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

// TestInstagramDMDebug is a diagnostic test to figure out what the DM page
// looks like after typing a username in the search box on /direct/new/.
func TestInstagramDMDebug(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	_, page, cleanup := launchTestBrowser(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	_ = ctx

	targetURL := "https://www.instagram.com/direct/new/"
	t.Logf("Navigating to %s", targetURL)

	err := rod.Try(func() {
		page.MustNavigate(targetURL).MustWaitLoad()
	})
	if err != nil {
		t.Fatalf("Failed to navigate: %v", err)
	}
	time.Sleep(3 * time.Second)

	t.Logf("Current URL: %s", page.MustInfo().URL)

	// Step 1: Dismiss "Turn on Notifications" dialog if present.
	t.Log("Looking for notification dialog...")
	dismissXPaths := []string{
		"//button[normalize-space(.)='Not Now']",
		"//div[@role='button'][normalize-space(.)='Not Now']",
	}
	for _, xpath := range dismissXPaths {
		var btn *rod.Element
		tryErr := rod.Try(func() {
			btn = page.Timeout(3 * time.Second).MustElementX(xpath)
		})
		if tryErr == nil && btn != nil {
			t.Logf("Dismissing notification dialog via %q", xpath)
			_ = btn.Click(proto.InputMouseButtonLeft, 1)
			time.Sleep(2 * time.Second)
			break
		}
	}

	// Step 2: Find search input.
	t.Log("Looking for search input...")
	searchSelectors := []string{
		"input[name='searchInput']",
		"input[placeholder='Search']",
		"input[placeholder='Search...']",
		"input[type='text']",
		"input",
	}
	var searchInput *rod.Element
	for _, sel := range searchSelectors {
		el, findErr := page.Timeout(3 * time.Second).Element(sel)
		if findErr == nil && el != nil {
			attrs, _ := el.Eval(`() => this.tagName + ' | name=' + this.getAttribute('name') + ' | placeholder=' + this.getAttribute('placeholder') + ' | type=' + this.getAttribute('type')`)
			t.Logf("FOUND search input via %q: %v", sel, attrs.Value)
			searchInput = el
			break
		}
	}
	if searchInput == nil {
		t.Fatal("Could not find search input")
	}

	// Step 3: Click to focus.
	t.Log("Clicking search input...")
	_ = searchInput.Click(proto.InputMouseButtonLeft, 1)
	time.Sleep(500 * time.Millisecond)

	// Step 4: Type username character by character using page.Keyboard.
	// We use page.Keyboard instead of searchInput.Type() because the element
	// inherits the timeout context from page.Timeout() used during lookup.
	username := "mortezanoes"
	t.Logf("Typing username %q via page.Keyboard...", username)
	for _, ch := range username {
		if err := page.Keyboard.Type(input.Key(ch)); err != nil {
			t.Logf("WARNING: Keyboard.Type failed for char %c: %v", ch, err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Step 5: Wait for search results.
	t.Log("Waiting 4 seconds for search results...")
	time.Sleep(4 * time.Second)

	// Step 6: Check the search input value.
	val, _ := searchInput.Eval(`() => this.value`)
	if val != nil {
		t.Logf("Search input value: %q", val.Value.Str())
	}

	// Step 7: Screenshot after typing.
	screenshotData, ssErr := page.Screenshot(true, nil)
	if ssErr == nil {
		ssPath := "/tmp/instagram_dm_after_typing.png"
		os.WriteFile(ssPath, screenshotData, 0644)
		t.Logf("Screenshot saved to %s", ssPath)
	}

	// Step 8: Dump dialog contents — look for search results.
	t.Log("Dumping dialog contents after typing...")
	res, evalErr := page.Eval(`() => {
		const dialogs = document.querySelectorAll('div[role="dialog"]');
		let output = "Dialogs: " + dialogs.length + "\n";
		dialogs.forEach((d, i) => {
			const children = d.querySelectorAll('*');
			output += "Dialog " + i + ": " + children.length + " descendants\n";
			children.forEach(c => {
				const text = (c.textContent || "").trim();
				if (text.length > 0 && text.length < 80) {
					const tag = c.tagName.toLowerCase();
					const role = c.getAttribute('role') || '';
					const type = c.getAttribute('type') || '';
					const tabindex = c.getAttribute('tabindex') || '';
					const cls = (c.className || '').toString().substring(0, 40);
					if (role || type || tag === 'input' || tag === 'button' || tag === 'span' || tag === 'img' || tabindex) {
						output += "  <" + tag + " role=" + role + " type=" + type + " tabindex=" + tabindex + " class=" + cls + "> " + text.substring(0, 50) + "\n";
					}
				}
			});
		});
		return output;
	}`)
	if evalErr == nil && res != nil {
		t.Logf("Dialog dump:\n%s", res.Value.Str())
	} else {
		t.Logf("Dialog dump failed: %v", evalErr)
	}

	// Step 9: Try broader selectors to find anything that looks like search results.
	broadSelectors := []string{
		"div[role='listbox']",
		"div[role='option']",
		"div[role='radiogroup']",
		"input[type='checkbox']",
		"input[type='radio']",
		"div[role='checkbox']",
		"div[role='radio']",
	}
	t.Log("Trying broad result selectors...")
	for _, sel := range broadSelectors {
		elements, findErr := page.Elements(sel)
		if findErr == nil && len(elements) > 0 {
			t.Logf("  FOUND %d elements for %q", len(elements), sel)
			for i, el := range elements {
				if i > 3 {
					break
				}
				text, _ := el.Text()
				if len(text) > 80 {
					text = text[:80]
				}
				attrs, _ := el.Eval(`() => this.tagName + ' role=' + this.getAttribute('role') + ' class=' + (this.className||'').toString().substring(0,40)`)
				t.Logf("    [%d] text=%q attrs=%v", i, text, attrs.Value)
			}
		} else {
			t.Logf("  NONE for %q", sel)
		}
	}

	// Step 10: Look for spans/divs that contain "mortezanoes" and trace ancestry.
	t.Log("Looking for elements containing the username text...")
	usernameRes, _ := page.Eval(fmt.Sprintf(`() => {
		const all = document.querySelectorAll('span, div, button, a');
		let found = [];
		for (const el of all) {
			const ownText = Array.from(el.childNodes)
				.filter(n => n.nodeType === 3)
				.map(n => n.textContent.trim())
				.join('');
			if (ownText.toLowerCase().includes('%s')) {
				const tag = el.tagName.toLowerCase();
				const role = el.getAttribute('role') || '';
				// Trace 5 levels up for the ancestor chain.
				let ancestors = [];
				let curr = el;
				for (let i = 0; i < 6; i++) {
					curr = curr.parentElement;
					if (!curr) break;
					const aTag = curr.tagName.toLowerCase();
					const aRole = curr.getAttribute('role') || '';
					const aTabindex = curr.getAttribute('tabindex') || '';
					const aCursor = curr.style.cursor || '';
					const aClass = (curr.className||'').toString().substring(0, 30);
					const aOnClick = curr.onclick ? 'hasOnClick' : '';
					ancestors.push(aTag + '[role=' + aRole + ' tabindex=' + aTabindex + ' cursor=' + aCursor + ' class=' + aClass + ' ' + aOnClick + ']');
				}
				found.push(tag + '[role=' + role + ']: "' + ownText.substring(0,50) + '"');
				found.push('  Ancestors: ' + ancestors.join(' > '));
			}
		}
		return found.join('\n');
	}`, username))
	if usernameRes != nil {
		t.Logf("Elements containing %q:\n%s", username, usernameRes.Value.Str())
	}

	// Step 11: Dump the full HTML of the area around search results.
	t.Log("Looking for search result container...")
	containerRes, _ := page.Eval(`() => {
		// Find the search input and look at its siblings/parent for results.
		const input = document.querySelector('input[name="searchInput"]');
		if (!input) return "No search input found";
		// Walk up to find the form/container.
		let container = input;
		for (let i = 0; i < 5; i++) {
			container = container.parentElement;
			if (!container) break;
		}
		if (!container) return "No container found";
		// Dump children structure.
		let output = "Container tag: " + container.tagName + " role=" + (container.getAttribute('role')||'') + "\n";
		output += "Container children: " + container.children.length + "\n";
		// Walk immediate children and their first-level children.
		for (let i = 0; i < container.children.length; i++) {
			const child = container.children[i];
			const cTag = child.tagName.toLowerCase();
			const cRole = child.getAttribute('role') || '';
			const cText = (child.textContent || '').trim().substring(0, 60);
			output += "  Child[" + i + "] <" + cTag + " role=" + cRole + "> childCount=" + child.children.length + " text=" + cText + "\n";
			for (let j = 0; j < Math.min(child.children.length, 5); j++) {
				const gc = child.children[j];
				const gcTag = gc.tagName.toLowerCase();
				const gcRole = gc.getAttribute('role') || '';
				const gcText = (gc.textContent || '').trim().substring(0, 40);
				output += "    GC[" + j + "] <" + gcTag + " role=" + gcRole + "> childCount=" + gc.children.length + " text=" + gcText + "\n";
			}
		}
		return output;
	}`)
	if containerRes != nil {
		t.Logf("Container structure:\n%s", containerRes.Value.Str())
	}
}

func init() {
	// Suppress unused import.
	_ = fmt.Sprintf
}
