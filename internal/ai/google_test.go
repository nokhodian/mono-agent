package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGoogleComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify API key in query param
		key := r.URL.Query().Get("key")
		if key != "test-key" {
			t.Errorf("key = %q, want %q", key, "test-key")
		}

		// Verify path contains model and action
		if r.URL.Path != "/models/gemini-2.0-flash:generateContent" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/models/gemini-2.0-flash:generateContent")
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"Hello from Gemini!"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`))
	}))
	defer srv.Close()

	client := NewGoogleClient("test-key", srv.URL)
	resp, err := client.Complete(context.Background(), CompletionRequest{
		Model: "gemini-2.0-flash",
		Messages: []Message{
			{Role: RoleUser, Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if resp.Content != "Hello from Gemini!" {
		t.Errorf("Content = %q, want %q", resp.Content, "Hello from Gemini!")
	}
	if resp.FinishReason != "STOP" {
		t.Errorf("FinishReason = %q, want %q", resp.FinishReason, "STOP")
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

func TestGoogleSystemMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var reqBody map[string]interface{}
		if err := json.Unmarshal(body, &reqBody); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}

		// Verify systemInstruction is present
		si, ok := reqBody["systemInstruction"]
		if !ok {
			t.Fatal("expected systemInstruction in request body")
		}
		siMap, ok := si.(map[string]interface{})
		if !ok {
			t.Fatal("systemInstruction is not an object")
		}
		parts, ok := siMap["parts"].([]interface{})
		if !ok || len(parts) == 0 {
			t.Fatal("systemInstruction has no parts")
		}
		part0, ok := parts[0].(map[string]interface{})
		if !ok {
			t.Fatal("systemInstruction part is not an object")
		}
		if part0["text"] != "You are helpful." {
			t.Errorf("systemInstruction text = %q, want %q", part0["text"], "You are helpful.")
		}

		// Verify contents only has user messages (not system)
		contents, ok := reqBody["contents"].([]interface{})
		if !ok {
			t.Fatal("contents is not an array")
		}
		if len(contents) != 1 {
			t.Fatalf("contents len = %d, want 1", len(contents))
		}
		c0, ok := contents[0].(map[string]interface{})
		if !ok {
			t.Fatal("contents[0] is not an object")
		}
		if c0["role"] != "user" {
			t.Errorf("contents[0].role = %q, want %q", c0["role"], "user")
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"OK"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2,"totalTokenCount":7}}`))
	}))
	defer srv.Close()

	client := NewGoogleClient("test-key", srv.URL)
	resp, err := client.Complete(context.Background(), CompletionRequest{
		Model: "gemini-2.0-flash",
		Messages: []Message{
			{Role: RoleSystem, Content: "You are helpful."},
			{Role: RoleUser, Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Content != "OK" {
		t.Errorf("Content = %q, want %q", resp.Content, "OK")
	}
}

func TestGoogleStreamComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify streaming endpoint path
		if r.URL.Path != "/models/gemini-2.0-flash:streamGenerateContent" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/models/gemini-2.0-flash:streamGenerateContent")
		}
		if r.URL.Query().Get("alt") != "sse" {
			t.Errorf("alt = %q, want %q", r.URL.Query().Get("alt"), "sse")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected http.Flusher")
		}

		chunks := []string{
			`data: {"candidates":[{"content":{"parts":[{"text":"Hel"}],"role":"model"}}]}`,
			`data: {"candidates":[{"content":{"parts":[{"text":"lo!"}],"role":"model"},"finishReason":"STOP"}]}`,
		}
		for _, chunk := range chunks {
			w.Write([]byte(chunk + "\n\n"))
			flusher.Flush()
		}
	}))
	defer srv.Close()

	client := NewGoogleClient("test-key", srv.URL)

	var accumulated string
	var gotDone bool

	err := client.StreamComplete(context.Background(), CompletionRequest{
		Model: "gemini-2.0-flash",
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
