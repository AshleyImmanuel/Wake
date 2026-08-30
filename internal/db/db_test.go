package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/wake/wake/internal/events"
	"github.com/wake/wake/internal/state"
)

// Helper for assertions
func assertEqual(t *testing.T, expected, actual interface{}, msg string) {
	t.Helper()
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("%s: expected %v, got %v", msg, expected, actual)
	}
}

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertError(t *testing.T, err error, msg string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error for %s, got nil", msg)
	}
}

func TestInitDB(t *testing.T) {
	t.Run("creates directories and sets connections", func(t *testing.T) {
		tmpDir := t.TempDir()
		db, err := InitDB(tmpDir)
		assertNoError(t, err)
		defer db.Close()

		// Verify dir and file
		wakeDir := filepath.Join(tmpDir, ".wake")
		if _, err := os.Stat(wakeDir); os.IsNotExist(err) {
			t.Errorf(".wake dir was not created")
		}
		if _, err := os.Stat(filepath.Join(wakeDir, "state.db")); os.IsNotExist(err) {
			t.Errorf("state.db was not created")
		}
		if _, err := os.Stat(filepath.Join(wakeDir, ".gitignore")); os.IsNotExist(err) {
			t.Errorf(".gitignore was not created")
		}

		// Verify max open conns
		stats := db.Stats()
		if stats.MaxOpenConnections != 1 {
			t.Errorf("expected MaxOpenConns to be 1, got %d", stats.MaxOpenConnections)
		}
	})

	t.Run("is idempotent", func(t *testing.T) {
		tmpDir := t.TempDir()
		db1, err := InitDB(tmpDir)
		assertNoError(t, err)
		db1.Close()

		db2, err := InitDB(tmpDir)
		assertNoError(t, err)
		defer db2.Close()
	})

	t.Run("invalid path returns error", func(t *testing.T) {
		_, err := InitDB(filepath.Join(t.TempDir(), "nonexistent", "nested", "path\x00")) // Null byte might fail dir creation
		assertError(t, err, "invalid path")
	})
}

func TestMigrations(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := InitDB(tmpDir)
	assertNoError(t, err)
	defer db.Close()

	t.Run("tables exist", func(t *testing.T) {
		tables := []string{"events", "checkpoints"}
		for _, table := range tables {
			var name string
			err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
			assertNoError(t, err)
			assertEqual(t, table, name, "table existence")
		}
	})

	t.Run("indices exist", func(t *testing.T) {
		indices := []string{
			"idx_events_task_timestamp",
			"idx_checkpoints_task_timestamp",
			"idx_checkpoints_task_version",
		}
		for _, idx := range indices {
			var name string
			err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name=?", idx).Scan(&name)
			assertNoError(t, err)
			assertEqual(t, idx, name, "index existence")
		}
	})

	t.Run("unique constraint task_id state_version", func(t *testing.T) {
		taskID := uuid.New().String()
		_, err := db.Exec(`INSERT INTO checkpoints (id, task_id, state_version, commit_hash, event_position, state_data) VALUES (?, ?, ?, '', 0, '{}')`, uuid.New().String(), taskID, 1)
		assertNoError(t, err)

		_, err = db.Exec(`INSERT INTO checkpoints (id, task_id, state_version, commit_hash, event_position, state_data) VALUES (?, ?, ?, '', 0, '{}')`, uuid.New().String(), taskID, 1)
		assertError(t, err, "inserting duplicate task_id and state_version")
	})

	t.Run("addColumnIfNotExists handles existing columns", func(t *testing.T) {
		tx, err := db.Begin()
		assertNoError(t, err)
		defer tx.Rollback()

		err = addColumnIfNotExists(tx, "checkpoints", "repository", "TEXT")
		assertNoError(t, err) // Should not error since it already exists and error is suppressed
	})
}

func TestCheckpoints(t *testing.T) {
	ctx := context.Background()

	t.Run("nil db error", func(t *testing.T) {
		err := SaveCheckpoint(ctx, nil, state.Checkpoint{})
		assertError(t, err, "save with nil db")
		
		_, err = GetLatestCheckpoint(ctx, nil, "")
		assertError(t, err, "get with nil db")
	})

	t.Run("no rows returns ErrNoRows", func(t *testing.T) {
		tmpDir := t.TempDir()
		db, err := InitDB(tmpDir)
		assertNoError(t, err)
		defer db.Close()

		_, err = GetLatestCheckpoint(ctx, db, uuid.New().String())
		if err != sql.ErrNoRows {
			t.Errorf("expected sql.ErrNoRows, got %v", err)
		}
	})

	t.Run("save and get round-trip", func(t *testing.T) {
		tmpDir := t.TempDir()
		db, err := InitDB(tmpDir)
		assertNoError(t, err)
		defer db.Close()

		cp := state.Checkpoint{
			ID:            uuid.New(),
			TaskID:        uuid.New(),
			Timestamp:     time.Now().UTC().Format(time.RFC3339),
			Repository:    "repo",
			Branch:        "main",
			Commit:        "abcdef",
			StateVersion:  1,
			EventPosition: 2,
			StateData: state.State{
				TaskID:      uuid.New(),
				Objective:   "Test objective",
				Constraints: []string{"C1"},
				Confidence:  state.ConfidenceHigh,
			},
		}

		err = SaveCheckpoint(ctx, db, cp)
		assertNoError(t, err)

		fetched, err := GetLatestCheckpoint(ctx, db, cp.TaskID.String())
		assertNoError(t, err)
		assertEqual(t, cp.ID, fetched.ID, "ID")
		assertEqual(t, cp.TaskID, fetched.TaskID, "TaskID")
		assertEqual(t, cp.Timestamp, fetched.Timestamp, "Timestamp")
		assertEqual(t, cp.Repository, fetched.Repository, "Repository")
		assertEqual(t, cp.Branch, fetched.Branch, "Branch")
		assertEqual(t, cp.Commit, fetched.Commit, "Commit")
		assertEqual(t, cp.StateVersion, fetched.StateVersion, "StateVersion")
		assertEqual(t, cp.EventPosition, fetched.EventPosition, "EventPosition")
		
		// JSON serialization converts string slice to nil if empty, so let's only compare initialized fields
		assertEqual(t, cp.StateData.Objective, fetched.StateData.Objective, "Objective")
		assertEqual(t, cp.StateData.Constraints, fetched.StateData.Constraints, "Constraints")
		assertEqual(t, cp.StateData.Confidence, fetched.StateData.Confidence, "Confidence")
	})

	t.Run("auto-generates fields", func(t *testing.T) {
		tmpDir := t.TempDir()
		db, err := InitDB(tmpDir)
		assertNoError(t, err)
		defer db.Close()

		cp := state.Checkpoint{}
		err = SaveCheckpoint(ctx, db, cp)
		assertNoError(t, err)

		fetched, err := GetLatestCheckpoint(ctx, db, "")
		assertNoError(t, err)

		if fetched.ID == uuid.Nil {
			t.Error("ID should have been generated")
		}
		if fetched.TaskID == uuid.Nil {
			t.Error("TaskID should have been generated")
		}
		if fetched.Timestamp == "" {
			t.Error("Timestamp should have been generated")
		}
	})

	t.Run("get latest checkpoint ordering", func(t *testing.T) {
		tmpDir := t.TempDir()
		db, err := InitDB(tmpDir)
		assertNoError(t, err)
		defer db.Close()

		taskID := uuid.New()
		
		// Insert older
		cp1 := state.Checkpoint{TaskID: taskID, StateVersion: 1, Timestamp: "2020-01-01T00:00:00Z"}
		err = SaveCheckpoint(ctx, db, cp1)
		assertNoError(t, err)

		// Insert newer
		cp2 := state.Checkpoint{TaskID: taskID, StateVersion: 2, Timestamp: "2020-01-02T00:00:00Z"}
		err = SaveCheckpoint(ctx, db, cp2)
		assertNoError(t, err)

		// Insert for different task but even newer
		taskID2 := uuid.New()
		cp3 := state.Checkpoint{TaskID: taskID2, StateVersion: 1, Timestamp: "2020-01-03T00:00:00Z"}
		err = SaveCheckpoint(ctx, db, cp3)
		assertNoError(t, err)

		// Get for taskID (should be cp2)
		fetched1, err := GetLatestCheckpoint(ctx, db, taskID.String())
		assertNoError(t, err)
		assertEqual(t, 2, fetched1.StateVersion, "StateVersion for taskID")

		// Get for empty taskID (should be cp3)
		fetched2, err := GetLatestCheckpoint(ctx, db, "")
		assertNoError(t, err)
		assertEqual(t, taskID2, fetched2.TaskID, "TaskID for all tasks")
	})

	t.Run("corrupted data propagation", func(t *testing.T) {
		tmpDir := t.TempDir()
		db, err := InitDB(tmpDir)
		assertNoError(t, err)
		defer db.Close()

		taskID := uuid.New().String()
		_, err = db.Exec("INSERT INTO checkpoints (id, task_id, state_version, commit_hash, event_position, state_data) VALUES ('invalid-uuid', ?, 1, '', 0, '{}')", taskID)
		assertNoError(t, err)

		_, err = GetLatestCheckpoint(ctx, db, taskID)
		assertError(t, err, "parsing invalid id uuid")
		if !strings.Contains(err.Error(), "failed to parse checkpoint id") {
			t.Errorf("unexpected error msg: %v", err)
		}

		// Fix ID, break TaskID
		_, err = db.Exec("UPDATE checkpoints SET id=?, task_id='invalid-task' WHERE task_id=?", uuid.New().String(), taskID)
		assertNoError(t, err)
		_, err = GetLatestCheckpoint(ctx, db, "invalid-task")
		assertError(t, err, "parsing invalid task uuid")
		
		// Fix TaskID, break state_data
		fixedTask := uuid.New().String()
		_, err = db.Exec("UPDATE checkpoints SET task_id=?, state_data='invalid-json' WHERE task_id='invalid-task'", fixedTask)
		assertNoError(t, err)
		_, err = GetLatestCheckpoint(ctx, db, fixedTask)
		assertError(t, err, "unmarshalling state data")
	})
}

func TestEvents(t *testing.T) {
	ctx := context.Background()

	t.Run("nil db error", func(t *testing.T) {
		err := SaveEvent(ctx, nil, events.Event{})
		assertError(t, err, "save with nil db")
		
		_, err = GetEvents(ctx, nil, "")
		assertError(t, err, "get with nil db")
	})

	t.Run("save and get round-trip", func(t *testing.T) {
		tmpDir := t.TempDir()
		db, err := InitDB(tmpDir)
		assertNoError(t, err)
		defer db.Close()

		ev := events.Event{
			ID:        uuid.New(),
			TaskID:    uuid.New(),
			Type:      events.TaskStarted,
			Timestamp: time.Now().UTC().Truncate(time.Second), // Truncate for exact RFC3339 comparison
			Payload:   map[string]interface{}{"key": "value", "num": float64(42)}, // JSON unmarshals numbers as float64
		}

		err = SaveEvent(ctx, db, ev)
		assertNoError(t, err)

		fetched, err := GetEvents(ctx, db, ev.TaskID.String())
		assertNoError(t, err)
		if len(fetched) != 1 {
			t.Fatalf("expected 1 event, got %d", len(fetched))
		}

		assertEqual(t, ev.ID, fetched[0].ID, "ID")
		assertEqual(t, ev.TaskID, fetched[0].TaskID, "TaskID")
		assertEqual(t, ev.Type, fetched[0].Type, "Type")
		assertEqual(t, ev.Timestamp.Format(time.RFC3339), fetched[0].Timestamp.Format(time.RFC3339), "Timestamp")
		assertEqual(t, ev.Payload, fetched[0].Payload, "Payload")
	})

	t.Run("auto-generates fields", func(t *testing.T) {
		tmpDir := t.TempDir()
		db, err := InitDB(tmpDir)
		assertNoError(t, err)
		defer db.Close()

		taskID := uuid.New()
		ev := events.Event{
			TaskID: taskID,
			Type:   events.FileChanged,
		}

		err = SaveEvent(ctx, db, ev)
		assertNoError(t, err)

		fetched, err := GetEvents(ctx, db, taskID.String())
		assertNoError(t, err)
		if len(fetched) != 1 {
			t.Fatalf("expected 1 event, got %d", len(fetched))
		}

		if fetched[0].ID == uuid.Nil {
			t.Error("ID should have been generated")
		}
		if fetched[0].Timestamp.IsZero() {
			t.Error("Timestamp should have been generated")
		}
	})

	t.Run("chronological order", func(t *testing.T) {
		tmpDir := t.TempDir()
		db, err := InitDB(tmpDir)
		assertNoError(t, err)
		defer db.Close()

		taskID := uuid.New()
		
		// Add older event
		err = SaveEvent(ctx, db, events.Event{TaskID: taskID, Timestamp: time.Now().Add(-1 * time.Hour)})
		assertNoError(t, err)

		// Add newer event
		err = SaveEvent(ctx, db, events.Event{TaskID: taskID, Timestamp: time.Now()})
		assertNoError(t, err)

		fetched, err := GetEvents(ctx, db, taskID.String())
		assertNoError(t, err)
		if len(fetched) != 2 {
			t.Fatalf("expected 2 events")
		}

		if fetched[0].Timestamp.After(fetched[1].Timestamp) {
			t.Error("events not in chronological order")
		}
	})

	t.Run("unknown taskID returns empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		db, err := InitDB(tmpDir)
		assertNoError(t, err)
		defer db.Close()

		fetched, err := GetEvents(ctx, db, uuid.New().String())
		assertNoError(t, err)
		if len(fetched) != 0 {
			t.Errorf("expected empty slice, got %d", len(fetched))
		}
	})

	t.Run("corrupted data propagation", func(t *testing.T) {
		tmpDir := t.TempDir()
		db, err := InitDB(tmpDir)
		assertNoError(t, err)
		defer db.Close()

		taskID := uuid.New().String()
		_, err = db.Exec("INSERT INTO events (id, task_id, type, timestamp, payload) VALUES ('invalid-uuid', ?, 'TYPE', '2006-01-02T15:04:05Z', '{}')", taskID)
		assertNoError(t, err)

		_, err = GetEvents(ctx, db, taskID)
		assertError(t, err, "parsing invalid id uuid")

		// Fix ID, break TaskID
		_, err = db.Exec("UPDATE events SET id=?, task_id='invalid-task' WHERE task_id=?", uuid.New().String(), taskID)
		assertNoError(t, err)
		_, err = GetEvents(ctx, db, "invalid-task")
		assertError(t, err, "parsing invalid task uuid")

		// Fix TaskID, break timestamp
		fixedTask := uuid.New().String()
		_, err = db.Exec("UPDATE events SET task_id=?, timestamp='invalid-time' WHERE task_id='invalid-task'", fixedTask)
		assertNoError(t, err)
		_, err = GetEvents(ctx, db, fixedTask)
		assertError(t, err, "parsing invalid timestamp")
	})
}

func TestParseTimestamp(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		isValid bool
	}{
		{"RFC3339Nano", "2020-01-01T12:00:00.123456Z", true},
		{"RFC3339", "2020-01-01T12:00:00Z", true},
		{"SQL space", "2020-01-01 12:00:00", true},
		{"SQL T", "2020-01-01T12:00:00", true},
		{"invalid", "not-a-time", false},
		{"empty", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseTimestamp(tc.input)
			if tc.isValid && err != nil {
				t.Errorf("expected valid for %q, got err: %v", tc.input, err)
			}
			if !tc.isValid && err == nil {
				t.Errorf("expected invalid for %q, got no error", tc.input)
			}
		})
	}
}
