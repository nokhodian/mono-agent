package control

import (
	"context"
	"fmt"
	"time"

	"github.com/monoes/monoes-agent/internal/workflow"
)

// WaitNode pauses execution for a configurable number of seconds.
// Config fields:
//
//	"duration" (float64|int, required): seconds to wait, 1–3600
type WaitNode struct{}

func (n *WaitNode) Type() string { return "core.wait" }

func (n *WaitNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	rawDuration, ok := config["duration"]
	if !ok {
		return nil, fmt.Errorf("%w: wait node requires \"duration\"", workflow.ErrInvalidConfig)
	}

	var seconds float64
	switch v := rawDuration.(type) {
	case float64:
		seconds = v
	case int:
		seconds = float64(v)
	case int64:
		seconds = float64(v)
	case float32:
		seconds = float64(v)
	default:
		return nil, fmt.Errorf("%w: wait node \"duration\" must be a number, got %T", workflow.ErrInvalidConfig, rawDuration)
	}

	if seconds < 1 || seconds > 3600 {
		return nil, fmt.Errorf("%w: wait node \"duration\" must be between 1 and 3600 seconds", workflow.ErrInvalidConfig)
	}

	select {
	case <-ctx.Done():
		return nil, workflow.ErrExecutionCancelled
	case <-time.After(time.Duration(seconds * float64(time.Second))):
	}

	return []workflow.NodeOutput{
		{Handle: "main", Items: input.Items},
	}, nil
}
