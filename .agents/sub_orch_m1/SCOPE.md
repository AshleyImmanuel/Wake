# Scope: Milestone 1 - Test Infrastructure & Shared Harness

## Architecture & Responsibilities
Milestone 1 establishes standard test utilities and shared fixtures for the entire Wake test suite, and resolves the pre-existing test failure in `internal/git/adversarial_test.go`.

```
[internal/testutil]
  ├── git.go      <- GitRepo test fixture helper (init, commit, branch, dirty tree, cleanup)
  ├── db.go       <- Test SQLite database helper (temp file / memory DB initialization & schema setup)
  ├── fixtures.go <- Test fixture builders (Events, Checkpoints, GitStatus, FileChanges)
  └── testutil_test.go <- Verification tests for internal/testutil

[internal/git]
  └── adversarial_test.go <- Fix UTF-8 byte sort order expectation on line 206
```

## Feature Inventory
| # | Feature | Description | Milestone | Source |
|---|---------|-------------|-----------|--------|
| 1 | Shared Test Harness & Fixture Utility | `internal/testutil` with `GitRepo` fixture, SQLite test helper, and fixture builders | M1 | Survey & Test Gap |
| 2 | UTF-8 Byte Sort Order Fix | Fix inverted UTF-8 string sort order in `internal/git/adversarial_test.go:206` | M1 | Survey & Test Fix |

## Milestones
| # | Sub-Milestone / Phase | Scope | Dependencies | Status |
|---|-----------------------|-------|-------------|--------|
| 1.1 | Exploration & Survey | Inspect `internal/git/adversarial_test.go` failure & test fixture requirements | none | IN_PROGRESS |
| 1.2 | Implementation | Implement `internal/testutil` and fix `internal/git/adversarial_test.go` | 1.1 | PLANNED |
| 1.3 | Review & Verification | Code review for completeness, clean design, zero regressions | 1.2 | PLANNED |
| 1.4 | Challenger Stress Test | Adversarial validation of `testutil` helpers and git sort order | 1.3 | PLANNED |
| 1.5 | Forensic Audit & Gate | Integrity audit (no facades, genuine logic) and gate pass | 1.4 | PLANNED |

## Interface Contracts & Package Specifications

### `internal/testutil`
```go
package testutil

// GitRepo wraps a temporary git repository for tests
type GitRepo struct {
    Dir string
    T   testing.TB
}

func NewGitRepo(t testing.TB) *GitRepo
func (g *GitRepo) WriteFile(relPath, content string)
func (g *GitRepo) Commit(msg string) string
func (g *GitRepo) Branch(name string)
func (g *GitRepo) Checkout(branchOrCommit string)
func (g *GitRepo) Stage(relPath string)
func (g *GitRepo) Cleanup()

// DB Test helpers
func NewTestDB(t testing.TB) *sql.DB
func NewTestDBPath(t testing.TB) string

// Fixture Builders
func SampleEvent(eventType string) events.Event
func SampleCheckpoint() state.Checkpoint
```

## Code Layout Ownership for Milestone 1
- `internal/testutil/git.go` (new)
- `internal/testutil/db.go` (new)
- `internal/testutil/fixtures.go` (new)
- `internal/testutil/testutil_test.go` (new)
- `internal/git/adversarial_test.go` (modify sort expectation at line 206)
