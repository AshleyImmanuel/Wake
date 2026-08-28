package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

func migrate(db *sql.DB) error {
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
			branch TEXT DEFAULT ''
		);`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return err
		}
	}

	// Safely add repository and branch columns if table existed without them
	_, _ = db.Exec(`ALTER TABLE checkpoints ADD COLUMN repository TEXT DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE checkpoints ADD COLUMN branch TEXT DEFAULT ''`)

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

	if parsedID, err := uuid.Parse(idStr); err == nil {
		cp.ID = parsedID
	}
	if parsedTaskID, err := uuid.Parse(taskIDStr); err == nil {
		cp.TaskID = parsedTaskID
	}

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

		if parsedID, err := uuid.Parse(idStr); err == nil {
			e.ID = parsedID
		}
		if parsedTaskID, err := uuid.Parse(taskIDStr); err == nil {
			e.TaskID = parsedTaskID
		}
		e.Type = events.EventType(typeStr)
		if parsedTime, err := time.Parse(time.RFC3339, timeStr); err == nil {
			e.Timestamp = parsedTime
		}

		if payloadStr != "" {
			var payload map[string]interface{}
			if err := json.Unmarshal([]byte(payloadStr), &payload); err == nil {
				e.Payload = payload
			}
		}

		result = append(result, e)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

