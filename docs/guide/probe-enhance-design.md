# P1 Design: Probe Enhancement (configurable probe interval + notification channel integration)


> 2026-09-14. First-phase design for P1 "probe enhancement". Prerequisite: automatic health probing is already live (`internal/probe`,
> a hardcoded 5-minute round; `internal/alert` generates alerts hourly from in-database state; `internal/notification`
> only supports the single Herald channel). Approach references: Uptime Kuma (configurable interval UI, heartbeat-style
> status, recovery notifications), Gatus (multi-channel notifier abstraction). This document only makes design decisions
> and interface definitions.
>
> **M2 added 2026-09-15**: state history heartbeat bar + configurable alert thresholds + duplicate alert deduplication,
> see the "M2" section (decisions D7 onward). The corresponding three items from the original "not doing" list moved into M2 and landed.

## Current state and pain points

| Pain point | Current code | Consequence |
|------|----------|------|
| The probe interval is hardcoded | `server.go` `probe.NewRunner(s.db, 5*time.Minute)` | to increase frequency you must change code and restart |
| Outage notifications lag | probe only persists; `alertCheckLoop` scans the database hourly to generate alerts | an outage is notified up to 1 hour late |
| A single notification channel | `notification.Client` only POSTs to Herald's `/api/v1/events` | no Herald service means no notifications |
| A fake frontend form | the three threshold inputs in `AlertSettings.tsx` have no onFinish and never took effect | misleading users |
| Duplicate alerts | the `alert.Generator.createAlertIfNotExists` comment admits "simplified handling, create directly" | an alert repeats every hour (not fixed this round, see "Not doing") |

## Key decisions

| # | Decision | Choice | Rationale |
|---|------|------|------|
| D1 | Where the interval is stored | the DB `Setting` KV table + REST + UI, not config.yaml | An Uptime Kuma-style experience: changeable at runtime, effective immediately; settings-type config in the database matches the existing Settings page mental model |
| D2 | How the interval takes effect | `atomic.Int64` inside the Runner holding seconds, the loop switching from `Ticker` to `time.After(r.Interval())` per round | A `Ticker` doesn't respond to interval changes; `After` re-reads the atomic value each round, so SetInterval naturally takes effect on the next round |
| D3 | Notification channel abstraction | a `notification.Channel` interface + a `Service` fan-out; Herald degrades to being just one channel | ntfy/webhook/Telegram are all "HTTP POST one message" just like Herald, so one interface covers everything; failures are isolated per channel |
| D4 | Where channel config lives | config.yaml (`notification.ntfy[]/webhook[]/telegram[]`) | tokens/secrets are sensitive credentials, travelling with the existing Herald config via yaml + environment variable extension; the UI only shows status read-only plus a test button, no online editing |
| D5 | Immediate outage notification | probe-side state edge detection: N consecutive failures (default 2) before counting as down, notify up on recovery | One probe round already knows the result, no need to wait for the hourly scan; N=2 filters transient network jitter (the minimal implementation of Uptime Kuma's retry semantics) |
| D6 | Event filtering | reuse `cfg.Events` (`events.<name>.enabled`), filtered uniformly by the Service | Consistent with Herald's existing event switch semantics, with all channels sharing the same switches |

## Notification channel design

### Unified payload and interface

```go
// Notification is the channel-agnostic notification payload
type Notification struct {
    EventType    string    // service.down / service.up / certificate.expired / ...
    Title, Message string
    Level        string    // info / warning / error
    ResourceType string    // service / certificate / domain / agent
    ResourceID   string
    Time         time.Time
}

type Channel interface {
    Name() string                                  // herald / ntfy / webhook / telegram
    Send(ctx context.Context, n *Notification) error
}
```

`Service`: `NewService(cfg)` builds the slice of enabled channels at startup per config; `Send(ctx, n)` first
passes the `cfg.Events` event switches, then fans out to all channels concurrently, logging per channel with
failures not affecting others; `SendNonBlocking` serves async scenarios like alerting/probing.
`TestNotification(ctx)` returns per-channel success/failure detail.

### Channel implementations (all pure HTTP, no SDK dependencies)

| Channel | Request | Config fields |
|------|------|----------|
| herald (existing) | `POST {base_url}/api/v1/events` `{type, labels}` | `base_url` / `timeout` |
| ntfy | `POST {server}/{topic}`, body = message text, headers `Title` / `Priority` / `Tags=warning,cockpit`, optional `Authorization: Bearer {token}` | `server` / `topic` / `token?` / `priority?` |
| webhook | `POST {url}` with JSON `{event_type, title, message, level, resource_type, resource_id, time}`, optional header `X-Cockpit-Secret: {secret}` for receiver authentication | `url` / `secret?` |
| telegram | `POST https://api.telegram.org/bot{token}/sendMessage` `{chat_id, text, parse_mode:"HTML"}` | `bot_token` / `chat_id` |

Each channel type is configured as a **list** (multiple topics / webhooks / chats). Credential fields only
return a target summary (such as the topic name or host) in any API response, never echoing the raw value.

### Config example (config.yaml extension, backward compatible)

```yaml
notification:
  enabled: true
  herald:
    base_url: http://herald:8080
  ntfy:
    - server: https://ntfy.example.com
      topic: cockpit-alerts
      # token: ${NTFY_TOKEN}
  webhook:
    - url: https://example.com/hooks/cockpit
      # secret: ${WEBHOOK_SECRET}
  telegram:
    - bot_token: ${TG_BOT_TOKEN}
      chat_id: "123456789"
  events:
    service-down:  { type: service.down,  enabled: true }
    service-up:    { type: service.up,    enabled: true }
    cert-expired:  { type: certificate.expired, enabled: true }
```

## Configurable probe interval

- **Storage**: the generic `storage.Setting{Key, Value}` KV table (`probe.interval_seconds`,
  default 300); `GetSetting/SetSetting` (upsert)
- **Range validation**: `[30, 3600]` seconds. The lower bound prevents self-DDoS (probes are all outbound
  from the server, so 30s is already high frequency), and the 1h upper bound means anything longer is better
  served by turning probing off
- **API** (new `api_probe.go`, `registerProbeAPI(mux)`, JWT protected):
  - `GET /api/probe/config` → `{interval_seconds}`
  - `PUT /api/probe/config` `{interval_seconds}` → validate → persist + `Runner.SetInterval` + audit
- **Runner refactor**: `NewRunner(db, interval, notifier)`; `SetInterval(time.Duration)` updates
  atomically; the loop switches `Ticker` → `time.After(Interval())` (an interval change takes effect on the next round)
- **Startup order**: `NewServer` builds the Runner (config defaults) → at `Start` read the DB to override

## Service state edge notifications (immediate probe alerts)

- The Runner holds `states map[resourceKey]*state{failCount, notified}` (only the probe loop
  goroutine accesses it, so no lock is needed);
- A service probe result of down: `failCount++`, and the first time it reaches the threshold (2) → send `service.down`
- The result recovers to up and `notified` is set → send the `service.up` recovery notification
- Only service types get edge notifications: domain/cert changes slowly and the hourly `alert.Generator` scan already covers them;
  probe notifications **do not write to the alert table** (to avoid duplicating the Generator), and the notification channel is filtered by `cfg.Events`
  (if `service.up` isn't enabled then recovery isn't notified)

## Web UI (Settings page rework)

`AlertSettings.tsx` (the fake form) is redone as a "Probing and Notifications" card:

- **Probing**: probe interval input (30–3600 seconds) + save → `PUT /api/probe/config`
- **Notification channels**: a read-only status list (type + target summary + enabled count, from `GET /api/notification/status`);
  when no channel is configured, show how to configure via yaml; a "send test notification" button → `POST /api/notification/test`,
  displaying per-channel results
- Delete the three threshold fake forms that never took effect (the certificate 7/30-day thresholds can come back when they become real config)

## Not doing (later items)

- Online editing of notification channels in the UI (yaml + environment variables are enough for personal scenarios; doing it requires solving credential encryption storage first)
- ~~Heartbeat-bar style state history UI~~ → landed in M2 (see below)
- ~~`alert.Generator` duplicate alert deduplication~~ → landed in M2 (see below)
- ~~Configurable alert thresholds (disk 80% / memory 85% / certificate 7/30 days)~~ → landed in M2 (see below)

## Phased delivery

### M1 — configurable interval + multi-channel notifications (done 2026-09-14)

- [x] notification: `Notification`/`Channel`/`Service` + ntfy/webhook/telegram implementations (the herald adapter is retained)
- [x] config: the `ntfy[]/webhook[]/telegram[]` config structures + defaults + tests
- [x] probe: Runner dynamic interval (SetInterval + `time.After` loop) + service edge notifications (2 consecutive failures debounce)
- [x] storage: the `Setting` KV table
- [x] server: `api_probe.go` (GET/PUT config + notification/status + notification/test) + route registration + audit (`probe` / `notification` resource types)
- [x] web: the Settings "Alert Settings" tab redone as probing and notifications (interval form + channel status + test notification + per-channel results); delete the three threshold fake forms that never took effect
- [x] tests: httptest for the four notification channels (including whitelist filtering, failure isolation, no credential leakage), probe dynamic interval clamping and edge notifications, all server API branches, storage KV

## M2 — state history heartbeat bar + threshold configuration + alert deduplication (2026-09-15)

> The three leftovers flagged on the P1 probing ticket are cleared in one go: M1's duplicate alerts "not fixed
> this round", the heartbeat bar "separately proposed", and the thresholds "come back when made into real config".
> All three share the same foundation (the Setting KV + the existing probe/alert structures), so landing them
> together is the cheapest.

### Gaps after M1

| Gap | Current code | Consequence |
|------|----------|------|
| No state history | `RunAllChecks` calls `UpdateServiceStatus` each round, overwriting; results aren't kept | when a service started breaking, how long it has been broken, and latency trends are all unqueryable |
| Hardcoded thresholds | probe `serviceFailureThreshold = 2` const; alert `CheckDiskSpace(80)` / `CheckMemoryUsage(85)` / `CheckExpiringCertificates(30, 7)` | an unreasonable threshold requires a code change; the hole left by M1's deleted fake forms |
| Duplicate alerts | the `createAlertIfNotExists` comment admits "simplified handling, create directly" | during a sustained outage it creates a new Alert + repeated notification every hour, flooding the alert list |

### Key decisions

| # | Decision | Choice | Rationale |
|---|------|------|------|
| D7 | History table | a new `ProbeResult` table (resource_type + resource_id + name + status + latency_ms + message + checked_at), composite index `(resource_type, resource_id, checked_at)` | The heartbeat bar takes the most recent N rows per target through the index alone; name is duplicated once for cross-table display (a service may be deleted) |
| D8 | Write scope | all three target types (service/domain/certificate) write history | Uniform table structure, consistent cost; cert/domain change slowly with only a few rows a day; the heartbeat bar UI only consumes services, the rest stays for history queries |
| D9 | Write method | `RunAllChecks` aggregates all results of the round and writes them in a batch; a write failure only logs and doesn't block the main probe flow | A few dozen rows per round is no pressure on SQLite; history is an enhancement and must not drag down the probe itself |
| D10 | Retention policy | constant 30 days; `RunAllChecks` checks each round whether ≥24h has passed since the last cleanup and then calls `DeleteProbeResultsOlderThan` | 20 targets × 5min ≈ 2M rows/year would bloat without cleanup; cleaning once a day amortises the cost to nothing; after a service is deleted its history naturally ages out, no cascade delete |
| D11 | History API | `GET /api/probe/history?resource_type=&resource_id=&limit=` (default 50, max 200, checked_at descending) | Read-only, filtered by target; reuses the `probeAPIPrefix` dispatch |
| D12 | Threshold configuration | 5 keys go into the `Setting` table: `probe.fail_threshold` (1-10, default 2), `alert.disk_percent` (50-99, default 80), `alert.memory_percent` (50-99, default 85), `alert.cert_warn_days` (1-90, default 7), `alert.cert_info_days` (1-365, default 30) | Turns every threshold from M1's deleted fake forms into a real one at once; the KV table precedent (`probe.interval_seconds`) is reused directly |
| D13 | How thresholds take effect | probe side `failThreshold atomic.Int64` + `SetFailThreshold` (same pattern as interval); alert side reads the DB once at the start of each `CheckAllChecks` round (hourly polling, no caching needed) | The probe loop only touches atomic values; the alert side reading 3 KV values hourly is negligible, saving setter synchronisation |
| D14 | Config API | extend `GET/PUT /api/probe/config`: the response body gains 5 threshold fields, and PUT saves the whole object at once (per-field validation → persist → apply → one audit) | The `interval_seconds` field semantics are unchanged and backward compatible; the UI is one card with one save, and splitting into two endpoints would be awkward |
| D15 | Alert deduplication | `createAlertIfNotExists` becomes real deduplication: if an **unread** alert with the same `(resource_type, resource_id, title)` exists, skip creation and skip notification; storage gains `HasUnreadAlert` | Unread means "not yet known to the user"; after the user marks it read, the same problem recurring produces a new alert and a new notification, matching the intuition "read = handled, recurring is worth reminding about" |
| D16 | Heartbeat bar UI | a `HeartbeatBar` component: colour blocks for the last 30 rounds (green up / red down / grey no data), with a Tooltip showing time + latency + message; mounted as a new "Recent status" column on the Resources page service table | The minimal implementation of Uptime Kuma's signature experience; the service list is the highest-frequency entry to status, with a detail modal added later |

### Data model

```go
// storage.ProbeResult is the probe history (one row per probe round)
type ProbeResult struct {
    ID           string    `gorm:"primaryKey" json:"id"`
    ResourceType string    `gorm:"index:idx_probe_target,priority:1" json:"resourceType"`
    ResourceID   string    `gorm:"index:idx_probe_target,priority:2" json:"resourceId"`
    Name         string    `json:"name"`
    Status       string    `json:"status"`     // up / down / degraded
    LatencyMs    int       `json:"latencyMs"`
    Message      string    `json:"message"`
    CheckedAt    time.Time `gorm:"index:idx_probe_target,priority:3" json:"checkedAt"`
}
```

New in storage: `CreateProbeResults([]ProbeResult)` (batch),
`ListProbeResults(resourceType, resourceID string, limit int)` (the most recent N in reverse order),
`DeleteProbeResultsOlderThan(cutoff)`, `HasUnreadAlert(resourceType, resourceID, title string)`.

### probe/alert refactor

- Runner: `failThreshold atomic.Int64` (default 2, overridden by reading the Setting at startup);
  `trackServiceEdge` reads the atomic value; each `RunAllChecks` round batch-writes history at the end + cleans up on demand;
- Generator: `CheckAllChecks` starts with `loadThresholds()` reading the three alert keys from the DB (falling back
  to defaults), passing them to the existing `CheckDiskSpace/CheckMemoryUsage/CheckExpiringCertificates`
  (the signatures are already parameterised, only the call sites change);
- `createAlertIfNotExists`: checks `HasUnreadAlert` before creating and returns on a hit (no Alert created,
  no notification sent); all 6 check families benefit automatically (service/agent/disk/memory/certificate/domain).

### REST API changes

| Method | Path | Notes |
|------|------|------|
| GET  | `/api/probe/config` | `{interval_seconds, fail_threshold, disk_percent, memory_percent, cert_warn_days, cert_info_days, min/max...}` |
| PUT  | `/api/probe/config` | the whole object; per-field range validation → SetInterval/SetFailThreshold + persist → one audit |
| GET  | `/api/probe/history` | `resource_type` + `resource_id` mandatory, `limit` default 50 max 200 |

### Web UI

- A new "Recent status" column on the **Resources** service table: `HeartbeatBar` (30 cells) + the current status colour;
  data comes from `GET /history?resource_type=service&resource_id=...` pulled per row (table row counts run from
  single digits to tens, acceptable); the Tooltip shows `MM-DD HH:mm · 123ms · message` per cell;
- The **Settings** probing and notifications card: a threshold form area next to the interval input (consecutive failure count, disk %,
  memory %, certificate warning/reminder days), saved in one `PUT /config`.

### M2 checklist

- [x] storage: the `ProbeResult` table + AutoMigrate + `CreateProbeResults/ListProbeResults/
      DeleteProbeResultsOlderThan` + `HasUnreadAlert`
- [x] probe: failThreshold atomic (Setting read + SetFailThreshold) + history written each round +
      24h retention cleanup
- [x] alert: thresholds read from Setting (disk/memory/certificate) + `createAlertIfNotExists` unread deduplication
- [x] server: `/api/probe/config` extended (GET/PUT full thresholds) + `/api/probe/history` + audit
- [x] web: the `HeartbeatBar` component + the Resources service column (ProbeHeartbeatCell per-row pull) +
      the Settings threshold form
- [x] tests: storage history CRUD/deduplication queries, probe threshold effectiveness and history write/cleanup, alert deduplication
      (skip on repeat + re-notify once read + threshold tiers), server config extension validation + history branches
- [x] documentation wrap-up (this checklist ticked) + todo.md synced

## Config file field (probe.interval, added 2026-10-12)

M1 made the probe interval configurable in the UI (the Setting table's `probe.interval_seconds`), and M3
added a config file entry so the interval can be declared with deployment orchestration (IaC / container env
rendering yaml) without having to log into the UI first:

```yaml
probe:
  interval: 5m   # probe interval, default 5m; out-of-range values are clamped to [30s, 1h]
```

| Field | Type | Default | Notes |
|------|------|------|------|
| `probe.interval` | duration | `5m` | The cycle of a full probe round. Lower bound 30s (prevents self-DDoS), upper bound 1h (better to turn probing off), out-of-range values clamp to the boundary (consistent with `Min/MaxIntervalSeconds` in internal/probe; the config package can't import probe in reverse through the notification dependency chain, so the constants are mirrored on the config side). |

**Value precedence**: the UI-saved value in the DB (`probe.interval_seconds`, effective immediately on save) >
the config file's `probe.interval` (read once at startup) > the built-in default 5m. `startProbeRunner` follows
this chain: the yaml value only acts as a baseline when there's no UI-saved value; once the UI has saved, the
yaml declaration is overridden — the UI is a later, more explicit statement of runtime intent, and an
orchestration declaration shouldn't overturn an operator's manual adjustment in the interface.

**Cross-check against the notification egress red lines** (the notification channel config is in the notification section of `config/cockpit.yaml`):
`notification.enabled` defaults to false — not configured means zero egress; the channels (herald/ntfy/webhook/telegram)
are built only from explicit yaml entries; the event whitelist `events` denies everything by default (`IsEventEnabled` returns false with no match);
the list of enabled channels is made explicit through `GET /api/notification/status` (without credentials).

## References

- [Uptime Kuma](https://github.com/louislam/uptime-kuma) — interval UI configuration, retry debounce, recovery notifications
- [Gatus](https://github.com/TwiN/gatus) — multi-notifier abstraction and alert event naming
- Internal: [protocol definitions](/guide/protocol), the P1 probe enhancement item in `todo.md`, `internal/probe/runner.go`