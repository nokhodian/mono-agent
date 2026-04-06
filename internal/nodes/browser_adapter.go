package nodes

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/nokhodian/mono-agent/internal/action"
	"github.com/nokhodian/mono-agent/internal/browser"
	"github.com/nokhodian/mono-agent/internal/connections"
	"github.com/nokhodian/mono-agent/internal/workflow"
	"github.com/rs/zerolog"
)

// globalSessionProvider is the process-wide SessionProvider injected at startup.
var globalSessionProvider SessionProvider

// globalBotRegistry is the process-wide BotRegistry injected at startup.
var globalBotRegistry BotRegistry

// globalConfigMgr is the process-wide config manager for selector resolution.
var globalConfigMgr action.ConfigInterface

// globalCredentialStore allows BrowserNode to resolve credential_id → username.
var globalCredentialStore *connections.Store

// SessionProvider returns a browser page for a given platform and session username.
type SessionProvider interface {
	GetPage(ctx context.Context, platform string, username string) (browser.PageInterface, error)
}

// BotRegistry returns a BotAdapter for a given platform.
type BotRegistry interface {
	GetAdapter(platform string) (action.BotAdapter, bool)
}

// SetGlobalSessionProvider sets the session provider used by all BrowserNodes.
// Called once during engine startup.
func SetGlobalSessionProvider(sp SessionProvider) {
	globalSessionProvider = sp
}

// SetGlobalBotRegistry sets the bot registry used by all BrowserNodes.
// Called once during engine startup.
func SetGlobalBotRegistry(br BotRegistry) {
	globalBotRegistry = br
}

// SetGlobalConfigMgr sets the config manager used by all BrowserNodes for selector resolution.
func SetGlobalConfigMgr(cm action.ConfigInterface) {
	globalConfigMgr = cm
}

// SetGlobalCredentialStore sets the connections store used to resolve
// credential_id values to a platform username. Call once at startup.
func SetGlobalCredentialStore(cs *connections.Store) {
	globalCredentialStore = cs
}

// BrowserNode wraps the existing action.ActionExecutor to satisfy the workflow.NodeExecutor interface.
// This allows all existing platform actions (Instagram, LinkedIn, etc.) to be used as workflow nodes.
type BrowserNode struct {
	platform   string // "instagram", "linkedin", "tiktok", "x"
	actionType string // "BULK_FOLLOWING", "KEYWORD_SEARCH", etc.
}

// newNopLogger returns a zerolog.Logger that discards all output.
func newNopLogger() zerolog.Logger {
	return zerolog.New(io.Discard).With().Logger()
}

// NewBrowserNode creates a new BrowserNode for the given platform and action type.
func NewBrowserNode(platform, actionType string) *BrowserNode {
	return &BrowserNode{
		platform:   platform,
		actionType: actionType,
	}
}

// Type returns the node type string: "action.{platform}.{actionType}"
// e.g. "action.instagram.KEYWORD_SEARCH"
func (b *BrowserNode) Type() string {
	return fmt.Sprintf("action.%s.%s", b.platform, b.actionType)
}

// Execute runs the platform action against the bot's browser.
// It converts workflow Items to action parameters, runs ActionExecutor, and converts results back to Items.
//
// The SessionProvider and BotRegistry must be set via SetGlobalSessionProvider and SetGlobalBotRegistry
// before Execute is called.
func (b *BrowserNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	if globalSessionProvider == nil {
		return nil, fmt.Errorf("nodes: SessionProvider not set; call SetGlobalSessionProvider at startup")
	}

	// 1. Extract username from config.
	// If credential_id is set, resolve it via the connections store to get the
	// stored username. Fall back to the explicit "username" field, then "unknown".
	username, _ := config["username"].(string)
	if credID, ok := config["credential_id"].(string); ok && credID != "" && globalCredentialStore != nil {
		if conn, err := globalCredentialStore.Get(ctx, credID); err == nil && conn != nil {
			if u, _ := conn.Data["username"].(string); u != "" {
				username = u
			} else if conn.AccountID != "" {
				username = conn.AccountID
			}
		}
	}
	if username == "" {
		username = "unknown"
	}

	// 2. Get a session (browser page) via the SessionProvider.
	page, err := globalSessionProvider.GetPage(ctx, b.platform, username)
	if err != nil {
		return nil, fmt.Errorf("nodes: getting page for %s/%s: %w", b.platform, username, err)
	}

	// 3. Get the appropriate bot adapter via the BotRegistry (optional — not all platforms need it).
	var botAdapter action.BotAdapter
	if globalBotRegistry != nil {
		botAdapter, _ = globalBotRegistry.GetAdapter(b.platform)
	}

	// 4. Build a StorageAction from config fields.
	storageAction := &action.StorageAction{
		ID:             uuid.New().String(),
		Type:           b.actionType,
		TargetPlatform: b.platform,
	}

	if msg, ok := config["message"].(string); ok {
		storageAction.ContentMessage = msg
	}
	if kw, ok := config["keywords"].(string); ok {
		storageAction.Keywords = kw
	}

	// Collect selectedListItems from "targets".
	var selectedListItems []interface{}
	if targetsRaw, ok := config["targets"]; ok {
		if targets, ok := targetsRaw.([]interface{}); ok {
			selectedListItems = targets
		}
	}

	// Build a Params map from all remaining config keys (excluding reserved keys).
	reserved := map[string]struct{}{
		"username": {},
		"targets":  {},
		"message":  {},
		"keywords": {},
	}
	params := make(map[string]interface{})
	for k, v := range config {
		if _, skip := reserved[k]; !skip {
			params[k] = v
		}
	}
	// Seed session username so {{username}} resolves in actions that reference it.
	// If a "targetUsername" key is provided it overrides {{username}} in template context,
	// allowing callers to distinguish session identity from action target.
	params["username"] = username
	if targetU, ok := config["targetUsername"].(string); ok && targetU != "" {
		params["username"] = targetU
	}
	storageAction.Params = params

	// 5. Create ActionExecutor and call Execute.
	logger := newNopLogger()
	if os.Getenv("MONOES_DEBUG") != "" {
		logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
	}
	executor := action.NewActionExecutor(
		ctx,
		page,
		nil, // db: not used in workflow context (no state persistence needed here)
		globalConfigMgr,
		nil, // events: no external monitoring channel
		botAdapter,
		logger,
	)

	// Seed selectedListItems as a variable so loops over target lists work.
	if len(selectedListItems) > 0 {
		executor.SetVariable("selectedListItems", selectedListItems)
	}

	result, err := executor.Execute(storageAction)
	if err != nil {
		return nil, fmt.Errorf("nodes: BrowserNode execute %s/%s: %w", b.platform, b.actionType, err)
	}

	// 6. Convert and normalize ExtractedItems to []workflow.Item.
	items := make([]workflow.Item, 0, len(result.ExtractedItems))
	for _, raw := range result.ExtractedItems {
		items = append(items, workflow.NewItem(normalizeBrowserItem(raw, b.platform)))
	}

	// When the action produced no extracted items (e.g. publish actions that
	// don't scrape data), pass the input items through so downstream nodes
	// that need fields like _row_range or _row_index continue to work.
	if len(items) == 0 && len(input.Items) > 0 {
		items = input.Items
	}

	return []workflow.NodeOutput{
		{Handle: "main", Items: items},
	}, nil
}

// normalizeBrowserItem enriches a raw extracted item with structured fields.
//
// stepExtractMultiple produces items with generic keys: "text" (visible text of
// the DOM element) and "href" (the element's href attribute if present).
// This function maps those to the canonical people fields that people.save
// and SaveExtractedData expect.
func normalizeBrowserItem(raw map[string]interface{}, platform string) map[string]interface{} {
	out := make(map[string]interface{}, len(raw)+4)
	for k, v := range raw {
		out[k] = v
	}

	// Canonical platform field (lowercase).
	if _, exists := out["platform"]; !exists {
		out["platform"] = strings.ToLower(platform)
	}

	// Map href → profile_url (and url for SaveExtractedData compatibility).
	if href, ok := raw["href"].(string); ok && href != "" {
		if _, exists := out["profile_url"]; !exists {
			out["profile_url"] = href
		}
		if _, exists := out["url"]; !exists {
			out["url"] = href
		}
	}

	// Split text into full_name and job_title.
	// LinkedIn result cards include noise lines before the actual job title
	// (e.g. "View X's profile", "• 2nd", "2nd degree connection").
	// Scan past those to find the real professional headline.
	if text, ok := raw["text"].(string); ok && text != "" {
		lines := strings.Split(strings.TrimSpace(text), "\n")
		if _, exists := out["full_name"]; !exists && len(lines) > 0 {
			out["full_name"] = strings.TrimSpace(lines[0])
		}
		if _, exists := out["job_title"]; !exists {
			name, _ := out["full_name"].(string)
			for _, line := range lines[1:] {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				// Skip accessibility/noise lines common in LinkedIn cards.
				lower := strings.ToLower(line)
				if strings.HasPrefix(lower, "view ") && strings.Contains(lower, "profile") {
					continue
				}
				if strings.HasPrefix(line, "• ") || line == "Connect" || line == "Follow" ||
					line == "Message" || line == "InMail" || strings.HasPrefix(line, "Visit my") {
					continue
				}
				if strings.Contains(lower, "degree connection") || strings.Contains(lower, "mutual connection") {
					continue
				}
				if name != "" && strings.Contains(line, name) {
					continue
				}
				out["job_title"] = line
				break
			}
		}
	}

	return out
}
