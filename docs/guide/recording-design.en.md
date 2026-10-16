# Remote Session Recording Design (M1: terminal output recording + playback)

> Language: [中文](./recording-design.md) ｜ **English**

> Proposed 2026-09-15. Corresponds to the todo item "remote session recording: terminal output recording,
> strengthening remote-access audit (the current audit only has session start/end)". Approach references:
> Guacamole session recording, Teleport session-granularity audit.

## Pain point

The audit of remote access (SSH/telnet terminals) currently only has **two records, session start/end**
(`auditRemoteStart` / `auditRemoteEnd`): you know who connected to which host at what time, but
**what happened inside the session is completely untraceable**. In a personal cloud setup, all administrators'
terminal operations funnel through this one entry point, and when something goes wrong (a file deleted by
mistake, a config edited wrong) the scene cannot be reconstructed.

## Architecture

Terminal data already flows entirely through the server (the Agent dials out over WebSocket, and the browser ←
server ← agent path is bidirectionally forwarded), so **the recording hangs off the server-side forwarding
pipe** and the agent needs zero changes:

```
browser ──input──▶ terminalSendLoop ──▶ agent
browser ◀─data─── HandleTerminalData ◀── agent
                      │
                      └─▶ castRecorder.Write() ──▶ data/recordings/<sid>.cast
                                                     + SQLite metadata
```

- `api_remote.go:341 HandleTerminalData`: the single exit point from agent to browser, and the recording hook point;
- `closeTerminalSession` / `HandleTerminalClose`: the session terminal states, where finalize backfills the duration and byte count.

## Decisions

| # | Decision | Content | Rationale / notes |
|---|------|------|------------|
| D1 | M1 scope | Terminals only (SSH/telnet sessions via `/api/remote/terminal`); VNC/RDP pixel streams are not recorded (this judgement was overturned by M3, see below) | The terminal stream is naturally a sequence of character events, and the asciinema format is the industry standard for it; pixel stream recording is heavy in size and complex to replay, for low value |
| D2 | Recording location | The server-side forwarding pipe, zero agent changes | The data already flows through the server; recording agent-side would drag in file management and an upload chain |
| D3 | Format | asciinema v2 (`.cast`): a JSON header on the first line, then one line per event `[relative seconds,"o","data"]` | An open industry format that can be played/converted outside Cockpit with `asciinema play`; line-append writing naturally suits streaming |
| D4 | Output only | The input stream is not recorded (no `"i"` events) | Input contains plaintext passwords (SSH/sudo), so recording it leaks the source; Guacamole likewise records only the output side. What audit wants is "what appeared on the terminal", not "what keys were pressed" |
| D5 | Storage | File `<db dir>/recordings/<sessionID>.cast` + a new table `TerminalRecording` (unique session_id, username, agent_id, host, port, protocol, started_at, duration_ms, bytes) | Files hold the content, the DB holds the index (same idea as separating backup files from config); duration/bytes are backfilled at the end |
| D6 | Toggle and retention | Setting `recording.enabled` ("true"/"false", default true) + `recording.retention_days` (0-365, default 7, 0 = forever); cleanup hangs off cleanupLoop waking every 30s, throttled to hourly execution | On by default (the closed audit loop is the selling point) + bounded retention to prevent disk bloat; same "read the Setting in the loop, no caching" approach as the drift inspection |
| D7 | Write strategy | Direct `os.File` writes (no fsync), one line per event; on write failure log once and stop recording, **never affecting the forwarding main chain** | Terminal output arrives in small low-frequency chunks, so losing the tail on a crash is acceptable; remote-access availability outranks recording completeness |
| D8 | finalize idempotency | A `sync.Once` inside the recorder; both `closeTerminalSession` and `HandleTerminalClose` call Close (the latter path arrives late by ≤30s via the keepalive-ping failure fallback, calling both directly backfills immediately) | A session has two end entry points; Once prevents a double backfill |
| D9 | REST | `GET /api/recordings` (reverse-chronological list) `GET /api/recordings/{id}/cast` (file stream, i.e. the playback data source, also serving downloads) `DELETE /api/recordings/{id}` (deletes file and record together) | Playback and download take the full content from the same endpoint, with audit recorded uniformly |
| D10 | Audit | `recording_read` (fetching cast content) and `recording_delete` are audited; browsing the list is not | Recorded content contains historical output (possibly sensitive); who took the content should be traceable; the list is an index, not content |
| D11 | Player | Developed in-house on the web: lightweight — fetch .cast → parse line by line → schedule `term.write()` by time delta, with 1/2/4/8x speeds and drag-to-seek | No asciinema-player dependency; the project already has @xterm/xterm |
| D12 | Size | The cast header is fixed at 80x24; the playback xterm adapts to the container without locking the column width | The server side does not track resize (the session does not store rows/cols); the asciinema v2 header is also single-size, and misalignment is acceptable |
| D13 | Web entry | A new `/recordings` "Session Recordings" page, with the menu item next to "Audit Log" | Recording is an extension of audit, so the entries live together |

## Not doing (later versions)

- VNC/RDP desktop stream recording (WebM/image sequences, large size and heavy replay);
- Input event recording (password leak risk, see D4);
- Live spectating (live-tailing an in-progress session);
- ~~Export/archive of recording files (S3/rclone, the same route as off-site backup, do it together with backup M2)~~
  → M2 was proposed (2026-09-19; the prerequisite routes, backup-design.md M2 and the rclone mode of server-backup-design.md M2, have both landed).

## M2: off-site recording archive + config entry (designed 2026-09-19)

### Pain point

Recordings are the only trace left by remote-access audit, but local `.cast` files are deleted by retention
after 7 days by default and share a disk with cockpit.db — when the disk fails, the audit records vanish
together with the database. Backup M2 (agent rclone) and server backup M2 have already established a
complete remote:path push pattern; recording archiving reuses the same route: push off-site as soon as a
recording finishes, retention only deletes locally, and the remote becomes the long-term audit archive.

It also closes an M1 gap: the two Settings `recording.enabled` / `recording.retention_days` still have no
REST/UI write entry (you have to edit the database directly) — M2 adds the config endpoints too.

### Decisions

| # | Decision | Content | Rationale / notes |
|---|------|------|------------|
| D14 | Config | Setting `recording.remote_dest` (empty = off), validated on write with the same source as server backup M2 D13 (remote:path regex + 512 cap), dirty values treated as unconfigured on read | One rule across the three backup/archive modules; credentials live only in rclone.conf on the server host, never in the DB |
| D15 | Shared executor | Extract `rcloneCopyLocalFile(local, remoteDest)` (exec copy --transfers 2 + 5min timeout + WaitDelay hard pipe break + stderr summary), shared by the server backup push and the recording archive; the package-level var is renamed `serverRcloneBin` | The two are isomorphic down to the letter; extracting eliminates drift; WaitDelay semantics see the server-backup M2 delta note |
| D16 | Timing | Push **asynchronously** (goroutine) in the finish callback where recording `Close` backfills metadata: it does not hold up the session exit path; the `.cast` is a complete closed file, so a single-file copy takes seconds | A synchronous exec on the session close path would delay the ws cleanup; recording files need no "abort on push failure" semantics (D28's counter-example — a late audit trail is acceptable, losing it is fatal) |
| D17 | Failure semantics | Log `[remote]` + a new event `recording.remote-failed` (explicitly enabled via the whitelist): the local file remains and can be manually re-pushed within the retention window; a successful push is not notified | Symmetric with backup.remote-failed / server_backup.remote-failed; after retention deletion a failed file is permanently lost, so the notification is the only perceivable surface |
| D18 | Manual re-push | `POST /api/recordings/{sid}/sync-remote`: push per the current remote_dest, audited as `recording sync_remote` (action=update) | Symmetric across the three modules; sid is the metadata primary key with inherently no traversal surface, 404 when the file does not exist |
| D19 | Config endpoints | `GET/PUT /api/recordings/config`: enabled (bool) / retention_days (0-365) / remote_dest (regex-validated) + live rclone_available probing; a config row at the top of the web /recordings page | Closes the M1 gap (Settings have no write entry); config lives with the use case instead of in global Settings (same as drift D18) |

Retention only deletes local files and records (the remote archive is not cleaned — rclone copy is idempotent
and does not delete on the remote, and cleaning the remote risks deleting the only archive, which is enough to document).

### M2 checklist

- [x] notification: the `RecordingRemoteFailed = "recording.remote-failed"` constant
- [x] server: extract `rcloneCopyLocalFile` (server backup switched to it, behaviour unchanged) +
      recording remote push (async on Close) + the `recording.remote_dest` Setting
- [x] REST: `/api/recordings/config` GET/PUT + `/{sid}/sync-remote` (audited)
- [x] web: config row on the /recordings page (toggle/retention/remote destination/rclone hint) +
      a "re-push" button in the list + api/types
- [x] tests: archive push ok / failure notification / skip when unconfigured / all config validation shapes / all sync-remote
      paths / Close async not blocking
- [x] documentation wrap-up (this checklist ticked) + todo.md synced

M2 done (2026-09-19): all 6 server tests green (fake rclone argv injection following the same
pattern as server backup M2). Two deltas at landing: ① D18's design assumed "sid is the metadata primary key
with inherently no traversal surface", but the implementation still added a strict UUID regex (`^[0-9a-f]{8}-…`)
as a 400 precondition — defence in depth at zero cost, since a non-UUID form never reaches file addressing;
the tests also cover deep traversal forms being blocked at 404 by the router's segment check. ② D19's Setting
key was written as `recording_remote_dest` in the checklist draft but landed uniformly as the dotted style
`recording.remote_dest` (isomorphic with `recording.enabled` / `recording.retention_days` / `server_backup.remote_dest`).
Real-machine remainder: verification of pushing to a real rclone remote (e.g. gdrive/S3) — the local fake rclone
already covers argv and failure semantics.

## M1 checklist

- [x] storage: the `TerminalRecording` model + migration + Create/Finish/List/Get/
      Delete/DeleteExpiredUntil
- [x] server: `recording.go` (castRecorder read/write + Setting read/write + expiry cleanup
      hooked into cleanupLoop) + terminal pipeline wiring (create / write output / idempotent finalize)
- [x] REST: `api_recordings.go` list / get content / delete + audit + serveAPI integration
- [x] web: the `/recordings` page (list + xterm playback Modal with speed control + download/delete)
      + routing menu
- [x] tests: cast format and finalize idempotency, API CRUD and audit, expiry cleanup,
      the HandleTerminalData persistence chain
- [x] documentation wrap-up (this checklist ticked) + todo.md synced

M1 done (2026-09-15): all 5 server tests green — cast format (header version=2 +
`[dt,"o",data]` lines, idempotent Close backfilling once, nil receiver safety, file mode 0600),
startRecording metadata registration and pipeline persistence, toggle semantics (on by default / "false" off, retention days
valid/0/invalid falling back to default), expiry cleanup (file and record deleted together, 0 = forever), API (list / get content
Content-Type / 404 / 405 / delete / audit view+delete persisted). The player was developed in-house: `term.write` scheduled by
setTimeout according to each event's time delta, switching to 1/2/4/8x speed replays, with a progress display.

## M3: Guacamole desktop session recording (landed 2026-09-30, overturns M1 D1)

M1 D1 judged "VNC/RDP pixel streams are not recorded", but guacd ships session recording of its own (the
`recording-path` / `recording-name` connect parameters, `Format=guac`), nearly free, so M3 landed it too:

- **All guacd protocols are recorded**: `internal/server/api_guacamole.go:165-171` sets `recording-path` / `recording-name` / `create-recording-path=true` for RDP/VNC/SSH alike, turning it off only when `recording.enabled=false`; the format is `.guac` (an instruction stream, not pixel frames), and playback goes through the web-side GuacPlayer (working around the upstream guacamole-common-js 1.5.0 `SessionRecording(Blob)` defect by duck-tunneling the instruction stream).
- **Unified ledger**: recording metadata is registered when the session starts (`Format=guac`, `recording.go` / `storage/recording.go`), while terminal asciinema recordings use `Format=cast` (empty value stays compatible with old data); both share the `/recordings` list and `/api/recordings/{sid}/cast` content endpoint.
- **Collection chain**: `collectGuacRecording` moves the file from the guacd container volume into the server recording directory, landing alongside terminal recordings in `<db dir>/recordings`; M2's off-site archive (`recording.remote_dest`) is shared by both.
- Acceptance evidence: probe S10 + browser CDP C4 (2026-09-30, `.acceptance/guac/evidence/`), with the real-machine checklist in the "Remote access three protocols" section of `docs/guide/acceptance-checklist.md`.

## References

- Internal: [logs-design.md](./logs-design.en.md) (the counter-example of server-side forwarding not touching disk —
  this feature is precisely the "does touch disk" side of audit), the audit chapter of architecture.md
- External: the asciinema v2 format spec (<https://github.com/asciinema/asciinema/blob/develop/doc/asciicast-v2.md>),
  Guacamole session recording, Teleport session recording