# Service Management Design (systemd + Windows SCM)

> Language: [中文](./service-design.md) ｜ **English**


> Status: M1 design finalized (systemd); M2 appends the Windows SCM backend (D9), with capability
> unified as `service`. On the agent side there is one provider per platform backend; the server purely
> forwards without persisting (systemd / SCM are each their own source of truth); the Web /services page
> follows the cron page pattern.

## D1 Goals and scope

**M1 delivers**: on systemd hosts, list observation + runtime operations for `*.service` units:

- List services: load/active state, whether auto-start is on (unit file state), description
- Overview: overall systemd running state (running / degraded / ...) + active/failed counts
- Operations: start / stop / restart / reload (runtime), enable / disable (auto-start persistence)

**M1 does not do** (later candidates):

- journal log viewing (the logs provider already covers journalctl queries)
- unit types such as timer / socket / mount (only `.service`)
- unit file editing, daemon-reload (M2 candidates)
- non-systemd platforms: macOS launchd (M3 candidate, see the scope notes in D9); OpenRC / runit are not done

Positioning: a foundational capability for the NAS scenario — service start/stop and auto-start management is the basic operating surface of host operations; the pattern is isomorphic to the cron page, with high delivery certainty.

## D2 Detection and capability

```go
// DetectSystemd probes whether systemd is available
func DetectSystemd() bool {
    if _, err := exec.LookPath("systemctl"); err != nil {
        return false
    }
    fi, err := os.Stat("/run/systemd/system")
    return err == nil && fi.IsDir()
}
```

- The existence of `/run/systemd/system` is the classic "systemd has booted on this machine" test: it rules out the
  container case where the systemctl binary is installed but there is no systemd PID 1, as well as chroot / rescue
  environments
- capability: `{Type: "service", Version: "2", Metadata: {"backend": "systemd"}}`, appended in agent.go detectCapabilities
  (the same approach as the cron capability — no dedicated detector package entry needed)
- providers.go: `case "service"` registers `NewServiceProvider(nil)`
- Permissions: operating on system-level units requires root (the agent normally runs as root, the same premise as
  backup/nginx reload). When non-root, systemctl reports an error; the agent passes through the stderr summary as-is
  and performs no sudo escalation

## D3 RPC methods (provider type = `service`)

```go
type Commander func(ctx context.Context, name string, args ...string) (stdout []byte, stderr []byte, err error)
```

Reuses the Commander abstraction and defaultCommander from nginx_provider.go; argv is passed directly without going through a shell.

### service.list — Service list

Two rounds of queries merged (a failure in either round is an error):

1. `systemctl list-units --type=service --all --no-legend --no-pager`
   columns: UNIT LOAD ACTIVE SUB DESCRIPTION (loaded units only, including inactive)
2. `systemctl list-unit-files --type=service --no-legend --no-pager`
   columns: UNIT FILE STATE VENDOR PRESET (every installed unit, including unloaded ones)

Merge by unit name: list-units supplies the runtime state (load/active/sub + description),
list-unit-files adds the unloaded install entries and supplies the auto-start state (enabled/disabled/static/...).
For unloaded entries activeState is recorded as `inactive`. The list is returned sorted by name:

```json
{ "services": [ { "name": "nginx.service", "description": "...",
    "loadState": "loaded", "activeState": "active", "subState": "running",
    "unitFileState": "enabled", "preset": "enabled" } ] }
```

Parsing: split on whitespace between columns, DESCRIPTION is the remaining columns merged back together (`strings.SplitN`).
`--no-pager` prevents systemctl from hanging on paging when there is no tty; `--no-legend` removes the header footnote.
A typical host has tens to hundreds of services; the full set is returned (tens of KB in magnitude) with no pagination.

### service.status — Overview

- `systemctl is-system-running`: outputs running / degraded / maintenance /
  stopping / offline and so on. Note that in the degraded state the exit code is non-zero but stdout
  has a value — stdout takes precedence, and err is treated as a failure (probe unavailable) only when stdout is empty
- Service counts: tally active / failed / enabled from the service.list result
- Returns `{ "systemState": "degraded", "total": 87, "active": 62, "failed": 1, "enabled": 40 }`

### service.action — Performing an operation

params: `{ "name": "nginx.service", "action": "restart" }`

- **Action whitelist**: start / stop / restart / reload / enable / disable /
  mask / unmask (M2 extension: mask links the unit to /dev/null to forbid starting it
  entirely — a stronger lockout than disable, guarding against mistaken starts and disguise
  services; the masked state surfaces in the list's unitFileState, and the front end already
  has rendering for a "Masked" Tag. Note that mask does not affect a running service)
- **Unit name whitelist**: regex `^[A-Za-z0-9@._+-]+$` and it must end with `.service`;
  `/`, spaces, `..` and every other ambiguous character are excluded. The front end passes through the
  full unit name returned by the list verbatim (systemd already includes `.service`)
- Executes `systemctl <action> <name>`, 30s timeout (restarting database-type services can be slow)
- On success returns `{ "name": "nginx.service", "action": "restart" }`; on failure the
  stderr summary is passed through as-is (commandErrSummary), e.g. `Failed to restart nginx.service: ...`

## D4 Security boundary

1. Only the eight systemctl verbs are exposed; there is no arbitrary command execution surface
2. Unit name regex + enforced `.service` suffix, validated by the same rules on both ends (the server refuses once, the agent refuses once)
3. The Commander pattern passes argv directly; arguments never go through a shell, so injection is impossible
4. Auditing: action-class operations record `service_action` (details carries the action verb and the agent ID);
   list / status browsing records no audit (cron precedent). enable/disable are persistent changes,
   but they also record service_action, distinguished by details.action
5. systemd itself is the source of truth and the permission boundary: the agent can only operate on the units its
   runtime identity is privileged to operate; cockpit performs no escalation

## D5 server REST endpoints

Mounted under the serveAPI `/agents/` branch (api.go: `strings.HasSuffix(agentID, "/services") || strings.Contains(agentID, "/services/")`),
handler file api_service.go, following the api_cron.go pattern (pure forwarding, no persistence — the same discipline as D9):

```
GET  /api/agents/{id}/services/status            → service.status (browse, not audited)
GET  /api/agents/{id}/services                   → service.list (browse, not audited)
POST /api/agents/{id}/services/{unit}/{action}   → service.action (audits service_action)
```

- `{unit}` and `{action}` are validated by the same rules on both ends (whitelist regex + action whitelist);
  an illegal request gets a direct 400 from the server, without consuming an agent round trip
- `{action}` sits in the URL path (the verb is the resource semantics); the `.` in unit names does no harm to path parsing
- Agent offline → 503 (registry.Get check); agent-side errors are passed through as 502
- audit constants: `ResourceService = "service"`, action `service_action`,
  resourceID = unit name

## D6 Web /services page

- Route `/services`, menu item "Service Management" (ThunderboltOutlined), placed after "Scheduled Tasks"
- Agent selection: follows the cron page — a dropdown listing the registered agents, annotated with the systemd capability;
  agents with no detected systemd capability cannot be selected (or show a guidance card once selected)
- Status card: systemState badge (degraded orange / running green) + total/active/failed counts
- Service table:
  - Name (code style) + description
  - Active badge: active green / activating blue / failed red / inactive gray / anything else default
  - Auto-start Tag: enabled green / disabled gray / static blue (tooltip: a static dependency, which cannot be directly started, stopped, or managed)
  - Action buttons (shown/hidden by state): active → Stop/Restart; inactive → Start;
    unitFileState enabled ↔ disabled → disable/enable toggle; reload only shown for active
  - Row-level loading while an action is in flight; on completion invalidate the list + the status card
- The list polls automatically every 30s (refetchInterval), and refreshes immediately after an operation
- When there is no systemd agent, a guidance card is shown (explaining the detection conditions and the agent permission requirements)

## D7 Test strategy

- **agent** (rpc/service_provider_test.go): fake Commander injection (the same as the cron tests) —
  sample text output for the two systemctl commands → merged-parse assertions (including unloaded entries, DESCRIPTION containing spaces,
  ordering); action whitelist rejection; illegal unit name rejection (`../`, spaces, no .service suffix);
  action failure passes through stderr; is-system-running with a non-zero exit code but output → no error
- **server** (api_service_test.go): validation on both ends (bad unit name / illegal action → 400);
  offline agent → 503; successful forwarding + exactly one service_action audit (details carries the action);
  list/status produce no audit
- **web**: `npx tsc --noEmit` + `npm run build`

## D8 Acceptance checklist

- [ ] `go test ./internal/agent/rpc/ ./internal/server/` passes
- [ ] `go vet ./...` is clean; new files are gofmt-clean
- [ ] web tsc + build pass
- [ ] Real environment (systemd host): the list contains unloaded units, restart/enable verified in practice,
      error pass-through from a non-root agent (recorded in todo.md)

## D9 Cross-platform unification: the Windows SCM backend (M2)

### D9.1 Motivation and the unified model

The Windows Service Control Manager (SCM) is highly isomorphic to systemd in semantics: a service has a runtime
state and an auto-start state, and supports start/stop plus auto-start toggling. The two have a minimal common
set, so the observation model directly reuses `ServiceUnit` with no new DTO:

| ServiceUnit field | systemd | Windows SCM |
|---|---|---|
| Name | `nginx.service` | SCM service name (e.g. `wuauserv`, no suffix) |
| Description | unit description | DisplayName |
| LoadState | loaded/not-found | always `loaded` (a service is necessarily registered in the SCM) |
| ActiveState | active/inactive/failed/... | status mapping (see D9.3) |
| SubState | systemd fine-grained states | the SCM's raw status lowercased (`running`/`stopped`/`paused`...) |
| UnitFileState (auto-start state) | enabled/disabled/static/... | StartType mapping (see D9.3) |
| Preset | vendor preset | empty (Windows has no preset concept; the front end falls back to `—`) |

Ecosystem choice: the official Go `golang.org/x/sys/windows/svc/mgr` (pure syscall,
usable with CGO_ENABLED=0, compatible with the Windows build matrix). kardianos/service means
"install myself as a service", which differs from "manage existing services", so it is not adopted; gopsutil only offers a process view,
with no auto-start state. macOS launchd differs greatly in semantics (the dual LaunchAgent/LaunchDaemon domains,
plist files as the source of truth), so it is left for separate design in M3 and is out of scope for this section.

### D9.2 Unifying capability and detection

capability is renamed from `systemd` to `service`, with the backend difference moved into metadata:

```json
{ "type": "service", "version": "2", "metadata": { "backend": "systemd" } }
{ "type": "service", "version": "2", "metadata": { "backend": "windows-scm" } }
```

- Reason for the rename: agent/server/front end ship from the same repo with no compatibility baggage; "systemd" as a capability
  name would block Windows hosts, while `service` is the actual semantics of the capability itself
- Linux: keep using DetectSystemd (systemctl + /run/systemd/system), and report the capability
  only after detection passes, backend=systemd
- Windows: the SCM is a built-in OS component that always exists, so no file detection is needed; report
  backend=windows-scm directly
- providers.go `case "service"`: construct the matching provider from metadata.backend
  (systemd → `NewServiceProvider(nil)`; windows-scm → `NewWindowsServiceProvider()`)
- The `systemd` log source type on the logs page (logs provider) is unrelated to this capability and stays unchanged

### D9.3 Field and action mapping

**Runtime state mapping** (`windowsStatusToActiveState`, a pure function):

| SCM Status | ActiveState | SubState |
|---|---|---|
| Running | active | running |
| Stopped | inactive | stopped |
| Start Pending | activating | start_pending |
| Stop Pending | deactivating | stop_pending |
| Paused / Pause Pending | paused | paused / pause_pending |
| Continue Pending | activating | continue_pending |

`paused` is not mapped to active: pausing is a Windows-specific semantic, and the front end shows unknown values
with a gray badge under the original word rather than distorting the semantics.

**Auto-start state mapping** (`startTypeToUnitFileState`, a pure function):

| SCM StartType | UnitFileState |
|---|---|
| Automatic (including DelayedAutostart) | enabled |
| Manual | disabled |
| Disabled | disabled |

The front end's "Auto-start" column uses only the two states enabled/disabled; Manual and Disabled are both "manual".

**Action mapping** (Windows supports 5 of the 6 verbs, `reload` is not supported):

| Action | SCM implementation | Notes |
|---|---|---|
| start | `svc.Start()` | fire-and-return; state convergence relies on the front end's 30s polling |
| stop | `svc.Stop()` | same; the SCM's start/stop is an asynchronous request, which differs from systemd's synchronous semantics |
| restart | Stop → wait for Stopped (30s timeout) → Start | the SCM has no native restart |
| reload | — | returns the error "reload is not supported on windows-scm backend"; the front end hides the button per backend |
| enable | `SetStartType(Automatic)` | equivalent to systemd enable's boot-time auto-start |
| disable | `SetStartType(Disabled)` | equivalent to systemd disable |

The timeout reuses `serviceActionTimeout` (30s).

### D9.4 Security boundary

1. **Windows service name whitelist** (separate from the systemd regex, `validateWindowsServiceUnit`):
   regex `^[A-Za-z0-9_.\- ]{1,256}$` and it must not be `.` / `..` (the SCM registry key name character set,
   which excludes `/` `\` and wildcards). A service name and a DisplayName are two different things; operations always use the service name
2. Action whitelist: the systemd 8 verbs (M2 extends with mask/unmask); Windows and launchd each have
   5 verbs (no reload/mask), each validated inside its own provider
3. **Server-side validation relaxed to the union of the two rules**: `{unit}` is allowed through for forwarding if it satisfies the systemd regex **or** the Windows
   regex. The server does not parse the agent capability to do exact matching — the union is still a
   pure whitelist (no `/` `\` `..` null bytes), and the genuinely backend-specific strict validation still lives on the agent side
   as a backstop (the systemd side rejects a missing `.service` suffix; the Windows side rejects out-of-bounds characters outside the `.service` suffix);
   the action whitelist is unchanged
4. SCM permission boundary: the agent's runtime identity's write permission over a service is decided by the Windows ACL, and cockpit
   performs no escalation; without permission the SCM reports an error which is passed through as-is
5. Auditing unchanged: actions record `service_action`, list/status are not audited

### D9.5 Implementation organization (cross-platform compilation)

Under the three-platform CI matrix all agent code compiles on every platform. After P7b, Windows SCM interaction goes through the Host
service port (platform.Current().Services()), and the original build-tag two-file layout (the scm implementation +
a !windows stub) collapses into a single untagged file, so the entire Linux CI chain is testable:

```
service_provider.go        no tag: systemd implementation (existing, logic unchanged)
service_windows_model.go   no tag: pure functions mapping WinService DTO → ServiceUnit +
                                   validateWindowsServiceUnit (testable on Linux CI)
service_windows_scm.go     no tag: host-port retrieval + mapping/validation/count orchestration;
                                   on non-Windows the GOOS port is always nil and the three
                                   actions report an explicit error (the original stub's
                                   defensive semantics are carried by the nil path, error
                                   messages unchanged)
core/platform/windows      //go:build windows: SCM collection + real action implementation (leaf)
```

`x/sys` is already in the dependency tree (indirect) and becomes a direct dependency once implemented. Scale of collection: enumerating several hundred services over local SCM RPC
takes milliseconds, on the same order as systemctl's full list; the whole set is returned at once with no pagination.

### D9.6 Web adaptation

- The agent dropdown filter becomes `c.type === 'service'`, reading backend from the capability metadata
- On hosts with backend=windows-scm: hide the "Reload" button and the "System State" overview item
  (status returns systemState=`unknown`; the SCM has no concept of a global systemd state)
- The name column's `replace(/\.service$/, '')` has no side effect on Windows names; on Windows the "Auto-start" column
  only ever shows the two states enabled/disabled, so the rendering logic is reused
- The "not detected" dropdown copy becomes "No service management detected"

### D9.7 Test strategy

- **agent** (service_windows_model_test.go, runnable in Linux CI): table-driven coverage of every branch of the
  runtime-state / auto-start-state mapping; validateWindowsServiceUnit with legal names / containing `/` / `..` / overlong / reload rejection
- **stub**: Call on non-Windows platforms returns an explicit error (the CI platform tests it directly)
- **server** (extra cases in api_service_test.go): Windows names (`wuauserv`, with hyphen and
  space samples) forward with 200; still-illegal names (`../x`, null byte) get 400
- **systemd regression**: the existing service_provider_test.go passes unchanged (provider logic unchanged)
- **Real-hardware acceptance** (recorded in todo.md): on a Windows host, list / start-stop / auto-start toggle / restart waiting,
  error pass-through for a service without permission; confirm the reload button is hidden

### D9.8 Acceptance checklist

- [ ] `GOOS=windows go build ./...` passes; `go test ./...` (Linux) passes
- [ ] `go vet ./...` is clean; new files are gofmt-clean
- [ ] web tsc + build pass
- [ ] Real-Windows acceptance items are recorded in todo.md

## D10 The macOS launchd backend (M3)

### D10.1 Scope and detection

- Only **system-domain LaunchDaemons** are handled (`/Library/LaunchDaemons` +
  `/System/Library/LaunchDaemons`; the premise that the agent runs as root is the same as on the systemd side).
  LaunchAgents (the user session domain, `gui/<UID>`) are not handled — the domain prefix depends on the user UID, and the semantics are a separate discussion
- Detection: `runtime.GOOS == "darwin"` registers unconditionally; the capability follows the same unified model as D9.2
  `{type: "service", metadata: {backend: "launchd"}}` (launchd is PID 1, so no file detection is needed)
- D1's "macOS launchd is an M3 candidate" is closed here; OpenRC / runit are still not done

### D10.2 Observation model mapping (reusing ServiceUnit)

| ServiceUnit field | launchd source |
|---|---|
| Name | the plist `Label` key (e.g. `com.example.ssh`, ≠ the file name) |
| Description | the plist file path (launchd has no description concept; the path is the clue to "which daemon this is") |
| ActiveState | `launchctl list` PID>0 → active; non-zero Status (abnormal exit last time) → failed; anything else (including installed-but-unloaded) → inactive |
| SubState | empty (launchd has no fine-grained states) |
| UnitFileState (auto-start state) | `RunAtLoad && !Disabled` → enabled, otherwise disabled |
| Preset | empty |

Merging the two sources (isomorphic to systemd list-units/list-unit-files):

1. `launchctl list` (three columns PID/Status/Label, TSV) → runtime state; a Status of `-` means
   it has never run (inactive)
2. Scan the LaunchDaemons directory for `*.plist` → the full set of installed entries + the RunAtLoad/Disabled keys.
   plist is XML, and the standard library's `encoding/xml` lightly parses just the three
   Label/RunAtLoad/Disabled keys of the top-level dict; **no third-party plist library is introduced**.
   Files that fail to parse are skipped (logged); if everything fails, that is not an error (the directory may be empty)

### D10.3 Action mapping (5 verbs, `system/<label>` service target)

| Action | launchctl implementation | Notes |
|---|---|---|
| start | `kickstart system/<label>`, falling back to `bootstrap system <plist>` on failure | kickstart only works for already-loaded services; ones that were bootout-ed need to be bootstrapped again |
| stop | `bootout system/<label>` | launchd has no stop verb, unloading is stopping; the plist remains (contrast with systemd stop, which does not unload — the semantic difference is discussed below) |
| restart | `kickstart -k system/<label>` | native kill + restart |
| reload | — | launchd has no reload semantic, the error is the same as on Windows SCM |
| enable | `enable system/<label>` | clears the disabled DB flag |
| disable | `disable system/<label>` | writes the disabled DB flag; **it does not unload the running instance** (different from systemd disable, which takes effect immediately; the front end's prompt copy has already been generalized and does not involve this) |

The timeout reuses serviceActionTimeout; the Commander abstraction is reused too (argv calls launchctl directly, no shell).

### D10.4 Security boundary

1. **label whitelist** (`validateLaunchdLabel`): `^[A-Za-z0-9_][A-Za-z0-9._-]{0,254}$`
   (the reverse-domain-name convention character set, excluding `/` and spaces) + an action whitelist of 5 verbs;
   together with the systemd/Windows validation it forms a three-pillar whole, with the agent side validating strictly per backend
2. The server-side union whitelist already naturally covers the label character set (it matches the Windows name regex), zero changes
3. System-domain actions require root; permission errors are passed through as-is
4. The auditing/browsing discipline is unchanged (actions record service_action)

### D10.5 Implementation organization and Web

- The three-layer organization is the same as D9.5: `service_launchd_model.go` (untagged: plist parsing +
  launchctl list parsing + mapping + validation as pure functions, testable in Linux CI),
  `service_launchd_darwin.go` (//go:build darwin real execution),
  `service_launchd_stub.go` (//go:build !darwin defensive stub)
- Web: D9.6's `isWindows` special case generalizes into "only show the reload
  button and the system-state item when backend === 'systemd'" — launchd and windows-scm hide both in the same way; status's
  systemState is always `unknown` (launchd has no global degraded concept)
- The capability filter, the dropdown copy, and name-column rendering need zero changes (`replace(/\.service$/)`
  has no side effect on labels)

### D10.6 Test strategy

- **Model layer** (service_launchd_model_test.go, Linux CI): plist XML parsing
  (RunAtLoad/Disabled presence combinations, missing Label skipped, malformed files), launchctl list
  output parsing (PID / `-` / non-zero Status), auto-start state mapping, label validation (legal / `/` / spaces / overlong),
  action whitelist (5 verbs pass, reload rejected)
- **stub**: Call on non-darwin platforms returns an explicit error
- **server/web**: zero changes means regression passes; `GOOS=darwin go build` verifies compilation
- **Real-hardware acceptance** (recorded in todo.md): on a macOS host, list / start-stop / auto-start toggle,
  the kickstart-falls-back-to-bootstrap path, error pass-through without permission


### D11 Journal log jump (M2 extension, web only)

Service-page troubleshooting (locating the cause of failed / inactive) requires reading the journal; the logs provider
(logs-design.md) already provides the `journalctl -u <unit>` query primitive — the service page is only missing
an entry point and source pre-selection — zero new endpoints, zero Go changes.

- **Entry point**: add a "Logs" button to the service table's action column (FileTextOutlined icon + Tooltip),
  visible on every row (troubleshooting is not limited to failed); shown only on hosts with `backend === 'systemd'`
  (launchd / windows-scm have no journal)
- **Interaction**: clicking opens a Drawer (760px) embedding a `LogsPanel` of the same kind as Workbench,
  passing the current row's unit name as `initialSource` — entering immediately locks that service as the log source
- **LogsPanel backward-compatible extension**:
  - New optional prop `initialSource`: the initial value of `pickedSource`; the source-derivation logic gains one
    exception — when `pickedSource === initialSource` it **wins unconditionally**, without the
    "fall back to the first item when it is not in the list" behavior. Reason: `logs.sources` only enumerates
    units registered in `systemctl list-units`; an unloaded inactive service is not in the list,
    but `journalctl -u` can still return historical logs for it (with no entries, `-q` outputs
    "-- No Entries --"), and falling back would query the wrong target
  - Automatic first query: when `initialSource` exists, one query runs automatically once `logs.status` is ready
    (useRef prevents duplication), with no need to click "Query" again
- **Auditing**: `logs.query` is browse-class and records no audit (logs-design discipline); the entry point reuses the
  existing `/api/agents/{id}/logs/query` forwarding
- **Tests**: web-side only, verified with tsc + build; zero Go changes

### D12 daemon-reload (M2 extension)

`systemctl daemon-reload`: refreshes the systemd manager configuration after a unit file changes (hand-edited /
installed or removed by a package). It is a global operation that does not target a single unit, its semantics
differ from unit actions, so it becomes an independent RPC rather than being stuffed into service.action:

- **agent**: `ServiceProvider.DaemonReload()` → `systemctl daemon-reload`
  (the timeout is the same as serviceActionTimeout, normally returning within seconds); the RPC method is `service.daemon-reload`
  (the Call case has the same name), params empty. Only the systemd provider implements it — the Call on the windows-scm /
  launchd backends already errors on an unknown action, so this is defense in depth
- **server**: `POST /api/agents/{id}/services/daemon-reload` — the sub path has only one segment,
  so it is special-cased **before** the two-segment unit/action parsing (the unit action route is always two segments, no ambiguity);
  records a service_action audit (both resourceID and details.action are daemon-reload)
- **web**: a "Reload Config" button (shown only when isSystemd) sits on the table toolbar next to refresh,
  and calls the API after a Popconfirm confirmation; on success the list and status card refresh (unitFileState may have changed)
- **Tests**: the agent mock asserts that `systemctl daemon-reload` is invoked with argv passed directly; the server
  forwards + records exactly one audit entry, rejects GET/other methods, and the two-segment unit route is unaffected

### D13 Unit file viewing and editing (M2 extension)

Hand-tuning a unit (changing the restart policy / resource limits / dependencies) is routine work in service
management. Observation uses `systemctl cat` (the effective view = main file + drop-in merges), and the write semantics
**align with `systemctl edit --full`**:

- **Read** (`service.unitfile`, params `{name}`):
  - `systemctl show -p FragmentPath --value <name>` gets the real path
  - `systemctl cat <name>` gets the effective full text (including drop-in content)
  - Returns `{name, fragmentPath, content}`; errors if the unit is not installed (no
    FragmentPath)
- **Write** (`service.unitsave`, params `{name, content}`):
  - The path **does not accept a user parameter** — only the unit name is accepted, and FragmentPath decides where it is
    written, so path traversal is out of the question (the unit name whitelist has no `/` anyway)
  - When FragmentPath is not under `/etc/systemd/system/` (a package-managed file, overwritten on upgrade):
    first `cp the original file → /etc/systemd/system/<name>` (/etc has higher priority, systemd's
    official override mechanism, i.e. `edit --full` behavior) and then write
  - A temp file in the same directory + an atomic `rename`, 0644; content capped at 256KB
  - **Automatic `systemctl daemon-reload`** (bundled execution, changes take effect immediately), returning
    `{name, path, reloaded: true}`
- **server**: `GET/PUT /api/agents/{id}/services/{unit}/file` — reuses the two-segment route
  (GET/PUT does not overlap with the POST method of the unit action, no ambiguity); GET browsing records no audit,
  PUT records service_action (details.action=unitfile-save); the content size is
  capped at 256KB on the server side as well
- **web**: a "File" button in the action column (isSystemd) → a monospace-editor Modal (showing the
  effective full text of systemctl cat and explaining the write semantics), refreshing the list after saving
- **Tests**: the agent injects a Commander to control FragmentPath pointing at t.TempDir for real
  read/write assertions (including the /usr/lib → /etc copy branch, atomic write, bad-name rejection); server
  routing / auditing / size limits; web tsc + build