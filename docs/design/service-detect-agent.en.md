---
title: Service-detect agent — healthprobe standalone probing surface (target format + state machine + failure window)
---

# Service-detect Agent Brief

> Language: [中文](./service-detect-agent.md) ｜ **English**

## Status

- Status: **Active — B1-B6 all landed (2026-10-10, the B6 web surface landed as the ProbePanel on the Services page)**.
  User directive 2026-10-09: build a formal probe agent on top of core/healthprobe, brief first, then code.
- Positioning: a small standalone (probe-dedicated) agent — the same core parts + platform layer + probe business plugin, following the same three-layer discipline as [agent-core](agent-core-architecture.en.md). In standalone mode (no server) it keeps state locally; once a server is configured it uplinks the full chain via register/heartbeat/report.
- Relationship with the suspended P6 item: P6 (switching the rpc service_health engine to Liveness/Readiness/Heartbeat semantics) **stays suspended pending a decision**; this agent's failure window is the croupier "system-role probe" framing landing on a **standalone probing surface**. It does not modify the existing service_health engine — the two neither exclude nor block each other.

## 1. Target format (YAML, yaml.v3 is already a dependency)

```yaml
# /etc/cockpit-agent/probe.yaml (default path = platform Paths().ConfigDir/probe.yaml,
# overridable with -config; this is the first consumer of the platform Paths contract)
server: ws://127.0.0.1:9000/ws   # optional; omit for pure local mode
secret: ""                        # optional, registration auth
interval: 30s                     # global default interval
timeout: 5s                       # global default timeout
threshold: 3                      # global default consecutive failures before Faulty (debounce)
recovery: 2                       # global default consecutive successes before recovery
targets:
  - name: blog-http
    type: http
    url: http://127.0.0.1:8080/health
    expect_status: 200            # http only, default 200
    interval: 15s                 # overrides the global default
  - name: pg-tcp
    type: tcp
    addr: 10.0.0.5:5432
    timeout: 2s
    threshold: 2
```

- `name` is required and unique (a duplicate fails at startup); `type` is only `http|tcp` (a one-to-one mapping onto core/healthprobe's existing Checkers).
- Per-target `interval`/`timeout`/`threshold`/`recovery` override the global defaults; duration fields are parsed with `time.ParseDuration`.

## 2. State machine (debounce)

Each target gets its own independent state machine with three states:

```text
Unknown ──consecutive successes ≥ recovery──▶ Healthy ──consecutive failures ≥ threshold──▶ Faulty
   │                               ▲                              │
   └─────consecutive failures──────┼──────────────────────────────┘
                                   └────────consecutive successes─┘
```

- **Unknown is the cold-start zone**: the first result does not immediately qualify (with threshold=1 a first-round failure is Faulty).
- Consecutive counting: a result matching the current tendency accumulates, a differing result resets the counter (standard debounce).
- A state transition is the only "event": Healthy→Faulty opens a failure window, Faulty→Healthy closes it; inside a window a single failed/successful probe only updates the counter and last_error and produces no event.

## 3. Failure window data structure (queryable)

```go
// core/healthprobe/window.go
type Window struct {
    Target    string    `json:"target"`
    Semantics Semantics `json:"semantics,omitempty"` // three semantics (Liveness/Readiness/Heartbeat)
    StartedAt time.Time `json:"startedAt"`            // Healthy→Faulty moment
    EndedAt   time.Time `json:"endedAt,omitempty"`    // zero value = still in failure
    LastError string    `json:"lastError"`
}
```

- Each target has one ring retention area: at most 1 in-progress window plus 64 closed windows (a capacity constant, to bound long-running growth).
- Query surface: `Snapshot() → {target, state, since, lastChecked, lastError, open, recent []Window}`; the full `Snapshots()` feeds uplink and the status file.
- "Which period was the service unavailable" = the closed window sequence [StartedAt, EndedAt]; "is it unavailable now" = the open window.

## 4. Output surface

1. **Local**: in-memory Snapshot (`-status-file` writes JSON and the file is read back; off by default, recommended to place under `Paths().DataDir` when enabled).
2. **Uplink**: a new protocol message `probe_report` (MessageType string enum append), payload = `{targets:[{name,state,since,last_checked,last_error}], windows:[recent windows]}` — sent on each state transition + a full snapshot every 10×interval as a fallback; reuses the core/report Enqueue queue and the core/register/heartbeat registration chain.
3. **Server**: the handler receives probe_report and persists it (snapshot table + window table); REST `GET /api/probe/targets` and `GET /api/probe/windows?target=&limit=` feed the panel; hooked into the existing auth middleware.
4. **herald alert egress (slot reserved, not implemented)**: `Alerter interface { Alert(ev Transition) }` plus a noop default implementation — interface first, the delivery channel waits for a separate herald integration batch.

## 5. Three-layer placement

```text
core/healthprobe/            # ① Generic capability: existing HTTP/TCP Checker + new state.go (state machine)
                             #    + window.go (window ring) — zero business logic, zero platform specifics, independently reusable
internal/probeagent/         # ③ Business plugin: config.go (YAML parsing/validation), agent.go (scheduler:
                             #    per-target ticker + Checker + state machine + event dispatch), report.go
                             #    (uplink + status file + Alerter slot)
cmd/cockpit-probe-agent/     # main thin shell: -config/-status-file flags, standalone/server dual mode
core/platform                # ② Platform adaptation: the Paths contract for default config/status paths (first consumer);
                             #    the Signals contract for graceful shutdown (same as startcmd)
```

Dependency direction matches agent-core: cmd → internal/probeagent → core/* → internal/protocol (whitelisted exception).

## 6. Batch plan (small steps: implement + green tests + English commit)

| Batch | Content | Gate |
| --- | --- | --- |
| B1 | This brief | docs |
| B2 | core/healthprobe +state.go+window.go (state machine/window ring/event callback) + unit tests | Go gate |
| B3 | internal/probeagent config+scheduler+snapshot+Alerter slot + unit tests | Go gate |
| B4 | cmd/cockpit-probe-agent main (dual mode + platform wiring) + **local sample run** | build + real run |
| B5 | protocol probe_report + server handler/storage/REST | Go gate + server tests |
| B6 | Web surface (hooked into the existing health/resource panels) | web full gate with TZ=UTC |

## 7. Boundaries (honest list)

- Alerts only reserve the Alerter interface and implement no delivery channel (herald integration is a separate batch).
- Server storage uses sqlite (the existing AutoMigrate surface), no dedicated time-series database; the window retention cap is the same ring capacity as locally, and longer-range queries are a future need for a separate batch.
- Probing is initiated agent-side (black-box); there is no server-side distributed probe placement — multi-point placement follows the croupier framing and is discussed separately.
- The rpc service_health engine (the existing service self-healing surface) is untouched; the two probe semantics (service self-healing vs target liveness) coexist and are presented in separate sections on the panel.