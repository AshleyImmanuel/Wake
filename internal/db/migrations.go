package db

import (
	"database/sql"
	"fmt"
	"strings"
)

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
			payload TEXT NOT NULL, -- JSON payload
			checksum TEXT NOT NULL DEFAULT ''
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
			checksum TEXT NOT NULL DEFAULT '',
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

	_ = addColumnIfNotExists(tx, "checkpoints", "repository", "TEXT DEFAULT ''")
	_ = addColumnIfNotExists(tx, "checkpoints", "branch", "TEXT DEFAULT ''")
	_ = addColumnIfNotExists(tx, "checkpoints", "author", "TEXT DEFAULT ''")
	_ = addColumnIfNotExists(tx, "events", "author", "TEXT DEFAULT ''")
	_ = addColumnIfNotExists(tx, "events", "checksum", "TEXT DEFAULT ''")
	_ = addColumnIfNotExists(tx, "checkpoints", "checksum", "TEXT DEFAULT ''")

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
