package events

import (
	"time"

	"github.com/google/uuid"
)

type EventType string

const (
	TaskStarted        EventType = "TASK_STARTED"
	RequirementAdded   EventType = "REQUIREMENT_ADDED"
	ConstraintAdded    EventType = "CONSTRAINT_ADDED"
	UserApproval       EventType = "USER_APPROVAL"
	UserRejection      EventType = "USER_REJECTION"
	DecisionMade       EventType = "DECISION_MADE"
	FileChanged        EventType = "FILE_CHANGED"
	CommandExecuted    EventType = "COMMAND_EXECUTED"
	TestStarted        EventType = "TEST_STARTED"
	TestPassed         EventType = "TEST_PASSED"
	TestFailed         EventType = "TEST_FAILED"
	BlockerCreated     EventType = "BLOCKER_CREATED"
	BlockerResolved    EventType = "BLOCKER_RESOLVED"
	MilestoneCompleted EventType = "MILESTONE_COMPLETED"
	GitCommit          EventType = "GIT_COMMIT"
	SessionInterrupted EventType = "SESSION_INTERRUPTED"
	SessionResumed     EventType = "SESSION_RESUMED"
)

// Event represents a single state transition in the agent's task lifecycle.
type Event struct {
	ID        uuid.UUID              `json:"id"`
	TaskID    uuid.UUID              `json:"task_id"`
	SessionID uuid.UUID              `json:"session_id,omitempty"`
	Type      EventType              `json:"type"`
	Author    string                 `json:"author"`
	Timestamp time.Time              `json:"timestamp"`
	Payload   map[string]interface{} `json:"payload"` // Flexible payload depending on EventType
}

// NewEvent is a helper to create a new event with a defensively cloned payload.
func NewEvent(taskID uuid.UUID, sessionID uuid.UUID, eventType EventType, author string, payload map[string]interface{}) Event {
	return Event{
		ID:        uuid.New(),
		TaskID:    taskID,
		SessionID: sessionID,
		Type:      eventType,
		Author:    author,
		Timestamp: time.Now().UTC(),
		Payload:   ClonePayload(payload),
	}
}

// Clone creates a deep copy of the Event to guarantee thread safety.
func (e Event) Clone() Event {
	return Event{
		ID:        e.ID,
		TaskID:    e.TaskID,
		SessionID: e.SessionID,
		Type:      e.Type,
		Author:    e.Author,
		Timestamp: e.Timestamp,
		Payload:   ClonePayload(e.Payload),
	}
}

// ClonePayload performs a recursive deep copy of an event payload map.
func ClonePayload(p map[string]interface{}) map[string]interface{} {
	if p == nil {
		return nil
	}
	cloned := make(map[string]interface{}, len(p))
	for k, v := range p {
		cloned[k] = cloneValue(v)
	}
	return cloned
}

func cloneValue(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case map[string]interface{}:
		return ClonePayload(val)
	case []interface{}:
		cp := make([]interface{}, len(val))
		for i, item := range val {
			cp[i] = cloneValue(item)
		}
		return cp
	case []string:
		cp := make([]string, len(val))
		copy(cp, val)
		return cp
	default:
		return val
	}
}
