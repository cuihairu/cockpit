# P2 Design: Scheduled Task Management (Crontab Visualization)


> 2026-09-15. First-phase design for P2 "Scheduled Tasks / Cron Management". Original text in todo.md: "Agent-side crontab
> visualization and scheduling, supporting automated ops". This document covers design decisions and interface definitions only.

## Current State and Pain Points

| Pain point | Current state | Consequence |
|------|------|------|
| Adding a scheduled task requires SSH and hand-editing crontab | Hand-writing expressions with `crontab -e`, no way to verify them after writing | A wrong expression silently never runs, discovered only much later |
| No way to see what scheduled tasks exist on hosts | Scattered across crontabs / scripts on each host | Half a year later nobody knows which cron does what, or whether it is still in use |
| Editing crontab is nerve-racking | Full-replacement write-back; one slip deletes someone else's entries | User-handwritten entries get clobbered with no backup |
| Cockpit has no automation entry point | Backup scheduling is a server-side private implementation, zero capability on the agent side | "Automated ops" is out of the question |

## Architecture Overview

Same channel model as reverse proxy: **synchronous RPC + server pure forwarding**. crontab is KB-scale text,
reads/writes take milliseconds, a 30s RPC timeout is more than enough.

```
┌─ server ────────────────────┐        ┌─ agent (host where crontab lives) ─────────────────┐
│ REST /api/agents/{id}/cron  │ ─RPC─▶ │ cron provider (new)                                │
│ task CRUD + audit           │ ◀─RPC─ │ read all → replace own section → write back        │
│ no DB: crontab is the state │        │ user-written entries untouched (kept line by line) │
└─────────────────────────────┘        └────────────────────────────────────────────────────┘
```

## Key Decisions

| # | Decision | Choice | Rationale |
|---|------|------|------|
| D1 | M1 scope | Only **crontab** (agent's running user, read with `crontab -l` / write back with `crontab <tmpfile>`, argv passes the file path with no stdin); systemd timer read-only listing comes later | The vast majority of personal-cloud scheduled tasks live in crontab; multi-user `-u` and timer listing can be added later as needed |
| D2 | Management boundary | cockpit only touches its own entries: each task is two lines (a meta comment line + a command line), write-back = read full content → replace only the cockpit section → all other lines **preserved line by line verbatim**; before writing back, self-check that non-cockpit lines match the old content, refuse to write if they differ | crontab is full-replacement write-back, there is no fragment file to use; the self-check backstop guarantees user-handwritten entries are never clobbered |
| D3 | Task metadata | meta comment line `# cockpit:job {json}` embedded (containing name/schedule/command/enabled), read-back treats meta as authoritative | Structurally identical to nginx `# cockpit:meta`; the roundtrip works by parsing its own rendered output |
| D4 | Task model | `{name, schedule, command, enabled}`; when enabled=false the command line is rendered as a comment (disabling without losing the entry) | Covers personal scenarios; disabling is a high-frequency need (temporarily off rather than deleted) |
| D5 | Expressions | 5-field standard cron + `@reboot/@hourly/@daily/@weekly/@monthly/@yearly/@annually` whitelist; both ends validate with the same rules (field ranges + syntax) | Immediate frontend feedback; agent-side defensive backstop |
| D6 | Execution model | Fully synchronous RPC + provider-level mutex serializing read-modify-write | Operation frequency is extremely low, async tasks would be over-engineering; the mutex prevents concurrent server applies from interleaving writes |
| D7 | capability | `cron`, detected via `exec.LookPath("crontab")`; agents without it installed show no false capability | Same pattern as nginx-proxy |
| D8 | REST mounting | `/api/agents/{id}/cron/...`, reusing the serveAPI `/agents/` dispatch pattern | Scoped per agent |
| D9 | Persistence | **No database** — the agent-side crontab is the single source of truth, the server purely forwards + audits | No state synchronization problems; apply/delete are audited and traceable |
| D10 | Execution history | Not in M1 (cron has no built-in execution records; collecting them requires wrapper scripts) | Belongs to the log aggregation domain, separate project |

## Agent-Side Design

### Detection and Registration

- If `exec.LookPath("crontab")` succeeds, append the `cron` capability (providers.go registers by capability);
- The provider reuses nginx's `Commander` abstraction to run `crontab -l` (read) / `crontab <tmpfile>` (write back), injectable in tests.

### RPC Methods

| Method | Parameters | Returns | Description |
|------|------|------|------|
| `cron.status` | `{}` | `{user, cockpitCount, externalCount}` | Overview: running user + entry counts |
| `cron.jobs` | `{}` | `{jobs: [...], external: "..."}` | List of tasks under cockpit's name + raw text of external entries (read-only display) |
| `cron.job.apply` | `{job}` | `{name}` | Validate → read full content → replace cockpit section → self-check → write back |
| `cron.job.delete` | `{name}` | `{}` | Remove the task from the cockpit section → self-check → write back |

### Task Parameters and Validation (same rules on both ends)

| Field | Rule |
|------|------|
| `name` | `^[a-z0-9][a-z0-9_-]{0,63}$` (same as proxyNameRe) |
| `schedule` | 5-field cron (minute 0-59 / hour 0-23 / day 1-31 / month 1-12 / dow 0-7, supports `* , - /`) or `@` whitelist |
| `command` | Non-empty, ≤4KB, **contains no newlines** (crontab single-line semantics) |
| `enabled` | bool |

### Rendering and Write-Back Flow

```
Render (two lines per task):
  # cockpit:job {"name":"backup","schedule":"0 3 * * *","command":"/opt/bk.sh","enabled":true}
  0 3 * * * /opt/bk.sh
  (when enabled=false the command line is rendered as #0 3 * * * /opt/bk.sh)

apply/delete:
  crontab -l reads the full content (an empty crontab counts as an empty string)
    ↓
  Replace the cockpit section: scan the meta marker lines to locate each task pair, replace/delete the whole pair;
  new tasks are appended to the end of the section; non-cockpit lines keep their original order and content
    ↓
  Self-check: the non-cockpit lines of the old and new content must match line by line, otherwise refuse to write back (bug defense)
    ↓
  crontab <tmpfile> writes back (argv passes the path, no stdin)
```

- The read-modify-write critical section is serialized by a provider-level `sync.Mutex`;
- External entries (non-cockpit lines) are passed through verbatim to the UI for read-only display — part of the "visualization".

### cron Expression Validator

- `@`-prefixed goes through the whitelist (7 entries), otherwise split into 5 fields on whitespace;
- Each field matches comma-separated combinations of `^(\*|\d+)(-\d+)?(/\d+)?$`, numeric tokens are validated against the field range
  (dow also accepts 0-7, with 7 treated as 0);
- Both ends implement the same rules (Go `validateCronExpr` / TS `validateCronExpr`).

## Server-Side Design

### REST API (server/api_cron.go, mounted on the serveAPI `/agents/` branch, JWT)

| Method | Path | Description |
|------|------|------|
| GET  | `/api/agents/{id}/cron/status` | Forward cron.status |
| GET  | `/api/agents/{id}/cron/jobs` | Forward cron.jobs (including external raw text) |
| PUT  | `/api/agents/{id}/cron/jobs/{name}` | body is task parameters → cron.job.apply + audit cron_apply |
| DELETE | `/api/agents/{id}/cron/jobs/{name}` | cron.job.delete + audit cron_delete |

- Parameter validation uses the same rules on both ends (D5); agent errors pass through as-is (502);
- Audit: `ResourceCronJob = "cron_job"`, actions cron_apply / cron_delete,
  detail records a name/schedule/command summary (the command is the user's own ops content, safe to record in full).

## Web UI Design

- New page `/cron` "Scheduled Tasks" (menu, ClockCircleOutlined):
  - **Agent selection**: dropdown; agents without the cron capability are disabled with a hint;
  - **Status card**: running user / cockpit task count / external entry count;
  - **Task list** Table: name / expression (monospace) / command (ellipsis to expand) / status
    (enabled Tag / disabled Tag) / actions (edit, enable/disable Switch, delete Popconfirm);
  - **Create/Edit Modal**: name, expression (common preset dropdown: every minute / every 5 minutes / hourly on the hour / daily at 03:00 /
    Mondays at 04:00 / the 1st of each month at 05:00 / at boot (@reboot), auto-filled on selection), command TextArea (monospace, validates no newlines), enable switch;
  - **External entries** collapse panel: read-only pre showing the raw lines not under cockpit's name (visible but not editable).

## M2: Next-Trigger Preview (2026-09-18)

The task list is upgraded from "when it was configured" to "when it will run": the agent computes the next trigger with
a self-written cron parser, returned alongside the `cron.jobs` response, zero server changes.

| # | Decision | Content | Rationale / Notes |
|---|------|------|------------|
| D14 | Computation happens on the agent | `next_run` is computed by the agent using the server's local timezone (`time.Now()`), returned as unix seconds | cron trigger semantics are bound to the server timezone — computing in the browser's timezone is necessarily wrong; the server purely forwards, unchanged |
| D15 | Self-written parser | `cron_next.go`: reuses the syntax already defined by `validateCronExpr` (`cronFieldRe` field shape + `cronFieldRanges` ranges + @ whitelist) — whitelist-style validation guarantees the expression forms are bounded, no need for Robfig's superset with seconds/year fields; @ shorthands expand to equivalent 5-field forms (@hourly=`0 * * * *`, @daily=`0 0 * * *`, @weekly=`0 0 * * 0`, @monthly=`0 0 1 * *`, @yearly/@annually=`0 0 1 1 *`); @reboot returns 0 (no next trigger) | No third-party library: the supported expression domain is a subset of our own validator; parser and validator evolve from the same source |
| D16 | Trigger-determination semantics | Fields expand to bitsets with minute-by-minute scanning (upper bound 5 years, 2.62 million integer comparisons <10ms, task counts from single digits to hundreds); the dom/dow union follows the Vixie cron rule — **OR when both are restricted** (`0 0 1 * 1` = the 1st of each month OR every Monday), **AND when either is `*`**; dow 7 normalizes to 0 (Sunday) | Vixie union semantics are the de facto standard of crontab(5); missing it guarantees wrong results; minute-by-minute scanning is simple and correctness-first |
| D17 | Presentation | `Jobs()` adds `next_run` to each job (unix seconds; disabled=0 not computed, parse failure=0 not fatal — invalid expressions hand-edited externally do not affect the list); web task list gains a "next trigger" column: disabled → "Disabled", @reboot → "At boot", 0 → `-`, otherwise local time | Returned alongside the list RPC, zero extra round trips |

### Agent Side

- `cron_next.go`: `parseCronSchedule` (expression → bitsets) + `nextCronRun`
  (minute-by-minute scan starting from the next whole minute);
- `Jobs()` integration: compute `next_run` for each enabled task.

### Web Side

- "Next trigger" column in the Cron page task list (`dayjs.unix` local format YYYY-MM-DD HH:mm).

**Tests**: agent — every minute/@daily/@reboot/step ranges/OR semantics/AND semantics/leap year Feb 29/
dow=7/invalid expression 0/disabled not computed; web — tsc + build.

## M3: systemd timer Read-Only Listing (2026-09-18)

The second scheduling mechanism beyond crontab. Read-only preview: list schedules and trigger times of
system timer units, no enable/disable or editing (the write path involves the root boundary, not done).

| Decision | Content | Rationale |
|------|------|------|
| D18 | Listing and querying use stable APIs: `systemctl list-unit-files --type=timer --no-legend` takes the **first column** of each line (unit name, independent of column count), then **one command** `systemctl show <all unit names> -p Description -p ActiveState -p UnitFileState -p LastTriggerUSec -p NextElapseUSecRealtime -p TimersCalendar -p TimersMonotonic` queries properties, output split into sections by blank lines, each section `Key=Value` | `list-timers` table-style output columns vary across systemd versions (the root cause of the earlier "not doing it"); `show -p` property names are a stable bus API, verified in practice: two stages, one exec each |
| D19 | Schedule taken as raw text without semantic parsing: the `OnCalendar=` raw text of `TimersCalendar` takes priority, otherwise concatenate the lines of `TimersMonotonic` (OnBootUSec/OnUnitActiveUSec); `NextElapseUSecRealtime`/`LastTriggerUSec` have the trailing timezone abbreviation stripped and are parsed as unix seconds in the **host's local timezone** (`@` prefix = epoch microseconds parsed directly; `n/a`/empty/parse failure map to 0); units where show fails (e.g. template `@.timer`) are listed by name only, properties left empty | systemd's OnCalendar syntax superset is too large to hand-write a parser; systemctl and the agent share the same host and TZ, wall-clock strings match `time.Local`; template units error in practice on show, the fallback is not fatal |
| D20 | Presentation and read-only boundary: agent cron provider new action `timers` (pure read, capability unchanged — the entry is mounted on the Cron page); server `GET /api/agents/{id}/cron/timers` pure forwarding, no database, no audit; web Cron page adds a "systemd timers" collapse panel (unit/description/schedule/state/last trigger/next trigger, keyword filter, panel not shown on request failure or empty list, times in `dayjs.unix` local format) | The listing is the same nature as the external-entries collapse panel; read-only involves no audit or capability semantic changes |

### Agent Side

- `cron_timers.go`: `ListTimers` (two execs + sectioned parsing + time parsing
  `parseSystemdTime`); `Call` adds case `timers` (in `cron_provider.go`).

### Web Side

- Cron page "systemd timers" collapse panel (collapsed by default; not rendered when empty/failed).

**Tests**: agent — time parsing of all forms (UTC/local suffix/`@`epoch/`n/a`/garbage),
sectioned parsing (two units + skip bad sections + template units listed by name only), raw schedule extraction
(calendar and multi-line monotonic); server — forwarding; web — tsc + build.

## M4: Multi-User crontab (-u) (2026-09-19)

The first item of the "not doing" list becomes a project. Motivation: scheduled tasks of service users
(e.g. www-data cleanup scripts, postgres maintenance tasks) all live in **that user's crontab**, currently
manageable only by SSH-ing in and hand-editing; `-u` is crontab's native semantics, the agent can manage it
by passing through via argv, with a clean permission boundary.

| Decision | Content | Rationale |
|------|------|------|
| D21 | user parameter semantics and security boundary: the 4 crontab-related actions (`status`/`jobs`/`job.apply`/`job.delete`) gain an optional `user` parameter — default empty = current running user (zero breakage, backward compatible); validated as `^[a-z_][a-z0-9_-]{0,31}$` (case-sensitive, rejects weird forms and over-length, does not accept numeric uid forms); exec is always an argv array (`crontab -u <name> -l`), never builds a shell; for non-root agents crontab's own "must be privileged to use -u" surfaces as-is | `-u` gets no invented semantics; the name whitelist regex is narrow enough, zero injection surface; the permission model = the crontab command's own boundary (the agent is limited by whoever it runs as), no new privilege escalation invented |
| D22 | New action `users` for user enumeration: `getent passwd` preferred (NSS compatible, covers LDAP/SSSD), on missing command or execution failure falls back to parsing `/etc/passwd` (same line format); per line take `name`/`uid`/`shell` and return the full set, **no nologin filtering** (service users often have nologin yet are exactly the owners of crontabs) | Enumeration is dropdown usability, not a security boundary (security relies on D21 validation + crontab's own permissions); the dual-path fallback covers OpenWrt busybox which has no getent |
| D23 | server API and audit: new `GET /api/agents/{id}/cron/users` (pure forwarding, no audit, same treatment as timers); the four endpoints status/jobs/PUT jobs/DELETE jobs gain an optional `?user=` query parameter (same D21 regex validated on both ends, invalid → 400); cron_apply/cron_delete audit details record `user` (written only when explicitly specified — default stays empty, historical audit semantics unchanged) | Browse-type actions are not audited; audits carrying user answer "who changed postgres's crontab"; default-empty keeps existing audit rows unchanged |
| D24 | web UI: a "User" Select next to the agent selector on the Cron page — default "current user" (empty value), options from the users enumeration (showSearch allows typing any valid name, **enumeration failure does not block manual input**); switching user re-queries status+jobs; the timers panel is unaffected (systemd is global, no user dimension) | Enumeration is a UX enhancement, not a functional prerequisite; the panel layout has zero conflict with the existing status card/task table |
| D25 | cockpit section isolation holds per the (user, name) tuple: `readCrontab`/`writeCrontab`/`writeViaFile` gain a user dimension pass-through, splitCockpit/upsertJob/removeJob/**pre-write-back self-check** logic unchanged; the meta JSON gains no user field (which user's crontab the section lives in is itself the ownership) | The "only manage task pairs under our own name" semantics holds independently per user; the protective guard of the pre-write self-check on non-cockpit line equality naturally carries over across users |

### Agent Side

- `cron_provider.go`: each action in `Call` takes `params["user"]` → `validateCronUser`
  → `readCrontab(user)` / `writeViaFile(content, user)` (argv assembles
  `-u <name>`); status's whoami is returned only in the default view (when a user is specified that field is the user).
- `cron_users.go`: `Users()` (getent preferred → /etc/passwd fallback) +
  `Call` adds case `users` (in `cron_provider.go`).

### Server Side

- `api_cron.go`: `cronUserRe` validation + `?user=` extraction pass-through on the four endpoints + the `users`
  endpoint + audit details `user`.

### Web Side

- Cron page "User" Select + `api.ts` methods gain an optional `user` parameter + `cronUsers()`
  + `CronStatus`/task requests carry user; types add `CronUserEntry`.

**Tests**: agent — user validation (invalid-form matrix), `-u` argv assembly (fake
Commander asserts the argument sequence), getent parsing and /etc/passwd fallback, full chain of jobs/apply/delete
for a specified user; server — `?user=` validation (invalid 400/valid pass-through/
default not sent), users endpoint forwarding, audit details carrying user (default does not); web —
tsc zero new errors + build.

## M4 Checklist

- [x] agent: `cron_provider.go` user parameter (validated and rejected right at the Call entry) + user dimension
      on reads/writes + StatusAs skips whoami for a specified user; `cron_users.go` enumeration
      (getent preferred / /etc/passwd fallback)
- [x] server: `GET /cron/users` no audit + four endpoints `?user=` validated with the same rules on both ends
      + audit details record user (only when explicitly specified)
- [x] web: types + api user parameter/getCronUsers + Cron page target-user
      AutoComplete (enumerated dropdown + manual input)
- [x] tests: agent 8 new tests + server 3 new tests all green (agent/server full packages
      pass -race); web tsc zero new (baseline 9 unchanged) + build passed
- [x] Documentation wrap-up (checking off this list) + todo.md synced

M4 complete (2026-09-19). Post-implementation notes: ① the "Select" in D24's wording landed as
**AutoComplete** — enumeration serves as a suggestion dropdown, any valid name can be typed directly, which exactly
satisfies the same decision's "enumeration failure does not block manual input" requirement; ② the agent-side
public methods keep their no-argument versions (existing call sites are tied to the drift baseline, zero breakage),
the user capability is split at the Call layer via `*As` variants.

## Not Doing (Future Items)

- Execution history / failure alerts: require wrappers or log collection, belong to the log aggregation domain;
- Distributed locking / server-side scheduling dispatch: the agent-side crontab is already the source of truth, no centralized scheduling needed.

## M1 Checklist

- [x] agent: `internal/agent/rpc/cron_provider.go` (detection/rendering/read-modify-write/self-check/mutex)
      + capability appended + registration by capability
- [x] server: `api_cron.go` (4 endpoints + parameter validation on both ends + audit) + serveAPI integration
      + audit constants
- [x] web: `/cron` page (agent selection/status card/task list/edit modal/external entries) + route menu
- [x] tests: rendering of all forms (enabled/disabled), meta roundtrip, expression validation, external-line
      preservation self-check, concurrent apply serialization, server forwarding and audit
- [x] Documentation wrap-up + todo.md synced

## References

- Internal: [proxy-design.md](/guide/proxy-design) (structurally identical meta-comment roundtrip and server
  forwarding patterns),
  [backup-design.md](/guide/backup-design) (nameLock concurrency discipline), `todo.md` P2 scheduled-task item
- External: [crontab(5) manual](https://man7.org/linux/man-pages/man5/crontab.5.html)
  (field semantics and @extensions), vixie-cron source (write-back semantics)
