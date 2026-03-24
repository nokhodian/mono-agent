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

// OpenAIClient implements AIClient for OpenAI-compatible APIs.
type OpenAIClient struct {
	apiKey       string
	baseURL      string
	extraHeaders map[string]string
	httpClient   *http.Client
}

// NewOpenAIClient creates an OpenAIClient. extraHeadersJSON is a JSON-encoded
// map[string]string (or empty string for no extra headers).
func NewOpenAIClient(apiKey, baseURL, extraHeadersJSON string) *OpenAIClient {
	baseURL = strings.TrimRight(baseURL, "/")
	headers := make(map[string]string)
	if extraHeadersJSON != "" {
		_ = json.Unmarshal([]byte(extraHeadersJSON), &headers)
	}
	return &OpenAIClient{
		apiKey:       apiKey,
		baseURL:      baseURL,
		extraHeaders: headers,
		httpClient:   &http.Client{Timeout: 5 * time.Minute},
	}
}

// ── OpenAI wire types ───────────────────────────────────────────────────────

type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMsg     `json:"messages"`
	Tools       []ToolDef       `json:"tools,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
}

type openAIMsg struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

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
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ── Complete ────────────────────────────────────────────────────────────────

func (c *OpenAIClient) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	oReq := c.toWireRequest(req, false)
	body, err := json.Marshal(oReq)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
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
		return CompletionResponse{}, fmt.Errorf("openai: status %d: %s", resp.StatusCode, string(respBody))
	}

	var oResp openAIResponse
	if err := json.Unmarshal(respBody, &oResp); err != nil {
		return CompletionResponse{}, fmt.Errorf("unmarshal response: %w", err)
	}

	return c.fromWireResponse(oResp), nil
}

// ── StreamComplete ──────────────────────────────────────────────────────────

func (c *OpenAIClient) StreamComplete(ctx context.Context, req CompletionRequest, onChunk func(StreamChunk)) error {
	oReq := c.toWireRequest(req, true)
	body, err := json.Marshal(oReq)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
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
		return fmt.Errorf("openai: status %d: %s", resp.StatusCode, string(respBody))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
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
			continue // skip malformed chunks
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]
		sc := StreamChunk{
			Content:   choice.Delta.Content,
			ToolCalls: choice.Delta.ToolCalls,
		}
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			sc.FinishReason = *choice.FinishReason
			sc.Done = true
		}
		onChunk(sc)
		if sc.Done {
			return nil
		}
	}
	return scanner.Err()
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func (c *OpenAIClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	for k, v := range c.extraHeaders {
		req.Header.Set(k, v)
	}
}

func (c *OpenAIClient) toWireRequest(req CompletionRequest, stream bool) openAIRequest {
	msgs := make([]openAIMsg, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = openAIMsg{
			Role:       m.Role,
			Content:    m.Content,
			ToolCalls:  m.ToolCalls,
			ToolCallID: m.ToolCallID,
		}
	}
	return openAIRequest{
		Model:       req.Model,
		Messages:    msgs,
		Tools:       req.Tools,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      stream,
	}
}

func (c *OpenAIClient) fromWireResponse(resp openAIResponse) CompletionResponse {
	cr := CompletionResponse{
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}
	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		cr.Content = choice.Message.Content
		cr.ToolCalls = choice.Message.ToolCalls
		cr.FinishReason = choice.FinishReason
	}
	return cr
}
