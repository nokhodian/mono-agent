package action

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/rs/zerolog"
)

// ActionRunner manages concurrent execution of queued actions using a bounded
// worker pool. It coordinates browser page acquisition, executor creation, and
// result aggregation.
type ActionRunner struct {
	maxWorkers int
	db         StorageInterface
	configMgr  ConfigInterface
	logger     zerolog.Logger
	events     chan ExecutionEvent
}

// NewActionRunner creates a runner with the specified concurrency level.
// If maxWorkers is <= 0 it defaults to 1.
func NewActionRunner(
	maxWorkers int,
	db StorageInterface,
	configMgr ConfigInterface,
	logger zerolog.Logger,
) *ActionRunner {
	if maxWorkers <= 0 {
		maxWorkers = 1
	}
	return &ActionRunner{
		maxWorkers: maxWorkers,
		db:         db,
		configMgr:  configMgr,
		logger:     logger.With().Str("component", "runner").Logger(),
		events:     make(chan ExecutionEvent, 256),
	}
}

// Events returns the read-only event channel that consumers can use to
// monitor execution progress.
func (r *ActionRunner) Events() <-chan ExecutionEvent {
	return r.events
}

// RunAll executes all supplied actions using a bounded goroutine pool
// (semaphore pattern). The pageProvider callback is responsible for returning
// a browser page and bot adapter for each action; this keeps the runner
// agnostic about browser lifecycle management.
//
// RunAll blocks until all actions have been processed or the context is
// cancelled. It returns one ExecutionResult per action (indexed the same as
// the input slice). A nil result indicates the action could not be started
// (e.g. pageProvider failed).
func (r *ActionRunner) RunAll(
	ctx context.Context,
	actions []StorageAction,
	pageProvider func(action StorageAction) (*rod.Page, BotAdapter, error),
) []ExecutionResult {
	if len(actions) == 0 {
		return nil
	}

	r.logger.Info().
		Int("actionCount", len(actions)).
		Int("maxWorkers", r.maxWorkers).
		Msg("starting action queue")

	results := make([]ExecutionResult, len(actions))
	var wg sync.WaitGroup

	// Buffered channel acts as a semaphore to limit concurrency.
	sem := make(chan struct{}, r.maxWorkers)

	for i, action := range actions {
		select {
		case <-ctx.Done():
			r.logger.Warn().Msg("context cancelled, stopping queue submission")
			// Fill remaining results with context cancellation errors.
			for j := i; j < len(actions); j++ {
				results[j] = ExecutionResult{
					FailedItems: []FailedItem{{
						StepID:    "runner",
						Error:     ctx.Err(),
						Timestamp: time.Now(),
					}},
				}
			}
			break
		default:
		}

		wg.Add(1)
		sem <- struct{}{} // Acquire worker slot.

		go func(idx int, act StorageAction) {
			defer wg.Done()
			defer func() { <-sem }() // Release worker slot.

			result := r.safeExecuteSingle(ctx, act, pageProvider)
			results[idx] = result
		}(i, action)
	}

	wg.Wait()

	// Log summary.
	var totalExtracted, totalFailed int
	for _, res := range results {
		totalExtracted += len(res.ExtractedItems)
		totalFailed += len(res.FailedItems)
	}

	r.logger.Info().
		Int("totalActions", len(actions)).
		Int("totalExtracted", totalExtracted).
		Int("totalFailed", totalFailed).
		Msg("action queue completed")

	return results
}

// RunSingle executes a single action synchronously with the given page and
// bot adapter. It creates a new ActionExecutor and runs the full action
// lifecycle.
func (r *ActionRunner) RunSingle(
	ctx context.Context,
	action StorageAction,
	page *rod.Page,
	botAdapter BotAdapter,
) (*ExecutionResult, error) {
	r.logger.Info().
		Str("actionID", action.ID).
		Str("type", action.Type).
		Str("platform", action.TargetPlatform).
		Msg("executing single action")

	executor := NewActionExecutor(ctx, page, r.db, r.configMgr, r.events, botAdapter, r.logger)
	result, err := executor.Execute(&action)
	if err != nil {
		return result, fmt.Errorf("action %s execution error: %w", action.ID, err)
	}

	return result, nil
}

// safeExecuteSingle wraps RunSingle with panic recovery and page acquisition
// via the pageProvider. It always returns an ExecutionResult (never panics).
func (r *ActionRunner) safeExecuteSingle(
	ctx context.Context,
	action StorageAction,
	pageProvider func(action StorageAction) (*rod.Page, BotAdapter, error),
) (result ExecutionResult) {
	start := time.Now()

	// Recover from panics in the executor.
	defer func() {
		if rec := recover(); rec != nil {
			result = ExecutionResult{
				FailedItems: []FailedItem{{
					StepID:    "runner_panic",
					Error:     fmt.Errorf("panic during action %s execution: %v", action.ID, rec),
					Timestamp: time.Now(),
				}},
				Duration: time.Since(start),
			}

			r.logger.Error().
				Str("actionID", action.ID).
				Interface("panic", rec).
				Msg("recovered from panic during action execution")

			// Attempt to update the action state to FAILED.
			if r.db != nil {
				_ = r.db.UpdateActionState(action.ID, "FAILED")
			}

			// Emit a failure event.
			select {
			case r.events <- ExecutionEvent{
				Type:     "action_panic",
				ActionID: action.ID,
				Message:  fmt.Sprintf("panic: %v", rec),
			}:
			default:
			}
		}
	}()

	// Acquire page and bot adapter from the provider.
	page, botAdapter, err := pageProvider(action)
	if err != nil {
		r.logger.Error().
			Err(err).
			Str("actionID", action.ID).
			Msg("failed to acquire page from provider")

		if r.db != nil {
			_ = r.db.UpdateActionState(action.ID, "FAILED")
		}

		return ExecutionResult{
			FailedItems: []FailedItem{{
				StepID:    "page_provider",
				Error:     fmt.Errorf("page provider failed for action %s: %w", action.ID, err),
				Timestamp: time.Now(),
			}},
			Duration: time.Since(start),
		}
	}

	// Execute the action.
	execResult, execErr := r.RunSingle(ctx, action, page, botAdapter)
	if execErr != nil {
		r.logger.Error().
			Err(execErr).
			Str("actionID", action.ID).
			Dur("duration", time.Since(start)).
			Msg("action failed")

		if execResult != nil {
			execResult.Duration = time.Since(start)
			return *execResult
		}

		return ExecutionResult{
			FailedItems: []FailedItem{{
				StepID:    "executor",
				Error:     execErr,
				Timestamp: time.Now(),
			}},
			Duration: time.Since(start),
		}
	}

	if execResult != nil {
		execResult.Duration = time.Since(start)

		r.logger.Info().
			Str("actionID", action.ID).
			Int("extracted", len(execResult.ExtractedItems)).
			Int("failed", len(execResult.FailedItems)).
			Int("total", execResult.TotalProcessed).
			Dur("duration", execResult.Duration).
			Msg("action completed")

		return *execResult
	}

	return ExecutionResult{Duration: time.Since(start)}
}

// Close shuts down the runner and closes the events channel. Callers should
// ensure no goroutines are still executing actions before calling Close.
func (r *ActionRunner) Close() {
	close(r.events)
}
