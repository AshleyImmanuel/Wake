package mcp

import (
	"context"
	"fmt"

	"wake/internal/service"
)

func getPrompts() []Prompt {
	return []Prompt{
		{
			Name:        "wake_session_start",
			Description: "Generates a session start prompt with current state context",
			Arguments: []PromptArgument{
				{
					Name:        "task_id",
					Description: "Optional Task ID",
					Required:    false,
				},
			},
		},
		{
			Name:        "wake_pre_commit_audit",
			Description: "Generates a pre-commit review prompt with changed files",
			Arguments: []PromptArgument{
				{
					Name:        "task_id",
					Description: "Optional Task ID",
					Required:    false,
				},
			},
		},
		{
			Name:        "wake_conflict_resolution",
			Description: "Generates conflict resolution guidance when state is CONFLICT",
			Arguments: []PromptArgument{
				{
					Name:        "task_id",
					Description: "Optional Task ID",
					Required:    false,
				},
			},
		},
	}
}

func (s *Server) handlePromptGet(ctx context.Context, name string, args map[string]string) (*GetPromptResult, error) {
	taskID := args["task_id"]

	switch name {
	case "wake_session_start":
		resume, err := s.svc.ResumeTask(ctx, taskID)
		if err != nil {
			return nil, err
		}

		textOut := service.FormatResumePacket(resume)
		content := fmt.Sprintf("%s\nBriefly tell the user where you left off, then continue from Next Action.", textOut)

		return &GetPromptResult{
			Description: "Session Start Context",
			Messages: []PromptMessage{
				{
					Role: "assistant",
					Content: TextContent{
						Type: "text",
						Text: content,
					},
				},
			},
		}, nil

	case "wake_pre_commit_audit":
		resume, err := s.svc.ResumeTask(ctx, taskID)
		if err != nil {
			return nil, err
		}

		changes := resume.ReconciliationResult.ChangedFiles
		content := fmt.Sprintf("Review changed files before checkpoint: %v\nObjective: %s", changes, resume.Checkpoint.StateData.Objective)

		return &GetPromptResult{
			Description: "Pre-commit Audit",
			Messages: []PromptMessage{
				{
					Role: "assistant",
					Content: TextContent{
						Type: "text",
						Text: content,
					},
				},
			},
		}, nil

	case "wake_conflict_resolution":
		content := "CONFLICT: Resolve merge conflicts, run git merge/rebase --continue, then wake_checkpoint."
		return &GetPromptResult{
			Description: "Conflict Resolution Guidance",
			Messages: []PromptMessage{
				{
					Role: "assistant",
					Content: TextContent{
						Type: "text",
						Text: content,
					},
				},
			},
		}, nil

	default:
		return nil, fmt.Errorf("prompt not found: %s", name)
	}
}
