package ai

import (
	"context"
	"fmt"
)

// Role constants
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

type CompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []ToolDef `json:"tools,omitempty"`
	Temperature *float64  `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolDef struct {
	Type     string       `json:"type"` // "function"
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function"
	Function ToolCallFunc `json:"function"`
}

type ToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type CompletionResponse struct {
	Content      string     `json:"content"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string     `json:"finish_reason"`
	Usage        Usage      `json:"usage"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type StreamChunk struct {
	Content      string     `json:"content,omitempty"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string     `json:"finish_reason,omitempty"`
	Done         bool       `json:"done"`
}

type AIClient interface {
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
	StreamComplete(ctx context.Context, req CompletionRequest, onChunk func(StreamChunk)) error
}

// NewClient creates the appropriate AIClient for a given provider config.
func NewClient(provider AIProvider) (AIClient, error) {
	def, ok := GetProviderDef(provider.ProviderID)
	if !ok {
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
