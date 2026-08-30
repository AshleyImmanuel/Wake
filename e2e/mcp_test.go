package e2e

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/AshleyImmanuel/Wake/internal/testutil"
)

func TestE2E_MCP(t *testing.T) {
	repoDir := testutil.SetupTempRepo(t)

	// Init workspace
	runWake(t, repoDir, "init")

	// Start MCP server subprocess
	cmd := exec.Command(wakeBin, "mcp")
	cmd.Dir = repoDir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("failed to get stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to get stdout pipe: %v", err)
	}

	// Capture stderr for debugging
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start mcp: %v", err)
	}

	// Helper to send request and read response
	sendRequest := func(req map[string]interface{}) map[string]interface{} {
		b, _ := json.Marshal(req)
		stdin.Write(b)
		stdin.Write([]byte("\n"))

		reader := bufio.NewReader(stdout)
		line, err := reader.ReadBytes('\n')
		if err != nil {
			t.Fatalf("failed to read response: %v\nStderr: %s", err, stderrBuf.String())
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(line, &resp); err != nil {
			t.Fatalf("failed to parse response %s: %v", string(line), err)
		}
		return resp
	}

	// 1. Initialize
	initReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	}
	resp := sendRequest(initReq)
	if resp["id"] != float64(1) {
		t.Errorf("expected id 1, got %v", resp["id"])
	}

	// 2. Call wake_status tool
	callReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      "wake_status",
			"arguments": map[string]interface{}{},
		},
	}
	resp = sendRequest(callReq)
	if resp["id"] != float64(2) {
		t.Errorf("expected id 2, got %v", resp["id"])
	}

	// Check result - it should be an error because no checkpoint exists
	result, ok := resp["result"].(map[string]interface{})
	if !ok || result["isError"] != true {
		t.Errorf("expected error result (no checkpoint), got %v", resp)
	}

	// Close stdin to stop the server
	stdin.Close()
	cmd.Wait()
}
