# Workflow JSON Storage & Smart Node Configuration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace SQLite workflow storage with self-contained JSON files and add schema-driven Inspector UI with resource pickers.

**Architecture:** Each workflow is a single `~/.monoes/workflows/<id>.json` file carrying nodes, connections, and per-node config schemas inline. Default schemas are embedded in the Go binary via `//go:embed`. The Inspector renders fields entirely from the node's embedded schema — no hardcoded `NODE_CONFIG_FIELDS`. Two new Wails-bound functions `ListResources` and `CreateResource` let the UI browse/create external assets server-side.

**Tech Stack:** Go 1.21+, `//go:embed`, `encoding/json`, `os.ReadDir`, React JSX (no TypeScript), Wails v2

**Key files to understand before starting:**
- `internal/workflow/models.go` — `WorkflowNode`, `Workflow` structs
- `internal/workflow/storage.go` — `WorkflowStore` interface + `SQLiteWorkflowStore`
- `wails-app/app.go` — `SaveWorkflow`, `LoadWorkflow`, `ListWorkflows`, `GetWorkflowNodeTypes`
- `wails-app/frontend/src/pages/NodeRunner.jsx` — Inspector component, `NODE_CONFIG_FIELDS`
- Design doc: `docs/plans/2026-03-13-workflow-json-node-config-design.md`

---

### Task 1: Write default schema JSON files (triggers + core nodes)

**Files:**
- Create: `internal/workflow/schemas/trigger.manual.json`
- Create: `internal/workflow/schemas/trigger.schedule.json`
- Create: `internal/workflow/schemas/trigger.webhook.json`
- Create: `internal/workflow/schemas/core.if.json`
- Create: `internal/workflow/schemas/core.switch.json`
- Create: `internal/workflow/schemas/core.code.json`
- Create: `internal/workflow/schemas/core.filter.json`
- Create: `internal/workflow/schemas/core.set.json`
- Create: `internal/workflow/schemas/core.limit.json`
- Create: `internal/workflow/schemas/core.sort.json`
- Create: `internal/workflow/schemas/core.aggregate.json`
- Create: `internal/workflow/schemas/core.merge.json`
- Create: `internal/workflow/schemas/core.wait.json`
- Create: `internal/workflow/schemas/core.stop_error.json`
- Create: `internal/workflow/schemas/core.split_in_batches.json`
- Create: `internal/workflow/schemas/core.remove_duplicates.json`
- Create: `internal/workflow/schemas/core.compare_datasets.json`

**Step 1: Create the schemas directory**
```bash
mkdir -p internal/workflow/schemas
```

**Step 2: Write trigger schemas**

`internal/workflow/schemas/trigger.manual.json`:
```json
{ "credential_platform": null, "fields": [] }
```

`internal/workflow/schemas/trigger.schedule.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "cron", "label": "Cron Expression", "type": "text", "required": true, "placeholder": "e.g. 0 9 * * 1-5", "help": "Standard 5-field cron expression. e.g. '0 9 * * 1-5' = weekdays at 9am." },
    { "key": "timezone", "label": "Timezone", "type": "text", "required": false, "default": "UTC", "help": "IANA timezone name, e.g. America/New_York" }
  ]
}
```

`internal/workflow/schemas/trigger.webhook.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "path", "label": "Webhook Path", "type": "text", "required": false, "placeholder": "/webhook/my-hook", "help": "URL path suffix. Leave blank to auto-generate." },
    { "key": "method", "label": "HTTP Method", "type": "select", "required": false, "options": ["GET", "POST", "PUT", "PATCH", "DELETE"], "default": "POST" },
    { "key": "auth_header", "label": "Auth Header Name", "type": "text", "required": false, "placeholder": "X-Webhook-Secret", "help": "If set, requests must include this header with the auth token value." },
    { "key": "auth_token", "label": "Auth Token", "type": "password", "required": false, "help": "Secret value expected in the auth header." }
  ]
}
```

**Step 3: Write core schemas**

`internal/workflow/schemas/core.if.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "condition", "label": "Condition Expression", "type": "text", "required": true, "placeholder": "e.g. {{ $json.status == 'active' }}", "help": "Expression evaluated against the current item. Routes to 'true' or 'false' output." }
  ]
}
```

`internal/workflow/schemas/core.switch.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "field", "label": "Field to Switch On", "type": "text", "required": true, "placeholder": "{{ $json.status }}", "help": "Value expression to match against cases." },
    { "key": "cases", "label": "Cases (comma-separated)", "type": "text", "required": true, "placeholder": "active,inactive,pending", "help": "Comma-separated values. Each becomes an output handle." },
    { "key": "fallthrough", "label": "Fallthrough to 'default'", "type": "boolean", "required": false, "default": true }
  ]
}
```

`internal/workflow/schemas/core.code.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "language", "label": "Language", "type": "select", "required": true, "options": ["javascript", "python"], "default": "javascript" },
    { "key": "code", "label": "Code", "type": "code", "language": "javascript", "required": true, "help": "Return an array of objects. Each object becomes an output item. Access input via $input.all()." }
  ]
}
```

`internal/workflow/schemas/core.filter.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "condition", "label": "Keep items where", "type": "text", "required": true, "placeholder": "{{ $json.age > 18 }}", "help": "Items where this evaluates to true are passed through." }
  ]
}
```

`internal/workflow/schemas/core.set.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "assignments", "label": "Field Assignments (JSON)", "type": "textarea", "required": true, "rows": 5, "placeholder": "{\n  \"fullName\": \"{{ $json.first }} {{ $json.last }}\"\n}", "help": "JSON object. Keys are field names to set. Values are expressions." }
  ]
}
```

`internal/workflow/schemas/core.limit.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "max_items", "label": "Max Items", "type": "number", "required": true, "default": 10, "min": 1, "help": "Only the first N items pass through." }
  ]
}
```

`internal/workflow/schemas/core.sort.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "field", "label": "Sort By Field", "type": "text", "required": true, "placeholder": "created_at" },
    { "key": "direction", "label": "Direction", "type": "select", "required": true, "options": ["asc", "desc"], "default": "asc" }
  ]
}
```

`internal/workflow/schemas/core.aggregate.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["count", "sum", "avg", "min", "max", "collect"], "default": "count" },
    { "key": "field", "label": "Field Name", "type": "text", "required": false, "placeholder": "amount", "help": "Field to aggregate. Not needed for 'count' or 'collect'.", "depends_on": { "field": "operation", "values": ["sum", "avg", "min", "max"] } },
    { "key": "group_by", "label": "Group By Field", "type": "text", "required": false, "placeholder": "category", "help": "Optional: group results by this field." }
  ]
}
```

`internal/workflow/schemas/core.merge.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "mode", "label": "Merge Mode", "type": "select", "required": true, "options": ["append", "merge_by_key", "zip"], "default": "append" },
    { "key": "key_field", "label": "Key Field", "type": "text", "required": false, "placeholder": "id", "depends_on": { "field": "mode", "values": ["merge_by_key"] } }
  ]
}
```

`internal/workflow/schemas/core.wait.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "duration", "label": "Wait Duration (seconds)", "type": "number", "required": true, "default": 5, "min": 1, "max": 3600 }
  ]
}
```

`internal/workflow/schemas/core.stop_error.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "message", "label": "Error Message", "type": "text", "required": true, "placeholder": "Workflow stopped: condition not met" }
  ]
}
```

`internal/workflow/schemas/core.split_in_batches.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "batch_size", "label": "Batch Size", "type": "number", "required": true, "default": 10, "min": 1 },
    { "key": "reset", "label": "Reset on new execution", "type": "boolean", "default": true }
  ]
}
```

`internal/workflow/schemas/core.remove_duplicates.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "field", "label": "Deduplicate By Field", "type": "text", "required": true, "placeholder": "id", "help": "Items with the same value in this field are deduplicated." }
  ]
}
```

`internal/workflow/schemas/core.compare_datasets.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "key_field", "label": "Key Field", "type": "text", "required": true, "placeholder": "id" },
    { "key": "output", "label": "Output", "type": "select", "required": true, "options": ["new_items", "removed_items", "changed_items", "all_differences"], "default": "new_items" }
  ]
}
```

**Step 4: Verify files exist**
```bash
ls internal/workflow/schemas/ | wc -l
# Expected: 17 files
```

**Step 5: Commit**
```bash
git add internal/workflow/schemas/
git commit -m "feat: add default schemas for trigger and core node types"
```

---

### Task 2: Write default schema JSON files (http, system, data, db, comm, service, browser)

**Files:** Create remaining 33 schema files in `internal/workflow/schemas/`

**Step 1: Write HTTP schemas**

`internal/workflow/schemas/http.request.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "url", "label": "URL", "type": "text", "required": true, "placeholder": "https://api.example.com/data" },
    { "key": "method", "label": "Method", "type": "select", "required": true, "options": ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD"], "default": "GET" },
    { "key": "headers", "label": "Headers (JSON)", "type": "textarea", "required": false, "rows": 3, "placeholder": "{ \"Authorization\": \"Bearer {{$env.API_KEY}}\" }" },
    { "key": "body", "label": "Request Body", "type": "textarea", "required": false, "rows": 5, "depends_on": { "field": "method", "values": ["POST", "PUT", "PATCH"] } },
    { "key": "timeout", "label": "Timeout (seconds)", "type": "number", "required": false, "default": 30 }
  ]
}
```

`internal/workflow/schemas/http.ftp.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "host", "label": "FTP Host", "type": "text", "required": true },
    { "key": "port", "label": "Port", "type": "number", "required": false, "default": 21 },
    { "key": "username", "label": "Username", "type": "text", "required": true },
    { "key": "password", "label": "Password", "type": "password", "required": true },
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["list", "download", "upload", "delete"], "default": "list" },
    { "key": "path", "label": "Remote Path", "type": "text", "required": true, "placeholder": "/remote/dir/file.csv" }
  ]
}
```

`internal/workflow/schemas/http.ssh.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "host", "label": "SSH Host", "type": "text", "required": true },
    { "key": "port", "label": "Port", "type": "number", "required": false, "default": 22 },
    { "key": "username", "label": "Username", "type": "text", "required": true },
    { "key": "password", "label": "Password", "type": "password", "required": false },
    { "key": "private_key", "label": "Private Key (PEM)", "type": "textarea", "required": false, "rows": 6 },
    { "key": "command", "label": "Command", "type": "text", "required": true, "placeholder": "ls -la /var/log" }
  ]
}
```

**Step 2: Write system schemas**

`internal/workflow/schemas/system.execute_command.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "command", "label": "Shell Command", "type": "text", "required": true, "placeholder": "echo hello world" },
    { "key": "working_dir", "label": "Working Directory", "type": "text", "required": false, "placeholder": "/tmp" },
    { "key": "timeout", "label": "Timeout (seconds)", "type": "number", "required": false, "default": 60 }
  ]
}
```

`internal/workflow/schemas/system.rss_read.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "url", "label": "RSS/Atom Feed URL", "type": "text", "required": true, "placeholder": "https://feeds.example.com/rss.xml" },
    { "key": "limit", "label": "Max Items", "type": "number", "required": false, "default": 20 }
  ]
}
```

**Step 3: Write data schemas**

`internal/workflow/schemas/data.datetime.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["format", "parse", "add", "subtract", "diff", "now"], "default": "format" },
    { "key": "field", "label": "Date Field", "type": "text", "required": false, "placeholder": "created_at", "depends_on": { "field": "operation", "values": ["format", "parse", "add", "subtract"] } },
    { "key": "format", "label": "Output Format", "type": "text", "required": false, "placeholder": "2006-01-02T15:04:05Z07:00", "help": "Go time layout string." },
    { "key": "duration", "label": "Duration", "type": "text", "required": false, "placeholder": "24h", "depends_on": { "field": "operation", "values": ["add", "subtract"] } }
  ]
}
```

`internal/workflow/schemas/data.crypto.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["hash_md5", "hash_sha256", "hash_sha512", "base64_encode", "base64_decode", "hmac_sha256"], "default": "hash_sha256" },
    { "key": "field", "label": "Input Field", "type": "text", "required": true, "placeholder": "password" },
    { "key": "secret", "label": "HMAC Secret", "type": "password", "required": false, "depends_on": { "field": "operation", "values": ["hmac_sha256"] } }
  ]
}
```

`internal/workflow/schemas/data.html.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["extract_text", "extract_links", "select", "sanitize"], "default": "extract_text" },
    { "key": "field", "label": "HTML Field", "type": "text", "required": true, "placeholder": "html_content" },
    { "key": "selector", "label": "CSS Selector", "type": "text", "required": false, "placeholder": "div.article p", "depends_on": { "field": "operation", "values": ["select"] } }
  ]
}
```

`internal/workflow/schemas/data.xml.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["parse", "build", "xpath"], "default": "parse" },
    { "key": "field", "label": "XML Field", "type": "text", "required": true, "placeholder": "xml_content" },
    { "key": "xpath", "label": "XPath Expression", "type": "text", "required": false, "depends_on": { "field": "operation", "values": ["xpath"] } }
  ]
}
```

`internal/workflow/schemas/data.markdown.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["to_html", "to_text", "from_html"], "default": "to_html" },
    { "key": "field", "label": "Input Field", "type": "text", "required": true, "placeholder": "content" }
  ]
}
```

`internal/workflow/schemas/data.spreadsheet.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["read_csv", "write_csv", "read_xlsx", "write_xlsx"], "default": "read_csv" },
    { "key": "file_path", "label": "File Path", "type": "text", "required": true, "placeholder": "/tmp/data.csv" },
    { "key": "sheet_name", "label": "Sheet Name", "type": "text", "required": false, "default": "Sheet1", "depends_on": { "field": "operation", "values": ["read_xlsx", "write_xlsx"] } }
  ]
}
```

`internal/workflow/schemas/data.compression.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["zip", "unzip", "gzip", "gunzip"], "default": "zip" },
    { "key": "input_field", "label": "Input Field", "type": "text", "required": true, "placeholder": "file_data" },
    { "key": "output_field", "label": "Output Field Name", "type": "text", "required": false, "default": "compressed" }
  ]
}
```

`internal/workflow/schemas/data.write_binary_file.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "path", "label": "File Path", "type": "text", "required": true, "placeholder": "/tmp/output.pdf" },
    { "key": "data_field", "label": "Binary Data Field", "type": "text", "required": true, "placeholder": "pdf_bytes" }
  ]
}
```

**Step 4: Write database schemas**

`internal/workflow/schemas/db.postgres.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "connection_string", "label": "Connection String", "type": "password", "required": true, "placeholder": "postgres://user:pass@host:5432/db" },
    { "key": "query", "label": "SQL Query", "type": "code", "language": "sql", "required": true, "placeholder": "SELECT * FROM users WHERE active = true" },
    { "key": "params", "label": "Query Parameters (JSON array)", "type": "textarea", "required": false, "rows": 3, "placeholder": "[true, 100]", "help": "Positional parameters for $1, $2, etc." }
  ]
}
```

`internal/workflow/schemas/db.mysql.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "connection_string", "label": "Connection String", "type": "password", "required": true, "placeholder": "user:pass@tcp(host:3306)/db" },
    { "key": "query", "label": "SQL Query", "type": "code", "language": "sql", "required": true, "placeholder": "SELECT * FROM users WHERE active = ?" },
    { "key": "params", "label": "Query Parameters (JSON array)", "type": "textarea", "required": false, "rows": 3, "placeholder": "[1]" }
  ]
}
```

`internal/workflow/schemas/db.mongodb.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "uri", "label": "MongoDB URI", "type": "password", "required": true, "placeholder": "mongodb://user:pass@host:27017/db" },
    { "key": "collection", "label": "Collection", "type": "text", "required": true },
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["find", "findOne", "insertOne", "insertMany", "updateOne", "updateMany", "deleteOne", "deleteMany"], "default": "find" },
    { "key": "filter", "label": "Filter (JSON)", "type": "textarea", "required": false, "rows": 3, "placeholder": "{ \"status\": \"active\" }" },
    { "key": "update", "label": "Update (JSON)", "type": "textarea", "required": false, "rows": 3, "depends_on": { "field": "operation", "values": ["updateOne", "updateMany"] } }
  ]
}
```

`internal/workflow/schemas/db.redis.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "url", "label": "Redis URL", "type": "password", "required": true, "placeholder": "redis://localhost:6379" },
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["get", "set", "del", "exists", "expire", "hget", "hset", "lpush", "rpop"], "default": "get" },
    { "key": "key", "label": "Key", "type": "text", "required": true },
    { "key": "value", "label": "Value", "type": "text", "required": false, "depends_on": { "field": "operation", "values": ["set", "hset", "lpush"] } },
    { "key": "ttl", "label": "TTL (seconds)", "type": "number", "required": false, "depends_on": { "field": "operation", "values": ["set", "expire"] } }
  ]
}
```

**Step 5: Write communication schemas**

`internal/workflow/schemas/comm.slack.json`:
```json
{
  "credential_platform": "slack",
  "fields": [
    { "key": "credential_id", "label": "Slack Connection", "type": "text", "required": true, "help": "Select your Slack workspace connection." },
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["send_message", "update_message", "delete_message", "get_channel", "list_channels", "get_user"], "default": "send_message" },
    { "key": "channel_id", "label": "Channel", "type": "resource_picker", "required": true, "resource": { "type": "channels", "create_label": null }, "help": "Select a Slack channel." },
    { "key": "text", "label": "Message Text", "type": "textarea", "required": true, "rows": 3, "depends_on": { "field": "operation", "values": ["send_message", "update_message"] } },
    { "key": "message_ts", "label": "Message Timestamp", "type": "text", "required": false, "depends_on": { "field": "operation", "values": ["update_message", "delete_message"] } }
  ]
}
```

`internal/workflow/schemas/comm.discord.json`:
```json
{
  "credential_platform": "discord",
  "fields": [
    { "key": "credential_id", "label": "Discord Connection", "type": "text", "required": true },
    { "key": "channel_id", "label": "Channel", "type": "resource_picker", "required": true, "resource": { "type": "channels", "create_label": null } },
    { "key": "message", "label": "Message", "type": "textarea", "required": true, "rows": 3 }
  ]
}
```

`internal/workflow/schemas/comm.telegram.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "bot_token", "label": "Bot Token", "type": "password", "required": true, "help": "Get from @BotFather on Telegram." },
    { "key": "chat_id", "label": "Chat ID", "type": "text", "required": true, "help": "Numeric chat ID or @channel_username." },
    { "key": "message", "label": "Message", "type": "textarea", "required": true, "rows": 3 },
    { "key": "parse_mode", "label": "Parse Mode", "type": "select", "required": false, "options": ["plain", "Markdown", "HTML"], "default": "plain" }
  ]
}
```

`internal/workflow/schemas/comm.email_send.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "smtp_host", "label": "SMTP Host", "type": "text", "required": true, "placeholder": "smtp.gmail.com" },
    { "key": "smtp_port", "label": "SMTP Port", "type": "number", "required": true, "default": 587 },
    { "key": "username", "label": "Username", "type": "text", "required": true },
    { "key": "password", "label": "Password / App Password", "type": "password", "required": true },
    { "key": "from", "label": "From Address", "type": "text", "required": true, "placeholder": "you@example.com" },
    { "key": "to", "label": "To Address(es)", "type": "text", "required": true, "placeholder": "user@example.com, other@example.com" },
    { "key": "subject", "label": "Subject", "type": "text", "required": true },
    { "key": "body", "label": "Email Body", "type": "textarea", "required": true, "rows": 6 },
    { "key": "html", "label": "Body is HTML", "type": "boolean", "default": false }
  ]
}
```

`internal/workflow/schemas/comm.email_read.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "imap_host", "label": "IMAP Host", "type": "text", "required": true, "placeholder": "imap.gmail.com" },
    { "key": "imap_port", "label": "IMAP Port", "type": "number", "required": true, "default": 993 },
    { "key": "username", "label": "Username", "type": "text", "required": true },
    { "key": "password", "label": "Password / App Password", "type": "password", "required": true },
    { "key": "mailbox", "label": "Mailbox", "type": "text", "required": false, "default": "INBOX" },
    { "key": "limit", "label": "Max Emails", "type": "number", "required": false, "default": 20 },
    { "key": "unread_only", "label": "Unread Only", "type": "boolean", "default": false }
  ]
}
```

`internal/workflow/schemas/comm.twilio.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "account_sid", "label": "Account SID", "type": "text", "required": true },
    { "key": "auth_token", "label": "Auth Token", "type": "password", "required": true },
    { "key": "from", "label": "From Number", "type": "text", "required": true, "placeholder": "+15551234567" },
    { "key": "to", "label": "To Number", "type": "text", "required": true, "placeholder": "+15557654321" },
    { "key": "message", "label": "SMS Message", "type": "textarea", "required": true, "rows": 3 }
  ]
}
```

`internal/workflow/schemas/comm.whatsapp.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "phone_number_id", "label": "Phone Number ID", "type": "text", "required": true },
    { "key": "access_token", "label": "Access Token", "type": "password", "required": true },
    { "key": "to", "label": "Recipient Number", "type": "text", "required": true, "placeholder": "+15551234567" },
    { "key": "message", "label": "Message Text", "type": "textarea", "required": true, "rows": 3 }
  ]
}
```

**Step 6: Write service schemas**

`internal/workflow/schemas/service.google_sheets.json`:
```json
{
  "credential_platform": "google_sheets",
  "fields": [
    { "key": "credential_id", "label": "Google Account", "type": "text", "required": true, "help": "Select your connected Google account." },
    { "key": "spreadsheet_id", "label": "Spreadsheet", "type": "resource_picker", "required": true, "resource": { "type": "spreadsheets", "create_label": "Create New Spreadsheet" }, "help": "Select or create a Google Sheets spreadsheet." },
    { "key": "sheet_name", "label": "Sheet Name", "type": "text", "required": false, "default": "Sheet1", "help": "Name of the sheet tab. Defaults to Sheet1." },
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["read_rows", "append_rows", "update_rows", "clear_range"], "default": "append_rows" },
    { "key": "range", "label": "Range", "type": "text", "required": false, "placeholder": "e.g. A1:D100", "help": "Leave empty to auto-detect from data.", "depends_on": { "field": "operation", "values": ["read_rows", "update_rows", "clear_range"] } }
  ]
}
```

`internal/workflow/schemas/service.google_drive.json`:
```json
{
  "credential_platform": "google_drive",
  "fields": [
    { "key": "credential_id", "label": "Google Account", "type": "text", "required": true },
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["list_files", "upload", "download", "delete", "create_folder", "move"], "default": "list_files" },
    { "key": "folder_id", "label": "Folder", "type": "resource_picker", "required": false, "resource": { "type": "folders", "create_label": "Create New Folder" } },
    { "key": "file_name", "label": "File Name", "type": "text", "required": false, "depends_on": { "field": "operation", "values": ["upload", "download"] } }
  ]
}
```

`internal/workflow/schemas/service.gmail.json`:
```json
{
  "credential_platform": "gmail",
  "fields": [
    { "key": "credential_id", "label": "Gmail Account", "type": "text", "required": true },
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["send", "list", "get", "label"], "default": "send" },
    { "key": "to", "label": "To", "type": "text", "required": false, "depends_on": { "field": "operation", "values": ["send"] } },
    { "key": "subject", "label": "Subject", "type": "text", "required": false, "depends_on": { "field": "operation", "values": ["send"] } },
    { "key": "body", "label": "Body", "type": "textarea", "required": false, "rows": 5, "depends_on": { "field": "operation", "values": ["send"] } },
    { "key": "label_id", "label": "Label", "type": "resource_picker", "required": false, "resource": { "type": "labels", "create_label": null }, "depends_on": { "field": "operation", "values": ["list", "label"] } }
  ]
}
```

`internal/workflow/schemas/service.github.json`:
```json
{
  "credential_platform": "github",
  "fields": [
    { "key": "credential_id", "label": "GitHub Account", "type": "text", "required": true },
    { "key": "repo", "label": "Repository", "type": "resource_picker", "required": true, "resource": { "type": "repos", "create_label": null } },
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["list_issues", "create_issue", "close_issue", "list_prs", "get_file", "create_file"], "default": "list_issues" },
    { "key": "title", "label": "Issue/PR Title", "type": "text", "required": false, "depends_on": { "field": "operation", "values": ["create_issue"] } },
    { "key": "body", "label": "Body", "type": "textarea", "required": false, "rows": 5, "depends_on": { "field": "operation", "values": ["create_issue"] } }
  ]
}
```

`internal/workflow/schemas/service.notion.json`:
```json
{
  "credential_platform": "notion",
  "fields": [
    { "key": "credential_id", "label": "Notion Connection", "type": "text", "required": true },
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["query_database", "create_page", "update_page", "get_page"], "default": "query_database" },
    { "key": "database_id", "label": "Database", "type": "resource_picker", "required": false, "resource": { "type": "databases", "create_label": null }, "depends_on": { "field": "operation", "values": ["query_database", "create_page"] } },
    { "key": "page_id", "label": "Page ID", "type": "text", "required": false, "depends_on": { "field": "operation", "values": ["update_page", "get_page"] } },
    { "key": "properties", "label": "Page Properties (JSON)", "type": "textarea", "required": false, "rows": 5, "depends_on": { "field": "operation", "values": ["create_page", "update_page"] } }
  ]
}
```

`internal/workflow/schemas/service.airtable.json`:
```json
{
  "credential_platform": "airtable",
  "fields": [
    { "key": "credential_id", "label": "Airtable Account", "type": "text", "required": true },
    { "key": "base_id", "label": "Base", "type": "resource_picker", "required": true, "resource": { "type": "bases", "create_label": null } },
    { "key": "table_id", "label": "Table", "type": "resource_picker", "required": true, "resource": { "type": "tables", "create_label": null, "param_field": "base_id" } },
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["list", "get", "create", "update", "delete"], "default": "list" },
    { "key": "record_id", "label": "Record ID", "type": "text", "required": false, "depends_on": { "field": "operation", "values": ["get", "update", "delete"] } },
    { "key": "fields", "label": "Fields (JSON)", "type": "textarea", "required": false, "rows": 5, "depends_on": { "field": "operation", "values": ["create", "update"] } }
  ]
}
```

`internal/workflow/schemas/service.jira.json`:
```json
{
  "credential_platform": "jira",
  "fields": [
    { "key": "credential_id", "label": "Jira Connection", "type": "text", "required": true },
    { "key": "project_key", "label": "Project", "type": "resource_picker", "required": true, "resource": { "type": "projects", "create_label": null } },
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["list_issues", "create_issue", "update_issue", "transition_issue", "get_issue"], "default": "list_issues" },
    { "key": "issue_key", "label": "Issue Key", "type": "text", "required": false, "placeholder": "PROJ-123", "depends_on": { "field": "operation", "values": ["update_issue", "transition_issue", "get_issue"] } },
    { "key": "summary", "label": "Summary", "type": "text", "required": false, "depends_on": { "field": "operation", "values": ["create_issue"] } },
    { "key": "issue_type", "label": "Issue Type", "type": "text", "required": false, "default": "Task", "depends_on": { "field": "operation", "values": ["create_issue"] } }
  ]
}
```

`internal/workflow/schemas/service.linear.json`:
```json
{
  "credential_platform": "linear",
  "fields": [
    { "key": "credential_id", "label": "Linear Connection", "type": "text", "required": true },
    { "key": "team_id", "label": "Team", "type": "resource_picker", "required": true, "resource": { "type": "teams", "create_label": null } },
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["list_issues", "create_issue", "update_issue", "list_projects"], "default": "list_issues" },
    { "key": "title", "label": "Issue Title", "type": "text", "required": false, "depends_on": { "field": "operation", "values": ["create_issue"] } },
    { "key": "description", "label": "Description", "type": "textarea", "required": false, "rows": 4, "depends_on": { "field": "operation", "values": ["create_issue"] } },
    { "key": "priority", "label": "Priority", "type": "select", "required": false, "options": ["0", "1", "2", "3", "4"], "default": "0", "depends_on": { "field": "operation", "values": ["create_issue"] } }
  ]
}
```

`internal/workflow/schemas/service.asana.json`:
```json
{
  "credential_platform": "asana",
  "fields": [
    { "key": "credential_id", "label": "Asana Connection", "type": "text", "required": true },
    { "key": "workspace_id", "label": "Workspace", "type": "resource_picker", "required": true, "resource": { "type": "workspaces", "create_label": null } },
    { "key": "project_id", "label": "Project", "type": "resource_picker", "required": false, "resource": { "type": "projects", "create_label": null } },
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["list_tasks", "create_task", "complete_task", "get_task"], "default": "list_tasks" },
    { "key": "name", "label": "Task Name", "type": "text", "required": false, "depends_on": { "field": "operation", "values": ["create_task"] } },
    { "key": "due_on", "label": "Due Date (YYYY-MM-DD)", "type": "text", "required": false, "depends_on": { "field": "operation", "values": ["create_task"] } }
  ]
}
```

`internal/workflow/schemas/service.stripe.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "api_key", "label": "Stripe Secret Key", "type": "password", "required": true },
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["list_customers", "get_customer", "list_payments", "get_payment", "create_refund", "list_subscriptions"], "default": "list_customers" },
    { "key": "customer_id", "label": "Customer ID", "type": "text", "required": false, "depends_on": { "field": "operation", "values": ["get_customer", "list_payments", "list_subscriptions"] } },
    { "key": "limit", "label": "Max Results", "type": "number", "required": false, "default": 20 }
  ]
}
```

`internal/workflow/schemas/service.shopify.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "shop_domain", "label": "Shop Domain", "type": "text", "required": true, "placeholder": "mystore.myshopify.com" },
    { "key": "access_token", "label": "Access Token", "type": "password", "required": true },
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["list_orders", "get_order", "list_products", "get_product", "list_customers", "update_order"], "default": "list_orders" },
    { "key": "limit", "label": "Max Results", "type": "number", "required": false, "default": 20 },
    { "key": "status", "label": "Order Status Filter", "type": "select", "required": false, "options": ["any", "open", "closed", "cancelled"], "default": "open", "depends_on": { "field": "operation", "values": ["list_orders"] } }
  ]
}
```

`internal/workflow/schemas/service.salesforce.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "instance_url", "label": "Instance URL", "type": "text", "required": true, "placeholder": "https://myorg.salesforce.com" },
    { "key": "access_token", "label": "Access Token", "type": "password", "required": true },
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["query", "create", "update", "delete", "describe"], "default": "query" },
    { "key": "object_type", "label": "Object Type", "type": "text", "required": true, "placeholder": "Contact" },
    { "key": "soql", "label": "SOQL Query", "type": "code", "language": "sql", "required": false, "depends_on": { "field": "operation", "values": ["query"] } },
    { "key": "record", "label": "Record Data (JSON)", "type": "textarea", "required": false, "rows": 5, "depends_on": { "field": "operation", "values": ["create", "update"] } }
  ]
}
```

`internal/workflow/schemas/service.hubspot.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "api_key", "label": "HubSpot API Key", "type": "password", "required": true },
    { "key": "object_type", "label": "Object Type", "type": "select", "required": true, "options": ["contacts", "companies", "deals", "tickets"], "default": "contacts" },
    { "key": "operation", "label": "Operation", "type": "select", "required": true, "options": ["list", "get", "create", "update", "delete", "search"], "default": "list" },
    { "key": "properties", "label": "Properties (JSON)", "type": "textarea", "required": false, "rows": 5, "depends_on": { "field": "operation", "values": ["create", "update"] } },
    { "key": "filter_groups", "label": "Search Filters (JSON)", "type": "textarea", "required": false, "rows": 5, "depends_on": { "field": "operation", "values": ["search"] } }
  ]
}
```

`internal/workflow/schemas/browser.generic.json`:
```json
{
  "credential_platform": null,
  "fields": [
    { "key": "username", "label": "Session Username", "type": "text", "required": false, "help": "Username of the browser session to use. Leave blank to use the default session." },
    { "key": "keywords", "label": "Keywords", "type": "text", "required": false },
    { "key": "message", "label": "Message", "type": "textarea", "required": false, "rows": 3 },
    { "key": "targets", "label": "Target List", "type": "array", "required": false, "item_type": "text", "help": "List of target usernames, URLs, or identifiers." },
    { "key": "limit", "label": "Max Actions", "type": "number", "required": false, "default": 10 }
  ]
}
```

**Step 7: Verify count**
```bash
ls internal/workflow/schemas/ | wc -l
# Expected: 50 files
```

**Step 8: Commit**
```bash
git add internal/workflow/schemas/
git commit -m "feat: add default schemas for all 50 node types"
```

---

### Task 3: Go schema loader with //go:embed

**Files:**
- Create: `internal/workflow/schema_loader.go`
- Modify: `internal/workflow/models.go` — add `Schema` field to `WorkflowNode`

**Step 1: Add Schema structs and loader**

Create `internal/workflow/schema_loader.go`:
```go
package workflow

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed schemas/*.json
var embeddedSchemas embed.FS

// NodeSchemaField represents one configurable field in a node's schema.
type NodeSchemaField struct {
	Key         string                 `json:"key"`
	Label       string                 `json:"label"`
	Type        string                 `json:"type"`        // text, number, password, textarea, select, boolean, array, code, resource_picker
	Required    bool                   `json:"required"`
	Default     interface{}            `json:"default,omitempty"`
	Placeholder string                 `json:"placeholder,omitempty"`
	Help        string                 `json:"help,omitempty"`
	Options     []string               `json:"options,omitempty"`       // for select
	Language    string                 `json:"language,omitempty"`      // for code
	Rows        int                    `json:"rows,omitempty"`          // for textarea
	Min         *float64               `json:"min,omitempty"`           // for number
	Max         *float64               `json:"max,omitempty"`           // for number
	ItemType    string                 `json:"item_type,omitempty"`     // for array
	Resource    *ResourcePickerConfig  `json:"resource,omitempty"`      // for resource_picker
	DependsOn   *FieldDependency       `json:"depends_on,omitempty"`
}

// ResourcePickerConfig configures a resource_picker field.
type ResourcePickerConfig struct {
	Type        string `json:"type"`                    // e.g. "spreadsheets", "channels"
	CreateLabel string `json:"create_label,omitempty"`  // if set, show "Create New" button
	ParamField  string `json:"param_field,omitempty"`   // field that provides a parent ID
}

// FieldDependency hides a field unless another field has one of the given values.
type FieldDependency struct {
	Field  string   `json:"field"`
	Values []string `json:"values"`
}

// NodeSchema is the schema embedded in each workflow node.
type NodeSchema struct {
	CredentialPlatform *string          `json:"credential_platform"` // nil = no credential needed
	Fields             []NodeSchemaField `json:"fields"`
}

// LoadDefaultSchema loads the embedded schema JSON for the given node type.
// nodeType uses dots (e.g. "service.google_sheets") which maps to "schemas/service.google_sheets.json".
// Returns an empty schema (no fields) if the node type has no schema file.
func LoadDefaultSchema(nodeType string) (*NodeSchema, error) {
	fileName := "schemas/" + nodeType + ".json"
	data, err := embeddedSchemas.ReadFile(fileName)
	if err != nil {
		// No schema for this node type — return empty schema
		return &NodeSchema{Fields: []NodeSchemaField{}}, nil
	}
	var schema NodeSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("schema_loader: parse %s: %w", fileName, err)
	}
	if schema.Fields == nil {
		schema.Fields = []NodeSchemaField{}
	}
	return &schema, nil
}

// ListEmbeddedSchemas returns all node type names that have an embedded schema.
func ListEmbeddedSchemas() []string {
	entries, err := embeddedSchemas.ReadDir("schemas")
	if err != nil {
		return nil
	}
	types := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".json") {
			types = append(types, strings.TrimSuffix(name, ".json"))
		}
	}
	return types
}
```

**Step 2: Add Schema field to WorkflowNode**

In `internal/workflow/models.go`, add `Schema` field after `Disabled`:
```go
// WorkflowNode is one node in the workflow graph.
type WorkflowNode struct {
	ID         string                 `json:"id" db:"id"`
	WorkflowID string                 `json:"workflow_id" db:"workflow_id"`
	Type       string                 `json:"node_type" db:"node_type"`
	Name       string                 `json:"name" db:"name"`
	Config     map[string]interface{} `json:"config" db:"-"`
	ConfigRaw  string                 `json:"-" db:"config"`
	PositionX  float64                `json:"position_x" db:"position_x"`
	PositionY  float64                `json:"position_y" db:"position_y"`
	Disabled   bool                   `json:"disabled" db:"disabled"`
	Schema     *NodeSchema            `json:"schema,omitempty" db:"-"` // embedded schema, not stored in SQLite
	CreatedAt  time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at" db:"updated_at"`
}
```

**Step 3: Write test**

Create `internal/workflow/schema_loader_test.go`:
```go
package workflow

import (
	"testing"
)

func TestLoadDefaultSchema_KnownType(t *testing.T) {
	schema, err := LoadDefaultSchema("service.google_sheets")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(schema.Fields) == 0 {
		t.Fatal("expected fields for service.google_sheets")
	}
	// Must have spreadsheet_id as resource_picker
	var found bool
	for _, f := range schema.Fields {
		if f.Key == "spreadsheet_id" && f.Type == "resource_picker" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected spreadsheet_id resource_picker field")
	}
}

func TestLoadDefaultSchema_UnknownType(t *testing.T) {
	schema, err := LoadDefaultSchema("unknown.node_type")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if schema == nil {
		t.Fatal("expected non-nil schema for unknown type")
	}
	if len(schema.Fields) != 0 {
		t.Fatal("expected empty fields for unknown type")
	}
}

func TestListEmbeddedSchemas(t *testing.T) {
	types := ListEmbeddedSchemas()
	if len(types) < 30 {
		t.Fatalf("expected at least 30 embedded schemas, got %d", len(types))
	}
}
```

**Step 4: Run test**
```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes
go test ./internal/workflow/... -run TestLoadDefaultSchema -v
# Expected: PASS
```

**Step 5: Commit**
```bash
git add internal/workflow/schema_loader.go internal/workflow/schema_loader_test.go internal/workflow/models.go
git commit -m "feat: add NodeSchema types, schema_loader with go:embed, Schema field on WorkflowNode"
```

---

### Task 4: WorkflowFileStore — read/write JSON files

**Files:**
- Create: `internal/workflow/file_store.go`
- Create: `internal/workflow/file_store_test.go`

**Context:** The `WorkflowStore` interface is in `internal/workflow/storage.go`. The file store only needs to implement workflow CRUD — executions and credentials still use `SQLiteWorkflowStore`.

**Step 1: Write the file store**

Create `internal/workflow/file_store.go`:
```go
package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
)

// WorkflowFile is the on-disk representation of a workflow JSON file.
// It extends Workflow with file-format specifics (connections use a flat format).
type WorkflowFile struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Version     int                  `json:"version"`
	IsActive    bool                 `json:"is_active"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
	Nodes       []WorkflowFileNode   `json:"nodes"`
	Connections []WorkflowFileEdge   `json:"connections"`
}

// WorkflowFileNode is a node as stored in the JSON file.
type WorkflowFileNode struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Name     string                 `json:"name"`
	Position struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	} `json:"position"`
	Disabled bool                   `json:"disabled"`
	Config   map[string]interface{} `json:"config"`
	Schema   *NodeSchema            `json:"schema"`
}

// WorkflowFileEdge is a connection as stored in the JSON file.
type WorkflowFileEdge struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	SourceHandle string `json:"source_handle"`
	Target       string `json:"target"`
	TargetHandle string `json:"target_handle"`
}

// WorkflowFileStore implements workflow CRUD using JSON files in a directory.
// Executions and credentials are NOT handled here — use SQLiteWorkflowStore for those.
type WorkflowFileStore struct {
	dir string // e.g. ~/.monoes/workflows
}

// NewWorkflowFileStore creates a WorkflowFileStore backed by the given directory.
// The directory is created if it does not exist.
func NewWorkflowFileStore(dir string) (*WorkflowFileStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("file_store: create dir %s: %w", dir, err)
	}
	return &WorkflowFileStore{dir: dir}, nil
}

func (s *WorkflowFileStore) filePath(id string) string {
	return filepath.Join(s.dir, id+".json")
}

// SaveWorkflow writes or updates a workflow JSON file.
// If wf.ID is empty a new UUID is assigned.
// Nodes must have their Schema field pre-populated before calling SaveWorkflow.
func (s *WorkflowFileStore) SaveWorkflow(ctx context.Context, wf *Workflow) error {
	if wf.ID == "" {
		wf.ID = uuid.New().String()
		wf.CreatedAt = time.Now().UTC()
	}
	wf.UpdatedAt = time.Now().UTC()
	if wf.Version == 0 {
		wf.Version = 1
	}

	// Convert to file format
	wfFile := WorkflowFile{
		ID:          wf.ID,
		Name:        wf.Name,
		Description: wf.Description,
		Version:     wf.Version,
		IsActive:    wf.IsActive,
		CreatedAt:   wf.CreatedAt,
		UpdatedAt:   wf.UpdatedAt,
	}

	for _, n := range wf.Nodes {
		fn := WorkflowFileNode{
			ID:       n.ID,
			Type:     n.Type,
			Name:     n.Name,
			Disabled: n.Disabled,
			Config:   n.Config,
			Schema:   n.Schema,
		}
		fn.Position.X = n.PositionX
		fn.Position.Y = n.PositionY
		if fn.Config == nil {
			fn.Config = map[string]interface{}{}
		}
		// If schema not set, load default
		if fn.Schema == nil {
			schema, err := LoadDefaultSchema(n.Type)
			if err == nil {
				fn.Schema = schema
			}
		}
		wfFile.Nodes = append(wfFile.Nodes, fn)
	}

	for _, c := range wf.Connections {
		wfFile.Connections = append(wfFile.Connections, WorkflowFileEdge{
			ID:           c.ID,
			Source:       c.SourceNodeID,
			SourceHandle: c.SourceHandle,
			Target:       c.TargetNodeID,
			TargetHandle: c.TargetHandle,
		})
	}

	data, err := json.MarshalIndent(wfFile, "", "  ")
	if err != nil {
		return fmt.Errorf("file_store: marshal %s: %w", wf.ID, err)
	}
	return os.WriteFile(s.filePath(wf.ID), data, 0o644)
}

// GetWorkflow reads a workflow JSON file and returns the Workflow.
func (s *WorkflowFileStore) GetWorkflow(ctx context.Context, id string) (*Workflow, error) {
	data, err := os.ReadFile(s.filePath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("file_store: read %s: %w", id, err)
	}
	return parseWorkflowFile(data)
}

// ListWorkflows scans the directory and returns all workflows sorted by UpdatedAt desc.
func (s *WorkflowFileStore) ListWorkflows(ctx context.Context) ([]*Workflow, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("file_store: readdir %s: %w", s.dir, err)
	}
	var wfs []*Workflow
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := e.Name()[:len(e.Name())-5]
		data, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		wf, err := parseWorkflowFile(data)
		if err != nil {
			continue
		}
		_ = id
		wfs = append(wfs, wf)
	}
	sort.Slice(wfs, func(i, j int) bool {
		return wfs[i].UpdatedAt.After(wfs[j].UpdatedAt)
	})
	return wfs, nil
}

// DeleteWorkflow removes the workflow JSON file.
func (s *WorkflowFileStore) DeleteWorkflow(ctx context.Context, id string) error {
	err := os.Remove(s.filePath(id))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// parseWorkflowFile converts WorkflowFile JSON bytes into a *Workflow.
func parseWorkflowFile(data []byte) (*Workflow, error) {
	var wfFile WorkflowFile
	if err := json.Unmarshal(data, &wfFile); err != nil {
		return nil, fmt.Errorf("file_store: unmarshal: %w", err)
	}
	wf := &Workflow{
		ID:          wfFile.ID,
		Name:        wfFile.Name,
		Description: wfFile.Description,
		Version:     wfFile.Version,
		IsActive:    wfFile.IsActive,
		CreatedAt:   wfFile.CreatedAt,
		UpdatedAt:   wfFile.UpdatedAt,
	}
	for _, fn := range wfFile.Nodes {
		n := WorkflowNode{
			ID:         fn.ID,
			WorkflowID: wf.ID,
			Type:       fn.Type,
			Name:       fn.Name,
			PositionX:  fn.Position.X,
			PositionY:  fn.Position.Y,
			Disabled:   fn.Disabled,
			Config:     fn.Config,
			Schema:     fn.Schema,
		}
		if n.Config == nil {
			n.Config = map[string]interface{}{}
		}
		wf.Nodes = append(wf.Nodes, n)
	}
	for _, fe := range wfFile.Connections {
		wf.Connections = append(wf.Connections, WorkflowConnection{
			ID:           fe.ID,
			WorkflowID:   wf.ID,
			SourceNodeID: fe.Source,
			SourceHandle: fe.SourceHandle,
			TargetNodeID: fe.Target,
			TargetHandle: fe.TargetHandle,
		})
	}
	return wf, nil
}
```

**Step 2: Write test**

Create `internal/workflow/file_store_test.go`:
```go
package workflow

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestWorkflowFileStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := NewWorkflowFileStore(dir)
	if err != nil {
		t.Fatalf("NewWorkflowFileStore: %v", err)
	}
	ctx := context.Background()

	wf := &Workflow{
		Name:      "Test Workflow",
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
		Nodes: []WorkflowNode{
			{
				ID:   "node-1",
				Type: "trigger.manual",
				Name: "Start",
				Schema: &NodeSchema{Fields: []NodeSchemaField{}},
				Config: map[string]interface{}{},
			},
		},
		Connections: []WorkflowConnection{},
	}

	if err := store.SaveWorkflow(ctx, wf); err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}
	if wf.ID == "" {
		t.Fatal("expected ID to be assigned")
	}

	loaded, err := store.GetWorkflow(ctx, wf.ID)
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil workflow")
	}
	if loaded.Name != "Test Workflow" {
		t.Errorf("name mismatch: got %q", loaded.Name)
	}
	if len(loaded.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(loaded.Nodes))
	}

	list, err := store.ListWorkflows(ctx)
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 workflow in list, got %d", len(list))
	}

	if err := store.DeleteWorkflow(ctx, wf.ID); err != nil {
		t.Fatalf("DeleteWorkflow: %v", err)
	}
	_, statErr := os.Stat(store.filePath(wf.ID))
	if !os.IsNotExist(statErr) {
		t.Fatal("expected file to be deleted")
	}
}
```

**Step 3: Run test**
```bash
go test ./internal/workflow/... -run TestWorkflowFileStore -v
# Expected: PASS
```

**Step 4: Commit**
```bash
git add internal/workflow/file_store.go internal/workflow/file_store_test.go
git commit -m "feat: add WorkflowFileStore for JSON-file-based workflow CRUD"
```

---

### Task 5: Migration CLI command

**Files:**
- Modify: `cmd/monoes/workflow.go` — add `monoes workflow migrate` subcommand

**Context:** The migration reads all workflows from SQLite (`SQLiteWorkflowStore`), embeds default schemas for each node, and writes JSON files. It does not delete SQLite data.

**Step 1: Read the existing workflow.go subcommands**

Look at how `runWorkflowList` and `runWorkflowRun` are structured in `cmd/monoes/workflow.go`. The `migrate` command follows the same pattern.

**Step 2: Add migrate command**

In `cmd/monoes/workflow.go`, add to the `workflowCmd` subcommands and add this function:
```go
func runWorkflowMigrate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Open SQLite store (existing source)
	dbPath := filepath.Join(os.Getenv("HOME"), ".monoes", "monoes.db")
	sqliteStore, err := workflow.NewSQLiteWorkflowStore(dbPath)
	if err != nil {
		return fmt.Errorf("open sqlite store: %w", err)
	}

	// Open file store (destination)
	wfDir := filepath.Join(os.Getenv("HOME"), ".monoes", "workflows")
	fileStore, err := workflow.NewWorkflowFileStore(wfDir)
	if err != nil {
		return fmt.Errorf("open file store: %w", err)
	}

	workflows, err := sqliteStore.ListWorkflows(ctx)
	if err != nil {
		return fmt.Errorf("list workflows from sqlite: %w", err)
	}

	fmt.Printf("Found %d workflows in SQLite. Migrating to %s...\n", len(workflows), wfDir)

	var migrated, skipped int
	for _, wf := range workflows {
		// Load full workflow (nodes + connections)
		full, err := sqliteStore.GetWorkflow(ctx, wf.ID)
		if err != nil {
			fmt.Printf("  SKIP %s (%s): load error: %v\n", wf.ID, wf.Name, err)
			skipped++
			continue
		}
		if full == nil {
			skipped++
			continue
		}

		// Embed default schema for each node if not already set
		for i, n := range full.Nodes {
			if n.Schema == nil {
				schema, err := workflow.LoadDefaultSchema(n.Type)
				if err != nil {
					schema = &workflow.NodeSchema{Fields: []workflow.NodeSchemaField{}}
				}
				full.Nodes[i].Schema = schema
			}
		}

		// Write JSON file (preserves existing ID and timestamps)
		if err := fileStore.SaveWorkflow(ctx, full); err != nil {
			fmt.Printf("  SKIP %s (%s): write error: %v\n", wf.ID, wf.Name, err)
			skipped++
			continue
		}
		fmt.Printf("  OK   %s (%s)\n", wf.ID, wf.Name)
		migrated++
	}

	fmt.Printf("\nMigration complete: %d migrated, %d skipped.\n", migrated, skipped)
	fmt.Println("SQLite data was NOT modified — safe to roll back.")
	return nil
}
```

Register in the cobra command tree:
```go
var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate workflows from SQLite to JSON files",
	RunE:  runWorkflowMigrate,
}
// In init() or where subcommands are added:
workflowCmd.AddCommand(migrateCmd)
```

**Step 3: Build and test manually**
```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes
go build ./cmd/monoes/...
./monoes workflow migrate
# Expected: "Found N workflows in SQLite. Migrating..."
```

**Step 4: Commit**
```bash
git add cmd/monoes/workflow.go
git commit -m "feat: add monoes workflow migrate command"
```

---

### Task 6: Update app.go to use WorkflowFileStore

**Files:**
- Modify: `wails-app/app.go` — replace SQLite workflow CRUD with file store

**Context:** `SaveWorkflow`, `LoadWorkflow` (aka `GetWorkflow`), `ListWorkflows`, `DeleteWorkflow`, and `SetWorkflowActive` currently use `a.db` directly. They need to delegate to `WorkflowFileStore`. Executions, credentials, connections remain in SQLite.

**Step 1: Add WorkflowFileStore to App struct**

In `wails-app/app.go`, find the `App` struct. Add a field:
```go
type App struct {
	// ... existing fields ...
	wfStore *workflow.WorkflowFileStore
}
```

**Step 2: Initialize in startup**

In `startup()` (called by Wails), after the existing DB init:
```go
wfDir := filepath.Join(os.Getenv("HOME"), ".monoes", "workflows")
wfStore, err := workflow.NewWorkflowFileStore(wfDir)
if err != nil {
	a.logger.Error().Err(err).Msg("failed to init workflow file store")
} else {
	a.wfStore = wfStore
}
```

**Step 3: Rewrite ListWorkflows**

Replace the existing `ListWorkflows` function body:
```go
func (a *App) ListWorkflows() ([]WorkflowSummary, error) {
	if a.wfStore == nil {
		return nil, fmt.Errorf("workflow store not available")
	}
	ctx := context.Background()
	wfs, err := a.wfStore.ListWorkflows(ctx)
	if err != nil {
		return nil, err
	}
	summaries := make([]WorkflowSummary, 0, len(wfs))
	for _, wf := range wfs {
		summaries = append(summaries, WorkflowSummary{
			ID:          wf.ID,
			Name:        wf.Name,
			Description: wf.Description,
			IsActive:    wf.IsActive,
			CreatedAt:   wf.CreatedAt.Format(time.RFC3339),
			UpdatedAt:   wf.UpdatedAt.Format(time.RFC3339),
		})
	}
	return summaries, nil
}
```

**Step 4: Rewrite GetWorkflow**

Replace the existing `GetWorkflow` body:
```go
func (a *App) GetWorkflow(id string) (*WorkflowDetail, error) {
	if a.wfStore == nil {
		return nil, fmt.Errorf("workflow store not available")
	}
	ctx := context.Background()
	wf, err := a.wfStore.GetWorkflow(ctx, id)
	if err != nil {
		return nil, err
	}
	if wf == nil {
		return nil, fmt.Errorf("workflow %s not found", id)
	}
	return workflowToDetail(wf), nil
}
```

You'll need a helper `workflowToDetail(*workflow.Workflow) *WorkflowDetail` that converts the internal model to the Wails-exposed struct. Look at the existing conversion logic in the current `GetWorkflow` and extract it.

**Step 5: Rewrite SaveWorkflow**

Replace the existing `SaveWorkflow` body. The request comes in as `SaveWorkflowRequest` (existing struct). Convert it to `workflow.Workflow`, call `wfStore.SaveWorkflow`, return the ID.

```go
func (a *App) SaveWorkflow(req SaveWorkflowRequest) (*WorkflowSummary, error) {
	if a.wfStore == nil {
		return nil, fmt.Errorf("workflow store not available")
	}
	ctx := context.Background()

	wf := &workflow.Workflow{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
		IsActive:    req.IsActive,
	}

	for _, n := range req.Nodes {
		node := workflow.WorkflowNode{
			ID:        n.ID,
			Type:      n.Type,
			Name:      n.Name,
			PositionX: n.PositionX,
			PositionY: n.PositionY,
			Disabled:  n.Disabled,
			Config:    n.Config,
		}
		// Embed schema: use the schema from the request if present,
		// otherwise load the default for this node type.
		if n.Schema != nil {
			node.Schema = n.Schema
		} else {
			schema, _ := workflow.LoadDefaultSchema(n.Type)
			node.Schema = schema
		}
		wf.Nodes = append(wf.Nodes, node)
	}

	for _, c := range req.Connections {
		wf.Connections = append(wf.Connections, workflow.WorkflowConnection{
			ID:           c.ID,
			SourceNodeID: c.SourceNodeID,
			SourceHandle: c.SourceHandle,
			TargetNodeID: c.TargetNodeID,
			TargetHandle: c.TargetHandle,
		})
	}

	if err := a.wfStore.SaveWorkflow(ctx, wf); err != nil {
		return nil, err
	}
	return &WorkflowSummary{
		ID:        wf.ID,
		Name:      wf.Name,
		IsActive:  wf.IsActive,
		UpdatedAt: wf.UpdatedAt.Format(time.RFC3339),
	}, nil
}
```

**Step 6: Rewrite DeleteWorkflow**

```go
func (a *App) DeleteWorkflow(id string) error {
	if a.wfStore == nil {
		return fmt.Errorf("workflow store not available")
	}
	return a.wfStore.DeleteWorkflow(context.Background(), id)
}
```

**Step 7: Update SetWorkflowActive**

Read the workflow, flip IsActive, re-save:
```go
func (a *App) SetWorkflowActive(id string, active bool) error {
	if a.wfStore == nil {
		return fmt.Errorf("workflow store not available")
	}
	ctx := context.Background()
	wf, err := a.wfStore.GetWorkflow(ctx, id)
	if err != nil || wf == nil {
		return fmt.Errorf("workflow %s not found", id)
	}
	wf.IsActive = active
	return a.wfStore.SaveWorkflow(ctx, wf)
}
```

**Step 8: Update SaveWorkflowRequest and NodeRequest structs**

In `wails-app/app.go` (where request/response structs are defined), add `Schema *workflow.NodeSchema` to the node request struct so the frontend can pass schemas through:
```go
type WorkflowNodeRequest struct {
	ID        string                       `json:"id"`
	Type      string                       `json:"type"`
	Name      string                       `json:"name"`
	PositionX float64                      `json:"position_x"`
	PositionY float64                      `json:"position_y"`
	Disabled  bool                         `json:"disabled"`
	Config    map[string]interface{}        `json:"config"`
	Schema    *workflow.NodeSchema          `json:"schema,omitempty"`
}
```

**Step 9: Build to verify compilation**
```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes/wails-app
make build 2>&1 | head -50
# Must compile without errors
```

**Step 10: Manually add new functions to wailsjs bindings**

Edit `wails-app/frontend/src/wailsjs/go/main/App.js` — the Wails binding generator won't automatically pick up parameter type changes. Verify `SaveWorkflow`, `ListWorkflows`, `GetWorkflow`, `DeleteWorkflow`, `SetWorkflowActive` are present and have correct signatures.

**Step 11: Commit**
```bash
git add wails-app/app.go wails-app/frontend/src/wailsjs/go/main/App.js
git commit -m "feat: wire app.go workflow CRUD to WorkflowFileStore"
```

---

### Task 7: ListResources backend (google_sheets + slack)

**Files:**
- Create: `wails-app/resources.go` (or add to `app.go` — prefer separate file)

**Step 1: Write resource types and dispatcher**

Create `wails-app/resources.go`:
```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// ResourceItem is a single listable resource (spreadsheet, channel, etc.)
type ResourceItem struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ResourceListResult is returned by ListResources.
type ResourceListResult struct {
	Items      []ResourceItem `json:"items"`
	NextCursor string         `json:"next_cursor,omitempty"`
	Error      string         `json:"error,omitempty"`
}

// ResourceItemResult is returned by CreateResource.
type ResourceItemResult struct {
	Item  *ResourceItem `json:"item,omitempty"`
	Error string        `json:"error,omitempty"`
}

// ListResources lists external resources for a given platform and resource type.
// credentialID is the connection ID stored in the connections table.
// query is an optional search string.
func (a *App) ListResources(platform, resourceType, credentialID, query string) ResourceListResult {
	ctx := context.Background()

	// Fetch credential data
	creds, err := a.getCredentialData(ctx, credentialID)
	if err != nil {
		return ResourceListResult{Error: fmt.Sprintf("credential lookup: %v", err)}
	}

	switch platform {
	case "google_sheets", "google_drive":
		return listGoogleDriveResources(creds, resourceType, query)
	case "gmail":
		return listGmailResources(creds, resourceType, query)
	case "slack":
		return listSlackResources(creds, resourceType, query)
	default:
		return ResourceListResult{Error: fmt.Sprintf("platform %q not supported for resource listing", platform)}
	}
}

// CreateResource creates a new external resource and returns the created item.
func (a *App) CreateResource(platform, resourceType, credentialID, name string) ResourceItemResult {
	ctx := context.Background()

	creds, err := a.getCredentialData(ctx, credentialID)
	if err != nil {
		return ResourceItemResult{Error: fmt.Sprintf("credential lookup: %v", err)}
	}

	switch platform {
	case "google_sheets":
		return createGoogleSheet(creds, name)
	case "google_drive":
		return createGoogleDriveFolder(creds, name)
	default:
		return ResourceItemResult{Error: fmt.Sprintf("create not supported for platform %q", platform)}
	}
}

// getCredentialData fetches credential data from the connections store.
func (a *App) getCredentialData(ctx context.Context, credentialID string) (map[string]interface{}, error) {
	if a.connStore == nil {
		return nil, fmt.Errorf("connections store not available")
	}
	conn, err := a.connStore.Get(ctx, credentialID)
	if err != nil || conn == nil {
		return nil, fmt.Errorf("credential %s not found", credentialID)
	}
	return conn.Data, nil
}

// listGoogleDriveResources lists Google Drive/Sheets resources.
func listGoogleDriveResources(creds map[string]interface{}, resourceType, query string) ResourceListResult {
	accessToken, _ := creds["access_token"].(string)
	if accessToken == "" {
		return ResourceListResult{Error: "google: access_token not found in credential"}
	}

	var apiURL string
	switch resourceType {
	case "spreadsheets":
		q := "mimeType='application/vnd.google-apps.spreadsheet' and trashed=false"
		if query != "" {
			q += " and name contains '" + query + "'"
		}
		apiURL = "https://www.googleapis.com/drive/v3/files?q=" + url.QueryEscape(q) + "&fields=files(id,name,modifiedTime)&pageSize=50"
	case "folders":
		q := "mimeType='application/vnd.google-apps.folder' and trashed=false"
		if query != "" {
			q += " and name contains '" + query + "'"
		}
		apiURL = "https://www.googleapis.com/drive/v3/files?q=" + url.QueryEscape(q) + "&fields=files(id,name,modifiedTime)&pageSize=50"
	default:
		return ResourceListResult{Error: fmt.Sprintf("google_drive: unsupported resource type %q", resourceType)}
	}

	body, err := googleAPIGet(apiURL, accessToken)
	if err != nil {
		return ResourceListResult{Error: err.Error()}
	}

	var resp struct {
		Files []struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			ModifiedTime string `json:"modifiedTime"`
		} `json:"files"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return ResourceListResult{Error: fmt.Sprintf("google: parse response: %v", err)}
	}

	items := make([]ResourceItem, 0, len(resp.Files))
	for _, f := range resp.Files {
		items = append(items, ResourceItem{
			ID:   f.ID,
			Name: f.Name,
			Metadata: map[string]interface{}{
				"modified_time": f.ModifiedTime,
			},
		})
	}
	return ResourceListResult{Items: items}
}

// listGmailResources lists Gmail labels.
func listGmailResources(creds map[string]interface{}, resourceType, query string) ResourceListResult {
	accessToken, _ := creds["access_token"].(string)
	if accessToken == "" {
		return ResourceListResult{Error: "gmail: access_token not found in credential"}
	}
	if resourceType != "labels" {
		return ResourceListResult{Error: fmt.Sprintf("gmail: unsupported resource type %q", resourceType)}
	}

	body, err := googleAPIGet("https://gmail.googleapis.com/gmail/v1/users/me/labels", accessToken)
	if err != nil {
		return ResourceListResult{Error: err.Error()}
	}

	var resp struct {
		Labels []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return ResourceListResult{Error: fmt.Sprintf("gmail: parse response: %v", err)}
	}

	items := make([]ResourceItem, 0, len(resp.Labels))
	for _, l := range resp.Labels {
		items = append(items, ResourceItem{ID: l.ID, Name: l.Name})
	}
	return ResourceListResult{Items: items}
}

// listSlackResources lists Slack channels or users.
func listSlackResources(creds map[string]interface{}, resourceType, query string) ResourceListResult {
	token, _ := creds["access_token"].(string)
	if token == "" {
		token, _ = creds["bot_token"].(string)
	}
	if token == "" {
		return ResourceListResult{Error: "slack: access_token or bot_token not found in credential"}
	}

	var apiURL string
	switch resourceType {
	case "channels":
		apiURL = "https://slack.com/api/conversations.list?limit=200&exclude_archived=true"
	case "users":
		apiURL = "https://slack.com/api/users.list?limit=200"
	default:
		return ResourceListResult{Error: fmt.Sprintf("slack: unsupported resource type %q", resourceType)}
	}

	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ResourceListResult{Error: fmt.Sprintf("slack: http: %v", err)}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var slackResp struct {
		OK       bool   `json:"ok"`
		Error    string `json:"error"`
		Channels []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"channels"`
		Members []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Profile struct {
				RealName string `json:"real_name"`
			} `json:"profile"`
		} `json:"members"`
	}
	if err := json.Unmarshal(body, &slackResp); err != nil {
		return ResourceListResult{Error: fmt.Sprintf("slack: parse: %v", err)}
	}
	if !slackResp.OK {
		return ResourceListResult{Error: fmt.Sprintf("slack: API error: %s", slackResp.Error)}
	}

	var items []ResourceItem
	for _, c := range slackResp.Channels {
		items = append(items, ResourceItem{ID: c.ID, Name: "#" + c.Name})
	}
	for _, m := range slackResp.Members {
		displayName := m.Profile.RealName
		if displayName == "" {
			displayName = m.Name
		}
		items = append(items, ResourceItem{ID: m.ID, Name: displayName})
	}
	return ResourceListResult{Items: items}
}

// createGoogleSheet creates a new Google Sheet and returns its ID.
func createGoogleSheet(creds map[string]interface{}, name string) ResourceItemResult {
	accessToken, _ := creds["access_token"].(string)
	if accessToken == "" {
		return ResourceItemResult{Error: "google: access_token not found"}
	}

	payload := fmt.Sprintf(`{"properties":{"title":%q}}`, name)
	req, _ := http.NewRequest("POST", "https://sheets.googleapis.com/v4/spreadsheets",
		io.NopCloser(bytes.NewBufferString(payload)))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ResourceItemResult{Error: fmt.Sprintf("google: create sheet: %v", err)}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var created struct {
		SpreadsheetID string `json:"spreadsheetId"`
		Properties    struct {
			Title string `json:"title"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(body, &created); err != nil || created.SpreadsheetID == "" {
		return ResourceItemResult{Error: fmt.Sprintf("google: parse create response: %v", err)}
	}
	return ResourceItemResult{Item: &ResourceItem{
		ID:   created.SpreadsheetID,
		Name: created.Properties.Title,
	}}
}

// createGoogleDriveFolder creates a new folder in Google Drive.
func createGoogleDriveFolder(creds map[string]interface{}, name string) ResourceItemResult {
	accessToken, _ := creds["access_token"].(string)
	if accessToken == "" {
		return ResourceItemResult{Error: "google: access_token not found"}
	}

	payload := fmt.Sprintf(`{"name":%q,"mimeType":"application/vnd.google-apps.folder"}`, name)
	req, _ := http.NewRequest("POST", "https://www.googleapis.com/drive/v3/files",
		io.NopCloser(bytes.NewBufferString(payload)))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ResourceItemResult{Error: fmt.Sprintf("google drive: create folder: %v", err)}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var created struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &created); err != nil || created.ID == "" {
		return ResourceItemResult{Error: fmt.Sprintf("google drive: parse create response: %v", err)}
	}
	return ResourceItemResult{Item: &ResourceItem{ID: created.ID, Name: created.Name}}
}

// googleAPIGet performs a GET request to a Google API endpoint with bearer auth.
func googleAPIGet(apiURL, accessToken string) ([]byte, error) {
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google API GET: %w", err)
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
```

Add `"bytes"` to the imports.

**Step 2: Build**
```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes/wails-app
go build ./... 2>&1 | head -30
```

**Step 3: Add to wailsjs bindings**

In `wails-app/frontend/src/wailsjs/go/main/App.js`, add:
```js
export function ListResources(platform, resourceType, credentialID, query) {
  return window['go']['main']['App']['ListResources'](platform, resourceType, credentialID, query);
}
export function CreateResource(platform, resourceType, credentialID, name) {
  return window['go']['main']['App']['CreateResource'](platform, resourceType, credentialID, name);
}
```

**Step 4: Commit**
```bash
git add wails-app/resources.go wails-app/frontend/src/wailsjs/go/main/App.js
git commit -m "feat: add ListResources and CreateResource Wails functions (google_sheets, slack)"
```

---

### Task 8: Update GetWorkflowNodeTypes to return schemas

**Files:**
- Modify: `wails-app/app.go` — `GetWorkflowNodeTypes`

**Step 1: Extend nodeDesc to include schema**

In `GetWorkflowNodeTypes`, change `nodeDesc` to include `Schema`:
```go
type nodeDesc struct {
	Type        string               `json:"type"`
	Label       string               `json:"label"`
	Category    string               `json:"category"`
	Description string               `json:"description"`
	Schema      *workflow.NodeSchema `json:"schema,omitempty"`
}
mkNode := func(t, label, cat, desc string) nodeDesc {
	schema, _ := workflow.LoadDefaultSchema(t)
	return nodeDesc{Type: t, Label: label, Category: cat, Description: desc, Schema: schema}
}
```

That's the only change — `mkNode` now auto-loads the schema. All existing `mkNode(...)` calls stay the same.

**Step 2: Build and verify**
```bash
go build ./... && echo OK
```

**Step 3: Commit**
```bash
git add wails-app/app.go
git commit -m "feat: GetWorkflowNodeTypes now includes embedded schema per node type"
```

---

### Task 9: Update addNode in NodeRunner.jsx to embed schema

**Files:**
- Modify: `wails-app/frontend/src/pages/NodeRunner.jsx`

**Context:** When a user adds a new node by clicking a node type in the palette, `addNode` is called. It needs to copy the schema from the node type definition into the new node object.

**Step 1: Find addNode**

Search for `addNode` in NodeRunner.jsx. It currently creates a node object with type/label/position but no schema.

**Step 2: Update addNode**

The node type catalog comes from `GetWorkflowNodeTypes()`. The returned objects now include a `schema` field. When constructing the new node, copy that schema:

```js
// In addNode (or wherever a new node is constructed from the palette):
const newNode = {
  id: `node-${Date.now()}`,
  node_type: nodeTypeDef.type,
  subtype: nodeTypeDef.type,
  name: nodeTypeDef.label,
  position_x: position.x,
  position_y: position.y,
  disabled: false,
  config: {},
  // Embed the schema from the node type definition so it travels with the workflow
  schema: nodeTypeDef.schema || { credential_platform: null, fields: [] },
  configFields: nodeTypeDef.schema?.fields || [],
}
```

**Step 3: Update workflow loading to read schema from node**

When loading a workflow (in the `useEffect` that processes workflow data), if a node has a `schema` field, prefer it over `NODE_CONFIG_FIELDS`:

```js
// When loading nodes from a saved workflow:
const processedNode = {
  ...rawNode,
  subtype: normalizeNodeType(rawNode.node_type || ''),
  // schema from file takes precedence; fall back to NODE_CONFIG_FIELDS
  schema: rawNode.schema || null,
  configFields: rawNode.schema?.fields
    ? rawNode.schema.fields
    : getConfigFields(normalizeNodeType(rawNode.node_type || '')),
}
```

**Step 4: Commit**
```bash
git add wails-app/frontend/src/pages/NodeRunner.jsx
git commit -m "feat: embed schema in new nodes from node type catalog; prefer schema over NODE_CONFIG_FIELDS on load"
```

---

### Task 10: Schema-driven Inspector rendering

**Files:**
- Modify: `wails-app/frontend/src/pages/NodeRunner.jsx` — Inspector component

**Context:** The Inspector currently renders fields using hardcoded `NODE_CONFIG_FIELDS`. It needs to render from `node.schema.fields` (or fall back to `node.configFields` for backward compat).

**Step 1: Find the Inspector component**

In NodeRunner.jsx, find the `Inspector` function/component. It likely has a pattern like:
```js
const fields = NODE_CONFIG_FIELDS[node.subtype] || []
```

**Step 2: Replace with schema-driven rendering**

```js
// Replace the hardcoded fields lookup with:
const fields = node.schema?.fields || node.configFields || []
```

The rest of the rendering loop (iterating `fields`, rendering inputs by `field.type`) stays the same — we're only changing the source of the field definitions.

**Step 3: Add rendering for new field types**

In the field rendering switch/if-else, ensure these types are handled (add if missing):

- `"boolean"` → checkbox or toggle
- `"textarea"` → `<textarea rows={field.rows || 3}>`
- `"number"` → `<input type="number" min={field.min} max={field.max}>`
- `"password"` → `<input type="password">`
- `"array"` → simple comma-separated text input for now (ResourcePickerField comes in Task 11)
- `"code"` → `<textarea>` with a code comment for now (CodeEditorField comes in Task 13)
- `"resource_picker"` → render a `<ResourcePickerField>` placeholder (full impl in Task 11)

**Step 4: Add DependsOn hide logic**

Before rendering each field, check if it should be hidden:
```js
function fieldIsVisible(field, config) {
  if (!field.depends_on) return true
  const depValue = String(config[field.depends_on.field] ?? '')
  return field.depends_on.values.includes(depValue)
}
```

Wrap each field render with: `{fieldIsVisible(field, node.config) && <FieldRenderer ... />}`

**Step 5: Add help text rendering**

After each field input, if `field.help` is set:
```jsx
{field.help && <p className="field-help">{field.help}</p>}
```

Add CSS: `.field-help { font-size: 12px; color: var(--text-muted); margin-top: 2px; }`

**Step 6: Commit**
```bash
git add wails-app/frontend/src/pages/NodeRunner.jsx
git commit -m "feat: schema-driven Inspector rendering with depends_on and help text"
```

---

### Task 11: ResourcePickerField component (compact + expanded)

**Files:**
- Create: `wails-app/frontend/src/components/ResourcePickerField.jsx`
- Modify: `wails-app/frontend/src/pages/NodeRunner.jsx` — import and use `ResourcePickerField`

**Step 1: Write ResourcePickerField**

Create `wails-app/frontend/src/components/ResourcePickerField.jsx`:
```jsx
import { useState, useEffect, useRef } from 'react'
import { ListResources, CreateResource } from '../wailsjs/go/main/App'

/**
 * ResourcePickerField renders a searchable dropdown for external resources
 * (spreadsheets, channels, etc.) with an expand button for the full browser.
 *
 * Props:
 *   field: NodeSchemaField with field.resource = { type, create_label, param_field }
 *   value: current value (resource ID)
 *   onChange: (newValue) => void
 *   credentialId: string — the credential_id from the node's config
 *   platform: string — e.g. "google_sheets"
 *   nodeConfig: object — full node config (for param_field resolution)
 */
export default function ResourcePickerField({ field, value, onChange, credentialId, platform, nodeConfig }) {
  const [items, setItems] = useState([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)
  const [query, setQuery] = useState('')
  const [expanded, setExpanded] = useState(false)
  const [creating, setCreating] = useState(false)
  const [newName, setNewName] = useState('')
  const [selectedLabel, setSelectedLabel] = useState('')
  const expandRef = useRef(null)

  const resourceType = field.resource?.type || ''

  // Load resources when expanded or query changes
  useEffect(() => {
    if (!expanded && items.length > 0) return
    if (!credentialId || !platform) return
    setLoading(true)
    setError(null)
    ListResources(platform, resourceType, credentialId, query)
      .then(result => {
        if (result.error) setError(result.error)
        else setItems(result.items || [])
      })
      .catch(e => setError(String(e)))
      .finally(() => setLoading(false))
  }, [expanded, query, credentialId, platform, resourceType])

  // Resolve selected item label
  useEffect(() => {
    if (value && items.length > 0) {
      const found = items.find(i => i.id === value)
      if (found) setSelectedLabel(found.name)
    }
  }, [value, items])

  function selectItem(item) {
    onChange(item.id)
    setSelectedLabel(item.name)
    setExpanded(false)
  }

  async function handleCreate() {
    if (!newName.trim()) return
    setLoading(true)
    try {
      const result = await CreateResource(platform, resourceType, credentialId, newName.trim())
      if (result.error) { setError(result.error); return }
      if (result.item) {
        setItems(prev => [result.item, ...prev])
        selectItem(result.item)
        setCreating(false)
        setNewName('')
      }
    } catch(e) { setError(String(e)) }
    finally { setLoading(false) }
  }

  const displayValue = selectedLabel || value || ''

  return (
    <div className="resource-picker" ref={expandRef}>
      {/* Compact row */}
      <div className="resource-picker-compact">
        <input
          type="text"
          className="resource-picker-search"
          placeholder={`Search ${resourceType}...`}
          value={query}
          onChange={e => { setQuery(e.target.value); setExpanded(true) }}
          onFocus={() => setExpanded(true)}
        />
        {displayValue && <span className="resource-picker-selected">{displayValue}</span>}
        <button
          type="button"
          className="resource-picker-expand-btn"
          title="Browse all"
          onClick={() => setExpanded(e => !e)}
        >⊞</button>
      </div>

      {/* Expanded browser */}
      {expanded && (
        <div className="resource-browser">
          <div className="resource-browser-header">
            <span>Select {field.label}</span>
            {field.resource?.create_label && (
              <button type="button" className="resource-create-btn" onClick={() => setCreating(c => !c)}>
                + {field.resource.create_label}
              </button>
            )}
          </div>

          {creating && (
            <div className="resource-create-row">
              <input
                type="text"
                placeholder="Name..."
                value={newName}
                onChange={e => setNewName(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleCreate()}
                autoFocus
              />
              <button type="button" onClick={handleCreate} disabled={loading}>Create</button>
              <button type="button" onClick={() => setCreating(false)}>Cancel</button>
            </div>
          )}

          {error && <div className="resource-error">{error}</div>}
          {loading && <div className="resource-loading">Loading...</div>}

          <ul className="resource-list">
            {items.map(item => (
              <li
                key={item.id}
                className={`resource-list-item ${item.id === value ? 'selected' : ''}`}
                onClick={() => selectItem(item)}
              >
                <span className="resource-item-name">{item.name}</span>
                {item.metadata?.modified_time && (
                  <span className="resource-item-meta">{item.metadata.modified_time}</span>
                )}
              </li>
            ))}
            {!loading && items.length === 0 && !error && (
              <li className="resource-empty">No results found</li>
            )}
          </ul>
        </div>
      )}
    </div>
  )
}
```

**Step 2: Add CSS**

In the main CSS file (or a new `ResourcePickerField.css` that you import), add:
```css
.resource-picker { position: relative; }
.resource-picker-compact { display: flex; align-items: center; gap: 4px; }
.resource-picker-search { flex: 1; }
.resource-picker-selected { font-size: 12px; color: var(--text-muted); max-width: 120px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.resource-picker-expand-btn { padding: 2px 6px; cursor: pointer; }
.resource-browser { position: absolute; z-index: 100; left: 0; right: 0; top: 100%; background: var(--bg-surface); border: 1px solid var(--border); border-radius: 6px; max-height: 280px; overflow-y: auto; box-shadow: 0 4px 16px rgba(0,0,0,0.2); }
.resource-browser-header { display: flex; justify-content: space-between; align-items: center; padding: 8px 12px; border-bottom: 1px solid var(--border); font-size: 13px; font-weight: 600; }
.resource-create-btn { font-size: 12px; cursor: pointer; }
.resource-create-row { display: flex; gap: 4px; padding: 6px 8px; border-bottom: 1px solid var(--border); }
.resource-create-row input { flex: 1; }
.resource-list { list-style: none; margin: 0; padding: 4px 0; }
.resource-list-item { display: flex; justify-content: space-between; padding: 6px 12px; cursor: pointer; font-size: 13px; }
.resource-list-item:hover, .resource-list-item.selected { background: var(--bg-hover); }
.resource-item-meta { font-size: 11px; color: var(--text-muted); }
.resource-error { padding: 8px 12px; color: var(--error); font-size: 12px; }
.resource-loading { padding: 8px 12px; color: var(--text-muted); font-size: 12px; }
.resource-empty { padding: 8px 12px; color: var(--text-muted); font-size: 12px; font-style: italic; }
```

**Step 3: Wire into Inspector**

In NodeRunner.jsx's Inspector field renderer, handle `resource_picker`:
```js
import ResourcePickerField from '../components/ResourcePickerField'

// In field renderer switch:
case 'resource_picker':
  return (
    <ResourcePickerField
      key={field.key}
      field={field}
      value={node.config[field.key] || ''}
      onChange={v => updateNodeConfig(node.id, field.key, v)}
      credentialId={node.config.credential_id || ''}
      platform={node.schema?.credential_platform || ''}
      nodeConfig={node.config}
    />
  )
```

**Step 4: Commit**
```bash
git add wails-app/frontend/src/components/ResourcePickerField.jsx wails-app/frontend/src/pages/NodeRunner.jsx
git commit -m "feat: ResourcePickerField component with search, list, and create"
```

---

### Task 12: DependsOnWrapper, ArrayField, CodeEditorField

**Files:**
- These are small additions to the Inspector field renderer in NodeRunner.jsx
- No new files needed — add inline

**Step 1: ArrayField (tag/chip input)**

In the Inspector field renderer, add case for `array`:
```jsx
case 'array': {
  const arrValue = Array.isArray(node.config[field.key])
    ? node.config[field.key]
    : (node.config[field.key] ? String(node.config[field.key]).split(',').map(s => s.trim()) : [])
  return (
    <div key={field.key} className="field-array">
      <label>{field.label}{field.required && ' *'}</label>
      <input
        type="text"
        placeholder="Type and press Enter..."
        onKeyDown={e => {
          if (e.key === 'Enter' && e.target.value.trim()) {
            const newArr = [...arrValue, e.target.value.trim()]
            updateNodeConfig(node.id, field.key, newArr)
            e.target.value = ''
          }
        }}
      />
      <div className="field-tags">
        {arrValue.map((tag, i) => (
          <span key={i} className="field-tag">
            {tag}
            <button type="button" onClick={() => {
              const newArr = arrValue.filter((_, idx) => idx !== i)
              updateNodeConfig(node.id, field.key, newArr)
            }}>×</button>
          </span>
        ))}
      </div>
      {field.help && <p className="field-help">{field.help}</p>}
    </div>
  )
}
```

**Step 2: CodeEditorField (syntax-highlighted textarea)**

For now, render as a `<textarea>` with a language badge. A full code editor can be wired later:
```jsx
case 'code':
  return (
    <div key={field.key} className="field-code-wrapper">
      <label>{field.label}{field.required && ' *'} <span className="code-lang">{field.language || 'text'}</span></label>
      <textarea
        className="field-code"
        rows={8}
        value={node.config[field.key] || ''}
        onChange={e => updateNodeConfig(node.id, field.key, e.target.value)}
        placeholder={field.placeholder || `Enter ${field.language || ''} code...`}
        spellCheck={false}
      />
      {field.help && <p className="field-help">{field.help}</p>}
    </div>
  )
```

Add CSS:
```css
.field-code { font-family: 'JetBrains Mono', 'Fira Code', monospace; font-size: 12px; resize: vertical; }
.code-lang { font-size: 10px; background: var(--bg-muted); padding: 1px 5px; border-radius: 3px; margin-left: 4px; text-transform: uppercase; }
.field-tags { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 4px; }
.field-tag { display: flex; align-items: center; gap: 2px; background: var(--bg-hover); padding: 2px 6px; border-radius: 12px; font-size: 12px; }
.field-tag button { background: none; border: none; cursor: pointer; font-size: 10px; color: var(--text-muted); }
```

**Step 3: Commit**
```bash
git add wails-app/frontend/src/pages/NodeRunner.jsx
git commit -m "feat: add ArrayField and CodeEditorField to Inspector"
```

---

### Task 13: End-to-end test

**Goal:** Verify the full system works: save a workflow as JSON, reload it with schema, see resource picker in Inspector.

**Step 1: Build the Wails app**
```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes/wails-app
make run
```

**Step 2: Migrate existing workflows**
```bash
./monoes workflow migrate
# Expected: workflows appear in ~/.monoes/workflows/
ls ~/.monoes/workflows/
```

**Step 3: Open the app and verify**
1. Open a workflow — nodes should load with `schema` field populated
2. Click a Google Sheets node — Inspector should show schema-driven fields including "Spreadsheet" resource_picker
3. Click the ⊞ expand button on the Spreadsheet field — ResourceBrowser should appear
4. If credential_id is set, spreadsheets should load from Google Drive API

**Step 4: Test adding a new node**
1. Open the node palette
2. Add a "Slack" node
3. Open Inspector — should show schema-driven fields with channel resource_picker

**Step 5: Test save/reload round-trip**
1. Configure a node's fields
2. Save the workflow (Cmd+S or Save button)
3. Reload the app
4. Re-open the workflow — field values should be preserved with schema intact

**Step 6: Run Go unit tests**
```bash
cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes
go test ./internal/workflow/... -v
# Expected: all tests pass
```

**Step 7: Commit**
```bash
git add .
git commit -m "feat: workflow JSON storage + schema-driven Inspector complete"
```

---

## Summary

| Task | Outcome |
|------|---------|
| 1–2 | 50 schema JSON files in `internal/workflow/schemas/` |
| 3 | `NodeSchema`, `LoadDefaultSchema`, `Schema` on `WorkflowNode` |
| 4 | `WorkflowFileStore` with full CRUD + tests |
| 5 | `monoes workflow migrate` CLI command |
| 6 | `app.go` workflow CRUD → file store |
| 7 | `ListResources` + `CreateResource` Wails functions |
| 8 | `GetWorkflowNodeTypes` returns schemas |
| 9 | New nodes embed schema from catalog |
| 10 | Inspector renders from `node.schema.fields` |
| 11 | `ResourcePickerField` component |
| 12 | `ArrayField`, `CodeEditorField` components |
| 13 | End-to-end verification |
