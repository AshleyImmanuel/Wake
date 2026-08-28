# Handoff Report: Investigation of UTF-8 Byte Sort Order in internal/git

**Agent**: Explorer 1 (Milestone 1)  
**Date**: 2026-08-29  
**Status**: Investigation Complete  

---

## 1. Observation

### 1.1 Test Failure Reproduction
Executing `go test -v ./internal/git/...` reproduces a single test failure in `internal/git/adversarial_test.go:206`:

```
=== RUN   TestAdversarial_FilenamesWithSpacesAndUnicode
    adversarial_test.go:206: ExtractModifiedFiles mismatch:
        expected: [deeply/nested/path with spaces/another file.go new name with space.txt normal_deleted.txt old name with space.txt path with spaces/my file.txt unicode_日本語_test.txt unicode_üñîçødé_файл.md]
        got:      [deeply/nested/path with spaces/another file.go new name with space.txt normal_deleted.txt old name with space.txt path with spaces/my file.txt unicode_üñîçødé_файл.md unicode_日本語_test.txt]
--- FAIL: TestAdversarial_FilenamesWithSpacesAndUnicode (0.00s)
```

### 1.2 Code Under Test: `internal/git/adversarial_test.go` (lines 185-208)
```go
185: 	// Verify untracked files
186: 	expectedUntracked := []string{
187: 		"unicode_日本語_test.txt",
188: 		"unicode_üñîçødé_файл.md",
189: 	}
190: 	if !reflect.DeepEqual(status.UntrackedFiles, expectedUntracked) {
191: 		t.Errorf("untracked files mismatch: expected %v, got %v", expectedUntracked, status.UntrackedFiles)
192: 	}
193: 
194: 	// Verify ExtractModifiedFiles
195: 	modified := ExtractModifiedFiles(status)
196: 	expectedModified := []string{
197: 		"deeply/nested/path with spaces/another file.go",
198: 		"new name with space.txt",
199: 		"normal_deleted.txt",
200: 		"old name with space.txt",
201: 		"path with spaces/my file.txt",
202: 		"unicode_日本語_test.txt",
203: 		"unicode_üñîçødé_файл.md",
204: 	}
205: 	if !reflect.DeepEqual(modified, expectedModified) {
206: 		t.Errorf("ExtractModifiedFiles mismatch:\nexpected: %v\ngot:      %v", expectedModified, modified)
207: 	}
```

### 1.3 Implementation: `internal/git/parser.go` (lines 125-163)
```go
125: func ExtractModifiedFiles(status *StatusResult) []string {
126: 	if status == nil {
127: 		return []string{}
128: 	}
129: 
130: 	seen := make(map[string]struct{})
131: 
132: 	addPath := func(p string) {
133: 		if p != "" {
134: 			seen[p] = struct{}{}
135: 		}
136: 	}
137: 
138: 	for _, f := range status.StagedFiles {
139: 		addPath(f.Path)
140: 		addPath(f.OrigPath)
141: 	}
142: 
143: 	for _, f := range status.UnstagedFiles {
144: 		addPath(f.Path)
145: 		addPath(f.OrigPath)
146: 	}
147: 
148: 	for _, p := range status.UntrackedFiles {
149: 		addPath(p)
150: 	}
151: 
152: 	for _, p := range status.UnmergedFiles {
153: 		addPath(p)
154: 	}
155: 
156: 	result := make([]string, 0, len(seen))
157: 	for p := range seen {
158: 		result = append(result, p)
159: 	}
160: 
161: 	sort.Strings(result)
162: 	return result
163: }
```

### 1.4 Byte-Level UTF-8 Encoding Comparison
| String | Shared Prefix | First Differing Rune | Unicode Code Point | UTF-8 Bytes (Hex) | UTF-8 Bytes (Dec) |
|---|---|---|---|---|---|
| `unicode_üñîçødé_файл.md` | `unicode_` | `ü` | U+00FC | `0xC3 0xBC` | `195 188` |
| `unicode_日本語_test.txt` | `unicode_` | `日` | U+65E5 | `0xE6 0x97 0xA5` | `230 151 165` |

---

## 2. Logic Chain

1. **`ExtractModifiedFiles` Contract**: `ExtractModifiedFiles` collects all file paths from staged, unstaged, untracked, and unmerged slices into a hash set (`seen`), extracts unique keys into a slice `result`, and sorts them via Go's standard `sort.Strings(result)` (`internal/git/parser.go:161`).
2. **Go String Comparison Semantics**: In Go, string comparison (`<`) and `sort.Strings` operate byte-by-byte in ascending lexicographical order over raw UTF-8 byte sequences.
3. **Byte Value Evaluation**:
   - Both strings share the identical 8-byte prefix `"unicode_"` (`[0x75, 0x6e, 0x69, 0x63, 0x6f, 0x64, 0x65, 0x5f]`).
   - At byte index 8, `"unicode_üñîçødé_файл.md"` has byte `0xC3` (195).
   - At byte index 8, `"unicode_日本語_test.txt"` has byte `0xE6` (230).
   - Since `0xC3 < 0xE6` (195 < 230), `"unicode_üñîçødé_файл.md" < "unicode_日本語_test.txt"` evaluates to `true`.
4. **Source of the Discrepancy**:
   - `ParsePorcelainStatus` preserves line-by-line stream encounter order when populating `status.UntrackedFiles`.
   - In `porcelainOutput`, `"unicode_日本語_test.txt"` was listed before `"unicode_üñîçødé_файл.md"`, so `status.UntrackedFiles` received them in that order (which passed line 190).
   - When writing `expectedModified`, the test author inadvertently assumed the untracked file ordering remained unchanged or assumed incorrect Unicode sorting, placing `"unicode_日本語_test.txt"` before `"unicode_üñîçødé_файл.md"`.
5. **Conclusion from Logic Chain**: The parser logic in `internal/git/parser.go` is completely correct and conforms to Go's standard lexicographical sort specification. The expectation array `expectedModified` in `internal/git/adversarial_test.go` contains an inverted ordering for those two entries.

---

## 3. Caveats

- **Untracked Order vs Modified Order**: `status.UntrackedFiles` in `StatusResult` is not sorted by `ParsePorcelainStatus`; it maintains the raw porcelain line order. This is intentional and standard for status stream parsing.
- **Production Code Integrity**: No changes should be made to `internal/git/parser.go` or any other production code in `internal/git/`. Go's `sort.Strings` is the correct, deterministic sorting method for consolidated file lists.
- No other test failures exist in `internal/git` or across the entire repository.

---

## 4. Conclusion & Recommendations for Worker

### 4.1 Recommended Action
In `internal/git/adversarial_test.go`, update `expectedModified` at lines 202-203 by swapping the order of the two Unicode strings.

### 4.2 Code Diff

```diff
--- a/internal/git/adversarial_test.go
+++ b/internal/git/adversarial_test.go
@@ -199,8 +199,8 @@ func TestAdversarial_FilenamesWithSpacesAndUnicode(t *testing.T) {
 		"normal_deleted.txt",
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

### 5.1 Verification Commands
1. Run target test:
   ```powershell
   & "C:\Program Files\Go\bin\go.exe" test -v -run TestAdversarial_FilenamesWithSpacesAndUnicode ./internal/git
   ```
2. Run entire git test suite:
   ```powershell
   & "C:\Program Files\Go\bin\go.exe" test -v ./internal/git/...
   ```
3. Run entire repository test suite:
   ```powershell
   & "C:\Program Files\Go\bin\go.exe" test -v ./...
   ```
4. Verify structural soundness:
   ```powershell
   & "C:\Program Files\Go\bin\go.exe" vet ./...
   ```

### 5.2 Invalidation Conditions
- Any failure in `TestAdversarial_FilenamesWithSpacesAndUnicode`.
- Any mismatch in `ExtractModifiedFiles` output ordering.
