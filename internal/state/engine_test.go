package state

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/wake/wake/internal/events"
)

func TestReduce_TaskStarted(t *testing.T) {
	taskID := uuid.New()
	history := []events.Event{
		{
			ID:        uuid.New(),
			TaskID:    taskID,
			Type:      events.TaskStarted,
			Timestamp: time.Now(),
			Payload: map[string]interface{}{
				"objective": "Migrate from MongoDB to PostgreSQL",
			},
		},
	}

	result := Reduce(taskID.String(), history)

	if result.Objective != "Migrate from MongoDB to PostgreSQL" {
		t.Errorf("Expected objective to be set, got: %s", result.Objective)
	}
}

func TestReduce_BlockerLifecycle(t *testing.T) {
	taskID := uuid.New()
	history := []events.Event{
		{
			ID:        uuid.New(),
			TaskID:    taskID,
			Type:      events.BlockerCreated,
			Payload: map[string]interface{}{
				"id":          "b-1",
				"description": "Stripe webhook failing",
			},
		},
	}

	// 1. Check Active Blocker
	result := Reduce(taskID.String(), history)
	if len(result.Blocked) != 1 || result.Blocked[0].Status != "ACTIVE" {
		t.Errorf("Expected 1 ACTIVE blocker, got: %+v", result.Blocked)
	}

	// 2. Add Resolution Event
	history = append(history, events.Event{
		ID:        uuid.New(),
		TaskID:    taskID,
		Type:      events.BlockerResolved,
		Payload: map[string]interface{}{
			"id": "b-1",
		},
	})

	// 3. Check Resolved Blocker
	result = Reduce(taskID.String(), history)
	if len(result.Blocked) != 1 || result.Blocked[0].Status != "RESOLVED" {
		t.Errorf("Expected 1 RESOLVED blocker, got: %+v", result.Blocked)
	}
}

func TestReduce_MilestoneAndDecision(t *testing.T) {
	taskID := uuid.New()
	history := []events.Event{
		{
			Type: events.MilestoneCompleted,
			Payload: map[string]interface{}{
				"milestone": "Customer table migrated",
			},
		},
		{
			Type: events.DecisionMade,
			Payload: map[string]interface{}{
				"id":          "d-1",
				"description": "Keep Auth unchanged",
				"source":      "Developer",
			},
		},
	}

	result := Reduce(taskID.String(), history)

	if len(result.Completed) != 1 || result.Completed[0] != "Customer table migrated" {
		t.Errorf("Expected milestone to be recorded")
	}

	if len(result.Decisions) != 1 || result.Decisions[0].Description != "Keep Auth unchanged" {
		t.Errorf("Expected decision to be recorded")
	}
}

func TestReduce_All17EventTypes(t *testing.T) {
	taskID := uuid.New()
	history := []events.Event{
		events.NewEvent(taskID, events.TaskStarted, map[string]interface{}{
			"objective": "Build Sentinel MVP",
			"remaining": []string{"Task 1", "Task 2"},
			"current":   "Starting project",
		}),
		events.NewEvent(taskID, events.RequirementAdded, map[string]interface{}{
			"requirement": "Task 3",
		}),
		events.NewEvent(taskID, events.ConstraintAdded, map[string]interface{}{
			"constraint": "internal/legacy/*",
		}),
		events.NewEvent(taskID, events.DecisionMade, map[string]interface{}{
			"id":          "dec-1",
			"description": "Use SQLite WAL mode",
			"source":      "Architect",
		}),
		events.NewEvent(taskID, events.UserApproval, map[string]interface{}{
			"id":          "dec-1",
			"next_action": "Implement migrations",
		}),
		events.NewEvent(taskID, events.FileChanged, map[string]interface{}{
			"path":   "internal/db/db.go",
			"action": "modify",
		}),
		events.NewEvent(taskID, events.CommandExecuted, map[string]interface{}{
			"command":   "go build ./...",
			"exit_code": 0,
		}),
		events.NewEvent(taskID, events.TestStarted, map[string]interface{}{
			"suite": "internal/db",
		}),
		events.NewEvent(taskID, events.TestPassed, map[string]interface{}{
			"suite": "internal/db",
		}),
		events.NewEvent(taskID, events.BlockerCreated, map[string]interface{}{
			"id":          "blk-1",
			"description": "Port 8080 collision",
		}),
		events.NewEvent(taskID, events.BlockerResolved, map[string]interface{}{
			"id": "blk-1",
		}),
		events.NewEvent(taskID, events.MilestoneCompleted, map[string]interface{}{
			"milestone": "Task 1",
		}),
		events.NewEvent(taskID, events.GitCommit, map[string]interface{}{
			"hash": "commitabcdef123456",
		}),
		events.NewEvent(taskID, events.DecisionMade, map[string]interface{}{
			"id":          "dec-2",
			"description": "Delete auth submodule",
		}),
		events.NewEvent(taskID, events.UserRejection, map[string]interface{}{
			"id":            "dec-2",
			"reason":        "Auth is required for enterprise",
			"do_not_repeat": "auth/legacy.go",
		}),
		events.NewEvent(taskID, events.SessionInterrupted, map[string]interface{}{
			"reason": "Process killed by OS",
		}),
		events.NewEvent(taskID, events.SessionResumed, map[string]interface{}{
			"session_id": "sess-123",
		}),
	}

	state := Reduce(taskID.String(), history)

	if state.TaskID != taskID {
		t.Errorf("expected TaskID %s, got %s", taskID, state.TaskID)
	}
	if state.Objective != "Build Sentinel MVP" {
		t.Errorf("expected objective 'Build Sentinel MVP', got %s", state.Objective)
	}
	if len(state.Remaining) != 2 || state.Remaining[0] != "Task 2" || state.Remaining[1] != "Task 3" {
		t.Errorf("expected remaining ['Task 2', 'Task 3'] (Task 1 completed), got %v", state.Remaining)
	}
	if len(state.Completed) != 1 || state.Completed[0] != "Task 1" {
		t.Errorf("expected completed ['Task 1'], got %v", state.Completed)
	}
	if len(state.Constraints) != 1 || state.Constraints[0] != "internal/legacy/*" {
		t.Errorf("expected constraints ['internal/legacy/*'], got %v", state.Constraints)
	}
	if len(state.Decisions) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(state.Decisions))
	}
	if state.Decisions[0].Status != "ACTIVE" {
		t.Errorf("expected dec-1 status ACTIVE, got %s", state.Decisions[0].Status)
	}
	if state.Decisions[1].Status != "REJECTED" {
		t.Errorf("expected dec-2 status REJECTED, got %s", state.Decisions[1].Status)
	}
	if len(state.DoNotRepeat) != 1 || state.DoNotRepeat[0] != "auth/legacy.go" {
		t.Errorf("expected DoNotRepeat ['auth/legacy.go'], got %v", state.DoNotRepeat)
	}
	if len(state.Blocked) != 1 || state.Blocked[0].Status != "RESOLVED" {
		t.Errorf("expected Blocker blk-1 RESOLVED, got %v", state.Blocked)
	}
	if state.LastVerified != "commitabcdef123456" {
		t.Errorf("expected LastVerified 'commitabcdef123456', got %s", state.LastVerified)
	}
	if state.Current != "Session resumed" {
		t.Errorf("expected Current 'Session resumed', got %s", state.Current)
	}
}

func TestReduce_TaskIDResolution(t *testing.T) {
	taskID := uuid.New()

	// Case 1: Populated from function parameter
	s1 := Reduce(taskID.String(), nil)
	if s1.TaskID != taskID {
		t.Errorf("expected TaskID from parameter %s, got %s", taskID, s1.TaskID)
	}

	// Case 2: Empty parameter, populated from event envelope
	ev := events.Event{
		ID:     uuid.New(),
		TaskID: taskID,
		Type:   events.TaskStarted,
		Payload: map[string]interface{}{
			"objective": "Envelope Task",
		},
	}
	s2 := Reduce("", []events.Event{ev})
	if s2.TaskID != taskID {
		t.Errorf("expected TaskID from event envelope %s, got %s", taskID, s2.TaskID)
	}

	// Case 3: Empty parameter & empty event envelope, populated from payload
	evPayload := events.Event{
		ID:     uuid.New(),
		TaskID: uuid.Nil,
		Type:   events.TaskStarted,
		Payload: map[string]interface{}{
			"task_id":   taskID.String(),
			"objective": "Payload Task",
		},
	}
	s3 := Reduce("", []events.Event{evPayload})
	if s3.TaskID != taskID {
		t.Errorf("expected TaskID from payload %s, got %s", taskID, s3.TaskID)
	}
}

func TestReduce_DynamicConfidenceTransitions(t *testing.T) {
	taskID := uuid.New()

	// 1. Initial clean state -> High
	h := []events.Event{
		events.NewEvent(taskID, events.TaskStarted, map[string]interface{}{"objective": "Test"}),
	}
	if s := Reduce(taskID.String(), h); s.Confidence != ConfidenceHigh {
		t.Errorf("expected ConfidenceHigh, got %s", s.Confidence)
	}

	// 2. 1 Active Blocker -> Low
	h = append(h, events.NewEvent(taskID, events.BlockerCreated, map[string]interface{}{"id": "b1", "description": "Blocker 1"}))
	if s := Reduce(taskID.String(), h); s.Confidence != ConfidenceLow {
		t.Errorf("expected ConfidenceLow on 1 blocker, got %s", s.Confidence)
	}

	// 3. 2 Active Blockers -> None
	h = append(h, events.NewEvent(taskID, events.BlockerCreated, map[string]interface{}{"id": "b2", "description": "Blocker 2"}))
	if s := Reduce(taskID.String(), h); s.Confidence != ConfidenceNone {
		t.Errorf("expected ConfidenceNone on 2 blockers, got %s", s.Confidence)
	}

	// 4. Resolve 1 blocker -> Low (1 active blocker remains)
	h = append(h, events.NewEvent(taskID, events.BlockerResolved, map[string]interface{}{"id": "b1"}))
	if s := Reduce(taskID.String(), h); s.Confidence != ConfidenceLow {
		t.Errorf("expected ConfidenceLow on 1 remaining blocker, got %s", s.Confidence)
	}

	// 5. Test failure with active blocker -> None
	h = append(h, events.NewEvent(taskID, events.TestFailed, map[string]interface{}{"suite": "pkg/auth"}))
	if s := Reduce(taskID.String(), h); s.Confidence != ConfidenceNone {
		t.Errorf("expected ConfidenceNone on blocker + test failure, got %s", s.Confidence)
	}

	// 6. Resolve all blockers -> Low (test failure still active)
	h = append(h, events.NewEvent(taskID, events.BlockerResolved, map[string]interface{}{"id": "b2"}))
	if s := Reduce(taskID.String(), h); s.Confidence != ConfidenceLow {
		t.Errorf("expected ConfidenceLow on test failure alone, got %s", s.Confidence)
	}

	// 7. Tests pass -> High
	h = append(h, events.NewEvent(taskID, events.TestPassed, map[string]interface{}{"suite": "pkg/auth"}))
	if s := Reduce(taskID.String(), h); s.Confidence != ConfidenceHigh {
		t.Errorf("expected ConfidenceHigh after tests pass, got %s", s.Confidence)
	}

	// 8. Session interrupted -> Low
	h = append(h, events.NewEvent(taskID, events.SessionInterrupted, map[string]interface{}{"reason": "crash"}))
	if s := Reduce(taskID.String(), h); s.Confidence != ConfidenceLow {
		t.Errorf("expected ConfidenceLow on session interruption, got %s", s.Confidence)
	}

	// 9. Session resumed -> High
	h = append(h, events.NewEvent(taskID, events.SessionResumed, nil))
	if s := Reduce(taskID.String(), h); s.Confidence != ConfidenceHigh {
		t.Errorf("expected ConfidenceHigh after session resumption, got %s", s.Confidence)
	}
}

func TestReduce_CommandFailedExitCode(t *testing.T) {
	taskID := uuid.New()
	history := []events.Event{
		events.NewEvent(taskID, events.CommandExecuted, map[string]interface{}{
			"command":   "make build",
			"exit_code": 1,
		}),
	}

	s := Reduce(taskID.String(), history)
	if s.NextAction != "Investigate failed command: make build" {
		t.Errorf("expected NextAction 'Investigate failed command: make build', got %s", s.NextAction)
	}
}

func TestState_Clone(t *testing.T) {
	s := State{
		TaskID:      uuid.New(),
		Objective:   "Original Objective",
		Constraints: []string{"c1", "c2"},
		Decisions: []Decision{
			{ID: "d1", Description: "Original Decision", Status: "ACTIVE"},
		},
		Completed:   []string{"m1"},
		Remaining:   []string{"r1"},
		Blocked:     []Blocker{{ID: "b1", Description: "Original Blocker", Status: "ACTIVE"}},
		DoNotRepeat: []string{"dnr1"},
		Confidence:  ConfidenceHigh,
	}

	cloned := s.Clone()

	// Mutate original slices
	s.Constraints[0] = "mutated_c1"
	s.Decisions[0].Description = "mutated_d1"
	s.Completed[0] = "mutated_m1"
	s.Remaining[0] = "mutated_r1"
	s.Blocked[0].Description = "mutated_b1"
	s.DoNotRepeat[0] = "mutated_dnr1"

	// Verify clone remains unchanged
	if cloned.Constraints[0] != "c1" {
		t.Errorf("expected cloned constraint 'c1', got %s", cloned.Constraints[0])
	}
	if cloned.Decisions[0].Description != "Original Decision" {
		t.Errorf("expected cloned decision 'Original Decision', got %s", cloned.Decisions[0].Description)
	}
	if cloned.Completed[0] != "m1" {
		t.Errorf("expected cloned completed 'm1', got %s", cloned.Completed[0])
	}
	if cloned.Remaining[0] != "r1" {
		t.Errorf("expected cloned remaining 'r1', got %s", cloned.Remaining[0])
	}
	if cloned.Blocked[0].Description != "Original Blocker" {
		t.Errorf("expected cloned blocker 'Original Blocker', got %s", cloned.Blocked[0].Description)
	}
	if cloned.DoNotRepeat[0] != "dnr1" {
		t.Errorf("expected cloned DoNotRepeat 'dnr1', got %s", cloned.DoNotRepeat[0])
	}
}

func TestReduce_PayloadDefensiveCloning(t *testing.T) {
	taskID := uuid.New()
	rawPayload := map[string]interface{}{
		"objective": "Thread Safety Test",
		"tasks":     []interface{}{"task A", "task B"},
	}

	ev := events.NewEvent(taskID, events.TaskStarted, rawPayload)

	// Mutate rawPayload after event creation
	rawPayload["objective"] = "Mutated Objective"
	if ev.Payload["objective"] == "Mutated Objective" {
		t.Fatalf("expected Event.Payload to be defensively cloned and immutable to outside mutations")
	}

	// Verify State.Reduce output
	s := Reduce(taskID.String(), []events.Event{ev})
	if s.Objective != "Thread Safety Test" {
		t.Errorf("expected objective 'Thread Safety Test', got %s", s.Objective)
	}
}
