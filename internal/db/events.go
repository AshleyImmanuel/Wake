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

	"github.com/google/uuid"
	"wake/internal/events"
)

func generateEventChecksum(id, taskID, sessionID, eventType, timestamp, payload, author string) string {
	h := sha256.New()
	h.Write([]byte(id + taskID + sessionID + eventType + timestamp + payload + author))
	return hex.EncodeToString(h.Sum(nil))
}

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
	checksum := generateEventChecksum(e.ID.String(), e.TaskID.String(), e.SessionID.String(), string(e.Type), timestampStr, payloadStr, e.Author)

	query := `INSERT INTO events (id, task_id, session_id, type, timestamp, payload, author, checksum) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err = db.ExecContext(ctx, query,
		e.ID.String(),
		e.TaskID.String(),
		e.SessionID.String(),
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

	query := `SELECT id, task_id, COALESCE(session_id, '00000000-0000-0000-0000-000000000000'), type, timestamp, payload, COALESCE(author, ''), COALESCE(checksum, '') FROM events WHERE task_id = ? ORDER BY timestamp ASC, rowid ASC`
	rows, err := db.QueryContext(ctx, query, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()

	var result []events.Event
	for rows.Next() {
		var e events.Event
		var idStr, taskIDStr, sessionIDStr, typeStr, timeStr, payloadStr, checksumStr string

		if err := rows.Scan(&idStr, &taskIDStr, &sessionIDStr, &typeStr, &timeStr, &payloadStr, &e.Author, &checksumStr); err != nil {
			return nil, fmt.Errorf("failed to scan event row: %w", err)
		}

		if checksumStr == "" {
			fmt.Fprintf(os.Stderr, "warning: legacy event %s found without checksum\n", idStr)
		} else {
			expected := generateEventChecksum(idStr, taskIDStr, sessionIDStr, typeStr, timeStr, payloadStr, e.Author)
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
		parsedSessionID, _ := uuid.Parse(sessionIDStr)
		e.SessionID = parsedSessionID

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
