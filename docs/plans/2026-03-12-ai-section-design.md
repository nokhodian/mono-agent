# AI Section Design

## Goal

Add a first-class AI system to Mono Agent with three integrated components:
1. **AI Provider Registry** — connect and manage 50+ AI providers (API keys, models)
2. **AI Chat Panel** — per-workflow chat assistant that can read and write the workflow canvas in real-time
3. **AI Nodes** — six new workflow node types that call any configured AI provider mid-execution

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                    Mono Agent UI                       │
│                                                          │
│  ┌──────────────────┐    ┌──────────────────────────┐   │
│  │  AI Providers    │    │    Workflow Builder        │   │
│  │  (new page)      │    │  ┌────────────┐ ┌──────┐  │   │
│  │                  │    │  │  Canvas    │ │ Chat │  │   │
│  │  - Known tier    │    │  │  (DAG)     │ │Panel │  │   │
│  │  - Gateway tier  │    │  │            │ │      │  │   │
│  └──────────────────┘    │  └────────────┘ └──────┘  │   │
│                          └──────────────────────────┘   │
└──────────────────────────────────────────────────────────┘
                            │
                ┌───────────▼───────────┐
                │      Go Backend        │
                │                        │
                │  AI Provider Store     │
                │  AI Chat Service       │
                │  AI Node Executors     │
                │                        │
                │  Unified AI Client     │
                │  ┌────────────────┐    │
                │  │ OpenAI-compat  │    │  ← covers ~35 providers
                │  │ Anthropic      │    │
                │  │ Google Gemini  │    │
                │  │ AWS Bedrock    │    │
                │  └────────────────┘    │
                └───────────────────────┘
```

**Core principle:** Use the OpenAI-compatible `/v1/chat/completions` API as the universal interface. ~80% of providers support it natively. Thin adapters handle the rest (Anthropic Messages API, Google GenerateContent, AWS Bedrock SigV4).

---

## Component 1: AI Provider Registry

### Data Model

New SQLite table `ai_providers`:

```sql
CREATE TABLE IF NOT EXISTS ai_providers (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,         -- user label, e.g. "My OpenAI"
    provider_id   TEXT NOT NULL,         -- e.g. "openai", "anthropic", "openrouter"
    tier          TEXT NOT NULL,         -- "known" | "gateway"
    api_key       TEXT NOT NULL,
    base_url      TEXT,                  -- overrides default; required for gateway tier
    default_model TEXT,                  -- user's preferred model
    extra_headers TEXT,                  -- JSON: extra HTTP headers (Portkey routing, etc.)
    status        TEXT NOT NULL DEFAULT 'untested',  -- "active" | "error" | "untested"
    last_tested   TEXT,                  -- RFC3339
    created_at    TEXT NOT NULL
);
```

### Provider Registry (hardcoded, ~50 providers)

Each entry in `internal/ai/registry.go`:

```go
type ProviderDef struct {
    ID           string   // "openai", "anthropic", "openrouter"
    Name         string   // "OpenAI", "Anthropic", "OpenRouter"
    IconEmoji    string
    Category     string   // "frontier" | "cloud" | "inference" | "gateway"
    Tier         string   // "known" | "gateway"
    DefaultBaseURL string
    AuthLabel    string   // "API Key", "Access Key + Secret", etc.
    Models       []ModelDef
    Adapter      string   // "openai" | "anthropic" | "google" | "bedrock"
    DocsURL      string
}

type ModelDef struct {
    ID           string  // "gpt-4o", "claude-sonnet-4-6"
    Name         string  // display name
    ContextWindow int    // tokens
    Capabilities []string // "chat", "vision", "tools", "embed"
}
```

**Provider categories:**

| Category | Key Providers |
|---|---|
| Frontier | OpenAI, Anthropic, Google (Gemini), xAI (Grok), Mistral, Cohere, DeepSeek, Meta (Llama via API) |
| Cloud | AWS Bedrock, Azure AI Foundry, Google Vertex AI, IBM watsonx, Databricks, Snowflake Cortex, Cloudflare Workers AI |
| Inference | Groq, Cerebras, Together AI, Fireworks AI, DeepInfra, SambaNova, Replicate, Perplexity AI, RunPod, Anyscale, Lambda Labs, Novita AI, Hyperbolic |
| Gateway | OpenRouter, LiteLLM, Portkey, Helicone, Bifrost, Kong AI, Martian, Unify AI, Neutrino |
| Other Frontier | AI21 Labs, Aleph Alpha, Writer, Baidu (Ernie), Tencent (Hunyuan), Zhipu AI (GLM), Moonshot AI (Kimi), MiniMax, Alibaba (Qwen), OctoAI |

### UI — AI Providers Page

- New nav item "AI" (or sub-section under Connections)
- Same tile-grid pattern as Connections page
- Tile shows: provider emoji, name, status dot, connected model count
- **Add Provider modal:**
  - Step 1: Pick provider from grid (grouped by category) or search
  - Step 2: Enter API key (+ base URL for gateway tier)
  - Step 3: Pick default model from **combobox** (curated list + free-text for custom models)
  - Test Connection button → validates with a minimal API call
- **Manage modal** (click existing tile): update key, change default model, test, delete

### Model Combobox

Both tiers get a combobox — dropdown of curated models, but free-text allowed for custom model strings (e.g. `anthropic/claude-3-5-sonnet` on OpenRouter). Known models shown with context window size.

### Test Connection

- For OpenAI-compat providers: `GET /v1/models` or minimal `POST /v1/chat/completions` with `max_tokens: 1`
- For Anthropic: `POST /v1/messages` with `max_tokens: 1`
- For Google: `POST /v1beta/models/{model}:generateContent` with minimal prompt
- Returns provider name / account info on success

---

## Component 2: AI Chat Panel

### Placement

Collapsible right-side panel inside the Workflow Builder. Toggle button in workflow toolbar. Default width 340px, user-resizable. Canvas shrinks to accommodate.

### Per-Workflow Persistence

New SQLite table `ai_chat_messages`:

```sql
CREATE TABLE IF NOT EXISTS ai_chat_messages (
    id           TEXT PRIMARY KEY,
    workflow_id  TEXT NOT NULL,
    role         TEXT NOT NULL,   -- "user" | "assistant" | "tool"
    content      TEXT NOT NULL,   -- message text or tool result JSON
    tool_calls   TEXT,            -- JSON array of tool calls (if role=assistant)
    provider_id  TEXT,            -- which provider was used
    model        TEXT,
    token_count  INTEGER,
    created_at   TEXT NOT NULL
);
```

History loads when workflow opens, persists across app restarts. "Clear history" button in panel header.

### Chat Header

```
┌─ AI Assistant ──────────────────── [×] ┐
│ [Anthropic ▼] [claude-sonnet-4-6 ▼]   │
└────────────────────────────────────────┘
```

Provider + model dropdowns filter to only configured AI providers. Selection persists per workflow in `workflows` table (new `ai_provider_id`, `ai_model` columns).

### Streaming

Tokens stream from Go backend via Wails `EventsEmit("ai:chunk", {workflowID, token, done, toolCall?})`. Frontend accumulates tokens into the current assistant message bubble. Tool call results shown inline as collapsible cards.

### System Prompt (injected automatically)

```
You are an AI assistant embedded in Mono Agent, a workflow automation tool.
You help users build and modify automation workflows.

Current workflow: {name}
Description: {description}

Current nodes on canvas:
{node_list}

Available node types:
{node_catalog_summary}

User's connected platforms: {connections_list}

Last execution result: {last_execution_summary}

You have tools to read and modify the workflow canvas. Always call
get_workflow_state before making changes to understand current state.
When creating nodes, choose positions that don't overlap existing nodes.
```

### Canvas Tool Implementations

| Tool | Parameters | Effect |
|---|---|---|
| `get_workflow_state` | — | Returns nodes[], edges[], canvas bounds |
| `create_nodes` | nodes: [{type, name, config, position?}] | Adds nodes, auto-positions if no position given |
| `update_node_config` | nodeId, config | Merges config into existing node |
| `delete_nodes` | nodeIds[] | Removes nodes and their connected edges |
| `connect_nodes` | sourceId, sourceHandle, targetId, targetHandle | Draws edge |
| `disconnect_nodes` | sourceId, targetId | Removes edge |
| `list_available_nodes` | category? | Returns node catalog with type IDs + descriptions |
| `list_connections` | — | Returns user's configured platform connections |
| `run_workflow` | — | Triggers manual execution |
| `get_execution_result` | executionId? | Returns last (or specific) execution output |

Tools are implemented as Go functions called by the AI service when the model emits a tool call. Results are fed back to the model, which then continues and emits the final text response.

---

## Component 3: AI Nodes

New category **"AI"** in the workflow node palette.

### Node Types

**`ai.chat`** — General completion
```
Config: provider, model, system_prompt, prompt (template), temperature, max_tokens, output_key
Input:  Any items
Output: Items with ai_response field added
```

**`ai.extract`** — Structured extraction
```
Config: provider, model, prompt, output_schema (JSON Schema), output_key
Input:  Items with text content
Output: Items with structured JSON extracted per schema
```

**`ai.classify`** — Classification + routing
```
Config: provider, model, categories[], prompt_template
Input:  Items
Output: "main" handle (all items with category + confidence added)
        One output handle per category (for routing)
```

**`ai.transform`** — Rewrite/translate/reformat
```
Config: provider, model, instruction (template), input_field, output_key
Input:  Items
Output: Items with transformed field
```

**`ai.embed`** — Vector embeddings
```
Config: provider, model, input_field, output_key
Input:  Items with text
Output: Items with float[] vector added
```

**`ai.agent`** — Autonomous multi-step agent
```
Config: provider, model, goal (template), available_tools[], max_steps
Input:  Items (passed as context)
Output: Items with agent_result + steps_taken added
available_tools: subset of [http_request, code_execute, read_file, workflow_node]
```

### Per-Node Config Panel

```
┌─ AI Chat Node ──────────────────────────┐
│ Provider    [Anthropic        ▼]         │
│ Model       [claude-sonnet-4-6 ▼]       │
│                                          │
│ System msg  [You are a helpful...]       │
│                                          │
│ Prompt      ┌──────────────────────┐    │
│             │ Summarize this:      │    │
│             │ {{$json.content}}    │    │
│             └──────────────────────┘    │
│ Temperature [0.7 ──────●────────] 2.0  │
│ Max tokens  [1024        ]               │
│ Output key  [ai_response ]               │
└─────────────────────────────────────────┘
```

### Execution

- Items flow through the node in parallel (default concurrency: 3)
- Configurable concurrency limit per node to respect rate limits
- On error: follows workflow's existing error handling (stop / continue / route to error handle)
- Token usage logged to `workflow_execution_nodes` metadata

---

## Backend Package Structure

```
internal/ai/
├── registry.go          — Provider definitions (~50 providers, model lists)
├── store.go             — SQLite CRUD for ai_providers + ai_chat_messages
├── client.go            — Client interface + CompletionRequest/Response types
├── adapters/
│   ├── openai.go        — OpenAI-compat adapter (covers ~35 providers)
│   ├── anthropic.go     — Anthropic Messages API adapter
│   ├── google.go        — Google Gemini GenerateContent adapter
│   └── bedrock.go       — AWS Bedrock (SigV4, model routing)
├── chat/
│   ├── service.go       — Per-workflow chat: history, system prompt, tool dispatch, streaming
│   └── tools.go         — Canvas tool implementations (read/write workflow state)
└── nodes/
    ├── chat.go          — ai.chat node executor
    ├── extract.go       — ai.extract node executor
    ├── classify.go      — ai.classify node executor
    ├── transform.go     — ai.transform node executor
    ├── embed.go         — ai.embed node executor
    └── agent.go         — ai.agent node executor
```

### New Wails-bound functions in app.go

```go
// Provider management
ListAIProviders() string
SaveAIProvider(providerJSON string) string
DeleteAIProvider(id string) string
TestAIProvider(id string) string
GetAIModels(providerID string) string

// Chat
StreamAIChat(workflowID, message, providerID, model string) string
GetAIChatHistory(workflowID string) string
ClearAIChatHistory(workflowID string) string
```

### Events (Wails runtime)

```
ai:chunk   — {workflowID, token, done, toolCall?}   — streaming token
ai:tool    — {workflowID, tool, args, result}        — tool call result
ai:error   — {workflowID, error}                     — stream error
```

---

## Data Flow: Chat-to-Canvas

```
User types message
      │
      ▼
StreamAIChat() called
      │
      ▼
Chat service builds messages array:
  [system_prompt + canvas_state, history, new_user_message]
      │
      ▼
AI client streams response
      │
      ├─ text token → EventsEmit("ai:chunk") → frontend appends to bubble
      │
      └─ tool_call → tool dispatcher
                │
                ├─ create_nodes → modify workflow in DB → emit canvas refresh event
                ├─ update_node_config → modify workflow in DB → emit canvas refresh
                ├─ connect_nodes → modify workflow in DB → emit canvas refresh
                └─ get_workflow_state → read DB → return JSON to AI
                        │
                        ▼
                AI continues streaming...
                        │
                        ▼
                Final text response → frontend shows message
```

---

## Testing Strategy

### Unit Tests
- Adapter tests with mocked HTTP servers — verify request format, auth headers, response parsing
- Store tests — CRUD on ai_providers and ai_chat_messages with in-memory SQLite
- Model registry — verify all providers have required fields, no duplicate IDs
- Tool implementations — mock canvas state, verify tool calls produce correct DB mutations
- Node executors — mock AI client, verify prompt template expansion and output shaping

### Integration Tests (opt-in, skip without env vars)
```bash
OPENAI_API_KEY=sk-... go test ./internal/ai/... -tags integration
ANTHROPIC_API_KEY=sk-... go test ./internal/ai/... -tags integration
OPENROUTER_API_KEY=sk-... go test ./internal/ai/... -tags integration
```

### Manual Testing Checklist
- [ ] Add OpenAI provider, test connection
- [ ] Add OpenRouter (gateway tier), enter custom model string, test
- [ ] Open workflow, open chat panel, ask AI to add two nodes and connect them
- [ ] Verify nodes appear on canvas in real-time
- [ ] Add ai.chat node to workflow, configure with provider, run workflow, verify output
- [ ] Add ai.classify node, verify per-category output handles appear
- [ ] Clear chat history, verify panel resets

---

## Implementation Sequence

1. **`internal/ai/registry.go`** — Provider definitions + model lists (pure data, no deps)
2. **`internal/ai/store.go`** — SQLite store for ai_providers + ai_chat_messages
3. **`internal/ai/client.go`** — Client interface + types
4. **`internal/ai/adapters/openai.go`** — OpenAI-compat adapter (covers most providers)
5. **`internal/ai/adapters/anthropic.go`** — Anthropic adapter
6. **`internal/ai/adapters/google.go`** — Google Gemini adapter
7. **`internal/ai/adapters/bedrock.go`** — AWS Bedrock adapter
8. **`app.go` provider functions** — ListAIProviders, SaveAIProvider, DeleteAIProvider, TestAIProvider, GetAIModels
9. **Frontend: AI Providers page** — tile grid, add/manage modals, combobox model picker
10. **`internal/ai/chat/tools.go`** — Canvas tool implementations
11. **`internal/ai/chat/service.go`** — Chat service: history, system prompt, streaming, tool dispatch
12. **`app.go` chat functions** — StreamAIChat, GetAIChatHistory, ClearAIChatHistory
13. **Frontend: AI Chat Panel** — sidebar in Workflow builder, streaming UI, tool call cards
14. **`internal/ai/nodes/`** — All 6 AI node executors
15. **Frontend: AI node palette + config panels** — node types in palette, per-node provider/model config
16. **Wire AI nodes into workflow engine** — register executors in node registry
