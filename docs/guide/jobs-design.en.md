# P1 Design: Execution Job Model (Minimal Closed Loop)

> Language: [中文](./jobs-design.md) ｜ **English**

> 2026-10-04. First-phase landing design for P1 "Job". The strategy review (see reference-projects) listed
> Job/Action/Workflow/Event as the control plane's four core abstractions; this document only covers Job's
> minimal closed loop: one persistent execution record + one executable built-in type (agent.exec);
> Action/Workflow/Event are left to grow on top of Job later.

## Implementation Status (reconciled 2026-10-04)

- **Implemented** (22bd7fd → 4a0660f, six commits): the whole "minimal closed loop" scope of this
  document landed — the `storage.Job` model + AutoMigrate; Job type `agent.exec` going through the
  agent-side `job.exec` RPC provider (command ≤16KB, timeout ≤300s default 60s, 64KB output truncation,
  SIGKILL the process group on timeout); `GET|POST /api/jobs` + `GET /api/jobs/{id}` (synchronously
  waiting for terminal-state write-back); a single terminal audit entry `job_run` (details containing
  type/target/status/params); the Jobs page (all-machine ledger 5s polling + create-and-run modal +
  terminal-state details); permission points `jobs:read`/`jobs:write` (PermGuard gates the create entry).
- **Not implemented** (per this document's "Boundaries (explicitly not doing)" section, to be designed
  when the project starts; not written into current capabilities): streaming output, the Action/Event
  abstractions and AI wiring (P5).
- **Landed by P2 (2026-10-09, [workflow-design](./workflow-design.en.md) M1)**:
  asynchronous execution/cancellation (`cancelled` terminal state, cancellable while pending, ledger
  filtering) and Workflow orchestration; `POST /api/jobs` semantics changed accordingly to "create
  returns 201 pending immediately, dispatched in the background" (the original synchronous wait
  removed), and timeout still maps to `failed` per the design (no separate terminal state). The
  cancellation surface was then extended by M2c to running: `POST /api/jobs/{id}/cancel` kills the
  in-flight process via the `job.cancel` RPC (2026-10-11). For Workflow/scheduled triggering, the
  scheduling part remains not done (per workflow-design W11).
- **Live acceptance** (2026-10-05, probes `scripts/acceptance/jobs/` five-piece set 9/9 PASS,
  acceptance-checklist "unified Job execution" section all seven items checked): the execution chain
  (82ms success receipt / exit 3 output returned / timeout 1.078s returned on time with no orphans in
  `pgrep` / tail truncation exactly 64KB with the tail retained), offline 503 leaving no ghost records,
  400×7 across the validation surface, exactly one terminal audit entry, all 403s across the dual
  permission roles empirically verified. **Acceptance caught a gap in D8's routing surface**:
  `/api/jobs` was not registered in `resourceRules`, so RBAC governed=false let everything through
  (viewer could execute); fixed by adding the rule + a 7-case three-role matrix.

## Current State and Pain Points

| Pain point | Current state | Consequence |
|------|------|------|
| "Click to execute" has no unified entry | Running a backup once, manually triggering cron, and restarting a service each have their own API, none reused | Every new kind of action means a new RPC + a new page entry |
| No ledger for executions | Execution results scatter on completion; you can only piece the scene together from audit logs | You cannot even start to look up "what ran on that machine last night and what was the output" |
| Coarse permission granularity | Backup/proxy/scheduled tasks each have permission points, but generic command execution has no model | Cannot answer "who can run commands on this machine" |
| No executable surface for AI/automation to hook into | Without a unified Job API, any automation must route through business interfaces | The strategy review's P5 "AI wiring" has nowhere to land |

## Architecture Overview

The same channel model as scheduled tasks: **synchronous RPC + server persistence to DB**. Unlike cron's
"crontab is the state, server purely forwards", the value of Job lies exactly in the ledger — the
server is the source of truth.

```
┌─ server ───────────────────────────────────┐        ┌─ agent (target host) ─────────────────────────────────────────┐
│ POST /api/jobs                             │ ─RPC─▶ │ job provider (new)                                            │
│ validate → create Job(pending)             │        │ job.exec: sh -c / cmd /C                                      │
│   → CallAgent(job.exec)                    │ ◀─RPC─ │ run with timeout, return exit_code/output                     │
│   → write back final state + audit job_run │        │ (output truncated at 64KB, business fields never blow up RPC) │
│ GET /api/jobs[/{id}] ledger query          │        └───────────────────────────────────────────────────────────────┘
└────────────────────────────────────────────┘
```

## Key Decisions

| # | Decision | Choice | Rationale |
|---|------|------|------|
| D1 | State machine | `pending → running → success \| failed`; cancelled/timeout get no separate states — a killed timeout is failed (error="timed out after Ns", exit_code=-1); cancellation is a capability that only exists with later async-ification (**updated after P2**: `cancelled` is a real terminal state — DB-only cancellation while pending (W3), kill via RPC while running (M2c); timeout still folds into failed, unchanged) | Under synchronous RPC, running is transient, and a two-way terminal state already covers all current paths; the state set extends on demand |
| D2 | First type | `agent.exec`: run one shell command on the target agent's own machine (unix `sh -c` / windows `cmd /C`) | The first brick of the Control domain; command execution is the backstop semantics of every other action |
| D3 | Channel | Reuses the existing RPC: server `CallAgent(target, "job.exec", params)`, the agent-side JobProvider registered (platform-agnostic, registered unconditionally) | Structurally identical to cron/backup, zero new infrastructure |
| D4 | Exit-code semantics | A non-zero exit code is a **business field**, not a protocol error: the RPC layer is always success, `exit_code`/`output`/`error` go in data; the server lands success/failed accordingly (output still returned) | "Command failed" is a normal product of a Job, it should not be mixed into transport-layer errors |
| D5 | Timeout | Same limits on both ends: input `timeout_s` 1-300 (default 60); unix-side process group isolation (Setpgid) + kill the whole group on timeout, preventing grandchild processes from holding the output pipe and causing a "false timeout" | Timeouts must actually return on time; the `sh -c` derivation chain is the most common hanging source |
| D6 | Output cap | The agent truncates to the last 64KB (`jobExecMaxOutput`), the server persists at the same limit (`storage.JobMaxOutput`); command length capped at 16KB | Error messages are usually at the tail; prevents table bloat from long output |
| D7 | Offline blocking | Check the registry before creating; if the target is offline, return 503 directly, **no ghost record created** | Same criterion as cron; the ledger never shows "never executed" records |
| D8 | Permissions | Permission points `jobs:read` / `jobs:write` (registered in both resourceActions and allPermissions); server-side validation same criterion as cron (route-layer auth middleware) | The ledger is visible to everyone (read); execution is a high-risk operation (write) |
| D9 | Audit | One `job_run` at the terminal state (resource=job, id=Job ID), details containing type/target/status/params | Command inputs entering the audit share the cron_apply criterion, easing the trace "who ran what on which machine" |
| D10 | Storage | SQLite `jobs` table (AutoMigrate), Parameters stored as a JSON string and parsed back to an object at the API layer for return | Sufficient for single-machine personal-cloud scale; existing gorm conventions |

## Server-Side Design

### REST API

| Method | Path | Description |
|------|------|------|
| `POST /api/jobs` | `{type, target, parameters}` | Create and execute, 201 returns the pending view, terminal state dispatched in the background |
| `POST /api/jobs/{id}/cancel` | — | Cancel a pending/running Job (terminal states → 409) |
| `GET /api/jobs` | — | The most recent 50 ledger entries (descending) |
| `GET /api/jobs/{id}` | — | Single entry detail (including output) |

Validation (`validateJobPayload`): type required and on the whitelist (currently only `agent.exec`);
target non-empty; exec requires a non-empty `parameters.command` ≤16KB; `timeout_s` 1-300.

### Job View

```json
{
  "id": "…", "type": "agent.exec", "target": "agent-id", "actor": "username",
  "status": "success", "parameters": {"command": "uptime"},
  "output": "…tail 64KB…", "exitCode": 0, "error": "",
  "createdAt": "…", "startedAt": "…", "finishedAt": "…"
}
```

## Agent-Side Design

### RPC Method

| Method | Parameters | Returns | Description |
|------|------|------|------|
| `job.exec` | `{command, timeout_s?}` | `{exit_code, output, truncated, error, duration_ms}` | exit_code -1 = startup failure/killed on timeout |

- Cross-platform: `sh -c` (unix) / `cmd /C` (windows), `exec.CommandContext` + deadline;
- unix side: `Setpgid` process group + Cancel kills the whole group — grandchild processes spawned by
  `sh -c` (such as sleep) hold the output pipe, so killing only the direct child would leave
  `CombinedOutput` waiting until the grandchild exits naturally — the timeout semantics must be
  "the whole group terminates immediately";
- Platform differences outside agent.exec (in-container execution, idempotent Actions, etc.) belong to
  later types; the provider's action dispatch already has a slot reserved.

## Frontend Design

- `web/src/pages/Jobs/index.tsx`: all-machine ledger table (5s polling + manual refresh),
  create modal (online-agent dropdown + command + timeout), detail modal (state/exit code/output tail);
- `web/src/services/jobs.ts`: listJobs/createJob/getJob/cancelJob/createJobWithApproval (axios, Bearer injection);
- Route `/jobs`, menu "Job Execution", perm `jobs:read`, create button `jobs:write`.

## Tests

- storage: CRUD/descending/limit (`internal/storage/job_test.go`);
- agent rpc: success/non-zero exit/timeout killing the group/tail truncation/out-of-range/unknown action (`internal/agent/rpc/cov_job_provider_test.go`, the cases always applicable on windows run directly);
- server: each 400 branch of validation, offline 503, success/failure dispatch, list/detail/405/404 (`internal/server/cov_jobs_test.go`, fake agent injection);
- Coverage: defensive branches registered in `tool/known_uncoverable.txt` (SQLite query/write cannot error, the auth-context unit test convention, protocol cannot be deformed).

## Boundaries (Explicitly Not Doing)

- **No asynchronous execution** (**redeemed by P2, see the "Landed by P2" note at the top**): create
  waits synchronously for the terminal state. Async-ification (queueing/cancellation/progress) is designed
  together when Workflow starts as a project; at that point cancelled/timeout as separate states, the
  `GET /jobs?status=running` filter, and SSE push will have real consumers (landing criterion: the
  `cancelled` terminal state + ledger filtering are implemented; SSE remains decided not to do);
- **No Workflow/Action**: Workflow is "orchestration of a group of Jobs" (dependencies/parallelism/
  retries), Action is "an idempotent operation with semantics"; both sit on top of Job, and this phase
  has only Job itself;
- **No scheduled triggering**: Job and cron are two planes — cron is the agent-side scheduling plane
  (crontab is the state), Job is the control-plane execution ledger; "run a Job on a schedule" = cron
  writes a command line that calls `POST /api/jobs`, introducing no new coupling;
- **No real-time output streaming**: the terminal state returns everything at once. Streaming output
  (WebSocket attach) is left to a unified design in the remote-control/terminal domain.

## Future Growth Points

| Priority | Direction | Depends on |
|--------|------|------|
| P2 | Workflow: Job orchestration (linear chains/retries/cancellation, DAG/approval left to M2) — landed (P2 async-ification + M1 orchestration 2026-10-09; M2a variables/M2b fan-out/M2c cancel signal 2026-10-11, see [workflow-design](./workflow-design.en.md)) | This model |
| P0′ | Event model: Job terminal-state events enter a unified event bus | The Event abstraction project |
| P5 | AI wiring: the Job API is AI's execution surface (add structured types beyond agent.exec) | Audit and quotas first |
