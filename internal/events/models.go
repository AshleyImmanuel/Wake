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
	Type      EventType              `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Payload   map[string]interface{} `json:"payload"` // Flexible payload depending on EventType
}

// NewEvent is a helper to create a new event
func NewEvent(taskID uuid.UUID, eventType EventType, payload map[string]interface{}) Event {
	return Event{
		ID:        uuid.New(),
		TaskID:    taskID,
		Type:      eventType,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	}
}
