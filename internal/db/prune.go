package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func parseTimestamp(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("invalid timestamp format: %q", s)
}

// PruneStats holds the results of a prune operation.
type PruneStats struct {
	DeletedCheckpoints int64
	DeletedEvents      int64
}

// PruneHistory deletes old checkpoints and events to save space.
func PruneHistory(ctx context.Context, db *sql.DB, olderThan time.Time) (*PruneStats, error) {
	if db == nil {
		return nil, fmt.Errorf("db connection is nil")
	}

	threshold := olderThan.UTC().Format(time.RFC3339)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin tx: %w", err)
	}
	defer tx.Rollback()

	queryCheckpoints := `
		DELETE FROM checkpoints 
		WHERE timestamp < ? 
		AND id NOT IN (
			SELECT id FROM (
				SELECT id, ROW_NUMBER() OVER(PARTITION BY task_id ORDER BY timestamp DESC, state_version DESC) as rn 
				FROM checkpoints
			) WHERE rn = 1
		)
	`
	resCp, err := tx.ExecContext(ctx, queryCheckpoints, threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to delete old checkpoints: %w", err)
	}
	deletedCp, _ := resCp.RowsAffected()

	queryEvents := `
		DELETE FROM events 
		WHERE timestamp < ? 
		AND task_id IN (
			SELECT c.task_id FROM checkpoints c WHERE c.timestamp >= events.timestamp
		)
	`
	resEv, err := tx.ExecContext(ctx, queryEvents, threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to delete old events: %w", err)
	}
	deletedEv, _ := resEv.RowsAffected()

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit prune transaction: %w", err)
	}

	return &PruneStats{
		DeletedCheckpoints: deletedCp,
		DeletedEvents:      deletedEv,
	}, nil
}

// GetCheckpointByVersion queries a specific checkpoint version for a task.
