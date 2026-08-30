package state

import (
	"github.com/google/uuid"
)

func handleTaskStarted(currentState *State, payload map[string]interface{}) {
	if obj := getString(payload, "objective", "description"); obj != "" {
		currentState.Objective = obj
	}
	if currentState.TaskID == uuid.Nil {
		if tidStr := getString(payload, "task_id", "id"); tidStr != "" {
			if parsed, err := uuid.Parse(tidStr); err == nil {
				currentState.TaskID = parsed
			}
		}
	}
	if tasks := getStringSlice(payload, "tasks", "remaining", "initial_tasks"); len(tasks) > 0 {
		for _, t := range tasks {
			if !containsString(currentState.Remaining, t) {
				currentState.Remaining = append(currentState.Remaining, t)
			}
		}
	}
	if curr := getString(payload, "current"); curr != "" {
		currentState.Current = curr
	}
}

func handleRequirementAdded(currentState *State, payload map[string]interface{}) {
	if req := getString(payload, "requirement", "description", "name"); req != "" {
		if !containsString(currentState.Remaining, req) {
			currentState.Remaining = append(currentState.Remaining, req)
		}
	}
}

func handleMilestoneCompleted(currentState *State, payload map[string]interface{}) {
	if milestone := getString(payload, "milestone", "name"); milestone != "" {
		if !containsString(currentState.Completed, milestone) {
			currentState.Completed = append(currentState.Completed, milestone)
		}
		// Remove from Remaining if present
		currentState.Remaining = removeString(currentState.Remaining, milestone)
		currentState.Current = "Completed milestone: " + milestone
	}
}

func handleSessionInterrupted(currentState *State, payload map[string]interface{}) {
	reason := getString(payload, "reason")
	if reason != "" {
		currentState.Current = "Session interrupted: " + reason
	} else {
		currentState.Current = "Session interrupted"
	}
	currentState.NextAction = "Resume interrupted session"
}

func handleSessionResumed(currentState *State) {
	currentState.Current = "Session resumed"
}
