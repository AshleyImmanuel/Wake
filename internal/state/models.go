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
	TaskID            uuid.UUID         `json:"task_id,omitempty"`
	Objective         string            `json:"objective,omitempty"`
	Constraints       []string          `json:"constraints,omitempty"`
	Decisions         []Decision        `json:"decisions,omitempty"`
	Completed         []string          `json:"completed,omitempty"`
	Current           string            `json:"current,omitempty"`
	Remaining         []string          `json:"remaining,omitempty"`
	Blocked           []Blocker         `json:"blocked,omitempty"`
	DoNotRepeat       []string          `json:"do_not_repeat,omitempty"`
	LastVerified      string            `json:"last_verified,omitempty"`
	NextAction        string            `json:"next_action,omitempty"`
	Confidence        ConfidenceLevel   `json:"confidence,omitempty"`
	Files             map[string]string `json:"files,omitempty"`
	LastKnownAction   string            `json:"last_action,omitempty"`
	LastCommand       string            `json:"last_command,omitempty"`
	LastCommandResult string            `json:"last_command_result,omitempty"`
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

// Decision represents a recorded design choice.
type Decision struct {
	ID          string `json:"id,omitempty"`
	Description string `json:"desc,omitempty"`
	Source      string `json:"src,omitempty"`
	Status      string `json:"status,omitempty"`
}

// Blocker represents an active or resolved impediment.
type Blocker struct {
	ID          string `json:"id,omitempty"`
	Description string `json:"desc,omitempty"`
	Status      string `json:"status,omitempty"`
}

// Checkpoint represents a versioned snapshot of the state (PRD Section 8.4)
type Checkpoint struct {
	ID            uuid.UUID
	TaskID        uuid.UUID
	SessionID     uuid.UUID
	Timestamp     string
	Repository    string
	Branch        string
	Commit        string
	Author        string
	StateVersion  int
	EventPosition int
	StateData     State // The snapshot of the state at this point
}
