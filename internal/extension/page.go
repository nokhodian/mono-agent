package extension

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nokhodian/mono-agent/internal/browser"
)

const defaultTimeout = 30 * time.Second

// ExtensionPage implements browser.PageInterface by sending commands to the
// Chrome Extension over WebSocket.
type ExtensionPage struct {
	server  *Server
	tabID   int
	timeout time.Duration
}

// Compile-time check that ExtensionPage satisfies browser.PageInterface.
var _ browser.PageInterface = (*ExtensionPage)(nil)

// NewExtensionPage creates a page handle for the given tab.
func NewExtensionPage(server *Server, tabID int) *ExtensionPage {
	return &ExtensionPage{
		server:  server,
		tabID:   tabID,
		timeout: defaultTimeout,
	}
}

func (ep *ExtensionPage) effectiveTimeout() time.Duration {
	if ep.timeout > 0 {
		return ep.timeout
	}
	return defaultTimeout
}

// dataMap safely extracts the Data field as a map.
func (r *Response) dataMap() map[string]interface{} {
	if m, ok := r.Data.(map[string]interface{}); ok {
		return m
	}
	return nil
}

func (ep *ExtensionPage) send(cmdType string, params map[string]interface{}) (*Response, error) {
	if params == nil {
		params = make(map[string]interface{})
	}
	return ep.server.SendCommand(&Command{
		Type:   cmdType,
		TabID:  ep.tabID,
		Params: params,
	}, ep.effectiveTimeout())
}

// ---------------------------------------------------------------------------
// Navigation
// ---------------------------------------------------------------------------

func (ep *ExtensionPage) Navigate(url string) error {
	_, err := ep.send(CmdNavigate, map[string]interface{}{"url": url})
	return err
}

func (ep *ExtensionPage) WaitLoad() error {
	_, err := ep.send(CmdWaitLoad, nil)
	return err
}

func (ep *ExtensionPage) WaitDOMStable(timeout time.Duration) error {
	_, err := ep.send(CmdWaitLoad, map[string]interface{}{
		"timeout": timeout.Milliseconds(),
		"mode":    "dom_stable",
	})
	return err
}

func (ep *ExtensionPage) WaitIdle(timeout time.Duration) error {
	_, err := ep.send(CmdWaitLoad, map[string]interface{}{
		"timeout": timeout.Milliseconds(),
		"mode":    "idle",
	})
	return err
}

func (ep *ExtensionPage) Reload() error {
	_, err := ep.send(CmdReload, nil)
	return err
}

func (ep *ExtensionPage) GetURL() (string, error) {
	resp, err := ep.send(CmdPageInfo, nil)
	if err != nil {
		return "", err
	}
	url, _ := resp.dataMap()["url"].(string)
	return url, nil
}

// ---------------------------------------------------------------------------
// Element queries
// ---------------------------------------------------------------------------

func (ep *ExtensionPage) Element(selector string, timeout time.Duration) (browser.ElementHandle, error) {
	resp, err := ep.server.SendCommand(&Command{
		Type:  CmdElement,
		TabID: ep.tabID,
		Params: map[string]interface{}{
			"selector": selector,
			"timeout":  timeout.Milliseconds(),
		},
	}, timeout+5*time.Second)
	if err != nil {
		return nil, err
	}
	elemID, _ := resp.dataMap()["elementId"].(string)
	if elemID == "" {
		return nil, fmt.Errorf("element not found: %s", selector)
	}
	return &ExtensionElement{server: ep.server, tabID: ep.tabID, elementID: elemID, timeout: ep.effectiveTimeout()}, nil
}

func (ep *ExtensionPage) ElementX(xpath string, timeout time.Duration) (browser.ElementHandle, error) {
	resp, err := ep.server.SendCommand(&Command{
		Type:  CmdElement,
		TabID: ep.tabID,
		Params: map[string]interface{}{
			"xpath":   xpath,
			"timeout": timeout.Milliseconds(),
		},
	}, timeout+5*time.Second)
	if err != nil {
		return nil, err
	}
	elemID, _ := resp.dataMap()["elementId"].(string)
	if elemID == "" {
		return nil, fmt.Errorf("element not found: %s", xpath)
	}
	return &ExtensionElement{server: ep.server, tabID: ep.tabID, elementID: elemID, timeout: ep.effectiveTimeout()}, nil
}

func (ep *ExtensionPage) Elements(selector string) ([]browser.ElementHandle, error) {
	resp, err := ep.send(CmdElements, map[string]interface{}{"selector": selector})
	if err != nil {
		return nil, err
	}
	ids, ok := resp.dataMap()["elementIds"].([]interface{})
	if !ok {
		return nil, nil
	}
	elems := make([]browser.ElementHandle, 0, len(ids))
	for _, raw := range ids {
		if id, ok := raw.(string); ok {
			elems = append(elems, &ExtensionElement{
				server: ep.server, tabID: ep.tabID, elementID: id, timeout: ep.effectiveTimeout(),
			})
		}
	}
	return elems, nil
}

func (ep *ExtensionPage) Has(selector string) (bool, error) {
	resp, err := ep.send(CmdHas, map[string]interface{}{"selector": selector})
	if err != nil {
		return false, err
	}
	found, _ := resp.dataMap()["found"].(bool)
	return found, nil
}

func (ep *ExtensionPage) Race(selectors []string, timeout time.Duration) (int, browser.ElementHandle, error) {
	resp, err := ep.server.SendCommand(&Command{
		Type:  CmdRace,
		TabID: ep.tabID,
		Params: map[string]interface{}{
			"selectors": selectors,
			"timeout":   timeout.Milliseconds(),
		},
	}, timeout+5*time.Second)
	if err != nil {
		return -1, nil, err
	}
	idx, _ := resp.dataMap()["index"].(float64)
	elemID, _ := resp.dataMap()["elementId"].(string)
	if elemID == "" {
		return int(idx), nil, nil
	}
	elem := &ExtensionElement{server: ep.server, tabID: ep.tabID, elementID: elemID, timeout: ep.effectiveTimeout()}
	return int(idx), elem, nil
}

// ---------------------------------------------------------------------------
// Keyboard / input
// ---------------------------------------------------------------------------

func (ep *ExtensionPage) KeyboardType(keys ...rune) error {
	_, err := ep.send(CmdKeyboardType, map[string]interface{}{
		"text": string(keys),
	})
	return err
}

func (ep *ExtensionPage) KeyboardPress(key rune) error {
	_, err := ep.send(CmdKeyboardPress, map[string]interface{}{
		"key": string(key),
	})
	return err
}

// EvalCDP evaluates JavaScript via chrome.debugger Runtime.evaluate (bypasses CSP).
func (ep *ExtensionPage) EvalCDP(js string) (interface{}, error) {
	resp, err := ep.send("eval_cdp", map[string]interface{}{
		"expression": js,
	})
	if err != nil {
		return nil, err
	}
	data := resp.Data
	if m := resp.dataMap(); m != nil {
		if r, ok := m["result"]; ok {
			data = r
		}
	}
	return data, nil
}

// TypeCDP types text using Chrome Debugger Protocol (Input.insertText).
// Optionally clicks the element via CDP first for real focus.
func (ep *ExtensionPage) TypeCDP(text string) error {
	_, err := ep.send("type_cdp", map[string]interface{}{
		"text": text,
	})
	return err
}

// TypeCDPOnElement types text via CDP after clicking the element for real focus.
func (ep *ExtensionPage) TypeCDPOnElement(text string, elementID string) error {
	_, err := ep.send("type_cdp", map[string]interface{}{
		"text":      text,
		"elementId": elementID,
	})
	return err
}

// TypeCDPWithTabs presses Tab N times to focus, then inserts text via CDP.
func (ep *ExtensionPage) TypeCDPWithTabs(text string, tabCount int) error {
	_, err := ep.send("type_cdp", map[string]interface{}{
		"text":     text,
		"tabCount": tabCount,
	})
	return err
}

// InsertTextOnElement sends text to a specific element by ID.
func (ep *ExtensionPage) InsertTextOnElement(text string, elementID string) error {
	_, err := ep.send(CmdInsertText, map[string]interface{}{
		"text":      text,
		"elementId": elementID,
	})
	return err
}

func (ep *ExtensionPage) InsertText(text string) error {
	_, err := ep.send(CmdInsertText, map[string]interface{}{
		"text": text,
	})
	return err
}

// ---------------------------------------------------------------------------
// Mouse
// ---------------------------------------------------------------------------

func (ep *ExtensionPage) MouseScroll(x, y float64, steps int) error {
	_, err := ep.send(CmdScroll, map[string]interface{}{
		"x": x, "y": y, "steps": steps,
	})
	return err
}

// ---------------------------------------------------------------------------
// Eval
// ---------------------------------------------------------------------------

func (ep *ExtensionPage) Eval(js string, args ...interface{}) (*browser.EvalResult, error) {
	// For simple DOM queries, use content script commands that work reliably
	// without eval (bypassing CSP issues). The content script can read DOM
	// in its isolated world.
	// Falls back to eval command for complex JS.

	resp, err := ep.send(CmdEval, map[string]interface{}{
		"expression": js,
		"args":       args,
	})
	if err != nil {
		return browser.NewEvalResult(nil), nil // Return nil result, don't error — let caller handle
	}
	data := resp.Data
	if m := resp.dataMap(); m != nil {
		if r, ok := m["result"]; ok {
			data = r
		}
	}
	return browser.NewEvalResult(data), nil
}

// QueryCount counts elements matching a CSS selector via the content script.
func (ep *ExtensionPage) QueryCount(selector string) (int, error) {
	resp, err := ep.send("query_count", map[string]interface{}{
		"selector": selector,
	})
	if err != nil {
		return 0, err
	}
	if m := resp.dataMap(); m != nil {
		if r, ok := m["result"].(float64); ok {
			return int(r), nil
		}
	}
	return 0, nil
}

// QueryText gets text content of the last element matching a CSS selector.
func (ep *ExtensionPage) QueryText(selector string) (string, error) {
	resp, err := ep.send("query_text", map[string]interface{}{
		"selector": selector,
	})
	if err != nil {
		return "", err
	}
	if m := resp.dataMap(); m != nil {
		if r, ok := m["result"].(string); ok {
			return r, nil
		}
	}
	return "", nil
}

// FetchImageBase64 extracts images matching the selector via the content script.
func (ep *ExtensionPage) FetchImageBase64(selector string) ([]map[string]interface{}, error) {
	resp, err := ep.send("fetch_image_base64", map[string]interface{}{
		"selector": selector,
	})
	if err != nil {
		return nil, err
	}
	if m := resp.dataMap(); m != nil {
		if arr, ok := m["result"].([]interface{}); ok {
			var results []map[string]interface{}
			for _, item := range arr {
				if im, ok := item.(map[string]interface{}); ok {
					results = append(results, im)
				}
			}
			return results, nil
		}
	}
	return nil, fmt.Errorf("no images found")
}

// ---------------------------------------------------------------------------
// Cookies (no-op — the extension already has the browser's real cookies)
// ---------------------------------------------------------------------------

func (ep *ExtensionPage) SetCookies(_ interface{}) error    { return nil }
func (ep *ExtensionPage) GetCookies() (interface{}, error)   { return nil, nil }

// ---------------------------------------------------------------------------
// Timeout
// ---------------------------------------------------------------------------

// Timeout returns a new ExtensionPage whose operations are bounded by d.
func (ep *ExtensionPage) Timeout(d time.Duration) browser.PageInterface {
	return &ExtensionPage{
		server:  ep.server,
		tabID:   ep.tabID,
		timeout: d,
	}
}

// ---------------------------------------------------------------------------
// ExtensionElement
// ---------------------------------------------------------------------------

// ExtensionElement implements browser.ElementHandle by sending commands for a
// specific element (identified by elementID) to the Chrome Extension.
type ExtensionElement struct {
	server    *Server
	tabID     int
	elementID string
	timeout   time.Duration
}

// ElementID returns the content-script element reference ID.
func (ee *ExtensionElement) ElementID() string { return ee.elementID }

// Compile-time check.
var _ browser.ElementHandle = (*ExtensionElement)(nil)

func (ee *ExtensionElement) send(cmdType string, extra map[string]interface{}) (*Response, error) {
	params := map[string]interface{}{
		"elementId": ee.elementID,
	}
	for k, v := range extra {
		params[k] = v
	}
	return ee.server.SendCommand(&Command{
		Type:   cmdType,
		TabID:  ee.tabID,
		Params: params,
	}, ee.timeout)
}

func (ee *ExtensionElement) Click() error {
	_, err := ee.send(CmdClick, nil)
	return err
}

func (ee *ExtensionElement) Input(text string) error {
	_, err := ee.send(CmdInput, map[string]interface{}{"text": text})
	return err
}

func (ee *ExtensionElement) Text() (string, error) {
	resp, err := ee.send(CmdText, nil)
	if err != nil {
		return "", err
	}
	text, _ := resp.dataMap()["text"].(string)
	return text, nil
}

func (ee *ExtensionElement) Attribute(name string) (*string, error) {
	resp, err := ee.send(CmdAttribute, map[string]interface{}{"name": name})
	if err != nil {
		return nil, err
	}
	val, ok := resp.dataMap()["value"].(string)
	if !ok {
		return nil, nil
	}
	return &val, nil
}

func (ee *ExtensionElement) SetFiles(paths []string) error {
	// Read files on Go side and send base64 data to the extension.
	// Extensions can't read local files, so we read and encode here.
	type fileData struct {
		Name     string `json:"name"`
		Data     string `json:"data"`     // base64
		MimeType string `json:"mimeType"`
	}
	var files []fileData
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("set_files: read %s: %w", p, err)
		}
		b64 := base64.StdEncoding.EncodeToString(data)
		name := filepath.Base(p)
		mime := "application/octet-stream"
		switch {
		case strings.HasSuffix(name, ".png"):
			mime = "image/png"
		case strings.HasSuffix(name, ".jpg"), strings.HasSuffix(name, ".jpeg"):
			mime = "image/jpeg"
		case strings.HasSuffix(name, ".mp4"):
			mime = "video/mp4"
		}
		files = append(files, fileData{Name: name, Data: b64, MimeType: mime})
	}
	_, err := ee.send(CmdSetFiles, map[string]interface{}{"fileData": files})
	return err
}

func (ee *ExtensionElement) Focus() error {
	_, err := ee.send(CmdFocus, nil)
	return err
}

func (ee *ExtensionElement) ScrollIntoView() error {
	_, err := ee.send(CmdScrollIntoView, nil)
	return err
}

func (ee *ExtensionElement) WaitStable(d time.Duration) error {
	_, err := ee.send(CmdWaitElement, map[string]interface{}{
		"mode":    "stable",
		"timeout": d.Milliseconds(),
	})
	return err
}

func (ee *ExtensionElement) HTML() (string, error) {
	resp, err := ee.send(CmdHTML, nil)
	if err != nil {
		return "", err
	}
	html, _ := resp.dataMap()["html"].(string)
	return html, nil
}

func (ee *ExtensionElement) Property(name string) (interface{}, error) {
	resp, err := ee.send(CmdProperty, map[string]interface{}{"name": name})
	if err != nil {
		return nil, err
	}
	return resp.dataMap()["value"], nil
}
