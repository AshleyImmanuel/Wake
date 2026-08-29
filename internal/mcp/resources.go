package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

func getResources() []Resource {
	return []Resource{
		{
			Uri:         "wake://state/current",
			Name:        "Current State",
			Description: "Current task state data",
		},
		{
			Uri:         "wake://events/recent",
			Name:        "Recent Events",
			Description: "Recent event history for the current task",
		},
		{
			Uri:         "wake://checkpoint/latest",
			Name:        "Latest Checkpoint",
			Description: "Data from the latest task checkpoint",
		},
		{
			Uri:         "wake://reconciliation/status",
			Name:        "Reconciliation Status",
			Description: "Current git and state reconciliation status",
		},
	}
}

func (s *Server) handleResourceRead(ctx context.Context, uri string) (*ReadResourceResult, error) {
	switch uri {
	case "wake://state/current":
		resume, err := s.svc.ResumeTask(ctx, "")
		if err != nil {
			return nil, err
		}
		return toResourceResult(uri, resume.Checkpoint.StateData)

	case "wake://events/recent":
		evs, err := s.svc.GetHistory(ctx, "", 20)
		if err != nil {
			return nil, err
		}
		return toResourceResult(uri, evs)

	case "wake://checkpoint/latest":
		resume, err := s.svc.ResumeTask(ctx, "")
		if err != nil {
			return nil, err
		}
		return toResourceResult(uri, resume.Checkpoint)

	case "wake://reconciliation/status":
		resume, err := s.svc.ResumeTask(ctx, "")
		if err != nil {
			return nil, err
		}
		return toResourceResult(uri, resume.ReconciliationResult)

	default:
		return nil, fmt.Errorf("resource not found: %s", uri)
	}
}

func toResourceResult(uri string, data interface{}) (*ReadResourceResult, error) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, err
	}
	return &ReadResourceResult{
		Contents: []ResourceContents{
			{
				Uri:      uri,
				MimeType: "application/json",
				Text:     string(b),
			},
		},
	}, nil
}
