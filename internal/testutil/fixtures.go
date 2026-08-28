package testutil

import (
	"time"

	"github.com/google/uuid"
	"github.com/wake/wake/internal/events"
	"github.com/wake/wake/internal/git"
	"github.com/wake/wake/internal/state"
)

// SampleTaskID generates a fresh UUID for task identification.
func SampleTaskID() uuid.UUID {
	return uuid.New()
}

// SampleEvent returns an Event populated with standard default payload for the given event type.
func SampleEvent(eventType events.EventType) events.Event {
	taskID := SampleTaskID()
	payload := DefaultPayloadForType(eventType)
	return events.NewEvent(taskID, eventType, payload)
}

// SampleEventForTask returns an Event bound to a specific task ID.
func SampleEventForTask(taskID uuid.UUID, eventType events.EventType) events.Event {
	return events.NewEvent(taskID, eventType, DefaultPayloadForType(eventType))
}

// SampleEventWithPayload constructs an Event with an explicit custom payload.
func SampleEventWithPayload(taskID uuid.UUID, eventType events.EventType, payload map[string]interface{}) events.Event {
	return events.NewEvent(taskID, eventType, payload)
}

// DefaultPayloadForType returns canonical mock payload fields for all 17 event types.
func DefaultPayloadForType(eventType events.EventType) map[string]interface{} {
	switch eventType {
	case events.TaskStarted:
		return map[string]interface{}{
			"objective": "Build Sentinel core architecture",
		}
	case events.RequirementAdded:
		return map[string]interface{}{
			"requirement": "Support SQLite WAL journal mode and PRAGMA busy_timeout",
		}
	case events.ConstraintAdded:
		return map[string]interface{}{
			"constraint": "auth/*",
		}
	case events.UserApproval:
		return map[string]interface{}{
			"note": "Approved checkpoint for Milestone 1",
		}
	case events.UserRejection:
		return map[string]interface{}{
			"reason": "Missing adversarial test coverage",
		}
	case events.DecisionMade:
		return map[string]interface{}{
			"id":          "DEC-01",
			"description": "Use modernc.org/sqlite pure-Go driver",
			"source":      "Developer instruction",
		}
	case events.FileChanged:
		return map[string]interface{}{
			"path":   "internal/reconcile/engine.go",
			"action": "modified",
		}
	case events.CommandExecuted:
		return map[string]interface{}{
			"command":   "go test ./...",
			"exit_code": 0,
		}
	case events.TestStarted:
		return map[string]interface{}{
			"suite": "internal/reconcile",
		}
	case events.TestPassed:
		return map[string]interface{}{
			"suite":  "internal/reconcile",
			"passed": 8,
		}
	case events.TestFailed:
		return map[string]interface{}{
			"suite":  "internal/git",
			"failed": 1,
		}
	case events.BlockerCreated:
		return map[string]interface{}{
			"id":          "BLK-01",
			"description": "Git index lock prevents write operations",
		}
	case events.BlockerResolved:
		return map[string]interface{}{
			"id": "BLK-01",
		}
	case events.MilestoneCompleted:
		return map[string]interface{}{
			"milestone": "Milestone 1 Test Harness",
		}
	case events.GitCommit:
		return map[string]interface{}{
			"hash": "a1b2c3d4e5f67890123456789abcdef012345678",
		}
	case events.SessionInterrupted:
		return map[string]interface{}{
			"reason": "Process received SIGINT",
		}
	case events.SessionResumed:
		return map[string]interface{}{
			"session_id": uuid.New().String(),
		}
	default:
		return map[string]interface{}{
			"info": "generic event payload",
		}
	}
}

// SampleEventSequence returns a canonical chronological sequence of events representing a full task lifecycle.
func SampleEventSequence(taskID uuid.UUID) []events.Event {
	baseTime := time.Now().UTC().Add(-1 * time.Hour)
	eventTypes := []events.EventType{
		events.TaskStarted,
		events.RequirementAdded,
		events.ConstraintAdded,
		events.DecisionMade,
		events.FileChanged,
		events.GitCommit,
		events.MilestoneCompleted,
		events.BlockerCreated,
		events.BlockerResolved,
	}

	seq := make([]events.Event, len(eventTypes))
	for i, et := range eventTypes {
		seq[i] = events.Event{
			ID:        uuid.New(),
			TaskID:    taskID,
			Type:      et,
			Timestamp: baseTime.Add(time.Duration(i*5) * time.Minute),
			Payload:   DefaultPayloadForType(et),
		}
	}
	return seq
}

// SampleState returns a populated, valid state.State instance.
func SampleState() state.State {
	taskID := SampleTaskID()
	return state.State{
		TaskID:      taskID,
		Objective:   "Implement Sentinel MVP",
		Constraints: []string{"auth/*", "protected/config.json"},
		Decisions: []state.Decision{
			{
				ID:          "DEC-01",
				Description: "Use modernc.org/sqlite",
				Source:      "Developer",
				Status:      "ACTIVE",
			},
		},
		Completed:    []string{"internal/testutil/git.go", "schema/init.sql"},
		Current:      "Building shared test fixtures",
		Remaining:    []string{"Milestone 2 Events", "Milestone 3 DB"},
		Blocked:      make([]state.Blocker, 0),
		DoNotRepeat:  []string{"internal/git/parser.go"},
		LastVerified: "a1b2c3d4e5f67890123456789abcdef012345678",
		NextAction:   "Run test suite",
		Confidence:   state.ConfidenceHigh,
	}
}

// SampleCheckpoint returns a standard state.Checkpoint instance with default test properties.
func SampleCheckpoint() state.Checkpoint {
	taskID := SampleTaskID()
	commit := "a1b2c3d4e5f67890123456789abcdef012345678"
	st := SampleState()
	st.TaskID = taskID

	return state.Checkpoint{
		ID:            uuid.New(),
		TaskID:        taskID,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Repository:    "/workspace",
		Branch:        "main",
		Commit:        commit,
		StateVersion:  1,
		EventPosition: 10,
		StateData:     st,
	}
}

// SampleCheckpointWithCommit returns a Checkpoint configured with specific commit and branch.
func SampleCheckpointWithCommit(commit, branch string) state.Checkpoint {
	cp := SampleCheckpoint()
	cp.Commit = commit
	cp.Branch = branch
	cp.StateData.LastVerified = commit
	return cp
}

// SampleDecision returns a structured Decision model.
func SampleDecision(id, description, status string) state.Decision {
	if status == "" {
		status = "ACTIVE"
	}
	return state.Decision{
		ID:          id,
		Description: description,
		Source:      "Developer",
		Status:      status,
	}
}

// SampleBlocker returns a structured Blocker model.
func SampleBlocker(id, description, status string) state.Blocker {
	if status == "" {
		status = "ACTIVE"
	}
	return state.Blocker{
		ID:          id,
		Description: description,
		Status:      status,
	}
}

// SampleFileStatus returns a git.FileStatus instance.
func SampleFileStatus(path string, stagingStatus, workTreeStatus git.StatusCode) git.FileStatus {
	return git.FileStatus{
		Path:           path,
		StagingStatus:  stagingStatus,
		WorkTreeStatus: workTreeStatus,
	}
}

// SampleFileChange returns a git.FileChange instance.
func SampleFileChange(path string, status git.StatusCode) git.FileChange {
	return git.FileChange{
		Path:   path,
		Status: status,
	}
}

// SampleRepositoryState returns a git.RepositoryState with matching status.
func SampleRepositoryState(repoDir, branch, commit string, isClean bool) git.RepositoryState {
	return git.RepositoryState{
		RootPath:          repoDir,
		Branch:            branch,
		CommitHash:        commit,
		IsDetached:        branch == "HEAD",
		HasCommits:        commit != "",
		IsClean:           isClean,
		HasMergeConflicts: false,
		StagedFiles:       make([]git.FileStatus, 0),
		UnstagedFiles:     make([]git.FileStatus, 0),
		UntrackedFiles:    make([]string, 0),
		UnmergedFiles:     make([]string, 0),
		ModifiedFiles:     make([]string, 0),
	}
}
