package storage

import (
	"database/sql"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/monoes/monoes-agent/data"

	_ "modernc.org/sqlite"
)

// Database wraps a *sql.DB connection to a SQLite database and provides
// migration and maintenance helpers.
type Database struct {
	DB     *sql.DB
	dbPath string
}

// NewDatabase opens (or creates) a SQLite database at dbPath and configures it
// for production use: WAL journal mode, a busy timeout of 5 seconds, and
// foreign key enforcement.
func NewDatabase(dbPath string) (*Database, error) {
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, fmt.Errorf("resolving database path: %w", err)
	}

	db, err := sql.Open("sqlite", absPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Verify the connection is usable.
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	// Configure SQLite pragmas for reliability and performance.
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
		"PRAGMA synchronous=NORMAL",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("executing %q: %w", p, err)
		}
	}

	return &Database{DB: db, dbPath: absPath}, nil
}

// ApplyMigrations reads embedded SQL migration files from data/migrations/ and
// applies any that have not yet been recorded in the schema_migrations table.
// Migrations are applied in filename order (e.g. 001_initial.sql before
// 002_add_feature.sql).
func (d *Database) ApplyMigrations() error {
	// Ensure the schema_migrations table itself exists before we query it.
	_, err := d.DB.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("ensuring schema_migrations table: %w", err)
	}

	// Discover migration files from the embedded filesystem.
	entries, err := data.MigrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("reading embedded migrations dir: %w", err)
	}

	// Filter to .sql files and sort by name.
	type migration struct {
		version  int
		filename string
	}
	var migrations []migration
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}
		// Extract version number from the filename prefix (e.g. "001" from "001_initial.sql").
		parts := strings.SplitN(name, "_", 2)
		if len(parts) < 2 {
			continue
		}
		ver, err := strconv.Atoi(parts[0])
		if err != nil {
			log.Printf("skipping migration file with non-numeric prefix: %s", name)
			continue
		}
		migrations = append(migrations, migration{version: ver, filename: name})
	}
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	for _, m := range migrations {
		// Check if this version has already been applied.
		var exists int
		err := d.DB.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", m.version).Scan(&exists)
		if err != nil {
			return fmt.Errorf("checking migration version %d: %w", m.version, err)
		}
		if exists > 0 {
			continue
		}

		// Read the migration SQL.
		content, err := data.MigrationsFS.ReadFile("migrations/" + m.filename)
		if err != nil {
			return fmt.Errorf("reading migration file %s: %w", m.filename, err)
		}

		// Execute the migration within a transaction.
		tx, err := d.DB.Begin()
		if err != nil {
			return fmt.Errorf("beginning transaction for migration %d: %w", m.version, err)
		}

		// Split on semicolons and execute each statement individually.
		// This is necessary because modernc.org/sqlite's Exec does not
		// support multiple statements in a single call reliably.
		statements := splitStatements(string(content))
		for _, stmt := range statements {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err := tx.Exec(stmt); err != nil {
				tx.Rollback()
				return fmt.Errorf("executing migration %d statement %q: %w", m.version, truncate(stmt, 80), err)
			}
		}

		// Record the migration as applied.
		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.version); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording migration %d: %w", m.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %d: %w", m.version, err)
		}

		log.Printf("applied migration %03d (%s)", m.version, m.filename)
	}

	return nil
}

// splitStatements splits a SQL script by semicolons while respecting quoted
// strings and comments.
func splitStatements(sql string) []string {
	var statements []string
	var current strings.Builder
	inSingleQuote := false
	inLineComment := false
	runes := []rune(sql)

	for i := 0; i < len(runes); i++ {
		ch := runes[i]

		// Handle line comments (-- ...)
		if !inSingleQuote && ch == '-' && i+1 < len(runes) && runes[i+1] == '-' {
			inLineComment = true
			current.WriteRune(ch)
			continue
		}
		if inLineComment {
			current.WriteRune(ch)
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}

		// Handle single-quoted strings.
		if ch == '\'' && !inLineComment {
			inSingleQuote = !inSingleQuote
			current.WriteRune(ch)
			continue
		}

		if ch == ';' && !inSingleQuote {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
			continue
		}

		current.WriteRune(ch)
	}

	// Capture any trailing statement without a semicolon.
	if stmt := strings.TrimSpace(current.String()); stmt != "" {
		statements = append(statements, stmt)
	}

	return statements
}

// truncate shortens a string to at most n characters, appending "..." if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// Close closes the underlying database connection.
func (d *Database) Close() error {
	if d.DB != nil {
		return d.DB.Close()
	}
	return nil
}

// CleanExpiredSessions deletes all crawler_sessions whose expiry timestamp is
// in the past.
func (d *Database) CleanExpiredSessions() error {
	_, err := d.DB.Exec("DELETE FROM crawler_sessions WHERE expiry < ?", time.Now().UTC())
	if err != nil {
		return fmt.Errorf("cleaning expired sessions: %w", err)
	}
	return nil
}

// NormalizePlatformNames migrates legacy platform name "twitter" to "x" in the
// crawler_sessions table.
func (d *Database) NormalizePlatformNames() error {
	_, err := d.DB.Exec("UPDATE crawler_sessions SET platform = 'x' WHERE platform = 'twitter'")
	if err != nil {
		return fmt.Errorf("normalizing platform names: %w", err)
	}
	return nil
}
