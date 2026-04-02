# Monoes Agent — Node Architecture & Best Practices Review

**Date:** 2026-04-02  
**Scope:** Full review of all 106 workflow node implementations, execution engine, schemas, UI/CLI integration.

---

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [Node Type Registry & Schemas](#2-node-type-registry--schemas)
3. [Execution Engine](#3-execution-engine)
4. [Critical Bugs](#4-critical-bugs)
5. [Important Issues](#5-important-issues)
6. [UI/CLI Gaps](#6-uicli-gaps)
7. [Per-Category Review](#7-per-category-review)
8. [Recommended Fixes (Priority Order)](#8-recommended-fixes-priority-order)

---

## 1. Architecture Overview

### Dual Execution Paths

| Path | Used For | Mechanism |
|------|----------|-----------|
| **Subprocess** | All non-browser nodes (http, comm, service, db, control, data, system, ai, people) | Wails calls `monoes node run <type> --config ... --input ...`, parses stdout JSON |
| **In-process** | Browser/social nodes (instagram.*, linkedin.*, x.*, tiktok.*) | Wails calls `runBrowserNode()` directly, launches Chrome via Rod |

### Node Counts by Category

| Category | Count | Examples |
|----------|-------|---------|
| triggers | 3 | manual, schedule, webhook |
| control | 14 | if, switch, filter, merge, code, set, sort, etc. |
| data | 8 | compression, crypto, datetime, html, xml, etc. |
| http | 3 | request, ftp, ssh |
| system | 2 | execute_command, rss_read |
| db | 4 | postgres, mysql, mongodb, redis |
| comm | 7 | slack, discord, telegram, twilio, email_send/read, whatsapp |
| service | 15 | google_sheets, gmail, github, stripe, etc. |
| ai | 6 | chat, extract, etc. |
| browser | 36 | 15 instagram, 7 linkedin, 7 x, 7 tiktok |
| people | 1 | save |
| **Total** | **106** | |

---

## 2. Node Type Registry & Schemas

### Schema System

- 74 JSON schema files embedded at compile time from `internal/workflow/schemas/`
- Schema resolution: try `schemas/<nodeType>.json` → for browser nodes try `schemas/action.<suffix>.json` → fallback to `schemas/browser.generic.json` → empty schema
- Schemas drive the Inspector UI in NodeRunner.jsx — field types: text, number, boolean, select, textarea, code, password, array, resource_picker, credential_picker

### Registration

All nodes registered via `RegisterAll()` functions in each `internal/nodes/*/register.go`. Browser nodes registered dynamically from embedded action JSON files via `RegisterBrowserNodes()`.

### Key Gaps

- **Trigger nodes not registered in CLI registry** — `trigger.manual/schedule/webhook` appear in UI palette and have schemas but `monoes node run trigger.*` fails with "unknown node type"
- **20+ browser nodes registered but invisible in UI** — `RegisterBrowserNodes` creates them from action JSONs, but `GetWorkflowNodeTypes` hardcodes a shorter list. Missing from UI: `instagram.reply_to_comments`, `instagram.list_post_comments`, `instagram.list_user_posts`, all `tiktok.*` (9 nodes), several `linkedin.*` nodes
- **CREDENTIAL_PLATFORMS mismatch** — 8 nodes in the hardcoded map have `credential_platform: null` in their schema (stripe, shopify, db.*, etc.), rendering both a useless credential dropdown AND inline API key fields

---

## 3. Execution Engine

### Workflow Engine (`internal/workflow/`)

- **DAG construction** via Kahn's algorithm (`dag.go`) with cycle detection
- **BFS execution loop** (`execution.go`) processes topologically-sorted nodes, routes data via `pendingInputs`
- **Queue** with N worker goroutines and cancellable contexts per execution
- **Expression engine** using Go `text/template` with caching
- **Trigger manager** handles cron schedules and webhooks with mutex-protected maps
- **Hybrid store** — JSON files for workflow definitions (Wails), SQLite for executions

---

## 4. Critical Bugs

### CRIT-1: SQL Injection in postgres.go and mysql.go

**Files:** `internal/nodes/db/postgres.go:136-169`, `internal/nodes/db/mysql.go:138-163`

Table names and column names from user-controlled `config` are interpolated directly into SQL strings with only quote wrapping. A table name containing a closing quote breaks out of the identifier. `database/sql` parameterization only protects values, not identifiers.

**Fix:** Validate identifiers against `^[a-zA-Z_][a-zA-Z0-9_]*$` before query construction.

### CRIT-2: Broken UPDATE WHERE clause (postgres.go, mysql.go)

**Files:** `postgres.go:160`, `mysql.go:154`

Generates `UPDATE "table" SET "col" = $1 WHERE $2` — a placeholder in the WHERE position with no column or operator. Every UPDATE operation produces invalid SQL.

**Fix:** Require a `where` clause string in config for UPDATE operations.

### CRIT-3: DELETE with no WHERE clause

**Files:** `postgres.go:164-165`, `mysql.go:158-159`

Generates `DELETE FROM "table"` with params passed but never used in the query. Every DELETE operation wipes the entire table.

**Fix:** Require a `where` clause in config for DELETE, error if absent.

### CRIT-4: HTTP pagination infinite loop

**File:** `internal/nodes/http/request.go:146`

In `executePaginated`, `break` inside a `switch` breaks the switch, not the `for` loop. Empty array response causes infinite requests to the remote server.

**Fix:** Use a labeled break: `break outer`.

### CRIT-5: Merge nodes silently skipped in diamond-graph DAGs

**File:** `internal/workflow/execution.go:88-105`

In a diamond graph (A→B, A→C, B→merge, C→merge), the merge node may be visited in topological order before both predecessors have decremented `mergeWaiting`, causing it to be permanently skipped. The merge node is never revisited.

**Fix:** After processing all nodes in topological order, re-check any merge nodes that still have `mergeWaiting == 0` but were skipped on their first visit.

### CRIT-6: Context leak — `dbCtx()` discards cancel function

**File:** `internal/workflow/execution.go:370`

Every node execution leaks a goroutine-backed timer (10s lifetime). In a workflow with many nodes running frequently, this accumulates unboundedly.

**Fix:** Return and defer the cancel function: `ctx, cancel := dbCtx(); defer cancel()`.

---

## 5. Important Issues

### Security

| ID | Issue | File | Severity |
|----|-------|------|----------|
| SEC-1 | `ssh.go` uses `InsecureIgnoreHostKey` — MITM vulnerability | `http/ssh.go:71` | High |
| SEC-2 | `execute_command.go` — arbitrary shell execution, no sandboxing | `system/execute_command.go:42-52` | High |
| SEC-3 | Webhook CORS `Access-Control-Allow-Origin: *` on all routes including unauthenticated | `workflow/webhook_server.go:120-130` | High |
| SEC-4 | XML element name injection via unsanitized map keys | `data/xml.go:147` | Medium |
| SEC-5 | Temp files written to `/tmp` with 0644 (world-readable) | `service/openrouter.go:190`, `service/huggingface.go:97` | Medium |

### Correctness

| ID | Issue | File | Severity |
|----|-------|------|----------|
| BUG-1 | `xml.go:121` — unchecked type assertion causes panic | `data/xml.go:121` | High |
| BUG-2 | `execute_command.go:51` — setting `cmd.Env` strips parent process environment | `system/execute_command.go:49-51` | High |
| BUG-3 | `email_send.go:156-159` — malformed MIME when attachments present | `comm/email_send.go:156-159` | High |
| BUG-4 | `telegram.go:119` — nil pointer dereference on `u.Message.From` for channel posts | `comm/telegram.go:119` | High |
| BUG-5 | `github.go` — `create_pr` missing required `head`/`base` fields → always 422 | `service/github.go:210-221` | High |
| BUG-6 | `shopify.go` — `get_product/order/customer` silently return empty on success | `service/shopify.go:99-101,167-169,205-207` | Medium |
| BUG-7 | `huggingface.go` — sends `max_new_tokens: 0` when not configured | `service/huggingface.go:121` | Medium |
| BUG-8 | `people/save.go:59` — `tx.Rollback()` not deferred; commit failure leaks transaction | `people/save.go:59-62` | Medium |
| BUG-9 | `sort.go:47,81` — `sortErr` declared but never set; dead error path | `control/sort.go:47,81` | Low |
| BUG-10 | `compare_datasets.go:81-98` — non-deterministic output order from map iteration | `control/compare_datasets.go:81-98` | Low |

### Performance

| ID | Issue | File | Severity |
|----|-------|------|----------|
| PERF-1 | All DB nodes open a new connection per `Execute()` — no pooling | `db/*.go:~40-44` | Medium |
| PERF-2 | Redis `KEYS` command blocks server in production | `db/redis.go:193-204` | Medium |
| PERF-3 | All service nodes use `http.DefaultClient` with no timeout | All `service/*.go` | Medium |

### Encoding

| ID | Issue | File | Severity |
|----|-------|------|----------|
| ENC-1 | `gmailURLEncode` / `driveURLEncode` — byte-by-byte processing breaks multi-byte UTF-8 | `service/gmail.go:190-205`, `service/google_drive.go:298-313` | Medium |
| ENC-2 | `salesforceURLEncode` — JSON marshal pre-processing corrupts URL encoding | `service/salesforce.go:167-189` | Medium |

### Engine

| ID | Issue | File | Severity |
|----|-------|------|----------|
| ENG-1 | `handleTrigger` doesn't check `IsActive` — inactive workflows can execute via stale triggers | `workflow/engine.go:266` | Medium |
| ENG-2 | TOCTOU race in `activateSchedule`/`activateWebhook` — duplicate cron/webhook registration | `workflow/trigger_manager.go:109-157` | Medium |
| ENG-3 | `HybridWorkflowStore.SaveWorkflowNodes` writes to SQLite only — file store diverges | `workflow/hybrid_store.go:95-101` | Medium |

---

## 6. UI/CLI Gaps

### Missing from UI Palette

These nodes are registered in Go and runnable via `monoes node run` but **not shown** in the UI's Palette component:

| Platform | Missing Nodes |
|----------|--------------|
| Instagram | `reply_to_comments`, `list_post_comments`, `list_user_posts` |
| LinkedIn | `list_user_posts`, `like_posts`, `comment_on_posts`, `list_post_comments`, `like_comments` |
| TikTok | All 9: `list_user_videos`, `like_video`, `comment_on_video`, `list_video_comments`, `like_comment`, `follow_user`, `stitch_video`, `duet_video`, `share_video` |

### CREDENTIAL_PLATFORMS vs Schema Mismatch

These nodes have a credential dropdown in the UI but their schema has `credential_platform: null` and uses inline key fields — creating a confusing double-input:

| Node | Schema has inline key field | CREDENTIAL_PLATFORMS entry |
|------|---------------------------|---------------------------|
| `service.stripe` | `api_key` (password) | `'stripe'` |
| `service.shopify` | `shop`, `access_token` | `'shopify'` |
| `service.salesforce` | `instance_url`, `access_token` | `'salesforce'` |
| `service.hubspot` | `access_token` | `'hubspot'` |
| `db.postgres` | `connection_string` | `'postgresql'` |
| `db.mysql` | `connection_string` | `'mysql'` |
| `db.mongodb` | `connection_string` | `'mongodb'` |
| `db.redis` | `address`, `password` | `'redis'` |
| `comm.twilio` | `account_sid`, `auth_token` | `'twilio'` |
| `comm.whatsapp` | `access_token`, `phone_id` | `'whatsapp'` |

### Broken Navigation

`NodeRunner.jsx:591` — "Manage credentials →" link navigates to `'credentials'` (non-existent route). **Already fixed** in the P0 commit to `'connections'`.

### Code Duplication in Service Nodes

`google_sheets.go`, `gmail.go`, `google_drive.go` each implement their own private HTTP helper (`sheetsRequest`, `gmailRequest`, `driveRequest`) that duplicates `apiRequest` from `helpers.go`. Future fixes to one won't propagate to others.

---

## 7. Per-Category Review

### Service Nodes (16 files)

| Node | Status | Issues |
|------|--------|--------|
| google_sheets | Working | Duplicate `sheetsRequest` helper |
| gmail | Has bugs | Missing `to` validation; UTF-8 encoding bug |
| google_drive | Has bugs | UTF-8 encoding bug; hardcoded public share |
| github | Has bugs | `create_pr` missing `head`/`base` fields; double auth header |
| notion | Clean | — |
| airtable | Clean | Cleanest implementation — uses shared helpers properly |
| asana | Clean | — |
| hubspot | Clean | Missing `get_deal`/`get_company` (feature gap) |
| salesforce | Has bugs | URL encoding corruption via JSON marshal |
| shopify | Has bugs | Silent empty returns on get operations |
| stripe | Clean | — |
| linear | Minor | Unused GraphQL query variables |
| jira | Clean | — |
| openrouter | Minor | World-readable temp files |
| huggingface | Has bugs | `max_new_tokens: 0` breaks generation; world-readable temp files |

### Control Nodes (14 files) — Mostly Clean

All logic nodes (if, switch, filter, merge, set, limit, remove_duplicates, split_in_batches, stop_error, wait, aggregate) are correct. Two issues: `sort.go` has dead error path, `compare_datasets.go` has non-deterministic output. `code.go` (Goja JS) has no memory sandboxing.

### Data Nodes (8 files)

`xml.go` has panic risk and element name injection. All others (compression, crypto, datetime, html, markdown, spreadsheet, write_binary_file) are sound.

### DB Nodes (4 files) — Critical Issues

SQL injection, broken UPDATE/DELETE, no connection pooling, Redis KEYS command.

### Comm Nodes (7 files)

Email MIME bug, Telegram nil dereference. Slack, Discord, WhatsApp, Twilio are clean.

### System Nodes (2 files)

`execute_command.go` has env stripping bug and no sandboxing. `rss_read.go` is clean.

### HTTP Nodes (3 files)

`request.go` has critical pagination infinite loop. `ssh.go` has InsecureIgnoreHostKey. `ftp.go` is clean.

### Workflow Engine

Merge-node skip bug in diamond graphs, context leak, trigger race conditions, hybrid store divergence.

---

## 8. Recommended Fixes (Priority Order)

### P0 — Critical / Data Loss / Security

| # | Fix | Files | Effort |
|---|-----|-------|--------|
| 1 | Fix SQL injection in DB nodes — validate identifiers | `db/postgres.go`, `db/mysql.go` | Small |
| 2 | Fix broken UPDATE WHERE / DELETE no-WHERE | `db/postgres.go`, `db/mysql.go` | Small |
| 3 | Fix HTTP pagination infinite loop | `http/request.go:146` | 1 line (labeled break) |
| 4 | Fix merge-node skip in diamond DAGs | `workflow/execution.go:88-105` | Medium |
| 5 | Fix context leak in `dbCtx()` | `workflow/execution.go:370` | 1 line |
| 6 | Bind SSH to known_hosts or error | `http/ssh.go:71` | Small |

### P1 — Bugs Causing Runtime Failures

| # | Fix | Files | Effort |
|---|-----|-------|--------|
| 7 | Fix `execute_command.go` env stripping | `system/execute_command.go:49-51` | 1 line |
| 8 | Fix `xml.go` panic on type assertion | `data/xml.go:121` | 1 line |
| 9 | Fix Telegram nil dereference | `comm/telegram.go:119` | 2 lines |
| 10 | Fix email MIME when attachments present | `comm/email_send.go:156-159` | Medium |
| 11 | Fix `github.go` create_pr missing head/base | `service/github.go:210-221` | Small |
| 12 | Fix `shopify.go` silent empty returns | `service/shopify.go` (3 locations) | Small |
| 13 | Fix `huggingface.go` max_new_tokens: 0 | `service/huggingface.go:121` | Small |
| 14 | Defer tx.Rollback in people/save.go | `people/save.go:59` | 1 line |

### P2 — Quality / Performance

| # | Fix | Files | Effort |
|---|-----|-------|--------|
| 15 | Replace custom URL encoders with `url.QueryEscape` | `gmail.go`, `google_drive.go`, `salesforce.go` | Small |
| 16 | Add HTTP client timeout for service nodes | `service/helpers.go` | Small |
| 17 | Add connection pooling for DB nodes | `db/*.go` | Medium |
| 18 | Replace Redis KEYS with SCAN | `db/redis.go:193-204` | Small |
| 19 | Use shared `apiRequest` in Google nodes | `google_sheets.go`, `gmail.go`, `google_drive.go` | Small |
| 20 | Add missing browser nodes to UI palette | `wails-app/app.go:GetWorkflowNodeTypes` | Small |
| 21 | Fix CREDENTIAL_PLATFORMS/schema mismatch | `NodeRunner.jsx:485-510` | Medium |
| 22 | Add `IsActive` guard in `handleTrigger` | `workflow/engine.go:266` | 1 line |
| 23 | Fix TOCTOU race in trigger activation | `workflow/trigger_manager.go:109-157` | Small |
| 24 | Restrict webhook CORS | `workflow/webhook_server.go:120-130` | Small |
| 25 | Use `os.CreateTemp` for generated images | `openrouter.go`, `huggingface.go` | 1 line each |

---

## Key Files Reference

| File | Purpose |
|------|---------|
| `internal/workflow/execution.go` | Main BFS execution loop |
| `internal/workflow/dag.go` | Kahn's algorithm DAG construction |
| `internal/workflow/registry.go` | Node type registry |
| `internal/workflow/schema_loader.go` | Schema loading + fallback |
| `internal/workflow/schemas/` | 74 embedded JSON schema files |
| `internal/workflow/webhook_server.go` | Webhook trigger server |
| `internal/workflow/trigger_manager.go` | Schedule + webhook trigger management |
| `internal/workflow/hybrid_store.go` | File + SQLite hybrid storage |
| `internal/nodes/browser_adapter.go` | Browser node execution adapter |
| `internal/nodes/browser_register.go` | Dynamic browser node registration |
| `internal/nodes/service/helpers.go` | Shared HTTP helpers for service nodes |
| `internal/nodes/db/postgres.go` | PostgreSQL node (has critical SQL injection) |
| `internal/nodes/db/mysql.go` | MySQL node (has critical SQL injection) |
| `internal/nodes/http/request.go` | HTTP request node (has infinite loop bug) |
| `wails-app/app.go` | GetWorkflowNodeTypes, RunNode, runBrowserNode |
| `wails-app/frontend/src/pages/NodeRunner.jsx` | UI: Palette, Inspector, CREDENTIAL_PLATFORMS |
| `cmd/monoes/node.go` | CLI: buildNodeRegistry, node run command |
