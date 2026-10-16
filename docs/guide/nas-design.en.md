# NAS System Integration Design

> Language: [中文](./nas-design.md) ｜ **English**

> Status: M1 + M2 fully implemented. A unified snapshot model + multi-provider architecture: M1 covers generic Linux
> NAS (agent local command observation, zero credentials, zero configuration) through the whole chain; M2 covers Synology DSM / TrueNAS /
> OpenMediaVault network API providers (agent intranet access, same as the OpenWrt/PVE ones), **all implemented**.

## D1 Goals and the unified model

**Goal**: unify the storage pools, mount capacity and share exports of common NASes into an observation
surface visible on one screen + anomaly alerting. Observation only (M1); no NAS configuration write path.

Unified snapshot model (every provider maps onto the same structure, so the frontend and alerting never see backend differences):

```go
type NasSnapshot struct {
    Available bool       // whether this provider is usable (false when tools are missing/the API is unreachable, no error)
    Source    string     // linux | dsm | truenas | omv
    Pools     []NasPool  // storage pools / RAID / volume groups
    Mounts    []NasMount // local filesystem mount capacity
    Shares    []NasShare // SMB / NFS share exports
}

type NasPool struct {
    Name     string   // md0, tank, vg0, volume_1 ...
    Kind     string   // mdadm | zfs | lvm | btrfs | dsm (vendor pools use the source identifier)
    State    string   // healthy | degraded | resync | failed | unknown
    TotalGB  float64
    UsedGB   float64  // available for ZFS/btrfs; left at 0 when unobtainable
    Devices  []string
    Detail   string   // raw status lines (resync progress, etc.)
    Host     string   // source device: empty for local observation, the target name for network NAS (M2)
}

type NasMount struct {
    Device    string
    MountPath string
    FsType    string
    TotalGB   float64
    UsedGB    float64
    Host      string  // same as above
}

type NasShare struct {
    Protocol string // smb | nfs
    Name     string
    Path     string
    Comment  string
    Hosts    string // NFS export scope (e.g. 192.168.1.0/24)
    Host     string // same as above
}
```

Disk health (SMART) is not repeated here — the Disk page already covers it; the NAS page focuses on pools/capacity/shares.

## D2 Provider architecture and milestones

`NasProvider` implements one of three (decided by capability/config, all exposed over RPC on the agent side):

| Provider | Route | Data source | Milestone |
|---|---|---|---|
| linux | agent local commands | /proc/mdstat, zpool, vgs, btrfs, df, testparm, exportfs | **M1** done |
| dsm | agent→DSM HTTP API | the SYNO.API family, Session login | **M2** done |
| truenas | agent→TrueNAS REST | /api/v2.0 pool/dataset/sharing, Basic Auth | **M2** done |
| omv | agent→OMV JSON-RPC | Login + Rpc | **M2** done |

One agent can observe multiple network NASes (a targets array); the merged snapshot is returned with the source
device marked in each record's `Host` field (empty for local observation; the frontend host column shows `agent · device`),
`Source` participating in the origin as a comma join (`linux,dsm`); a single target failing only degrades with a
log and does not drag down the snapshot (same discipline as a single data source failing).

Shared discipline for M2's network API form (same as the OpenWrt/PVE providers):

- The agent initiates HTTP proactively on the intranet (the agent already dials out to the server, so lateral intranet access is legitimate)
- The first source of credentials is the server vault entry `nas/<name>` (AES-256-GCM encrypted at rest, delivered via the registration response plus a supplementary push through the `nas.set_targets` RPC, see `docs/guide/vault-design.md` §5.3/5.4);
  `COCKPIT_NAS_TARGETS` (a JSON array) is kept as a fallback and for bootstrap:
  **credentials never enter config.yaml and never appear in the snapshot/logs/audit/error messages**:

  ```json
  [
    {"name":"home-dsm","type":"dsm","addr":"https://192.168.1.10:5001",
     "username":"cockpit","password":"***","insecureTls":true}
  ]
  ```

  - Entries whose `type` is not an implemented type (currently `dsm` / `truenas` / `omv` supported) are ignored outright
  - Entries that fail to parse or lack fields (name/addr/username/password) are dropped with a log, not fatal
- Self-signed certificates default to InsecureTLS (the norm for home NASes), and a target can turn it off (`insecureTls: false`)
- The snapshot maps onto the same NasSnapshot, with Source marking the origin

## D3 M1: linux provider data sources (each source degrades independently, one failure never drags down the snapshot)

| Category | Command | Parsing notes |
|---|---|---|
| mdadm RAID | read the `/proc/mdstat` file | `md0 : active raid1 sda1[0] sdb1[2](F)` → the (F) mark flags a failed disk; a resync/recovery progress line → State=resync; a missing U in the `[U_]` bitmap → degraded |
| ZFS pools | `zpool list -H -p -o name,size,alloc,health` | ONLINE in the health column→healthy, other words lowercased as-is; skip when zpool is absent |
| LVM volume groups | `vgs --noheadings --units b --nosuffix -o vg_name,vg_size,vg_free` | VG capacity and free space; skip when vgs is absent |
| btrfs | — (no pooling) | single-disk btrfs capacity is visible via the df mount table, M1 does not parse pooling |
| Mount capacity | `df -k -P` (common to GNU/busybox) | source starts with `/` and is not a `//` prefix (excluding pseudo and remote FS like tmpfs/overlay/network mounts) and ≥1GB; dedupe by (source, capacity) to suppress bind-mount noise |
| SMB shares | `testparm -s` (config comes out on stderr) | parse path/comment from non-global `[share]` sections; skip when samba is absent |
| NFS exports | `exportfs -v` | line format `<path> <host>(opts)`; on empty output fall back to `/etc/exports` with the same rules; skip when nfs-utils is absent |

- Pool State judgement: mdadm → an (F) disk or a missing U in the `[U_]` bitmap = degraded, a resync/recovery/check progress line = resync, inactive/failed = failed, otherwise healthy; ZFS → ONLINE=healthy, other words lowercased as-is; LVM has no state concept=unknown (no alerting)
- `Available` = any sub-source produced output or any tool exists; when all are absent (pure container/minimal system) available=false and inspection plus the frontend skip it without error (same as smart available=false)
- Command timeouts: 5s per command, 30s ceiling for a whole RPC; argv is invoked directly without a shell, a **read-only command whitelist**, so there is no argument-injection surface

## D3b M2: DSM provider (Synology)

The webapi entry shared by DSM 6/7 (`{addr}/webapi/entry.cgi`, all GET + query parameters,
`url.Values`-encoded — safe for passwords containing `&=` and other special characters), with a 5s
per-request timeout that respects the overall 30s ctx:

| Step | API | Notes |
|---|---|---|
| Login | `SYNO.API.Auth` v6 `login` | `account/passwd/session=StorageManager/format=sid` → `data.sid`; create a dedicated account for cockpit and do not enable 2FA (OTP accounts fail login outright) |
| Pools + volumes | `SYNO.Storage.CGI.Storage` v2 `load_info` | `pools[]` (id/status/devices/total_size/used_size) + `volumes[]` (volume/status/fs_type/total_size/used_size/pool_id) |
| Shares | `SYNO.FileStation.List` v2 `list_share` | `shares[]` (name/path); DSM's share-folder protocol granularity is unobtainable (SMB/NFS permissions live in per-share internal APIs), uniformly marked `smb` |
| Logout | `SYNO.API.Auth` v6 `logout` | best-effort defer, failure does not affect the snapshot |

Subsequent requests uniformly carry `_sid`. Field mapping:

- pools: status `Normal→healthy`, `Degrade→degraded`, `Crash(ing)→failed`,
  `Resync(ing)/Migrat(ing)→resync`, otherwise→unknown; `Kind="dsm"`;
  devices as-is, bytes→GB
- volumes → Mounts (a DSM volume is essentially a mount point, so capacity alerting semantics are reused directly):
  Device=pool_id, MountPath=/volume1, FsType=fs_type, bytes→GB
- shares → Shares: Protocol=`smb`, Hosts left empty (NFS rules live in internal APIs, not fetched)

Error discipline:

- Login failure / non-200 HTTP / `success:false` → degrade that target (one log line, containing the target name and the DSM error code, **never the account or password**); DSM code 407 = the IP is banned after repeated failures, so log a special note (no automatic backoff — the inspection interval of ≥5min is sparse enough)
- The target addr must carry a protocol (`https://` or `http://`); entries missing the protocol are dropped
- The snapshot mapping marks `Host=target.Name` on every record; credentials never enter the snapshot

Capability semantics unchanged: `DetectNas()` still only looks at local tools (targets are runtime config,
and an agent with targets configured naturally has observation value, but capability governs "does this agent have a NAS to look at",
so detection stays reproducible — an agent with neither local tools nor targets does not register).

## D3c M2: TrueNAS provider

TrueNAS CORE/SCALE share REST v2.0 (`{addr}/api/v2.0/...`, all GET +
JSON array responses), with **Basic Auth** (username/password, no session state — simpler than DSM's
sid flow; a dedicated account is recommended). The TLS/insecure/timeout/degradation discipline matches DSM.

| Endpoint | Consumed fields | Notes |
|---|---|---|
| `GET /pool` | name/status/size/allocated/topology.data[].disk/scan | pool status uses ZFS's own words (ONLINE/DEGRADED/FAULTED/OFFLINE/UNAVAIL/REMOVED), scan.state=SCANNING (resilver/scrub in progress) → resync |
| `GET /dataset?limit=0` | filter `type==FILESYSTEM && mounted && name does not contain /` (top level) | the top level is one mount per pool (`/mnt/tank`), and child dataset quotas are fine-grained management that observation skips (noise guard); `used + available` = total |
| `GET /sharing/smb` | name/path/comment | Protocol=smb |
| `GET /sharing/nfs` | paths[]/networks[]/hosts[]/comment | expand one share per path; Hosts = networks+hosts joined by comma |

- Status mapping: ONLINE→healthy (scan SCANNING→resync), DEGRADED→degraded,
  FAULTED/OFFLINE/UNAVAIL/REMOVED→failed, otherwise→unknown; `Kind="truenas"`
- Capacity fields like `used`/`available` are **numeric strings** in some versions (e.g. `"123456"`), so parsing uses flexInt to accept both number and string
- Auth failure (HTTP 401) → degrade that target, the error message carries no credentials
- The credential/Host/degradation discipline is identical to DSM (D2/D3b); the consuming side (inspection alerting, frontend)
  needs zero changes

## D3d M2: OMV provider (OpenMediaVault)

OMV uses one JSON-RPC across all versions (`POST {addr}/rpc.php`, body
`{"service","method","params"}`, the response wrapped as `{"response":...,"error":...}`,
with error carrying code/message on failure). **Auth goes through the request header**
`X-Openmediavault-Sessionid: <sid>` (neither Cookie nor body; the source's
session.inc `HTTP_X_OPENMEDIAVAULT_SESSIONID`): `session.login`
params `{username,password}` (an object) → `response.sessionid`, with failure being HTTP 400
and a fixed message; `session.logout` is best-effort. The TLS/insecure/timeout/degradation discipline is the same as before.

| Endpoint (service.method) | Consumed fields | Notes |
|---|---|---|
| `filesystemmgmt.enumerateFilesystems` | devicefile/type/mounted/mountpoint/size/used/available | all filesystem entries, `mounted=true && mountpoint!=""` → Mounts; unmounted entries like swap are naturally filtered out |
| `smb.getShareList` (params `{start:0,limit:-1}`) | data[].sharedfoldername/comment/guest/readonly/hostsallow | only `enable=true`; wrapped in `{total,data}` |
| `nfs.getShareList` (params as above) | data[].sharedfoldername/client/options/comment | full set (the schema has no enable field) |

- **Pools left empty**: OMV has no unified storage pool concept (the underlying mdadm/btrfs are surfaced by the OS layer),
  so if an OMV host runs the agent, local linux sources cover it, and the network API only adds the
  filesystem/share view; the `available` judgement depends on mounts/shares, and `Kind` has no pool entries
- **Capacity is a binary_format string** (`"1.50 GiB"`, `"-1"` when unmounted): the parser function
  `parseBinarySize` converts B/KiB/MiB/GiB/TiB/PiB units into GB (decimal, consistent with
  the df side), with illegal/negative values → 0 (the frontend shows —)
- **Share Path left empty**: the OMV API does not give the absolute share path directly (the sharedfolder location =
  the mntent mount point + reldirpath, which needs two join queries and whose export root differs across versions
  `/export` vs `/sharedfolders`), so nothing is fabricated; Name=sharedfoldername,
  SMB Hosts=hostsallow, NFS Hosts=client
- An account with 2FA enabled gets `challengeRequired` and no sessionid on login, which is treated as a login failure
  and degrades (the hint text carries no sensitive information); observation is recommended with a dedicated 2FA-free account
- The credential/Host/degradation discipline is identical to DSM (D2/D3b); the consuming side needs zero changes

## D4 capability and registration

- `DetectNas()`: a successful LookPath for any of mdadm/zpool/vgs/btrfs/testparm/exportfs,
  **or valid entries parsed out of `COCKPIT_NAS_TARGETS`** (the springboard mode: Windows/macOS hosts without
  local storage tools can still observe network NASes; invalid JSON/empty values do not trigger it)
  (pure container minimal systems do not register, avoiding a redundant capability on every agent)
- capability: `{Type: "nas", Version: "1"}`, appended in detectCapabilities (same as cron)
- providers.go: `case "nas"` registers `NewNasProvider(NasConfig{})`
- Two RPC methods: `nas.status` → NasSnapshot (a full snapshot, same as overlay.status); `nas.set_targets` (vault V6 first wave ②: the server pushes `nas/` entries to the agent)

## D5 server: REST and inspection alerting

REST (following the smart pattern):

```
GET  /api/agents/{id}/nas/status   pure forwarding of nas.status (browsing, no audit, no persistence)
GET  /api/nas/config               inspection settings + thresholds
PUT  /api/nas/config               save (inspection-config classes are not audited, isomorphic with smart/drift)
```

Inspection `nasScanLoop` (the same skeleton as smart/ddns/acme):

- Setting `nas.scan_interval_seconds`: min 300 / max 86400 / default 1800, 0 = off
- Each round pulls snapshots from every online agent carrying the nas capability one by one, only judging alerts, never persisting
- Alert rules (`CheckNasScan`, genuinely deduplicated — reported once per resource+title while unread):
  - pool State ∈ {degraded, resync} → warning; failed → error (the title distinguishes pools by name)
  - mount usage ≥ `nas.usage_warn_percent` (default 80, 50-99) → warning,
    with the mount path in the title; silent after recovery (`createAlertIfNotExists` only adds and never clears, so no repeat alerting; existing alerts still need a human to mark them read)
  - available=false and the unknown state are not alerted (missing observation ≠ failure, same rule as smart D9)
  - when a record's `Host` is non-empty (network NAS) the alert title's "host" slot shows the device name
    (`storage pool failure: volume_1 (host home-dsm)`), and deduplication naturally isolates different devices

## D6 Web /nas page

- A menu item "Storage Pools" (HddOutlined distinguishes it from disk health: disk health looks at disks, storage pools look at volumes),
  placed after "Disk Health"
- A cross-agent overview (the same layout as the Disk page):
  - anomalies on top: degraded/resync/failed pools + mounts over the threshold aggregate into a banner at the top
  - grouped by host in Collapses, three sections per host:
    - pool table: name/Kind Tag/status badge (healthy green, degraded orange, failed red, resync blue)/capacity bar/devices/detail
    - mount table: device/mount point/FsType/capacity bar (red once over the threshold)
    - share table: protocol Tag/name/path/export scope
  - agents filtered by nas capability (the same dropdown as the cron page)
- When no agent has the nas capability, a bootstrap explanation is shown (the detection condition = any storage tool exists)

## D7 Security boundaries

1. Everything on the agent side is read-only: reading /proc files + a whitelisted set of read-only commands, zero write paths
2. argv is invoked directly without a shell; command names are fixed and no argument comes from user input
3. The snapshot contains no credentials/secrets (for SMB only the share name and path are taken, never the account/password sections)
4. The server purely forwards and never persists: the NAS itself is the only source of truth; inspection only produces alerts
5. M2 network-API credentials take the server vault `nas/<name>` as their first source (encrypted at rest), with agent environment variables only as a fallback; same discipline as OpenWrt/PVE

## D8 Test strategy

- **agent** (rpc/nas_provider_test.go): fake Commander + temp-file injection —
  /proc/mdstat samples (three states: active/resync/(F) disk), zpool list/status samples,
  df -kP samples (asserting pseudo-FS filtering), testparm share section parsing, exportfs parsing;
  one source failing does not affect the rest (inject an erroring source); all sources missing → available=false;
  no-injection-surface validation (a fixed command-name-list assertion)
- **agent DSM** (rpc/nas_dsm_test.go): `httptest` serving a **fake DSM**
  (a handler answering entry.cgi JSON samples by api/method) + `t.Setenv` to
  inject `COCKPIT_NAS_TARGETS` — the full login→load_info→list_share→logout flow,
  pool status mapping (Normal/Degrade/Crash/Resync), bytes→GB, volumes→Mounts,
  shares→smb, Host=target name in snapshot records, **no password in the snapshot or error messages**;
  a login failure degrades that target without affecting the rest; targets JSON invalid/missing fields/entries
  dropped; entries with an unimplemented type ignored
- **agent OMV** (rpc/nas_omv_test.go): `httptest` fake OMV (dispatching on the body's
  service/method + validating the `X-Openmediavault-Sessionid` header) — the full login→
  enumerateFilesystems→getShareList×2→logout flow, binary capacity string
  parsing (GiB/MB/bare number/`-1`), mounted filtering, SMB enable filtering,
  a pools-empty assertion, bad-target degradation, no password in the snapshot
- **server** (api_nas_test.go + nas_scan_test.go): forwarding + config validation
  (interval 0/300/86400 legal, 299/86401 rejected; threshold 50-99); inspection alerting —
  degraded pool alerts warning, failed pool error, mount over threshold alerts, no repeat after recovery,
  available=false does not alert; exactly one nas_config_update audit record
- **web**: `npx tsc --noEmit` (zero errors in new files) + `npm run build`

## D9 Acceptance checklist

- [ ] `go test ./internal/agent/rpc/ ./internal/server/` passes
- [ ] New files are gofmt clean; `go build ./...` passes
- [ ] web tsc + build pass
- [ ] Real-environment acceptance (listed in todo.md): measured pool status and capacity, SMB/NFS share parsing
      and genuinely deduplicated alerting on a real host with mdadm/ZFS/large mounts; real-machine API acceptance for each M2 NAS