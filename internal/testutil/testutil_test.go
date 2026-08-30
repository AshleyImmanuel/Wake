package testutil

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/wake/wake/internal/db"
	"github.com/wake/wake/internal/events"
	"github.com/wake/wake/internal/git"
	"github.com/wake/wake/internal/state"
)

func TestGitRepo_Lifecycle(t *testing.T) {
	repo := NewGitRepo(t)

	// 1. Initial empty state
	if repo.Dir == "" {
		t.Fatalf("expected non-empty repo directory")
	}

	// 2. Write file, stage and commit
	repo.WriteFile("hello.txt", "world\n")
	content := repo.ReadFile("hello.txt")
	if content != "world\n" {
		t.Errorf("expected 'world\\n', got %q", content)
	}

	commit1 := repo.Commit("Initial commit")
	if commit1 == "" {
		t.Fatalf("expected non-empty commit hash")
	}

	if repo.CurrentCommit() != commit1 {
		t.Errorf("expected current commit %s, got %s", commit1, repo.CurrentCommit())
	}

	// 3. Branching and switching
	repo.CreateAndCheckoutBranch("feature-test")
	if repo.CurrentBranch() != "feature-test" {
		t.Errorf("expected branch feature-test, got %s", repo.CurrentBranch())
	}

	repo.WriteFile("feature.txt", "feature content\n")
	commit2 := repo.CommitOnly("Feature commit", "feature.txt")
	if commit2 == "" {
		t.Fatalf("expected non-empty commit2 hash")
	}

	// 4. Branch without checkout
	repo.Branch("another-branch")

	// 5. Checkout main
	repo.Checkout("main")
	if repo.CurrentBranch() != "main" {
		t.Errorf("expected branch main, got %s", repo.CurrentBranch())
	}

	// 6. Delete file and stage
	repo.WriteFile("temp.txt", "temp")
	repo.DeleteFile("temp.txt")
	repo.Stage()

	// 7. Client and GetState
	client := repo.Client()
	if client == nil {
		t.Fatalf("expected non-nil client")
	}

	st := repo.GetState()
	if st == nil {
		t.Fatalf("expected non-nil repository state")
	}
	if st.Branch != "main" {
		t.Errorf("expected state branch main, got %s", st.Branch)
	}

	// 8. RunGit and RunGitAllowError
	out := repo.RunGit("status", "--porcelain")
	if out != "" {
		t.Errorf("expected clean status output, got %q", out)
	}

	_, err := repo.RunGitAllowError("log", "-n", "1")
	if err != nil {
		t.Errorf("expected no error for git log, got %v", err)
	}

	repo.Cleanup()
}

func TestGitRepo_ConflictSimulation(t *testing.T) {
	repo := NewGitRepo(t)
	repo.CauseConflict("conflict-branch", "shared.txt", "base line\n", "main line edit\n", "branch line edit\n")

	state := repo.GetState()
	if !state.HasMergeConflicts {
		t.Errorf("expected HasMergeConflicts=true")
	}
	if len(state.UnmergedFiles) == 0 {
		t.Errorf("expected unmerged files listed")
	}
}

func TestDB_NewTestDB(t *testing.T) {
	database := NewTestDB(t)
	if database == nil {
		t.Fatalf("expected non-nil database")
	}

	ctx := context.Background()
	cp := SampleCheckpoint()
	if err := db.SaveCheckpoint(ctx, database, cp); err != nil {
		t.Fatalf("failed to save checkpoint: %v", err)
	}

	count := CountRows(t, database, "checkpoints")
	if count != 1 {
		t.Errorf("expected 1 checkpoint row, got %d", count)
	}

	latest, err := db.GetLatestCheckpoint(ctx, database, cp.TaskID.String())
	if err != nil {
		t.Fatalf("failed to get latest checkpoint: %v", err)
	}
	if latest.Commit != cp.Commit {
		t.Errorf("expected commit %s, got %s", cp.Commit, latest.Commit)
	}

	ev := SampleEventForTask(cp.TaskID, events.TaskStarted)
	if err := db.SaveEvent(ctx, database, ev); err != nil {
		t.Fatalf("failed to save event: %v", err)
	}

	evCount := CountRows(t, database, "events")
	if evCount != 1 {
		t.Errorf("expected 1 event row, got %d", evCount)
	}

	evs, err := db.GetEvents(ctx, database, cp.TaskID.String())
	if err != nil {
		t.Fatalf("failed to get events: %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evs))
	}
	if evs[0].Type != events.TaskStarted {
		t.Errorf("expected event type %s, got %s", events.TaskStarted, evs[0].Type)
	}
}

func TestDB_NewTestDBPath(t *testing.T) {
	dbPath := NewTestDBPath(t)
	if dbPath == "" {
		t.Fatalf("expected non-empty db path")
	}

	dbFile := filepath.Join(dbPath, ".wake", "state.db")
	if fi, err := os.Stat(dbFile); err != nil || fi.IsDir() {
		t.Fatalf("expected valid sqlite file at %s, err: %v", dbFile, err)
	}
}

func TestDB_NewInMemoryDB(t *testing.T) {
	memDB := NewInMemoryDB(t)
	if memDB == nil {
		t.Fatalf("expected non-nil in-memory database")
	}

	count := CountRows(t, memDB, "events")
	if count != 0 {
		t.Errorf("expected 0 events initially, got %d", count)
	}

	cpCount := CountRows(t, memDB, "checkpoints")
	if cpCount != 0 {
		t.Errorf("expected 0 checkpoints initially, got %d", cpCount)
	}

	ctx := context.Background()
	ev := SampleEvent(events.TaskStarted)
	if err := db.SaveEvent(ctx, memDB, ev); err != nil {
		t.Fatalf("failed to save event to in-memory db: %v", err)
	}

	if CountRows(t, memDB, "events") != 1 {
		t.Errorf("expected 1 event row in in-memory db")
	}
}

func TestFixtures_AllEventTypes(t *testing.T) {
	allTypes := []events.EventType{
		events.TaskStarted,
		events.RequirementAdded,
		events.ConstraintAdded,
		events.UserApproval,
		events.UserRejection,
		events.DecisionMade,
		events.FileChanged,
		events.CommandExecuted,
		events.TestStarted,
		events.TestPassed,
		events.TestFailed,
		events.BlockerCreated,
		events.BlockerResolved,
		events.MilestoneCompleted,
		events.GitCommit,
		events.SessionInterrupted,
		events.SessionResumed,
	}

	if len(allTypes) != 17 {
		t.Fatalf("expected exactly 17 event types, got %d", len(allTypes))
	}

	for _, et := range allTypes {
		ev := SampleEvent(et)
		if ev.Type != et {
			t.Errorf("expected event type %s, got %s", et, ev.Type)
		}
		if ev.Payload == nil {
			t.Errorf("expected non-nil payload for event type %s", et)
		}
		if ev.ID == uuid.Nil {
			t.Errorf("expected non-nil UUID for event type %s", et)
		}
		if ev.TaskID == uuid.Nil {
			t.Errorf("expected non-nil TaskID for event type %s", et)
		}
		if ev.Timestamp.IsZero() {
			t.Errorf("expected non-zero Timestamp for event type %s", et)
		}
	}
}

func TestFixtures_SampleSequence(t *testing.T) {
	taskID := SampleTaskID()
	seq := SampleEventSequence(taskID)
	if len(seq) < 5 {
		t.Fatalf("expected at least 5 events in sequence, got %d", len(seq))
	}

	var prevTime time.Time
	for i, ev := range seq {
		if ev.TaskID != taskID {
			t.Errorf("expected task ID %s, got %s", taskID, ev.TaskID)
		}
		if ev.ID == uuid.Nil {
			t.Errorf("expected non-nil event ID at index %d", i)
		}
		if !prevTime.IsZero() && !ev.Timestamp.After(prevTime) {
			t.Errorf("expected event %d timestamp %v to be after %v", i, ev.Timestamp, prevTime)
		}
		prevTime = ev.Timestamp
	}

	// Verify state reduction over the sequence
	reduced := state.Reduce(taskID.String(), seq)
	if reduced.Objective != "Build Sentinel core architecture" {
		t.Errorf("expected objective 'Build Sentinel core architecture', got %q", reduced.Objective)
	}
	if len(reduced.Constraints) == 0 {
		t.Errorf("expected constraints to be populated")
	}
	if len(reduced.Decisions) == 0 {
		t.Errorf("expected decisions to be populated")
	}
	if len(reduced.Completed) == 0 {
		t.Errorf("expected completed milestones to be populated")
	}
	if len(reduced.Blocked) == 0 {
		t.Errorf("expected blockers to be tracked")
	} else if reduced.Blocked[0].Status != "RESOLVED" {
		t.Errorf("expected blocker status to be RESOLVED, got %s", reduced.Blocked[0].Status)
	}
	if reduced.LastVerified == "" {
		t.Errorf("expected LastVerified commit hash to be set")
	}
}

func TestFixtures_SampleEventVariants(t *testing.T) {
	taskID := SampleTaskID()
	ev1 := SampleEventForTask(taskID, events.DecisionMade)
	if ev1.TaskID != taskID {
		t.Errorf("expected task ID %s, got %s", taskID, ev1.TaskID)
	}

	customPayload := map[string]interface{}{"custom_key": "custom_val"}
	ev2 := SampleEventWithPayload(taskID, events.FileChanged, customPayload)
	if ev2.Payload["custom_key"] != "custom_val" {
		t.Errorf("expected custom payload value 'custom_val', got %v", ev2.Payload["custom_key"])
	}

	unknownPayload := DefaultPayloadForType("UNKNOWN_TYPE")
	if unknownPayload["info"] != "generic event payload" {
		t.Errorf("expected default generic payload, got %v", unknownPayload)
	}
}

func TestFixtures_StateAndCheckpoint(t *testing.T) {
	st := SampleState()
	if st.Objective == "" {
		t.Errorf("expected non-empty objective")
	}
	if len(st.Constraints) == 0 {
		t.Errorf("expected constraints")
	}
	if st.Confidence != state.ConfidenceHigh {
		t.Errorf("expected high confidence, got %s", st.Confidence)
	}

	cp := SampleCheckpoint()
	if cp.ID == uuid.Nil {
		t.Errorf("expected non-nil checkpoint ID")
	}
	if cp.Commit == "" {
		t.Errorf("expected non-empty commit hash")
	}

	customCommit := "deadbeef1234567890deadbeef1234567890dead"
	customBranch := "feature/auth"
	cpWithCommit := SampleCheckpointWithCommit(customCommit, customBranch)
	if cpWithCommit.Commit != customCommit {
		t.Errorf("expected commit %s, got %s", customCommit, cpWithCommit.Commit)
	}
	if cpWithCommit.Branch != customBranch {
		t.Errorf("expected branch %s, got %s", customBranch, cpWithCommit.Branch)
	}
	if cpWithCommit.StateData.LastVerified != customCommit {
		t.Errorf("expected StateData.LastVerified %s, got %s", customCommit, cpWithCommit.StateData.LastVerified)
	}
}

func TestFixtures_DecisionAndBlocker(t *testing.T) {
	dec1 := SampleDecision("DEC-1", "Use SQLite", "ACTIVE")
	if dec1.ID != "DEC-1" || dec1.Status != "ACTIVE" {
		t.Errorf("unexpected decision: %+v", dec1)
	}

	dec2 := SampleDecision("DEC-2", "Use SQLite", "")
	if dec2.Status != "ACTIVE" {
		t.Errorf("expected default ACTIVE status, got %s", dec2.Status)
	}

	blk1 := SampleBlocker("BLK-1", "Network timeout", "RESOLVED")
	if blk1.ID != "BLK-1" || blk1.Status != "RESOLVED" {
		t.Errorf("unexpected blocker: %+v", blk1)
	}

	blk2 := SampleBlocker("BLK-2", "Network timeout", "")
	if blk2.Status != "ACTIVE" {
		t.Errorf("expected default ACTIVE status, got %s", blk2.Status)
	}
}

func TestFixtures_GitModels(t *testing.T) {
	fs := SampleFileStatus("pkg/foo.go", git.StatusModified, git.StatusUnmodified)
	if fs.Path != "pkg/foo.go" || fs.StagingStatus != git.StatusModified || fs.WorkTreeStatus != git.StatusUnmodified {
		t.Errorf("unexpected file status: %+v", fs)
	}

	fc := SampleFileChange("pkg/bar.go", git.StatusAdded)
	if fc.Path != "pkg/bar.go" || fc.Status != git.StatusAdded {
		t.Errorf("unexpected file change: %+v", fc)
	}

	repoState := SampleRepositoryState("/tmp/repo", "main", "a1b2c3d", true)
	if repoState.RootPath != "/tmp/repo" || repoState.Branch != "main" || !repoState.IsClean || repoState.IsDetached {
		t.Errorf("unexpected repository state: %+v", repoState)
	}

	detachedState := SampleRepositoryState("/tmp/repo", "HEAD", "a1b2c3d", false)
	if !detachedState.IsDetached {
		t.Errorf("expected detached HEAD state")
	}
}
