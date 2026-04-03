package control

import (
	"context"
	"fmt"

	"github.com/nokhodian/mono-agent/internal/workflow"
)

// SwitchNode evaluates an expression and routes items to one of N output handles.
// Config fields:
//
//	"expression" (string, required): value expression, e.g. "{{$json.status}}"
//	"cases" ([]interface{}, required): each element is map{"value": "pending", "handle": "case0"}
//	"default_handle" (string, optional): handle name for unmatched items, default "default"
//	"fallthrough" (bool, optional): if true, item can match multiple cases
type SwitchNode struct{}

func (n *SwitchNode) Type() string { return "core.switch" }

func (n *SwitchNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	expression, _ := config["expression"].(string)
	casesRaw, _ := config["cases"].([]interface{})
	defaultHandle, _ := config["default_handle"].(string)
	if defaultHandle == "" {
		defaultHandle = "default"
	}
	fallthroughMode, _ := config["fallthrough"].(bool)

	engine := workflow.NewExpressionEngine()

	// Collect results per handle: handle -> []Item
	handleItems := make(map[string][]workflow.Item)

	for _, item := range input.Items {
		exprCtx := workflow.ExpressionContext{
			JSON:        item.JSON,
			Node:        input.NodeOutputs,
			WorkflowID:  input.WorkflowID,
			ExecutionID: input.ExecutionID,
		}

		val, err := engine.EvaluateString(expression, exprCtx)
		if err != nil {
			val = ""
		}

		matched := false
		for _, caseRaw := range casesRaw {
			caseMap, ok := caseRaw.(map[string]interface{})
			if !ok {
				continue
			}
			caseValue := fmt.Sprintf("%v", caseMap["value"])
			caseHandle, _ := caseMap["handle"].(string)
			if caseHandle == "" {
				continue
			}

			if val == caseValue {
				handleItems[caseHandle] = append(handleItems[caseHandle], item)
				matched = true
				if !fallthroughMode {
					break
				}
			}
		}

		if !matched {
			handleItems[defaultHandle] = append(handleItems[defaultHandle], item)
		}
	}

	// Build ordered outputs: cases first (in order), then default if populated.
	seen := make(map[string]bool)
	var outputs []workflow.NodeOutput

	for _, caseRaw := range casesRaw {
		caseMap, ok := caseRaw.(map[string]interface{})
		if !ok {
			continue
		}
		handle, _ := caseMap["handle"].(string)
		if handle == "" || seen[handle] {
			continue
		}
		seen[handle] = true
		outputs = append(outputs, workflow.NodeOutput{
			Handle: handle,
			Items:  handleItems[handle],
		})
	}

	if !seen[defaultHandle] {
		outputs = append(outputs, workflow.NodeOutput{
			Handle: defaultHandle,
			Items:  handleItems[defaultHandle],
		})
	}

	return outputs, nil
}
