# Mono Agent — Authentication Architecture & Security Review

**Date:** 2026-04-02  
**Scope:** Full codebase analysis of authentication flows, credential storage, component data flow, and security best practices.

---

## Table of Contents

1. [Authentication Method Taxonomy](#1-authentication-method-taxonomy)
2. [Backend Auth Flows](#2-backend-auth-flows)
3. [Credential Storage Architecture](#3-credential-storage-architecture)
4. [Frontend Component Auth Flow](#4-frontend-component-auth-flow)
5. [Database Schema & Migrations](#5-database-schema--migrations)
6. [Security Findings](#6-security-findings)
7. [Dead Code & Inconsistencies](#7-dead-code--inconsistencies)
8. [Recommended Fixes (Priority Order)](#8-recommended-fixes-priority-order)

---

## 1. Authentication Method Taxonomy

Five distinct `AuthMethod` types defined in `internal/connections/registry.go:6-12`:

| Method | Constant | Platforms |
|--------|----------|-----------|
| `oauth` | `MethodOAuth` | Google Sheets, Gmail, Google Drive, Slack, GitHub, Notion, HubSpot, Salesforce, etc. |
| `apikey` | `MethodAPIKey` | Telegram, Stripe, Discord, Twilio, OpenRouter |
| `browser` | `MethodBrowser` | Instagram, LinkedIn, X, TikTok |
| `connstring` | `MethodConnStr` | PostgreSQL, MySQL, MongoDB, Redis |
| `apppassword` | `MethodAppPass` | Gmail (alt), SMTP |

---

## 2. Backend Auth Flows

### 2.1 OAuth Flow

**Entry points:**
- **Wails UI:** `wails-app/app.go:ConnectPlatformOAuth` (line 2437) → spawns goroutine → `connMgr.ConnectOAuthWithProgress`
- **CLI:** `internal/connections/manager.go:Connect` (line 36) → `connectOAuth` → `RunOAuthFlow`

**Sequence:**
1. Client ID/Secret resolved from: (a) `OAuthConfig` compile-time values (all blank), (b) `MONOES_{PLATFORM}_CLIENT_ID` env vars, (c) `platform_oauth_credentials` DB table → injected into env vars via `os.Setenv`
2. `oauth.go:RunOAuthFlow` generates CSRF state (16 bytes `crypto/rand`), builds auth URL, starts localhost HTTP callback server
3. Opens browser via `exec.Command("open", url)` (macOS only)
4. Callback validates state, extracts code
5. `exchangeCode` POSTs to token endpoint with `grant_type=authorization_code`
6. Tokens stored in `conn.Data` map: `access_token`, `refresh_token`, `token_type`, `scope`, `expires_at`
7. `ValidateConnection` called per-platform to resolve account ID
8. Connection persisted via `store.Save`

### 2.2 Token Refresh

`wails-app/resources.go:getResourceCredentialData` (line 86):
- Checks `expires_at` — if within 60s of expiry, calls `refreshOAuthToken`
- Refresh uses stored `refresh_token` + platform OAuth config to POST `grant_type=refresh_token`
- New tokens persisted to DB
- **Soft failure:** if refresh fails, falls through to use expired token; API call fails → `NeedsReauth: true` returned to frontend

**Gap:** Token refresh only happens in `ListResources`/`CreateResource` path. `RunNode` does NOT auto-refresh — expired tokens cause workflow failures silently.

### 2.3 Browser Session Auth (Social Platforms)

`wails-app/app.go:LoginSocial` (line 2493):
1. Launches visible Chrome via Rod
2. Navigates to platform login URL
3. Polls `adapter.IsLoggedIn(page)` every 2s
4. On success: captures all cookies, stores in `crawler_sessions` table
5. Mirrors a stub `Connection` record (username only, no cookies) into `connections` table
6. Session expiry: fixed 30-day window, no refresh mechanism

### 2.4 Credential Resolution in Workflows

`wails-app/app.go:RunNode` (lines 1880-1891):
1. Reads `credential_id` from node config
2. Fetches `Connection` via `connMgr.Get`
3. Merges ALL `conn.Data` keys into `req.Config` (access_token, refresh_token, API keys — everything)
4. For non-browser nodes, config is JSON-serialized and passed as `--config` CLI argument (visible in `ps aux`)

---

## 3. Credential Storage Architecture

### Three Separate Systems (Critical Finding)

| System | Table | Created By | Status |
|--------|-------|-----------|--------|
| **A: `credentials`** | `credentials` | Migration 006 | **BROKEN** — `app.go` queries `service_type`/`encrypted_data` but table has `type`/`data` columns |
| **B: `workflow_credentials`** | `workflow_credentials` | Never created | **BROKEN** — referenced in `internal/workflow/storage.go` but no migration exists |
| **C: `connections`** | `connections` | Runtime `EnsureTable()` | **Active** — the only working credential system |

Additionally:
- `platform_oauth_credentials` — stores OAuth app client_id/client_secret (runtime-created, not in migrations)
- `crawler_sessions` — stores browser cookies as plaintext JSON

### No Encryption Anywhere

All credential material is stored as plaintext JSON in SQLite:
- `connections.data` — access_token, refresh_token, API keys, connection strings
- `platform_oauth_credentials` — client_id, client_secret
- `crawler_sessions.cookies_json` — full browser cookie jar
- `credentials.encrypted_data` — **misleading column name**, stores plaintext JSON

---

## 4. Frontend Component Auth Flow

### Active Auth UI: Connections Page (`pages/Connections.jsx`)

Handles all three auth methods inline:
- **OAuth:** client_id/secret setup → save → "Connect with {platform}" → `ConnectPlatformOAuth` → event stream (conn:progress/conn:done)
- **Browser:** "Login" → `LoginSocial` → visible Chrome window → event stream
- **API Key/Fields:** form fields → `SaveConnectionDirect`

### Credential Flow Through Components

```
Connections.jsx (manage/connect)
    ↓ stores Connection in DB
NodeRunner.jsx (workflow editor)
    ↓ reads via GetConnectionsForPlatform
    ↓ user selects credential_id in dropdown
    ↓ passes to ResourcePickerField
ResourcePickerField.jsx (resource browser)
    ↓ calls ListResources(platform, type, credentialId, query)
    ↓ backend auto-refreshes token if expired
    ↓ on 401: shows "Reconnect Account" button → ConnectPlatformOAuth
```

### Dead Code

| Item | File | Issue |
|------|------|-------|
| `Credentials.jsx` | `pages/Credentials.jsx` | Never imported in `App.jsx`, not in sidebar. Entirely dead. |
| `ListCredentials`/`SaveCredential`/`DeleteCredential` | Wails bindings in `App.js` | Only used by `Workflow.jsx`; reference the broken `credentials` table |
| "Manage credentials" link in NodeRunner | `NodeRunner.jsx:591` | Navigates to `'credentials'` which doesn't exist in router — silently falls back to dashboard |

### Bugs

| Bug | File:Line | Impact |
|-----|-----------|--------|
| `handleReconnect` missing error path | `ResourcePickerField.jsx:72-91` | If `ConnectPlatformOAuth` returns error synchronously, `reconnecting` stays true forever |
| `EventsOff('conn:progress')` kills all listeners | `Connections.jsx:202` | Global unsubscribe — if another component listens to same event, it gets silently removed |
| `CREDENTIAL_PLATFORMS` map is hardcoded | `NodeRunner.jsx:485-510` | New service node types won't get credential dropdown until manually added |

---

## 5. Database Schema & Migrations

### Migration vs Wails safeMigrations Divergence

| Feature | Migration System | Wails safeMigrations | Divergence |
|---------|-----------------|---------------------|------------|
| `workflow_connections` UNIQUE constraint | Yes (migration 006) | **Missing** | Wails-only installs allow duplicate edges |
| `workflow_executions` table | Yes (migration 006) | **Missing** | Wails-only installs: workflow execution fails |
| `workflow_execution_nodes` table | Yes (migration 006) | **Missing** | Same as above |
| `credentials` table | Yes (migration 006) | **Missing** | Not usable from Wails (also has column mismatch) |
| `connections` table | Not in migrations | Runtime `EnsureTable` | Outside migration tracking |
| `platform_oauth_credentials` | Not in migrations | Runtime ensure | Outside migration tracking |

---

## 6. Security Findings

### Critical

| ID | Issue | File | Fix |
|----|-------|------|-----|
| **CRIT-1** | All credentials stored as plaintext in SQLite | `connections/storage.go:76`, `app.go:2386` | Encrypt `data` column with AES-256-GCM using OS keychain-derived key |
| **CRIT-2** | OAuth callback binds to `0.0.0.0:9876` (all interfaces) | `oauth.go:88-91` | Change to `"127.0.0.1:%d"` |
| **CRIT-3** | client_secret injected into process env vars via `os.Setenv` | `app.go:2464-2465` | Pass credentials as function arguments directly |
| **CRIT-4** | Config API uses plain HTTP | `config/apiclient.go:16` | Change to `https://apiv1.monoes.me` |

### High

| ID | Issue | File | Fix |
|----|-------|------|-----|
| **HIGH-1** | Google Drive query injection (unescaped user input in API query) | `resources.go:203-213` | Escape single quotes: `strings.ReplaceAll(query, "'", "\\'")` |
| **HIGH-2** | DB directory created with 0755, file may be world-readable | `cmd/monoes/root.go:84` | Use `0700` for dir, `0600` for file |
| **HIGH-3** | No PKCE in OAuth flow | `oauth.go` | Add `code_verifier`/`code_challenge` (S256) |
| **HIGH-4** | `system.execute_command` node allows arbitrary shell commands | `nodes/system/execute_command.go:42-52` | Require explicit opt-in flag or allowlist |
| **HIGH-5** | Webhook server has no mandatory authentication | `workflow/webhook_server.go:180-190` | Require `HMACSecret` be non-empty |

### Medium

| ID | Issue | File | Fix |
|----|-------|------|-----|
| **MED-1** | Debug log written to world-readable `/tmp` | `ai/google.go:16-23` | Guard with env flag, write to `~/.monoes/logs/` with 0600 |
| **MED-2** | `GetOAuthCredentials` returns client_secret to frontend JS | `app.go:2397-2411` | Return only client_id, or mask the secret |
| **MED-3** | `encrypted_data` column name stores plaintext | `app.go:1642` | Rename to `data` or implement actual encryption |
| **MED-4** | `http.DefaultClient` used for token exchange (no timeout) | `oauth.go:168`, `resources.go:149` | Use client with 30s timeout |
| **MED-5** | No token refresh in `RunNode` — expired tokens fail silently | `app.go:1880-1891` | Add refresh check before merging creds |
| **MED-6** | Credentials visible in `ps aux` via `--config` CLI arg | `app.go:1924-1929` | Pass config via stdin or temp file (0600) |

### Low

| ID | Issue | File |
|----|-------|------|
| **LOW-1** | `openBrowser` macOS-only (`exec.Command("open", ...)`) | `oauth.go:202` |
| **LOW-2** | Browser session cookies fixed 30-day expiry, no early invalidation detection | `app.go:2571` |
| **LOW-3** | Social `Connection` record holds only username, not cookies — two tables must stay in sync | `app.go:2591-2599` |

---

## 7. Dead Code & Inconsistencies

| Item | Location | Action |
|------|----------|--------|
| `pages/Credentials.jsx` | Frontend | **Delete** — never imported, fully replaced by Connections |
| `SaveCredential` / `ListCredentials` | `app.go:1602-1672` | **Fix or delete** — queries wrong column names (`service_type`/`encrypted_data` vs `type`/`data`) |
| `workflow_credentials` table references | `internal/workflow/storage.go:686-775` | **Fix** — table never created; either add migration or redirect to `connections` |
| `"Manage credentials"` nav link | `NodeRunner.jsx:591` | **Fix** — change `'credentials'` to `'connections'` |
| Old OAuth Credentials tab in Settings | Already removed this session | Done |

---

## 8. Recommended Fixes (Priority Order)

### P0 — Fix Before Next Release

1. **Bind OAuth callback to localhost only** (`oauth.go:88`) — one-line fix
2. **Fix Google Drive query injection** (`resources.go:203`) — one-line fix
3. **Fix DB directory permissions** (`root.go:84`, `app.go:54`) — two-line fix
4. **Fix broken "Manage credentials" link** (`NodeRunner.jsx:591`) — `'credentials'` → `'connections'`
5. **Delete dead `Credentials.jsx`**
6. **Switch config API to HTTPS** (`config/apiclient.go:16`)

### P1 — Next Sprint

7. **Add PKCE to OAuth flow** (`oauth.go`) — ~30 lines
8. **Stop injecting secrets into env vars** (`app.go:2464`) — refactor to pass as arguments
9. **Add token refresh to `RunNode`** (`app.go:1880`) — reuse `getResourceCredentialData` pattern
10. **Fix `SaveCredential`/`ListCredentials` column names** or delete if unused
11. **Add `workflow_executions` + `workflow_execution_nodes` to Wails safeMigrations**
12. **Guard debug log** (`ai/google.go`) with env flag
13. **Require HMAC for webhook server** or add auth flag

### P2 — Medium Term

14. **Encrypt credentials at rest** — AES-256-GCM with OS keychain key
15. **Pass node config via stdin** instead of CLI args (avoids `ps aux` exposure)
16. **Unify credential storage** — migrate everything to `connections` table, remove `credentials` and `workflow_credentials`
17. **Add `connections` and `platform_oauth_credentials` tables to migration system**

---

## Key Files Reference

| File | Purpose |
|------|---------|
| `internal/connections/registry.go` | Platform definitions, OAuth configs, auth methods |
| `internal/connections/oauth.go` | OAuth flow engine (RunOAuthFlow, exchangeCode) |
| `internal/connections/manager.go` | Connection lifecycle (Connect, Refresh, Save) |
| `internal/connections/storage.go` | Connection CRUD, `connections` table DDL |
| `internal/connections/validate.go` | Per-platform connection validators |
| `wails-app/app.go` | Wails API: ConnectPlatformOAuth, LoginSocial, RunNode, credential resolution |
| `wails-app/resources.go` | Token refresh, ListResources, needs_reauth |
| `wails-app/updater.go` | Version check and self-update |
| `data/migrations/001_initial.sql` | Core schema (sessions, actions, people) |
| `data/migrations/006_workflow_system.sql` | Workflows + credentials table |
| `frontend/src/pages/Connections.jsx` | Primary auth management UI |
| `frontend/src/components/ResourcePickerField.jsx` | Resource picker with reconnect flow |
| `frontend/src/pages/NodeRunner.jsx` | Workflow editor with credential dropdown |
