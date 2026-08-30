package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/AshleyImmanuel/Wake/internal/db"
	"github.com/AshleyImmanuel/Wake/internal/state"
)

// DiffCheckpoints computes the delta between two checkpoints.
func (s *taskService) DiffCheckpoints(ctx context.Context, taskID string, v1, v2 int) (*state.StateDiff, error) {
	if taskID == "" {
		latestCP, err := db.GetLatestCheckpoint(ctx, s.db, "")
		if err != nil {
			return nil, fmt.Errorf("could not find active task: %w", err)
		}
		taskID = latestCP.TaskID.String()
	}

	var cp1, cp2 *state.Checkpoint
	var err error

	if v2 == 0 {
		cp2, err = db.GetLatestCheckpoint(ctx, s.db, taskID)
		if err != nil {
			return nil, fmt.Errorf("could not find latest checkpoint: %w", err)
		}
	} else {
		cp2, err = db.GetCheckpointByVersion(ctx, s.db, taskID, v2)
		if err != nil {
			return nil, fmt.Errorf("could not find checkpoint v%d: %w", v2, err)
		}
	}

	if v1 == 0 {
		// find the checkpoint immediately preceding cp2
		if cp2.StateVersion <= 1 {
			return nil, fmt.Errorf("not enough checkpoints to diff")
		}
		cp1, err = db.GetCheckpointByVersion(ctx, s.db, taskID, cp2.StateVersion-1)
		if err != nil {
			return nil, fmt.Errorf("could not find previous checkpoint: %w", err)
		}
	} else {
		cp1, err = db.GetCheckpointByVersion(ctx, s.db, taskID, v1)
		if err != nil {
			return nil, fmt.Errorf("could not find checkpoint v%d: %w", v1, err)
		}
	}

	diff := state.DiffStates(cp1.StateData, cp2.StateData)
	return &diff, nil
}

func FormatDiff(diff state.StateDiff) string {
	var sb strings.Builder

	sb.WriteString("RECOVERY STATE CHANGES\n\n")

	hasChanges := false

	if len(diff.CompletedAdded) > 0 || len(diff.CompletedRemoved) > 0 {
		sb.WriteString("Completed:\n")
		for _, c := range diff.CompletedAdded {
			sb.WriteString(fmt.Sprintf("+ %s\n", c))
		}
		for _, c := range diff.CompletedRemoved {
			sb.WriteString(fmt.Sprintf("- %s\n", c))
		}
		sb.WriteString("\n")
		hasChanges = true
	}

	if diff.CurrentOld != diff.CurrentNew {
		sb.WriteString("Current:\n")
		sb.WriteString(fmt.Sprintf("~ %s\n\n", diff.CurrentNew))
		hasChanges = true
	}

	if len(diff.BlockedAdded) > 0 || len(diff.BlockedRemoved) > 0 {
		sb.WriteString("Blocker:\n")
		for _, b := range diff.BlockedAdded {
			sb.WriteString(fmt.Sprintf("+ %s\n", b.Description))
		}
		for _, b := range diff.BlockedRemoved {
			sb.WriteString(fmt.Sprintf("- %s\n", b.Description))
		}
		sb.WriteString("\n")
		hasChanges = true
	}

	if diff.NextActionOld != diff.NextActionNew {
		sb.WriteString("Next:\n")
		sb.WriteString(fmt.Sprintf("~ %s\n\n", diff.NextActionNew))
		hasChanges = true
	}

	if !hasChanges {
		return "No changes between these checkpoints."
	}

	return strings.TrimSpace(sb.String())
}
