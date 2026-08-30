package state

func handleConstraintAdded(currentState *State, payload map[string]interface{}) {
	if constraint := getString(payload, "constraint", "description", "pattern"); constraint != "" {
		if !containsString(currentState.Constraints, constraint) {
			currentState.Constraints = append(currentState.Constraints, constraint)
		}
	}
}

func handleUserApproval(currentState *State, payload map[string]interface{}) {
	decisionID := getString(payload, "id", "decision_id")
	if decisionID != "" {
		for i, d := range currentState.Decisions {
			if d.ID == decisionID {
				currentState.Decisions[i].Status = "ACTIVE"
			}
		}
	}
	if reason := getString(payload, "reason"); reason != "" {
		currentState.Current = "Approval: " + reason
	}
}

func handleUserRejection(currentState *State, payload map[string]interface{}) {
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
}

func handleDecisionMade(currentState *State, payload map[string]interface{}) {
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
}
