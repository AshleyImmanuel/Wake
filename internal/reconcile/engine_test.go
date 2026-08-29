package reconcile

import (
	"reflect"
	"testing"

	"github.com/AshleyImmanuel/Wake/internal/git"
	"github.com/AshleyImmanuel/Wake/internal/state"
)

func TestReconcile(t *testing.T) {
	tests := []struct {
		name      string
		cp        state.Checkpoint
		repo      git.RepositoryState
		taskFiles []string
		want      ReconciliationResult
	}{
		{
			name: "StatusSafe: clean repo, matching commit, matching branch",
			cp: state.Checkpoint{
				Branch: "main",
				Commit: "abc1234",
			},
			repo: git.RepositoryState{
				Branch:     "main",
				CommitHash: "abc1234",
				HasCommits: true,
				IsClean:    true,
			},
			want: ReconciliationResult{
				Status:             StatusSafe,
				CheckpointCommit:   "abc1234",
				CurrentCommit:      "abc1234",
				BranchMatch:        true,
				ChangedFiles:       []string{},
				TaskRelatedChanges: []string{},
				UnrelatedChanges:   []string{},
				ConfidenceLevel:    state.ConfidenceHigh,
			},
		},
		{
			name: "StatusStale: commit mismatch",
			cp: state.Checkpoint{
				Branch: "main",
				Commit: "abc1234",
			},
			repo: git.RepositoryState{
				Branch:     "main",
				CommitHash: "def5678",
				HasCommits: true,
				IsClean:    true,
			},
			want: ReconciliationResult{
				Status:             StatusStale,
				CheckpointCommit:   "abc1234",
				CurrentCommit:      "def5678",
				BranchMatch:        true,
				ChangedFiles:       []string{},
				TaskRelatedChanges: []string{},
				UnrelatedChanges:   []string{},
				ConfidenceLevel:    state.ConfidenceLow,
			},
		},
		{
			name: "StatusStale: branch mismatch",
			cp: state.Checkpoint{
				Branch: "main",
				Commit: "abc1234",
			},
			repo: git.RepositoryState{
				Branch:     "feature",
				CommitHash: "abc1234",
				HasCommits: true,
				IsClean:    true,
			},
			want: ReconciliationResult{
				Status:             StatusStale,
				CheckpointCommit:   "abc1234",
				CurrentCommit:      "abc1234",
				BranchMatch:        false,
				ChangedFiles:       []string{},
				TaskRelatedChanges: []string{},
				UnrelatedChanges:   []string{},
				ConfidenceLevel:    state.ConfidenceLow,
			},
		},
		{
			name: "StatusStale: uncommitted changed files",
			cp: state.Checkpoint{
				Branch: "main",
				Commit: "abc1234",
			},
			repo: git.RepositoryState{
				Branch:        "main",
				CommitHash:    "abc1234",
				HasCommits:    true,
				ModifiedFiles: []string{"main.go"},
			},
			want: ReconciliationResult{
				Status:             StatusStale,
				CheckpointCommit:   "abc1234",
				CurrentCommit:      "abc1234",
				BranchMatch:        true,
				ChangedFiles:       []string{"main.go"},
				TaskRelatedChanges: []string{"main.go"},
				UnrelatedChanges:   []string{},
				ConfidenceLevel:    state.ConfidenceLow,
			},
		},
		{
			name: "StatusConflict: unresolved merge conflicts",
			cp: state.Checkpoint{
				Branch: "main",
				Commit: "abc1234",
			},
			repo: git.RepositoryState{
				Branch:            "main",
				CommitHash:        "abc1234",
				HasCommits:        true,
				HasMergeConflicts: true,
			},
			want: ReconciliationResult{
				Status:             StatusConflict,
				CheckpointCommit:   "abc1234",
				CurrentCommit:      "abc1234",
				BranchMatch:        true,
				ChangedFiles:       []string{},
				TaskRelatedChanges: []string{},
				UnrelatedChanges:   []string{},
				ConfidenceLevel:    state.ConfidenceNone,
			},
		},
		{
			name: "StatusConflict: constraint violation",
			cp: state.Checkpoint{
				Branch: "main",
				Commit: "abc1234",
				StateData: state.State{
					Constraints: []string{"Do not modify auth.go"},
				},
			},
			repo: git.RepositoryState{
				Branch:        "main",
				CommitHash:    "abc1234",
				HasCommits:    true,
				ModifiedFiles: []string{"auth.go"},
			},
			want: ReconciliationResult{
				Status:             StatusConflict,
				CheckpointCommit:   "abc1234",
				CurrentCommit:      "abc1234",
				BranchMatch:        true,
				ChangedFiles:       []string{"auth.go"},
				TaskRelatedChanges: []string{"auth.go"},
				UnrelatedChanges:   []string{},
				ConfidenceLevel:    state.ConfidenceNone,
			},
		},
		{
			name: "StatusConflict: decision violation",
			cp: state.Checkpoint{
				Branch: "main",
				Commit: "abc1234",
				StateData: state.State{
					Decisions: []state.Decision{
						{Description: "Leave legacy.go alone", Status: "ACTIVE"},
					},
				},
			},
			repo: git.RepositoryState{
				Branch:        "main",
				CommitHash:    "abc1234",
				HasCommits:    true,
				ModifiedFiles: []string{"legacy.go"},
			},
			want: ReconciliationResult{
				Status:             StatusConflict,
				CheckpointCommit:   "abc1234",
				CurrentCommit:      "abc1234",
				BranchMatch:        true,
				ChangedFiles:       []string{"legacy.go"},
				TaskRelatedChanges: []string{"legacy.go"},
				UnrelatedChanges:   []string{},
				ConfidenceLevel:    state.ConfidenceNone,
			},
		},
		{
			name: "StatusConflict: completed milestone artifact modified",
			cp: state.Checkpoint{
				Branch: "main",
				Commit: "abc1234",
				StateData: state.State{
					Completed: []string{"api/docs.md"},
				},
			},
			repo: git.RepositoryState{
				Branch:        "main",
				CommitHash:    "abc1234",
				HasCommits:    true,
				ModifiedFiles: []string{"api/docs.md"},
			},
			want: ReconciliationResult{
				Status:             StatusConflict,
				CheckpointCommit:   "abc1234",
				CurrentCommit:      "abc1234",
				BranchMatch:        true,
				ChangedFiles:       []string{"api/docs.md"},
				TaskRelatedChanges: []string{"api/docs.md"},
				UnrelatedChanges:   []string{},
				ConfidenceLevel:    state.ConfidenceNone,
			},
		},
		{
			name: "StatusConflict: DoNotRepeat artifact modified",
			cp: state.Checkpoint{
				Branch: "main",
				Commit: "abc1234",
				StateData: state.State{
					DoNotRepeat: []string{"build.sh"},
				},
			},
			repo: git.RepositoryState{
				Branch:        "main",
				CommitHash:    "abc1234",
				HasCommits:    true,
				ModifiedFiles: []string{"build.sh"},
			},
			want: ReconciliationResult{
				Status:             StatusConflict,
				CheckpointCommit:   "abc1234",
				CurrentCommit:      "abc1234",
				BranchMatch:        true,
				ChangedFiles:       []string{"build.sh"},
				TaskRelatedChanges: []string{"build.sh"},
				UnrelatedChanges:   []string{},
				ConfidenceLevel:    state.ConfidenceNone,
			},
		},
		{
			name: "StatusConflict: checkpoint references commit but repo has no commits",
			cp: state.Checkpoint{
				Branch: "main",
				Commit: "abc1234",
			},
			repo: git.RepositoryState{
				Branch:     "main",
				CommitHash: "",
				HasCommits: false,
			},
			want: ReconciliationResult{
				Status:             StatusConflict,
				CheckpointCommit:   "abc1234",
				CurrentCommit:      "",
				BranchMatch:        true,
				ChangedFiles:       []string{},
				TaskRelatedChanges: []string{},
				UnrelatedChanges:   []string{},
				ConfidenceLevel:    state.ConfidenceNone,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := Reconcile(tt.cp, tt.repo, tt.taskFiles)
			if res.Status != tt.want.Status {
				t.Errorf("Reconcile() Status = %v, want %v", res.Status, tt.want.Status)
			}
			if !reflect.DeepEqual(res.ChangedFiles, tt.want.ChangedFiles) {
				t.Errorf("Reconcile() ChangedFiles = %v, want %v", res.ChangedFiles, tt.want.ChangedFiles)
			}
			if !reflect.DeepEqual(res.TaskRelatedChanges, tt.want.TaskRelatedChanges) {
				t.Errorf("Reconcile() TaskRelatedChanges = %v, want %v", res.TaskRelatedChanges, tt.want.TaskRelatedChanges)
			}
			if res.ConfidenceLevel != tt.want.ConfidenceLevel {
				t.Errorf("Reconcile() ConfidenceLevel = %v, want %v", res.ConfidenceLevel, tt.want.ConfidenceLevel)
			}
		})
	}
}

func TestFileCategorization(t *testing.T) {
	cp := state.Checkpoint{Branch: "main", Commit: "abc"}
	repo := git.RepositoryState{
		Branch:     "main",
		CommitHash: "abc",
		HasCommits: true,
		ModifiedFiles: []string{"mod.go"},
		UntrackedFiles: []string{"unt.go", ".wake/tmp.txt"},
		UnmergedFiles: []string{"unm.go"},
		StagedFiles: []git.FileStatus{
			{Path: "staged.go"},
		},
		UnstagedFiles: []git.FileStatus{
			{Path: "unstaged.go", OrigPath: "orig.go"},
		},
	}
	taskFiles := []string{"mod.go", "staged.go"}

	res := Reconcile(cp, repo, taskFiles)

	expectedChanged := []string{"mod.go", "orig.go", "staged.go", "unm.go", "unstaged.go", "unt.go"}
	if !reflect.DeepEqual(res.ChangedFiles, expectedChanged) {
		t.Errorf("ChangedFiles = %v, want %v", res.ChangedFiles, expectedChanged)
	}

	expectedTaskRelated := []string{"mod.go", "staged.go"}
	if !reflect.DeepEqual(res.TaskRelatedChanges, expectedTaskRelated) {
		t.Errorf("TaskRelatedChanges = %v, want %v", res.TaskRelatedChanges, expectedTaskRelated)
	}

	expectedUnrelated := []string{"orig.go", "unm.go", "unstaged.go", "unt.go"}
	if !reflect.DeepEqual(res.UnrelatedChanges, expectedUnrelated) {
		t.Errorf("UnrelatedChanges = %v, want %v", res.UnrelatedChanges, expectedUnrelated)
	}
}

func TestMatchSinglePattern(t *testing.T) {
	tests := []struct {
		pattern  string
		filepath string
		want     bool
	}{
		{"exact.go", "exact.go", true},
		{"Exact.go", "exact.go", true},
		{"auth", "auth/session.go", true},
		{"auth/", "auth/session.go", true},
		{"*.go", "main.go", true},
		{"*.go", "dir/main.go", true}, // Match on base name
		{"dir/*.go", "dir/main.go", true},
		{"*.go", "main.txt", false},
		{"nonmatch", "other/file", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.filepath, func(t *testing.T) {
			if got := matchSinglePattern(tt.pattern, tt.filepath); got != tt.want {
				t.Errorf("matchSinglePattern(%q, %q) = %v, want %v", tt.pattern, tt.filepath, got, tt.want)
			}
		})
	}
}

func TestMatchesConstraint(t *testing.T) {
	tests := []struct {
		filepath   string
		constraint string
		want       bool
	}{
		{"auth.go", "auth.go", true},
		{"internal/auth/session.go", "Do not modify internal/auth", true},
		{"internal/auth/session.go", "leave internal/auth/session.go alone", true},
		{"random.go", "Do not modify internal/auth", false},
		{"file.go", "v2.0", false},
		{"file.go", "https://example.com", false},
		{"file.go", "1.", false},
		{"file.go", "2.1", false},
		{"file.go", "Do not modify", false}, // Only stopwords
	}

	for _, tt := range tests {
		t.Run(tt.constraint+"_"+tt.filepath, func(t *testing.T) {
			if got := matchesConstraint(tt.filepath, tt.constraint); got != tt.want {
				t.Errorf("matchesConstraint(%q, %q) = %v, want %v", tt.filepath, tt.constraint, got, tt.want)
			}
		})
	}
}

func TestIsSafeRelativePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"valid/path.go", true},
		{"path.go", true},
		{"../escape.go", false},
		{"dir/../../escape.go", false},
		{"/absolute/path", false},
		{"\\absolute\\path", false},
		{"C:\\windows", false},
		{"c:/windows", false},
		{"\\\\server\\share", false},
		{"//server/share", false},
		{"", false},
		{".", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isSafeRelativePath(tt.path); got != tt.want {
				t.Errorf("isSafeRelativePath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestLooksLikeFilePath(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"foo.go", true},
		{"config.yaml", true},
		{"cmd/root.go", true},
		{"Makefile", true},
		{"Dockerfile", true},
		{"README", true},
		{"*.go", false},
		{"https://example.com", false},
		{"v1.0.0", false},
		{"i.e.", false},
		{"e.g.", false},
		{"#1", false},
		{"2.1", false},
		{"1.", false},
		{"../foo", false}, // unsafe relative path
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			if got := looksLikeFilePath(tt.s); got != tt.want {
				t.Errorf("looksLikeFilePath(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestEdgeCases(t *testing.T) {
	// Empty checkpoint vs empty repo state
	res := Reconcile(state.Checkpoint{}, git.RepositoryState{}, nil)
	if res.Status != StatusStale {
		t.Errorf("Empty states should be StatusStale, got %v", res.Status)
	}

	// Checkpoint with no commit, repo with commits
	res = Reconcile(state.Checkpoint{}, git.RepositoryState{CommitHash: "abc", HasCommits: true, Branch: "main"}, nil)
	if res.Status != StatusStale {
		t.Errorf("Checkpoint no commit, repo has commit should be StatusStale, got %v", res.Status)
	}

	// Constraints containing special regex characters
	constraint := "[a-z]+.*"
	filepath := "[a-z]+.*"
	if matchesConstraint(filepath, constraint) != true {
		t.Errorf("Should match literal special characters if it is exactly the same")
	}

	// Unicode file paths
	unicodePath := "🚀/test.go"
	if !isSafeRelativePath(unicodePath) {
		t.Errorf("isSafeRelativePath should accept unicode paths")
	}
}
