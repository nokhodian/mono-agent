# Mono Agent — Complete Feature Specification

> **Purpose:** This document is a comprehensive machine-readable reference of every feature, pattern, and specification in the Mono Agent codebase. It is intended for use by AI agents that need to understand, replicate, or extend any part of this system. Every feature includes precise file paths, function signatures, data structures, and behavioral specifications.

---

## Table of Contents

1. [System Overview](#1-system-overview)
2. [Core Action Engine](#2-core-action-engine)
3. [All Step Types](#3-all-step-types)
4. [Variable & Template System](#4-variable--template-system)
5. [Error Handling & Control Flow](#5-error-handling--control-flow)
6. [Bot Adapter Framework](#6-bot-adapter-framework)
7. [Browser Pool & Anti-Detection](#7-browser-pool--anti-detection)
8. [Humanization System](#8-humanization-system)
9. [3-Tier Fallback Pattern](#9-3-tier-fallback-pattern)
10. [Platform: Instagram Features](#10-platform-instagram-features)
11. [Platform: LinkedIn Features](#11-platform-linkedin-features)
12. [Platform: X (Twitter) Features](#12-platform-x-twitter-features)
13. [Platform: TikTok Features](#13-platform-tiktok-features)
14. [Platform: Telegram Features](#14-platform-telegram-features)
15. [Storage & Database](#15-storage--database)
16. [Scheduler](#16-scheduler)
17. [Config Manager (AI-Powered)](#17-config-manager-ai-powered)
18. [Authentication & Sessions](#18-authentication--sessions)
19. [CLI Interface](#19-cli-interface)
20. [REST API (FastAPI)](#20-rest-api-fastapi)
21. [Action Runner (Concurrency)](#21-action-runner-concurrency)
22. [Utility Systems](#22-utility-systems)

---

## 1. System Overview

**Project:** Mono Agent
**Module:** `github.com/monoes/mono-agent`
**Language:** Go 1.21+
**Root Directory:** `newmonoes/`
**Key Libraries:**
- `github.com/go-rod/rod v0.116.2` — Chrome DevTools Protocol browser automation
- `github.com/spf13/cobra` — CLI framework
- `modernc.org/sqlite` — Pure Go SQLite (no CGO)
- `github.com/rs/zerolog` — Structured JSON logging
- `github.com/robfig/cron/v3` — Cron scheduler
- `github.com/nicholasgasior/datefmt` — Date formatting

**Platforms Supported:** Instagram (15 actions), LinkedIn (7), X/Twitter (7), TikTok (7), Telegram (bot only), Email (bot only)

**Architecture Summary:**
- JSON action definitions drive all automation (embedded in binary at compile time)
- An `ActionExecutor` interprets JSON steps against a live Chrome browser page (via Rod)
- Each platform implements a `BotAdapter` interface with Go methods
- 3-tier fallback pattern: Go method → hardcoded XPath → AI-generated selector
- SQLite database stores sessions, actions, contacts, lists, threads, templates
- Cron scheduler triggers recurring actions
- External LLM API at `http://apiv1.monoes.me` generates selectors on demand

---

## 2. Core Action Engine

### 2.1 Action Definition Structure

**File:** `internal/action/loader.go`

```go
type ActionDef struct {
    ActionType  string                 // e.g. "POST_LIKING"
    Platform    string                 // e.g. "instagram"
    Version     string                 // e.g. "1.0.0"
    Description string
    Metadata    map[string]interface{} // requiresAuth, supportsPagination, supportsRetry
    Inputs      *InputDef              // required/optional field names
    Outputs     map[string][]string    // success/failure output field names
    Steps       []StepDef              // flat list of all steps (including sub-steps)
    Loops       []LoopDef              // loop definitions
    ErrorConfig *GlobalErrorConfig     // globalRetries, retryDelay, onFinalFailure
}

type InputDef struct {
    Required []string  // e.g. ["selectedListItems"]
    Optional []string  // e.g. ["messageText"]
}

type LoopDef struct {
    ID         string      // unique ID
    Iterator   string      // variable name to iterate (e.g. "selectedListItems")
    IndexVar   string      // variable to store current index (e.g. "reachedIndex")
    Steps      []string    // ordered step IDs to execute each iteration
    OnComplete interface{} // "update_action_state" or struct
}

type GlobalErrorConfig struct {
    GlobalRetries int    // default: 3
    RetryDelay    int    // milliseconds
    OnFinalFailure string // "log_and_continue"
}
```

**JSON File Locations:** `data/actions/<platform>/<ACTION_TYPE>.json`

**Embedded at Compile Time:** `data/embed.go` using `//go:embed` directives

**Singleton Cache:** `ActionLoader` caches parsed `ActionDef` objects by `platform/ActionType` key

**Loading API:**
```go
loader := action.NewActionLoader()
def, err := loader.Load("instagram", "POST_LIKING")
// Reads: data/actions/instagram/POST_LIKING.json
```

### 2.2 ActionExecutor

**File:** `internal/action/executor.go`

```go
type ActionExecutor struct {
    loader    *ActionLoader
    db        StorageInterface
    configMgr ConfigInterface
    logger    zerolog.Logger
}

func NewActionExecutor(loader *ActionLoader, db StorageInterface, configMgr ConfigInterface, logger zerolog.Logger) *ActionExecutor

func (e *ActionExecutor) Execute(
    ctx context.Context,
    action StorageAction,
    page *rod.Page,
    botAdapter BotAdapter,
) (*ExecutionContext, error)
```

**Execution Phases:**
1. Load action definition via `ActionLoader`
2. Create `ExecutionContext`, seed initial variables from `StorageAction`
3. Execute initial steps (non-loop steps from `Steps[]` not in any loop)
4. Identify loops from `Loops[]`
5. For each loop, iterate items from `Variables[loop.Iterator]`
6. Per iteration: set `item`, `loopIndex`, `loopTotal`, `indexVar` variables, execute loop steps
7. After each loop iteration, persist `reachedIndex` via `db.UpdateActionReachedIndex()`

**StorageAction fields consumed:**
```go
type StorageAction struct {
    ID              string
    Type            string   // ActionType
    TargetPlatform  string
    ReachedIndex    int      // Resume point
    ContentMessage  string   // → messageText, commentText, replyText, text
    ContentSubject  string   // → messageSubject, contentSubject
    Keywords        string   // → keyword, keywords, keywordEncoded
    ContentBlobURLs []string // → contentBlobUrls
    Params          map[string]string // arbitrary extra vars
}
```

### 2.3 ExecutionContext

**File:** `internal/action/executor.go`

```go
type ExecutionContext struct {
    mu           sync.RWMutex
    Variables    map[string]interface{}
    Elements     map[string]*rod.Element     // elementRef name → element
    StepResults  map[string]*StepResult      // step ID → result
    ExtractedItems []map[string]interface{}  // for save_data
    FailedItems  []FailedItem
    Progress     int
    Data         map[string]interface{}      // generic KV store
}

type StepResult struct {
    Success bool
    Data    interface{}
    Error   error
    Skip    bool
    Abort   bool
    Retry   bool
}

type FailedItem struct {
    StepID    string
    Error     error
    Timestamp time.Time
}
```

**Thread Safety:** All reads/writes use `sync.RWMutex`

### 2.4 StepDef (Complete Field Reference)

**File:** `internal/action/executor.go`

```go
type StepDef struct {
    // Identity
    ID          string  // unique step identifier
    Type        string  // step type (see Section 3)
    Description string  // human-readable description

    // Navigation
    URL     string  // URL to navigate to (supports templates)
    WaitFor string  // CSS selector to wait for after navigation
    WaitAfter string // CSS selector to wait for after action

    // Element finding
    Selector     string   // CSS selector
    XPath        string   // XPath expression
    ConfigKey    string   // key in config schema (triggers AI lookup)
    Alternatives []string // fallback selectors
    ElementRef   string   // reference to previously found element

    // Interaction
    Text      string      // text to type
    Value     interface{} // generic value
    Attribute string      // attribute name to extract
    Direction string      // scroll direction: "down", "up", "left", "right"
    Duration  interface{} // wait duration (ms or template)
    HumanLike bool        // use humanize typing

    // Conditional
    Condition interface{} // ConditionDef or bool
    Then      []string    // step IDs to execute if true
    Else      []string    // step IDs to execute if false

    // Race conditions
    RaceSelectors map[string]string // label → CSS selector

    // Bot method delegation
    MethodName string        // method name in GetMethodByName
    Method     string        // alias for MethodName
    Args       []interface{} // arguments (page auto-prepended)

    // Variable storage
    Variable     string // alias for VariableName
    VariableName string // store result here

    // Timeout
    Timeout float64 // seconds

    // Error/Success handling
    OnError   *ErrorHandlerDef
    OnSuccess *SuccessAction

    // Data management
    Set        map[string]interface{} // set variables directly
    DataSource string
    Increment  string
    BatchSize  int
}
```

---

## 3. All Step Types

**File:** `internal/action/steps.go`

Step type strings are matched in a dispatch map in `executor.go`. All handlers have signature:
```go
func(ctx context.Context, step StepDef) (*StepResult, error)
```

### 3.1 `navigate`

Navigate browser to a URL.

```json
{
  "id": "goto_profile",
  "type": "navigate",
  "url": "https://instagram.com/{{item.username}}",
  "waitFor": "main[role='main']",
  "timeout": 30
}
```

**Behavior:**
- Resolves `url` template
- Calls `page.Navigate(url)`
- If `waitFor` set: waits for CSS selector to appear
- If `waitAfter` set: additional selector wait post-navigation
- Timeout in seconds (default 30)
- On error: returns `StepResult{Success: false, Error: err}`

### 3.2 `wait`

Wait for a CSS selector to appear or a fixed duration.

```json
{
  "id": "wait_load",
  "type": "wait",
  "waitFor": "div.loaded",
  "duration": 2000,
  "timeout": 10
}
```

**Behavior:**
- If `waitFor`: `page.Race().Element(selector).Do(ctx)` with timeout
- If `duration`: `time.Sleep(duration * time.Millisecond)`
- Both can be set (duration first, then selector wait)

### 3.3 `find_element`

Find a DOM element and store it for later use.

```json
{
  "id": "find_btn",
  "type": "find_element",
  "xpath": "//button[@aria-label='Like']",
  "alternatives": ["//button[contains(@class,'like')]"],
  "variable_name": "likeBtn",
  "timeout": 10,
  "onError": { "action": "skip" }
}
```

**Resolution Priority:**
1. `configKey` → resolve via ConfigManager (AI-generated selector)
2. `selector` (CSS)
3. `xpath`
4. `alternatives[]` tried in order

**Storage:**
- Stores element in `ExecutionContext.Elements[step.ID]`
- Stores element in `ExecutionContext.Variables[variable_name]` if set

**ConfigKey Resolution:**
```
configKey → platform + actionType → schema lookup → ConfigManager.GetConfig()
```

### 3.4 `click`

Click a previously found or freshly located element.

```json
{
  "id": "click_like",
  "type": "click",
  "elementRef": "find_btn"
}
```

**Behavior:**
- If `elementRef` set: retrieves element from `Elements[elementRef]`
- If `selector`/`xpath` set: finds element first, then clicks
- Uses Rod's `elem.Click(proto.InputMouseButtonLeft, 1)` (NOT JS `.click()`)
- Scroll element into view before clicking

### 3.5 `type`

Type text into a focused element.

```json
{
  "id": "enter_comment",
  "type": "type",
  "text": "{{commentText}}",
  "human_like": true,
  "elementRef": "comment_input"
}
```

**Behavior:**
- If `human_like`: delegates to `WriteHumanLike()` (see Section 8)
- Else: uses `elem.Input(text)` or `page.Keyboard.Type()`
- Resolves `text` template before typing

### 3.6 `scroll`

Scroll the page or an element.

```json
{
  "id": "scroll_down",
  "type": "scroll",
  "direction": "down",
  "duration": 500
}
```

**Behavior:**
- Directions: `"down"`, `"up"`, `"left"`, `"right"`
- Uses `page.Mouse.Scroll()` with pixel amounts
- Duration in milliseconds controls total scroll distance
- Adds random pauses for human-like behavior

### 3.7 `extract_text`

Extract text content from a DOM element.

```json
{
  "id": "get_username",
  "type": "extract_text",
  "selector": "h2.username",
  "variable_name": "extracted_username"
}
```

**Behavior:**
- Finds element via `selector` or `xpath`
- Calls `elem.Text()`
- Stores in `Variables[variable_name]`
- Also stores in `StepResults[step.ID].Data`

### 3.8 `extract_attribute`

Extract a specific HTML attribute from an element.

```json
{
  "id": "get_href",
  "type": "extract_attribute",
  "selector": "a.profile-link",
  "attribute": "href",
  "variable_name": "profileURL"
}
```

**Behavior:**
- Finds element
- Calls `elem.Attribute(attribute_name)`
- Stores in `Variables[variable_name]`

### 3.9 `extract_multiple`

Extract data from all elements matching a selector, storing an array.

```json
{
  "id": "get_all_posts",
  "type": "extract_multiple",
  "selector": "article a[href]",
  "attribute": "href",
  "variable_name": "post_urls"
}
```

**Behavior:**
- Queries all elements matching `selector`
- For each element: extracts `attribute` or text content
- Builds `[]interface{}` array
- Stores in `Variables[variable_name]`

### 3.10 `condition`

Conditional branching based on variable state.

```json
{
  "id": "check_success",
  "type": "condition",
  "condition": {
    "variable": "t1Result",
    "operator": "not_exists"
  },
  "then": ["tier2_find", "tier2_click"],
  "else": ["log_success"]
}
```

**ConditionDef:**
```go
type ConditionDef struct {
    Variable string      // variable name to evaluate
    Operator string      // comparison operator
    Value    interface{} // comparison value
}
```

**Operators:**
- `"exists"` — variable is non-nil
- `"not_exists"` — variable is nil
- `"equals"` — variable == value
- `"not_equals"` — variable != value
- `"greater_than"` — variable > value
- `"less_than"` — variable < value
- `"contains"` — string contains value
- `"not_contains"` — string does not contain value

**Then/Else:** Arrays of step IDs. Steps executed inline (not looped).

### 3.11 `call_bot_method`

Delegate complex interaction to a Go bot method.

```json
{
  "id": "like_post_t1",
  "type": "call_bot_method",
  "methodName": "like_post",
  "args": ["{{item.url}}"],
  "variable_name": "likeResult",
  "timeout": 30,
  "onError": { "action": "skip" }
}
```

**Behavior:**
1. Resolve `methodName` template
2. Resolve all `args` templates
3. Auto-prepend `*rod.Page` as `args[0]`
4. Call `botAdapter.GetMethodByName(methodName)` → get function
5. Call function with `(ctx, page, ...resolvedArgs)`
6. Store result in `Variables[variable_name]`
7. On method not found: `StepResult{Success: false, Error: "method not found"}`

### 3.12 `update_progress`

Increment a progress counter variable.

```json
{
  "id": "increment_count",
  "type": "update_progress",
  "variable": "successCount",
  "increment": "successCount"
}
```

**Behavior:**
- Gets current value of `Variables[increment]`
- Increments by 1
- Stores back in `Variables[variable]`

### 3.13 `save_data`

Persist extracted data to the database.

```json
{
  "id": "save_results",
  "type": "save_data",
  "data_source": "extractedItems"
}
```

**Behavior:**
- Calls `db.SaveExtractedData(actionID, execCtx.ExtractedItems)`
- Clears `ExtractedItems` after save
- Used for follower lists, profile data, etc.

### 3.14 `mark_failed`

Mark the current item as failed and continue.

```json
{
  "id": "record_failure",
  "type": "mark_failed"
}
```

**Behavior:**
- Adds entry to `ExecutionContext.FailedItems`
- Records `item`, step ID, error, timestamp
- Does NOT stop execution (continues to next loop item)

### 3.15 `loop`

Inline loop over a variable (rare; most loops are defined in `loops[]`).

```json
{
  "id": "process_list",
  "type": "loop",
  "iterator": "{{someList}}",
  "steps": ["step_a", "step_b"]
}
```

### 3.16 `log`

Emit a structured log message.

```json
{
  "id": "log_done",
  "type": "log",
  "value": "Completed processing {{item.url}}"
}
```

**Behavior:**
- Resolves `value` template
- Emits via `zerolog` logger at INFO level
- Always succeeds (never returns error)

### 3.17 `set_variable`

Directly set a variable in ExecutionContext.

```json
{
  "id": "init_counter",
  "type": "set_variable",
  "set": {
    "successCount": 0,
    "errorCount": 0
  }
}
```

**Behavior:**
- Iterates `set` map
- Resolves each value as template
- Sets in `Variables`

### 3.18 `race_wait`

Wait for one of multiple selectors to appear first.

```json
{
  "id": "wait_outcome",
  "type": "race_wait",
  "race_selectors": {
    "success": "div.success-message",
    "error": "div.error-message",
    "loading": "div.loading-spinner"
  },
  "variable_name": "outcome",
  "timeout": 15
}
```

**Behavior:**
- Races all selectors simultaneously via goroutines
- Returns label of first matching selector
- Stores label in `Variables[variable_name]`
- Used for detecting modal states, confirmation dialogs

---

## 4. Variable & Template System

**File:** `internal/action/variables.go`

### 4.1 VariableResolver

```go
type VariableResolver struct {
    ctx *ExecutionContext
}

func NewVariableResolver(ctx *ExecutionContext) *VariableResolver
```

### 4.2 Template Syntax

Pattern: `{{variable.path}}`
Regex: `\{\{([^}]+)\}\}`

**Simple Variables:**
- `{{messageText}}` → `Variables["messageText"]`
- `{{item}}` → `Variables["item"]`

**Nested Access:**
- `{{item.url}}` → `Variables["item"].(map)["url"]`
- `{{item[0]}}` → `Variables["item"].([]interface{})[0]`
- `{{item[0].url}}` → Combined array + map access

**Step Results:**
- `{{step_id.data}}` → `StepResults["step_id"].Data`
- `{{step_id.count}}` → `len(StepResults["step_id"].Data)`
- `{{step_id.success}}` → `StepResults["step_id"].Success`
- `{{step_id.error}}` → `StepResults["step_id"].Error.Error()`

**Data Map:**
- `{{data.key}}` → `Data["key"]`

**Or / Fallback Chain:**
- `{{field1 or field2 or "default"}}` → first non-nil, non-empty value
- Supports literals: `"text"`, `123`, `true`, `false`

### 4.3 Resolve Methods

```go
// String interpolation (returns string, preserves surrounding text)
Resolve(template string) string
// Example: "Visit {{item.url}}" → "Visit https://instagram.com/john"

// Type-preserving resolution (returns actual type if whole string is one template)
ResolveValue(value interface{}) interface{}
// Example: "{{extract_urls.data}}" → []map[string]interface{}{...}

// Dot-path navigation (priority: StepResults → Variables → Data → nested)
ResolvePath(path string) interface{}
// Example: "item.url" → Variables["item"]["url"]

// Resolve all template fields in a StepDef (deep copy, returns resolved copy)
ResolveStepDef(step StepDef) StepDef
// Resolves: URL, Selector, ConfigKey, ElementRef, Attribute, Direction,
//           MethodName, Variable, WaitFor, WaitAfter, Duration, Value,
//           Alternatives, Args, RaceSelectors, Set
```

### 4.4 Variables Seeded at Action Start

```
actionId           → action.ID
actionType         → action.Type
platform           → action.TargetPlatform
reachedIndex       → action.ReachedIndex
messageText        → action.ContentMessage
contentMessage     → action.ContentMessage (alias)
commentText        → action.ContentMessage (alias)
replyText          → action.ContentMessage (alias)
text               → action.ContentMessage (alias)
messageSubject     → action.ContentSubject
contentSubject     → action.ContentSubject (alias)
keyword            → action.Keywords
keywords           → action.Keywords (alias)
keywordEncoded     → url.QueryEscape(action.Keywords)
contentBlobUrls    → action.ContentBlobURLs
<all action.Params keys> → action.Params values
```

**Loop Variables (Set Per Iteration):**
```
item       → current loop item (full object, e.g. {url, platform, status})
<indexVar> → current 0-based index
loopIndex  → current 0-based index (alias)
loopTotal  → total item count
```

**Step-Generated Variables** (stored by step handlers):
- `extract_text` → `Variables[variable_name]`
- `extract_attribute` → `Variables[variable_name]`
- `extract_multiple` → `Variables[variable_name]` (array)
- `find_element` → `Elements[step.ID]` + `Variables[variable_name]`
- `call_bot_method` → `Variables[variable_name]`

---

## 5. Error Handling & Control Flow

**File:** `internal/action/errors.go`

### 5.1 ErrorHandlerDef

```go
type ErrorHandlerDef struct {
    Action    string // primary error action
    MaxRetries int   // for "retry" action
    OnFailure  string // action after retries exhausted
}
```

**JSON representation:**
```json
{
  "onError": {
    "action": "retry",
    "maxRetries": 3,
    "onFailure": "mark_failed"
  }
}
```

### 5.2 Error Actions

| Action | Behavior |
|--------|----------|
| `"retry"` | Re-run step up to `maxRetries` times (default 3); then apply `onFailure` |
| `"try_alternative"` | Always retry (used for 3-tier fallback transitions) |
| `"mark_failed"` | Add to FailedItems, continue to next item |
| `"skip"` | Skip step, continue execution |
| `"continue"` | Record failure, continue (same as mark_failed without FailedItem) |
| `"abort"` | Stop entire action immediately (propagates `ErrAbort`) |

### 5.3 ErrorHandler Struct

```go
type ErrorHandler struct {
    mu          sync.Mutex
    retryCounts map[string]int  // per-step retry counter
}

func (h *ErrorHandler) Handle(
    ctx context.Context,
    def *ErrorHandlerDef,
    result *StepResult,
    execCtx *ExecutionContext,
) *StepResult

func (h *ErrorHandler) ResetRetries(stepID string)
```

### 5.4 Executor Error Flow

```
Per step execution:
    result, err = handler(ctx, resolvedStep)

    if err != nil || !result.Success:
        handled = errorHandler.Handle(ctx, step.OnError, result, execCtx)

        if handled.Abort  → return ErrAbort (stops action)
        if handled.Retry  → i-- (re-execute same step)
        if handled.Skip   → continue (skip to next step)

    else:
        if step.OnSuccess != nil → handleOnSuccess()
        errorHandler.ResetRetries(step.ID)
```

### 5.5 SuccessAction

```go
type SuccessAction struct {
    Action    string      // "set_variable", "increment", "save_data", "update_progress"
    Variable  string
    Value     interface{}
    Increment string      // variable name to increment
}
```

### 5.6 WithRetry Helper

```go
func WithRetry(ctx context.Context, maxRetries int, baseDelay time.Duration, fn func() error) error
```

**Exponential Backoff:** `delay = 2^attempt * baseDelay`, capped at 60s

### 5.7 Sentinel Errors

```go
var ErrAbort = errors.New("action execution aborted")
```

---

## 6. Bot Adapter Framework

**File:** `internal/bot/adapter.go`

### 6.1 BotAdapter Interface

```go
type BotAdapter interface {
    Platform() string                                    // "INSTAGRAM", "LINKEDIN", "X", "TIKTOK"
    LoginURL() string                                    // URL for manual login
    IsLoggedIn(page *rod.Page) (bool, error)            // check auth status
    ResolveURL(rawURL string) string                     // normalize/complete URL
    ExtractUsername(pageURL string) string               // extract username from URL
    SearchURL(keyword string) string                     // build keyword search URL
    SendMessage(ctx, page, username, message string) error
    GetProfileData(ctx, page) (map[string]interface{}, error)
    GetMethodByName(name string) (func(ctx context.Context, args ...interface{}) (interface{}, error), bool)
}
```

### 6.2 PlatformRegistry

```go
var PlatformRegistry = map[string]func() BotAdapter{}

func NewBot(platform string) (BotAdapter, error) {
    platform = strings.ToUpper(strings.TrimSpace(platform))
    constructor, ok := PlatformRegistry[platform]
    // ...
    return constructor(), nil
}
```

**Registration (in each platform's `init()`):**
```go
func init() {
    bot.PlatformRegistry["INSTAGRAM"] = func() bot.BotAdapter {
        return &InstagramBot{}
    }
}
```

### 6.3 GetMethodByName Pattern

```go
func (b *PlatformBot) GetMethodByName(name string) (func(ctx context.Context, args ...interface{}) (interface{}, error), bool) {
    switch name {
    case "method_name":
        return func(ctx context.Context, args ...interface{}) (interface{}, error) {
            page := args[0].(*rod.Page)   // ALWAYS auto-prepended by executor
            arg1 := args[1].(string)

            err := b.internalMethod(ctx, page, arg1)
            if err != nil {
                return nil, err
            }
            return map[string]interface{}{"success": true}, nil
        }, true

    default:
        return nil, false
    }
}
```

---

## 7. Browser Pool & Anti-Detection

**File:** `internal/bot/browser.go`

### 7.1 BrowserPool

```go
type BrowserPool struct {
    mu       sync.Mutex
    browsers []*rod.Browser
    pages    map[string]*rod.Page  // sessionID → page
    maxSize  int
    headless bool
    logger   zerolog.Logger
}

func NewBrowserPool(maxSize int, headless bool, logger zerolog.Logger) *BrowserPool
```

### 7.2 Page Acquisition

```go
func (bp *BrowserPool) AcquirePage(sessionID string) (*rod.Page, error)
```

**Flow:**
1. Check if page already exists for `sessionID` → reuse
2. Call `getOrCreateBrowser()`
3. Create stealth page: `stealth.Page(browser)` (disables `AutomationControlled`)
4. Store in `pages[sessionID]`

### 7.3 Browser Pool Logic

```go
func (bp *BrowserPool) getOrCreateBrowser() (*rod.Browser, error)
```

- If browsers exist AND last browser has < 10 pages → reuse last
- If at `maxSize` → round-robin reuse first browser
- Else → launch new browser with anti-detection flags

### 7.4 Anti-Detection Chrome Flags

```
--disable-blink-features=AutomationControlled
--disable-infobars
--disable-dev-shm-usage
--no-sandbox
--disable-setuid-sandbox
--disable-gpu
--disable-extensions
--disable-popup-blocking
--disable-background-networking
--disable-background-timer-throttling
--disable-backgrounding-occluded-windows
--disable-renderer-backgrounding
--disable-component-update
--disable-default-apps
--disable-hang-monitor
--disable-prompt-on-repost
--disable-sync
--disable-translate
--metrics-recording-only
--no-first-run
--safebrowsing-disable-auto-update
```

**Stealth Page:** Uses `go-rod/rod/lib/proto` stealth mode to evade bot detection scripts.

### 7.5 Cleanup

```go
func (bp *BrowserPool) ReleasePage(sessionID string)   // close and remove specific page
func (bp *BrowserPool) Close()                          // shutdown all pages and browsers
func (bp *BrowserPool) CleanupZombies()                // kill orphaned Chrome processes
```

**CleanupZombies OS commands:**
- Windows: `taskkill /F /IM chrome.exe` and `taskkill /F /IM chromium.exe`
- macOS: `pkill -f "Google Chrome for Testing"` and `pkill -f Chromium`
- Linux: `pkill -f chrome` and `pkill -f chromium`

---

## 8. Humanization System

**File:** `internal/bot/humanize.go`

### 8.1 WriteHumanLike (Main Entry)

```go
func WriteHumanLike(page *rod.Page, el *rod.Element, text string, mistakeProbability float64) error
```

**Flow:**
1. `el.Click(proto.InputMouseButtonLeft, 1)` → sleep 200-400ms
2. `el.Focus()` → sleep 100-200ms
3. For each character in text:
   - If `rand.Float64() < mistakeProbability` (default 0.05 = 5%): call `simulateTypo()`
   - Else: call `typeCharacter()`
   - Sleep 50-250ms between keystrokes
4. Post-typing pause: 1-3 seconds

**Critical:** Uses `page.Keyboard.Type()` NOT `elem.Type()` — ensures input reaches the focused element even when Instagram/others swap DOM on focus.

### 8.2 Typo Simulation

```go
func simulateTypo(page *rod.Page, correct rune) error
```

1. Type wrong character (`randomWrongChar(correct)`)
2. Pause 200-500ms
3. Backspace 1-3 times (sleep 50-150ms between each)
4. Retype correct character
5. Pause 100-300ms

### 8.3 Character Input

```go
func typeCharacter(page *rod.Page, ch rune) error
```

- BMP characters (standard): `page.Keyboard.Type(input.Key(ch))`
- Non-BMP (emoji, etc.): `page.InsertText(string(ch))`

### 8.4 Multi-line Input

```go
func InputWithNewlines(el *rod.Element, text string, removeNewlines bool) error
```

- If `removeNewlines`: strip `\n`/`\r`, call `elem.Input(clean)`
- Else: split on newlines, input segments, send `Shift+Enter` for soft line breaks, sleep 100-300ms between

### 8.5 Navigation Helpers

```go
func ClickAndWaitNavigation(page *rod.Page, el *rod.Element) error
    // 1. Set up navigation listener BEFORE click
    // 2. Click element
    // 3. Wait for navigation complete

func ClickAndWaitForContent(page *rod.Page, el *rod.Element, contentSelector string, timeout time.Duration) (*rod.Element, error)
    // 1. Click element
    // 2. Wait for contentSelector to appear
    // 3. WaitStable (500ms stabilization)
    // 4. Return element
```

### 8.6 Element Discovery with Fallbacks

```go
func FindElementWithAlternatives(page *rod.Page, primary string, alternatives []string, timeout time.Duration) (*rod.Element, error)
```

1. Try primary selector with probe timeout
2. On failure, try each alternative sequentially
3. Remaining time divided among alternatives
4. Minimum 500ms per selector

### 8.7 Race/Outcome Waiting

```go
func WaitForOutcome(page *rod.Page, outcomes map[string]string, timeout time.Duration) (string, *rod.Element, error)
// outcomes: map of label → CSS selector
// Races all selectors simultaneously
// Returns: label of first match, element, error
```

### 8.8 Scroll & Collect (Infinite Scroll)

```go
func ScrollAndCollect(page *rod.Page, itemSelector string, maxItems int) ([]*rod.Element, error)
```

1. Query all matching elements
2. Stop if reached `maxItems` or no new elements after 5 scrolls
3. Scroll 600px down in 3 increments
4. Wait 800-1300ms for lazy-loaded content
5. Repeat

### 8.9 File Upload

```go
func UploadFile(page *rod.Page, fileInputSelector string, filePaths []string) error
// Finds file input (including hidden inputs)
// Calls elem.SetFiles(filePaths)
```

### 8.10 Resource Blocking

```go
func BlockUnnecessaryResources(page *rod.Page)
// Blocks: images, fonts, media, stylesheets
// Reduces bandwidth and speeds page loads
```

---

## 9. 3-Tier Fallback Pattern

**File:** `THREE_TIER_FALLBACK.md`, `internal/config/schemas.go`

This pattern is **required for all Instagram DOM interactions** and recommended for other platforms.

### 9.1 Three Tiers

| Tier | Mechanism | `onError` | When to Use |
|------|-----------|-----------|-------------|
| 1 | `call_bot_method` | `"skip"` | Go code handles DOM complexity |
| 2 | `find_element` with `xpath` + `alternatives` | `"skip"` | Hardcoded XPath fallback |
| 3 | `find_element` with `configKey` | `"mark_failed"` | AI-generated selector (last resort) |

### 9.2 Full JSON Template

```json
{
  "steps": [
    {
      "id": "action_t1",
      "type": "call_bot_method",
      "methodName": "method_name",
      "args": ["{{item.url}}"],
      "variable_name": "t1Result",
      "timeout": 30,
      "onError": { "action": "skip" }
    },
    {
      "id": "check_t1",
      "type": "condition",
      "condition": { "variable": "t1Result", "operator": "not_exists" },
      "then": ["action_t2_find", "action_t2_click", "check_t2"]
    },
    {
      "id": "action_t2_find",
      "type": "find_element",
      "xpath": "//primary/xpath",
      "alternatives": ["//fallback/1", "//fallback/2"],
      "variable_name": "t2Element",
      "timeout": 10,
      "onError": { "action": "skip" }
    },
    {
      "id": "action_t2_click",
      "type": "click",
      "elementRef": "action_t2_find"
    },
    {
      "id": "check_t2",
      "type": "condition",
      "condition": { "variable": "t2Element", "operator": "not_exists" },
      "then": ["action_t3_find", "action_t3_click"]
    },
    {
      "id": "action_t3_find",
      "type": "find_element",
      "configKey": "field_name",
      "timeout": 15,
      "onError": { "action": "mark_failed" }
    },
    {
      "id": "action_t3_click",
      "type": "click",
      "elementRef": "action_t3_find"
    }
  ],
  "loops": [{
    "id": "process_items",
    "iterator": "selectedListItems",
    "indexVar": "reachedIndex",
    "steps": ["action_t1", "check_t1", "wait_settle", "log_result"]
  }]
}
```

### 9.3 Execution Flow

```
[Tier 1] call_bot_method (onError: skip)
    → SUCCESS: skip tiers 2 & 3
    → FAIL: t1Result = nil
        → [Gate] condition: t1Result not_exists
            → [Tier 2] find_element + xpath (onError: skip)
                → SUCCESS: t2Element set, click, skip tier 3
                → FAIL: t2Element = nil
                    → [Gate] condition: t2Element not_exists
                        → [Tier 3] find_element + configKey (onError: mark_failed)
                            → SUCCESS: click
                            → FAIL: mark_failed → next item
```

### 9.4 ConfigKey Schemas

**File:** `internal/config/schemas.go`

Each action type has a schema mapping `configKey` names to human-readable descriptions:

```go
// Instagram
INSTAGRAM_POST_LIKING         → {like_button, unlike_button}
INSTAGRAM_POST_COMMENTING     → {comment_textarea, post_button}
INSTAGRAM_BULK_MESSAGING      → {conversation_list, message_input, send_button}
INSTAGRAM_PROFILE_FETCH       → {followers_link, following_link, user_item_link}
INSTAGRAM_PUBLISH_CONTENT     → {create_button, file_input, next_button, caption_input, location_input, share_button}
INSTAGRAM_LOGIN               → {username_input, password_input, login_button}
INSTAGRAM_BULK_REPLYING       → {reply_button, reply_input}

// LinkedIn
LINKEDIN_LOGIN                → {username_input, password_input, login_button}
LINKEDIN_PROFILE_INFO         → {name, headline, location, connections, followers, about, picture}
LINKEDIN_SEND_MESSAGE         → {message_input, send_button, compose_button}
LINKEDIN_KEYWORD_SEARCH       → {search_input, people_filter, result_items}

// X (Twitter)
X_LOGIN                       → {username_input, password_input, login_button}
X_PROFILE_INFO                → {name, bio, location, following, followers, verified}
X_FOLLOWERS                   → {followers_list, user_item}
X_SEND_MESSAGE                → {compose_button, recipient_input, message_input, send_button}

// TikTok
TIKTOK_LOGIN                  → {username_input, password_input, login_button}
TIKTOK_PROFILE_INFO           → {handle, bio, following, followers, likes}
```

**ConfigKey Resolution Flow:**
```
step.configKey = "like_button"
    → action platform + actionType → lookup schema
    → schema["like_button"] = SchemaField{Description: "Like button..."}
    → ConfigManager.GetConfig(platform, configKey, context, html, purpose)
    → Returns XPath or CSS selector
```

---

## 10. Platform: Instagram Features

**Bot File:** `internal/bot/instagram/bot.go`
**Actions File:** `internal/bot/instagram/actions.go`
**Helpers File:** `internal/bot/instagram/instagram.go`
**Action JSONs:** `data/actions/instagram/*.json`

### 10.1 BotAdapter Implementation

```go
type InstagramBot struct{}

func (b *InstagramBot) Platform() string        { return "INSTAGRAM" }
func (b *InstagramBot) LoginURL() string        { return "https://www.instagram.com/accounts/login/" }
func (b *InstagramBot) IsLoggedIn(page) (bool, error)
    // Navigates to instagram.com, checks for profile icon presence
func (b *InstagramBot) ResolveURL(rawURL string) string
    // Ensures full instagram.com URL (adds https://www.instagram.com/ prefix if needed)
func (b *InstagramBot) ExtractUsername(pageURL string) string
    // Parses URL path, skips non-profile paths:
    // Skipped: home, explore, reels, direct, accounts, p, stories, tv, reel
func (b *InstagramBot) SearchURL(keyword string) string
    // Returns: "https://www.instagram.com/explore/tags/" + url.PathEscape(keyword)
```

### 10.2 Registered Bot Methods (GetMethodByName)

All registered in `internal/bot/instagram/bot.go`:

| Method Name | Go Function | Args (after page) | Returns |
|-------------|------------|-------------------|---------|
| `like_post` | `likePost()` | `url string` | `{success: bool}` |
| `comment_post` | `CommentPost()` | `postURL, commentText string` | `{success: bool}` |
| `follow_user` | `followUser()` | `username string` | `{success: bool}` |
| `unfollow_user` | `UnfollowUser()` | `username string` | `{success: bool}` |
| `send_message` | `SendMessage()` | `username, message string` | `{success: bool}` |
| `get_user_info` | `GetUserInfo()` | `username string` | profile map |
| `like_comment` | `LikeComment()` | `postURL string` | `{success: bool, count: int}` |
| `fetch_followers_list` | `FetchFollowersList()` | `username string, limit int` | followers array |
| `interact_with_posts` | `InteractWithPosts()` | `keyword string, limit int` | `{success: bool, count: int}` |
| `interact_with_user_posts` | `InteractWithUserPosts()` | `username string, limit int` | `{success: bool, count: int}` |
| `view_stories` | `ViewStories()` | `username string` | `{success: bool}` |
| `publish_content` | `PublishContent()` | `filePaths []string, caption string` | `{success: bool}` |
| `reply_to_conversation` | `ReplyToConversation()` | `conversationURL, reply string` | `{success: bool}` |
| `scrape_post_data` | `ScrapePostData()` | `postURL string` | post data map |
| `get_profile_data` | `GetProfileData()` | *(no extra args)* | profile map |
| `extract_username_from_metadata` | internal | *(no extra args)* | `{username: string}` |

### 10.3 Instagram DOM Patterns (Critical Knowledge)

```
Structure (as of 2026):
- No <article> on individual post pages
- Post action bar (Like, Comment, Share, Save) → inside <section>
- Sections are NESTED: outer wraps comments+action bar, inner IS action bar
- Always select INNERMOST (last) matching section for action bar
- Comments have own like buttons (NOT inside action <section>)
- svg[aria-label='Like'] AND svg[aria-label='Unlike'] exist for both posts AND comments
```

**Element Identification Pattern:**
```javascript
// 1. JS scan to identify correct element
const sections = document.querySelectorAll('section');
const innermost = sections[sections.length - 1]; // action bar is last
const likeBtn = innermost.querySelector('svg[aria-label="Like"]');
// 2. Mark with temp attribute
likeBtn.setAttribute('data-monoes-target', 'true');
```
```go
// 3. Rod finds marked element
el, _ := page.Element('[data-monoes-target="true"]')
// 4. Native click (NOT JS .click())
el.Click(proto.InputMouseButtonLeft, 1)
// 5. Remove marker
page.Eval(`document.querySelector('[data-monoes-target]').removeAttribute('data-monoes-target')`)
```

### 10.4 Dialog Dismissal

```go
func dismissNotificationDialog(page *rod.Page) error
// Looks for "Turn on Notifications" dialog
// Clicks "Not Now" button
// Must be called after every DM navigation
```

### 10.5 Instagram Actions (15 Total)

#### Feature: Post Liking
- **Action JSON:** `data/actions/instagram/like_posts.json`
- **actionType:** `POST_LIKING`
- **Bot Method:** `like_post(ctx, page, postURL string)`
- **Flow:** Navigate to post URL → identify action bar section (innermost) → check for Unlike (avoid toggle-off) → click Like button
- **3-Tier:** T1=`like_post`, T2=XPath to SVG Like, T3=configKey `like_button`
- **Inputs:** `selectedListItems` (array of `{url, platform, status}`)

#### Feature: Post Commenting
- **Action JSON:** `data/actions/instagram/comment_on_posts.json`
- **actionType:** `POST_COMMENTING`
- **Bot Method:** `comment_post(ctx, page, postURL, commentText string)`
- **Flow:** Navigate to post → find comment textarea → `WriteHumanLike()` → click Post button
- **3-Tier:** T1=`comment_on_post`, T2=XPath textarea, T3=configKey `comment_textarea`
- **Inputs:** `selectedListItems`, `commentText`

#### Feature: Direct Messaging (Bulk)
- **Action JSON:** `data/actions/instagram/send_dms.json`
- **actionType:** `BULK_MESSAGING`
- **Bot Method:** `send_message(ctx, page, username, message string)`
- **Flow:** Navigate to DM page → `dismissNotificationDialog()` → find/create conversation → type message → send
- **Inputs:** `selectedListItems`, `messageText`
- **Dialog Handling:** Calls `dismissNotificationDialog()` after every navigation

#### Feature: Auto-Reply to DMs
- **Action JSON:** `data/actions/instagram/auto_reply_dms.json`
- **actionType:** `BULK_REPLYING`
- **Bot Method:** `reply_to_conversation(ctx, page, conversationURL, replyText string)`
- **Flow:** Open conversation URL → type reply → send
- **Inputs:** `selectedListItems` (conversation URLs), `messageText`

#### Feature: Follow Users
- **Action JSON:** `data/actions/instagram/follow_users.json`
- **actionType:** `FOLLOW_USERS`
- **Bot Method:** `follow_user(ctx, page, username string)`
- **Flow:** Navigate to profile → find Follow button → click (checks if already following)
- **Inputs:** `selectedListItems`

#### Feature: Unfollow Users
- **Action JSON:** `data/actions/instagram/unfollow_users.json`
- **actionType:** `UNFOLLOW_USERS`
- **Bot Method:** `unfollow_user(ctx, page, username string)`
- **Flow:** Navigate to profile → find Following button → click → confirm in dialog
- **Inputs:** `selectedListItems`

#### Feature: Export Followers
- **Action JSON:** `data/actions/instagram/export_followers.json`
- **actionType:** `PROFILE_FETCH`
- **Bot Method:** `fetch_followers_list(ctx, page, username string, limit int)`
- **Flow:** Navigate to profile → click Followers → scroll & collect user items → extract usernames/URLs
- **Output:** Array of `{username, profileURL, platform}`
- **Inputs:** `selectedListItems`, optional `limit`

#### Feature: Scrape Profile Info
- **Action JSON:** `data/actions/instagram/scrape_profile_info.json`
- **actionType:** `PROFILE_INFO_FETCH`
- **Bot Method:** `get_user_info(ctx, page, username string)`
- **Returns:** `{username, full_name, bio, followers_count, following_count, posts_count, is_private, is_verified, profile_picture_url, external_url}`
- **Inputs:** `selectedListItems`

#### Feature: Keyword/Hashtag Discovery
- **Action JSON:** `data/actions/instagram/find_by_keyword.json`
- **actionType:** `KEYWORD_SEARCH`
- **Flow:** Navigate to `SearchURL(keyword)` (hashtag explore page) → scroll & collect post links
- **Inputs:** `keyword`

#### Feature: Engage with Posts (Keyword-Based)
- **Action JSON:** `data/actions/instagram/engage_with_posts.json`
- **actionType:** `KEYWORD_ENGAGEMENT`
- **Bot Method:** `interact_with_posts(ctx, page, keyword string, limit int)`
- **Flow:** Search hashtag → collect posts → like + optional comment each
- **Inputs:** `keyword`, optional `commentText`, `limit`

#### Feature: Engage with User's Posts
- **Action JSON:** `data/actions/instagram/engage_user_posts.json`
- **actionType:** `USER_POST_ENGAGEMENT`
- **Bot Method:** `interact_with_user_posts(ctx, page, username string, limit int)`
- **Flow:** Navigate to profile → collect post URLs → like each post
- **Inputs:** `selectedListItems`

#### Feature: Extract Post Data
- **Action JSON:** `data/actions/instagram/extract_post_data.json`
- **actionType:** `POST_DATA_EXTRACTION`
- **Bot Method:** `scrape_post_data(ctx, page, postURL string)`
- **Returns:** `{url, likes_count, comments_count, caption, author, timestamp, media_type}`
- **Inputs:** `selectedListItems`

#### Feature: Like Comments on Posts
- **Action JSON:** `data/actions/instagram/like_comments_on_posts.json`
- **actionType:** `COMMENT_LIKING`
- **Bot Method:** `like_comment(ctx, page, postURL string)`
- **Flow:** Navigate to post → find comment like buttons (NOT in action section) → click each
- **Returns:** `{success: bool, count: int}`
- **Inputs:** `selectedListItems`

#### Feature: Watch Stories
- **Action JSON:** `data/actions/instagram/watch_stories.json`
- **actionType:** `STORY_VIEWING`
- **Bot Method:** `view_stories(ctx, page, username string)`
- **Flow:** Navigate to profile → click story ring → wait for story → advance through all stories
- **Inputs:** `selectedListItems`

#### Feature: Publish Post/Content
- **Action JSON:** `data/actions/instagram/publish_post.json`
- **actionType:** `PUBLISH_CONTENT`
- **Bot Method:** `publish_content(ctx, page, filePaths []string, caption string)`
- **Flow:** Click create → upload file → add caption → add location (optional) → share
- **configKey Fields:** `create_button`, `file_input`, `next_button`, `caption_input`, `location_input`, `share_button`
- **Inputs:** `contentBlobUrls`, optional `messageText` (caption), optional `locationText`

---

## 11. Platform: LinkedIn Features

**Bot File:** `internal/bot/linkedin/bot.go`
**Action JSONs:** `data/actions/linkedin/*.json`

### 11.1 BotAdapter Implementation

```go
type LinkedInBot struct{}

func (b *LinkedInBot) Platform() string     { return "LINKEDIN" }
func (b *LinkedInBot) LoginURL() string     { return "https://www.linkedin.com/login" }
func (b *LinkedInBot) IsLoggedIn(page) (bool, error)
    // Checks for linkedin.com/feed/ URL or main nav presence
func (b *LinkedInBot) ResolveURL(rawURL string) string
    // Ensures full linkedin.com URL
func (b *LinkedInBot) ExtractUsername(pageURL string) string
    // Parses /in/<username>/ path segments
func (b *LinkedInBot) SearchURL(keyword string) string
    // Returns: "https://www.linkedin.com/search/results/people/?keywords=" + url.QueryEscape(keyword)
```

### 11.2 Registered Bot Methods

| Method Name | Description | Returns |
|-------------|-------------|---------|
| `send_message` | Send DM to user | `{success: bool}` |
| `get_profile_data` | Scrape profile | profile map |

### 11.3 GetProfileData Returns

```go
map[string]interface{}{
    "full_name":           string,
    "headline":            string,
    "location":            string,
    "connection_count":    int,
    "follower_count":      int,
    "about":               string,
    "profile_picture_url": string,
    "current_experience":  []map{title, company, duration},
    "education":           []map{school, degree, years},
}
```

### 11.4 LinkedIn Actions (7 Total)

| Action | JSON File | actionType |
|--------|-----------|------------|
| Send DMs | `send_dms.json` | `BULK_MESSAGING` |
| Auto-Reply | `auto_reply_dms.json` | `BULK_REPLYING` |
| Engage with Posts | `engage_with_posts.json` | `KEYWORD_ENGAGEMENT` |
| Export Followers | `export_followers.json` | `PROFILE_FETCH` |
| Find by Keyword | `find_by_keyword.json` | `KEYWORD_SEARCH` |
| Publish Post | `publish_post.json` | `PUBLISH_CONTENT` |
| Scrape Profile | `scrape_profile_info.json` | `PROFILE_INFO_FETCH` |

**SendMessage Flow:** Navigates to `linkedin.com/messaging/compose/` → searches for recipient → selects from suggestions → types message → sends

---

## 12. Platform: X (Twitter) Features

**Bot File:** `internal/bot/x/bot.go`
**Action JSONs:** `data/actions/x/*.json`

### 12.1 BotAdapter Implementation

```go
type XBot struct{}

func (b *XBot) Platform() string     { return "X" }
func (b *XBot) LoginURL() string     { return "https://twitter.com/i/flow/login" }
func (b *XBot) IsLoggedIn(page) (bool, error)
    // Checks for x.com/home URL or main nav presence
func (b *XBot) ResolveURL(rawURL string) string
    // Handles both twitter.com and x.com URLs
func (b *XBot) ExtractUsername(pageURL string) string
    // Parses /<username> path, skips: i, home, explore, notifications, messages, search
func (b *XBot) SearchURL(keyword string) string
    // Returns: "https://x.com/search?q=" + url.QueryEscape(keyword) + "&src=typed_query&f=user"
```

### 12.2 Registered Bot Methods

| Method Name | Description | Returns |
|-------------|-------------|---------|
| `send_message` | Send DM via compose URL | `{success: bool}` |
| `get_profile_data` | Scrape profile | profile map |

### 12.3 SendMessage Implementation

```go
// Uses compose DM URL:
page.Navigate("https://x.com/messages/compose?recipient_id=<resolved_id>")
// Finds message input via data-testid="dmComposerTextInput"
// Types message using page.Keyboard.Type()
// Sends via data-testid="dmComposerSendButton"
```

### 12.4 GetProfileData Returns

```go
map[string]interface{}{
    "username":            string,
    "full_name":           string,
    "bio":                 string,
    "location":            string,
    "following_count":     int,
    "follower_count":      int,
    "is_verified":         bool,
    "profile_picture_url": string,
    "banner_url":          string,
}
```

### 12.5 X Actions (7 Total)

| Action | JSON File | actionType |
|--------|-----------|------------|
| Send DMs | `send_dms.json` | `BULK_MESSAGING` |
| Auto-Reply | `auto_reply_dms.json` | `BULK_REPLYING` |
| Engage with Posts | `engage_with_posts.json` | `KEYWORD_ENGAGEMENT` |
| Export Followers | `export_followers.json` | `PROFILE_FETCH` |
| Find by Keyword | `find_by_keyword.json` | `KEYWORD_SEARCH` |
| Publish Post | `publish_post.json` | `PUBLISH_CONTENT` |
| Scrape Profile | `scrape_profile_info.json` | `PROFILE_INFO_FETCH` |

---

## 13. Platform: TikTok Features

**Bot File:** `internal/bot/tiktok/bot.go`
**Action JSONs:** `data/actions/tiktok/*.json`

### 13.1 BotAdapter Implementation

```go
type TikTokBot struct{}

func (b *TikTokBot) Platform() string     { return "TIKTOK" }
func (b *TikTokBot) LoginURL() string     { return "https://www.tiktok.com/login" }
func (b *TikTokBot) IsLoggedIn(page) (bool, error)
    // Checks for tiktok.com/foryou URL or profile icon
func (b *TikTokBot) ResolveURL(rawURL string) string
    // Ensures full tiktok.com URL
func (b *TikTokBot) ExtractUsername(pageURL string) string
    // Parses /@<username> path (strips @ prefix from path segment)
func (b *TikTokBot) SearchURL(keyword string) string
    // Returns: "https://www.tiktok.com/search/user?q=" + url.QueryEscape(keyword)
```

### 13.2 Registered Bot Methods

| Method Name | Description | Returns |
|-------------|-------------|---------|
| `send_message` | Send DM | `{success: bool}` |
| `get_profile_data` | Scrape profile | profile map |

### 13.3 GetProfileData Returns

```go
map[string]interface{}{
    "handle":         string,   // @username
    "bio":            string,
    "following_count": int,
    "follower_count":  int,
    "likes_count":     int,
    "is_verified":     bool,
    "website":         string,
}
```

### 13.4 TikTok Actions (7 Total)

Same action types as LinkedIn/X: `BULK_MESSAGING`, `BULK_REPLYING`, `KEYWORD_ENGAGEMENT`, `PROFILE_FETCH`, `KEYWORD_SEARCH`, `PUBLISH_CONTENT`, `PROFILE_INFO_FETCH`

---

## 14. Platform: Telegram Features

**Bot File:** `internal/bot/telegram/bot.go`

### 14.1 BotAdapter Implementation

```go
type TelegramBot struct{}

func (b *TelegramBot) Platform() string     { return "TELEGRAM" }
func (b *TelegramBot) LoginURL() string     { return "https://web.telegram.org/" }
func (b *TelegramBot) ResolveURL(rawURL string) string
    // Converts @username → "#@username" (hash-based routing in web app)
func (b *TelegramBot) ExtractUsername(pageURL string) string
    // Parses #@username from URL fragment
func (b *TelegramBot) SearchURL(keyword string) string
    // Returns: "https://web.telegram.org/#@" + keyword
```

### 14.2 SendMessage Implementation

```go
func (b *TelegramBot) SendMessage(ctx, page, username, message string) error
```

**Flow:**
1. Determine if username is phone number via `isPhoneNumber()` (regex: all digits/+/-)
2. If phone: navigate directly to `t.me/+<phone>`
3. If username: search via sidebar search input
4. Wait for conversation to load
5. Find message input box
6. Type message via `page.Keyboard.Type()`
7. Press Enter or click Send button

```go
func isPhoneNumber(s string) bool
    // regex: ^[+\-0-9]+$
```

**Note:** No action JSONs exist for Telegram yet; only the bot adapter is implemented.

---

## 15. Storage & Database

**Files:** `internal/storage/database.go`, `models.go`, `repository.go`, `filestore.go`
**Migrations:** `data/migrations/001_initial.sql` through `005_add_action_params.sql`

### 15.1 Database Setup

```go
func NewDatabase(dbPath string, logger zerolog.Logger) (*Database, error)
```

- SQLite with WAL mode: `PRAGMA journal_mode=WAL`
- Busy timeout: 5000ms
- Migrations auto-applied on startup
- DB path: `~/.monoes/monoes.db` (default)

### 15.2 Data Models

**Session:**
```go
type Session struct {
    ID        string    // UUID
    Platform  string    // "INSTAGRAM", "LINKEDIN", etc.
    Username  string
    Cookies   string    // JSON-serialized cookies
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

**Action:**
```go
type Action struct {
    ID              string
    Type            string    // actionType e.g. "POST_LIKING"
    Status          string    // "PENDING", "RUNNING", "COMPLETED", "FAILED", "PAUSED"
    TargetPlatform  string
    ReachedIndex    int       // resume point
    ContentMessage  string
    ContentSubject  string
    Keywords        string
    ContentBlobURLs []string
    ScheduledAt     *time.Time
    CronExpression  string
    Params          map[string]string  // arbitrary extra parameters
    CreatedAt       time.Time
    UpdatedAt       time.Time
}
```

**ActionTarget:**
```go
type ActionTarget struct {
    ID         string
    ActionID   string    // FK → Action.ID
    TargetURL  string
    Platform   string
    Status     string    // "PENDING", "SUCCESS", "FAILED"
    Error      string
    ProcessedAt *time.Time
}
```

**Person:**
```go
type Person struct {
    ID          string
    Platform    string
    Username    string
    ProfileURL  string
    DisplayName string
    Bio         string
    FollowerCount int
    FollowingCount int
    Data        map[string]interface{}  // platform-specific extra data
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

**SocialList / SocialListItem:**
```go
type SocialList struct {
    ID          string
    Name        string
    Platform    string
    Description string
    CreatedAt   time.Time
}

type SocialListItem struct {
    ID         string
    ListID     string    // FK → SocialList.ID
    URL        string
    Platform   string
    Status     string    // "PENDING", "PROCESSED"
    AddedAt    time.Time
}
```

**Thread:**
```go
type Thread struct {
    ID           string
    Platform     string
    ThreadURL    string
    Participants []string  // usernames
    LastMessage  string
    LastMessageAt time.Time
    UpdatedAt    time.Time
}
```

**Template:**
```go
type Template struct {
    ID       string
    Name     string
    Platform string
    Content  string    // message template text
    Subject  string    // for email subjects
    Tags     []string
}
```

**ConfigEntry:**
```go
type ConfigEntry struct {
    ID        string
    Platform  string
    ActionType string
    Key       string     // configKey name
    Selector  string     // XPath or CSS selector
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

### 15.3 Repository API

**File:** `internal/storage/repository.go`

```go
// Actions
CreateAction(action *Action) error
GetAction(id string) (*Action, error)
ListActions(platform, status string) ([]*Action, error)
UpdateActionState(id, state string) error
UpdateActionReachedIndex(id string, index int) error
DeleteAction(id string) error

// ActionTargets
CreateActionTarget(target *ActionTarget) error
ListActionTargets(actionID string) ([]*ActionTarget, error)
UpdateActionTargetStatus(id, status, errMsg string) error

// Persons
UpsertPerson(person *Person) error
GetPerson(platform, username string) (*Person, error)
ListPersons(platform string) ([]*Person, error)

// SocialLists
CreateList(list *SocialList) error
GetList(id string) (*SocialList, error)
ListLists(platform string) ([]*SocialList, error)
AddListItem(item *SocialListItem) error
ListItems(listID string, status string) ([]*SocialListItem, error)

// Threads
UpsertThread(thread *Thread) error
GetThread(platform, threadURL string) (*Thread, error)
ListThreads(platform string) ([]*Thread, error)

// Templates
CreateTemplate(tmpl *Template) error
GetTemplate(id string) (*Template, error)
ListTemplates(platform string) ([]*Template, error)

// Config
SaveConfig(entry *ConfigEntry) error
GetConfig(platform, actionType, key string) (*ConfigEntry, error)

// Data Export
SaveExtractedData(actionID string, items []map[string]interface{}) error
```

### 15.4 File Export

**File:** `internal/storage/filestore.go`

```go
func SaveExtractedData(actionID string, items []map[string]interface{}) error
    // Saves to: ~/.monoes/data/<actionID>_<timestamp>.json

func ExportAllData(outputDir string) error
    // Exports all DB records to JSON files
```

### 15.5 SQL Migrations

```
001_initial.sql         → sessions, actions, action_targets, persons, social_lists, social_list_items tables
002_add_threads.sql     → threads table
003_add_templates.sql   → templates table
004_add_config.sql      → config_entries table
005_add_action_params.sql → adds params column to actions table (JSON map)
```

---

## 16. Scheduler

**File:** `internal/scheduler/scheduler.go`

### 16.1 Scheduler Setup

```go
type Scheduler struct {
    cron    *cron.Cron
    db      StorageInterface
    runner  RunnerInterface
    logger  zerolog.Logger
}

func NewScheduler(db StorageInterface, runner RunnerInterface, logger zerolog.Logger) *Scheduler

func (s *Scheduler) Start()  // Start cron engine
func (s *Scheduler) Stop()   // Graceful shutdown
```

### 16.2 Scheduling Actions

```go
func (s *Scheduler) ScheduleAction(action *Action) (cron.EntryID, error)
```

- Uses `action.CronExpression` as the cron schedule
- On trigger: sets action status to "RUNNING", calls `runner.RunSingle(ctx, action, ...)`
- On completion: updates status to "COMPLETED" or "FAILED"
- Returns `cron.EntryID` for later cancellation

### 16.3 Period Calculation

```go
func NextPeriod(cronExpr string) (time.Time, error)
```

- Parses cron expression
- Returns next scheduled run time from now
- Used for UI display

### 16.4 Cron Expression Format

Standard 5-field cron: `minute hour dayOfMonth month dayOfWeek`

Examples:
```
"0 9 * * *"     → Daily at 9:00 AM
"0 9 * * 1-5"   → Weekdays at 9:00 AM
"*/30 * * * *"  → Every 30 minutes
"0 0 1 * *"     → First day of month at midnight
```

---

## 17. Config Manager (AI-Powered)

**Files:** `internal/config/manager.go`, `apiclient.go`, `schemas.go`

### 17.1 ConfigManager

```go
type ConfigManager struct {
    db            StorageInterface
    apiClient     *APIClient
    activeConfigs map[string]*ConfigEntry  // in-memory cache
    logger        zerolog.Logger
}

func NewConfigManager(db StorageInterface, apiClient *APIClient, logger zerolog.Logger) *ConfigManager
```

### 17.2 3-Tier Config Resolution

```go
func (cm *ConfigManager) GetConfig(
    platform string,
    configKey string,
    actionType string,
    pageHTML string,   // current DOM HTML for AI analysis
    purpose string,    // human-readable description for AI prompt
) (string, error)
// Returns: XPath or CSS selector string
```

**Resolution Order:**
1. **In-Memory Cache:** `activeConfigs[platform+":"+actionType+":"+configKey]`
2. **Database:** `db.GetConfig(platform, actionType, configKey)`
3. **LLM API:** `apiClient.GenerateConfig(platform, actionType, configKey, pageHTML, purpose)`

**Force Refresh:**
```go
func (cm *ConfigManager) RefreshConfig(platform, actionType, configKey, pageHTML, purpose string) error
// Bypasses cache and DB, always calls LLM API
```

### 17.3 API Client

**File:** `internal/config/apiclient.go`

```go
type APIClient struct {
    baseURL    string  // "http://apiv1.monoes.me"
    httpClient *http.Client  // 90-second timeout
}

func NewAPIClient() *APIClient
```

**Endpoints:**
```
POST /extracttest
    Body: {platform, actionType, configKey, html, purpose}
    Returns: {selector: "xpath_or_css_string"}

GET /configs/{name}
    Returns: {name, selector, ...}

POST /generate-config
    Body: {platform, actionType, configKey, html, purpose}
    Returns: {selector: "xpath_or_css_string"}
    Timeout: 90 seconds (LLM inference)
```

---

## 18. Authentication & Sessions

**File:** `internal/auth/session.go`

### 18.1 AuthManager

```go
type AuthManager struct {
    db     StorageInterface
    logger zerolog.Logger
}

func NewAuthManager(db StorageInterface, logger zerolog.Logger) *AuthManager
```

### 18.2 Cookie Management

```go
func (am *AuthManager) SaveCookies(platform string, page *rod.Page) error
    // Extracts cookies from Rod page
    // Serializes to JSON string
    // Stores in sessions table via db.UpsertSession()

func (am *AuthManager) RestoreCookies(platform string, page *rod.Page) error
    // Loads cookie JSON from sessions table
    // Deserializes and sets on Rod page
    // Uses page.SetCookies(cookies)

func (am *AuthManager) HasValidSession(platform string) (bool, error)
    // Checks if session exists in DB
    // Checks if not expired (platform-specific TTL)
```

### 18.3 Session Restoration Flow

```
On action start:
1. AuthManager.HasValidSession(platform) → true?
2. BrowserPool.AcquirePage(sessionID)
3. AuthManager.RestoreCookies(platform, page)
4. BotAdapter.IsLoggedIn(page) → true?
5. If false: navigate to LoginURL(), wait for user, re-save cookies
6. Proceed with action
```

---

## 19. CLI Interface

**Files:** `cmd/monoes/`

Built with `github.com/spf13/cobra`. Entry point: `cmd/monoes/main.go`

### 19.1 Available Commands

```
monoes
├── login <platform>          Navigate to platform login URL, wait for completion, save cookies
├── run <platform> <action>   Execute a single action immediately
│   --list-id <id>            Target a specific social list
│   --message "text"          Set message content
│   --keyword "term"          Set search keyword
├── schedule <platform> <action> <cron>  Schedule recurring action
├── list actions              List all scheduled/pending actions
├── list lists                List all social lists
├── list people               List all scraped persons
├── import <file>             Import URLs from CSV/JSON into a social list
├── export <action-id>        Export extracted data to file
└── serve                     Start background scheduler daemon
```

### 19.2 Action Execution Flow (CLI)

```
monoes run instagram POST_LIKING --list-id <uuid>
    ↓
1. Load social list items from DB (status=PENDING)
2. Build StorageAction with ContentMessage, Keywords, Params
3. Create BrowserPool + AuthManager
4. RestoreCookies for platform
5. AcquirePage for session
6. ActionLoader.Load("instagram", "POST_LIKING")
7. ActionExecutor.Execute(ctx, action, page, botAdapter)
8. Print results (success count, failed items)
```

---

## 20. REST API (FastAPI)

**File:** `monoes_apis/openapi.json`
**Language:** Python (FastAPI)
**Base URL:** `http://apiv1.monoes.me`

### 20.1 Endpoints

```
POST /extracttest
    Description: Test selector extraction against HTML
    Request: {
        platform: string,
        actionType: string,
        configKey: string,
        html: string,
        purpose: string
    }
    Response: { selector: string, confidence: float }

GET /configs/{name}
    Description: Get a named config
    Path: name = "<platform>_<actionType>_<configKey>"
    Response: { name: string, selector: string, updatedAt: string }

POST /generate-config
    Description: Generate selector via LLM
    Request: {
        platform: string,
        actionType: string,
        configKey: string,
        html: string,
        purpose: string
    }
    Response: { selector: string }
    Note: 90-second timeout (LLM inference time)
```

---

## 21. Action Runner (Concurrency)

**File:** `internal/action/runner.go`

### 21.1 ActionRunner

```go
type ActionRunner struct {
    maxWorkers int
    db         StorageInterface
    configMgr  ConfigInterface
    logger     zerolog.Logger
    events     chan ExecutionEvent  // buffered, size 256
}

func NewActionRunner(maxWorkers int, db StorageInterface, configMgr ConfigInterface, logger zerolog.Logger) *ActionRunner

func (r *ActionRunner) RunAll(
    ctx context.Context,
    actions []StorageAction,
    pageProvider func(action StorageAction) (*rod.Page, BotAdapter, error),
) []ExecutionResult
```

### 21.2 Bounded Worker Pool

```go
sem := make(chan struct{}, maxWorkers)  // semaphore

for i, action := range actions {
    wg.Add(1)
    sem <- struct{}{}  // acquire worker slot
    go func(idx int, act StorageAction) {
        defer wg.Done()
        defer func() { <-sem }()  // release on completion
        results[idx] = r.safeExecuteSingle(ctx, act, pageProvider)
    }(i, action)
}
wg.Wait()
```

### 21.3 Panic Recovery

```go
defer func() {
    if rec := recover(); rec != nil {
        result = ExecutionResult{
            FailedItems: []FailedItem{{StepID: "runner_panic", Error: fmt.Errorf("panic: %v", rec)}},
        }
        db.UpdateActionState(actionID, "FAILED")
    }
}()
```

### 21.4 ExecutionResult

```go
type ExecutionResult struct {
    SuccessCount int
    FailedItems  []FailedItem
    Duration     time.Duration
    Data         []map[string]interface{}
}
```

### 21.5 Event Streaming

```go
func (r *ActionRunner) Events() <-chan ExecutionEvent

type ExecutionEvent struct {
    Type     string    // "action_start", "action_complete", "action_panic", "step_start", "step_complete", "loop_iteration"
    ActionID string
    StepID   string
    Data     interface{}
}
```

Events are emitted non-blocking; dropped if channel full (capacity 256).

---

## 22. Utility Systems

### 22.1 SleepRandom (Timing)

**File:** `internal/util/sleep.go`

```go
type SleepConfig struct {
    ActionDelay    time.Duration  // between actions (default 2-5s)
    NavigationWait time.Duration  // after navigation (default 1-3s)
    TypeDelay      time.Duration  // between keystrokes (default 50-250ms)
    ScrollPause    time.Duration  // after scrolling (default 800-1300ms)
}

func SleepRandom(min, max time.Duration)
    // Sleeps for random duration in [min, max]
    // Uses crypto/rand for unpredictability

func DefaultSleepConfig() SleepConfig
```

### 22.2 Number Conversion

**File:** `internal/util/converter.go`

```go
func ConvertAbbreviatedNumber(s string) int
    // "1.5K" → 1500
    // "2.3M" → 2300000
    // "500"  → 500
    // Handles K, M, B suffixes
    // Used when scraping follower/following counts
```

### 22.3 Queue Merging

**File:** `internal/algorithms/merge.go`

```go
func MergePrevCurrentQueue(prev, current []ActionTarget) []ActionTarget
    // Merges two action target queues
    // Preserves order, deduplicates by URL
    // Used when resuming partially-completed actions
```

### 22.4 Logging

All components use `github.com/rs/zerolog` structured logging:

```go
logger.Info().
    Str("platform", platform).
    Str("action", actionType).
    Int("index", index).
    Msg("Processing item")
```

Log levels used: `Debug`, `Info`, `Warn`, `Error`

---

## Appendix A: Adding a New Platform

To add a new platform (e.g., "PINTEREST"):

1. **Create bot file:** `internal/bot/pinterest/bot.go`
   - Define `PinterestBot struct{}`
   - Implement all `BotAdapter` interface methods
   - Implement `GetMethodByName()` with all method cases
   - Add `init()` registration: `bot.PlatformRegistry["PINTEREST"] = func() bot.BotAdapter { return &PinterestBot{} }`

2. **Create action JSONs:** `data/actions/pinterest/<ACTION_TYPE>.json`
   - Follow the JSON schema in Section 2.1
   - Use 3-tier fallback pattern for all DOM interactions (Section 9)

3. **Add config schemas:** `internal/config/schemas.go`
   - Add `var pinterestLoginSchema = schema("LOGIN", ...)` with field definitions
   - Register in `schemas` map: `"PINTEREST_LOGIN": pinterestLoginSchema`

4. **Update embed:** `data/embed.go`
   - Add `//go:embed actions/pinterest` if a new directory embed is needed

5. **Add integration tests:** `internal/action/pinterest_integration_test.go`
   - Use `//go:build integration` build tag
   - Follow existing test patterns

## Appendix B: Adding a New Action to Existing Platform

1. Create `data/actions/<platform>/<ACTION_TYPE>.json`
2. If using `call_bot_method`: add Go method to `internal/bot/<platform>/bot.go` in both implementation and `GetMethodByName()`
3. If using `configKey` for Tier 3: add schema fields to `internal/config/schemas.go`
4. Write integration test

## Appendix C: Action JSON Field Reference

```json
{
  "actionType": "STRING",           // e.g. "POST_LIKING"
  "platform": "STRING",             // e.g. "instagram"
  "version": "STRING",              // e.g. "1.0.0"
  "description": "STRING",
  "metadata": {
    "requiresAuth": true,
    "supportsPagination": false,
    "supportsRetry": true
  },
  "inputs": {
    "required": ["selectedListItems"],
    "optional": ["messageText", "keyword"]
  },
  "outputs": {
    "success": ["count", "reachedIndex"],
    "failure": ["failedItems"]
  },
  "steps": [ /* StepDef objects - see Section 2.4 */ ],
  "loops": [{
    "id": "STRING",
    "iterator": "selectedListItems",
    "indexVar": "reachedIndex",
    "steps": ["step_id_1", "step_id_2"],
    "onComplete": "update_action_state"
  }],
  "errorHandling": {
    "globalRetries": 3,
    "retryDelay": 2000,
    "onFinalFailure": "log_and_continue"
  }
}
```

## Appendix D: selectedListItems Format

```json
[
  {
    "url": "https://www.instagram.com/username/",
    "platform": "INSTAGRAM",
    "status": "PENDING"
  }
]
```

This is the primary loop iterator for most actions.

---

*Generated by multi-agent analysis of the Mono Agent codebase. All file paths are relative to `newmonoes/`. Last updated: 2026-03-10.*
