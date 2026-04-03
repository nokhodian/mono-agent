package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/nokhodian/mono-agent/internal/workflow"
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
	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]interface{}{
			{"role": "user", "content": prompt},
		},
		// Limit text tokens to avoid 402 credit-cap errors; image tokens are billed separately.
		"max_tokens": 512,
	}

	// OpenRouter routes image models through /chat/completions (same as text).
	data, err := apiRequest(ctx, http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", apiKey, reqBody)
	if err != nil {
		return item, fmt.Errorf("openrouter generate_image: %w", err)
	}

	// Extract image URL from response — OpenRouter image models return the URL in one of:
	//   a) choices[0].message.content = "https://..."  (string URL)
	//   b) choices[0].message.content = [{"type":"image_url","image_url":{"url":"..."}}]  (array)
	//   c) choices[0].message.images = [{"type":"image_url","image_url":{"url":"data:..."}}]  (Gemini-style)
	imageURL := ""
	if choices, ok := data["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if msg, ok := choice["message"].(map[string]interface{}); ok {
				// Check message.images first (Gemini image models return here)
				if imgs, ok := msg["images"].([]interface{}); ok && len(imgs) > 0 {
					if img, ok := imgs[0].(map[string]interface{}); ok {
						if iu, ok := img["image_url"].(map[string]interface{}); ok {
							imageURL, _ = iu["url"].(string)
						}
					}
				}
				// Fall back to message.content
				if imageURL == "" {
					switch content := msg["content"].(type) {
					case string:
						imageURL = content
					case []interface{}:
						for _, part := range content {
							if p, ok := part.(map[string]interface{}); ok {
								if p["type"] == "image_url" {
									if iu, ok := p["image_url"].(map[string]interface{}); ok {
										imageURL, _ = iu["url"].(string)
									}
								}
							}
						}
					}
				}
			}
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

// downloadImageToTemp saves an image to a temp file and returns the local file path.
// Handles both remote URLs (http/https) and data URIs (data:image/...;base64,...).
func downloadImageToTemp(ctx context.Context, imageURL string) (string, error) {
	// Handle base64 data URIs (e.g. from Gemini image models).
	if strings.HasPrefix(imageURL, "data:") {
		comma := strings.Index(imageURL, ",")
		if comma < 0 {
			return "", fmt.Errorf("invalid data URI: no comma separator")
		}
		imgData, err := base64.StdEncoding.DecodeString(imageURL[comma+1:])
		if err != nil {
			return "", fmt.Errorf("decode base64 image: %w", err)
		}
		f, err := os.CreateTemp("", "monoes_post_*.png")
		if err != nil {
			return "", fmt.Errorf("create temp file: %w", err)
		}
		filePath := f.Name()
		if _, err := f.Write(imgData); err != nil {
			f.Close()
			return "", fmt.Errorf("write image file: %w", err)
		}
		f.Close()
		return filePath, nil
	}

	// Remote URL — plain HTTP download.
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

	f, err := os.CreateTemp("", "monoes_post_*.png")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	filePath := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		return "", fmt.Errorf("write image file: %w", err)
	}
	f.Close()
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
