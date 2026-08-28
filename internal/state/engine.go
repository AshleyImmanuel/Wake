package state

import (
	"github.com/wake/wake/internal/events"
)

// Reduce takes a slice of ordered events and reduces them into a current State snapshot.
func Reduce(taskID string, history []events.Event) State {
	currentState := State{
		Constraints: make([]string, 0),
		Decisions:   make([]Decision, 0),
		Completed:   make([]string, 0),
		Remaining:   make([]string, 0),
		Blocked:     make([]Blocker, 0),
		DoNotRepeat: make([]string, 0),
		Confidence:  ConfidenceHigh,
	}

	for _, e := range history {
		switch e.Type {
		case events.TaskStarted:
			if obj, ok := e.Payload["objective"].(string); ok {
				currentState.Objective = obj
			}

		case events.ConstraintAdded:
			if constraint, ok := e.Payload["constraint"].(string); ok {
				currentState.Constraints = append(currentState.Constraints, constraint)
			}

		case events.DecisionMade:
			if desc, ok := e.Payload["description"].(string); ok {
				source, _ := e.Payload["source"].(string)
				id, _ := e.Payload["id"].(string)
				currentState.Decisions = append(currentState.Decisions, Decision{
					ID:          id,
					Description: desc,
					Source:      source,
					Status:      "ACTIVE",
				})
			}

		case events.MilestoneCompleted:
			if milestone, ok := e.Payload["milestone"].(string); ok {
				currentState.Completed = append(currentState.Completed, milestone)
			}

		case events.BlockerCreated:
			if desc, ok := e.Payload["description"].(string); ok {
				id, _ := e.Payload["id"].(string)
				currentState.Blocked = append(currentState.Blocked, Blocker{
					ID:          id,
					Description: desc,
					Status:      "ACTIVE",
				})
			}

		case events.BlockerResolved:
			if id, ok := e.Payload["id"].(string); ok {
				// Find and mark resolved
				for i, b := range currentState.Blocked {
					if b.ID == id {
						currentState.Blocked[i].Status = "RESOLVED"
					}
				}
			}

		case events.GitCommit:
			if hash, ok := e.Payload["hash"].(string); ok {
				currentState.LastVerified = hash
			}
		}
	}

	return currentState
}
