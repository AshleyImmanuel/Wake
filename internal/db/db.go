package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/wake/wake/internal/events"
	"github.com/wake/wake/internal/state"
	_ "modernc.org/sqlite"
)

// InitDB connects to the SQLite database and runs migrations.
// It assumes the DB file is located in the .wake directory of the project root.
func InitDB(projectRoot string) (*sql.DB, error) {
	wakeDir := filepath.Join(projectRoot, ".wake")
	if err := os.MkdirAll(wakeDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create .wake directory: %w", err)
	}

	// Create a .gitignore file in the .wake directory to ignore the database
	gitignorePath := filepath.Join(wakeDir, ".gitignore")
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		_ = os.WriteFile(gitignorePath, []byte("*\n"), 0644)
	}

	dsn := filepath.Join(wakeDir, "state.db") + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// SEC-07: Single connection pool configuration prevents SQLITE_BUSY locking in WAL mode
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

func migrate(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin migration transaction: %w", err)
	}
	defer tx.Rollback()

	queries := []string{
		`CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			type TEXT NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			payload TEXT NOT NULL -- JSON payload
		);`,
		`CREATE TABLE IF NOT EXISTS checkpoints (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			commit_hash TEXT NOT NULL,
			state_version INTEGER NOT NULL,
			event_position INTEGER NOT NULL,
			state_data TEXT NOT NULL, -- JSON representation of State struct
			repository TEXT DEFAULT '',
			branch TEXT DEFAULT '',
			UNIQUE(task_id, state_version)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_events_task_timestamp ON events (task_id, timestamp ASC);`,
		`CREATE INDEX IF NOT EXISTS idx_checkpoints_task_timestamp ON checkpoints (task_id, timestamp DESC);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_checkpoints_task_version ON checkpoints (task_id, state_version);`,
	}

	for _, query := range queries {
		if _, err := tx.Exec(query); err != nil {
			return fmt.Errorf("failed to execute migration query %q: %w", query, err)
		}
	}

	// Safely add repository and branch columns if table existed without them
	_ = addColumnIfNotExists(tx, "checkpoints", "repository", "TEXT DEFAULT ''")
	_ = addColumnIfNotExists(tx, "checkpoints", "branch", "TEXT DEFAULT ''")

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration transaction: %w", err)
	}

	return nil
}

func addColumnIfNotExists(tx *sql.Tx, table, column, colDef string) error {
	query := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, colDef)
	_, err := tx.Exec(query)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			return nil
		}
		return err
	}
	return nil
}

// SaveCheckpoint writes a state.Checkpoint snapshot to the SQLite checkpoints table.
func SaveCheckpoint(ctx context.Context, db *sql.DB, cp state.Checkpoint) error {
	if db == nil {
		return fmt.Errorf("db connection is nil")
	}

	if cp.ID == uuid.Nil {
		cp.ID = uuid.New()
	}
	if cp.TaskID == uuid.Nil {
		cp.TaskID = uuid.New()
	}
	if cp.Timestamp == "" {
		cp.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	stateBytes, err := json.Marshal(cp.StateData)
	if err != nil {
		return fmt.Errorf("failed to marshal state data: %w", err)
	}

	query := `INSERT INTO checkpoints (id, task_id, timestamp, commit_hash, state_version, event_position, state_data, repository, branch)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = db.ExecContext(ctx, query,
		cp.ID.String(),
		cp.TaskID.String(),
		cp.Timestamp,
		cp.Commit,
		cp.StateVersion,
		cp.EventPosition,
		string(stateBytes),
		cp.Repository,
		cp.Branch,
	)
	if err != nil {
		return fmt.Errorf("failed to insert checkpoint: %w", err)
	}

	return nil
}

// GetLatestCheckpoint queries the most recent Checkpoint for a task (or across all tasks if taskID is empty).
func GetLatestCheckpoint(ctx context.Context, db *sql.DB, taskID string) (*state.Checkpoint, error) {
	if db == nil {
		return nil, fmt.Errorf("db connection is nil")
	}

	var query string
	var rows *sql.Rows
	var err error

	if taskID != "" && taskID != "all" {
		query = `SELECT id, task_id, timestamp, commit_hash, state_version, event_position, state_data, COALESCE(repository, ''), COALESCE(branch, '')
			FROM checkpoints
			WHERE task_id = ?
			ORDER BY timestamp DESC, state_version DESC, rowid DESC
			LIMIT 1`
		rows, err = db.QueryContext(ctx, query, taskID)
	} else {
		query = `SELECT id, task_id, timestamp, commit_hash, state_version, event_position, state_data, COALESCE(repository, ''), COALESCE(branch, '')
			FROM checkpoints
			ORDER BY timestamp DESC, state_version DESC, rowid DESC
			LIMIT 1`
		rows, err = db.QueryContext(ctx, query)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query latest checkpoint: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}

	var cp state.Checkpoint
	var idStr, taskIDStr, stateDataStr string

	if err := rows.Scan(
		&idStr,
		&taskIDStr,
		&cp.Timestamp,
		&cp.Commit,
		&cp.StateVersion,
		&cp.EventPosition,
		&stateDataStr,
		&cp.Repository,
		&cp.Branch,
	); err != nil {
		return nil, fmt.Errorf("failed to scan checkpoint row: %w", err)
	}

	// SEC-12, BUG-07: Explicitly validate and propagate all parsing errors
	parsedID, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse checkpoint id %q: %w", idStr, err)
	}
	cp.ID = parsedID

	parsedTaskID, err := uuid.Parse(taskIDStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse checkpoint task_id %q: %w", taskIDStr, err)
	}
	cp.TaskID = parsedTaskID

	if stateDataStr != "" {
		if err := json.Unmarshal([]byte(stateDataStr), &cp.StateData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal state data: %w", err)
		}
	}

	return &cp, nil
}

// SaveEvent persists an event into the SQLite events table.
func SaveEvent(ctx context.Context, db *sql.DB, e events.Event) error {
	if db == nil {
		return fmt.Errorf("db connection is nil")
	}

	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}

	payloadBytes, err := json.Marshal(e.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}

	query := `INSERT INTO events (id, task_id, type, timestamp, payload) VALUES (?, ?, ?, ?, ?)`
	_, err = db.ExecContext(ctx, query,
		e.ID.String(),
		e.TaskID.String(),
		string(e.Type),
		e.Timestamp.Format(time.RFC3339),
		string(payloadBytes),
	)
	if err != nil {
		return fmt.Errorf("failed to insert event: %w", err)
	}

	return nil
}

// GetEvents retrieves all events for a given taskID ordered chronologically.
func GetEvents(ctx context.Context, db *sql.DB, taskID string) ([]events.Event, error) {
	if db == nil {
		return nil, fmt.Errorf("db connection is nil")
	}

	query := `SELECT id, task_id, type, timestamp, payload FROM events WHERE task_id = ? ORDER BY timestamp ASC, rowid ASC`
	rows, err := db.QueryContext(ctx, query, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()

	var result []events.Event
	for rows.Next() {
		var e events.Event
		var idStr, taskIDStr, typeStr, timeStr, payloadStr string

		if err := rows.Scan(&idStr, &taskIDStr, &typeStr, &timeStr, &payloadStr); err != nil {
			return nil, fmt.Errorf("failed to scan event row: %w", err)
		}

		// SEC-12, BUG-07: Explicitly validate and propagate all parsing errors
		parsedID, err := uuid.Parse(idStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse event id %q: %w", idStr, err)
		}
		e.ID = parsedID

		parsedTaskID, err := uuid.Parse(taskIDStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse event task_id %q: %w", taskIDStr, err)
		}
		e.TaskID = parsedTaskID

		e.Type = events.EventType(typeStr)

		parsedTime, err := parseTimestamp(timeStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse event timestamp %q: %w", timeStr, err)
		}
		e.Timestamp = parsedTime

		if payloadStr != "" && payloadStr != "null" {
			var payload map[string]interface{}
			if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
				return nil, fmt.Errorf("failed to unmarshal event payload: %w", err)
			}
			e.Payload = payload
		}

		result = append(result, e)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func parseTimestamp(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("invalid timestamp format: %q", s)
}

