package control

import (
	"context"
	"fmt"

	"github.com/nokhodian/mono-agent/internal/workflow"
)

// SplitInBatchesNode splits the input items into batches of size N.
// Each batch is emitted as a separate NodeOutput on handle "batch".
// A final empty NodeOutput on handle "done" is always added.
// Config fields:
//
//	"batch_size" (int/float, required): items per batch, 1–1000
//	"reset" (bool, optional): if true, reset internal counter (not used at node level, stateless here)
type SplitInBatchesNode struct{}

func (n *SplitInBatchesNode) Type() string { return "core.split_in_batches" }

func (n *SplitInBatchesNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	var batchSize int

	switch v := config["batch_size"].(type) {
	case int:
		batchSize = v
	case int64:
		batchSize = int(v)
	case float64:
		batchSize = int(v)
	case float32:
		batchSize = int(v)
	default:
		return nil, fmt.Errorf("split_in_batches: batch_size is required and must be a number")
	}

	if batchSize < 1 || batchSize > 1000 {
		return nil, fmt.Errorf("split_in_batches: batch_size must be between 1 and 1000, got %d", batchSize)
	}

	items := input.Items
	var outputs []workflow.NodeOutput

	for len(items) > 0 {
		end := batchSize
		if end > len(items) {
			end = len(items)
		}
		batch := make([]workflow.Item, end)
		copy(batch, items[:end])
		outputs = append(outputs, workflow.NodeOutput{
			Handle: "batch",
			Items:  batch,
		})
		items = items[end:]
	}

	// Always append a final "done" output with no items.
	outputs = append(outputs, workflow.NodeOutput{
		Handle: "done",
		Items:  []workflow.Item{},
	})

	return outputs, nil
}
