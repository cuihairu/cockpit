# Service Health Probes and Self-Healing

> Language: [中文](./service-health-design.md) ｜ **English**

> Project initiated 2026-09-28. Target scenario: the cloudflared tunnel on a docker VM (192.168.5.5)
> repeatedly goes into a "process alive, connection wedged" state — systemd only sees active, while
> the business has long since been unreachable; currently propped up by an external cron watchdog.
> This design makes the detection and self-healing of such "wedging that systemd cannot see" a cockpit
> built-in capability: agent periodic probes (three kinds: HTTP/TCP/systemd) + systemctl self-healing
> within a whitelist + auditing and dashboard. Along with [service-design](./service-design.en.md)
> (the three-backend execution layer) and [overlay-design](./overlay-design.en.md) M3/M3.5 (daemon
> service management), it belongs to the service management surface; see the architecture section below.
>
> The onboarding manual for the first integration case (cloudflared) is in the "First Integration Case"
> section; the user's cron watchdog is retained as a backstop and retired once the cockpit version is
> stable.

## Architecture: Service Management Surface Layers

| Layer | Carrier | Responsibility | Relation to this doc |
|----|------|------|----------|
| Execution | `service.*` (service-design three backends) | list/status/start/stop/restart/…, unit files | Self-healing actions **reuse** its restart path (same validation/timeout/argv discipline) |
| Observation | `overlay.daemon` (M3.5 read), `overlay.service` (M3.5 write) | Observing install/run state and starting/stopping networking-tool daemons | Also a wrapper "aimed at a specific service surface"; the whitelist discipline shares the same source (D27) |
| Policy | **This doc's `service.health.*`** | Probing (when to call it unhealthy) + self-healing (whether to act) + leaving traces | No new execution primitives: on top of the decision and throttling, all actions land in the execution layer |

The policy layer does not establish a separate systemctl call channel — self-healing restart goes through `ServiceProvider.DoAction(unit, "restart")`,
the same function, inheriting the unit-name whitelist, 30s timeout, and stderr-summary pass-through (D1).

## Key Decisions

| # | Decision | Choice | Rationale |
|---|------|------|------|
| D1 | Architectural placement | Mounted on the `service` provider (methods `service.health.config / health.status / health.now`), self-healing via `DoAction(unit,"restart")` | User semantics: "probes/self-healing are the policy layer of service management"; reuses the execution layer's validation and timeout, no second systemctl channel appears (same gap discipline as M3.5 D27, but the permission resource is likewise the services surface) |
| D2 | Where probing runs | **Agent-side** periodic probing (direct to the business port over the local loopback 127.0.0.1) | Wedging detection must sit right on the local machine; when the server is unreachable, probing and self-healing still proceed, with events buffered for later report (D8). The server only acts as the configuration authority and result aggregator |
| D3 | Configuration authority and delivery | Server DB `Setting` KV (`service_health.config.<agentID>`) is the sole source of truth; the agent holds only a runtime copy. The server pushes the full config over the existing `rpc_request` channel at two moments: "configuration saved" and "agent registers online" | Config-type data on Setting KV shares the mental model with the probing D1/SMART intervals; no new protocol message type; the agent discards local state for idempotence |
| D4 | Probe types | `http` (GET status code **exactly equal** to expectStatus, default 200) / `tcp` (a successful DialTimeout means alive) / `systemd` (`systemctl is-active` outputs `active`); zero standard-library-only dependencies | The cloudflared scenario needs only http (/ready); tcp backstops the "process is still listening" class; systemd backstops "what systemd can itself see as dead". All read-only |
| D5 | Decision semantics | Down is judged and self-healing triggered only after `failThreshold` consecutive failures (default 3); any single success recovers (recovered) and the counter resets | Filters out transient jitter (the minimal implementation of the same edge semantics as probing D5); recovery does not emit noisy "down again within half a cycle" events |
| D6 | Self-healing whitelist | A per-agent `whitelist[]` (*.service list) is delivered with the config; **validated on both ends**: the server refuses on save any probe with `heal=true` whose target is not whitelisted (the error text guides toward `heal=false` for alerting only); the agent validates once more before executing, and a non-whitelisted target only alerts without acting. `cloudflared.service` is the first candidate | An extension of M3.5 D27's "the whitelist must never become a gap for acting on arbitrary units" discipline: self-healing's blast radius (unattended restarts) exceeds a manual click, so it must be named explicitly; one guard on each end follows the D23 write-operation discipline |
| D7 | Restart-storm backoff | A per-unit sliding-window ledger: at most `maxRestartsInWindow` (default 3) restarts within `backoffWindowSec` (default 600) (counting failures too); when the window is exhausted only an alert fires with no action, at most one `heal_backoff` event per window | User requirement "at most Y times within X minutes"; the ledger is per unit (two probes pointing at the same unit share the quota); failed restarts also consume quota — prevents hammering a bad unit repeatedly |
| D8 | Event reporting | No new protocol message: the agent's event ring buffer (256 entries, dropping the oldest when full); the server aggregation loop polls `service.health.status` every 30s with `drain=true` (pull and clear); the dashboard GETs the same RPC but with `drain=false`, peeking without taking (so browser reads don't swallow events the aggregation loop has not yet audited); the latest state snapshot is cached in `service_health.state.<agentID>` for offline gray state | Zero protocol change (the agent has no server-directed RPC primitive); after splitting drain/peek the event's only consumer is the aggregation loop (the audit/alerting path is not raced by read paths); a 30s aggregation delay is imperceptible for a "minute-scale self-healing chain"; the polling rhythm is fixed, not configurable (probe interval is the user's knob) |
| D9 | Audit and alerting | Six event kinds (probe_down/probe_recovered/heal_restarted/heal_failed/heal_blocked/heal_backoff) are split: the three self-healing states each land in the audit as `service_health_heal / service_health_heal_failed / service_health_heal_blocked`, username=`cockpit-agent`, resourceID=`{agentID}/{unit}`, details carrying a probe-result summary; down/blocked/backoff → alert (title contains probeId, unread de-duplication, blocked/backoff are warnings, down is an error), recovered does not alert; all six kinds send a notification (`service_health.down / .up / .healed / .heal_failed / .heal_blocked / .backoff`; must be explicitly enabled in `notification.events` — the default config only enables `certificate_*`/`service_down`/`agent_offline`/`domain_expired`) | User requirement "every self-heal lands in the audit"; recovered and backoff do not enter the audit (noise classes are not audited), but backoff must be visible via "alert+notification" — an exhausted quota means self-healing has stalled, and the user must know |
| D10 | RBAC | Read uses `services:read`; config writes (PUT) and immediate probing use `services:write` | The permission-point list already has a services surface (storage/role.go resourceActions); the policy layer does not establish separate permission points; server-side PUT/check hard-checks with `userHasPerm` |
| D11 | Platform boundary | Self-healing is Linux + systemd only (`DetectSystemd()`); on non-systemd platforms a config with `heal=true` is refused on the agent side (server-side validation first). http/tcp probes share this boundary in v1 | DoAction's systemctl path is the platform boundary; Windows SCM/macOS launchd self-healing can be extended later as needed (the model exists, only the executor is missing) |
| D12 | Config validation | `id` non-empty and unique within the agent (`^[a-z0-9][a-z0-9-]{0,63}$`); `intervalSec` 5–3600 (default 30); `timeoutSec` 1–30 (default 5); `failThreshold` 1–60 (default 3); `backoffWindowSec` 60–86400, `maxRestartsInWindow` 1–10; unit names match the `*.service` whitelist regex (reusing serviceUnitNameRe); http target must be an `http(s)://` URL; tcp target is `host:port` (port 1–65535, no scheme/path) | Same rules on both ends (one guard each when the server saves and when the agent loads; an invalid whole packet is refused — config is atomic, no partial application); numeric caps prevent "a configuration that becomes a 1s-interval self-healing storm" |
| D13 | Known boundary of agent restarts | After the agent process restarts the consecutive-failure counter resets (rebuilding the threshold), and the event buffer is cleared | Probe state not persisting to disk is an intentional trade-off: agent restarts are themselves rare, and persisting introduces the new risk surface of "self-healing on stale counters"; server-side alert unread de-duplication guarantees no repeated hammering while rebuilding |

## Data Model

### Probe Configuration (server storage = delivery payload, full-replacement semantics)

```json
{
  "probes": [
    {
      "id": "cloudflared-ready",
      "type": "http",
      "target": "http://127.0.0.1:20241/ready",
      "expectStatus": 200,
      "intervalSec": 30,
      "timeoutSec": 5,
      "failThreshold": 3,
      "heal": true,
      "unit": "cloudflared.service",
      "backoffWindowSec": 600,
      "maxRestartsInWindow": 3
    }
  ],
  "whitelist": ["cloudflared.service"]
}
```

`unit` is required when `heal=true`; for `type=systemd` the target is the unit name itself (the unit field
defaults to target). The probe target itself is not constrained by the whitelist (observation is free); only
the self-healing action is constrained (D6).

### Runtime State (held by the agent, polled back by the server)

```json
{
  "states": {
    "cloudflared-ready": {
      "status": "fail",
      "semantics": "liveness",
      "lastCheck": 1759000000,
      "consecutiveFails": 3,
      "lastError": "HTTP 502 (expect 200)",
      "openWindow": { "target": "cloudflared-ready", "semantics": "liveness",
                      "startedAt": "2026-09-28T11:59:30+08:00",
                      "endedAt": "0001-01-01T00:00:00Z",
                      "lastError": "HTTP 502 (expect 200)" },
      "lastHeal": { "time": 1759000001, "unit": "cloudflared.service",
                    "result": "restarted", "detail": "exit 0", "durationMs": 812 }
    }
  },
  "events": [
    { "time": 1759000001, "probeId": "cloudflared-ready",
      "kind": "heal_restarted", "unit": "cloudflared.service",
      "detail": "consecutiveFails=3, restart exit 0" }
  ]
}
```

Since P6, state carries decision semantics and a fault window: `openWindow` non-empty ⟺ Faulty during the decision period,
`endedAt` zero-value string ⟺ still faulted; `recentWindows` are recently closed windows (agent-side
ring cap 64). Event `kind` ∈ `probe_down / probe_recovered /
heal_restarted / heal_failed / heal_blocked / heal_backoff`. `states` is a full
snapshot, `events` has drain semantics (taking them clears them).

## RPC Contract

| Method | Parameters | Result | Description |
|------|------|------|------|
| `service.health.config` | `{probes:[…], whitelist:[…]}` | `{applied: N}` | Full replacement; a validation failure **refuses the whole packet** (D12); loading drives the engine loop |
| `service.health.status` | `{}` | `{states, events}` | events drained; with no config `{states:{}, events:[]}` |
| `service.health.now` | `{id}` | `{probe, status, lastError, consecutiveFails}` | Immediately runs the specified probe once (counts toward the consecutive-failure counter and edge decision, equivalent to a scheduled trigger) |

## REST API (mounted at `/api/agents/{id}/health`)

| Route | Permission | Audit | Description |
|------|------|------|------|
| `GET /api/agents/{id}/health` | services:read | No | `{config, states, online}` — when online, pulls the agent's runtime state live; offline falls back to the cached gray state |
| `PUT /api/agents/{id}/health` | services:write | `service_health_config` | Validate (D12 + D6 whitelist) → persist to Setting KV → push if online → return `{applied, pushed}` |
| `POST /api/agents/{id}/health/probes/{probeId}/check` | services:write | No | Forwards `service.health.now` (a diagnostic action; read semantics, not audited) |

When the agent is offline a PUT still succeeds (the config is already persisted and will be pushed when it
registers online) — the response carries `pushed:false`
as a hint.

## First Integration Case: docker VM (192.168.5.5) cloudflared

1. **Deploy the agent**: install `cockpit-agent` on the VM (a systemd service, registering the
   `service` capability with the package); confirm the /services page shows this machine and `cloudflared.service`
   in the list.
2. **Choose the probe**: cloudflared ships metrics/ready endpoints (when running with `--metrics
   127.0.0.1:20241`, it is `http://127.0.0.1:20241/ready`, returning 200 only when the tunnel is connected).
   **Do not use a systemd probe as the primary probe** — "process alive, connection wedged" is exactly
   the blind spot where systemd judges active, and only the http /ready endpoint can see the business
   truth; a systemd probe can serve as a second backstop (systemd knows faster when the process is truly dead).
3. **Configure the probe**: panel /services → health probes → new: `id=cloudflared-ready`,
   `type=http`, `target=http://127.0.0.1:20241/ready`, `intervalSec=30`,
   `failThreshold=3` (≈90s detection), `heal=true`, `unit=cloudflared.service`.
4. **Whitelist**: on the same page add `cloudflared.service` to this machine's self-healing whitelist —
   the server refuses "heal=true but not whitelisted" configs before saving (D6); this is an intentional
   mid-way checkpoint.
5. **Observe a real fault**: manually `kill -STOP` cloudflared to simulate wedging (process present,
   /ready not answering), and confirm: within ~90s the probe turns fail → the audit shows
   `service_health_heal` (username=cockpit-agent) → the service is restarted → /ready
   returns 200 → a recovered event lands in the alerts.
6. **Observe backoff**: artificially create 4 faults in a row; the 4th (after the 3-quota in the window
   is exhausted) should show only a `heal_backoff` alert with no restart audit — the restart storm is
   blocked.
7. **Retire the cron watchdog**: keep it as a backstop for 1–2 weeks, comparing the cockpit version's
   self-healing success rate in the audit during the comparison period; remove the cron entry once
   stability is confirmed.

## P6: Hoisting the Decision Layer onto a Shared Probe (Three Semantics + Fault Windows)

The M1–M3 decision logic (debounce threshold, edges, recovery) was self-contained inside the `service_health.go` engine.
P6 hoists it to `core/healthprobe.Guard` (a shared component with the same source as probeagent),
leaving the engine with policy only (event edges, the two self-healing gates, the ledger).

- **Semantic mapping**: `heal=true` probes ask "is the monitored service itself alive" → Liveness
  (fault consequence = self-healing restart); `heal=false` probes ask "is the dependency ready" → Readiness
  (fault consequence = alert only). Both semantics decide identically; the difference shows only in the fault-window trace.
- **Lock ordering**: the engine holds `engine.mu` → `guard.mu`; the Guard **registers no callbacks**, so migration edges
  are derived from before/after snapshot comparison in `Record` — there is no reverse lock-holding path, no ABBA.
- **Failure streaks**: `consecutiveFails` keeps accumulating across state migrations (the true failure
  streak during a fault), and resets on success — consistent with M1 behavior.
- **recovered edge**: emitted only when the Guard judges Faulty→Healthy. Sub-threshold jitter
  (recovering before reaching the threshold) no longer emits a recovery event — D5's strict
  "any single success recovers" criterion.
- **heal gate**: during Faulty in the decision period, **every failure** passes the whitelist/backoff gates
  (each cycle during the fault attempts another rescue, until the ledger is exhausted) — D7's budget
  semantics, not just the moment of migration.
- **New wire fields** (sent up with the state snapshot, mirrored through by the server to the web): `semantics`,
  `openWindow`, `recentWindows`. `openWindow` non-empty ⟺ Faulty during the decision period;
  `endedAt` zero-value string (`0001-01-01T00:00:00Z`) ⟺ still faulted —
  `time.Time`'s `omitempty` is ineffective for structs, so front and back ends judge open/closed by the zero value.
- **web**: the state table gains a "fault window" column — an open window is pinned red at the top (`Faulted since`),
  recently closed ones use a "last N" summary + Tooltip details (start/end times/semantics/most recent failure reason).

## Implementation and Test Checklist

| Batch | Content | Tests |
|------|------|------|
| M1 (agent) | `internal/agent/rpc/service_health.go`: engine + three probe executors + self-healing (whitelist/backoff/event ring) + provider three-action wiring | Fake Commander/httptest injection: three probe decisions, threshold edges, reset on recovery, whitelist blocking, backoff window, event drain, whole-packet validation refusal, heal refusal on non-systemd platforms |
| M2 (server) | `internal/server/api_service_health.go` (API + validation + push), `internal/server/service_health_scan.go` (30s aggregation loop + audit + alerts + notifications + state cache), register-online push hook, events constants, RBAC routing | API validation matrix (invalid target/whitelist refusal), offline PUT persists without pushing, aggregation loop events→audit/alert/notification, cached gray state, online push-back |
| M3 (web) | The /services page "health probe" panel: probe list (status badge/consecutive failures/last self-heal), edit form, whitelist editing, immediate-probe button | vitest component tests |
| P6 (core+agent+server+web) | `core/healthprobe` three-semantic Guard (Liveness/Readiness/Heartbeat + fault windows); `service_health` engine decision hoisting (edge events/semantic tracing/window sync); server mirror pass-through; web fault-window column | Probe window open/close/recovery assertions, semantic marking, failure streaks accumulating across migrations, server pass-through round trip, web window-column rendering |

Acceptance boundaries (left to live hardware): real cloudflared wedging injection (steps 5/6 of the case
above), cross-machine multi-agent aggregation rhythm, GB-scale is irrelevant (this capability has no large-data
path).
