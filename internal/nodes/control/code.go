package control

import (
	"context"
	"fmt"
	"time"

	"github.com/dop251/goja"
	"github.com/nokhodian/mono-agent/internal/workflow"
)

// CodeNode executes JavaScript code via the goja runtime.
// Config fields:
//
//	"code" (string, required): JavaScript code to execute.
//	    The JS environment exposes:
//	    - $input: object with all() method returning all items as JS objects
//	    - $json: first item's JSON map (shorthand)
//	    The code must return an array of objects (or a single object).
//	    Each returned object becomes an Item on the "main" handle.
type CodeNode struct{}

func (n *CodeNode) Type() string { return "core.code" }

func (n *CodeNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	code, _ := config["code"].(string)
	if code == "" {
		return nil, fmt.Errorf("%w: code node requires \"code\"", workflow.ErrInvalidConfig)
	}

	rt := goja.New()

	// Build the array of item JSON maps for $input.all().
	itemMaps := make([]interface{}, len(input.Items))
	for i, item := range input.Items {
		m := make(map[string]interface{})
		for k, v := range item.JSON {
			m[k] = v
		}
		itemMaps[i] = m
	}

	// $input object with all() method.
	inputObj := rt.NewObject()
	if err := inputObj.Set("all", func(call goja.FunctionCall) goja.Value {
		return rt.ToValue(itemMaps)
	}); err != nil {
		return nil, fmt.Errorf("code node: failed to set $input.all: %w", err)
	}
	if err := rt.Set("$input", inputObj); err != nil {
		return nil, fmt.Errorf("code node: failed to set $input: %w", err)
	}

	// $json — first item's JSON, or empty map.
	firstJSON := make(map[string]interface{})
	if len(input.Items) > 0 && input.Items[0].JSON != nil {
		for k, v := range input.Items[0].JSON {
			firstJSON[k] = v
		}
	}
	if err := rt.Set("$json", firstJSON); err != nil {
		return nil, fmt.Errorf("code node: failed to set $json: %w", err)
	}

	// Enforce a 30-second timeout via goja's interrupt mechanism.
	timeout := 30 * time.Second
	timer := time.AfterFunc(timeout, func() {
		rt.Interrupt("code node: execution timeout (30s)")
	})
	defer timer.Stop()

	// Also respect the parent context cancellation.
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			rt.Interrupt("code node: context cancelled")
		case <-done:
		}
	}()

	val, err := rt.RunString(code)
	close(done)

	if err != nil {
		// Translate context cancellation into the canonical error.
		if ctx.Err() != nil {
			return nil, workflow.ErrExecutionCancelled
		}
		return nil, fmt.Errorf("code node: JS execution error: %w", err)
	}

	// Convert the returned value to []workflow.Item.
	items, err := jsValueToItems(val)
	if err != nil {
		return nil, fmt.Errorf("code node: result conversion error: %w", err)
	}

	return []workflow.NodeOutput{
		{Handle: "main", Items: items},
	}, nil
}

// jsValueToItems converts a goja.Value (array of objects or single object)
// to a slice of workflow.Item.
func jsValueToItems(val goja.Value) ([]workflow.Item, error) {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return []workflow.Item{}, nil
	}

	exported := val.Export()

	switch v := exported.(type) {
	case []interface{}:
		items := make([]workflow.Item, 0, len(v))
		for i, elem := range v {
			m, err := toStringMap(elem)
			if err != nil {
				return nil, fmt.Errorf("result[%d]: %w", i, err)
			}
			items = append(items, workflow.Item{JSON: m})
		}
		return items, nil

	case map[string]interface{}:
		return []workflow.Item{{JSON: v}}, nil

	default:
		return nil, fmt.Errorf("code must return an array of objects or a single object, got %T", exported)
	}
}

// toStringMap casts an interface{} to map[string]interface{}.
func toStringMap(v interface{}) (map[string]interface{}, error) {
	if m, ok := v.(map[string]interface{}); ok {
		return m, nil
	}
	return nil, fmt.Errorf("expected object, got %T", v)
}
