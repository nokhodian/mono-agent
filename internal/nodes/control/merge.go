package control

import (
	"context"

	"github.com/monoes/monoes-agent/internal/workflow"
)

// MergeNode collects items from all input branches and emits a single combined output.
// Config fields:
//
//	"mode" (string, optional): "append" (default) = concatenate all input items into one output slice
//	                            "first" = pass through items from the first branch that arrives
//
// Note: In the workflow engine, MergeNode.Execute is called once ALL predecessors have completed.
// The engine passes ALL items from all predecessor outputs in input.Items (already merged by engine).
// MergeNode just re-emits them on "main" handle.
type MergeNode struct{}

func (n *MergeNode) Type() string { return "core.merge" }

func (n *MergeNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	return []workflow.NodeOutput{
		{Handle: "main", Items: input.Items},
	}, nil
}
