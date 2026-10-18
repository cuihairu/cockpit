# Architecture and Boundaries

> Language: [中文](./architecture.md) ｜ **English**

This document is authoritative on the current repository implementation, describing Cockpit's runtime topology, module responsibilities, and boundaries. It is not a long-term vision document; when code behavior changes, this page should be updated first.

## Overview

Cockpit currently consists of three runtime planes:

```text
Browser Web UI
    |
    | HTTP JSON API / WebSocket
    v
Cockpit Server (cmd/cockpit)
    |  \
    |   \ SQLite runtime store
    |
    | WebSocket /ws
    v
Cockpit Agent (cmd/cockpit-agent)
    |
    | local API / socket / TCP
    v
Managed targets (host, Docker, PVE, OpenWrt, SSH/RDP/VNC service)
```

The Server is the central control plane; the Agent is the node-side execution plane. Agents connect to the Server proactively, so nodes behind NAT need not expose inbound ports. The Web UI talks only to the Server, never directly to Agents or intranet targets.

## Process Boundaries

| Process | Entry point | Responsibilities | Not responsible for |
| --- | --- | --- | --- |
| Cockpit Server | `cmd/cockpit` | HTTP API, Web UI static assets, Agent WebSocket registration and routing, SQLite persistence, authentication, auditing, alerting, reverse proxy and remote session orchestration | Directly accessing intranet services behind NAT, directly collecting the Agent's host metrics |
| Cockpit Agent | `cmd/cockpit-agent` | Node capability detection, heartbeat and system metric reporting, RPC/proxy/remote desktop execution, connecting to local or locally reachable targets | User authentication, global state persistence, cross-Agent scheduling decisions |
| Web UI | `web/` | Login, resource views, workbench, monitoring, settings, remote connection interactions | Storing final runtime state, bypassing the Server to access Agents |
| CLI | `cockpit init/sync/status/server/agent` | Local initialization, inventory sync, status queries, starting the Server; `cockpit agent` accepts the same startup parameters as `cockpit-agent start` for compatibility | Long-running Agent duties still live inside the Agent process |

## Data Boundaries

| Data | Location | Role | Lifecycle |
| --- | --- | --- | --- |
| Config file | `config/cockpit.yaml` or the path given via `-config` | Server startup configuration: listen address, database, JWT, email, notifications, inventory | Read at startup; environment variable placeholders are expanded |
| Inventory YAML | `inventory.path` or `cockpit sync -inventory` | Declarative asset inventory | Both manual `cockpit sync` and `inventory.watch` hot-reload write to the database through the same Syncer |
| SQLite | `database.path` | Runtime query and state store | Migrated automatically when opened by the Server/CLI |
| Agent Registry | Server memory | Pool of currently online Agent connections and pending RPC responses | In-process state, lost after a Server restart |
| Web UI local storage | Browser `localStorage` | JWT, remote-control preferences, theme/site name/refresh interval and other UI-local preferences | Browser-side state, not a system source of truth |

The current sources of truth are not single. Inventory is declarative asset input, SQLite is the API and UI runtime query plane, and Agent heartbeats update online status and system metrics. It is necessary to distinguish "assets that should exist" from "a node currently online / current metric values".

## Code Module Boundaries

| Module | Main responsibilities |
| --- | --- |
| `internal/server` | HTTP routing, Agent WebSocket, API aggregation, remote sessions, proxy, alert loop, metric cleanup |
| `internal/agent` | Agent lifecycle, capability detection, heartbeat, message loop, collectors, remote proxy execution |
| `internal/protocol` | WebSocket message structures between Server and Agent, message types and decode helpers |
| `internal/storage` | GORM/SQLite models, migrations, resource CRUD, users, auditing, alerts, metrics, proxy configuration |
| `internal/inventory` | Inventory YAML schema, parsing, validation, syncing to storage |
| `internal/sync` | Runtime inventory file watching by the Server; reuses `internal/inventory.Syncer` to sync declarative resources to storage |
| `internal/auth` | Login, JWT, middleware, TOTP, password reset |
| `internal/proxy` | Server-side proxy port listening; Agent-side connecting to targets and forwarding data |
| `internal/pve` / `internal/docker` / `internal/openwrt` | Third-party platform clients |
| `web/src/services` | Frontend API client; communicates with the Server uniformly through `/api` |

Cross-module calls should respect these boundaries. For instance, an API handler should not parse inventory files directly; it should query runtime results through storage. The Agent should not write to the Server database directly; it should have the Server persist things through protocol messages.

## Control Flow

### Server Startup

1. `cmd/cockpit` reads `-config`, or tries in turn `./config/cockpit.yaml`, `./cockpit.yaml`, `/etc/cockpit/config.yaml`.
2. The Server opens SQLite and migrates automatically.
3. Initializes JWT, email configuration, notification clients, auditing, the proxy manager, the remote session manager, and the ticket manager.
4. The admin password must be initialized through `ADMIN_PASSWORD`; production mode additionally validates `TOTP_ENCRYPTION_KEY`.
5. Registers the HTTP/API/WebSocket routes and starts listening.

### Agent Registration and Heartbeat

1. `cockpit-agent start -server ws://host:9000/ws` connects to the Server; the compatibility entry `cockpit agent -server ws://host:9000/ws` can also be used.
2. The Agent runs capability detectors and forms a capability list.
3. The Agent's first message must be `register`.
4. The Server validates duplicate connections and a configured Agent secret; once accepted it writes to SQLite and adds it to the in-memory Registry.
5. The Agent sends a `heartbeat` every 30 seconds, which may include system information; the Server writes historical metrics and snapshots.

### Inventory Sync

`cockpit sync` is the current complete sync path. It parses `version: v1` YAML and syncs the following resources:

- Regions / Zones / Agents
- Domains / Certificates
- ComputeInstances
- Services
- Gateways
- Storages

Server `inventory.watch: true` watches a single inventory file and hot-reloads; the current implementation reuses `inventory.Syncer`, going through the same resource sync logic as `cockpit sync`. `inventory.strict: true` treats a missing inventory path, parse failure, or initial sync failure as a startup error; with the default `false` it only logs and continues starting the Server.

### HTTP API and Web UI

The Web UI requests `/api` by default. Login, token refresh, TOTP verification, password reset, and `/health` are public endpoints. `POST /api/stacks/webhook` is also outside JWT middleware and authenticates with `X-Cockpit-Token`; other `/api/` resource endpoints use JWT middleware.

Main API surfaces:

- `/api/status`
- `/api/me`, `/api/me/profile`, `/api/me/password`
- `/api/settings`
- `/api/agents`, `/api/agents/{id}/secret`
- `/api/resources/{compute-instances|domains|certificates|services|gateways|storages}`
- `/api/users` (admin manages users)
- `/api/alerts` (alert list and read markers)
- `/api/metrics/*`
- `/api/proxies`
- `/api/remote/*` (tickets / sessions / terminal / guacamole / vault)
- `/api/admin/audit/*`
- `/api/auth/totp/*` (generate / enable / disable require JWT; verify is public)

### Remote Access and Proxy

Remote terminal and desktop connections (Guacamole tunnels) use short-lived tickets:

1. The browser first carries its JWT in a `POST /api/remote/tickets` request.
2. The Server generates a 5-minute-valid, single-use ticket.
3. The browser connects over WebSocket: terminal sends the ticket in `Sec-WebSocket-Protocol`; Guacamole prefers the `?ticket=` URL query and falls back to `Sec-WebSocket-Protocol` when the query is absent.
4. After validating the ticket, the Server converts the connection request into a `proxy_*` message to the corresponding Agent (terminal connects directly to the target; desktop is relayed via `guac:` to guacd).
5. The Agent connects to the target service and forwards data back to the browser through the Server.

This boundary ensures the browser needs neither to know the intranet topology nor to connect directly to Agents.
Remote targets by default only permit hostnames, IPs, or CIDRs explicitly configured in `remote_control.allowed_targets`.

## Protocol Boundaries

Server–Agent messages are unified as:

```json
{
  "id": "message-id",
  "type": "heartbeat",
  "timestamp": 1710000000,
  "payload": {}
}
```

Current message types:

| Direction | Type |
| --- | --- |
| Agent -> Server | `register`, `rpc_response`, `proxy_close`, `proxy_error`, `probe_report`, `exec_log` |
| Server -> Agent | `rpc_request`, `ping`, `proxy_new` |
| Bidirectional | `heartbeat` (reported by the Agent, ACKed by the Server), `error`, `proxy_data` |

`register` must be the Agent's first message after establishing `/ws`. RPC correlates responses using the request message ID. Proxy data correlates specific connections using `proxyId` and `connId`.

## Security Boundaries

- The Server admin password must be provided via `ADMIN_PASSWORD`, at least 8 characters long.
- Production should set `PRODUCTION=true` and a strong random `TOTP_ENCRYPTION_KEY`.
- `ALLOWED_ORIGINS` controls the WebSocket Origin; when unset, all origins are accepted, suitable only for development.
- The Web UI static directory is specified by `STATIC_DIR` or `server.static_dir`; when unset, the root path returns only an API runtime notice.
- Since 2026-10-18, Agent registration is fail-closed by default: an unknown Agent must present `server.agent_registration.enrollment_secret` (or use explicit development-only `allow_unknown`); its presented secret is then stored as the per-Agent authentication key. Existing Agents without a secret are rejected by default and require explicit `allow_secretless`; Agents with a configured secret must present the matching key on reconnect.
- Remote connection tickets are in-memory, 5-minute valid, single-use; they expire after a Server restart.

## Current Implementation Boundaries

- The directory-split model for inventory is not yet the main implementation path; the current CLI examples and parser are based on single-file `version: v1` YAML.
- The SystemProvider in `internal/agent/rpc` is always registered; the Docker/PVE/OpenWrt Providers are conditionally registered by `Agent.setupProviders` based on detected capabilities and environment variables (such as `DOCKER_HOST`, `PVE_TOKEN_ID`, `OPENWRT_HOST`); when credentials are missing it only logs and skips, without affecting the base heartbeat.
- `/api/remote/sessions` is an in-memory session ledger (pending/connected/closed/failed), wired into the terminal / guacamole live session lifecycle (D-2026-10-12-4); it is not full connection configuration persistence, and the ledger clears on restart.
- Some preference settings in the Web UI are stored locally in the browser and do not enter the Server database; `/api/settings` currently serves only as a compatibility entry point, with frontend local preferences deferring to the browser-stored values.
- `docs/archive` (formerly `docs/superpowers`) keeps historical design drafts and implementation plans for TOTP, the notification module, and others, for tracing only and not representing the current running architecture; see [`docs/archive/README.md`](../archive/README.md).
