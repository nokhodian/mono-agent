package action

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

// ErrorAction defines the set of actions that can be taken when a step fails.
type ErrorAction string

const (
	ErrorActionRetry          ErrorAction = "retry"
	ErrorActionTryAlternative ErrorAction = "try_alternative"
	ErrorActionMarkFailed     ErrorAction = "mark_failed"
	ErrorActionSkip           ErrorAction = "skip"
	ErrorActionContinue       ErrorAction = "continue"
	ErrorActionAbort          ErrorAction = "abort"
)

// ErrAbort is a sentinel error indicating that execution should be aborted.
var ErrAbort = errors.New("action execution aborted")

// ErrorHandlerDef describes the error handling policy for a single step.
type ErrorHandlerDef struct {
	Action     ErrorAction `json:"action"`
	MaxRetries int         `json:"maxRetries,omitempty"`
	OnFailure  string      `json:"onFailure,omitempty"`
}

// ErrorHandler manages per-step retry counts and applies error-handling
// policies defined in the action JSON.
type ErrorHandler struct {
	mu          sync.Mutex
	retryCounts map[string]int
}

// NewErrorHandler returns an initialised ErrorHandler.
func NewErrorHandler() *ErrorHandler {
	return &ErrorHandler{
		retryCounts: make(map[string]int),
	}
}

// Handle inspects the error handler definition and the current step result,
// then returns a new StepResult that tells the executor what to do next.
//
// When def is nil the original result is returned unchanged.
func (eh *ErrorHandler) Handle(
	ctx context.Context,
	def *ErrorHandlerDef,
	result *StepResult,
	execCtx *ExecutionContext,
) *StepResult {
	if def == nil {
		return result
	}

	switch def.Action {
	case ErrorActionRetry:
		eh.mu.Lock()
		count := eh.retryCounts[result.StepID]
		eh.mu.Unlock()

		maxRetries := def.MaxRetries
		if maxRetries <= 0 {
			maxRetries = 3
		}

		if count < maxRetries {
			eh.mu.Lock()
			eh.retryCounts[result.StepID]++
			eh.mu.Unlock()

			return &StepResult{
				Success: false,
				Retry:   true,
				StepID:  result.StepID,
				Error:   result.Error,
			}
		}
		// Retries exhausted — fall through to onFailure.
		return eh.handleOnFailure(def, result, execCtx)

	case ErrorActionTryAlternative:
		return &StepResult{
			Success: false,
			Retry:   true,
			StepID:  result.StepID,
			Error:   result.Error,
		}

	case ErrorActionMarkFailed:
		if execCtx != nil {
			execCtx.AddFailedItem(FailedItem{
				StepID:    result.StepID,
				Error:     result.Error,
				Timestamp: time.Now(),
			})
		}
		return &StepResult{
			Success: false,
			StepID:  result.StepID,
			Error:   result.Error,
		}

	case ErrorActionSkip:
		return &StepResult{
			Success: false,
			Skip:    true,
			StepID:  result.StepID,
			Error:   result.Error,
		}

	case ErrorActionContinue:
		return &StepResult{
			Success: false,
			Skip:    false,
			StepID:  result.StepID,
			Error:   result.Error,
		}

	case ErrorActionAbort:
		return &StepResult{
			Success: false,
			Abort:   true,
			StepID:  result.StepID,
			Error:   ErrAbort,
		}

	default:
		return result
	}
}

// handleOnFailure processes the cascading onFailure policy after retries are
// exhausted.
func (eh *ErrorHandler) handleOnFailure(
	def *ErrorHandlerDef,
	result *StepResult,
	execCtx *ExecutionContext,
) *StepResult {
	switch ErrorAction(def.OnFailure) {
	case ErrorActionMarkFailed:
		if execCtx != nil {
			execCtx.AddFailedItem(FailedItem{
				StepID:    result.StepID,
				Error:     result.Error,
				Timestamp: time.Now(),
			})
		}
		return &StepResult{
			Success: false,
			Skip:    true,
			StepID:  result.StepID,
			Error:   result.Error,
		}

	case ErrorActionAbort:
		return &StepResult{
			Success: false,
			Abort:   true,
			StepID:  result.StepID,
			Error:   ErrAbort,
		}

	case ErrorActionSkip:
		return &StepResult{
			Success: false,
			Skip:    true,
			StepID:  result.StepID,
			Error:   result.Error,
		}

	default:
		// When no onFailure is specified, skip the step after exhausting retries.
		return &StepResult{
			Success: false,
			Skip:    true,
			StepID:  result.StepID,
			Error:   fmt.Errorf("retries exhausted for step %s: %w", result.StepID, result.Error),
		}
	}
}

// ResetRetries clears the retry counter for a specific step.
func (eh *ErrorHandler) ResetRetries(stepID string) {
	eh.mu.Lock()
	defer eh.mu.Unlock()
	delete(eh.retryCounts, stepID)
}

// WithRetry executes fn with exponential backoff. The delay is capped at 60
// seconds. It respects context cancellation.
func WithRetry(ctx context.Context, maxRetries int, baseDelay time.Duration, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("retry cancelled: %w", err)
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		if attempt < maxRetries {
			delay := time.Duration(math.Pow(2, float64(attempt))) * baseDelay
			const maxDelay = 60 * time.Second
			if delay > maxDelay {
				delay = maxDelay
			}

			select {
			case <-ctx.Done():
				return fmt.Errorf("retry cancelled during backoff: %w", ctx.Err())
			case <-time.After(delay):
			}
		}
	}
	return fmt.Errorf("all %d retries exhausted: %w", maxRetries, lastErr)
}
