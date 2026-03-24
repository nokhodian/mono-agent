package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Errorf("x-api-key = %q, want %q", got, "test-key")
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Errorf("anthropic-version = %q, want %q", got, "2023-06-01")
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want %q", got, "application/json")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": "Hello from Claude!"},
			},
			"stop_reason": "end_turn",
			"usage": map[string]interface{}{
				"input_tokens":  10,
				"output_tokens": 5,
			},
		})
	}))
	defer srv.Close()

	client := NewAnthropicClient("test-key", srv.URL)
	resp, err := client.Complete(context.Background(), CompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []Message{
			{Role: RoleUser, Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if resp.Content != "Hello from Claude!" {
		t.Errorf("Content = %q, want %q", resp.Content, "Hello from Claude!")
	}
	if resp.FinishReason != "end_turn" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "end_turn")
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

func TestAnthropicSystemMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]interface{}
		if err := json.Unmarshal(body, &reqBody); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}

		// Verify system field is present and correct
		system, ok := reqBody["system"]
		if !ok {
			t.Fatal("expected 'system' field in request body")
		}
		if system != "You are a helpful assistant." {
			t.Errorf("system = %q, want %q", system, "You are a helpful assistant.")
		}

		// Verify system message is NOT in the messages array
		messages, ok := reqBody["messages"].([]interface{})
		if !ok {
			t.Fatal("expected 'messages' to be an array")
		}
		for _, m := range messages {
			msg, ok := m.(map[string]interface{})
			if !ok {
				continue
			}
			if msg["role"] == "system" {
				t.Error("system message should not appear in messages array")
			}
		}

		// Verify only user message is in messages
		if len(messages) != 1 {
			t.Errorf("messages len = %d, want 1", len(messages))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": "I am helpful."},
			},
			"stop_reason": "end_turn",
			"usage": map[string]interface{}{
				"input_tokens":  15,
				"output_tokens": 3,
			},
		})
	}))
	defer srv.Close()

	client := NewAnthropicClient("test-key", srv.URL)
	resp, err := client.Complete(context.Background(), CompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []Message{
			{Role: RoleSystem, Content: "You are a helpful assistant."},
			{Role: RoleUser, Content: "Hello"},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if resp.Content != "I am helpful." {
		t.Errorf("Content = %q, want %q", resp.Content, "I am helpful.")
	}
}

func TestAnthropicToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type":  "tool_use",
					"id":    "tc1",
					"name":  "get_weather",
					"input": map[string]interface{}{"city": "SF"},
				},
			},
			"stop_reason": "tool_use",
			"usage": map[string]interface{}{
				"input_tokens":  20,
				"output_tokens": 10,
			},
		})
	}))
	defer srv.Close()

	client := NewAnthropicClient("test-key", srv.URL)
	resp, err := client.Complete(context.Background(), CompletionRequest{
		Model: "claude-sonnet-4-6",
		Messages: []Message{
			{Role: RoleUser, Content: "What is the weather in SF?"},
		},
		Tools: []ToolDef{
			{
				Type: "function",
				Function: ToolFunction{
					Name:        "get_weather",
					Description: "Get weather for a city",
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"city": map[string]interface{}{"type": "string"},
						},
					},
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
	if tc.ID != "tc1" {
		t.Errorf("ToolCall.ID = %q, want %q", tc.ID, "tc1")
	}
	if tc.Type != "function" {
		t.Errorf("ToolCall.Type = %q, want %q", tc.Type, "function")
	}
	if tc.Function.Name != "get_weather" {
		t.Errorf("ToolCall.Function.Name = %q, want %q", tc.Function.Name, "get_weather")
	}

	// Parse the arguments to verify content (order may vary in JSON)
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		t.Fatalf("parse ToolCall arguments: %v", err)
	}
	if args["city"] != "SF" {
		t.Errorf("ToolCall args city = %v, want %q", args["city"], "SF")
	}

	if resp.FinishReason != "tool_use" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "tool_use")
	}
}

func TestAnthropicStreamComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected http.Flusher")
		}

		events := []string{
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hel\"}}\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"lo!\"}}\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"}}\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n",
		}
		for _, ev := range events {
			fmt.Fprint(w, ev+"\n")
			flusher.Flush()
		}
	}))
	defer srv.Close()

	client := NewAnthropicClient("test-key", srv.URL)

	var accumulated string
	var gotDone bool

	err := client.StreamComplete(context.Background(), CompletionRequest{
		Model: "claude-sonnet-4-6",
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
