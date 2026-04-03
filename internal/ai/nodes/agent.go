package ainodes

import (
	"context"
	"fmt"

	"github.com/nokhodian/mono-agent/internal/ai"
	"github.com/nokhodian/mono-agent/internal/workflow"
)

// AgentNode executes a multi-step AI agent that works towards a goal.
//
// Current implementation is simplified: it performs a single completion with the goal
// and returns the response. Multi-turn tool-calling loop can be added later.
//
// Config fields:
//
//	"provider_id" (string, required): AI provider ID.
//	"model" (string): Model name; falls back to provider's default.
//	"goal" (string, required): Goal template. Supports {{$json.FIELD}} placeholders.
//	"max_steps" (int): Maximum agent steps (default 5). Currently unused in simplified impl.
//	"temperature" (float64): Sampling temperature (default 0.7).
//	"max_tokens" (int): Maximum response tokens (default 2048).
type AgentNode struct {
	Store          *ai.AIStore
	ClientOverride ai.AIClient
}

func (n *AgentNode) Type() string { return "ai.agent" }

func (n *AgentNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	client, model, err := n.getClientAndModel(config)
	if err != nil {
		return nil, err
	}

	goalTemplate := configString(config, "goal", "")
	if goalTemplate == "" {
		return nil, fmt.Errorf("%w: agent node requires \"goal\"", workflow.ErrInvalidConfig)
	}

	maxSteps := configInt(config, "max_steps", 5)
	temperature := configFloat(config, "temperature", 0.7)
	maxTokens := configInt(config, "max_tokens", 2048)

	systemPrompt := "You are an AI agent. Work towards the given goal step by step. " +
		"Provide a clear, comprehensive response that addresses the goal."

	items := make([]workflow.Item, 0, len(input.Items))
	for _, item := range input.Items {
		goal := expandTemplate(goalTemplate, item)

		messages := []ai.Message{
			{Role: ai.RoleSystem, Content: systemPrompt},
			{Role: ai.RoleUser, Content: goal},
		}

		resp, err := client.Complete(ctx, ai.CompletionRequest{
			Model:       model,
			Messages:    messages,
			Temperature: &temperature,
			MaxTokens:   maxTokens,
		})
		if err != nil {
			return nil, fmt.Errorf("agent node: AI completion failed: %w", err)
		}

		outJSON := copyItemJSON(item)
		outJSON["agent_result"] = trimResponse(resp.Content)
		outJSON["steps_taken"] = 1 // Simplified: always 1 step for now.
		outJSON["max_steps"] = maxSteps
		items = append(items, workflow.Item{JSON: outJSON})
	}

	return []workflow.NodeOutput{
		{Handle: "main", Items: items},
	}, nil
}

func (n *AgentNode) getClientAndModel(config map[string]interface{}) (ai.AIClient, string, error) {
	if n.ClientOverride != nil {
		model := configString(config, "model", "test-model")
		return n.ClientOverride, model, nil
	}
	return getClient(n.Store, config)
}
