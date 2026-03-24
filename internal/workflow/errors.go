package workflow

import "errors"

var (
	ErrQueueFull          = errors.New("workflow: execution queue is full")
	ErrWorkflowNotFound   = errors.New("workflow: workflow not found")
	ErrExecutionNotFound  = errors.New("workflow: execution not found")
	ErrCycleDetected      = errors.New("workflow: cycle detected in workflow graph")
	ErrInvalidConfig      = errors.New("workflow: invalid node configuration")
	ErrNodeTypeUnknown    = errors.New("workflow: unknown node type")
	ErrNoTriggerNode      = errors.New("workflow: workflow has no trigger node")
	ErrWorkflowInactive   = errors.New("workflow: workflow is not active")
	ErrExecutionCancelled = errors.New("workflow: execution was cancelled")
	ErrExecutionTimeout   = errors.New("workflow: execution timed out")
	ErrTriggerActive      = errors.New("workflow: trigger already active for this workflow")
)
