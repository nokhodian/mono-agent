package ainodes

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nokhodian/mono-agent/internal/ai"
	"github.com/nokhodian/mono-agent/internal/workflow"
)

// ExtractNode sends each input item to an AI model with instructions to extract structured data.
//
// Config fields:
//
//	"provider_id" (string, required): AI provider ID.
//	"model" (string): Model name; falls back to provider's default.
//	"prompt" (string, required): Prompt template describing what to extract. Supports {{$json.FIELD}}.
//	"output_schema" (string): JSON string describing the expected output schema.
//	"output_key" (string): Key to store extracted data under (default "extracted").
//	"temperature" (float64): Sampling temperature (default 0.2 for deterministic extraction).
//	"max_tokens" (int): Maximum response tokens.
type ExtractNode struct {
	Store          *ai.AIStore
	ClientOverride ai.AIClient
}

func (n *ExtractNode) Type() string { return "ai.extract" }

func (n *ExtractNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	client, model, err := n.getClientAndModel(config)
	if err != nil {
		return nil, err
	}

	promptTemplate := configString(config, "prompt", "")
	if promptTemplate == "" {
		return nil, fmt.Errorf("%w: extract node requires \"prompt\"", workflow.ErrInvalidConfig)
	}

	outputSchema := configString(config, "output_schema", "")
	temperature := configFloat(config, "temperature", 0.2)
	maxTokens := configInt(config, "max_tokens", 1024)
	outputKey := configString(config, "output_key", "extracted")

	systemPrompt := "You are a data extraction assistant. You MUST respond with valid JSON only, no additional text or markdown formatting."
	if outputSchema != "" {
		systemPrompt += fmt.Sprintf("\n\nThe output must match this JSON schema:\n%s", outputSchema)
	}

	items := make([]workflow.Item, 0, len(input.Items))
	for _, item := range input.Items {
		prompt := expandTemplate(promptTemplate, item)

		messages := []ai.Message{
			{Role: ai.RoleSystem, Content: systemPrompt},
			{Role: ai.RoleUser, Content: prompt},
		}

		resp, err := client.Complete(ctx, ai.CompletionRequest{
			Model:       model,
			Messages:    messages,
			Temperature: &temperature,
			MaxTokens:   maxTokens,
		})
		if err != nil {
			return nil, fmt.Errorf("extract node: AI completion failed: %w", err)
		}

		content := trimResponse(resp.Content)

		// Try to parse the response as JSON.
		var parsed interface{}
		if err := json.Unmarshal([]byte(content), &parsed); err != nil {
			// If JSON parsing fails, store the raw response as a string.
			parsed = content
		}

		outJSON := copyItemJSON(item)
		outJSON[outputKey] = parsed
		items = append(items, workflow.Item{JSON: outJSON})
	}

	return []workflow.NodeOutput{
		{Handle: "main", Items: items},
	}, nil
}

func (n *ExtractNode) getClientAndModel(config map[string]interface{}) (ai.AIClient, string, error) {
	if n.ClientOverride != nil {
		model := configString(config, "model", "test-model")
		return n.ClientOverride, model, nil
	}
	return getClient(n.Store, config)
}
