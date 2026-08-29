package mcp

import (
	"context"
	"encoding/json"
	"fmt"
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

		b, _ := json.MarshalIndent(resume.Checkpoint.StateData, "", "  ")
		content := fmt.Sprintf("Welcome back to your task! Here is your current state:\n\n%s\n\nReconciliation Guidance: %s", string(b), resume.Guidance)

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
		
		content := fmt.Sprintf("Please review the following changed files before creating a checkpoint:\n\n%v\n\nEnsure that these changes align with the objective: %s", changes, resume.Checkpoint.StateData.Objective)

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
		content := "The repository is currently in a CONFLICT state due to a merge or rebase. Please resolve the conflicts in the affected files and run `git merge --continue` or `git rebase --continue`. Finally, run the `wake_checkpoint` tool to record your resolution."
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
