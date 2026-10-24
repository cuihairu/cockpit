# Protocol and API Boundaries


This document records the communication boundaries between the current Server, Agent, and Web UI.

## Communication Surfaces

| Surface | Initiator | Entry | Auth |
| --- | --- | --- | --- |
| Web UI HTTP API | Browser | `/api/*` | JWT Bearer; login/password reset are public, Stacks webhook uses `X-Cockpit-Token` |
| Agent control channel | Agent | `/ws` | First `register` message; unknown Agents need enrollment secret, reconnects need per-Agent secret |
| Remote terminal | Browser | `/api/remote/terminal` | Short-lived ticket, passed via `Sec-WebSocket-Protocol` |
| Guacamole desktop (RDP/VNC/SSH) | Browser | `/api/remote/guacamole` | Short-lived ticket, passed via `?ticket=` (falls back to `Sec-WebSocket-Protocol`) |

## Agent WebSocket

The Agent actively connects to the Server over WebSocket:

```text
cockpit-agent start -server ws://server:9000/ws
```

The compatible entry `cockpit agent -server ws://server:9000/ws` uses the same startup parameters.

After the connection is established, the first message must be `register`. If the first message is not a registration message, the Server closes the connection.

Message envelope:

```json
{
  "id": "msg_...",
  "type": "register",
  "timestamp": 1710000000,
  "payload": {}
}
```

### register

Direction: Agent -> Server

```json
{
  "type": "register",
  "payload": {
    "agentId": "agent-home-server01",
    "secret": "<enrollment-secret-or-agent-secret>",
    "location": {
      "region": "home",
      "zone": "datacenter"
    },
    "hostname": "server01",
    "capabilities": [
      {
        "type": "docker-api",
        "endpoint": "unix:///var/run/docker.sock"
      }
    ],
    "virtualization": {
      "type": "kvm",
      "role": "guest"
    },
    "labels": {
      "env": "home"
    }
  }
}
```

Registration is fail-closed by default: an unknown Agent's `secret` must match the
Server's `server.agent_registration.enrollment_secret`. After registration, that value
becomes the Agent's authentication secret and is required on reconnect. Anonymous
registration is only available when `allow_unknown` is explicitly enabled for development.

After the Server accepts, it returns the same message type:

```json
{
  "type": "register",
  "payload": {
    "status": "accepted",
    "serverTime": 1710000000,
    "heartbeatInterval": 30
  }
}
```

Registration constraints:

- When `agentId` is empty, the Server can generate an ID, but the Agent currently also generates a default ID based on the hostname.
- If the database already has that Agent and a secret is configured, registration must provide the matching secret.
- When the registry already has an active connection with the same ID, the new connection is rejected.

### heartbeat

Direction: Agent -> Server

The Agent reports a heartbeat every 30 seconds. The heartbeat can carry system information, and the Server writes it into system metrics history and snapshots.

```json
{
  "type": "heartbeat",
  "payload": {
    "agentId": "agent-home-server01",
    "status": "online",
    "systemInfo": {
      "hostname": "server01",
      "cpuUsage": 12.3,
      "memUsagePercent": 48.1,
      "diskUsagePercent": 61.5,
      "osName": "linux",
      "arch": "amd64"
    }
  }
}
```

The Server returns a `heartbeat` ACK, reusing the request message ID.

### rpc_request / rpc_response

Direction: Server -> Agent -> Server

RPC payloads use `method` and `params`. Method names are resolved by `<provider>.<action>`; without a provider prefix, the default provider is `system`.

```json
{
  "type": "rpc_request",
  "payload": {
    "method": "system.status",
    "params": {}
  }
}
```

The response must reuse the request ID:

```json
{
  "type": "rpc_response",
  "payload": {
    "status": "success",
    "data": {}
  }
}
```

Note: SystemProvider is always registered at Agent startup; the PVE, Docker, and OpenWrt providers are conditionally registered by `Agent.setupProviders` based on detected capabilities and environment variables (such as `PVE_URL`/`PVE_TOKEN_ID`/`PVE_TOKEN_SECRET`, `DOCKER_HOST`, `OPENWRT_HOST`/`OPENWRT_USER`/`OPENWRT_PASS`). When credentials are missing, that provider is skipped with a log only, without affecting the other providers or the basic heartbeat.

## Proxy Messages

Proxy messages let the Server reach the Agent itself or TCP targets reachable from its network through an online Agent.

> Note: the proxy supports TCP forwarding only; a `proxyType` param other than `tcp` is rejected with 400
> (`udp` was once accepted while the data plane still dialed TCP — a silent misroute; it now fails
> honestly, see D-2026-10-08-2).

| Type | Direction | Purpose |
| --- | --- | --- |
| `proxy_new` | Server -> Agent | Ask the Agent to establish a connection to the target |
| `proxy_data` | Bidirectional | Forward connection data |
| `proxy_close` | Bidirectional | Close the connection |
| `proxy_error` | Agent -> Server | Report proxy errors |

Key fields:

- `proxyId`: proxy config or session ID.
- `connId`: per-connection ID.
- `target`: the `host:port` the Agent should connect to.
- `data`: data content; when JSON-encoded, possibly a base64 string or byte array.

## Remote Connection Ticket

The browser does not put the JWT directly into the remote WebSocket URL. The flow is:

1. The browser calls `POST /api/remote/tickets` with the JWT.
2. The Server generates a 5-minute, single-use ticket.
3. The browser opens a WebSocket: terminal sends the ticket as the first `Sec-WebSocket-Protocol` value; Guacamole prefers the `?ticket=` URL query and reads the subprotocol only when the query is absent.
4. The Server validates and consumes the ticket, then starts forwarding.

Creating a ticket:

```http
POST /api/remote/tickets
Authorization: Bearer <jwt>
Content-Type: application/json
```

```json
{
  "agent_id": "agent-home-server01",
  "host": "127.0.0.1",
  "port": 22,
  "protocol": "ssh"
}
```

Response:

```json
{
  "ticket": "random-ticket-id",
  "expires_at": "2026-06-11T12:00:00Z"
}
```

Then connect:

```javascript
new WebSocket("ws://server:9000/api/remote/terminal", [ticket])
```

Remote targets may by default only connect to hostnames, IPs, or CIDRs explicitly configured in `remote_control.allowed_targets`; arbitrary targets are allowed only after setting `remote_control.allow_arbitrary_target: true`. The current implementation does not auto-derive the remote-control allow-list from inventory.

RDP/VNC/SSH desktops go through the Guacamole tunnel (`/api/remote/guacamole`): guacd terminates the protocol, the Agent only relays TCP with a `guac:` prefix, with no special build requirements.

## HTTP API

Public interfaces:

- `GET /health`
- `POST /api/auth/login`
- `POST /api/auth/refresh`
- `POST /api/auth/totp/verify`
- `POST /api/auth/forgot-password`
- `POST /api/auth/reset-password`
- `POST /api/auth/verify-reset-code`

JWT-protected interfaces:

- `GET /api/status`
- `GET /api/me`
- `PUT /api/me/profile`
- `POST|PUT /api/me/password`
- `GET|PUT /api/settings`
- `GET /api/agents`
- `GET /api/agents/{id}`
- `GET|POST|DELETE /api/agents/{id}/secret`
- `GET /api/resources/{type}`
- `GET /api/resources/{type}/{id}`
- `GET|POST /api/users` (creation is admin-only)
- `PUT|DELETE /api/users/{id}`
- `POST /api/users/{id}/password`
- `GET /api/alerts`
- `PUT /api/alerts/read-all`
- `GET|PUT|DELETE /api/alerts/{id}` (`PUT /api/alerts/{id}/read` marks as read)
- `GET /api/metrics/snapshots`
- `GET /api/metrics/snapshot?agent_id=...`
- `GET /api/metrics/history?agent_id=...`
- `GET|POST|PUT|PATCH|DELETE /api/proxies`
- `GET /api/proxies/status`
- `POST /api/remote/tickets`
- `GET|POST /api/remote/sessions`
- `GET|DELETE /api/remote/sessions/{id}`
- `POST /api/remote/vault/verify` (password/TOTP second-factor verification; issues a 10-minute vault token)
- `GET|PUT /api/remote/vault/credentials` (credential metadata list / encrypted persist; plaintext never leaves the store; write operations require `X-Vault-Token`)
- `DELETE /api/remote/vault/credentials/{id}` (same as above)
- `GET|POST /api/jobs` (unified Job ledger, filterable by `status/target/type/workflow_run_id` / create a Job, `agent.exec` only; creation is async: 201 returns pending, dispatched in the background, terminal state polled via the ledger)
- `GET /api/jobs/{id}`
- `POST /api/jobs/{id}/cancel` (cancel a Job: pending cancels unconditionally (W3); running kills the in-flight process via the `job.cancel` RPC then lands as `cancelled` (M2c C6); terminal states always return 409)
- `GET|POST /api/workflows` (workflow definition list / create; linear step chain ≤20 steps, each step is one Job)
- `GET|PUT|DELETE /api/workflows/{id}` (definition detail / update / delete; 409 while an active run is in flight)
- `POST /api/workflows/{id}/run` (creates a run and advances it in the background; 201 returns the run view; re-entering the same definition is 409)
- `GET /api/workflows/{id}/runs` (run ledger for that definition)
- `GET /api/workflow-runs/{rid}` (run detail, including step snapshots)
- `POST /api/workflow-runs/{rid}/cancel` (cancel a run: set to `cancelled` and kill in-flight step Jobs via `job.cancel`; the pending window cancels DB-only (M2c C8); later steps do not advance; 409 if already terminal)
- `GET /api/admin/audit/logs`
- `GET /api/admin/audit/stats`

> This section is a representative contract surface, not a full endpoint inventory: endpoints of each
> feature domain (such as the remote-control credential vault, service management, backup, and log
> search) are governed by the API section of the corresponding design doc and the server's route
> registration.

TOTP setup interfaces (also require JWT, distinct from the public `totp/verify` above):

- `POST /api/auth/totp/generate`
- `POST /api/auth/totp/enable`
- `POST /api/auth/totp/disable`

Currently supported resource types:

- `compute-instances`
- `domains`
- `certificates`
- `services`
- `gateways`
- `storages`
