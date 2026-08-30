package git

import (
	"reflect"
	"testing"
)

func TestParsePorcelainStatus_Clean(t *testing.T) {
	output := ""
	res := ParsePorcelainStatus(output)

	if !res.IsClean {
		t.Errorf("expected IsClean=true, got false")
	}
	if len(res.StagedFiles) != 0 {
		t.Errorf("expected 0 staged files, got %d", len(res.StagedFiles))
	}
	if len(res.UnstagedFiles) != 0 {
		t.Errorf("expected 0 unstaged files, got %d", len(res.UnstagedFiles))
	}
	if len(res.UntrackedFiles) != 0 {
		t.Errorf("expected 0 untracked files, got %d", len(res.UntrackedFiles))
	}
	if len(res.UnmergedFiles) != 0 {
		t.Errorf("expected 0 unmerged files, got %d", len(res.UnmergedFiles))
	}

	mod := ExtractModifiedFiles(res)
	if len(mod) != 0 {
		t.Errorf("expected 0 modified files, got %v", mod)
	}
}

func TestParsePorcelainStatus_Mixed(t *testing.T) {
	output := `M  staged_mod.go
A  staged_new.go
D  staged_del.go
R  old_name.go -> new_name.go
 M unstaged_mod.go
 D unstaged_del.go
MM both_mod.go
?? untracked.txt
?? "quoted space/nested path.go"
UU conflict.go
AA conflict_add.go
`
	res := ParsePorcelainStatus(output)

	if res.IsClean {
		t.Errorf("expected IsClean=false, got true")
	}

	// Verify Staged files: staged_mod.go, staged_new.go, staged_del.go, new_name.go, both_mod.go
	expectedStaged := []FileStatus{
		{Path: "staged_mod.go", StagingStatus: StatusModified, WorkTreeStatus: StatusUnmodified},
		{Path: "staged_new.go", StagingStatus: StatusAdded, WorkTreeStatus: StatusUnmodified},
		{Path: "staged_del.go", StagingStatus: StatusDeleted, WorkTreeStatus: StatusUnmodified},
		{Path: "new_name.go", OrigPath: "old_name.go", StagingStatus: StatusRenamed, WorkTreeStatus: StatusUnmodified},
		{Path: "both_mod.go", StagingStatus: StatusModified, WorkTreeStatus: StatusModified},
	}

	if len(res.StagedFiles) != len(expectedStaged) {
		t.Fatalf("expected %d staged files, got %d: %+v", len(expectedStaged), len(res.StagedFiles), res.StagedFiles)
	}
	for i, exp := range expectedStaged {
		actual := res.StagedFiles[i]
		if actual.Path != exp.Path || actual.OrigPath != exp.OrigPath || actual.StagingStatus != exp.StagingStatus || actual.WorkTreeStatus != exp.WorkTreeStatus {
			t.Errorf("staged[%d]: expected %+v, got %+v", i, exp, actual)
		}
	}

	// Verify Unstaged files: unstaged_mod.go, unstaged_del.go, both_mod.go
	expectedUnstaged := []FileStatus{
		{Path: "unstaged_mod.go", StagingStatus: StatusUnmodified, WorkTreeStatus: StatusModified},
		{Path: "unstaged_del.go", StagingStatus: StatusUnmodified, WorkTreeStatus: StatusDeleted},
		{Path: "both_mod.go", StagingStatus: StatusModified, WorkTreeStatus: StatusModified},
	}

	if len(res.UnstagedFiles) != len(expectedUnstaged) {
		t.Fatalf("expected %d unstaged files, got %d: %+v", len(expectedUnstaged), len(res.UnstagedFiles), res.UnstagedFiles)
	}
	for i, exp := range expectedUnstaged {
		actual := res.UnstagedFiles[i]
		if actual.Path != exp.Path || actual.OrigPath != exp.OrigPath || actual.StagingStatus != exp.StagingStatus || actual.WorkTreeStatus != exp.WorkTreeStatus {
			t.Errorf("unstaged[%d]: expected %+v, got %+v", i, exp, actual)
		}
	}

	// Verify Untracked files: untracked.txt, quoted space/nested path.go
	expectedUntracked := []string{
		"untracked.txt",
		"quoted space/nested path.go",
	}
	if !reflect.DeepEqual(res.UntrackedFiles, expectedUntracked) {
		t.Errorf("untracked mismatch: expected %v, got %v", expectedUntracked, res.UntrackedFiles)
	}

	// Verify Unmerged files: conflict.go, conflict_add.go
	expectedUnmerged := []string{
		"conflict.go",
		"conflict_add.go",
	}
	if !reflect.DeepEqual(res.UnmergedFiles, expectedUnmerged) {
		t.Errorf("unmerged mismatch: expected %v, got %v", expectedUnmerged, res.UnmergedFiles)
	}

	// Verify ExtractModifiedFiles includes all paths deduplicated and sorted
	modified := ExtractModifiedFiles(res)
	expectedModified := []string{
		"both_mod.go",
		"conflict.go",
		"conflict_add.go",
		"new_name.go",
		"old_name.go",
		"quoted space/nested path.go",
		"staged_del.go",
		"staged_mod.go",
		"staged_new.go",
		"unstaged_del.go",
		"unstaged_mod.go",
		"untracked.txt",
	}
	if !reflect.DeepEqual(modified, expectedModified) {
		t.Errorf("modified files mismatch:\nexpected: %v\ngot:      %v", expectedModified, modified)
	}
}

func TestParsePorcelainStatus_UnmergedVariations(t *testing.T) {
	output := `DD both_deleted.txt
AU added_by_us.txt
UD deleted_by_them.txt
UA added_by_them.txt
DU deleted_by_us.txt
AA both_added.txt
UU both_modified.txt
`
	res := ParsePorcelainStatus(output)

	if res.IsClean {
		t.Errorf("expected IsClean=false")
	}
	if len(res.UnmergedFiles) != 7 {
		t.Fatalf("expected 7 unmerged files, got %d: %v", len(res.UnmergedFiles), res.UnmergedFiles)
	}
	if len(res.StagedFiles) != 0 || len(res.UnstagedFiles) != 0 || len(res.UntrackedFiles) != 0 {
		t.Errorf("unmerged entries should not pollute staged/unstaged/untracked slices")
	}
}

func TestParseNameOnlyList(t *testing.T) {
	output := `fileA.go
fileB.go
dir/fileC.go
"path with spaces/fileD.go"
fileA.go
`
	files := ParseNameOnlyList(output)
	expected := []string{
		"dir/fileC.go",
		"fileA.go",
		"fileB.go",
		"path with spaces/fileD.go",
	}

	if !reflect.DeepEqual(files, expected) {
		t.Errorf("expected %v, got %v", expected, files)
	}
}

func TestParseDiffNameStatus(t *testing.T) {
	output := `M	main.go
A	internal/git/client.go
D	old_helper.go
R100	old_path.go	new_path.go
`
	changes := ParseDiffNameStatus(output)
	expected := []FileChange{
		{Path: "main.go", Status: StatusModified},
		{Path: "internal/git/client.go", Status: StatusAdded},
		{Path: "old_helper.go", Status: StatusDeleted},
		{Path: "new_path.go", OrigPath: "old_path.go", Status: StatusRenamed},
	}

	if len(changes) != len(expected) {
		t.Fatalf("expected %d changes, got %d", len(expected), len(changes))
	}

	for i, exp := range expected {
		if changes[i] != exp {
			t.Errorf("change[%d]: expected %+v, got %+v", i, exp, changes[i])
		}
	}
}

func TestParsePorcelainStatus_OctalAndEscapes(t *testing.T) {
	output := `?? "unicode_\346\227\245\346\234\254\350\252\236_test.txt"
?? "unicode_\303\274\303\261\303\256\303\247\303\270d\303\251_\321\204\320\260\320\271\320\273.md"
?? " leading and trailing space.txt "
M  "notes -> draft.txt"
`
	res := ParsePorcelainStatus(output)
	if len(res.UntrackedFiles) != 3 {
		t.Fatalf("expected 3 untracked files, got %d: %v", len(res.UntrackedFiles), res.UntrackedFiles)
	}
	if res.UntrackedFiles[0] != "unicode_日本語_test.txt" {
		t.Errorf("octal unicode unescape failed: got %q", res.UntrackedFiles[0])
	}
	if res.UntrackedFiles[1] != "unicode_üñîçødé_файл.md" {
		t.Errorf("octal unicode unescape failed: got %q", res.UntrackedFiles[1])
	}
	if res.UntrackedFiles[2] != " leading and trailing space.txt " {
		t.Errorf("leading/trailing whitespace loss: got %q", res.UntrackedFiles[2])
	}

	// Verify false rename prevention on 'M' status
	if len(res.StagedFiles) != 1 {
		t.Fatalf("expected 1 staged file, got %d: %+v", len(res.StagedFiles), res.StagedFiles)
	}
	if res.StagedFiles[0].Path != "notes -> draft.txt" || res.StagedFiles[0].OrigPath != "" {
		t.Errorf("false rename occurred on non-R status: %+v", res.StagedFiles[0])
	}
}
