// Package browser provides abstractions over browser page and element
// interactions. The interfaces defined here decouple automation logic from a
// specific browser-driving library (Rod, Chrome Extension, etc.), enabling the
// same action executor to operate in both headful Rod mode and extension mode.
package browser

import "time"

// PageInterface abstracts every browser page operation used by the monoes
// action executor and bot adapters. A concrete implementation wraps either a
// Rod *Page or a Chrome Extension messaging bridge.
type PageInterface interface {
	// Navigation

	Navigate(url string) error
	WaitLoad() error
	WaitDOMStable(timeout time.Duration) error
	WaitIdle(timeout time.Duration) error
	Reload() error
	GetURL() (string, error)

	// Element queries

	Element(selector string, timeout time.Duration) (ElementHandle, error)
	ElementX(xpath string, timeout time.Duration) (ElementHandle, error)
	Elements(selector string) ([]ElementHandle, error)
	Has(selector string) (bool, error)
	Race(selectors []string, timeout time.Duration) (int, ElementHandle, error)

	// Keyboard / input

	KeyboardType(keys ...rune) error
	KeyboardPress(key rune) error
	InsertText(text string) error

	// Mouse

	MouseScroll(x, y float64, steps int) error

	// JavaScript evaluation
	//
	// Eval executes a JavaScript expression on the page and returns a wrapped
	// result. The returned *EvalResult exposes helpers (Str, Bool, Int, Get,
	// Arr, Nil) that mirror the gson.JSON API used throughout the codebase.
	Eval(js string, args ...interface{}) (*EvalResult, error)

	// Cookies

	SetCookies(cookies interface{}) error
	GetCookies() (interface{}, error)

	// Timeout returns a new PageInterface whose subsequent operations are
	// bounded by d. This mirrors Rod's Page.Timeout pattern.
	Timeout(d time.Duration) PageInterface
}

// ElementHandle abstracts a single DOM element on the page.
type ElementHandle interface {
	Click() error
	Input(text string) error
	Text() (string, error)
	Attribute(name string) (*string, error)
	SetFiles(paths []string) error
	Focus() error
	ScrollIntoView() error
	WaitStable(d time.Duration) error
	HTML() (string, error)
	Property(name string) (interface{}, error)
}

// EvalResult wraps the value returned by PageInterface.Eval. It provides
// accessor methods that work identically regardless of whether the underlying
// driver is Rod (gson.JSON) or a Chrome Extension (map/JSON).
type EvalResult struct {
	raw interface{} // gson.JSON for Rod, map[string]interface{} for extension
}

// NewEvalResult constructs an EvalResult from a raw driver-specific value.
func NewEvalResult(raw interface{}) *EvalResult {
	return &EvalResult{raw: raw}
}

// Raw returns the underlying driver-specific value for callers that need direct
// access (e.g. to pass through to driver-specific helpers).
func (er *EvalResult) Raw() interface{} {
	return er.raw
}
