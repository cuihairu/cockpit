# Docker Deployment Guide

> Language: [中文](./deploy-docker.md) ｜ **English**

Run Cockpit Server on a server using the officially published image (`ghcr.io/cuihairu/cockpit`).
Aimed at deployers who "have a server and want it running"; for running the service locally while changing code see
[Quick start](../guide/getting-started.en.md).

## Two deployment postures, clarified up front

| | Purpose | Image source | compose file |
| --- | --- | --- | --- |
| Pull the image (this document) | production / long-running | `ghcr.io/cuihairu/cockpit:<tag>` | `deployments/docker/docker-compose.yml` |
| Local build | self-testing code changes | local `docker build` | repo root `docker-compose.yml` |

Both compose files define their services from the same source (server + optional guacd); the only difference is the image source. The repo root `docker-compose.yml` additionally maps host port `9090:9000` (a leftover from the CF Tunnel origin era).

## Prerequisites

- Docker Engine 20.10+ and Compose V2 (`docker compose version` must run);
- 1 core and 1 GB of memory to start (guacd for remote access counts separately — allow another 512M);
- Port 9000/tcp open to the outside (or bind to loopback only and go through a reverse proxy).

## Quick start

```bash
# 1. Prepare the variable file
cp deployments/docker/.env.example deployments/docker/.env
vi deployments/docker/.env        # at minimum change ADMIN_PASSWORD / JWT_SECRET / TOTP_ENCRYPTION_KEY

# 2. Start the service (pinning the tag is safer; leaving it empty follows latest)
cd deployments/docker
COCKPIT_IMAGE_TAG=latest docker compose up -d

# 3. Check status
docker compose ps
curl -fsS http://127.0.0.1:9000/health
```

Open `http://<server IP>:9000` and log in with `ADMIN_USERNAME` / `ADMIN_PASSWORD` from `.env`.

Generate random values:

```bash
openssl rand -base64 32
```

## Image and tags

The image is built by `.github/workflows/docker.yml` with buildx on a GitHub-hosted runner and pushed to GHCR. One build produces several tags:

| Tag | Meaning |
| --- | --- |
| `latest` | newest on the `main` branch (overwritten on every merge to main) |
| `main` | the same, explicitly named after the branch |
| `1.2.3` / `1.2` / `1` | the semantic-version series produced when a `v*` tag is pushed (`docker/metadata-action`'s `pattern={{version}}` strips the `v` prefix) |
| `<short sha>` | an immutable snapshot of each build, used for rollbacks |
| `pr-123` | PR builds (not pushed to the registry, only verify it builds) |

**For deployments, pin a sha or a semantic-version tag** (`COCKPIT_IMAGE_TAG=<sha>`) rather than following
`latest` long-term — `latest` moves along with main, so a rollback can never say which version it went back to.

```bash
docker compose pull && docker compose up -d
```

The image is only published for `linux/amd64`: the Server uses CGO (SQLite is hard-bound to cgo), and multi-arch builds need a cross toolchain chain. For arm64 use the binary release packages (`nightly.yml` / Releases).

Check the version baked into the image:

```bash
docker compose exec cockpit-server cockpit version
```

## Environment variables

The complete list is in `deployments/docker/.env.example`.

**Must change** (placeholder values make the container refuse to start; `entrypoint.sh` hard-validates them):

| Variable | Description |
| --- | --- |
| `ADMIN_PASSWORD` | initial admin password, ≥8 characters, not the example value |
| `JWT_SECRET` | JWT signing key, a long random string |
| `TOTP_ENCRYPTION_KEY` | required when `PRODUCTION=true`, ≥32 characters. **Must be backed up once TOTP is enabled** — without it existing secrets cannot be decrypted |

**Optional**:

| Variable | Description |
| --- | --- |
| `COCKPIT_IMAGE_TAG` | image tag, defaults to `latest` |
| `COCKPIT_HTTP_PORT` | host port, defaults to 9000 |
| `ADMIN_USERNAME` | defaults to `admin` |
| `PRODUCTION` | defaults to `true` (turning it off relaxes the Host/Origin checks and other dev conveniences) |
| `BASE_URL` | external base URL, used to build links in emails/notifications |
| `ALLOWED_ORIGINS` | external domain, e.g. `https://cockpit.example.com`. When unset the WS accepts all Origins (with a warning) — **this does not block startup but must be set in production**; `.env.example` ships with `http://localhost:9000` as a usable default |
| `TZ` | defaults to `Asia/Shanghai` |
| `GUACD_ADDR` | guacd address (`guacd:4822` when remote access is enabled) |
| `GUACD_RECORDING_PATH` | guacd recording directory, defaults to `/var/lib/guacamole` |
| `AGENT_EXPIRE_MINUTES` | agent expiry threshold (minutes), defaults to 5: a heartbeat older than that marks the agent expired — the server automatically marks it offline and releases the connection, and the list hides it by default (keys/tags are kept, physical removal goes through the "clean offline agents" button); a negative value disables it. **The current Docker compose does not wire this variable up**, so setting it in `.env` has no effect |
| `GUACD_LOG_LEVEL` | defaults to `info`; **do not enable `debug`** (it prints connect instruction arguments, including passwords/keys) |

## Remote access stack (guacd, optional)

The remote access entry for the RDP / VNC / SSH protocols needs a `guacd` daemon on the same network
(browser-side `guacamole-common-js` + a Go gateway reverse proxy; see
[Remote desktop design](../remote-desktop-guacamole-design.en.md) and
[Unified three-protocol stack](../remote-access-integration-design.en.md)).

```bash
# add one line to .env (a DNS name on the same network, do not write 127.0.0.1 — that is the guacd container itself)
GUACD_ADDR=guacd:4822

docker compose --profile guacd up -d
```

Key points:

- **The port is not exposed externally**. guacd is not an HTTP service; the browser only goes through Cockpit's `/api/remote/guacamole` WebSocket, and the Go gateway dials guacd over the same network.
- **The recording volume must be the same volume at the same path**. compose already mounts `guacd-recordings` into both containers (both at `/var/lib/guacamole`): guacd writes `<session>.guac` at the `recording-path` from the `connect` instruction, and the Go gateway collects the file at the same path when the session ends and archives it into `/data/recordings` (`collectGuacRecording`). If the two paths disagree, recordings are never collected and the `/recordings` page shows no desktop/terminal sessions. Changing `GUACD_RECORDING_PATH` means changing the mount points of both containers.
- When guacd is unreachable SSH still has a fallback: the SSH Tab in Workbench keeps the "built-in terminal (via Agent)" secondary entry (going through the agent channel + xterm.js, not guacd).

## Reverse proxy

A reverse proxy must allow two kinds of WebSocket upgrade, otherwise agents drop off and remote connections cannot be established:

| Path | Purpose |
| --- | --- |
| `/ws` | agent long-lived connection |
| `/api/remote/guacamole` | Guacamole tunnel (RDP/VNC/SSH) |
| `/api/remote/terminal` | built-in terminal (the agent-mediated SSH fallback channel) |

Nginx snippet:

```nginx
location ~ ^/api/remote/(terminal|guacamole) {
    proxy_pass http://127.0.0.1:9000;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_read_timeout 86400;   # remote sessions are silent for long stretches, don't set this too low
}
```

The repo ships a site config template: `deployments/nginx/cockpit.cuihairu.site.conf` (its `location ~ ^/api/remote/(terminal|desktop|vnc|guacamole)` still contains the deleted desktop/vnc endpoints; if you want to converge to terminal/guacamole in production you can adjust it yourself, it does not affect the fallback behaviour of the deleted endpoints).

Under HTTPS the browser uses `wss://` and the Go gateway reverse-proxies same-origin, so no extra configuration is needed.

## Data and backups

All data lives in `/data`, mounted from volume `cockpit-data`:

| Path | Content |
| --- | --- |
| `/data/cockpit.db` | SQLite main database (accounts, config, audit, session records…) |
| `/data/inventory.yaml` | inventory manifest |
| `/data/recordings/` | session recording archives (terminal `.cast`, desktop/remote `.guac`) |

Backup and rollback:

```bash
docker run --rm -v cockpit-data:/data -v "$PWD":/backup debian:bookworm-slim \
  tar czf /backup/cockpit-data-$(date +%F).tar.gz -C /data .
```

Do this once before upgrading. Rolling back = change `COCKPIT_IMAGE_TAG` back to the old tag/sha and run `docker compose up -d` again (the database is forward compatible; there is no automatic downgrade migration).

## Building from source

To build locally on the deploy host (offline, with no registry access):

```bash
docker compose up -d --build          # repo root compose, image name cockpit:local
# or build separately
docker build -t cockpit:local --build-arg VERSION=$(git rev-parse --short HEAD) .
```

`--build-arg VERSION` is injected into the binary's `cockpit version` (`main.version`).
The pnpm version inside the image is pinned (`pnpm@11.19.0`) to match the lockfile.

## Troubleshooting

| Symptom | Investigation |
| --- | --- |
| The container won't start, log shows `ADMIN_PASSWORD must be set...` | `.env` still holds a placeholder value; `entrypoint.sh` deliberately refuses to start on a weak password |
| `/health` unreachable | `docker compose logs cockpit-server`; confirm 9000 isn't taken; when `ALLOWED_ORIGINS` doesn't match the domain you actually access, static assets get blocked by the browser |
| The remote access entry doesn't show | `GUACD_ADDR` is unset or wrong. Leaving it empty hides the RDP/VNC/SSH entries in the frontend (expected behaviour) |
| Clicking remote access won't connect, log shows `dial guacd ... failed` | the guacd container isn't running (`--profile guacd` was missed), or `GUACD_ADDR` was written as `127.0.0.1` (that's the loopback inside the guacd container, unreachable from the Go gateway) |
| No remote session recordings under `/recordings` | the guacd recording volume isn't co-mounted with the server (see "Remote access stack") |
| Remote access handshake fails behind the reverse proxy | the Upgrade header for `/api/remote/guacamole` is missing (see "Reverse proxy") |
| Terminal/desktop sessions drop mid-way | the reverse proxy's `proxy_read_timeout` is too small |

## Related documents

- [Quick start](../guide/getting-started.en.md) —— binary deployment and running locally
- Deployment asset notes `deployments/docker/README.md` —— an item-by-item explanation of the variables and scripts
- [Real-machine acceptance checklist](../guide/acceptance-checklist.en.md) —— functional acceptance after going live
- [Remote desktop design](../remote-desktop-guacamole-design.en.md) /
  [Unified three-protocol stack](../remote-access-integration-design.en.md) —— guacd integration details