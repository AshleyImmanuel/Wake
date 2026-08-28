package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/wake/wake/internal/events"
	"github.com/wake/wake/internal/state"
)

func TestDB_InitAndMigrations(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := InitDB(tmpDir)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	// Verify tables exist
	var count int
	err = db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('checkpoints', 'events')`).Scan(&count)
	if err != nil {
		t.Fatalf("table verification query failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 tables, found %d", count)
	}
}

func TestDB_SaveAndGetLatestCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := InitDB(tmpDir)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	taskID := uuid.New()

	// 1. Querying when no checkpoints exist should return sql.ErrNoRows
	_, err = GetLatestCheckpoint(ctx, db, taskID.String())
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}

	// 2. Save first checkpoint
	cp1 := state.Checkpoint{
		ID:            uuid.New(),
		TaskID:        taskID,
		Timestamp:     time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
		Repository:    "/workspace",
		Branch:        "main",
		Commit:        "commit11111111111111111111111111111111",
		StateVersion:  1,
		EventPosition: 1,
		StateData: state.State{
			TaskID:      taskID,
			Objective:   "Implement DB",
			Constraints: []string{"do not touch legacy"},
			Completed:   []string{"step 1"},
			Confidence:  state.ConfidenceHigh,
		},
	}

	if err := SaveCheckpoint(ctx, db, cp1); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	// 3. Save second checkpoint with higher state version
	cp2 := state.Checkpoint{
		ID:            uuid.New(),
		TaskID:        taskID,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Repository:    "/workspace",
		Branch:        "feature/db",
		Commit:        "commit22222222222222222222222222222222",
		StateVersion:  2,
		EventPosition: 3,
		StateData: state.State{
			TaskID:      taskID,
			Objective:   "Implement DB & Reconcile",
			Constraints: []string{"do not touch legacy", "keep SQLite fast"},
			Decisions: []state.Decision{
				{
					ID:          "DEC-01",
					Description: "Use modernc.org/sqlite",
					Status:      "ACTIVE",
				},
			},
			Completed:   []string{"step 1", "step 2"},
			Confidence:  state.ConfidenceHigh,
		},
	}

	if err := SaveCheckpoint(ctx, db, cp2); err != nil {
		t.Fatalf("SaveCheckpoint cp2 failed: %v", err)
	}

	// 4. Retrieve latest checkpoint for specific taskID
	retrieved, err := GetLatestCheckpoint(ctx, db, taskID.String())
	if err != nil {
		t.Fatalf("GetLatestCheckpoint failed: %v", err)
	}

	if retrieved.ID != cp2.ID {
		t.Errorf("expected checkpoint ID %s, got %s", cp2.ID, retrieved.ID)
	}
	if retrieved.Commit != cp2.Commit {
		t.Errorf("expected commit %s, got %s", cp2.Commit, retrieved.Commit)
	}
	if retrieved.Branch != cp2.Branch {
		t.Errorf("expected branch %s, got %s", cp2.Branch, retrieved.Branch)
	}
	if retrieved.StateVersion != 2 {
		t.Errorf("expected state version 2, got %d", retrieved.StateVersion)
	}
	if len(retrieved.StateData.Constraints) != 2 {
		t.Errorf("expected 2 constraints, got %d", len(retrieved.StateData.Constraints))
	}
	if len(retrieved.StateData.Decisions) != 1 || retrieved.StateData.Decisions[0].ID != "DEC-01" {
		t.Errorf("expected decision DEC-01, got %v", retrieved.StateData.Decisions)
	}

	// 5. Retrieve latest checkpoint globally (empty taskID)
	retrievedGlobal, err := GetLatestCheckpoint(ctx, db, "")
	if err != nil {
		t.Fatalf("GetLatestCheckpoint global failed: %v", err)
	}
	if retrievedGlobal.ID != cp2.ID {
		t.Errorf("expected latest global checkpoint ID %s, got %s", cp2.ID, retrievedGlobal.ID)
	}
}

func TestDB_EventsPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := InitDB(tmpDir)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	taskID := uuid.New()

	ev1 := events.NewEvent(taskID, events.TaskStarted, map[string]interface{}{
		"objective": "Build Sentinel",
	})
	ev2 := events.NewEvent(taskID, events.ConstraintAdded, map[string]interface{}{
		"constraint": "auth/*",
	})

	if err := SaveEvent(ctx, db, ev1); err != nil {
		t.Fatalf("SaveEvent ev1 failed: %v", err)
	}
	if err := SaveEvent(ctx, db, ev2); err != nil {
		t.Fatalf("SaveEvent ev2 failed: %v", err)
	}

	history, err := GetEvents(ctx, db, taskID.String())
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}

	if len(history) != 2 {
		t.Fatalf("expected 2 events, got %d", len(history))
	}
	if history[0].Type != events.TaskStarted {
		t.Errorf("expected first event TaskStarted, got %s", history[0].Type)
	}
	if history[1].Type != events.ConstraintAdded {
		t.Errorf("expected second event ConstraintAdded, got %s", history[1].Type)
	}
}

func TestDB_NilDBErrors(t *testing.T) {
	ctx := context.Background()
	if err := SaveCheckpoint(ctx, nil, state.Checkpoint{}); err == nil {
		t.Errorf("expected error when saving checkpoint with nil db")
	}
	if _, err := GetLatestCheckpoint(ctx, nil, ""); err == nil {
		t.Errorf("expected error when getting latest checkpoint with nil db")
	}
	if err := SaveEvent(ctx, nil, events.Event{}); err == nil {
		t.Errorf("expected error when saving event with nil db")
	}
	if _, err := GetEvents(ctx, nil, ""); err == nil {
		t.Errorf("expected error when getting events with nil db")
	}
}

func TestDB_ConnectionPoolSettings(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := InitDB(tmpDir)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	stats := db.Stats()
	if stats.MaxOpenConnections != 1 {
		t.Errorf("expected MaxOpenConnections = 1, got %d", stats.MaxOpenConnections)
	}
}

func TestDB_ConcurrentAccess_NoLocking(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := InitDB(tmpDir)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	taskID := uuid.New()
	concurrency := 20
	errChan := make(chan error, concurrency*2)

	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			ev := events.NewEvent(taskID, events.CommandExecuted, map[string]interface{}{
				"command": fmt.Sprintf("go test ./pkg%d", idx),
			})
			if err := SaveEvent(ctx, db, ev); err != nil {
				errChan <- fmt.Errorf("concurrent SaveEvent %d failed: %w", idx, err)
				return
			}
			errChan <- nil
		}(i)

		go func(idx int) {
			cp := state.Checkpoint{
				ID:            uuid.New(),
				TaskID:        uuid.New(), // unique task per goroutine to avoid version collisions
				Timestamp:     time.Now().UTC().Format(time.RFC3339),
				Repository:    "/workspace",
				Branch:        "main",
				Commit:        fmt.Sprintf("commit%d", idx),
				StateVersion:  1,
				EventPosition: idx,
				StateData: state.State{
					Objective: fmt.Sprintf("Objective %d", idx),
				},
			}
			if err := SaveCheckpoint(ctx, db, cp); err != nil {
				errChan <- fmt.Errorf("concurrent SaveCheckpoint %d failed: %w", idx, err)
				return
			}
			errChan <- nil
		}(i)
	}

	for i := 0; i < concurrency*2; i++ {
		if err := <-errChan; err != nil {
			t.Fatalf("concurrency test failed: %v", err)
		}
	}

	evs, err := GetEvents(ctx, db, taskID.String())
	if err != nil {
		t.Fatalf("GetEvents failed after concurrent writes: %v", err)
	}
	if len(evs) != concurrency {
		t.Errorf("expected %d events, got %d", concurrency, len(evs))
	}
}

func TestDB_IndexVerification(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := InitDB(tmpDir)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	expectedIndices := []string{
		"idx_events_task_timestamp",
		"idx_checkpoints_task_timestamp",
		"idx_checkpoints_task_version",
	}

	for _, idx := range expectedIndices {
		var count int
		err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='index' AND name = ?`, idx).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query sqlite_master for index %s: %v", idx, err)
		}
		if count != 1 {
			t.Errorf("expected index %s to exist, count = %d", idx, count)
		}
	}
}

func TestDB_StateVersionUniquenessConstraint(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := InitDB(tmpDir)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	taskID := uuid.New()

	cp1 := state.Checkpoint{
		ID:            uuid.New(),
		TaskID:        taskID,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Commit:        "commit1",
		StateVersion:  1,
		EventPosition: 1,
	}

	if err := SaveCheckpoint(ctx, db, cp1); err != nil {
		t.Fatalf("SaveCheckpoint cp1 failed: %v", err)
	}

	// Attempt to save duplicate (task_id, state_version) with different ID
	cp2 := state.Checkpoint{
		ID:            uuid.New(),
		TaskID:        taskID,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Commit:        "commit2",
		StateVersion:  1, // duplicate state version for same taskID
		EventPosition: 2,
	}

	err = SaveCheckpoint(ctx, db, cp2)
	if err == nil {
		t.Fatalf("expected UNIQUE constraint violation error when saving duplicate state_version, got nil")
	}
}

func TestDB_Deserialization_CheckpointUUIDError(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := InitDB(tmpDir)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Insert corrupted checkpoint row with invalid UUID
	_, err = db.Exec(`INSERT INTO checkpoints (id, task_id, timestamp, commit_hash, state_version, event_position, state_data)
		VALUES ('corrupt-uuid', 'corrupt-task-id', '2026-08-28T12:00:00Z', 'commit1', 1, 1, '{}')`)
	if err != nil {
		t.Fatalf("failed to insert corrupt checkpoint row: %v", err)
	}

	_, err = GetLatestCheckpoint(ctx, db, "corrupt-task-id")
	if err == nil {
		t.Fatalf("expected error on corrupt checkpoint id/task_id, got nil")
	}
}

func TestDB_Deserialization_EventErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := InitDB(tmpDir)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	taskID := uuid.New().String()

	// Test 1: Corrupted Event ID
	_, err = db.Exec(`INSERT INTO events (id, task_id, type, timestamp, payload) VALUES ('bad-id', ?, 'TASK_STARTED', '2026-08-28T12:00:00Z', '{}')`, taskID)
	if err != nil {
		t.Fatalf("failed to insert event: %v", err)
	}

	_, err = GetEvents(ctx, db, taskID)
	if err == nil {
		t.Fatalf("expected error on corrupted event UUID, got nil")
	}

	// Clean table
	_, _ = db.Exec(`DELETE FROM events`)

	// Test 2: Corrupted Event Timestamp
	validID := uuid.New().String()
	_, err = db.Exec(`INSERT INTO events (id, task_id, type, timestamp, payload) VALUES (?, ?, 'TASK_STARTED', 'not-a-timestamp', '{}')`, validID, taskID)
	if err != nil {
		t.Fatalf("failed to insert event: %v", err)
	}

	_, err = GetEvents(ctx, db, taskID)
	if err == nil {
		t.Fatalf("expected error on corrupted timestamp, got nil")
	}

	// Clean table
	_, _ = db.Exec(`DELETE FROM events`)

	// Test 3: Corrupted Event JSON Payload
	_, err = db.Exec(`INSERT INTO events (id, task_id, type, timestamp, payload) VALUES (?, ?, 'TASK_STARTED', '2026-08-28T12:00:00Z', '{invalid-json')`, validID, taskID)
	if err != nil {
		t.Fatalf("failed to insert event: %v", err)
	}

	_, err = GetEvents(ctx, db, taskID)
	if err == nil {
		t.Fatalf("expected error on corrupted JSON payload, got nil")
	}
}
