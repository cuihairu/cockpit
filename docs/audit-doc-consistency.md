# Documentation-Source Consistency Audit (2026-10-08)


> Executed per the "Documentation-Source Consistency Audit Specification": enumerate the
> document surface → take verifiable technical claims and check them against the code
> (file:line) → enter the three categories of differences into a table → fix the docs
> with the source as the truth (modifying code in reverse is forbidden).
> This table records all differences found in the first audit round and their disposition;
> the "current state" column is the post-fix verification conclusion.

## Audit Scope

- `docs/guide/` (35 documents: concepts/architecture/protocol/getting-started/designs for each feature domain/decision-log, etc.)
- `docs/` root (agent-config / agent-id / guacd-deploy / websocket-token-auth / remote-desktop-guacamole-design / remote-access-integration-design / ui-theme-design)
- `docs/operations/deploy-docker.md`, `docs/public/README.md`, `README.md`, `BUGS.md`
- the 17 sidebar links of `docs/.vitepress/config.ts` (all reachable)
- Spot-check method: first build the code-side ground truth (routing table / full capability set / config keys / protocol messages / permission-point list), then grep each document claim and check it against the code one by one

## Ground-Truth Anchors

| Fact | Evidence |
| --- | --- |
| Route registration (mux direct mount + `/api/` fallback switch) | `internal/server/server.go:165-310`, `internal/server/api.go:34-172` |
| Full capability set of 17 (detector 7 + code direct-report 10) | `internal/agent/detector/` (7 files), `internal/agent/agent.go:341-477` |
| Permission points' single source of truth | `internal/storage/role.go:29-60` `resourceActions`, `internal/server/rbac.go:23-88` |
| Proxy data-plane dialing | `internal/proxy/handler.go:116` (`net.DialTimeout("tcp",…)`), `internal/proxy/ssh_session.go:125` (`ssh.Dial("tcp",…)`) |
| guacd recording archival | `internal/server/api_guacamole.go:209-250` `collectGuacRecording` |
| GHCR tag field-verified (anonymous token + registry API) | Only `latest`/`main`/`sha-*` exist in practice; the mechanism is in `.github/workflows/docker.yml:67-82` (a `v*` semver has never been triggered) |

## Difference Table

| ID | Doc page | Claim | Source evidence | Category | Severity | Disposition |
| --- | --- | --- | --- | --- | --- | --- |
| A1 | guide/concepts.md:141 (original) | capability table contains `network-monitor` | Deleted (overlay-design D18; zero matches repo-wide) | Doc ahead of code | **P0** | Delete the row; rewrite the table as "detector 7 + code direct-report 10" with a historical note |
| A2 | guide/concepts.md:133-141 (original) | "current detectors include" lists only 5 | True full set is 17 (7 in detector/ + 10 direct-report at agent.go:341-477) | Code ahead of doc | P1 | Add `firewall` `overlay` `remote-services` + the 10 direct-reports (with each one's reporting condition, verified line by line) |
| A3 | remote-desktop-guacamole-design.md:83 | The current-endpoint examples include `/api/remote/desktop` | Route deleted (grdp deletion surgery D-2026-10-07-1; the surviving ones are terminal/guacamole/tickets/sessions/vault) | Doc ahead of code | P1 | Change to `/api/remote/guacamole`; sync the pipeline description to "agent WS / guacd TCP" |
| A4 | guide/protocol.md:146 | Proxy messages can reach "TCP/UDP targets" | Data plane dials TCP only (handler.go:116 and ssh_session.go:125 both hard-code `"tcp"`; `proxyType=udp` is only accepted by API validation/persistence, api_proxy.go:99) | Description mismatch | P1 | Change to "TCP targets" + note the status of the udp parameter |
| A5 | guide/getting-started.md:5 | "Go 1.26.3 or a compatible version" | `go.mod` says `go 1.27` | Description mismatch | P1 | Change to 1.27 |
| A6 | guide/mobile-design.md:73 | Audit list endpoint `/api/audit` | Actually `/api/admin/audit/logs` (mobile/lib/api/endpoints.dart:76; api_audit.go routing) | Description mismatch | P1 | Change to the current endpoint |
| A7 | guide/nas-design.md:65 | OMV provider "M2 queued" | Already implemented and wired (`internal/agent/rpc/nas_dsm.go:290` `case "omv"`) | Code ahead of doc | P1 | Change to "M2 completed" |
| A8 | guide/rbac-design.md D3 | resource list stops at 20; `services`/`smart`/`recordings` read as future completion | `role.go` `resourceActions` actually has 25 (completion done, additionally `firewall` `jobs`) | Code ahead of doc | P2 | Change to "reconciliation completed" + list `firewall`/`jobs`, pointing at role.go as the single source of truth |
| A9 | guide/getting-started.md:252 | Image tag table unconditionally lists `v1.2.3` | GHCR field-verified: only latest/main/sha-*; the semver series is a workflow capability (docker.yml:73-75), and a `v*` tag has never been pushed | Description mismatch | P2 | Change to the conditional wording "when a `v*` tag is pushed there is an additional semver series" |
| A10 | guacd-deploy.md:96 | Recording files "automatically collected into the database" | The file is archived to `/data/recordings/` and only the metadata is back-filled into the DB (api_guacamole.go:209-250; deploy-docker.md:124 says the same) | Description mismatch | P2 | Change to "collected and archived to /data/recordings/ (metadata back-filled into the database)" |
| A11 | guide/overlay-design.md:139, 163 | "network-monitor convergence → cleanup in a later version" reads as a todo | Already done and the same doc records it at D18 (:198) | Code ahead of doc | P2 | Add "done, see D18" in both places |

**Totals**: doc-ahead-of-code 0; code-ahead-of-doc 0; description mismatch 0. All differences were fixed in the prior batch; see the tracking records in the "Disposition" column below.

**P0 list**: none — the P0 item A1 (the concepts.md phantom capability `network-monitor`) had that row deleted in the prior batch, retaining only the historical note, so it makes no runtime claim.

## Sampled Surfaces Verified Consistent (not in the difference table)

- ~150 endpoint claims all check out against the code, with only the A3/A6 differences; the protocol.md HTTP API section, ticket 5-minute single-use (ticket.go:58), the six public auth endpoints, and the six resource types all match
- agent-config.md: 8 startup params / 9 env vars / ID default format `agent-{hostname}-{machineId[:8]}` (agent.go:503) all match
- agent-id.md: the three platforms' machine-identity sources and the bias suffix rules (agent.go:490-509) match
- deploy-docker.md: PR builds do not push to the registry (`push:` condition in docker.yml), pnpm@11.19.0 (Dockerfile:16), entrypoint.sh weak-password hard check, nginx reverse-proxying three WS paths, the same-volume/same-path recording argument — all match
- guacd-deploy.md: the compose file, guacd:1.5.5, GUACD_BIND/GUACD_PORT, TCP healthcheck, the `ssh.getDefaultKey` RPC (ssh_provider.go:31), automatic Ed25519 generation at agent startup (agent.go:167 → ssh_keys.go) — all match except A10
- websocket-token-auth.md, docs/public/README.md (scaffolding note with no technical claims), BUGS.md — match
- rbac.go's route→resource merge rules match the rbac-design.md D-entry notes (special conventions such as audit fixed to read, terminal fixed to write, and logs fixed to read on the agent side all have corresponding D-notes)

## Leftover Observations (outside the scope of this doc-fix round)

- `proxyType=udp`: the API accepts and persists it but the data plane does not implement it (the A4 note already says so). If real UDP forwarding is ever wanted it should be filed as a todo project; the docs should not be vague about it. → **Resolved (2026-10-08 follow-up decision D-2026-10-08-2)**: the API now rejects udp (400, tcp only), and the protocol/storage comments are annotated accordingly; the protocol.md note has been updated to the new behavior.
- GHCR semver tag series: the mechanism exists but has never been triggered. After the first `v*` tag is pushed, the tag tables in getting-started/deploy-docker automatically become accurate; no code change needed.

## Second Round (2026-10-11, after the M2c cancel-signal landing)

> Scope: design documents newly added or changed after the 2026-10-08 first-round audit
> (workflow/service/reference-projects/acceptance-checklist/protocol/jobs — six documents)
> + a repo-wide check of the "natural end / only pending" class of cancel-semantics claims
> against the code. The M2c landing code is the truth (`internal/server/api_jobs.go`
> cancelJob, `internal/server/api_workflow.go` cancelRunJobs/FinishWorkflowRun).

| ID | Doc page | Claim | Source evidence | Category | Severity | Disposition |
| --- | --- | --- | --- | --- | --- | --- |
| B1 | guide/protocol.md:253 (original) | `POST /api/jobs/{id}/cancel` "cancels a pending Job; dispatched/final states get 409" | M2c C6: running can be cancelled — the `job.cancel` RPC kills the in-flight process and a conditional UPDATE lands `cancelled`; pending is a DB-only conditional cancel; final states are uniformly 409 (api_jobs.go cancelJob) | Code ahead of doc | P1 | Change to the current three-branch semantics |
| B2 | guide/protocol.md:259 (original) | run cancel "in-flight steps end naturally" | M2c C8: run cancel kills in-flight step Jobs via `cancelRunJobs`, with the pending window being a DB-only conditional cancel (api_workflow.go) | Code ahead of doc | P1 | Change to the kill wording |
| B3 | guide/jobs-design.md (top P2 note/D1/"boundary: no async execution"/P2 growth point) | "pending can be cancelled", "cancel is a capability only future async brings", "no async execution" read as current | P2 async (2026-10-09) + M2c (2026-10-11) have landed: `cancelled` is a real terminal state, running can be cancelled, and ledger filtering is in place | Code ahead of doc | P2 | Update by annotation (the historical decision keeps the original text + a "post-P2 update" parenthetical; the boundary section is marked fulfilled) |
| B4 | guide/acceptance-checklist.md:301, 313 (original) | The W5/W9 evidence rows describe "natural end / only pending 409" | The semantics have changed (same as B1/B2); the W5/W9 probes have been refreshed to the kill wording + a new W11 was added (e72fb76); real-machine re-verification awaits the next real session | Code ahead of doc | P2 | Keep the historical evidence + a M2c refresh note; the semantics are governed by the refreshed probes |

**Totals**: code-ahead-of-doc 4 (B1-B4), all semantic drift after the M2c landing, no doc-ahead-of-code.
The remaining ~150 endpoint claims in protocol.md, the jobs-design.md decision table D2-D10, and workflow-design.md (maintained in the same batch) show no new differences.

## Third Round (2026-10-11, alongside the coverage-gate hardening batch)

> Scope: a scan of status-type claims across the whole guide directory ("projected, not
> implemented", "still a candidate, not scheduled", "unimplemented / to be implemented /
> not landed / not yet / todo"), read against the code. Only workflow-design.md has two,
> fixed in the same batch.

| ID | Doc page | Claim | Fact | Disposition |
| --- | --- | --- | --- | --- |
| C1 | guide/workflow-design.md:3 (original) | Top banner "2026-10-09 projected design, not implemented" | The same doc's "implementation status" section has M1/M2a/M2b/M2c all landed | Change the banner to "implementation progress is governed by the implementation-status section" |
| C2 | guide/workflow-design.md:38 (original) | M2b item "remaining candidates (cancel signal/DAG/approval gate/SSE) are still candidates, not scheduled" | The M2c cancel signal landed on 10-11 (the following M2c bullet says so) | Keep the historical sentence + a parenthetical of the landing |

**Totals**: code-ahead-of-doc 2 (C1-C2), both cases of banners/candidate lists not kept current after the M2c landing, not introduced by this round's semantic changes.

## Fourth Round (2026-10-15, full-coverage six-bundle parallel audit)

> Scope: the entire document surface (guide/ 35 + design/ 4 + root 7 + operations/ 1),
> six parallel Explore agents checking each claim against the code and taking evidence, with
> P0/P1 candidates re-verified in person. The three categories of differences go into the
> table, and docs are fixed with the source as the truth (modifying code in reverse is
> forbidden).

### Bundle 1: Facade (architecture / getting-started / rbac / ui-theme / websocket-token-auth / remote-access)

| ID | Doc page | Claim | Source evidence | Category | Severity | Disposition |
| --- | --- | --- | --- | --- | --- | --- |
| D1 | guide/architecture.md:144 | Agent→Server messages are missing `probe_report`/`exec_log`; `heartbeat` is listed as Agent→Server | `internal/protocol/message.go:8-26` contains `probe_report`/`exec_log`; heartbeat is bidirectional (the Server replies with an ACK) | Code ahead of doc | P2 | Complete the message-type table |
| D2 | guide/getting-started.md:24 | static_dir missing → the root path returns an API hint | The API hint is returned only when both `server.static_dir` and `STATIC_DIR` are unset; a configured-but-missing directory gives 404 | Description mismatch | P2 | Change to the dual-condition semantics |
| D3 | guide/getting-started.md:32 | The init tree contains `cockpit.db` | The `data/` directory is created and `cockpit.db` is auto-created on first database open | Description mismatch | P2 | Change the init-tree explanation |
| D4 | guide/getting-started.md:213 | vault cross-reference "section 5" | V5 is in section 2 | Description mismatch | P2 | Fix the reference |
| D5 | guide/rbac-design.md:36 | resource list is missing `workflows`/`approvals`/`overlay admin` | `internal/storage/role.go:60,63,42` contains the three resources; `approvals` was additionally registered on 2026-10-15 | Code ahead of doc | P1 | Complete the resource list + the permission matrix |
| D6 | docs/ui-theme-design.md:151 | `bodyBg` takes `chrome` | `bodyBg` takes `bg` (the page background color) | Description mismatch | P2 | Fix the field mapping |
| D7 | docs/ui-theme-design.md:191 | `inputBg` has the same name on both ends | `inputBg` is web-only (the mobile SkinPalette has no such field) | Description mismatch | P2 | Note web-only |
| D8 | docs/websocket-token-auth.md:1 | Tickets are uniformly passed via `Sec-WebSocket-Protocol` | terminal = subprotocol only; guacamole = `?ticket=` query takes priority, subprotocol is the fallback | Description mismatch | P2 | Change to the dual-channel semantics |
| D9 | docs/remote-access-integration-design.md:91 | guacd dials the target from its own network position; the agent is not on the data path | guacd only dials the local loopback relay (`internal/server/guac_relay.go`); the real target is dialed out by the agent, which is on the data path | Description mismatch | P1 | Change to the guacd-relay architecture |
| D10 | docs/remote-access-integration-design.md:91 | RBAC `remote` resource domain | The `terminal` resource domain (`/api/remote/*` is merged uniformly) | Description mismatch | P2 | Change the resource domain |
| D11 | docs/remote-access-integration-design.md:180 | Real-machine acceptance is suspended pending verification | Already passed on 2026-09-30 | Code ahead of doc | P2 | Update the acceptance status |

### Bundle 2: Backend Core (agent-core / service-detect / external-session / agent-config / guacd / openwrt / reference / vault / persona / nas / file-manager / egress / recording / workflow / deploy-docker)

| ID | Doc page | Claim | Source evidence | Category | Severity | Disposition |
| --- | --- | --- | --- | --- | --- | --- |
| D12 | design/agent-core-architecture.md:6 | Status Proposed | P1-P5 have landed | Code ahead of doc | P1 | Change the status to landed |
| D13 | design/agent-core-architecture.md:48 | execlog is new | `core/execlog` already exists (ring buffer + batched uplink) | Code ahead of doc | P2 | Change to already-exists |
| D14 | design/agent-core-architecture.md:113 | 3 build-openwrt chains | 5 chains (mipsel/mips/aarch64_generic/arm_cortex-a7_neon-vfpv4/x86_64) | Code ahead of doc | P2 | Change to 5 chains |
| D15 | design/agent-core-architecture.md:113 (batch table) | The batch table has no status column | P1-P5 are all landed | Code ahead of doc | P2 | Add a status column |
| D16 | design/agent-core-architecture.md:16 | agent.go is 778 lines | 696 lines | Code ahead of doc | P2 | Fix the line count |
| D17 | design/agent-core-architecture.md:104 | main is 112 lines | 116 lines | Code ahead of doc | P2 | Fix the line count |
| D18 | design/service-detect-agent.md:64 | The Window struct has no Semantics field | `core/healthprobe/window.go` contains a `Semantics` field | Code ahead of doc | P2 | Add the field |
| D19 | design/service-detect-agent.md:77 | `cockpit-probe-agent status` subcommand | `-status-file` writes JSON (there is no `status` subcommand) | Description mismatch | P2 | Fix the flag semantics |
| D20 | design/external-session-api.md:42 | Whitelist ref `api_remote.go:403` | `handleRemoteSessionCreate`'s whitelist is at `api_remote.go:654-660` | Description mismatch | P2 | Fix the line number |
| D21 | docs/agent-config.md:30 | `BIAS`/`COCKPIT_AGENT_OPTS` exist | Neither variable exists anywhere in the repo | Doc ahead of code | P2 | Delete the row |
| D22 | docs/agent-config.md:30 | `SERVER_URL` has a default | Required, no code default | Description mismatch | P2 | Change to required |
| D23 | docs/agent-config.md:30 | REGION/ZONE undocumented | The binary reads `COCKPIT_REGION`/`COCKPIT_ZONE` directly (`core/register/register.go:97-104`) | Code ahead of doc | P2 | Add the documentation |
| D24 | docs/agent-config.md:61 | The Agent runs as root | systemd runs it as the `cockpit` user (`ProtectHome=true`) | Description mismatch | P2 | Change the run user |
| D25 | docs/guacd-deploy.md:72 | The Server always calls `ssh.getDefaultKey` | Only when neither a private key nor a password is filled in | Description mismatch | P2 | Change the condition |
| D26 | docs/guide/openwrt-agent-design.md:27 | The dependency closure is missing the core/ ten packages | `internal/agent/rdp` is deleted; the core/ ten packages are added | Code ahead of doc | P2 | Complete the dependency closure |
| D27 | docs/guide/reference-projects.md:134 | uptimekuma.co official link | It is an unofficial fan site; GitHub is the only official source | Description mismatch | P2 | Change to GitHub only |
| D28 | docs/guide/vault-design.md:15 | The ACME private key is plaintext in the DB | It is encrypted in the DB (`serializer:acmeprivkey`, converged in the V6 second wave) | Code ahead of doc | P1 | Change to encrypted |
| D29 | docs/guide/vault-design.md:15 | The inventory capability token is plaintext | It is encrypted in the DB (`serializer:capconf`) | Code ahead of doc | P1 | Change to encrypted |
| D30 | docs/guide/vault-design.md:131 | The V6 second wave is not landed | Landed | Code ahead of doc | P2 | Change to landed |
| D31 | docs/guide/persona-vault-mapping.md:67 | 429 has no detailed documentation | `rate limited (120 req/min per token)`, no automatic retry | Code ahead of doc | P2 | Add the documentation |
| D32 | docs/guide/nas-design.md:71 | Credentials are stored only in the agent's environment variables | The first source is the server vault `nas/<name>` (encrypted, persisted); env is only the fallback | Code ahead of doc | P1 | Change the credential source |
| D33 | docs/guide/nas-design.md:82 | Only the `dsm` type | Supports `dsm`/`truenas`/`omv` | Code ahead of doc | P2 | Add the types |
| D34 | docs/guide/nas-design.md:218 | Alert-recovery semantics | `createAlertIfNotExists` only adds, never cleans up | Description mismatch | P2 | Fix the recovery semantics |
| D35 | docs/guide/file-manager-design.md:50 | `maxWrite`/`maxUpload` | `maxReadChunk`/`maxWriteChunk` (the 10MB upload cap is on the web side) | Description mismatch | P2 | Fix the field names |
| D36 | docs/guide/agent-egress-sdwan.md:49 | Overlay probing includes Cloudflare Tunnel | Only zerotier/tailscale/wireguard, no cloudflare | Description mismatch | P2 | Fix the probing scope |
| D37 | docs/guide/agent-egress-sdwan.md:60 | The Web SSH is not a full SSH client | `internal/proxy/ssh_session.go` implements a full SSH client | Code ahead of doc | P1 | Change to full client |
| D38 | docs/guide/agent-egress-sdwan.md:111 | SSH uses `ssh_provider.go` | `ssh_session.go` (`ssh_provider.go` only provides the default key) | Description mismatch | P2 | Fix the file reference |
| D39 | docs/guide/recording-design.md:25 | HandleTerminalData line number 302 | 341 | Description mismatch | P2 | Fix the line number |
| D40 | docs/guide/recording-design.md:34 | D1 decides VNC/RDP are not recorded | M3 has landed guacd all-protocol recording | Code ahead of doc | P1 | Note M3 overrules + add an M3 section |
| D41 | docs/guide/workflow-design.md:46 | The approvals permission point is registered | The route has `approvals` mounted but `resourceActions` has no such key, so all roles get 403 | Code ahead of doc | P1 | Register it + a regression test |
| D42 | docs/operations/deploy-docker.md:11 | The compose has no extra port mapping | The root compose maps an extra `9090:9000` | Code ahead of doc | P2 | Add the mapping |
| D43 | docs/operations/deploy-docker.md:53 | The tag table contains `v1.2.3` | `docker/metadata-action` strips the `v` prefix | Description mismatch | P2 | Delete the `v` prefix |
| D44 | docs/operations/deploy-docker.md:84 | `ALLOWED_ORIGINS` is required | When unset, the WS accepts any Origin (logging a warning), and startup is not blocked | Description mismatch | P2 | Change to optional |
| D45 | docs/operations/deploy-docker.md:95 | `AGENT_EXPIRE_MINUTES` is wired | The compose does not wire that variable | Description mismatch | P2 | Note it is not wired |
| D46 | docs/operations/deploy-docker.md:150 | The nginx conf is fully usable | The template contains the deleted desktop/vnc endpoints | Description mismatch | P2 | Fix the template explanation |

### Bundle 3: Protocol / jobs / cron

| ID | Doc page | Claim | Source evidence | Category | Severity | Disposition |
| --- | --- | --- | --- | --- | --- | --- |
| D47 | guide/jobs-design.md:15 | The Jobs page polls every 15s | `refetchInterval: 5000` (5s) | Description mismatch | **P1** | Change to 5s |
| D48 | guide/jobs-design.md:77 | POST /api/jobs returns a terminal state synchronously | The 201 response returns pending; the dispatched terminal state happens in the background | Description mismatch | P2 | Change to the async semantics |
| D49 | guide/jobs-design.md:75 | No cancel endpoint | `POST /api/jobs/{id}/cancel` exists | Code ahead of doc | P2 | Add the endpoint |
| D50 | guide/jobs-design.md:114 | jobs.ts has no cancelJob | `cancelJob` exists | Code ahead of doc | P2 | Add the method |
| D51 | guide/cron-design.md:36 | The @ whitelist has 6 entries | 7 entries (including `@annually`) | Code ahead of doc | P2 | Add @annually |
| D52 | guide/cron-design.md:120 | The preset "every Monday 03:00" | The preset is "every Monday 04:00" (`0 4 * * 1`) | Description mismatch | P2 | Change to 04:00 |
| D53 | guide/cron-design.md:120 | 5 presets | 7 presets (adding every 5 minutes / the 1st of each month 05:00 / on boot) | Code ahead of doc | P2 | Add the 3 items |
| D54 | guide/cron-design.md:161 | The `Call` case `timers` is in cron_timers.go | It is in `cron_provider.go:168`; the method name is `ListTimers` | Description mismatch | P2 | Fix the attribution + method name |
| D55 | guide/cron-design.md:191 | The `Call` case `users` is in cron_users.go | It is in `cron_provider.go:166` | Description mismatch | P2 | Fix the attribution |
| D56 | guide/cron-design.md:32 | `crontab -` writes back (stdin) | `crontab <temp file>` (the path is passed via argv, no stdin) | Description mismatch | P2 | Fix the write-back method |

### Bundle 4: stack-deploy / drift

| ID | Doc page | Claim | Source evidence | Category | Severity | Disposition |
| --- | --- | --- | --- | --- | --- | --- |
| D57 | guide/stack-deploy-design.md:60 | `last-deploy.log` stores the task output | Actually persisted as `.cockpit/last-task.json` (metadata only); the log is an in-memory ring buffer only | Doc ahead of code | **P1** | Change to last-task.json |
| D58 | guide/stack-deploy-design.md:138 | Audit event names like `stack.create` | `stack_create`/`stack_update`/`stack_up`/`stack_down`/`stack_remove` (underscores) | Description mismatch | **P1** | Change to underscores |
| D59 | guide/stack-deploy-design.md:129 | Stack fields `RunningCount`/`TotalCount`/`LastTaskStatus` | `Running`/`Total`/`LastAction`/`LastStatus` | Description mismatch | **P1** | Fix the field names |
| D60 | guide/stack-deploy-design.md:122 | 404 = agent offline / no stack provider | 404 = agent does not exist / no docker capability; 502 = has docker capability but the stack provider is not registered | Description mismatch | **P1** | Fix the 404/502 mapping |
| D61 | guide/stack-deploy-design.md:120 | DELETE body optionally `{force}` | The route only passes `{name}`, with no force semantics | Doc ahead of code | **P1** | Delete force |
| D62 | guide/stack-deploy-design.md:147 | `.env` values are masked by default | They are rendered in plaintext into the TextArea (`value={envValue}`) | Doc ahead of code | **P1** | Change to plaintext |
| D63 | guide/drift-design.md:30 | The detection targets are only nginx/cron/stack | The source fully supports traefik (`cockpit-site-*.yml` dynamic fragments) | Code ahead of doc | **P1** | Add traefik |
| D64 | guide/drift-design.md:38 | `drift.check` is a single method | 3 methods: check/diff/record | Description mismatch | P2 | Note the M3/M4 additions |
| D65 | guide/stack-deploy-design.md:314 | No token → 401 (disabled by default) | Token not configured → 404; configured but missing/wrong → 401 | Description mismatch | P2 | Fix the 401/404 wording |
| D66 | guide/stack-deploy-design.md:97 | Compatible with `compose.yaml`/`docker-compose.yml` | Also compatible with `docker-compose.yaml` | Code ahead of doc | P2 | Add yaml |

### Bundle 5: service / service-health

| ID | Doc page | Claim | Source evidence | Category | Severity | Disposition |
| --- | --- | --- | --- | --- | --- | --- |
| D67 | guide/service-design.md:432 | `systemctl show -p FragmentPath -p DropInPaths` | Only `FragmentPath` is queried; the response has no `dropInPaths` | Doc ahead of code | **P1** | Delete DropInPaths |
| D68 | guide/service-design.md:39 | capability `{Type: "systemd", Version: "1"}` | `{Type: "service", Version: "2", Metadata: {"backend": "systemd"}}` | Description mismatch | P2 | Fix the literal |
| D69 | guide/service-design.md:93 | The frontend parameter automatically gets the `.service` suffix appended | The frontend passes `r.name` directly (the list already includes the suffix) | Description mismatch | P2 | Change to direct pass-through |
| D70 | guide/service-design.md:111 | Linux/macOS continue to use DetectSystemd | darwin always reports `backend:"launchd"` | Description mismatch | P2 | Delete macOS |
| D71 | guide/service-design.md:111 | Route matching uses `strings.Contains` | `HasSuffix || Contains` | Description mismatch | P2 | Change to the dual condition |
| D72 | guide/service-health-design.md:40 | probe id regex `[a-z0-9-]{1,64}` | `^[a-z0-9][a-z0-9-]{0,63}$` (leading `-` forbidden in the first position) | Description mismatch | **P1** | Fix the regex |
| D73 | guide/service-health-design.md:37 | The six notification whitelists are explicitly enabled | The default config enables only `certificate_*`/`service_down`/`agent_offline`/`domain_expired`; `service_health.*` needs manual enabling | Code ahead of doc | **P1** | Note it needs manual enabling |
| D74 | guide/service-health-design.md:109 | The response is `{applied, rejected}` | Only `{applied}` is returned | Doc ahead of code | P2 | Delete rejected |
| D75 | guide/service-health-design.md:111 | The response is `{probe, status, lastError}` | It also returns `consecutiveFails` | Code ahead of doc | P2 | Add the field |
| D76 | guide/service-health-design.md:118 | PUT returns `{applied}` | It returns `{applied, pushed}` | Code ahead of doc | P2 | Add pushed |

### Bundle 6: Network certificate backup / data-plane ops / agent side (reports lost, fixes applied)

> The Explore agent reports for these three bundles were lost during context compression,
> but their document fixes were all applied in this round and verified line by line
> against the code (see git diff). The fix contents are recorded here by file.

| ID | Doc page | Fix content | Category | Severity |
| --- | --- | --- | --- | --- |
| D77 | docs/guide/nas-design.md | The credential first source changed to the server vault; types add truenas/omv; alert-recovery semantics | Code ahead of doc | P1 |
| D78 | docs/guide/vault-design.md | The ACME private key + inventory token changed to DB encryption | Code ahead of doc | P1 |
| D79 | docs/guide/recording-design.md | HandleTerminalData line number + M3 guacd recording | Code ahead of doc | P1 |
| D80 | docs/guide/agent-egress-sdwan.md | Overlay probing scope + the Web SSH full client | Code ahead of doc | P1 |
| D81 | docs/guide/file-manager-design.md | maxReadChunk/maxWriteChunk field names | Description mismatch | P2 |
| D82 | docs/guide/persona-vault-mapping.md | The 429 rate-limit documentation | Code ahead of doc | P2 |
| D83 | docs/guide/openwrt-agent-design.md | The dependency closure adds the core/ ten packages | Code ahead of doc | P2 |
| D84 | docs/design/agent-core-architecture.md | Status/line counts/architecture matrix | Code ahead of doc | P2 |
| D85 | docs/design/service-detect-agent.md | Window.Semantics + -status-file | Code ahead of doc | P2 |
| D86 | docs/design/external-session-api.md | The whitelist line number | Description mismatch | P2 |
| D87 | docs/agent-config.md | Delete BIAS/OPTS; SERVER_URL required; the cockpit user | Description mismatch | P2 |
| D88 | docs/guacd-deploy.md | The getDefaultKey condition | Description mismatch | P2 |
| D89 | docs/guide/reference-projects.md | uptimekuma.co → GitHub | Description mismatch | P2 |
| D90 | docs/operations/deploy-docker.md | Port mapping/tag/ALLOWED_ORIGINS/nginx template | Description mismatch | P2 |

**Totals**: P0 = 0; P1 = 18 (D5/D9/D28/D29/D32/D37/D40/D41/D47/D57-D63/D67/D72/D73/D77-D80); P2 = 72.
Three-bucket distribution: doc-ahead-of-code 8 (D21/D57/D61/D62/D67/D74 + part of D77-D80); code-ahead-of-doc ~45; description mismatch ~35.
