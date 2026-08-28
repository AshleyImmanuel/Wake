package db

import (
	"context"
	"database/sql"
	"errors"
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
