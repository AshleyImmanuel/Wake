package state

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/AshleyImmanuel/Wake/internal/events"
)

func TestReduceEventTypes(t *testing.T) {
	taskID := uuid.New()
	history := []events.Event{
		{
			Type: events.TaskStarted,
			Payload: map[string]interface{}{
				"objective": "Build a feature",
				"tasks":     []string{"task1", "task2"},
				"current":   "Planning",
			},
		},
		{
			Type: events.RequirementAdded,
			Payload: map[string]interface{}{
				"requirement": "Must be fast",
			},
		},
		{
			Type: events.RequirementAdded,
			Payload: map[string]interface{}{
				"requirement": "Must be fast", // Duplicate
			},
		},
		{
			Type: events.ConstraintAdded,
			Payload: map[string]interface{}{
				"constraint": "No external DB",
			},
		},
		{
			Type: events.ConstraintAdded,
			Payload: map[string]interface{}{
				"constraint": "No external DB", // Duplicate
			},
		},
		{
			Type: events.DecisionMade,
			Payload: map[string]interface{}{
				"id":          "dec1",
				"description": "Use SQLite",
				"source":      "Agent",
			},
		},
		{
			Type: events.DecisionMade,
			Payload: map[string]interface{}{
				"id":          "dec1",
				"description": "Use SQLite (Updated)",
				"source":      "Agent",
			},
		},
		{
			Type: events.UserApproval,
			Payload: map[string]interface{}{
				"id":          "dec1",
				"next_action": "Implement DB",
			},
		},
		{
			Type: events.DecisionMade,
			Payload: map[string]interface{}{
				"id":          "dec2",
				"description": "Use Mongo",
			},
		},
		{
			Type: events.UserRejection,
			Payload: map[string]interface{}{
				"id":            "dec2",
				"reason":        "Too heavy",
				"do_not_repeat": []string{"mongo.go"},
			},
		},
		{
			Type: events.FileChanged,
			Payload: map[string]interface{}{
				"path":   "main.go",
				"action": "do_not_repeat",
			},
		},
		{
			Type: events.CommandExecuted,
			Payload: map[string]interface{}{
				"command":   "go build",
				"exit_code": 1,
			},
		},
		{
			Type: events.TestStarted,
			Payload: map[string]interface{}{
				"suite": "unit-tests",
			},
		},
		{
			Type: events.TestPassed,
			Payload: map[string]interface{}{
				"suite":       "unit-tests",
				"next_action": "Deploy",
			},
		},
		{
			Type: events.TestFailed,
			Payload: map[string]interface{}{
				"suite": "e2e-tests",
				"error": "timeout",
			},
		},
		{
			Type: events.BlockerCreated,
			Payload: map[string]interface{}{
				"id":          "blk1",
				"description": "API down",
			},
		},
		{
			Type: events.BlockerCreated,
			Payload: map[string]interface{}{
				"id":          "blk1",
				"description": "API really down", // Upsert
			},
		},
		{
			Type: events.BlockerResolved,
			Payload: map[string]interface{}{
				"id": "blk1",
			},
		},
		{
			Type: events.MilestoneCompleted,
			Payload: map[string]interface{}{
				"milestone": "task1",
			},
		},
		{
			Type: events.GitCommit,
			Payload: map[string]interface{}{
				"hash": "abcdef",
			},
		},
		{
			Type: events.SessionInterrupted,
			Payload: map[string]interface{}{
				"reason": "OOM",
			},
		},
		{
			Type: events.SessionResumed,
			Payload: map[string]interface{}{},
		},
	}

	state := Reduce(taskID.String(), history)

	// Asserts
	if state.Objective != "Build a feature" {
		t.Errorf("unexpected objective: %s", state.Objective)
	}
	if !reflect.DeepEqual(state.Remaining, []string{"task2", "Must be fast"}) {
		t.Errorf("unexpected remaining: %v", state.Remaining)
	}
	if !reflect.DeepEqual(state.Constraints, []string{"No external DB"}) {
		t.Errorf("unexpected constraints: %v", state.Constraints)
	}
	if len(state.Decisions) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(state.Decisions))
	}
	if state.Decisions[0].ID != "dec1" || state.Decisions[0].Status != "ACTIVE" || state.Decisions[0].Description != "Use SQLite (Updated)" {
		t.Errorf("unexpected dec1: %+v", state.Decisions[0])
	}
	if state.Decisions[1].ID != "dec2" || state.Decisions[1].Status != "REJECTED" {
		t.Errorf("unexpected dec2: %+v", state.Decisions[1])
	}
	if !reflect.DeepEqual(state.DoNotRepeat, []string{"mongo.go", "main.go"}) {
		t.Errorf("unexpected do not repeat: %v", state.DoNotRepeat)
	}
	if len(state.Blocked) != 1 || state.Blocked[0].Status != "RESOLVED" || state.Blocked[0].Description != "API really down" {
		t.Errorf("unexpected blockers: %+v", state.Blocked)
	}
	if !reflect.DeepEqual(state.Completed, []string{"task1"}) {
		t.Errorf("unexpected completed: %v", state.Completed)
	}
	if state.LastVerified != "abcdef" {
		t.Errorf("unexpected last verified: %s", state.LastVerified)
	}
	if state.Current != "Session resumed" {
		t.Errorf("unexpected current: %s", state.Current)
	}
}

func TestDynamicConfidenceCalculation(t *testing.T) {
	tests := []struct {
		name     string
		history  []events.Event
		expected ConfidenceLevel
	}{
		{
			name:     "ConfidenceHigh_Empty",
			history:  []events.Event{},
			expected: ConfidenceHigh,
		},
		{
			name: "ConfidenceHigh_OnlySessionEvents",
			history: []events.Event{
				{Type: events.SessionInterrupted},
				{Type: events.SessionResumed},
			},
			expected: ConfidenceHigh,
		},
		{
			name: "ConfidenceHigh_Clean",
			history: []events.Event{
				{Type: events.TestPassed},
				{Type: events.UserApproval},
			},
			expected: ConfidenceHigh,
		},
		{
			name: "ConfidenceLow_1Blocker",
			history: []events.Event{
				{Type: events.BlockerCreated, Payload: map[string]interface{}{"id": "1", "description": "b", "status": "ACTIVE"}},
			},
			expected: ConfidenceLow,
		},
		{
			name: "ConfidenceLow_FailingTest",
			history: []events.Event{
				{Type: events.TestFailed},
			},
			expected: ConfidenceLow,
		},
		{
			name: "ConfidenceLow_UserRejection",
			history: []events.Event{
				{Type: events.UserRejection},
			},
			expected: ConfidenceLow,
		},
		{
			name: "ConfidenceLow_SessionInterrupted",
			history: []events.Event{
				{Type: events.SessionInterrupted},
			},
			expected: ConfidenceLow,
		},
		{
			name: "ConfidenceNone_2Blockers",
			history: []events.Event{
				{Type: events.BlockerCreated, Payload: map[string]interface{}{"id": "1", "description": "b1", "status": "ACTIVE"}},
				{Type: events.BlockerCreated, Payload: map[string]interface{}{"id": "2", "description": "b2", "status": "ACTIVE"}},
			},
			expected: ConfidenceNone,
		},
		{
			name: "ConfidenceNone_BlockerAndFailingTest",
			history: []events.Event{
				{Type: events.BlockerCreated, Payload: map[string]interface{}{"id": "1", "description": "b1", "status": "ACTIVE"}},
				{Type: events.TestFailed},
			},
			expected: ConfidenceNone,
		},
		{
			name: "ConfidenceNone_RejectionAndFailingTest",
			history: []events.Event{
				{Type: events.UserRejection},
				{Type: events.TestFailed},
			},
			expected: ConfidenceNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := Reduce(uuid.NewString(), tt.history)
			if state.Confidence != tt.expected {
				t.Errorf("expected confidence %s, got %s", tt.expected, state.Confidence)
			}
		})
	}
}

func TestStateClone(t *testing.T) {
	state := State{
		Constraints: []string{"C1"},
		Decisions:   []Decision{{ID: "D1"}},
		Completed:   []string{"M1"},
		Remaining:   []string{"R1"},
		Blocked:     []Blocker{{ID: "B1"}},
		DoNotRepeat: []string{"F1"},
	}

	cloned := state.Clone()

	// Modify cloned
	cloned.Constraints[0] = "C2"
	cloned.Decisions[0].ID = "D2"
	cloned.Completed[0] = "M2"
	cloned.Remaining[0] = "R2"
	cloned.Blocked[0].ID = "B2"
	cloned.DoNotRepeat[0] = "F2"

	// Original should be unchanged
	if state.Constraints[0] != "C1" {
		t.Errorf("Constraints was modified: %v", state.Constraints)
	}
	if state.Decisions[0].ID != "D1" {
		t.Errorf("Decisions was modified: %v", state.Decisions)
	}
	if state.Completed[0] != "M1" {
		t.Errorf("Completed was modified: %v", state.Completed)
	}
	if state.Remaining[0] != "R1" {
		t.Errorf("Remaining was modified: %v", state.Remaining)
	}
	if state.Blocked[0].ID != "B1" {
		t.Errorf("Blocked was modified: %v", state.Blocked)
	}
	if state.DoNotRepeat[0] != "F1" {
		t.Errorf("DoNotRepeat was modified: %v", state.DoNotRepeat)
	}
}

func TestHelpers(t *testing.T) {
	m := map[string]interface{}{
		"str":         " val ",
		"strSlice":    []string{"a", "b"},
		"ifaceSlice":  []interface{}{"c", 1, "d", ""},
		"int":         42,
		"int64":       int64(43),
		"float64":     float64(44),
	}

	// getString
	if got := getString(m, "missing", "str"); got != "val" {
		t.Errorf("getString expected 'val', got '%s'", got)
	}
	if got := getString(nil, "str"); got != "" {
		t.Errorf("getString with nil expected '', got '%s'", got)
	}

	// getStringSlice
	s1 := getStringSlice(m, "missing", "strSlice")
	if !reflect.DeepEqual(s1, []string{"a", "b"}) {
		t.Errorf("getStringSlice expected [a, b], got %v", s1)
	}
	s2 := getStringSlice(m, "ifaceSlice")
	if !reflect.DeepEqual(s2, []string{"c", "d"}) {
		t.Errorf("getStringSlice expected [c, d], got %v", s2)
	}

	// getInt
	i1, ok1 := getInt(m, "missing", "int")
	if i1 != 42 || !ok1 {
		t.Errorf("getInt expected 42, true; got %d, %v", i1, ok1)
	}
	i2, ok2 := getInt(m, "int64")
	if i2 != 43 || !ok2 {
		t.Errorf("getInt expected 43, true; got %d, %v", i2, ok2)
	}
	i3, ok3 := getInt(m, "float64")
	if i3 != 44 || !ok3 {
		t.Errorf("getInt expected 44, true; got %d, %v", i3, ok3)
	}
	i4, ok4 := getInt(m, "missing")
	if i4 != 0 || ok4 {
		t.Errorf("getInt expected 0, false; got %d, %v", i4, ok4)
	}

	// containsString
	if !containsString([]string{"a", "b"}, "b") {
		t.Error("containsString should be true")
	}
	if containsString([]string{"a", "b"}, "c") {
		t.Error("containsString should be false")
	}

	// removeString
	removed := removeString([]string{"a", "b", "c"}, "b")
	if !reflect.DeepEqual(removed, []string{"a", "c"}) {
		t.Errorf("removeString expected [a, c], got %v", removed)
	}
}

func TestTaskIDPopulationPriority(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()
	id3 := uuid.New()

	// 1. From parameter
	s1 := Reduce(id1.String(), []events.Event{
		{TaskID: id2, Type: events.TaskStarted, Payload: map[string]interface{}{"task_id": id3.String()}},
	})
	if s1.TaskID != id1 {
		t.Errorf("expected parameter ID %s, got %s", id1, s1.TaskID)
	}

	// 2. From event envelope
	s2 := Reduce("", []events.Event{
		{TaskID: id2, Type: events.TaskStarted, Payload: map[string]interface{}{"task_id": id3.String()}},
	})
	if s2.TaskID != id2 {
		t.Errorf("expected envelope ID %s, got %s", id2, s2.TaskID)
	}

	// 3. From TaskStarted payload
	s3 := Reduce("", []events.Event{
		{Type: events.TaskStarted, Payload: map[string]interface{}{"task_id": id3.String()}},
	})
	if s3.TaskID != id3 {
		t.Errorf("expected payload ID %s, got %s", id3, s3.TaskID)
	}
}
