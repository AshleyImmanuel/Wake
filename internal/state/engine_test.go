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
