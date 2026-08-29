package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootCommand(t *testing.T) {
	cmd := rootCmd
	if cmd.Use != "wake" {
		t.Errorf("expected use 'wake', got '%s'", cmd.Use)
	}

	subCommands := cmd.Commands()
	expectedCmds := []string{"checkpoint", "status", "resume", "history", "objective", "init"}
	
	for _, expected := range expectedCmds {
		found := false
		for _, sub := range subCommands {
			if sub.Use == expected || strings.HasPrefix(sub.Use, expected+" ") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected to find subcommand '%s'", expected)
		}
	}
}

func TestInitCommand(t *testing.T) {
	dir := t.TempDir()
	
	// Temporarily change directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	
	err = os.Chdir(dir)
	if err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	
	// Execute init command
	cmd := rootCmd
	cmd.SetArgs([]string{"init"})
	
	b := bytes.NewBufferString("")
	cmd.SetOut(b)
	cmd.SetErr(b)
	
	err = cmd.Execute()
	if err != nil {
		t.Fatalf("expected no error executing init, got %v", err)
	}
	
	wakeDir := filepath.Join(dir, ".wake")
	if _, err := os.Stat(wakeDir); os.IsNotExist(err) {
		t.Errorf("expected .wake directory to be created by init command")
	}
}

func TestCommandErrorsOutsideGitRepo(t *testing.T) {
	dir := t.TempDir()
	
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	
	err = os.Chdir(dir)
	if err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	
	cmd := rootCmd
	cmd.SetArgs([]string{"status"})
	
	b := bytes.NewBufferString("")
	cmd.SetOut(b)
	cmd.SetErr(b)
	
	err = cmd.Execute()
	if err == nil {
		t.Errorf("expected error running status outside git repo")
	}
}
