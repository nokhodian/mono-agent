package ainodes

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/monoes/monoes-agent/internal/ai"
	"github.com/monoes/monoes-agent/internal/workflow"
)

// templatePattern matches {{$json.FIELD}} placeholders in prompt templates.
var templatePattern = regexp.MustCompile(`\{\{\$json\.(\w+)\}\}`)

// getClient reads provider_id and model from config, fetches the provider from the store,
// and creates an AIClient. Returns the client and model name.
func getClient(store *ai.AIStore, config map[string]interface{}) (ai.AIClient, string, error) {
	providerID := configString(config, "provider_id", "")
	if providerID == "" {
		return nil, "", fmt.Errorf("%w: provider_id is required", workflow.ErrInvalidConfig)
	}

	provider, err := store.GetProvider(providerID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get AI provider %q: %w", providerID, err)
	}

	model := configString(config, "model", provider.DefaultModel)
	if model == "" {
		return nil, "", fmt.Errorf("%w: model is required", workflow.ErrInvalidConfig)
	}

	client, err := ai.NewClient(provider)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create AI client: %w", err)
	}

	return client, model, nil
}

// expandTemplate replaces {{$json.KEY}} placeholders in template with values from item.JSON.
func expandTemplate(template string, item workflow.Item) string {
	return templatePattern.ReplaceAllStringFunc(template, func(match string) string {
		parts := templatePattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		key := parts[1]
		if item.JSON == nil {
			return match
		}
		val, ok := item.JSON[key]
		if !ok {
			return match
		}
		return fmt.Sprintf("%v", val)
	})
}

// configString extracts a string value from config, returning defaultVal if not found.
func configString(config map[string]interface{}, key, defaultVal string) string {
	if v, ok := config[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return defaultVal
}

// configFloat extracts a float64 value from config, returning defaultVal if not found.
func configFloat(config map[string]interface{}, key string, defaultVal float64) float64 {
	if v, ok := config[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int:
			return float64(n)
		case json_number:
			if f, err := n.Float64(); err == nil {
				return f
			}
		}
	}
	return defaultVal
}

// configInt extracts an int value from config, returning defaultVal if not found.
func configInt(config map[string]interface{}, key string, defaultVal int) int {
	if v, ok := config[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case json_number:
			if i, err := n.Int64(); err == nil {
				return int(i)
			}
		}
	}
	return defaultVal
}

// json_number is an alias for encoding/json.Number to handle JSON-decoded numbers.
type json_number = interface {
	Float64() (float64, error)
	Int64() (int64, error)
	String() string
}

// copyItemJSON creates a shallow copy of an item's JSON map.
func copyItemJSON(item workflow.Item) map[string]interface{} {
	m := make(map[string]interface{}, len(item.JSON))
	for k, v := range item.JSON {
		m[k] = v
	}
	return m
}

// configStringSlice extracts a []string from config. Supports both []string and []interface{}.
func configStringSlice(config map[string]interface{}, key string) []string {
	v, ok := config[key]
	if !ok {
		return nil
	}
	switch s := v.(type) {
	case []string:
		return s
	case []interface{}:
		result := make([]string, 0, len(s))
		for _, elem := range s {
			if str, ok := elem.(string); ok {
				result = append(result, str)
			}
		}
		return result
	}
	return nil
}

// trimResponse cleans up AI responses by removing leading/trailing whitespace.
func trimResponse(s string) string {
	return strings.TrimSpace(s)
}
