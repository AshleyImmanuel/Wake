package service

import (
	"fmt"
	"strings"

	"github.com/AshleyImmanuel/Wake/internal/reconcile"
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
		for _, c := range cp.StateData.Completed {
			sb.WriteString(fmt.Sprintf("- %s\n", c))
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
