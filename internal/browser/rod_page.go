package browser

import (
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	"github.com/ysmood/gson"
)

// Compile-time interface checks.
var (
	_ PageInterface = (*RodPage)(nil)
	_ ElementHandle = (*RodElement)(nil)
)

// ---------------------------------------------------------------------------
// RodPage — PageInterface backed by *rod.Page
// ---------------------------------------------------------------------------

// RodPage wraps a Rod *Page and implements PageInterface. Each method
// delegates directly to the underlying Rod API so the wrapper is as thin as
// possible. The optional timeout field is applied via Page.Timeout when set.
type RodPage struct {
	page    *rod.Page
	timeout time.Duration // zero means "no override"
}

// NewRodPage creates a RodPage from an existing Rod page.
func NewRodPage(page *rod.Page) *RodPage {
	return &RodPage{page: page}
}

// UnwrapRodPage returns the underlying *rod.Page. This escape-hatch is
// provided for callers that need direct Rod access during the migration period.
func (rp *RodPage) UnwrapRodPage() *rod.Page {
	return rp.page
}

// effective returns the Rod page with the configured timeout applied, or the
// bare page if no timeout override has been set.
func (rp *RodPage) effective() *rod.Page {
	if rp.timeout > 0 {
		return rp.page.Timeout(rp.timeout)
	}
	return rp.page
}

// -- Navigation --------------------------------------------------------------

func (rp *RodPage) Navigate(url string) error {
	return rp.effective().Navigate(url)
}

func (rp *RodPage) WaitLoad() error {
	return rp.effective().WaitLoad()
}

func (rp *RodPage) WaitDOMStable(timeout time.Duration) error {
	// Rod's WaitDOMStable(d, diff) checks that the DOM change is <= diff%
	// over interval d. We use the provided timeout as both the page-level
	// deadline and the stability check interval, with 0 diff (exact match).
	return rp.page.Timeout(timeout).WaitDOMStable(timeout, 0)
}

func (rp *RodPage) WaitIdle(timeout time.Duration) error {
	return rp.page.Timeout(timeout).WaitIdle(timeout)
}

func (rp *RodPage) Reload() error {
	return rp.effective().Reload()
}

func (rp *RodPage) GetURL() (string, error) {
	info, err := rp.effective().Info()
	if err != nil {
		return "", fmt.Errorf("get page URL: %w", err)
	}
	return info.URL, nil
}

// -- Element queries ---------------------------------------------------------

func (rp *RodPage) Element(selector string, timeout time.Duration) (ElementHandle, error) {
	el, err := rp.page.Timeout(timeout).Element(selector)
	if err != nil {
		return nil, err
	}
	return NewRodElement(el), nil
}

func (rp *RodPage) ElementX(xpath string, timeout time.Duration) (ElementHandle, error) {
	el, err := rp.page.Timeout(timeout).ElementX(xpath)
	if err != nil {
		return nil, err
	}
	return NewRodElement(el), nil
}

func (rp *RodPage) Elements(selector string) ([]ElementHandle, error) {
	elems, err := rp.effective().Elements(selector)
	if err != nil {
		return nil, err
	}
	handles := make([]ElementHandle, len(elems))
	for i, el := range elems {
		handles[i] = NewRodElement(el)
	}
	return handles, nil
}

func (rp *RodPage) Has(selector string) (bool, error) {
	has, _, err := rp.effective().Has(selector)
	return has, err
}

func (rp *RodPage) Race(selectors []string, timeout time.Duration) (int, ElementHandle, error) {
	if len(selectors) == 0 {
		return -1, nil, fmt.Errorf("Race: no selectors provided")
	}

	timedPage := rp.page.Timeout(timeout)
	race := timedPage.Race()
	for _, sel := range selectors {
		race = race.Element(sel)
	}

	el, err := race.Do()
	if err != nil {
		return -1, nil, err
	}

	// Determine which selector won by testing Matches on the returned element.
	for i, sel := range selectors {
		matched, mErr := el.Matches(sel)
		if mErr == nil && matched {
			return i, NewRodElement(el), nil
		}
	}

	// Fallback — return the element with index -1 (should not normally happen).
	return -1, NewRodElement(el), nil
}

// -- Keyboard / input --------------------------------------------------------

func (rp *RodPage) KeyboardType(keys ...rune) error {
	for _, ch := range keys {
		if err := rp.effective().Keyboard.Type(input.Key(ch)); err != nil {
			return fmt.Errorf("keyboard type rune %q: %w", ch, err)
		}
	}
	return nil
}

func (rp *RodPage) KeyboardPress(key rune) error {
	return rp.effective().Keyboard.Press(input.Key(key))
}

func (rp *RodPage) InsertText(text string) error {
	return rp.effective().InsertText(text)
}

// -- Mouse -------------------------------------------------------------------

func (rp *RodPage) MouseScroll(x, y float64, steps int) error {
	return rp.effective().Mouse.Scroll(x, y, steps)
}

// -- JavaScript evaluation ---------------------------------------------------

func (rp *RodPage) Eval(js string, args ...interface{}) (*EvalResult, error) {
	res, err := rp.effective().Eval(js, args...)
	if err != nil {
		return nil, err
	}
	return newRodEvalResult(res.Value), nil
}

// -- Cookies -----------------------------------------------------------------

func (rp *RodPage) SetCookies(cookies interface{}) error {
	// Expect []*proto.NetworkCookieParam from Rod callers.
	params, ok := cookies.([]*proto.NetworkCookieParam)
	if !ok {
		return fmt.Errorf("SetCookies: expected []*proto.NetworkCookieParam, got %T", cookies)
	}
	return rp.effective().SetCookies(params)
}

func (rp *RodPage) GetCookies() (interface{}, error) {
	cookies, err := rp.effective().Cookies(nil)
	if err != nil {
		return nil, err
	}
	return cookies, nil
}

// -- Timeout -----------------------------------------------------------------

func (rp *RodPage) Timeout(d time.Duration) PageInterface {
	return &RodPage{
		page:    rp.page,
		timeout: d,
	}
}

// ---------------------------------------------------------------------------
// RodElement — ElementHandle backed by *rod.Element
// ---------------------------------------------------------------------------

// RodElement wraps a Rod *Element and implements ElementHandle.
type RodElement struct {
	elem *rod.Element
}

// NewRodElement creates a RodElement from an existing Rod element.
func NewRodElement(elem *rod.Element) *RodElement {
	return &RodElement{elem: elem}
}

// UnwrapRodElement returns the underlying *rod.Element for callers that need
// direct Rod access during the migration period.
func (re *RodElement) UnwrapRodElement() *rod.Element {
	return re.elem
}

func (re *RodElement) Click() error {
	return re.elem.Click(proto.InputMouseButtonLeft, 1)
}

func (re *RodElement) Input(text string) error {
	return re.elem.Input(text)
}

func (re *RodElement) Text() (string, error) {
	return re.elem.Text()
}

func (re *RodElement) Attribute(name string) (*string, error) {
	return re.elem.Attribute(name)
}

func (re *RodElement) SetFiles(paths []string) error {
	return re.elem.SetFiles(paths)
}

func (re *RodElement) Focus() error {
	return re.elem.Focus()
}

func (re *RodElement) ScrollIntoView() error {
	return re.elem.ScrollIntoView()
}

func (re *RodElement) WaitStable(d time.Duration) error {
	return re.elem.WaitStable(d)
}

func (re *RodElement) HTML() (string, error) {
	return re.elem.HTML()
}

func (re *RodElement) Property(name string) (interface{}, error) {
	val, err := re.elem.Property(name)
	if err != nil {
		return nil, err
	}
	return val.Val(), nil
}

// ---------------------------------------------------------------------------
// rodEvalResult — EvalResult backed by gson.JSON
// ---------------------------------------------------------------------------

// newRodEvalResult creates an EvalResult wrapping a gson.JSON value.
func newRodEvalResult(v gson.JSON) *EvalResult {
	return &EvalResult{raw: v}
}

// asGson extracts the underlying gson.JSON. Panics if the raw value is not a
// gson.JSON (programming error — should only happen if mixed with a non-Rod
// implementation).
func (er *EvalResult) asGson() gson.JSON {
	if er == nil {
		return gson.New(nil)
	}
	v, ok := er.raw.(gson.JSON)
	if !ok {
		return gson.New(er.raw)
	}
	return v
}

// Str returns the result as a string (mirrors gson.JSON.Str).
func (er *EvalResult) Str() string {
	return er.asGson().Str()
}

// Int returns the result as an int (mirrors gson.JSON.Int).
func (er *EvalResult) Int() int {
	return er.asGson().Int()
}

// Bool returns the result as a bool (mirrors gson.JSON.Bool).
func (er *EvalResult) Bool() bool {
	return er.asGson().Bool()
}

// Get navigates into a nested value by key path (mirrors gson.JSON.Get).
func (er *EvalResult) Get(path string) *EvalResult {
	return newRodEvalResult(er.asGson().Get(path))
}

// Arr returns the result as a slice of EvalResults (mirrors gson.JSON.Arr).
func (er *EvalResult) Arr() []*EvalResult {
	arr := er.asGson().Arr()
	results := make([]*EvalResult, len(arr))
	for i, item := range arr {
		results[i] = newRodEvalResult(item)
	}
	return results
}

// Nil returns true when the underlying value is nil / JSON null.
func (er *EvalResult) Nil() bool {
	return er.asGson().Nil()
}
