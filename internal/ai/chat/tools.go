package chat

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/monoes/monoes-agent/internal/ai"
)

// NodeTypeInfo describes a node type available in the system.
type NodeTypeInfo struct {
	Type        string `json:"type"`
	Label       string `json:"label"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

// CanvasTools provides AI tool functions for manipulating workflow canvases.
type CanvasTools struct {
	db        *sql.DB
	nodeTypes []NodeTypeInfo
}

// NewCanvasTools creates a CanvasTools backed by the given database.
func NewCanvasTools(db *sql.DB) *CanvasTools {
	return &CanvasTools{db: db}
}

// SetNodeTypes provides the list of available node types for list_available_nodes.
func (ct *CanvasTools) SetNodeTypes(types []NodeTypeInfo) {
	ct.nodeTypes = types
}

// ToolDefs returns the tool definitions the AI model can call.
func (ct *CanvasTools) ToolDefs() []ai.ToolDef {
	return []ai.ToolDef{
		{
			Type: "function",
			Function: ai.ToolFunction{
				Name:        "get_workflow_state",
				Description: "Get the current workflow nodes and connections",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"workflow_id": map[string]interface{}{"type": "string", "description": "The workflow ID"},
					},
					"required": []string{"workflow_id"},
				},
			},
		},
		{
			Type: "function",
			Function: ai.ToolFunction{
				Name:        "create_workflow",
				Description: "Create a new workflow. Returns the new workflow_id. You MUST call this first before creating nodes when the current workflow_id is 'general' or 'draft'.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name":        map[string]interface{}{"type": "string", "description": "Name for the new workflow"},
						"description": map[string]interface{}{"type": "string", "description": "Optional description of what the workflow does"},
					},
					"required": []string{"name"},
				},
			},
		},
		{
			Type: "function",
			Function: ai.ToolFunction{
				Name:        "create_nodes",
				Description: "Create one or more new nodes in a workflow",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"workflow_id": map[string]interface{}{"type": "string", "description": "The workflow ID"},
						"nodes": map[string]interface{}{
							"type":        "array",
							"description": "List of nodes to create",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"node_type":  map[string]interface{}{"type": "string", "description": "The node type identifier"},
									"name":       map[string]interface{}{"type": "string", "description": "Display name for the node"},
									"config":     map[string]interface{}{"type": "object", "description": "Node configuration JSON"},
									"position_x": map[string]interface{}{"type": "number", "description": "X position on canvas"},
									"position_y": map[string]interface{}{"type": "number", "description": "Y position on canvas"},
								},
								"required": []string{"node_type", "name"},
							},
						},
					},
					"required": []string{"workflow_id", "nodes"},
				},
			},
		},
		{
			Type: "function",
			Function: ai.ToolFunction{
				Name:        "update_node_config",
				Description: "Update a node's configuration by merging new values into the existing config",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"workflow_id": map[string]interface{}{"type": "string", "description": "The workflow ID"},
						"node_id":     map[string]interface{}{"type": "string", "description": "The node ID to update"},
						"config":      map[string]interface{}{"type": "object", "description": "Config keys to merge into the existing config"},
					},
					"required": []string{"workflow_id", "node_id", "config"},
				},
			},
		},
		{
			Type: "function",
			Function: ai.ToolFunction{
				Name:        "delete_nodes",
				Description: "Delete one or more nodes and their connections from a workflow",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"workflow_id": map[string]interface{}{"type": "string", "description": "The workflow ID"},
						"node_ids": map[string]interface{}{
							"type":        "array",
							"description": "List of node IDs to delete",
							"items":       map[string]interface{}{"type": "string"},
						},
					},
					"required": []string{"workflow_id", "node_ids"},
				},
			},
		},
		{
			Type: "function",
			Function: ai.ToolFunction{
				Name:        "connect_nodes",
				Description: "Create a connection between two nodes",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"workflow_id":    map[string]interface{}{"type": "string", "description": "The workflow ID"},
						"source_node_id": map[string]interface{}{"type": "string", "description": "Source node ID"},
						"source_handle":  map[string]interface{}{"type": "string", "description": "Source output handle name"},
						"target_node_id": map[string]interface{}{"type": "string", "description": "Target node ID"},
						"target_handle":  map[string]interface{}{"type": "string", "description": "Target input handle name"},
					},
					"required": []string{"workflow_id", "source_node_id", "source_handle", "target_node_id", "target_handle"},
				},
			},
		},
		{
			Type: "function",
			Function: ai.ToolFunction{
				Name:        "disconnect_nodes",
				Description: "Remove a connection between two nodes",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"workflow_id":    map[string]interface{}{"type": "string", "description": "The workflow ID"},
						"source_node_id": map[string]interface{}{"type": "string", "description": "Source node ID"},
						"target_node_id": map[string]interface{}{"type": "string", "description": "Target node ID"},
					},
					"required": []string{"workflow_id", "source_node_id", "target_node_id"},
				},
			},
		},
		{
			Type: "function",
			Function: ai.ToolFunction{
				Name:        "list_available_nodes",
				Description: "List the available node types that can be added to a workflow",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"category": map[string]interface{}{"type": "string", "description": "Optional category filter (e.g. 'core', 'trigger', 'action')"},
					},
				},
			},
		},
	}
}

// Execute dispatches a tool call by name. args is the raw JSON arguments string.
// Returns a JSON result string.
func (ct *CanvasTools) Execute(name string, args string) (string, error) {
	switch name {
	case "get_workflow_state":
		return ct.getWorkflowState(args)
	case "create_workflow":
		return ct.createWorkflow(args)
	case "create_nodes":
		return ct.createNodes(args)
	case "update_node_config":
		return ct.updateNodeConfig(args)
	case "delete_nodes":
		return ct.deleteNodes(args)
	case "connect_nodes":
		return ct.connectNodes(args)
	case "disconnect_nodes":
		return ct.disconnectNodes(args)
	case "list_available_nodes":
		return ct.listAvailableNodes(args)
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

// --- tool implementations ---

type getWorkflowStateArgs struct {
	WorkflowID string `json:"workflow_id"`
}

type nodeRow struct {
	ID        string  `json:"id"`
	NodeType  string  `json:"node_type"`
	Name      string  `json:"name"`
	Config    any     `json:"config"`
	PositionX float64 `json:"position_x"`
	PositionY float64 `json:"position_y"`
	Disabled  bool    `json:"disabled"`
}

type connectionRow struct {
	ID           string `json:"id"`
	SourceNodeID string `json:"source_node_id"`
	SourceHandle string `json:"source_handle"`
	TargetNodeID string `json:"target_node_id"`
	TargetHandle string `json:"target_handle"`
	Position     int    `json:"position"`
}

func (ct *CanvasTools) getWorkflowState(args string) (string, error) {
	var a getWorkflowStateArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}

	// Query nodes
	rows, err := ct.db.Query(
		`SELECT id, node_type, name, config, position_x, position_y, disabled
		 FROM workflow_nodes WHERE workflow_id = ?`, a.WorkflowID)
	if err != nil {
		return "", fmt.Errorf("query nodes: %w", err)
	}
	defer rows.Close()

	nodes := make([]nodeRow, 0)
	for rows.Next() {
		var n nodeRow
		var configStr string
		if err := rows.Scan(&n.ID, &n.NodeType, &n.Name, &configStr, &n.PositionX, &n.PositionY, &n.Disabled); err != nil {
			return "", fmt.Errorf("scan node: %w", err)
		}
		if configStr != "" {
			var cfg interface{}
			if err := json.Unmarshal([]byte(configStr), &cfg); err == nil {
				n.Config = cfg
			} else {
				n.Config = configStr
			}
		} else {
			n.Config = map[string]interface{}{}
		}
		nodes = append(nodes, n)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate nodes: %w", err)
	}

	// Query connections
	connRows, err := ct.db.Query(
		`SELECT id, source_node_id, source_handle, target_node_id, target_handle, position
		 FROM workflow_connections WHERE workflow_id = ?`, a.WorkflowID)
	if err != nil {
		return "", fmt.Errorf("query connections: %w", err)
	}
	defer connRows.Close()

	connections := make([]connectionRow, 0)
	for connRows.Next() {
		var c connectionRow
		if err := connRows.Scan(&c.ID, &c.SourceNodeID, &c.SourceHandle, &c.TargetNodeID, &c.TargetHandle, &c.Position); err != nil {
			return "", fmt.Errorf("scan connection: %w", err)
		}
		connections = append(connections, c)
	}
	if err := connRows.Err(); err != nil {
		return "", fmt.Errorf("iterate connections: %w", err)
	}

	result := map[string]interface{}{
		"workflow_id": a.WorkflowID,
		"nodes":       nodes,
		"connections": connections,
	}
	return marshalJSON(result)
}

type createWorkflowArgs struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (ct *CanvasTools) createWorkflow(args string) (string, error) {
	var a createWorkflowArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}

	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := ct.db.Exec(
		`INSERT INTO workflows (id, name, description, is_active, version, created_at, updated_at)
		 VALUES (?, ?, ?, 0, 1, ?, ?)`,
		id, a.Name, a.Description, now, now); err != nil {
		return "", fmt.Errorf("insert workflow: %w", err)
	}

	result := map[string]interface{}{
		"workflow_id": id,
		"name":       a.Name,
	}
	return marshalJSON(result)
}

type createNodesArgs struct {
	WorkflowID string           `json:"workflow_id"`
	Nodes      []createNodeSpec `json:"nodes"`
}

type createNodeSpec struct {
	NodeType  string                 `json:"node_type"`
	Name      string                 `json:"name"`
	Config    map[string]interface{} `json:"config"`
	PositionX float64                `json:"position_x"`
	PositionY float64                `json:"position_y"`
}

func (ct *CanvasTools) createNodes(args string) (string, error) {
	var a createNodesArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}

	now := time.Now().UTC()
	createdIDs := make([]string, 0, len(a.Nodes))

	tx, err := ct.db.Begin()
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		`INSERT INTO workflow_nodes (id, workflow_id, node_type, name, config, position_x, position_y, disabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`)
	if err != nil {
		return "", fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, n := range a.Nodes {
		id := uuid.New().String()
		configJSON := "{}"
		if n.Config != nil {
			b, err := json.Marshal(n.Config)
			if err != nil {
				return "", fmt.Errorf("marshal config: %w", err)
			}
			configJSON = string(b)
		}
		if _, err := stmt.Exec(id, a.WorkflowID, n.NodeType, n.Name, configJSON, n.PositionX, n.PositionY, now, now); err != nil {
			return "", fmt.Errorf("insert node: %w", err)
		}
		createdIDs = append(createdIDs, id)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}

	result := map[string]interface{}{
		"created_node_ids": createdIDs,
	}
	return marshalJSON(result)
}

type updateNodeConfigArgs struct {
	WorkflowID string                 `json:"workflow_id"`
	NodeID     string                 `json:"node_id"`
	Config     map[string]interface{} `json:"config"`
}

func (ct *CanvasTools) updateNodeConfig(args string) (string, error) {
	var a updateNodeConfigArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}

	// Read existing config
	var existingConfig string
	err := ct.db.QueryRow(
		`SELECT config FROM workflow_nodes WHERE id = ? AND workflow_id = ?`,
		a.NodeID, a.WorkflowID).Scan(&existingConfig)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("node %s not found in workflow %s", a.NodeID, a.WorkflowID)
	}
	if err != nil {
		return "", fmt.Errorf("query node: %w", err)
	}

	// Parse existing config
	existing := make(map[string]interface{})
	if existingConfig != "" {
		if err := json.Unmarshal([]byte(existingConfig), &existing); err != nil {
			return "", fmt.Errorf("parse existing config: %w", err)
		}
	}

	// Merge new keys into existing
	for k, v := range a.Config {
		existing[k] = v
	}

	merged, err := json.Marshal(existing)
	if err != nil {
		return "", fmt.Errorf("marshal merged config: %w", err)
	}

	now := time.Now().UTC()
	if _, err := ct.db.Exec(
		`UPDATE workflow_nodes SET config = ?, updated_at = ? WHERE id = ? AND workflow_id = ?`,
		string(merged), now, a.NodeID, a.WorkflowID); err != nil {
		return "", fmt.Errorf("update node: %w", err)
	}

	result := map[string]interface{}{
		"node_id": a.NodeID,
		"config":  existing,
	}
	return marshalJSON(result)
}

type deleteNodesArgs struct {
	WorkflowID string   `json:"workflow_id"`
	NodeIDs    []string `json:"node_ids"`
}

func (ct *CanvasTools) deleteNodes(args string) (string, error) {
	var a deleteNodesArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}

	tx, err := ct.db.Begin()
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	for _, nodeID := range a.NodeIDs {
		// Delete connections involving this node
		if _, err := tx.Exec(
			`DELETE FROM workflow_connections WHERE workflow_id = ? AND (source_node_id = ? OR target_node_id = ?)`,
			a.WorkflowID, nodeID, nodeID); err != nil {
			return "", fmt.Errorf("delete connections for node %s: %w", nodeID, err)
		}
		// Delete the node
		if _, err := tx.Exec(
			`DELETE FROM workflow_nodes WHERE id = ? AND workflow_id = ?`,
			nodeID, a.WorkflowID); err != nil {
			return "", fmt.Errorf("delete node %s: %w", nodeID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}

	result := map[string]interface{}{
		"deleted_node_ids": a.NodeIDs,
	}
	return marshalJSON(result)
}

type connectNodesArgs struct {
	WorkflowID   string `json:"workflow_id"`
	SourceNodeID string `json:"source_node_id"`
	SourceHandle string `json:"source_handle"`
	TargetNodeID string `json:"target_node_id"`
	TargetHandle string `json:"target_handle"`
}

func (ct *CanvasTools) connectNodes(args string) (string, error) {
	var a connectNodesArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}

	id := uuid.New().String()
	if _, err := ct.db.Exec(
		`INSERT INTO workflow_connections (id, workflow_id, source_node_id, source_handle, target_node_id, target_handle, position)
		 VALUES (?, ?, ?, ?, ?, ?, 0)`,
		id, a.WorkflowID, a.SourceNodeID, a.SourceHandle, a.TargetNodeID, a.TargetHandle); err != nil {
		return "", fmt.Errorf("insert connection: %w", err)
	}

	result := map[string]interface{}{
		"connection_id": id,
	}
	return marshalJSON(result)
}

type disconnectNodesArgs struct {
	WorkflowID   string `json:"workflow_id"`
	SourceNodeID string `json:"source_node_id"`
	TargetNodeID string `json:"target_node_id"`
}

func (ct *CanvasTools) disconnectNodes(args string) (string, error) {
	var a disconnectNodesArgs
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}

	res, err := ct.db.Exec(
		`DELETE FROM workflow_connections WHERE workflow_id = ? AND source_node_id = ? AND target_node_id = ?`,
		a.WorkflowID, a.SourceNodeID, a.TargetNodeID)
	if err != nil {
		return "", fmt.Errorf("delete connection: %w", err)
	}

	deleted, _ := res.RowsAffected()
	result := map[string]interface{}{
		"deleted_count": deleted,
	}
	return marshalJSON(result)
}

type listAvailableNodesArgs struct {
	Category string `json:"category"`
}

func (ct *CanvasTools) listAvailableNodes(args string) (string, error) {
	var a listAvailableNodesArgs
	if args != "" {
		if err := json.Unmarshal([]byte(args), &a); err != nil {
			return "", fmt.Errorf("invalid args: %w", err)
		}
	}

	if a.Category == "" {
		return marshalJSON(map[string]interface{}{"node_types": ct.nodeTypes})
	}

	filtered := make([]NodeTypeInfo, 0)
	for _, nt := range ct.nodeTypes {
		if nt.Category == a.Category {
			filtered = append(filtered, nt)
		}
	}
	return marshalJSON(map[string]interface{}{"node_types": filtered})
}

func marshalJSON(v interface{}) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal result: %w", err)
	}
	return string(b), nil
}
