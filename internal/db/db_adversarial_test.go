package db

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/wake/wake/internal/events"
	"github.com/wake/wake/internal/state"
)

// TestDB_HighVolumeConcurrencyStress tests 100 concurrent workers performing rapid reads and writes
// to verify that SQLite in WAL mode with connection pooling (MaxOpenConns=1) produces ZERO SQLITE_BUSY / locked errors.
func TestDB_HighVolumeConcurrencyStress(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := InitDB(tmpDir)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	numWorkers := 50
	opsPerWorker := 10
	taskCount := 5

	taskIDs := make([]uuid.UUID, taskCount)
	for i := 0; i < taskCount; i++ {
		taskIDs[i] = uuid.New()
	}

	var totalOps int64
	var errorCount int64
	errChan := make(chan error, numWorkers*opsPerWorker*2)

	var wg sync.WaitGroup

	for worker := 0; worker < numWorkers; worker++ {
		wg.Add(1)
		go func(wID int) {
			defer wg.Done()
			taskID := taskIDs[wID%taskCount]

			for op := 0; op < opsPerWorker; op++ {
				version := (wID * opsPerWorker) + op + 1

				// 1. Write an event
				ev := events.NewEvent(taskID, events.CommandExecuted, map[string]interface{}{
					"worker":  wID,
					"op":      op,
					"command": fmt.Sprintf("cmd_%d_%d", wID, op),
				})
				if err := SaveEvent(ctx, db, ev); err != nil {
					atomic.AddInt64(&errorCount, 1)
					errChan <- fmt.Errorf("worker %d SaveEvent error: %w", wID, err)
					return
				}
				atomic.AddInt64(&totalOps, 1)

				// 2. Write a checkpoint (unique version per task)
				cp := state.Checkpoint{
					ID:            uuid.New(),
					TaskID:        taskID,
					Timestamp:     time.Now().UTC().Format(time.RFC3339Nano),
					Repository:    "/workspace",
					Branch:        "main",
					Commit:        fmt.Sprintf("commit_w%d_op%d", wID, op),
					StateVersion:  version,
					EventPosition: version,
					StateData: state.State{
						TaskID:    taskID,
						Objective: fmt.Sprintf("Objective worker %d op %d", wID, op),
					},
				}
				if err := SaveCheckpoint(ctx, db, cp); err != nil {
					atomic.AddInt64(&errorCount, 1)
					errChan <- fmt.Errorf("worker %d SaveCheckpoint error: %w", wID, err)
					return
				}
				atomic.AddInt64(&totalOps, 1)

				// 3. Read latest checkpoint
				if _, err := GetLatestCheckpoint(ctx, db, taskID.String()); err != nil && err != sql.ErrNoRows {
					atomic.AddInt64(&errorCount, 1)
					errChan <- fmt.Errorf("worker %d GetLatestCheckpoint error: %w", wID, err)
					return
				}
				atomic.AddInt64(&totalOps, 1)

				// 4. Read events
				if evs, err := GetEvents(ctx, db, taskID.String()); err != nil {
					atomic.AddInt64(&errorCount, 1)
					errChan <- fmt.Errorf("worker %d GetEvents error: %w", wID, err)
					return
				} else if len(evs) == 0 {
					atomic.AddInt64(&errorCount, 1)
					errChan <- fmt.Errorf("worker %d expected >0 events, got 0", wID)
					return
				}
				atomic.AddInt64(&totalOps, 1)
			}
		}(worker)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Errorf("Concurrency stress error: %v", err)
	}

	if errorCount > 0 {
		t.Fatalf("Encountered %d errors during concurrency stress test", errorCount)
	}

	expectedMinOps := int64(numWorkers * opsPerWorker * 4)
	if totalOps < expectedMinOps {
		t.Fatalf("Expected at least %d operations, executed %d", expectedMinOps, totalOps)
	}
}

// TestDB_StateVersionUniqueness_CrossTaskAndSameTask tests UNIQUE(task_id, state_version) constraints.
func TestDB_StateVersionUniqueness_CrossTaskAndSameTask(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := InitDB(tmpDir)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	taskA := uuid.New()
	taskB := uuid.New()

	// 1. Task A version 1 should succeed
	cpA1 := state.Checkpoint{
		ID:           uuid.New(),
		TaskID:       taskA,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Commit:       "commitA1",
		StateVersion: 1,
	}
	if err := SaveCheckpoint(ctx, db, cpA1); err != nil {
		t.Fatalf("SaveCheckpoint cpA1 failed: %v", err)
	}

	// 2. Task B version 1 (same state_version, different task_id) MUST succeed
	cpB1 := state.Checkpoint{
		ID:           uuid.New(),
		TaskID:       taskB,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Commit:       "commitB1",
		StateVersion: 1,
	}
	if err := SaveCheckpoint(ctx, db, cpB1); err != nil {
		t.Fatalf("SaveCheckpoint cpB1 should succeed for distinct task_id: %v", err)
	}

	// 3. Task A version 1 duplicate MUST fail with UNIQUE constraint error
	cpA1Duplicate := state.Checkpoint{
		ID:           uuid.New(),
		TaskID:       taskA,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Commit:       "commitA1_collision",
		StateVersion: 1,
	}
	err = SaveCheckpoint(ctx, db, cpA1Duplicate)
	if err == nil {
		t.Fatalf("expected UNIQUE constraint violation when inserting duplicate (taskA, version 1), got nil")
	}

	// 4. Task A version 2 should succeed
	cpA2 := state.Checkpoint{
		ID:           uuid.New(),
		TaskID:       taskA,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Commit:       "commitA2",
		StateVersion: 2,
	}
	if err := SaveCheckpoint(ctx, db, cpA2); err != nil {
		t.Fatalf("SaveCheckpoint cpA2 failed: %v", err)
	}
}

// TestDB_Deserialization_CorruptDataVariations tests various corrupted data rows in DB.
func TestDB_Deserialization_CorruptDataVariations(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := InitDB(tmpDir)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	// Case 1: Corrupt StateData JSON in Checkpoints
	validTaskID := uuid.New()
	_, err = db.Exec(`INSERT INTO checkpoints (id, task_id, timestamp, commit_hash, state_version, event_position, state_data)
		VALUES (?, ?, '2026-08-28T12:00:00Z', 'commit1', 1, 1, '{unclosed json')`,
		uuid.New().String(), validTaskID.String())
	if err != nil {
		t.Fatalf("failed to insert corrupt state_data: %v", err)
	}

	_, err = GetLatestCheckpoint(ctx, db, validTaskID.String())
	if err == nil {
		t.Fatalf("expected error on unclosed state_data JSON, got nil")
	}

	// Case 2: Corrupt timestamp formats in events
	tFormats := []struct {
		timestamp string
		valid     bool
	}{
		{"2026-08-28T12:00:00.123456789Z", true},
		{"2026-08-28T12:00:00Z", true},
		{"2026-08-28 12:00:00", true},
		{"2026-08-28T12:00:00", true},
		{"not-a-timestamp", false},
		{"2026-13-45 99:99:99", false},
		{"1724846400", false},
	}

	for _, tf := range tFormats {
		testTaskID := uuid.New()
		evID := uuid.New().String()
		_, err := db.Exec(`INSERT INTO events (id, task_id, type, timestamp, payload) VALUES (?, ?, 'TEST', ?, '{}')`,
			evID, testTaskID.String(), tf.timestamp)
		if err != nil {
			t.Fatalf("failed to insert test event: %v", err)
		}

		evs, err := GetEvents(ctx, db, testTaskID.String())
		if tf.valid {
			if err != nil {
				t.Errorf("expected valid timestamp %q to parse successfully, got error: %v", tf.timestamp, err)
			}
			if len(evs) != 1 {
				t.Errorf("expected 1 event for timestamp %q, got %d", tf.timestamp, len(evs))
			}
		} else {
			if err == nil {
				t.Errorf("expected invalid timestamp %q to fail parsing, got nil error", tf.timestamp)
			}
		}
	}
}

// TestDB_ContextCancellation tests query abortion when Context is cancelled.
func TestDB_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := InitDB(tmpDir)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	taskID := uuid.New()
	ev := events.NewEvent(taskID, events.TaskStarted, map[string]interface{}{"objective": "Cancelled"})

	if err := SaveEvent(ctx, db, ev); err == nil {
		t.Errorf("expected error when SaveEvent with cancelled context, got nil")
	}

	if _, err := GetEvents(ctx, db, taskID.String()); err == nil {
		t.Errorf("expected error when GetEvents with cancelled context, got nil")
	}

	cp := state.Checkpoint{
		ID:           uuid.New(),
		TaskID:       taskID,
		Commit:       "abc",
		StateVersion: 1,
	}
	if err := SaveCheckpoint(ctx, db, cp); err == nil {
		t.Errorf("expected error when SaveCheckpoint with cancelled context, got nil")
	}

	if _, err := GetLatestCheckpoint(ctx, db, taskID.String()); err == nil {
		t.Errorf("expected error when GetLatestCheckpoint with cancelled context, got nil")
	}
}

// TestDB_IdempotentInitDB tests running InitDB multiple times on existing directory.
func TestDB_IdempotentInitDB(t *testing.T) {
	tmpDir := t.TempDir()

	for i := 0; i < 5; i++ {
		db, err := InitDB(tmpDir)
		if err != nil {
			t.Fatalf("InitDB iteration %d failed: %v", i, err)
		}
		// Write something
		ev := events.NewEvent(uuid.New(), events.TaskStarted, map[string]interface{}{"iter": i})
		if err := SaveEvent(context.Background(), db, ev); err != nil {
			t.Fatalf("SaveEvent iteration %d failed: %v", i, err)
		}
		db.Close()
	}
}
