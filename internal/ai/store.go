package ai

import (
	"database/sql"
	"fmt"
	"time"
)

// AIProvider represents a user-configured AI provider instance.
type AIProvider struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderID   string `json:"provider_id"`   // references registry e.g. "openai"
	Tier         string `json:"tier"`           // "known" | "gateway"
	APIKey       string `json:"api_key"`
	BaseURL      string `json:"base_url"`
	DefaultModel string `json:"default_model"`
	ExtraHeaders string `json:"extra_headers"` // JSON string
	Status       string `json:"status"`        // "active" | "error" | "untested"
	LastTested   string `json:"last_tested"`
	CreatedAt    string `json:"created_at"`
}

// ChatMessage represents a single message in an AI chat conversation.
type ChatMessage struct {
	ID         string `json:"id"`
	WorkflowID string `json:"workflow_id"`
	Role       string `json:"role"`                    // "user" | "assistant" | "tool"
	Content    string `json:"content"`
	ToolCalls  string `json:"tool_calls,omitempty"`     // JSON array
	ToolCallID string `json:"tool_call_id,omitempty"`   // For tool result messages
	ProviderID string `json:"provider_id,omitempty"`
	Model      string `json:"model,omitempty"`
	TokenCount int    `json:"token_count,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// AIStore provides persistence for AI providers and chat messages.
type AIStore struct {
	db *sql.DB
}

// NewAIStore creates a new AIStore and ensures the required tables exist.
func NewAIStore(db *sql.DB) (*AIStore, error) {
	s := &AIStore{db: db}
	if err := s.initTables(); err != nil {
		return nil, fmt.Errorf("ai store: init tables: %w", err)
	}
	return s, nil
}

func (s *AIStore) initTables() error {
	const providersSQL = `CREATE TABLE IF NOT EXISTS ai_providers (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		provider_id TEXT NOT NULL,
		tier TEXT NOT NULL,
		api_key TEXT NOT NULL,
		base_url TEXT NOT NULL DEFAULT '',
		default_model TEXT NOT NULL DEFAULT '',
		extra_headers TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'untested',
		last_tested TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	)`

	const messagesSQL = `CREATE TABLE IF NOT EXISTS ai_chat_messages (
		id TEXT PRIMARY KEY,
		workflow_id TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		tool_calls TEXT NOT NULL DEFAULT '',
		tool_call_id TEXT NOT NULL DEFAULT '',
		provider_id TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		token_count INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL
	)`

	if _, err := s.db.Exec(providersSQL); err != nil {
		return fmt.Errorf("create ai_providers: %w", err)
	}
	if _, err := s.db.Exec(messagesSQL); err != nil {
		return fmt.Errorf("create ai_chat_messages: %w", err)
	}
	// Migrate: add tool_call_id column if missing (existing DBs).
	s.db.Exec(`ALTER TABLE ai_chat_messages ADD COLUMN tool_call_id TEXT NOT NULL DEFAULT ''`)
	return nil
}

// SaveProvider upserts an AI provider. If CreatedAt is empty it is set to now.
func (s *AIStore) SaveProvider(p AIProvider) error {
	if p.CreatedAt == "" {
		p.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	const q = `INSERT INTO ai_providers (id, name, provider_id, tier, api_key, base_url, default_model, extra_headers, status, last_tested, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			provider_id=excluded.provider_id,
			tier=excluded.tier,
			api_key=excluded.api_key,
			base_url=excluded.base_url,
			default_model=excluded.default_model,
			extra_headers=excluded.extra_headers,
			status=excluded.status,
			last_tested=excluded.last_tested,
			created_at=excluded.created_at`
	_, err := s.db.Exec(q,
		p.ID, p.Name, p.ProviderID, p.Tier, p.APIKey,
		p.BaseURL, p.DefaultModel, p.ExtraHeaders,
		p.Status, p.LastTested, p.CreatedAt,
	)
	return err
}

// GetProvider retrieves a single provider by ID.
func (s *AIStore) GetProvider(id string) (AIProvider, error) {
	const q = `SELECT id, name, provider_id, tier, api_key, base_url, default_model, extra_headers, status, last_tested, created_at
		FROM ai_providers WHERE id = ?`
	var p AIProvider
	err := s.db.QueryRow(q, id).Scan(
		&p.ID, &p.Name, &p.ProviderID, &p.Tier, &p.APIKey,
		&p.BaseURL, &p.DefaultModel, &p.ExtraHeaders,
		&p.Status, &p.LastTested, &p.CreatedAt,
	)
	if err != nil {
		return AIProvider{}, err
	}
	return p, nil
}

// ListProviders returns all providers ordered by created_at descending.
func (s *AIStore) ListProviders() ([]AIProvider, error) {
	const q = `SELECT id, name, provider_id, tier, api_key, base_url, default_model, extra_headers, status, last_tested, created_at
		FROM ai_providers ORDER BY created_at DESC`
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var providers []AIProvider
	for rows.Next() {
		var p AIProvider
		if err := rows.Scan(
			&p.ID, &p.Name, &p.ProviderID, &p.Tier, &p.APIKey,
			&p.BaseURL, &p.DefaultModel, &p.ExtraHeaders,
			&p.Status, &p.LastTested, &p.CreatedAt,
		); err != nil {
			return nil, err
		}
		providers = append(providers, p)
	}
	return providers, rows.Err()
}

// DeleteProvider removes a provider by ID.
func (s *AIStore) DeleteProvider(id string) error {
	_, err := s.db.Exec(`DELETE FROM ai_providers WHERE id = ?`, id)
	return err
}

// UpdateProviderStatus updates the status and last_tested fields for a provider.
func (s *AIStore) UpdateProviderStatus(id, status, lastTested string) error {
	_, err := s.db.Exec(
		`UPDATE ai_providers SET status = ?, last_tested = ? WHERE id = ?`,
		status, lastTested, id,
	)
	return err
}

// SaveChatMessage inserts a chat message. If CreatedAt is empty it is set to now.
func (s *AIStore) SaveChatMessage(m ChatMessage) error {
	if m.CreatedAt == "" {
		m.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	const q = `INSERT INTO ai_chat_messages (id, workflow_id, role, content, tool_calls, tool_call_id, provider_id, model, token_count, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := s.db.Exec(q,
		m.ID, m.WorkflowID, m.Role, m.Content,
		m.ToolCalls, m.ToolCallID, m.ProviderID, m.Model,
		m.TokenCount, m.CreatedAt,
	)
	return err
}

// GetChatHistory returns all messages for a workflow ordered by created_at ascending.
func (s *AIStore) GetChatHistory(workflowID string) ([]ChatMessage, error) {
	const q = `SELECT id, workflow_id, role, content, tool_calls, tool_call_id, provider_id, model, token_count, created_at
		FROM ai_chat_messages WHERE workflow_id = ? ORDER BY created_at ASC`
	rows, err := s.db.Query(q, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []ChatMessage
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(
			&m.ID, &m.WorkflowID, &m.Role, &m.Content,
			&m.ToolCalls, &m.ToolCallID, &m.ProviderID, &m.Model,
			&m.TokenCount, &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// ClearChatHistory deletes all messages for a given workflow.
func (s *AIStore) ClearChatHistory(workflowID string) error {
	_, err := s.db.Exec(`DELETE FROM ai_chat_messages WHERE workflow_id = ?`, workflowID)
	return err
}
