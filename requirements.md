# Monoes Workflow System — Requirements Document

**Version:** 1.0
**Date:** 2026-03-10
**Project:** Mono Agent — Go automation backend + Wails v2 desktop UI
**Module:** `github.com/monoes/mono-agent`

---

## Table of Contents

1. [Project Context and Constraints](#1-project-context-and-constraints)
2. [Workflow Engine Core](#2-workflow-engine-core)
3. [Trigger System](#3-trigger-system)
4. [Error Handling](#4-error-handling)
5. [Queue and Concurrency](#5-queue-and-concurrency)
6. [New Platform Actions](#6-new-platform-actions)
7. [API Surface](#7-api-surface)
8. [UI Requirements](#8-ui-requirements)
9. [Data Model Changes](#9-data-model-changes)
10. [Non-Functional Requirements](#10-non-functional-requirements)

---

## 1. Project Context and Constraints

### 1.1 Existing System Summary

The codebase is a Go 1.24 automation agent with the following structural components:

- **Action executor** (`internal/action/`): JSON-defined action files (embedded in `data/actions/<platform>/<type>.json`) that describe steps executed against a Rod/Chromium browser page. Step types: `navigate`, `wait`, `refresh`, `find_element`, `click`, `type`, `upload`, `scroll`, `hover`, `extract_text`, `extract_attribute`, `extract_multiple`, `condition`, `update_progress`, `save_data`, `mark_failed`, `log`, `call_bot_method`.
- **Platform bots** (`internal/bot/`): Instagram, LinkedIn, TikTok, X/Twitter, Telegram, Email. Each implements `BotAdapter` interface and registers via `init()` in `PlatformRegistry`.
- **Storage** (`internal/storage/`): SQLite via `modernc.org/sqlite` with numbered SQL migration files. Models: `Action`, `ActionTarget`, `Person`, `SocialList`, `SocialListItem`, `Thread`, `Template`, `ConfigEntry`.
- **Action states**: PENDING, RUNNING, PAUSED, COMPLETED, FAILED, CANCELLED.
- **Scheduler** (`internal/scheduler/`): `robfig/cron v3` wrapping, per-action cron scheduling.
- **CLI** (`cmd/monoes/`): cobra-based commands including `run`, `schedule`, `login`, `list`, etc.
- **Wails v2 UI** (`wails-app/`): React/JSX frontend that calls Go methods bound through `app.go`. Actions are executed by shelling out to the `monoes` CLI binary via `exec.Command`.
- **3-tier fallback system**: Every DOM interaction in action JSON follows: Tier 1 = `call_bot_method` (Go bot method, `onError: skip`), Tier 2 = `find_element` with `xpath`/`alternatives` (`onError: skip`), Tier 3 = `find_element` with `configKey` for AI-generated selectors (`onError: mark_failed`). Condition steps with `not_exists` gate progression between tiers.

### 1.2 Backward Compatibility Constraint

The existing `Action` system (CRUD, scheduling, execution via `monoes run <id>`) MUST continue to work unchanged after the workflow system is added. Existing DB tables MUST NOT be altered. New tables are additive only. The workflow system is a new layer on top of — not a replacement for — the action system.

### 1.3 What Is Being Built

A workflow system inspired by n8n's execution model, adapted to the Go/Wails/Rod architecture. Workflows are directed acyclic graphs (DAGs) of nodes that execute in order, passing data from node to node. Each node is either a Trigger, an Action (wrapping an existing platform action), a Control node (IF, Switch, Merge, Wait, Loop, Stop), or a Transform node (Set, Code). Workflows are stored in SQLite, executed in-process using goroutines (no Redis, no external queue), and managed through new Wails UI pages and new REST-like Wails bound methods.

---

## 2. Workflow Engine Core

### 2.1 Workflow Data Model

A `Workflow` is a named, versioned DAG stored in the `workflows` table. It contains:

- **Nodes**: Each node has a unique ID within the workflow, a type, a position (for canvas rendering), a configuration blob, and optional display metadata.
- **Connections**: Directed edges from one node's output handle to another node's input handle.
- **Execution order**: Determined at runtime by topological sort of the DAG starting from trigger nodes.

**Workflow fields:**
```
id          TEXT PRIMARY KEY  (UUID v4)
name        TEXT NOT NULL
description TEXT
is_active   INTEGER NOT NULL DEFAULT 0   -- 0=inactive, 1=active
version     INTEGER NOT NULL DEFAULT 1
created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
```

**Node fields** (stored as JSON in `workflow_nodes` table, one row per node):
```
id          TEXT PRIMARY KEY  (UUID v4)
workflow_id TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE
node_type   TEXT NOT NULL     -- see node type registry
name        TEXT NOT NULL     -- display name, user-editable
config      TEXT NOT NULL DEFAULT '{}'  -- JSON blob, type-specific
position_x  REAL NOT NULL DEFAULT 0    -- canvas X coordinate
position_y  REAL NOT NULL DEFAULT 0    -- canvas Y coordinate
disabled    INTEGER NOT NULL DEFAULT 0
created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
```

**Connection fields** (stored in `workflow_connections` table):
```
id              TEXT PRIMARY KEY  (UUID v4)
workflow_id     TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE
source_node_id  TEXT NOT NULL REFERENCES workflow_nodes(id) ON DELETE CASCADE
source_handle   TEXT NOT NULL DEFAULT 'main'   -- output handle name: "main", "error", "true", "false"
target_node_id  TEXT NOT NULL REFERENCES workflow_nodes(id) ON DELETE CASCADE
target_handle   TEXT NOT NULL DEFAULT 'main'   -- input handle name: "main"
position        INTEGER NOT NULL DEFAULT 0     -- ordering when multiple connections share the same target handle
UNIQUE(source_node_id, source_handle, target_node_id, target_handle)
```

### 2.2 Node Type Registry

All node types are identified by a string constant. The system must support the following types at initial release:

**Trigger nodes** (one per workflow, always the root; a workflow may have multiple trigger nodes if they are disconnected entry points):
- `trigger.manual` — triggered by user pressing "Run" in the UI or calling the manual execution API.
- `trigger.schedule` — triggered by a cron expression.
- `trigger.webhook` — triggered by an incoming HTTP POST to a unique URL.

**Action nodes** (wrap existing platform actions):
- `action.instagram.<TYPE>` — e.g., `action.instagram.KEYWORD_SEARCH`
- `action.linkedin.<TYPE>`
- `action.tiktok.<TYPE>`
- `action.x.<TYPE>`

  The `<TYPE>` segment maps directly to the action type string recognized by the existing `ActionExecutor` and the `ActionLoader` (which loads `data/actions/<platform>/<type>.json`). All action types listed in section 6 must be supported.

**Control nodes:**
- `control.if` — evaluates a single boolean expression; has two output handles: `true` and `false`.
- `control.switch` — evaluates an expression and routes to one of N output handles by matching a value against case values; has one output handle per case plus an optional `default` handle.
- `control.merge` — waits for all connected input handles to receive data, then emits a single merged output item. The number of expected inputs is determined by the number of incoming connections. Input data is merged by appending arrays: the output is `{ branches: [outputFromBranch1, outputFromBranch2, ...] }`.
- `control.wait` — pauses workflow execution for a specified duration (seconds, up to 3600). Does not block other concurrent workflow executions. Implemented via `time.Sleep` in the execution goroutine.
- `control.loop` — iterates over an array value from the input data and executes connected child nodes for each item. Loop body is a sub-chain of nodes. The loop node has one `loop_item` output handle (fires once per item) and one `done` output handle (fires once after all items processed).
- `control.stop_error` — terminates the workflow execution immediately with a custom error message. Sets execution status to FAILED with the provided message stored in the execution log.
- `control.noop` — passes input data through unchanged. Used as a placeholder or junction.

**Transform nodes:**
- `transform.set` — sets or overwrites fields in the item data. Config contains a list of assignments: `[{key: "field.path", value: "expression or literal"}]`. Uses the expression engine.
- `transform.code` — executes a Go template script (see section 2.5) against the input item and produces output items. Input: one item. Output: one or more items. Script must return a `[]map[string]interface{}`.

### 2.3 Node Configuration Schemas

Each node type has a typed configuration structure stored as JSON in `workflow_nodes.config`. The engine validates config at workflow save time. Missing required fields cause a validation error that is returned to the caller; the workflow is not saved in an invalid state.

**trigger.manual config:**
```json
{
  "sample_data": {}    // optional: pre-seeded data injected as the first node output when manually triggered
}
```

**trigger.schedule config:**
```json
{
  "cron_expression": "0 9 * * 1-5",   // required: standard 5-field cron; no seconds field
  "timezone": "UTC"                    // optional: IANA timezone name, default UTC
}
```

**trigger.webhook config:**
```json
{
  "http_method": "POST",               // required: GET or POST
  "path_suffix": "my-webhook",         // optional: appended to base webhook URL; auto-generated UUID if empty
  "require_auth": false,               // optional: if true, expect X-Monoes-Token header matching webhook_secret
  "webhook_secret": ""                 // optional: HMAC secret for request validation
}
```

**action.<platform>.<TYPE> config:**
```json
{
  "platform": "instagram",             // required
  "action_type": "KEYWORD_SEARCH",     // required
  "session_username": "",              // optional: if empty, uses the first active session for platform
  "params": {                          // required: maps to action.Params; type-specific
    "keyword": "{{$json.keyword}}",    // may use expression syntax
    "max_results": 100
  },
  "retry_policy": {                    // optional: overrides workflow-level defaults
    "max_retries": 3,
    "backoff": "exponential",
    "initial_delay_seconds": 5,
    "max_delay_seconds": 300
  },
  "on_error": "stop"                   // "stop" | "continue" | "error_branch"
}
```

**control.if config:**
```json
{
  "condition": "{{$json.count}} > 10"  // required: expression that evaluates to boolean
}
```

**control.switch config:**
```json
{
  "expression": "{{$json.platform}}",  // required: value to switch on
  "cases": [
    {"value": "instagram", "handle": "instagram"},
    {"value": "linkedin", "handle": "linkedin"}
  ],
  "default_handle": "other"            // optional: handle name for non-matching cases
}
```

**control.merge config:**
```json
{
  "mode": "append"    // "append": merge all branch outputs into one array; "first": use first to arrive
}
```

**control.wait config:**
```json
{
  "seconds": 30       // required: 1–3600
}
```

**control.loop config:**
```json
{
  "input_field": "items",   // required: field path in input data containing the array to iterate over
  "item_var": "item"        // required: variable name to bind each iteration's value
}
```

**control.stop_error config:**
```json
{
  "message": "{{$json.error_reason}}"   // required: error message, may use expression
}
```

**control.noop config:** `{}` (no fields required).

**transform.set config:**
```json
{
  "assignments": [
    {"key": "result.count", "value": "{{$json.items.length}}"},
    {"key": "processed", "value": true}
  ]
}
```

**transform.code config:**
```json
{
  "script": "// Go text/template syntax\n{{ range .items }}...{{ end }}"  // required
}
```

### 2.4 Execution Order and Data Flow

**Topological resolution:** Before execution begins, the engine performs a topological sort (Kahn's algorithm) of the workflow DAG. Cycles are detected during save (not at runtime). If a cycle is detected, saving the workflow returns an error: `"workflow contains a cycle involving nodes: [nodeA, nodeB]"`.

**Execution phases:**
1. Load workflow definition from DB (nodes + connections).
2. Identify trigger node(s) that match the current trigger event. If multiple disconnected trigger branches exist, all matching triggers fire independently.
3. For the triggered branch, start execution from the trigger node.
4. Maintain an execution stack: a slice of pending `(node, inputData)` tuples.
5. Pop the next tuple, execute the node, collect outputs.
6. For each output handle with connections, push `(targetNode, outputData)` onto the stack.
7. Continue until the stack is empty or a terminal node (stop_error, or leaf with no outgoing connections) is reached.

**Data passing between nodes:**

Each node receives a `NodeInput` structure and produces a `NodeOutput` structure:

```go
type NodeInput struct {
    Items  []map[string]interface{}  // output items from the previous node
    Source string                    // source node ID
}

type NodeOutput struct {
    Handle string                    // which output handle emitted this data (e.g. "main", "true", "error")
    Items  []map[string]interface{}  // output items to pass to connected nodes
    Error  *NodeError                // non-nil if node encountered an error
}

type NodeError struct {
    Message   string
    NodeID    string
    Timestamp time.Time
}
```

The convention is that a node receives all its inputs as a slice of items, executes, and emits items on one or more output handles.

**Action node data flow:** When an action node executes, the `params` config values are resolved through the expression engine (substituting `{{$json.*}}` references) using the first item in the input slice. The action runs via the existing `ActionExecutor`. Upon completion, the `ExecutionResult.ExtractedItems` array is converted to the action node's output items. Each extracted item becomes one element in the output `Items` slice.

**Control.loop data flow:** The loop node reads the field named by `input_field` from the first input item. It expects an array. For each element, it binds the element to `item_var` and executes the loop body. The loop body nodes receive a single-item input where the item is `{"<item_var>": <element_value>}`. After all iterations complete, the loop node emits on `done` handle with the original input item plus a `loop_results` field containing all items emitted by the last node in the loop body.

**Control.merge data flow:** The merge node buffers input from each connected source. It waits until all connected inputs have delivered at least one batch. Once all inputs are received, it emits one item on `main` with the field `branches` set to an array of the received item arrays. Maximum wait time for a merge node is governed by the workflow execution timeout (see section 4.3). If the timeout expires before all branches arrive, the workflow execution transitions to FAILED state.

### 2.5 Expression Engine

Expressions are evaluated using Go's `text/template` package extended with a custom function map. Expressions appear in node config fields as `{{expression}}`.

**Syntax:**
- `{{$json.fieldName}}` — access a field from the current item's top-level map.
- `{{$json.nested.field}}` — dot-path access (recursive map lookup by splitting on `.`).
- `{{$json.arrayField[0]}}` — index access on arrays (not supported in text/template natively; expose as a template function `index`).
- `{{$node["NodeName"].json.field}}` — access the output of a previously executed named node. The node name is the user-set `name` field in `workflow_nodes`. Only nodes that have already executed in the current execution are accessible.
- `{{$workflow.id}}` — the current workflow ID.
- `{{$execution.id}}` — the current execution ID.
- `{{env "VAR_NAME"}}` — access an OS environment variable (read-only, from `os.Getenv`).
- `{{now}}` — current UTC time as `time.Time`.
- `{{len $json.items}}` — standard template `len` function.
- Arithmetic and comparison via standard template operators: `eq`, `ne`, `lt`, `gt`, `le`, `ge`, `and`, `or`, `not`.

**Evaluation context for a node:**
```go
type ExpressionContext struct {
    JSON      map[string]interface{}            // current item data
    Node      map[string]NodeOutputSnapshot     // keyed by node name
    Workflow  WorkflowSnapshot
    Execution ExecutionSnapshot
}
```

**Error behavior:** If an expression fails to evaluate (field not found, type mismatch), the engine returns the empty string `""` and logs a warning. It does not abort the workflow. If a required config field evaluates to empty string when a non-empty value is required (e.g., `action_type`), the node fails with error: `"required config field '<field>' resolved to empty string"`.

**Security:** The expression engine runs in the same process with access to all Go functions registered in the template FuncMap. There is no sandboxing. The FuncMap must be explicitly limited to safe functions: string manipulation, math, date/time, map access, slice operations. OS exec, file I/O, and network calls must NOT be registered in the FuncMap.

### 2.6 Workflow Lifecycle States

A workflow record has an `is_active` flag (INTEGER 0/1), not a state enum, because the workflow definition is separate from its execution instances:

- **Inactive (is_active = 0)**: Default state. Workflow is saved but schedule triggers and webhooks are not registered. Manual triggers still work (they do not require activation).
- **Active (is_active = 1)**: Schedule triggers have active cron jobs registered in the Scheduler. Webhook triggers have registered URL paths. Setting a workflow active runs validation first; if validation fails, activation is rejected with an error.

Activation validation checks:
1. Workflow must have at least one node.
2. Workflow must have at least one trigger node.
3. All `action.<platform>.<TYPE>` nodes must have a valid `platform` and `action_type` that resolves to an existing action JSON file in `data/actions/`.
4. Required config fields must be non-empty (literal values; expressions are not evaluated at validation time).
5. No cycle in the DAG.

### 2.7 Execution Model

Each workflow execution is a Go struct (`WorkflowExecution`) that runs within a single goroutine. It maintains a local state map, the execution stack, and accumulated run data.

```go
type WorkflowExecution struct {
    ID          string
    WorkflowID  string
    TriggerType string                       // "manual" | "schedule" | "webhook"
    Status      string                       // "RUNNING" | "COMPLETED" | "FAILED" | "CANCELLED"
    StartedAt   time.Time
    FinishedAt  *time.Time
    NodeRunData map[string][]NodeRunRecord   // keyed by node ID, slice indexed by run index
    Error       *ExecutionError
    cancelFunc  context.CancelFunc
}

type NodeRunRecord struct {
    RunIndex    int
    StartedAt   time.Time
    FinishedAt  time.Time
    Status      string               // "SUCCESS" | "FAILED" | "SKIPPED"
    InputItems  []map[string]interface{}
    OutputItems []map[string]interface{}
    Error       string
}
```

The `WorkflowExecution` is persisted to the `workflow_executions` table at start. Node run records are persisted to `workflow_execution_nodes` at each node completion. Execution status is updated atomically in the DB at each transition.

---

## 3. Trigger System

### 3.1 Manual Trigger

**Purpose:** Start a workflow execution immediately from the UI or via direct API call.

**Behavior:**
1. UI calls the Wails method `ExecuteWorkflow(workflowID string, inputData map[string]interface{}) (string, error)` which returns the execution ID.
2. The engine creates a new `WorkflowExecution` record in DB with status RUNNING.
3. Execution starts in a new goroutine. The calling goroutine returns the execution ID immediately (non-blocking).
4. The trigger node's `sample_data` (if configured) is merged with `inputData` (caller-supplied values override sample_data). The merged map becomes the first node's output items: `[{"triggeredBy": "manual", "timestamp": "<ISO8601>", ...mergedData}]`.
5. If the workflow is already executing (has a RUNNING execution), the new execution starts anyway — concurrent manual executions of the same workflow are allowed, subject to global concurrency limits (section 5).

**Validation:** Workflow must exist and not be disabled (disabled = having `is_active = 0` AND zero completed executions is acceptable; the check is only that the workflow definition is valid per section 2.6 validation rules).

### 3.2 Schedule Trigger

**Purpose:** Start a workflow on a recurring cron schedule.

**Behavior:**
1. When a workflow with a `trigger.schedule` node is activated (`is_active` set to 1), the engine calls `Scheduler.ScheduleWorkflow(workflowID, cronExpr, timezone)`.
2. The scheduler registers a cron job using the existing `robfig/cron v3` instance. Cron expression is 5-field standard format (minute, hour, day-of-month, month, day-of-week). Seconds-level precision is not supported for workflow triggers.
3. When the cron job fires, it calls `WorkflowEngine.TriggerWorkflow(workflowID, "schedule", triggerData)` where `triggerData` is `{"triggeredBy": "schedule", "timestamp": "<ISO8601>", "cronExpression": "<expr>"}`.
4. When a workflow is deactivated (`is_active` set to 0), the scheduler removes the cron job immediately.
5. If the workflow is deleted while active, the cron job is removed before deletion.

**Timezone handling:** If `timezone` is not empty, the cron job is created with `cron.WithLocation(loc)` where `loc` is parsed from the IANA timezone name using `time.LoadLocation`. If parsing fails, activation returns error: `"invalid timezone '<value>': <parse error>"`.

**Cron validation:** The cron expression is validated at activation time by attempting to parse it with `robfig/cron`. If invalid, activation returns error: `"invalid cron expression '<expr>': <parse error>"`.

**Missed schedule behavior:** If the process was stopped and misses a scheduled trigger time, the cron job simply does not fire for missed times. No catch-up execution occurs.

### 3.3 Webhook Trigger

**Purpose:** Start a workflow when an external HTTP request arrives at a unique URL.

**Architecture:** A new HTTP server (separate from the Wails embedded webserver) listens on a configurable port (default 9321, configurable via `settings` table key `webhook_port`). This server is started when the Monoes application starts and shut down cleanly on exit.

**URL structure:** `POST http://localhost:<port>/webhook/<path_suffix>`
The `path_suffix` is taken from the trigger node config. If not configured, it defaults to the workflow ID. Each workflow can have at most one active webhook trigger at a given path_suffix. Duplicate paths cause activation to fail with: `"webhook path '<path>' is already registered by workflow '<id>'"`.

**Request handling:**
1. Incoming request body is parsed as JSON. If body is not valid JSON, the entire raw body string is set as the value of a field `rawBody` in the trigger data.
2. Parsed JSON (or `{"rawBody": "<...>"}`) becomes the input item for the trigger node: `{"triggeredBy": "webhook", "timestamp": "<ISO8601>", "method": "POST", "headers": {...}, "body": <parsed_json_or_rawBody_object>}`.
3. Workflow execution is started asynchronously. The HTTP server responds immediately with `202 Accepted` and `{"executionId": "<id>"}`.
4. If `require_auth = true`, the request must include `X-Monoes-Token: <HMAC-SHA256(body, webhook_secret) in hex>`. If the token is missing or does not match, return `401 Unauthorized` with `{"error": "invalid token"}` and do not start execution.
5. GET requests to the webhook URL return `200 OK` with `{"status": "active", "workflowId": "<id>"}` for health-check purposes.

**Registration lifecycle:** When a workflow with a webhook trigger is activated, the path is registered in-memory in the webhook server's router (a simple `sync.Map` keyed by path). When deactivated, the path is removed. The registration state is NOT persisted across restarts; on application startup, the engine iterates all active workflows and re-registers all webhook paths.

### 3.4 Trigger Data Injection

For all trigger types, the trigger node's output is a single-item array:
```go
[]map[string]interface{}{
    {
        "triggeredBy": "manual" | "schedule" | "webhook",
        "timestamp":   time.Now().UTC().Format(time.RFC3339),
        // plus trigger-type-specific fields described above
    },
}
```
This becomes the `NodeInput.Items` for the first connected node.

---

## 4. Error Handling

### 4.1 Node-Level Error Behavior

Each action node config has an `on_error` field with one of three values:

- **`"stop"`** (default): When the node fails (all retry attempts exhausted), the workflow execution transitions to FAILED. The error message is stored in `workflow_executions.error_message`. No further nodes execute. The execution status is set to FAILED in the DB.
- **`"continue"`**: When the node fails, the error is logged in the node's run record, but execution continues. The node emits its input data unchanged on the `main` output handle (pass-through behavior). Downstream nodes receive the original input as if the node had succeeded but produced no transformation.
- **`"error_branch"`**: When the node fails, the error is not propagated on the `main` handle. Instead, the node emits on the `error` output handle with the item: `{"error": "<error message>", "failedNodeId": "<id>", "failedNodeName": "<name>", "originalInput": <input item>}`. If no node is connected to the `error` handle, the behavior falls back to `"stop"`.

Control nodes (`control.if`, `control.switch`, etc.) do not have `on_error`; they always use `"stop"` behavior on failure.

### 4.2 Node Retry Policy

Each action node config has an optional `retry_policy` object. If absent, the workflow-level defaults apply. Workflow-level defaults are stored in the `workflow_nodes` table's `config` field for the trigger node:

```json
{
  "default_retry_policy": {
    "max_retries": 0,
    "backoff": "fixed",
    "initial_delay_seconds": 10,
    "max_delay_seconds": 300
  }
}
```

**Node-level retry policy fields:**
- `max_retries` (integer, 0–10, default 0): Number of retry attempts after the first failure. 0 means no retries.
- `backoff` (string, `"fixed"` | `"exponential"`, default `"fixed"`): Delay calculation strategy.
- `initial_delay_seconds` (integer, 1–300, default 10): Delay before first retry (fixed) or starting delay (exponential).
- `max_delay_seconds` (integer, 1–3600, default 300): Cap on delay between retries (exponential only).

**Delay calculation:**
- Fixed: every retry waits exactly `initial_delay_seconds`.
- Exponential: retry N waits `min(initial_delay_seconds * 2^(N-1), max_delay_seconds)`. For example, with initial=5 and max=300: retry 1 = 5s, retry 2 = 10s, retry 3 = 20s, retry 4 = 40s, retry 5 = 80s, retry 6+ = 160s (capped at 300 for max=300).

**Retry tracking:** The current retry attempt count is stored in memory in the `WorkflowExecution` struct, not persisted between application restarts. If the application restarts while a workflow is executing, that execution transitions to FAILED during startup recovery (see section 5.3).

**Retry log:** Each retry attempt is recorded as a separate `NodeRunRecord` with status `"FAILED"` for failed attempts and `"SUCCESS"` for the final successful attempt (if any). The retry count is visible in the execution history UI.

### 4.3 Workflow Execution Timeout

Each workflow has a global execution timeout. Default: 3600 seconds (1 hour). Configurable per-workflow via a `timeout_seconds` field in the `workflows` table.

When the timeout expires:
1. The executing goroutine's context is cancelled via `cancelFunc()`.
2. The current node's action executor receives the context cancellation and stops.
3. The workflow execution status is set to FAILED with error: `"execution timed out after <N> seconds"`.
4. Any in-progress browser action (Rod operation) is cancelled via context propagation.

### 4.4 Error Workflow

A workflow can designate another workflow as its "error workflow". This is configured by storing the error workflow's ID in the `workflows` table field `error_workflow_id`. When a workflow execution transitions to FAILED:

1. If `error_workflow_id` is set and the referenced workflow is valid and active, a new execution of the error workflow is triggered.
2. The error workflow's trigger input data is: `{"triggeredBy": "error_workflow", "failedWorkflowId": "<id>", "failedExecutionId": "<id>", "error": "<error message>", "timestamp": "<ISO8601>"}`.
3. The error workflow execution runs independently and asynchronously. Its success or failure does not affect the original failed execution.
4. Circular references (workflow A's error workflow is workflow B, and B's error workflow is A) are not validated at configuration time. The executor detects circular invocations at runtime by checking if the current execution was triggered by `"error_workflow"` — error workflows themselves do not trigger further error workflows.

### 4.5 Failed Execution Retry

Users can retry a failed workflow execution from the UI. Two retry modes:

- **Retry from beginning**: Creates a new `WorkflowExecution` record with the same trigger input data (stored in `workflow_executions.input_data`). Starts execution from the trigger node. The failed execution record remains in history with status FAILED.
- **Retry from failed node**: Not supported in v1. All retries start from the beginning.

The retry action is exposed as Wails method `RetryWorkflowExecution(executionID string) (string, error)` which returns the new execution ID.

---

## 5. Queue and Concurrency

### 5.1 Execution Queue

Workflow executions are managed by an in-process queue implemented as a buffered channel (`chan *WorkflowExecutionRequest`, capacity 1000). There is no Redis, no Bull, no external queue.

```go
type WorkflowExecutionRequest struct {
    WorkflowID  string
    TriggerType string
    InputData   map[string]interface{}
    ResultChan  chan<- WorkflowExecutionResult   // nil for fire-and-forget
}

type WorkflowExecutionResult struct {
    ExecutionID string
    Error       error
}
```

**Dispatch flow:**
1. When a trigger fires, a `WorkflowExecutionRequest` is created and pushed onto the queue channel.
2. A pool of worker goroutines (size = `max_concurrent_workflows`) reads from the channel.
3. Each worker calls `WorkflowEngine.executeWorkflow(req)` which runs the full workflow synchronously within the worker goroutine.
4. When the queue is full (1000 pending requests), new trigger invocations return error: `"workflow execution queue is full; try again later"`.

### 5.2 Concurrency Limits

**Global concurrency limit:** The maximum number of simultaneously executing workflows is configured in the `settings` table as `max_concurrent_workflows` (integer, default 3, min 1, max 20). This is the size of the worker goroutine pool. The pool is created at application startup and cannot be resized without a restart.

**Per-workflow concurrency:** Not restricted by default. A single workflow can have multiple simultaneous executions. If required in the future, per-workflow limits can be added as a `max_concurrent_executions` field in the `workflows` table (not implemented in v1).

**Browser resource contention:** Each action node that runs a browser automation action requires a browser session (a Rod `*rod.Page`). Browser sessions are associated with a platform + username combination (existing `crawler_sessions` table). The engine does NOT implement a browser session pool in v1. Multiple concurrent action nodes for the same platform/session will attempt to acquire the same session concurrently, which may cause browser state conflicts. Users must design workflows to avoid concurrent actions against the same session. A warning is logged if two action nodes for the same `session_username` attempt to run concurrently within the same execution.

### 5.3 Execution Cancellation

Cancellation is exposed at the execution level, not at the node level.

**How to cancel:** Wails method `CancelWorkflowExecution(executionID string) error`.

**Mechanism:**
1. The engine finds the running execution's `cancelFunc` in memory (stored in a `sync.Map` keyed by execution ID).
2. Calls `cancelFunc()`, which cancels the context passed to the action executor.
3. The action executor's `executeSteps` loop checks `ctx.Done()` at every iteration boundary.
4. Rod operations propagate context cancellation natively.
5. When the goroutine exits due to context cancellation, the execution status is set to CANCELLED in the DB.
6. `CancelWorkflowExecution` returns immediately. The actual cancellation is asynchronous.

**Cancellation timeout:** If the executing goroutine has not exited 30 seconds after cancellation, the status is force-set to CANCELLED in the DB by a background cleanup goroutine. The goroutine itself is not forcibly killed (Go does not support goroutine killing).

### 5.4 Startup Recovery

On application startup, the engine checks for executions in the `workflow_executions` table with status RUNNING. These are executions that were interrupted by a previous abnormal process exit. All such executions are set to FAILED with error: `"execution interrupted by process restart"`. No automatic re-execution occurs.

---

## 6. New Platform Actions

All new actions follow the 3-tier fallback pattern documented in `THREE_TIER_FALLBACK.md`. Each new action requires:
1. A Go bot method in `internal/bot/<platform>/actions.go` implementing Tier 1.
2. Registration of the method in `GetMethodByName()` in `internal/bot/<platform>/bot.go`.
3. A schema entry in `internal/config/schemas.go` for Tier 3 configKey fields.
4. An action JSON file in `data/actions/<platform>/<type>.json` following the 3-tier template.
5. Integration tests in `internal/action/<platform>_integration_test.go`.

### 6.1 LinkedIn Actions

#### KEYWORD_SEARCH (action type: `KEYWORD_SEARCH`)

**Purpose:** Search LinkedIn for people by keyword and save profiles to the `people` table.

**Input parameters:**
- `keyword` (string, required): Search keyword (e.g., "product manager").
- `max_results` (integer, optional, default 50): Maximum number of profiles to collect.
- `filters` (object, optional): LinkedIn-specific filters. Supported: `{"connection_level": "1st" | "2nd" | "3rd", "location": "<string>"}`.

**Output data shape:**
```json
{
  "extracted_count": 12,
  "people_ids": ["<uuid>", "..."]
}
```

**3-tier fallback applicability:** Full 3-tier applies to all DOM interactions (search input, result items, profile navigation).

**Tier 1 bot method:** `LinkedInKeywordSearch(page *rod.Page, keyword string, maxResults int) ([]map[string]interface{}, error)` — navigates to `linkedin.com/search/results/people/?keywords=<encoded>`, scrolls through results, extracts profile URLs. Registered as `"keyword_search"` in `GetMethodByName`.

**Schema key:** `LINKEDIN_KEYWORD_SEARCH` with fields: `search_input`, `search_submit`, `result_item`, `profile_link`.

---

#### PROFILE_INTERACTION (action type: `PROFILE_INTERACTION`)

**Purpose:** Search LinkedIn by keyword and like (and optionally comment on) matching posts.

**Input parameters:**
- `keywords` (string, required): Search keywords.
- `max_posts` (integer, optional, default 20): Maximum posts to interact with.
- `comment_text` (string, optional): If non-empty, posts a comment on each post in addition to liking.
- `target_filter` (string, optional): Filter by connection level or company.

**Output data shape:**
```json
{
  "posts_liked": 15,
  "posts_commented": 8,
  "failed_posts": 2
}
```

**3-tier fallback applicability:** Full 3-tier applies to like button, comment input, post button.

**Tier 1 bot method:** `LinkedInProfileInteraction(page *rod.Page, postURL string, commentText string) error`. Registered as `"profile_interaction"`.

**Schema key:** `LINKEDIN_POST_INTERACTION` with fields: `like_button`, `comment_textarea`, `post_button`.

---

#### PUBLISH_CONTENT (action type: `PUBLISH_CONTENT`)

**Purpose:** Publish a new post to the user's LinkedIn feed.

**Input parameters:**
- `post_text` (string, required): Text content of the post.
- `media_path` (string, optional): Absolute local file path of an image or video to attach.
- `visibility` (string, optional, default `"anyone"`): `"anyone"` | `"connections"`.

**Output data shape:**
```json
{
  "published": true,
  "post_url": "<url_if_extractable>"
}
```

**3-tier fallback applicability:** Full 3-tier applies to compose button, text input, media upload, publish button.

**Tier 1 bot method:** `LinkedInPublishContent(page *rod.Page, postText, mediaPath string) error`. Registered as `"publish_content"`.

**Schema key:** `LINKEDIN_PUBLISH_CONTENT` with fields: `start_post_button`, `post_text_input`, `media_upload_button`, `file_input`, `publish_button`.

---

### 6.2 TikTok Actions

#### KEYWORD_SEARCH (action type: `KEYWORD_SEARCH`)

**Purpose:** Search TikTok for videos matching a keyword and save video metadata.

**Input parameters:**
- `keyword` (string, required): Search keyword or hashtag.
- `max_results` (integer, optional, default 50): Maximum videos to collect.

**Output data shape:**
```json
{
  "videos": [
    {
      "video_url": "https://tiktok.com/@user/video/123",
      "author_username": "user",
      "description": "caption text",
      "like_count": "1.2K",
      "comment_count": "45"
    }
  ],
  "extracted_count": 12
}
```

**3-tier fallback applicability:** Full 3-tier applies to search input, search result items.

**Tier 1 bot method:** `TikTokKeywordSearch(page *rod.Page, keyword string, maxResults int) ([]map[string]interface{}, error)`. Registered as `"keyword_search"`.

**Schema key:** `TIKTOK_KEYWORD_SEARCH` with fields: `search_input`, `search_submit`, `result_item`, `result_link`.

---

#### PROFILE_FETCH (action type: `PROFILE_FETCH`)

**Purpose:** Export a TikTok account's followers or following list to the `people` table.

**Input parameters:**
- `profile_url` (string, required): TikTok profile URL to fetch followers/following from.
- `source_type` (string, required): `"followers"` | `"following"`.
- `max_results` (integer, optional, default 100): Maximum number of accounts to collect.
- `list_id` (string, optional): If provided, save results to this social list ID.

**Output data shape:**
```json
{
  "extracted_count": 87,
  "people_ids": ["<uuid>", "..."]
}
```

**3-tier fallback applicability:** Full 3-tier applies to follower/following tab buttons, user list items, infinite scroll trigger.

**Tier 1 bot method:** `TikTokProfileFetch(page *rod.Page, sourceType string, maxResults int) ([]map[string]interface{}, error)`. Registered as `"profile_fetch"`.

**Schema key:** `TIKTOK_PROFILE_FETCH` with fields: `followers_tab`, `following_tab`, `user_list_dialog`, `user_item_link`.

---

#### PROFILE_INTERACTION (action type: `PROFILE_INTERACTION`)

**Purpose:** Search TikTok by keyword and like (and optionally comment on) matching videos.

**Input parameters:**
- `keywords` (string, required): Search keywords or hashtag.
- `max_videos` (integer, optional, default 20): Maximum videos to interact with.
- `comment_text` (string, optional): If non-empty, post a comment on each video.
- `target_filter` (string, optional): Filter by minimum like count or account type.

**Output data shape:**
```json
{
  "videos_liked": 18,
  "videos_commented": 5,
  "failed_videos": 1
}
```

**3-tier fallback applicability:** Full 3-tier applies to like button, comment input, submit button.

**Tier 1 bot method:** `TikTokProfileInteraction(page *rod.Page, videoURL string, commentText string) error`. Registered as `"profile_interaction"`.

**Schema key:** `TIKTOK_VIDEO_INTERACTION` with fields: `like_button`, `comment_textarea`, `post_button`.

---

#### PUBLISH_CONTENT (action type: `PUBLISH_CONTENT`)

**Purpose:** Upload and publish a video to TikTok.

**Input parameters:**
- `caption` (string, required): Video caption/description.
- `video_path` (string, required): Absolute local file path of the video to upload.
- `location` (string, optional): Location tag to add to the post.

**Output data shape:**
```json
{
  "published": true,
  "post_url": "<url_if_extractable>"
}
```

**3-tier fallback applicability:** Full 3-tier applies to upload button, file input, caption input, publish button.

**Tier 1 bot method:** `TikTokPublishContent(page *rod.Page, videoPath, caption string) error`. Registered as `"publish_content"`.

**Schema key:** `TIKTOK_PUBLISH_CONTENT` with fields: `upload_button`, `file_input`, `caption_input`, `post_button`.

---

### 6.3 X/Twitter Actions

#### KEYWORD_SEARCH (action type: `KEYWORD_SEARCH`)

**Purpose:** Search X for tweets matching a keyword and save tweet data.

**Input parameters:**
- `keyword` (string, required): Search keyword or hashtag.
- `max_results` (integer, optional, default 50): Maximum tweets to collect.
- `filter` (string, optional): One of `"latest"` | `"top"` | `"people"` | `"media"` (default `"latest"`).

**Output data shape:**
```json
{
  "tweets": [
    {
      "tweet_url": "https://x.com/user/status/123",
      "author_username": "user",
      "text": "tweet text",
      "like_count": "42",
      "retweet_count": "7",
      "timestamp": "2026-01-01T10:00:00Z"
    }
  ],
  "extracted_count": 45
}
```

**3-tier fallback applicability:** Full 3-tier applies to search input, result tweet items.

**Tier 1 bot method:** `XKeywordSearch(page *rod.Page, keyword string, maxResults int, filter string) ([]map[string]interface{}, error)`. Registered as `"keyword_search"`.

**Schema key:** `X_KEYWORD_SEARCH` with fields: `search_input`, `search_submit`, `tweet_item`, `tweet_link`.

---

#### PROFILE_FETCH (action type: `PROFILE_FETCH`)

**Purpose:** Export an X profile's followers or following list to the `people` table.

**Input parameters:**
- `profile_url` (string, required): X profile URL.
- `source_type` (string, required): `"followers"` | `"following"`.
- `max_results` (integer, optional, default 100): Maximum accounts to collect.
- `list_id` (string, optional): Save results to this social list ID.

**Output data shape:**
```json
{
  "extracted_count": 95,
  "people_ids": ["<uuid>", "..."]
}
```

**3-tier fallback applicability:** Full 3-tier applies to follower/following tab, user list items.

**Tier 1 bot method:** `XProfileFetch(page *rod.Page, sourceType string, maxResults int) ([]map[string]interface{}, error)`. Registered as `"profile_fetch"`.

**Schema key:** `X_PROFILE_FETCH` with fields: `followers_link`, `following_link`, `user_list`, `user_item_link`.

---

#### PROFILE_INTERACTION (action type: `PROFILE_INTERACTION`)

**Purpose:** Search X by keyword and like (and optionally reply to) matching tweets.

**Input parameters:**
- `keywords` (string, required): Search keyword or hashtag.
- `max_posts` (integer, optional, default 20): Maximum tweets to interact with.
- `reply_text` (string, optional): If non-empty, reply to each tweet with this text.
- `target_filter` (string, optional): Filter by minimum likes or verified accounts.

**Output data shape:**
```json
{
  "tweets_liked": 19,
  "tweets_replied": 4,
  "failed_tweets": 0
}
```

**3-tier fallback applicability:** Full 3-tier applies to like button, reply button, reply input, submit button.

**Tier 1 bot method:** `XProfileInteraction(page *rod.Page, tweetURL string, replyText string) error`. Registered as `"profile_interaction"`.

**Schema key:** `X_TWEET_INTERACTION` with fields: `like_button`, `reply_button`, `reply_textarea`, `reply_submit_button`.

---

#### PUBLISH_CONTENT (action type: `PUBLISH_CONTENT`)

**Purpose:** Publish a new tweet to the user's X account.

**Input parameters:**
- `tweet_text` (string, required): Text of the tweet. Max 280 characters.
- `media_path` (string, optional): Absolute local file path of an image or video to attach.

**Output data shape:**
```json
{
  "published": true,
  "tweet_url": "<url_if_extractable>"
}
```

**3-tier fallback applicability:** Full 3-tier applies to compose button, tweet text input, media attach button, tweet submit button.

**Tier 1 bot method:** `XPublishContent(page *rod.Page, tweetText, mediaPath string) error`. Registered as `"publish_content"`.

**Schema key:** `X_PUBLISH_CONTENT` with fields: `compose_button`, `tweet_textarea`, `media_attach_button`, `file_input`, `tweet_submit_button`.

---

### 6.4 Three-Tier Fallback Preservation for All New Actions

Every bot method listed in section 6.1–6.3 must be wrapped in the 3-tier fallback pattern in the action JSON. The action JSON structure must follow exactly the template in `THREE_TIER_FALLBACK.md`:

- Tier 1 step: `call_bot_method` with `onError: {"action": "skip"}`.
- `check_t1` condition: `{"variable": "t1Result", "operator": "not_exists"}`.
- Tier 2 steps: `find_element` with `xpath` + `alternatives`, `onError: {"action": "skip"}`.
- `check_t2` condition: `{"variable": "t2Element", "operator": "not_exists"}`.
- Tier 3 steps: `find_element` with `configKey`, `onError: {"action": "mark_failed"}`.

All new schema keys must be registered in `internal/config/schemas.go` before the action JSON is created.

---

## 7. API Surface

The Monoes system does not expose an external REST API. All API surface is through Wails bound methods, which are Go methods on the `App` struct called by the React frontend via the Wails bridge. A small separate HTTP server handles webhook triggers only.

### 7.1 Wails Bound Methods — Workflow Management

All methods below are on the `App` struct in `wails-app/app.go` (or a new `wails-app/workflow.go` file). They are exposed to the frontend via Wails binding.

---

**`GetWorkflows() []WorkflowSummary`**

Returns all workflows, sorted by `updated_at DESC`.

Response type:
```go
type WorkflowSummary struct {
    ID          string `json:"id"`
    Name        string `json:"name"`
    Description string `json:"description"`
    IsActive    bool   `json:"is_active"`
    NodeCount   int    `json:"node_count"`
    LastRunAt   string `json:"last_run_at"`    // ISO8601 or "" if never run
    LastRunStatus string `json:"last_run_status"` // "" | "COMPLETED" | "FAILED" | "CANCELLED"
    CreatedAt   string `json:"created_at"`
    UpdatedAt   string `json:"updated_at"`
}
```

Caller: UI workflow list page.

---

**`GetWorkflow(id string) (*WorkflowDetail, error)`**

Returns full workflow definition including nodes and connections. Used by the canvas editor to load a workflow.

Response type:
```go
type WorkflowDetail struct {
    ID          string             `json:"id"`
    Name        string             `json:"name"`
    Description string             `json:"description"`
    IsActive    bool               `json:"is_active"`
    TimeoutSecs int                `json:"timeout_seconds"`
    ErrorWorkflowID string         `json:"error_workflow_id"`
    Nodes       []WorkflowNodeDTO  `json:"nodes"`
    Connections []WorkflowConnDTO  `json:"connections"`
    CreatedAt   string             `json:"created_at"`
    UpdatedAt   string             `json:"updated_at"`
}

type WorkflowNodeDTO struct {
    ID        string                 `json:"id"`
    NodeType  string                 `json:"node_type"`
    Name      string                 `json:"name"`
    Config    map[string]interface{} `json:"config"`
    PositionX float64                `json:"position_x"`
    PositionY float64                `json:"position_y"`
    Disabled  bool                   `json:"disabled"`
}

type WorkflowConnDTO struct {
    ID            string `json:"id"`
    SourceNodeID  string `json:"source_node_id"`
    SourceHandle  string `json:"source_handle"`
    TargetNodeID  string `json:"target_node_id"`
    TargetHandle  string `json:"target_handle"`
}
```

Error returned if `id` does not exist: `"workflow not found: <id>"`.

Caller: Canvas editor on open.

---

**`CreateWorkflow(req CreateWorkflowRequest) (*WorkflowSummary, error)`**

Creates a new empty workflow with a default manual trigger node.

Request type:
```go
type CreateWorkflowRequest struct {
    Name        string `json:"name"`          // required, 1–200 chars
    Description string `json:"description"`   // optional
}
```

Behavior: Creates `workflows` row, creates one `workflow_nodes` row of type `trigger.manual` with name `"Manual Trigger"` at position (100, 100). Returns the new `WorkflowSummary`.

Error if `name` is empty: `"workflow name is required"`.

Caller: UI "New Workflow" button.

---

**`SaveWorkflow(req SaveWorkflowRequest) error`**

Saves the full workflow definition (overwrites nodes and connections). Used by the canvas editor's auto-save and manual save.

Request type:
```go
type SaveWorkflowRequest struct {
    ID          string             `json:"id"`
    Name        string             `json:"name"`
    Description string             `json:"description"`
    TimeoutSecs int                `json:"timeout_seconds"`
    ErrorWorkflowID string         `json:"error_workflow_id"`
    Nodes       []WorkflowNodeDTO  `json:"nodes"`
    Connections []WorkflowConnDTO  `json:"connections"`
}
```

Behavior:
1. Validate: name non-empty, no duplicate node IDs, no duplicate connection (source_node+source_handle+target_node+target_handle), no cycles (topological sort).
2. Within a single DB transaction: delete all existing `workflow_nodes` and `workflow_connections` for this workflow ID, then insert the new nodes and connections.
3. Update `workflows` record with new name, description, timeout_seconds, error_workflow_id, updated_at.
4. If the workflow `is_active = 1` and the save changed trigger node config, re-register triggers (deregister old, register new).

Error cases:
- Workflow not found: `"workflow not found: <id>"`.
- Cycle detected: `"workflow contains a cycle involving nodes: [<name1>, <name2>]"`.
- Invalid action type: `"node '<name>' references unknown action type: '<platform>/<type>'"`.
- Validation errors from section 2.6 (when `is_active = 1`).

Caller: Canvas editor on save.

---

**`DeleteWorkflow(id string) error`**

Deletes a workflow and all associated data (nodes, connections, executions, execution nodes). If the workflow is active, deactivates it first (removes triggers/webhooks).

Caller: UI delete button.

---

**`ActivateWorkflow(id string) error`**

Sets `is_active = 1` for the workflow. Performs full validation (section 2.6). Registers schedule cron jobs and webhook paths.

Error cases:
- Validation failure: returns the first validation error encountered.
- Webhook path conflict: `"webhook path '<path>' is already registered by workflow '<conflicting_id>'"`.
- Invalid cron: `"invalid cron expression '<expr>': <parse error>"`.

Caller: UI "Activate" toggle.

---

**`DeactivateWorkflow(id string) error`**

Sets `is_active = 0`. Removes cron jobs and webhook paths for this workflow.

Caller: UI "Deactivate" toggle.

---

**`ExecuteWorkflow(id string, inputData map[string]interface{}) (string, error)`**

Starts a manual execution. Returns the new execution ID. Non-blocking.

Error if workflow not found: `"workflow not found: <id>"`.
Error if queue full: `"workflow execution queue is full; try again later"`.

Caller: UI "Run" button.

---

**`CancelWorkflowExecution(executionID string) error`**

Cancels a running execution. Returns immediately; actual cancellation is asynchronous.

Error if execution not found or not RUNNING: `"execution '<id>' is not currently running"`.

Caller: UI "Cancel" button in execution history.

---

**`RetryWorkflowExecution(executionID string) (string, error)`**

Retries a failed or cancelled execution from the beginning. Returns the new execution ID.

Error if execution not found or not FAILED/CANCELLED: `"execution '<id>' cannot be retried (status: <status>)"`.

Caller: UI "Retry" button in execution history.

---

**`GetWorkflowExecutions(workflowID string, limit, offset int) []ExecutionSummary`**

Returns execution history for a workflow, paginated, sorted by `started_at DESC`.

Response type:
```go
type ExecutionSummary struct {
    ID          string `json:"id"`
    WorkflowID  string `json:"workflow_id"`
    TriggerType string `json:"trigger_type"`
    Status      string `json:"status"`
    StartedAt   string `json:"started_at"`
    FinishedAt  string `json:"finished_at"`
    DurationMs  int64  `json:"duration_ms"`
    ErrorMsg    string `json:"error_message"`
}
```

Caller: UI execution history sidebar/page.

---

**`GetWorkflowExecutionDetail(executionID string) (*ExecutionDetail, error)`**

Returns full execution detail including per-node run records.

Response type:
```go
type ExecutionDetail struct {
    ExecutionSummary
    InputData  map[string]interface{} `json:"input_data"`
    NodeRuns   []NodeRunSummary       `json:"node_runs"`
}

type NodeRunSummary struct {
    NodeID      string                   `json:"node_id"`
    NodeName    string                   `json:"node_name"`
    NodeType    string                   `json:"node_type"`
    RunIndex    int                      `json:"run_index"`
    Status      string                   `json:"status"`
    StartedAt   string                   `json:"started_at"`
    FinishedAt  string                   `json:"finished_at"`
    DurationMs  int64                    `json:"duration_ms"`
    InputItems  []map[string]interface{} `json:"input_items"`
    OutputItems []map[string]interface{} `json:"output_items"`
    Error       string                   `json:"error"`
    RetryCount  int                      `json:"retry_count"`
}
```

Caller: UI execution detail panel.

---

### 7.2 Webhook HTTP Server

This is a minimal Go `net/http` server, separate from the Wails internal server.

**Startup:** The server starts in a background goroutine when the application starts. It binds to `127.0.0.1:<webhook_port>` where `webhook_port` is read from the `settings` table (key `webhook_port`, default `9321`). If the port is already in use, startup logs a warning and the webhook feature is unavailable.

**Endpoints:**

`GET /` → `200 OK` with `{"status": "monoes webhook server", "version": "1.0"}`

`GET /webhook/:path_suffix`
- If path is registered: `200 OK` with `{"status": "active", "workflowId": "<id>"}`
- If path is not registered: `404 Not Found` with `{"error": "webhook not found"}`

`POST /webhook/:path_suffix`
- Request: any Content-Type; body read as raw bytes, then attempted JSON parse.
- If path not registered: `404 Not Found` with `{"error": "webhook not found"}`
- If auth required and token invalid: `401 Unauthorized` with `{"error": "invalid token"}`
- If queue full: `503 Service Unavailable` with `{"error": "execution queue full"}`
- On success: `202 Accepted` with `{"executionId": "<id>"}`

`OPTIONS /webhook/:path_suffix`
Returns `204 No Content` with CORS headers: `Access-Control-Allow-Origin: *`, `Access-Control-Allow-Methods: POST, GET, OPTIONS`, `Access-Control-Allow-Headers: Content-Type, X-Monoes-Token`.

**Request size limit:** Maximum 1MB request body. Requests exceeding this limit receive `413 Request Entity Too Large`.

---

## 8. UI Requirements

The UI is built in React/JSX within the Wails v2 `wails-app/frontend/` directory. The existing pages (Dashboard, Actions, People, Sessions, Logs) must remain unchanged. New pages are additive.

### 8.1 Navigation

Add a "Workflows" item to the existing `Sidebar.jsx` navigation. Icon: a node graph or flow icon (SVG). Clicking "Workflows" navigates to the Workflow List page.

### 8.2 Workflow List Page (`src/pages/Workflows.jsx`)

**Layout:** Full-width page with a header bar and a list/table of workflows.

**Header bar:**
- Page title: "Workflows"
- "New Workflow" button: opens a modal prompting for workflow name and optional description, then calls `CreateWorkflow`, then navigates to the canvas editor for the new workflow.

**Workflow table columns:**
- Name (clickable, navigates to canvas editor)
- Status: "Active" (green badge) or "Inactive" (grey badge), togglable in-place.
- Node count
- Last run: relative time (e.g., "2 hours ago") or "Never"
- Last run status: color-coded badge: COMPLETED (green), FAILED (red), CANCELLED (grey)
- Actions column: Edit (pencil icon, navigates to canvas), Run (play icon, calls `ExecuteWorkflow` with empty inputData), Delete (trash icon, confirms then calls `DeleteWorkflow`).

**Toggle active state:** Clicking the status badge calls `ActivateWorkflow` or `DeactivateWorkflow`. If activation fails, show the error message in a toast notification. Do not change the badge state on error.

**Polling:** The list auto-refreshes every 10 seconds while the page is visible, using Wails event subscription `workflow:execution:status_changed` emitted by the engine. Alternatively, a polling interval on `GetWorkflows` is acceptable for v1.

### 8.3 Workflow Canvas Editor (`src/pages/WorkflowCanvas.jsx`)

The canvas editor is the primary interface for building workflows. It is opened when clicking a workflow name in the list.

**Layout:**
- Top toolbar (horizontal bar): Back button, workflow name (editable inline), Active/Inactive toggle, Save button (manual save), Run button (executes workflow manually), Delete button.
- Left panel (200px): Node palette — a vertically scrolling list of node types grouped by category (Trigger, Action, Control, Transform). Each entry shows the node type icon and name. Nodes are draggable from the palette onto the canvas.
- Center: Canvas area (takes remaining width).
- Right panel (300px, collapsible): Node configuration panel — opens when a node is selected on the canvas.
- Bottom panel (200px, collapsible): Execution log panel — shows the latest execution's node run statuses.

**Canvas behavior:**
- Canvas uses a React-based node graph library. Recommended: `reactflow` (React Flow, MIT license). If not already in dependencies, it must be added.
- Nodes are rendered as rectangular cards with: node type icon (small), node name (bold), input handles on the left, output handles on the right.
- Handle colors: `main` handle = blue, `error` handle = red, `true` handle = green, `false` handle = orange, `done` handle = purple.
- Edges (connections) are rendered as bezier curves. Hovering an edge shows a delete button (×).
- Node states during execution are indicated by a colored border: running = blue pulsing, success = green, error = red, skipped = grey.
- Disabled nodes are rendered with reduced opacity (0.4).

**Node palette drag-and-drop:**
- User drags a node type from the palette onto the canvas. On drop, a new `WorkflowNodeDTO` is created with a generated UUID, the dropped position snapped to a 20px grid, default config for the node type, and name defaulting to the node type's display name.
- The new node is appended to the local state and the canvas re-renders. The workflow is not saved until the user clicks Save or auto-save triggers.

**Connection creation:**
- User drags from an output handle to an input handle. The canvas library provides drag-to-connect behavior natively via React Flow.
- On connection: validate that the source handle exists for the source node type, the target handle exists for the target node type, no duplicate connection, no self-connection. If invalid, flash the handles red and show a tooltip: `"Cannot connect: <reason>"`.
- Valid connections are added to local state.

**Node selection and configuration panel:**
- Clicking a node selects it (single selection only in v1) and opens the configuration panel on the right.
- The configuration panel renders fields based on the node type. Each node type has a corresponding configuration form component.
- Field rendering by config field type:
  - String: text input.
  - Integer: number input with min/max validation.
  - Boolean: checkbox or toggle.
  - Enum (one of N values): select dropdown.
  - JSON object (e.g., `params`): a JSON editor (a `<textarea>` that validates JSON on blur in v1; a proper code editor in v2).
  - Expression field: text input with `{{` prefix hint and syntax highlighting in the future.
- Changes to config fields update the node's config in local state immediately. They are not persisted until Save.
- The node name is editable in the configuration panel header.
- A "Disable node" checkbox is shown in the panel footer.

**Auto-save:** 3 seconds after the last canvas change (node move, connection create/delete, config change), call `SaveWorkflow` automatically. While saving, show a spinner in the toolbar. On save error, show error toast and stop auto-save until user manually saves.

**Manual save:** Ctrl+S or the Save button in the toolbar. Calls `SaveWorkflow`. On success, show "Saved" badge for 2 seconds.

**Run from canvas:** The Run button in the toolbar opens a small modal with a textarea for optional JSON input data, then calls `ExecuteWorkflow`. Displays a toast: "Execution started: `<executionId>`".

**Undo/Redo:** Maintain a local history of canvas state changes. Ctrl+Z undoes the last change (node add/delete, connection add/delete, node position change). Ctrl+Y / Ctrl+Shift+Z redoes. History depth: 50 states. History is in-memory only; lost on page navigation.

**Zoom/Pan:** Mouse wheel to zoom (10%–400%). Click and drag on empty canvas to pan. "Fit to screen" button in top-right canvas corner.

**Node actions (right-click context menu):**
- Copy node (duplicates node with new ID, offset by 20px).
- Delete node (removes node and all its connections).
- Disable/Enable node.
- View last execution output (shows a modal with the `outputItems` from the most recent execution of this node).

### 8.4 Execution History Page/Panel

**Location:** Accessible as a bottom panel on the canvas editor (slide-up, toggled by a "History" button in the toolbar) and as a dedicated sub-page for each workflow.

**Execution list:**
- Columns: Execution ID (truncated, clickable), Trigger type, Status (colored badge), Started at, Duration.
- Paginated: 20 per page.
- "Cancel" button on RUNNING executions.
- "Retry" button on FAILED/CANCELLED executions.

**Execution detail (on click):**
- Shows the workflow canvas with nodes color-coded by their run status in this execution.
- Shows the bottom panel with a timeline of node executions: node name, status, duration, retry count.
- Clicking a node in the timeline opens a panel showing: input items (JSON tree), output items (JSON tree), error message (if any).

### 8.5 Node Configuration Panels (per node type)

Each node type requires a dedicated React component for its configuration. Required components:

**TriggerManualConfigPanel:** Single optional JSON textarea for `sample_data`.

**TriggerScheduleConfigPanel:** Text input for `cron_expression` with a human-readable cron description (e.g., "Every weekday at 9:00 AM"). Timezone selector (searchable select from IANA timezone list). A "Next 5 runs" preview list.

**TriggerWebhookConfigPanel:** Display of the full webhook URL (read-only: `http://localhost:<port>/webhook/<path_suffix>`). Editable `path_suffix` text input. Toggle for `require_auth`. If enabled, show `webhook_secret` input field.

**ActionNodeConfigPanel:** Platform selector (Instagram/LinkedIn/TikTok/X), action type selector (filtered by platform, showing only types with existing JSON files), session username selector (dropdown of active sessions for the selected platform), params JSON editor, retry policy fields, on_error selector.

**ControlIfConfigPanel:** Single text input for `condition` expression with `{{` hint.

**ControlSwitchConfigPanel:** Expression input, plus a dynamic list of cases (add/remove case rows, each with value and handle name inputs).

**ControlMergeConfigPanel:** Dropdown for `mode` (append/first).

**ControlWaitConfigPanel:** Number input for `seconds` (1–3600) with a human-readable display (e.g., "30 seconds").

**ControlLoopConfigPanel:** Text input for `input_field`, text input for `item_var`.

**ControlStopErrorConfigPanel:** Text input for `message` expression.

**TransformSetConfigPanel:** Dynamic list of assignments. Each row: `key` text input, `value` text input. Add/remove rows.

**TransformCodeConfigPanel:** A `<textarea>` for the Go template script. Display a syntax hint.

---

## 9. Data Model Changes

All changes are additive. No existing tables are altered.

### 9.1 New Tables

**Migration file:** `006_workflow_system.sql`

---

#### `workflows`

```sql
CREATE TABLE IF NOT EXISTS workflows (
    id                TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    is_active         INTEGER NOT NULL DEFAULT 0,
    version           INTEGER NOT NULL DEFAULT 1,
    timeout_seconds   INTEGER NOT NULL DEFAULT 3600,
    error_workflow_id TEXT REFERENCES workflows(id) ON DELETE SET NULL,
    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_workflows_is_active ON workflows(is_active);
CREATE INDEX IF NOT EXISTS idx_workflows_updated ON workflows(updated_at);
```

---

#### `workflow_nodes`

```sql
CREATE TABLE IF NOT EXISTS workflow_nodes (
    id          TEXT PRIMARY KEY,
    workflow_id TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    node_type   TEXT NOT NULL,
    name        TEXT NOT NULL,
    config      TEXT NOT NULL DEFAULT '{}',
    position_x  REAL NOT NULL DEFAULT 0,
    position_y  REAL NOT NULL DEFAULT 0,
    disabled    INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_workflow_nodes_workflow ON workflow_nodes(workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_nodes_type ON workflow_nodes(node_type);
```

---

#### `workflow_connections`

```sql
CREATE TABLE IF NOT EXISTS workflow_connections (
    id              TEXT PRIMARY KEY,
    workflow_id     TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    source_node_id  TEXT NOT NULL REFERENCES workflow_nodes(id) ON DELETE CASCADE,
    source_handle   TEXT NOT NULL DEFAULT 'main',
    target_node_id  TEXT NOT NULL REFERENCES workflow_nodes(id) ON DELETE CASCADE,
    target_handle   TEXT NOT NULL DEFAULT 'main',
    position        INTEGER NOT NULL DEFAULT 0,
    UNIQUE(source_node_id, source_handle, target_node_id, target_handle)
);

CREATE INDEX IF NOT EXISTS idx_workflow_connections_workflow ON workflow_connections(workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_connections_source ON workflow_connections(source_node_id);
CREATE INDEX IF NOT EXISTS idx_workflow_connections_target ON workflow_connections(target_node_id);
```

---

#### `workflow_triggers`

This table caches the resolved trigger registrations for active workflows to enable fast re-registration on startup.

```sql
CREATE TABLE IF NOT EXISTS workflow_triggers (
    id           TEXT PRIMARY KEY,
    workflow_id  TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    node_id      TEXT NOT NULL REFERENCES workflow_nodes(id) ON DELETE CASCADE,
    trigger_type TEXT NOT NULL,         -- "manual" | "schedule" | "webhook"
    config       TEXT NOT NULL DEFAULT '{}',   -- snapshot of trigger node config at activation time
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(node_id)
);

CREATE INDEX IF NOT EXISTS idx_workflow_triggers_workflow ON workflow_triggers(workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_triggers_type ON workflow_triggers(trigger_type);
```

Note: On startup, active workflows re-register triggers from this table. If a workflow_trigger row has a `config` that no longer matches the current node config (i.e., the workflow was saved while inactive), the discrepancy is detected during activation; the trigger table is always updated atomically when `ActivateWorkflow` is called.

---

#### `workflow_executions`

```sql
CREATE TABLE IF NOT EXISTS workflow_executions (
    id              TEXT PRIMARY KEY,
    workflow_id     TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    trigger_type    TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'RUNNING',
    input_data      TEXT NOT NULL DEFAULT '{}',    -- JSON: trigger input data
    error_message   TEXT NOT NULL DEFAULT '',
    started_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at     TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_workflow_executions_workflow ON workflow_executions(workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_executions_status ON workflow_executions(status);
CREATE INDEX IF NOT EXISTS idx_workflow_executions_started ON workflow_executions(started_at);
```

---

#### `workflow_execution_nodes`

```sql
CREATE TABLE IF NOT EXISTS workflow_execution_nodes (
    id           TEXT PRIMARY KEY,
    execution_id TEXT NOT NULL REFERENCES workflow_executions(id) ON DELETE CASCADE,
    node_id      TEXT NOT NULL,              -- NOT a FK; node may not exist after workflow edit
    node_name    TEXT NOT NULL,
    node_type    TEXT NOT NULL,
    run_index    INTEGER NOT NULL DEFAULT 0,
    status       TEXT NOT NULL,              -- "SUCCESS" | "FAILED" | "SKIPPED"
    input_items  TEXT NOT NULL DEFAULT '[]', -- JSON array
    output_items TEXT NOT NULL DEFAULT '[]', -- JSON array
    error_msg    TEXT NOT NULL DEFAULT '',
    retry_count  INTEGER NOT NULL DEFAULT 0,
    started_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at  TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_exec_nodes_execution ON workflow_execution_nodes(execution_id);
CREATE INDEX IF NOT EXISTS idx_exec_nodes_node ON workflow_execution_nodes(node_id);
CREATE INDEX IF NOT EXISTS idx_exec_nodes_status ON workflow_execution_nodes(status);
```

### 9.2 Schema Notes

- All `id` columns for workflow system tables use UUID v4 strings (same pattern as existing tables).
- `input_items` and `output_items` in `workflow_execution_nodes` store JSON arrays. Maximum stored size: 1MB per row. If execution data exceeds 1MB, truncate to first 100 items and append `{"_truncated": true}` as the last item.
- `workflow_triggers` is the authoritative source for active trigger registrations on startup. It is updated atomically with the activation/deactivation of the workflow (inside a DB transaction).
- No new indexes on existing tables are required.
- All new tables use `ON DELETE CASCADE` or `ON DELETE SET NULL` appropriately to maintain referential integrity when workflows or nodes are deleted.

---

## 10. Non-Functional Requirements

### 10.1 Performance

- **Workflow save latency:** `SaveWorkflow` must complete within 200ms for workflows with up to 50 nodes and 100 connections (measured on macOS M-series hardware with SQLite WAL mode).
- **Execution start latency:** From trigger event to first node beginning execution: less than 100ms (excluding browser startup time).
- **Node graph rendering:** The canvas editor must render a workflow with up to 50 nodes without frame drops below 30fps on modern hardware.
- **Max nodes per workflow:** Hard limit of 200 nodes. Saving a workflow with more than 200 nodes returns error: `"workflow exceeds maximum node limit of 200"`.
- **Max connections per workflow:** Hard limit of 500 connections.
- **Execution history retention:** Keep the last 500 executions per workflow. When a new execution is created and the count exceeds 500, delete the oldest non-RUNNING executions in a background goroutine (not in the critical path).

### 10.2 3-Tier Fallback Preservation

The 3-tier fallback system MUST be preserved for ALL browser automation nodes. Specifically:

- The `WorkflowEngine` does not bypass or modify the action JSON execution path. When an action node fires, it calls the existing `ActionExecutor.Execute()` unchanged.
- The action JSON file for each action type remains the authoritative source of the 3-tier step sequence.
- No workflow-level constructs (retry policy, on_error) interfere with the internal 3-tier cascade within a single action execution. The `retry_policy` on an action node retries the entire `ActionExecutor.Execute()` call (the full 3-tier sequence), not individual tiers.
- New action types added in section 6 must have complete action JSON files with all three tiers before the node type is enabled for use in workflows.

### 10.3 Backward Compatibility

- All existing `Action` CRUD, scheduling, and execution via `monoes run <id>` continues to function without modification.
- The existing `Scheduler` struct is used as-is for both action scheduling (existing) and workflow schedule trigger scheduling (new). The scheduler's `ScheduleAction` method is reused; a new `ScheduleWorkflow` method is added to its interface that calls `WorkflowEngine.TriggerWorkflow` instead of `ActionExecutor.RunSingle`.
- The existing DB tables (`actions`, `action_targets`, `people`, `sessions`, etc.) are not altered.
- Existing Wails bound methods (`GetActions`, `CreateAction`, `ExecuteAction`, etc.) are unchanged.
- The migration file `006_workflow_system.sql` uses `CREATE TABLE IF NOT EXISTS` for all new tables, making it safe to apply to databases that already have the tables (idempotent).

### 10.4 Data Integrity

- Workflow `SaveWorkflow` must execute node/connection inserts inside a single SQLite transaction. Partial saves (inserting some nodes but not others due to an error) must roll back completely.
- Execution status transitions must be performed with SQLite's `UPDATE ... WHERE status = 'RUNNING'` pattern to prevent races: `UPDATE workflow_executions SET status = 'COMPLETED', finished_at = ? WHERE id = ? AND status = 'RUNNING'`. If 0 rows are updated (status already changed by another path), log a warning and skip.
- The `cancelFunc` map (in-memory) must be protected by a `sync.RWMutex`.

### 10.5 Logging

- All workflow engine operations use the existing `zerolog` logger with structured fields: `workflow_id`, `execution_id`, `node_id`, `node_type`.
- At info level: execution start, execution complete, trigger fired, workflow activated/deactivated.
- At debug level: each node start/complete, expression evaluations.
- At warn level: node failures with retry (before retries exhausted), expression evaluation errors (field not found).
- At error level: execution failures, DB errors, scheduler registration failures.

### 10.6 Dependencies

No new Go module dependencies should be added unless absolutely necessary. The workflow engine must be implemented using the existing dependency set:
- `robfig/cron v3` — already present, used for schedule triggers.
- `google/uuid` — already present, used for ID generation.
- `rs/zerolog` — already present, used for logging.
- `modernc.org/sqlite` — already present, used for DB.
- `text/template` (stdlib) — used for expression engine.
- `net/http` (stdlib) — used for webhook HTTP server.
- `sync` (stdlib) — used for concurrency primitives.

The React frontend may add `reactflow` (React Flow) as a new npm dependency for the canvas editor. This is acceptable.

### 10.7 Testing

- Each new action type (section 6) must have an integration test in `internal/action/<platform>_integration_test.go` following the existing pattern with build tag `//go:build integration`.
- The workflow engine (DAG execution, expression evaluation, trigger registration) must have unit tests with mocked storage and scheduler interfaces.
- Cycle detection in DAG must be unit tested with: no cycle (valid), direct cycle (A→B→A), indirect cycle (A→B→C→A), diamond (A→B, A→C, B→D, C→D — valid, not a cycle).
- Expression engine must have unit tests covering: simple field access, nested field access, node output access, missing field (returns ""), boolean comparison.

---

*End of requirements document.*
