# Original User Request

## Initial Request — 2026-08-28T16:52:54Z

# Teamwork Project Prompt — Draft

> Status: Launched
> Goal: Craft prompt → get user approval → delegate to teamwork_preview
> Requested team: [none — teamwork routes from the description]

Implement Phase 2 (Reconciliation) of the Sentinel MVP. This involves building the Git comparison utilities and a reconciliation engine that checks a saved task state checkpoint against the current live Git repository to determine if the state is SAFE, STALE, or CONFLICT.

Working directory: C:/Users/USER/Desktop/Sentinel
Integrity mode: development

## Requirements

### R1. Git CLI Wrapper
Build a utility layer that shells out to the local `git` binary to retrieve current repository information. It must be able to retrieve the current commit hash, list modified files, and list uncommitted changes.

### R2. Reconciliation Engine
Implement a comparison function that takes a saved `Checkpoint` object (from the SQLite database layer) and the current Git repository state. It must evaluate the differences and return a status of SAFE, STALE, or CONFLICT.

## Acceptance Criteria

### Verification Suite
- [ ] A Go test suite exists that uses a temporary Git repository to simulate SAFE, STALE, and CONFLICT scenarios.
- [ ] The test suite runs automatically via `go test` and passes without human intervention.
- [ ] The reconciliation engine correctly returns SAFE when the simulated repository exactly matches the checkpoint commit with no uncommitted changes.
- [ ] The reconciliation engine correctly returns CONFLICT or STALE when simulated task-related files have been manually modified since the checkpoint.
