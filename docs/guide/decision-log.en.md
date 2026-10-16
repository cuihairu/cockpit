# Architecture Decision Record (Decision Log)

> Language: [中文](./decision-log.md) ｜ **English**

> Established as of 2026-10-07. Once a ruling authorizes it, any item previously marked "pending
> a ruling / to be advanced when there is a clear need" is settled directly by the maintenance
> agent following the recommended option and registered here: **what was decided / why / what the
> alternatives were / when it reopens**. If the user's later review turns up a problem, it is
> changed as usual; changes are appended as new entries and old ones are never deleted.

## D-2026-10-07-1 Deleting the grdp/legacy desktop route (keep-or-drop ruling)

**What was decided**: Delete the entire surface of the in-house grdp fallback route — `internal/agent/rdp/` (the grdp client + handler),
`cap_rdp*.go` (the `-tags rdp` build split and `rdp-client` capability reporting), the agent-side
`desktop_new/desktop_data/desktop_close` message routing, the server-side `api_desktop.go`/`api_vnc.go`
(the legacy desktop/VNC WebSocket entries) and the desktop protocol message family, the orphaned web
components `DesktopModal`/`VNCModal` (mounted on zero pages) and their tests, and the `-tags rdp`
step in four workflows. **RDP/VNC functionality is unaffected** — the entry point is unified behind
GuacamoleModal (guacd + guacamole-common-js, all 16 scenarios passed real-device acceptance on
2026-09-30), and the agent only does TCP forwarding (the proxy_* message family), with no overlap
with the grdp path.

**Why**:
1. The 2026-09-24 "keep the fallback" ruling carried its own reopen condition, "evaluate it
   separately once the Guacamole approach is actually being rolled out" — that condition has been met
   (real-device acceptance closed for all three protocols, deployment docs/acceptance all checked
   off), and the re-evaluation concludes in favour of deletion;
2. `DesktopModal`/`VNCModal` are now mounted nowhere (orphans) and `hasRdpClient` has no consumer —
   the UI surface has long since been reduced to the single Guacamole entry, so "coexistence" in
   practice only leaves dead backend code;
3. The `-tags rdp` build matrix (four workflows: test/agent-test/build/nightly) plus the grdp callback
   closure coverage debt (which requires a real RDP server) is a permanent maintenance tax, and the
   only fallback scenario it buys is "the guacd container crashed" — while guacd is a documented
   first-class deployment component, keeping a second complete RDP protocol stack for a single
   container failure is disproportionate;
4. The "do not rebuild the low-level layer" boundary discipline (2026-10-04 strategy review): the
   in-house grdp/noVNC route is precisely rebuilding the low-level layer.

**Alternatives**: Keep both (the original option) — rejected for the reasons above; partial deletion
(delete only the UI, keep the agent) — rejected: in a half-deleted state the path does not work end
to end and is harder to understand. **Restore path**: the git history is fully preserved, so a single
`git revert` brings it back.

## D-2026-10-07-2 Deferring the HarmonyOS side (project-start ruling)

**What was decided**: The HarmonyOS (HarmonyOS/OpenHarmony) side is **deferred from project start**,
not permanently dropped. No HarmonyOS project is created in the repo, and no code is written blind
into main without a verification path.

**Why**: The third revision of the 2026-10-05 stance defined HarmonyOS as "just write the code, it
only has to compile", but a reality check says otherwise: ① official Flutter ships no OHOS target and
needs a third-party fork (openharmony-sig/flutter_flutter_ohos); ② there is no toolchain on this
machine and no OHOS image on GitHub hosted runners — **"it compiles" does not hold on any available
path**; ③ the repo has no existing project, and writing an application from scratch that can neither
be compiled nor verified violates the all-green discipline (unverified code is a liability the larger
it gets). The iOS side under the same stance has already closed the loop (the build-ios job was green
on its first run); what the HarmonyOS half lacks is precisely the verification path itself.

**Alternatives**: Writing code blind under the stated stance — rejected (not verifiable); standing up
a CI experimental path with the OHOS SDK — designated as the first step after a reopen rather than
something to do now (its mainline value is doubtful and it burns time easily). **Reopen triggers**
(any one is sufficient): a real need to use HarmonyOS devices appears; the openharmony-sig fork
matures enough to produce HAP stably on GitHub Actions (build the CI verification path first, then
write the code); OHOS lands in Flutter stable.

## D-2026-10-07-3 Starting firewall management (go-ahead ruling)

**What was decided**: The only unstarted item on the P2 roadmap, "firewall management
(iptables/nftables rule visualisation)", officially starts, following the existing per-agent
pattern: design document (`docs/guide/firewall-design.md`, decisions D1-Dn) → M1 read-only
observation (agent capability + ruleset snapshot RPC + a read-only three-state Web page) → write
operations evaluated in later batches.

**Why**: The user ruled to authorize advancing the entire todo list; the firewall is the last blank
spot in read-only observation on the security plane, in the same family as SMART/NAS/Overlay
(observe → alert → (long term) operate), and the capabilities are all reusable as they are (direct
Commander argv invocation, capability detection, per-agent endpoints, single action entry). The
original "advance when there is a clear need" judgement is narrowed to this: taking a look at
firewall state is a high-frequency inspection action for a personal homelab console, and it
complements the network resource panel (domains/certificates/reverse proxy).

**Alternatives**: Continuing to shelve it — rejected (the authorization explicitly advances
everything); going straight to write operations (adding/removing rules) — rejected: firewall writes
carry the highest risk level (locking yourself out of remote access), so read-only must come first
and writes can only be discussed after real-device acceptance; this is structurally identical to the
existing Cron discipline of "external entries are read-only". **Boundaries**: observe only the
current iptables/nftables state and annotations under the cockpit name; user-owned rules are not
touched. Cloud security groups (AWS SG, etc.) are out of scope.

## D-2026-10-08-1 Empty-value semantics for region/zone (todo Phase 2.2 ruling)

**What was decided**: Validation of region/zone on inventory resources and Agent registration is set
to **allow empty and mark `unknown`** — sync/registration is not rejected: an explicit field
(non-blank) is preserved preferentially; an omitted or blank value is uniformly stored as
`unknown`; `region`/`zone` keys the user set explicitly in labels are not overwritten by the empty
fields. This lands in `internal/inventory/sync.go` (the `locationOrUnknown` column plus
`applyLocationLabels` labels) and in `toStorageAgent` in `internal/server/server.go` (registration
fallback).

**Why**: In a personal homelab the region is often genuinely unknown, and a validation failure
forces the user to invent a value; the explicit `unknown` marker distinguishes "not filled in" from
"left blank for display" and aligns with the existing behaviour of `detectLocation` on the agent
side, which defaults to `unknown` (previously the resource side left blanks while the agent side
used unknown — two inconsistent stances).

**Alternatives**: Failing validation outright (reject the sync) — rejected: a single-file inventory
is maintained by hand, so a hard failure hurts usability and conflicts with the existing sync
discipline of "tolerant sync, Errors counted without aborting". **Reopen condition**: revisit when a
multi-tenant/CMDB strict-stance requirement appears.

## D-2026-10-08-2 Rejecting udp for proxy proxyType (ruling on a leftover observation from the doc-consistency audit)

**What was decided**: `proxyType` validation on `POST/PUT/PATCH /api/proxies` is tightened to
**accept only `tcp` (empty defaults to tcp); everything else returns 400** (the error body states
that the data plane is TCP only). This lands in the create and update paths of
`internal/server/api_proxy.go`; the protocol and storage comments are updated in parallel to note
"udp reserved, not implemented".

**Why**: The doc-consistency audit (docs/audit-doc-consistency.en.md A4) found that the API previously
accepted `proxyType: "udp"` (both validation and storage passed), while the Agent-side forwarding
implementation only dials over TCP (`DialTimeout` in `proxy/handler.go` hardcodes `"tcp"`) — the
user would end up with a proxy named UDP that silently forwards over TCP. Neither web nor mobile
exposes a udp entry point (no UI surface, no existing users), so failing honestly beats forwarding
silently and wrongly.

**Alternatives**: Really implementing UDP data-plane forwarding — rejected: there is no UI need and
no consumption scenario, and connectionless datagram forwarding would need its own design project,
which is not worth it; documenting only without changing behaviour — rejected: the trap lives on
forever and is rediscovered in every audit round. **Reopen condition**: when a real UDP proxy need
appears (DNS tunnelling, etc.), run a design project to implement the data plane and relax the
validation at that point.

## D-2026-10-08-3 Automatic stale-Agent detection and list hiding (closing the loop on the user's "clean up stale Agents")

**What was decided**: A set of three — ① **heartbeats write `last_seen` back to the DB**
(`dispatcher.go` handleHeartbeat calls `TouchAgentLastSeen` on every 30s heartbeat; previously it
was only persisted on registration/disconnect); ② **the Server decides staleness automatically**
(`alert_loop.go` cleanupLoop ticks every 30s running `expireStaleAgents`: fake-online rows whose
`last_seen` exceeds the threshold while still marked `online` are bulk-marked `offline` and their
registry connections are released, with the cleanup action logged); ③ **the threshold is
configurable and visible**: the `AGENT_EXPIRE_MINUTES` env var wins, `yaml agent.expire_minutes` is
the fallback, the default is 5 minutes, and a negative value turns it off; `/api/status` ships
`agentExpireMinutes` and the web Agents page shows "Stale threshold: X" at the top. The list side
**hides** stale rows by default (hidden as soon as `last_seen` exceeds the threshold, regardless of
status), with an alert saying "N hidden" plus a toggle to show them; **DB rows are not deleted** —
secrets, labels and history records are preserved, and physical cleanup goes through the existing
manual "clean up offline Agents" button (thresholdMinutes semantics unchanged).

**Why (why this was raised repeatedly without being implemented — three broken links)**: 077b10f
(10-04) only built the **manual cleanup** layer (`POST /api/agents/cleanup` + CleanupOfflineButton),
and the whole automatic detection chain was missing: (a) heartbeats **never wrote to the DB**, so
`last_seen` froze at registration time and after a Server restart every DB row stayed `status=online`
forever (fake-online ghosts, with no time evidence on the DB side to decide anything by);
(b) cleanupLoop only cleared the **in-memory registry** (closing connections after a 60s timeout),
leaving DB ghost rows with no automatic detection and nobody to clean them; (c) the threshold was
just a free-form parameter typed into a dialog, with no Server-side configuration and no UI display —
the concept of "stale" did not exist on the product surface. This batch fills in all three gaps:
(a) evidence, (b) detection, (c) visible configuration.

**The boundary between stale and deleted**: auto-deleting rows would take `secret_hash` (the Agent
secret configuration), label associations and profiles down with them — an Agent that is briefly
offline and gets removed would lose its secret and its inventory on re-registration; and
`authenticateAgentRegistration` lets rows without a secret hash through (backward compatibility), so
auto-deleting rows would open the door to ID impersonation registration. Staleness therefore only
performs "detect + mark offline + hide from the list + release resources"; the right to delete stays
with the manual button (deletion is unrecoverable, and the dialog already has a second confirmation).

**Alternatives**: Auto-deleting DB rows as soon as they go stale — rejected (secret/profile loss
plus the backward-compatible hole described above); changing only the web side and leaving the Server
alone — rejected: there is no basis on the DB side for detecting fake-online rows, so the hiding rule
has no data behind it. **Reopen condition**: if an asset-rotation policy of "automatically drop
inventory after N days offline" is needed, add an **independent retention-period setting** (by
default far longer than 5 minutes, e.g. 30 days) rather than reusing the stale threshold, and run it
as a todo project.

## D-2026-10-12-1 Starting the password vault second wave (self-ruling under inspection authorization)

**What was decided**: Start work directly on the second-wave items registered in `vault-design.md`
V6 (the user's 2026-10-12 inspection authorization permits self-ruling; hard acceptance item ① and
the hardware real-device items stay on hold): ① **add a vault source for Server API tokens** — DNS
(cloudflare api_token / dnspod login_token / alidns access_key+secret_key), overlay (zerotier /
tailscale api_token), notification (ntfy[].token / webhook[].secret / telegram[].bot_token), SMTP
(password). The yaml fields support explicit `vault:<path>` references, which the server resolves
once into the in-memory config after initVault at startup; **precedence is env > yaml > vault** (the
vault only takes effect through explicit references, with no implicit lookup by conventional path —
implicit lookup would let "whether the vault happens to hold an entry" silently change configuration
semantics); a failed reference resolution (ErrNotFound/read failure) is **fatal at startup with no
degradation** (same stance as V7; the error message contains only the path, never the content); the
entry convention is `value` = credential. ② **Encrypt the ACME private key columns at rest** (the
plaintext account/certificate private key columns in `acme_certificates` → vault core v1 ciphertext;
reads stay compatible with v0/plaintext existing rows, and all writes go through encryption).
③ **Encrypt inventory capability config at rest** (`config.*` inside the `agents.capabilities` JSON
is plaintext today and has no consumer at all → encrypt at rest rather than switching to a ref). The
jenkins external ref stays unchanged, "to be started together with the links feature".

**Why**: All three second-wave items are known debts on the M1 inventory sheet — API tokens sit in
plaintext yaml on the same privilege surface as user passwords, ACME private keys are the domain's
trust root (drag the DB file away and the private keys leak), and capability config lands in the DB
in plaintext; M1 has already converged the encryption and unlock conventions into the vault package,
so all three are pure consumer-side wiring with no new abstraction. Authorization basis: the in-repo
decision `vault-design.md` V6 "second wave (register as a follow-up)" plus the user's 2026-10-12
inspection directive permitting self-ruling.

**Alternatives**: ① Implicit backfill by conventional path (look up the vault whenever a field is
empty) — rejected: when troubleshooting "where did this token actually come from", three overlapping
sources are unexplainable, and "not configured" cannot be distinguished from "configured but the
vault has no entry"; ② switching capability config to a vault ref — rejected: the field has zero
consumers today, so a ref would be an indirection layer with no consumer; encrypt at rest first and
revisit the ref once a consumer appears; ③ moving ACME private keys into vault entries (per-cert
path) — rejected: the migration surface is large and deployment scripts (the acme.sh integration)
read the values from the row, whereas column encryption keeps the table structure with zero migration.

**Reopen condition**: once an external password vault provider lands, the `vault:` reference syntax
is naturally cross-provider (path semantics are interpreted by the provider); if "the same secret
shared in several places" appears, revisit entry aliases/inheritance.

## D-2026-10-12-2 Defining the match rule for multiple remote-control egress policies

**What was decided**: When `remote_control.egress[]` configures multiple policies for the same Agent,
the match rule is a **union** judgement — "if any policy allows both the target and the port, it is
allowed" — independent of configuration order; nothing is assembled across policies (a target allowed
by policy A × a port allowed by policy B does not constitute an allow). The Agent appears in no
policy at all → the rejection message points at `remote_control.egress` (Agent level); the target
appears in some policy but the (target, port) combination is covered by no policy → the rejection
message points at `allowed_ports` (port level). Implemented in `matchRemoteEgress` in
`internal/server/remote_audit.go`, using the `agentListed`/`targetAllowed` flags to distinguish the
three rejection states.

**Why**: The original implementation judged by "the first policy that matches the Agent decides" —
identical in the single-policy case, but with multiple policies the order decides the outcome (with A
first, a correct target/port pair is wrongly rejected; with B first, the correct pair never reaches
B), and the semantics become unexplainable; the union judgement makes the configuration semantics of
"each policy is an independent allow declaration" hold. Splitting the Agent-level and port-level
rejection messages avoids the misleading impression that "the Agent has policies configured but was
treated as if it had none".

**Alternatives**: ① First-policy-wins (order sensitive) — rejected: yaml order must not change a
security decision; ② cross-policy assembly (any policy allows the target × any policy allows the
port ⇒ allow) — rejected: a policy is meant to be "(target, port) pair authorisation", and assembly
would allow combinations nobody ever declared, widening the exposure surface.

**Reopen condition**: If policy priority is needed in future (e.g. a deny policy overriding allow),
introduce an explicit priority field; currently only allow semantics exist and the union is
sufficient.

## D-2026-10-12-3 Multi-target deployment model for ACME certificates

**What was decided**: Certificate deployment binding is upgraded from the five inline columns on
`AcmeCert` (one-to-one) to a new table `AcmeDeployTarget` (unique on `(cert_id, agent_id)`, carrying
CertPath/KeyPath/LastDeployAt/LastDeployError); a single certificate can be deployed to at most 8
agents. The API input becomes `deployTargets[]` (agentId required, absolute paths, deduplicated per
agent) and the view outputs `deployTargets[]`; the old five columns are deprecated (retained only for
rollback safety, no longer emitted in json), and migrate performs a one-time idempotent backfill that
generates target rows of identical content and clears the old columns. Deployment semantics:
`file.write` per target and status written back per target; the manual deployment endpoint returns
400 with no targets / 503 when no target is online / **502 only when every target fails** (partial
success = 200); the audit `acme_deploy` records per target (no PEM). Implemented in
`internal/storage/acme_deploy.go` + `internal/server/api_acme.go`, and the web form switches to
Form.List with multiple target rows.

**Why**: The original one-to-one binding only covers "one certificate → one gateway", whereas in
practice a wildcard certificate has to be rolled out to many reverse proxies/gateways (proxy-design.md
D7: each https site references its own path), and copying the certificate to each machine by hand is
exactly the step D14 is meant to eliminate; extending inline columns to N columns is not viable, so a
separate table is the only clean shape. Partial success does not count as failure: an error is raised
only when everything fails, so that "1 of 8 machines had a network hiccup" does not paint the whole
thing red and mislead troubleshooting; per-target status is visible both in the target row and in the
response, so no information is lost in fault localisation.

**Alternatives**: ① An inline `DeployAgentIDs []string` JSON column — rejected: it cannot carry
per-target paths and deployment status, and both queries and updates have to read/write the whole
column; ② 502 only when all targets fail vs 502 as soon as any target fails — the latter rejected: in
the automatic post-renewal deployment scenario, partial success is normal (one machine is offline),
and an overall error would disconnect inspection alerts from the actual state; ③ deleting the old
columns outright — rejected: keep one rollback-able version (if a problem shows up on the first day
of the migration, roll the binary back directly) and delete them in the next version.

**Reopen condition**: Once the rollback window has passed (and no rollback need is confirmed), delete
the five columns; if a real need appears for more than 8 targets on a single certificate, revisit
relaxing the limit or switching to target group (group) references.

## D-2026-10-12-4 Closing the loop on remote-control session lifecycle registration

**What was decided**: Wire `RemoteSessionManager` from its isolated REST registration entry into the
two live session paths, terminal and guacamole. Terminal path: `handleTerminalWebSocket` calls
`Register(pending)` after ticket validation; once proxy_new has been delivered successfully it calls
`UpdateStatus(connected)`; `closeTerminalSession` and `HandleTerminalClose` each call
`UpdateStatus(closed)`. Guacamole path: `handleGuacamoleWebSocket` calls `Register(pending)` after
the session object is created and inserts it into the global `guacSessions` index; after the
handshake completes and auditing starts it calls `UpdateStatus(connected)`; the 5 early-failure
paths each call `UpdateStatus(failed)`; `closeGuacamoleSession` calls `UpdateStatus(closed)` and
removes it from `guacSessions`. The semantics of `DELETE /api/remote/sessions/{id}` are tightened
from "delete the ledger entry" to "terminate first, then delete the entry": look up the two
data-plane indexes `terminalSessions` / `guacSessions` first; on a hit call the matching close
function (idempotent + audit + ledger close-out), otherwise fall through to the soft `Delete`.

**Why**: `RemoteSessionManager` previously had only the REST `/api/remote/sessions` POST/GET/DELETE
entry points, which the web frontend never called (zero references), and `UpdateStatus` was never
called anywhere in production code — administrators could not see from the API which remote-control
sessions were currently active, and DELETE could not actually tear down established connections.
Design document D4 explicitly requires "`RemoteSessionManager` session registration
(pending/connected/closed/failed)", and this wiring aligns ledger state with the live session
lifecycle.

**Alternatives**: ① Wiring only terminal and not guacamole — rejected: guacamole sessions are
remote-control sessions too and the management semantics should be uniform; ② DELETE removing only
the ledger entry without terminating the live session — rejected: an administrator clicking "delete"
expects the connection to drop, and leaving the live session while only deleting the ledger row
creates the false sense of security that "it is deleted but still connected"; ③ introducing a
separate session ID instead of reusing the session's own ID — rejected: the audit `Session` field,
the recording `SessionID` and the `guacSessions` index all use the session's own ID, and reuse keeps
the ledger and the data plane on the same source, so DELETE hits the same ID.

**Reopen condition**: If the web frontend later gains a session list view, a REST query permission
point has to be added (currently `terminal:write` covers only ticket issuance); if growing guacd
session volume creates memory pressure on `guacSessions`, revisit an LRU eviction policy.

## D-2026-10-12-5 Stack Git-to-deploy ① (Git worktree-isation) stance

**What was decided**: The stacks root directory as a whole is attached as a single git repo (not one
repo per stack); add two RPCs, `stack.git.status` / `stack.git.pull`, and two REST endpoints,
`GET .../git` and `POST .../git/pull`. The attach flow is
`init → symbolic-ref → remote add → fetch → checkout -B` (requires the remote branch to already
exist); pulling when already attached is `fetch --prune → merge --ff-only`, with no automatic commit
and no rebase/merge performed on the user's behalf. drift = porcelain changes in the working tree
relative to HEAD (`-uall` expanded per file), and `.cockpit/` and `.git/` are filtered in code rather
than relying on `.gitignore`. Security side: remote URLs reject the `::` transport-helper syntax, a
leading `-` and control characters; branch names must match `^[A-Za-z0-9][A-Za-z0-9._/-]{0,99}$` and
reject `..`; userinfo credentials in a URL are uniformly stripped before any echo. The remote URL is
stored only in the agent-side `.git/config` and the server creates no table; write operations are
audited as `stack_git_pull`, while status queries are not audited.

**Why**: ① Whole-repo granularity — in a personal stacks scenario there is usually just a single
repo, and per-stack repos multiply configuration cost with no real need behind them; ② No automatic
commit — under the compose-file-first principle a UI edit *is* drift, and committing on the user's
behalf would erase the notion of "local changes" and conflict with "saving is the source of truth";
③ Status stays offline — triggering a remote fetch on every list page load would turn remote failures
into UI stalls, and behind/ahead reflecting the last fetch point is enough to guide the operation;
④ Rejecting `::` — a git transport helper such as `ext::sh -c ...` is equivalent to arbitrary command
execution, and the URL is entered through a web form, so it must be blocked at the entry point;
⑤ Credential stripping — git echoes URLs containing credentials verbatim in its errors, violating the
"credentials must not land in logs" red line.

**Alternatives**: ① One git repo per stack — rejected: it doubles the configuration surface, and the
reserved `.cockpit/` directory would appear in every repo; ② Server-side webhook auto-pull —
rejected: reserved for P2 ② (webhook-triggered deployment) to be done in one place, and the blast
radius of auto-pull plus auto-restart needs its own review; ③ Replacing shell-out with the go-git
library — rejected: shell-out stays consistent with the provider's existing `docker compose`
convention, while go-git has incomplete support for sparse/worktree/credential helpers;
④ Filtering `.cockpit/` through `.gitignore` — rejected: what is being attached is the user's own
existing repo, and we have no right to edit the user's `.gitignore`.

**Reopen condition**: Revisit per-stack repos or multiple remotes when a real multi-repo need
appears (e.g. partitioning across teams); when P2 ② webhook auto-pull lands, a drift-protection
strategy review has to be added (auto-stash? refuse?).

## D-2026-10-12-6 Stack webhook-triggered deployment + streaming task log push (P2 ②) stance

**What was decided**: Add `POST /api/stacks/webhook` (a forge callback where the token is the
credential: the `X-Cockpit-Token` header is compared in constant time against the configured
`stacks.webhook_token`; an empty token disables the endpoint as 404, off by default). The semantics
only do "pull the attached origin + deploy changed stacks", and any `remote` parameter in the request
is stripped outright (a webhook must not redirect the deployment source). The body optionally takes
`{agentId, branch}`, defaulting to fanning out to every agent with an online heartbeat; the response
`{triggered:[...]}` returns immediately and the deployment runs asynchronously, with success writing
audit (username=webhook) + deployment history (mapped as up); RPC failures only land in the server
log. On the agent side the deployment scope of `stack.git.deploy` is: no HEAD before attach → all
stacks; HEAD moved forward → top-level directories of the diff; unchanged → nothing; a target stack
whose task is already running is recorded as skipped without blocking. Drift protection keeps git's
own refusal (a `--ff-only` failure errors out the whole operation, no stash, no committing on the
user's behalf) — this answers the reopen condition of D-2026-10-12-5. The streaming task log push
fully reuses the logs-design.md F4 architecture: the agent's `stack.task.follow` pushes increments
back through proxy_data (with the `stacktask:` prefix) + proxy_close returns the terminal state; the
server's NDJSON stream has a quota of 8 global / 4 per agent, a 256-frame buffer that drops frames
when full, and it sends follow.stop on disconnect; in the web StackDetail, follow replaces polling
during runtime and automatically falls back to polling when it cannot be opened.

**Why**: ① Empty token means off — a webhook is a new entry point with no authentication surface,
and enabling it by default hands public internet scanners a "trigger any deployment" button, the same
red-line stance as notification; ② Rejecting remote — the deployment source must be governed by the
agent-side `.git/config`, and accepting a remote from the request lets anyone holding the token
redirect the deployment source to an arbitrary repo; ③ Async fan-out — serial pulls across multiple
agents would drag the forge callback until it times out, while the caller only cares whether the
request was accepted; ④ No stashing for drift — auto-stashing would hide the user's uncommitted
changes and then deploy old content, with an invisible blast radius; git refusing plus the raw error
landing in the log is the only honest semantics; ⑤ Reusing F4 for log streaming — the whole follow
session/quota/backpressure/disconnect-cleanup set has already been validated on the logs path, and a
second implementation would only introduce a second set of bugs.

**Alternatives**: ① Webhook with a secret signature (HMAC payload) — rejected: in a personal scenario
the forge usually sits on the LAN, so the token header is enough, and HMAC would require configuring
a secret per forge at a high documentation cost; upgrade only when a real need appears; ② Responding
synchronously after the deployment finishes — rejected: deployments take minutes while the caller
(the forge) usually times out within 10s; ③ SSE on the UI side instead of an NDJSON fetch stream —
rejected: EventSource cannot carry an Authorization header, and keeping the same fetch-stream shape as
followLogs is more consistent; ④ Scoping the deployment by changed files down to per-file restart —
rejected: the deployment unit of a compose stack is the stack directory, so per-file granularity is
meaningless.

**Reopen condition**: A multi-forge / multi-secret need appears (upgrade to HMAC signing); large-repo
fetches time out routinely (consider an agent-side background fetch queue in place of the synchronous
RPC).

## D-2026-10-13-1 Automatically incorporating Stack ports as CMDB Service ledger rows (P2 ③) stance

**What was decided**: The inventory linkage does not use manual mapping but automatic incorporation —
service entries in the agent's `stack.list`/`stack.status` gain `ports: [{hostPort, containerPort,
proto}]` (the docker list API already carries this, so zero extra Engine calls, keeping only
hostPort>0); at the `cacheStack` sync point the server calls `syncStackServices`, upserting one
`services` table ledger row per **tcp** published port: deterministic ID
`stackport-{agentId}-{stack}-{hostPort}-tcp`, `Type=tcp`, `URL=tcp://{agentIP}:{hostPort}`,
`Labels{source:"stack", agent_id, stack}`. host takes the agent IP (registry first, falling back to
the DB profile when offline; an empty IP creates no row); an IP change refreshes the URL but preserves
probe status (the new store method `UpsertServiceTarget` touches only addressing fields, with
status/lastCheck owned exclusively by probe). Reconciliation uses two brushes: port reconciliation
runs on every sync (rows outside the set are deleted), and reconciliation of vanished stacks runs
only after a successful full list (an unreachable agent ≠ deleted). udp/sctp are displayed in the UI
but not incorporated. Zero changes on the probe side — `Type=tcp` naturally takes the existing
`parseHostPort` + `CheckTCP` path.

**Why**: ① Automatic incorporation rather than manual mapping — CMDB Service rows are currently
managed only by the inventory.yaml file (no CRUD API, read-only UI), so a manual association has
nowhere to land, while stack ports are already a machine-readable source of truth, and making users
hand-copy the YAML only creates a new drift surface; ② tcp-only — probe only implements TCP/HTTP
checks, so incorporating udp rows would manufacture permanently-down false-negative alerts;
③ Deterministic ID + labels.source — same `dns-` precedent: deletion needs no reverse lookup, and
reconciliation never touches rows declared by inventory or registered by hand; ④ Separating probe
status from addressing fields — list polling is a high-frequency write path, and a wholesale upsert
would wipe the status/lastCheck that probe just wrote, leaving probe history permanently stuck between
two polls; ⑤ Accepting the Dashboard count coupling — a stack ledger row ought to be monitored anyway,
and having ServicesUp/Down include it is the natural consequence of ledger semantics.

**Alternatives**: ① Adding a dedicated `stackPortID` foreign key field to Service — rejected: labels
are already the established extension point (the DNS precedent), and adding a column means touching
migrations; ② Having probe read the stack cache directly without going through the services table —
rejected: probe's entire enumeration/notification/result-persistence machinery revolves around Service
rows, so a second implementation alongside it brings no benefit; ③ Incorporating udp rows too, with
probe showing unreachable — rejected: the user cannot distinguish "the service is down" from "the
protocol cannot be probed"; ④ Manually selecting which ports to record via compose labels —
rejected: add that only when a real need appears; automatic full coverage plus reconciliation is
already the minimum predictable semantics.

**Reopen condition**: CMDB Service gains manual CRUD (then consider conflict rules between inventory
declarations and automatic incorporation); probe gains udp support (then open up udp
incorporation); a user requires that certain ports not be recorded (then add a compose label
blacklist).
## D-2026-10-16-1 Historical archives stay Chinese-only (stance)

**What was decided**: the 5 historical archive documents under `docs/archive/` (2 plans / 2 specs /
1 archive note) stay Chinese-only; no English versions will be produced. The corresponding todo.md
item is closed per its own unlock instruction (decided on behalf of the user on 2026-10-16, may be
overturned by the user afterwards).

**Rationale**: archive pages are historical records — their reader value lies in the original
context, not in coverage; maintaining synced translations for sealed documents is a standing cost
with no corresponding readers. The bilingual scope completed in the same batch covers active
documents only (docs root + guide/design/operations/research).

**Reopen condition**: an archived document is reactivated as an active document (moved out of
archive and translated with the next batch); or the user explicitly requests an English version
of a specific archive page.
