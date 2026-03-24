package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/monoes/monoes-agent/internal/workflow"
)

// OpenRouterNode implements the service.openrouter node type.
// It supports two operations:
//   - generate_image: calls FLUX 1.1 Pro, downloads the result to /tmp, adds url + file_path to item
//   - generate_text: calls a chat model, adds text to item
//
// Both operations ENRICH the input item (add fields) rather than replacing it.
type OpenRouterNode struct{}

func (n *OpenRouterNode) Type() string { return "service.openrouter" }

func (n *OpenRouterNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	apiKey := strVal(config, "api_key")
	if apiKey == "" {
		return nil, fmt.Errorf("openrouter: api_key is required")
	}

	operation := strVal(config, "operation")
	if operation == "" {
		operation = "generate_text"
	}

	// If no items are flowing in, create one empty item so the node can act as a source.
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
			return nil, fmt.Errorf("openrouter: unknown operation %q", operation)
		}
		if err != nil {
			return nil, err
		}
		outputItems = append(outputItems, enriched)
	}

	return []workflow.NodeOutput{{Handle: "main", Items: outputItems}}, nil
}

// generateImage calls the OpenRouter image generation API (FLUX 1.1 Pro by default),
// downloads the generated image to a temp file, and adds "url" and "file_path" to the item.
func (n *OpenRouterNode) generateImage(ctx context.Context, apiKey string, config map[string]interface{}, item workflow.Item) (workflow.Item, error) {
	prompt := strVal(config, "prompt")
	if prompt == "" {
		return item, fmt.Errorf("openrouter generate_image: prompt is required")
	}
	model := strVal(config, "model")
	if model == "" {
		model = "black-forest-labs/flux-1.1-pro"
	}
	width := intVal(config, "width")
	if width == 0 {
		width = 1024
	}
	height := intVal(config, "height")
	if height == 0 {
		height = 1024
	}

	reqBody := map[string]interface{}{
		"model":  model,
		"prompt": prompt,
		"width":  width,
		"height": height,
	}

	// Use the shared apiRequest helper (same package, sets Authorization + Content-Type headers).
	data, err := apiRequest(ctx, http.MethodPost, "https://openrouter.ai/api/v1/images/generations", apiKey, reqBody)
	if err != nil {
		return item, fmt.Errorf("openrouter generate_image: %w", err)
	}

	imageURL := ""
	if imageData, ok := data["data"].([]interface{}); ok && len(imageData) > 0 {
		if img, ok := imageData[0].(map[string]interface{}); ok {
			imageURL, _ = img["url"].(string)
		}
	}
	if imageURL == "" {
		return item, fmt.Errorf("openrouter generate_image: no image URL in response: %v", data)
	}

	// Download the image to a temp file (binary download — cannot use apiRequest which returns JSON).
	filePath, err := downloadImageToTemp(ctx, imageURL)
	if err != nil {
		return item, fmt.Errorf("openrouter generate_image: download failed: %w", err)
	}

	// Enrich the item with image URL and local file path.
	enriched := copyItem(item)
	enriched.JSON["url"] = imageURL
	enriched.JSON["file_path"] = filePath
	return enriched, nil
}

// generateText calls the OpenRouter chat completions API and adds "text" to the item.
func (n *OpenRouterNode) generateText(ctx context.Context, apiKey string, config map[string]interface{}, item workflow.Item) (workflow.Item, error) {
	prompt := strVal(config, "prompt")
	if prompt == "" {
		return item, fmt.Errorf("openrouter generate_text: prompt is required")
	}
	model := strVal(config, "model")
	if model == "" {
		model = "anthropic/claude-3.5-sonnet"
	}
	maxTokens := intVal(config, "max_tokens")
	if maxTokens == 0 {
		maxTokens = 500
	}

	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]interface{}{
			{"role": "user", "content": prompt},
		},
		"max_tokens": maxTokens,
	}

	// Use the shared apiRequest helper (same package).
	data, err := apiRequest(ctx, http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", apiKey, reqBody)
	if err != nil {
		return item, fmt.Errorf("openrouter generate_text: %w", err)
	}

	text := ""
	if choices, ok := data["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if msg, ok := choice["message"].(map[string]interface{}); ok {
				text, _ = msg["content"].(string)
			}
		}
	}

	// Enrich the item.
	enriched := copyItem(item)
	enriched.JSON["text"] = text
	return enriched, nil
}

// downloadImageToTemp downloads a binary image URL to /tmp and returns the local file path.
// Uses plain http.Get — NOT apiRequest (which parses JSON responses).
func downloadImageToTemp(ctx context.Context, imageURL string) (string, error) {
	filePath := fmt.Sprintf("/tmp/monoes_post_%d.jpg", time.Now().UnixNano())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("download image HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read image body: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return "", fmt.Errorf("write image file: %w", err)
	}
	return filePath, nil
}

// copyItem returns a shallow copy of an Item with a new JSON map containing all original fields.
func copyItem(item workflow.Item) workflow.Item {
	newJSON := make(map[string]interface{}, len(item.JSON)+2)
	for k, v := range item.JSON {
		newJSON[k] = v
	}
	return workflow.Item{JSON: newJSON, Binary: item.Binary}
}
