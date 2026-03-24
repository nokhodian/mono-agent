package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
)

// WorkflowFile is the on-disk JSON representation of a workflow.
type WorkflowFile struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Version     int                `json:"version"`
	IsActive    bool               `json:"is_active"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
	Nodes       []WorkflowFileNode `json:"nodes"`
	Connections []WorkflowFileEdge `json:"connections"`
}

// WorkflowFileNode is a node as stored in the JSON file.
type WorkflowFileNode struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Position struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	} `json:"position"`
	Disabled bool                   `json:"disabled"`
	Config   map[string]interface{} `json:"config"`
	Schema   *NodeSchema            `json:"schema"`
}

// WorkflowFileEdge is a connection as stored in the JSON file.
type WorkflowFileEdge struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	SourceHandle string `json:"source_handle"`
	Target       string `json:"target"`
	TargetHandle string `json:"target_handle"`
}

// WorkflowFileStore implements workflow CRUD using JSON files.
// Does NOT handle executions or credentials — use SQLiteWorkflowStore for those.
type WorkflowFileStore struct {
	dir string
}

// NewWorkflowFileStore creates a WorkflowFileStore backed by the given directory.
// The directory is created if it does not exist.
func NewWorkflowFileStore(dir string) (*WorkflowFileStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("file_store: create dir %s: %w", dir, err)
	}
	return &WorkflowFileStore{dir: dir}, nil
}

func (s *WorkflowFileStore) filePath(id string) string {
	return filepath.Join(s.dir, id+".json")
}

// SaveWorkflow writes or updates a workflow JSON file.
// Assigns a new UUID if wf.ID is empty.
// Embeds default schema for any node missing its Schema field.
func (s *WorkflowFileStore) SaveWorkflow(ctx context.Context, wf *Workflow) error {
	if wf.ID == "" {
		wf.ID = uuid.New().String()
		wf.CreatedAt = time.Now().UTC()
	}
	wf.UpdatedAt = time.Now().UTC()
	if wf.Version == 0 {
		wf.Version = 1
	}

	wfFile := WorkflowFile{
		ID:          wf.ID,
		Name:        wf.Name,
		Description: wf.Description,
		Version:     wf.Version,
		IsActive:    wf.IsActive,
		CreatedAt:   wf.CreatedAt,
		UpdatedAt:   wf.UpdatedAt,
	}

	for _, n := range wf.Nodes {
		fn := WorkflowFileNode{
			ID:       n.ID,
			Type:     n.Type,
			Name:     n.Name,
			Disabled: n.Disabled,
			Config:   n.Config,
			Schema:   n.Schema,
		}
		fn.Position.X = n.PositionX
		fn.Position.Y = n.PositionY
		if fn.Config == nil {
			fn.Config = map[string]interface{}{}
		}
		if fn.Schema == nil {
			schema, err := LoadDefaultSchema(n.Type)
			if err == nil {
				fn.Schema = schema
			}
		}
		wfFile.Nodes = append(wfFile.Nodes, fn)
	}

	for _, c := range wf.Connections {
		wfFile.Connections = append(wfFile.Connections, WorkflowFileEdge{
			ID:           c.ID,
			Source:       c.SourceNodeID,
			SourceHandle: c.SourceHandle,
			Target:       c.TargetNodeID,
			TargetHandle: c.TargetHandle,
		})
	}

	data, err := json.MarshalIndent(wfFile, "", "  ")
	if err != nil {
		return fmt.Errorf("file_store: marshal %s: %w", wf.ID, err)
	}
	return os.WriteFile(s.filePath(wf.ID), data, 0o644)
}

// GetWorkflow reads a workflow JSON file by ID.
// Returns nil, nil if not found.
func (s *WorkflowFileStore) GetWorkflow(ctx context.Context, id string) (*Workflow, error) {
	data, err := os.ReadFile(s.filePath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("file_store: read %s: %w", id, err)
	}
	return parseWorkflowFile(data)
}

// ListWorkflows scans the directory and returns all workflows sorted by UpdatedAt desc.
func (s *WorkflowFileStore) ListWorkflows(ctx context.Context) ([]*Workflow, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("file_store: readdir %s: %w", s.dir, err)
	}
	var wfs []*Workflow
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		wf, err := parseWorkflowFile(data)
		if err != nil {
			continue
		}
		wfs = append(wfs, wf)
	}
	sort.Slice(wfs, func(i, j int) bool {
		return wfs[i].UpdatedAt.After(wfs[j].UpdatedAt)
	})
	return wfs, nil
}

// DeleteWorkflow removes the workflow JSON file.
func (s *WorkflowFileStore) DeleteWorkflow(ctx context.Context, id string) error {
	err := os.Remove(s.filePath(id))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ParseWorkflowFileBytes is the exported counterpart of parseWorkflowFile.
// It converts raw workflow-file JSON (using "type", "source", "target" keys)
// into a *Workflow. Used by the import command.
func ParseWorkflowFileBytes(data []byte) (Workflow, error) {
	wf, err := parseWorkflowFile(data)
	if err != nil {
		return Workflow{}, err
	}
	return *wf, nil
}

// parseWorkflowFile converts raw JSON bytes into a *Workflow.
func parseWorkflowFile(data []byte) (*Workflow, error) {
	var wfFile WorkflowFile
	if err := json.Unmarshal(data, &wfFile); err != nil {
		return nil, fmt.Errorf("file_store: unmarshal: %w", err)
	}
	wf := &Workflow{
		ID:          wfFile.ID,
		Name:        wfFile.Name,
		Description: wfFile.Description,
		Version:     wfFile.Version,
		IsActive:    wfFile.IsActive,
		CreatedAt:   wfFile.CreatedAt,
		UpdatedAt:   wfFile.UpdatedAt,
	}
	for _, fn := range wfFile.Nodes {
		n := WorkflowNode{
			ID:         fn.ID,
			WorkflowID: wf.ID,
			Type:       fn.Type,
			Name:       fn.Name,
			PositionX:  fn.Position.X,
			PositionY:  fn.Position.Y,
			Disabled:   fn.Disabled,
			Config:     fn.Config,
			Schema:     fn.Schema,
		}
		if n.Config == nil {
			n.Config = map[string]interface{}{}
		}
		wf.Nodes = append(wf.Nodes, n)
	}
	for _, fe := range wfFile.Connections {
		wf.Connections = append(wf.Connections, WorkflowConnection{
			ID:           fe.ID,
			WorkflowID:   wf.ID,
			SourceNodeID: fe.Source,
			SourceHandle: fe.SourceHandle,
			TargetNodeID: fe.Target,
			TargetHandle: fe.TargetHandle,
		})
	}
	return wf, nil
}
