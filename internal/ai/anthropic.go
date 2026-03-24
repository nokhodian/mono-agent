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

// AnthropicClient implements AIClient for Anthropic's Messages API.
type AnthropicClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewAnthropicClient creates an AnthropicClient.
func NewAnthropicClient(apiKey, baseURL string) *AnthropicClient {
	baseURL = strings.TrimRight(baseURL, "/")
	return &AnthropicClient{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 5 * time.Minute},
	}
}

// ── Anthropic wire types ──────────────────────────────────────────────────────

type anthropicRequest struct {
	Model       string            `json:"model"`
	Messages    []anthropicMsg    `json:"messages"`
	System      string            `json:"system,omitempty"`
	MaxTokens   int               `json:"max_tokens"`
	Temperature *float64          `json:"temperature,omitempty"`
	Tools       []anthropicTool   `json:"tools,omitempty"`
	Stream      bool              `json:"stream"`
}

type anthropicMsg struct {
	Role    string               `json:"role"`
	Content json.RawMessage      `json:"content"`
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
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// SSE event types for streaming
type anthropicStreamEvent struct {
	Type string `json:"type"`
}

type anthropicContentBlockStart struct {
	Type         string           `json:"type"`
	Index        int              `json:"index"`
	ContentBlock anthropicContent `json:"content_block"`
}

type anthropicContentBlockDelta struct {
	Type  string                `json:"type"`
	Index int                   `json:"index"`
	Delta anthropicDeltaContent `json:"delta"`
}

type anthropicDeltaContent struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

type anthropicMessageDelta struct {
	Type  string                     `json:"type"`
	Delta anthropicMessageDeltaInner `json:"delta"`
}

type anthropicMessageDeltaInner struct {
	StopReason string `json:"stop_reason,omitempty"`
}

// ── Complete ──────────────────────────────────────────────────────────────────

func (c *AnthropicClient) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	aReq := c.toWireRequest(req, false)
	body, err := json.Marshal(aReq)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return CompletionResponse{}, fmt.Errorf("anthropic: status %d: %s", resp.StatusCode, string(respBody))
	}

	var aResp anthropicResponse
	if err := json.Unmarshal(respBody, &aResp); err != nil {
		return CompletionResponse{}, fmt.Errorf("unmarshal response: %w", err)
	}

	return c.fromWireResponse(aResp), nil
}

// ── StreamComplete ────────────────────────────────────────────────────────────

func (c *AnthropicClient) StreamComplete(ctx context.Context, req CompletionRequest, onChunk func(StreamChunk)) error {
	aReq := c.toWireRequest(req, true)
	body, err := json.Marshal(aReq)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("anthropic: status %d: %s", resp.StatusCode, string(respBody))
	}

	// Track tool calls being built across streaming events
	type pendingToolCall struct {
		id        string
		name      string
		inputJSON strings.Builder
	}
	toolCalls := make(map[int]*pendingToolCall)

	scanner := bufio.NewScanner(resp.Body)
	var currentEvent string

	for scanner.Scan() {
		line := scanner.Text()

		// Parse event type line
		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
			continue
		}

		// Parse data line
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		switch currentEvent {
		case "content_block_start":
			var cbs anthropicContentBlockStart
			if err := json.Unmarshal([]byte(data), &cbs); err != nil {
				continue
			}
			if cbs.ContentBlock.Type == "tool_use" {
				toolCalls[cbs.Index] = &pendingToolCall{
					id:   cbs.ContentBlock.ID,
					name: cbs.ContentBlock.Name,
				}
			}

		case "content_block_delta":
			var cbd anthropicContentBlockDelta
			if err := json.Unmarshal([]byte(data), &cbd); err != nil {
				continue
			}
			switch cbd.Delta.Type {
			case "text_delta":
				onChunk(StreamChunk{Content: cbd.Delta.Text})
			case "input_json_delta":
				if tc, ok := toolCalls[cbd.Index]; ok {
					tc.inputJSON.WriteString(cbd.Delta.PartialJSON)
				}
			}

		case "message_delta":
			var md anthropicMessageDelta
			if err := json.Unmarshal([]byte(data), &md); err != nil {
				continue
			}
			// Emit any completed tool calls
			var tcs []ToolCall
			for _, tc := range toolCalls {
				tcs = append(tcs, ToolCall{
					ID:   tc.id,
					Type: "function",
					Function: ToolCallFunc{
						Name:      tc.name,
						Arguments: tc.inputJSON.String(),
					},
				})
			}
			sc := StreamChunk{
				ToolCalls:    tcs,
				FinishReason: md.Delta.StopReason,
				Done:         true,
			}
			onChunk(sc)

		case "message_stop":
			onChunk(StreamChunk{Done: true})
			return nil
		}
	}
	return scanner.Err()
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (c *AnthropicClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
}

func (c *AnthropicClient) toWireRequest(req CompletionRequest, stream bool) anthropicRequest {
	var system string
	var msgs []anthropicMsg

	for _, m := range req.Messages {
		if m.Role == RoleSystem {
			system = m.Content
			continue
		}

		// For tool role, Anthropic expects the role to be "user" with a tool_result content block,
		// but for simplicity we pass the content as a string.
		role := m.Role
		if role == RoleTool {
			role = RoleUser
		}

		contentBytes, _ := json.Marshal(m.Content)
		msgs = append(msgs, anthropicMsg{
			Role:    role,
			Content: contentBytes,
		})
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	// Convert tools from OpenAI format to Anthropic format
	var tools []anthropicTool
	for _, td := range req.Tools {
		tools = append(tools, anthropicTool{
			Name:        td.Function.Name,
			Description: td.Function.Description,
			InputSchema: td.Function.Parameters,
		})
	}

	return anthropicRequest{
		Model:       req.Model,
		Messages:    msgs,
		System:      system,
		MaxTokens:   maxTokens,
		Temperature: req.Temperature,
		Tools:       tools,
		Stream:      stream,
	}
}

func (c *AnthropicClient) fromWireResponse(resp anthropicResponse) CompletionResponse {
	cr := CompletionResponse{
		FinishReason: resp.StopReason,
		Usage: Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}

	var textParts []string
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_use":
			inputStr := ""
			if block.Input != nil {
				inputStr = string(block.Input)
			}
			cr.ToolCalls = append(cr.ToolCalls, ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: ToolCallFunc{
					Name:      block.Name,
					Arguments: inputStr,
				},
			})
		}
	}
	cr.Content = strings.Join(textParts, "")

	return cr
}
