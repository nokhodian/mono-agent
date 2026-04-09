# Image Vault Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a central image vault — auto-populated by Gemini/AI workflow nodes, manually addable via drag-drop/picker, browsable in the Wails UI, and referenceable in workflow configs via `@img-NNN` syntax.

**Architecture:** CLI side (Go) owns the vault DB table and file storage (`~/.monoes/vault/`); the Gemini bot auto-registers images after download; `@img-NNN` refs are resolved in the workflow execution engine before node execution. Wails app.go exposes 6 read/write methods; the frontend adds an ImageVault page under DATA nav and `@`-autocomplete + picker in the workflow editor.

**Tech Stack:** Go (SQLite via modernc.org/sqlite), React/JSX (Wails v2 frontend), Lucide icons, existing `wails-app/frontend` conventions.

---

## File Map

**Create:**
- `data/migrations/009_image_vault.sql` — vault_images table
- `internal/vault/vault.go` — Register, Resolve, ResolveConfig, context helpers
- `wails-app/frontend/src/pages/ImageVault.jsx` — main vault page
- `wails-app/frontend/src/components/ImageDetailModal.jsx` — click-row modal
- `wails-app/frontend/src/components/ImagePickerModal.jsx` — picker from workflow editor

**Modify:**
- `internal/workflow/storage.go` — add `RawDB() *sql.DB` to interface + both implementations
- `internal/workflow/execution.go` — call `vault.ResolveConfig` after template resolution
- `internal/workflow/engine.go` — pass vault DB via context before RunExecution
- `internal/bot/gemini/bot.go` — auto-register images after download
- `wails-app/app.go` — 6 vault methods + vault dir creation in startup
- `wails-app/main.go` — asset handler for `/vault-image/` paths
- `wails-app/frontend/src/components/Sidebar.jsx` — add Images nav item
- `wails-app/frontend/src/App.jsx` — add vault page route
- `wails-app/frontend/src/wailsjs/wailsjs/go/main/App.js` — new JS bindings
- `wails-app/frontend/src/wailsjs/wailsjs/go/main/App.d.ts` — type defs
- `wails-app/frontend/src/pages/NodeRunner.jsx` — `@`-autocomplete + picker button

---

## Task 1: DB Migration

**Files:**
- Create: `data/migrations/009_image_vault.sql`

- [ ] **Create the migration file**

```sql
-- data/migrations/009_image_vault.sql
CREATE TABLE IF NOT EXISTS vault_images (
    id           TEXT PRIMARY KEY,
    seq          INTEGER NOT NULL UNIQUE,
    path         TEXT NOT NULL,
    filename     TEXT NOT NULL,
    size_bytes   INTEGER NOT NULL DEFAULT 0,
    source       TEXT NOT NULL DEFAULT 'upload',
    workflow_id  TEXT,
    execution_id TEXT,
    label        TEXT,
    created_at   TIMESTAMP NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_vault_images_seq ON vault_images(seq DESC);
```

- [ ] **Verify it gets picked up by the embed system**

Check `data/embed.go` — it should have a `//go:embed migrations/*.sql` directive. If it embeds all `.sql` files in migrations/, the new file is automatically included. Run:

```bash
cd /Users/morteza/Desktop/monoes/monoes-agent/newmonoes && go build ./... 2>&1
```
Expected: no errors.

- [ ] **Commit**

```bash
git add data/migrations/009_image_vault.sql
git commit -m "feat: add vault_images DB migration"
```

---

## Task 2: vault package

**Files:**
- Create: `internal/vault/vault.go`

- [ ] **Create the vault package**

```go
package vault

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type ctxKey struct{}

// ContextWithDB returns a new context carrying db for vault operations.
func ContextWithDB(ctx context.Context, db *sql.DB) context.Context {
	return context.WithValue(ctx, ctxKey{}, db)
}

// DBFromContext retrieves the *sql.DB stored by ContextWithDB. Returns nil if absent.
func DBFromContext(ctx context.Context) *sql.DB {
	db, _ := ctx.Value(ctxKey{}).(*sql.DB)
	return db
}

// VaultDir returns the absolute path of the vault directory (~/.monoes/vault/).
func VaultDir() string {
	return filepath.Join(os.Getenv("HOME"), ".monoes", "vault")
}

// EnsureVaultDir creates the vault directory if it does not exist.
func EnsureVaultDir() error {
	return os.MkdirAll(VaultDir(), 0700)
}

// Register copies the file at src into the vault, inserts a DB row, and
// returns the new vault ID (e.g. "img-001").
// source should be "gemini", "upload", "huggingface", etc.
// workflowID and executionID may be empty strings.
func Register(ctx context.Context, db *sql.DB, src, source, workflowID, executionID string) (string, error) {
	if db == nil {
		return "", fmt.Errorf("vault.Register: db is nil")
	}
	if err := EnsureVaultDir(); err != nil {
		return "", fmt.Errorf("vault.Register: ensure vault dir: %w", err)
	}

	// Determine next seq atomically.
	var seq int
	err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq), 0) + 1 FROM vault_images`).Scan(&seq)
	if err != nil {
		return "", fmt.Errorf("vault.Register: get next seq: %w", err)
	}

	id := fmt.Sprintf("img-%03d", seq)
	ext := filepath.Ext(src)
	if ext == "" {
		ext = ".png"
	}
	destFilename := id + ext
	destPath := filepath.Join(VaultDir(), destFilename)

	// Copy source file to vault.
	if err := copyFile(src, destPath); err != nil {
		return "", fmt.Errorf("vault.Register: copy file: %w", err)
	}

	// Get file size.
	fi, err := os.Stat(destPath)
	if err != nil {
		return "", fmt.Errorf("vault.Register: stat dest: %w", err)
	}

	// Nullable string helpers.
	nullStr := func(s string) interface{} {
		if s == "" {
			return nil
		}
		return s
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO vault_images (id, seq, path, filename, size_bytes, source, workflow_id, execution_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, seq, destPath, destFilename, fi.Size(), source,
		nullStr(workflowID), nullStr(executionID),
	)
	if err != nil {
		return "", fmt.Errorf("vault.Register: insert: %w", err)
	}
	return id, nil
}

// Resolve turns "@img-001" into the absolute file path stored in the DB.
// Returns an error if the image is not found.
func Resolve(db *sql.DB, ref string) (string, error) {
	if !strings.HasPrefix(ref, "@") {
		return ref, nil
	}
	id := strings.TrimPrefix(ref, "@")
	var path string
	err := db.QueryRow(`SELECT path FROM vault_images WHERE id = ?`, id).Scan(&path)
	if err == sql.ErrNoRows {
		return ref, fmt.Errorf("vault.Resolve: image %q not found", id)
	}
	if err != nil {
		return ref, fmt.Errorf("vault.Resolve: %w", err)
	}
	return path, nil
}

// ResolveConfig walks a config map and replaces any string value that starts
// with "@img-" with its absolute file path from the vault.
// Keys with missing images are left as-is (the ref string) and a warning is logged to stderr.
func ResolveConfig(db *sql.DB, config map[string]interface{}) error {
	if db == nil {
		return nil
	}
	for k, v := range config {
		s, ok := v.(string)
		if !ok {
			continue
		}
		if !strings.HasPrefix(s, "@img-") {
			continue
		}
		resolved, err := Resolve(db, s)
		if err != nil {
			// Non-fatal: leave original ref, emit warning.
			fmt.Fprintf(os.Stderr, "vault: warning: %v\n", err)
			continue
		}
		config[k] = resolved
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
```

- [ ] **Build check**

```bash
cd /Users/morteza/Desktop/monoes/monoes-agent/newmonoes && go build ./internal/vault/ 2>&1
```
Expected: no errors.

- [ ] **Commit**

```bash
git add internal/vault/vault.go
git commit -m "feat: add internal/vault package (Register, Resolve, ResolveConfig)"
```

---

## Task 3: WorkflowStore.RawDB()

**Files:**
- Modify: `internal/workflow/storage.go`
- Modify: `internal/workflow/hybrid_store.go`

- [ ] **Add RawDB() to WorkflowStore interface** (`internal/workflow/storage.go` after line 109, before the closing `}` of the interface)

Find the end of the `WorkflowStore` interface (line ~110 where `}` closes it) and add the method before it:

```go
	// RawDB returns the underlying *sql.DB for use by subsystems that need
	// direct DB access (e.g., vault registration).
	RawDB() *sql.DB
```

- [ ] **Implement RawDB() on SQLiteWorkflowStore** (add after the `NewSQLiteWorkflowStore` function, ~line 125):

```go
// RawDB returns the underlying *sql.DB.
func (s *SQLiteWorkflowStore) RawDB() *sql.DB { return s.db }
```

- [ ] **Implement RawDB() on HybridWorkflowStore** (add to `internal/workflow/hybrid_store.go`):

Open `internal/workflow/hybrid_store.go` and append at the end:

```go
// RawDB delegates to the SQLite store's underlying DB.
func (h *HybridWorkflowStore) RawDB() *sql.DB { return h.sql.RawDB() }
```

- [ ] **Build check**

```bash
cd /Users/morteza/Desktop/monoes/monoes-agent/newmonoes && go build ./internal/workflow/ 2>&1
```
Expected: no errors.

- [ ] **Commit**

```bash
git add internal/workflow/storage.go internal/workflow/hybrid_store.go
git commit -m "feat: add RawDB() to WorkflowStore interface for vault access"
```

---

## Task 4: vault.ResolveConfig in execution engine

**Files:**
- Modify: `internal/workflow/engine.go`
- Modify: `internal/workflow/execution.go`

- [ ] **Pass vault DB via context in engine.go**

In `internal/workflow/engine.go`, find `runExecution` (~line 687):

```go
func (e *WorkflowEngine) runExecution(ctx context.Context, exec *WorkflowExecution, wf *Workflow, dag *DAG) error {
	return RunExecution(ctx, exec, wf, dag, e.registry, e.store, e.connStore, e.expr, e.logger)
}
```

Replace with:

```go
func (e *WorkflowEngine) runExecution(ctx context.Context, exec *WorkflowExecution, wf *Workflow, dag *DAG) error {
	ctx = vault.ContextWithDB(ctx, e.store.RawDB())
	return RunExecution(ctx, exec, wf, dag, e.registry, e.store, e.connStore, e.expr, e.logger)
}
```

Add `"github.com/nokhodian/mono-agent/internal/vault"` to the imports in `engine.go`.

- [ ] **Call vault.ResolveConfig in execution.go after template resolution**

In `internal/workflow/execution.go`, find the block that restores per-item fields (around line 225-228):

```go
		// Restore the per-item fields unresolved.
		for k, v := range savedFields {
			resolvedConfig[k] = v
		}
```

Add vault resolution immediately after this block:

```go
		// Restore the per-item fields unresolved.
		for k, v := range savedFields {
			resolvedConfig[k] = v
		}

		// Resolve @img-NNN references to absolute vault file paths.
		if vaultDB := vault.DBFromContext(ctx); vaultDB != nil {
			_ = vault.ResolveConfig(vaultDB, resolvedConfig)
		}
```

Add `"github.com/nokhodian/mono-agent/internal/vault"` to the imports in `execution.go`.

- [ ] **Build check**

```bash
cd /Users/morteza/Desktop/monoes/monoes-agent/newmonoes && go build ./... 2>&1
```
Expected: no errors.

- [ ] **Commit**

```bash
git add internal/workflow/engine.go internal/workflow/execution.go
git commit -m "feat: resolve @img-NNN vault refs in workflow execution"
```

---

## Task 5: Gemini bot auto-registration

**Files:**
- Modify: `internal/bot/gemini/bot.go`

- [ ] **Add vault import to gemini/bot.go**

Add to the import block in `internal/bot/gemini/bot.go`:

```go
"github.com/nokhodian/mono-agent/internal/vault"
```

- [ ] **Register images after download in extract_and_download_images**

Find the end of `extract_and_download_images` (around lines 754-768) — after the symlink creation and before the return statement:

```go
	if first, ok := downloaded[0]["path"].(string); ok {
		latestLink := filepath.Join(downloadDir, "latest_gemini.png")
		_ = os.Remove(latestLink)
		_ = os.Symlink(first, latestLink)
	}

	return map[string]interface{}{
		"success":     true,
		"images":      downloaded,
		"image_count": len(downloaded),
	}, nil
```

Replace with:

```go
	if first, ok := downloaded[0]["path"].(string); ok {
		latestLink := filepath.Join(downloadDir, "latest_gemini.png")
		_ = os.Remove(latestLink)
		_ = os.Symlink(first, latestLink)
	}

	// Register each downloaded image in the vault.
	if vaultDB := vault.DBFromContext(ctx); vaultDB != nil {
		for i, img := range downloaded {
			path, _ := img["path"].(string)
			if path == "" {
				continue
			}
			vaultID, err := vault.Register(ctx, vaultDB, path, "gemini", "", "")
			if err == nil {
				downloaded[i]["vault_id"] = vaultID
			}
		}
	}

	return map[string]interface{}{
		"success":     true,
		"images":      downloaded,
		"image_count": len(downloaded),
	}, nil
```

Note: The `ctx` variable is already in scope — `extract_and_download_images` signature is `func(ctx context.Context, args ...interface{})`.

- [ ] **Build check**

```bash
cd /Users/morteza/Desktop/monoes/monoes-agent/newmonoes && go build ./... 2>&1
```
Expected: no errors.

- [ ] **Commit**

```bash
git add internal/bot/gemini/bot.go
git commit -m "feat: auto-register Gemini images in vault after download"
```

---

## Task 6: Wails app.go vault methods

**Files:**
- Modify: `wails-app/app.go`

- [ ] **Add vault dir creation in startup**

In `wails-app/app.go`, in the `startup` function, after `a.db = db` (~line 60), add:

```go
	// Ensure vault directory exists.
	vaultDir := filepath.Join(os.Getenv("HOME"), ".monoes", "vault")
	if err := os.MkdirAll(vaultDir, 0700); err != nil {
		runtime.LogErrorf(ctx, "vault dir error: %v", err)
	}
```

(The `os` and `filepath` packages are already imported.)

- [ ] **Add the six vault methods** — append to the end of `wails-app/app.go`:

```go
// ── Image Vault ──────────────────────────────────────────────────────────────

func (a *App) GetVaultImages(limit int) ([]map[string]interface{}, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not available")
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := a.db.Query(`
		SELECT id, seq, path, filename, size_bytes, source,
		       COALESCE(workflow_id,'') as workflow_id,
		       COALESCE(execution_id,'') as execution_id,
		       COALESCE(label,'') as label, created_at
		FROM vault_images ORDER BY seq DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]interface{}
	for rows.Next() {
		var id, path, filename, source, workflowID, executionID, label, createdAt string
		var seq, sizeBytes int
		if err := rows.Scan(&id, &seq, &path, &filename, &sizeBytes, &source, &workflowID, &executionID, &label, &createdAt); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"id": id, "seq": seq, "path": path, "filename": filename,
			"size_bytes": sizeBytes, "source": source,
			"workflow_id": workflowID, "execution_id": executionID,
			"label": label, "created_at": createdAt,
			"url": "/vault-image/" + filename,
		})
	}
	if out == nil {
		out = []map[string]interface{}{}
	}
	return out, nil
}

func (a *App) GetVaultImage(id string) (map[string]interface{}, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not available")
	}
	var path, filename, source, workflowID, executionID, label, createdAt string
	var seq, sizeBytes int
	err := a.db.QueryRow(`
		SELECT id, seq, path, filename, size_bytes, source,
		       COALESCE(workflow_id,'') as workflow_id,
		       COALESCE(execution_id,'') as execution_id,
		       COALESCE(label,'') as label, created_at
		FROM vault_images WHERE id = ?`, id).
		Scan(&id, &seq, &path, &filename, &sizeBytes, &source, &workflowID, &executionID, &label, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("vault image %q not found: %w", id, err)
	}
	return map[string]interface{}{
		"id": id, "seq": seq, "path": path, "filename": filename,
		"size_bytes": sizeBytes, "source": source,
		"workflow_id": workflowID, "execution_id": executionID,
		"label": label, "created_at": createdAt,
		"url": "/vault-image/" + filename,
	}, nil
}

func (a *App) AddVaultImage(srcPath, label string) (map[string]interface{}, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not available")
	}
	vaultDir := filepath.Join(os.Getenv("HOME"), ".monoes", "vault")
	if err := os.MkdirAll(vaultDir, 0700); err != nil {
		return nil, fmt.Errorf("vault dir: %w", err)
	}

	var seq int
	if err := a.db.QueryRow(`SELECT COALESCE(MAX(seq), 0) + 1 FROM vault_images`).Scan(&seq); err != nil {
		return nil, fmt.Errorf("vault seq: %w", err)
	}
	id := fmt.Sprintf("img-%03d", seq)
	ext := filepath.Ext(srcPath)
	if ext == "" {
		ext = ".png"
	}
	destFilename := id + ext
	destPath := filepath.Join(vaultDir, destFilename)

	in, err := os.Open(srcPath)
	if err != nil {
		return nil, fmt.Errorf("open source: %w", err)
	}
	defer in.Close()
	out, err := os.Create(destPath)
	if err != nil {
		return nil, fmt.Errorf("create dest: %w", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return nil, fmt.Errorf("copy: %w", err)
	}

	fi, _ := os.Stat(destPath)
	sizeBytes := int64(0)
	if fi != nil {
		sizeBytes = fi.Size()
	}

	nullLabel := interface{}(nil)
	if label != "" {
		nullLabel = label
	}
	if _, err := a.db.Exec(`
		INSERT INTO vault_images (id, seq, path, filename, size_bytes, source, label)
		VALUES (?, ?, ?, ?, ?, 'upload', ?)`,
		id, seq, destPath, destFilename, sizeBytes, nullLabel,
	); err != nil {
		return nil, fmt.Errorf("insert: %w", err)
	}
	return a.GetVaultImage(id)
}

func (a *App) DeleteVaultImage(id string) error {
	if a.db == nil {
		return fmt.Errorf("database not available")
	}
	var path string
	if err := a.db.QueryRow(`SELECT path FROM vault_images WHERE id = ?`, id).Scan(&path); err != nil {
		return fmt.Errorf("vault image %q not found: %w", id, err)
	}
	if _, err := a.db.Exec(`DELETE FROM vault_images WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete record: %w", err)
	}
	_ = os.Remove(path) // best-effort
	return nil
}

func (a *App) SearchVaultImages(query string) ([]map[string]interface{}, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not available")
	}
	q := "%" + query + "%"
	rows, err := a.db.Query(`
		SELECT id, seq, path, filename, size_bytes, source,
		       COALESCE(workflow_id,'') as workflow_id,
		       COALESCE(execution_id,'') as execution_id,
		       COALESCE(label,'') as label, created_at
		FROM vault_images
		WHERE label LIKE ? OR filename LIKE ? OR source LIKE ? OR workflow_id LIKE ?
		ORDER BY seq DESC LIMIT 100`, q, q, q, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]interface{}
	for rows.Next() {
		var id, path, filename, source, workflowID, executionID, label, createdAt string
		var seq, sizeBytes int
		if err := rows.Scan(&id, &seq, &path, &filename, &sizeBytes, &source, &workflowID, &executionID, &label, &createdAt); err != nil {
			continue
		}
		out = append(out, map[string]interface{}{
			"id": id, "seq": seq, "path": path, "filename": filename,
			"size_bytes": sizeBytes, "source": source,
			"workflow_id": workflowID, "execution_id": executionID,
			"label": label, "created_at": createdAt,
			"url": "/vault-image/" + filename,
		})
	}
	if out == nil {
		out = []map[string]interface{}{}
	}
	return out, nil
}

func (a *App) GetVaultStats() (map[string]interface{}, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not available")
	}
	var count int
	var totalBytes int64
	err := a.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(size_bytes),0) FROM vault_images`).
		Scan(&count, &totalBytes)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"count": count, "total_bytes": totalBytes}, nil
}
```

Add `"io"` to the import block in `wails-app/app.go` (it may already be there; if not, add it).

- [ ] **Build check**

```bash
cd /Users/morteza/Desktop/monoes/monoes-agent/newmonoes/wails-app && go build ./... 2>&1
```
Expected: no errors.

- [ ] **Commit**

```bash
git add wails-app/app.go
git commit -m "feat: add vault methods to Wails app.go (GetVaultImages, AddVaultImage, DeleteVaultImage, SearchVaultImages, GetVaultStats)"
```

---

## Task 7: Wails asset handler for /vault-image/

**Files:**
- Modify: `wails-app/main.go`

- [ ] **Add the vault image handler**

In `wails-app/main.go`, add this function before `main()`:

```go
// vaultImageHandler serves files from ~/.monoes/vault/ at /vault-image/<filename>.
func vaultImageHandler() http.Handler {
	vaultDir := filepath.Join(os.Getenv("HOME"), ".monoes", "vault")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/vault-image/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Use filepath.Base to prevent path traversal.
		name := filepath.Base(r.URL.Path)
		http.ServeFile(w, r, filepath.Join(vaultDir, name))
	})
}
```

- [ ] **Wire handler into AssetServer options** in `main()`:

Find:
```go
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
```

Replace with:
```go
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: vaultImageHandler(),
		},
```

- [ ] **Add missing imports to main.go** if not present: `"net/http"`, `"os"`, `"path/filepath"`, `"strings"`.

- [ ] **Build check**

```bash
cd /Users/morteza/Desktop/monoes/monoes-agent/newmonoes/wails-app && go build ./... 2>&1
```
Expected: no errors.

- [ ] **Commit**

```bash
git add wails-app/main.go
git commit -m "feat: serve vault images via /vault-image/ asset handler"
```

---

## Task 8: Wails JS bindings

**Files:**
- Modify: `wails-app/frontend/src/wailsjs/wailsjs/go/main/App.js`
- Modify: `wails-app/frontend/src/wailsjs/wailsjs/go/main/App.d.ts`

- [ ] **Add JS bindings to App.js** — append to the end of the file:

```js
export function GetVaultImages(arg1) {
  return window['go']['main']['App']['GetVaultImages'](arg1);
}

export function GetVaultImage(arg1) {
  return window['go']['main']['App']['GetVaultImage'](arg1);
}

export function AddVaultImage(arg1, arg2) {
  return window['go']['main']['App']['AddVaultImage'](arg1, arg2);
}

export function DeleteVaultImage(arg1) {
  return window['go']['main']['App']['DeleteVaultImage'](arg1);
}

export function SearchVaultImages(arg1) {
  return window['go']['main']['App']['SearchVaultImages'](arg1);
}

export function GetVaultStats() {
  return window['go']['main']['App']['GetVaultStats']();
}
```

- [ ] **Add TypeScript defs to App.d.ts** — append to the end of the file:

```typescript
export function GetVaultImages(arg1: number): Promise<Array<Record<string, any>>>;
export function GetVaultImage(arg1: string): Promise<Record<string, any>>;
export function AddVaultImage(arg1: string, arg2: string): Promise<Record<string, any>>;
export function DeleteVaultImage(arg1: string): Promise<void>;
export function SearchVaultImages(arg1: string): Promise<Array<Record<string, any>>>;
export function GetVaultStats(): Promise<Record<string, any>>;
```

- [ ] **Commit**

```bash
git add wails-app/frontend/src/wailsjs/wailsjs/go/main/App.js wails-app/frontend/src/wailsjs/wailsjs/go/main/App.d.ts
git commit -m "feat: add vault Wails JS bindings"
```

---

## Task 9: ImageDetailModal component

**Files:**
- Create: `wails-app/frontend/src/components/ImageDetailModal.jsx`

- [ ] **Create the modal component**

```jsx
import { useEffect, useRef } from 'react'
import { X, Copy, Trash2, Edit3 } from 'lucide-react'
import * as WailsApp from '../wailsjs/wailsjs/go/main/App'

export default function ImageDetailModal({ image, onClose, onDelete, onRename }) {
  const overlayRef = useRef(null)

  useEffect(() => {
    const handler = (e) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [onClose])

  if (!image) return null

  const handleDelete = async () => {
    if (!window.confirm(`Delete ${image.id}? This cannot be undone.`)) return
    try {
      await WailsApp.DeleteVaultImage(image.id)
      onDelete(image.id)
      onClose()
    } catch (e) {
      alert('Delete failed: ' + e)
    }
  }

  const copyRef = () => {
    navigator.clipboard.writeText('@' + image.id).catch(() => {})
  }

  const fmtBytes = (b) => {
    if (b < 1024) return b + ' B'
    if (b < 1024 * 1024) return (b / 1024).toFixed(1) + ' KB'
    return (b / 1024 / 1024).toFixed(1) + ' MB'
  }

  const fmtDate = (s) => {
    if (!s) return '—'
    const d = new Date(s.includes('T') ? s : s.replace(' ', 'T'))
    return isNaN(d) ? s : d.toLocaleDateString()
  }

  return (
    <div
      ref={overlayRef}
      onClick={(e) => { if (e.target === overlayRef.current) onClose() }}
      style={{
        position: 'fixed', inset: 0, zIndex: 1000,
        background: 'rgba(0,0,0,0.75)',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
      }}
    >
      <div style={{
        background: '#0d1a26', border: '1px solid #1e3a4f', borderRadius: 12,
        padding: 20, width: 420, maxWidth: '90vw',
        display: 'flex', flexDirection: 'column', gap: 14,
        boxShadow: '0 20px 60px rgba(0,0,0,0.6)',
      }}>
        {/* Header */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 13, color: '#00b4d8' }}>
            {image.id}{image.label ? ` · ${image.label}` : ''}
          </span>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#475569', padding: 2 }}>
            <X size={16} />
          </button>
        </div>

        {/* Image preview */}
        <div style={{
          background: '#060b11', borderRadius: 8, overflow: 'hidden',
          border: '1px solid #1e3a4f', maxHeight: 280,
          display: 'flex', alignItems: 'center', justifyContent: 'center',
        }}>
          <img
            src={image.url}
            alt={image.label || image.id}
            style={{ maxWidth: '100%', maxHeight: 280, objectFit: 'contain', display: 'block' }}
            onError={(e) => { e.target.style.display = 'none' }}
          />
        </div>

        {/* Metadata */}
        <div style={{ display: 'flex', gap: 16, fontFamily: 'var(--font-mono)', fontSize: 10, color: '#475569', flexWrap: 'wrap' }}>
          <span style={{ color: '#a78bfa' }}>{image.source}</span>
          <span>{fmtBytes(image.size_bytes)}</span>
          <span>{fmtDate(image.created_at)}</span>
          {image.workflow_id && <span style={{ color: '#64748b' }}>{image.workflow_id}</span>}
        </div>

        {/* Actions */}
        <div style={{ display: 'flex', gap: 8 }}>
          <button
            onClick={copyRef}
            style={{
              flex: 1, background: '#0a1829', border: '1px solid rgba(0,180,216,0.3)',
              borderRadius: 6, padding: '7px 12px', color: '#00b4d8',
              fontFamily: 'var(--font-mono)', fontSize: 11, cursor: 'pointer',
              display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 5,
            }}
          >
            <Copy size={12} /> Copy @{image.id}
          </button>
          {onRename && (
            <button
              onClick={() => onRename(image)}
              style={{
                background: '#0a1829', border: '1px solid #1e3a4f',
                borderRadius: 6, padding: '7px 10px', color: '#475569',
                cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 4,
                fontFamily: 'var(--font-mono)', fontSize: 11,
              }}
            >
              <Edit3 size={12} /> Rename
            </button>
          )}
          <button
            onClick={handleDelete}
            style={{
              background: 'rgba(239,68,68,0.08)', border: '1px solid rgba(239,68,68,0.3)',
              borderRadius: 6, padding: '7px 10px', color: '#ef4444',
              cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 4,
              fontFamily: 'var(--font-mono)', fontSize: 11,
            }}
          >
            <Trash2 size={12} /> Delete
          </button>
        </div>
      </div>
    </div>
  )
}
```

- [ ] **Commit**

```bash
git add wails-app/frontend/src/components/ImageDetailModal.jsx
git commit -m "feat: add ImageDetailModal component"
```

---

## Task 10: ImagePickerModal component

**Files:**
- Create: `wails-app/frontend/src/components/ImagePickerModal.jsx`

- [ ] **Create the picker component**

```jsx
import { useState, useEffect, useRef } from 'react'
import { X, Search } from 'lucide-react'
import * as WailsApp from '../wailsjs/wailsjs/go/main/App'

export default function ImagePickerModal({ onSelect, onClose }) {
  const [images, setImages] = useState([])
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState(null)
  const searchRef = useRef(null)

  useEffect(() => {
    WailsApp.GetVaultImages(100).then(setImages).catch(() => {})
    setTimeout(() => searchRef.current?.focus(), 50)
  }, [])

  useEffect(() => {
    const handler = (e) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [onClose])

  const filtered = query
    ? images.filter(img =>
        img.id.includes(query) ||
        (img.label || '').toLowerCase().includes(query.toLowerCase()) ||
        img.filename.toLowerCase().includes(query.toLowerCase())
      )
    : images

  const fmtBytes = (b) => b < 1024 * 1024 ? (b / 1024).toFixed(0) + ' KB' : (b / 1024 / 1024).toFixed(1) + ' MB'

  return (
    <div
      onClick={(e) => { if (e.target === e.currentTarget) onClose() }}
      style={{
        position: 'fixed', inset: 0, zIndex: 1100,
        background: 'rgba(0,0,0,0.75)',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
      }}
    >
      <div style={{
        background: '#0d1a26', border: '1px solid #1e3a4f', borderRadius: 10,
        padding: 16, width: 360, maxWidth: '90vw',
        display: 'flex', flexDirection: 'column', gap: 10,
        boxShadow: '0 20px 60px rgba(0,0,0,0.6)',
      }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: '#e2e8f0' }}>Pick an image</span>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#475569' }}>
            <X size={14} />
          </button>
        </div>

        <div style={{ position: 'relative' }}>
          <Search size={12} style={{ position: 'absolute', left: 8, top: '50%', transform: 'translateY(-50%)', color: '#475569' }} />
          <input
            ref={searchRef}
            value={query}
            onChange={e => setQuery(e.target.value)}
            placeholder="search…"
            style={{
              width: '100%', background: '#060b11', border: '1px solid #1e3a4f',
              borderRadius: 5, padding: '5px 8px 5px 26px', color: '#e2e8f0',
              fontFamily: 'var(--font-mono)', fontSize: 11, boxSizing: 'border-box',
            }}
          />
        </div>

        <div style={{ maxHeight: 260, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 3 }}>
          {filtered.length === 0 && (
            <div style={{ color: '#475569', fontFamily: 'var(--font-mono)', fontSize: 11, padding: '12px 0', textAlign: 'center' }}>
              No images found
            </div>
          )}
          {filtered.map(img => (
            <div
              key={img.id}
              onClick={() => setSelected(img.id === selected ? null : img.id)}
              style={{
                display: 'flex', alignItems: 'center', gap: 8,
                background: selected === img.id ? '#1e3a2f' : '#111827',
                border: `1px solid ${selected === img.id ? 'rgba(16,185,129,0.3)' : '#1e3a4f'}`,
                borderRadius: 5, padding: '6px 8px', cursor: 'pointer',
              }}
            >
              <div style={{ width: 32, height: 32, borderRadius: 3, overflow: 'hidden', flexShrink: 0, background: '#060b11' }}>
                <img src={img.url} alt="" style={{ width: '100%', height: '100%', objectFit: 'cover' }}
                  onError={e => { e.target.style.display = 'none' }} />
              </div>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: selected === img.id ? '#10b981' : '#00b4d8' }}>{img.id}</div>
                <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: '#475569', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {img.label || img.filename} · {fmtBytes(img.size_bytes)}
                </div>
              </div>
            </div>
          ))}
        </div>

        <div style={{ display: 'flex', gap: 8 }}>
          <button
            onClick={() => { if (selected) { onSelect('@' + selected); onClose() } }}
            disabled={!selected}
            style={{
              flex: 1, background: selected ? 'rgba(0,180,216,0.1)' : '#060b11',
              border: `1px solid ${selected ? 'rgba(0,180,216,0.4)' : '#1e3a4f'}`,
              borderRadius: 6, padding: '7px 12px',
              color: selected ? '#00b4d8' : '#334155',
              fontFamily: 'var(--font-mono)', fontSize: 11, cursor: selected ? 'pointer' : 'not-allowed',
            }}
          >
            Select
          </button>
          <button
            onClick={onClose}
            style={{
              background: '#060b11', border: '1px solid #1e3a4f',
              borderRadius: 6, padding: '7px 14px', color: '#475569',
              fontFamily: 'var(--font-mono)', fontSize: 11, cursor: 'pointer',
            }}
          >
            Cancel
          </button>
        </div>
      </div>
    </div>
  )
}
```

- [ ] **Commit**

```bash
git add wails-app/frontend/src/components/ImagePickerModal.jsx
git commit -m "feat: add ImagePickerModal component"
```

---

## Task 11: ImageVault.jsx main page

**Files:**
- Create: `wails-app/frontend/src/pages/ImageVault.jsx`

- [ ] **Create the page**

```jsx
import { useState, useEffect, useCallback, useRef } from 'react'
import { Trash2, Plus, Search, Image as ImageIcon } from 'lucide-react'
import * as WailsApp from '../wailsjs/wailsjs/go/main/App'
import ImageDetailModal from '../components/ImageDetailModal'

const fmtBytes = (b) => {
  if (!b) return '0 B'
  if (b < 1024) return b + ' B'
  if (b < 1024 * 1024) return (b / 1024).toFixed(1) + ' KB'
  return (b / 1024 / 1024).toFixed(1) + ' MB'
}

const fmtDate = (s) => {
  if (!s) return '—'
  const d = new Date(s.includes('T') ? s : s.replace(' ', 'T') + 'Z')
  if (isNaN(d)) return s
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

const SOURCE_COLORS = {
  gemini: { bg: 'rgba(124,58,237,0.15)', border: 'rgba(124,58,237,0.3)', color: '#a78bfa' },
  upload: { bg: 'rgba(16,185,129,0.1)', border: 'rgba(16,185,129,0.25)', color: '#34d399' },
  huggingface: { bg: 'rgba(0,180,216,0.1)', border: 'rgba(0,180,216,0.25)', color: '#00b4d8' },
}
const sourceBadge = (source) => {
  const s = SOURCE_COLORS[source] || { bg: '#1a2332', border: '#334', color: '#64748b' }
  return (
    <span style={{
      background: s.bg, border: `1px solid ${s.border}`, borderRadius: 3,
      padding: '1px 6px', fontFamily: 'var(--font-mono)', fontSize: 9, color: s.color,
    }}>{source}</span>
  )
}

export default function ImageVault() {
  const [images, setImages] = useState([])
  const [stats, setStats] = useState(null)
  const [search, setSearch] = useState('')
  const [dragging, setDragging] = useState(false)
  const [detail, setDetail] = useState(null) // image object for modal
  const [error, setError] = useState(null)
  const fileInputRef = useRef(null)
  const pageRef = useRef(null)

  const load = useCallback(async () => {
    try {
      const [imgs, st] = await Promise.all([
        WailsApp.GetVaultImages(200),
        WailsApp.GetVaultStats(),
      ])
      setImages(imgs || [])
      setStats(st)
    } catch (e) {
      setError('Failed to load vault: ' + e)
    }
  }, [])

  useEffect(() => { load() }, [load])

  const filtered = search
    ? images.filter(img =>
        img.id.includes(search) ||
        (img.label || '').toLowerCase().includes(search.toLowerCase()) ||
        img.filename.toLowerCase().includes(search.toLowerCase()) ||
        img.source.includes(search)
      )
    : images

  const handleFileAdd = async (files) => {
    setError(null)
    for (const file of files) {
      try {
        await WailsApp.AddVaultImage(file.path || file.name, '')
      } catch (e) {
        setError('Upload failed: ' + e)
      }
    }
    load()
  }

  const handleDrop = (e) => {
    e.preventDefault()
    setDragging(false)
    const files = Array.from(e.dataTransfer.files).filter(f => f.type.startsWith('image/'))
    if (files.length) handleFileAdd(files)
  }

  const handleDelete = (id) => {
    setImages(prev => prev.filter(img => img.id !== id))
    load() // refresh stats
  }

  return (
    <div
      ref={pageRef}
      onDragOver={(e) => { e.preventDefault(); setDragging(true) }}
      onDragLeave={(e) => { if (!pageRef.current?.contains(e.relatedTarget)) setDragging(false) }}
      onDrop={handleDrop}
      style={{
        display: 'flex', flexDirection: 'column', height: '100%', overflow: 'hidden',
        outline: dragging ? '2px dashed #00b4d8' : 'none',
        outlineOffset: -3,
      }}
    >
      {/* Header */}
      <div style={{
        padding: '14px 20px 10px', borderBottom: '1px solid #0d1a26',
        display: 'flex', alignItems: 'center', gap: 12,
      }}>
        <div>
          <div style={{ color: '#e2e8f0', fontSize: 16, fontWeight: 600 }}>Image Vault</div>
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: '#475569' }}>
            {stats ? `${stats.count} images · ${fmtBytes(stats.total_bytes)}` : 'loading…'}
          </div>
        </div>
        <div style={{ flex: 1 }} />
        <div style={{ position: 'relative' }}>
          <Search size={11} style={{ position: 'absolute', left: 8, top: '50%', transform: 'translateY(-50%)', color: '#475569' }} />
          <input
            value={search}
            onChange={e => setSearch(e.target.value)}
            placeholder="search…"
            style={{
              background: '#0d1a26', border: '1px solid #1e3a4f', borderRadius: 5,
              padding: '5px 8px 5px 26px', color: '#e2e8f0',
              fontFamily: 'var(--font-mono)', fontSize: 11, width: 160,
            }}
          />
        </div>
        <button
          onClick={() => fileInputRef.current?.click()}
          style={{
            background: 'rgba(0,180,216,0.1)', border: '1px solid rgba(0,180,216,0.3)',
            borderRadius: 6, padding: '6px 12px', color: '#00b4d8',
            fontFamily: 'var(--font-mono)', fontSize: 11, cursor: 'pointer',
            display: 'flex', alignItems: 'center', gap: 5,
          }}
        >
          <Plus size={12} /> Add Image
        </button>
        <input
          ref={fileInputRef}
          type="file"
          accept="image/*"
          multiple
          style={{ display: 'none' }}
          onChange={e => { handleFileAdd(Array.from(e.target.files)); e.target.value = '' }}
        />
      </div>

      {/* Drop hint */}
      <div style={{
        padding: '4px 20px', background: dragging ? 'rgba(0,180,216,0.06)' : '#060b11',
        borderBottom: '1px solid #0a1520',
        fontFamily: 'var(--font-mono)', fontSize: 9,
        color: dragging ? '#00b4d8' : '#1e3a4f', textAlign: 'center',
        transition: 'all 0.15s',
      }}>
        {dragging ? 'Drop to add to vault' : 'Drop images anywhere to add them to the vault'}
      </div>

      {/* Error */}
      {error && (
        <div style={{ margin: '8px 20px', background: 'rgba(239,68,68,0.08)', border: '1px solid rgba(239,68,68,0.2)', borderRadius: 5, padding: '7px 10px', fontFamily: 'var(--font-mono)', fontSize: 11, color: '#fca5a5' }}>
          {error}
        </div>
      )}

      {/* Column headers */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 0,
        padding: '5px 20px', borderBottom: '1px solid #0a1520',
        fontFamily: 'var(--font-mono)', fontSize: 9, color: '#334155',
        letterSpacing: '1px', textTransform: 'uppercase',
      }}>
        <div style={{ width: 44 }} />
        <div style={{ width: 72 }}>ID</div>
        <div style={{ flex: 1 }}>Label / Filename</div>
        <div style={{ width: 80 }}>Source</div>
        <div style={{ width: 120, overflow: 'hidden' }}>Workflow</div>
        <div style={{ width: 56 }}>Size</div>
        <div style={{ width: 56 }}>Date</div>
        <div style={{ width: 28 }} />
      </div>

      {/* Rows */}
      <div style={{ flex: 1, overflowY: 'auto' }}>
        {filtered.length === 0 && (
          <div style={{
            display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center',
            height: 200, gap: 12, color: '#334155',
          }}>
            <ImageIcon size={32} style={{ opacity: 0.3 }} />
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: 12 }}>
              {search ? 'No images match your search' : 'No images yet — run a workflow or drop images here'}
            </div>
          </div>
        )}
        {filtered.map(img => (
          <div
            key={img.id}
            onClick={() => setDetail(img)}
            style={{
              display: 'flex', alignItems: 'center', gap: 0,
              padding: '6px 20px', borderBottom: '1px solid #0a1520',
              cursor: 'pointer',
              transition: 'background 0.1s',
            }}
            onMouseEnter={e => e.currentTarget.style.background = '#0d1f35'}
            onMouseLeave={e => e.currentTarget.style.background = ''}
          >
            <div style={{ width: 44, paddingRight: 8 }}>
              <div style={{
                width: 36, height: 36, borderRadius: 4, overflow: 'hidden',
                background: '#111827', border: '1px solid #1e3a4f', flexShrink: 0,
              }}>
                <img
                  src={img.url}
                  alt=""
                  style={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }}
                  onError={e => { e.target.style.display = 'none' }}
                />
              </div>
            </div>
            <div style={{ width: 72, fontFamily: 'var(--font-mono)', fontSize: 11, color: '#00b4d8', fontWeight: 600 }}>{img.id}</div>
            <div style={{ flex: 1, minWidth: 0, paddingRight: 10 }}>
              <div style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: '#94a3b8', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {img.label || '—'}
              </div>
              <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: '#334155', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {img.filename}
              </div>
            </div>
            <div style={{ width: 80 }}>{sourceBadge(img.source)}</div>
            <div style={{ width: 120, fontFamily: 'var(--font-mono)', fontSize: 10, color: '#475569', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', paddingRight: 8 }}>
              {img.workflow_id || '—'}
            </div>
            <div style={{ width: 56, fontFamily: 'var(--font-mono)', fontSize: 10, color: '#475569' }}>{fmtBytes(img.size_bytes)}</div>
            <div style={{ width: 56, fontFamily: 'var(--font-mono)', fontSize: 10, color: '#475569' }}>{fmtDate(img.created_at)}</div>
            <div style={{ width: 28 }}>
              <button
                onClick={async (e) => {
                  e.stopPropagation()
                  try {
                    await WailsApp.DeleteVaultImage(img.id)
                    setImages(prev => prev.filter(i => i.id !== img.id))
                    load()
                  } catch (err) { setError('Delete failed: ' + err) }
                }}
                style={{
                  background: 'none', border: 'none', cursor: 'pointer',
                  color: '#4b5563', padding: 4, borderRadius: 3,
                  display: 'flex', alignItems: 'center', justifyContent: 'center',
                }}
                onMouseEnter={e => e.currentTarget.style.color = '#ef4444'}
                onMouseLeave={e => e.currentTarget.style.color = '#4b5563'}
              >
                <Trash2 size={13} />
              </button>
            </div>
          </div>
        ))}
      </div>

      {/* Detail modal */}
      {detail && (
        <ImageDetailModal
          image={detail}
          onClose={() => setDetail(null)}
          onDelete={handleDelete}
        />
      )}
    </div>
  )
}
```

- [ ] **Commit**

```bash
git add wails-app/frontend/src/pages/ImageVault.jsx
git commit -m "feat: add ImageVault page"
```

---

## Task 12: Sidebar nav + App.jsx routing

**Files:**
- Modify: `wails-app/frontend/src/components/Sidebar.jsx`
- Modify: `wails-app/frontend/src/App.jsx`

- [ ] **Add Images nav item to Sidebar.jsx**

Add import at the top: `Image` from lucide-react (add to the existing import list):

```js
import {
  LayoutDashboard, Users,
  Terminal, ChevronRight, PlayCircle, Link2, Brain, Settings, Image
} from 'lucide-react'
```

In `NAV_ITEMS`, add after the `connections` entry:

```js
  { id: 'vault',       label: 'Images',      icon: Image,           section: 'DATA' },
```

- [ ] **Add vault page route in App.jsx**

Add import at the top:
```js
import ImageVault from './pages/ImageVault.jsx'
```

In the `pages` object (where `people`, `connections`, etc. are listed), add:
```js
    vault: <ImageVault />,
```

- [ ] **Build frontend to verify no import errors**

```bash
cd /Users/morteza/Desktop/monoes/monoes-agent/newmonoes/wails-app/frontend && npm run build 2>&1 | tail -15
```
Expected: `✓ built in ...ms`

- [ ] **Commit**

```bash
git add wails-app/frontend/src/components/Sidebar.jsx wails-app/frontend/src/App.jsx
git commit -m "feat: add Image Vault to sidebar nav and App routing"
```

---

## Task 13: @-autocomplete + picker in NodeRunner

**Files:**
- Modify: `wails-app/frontend/src/pages/NodeRunner.jsx`

- [ ] **Add vault image autocomplete state to Inspector**

In the `Inspector` function (`function Inspector({ node, onConfigChange, onClose, onNavigate })`), add state after the existing `useState` calls:

```jsx
  const [vaultImages, setVaultImages] = useState([])
  const [atAC, setAtAC] = useState({ open: false, query: '', fieldKey: null }) // @ autocomplete
  const [pickerField, setPickerField] = useState(null) // field key for picker modal
```

Add an effect to load vault images when inspector opens:
```jsx
  useEffect(() => {
    WailsApp.GetVaultImages(50).then(imgs => setVaultImages(imgs || [])).catch(() => {})
  }, [node?.id])
```

Add import at the top of NodeRunner.jsx (with other imports):
```jsx
import ImagePickerModal from '../components/ImagePickerModal'
```

And add `* as WailsApp` to existing wails imports:
```jsx
import * as WailsApp from '../wailsjs/wailsjs/go/main/App'
```

- [ ] **Replace the default text input with @-aware input**

Find the `else` block for text fields (around line 834-843) in the `Inspector` function:

```jsx
                } else {
                  // 'text' and any unknown types
                  inputEl = (
                    <input
                      type="text"
                      value={val}
                      onChange={onChange}
                      style={inputStyle}
                    />
                  )
                }
```

Replace with:

```jsx
                } else {
                  // 'text' and any unknown types — with @-autocomplete and picker button
                  const acMatches = atAC.open && atAC.fieldKey === f.key
                    ? vaultImages.filter(img =>
                        img.id.includes(atAC.query) ||
                        (img.label || '').toLowerCase().includes(atAC.query.toLowerCase())
                      ).slice(0, 8)
                    : []

                  const handleAtChange = (e) => {
                    const v = e.target.value
                    onConfigChange(node.id, f.key, v)
                    const lastAt = v.lastIndexOf('@')
                    if (lastAt !== -1) {
                      const afterAt = v.slice(lastAt + 1)
                      if (!afterAt.includes(' ')) {
                        setAtAC({ open: true, query: afterAt, fieldKey: f.key })
                        return
                      }
                    }
                    setAtAC({ open: false, query: '', fieldKey: null })
                  }

                  inputEl = (
                    <div style={{ position: 'relative' }}>
                      <div style={{ display: 'flex', gap: 4 }}>
                        <input
                          type="text"
                          value={val}
                          onChange={handleAtChange}
                          onBlur={() => setTimeout(() => setAtAC({ open: false, query: '', fieldKey: null }), 150)}
                          style={{ ...inputStyle, flex: 1 }}
                        />
                        <button
                          title="Pick from Image Vault"
                          onClick={() => setPickerField(f.key)}
                          style={{
                            background: '#0d1a26', border: '1px solid #1e3a4f', borderRadius: 4,
                            padding: '0 7px', cursor: 'pointer', color: '#475569', flexShrink: 0,
                            display: 'flex', alignItems: 'center',
                          }}
                          onMouseEnter={e => e.currentTarget.style.color = '#00b4d8'}
                          onMouseLeave={e => e.currentTarget.style.color = '#475569'}
                        >
                          🖼
                        </button>
                      </div>
                      {atAC.open && atAC.fieldKey === f.key && acMatches.length > 0 && (
                        <div style={{
                          position: 'absolute', top: '100%', left: 0, right: 0, zIndex: 200,
                          background: '#0d1a26', border: '1px solid #1e3a4f', borderRadius: 5,
                          boxShadow: '0 4px 16px rgba(0,0,0,0.5)', overflow: 'hidden', marginTop: 2,
                        }}>
                          <div style={{ padding: '3px 8px', background: '#111827', fontFamily: 'var(--font-mono)', fontSize: 8, color: '#334155', textTransform: 'uppercase', letterSpacing: 1 }}>
                            Vault Images
                          </div>
                          {acMatches.map(img => (
                            <div
                              key={img.id}
                              onMouseDown={() => {
                                const v = String(val)
                                const lastAt = v.lastIndexOf('@')
                                const newVal = v.slice(0, lastAt) + '@' + img.id + ' '
                                onConfigChange(node.id, f.key, newVal)
                                setAtAC({ open: false, query: '', fieldKey: null })
                              }}
                              style={{
                                display: 'flex', alignItems: 'center', gap: 7,
                                padding: '5px 8px', cursor: 'pointer',
                              }}
                              onMouseEnter={e => e.currentTarget.style.background = '#0a1829'}
                              onMouseLeave={e => e.currentTarget.style.background = ''}
                            >
                              <div style={{ width: 20, height: 20, borderRadius: 2, overflow: 'hidden', background: '#060b11', flexShrink: 0 }}>
                                <img src={img.url} alt="" style={{ width: '100%', height: '100%', objectFit: 'cover' }} onError={e => { e.target.style.display = 'none' }} />
                              </div>
                              <div>
                                <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: '#00b4d8' }}>@{img.id}</div>
                                <div style={{ fontFamily: 'var(--font-mono)', fontSize: 8, color: '#475569' }}>{img.label || img.filename}</div>
                              </div>
                            </div>
                          ))}
                        </div>
                      )}
                    </div>
                  )
                }
```

- [ ] **Add picker modal rendering** — at the bottom of `Inspector`'s return JSX, before the closing `</div>`:

Find the closing of the inspector return and add:
```jsx
      {pickerField && (
        <ImagePickerModal
          onSelect={(ref) => {
            onConfigChange(node.id, pickerField, ref)
            setPickerField(null)
          }}
          onClose={() => setPickerField(null)}
        />
      )}
```

- [ ] **Build frontend**

```bash
cd /Users/morteza/Desktop/monoes/monoes-agent/newmonoes/wails-app/frontend && npm run build 2>&1 | tail -10
```
Expected: `✓ built in ...ms`

- [ ] **Commit**

```bash
git add wails-app/frontend/src/pages/NodeRunner.jsx
git commit -m "feat: add @-autocomplete and image picker to NodeRunner config fields"
```

---

## Task 14: Rebuild CLI + full build verification

- [ ] **Rebuild CLI binary**

```bash
cd /Users/morteza/Desktop/monoes/monoes-agent/newmonoes && go build -o ./cmd/monoes/monoes ./cmd/monoes/ 2>&1 && echo "CLI OK"
```
Expected: `CLI OK`

- [ ] **Rebuild Wails app**

```bash
cd /Users/morteza/Desktop/monoes/monoes-agent/newmonoes/wails-app/frontend && npm run build 2>&1 | tail -5
```
Expected: `✓ built in ...ms`

- [ ] **Run a workflow that generates images and verify vault registration**

```bash
cd /Users/morteza/Desktop/monoes/monoes-agent/newmonoes && ./cmd/monoes/monoes workflow run instagram-daily-post-gemini-v1 2>&1 | tail -3
```

Then check the vault:
```bash
sqlite3 ~/.monoes/monoes.db "SELECT id, filename, source, created_at FROM vault_images ORDER BY seq DESC LIMIT 5;"
```
Expected: rows with `source = "gemini"` and filenames like `img-001.png`.

- [ ] **Verify vault image file exists on disk**

```bash
ls -la ~/.monoes/vault/
```
Expected: files like `img-001.png`.

- [ ] **Final commit**

```bash
cd /Users/morteza/Desktop/monoes/monoes-agent/newmonoes
git add -A
git commit -m "feat: complete Image Vault — auto-registration, UI, @-refs, picker"
```
