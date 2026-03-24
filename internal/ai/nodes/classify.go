package ainodes

import (
	"context"
	"fmt"
	"strings"

	"github.com/monoes/monoes-agent/internal/ai"
	"github.com/monoes/monoes-agent/internal/workflow"
)

// ClassifyNode classifies each input item into one of the configured categories using AI.
//
// Config fields:
//
//	"provider_id" (string, required): AI provider ID.
//	"model" (string): Model name; falls back to provider's default.
//	"categories" ([]string, required): List of category names.
//	"prompt_template" (string): Custom prompt template. Supports {{$json.FIELD}} placeholders.
//	    If not provided, a default classification prompt is used.
//	"temperature" (float64): Sampling temperature (default 0.3).
//	"max_tokens" (int): Maximum response tokens (default 256).
//
// Output handles: "main" (all items with category/confidence fields) + one handle per category.
type ClassifyNode struct {
	Store          *ai.AIStore
	ClientOverride ai.AIClient
}

func (n *ClassifyNode) Type() string { return "ai.classify" }

func (n *ClassifyNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	client, model, err := n.getClientAndModel(config)
	if err != nil {
		return nil, err
	}

	categories := configStringSlice(config, "categories")
	if len(categories) == 0 {
		return nil, fmt.Errorf("%w: classify node requires \"categories\"", workflow.ErrInvalidConfig)
	}

	promptTemplate := configString(config, "prompt_template", "")
	temperature := configFloat(config, "temperature", 0.3)
	maxTokens := configInt(config, "max_tokens", 256)

	categoriesList := strings.Join(categories, ", ")
	systemPrompt := fmt.Sprintf(
		"You are a classification assistant. Classify the input into exactly one of these categories: %s.\n"+
			"Respond with ONLY the category name, nothing else. No explanation, no punctuation, just the category name.",
		categoriesList,
	)

	// Build per-category item buckets.
	categoryItems := make(map[string][]workflow.Item, len(categories))
	for _, cat := range categories {
		categoryItems[cat] = nil
	}

	allItems := make([]workflow.Item, 0, len(input.Items))

	for _, item := range input.Items {
		var userPrompt string
		if promptTemplate != "" {
			userPrompt = expandTemplate(promptTemplate, item)
		} else {
			// Default: serialize the item JSON as the input to classify.
			userPrompt = fmt.Sprintf("Classify the following data:\n%v", item.JSON)
		}

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
			return nil, fmt.Errorf("classify node: AI completion failed: %w", err)
		}

		rawCategory := trimResponse(resp.Content)

		// Match the response to one of the known categories (case-insensitive).
		matched := ""
		confidence := 0.0
		lowerRaw := strings.ToLower(rawCategory)
		for _, cat := range categories {
			if strings.ToLower(cat) == lowerRaw {
				matched = cat
				confidence = 1.0
				break
			}
		}
		// Fallback: partial match.
		if matched == "" {
			for _, cat := range categories {
				if strings.Contains(lowerRaw, strings.ToLower(cat)) {
					matched = cat
					confidence = 0.8
					break
				}
			}
		}
		// If still no match, use the raw response.
		if matched == "" {
			matched = rawCategory
			confidence = 0.5
		}

		outJSON := copyItemJSON(item)
		outJSON["category"] = matched
		outJSON["confidence"] = confidence
		outItem := workflow.Item{JSON: outJSON}

		allItems = append(allItems, outItem)
		if _, ok := categoryItems[matched]; ok {
			categoryItems[matched] = append(categoryItems[matched], outItem)
		}
	}

	outputs := []workflow.NodeOutput{
		{Handle: "main", Items: allItems},
	}
	for _, cat := range categories {
		if items := categoryItems[cat]; len(items) > 0 {
			outputs = append(outputs, workflow.NodeOutput{Handle: cat, Items: items})
		}
	}

	return outputs, nil
}

func (n *ClassifyNode) getClientAndModel(config map[string]interface{}) (ai.AIClient, string, error) {
	if n.ClientOverride != nil {
		model := configString(config, "model", "test-model")
		return n.ClientOverride, model, nil
	}
	return getClient(n.Store, config)
}
