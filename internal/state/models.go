package state

import "github.com/google/uuid"

// ConfidenceLevel indicates how sure Wake is about the current state validity
type ConfidenceLevel string

const (
	ConfidenceHigh ConfidenceLevel = "High"
	ConfidenceLow  ConfidenceLevel = "Low"
	ConfidenceNone ConfidenceLevel = "None"
)

// EvidenceStatus represents the provenance of a piece of state
type EvidenceStatus string

const (
	Verified      EvidenceStatus = "VERIFIED"
	UserConfirmed EvidenceStatus = "USER_CONFIRMED"
	AgentInferred EvidenceStatus = "AGENT_INFERRED"
	Unknown       EvidenceStatus = "UNKNOWN"
)

// State represents the current execution state of a task (PRD Section 8.3 & 12).
type State struct {
	TaskID       uuid.UUID
	Objective    string
	Constraints  []string
	Decisions    []Decision
	Completed    []string
	Current      string
	Remaining    []string
	Blocked      []Blocker
	DoNotRepeat  []string
	LastVerified string // e.g. Git commit hash
	NextAction   string
	Confidence   ConfidenceLevel
	Files        map[string]string // File tracking for non-git projects
}

// Clone returns a deep copy of State to ensure thread safety.
func (s State) Clone() State {
	cloned := s
	if s.Constraints != nil {
		cloned.Constraints = make([]string, len(s.Constraints))
		copy(cloned.Constraints, s.Constraints)
	}
	if s.Decisions != nil {
		cloned.Decisions = make([]Decision, len(s.Decisions))
		copy(cloned.Decisions, s.Decisions)
	}
	if s.Completed != nil {
		cloned.Completed = make([]string, len(s.Completed))
		copy(cloned.Completed, s.Completed)
	}
	if s.Remaining != nil {
		cloned.Remaining = make([]string, len(s.Remaining))
		copy(cloned.Remaining, s.Remaining)
	}
	if s.Blocked != nil {
		cloned.Blocked = make([]Blocker, len(s.Blocked))
		copy(cloned.Blocked, s.Blocked)
	}
	if s.DoNotRepeat != nil {
		cloned.DoNotRepeat = make([]string, len(s.DoNotRepeat))
		copy(cloned.DoNotRepeat, s.DoNotRepeat)
	}
	if s.Files != nil {
		cloned.Files = make(map[string]string, len(s.Files))
		for k, v := range s.Files {
			cloned.Files[k] = v
		}
	}
	return cloned
}

type Decision struct {
	ID          string
	Description string
	Source      string // e.g., "Developer instruction", "Agent inferred"
	Status      string // e.g., "ACTIVE", "REJECTED"
}

type Blocker struct {
	ID          string
	Description string
	Status      string // e.g., "ACTIVE", "RESOLVED"
}

// Checkpoint represents a versioned snapshot of the state (PRD Section 8.4)
type Checkpoint struct {
	ID            uuid.UUID
	TaskID        uuid.UUID
	Timestamp     string
	Repository    string
	Branch        string
	Commit        string
	Author        string
	StateVersion  int
	EventPosition int
	StateData     State // The snapshot of the state at this point
}
