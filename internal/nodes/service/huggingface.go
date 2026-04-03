package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/nokhodian/mono-agent/internal/workflow"
)

// HuggingFaceNode implements service.huggingface for image and text generation
// via the HuggingFace Inference API (free tier).
//
// generate_image: POST to /models/{model} with {"inputs": prompt}, response is binary image.
// generate_text:  POST to /models/{model} with {"inputs": prompt}, response is JSON array.
type HuggingFaceNode struct{}

func (n *HuggingFaceNode) Type() string { return "service.huggingface" }

func (n *HuggingFaceNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	apiKey := strVal(config, "api_key")
	if apiKey == "" {
		return nil, fmt.Errorf("huggingface: api_key is required")
	}

	operation := strVal(config, "operation")
	if operation == "" {
		operation = "generate_image"
	}

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
			return nil, fmt.Errorf("huggingface: unknown operation %q", operation)
		}
		if err != nil {
			return nil, err
		}
		outputItems = append(outputItems, enriched)
	}
	return []workflow.NodeOutput{{Handle: "main", Items: outputItems}}, nil
}

func (n *HuggingFaceNode) generateImage(ctx context.Context, apiKey string, config map[string]interface{}, item workflow.Item) (workflow.Item, error) {
	prompt := strVal(config, "prompt")
	if prompt == "" {
		return item, fmt.Errorf("huggingface generate_image: prompt is required")
	}
	model := strVal(config, "model")
	if model == "" {
		model = "black-forest-labs/FLUX.1-schnell"
	}

	body, _ := json.Marshal(map[string]interface{}{"inputs": prompt})
	url := fmt.Sprintf("https://router.huggingface.co/hf-inference/models/%s", model)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return item, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return item, fmt.Errorf("huggingface generate_image: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return item, fmt.Errorf("huggingface: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return item, fmt.Errorf("huggingface generate_image: HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	// Response is raw binary image data — save to temp file.
	f, err := os.CreateTemp("", "monoes_hf_*.png")
	if err != nil {
		return item, fmt.Errorf("huggingface: create temp file: %w", err)
	}
	filePath := f.Name()
	if _, err := f.Write(respBytes); err != nil {
		f.Close()
		return item, fmt.Errorf("huggingface: write image: %w", err)
	}
	f.Close()

	enriched := copyItem(item)
	enriched.JSON["file_path"] = filePath
	enriched.JSON["url"] = url
	return enriched, nil
}

func (n *HuggingFaceNode) generateText(ctx context.Context, apiKey string, config map[string]interface{}, item workflow.Item) (workflow.Item, error) {
	prompt := strVal(config, "prompt")
	if prompt == "" {
		return item, fmt.Errorf("huggingface generate_text: prompt is required")
	}
	model := strVal(config, "model")
	if model == "" {
		model = "meta-llama/Llama-3.2-3B-Instruct"
	}

	params := map[string]interface{}{}
	if maxTokens := intVal(config, "max_tokens"); maxTokens > 0 {
		params["max_new_tokens"] = maxTokens
	}
	reqPayload := map[string]interface{}{
		"inputs": prompt,
	}
	if len(params) > 0 {
		reqPayload["parameters"] = params
	}
	body, _ := json.Marshal(reqPayload)
	url := fmt.Sprintf("https://router.huggingface.co/hf-inference/models/%s", model)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return item, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return item, fmt.Errorf("huggingface generate_text: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return item, fmt.Errorf("huggingface: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return item, fmt.Errorf("huggingface generate_text: HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	// Response is [{generated_text: "..."}]
	var result []map[string]interface{}
	text := ""
	if json.Unmarshal(respBytes, &result) == nil && len(result) > 0 {
		text, _ = result[0]["generated_text"].(string)
	}

	enriched := copyItem(item)
	enriched.JSON["text"] = text
	return enriched, nil
}
