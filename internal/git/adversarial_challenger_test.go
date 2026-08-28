package git

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestChallenger_GitRefValidationMatrix tests all malicious and adversarial git ref formats.
func TestChallenger_GitRefValidationMatrix(t *testing.T) {
	maliciousRefs := []struct {
		name string
		ref  string
	}{
		{"flag_output", "--output=pwned.txt"},
		{"flag_exec", "--exec=calc.exe"},
		{"flag_u0", "-U0"},
		{"flag_upload_pack", "--upload-pack=whoami"},
		{"flag_config", "--config=core.fsmonitor=true"},
		{"flag_short_o", "-o"},
		{"flag_short_v", "-v"},
		{"flag_help", "--help"},
		{"flag_dash_only", "-"},
		{"flag_double_dash", "--"},
		{"cmd_separator_semicolon", "commit1; whoami"},
		{"cmd_separator_and", "commit1 && rm -rf ."},
		{"cmd_separator_pipe", "commit1 | cat /etc/passwd"},
		{"cmd_separator_or", "commit1 || echo failed"},
		{"cmd_separator_newline", "commit1\nwhoami"},
		{"cmd_separator_crlf", "commit1\r\ncalc"},
		{"shell_subshell_dollar", "commit1$(whoami)"},
		{"shell_subshell_backtick", "commit1`whoami`"},
		{"shell_redirect_out", "commit1>pwn.txt"},
		{"shell_redirect_in", "commit1<input.txt"},
		{"shell_wildcard_star", "commit1*"},
		{"shell_wildcard_quest", "commit1?"},
		{"shell_background", "commit1 & calc"},
		{"shell_tilde", "commit1~1"},
		{"shell_caret", "commit1^2"},
		{"shell_at_brace", "HEAD@{1}"},
		{"null_byte", "commit1\x00payload"},
		{"space_padded", " commit1 "},
		{"tab_in_ref", "commit1\tref"},
		{"empty_ref", ""},
	}

	for _, tc := range maliciousRefs {
		t.Run("isValidGitRef_"+tc.name, func(t *testing.T) {
			if isValidGitRef(tc.ref) {
				t.Errorf("isValidGitRef(%q) should be false for %s", tc.ref, tc.name)
			}
		})
	}
}

// TestChallenger_ValidGitRefs tests that standard, legitimate Git refs are accepted.
func TestChallenger_ValidGitRefs(t *testing.T) {
	validRefs := []string{
		"a1b2c3d4e5f67890123456789abcdef012345678",
		"main",
		"master",
		"feature/reconcile",
		"feat/issue-123_test.v1",
		"v1.0.0",
		"release/2.0.0-rc1",
		"HEAD",
		"origin/main",
		"stash",
	}

	for _, ref := range validRefs {
		t.Run("valid_"+ref, func(t *testing.T) {
			if !isValidGitRef(ref) {
				t.Errorf("isValidGitRef(%q) should be true", ref)
			}
		})
	}
}

// TestChallenger_ClientInjectionAttacks tests that Client methods properly abort on malicious refs.
func TestChallenger_ClientInjectionAttacks(t *testing.T) {
	mock := NewMockRunner()
	ctx := context.Background()
	repoDir := "C:/test/repo"
	c := NewClient(mock)

	attacks := []string{
		"--output=pwned.txt",
		"-U0",
		"--exec=calc.exe",
		"commit1; id",
		"commit1 && calc",
		"commit1 | echo 1",
		"commit1`id`",
		"commit1$(whoami)",
		"commit1\x00injection",
	}

	for _, atk := range attacks {
		// 1. GetDiffBetween
		_, err := c.GetDiffBetween(ctx, repoDir, atk, "validcommit")
		if !errors.Is(err, ErrInvalidCommit) {
			t.Errorf("GetDiffBetween(from=%q) expected ErrInvalidCommit, got: %v", atk, err)
		}
		_, err = c.GetDiffBetween(ctx, repoDir, "validcommit", atk)
		if !errors.Is(err, ErrInvalidCommit) {
			t.Errorf("GetDiffBetween(to=%q) expected ErrInvalidCommit, got: %v", atk, err)
		}

		// 2. GetChangedFilesBetween
		_, err = c.GetChangedFilesBetween(ctx, repoDir, atk, "validcommit")
		if !errors.Is(err, ErrInvalidCommit) {
			t.Errorf("GetChangedFilesBetween(from=%q) expected ErrInvalidCommit, got: %v", atk, err)
		}
		_, err = c.GetChangedFilesBetween(ctx, repoDir, "validcommit", atk)
		if !errors.Is(err, ErrInvalidCommit) {
			t.Errorf("GetChangedFilesBetween(to=%q) expected ErrInvalidCommit, got: %v", atk, err)
		}

		// 3. CommitExists
		exists, err := c.CommitExists(ctx, repoDir, atk)
		if err != nil || exists {
			t.Errorf("CommitExists(%q) expected false, nil, got: %v, %v", atk, exists, err)
		}

		// 4. IsAncestor
		isAnc, err := c.IsAncestor(ctx, repoDir, atk, "validcommit")
		if !errors.Is(err, ErrInvalidCommit) || isAnc {
			t.Errorf("IsAncestor(ancestor=%q) expected false, ErrInvalidCommit, got: %v, %v", atk, isAnc, err)
		}
		isAnc, err = c.IsAncestor(ctx, repoDir, "validcommit", atk)
		if !errors.Is(err, ErrInvalidCommit) || isAnc {
			t.Errorf("IsAncestor(descendant=%q) expected false, ErrInvalidCommit, got: %v, %v", atk, isAnc, err)
		}
	}
}

// TestChallenger_ParserStress tests porcelain parser against complex path escaping scenarios.
func TestChallenger_ParserStress(t *testing.T) {
	output := strings.Join([]string{
		`M  "dir with spaces/sub dir/file.go"`,
		`A  "special_\"quotes\"_file.txt"`,
		`R  "old \"quoted\" name.go" -> "new \"quoted\" name.go"`,
		`?? "C:\\Windows\\System32\\file.txt"`,
		`?? "\\\\host\\share\\file.txt"`,
		`?? "../../../traversal.txt"`,
	}, "\n")

	res := ParsePorcelainStatus(output)

	if len(res.StagedFiles) != 3 {
		t.Fatalf("expected 3 staged files, got %d", len(res.StagedFiles))
	}
	if res.StagedFiles[0].Path != "dir with spaces/sub dir/file.go" {
		t.Errorf("unexpected path: %q", res.StagedFiles[0].Path)
	}
	if res.StagedFiles[1].Path != "special_\"quotes\"_file.txt" {
		t.Errorf("unexpected path: %q", res.StagedFiles[1].Path)
	}
	if res.StagedFiles[2].Path != "new \"quoted\" name.go" || res.StagedFiles[2].OrigPath != "old \"quoted\" name.go" {
		t.Errorf("unexpected rename: %+v", res.StagedFiles[2])
	}
}
