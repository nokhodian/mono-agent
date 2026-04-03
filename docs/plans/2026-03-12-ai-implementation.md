# AI Section Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a first-class AI system to Mono Agent — provider registry, chat panel, and AI workflow nodes.

**Architecture:** Go backend with `internal/ai/` package tree (registry, store, client, adapters, chat service, node executors). Frontend React pages + components. Wails bindings for IPC. Streaming via `runtime.EventsEmit`.

**Tech Stack:** Go 1.22, `net/http` for AI API calls, SQLite via `modernc.org/sqlite`, Wails v2, React/JSX, CSS variables.

---

## Task 1: AI Provider Registry — `internal/ai/registry.go`

**Files:**
- Create: `internal/ai/registry.go`
- Test: `internal/ai/registry_test.go`

**Step 1: Write the test**

```go
// internal/ai/registry_test.go
package ai

import "testing"

func TestRegistryNoDuplicateIDs(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range ProviderRegistry {
		if seen[p.ID] {
			t.Fatalf("duplicate provider ID: %s", p.ID)
		}
		seen[p.ID] = true
	}
}

func TestRegistryRequiredFields(t *testing.T) {
	for _, p := range ProviderRegistry {
		if p.ID == "" || p.Name == "" || p.Tier == "" || p.Adapter == "" {
			t.Fatalf("provider %q missing required field", p.ID)
		}
		if p.Tier != "known" && p.Tier != "gateway" {
			t.Fatalf("provider %q has invalid tier %q", p.ID, p.Tier)
		}
	}
}

func TestRegistryModels(t *testing.T) {
	for _, p := range ProviderRegistry {
		for _, m := range p.Models {
			if m.ID == "" || m.Name == "" {
				t.Fatalf("provider %q has model with empty ID or Name", p.ID)
			}
		}
	}
}

func TestGetProviderDef(t *testing.T) {
	p, ok := GetProviderDef("openai")
	if !ok {
		t.Fatal("openai not found")
	}
	if p.Name != "OpenAI" {
		t.Fatalf("expected OpenAI, got %s", p.Name)
	}
	_, ok = GetProviderDef("nonexistent")
	if ok {
		t.Fatal("nonexistent should not be found")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes && go test ./internal/ai/... -run TestRegistry -v`
Expected: FAIL — package doesn't exist yet

**Step 3: Write implementation**

```go
// internal/ai/registry.go
package ai

// ProviderDef defines a known AI provider in the hardcoded registry.
type ProviderDef struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	IconEmoji      string     `json:"icon_emoji"`
	Category       string     `json:"category"` // "frontier","cloud","inference","gateway","other"
	Tier           string     `json:"tier"`      // "known" | "gateway"
	DefaultBaseURL string     `json:"default_base_url"`
	AuthLabel      string     `json:"auth_label"`
	Models         []ModelDef `json:"models"`
	Adapter        string     `json:"adapter"` // "openai","anthropic","google","bedrock"
	DocsURL        string     `json:"docs_url"`
}

// ModelDef defines a model offered by a provider.
type ModelDef struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	ContextWindow int      `json:"context_window"`
	Capabilities  []string `json:"capabilities"` // "chat","vision","tools","embed"
}

// GetProviderDef returns a provider definition by ID.
func GetProviderDef(id string) (ProviderDef, bool) {
	for _, p := range ProviderRegistry {
		if p.ID == id {
			return p, true
		}
	}
	return ProviderDef{}, false
}

// ProvidersByCategory returns providers filtered by category.
func ProvidersByCategory(category string) []ProviderDef {
	var result []ProviderDef
	for _, p := range ProviderRegistry {
		if p.Category == category {
			result = append(result, p)
		}
	}
	return result
}

// ProviderRegistry is the hardcoded list of all supported AI providers.
var ProviderRegistry = []ProviderDef{
	// ─── Frontier ──────────────────────────────────────────────
	{
		ID: "openai", Name: "OpenAI", IconEmoji: "🟢", Category: "frontier", Tier: "known",
		DefaultBaseURL: "https://api.openai.com/v1", AuthLabel: "API Key", Adapter: "openai",
		DocsURL: "https://platform.openai.com/docs",
		Models: []ModelDef{
			{ID: "gpt-4o", Name: "GPT-4o", ContextWindow: 128000, Capabilities: []string{"chat", "vision", "tools"}},
			{ID: "gpt-4o-mini", Name: "GPT-4o Mini", ContextWindow: 128000, Capabilities: []string{"chat", "vision", "tools"}},
			{ID: "gpt-4-turbo", Name: "GPT-4 Turbo", ContextWindow: 128000, Capabilities: []string{"chat", "vision", "tools"}},
			{ID: "o1", Name: "o1", ContextWindow: 200000, Capabilities: []string{"chat", "tools"}},
			{ID: "o1-mini", Name: "o1 Mini", ContextWindow: 128000, Capabilities: []string{"chat"}},
			{ID: "o3-mini", Name: "o3 Mini", ContextWindow: 200000, Capabilities: []string{"chat", "tools"}},
			{ID: "text-embedding-3-small", Name: "Embedding 3 Small", ContextWindow: 8191, Capabilities: []string{"embed"}},
			{ID: "text-embedding-3-large", Name: "Embedding 3 Large", ContextWindow: 8191, Capabilities: []string{"embed"}},
		},
	},
	{
		ID: "anthropic", Name: "Anthropic", IconEmoji: "🟤", Category: "frontier", Tier: "known",
		DefaultBaseURL: "https://api.anthropic.com", AuthLabel: "API Key", Adapter: "anthropic",
		DocsURL: "https://docs.anthropic.com",
		Models: []ModelDef{
			{ID: "claude-opus-4-6", Name: "Claude Opus 4.6", ContextWindow: 200000, Capabilities: []string{"chat", "vision", "tools"}},
			{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6", ContextWindow: 200000, Capabilities: []string{"chat", "vision", "tools"}},
			{ID: "claude-haiku-4-5-20251001", Name: "Claude Haiku 4.5", ContextWindow: 200000, Capabilities: []string{"chat", "vision", "tools"}},
		},
	},
	{
		ID: "google", Name: "Google (Gemini)", IconEmoji: "🔵", Category: "frontier", Tier: "known",
		DefaultBaseURL: "https://generativelanguage.googleapis.com/v1beta", AuthLabel: "API Key", Adapter: "google",
		DocsURL: "https://ai.google.dev/docs",
		Models: []ModelDef{
			{ID: "gemini-2.0-flash", Name: "Gemini 2.0 Flash", ContextWindow: 1048576, Capabilities: []string{"chat", "vision", "tools"}},
			{ID: "gemini-2.0-pro", Name: "Gemini 2.0 Pro", ContextWindow: 1048576, Capabilities: []string{"chat", "vision", "tools"}},
			{ID: "gemini-1.5-pro", Name: "Gemini 1.5 Pro", ContextWindow: 2097152, Capabilities: []string{"chat", "vision", "tools"}},
			{ID: "text-embedding-004", Name: "Text Embedding 004", ContextWindow: 2048, Capabilities: []string{"embed"}},
		},
	},
	{
		ID: "xai", Name: "xAI (Grok)", IconEmoji: "⚡", Category: "frontier", Tier: "known",
		DefaultBaseURL: "https://api.x.ai/v1", AuthLabel: "API Key", Adapter: "openai",
		DocsURL: "https://docs.x.ai",
		Models: []ModelDef{
			{ID: "grok-2", Name: "Grok 2", ContextWindow: 131072, Capabilities: []string{"chat", "vision", "tools"}},
			{ID: "grok-2-mini", Name: "Grok 2 Mini", ContextWindow: 131072, Capabilities: []string{"chat", "tools"}},
		},
	},
	{
		ID: "mistral", Name: "Mistral AI", IconEmoji: "🌊", Category: "frontier", Tier: "known",
		DefaultBaseURL: "https://api.mistral.ai/v1", AuthLabel: "API Key", Adapter: "openai",
		DocsURL: "https://docs.mistral.ai",
		Models: []ModelDef{
			{ID: "mistral-large-latest", Name: "Mistral Large", ContextWindow: 128000, Capabilities: []string{"chat", "vision", "tools"}},
			{ID: "mistral-small-latest", Name: "Mistral Small", ContextWindow: 128000, Capabilities: []string{"chat", "tools"}},
			{ID: "codestral-latest", Name: "Codestral", ContextWindow: 256000, Capabilities: []string{"chat", "tools"}},
			{ID: "mistral-embed", Name: "Mistral Embed", ContextWindow: 8192, Capabilities: []string{"embed"}},
		},
	},
	{
		ID: "cohere", Name: "Cohere", IconEmoji: "🔷", Category: "frontier", Tier: "known",
		DefaultBaseURL: "https://api.cohere.com/v2", AuthLabel: "API Key", Adapter: "openai",
		DocsURL: "https://docs.cohere.com",
		Models: []ModelDef{
			{ID: "command-r-plus", Name: "Command R+", ContextWindow: 128000, Capabilities: []string{"chat", "tools"}},
			{ID: "command-r", Name: "Command R", ContextWindow: 128000, Capabilities: []string{"chat", "tools"}},
			{ID: "embed-english-v3.0", Name: "Embed English v3", ContextWindow: 512, Capabilities: []string{"embed"}},
		},
	},
	{
		ID: "deepseek", Name: "DeepSeek", IconEmoji: "🐋", Category: "frontier", Tier: "known",
		DefaultBaseURL: "https://api.deepseek.com/v1", AuthLabel: "API Key", Adapter: "openai",
		DocsURL: "https://platform.deepseek.com/docs",
		Models: []ModelDef{
			{ID: "deepseek-chat", Name: "DeepSeek V3", ContextWindow: 64000, Capabilities: []string{"chat", "tools"}},
			{ID: "deepseek-reasoner", Name: "DeepSeek R1", ContextWindow: 64000, Capabilities: []string{"chat"}},
		},
	},
	{
		ID: "ai21", Name: "AI21 Labs", IconEmoji: "🧬", Category: "frontier", Tier: "known",
		DefaultBaseURL: "https://api.ai21.com/studio/v1", AuthLabel: "API Key", Adapter: "openai",
		DocsURL: "https://docs.ai21.com",
		Models: []ModelDef{
			{ID: "jamba-1.5-large", Name: "Jamba 1.5 Large", ContextWindow: 256000, Capabilities: []string{"chat"}},
			{ID: "jamba-1.5-mini", Name: "Jamba 1.5 Mini", ContextWindow: 256000, Capabilities: []string{"chat"}},
		},
	},
	{
		ID: "alibaba", Name: "Alibaba (Qwen)", IconEmoji: "☁️", Category: "frontier", Tier: "known",
		DefaultBaseURL: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1", AuthLabel: "API Key", Adapter: "openai",
		DocsURL: "https://help.aliyun.com/zh/model-studio/",
		Models: []ModelDef{
			{ID: "qwen-max", Name: "Qwen Max", ContextWindow: 32768, Capabilities: []string{"chat", "tools"}},
			{ID: "qwen-plus", Name: "Qwen Plus", ContextWindow: 131072, Capabilities: []string{"chat", "tools"}},
			{ID: "qwen-turbo", Name: "Qwen Turbo", ContextWindow: 131072, Capabilities: []string{"chat", "tools"}},
		},
	},
	{
		ID: "zhipu", Name: "Zhipu AI (GLM)", IconEmoji: "🀄", Category: "frontier", Tier: "known",
		DefaultBaseURL: "https://open.bigmodel.cn/api/paas/v4", AuthLabel: "API Key", Adapter: "openai",
		Models: []ModelDef{
			{ID: "glm-4-plus", Name: "GLM-4 Plus", ContextWindow: 128000, Capabilities: []string{"chat", "tools"}},
		},
	},
	{
		ID: "moonshot", Name: "Moonshot AI (Kimi)", IconEmoji: "🌙", Category: "frontier", Tier: "known",
		DefaultBaseURL: "https://api.moonshot.cn/v1", AuthLabel: "API Key", Adapter: "openai",
		Models: []ModelDef{
			{ID: "moonshot-v1-128k", Name: "Moonshot v1 128K", ContextWindow: 128000, Capabilities: []string{"chat"}},
		},
	},
	{
		ID: "minimax", Name: "MiniMax", IconEmoji: "🤖", Category: "frontier", Tier: "known",
		DefaultBaseURL: "https://api.minimax.chat/v1", AuthLabel: "API Key", Adapter: "openai",
		Models: []ModelDef{
			{ID: "abab6.5s-chat", Name: "ABAB 6.5s", ContextWindow: 245760, Capabilities: []string{"chat"}},
		},
	},
	{
		ID: "writer", Name: "Writer", IconEmoji: "✍️", Category: "frontier", Tier: "known",
		DefaultBaseURL: "https://api.writer.com/v1", AuthLabel: "API Key", Adapter: "openai",
		Models: []ModelDef{
			{ID: "palmyra-x-004", Name: "Palmyra X 004", ContextWindow: 128000, Capabilities: []string{"chat", "tools"}},
		},
	},

	// ─── Cloud ─────────────────────────────────────────────────
	{
		ID: "bedrock", Name: "AWS Bedrock", IconEmoji: "🏔️", Category: "cloud", Tier: "known",
		DefaultBaseURL: "", AuthLabel: "Access Key + Secret + Region", Adapter: "bedrock",
		DocsURL: "https://docs.aws.amazon.com/bedrock/",
		Models: []ModelDef{
			{ID: "anthropic.claude-sonnet-4-6-20250514-v1:0", Name: "Claude Sonnet 4.6 (Bedrock)", ContextWindow: 200000, Capabilities: []string{"chat", "vision", "tools"}},
			{ID: "amazon.nova-pro-v1:0", Name: "Amazon Nova Pro", ContextWindow: 300000, Capabilities: []string{"chat", "vision", "tools"}},
		},
	},
	{
		ID: "azure", Name: "Azure AI Foundry", IconEmoji: "🔷", Category: "cloud", Tier: "known",
		DefaultBaseURL: "", AuthLabel: "Endpoint + API Key", Adapter: "openai",
		DocsURL: "https://learn.microsoft.com/en-us/azure/ai-services/openai/",
		Models: []ModelDef{},
	},
	{
		ID: "vertex", Name: "Google Vertex AI", IconEmoji: "🔺", Category: "cloud", Tier: "known",
		DefaultBaseURL: "", AuthLabel: "Service Account JSON", Adapter: "google",
		DocsURL: "https://cloud.google.com/vertex-ai/docs",
		Models: []ModelDef{},
	},
	{
		ID: "ibm", Name: "IBM watsonx.ai", IconEmoji: "🏢", Category: "cloud", Tier: "known",
		DefaultBaseURL: "https://us-south.ml.cloud.ibm.com/ml/v1", AuthLabel: "API Key + Project ID", Adapter: "openai",
		Models: []ModelDef{},
	},
	{
		ID: "databricks", Name: "Databricks", IconEmoji: "🧱", Category: "cloud", Tier: "known",
		DefaultBaseURL: "", AuthLabel: "Workspace URL + Token", Adapter: "openai",
		Models: []ModelDef{},
	},
	{
		ID: "snowflake", Name: "Snowflake Cortex", IconEmoji: "❄️", Category: "cloud", Tier: "known",
		DefaultBaseURL: "", AuthLabel: "Account + Token", Adapter: "openai",
		Models: []ModelDef{},
	},
	{
		ID: "cloudflare", Name: "Cloudflare Workers AI", IconEmoji: "🟠", Category: "cloud", Tier: "known",
		DefaultBaseURL: "https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1", AuthLabel: "Account ID + API Token", Adapter: "openai",
		Models: []ModelDef{
			{ID: "@cf/meta/llama-3.1-70b-instruct", Name: "Llama 3.1 70B", ContextWindow: 131072, Capabilities: []string{"chat"}},
		},
	},

	// ─── Inference ─────────────────────────────────────────────
	{
		ID: "groq", Name: "Groq", IconEmoji: "⚡", Category: "inference", Tier: "known",
		DefaultBaseURL: "https://api.groq.com/openai/v1", AuthLabel: "API Key", Adapter: "openai",
		DocsURL: "https://console.groq.com/docs",
		Models: []ModelDef{
			{ID: "llama-3.3-70b-versatile", Name: "Llama 3.3 70B", ContextWindow: 131072, Capabilities: []string{"chat", "tools"}},
			{ID: "llama-3.1-8b-instant", Name: "Llama 3.1 8B", ContextWindow: 131072, Capabilities: []string{"chat", "tools"}},
			{ID: "mixtral-8x7b-32768", Name: "Mixtral 8x7B", ContextWindow: 32768, Capabilities: []string{"chat"}},
			{ID: "gemma2-9b-it", Name: "Gemma 2 9B", ContextWindow: 8192, Capabilities: []string{"chat"}},
		},
	},
	{
		ID: "cerebras", Name: "Cerebras", IconEmoji: "🧠", Category: "inference", Tier: "known",
		DefaultBaseURL: "https://api.cerebras.ai/v1", AuthLabel: "API Key", Adapter: "openai",
		Models: []ModelDef{
			{ID: "llama3.1-70b", Name: "Llama 3.1 70B", ContextWindow: 131072, Capabilities: []string{"chat"}},
			{ID: "llama3.1-8b", Name: "Llama 3.1 8B", ContextWindow: 131072, Capabilities: []string{"chat"}},
		},
	},
	{
		ID: "together", Name: "Together AI", IconEmoji: "🤝", Category: "inference", Tier: "known",
		DefaultBaseURL: "https://api.together.xyz/v1", AuthLabel: "API Key", Adapter: "openai",
		DocsURL: "https://docs.together.ai",
		Models: []ModelDef{
			{ID: "meta-llama/Meta-Llama-3.1-405B-Instruct-Turbo", Name: "Llama 3.1 405B", ContextWindow: 131072, Capabilities: []string{"chat", "tools"}},
			{ID: "meta-llama/Meta-Llama-3.1-70B-Instruct-Turbo", Name: "Llama 3.1 70B", ContextWindow: 131072, Capabilities: []string{"chat", "tools"}},
			{ID: "mistralai/Mixtral-8x22B-Instruct-v0.1", Name: "Mixtral 8x22B", ContextWindow: 65536, Capabilities: []string{"chat"}},
		},
	},
	{
		ID: "fireworks", Name: "Fireworks AI", IconEmoji: "🎆", Category: "inference", Tier: "known",
		DefaultBaseURL: "https://api.fireworks.ai/inference/v1", AuthLabel: "API Key", Adapter: "openai",
		Models: []ModelDef{
			{ID: "accounts/fireworks/models/llama-v3p1-405b-instruct", Name: "Llama 3.1 405B", ContextWindow: 131072, Capabilities: []string{"chat", "tools"}},
		},
	},
	{
		ID: "deepinfra", Name: "DeepInfra", IconEmoji: "🔬", Category: "inference", Tier: "known",
		DefaultBaseURL: "https://api.deepinfra.com/v1/openai", AuthLabel: "API Key", Adapter: "openai",
		Models: []ModelDef{
			{ID: "meta-llama/Meta-Llama-3.1-70B-Instruct", Name: "Llama 3.1 70B", ContextWindow: 131072, Capabilities: []string{"chat", "tools"}},
		},
	},
	{
		ID: "sambanova", Name: "SambaNova", IconEmoji: "🏃", Category: "inference", Tier: "known",
		DefaultBaseURL: "https://api.sambanova.ai/v1", AuthLabel: "API Key", Adapter: "openai",
		Models: []ModelDef{
			{ID: "Meta-Llama-3.1-405B-Instruct", Name: "Llama 3.1 405B", ContextWindow: 131072, Capabilities: []string{"chat"}},
		},
	},
	{
		ID: "replicate", Name: "Replicate", IconEmoji: "🔁", Category: "inference", Tier: "known",
		DefaultBaseURL: "https://api.replicate.com/v1", AuthLabel: "API Token", Adapter: "openai",
		Models: []ModelDef{},
	},
	{
		ID: "perplexity", Name: "Perplexity AI", IconEmoji: "🔍", Category: "inference", Tier: "known",
		DefaultBaseURL: "https://api.perplexity.ai", AuthLabel: "API Key", Adapter: "openai",
		Models: []ModelDef{
			{ID: "sonar-pro", Name: "Sonar Pro", ContextWindow: 200000, Capabilities: []string{"chat"}},
			{ID: "sonar", Name: "Sonar", ContextWindow: 128000, Capabilities: []string{"chat"}},
		},
	},
	{
		ID: "runpod", Name: "RunPod", IconEmoji: "🏃‍♂️", Category: "inference", Tier: "known",
		DefaultBaseURL: "https://api.runpod.ai/v2", AuthLabel: "API Key", Adapter: "openai",
		Models: []ModelDef{},
	},
	{
		ID: "lambda", Name: "Lambda Labs", IconEmoji: "λ", Category: "inference", Tier: "known",
		DefaultBaseURL: "https://api.lambdalabs.com/v1", AuthLabel: "API Key", Adapter: "openai",
		Models: []ModelDef{},
	},
	{
		ID: "novita", Name: "Novita AI", IconEmoji: "🌟", Category: "inference", Tier: "known",
		DefaultBaseURL: "https://api.novita.ai/v3/openai", AuthLabel: "API Key", Adapter: "openai",
		Models: []ModelDef{},
	},
	{
		ID: "hyperbolic", Name: "Hyperbolic", IconEmoji: "📐", Category: "inference", Tier: "known",
		DefaultBaseURL: "https://api.hyperbolic.xyz/v1", AuthLabel: "API Key", Adapter: "openai",
		Models: []ModelDef{},
	},

	// ─── Gateways ──────────────────────────────────────────────
	{
		ID: "openrouter", Name: "OpenRouter", IconEmoji: "🔀", Category: "gateway", Tier: "gateway",
		DefaultBaseURL: "https://openrouter.ai/api/v1", AuthLabel: "API Key", Adapter: "openai",
		DocsURL: "https://openrouter.ai/docs",
		Models: []ModelDef{
			{ID: "openai/gpt-4o", Name: "GPT-4o (via OR)", ContextWindow: 128000, Capabilities: []string{"chat", "vision", "tools"}},
			{ID: "anthropic/claude-sonnet-4-6", Name: "Claude Sonnet 4.6 (via OR)", ContextWindow: 200000, Capabilities: []string{"chat", "vision", "tools"}},
			{ID: "google/gemini-2.0-flash-001", Name: "Gemini 2.0 Flash (via OR)", ContextWindow: 1048576, Capabilities: []string{"chat", "vision", "tools"}},
			{ID: "meta-llama/llama-3.1-405b-instruct", Name: "Llama 3.1 405B (via OR)", ContextWindow: 131072, Capabilities: []string{"chat", "tools"}},
		},
	},
	{
		ID: "litellm", Name: "LiteLLM", IconEmoji: "🪶", Category: "gateway", Tier: "gateway",
		DefaultBaseURL: "http://localhost:4000", AuthLabel: "API Key (optional)", Adapter: "openai",
		DocsURL: "https://docs.litellm.ai",
		Models: []ModelDef{},
	},
	{
		ID: "portkey", Name: "Portkey", IconEmoji: "🔑", Category: "gateway", Tier: "gateway",
		DefaultBaseURL: "https://api.portkey.ai/v1", AuthLabel: "API Key", Adapter: "openai",
		DocsURL: "https://docs.portkey.ai",
		Models: []ModelDef{},
	},
	{
		ID: "helicone", Name: "Helicone", IconEmoji: "☀️", Category: "gateway", Tier: "gateway",
		DefaultBaseURL: "https://oai.helicone.ai/v1", AuthLabel: "API Key + Helicone Auth", Adapter: "openai",
		Models: []ModelDef{},
	},
	{
		ID: "bifrost", Name: "Bifrost (Maxim AI)", IconEmoji: "🌈", Category: "gateway", Tier: "gateway",
		DefaultBaseURL: "", AuthLabel: "API Key", Adapter: "openai",
		Models: []ModelDef{},
	},
	{
		ID: "kong", Name: "Kong AI Gateway", IconEmoji: "🦍", Category: "gateway", Tier: "gateway",
		DefaultBaseURL: "", AuthLabel: "Gateway URL + API Key", Adapter: "openai",
		Models: []ModelDef{},
	},
	{
		ID: "martian", Name: "Martian", IconEmoji: "👽", Category: "gateway", Tier: "gateway",
		DefaultBaseURL: "https://api.withmartian.com/v1", AuthLabel: "API Key", Adapter: "openai",
		Models: []ModelDef{},
	},
	{
		ID: "unify", Name: "Unify AI", IconEmoji: "🔗", Category: "gateway", Tier: "gateway",
		DefaultBaseURL: "https://api.unify.ai/v0", AuthLabel: "API Key", Adapter: "openai",
		Models: []ModelDef{},
	},
	{
		ID: "neutrino", Name: "Neutrino", IconEmoji: "⚛️", Category: "gateway", Tier: "gateway",
		DefaultBaseURL: "https://api.neutrinoapp.com/v1", AuthLabel: "API Key", Adapter: "openai",
		Models: []ModelDef{},
	},
}
```

**Step 4: Run tests**

Run: `cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes && go test ./internal/ai/... -run TestRegistry -v`
Expected: PASS (4 tests)

**Step 5: Commit**

```bash
git add internal/ai/registry.go internal/ai/registry_test.go
git commit -m "feat(ai): add provider registry with 40+ AI providers and model definitions"
```

---

## Task 2: AI Store — `internal/ai/store.go`

**Files:**
- Create: `internal/ai/store.go`
- Test: `internal/ai/store_test.go`

**Step 1: Write the test**

```go
// internal/ai/store_test.go
package ai

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestStoreInitTables(t *testing.T) {
	db := testDB(t)
	s, err := NewAIStore(db)
	if err != nil {
		t.Fatal(err)
	}
	_ = s
	// Verify tables exist by inserting
	_, err = db.Exec(`INSERT INTO ai_providers (id,name,provider_id,tier,api_key,status,created_at) VALUES ('t','t','openai','known','sk-test','untested','2025-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("ai_providers table not created: %v", err)
	}
	_, err = db.Exec(`INSERT INTO ai_chat_messages (id,workflow_id,role,content,created_at) VALUES ('m1','w1','user','hello','2025-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("ai_chat_messages table not created: %v", err)
	}
}

func TestProviderCRUD(t *testing.T) {
	db := testDB(t)
	s, _ := NewAIStore(db)

	p := AIProvider{
		ID: "p1", Name: "My OpenAI", ProviderID: "openai", Tier: "known",
		APIKey: "sk-test", Status: "untested",
	}
	if err := s.SaveProvider(p); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetProvider("p1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "My OpenAI" || got.APIKey != "sk-test" {
		t.Fatalf("unexpected provider: %+v", got)
	}

	all, err := s.ListProviders()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(all))
	}

	p.Name = "Updated"
	if err := s.SaveProvider(p); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetProvider("p1")
	if got.Name != "Updated" {
		t.Fatal("update failed")
	}

	if err := s.DeleteProvider("p1"); err != nil {
		t.Fatal(err)
	}
	all, _ = s.ListProviders()
	if len(all) != 0 {
		t.Fatal("delete failed")
	}
}

func TestChatMessageCRUD(t *testing.T) {
	db := testDB(t)
	s, _ := NewAIStore(db)

	msg := ChatMessage{
		ID: "m1", WorkflowID: "w1", Role: "user", Content: "hello",
	}
	if err := s.SaveChatMessage(msg); err != nil {
		t.Fatal(err)
	}

	msgs, err := s.GetChatHistory("w1")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Content != "hello" {
		t.Fatalf("unexpected: %+v", msgs)
	}

	if err := s.ClearChatHistory("w1"); err != nil {
		t.Fatal(err)
	}
	msgs, _ = s.GetChatHistory("w1")
	if len(msgs) != 0 {
		t.Fatal("clear failed")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes && go test ./internal/ai/... -run TestStore -v`
Expected: FAIL — NewAIStore not defined

**Step 3: Write implementation**

```go
// internal/ai/store.go
package ai

import (
	"database/sql"
	"time"
)

// AIProvider is a user-configured AI provider connection stored in SQLite.
type AIProvider struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderID   string `json:"provider_id"`
	Tier         string `json:"tier"`
	APIKey       string `json:"api_key"`
	BaseURL      string `json:"base_url"`
	DefaultModel string `json:"default_model"`
	ExtraHeaders string `json:"extra_headers"`
	Status       string `json:"status"`
	LastTested   string `json:"last_tested"`
	CreatedAt    string `json:"created_at"`
}

// ChatMessage is a single message in a workflow's AI chat history.
type ChatMessage struct {
	ID         string `json:"id"`
	WorkflowID string `json:"workflow_id"`
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCalls  string `json:"tool_calls,omitempty"`
	ProviderID string `json:"provider_id,omitempty"`
	Model      string `json:"model,omitempty"`
	TokenCount int    `json:"token_count,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// AIStore handles SQLite persistence for AI providers and chat messages.
type AIStore struct {
	db *sql.DB
}

// NewAIStore creates the store and ensures tables exist.
func NewAIStore(db *sql.DB) (*AIStore, error) {
	s := &AIStore{db: db}
	if err := s.init(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *AIStore) init() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS ai_providers (
			id            TEXT PRIMARY KEY,
			name          TEXT NOT NULL,
			provider_id   TEXT NOT NULL,
			tier          TEXT NOT NULL,
			api_key       TEXT NOT NULL,
			base_url      TEXT NOT NULL DEFAULT '',
			default_model TEXT NOT NULL DEFAULT '',
			extra_headers TEXT NOT NULL DEFAULT '',
			status        TEXT NOT NULL DEFAULT 'untested',
			last_tested   TEXT NOT NULL DEFAULT '',
			created_at    TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS ai_chat_messages (
			id           TEXT PRIMARY KEY,
			workflow_id  TEXT NOT NULL,
			role         TEXT NOT NULL,
			content      TEXT NOT NULL,
			tool_calls   TEXT NOT NULL DEFAULT '',
			provider_id  TEXT NOT NULL DEFAULT '',
			model        TEXT NOT NULL DEFAULT '',
			token_count  INTEGER NOT NULL DEFAULT 0,
			created_at   TEXT NOT NULL
		)`,
	}
	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func (s *AIStore) SaveProvider(p AIProvider) error {
	if p.CreatedAt == "" {
		p.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.db.Exec(`INSERT INTO ai_providers (id,name,provider_id,tier,api_key,base_url,default_model,extra_headers,status,last_tested,created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, provider_id=excluded.provider_id, tier=excluded.tier,
			api_key=excluded.api_key, base_url=excluded.base_url, default_model=excluded.default_model,
			extra_headers=excluded.extra_headers, status=excluded.status, last_tested=excluded.last_tested`,
		p.ID, p.Name, p.ProviderID, p.Tier, p.APIKey, p.BaseURL, p.DefaultModel, p.ExtraHeaders, p.Status, p.LastTested, p.CreatedAt)
	return err
}

func (s *AIStore) GetProvider(id string) (AIProvider, error) {
	var p AIProvider
	err := s.db.QueryRow(`SELECT id,name,provider_id,tier,api_key,base_url,default_model,extra_headers,status,last_tested,created_at FROM ai_providers WHERE id=?`, id).
		Scan(&p.ID, &p.Name, &p.ProviderID, &p.Tier, &p.APIKey, &p.BaseURL, &p.DefaultModel, &p.ExtraHeaders, &p.Status, &p.LastTested, &p.CreatedAt)
	return p, err
}

func (s *AIStore) ListProviders() ([]AIProvider, error) {
	rows, err := s.db.Query(`SELECT id,name,provider_id,tier,api_key,base_url,default_model,extra_headers,status,last_tested,created_at FROM ai_providers ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var providers []AIProvider
	for rows.Next() {
		var p AIProvider
		if err := rows.Scan(&p.ID, &p.Name, &p.ProviderID, &p.Tier, &p.APIKey, &p.BaseURL, &p.DefaultModel, &p.ExtraHeaders, &p.Status, &p.LastTested, &p.CreatedAt); err != nil {
			return nil, err
		}
		providers = append(providers, p)
	}
	return providers, rows.Err()
}

func (s *AIStore) DeleteProvider(id string) error {
	_, err := s.db.Exec(`DELETE FROM ai_providers WHERE id=?`, id)
	return err
}

func (s *AIStore) UpdateProviderStatus(id, status, lastTested string) error {
	_, err := s.db.Exec(`UPDATE ai_providers SET status=?, last_tested=? WHERE id=?`, status, lastTested, id)
	return err
}

func (s *AIStore) SaveChatMessage(m ChatMessage) error {
	if m.CreatedAt == "" {
		m.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.db.Exec(`INSERT INTO ai_chat_messages (id,workflow_id,role,content,tool_calls,provider_id,model,token_count,created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		m.ID, m.WorkflowID, m.Role, m.Content, m.ToolCalls, m.ProviderID, m.Model, m.TokenCount, m.CreatedAt)
	return err
}

func (s *AIStore) GetChatHistory(workflowID string) ([]ChatMessage, error) {
	rows, err := s.db.Query(`SELECT id,workflow_id,role,content,tool_calls,provider_id,model,token_count,created_at FROM ai_chat_messages WHERE workflow_id=? ORDER BY created_at ASC`, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []ChatMessage
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(&m.ID, &m.WorkflowID, &m.Role, &m.Content, &m.ToolCalls, &m.ProviderID, &m.Model, &m.TokenCount, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func (s *AIStore) ClearChatHistory(workflowID string) error {
	_, err := s.db.Exec(`DELETE FROM ai_chat_messages WHERE workflow_id=?`, workflowID)
	return err
}
```

**Step 4: Run tests**

Run: `cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes && go test ./internal/ai/... -run "TestStore|TestProvider|TestChat" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/ai/store.go internal/ai/store_test.go
git commit -m "feat(ai): add SQLite store for AI providers and chat messages"
```

---

## Task 3: AI Client Interface — `internal/ai/client.go`

**Files:**
- Create: `internal/ai/client.go`

**Step 1: Write implementation**

```go
// internal/ai/client.go
package ai

import "context"

// Role constants for chat messages.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// CompletionRequest is the unified request for all adapters.
type CompletionRequest struct {
	Model       string            `json:"model"`
	Messages    []Message         `json:"messages"`
	Tools       []ToolDef         `json:"tools,omitempty"`
	Temperature *float64          `json:"temperature,omitempty"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Stream      bool              `json:"stream,omitempty"`
}

// Message is a single message in the conversation.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolDef defines a tool the AI can call.
type ToolDef struct {
	Type     string         `json:"type"` // "function"
	Function ToolFunction   `json:"function"`
}

// ToolFunction describes a callable function.
type ToolFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"` // JSON Schema object
}

// ToolCall is a tool invocation from the model.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function"
	Function ToolCallFunc `json:"function"`
}

// ToolCallFunc holds the function name and arguments JSON string.
type ToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// CompletionResponse is the unified response from all adapters.
type CompletionResponse struct {
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string   `json:"finish_reason"`
	Usage      Usage      `json:"usage"`
}

// Usage tracks token consumption.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// StreamChunk is emitted during streaming completions.
type StreamChunk struct {
	Content      string     `json:"content,omitempty"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string     `json:"finish_reason,omitempty"`
	Done         bool       `json:"done"`
}

// AIClient is the interface every adapter must implement.
type AIClient interface {
	// Complete sends a non-streaming completion request.
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
	// StreamComplete sends a streaming completion request, emitting chunks via the callback.
	StreamComplete(ctx context.Context, req CompletionRequest, onChunk func(StreamChunk)) error
}

// NewClient creates the appropriate AIClient for a given provider config.
func NewClient(provider AIProvider) (AIClient, error) {
	def, ok := GetProviderDef(provider.ProviderID)
	if !ok {
		// For gateway tier, default to openai adapter
		if provider.Tier == "gateway" {
			return NewOpenAIClient(provider.APIKey, provider.BaseURL, provider.ExtraHeaders), nil
		}
		return nil, fmt.Errorf("unknown provider: %s", provider.ProviderID)
	}

	baseURL := provider.BaseURL
	if baseURL == "" {
		baseURL = def.DefaultBaseURL
	}

	switch def.Adapter {
	case "openai":
		return NewOpenAIClient(provider.APIKey, baseURL, provider.ExtraHeaders), nil
	case "anthropic":
		return NewAnthropicClient(provider.APIKey, baseURL), nil
	case "google":
		return NewGoogleClient(provider.APIKey, baseURL), nil
	case "bedrock":
		return NewBedrockClient(provider.APIKey, baseURL, provider.ExtraHeaders), nil
	default:
		return NewOpenAIClient(provider.APIKey, baseURL, provider.ExtraHeaders), nil
	}
}
```

Note: this file references adapter constructors that will be created in Tasks 4-7. The `fmt` import is also needed.

**Step 2: Commit**

```bash
git add internal/ai/client.go
git commit -m "feat(ai): add unified AI client interface and types"
```

---

## Task 4: OpenAI-Compatible Adapter — `internal/ai/adapters/openai.go`

**Files:**
- Create: `internal/ai/openai.go` (keep in same package for simplicity)
- Test: `internal/ai/openai_test.go`

**Step 1: Write the test**

```go
// internal/ai/openai_test.go
package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("missing auth header")
		}
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "gpt-4o" {
			t.Fatalf("unexpected model: %v", body["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"role": "assistant", "content": "Hello!"}, "finish_reason": "stop"},
			},
			"usage": map[string]interface{}{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		})
	}))
	defer srv.Close()

	client := NewOpenAIClient("test-key", srv.URL, "")
	resp, err := client.Complete(context.Background(), CompletionRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "Hello!" {
		t.Fatalf("unexpected content: %s", resp.Content)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}
}

func TestOpenAIStreamComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		chunks := []string{
			`{"choices":[{"delta":{"content":"Hel"},"finish_reason":null}]}`,
			`{"choices":[{"delta":{"content":"lo!"},"finish_reason":null}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		}
		for _, c := range chunks {
			w.Write([]byte("data: " + c + "\n\n"))
			flusher.Flush()
		}
		w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	client := NewOpenAIClient("test-key", srv.URL, "")
	var collected string
	var done bool
	err := client.StreamComplete(context.Background(), CompletionRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: "user", Content: "Hi"}},
		Stream:   true,
	}, func(chunk StreamChunk) {
		collected += chunk.Content
		if chunk.Done {
			done = true
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if collected != "Hello!" {
		t.Fatalf("unexpected collected: %s", collected)
	}
	if !done {
		t.Fatal("expected done=true")
	}
}

func TestOpenAIExtraHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "value" {
			t.Fatal("missing custom header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]interface{}{"role": "assistant", "content": "ok"}, "finish_reason": "stop"},
			},
			"usage": map[string]interface{}{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer srv.Close()

	headers := `{"X-Custom":"value"}`
	client := NewOpenAIClient("key", srv.URL, headers)
	_, err := client.Complete(context.Background(), CompletionRequest{
		Model:    "test",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
}
```

**Step 2: Run test — FAIL**

**Step 3: Write implementation**

```go
// internal/ai/openai.go
package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAIClient implements AIClient for all OpenAI-compatible APIs.
type OpenAIClient struct {
	apiKey       string
	baseURL      string
	extraHeaders map[string]string
	httpClient   *http.Client
}

// NewOpenAIClient creates an OpenAI-compatible client.
func NewOpenAIClient(apiKey, baseURL, extraHeadersJSON string) *OpenAIClient {
	headers := map[string]string{}
	if extraHeadersJSON != "" {
		_ = json.Unmarshal([]byte(extraHeadersJSON), &headers)
	}
	return &OpenAIClient{
		apiKey:       apiKey,
		baseURL:      strings.TrimRight(baseURL, "/"),
		extraHeaders: headers,
		httpClient:   &http.Client{Timeout: 5 * time.Minute},
	}
}

func (c *OpenAIClient) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	req.Stream = false
	body, err := json.Marshal(req)
	if err != nil {
		return CompletionResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return CompletionResponse{}, err
	}
	c.setHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return CompletionResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return CompletionResponse{}, fmt.Errorf("openai API error %d: %s", resp.StatusCode, string(b))
	}

	var oaiResp openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&oaiResp); err != nil {
		return CompletionResponse{}, err
	}

	return oaiResp.toCompletionResponse(), nil
}

func (c *OpenAIClient) StreamComplete(ctx context.Context, req CompletionRequest, onChunk func(StreamChunk)) error {
	req.Stream = true
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.setHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("openai API error %d: %s", resp.StatusCode, string(b))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			onChunk(StreamChunk{Done: true})
			return nil
		}

		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		sc := StreamChunk{}
		if len(chunk.Choices) > 0 {
			choice := chunk.Choices[0]
			sc.Content = choice.Delta.Content
			sc.FinishReason = stringVal(choice.FinishReason)
			if choice.FinishReason != nil && *choice.FinishReason == "stop" {
				sc.Done = true
			}
			if len(choice.Delta.ToolCalls) > 0 {
				sc.ToolCalls = choice.Delta.ToolCalls
			}
		}
		onChunk(sc)
	}

	return scanner.Err()
}

func (c *OpenAIClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	for k, v := range c.extraHeaders {
		req.Header.Set(k, v)
	}
}

// Internal response types matching OpenAI API shape.

type openAIResponse struct {
	Choices []openAIChoice `json:"choices"`
	Usage   openAIUsage    `json:"usage"`
}

type openAIChoice struct {
	Message      openAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type openAIMessage struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openAIStreamChunk struct {
	Choices []openAIStreamChoice `json:"choices"`
}

type openAIStreamChoice struct {
	Delta        openAIDelta `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

type openAIDelta struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

func (r openAIResponse) toCompletionResponse() CompletionResponse {
	cr := CompletionResponse{
		Usage: Usage{
			PromptTokens:     r.Usage.PromptTokens,
			CompletionTokens: r.Usage.CompletionTokens,
			TotalTokens:      r.Usage.TotalTokens,
		},
	}
	if len(r.Choices) > 0 {
		cr.Content = r.Choices[0].Message.Content
		cr.ToolCalls = r.Choices[0].Message.ToolCalls
		cr.FinishReason = r.Choices[0].FinishReason
	}
	return cr
}

func stringVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
```

**Step 4: Run tests**

Run: `cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes && go test ./internal/ai/... -run TestOpenAI -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/ai/openai.go internal/ai/openai_test.go
git commit -m "feat(ai): add OpenAI-compatible adapter with streaming support"
```

---

## Task 5: Anthropic Adapter — `internal/ai/anthropic.go`

**Files:**
- Create: `internal/ai/anthropic.go`
- Test: `internal/ai/anthropic_test.go`

**Step 1: Write the test**

```go
// internal/ai/anthropic_test.go
package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Fatal("missing x-api-key header")
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Fatal("missing anthropic-version header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": "Hello from Claude!"},
			},
			"stop_reason": "end_turn",
			"usage":       map[string]interface{}{"input_tokens": 10, "output_tokens": 5},
		})
	}))
	defer srv.Close()

	client := NewAnthropicClient("test-key", srv.URL)
	resp, err := client.Complete(context.Background(), CompletionRequest{
		Model:    "claude-sonnet-4-6",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "Hello from Claude!" {
		t.Fatalf("unexpected: %s", resp.Content)
	}
}
```

**Step 2: Run test — FAIL**

**Step 3: Write implementation**

```go
// internal/ai/anthropic.go
package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type AnthropicClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewAnthropicClient(apiKey, baseURL string) *AnthropicClient {
	return &AnthropicClient{
		apiKey:     apiKey,
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 5 * time.Minute},
	}
}

func (c *AnthropicClient) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	anthReq := c.toAnthropicRequest(req)
	anthReq.Stream = false

	body, err := json.Marshal(anthReq)
	if err != nil {
		return CompletionResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return CompletionResponse{}, err
	}
	c.setHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return CompletionResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return CompletionResponse{}, fmt.Errorf("anthropic API error %d: %s", resp.StatusCode, string(b))
	}

	var anthResp anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthResp); err != nil {
		return CompletionResponse{}, err
	}

	return anthResp.toCompletionResponse(), nil
}

func (c *AnthropicClient) StreamComplete(ctx context.Context, req CompletionRequest, onChunk func(StreamChunk)) error {
	anthReq := c.toAnthropicRequest(req)
	anthReq.Stream = true

	body, err := json.Marshal(anthReq)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.setHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("anthropic API error %d: %s", resp.StatusCode, string(b))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var event map[string]interface{}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		eventType, _ := event["type"].(string)
		switch eventType {
		case "content_block_delta":
			delta, _ := event["delta"].(map[string]interface{})
			if text, ok := delta["text"].(string); ok {
				onChunk(StreamChunk{Content: text})
			}
		case "message_delta":
			onChunk(StreamChunk{Done: true, FinishReason: "stop"})
		case "message_stop":
			onChunk(StreamChunk{Done: true})
			return nil
		}
	}
	return scanner.Err()
}

func (c *AnthropicClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
}

func (c *AnthropicClient) toAnthropicRequest(req CompletionRequest) anthropicRequest {
	ar := anthropicRequest{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
	}
	if ar.MaxTokens == 0 {
		ar.MaxTokens = 4096
	}
	if req.Temperature != nil {
		ar.Temperature = req.Temperature
	}

	for _, m := range req.Messages {
		if m.Role == RoleSystem {
			ar.System = m.Content
			continue
		}
		ar.Messages = append(ar.Messages, anthropicMessage{Role: m.Role, Content: m.Content})
	}

	if len(req.Tools) > 0 {
		for _, t := range req.Tools {
			ar.Tools = append(ar.Tools, anthropicTool{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				InputSchema: t.Function.Parameters,
			})
		}
	}

	return ar
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
	Tools       []anthropicTool    `json:"tools,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
}

type anthropicResponse struct {
	Content    []anthropicContent `json:"content"`
	StopReason string             `json:"stop_reason"`
	Usage      anthropicUsage     `json:"usage"`
}

type anthropicContent struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func (r anthropicResponse) toCompletionResponse() CompletionResponse {
	cr := CompletionResponse{
		FinishReason: r.StopReason,
		Usage: Usage{
			PromptTokens:     r.Usage.InputTokens,
			CompletionTokens: r.Usage.OutputTokens,
			TotalTokens:      r.Usage.InputTokens + r.Usage.OutputTokens,
		},
	}
	var toolCalls []ToolCall
	for _, c := range r.Content {
		switch c.Type {
		case "text":
			cr.Content += c.Text
		case "tool_use":
			toolCalls = append(toolCalls, ToolCall{
				ID:   c.ID,
				Type: "function",
				Function: ToolCallFunc{
					Name:      c.Name,
					Arguments: string(c.Input),
				},
			})
		}
	}
	cr.ToolCalls = toolCalls
	return cr
}
```

**Step 4: Run tests**

Run: `cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes && go test ./internal/ai/... -run TestAnthropic -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/ai/anthropic.go internal/ai/anthropic_test.go
git commit -m "feat(ai): add Anthropic Messages API adapter"
```

---

## Task 6: Google Gemini Adapter — `internal/ai/google.go`

**Files:**
- Create: `internal/ai/google.go`
- Test: `internal/ai/google_test.go`

**Step 1: Write the test**

```go
// internal/ai/google_test.go
package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGoogleComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "test-key" {
			t.Fatal("missing API key in query")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"parts": []map[string]interface{}{{"text": "Hello from Gemini!"}},
						"role":  "model",
					},
					"finishReason": "STOP",
				},
			},
			"usageMetadata": map[string]interface{}{"promptTokenCount": 10, "candidatesTokenCount": 5, "totalTokenCount": 15},
		})
	}))
	defer srv.Close()

	client := NewGoogleClient("test-key", srv.URL)
	resp, err := client.Complete(context.Background(), CompletionRequest{
		Model:    "gemini-2.0-flash",
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "Hello from Gemini!" {
		t.Fatalf("unexpected: %s", resp.Content)
	}
}
```

**Step 2: Run test — FAIL**

**Step 3: Write implementation**

```go
// internal/ai/google.go
package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type GoogleClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewGoogleClient(apiKey, baseURL string) *GoogleClient {
	return &GoogleClient{
		apiKey:     apiKey,
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 5 * time.Minute},
	}
}

func (c *GoogleClient) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	gReq := c.toGoogleRequest(req)
	body, err := json.Marshal(gReq)
	if err != nil {
		return CompletionResponse{}, err
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", c.baseURL, req.Model, c.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return CompletionResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return CompletionResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return CompletionResponse{}, fmt.Errorf("google API error %d: %s", resp.StatusCode, string(b))
	}

	var gResp googleResponse
	if err := json.NewDecoder(resp.Body).Decode(&gResp); err != nil {
		return CompletionResponse{}, err
	}

	return gResp.toCompletionResponse(), nil
}

func (c *GoogleClient) StreamComplete(ctx context.Context, req CompletionRequest, onChunk func(StreamChunk)) error {
	gReq := c.toGoogleRequest(req)
	body, err := json.Marshal(gReq)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse&key=%s", c.baseURL, req.Model, c.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("google API error %d: %s", resp.StatusCode, string(b))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var chunk googleResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		sc := StreamChunk{}
		if len(chunk.Candidates) > 0 && len(chunk.Candidates[0].Content.Parts) > 0 {
			sc.Content = chunk.Candidates[0].Content.Parts[0].Text
			if chunk.Candidates[0].FinishReason == "STOP" {
				sc.Done = true
			}
		}
		onChunk(sc)
	}
	return scanner.Err()
}

func (c *GoogleClient) toGoogleRequest(req CompletionRequest) googleRequest {
	gr := googleRequest{}
	for _, m := range req.Messages {
		if m.Role == RoleSystem {
			gr.SystemInstruction = &googleContent{
				Parts: []googlePart{{Text: m.Content}},
			}
			continue
		}
		role := "user"
		if m.Role == RoleAssistant {
			role = "model"
		}
		gr.Contents = append(gr.Contents, googleContent{
			Role:  role,
			Parts: []googlePart{{Text: m.Content}},
		})
	}
	gr.GenerationConfig = &googleGenConfig{}
	if req.MaxTokens > 0 {
		gr.GenerationConfig.MaxOutputTokens = req.MaxTokens
	}
	if req.Temperature != nil {
		gr.GenerationConfig.Temperature = req.Temperature
	}
	return gr
}

type googleRequest struct {
	Contents          []googleContent  `json:"contents"`
	SystemInstruction *googleContent   `json:"systemInstruction,omitempty"`
	GenerationConfig  *googleGenConfig `json:"generationConfig,omitempty"`
}

type googleContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []googlePart `json:"parts"`
}

type googlePart struct {
	Text string `json:"text,omitempty"`
}

type googleGenConfig struct {
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`
}

type googleResponse struct {
	Candidates    []googleCandidate `json:"candidates"`
	UsageMetadata *googleUsage      `json:"usageMetadata,omitempty"`
}

type googleCandidate struct {
	Content      googleContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type googleUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

func (r googleResponse) toCompletionResponse() CompletionResponse {
	cr := CompletionResponse{}
	if len(r.Candidates) > 0 {
		c := r.Candidates[0]
		for _, p := range c.Content.Parts {
			cr.Content += p.Text
		}
		cr.FinishReason = c.FinishReason
	}
	if r.UsageMetadata != nil {
		cr.Usage = Usage{
			PromptTokens:     r.UsageMetadata.PromptTokenCount,
			CompletionTokens: r.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      r.UsageMetadata.TotalTokenCount,
		}
	}
	return cr
}
```

**Step 4: Run tests, Step 5: Commit**

```bash
git add internal/ai/google.go internal/ai/google_test.go
git commit -m "feat(ai): add Google Gemini GenerateContent adapter"
```

---

## Task 7: AWS Bedrock Adapter (Stub) — `internal/ai/bedrock.go`

**Files:**
- Create: `internal/ai/bedrock.go`

This is a minimal stub that routes Bedrock calls through the OpenAI-compatible Converse API. Full SigV4 support is deferred since most users will use direct provider keys or gateways.

```go
// internal/ai/bedrock.go
package ai

// NewBedrockClient creates a Bedrock client. For now, it wraps OpenAI-compat
// since AWS Bedrock supports the /v1/converse endpoint for most models.
// Full SigV4 auth can be added later.
func NewBedrockClient(apiKey, baseURL, extraHeaders string) AIClient {
	if baseURL == "" {
		baseURL = "https://bedrock-runtime.us-east-1.amazonaws.com"
	}
	return NewOpenAIClient(apiKey, baseURL, extraHeaders)
}
```

**Commit:**

```bash
git add internal/ai/bedrock.go
git commit -m "feat(ai): add Bedrock adapter stub (delegates to OpenAI-compat)"
```

---

## Task 8: Fix client.go `fmt` import + `NewClient` wiring

**Files:**
- Modify: `internal/ai/client.go`

Add the missing `fmt` import and ensure `NewClient` compiles with all adapter constructors now in place.

```go
// Add to imports:
import (
	"context"
	"fmt"
)
```

**Run:** `cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes && go build ./internal/ai/...`
Expected: compiles clean

**Commit:**

```bash
git add internal/ai/client.go
git commit -m "fix(ai): add fmt import to client.go for NewClient error handling"
```

---

## Task 9: Wails Provider Functions — `wails-app/app.go`

**Files:**
- Modify: `wails-app/app.go` — add AI store field + 5 bound functions
- Modify: `wails-app/frontend/src/wailsjs/go/main/App.js` — add JS bindings
- Modify: `wails-app/frontend/src/services/api.js` — add api wrappers

**Step 1: Add to `wails-app/app.go`**

In the `App` struct, add:
```go
aiStore *ai.AIStore
```

In `startup()`, after `a.connMgr = mgr`:
```go
aiStore, err := ai.NewAIStore(db)
if err != nil {
	fmt.Printf("ai store init error: %v\n", err)
} else {
	a.aiStore = aiStore
}
```

Add import: `"github.com/monoes/mono-agent/internal/ai"`

Add new bound functions:

```go
// ─────────────────────────────────────────────────────────────────────────────
// AI Providers
// ─────────────────────────────────────────────────────────────────────────────

func (a *App) ListAIProviders() string {
	providers, err := a.aiStore.ListProviders()
	if err != nil {
		return jsonError(err)
	}
	b, _ := json.Marshal(providers)
	return string(b)
}

func (a *App) SaveAIProvider(providerJSON string) string {
	var p ai.AIProvider
	if err := json.Unmarshal([]byte(providerJSON), &p); err != nil {
		return jsonError(err)
	}
	if p.ID == "" {
		p.ID = newUUID()
	}
	if err := a.aiStore.SaveProvider(p); err != nil {
		return jsonError(err)
	}
	b, _ := json.Marshal(p)
	return string(b)
}

func (a *App) DeleteAIProvider(id string) string {
	if err := a.aiStore.DeleteProvider(id); err != nil {
		return jsonError(err)
	}
	return `{"ok":true}`
}

func (a *App) TestAIProvider(id string) string {
	p, err := a.aiStore.GetProvider(id)
	if err != nil {
		return jsonError(err)
	}
	client, err := ai.NewClient(p)
	if err != nil {
		return jsonError(err)
	}

	model := p.DefaultModel
	if model == "" {
		def, ok := ai.GetProviderDef(p.ProviderID)
		if ok && len(def.Models) > 0 {
			model = def.Models[0].ID
		} else {
			model = "gpt-4o-mini"
		}
	}

	_, err = client.Complete(context.Background(), ai.CompletionRequest{
		Model:     model,
		Messages:  []ai.Message{{Role: "user", Content: "Say ok"}},
		MaxTokens: 5,
	})

	status := "active"
	if err != nil {
		status = "error"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_ = a.aiStore.UpdateProviderStatus(id, status, now)

	if err != nil {
		return fmt.Sprintf(`{"status":"error","error":%q}`, err.Error())
	}
	return `{"status":"active"}`
}

func (a *App) GetAIModels(providerID string) string {
	def, ok := ai.GetProviderDef(providerID)
	if !ok {
		return "[]"
	}
	b, _ := json.Marshal(def.Models)
	return string(b)
}

func (a *App) GetAIRegistry() string {
	b, _ := json.Marshal(ai.ProviderRegistry)
	return string(b)
}

func jsonError(err error) string {
	return fmt.Sprintf(`{"error":%q}`, err.Error())
}
```

**Step 2: Add JS bindings to `App.js`**

```js
export function ListAIProviders() {
  return window['go']['main']['App']['ListAIProviders']();
}
export function SaveAIProvider(arg1) {
  return window['go']['main']['App']['SaveAIProvider'](arg1);
}
export function DeleteAIProvider(arg1) {
  return window['go']['main']['App']['DeleteAIProvider'](arg1);
}
export function TestAIProvider(arg1) {
  return window['go']['main']['App']['TestAIProvider'](arg1);
}
export function GetAIModels(arg1) {
  return window['go']['main']['App']['GetAIModels'](arg1);
}
export function GetAIRegistry() {
  return window['go']['main']['App']['GetAIRegistry']();
}
```

**Step 3: Add to `api.js`**

```js
  listAIProviders:    () => GoApp.ListAIProviders().then(s => JSON.parse(s)).catch(() => []),
  saveAIProvider:     (provider) => GoApp.SaveAIProvider(JSON.stringify(provider)).then(s => JSON.parse(s)),
  deleteAIProvider:   (id) => GoApp.DeleteAIProvider(id).then(s => JSON.parse(s)),
  testAIProvider:     (id) => GoApp.TestAIProvider(id).then(s => JSON.parse(s)),
  getAIModels:        (providerID) => GoApp.GetAIModels(providerID).then(s => JSON.parse(s)).catch(() => []),
  getAIRegistry:      () => GoApp.GetAIRegistry().then(s => JSON.parse(s)).catch(() => []),
```

Add to imports: `GetAIRegistry, ListAIProviders, SaveAIProvider, DeleteAIProvider, TestAIProvider, GetAIModels`

**Step 4: Build**

Run: `cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes/wails-app && go build .`
Expected: compiles

**Step 5: Commit**

```bash
git add wails-app/app.go wails-app/frontend/src/wailsjs/go/main/App.js wails-app/frontend/src/services/api.js
git commit -m "feat(ai): add Wails-bound AI provider management functions"
```

---

## Task 10: Frontend — AI Providers Page

**Files:**
- Create: `wails-app/frontend/src/pages/AIProviders.jsx`
- Modify: `wails-app/frontend/src/App.jsx` — add page route
- Modify: `wails-app/frontend/src/components/Sidebar.jsx` — add nav item

**Step 1: Add nav item in `Sidebar.jsx`**

Add import: `Brain` from `lucide-react`

Add to `NAV_ITEMS` array after the `connections` entry:
```js
{ id: 'ai', label: 'AI', icon: Brain, section: 'DATA' },
```

**Step 2: Add page route in `App.jsx`**

Add import:
```js
import AIProviders from './pages/AIProviders.jsx'
```

Add to `pages` object:
```js
ai: <AIProviders />,
```

**Step 3: Create `AIProviders.jsx`**

This page follows the same tile-grid pattern as `Connections.jsx`. It shows:
- Connected AI providers as tiles (emoji, name, status dot, model)
- "Add Provider" button opens a modal with:
  - Step 1: Pick provider from categorized grid (searchable)
  - Step 2: Enter API key (+ base URL for gateway tier)
  - Step 3: Pick model from combobox (curated + free-text)
  - Test Connection button
- Click existing tile opens manage modal (update key, model, test, delete)

The full JSX component should be ~400 lines following the exact patterns from `Connections.jsx`:
- Same `useState` pattern for modals, loading, etc.
- Same tile grid CSS classes
- Same modal structure
- Provider categories grouped: Frontier, Cloud, Inference, Gateway
- Combobox = `<input>` with `<datalist>` for model selection
- Status dot: green=active, red=error, gray=untested

**Step 4: Build and verify**

Run: `cd /Users/morteza/Desktop/monoes/mono-agent/newmonoes/wails-app && make run`
Expected: AI nav item appears, page loads, add provider flow works

**Step 5: Commit**

```bash
git add wails-app/frontend/src/pages/AIProviders.jsx wails-app/frontend/src/App.jsx wails-app/frontend/src/components/Sidebar.jsx
git commit -m "feat(ai): add AI Providers page with add/manage/test flow"
```

---

## Task 11: Chat Canvas Tools — `internal/ai/chat/tools.go`

**Files:**
- Create: `internal/ai/chat/tools.go`
- Test: `internal/ai/chat/tools_test.go`

Implements the 10 canvas manipulation tools from the design doc. Each tool is a function that takes JSON args and returns JSON result. Tools read/write the workflow DB via `workflow.WorkflowStore`.

Key tools: `get_workflow_state`, `create_nodes`, `update_node_config`, `delete_nodes`, `connect_nodes`, `disconnect_nodes`, `list_available_nodes`, `list_connections`, `run_workflow`, `get_execution_result`.

Each tool returns `ToolDef` for the AI request and has an `Execute(args string) (string, error)` method.

**Commit:**
```bash
git add internal/ai/chat/tools.go internal/ai/chat/tools_test.go
git commit -m "feat(ai): add canvas manipulation tools for AI chat"
```

---

## Task 12: Chat Service — `internal/ai/chat/service.go`

**Files:**
- Create: `internal/ai/chat/service.go`
- Test: `internal/ai/chat/service_test.go`

The chat service:
1. Builds system prompt with workflow context
2. Loads chat history from store
3. Appends user message
4. Sends to AI client with tool definitions
5. Streams response via callback
6. When tool calls come in, executes them and feeds results back
7. Saves all messages to store

**Commit:**
```bash
git add internal/ai/chat/service.go internal/ai/chat/service_test.go
git commit -m "feat(ai): add chat service with streaming and tool dispatch"
```

---

## Task 13: Wails Chat Functions — `wails-app/app.go`

**Files:**
- Modify: `wails-app/app.go` — add `StreamAIChat`, `GetAIChatHistory`, `ClearAIChatHistory`
- Modify: `wails-app/frontend/src/wailsjs/go/main/App.js`
- Modify: `wails-app/frontend/src/services/api.js`

`StreamAIChat` runs in a goroutine, emitting `ai:chunk`, `ai:tool`, `ai:error` events via `runtime.EventsEmit`. Returns immediately with `{"ok":true}`.

**Commit:**
```bash
git add wails-app/app.go wails-app/frontend/src/wailsjs/go/main/App.js wails-app/frontend/src/services/api.js
git commit -m "feat(ai): add Wails-bound chat streaming functions"
```

---

## Task 14: Frontend — AI Chat Panel

**Files:**
- Create: `wails-app/frontend/src/components/AIChatPanel.jsx`
- Modify: `wails-app/frontend/src/pages/NodeRunner.jsx` — add chat toggle + panel

Collapsible right panel (340px default) in the Workflow builder page. Features:
- Provider + model dropdowns (filtered to configured providers)
- Message bubbles with streaming text accumulation
- Tool call results as collapsible cards
- Input box with send button
- Clear history button
- Listens to `ai:chunk`, `ai:tool`, `ai:error` events

**Commit:**
```bash
git add wails-app/frontend/src/components/AIChatPanel.jsx wails-app/frontend/src/pages/NodeRunner.jsx
git commit -m "feat(ai): add AI Chat Panel sidebar in Workflow builder"
```

---

## Task 15: AI Node Executors — `internal/ai/nodes/`

**Files:**
- Create: `internal/ai/nodes/chat.go`
- Create: `internal/ai/nodes/extract.go`
- Create: `internal/ai/nodes/classify.go`
- Create: `internal/ai/nodes/transform.go`
- Create: `internal/ai/nodes/embed.go`
- Create: `internal/ai/nodes/agent.go`
- Test: `internal/ai/nodes/chat_test.go`

Each implements `workflow.NodeExecutor`. Config includes `provider_id`, `model`, and node-specific fields. The executor:
1. Looks up the AI provider from the store
2. Creates an AI client
3. Builds the prompt from config templates + input items
4. Calls the AI API
5. Returns items with AI response fields added

`ai.classify` is special — it has one output handle per configured category plus a "main" handle.

**Commit:**
```bash
git add internal/ai/nodes/
git commit -m "feat(ai): add 6 AI node executors (chat, extract, classify, transform, embed, agent)"
```

---

## Task 16: Wire AI Nodes into Workflow Engine + Frontend Palette

**Files:**
- Modify: `wails-app/app.go` — register AI node factories in the workflow engine's registry
- Modify: Frontend node palette — add "AI" category with 6 node types
- Modify: Frontend node config panels — add provider/model dropdowns to AI node configs

In `app.go` where the node registry is initialized, add:
```go
import ainodes "github.com/monoes/mono-agent/internal/ai/nodes"

registry.Register("ai.chat", func() workflow.NodeExecutor { return &ainodes.ChatNode{AIStore: a.aiStore} })
registry.Register("ai.extract", func() workflow.NodeExecutor { return &ainodes.ExtractNode{AIStore: a.aiStore} })
registry.Register("ai.classify", func() workflow.NodeExecutor { return &ainodes.ClassifyNode{AIStore: a.aiStore} })
registry.Register("ai.transform", func() workflow.NodeExecutor { return &ainodes.TransformNode{AIStore: a.aiStore} })
registry.Register("ai.embed", func() workflow.NodeExecutor { return &ainodes.EmbedNode{AIStore: a.aiStore} })
registry.Register("ai.agent", func() workflow.NodeExecutor { return &ainodes.AgentNode{AIStore: a.aiStore} })
```

**Commit:**
```bash
git add wails-app/app.go wails-app/frontend/src/
git commit -m "feat(ai): wire AI nodes into workflow engine and add to frontend palette"
```

---

## Execution Order

Tasks 1-8 are sequential (each depends on the previous).
Task 9 depends on Task 8.
Task 10 depends on Task 9.
Tasks 11-12 depend on Tasks 1-4 (can start after Task 4).
Task 13 depends on Task 12.
Task 14 depends on Tasks 10 + 13.
Task 15 depends on Tasks 1-4.
Task 16 depends on Tasks 15 + 10.

Parallelization opportunity: after Task 4, Tasks 5-7 can run in parallel. Tasks 11-12 can run in parallel with Tasks 9-10.
