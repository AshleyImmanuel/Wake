package service

import (
	"github.com/AshleyImmanuel/Wake/internal/reconcile"
	"github.com/AshleyImmanuel/Wake/internal/state"
)

type CheckpointRequest struct {
	TaskID       string
	Objective    string
	Dir          string
	TrackedFiles []string
	Force        bool
}

type StatusRequest struct {
	TaskID string
	Dir    string
}

type ResumePacket struct {
	Checkpoint           state.Checkpoint
	ReconciliationResult reconcile.ReconciliationResult
	Guidance             string
}
