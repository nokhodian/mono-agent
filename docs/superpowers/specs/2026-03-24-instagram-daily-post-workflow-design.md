# Instagram Daily Post Workflow Design

**Goal:** A scheduled (and manually-triggerable) workflow that reads a Google Sheet, picks the first `todo` item, generates an AI image and caption via OpenRouter, posts to Instagram, and marks the row as `done`.

**Architecture:** A workflow JSON definition wires together existing nodes (`service.google_sheets`, `http.request`, `data.write_binary_file`, `instagram.publish_post`) with one new node executor (`service.openrouter`). The OpenRouter API key is stored as a connection. The workflow embeds a `trigger.schedule` node (cron `0 9 * * *`) for daily execution and can also be triggered manually via CLI.

**Tech Stack:** Go, go-rod, OpenRouter API (FLUX 1.1 Pro for images, Claude 3.5 Sonnet for captions), Google Sheets API (OAuth), existing workflow engine + cron scheduler.

---

## 1. Google Sheet Column Layout

Row 1 = headers. The workflow reads the first row where `status = todo`.

| Column | Name | Type | Description |
|--------|------|------|-------------|
| A | `title` | string | Post topic title |
| B | `description` | string | Detailed description for AI context |
| C | `image_prompt` | string (optional) | Explicit image prompt; AI builds one from title+description if empty |
| D | `caption` | string (optional) | Explicit Instagram caption; AI writes one if empty |
| E | `hashtags` | string | Space or comma-separated hashtags (e.g. `#art #daily`) |
| F | `tone` | string | Style hint: `inspirational`, `educational`, `humorous`, `aesthetic` |
| G | `scheduled_date` | string (YYYY-MM-DD, optional) | Informational target date, not used for row selection |
| H | `status` | string | `todo` or `done` |
| I | `posted_at` | string | Filled by system after posting (ISO 8601 timestamp) |
| J | `post_url` | string | Left empty (Instagram publish does not return a post URL — best-effort) |

---

## 2. New Component: `service.openrouter` Node Executor

### File
`internal/nodes/service/openrouter.go`

### Registration
Add to `internal/nodes/service/register_b.go` (following the existing pattern for other service nodes).

### Connection Type
Add `openrouter` to `internal/connections/registry.go`:
- Auth method: `api_key`
- Stores: `api_key` string
- Base URL: `https://openrouter.ai/api/v1`

### Schema File
`internal/workflow/schemas/service.openrouter.json`
(matches the flat `{node_type}.json` naming convention; embedded via `//go:embed schemas/*.json` in `schema_loader.go`)

### Operations

**`generate_image`**
- Input config:
  - `prompt` (string, required)
  - `model` (string, default: `black-forest-labs/flux-1.1-pro`)
  - `width` (int, default: 1024)
  - `height` (int, default: 1024)
- HTTP call: `POST https://openrouter.ai/api/v1/images/generations`
- Request body:
  ```json
  {
    "model": "black-forest-labs/flux-1.1-pro",
    "prompt": "...",
    "width": 1024,
    "height": 1024
  }
  ```
- Output: `{ "url": "https://..." }` — URL of generated image

**`generate_text`**
- Input config:
  - `prompt` (string, required)
  - `model` (string, default: `anthropic/claude-3.5-sonnet`)
  - `max_tokens` (int, default: 500)
- HTTP call: `POST https://openrouter.ai/api/v1/chat/completions`
- Request body:
  ```json
  {
    "model": "anthropic/claude-3.5-sonnet",
    "messages": [{"role": "user", "content": "..."}],
    "max_tokens": 500
  }
  ```
- Output: `{ "text": "..." }` — generated text content

---

## 3. Workflow Definition

### File
`tools/seed/instagram_daily_post.json`

This file is imported into the workflow engine via `monoes workflow import tools/seed/instagram_daily_post.json`. It is not auto-loaded from disk at runtime.

### Trigger
- **Cron**: embedded as a `trigger.schedule` node in the workflow JSON with `"cron": "0 9 * * *"` (9:00 AM daily)
- **Manual**: triggerable via `monoes workflow run instagram_daily_post`

### Connections Required (configured once in UI/CLI)
- `google_sheets_connection_id` — Google OAuth connection
- `openrouter_connection_id` — OpenRouter API key connection

### Node Graph (in order)

```
[trigger.schedule]
  → [service.google_sheets: read_rows]          — read all rows with use_header_row=true (adds _row_index)
  → [core.filter]                               — keep only rows where status == "todo"
  → [core.limit]                                — take first 1 item
  → [core.set: build_prompts]                   — build image_prompt_resolved and row_range string
  → [service.openrouter: generate_image]        — generate image via FLUX 1.1 Pro; downloads to /tmp; adds url + file_path to item
  → [service.openrouter: generate_text]         — generate caption; if caption column filled, prompt passes it through; adds text to item
  → [core.set: build_post_data]                 — build media array [{"url": file_path}] and post_text (caption + hashtags)
  → [instagram.publish_post]                    — post image + text
  → [service.google_sheets: update_rows]        — set status=done, posted_at=now (post_url left empty)
```

**Design note:** `service.openrouter` generates and downloads the image in one step (no separate `http.request`/`data.write_binary_file` nodes). Caption branching is handled via Go template conditionals in the prompt string, avoiding a `core.if` node.

---

## 4. Data Flow Detail

```
service.google_sheets (read_rows)
  operation: "read_rows"
  spreadsheetId: "{{config.spreadsheet_id}}"
  range: "Sheet1!A:J"
  output: rows[]  (array of row arrays)

core.set (extract_row)
  find first row where row[7] == "todo"  (column H, index 7)
  title             = row[0]
  description       = row[1]
  image_prompt_col  = row[2]
  caption_col       = row[3]
  hashtags          = row[4]
  tone              = row[5]
  row_index         = index of matched row (1-based, accounting for header row)
  row_range         = "Sheet1!H{row_index+1}:J{row_index+1}"
  image_prompt_resolved = image_prompt_col || "{{title}}. {{description}}. Photorealistic, high quality, social media post."

service.openrouter (generate_image)
  operation: "generate_image"
  prompt: image_prompt_resolved
  model: "black-forest-labs/flux-1.1-pro"
  width: 1024
  height: 1024
  output: { url: "https://..." }

http.request (download_image)
  method: GET
  url: generate_image.url
  responseType: binary
  output: binary image data

data.write_binary_file (save_image)
  data: download_image.data
  path: "/tmp/monoes_post_{{timestamp}}.jpg"
  output: { file_path: "/tmp/monoes_post_1234567890.jpg" }

core.if (caption_filled)
  condition: caption_col != ""
  true branch  → core.set: final_caption = caption_col
  false branch → service.openrouter.generate_text:
    operation: "generate_text"
    prompt: "Write an engaging Instagram caption for a post about: {{title}}. Context: {{description}}. Tone: {{tone}}. Keep it under 150 words. Do not include hashtags."
    output: { text: "..." }
    then core.set: final_caption = generate_text.text

core.set (build_media_input)
  media = [{"url": save_image.file_path}]

instagram.publish_post
  input:
    media: [{"url": "/tmp/monoes_post_1234567890.jpg"}]
    text: "{{final_caption}}\n\n{{hashtags}}"
  note: instagram.publish_post returns no post URL (outputs.success is empty)

service.google_sheets (update_rows)
  operation: "update_rows"
  spreadsheetId: "{{config.spreadsheet_id}}"
  range: extract_row.row_range          — e.g. "Sheet1!H3:J3"
  values: [["done", "{{now()}}", ""]]   — status=done, posted_at=now, post_url=empty
```

---

## 5. Error Handling

- **No todo rows found**: `core.if` on row existence exits gracefully with a log message — no error, no action
- **Image generation fails**: workflow stops, row stays `todo`, error logged
- **Instagram post fails**: workflow stops, row stays `todo`, temp image cleaned up
- **Sheet update fails**: post was made but sheet not updated — log warning with timestamp so user can manually update

---

## 6. Files to Create / Modify

| Action | File |
|--------|------|
| Create | `internal/nodes/service/openrouter.go` |
| Modify | `internal/connections/registry.go` — add `openrouter` connection type |
| Modify | `internal/connections/registry_test.go` — update count 27→28 |
| Modify | `internal/nodes/service/register_b.go` — register `service.openrouter` executor |
| Create | `internal/workflow/schemas/service.openrouter.json` — UI schema for node inspector |
| Modify | `internal/nodes/service/google_sheets.go` — add `_row_index` to `sheetsValuesToItems` |
| Create | `internal/nodes/service/google_sheets_test.go` — test for `_row_index` |
| Create | `tools/seed/instagram_daily_post.json` — workflow definition (imported via CLI) |

---

## 7. Out of Scope

- Multi-image posts (single image only)
- Video or story posts
- Retry logic for image generation (one attempt per run)
- Multiple Instagram accounts
- Automatic post URL capture (Instagram publish returns no URL)
- Sheet or tab selection via UI (spreadsheet ID configured statically in workflow JSON)
