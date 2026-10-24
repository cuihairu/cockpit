# P1 Design: File Manager (Agent Remote File Browse / Edit / Upload / Download)


> 2026-09-15. First-phase design of the P1 "File Manager". Original todo.md text: "Remote file browsing/
> upload/download via Agent (Workbench currently has only terminal and desktop), supporting config
> editing and log file viewing".
> This document covers only design decisions and interface definitions.

## Current State and Pain Points

| Pain point | Current state | Consequence |
|------|------|------|
| Editing a config file requires SSH | Workbench has only terminal/desktop; editing nginx.conf means `vi` | High-friction for a frequent action; essentially unusable on mobile |
| Viewing logs means typing commands | Manual scrolling with `tail -f` / `less` | No search, no highlighting, no download |
| Transferring files needs scp | Typing commands back and forth between local machine ↔ server | The classic blunder spot: forgotten paths, permission problems |
| Dangerous operations unaudited | rm/mv live only in shell history | No traceability when things go wrong |

## Architecture Overview

Reuse the channel proven by backup M1.5: **chunked RPC base64 over the outbound WebSocket, with the
server purely forwarding and never touching disk**
(the file manager has no large-file relay need — editing/uploading are KB–MB scale,
and downloads use streamed chunked pulls).

```
┌─ server ───────────────────────────────────────┐        ┌─ agent ────────────────┐
│ REST /api/agents/{id}/files                    │ ─RPC─▶ │ file provider (new)    │
│ param validation + audit (mutating ops)        │ ◀─RPC─ │ list/read/write/mkdir/ │
│ download streamed through (never touches disk) │        │ delete/rename          │
└────────────────────────────────────────────────┘        └────────────────────────┘
```

## Key Decisions

| # | Decision | Choice | Rationale |
|---|------|------|------|
| D1 | Visible scope | Free navigation across the agent's full filesystem; no root-directory whitelist | The agent already has a terminal (equivalent to root power); a whitelist creates a false sense of security and blocks legitimate scenarios |
| D2 | M1 capability boundary | Browse / read / edit+save / delete / rename / mkdir / upload (≤10MB) / download (no size limit); no chmod/chown, search, or directory size stats | High-frequency ops actions first; permission changes are risky with few scenarios and easy to add later (chmod/chown were added by M4, search by M2) |
| D3 | Transport channel | `file.read` / `file.write` chunked base64 RPC, reusing the outbound WebSocket | Isomorphic to `backup.read`; zero protocol extension; the server never touches disk |
| D4 | Read channel split | The browser **editor** uses `read` (≤1MB text, rejected beyond); **download** uses the server looping read in 256KB chunks for stream forwarding (no total size limit, stops on disconnect) | Editing large text would needlessly freeze the UI; streamed downloads naturally support GB-scale logs |
| D5 | Write/upload | `file.write {path, data, truncate}`: truncate=true overwrites (edit-save/new file), false appends (upload chunk resume); single chunk ≤1MB, client-side whole file ≤10MB | M1 has no resume support; 10MB covers config/certificate/small-log scenarios |
| D6 | Delete | `file.delete` supports recursive directory deletion; UI: Popconfirm for files, **directories require typing the directory name to confirm** (GitHub mode); always audited | rm -rf must have friction |
| D7 | Symlinks | List entries marked `isSymlink`+`target`; read/write/delete refuse to act directly on symlink paths; no following | Consistent with backup's "no following" principle; symlink traversal is the most classic attack surface |
| D8 | capability | New `file` capability, **registered on all platforms** (Go standard library, works on openwrt/busybox) | File operations have no platform differences; backup is Linux-only, file is not |
| D9 | Audit | Write/delete/rename/mkdir/upload are audited (ResourceFile); browse/read/download are not | Reads are high-volume with no audit value; changes must be traceable |
| D10 | Search semantics | `file.search {dir, query, caseSensitive?, maxResults?}`; query is plain-text contains (not regex), case-insensitive by default | Same discipline as logs grep (ReDoS-proof, consistent and testable behavior on both ends); ops config searches mix cases often, so insensitive-by-default is more practical |
| D11 | Scan bounds | Recursion depth ≤8; skip symlinks (no following, extends D7) and non-regular files; skip binaries (NUL in first 512B); skip files >1MB (same cap as the editor read); total files ≤5000; stop at 200 hits; total timeout 15s returning the scanned portion; exclude `.git`/`node_modules` directories, search other hidden directories as usual (`.ssh`/`.config` may be exactly the target) | Large directory trees/large logs must not drag down the agent; every limit has a truncated/skipped count for explainability |
| D12 | Server endpoint | `POST /api/agents/{id}/files/search` pure forwarding, both ends validate the same rules (dir path + query non-empty ≤256), not audited (browsing in nature, extends D9) | Same pattern as the existing file endpoints |
| D13 | Web interaction | Toolbar "Search" button → Modal (current directory as root + keyword + case toggle) → result list (relative path + line number + hit-line highlight, lines truncated to 200 chars) → click a path to jump to its directory | Search naturally starts from the current browsing location |
| D14 | Image preview | Pure web implementation reusing the `files/read` chunk endpoint (zero Go changes); extension whitelist jpg/jpeg/png/gif/webp/bmp/svg/ico/avif; >20MB refuses preview and directs to download; always rendered via `<img>` (including SVG) | Preview is browsing in nature; going through the existing read endpoint adds no new attack surface; SVG in an `<img>` context is in secure static mode where scripts don't execute — natural isolation; MIME is mapped by extension, not content sniffing; images beyond the 20MB cap are rare, and downloading to view locally is a better experience |

## Agent-Side Design

### Capability and Registration

- `agent.go` detectCapabilities unconditionally appends the `file` capability (metadata carries the
  M1-supported hint values maxReadChunk=1MB / maxWriteChunk=1MB; the 10MB upload cap is enforced
  on the web side);
- `providers.go` unconditionally calls `RegisterProvider(rpc.NewFileProvider(rpc.FileConfig{}))`
  (no platform check, in contrast to backup's GOOS==linux).

### RPC Methods

| Method | Params | Returns | Sync/Async |
|------|------|------|-----------|
| `file.list` | `{dir}` | `{entries: [{name, size, mode, mtime, isDir, isSymlink, target}]}` | Sync |
| `file.read` | `{path, offset, length}` | `{data(base64), size, eof}` | Sync chunked |
| `file.write` | `{path, data(base64), truncate, mode?}` (mode optional, whitelist 0600/0644, default 0644) | `{size}` (total file size after write) | Sync |
| `file.mkdir` | `{path}` | `{}` | Sync |
| `file.delete` | `{path}` | `{}` | Sync |
| `file.rename` | `{path, name}` | `{}` (rename within the same directory) | Sync |

- `file.list` returns only the directory's direct entries (no recursion), sorted directories-first
  + by name; `mode` is shown in octal (e.g. 0644); symlink entries are marked via Lstat, with
  `target` being the link destination (read-only display);
- `file.read` single-chunk cap `fileReadChunkLimit = 1MB`; offset≥size returns empty data with
  eof=true (fully isomorphic to backup.read); only **regular files** can be read
  (Mode().IsRegular() after Lstat);
- `file.write`: O_CREATE|O_TRUNC when truncate=true; O_APPEND when truncate=false (and the file
  must already exist); missing parent directories are auto-created via MkdirAll (0700); returns
  the total file size after the write;
- `file.mkdir`: errors if it already exists (to avoid mistaking an existing directory for a new
  one); MkdirAll supports creating multiple levels at once;
- `file.delete`: files are deleted directly; directories are deleted recursively (RemoveAll);
  a symlink deletes only the link itself (Remove doesn't follow); a path that Cleans to `/`
  is rejected;
- `file.rename`: `name` allows only a filename (`^[^/\x00]{1,255}$`, no path separators),
  an existing target is rejected (no silent overwrite).

### Path Safety (defense on both ends, same rules on the server)

- All paths: after `filepath.Clean` they must satisfy `filepath.IsAbs`; relative paths and `..`
  forms are rejected (Clean already resolves `a/../b`; a residual leading `..` means it's not absolute);
- write/mkdir/delete paths must not be `/` (the root directory itself is neither writable nor deletable);
- rename's `name` contains no separator, so the joined target path stays inside the original
  parent directory (structural guarantee);
- read/write targets that are symlinks are rejected outright (D7), eliminating writes punched
  through links to arbitrary locations.

## Server-Side Design

### REST API (server/api_files.go, registerFilesAPI, JWT protected)

| Method | Path | Notes |
|------|------|------|
| POST | `/api/agents/{id}/files/list` | `{dir}` → forwards to file.list |
| POST | `/api/agents/{id}/files/read` | `{path, offset, length}` → forwards to file.read (editor-only) |
| POST | `/api/agents/{id}/files/write` | `{path, data, truncate}` → file.write + audit file_write |
| POST | `/api/agents/{id}/files/mkdir` | `{path}` + audit file_mkdir |
| POST | `/api/agents/{id}/files/delete` | `{path}` + audit file_delete |
| POST | `/api/agents/{id}/files/rename` | `{path, name}` + audit file_rename |
| GET  | `/api/agents/{id}/files/download?path=` | Loops file.read (256KB chunks) for stream forwarding, attachment, stops on disconnect, 15-minute total timeout |

- All path parameters go through the same Clean/IsAbs/root checks on the server as on the agent
  (defense on both ends);
- Download reuses the backup download's stream-forwarding skeleton (Content-Length passed through
  from the first chunk's size, nosniff,
  mid-stream failures exposed as a Content-Length mismatch);
- Uploads are not staged on the server: the browser base64s and POSTs write; the server forwards
  as-is (passes through memory and is discarded);
- Audit: `ResourceFile = "file"`, actions file_write / file_mkdir / file_delete /
  file_rename; detail records the path (a path is operational information and may be recorded,
  **file contents never enter the audit log**).

## Web UI Design

- Workbench's `WorkbenchTab` gains `'files'`, a "Files" tab (FolderOutlined),
  alongside SSH/RDP/VNC — file management is naturally agent-scoped and reuses the left
  Agent tree;
- New component `web/src/components/FileBrowser/index.tsx`:
  - **Toolbar**: breadcrumb path (clickable level-by-level back + manual path entry to jump), refresh, new directory, upload;
  - **List** Table: name (directory/file icon + symlink marker) / size / permissions / modified time /
    actions (edit [regular files ≤1MB only], download, rename, delete);
  - **Edit Modal**: read the whole file → TextArea (monospace) → save (truncate write);
  - **Upload**: antd Upload in manual mode, beforeUpload reads the File → base64 →
    write (truncate=true); >10MB rejected on the frontend;
  - **Download**: GET download → axios blob → createObjectURL (same as backup);
  - **Delete**: Popconfirm for files; for directories, a Modal asking to type the directory name to confirm (D6).

## M2: Text Search (2026-09-18)

Recursive in-directory text search (D10-D13), locating "which file a config entry lives in" for troubleshooting scenarios.

### Agent Side

- RPC `file.search`: `{dir, query, caseSensitive, maxResults}` →
  `{matches: [{path, line, text}], truncated, scanned, skipped}`;
  - Recursion via `filepath.WalkDir`; relative paths are presented relative to `dir`; line numbers start at 1;
  - Matching: `strings.Contains` (ToLower on both sides when insensitive); the hit line's `text` is truncated to 200 chars;
  - Stops once `matches` reaches the cap (D11) with `truncated=true`; depth beyond 8 / more than
    5000 files / 15s total timeout likewise set truncated and wrap up early; skipped counts
    large files and binaries;
- Path validation same as list (Clean/IsAbs/not root); an empty `query` or >256 chars is rejected.

### Server Side

- `POST /api/agents/{id}/files/search` (dispatched in api_files.go): validates the same rules on both ends,
  then forwards `file.search`; agent errors pass through as 502; not audited.

### Web Side

- FileBrowser toolbar gains "Search": a Modal with keyword input (current directory as root) + a case toggle;
- Result list: hit path (click to jump to its directory and close the Modal) + line number + hit line (keyword highlighted);
- Bottom status line: `N hits · M files scanned`, a hint Tag when truncated/skipped>0.

**Tests**: agent temp directory tree (hits and case toggle / depth cap / binary and large-file skipping /
truncation at 200 / path validation rejections / empty result is not an error); server forwarding and 400
validation; web build.

## M2: Image Preview (2026-09-18)

Preview images directly in the file list (D14), for screenshot/icon/certificate-QR-code viewing without downloading.

### Web Side (pure frontend, zero Go changes)

- **Trigger**: a "Preview" button in the actions column (EyeOutlined), shown only for files whose
  extension hits the whitelist
  (jpg/jpeg/png/gif/webp/bmp/svg/ico/avif, case-insensitive) and that are neither directories nor
  symlinks;
- **Fetch**: `readFileChunk` loops in 1MB chunks to assemble; offset advances by **bytes already read**
  (the returned `size` is the file's total size and cannot be used as a cursor); if the first chunk's
  `size` is >20MB, refuse immediately (hint to download instead); closing the Modal sets a
  cancelled flag, the loop aborts at once and no further requests are sent;
- **Render**: map MIME by extension (image/jpeg etc., complete within the whitelist) → `Blob` →
  `createObjectURL` → `<img>`; both closing and reopening revoke the old URL;
- **Security**: SVG always goes through `<img>` (browser secure static mode, scripts don't execute),
  never innerHTML; MIME doesn't trust agent content sniffing (the read endpoint doesn't return
  content-type anyway);
- **Modal**: title is the filename; Spin while loading; failure Alert as fallback + guidance to download;
  image `max-width:100% / max-height:60vh` with a dark background and translucent texture (so
  transparent PNGs are visible);
  the footer shows the natural resolution (naturalWidth×Height read on onLoad) and file size.

**Tests**: pure web change — `pnpm build` (zero tsc errors) + zero regression in existing Go tests.

## M3: Chunked Upload / Resume (2026-09-18)

Uploads upgrade from "≤10MB single write" to "any ≤2GB chunked streaming": the append channel of
`file.write` (reserved in M1 D5) is officially enabled, with zero agent changes.

| # | Decision | Content | Rationale / Notes |
|---|------|------|------------|
| D15 | Chunk protocol | The web loops the existing `files/write` (zero changes to the server endpoint; the 1MB single-chunk precheck stays as-is): 1MB per chunk (the agent's full `fileWriteChunkLimit`); the first chunk uses `truncate=true` to create the file/clear old content, subsequent chunks `truncate=false` (O_APPEND — the agent already requires "the append target must exist", guarding against fragments); after each chunk, verify the returned `size === bytes sent` (the agent returns the total size after write); on mismatch, abort (preventing interleaving/misalignment from producing a corrupt file) | The first-chunk-creates-file protocol is the agent's existing design (the append target must exist); the size check is sequential writing proving itself |
| D16 | Resume (within a session) | Failed chunk writes auto-retry ≤3 times: before retrying, `files/list` probes the target file's current size — equal to bytes sent → resume appending from the breakpoint; 0/nonexistent → start over (first chunk rebuilds); any other value (concurrent write) → abort with an error. No resume across browser refreshes (v1; uploads are mostly one-shot actions) | The server-side file size is the resume cursor, so no server session state is needed; concurrent-write detection protects users from overwriting others' changes |
| D17 | Size-based routing | ≤10MB keeps the single-chunk fast path (one write, unchanged experience); >10MB goes chunked with a 2GB cap (larger is directed to scp/terminal — the sensible boundary for base64 over WebSocket RPC) | Zero overhead for small files, unlocks large ones; the cap mirrors download's "no limit" (pulling a stream is cheap, pushing is expensive) |
| D18 | Audit routing | The server write audit records only `truncate=true` (new write sessions: one row each for edit-save/chunked first chunk/overwrite upload); `truncate=false` (chunked continuation) is not recorded — otherwise a 500MB upload would spam 500 file_write rows, and continuation chunks have no standalone meaning | Session-level audit is represented by the first chunk; the append channel is currently used only by chunked upload; terminal-equivalent root power is already outside the audit surface (D1 semantics) |
| D19 | Web interaction | The toolbar upload entry stays; while a >10MB file uploads, the toolbar shows progress (`Progress` + sent/total) and "Cancel"; cancel = abort remaining chunks + delete the partial file (`files/delete`, silent on failure) + notify; on completion, refresh the list | Progress visible in place; no partial-file garbage left; re-uploading after cancel starts with a truncate-clearing first chunk, semantically self-consistent |

### Server Side

- `forwardFileRPC` write branch: the audit condition gains `truncate` (only first chunks/overwrites record
  `file_write`), the rest of the logic is untouched.

### Web Side

- `UPLOAD_MAX_BYTES` semantics change to the single-chunk path threshold; a new 2GB chunked cap;
- `uploadFiles` chunk loop (`File.slice` → base64 → write); progress/cancel via
  an incrementing token (same as image preview) + partial-file cleanup; resume on failure by
  probing (D16).

**Tests**: server — truncate=true audited, truncate=false not; web — tsc +
build; agent zero changes, zero regression.

## M4: chmod / chown Permission Editing (2026-09-19)

The "chmod/chown later" item scoped out of M1 in D2 is now formally filled in. Pure permission bits
and ownership changes, without touching content — reusing the existing channels (isomorphic chunked RPC,
path validation, audit patterns).

| # | Decision | Content | Rationale / Notes |
|---|------|------|------------|
| D20 | agent RPC | `file.chmod {path, mode}` / `file.chown {path, uid, gid}`: path goes through the existing `cleanAbsPath`; **symlinks rejected** (extends D7 — chmod/chown on a link would act on the target, carving an exception into the "no following" discipline); `mode` numeric 0–0o777 (perm bits only, no setuid/sticky); `uid`/`gid` numeric 0–4294967295, **no username resolution** (username namespaces are unreliable across machines — `nobody` on two machines is not the same person); no platform gating — on Windows Chmod affects only the read-only bit and Chown errors, honestly surfaced | Permission changes are low-frequency with a small parameter surface; passing error messages back to the UI verbatim (EPERM etc.) is exactly what ops needs |
| D21 | list gains ownership | list entries gain `uid`/`gid` (int); taken from `syscall.Stat_t` (unix-specific) → files split by GOOS: `file_stat_unix.go` / `file_stat_windows.go` (always -1, UI shows "—") | You must see the current state before changing it; CI cross-compiles darwin/windows, so the type-assertion target must exist |
| D22 | server + web | `POST /api/agents/{id}/files/chmod {path, mode}` and `/files/chown {path, uid, gid}`: same-rule validation on both ends (absolute path + numeric ranges); **audited** as `file_chmod`/`file_chown` (detail records mode/uid/gid) — chown changes ownership and directly affects access control, so it must be traceable; the web row actions gain "Permissions" → Modal: current mode/uid/gid shown read-only + an rwx nine-cell grid (owner/group/other) + numeric uid/gid inputs; on apply, route by what changed — mode changed calls chmod, uid/gid changed calls chown, nothing changed reports no change | One Modal captures both actions; the nine-cell grid is more intuitive and error-proof than raw octal entry, while also showing the composed octal for verification |

### Agent Side

- `file_provider.go` gains `Chmod`/`Chown` (lstat → symlink rejection → numeric validation →
  `os.Chmod`/`os.Chown`) + Call dispatch; `file_stat_*.go` provides
  `fileStatOwner(path) (uid, gid int)` (Lstat semantics, no following).

### Server Side

- `api_files.go`: the action whitelist gains chmod/chown; `fileRPRequest` gains
  `Mode/UID/GID *float64` (JSON number); validate, then forward + audit.

### Web Side

- `api.chmodFile/chownFile`; `FileEntry` gains `uid?/gid?`; FileBrowser row actions
  gain a "Permissions" button (disabled for symlinks) → PermissionModal (nine-cell grid + ownership inputs).

**Tests**: agent — chmod roundtrip (0600→0644 read back), symlink rejection, out-of-range
mode/uid/gid rejection, missing-path errors, list includes uid/gid; server — forwarded params,
both-end validation (path/mode range/uid range), audit persisted, agent failure not audited;
web — tsc + build.

## M5: Resume Across Refreshes (2026-09-19)

The "follow-up item" reserved in M3 D16. Key observation: the resume cursor (the target file's
actual size) already has a probing mechanism agent-side (same source as D16's failure resume);
what a refresh really loses is only **the memory of the upload coordinates**
— the association between the local File object and "how far it got". This is precisely
localStorage's sweet spot (single-browser, single-user semantics), **no server-side upload
session state needed** — overturning the original assumption in the "not doing" item's
parentheses; once the mechanism was simplified, the change surface collapsed to pure web.

| # | Decision | Content | Rationale / Notes |
|---|------|------|------------|
| D23 | Pending registry | After each successful chunk, upsert `{agentId, dir, name, size, lastModified, uploaded, ts}` into localStorage `cockpit-pending-uploads` (array, capacity 10 FIFO, entries older than 7 days purged on load); removed on upload success/cancel; **final failures with retries exhausted are kept** — network drop plus closing the page is exactly the scenario resume is for; the ≤10MB single-chunk path doesn't register (cheap to re-upload) | localStorage is established usage for per-viewer convenience storage (SettingsContext precedent); all reads/writes are wrapped in try/catch, so in private windows/with storage disabled the feature is silently absent with zero impact on the main upload flow |
| D24 | Resume interaction | On mount, FileBrowser reads pending entries by agentId and shows them in an Alert above the list (filename/directory/progress/continue/abandon; hidden while an upload is in progress). "Continue" → the user re-selects the local file (File handles can't be restored by the browser; manual re-selection is mandatory) → validation matrix: size mismatch errors and keeps the record; mtime mismatch asks for Modal confirmation (mixed-content risk); then probe the remote — null/0 start from scratch; 1MB whole-chunk aligned and < size resumes from that offset; === size counts as fully uploaded (clear record and refresh); misaligned or > size asks for Modal confirmation to overwrite and re-upload | The validation matrix delegates step by step: what machines can judge is judged automatically (size/alignment), semantically ambiguous calls go to the user (mtime/overwrite); the alignment check reuses the chunk protocol invariant (always 1MB except the last chunk) |
| D25 | Boundaries | Pure web, zero Go changes; resume reuses uploadChunked (parameterized start offset), with size-mismatch and concurrent-write protection intact; "Abandon" only clears the local record and **does not delete the remote partial file** (remote state may have changed after refresh; auto-deletion has friendly-fire risk, so a Tooltip guides manual cleanup or overwrite by re-upload) | The D16 philosophy — the remote file size is the cursor — is unchanged; zero new server state, zero new endpoints, zero audit changes |

### Web Side

- `readPendingUploads` / `persistPendingUpload` / `removePendingUpload`
  (localStorage try/catch + TTL + FIFO);
- `uploadChunked(raw, path, token, resume?)` gains an optional start offset; each successful chunk
  records progress;
- Pending-uploads Alert below the toolbar (each row an Upload to re-select the file + an
  Abandon Popconfirm), `confirmAsync` wraps Modal.confirm into a Promise.

**Tests**: web — zero new tsc errors (9 pre-existing) + build; server/agent zero changes, zero regression.

## Not Doing (follow-up items)

- Directory size stats / recycle bin;
- Recursive chmod/chown (batch permission changes across a directory tree — large blast radius,
  wait for a real need);
- Binary hex preview: the editor/image preview first cover the main text and image scenarios.

## M1 Checklist

- [x] agent: `internal/agent/rpc/file_provider.go` (list/read/write/mkdir/delete/rename
      + path safety + symlink rejection) + unconditional registration + file capability (all platforms)
- [x] server: `api_files.go` (7 endpoints + both-end path validation + audit) + serveAPI `/agents/`
      branch wiring + audit constants (ResourceFile + file_write/mkdir/delete/rename)
- [x] web: Workbench "Files" tab + FileBrowser (breadcrumb/manual path jump/list/edit ≤1MB/
      upload ≤10MB/download unlimited/rename/delete — directories confirmed by typing the name)
- [x] tests: provider unit tests (list sorting and symlink marking / read chunk reassembly /
      write overwrite and append / appending to a missing file rejected (the test caught an
      implementation deviation) / traversal/symlink/root rejection / rename validation /
      symlink deletion removes only the link itself), server API forwarding and validation,
      byte-for-byte comparison of download stream forwarding, agent offline 503
- [x] docs wrap-up + todo.md sync

## References

- Internal: [backup-design.md](/guide/backup-design) M1.5 (source of the file.read chunking and download
  stream-forwarding skeleton), [stack-deploy-design.md](/guide/stack-deploy-design) (provider
  registration pattern), [probe-enhance-design.md](/guide/probe-enhance-design) (audit and notification
  patterns), `todo.md` P1 file manager item
- External: [Filebrowser](https://github.com/filebrowser/filebrowser) (browsing interaction paradigm),
  OWASP Path Traversal (both-end validation and symlink policy)
