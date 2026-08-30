package state

import (
	"reflect"
	"testing"
)

func TestDiffStates(t *testing.T) {
	oldState := State{
		Current:    "Doing A",
		NextAction: "Do B",
		Completed:  []string{"Task 1", "Task 2"},
		Blocked: []Blocker{
			{ID: "B1", Description: "Need DB", Status: "ACTIVE"},
			{ID: "B2", Description: "Need API", Status: "RESOLVED"},
		},
	}

	newState := State{
		Current:    "Doing B",
		NextAction: "Do C",
		Completed:  []string{"Task 1", "Task 2", "Task 3"},
		Blocked: []Blocker{
			{ID: "B2", Description: "Need API", Status: "RESOLVED"},
			{ID: "B3", Description: "Need Auth", Status: "ACTIVE"},
		},
	}

	diff := DiffStates(oldState, newState)

	if diff.CurrentOld != "Doing A" || diff.CurrentNew != "Doing B" {
		t.Errorf("Current diff incorrect")
	}
	if diff.NextActionOld != "Do B" || diff.NextActionNew != "Do C" {
		t.Errorf("NextAction diff incorrect")
	}

	expectedAdded := []string{"Task 3"}
	if !reflect.DeepEqual(diff.CompletedAdded, expectedAdded) {
		t.Errorf("CompletedAdded incorrect, got %v", diff.CompletedAdded)
	}
	if len(diff.CompletedRemoved) > 0 {
		t.Errorf("CompletedRemoved should be empty")
	}

	if len(diff.BlockedAdded) != 1 || diff.BlockedAdded[0].ID != "B3" {
		t.Errorf("BlockedAdded incorrect")
	}
	if len(diff.BlockedRemoved) != 1 || diff.BlockedRemoved[0].ID != "B1" {
		t.Errorf("BlockedRemoved incorrect")
	}
}
