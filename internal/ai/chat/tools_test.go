package chat

import (
	"database/sql"
	"encoding/json"
	"testing"

	_ "modernc.org/sqlite"
)

// setupTestDB creates an in-memory SQLite database with the workflow tables.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	schema := `
	CREATE TABLE workflow_nodes (
		id TEXT PRIMARY KEY,
		workflow_id TEXT NOT NULL,
		node_type TEXT NOT NULL,
		name TEXT NOT NULL,
		config TEXT NOT NULL DEFAULT '{}',
		position_x REAL NOT NULL DEFAULT 0,
		position_y REAL NOT NULL DEFAULT 0,
		disabled INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE workflow_connections (
		id TEXT PRIMARY KEY,
		workflow_id TEXT NOT NULL,
		source_node_id TEXT NOT NULL,
		source_handle TEXT NOT NULL,
		target_node_id TEXT NOT NULL,
		target_handle TEXT NOT NULL,
		position INTEGER NOT NULL DEFAULT 0
	);`

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create tables: %v", err)
	}

	t.Cleanup(func() { db.Close() })
	return db
}

func TestGetWorkflowState(t *testing.T) {
	db := setupTestDB(t)
	ct := NewCanvasTools(db)

	wfID := "wf-1"

	// Insert test nodes
	_, err := db.Exec(
		`INSERT INTO workflow_nodes (id, workflow_id, node_type, name, config, position_x, position_y, disabled)
		 VALUES ('n1', ?, 'trigger.manual', 'Start', '{}', 100, 200, 0)`, wfID)
	if err != nil {
		t.Fatalf("insert node 1: %v", err)
	}
	_, err = db.Exec(
		`INSERT INTO workflow_nodes (id, workflow_id, node_type, name, config, position_x, position_y, disabled)
		 VALUES ('n2', ?, 'core.set', 'Set Fields', '{"key":"value"}', 300, 200, 0)`, wfID)
	if err != nil {
		t.Fatalf("insert node 2: %v", err)
	}

	// Insert a connection
	_, err = db.Exec(
		`INSERT INTO workflow_connections (id, workflow_id, source_node_id, source_handle, target_node_id, target_handle, position)
		 VALUES ('c1', ?, 'n1', 'main', 'n2', 'main', 0)`, wfID)
	if err != nil {
		t.Fatalf("insert connection: %v", err)
	}

	result, err := ct.Execute("get_workflow_state", `{"workflow_id":"wf-1"}`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var state struct {
		WorkflowID  string          `json:"workflow_id"`
		Nodes       []nodeRow       `json:"nodes"`
		Connections []connectionRow `json:"connections"`
	}
	if err := json.Unmarshal([]byte(result), &state); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if state.WorkflowID != wfID {
		t.Errorf("workflow_id = %q, want %q", state.WorkflowID, wfID)
	}
	if len(state.Nodes) != 2 {
		t.Fatalf("got %d nodes, want 2", len(state.Nodes))
	}
	if len(state.Connections) != 1 {
		t.Fatalf("got %d connections, want 1", len(state.Connections))
	}
	if state.Connections[0].SourceNodeID != "n1" || state.Connections[0].TargetNodeID != "n2" {
		t.Errorf("connection mismatch: source=%s target=%s", state.Connections[0].SourceNodeID, state.Connections[0].TargetNodeID)
	}
}

func TestCreateNodes(t *testing.T) {
	db := setupTestDB(t)
	ct := NewCanvasTools(db)

	args := `{
		"workflow_id": "wf-1",
		"nodes": [
			{"node_type": "trigger.manual", "name": "Start", "position_x": 100, "position_y": 200},
			{"node_type": "core.set", "name": "Transform", "config": {"field": "val"}, "position_x": 300, "position_y": 200}
		]
	}`

	result, err := ct.Execute("create_nodes", args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var res struct {
		CreatedNodeIDs []string `json:"created_node_ids"`
	}
	if err := json.Unmarshal([]byte(result), &res); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if len(res.CreatedNodeIDs) != 2 {
		t.Fatalf("got %d IDs, want 2", len(res.CreatedNodeIDs))
	}

	// Verify nodes exist in DB
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM workflow_nodes WHERE workflow_id = 'wf-1'`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("DB has %d nodes, want 2", count)
	}

	// Verify the second node has config
	var configStr string
	if err := db.QueryRow(`SELECT config FROM workflow_nodes WHERE id = ?`, res.CreatedNodeIDs[1]).Scan(&configStr); err != nil {
		t.Fatalf("query config: %v", err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(configStr), &cfg); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg["field"] != "val" {
		t.Errorf("config field = %v, want 'val'", cfg["field"])
	}
}

func TestUpdateNodeConfig(t *testing.T) {
	db := setupTestDB(t)
	ct := NewCanvasTools(db)

	// Create a node first
	_, err := db.Exec(
		`INSERT INTO workflow_nodes (id, workflow_id, node_type, name, config, position_x, position_y, disabled)
		 VALUES ('n1', 'wf-1', 'core.set', 'Set', '{"existing":"keep"}', 0, 0, 0)`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	result, err := ct.Execute("update_node_config", `{"workflow_id":"wf-1","node_id":"n1","config":{"new_key":"new_val"}}`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var res struct {
		NodeID string                 `json:"node_id"`
		Config map[string]interface{} `json:"config"`
	}
	if err := json.Unmarshal([]byte(result), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if res.Config["existing"] != "keep" {
		t.Errorf("existing key lost: %v", res.Config)
	}
	if res.Config["new_key"] != "new_val" {
		t.Errorf("new key missing: %v", res.Config)
	}

	// Verify in DB
	var configStr string
	if err := db.QueryRow(`SELECT config FROM workflow_nodes WHERE id = 'n1'`).Scan(&configStr); err != nil {
		t.Fatalf("query: %v", err)
	}
	var dbCfg map[string]interface{}
	if err := json.Unmarshal([]byte(configStr), &dbCfg); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if dbCfg["existing"] != "keep" || dbCfg["new_key"] != "new_val" {
		t.Errorf("DB config mismatch: %v", dbCfg)
	}
}

func TestConnectNodes(t *testing.T) {
	db := setupTestDB(t)
	ct := NewCanvasTools(db)

	// Create two nodes
	for _, q := range []string{
		`INSERT INTO workflow_nodes (id, workflow_id, node_type, name, config) VALUES ('n1', 'wf-1', 'trigger.manual', 'Start', '{}')`,
		`INSERT INTO workflow_nodes (id, workflow_id, node_type, name, config) VALUES ('n2', 'wf-1', 'core.set', 'Set', '{}')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	result, err := ct.Execute("connect_nodes", `{
		"workflow_id": "wf-1",
		"source_node_id": "n1",
		"source_handle": "main",
		"target_node_id": "n2",
		"target_handle": "main"
	}`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var res struct {
		ConnectionID string `json:"connection_id"`
	}
	if err := json.Unmarshal([]byte(result), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if res.ConnectionID == "" {
		t.Fatal("expected non-empty connection_id")
	}

	// Verify in DB
	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM workflow_connections WHERE workflow_id='wf-1' AND source_node_id='n1' AND target_node_id='n2'`).Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Errorf("got %d connections, want 1", count)
	}
}

func TestDisconnectNodes(t *testing.T) {
	db := setupTestDB(t)
	ct := NewCanvasTools(db)

	// Insert a connection
	_, err := db.Exec(
		`INSERT INTO workflow_connections (id, workflow_id, source_node_id, source_handle, target_node_id, target_handle, position)
		 VALUES ('c1', 'wf-1', 'n1', 'main', 'n2', 'main', 0)`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	result, err := ct.Execute("disconnect_nodes", `{"workflow_id":"wf-1","source_node_id":"n1","target_node_id":"n2"}`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var res struct {
		DeletedCount int64 `json:"deleted_count"`
	}
	if err := json.Unmarshal([]byte(result), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if res.DeletedCount != 1 {
		t.Errorf("deleted_count = %d, want 1", res.DeletedCount)
	}

	// Verify gone
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM workflow_connections WHERE workflow_id='wf-1'`).Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 0 {
		t.Errorf("still %d connections, want 0", count)
	}
}

func TestDeleteNodes(t *testing.T) {
	db := setupTestDB(t)
	ct := NewCanvasTools(db)

	// Create a node and a connection involving it
	_, err := db.Exec(
		`INSERT INTO workflow_nodes (id, workflow_id, node_type, name, config) VALUES ('n1', 'wf-1', 'trigger.manual', 'Start', '{}')`)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}
	_, err = db.Exec(
		`INSERT INTO workflow_nodes (id, workflow_id, node_type, name, config) VALUES ('n2', 'wf-1', 'core.set', 'Set', '{}')`)
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}
	_, err = db.Exec(
		`INSERT INTO workflow_connections (id, workflow_id, source_node_id, source_handle, target_node_id, target_handle, position)
		 VALUES ('c1', 'wf-1', 'n1', 'main', 'n2', 'main', 0)`)
	if err != nil {
		t.Fatalf("insert conn: %v", err)
	}

	result, err := ct.Execute("delete_nodes", `{"workflow_id":"wf-1","node_ids":["n1"]}`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var res struct {
		DeletedNodeIDs []string `json:"deleted_node_ids"`
	}
	if err := json.Unmarshal([]byte(result), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(res.DeletedNodeIDs) != 1 || res.DeletedNodeIDs[0] != "n1" {
		t.Errorf("unexpected deleted IDs: %v", res.DeletedNodeIDs)
	}

	// Node should be gone
	var nodeCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM workflow_nodes WHERE id='n1'`).Scan(&nodeCount); err != nil {
		t.Fatalf("query: %v", err)
	}
	if nodeCount != 0 {
		t.Errorf("node still exists")
	}

	// Connection should also be gone (cascade)
	var connCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM workflow_connections WHERE source_node_id='n1' OR target_node_id='n1'`).Scan(&connCount); err != nil {
		t.Fatalf("query: %v", err)
	}
	if connCount != 0 {
		t.Errorf("connections still exist for deleted node")
	}

	// n2 should still exist
	var n2Count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM workflow_nodes WHERE id='n2'`).Scan(&n2Count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n2Count != 1 {
		t.Errorf("n2 was incorrectly deleted")
	}
}

func TestListAvailableNodes(t *testing.T) {
	db := setupTestDB(t)
	ct := NewCanvasTools(db)
	ct.SetNodeTypes([]NodeTypeInfo{
		{Type: "trigger.manual", Label: "Manual Trigger", Category: "trigger", Description: "Start manually"},
		{Type: "trigger.schedule", Label: "Schedule", Category: "trigger", Description: "Cron trigger"},
		{Type: "if", Label: "If", Category: "control", Description: "Branch"},
	})

	// No category filter
	result, err := ct.Execute("list_available_nodes", `{}`)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var res struct {
		NodeTypes []map[string]interface{} `json:"node_types"`
	}
	if err := json.Unmarshal([]byte(result), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(res.NodeTypes) == 0 {
		t.Fatal("expected non-empty node_types")
	}

	// With category filter
	result2, err := ct.Execute("list_available_nodes", `{"category":"trigger"}`)
	if err != nil {
		t.Fatalf("execute with filter: %v", err)
	}

	var res2 struct {
		NodeTypes []map[string]interface{} `json:"node_types"`
	}
	if err := json.Unmarshal([]byte(result2), &res2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, nt := range res2.NodeTypes {
		if nt["category"] != "trigger" {
			t.Errorf("expected category=trigger, got %v", nt["category"])
		}
	}
	if len(res2.NodeTypes) == 0 {
		t.Fatal("expected at least one trigger node type")
	}
}

func TestToolDefs(t *testing.T) {
	ct := NewCanvasTools(nil)
	defs := ct.ToolDefs()
	if len(defs) != 8 {
		t.Fatalf("got %d tool defs, want 8", len(defs))
	}

	names := make(map[string]bool)
	for _, d := range defs {
		if d.Type != "function" {
			t.Errorf("tool %s has type %q, want 'function'", d.Function.Name, d.Type)
		}
		names[d.Function.Name] = true
	}

	expected := []string{
		"get_workflow_state", "create_workflow", "create_nodes", "update_node_config",
		"delete_nodes", "connect_nodes", "disconnect_nodes", "list_available_nodes",
	}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing tool def: %s", name)
		}
	}
}

func TestExecuteUnknownTool(t *testing.T) {
	ct := NewCanvasTools(nil)
	_, err := ct.Execute("nonexistent", `{}`)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}
