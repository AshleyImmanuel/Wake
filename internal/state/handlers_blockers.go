package state

func handleBlockerCreated(currentState *State, payload map[string]interface{}) {
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
}

func handleBlockerResolved(currentState *State, payload map[string]interface{}) {
	if id := getString(payload, "id"); id != "" {
		for i, b := range currentState.Blocked {
			if b.ID == id {
				currentState.Blocked[i].Status = "RESOLVED"
			}
		}
	}
}

func handleGitCommit(currentState *State, payload map[string]interface{}) {
	if hash := getString(payload, "hash", "commit"); hash != "" {
		currentState.LastVerified = hash
	}
}
