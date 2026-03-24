package ainodes

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/monoes/monoes-agent/internal/ai"
	"github.com/monoes/monoes-agent/internal/workflow"
)

// EmbedNode generates a numerical embedding for a text field in each input item.
//
// Note: This is a stub implementation that uses the chat completion endpoint
// to produce a pseudo-embedding. A proper embedding API integration can be added later.
//
// Config fields:
//
//	"provider_id" (string, required): AI provider ID.
//	"model" (string): Model name; falls back to provider's default.
//	"input_field" (string): Field name containing text to embed (default uses full item JSON).
//	"output_key" (string): Key to store the embedding under (default "embedding").
//	"max_tokens" (int): Maximum response tokens (default 256).
type EmbedNode struct {
	Store          *ai.AIStore
	ClientOverride ai.AIClient
}

func (n *EmbedNode) Type() string { return "ai.embed" }

func (n *EmbedNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	client, model, err := n.getClientAndModel(config)
	if err != nil {
		return nil, err
	}

	inputField := configString(config, "input_field", "")
	outputKey := configString(config, "output_key", "embedding")
	maxTokens := configInt(config, "max_tokens", 256)

	systemPrompt := "You are an embedding generator. Return ONLY a JSON array of 3 floating-point numbers representing the semantic meaning of the input text. Example: [0.123, -0.456, 0.789]. No other text."
	temperature := 0.0

	items := make([]workflow.Item, 0, len(input.Items))
	for _, item := range input.Items {
		var text string
		if inputField != "" {
			if v, ok := item.JSON[inputField]; ok {
				text = fmt.Sprintf("%v", v)
			} else {
				text = ""
			}
		} else {
			text = fmt.Sprintf("%v", item.JSON)
		}

		messages := []ai.Message{
			{Role: ai.RoleSystem, Content: systemPrompt},
			{Role: ai.RoleUser, Content: text},
		}

		resp, err := client.Complete(ctx, ai.CompletionRequest{
			Model:       model,
			Messages:    messages,
			Temperature: &temperature,
			MaxTokens:   maxTokens,
		})
		if err != nil {
			return nil, fmt.Errorf("embed node: AI completion failed: %w", err)
		}

		content := trimResponse(resp.Content)

		// Try to parse the response as a JSON array of floats.
		var embedding []float64
		if err := json.Unmarshal([]byte(content), &embedding); err != nil {
			// If parsing fails, store the raw response string.
			outJSON := copyItemJSON(item)
			outJSON[outputKey] = content
			items = append(items, workflow.Item{JSON: outJSON})
			continue
		}

		outJSON := copyItemJSON(item)
		outJSON[outputKey] = embedding
		items = append(items, workflow.Item{JSON: outJSON})
	}

	return []workflow.NodeOutput{
		{Handle: "main", Items: items},
	}, nil
}

func (n *EmbedNode) getClientAndModel(config map[string]interface{}) (ai.AIClient, string, error) {
	if n.ClientOverride != nil {
		model := configString(config, "model", "test-model")
		return n.ClientOverride, model, nil
	}
	return getClient(n.Store, config)
}
