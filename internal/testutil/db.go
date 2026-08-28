package testutil

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/wake/wake/internal/db"
	_ "modernc.org/sqlite"
)

// SchemaDDL contains the baseline table definitions for events and checkpoints.
const SchemaDDL = `
CREATE TABLE IF NOT EXISTS events (
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL,
	type TEXT NOT NULL,
	timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
	payload TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS checkpoints (
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL,
	timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
	commit_hash TEXT NOT NULL,
	state_version INTEGER NOT NULL,
	event_position INTEGER NOT NULL,
	state_data TEXT NOT NULL,
	repository TEXT DEFAULT '',
	branch TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_events_task_id ON events (task_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_checkpoints_task_id ON checkpoints (task_id, timestamp DESC);
`

// NewTestDB initializes a file-backed SQLite test database in a temporary directory.
// It automatically closes the database when the test completes.
func NewTestDB(t testing.TB) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	database, err := db.InitDB(tmpDir)
	if err != nil {
		t.Fatalf("NewTestDB failed to initialize database in %s: %v", tmpDir, err)
	}

	t.Cleanup(func() {
		_ = database.Close()
	})

	return database
}

// NewTestDBPath creates a temporary project root with an initialized .sentinel/state.db.
// Returns the root directory path.
func NewTestDBPath(t testing.TB) string {
	t.Helper()
	tmpDir := t.TempDir()
	database, err := db.InitDB(tmpDir)
	if err != nil {
		t.Fatalf("NewTestDBPath failed: %v", err)
	}
	_ = database.Close()
	return tmpDir
}

// NewInMemoryDB initializes an in-memory SQLite database with migrations applied.
func NewInMemoryDB(t testing.TB) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite database: %v", err)
	}

	if _, err := database.Exec(SchemaDDL); err != nil {
		_ = database.Close()
		t.Fatalf("failed to execute schema migrations on in-memory db: %v", err)
	}

	t.Cleanup(func() {
		_ = database.Close()
	})

	return database
}

// CountRows returns the number of rows in the specified table.
func CountRows(t testing.TB, database *sql.DB, tableName string) int {
	t.Helper()
	var count int
	query := fmt.Sprintf("SELECT count(*) FROM %s", tableName)
	if err := database.QueryRow(query).Scan(&count); err != nil {
		t.Fatalf("CountRows failed on table %s: %v", tableName, err)
	}
	return count
}
