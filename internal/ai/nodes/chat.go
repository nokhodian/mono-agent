package ainodes

import (
	"context"
	"fmt"

	"github.com/nokhodian/mono-agent/internal/ai"
	"github.com/nokhodian/mono-agent/internal/workflow"
)

// ChatNode sends each input item through an AI chat completion.
//
// Config fields:
//
//	"provider_id" (string, required): AI provider ID.
//	"model" (string): Model name; falls back to provider's default.
//	"system_prompt" (string): Optional system message.
//	"prompt" (string, required): User prompt template. Supports {{$json.FIELD}} placeholders.
//	"temperature" (float64): Sampling temperature.
//	"max_tokens" (int): Maximum response tokens.
//	"output_key" (string): Key to store the AI response under (default "ai_response").
type ChatNode struct {
	Store          *ai.AIStore
	ClientOverride ai.AIClient // if set, used instead of creating from store
}

func (n *ChatNode) Type() string { return "ai.chat" }

func (n *ChatNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	client, model, err := n.getClientAndModel(config)
	if err != nil {
		return nil, err
	}

	systemPrompt := configString(config, "system_prompt", "")
	promptTemplate := configString(config, "prompt", "")
	if promptTemplate == "" {
		return nil, fmt.Errorf("%w: chat node requires \"prompt\"", workflow.ErrInvalidConfig)
	}

	temperature := configFloat(config, "temperature", 0.7)
	maxTokens := configInt(config, "max_tokens", 1024)
	outputKey := configString(config, "output_key", "ai_response")

	items := make([]workflow.Item, 0, len(input.Items))
	for _, item := range input.Items {
		prompt := expandTemplate(promptTemplate, item)

		var messages []ai.Message
		if systemPrompt != "" {
			messages = append(messages, ai.Message{Role: ai.RoleSystem, Content: systemPrompt})
		}
		messages = append(messages, ai.Message{Role: ai.RoleUser, Content: prompt})

		resp, err := client.Complete(ctx, ai.CompletionRequest{
			Model:       model,
			Messages:    messages,
			Temperature: &temperature,
			MaxTokens:   maxTokens,
		})
		if err != nil {
			return nil, fmt.Errorf("chat node: AI completion failed: %w", err)
		}

		outJSON := copyItemJSON(item)
		outJSON[outputKey] = trimResponse(resp.Content)
		items = append(items, workflow.Item{JSON: outJSON})
	}

	return []workflow.NodeOutput{
		{Handle: "main", Items: items},
	}, nil
}

func (n *ChatNode) getClientAndModel(config map[string]interface{}) (ai.AIClient, string, error) {
	if n.ClientOverride != nil {
		model := configString(config, "model", "test-model")
		return n.ClientOverride, model, nil
	}
	return getClient(n.Store, config)
}
