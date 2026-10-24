# P2 Design: Log Viewing (Remote journalctl / docker logs Query)


> 2026-09-15. First-phase design for P2 "Log Aggregation and Viewing". Original text in todo.md: "Agent pushes container/system
> logs, WebUI unified search (can integrate Loki or lightweight in-house development)". This document covers design decisions and interface definitions only.

## Current State and Pain Points

| Pain point | Current state | Consequence |
|------|------|------|
| "Why did the service die" requires SSH-ing machine by machine | journalctl / docker logs are command output, not files; the file manager can't cover them | Long troubleshooting path; in multi-machine scenarios you log into each host one by one |
| No entry point for systemd service logs | Workbench only has terminal/desktop/files | You don't know when a service started misbehaving or what it reported |
| No entry point for container stdout logs | The Docker page only has the container list and lifecycle operations | On container crash you must enter a terminal and run docker logs |
| cron execution failures are unfindable | cron-design.md D10 explicitly places execution history in the logs domain | Scheduled tasks fail silently (journal has cron.log) |

## Architecture Overview

Same channel model as reverse proxy/scheduled tasks: **synchronous RPC + server pure forwarding + no database**.
M1 is a query-style viewer (pull type): each query runs journalctl / docker logs live on the agent,
no collection, no storage, no push — the aggregation collection pipeline (Loki-style) is deferred to M2.

```
┌─ server ───────────────────────────┐        ┌─ agent (host where logs live) ────────────┐
│ REST /api/agents/{id}/logs         │ ─RPC─▶ │ logs provider (new)                       │
│ pure forwarding, no database       │ ◀─RPC─ │ journalctl / docker logs live query       │
│ browse-type operations not audited │        │ whitelist validation + byte cap + timeout │
└────────────────────────────────────┘        └───────────────────────────────────────────┘
```

## Key Decisions

| # | Decision | Choice | Rationale |
|---|------|------|------|
| D1 | M1 boundary | Query-style viewer (pull): the agent neither collects nor stores; queries execute commands live | In the personal scenario log queries are infrequent; live execution returning in milliseconds closes the loop; a collection pipeline + cross-machine search is a different order of magnitude, M2 project |
| D2 | Log source scope | Two classes: systemd services (journalctl) + docker containers (docker logs); file logs go through the existing file manager, no duplication | Covers the two major entry points, system-level and container-level; journal incidentally covers system logs such as cron/ssh |
| D3 | capability | `logs`: registered only if at least one of journalctl or docker is available (LookPath); status returns the availability of both | Same discipline as cron D7, no false capability; single-source hosts still work |
| D4 | Query parameters | `{type, source, tail, since_minutes, grep}`: tail defaults to 200, capped at 2000 lines; since_minutes = last N minutes (0=unlimited); grep keyword filtered in memory on the agent side | tail+since+grep covers ninety percent of troubleshooting queries; grep uses uniform in-memory contains filtering instead of journalctl -g (PCRE semantics) / docker logs which has none built in — **consistent and testable behavior on both ends** |
| D5 | Security boundary | source name whitelist regex `^[a-zA-Z0-9][a-zA-Z0-9_.@+-]{0,127}$`; output cap 1MB (truncated on overflow + truncated flag); command timeout 30s; argv passed directly, not through a shell | Defense in depth: even if the regex lets something through, injection is impossible (no shell); 1MB/2000 lines prevent a huge log from dragging down the WS; reading logs is at the same permission level as the terminal, reusing the existing agent permission model, no separate RBAC |
| D6 | Output shape | Whole-block text (string, short-iso/`--timestamps` uniformly timestamped) + truncated flag | A JSON array of lines is bulky and the frontend would just join it anyway; whole-block text rendered in pre + frontend grep highlighting suffices |
| D7 | RPC methods | Three methods: `logs.status` / `logs.sources` / `logs.query` | Minimal closed loop: availability, source enumeration, query |
| D8 | REST mounting | `/api/agents/{id}/logs/*`, reusing the serveAPI `/agents/` dispatch pattern | Scoped per agent; pure forwarding, no database (same reasoning as cron D9) |
| D9 | Audit | Query operations are not audited (browsing in nature, same discipline as file browsing) | M1 has no mutating operations; if "log download/export" is added later, revisit |
| D10 | Source enumeration implementation | systemd: `systemctl list-units --type=service --no-legend --plain` for unit names; docker: <span v-pre>`docker ps --format '{{.Names}}'`</span> (inline code has no v-pre; Go template curly braces must be wrapped manually to prevent Vue interpolation parsing) | Only lists **running** objects (we query live logs); `--all` extension added later as needed |

## Agent-Side Design

### Detection and Registration

- `DetectLogs() (journalctl, docker bool)`: dual detection via `exec.LookPath`;
  if both are missing → do not append the `logs` capability, do not register the provider;
- The provider reuses the nginx/cron `Commander` abstraction, injectable in tests.

### RPC Methods

| Method | Parameters | Returns | Description |
|------|------|------|------|
| `logs.status` | `{}` | `{journalctl, docker}` | Availability of the two source classes |
| `logs.sources` | `{}` | `{systemd: [unit...], docker: [name...]}` | Enumeration of running objects (unavailable classes return empty arrays) |
| `logs.query` | `{query}` | `{lines, truncated}` | See below |

### query Parameters and Validation (same rules on the server, both ends)

| Field | Rule |
|------|------|
| `type` | `systemd` / `docker` |
| `source` | Whitelist regex (D5); the systemd side allows suffixes like `.service` |
| `tail` | 1-2000, default 200 |
| `since_minutes` | 0-1440 (0 = unlimited, default 0) |
| `grep` | Optional, ≤256 characters, no newlines (line-by-line contains) |

### Command Construction

```
systemd：journalctl -u {source} -n {tail} --no-pager -q -o short-iso [--since=-{N}min]
docker ：docker logs --tail {tail} --timestamps [{--since Nm}] {name}
```

- Output processing: read stdout → filter by grep → truncate past 1MB to the last line boundary → `truncated=true`;
- An empty result after grep filtering is a normal outcome (`lines: ""`), not an error.

## Server-Side Design

### REST API (server/api_logs.go, mounted on the serveAPI `/agents/` branch, JWT)

| Method | Path | Description |
|------|------|------|
| GET  | `/api/agents/{id}/logs/status`  | Forward logs.status |
| GET  | `/api/agents/{id}/logs/sources` | Forward logs.sources |
| POST | `/api/agents/{id}/logs/query`   | body is query parameters → validated on both ends → forward logs.query |

- Parameter validation uses the same rules on both ends (D5); agent errors pass through as-is (502); offline → 503.

## Web UI Design

- Workbench gains a "Logs" Tab (no connection semantics, an embedded panel like "Files"):
  - **Source selection**: type Segmented (systemd services / docker containers) → specific source Select
    (from sources, linked to the type); a capability hint is shown when no sources are available;
  - **Filter bar**: recent-duration Select (unlimited/5 minutes/1 hour/24 hours), line-count InputNumber
    (≤2000), keyword input (Enter to query), query button + refresh button;
  - **Log view**: monospace pre, dark background, auto-scroll to bottom; grep hits highlighted;
    ERROR/FATAL lines red, WARN lines orange (frontend regex coloring, agent output untouched);
  - **Status line**: returned line count / truncation notice (`Truncated, showing only the last N KB`) / query duration.

## M2: Live Tail (2026-09-18)

tail -f style streaming view: the agent-side follower process continuously pushes chunks, the server forwards them to the browser via an HTTP streaming response, stopping on disconnect. The data plane reuses the ProxyData channel (structurally identical to terminal), the control plane goes over regular RPC — **zero new protocol**.

| D | Decision | Rationale |
|---|------|------|
| F1 | Channel split | **Control plane** RPC: `logs.follow.start` (`{followId, type, source, tail, grep}`, success return means started) / `logs.follow.stop` (`{followId}`); **data plane** reuses `proxy_data`/`proxy_close` messages, `proxyId = "logs:<followId>"` — structurally identical to the terminal prefix pattern, no new protocol types | RPC validation/errors/testing follow existing conventions; data is one-way and high-frequency, ProxyData is already the efficient binary path |
| F2 | agent follower process | systemd: `journalctl -f -n $TAIL -o short-iso -u $UNIT`; docker: `docker logs -f --tail $TAIL --timestamps $NAME`; bufio line-by-line scanning (line cap 1MB same as query), grep in-memory contains filtering of the same kind | Command assembly reuses the same parameter semantics as query, consistent behavior; argv passed directly, not through a shell |
| F3 | agent session limits | A single session stops at 4MB cumulative output (sends close reason=limit); sessions auto-stop after a 10-minute timeout; the follows map caps at 16; a duplicate followId start overwrites the old session (killing the old process first) | Prevents blowing up the WS and browser memory; the limits leave ample headroom for the personal scenario |
| F4 | server streaming endpoint | `POST /api/agents/{id}/logs/follow` (body same as query parameters), response is an NDJSON stream: `{"data":"..."}` data frames / `{"eof":true,"reason":"..."}` termination frame; authentication via the existing JWT middleware (fetch POST can carry headers, no EventSource needed) | fetch streaming (ReadableStream) is supported by all modern browsers; abort() disconnects; client closing → server detects ctx.Done → sends follow.stop |
| F5 | server lifecycle | `logsFollowers` registry (followId → chan + agentID); ProxyData `logs:` prefix dispatch writes; ProxyClose same prefix → stream sends an eof frame to finish; global follow cap 8 / per-agent cap 2; a failed CallAgent yields 4xx/5xx and takes no table slot | Same registry pattern as terminalSessions; caps prevent abuse |
| F6 | Semantic details | After start, the agent first pushes backfilled tail lines then keeps following (journalctl -f has this built in, docker logs -f --tail too); docker container stopping → process exits → ProxyClose reason=exited; server validation uses exactly the same rules as query (source whitelist/tail cap/grep no newlines) | tail -f mental model: backfill + follow as one |
| F7 | web | LogsPanel "Live tail" toggle: turning it on clears the current view → fetch POST streaming read (AbortController), rendering line by line (reusing GrepLine coloring and grep highlighting); while tailing, source/parameter inputs are locked; close/switch source/component unmount aborts | One-way stream + abort suffices, no WS complexity introduced |
| F8 | Audit | Tailing is not audited (same browsing discipline as queries, D9) | No mutating operations |

**Tests**: agent follow sessions (mock Commander output pushed line by line, grep filtering, 4MB cap stop, stop kills the process, followId overwrite); server endpoint (validation 400, cap 429, NDJSON frame sequence, client disconnect triggers stop RPC, ProxyClose finalization); web build.

## M3: Cross-Machine Federated Search (designed 2026-09-19)

The todo's original text: "push-based collection with cross-machine search, structured parsing". The design phase first answers the routing question — "cross-machine search"
does not have to depend on "push-based collection first":

| Route | Shape | Magnitude | Conclusion |
|------|------|------|------|
| (a) Federated fan-out search | The server forwards the **existing** `logs.query` in parallel to online agents, returning results grouped by host | Small: 1 server endpoint + 1 web page, zero agent changes, zero storage | Chosen as M3 proper |
| (b) Push-based collection + server-side indexing | agent resident tail → push → server SQLite persistence → full-text search | A separate magnitude: collection sessions, offline backfill, backpressure drops, retention cleanup, query pagination | Remains not done; trigger conditions in the "not doing" list |

Reasons for choosing (a):

- D1's "query frequency is low, live execution closes the loop" holds equally in the cross-machine
  dimension — the high-frequency troubleshooting action is
  "grep the same error on all machines at once", not "build an index over the full history";
- The earlier judgment "unified cross-machine search: depends on the collection pipeline coming first" is
  broken by the federated route: zero collection and zero storage still close the loop, fully preserving
  the D1/D8 pure-forwarding philosophy (the server still doesn't persist);
- Logs on offline machines are inherently inaccessible (the agent is the only channel); a collection
  pipeline cannot buy back the "offline machine history is searchable" capability in the personal-cloud
  scenario;
- The server does have SQLite precedents (probe_results/alert), but the log volume of 3-10 machines
  cannot justify index gains, while the ops burden (capacity/cleanup/migration) arrives first.

Structured parsing (journalctl -o json field extraction) remains not done: docker sources have no
corresponding semantics, doing it would require dual-track parsing, and the text shape + grep +
frontend coloring (M1) already suffices.

| D | Decision | Rationale |
|---|------|------|
| D11 | Global endpoint `POST /api/logs/search` (not mounted under `/api/agents/{id}`): forwards the existing `logs.query` to each target agent, one source one query per machine; tail narrowed to ≤500/machine | Reuses all of M1's building blocks and security boundaries (source whitelist/1MB truncation/30s timeout/argv direct pass); fan-out is an amplifier, 10 machines × 500 lines keeps rendering and transfer controlled |
| D12 | Targets = agents that are online with the `logs` capability; when body.agents specifies a subset, take the intersection; excluded targets return `skipped` with attribution (offline / no-logs / not-found); server fan-out concurrency cap 4, endpoint globally at most 2 in-flight requests concurrently, beyond → 429 | `Registry.List` + capability filtering are off the shelf; the double gate prevents fan-out amplification; 2 concurrent searches are ample for the personal scenario |
| D13 | A single agent failure/timeout does not fail the whole request: that machine's result `ok=false` + error text; all targets unavailable → 200 empty result + skipped explanation (not 5xx); per-target timeout 35s (agent 30s + margin) | Same discipline as NAS single-datasource degradation (D8) and logs multi-source degradation (D2); search is browsing in nature, partial success is still a valid result |
| D14 | Results returned grouped by agent, **no cross-machine time merging**; per machine `{agentId, hostname, ok, truncated, output}`, output in the same shape as `logs.query` | Machine clocks cannot be trusted; cross-machine merging is false precision; within a grouped card the single machine's timeline is naturally ordered |
| D15 | Parameter validation mirrors M1's same rules on the server side: type enum, source required and whitelist-validated before forwarding, grep ≤256 no newlines, since_minutes 1-1440, tail 1-500 (narrowed from the single-machine 2000); browsing in nature, not audited (same discipline as D9) | Consistent defense on both ends; fan-out has no write path, auditing would only spam |
| D16 | web standalone "Log Search" page (new route + menu item): form (type / source typed into AutoComplete / tail / since / grep / agent multi-select defaulting to all) → host-grouped cards + grep hit highlighting; the Workbench single-machine "Logs" Tab stays (deep-dive + live tail flow) | Cross-machine search and single-machine deep dive are two separate flows; a cross-machine source dropdown would need N sources enumerations (yet another fan-out amplifier), typed AutoComplete avoids it; coloring and highlighting reuse LogsPanel's existing implementation |

API shape:

```
POST /api/logs/search
{"type": "systemd", "source": "nginx.service", "grep": "ERROR",
 "since_minutes": 60, "tail": 200, "agents": ["a1", "a2"]}   // agents omitted = all

200 →
{"results": [
   {"agentId": "a1", "hostname": "web-1", "ok": true,
    "truncated": false, "output": "...same shape as logs.query..."},
   {"agentId": "a2", "hostname": "db-1", "ok": false, "error": "timeout"}],
 "skipped": [{"agentId": "a3", "reason": "offline"}]}
```

### Server Side

- New file `api_logs_search.go`: target filtering (capability filtering / agents intersection / skipped
  attribution) → semaphore-concurrency-4 fan-out of `CallAgent("logs.query", …)` → per-machine result
  pass-through + hostname annotation; serveAPI mounts the standalone `/api/logs/search` branch (not the
  `/agents/` prefix);
- Endpoint-level in-flight counter (atomic) ≤2, beyond → 429; per-target `context.WithTimeout(35s)`,
  a single target's failure does not spread.

### Web Side

- New route + menu "Log Search" page: `api.ts` adds `searchLogs(payload)`; after form submission,
  render host cards in `results` order (ok=false red error state, skipped gray collapsed rows),
  reusing LogsPanel's log-line coloring and grep hit highlighting inside cards;
- Form validation mirrors D15's rules up front (source required, grep length, tail/since ranges).

**Tests**: server `api_logs_search_test.go` — parameter validation table-driven (invalid type / empty
source / over-long grep / out-of-range tail / agents shape), target filtering and skipped attribution
(offline / no capability / nonexistent id), fan-out success (register 2 fake agents each returning
results), single-agent failure degradation (whole still 200), in-flight cap 429; web `npx tsc
--noEmit` zero errors + `pnpm build` + manual form-interaction verification.

### M3 Checklist

- [x] server: `api_logs_search.go` (filtering / fan-out / degradation / concurrency gate) + serveAPI integration
- [x] web: cross-machine search page (form / grouped cards / highlighting)
- [x] Tests + documentation wrap-up (checking off this list) + todo.md synced

M3 complete (2026-09-19): server 6 tests (validation 10 forms + 405 / fan-out aggregation and ordering /
no-logs skip dual forms / single-machine failure degradation / all targets unavailable 200 / in-flight
gate 429) all green; web tsc zero new errors + build passed. Implementation notes: the per-target
timeout is CallAgent's existing 30s cap (no separate 35s); an agent disconnecting deregisters from the
registry, so offline and not-found attribution merge into offline; results sorted by agentId to
guarantee stable output (registry.List iterates a map, unordered).

## Not Doing (Future Items)

- Push-based collection and server-side storage aggregation (agent resident collection → server
  persistence and indexing, or Loki integration):
  a separate magnitude; M3's federated search already closes the loop on cross-machine troubleshooting;
  only start a project when historical trend search / compliance-retention needs emerge;
- Structured parsing at the log level (field extraction from journalctl -o json): the current text
  shape suffices, and docker sources lack the corresponding semantics (reconfirmed in the M3 design);
- `--all` historical object enumeration, multi-unit joint queries, PCRE filtering: added later as
  needed.

## M1 Checklist

- [x] agent: `internal/agent/rpc/logs_provider.go` (detection/source enumeration/query/filtering/truncation)
      + `logs` capability appended + registration by capability
- [x] server: `api_logs.go` (3 endpoints + parameter validation on both ends) + serveAPI integration
- [x] web: Workbench "Logs" Tab (source selection/filter bar/colored view/status line)
- [x] tests: source enumeration parsing, query in all forms (tail/since/grep/truncated), whitelist
      validation, server forwarding and error pass-through
- [x] Documentation wrap-up (checking off this list) + todo.md synced

M1 complete (2026-09-15): agent 7 tests + server 4 tests all green; the frontend highlighting of grep
hits has the same semantics as the agent's contains filtering (case-sensitive); the Workbench "Logs"
Tab embeds in the same pattern as the "Files" Tab; hosts without the logs capability show a disabled
type + hint bar.

## References

- Internal: [cron-design.md](/guide/cron-design) (Commander + server forwarding pattern, structurally
  identical),
  [file-manager-design.md](/guide/file-manager-design) (Workbench Tab and file-log boundary),
  `todo.md` P2 log aggregation viewing item
- External: [journalctl(1)](https://www.freedesktop.org/software/systemd/man/journalctl.html)
  (-o short-iso / --since semantics), [docker logs](https://docs.docker.com/engine/reference/commandline/logs/)
  (--tail/--timestamps/--since)
