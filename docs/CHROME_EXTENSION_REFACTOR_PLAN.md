# Chrome Extension Refactor Plan

## Overview
Replace Rod/CDP browser automation with a Chrome Extension that runs inside the user's existing Chrome profile, eliminating cookie restoration, session management, and bot detection issues.

## Architecture

```
Go Backend (CLI/Wails)                Chrome Extension (Manifest V3)
┌─────────────────────────┐           ┌──────────────────────────────┐
│ WebSocket Server        │◄──WS───►  │ Background Service Worker    │
│ (:9222/monoes)          │           │ - WS client + reconnect      │
│                         │           │ - chrome.tabs API             │
│ PageInterface           │           │ - chrome.scripting.executeScript│
│ ├─ ExtensionPage (new)  │           │                              │
│ └─ RodPage (fallback)   │           │ Content Script               │
│                         │           │ - Element finding/interaction │
│ ActionExecutor          │           │ - MutationObserver for waits  │
│ BotAdapters (unchanged) │           │ - WeakRef element registry    │
└─────────────────────────┘           └──────────────────────────────┘
```

## 6 Phases

### Phase 1: PageInterface Abstraction
**New files:**
- `internal/browser/interfaces.go` — PageInterface, ElementHandle interfaces
- `internal/browser/rod_page.go` — RodPage wrapper implementing PageInterface
- `internal/browser/rod_helpers.go` — Migrated humanize helpers

### Phase 2: Migrate ActionExecutor
**Modified files:**
- `internal/action/executor.go` — `*rod.Page` → `PageInterface`
- `internal/action/steps.go` — All 20 step handlers
- `internal/action/runner.go` — Page provider signature

### Phase 3: Migrate Bot Adapters
**Modified files (all bots):**
- `internal/bot/adapter.go` — BotAdapter interface
- `internal/bot/{instagram,linkedin,x,tiktok,gemini,telegram,email}/bot.go`
- `internal/bot/humanize.go`
- `internal/nodes/browser_adapter.go` — SessionProvider interface
- `internal/auth/session.go`

### Phase 4: Chrome Extension + WebSocket
**New files:**
- `internal/extension/server.go` — WS server
- `internal/extension/protocol.go` — Command/Response types
- `internal/extension/page.go` — ExtensionPage implementing PageInterface
- `chrome-extension/manifest.json`
- `chrome-extension/background.js`
- `chrome-extension/content.js`
- `chrome-extension/commands/{navigate,element,interact,extract,keyboard}.js`

### Phase 5: Integration + Rod Fallback
**New files:**
- `internal/browser/provider.go` — HybridSessionProvider (extension first, Rod fallback)

**Modified files:**
- `cmd/monoes/run.go`, `node.go`, `workflow.go`
- `wails-app/app.go`
- `internal/bot/browser.go`

### Phase 6: Edge Cases
- Human-like typing via dispatchEvent
- File upload via chrome.debugger
- Cookie handling (no-op in extension mode)
- Resource blocking via declarativeNetRequest
- Service worker lifecycle (keep-alive via WS)

## Communication Protocol

```json
// Command (Go → Extension)
{"id":"uuid","type":"navigate","tabId":12,"params":{"url":"https://gemini.google.com"}}

// Response (Extension → Go)  
{"id":"uuid","success":true,"data":{"url":"https://gemini.google.com/app"}}
```

Command types: navigate, element, elements, has, click, input, text, attribute, 
set_files, scroll, keyboard, page_info, reload, wait_load, wait_dom_stable, race

## Key Decisions
1. **WebSocket** over Native Messaging (no message size limit, bidirectional, no OS registration)
2. **chrome.scripting.executeScript** over chrome.debugger (no yellow infobar)
3. **Hybrid fallback** — extension preferred, Rod when extension unavailable
4. **Action JSON unchanged** — same format, same 3-tier fallback
5. **BotAdapter interface preserved** — only `*rod.Page` → `PageInterface` signature change

## Surface Area
- 26 Go files import rod
- 36 page.* call sites in steps.go
- 7 bot adapters with GetMethodByName closures
- 55 action JSON files (unchanged)
- ~140 total *rod.Page/*rod.Element references to migrate
