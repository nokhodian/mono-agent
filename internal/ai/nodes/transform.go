package ainodes

import (
	"context"
	"fmt"

	"github.com/monoes/monoes-agent/internal/ai"
	"github.com/monoes/monoes-agent/internal/workflow"
)

// TransformNode uses AI to transform a specific field of each input item.
//
// Config fields:
//
//	"provider_id" (string, required): AI provider ID.
//	"model" (string): Model name; falls back to provider's default.
//	"instruction" (string, required): Transformation instruction template. Supports {{$json.FIELD}}.
//	"input_field" (string): Field name to read from each item (default uses the full item JSON).
//	"output_key" (string): Key to store the transformed result under (default "transformed").
//	"temperature" (float64): Sampling temperature (default 0.5).
//	"max_tokens" (int): Maximum response tokens (default 1024).
type TransformNode struct {
	Store          *ai.AIStore
	ClientOverride ai.AIClient
}

func (n *TransformNode) Type() string { return "ai.transform" }

func (n *TransformNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	client, model, err := n.getClientAndModel(config)
	if err != nil {
		return nil, err
	}

	instruction := configString(config, "instruction", "")
	if instruction == "" {
		return nil, fmt.Errorf("%w: transform node requires \"instruction\"", workflow.ErrInvalidConfig)
	}

	inputField := configString(config, "input_field", "")
	outputKey := configString(config, "output_key", "transformed")
	temperature := configFloat(config, "temperature", 0.5)
	maxTokens := configInt(config, "max_tokens", 1024)

	systemPrompt := "You are a data transformation assistant. Apply the given instruction to the input and return only the result, with no additional explanation."

	items := make([]workflow.Item, 0, len(input.Items))
	for _, item := range input.Items {
		expandedInstruction := expandTemplate(instruction, item)

		var inputValue string
		if inputField != "" {
			if v, ok := item.JSON[inputField]; ok {
				inputValue = fmt.Sprintf("%v", v)
			} else {
				inputValue = ""
			}
		} else {
			inputValue = fmt.Sprintf("%v", item.JSON)
		}

		userPrompt := fmt.Sprintf("Instruction: %s\n\nInput:\n%s", expandedInstruction, inputValue)

		messages := []ai.Message{
			{Role: ai.RoleSystem, Content: systemPrompt},
			{Role: ai.RoleUser, Content: userPrompt},
		}

		resp, err := client.Complete(ctx, ai.CompletionRequest{
			Model:       model,
			Messages:    messages,
			Temperature: &temperature,
			MaxTokens:   maxTokens,
		})
		if err != nil {
			return nil, fmt.Errorf("transform node: AI completion failed: %w", err)
		}

		outJSON := copyItemJSON(item)
		outJSON[outputKey] = trimResponse(resp.Content)
		items = append(items, workflow.Item{JSON: outJSON})
	}

	return []workflow.NodeOutput{
		{Handle: "main", Items: items},
	}, nil
}

func (n *TransformNode) getClientAndModel(config map[string]interface{}) (ai.AIClient, string, error) {
	if n.ClientOverride != nil {
		model := configString(config, "model", "test-model")
		return n.ClientOverride, model, nil
	}
	return getClient(n.Store, config)
}
