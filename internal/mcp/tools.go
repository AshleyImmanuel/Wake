package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AshleyImmanuel/Wake/internal/events"
	"github.com/AshleyImmanuel/Wake/internal/service"
)

func getTools() []Tool {
	return []Tool{
		{
			Name:        "wake_checkpoint",
			Description: "Creates a checkpoint of the current workspace state",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"task_id": map[string]interface{}{
						"type":        "string",
						"description": "Optional task ID to associate with the checkpoint",
					},
					"objective": map[string]interface{}{
						"type":        "string",
						"description": "Optional updated objective",
					},
					"force": map[string]interface{}{
						"type":        "boolean",
						"description": "Force checkpoint creation even if guard checks fail",
					},
					"tracked_files": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "string",
						},
						"description": "Optional list of files to track",
					},
				},
			},
		},
		{
			Name:        "wake_status",
			Description: "Gets reconciliation status of the repository",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"task_id": map[string]interface{}{
						"type":        "string",
						"description": "Optional task ID",
					},
				},
			},
		},
		{
			Name:        "wake_resume",
			Description: "Generates a resume packet to continue a task",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"task_id": map[string]interface{}{
						"type":        "string",
						"description": "Optional task ID",
					},
				},
			},
		},
		{
			Name:        "wake_history",
			Description: "Gets event history for a task",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"task_id": map[string]interface{}{
						"type":        "string",
						"description": "Optional task ID",
					},
					"limit": map[string]interface{}{
						"type":        "number",
						"description": "Limit number of events returned (default 50)",
					},
				},
			},
		},
		{
			Name:        "wake_update_objective",
			Description: "Updates the objective of a task",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"task_id": map[string]interface{}{
						"type":        "string",
						"description": "Task ID",
					},
					"objective": map[string]interface{}{
						"type":        "string",
						"description": "New objective",
					},
				},
				Required: []string{"task_id", "objective"},
			},
		},
		{
			Name:        "wake_record_event",
			Description: "Records an event in the task history",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"task_id": map[string]interface{}{
						"type":        "string",
						"description": "Task ID",
					},
					"event_type": map[string]interface{}{
						"type":        "string",
						"description": "Type of event",
					},
					"payload": map[string]interface{}{
						"type":        "object",
						"description": "Event payload",
					},
				},
				Required: []string{"task_id", "event_type"},
			},
		},
		{
			Name:        "wake_init",
			Description: "Initializes a Wake workspace",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"dir": map[string]interface{}{
						"type":        "string",
						"description": "Optional directory to initialize (defaults to current)",
					},
				},
			},
		},
	}
}

func (s *Server) handleToolCall(ctx context.Context, name string, args map[string]interface{}) (*CallToolResult, error) {
	var result interface{}
	var err error

	getString := func(k string) string {
		if v, ok := args[k].(string); ok {
			return v
		}
		return ""
	}

	switch name {
	case "wake_checkpoint":
		req := service.CheckpointRequest{
			TaskID:    getString("task_id"),
			Objective: getString("objective"),
			Dir:       s.workDir,
			Force:     false,
		}
		if force, ok := args["force"].(bool); ok {
			req.Force = force
		}
		if tFiles, ok := args["tracked_files"].([]interface{}); ok {
			for _, f := range tFiles {
				if fs, ok := f.(string); ok {
					req.TrackedFiles = append(req.TrackedFiles, fs)
				}
			}
		}
		result, err = s.svc.CreateCheckpoint(ctx, req)

	case "wake_status":
		req := service.StatusRequest{
			TaskID: getString("task_id"),
			Dir:    s.workDir,
		}
		result, err = s.svc.GetStatus(ctx, req)

	case "wake_resume":
		result, err = s.svc.ResumeTask(ctx, getString("task_id"))

	case "wake_history":
		limit := 50
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}
		result, err = s.svc.GetHistory(ctx, getString("task_id"), limit)

	case "wake_update_objective":
		taskID := getString("task_id")
		objective := getString("objective")
		if taskID == "" || objective == "" {
			return nil, fmt.Errorf("task_id and objective are required")
		}
		err = s.svc.UpdateObjective(ctx, taskID, objective)
		if err == nil {
			result = map[string]string{"status": "success"}
		}

	case "wake_record_event":
		taskID := getString("task_id")
		eType := getString("event_type")
		if taskID == "" || eType == "" {
			return nil, fmt.Errorf("task_id and event_type are required")
		}
		var payload map[string]interface{}
		if p, ok := args["payload"].(map[string]interface{}); ok {
			payload = p
		}
		result, err = s.svc.RecordEvent(ctx, taskID, events.EventType(eType), payload)

	case "wake_init":
		dir := getString("dir")
		if dir == "" {
			dir = s.workDir
		}
		err = s.svc.InitWorkspace(ctx, dir)
		if err == nil {
			result = map[string]string{"status": "initialized", "dir": dir}
		}

	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}

	if err != nil {
		return nil, err
	}

	b, _ := json.MarshalIndent(result, "", "  ")
	return &CallToolResult{
		Content: []interface{}{
			TextContent{
				Type: "text",
				Text: string(b),
			},
		},
	}, nil
}
