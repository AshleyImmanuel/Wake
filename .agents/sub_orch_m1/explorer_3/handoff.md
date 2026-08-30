# Explorer 3 Investigation Report: Package Dependencies, Import Graphs & Testutil Harness

## 1. Observation

### 1.1 `go.mod` Dependencies and Module Identity
- **File**: `go.mod` (lines 1-23)
- **Module Path**: `github.com/wake/wake`
- **Go Toolchain Version**: `go 1.27.0`
- **Direct Dependencies**:
  - `github.com/google/uuid v1.6.0`: Used for RFC 4122 UUID generation across events, state identifiers, and checkpoints.
  - `github.com/spf13/cobra v1.10.2`: Used for CLI command structuring in `cmd/`.
  - `modernc.org/sqlite v1.57.0`: Pure Go SQLite driver (CGO-free, registers under driver name `"sqlite"` with standard `database/sql`).
- **Indirect Dependencies**: `modernc.org/libc v1.74.4`, `modernc.org/mathutil v1.7.1`, `modernc.org/memory v1.11.0`, `github.com/dustin/go-humanize v1.0.1`, `github.com/mattn/go-isatty v0.0.24`, `github.com/ncruces/go-strftime v1.0.0`, `github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec`, `github.com/spf13/pflag v1.0.10`, `golang.org/x/sys v0.47.0`.

### 1.2 Current Package Dependency Mapping & Import Graphs
Direct inspection of imports across all existing packages reveals the following import topology:

| Package | File Path(s) | External & Stdlib Imports | Internal Package Imports | Dependency Tier |
|---|---|---|---|---|
| `internal/events` | `internal/events/models.go:3-7` | `time`, `github.com/google/uuid` | **None** | Tier 0 (Leaf) |
| `internal/git` | `internal/git/models.go`, `runner.go:3-10`, `parser.go:3-7`, `client.go:3-8`, `errors.go:3-7` | `bytes`, `context`, `errors`, `fmt`, `os/exec`, `path/filepath`, `sort`, `strings`, `sync` | **None** | Tier 0 (Leaf) |
| `internal/state` | `internal/state/models.go:3`, `internal/state/engine.go:3-5` | `github.com/google/uuid` | `github.com/wake/wake/internal/events` | Tier 1 (Core Domain) |
| `internal/db` | `internal/db/db.go:3-16` | `context`, `database/sql`, `encoding/json`, `fmt`, `os`, `path/filepath`, `time`, `github.com/google/uuid`, `modernc.org/sqlite` | `github.com/wake/wake/internal/events`, `github.com/wake/wake/internal/state` | Tier 2 (Persistence Store) |
| `internal/reconcile` | `internal/reconcile/models.go:3`, `internal/reconcile/engine.go:3-15` | `context`, `fmt`, `os`, `path`, `path/filepath`, `regexp`, `sort`, `strings` | `github.com/wake/wake/internal/git`, `github.com/wake/wake/internal/state` | Tier 2 (Engine) |
| `cmd` | `cmd/root.go:3-8`, `checkpoint.go:3-15`, `status.go:3-15`, `history.go:3-11`, `resume.go:3-12` | `context`, `database/sql`, `encoding/json`, `errors`, `fmt`, `os`, `time`, `github.com/google/uuid`, `github.com/spf13/cobra` | `github.com/wake/wake/internal/db`, `github.com/wake/wake/internal/events`, `github.com/wake/wake/internal/git`, `github.com/wake/wake/internal/reconcile`, `github.com/wake/wake/internal/state` | Tier 3 (CLI Presentation) |
| `main` | `main.go:3` | **None** | `github.com/wake/wake/cmd` | Tier 4 (Entrypoint) |

### 1.3 `internal/testutil` Role & Architecture Requirements
From `PROJECT.md` (lines 25-27, 169-173) and `SCOPE.md` (lines 7-11, 34-60):
- Target files to create in Milestone 1:
  - `internal/testutil/git.go`: `GitRepo` test fixture helper (`NewGitRepo(t testing.TB)`, `WriteFile`, `Commit`, `Branch`, `Checkout`, `Stage`, `Cleanup`).
  - `internal/testutil/db.go`: SQLite test database helper (`NewTestDB(t testing.TB) *sql.DB`, `NewTestDBPath(t testing.TB) string`).
  - `internal/testutil/fixtures.go`: Fixture builders (`SampleEvent(eventType string) events.Event`, `SampleCheckpoint() state.Checkpoint`, `SampleRepositoryState() git.RepositoryState`).
  - `internal/testutil/testutil_test.go`: Comprehensive unit tests verifying `internal/testutil` helpers themselves.

### 1.4 Test Suite Execution and Pre-Existing Failure
- Command: `& "C:\Program Files\Go\bin\go.exe" test ./...`
- Observed Output:
  - `ok github.com/wake/wake/cmd`
  - `ok github.com/wake/wake/internal/db`
  - `? github.com/wake/wake/internal/events [no test files]`
  - `FAIL github.com/wake/wake/internal/git`
  - `ok github.com/wake/wake/internal/reconcile`
  - `ok github.com/wake/wake/internal/state`
- Specific Failure in `internal/git/adversarial_test.go:206`:
  ```
  adversarial_test.go:206: ExtractModifiedFiles mismatch:
      expected: [deeply/nested/path with spaces/another file.go new name with space.txt normal_deleted.txt old name with space.txt path with spaces/my file.txt unicode_日本語_test.txt unicode_üñîçødé_файл.md]
      got:      [deeply/nested/path with spaces/another file.go new name with space.txt normal_deleted.txt old name with space.txt path with spaces/my file.txt unicode_üñîçødé_файл.md unicode_日本語_test.txt]
  ```
- Command: `& "C:\Program Files\Go\bin\go.exe" vet ./...`
  - Result: 0 warnings, exit code 0.

---

## 2. Logic Chain

### 2.1 Package Import Graph Directed Acyclic Graph (DAG) Structure
1. `internal/events` defines raw event types and payload maps with zero internal dependencies. It is completely isolated.
2. `internal/git` encapsulates OS git command execution, error classification, and porcelain parsing with zero internal dependencies. It is completely isolated.
3. `internal/state` consumes `internal/events` to implement `Reduce()`, and defines the core `State`, `Decision`, `Blocker`, and `Checkpoint` data models. It depends only on `events`.
4. `internal/db` persists `events.Event` and `state.Checkpoint` records into SQLite. It depends on `internal/events` and `internal/state`. It does not depend on `internal/git` or `internal/reconcile`.
5. `internal/reconcile` compares `state.Checkpoint` against `git.RepositoryState`. It depends on `internal/state` and `internal/git`. It does not depend on `internal/db` or `cmd`.
6. Therefore, the production dependency tree is strictly acyclic:
   ```
   [internal/events] (Tier 0)     [internal/git] (Tier 0)
          │                               │
          ▼                               │
   [internal/state] (Tier 1)              │
      ┌───┴───────────────┐               │
      ▼                   ▼               ▼
   [internal/db]       [internal/reconcile] (Tier 2)
      │                   │
      └───────────┬───────┘
                  ▼
         [internal/service] (Tier 3 - Planned)
                  │
                  ▼
                [cmd] (Tier 4)
                  │
                  ▼
               [main] (Tier 5)
   ```

### 2.2 Circular Import Prevention Rules for `internal/testutil`
When introducing `internal/testutil`, it will need to construct test fixtures and database instances that reference `internal/events`, `internal/state`, `internal/git`, and `internal/db`.

To guarantee that `internal/testutil` never creates circular import cycles in Go:
1. **Rule 1 — Strict Test-Only Scope**:
   - `internal/testutil` is an infrastructure package intended exclusively for test code.
   - **NO non-test `.go` file in ANY package (`events`, `state`, `db`, `git`, `reconcile`, `service`, `cmd`, `main`) may ever import `internal/testutil`**.
2. **Rule 2 — Tier 0 and Tier 1 Package Test Isolation**:
   - Because `internal/testutil` imports `internal/events` and `internal/state` to build fixture objects, any unit test residing in `package events` or `package state` that attempts to import `internal/testutil` would create an import cycle:
     - `internal/events` (test) -> `internal/testutil` -> `internal/events` (CYCLE ERROR).
   - Therefore, `internal/events` and `internal/state` unit tests must construct their own test structs directly using their package constructors (e.g. `events.NewEvent`, `state.State{...}`) and must NEVER import `internal/testutil`.
3. **Rule 3 — Tier 0 Git Isolation**:
   - `internal/testutil/git.go` implements a high-level `GitRepo` test fixture wrapping `exec.Command("git", ...)`.
   - If `testutil/git.go` does not import `internal/git`, then `internal/testutil` does not depend on `internal/git`.
   - If `testutil` provides a helper to obtain a `git.Client`, it imports `internal/git`. In this case, `internal/git` tests (`client_test.go`, `adversarial_test.go`) keep their self-contained `MockRunner` and do not import `internal/testutil`.
4. **Rule 4 — Permitted Consumers of `internal/testutil`**:
   - The following packages are at Tier 2, Tier 3, and Tier 4 in the DAG and may safely import `internal/testutil` within their `*_test.go` files:
     - `internal/reconcile` (`reconcile_test.go`, `engine_test.go`)
     - `internal/service` (`service_test.go`)
     - `cmd` (`checkpoint_test.go`, `status_test.go`, `history_test.go`, `resume_test.go`)

### 2.3 Verification of SQLite Driver Choice in `go.mod`
- `modernc.org/sqlite v1.57.0` is a 100% pure Go SQLite implementation generated from the SQLite C sources via ccgo.
- **Advantages for Sentinel / Wake**:
  - Completely eliminates the CGO requirement (`CGO_ENABLED=0` builds work).
  - Cross-platform out of the box on Windows, macOS, and Linux without requiring `gcc`, `mingw-w64`, or `clang` on host development environments.
  - Registered as the `"sqlite"` driver name with standard `database/sql`, so code uses standard `sql.Open("sqlite", dbPath)`.
- **Contrast with `github.com/mattn/go-sqlite3`**:
  - `mattn/go-sqlite3` requires CGO and an active C compiler toolchain on Windows, which introduces build fragility across developer and CI environments.
  - The current Sentinel codebase is already wired to `_ "modernc.org/sqlite"` in `internal/db/db.go:15`.

### 2.4 Testing Best Practices for `internal/testutil`
1. **`t.Helper()` Registration**:
   - Every exported function and method in `internal/testutil` (`NewGitRepo`, `WriteFile`, `Commit`, `Stage`, `Branch`, `Checkout`, `NewTestDB`, `SampleEvent`, `SampleCheckpoint`) must start with `t.Helper()`.
   - Rationale: Ensures that when an assertion fails inside a helper, Go test output points directly to the line of code in the caller test file rather than the internal line in `testutil`.
2. **`t.Cleanup()` Teardown**:
   - `NewGitRepo(t testing.TB)` and `NewTestDB(t testing.TB)` must register cleanup routines via `t.Cleanup()` automatically:
     - For `NewTestDB`: `t.Cleanup(func() { _ = db.Close() })`.
     - For `NewGitRepo`: automatic directory cleanup is handled by `t.TempDir()`, while file lock release / process cleanup is registered via `t.Cleanup()`.
   - Rationale: Guarantees that resources (open SQLite database connections, locked files) are cleanly released even if test assertions fail or abort early.
3. **`testing.TB` Interface**:
   - Accept `testing.TB` (the standard Go interface implemented by `*testing.T`, `*testing.B`, and `*testing.F`) rather than strictly `*testing.T`.
   - Rationale: Allows the test harness to be reused across unit tests, benchmarks, and fuzz tests.
4. **Robust Diagnostics & Error Reporting**:
   - Test helpers must capture stdout and stderr from OS commands, check all errors, and format comprehensive error messages via `t.Fatalf("git %v failed in %s: %v\nOutput: %s", args, dir, err, string(out))`.
5. **Self-Verification Suite (`testutil_test.go`)**:
   - `internal/testutil/testutil_test.go` must test all harness capabilities:
     - `TestGitRepo_Lifecycle`: Verifies repo init, `WriteFile`, `Stage`, `Commit`, `Branch`, `Checkout`, `IsClean`.
     - `TestDB_NewTestDB`: Verifies database connection, schema migration application, and query readiness.
     - `TestFixtures_SampleBuilders`: Verifies validity of generated `SampleEvent`, `SampleCheckpoint`, and `SampleRepositoryState`.

### 2.5 Root Cause of UTF-8 Byte Sort Order Failure in `internal/git/adversarial_test.go:206`
1. `internal/git/parser.go:161` uses standard Go `sort.Strings(result)` in `ExtractModifiedFiles()`.
2. Go's `sort.Strings` compares strings lexicographically by byte values (Unicode UTF-8 byte sequences).
3. In `adversarial_test.go:202-204`, the test expected:
   - `[... "unicode_日本語_test.txt", "unicode_üñîçødé_файл.md"]`
4. Byte comparison after common prefix `"unicode_"`:
   - `'ü'` is UTF-8 encoded as bytes `0xC3 0xBC` (first byte: `0xC3` = 195).
   - `'日'` is UTF-8 encoded as bytes `0xE6 0x97 0xA5` (first byte: `0xE6` = 230).
   - Since `0xC3` < `0xE6`, `"unicode_üñîçødé_файл.md"` precedes `"unicode_日本語_test.txt"` in byte order.
5. Therefore, the implementation in `parser.go` is correct; the test expectation in `adversarial_test.go:202-203` had inverted the two elements.

---

## 3. Caveats

1. **Read-Only Investigation**:
   - In accordance with constraints, no production or test source files were modified during this investigation.
   - The fix for `internal/git/adversarial_test.go:206` and the implementation of `internal/testutil` are ready for Milestone 1 implementers.
2. **Go Toolchain Path on Windows**:
   - The `go` command is not in the system default `PATH` in this environment, but is located at `C:\Program Files\Go\bin\go.exe`. All subagents and automated scripts should invoke `& "C:\Program Files\Go\bin\go.exe"`.
3. **Database File Cleanup**:
   - When using SQLite in tests, SQLite creates temporary `-wal` and `-shm` files when WAL mode is active. Harness implementations must ensure `db.Close()` is called prior to `t.TempDir()` removal to avoid Windows file lock errors (`ERROR_SHARING_VIOLATION`).

---

## 4. Conclusion

1. **Dependency Graph Integrity**:
   - The Sentinel codebase has a clean, acyclic 5-tier architecture.
   - `internal/events` and `internal/git` are pure Tier 0 leaf packages.
2. **`internal/testutil` Specifications**:
   - Files to implement in `internal/testutil/`:
     - `git.go`: `GitRepo` test fixture helper with `NewGitRepo(t testing.TB)`, `WriteFile`, `Commit`, `Branch`, `Checkout`, `Stage`, `Cleanup`.
     - `db.go`: SQLite test fixture helper with `NewTestDB(t testing.TB) *sql.DB`, `NewTestDBPath(t testing.TB) string`.
     - `fixtures.go`: `SampleEvent(eventType string) events.Event`, `SampleCheckpoint() state.Checkpoint`, `SampleRepositoryState() git.RepositoryState`.
     - `testutil_test.go`: Harness self-testing suite.
3. **Circular Import Rules**:
   - No production `.go` file may import `internal/testutil`.
   - `internal/events` and `internal/state` unit tests must construct fixtures locally and not import `internal/testutil`.
   - `internal/reconcile`, `internal/service`, and `cmd` unit tests should consume `internal/testutil` to eliminate code duplication.
4. **Third-Party Dependencies (`go.mod`)**:
   - `modernc.org/sqlite v1.57.0`, `github.com/google/uuid v1.6.0`, and `github.com/spf13/cobra v1.10.2` are properly configured and require no additional external dependencies.
5. **Pre-Existing Failure Fix**:
   - Swap lines 202 and 203 in `internal/git/adversarial_test.go` to match Go's byte sort order (`"unicode_ü..."` before `"unicode_日..."`).

---

## 5. Verification Method

### 5.1 Test Execution Commands
Run the complete test suite and code vet check:
```powershell
# 1. Run all existing tests across all packages
& "C:\Program Files\Go\bin\go.exe" test -v ./...

# 2. Run static analysis (go vet) across all packages
& "C:\Program Files\Go\bin\go.exe" vet ./...

# 3. Test git package specifically to verify sort order fix
& "C:\Program Files\Go\bin\go.exe" test -v -run TestAdversarial_FilenamesWithSpacesAndUnicode ./internal/git

# 4. Verify testutil once implemented
& "C:\Program Files\Go\bin\go.exe" test -v ./internal/testutil
```

### 5.2 Circular Dependency Invalidation Condition
Run `& "C:\Program Files\Go\bin\go.exe" build ./...` after implementing `internal/testutil`.
- **Pass Condition**: Builds with 0 errors.
- **Fail Condition**: Any output containing `import cycle not allowed`.
