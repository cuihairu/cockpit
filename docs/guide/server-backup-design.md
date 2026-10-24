# Server's Own Database Backup Design (backup management follow-up item)


> Initiated 2026-09-15. Corresponds to the backup-management leftover todo item "backup the server's own SQLite (VACUUM INTO)". Prerequisite: `backup-design.md` M1/M1.5 (agent-side file backup already closed out).

## Pain points

Backup management currently covers only **agent-side files** (config/database/volume packing). But the panel's own SQLite database — the Git-first CMDB's full set of resource records (Agent/Domain/Certificate/Service/Gateway/Storage), users and permissions, audit logs, alerts, backup configuration, probe history, **session recording metadata** — has no backup at all. One disk failure or one mishandled `cockpit.db`, and all management-plane data goes to zero — worse, agent-side backups depend on exactly this configuration to know what to back up.

## Approach

SQLite's native `VACUUM INTO 'path'`: online (no shutdown, no blocking read lock) produces a **compact complete copy** of the current database; the artifact is an ordinary `.db` file that can be renamed directly into place for recovery. Server-side-only change, zero agent awareness.

## Decisions

| # | Decision | Content | Rationale / notes |
|---|------|------|------------|
| D1 | Backup method | `VACUUM INTO` (the storage layer exposes `VacuumInto(path)`), not file copy | File copy can produce torn pages under concurrent writes; VACUUM INTO is SQLite's officially recommended online backup path, and the artifact also defragments as a bonus |
| D2 | Storage | `<db dir>/server-backups/cockpit-YYYYMMDD-HHMMSS.db` (0600); **the directory is the source of truth, no DB table** | The filename carries its own timestamp, so listing = scanning the directory; no per-record field needs (unlike BackupRun, which needs a task state machine) |
| D3 | Scheduling | A standalone `serverBackupLoop` (1h ticker): read the interval from Settings, scan the directory's newest file timestamp, and run only when the gap ≥ the interval | The same "ticker + timestamp comparison, interval dynamically changeable" pattern as the drift inspection; not attached to the agent backup loop (that one is per-agent config-driven dispatchDue) |
| D4 | Configuration | Settings `server_backup.interval_hours` (0=off, 1-168, default 24) + `server_backup.retention_days` (0=forever, 1-365, default 7); `GET/PUT /api/server-backups/config` | Global-config Settings pattern (same as probe/drift); default is a backup per day |
| D5 | Manual trigger | `POST /api/server-backups/run` backs up immediately (same path reused), recorded in audit | The disk-full/accidental-delete case where you want a copy right now; does not interfere with the loop (a same-second conflict simply errors) |
| D6 | REST | `GET /api/server-backups` (list: filename/time/size), `GET .../{name}/download`, `DELETE .../{name}`; download/delete recorded in audit, list/manual backup not | Same discipline as backup-file download (download goes through a JWT blob, changes recorded in audit); a manual backup is a protective action, not a sensitive read |
| D7 | Filename safety | Accept only the strict pattern `cockpit-\d{8}-\d{6}\.db`; everything else 400 | Download/delete address files by name, so `../` traversal and arbitrary file deletion must be prevented |
| D8 | Recovery | M1 has no online recovery: download the artifact → stop the service → replace `cockpit.db` → start (explained in README/docs) | Hot-swapping a SQLite database that is being written to will inevitably corrupt it; stop-and-replace is the only safe path |
| D9 | Failure semantics | VACUUM INTO failure is logged and retried next round; no alert | Environmental issues like a full disk; logging suffices (handled at the same level as the drift error state) |
| D10 | Web entry | The `/backups` page gets a "panel database" section card: config row (interval/retention) + a backup-now button + file list (download/delete) | Same usage scenario (backup); no new page |

## Not doing (later versions)

- Online recovery / one-click restore (D8);
- ~~Off-site transfer (S3/rclone, to be done together with the same route as agent backup off-site)~~ → M2 initiated (2026-09-19; the prerequisite route backup-design.md M2 rclone has landed);
- Custom-time scheduling (daily@HH:mm; hour-based intervals are sufficient).

## M2: off-site transfer — rclone (designed 2026-09-19)

### Pain points

M1's local backup and `cockpit.db` are on **the same disk**: mishandling can be rolled back, but if the disk dies the local backup dies with it — only an off-site copy can truly back up the pain point above, "all management-plane data goes to zero". backup-design.md M2 already landed the rclone push route on the agent side (remote:path validation, argv not through a shell, independent semantics for local success with push failure); the server side reuses the same route.

### Decisions

| # | Decision | Content | Rationale / notes |
|---|------|------|------------|
| D11 | Transfer method | The server process directly execs `rclone copy --transfers 2 <file> <remoteDest>`, argv not through a shell | Same as agent D18; the user installs rclone on the server host and configures rclone.conf themselves; cockpit is just the caller |
| D12 | Credentials | Only in rclone.conf on the server host (rclone's default path); cockpit stores and displays no credentials | Symmetric with agent D19; the API/Web only ever shows the remote:path target string |
| D13 | Configuration | Setting `server_backup.remote_dest` (empty=off); validated **on write** with the same regex as agent D22, `^[A-Za-z0-9][A-Za-z0-9._-]*:[^\x00-\x20\x7f]+$`, plus a 512-byte cap; on read an invalid value is treated as unconfigured and logged | A remote name not starting with `-` eliminates flag confusion (same origin and rule as agent D22); lenient on read, strict on write — a dirty value does not block local backup |
| D14 | Timing | Synchronous push after `runServerBackup` succeeds (the scheduled loop and manual trigger are naturally both covered via the same path); push failure only logs + notifies, without affecting the backup API response or the local artifact | Local success means there is a copy; push is an enhancement (symmetric with D21 semantics); a single-file copy takes seconds, synchronous within HTTP is acceptable |
| D15 | Timeout | `exec.CommandContext` with a 5-minute timeout (a package-level variable, shortenable in tests) | Synchronous execution inside a handler must not hang the request forever; same value as the agent pre-hook timeout (M3 D29) |
| D16 | Status recording | **Not persisted** (D2's "directory is the source of truth" stands): the push result is logged in a `[remote]` log line; on failure a notification is sent (new event `server_backup.remote-failed`, explicitly enabled via allowlist, skipped when the notifier is nil) | No per-file status table; the notification is the only user-visible failure surface (backup M2 D21 precedent) |
| D17 | Manual re-push | `POST /api/server-backups/{name}/sync-remote`: push the specified local file to the **current** remote_dest, recorded in audit (server_backup sync_remote) | Symmetric with agent D24; the remediation path after receiving a failure notification; rclone copy is idempotent (same size skips) |
| D18 | Detection and remote | config GET returns `rclone_available` (a live `exec.LookPath` probe each time); no retention cleanup on the remote | A single server process has no reporting path, and live detection means installing rclone takes effect without a restart (simpler than agent D23's startup detection; exec itself goes through PATH each time anyway); remote cleanup risks deleting the only off-site copy — in a personal scenario the remote has ample capacity, so documenting it suffices |

### Execution flow

```
serverBackupLoop expiry / POST /run
  └─ runServerBackup()          VACUUM INTO local artifact (unchanged)
       └─ pushServerBackupRemote(name)   when remote_dest is non-empty
            ├─ exec rclone copy (5min timeout)
            ├─ ok     → [remote] log, done
            └─ failed → [remote] log + server_backup.remote-failed notification
```

### M2 checklist

- [x] notification: `ServerBackupRemoteFailed = "server_backup.remote-failed"` constant
- [x] server: `server_backup.go` — remote_dest Setting read/write/validation, pushServerBackupRemote (timeout/log/notification), runServerBackup wiring, rclone_available detection
- [x] REST: config GET/PUT adds remote_dest + rclone_available; `POST /{name}/sync-remote` (filename validation reuses the D7 strict pattern + audit)
- [x] web: the ServerBackupCard config row adds an "off-site target" input (validation uses the same regex) + an rclone-not-installed hint; the file operations column gets a "re-push" button (shown only when remote_dest is configured)
- [x] tests: server push ok/failure notification/unconfigured skip/sync-remote full path/regex validation/timeout (fake rclone script, same pattern as agent)
- [x] documentation wrap-up (this checklist checked) + todo.md sync

M2 complete (2026-09-19): 7 new server tests all green + no notification regression + web tsc with zero new errors (9 pre-existing) + build passing. Deviation between implementation and design: beyond `exec.CommandContext`, the D15 timeout adds `cmd.WaitDelay = 1s` — testing exposed that the stdout pipe write end of `CombinedOutput` gets inherited by rclone's child process (the fake script's sleep; real rclone normally has no child-process chain), so `Wait` waits for pipe EOF and gets dragged by the orphan until the grandchild exits, rendering the timeout moot; WaitDelay forcibly breaks the pipe on expiry, restoring the semantics — no need for agent M3 D29's platform-split process-group kill (no free-form command hook scenario on the server side).

## M1 checklist

- [x] storage: `VacuumInto(path)` + unit test (artifact openable, tables present)
- [x] server: `server_backup.go` (Setting read/write + directory utilities + backup execution + retention cleanup + scheduling loop) + server.go startup wiring
- [x] REST: `api_server_backup.go` list/download/delete/manual backup/config + audit + serveAPI hookup
- [x] web: /backups page "panel database" card (config + backup now + list)
- [x] tests: VacuumInto artifact validity, filename pattern rejection, full API paths, retention cleanup, interval semantics
- [x] documentation wrap-up (this checklist checked) + todo.md sync

M1 complete (2026-09-15): 6 server tests all green — the VACUUM INTO artifact reopened with real GORM reads data written before the backup (hard integrity verification) with mode 0600; the strict filename pattern rejects `../` traversal and malformed names; config semantics (default 24h/7d, 0=off or forever, out-of-range/invalid falls back to defaults); retention cleanup deletes expired and keeps forever ones; full API paths (config GET/PUT incl. out-of-range 400, immediate backup, list, download, delete, 404/400, four categories of audit actions persisted).

## References

- Internal: [backup-design.md](/guide/backup-design) (agent-side backup and task model), [recording-design.md](/guide/recording-design) (retention cleanup and download audit patterns)
- External: SQLite documentation Backup API / VACUUM INTO (3.27+)
