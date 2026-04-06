// Package extension implements a WebSocket-based communication layer between
// the Go monoes-agent and a Chrome Extension. The extension acts as a browser
// bridge, executing DOM commands on real Chrome tabs that already have the
// user's session cookies.
package extension

// Command is sent from Go to the Chrome extension over WebSocket.
type Command struct {
	ID     string                 `json:"id"`
	Type   string                 `json:"type"`
	TabID  int                    `json:"tabId,omitempty"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// Response is received from the Chrome extension over WebSocket.
type Response struct {
	ID      string      `json:"id"`
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// Command type constants.
const (
	// Tab management
	CmdCreateTab = "create_tab"
	CmdNavigate  = "navigate"
	CmdReload    = "reload"
	CmdPageInfo  = "page_info"

	// Element queries
	CmdElement  = "element"
	CmdElements = "elements"
	CmdHas      = "has"

	// Element actions
	CmdClick          = "click"
	CmdInput          = "input"
	CmdText           = "text"
	CmdAttribute      = "attribute"
	CmdHTML           = "html"
	CmdProperty       = "property"
	CmdFocus          = "focus"
	CmdScrollIntoView = "scroll_into_view"
	CmdSetFiles       = "set_files"

	// Page-level input
	CmdScroll        = "scroll"
	CmdKeyboardType  = "keyboard_type"
	CmdKeyboardPress = "keyboard_press"
	CmdInsertText    = "insert_text"

	// JavaScript evaluation
	CmdEval = "eval"

	// Waiting
	CmdWaitLoad    = "wait_load"
	CmdWaitElement = "wait_element"
	CmdRace        = "race"
)
