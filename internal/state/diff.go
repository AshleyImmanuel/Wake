package state

// StateDiff represents the delta between two states.
type StateDiff struct {
	CompletedAdded   []string
	CompletedRemoved []string
	BlockedAdded     []Blocker
	BlockedRemoved   []Blocker
	CurrentOld       string
	CurrentNew       string
	NextActionOld    string
	NextActionNew    string
}

// DiffStates compares two State objects and computes the delta.
func DiffStates(oldState, newState State) StateDiff {
	diff := StateDiff{
		CurrentOld:    oldState.Current,
		CurrentNew:    newState.Current,
		NextActionOld: oldState.NextAction,
		NextActionNew: newState.NextAction,
	}

	// Completed
	oldCompleted := make(map[string]bool)
	for _, c := range oldState.Completed {
		oldCompleted[c] = true
	}
	newCompleted := make(map[string]bool)
	for _, c := range newState.Completed {
		newCompleted[c] = true
		if !oldCompleted[c] {
			diff.CompletedAdded = append(diff.CompletedAdded, c)
		}
	}
	for _, c := range oldState.Completed {
		if !newCompleted[c] {
			diff.CompletedRemoved = append(diff.CompletedRemoved, c)
		}
	}

	// Blocked
	oldBlocked := make(map[string]Blocker)
	for _, b := range oldState.Blocked {
		if b.Status == "ACTIVE" {
			oldBlocked[b.ID] = b
		}
	}
	newBlocked := make(map[string]Blocker)
	for _, b := range newState.Blocked {
		if b.Status == "ACTIVE" {
			newBlocked[b.ID] = b
			if _, ok := oldBlocked[b.ID]; !ok {
				diff.BlockedAdded = append(diff.BlockedAdded, b)
			}
		}
	}
	for _, b := range oldState.Blocked {
		if b.Status == "ACTIVE" {
			if _, ok := newBlocked[b.ID]; !ok {
				diff.BlockedRemoved = append(diff.BlockedRemoved, b)
			}
		}
	}

	return diff
}
