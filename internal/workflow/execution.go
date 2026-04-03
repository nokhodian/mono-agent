package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"runtime/debug"
	"strings"
	"time"

	"github.com/monoes/monoes-agent/internal/connections"
	"github.com/rs/zerolog"
)

// RunExecution executes a workflow against its DAG. Called by WorkflowEngine.
// This is the core BFS execution loop.
func RunExecution(
	ctx context.Context,
	exec *WorkflowExecution,
	wf *Workflow,
	dag *DAG,
	registry *NodeTypeRegistry,
	store WorkflowStore,
	connStore *connections.Store,
	expr *ExpressionEngine,
	logger zerolog.Logger,
) error {
	// Phase 1: Initialize
	triggerNodes := dag.TriggerNodes()
	if len(triggerNodes) == 0 {
		return ErrNoTriggerNode
	}

	// Build initial trigger items — wrap TriggerData as a single Item.
	triggerItems := buildTriggerItems(exec.TriggerData)

	// nodeOutputs accumulates the "main" handle output for each node by name.
	// Used for $node["Name"] expression access.
	nodeOutputs := make(map[string][]Item)

	// pendingInputs accumulates items routed to each node by its ID.
	pendingInputs := make(map[string][]Item)

	// triggerNodeIDs is a set of trigger node IDs for quick lookup.
	triggerNodeIDs := make(map[string]bool, len(triggerNodes))
	for _, tn := range triggerNodes {
		triggerNodeIDs[tn.ID] = true
	}

	// mergeWaiting tracks how many predecessors still need to complete
	// before a merge node runs.  A node is treated as a merge node when it
	// has more than one incoming edge.  The counter is initialised to
	// InDegree(nodeID) - 1 on the first predecessor completion, then
	// decremented by each subsequent predecessor until it reaches zero.
	mergeWaiting := make(map[string]int)

	// Phase 2: BFS execution loop — process nodes in topological order.
	order, err := dag.TopologicalSort()
	if err != nil {
		return err
	}

	// completedNodes tracks which nodes have finished (by ID) so we can
	// manage the merge counters.
	completedNodes := make(map[string]bool, len(order))

	for _, node := range order {
		// Check context cancellation.
		select {
		case <-ctx.Done():
			return ErrExecutionCancelled
		default:
		}

		// Skip disabled nodes; still mark their successors so mergeWaiting
		// is decremented correctly.
		if node.Disabled {
			logger.Debug().
				Str("node_id", node.ID).
				Str("node_name", node.Name).
				Msg("skipping disabled node")
			completedNodes[node.ID] = true
			decrementMergeWaiting(node.ID, dag, mergeWaiting, completedNodes)
			continue
		}

		// Determine whether this is a merge node (InDegree > 1).
		inDeg := dag.InDegree(node.ID)
		if inDeg > 1 {
			// On first encounter (before any predecessor has completed) we have
			// nothing to wait for yet — skip until all predecessors are done.
			if _, initialised := mergeWaiting[node.ID]; !initialised {
				// This shouldn't happen in topological order (all predecessors
				// run before us), but guard defensively.
				logger.Warn().
					Str("node_id", node.ID).
					Str("node_name", node.Name).
					Msg("merge node reached with uninitialised counter; skipping")
				continue
			}
			if mergeWaiting[node.ID] > 0 {
				// Still waiting for more predecessors.
				continue
			}
		}

		// Determine input items.
		var inputItems []Item
		if triggerNodeIDs[node.ID] {
			inputItems = triggerItems
		} else {
			inputItems = pendingInputs[node.ID]
		}
		if inputItems == nil {
			inputItems = []Item{}
		}

		// Parse node config.
		nodeCopy := node
		if err := nodeCopy.ParseConfig(); err != nil {
			return fmt.Errorf("node %s (%s): parse config: %w", node.ID, node.Name, err)
		}
		config := nodeCopy.Config
		if config == nil {
			config = make(map[string]interface{})
		}

		// Inject credential data if credential_id is present.
		// Lookup order:
		//   1. If credID does NOT start with "wc_", try the new connections table first.
		//   2. Fall back to the legacy workflow_credentials table (backward compat).
		if credIDRaw, ok := config["credential_id"]; ok {
			if credID, ok := credIDRaw.(string); ok && credID != "" {
				injected := false

				// Try the connections table unless the ID looks like a legacy wc_ credential.
				if !strings.HasPrefix(credID, "wc_") && connStore != nil {
					conn, err := connStore.Get(ctx, credID)
					if err != nil {
						// DB error — log and fall back to legacy table
						fmt.Printf("warning: connections lookup for %s failed: %v; falling back to workflow_credentials\n", credID, err)
						// fall through: conn is nil, injected stays false, legacy path below will run
					} else if conn != nil {
						// Merge all connection Data fields directly into config.
						for k, v := range conn.Data {
							config[k] = v
						}
						config["credential"] = conn.Data
						injected = true
					}
				}

				// Fall back to legacy workflow_credentials table.
				if !injected {
					cred, err := store.GetCredential(ctx, credID)
					if err != nil {
						return fmt.Errorf("node %s (%s): fetching credential %s: %w", node.ID, node.Name, credID, err)
					}
					if cred != nil {
						// Merge credential data into config under the "credential" key.
						config["credential"] = cred.Data
					}
				}
			}
		}

		// Build expression context from the first input item (if any).
		var currentItemJSON map[string]interface{}
		if len(inputItems) > 0 {
			currentItemJSON = inputItems[0].JSON
		}
		exprCtx := buildExpressionContext(currentItemJSON, nodeOutputs, wf.ID, exec.ID)

		// Resolve config templates.
		resolvedConfig, err := expr.ResolveConfig(config, exprCtx)
		if err != nil {
			return fmt.Errorf("node %s (%s): resolve config: %w", node.ID, node.Name, err)
		}

		// Extract retry policy and on_error behaviour from config.
		retryPolicy := extractRetryPolicy(resolvedConfig)
		onError := extractOnError(resolvedConfig)

		// Trigger nodes are pass-through — they emit their trigger items
		// without needing a registered executor.
		if strings.HasPrefix(node.Type, "trigger.") {
			outputs := []NodeOutput{{Handle: "main", Items: inputItems}}
			// Route outputs to successors.
			for _, succ := range dag.Successors(node.ID) {
				pendingInputs[succ.ID] = append(pendingInputs[succ.ID], inputItems...)
			}
			// Store outputs for expression access.
			nodeOutputs[node.Name] = inputItems

			// Record execution node as SUCCESS.
			now2 := time.Now().UTC()
			execNode := &WorkflowExecutionNode{
				ExecutionID: exec.ID,
				NodeID:      node.ID,
				NodeName:    node.Name,
				Status:      "SUCCESS",
				InputItems:  inputItems,
				StartedAt:   &now2,
				FinishedAt:  &now2,
			}
			_ = store.CreateExecutionNode(ctx, execNode)
			_ = outputs // suppress unused warning
			completedNodes[node.ID] = true
			decrementMergeWaiting(node.ID, dag, mergeWaiting, completedNodes)
			logger.Debug().
				Str("node_id", node.ID).
				Str("node_type", node.Type).
				Msg("trigger node pass-through")
			continue
		}

		// Get executor from registry.
		factory, ok := registry.Get(node.Type)
		if !ok {
			return fmt.Errorf("%w: %s", ErrNodeTypeUnknown, node.Type)
		}
		executor := factory()

		// Create execution-node record in RUNNING state.
		now := time.Now().UTC()
		execNode := &WorkflowExecutionNode{
			ExecutionID: exec.ID,
			NodeID:      node.ID,
			NodeName:    node.Name,
			Status:      "RUNNING",
			InputItems:  inputItems,
			StartedAt:   &now,
		}
		dbCtx1, dbCancel1 := dbCtx()
		defer dbCancel1()
		if err := store.CreateExecutionNode(dbCtx1, execNode); err != nil {
			logger.Error().Err(err).
				Str("node_id", node.ID).
				Str("node_name", node.Name).
				Msg("failed to create execution node record")
			// Non-fatal for the execution itself — continue.
		}

		// Build NodeInput.
		nodeInput := NodeInput{
			Items:       inputItems,
			NodeOutputs: nodeOutputs,
			WorkflowID:  wf.ID,
			ExecutionID: exec.ID,
			NodeID:      node.ID,
			NodeName:    node.Name,
		}

		// Execute with retry.
		outputs, execErr := executeWithRetry(ctx, executor, nodeInput, resolvedConfig, retryPolicy)

		if execErr != nil {
			logger.Error().Err(execErr).
				Str("node_id", node.ID).
				Str("node_name", node.Name).
				Str("on_error", onError).
				Msg("node execution failed")

			// Persist failure.
			dbCtx2, dbCancel2 := dbCtx()
			defer dbCancel2()
			if storeErr := store.SetExecutionNodeFinished(dbCtx2, execNode.ID, "FAILED", nil, execErr.Error()); storeErr != nil {
				logger.Error().Err(storeErr).
					Str("node_id", node.ID).
					Msg("failed to persist node failure")
			}

			switch onError {
			case "continue":
				// Pass through the input items so downstream nodes still receive data.
				// This preserves pipeline data even when a node fails (e.g. rate-limited AI).
				successors := dag.SuccessorsOnHandle(node.ID, "main")
				for _, succ := range successors {
					pendingInputs[succ.ID] = append(pendingInputs[succ.ID], inputItems...)
				}
				nodeOutputs[node.Name] = inputItems
				completedNodes[node.ID] = true
				decrementMergeWaiting(node.ID, dag, mergeWaiting, completedNodes)
				continue

			case "error_branch":
				// Route an error item to successors on the "error" handle.
				errorItems := []Item{
					NewItem(map[string]interface{}{
						"error":   execErr.Error(),
						"node_id": node.ID,
						"node":    node.Name,
					}),
				}
				errorSuccessors := dag.SuccessorsOnHandle(node.ID, "error")
				for _, succ := range errorSuccessors {
					pendingInputs[succ.ID] = append(pendingInputs[succ.ID], errorItems...)
				}
				completedNodes[node.ID] = true
				decrementMergeWaiting(node.ID, dag, mergeWaiting, completedNodes)
				continue

			default: // "stop" or anything else
				return fmt.Errorf("node %s (%s): %w", node.ID, node.Name, execErr)
			}
		}

		// Collect all items emitted on the "main" handle for nodeOutputs.
		var mainItems []Item
		for _, out := range outputs {
			if out.Handle == "main" || out.Handle == "" {
				mainItems = append(mainItems, out.Items...)
			}
		}
		nodeOutputs[node.Name] = mainItems

		// Persist success.
		dbCtx3, dbCancel3 := dbCtx()
		defer dbCancel3()
		if storeErr := store.SetExecutionNodeFinished(dbCtx3, execNode.ID, "SUCCESS", mainItems, ""); storeErr != nil {
			logger.Error().Err(storeErr).
				Str("node_id", node.ID).
				Msg("failed to persist node success")
		}

		// Route outputs to downstream nodes by handle.
		for _, out := range outputs {
			handle := out.Handle
			if handle == "" {
				handle = "main"
			}
			successors := dag.SuccessorsOnHandle(node.ID, handle)
			for _, succ := range successors {
				pendingInputs[succ.ID] = append(pendingInputs[succ.ID], out.Items...)
			}
		}

		// Mark complete and update merge counters for downstream merge nodes.
		completedNodes[node.ID] = true
		decrementMergeWaiting(node.ID, dag, mergeWaiting, completedNodes)

		logger.Debug().
			Str("node_id", node.ID).
			Str("node_name", node.Name).
			Int("output_items", len(mainItems)).
			Msg("node completed successfully")
	}

	return nil
}

// decrementMergeWaiting decrements the mergeWaiting counter for each direct
// successor of completedNodeID that is a merge node (InDegree > 1).
// On the first predecessor completion of a merge node the counter is
// initialised to InDegree - 1 (because the just-completed predecessor
// already "arrived").  On subsequent completions it is decremented by 1.
func decrementMergeWaiting(completedNodeID string, dag *DAG, mergeWaiting map[string]int, completedNodes map[string]bool) {
	for _, succ := range dag.Successors(completedNodeID) {
		if dag.InDegree(succ.ID) <= 1 {
			continue
		}
		if _, initialised := mergeWaiting[succ.ID]; !initialised {
			// First predecessor completing: initialise to InDegree - 1.
			mergeWaiting[succ.ID] = dag.InDegree(succ.ID) - 1
		} else {
			mergeWaiting[succ.ID]--
		}
	}
}

// dbCtx returns a short-lived context for DB persistence operations.
// It is intentionally independent of the execution context so that
// persistence succeeds even when the execution has been cancelled.
func dbCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

// executeWithRetry executes a node, retrying on failure according to the
// supplied RetryPolicy.  Panics from the executor are caught and returned
// as errors so that DB persistence in the caller is not bypassed.
func executeWithRetry(
	ctx context.Context,
	executor NodeExecutor,
	input NodeInput,
	config map[string]interface{},
	policy RetryPolicy,
) (outputs []NodeOutput, err error) {
	maxRetries := policy.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	if maxRetries > 10 {
		maxRetries = 10
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := computeDelay(attempt, policy)
			select {
			case <-ctx.Done():
				return nil, ErrExecutionCancelled
			case <-time.After(delay):
			}
		}

		outputs, err = func() (out []NodeOutput, execErr error) {
			defer func() {
				if r := recover(); r != nil {
					execErr = fmt.Errorf("node executor panicked: %v\n%s", r, debug.Stack())
				}
			}()
			return executor.Execute(ctx, input, config)
		}()
		if err == nil {
			return outputs, nil
		}
		lastErr = err

		// Do not retry if the context is done.
		select {
		case <-ctx.Done():
			return nil, ErrExecutionCancelled
		default:
		}
	}
	return nil, lastErr
}

// computeDelay returns the sleep duration before the given retry attempt
// (attempt is 1-based: first retry is attempt=1).
func computeDelay(attempt int, p RetryPolicy) time.Duration {
	const maxDelaySeconds = 3600.0

	var seconds float64
	switch strings.ToLower(p.BackoffType) {
	case "exponential":
		// p.InitialDelay * 2^(attempt-1)
		seconds = p.InitialDelay * math.Pow(2, float64(attempt-1))
	default: // "fixed" or anything else
		seconds = p.InitialDelay
	}

	if seconds > maxDelaySeconds {
		seconds = maxDelaySeconds
	}
	if seconds < 0 {
		seconds = 0
	}

	return time.Duration(seconds * float64(time.Second))
}

// buildExpressionContext constructs an ExpressionContext from the current
// item's JSON map and the accumulated nodeOutputs map.
func buildExpressionContext(
	currentJSON map[string]interface{},
	nodeOutputs map[string][]Item,
	workflowID string,
	executionID string,
) ExpressionContext {
	if currentJSON == nil {
		currentJSON = make(map[string]interface{})
	}
	// Shallow copy nodeOutputs so the context is not mutated by later nodes.
	nodeCopy := make(map[string][]Item, len(nodeOutputs))
	for k, v := range nodeOutputs {
		nodeCopy[k] = v
	}
	return ExpressionContext{
		JSON:        currentJSON,
		Node:        nodeCopy,
		WorkflowID:  workflowID,
		ExecutionID: executionID,
		Env:         nil, // populated from os.Environ inside expression engine
	}
}

// buildTriggerItems wraps trigger data as a single Item.
// If triggerData is nil or empty, a single empty Item is returned so
// trigger nodes always receive at least one item to process.
func buildTriggerItems(triggerData map[string]interface{}) []Item {
	if triggerData == nil {
		triggerData = make(map[string]interface{})
	}
	return []Item{NewItem(triggerData)}
}

// extractRetryPolicy reads RetryPolicy fields from the resolved config map.
// Defaults: MaxRetries=0, BackoffType="fixed", InitialDelay=1.
func extractRetryPolicy(config map[string]interface{}) RetryPolicy {
	p := RetryPolicy{
		MaxRetries:   0,
		BackoffType:  "fixed",
		InitialDelay: 1,
	}

	rpRaw, ok := config["retry_policy"]
	if !ok {
		return p
	}

	// Try to round-trip through JSON to get a clean RetryPolicy struct.
	b, err := json.Marshal(rpRaw)
	if err != nil {
		return p
	}
	var parsed RetryPolicy
	if err := json.Unmarshal(b, &parsed); err != nil {
		return p
	}
	if parsed.MaxRetries > 0 {
		p.MaxRetries = parsed.MaxRetries
	}
	if parsed.BackoffType != "" {
		p.BackoffType = parsed.BackoffType
	}
	if parsed.InitialDelay > 0 {
		p.InitialDelay = parsed.InitialDelay
	}
	return p
}

// extractOnError reads the on_error string from the resolved config map.
// Defaults to "stop".
func extractOnError(config map[string]interface{}) string {
	if v, ok := config["on_error"]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return "stop"
}
