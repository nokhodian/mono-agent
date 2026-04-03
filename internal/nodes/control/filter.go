package control

import (
	"context"
	"fmt"

	"github.com/nokhodian/mono-agent/internal/workflow"
)

// FilterNode filters items by evaluating a boolean expression on each item.
// Config fields:
//
//	"condition" (string, required): boolean expression template
//	"mode" (string, optional): "keep" (default) = keep items where condition is true,
//	                           "remove" = keep items where condition is false
//
// Items that pass the condition are emitted on "main".
// Items that do not pass are emitted on "rejected".
type FilterNode struct{}

func (n *FilterNode) Type() string { return "core.filter" }

func (n *FilterNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	condition, _ := config["condition"].(string)
	if condition == "" {
		return nil, fmt.Errorf("%w: filter node requires \"condition\"", workflow.ErrInvalidConfig)
	}

	mode, _ := config["mode"].(string)
	if mode == "" {
		mode = "keep"
	}
	if mode != "keep" && mode != "remove" {
		return nil, fmt.Errorf("%w: filter node \"mode\" must be \"keep\" or \"remove\"", workflow.ErrInvalidConfig)
	}

	engine := workflow.NewExpressionEngine()

	passed := make([]workflow.Item, 0)
	rejected := make([]workflow.Item, 0)

	for _, item := range input.Items {
		exprCtx := workflow.ExpressionContext{
			JSON:        item.JSON,
			Node:        input.NodeOutputs,
			WorkflowID:  input.WorkflowID,
			ExecutionID: input.ExecutionID,
		}

		result, err := engine.EvaluateBool(condition, exprCtx)
		if err != nil {
			// On evaluation error, treat condition as false.
			result = false
		}

		// Decide whether to keep this item based on mode.
		keep := (mode == "keep" && result) || (mode == "remove" && !result)
		if keep {
			passed = append(passed, item)
		} else {
			rejected = append(rejected, item)
		}
	}

	return []workflow.NodeOutput{
		{Handle: "main", Items: passed},
		{Handle: "rejected", Items: rejected},
	}, nil
}
