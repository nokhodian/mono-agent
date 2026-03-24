# Instagram Daily Post Workflow Design

**Goal:** A scheduled (and manually-triggerable) workflow that reads a Google Sheet, picks the first `todo` item, generates an AI image and caption via OpenRouter, posts to Instagram, and marks the row as `done`.

**Architecture:** A workflow JSON definition wires together existing nodes (`service.google_sheets`, `http.request`, `data.write_binary_file`, `instagram.publish_post`) with one new node executor (`service.openrouter`). The OpenRouter API key is stored as a connection. The workflow is registered with the cron scheduler and can also be triggered manually.

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
| J | `post_url` | string | Filled by system after posting |

---

## 2. New Component: `service.openrouter` Node Executor

### File
`internal/nodes/service/openrouter.go`

### Connection Type
Add `openrouter` to `internal/connections/registry.go`:
- Auth method: `api_key`
- Stores: `api_key` string
- Base URL: `https://openrouter.ai/api/v1`

### Operations

**`generate_image`**
- Input config:
  - `prompt` (string, required)
  - `model` (string, default: `black-forest-labs/flux-1.1-pro`)
  - `width` (int, default: 1024)
  - `height` (int, default: 1024)
- HTTP call: `POST https://openrouter.ai/api/v1/images/generations`
- Request body: `{ "model": "...", "prompt": "...", "size": "1024x1024" }`
- Output: `{ "url": "https://..." }` — URL of generated image

**`generate_text`**
- Input config:
  - `prompt` (string, required)
  - `model` (string, default: `anthropic/claude-3.5-sonnet`)
  - `max_tokens` (int, default: 500)
- HTTP call: `POST https://openrouter.ai/api/v1/chat/completions`
- Request body: `{ "model": "...", "messages": [{"role": "user", "content": "..."}], "max_tokens": 500 }`
- Output: `{ "text": "..." }` — generated text content

---

## 3. Workflow Definition

### File
`data/workflows/instagram_daily_post.json`

### Trigger
- **Cron**: `0 9 * * *` (9:00 AM daily) — registered via `monoes schedule add`
- **Manual**: triggerable via CLI (`monoes workflow run instagram_daily_post`) or UI

### Connections Required (configured once)
- `google_sheets_connection_id` — Google OAuth connection
- `openrouter_connection_id` — OpenRouter API key connection

### Node Graph (in order)

```
[trigger.cron]
  → [service.google_sheets: read_range]        — read all rows, find first status=todo
  → [core.set: extract_row]                    — extract row data, build fallback prompts
  → [service.openrouter: generate_image]       — generate image from prompt
  → [http.request: download_image]             — GET image URL → binary
  → [data.write_binary_file: save_image]       — write to /tmp/monoes_post_{timestamp}.jpg
  → [core.if: caption_filled]                  — check if caption column is non-empty
       ├─ yes → [core.set: use_caption]        — pass through existing caption
       └─ no  → [service.openrouter: generate_text]  — generate caption via AI
  → [instagram.publish_post]                   — post image + caption + hashtags
  → [service.google_sheets: update_row]        — set status=done, posted_at, post_url
```

### Prompt Fallbacks

**Image prompt** (used when `image_prompt` column is empty):
```
{{title}}. {{description}}. Photorealistic, high quality, social media post.
```

**Caption generation prompt** (used when `caption` column is empty):
```
Write an engaging Instagram caption for a post about: {{title}}.
Context: {{description}}.
Tone: {{tone}}.
Keep it under 150 words. Do not include hashtags.
```

---

## 4. Data Flow Detail

```
google_sheets.read_range
  output: rows[]

core.set (extract first todo row)
  image_prompt_resolved = row.image_prompt || "{{row.title}}. {{row.description}}. Photorealistic, high quality."
  row_index = index of first todo row

service.openrouter.generate_image
  input: image_prompt_resolved
  output: { url: "https://cdn.openrouter.ai/..." }

http.request (GET image URL)
  output: binary image data

data.write_binary_file
  path: /tmp/monoes_post_{{timestamp}}.jpg
  output: { path: "/tmp/monoes_post_1234567890.jpg" }

core.if (caption_filled)
  condition: row.caption != ""

  [branch: caption filled]
  core.set: final_caption = row.caption

  [branch: caption empty]
  service.openrouter.generate_text
    prompt: "Write an engaging Instagram caption for: {{row.title}}. Context: {{row.description}}. Tone: {{row.tone}}. Under 150 words, no hashtags."
    output: { text: "..." }
  core.set: final_caption = generate_text.text

instagram.publish_post
  mediaPath: write_binary_file.path
  caption: "{{final_caption}}\n\n{{row.hashtags}}"
  output: { postUrl: "https://www.instagram.com/p/..." }

service.google_sheets.update_row
  row_index: extract_row.row_index
  updates: { status: "done", posted_at: now(), post_url: instagram.postUrl }
```

---

## 5. Error Handling

- **No todo rows found**: workflow exits gracefully with a log message — no error
- **Image generation fails**: workflow stops, row stays `todo`, error logged
- **Instagram post fails**: workflow stops, row stays `todo`, temp image cleaned up
- **Sheet update fails**: post was made but sheet not updated — log warning with post URL so user can manually update

---

## 6. Files to Create / Modify

| Action | File |
|--------|------|
| Create | `internal/nodes/service/openrouter.go` |
| Modify | `internal/connections/registry.go` — add `openrouter` connection type |
| Modify | `cmd/monoes/node.go` — register `service.openrouter` executor |
| Create | `data/workflows/instagram_daily_post.json` |
| Create | `data/schemas/service/openrouter.json` — UI schema for node inspector |

---

## 7. Out of Scope

- Multi-image posts (single image only)
- Video posts
- Story posts
- Retry logic for image generation (one attempt per run)
- Multiple Instagram accounts
- Sheet selection via UI (spreadsheet ID configured in workflow JSON)
