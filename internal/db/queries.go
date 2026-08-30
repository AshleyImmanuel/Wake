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

	"github.com/AshleyImmanuel/Wake/internal/events"
	"github.com/AshleyImmanuel/Wake/internal/state"
	"github.com/google/uuid"
)

func generateEventChecksum(id, taskID, eventType, timestamp, payload, author string) string {
	h := sha256.New()
	h.Write([]byte(id + taskID + eventType + timestamp + payload + author))
	return hex.EncodeToString(h.Sum(nil))
}

func generateCheckpointChecksum(id, taskID, timestamp, commit, stateData, repository, branch, author string) string {
	h := sha256.New()
	h.Write([]byte(id + taskID + timestamp + commit + stateData + repository + branch + author))
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
	checksum := generateCheckpointChecksum(cp.ID.String(), cp.TaskID.String(), cp.Timestamp, cp.Commit, stateDataStr, cp.Repository, cp.Branch, cp.Author)

	query := `INSERT INTO checkpoints (id, task_id, timestamp, commit_hash, state_version, event_position, state_data, repository, branch, author, checksum)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = db.ExecContext(ctx, query,
		cp.ID.String(),
		cp.TaskID.String(),
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
		query = `SELECT id, task_id, timestamp, commit_hash, state_version, event_position, state_data, COALESCE(repository, ''), COALESCE(branch, ''), COALESCE(author, ''), COALESCE(checksum, '')
			FROM checkpoints
			WHERE task_id = ?
			ORDER BY timestamp DESC, state_version DESC, rowid DESC
			LIMIT 1`
		rows, err = db.QueryContext(ctx, query, taskID)
	} else {
		query = `SELECT id, task_id, timestamp, commit_hash, state_version, event_position, state_data, COALESCE(repository, ''), COALESCE(branch, ''), COALESCE(author, ''), COALESCE(checksum, '')
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
	var idStr, taskIDStr, stateDataStr, checksumStr string

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
		&cp.Author,
		&checksumStr,
	); err != nil {
		return nil, fmt.Errorf("failed to scan checkpoint row: %w", err)
	}

	if checksumStr == "" {
		fmt.Fprintf(os.Stderr, "warning: legacy checkpoint %s found without checksum\n", idStr)
	} else {
		expected := generateCheckpointChecksum(idStr, taskIDStr, cp.Timestamp, cp.Commit, stateDataStr, cp.Repository, cp.Branch, cp.Author)
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

	clonedPayload := events.ClonePayload(e.Payload)
	payloadBytes, err := json.Marshal(clonedPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}

	payloadStr := string(payloadBytes)
	timestampStr := e.Timestamp.Format(time.RFC3339)
	checksum := generateEventChecksum(e.ID.String(), e.TaskID.String(), string(e.Type), timestampStr, payloadStr, e.Author)

	query := `INSERT INTO events (id, task_id, type, timestamp, payload, author, checksum) VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err = db.ExecContext(ctx, query,
		e.ID.String(),
		e.TaskID.String(),
		string(e.Type),
		timestampStr,
		payloadStr,
		e.Author,
		checksum,
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

	query := `SELECT id, task_id, type, timestamp, payload, COALESCE(author, ''), COALESCE(checksum, '') FROM events WHERE task_id = ? ORDER BY timestamp ASC, rowid ASC`
	rows, err := db.QueryContext(ctx, query, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()

	var result []events.Event
	for rows.Next() {
		var e events.Event
		var idStr, taskIDStr, typeStr, timeStr, payloadStr, checksumStr string

		if err := rows.Scan(&idStr, &taskIDStr, &typeStr, &timeStr, &payloadStr, &e.Author, &checksumStr); err != nil {
			return nil, fmt.Errorf("failed to scan event row: %w", err)
		}

		if checksumStr == "" {
			fmt.Fprintf(os.Stderr, "warning: legacy event %s found without checksum\n", idStr)
		} else {
			expected := generateEventChecksum(idStr, taskIDStr, typeStr, timeStr, payloadStr, e.Author)
			if checksumStr != expected {
				return nil, fmt.Errorf("state poisoning detected: event %s checksum mismatch", idStr)
			}
		}

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

// PruneStats holds the results of a prune operation.
type PruneStats struct {
	DeletedCheckpoints int64
	DeletedEvents      int64
}

// PruneHistory deletes old checkpoints and events to save space.
func PruneHistory(ctx context.Context, db *sql.DB, olderThan time.Time) (*PruneStats, error) {
	if db == nil {
		return nil, fmt.Errorf("db connection is nil")
	}

	threshold := olderThan.UTC().Format(time.RFC3339)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin tx: %w", err)
	}
	defer tx.Rollback()

	queryCheckpoints := `
		DELETE FROM checkpoints 
		WHERE timestamp < ? 
		AND id NOT IN (
			SELECT id FROM (
				SELECT id, ROW_NUMBER() OVER(PARTITION BY task_id ORDER BY timestamp DESC, state_version DESC) as rn 
				FROM checkpoints
			) WHERE rn = 1
		)
	`
	resCp, err := tx.ExecContext(ctx, queryCheckpoints, threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to delete old checkpoints: %w", err)
	}
	deletedCp, _ := resCp.RowsAffected()

	queryEvents := `
		DELETE FROM events 
		WHERE timestamp < ? 
		AND task_id IN (
			SELECT c.task_id FROM checkpoints c WHERE c.timestamp >= events.timestamp
		)
	`
	resEv, err := tx.ExecContext(ctx, queryEvents, threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to delete old events: %w", err)
	}
	deletedEv, _ := resEv.RowsAffected()

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit prune transaction: %w", err)
	}

	return &PruneStats{
		DeletedCheckpoints: deletedCp,
		DeletedEvents:      deletedEv,
	}, nil
}
