package git

import (
	"reflect"
	"testing"
)

// TestStress_PorcelainAdversarialOctalAndUnicode tests complex C-style octals, multi-byte UTF-8, and escape sequences.
func TestStress_PorcelainAdversarialOctalAndUnicode(t *testing.T) {
	testCases := []struct {
		name              string
		input             string
		expectedStaged    []FileStatus
		expectedUnstaged  []FileStatus
		expectedUntracked []string
		expectedUnmerged  []string
		expectedModified  []string
	}{
		{
			name:              "Japanese UTF-8 3-byte octal unescape",
			input:             "?? \"\\346\\227\\245\\346\\234\\254\\350\\252\\236/\\343\\203\\210\\343\\203\\251\\343\\203\\203\\343\\202\\257.go\"\n",
			expectedUntracked: []string{"日本語/トラック.go"},
			expectedModified:  []string{"日本語/トラック.go"},
		},
		{
			name:  "German and Russian mixed UTF-8 octals",
			input: "M  \"\\303\\274\\303\\261\\303\\256\\303\\247\\303\\270d\\303\\251_\\321\\204\\320\\260\\320\\271\\320\\273.md\"\n",
			expectedStaged: []FileStatus{
				{
					Path:           "üñîçødé_файл.md",
					OrigPath:       "",
					StagingStatus:  StatusModified,
					WorkTreeStatus: StatusUnmodified,
				},
			},
			expectedModified: []string{"üñîçødé_файл.md"},
		},
		{
			name:              "4-byte UTF-8 octal unescape",
			input:             "?? \"\\360\\237\\222\\251_dump.txt\"\n",
			expectedUntracked: []string{"💩_dump.txt"},
			expectedModified:  []string{"💩_dump.txt"},
		},
		{
			name:              "C-style special character escapes: newline, tab, quote",
			input:             "?? \"escaped_\\t_tab_\\\"quote\\\"_path.txt\"\n",
			expectedUntracked: []string{"escaped_\t_tab_\"quote\"_path.txt"},
			expectedModified:  []string{"escaped_\t_tab_\"quote\"_path.txt"},
		},
		{
			name:              "Filename with leading, internal, and trailing whitespace",
			input:             "?? \"  spaced  filename  .txt  \"\n",
			expectedUntracked: []string{"  spaced  filename  .txt  "},
			expectedModified:  []string{"  spaced  filename  .txt  "},
		},
		{
			name:  "Non-rename file containing arrow ' -> ' in filename for M status",
			input: "M  \"file -> with -> arrows -> in -> name.txt\"\n",
			expectedStaged: []FileStatus{
				{
					Path:           "file -> with -> arrows -> in -> name.txt",
					OrigPath:       "",
					StagingStatus:  StatusModified,
					WorkTreeStatus: StatusUnmodified,
				},
			},
			expectedModified: []string{"file -> with -> arrows -> in -> name.txt"},
		},
		{
			name:              "Non-rename file containing arrow ' -> ' in filename for untracked status",
			input:             "?? \"notes -> draft -> v2.txt\"\n",
			expectedUntracked: []string{"notes -> draft -> v2.txt"},
			expectedModified:  []string{"notes -> draft -> v2.txt"},
		},
		{
			name:  "Standard rename with quotes and spaces",
			input: "R  \"old path with space.txt\" -> \"new path with space.txt\"\n",
			expectedStaged: []FileStatus{
				{
					Path:           "new path with space.txt",
					OrigPath:       "old path with space.txt",
					StagingStatus:  StatusRenamed,
					WorkTreeStatus: StatusUnmodified,
				},
			},
			expectedModified: []string{"new path with space.txt", "old path with space.txt"},
		},
		{
			name:  "Rename with octals in source and destination",
			input: "R  \"\\346\\227\\245\\346\\234\\254\\350\\252\\236_old.txt\" -> \"\\346\\227\\245\\346\\234\\254\\350\\252\\236_new.txt\"\n",
			expectedStaged: []FileStatus{
				{
					Path:           "日本語_new.txt",
					OrigPath:       "日本語_old.txt",
					StagingStatus:  StatusRenamed,
					WorkTreeStatus: StatusUnmodified,
				},
			},
			expectedModified: []string{"日本語_new.txt", "日本語_old.txt"},
		},
		{
			name: "Mixed complex status with 10 files across staged, unstaged, untracked, unmerged",
			input: "M  \"src/app.go\"\n" +
				"A  \"docs/\\346\\227\\245\\346\\234\\254.md\"\n" +
				"D  \"old/deprecated.go\"\n" +
				"R  \"old_name.txt\" -> \"new_name.txt\"\n" +
				" M \"unstaged -> mod.go\"\n" +
				" D \"deleted.txt\"\n" +
				"MM \"dual_mod.go\"\n" +
				"?? \"untracked_\\303\\274.txt\"\n" +
				"UU \"conflict_\\346\\227\\245.go\"\n" +
				"!! \"ignored/cache.bin\"\n",
			expectedStaged: []FileStatus{
				{Path: "src/app.go", StagingStatus: StatusModified, WorkTreeStatus: StatusUnmodified},
				{Path: "docs/日本.md", StagingStatus: StatusAdded, WorkTreeStatus: StatusUnmodified},
				{Path: "old/deprecated.go", StagingStatus: StatusDeleted, WorkTreeStatus: StatusUnmodified},
				{Path: "new_name.txt", OrigPath: "old_name.txt", StagingStatus: StatusRenamed, WorkTreeStatus: StatusUnmodified},
				{Path: "dual_mod.go", StagingStatus: StatusModified, WorkTreeStatus: StatusModified},
			},
			expectedUnstaged: []FileStatus{
				{Path: "unstaged -> mod.go", StagingStatus: StatusUnmodified, WorkTreeStatus: StatusModified},
				{Path: "deleted.txt", StagingStatus: StatusUnmodified, WorkTreeStatus: StatusDeleted},
				{Path: "dual_mod.go", StagingStatus: StatusModified, WorkTreeStatus: StatusModified},
			},
			expectedUntracked: []string{"untracked_ü.txt"},
			expectedUnmerged:  []string{"conflict_日.go"},
			expectedModified: []string{
				"conflict_日.go",
				"deleted.txt",
				"docs/日本.md",
				"dual_mod.go",
				"new_name.txt",
				"old/deprecated.go",
				"old_name.txt",
				"src/app.go",
				"unstaged -> mod.go",
				"untracked_ü.txt",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res := ParsePorcelainStatus(tc.input)

			if len(tc.expectedStaged) > 0 || len(tc.expectedUnstaged) > 0 || len(tc.expectedUntracked) > 0 || len(tc.expectedUnmerged) > 0 {
				if res.IsClean {
					t.Errorf("expected IsClean=false, got true")
				}
			}

			if len(res.StagedFiles) != len(tc.expectedStaged) {
				t.Fatalf("staged count mismatch: expected %d, got %d (%+v)", len(tc.expectedStaged), len(res.StagedFiles), res.StagedFiles)
			}
			for i, exp := range tc.expectedStaged {
				act := res.StagedFiles[i]
				if act.Path != exp.Path || act.OrigPath != exp.OrigPath || act.StagingStatus != exp.StagingStatus || act.WorkTreeStatus != exp.WorkTreeStatus {
					t.Errorf("staged[%d]: expected %+v, got %+v", i, exp, act)
				}
			}

			if len(res.UnstagedFiles) != len(tc.expectedUnstaged) {
				t.Fatalf("unstaged count mismatch: expected %d, got %d (%+v)", len(tc.expectedUnstaged), len(res.UnstagedFiles), res.UnstagedFiles)
			}
			for i, exp := range tc.expectedUnstaged {
				act := res.UnstagedFiles[i]
				if act.Path != exp.Path || act.OrigPath != exp.OrigPath || act.StagingStatus != exp.StagingStatus || act.WorkTreeStatus != exp.WorkTreeStatus {
					t.Errorf("unstaged[%d]: expected %+v, got %+v", i, exp, act)
				}
			}

			if tc.expectedUntracked != nil {
				if !reflect.DeepEqual(res.UntrackedFiles, tc.expectedUntracked) {
					t.Errorf("untracked mismatch: expected %v, got %v", tc.expectedUntracked, res.UntrackedFiles)
				}
			}

			if tc.expectedUnmerged != nil {
				if !reflect.DeepEqual(res.UnmergedFiles, tc.expectedUnmerged) {
					t.Errorf("unmerged mismatch: expected %v, got %v", tc.expectedUnmerged, res.UnmergedFiles)
				}
			}

			if tc.expectedModified != nil {
				mod := ExtractModifiedFiles(res)
				if !reflect.DeepEqual(mod, tc.expectedModified) {
					t.Errorf("modified files mismatch:\nexpected: %v\ngot:      %v", tc.expectedModified, mod)
				}
			}
		})
	}
}

// TestStress_ParsePorcelainFuzzBoundary tests edge cases and boundary conditions.
func TestStress_ParsePorcelainFuzzBoundary(t *testing.T) {
	edgeCases := []string{
		"",
		"\n",
		"\r\n\r\n",
		"   \n\t\n",
		"X",
		"XY",
		"M",
		" M",
		"M ",
		"??",
		"!!",
		"?? ",
		"M  ",
		"R  ",
		"R  a",
		"R  a -> ",
		"R   -> b",
		"R  \"\" -> \"\"",
		"?? \"\"",
		"?? \"   \"",
		"?? \"\\377\\377\\377\"",
		"?? \"\\000\\001\\002\"",
		"?? \"unclosed_quote",
		"?? unquoted_with_special_chars!@#$%^&*()_+.txt",
	}

	for _, input := range edgeCases {
		res := ParsePorcelainStatus(input)
		if res == nil {
			t.Fatalf("ParsePorcelainStatus returned nil for input: %q", input)
		}
		// ExtractModifiedFiles must never panic or return nil
		mod := ExtractModifiedFiles(res)
		if mod == nil {
			t.Fatalf("ExtractModifiedFiles returned nil for input: %q", input)
		}
	}
}
