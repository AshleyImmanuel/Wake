package state

import (
	"wake/internal/events"
	"github.com/google/uuid"
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

	// 1. Populate TaskID from parameter
	if taskID != "" {
		if parsed, err := uuid.Parse(taskID); err == nil && parsed != uuid.Nil {
			currentState.TaskID = parsed
		}
	}

	// 2. Fold each event in chronological sequence
	for _, e := range history {
		// Populate TaskID from event envelope if still empty
		if currentState.TaskID == uuid.Nil && e.TaskID != uuid.Nil {
			currentState.TaskID = e.TaskID
		}

		payload := e.Payload
		if payload == nil {
			payload = make(map[string]interface{})
		}

		switch e.Type {
		case events.TaskStarted:
			handleTaskStarted(&currentState, payload)
		case events.RequirementAdded:
			handleRequirementAdded(&currentState, payload)
		case events.ConstraintAdded:
			handleConstraintAdded(&currentState, payload)
		case events.UserApproval:
			handleUserApproval(&currentState, payload)
		case events.UserRejection:
			handleUserRejection(&currentState, payload)
		case events.DecisionMade:
			handleDecisionMade(&currentState, payload)
		case events.FileChanged:
			handleFileChanged(&currentState, payload)
		case events.CommandExecuted:
			handleCommandExecuted(&currentState, payload)
		case events.TestStarted:
			handleTestStarted(&currentState, payload)
		case events.TestPassed:
			handleTestPassed(&currentState, payload)
		case events.TestFailed:
			handleTestFailed(&currentState, payload)
		case events.BlockerCreated:
			handleBlockerCreated(&currentState, payload)
		case events.BlockerResolved:
			handleBlockerResolved(&currentState, payload)
		case events.MilestoneCompleted:
			handleMilestoneCompleted(&currentState, payload)
		case events.GitCommit:
			handleGitCommit(&currentState, payload)
		case events.SessionInterrupted:
			handleSessionInterrupted(&currentState, payload)
		case events.SessionResumed:
			handleSessionResumed(&currentState)
		}
	}

	// 3. Compute dynamic confidence score
	currentState.Confidence = calculateConfidence(currentState.Blocked, history)

	return currentState
}

// calculateConfidence evaluates active blockers, test results, user feedback, and session continuity.
func calculateConfidence(blocked []Blocker, history []events.Event) ConfidenceLevel {
	activeBlockers := 0
	for _, b := range blocked {
		if b.Status == "ACTIVE" {
			activeBlockers++
		}
	}

	var latestTestPassed *bool
	var latestUserApproved *bool
	var sessionInterrupted bool

	for _, e := range history {
		switch e.Type {
		case events.TestPassed:
			v := true
			latestTestPassed = &v
		case events.TestFailed:
			v := false
			latestTestPassed = &v
		case events.UserApproval:
			v := true
			latestUserApproved = &v
		case events.UserRejection:
			v := false
			latestUserApproved = &v
		case events.SessionInterrupted:
			sessionInterrupted = true
		case events.SessionResumed:
			sessionInterrupted = false
		}
	}

	// Tier 1: None (Multiple blockers or blocker + test failure or rejection + test failure)
	if activeBlockers > 1 {
		return ConfidenceNone
	}
	if activeBlockers > 0 && latestTestPassed != nil && !*latestTestPassed {
		return ConfidenceNone
	}
	if latestUserApproved != nil && !*latestUserApproved && latestTestPassed != nil && !*latestTestPassed {
		return ConfidenceNone
	}

	// Tier 2: Low (1 active blocker, failing test, active rejection, or interrupted session)
	if activeBlockers == 1 {
		return ConfidenceLow
	}
	if latestTestPassed != nil && !*latestTestPassed {
		return ConfidenceLow
	}
	if latestUserApproved != nil && !*latestUserApproved {
		return ConfidenceLow
	}
	if sessionInterrupted {
		return ConfidenceLow
	}

	// Tier 3: High (Clean, verified, 0 active blockers)
	return ConfidenceHigh
}
