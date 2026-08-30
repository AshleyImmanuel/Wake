package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"wake/internal/events"
	"wake/internal/service"
)

func getTools() []Tool {
	return []Tool{
		{
			Name:        "wake_checkpoint",
			Description: "Save state",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"task_id":       map[string]interface{}{"type": "string"},
					"objective":     map[string]interface{}{"type": "string"},
					"force":         map[string]interface{}{"type": "boolean"},
					"tracked_files": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
				},
			},
		},
		{
			Name:        "wake_status",
			Description: "Check status",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"task_id": map[string]interface{}{"type": "string"},
				},
			},
		},
		{
			Name:        "wake_resume",
			Description: "Resume task",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"task_id": map[string]interface{}{"type": "string"},
				},
			},
		},
		{
			Name:        "wake_diff",
			Description: "Compare states",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"task_id": map[string]interface{}{"type": "string"},
					"v1":      map[string]interface{}{"type": "number"},
					"v2":      map[string]interface{}{"type": "number"},
				},
			},
		},
		{
			Name:        "wake_history",
			Description: "Get history",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"task_id": map[string]interface{}{"type": "string"},
					"limit":   map[string]interface{}{"type": "number"},
				},
			},
		},
		{
			Name:        "wake_update_objective",
			Description: "Update objective",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"task_id":   map[string]interface{}{"type": "string"},
					"objective": map[string]interface{}{"type": "string"},
				},
				Required: []string{"task_id", "objective"},
			},
		},
		{
			Name:        "wake_record_event",
			Description: "Log event",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"task_id":    map[string]interface{}{"type": "string"},
					"event_type": map[string]interface{}{"type": "string"},
					"payload":    map[string]interface{}{"type": "object"},
				},
				Required: []string{"task_id", "event_type"},
			},
		},
		{
			Name:        "wake_init",
			Description: "Init Wake",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"dir": map[string]interface{}{"type": "string"},
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
		res, e := s.svc.CreateCheckpoint(ctx, req)
		err = e
		if err == nil {
			result = fmt.Sprintf("Checkpoint created successfully.\nTask ID: %s\nState Version: %d", res.TaskID.String(), res.StateVersion)
		}

	case "wake_status":
		req := service.StatusRequest{
			TaskID: getString("task_id"),
			Dir:    s.workDir,
		}
		res, e := s.svc.GetStatus(ctx, req)
		err = e
		if err == nil {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Status: %s\nConfidence: %s\nReason: %s\n", res.Status, res.ConfidenceLevel, res.Reason))
			if len(res.ChangedFiles) > 0 {
				sb.WriteString("Changed:\n")
				for _, f := range res.ChangedFiles {
					sb.WriteString(fmt.Sprintf("- %s\n", f))
				}
			}
			if len(res.ConstraintViolations) > 0 {
				sb.WriteString("Violations:\n")
				for _, v := range res.ConstraintViolations {
					sb.WriteString(fmt.Sprintf("- %s\n", v))
				}
			}
			result = sb.String()
		}

	case "wake_resume":
		res, e := s.svc.ResumeTask(ctx, getString("task_id"))
		err = e
		if err == nil {
			result = map[string]interface{}{
				"state":    res.Checkpoint.StateData,
				"status":   res.ReconciliationResult.Status,
				"guidance": res.Guidance,
			}
		}

	case "wake_diff":
		taskID := getString("task_id")
		v1 := 0
		if val, ok := args["v1"].(float64); ok {
			v1 = int(val)
		}
		v2 := 0
		if val, ok := args["v2"].(float64); ok {
			v2 = int(val)
		}
		res, e := s.svc.DiffCheckpoints(ctx, taskID, v1, v2)
		err = e
		if err == nil {
			result = service.FormatDiff(*res)
		}

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
		// Validate event type against known types
		validTypes := map[events.EventType]bool{
			events.TaskStarted: true, events.RequirementAdded: true,
			events.ConstraintAdded: true, events.UserApproval: true,
			events.UserRejection: true, events.DecisionMade: true,
			events.FileChanged: true, events.CommandExecuted: true,
			events.TestStarted: true, events.TestPassed: true,
			events.TestFailed: true, events.BlockerCreated: true,
			events.BlockerResolved: true, events.MilestoneCompleted: true,
			events.GitCommit: true, events.SessionInterrupted: true,
			events.SessionResumed: true,
		}
		if !validTypes[events.EventType(eType)] {
			return nil, fmt.Errorf("invalid event_type: %s", eType)
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

	var textOut string
	if str, ok := result.(string); ok {
		textOut = str
	} else {
		b, _ := json.Marshal(result) // Caveman: No indent, huge token savings
		textOut = string(b)
	}

	return &CallToolResult{
		Content: []interface{}{
			TextContent{
				Type: "text",
				Text: textOut,
			},
		},
	}, nil
}
