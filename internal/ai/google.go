package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func debugLog(format string, args ...interface{}) {
	f, err := os.OpenFile("/tmp/monoes-google-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, format+"\n", args...)
}

// GoogleClient implements AIClient for Google Gemini APIs.
type GoogleClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewGoogleClient creates a GoogleClient.
func NewGoogleClient(apiKey, baseURL string) *GoogleClient {
	baseURL = strings.TrimRight(baseURL, "/")
	return &GoogleClient{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 5 * time.Minute},
	}
}

// ── Google Gemini wire types ─────────────────────────────────────────────────

type googleRequest struct {
	Contents          []googleContent       `json:"contents"`
	SystemInstruction *googleContent        `json:"systemInstruction,omitempty"`
	GenerationConfig  *googleGenerationConf `json:"generationConfig,omitempty"`
	Tools             []googleTool          `json:"tools,omitempty"`
}

type googleTool struct {
	FunctionDeclarations []googleFuncDecl `json:"functionDeclarations"`
}

type googleFuncDecl struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

type googleContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []googlePart `json:"parts"`
}

type googlePart struct {
	Text         string                 `json:"text,omitempty"`
	FunctionCall *googleFunctionCall    `json:"functionCall,omitempty"`
	FunctionResp *googleFunctionResp    `json:"functionResponse,omitempty"`
}

type googleFunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

type googleFunctionResp struct {
	Name     string      `json:"name"`
	Response interface{} `json:"response"`
}

type googleGenerationConf struct {
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`
}

type googleResponse struct {
	Candidates    []googleCandidate    `json:"candidates"`
	UsageMetadata googleUsageMetadata  `json:"usageMetadata"`
}

type googleCandidate struct {
	Content      googleContent `json:"content"`
	FinishReason string        `json:"finishReason"` // STOP, MAX_TOKENS, SAFETY, etc.
}

type googleUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// ── Complete ─────────────────────────────────────────────────────────────────

func (c *GoogleClient) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	gReq := c.toWireRequest(req)
	body, err := json.Marshal(gReq)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", c.baseURL, req.Model, c.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

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
		return CompletionResponse{}, fmt.Errorf("google: status %d: %s", resp.StatusCode, string(respBody))
	}

	var gResp googleResponse
	if err := json.Unmarshal(respBody, &gResp); err != nil {
		return CompletionResponse{}, fmt.Errorf("unmarshal response: %w", err)
	}

	return c.fromWireResponse(gResp), nil
}

// ── StreamComplete ───────────────────────────────────────────────────────────

func (c *GoogleClient) StreamComplete(ctx context.Context, req CompletionRequest, onChunk func(StreamChunk)) error {
	gReq := c.toWireRequest(req)
	body, err := json.Marshal(gReq)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	// DEBUG: log the tools being sent
	debugLog("[GOOGLE DEBUG] Request has %d tools", len(gReq.Tools))
	if len(gReq.Tools) > 0 {
		debugLog("[GOOGLE DEBUG] Tool declarations: %d", len(gReq.Tools[0].FunctionDeclarations))
		for _, d := range gReq.Tools[0].FunctionDeclarations {
			debugLog("[GOOGLE DEBUG]   tool: %s", d.Name)
		}
	}
	debugLog("[GOOGLE DEBUG] Request body (first 2000 chars): %.2000s", string(body))

	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse&key=%s", c.baseURL, req.Model, c.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("google: status %d: %s", resp.StatusCode, string(respBody))
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

		debugLog("[GOOGLE DEBUG] SSE chunk: %.500s", data)

		var gResp googleResponse
		if err := json.Unmarshal([]byte(data), &gResp); err != nil {
			debugLog("[GOOGLE DEBUG] unmarshal error: %v", err)
			continue // skip malformed chunks
		}

		sc := StreamChunk{}
		if len(gResp.Candidates) > 0 {
			cand := gResp.Candidates[0]
			for _, part := range cand.Content.Parts {
				if part.Text != "" {
					sc.Content += part.Text
				}
				if part.FunctionCall != nil {
					argsJSON, _ := json.Marshal(part.FunctionCall.Args)
					sc.ToolCalls = append(sc.ToolCalls, ToolCall{
						ID:   fmt.Sprintf("call_%s_%d", part.FunctionCall.Name, len(sc.ToolCalls)),
						Type: "function",
						Function: ToolCallFunc{
							Name:      part.FunctionCall.Name,
							Arguments: string(argsJSON),
						},
					})
				}
			}
			if cand.FinishReason == "STOP" || cand.FinishReason == "MAX_TOKENS" ||
				len(sc.ToolCalls) > 0 {
				sc.FinishReason = cand.FinishReason
				sc.Done = true
			}
		}
		debugLog("[GOOGLE DEBUG] chunk: content=%q toolCalls=%d done=%v finishReason=%q", sc.Content, len(sc.ToolCalls), sc.Done, sc.FinishReason)
		onChunk(sc)
		if sc.Done {
			return nil
		}
	}
	return scanner.Err()
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func (c *GoogleClient) toWireRequest(req CompletionRequest) googleRequest {
	gReq := googleRequest{}

	for _, m := range req.Messages {
		switch m.Role {
		case RoleSystem:
			gReq.SystemInstruction = &googleContent{
				Parts: []googlePart{{Text: m.Content}},
			}
		case RoleUser:
			gReq.Contents = append(gReq.Contents, googleContent{
				Role:  "user",
				Parts: []googlePart{{Text: m.Content}},
			})
		case RoleAssistant:
			parts := make([]googlePart, 0)
			if m.Content != "" {
				parts = append(parts, googlePart{Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				// Parse arguments from JSON string to map
				var args map[string]interface{}
				json.Unmarshal([]byte(tc.Function.Arguments), &args)
				parts = append(parts, googlePart{
					FunctionCall: &googleFunctionCall{
						Name: tc.Function.Name,
						Args: args,
					},
				})
			}
			if len(parts) > 0 {
				gReq.Contents = append(gReq.Contents, googleContent{
					Role:  "model",
					Parts: parts,
				})
			}
		case RoleTool:
			// Tool results map to functionResponse parts
			var respData interface{}
			if err := json.Unmarshal([]byte(m.Content), &respData); err != nil {
				respData = map[string]interface{}{"result": m.Content}
			}
			// Find the tool name from the ToolCallID by looking at previous messages
			toolName := m.ToolCallID // fallback
			for _, prev := range req.Messages {
				for _, tc := range prev.ToolCalls {
					if tc.ID == m.ToolCallID {
						toolName = tc.Function.Name
						break
					}
				}
			}
			gReq.Contents = append(gReq.Contents, googleContent{
				Role: "user",
				Parts: []googlePart{{
					FunctionResp: &googleFunctionResp{
						Name:     toolName,
						Response: respData,
					},
				}},
			})
		}
	}

	// Add tool definitions
	if len(req.Tools) > 0 {
		decls := make([]googleFuncDecl, 0, len(req.Tools))
		for _, t := range req.Tools {
			decls = append(decls, googleFuncDecl{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			})
		}
		gReq.Tools = []googleTool{{FunctionDeclarations: decls}}
	}

	if req.MaxTokens > 0 || req.Temperature != nil {
		gc := &googleGenerationConf{}
		if req.MaxTokens > 0 {
			gc.MaxOutputTokens = req.MaxTokens
		}
		if req.Temperature != nil {
			gc.Temperature = req.Temperature
		}
		gReq.GenerationConfig = gc
	}

	return gReq
}

func (c *GoogleClient) fromWireResponse(resp googleResponse) CompletionResponse {
	cr := CompletionResponse{
		Usage: Usage{
			PromptTokens:     resp.UsageMetadata.PromptTokenCount,
			CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      resp.UsageMetadata.TotalTokenCount,
		},
	}
	if len(resp.Candidates) > 0 {
		cand := resp.Candidates[0]
		for _, part := range cand.Content.Parts {
			if part.Text != "" {
				cr.Content += part.Text
			}
			if part.FunctionCall != nil {
				argsJSON, _ := json.Marshal(part.FunctionCall.Args)
				cr.ToolCalls = append(cr.ToolCalls, ToolCall{
					ID:   fmt.Sprintf("call_%s_%d", part.FunctionCall.Name, len(cr.ToolCalls)),
					Type: "function",
					Function: ToolCallFunc{
						Name:      part.FunctionCall.Name,
						Arguments: string(argsJSON),
					},
				})
			}
		}
		cr.FinishReason = cand.FinishReason
	}
	return cr
}
