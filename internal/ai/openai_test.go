package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			t.Errorf("Authorization = %q, want %q", auth, "Bearer test-key")
		}

		// Verify request body has correct model
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]interface{}
		if err := json.Unmarshal(body, &reqBody); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}
		if reqBody["model"] != "gpt-4o" {
			t.Errorf("model = %v, want %q", reqBody["model"], "gpt-4o")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Hello, world!",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		})
	}))
	defer srv.Close()

	client := NewOpenAIClient("test-key", srv.URL, "")
	resp, err := client.Complete(context.Background(), CompletionRequest{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: RoleUser, Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if resp.Content != "Hello, world!" {
		t.Errorf("Content = %q, want %q", resp.Content, "Hello, world!")
	}
	if resp.FinishReason != "stop" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "stop")
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("PromptTokens = %d, want 10", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 5 {
		t.Errorf("CompletionTokens = %d, want 5", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("TotalTokens = %d, want 15", resp.Usage.TotalTokens)
	}
}

func TestOpenAIStreamComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected http.Flusher")
		}

		chunks := []string{
			`data: {"choices":[{"delta":{"content":"Hel"},"finish_reason":null}]}`,
			`data: {"choices":[{"delta":{"content":"lo!"},"finish_reason":null}]}`,
			`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		}
		for _, chunk := range chunks {
			w.Write([]byte(chunk + "\n\n"))
			flusher.Flush()
		}
	}))
	defer srv.Close()

	client := NewOpenAIClient("test-key", srv.URL, "")

	var accumulated string
	var gotDone bool

	err := client.StreamComplete(context.Background(), CompletionRequest{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: RoleUser, Content: "Hi"},
		},
	}, func(chunk StreamChunk) {
		accumulated += chunk.Content
		if chunk.Done {
			gotDone = true
		}
	})
	if err != nil {
		t.Fatalf("StreamComplete: %v", err)
	}

	if accumulated != "Hello!" {
		t.Errorf("accumulated = %q, want %q", accumulated, "Hello!")
	}
	if !gotDone {
		t.Error("expected Done=true chunk")
	}
}

func TestOpenAIExtraHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		custom := r.Header.Get("X-Custom")
		if custom != "value" {
			t.Errorf("X-Custom = %q, want %q", custom, "value")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message":       map[string]interface{}{"role": "assistant", "content": "ok"},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     1,
				"completion_tokens": 1,
				"total_tokens":      2,
			},
		})
	}))
	defer srv.Close()

	client := NewOpenAIClient("key", srv.URL, `{"X-Custom":"value"}`)
	_, err := client.Complete(context.Background(), CompletionRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: RoleUser, Content: "test"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
}

func TestOpenAIToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "",
						"tool_calls": []map[string]interface{}{
							{
								"id":   "call_abc123",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "get_weather",
									"arguments": `{"location":"London"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     20,
				"completion_tokens": 10,
				"total_tokens":      30,
			},
		})
	}))
	defer srv.Close()

	client := NewOpenAIClient("key", srv.URL, "")
	resp, err := client.Complete(context.Background(), CompletionRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: RoleUser, Content: "What is the weather in London?"}},
		Tools: []ToolDef{
			{
				Type: "function",
				Function: ToolFunction{
					Name:        "get_weather",
					Description: "Get weather for a location",
					Parameters:  map[string]interface{}{"type": "object"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "call_abc123" {
		t.Errorf("ToolCall.ID = %q, want %q", tc.ID, "call_abc123")
	}
	if tc.Type != "function" {
		t.Errorf("ToolCall.Type = %q, want %q", tc.Type, "function")
	}
	if tc.Function.Name != "get_weather" {
		t.Errorf("ToolCall.Function.Name = %q, want %q", tc.Function.Name, "get_weather")
	}
	if tc.Function.Arguments != `{"location":"London"}` {
		t.Errorf("ToolCall.Function.Arguments = %q, want %q", tc.Function.Arguments, `{"location":"London"}`)
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "tool_calls")
	}
}

func TestNewClient(t *testing.T) {
	// Known provider (openai) — should succeed
	c, err := NewClient(AIProvider{
		ProviderID: "openai",
		APIKey:     "sk-test",
		Tier:       "known",
	})
	if err != nil {
		t.Fatalf("NewClient(openai): %v", err)
	}
	if c == nil {
		t.Fatal("NewClient(openai) returned nil")
	}

	// Unknown provider with gateway tier — should succeed
	c, err = NewClient(AIProvider{
		ProviderID: "custom-thing",
		APIKey:     "key",
		BaseURL:    "https://example.com/v1",
		Tier:       "gateway",
	})
	if err != nil {
		t.Fatalf("NewClient(gateway): %v", err)
	}
	if c == nil {
		t.Fatal("NewClient(gateway) returned nil")
	}

	// Unknown provider with known tier — should fail
	_, err = NewClient(AIProvider{
		ProviderID: "nonexistent",
		Tier:       "known",
	})
	if err == nil {
		t.Error("NewClient(unknown known) should return error")
	}
}
