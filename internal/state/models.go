package state

import "github.com/google/uuid"

// ConfidenceLevel indicates how sure Sentinel is about the current state validity
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
	TaskID         uuid.UUID
	Objective      string
	Constraints    []string
	Decisions      []Decision
	Completed      []string
	Current        string
	Remaining      []string
	Blocked        []Blocker
	DoNotRepeat    []string
	LastVerified   string // e.g. Git commit hash
	NextAction     string
	Confidence     ConfidenceLevel
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
	StateVersion  int
	EventPosition int
	StateData     State // The snapshot of the state at this point
}
