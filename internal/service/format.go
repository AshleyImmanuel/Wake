package service

import (
	"fmt"
	"strings"

	"wake/internal/reconcile"
	"wake/internal/state"
)

// FormatResumePacket generates an ultra-minimal text block
// summarizing the current task state. This is used by both the CLI and the MCP server.
func FormatResumePacket(packet *ResumePacket) string {
	var sb strings.Builder
	cp := packet.Checkpoint
	result := packet.ReconciliationResult

	sb.WriteString(fmt.Sprintf("# Task: %s\n", cp.TaskID.String()))

	if cp.StateData.Objective != "" {
		sb.WriteString(fmt.Sprintf("## Goal\n%s\n", cp.StateData.Objective))
	}

	if len(cp.StateData.Completed) > 0 {
		sb.WriteString("## Completed\n")

		displayCount := 3
		total := len(cp.StateData.Completed)

		if total > displayCount {
			sb.WriteString(fmt.Sprintf("... +%d more\n", total-displayCount))
			for i := total - displayCount; i < total; i++ {
				sb.WriteString(fmt.Sprintf("- %s\n", cp.StateData.Completed[i]))
			}
		} else {
			for _, c := range cp.StateData.Completed {
				sb.WriteString(fmt.Sprintf("- %s\n", c))
			}
		}
	}

	if cp.StateData.Current != "" {
		sb.WriteString(fmt.Sprintf("## Current\n%s\n", cp.StateData.Current))
	}

	activeBlockers := 0
	for _, b := range cp.StateData.Blocked {
		if b.Status == "ACTIVE" {
			if activeBlockers == 0 {
				sb.WriteString("## Blockers\n")
			}
			sb.WriteString(fmt.Sprintf("- %s: %s\n", b.ID, b.Description))
			activeBlockers++
		}
	}

	if len(cp.StateData.Constraints) > 0 {
		sb.WriteString("## Constraints\n")
		for _, c := range cp.StateData.Constraints {
			sb.WriteString(fmt.Sprintf("- %s\n", c))
		}
	}

	if len(cp.StateData.DoNotRepeat) > 0 {
		sb.WriteString("## Do Not Repeat\n")
		for _, c := range cp.StateData.DoNotRepeat {
			sb.WriteString(fmt.Sprintf("- %s\n", c))
		}
	}

	if cp.Commit != "" {
		sb.WriteString(fmt.Sprintf("## Last Verified\nCommit %s\n", cp.Commit))
	}

	if cp.StateData.NextAction != "" {
		sb.WriteString(fmt.Sprintf("## Next Action\n%s\n", cp.StateData.NextAction))
	}

	sb.WriteString(fmt.Sprintf("## State Confidence\n%s\n", result.ConfidenceLevel))

	sb.WriteString("## Workspace Delta\n")
	if result.Status == reconcile.StatusSafe {
		sb.WriteString("Safe to resume. No modifications since last checkpoint.\n")
	} else {
		sb.WriteString(fmt.Sprintf("Status: %s\n", result.Status))

		if packet.Guidance != "" {
			if !strings.Contains(packet.Guidance, "CRITICAL") && len(result.ChangedFiles) > 0 {
				sb.WriteString("Changed files:\n")
				for _, f := range result.ChangedFiles {
					sb.WriteString(fmt.Sprintf("- %s\n", f))
				}
			}
			sb.WriteString(fmt.Sprintf("%s\n", packet.Guidance))
		}
	}

	return sb.String()
}

// FormatVisualStatus generates a human-readable visual status for the CLI (PRD Section 6).
func FormatVisualStatus(cp *state.Checkpoint) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Task:\n%s\n\n", cp.StateData.Objective))
	if cp.StateData.Objective == "" {
		sb.WriteString("Task:\n(No objective set)\n\n")
	}

	// Calculate Progress
	completedCount := len(cp.StateData.Completed)
	remainingCount := len(cp.StateData.Remaining)
	blockedCount := 0
	for _, b := range cp.StateData.Blocked {
		if b.Status == "ACTIVE" {
			blockedCount++
		}
	}
	total := completedCount + remainingCount + blockedCount
	percentage := 0
	if total > 0 {
		percentage = (completedCount * 100) / total
	}

	blocks := percentage / 10
	bar := strings.Repeat("█", blocks) + strings.Repeat("░", 10-blocks)
	sb.WriteString(fmt.Sprintf("Progress:\n%s %d%%\n\n", bar, percentage))

	if len(cp.StateData.Completed) > 0 {
		sb.WriteString("Completed:\n")
		for _, c := range cp.StateData.Completed {
			sb.WriteString(fmt.Sprintf("✓ %s\n", c))
		}
		sb.WriteString("\n")
	}

	if cp.StateData.Current != "" {
		sb.WriteString("In progress:\n")
		sb.WriteString(fmt.Sprintf("→ %s\n\n", cp.StateData.Current))
	}

	if blockedCount > 0 {
		sb.WriteString("Blocker:\n")
		for _, b := range cp.StateData.Blocked {
			if b.Status == "ACTIVE" {
				sb.WriteString(fmt.Sprintf("⚠ %s\n", b.Description))
			}
		}
		sb.WriteString("\n")
	}

	if cp.StateData.NextAction != "" {
		sb.WriteString("Next:\n")
		sb.WriteString(fmt.Sprintf("→ %s\n", cp.StateData.NextAction))
	}

	if cp.StateData.LastCommandResult == "FAILED" {
		sb.WriteString(fmt.Sprintf("\nLAST CRASH CONTEXT:\nCommand: %s\nResult: FAILED\n", cp.StateData.LastCommand))
	}

	return strings.TrimSpace(sb.String())
}
