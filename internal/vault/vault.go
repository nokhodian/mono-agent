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
	if src == "" {
		return "", fmt.Errorf("vault.Register: src path is empty")
	}
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return "", fmt.Errorf("vault.Register: invalid src path: %w", err)
	}
	src = absSrc

	if err := EnsureVaultDir(); err != nil {
		return "", fmt.Errorf("vault.Register: ensure vault dir: %w", err)
	}

	// Begin an exclusive transaction to prevent TOCTOU in seq allocation.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("vault.Register: begin tx: %w", err)
	}
	defer tx.Rollback()

	var seq int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq), 0) + 1 FROM vault_images`).Scan(&seq); err != nil {
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

	_, err = tx.ExecContext(ctx, `
		INSERT INTO vault_images (id, seq, path, filename, size_bytes, source, workflow_id, execution_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, seq, destPath, destFilename, fi.Size(), source,
		nullStr(workflowID), nullStr(executionID),
	)
	if err != nil {
		os.Remove(destPath) // best-effort cleanup
		return "", fmt.Errorf("vault.Register: insert: %w", err)
	}
	if err := tx.Commit(); err != nil {
		os.Remove(destPath)
		return "", fmt.Errorf("vault.Register: commit: %w", err)
	}
	return id, nil
}

// Resolve turns "@img-001" into the absolute file path stored in the DB.
// Returns an error if the image is not found.
func Resolve(ctx context.Context, db *sql.DB, ref string) (string, error) {
	if !strings.HasPrefix(ref, "@") {
		return ref, nil
	}
	id := strings.TrimPrefix(ref, "@")
	var path string
	err := db.QueryRowContext(ctx, `SELECT path FROM vault_images WHERE id = ?`, id).Scan(&path)
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
		resolved, err := Resolve(context.Background(), db, s)
		if err != nil {
			// Non-fatal: leave original ref, emit warning.
			fmt.Fprintf(os.Stderr, "vault: warning: %v\n", err)
			continue
		}
		config[k] = resolved
	}
	return nil
}

func copyFile(src, dst string) (retErr error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		cerr := out.Close()
		if retErr == nil {
			retErr = cerr
		}
	}()
	_, err = io.Copy(out, in)
	return err
}
