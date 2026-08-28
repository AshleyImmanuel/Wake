# Project Wake
## Product Requirements Document — v0.3

**Working name:** Wake  
**Product category:** AI coding-agent recovery infrastructure  
**Status:** Pre-MVP / validation stage  
**Date:** August 28, 2026

---

# 1. Product Definition

Wake is a plug-and-play layer for existing AI coding agents that reconstructs and restores the **current executable state of a coding task** after a context or session transition.

Wake does not replace:

- the IDE
- the coding agent
- project instructions
- memory systems
- planning systems
- testing systems
- observability platforms

Wake exists specifically to answer:

> **“What state is this coding task actually in right now, and can a new agent session safely continue from it?”**

---

# 2. The Single Problem

## Problem

Long-running AI coding tasks can cross context or session boundaries.

Examples:

- context compaction
- session restart
- IDE restart
- agent crash
- terminal restart
- interrupted task
- handoff to another agent/session

At that point, the next session has access to some combination of:

- conversation summaries
- memory
- `AGENTS.md`
- `CLAUDE.md`
- plans/specifications
- git history
- repository contents
- test results

But those sources do not necessarily represent the **actual current execution state of the task**.

The result can be:

- repeated work
- forgotten completed steps
- lost blockers
- forgotten user decisions
- contradictory actions
- unnecessary repository rediscovery
- incorrect next actions
- increased token consumption
- developer intervention

---

# 3. Precise Problem Statement

> **Existing agent context mechanisms preserve knowledge and instructions, but they do not guarantee accurate reconstruction of the live execution state of an interrupted coding task.**

Wake is designed around this gap.

---

# 4. What Wake Is Not

This distinction is mandatory.

## Wake is NOT an AI memory system.

Memory answers:

> “What information should the agent retain?”

Wake answers:

> “What state must be restored for this task to continue correctly?”

## Wake is NOT an `AGENTS.md` replacement.

`AGENTS.md` describes rules and persistent project instructions.

Wake records the evolving state of a specific task.

## Wake is NOT a plan manager.

A plan says what should happen.

Wake tracks what has actually happened.

## Wake is NOT observability.

Observability records activity.

Wake turns relevant activity into recoverable task state.

## Wake is NOT a code reviewer.

It does not primarily judge code quality.

## Wake is NOT a browser tester.

Testing can provide evidence to Wake, but browser testing itself is outside the core product.

---

# 5. Target Customer

## Primary

Individual developers and small teams using AI coding agents for multi-step tasks.

Examples:

- feature implementation
- migrations
- large refactors
- bug investigations
- infrastructure changes
- long-running upgrades

## Initial target

Developers who regularly allow agents to work for long enough that:

> **their original context is no longer sufficient for reliable continuation.**

---

# 6. Core User Story

A developer starts:

> “Migrate the billing service from MongoDB to PostgreSQL without touching authentication.”

An agent works for several hours.

Before the session ends, Wake has:

```text
TASK
Billing migration

GOAL
MongoDB → PostgreSQL

CONSTRAINT
Authentication must remain unchanged

COMPLETED
✓ customer migration
✓ invoice migration
✓ schema conversion
✓ billing API

CURRENT
Payment webhooks

BLOCKER
Stripe webhook still expects ObjectId

VALIDATED
✓ customer API
✓ invoice API

REMAINING
○ webhook migration
○ refund flow
○ final verification

LAST VERIFIED COMMIT
83ab21

NEXT ACTION
Fix ObjectId conversion in webhook layer
```

The session ends.

The developer starts a new session.

Instead of making the agent rediscover everything, Wake performs state recovery.

---

# 7. Core Product Loop

```text
Existing Agent
      ↓
Observe execution
      ↓
Capture relevant events
      ↓
Update task state
      ↓
Attach evidence
      ↓
Create checkpoint
      ↓
Context/session transition
      ↓
Reconcile checkpoint with current repository
      ↓
Produce recovery state
      ↓
Resume agent
```

---

# 8. Core Components

Wake MVP consists of five major components.

## 8.1 Agent Adapter

Connects Wake to an existing coding-agent environment.

Responsibilities:

- observe agent execution
- capture relevant tool events
- detect session boundaries where possible
- expose recovery injection
- expose task lifecycle

The adapter must be vendor-specific.

The core state engine must remain vendor-independent.

---

# 8.2 Event Collector

Collects important events such as:

```text
TASK_STARTED
REQUIREMENT_ADDED
CONSTRAINT_ADDED
USER_APPROVAL
USER_REJECTION
DECISION_MADE

FILE_CHANGED
COMMAND_EXECUTED
TEST_STARTED
TEST_PASSED
TEST_FAILED

BLOCKER_CREATED
BLOCKER_RESOLVED
MILESTONE_COMPLETED

GIT_COMMIT
SESSION_INTERRUPTED
SESSION_RESUMED
```

Wake must avoid treating every conversational message as important state.

---

# 8.3 State Engine

Converts events into structured task state.

The state contains:

```text
Objective
Constraints
Decisions
Completed
Current
Remaining
Blocked
Evidence
Validation
Next Action
Do Not Repeat
```

---

# 8.4 Checkpoint Store

Stores versioned task checkpoints.

Example:

```text
Checkpoint 01
Checkpoint 02
Checkpoint 03
...
Checkpoint 18 ← latest
```

Each checkpoint should reference the repository state against which it was created.

Important metadata:

```text
task_id
timestamp
repository
branch
commit
state_version
event_position
```

---

# 8.5 Recovery Engine

The recovery engine is the central differentiator.

It must not simply load the previous checkpoint.

It must determine:

> **Is this checkpoint still valid against the current state of the project?**

---

# 9. State Reconciliation

This is the most important new component in v0.3.

When a task resumes:

```text
Previous checkpoint
        +
Current repository
        +
Current git state
        +
Available validation evidence
        ↓
STATE RECONCILIATION
        ↓
SAFE / STALE / CONFLICT
```

## SAFE

Checkpoint remains consistent.

Example:

```text
Checkpoint:
payment API completed

Current repository:
same relevant implementation

Result:
SAFE TO RESUME
```

## STALE

Repository has changed since checkpoint but the change can potentially be incorporated.

Example:

```text
Checkpoint:
payment API completed

Current:
payment API manually modified

Result:
STALE
→ update task state before resume
```

## CONFLICT

Checkpoint assumptions materially contradict current repository state.

Example:

```text
Checkpoint:
authentication untouched

Current:
auth/session.ts modified significantly

Result:
CONFLICT
→ require reconciliation
```

Wake must prefer uncertainty over silently resuming from incorrect state.

---

# 10. Evidence Model

Every important state entry should have provenance.

## VERIFIED

Supported by observable evidence.

Example:

```text
DATABASE MIGRATION = VERIFIED

Evidence:
migration command exited 0
schema check passed
24 migration tests passed
commit 83ab21
```

## USER_CONFIRMED

Explicitly provided or approved by the developer.

Example:

```text
Authentication must remain unchanged.
```

## AGENT_INFERRED

Claim derived from agent reasoning but not independently supported.

Example:

```text
Webhook implementation is probably complete.
```

## UNKNOWN

Insufficient information.

This prevents Wake from turning previous agent guesses into permanent facts.

---

# 11. Checkpoint Design

Wake should checkpoint **state transitions**, not every event.

Recommended triggers:

- milestone completion
- important user decision
- blocker creation/resolution
- successful validation milestone
- git commit
- explicit user checkpoint
- interruption
- session termination
- context transition when detectable
- significant task-state change

---

# 12. Recovery Packet

On resume, Wake should create a compact recovery packet.

Example:

```text
RESUMING TASK: #184

GOAL
Implement Stripe subscriptions.

COMPLETED
✓ database schema
✓ customer creation
✓ checkout

CURRENT
Webhook processing

BLOCKER
Signature verification failing in staging.

CONSTRAINT
Do not modify existing authentication.

DO NOT REPEAT
Database migration
Customer creation

LAST VERIFIED
Commit 8a4d92f

NEXT ACTION
Investigate webhook signature middleware.

STATE CONFIDENCE
High
```

The recovery packet should be much smaller than replaying an entire session.

---

# 13. Recovery Must Be State-First

Wake should not tell a new agent:

> “Here is a giant summary of the previous conversation.”

Instead it should provide structured state first.

Potential format:

```text
TASK STATE
↓
RELEVANT EVIDENCE
↓
CURRENT REPOSITORY DELTA
↓
RECOVERY INSTRUCTIONS
```

This reduces dependence on raw conversational history.

---

# 14. Repository Reconciliation

Before recovery, Wake should compare:

### Git

- checkpoint commit
- current commit
- branch
- uncommitted files
- modified files

### Files

- expected files
- changed task files
- deleted files
- unexpected modifications

### Validation

- latest test results
- known failures
- stale test results

### Task state

- completed steps
- blockers
- remaining work
- decisions

The system should produce a reconciliation result.

Example:

```text
CHECKPOINT RECONCILIATION

Repository changed since checkpoint.

Unrelated changes:
3 files

Task-related changes:
2 files

Previous completed claim:
Payment API complete

Current status:
Still consistent

Result:
SAFE WITH UPDATES
```

---

# 15. Do-Not-Repeat Mechanism

Wake should explicitly record completed work that should not normally be repeated.

Example:

```text
DO NOT REPEAT

✓ database migration
✓ customer migration
✓ schema generation
```

This is not a rigid prohibition.

If repository reconciliation shows that the migration has become invalid, Wake should downgrade the claim:

```text
Previous:
migration verified

Current:
migration files changed

Result:
verification invalidated
```

This makes “do not repeat” evidence-dependent rather than blindly authoritative.

---

# 16. Decision History

Important decisions should become first-class state.

Example:

```text
DECISION #12

Decision:
Keep authentication unchanged.

Source:
Developer instruction.

Status:
ACTIVE
```

If an agent later proposes changing authentication:

```text
STATE CONFLICT

Existing decision:
Authentication must remain unchanged.

Current proposed change:
auth/session.ts

Action:
Require developer attention.
```

---

# 17. State Invalidation

A checkpoint must not remain trusted forever.

State can be invalidated by:

- repository changes
- failed tests
- reverted commits
- changed requirements
- user rejection
- external configuration changes
- detected contradictions

Example:

```text
Previous:
Checkout integration verified

New evidence:
checkout test failing

State:
INVALIDATED
```

---

# 18. User Experience

The MVP should be lightweight.

## CLI

```bash
Wake run
Wake status
Wake checkpoint
Wake history
Wake resume
Wake inspect
```

Example:

```text
$ Wake status

TASK
Stripe subscriptions

STATUS
68%

CURRENT
Webhook handling

BLOCKER
Signature verification

LAST CHECKPOINT
18:43

STATE
Consistent
```

Resume:

```bash
Wake resume
```

---

# 19. No New IDE Required

The product should preserve the existing developer workflow.

Desired experience:

```text
Developer
   ↓
Existing IDE
   ↓
Existing AI agent
   ↓
Wake
```

The developer should not need to migrate their project into another development environment.

---

# 20. Initial Integration Strategy

Do not support every agent at launch.

The first integration should be selected based on:

- reliable process boundary
- useful event visibility
- ability to inject recovery state
- reproducible session behavior

A CLI-based agent is the preferred experimental starting point.

Potential later adapters:

```text
Codex
Claude Code
Cursor
Antigravity
Gemini CLI / Antigravity CLI
```

Integration claims should be based on actual available APIs/hooks/interfaces rather than assumptions.

---

# 21. Local-First Architecture

The MVP should operate locally whenever possible.

Recommended storage:

```text
SQLite
```

Source code and task state should not be uploaded to a cloud service by default.

Cloud synchronization is deferred.

---

# 22. Security

Wake may observe source-code and command activity.

Therefore:

- secrets must be filtered
- API keys must not be persisted
- credentials must not be persisted
- users can exclude files/directories
- task history must be deletable
- storage should be encrypted where appropriate
- remote transmission must be opt-in

---

# 23. Technical Architecture

```text
                 EXISTING CODING AGENT
                           │
                           ▼
                     AGENT ADAPTER
                           │
                           ▼
                      EVENT BUS
                           │
                           ▼
                     STATE ENGINE
                 ┌─────────┼─────────┐
                 ↓         ↓         ↓
              TASK       EVIDENCE   DECISIONS
               STATE        │         │
                 └─────────┼─────────┘
                           ↓
                     CHECKPOINT STORE
                           │
                           ▼
                  SESSION / CONTEXT EVENT
                           │
                           ▼
                   RECONCILIATION ENGINE
                     ┌─────┼─────┐
                     ↓     ↓     ↓
                    SAFE  STALE CONFLICT
                           │
                           ▼
                    RECOVERY ENGINE
                           │
                           ▼
                     RECOVERY PACKET
                           │
                           ▼
                     NEW AGENT SESSION
```

---

# 24. Event-Sourced Foundation

The underlying system should retain an event history rather than only the latest summary.

Example:

```text
E1  Task started
E2  User constraint added
E3  Schema changed
E4  Migration passed
E5  API completed
E6  Test failed
E7  Bug fixed
E8  Test passed
E9  User rejected approach B
E10 Session interrupted
```

The current task state is derived from these events.

Benefits:

- checkpoint history
- state reconstruction
- debugging
- state invalidation
- conflict detection
- recovery from corrupted summaries

---

# 25. Why This Is Different From Memory

### Memory system

```text
"The project uses PostgreSQL."
```

### Wake state

```text
Task:
MongoDB → PostgreSQL migration

Completed:
7/10 steps

Current:
Payment migration

Blocker:
Stripe webhook ObjectId conversion

Verified:
Users + Orders migrations

Decision:
Authentication unchanged

Next:
Fix webhook conversion
```

Memory stores knowledge.

Wake stores **task execution state**.

---

# 26. Why This Is Different From `.md` Files

A static file can describe:

```text
Project rules
Architecture
Plans
Requirements
```

Wake additionally tracks:

```text
What happened during this execution?

What actually passed?

What failed?

What was changed?

What was rejected?

What is currently blocked?

What evidence supports completion?

What is still safe to continue?
```

Wake may eventually use files such as `AGENTS.md`, `spec.md`, or plan documents as inputs during reconciliation, but it does not attempt to replace them.

---

# 27. Why This Is Different From Logs

Logs answer:

> “What happened?”

Wake answers:

> “Given what happened, what is the current recoverable state of this task?”

This distinction is central.

---

# 28. Why This Is Different From Existing Agent Memory

The important unit is not the **fact**.

The important unit is the **state transition**.

Example:

```text
Fact:
Stripe is used.

State:
Stripe checkout is complete,
webhook integration is blocked,
refund flow has not started,
and the developer rejected approach B.
```

---

# 29. MVP Technical Goals

The MVP should prove four technical capabilities.

### 1. Capture

Observe enough activity to construct meaningful state.

### 2. Checkpoint

Persist that state independently from the conversation.

### 3. Reconcile

Compare checkpoint state with current repository state.

### 4. Recover

Provide the new session with an accurate continuation state.

---

# 30. MVP Benchmark

Before product launch, construct controlled tasks that intentionally cross a context/session boundary.

Example:

```text
Task
↓
20–100 meaningful operations
↓
interruption
↓
new session
↓
resume
```

Compare:

**Baseline agent**

versus

**Agent + Wake**

Measure:

- state retention
- repeated work
- incorrect continuation
- recovery time
- developer intervention
- token usage
- completion rate

---

# 31. Primary Success Metrics

## Recovery Accuracy

Percentage of interrupted tasks that resume from the correct state.

Target:

**≥90% on controlled benchmark tasks**

## Critical-State Accuracy

Percentage of critical requirements, decisions, blockers and milestones reconstructed correctly.

Target:

**≥95%**

## Repeated Work Reduction

Target:

**≥50% reduction**

## Developer Intervention Reduction

Target:

**≥60% reduction**

## Recovery Time

Target:

**<60 seconds for normal tasks**

---

# 32. Go / No-Go Criteria

Do not proceed to a full startup build unless the benchmark shows a meaningful advantage.

### GO

Proceed when:

- existing agents exhibit reproducible recovery failures
- Wake materially improves recovery accuracy
- repeated work decreases significantly
- users report meaningful time savings
- one integration is technically practical

### NO-GO

Stop or redesign if:

- current agents already recover reliably
- Wake cannot improve outcomes
- checkpoint state becomes less reliable than native mechanisms
- integration requires invasive changes
- users do not perceive meaningful value

---

# 33. MVP Roadmap

## Phase 0 — Research Benchmark

No polished product.

Build controlled experiments.

Goal:

> Prove the failure mode and quantify it.

---

## Phase 1 — Local State Engine

Build:

- event model
- state engine
- checkpoint store
- CLI status
- checkpoint history

---

## Phase 2 — Reconciliation

Build:

- git comparison
- file comparison
- validation evidence
- SAFE / STALE / CONFLICT states
- state invalidation

---

## Phase 3 — Recovery

Build:

- recovery packet
- session resume
- recovery injection
- manual resume workflow

---

## Phase 4 — First Agent Integration

Integrate with one real coding-agent environment.

Goal:

> Developer continues using the existing agent with minimal workflow change.

---

# 34. Explicitly Deferred

Do not implement initially:

- multi-agent orchestration
- model routing
- automatic token optimization
- generic agent memory
- general-purpose evaluation
- AI code review
- browser automation
- enterprise governance
- cloud collaboration
- team dashboards
- automatic bug fixing

These can be reconsidered only after the core recovery problem is validated.

---

# 35. Future Direction

If the core recovery layer proves valuable, Wake could eventually evolve toward:

```text
Task State
     ↓
Recovery
     ↓
Handoff
     ↓
Multi-agent continuity
     ↓
Long-running agent infrastructure
```

But these are future extensions.

The company should initially own one problem.

---

# 36. Final Product Statement

> **Wake is a plug-and-play recovery layer for AI coding agents that maintains evidence-backed execution state and reconciles it with the current repository so interrupted or context-reset tasks can resume safely without forcing developers to reconstruct the work manually.**

---

# 37. Core Hypothesis

The entire product depends on one hypothesis:

> **When long-running AI coding tasks cross context or session boundaries, existing memory, instruction, planning, and resume mechanisms can still leave the next session without a reliable representation of the task's actual execution state. A dedicated state-reconciliation and recovery layer can reduce incorrect continuation, repeated work, and developer intervention.**

That hypothesis must be experimentally validated before Wake expands beyond this single problem.

---

# 38. v1.1 Amendments — Discovered During Live Pressure Testing

The following architectural requirements were discovered during adversarial human pressure testing of the v1.0-beta engine.

## 38.1 Pre-Checkpoint Guard

**Discovery:** An AI agent can blindly run `wake checkpoint` and accidentally lock unreviewed human modifications into the baseline memory, defeating the purpose of the safety net.

**Requirement:** Before any checkpoint is saved, the engine must scan the working tree for uncommitted changes that were not authored by the AI. If detected, the checkpoint must be blocked and a fatal error returned.

## 38.2 Git-less File Hashing Engine

**Discovery:** Requiring Git as a dependency excludes a large percentage of potential users (beginners, data scientists, non-technical founders).

**Requirement:** Wake must support a fallback mode that uses SHA-256 file hashing to detect human modifications on the local file system without requiring Git to be installed.

## 38.3 Author Attribution Markers

**Discovery:** When Wake flags a file as modified, it cannot distinguish whether the Human or the AI made the change, leading to false blame attribution.

**Requirement:** The Wake IDE extension / MCP server must tag each file save with an attribution marker (`HUMAN_MODIFIED` or `AI_MODIFIED`) so the reconciliation report can explicitly identify who touched what.

## 38.4 Continuous Recovery Stashing

**Discovery:** If a human edits a file and the AI subsequently overwrites it with `write_to_file` before a checkpoint is taken, the human's code is permanently destroyed with no recovery path.

**Requirement:** The Wake IDE extension must continuously monitor file saves. If it detects that an AI tool overwrote a file containing uncommitted human modifications, it must automatically stash the human's version into `.wake/recovery_stash/` before the overwrite completes.

## 38.5 Write-Write Conflict Detection

**Discovery:** If a human and an AI edit the same file at the same millisecond, the AI's `write_to_file` tool will silently destroy the human's work without any warning.

**Requirement:** AI tool integrations should implement optimistic concurrency control. Before overwriting a file, the tool must check the file's last-modified timestamp against its expected value. If the timestamp has changed, the write must abort.

---
