package workflow

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog"
)

// SchedulerInterface abstracts the scheduler for testability.
type SchedulerInterface interface {
	AddWorkflowJob(spec string, fn func()) (cron.EntryID, error)
	RemoveJob(id cron.EntryID)
}

// triggerEntry holds state for an active trigger registration.
// For schedule triggers it holds the cron EntryID; for webhook triggers the path.
type triggerEntry struct {
	kind      string // "schedule" or "webhook"
	cronID    cron.EntryID
	webhookPath string
}

// TriggerManager activates and deactivates triggers for workflows.
// It coordinates three trigger types: manual (no-op), schedule, and webhook.
type TriggerManager struct {
	store         WorkflowStore
	webhookServer *WebhookServer
	scheduler     SchedulerInterface
	triggerFn     func(workflowID string, nodeID string, items []Item)
	mu            sync.Mutex
	active        map[string]*triggerEntry // key: workflowID+"_"+nodeID
	logger        zerolog.Logger
}

// NewTriggerManager creates a TriggerManager.
// triggerFn is called when any trigger fires with the workflowID, nodeID, and initial items.
func NewTriggerManager(
	store WorkflowStore,
	webhookServer *WebhookServer,
	scheduler SchedulerInterface,
	triggerFn func(workflowID string, nodeID string, items []Item),
	logger zerolog.Logger,
) *TriggerManager {
	return &TriggerManager{
		store:         store,
		webhookServer: webhookServer,
		scheduler:     scheduler,
		triggerFn:     triggerFn,
		active:        make(map[string]*triggerEntry),
		logger:        logger,
	}
}

// activeKey returns the map key for a workflow+node pair.
func activeKey(workflowID, nodeID string) string {
	return workflowID + "_" + nodeID
}

// ActivateWorkflow registers all trigger nodes in the given workflow.
// It is idempotent: nodes already registered are skipped.
func (tm *TriggerManager) ActivateWorkflow(ctx context.Context, w *Workflow) error {
	var errs []error

	for i := range w.Nodes {
		node := &w.Nodes[i]

		if node.Disabled {
			continue
		}

		switch node.Type {
		case "trigger.manual":
			// Manual triggers are fired by direct API call — nothing to register.
			tm.logger.Debug().
				Str("workflow_id", w.ID).
				Str("node_id", node.ID).
				Msg("manual trigger: no background registration needed")

		case "trigger.schedule":
			if err := tm.activateSchedule(w.ID, node); err != nil {
				errs = append(errs, fmt.Errorf("node %s (%s): %w", node.ID, node.Type, err))
			}

		case "trigger.webhook":
			if err := tm.activateWebhook(w.ID, node); err != nil {
				errs = append(errs, fmt.Errorf("node %s (%s): %w", node.ID, node.Type, err))
			}
		}
	}

	if len(errs) > 0 {
		combined := errs[0]
		for _, e := range errs[1:] {
			combined = fmt.Errorf("%w; %v", combined, e)
		}
		return combined
	}
	return nil
}

// activateSchedule registers a cron job for a trigger.schedule node.
func (tm *TriggerManager) activateSchedule(workflowID string, node *WorkflowNode) error {
	key := activeKey(workflowID, node.ID)

	// Parse config outside the lock — no shared state involved.
	if node.Config == nil {
		if err := node.ParseConfig(); err != nil {
			return fmt.Errorf("parse config: %w", err)
		}
	}

	spec, ok := node.Config["cron"].(string)
	if !ok || spec == "" {
		return fmt.Errorf("trigger.schedule: missing or invalid \"cron\" field in config")
	}

	// Capture loop variables for the closure.
	wfID := workflowID
	nID := node.ID

	// Hold the mutex through the entire check-then-act to prevent TOCTOU races.
	tm.mu.Lock()
	if _, exists := tm.active[key]; exists {
		tm.mu.Unlock()
		tm.logger.Debug().
			Str("workflow_id", workflowID).
			Str("node_id", node.ID).
			Msg("schedule trigger already active, skipping")
		return nil
	}

	entryID, err := tm.scheduler.AddWorkflowJob(spec, func() {
		items := []Item{
			{
				JSON: map[string]interface{}{
					"trigger_type": "schedule",
					"timestamp":    time.Now().UTC().Format(time.RFC3339),
				},
			},
		}
		tm.triggerFn(wfID, nID, items)
	})
	if err != nil {
		tm.mu.Unlock()
		return fmt.Errorf("add cron job with spec %q: %w", spec, err)
	}

	tm.active[key] = &triggerEntry{
		kind:   "schedule",
		cronID: entryID,
	}
	tm.mu.Unlock()

	tm.logger.Info().
		Str("workflow_id", workflowID).
		Str("node_id", node.ID).
		Str("cron", spec).
		Msg("schedule trigger activated")

	return nil
}

// activateWebhook registers a webhook route for a trigger.webhook node.
func (tm *TriggerManager) activateWebhook(workflowID string, node *WorkflowNode) error {
	key := activeKey(workflowID, node.ID)

	// Parse config outside the lock — no shared state involved.
	if node.Config == nil {
		if err := node.ParseConfig(); err != nil {
			return fmt.Errorf("parse config: %w", err)
		}
	}

	path, ok := node.Config["path"].(string)
	if !ok || path == "" {
		return fmt.Errorf("trigger.webhook: missing or invalid \"path\" field in config")
	}

	method, _ := node.Config["method"].(string)
	if method == "" {
		method = "POST"
	}

	hmacSecret, _ := node.Config["hmac_secret"].(string)

	// Capture loop variables for the closure.
	wfID := workflowID
	nID := node.ID

	reg := &WebhookRegistration{
		WorkflowID: workflowID,
		NodeID:     node.ID,
		Path:       path,
		Method:     method,
		HMACSecret: hmacSecret,
		TriggerFn: func(items []Item) {
			tm.triggerFn(wfID, nID, items)
		},
	}

	// Hold the mutex through the entire check-then-act to prevent TOCTOU races.
	tm.mu.Lock()
	if _, exists := tm.active[key]; exists {
		tm.mu.Unlock()
		tm.logger.Debug().
			Str("workflow_id", workflowID).
			Str("node_id", node.ID).
			Msg("webhook trigger already active, skipping")
		return nil
	}

	if err := tm.webhookServer.Register(reg); err != nil {
		tm.mu.Unlock()
		return fmt.Errorf("register webhook path %q: %w", path, err)
	}

	tm.active[key] = &triggerEntry{
		kind:        "webhook",
		webhookPath: path,
	}
	tm.mu.Unlock()

	tm.logger.Info().
		Str("workflow_id", workflowID).
		Str("node_id", node.ID).
		Str("path", path).
		Str("method", method).
		Msg("webhook trigger activated")

	return nil
}

// DeactivateWorkflow removes all triggers for the given workflow.
func (tm *TriggerManager) DeactivateWorkflow(workflowID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for key, entry := range tm.active {
		// Keys are workflowID+"_"+nodeID; check prefix.
		prefix := workflowID + "_"
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			tm.deactivateEntry(key, entry)
		}
	}
}

// DeactivateAll removes all triggers (called on shutdown).
func (tm *TriggerManager) DeactivateAll() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for key, entry := range tm.active {
		tm.deactivateEntry(key, entry)
	}
}

// deactivateEntry tears down a single trigger entry and removes it from the map.
// Must be called with tm.mu held.
func (tm *TriggerManager) deactivateEntry(key string, entry *triggerEntry) {
	switch entry.kind {
	case "schedule":
		tm.scheduler.RemoveJob(entry.cronID)
		tm.logger.Info().
			Str("key", key).
			Msg("schedule trigger deactivated")

	case "webhook":
		tm.webhookServer.Deregister(entry.webhookPath)
		tm.logger.Info().
			Str("key", key).
			Str("path", entry.webhookPath).
			Msg("webhook trigger deactivated")
	}
	delete(tm.active, key)
}
