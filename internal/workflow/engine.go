package workflow

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nokhodian/mono-agent/internal/connections"
	"github.com/rs/zerolog"
)

// WorkflowEngine is the central coordinator for workflow management and execution.
// It owns the queue, trigger manager, webhook server, node registry, and store.
type WorkflowEngine struct {
	store          WorkflowStore
	connStore      *connections.Store
	registry       *NodeTypeRegistry
	queue          *ExecutionQueue
	triggerMgr     *TriggerManager
	webhookServer  *WebhookServer
	expr           *ExpressionEngine
	logger         zerolog.Logger
	mu             sync.RWMutex
	cancelFuncs    map[string]context.CancelFunc // executionID → cancel
	ctx            context.Context
	cancel         context.CancelFunc
	pruneInterval  time.Duration
	maxExecHistory int
}

// EngineConfig holds WorkflowEngine configuration.
type EngineConfig struct {
	WebhookAddr    string        // e.g. ":9321"
	MaxConcurrent  int           // default 3, max 20
	QueueCapacity  int           // default 1000
	PruneInterval  time.Duration // default 1h
	MaxExecHistory int           // default 500
}

// NewWorkflowEngine creates a fully wired engine. Call Start() to begin processing.
// NewWorkflowEngineWithStore creates a WorkflowEngine using a caller-supplied
// WorkflowStore. Use this when you need a HybridWorkflowStore (file store +
// SQLite). db is still required for the connections store.
func NewWorkflowEngineWithStore(store WorkflowStore, db *sql.DB, scheduler SchedulerInterface, registry *NodeTypeRegistry, cfg EngineConfig, logger zerolog.Logger) *WorkflowEngine {
	applyEngineDefaults(&cfg)
	connStore := connections.NewStore(db)
	webhookServer := NewWebhookServer(cfg.WebhookAddr, logger)
	e := &WorkflowEngine{
		store:          store,
		connStore:      connStore,
		registry:       registry,
		webhookServer:  webhookServer,
		expr:           NewExpressionEngine(),
		logger:         logger,
		cancelFuncs:    make(map[string]context.CancelFunc),
		pruneInterval:  cfg.PruneInterval,
		maxExecHistory: cfg.MaxExecHistory,
	}
	e.triggerMgr = NewTriggerManager(store, webhookServer, scheduler,
		func(workflowID string, nodeID string, items []Item) { e.handleTrigger(workflowID, nodeID, items) },
		logger,
	)
	e.queue = NewExecutionQueue(cfg.QueueCapacity, cfg.MaxConcurrent,
		func(ctx context.Context, req ExecutionRequest) { e.handleExecution(ctx, req) },
		logger,
	)
	return e
}

func applyEngineDefaults(cfg *EngineConfig) {
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 3
	}
	if cfg.MaxConcurrent > 20 {
		cfg.MaxConcurrent = 20
	}
	if cfg.QueueCapacity <= 0 {
		cfg.QueueCapacity = 1000
	}
	if cfg.PruneInterval <= 0 {
		cfg.PruneInterval = time.Hour
	}
	if cfg.MaxExecHistory <= 0 {
		cfg.MaxExecHistory = 500
	}
	if cfg.WebhookAddr == "" {
		cfg.WebhookAddr = ":9321"
	}
}

func NewWorkflowEngine(db *sql.DB, scheduler SchedulerInterface, registry *NodeTypeRegistry, cfg EngineConfig, logger zerolog.Logger) *WorkflowEngine {
	applyEngineDefaults(&cfg)

	store := NewSQLiteWorkflowStore(db)
	connStore := connections.NewStore(db)
	webhookServer := NewWebhookServer(cfg.WebhookAddr, logger)

	e := &WorkflowEngine{
		store:          store,
		connStore:      connStore,
		registry:       registry,
		webhookServer:  webhookServer,
		expr:           NewExpressionEngine(),
		logger:         logger,
		cancelFuncs:    make(map[string]context.CancelFunc),
		pruneInterval:  cfg.PruneInterval,
		maxExecHistory: cfg.MaxExecHistory,
	}

	// Wire trigger manager with a handleTrigger closure.
	e.triggerMgr = NewTriggerManager(
		store,
		webhookServer,
		scheduler,
		func(workflowID string, nodeID string, items []Item) {
			e.handleTrigger(workflowID, nodeID, items)
		},
		logger,
	)

	// Wire execution queue with handleExecution as the handler.
	e.queue = NewExecutionQueue(
		cfg.QueueCapacity,
		cfg.MaxConcurrent,
		func(ctx context.Context, req ExecutionRequest) {
			e.handleExecution(ctx, req)
		},
		logger,
	)

	return e
}

// Start initializes the engine: starts the queue workers, webhook server,
// recovers stale executions, re-registers triggers for active workflows,
// and starts the prune loop.
func (e *WorkflowEngine) Start(ctx context.Context) error {
	e.ctx, e.cancel = context.WithCancel(ctx)

	// 1. Start webhook server.
	if err := e.webhookServer.Start(); err != nil {
		return fmt.Errorf("engine: start webhook server: %w", err)
	}

	// 2. Start queue workers.
	e.queue.Start(e.ctx)

	// 3. Recover stale executions left over from a prior crash or restart.
	if err := e.store.RecoverStaleExecutions(e.ctx); err != nil {
		e.logger.Warn().Err(err).Msg("engine: failed to recover stale executions")
	}

	// 4. Re-register triggers for all active workflows.
	if err := e.reregisterTriggers(e.ctx); err != nil {
		e.logger.Warn().Err(err).Msg("engine: failed to re-register some triggers on startup")
	}

	// 5. Start prune loop.
	go e.pruneLoop(e.ctx)

	e.logger.Info().Msg("workflow engine started")
	return nil
}

// Stop gracefully shuts down all components.
func (e *WorkflowEngine) Stop() error {
	e.logger.Info().Msg("workflow engine stopping")

	// Signal all goroutines driven by e.ctx.
	if e.cancel != nil {
		e.cancel()
	}

	// Deactivate all triggers (stops cron jobs and deregisters webhooks).
	e.triggerMgr.DeactivateAll()

	// Stop the queue — drains in-flight work and waits for workers to exit.
	e.queue.Stop()

	// Shut down the webhook HTTP server.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := e.webhookServer.Stop(shutdownCtx); err != nil {
		e.logger.Warn().Err(err).Msg("engine: webhook server shutdown error")
	}

	e.logger.Info().Msg("workflow engine stopped")
	return nil
}

// pruneLoop runs on the prune interval and culls old execution history for
// every workflow.
func (e *WorkflowEngine) pruneLoop(ctx context.Context) {
	ticker := time.NewTicker(e.pruneInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.runPrune(ctx)
		}
	}
}

// runPrune iterates over all workflows and prunes execution history.
func (e *WorkflowEngine) runPrune(ctx context.Context) {
	workflows, err := e.store.ListWorkflows(ctx)
	if err != nil {
		e.logger.Warn().Err(err).Msg("engine: prune: failed to list workflows")
		return
	}
	for _, wf := range workflows {
		if err := e.store.PruneExecutions(ctx, wf.ID, e.maxExecHistory); err != nil {
			e.logger.Warn().Err(err).
				Str("workflow_id", wf.ID).
				Msg("engine: prune: failed to prune executions")
		}
	}
}

// reregisterTriggers loads all active workflows and activates their triggers.
func (e *WorkflowEngine) reregisterTriggers(ctx context.Context) error {
	workflows, err := e.store.ListWorkflows(ctx)
	if err != nil {
		return fmt.Errorf("list workflows: %w", err)
	}

	var firstErr error
	for _, wf := range workflows {
		if !wf.IsActive {
			continue
		}
		// Load the full workflow (with nodes) for trigger registration.
		full, err := e.store.GetWorkflow(ctx, wf.ID)
		if err != nil {
			e.logger.Warn().Err(err).
				Str("workflow_id", wf.ID).
				Msg("engine: reregister triggers: failed to load workflow")
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if full == nil {
			continue
		}
		if err := e.triggerMgr.ActivateWorkflow(ctx, full); err != nil {
			e.logger.Warn().Err(err).
				Str("workflow_id", wf.ID).
				Msg("engine: reregister triggers: failed to activate workflow triggers")
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// handleTrigger is called by TriggerManager whenever a trigger fires.
// It creates a WorkflowExecution record and enqueues it for execution.
func (e *WorkflowEngine) handleTrigger(workflowID string, nodeID string, items []Item) {
	ctx := e.ctx

	// Determine trigger type from the node.  We load the workflow to find the
	// node's Type field.
	triggerType := "unknown"
	wf, err := e.store.GetWorkflow(ctx, workflowID)
	if err == nil && wf != nil {
		if !wf.IsActive {
			e.logger.Warn().
				Str("workflow_id", workflowID).
				Msg("engine: handleTrigger: workflow is not active, ignoring trigger")
			return
		}
		for _, n := range wf.Nodes {
			if n.ID == nodeID {
				triggerType = n.Type
				break
			}
		}
	}

	// Build trigger data from the first item (if any).
	triggerData := make(map[string]interface{})
	if len(items) > 0 && items[0].JSON != nil {
		triggerData = items[0].JSON
	}

	exec := &WorkflowExecution{
		WorkflowID:  workflowID,
		Status:      "QUEUED",
		TriggerType: triggerType,
		TriggerData: triggerData,
	}

	if err := e.store.CreateExecution(ctx, exec); err != nil {
		e.logger.Error().Err(err).
			Str("workflow_id", workflowID).
			Str("node_id", nodeID).
			Msg("engine: handleTrigger: failed to create execution record")
		return
	}

	req := ExecutionRequest{
		WorkflowID:  workflowID,
		ExecutionID: exec.ID,
		TriggerType: triggerType,
		TriggerData: triggerData,
	}

	if err := e.queue.Enqueue(req); err != nil {
		// Queue is full — mark the execution as FAILED immediately.
		e.logger.Warn().
			Str("execution_id", exec.ID).
			Str("workflow_id", workflowID).
			Msg("engine: handleTrigger: queue full, execution will not run")
		if updateErr := e.store.SetExecutionFinished(ctx, exec.ID, "FAILED", "queue full"); updateErr != nil {
			e.logger.Error().Err(updateErr).
				Str("execution_id", exec.ID).
				Msg("engine: handleTrigger: failed to mark execution as FAILED after queue full")
		}
	}
}

// handleExecution is called by a queue worker for each execution request.
func (e *WorkflowEngine) handleExecution(ctx context.Context, req ExecutionRequest) {
	log := e.logger.With().
		Str("execution_id", req.ExecutionID).
		Str("workflow_id", req.WorkflowID).
		Logger()

	// 1. Load the workflow.
	wf, err := e.store.GetWorkflow(ctx, req.WorkflowID)
	if err != nil {
		log.Error().Err(err).Msg("engine: handleExecution: failed to load workflow")
		_ = e.store.SetExecutionFinished(ctx, req.ExecutionID, "FAILED", fmt.Sprintf("load workflow: %s", err.Error()))
		return
	}
	if wf == nil {
		log.Error().Msg("engine: handleExecution: workflow not found")
		_ = e.store.SetExecutionFinished(ctx, req.ExecutionID, "FAILED", "workflow not found")
		return
	}

	// 2. Update execution status to RUNNING, record started_at.
	if err := e.store.SetExecutionStarted(ctx, req.ExecutionID); err != nil {
		log.Error().Err(err).Msg("engine: handleExecution: failed to mark execution as RUNNING")
		// Non-fatal — attempt to continue.
	}

	// 3. Build the DAG.
	dag, err := BuildDAG(wf.Nodes, wf.Connections)
	if err != nil {
		log.Error().Err(err).Msg("engine: handleExecution: failed to build DAG")
		_ = e.store.SetExecutionFinished(ctx, req.ExecutionID, "FAILED", fmt.Sprintf("build dag: %s", err.Error()))
		return
	}

	// Load the full execution record (with trigger data) so runExecution has it.
	exec, err := e.store.GetExecution(ctx, req.ExecutionID)
	if err != nil || exec == nil {
		log.Error().Err(err).Msg("engine: handleExecution: failed to load execution record")
		_ = e.store.SetExecutionFinished(ctx, req.ExecutionID, "FAILED", "could not load execution record")
		return
	}

	// 4. Execute via runExecution (defined in execution.go).
	runErr := e.runExecution(ctx, exec, wf, dag)

	// 5. On completion: update execution status to SUCCESS or FAILED.
	// Use a detached context so DB writes succeed even if the execution context
	// was cancelled (e.g. by engine shutdown or a browser panic).
	persistCtx, persistCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer persistCancel()
	if runErr != nil {
		errMsg := runErr.Error()
		finalStatus := "FAILED"
		if strings.Contains(errMsg, ErrExecutionCancelled.Error()) {
			finalStatus = "CANCELLED"
		}
		log.Warn().Err(runErr).Str("final_status", finalStatus).Msg("engine: execution finished with error")
		_ = e.store.SetExecutionFinished(persistCtx, req.ExecutionID, finalStatus, errMsg)
	} else {
		log.Info().Msg("engine: execution finished successfully")
		_ = e.store.SetExecutionFinished(persistCtx, req.ExecutionID, "SUCCESS", "")
	}
}

// ---------------------------------------------------------------------------
// Workflow lifecycle
// ---------------------------------------------------------------------------

// CreateWorkflow saves a new workflow (inactive by default).
func (e *WorkflowEngine) CreateWorkflow(ctx context.Context, w *Workflow) error {
	w.IsActive = false
	if err := e.store.CreateWorkflow(ctx, w); err != nil {
		return fmt.Errorf("engine: create workflow: %w", err)
	}
	if len(w.Nodes) > 0 {
		if err := e.store.SaveWorkflowNodes(ctx, w.ID, w.Nodes); err != nil {
			return fmt.Errorf("engine: create workflow nodes: %w", err)
		}
	}
	if len(w.Connections) > 0 {
		if err := e.store.SaveWorkflowConnections(ctx, w.ID, w.Connections); err != nil {
			return fmt.Errorf("engine: create workflow connections: %w", err)
		}
	}
	e.logger.Info().Str("workflow_id", w.ID).Str("name", w.Name).Msg("engine: workflow created")
	return nil
}

// SaveWorkflow updates a workflow's definition (nodes + connections).
// If the workflow is currently active, it is deactivated first, then saved,
// then reactivated.
func (e *WorkflowEngine) SaveWorkflow(ctx context.Context, w *Workflow) error {
	// Load current state to check IsActive.
	existing, err := e.store.GetWorkflow(ctx, w.ID)
	if err != nil {
		return fmt.Errorf("engine: save workflow: load existing: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("engine: save workflow: %w", ErrWorkflowNotFound)
	}

	wasActive := existing.IsActive

	// Deactivate triggers if currently active.
	if wasActive {
		e.triggerMgr.DeactivateWorkflow(w.ID)
		if err := e.store.SetWorkflowActive(ctx, w.ID, false); err != nil {
			return fmt.Errorf("engine: save workflow: deactivate: %w", err)
		}
	}

	// Preserve the active flag from the existing record; caller controls it
	// via Activate/DeactivateWorkflow.
	w.IsActive = false

	if err := e.store.UpdateWorkflow(ctx, w); err != nil {
		return fmt.Errorf("engine: save workflow: update: %w", err)
	}
	if err := e.store.SaveWorkflowNodes(ctx, w.ID, w.Nodes); err != nil {
		return fmt.Errorf("engine: save workflow: save nodes: %w", err)
	}
	if err := e.store.SaveWorkflowConnections(ctx, w.ID, w.Connections); err != nil {
		return fmt.Errorf("engine: save workflow: save connections: %w", err)
	}

	// Reactivate if it was active before the save.
	if wasActive {
		if err := e.ActivateWorkflow(ctx, w.ID); err != nil {
			return fmt.Errorf("engine: save workflow: reactivate: %w", err)
		}
	}

	e.logger.Info().Str("workflow_id", w.ID).Msg("engine: workflow saved")
	return nil
}

// DeleteWorkflow deactivates and deletes a workflow.
func (e *WorkflowEngine) DeleteWorkflow(ctx context.Context, id string) error {
	existing, err := e.store.GetWorkflow(ctx, id)
	if err != nil {
		return fmt.Errorf("engine: delete workflow: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("engine: delete workflow: %w", ErrWorkflowNotFound)
	}

	if existing.IsActive {
		e.triggerMgr.DeactivateWorkflow(id)
	}

	if err := e.store.DeleteWorkflow(ctx, id); err != nil {
		return fmt.Errorf("engine: delete workflow: %w", err)
	}

	e.logger.Info().Str("workflow_id", id).Msg("engine: workflow deleted")
	return nil
}

// ActivateWorkflow enables a workflow and registers its triggers.
func (e *WorkflowEngine) ActivateWorkflow(ctx context.Context, id string) error {
	wf, err := e.store.GetWorkflow(ctx, id)
	if err != nil {
		return fmt.Errorf("engine: activate workflow: %w", err)
	}
	if wf == nil {
		return fmt.Errorf("engine: activate workflow: %w", ErrWorkflowNotFound)
	}

	if err := e.store.SetWorkflowActive(ctx, id, true); err != nil {
		return fmt.Errorf("engine: activate workflow: %w", err)
	}
	wf.IsActive = true

	if err := e.triggerMgr.ActivateWorkflow(ctx, wf); err != nil {
		// Revert the active flag so the DB stays consistent.
		_ = e.store.SetWorkflowActive(ctx, id, false)
		return fmt.Errorf("engine: activate workflow: register triggers: %w", err)
	}

	e.logger.Info().Str("workflow_id", id).Msg("engine: workflow activated")
	return nil
}

// DeactivateWorkflow disables a workflow and unregisters its triggers.
func (e *WorkflowEngine) DeactivateWorkflow(ctx context.Context, id string) error {
	existing, err := e.store.GetWorkflow(ctx, id)
	if err != nil {
		return fmt.Errorf("engine: deactivate workflow: %w", err)
	}
	if existing == nil {
		return fmt.Errorf("engine: deactivate workflow: %w", ErrWorkflowNotFound)
	}

	e.triggerMgr.DeactivateWorkflow(id)

	if err := e.store.SetWorkflowActive(ctx, id, false); err != nil {
		return fmt.Errorf("engine: deactivate workflow: %w", err)
	}

	e.logger.Info().Str("workflow_id", id).Msg("engine: workflow deactivated")
	return nil
}

// ---------------------------------------------------------------------------
// Execution management
// ---------------------------------------------------------------------------

// TriggerWorkflow manually triggers a workflow (for manual trigger nodes).
// Returns the new execution ID.
func (e *WorkflowEngine) TriggerWorkflow(ctx context.Context, workflowID string, data map[string]interface{}) (string, error) {
	wf, err := e.store.GetWorkflow(ctx, workflowID)
	if err != nil {
		return "", fmt.Errorf("engine: trigger workflow: %w", err)
	}
	if wf == nil {
		return "", fmt.Errorf("engine: trigger workflow: %w", ErrWorkflowNotFound)
	}
	if !wf.IsActive {
		return "", fmt.Errorf("engine: trigger workflow: %w", ErrWorkflowInactive)
	}

	if data == nil {
		data = make(map[string]interface{})
	}

	exec := &WorkflowExecution{
		WorkflowID:  workflowID,
		Status:      "QUEUED",
		TriggerType: "trigger.manual",
		TriggerData: data,
	}

	if err := e.store.CreateExecution(ctx, exec); err != nil {
		return "", fmt.Errorf("engine: trigger workflow: create execution: %w", err)
	}

	req := ExecutionRequest{
		WorkflowID:  workflowID,
		ExecutionID: exec.ID,
		TriggerType: "trigger.manual",
		TriggerData: data,
	}

	if err := e.queue.Enqueue(req); err != nil {
		_ = e.store.SetExecutionFinished(ctx, exec.ID, "FAILED", "queue full")
		return exec.ID, fmt.Errorf("engine: trigger workflow: %w", ErrQueueFull)
	}

	e.logger.Info().
		Str("workflow_id", workflowID).
		Str("execution_id", exec.ID).
		Msg("engine: manual trigger queued")
	return exec.ID, nil
}

// CancelExecution signals an in-flight execution to stop.
func (e *WorkflowEngine) CancelExecution(executionID string) {
	// The queue manages per-execution contexts and cancel funcs.
	e.queue.Cancel(executionID)
	e.logger.Info().Str("execution_id", executionID).Msg("engine: execution cancel requested")
}

// RetryExecution re-queues a failed execution as a new execution.
func (e *WorkflowEngine) RetryExecution(ctx context.Context, executionID string) (string, error) {
	orig, err := e.store.GetExecution(ctx, executionID)
	if err != nil {
		return "", fmt.Errorf("engine: retry execution: %w", err)
	}
	if orig == nil {
		return "", fmt.Errorf("engine: retry execution: %w", ErrExecutionNotFound)
	}

	wf, err := e.store.GetWorkflow(ctx, orig.WorkflowID)
	if err != nil {
		return "", fmt.Errorf("engine: retry execution: load workflow: %w", err)
	}
	if wf == nil {
		return "", fmt.Errorf("engine: retry execution: %w", ErrWorkflowNotFound)
	}

	exec := &WorkflowExecution{
		WorkflowID:  orig.WorkflowID,
		Status:      "QUEUED",
		TriggerType: orig.TriggerType,
		TriggerData: orig.TriggerData,
	}

	if err := e.store.CreateExecution(ctx, exec); err != nil {
		return "", fmt.Errorf("engine: retry execution: create new execution: %w", err)
	}

	req := ExecutionRequest{
		WorkflowID:  orig.WorkflowID,
		ExecutionID: exec.ID,
		TriggerType: orig.TriggerType,
		TriggerData: orig.TriggerData,
	}

	if err := e.queue.Enqueue(req); err != nil {
		_ = e.store.SetExecutionFinished(ctx, exec.ID, "FAILED", "queue full")
		return exec.ID, fmt.Errorf("engine: retry execution: %w", ErrQueueFull)
	}

	e.logger.Info().
		Str("original_execution_id", executionID).
		Str("new_execution_id", exec.ID).
		Msg("engine: execution retry queued")
	return exec.ID, nil
}

// ---------------------------------------------------------------------------
// Query helpers
// ---------------------------------------------------------------------------

// GetWorkflow loads a workflow with all nodes and connections.
func (e *WorkflowEngine) GetWorkflow(ctx context.Context, id string) (*Workflow, error) {
	wf, err := e.store.GetWorkflow(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("engine: get workflow: %w", err)
	}
	if wf == nil {
		return nil, fmt.Errorf("engine: get workflow: %w", ErrWorkflowNotFound)
	}
	return wf, nil
}

// ListWorkflows returns all workflows.
func (e *WorkflowEngine) ListWorkflows(ctx context.Context) ([]Workflow, error) {
	workflows, err := e.store.ListWorkflows(ctx)
	if err != nil {
		return nil, fmt.Errorf("engine: list workflows: %w", err)
	}
	return workflows, nil
}

// GetExecution loads a workflow execution with all node results.
func (e *WorkflowEngine) GetExecution(ctx context.Context, id string) (*WorkflowExecution, error) {
	exec, err := e.store.GetExecution(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("engine: get execution: %w", err)
	}
	if exec == nil {
		return nil, fmt.Errorf("engine: get execution: %w", ErrExecutionNotFound)
	}
	return exec, nil
}

// ListExecutions returns recent executions for a workflow.
func (e *WorkflowEngine) ListExecutions(ctx context.Context, workflowID string, limit int) ([]WorkflowExecution, error) {
	executions, err := e.store.ListExecutions(ctx, workflowID, limit)
	if err != nil {
		return nil, fmt.Errorf("engine: list executions: %w", err)
	}
	return executions, nil
}

// runExecution delegates to RunExecution defined in execution.go.
func (e *WorkflowEngine) runExecution(ctx context.Context, exec *WorkflowExecution, wf *Workflow, dag *DAG) error {
	return RunExecution(ctx, exec, wf, dag, e.registry, e.store, e.connStore, e.expr, e.logger)
}
