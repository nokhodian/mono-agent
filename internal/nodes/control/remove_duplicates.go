package control

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nokhodian/mono-agent/internal/workflow"
)

// RemoveDuplicatesNode deduplicates items by a key field.
// Config fields:
//
//	"field" (string, optional): dot-path key field. If empty, deduplicate by entire JSON serialization.
//	"keep"  (string, optional): "first" (default) or "last"
type RemoveDuplicatesNode struct{}

func (n *RemoveDuplicatesNode) Type() string { return "core.remove_duplicates" }

func (n *RemoveDuplicatesNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	field, _ := config["field"].(string)
	keep, _ := config["keep"].(string)
	if keep == "" {
		keep = "first"
	}
	if keep != "first" && keep != "last" {
		return nil, fmt.Errorf("remove_duplicates: 'keep' must be 'first' or 'last', got %q", keep)
	}

	keyOf := func(item workflow.Item) (string, error) {
		if field != "" {
			val := dotGet(item.JSON, field)
			return fmt.Sprintf("%v", val), nil
		}
		// Entire JSON as key
		b, err := json.Marshal(item.JSON)
		if err != nil {
			return "", fmt.Errorf("remove_duplicates: failed to marshal item: %w", err)
		}
		return string(b), nil
	}

	if keep == "first" {
		seen := make(map[string]bool)
		result := make([]workflow.Item, 0, len(input.Items))
		for _, item := range input.Items {
			k, err := keyOf(item)
			if err != nil {
				return nil, err
			}
			if !seen[k] {
				seen[k] = true
				result = append(result, item)
			}
		}
		return []workflow.NodeOutput{{Handle: "main", Items: result}}, nil
	}

	// keep == "last": track position of last occurrence, then reconstruct in original order
	type entry struct {
		item  workflow.Item
		index int
	}
	lastSeen := make(map[string]entry)
	order := make([]string, 0, len(input.Items))
	for idx, item := range input.Items {
		k, err := keyOf(item)
		if err != nil {
			return nil, err
		}
		if _, exists := lastSeen[k]; !exists {
			order = append(order, k)
		}
		lastSeen[k] = entry{item: item, index: idx}
	}

	result := make([]workflow.Item, 0, len(order))
	for _, k := range order {
		result = append(result, lastSeen[k].item)
	}

	return []workflow.NodeOutput{{Handle: "main", Items: result}}, nil
}
