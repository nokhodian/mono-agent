package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
)

// newID generates a new UUID string for use as a primary key.
func newID() string {
	return uuid.New().String()
}

// sqliteTime wraps time.Time so it can scan both time.Time and string values
// from SQLite (modernc.org/sqlite stores timestamps as text).
type sqliteTime struct{ time.Time }

func (st *sqliteTime) Scan(src interface{}) error {
	switch v := src.(type) {
	case time.Time:
		st.Time = v
		return nil
	case string:
		for _, layout := range []string{
			time.RFC3339Nano, time.RFC3339,
			"2006-01-02T15:04:05Z", "2006-01-02 15:04:05",
			"2006-01-02 15:04:05.999999999 -0700 MST",
			"2006-01-02 15:04:05.999999999 +0000 UTC",
		} {
			if t, err := time.Parse(layout, v); err == nil {
				st.Time = t
				return nil
			}
		}
		return fmt.Errorf("sqliteTime: cannot parse %q", v)
	case nil:
		st.Time = time.Time{}
		return nil
	default:
		return fmt.Errorf("sqliteTime: unsupported type %T", src)
	}
}

// sqliteNullTime wraps *time.Time for nullable timestamp columns.
type sqliteNullTime struct{ ptr **time.Time }

func newSqliteNullTime(dst **time.Time) sqliteNullTime { return sqliteNullTime{ptr: dst} }

func (sn sqliteNullTime) Scan(src interface{}) error {
	if src == nil {
		*sn.ptr = nil
		return nil
	}
	var st sqliteTime
	if err := st.Scan(src); err != nil {
		return err
	}
	t := st.Time
	*sn.ptr = &t
	return nil
}

// ---------------------------------------------------------------------------
// WorkflowStore interface
// ---------------------------------------------------------------------------

// WorkflowStore defines all persistence operations for the workflow system.
type WorkflowStore interface {
	// Workflows
	CreateWorkflow(ctx context.Context, w *Workflow) error
	GetWorkflow(ctx context.Context, id string) (*Workflow, error)
	ListWorkflows(ctx context.Context) ([]Workflow, error)
	UpdateWorkflow(ctx context.Context, w *Workflow) error
	DeleteWorkflow(ctx context.Context, id string) error
	SetWorkflowActive(ctx context.Context, id string, active bool) error

	// Nodes (upsert all nodes for a workflow — delete removed, insert/update existing)
	SaveWorkflowNodes(ctx context.Context, workflowID string, nodes []WorkflowNode) error
	// Connections (upsert all connections for a workflow — delete removed, insert/update existing)
	SaveWorkflowConnections(ctx context.Context, workflowID string, conns []WorkflowConnection) error

	// Executions
	CreateExecution(ctx context.Context, e *WorkflowExecution) error
	GetExecution(ctx context.Context, id string) (*WorkflowExecution, error)
	ListExecutions(ctx context.Context, workflowID string, limit int) ([]WorkflowExecution, error)
	UpdateExecutionStatus(ctx context.Context, id string, status string, errMsg string) error
	SetExecutionStarted(ctx context.Context, id string) error
	SetExecutionFinished(ctx context.Context, id string, status string, errMsg string) error

	// Execution nodes
	CreateExecutionNode(ctx context.Context, en *WorkflowExecutionNode) error
	UpdateExecutionNode(ctx context.Context, en *WorkflowExecutionNode) error
	SetExecutionNodeFinished(ctx context.Context, id string, status string, outputItems []Item, errMsg string) error

	// Credentials
	CreateCredential(ctx context.Context, c *Credential) error
	GetCredential(ctx context.Context, id string) (*Credential, error)
	ListCredentials(ctx context.Context, credType string) ([]Credential, error)
	UpdateCredential(ctx context.Context, c *Credential) error
	DeleteCredential(ctx context.Context, id string) error

	// Recovery
	RecoverStaleExecutions(ctx context.Context) error
	PruneExecutions(ctx context.Context, workflowID string, keepCount int) error

	// RawDB returns the underlying *sql.DB for use by subsystems that need
	// direct DB access (e.g., vault registration).
	RawDB() *sql.DB
}

// ---------------------------------------------------------------------------
// SQLiteWorkflowStore
// ---------------------------------------------------------------------------

// SQLiteWorkflowStore implements WorkflowStore using a *sql.DB (SQLite).
type SQLiteWorkflowStore struct {
	db *sql.DB
}

// NewSQLiteWorkflowStore creates a new SQLiteWorkflowStore backed by db.
// The store does not run migrations; use the existing migration system.
func NewSQLiteWorkflowStore(db *sql.DB) *SQLiteWorkflowStore {
	return &SQLiteWorkflowStore{db: db}
}

// RawDB returns the underlying *sql.DB.
func (s *SQLiteWorkflowStore) RawDB() *sql.DB { return s.db }

// ---------------------------------------------------------------------------
// Workflow CRUD
// ---------------------------------------------------------------------------

// CreateWorkflow inserts a new workflow row. If w.ID is empty a UUID is generated.
// w.Nodes and w.Connections are ignored here; use SaveWorkflowNodes /
// SaveWorkflowConnections to persist the graph.
func (s *SQLiteWorkflowStore) CreateWorkflow(ctx context.Context, w *Workflow) error {
	if w.ID == "" {
		w.ID = newID()
	}
	now := time.Now().UTC()
	w.CreatedAt = now
	w.UpdatedAt = now
	if w.Version == 0 {
		w.Version = 1
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO workflows (id, name, description, is_active, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.Name, w.Description, boolToInt(w.IsActive), w.Version,
		w.CreatedAt, w.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("creating workflow %s: %w", w.ID, err)
	}
	return nil
}

// GetWorkflow retrieves a workflow by ID along with its nodes and connections.
// Returns nil, nil when not found.
func (s *SQLiteWorkflowStore) GetWorkflow(ctx context.Context, id string) (*Workflow, error) {
	w := &Workflow{}
	var isActive int
	var createdAt, updatedAt sqliteTime
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, is_active, version, created_at, updated_at
		FROM workflows WHERE id = ?`, id,
	).Scan(&w.ID, &w.Name, &w.Description, &isActive, &w.Version, &createdAt, &updatedAt)
	w.CreatedAt = createdAt.Time
	w.UpdatedAt = updatedAt.Time
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting workflow %s: %w", id, err)
	}
	w.IsActive = isActive != 0

	nodes, err := s.loadWorkflowNodes(ctx, id)
	if err != nil {
		return nil, err
	}
	w.Nodes = nodes

	conns, err := s.loadWorkflowConnections(ctx, id)
	if err != nil {
		return nil, err
	}
	w.Connections = conns

	return w, nil
}

// ListWorkflows returns all workflows ordered by created_at DESC.
// Nodes and Connections are not populated; call GetWorkflow for full detail.
func (s *SQLiteWorkflowStore) ListWorkflows(ctx context.Context) ([]Workflow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description, is_active, version, created_at, updated_at
		FROM workflows ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing workflows: %w", err)
	}
	defer rows.Close()

	var out []Workflow
	for rows.Next() {
		w := Workflow{}
		var isActive int
		var createdAt, updatedAt sqliteTime
		if err := rows.Scan(&w.ID, &w.Name, &w.Description, &isActive, &w.Version,
			&createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scanning workflow row: %w", err)
		}
		w.CreatedAt = createdAt.Time
		w.UpdatedAt = updatedAt.Time
		w.IsActive = isActive != 0
		out = append(out, w)
	}
	return out, rows.Err()
}

// UpdateWorkflow updates the mutable fields of an existing workflow.
// Nodes and Connections are not touched; use SaveWorkflowNodes /
// SaveWorkflowConnections for that.
func (s *SQLiteWorkflowStore) UpdateWorkflow(ctx context.Context, w *Workflow) error {
	w.UpdatedAt = time.Now().UTC()
	w.Version++

	_, err := s.db.ExecContext(ctx, `
		UPDATE workflows
		SET name = ?, description = ?, is_active = ?, version = ?, updated_at = ?
		WHERE id = ?`,
		w.Name, w.Description, boolToInt(w.IsActive), w.Version, w.UpdatedAt, w.ID,
	)
	if err != nil {
		return fmt.Errorf("updating workflow %s: %w", w.ID, err)
	}
	return nil
}

// DeleteWorkflow removes a workflow and, via ON DELETE CASCADE, all associated
// nodes, connections, and executions.
func (s *SQLiteWorkflowStore) DeleteWorkflow(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM workflows WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting workflow %s: %w", id, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("workflow %s not found", id)
	}
	return nil
}

// SetWorkflowActive toggles the is_active flag on a workflow.
func (s *SQLiteWorkflowStore) SetWorkflowActive(ctx context.Context, id string, active bool) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE workflows SET is_active = ?, updated_at = ? WHERE id = ?",
		boolToInt(active), time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("setting workflow %s active=%v: %w", id, active, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Nodes & Connections
// ---------------------------------------------------------------------------

// SaveWorkflowNodes replaces all nodes for a workflow atomically.
// Existing nodes are deleted and the supplied slice is inserted fresh.
func (s *SQLiteWorkflowStore) SaveWorkflowNodes(ctx context.Context, workflowID string, nodes []WorkflowNode) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning save-nodes transaction: %w", err)
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM workflow_nodes WHERE workflow_id = ?", workflowID); err != nil {
		tx.Rollback()
		return fmt.Errorf("deleting old nodes for workflow %s: %w", workflowID, err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO workflow_nodes
			(id, workflow_id, node_type, name, config, position_x, position_y, disabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("preparing node insert: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC()
	for i := range nodes {
		n := &nodes[i]
		if n.ID == "" {
			n.ID = newID()
		}
		n.WorkflowID = workflowID
		if err := n.MarshalConfig(); err != nil {
			tx.Rollback()
			return fmt.Errorf("marshalling config for node %s: %w", n.ID, err)
		}
		n.CreatedAt = now
		n.UpdatedAt = now

		if _, err := stmt.ExecContext(ctx,
			n.ID, n.WorkflowID, n.Type, n.Name, n.ConfigRaw,
			n.PositionX, n.PositionY, boolToInt(n.Disabled),
			n.CreatedAt, n.UpdatedAt,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("inserting node %s: %w", n.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing save-nodes transaction: %w", err)
	}
	return nil
}

// SaveWorkflowConnections replaces all connections for a workflow atomically.
// Existing connections are deleted and the supplied slice is inserted fresh.
func (s *SQLiteWorkflowStore) SaveWorkflowConnections(ctx context.Context, workflowID string, conns []WorkflowConnection) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning save-connections transaction: %w", err)
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM workflow_connections WHERE workflow_id = ?", workflowID); err != nil {
		tx.Rollback()
		return fmt.Errorf("deleting old connections for workflow %s: %w", workflowID, err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO workflow_connections
			(id, workflow_id, source_node_id, source_handle, target_node_id, target_handle, position)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("preparing connection insert: %w", err)
	}
	defer stmt.Close()

	for i := range conns {
		c := &conns[i]
		if c.ID == "" {
			c.ID = newID()
		}
		c.WorkflowID = workflowID

		if _, err := stmt.ExecContext(ctx,
			c.ID, c.WorkflowID, c.SourceNodeID, c.SourceHandle,
			c.TargetNodeID, c.TargetHandle, c.Position,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("inserting connection %s: %w", c.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing save-connections transaction: %w", err)
	}
	return nil
}

// loadWorkflowNodes is an internal helper to fetch nodes for a workflow.
func (s *SQLiteWorkflowStore) loadWorkflowNodes(ctx context.Context, workflowID string) ([]WorkflowNode, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, workflow_id, node_type, name, config, position_x, position_y, disabled, created_at, updated_at
		FROM workflow_nodes WHERE workflow_id = ? ORDER BY created_at ASC`, workflowID)
	if err != nil {
		return nil, fmt.Errorf("loading nodes for workflow %s: %w", workflowID, err)
	}
	defer rows.Close()

	var nodes []WorkflowNode
	for rows.Next() {
		n := WorkflowNode{}
		var disabled int
		var createdAt, updatedAt sqliteTime
		if err := rows.Scan(
			&n.ID, &n.WorkflowID, &n.Type, &n.Name, &n.ConfigRaw,
			&n.PositionX, &n.PositionY, &disabled, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning node row: %w", err)
		}
		n.CreatedAt = createdAt.Time
		n.UpdatedAt = updatedAt.Time
		n.Disabled = disabled != 0
		if err := n.ParseConfig(); err != nil {
			return nil, fmt.Errorf("parsing config for node %s: %w", n.ID, err)
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// loadWorkflowConnections is an internal helper to fetch connections for a workflow.
func (s *SQLiteWorkflowStore) loadWorkflowConnections(ctx context.Context, workflowID string) ([]WorkflowConnection, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, workflow_id, source_node_id, source_handle, target_node_id, target_handle, position
		FROM workflow_connections WHERE workflow_id = ? ORDER BY position ASC`, workflowID)
	if err != nil {
		return nil, fmt.Errorf("loading connections for workflow %s: %w", workflowID, err)
	}
	defer rows.Close()

	var conns []WorkflowConnection
	for rows.Next() {
		c := WorkflowConnection{}
		if err := rows.Scan(
			&c.ID, &c.WorkflowID, &c.SourceNodeID, &c.SourceHandle,
			&c.TargetNodeID, &c.TargetHandle, &c.Position,
		); err != nil {
			return nil, fmt.Errorf("scanning connection row: %w", err)
		}
		conns = append(conns, c)
	}
	return conns, rows.Err()
}

// ---------------------------------------------------------------------------
// Executions
// ---------------------------------------------------------------------------

// CreateExecution inserts a new workflow execution record.
// If e.ID is empty a UUID is generated.
func (s *SQLiteWorkflowStore) CreateExecution(ctx context.Context, e *WorkflowExecution) error {
	if e.ID == "" {
		e.ID = newID()
	}
	e.CreatedAt = time.Now().UTC()
	if e.Status == "" {
		e.Status = "QUEUED"
	}

	if err := e.MarshalTriggerData(); err != nil {
		return fmt.Errorf("marshalling trigger data for execution %s: %w", e.ID, err)
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO workflow_executions
			(id, workflow_id, status, trigger_type, trigger_data, started_at, finished_at, error_message, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.WorkflowID, e.Status, e.TriggerType, e.TriggerDataRaw,
		e.StartedAt, e.FinishedAt, e.ErrorMessage, e.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("creating execution %s: %w", e.ID, err)
	}
	return nil
}

// GetExecution retrieves a single execution by ID, populating its Nodes slice.
// Returns nil, nil when not found.
func (s *SQLiteWorkflowStore) GetExecution(ctx context.Context, id string) (*WorkflowExecution, error) {
	e := &WorkflowExecution{}
	var createdAt sqliteTime
	err := s.db.QueryRowContext(ctx, `
		SELECT id, workflow_id, status, trigger_type, trigger_data, started_at, finished_at, error_message, created_at
		FROM workflow_executions WHERE id = ?`, id,
	).Scan(
		&e.ID, &e.WorkflowID, &e.Status, &e.TriggerType, &e.TriggerDataRaw,
		newSqliteNullTime(&e.StartedAt), newSqliteNullTime(&e.FinishedAt), &e.ErrorMessage, &createdAt,
	)
	e.CreatedAt = createdAt.Time
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting execution %s: %w", id, err)
	}

	if err := e.ParseTriggerData(); err != nil {
		return nil, fmt.Errorf("parsing trigger data for execution %s: %w", id, err)
	}

	nodes, err := s.loadExecutionNodes(ctx, id)
	if err != nil {
		return nil, err
	}
	e.Nodes = nodes

	return e, nil
}

// ListExecutions returns executions for a workflow ordered by created_at DESC.
// Pass limit <= 0 to return all executions. Execution nodes are not populated.
func (s *SQLiteWorkflowStore) ListExecutions(ctx context.Context, workflowID string, limit int) ([]WorkflowExecution, error) {
	query := `
		SELECT id, workflow_id, status, trigger_type, trigger_data, started_at, finished_at, error_message, created_at
		FROM workflow_executions WHERE workflow_id = ? ORDER BY created_at DESC`
	var args []interface{}
	args = append(args, workflowID)

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing executions for workflow %s: %w", workflowID, err)
	}
	defer rows.Close()

	var out []WorkflowExecution
	for rows.Next() {
		e := WorkflowExecution{}
		var createdAt sqliteTime
		if err := rows.Scan(
			&e.ID, &e.WorkflowID, &e.Status, &e.TriggerType, &e.TriggerDataRaw,
			newSqliteNullTime(&e.StartedAt), newSqliteNullTime(&e.FinishedAt), &e.ErrorMessage, &createdAt,
		); err != nil {
			return nil, fmt.Errorf("scanning execution row: %w", err)
		}
		e.CreatedAt = createdAt.Time
		if err := e.ParseTriggerData(); err != nil {
			return nil, fmt.Errorf("parsing trigger data for execution %s: %w", e.ID, err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// UpdateExecutionStatus sets status and error_message on an execution.
func (s *SQLiteWorkflowStore) UpdateExecutionStatus(ctx context.Context, id string, status string, errMsg string) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE workflow_executions SET status = ?, error_message = ? WHERE id = ?",
		status, errMsg, id,
	)
	if err != nil {
		return fmt.Errorf("updating execution status %s: %w", id, err)
	}
	return nil
}

// SetExecutionStarted marks an execution as RUNNING and records started_at and the current PID.
func (s *SQLiteWorkflowStore) SetExecutionStarted(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		"UPDATE workflow_executions SET status = 'RUNNING', started_at = ?, pid = ? WHERE id = ?",
		now, os.Getpid(), id,
	)
	if err != nil {
		return fmt.Errorf("setting execution started %s: %w", id, err)
	}
	return nil
}

// SetExecutionFinished records finished_at, final status and optional error.
func (s *SQLiteWorkflowStore) SetExecutionFinished(ctx context.Context, id string, status string, errMsg string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		"UPDATE workflow_executions SET status = ?, finished_at = ?, error_message = ? WHERE id = ?",
		status, now, errMsg, id,
	)
	if err != nil {
		return fmt.Errorf("setting execution finished %s: %w", id, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Execution nodes
// ---------------------------------------------------------------------------

// CreateExecutionNode inserts a new execution-node record.
// If en.ID is empty a UUID is generated.
func (s *SQLiteWorkflowStore) CreateExecutionNode(ctx context.Context, en *WorkflowExecutionNode) error {
	if en.ID == "" {
		en.ID = newID()
	}
	if en.Status == "" {
		en.Status = "PENDING"
	}

	if err := en.MarshalItems(); err != nil {
		return fmt.Errorf("marshalling items for execution node %s: %w", en.ID, err)
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO workflow_execution_nodes
			(id, execution_id, node_id, node_name, status, input_items, output_items, error_message, started_at, finished_at, retry_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		en.ID, en.ExecutionID, en.NodeID, en.NodeName, en.Status,
		en.InputRaw, en.OutputRaw, en.ErrorMessage,
		en.StartedAt, en.FinishedAt, en.RetryCount,
	)
	if err != nil {
		return fmt.Errorf("creating execution node %s: %w", en.ID, err)
	}
	return nil
}

// UpdateExecutionNode updates all mutable fields of an execution-node record.
func (s *SQLiteWorkflowStore) UpdateExecutionNode(ctx context.Context, en *WorkflowExecutionNode) error {
	if err := en.MarshalItems(); err != nil {
		return fmt.Errorf("marshalling items for execution node %s: %w", en.ID, err)
	}

	_, err := s.db.ExecContext(ctx, `
		UPDATE workflow_execution_nodes
		SET status = ?, input_items = ?, output_items = ?, error_message = ?,
		    started_at = ?, finished_at = ?, retry_count = ?
		WHERE id = ?`,
		en.Status, en.InputRaw, en.OutputRaw, en.ErrorMessage,
		en.StartedAt, en.FinishedAt, en.RetryCount, en.ID,
	)
	if err != nil {
		return fmt.Errorf("updating execution node %s: %w", en.ID, err)
	}
	return nil
}

// SetExecutionNodeFinished marks an execution node as finished, marshals
// outputItems to JSON, and records status, error_message, and finished_at.
func (s *SQLiteWorkflowStore) SetExecutionNodeFinished(ctx context.Context, id string, status string, outputItems []Item, errMsg string) error {
	outputRaw, err := marshalItems(outputItems)
	if err != nil {
		return fmt.Errorf("marshalling output items for execution node %s: %w", id, err)
	}

	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		UPDATE workflow_execution_nodes
		SET status = ?, output_items = ?, error_message = ?, finished_at = ?
		WHERE id = ?`,
		status, outputRaw, errMsg, now, id,
	)
	if err != nil {
		return fmt.Errorf("setting execution node finished %s: %w", id, err)
	}
	return nil
}

// loadExecutionNodes is an internal helper to fetch execution-node records.
func (s *SQLiteWorkflowStore) loadExecutionNodes(ctx context.Context, executionID string) ([]WorkflowExecutionNode, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, execution_id, node_id, node_name, status, input_items, output_items, error_message, started_at, finished_at, retry_count
		FROM workflow_execution_nodes WHERE execution_id = ? ORDER BY rowid ASC`, executionID)
	if err != nil {
		return nil, fmt.Errorf("loading execution nodes for execution %s: %w", executionID, err)
	}
	defer rows.Close()

	var out []WorkflowExecutionNode
	for rows.Next() {
		en := WorkflowExecutionNode{}
		if err := rows.Scan(
			&en.ID, &en.ExecutionID, &en.NodeID, &en.NodeName, &en.Status,
			&en.InputRaw, &en.OutputRaw, &en.ErrorMessage,
			newSqliteNullTime(&en.StartedAt), newSqliteNullTime(&en.FinishedAt), &en.RetryCount,
		); err != nil {
			return nil, fmt.Errorf("scanning execution node row: %w", err)
		}
		if err := en.ParseItems(); err != nil {
			return nil, fmt.Errorf("parsing items for execution node %s: %w", en.ID, err)
		}
		out = append(out, en)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Credentials
// ---------------------------------------------------------------------------

// CreateCredential inserts a new credential record.
// If c.ID is empty a UUID is generated.
func (s *SQLiteWorkflowStore) CreateCredential(ctx context.Context, c *Credential) error {
	if c.ID == "" {
		c.ID = newID()
	}
	now := time.Now().UTC()
	c.CreatedAt = now
	c.UpdatedAt = now

	dataRaw, err := marshalCredentialData(c.Data)
	if err != nil {
		return fmt.Errorf("marshalling credential data for %s: %w", c.ID, err)
	}
	c.DataRaw = dataRaw

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO workflow_credentials (id, name, type, data, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		c.ID, c.Name, c.Type, c.DataRaw, c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("creating credential %s: %w", c.ID, err)
	}
	return nil
}

// GetCredential retrieves a single credential by ID.
// Returns nil, nil when not found.
func (s *SQLiteWorkflowStore) GetCredential(ctx context.Context, id string) (*Credential, error) {
	c := &Credential{}
	var createdAt, updatedAt sqliteTime
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, type, data, created_at, updated_at
		FROM workflow_credentials WHERE id = ?`, id,
	).Scan(&c.ID, &c.Name, &c.Type, &c.DataRaw, &createdAt, &updatedAt)
	c.CreatedAt = createdAt.Time
	c.UpdatedAt = updatedAt.Time
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting credential %s: %w", id, err)
	}
	if err := parseCredentialData(c); err != nil {
		return nil, fmt.Errorf("parsing credential data for %s: %w", id, err)
	}
	return c, nil
}

// ListCredentials returns all credentials, optionally filtered by type.
// Pass an empty string to return all types.
func (s *SQLiteWorkflowStore) ListCredentials(ctx context.Context, credType string) ([]Credential, error) {
	query := "SELECT id, name, type, data, created_at, updated_at FROM workflow_credentials"
	var args []interface{}
	if credType != "" {
		query += " WHERE type = ?"
		args = append(args, credType)
	}
	query += " ORDER BY name ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing credentials: %w", err)
	}
	defer rows.Close()

	var out []Credential
	for rows.Next() {
		c := Credential{}
		var createdAt, updatedAt sqliteTime
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &c.DataRaw, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scanning credential row: %w", err)
		}
		c.CreatedAt = createdAt.Time
		c.UpdatedAt = updatedAt.Time
		if err := parseCredentialData(&c); err != nil {
			return nil, fmt.Errorf("parsing credential data for %s: %w", c.ID, err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateCredential updates the mutable fields of a credential.
func (s *SQLiteWorkflowStore) UpdateCredential(ctx context.Context, c *Credential) error {
	c.UpdatedAt = time.Now().UTC()

	dataRaw, err := marshalCredentialData(c.Data)
	if err != nil {
		return fmt.Errorf("marshalling credential data for %s: %w", c.ID, err)
	}
	c.DataRaw = dataRaw

	_, err = s.db.ExecContext(ctx, `
		UPDATE workflow_credentials SET name = ?, type = ?, data = ?, updated_at = ? WHERE id = ?`,
		c.Name, c.Type, c.DataRaw, c.UpdatedAt, c.ID,
	)
	if err != nil {
		return fmt.Errorf("updating credential %s: %w", c.ID, err)
	}
	return nil
}

// DeleteCredential removes a credential by ID.
func (s *SQLiteWorkflowStore) DeleteCredential(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM workflow_credentials WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting credential %s: %w", id, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("credential %s not found", id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Recovery & maintenance
// ---------------------------------------------------------------------------

// RecoverStaleExecutions transitions any RUNNING or QUEUED executions to FAILED.
// This should be called once on process startup to handle executions that were
// interrupted by a previous crash or restart.
func (s *SQLiteWorkflowStore) RecoverStaleExecutions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE workflow_executions
		SET status = 'FAILED',
		    error_message = 'recovered: process restart',
		    finished_at = CURRENT_TIMESTAMP
		WHERE status IN ('RUNNING', 'QUEUED')`)
	if err != nil {
		return fmt.Errorf("recovering stale executions: %w", err)
	}
	return nil
}

// PruneExecutions deletes the oldest executions for a workflow when the total
// count exceeds keepCount. Executions are ordered by created_at; the oldest
// are removed first.
func (s *SQLiteWorkflowStore) PruneExecutions(ctx context.Context, workflowID string, keepCount int) error {
	if keepCount < 0 {
		keepCount = 0
	}
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM workflow_executions
		WHERE workflow_id = ?
		  AND id NOT IN (
		      SELECT id FROM workflow_executions
		      WHERE workflow_id = ?
		      ORDER BY created_at DESC
		      LIMIT ?
		  )`,
		workflowID, workflowID, keepCount,
	)
	if err != nil {
		return fmt.Errorf("pruning executions for workflow %s: %w", workflowID, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// boolToInt converts a Go bool to a SQLite-friendly integer (0 or 1).
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// marshalItems encodes a slice of Items to a JSON string.
// Returns "[]" for a nil or empty slice.
func marshalItems(items []Item) (string, error) {
	if len(items) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// marshalCredentialData encodes a credential data map to a JSON string.
func marshalCredentialData(data map[string]interface{}) (string, error) {
	if data == nil {
		return "{}", nil
	}
	b, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// parseCredentialData unmarshals DataRaw JSON into Data.
func parseCredentialData(c *Credential) error {
	if c.DataRaw == "" {
		c.Data = make(map[string]interface{})
		return nil
	}
	var d map[string]interface{}
	if err := json.Unmarshal([]byte(c.DataRaw), &d); err != nil {
		return err
	}
	c.Data = d
	return nil
}
