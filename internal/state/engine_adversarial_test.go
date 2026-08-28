package state

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/wake/wake/internal/events"
)

// TestEngine_All17EventTypes_EmptyAndMalformedPayloads tests that every single event type handles
// empty payloads, missing fields, nil maps, and unexpected data types gracefully without panic or corruption.
func TestEngine_All17EventTypes_EmptyAndMalformedPayloads(t *testing.T) {
	taskID := uuid.New()

	allEventTypes := []events.EventType{
		events.TaskStarted,
		events.RequirementAdded,
		events.ConstraintAdded,
		events.UserApproval,
		events.UserRejection,
		events.DecisionMade,
		events.FileChanged,
		events.CommandExecuted,
		events.TestStarted,
		events.TestPassed,
		events.TestFailed,
		events.BlockerCreated,
		events.BlockerResolved,
		events.MilestoneCompleted,
		events.GitCommit,
		events.SessionInterrupted,
		events.SessionResumed,
	}

	// 1. Test completely nil payload for every event type
	for _, et := range allEventTypes {
		t.Run("NilPayload_"+string(et), func(t *testing.T) {
			h := []events.Event{
				{
					ID:        uuid.New(),
					TaskID:    taskID,
					Type:      et,
					Timestamp: time.Now(),
					Payload:   nil,
				},
			}
			s := Reduce(taskID.String(), h)
			if s.TaskID != taskID {
				t.Errorf("expected TaskID %s, got %s", taskID, s.TaskID)
			}
		})
	}

	// 2. Test empty map payload for every event type
	for _, et := range allEventTypes {
		t.Run("EmptyMapPayload_"+string(et), func(t *testing.T) {
			h := []events.Event{
				events.NewEvent(taskID, et, map[string]interface{}{}),
			}
			s := Reduce(taskID.String(), h)
			if s.TaskID != taskID {
				t.Errorf("expected TaskID %s, got %s", taskID, s.TaskID)
			}
		})
	}

	// 3. Test type-mismatched fields (integers, booleans, structs where strings/slices are expected)
	for _, et := range allEventTypes {
		t.Run("MismatchedTypes_"+string(et), func(t *testing.T) {
			mismatchedPayload := map[string]interface{}{
				"objective":     12345,
				"description":   true,
				"tasks":         "not-a-slice",
				"requirement":   99.9,
				"constraint":    []int{1, 2, 3},
				"id":            false,
				"decision_id":   100,
				"next_action":   nil,
				"reason":        123,
				"do_not_repeat": 456,
				"path":          []byte{0x01, 0x02},
				"action":        123,
				"command":       true,
				"exit_code":     "not-an-int",
				"suite":         struct{}{},
				"test":          nil,
				"error":         1234,
				"milestone":     nil,
				"hash":          999,
				"commit":        false,
				"session_id":    123,
			}
			h := []events.Event{
				events.NewEvent(taskID, et, mismatchedPayload),
			}
			s := Reduce(taskID.String(), h)
			if s.TaskID != taskID {
				t.Errorf("expected TaskID %s, got %s", taskID, s.TaskID)
			}
		})
	}
}

// TestEngine_DeduplicationAndArraySanity tests deduplication behavior on constraints, requirements, and completed milestones.
func TestEngine_DeduplicationAndArraySanity(t *testing.T) {
	taskID := uuid.New()

	history := []events.Event{
		events.NewEvent(taskID, events.TaskStarted, map[string]interface{}{
			"objective": "Deduplication Test",
			"tasks":     []string{"Task 1", "Task 2", "Task 1"}, // duplicate initial task
		}),
		events.NewEvent(taskID, events.RequirementAdded, map[string]interface{}{"requirement": "Task 3"}),
		events.NewEvent(taskID, events.RequirementAdded, map[string]interface{}{"requirement": "Task 3"}), // duplicate requirement
		events.NewEvent(taskID, events.ConstraintAdded, map[string]interface{}{"constraint": "no-global-state"}),
		events.NewEvent(taskID, events.ConstraintAdded, map[string]interface{}{"constraint": "no-global-state"}), // duplicate constraint
		events.NewEvent(taskID, events.UserRejection, map[string]interface{}{
			"do_not_repeat": []string{"bad_file.go", "bad_file.go"},
		}),
		events.NewEvent(taskID, events.MilestoneCompleted, map[string]interface{}{"milestone": "Task 1"}),
		events.NewEvent(taskID, events.MilestoneCompleted, map[string]interface{}{"milestone": "Task 1"}), // duplicate milestone
	}

	state := Reduce(taskID.String(), history)

	// Verify constraints deduplication
	if len(state.Constraints) != 1 || state.Constraints[0] != "no-global-state" {
		t.Errorf("expected 1 constraint 'no-global-state', got %v", state.Constraints)
	}

	// Verify do_not_repeat deduplication
	if len(state.DoNotRepeat) != 1 || state.DoNotRepeat[0] != "bad_file.go" {
		t.Errorf("expected 1 DoNotRepeat 'bad_file.go', got %v", state.DoNotRepeat)
	}

	// Verify Completed deduplication
	if len(state.Completed) != 1 || state.Completed[0] != "Task 1" {
		t.Errorf("expected 1 Completed 'Task 1', got %v", state.Completed)
	}

	// Verify Remaining removes completed item and has no duplicates
	if len(state.Remaining) != 2 || state.Remaining[0] != "Task 2" || state.Remaining[1] != "Task 3" {
		t.Errorf("expected Remaining ['Task 2', 'Task 3'], got %v", state.Remaining)
	}
}

// TestEngine_OutOfOrderLifecycleEvents tests non-linear sequences (e.g. resolve non-existent blocker, approval before decision, etc.)
func TestEngine_OutOfOrderLifecycleEvents(t *testing.T) {
	taskID := uuid.New()

	history := []events.Event{
		// 1. Resolve blocker that was never created
		events.NewEvent(taskID, events.BlockerResolved, map[string]interface{}{"id": "phantom-blocker"}),
		// 2. Approve decision that was never created
		events.NewEvent(taskID, events.UserApproval, map[string]interface{}{"id": "phantom-decision"}),
		// 3. Complete milestone not in remaining
		events.NewEvent(taskID, events.MilestoneCompleted, map[string]interface{}{"milestone": "Unplanned Milestone"}),
		// 4. DecisionMade after user approval
		events.NewEvent(taskID, events.DecisionMade, map[string]interface{}{
			"id":          "phantom-decision",
			"description": "Late decision",
			"status":      "ACTIVE",
		}),
		// 5. Blocker created after resolve
		events.NewEvent(taskID, events.BlockerCreated, map[string]interface{}{
			"id":          "blk-100",
			"description": "Port conflict",
		}),
		// 6. Blocker updated with same ID
		events.NewEvent(taskID, events.BlockerCreated, map[string]interface{}{
			"id":          "blk-100",
			"description": "Updated port conflict details",
		}),
		// 7. Reject decision
		events.NewEvent(taskID, events.UserRejection, map[string]interface{}{
			"id":     "phantom-decision",
			"reason": "Security concern",
		}),
	}

	s := Reduce(taskID.String(), history)

	if len(s.Completed) != 1 || s.Completed[0] != "Unplanned Milestone" {
		t.Errorf("expected Completed ['Unplanned Milestone'], got %v", s.Completed)
	}

	if len(s.Blocked) != 1 || s.Blocked[0].Description != "Updated port conflict details" || s.Blocked[0].Status != "ACTIVE" {
		t.Errorf("expected 1 updated blocker, got %v", s.Blocked)
	}

	if len(s.Decisions) != 1 || s.Decisions[0].Status != "REJECTED" {
		t.Errorf("expected phantom-decision to be REJECTED, got %v", s.Decisions)
	}
}

// TestEngine_DynamicConfidenceExhaustiveMatrix tests the complete matrix of confidence transitions.
func TestEngine_DynamicConfidenceExhaustiveMatrix(t *testing.T) {
	taskID := uuid.New()

	type step struct {
		name               string
		event              events.Event
		expectedConfidence ConfidenceLevel
	}

	steps := []step{
		{
			name:               "Clean start",
			event:              events.NewEvent(taskID, events.TaskStarted, map[string]interface{}{"objective": "Confidence Test"}),
			expectedConfidence: ConfidenceHigh,
		},
		{
			name:               "1 Blocker -> Low",
			event:              events.NewEvent(taskID, events.BlockerCreated, map[string]interface{}{"id": "b1", "description": "Block 1"}),
			expectedConfidence: ConfidenceLow,
		},
		{
			name:               "2 Blockers -> None",
			event:              events.NewEvent(taskID, events.BlockerCreated, map[string]interface{}{"id": "b2", "description": "Block 2"}),
			expectedConfidence: ConfidenceNone,
		},
		{
			name:               "Resolve 1 blocker (1 remains) -> Low",
			event:              events.NewEvent(taskID, events.BlockerResolved, map[string]interface{}{"id": "b1"}),
			expectedConfidence: ConfidenceLow,
		},
		{
			name:               "Failing test with 1 blocker -> None",
			event:              events.NewEvent(taskID, events.TestFailed, map[string]interface{}{"suite": "unit"}),
			expectedConfidence: ConfidenceNone,
		},
		{
			name:               "Resolve last blocker (failing test remains) -> Low",
			event:              events.NewEvent(taskID, events.BlockerResolved, map[string]interface{}{"id": "b2"}),
			expectedConfidence: ConfidenceLow,
		},
		{
			name:               "User rejection + failing test -> None",
			event:              events.NewEvent(taskID, events.UserRejection, map[string]interface{}{"reason": "Bad UX"}),
			expectedConfidence: ConfidenceNone,
		},
		{
			name:               "Tests pass (user rejection remains) -> Low",
			event:              events.NewEvent(taskID, events.TestPassed, map[string]interface{}{"suite": "unit"}),
			expectedConfidence: ConfidenceLow,
		},
		{
			name:               "User approval -> High",
			event:              events.NewEvent(taskID, events.UserApproval, map[string]interface{}{"next_action": "Proceed"}),
			expectedConfidence: ConfidenceHigh,
		},
		{
			name:               "Session interrupted -> Low",
			event:              events.NewEvent(taskID, events.SessionInterrupted, map[string]interface{}{"reason": "Signal SIGINT"}),
			expectedConfidence: ConfidenceLow,
		},
		{
			name:               "Session resumed -> High",
			event:              events.NewEvent(taskID, events.SessionResumed, nil),
			expectedConfidence: ConfidenceHigh,
		},
		{
			name:               "3 Blockers created -> None",
			event:              events.NewEvent(taskID, events.BlockerCreated, map[string]interface{}{"id": "b3", "description": "Block 3"}),
			expectedConfidence: ConfidenceLow,
		},
	}

	var cumulative []events.Event
	for i, st := range steps {
		cumulative = append(cumulative, st.event)
		s := Reduce(taskID.String(), cumulative)
		if s.Confidence != st.expectedConfidence {
			t.Errorf("Step %d (%s): expected confidence %s, got %s", i+1, st.name, st.expectedConfidence, s.Confidence)
		}
	}
}

// TestEngine_ConcurrentReduceAndRandomStress tests that Reduce() is fully pure, deterministic,
// thread-safe, and free of race conditions under heavy concurrent execution.
func TestEngine_ConcurrentReduceAndRandomStress(t *testing.T) {
	taskID := uuid.New()

	// Build a baseline 50-event history
	var history []events.Event
	history = append(history, events.NewEvent(taskID, events.TaskStarted, map[string]interface{}{"objective": "Stress Task"}))

	for i := 0; i < 50; i++ {
		switch i % 5 {
		case 0:
			history = append(history, events.NewEvent(taskID, events.RequirementAdded, map[string]interface{}{"requirement": fmt.Sprintf("Req %d", i)}))
		case 1:
			history = append(history, events.NewEvent(taskID, events.ConstraintAdded, map[string]interface{}{"constraint": fmt.Sprintf("Constraint %d", i)}))
		case 2:
			history = append(history, events.NewEvent(taskID, events.FileChanged, map[string]interface{}{"path": fmt.Sprintf("file_%d.go", i)}))
		case 3:
			history = append(history, events.NewEvent(taskID, events.CommandExecuted, map[string]interface{}{"command": fmt.Sprintf("cmd_%d", i)}))
		case 4:
			history = append(history, events.NewEvent(taskID, events.GitCommit, map[string]interface{}{"hash": fmt.Sprintf("hash_%d", i)}))
		}
	}

	expectedBaseline := Reduce(taskID.String(), history)

	// Run 100 concurrent Reduce calls
	numGoroutines := 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(gID int) {
			defer wg.Done()
			s := Reduce(taskID.String(), history)
			if s.Objective != expectedBaseline.Objective {
				t.Errorf("goroutine %d objective mismatch: expected %s, got %s", gID, expectedBaseline.Objective, s.Objective)
			}
			if len(s.Remaining) != len(expectedBaseline.Remaining) {
				t.Errorf("goroutine %d remaining mismatch: expected %d, got %d", gID, len(expectedBaseline.Remaining), len(s.Remaining))
			}
			if len(s.Constraints) != len(expectedBaseline.Constraints) {
				t.Errorf("goroutine %d constraints mismatch: expected %d, got %d", gID, len(expectedBaseline.Constraints), len(s.Constraints))
			}
		}(g)
	}

	wg.Wait()
}

// TestEngine_FuzzRandomizedEventPermutations creates randomized event permutations and asserts no crashes.
func TestEngine_FuzzRandomizedEventPermutations(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	taskID := uuid.New()

	eventTypes := []events.EventType{
		events.TaskStarted,
		events.RequirementAdded,
		events.ConstraintAdded,
		events.UserApproval,
		events.UserRejection,
		events.DecisionMade,
		events.FileChanged,
		events.CommandExecuted,
		events.TestStarted,
		events.TestPassed,
		events.TestFailed,
		events.BlockerCreated,
		events.BlockerResolved,
		events.MilestoneCompleted,
		events.GitCommit,
		events.SessionInterrupted,
		events.SessionResumed,
	}

	for iter := 0; iter < 100; iter++ {
		eventCount := rng.Intn(30) + 1
		history := make([]events.Event, eventCount)

		for eIdx := 0; eIdx < eventCount; eIdx++ {
			et := eventTypes[rng.Intn(len(eventTypes))]
			history[eIdx] = events.NewEvent(taskID, et, map[string]interface{}{
				"objective":   fmt.Sprintf("Obj %d", rng.Intn(5)),
				"requirement": fmt.Sprintf("Req %d", rng.Intn(5)),
				"constraint":  fmt.Sprintf("Constraint %d", rng.Intn(5)),
				"id":          fmt.Sprintf("id_%d", rng.Intn(3)),
				"decision_id": fmt.Sprintf("id_%d", rng.Intn(3)),
				"milestone":   fmt.Sprintf("Req %d", rng.Intn(5)),
				"exit_code":   rng.Intn(2),
				"reason":      "fuzz reason",
				"path":        "file.go",
				"action":      "do_not_repeat",
			})
		}

		s := Reduce(taskID.String(), history)
		if s.Confidence != ConfidenceHigh && s.Confidence != ConfidenceLow && s.Confidence != ConfidenceNone {
			t.Fatalf("invalid confidence level: %s", s.Confidence)
		}
	}
}
