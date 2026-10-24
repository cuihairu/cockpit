# Real-Machine Acceptance Checklist


> The automated verification boundary at feature delivery: Go unit tests (including `-race`)
> and web vitest all green, and the local end-to-end scripts
> (`scripts/e2e-smoke.sh`, `scripts/local-acceptance/`) passing.
> The scenarios below depend on **real environments**—real machines, real credentials, real
> network, real versions—which cannot be automated offline, and are scattered per feature in
> the "Remaining" part of each `todo.md` entry. This document aggregates them into one checkable
> list, so that once a test machine or credentials are in hand you advance domain by domain,
> checking one item off and backfilling one place at a time.

## General Prerequisites

- [x] server started, at least one Linux agent registered online (green marker on the `/agents` page) — verified by the e2e smoke test
- [x] `scripts/e2e-smoke.sh` smoke passes (server→agent→inventory sync→`/api/resources` closed loop) (2026-09-28, ran twice locally, all green)
- [x] At least one notification channel configured (webhook/ntfy etc.); when accepting alert-type features, use the "Test Notification" button to check delivery — the webhook channel was accepted locally (2026-10-05, probe `scripts/acceptance/notify/` five-piece set N0-N4 15/15 PASS): config with two webhook channels (the `127.0.0.1:9700` receiver actually received + `9799` as a dead-port failure sample), `POST /api/notification/test` returns per channel — the live channel ok, the dead channel ok=false carrying connection refused (the failure is presented without a 500 and without being masked), and the response contains no channel secret; the receiver received exactly 1 message (`event_type=test`, title "Cockpit Test Notification", `X-Cockpit-Secret` auth passed); the audit entry `notification/test/channels` is exactly one (details sent=1/failed=1). The ntfy/telegram/herald cloud channels cannot be verified locally, remains blocked
- [x] Habitual check: change-type operations leave a trace on the "Audit Log" page — audit path completeness checked via go test ./audit
- [x] Sensitive fields (certificate content, keys) never appear in audit detail — verified by inspecting the audit log content

## Windows Agent Installer (setup.exe)

Design: [windows-agent-design](/guide/windows-agent-design). The artifact `cockpit-agent-setup-nightly.exe` (+`.sha256`) goes into the nightly release; real-machine acceptance runs automatically inside the nightly `Build Agent Installer` job (`windows-latest`), with that job's artifacts and logs as evidence.

- [ ] Silent install → `%Program Files%\Cockpit Agent` lands on disk, Start Menu + desktop shortcut present
- [ ] Service `CockpitAgent`: `Automatic` (start on boot) + `Running`; `service status` output is consistent
- [ ] exe icon: `ExtractAssociatedIcon` extracts an icon (RT_ICON/RT_GROUP_ICON in the PE `.rsrc` readable on a real machine), evidence `walkthrough-icon.png`
- [ ] Desktop screenshot archived (installed-state UI + visible icon), evidence `walkthrough-desktop.png`
- [ ] Silent uninstall → exe/service/Start Menu/desktop shortcut/`%ProgramData%\CockpitAgent` all removed
- [ ] Interactive install walkthrough: task checkboxes (service/desktop shortcut) + Server address page validation (empty input blocked)

## Application Deployment (Stacks)

Design: [stack-deploy-design](/guide/stack-deploy-design). Prerequisite: a real Docker host (agent online, docker available).

- [x] Create a stack by pasting/uploading compose.yml → deploy: images really pulled, containers reach healthy state, the status overview refreshes (2026-09-28, docker VM really pulled nginx:alpine, healthcheck healthy, aggregate running=2/total=2)
- [x] Full compose semantics: environment variables, volumes, custom networks, depends_on start order (2026-09-28, .env injection proven via printenv; named volume db written → app read + depends_on ordering; custom bridge network + inter-service DNS)
- [x] restart / pull actions really take effect; the deployment history timeline corresponds to the deployment log (2026-09-28, pull/restart tasks success, history finishedAt backfilled including an honest failed)
- [x] One-click creation from the template library and the import path (2026-09-28, the template/import paths are UI fill paths; once persisted they are the regular create+up — verified on the real machine with an equivalent raw compose, the UI chain is covered by vitest)
- [x] How the list page presents the directory self-check warning when the stacks directory does not exist / is not writable (2026-10-07 close-out, scripts/acceptance/stacks/probe-dir.sh K1-K4 all PASS: the local cui agent is itself in a non-root form, `COCKPIT_STACKS_DIR` points at a root:root 0700 directory — K1 a single agent's stack.list fails → 502 error body carrying info.dirError="write probe: … permission denied"+dirWritable=false; K2 the aggregate GET /api/stacks carries the same agentInfo (the web directory warning's actual consumption surface); K3/K4 by contrast a non-existent directory → the agent auto-creates it 0700, dirWritable=true+composeVersion. dirError → the list page warning banner rendering was already landed on 2026-09-29 (Stacks/index.tsx renders info.dirError + a server-side unit test shipping dirError down together with info), this probe fills in the E2E scenario with a real non-root agent. Already proven on 2026-09-28: a non-existent directory is auto-created, an illegal path reports the error through)

## Backup and Restore (agent side)

Design: [backup-design](/guide/backup-design). Prerequisite: the local server+agent alone is enough for acceptance (packaging/restore is an agent-side implementation, it does not depend on a Docker host); offsite items need rclone and a real remote.

- [x] The scheduled trigger fires once on time (either daily@HH:mm / every:Nh form); after downtime the catch-up runs exactly once — accepted locally (2026-10-05, probe `scripts/acceptance/agent-backup/` seven-piece set K0-K11 44/44 PASS, server `:19998` + agent abk-acc-a1): `daily@` created with due=now+2min, about +5s after the due time it auto-executes exactly once, `next_run_at` rolls forward 24h±600s; downtime catch-up — `restart-server.sh 110` stops the server for 110s across the due time, after recovery the first tick catches up exactly once, then 75s of observation shows no replay
- [x] Packaged artifact: tar.gz complete, path traversal protection (construct a directory holding a `../` file to verify rejection), symlinks not followed — same probe: the sha256 of the downloaded run artifact matches the one on disk, the archive is diffed entry by entry (fixture tree of 8 entries including a nested directory); delete/download requests carrying a `../` traversal name are both rejected with 400; symlink entries are recorded as the link itself and not followed (`link-to-readme`→`readme.txt`, absolute links pointing at `/etc/hostname` are both kept as links)
- [x] retention automatically cleans up expired backups; run history and the `backup.failed` notification are delivered — same probe: with retention=2, after running three times only the latest two remain (run1.File disappears from disk); `backup.failed` is actually received via the webhook receiver `:9701` (the event whitelist matches on the `EventConfig.Type` field, yaml must write `type: backup.failed`)
- [x] Restore in two phases: unpacking into a separate directory never overwrites the original path; a constructed Zip Slip sample is rejected; the task log is readable — same probe: restoring into an empty directory succeeds with zero sha change across the 8 entries of the source tree; a non-empty target directory is refused by the agent, 502 passes through the original error containing `not empty`; the constructed Zip Slip samples (`../zipslip-escape-1`, `sub/../../zipslip-escape-2`) are skipped and never land on disk, `ok.txt` unpacks normally; restore requires confirm, a relative dest request gets 400
- [x] Backup file download (chunked RPC stream forwarding, does not blow up memory at GB scale) — same probe: a 256MB sample forwarded as a 1025-chunk stream (256KB/chunk), Content-Length equals the real on-disk length, sha256 matches; the server's `/proc` VmHWM goes 30.6→47.2MB (cap 300MB), proving there is no whole-archive buffering (the GB-scale row is backed by the "GB-scale complete chain" below)
- [x] Offsite retention (the mechanism, half done): a real rclone push succeeds; on push failure `backup.remote-failed` is a separate notification and does not change the run terminal state; "re-push" succeeds — accepted in a real container (2026-10-07, probe `scripts/acceptance/agent-backup/probe-remote.sh` R0-R3 4/4 PASS, a debian:12+rclone container builds a local backend with remote `bklocal:/backups` — the real rclone exec chain is entirely real (argv/regex/RemoteStatus tracking/capability probing), zero cloud credentials): R1 push succeeds with remoteStatus=ok + the remote directory really receives the archive; R2 a ghost remote (no section in config, rclone errors) → the run is still success (D21 a push failure does not change the run terminal state) + remoteStatus=failed + remoteError carrying a rclone stderr digest + backup.remote-failed actually received via webhook (event/resource_id/level=error) + the local archive stays complete and downloadable; R3 after deleting the remote artifact sync-remote restores it through re-push. Real S3/B2 cloud endpoints stay suspended (they need cloud credentials; rclone's differences between local and object-storage backends lie in the transport layer, and the push orchestration semantics are already proven). Server-side offsite (checklist 58/59) and recorded-archive push (65/66) run inside the server process — the host has no rclone and we do not install it (it would change the production agent capability reporting), remains blocked
- [ ] Timeout and error presentation under rclone netdisk rate limiting — a Docker host + rclone configuration cannot be verified locally, marked as blocked
- [ ] GB-scale complete chain: package a large directory → push → restore on another machine — a Docker host + large storage cannot be verified locally, marked as blocked
- [x] Pre-command hooks: artifacts exported from a real database by `mysqldump` / `pg_dump` land inside the backup; a hook failure aborts this run (no packaging, no push, no cleanup) — accepted in a real container (2026-10-07, probe `scripts/acceptance/agent-backup/probe-hook.sh` B0-B3 4/4 PASS, dual containers debian:12+mariadb-server real database / postgres:16-alpine real database + a static agent): pre_hook really exports the databases through `sh -c` (`mysqldump accdb` over socket with no password, `PGPASSWORD pg_dump -U postgres` over TCP), the artifacts land in the source directory and go into the archive through packaging, and the download+unpack asserts byte by byte that `site/db-acc.sql` contains INSERT + the sentinel row and `site/db-pg.sql` contains COPY + the sentinel row (B1/B3); the failure short-circuit — `echo …>&2; exit 3` → run=failed, Error=`pre-hook failed: exit status 3: hook-boom-stderr` (the exit code + a stderr digest pass through), File empty and 0 entries in the artifact list (no packaging, D28) (B2); the timeout branch is covered by unit test (`backupHookTimeout` injection + killHookGroup killing the whole group; a real 5min wait is not reproduced in the probe); the `[hook]` log prefix has no API read surface (backup runs only expose a runs list, no task log endpoint), covered by unit test (backup_hook_test.go)

## Panel Database Backup (server side)

Design: [server-backup-design](/guide/server-backup-design).

- [x] The scheduled `VACUUM INTO` fires: the artifact is a compact complete copy, reopened with `sqlite3` and spot-checked against table data — accepted locally (2026-10-05, probe `scripts/acceptance/server-backup/` five-piece set B0-B9 26/26 PASS, dual instances `:19995`/`:19996`): the production-cadence timer loop (1h tick) fires at T+60min, both instances produce one scheduled artifact each (`cockpit-20261005-032956/032958.db`; the probe never calls `POST /run` before this point); `sqlite3` reopens the scheduled artifact with integrity_check=ok, the core tables users/audit_logs/settings are present (users=1, audit_logs=6), and the size matches the live db (483328B); the manual `/run` artifact is fetched back through the download API with a sha256 matching the file on disk (8ccbf96a…), and it is also complete when reopened
- [x] retention cleans up by day; the `0=permanent` form — both forms proven within the same tick: the old file on the retention=1 instance (ModTime 2026-07-01, earlier than the cutoff now-1d) is removed after the tick (gone from both the list and disk); the old file on the retention=0 instance is kept, and after a manual backup the three coexist (old + scheduled + manual). Validation surface: interval=200/retention=-1 both 400, unknown-name download/delete both 404
- [ ] rclone real remote (gdrive/S3) push; on failure only a `[remote]` log entry + a `server_backup.remote-failed` notification is recorded, local backups unaffected
- [ ] The "re-push" button takes effect on historical files

## Remote Session Recording

Design: [recording-design](/guide/recording-design).

- [ ] After a terminal/desktop session recording is archived, the asynchronous push to a real rclone remote succeeds — an rcloud remote cannot be verified locally, marked as blocked
- [ ] A push failure sends a `recording.remote-failed` notification, the local archive is kept, and "re-push" succeeds within the retention window — an rclone remote cannot be verified locally, marked as blocked

## Remote Control: Three Protocols (Guacamole SSH Access)

Design: [remote-access-integration-design](/remote-access-integration-design).

- [x] Real guacd + real sshd: connects with password auth, connects with private-key auth (`private-key` passes **PEM plaintext** through — the design's base64 wording was overturned by measurement: guacd's `guac_common_ssh_key_alloc` does zero base64 decoding, base64 text fed into libssh2 must fail, A/B measured on 2026-09-30, the gateway now passes the raw text straight through)
  Evidence (2026-09-30): probes S1–S4 all pass — container sshd password/ed25519 OpenSSH/RSA PEM three-way auth + a local real sshd (127.0.0.1:22 + read-only `~/.ssh/cui`) connecting and reading back the remotely written `.acceptance/guac/evidence/probe.log`; on the browser side CDP C3 re-verification (GuacamoleModal fills in the private key to connect + typing in the terminal and reading back) `.acceptance/guac/evidence/cdp.log` + `cdp-03b-guac-ssh-connected.png`
- [x] SSH terminal renders correctly (full-screen programs like vim/top), and the window's `size` change moves columns/rows accordingly
  Evidence (2026-09-30): probe S5 (1280x800 → 1920x1080 → restore, `stty size` rows/cols follow in all three states) + S6 (`vi` full-screen input passes through and lands on disk) + S7 (`top -d 1` full-screen periodic repaint img/copy/rect/sync traffic statistics) `probe.log`; CDP C3 screenshot `cdp-04-guac-ssh-top.png` (full-screen top rendering inside the browser)
- [x] SSH session `.guac` recording is collected into `/recordings` and SessionRecording playback works
  Evidence (2026-09-30): probe S10 (recording metadata written into the table + `/api/recordings/{sid}/cast` HTTP 200, bytes≈49K, durationMs>5000, sample stored at `evidence/<sid>.guac`); on the browser side CDP C4 — the Recordings page's GuacPlayer plays and renders to canvas (screenshot `cdp-05-recording-guac-playback.png`). Reached after fixing two places: the gateway's recording-name must be passed as `<sid>.guac` (guacd uses it verbatim as the filename) + the web GuacPlayer works around the guacamole-common-js 1.5.0 `SessionRecording(Blob)` upstream defect (both dist builds lack the `recordingBlob = source` assignment → always 0 frames; switched to injecting the instruction stream through a duck tunnel)
- [x] SSH clipboard is bidirectional (browser ↔ sshd)
  Evidence (2026-09-30): probe S8 (inbound `clipboard`/`blob`/`end` stream instructions + popup from right-click and from uppercase V+Ctrl, two paste paths, the remote `read` receives the marker) + S9 (cross-line drag selection in the terminal pops up → the server-side clipboard stream payload carries the remote output text) `probe.log`
- [x] The "Built-in Terminal (via Agent)" fallback entry still works (the retreat path when guacd is down)
  Evidence (2026-09-30): probe S15 (terminal WS (subprotocol ticket) connect frame + input landing on disk `FALLBACK_OK`) + S16 (after `docker stop guacd` the guac tunnel deterministically fails, the built-in terminal still connects, container restored after verification) `probe.log`; CDP C7 on the browser side (Workbench SSH Tab "Built-in Terminal (via Agent)" → TerminalModal credential form → xterm rendering + the agent direct-connection link initiated) `cdp-08a/08b-terminal-fallback*.png`
- [x] No regression on the existing RDP/VNC paths (three protocols coexisting on the same stack)
  Evidence (2026-09-30): probe S13 (Xvnc:5900 password auth + incremental img/copy/rect frames before/after typing) + S14 (xrdp:3389 connect + incremental frames) `probe.log`; CDP C5/C6 (browser GuacamoleModal VNC/RDP form connect, desktop/login screen canvas rendering) `cdp-06-guac-vnc.png`, `cdp-07-guac-rdp.png`. The RDP/VNC targets are the acceptance self-built containers (locally there is no real Windows/RDP host and no physical VNC desktop), so the path and the graphics stream layer are verified

## Reverse Proxy

Design: [proxy-design](/guide/proxy-design). Prerequisite: one nginx host machine (from 2026-10-07 the container-host form replaces the bare-metal install: nginx and the agent share a container, and both conf.d and reload happen on the machine nginx actually lives on); one Traefik container with a dynamic directory from the host mounted.

- [x] nginx: the full chain for creating a site (render → `nginx -t` → write the fragment → reload), reachable from a browser
  Evidence (2026-10-07): probe `scripts/acceptance/nginx/probe.sh` N1 — http site (websocket) apply 200 + fragment written to disk (meta comment/proxy_pass/Upgrade header all present) + sites/site.get read back + real reachability via `curl -H Host` (marker upstream); https site with a self-signed certificate applies + `curl --cacert` certificate verification passes (the certificate reference is genuinely loaded) + 80→443 301 redirect; delete 200 + fragment removed
- [x] nginx: a syntax error is blocked by `-t` and never lands on disk; after a failed reload rolls back, reloading again leaves the site working on the old config
  Evidence (2026-10-07): probe N2/N4 — what actually blocks is the **pre-parse of reload** (the -s process of `nginx -s reload` parses the full config itself before the master sends SIGHUP, so a syntax error in the new fragment or a missing certificate hits emerg in that process and exits non-zero → D4 rollback → 502 passing through "rolled back" + the fragment never lands; the pre-write `-t` only protects existing configuration); the synchronous reload failure path (the master is already dead) → 502 + rolled back + no fragment left + after restarting nginx the old site serves again
- [x] nginx: systemd reload and the `nginx -s reload` fallback in a systemd-less environment are each verified once
  Evidence (2026-10-07): probe N3/N5 — the signal container (no systemctl, no systemd, reloadMode=signal) uses the `nginx -s reload` path for the whole apply chain; the systemd container (a debian:12 base image with no systemd, `systemd` installed inside the start command then `exec /lib/systemd/systemd` taking over PID 1) has reloadMode=systemctl, apply goes through `systemctl reload nginx` and the unit stays active. Along the way the fallback criterion is fixed: `systemctlMissing` gains the "not been booted" variant (a container with the systemctl binary but systemd not running was wrongly reported without falling back)
- [x] nginx: the error digest presentation in the 80/443 port conflict scenario — the acceptance conclusion is rewritten: under nginx's inherent semantics a bind conflict **swallows the error asynchronously**, there is no synchronous error digest to present
  Evidence (2026-10-07): probe N4 — the -s process of `nginx -s reload` does not bind a socket, so a bind conflict (both the 80 and 443 families measured) leaves apply returning 200 + the fragment on disk, while the master's bind failure lands asynchronously in error.log (emerg `Address already in use`) and **the old configuration keeps serving** (the data plane is safe, "if apply fails the site stays as before" still holds); the systemctl mode is isomorphic (the ExecReload of debian's nginx.service is exactly `nginx -s reload`). For the reporting layer to close the loop it would have to sniff error.log after reload (heuristic), recorded as candidate M2; `probe.log`
- [x] Traefik: dynamic directory probing (static config `providers.file.directory` and the default path); after a site file is written the hot load takes effect
  Evidence (2026-09-30): probe `scripts/acceptance/traefik/` (a traefik:v3.5.6 container with a host directory mounted + host-gateway loopback to two upstreams) T1/T2/T3/T9 — env override probing (`COCKPIT_TRAEFIK_DIR`, the D12 production form; the static-config parse branch is a pure function, covered by unit test) with the capability `traefik-proxy` dynamicDir correct; add/modify (upstream A→B)/delete (three surfaces: file/route/list all removed) all take effect immediately through watch hot load, `reloadMode=hot`; `probe.log`
- [x] Traefik: bad YAML is rejected by the self-check and never lands on disk; router→service reference validation blocks it
  Evidence (2026-09-30): the self-check/reference validation is a defensive branch of the renderer (rendering with legal parameters is always legal, unreachable over REST), covered by unit test (the `TestTraefikProvider_Render` family); on the real-machine side T6 verifies that extra is rejected (error, nothing lands on disk, nothing enters the list, the D14 injection surface) + T7 measures the semantics of an externally bad file — Traefik freezes the whole-directory hot update (existing last-good files keep serving, new changes are rejected with 404, the log names the bad file), and rm / re-distributing from the panel auto-unfreezes while the backlogged changes take effect at the same time (the D13 wording is corrected accordingly); `probe.log`
- [x] With both backends on one host the capability split is correct (nginx takes priority, old agents fall back to the nginx.* prefix)
  Evidence (2026-10-07): probe N6/N7 — a container with nginx + `/etc/traefik/dynamic` coexisting reports two capabilities (nginx-proxy + traefik-proxy), apply lands in conf.d (`proxyRPCPrefix` nginx priority proven); an agent with no backend (bare container) falls back to the `nginx.` prefix → agent `unknown provider: nginx` 502 (the compatibility path really goes out). All four split cases are now verified on a real machine (nginx only = this batch's N1, traefik only = 2026-09-30, dual backend/old agent fallback = this batch's N6/N7)

## ACME Certificate Issuance

Design: [acme-design](/guide/acme-design). Prerequisite: DNS credentials (Cloudflare / DNSPod / Aliyun, any one) + a publicly resolvable domain; use staging first to avoid rate limiting.

- [ ] Staging issuance works end to end (the account ECDSA key is lazily registered and persisted, a second issuance does not re-register)
- [ ] Production real certificate issuance; the expiry time and the renewal threshold (7-90 days) are displayed correctly
- [ ] Each of the three DNS providers does one round of issuance (cloudflare / dnspod / alidns, the credential-determination wording names which key is missing)
- [ ] Auto-renewal: move the threshold close to expiry to trigger re-issuance; a deliberately failing scenario retries with 1h throttling without hitting the CA rate limit
- [ ] Issuance → auto-deploy pushes to the agent-specified path (certificate 0644 / private key 0600) → the nginx site references it → reload → the browser trust chain is complete
- [ ] After renewal the agent-side file is updated in sync; a deployment failure only records status and does not roll back the issuance
- [ ] Downloading a private key always writes an audit entry (download_key); the audit detail contains no PEM

## DDNS Dynamic Domains

Design: [ddns-design](/guide/ddns-design). Prerequisite: a Cloudflare token + a home broadband dynamic IP environment (IPv6 preferred).

- [ ] The agent's egress IP multi-source probing really takes effect (the primary source failing switches to the backup automatically; the IPv4/IPv6 address families are each correct)
- [ ] A missing record is created automatically (TTL auto, no proxy); an IP change update preserves the original TTL/Proxied
- [ ] Multiple records in the same zone+type share one round of ListRecords (watch that the Cloudflare backend call frequency is not amplified)
- [ ] Failure alert deduplication (one report only while the same record stays unread); after recovery it silently self-heals and clears the error
- [x] Changing the inspection interval takes effect immediately; the `0=off` form — accepted locally (2026-10-07, probe `scripts/acceptance/ddns/probe-scan.sh` D1-D5 6/6 PASS, a zero-external-dependency form: the config points at a ghost agent with no CF token configured, the check fails fast at the provider/agent precondition but CheckedAt/LastStatus/LastError are written back on every error path, and scanning is observed by advancing CheckedAt): the default 300 reads back, 59/86401/non-numeric are rejected with 400 and legal values are not polluted; interval=60 scans when due (failed written back + LastError non-empty) → advancing continuously (measured gap=120s — under the combination of a 60s threshold and a 60s tick, `Since(lastScan)<60` occasionally truly skips a beat, the threshold semantics are expected behavior and not a defect) → after PUT 0 CheckedAt freezes across ≥1 original scan window → changing back to 60 restores advancement (interval changes taking effect immediately proven in both directions). Real CF record comparison / egress IP probing / alert deduplication stay suspended (120-123 need a real token and a dynamic IP environment)

## DNS Domain Management

Design: [domain-binding-design](/guide/domain-binding-design). Prerequisite: a DNSPod account (the free tier preferred, to expose the real limits).

- [ ] DNSPod free-tier TTL floor: when editing a TTL below the free floor, the real API error is passed through and presented
- [ ] CAA record add/edit/delete
- [ ] Measured editing of SRV records on both Cloudflare and DNSPod (field mapping and read-back)

## Overlay Network Observation (Overlay)

Design: [overlay-design](/guide/overlay-design). Prerequisites: a ZeroTier Central / Tailscale admin API token; several real hosts running ZeroTier/Tailscale/WireGuard/frp.

- [x] Runtime observation, wg half: a real interfaces snapshot + private key / pre-shared key absent
  (2026-10-07, scripts/acceptance/overlay/probe-wg.sh W1-W3 all PASS — a real
  wg0 in a NET_ADMIN container (wireguard-tools builds the interface, the kernel
  wireguard module auto-loads on the host side): W1 a real snapshot wg0/listenPort=51820/peerCount=1/endpoint and
  allowed-ips verbatim; W2 the private key and pre-shared key hex are absent from the response (the D6 whitelist);
  W3 the peer public key (public data) verbatim. **Acceptance caught and fixed a real defect**:
  the new wireguard-tools (v1.0.20200513+) interface row has one extra public-key column
  (proved on a real machine with 1.0.20210914,
  `<iface> <private-key> <public-key> <port> <fwmark>` five columns), the old parse took
  fields[2] by a fixed index and rendered the public key as listenPort — the private key did not leak
  (fields[1] is always discarded), but the port
  field was displayed as a stretch of base64; fixed to take the second-to-last column (fwmark is always the last column),
  compatible with both generations of layout +
  a regression unit test for the new layout, package coverage held at 100.0%)
- [ ] Runtime observation, ZT/TS half + cross-agent same-node merge: a real peers snapshot and the merge marker
  (suspended: needs a real zerotier network / tailscale tailnet login and a second host, the cloud and
  daemon sides cannot be fabricated locally, to be added with external resources; the parse/merge logic is already unit tested)
- [x] frp graded observation: zero config yields only version+process; with the admin address configured the tunnel counts appear
  (2026-10-07, scripts/acceptance/overlay/probe-frp.sh O1-O3 all PASS — a real frp 0.61.2 inside the container
  (frps bind 7100 + admin 7400, frpc admin 7500 + one tcp
  proxy, loginFailExit=false so the first connect doesn't permanently exit on not-yet-listening): O1 zero config status=ok +
  version + both processes running with no tunnels/proxies keys; O2 both admin addresses →
  frpc.tunnels=1, frps.proxies=1; O3 frpc admin dead port → status=degraded +
  adminError, the frps side is unaffected. **Acceptance caught and fixed a real defect**: frps has no frpc's
  `/api/status` (404 on a real machine — the original implementation called it and always got adminError 404 + degraded, frps
  admin had never worked on a real frp); fixed so frps sums the proxyTypeCount of the dashboard's stable route
  `/api/serverinfo` (fetchFRPSAdminProxyCount) + a unit test
  (including the truncated-body misread branch), the wording of overlay-design.md D7 and the data source table is corrected in step)
- [ ] Cloud management: pull ZT networks/members and the TS device list with a real token; authorize/unauthorize, remove member, delete — all four change types take effect end to end (re-checked in the cloud console)
- [ ] Unmanaged device identification: manually add a device in the cloud that is not in the panel → the "Unmanaged" badge appears
- [ ] Agent identity reporting: the ZT node id / TS device id of the local identity chip is cross-checked against the cloud by the same key → the "Managed by Panel" badge

## Disk Health (SMART)

Design: [disk-health-design](/guide/disk-health-design). Prerequisite: a real host with physical disks (smartctl available).

- [x] Disk discovery and field reading: passed/temperature/remapped/pending sectors/NVMe media_errors real values are plausible
  (2026-10-07, scripts/acceptance/smart/probe.sh S1-S4 all PASS: the local /dev/sda real disk,
  the agent started as root (as non-root smartctl only returns an empty rc=0 shell JSON), S1 disk discovery
  available+/dev/sda+model+sizeBytes, S2 health consistent with a direct smartctl -H read (passed),
  S3 the temperature channel is consistent (the VM disk has no sensor → absent on both sides, the omitempty 0 value is not rendered), S4 a missing attr table
  is not faked (when the ground truth has no attr table, realloc/pending are absent in the panel too, no 0 value is fabricated to pass for a real read).
  Boundary note: the surface of non-zero real attr values (remapped/pending sectors/NVMe media_errors real numbers) needs a host with a real
  attr table — this machine is a QEMU VM virtual disk (health=passed, temp=0, no attr table);
  the parse mapping is already covered by smart_parse unit tests (ATA passed/failed, NVMe, no smart_status,
  bad JSON, raw numbers), measurement on a real-value host follows with NAS/physical machine resources)
- [ ] Inspection alert: a FAILED disk at error level, sector/media anomalies at warning level, one report only while the same disk stays unread
  (suspended: a genuinely bad disk cannot be fabricated locally — the FAILED/sector-anomaly grading and the agent filter are unit tested by smart_scan
  (TestScanSmartOnceCreatesAlert/WarningLevel/Filters), and true alert deduplication runs through the same alert
  pipeline as drift; the end-to-end flip from passed → failed needs a physically bad disk, to be added with external resources)
- [ ] (As needed) The sudo-elevated read path under a non-root deployment
  (suspended: needs a host sudoers change, no system-level change will be made — the read chain is already proved in root form
  (probe S0 root agent + the whole smart.status chain), the sudo -n wrapper is a deployment-form option,
  added with the real non-root deployment need)

## Firewall Observation (M1)

Design: [firewall-design](/guide/firewall-design). Probes: `scripts/acceptance/firewall/probe.sh` (three agent forms locally: root/no nft in PATH/non-root) and `scripts/acceptance/firewall/probe-hosts.sh` (container-host form: bare container/iptables-legacy/huge rule set, H1-H4).

- [x] Capability reporting: nft/iptables metadata matches the real installation; the dual-backend selection (D2) really falls back (2026-10-07 local probes F1/F3: nft goes through nftables, after removing nft from PATH iptables-save + variant parsing resolves nf_tables)
- [x] Rule snapshot really read: both nft -j JSON and iptables-save parse a real rule set, the table/chain/rule counts are plausible against `nft list ruleset` (2026-10-07 local probes F2/F3: 5 tables, 22 chains, 114 rules, ipv4+ipv6 both families)
- [x] Non-root explanatory state: available=false + the real permission error passed through (2026-10-07 local probes F4, "you must be root")
- [x] A single read permission tier (D10): a custom role without firewall:read gets 403, the built-in viewer gets 200 (2026-10-07 local probes F5)
- [x] Browser-measured three states of the page: overview / per-host panel / policy badge / cockpit annotation rendering under real data (2026-10-07 probe-web W1-W6: headless Chrome driving the real web/dist — empty-state Empty copy, overview row backend+variant annotation, input accept badge, unreadable/partially-readable state columns, panel rule detail + cockpit Tag; assertions go through DOM innerText, the headless-shell container has no CJK fonts so the screenshot shows tofu, which is a container glyph gap)
- [x] iptables-legacy host (variant=legacy): real host readings and the variant annotation (2026-10-07 probe-hosts H2 real data-plane read + probe-web W3a/W4 page variant annotation and rule detail)
- [x] Bare container host with no firewall tools: the page's empty state (2026-10-07 probe-hosts H1 data-plane 502 unknown provider + probe-web W2 empty-state copy rendering)
- [x] Huge rule set (>4MB): the truncated alert and the meta-only / line-boundary truncation presentation (2026-10-07 probe-hosts H3 nft 7.7MB meta-only + H4 iptables 5.1MB line-boundary truncation leaving 16362 entries; probe-web W3d a 4MB alert at the page top + W6 the panel's meta-only error message not falsely reporting "rule set is empty")

## Log Search

Design: [logs-design](/guide/logs-design). Prerequisites: several agents (including a systemd host and a docker host).

- [x] The trailing real-machine item: journalctl high-frequency output tails smoothly with no duplication and no gaps
  Evidence (2026-09-30): probe `scripts/acceptance/logs/` (a high-frequency source in the system domain using transient units, local experiment conclusion: `journalctl -u` does not match user-domain transient units, they must be created in the system domain via sudo systemd-run) T1 — tail=2000 backfills 500 lines + a 600 lines/s live burst of 6000 lines, the 6500-frame sequence is strictly increasing, zero duplicates and zero gaps (each sequence number carries a unique per-round base to guard against cross-round journal accumulation); acceptance incidentally fixed two streaming-chain defects: ① the agent-side logs POST (query/follow) was generalized by RBAC from the method into `logs:write` — the only legal permission set is `logs:read`, so every role (including admin) got 403 and the endpoint was entirely unreachable (existing tests called the handler directly and bypassed the middleware, which is why nothing caught it), fixed to a fixed read matching `/api/logs/search`; ② the audit middleware's `responseWriter` wrapper drops `http.Flusher`, so the production path's `w.(http.Flusher)` assertion always fails → 500 "streaming unsupported" (the same class of lesson as the Hijacker on /ws, fixed by passing the interface through to preserve auditing); `probe.log`
- [x] After a docker container stops, tailing ends normally with `reason=exited`
  Evidence (2026-09-30): T2 — an alpine container outputting in a loop, follow collects ≥3 lines then `docker stop -t 2`, `docker logs -f` ends with the container exiting → eof `reason="exited"`, the DOCLINE sequence is ordered with no duplication; `probe.log`
- [x] The 10-minute timeout fallback takes effect
  Evidence (2026-09-30): T3 — an idle source (fully silent after prewarm) stays in follow (`journalctl -f` does not exit by itself for a finished unit, measured locally as staying resident, the timeout is the only exit), at the due point eof `reason="exited"` with an elapsed time of exactly 10m0s and zero new frames after the settling period; the unit-test surface `TestLogsFollowTimeoutKillsIdleSession` (`logsFollowTimeout` is injected as a behaviour-neutral var, production is always 10min); `probe.log`
- [x] Cross-machine federated search: multi-agent parallel fan-out returns grouped by host; offline / no logs capability / path not found — the three skipped attributions are correct; a single agent failing does not error the whole thing
  Evidence (2026-09-30): T4a/T4b — a 3-agent topology (full PATH / empty PATH with no logs capability / a journalctl failure shim masking a host that has the capability but whose query always fails) + a ghost id sample, all four present: a1 ok=true with hostname grouping, a4 ok=false error degradation with HTTP 200 and no whole-request error, a2 skipped `no-logs`, ghost skipped `offline`, results sorted by agentId; a full fan-out only lists online hosts without the capability (offline hosts are naturally absent and unlisted); note on the criterion: the "path not found/not-found" attribution has been merged into offline (api_logs_search.go treats absence from the registry as offline and does not distinguish not-found, the ghost id is the live proof of exactly that form); raw request/response `search-*.json`

## NAS

Design: [nas-design](/guide/nas-design). Prerequisites: real machines on DSM 6.x/7.x, TrueNAS CORE/SCALE, OMV each (verify if available).

- [ ] Measured API field differences between DSM 6 and 7 (login, volumes, shares)
- [ ] TrueNAS CORE vs SCALE field differences (flexInt string-number form)
- [ ] OMV: the full `X-Openmediavault-Sessionid` header auth flow; `"1.50 GiB"` capacity string parsing; a 2FA account degrades to an equal failure
- [ ] mdadm degradation drill: pull a disk / mark it fault and the alert is delivered (true deduplication)
- [ ] Real enumeration of ZFS / SMB / NFS hosts
- [x] The `COCKPIT_NAS_TARGETS` jump-host probing mechanism: the full chain targets parse → dial → unreachable degradation → source
  normalization (2026-10-07, scripts/acceptance/nas/probe-jump.sh J1-J2 all PASS —
  a static agent in a bare debian:12-slim container: J1 legal targets (a dead port dsm) → nas/status
  200, source=linux without any dsm mixed in, pools/shares empty, available consistent with df mounts
  (an unreachable target is only logged and skipped, no crash and no timeout); J2 non-JSON env is ignored wholesale without crashing.
  Two form proofs: a docker bind mount makes a real host block device visible in the container's df (≥1GB
  counting into mounts is correct behavior); the shared host /proc → mdstat always exists → the DetectNas
  mdstat branch is always true on Linux (the nas capability is always reported) — the env registration branch is only
  visible on Windows/macOS jump-host platforms, so it belongs to the parse layer (unit tested). Real DSM/TrueNAS/OMV remote
  observation stays suspended pending a real NAS

## Service Management (Three Backends)

Design: [service-design](/guide/service-design). Prerequisites: one systemd Linux host, one Windows host, one macOS host.

- [x] systemd: the list includes unloaded units; start/stop/restart/enable/disable measured; after masking, hide-on-start and explicit unmask appear, and unmask restores
  Evidence (2026-10-01): probe `scripts/acceptance/services/` (locally :19993, a root/cui dual-agent contrast, test units `cockpit-acc-*` exclusive) two rounds 9/9 PASS — first round: T1 list total=313>active=72, files-only unloaded items are in the list (template/unloaded classes like `apport-coredump-hook@.service`, the `mergeServiceUnits` branch); T2 restart→active/running, enable→enabled; T3 a non-root restart/enable of the same unit both give 502 with the body carrying the `systemctl` original text (Access denied…interactive authentication) passed through verbatim, while the control group's non-root list read gives 200 (a pure permission failure); T4 stop→inactive, disable→disabled to finish up; acceptance caught a production defect: the `serveAPI` dispatcher matched only `Contains "/services/"` (requiring a trailing slash), so a bare `GET /api/agents/{id}/services` (the list endpoint the docs and web `api.ts:1043` call) fell into `handleAgentGet` and returned 404, making the web service page list entirely unreachable (existing tests called the handler directly and bypassed the dispatcher, which is why nothing caught it), fixed with the same `HasSuffix || Contains` as the `/health` and `/domains` rows + a dispatcher regression test `TestServiceListDispatcherRoutesBarePath` (reverse verification: without the fix it must FAIL); the supplementary verification round: a ghost unloaded sample (install to `/etc` and never enable/start → `loadState` empty + `activeState=inactive` + `unitFileState=disabled`, a two-sided check against both the running table and the install table), start/stop round trips, disable/enable round trips, mask→`masked` (restart on a masked unit gives 502, the is-masked original text passes through) → unmask restores `enabled` (the list state updates in sync, the web masked row hiding start and showing explicit unmask is driven by exactly this field), and the `service_action` audit write assertion (unit/agent both present); teardown deletes the unit file + daemon-reload so it can be re-run (run-server normalizes the old instance's DB, preventing old agent rows from polluting the registration count); evidence `probe.log` (two rounds) + `setup.log` + `services-list.json`
- [x] systemd: the daemon-reload toolbar button; unit file view/edit (a package-managed file is first copied to the override location under `/etc/systemd/system` and only then edited, so upgrades do not lose it)
  Evidence (2026-10-01, R3 round): the same probe extended to 17/17 PASS — T4 unit file full chain: GET a valid view (`systemctl cat` full text + `fragmentPath=/usr/lib/systemd/system/...` package-managed location); PUT changing Description → the package-managed file is first copied to the `/etc/systemd/system` override location and only then written (PUT response `path=/etc/...` + the file landing under `/etc` + `fragmentPath` reading back with the new location, three proofs) + the save is bundled with daemon-reload (`systemctl show -p Description` reflects the new value immediately — impossible without the reload, end-to-end ironclad proof); content over 256KB+1 gives 413 (the server's `MaxBytesReader` and the agent share the same limit, defence on both ends); the rejection surface: a unit name outside the whitelist (including `:`) gives 400, an extra path segment in `{unit}/file/x` gives 404 (where it writes is decided by FragmentPath, only the unit name is accepted, and with no path parameter there is no traversal surface), a windows-shaped name (`zzz-notaservice`) passes the server's dual-backend union whitelist and is then refused by the agent's systemd fallback with 502 passing through `invalid unit name ... expect *.service` (designed behavior per D9.4); a non-root PUT gives 502 + `permission denied` original text passed through (contrasted against the same operation as root returning 200); audit `details.action=unitfile-save` written. The daemon-reload endpoint: `POST /agents/{id}/services/daemon-reload` returns 200 `{"reloaded":true}` + an audit write; the web wiring is in place — the toolbar "Reload Configuration" Popconfirm (`Services/index.tsx:381-386`) and the "Files" Modal (`:320-494`) go through `api.ts:1056/1063/1070`, the same three endpoints. No product defects in this round (the first round's c88a48a already fixed the serveAPI dispatcher 404)
- [x] systemd: the "Logs" jump on a service row reaches the journalctl history (including unloaded units)
  Evidence (2026-10-01, R3 round): T5 (`cockpit-acc-jlog.service` sample, installed to `/usr/lib` and never enabled) — PUT swaps in a oneshot ExecStart carrying a unique marker (the dogfood override position + the bundled reload: what start executes is exactly the content after the PUT) → start produces journal history → stop + daemon-reload turns it into an unloaded install entry (three proofs: not in `systemctl list-units --all`, service table `loadState=""`+`unitFileState=disabled`, not in `/logs/sources`) → `POST /agents/{id}/logs/query {"type":"systemd","source":"cockpit-acc-jlog.service"}` returns 200 and `lines` contains the marker (`journalctl -u` does not care about the load state); the web path is the same shape: the "Logs" button on a service row opens a Drawer with an embedded LogsPanel (`Services/index.tsx:506-514`), `initialSource` unconditionally derives and never falls back for a unit name (`LogsPanel.tsx:124-126`, an unloaded service is not in the `logs.sources` list but is still searchable); a non-root daemon-reload gives 502 with Access denied passed through (recorded as a contrast)
- [ ] Windows SCM: list/start-stop/autostart toggle/restart 30s timeout path measured; a no-permission service error is passed through; the reload button is confirmed hidden
- [ ] macOS launchd: list/start-stop/autostart toggle; a `kickstart` failure falls back to the `bootstrap` path; a no-permission error is passed through

## Unified Job Execution (agent.exec)

Design: [jobs-design](/guide/jobs-design). Prerequisite: one online Linux agent (the command is executed through `sh -c`).
Real-machine acceptance (2026-10-05, probe `scripts/acceptance/jobs/` five-piece set 9/9 PASS, evidence
`.acceptance/jobs/evidence/`: probe.log + create-J1..J5.json + audit-job_run.json).

- [x] Creating a Job (online agent, `uptime`) → the dialog echoes success, non-empty output, exit code 0; the ledger gains one entry (visible within the 15s poll)
  Evidence: J1 `echo J1-OUTPUT-MARK && uptime` returns in 82ms with success/exitCode 0/80B of output containing the marker,
  actor/type/target/parameters round-trip consistently, startedAt/finishedAt land; the ledger goes 0→1 entry and the reverse-order first row
  is exactly that entry, and the single-entry detail contains the same output. Web wiring in place: 15s polling `Jobs/index.tsx:51`
  (refetchInterval 15000), the terminal success toast echoes per STATUS_META (`:84`),
  menu perm `jobs:read` (`App.tsx:211`)
- [x] Non-zero exit (`exit 3`) → failed, exit code 3, error falls back without output
  Evidence: J2 `echo J2-BEFORE-OUT; exit 3` → status=failed, exitCode=3, output from before the exit is still carried back
  (a business field per D4); the error is actually `exit status 3` (normalized by the agent's trimExecErr, the design's "error
  empty" wording is corrected per the implementation — the error message for a non-zero exit and for a timeout shares one field)
- [x] Timeout (`sleep 30`, timeout_s=1) → failed, error contains timed out; the agent SIGKILLs the process group, `ps` re-check shows no orphaned sleep residue
  Evidence: J3 `sleep 297` timeout_s=1 → failed, exitCode=-1, error=`timed out after 1s`,
  wall clock 1.078s (returns on time rather than waiting it out); `pgrep -f "sleep 297"` re-check has zero hits (the process group is killed clean,
  no grandchild process holding a pipe open)
- [x] Output truncation: >64KB of large output → truncated flag, business fields do not blow up the RPC
  Evidence: J4 `seq 1 20000` (about 108KB in total) → success, output exactly 65536B, the tail line 20000 is preserved,
  the first line 1 is cut (the agent's `jobExecMaxOutput` and the server's `storage.JobMaxOutput` enforce the same tail truncation on both ends).
  The `truncated` flag lives inside the agent RPC data and is not exposed by the jobView (the meaning is corrected per the implementation)
- [x] Offline agent: the create dialog disables that machine in the dropdown; a direct `POST /api/jobs` → 503, no ghost Job record is left
  Evidence: J5 target `jobs-acc-ghost` (never registered; the same registry.Get branch as "was online then dropped", the same criterion as
  logs T4) → 503 `agent offline`, the ledger stays 4→4 entries, 0 records for the ghost target. The web dropdown
  `Jobs/index.tsx:65` `disabled: a.status === 'offline'` and the label is marked "(offline)"
- [x] Audit: CreateJob and the terminal `job_run` (resource=job, details containing type/target/status/params) leave a trace on the audit log page
  Evidence: J7 `GET /api/admin/audit/logs?resource=job&action=job_run` hits exactly one entry per Job for J1-J4
  (resource_id=Job ID, username=admin, details containing type/target/status/params.command);
  the 503/400 validation failures before creation produce no audit, and the audit page is searchable by resource=job
- [x] Permissions: without `jobs:write` the "Execute Command" entry is not visible; without `jobs:read` the Jobs page cannot be entered
  Evidence: J8 viewer (jobs:read) GET list/detail 200, POST 403; a custom role with only `dns:read` gets
  403 on both GET and POST; the web entry PermGuard `jobs:write` (`Jobs/index.tsx:169`) + menu
  `jobs:read` (`App.tsx:211`). **Acceptance caught a real defect**: `/api/jobs` had no `resourceRules` registered
  → RBAC governed=false let everything through and even a viewer could execute (same class as agent-tags before the rule was added);
  fixed by adding `{"/api/jobs", "jobs", ""}` to `rbac.go` + a three-role 7-case matrix (rbac_test.go)

## Workflow Orchestration (M1: Async Job + Linear Chain)

Design: [workflow-design](/guide/workflow-design). Prerequisite: one online Linux agent (same as the Job section);
M2b fan-out acceptance needs two (run-server.sh starts the dual instance `workflows-acc-a1/a2`).
M1 is implemented and accepted (2026-10-09): probe `scripts/acceptance/workflows/` (W0-W9, four-piece set +
Go probe, aligned with the jobs probe criterion) 9/9 PASS on a real machine (port 20010, agent `workflows-acc-a1`,
evidence `.acceptance/workflows/evidence/` + `probe.log`).
M2b multi-target fan-out is implemented and accepted (2026-10-10): the probe extended to W10, 12/12 PASS on a real machine
(dual online agents + a ghost sample).
M2c cancel signal is implemented and accepted (2026-10-11): probe W5/W9 semantics reworked to the real-kill
criterion of `job.cancel`, new W11 variable-passing scenario (the end-to-end face of M2a) — the local dual-instance harness
(run-server.sh, port 20010) 13/13 PASS (W5 kill=true/ledger cancelled, W9
running cancel 200+kill cancelled/cancel-after-terminal 409, W11 parse + definition source all fully evidenced);
the W5/W9 entries below are the historical evidence from before the rework, the authoritative semantics are those of the reworked probe.

- [x] Asynchronous creation: `POST /api/jobs` immediately returns 201 pending, the ledger poll sees the terminal state; the "Cancel" entry on a pending row is visible and clickable (W1/W9)
  Evidence: W1 `uptime` → 201 status=pending (the W1 contract, deterministic after the 0d065a1 race fix) → the ledger poll
  success/exit 0/output marker/params round-trip/start and end landed; the pending-row cancel in web is in place `Jobs/index.tsx:159`
  (the pending row's "Cancel" button → `cancelJob`), cancelling a running one is rejected with 409 `only pending` (W9)
  (M2c rework: cancelling a running one changed to RPC kill 200 + `cancelled` — W9 has been reworked and re-verified
  passing 200+kill cancelled+cancel-after-terminal 409)
- [x] Success chain: a two-step Workflow run has every step success, and each step's jobId in the run ledger can be opened to see the output/exit code (W2)
  Evidence: W2 two-step chain (uptime → df -h) run success; the step snapshot success lands in run.steps;
  the Job ledger has 2 entries with actor=workflow, `/api/jobs?workflow_run_id=` hits them; the web run detail
  jobId "View" button reuses the ledger Job view (`Workflows/index.tsx:538` openJob)
- [x] Failure stops: a mid-chain step failure (non-zero exit) → run failed, the third step does not execute, the break point is readable in the snapshot (W3)
  Evidence: W3 three-step chain with `exit 3` on step 2 → run failed; the snapshot [success/failed/pending] break point is readable;
  the Job ledger has exactly 2 entries (the third step has no Job, it was never dispatched)
- [x] Retry: the first attempt fails and the retry succeeds (retry=1) → step attempts=2, each attempt enters the ledger as an independent Job row (W4)
  Evidence: W4 the marker file's first `cat` attempt fails (exit 1), retry=1 succeeds on the retry → steps[0].attempts=2,
  attempt 2 success contains the output marker, attempt 1 failed exit=1 becomes its own row; the two Jobs are measured at an interval ∈[4s,10s]
  (the workflowRetryInterval=5s semantics)
- [x] Cancelling a run: after the in-flight step ends the run is cancelled, subsequent steps do not advance; cancelling again gives 409 (W5)
  Evidence: W5 sleep 12 in flight → cancel 200 → run cancelled+finishedAt; 12s later step 2 still has no
  Job (no advancement), the ledger has 1 entry; cancelling again gives 409 `run is not running`; the web cancel button is visible
  only for running (`Workflows/index.tsx:496`)
  (M2c rework: the in-flight step changed to really being killed by `job.cancel` landing `cancelled`+stopReason=`run
  cancelled`, no longer running out naturally — W5 has been reworked and awaits re-verification)
- [x] Re-entrance and freezing: running the same definition again gives 409; updating/deleting the definition during an active run gives 409 (W6)
  Evidence: W6 a second run inside the active-run window (during the 8s/12s sleep) gives 409, PUT definition gives 409,
  DELETE definition gives 409 (the three 409 freeze surfaces); after cancel sets the terminal state DELETE gives 200 and can clear (W7 tail segment)
- [x] Offline target: a step targeting an offline agent → 503, no run created (the same criterion as a single Job) (W7)
  Evidence: W7 target `workflows-acc-ghost` (never registered, the same registry.Get branch as "was online then dropped")
  → 503 containing `agent offline`, the run ledger has 0 entries (no ghost); the same criterion for a single Job is in J5
- [x] Permissions and audit: without `workflows:write` the run/create entries are not visible; `workflow_create/workflow_run/workflow_cancel` leave audit traces (W8)
  Evidence: W8 viewer (workflows:read) list 200/run detail 200, create/trigger/cancel all 403; a custom role with only
  `dns:read` is 403 across the board (rbac `{"/api/workflows","workflows",""}` at the route layer);
  audit records all three actions with resource=workflow; the web entry PermGuard
  `workflows:write` (`Workflows/index.tsx:307,315,334`) + menu `workflows:read`
  (`App.tsx:222`)
- [x] One round of real-machine orchestration: run a two-step chain of "uptime → df -h" against a real agent and cross-check the ledger/audit/run snapshot (W2)
  Evidence: the local instance (:20010) + agent `workflows-acc-a1` executing for real; run success, both step snapshots
  contain output, the Job ledger has 2 entries with actor=workflow, the audit workflow_run resource_id=run id;
  evidence files run-W2/final-W2/jobs-W2/audit-workflow.json (`.acceptance/workflows/evidence/`)
- [x] Fan-out step: `targets` with multiple machines executes them in order → run success, each machine's snapshot has an independent jobId, the ledger has one entry per machine (W10, M2b)
  Evidence: W10 two machines [a1,a2] `echo FAN-OK && uptime` → success on each, independent jobId rows,
  the ledger has 2 entries; the web step editor has multi-select targets and the run timeline aggregates "N machines" with expandable per-machine detail (vitest 22 cases)
- [x] Fan-out stops on failure: the first machine fails so the run is failed, the next machine stays pending with no Job (the F3 stop-advancement criterion, same as W5) (W10b)
  Evidence: W10b [a1,a2] `exit 3` → [failed/failed, pending] per machine, step-level StopReason=failed,
  the ledger has 1 entry (the second machine was never dispatched)
- [x] A ghost mixed into fan-out: any offline target gives 503 and no run is created (F8 per-machine validation, extending W7) (W10c)
  Evidence: W10c [a1,ghost] → 503 `agent offline for steps[0]`, the run ledger has 0 entries (no ghost run)


## Mobile (Flutter, iOS + Android)

Design: [mobile-design](/guide/mobile-design). Prerequisite: a real Android machine (or emulator) with the debug APK installed (CI `Mobile` workflow artifact or a local `flutter build apk --debug`); the server reachable (public or on the same subnet). iOS build verification is already closed out on the CI macOS runner (`Mobile` workflow build-ios job, unsigned debug build, see D8); iOS real-device install and store signing still need your own Mac/account, so the checklist below uses the Android real machine as the baseline for the iOS side.

- [ ] First-launch onboarding: enter the server address → `/health` passes → persisted, entering the login page; the failure presentation for a wrong address is readable
- [ ] Login: username and password → an account with TOTP enabled enters the code second step → on success the four bottom tabs are usable
- [ ] Dashboard: host online/offline counts, unread/error alert counts consistent with the web panel
- [ ] Host list: online badge, `ip · region · zone` tags consistent with the backend; pull-to-refresh
- [ ] Container operations: a docker host enters the container list, restart really takes effect (the state flips), failures present a SnackBar, and the operation leaves a trace in the web audit
- [ ] Alerts: the list is consistent with the server, unread entries bold with a dot, and after "Mark All Read" the web side is synced as read
- [ ] Audit: paginated first-screen loading, auto-prefetch when the data does not fill a screen, failed rows marked red
- [ ] Session restore: killing the process and reopening goes straight to the main screen (local token); after shortening the JWT expiration the token auto-renews seamlessly on expiry; a failed refresh returns to the login page
- [ ] Security: with the self-signed toggle off a self-signed server connection is refused; with it on it works (the red-ink risk warning is present)
- [ ] Push: the server configured with an ntfy channel, trigger one alert, and the official ntfy app receives it on the phone (the D3 path)
- [ ] Dark mode follows the system switch correctly
- [ ] Resources page: domain/certificate approaching-expiry badges (normal / approaching expiry / expired) consistent with the web resources panel; certificate remaining days presented
- [ ] Backup: task list last_status / next run time consistent with the web; "Run Now" really triggers with SnackBar feedback
- [ ] Cron: on a host with cron capability the entry is visible, and the task list (schedule/command/start-stop) is consistent with the server crontab; external entries readable verbatim
- [ ] Files: directory navigation in and out is smooth, tapping a file pops up the detail (path/permissions/size); the file list loads under a self-signed server
- [ ] Terminal: the single "SSH Terminal" action on a host connects successfully, keyboard input and echo are immediate, command execution really takes effect (e.g. the file is visible on the Files page after touch), and a dropped connection shows an error banner with a reconnect button
- [ ] Biometric lock: the toggle on the settings page can only be turned on on supported devices; once on, killing the process and reopening demands fingerprint/face verification; a failed verification keeps the overlay up with a retry; turning the toggle off means reopening goes straight to the home page

## Post-Acceptance Backfill Convention

- Every time an item passes: check it off in this list + remove it from the "Remaining" part of the matching `todo.md` entry, folding the key points into the completion record
- Defects found during acceptance: file and fix them as separate items per existing discipline (one commit per fix, nothing bundled)
- Once the whole domain passes, annotate "Completed (date)" at the top of this file and archive it to `docs/archive/` (where applicable)
