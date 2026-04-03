# Mono Agent — Complete Fixes Changelog

**Date:** 2026-04-03  
**Commits:** 4 commits, 40 files changed, 592 insertions, 2567 deletions

---

## Summary

| Category | Fixes Applied | Severity Range |
|----------|--------------|----------------|
| Backend Security (Auth) | 8 | Critical → Medium |
| Backend Nodes (P0 Critical) | 8 | Critical |
| Backend Nodes (P1+P2) | 11 | High → Medium |
| Workflow Engine | 5 | Critical → Medium |
| Frontend UX (P0 Critical) | 12 | Critical |
| Frontend UX (P1 Major) | 6 | Major |
| Dead Code Removed | 2 files (2328 lines) | — |
| **Total** | **52 fixes** | |

---

## 1. Backend Security & Auth Fixes

### PKCE Added to OAuth Flow
- **File:** `internal/connections/oauth.go`
- **What:** Added `generateCodeVerifier()` (43-char base64url random) and `computeCodeChallenge()` (SHA-256 + base64url). `RunOAuthFlow` now generates verifier+challenge, passes challenge to auth URL and verifier to token exchange.
- **Why:** Without PKCE, an intercepted authorization code could be exchanged for tokens by another app on the machine.

### Secrets Removed from Process Environment
- **Files:** `wails-app/app.go`, `internal/connections/manager.go`
- **What:** `ConnectOAuthWithProgress` now accepts `clientID, clientSecret` parameters directly. `ConnectPlatformOAuth` in app.go reads DB credentials and passes them as arguments instead of calling `os.Setenv`.
- **Why:** `os.Setenv` makes secrets visible to all goroutines, child processes, and `/proc/<pid>/environ`.

### Token Refresh in RunNode
- **File:** `wails-app/app.go` (RunNode credential resolution)
- **What:** Replaced direct `connMgr.Get()` + `conn.Data` access with `getResourceCredentialData()` which auto-refreshes expired OAuth tokens.
- **Why:** Previously, expired tokens caused workflow failures with no auto-retry.

### Missing Tables in Wails safeMigrations
- **File:** `wails-app/app.go`
- **What:** Added `workflow_executions`, `workflow_execution_nodes`, `credentials` CREATE TABLE statements.
- **Why:** Wails-only installs were missing these tables, causing workflow execution failures.

### Debug Log Guarded
- **File:** `internal/ai/google.go`
- **What:** `debugLog()` now returns immediately unless `MONOES_GOOGLE_DEBUG=1` is set.
- **Why:** Was writing request/response data to world-readable `/tmp/monoes-google-debug.log` unconditionally.

### TOCTOU Race Fixed in Trigger Activation
- **File:** `internal/workflow/trigger_manager.go`
- **What:** Both `activateSchedule` and `activateWebhook` now hold mutex continuously through check+insert. Config parsing moved before lock acquisition.
- **Why:** Concurrent calls could register duplicate cron jobs for the same workflow.

### Webhook CORS Restricted
- **File:** `internal/workflow/webhook_server.go`
- **What:** CORS headers only set when HMAC secret is configured. Uses request `Origin` instead of `*`.
- **Why:** Wildcard CORS on unauthenticated endpoints enabled cross-origin workflow triggering.

### IsActive Guard in handleTrigger
- **File:** `internal/workflow/engine.go`
- **What:** Added `if !wf.IsActive { return }` check after fetching workflow in trigger handler.
- **Why:** Stale triggers could execute deactivated workflows.

---

## 2. Backend Node Critical Fixes (P0)

### SQL Injection Fixed in DB Nodes
- **Files:** `internal/nodes/db/postgres.go`, `internal/nodes/db/mysql.go`
- **What:** Added `validateIdentifier()` with regex `^[a-zA-Z_][a-zA-Z0-9_.]*$`. Called on table name and all column keys before query construction.
- **Why:** Table/column names from user-controlled config were interpolated directly into SQL strings.

### Broken UPDATE WHERE Clause Fixed
- **Files:** `internal/nodes/db/postgres.go`, `internal/nodes/db/mysql.go`
- **What:** UPDATE now reads `where` string from config and appends it properly. Returns error if no WHERE clause provided.
- **Why:** Was generating `WHERE $N` / `WHERE ?` — invalid SQL with a placeholder in the WHERE position.

### DELETE No-WHERE Fixed
- **Files:** Same as above
- **What:** DELETE now requires non-empty `where` from config. Returns error `"DELETE requires a WHERE clause"` if missing.
- **Why:** Was generating `DELETE FROM table` with no WHERE — full table wipe.

### HTTP Pagination Infinite Loop Fixed
- **File:** `internal/nodes/http/request.go`
- **What:** Added `outer:` label to pagination `for` loop, changed `break` to `break outer`.
- **Why:** `break` inside `switch` only broke the switch, not the loop — infinite requests to remote server.

### Context Leak in dbCtx() Fixed
- **File:** `internal/workflow/execution.go`
- **What:** `dbCtx()` now returns `(context.Context, context.CancelFunc)`. All 3 call sites updated to `defer cancel()`.
- **Why:** Leaked a goroutine-backed 10s timer per node execution.

### execute_command Environment Fix
- **File:** `internal/nodes/system/execute_command.go`
- **What:** When `env` config is provided, `cmd.Env` now starts from `os.Environ()` before appending custom vars.
- **Why:** Setting `cmd.Env` without parent env stripped `PATH`, `HOME`, etc. from child process.

### xml.go Panic Fix
- **File:** `internal/nodes/data/xml.go`
- **What:** Changed bare `existing.(string)` to safe comma-ok type assertion.
- **Why:** Unchecked type assertion could panic at runtime.

### Telegram Nil Dereference Fix
- **File:** `internal/nodes/comm/telegram.go`
- **What:** Wrapped `u.Message.From.UserName` access with nil check on `From`.
- **Why:** Telegram Bot API allows `From` to be nil for channel posts.

---

## 3. Backend Node P1+P2 Fixes

### github.go create_pr — Added head/base Fields
- **File:** `internal/nodes/service/github.go`
- **What:** Added `head` and `base` from config with non-empty validation.
- **Why:** GitHub API requires both fields — was always returning 422.

### shopify.go — Error on Missing Response Keys
- **File:** `internal/nodes/service/shopify.go`
- **What:** `get_product`, `get_order`, `get_customer` now return errors instead of silent empty items.
- **Why:** Type assertion failure was silently swallowed, returning empty output.

### huggingface.go — Omit max_new_tokens When 0
- **File:** `internal/nodes/service/huggingface.go`
- **What:** Only include `max_new_tokens` when value > 0.
- **Why:** Sending 0 breaks some models (treated as "generate zero tokens").

### people/save.go — Defer tx.Rollback
- **File:** `internal/nodes/people/save.go`
- **What:** Added `defer tx.Rollback()` after BeginTx. Removed redundant explicit rollback calls.
- **Why:** Commit failure left transaction open until connection timeout.

### Custom URL Encoders Replaced
- **Files:** `internal/nodes/service/gmail.go`, `google_drive.go`, `salesforce.go`
- **What:** Replaced `gmailURLEncode`, `driveURLEncode`, `salesforceURLEncode` with `url.QueryEscape`.
- **Why:** Custom byte-by-byte encoders broke multi-byte UTF-8 characters.

### HTTP Client Timeout Added
- **File:** `internal/nodes/service/helpers.go`
- **What:** Added `var httpClient = &http.Client{Timeout: 60 * time.Second}`. Used in `apiRequest` and `apiRequestList`.
- **Why:** `http.DefaultClient` has no timeout — stalled servers could block indefinitely.

### Redis KEYS Replaced with SCAN
- **File:** `internal/nodes/db/redis.go`
- **What:** Replaced `rdb.Keys()` with SCAN-based cursor iteration.
- **Why:** KEYS command blocks the Redis server for O(N) scan time.

### SSH Known Hosts Support
- **File:** `internal/nodes/http/ssh.go`
- **What:** Added optional `known_hosts` config field using `knownhosts.New`. Logs warning when using InsecureIgnoreHostKey.
- **Why:** Every SSH connection was vulnerable to MITM — no host key verification.

### Secure Temp Files for Generated Images
- **Files:** `internal/nodes/service/openrouter.go`, `huggingface.go`
- **What:** Replaced `fmt.Sprintf("/tmp/monoes_post_...")` with `os.CreateTemp()` (0600 permissions).
- **Why:** Generated images were world-readable in `/tmp`.

---

## 4. Frontend P0 Critical UX Fixes

### Disconnect Confirmation Dialog
- **File:** `wails-app/frontend/src/pages/Connections.jsx`
- **What:** Added `window.confirm()` before disconnect API call.

### Expired Session Tile — Yellow Dot
- **File:** `wails-app/frontend/src/pages/Connections.jsx`
- **What:** `connected` now checks `conn.status === 'active'`. Expired sessions show yellow dot (#fbbf24).

### OAuth Connect Hint Text
- **File:** `wails-app/frontend/src/pages/Connections.jsx`
- **What:** Added visible hint "Save your OAuth credentials above first" when Connect button is disabled.

### Clear profileId on Navigate
- **File:** `wails-app/frontend/src/App.jsx`
- **What:** Added `setProfileId(null)` when navigating away from profile/postDetail.

### Inspector Opens on Node Click
- **File:** `wails-app/frontend/src/pages/NodeRunner.jsx`
- **What:** Node click now calls `setInspectorOpen(true)` alongside `setSelectedId`.

### New Workflow Save Confirmation
- **File:** `wails-app/frontend/src/pages/NodeRunner.jsx`
- **What:** Added `window.confirm()` in `handleNew` when canvas has nodes.

### Clear Canvas Confirmation
- **File:** `wails-app/frontend/src/pages/NodeRunner.jsx`
- **What:** Added `window.confirm()` in trash button handler.

### AI Chat Send Feedback
- **File:** `wails-app/frontend/src/components/AIChatPanel.jsx`
- **What:** Added yellow warning banner when no provider selected.

### Focus Styles Restored
- **File:** `wails-app/frontend/src/index.css`
- **What:** Removed global `input:focus-visible { outline: none }` rule.

### prefers-reduced-motion
- **File:** `wails-app/frontend/src/index.css`
- **What:** Added media query to disable animations for users with motion sensitivity.

### CSS Variables Fixed
- **File:** `wails-app/frontend/src/index.css`
- **What:** Replaced undefined `--bg-hover`, `--bg-surface`, `--bg-selected`, `--error` with correct design system tokens.

### Dead Code Deleted
- **Files:** `pages/Workflow.jsx` (2117 lines), `pages/Sessions.jsx` (211 lines)
- **What:** Both files were never imported or routed — completely unreachable dead code.

---

## 5. Frontend P1 Major UX Fixes

### Error Handling on Page Loads
- **Files:** `People.jsx`, `Profile.jsx`, `Connections.jsx`
- **What:** Added try/catch/finally with error state and visible error banner.

### Dashboard Loading Skeletons
- **File:** `Dashboard.jsx`
- **What:** Added `dashLoading` state. StatCard shows pulsing placeholder during fetch.

### Human-Readable Action Labels
- **File:** `Actions.jsx`
- **What:** Added `ACTION_LABELS` map. Dropdown and list now show "Like Posts" instead of `like_posts`.

### Log Level Filter Dropdown
- **File:** `Logs.jsx`
- **What:** Added level filter select (All/INFO/WARN/ERROR/SYSTEM) as AND condition with text search.

### Dynamic Credential Platform Derivation
- **File:** `NodeRunner.jsx`
- **What:** `platformId` now checks `node.schema?.credential_platform` first, falls back to hardcoded map.

### Action Edit Capability
- **File:** `Actions.jsx`
- **What:** Added Edit button (Pencil icon) on action cards. CreateModal pre-populates form when editing.

---

## Previously Applied P0 Fixes (from AUTH review)

These were applied in earlier commits:

| Fix | Commit |
|-----|--------|
| OAuth callback bound to 127.0.0.1 (was 0.0.0.0) | `3067624` |
| Google Drive query injection escaped | `3067624` |
| DB directory permissions 0700 (was 0755) | `3067624` |
| Config API switched to HTTPS | `3067624` |
| Dead Credentials.jsx deleted | `3067624` |
| Broken "Manage credentials" nav link fixed | `3067624` |
| OAuth token auto-refresh for ListResources | `986dc4d` |
| Reconnect button in ResourcePickerField | `986dc4d` |

---

## Remaining P2 Items (Not Yet Implemented)

| # | Fix | Effort | Reason Deferred |
|---|-----|--------|----------------|
| 1 | Encrypt credentials at rest (AES-256-GCM) | Large | Requires key management strategy (OS keychain) |
| 2 | Pass node config via stdin (not CLI args) | Medium | Architecture change to subprocess execution |
| 3 | Unify 3 credential storage systems | Large | Migration + API compatibility work |
| 4 | Add connections/platform_oauth_credentials to migration system | Medium | Requires migration runner changes |
| 5 | Extract shared Avatar component | Small | CSS architecture cleanup |
| 6 | Replace inline styles with CSS classes | Large | Affects all 17 JSX files |
| 7 | Add spacing/typography tokens | Medium | Design system refactor |
| 8 | Add responsive breakpoints | Medium | Requires layout testing |
| 9 | Increase touch targets to 24px | Small | Multiple files |
| 10 | Add keyboard focus trap to tag editor | Small | Accessibility |
| 11 | Add interaction history pagination | Medium | Profile page |
| 12 | Use CSS modal classes consistently | Medium | Multiple pages |
| 13 | Add { value, label } for select options | Medium | Schema + UI change |
| 14 | Implement undo/redo in workflow editor | Medium | Requires snapshot stack |
| 15 | Add connection pooling for DB nodes | Medium | Requires shared pool manager |
