package workflow

import (
	"context"
	"sync"

	"github.com/rs/zerolog"
)

// ExecutionRequest is enqueued when a workflow execution is triggered.
type ExecutionRequest struct {
	WorkflowID  string
	ExecutionID string
	TriggerType string
	TriggerData map[string]interface{}
}

// ExecutionQueue manages a bounded channel of pending workflow execution requests.
// A pool of worker goroutines drains the queue.
type ExecutionQueue struct {
	ch          chan ExecutionRequest
	workers     int
	wg          sync.WaitGroup
	cancelFuncs sync.Map // executionID → context.CancelFunc
	handler     func(ctx context.Context, req ExecutionRequest)
	logger      zerolog.Logger
}

// NewExecutionQueue creates a queue with the given buffer capacity and worker count.
// handler is called for each dequeued request in its own goroutine context.
func NewExecutionQueue(capacity int, workers int, handler func(ctx context.Context, req ExecutionRequest), logger zerolog.Logger) *ExecutionQueue {
	return &ExecutionQueue{
		ch:      make(chan ExecutionRequest, capacity),
		workers: workers,
		handler: handler,
		logger:  logger,
	}
}

// Start launches the worker goroutines. Must be called before Enqueue.
// The provided context governs worker lifetime.
func (q *ExecutionQueue) Start(ctx context.Context) {
	for i := 0; i < q.workers; i++ {
		workerID := i
		q.wg.Add(1)
		go func() {
			defer q.wg.Done()
			q.logger.Info().Int("worker_id", workerID).Msg("workflow queue worker started")
			defer q.logger.Info().Int("worker_id", workerID).Msg("workflow queue worker stopped")

			for {
				select {
				case <-ctx.Done():
					return
				case req, ok := <-q.ch:
					if !ok {
						// channel closed by Stop()
						return
					}
					q.dispatch(ctx, req, workerID)
				}
			}
		}()
	}
}

// dispatch runs a single request: creates a cancellable child context, stores the
// cancel func, calls the handler, then cleans up.
func (q *ExecutionQueue) dispatch(ctx context.Context, req ExecutionRequest, workerID int) {
	execCtx, cancel := context.WithCancel(ctx)
	q.cancelFuncs.Store(req.ExecutionID, cancel)

	defer func() {
		q.cancelFuncs.Delete(req.ExecutionID)
		cancel()
		if r := recover(); r != nil {
			q.logger.Error().
				Int("worker_id", workerID).
				Str("execution_id", req.ExecutionID).
				Str("workflow_id", req.WorkflowID).
				Interface("panic", r).
				Msg("workflow handler panicked")
		}
	}()

	q.logger.Debug().
		Int("worker_id", workerID).
		Str("execution_id", req.ExecutionID).
		Str("workflow_id", req.WorkflowID).
		Str("trigger_type", req.TriggerType).
		Msg("dispatching execution request")

	q.handler(execCtx, req)
}

// Stop closes the channel and blocks until all in-flight workers have exited.
func (q *ExecutionQueue) Stop() {
	close(q.ch)
	q.wg.Wait()
}

// Enqueue adds a request to the queue. Returns ErrQueueFull if the buffer is full.
func (q *ExecutionQueue) Enqueue(req ExecutionRequest) error {
	select {
	case q.ch <- req:
		q.logger.Debug().
			Str("execution_id", req.ExecutionID).
			Str("workflow_id", req.WorkflowID).
			Msg("execution request enqueued")
		return nil
	default:
		q.logger.Warn().
			Str("execution_id", req.ExecutionID).
			Str("workflow_id", req.WorkflowID).
			Msg("execution queue full, request rejected")
		return ErrQueueFull
	}
}

// Cancel signals cancellation for a specific execution.
func (q *ExecutionQueue) Cancel(executionID string) {
	if val, ok := q.cancelFuncs.Load(executionID); ok {
		if cancel, ok := val.(context.CancelFunc); ok {
			cancel()
			q.logger.Info().
				Str("execution_id", executionID).
				Msg("execution cancellation signalled")
		}
	}
}

// Len returns the current number of items waiting in the queue.
func (q *ExecutionQueue) Len() int {
	return len(q.ch)
}
