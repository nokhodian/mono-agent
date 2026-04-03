package control

import (
	"context"

	"github.com/nokhodian/mono-agent/internal/workflow"
)

// IfNode evaluates a boolean expression and routes items to "true" or "false" output handles.
// Config fields:
//
//	"condition" (string, required): expression evaluated as bool, e.g. "{{gt ($json.count | toFloat) 10}}"
//	"mode" (string, optional): "all" (default) = route all items to same handle, "per_item" = route each item independently
type IfNode struct{}

func (n *IfNode) Type() string { return "core.if" }

func (n *IfNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	condition, _ := config["condition"].(string)
	mode, _ := config["mode"].(string)
	if mode == "" {
		mode = "all"
	}

	engine := workflow.NewExpressionEngine()

	trueItems := make([]workflow.Item, 0)
	falseItems := make([]workflow.Item, 0)

	if mode == "all" {
		// Evaluate condition using the first item's context (or empty if no items).
		exprCtx := workflow.ExpressionContext{
			Node:        input.NodeOutputs,
			WorkflowID:  input.WorkflowID,
			ExecutionID: input.ExecutionID,
		}
		if len(input.Items) > 0 {
			exprCtx.JSON = input.Items[0].JSON
		}

		result, err := engine.EvaluateBool(condition, exprCtx)
		if err != nil {
			result = false
		}

		if result {
			trueItems = append(trueItems, input.Items...)
		} else {
			falseItems = append(falseItems, input.Items...)
		}
	} else {
		// per_item: evaluate condition for each item independently.
		for _, item := range input.Items {
			exprCtx := workflow.ExpressionContext{
				JSON:        item.JSON,
				Node:        input.NodeOutputs,
				WorkflowID:  input.WorkflowID,
				ExecutionID: input.ExecutionID,
			}

			result, err := engine.EvaluateBool(condition, exprCtx)
			if err != nil {
				result = false
			}

			if result {
				trueItems = append(trueItems, item)
			} else {
				falseItems = append(falseItems, item)
			}
		}
	}

	return []workflow.NodeOutput{
		{Handle: "true", Items: trueItems},
		{Handle: "false", Items: falseItems},
	}, nil
}
