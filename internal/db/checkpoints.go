package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"wake/internal/state"
	"github.com/google/uuid"
)

func generateCheckpointChecksum(id, taskID, sessionID, timestamp, commit, stateData, repository, branch, author string) string {
	h := sha256.New()
	h.Write([]byte(id + taskID + sessionID + timestamp + commit + stateData + repository + branch + author))
	return hex.EncodeToString(h.Sum(nil))
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

	stateDataStr := string(stateBytes)
	checksum := generateCheckpointChecksum(cp.ID.String(), cp.TaskID.String(), cp.SessionID.String(), cp.Timestamp, cp.Commit, stateDataStr, cp.Repository, cp.Branch, cp.Author)

	query := `INSERT INTO checkpoints (id, task_id, session_id, timestamp, commit_hash, state_version, event_position, state_data, repository, branch, author, checksum)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = db.ExecContext(ctx, query,
		cp.ID.String(),
		cp.TaskID.String(),
		cp.SessionID.String(),
		cp.Timestamp,
		cp.Commit,
		cp.StateVersion,
		cp.EventPosition,
		stateDataStr,
		cp.Repository,
		cp.Branch,
		cp.Author,
		checksum,
	)
	if err != nil {
		return fmt.Errorf("failed to insert checkpoint: %w", err)
	}

	return nil
}

// GetLatestCheckpoint queries the most recent Checkpoint for a task.
func GetLatestCheckpoint(ctx context.Context, db *sql.DB, taskID string) (*state.Checkpoint, error) {
	if db == nil {
		return nil, fmt.Errorf("db connection is nil")
	}

	var query string
	var rows *sql.Rows
	var err error

	if taskID != "" && taskID != "all" {
		query = `SELECT id, task_id, COALESCE(session_id, '00000000-0000-0000-0000-000000000000'), timestamp, commit_hash, state_version, event_position, state_data, COALESCE(repository, ''), COALESCE(branch, ''), COALESCE(author, ''), COALESCE(checksum, '')
			FROM checkpoints
			WHERE task_id = ?
			ORDER BY timestamp DESC, state_version DESC, rowid DESC
			LIMIT 1`
		rows, err = db.QueryContext(ctx, query, taskID)
	} else {
		query = `SELECT id, task_id, COALESCE(session_id, '00000000-0000-0000-0000-000000000000'), timestamp, commit_hash, state_version, event_position, state_data, COALESCE(repository, ''), COALESCE(branch, ''), COALESCE(author, ''), COALESCE(checksum, '')
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
	var idStr, taskIDStr, sessionIDStr, stateDataStr, checksumStr string

	if err := rows.Scan(
		&idStr,
		&taskIDStr,
		&sessionIDStr,
		&cp.Timestamp,
		&cp.Commit,
		&cp.StateVersion,
		&cp.EventPosition,
		&stateDataStr,
		&cp.Repository,
		&cp.Branch,
		&cp.Author,
		&checksumStr,
	); err != nil {
		return nil, fmt.Errorf("failed to scan checkpoint row: %w", err)
	}

	if checksumStr == "" {
		fmt.Fprintf(os.Stderr, "warning: legacy checkpoint %s found without checksum\n", idStr)
	} else {
		expected := generateCheckpointChecksum(idStr, taskIDStr, sessionIDStr, cp.Timestamp, cp.Commit, stateDataStr, cp.Repository, cp.Branch, cp.Author)
		if checksumStr != expected {
			return nil, fmt.Errorf("state poisoning detected: checkpoint %s checksum mismatch", idStr)
		}
	}

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
	parsedSessionID, _ := uuid.Parse(sessionIDStr)
	cp.SessionID = parsedSessionID

	if stateDataStr != "" {
		if err := json.Unmarshal([]byte(stateDataStr), &cp.StateData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal state data: %w", err)
		}
	}

	return &cp, nil
}

// SaveEvent persists an event into the SQLite events table.
func GetCheckpointByVersion(ctx context.Context, db *sql.DB, taskID string, version int) (*state.Checkpoint, error) {
	if db == nil {
		return nil, fmt.Errorf("db connection is nil")
	}

	query := `SELECT id, task_id, COALESCE(session_id, '00000000-0000-0000-0000-000000000000'), timestamp, commit_hash, state_version, event_position, state_data, COALESCE(repository, ''), COALESCE(branch, ''), COALESCE(author, ''), COALESCE(checksum, '')
		FROM checkpoints
		WHERE task_id = ? AND state_version = ?
		LIMIT 1`
	rows, err := db.QueryContext(ctx, query, taskID, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}

	var cp state.Checkpoint
	var idStr, taskIDStr, sessionIDStr, stateDataStr, checksumStr string

	if err := rows.Scan(
		&idStr,
		&taskIDStr,
		&sessionIDStr,
		&cp.Timestamp,
		&cp.Commit,
		&cp.StateVersion,
		&cp.EventPosition,
		&stateDataStr,
		&cp.Repository,
		&cp.Branch,
		&cp.Author,
		&checksumStr,
	); err != nil {
		return nil, err
	}

	cp.ID, _ = uuid.Parse(idStr)
	cp.TaskID, _ = uuid.Parse(taskIDStr)
	cp.SessionID, _ = uuid.Parse(sessionIDStr)

	if stateDataStr != "" {
		_ = json.Unmarshal([]byte(stateDataStr), &cp.StateData)
	}

	return &cp, nil
}

// GetRecentCheckpoints queries the latest N checkpoints for a task.
func GetRecentCheckpoints(ctx context.Context, db *sql.DB, taskID string, limit int) ([]state.Checkpoint, error) {
	if db == nil {
		return nil, fmt.Errorf("db connection is nil")
	}

	query := `SELECT id, task_id, COALESCE(session_id, '00000000-0000-0000-0000-000000000000'), timestamp, commit_hash, state_version, event_position, state_data, COALESCE(repository, ''), COALESCE(branch, ''), COALESCE(author, ''), COALESCE(checksum, '')
		FROM checkpoints
		WHERE task_id = ?
		ORDER BY timestamp DESC, state_version DESC
		LIMIT ?`
	rows, err := db.QueryContext(ctx, query, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []state.Checkpoint
	for rows.Next() {
		var cp state.Checkpoint
		var idStr, taskIDStr, sessionIDStr, stateDataStr, checksumStr string

		if err := rows.Scan(
			&idStr,
			&taskIDStr,
			&sessionIDStr,
			&cp.Timestamp,
			&cp.Commit,
			&cp.StateVersion,
			&cp.EventPosition,
			&stateDataStr,
			&cp.Repository,
			&cp.Branch,
			&cp.Author,
			&checksumStr,
		); err != nil {
			return nil, err
		}

		cp.ID, _ = uuid.Parse(idStr)
		cp.TaskID, _ = uuid.Parse(taskIDStr)
		cp.SessionID, _ = uuid.Parse(sessionIDStr)

		if stateDataStr != "" {
			_ = json.Unmarshal([]byte(stateDataStr), &cp.StateData)
		}
		result = append(result, cp)
	}

	return result, nil
}
