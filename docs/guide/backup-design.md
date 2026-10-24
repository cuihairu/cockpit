# P1 Design: Backup Management (Scheduled Backup + Retention Policy + Run History)


> 2026-09-14. First-phase design for P1 "Backup Management". Original todo.md text: "snapshots and
> recovery for database/config/volumes, a scheduled backup policy + off-site retention. This is the
> core gap in personal-cloud data safety". Design references: Proxmox vzdump (tar full + keep N
> copies), restic (incremental dedup, not introduced this round). This document covers only design
> decisions and interface definitions.

## Current State and Pain Points

| Pain point | Current state | Consequence |
|------|------|------|
| No backup capability at all | Nothing backup-related anywhere in the codebase | Docker volumes, config directories and SQLite files are scattered across agent hosts, so a disk failure loses them |
| Manual backups leave no record | The user SSHes in and runs tar by hand | When it was backed up, where it went, how big it is — nowhere to look it up |
| No retention policy | Deleting old archives by hand | Either the disk fills up, or you are afraid to delete anything |
| Failures go unnoticed | Manual backups skip verification | You believe you have a backup and only find out it is corrupt at restore time |

## Architecture Overview

The Agent-initiated outbound WebSocket connection model dictates that **the backup data flow must
not be relayed through the server** (RPC is unsuitable for GB-scale files). Backup files are
therefore **generated locally on the agent host and stored locally / on mounted storage** (NAS mount
points, a second disk, a local backup directory); the server is a pure control plane: it stores
configuration, computes the schedule, dispatches commands and records history.

```
┌─ server ───────────────────────────────────┐        ┌─ agent ─────────────────────────────────┐
│ BackupConfig table (which dirs, what time) │        │ backup provider                         │
│ backupLoop (scan due configs every minute) │ ─RPC─▶ │ backup.run → async task tar.gz          │
│ BackupRun table (run history/size/status)  │ ◀─RPC─ │ backup.task.get → poll status           │
│ failure → notification backup.failed       │        │ retention cleanup (keep N copies)       │
│ REST /api/backups + Web page               │        │ backup files stay on agent-side storage │
└────────────────────────────────────────────┘        └─────────────────────────────────────────┘
```

## Key Decisions

| # | Decision | Choice | Rationale |
|---|------|------|------|
| D1 | Where the backup runs | Agent-side local tar.gz, files stay in agent-side storage | The outbound WebSocket model cannot carry large payloads; a NAS mount point already delivers "off-site"; the server stays a pure control plane |
| D2 | Task model | Reuse the stack provider async task pattern (launchTask + task.get polling + cappedBuffer log) | A multi-GB tar takes minutes, far beyond the 30s CallAgent timeout; this pattern is already proven |
| D3 | Where scheduling lives | Server-side loop (scan due configs every 60s) | The server is the only global clock; agents carry no resident scheduling duty; after downtime recovery it back-fills one run automatically |
| D4 | schedule form | `manual` / `daily@HH:mm` / `every:Nh` (1–168h) | A personal cloud does not need full cron; the three forms cover "3am every night", "every 6 hours" and "manual only" |
| D5 | Retention policy | retention N copies; after backup.run succeeds on the agent side, clean up the oldest by the `{name}-*.tar.gz` glob | Cleanup happens on the filesystem that holds the backup, so the server needs no file access; N=0 means no cleanup |
| D6 | Backup content | Pack the whole source path list into one tar.gz (config directory / docker volume directory / sqlite file) | Full + retention is simple and reliable; live database backup (dump/snapshot) is recorded in M1.5 |
| D7 | capability | New `backup` capability, registered unconditionally by the Linux agent | Filesystem packing has no external dependency (the system tar is replaced by the Go standard library, zero dependencies); Windows does not register |
| D8 | Failure notification | Integrate notification.Service, send `backup.failed` on failure (goes through the cfg.Events allowlist) | Success does not notify (too noisy); reuses the multi-channel notification already built for probe enhancement |

## Agent-Side Design

### capability and registration

- The detector is untouched — the `backup` capability is appended directly by `agent.go` when
  GOOS==linux (the same unconditional idea as hardware-monitor), with a version number in metadata;
- `providers.go` setupProviders calls `registerBackupProvider` for `cap.Type == "backup"`;
  registration condition: GOOS==linux (the openwrt/busybox environment is unverified and does not
  block registration, see "Not doing").

### RPC Methods

| Method | Parameters | Returns | Sync/async |
|------|------|------|-----------|
| `backup.run` | `{configId, name, sources[], destDir, retention}` | `{taskId}` | Async start |
| `backup.task.get` | `{taskId}` | `{taskId, status, error, file, size, startedAt, finishedAt, log}` | Synchronous query |
| `backup.list` | `{dir}` | `{files: [{name, size, mtime}]}` | Synchronous |
| `backup.delete` | `{dir, name}` | `{}` | Synchronous |

- `configId` exists only to correlate logs / task records; the agent parses no scheduling semantics;
- `backup.list` lists only regular files under `{dir}` matching `*-*.tar.gz` (browse backup
  artifacts, not arbitrary directory contents);
- `backup.delete` accepts a file name only (not a path), and deletes it inside `{dir}` only after
  validation passes.

### Task execution flow (backupTask)

1. Validate parameters: `name` matches `^[a-z0-9][a-z0-9_-]{0,63}$`; `sources` is non-empty and
   every path is still absolute after Clean; `destDir` is an absolute path;
2. `os.MkdirAll(destDir, 0o700)`;
3. File name `{destDir}/{name}-{20060102-150405}.tar.gz`; error out if it already exists (repeat
   within the same second);
4. Pack source by source: each source uses its **basename as the top-level directory**
   (`etc-nginx/nginx.conf`), so that multiple sources cannot overwrite each other; during the walk,
   symlinks are recorded as the link itself (the archive/tar default, not followed); a source that
   is missing or unreadable is recorded as a warning and skipped while the other sources continue;
5. gzip BestSpeed (in a personal scenario wall-clock matters more than artifact size, configurable
   later); record size when finished;
6. retention>0: list `{destDir}/{name}-*.tar.gz` in descending mtime order and delete the oldest
   files past the Nth copy; the cleanup result goes into the task log (a failed delete is only a
   warning and does not affect task success);
7. Terminal state `success` / `failed` (with an error summary); the log goes into cappedBuffer
   (reusing stack's 200KB ring-buffer implementation); the in-memory task map + pruneTasksLocked
   reuse the stack pattern.

### Path Safety

- Every incoming path is validated with `filepath.Clean` + `filepath.IsAbs`; relative paths are
  rejected;
- The `name` of `backup.delete` must match `^[A-Za-z0-9][A-Za-z0-9._-]*\.tar\.gz$` (it cannot
  contain `/`, which rules out traversal at the root);
- The server performs the same validation before dispatch (see the server section) — defence on
  both ends.

## Server-Side Design

### Data model (storage)

```go
// BackupConfig — backup task configuration
type BackupConfig struct {
    ID         uint
    AgentID    string `gorm:"index;size:64"`
    Name       string `gorm:"size:64"`   // filename prefix, ^[a-z0-9][a-z0-9_-]{0,63}$
    Sources    string `gorm:"type:text"` // JSON array string ["/etc/nginx", "/var/lib/postgres"]
    DestDir    string `gorm:"size:512"`
    Schedule   string `gorm:"size:32"`   // manual / daily@HH:mm / every:Nh
    Retention  int                       // retained copies, 0 = no cleanup
    Enabled    bool
    LastRunAt  int64
    NextRunAt  int64                     // always 0 for manual
    LastStatus string `gorm:"size:16"`  // "" / running / success / failed
    CreatedAt, UpdatedAt int64
}

// BackupRun — a single run record
type BackupRun struct {
    ID         uint
    ConfigID   uint `gorm:"index"`
    TaskID     string `gorm:"size:64"`
    Status     string `gorm:"size:16"` // running / success / failed
    File       string `gorm:"size:256"` // backup filename (directory not included)
    Size       int64
    Error      string `gorm:"size:512"`
    StartedAt, FinishedAt int64
}
```

- `Sources` holds a JSON string, marshalled/unmarshalled at the application layer (no
  datatypes.JSON dependency introduced);
- storage provides `CreateBackupConfig / UpdateBackupConfig / DeleteBackupConfig (cascades to runs) /
  ListBackupConfigs / GetBackupConfig / CreateBackupRun / UpdateBackupRun /
  ListBackupRuns(configID uint, limit int)` (configID=0 queries all).

### Scheduling loop (server/backup_loop.go)

- `backupCheckLoop` (modelled on alertCheckLoop): ticks every 60s with
  `SELECT ... WHERE enabled AND next_run_at <= now`;
- When due and `LastStatus != "running"` (no concurrency for the same config):
  1. Compute and write back `NextRunAt` (**advance first, then execute**, so that a failed execution
     does not retry every minute and overwhelm the agent);
  2. Call `s.startBackupRun(config, "scheduled")` (shared with the manual run);
- `startBackupRun`: verify the agent is online + HasCapability("backup") → dispatch `backup.run` →
  create BackupRun{running} → set Config.LastStatus=running → start a goroutine `trackBackupTask`
  (reusing the trackStackTask pattern: poll `backup.task.get` every 5s, declare a timeout failure if
  there is no terminal state within 30 minutes, `backupTrackTimeout`) → on the terminal state, write back BackupRun
  (file/size/error/finishedAt) + Config.LastStatus/LastRunAt;
- Failed terminal state → `s.notifier.SendNonBlocking(&notification.Notification{EventType:
  notification.EventBackupFailed, Level: "error", ResourceType: "backup", ...})`;
- NextRunAt computation: `daily@HH:mm` → today/tomorrow at that time (server local time zone);
  `every:Nh` → now + N*h; computed in sync when a config is created/updated;
- On server startup, for enabled non-manual configs with `NextRunAt == 0` (legacy data / first
  enable) the next run time is initialised.

### REST API (server/api_backups.go, registerBackupsAPI, JWT protected)

| Method | Path | Description |
|------|------|------|
| GET | `/api/backups/configs` | Config list (Sources is parsed into an array on return) |
| POST | `/api/backups/configs` | Create; validates that the agent exists, that name/schedule/retention are legal, and that sources are all absolute paths |
| PUT | `/api/backups/configs/{id}` | Update; recomputes NextRunAt |
| DELETE | `/api/backups/configs/{id}` | Delete (cascades to runs) |
| POST | `/api/backups/configs/{id}/run` | Manual run (rejected with 409 while running) |
| GET | `/api/backups/runs` | Run history across all configs (limit default 50, max 200) |
| GET | `/api/backups/configs/{id}/runs` | Run history for that config (limit 50) |
| GET | `/api/backups/configs/{id}/files` | List files in the agent backup directory (forwards backup.list, dir taken from the config) |
| POST | `/api/backups/configs/{id}/files/delete` | Delete a single backup file `{name}` (validated against the filename regex, then forwards backup.delete) |

- Both file browsing and deletion are rooted at the config's destDir and **do not accept a
  directory from the client**;
- Audit: `ResourceBackup = "backup"`, actions create/update/delete/run/delete_file; audit data
  contains no backup content, and the sources paths are configuration information, so they may be
  recorded.

### Notifications and events

- Add `EventBackupFailed = "backup.failed"` to `notification/events.go`;
- config.yaml `events.backup-failed: {type: backup.failed, enabled: true}` — it is only sent when
  explicitly enabled (the same allowlist mechanism as service.down).

## Web UI

New page `web/src/pages/Backups/index.tsx` + route `/backups` + menu entry "Backup Management":

- **Config list** Table: name / agent / source paths (Tag + Tooltip) / destination directory /
  schedule (daily 02:00, every 6 hours, manual) / retention / status Tag (success green / failed
  red / running blue) / last run time / actions (run now, edit, delete Popconfirm);
- **Create/Edit** Modal: agent dropdown (online first), name, source paths (Select mode="tags", one
  entry per Enter), destination directory Input, schedule type Radio (manual/daily/interval) + time
  picker/hour count, retention copy count InputNumber(0-365), enabled Switch;
- **Run history** Drawer: viewed per config (status, file name, size, elapsed time, error message);
- **File browser** Drawer: lists backup files under destDir (name/size/time) + delete Popconfirm;
- An empty state guides the user to create the first backup config.

## M1.5 Design: Restore and Backup File Download

### Key decisions (M1.5 additions)

| # | Decision | Choice | Rationale |
|---|------|------|------|
| D9 | restore target | destDir **must not exist or must be an empty directory**; never overwrite existing data | A hard guarantee of "unpack into a separate directory": the restore artifact is physically isolated from live data, and moving it back into place is an explicit user action |
| D10 | restore double confirmation | The UI Modal requires typing the backup name + the API `confirmName` field must match `file` | The GitHub repo-deletion pattern: a dangerous operation forces explicit input, so a misclick can never trigger it |
| D11 | restore execution | Async task (reusing the backup task model) + Zip Slip protection | Unpacking a large archive takes minutes; path traversal when unpacking tar is a classic attack surface |
| D12 | Download channel | Chunked RPC `backup.read {offset, length}` (base64 chunks) streamed through the server | Reuses the existing outbound WebSocket with no protocol extension; the server writes nothing to disk and streams straight to the browser |

### New agent-side RPC

| Method | Parameters | Returns | Sync/async |
|------|------|------|-----------|
| `backup.restore` | `{dir, name, destDir}` | `{taskId}` | Async task |
| `backup.read` | `{dir, name, offset, length}` | `{data(base64), size, eof}` | Synchronous chunked |

- restore validation: dir/name/destDir follow the existing path-safety rules; after Clean, destDir
  must not be `/`; an existing non-empty destDir → rejected (D9); the backup file must exist;
- Unpack safety: for each entry, `filepath.Join(destDir, hdr.Name)` is forced to keep its prefix
  inside destDir (Zip Slip); reg/dir/symlink are handled as normal, other typeflags (device, hard
  link, etc.) are skipped with a warning; a single failing entry warns and continues, and the log
  records the entry count;
- `backup.read` caps a single chunk at 1MB (the server uses 256KB); it returns the total file size
  for progress and EOF determination.

### New server-side REST

| Method | Path | Description |
|------|------|------|
| POST | `/api/backups/configs/{id}/restore` | `{file, destDir, confirmName}`; confirmName ≠ file → 400; dispatches backup.restore + audit `backup_restore` |
| GET | `/api/backups/configs/{id}/tasks/{taskId}` | Forwards `backup.task.get` (polls restore task status/log, not persisted) |
| GET | `/api/backups/configs/{id}/files/download?name=` | Loops backup.read chunk by chunk and streams into the ResponseWriter (attachment), stopping as soon as the client disconnects, with a 15-minute total timeout |

- Downloads never touch the server disk; name goes through the same filename regex validation.

### Web UI (backup file Drawer enhancement)

- Row actions for each backup file: **Download** / **Restore** / delete;
- Download: axios blob (with JWT) + `URL.createObjectURL` to trigger the save;
- Restore Modal: shows file name/size/time, takes a destination directory (a separate new
  directory), and **the confirm button is only clickable after the backup name is typed** (D10);
  after submitting, the Modal switches to a task view that polls the task status and shows the tail
  of the unpack log, displaying success/failure on the terminal state.

## Not doing (later items)

- Off-site S3/object storage: ~~an agent-side rclone integration or a server relay, as a separate
  project~~ → already initiated as M2 (see below, 2026-09-19), taking the agent-side rclone route;
- The server's own DB backup (SQLite `VACUUM INTO`): a separate small feature;
- Database hot-backup hooks (mysqldump / pg_dump / sqlite .backup pre-hooks):
  ~~a separate later item~~ → already initiated as M3 (see below, 2026-09-19), the per-config
  pre-hook route;
- Incremental/dedup (restic/borg level) and backup encryption (age/gpg): for a personal scenario,
  full backups + retention are enough for now;
- openwrt/busybox environment adaptation: unverified; it does not actively block anything but is
  out of the acceptance scope.

## Phased Delivery

### M1 — Scheduled Backup + Retention Policy + Run History (completed 2026-09-14)

- [x] agent: `internal/agent/rpc/backup_provider.go` (run/task.get/list/delete + async task model +
      retention cleanup + path safety)
- [x] agent: backup capability registration (GOOS==linux) + providers.go wiring
- [x] storage: BackupConfig / BackupRun tables + CRUD (including cascade delete)
- [x] server: `backup_loop.go` scheduling loop (NextRunAt computation + dispatch when due + track +
      timeout + failure notification)
- [x] server: `api_backups.go` full REST surface + audit (ResourceBackup) + route registration
- [x] notification: BackupFailed constant
- [x] web: backup management page (config CRUD + run now + run history + file browse/delete) + route
      and menu
- [x] tests: agent provider unit tests (packing structure / retention cleanup / path traversal
      rejection / symlinks), storage CRUD and cascade, NextRunAt computation for every schedule form,
      all server API branches
- [x] documentation wrap-up + todo.md sync

### M1.5 — Restore and Backup File Download (completed 2026-09-14)

- [x] agent: `backup.restore` (unpack into a separate directory + Zip Slip protection + async task)
      and `backup.read` (chunked base64 read)
- [x] server: restore REST (confirmName double confirmation + audit `backup_restore`) + task polling
      relay + file download stream relay (Content-Length passed through transparently, 15-minute
      total timeout)
- [x] web: per-file row "Download/Restore" actions (axios blob download) + restore Modal (type the
      backup name to confirm + a task view after submit polling the unpack log)
- [x] tests: restore roundtrip / non-empty directory rejection / Zip Slip rejection (crafted
      malicious archive) / chunked read boundaries (cross-chunk + EOF + out of range) / server confirm
      validation and byte-for-byte comparison of the download stream relay

## M2: Off-site Retention — agent-side rclone (designed 2026-09-19)

### Pain points and scope

All of M1's backup artifacts land on the agent host's local storage (destDir / NAS mount point), so
the "off-site retention" from the original todo is still a gap: the backup file lives and dies with
the disk, and when the site is encrypted by ransomware the backup goes with it. M2 lands the first
item on the "Not doing" list (off-site S3/object storage): after a backup task packs successfully it
gains one extra step that pushes to a remote managed by rclone (S3/B2/OneDrive/WebDAV/sftp and dozens
of other backends). Incremental/dedup and backup encryption remain not done.

### Decisions

| # | Decision | Content | Rationale / notes |
|---|------|------|------------|
| D18 | Transfer route | Agent-side rclone exec (`rclone copy <file> <remoteDest>`); the server only stores config and status and never touches the data flow | D1 (data is not relayed through the server) forces the transfer to happen in place: a direct agent→remote push consumes no server bandwidth; a single rclone binary covers all common backends, and personal-cloud users most likely already have it; the server-relay route (pull back via backup.read then push to S3) would move GB-scale traffic, contradicting D1, and drags in a heavy AWS SDK — rejected |
| D19 | Credential boundary | Credentials for the rclone remote live only in rclone.conf on the agent host; cockpit neither touches, stores nor displays them | The existing discipline that secrets never land in cockpit; cockpit only stores the non-sensitive `remote:path` target string |
| D20 | Configuration model | BackupConfig gains `RemoteDest` (`remote:path` syntax, empty=disabled); BackupRun gains `RemoteStatus` (""/ok/failed) + `RemoteError` | Per-config granularity (different backups go to different remotes); AutoMigrate adds the columns automatically, zero migration |
| D21 | Failure semantics | A successful local pack means the task is success; an rclone failure only sets RemoteStatus=failed and sends a separate notification, without changing the task terminal state or triggering a retry | That the backup itself succeeded is a fact; a `remote_failed` terminal state would drag LastStatus/notifications/UI across the whole chain; once the remote recovers the next scheduled run naturally produces a new file; the historical gap is covered by manual re-push (D24) |
| D22 | Command and safety | `rclone copy --transfers 2 <localFile> <remoteDest>` (file name preserved); an exec.Command argv array, not through a shell; RemoteDest is validated against `^[A-Za-z0-9][A-Za-z0-9._-]*:[^\s]+$` (the `remote:` prefix corresponds to an rclone.conf section name, which naturally never starts with `-`, ruling out rclone mistaking it for a flag; the path segment forbids whitespace and control characters) | The only remaining injection surface is flag confusion, and the remote name syntax constraint eliminates it at the root; local file names keep using the existing regex allowlist |
| D23 | capability awareness | backup capability metadata gains `rclone: true` (present only when LookPath("rclone") hits); when the server creates/updates a config with a non-empty RemoteDest it verifies the target agent has rclone, otherwise 400 | Probing costs nothing; an agent without rclone is blocked at configuration time instead of only blowing up at run time |
| D24 | Manual re-push | `POST /api/backups/configs/{id}/files/sync-remote {name}` → agent `backup.remote.sync {dir, name, remoteDest}` (implemented in the same rclone stage as the automatic path; a synchronous RPC re-pushes small files within the 30s timeout); audit `backup_remote_sync` | After an automatic failure notification the user needs a closed loop inside the panel, otherwise they can only SSH; it is idempotent (rclone copy skips a file that already exists with the same size) |
| D25 | web | The config Modal gains an "off-site target" input (placeholder `my-s3:cockpit/backups`, explaining that the credentials live in the agent's rclone.conf); the run history Drawer gains an off-site status Tag; file rows gain a "Sync to remote" button (shown only when that config's RemoteDest is non-empty) | Purely presentational, reusing existing interactions, with no new interaction pattern |

### Execution flow (extra stage appended to agent backupTask)

After pack completes and retention cleanup runs, if `remoteDest != ""`:

1. The task log records `[remote] rclone copy <file> → <remoteDest>`;
2. exec rclone (the binary is injectable for testing); a stdout/stderr summary goes into the task
   log;
3. Success → task.RemoteStatus="ok"; failure → task.RemoteStatus="failed" + a RemoteError summary,
   while the task still ends in the success terminal state (D21);
4. The `backup.task.get` response gains `remoteStatus`/`remoteError`, and the server's
   trackBackupTask writes back BackupRun.RemoteStatus/RemoteError;
5. RemoteStatus=failed → the server sends a `backup.remote-failed` notification (the event allowlist
   must be explicitly enabled, the same mechanism as backup.failed).

`backup.remote.sync` (D24) reuses the same rclone stage function: validate name / file existence /
remoteDest format → copy → return `{synced: true}` or an error.

### Not doing (M2 re-confirmation)

- Incremental/dedup (restic/borg level) and backup encryption (age/gpg) — still not done;
- Creating/managing rclone remotes (`rclone config`) — the credential boundary (D19) keeps them out
  of the panel;
- Making the large-file upload timeout configurable — backupTrackTimeout currently provides one
  unified window; a separate project starts only once a real timeout case appears;
- The server-relay route and raw WebDAV/sftp protocol implementations — rclone already covers them.

### M2 checklist

- [x] agent: backup provider rclone stage (binary injection + log + RemoteStatus) +
      `backup.remote.sync` RPC + capability metadata
- [x] server: config validation (RemoteDest format + rclone awareness) + track write-back +
      `backup.remote-failed` notification + the sync-remote endpoint and audit
- [x] storage: BackupConfig.RemoteDest / BackupRun.RemoteStatus+RemoteError fields
- [x] web: Modal off-site target + history Tag + file row re-push button + tsc/build
- [x] tests: agent fake rclone (argv assertion / success / failure / disabled path), server validation
      and end-to-end, notification triggering
- [x] documentation wrap-up (this checklist checked) + todo.md sync

**Live acceptance (outstanding)**: a real S3/B2 remote, rclone against a rate-limited cloud drive,
and the full chain for a GB-scale file.

**M2 complete (2026-09-19)**. Two implementation deviations recorded:

1. **When rclone existence is probed (D23)**: the `rclone` field of the capability metadata is
   probed via `exec.LookPath` during the agent's startup detection phase and reported together with
   the registration; a reconnect reuses the cache and does not re-probe — installing rclone on the
   agent host afterwards requires **restarting the agent process** before the metadata refreshes, and
   until then configuring an off-site target on the server is blocked with 400 (the copy already
   tells the user to install rclone).
2. **`pollBackupTask` signature refactor**: the return value changed from loose parameters to a
   `backupTaskResult` struct (which carries the new RemoteStatus/RemoteError fields), with no
   behaviour change; the front end uses the lenient validation for the remoteDest input (forbidding
   whitespace only), while control characters are stopped by the same regex on both the server and
   the agent.

## M3: Database Hot-Backup Hook — pre-hook (designed 2026-09-19)

### Pain points and scope

M1's source paths already allowed pointing directly at a SQLite file
(`docs/guide/backup-design.md` D6), but tarring a database file that is being written to can yield an
inconsistent snapshot — while writes are in flight, pages are out of sync, and after restore
`.integrity_check` fails and the application will not start. In a personal scenario SQLite is
everywhere (this panel itself uses it), MySQL/PG only occasionally. Packing a database in the middle
of a write is the last silent gap in the data-safety chain: a backup that is "successful" but whose
archive is corrupt is more dangerous than a failure.

Approach: **an optional per-config pre-hook command** — the agent executes a user-configured shell
command before packing (typical: `sqlite3 /data/app.db ".backup /tmp/app.db.bak"`,
`mysqldump ... > /tmp/db.sql`) and the source paths point at the snapshot artifacts; a hook failure
aborts this backup outright (an inconsistent snapshot is worse than no backup). No post-hook (once
packing is done there is no external party to notify, and the notification system already exists).

### Decisions

| # | Decision | Rationale |
|---|------|------|
| D26 | Mechanism = a free-form per-config command `PreHook string` (empty=do not run), executed via `sh -c` before the agent packs; no built-in mysql/pg client integration | The user knows their own database best (connection string/credentials/dialect), and a free-form command covers sqlite .backup / mysqldump / pg_dump / etcdctl snapshot in one stroke; built-in integration would be endless dialect adaptation |
| D27 | `sh -c` rather than argv tokenisation | The hook command needs redirection and quoting semantics (`.backup "path with space"`, `> /tmp/db.sql`); the cron provider writing back to crontab is the same precedent, and the agent is a trusted control plane to begin with |
| D28 | Hook failure (non-zero exit) → the task fails: no packing, no remote push, retention untouched | Consistency is the only reason the hook exists; carrying on while broken to emit corrupt archives defeats the purpose. The error message contains the exit code and a stderr summary (≤512 bytes, the same style as pushRemote) |
| D29 | Hook timeout fixed at 5 minutes, `exec.CommandContext` killing the whole process group | Dumping a large database can take minutes but cannot be waited on forever (the task still has packing + pushing to do); 5min covers a GB-scale pg_dump; not made a config option (YAGNI, revisit when a real need appears) |
| D30 | Output goes into the task log with a `[hook]` line prefix; the hook runs inside nameLock/sem (same ordering as packing) | Troubleshooting only needs the log to reconstruct the hook scene; running it under the lock guarantees that two runs of the same config do not stomp on each other's hook artifacts (the same /tmp snapshot path) |
| D31 | Configuration model: `BackupConfig.PreHook` (size:1024) + no new fields on `BackupRun` (a hook failure simply means the task failed, and Error already carries it); AutoMigrate means zero migration | The single run.Error field is enough to locate the problem; no hook state machine is introduced |
| D32 | The server passes `preHook` straight through on dispatch; the web config Modal gets a "Pre-command (optional)" TextArea + explanatory copy; there is no server-side command validation | `sh -c` on the agent side is already full shell semantics, so server-side validation could only be false security; the 1024 length is constrained by the gorm size + a too-long rejection on the agent side |

### Execution flow (extra stage appended before packing in agent backupTask)

After nameLock/sem are acquired and before pack, if `preHook != ""`:

1. The task log records `[hook] <command>`;
2. `exec.CommandContext(ctx, "sh", "-c", preHook)` with a 5min timeout; a CombinedOutput summary
   (≤2048) goes into the `[hook]` log;
3. Non-zero exit / timeout → the task fails immediately (Error=`pre-hook failed: ...`), the terminal
   state is returned and pack is not entered;
4. Success → continue with pack → retention → (M2) the rclone push, the original flow unchanged.

restore does not involve the hook (unpacking goes into a separate directory, so there is no
consistency problem).

### Not doing (M3 re-confirmation)

- Post-hook, multi-hook chains, configurable hook timeout — YAGNI;
- A built-in database provider (aware of the mysql/pg protocols) — rejected in D26;
- Server-side command allowlist / audit enhancement — the hook is configuration content, and config
  changes are already covered by the backup_update audit, so the command itself is not audited
  separately (same level as sources/destDir).

### M3 checklist

- [x] agent: backupTask gains PreHook + runHook (sh -c + timeout + log) + short-circuit without
      packing on failure + parameter validation (≤1024)
- [x] server: preHook passed through in the dispatch parameters; config validation (length);
      view/request body field
- [x] storage: BackupConfig.PreHook field (AutoMigrate, zero migration)
- [x] web: config Modal "Pre-command (optional)" TextArea + placeholder example + explanation
- [x] tests: agent fake hook (success path / sh -c semantics / failure short-circuit without packing
      / timeout kill); server pass-through and validation
- [x] documentation wrap-up (this checklist checked) + todo.md sync

**M3 complete (2026-09-19)**. Two implementation deviations recorded:

1. **PreHook is carried in a RunBackup local variable** rather than a backupTask field: the GetTask
   response does not need to echo the hook command back (the server does not poll it, and the reason
   for the failure is already in Error), which removes one concurrently read/written field.
2. **D29's whole-group kill landed as platform-split files**: `backup_hook_unix.go` (`Setpgid` +
   `Kill(-pid)`) and `backup_hook_other.go` (pass-through) — the backup capability is registered on
   linux only, and the non-unix file exists purely so the build compiles on all platforms.

## References

- [Proxmox VE vzdump](https://pve.proxmox.com/wiki/Backup_and_Restore) — the resource scheduling
  pattern of tar full backups + keeping N copies
- [restic](https://restic.net/) — the standard design for incremental dedup backup (not introduced
  this round, a candidate for later off-site work)
- [rclone](https://rclone.org/) — the de facto standard for multi-cloud storage sync (the M2
  transport layer)
- Internal: [stack-deploy-design.md](/guide/stack-deploy-design) (the source of the async task model),
  [probe-enhance-design.md](/guide/probe-enhance-design) (multi-channel notification integration), the
  P1 backup management entry in `todo.md`
