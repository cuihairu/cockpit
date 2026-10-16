# P2 Design: Workflow and Asynchronous Job

> Language: [中文](./workflow-design.md) ｜ **English**

> Initiated 2026-10-09; for implementation progress see the "Implementation Status" section of this document (M1 asynchronous Job + linear
> Workflow landed the same day, M2a variable passing / M2b multi-target fan-out / M2c cancel signal followed).
> The P1 execution Job model ([jobs-design](./jobs-design)) has already landed the minimal `agent.exec` closed loop;
> this design is its next growth point: Job asynchronization + linear
> Workflow orchestration. The design boundary follows the strategic review ruling: a Workflow is "orchestration of a set of
> Jobs", doing control-plane wiring only, not rebuilding the underlying scheduler.

## Why Now

The synchronous RPC model of P1 (POST waits for the terminal state) has two ceilings:

1. **At most 300s**: the `timeout_s` cap of `agent.exec` is 300 and a browser HTTP request
   cannot wait longer; past that the HTTP layer drops first, and the ledger may even show
   the dual-head anomaly of "HTTP dropped but the Job landed a terminal state".
2. **Nowhere to attach orchestration**: multi-step actions like shutdown → backup → upgrade
   → start → health check can only be clicked through by hand step by step, or each written
   as its own script calling the API. There is no home for post-failure handling of a step
   (stop or continue, how many retries).

Asynchronization is not a standalone feature — it is a prerequisite for Workflow: an orchestrator
necessarily advances steps in the background and cannot wait synchronously. Designing both
together extends the state machine all at once.

## Implementation Status

- **M1 implemented (2026-10-09)**: asynchronous Job (single-record semantics W1/W2/W3) + linear Workflow
  (W4-W9) + cancellation + ledger filtering + permissions/audit, across the whole chain
  `internal/storage/workflow.go`, `internal/server/api_workflow.go`, `web/src/pages/Workflows/`,
  `web/src/services/workflows.ts`; making POST /api/jobs fully asynchronous (W1) is a breaking
  semantic change (the original synchronous wait is removed, the only consumer is web).
  Real-machine acceptance proceeded per the Workflow section of the "Acceptance Checklist"
  (four probe scenarios: success chain / stop on failure / retry / cancel).
- **M2a implemented (2026-10-10)**: step variable passing (the first item of the M2 candidate
  list); design see the "M2 Initiation: Step Variable Passing" section, initiated at 824eb88,
  landed at 97696ad (including an incidental fix: dispatchJob now passes the resolved parameters,
  see V3).
- **M2b implemented (2026-10-10)**: multi-target fan-out (the second M2 candidate); design see
  the "M2 Initiation: Multi-Target Fan-Out" section — sequential fan-out on the server side,
  zero agent changes; the web step editor multi-select plus the run detail "N machines" aggregate
  with expandable per-machine detail landed in the same batch; the real-machine acceptance probe
  was extended to W10 (two-machine fan-out success / stop on failure / ghost mixing, 12/12 PASS).
  The remaining candidates were unscheduled at that time (the cancel signal landed on 10-11 as
  M2c, see the next bullet).
- **M2c implemented (2026-10-11)**: cancel signal (the third M2 candidate); design see the
  "M2 Initiation: Cancel Signal" section — the `job.cancel` RPC really kills in-flight processes
  (C1-C10, including DB-only conditional cancellation in the pending window and a fix to the
  fan-out aggregation semantics); the cancel entry on the running/pending rows of the web Jobs
  page landed in the same batch. The real-machine acceptance probes W5/W9 were redefined to the
  kill semantics and W11 (variable passing, the end-to-end face of M2a) was added; the local
  dual-instance harness passes 13/13. DAG stays deferred per W4, the approval gate is not started
  because the orphan-run problem is unsolved, and SSE was decided against.
- **M2d implemented (2026-10-11)**: approval gate (the fourth M2 candidate); design see the
  "M2 Initiation: Approval Gate" section — the prior blocker (orphan runs after a server restart)
  was cleared by the 9079b52 restart sweep; this batch puts the waiting state in the database
  instead of a goroutine (D1), and a high-risk submission returns 202 into approval and is
  dispatched only after approval. The `approvals:read/write` permission points were missed at
  registration time at that moment (the route already mounted the `approvals` resource but
  `resourceActions` had no such key), so every role got 403 on `/api/approvals` and the web
  "Approval Gate" menu never rendered — found and fixed by the 2026-10-15 documentation audit
  (including regression tests). The approval ledger page and the "requires approval to execute"
  toggle on the Jobs page landed in the same batch. DAG is last in the remaining M2 sequence,
  SSE decided against.
- **M2e implemented (2026-10-12)**: DAG dependencies (the fifth M2 candidate); design see the
  "M2 Initiation: DAG Dependencies" section — step-level `depends_on` dependency gating (G1-G8):
  array order is topological order, when a dependency fails with continue_on_error the downstream
  is truthfully marked skipped (a new step-level state), and no parallel execution is introduced
  (the same write-path decision as F2); the web dependency multi-select editor, the grey "Skipped"
  marker on the timeline, and the stop-reason column.
- The remaining M2+ items are in "Boundaries (Explicitly Not Doing)"; unimplemented items follow
  the design in this document.
- The implemented parts of P1 are unaffected: the `agent.exec` type, the job provider and the
  permission points are kept as-is; this design only changes "when the server side returns" and
  "who waits".

## Architecture Overview

```
┌─ server ─────────────────────────────────────────────────────────────────────────┐
│ POST /api/jobs                    POST /api/workflows/{id}/run                   │
│    create Job(pending) ──┐        create Run + stepwise Job                      │
│    return 201 at once  │              │                                          │
│                        ▼              ▼                                          │
│           dispatcher goroutine    orchestrator goroutine                         │
│           CallAgent(job.exec)     per step: create Job → await terminal state    │
│           write back final state  failure: continue_on_error sets stop/continue  │
│                                   run lands in a terminal state                  │
│ GET /api/jobs?status=&run=        GET /api/workflows/{id}/runs                   │
└──────────────────────────────────────────────────────────────────────────────────┘
```

Key point: **zero changes on the agent side**. The `job.exec` RPC already is "synchronous
execution with a timeout, returning exit_code/output"; both the orchestrator and the single-step
dispatcher are just server-side goroutines waiting on the same RPC. Cancellation capability is
constrained by the RPC having no cancel frame (D3), stated honestly.

## Key Decisions

| # | Decision | Choice | Rationale |
|---|------|------|------|
| W1 | POST /api/jobs semantics | **Fully asynchronous**: return 201 with a pending Job view immediately after creation, dispatch in the background; the original synchronous wait is removed | The only consumer is web (mobile never wired the jobs API), so there is no compatibility burden; "return the id on creation" is the essence of the ledger model, P1's synchronous return was an expedient |
| W2 | State machine | `pending → running → success \| failed \| cancelled`; **no separate state for timeout** (stay `failed` + error=`timed out after Ns`) | The error field already determines a timeout, a separate state only widens the filtering surface; cancelled is a real capability that only exists after asynchronization and must be independent (a Job cancelled while running should not count as failed) |
| W3 | Cancel semantics | Only **pending** (undispatched) Jobs may be cancelled; running is rejected (409, explained by the 300s cap of job.exec ending naturally). Cancelling a Workflow run = stop advancing subsequent steps + the current step under the same rule | The RPC protocol has no cancel frame, pretending to cancel a running one is lying; the honest boundary is "revocable before dispatch, after dispatch wait for it to come back". Signal-injection cancellation on the agent side is left to M2 |
| W4 | Orchestration shape | **Linear step chain** (an ordered array of steps executed one by one), not a DAG; step-level `continue_on_error` and `retry` (0-3 times, fixed 5s interval) | The most frequent thing in personal infrastructure is exactly a serial chain (shutdown→backup→upgrade→start→check); expressing DAG edges costs double in UI and storage, so extend only when real parallelism appears. Retry covers the most common resilience need |
| W5 | Relationship between steps and Jobs | Each step creates a **real Job** (a nullable column `workflow_run_id` added to the `jobs` table); every step in the run ledger can be opened to see output/exit code | A step is not a shadow record — it is an execution, entering the unified ledger and the existing `job_run` audit; no parallel "step log" system is invented |
| W6 | Storage | Two tables: `workflows` (definitions) + `workflow_runs` (instances); the run embeds a JSON snapshot of steps at runtime (with per-step status/attempt count/related Job id); AutoMigrate | At personal scale reads dominate writes, a steps subtable is over-engineering; the snapshot is the fact (how it ran, and each step's result) |
| W7 | run state machine | `running → success \| failed \| cancelled` (no pending — creating a run starts advancing immediately); partial failure = failed, details records the break point | A run is a process entity, it runs once created; a run "queued" has no consumer |
| W8 | Concurrency constraint | At most one active run per workflow at a time (rejected with 409); different workflows may run in parallel | At personal scale this prevents re-entrance (double-clicking "Run"); no global rate limiting |
| W9 | Permissions | `workflows:read` / `workflows:write`; run/cancel belong to write; step Jobs reuse the existing `jobs:*` surface | Same convention as cron/Job: the ledger is readable (read), execution is high-risk (write) |
| W10 | Audit | Definition operations `workflow_create/update/delete`; one `workflow_run` on run completion (resourceID=run id, details containing the workflow name/steps summary/break point); step Jobs carry `job_run` themselves | A definition is a change, a run is an execution, the two are separate; steps already have audit and are not doubled |
| W11 | Trigger | Manual `POST /api/workflows/{id}/run`. "Run a Workflow on a schedule" = a cron entry invoking that API (an existing convention in the P1 boundary section) | cron is the agent-side scheduling surface while Job/Workflow is the control-plane execution ledger; the two planes are not coupled; a server-side timer is another project |
| W12 | Step parameters | `{"name","type","target","parameters","timeout_s"?,"continue_on_error"?,"retry"?}`; the type whitelist reuses `jobTypes` (M1 only `agent.exec`) | Isomorphic to the Job payload, validation reuses `validateJobPayload`; a new type (container operations, backup triggering) becomes usable for orchestration automatically once registered in the whitelist |

## REST API

| Method | Path | Description |
|------|------|------|
| `GET /api/workflows` | — | Definition list |
| `POST /api/workflows` | `{name, description?, steps[]}` | Create a definition (≤20 steps) |
| `GET /api/workflows/{id}` | — | Definition detail |
| `PUT /api/workflows/{id}` | Same as create | Update (409 when an active run exists) |
| `DELETE /api/workflows/{id}` | — | Delete the definition (historical runs are kept) |
| `POST /api/workflows/{id}/run` | — | Create a run and advance it in the background, 201 returns the run view |
| `GET /api/workflows/{id}/runs` | `?status=` | Run ledger for this definition |
| `GET /api/workflow-runs/{rid}` | — | Run detail (including the steps snapshot) |
| `POST /api/workflow-runs/{rid}/cancel` | — | Cancel the run (W3 semantics) |
| `GET /api/jobs` | `?status=&target=&type=&workflow_run_id=` | P1 ledger plus filtering (used by the run detail page to list steps) |
| `POST /api/jobs/{id}/cancel` | — | Cancel a pending Job (W3) |
| `POST /api/jobs` (M2d) | `{..., approval:"required"}` | High-risk submission: 202 returns an approval view, no Job is created; after approval it is dispatched as-is |
| `GET /api/approvals` (M2d) | `?status=` | Approval ledger (lazy expiry sweep) |
| `GET /api/approvals/{id}` (M2d) | — | Single-record detail (payload parsed into parameters) |
| `POST /api/approvals/{id}/approve` (M2d) | — | Approve and dispatch (duplicate/expired 409; agent offline 503) |
| `POST /api/approvals/{id}/reject` (M2d) | — | Reject (terminal, no Job created) |

Validation: name non-empty and ≤64 characters; steps non-empty and ≤20; step.name unique within
the same workflow and ≤64; `timeout_s`/`command` etc. reuse the P1 limits; `retry` 0-3. M2d:
`approval` only accepts `required`; a payload going into approval must first pass
validateJobPayload (after approval it is dispatched as-is, validation cannot be deferred).

## Data Model

```go
type Workflow struct {
    ID          string    `gorm:"primaryKey" json:"id"`
    Name        string    `json:"name"`
    Description string    `json:"description,omitempty"`
    Steps       string    `json:"-"` // JSON string, parsed at the API layer
    CreatedBy   string    `json:"createdBy"`
    CreatedAt   time.Time `json:"createdAt"`
    UpdatedAt   time.Time `json:"updatedAt"`
}

type WorkflowRun struct {
    ID           string     `gorm:"primaryKey" json:"id"`
    WorkflowID   string     `gorm:"index" json:"workflowId"`
    WorkflowName string     `json:"workflowName"` // denormalized, so the ledger stays readable after the definition is deleted
    Status       string     `gorm:"index" json:"status"`
    Actor        string     `json:"actor"`
    Steps        string     `json:"-"` // runtime snapshot JSON (per-step state / attempt / JobID)
    CreatedAt    time.Time  `json:"createdAt"`
    FinishedAt   *time.Time `json:"finishedAt,omitempty"`
}
// storage.Job gains a column: WorkflowRunID string `gorm:"index;default:''"`
```

Step snapshot shape:

```json
[
  {"name":"backup","type":"agent.exec","target":"a1","status":"success",
   "jobId":"j-1","attempts":1},
  {"name":"upgrade","type":"agent.exec","target":"a1","status":"failed",
   "jobId":"j-2","attempts":2,"stopReason":"retries exhausted"}
]
```

## Frontend

- `web/src/pages/Workflows/index.tsx`: definition list (name / step count / latest run status /
  run button) + create/edit drawer (step editor: per-row name/target/command/timeout, a
  continue_on_error switch, a retry dropdown, move up/down ordering);
- run detail: step timeline (a status badge per step + attempts + a popup linking to Job detail),
  5s polling while in progress, polling stops at a terminal state;
- Jobs page: the create dialog switches to async semantics (closing on submit + a toast
  "Submitted, viewable in the ledger"), and the list gains status filtering; a "Cancel" button
  appears on pending rows (`jobs:write`);
- Menu entry "Workflow Orchestration" (`workflows:read`), perm `workflows:write` gates the write
  entry.

## Testing

- storage: two-table CRUD / snapshot read-back / WorkflowRunID linkage
  (`internal/storage/workflow_test.go`);
- server: orchestrator advancement (success chain / stop on a mid-chain failure /
  continue_on_error skip / retry succeeding on the second attempt / run cancel stopping advancement
  / pending Job cancel / running Job cancel 409 / re-entrant same workflow 409 / definition update
  with an active run 409 / validation 400 each) — the agent is injected with a fake provider
  (precedent: `cov_jobs_test`); jobs filter parameters (`cov_jobs_test.go` extended);
- web: the three states of the Workflows page + step editor validation + run timeline polling;
  async semantics and cancellation on the Jobs page;
- Coverage: new branches are closed to the 100% discipline, unreachable defenses are registered
  in the existing format in `tool/known_uncoverable.txt`.

## Milestone Breakdown

- **M1 (this design)**: asynchronous Job + linear Workflow + cancellation (pending semantics) +
  ledger filtering + permissions/audit. Acceptance: probes pass the four scenarios success chain /
  stop on failure / retry / cancel + one orchestration round against a real machine agent.
- **M2 (candidates, each a separate project)**: step variable passing (see the M2a initiation
  below), multi-target fan-out (see the M2b initiation), cancel signal (see the M2c initiation),
  DAG dependencies, approval gate (see the M2d initiation), SSE push. M2a/M2b/M2c/M2d have all
  landed; DAG stays deferred per W4 until real parallelism appears, and SSE was decided against
  because it conflicts with the repo-wide polling pattern.
- **Not doing**: scheduled triggers (the W11 convention), real-time output streaming (the P1
  boundary section leaves it to a unified remote-control design), Workflow nested inside Workflow
  (the recursive execution surface is left alone for now).

## M2 Initiation: Step Variable Passing (M2a, 2026-10-10)

The most real gap in the linear chain: a later step needs the product of an earlier step (feeding
scan output into a processing command, feeding a backup path into a verification command), and
today the only options are writing to disk and reading it back or copy-pasting whole blocks.
Variable passing connects the output between the earlier step's Job and the later step's command
— a purely server-side orchestrator enhancement, zero agent changes, no schema change, the
smallest high-value increment among the M2 candidates.

### Key Decisions

| # | Decision | Choice | Rationale |
|---|------|------|------|
| V1 | Syntax | <span v-pre>`{{steps.NAME.output}}`</span>, where NAME is the step name (`[A-Za-z0-9_-]+`); this is the only form | Definition validation already guarantees name uniqueness; referencing by name rather than by index (the original M2 draft `steps.N`) survives step reordering and gives readable errors. Other fields such as `exit_code` are not implemented for now, add them when needed |
| V2 | Validation timing | At definition save time (`validateWorkflowPayload`): a referenced name must exist in an **earlier** step; unknown name / forward reference returns 400 | Under sequential-chain semantics a forward reference necessarily yields an empty value, rejecting at save time is more honest than failing at runtime; self-reference likewise |
| V3 | Substitution timing | Resolved once before the step starts (before entering the retry loop), applied to the parameter tree after `sanitizeStepParams` (recursive map/slice, string values only; the orchestration meta keys name/retry/continue_on_error do not participate) | Every retry attempt uses the same resolved result, so the semantics are stable; the target field is a resource ID and is not substituted. **Incidentally fixes a leftover M1 defect**: dispatchJob previously passed the raw definition parameters — meta keys leaked to the agent and templates were not resolved (what is persisted is the sanitized version, inconsistent between the two); now dispatch and persistence both use the resolved parameters |
| V4 | Value source | The raw Job.Output of the previous step's final attempt (≤64KB, truncated at the tail for the stored value); when the source step failed (let through by continue_on_error) its already-stored output is used (possibly empty or an error output) — consistent with how W4 combines | 64KB is the existing truncation point on the agent side, no new cap is added; "referencing a failed step" is a natural extension of continue_on_error |
| V5 | Validate after substitution | Run the resolved result through `validateJobPayload`; if it fails the step goes straight to failed (the snapshot records stopReason) and is not dispatched | Variable injection may push the command past the 16KB cap; rejecting locally saves an RPC round compared with sending it to the agent and failing there, and the behavior is deterministic |
| V6 | Literal boundary | Only tokens exactly matching <span v-pre>`{{steps.NAME.output}}`</span> participate in substitution; other <span v-pre>`{{...}}`</span> forms are kept as-is | Braces in shell, JSON templates etc. are unaffected; a reference written with a misspelled name is blocked at definition time by V2 (unknown name) and never silently becomes a literal |

### Invariants

- The definition file (the steps column of the workflows table) stores the **template source**;
  the resolved result only lands in the Parameters of the Job row created by that step (the same
  "the snapshot is the fact" idea as W6).
- The run step snapshot does not store the resolved result again — it is queryable from the Job
  ledger (W5).
- Zero changes on the agent side: what `job.exec` receives is the already-resolved command.

### Frontend

Zero changes. What the step editor edits is the template source, and substitution is transparent
to the UI; the hint text will be added once there is real usage feedback.

### Testing

- server (the `cov_workflow` family): unknown reference / forward reference at definition time
  returns 400; two-step chain resolution (a fake provider asserts the second step's
  Job.Parameters contains the first step's output); nested parameter substitution; forms that
  are not <span v-pre>`{{steps.*.output}}`</span> keep <span v-pre>`{{...}}`</span> as-is; a step
  exceeding 16KB after substitution goes straight to failed with no Job dispatched; referencing a
  failed step (continue_on_error) takes its error output.
- All reachable, no new known_uncoverable registration expected.

## M2 Initiation: Multi-Target Fan-Out (M2b, 2026-10-10)

A single step can only hit one agent, so N homogeneous operations (fleet-wide upgrade / batch
restart / full inspection) can only be done by duplicating the step or running N times raw.
Fan-out lets one step create a real Job for each of N agents — a server-side orchestration
enhancement, zero agent changes, no protocol additions (the same boundary as M2a). Why this one
over cancel signal / DAG / approval gate / SSE: logs M3 has already validated the "server-side
parallel fan-out + degraded attribution" pattern (zero agent changes), and this batch brings it
into change-affecting orchestration; the cancel signal was taken up separately as M2c (C1-C9) —
no structural additions on the RPC protocol side (reusing the rpc_request frame + the router's
dotted method), it requires refactoring the agent read loop into concurrent dispatch, but agent
and server ship from the same repository with no compatibility burden; DAG is explicitly deferred
by W4 until real parallelism appears; the approval gate is not started because the server-restart
orphan-run problem (a waiting-state goroutine cannot be revived) is unsolved; SSE was decided
against because it conflicts with the repo-wide polling pattern.

### Key Decisions

| # | Decision | Choice | Rationale |
|---|------|------|------|
| F1 | Syntax | Optional step-level `targets: []string` (workflow steps only; the single `target` of the standalone Job API is untouched); `target` and `targets` are mutually exclusive (400 if both appear); entries are non-empty after trim, deduplicated, ≤20 machines | A single target is still the vast majority of scenarios, keeping it as the primary form; an array is straightforward; 20 machines = the personal infra scale ceiling (a sanity gate on the same order as WorkflowMaxSteps) |
| F2 | Execution order | **Sequential fan-out**: each machine runs to completion (including its own retry cycle), not in parallel | The orchestrator goroutine's synchronous model is unchanged, with zero concurrency coordination; the change-affecting surface (exec write operations) differs from the logs M3 query surface, and a parallel fan-out amplifier is unacceptable on the write path; N machines worst case N×(retry+1)×300s is bounded |
| F3 | Failure policy | Without continue_on_error: the first machine failing stops the step (later targets do not run); with continue_on_error: all targets run, step status=failure but the chain continues | Isomorphic to the W5 stop/continue failure semantics, the target dimension reuses the same switch, no new concept |
| F4 | Retry | A full independent retry cycle per target; each attempt is still a separate Job row | W4's "attempt history goes into the ledger" is preserved per machine |
| F5 | Cancellation | Cancellation is checked at target boundaries (the same mechanism as the existing step boundaries, reading run.Status); unrun targets are recorded `cancelled` + stopReason=`run cancelled` | W3's "stop advancing subsequent steps" is honored faithfully at a finer granularity; cancelled is an existing real state |
| F6 | Snapshot | The step row gains `targets[]` (agentId/status/jobId/attempts/stopReason, omitempty); the step-level Status = aggregate (success only if all succeed, otherwise failed); the snapshot shape of single-target steps is unchanged (Target/JobID stay at the step level) | Existing run JSON stays forward compatible (omitempty); the aggregate state keeps the timeline self-consistent without changes; the run detail expands into per-target detail |
| F7 | Output | A fan-out step **does not provide** step output: <span v-pre>`{{steps.NAME.output}}`</span> resolves to an empty string | Concatenating multi-machine output is false precision; per-target output references can be a separate project once the real need appears; the empty string carries the same semantics as M2a's "missing name yields an empty string" |
| F8 | Online validation | Before creating a run, validate every target is online one by one (same convention as the existing ghost defense); if any is missing return 503 and name the absentee of steps[i] | Same as the single-target `registry.Get`, fan-out is just N of those |

### Invariants

- The definition file stores the `targets` source; every machine's attempt lands as a separate Job
  row (the snapshot is the fact, W6).
- The single-target path (the `target` field) behaves byte for byte identically — existing runs
  and probe scenarios notice nothing.
- Zero agent changes: each machine still receives its own `job.exec`.

### Frontend

- Step editor: the target host Select gains a multi-select toggle (choosing several machines
  means the fan-out form, submitted collapsed into `targets`); single selection still submits
  `target`.
- Run detail timeline: fan-out steps show the aggregate in the in-row "Targets" column (e.g.
  `3 machines`), and the expanded row lists per-target detail (agentId/status/attempts/output
  entry).

### Testing

- server (the `cov_workflow` family): the targets validation matrix (mutual exclusion / empty /
  duplicate / over 20 / empty entry); two-machine sequential fan-out succeeding and aggregating to
  success; the first machine failing stops the step (no Job for the second); continue_on_error
  runs everything with the step failed and the chain continuing; per-target retry (the first
  machine succeeds on retry); cancellation at a target boundary (the second machine is recorded
  cancelled); online validation before creating a run returning 503 when one is missing; output
  references in a fan-out step resolving to an empty string; snapshot shape regression for
  single-target steps (no targets key).
- web: the editor switching back and forth between both forms (target/targets), expanded
  rendering of fan-out steps on the timeline.

## M2 Initiation: Cancel Signal (M2c, 2026-10-10)

The W3 cancellation only stops advancement: after the run is set to cancelled the orchestrator
stops at the next step boundary, and **the Jobs of in-flight steps are not touched** — a runaway
command has to run out the agent-side timeout (cap 300s) before ending naturally. M2c makes
cancellation a real signal: the server issues a `job.cancel` RPC and the agent kills the in-flight
process (SIGKILL to the process group, grandchildren die too), and the Job lands in the
`cancelled` terminal state. Why this one over DAG / approval gate / SSE (decided at the
2026-10-10 inspection): DAG stays deferred per W4 (waiting for real parallelism); the approval
gate is not started because the server-restart orphan-run problem (a waiting-state goroutine
cannot be revived) is unsolved; SSE was decided against because it conflicts with the repo-wide
polling pattern. The cancel signal is the only "user-perceivable stop-the-bleeding for runaways"
increment among the M2 candidates.

### Key Decisions

| # | Decision | Choice | Rationale |
|---|------|------|------|
| C1 | Signal carrier | The `job.cancel` RPC method (params `{id}`), reusing the existing `rpc_request` frame, **no new message type** | The probe_report (B5) precedent is an agent→server proactive report that adds a message type only when no existing channel exists; cancellation is a server→agent command, `rpc_request` is the standard carrier for that direction, and the router's `parseMethod` already supports dotted methods (`job.cancel` → provider=job, action=cancel), with zero structural protocol additions |
| C2 | Concurrent agent read loop | `handleRPCRequest` switches to goroutine dispatch (`go a.handleRPCRequest(msg)`) | **Fundamental premise**: `messageLoop` is a single serial goroutine, and while `job.exec` blocks (up to 300s) no subsequent frame — including cancel — can be read. RPC responses are correlated by message ID (the pending response maps do not interfere), `sendMessage`→`upstream.Enqueue` is already goroutine safe (writeLoop writes serially), and concurrent dispatch does not break the existing contract |
| C3 | Agent handle table | `JobProvider` gains a `map[jobID]*jobSession` (ctx cancel + mutex), mirroring the logs-follow `followState.follows` pattern | logs_follow has already validated "register handle + look up by ID + idempotent close"; the exec ctx cancel triggers the existing `cmd.Cancel` of `newJobCmd` (process-group SIGKILL), **reusing the same kill path as timeout**, without introducing a second kill mechanism |
| C4 | Kill semantics | Process-group SIGKILL on unix (`Kill(-pid)`); on Windows the direct child process | `newJobCmd` already implements this (Setpgid + group kill), the same as the timeout path; grandchildren spawned by `sh -c` die too, leaving no orphans. Windows has no process-group semantics, so the existing direct-child kill is kept |
| C5 | Idempotent semantics | A cancel for an unknown/finished ID returns success as a no-op (`{cancelled:false}`), mirroring `FollowStop` | There is an inherent race between cancel and exec completion (the job ends just before cancel arrives); returning an error would leave the server unable to distinguish "just stopped" from "ended long ago", and idempotent success keeps the server-side semantics single |
| C6 | Server-side cancel of a single Job | running is revocable: `CallAgent(job.cancel)` → on success a **conditional UPDATE** (`WHERE status='running'`) lands cancelled; if the agent is unreachable → 409, rejected truthfully. pending stays as in W3 (DB-only), any terminal state is always 409 | "Observe the terminal state ⟹ the state is trustworthy": cancelled only lands after the agent confirms the kill, never falsely claiming it stopped; the conditional UPDATE guards against a check-then-update race (the pending→running flip window between GetJob and CancelJob) |
| C7 | Terminal write-back must not overwrite cancellation | `dispatchJobAsync` / `runWorkflowStep` **re-read** the job before writing back; if it is already cancelled, cancelled is preserved (only output/exitCode/error are filled in) | The cancel write and the exec write-back are concurrent: after being killed exec returns exit -1 / "signal: killed", and without this guard cancelled would be overwritten with failed; re-reading converges the race without adding locks for serialization |
| C8 | Run-level cancel linkage | Besides setting the run cancelled, `handleWorkflowRunCancel` looks up in-flight step Jobs (`ListJobsFiltered` querying running and pending in one pass each) and walks each through the C6 kill path; pending (the microsecond window between row creation and dispatch) is cancelled conditionally in the DB only, and 0 rows matched means it has already flipped to running so it falls back to the kill path; in-flight step write-back hits the C7 guard, and the step snapshot records cancelled + stopReason=`run cancelled` (same convention as F5) | W3 only stops advancement and a runaway command runs out the timeout — the value of M2c is exactly "really stopping in-flight commands"; run cancellation is the user's most frequent "stop" entry point; if the pending window is not cancelled the ledger permanently keeps an orphan row with no terminal state |
| C9 | Step cancelled ⟹ run cancelled | The single-step branch of `advanceWorkflow` gains a cancelled guard (mirroring the return of the fan-out branch F5); if the run is still running (the single-Job cancel scenario) then `finishWorkflowRun(cancelled)` | If a step Job dies the chain breaks and the run must not hang as a zombie (never reaching a terminal state); propagating a single-Job cancel to the run is intuitive semantics — the user killed a step and expects the whole chain to stop |
| C10 | Conditional write of the run terminal state | `finishWorkflowRun` and the run cancel handler share the `FinishWorkflowRun(id, status, WHERE status='running')` conditional UPDATE (a new storage method, same convention as C6's Job CancelJob); 0 rows matched → the cancel handler returns 409 and the orchestrator gives up the rewrite | The trump card introduced by C8: when the kill loses to exec finishing up, the step ends as failed, and `finishWorkflowRun(failed)` would flip an already-landed cancelled terminal state; once both sides use the conditional write the first one wins and the later one reports truthfully (409 / giving up in the log), with cancelled and failed/success never overwriting each other. The original full `UpdateWorkflowRun` Save is kept for non-terminal surfaces such as snapshot write-back (recordStepResult / recordStepTargets) |

### Invariants

- The agent handle table is held only for the lifetime of an exec (register → execute →
  unregister); cancel does not build a new state machine, a ctx cancel is all there is. The
  unregister order is **cancel-then-unregister** (LIFO defer), preventing a miss in the
  "unregistered but not killed" microsecond window.
- `cancelled` is an existing real terminal state of a Job (already established in storage W3);
  the race between the cancel write and the exec write-back is converged by the conditional
  UPDATE + re-read, and the terminal state is whatever is in the database.
- Run terminal state: terminal writes always go through the conditional UPDATE (C10), cancelled
  and failed/success are first-come-first-served and never overwrite each other — the
  advanceWorkflow step boundary check + the C9 cancelled guard are the double insurance on the
  flow side, and C10 is the backstop on the write side; the run cancel handler itself is
  audit-then-update (same as the write-order invariant of finishWorkflowRun: observe the terminal
  state ⟹ no subsequent writes).
- The audit actions are unchanged (`job_cancel` / `workflow_cancel` exist already); step Jobs
  killed through run cancel linkage do not emit a separate cancel audit (their exec write-back
  already has a `job_run` audit).

### Frontend

- Cancel button on the Jobs page: the condition widens from only `pending` to `pending || running`
  (the original code rendered the button only for pending and running rows had no entry — this is
  not a pure behavior upgrade, the render condition must change); the button shape and text are
  unchanged.
- The run cancel wording changes stance: from "the current in-flight step will end naturally"
  (W3 semantics) to "the in-flight step will be terminated immediately" (M2c semantics), updated
  in both the Popconfirm and the success toast.
- Run timeline: after an in-flight step is killed it shows cancelled + stopReason
  `run cancelled` (reusing the F5 render path, zero new components).

### Testing

- agent (the `rpc` package): exec registers a handle → cancel kills the in-flight command (a real
  `sleep` + asserting the early return); an unknown ID is idempotently successful; cancel after
  exec ends naturally is idempotent; concurrent exec+cancel (`-race`); `job.exec` without an id
  parameter (the old call form) does not register and cancel is a no-op.
- server (the `cov_jobs` / `cov_workflow` families): running job cancel → a fake agent receives
  `job.cancel` → the job becomes cancelled + the output is still written back; an unreachable
  agent → 409 and no cancelled is landed; the exec write-back does not overwrite cancelled (the
  C7 guard); the pending→running flip race goes through the conditional UPDATE; run cancel linkage
  kills the in-flight step (C8, including the DB-only conditional cancellation for the pending
  window + the flip fallback); a single-step Job killed → the run lands cancelled (C9); a fan-out
  step's target Job killed → the step becomes cancelled + later targets are not dispatched + the
  run terminal state stays cancelled (the aggregate cancelled and the C10 conditional write form
  a double line of defense); cancelling after the run is already terminal returns 409 (0 rows
  from the C10 conditional write); a cancel from a stale running view is rejected (C6's "job
  already finished"); id injection when dispatch has no params (the C1 contract).
- storage: the `CancelJob` conditional UPDATE (only effective on a state match + re-cancel yields
  0 rows + `MarkJobCancelledOutput` only fills in the execution result); the `FinishWorkflowRun`
  conditional terminal write (only effective for running, 0 rows for an already terminal state and
  no rewrite).
- Dead defenses and login-state branches are registered in `tool/known_uncoverable.txt` per
  convention (9 entries for jobs / 17 for workflow, each annotated with its own branch reason; the
  registration convention for entries reachable only in a race window is the same as the C10
  0-rows 409 entry).

## M2 Initiation: Approval Gate (M2d, 2026-10-11)

High-risk `agent.exec` currently executes on submission: once a command gets past `jobs:write` it
reaches the target machine directly. M2d adds an explicit second confirmation — at submission you
can declare `approval="required"`, and the request lands in an approval record and returns 202
immediately (no Job created); the approver sees "target + command summary + submitter + expiry" in
the ledger, and only after approval is it dispatched with the original payload. Why this one as
the M2 closing item (decided 2026-10-10: "resolve the server-restart orphan run first, then start
the project"): the prior blocker has been cleared by the 9079b52 restart sweep; also, if the
approval waiting state reused W1's goroutine semantics, a server restart would lose the pending
approvals — this batch puts the waiting state in the database instead of a goroutine, so a restart
loses nothing (D1).

### Key Decisions

| # | Decision | Choice | Rationale |
|---|------|------|------|
| D1 | Where the waiting state lives | The SQLite `approvals` table, not a goroutine | The blocker for starting the project back in the M2b era was exactly "a waiting-state goroutine cannot be revived"; once stored in the database a server restart loses no pending approval, and the ledger is naturally queryable |
| D2 | Submission entry and permissions | `POST /api/jobs` gains an `approval` field; submission still goes through `jobs:write`, and the ledger/decisions go through `approvals:read/write` (that permission point was only backfilled on 2026-10-15, before that every role got 403 on the endpoint) | The gate shares the surface of the execution right — the value of approval lies in "explicit second confirmation + split audit accounts", not in privilege escalation; a separate resource point governs the decision action |
| D3 | Validation timing | validateJobPayload runs upfront at submission (a bad payload gets 400 and never enters approval; a targets fan-out is rejected too) | After approval the original payload is dispatched as-is; if validation were deferred, what got approved might be an unexecutable request |
| D4 | Concurrency and expiry closure | `DecideApproval` conditional UPDATE `WHERE status='pending' AND (expires_at IS NULL OR expires_at > ?)` returns the number of rows matched; 0 rows → 409 | Duplicate approval / reject after approval / approval after expiry all close within a single UPDATE (same convention as C10); TTL expiry does not depend on a sweep task, and the lazy sweep only changes ledger display |
| D5 | Agent offline at approval time | The approved terminal state has already landed and the dispatch honestly returns 503 (never falsely claiming it was dispatched); the retry surface = resubmit | Approval cannot be rolled back and must not pretend to succeed; offline is not blocked at submission time — the agent can come back online within the waiting window |
| D6 | Corrupt payload | Audit records `dispatch=payload corrupt` + 500 | Unreachable in practice (D3's upfront validation + JSON decode products are always serializable), pure defense; the defensive stance is still "never falsely claim it was dispatched" |
| D7 | Split audit accounts | The three actions submit (submitter) / approve (approver) / reject (approver) each record their own entry | The ledger cross-links (best-effort write-back of approval.job_id) + the audit splits "who submitted, who let it through" |

Invariants:

- No Job row exists while an approval is pending — the database-level embodiment of splitting
  "submission" from "execution" (a row is created only on approval).
- `approved`/`rejected`/`expired` are all terminal, and decisions only start from `pending`; an
  expired undecided item can only be resubmitted.
- A pending approval row does not depend on in-process state and is preserved as-is across a
  server restart.
- Dispatch reuses submitJob (the same validation / same 503 semantics as direct creation), and the
  actor records the submitter.

### Frontend

- `web/src/pages/Approvals/index.tsx`: approval ledger page (5s polling + status filtering + a
  second-confirmation Popconfirm for approve/reject, with the command summary in the approval
  popover); terminal rows show a copyable write-back jobId; menu entry "Approval Gate"
  (`approvals:read`), decision buttons `approvals:write` (the permission point is registered
  jointly by `internal/storage/role.go` `resourceActions` and the fixed sequence of
  `allPermissions`; missing one makes the convention diverge);
- The create dialog on the Jobs page gains a "requires approval to execute" toggle: it goes
  through `createJobWithApproval` and the toast says "Submitted for approval, execution starts
  after approval".

### Testing

- storage: CRUD / conditional terminal write (matched once, 0 rows when repeated, 0 rows when
  expired, always decidable without an expiry), `SetApprovalJobID` write-back, batch expiry only
  touching expired pending rows, the closed-database error surface
  (`internal/storage/approval_test.go`);
- server: the full chain (202 with no Job row → approve 200 → the Job runs to a terminal state →
  two audit entries), reject creating no Job, a duplicate decision returning 409, a negative TTL
  used to construct expiry, route guards (empty id / 405 / unknown id), closed database 500,
  corrupt payload 500, approval with an offline agent returning 503, truncation of long command
  summaries, RBAC permission points (`internal/server/api_approvals_test.go`);
- web: ledger rendering / approve / reject / failure toast / status filtering / hiding actions
  without write permission on the Approvals page, and the switch routing on the Jobs page
  (`web/src/pages/Approvals/index.test.tsx` extended to the Jobs page);
- Coverage: new branches are closed to the 100% discipline, unreachable defenses are registered
  in the existing format in `tool/known_uncoverable.txt` (5 entries for approvals; the jobs line
  numbers shift with the extraction of submitJob).
## M2 Initiation: DAG Dependencies (M2e, 2026-10-11)

W4 ruled that "a linear step chain is not a DAG, extend it only when a real parallelism need appears"; the 2026-10-11
patrol decided to put DAG into the remaining M2 sequence (cancel signal → approval gate → DAG), and this batch
lands it. The increment is not parallel execution (F2 already fixed ordered fan-out, and the write path introduces
no concurrency coordination) but **dependency gating**: a step can declare `depends_on`, and when a dependency fails
the downstream step is honestly skipped — expressing the conditional-branch semantics of "C needs A's success but
does not need B".

### Key Decisions

| # | Decision | Choice | Rationale |
|---|------|------|------|
| G1 | Syntax | Step-level `depends_on: []string` (a top-level step field, peer to `targets`; not inside parameters) | Same shape as M2b `targets`; a structural field rather than template text, so it can be validated at save time |
| G2 | Reference direction | A dependency may point **only to an earlier step** (by definition-array order, and the name must already be declared and unique) | Array order is the topological order, so the orchestrator's loop order needs zero changes and no topological sort is needed at run time; forward, self and unknown references are all 400 at save time (the same wording as M2a V2, "unknown or forward step reference"); cycles are impossible by construction, so no run-time cycle detection is needed |
| G3 | Execution order | Keep advancing cell by cell in array order (G2 guarantees this is a legal topological order); parallel branches are not done | The orchestrator's single-threaded state machine shares a decision surface with F2's ordered fan-out; parallelism waits until a real need appears |
| G4 | Skip semantics | If any step's dependency ends in a non-success state (failed or skipped) → that step is **skipped**: snapshot status=`skipped` (a new step-level state, not part of the JobStatus vocabulary), stopReason=`skipped: dependency <name> <failed\|skipped>`, attempts=0, no Job row; the skip propagates (a skipped step that others depend on → skipped again) | skipped honestly states "it did not run", distinct from pending (not its turn yet); empty output shares the semantics with M2a's "a missing name yields an empty string" |
| G5 | Interaction with the failure policy | A dependency failure without continue_on_error still stops the run (W5 unchanged, downstream never advances); with continue_on_error the run continues and the steps depending on it are skipped per G4 | W5's "continue the chain, not an exemption from failure" — a dependency declaration is an explicit input assumption, and once the assumption breaks the downstream does not run |
| G6 | Run terminal state | Semantics unchanged: a failed step with continue_on_error does not fail the run (existing W5), and a skipped step likewise does not fail the run; run=success while the timeline contains skipped steps | Isomorphic to the existing "run success + a red failed step in the timeline", zero new concepts |
| G7 | Cancellation | Unchanged: cancellation stops at existing step boundaries; a skipped step never executes, so there is nothing in flight to cancel | skipped has no Job, treated the same as pending steps |
| G8 | Interaction with fan-out | Gating happens at step level: a fan-out step whose dependencies are unmet skips the whole step; if they are met it proceeds per host as usual (target-boundary cancellation and snapshots unchanged) | depends_on and targets are orthogonal; each dimension holds on its own |

### Invariants

- The definition file stores the `depends_on` source text (omitempty; a step with no dependencies has no such key); array order is the execution order and is a legal topological order (G2).
- The snapshot gains a `skipped` step-level state; the existing JobStatus vocabulary (pending/running/success/failed/cancelled) does not gain a word.
- Existing behaviour for single-target steps and fan-out steps stays byte-identical — a run with no `depends_on` perceives nothing.
- Zero agent changes, zero protocol additions (the same boundary as M2a/M2b).

### Frontend

- Step editor: a "dependent steps" multi-select (listing only names before the current step; the control is not rendered when the list is empty), folded into `depends_on` on submit; a submission with no dependencies carries no key.
- Run timeline: a grey `skipped` label ("skipped"), with stopReason shown in the stop-reason column.

### Testing

- server (`cov_workflow` family): the validation matrix (unknown / forward / self-reference / duplicate / empty entry / trim); dependency gating success; a dependency failure without COE stopping the run (no downstream Job); a dependency failure with COE skipping downstream + run success + empty-string output; transitive skipping; gating on a fan-out step; the cancellation boundary; snapshot shape (attempts 0, no jobId, status skipped).
- web: the editor multi-select and its round-trip, and the skipped rendering in the timeline.

## Boundary Self-Check (the two strategic-review questions)

- Are we rebuilding something that already exists? No — the orchestrator is one server-side goroutine state machine, and the scheduling substrate is still OS processes plus the existing RPC channel; no queue/scheduler middleware was introduced.
- Does it go through the unified Job model or open yet another execution channel? Unified — a step is a Job, entering the same ledger, the same permissions and the same audit; the Workflow API is only CRUD plus run control at the orchestration layer.
