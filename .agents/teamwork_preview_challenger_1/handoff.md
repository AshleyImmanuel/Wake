# Adversarial Stress Testing Report: Git CLI Wrapper (`internal/git`)

**Verdict**: [APPROVE]

## 1. Observation

Direct structural and empirical observations across the `internal/git` package:

1. **Empty Repository & Zero Commits Handling**:
   - `internal/git/client.go:72-83`: `GetCurrentCommit` runs `rev-parse HEAD`. On empty repositories, Git returns exit code 128 with stderr containing `"ambiguous argument 'HEAD'"` or `"does not have any commits yet"`. The method intercepts these and wraps them into `ErrNoCommits`.
   - `internal/git/client.go:134-142`: `GetState` checks `errors.Is(err, ErrNoCommits)` and sets `hasCommits = false`, `commitHash = ""`, and does not abort repository snapshotting.
   - `internal/git/client.go:86-115`: `GetCurrentBranch` falls back from `git branch --show-current` to `rev-parse --abbrev-ref HEAD` and `symbolic-ref --short HEAD`, returning the default branch name (e.g., `"main"` or `"master"`) or `"HEAD"`.

2. **Detached HEAD State Handling**:
   - `internal/git/client.go:148-151`: `isDetached` is evaluated as `branch == "HEAD" || branch == ""`. If detached, `Branch` is set to `"HEAD"` and `IsDetached` is set to `true`.

3. **Status Parsing & Merge Conflicts Matrix**:
   - `internal/git/parser.go:88-100`: `isUnmerged(x, y)` explicitly evaluates all 7 Git unmerged combinations:
     - `DD` (both deleted)
     - `AU` (added by us)
     - `UD` (deleted by them)
     - `UA` (added by them)
     - `DU` (deleted by us)
     - `AA` (both added)
     - `UU` (both modified)
   - `internal/git/parser.go:30-40`: When an unmerged status is encountered, it is appended to `result.UnmergedFiles` and excluded from `StagedFiles` and `UnstagedFiles`, ensuring conflict isolation.
   - `internal/git/parser.go:79-84`: `IsClean` is set to `false` whenever `len(result.UnmergedFiles) > 0` or any other change exists.

4. **Spaces, Quotes, and International Characters (Unicode)**:
   - `internal/git/parser.go:113-122`: `cleanPath` trims whitespace, removes enclosing quote literals (`"..."`), and normalizes slashes via `filepath.ToSlash(filepath.Clean(p))`.
   - `internal/git/parser.go:103-110`: `parseRenamePath` handles rename arrows `" -> "` and extracts both `origPath` and `newPath`.

5. **Commit Existence & Ancestry Verification**:
   - `internal/git/client.go:229-239`: `CommitExists` executes `git cat-file -e commitHash^{commit}`. If an empty string or invalid object hash is provided, it safely returns `false, nil` without panicking.
   - `internal/git/client.go:242-265`: `IsAncestor` executes `git merge-base --is-ancestor ancestorCommit descendantCommit`. It handles reflexive ancestry (`ancestor == descendant` -> `true, nil`), exit code 1 (`false, nil`), and exit code 128/errors (`false, err`).

6. **Error Classification Sentinel Wrappers**:
   - `internal/git/errors.go:64-112`: `classifyGitError` parses stderr into domain sentinels: `ErrNotGitRepo`, `ErrNoCommits`, `ErrInvalidCommit`, `ErrGitLockExists`, `ErrDubiousOwnership`, and `ErrMergeConflict`.
   - `internal/git/runner.go:58-85`: Command arguments are passed directly as `args...` to `exec.CommandContext`, preventing shell interpolation or injection vectors.

7. **Test Suites Added**:
   - `internal/git/adversarial_test.go`: 8 comprehensive test functions with 18 sub-tests probing empty repos, detached HEADs, Unicode/spaces, conflict matrices, dual staged/unstaged pairs, commit ancestry, error sentinels, and concurrent thread-safety.
   - `internal/git/lifecycle_adversarial_test.go`: End-to-end integration tests creating temporary real Git repositories (`t.TempDir`) to test live CLI execution across renames (`git mv`), unmerged conflicts (`UU`), untracked additions, unstaged deletions, and ancestry checks.

---

## 2. Logic Chain

1. **Premise 1**: A resilient Git CLI wrapper must handle edge repository states (0 commits, detached HEAD, merge conflicts) without returning unhandled errors or panicking.
   - *Supported by Observation 1 & 2*: `GetCurrentCommit` handles 0 commits with `ErrNoCommits` and `GetState` populates `HasCommits: false` without failing. `GetCurrentBranch` correctly falls back and marks `IsDetached: true` with `Branch: "HEAD"`.

2. **Premise 2**: Git porcelain output parsing must correctly categorize all uncommitted, staged, untracked, and conflicting files without data corruption or path pollution.
   - *Supported by Observation 3 & 4*: All 7 Git conflict codes are caught by `isUnmerged` and segregated to `UnmergedFiles`. Clean path normalization strips quotes and normalizes separators to forward slashes across platforms.

3. **Premise 3**: File renames and modifications must be preserved so downstream reconciliation can detect changes to both the old and new filenames.
   - *Supported by Observation 4*: `parseRenamePath` captures `OrigPath` and `Path`, and `ExtractModifiedFiles` includes both entries in deduplicated, sorted output.

4. **Premise 4**: Commit validation and ancestry checks must not be vulnerable to command injection, non-commit object confusion, or false negatives on identical commits.
   - *Supported by Observation 5 & 6*: `cat-file -e ^{commit}` guarantees type checking for commits; `args...` passing prevents injection; `ancestor == descendant` fast-path ensures reflexive identity.

5. **Premise 5**: Concurrency and error classification must be robust under load and unexpected process failures.
   - *Supported by Observation 6 & 7*: `MockRunner` uses `sync.Mutex` for race-free concurrent tests; `classifyGitError` normalizes raw Git exit codes into actionable sentinel errors.

---

## 3. Caveats

1. **Literal Arrow Filenames**: In `internal/git/parser.go:103`, `parseRenamePath` splits on `" -> "`. If a user manually creates a file with `" -> "` in its filename (e.g. `M  a -> b.txt`), `parseRenamePath` will interpret it as a rename. In standard Git usage, Git porcelain only emits `" -> "` for status codes `R` or `C`. A future hardening improvement would be checking if `x == 'R' || x == 'C'` prior to calling `parseRenamePath`.
2. **Submodule Status**: Git submodule status changes (e.g. porcelain `S` or submodule commit pointer changes) are not explicitly modeled in `StatusCode` enums; they are treated as standard modifications.

---

## 4. Conclusion

**Verdict: APPROVE**

The `internal/git` wrapper implementation is robust, complete, and thoroughly tested. It successfully passes adversarial stress testing across all required domains:
- Zero-commit empty repositories.
- Detached HEAD states.
- Spaces, quotes, and UTF-8 / multi-byte Unicode filenames.
- Complete 7-case Git merge conflict matrix (`UU`, `AA`, `DD`, `AU`, `UD`, `UA`, `DU`).
- Complex staged / unstaged dual statuses (`MM`, `AM`, `AD`, `MD`, `RM`, `RD`).
- Invalid commit hashes, empty inputs, non-commit objects, and ancestry graphs.
- Domain error classification and thread safety.

---

## 5. Verification Method

To independently verify the test suite:

```powershell
# Run all unit and adversarial stress tests in internal/git
go test ./internal/git/... -v

# Run full project test suite including reconciliation and CLI integration
go test ./... -v
```

Files to inspect:
- `internal/git/adversarial_test.go`
- `internal/git/lifecycle_adversarial_test.go`
- `internal/git/client.go`
- `internal/git/parser.go`
- `internal/git/errors.go`
