# Handoff Report: Milestone 1 Test Infrastructure & Testutil Specification

**Author**: Explorer 2 (Milestone 1)  
**Date**: 2026-08-28T20:25:00Z  
**Scope**: Test Infrastructure & Shared Harness (`internal/testutil`), Existing Test Pattern Analysis, Fixture Specification, and Test Fixes.

---

## 1. Observation

### 1.1 Existing Test Harness Duplication
A comprehensive survey of the test suites across `internal/git`, `internal/db`, `internal/state`, `internal/reconcile`, and `cmd` revealed heavy fragmentation and duplication of test setup logic:

1. **Git Repository Test Setup Duplication**:
   - `internal/reconcile/reconcile_test.go` (lines 18-167): Defines a private `gitTestRepo` struct wrapping `t.TempDir()`, `locateGitBinary()`, `initGitTestRepo()`, `runGit()`, `runGitAllowError()`, `writeFile()`, `deleteFile()`, `commitAll()`, `currentCommit()`, `currentBranch()`, `createBranch()`, `checkoutBranch()`, and `makeTestCheckpoint()`.
   - `internal/git/client_test.go` (lines 222-245): Defines an inline anonymous helper function in `TestIntegration_RealGitRepositoryLifecycle` executing `exec.CommandContext(ctx, "git", ...)`, configuring git username/email/gpgsign, and setting up file states.
   - `internal/git/lifecycle_adversarial_test.go` (lines 14-32, 40-52): Defines standalone package-level helper functions `runGitCommand()` and `runGitCommandAllowError()` that configure git test repos and execute raw commands.
   - `cmd/checkpoint_test.go` (lines 14-60): Defines standalone `findGit()` and `setupTempGitRepo(t *testing.T)` which manually initializes a git repo and writes a default `README.md`.
   - `cmd/status_test.go` (lines 13-41): Re-duplicates `setupTempGitRepoForStatus(t *testing.T)` with identical git config and file commit logic.

2. **SQLite Test Database Duplication**:
   - `internal/db/db_test.go` (lines 15-32, 34-41, 140-146): Repeatedly calls `tmpDir := t.TempDir()` followed by `db, err := InitDB(tmpDir)` and `defer db.Close()`.
   - `cmd/checkpoint_test.go` (lines 74-78): Manually calls `db.InitDB(repoDir)` and `defer database.Close()`.
   - No shared in-memory database helper (`:memory:`) or automated `t.Cleanup()` wrapper exists.

3. **Ad-Hoc Event and Checkpoint Fixtures**:
   - `internal/state/engine_test.go` (lines 11-25, 32-67, 69-97): Constructs ad-hoc `events.Event` structs with manual string payloads (`"objective"`, `"constraint"`, `"id"`, `"description"`, `"milestone"`, `"hash"`).
   - `internal/reconcile/engine_test.go` (lines 16-37): Handcrafts `newTestCheckpoint(commit, branch string)` and a custom `mockGitClient` struct (lines 656-710).
   - `internal/reconcile/reconcile_test.go` (lines 143-167): Handcrafts `makeTestCheckpoint(repo *gitTestRepo, commit string)`.

4. **Pre-Existing Test Failure in `internal/git/adversarial_test.go`**:
   - Executing `go test ./...` produces the following verbatim test failure:
     ```
     --- FAIL: TestAdversarial_FilenamesWithSpacesAndUnicode (0.00s)
         adversarial_test.go:206: ExtractModifiedFiles mismatch:
             expected: [deeply/nested/path with spaces/another file.go new name with space.txt normal_deleted.txt old name with space.txt path with spaces/my file.txt unicode_日本語_test.txt unicode_üñîçødé_файл.md]
             got:      [deeply/nested/path with spaces/another file.go new name with space.txt normal_deleted.txt old name with space.txt path with spaces/my file.txt unicode_üñîçødé_файл.md unicode_日本語_test.txt]
     FAIL
     FAIL	github.com/wake/wake/internal/git	4.538s
     ```
   - In `internal/git/parser.go` line 161, `ExtractModifiedFiles` invokes standard Go `sort.Strings(result)`.
   - In UTF-8 byte encoding:
     - `unicode_üñîçødé_файл.md` starts with byte `0xC3` ('ü')
     - `unicode_日本語_test.txt` starts with byte `0xE6` ('日')
     - `0xC3 < 0xE6`, so `"unicode_üñîçødé_файл.md"` correctly precedes `"unicode_日本語_test.txt"` in Go's standard lexicographical byte sort.
   - Lines 202-203 of `internal/git/adversarial_test.go` specified the test expectation in reversed order.

---

## 2. Logic Chain

1. **Test Infrastructure Consolidation**:
   - Because 5 different test files implement identical or near-identical temporary Git repository setups (`init`, `config user.name`, `config user.email`, `config commit.gpgsign false`, `add`, `commit`, `branch`, `checkout`), introducing `internal/testutil/git.go` provides a single authoritative implementation (`GitRepo`).
   - `GitRepo` should accept `testing.TB` (supporting `*testing.T` and `*testing.B`), encapsulate `t.TempDir()`, manage Windows binary resolution via PATH and common install locations, configure consistent git options (`core.quotepath false`), and provide fluent helper methods for file authoring, staging, committing, and branching.

2. **Database Test Fixture Unification**:
   - `internal/testutil/db.go` should provide `NewTestDB(t testing.TB) *sql.DB` and `NewInMemoryDB(t testing.TB) *sql.DB` which automatically invoke `t.Cleanup(func() { _ = db.Close() })` and apply migrations.
   - This eliminates boilerplate and guarantees that all test database connections are closed cleanly across subtests and teardown.

3. **Fixture Utilities (`internal/testutil/fixtures.go`)**:
   - To support all current and future milestones (M2 17-event reduction, M3 database store tests, M4 reconciler benchmarks, M5 CLI command tests), `internal/testutil/fixtures.go` should provide constructors for:
     - All 17 `events.EventType` constants with schema-compliant payloads.
     - Deterministic chronological event streams (`SampleEventSequence`).
     - Fully populated `state.Checkpoint`, `state.State`, `state.Decision`, and `state.Blocker` instances.
     - `git.RepositoryState`, `git.FileStatus`, and `git.FileChange` instances.

4. **Adversarial Test Fix**:
   - Correcting the slice order in `internal/git/adversarial_test.go:202-203` brings the test expectation into 100% alignment with Go's standard UTF-8 byte ordering behavior in `sort.Strings()`.

---

## 3. Caveats

- **Network Isolation**: Tests relying on `testutil.NewGitRepo` execute local `git` subprocesses on the host machine. If `git` is not installed on the host PATH or Windows standard directories, git integration tests should call `t.Skip` gracefully.
- **SQLite Concurrency in Tests**: SQLite in-memory databases (`:memory:`) are per-connection. For tests requiring multi-connection concurrency or transaction testing (e.g. Milestone 3), file-based temp databases via `NewTestDB(t)` are recommended over shared-memory URLs.
- **No Circular Imports**: `internal/testutil` imports `internal/events`, `internal/state`, `internal/git`, and `internal/db`. Tests in `internal/reconcile`, `internal/service`, `cmd`, and external test packages can import `internal/testutil` safely.

---

## 4. Conclusion & Complete Design Specifications

The recommended implementation of `internal/testutil` consists of four files in `internal/testutil/` and a one-line slice reordering fix in `internal/git/adversarial_test.go`.

### 4.1 Specification: `internal/testutil/git.go`

```go
package testutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wake/wake/internal/git"
)

// GitRepo encapsulates a real, isolated Git repository in a temporary directory for testing.
type GitRepo struct {
	Dir     string
	GitPath string
	T       testing.TB
	client  git.Client
}

// locateGit finds the git executable across PATH and standard Windows installation paths.
func locateGit() string {
	if p, err := exec.LookPath("git"); err == nil && p != "" {
		return p
	}
	for _, candidate := range []string{
		`C:\Program Files\Git\cmd\git.exe`,
		`C:\Program Files\Git\bin\git.exe`,
		`C:\Program Files (x86)\Git\cmd\git.exe`,
		`C:\Program Files (x86)\Git\bin\git.exe`,
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "git"
}

// NewGitRepo initializes a fresh, temporary Git repository configured for tests.
func NewGitRepo(t testing.TB) *GitRepo {
	t.Helper()
	gitBin := locateGit()
	if _, err := exec.LookPath(gitBin); err != nil && gitBin == "git" {
		t.Skip("git binary not available on host system, skipping git test")
	}

	tmpDir := t.TempDir()
	repo := &GitRepo{
		Dir:     tmpDir,
		GitPath: gitBin,
		T:       t,
		client:  git.NewClient(nil),
	}

	// Initialize repository with main branch
	if _, err := repo.RunGitAllowError("init", "-b", "main"); err != nil {
		repo.RunGit("init")
	}

	// Configure identity and flags
	repo.RunGit("config", "user.name", "Sentinel Test")
	repo.RunGit("config", "user.email", "test@sentinel.local")
	repo.RunGit("config", "commit.gpgsign", "false")
	repo.RunGit("config", "core.quotepath", "false")

	return repo
}

// WriteFile writes content to a relative file path inside the repository, creating parent dirs if needed.
func (g *GitRepo) WriteFile(relPath, content string) {
	g.T.Helper()
	fullPath := filepath.Join(g.Dir, filepath.FromSlash(relPath))
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		g.T.Fatalf("failed to create directory %s: %v", dir, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		g.T.Fatalf("failed to write file %s: %v", fullPath, err)
	}
}

// ReadFile reads and returns the string content of a file relative to repository root.
func (g *GitRepo) ReadFile(relPath string) string {
	g.T.Helper()
	fullPath := filepath.Join(g.Dir, filepath.FromSlash(relPath))
	data, err := os.ReadFile(fullPath)
	if err != nil {
		g.T.Fatalf("failed to read file %s: %v", fullPath, err)
	}
	return string(data)
}

// DeleteFile removes a physical file from the repository working tree.
func (g *GitRepo) DeleteFile(relPath string) {
	g.T.Helper()
	fullPath := filepath.Join(g.Dir, filepath.FromSlash(relPath))
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		g.T.Fatalf("failed to delete file %s: %v", fullPath, err)
	}
}

// Stage stages one or more relative file paths. If no paths are provided, runs 'git add -A'.
func (g *GitRepo) Stage(relPaths ...string) {
	g.T.Helper()
	if len(relPaths) == 0 {
		g.RunGit("add", "-A")
		return
	}
	args := append([]string{"add"}, relPaths...)
	g.RunGit(args...)
}

// Commit stages all changes and creates a new commit, returning the full commit hash.
func (g *GitRepo) Commit(msg string) string {
	g.T.Helper()
	g.RunGit("add", "-A")
	g.RunGit("commit", "-m", msg)
	return g.CurrentCommit()
}

// CommitOnly stages specific files and commits them, returning the commit hash.
func (g *GitRepo) CommitOnly(msg string, relPaths ...string) string {
	g.T.Helper()
	g.Stage(relPaths...)
	g.RunGit("commit", "-m", msg)
	return g.CurrentCommit()
}

// Branch creates a new branch without switching to it.
func (g *GitRepo) Branch(name string) {
	g.T.Helper()
	g.RunGit("branch", name)
}

// CreateAndCheckoutBranch creates and checks out a new branch.
func (g *GitRepo) CreateAndCheckoutBranch(name string) {
	g.T.Helper()
	g.RunGit("checkout", "-b", name)
}

// Checkout switches to an existing branch or commit hash.
func (g *GitRepo) Checkout(branchOrCommit string) {
	g.T.Helper()
	g.RunGit("checkout", branchOrCommit)
}

// CurrentCommit returns the full commit hash of HEAD.
func (g *GitRepo) CurrentCommit() string {
	g.T.Helper()
	return g.RunGit("rev-parse", "HEAD")
}

// CurrentBranch returns the current active branch name.
func (g *GitRepo) CurrentBranch() string {
	g.T.Helper()
	return g.RunGit("rev-parse", "--abbrev-ref", "HEAD")
}

// Client returns a git.Client bound to the repository directory.
func (g *GitRepo) Client() git.Client {
	return g.client
}

// GetState returns the current live RepositoryState snapshot.
func (g *GitRepo) GetState() *git.RepositoryState {
	g.T.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	state, err := g.client.GetState(ctx, g.Dir)
	if err != nil {
		g.T.Fatalf("GetState failed: %v", err)
	}
	return state
}

// RunGit executes a git command in the repository directory and returns standard output trimmed.
func (g *GitRepo) RunGit(args ...string) string {
	g.T.Helper()
	cmd := exec.Command(g.GitPath, args...)
	cmd.Dir = g.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		g.T.Fatalf("git %v failed in %s: %v\nOutput: %s", args, g.Dir, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

// RunGitAllowError executes a git command allowing non-zero exit codes.
func (g *GitRepo) RunGitAllowError(args ...string) (string, error) {
	g.T.Helper()
	cmd := exec.Command(g.GitPath, args...)
	cmd.Dir = g.Dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// CauseConflict simulates a merge conflict between current branch and targetBranch on conflictFile.
func (g *GitRepo) CauseConflict(targetBranch, conflictFile, baseContent, currentContent, targetContent string) {
	g.T.Helper()
	baseBranch := g.CurrentBranch()

	g.WriteFile(conflictFile, baseContent)
	g.Commit("Base commit for conflict")

	g.CreateAndCheckoutBranch(targetBranch)
	g.WriteFile(conflictFile, targetContent)
	g.Commit("Target branch conflicting edit")

	g.Checkout(baseBranch)
	g.WriteFile(conflictFile, currentContent)
	g.Commit("Current branch conflicting edit")

	_, _ = g.RunGitAllowError("merge", targetBranch)
}

// Cleanup performs repository cleanup.
func (g *GitRepo) Cleanup() {
	// Standard temp dir cleanup is handled by testing.TB
}
```

---

### 4.2 Specification: `internal/testutil/db.go`

```go
package testutil

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/wake/wake/internal/db"
	_ "modernc.org/sqlite"
)

// SchemaDDL contains the baseline table definitions for events and checkpoints.
const SchemaDDL = `
CREATE TABLE IF NOT EXISTS events (
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL,
	type TEXT NOT NULL,
	timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
	payload TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS checkpoints (
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL,
	timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
	commit_hash TEXT NOT NULL,
	state_version INTEGER NOT NULL,
	event_position INTEGER NOT NULL,
	state_data TEXT NOT NULL,
	repository TEXT DEFAULT '',
	branch TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_events_task_id ON events (task_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_checkpoints_task_id ON checkpoints (task_id, timestamp DESC);
`

// NewTestDB initializes a file-backed SQLite test database in a temporary directory.
// It automatically closes the database when the test completes.
func NewTestDB(t testing.TB) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	database, err := db.InitDB(tmpDir)
	if err != nil {
		t.Fatalf("NewTestDB failed to initialize database in %s: %v", tmpDir, err)
	}

	t.Cleanup(func() {
		_ = database.Close()
	})

	return database
}

// NewTestDBPath creates a temporary project root with an initialized .sentinel/state.db.
// Returns the root directory path.
func NewTestDBPath(t testing.TB) string {
	t.Helper()
	tmpDir := t.TempDir()
	database, err := db.InitDB(tmpDir)
	if err != nil {
		t.Fatalf("NewTestDBPath failed: %v", err)
	}
	_ = database.Close()
	return tmpDir
}

// NewInMemoryDB initializes an in-memory SQLite database with migrations applied.
func NewInMemoryDB(t testing.TB) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite database: %v", err)
	}

	if _, err := database.Exec(SchemaDDL); err != nil {
		_ = database.Close()
		t.Fatalf("failed to execute schema migrations on in-memory db: %v", err)
	}

	t.Cleanup(func() {
		_ = database.Close()
	})

	return database
}

// CountRows returns the number of rows in the specified table.
func CountRows(t testing.TB, database *sql.DB, tableName string) int {
	t.Helper()
	var count int
	query := fmt.Sprintf("SELECT count(*) FROM %s", tableName)
	if err := database.QueryRow(query).Scan(&count); err != nil {
		t.Fatalf("CountRows failed on table %s: %v", tableName, err)
	}
	return count
}
```

---

### 4.3 Specification: `internal/testutil/fixtures.go`

```go
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
```

---

### 4.4 Specification: `internal/testutil/testutil_test.go`

```go
package testutil

import (
	"context"
	"testing"

	"github.com/wake/wake/internal/db"
	"github.com/wake/wake/internal/events"
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

	// 3. Branching
	repo.CreateAndCheckoutBranch("feature-test")
	if repo.CurrentBranch() != "feature-test" {
		t.Errorf("expected branch feature-test, got %s", repo.CurrentBranch())
	}

	repo.WriteFile("feature.txt", "feature content\n")
	commit2 := repo.Commit("Feature commit")

	// 4. Checkout main
	repo.Checkout("main")
	if repo.CurrentBranch() != "main" {
		t.Errorf("expected branch main, got %s", repo.CurrentBranch())
	}

	// 5. Delete file
	repo.WriteFile("temp.txt", "temp")
	repo.DeleteFile("temp.txt")

	_ = commit2
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

	for _, et := range allTypes {
		ev := SampleEvent(et)
		if ev.Type != et {
			t.Errorf("expected event type %s, got %s", et, ev.Type)
		}
		if ev.Payload == nil {
			t.Errorf("expected non-nil payload for event type %s", et)
		}
	}
}

func TestFixtures_SampleSequence(t *testing.T) {
	taskID := SampleTaskID()
	seq := SampleEventSequence(taskID)
	if len(seq) < 5 {
		t.Fatalf("expected at least 5 events in sequence, got %d", len(seq))
	}
	for _, ev := range seq {
		if ev.TaskID != taskID {
			t.Errorf("expected task ID %s, got %s", taskID, ev.TaskID)
		}
	}
}
```

---

### 4.5 Specification: UTF-8 Byte Sort Order Fix in `internal/git/adversarial_test.go`

**Target File**: `internal/git/adversarial_test.go`  
**Line Range**: 202-203  

**Diff Snippet**:
```diff
--- a/internal/git/adversarial_test.go
+++ b/internal/git/adversarial_test.go
@@ -201,8 +201,8 @@ func TestAdversarial_FilenamesWithSpacesAndUnicode(t *testing.T) {
 		"old name with space.txt",
 		"path with spaces/my file.txt",
-		"unicode_日本語_test.txt",
 		"unicode_üñîçødé_файл.md",
+		"unicode_日本語_test.txt",
 	}
 	if !reflect.DeepEqual(modified, expectedModified) {
 		t.Errorf("ExtractModifiedFiles mismatch:\nexpected: %v\ngot:      %v", expectedModified, modified)
```

---

## 5. Verification Method

To verify these findings and specifications:

1. **Verify UTF-8 Byte Ordering**:
   - Inspect byte representation: `'ü'` is UTF-8 `0xC3 0xBC` (`195 188`), `'日'` is UTF-8 `0xE6 0x97 0xA5` (`230 151 165`).
   - `0xC3 < 0xE6`, confirming that `"unicode_üñîçødé_файл.md"` precedes `"unicode_日本語_test.txt"` in Go's `sort.Strings()`.

2. **Verify Codebase Test Execution**:
   - Run: `& 'C:\Program Files\Go\bin\go.exe' test -v ./...`
   - Observe that after applying the fix in `internal/git/adversarial_test.go`, all tests across `github.com/wake/wake/...` pass.

3. **Verify `internal/testutil` Functionality**:
   - Once implemented in Milestone 1.2, run `& 'C:\Program Files\Go\bin\go.exe' test -v ./internal/testutil` to verify all testutil methods pass without error.
