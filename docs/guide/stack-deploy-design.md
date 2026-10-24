# P1 Design: Application Deployment (Compose Stack)


> 2026-09-14. Design for the first P1 feature. For reference sources and comparison conclusions see [Reference Project Comparison and Borrowing](/guide/reference-projects) (Komodo "Git-to-deploy" + Dockge "compose-file-first"). This document covers only design decisions and interface definitions; implementation proceeds per "Phased Delivery".

## Background and Goals

Cockpit can already manage the single-container lifecycle (the `/docker` page: start/stop/logs/delete), but there is no "application" layer — a typical personal service (such as `nginx + certbot + redis`) is a Compose project, and users have to run `docker compose up` manually on the host.

This feature fills that gap in Cockpit:

- Create, edit, and deploy Compose Stacks from the Web (up/down/view service status/view deployment logs)
- **The compose file itself is the source of truth** (Dockge's compose-file-first); the UI is a view
- Reserve an evolution path for P2's Git-to-deploy (Komodo style: stack files stored in a Git repo + triggered deployment)

**Non-goals (not in P1)**: image build pipelines, Git repository sync, multi-host orchestration template marketplaces, Kubernetes.

## Key Decisions

| # | Decision | Choice | Rationale |
|---|------|------|------|
| D1 | Compose execution approach | Agent shells out to the `docker compose` CLI | Same approach as Komodo/Dockge. The Go ecosystem has no embeddable compose engine library (compose-go is only a model/validation library); the CLI natively supports the full semantics of build/env/profiles/healthcheck |
| D2 | File source of truth | Local directory on the agent host `COCKPIT_STACKS_DIR` (default `/var/lib/cockpit/stacks`) | compose-file-first; the server database only stores an index cache. When P2 adds Git, this directory is upgraded to a Git worktree |
| D3 | RPC provider ownership | Create a separate `stack` provider instead of extending the `docker` provider | stack needs filesystem writes + the compose CLI, a different responsibility; providers register and degrade independently per capability, without affecting pure container management |
| D4 | Long-running deployments | Async task model: `up/down` returns `task_id` immediately; logs/status are obtained via polling | The server's `CallAgent` has a fixed 30s response timeout (`server.go`), which image pulls inevitably exceed; the safest solution without changing the protocol or adding a push channel |
| D5 | Frontend editor | P1 uses a monospace textarea; Monaco/CodeMirror are not introduced | Consistent with the earlier bundle-size strategy of "remove echarts-for-react to optimize chunking"; the editor upgrade is tracked as a separate item |

## Overall Architecture

Reuse the existing three-segment chain; no new protocol messages:

```
Web (Stacks page)
  │  REST /api/stacks/agents/{agentId}/...
  ▼
Server  api_stacks.go ── CallAgent RPC (existing 30s channel)
  │                        │
  │  storage.stacks        ▼
  │  (index + audit)    Agent  rpc.Handler ── stack provider
  │                                        │ ├─ docker.Client (status query, label filter)
  │                                        └ ─ docker compose CLI (up/down/logs)
  ▼                                        ▼
Audit log                             <stacks_dir>/<name>/compose.yml  ← source of truth
```

## Agent-Side Design

### Provider Registration

- Hooked under the existing `docker-api` capability: when `setupProviders` (`internal/agent/providers.go`) detects that capability and `docker compose version` is available, it registers the `stack` provider
- If the compose CLI is unavailable, no registration → the server returns 502 for that agent (the agent replies `unknown provider: stack`) → the UI shows an error, and container management is unaffected

### File Layout (compose-file-first)

```
/var/lib/cockpit/stacks/        # COCKPIT_STACKS_DIR, overridable
└── <stack-name>/
    ├── compose.yml             # source of truth, editable in UI
    ├── .env                    # optional, editable in UI (sensitive)
    └── .cockpit/
        └── last-task.json      # metadata of the most recent task (action/status/finishedAt)
```

Constraints:

- `<stack-name>` must match `^[a-z0-9][a-z0-9_-]{0,63}$` (which also satisfies the compose project name rules); all file operations are jailed inside `COCKPIT_STACKS_DIR`, eliminating path traversal
- Per-file limit 512KB; `.cockpit/` is a reserved directory, skipped during list
- A directory is a stack: valid directories placed manually from outside are recognized as well (Dockge behavior); the server side has no "register first, use later" flow

### RPC Action Table

Method names follow the existing `<provider>.<action>` format (`internal/agent/rpc/router.go`):

| Method | Params | Returns | Sync/Async |
|------|------|------|-----------|
| `stack.list` | — | `[{name, services: [{name, image, state, status, ports?: [{hostPort, containerPort, proto}]}], running, total}]` (ports are published ports, P2 ③) | Sync |
| `stack.status` | `name` | Service/container detail for a single stack (includes container IDs; can jump to `/docker`) | Sync |
| `stack.file.get` | `name` | `{compose, env, modifiedAt}` | Sync |
| `stack.file.save` | `name, compose, env?` | Runs `docker compose config -q` validation before saving; on failure returns an error without writing to disk | Sync |
| `stack.up` | `name` | `{task_id}`; equivalent to `docker compose up -d --remove-orphans` | Async |
| `stack.down` | `name` | `{task_id}`; equivalent to `docker compose down` | Async |
| `stack.logs` | `name, service?, tail?` | compose logs text | Sync |
| `stack.task.get` | `task_id` | `{stack, action, status: running/success/failed, log, startedAt, finishedAt}` | Sync |
| `stack.remove` | `name` | Runs `down` first (skipped if already stopped), then deletes the directory on success | Async |
| `stack.restart` | `name` | `{task_id}`; equivalent to `docker compose restart` (M1.5) | Async |
| `stack.pull` | `name` | `{task_id}`; equivalent to `docker compose pull` (M1.5) | Async |
| `stack.info` | — | `{dir, dirWritable, dirError?, composeVersion?}`: directory writability probe + compose version (M1.5) | Sync |
| `stack.git.status` | — | `{enabled, reason?, remote?, branch?, head?, remoteHead?, ahead, behind, changes?: [{code, path}]}`: stacks repo attach status + drift (P2 ①) | Sync |
| `stack.git.pull` | `remote?, branch?` | Post-attach snapshot same as `git.status`; the first call performs the attach (P2 ①) | Sync |
| `stack.git.deploy` | `branch?` | `{head, deployed: [{stack, taskId}], skipped: [{stack, reason}]}`: pull + deploy changed stacks (P2 ②, webhook-only; semantics in [P2 ② Design](#p2--design-2026-10-12)) | Sync (includes fetch; slow repos are bound by the CallAgent 30s limit) |
| `stack.task.follow` | `taskId, followId` | `{followId}`: subscribes to task log increments, pushed back via proxy_data (`proxyId="stacktask:<followId>"`); terminal state via proxy_close (P2 ②) | Async session |
| `stack.task.follow.stop` | `followId` | Stops a follow session (P2 ②) | Sync |

Implementation notes:

- **Task model**: the provider keeps `map[taskID]*Task` in memory plus one mutex per stack; when concurrent `up/down` happens on the same stack, the second request fails immediately with a `stack busy` error (the server maps it to 409). Task logs are written only to an in-memory ring buffer (keeping the most recent ~200KB), never to disk
- **Zero changes to status queries**: reuse `docker.Client.ListContainers(true)` (`internal/docker/client.go`), filtering on the client side by `Labels["com.docker.compose.project"] == name`, leaving the DockerAPI interface signature untouched; add a Filters parameter later once scale demands it
- **`compose.yml` vs `compose.yaml`**: reads and writes prefer `compose.yml`; `compose.yaml`/`docker-compose.yml`/`docker-compose.yaml` are recognized for compatibility (read-only compatibility; saving normalizes to `compose.yml`)

## Server-Side Design

### REST API

New file `internal/server/api_stacks.go`, replicating the `api_docker.go` pattern (JWT middleware + path parsing + `CallAgent` + error code mapping):

| Route | RPC | Notes |
|------|-----|------|
| `GET /api/stacks` | — | Aggregated view: iterate online agents with docker-api, fetch `stack.list` concurrently, and merge the DB cache (offline agents show cached state) |
| `GET /api/stacks/agents/{agentId}` | `stack.list` | Stack list for a single agent |
| `GET /api/stacks/agents/{agentId}/{name}` | `stack.status` | Stack detail |
| `GET /api/stacks/agents/{agentId}/{name}/compose` | `stack.file.get` | Read files |
| `PUT /api/stacks/agents/{agentId}/{name}/compose` | `stack.file.save` | Write files (body: `{compose, env}`) |
| `POST /api/stacks/agents/{agentId}/{name}/{action}` | `stack.up` / `stack.down` / `stack.restart` / `stack.pull` | action ∈ {up, down, restart, pull}; returns `{task_id}` |
| `GET /api/stacks/agents/{agentId}/{name}/history` | — | Deployment history (server-side `stack_deployments` table), `?limit=` (default 50, max 200) (M1.5) |
| `GET /api/stacks/agents/{agentId}/{name}/logs` | `stack.logs` | `?service=&tail=` |
| `GET /api/stacks/agents/{agentId}/tasks/{taskId}` | `stack.task.get` | Task polling |
| `POST /api/stacks/agents/{agentId}/tasks/{taskId}/follow` | `stack.task.follow` | Task log NDJSON stream (`{"data":...}` increments / `{"eof":true,"reason":terminal-state}`); limits are 8 global / 4 per agent (P2 ②; browsing-type actions are not audited, same discipline as log tailing) |
| `POST /api/stacks/webhook` | `stack.git.deploy` | Forge callback entry: the `X-Cockpit-Token` header is compared against the configured `stacks.webhook_token` in constant time; the body optionally takes `{agentId, branch}`, defaulting to fanning out to all online agents; returns `{triggered:[...]}` immediately while deployment runs async (audit username=webhook) (P2 ②; bypasses the auth middleware; empty token = endpoint disabled as 404) |
| `GET /api/stacks/agents/{agentId}/git` | `stack.git.status` | Stacks repo attach status + drift (P2 ①; not audited) |
| `POST /api/stacks/agents/{agentId}/git/pull` | `stack.git.pull` | Pull from the remote (the first call attaches; body optionally takes `{remote, branch}`; audited as stack_git_pull) (P2 ①) |
| `DELETE /api/stacks/agents/{agentId}/{name}` | `stack.remove` | Delete a stack (body is just `{name}`) |

Conventions: 404 = agent does not exist / lacks docker capability; 502 = has docker capability but the stack provider is not registered (compose CLI unavailable, agent replies `unknown provider: stack`); 409 = stack busy; 504 follows the RPC error mapping in `api_docker.go`. P2 ① addition: 409 is also used for git pull conflicts (local changes/divergence rejected by `--ff-only`); an agent reporting "remote branch not found" (empty repo / wrong branch name) → 404.

### Storage

`storage` gains a `stacks` table (index cache, not the source of truth):

```
Stack{ ID, AgentID, Name, Running, Total, LastAction, LastStatus, LastDeployedAt, UpdatedAt }
```

- Upserted when agents report list; when an agent is offline, `GET /api/stacks` uses it to render the greyed-out state
- The deployment history table (one row per up/down) is deferred to M1.5; P1 relies on audit logs for traceability
- The inventory integration (P2 ③) creates no new table: stack published ports are incorporated as `services`-table ledger rows (`labels.source=stack`, deterministic ID `stackport-*`), and probe reuses the existing service probing

### Audit

Reusing `audit.Logger`, new events: `stack_create` (first save), `stack_update` (save), `stack_up`, `stack_down`, `stack_remove`; detail records name + agent + task result. **The existing constraint is upheld: `.env` contents and deploy output never enter the audit log** — only events and metadata are recorded.

## Web-Side Design

New page `web/src/pages/Stacks/`, nav "Application Deployment", route `/stacks`; the `/docker` page keeps its single-container view untouched, and container rows in stack detail provide jump links.

- **List page**: cards/tables grouped by agent (name, running/total, last deployed time, status Tag); a "New Stack" button at the top (pick agent + name + template: empty / Nginx example)
- **Detail page** (drawer or sub-route) with three tabs:
  1. **Services**: service/container status table + "Start/Stop" primary buttons (trigger up/down → poll task → refresh on completion)
  2. **compose.yml**: monospace textarea editing + save (shows the `docker compose config` error output when backend validation fails); `.env` in a separate collapsible section, shown in plaintext (contents are not audited)
  3. **Deployment logs**: polls `tasks/{id}` every 2s while a task runs, in a terminal-style dark block (reusing the `/docker` log Modal styling)
- Agent offline / compose unavailable: the list is greyed out and actions are disabled with the reason shown

## Security Design

1. **Path safety**: whitelist regex for stack names; file reads/writes uniformly go through `filepath.Dir` cleaning + prefix validation, jailed inside the stacks root directory
2. **Least capability**: the stack provider registers only when both conditions — docker-api + compose CLI — are met; agents without the `docker-api` capability never have this attack surface
3. **Audit closure**: all write operations (save/up/down/remove) pass through the JWT middleware + audit events; .env contents are not audited
4. **Resource limits**: 512KB cap on compose files; 200KB ring buffer for task logs; provider-level concurrent task cap (default 4)
5. **File permissions**: `0700` recommended for the stacks root directory; docs provide a systemd unit example (to be added to `getting-started` later)

## Phased Delivery

### M1 — P1 core (scope of this project item)

- [x] Agent: `internal/agent/rpc/stack_provider.go` (list/status/file/task/up/down/logs/remove + compose CLI detection)
- [x] Agent: providers.go registration logic + unit tests (fake script simulating the compose CLI)
- [x] Server: `api_stacks.go` + storage stacks table + audit events
- [x] Web: Stacks page (list/detail/edit/deploy/log polling)
- [x] Acceptance (local end-to-end, 2026-09-14): real server/agent binaries + fake Docker Engine (`scripts/local-acceptance/`), 32 assertions all passing and repeatable, covering "create → validation failure not written to disk → up (concurrent 409) → status linkage → logs → down → delete → disconnected grey state → audit closure + .env not leaked"; see [Acceptance Records](#acceptance-records) below
- [x] Acceptance (real Docker test machine, 2026-09-28): real image pulls, container health/ports/restart policies, full compose semantics (env/volumes/custom networks/depends_on/pull/profiles/healthcheck), `COCKPIT_STACKS_DIR` 0700 practice — see [Acceptance Records](#acceptance-records)

### M1.5 — Experience gaps filled (completed 2026-09-14)

- [x] Deployment history table + detail page timeline: `stack_deployments` table (server-side records); start-type actions insert a running row when dispatched, and a background goroutine polls every 2s to backfill the task's terminal state (not terminated when the agent goes offline; resumes fetching the real terminal state after reconnect; 15min timeout fallback marks failed); detail page "History" tab timeline
- [x] Template library expansion (empty/Nginx/WordPress/Nginx Proxy Manager/Uptime Kuma) + import (in the new-stack dialog, pick a template to prefill or paste a compose.yml directly; saving creates it)
- [x] `restart` / `pull` actions (async tasks through the same launchTask channel as up/down + audit stack_restart/stack_pull); multi-file compose (`include:`) assessment: current single-file semantics suffice, revisit when the P2 Git repo scenario surfaces real multi-file needs
- [x] Self-check hints for stacks directory permissions/location: `stack.info` RPC (directory writability probe + compose version), the list API passes through `agentInfo`, and the list page shows a top Alert when the directory is abnormal

### P2 — Git-to-deploy (Komodo-style upgrade)

- [x] Stacks directory upgraded to a Git worktree: remote repo pull → drift hints (local changes vs repo) (2026-10-12; see [Git-to-deploy ① Design](#p2--git-to-deploy-design-2026-10-12) below)
- [x] Webhook-triggered deployment; streaming deployment log push (reusing the proxy data channel, replacing polling) (2026-10-12; see [P2 ② Design](#p2--design-2026-10-12) below)
- [x] Inventory integration: stacks linked to CMDB service entries, probe directly probes ports exposed by stacks (2026-10-13; see [P2 ③ Design](#p2--design-2026-10-13) below)

#### P2 ① Git-to-deploy Design (2026-10-12)

**Model**: the stacks root directory as a whole is attached to a single remote as one git repo (not one repo per stack) — in personal-service scenarios the stacks are usually just a single repo (same granularity as Komodo/Dockge); per-stack repos can be revisited when a real need appears.

**Semantics** (decision record: decision-log D-2026-10-12-5):

- **Attach** (directory not yet a repo): `git init` → `symbolic-ref HEAD refs/heads/<branch>` (`init -b` requires git≥2.28; symbolic-ref works on all versions) → `remote add origin` → `fetch` → `checkout -B <branch> origin/<branch>`. **Requires the remote branch to already exist** (for an empty repo, push the first version from somewhere else first); when remote files share names with existing local files, git refuses to overwrite untracked files rather than silently swallowing local files
- **Pull when already attached**: `fetch --prune` → `merge --ff-only origin/<branch>`. No automatic commits and no rebase/merge decisions made on the user's behalf; when local commits have diverged or tracked files were modified, git refuses to merge and the raw error is passed through to the UI (409 conflict) — that is itself the drift hint
- **Drift = changes in the working tree relative to HEAD** (`status --porcelain -uall`): UI edits to compose (stack.file.save) are inherently drift — under the compose-file-first principle this is a feature, not a flaw; the UI hints without blocking; `.cockpit/` and `.git/` paths are filtered in code (no `.gitignore` is pushed down, avoiding polluting the user's repo)
- **Status queries stay offline**: `stack.git.status` reads only local refs (`behind/ahead` reflect the last fetch point); the pull action itself does the fetch — the status surface never hangs the UI on a stuck remote

**Security**:

- Remote URLs reject `::` (git transport-helper syntax; `ext::sh -c ...` can execute arbitrary commands), leading `-` (to prevent being treated as a git option), and control characters; branch names are constrained to `^[A-Za-z0-9][A-Za-z0-9._/-]{0,99}$` and reject `..`
- Userinfo credentials in URLs (`user:pass@`) are uniformly masked to `***` before any echo (error messages/audit/status); the credentials themselves are managed by the git credential helper on the agent host and never stored by the server
- The remote URL persists in `.git/config` on the agent side (the server creates no table); write operations (pull) are audited as `stack_git_pull`, status queries are not

**UI (Stacks page GitBar)**: one alert row per agent — not attached = low-key info + an "Attach" entry; no git = warning; drift = warning listing changed files (collapsed beyond 5); behind>0 = info + "Pull updates". Old agents that don't recognize the new RPC (502) are skipped silently.

#### P2 ② Design (2026-10-12)

**Webhook-triggered deployment** (decision record: decision-log D-2026-10-12-6):

- **Entry**: `POST /api/stacks/webhook` (exact route, registered before the `/api/stacks/` auth prefix; ServeMux longest-match wins). Forge push callbacks carry no login state, so the token is the credential: the `X-Cockpit-Token` header is compared with the configured `stacks.webhook_token` in constant time; **empty token = endpoint disabled as 404 (off by default, enabled by explicit configuration, same red-line policy as notification)**
- **Semantics**: only triggers "pull the attached origin + deploy changed stacks"; **the `remote` parameter is not accepted** (any remote in the request is stripped — a webhook must not redirect the deployment source); the body optionally takes `{agentId, branch}`: to target a single agent, otherwise fans out to all agents with an online heartbeat (agents that haven't attached a repo fail the RPC, logged only)
- **Execution**: async — the response returns `{triggered:[...]}` immediately, and one goroutine per agent runs `stack.git.deploy`; success writes audit (`username=webhook`, action `stack_webhook_deploy`) + per-stack deployment history rows (mapped as up with terminal-state tracking); RPC failures only land in the server log
- **Deployment scope** (`stack.git.deploy`): no HEAD before attach → deploy all stacks in the repo; HEAD moved forward → top-level directories of `git diff --name-only old new`; HEAD unchanged → no deployment. A target stack whose task is still running (lock held) is recorded `skipped` (reason goes through credential masking) without blocking the remaining stacks
- **Drift protection**: keep git's own refusal — local changes/divergence that make `merge --ff-only` fail abort the whole deploy (no automatic stash, no committing on the user's behalf; same review conclusion as the reopen conditions of D-2026-10-12-5)

**Streaming deployment log push** (architecture same as logs-design.md F4 tailing):

- Agent side `stack.task.follow`: reuses the same proxy_data channel as log tailing with `proxyId="stacktask:<followId>"`; the pump pushes task log increments every 300ms, and when the task reaches a terminal state the reason is returned via proxy_close; the agent caps active follows at 8
- Server side `POST /api/stacks/agents/{agentId}/tasks/{taskId}/follow`: NDJSON stream (`{"data":...}` increments / `{"eof":true,"reason":terminal-state}`); registry quota is 8 global / 4 per agent, and a full 256-frame buffer drops frames (never blocking the agent WS read loop); client disconnects are detected via `r.Context().Done()` and followed up with `stack.task.follow.stop`; browsing-type actions are not audited
- Web side StackDetail: during task runtime, opens a follow instead of 2s polling; on `eof` it clears the live area + fetches the terminal state once; if follow cannot be opened (e.g. old agent 502), it automatically falls back to polling with a "polling" hint
- Known limitation: the CallAgent 30s timeout constrains the fetch duration of `stack.git.deploy` (webhook deployments of large repos over slow networks may time out and fail; just retry)

#### P2 ③ Design (2026-10-13)

**Inventory integration** (decision record: decision-log D-2026-10-13-1):

- **Source of facts**: service entries in the agent's `stack.list` / `stack.status` carry published ports (the docker list API returns `Ports` along with the container list, zero extra Engine calls); `ContainerInfo.Ports` → `StackService.Ports: [{hostPort, containerPort, proto}]`, keeping only `hostPort > 0` — unpublished ports are reachable only inside the container network and probe cannot reach them
- **Incorporation**: in `cacheStack` (the list sync point shared by both the single-agent and aggregated paths) the server calls `syncStackServices`: each **tcp** published port upserts a CMDB `Service` ledger row with `Type=tcp` and `URL=tcp://{agentIP}:{hostPort}` → probe goes through the existing `parseHostPort` + `CheckTCP`, **zero changes on the probe side**; no new table, the `services` table is reused
- **Deterministic ID**: `stackport-{agentId}-{stack}-{hostPort}-tcp` (same precedent as `dns-`); when a port disappears it is deleted by ID, no reverse lookup needed
- **Origin marker**: `Labels{source: "stack", agent_id, stack}` (same precedent as DNS `labels.source=dns`); rows declared by inventory or registered manually lack this label and are never touched by reconciliation
- **host takes the agent IP**: registry first, falling back to the DB record when offline; an empty IP creates no row (no probe target); on IP change the URL refreshes but the probe status is preserved — `storage.UpsertServiceTarget` touches only addressing fields (name/agent/type/url/labels), while `status/responseTimeMs/lastCheck` belong exclusively to probe
- **Reconciliation**: port reconciliation runs on every `syncStackServices` (rows outside the set owned by that stack are deleted); reconciliation of vanished stacks runs `pruneStackServices` after a successful full-list sync — an unreachable agent ≠ deleted, and agents whose probing failed are not reconciled
- **Boundaries**: udp/sctp ports are only displayed in the UI, not incorporated (probe implements TCP/HTTP only); no manual mapping UI (CMDB Services remain managed by the inventory.yaml file — there is no place for manual association until a real need appears); Dashboard ServicesUp/Down counts include stack rows (an acceptable coupling — ledger rows ought to be monitored anyway)
- **Best effort**: integration failures are only logged and do not affect stack caching or API responses (same policy as the DNS ledger integration D20)

## Risks and Trade-offs

| Risk | Mitigation |
|------|------|
| compose CLI behavior drifts across Docker versions | Depend only on the four stable subcommands `up -d --remove-orphans` / `down` / `logs` / `config -q`; the agent probes the version at startup and reports it |
| Polling latency (deployment completion is noticed at most 2s late) | Acceptable; P2 adds streaming. No premature protocol expansion for this |
| Agent process restart loses in-memory task state | Agent restart loses in-memory task state → task.get 404; historical terminal states are backstopped by the server's `stack_deployments` table |
| User hand-edits conflict with UI edits | compose-file-first principle: saving overwrites (with an optimistic modifiedAt hint; no forced merge) |
| Windows agent | Not supported in P1; provider registration checks `runtime.GOOS` and skips non-linux/darwin outright with a log |

## Acceptance Records

### 2026-09-14 Local end-to-end acceptance (no real Docker)

**Method**: a one-shot run of `scripts/local-acceptance/run-acceptance.sh` — real
`cockpit` server and `cockpit-agent` binaries, with Docker replaced by a stand-in (a
minimal Engine API on a unix socket + a fake compose CLI on PATH; after `up` the
status genuinely flows into `status` queries).
The chain passes fully through REST → JWT → audit → WebSocket RPC → stack provider → compose invocation.

**Result**: all 32 assertions pass, reruns are repeatable; `go test ./...` fully green.

**Coverage**: create (created=true) → compose/.env readback consistent → invalid
compose fails validation without touching disk → list → up async task
(taskId/poll to success) → concurrent up of the same name 409 →
status linkage (running=2/total=2) → logs → down → remove deletes the directory →
agent disconnect cached grey state (online=false) → audit events
(stack_create/up/down/remove) all present and `.env` values not leaked.

**Found and fixed during acceptance**: `DOCKER_HOST` did not support the
Docker-official conventional format `unix:///path` — the detector `stat`-ing the
prefixed string directly always failed, so on real machines the agent missed the
docker capability when users set the standard format; the bare-path form, while
detectable, failed again when the endpoint was handed to the docker library
(which requires a scheme). Fix: `detector.normalizeUnixHost` accepts both forms
and uniformly outputs a `unix://` endpoint (with unit tests).

**Left over (real Docker test machine)**: real image pulls, container
health/ports/restart policies, compose build/pull/profiles/healthcheck semantics,
`COCKPIT_STACKS_DIR` permission practice.

### 2026-09-14 Local end-to-end acceptance · M1.5 supplement

The same script suite, extended, runs the M1.5 new features; all 42 assertions pass and reruns are repeatable:

- **A15 restart/pull**: both actions go through the same async task channel as up; task polls reach success
- **A16 deployment history**: `/history` returns server-side records, all three actions up/restart/pull present, terminal states backfilled by the background goroutine (`finishedAt > 0` and status=success)
- **A17 directory self-check**: the aggregated API's `agentInfo` carries `dirWritable=true` and the stacks directory path
- **A14 audit expansion**: all six events present (create/up/down/remove/restart/pull), `.env` values still not leaked

### 2026-09-28 Real Docker test machine acceptance

**Environment**: Docker 29.8.1 + Compose v5.5.1 on a real machine (Linux VM, agent running as a root systemd service, `COCKPIT_STACKS_DIR=/var/lib/cockpit/stacks`); the server is the local acceptance binary (`.acceptance/` dedicated config, not committed); images pulled from the real Docker Hub (nginx:alpine, busybox).

**Passed (R1-R17 + supplements S1-S3)**:

- **Create/readback/validation**: created=true, compose readback md5 identical; invalid compose intercepted by the real `docker compose config -q` (502) and the original file not written
- **Full deployment chain**: up really pulls images and starts containers (running=2/total=2, `profiles: [debug]` service correctly excluded); port reachable across hosts (18080→HTTP 200); healthcheck reaches healthy; `unless-stopped` and `.env` variable substitution verified for real (printenv inside the container)
- **Self-healing/logs/actions**: after killing the container process, Docker restarts it per the restart policy automatically (~7s, within restart backoff); logs return real nginx output; pull/restart take real effect
- **History/audit**: deployment history finishedAt backfilled by the background job (including one honest failed record); all 5 audit events present and `.env` values not leaked
- **Full compose semantics (S1/S3)**: named volumes shared across containers + depends_on startup ordering (db writes the volume → app reads the seed; volume created with the project prefix); custom bridge network created + inter-service DNS resolution (container b `ping a` succeeded then dropped a file as proof)
- **Boundary behavior**: build without a Dockerfile fails gracefully (task failed log readable, server unaffected); `down` keeps named volumes (compose default semantics, not `-v`, as expected); remove deletes the directory; directory self-check dirWritable=true + composeVersion 5.5.1
- **Directory permissions**: `/var/lib/cockpit/stacks` 0700 and the writability probe passes; the "directory does not exist" scenario auto-creates it (list returns empty + info reports dirWritable=true)

**Environment noise (not a defect)**: the first up failed due to a transient Docker Hub auth blip (the task log faithfully records the original error); retry passed.

**Found and fixed during acceptance**: when the stacks directory path was invalid (e.g. misconfigured as a regular file), `stack.list` errored, and `GET /api/stacks/agents/{id}` used to replace the agent's original error (`read stacks dir: ... not a directory`) with a generic "Invalid agent response", inconsistent with `handleStackRPC`'s pass-through behavior; it now passes the original reason through (with a unit test asserting the pass-through text).

**Known boundaries (recorded as follow-up small items, not fixed this round)**:

- In the invalid-path scenario, `stack.info` (dirError) never reaches the list page — the server fetches self-check info only after list succeeds; needs a server-side "fetch info even when list fails" + web presentation linkage
- The "no permission" scenario cannot be simulated for a root agent (CAP_DAC_OVERRIDE bypasses DAC); needs a non-root deployment for real verification
- The nginx:alpine/busybox images were left as VM cache, not cleaned up; the template library/import are UI fill-in paths (persisting is just the regular create+up; the UI chain is covered by vitest), verified on the real machine equivalently with raw compose

### 2026-10-12 Local acceptance (P2 ① Git-to-deploy, A18)

**Method**: the `run-acceptance.sh` A18 section — a local bare repo serves as the remote (no external credentials needed), a seed working repo pushes three versions of content, and the chain goes through real REST → JWT → audit → WebSocket RPC → stack provider → git CLI. All 53 assertions pass; reruns are repeatable.

**Coverage (A18.1-A18.7)**: not attached reason=not-a-repo → attach (enabled=true, remote files landed, head=remoteHead) → second pull after a new remote commit fast-forwards (disk content updated) → local edit of a tracked file + new remote commit of the same file → pull 409 → `stack.git.status` drift list (a stack created via UI before attach is recorded as untracked `??`, the expected semantics) → `ext::` transport-helper remote rejected (502) → audit contains `stack_git_pull`. Agent unit tests additionally use real git to cover attach/untracked conflict not overwriting local files, branch switching, credential masking, and `gitPathSkipped` boundaries.

**Found and fixed during acceptance**: the agent-side execlog (router single-point instrumentation) recorded the `env` param of `stack.file.save` (plaintext .env credential payload) verbatim into the trace and uploaded it into the audit table — violating the "credentials never enter audit" red line; it had escaped exposure only because execlog batches at 30s, later than when acceptance collected audit records. Fix: the `core/execlog` sensitive-key regex gains `env` (the only user in the repo is stack.file.save), and the A14 regression assertion turned green.

### 2026-10-12 Local acceptance (P2 ② webhook + log stream, A19)

**Method**: the `run-acceptance.sh` A19 section — the same local bare remote, with `stacks.webhook_token` configured and enabled; the chain is REST (no login state, `X-Cockpit-Token`) → webhook fan-out → WebSocket RPC `stack.git.deploy` → git CLI → fake compose → task follow NDJSON stream. All 62 assertions pass; reruns are repeatable.

**Coverage (A19.1-A19.7)**: no token configured → 404 (off by default) → token configured but `X-Cockpit-Token` missing/wrong → 401 → malicious body with an off-disk `remote` accepted but the param stripped (200 + triggered) → new stack landed on disk + attached stack fast-forwarded (nginx:webhook) → deployment history action=up → follow stream contains the `"eof":true` frame → follow on unknown task 404 → audit contains `stack_webhook_deploy` (username=webhook). A18.4-A18.7 drift case regressions stay green.

**Found and fixed during acceptance**: ① `stack_provider.go`'s Call dispatch was missing the `git.deploy` case (the method was implemented but the RPC couldn't reach it, reporting unknown stack action) — unit tests calling the method directly masked the missing dispatch; added the case and switched one test to go through `Call` to cover the dispatch line; ② A19.4/A19.5 assertions raced against async persistence (webhook returns 200 upon acceptance, but the deployment history row has to wait for `startStackDeployment` to write it and `trackStackTask` to reach the terminal state before status flips) — changed to poll for a row with `action=up && status!=running` before asserting; ③ the follow-final `stop` RPC was changed to fire-and-forget (goroutine) — synchronously waiting for the reply would hang the handler on the CallAgent 30s timeout when the agent doesn't answer (the no-flusher 500 path race step measured 30s in practice). The race step also caught, in flagrante, a **pre-existing** race: `RemoteSessionManager.Get/List` leaked storage pointers directly, and the WS data-plane goroutine concurrently wrote the same session fields via `UpdateStatus` — the read side now takes a shallow-copy snapshot under lock (separate fix commit); the server package race-instrumented run locally measured 1004s, so the CI race step timeout went 900s→1500s.

### 2026-10-13 Local acceptance (P2 ③ inventory integration, A20)

**Environment**: local bare repo as the remote (built in A18) + fake-docker-daemon (from A20 on, `compose up` parses the compose ports section and outputs `Ports` public/private pairs) + a real agent binary; `cache-demo` carries dual ports `8080:80 tcp` + `5353:53 udp`.

**Found and fixed during acceptance**: the A20 assertion mistakenly used `/api/services` (that route doesn't exist; CMDB ledger rows hang off resource dispatch under `/api/resources/services`) — the service rows were in fact correctly persisted (seen via direct DB query and probe inspection alike), a pure assertion-URL typo; after correction, A20.1-A20.4 are all green (ledger rows incorporated / name finalized / URL pointing at the agent host port / udp not booked / service view with port list).

- [Komodo Git-to-deploy](https://komo.do/docs/intro) — Core/Periphery isomorphic, automation-first programs (a good comparison for this design's P2)
- [Dockge](https://github.com/louislam/dockge) — compose-file-first, directory-is-stack
- Internal: [Reference Project Comparison and Borrowing](/guide/reference-projects), [Protocol Definition](/guide/protocol), `todo.md` P1 items
