package service

import (
	"github.com/wake/wake/internal/state"
	"github.com/wake/wake/internal/reconcile"
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
