# Mono Agent — Development Guide

All active development happens in this `newmonoes/` directory. The sibling `src/` directory is the legacy codebase — do NOT develop there.

## Project Overview

Mono Agent is a Go CLI tool for browser-based social media automation. It uses Rod (Chrome DevTools Protocol) to drive a real browser, executing actions defined in JSON files. Platforms: Instagram, LinkedIn, X (Twitter), TikTok, Telegram, Email.

## Architecture

```
newmonoes/
├── cmd/monoes/          CLI commands (Cobra)
├── data/
│   ├── actions/         JSON action definitions (embedded at compile time)
│   │   ├── instagram/   POST_LIKING.json, POST_COMMENTING.json, BULK_MESSAGING.json, ...
│   │   ├── linkedin/
│   │   ├── tiktok/
│   │   └── x/
│   ├── migrations/      SQL migrations
│   └── embed.go         go:embed directives for actions + migrations
├── internal/
│   ├── action/          Action executor engine
│   │   ├── loader.go    Loads action JSON from embedded FS
│   │   ├── executor.go  Executes actions (steps, loops, variables)
│   │   ├── steps.go     Step type implementations (navigate, click, type, call_bot_method, ...)
│   │   ├── runner.go    Action run orchestration
│   │   └── *_test.go    Integration tests (build tag: integration)
│   ├── bot/             Platform bot adapters
│   │   ├── adapter.go   BotAdapter interface + PlatformRegistry
│   │   ├── browser.go   BrowserPool management
│   │   ├── humanize.go  Human-like typing, scrolling, delays
│   │   ├── instagram/bot.go
│   │   ├── linkedin/bot.go
│   │   ├── x/bot.go
│   │   └── tiktok/bot.go
│   ├── config/          Config manager, schemas, API client
│   ├── storage/         SQLite database + file storage
│   ├── scheduler/       Cron-based action scheduling
│   └── util/            Helpers (SleepRandom, etc.)
├── go.mod               Module: github.com/monoes/mono-agent
└── Makefile             Build targets
```

## How Actions Work

### Execution Flow

1. User triggers an action (e.g., `POST_LIKING` on Instagram)
2. `ActionLoader` reads `data/actions/instagram/POST_LIKING.json` from embedded FS
3. `ActionExecutor` receives the action definition + a Rod page + variables
4. Executor runs loops over `selectedListItems`, executing steps for each item
5. Steps interact with the browser via Rod (navigate, click, type) or delegate to bot methods

### Two Action Patterns

#### Pattern 1: Pure JSON Steps (find_element + click + type)
Used when DOM selectors are stable. Steps define XPaths/CSS selectors directly in JSON.
Example: `POST_COMMENTING.json` — finds comment textarea, types, clicks Post button.

**Downside**: Fragile on platforms like Instagram that swap DOM elements and class names.

#### Pattern 2: `call_bot_method` (Preferred for complex interactions)
Delegates all DOM complexity to Go code. The JSON just says "call this method with these args."

```json
{
  "id": "like_post",
  "type": "call_bot_method",
  "methodName": "like_post",
  "args": ["{{item.url}}"],
  "variable_name": "likeResult",
  "timeout": 30
}
```

The Go method handles navigation, element discovery, retries, and verification internally.
**Use this pattern for any interaction where selectors are unreliable.**

#### Pattern 3: 3-Tier Fallback (Required for Instagram)
All Instagram actions use a 3-tier cascade for every DOM interaction:
1. **Tier 1** — Go bot method (`call_bot_method`, `onError: skip`)
2. **Tier 2** — Hardcoded XPath selectors (`find_element`, `onError: skip`)
3. **Tier 3** — AI-generated selectors via `configKey` (`find_element`, `onError: mark_failed`)

See `THREE_TIER_FALLBACK.md` for the full pattern, JSON template, and checklist.

### Variables

- `{{item.url}}` — current loop item's URL
- `{{item.platform}}` — current loop item's platform
- `{{messageText}}` — message text from action inputs
- `{{commentText}}` — comment text from action inputs
- `selectedListItems` — array of `{url, platform, status}` objects to iterate over

## Adding a New Action (Step-by-Step)

### 1. Create the Action JSON

File: `data/actions/<platform>/<ACTION_TYPE>.json`

```json
{
  "actionType": "ACTION_TYPE",
  "platform": "PLATFORM",
  "version": "1.0.0",
  "description": "What this action does",
  "metadata": { "requiresAuth": true, "supportsPagination": false, "supportsRetry": true },
  "inputs": { "required": ["selectedListItems"], "optional": [] },
  "outputs": { "success": ["count", "reachedIndex"], "failure": ["failedItems"] },
  "steps": [ ... ],
  "loops": [{
    "id": "process_items",
    "iterator": "selectedListItems",
    "indexVar": "reachedIndex",
    "steps": ["step1", "step2"],
    "onComplete": "update_action_state"
  }],
  "errorHandling": { "globalRetries": 3, "retryDelay": 2000, "onFinalFailure": "log_and_continue" }
}
```

### 2. Add the Bot Method (if using call_bot_method)

File: `internal/bot/<platform>/bot.go`

1. Add a method: `func (b *PlatformBot) DoSomething(ctx context.Context, page *rod.Page, arg string) error`
2. Expose it in `GetMethodByName`:

```go
case "do_something":
    return func(ctx context.Context, args ...interface{}) (interface{}, error) {
        page, ok := args[0].(*rod.Page)  // page is always auto-prepended by executor
        if !ok { return nil, fmt.Errorf("first arg must be *rod.Page") }
        arg, ok := args[1].(string)
        if !ok { return nil, fmt.Errorf("second arg must be string") }
        err := b.DoSomething(ctx, page, arg)
        if err != nil { return nil, err }
        return map[string]interface{}{"success": true}, nil
    }, true
```

### 3. Add Config Schema (for Tier 3 configKey)

File: `internal/config/schemas.go`

Add a schema with fields matching the `configKey` values used in Tier 3 steps:
```go
var instagramNewActionSchema = schema("new_action", "Elements for the new action",
    field("element_name", "Description of the element"),
)
```
Register it in the `schemas` map as `"INSTAGRAM_NEW_ACTION": instagramNewActionSchema`.

### 4. Write Action JSON with 3-Tier Cascade

Follow the template in `THREE_TIER_FALLBACK.md` — every DOM interaction must have all 3 tiers.

### 5. Add Integration Test

File: `internal/action/instagram_integration_test.go` (build tag: `integration`)

Follow the existing test pattern:
```go
func TestInstagramNewAction(t *testing.T) {
    // launchTestBrowser → newTestExecutor → SetVariable → Execute → printReport → assertions
}
```

Run: `go test -tags integration -run TestInstagramNewAction -v -timeout 3m ./internal/action/`

### 4. Verify

```bash
go build ./...                    # compile check
go vet -tags integration ./...    # vet check
```

## Instagram-Specific Lessons (Critical)

### DOM Structure (as of 2026)
- **No `<article>` element** on individual post pages — do NOT rely on it
- Post action bar (Like, Comment, Share, Save) is inside a `<section>`
- Comments have their own like buttons (NOT inside `<section>` elements)
- **Sections are NESTED**: outer section wraps comments + action bar, inner section is the action bar
- Always select the **innermost (last) matching section** when searching for the post action bar

### Element Identification Strategy
1. Use JavaScript to **identify** the correct element (scan sections, match by co-presence of multiple icon types)
2. Mark the element with a temporary `data-*` attribute
3. Use Rod's CSS selector to **find** the marked element
4. Use Rod's native `.Click(proto.InputMouseButtonLeft, 1)` to **click** it

**Never use JS `.click()`** — it does NOT trigger Instagram's React synthetic event handlers.

### Like vs Unlike (Avoid Toggle-Off)
- `svg[aria-label='Like']` and `svg[aria-label='Unlike']` exist for both posts AND comments
- The post's Like/Unlike is in the **innermost section** that also has Comment + Share + Save icons
- Comment Like/Unlike buttons are standalone (no section parent, or in the outer section)
- Always check the **innermost action section** for Unlike before clicking, to avoid un-liking

### Dismissing Dialogs
- Instagram shows "Turn on Notifications" dialog on first DM page visit
- Use `dismissNotificationDialog(page)` which clicks "Not Now"
- Call this after every navigation before interacting with elements

### Typing Text
- Use `page.Keyboard.Type()` for character input (not `element.Type()`)
- `element.Type()` inherits timeout context and can expire
- Add small delays between keystrokes for React's synthetic event processing

## Key Dependencies

- **Rod** v0.116.2 — browser automation via Chrome DevTools Protocol
- **Cobra** — CLI framework
- **SQLite** (modernc.org/sqlite) — pure Go, no CGO
- **Zerolog** — structured logging

## Build & Test

```bash
make build                        # build for current platform
go build ./...                    # compile check
go test ./...                     # unit tests
go test -tags integration -v -timeout 3m ./internal/action/   # integration tests (needs browser + session)
```

Integration tests require:
- Chrome installed
- Instagram session cookies in `~/.monoes/monoes.db` (run `monoes login instagram` first)
