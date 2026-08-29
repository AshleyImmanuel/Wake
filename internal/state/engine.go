package state

import (
	"strings"

	"github.com/AshleyImmanuel/Wake/internal/events"
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

		case events.RequirementAdded:
			if req := getString(payload, "requirement", "description", "name"); req != "" {
				if !containsString(currentState.Remaining, req) {
					currentState.Remaining = append(currentState.Remaining, req)
				}
			}

		case events.ConstraintAdded:
			if constraint := getString(payload, "constraint", "description", "pattern"); constraint != "" {
				if !containsString(currentState.Constraints, constraint) {
					currentState.Constraints = append(currentState.Constraints, constraint)
				}
			}

		case events.UserApproval:
			decisionID := getString(payload, "id", "decision_id")
			if decisionID != "" {
				for i, d := range currentState.Decisions {
					if d.ID == decisionID {
						currentState.Decisions[i].Status = "ACTIVE"
					}
				}
			}
			if next := getString(payload, "next_action"); next != "" {
				currentState.NextAction = next
			}

		case events.UserRejection:
			decisionID := getString(payload, "id", "decision_id")
			if decisionID != "" {
				for i, d := range currentState.Decisions {
					if d.ID == decisionID {
						currentState.Decisions[i].Status = "REJECTED"
					}
				}
			}
			reason := getString(payload, "reason", "description")
			if reason != "" {
				currentState.NextAction = "Address rejection: " + reason
			}
			if dnr := getString(payload, "do_not_repeat", "file", "path"); dnr != "" {
				if !containsString(currentState.DoNotRepeat, dnr) {
					currentState.DoNotRepeat = append(currentState.DoNotRepeat, dnr)
				}
			}
			for _, dnr := range getStringSlice(payload, "do_not_repeat") {
				if !containsString(currentState.DoNotRepeat, dnr) {
					currentState.DoNotRepeat = append(currentState.DoNotRepeat, dnr)
				}
			}

		case events.DecisionMade:
			if desc := getString(payload, "description"); desc != "" {
				id := getString(payload, "id")
				source := getString(payload, "source")
				status := getString(payload, "status")
				if status == "" {
					status = "ACTIVE"
				}
				// Upsert decision
				found := false
				for i, d := range currentState.Decisions {
					if id != "" && d.ID == id {
						currentState.Decisions[i] = Decision{
							ID:          id,
							Description: desc,
							Source:      source,
							Status:      status,
						}
						found = true
						break
					}
				}
				if !found {
					currentState.Decisions = append(currentState.Decisions, Decision{
						ID:          id,
						Description: desc,
						Source:      source,
						Status:      status,
					})
				}
			}

		case events.FileChanged:
			filePath := getString(payload, "path", "file")
			action := getString(payload, "action")
			if filePath != "" {
				currentState.Current = "Editing " + filePath
				if action == "do_not_repeat" {
					if !containsString(currentState.DoNotRepeat, filePath) {
						currentState.DoNotRepeat = append(currentState.DoNotRepeat, filePath)
					}
				}
			}

		case events.CommandExecuted:
			cmd := getString(payload, "command")
			if cmd != "" {
				currentState.Current = "Executed: " + cmd
			}
			if next := getString(payload, "next_action"); next != "" {
				currentState.NextAction = next
			} else if exitCode, ok := getInt(payload, "exit_code"); ok && exitCode != 0 && cmd != "" {
				currentState.NextAction = "Investigate failed command: " + cmd
			}

		case events.TestStarted:
			suite := getString(payload, "suite", "test")
			if suite != "" {
				currentState.Current = "Running tests: " + suite
			}

		case events.TestPassed:
			suite := getString(payload, "suite", "test")
			if suite != "" {
				currentState.Current = "Tests passed: " + suite
			}
			if next := getString(payload, "next_action"); next != "" {
				currentState.NextAction = next
			}

		case events.TestFailed:
			suite := getString(payload, "suite", "test")
			if suite != "" {
				currentState.Current = "Test failed: " + suite
				currentState.NextAction = "Fix failing tests: " + suite
			}
			if errStr := getString(payload, "error"); errStr != "" {
				currentState.NextAction = "Fix failing test: " + errStr
			}

		case events.BlockerCreated:
			if desc := getString(payload, "description"); desc != "" {
				id := getString(payload, "id")
				found := false
				for i, b := range currentState.Blocked {
					if id != "" && b.ID == id {
						currentState.Blocked[i].Description = desc
						currentState.Blocked[i].Status = "ACTIVE"
						found = true
						break
					}
				}
				if !found {
					currentState.Blocked = append(currentState.Blocked, Blocker{
						ID:          id,
						Description: desc,
						Status:      "ACTIVE",
					})
				}
				currentState.NextAction = "Resolve blocker: " + desc
			}

		case events.BlockerResolved:
			if id := getString(payload, "id"); id != "" {
				for i, b := range currentState.Blocked {
					if b.ID == id {
						currentState.Blocked[i].Status = "RESOLVED"
					}
				}
			}

		case events.MilestoneCompleted:
			if milestone := getString(payload, "milestone", "name"); milestone != "" {
				if !containsString(currentState.Completed, milestone) {
					currentState.Completed = append(currentState.Completed, milestone)
				}
				// Remove from Remaining if present
				currentState.Remaining = removeString(currentState.Remaining, milestone)
				currentState.Current = "Completed milestone: " + milestone
			}

		case events.GitCommit:
			if hash := getString(payload, "hash", "commit"); hash != "" {
				currentState.LastVerified = hash
			}

		case events.SessionInterrupted:
			reason := getString(payload, "reason")
			if reason != "" {
				currentState.Current = "Session interrupted: " + reason
			} else {
				currentState.Current = "Session interrupted"
			}
			currentState.NextAction = "Resume interrupted session"

		case events.SessionResumed:
			currentState.Current = "Session resumed"
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

// Safe payload extraction helpers

func getString(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if val, ok := m[k]; ok && val != nil {
			if s, ok := val.(string); ok {
				trimmed := strings.TrimSpace(s)
				if trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return ""
}

func getStringSlice(m map[string]interface{}, keys ...string) []string {
	for _, k := range keys {
		if val, ok := m[k]; ok && val != nil {
			if slice, ok := val.([]string); ok {
				return slice
			}
			if slice, ok := val.([]interface{}); ok {
				res := make([]string, 0, len(slice))
				for _, item := range slice {
					if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
						res = append(res, strings.TrimSpace(s))
					}
				}
				return res
			}
		}
	}
	return nil
}

func getInt(m map[string]interface{}, keys ...string) (int, bool) {
	for _, k := range keys {
		if val, ok := m[k]; ok && val != nil {
			switch v := val.(type) {
			case int:
				return v, true
			case int64:
				return int(v), true
			case float64:
				return int(v), true
			}
		}
	}
	return 0, false
}

func containsString(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

func removeString(slice []string, target string) []string {
	res := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != target {
			res = append(res, s)
		}
	}
	return res
}
