package connections

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// newTestDB opens an in-memory SQLite database and ensures the connections
// table exists.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("newTestDB: open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	store := NewStore(db)
	if err := store.EnsureTable(context.Background()); err != nil {
		t.Fatalf("newTestDB: EnsureTable: %v", err)
	}
	return db
}

// TestStoreSaveAndGet verifies that a connection saved with an empty ID gets
// an auto-generated UUID, and that Get retrieves the same Label and Data.
func TestStoreSaveAndGet(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	conn := &Connection{
		Platform: "github",
		Method:   MethodAPIKey,
		Label:    "my github token",
		Data:     map[string]interface{}{"token": "ghp_test123"},
	}

	if err := store.Save(ctx, conn); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if conn.ID == "" {
		t.Fatal("Save did not assign an ID")
	}

	got, err := store.Get(ctx, conn.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil for an existing connection")
	}
	if got.Label != conn.Label {
		t.Errorf("Label: got %q, want %q", got.Label, conn.Label)
	}
	if got.Data["token"] != "ghp_test123" {
		t.Errorf("Data[token]: got %v, want %q", got.Data["token"], "ghp_test123")
	}
}

// TestStoreDelete verifies that after deleting a connection, Get returns nil.
func TestStoreDelete(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	conn := &Connection{
		Platform: "stripe",
		Method:   MethodAPIKey,
		Label:    "stripe test key",
		Data:     map[string]interface{}{"secret_key": "sk_test_abc"},
	}

	if err := store.Save(ctx, conn); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := store.Delete(ctx, conn.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := store.Get(ctx, conn.ID)
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if got != nil {
		t.Errorf("Get after delete: expected nil, got %+v", got)
	}
}

// TestStoreListByPlatform verifies that ListByPlatform filters correctly.
func TestStoreListByPlatform(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	conns := []*Connection{
		{Platform: "github", Method: MethodAPIKey, Label: "github work", Data: map[string]interface{}{}},
		{Platform: "github", Method: MethodOAuth, Label: "github personal", Data: map[string]interface{}{}},
		{Platform: "stripe", Method: MethodAPIKey, Label: "stripe prod", Data: map[string]interface{}{}},
	}
	for _, c := range conns {
		if err := store.Save(ctx, c); err != nil {
			t.Fatalf("Save %q: %v", c.Label, err)
		}
	}

	results, err := store.ListByPlatform(ctx, "github")
	if err != nil {
		t.Fatalf("ListByPlatform: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("ListByPlatform(\"github\"): got %d results, want 2", len(results))
	}
	for _, r := range results {
		if r.Platform != "github" {
			t.Errorf("unexpected platform %q in results", r.Platform)
		}
	}
}

// TestStoreMarkTested verifies that MarkTested updates status and last_tested,
// and returns an error for an unknown ID.
func TestStoreMarkTested(t *testing.T) {
	db := newTestDB(t)
	s := NewStore(db)
	ctx := context.Background()

	conn := &Connection{Platform: "github", Method: MethodAPIKey, Label: "G", Data: map[string]interface{}{}}
	_ = s.Save(ctx, conn)

	if err := s.MarkTested(ctx, conn.ID, "error"); err != nil {
		t.Fatalf("MarkTested: %v", err)
	}
	got, _ := s.Get(ctx, conn.ID)
	if got.Status != "error" {
		t.Errorf("expected status 'error', got %q", got.Status)
	}
	if got.LastTested == "" {
		t.Error("expected LastTested to be set")
	}

	// MarkTested on unknown ID should error
	if err := s.MarkTested(ctx, "nonexistent", "active"); err == nil {
		t.Error("expected error for unknown ID")
	}
}
