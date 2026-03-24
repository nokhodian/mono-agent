# Instagram Daily Post Workflow Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a scheduled workflow that reads a Google Sheet, picks the first `todo` row, generates an AI image (FLUX 1.1 Pro) and caption (Claude 3.5 Sonnet) via OpenRouter, posts to Instagram, and marks the row as `done`.

**Architecture:** One new `service.openrouter` node executor (Go) handles both image generation + download and text generation; it enriches the input item rather than replacing it. A seed workflow JSON wires together existing nodes (`service.google_sheets`, `core.filter`, `core.limit`, `core.set`, `instagram.publish_post`) with the new `service.openrouter` node. A small modification to `sheetsValuesToItems` adds `_row_index` so the update range can be built.

**Tech Stack:** Go, `net/http`, OpenRouter API (FLUX 1.1 Pro + Claude 3.5 Sonnet), existing `workflow.NodeExecutor` interface, Go text/template expressions (`$json.fieldName`).

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `internal/connections/registry.go` | Add `openrouter` API-key connection type |
| Modify | `internal/connections/registry_test.go` | Update platform count 27→28, add `openrouter` to list |
| Create | `internal/nodes/service/openrouter.go` | `service.openrouter` node with `generate_image` + `generate_text` |
| Modify | `internal/nodes/service/register_b.go` | Register `service.openrouter` |
| Create | `internal/workflow/schemas/service.openrouter.json` | UI schema for node inspector |
| Modify | `internal/nodes/service/google_sheets.go` | Add `_row_index` field to items in `sheetsValuesToItems` |
| Create | `tools/seed/instagram_daily_post.json` | Full workflow seed file (import via CLI) |

---

## Task 1: Add OpenRouter connection type

**Files:**
- Modify: `internal/connections/registry.go`
- Modify: `internal/connections/registry_test.go`

- [ ] **Step 1: Write the failing test**

Open `internal/connections/registry_test.go`. The existing test asserts exactly 27 platforms. Add `"openrouter"` to `allExpectedIDs` and update the counts.

```go
// In allExpectedIDs, add to the "service" group:
"openrouter",

// Update the count assertion:
if len(allExpectedIDs) != 28 {
    t.Fatalf("test setup error: expected 28 IDs, got %d", len(allExpectedIDs))
}
// ...and:
if got := len(Registry); got != 28 {
    t.Errorf("Registry has %d platforms, want 28", got)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /path/to/newmonoes
go test ./internal/connections/... -run TestRegistryHasAllExpectedPlatforms -v
```

Expected: FAIL — `Registry missing platform "openrouter"` and count mismatch.

- [ ] **Step 3: Add openrouter to registry**

In `internal/connections/registry.go`, add this entry in the `// ─── Services ───` section, after `"google_drive"`:

```go
"openrouter": {
    ID:         "openrouter",
    Name:       "OpenRouter",
    Category:   "service",
    ConnectVia: "API",
    Methods:    []AuthMethod{MethodAPIKey},
    Fields: map[AuthMethod][]CredentialField{
        MethodAPIKey: {
            {
                Key:      "api_key",
                Label:    "API Key",
                Secret:   true,
                Required: true,
                HelpText: "Your OpenRouter API key. Find it at openrouter.ai/keys.",
            },
        },
    },
    IconEmoji: "🤖",
},
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/connections/... -v
```

Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/connections/registry.go internal/connections/registry_test.go
git commit -m "feat: add openrouter connection type to registry"
```

---

## Task 2: Implement `service.openrouter` node

**Files:**
- Create: `internal/nodes/service/openrouter.go`

The node enriches each input item — it adds result fields (`url` + `file_path` for images, `text` for text) to the existing item's JSON rather than replacing items. This preserves pipeline context.

- [ ] **Step 1: Create the file with tests first**

There are no unit test files in `internal/nodes/service/`, but you must verify behavior manually. Write the implementation and then test via the CLI in Step 4.

- [ ] **Step 2: Create `internal/nodes/service/openrouter.go`**

Note: use the existing `apiRequest` helper from `helpers.go` (same package) for JSON API calls.
For the image download (binary response, not JSON), use a plain `http.Get`.

```go
package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/monoes/monoes-agent/internal/workflow"
)

// OpenRouterNode implements the service.openrouter node type.
// It supports two operations:
//   - generate_image: calls FLUX 1.1 Pro, downloads the result to /tmp, adds url + file_path to item
//   - generate_text: calls a chat model, adds text to item
//
// Both operations ENRICH the input item (add fields) rather than replacing it.
type OpenRouterNode struct{}

func (n *OpenRouterNode) Type() string { return "service.openrouter" }

func (n *OpenRouterNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	apiKey := strVal(config, "api_key")
	if apiKey == "" {
		return nil, fmt.Errorf("openrouter: api_key is required")
	}

	operation := strVal(config, "operation")
	if operation == "" {
		operation = "generate_text"
	}

	// If no items are flowing in, create one empty item so the node can act as a source.
	items := input.Items
	if len(items) == 0 {
		items = []workflow.Item{workflow.NewItem(make(map[string]interface{}))}
	}

	var outputItems []workflow.Item
	for _, item := range items {
		var enriched workflow.Item
		var err error
		switch operation {
		case "generate_image":
			enriched, err = n.generateImage(ctx, apiKey, config, item)
		case "generate_text":
			enriched, err = n.generateText(ctx, apiKey, config, item)
		default:
			return nil, fmt.Errorf("openrouter: unknown operation %q", operation)
		}
		if err != nil {
			return nil, err
		}
		outputItems = append(outputItems, enriched)
	}

	return []workflow.NodeOutput{{Handle: "main", Items: outputItems}}, nil
}

// generateImage calls the OpenRouter image generation API (FLUX 1.1 Pro by default),
// downloads the generated image to a temp file, and adds "url" and "file_path" to the item.
func (n *OpenRouterNode) generateImage(ctx context.Context, apiKey string, config map[string]interface{}, item workflow.Item) (workflow.Item, error) {
	prompt := strVal(config, "prompt")
	if prompt == "" {
		return item, fmt.Errorf("openrouter generate_image: prompt is required")
	}
	model := strVal(config, "model")
	if model == "" {
		model = "black-forest-labs/flux-1.1-pro"
	}
	width := intVal(config, "width")
	if width == 0 {
		width = 1024
	}
	height := intVal(config, "height")
	if height == 0 {
		height = 1024
	}

	reqBody := map[string]interface{}{
		"model":  model,
		"prompt": prompt,
		"width":  width,
		"height": height,
	}

	// Use the shared apiRequest helper (same package, sets Authorization + Content-Type headers).
	data, err := apiRequest(ctx, http.MethodPost, "https://openrouter.ai/api/v1/images/generations", apiKey, reqBody)
	if err != nil {
		return item, fmt.Errorf("openrouter generate_image: %w", err)
	}

	imageURL := ""
	if imageData, ok := data["data"].([]interface{}); ok && len(imageData) > 0 {
		if img, ok := imageData[0].(map[string]interface{}); ok {
			imageURL, _ = img["url"].(string)
		}
	}
	if imageURL == "" {
		return item, fmt.Errorf("openrouter generate_image: no image URL in response: %v", data)
	}

	// Download the image to a temp file (binary download — cannot use apiRequest which returns JSON).
	filePath, err := downloadImageToTemp(ctx, imageURL)
	if err != nil {
		return item, fmt.Errorf("openrouter generate_image: download failed: %w", err)
	}

	// Enrich the item with image URL and local file path.
	enriched := copyItem(item)
	enriched.JSON["url"] = imageURL
	enriched.JSON["file_path"] = filePath
	return enriched, nil
}

// generateText calls the OpenRouter chat completions API and adds "text" to the item.
func (n *OpenRouterNode) generateText(ctx context.Context, apiKey string, config map[string]interface{}, item workflow.Item) (workflow.Item, error) {
	prompt := strVal(config, "prompt")
	if prompt == "" {
		return item, fmt.Errorf("openrouter generate_text: prompt is required")
	}
	model := strVal(config, "model")
	if model == "" {
		model = "anthropic/claude-3.5-sonnet"
	}
	maxTokens := intVal(config, "max_tokens")
	if maxTokens == 0 {
		maxTokens = 500
	}

	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]interface{}{
			{"role": "user", "content": prompt},
		},
		"max_tokens": maxTokens,
	}

	// Use the shared apiRequest helper (same package).
	data, err := apiRequest(ctx, http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", apiKey, reqBody)
	if err != nil {
		return item, fmt.Errorf("openrouter generate_text: %w", err)
	}

	text := ""
	if choices, ok := data["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if msg, ok := choice["message"].(map[string]interface{}); ok {
				text, _ = msg["content"].(string)
			}
		}
	}

	// Enrich the item.
	enriched := copyItem(item)
	enriched.JSON["text"] = text
	return enriched, nil
}

// downloadImageToTemp downloads a binary image URL to /tmp and returns the local file path.
// Uses plain http.Get — NOT apiRequest (which parses JSON responses).
func downloadImageToTemp(ctx context.Context, imageURL string) (string, error) {
	filePath := fmt.Sprintf("/tmp/monoes_post_%d.jpg", time.Now().UnixNano())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("download image HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read image body: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return "", fmt.Errorf("write image file: %w", err)
	}
	return filePath, nil
}

// copyItem returns a shallow copy of an Item with a new JSON map containing all original fields.
func copyItem(item workflow.Item) workflow.Item {
	newJSON := make(map[string]interface{}, len(item.JSON)+2)
	for k, v := range item.JSON {
		newJSON[k] = v
	}
	return workflow.Item{JSON: newJSON, Binary: item.Binary}
}
```

- [ ] **Step 3: Verify it compiles**

```bash
go build ./internal/nodes/service/...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/nodes/service/openrouter.go
git commit -m "feat: add service.openrouter node with generate_image and generate_text"
```

---

## Task 3: Register `service.openrouter` + add schema

**Files:**
- Modify: `internal/nodes/service/register_b.go`
- Create: `internal/workflow/schemas/service.openrouter.json`

- [ ] **Step 1: Add to register_b.go**

Add one line to `RegisterGroupB` in `internal/nodes/service/register_b.go`:

```go
r.Register("service.openrouter", func() workflow.NodeExecutor { return &OpenRouterNode{} })
```

Final file should look like:

```go
package service

import "github.com/monoes/monoes-agent/internal/workflow"

func RegisterGroupB(r *workflow.NodeTypeRegistry) {
	r.Register("service.stripe", func() workflow.NodeExecutor { return &StripeNode{} })
	r.Register("service.shopify", func() workflow.NodeExecutor { return &ShopifyNode{} })
	r.Register("service.salesforce", func() workflow.NodeExecutor { return &SalesforceNode{} })
	r.Register("service.hubspot", func() workflow.NodeExecutor { return &HubSpotNode{} })
	r.Register("service.google_sheets", func() workflow.NodeExecutor { return &GoogleSheetsNode{} })
	r.Register("service.gmail", func() workflow.NodeExecutor { return &GmailNode{} })
	r.Register("service.google_drive", func() workflow.NodeExecutor { return &GoogleDriveNode{} })
	r.Register("service.openrouter", func() workflow.NodeExecutor { return &OpenRouterNode{} })
}
```

- [ ] **Step 2: Create `internal/workflow/schemas/service.openrouter.json`**

```json
{
  "credential_platform": "openrouter",
  "fields": [
    {
      "key": "credential_id",
      "label": "OpenRouter Account",
      "type": "text",
      "required": true,
      "help": "Select your OpenRouter API key connection."
    },
    {
      "key": "operation",
      "label": "Operation",
      "type": "select",
      "required": true,
      "options": ["generate_text", "generate_image"],
      "default": "generate_text"
    },
    {
      "key": "prompt",
      "label": "Prompt",
      "type": "text",
      "required": true,
      "help": "The prompt text. Supports {{ $json.fieldName }} expressions."
    },
    {
      "key": "model",
      "label": "Model",
      "type": "text",
      "required": false,
      "help": "Default: anthropic/claude-3.5-sonnet for generate_text, black-forest-labs/flux-1.1-pro for generate_image."
    },
    {
      "key": "max_tokens",
      "label": "Max Tokens",
      "type": "number",
      "required": false,
      "default": 500,
      "depends_on": { "key": "operation", "values": ["generate_text"] }
    },
    {
      "key": "width",
      "label": "Width (px)",
      "type": "number",
      "required": false,
      "default": 1024,
      "depends_on": { "key": "operation", "values": ["generate_image"] }
    },
    {
      "key": "height",
      "label": "Height (px)",
      "type": "number",
      "required": false,
      "default": 1024,
      "depends_on": { "key": "operation", "values": ["generate_image"] }
    }
  ]
}
```

- [ ] **Step 3: Verify build + schema file is embedded**

```bash
go build ./...
go test ./internal/workflow/... -v 2>&1 | tail -5
```

Expected: compiles without errors. The schema is embedded via `//go:embed schemas/*.json` — any `.json` file in that directory is automatically included.

- [ ] **Step 3b: Verify core.filter and core.limit are registered**

```bash
grep -r "core.filter\|core.limit" /path/to/newmonoes/internal/nodes/control/register.go
```

Expected: both `core.filter` and `core.limit` appear. If either is missing, the seed workflow will fail to execute — add the missing registration to `internal/nodes/control/register.go` following the same pattern as other control nodes.

- [ ] **Step 4: Commit**

```bash
git add internal/nodes/service/register_b.go internal/workflow/schemas/service.openrouter.json
git commit -m "feat: register service.openrouter node and add UI schema"
```

---

## Task 4: Add `_row_index` to Google Sheets items

**Files:**
- Modify: `internal/nodes/service/google_sheets.go` (function `sheetsValuesToItems`, lines 242–279)

This small change adds a `_row_index` field (1-based sheet row number) to each item, enabling the workflow to build the update range string (`Sheet1!H3:J3`).

- [ ] **Step 1: Write a test first**

There are no existing tests for `sheetsValuesToItems`. Create a quick inline test to verify the behavior before and after. Add this to a new file `internal/nodes/service/google_sheets_test.go`:

```go
package service

import (
	"testing"
)

func TestSheetsValuesToItems_RowIndex(t *testing.T) {
	values := []interface{}{
		[]interface{}{"title", "status"},    // header row
		[]interface{}{"Post 1", "todo"},     // data row 1 → _row_index = 2
		[]interface{}{"Post 2", "done"},     // data row 2 → _row_index = 3
	}

	items := sheetsValuesToItems(values, true)

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	if got := items[0].JSON["_row_index"]; got != 2 {
		t.Errorf("items[0]._row_index = %v, want 2", got)
	}
	if got := items[1].JSON["_row_index"]; got != 3 {
		t.Errorf("items[1]._row_index = %v, want 3", got)
	}

	// Without header row: first data row = sheet row 1
	itemsNoHeader := sheetsValuesToItems(values, false)
	if got := itemsNoHeader[0].JSON["_row_index"]; got != 1 {
		t.Errorf("no-header items[0]._row_index = %v, want 1", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/nodes/service/... -run TestSheetsValuesToItems_RowIndex -v
```

Expected: FAIL — `_row_index` field doesn't exist yet.

- [ ] **Step 3: Modify `sheetsValuesToItems` to add `_row_index`**

In `internal/nodes/service/google_sheets.go`, find the `sheetsValuesToItems` function (around line 242). Change the loop body to add `_row_index`:

```go
// Before (in the for loop body):
data := make(map[string]interface{}, len(row))
for j, cell := range row {
    var key string
    if useHeaderRow && j < len(headers) {
        key = headers[j]
    } else {
        key = sheetsColumnLetter(j)
    }
    data[key] = cell
}
items = append(items, workflow.NewItem(data))

// After:
data := make(map[string]interface{}, len(row)+1)
for j, cell := range row {
    var key string
    if useHeaderRow && j < len(headers) {
        key = headers[j]
    } else {
        key = sheetsColumnLetter(j)
    }
    data[key] = cell
}
data["_row_index"] = i + 1  // 1-based sheet row (row 1 = headers when useHeaderRow=true)
items = append(items, workflow.NewItem(data))
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/nodes/service/... -run TestSheetsValuesToItems_RowIndex -v
```

Expected: PASS.

- [ ] **Step 5: Run full build**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add internal/nodes/service/google_sheets.go internal/nodes/service/google_sheets_test.go
git commit -m "feat: add _row_index field to google_sheets read_rows items"
```

---

## Task 5: Create workflow seed file

**Files:**
- Create: `tools/seed/instagram_daily_post.json`

This is the complete workflow definition. Import it with:
```bash
mkdir -p tools/seed
monoes workflow import --file tools/seed/instagram_daily_post.json
```

The workflow uses these expression conventions (from `expression.go`):
- `$json.fieldName` — access current item's field
- `$node["NodeName"].json.fieldName` — access a named node's first-item output field

**Important:** After importing, edit the workflow in the UI to set:
- `REPLACE_WITH_GOOGLE_CREDENTIAL_ID` → your Google Sheets credential ID
- `REPLACE_WITH_SPREADSHEET_ID` → your Google Sheet ID (from the URL)
- `REPLACE_WITH_OPENROUTER_CREDENTIAL_ID` → your OpenRouter credential ID (with key `sk-or-v1-a384cfd4...`)
- `REPLACE_WITH_INSTAGRAM_CREDENTIAL_ID` → your Instagram browser session credential ID

- [ ] **Step 1: Create `tools/seed/` directory and workflow JSON**

```bash
mkdir -p tools/seed
```

Create `tools/seed/instagram_daily_post.json`:

```json
{
  "id": "instagram-daily-post-v1",
  "name": "Instagram Daily Post",
  "description": "Daily: reads first todo row from Google Sheet, generates AI image + caption via OpenRouter, posts to Instagram, marks row done.",
  "version": 1,
  "is_active": true,
  "nodes": [
    {
      "id": "n1",
      "type": "trigger.schedule",
      "name": "Daily 9am",
      "position": { "x": 100, "y": 200 },
      "config": {
        "cron": "0 9 * * *"
      }
    },
    {
      "id": "n2",
      "type": "service.google_sheets",
      "name": "Read Sheet",
      "position": { "x": 350, "y": 200 },
      "config": {
        "credential_id": "REPLACE_WITH_GOOGLE_CREDENTIAL_ID",
        "spreadsheet_id": "REPLACE_WITH_SPREADSHEET_ID",
        "sheet_name": "Sheet1",
        "operation": "read_rows",
        "range": "A:J",
        "use_header_row": true
      }
    },
    {
      "id": "n3",
      "type": "core.filter",
      "name": "Filter Todo",
      "position": { "x": 600, "y": 200 },
      "config": {
        "condition": "{{ eq $json.status \"todo\" }}"
      }
    },
    {
      "id": "n4",
      "type": "core.limit",
      "name": "First Item",
      "position": { "x": 850, "y": 200 },
      "config": {
        "max_items": 1
      }
    },
    {
      "id": "n5",
      "type": "core.set",
      "name": "Build Prompts",
      "position": { "x": 1100, "y": 200 },
      "config": {
        "assignments": [
          {
            "field": "image_prompt_resolved",
            "value": "{{ if $json.image_prompt }}{{ $json.image_prompt }}{{ else }}{{ $json.title }}. {{ $json.description }}. Photorealistic, high quality, social media post.{{ end }}",
            "type": "string"
          },
          {
            "field": "row_range",
            "value": "Sheet1!H{{ $json._row_index }}:J{{ $json._row_index }}",
            "type": "string"
          }
        ]
      }
    },
    {
      "id": "n6",
      "type": "service.openrouter",
      "name": "Generate Image",
      "position": { "x": 1350, "y": 200 },
      "config": {
        "credential_id": "REPLACE_WITH_OPENROUTER_CREDENTIAL_ID",
        "operation": "generate_image",
        "prompt": "{{ $json.image_prompt_resolved }}",
        "model": "black-forest-labs/flux-1.1-pro",
        "width": 1024,
        "height": 1024
      }
    },
    {
      "id": "n7",
      "type": "service.openrouter",
      "name": "Generate Caption",
      "position": { "x": 1600, "y": 200 },
      "config": {
        "credential_id": "REPLACE_WITH_OPENROUTER_CREDENTIAL_ID",
        "operation": "generate_text",
        "prompt": "{{ if $json.caption }}{{ $json.caption }}{{ else }}Write an engaging Instagram caption for a post about: {{ $json.title }}. Context: {{ $json.description }}. Tone: {{ default \"inspirational\" $json.tone }}. Keep it under 150 words. Do not include hashtags.{{ end }}",
        "model": "anthropic/claude-3.5-sonnet",
        "max_tokens": 500
      }
    },
    {
      "id": "n8",
      "type": "core.set",
      "name": "Build Post Data",
      "position": { "x": 1850, "y": 200 },
      "config": {
        "assignments": [
          {
            "field": "media",
            "value": "[{\"url\": \"{{ $json.file_path }}\"}]",
            "type": "json"
          },
          {
            "field": "post_text",
            "value": "{{ $json.text }}\n\n{{ $json.hashtags }}",
            "type": "string"
          }
        ]
      }
    },
    {
      "id": "n9",
      "type": "instagram.publish_post",
      "name": "Post to Instagram",
      "position": { "x": 2100, "y": 200 },
      "config": {
        "credential_id": "REPLACE_WITH_INSTAGRAM_CREDENTIAL_ID",
        "media": "{{ $json.media }}",
        "text": "{{ $json.post_text }}"
      }
    },
    {
      "id": "n10",
      "type": "service.google_sheets",
      "name": "Mark Done",
      "position": { "x": 2350, "y": 200 },
      "config": {
        "credential_id": "REPLACE_WITH_GOOGLE_CREDENTIAL_ID",
        "spreadsheet_id": "REPLACE_WITH_SPREADSHEET_ID",
        "operation": "update_rows",
        "range": "{{ $json.row_range }}",
        "values": [["done", "{{ now }}", ""]]
      }
    }
  ],
  "connections": [
    { "id": "e1", "source": "n1", "source_handle": "main", "target": "n2", "target_handle": "main" },
    { "id": "e2", "source": "n2", "source_handle": "main", "target": "n3", "target_handle": "main" },
    { "id": "e3", "source": "n3", "source_handle": "main", "target": "n4", "target_handle": "main" },
    { "id": "e4", "source": "n4", "source_handle": "main", "target": "n5", "target_handle": "main" },
    { "id": "e5", "source": "n5", "source_handle": "main", "target": "n6", "target_handle": "main" },
    { "id": "e6", "source": "n6", "source_handle": "main", "target": "n7", "target_handle": "main" },
    { "id": "e7", "source": "n7", "source_handle": "main", "target": "n8", "target_handle": "main" },
    { "id": "e8", "source": "n8", "source_handle": "main", "target": "n9", "target_handle": "main" },
    { "id": "e9", "source": "n9", "source_handle": "main", "target": "n10", "target_handle": "main" }
  ]
}
```

- [ ] **Step 2: Verify JSON is valid**

```bash
python3 -m json.tool tools/seed/instagram_daily_post.json > /dev/null && echo "JSON valid"
```

Expected: `JSON valid`

- [ ] **Step 3: Build the binary and do a dry-run import**

```bash
go build -o /tmp/monoes ./cmd/monoes/
/tmp/monoes workflow import --file tools/seed/instagram_daily_post.json
```

Expected: `Imported workflow "Instagram Daily Post" as id: <uuid>  (10 nodes, 9 connections)`

- [ ] **Step 4: Verify the workflow is listed**

```bash
/tmp/monoes workflow list
```

Expected: `Instagram Daily Post` appears in the list.

- [ ] **Step 5: Commit**

```bash
git add tools/seed/instagram_daily_post.json
git commit -m "feat: add Instagram daily post workflow seed file"
```

---

## Setup Instructions (after implementation)

After importing the workflow, configure credentials:

1. **OpenRouter connection** — in the UI, add a new OpenRouter connection with your OpenRouter API key (get one at openrouter.ai/keys)
2. **Google Sheets connection** — authenticate via OAuth in the UI
3. **Instagram connection** — authenticate via browser session in the UI
4. **Edit workflow nodes** — replace all `REPLACE_WITH_*` placeholders in node configs with real credential IDs and spreadsheet ID
5. **Activate the workflow** — set `is_active: true` in the UI or via:

```bash
/tmp/monoes workflow activate <workflow-id>
```

6. **Test manual run**:

```bash
/tmp/monoes workflow run <workflow-id>
```

---

## Google Sheet Setup

Create a spreadsheet with these headers in row 1:

```
title | description | image_prompt | caption | hashtags | tone | scheduled_date | status | posted_at | post_url
```

Add a test row (row 2):
```
Test Post | A beautiful sunset over the ocean | | | #sunset #ocean #daily | inspirational | | todo | |
```
