package guard

import (
	"context"
	"strings"
	"testing"

	"github.com/wake/wake/internal/git"
)

func TestValidatePreCheckpoint_CleanRepo(t *testing.T) {
	ctx := context.Background()
	repoState := &git.RepositoryState{
		RootPath:       "/workspace",
		Branch:         "main",
		CommitHash:     "commit123",
		IsClean:        true,
		UntrackedFiles: []string{},
		ModifiedFiles:  []string{},
	}

	err := ValidatePreCheckpoint(ctx, repoState, CheckpointGuardOptions{})
	if err != nil {
		t.Fatalf("expected nil error on clean repo, got: %v", err)
	}
}

func TestValidatePreCheckpoint_UntrackedFiles(t *testing.T) {
	ctx := context.Background()
	repoState := &git.RepositoryState{
		RootPath:       "/workspace",
		Branch:         "main",
		CommitHash:     "commit123",
		IsClean:        false,
		UntrackedFiles: []string{"notes.txt", "scratch/test.py"},
	}

	err := ValidatePreCheckpoint(ctx, repoState, CheckpointGuardOptions{})
	if err == nil {
		t.Fatalf("expected GuardViolation error on untracked files, got nil")
	}

	v, ok := err.(*GuardViolation)
	if !ok {
		t.Fatalf("expected *GuardViolation error type, got %T", err)
	}
	if len(v.UntrackedFiles) != 2 {
		t.Errorf("expected 2 untracked files, got %d", len(v.UntrackedFiles))
	}
}

func TestValidatePreCheckpoint_ModifiedFiles(t *testing.T) {
	ctx := context.Background()
	repoState := &git.RepositoryState{
		RootPath:      "/workspace",
		Branch:        "main",
		CommitHash:    "commit123",
		IsClean:       false,
		ModifiedFiles: []string{"pkg/auth/session.go"},
	}

	err := ValidatePreCheckpoint(ctx, repoState, CheckpointGuardOptions{})
	if err == nil {
		t.Fatalf("expected GuardViolation error on modified files, got nil")
	}

	v, ok := err.(*GuardViolation)
	if !ok {
		t.Fatalf("expected *GuardViolation error type, got %T", err)
	}
	if len(v.ModifiedFiles) != 1 || v.ModifiedFiles[0] != "pkg/auth/session.go" {
		t.Errorf("expected modified file 'pkg/auth/session.go', got %v", v.ModifiedFiles)
	}
}

func TestValidatePreCheckpoint_ForceOverride(t *testing.T) {
	ctx := context.Background()
	repoState := &git.RepositoryState{
		RootPath:          "/workspace",
		Branch:            "main",
		CommitHash:        "commit123",
		IsClean:           false,
		HasMergeConflicts: true,
		UntrackedFiles:    []string{"untracked.txt"},
		ModifiedFiles:     []string{"modified.go"},
	}

	err := ValidatePreCheckpoint(ctx, repoState, CheckpointGuardOptions{
		Force: true,
	})
	if err != nil {
		t.Fatalf("expected nil error when Force=true, got: %v", err)
	}
}

func TestValidatePreCheckpoint_InternalMetadataFiltered(t *testing.T) {
	ctx := context.Background()
	repoState := &git.RepositoryState{
		RootPath:       "/workspace",
		Branch:         "main",
		CommitHash:     "commit123",
		IsClean:        false,
		UntrackedFiles: []string{".wake/state.db", ".sentinel/config.json", ".git/index"},
		ModifiedFiles:  []string{".wake/session.json"},
	}

	err := ValidatePreCheckpoint(ctx, repoState, CheckpointGuardOptions{})
	if err != nil {
		t.Fatalf("expected internal metadata files to be ignored by guard, got error: %v", err)
	}
}

func TestValidatePreCheckpoint_MergeConflicts(t *testing.T) {
	ctx := context.Background()
	repoState := &git.RepositoryState{
		RootPath:          "/workspace",
		Branch:            "main",
		CommitHash:        "commit123",
		IsClean:           false,
		HasMergeConflicts: true,
	}

	err := ValidatePreCheckpoint(ctx, repoState, CheckpointGuardOptions{})
	if err == nil {
		t.Fatalf("expected error on merge conflicts, got nil")
	}
	v, ok := err.(*GuardViolation)
	if !ok || !v.HasMergeConflicts {
		t.Errorf("expected HasMergeConflicts=true in GuardViolation")
	}
}

func TestValidatePreCheckpoint_TrackedFiles(t *testing.T) {
	ctx := context.Background()

	// Scenario 1: All dirty files within tracked scope -> permitted
	repoState1 := &git.RepositoryState{
		RootPath:      "/workspace",
		Branch:        "main",
		CommitHash:    "commit123",
		IsClean:       false,
		ModifiedFiles: []string{"pkg/auth/login.go", "pkg/auth/session.go"},
	}

	err1 := ValidatePreCheckpoint(ctx, repoState1, CheckpointGuardOptions{
		TrackedFiles: []string{"pkg/auth/*"},
	})
	if err1 != nil {
		t.Fatalf("expected nil error when all modified files are within tracked files scope, got: %v", err1)
	}

	// Scenario 2: Dirty files exist OUTSIDE tracked scope -> violation
	repoState2 := &git.RepositoryState{
		RootPath:      "/workspace",
		Branch:        "main",
		CommitHash:    "commit123",
		IsClean:       false,
		ModifiedFiles: []string{"pkg/auth/login.go", "pkg/billing/invoice.go"},
	}

	err2 := ValidatePreCheckpoint(ctx, repoState2, CheckpointGuardOptions{
		TrackedFiles: []string{"pkg/auth/*"},
	})
	if err2 == nil {
		t.Fatalf("expected GuardViolation when files outside tracked scope are modified, got nil")
	}
	v2, ok := err2.(*GuardViolation)
	if !ok {
		t.Fatalf("expected *GuardViolation, got %T", err2)
	}
	if len(v2.UnreviewedFiles) != 1 || v2.UnreviewedFiles[0] != "pkg/billing/invoice.go" {
		t.Errorf("expected unreviewed file 'pkg/billing/invoice.go', got %v", v2.UnreviewedFiles)
	}
}

func TestValidatePreCheckpoint_NilRepoState(t *testing.T) {
	ctx := context.Background()
	err := ValidatePreCheckpoint(ctx, nil, CheckpointGuardOptions{})
	if err == nil {
		t.Fatalf("expected error on nil repository state, got nil")
	}
}

func TestGuardViolation_ErrorOutput(t *testing.T) {
	v := &GuardViolation{
		UntrackedFiles:    []string{"foo.txt"},
		ModifiedFiles:     []string{"bar.go"},
		UnreviewedFiles:   []string{"baz.py"},
		HasMergeConflicts: true,
	}

	msg := v.Error()
	if !strings.Contains(msg, "PRE-CHECKPOINT GUARD FATAL") {
		t.Errorf("expected message to contain 'PRE-CHECKPOINT GUARD FATAL', got: %s", msg)
	}
	if !strings.Contains(msg, "foo.txt") || !strings.Contains(msg, "bar.go") || !strings.Contains(msg, "baz.py") {
		t.Errorf("expected message to list all violating files, got: %s", msg)
	}
	if !strings.Contains(msg, "Unresolved merge conflicts") {
		t.Errorf("expected message to mention merge conflicts, got: %s", msg)
	}
}
