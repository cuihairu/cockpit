# Drift Detection Design

> Language: [中文](./drift-design.md) ｜ **English**

> Status: the M1 approach is finalised (2026-09-15) ｜ Reference: the "drift view" item in todo.md
> (NetBox's desired/actual separation); the main implementation is **configuration drift**: baselines are
> produced by the panel's write paths and compared against the current on-disk content.

## Pain points and positioning

On a self-hosted cloud server, "config the panel wrote gets hand-edited or accidentally deleted" is a high-frequency
failure source: after hand-editing an nginx snippet, the next panel save silently overwrites it; a cron job under
cockpit's name gets deleted by someone and nobody knows; a stack's compose.yml gets edited directly so the next
`up` behaves differently from what the page shows.

Cockpit already has three major config write paths (nginx site snippets / the cron cockpit section / stack
compose files), and **writing naturally produces a desired state** — the baseline is recorded the moment a write
succeeds, with no extra declaration from the user. Detection is then pure pull: the user actively checks,
reading disk in real time and comparing.

```
write paths (panel save)                   check (on demand)
nginx ApplySite ──Record──┐          GET /drift/check
cron ApplyJob/DeleteJob ──┤ baseline │  current content sha256
stack SaveCompose ────────┘  (JSON)  ←item-by-item compare→  vs baseline sha256
                                     → ok / drifted / missing / no_baseline
```

## Decisions

| # | Decision | Content | Rationale / notes |
|---|------|------|------------|
| D1 | M1 boundary | on-demand checking (pull), the baseline is read-only and never modified | scheduled inspection + alert notification, manual baseline registration, inventory field consistency (hostname/IP) are left to M2 |
| D2 | Detection targets | only objects under cockpit's name: nginx `cockpit-site-*.conf` snippets / traefik `cockpit-site-*.yml` dynamic snippets / the cron cockpit job section / stack `compose.yml` + `.env` | Consistent with nginx's "zero contact with existing user config" discipline; external entries are not detected |
| D3 | Baseline storage | an agent-local JSON at `/var/lib/cockpit/drift-baseline.json` (`COCKPIT_DRIFT_BASELINE` overrides), mutex + tmp+rename atomic write, 0600 | The baseline follows the agent; the server doesn't persist (forwarding discipline); the write path and the baseline share a process with no network round-trip, so there's no inconsistency window |
| D4 | Entry key | `nginx/<name>`, `traefik/<name>`, `cron/cockpit`, `stack/<name>/compose.yml`, `stack/<name>/.env`; each entry `{sha256, updated_at}` | File granularity, so problems localise directly |
| D5 | Status | `ok` (hash matches) / `drifted` (baseline present, current present, content changed) / `missing` (baseline present, file gone) / `no_baseline` (file present, no baseline — pre-existing objects from outside the panel); implementation additions: `error` (read failure, doesn't affect other kinds), `none` (cron empty section with no baseline, produces no noisy entry) | `no_baseline` hints that saving once through the panel auto-registers it; deleting an object goes through Forget and produces no `missing` |
| D6 | check algorithm | read the current content in real time and sha256 it against the baseline; nginx **hashes the raw file bytes** (a mangled meta comment also counts as drift); cron hashes the marshal of the job list parsed out of `splitCockpit` (a corrupt meta line drops out of the job list → hash changes → drift); stack per file | No semantic parsing; byte-level comparison is the most honest |
| D7 | Hook injection | a `BaselineRecorder` interface (`Record(kind, name, content)` / `Forget(kind, name)`) injected into the three providers, a nil zero value meaning no recording | Providers register independently, so interface injection is the smallest change; tests can inject a fake |
| D8 | Hook timing | Record **after a successful write** (nginx after a successful reload; a failed reload that rolls back records nothing); Forget after a successful delete (nginx DeleteSite / stack directory removal / cron DeleteJob) | The baseline = the content actually in effect in production |
| D9 | capability | `drift`: registered if **any** of nginx-proxy / traefik-proxy / cron / docker-api (stack) exists | With none of the four, check is meaningless |
| D10 | RPC | a single `drift.check` method (POST), with no user-input parameters (M3/M4 add `drift.diff`/`drift.record`) | Zero validation surface; the result is returned in full at once |
| D11 | server | `POST /api/agents/{id}/drift/check` purely forwards, doesn't persist; browsing-class requests aren't audited | Consistent with the cron/logs forwarding discipline |
| D12 | Web | a `/drift` page: agent selection + a "Check" button + a summary (N drifted) + a four-state list (kind Tag / name / status Tag / short baseline and current hashes) | Modelled on the /cron page structure |

## RPC

| Method | Parameters | Returns |
|------|------|------|
| `drift.check` | `{}` | `{items: [{kind, name, status, baseline_sha, current_sha}], checked_at}` |
| `drift.diff` | `{kind, name}` | `{expected, current}` (both sides in full; on the cron side it's the MarshalIndent-prettified cockpit section) |
| `drift.record` | `{kind, name}` | `{recorded: true, sha256}` (register the baseline from the current content, M4) |

Status semantics per D5; an empty `baseline_sha` / `current_sha` string means that side doesn't exist.

## M2: scheduled inspection + drift alerts (2026-09-15)

Upgrades M1's "on-demand check" to "automatic discovery": the server periodically runs drift.check against
online agents carrying the drift capability, and any drift produces an alert (reusing M2 probe's alert
deduplication and notification channel), with zero agent-side change.

| # | Decision | Content | Rationale / notes |
|---|------|------|------------|
| D13 | Inspection loop | server-side `driftScanLoop` (ctx+ticker, same as alert_loop), waking every minute and scanning only when "≥ interval since the last scan"; since the interval can change dynamically, a fixed 1min ticker plus timestamp comparison is used | same as probe runner's maybePrune/lastPrune; lastScan is only accessed by the loop goroutine so no lock is needed |
| D14 | Inspection targets | all online agents from `registry.ListByCapability("drift")`, one `CallAgent(drift.check)` each; failures log and skip to retry next round | A single agent failing doesn't affect others; offline is already covered by the existing offline alert |
| D15 | Alert condition | items containing `drifted` or `missing` → one summary alert per agent; `error` only logs (it may be a permissions issue and alerting would be noisy); `no_baseline` doesn't alert (pre-existing objects); all ok → no action | Aggregating to agent level controls alert storms; the fixed title means only one report per host while an unread one exists (D15 dedupe semantics: after the user marks it read, drift again alerts again) |
| D16 | Alert shape | the alert package exposes `CheckDriftScan(agentID, hostname, driftedNames)`: if non-empty then `createAlertIfNotExists("warning", "configuration drift: host X", detail list, agentID, "agent")`; at most 10 detail lines + a truncation notice | Reuses probe M2's real deduplication (HasUnreadAlert) and non-blocking multi-channel notification |
| D17 | Configuration | Setting key `drift.scan_interval_seconds`, [0, 86400], 0 = inspection off, default 1800 (30 minutes); REST `GET/PUT /api/agents/.../drift/config`? — no, the config is global, not per-agent: `GET/PUT /api/drift/config` | Same Setting pattern as the probe interval; read the Setting on each wake to avoid caching (same as loadThresholds) |
| D18 | Web | an "automatic inspection" settings row at the top of the `/drift` page (toggle + interval in minutes, saving PUT /api/drift/config); when inspection is on, the page notes that automatic discovery is available | The config entry lives with the use case, not in global Settings |

No persistence: inspection results aren't stored (the alert is the record), and the /drift page keeps its
real-time manual-check semantics.

## M3: drift diff view (2026-09-18)

Drifted entries upgrade from "did it change + a short hash" to "where did it change": the baseline additionally
stores the original text, `drift.diff` returns both sides in full, and the web renders a line-level diff.

| # | Decision | Content | Rationale / notes |
|---|------|------|------------|
| D19 | Baseline stores original text | `BaselineEntry` gains `Content string` (omitempty; written alongside the hash on `Record`, and for entries >256KB only the hash is stored without the text); when an old baseline has no text, `drift.diff` clearly reports "the baseline has no original text (recorded by an older version) — save once again on the corresponding management page" | Record already receives the content, so zero extra reads; the next save through the write path backfills the text automatically, zero migration; the baseline file is still local 0600; .env and similar content returned via diff has the same permission surface as a file-manager read (JWT + agent online), no new exposure |
| D20 | `drift.diff` | `{kind, name}`: kind whitelist {nginx, traefik, cron, stack}; expected = baseline original text, current = read in real time (nginx/traefik snippet file / stack compose.yml and .env / cron reading crontab then splitCockpit for jobs); either side missing or >256KB is rejected with a reason; **both cron sides are MarshalIndent-prettified before returning** (the hash/baseline layer keeps compact `json.Marshal` untouched — otherwise old baseline hashes would all mismatch after an upgrade and falsely report drifted; prettifying only happens on the diff display layer) | Reading current uses exactly the same source as check/Record (same marshal), guaranteeing the diff agrees with the hash judgement; JSON prettifying makes the cron section diffable by line |
| D21 | Server endpoint | `POST /api/agents/{id}/drift/diff`, body `{kind, name}`: after the kind whitelist + non-empty name ≤128 are validated by the same rules on both ends, forward `drift.diff`; no audit (browsing in nature, consistent with check) | reuses forwardDriftRPC |
| D22 | Web diff rendering | a "Diff" button on drifted rows → Modal: a hand-written LCS line-level diff (both sides first truncated to 1000 lines with a "content too long, comparing only the first 1000 lines" notice); unified diff colouring (del red = in baseline not current, add green = new in current, same grey); a footer hint "after verifying, save again on the corresponding management page to clear the drift" | Hand-written LCS (integer keys + full DP table + backtracking, 1000 lines O(n²) ≤10⁶ is acceptable) avoids a new dependency; no side-by-side view (a line-level unified diff suits config-section scenarios better; later overturned by M5/D26) |

### Agent side

- `drift.diff`: validate kind/name → fetch the baseline entry (absent → report "no baseline"; present but Content
  empty → report "the baseline has no original text") → read current by kind (same source as check) → both sides
  >256KB → error → MarshalIndent both cron sides → return;
- `Record` stores the original text: the entry in `update()` carries Content (the truncation logic lives inside Record, leaving Forget/hash semantics untouched).

### Server side

- `handleAgentDriftAPI` gains `sub == "diff"`: POST + body decoding + the kind whitelist
  {nginx,traefik,cron,stack} + non-empty name ≤128 → `forwardDriftRPC(..., "drift.diff", params)`.

### Web side

- `types` gains `DriftDiffResult {expected, current}`; `api.driftDiff(agentId, kind, name)`;
- `web/src/utils/lineDiff.ts`: LCS line-level diff (inputs are both strings, split on '\n' truncated to
  1000 lines with a truncated flag, output `{type: 'same'|'del'|'add', text}[]`);
- The Drift page: a "Diff" button on drifted rows → Modal (loading Spin / a failure Alert surfacing the agent's
  reason / diff rows in monospace + line numbers + colouring + truncation notice + footer action guidance).

**Tests**: agent — Record storing original text and truncation, the full diff flow (hand-edited nginx /
  cron prettifying with the same source / two stack files), an error for an old baseline without original text,
  kind/name validation, rejection when either side is too large; server — forwarding and 400 validation;
  web — tsc + build.

## M4: manual baseline registration "take current as authoritative" (2026-09-18)

Completes "the baseline only comes from the panel's write paths" into "it can also come from user confirmation":
pre-existing objects outside the panel (`no_baseline`) enter detection with one click; a drifted object that
was hand-edited and confirmed correct becomes the new standard with one click. The agent side reuses M3's
currentContent + Record, with zero new storage.

| # | Decision | Content | Rationale / notes |
|---|------|------|------------|
| D23 | `drift.record` | `{kind, name}`: kind/name reuse M3 validation → read current content in real time (the same `currentContent` as check/diff, compact marshal for cron) → register the original text via `baseline.Record`; applies to no_baseline (bring it into detection) and drifted (confirm the hand edit as the new standard); missing/error scenarios error out directly when current content can't be read (no client-side status whitelist needed); >256KB keeps Record's semantics of storing only the hash | Everything reuses M3's building blocks (validDriftTarget + currentContent + Record); "take current as authoritative" and saving again on the panel are complementary — the former takes disk as authoritative (no file rewrite), the latter takes the panel as authoritative (rewrite the file) |
| D24 | Server endpoint + audit | `POST /api/agents/{id}/drift/record`: validated by the same rules as diff (kind whitelist + non-empty name ≤128) then forwarded; **audited** as `drift_record` / `drift_baseline`, resourceID=`kind/name`, detail records the agent | check/diff are browsing and aren't audited, but record is a user actively changing the drift judgement standard — from then on whether that object "drifts" is judged by this, so it must be traceable (same tier as file-change auditing) |
| D25 | Web interaction | an "Take current as authoritative" button on drifted rows + a "Register" button on no_baseline rows (Popconfirm: "future drift detection will use the current on-disk content as the standard"); automatically re-check to refresh the list on success; missing/error rows show no buttons | The action entry lives inline in the result row for an in-place closed loop; the two wordings call the same RPC, and picking the more accurate verb per row status is better |

### Agent side

- `drift.record`: `validDriftTarget` → `currentContent` (read failures pass the reason through) →
  `baseline.Record` → return `{recorded: true, sha256}` (the hash after registration, for the frontend hint).

### Server side

- `handleAgentDriftAPI` gains `sub == "record"`: sharing diff's body validation →
  `forwardDriftRPC(..., "drift.record", params)` → on success `auditDriftRecord`
  (username/IP/UA following the auditFile pattern, detail records the agent).

### Web side

- `api.driftRecord(agentId, kind, name)`; the action column shows "Diff" (drifted) /
  "Take current as authoritative" (drifted) / "Register" (no_baseline) by status; Popconfirm confirm → call →
  success message → automatically re-run check to refresh the list (a failure Alert surfaces the agent's reason).

**Tests**: agent — no_baseline registration → check becomes ok (nginx/cron/stack), drifted
take-current-authoritative → ok and the baseline original text updated, errors on read failure, validation
rejection; server — forwarding parameters, 400 validation never reaching the agent, audit persisted;
web — tsc + build.

## M5: side-by-side diff and config syntax highlighting (2026-09-19)

Overturns D22's scope decision "no side-by-side view" (a line-level unified diff is unintuitive when additions
and deletions interleave). A pure web upgrade with zero Go changes.

| # | Decision | Content | Rationale / notes |
|---|------|------|------------|
| D26 | Side-by-side view | the DiffModal gains a Segmented "Side-by-side / Unified" toggle (side-by-side by default); `pairDiffLines` turns the unified diff into row pairs: same passes through to both columns, consecutive del/add blocks are paired in order one-to-one, the extra side occupies a row alone; the missing half gets a shaded background | Pairing happens in the render layer, so lineDiff's LCS output is untouched; the unified view is retained (narrow windows and whole-section copy scenarios) |
| D27 | Lightweight config highlighting | `configHighlight.ts` is a single-line stateless lexer: comment lines / leading keys (yaml `key:`, env `KEY=`, json `"key":` with quotes included) / nginx leading-word directives (only the `word …;` and `word … {` shapes are recognised) / quoted strings / standalone numbers (not adjacent to word characters or dots on either side) / `{};` punctuation; no cross-line parse state, no guessing at inline `#` comments | Better to under-recognise than to be wrong: covering the four kinds of content cockpit delivers (nginx conf, Traefik/compose YAML, cron JSON, .env) is enough; no syntax highlighting dependency |

### Web side

- `lineDiff.ts` gains `pairDiffLines` (unified diff → side-by-side row pairs); a new
  `configHighlight.ts` (highlighting lexer returning a span sequence);
- DiffModal: a Segmented view toggle (side-by-side by default) + both views share colouring and truncation
  notices; Modal width 960→1080 (side-by-side needs more width).

**Tests**: web — tsc + build + tsx runtime verification (comment lines / yaml keys / nginx directives
  with port numbers / env keys and values / JSON keys and string values / the `hunter2` value not being
  mis-coloured / no inline `#` comment guessing; side-by-side pairing aligning del/add, an extra side
  occupying a row alone).

## M6: CMDB consistency — inventory declaration vs agent report (2026-09-19)

The last item of the "not doing" list is formally proposed, and it is also the closing of the drift main line
(configuration drift) into the CMDB main line (desired vs actual): configuration drift manages "were the files
the panel delivered changed", CMDB consistency manages "is the machine declared in Git still that machine".
The data facts decide the shape — the declared state only lives in the inventory YAML (an in-memory copy
hot-reloaded by the watcher), and the actual hostname/IP overwrite the same DB row (via `persistRegisteredAgent`
→ `UpsertAgent`), so the declared values have no persisted copy. Therefore **no new table and no inspection
loop**: compare an on-demand read model.

| # | Decision | Content | Rationale / notes |
|---|------|------|------------|
| D28 | On-demand read model | `GET /api/inventory/consistency`: `watcher.GetInventory()` (declared) × `db.ListAgents()` (reported) compared immediately, zero new persistence, zero inspection loop, not entering the alert framework | Consistency is CMDB hygiene, not an incident — "UI highlighting" is the whole ask (P2 wording); the inventory watcher already guarantees the declared state is fresh; starting a loop would only produce a lagging copy in unread state |
| D29 | Comparison semantics | only compare when the declaration is non-empty; hostname is **case-insensitive** (DNS semantics) + TrimSpace, IP is an exact string match (no CIDR/multi-IP semantics — inventory v1 is a single string); four states: `ok` / `mismatch` (with field-level detail) / `unregistered` (declared but absent from the DB — not yet online, or declared before provisioning) / `undeclared` (in the DB but not declared in YAML — a CMDB completeness gap) | Case normalisation is the only exemption: hostnames are case-insensitive at resolution anyway, so reporting it is noise; everything else is better strict, since false positives are more useful than misses in a CMDB |
| D30 | API + UI | server side `handleInventoryConsistency` (JWT, browsing-class so unaudited, same as drift check; when `inventorySync == nil` a 503 names `inventory.path` as guidance); the comparison is a pure function in `internal/inventory` (inputs `*Inventory` + `[]*storage.Agent`, outputs a report, no IO so the whole table is testable); the web drift page gains Tabs "Configuration drift / CMDB consistency", with the consistency tab showing a summary bar + a four-state table (mismatch rows expand declared vs reported) | The pure function is decoupled from the endpoint so it's testable without the watcher; the UI hangs off the drift page — both belong to the "expected vs actual" theme, and a single report doesn't deserve its own page |

### Server side

- `internal/inventory/consistency.go`: `AgentConsistency` pure comparison +
  `CompareAgents(inv, agents)` report (four-state summary counts + details sorted by id);
- the `internal/sync` Manager gains a thin `Consistency(db)` wrapper (GetInventory + comparison);
- `api_inventory.go`: the route `/inventory/consistency`.

### Web side

- `api.getInventoryConsistency()`; the drift page becomes Tabs: the original content as "Configuration drift",
  a new "CMDB consistency" tab (useQuery with 30s staleTime, four-state Tags + a mismatch detail
  Tooltip comparing declared/reported).

**Tests**: inventory — table-driven tests of the pure comparison function (ok/hostname case/IP mismatch/
  multiple fields reported together/unregistered/undeclared/empty declaration skipped); server — endpoint wiring
  (200 shape, nil manager 503, JWT and outer validation not involved); web — zero new tsc errors + build.

## M6 checklist

- [x] inventory: the `consistency.go` pure comparison function (four states + field-level mismatch +
      summary counts + stable id sort) + table-driven unit tests
- [x] sync: `Manager.Consistency()` (GetInventory × ListAgents immediate comparison)
- [x] server: `GET /api/inventory/consistency` (JWT, browsing-class unaudited;
      `inventorySync == nil` → 503 naming `inventory.path` as guidance) + endpoint tests
- [x] web: types + `api.getInventoryConsistency()` + drift page Tabs +
      `ConsistencyTab` (summary bar + four-state table + mismatch rows expanded)
- [x] tests: 3 inventory tests + 2 server tests all green (the whole package passes under -race);
      web tsc with zero new errors (the baseline 9 unchanged) + build passing
- [x] documentation wrap-up (this checklist ticked) + todo.md synced

M6 done (2026-09-19). Landing delta notes: ① `Consistency()` is a no-argument method
(the db is held from `NewManager` construction), not the `Consistency(db)` in the design wording; ②
the mismatch comparison landed as **row expansion** (a field-level sub-table: field/declared value/reported value),
with the Tooltip in the "Web side" wording only carrying the four-state status semantics rather than stuffing the comparison into a Tooltip; ③
Tabs-isation came with the card title changing from "Drift Detection" to "Drift and Consistency" (both tabs belong to the expected vs
actual theme); ④ the 503 wording when inventory isn't enabled is surfaced on the web side as a guiding Alert
(including the `inventory.path` pointer), consistent with the server endpoint wording.

## Not doing (later versions)

- Detection of non-cockpit nginx snippets, external crontab entries, docker volume contents.

## M1 checklist

- [x] agent: `drift_provider.go` (baseline storage read/write + check) + hooks into the nginx/cron/stack
      three providers + the `drift` capability + providers.go wiring
- [x] server: `api_drift.go` (check forwarding) + serveAPI integration
- [x] web: the `/drift` page + routing and menu
- [x] tests: baseline atomic read/write, four-state judgement (nginx/cron/stack), hook injection,
      server forwarding and offline 503
- [x] documentation wrap-up (this checklist ticked) + todo.md synced

M1 done (2026-09-15): 8 agent tests + 3 server tests all green; baseline hook verification covers
"panel write succeeds → baseline registered", "validation fails → not registered", "external entry changed → not counted as drift";
the cron baseline hashes only the cockpit job section (zero contact with external crontab entries).

## M2 checklist

- [x] alert: `CheckDriftScan` (summary alert + reusing real dedupe) + unit tests
- [x] server: `drift_scan.go` (loop + Setting key + interval validation) + config
      GET/PUT endpoints + server.go startup
- [x] web: the "automatic inspection" settings row on the /drift page + api/types
- [x] tests: alert creation/dedupe/all-ok no action, config validation, scan failure skipping
- [x] documentation wrap-up (this checklist ticked) + todo.md synced

M2 done (2026-09-15): 2 alert tests (creation/field validation, unread dedupe, re-alert after being read,
empty list no-op, 13 items truncated to 10 lines + a total notice) + 5 server tests (config default/
legal including 0 written/out-of-range 400, scan producing an alert + second-round dedupe, all-ok and error
states not alerting, an agent error skipping without panic, the title locating the hostname) all green; the
inspection loop is a fixed 1min ticker + lastScan comparison (a dynamic interval change takes effect), and
startup waits 90s to let agents come online first.

## References

- Internal: [cron-design.md](./cron-design.en.md) (splitCockpit and the server forwarding pattern),
  [proxy-design.md](./proxy-design.en.md) (snippet directories and the zero-contact discipline),
  [stack-deploy-design.md](./stack-deploy-design.en.md) (stacks directory layout)
- External: NetBox's desired/actual model, driftctl / Terraform drift detection's
  state comparison presentation