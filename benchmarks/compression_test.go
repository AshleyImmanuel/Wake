package benchmarks

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"wake/internal/events"
	"wake/internal/state"
	"github.com/google/uuid"
)

// roughTokenEstimate provides a very crude 1 token ≈ 4 chars estimate for benchmarking.
func roughTokenEstimate(b []byte) int {
	return len(b) / 4
}

// TestContextCompression simulates a 500-event agent session and verifies the state reduction.
func TestContextCompression(t *testing.T) {
	taskID := uuid.New()
	sessionID := uuid.New()
	var history []events.Event

	history = append(history, events.NewEvent(taskID, sessionID, events.TaskStarted, "AI", map[string]interface{}{
		"objective": "Build a robust recovery state model with diffing and context compression",
		"tasks": []string{
			"Implement Diffing",
			"Implement Session Continuity",
			"Implement Dual Formats",
			"Benchmark Context Compression",
		},
	}))

	// Simulate 500 noisy events (e.g. commands run, files changed, tests failed)
	for i := 0; i < 500; i++ {
		cmdStr := fmt.Sprintf("go test ./internal/state -v -run TestFeature%d", i)
		history = append(history, events.NewEvent(taskID, sessionID, events.CommandExecuted, "AI", map[string]interface{}{
			"command":   cmdStr,
			"exit_code": 1,
		}))

		errStr := fmt.Sprintf("Error in feature %d: missing implementation", i)
		history = append(history, events.NewEvent(taskID, sessionID, events.TestFailed, "AI", map[string]interface{}{
			"test":  fmt.Sprintf("TestFeature%d", i),
			"error": errStr,
		}))

		// Some files changed to fix it
		history = append(history, events.NewEvent(taskID, sessionID, events.FileChanged, "AI", map[string]interface{}{
			"file":   fmt.Sprintf("internal/state/feature%d.go", i),
			"action": "modified",
		}))

		// Passing test
		history = append(history, events.NewEvent(taskID, sessionID, events.TestPassed, "AI", map[string]interface{}{
			"test": fmt.Sprintf("TestFeature%d", i),
		}))

		if i%125 == 0 {
			history = append(history, events.NewEvent(taskID, sessionID, events.MilestoneCompleted, "AI", map[string]interface{}{
				"milestone": fmt.Sprintf("Completed 25%% of Feature %d", i),
			}))
		}
	}

	// Final state
	history = append(history, events.NewEvent(taskID, sessionID, events.MilestoneCompleted, "AI", map[string]interface{}{
		"milestone": "Implement Diffing",
	}))
	history = append(history, events.NewEvent(taskID, sessionID, events.BlockerCreated, "AI", map[string]interface{}{
		"id":          "B1",
		"description": "Need to refactor database layer first",
	}))

	// 1. Calculate size of raw chat history (representing a full LLM context window)
	rawHistoryBytes, _ := json.Marshal(history)
	rawTokens := roughTokenEstimate(rawHistoryBytes)

	// 2. Reduce state
	startTime := time.Now()
	reducedState := state.Reduce(taskID.String(), history)
	reduceDuration := time.Since(startTime)

	// 3. Calculate size of collapsed state (what the new agent sees)
	stateBytes, _ := json.Marshal(reducedState)
	stateTokens := roughTokenEstimate(stateBytes)

	// 4. Verification & Output
	fmt.Printf("\n=======================================================\n")
	fmt.Printf("WAKE CONTEXT COMPRESSION BENCHMARK\n")
	fmt.Printf("=======================================================\n")
	fmt.Printf("Total Events Simulated: %d\n", len(history))
	fmt.Printf("Time to Reduce State:   %v\n\n", reduceDuration)

	fmt.Printf("Raw Event History:      ~%d tokens (%d bytes)\n", rawTokens, len(rawHistoryBytes))
	fmt.Printf("Wake Recovery State:    ~%d tokens (%d bytes)\n\n", stateTokens, len(stateBytes))

	compressionRatio := float64(rawTokens) / float64(stateTokens)
	percentage := (1.0 - (float64(stateTokens) / float64(rawTokens))) * 100

	fmt.Printf("Reduction:              %.2f%% (%.1fx smaller)\n", percentage, compressionRatio)
	fmt.Printf("=======================================================\n\n")

	if percentage < 95.0 {
		t.Fatalf("Expected at least 95%% context compression, got %.2f%%", percentage)
	}

	if !strings.Contains(reducedState.Objective, "diffing and context compression") {
		t.Fatalf("Objective was lost during reduction")
	}
}
