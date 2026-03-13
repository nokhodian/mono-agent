package workflow

import (
	"context"
	"encoding/json"
	"time"
)

// Item represents a single data record flowing between nodes.
// JSON field is arbitrary key-value data. Binary is optional base64 content.
type Item struct {
	JSON   map[string]interface{} `json:"json"`
	Binary map[string][]byte      `json:"binary,omitempty"`
}

// NewItem constructs an Item from an arbitrary data map.
func NewItem(data map[string]interface{}) Item {
	return Item{
		JSON: data,
	}
}

// NodeInput is passed to NodeExecutor.Execute.
type NodeInput struct {
	Items       []Item            // items from upstream node
	NodeOutputs map[string][]Item // all outputs indexed by node name (for $node["Name"])
	WorkflowID  string
	ExecutionID string
	NodeID      string
	NodeName    string
}

// NodeOutput is returned by NodeExecutor.Execute.
// Handle is the output port name: "main", "true", "false", "error", case names, etc.
type NodeOutput struct {
	Handle string
	Items  []Item
}

// NodeExecutor is the single interface every action type must implement.
type NodeExecutor interface {
	// Execute runs the node logic. config is the node's JSON config map (credentials already injected).
	// Returns one or more NodeOutputs (one per output handle used).
	Execute(ctx context.Context, input NodeInput, config map[string]interface{}) ([]NodeOutput, error)
	// Type returns the node type string, e.g. "core.if", "action.instagram.KEYWORD_SEARCH"
	Type() string
}

// TriggerProvider is implemented by nodes that can initiate workflow executions.
type TriggerProvider interface {
	NodeExecutor
	// Activate registers the trigger. When it fires, it calls triggerFn with initial items.
	Activate(ctx context.Context, workflowID string, nodeID string, config map[string]interface{}, triggerFn func(items []Item)) error
	// Deactivate stops the trigger.
	Deactivate(workflowID string, nodeID string) error
}

// Workflow is the top-level entity stored in the workflows table.
type Workflow struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	IsActive    bool      `json:"is_active" db:"is_active"`
	Version     int       `json:"version" db:"version"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
	// Populated by LoadWorkflow — not stored directly
	Nodes       []WorkflowNode       `json:"nodes,omitempty"`
	Connections []WorkflowConnection `json:"connections,omitempty"`
}

// WorkflowNode is one node in the workflow graph.
// The Type field is the node type string (e.g. "core.if", "trigger.schedule").
type WorkflowNode struct {
	ID         string                 `json:"id" db:"id"`
	WorkflowID string                 `json:"workflow_id" db:"workflow_id"`
	// Type is the node type identifier; stored as node_type in the database.
	Type      string                 `json:"node_type" db:"node_type"`
	Name       string                 `json:"name" db:"name"`
	Config     map[string]interface{} `json:"config" db:"-"`
	ConfigRaw  string                 `json:"-" db:"config"`
	PositionX  float64                `json:"position_x" db:"position_x"`
	PositionY  float64                `json:"position_y" db:"position_y"`
	Disabled   bool                   `json:"disabled" db:"disabled"`
	Schema     *NodeSchema            `json:"schema,omitempty" db:"-"`
	CreatedAt  time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at" db:"updated_at"`
}

// ParseConfig unmarshals ConfigRaw JSON into Config.
func (n *WorkflowNode) ParseConfig() error {
	if n.ConfigRaw == "" {
		n.Config = make(map[string]interface{})
		return nil
	}
	return json.Unmarshal([]byte(n.ConfigRaw), &n.Config)
}

// MarshalConfig marshals Config into ConfigRaw JSON.
func (n *WorkflowNode) MarshalConfig() error {
	b, err := json.Marshal(n.Config)
	if err != nil {
		return err
	}
	n.ConfigRaw = string(b)
	return nil
}

// WorkflowConnection is a directed edge in the workflow graph.
type WorkflowConnection struct {
	ID           string `json:"id" db:"id"`
	WorkflowID   string `json:"workflow_id" db:"workflow_id"`
	SourceNodeID string `json:"source_node_id" db:"source_node_id"`
	SourceHandle string `json:"source_handle" db:"source_handle"`
	TargetNodeID string `json:"target_node_id" db:"target_node_id"`
	TargetHandle string `json:"target_handle" db:"target_handle"`
	Position     int    `json:"position" db:"position"`
}

// WorkflowExecution is a single run of a workflow.
type WorkflowExecution struct {
	ID             string                 `json:"id" db:"id"`
	WorkflowID     string                 `json:"workflow_id" db:"workflow_id"`
	Status         string                 `json:"status" db:"status"` // QUEUED, RUNNING, SUCCESS, FAILED, CANCELLED
	TriggerType    string                 `json:"trigger_type" db:"trigger_type"`
	TriggerData    map[string]interface{} `json:"trigger_data" db:"-"`
	TriggerDataRaw string                 `json:"-" db:"trigger_data"`
	StartedAt      *time.Time             `json:"started_at" db:"started_at"`
	FinishedAt     *time.Time             `json:"finished_at" db:"finished_at"`
	ErrorMessage   string                 `json:"error_message" db:"error_message"`
	CreatedAt      time.Time              `json:"created_at" db:"created_at"`
	// Populated on load
	Nodes []WorkflowExecutionNode `json:"nodes,omitempty"`
}

// ParseTriggerData unmarshals TriggerDataRaw JSON into TriggerData.
func (e *WorkflowExecution) ParseTriggerData() error {
	if e.TriggerDataRaw == "" {
		e.TriggerData = make(map[string]interface{})
		return nil
	}
	return json.Unmarshal([]byte(e.TriggerDataRaw), &e.TriggerData)
}

// MarshalTriggerData marshals TriggerData into TriggerDataRaw JSON.
func (e *WorkflowExecution) MarshalTriggerData() error {
	b, err := json.Marshal(e.TriggerData)
	if err != nil {
		return err
	}
	e.TriggerDataRaw = string(b)
	return nil
}

// WorkflowExecutionNode is the per-node run record within an execution.
type WorkflowExecutionNode struct {
	ID           string     `json:"id" db:"id"`
	ExecutionID  string     `json:"execution_id" db:"execution_id"`
	NodeID       string     `json:"node_id" db:"node_id"`
	NodeName     string     `json:"node_name" db:"node_name"`
	Status       string     `json:"status" db:"status"` // PENDING, RUNNING, SUCCESS, FAILED, SKIPPED
	InputItems   []Item     `json:"input_items" db:"-"`
	InputRaw     string     `json:"-" db:"input_items"`
	OutputItems  []Item     `json:"output_items" db:"-"`
	OutputRaw    string     `json:"-" db:"output_items"`
	ErrorMessage string     `json:"error_message" db:"error_message"`
	StartedAt    *time.Time `json:"started_at" db:"started_at"`
	FinishedAt   *time.Time `json:"finished_at" db:"finished_at"`
	RetryCount   int        `json:"retry_count" db:"retry_count"`
}

// ParseItems unmarshals InputRaw and OutputRaw JSON into InputItems and OutputItems.
func (en *WorkflowExecutionNode) ParseItems() error {
	if en.InputRaw != "" {
		if err := json.Unmarshal([]byte(en.InputRaw), &en.InputItems); err != nil {
			return err
		}
	}
	if en.OutputRaw != "" {
		if err := json.Unmarshal([]byte(en.OutputRaw), &en.OutputItems); err != nil {
			return err
		}
	}
	return nil
}

// MarshalItems marshals InputItems and OutputItems into InputRaw and OutputRaw JSON.
func (en *WorkflowExecutionNode) MarshalItems() error {
	ib, err := json.Marshal(en.InputItems)
	if err != nil {
		return err
	}
	en.InputRaw = string(ib)

	ob, err := json.Marshal(en.OutputItems)
	if err != nil {
		return err
	}
	en.OutputRaw = string(ob)
	return nil
}

// Credential is a stored secret set for use by nodes.
type Credential struct {
	ID        string                 `json:"id" db:"id"`
	Name      string                 `json:"name" db:"name"`
	Type      string                 `json:"type" db:"type"`
	Data      map[string]interface{} `json:"data" db:"-"`
	DataRaw   string                 `json:"-" db:"data"`
	CreatedAt time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt time.Time              `json:"updated_at" db:"updated_at"`
}

// RetryPolicy defines how a node is retried on failure.
type RetryPolicy struct {
	MaxRetries   int     `json:"max_retries"`   // 0 = no retry, max 10
	BackoffType  string  `json:"backoff_type"`  // "fixed" or "exponential"
	InitialDelay float64 `json:"initial_delay"` // seconds, 1–3600
}

// NodeConfig is the parsed config for a workflow node including retry and error handling.
type NodeConfig struct {
	RetryPolicy    RetryPolicy            `json:"retry_policy"`
	OnError        string                 `json:"on_error"`         // "stop", "continue", "error_branch"
	ContinueOnFail bool                   `json:"continue_on_fail"`
	CredentialID   string                 `json:"credential_id"`
	Params         map[string]interface{} `json:"params"`
}

// ExpressionContext is passed to the expression engine for a node execution.
type ExpressionContext struct {
	JSON        map[string]interface{} // current item's JSON
	Node        map[string][]Item      // all node outputs indexed by name
	WorkflowID  string
	ExecutionID string
	Env         map[string]string // env var access
}
