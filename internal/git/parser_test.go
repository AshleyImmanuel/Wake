package git

import (
	"reflect"
	"testing"
)

func TestParsePorcelainStatus(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected *StatusResult
	}{
		{
			name:   "empty input",
			output: "",
			expected: &StatusResult{
				StagedFiles:    []FileStatus{},
				UnstagedFiles:  []FileStatus{},
				UntrackedFiles: []string{},
				UnmergedFiles:  []string{},
				IsClean:        true,
			},
		},
		{
			name:   "ignored files",
			output: "!! ignored.txt\n",
			expected: &StatusResult{
				StagedFiles:    []FileStatus{},
				UnstagedFiles:  []FileStatus{},
				UntrackedFiles: []string{},
				UnmergedFiles:  []string{},
				IsClean:        true,
			},
		},
		{
			name:   "untracked files",
			output: "?? newfile.txt\n?? \"quoted file.txt\"\n",
			expected: &StatusResult{
				StagedFiles:   []FileStatus{},
				UnstagedFiles: []FileStatus{},
				UntrackedFiles: []string{
					"newfile.txt",
					"quoted file.txt",
				},
				UnmergedFiles: []string{},
				IsClean:       false,
			},
		},
		{
			name: "staged and unstaged changes",
			output: "M  staged.txt\n" +
				" A added_staged.txt\n" +
				"D  deleted_staged.txt\n" +
				" M unstaged.txt\n" +
				" D deleted_unstaged.txt\n",
			expected: &StatusResult{
				StagedFiles: []FileStatus{
					{Path: "staged.txt", StagingStatus: "M", WorkTreeStatus: " "},
					{Path: "deleted_staged.txt", StagingStatus: "D", WorkTreeStatus: " "},
				},
				UnstagedFiles: []FileStatus{
					{Path: "added_staged.txt", StagingStatus: " ", WorkTreeStatus: "A"}, // " A" means untracked space, A in worktree. Actually wait.
					// X is index, Y is worktree.
					// " A added_staged.txt": X=' ', Y='A'.
					{Path: "unstaged.txt", StagingStatus: " ", WorkTreeStatus: "M"},
					{Path: "deleted_unstaged.txt", StagingStatus: " ", WorkTreeStatus: "D"},
				},
				UntrackedFiles: []string{},
				UnmergedFiles:  []string{},
				IsClean:        false,
			},
		},
		{
			name: "renamed files",
			output: "R  old.txt -> new.txt\n" +
				" C old_copy.txt -> new_copy.txt\n",
			expected: &StatusResult{
				StagedFiles: []FileStatus{
					{Path: "new.txt", OrigPath: "old.txt", StagingStatus: "R", WorkTreeStatus: " "},
				},
				UnstagedFiles: []FileStatus{
					{Path: "new_copy.txt", OrigPath: "old_copy.txt", StagingStatus: " ", WorkTreeStatus: "C"},
				},
				UntrackedFiles: []string{},
				UnmergedFiles:  []string{},
				IsClean:        false,
			},
		},
		{
			name: "unmerged states",
			output: "UU file1.txt\n" +
				"DD file2.txt\n" +
				"AU file3.txt\n" +
				"UD file4.txt\n" +
				"UA file5.txt\n" +
				"DU file6.txt\n" +
				"AA file7.txt\n",
			expected: &StatusResult{
				StagedFiles:    []FileStatus{},
				UnstagedFiles:  []FileStatus{},
				UntrackedFiles: []string{},
				UnmergedFiles: []string{
					"file1.txt", "file2.txt", "file3.txt", "file4.txt",
					"file5.txt", "file6.txt", "file7.txt",
				},
				IsClean: false,
			},
		},
		{
			name:   "quoted paths with spaces and special chars",
			output: "M  \"path with spaces.txt\"\n",
			expected: &StatusResult{
				StagedFiles: []FileStatus{
					{Path: "path with spaces.txt", StagingStatus: "M", WorkTreeStatus: " "},
				},
				UnstagedFiles:  []FileStatus{},
				UntrackedFiles: []string{},
				UnmergedFiles:  []string{},
				IsClean:        false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParsePorcelainStatus(tt.output)
			if !reflect.DeepEqual(result.StagedFiles, tt.expected.StagedFiles) {
				t.Errorf("StagedFiles mismatch: got %v, want %v", result.StagedFiles, tt.expected.StagedFiles)
			}
			if !reflect.DeepEqual(result.UnstagedFiles, tt.expected.UnstagedFiles) {
				t.Errorf("UnstagedFiles mismatch: got %v, want %v", result.UnstagedFiles, tt.expected.UnstagedFiles)
			}
			if !reflect.DeepEqual(result.UntrackedFiles, tt.expected.UntrackedFiles) {
				t.Errorf("UntrackedFiles mismatch: got %v, want %v", result.UntrackedFiles, tt.expected.UntrackedFiles)
			}
			if !reflect.DeepEqual(result.UnmergedFiles, tt.expected.UnmergedFiles) {
				t.Errorf("UnmergedFiles mismatch: got %v, want %v", result.UnmergedFiles, tt.expected.UnmergedFiles)
			}
			if result.IsClean != tt.expected.IsClean {
				t.Errorf("IsClean mismatch: got %v, want %v", result.IsClean, tt.expected.IsClean)
			}
		})
	}
}

func TestParseNameOnlyList(t *testing.T) {
	output := "file1.txt\n\n  \nfile2.txt\n\"file 3.txt\"\nfile1.txt\n"
	expected := []string{"  ", "file 3.txt", "file1.txt", "file2.txt"}
	
	result := ParseNameOnlyList(output)
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("ParseNameOnlyList mismatch: got %v, want %v", result, expected)
	}
}

func TestExtractModifiedFiles(t *testing.T) {
	status := &StatusResult{
		StagedFiles: []FileStatus{
			{Path: "staged.txt"},
			{Path: "renamed.txt", OrigPath: "old_renamed.txt"},
		},
		UnstagedFiles: []FileStatus{
			{Path: "unstaged.txt"},
		},
		UntrackedFiles: []string{"untracked.txt"},
		UnmergedFiles:  []string{"unmerged.txt"},
	}
	expected := []string{"old_renamed.txt", "renamed.txt", "staged.txt", "unmerged.txt", "unstaged.txt", "untracked.txt"}
	
	result := ExtractModifiedFiles(status)
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("ExtractModifiedFiles mismatch: got %v, want %v", result, expected)
	}

	if len(ExtractModifiedFiles(nil)) != 0 {
		t.Errorf("ExtractModifiedFiles(nil) should return empty slice")
	}
}

func TestUnescapeGitPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`"normal/path.txt"`, `normal/path.txt`},
		{`normal/path.txt`, `normal/path.txt`},
		{`"\a\b\f\n\r\t\v\\\""`, "\a\b\f\n\r\t\v\\\""},
		{`"\303\251"`, "é"},
		{`"\344\270\255"`, "中"},
		{`"already_unquoted"`, `already_unquoted`}, // it drops quotes
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := unescapeGitPath(tt.input)
			if result != tt.expected {
				t.Errorf("unescapeGitPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseRenamePath(t *testing.T) {
	tests := []struct {
		input    string
		origPath string
		newPath  string
	}{
		{"old.txt -> new.txt", "old.txt", "new.txt"},
		{"\"old file.txt\" -> \"new file.txt\"", "old file.txt", "new file.txt"},
		{"\"old \\\"quote\\\".txt\" -> new.txt", "old \"quote\".txt", "new.txt"},
		{"invalid format", "", "invalid format"},
		{"\"quoted no arrow\"", "", "quoted no arrow"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			orig, dest := parseRenamePath(tt.input)
			if orig != tt.origPath || dest != tt.newPath {
				t.Errorf("parseRenamePath(%q) = (%q, %q), want (%q, %q)", tt.input, orig, dest, tt.origPath, tt.newPath)
			}
		})
	}
}

func TestIsValidGitRef(t *testing.T) {
	tests := []struct {
		ref      string
		expected bool
	}{
		{"main", true},
		{"feature/branch", true},
		{"v1.0.0", true},
		{"abc123_", true},
		{"-invalid", false},
		{"", false},
		{"invalid space", false},
		{"invalid&char", false},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			result := isValidGitRef(tt.ref)
			if result != tt.expected {
				t.Errorf("isValidGitRef(%q) = %v, want %v", tt.ref, result, tt.expected)
			}
		})
	}
}
