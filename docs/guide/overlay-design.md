# P1 Design: Overlay Network Observability (ZeroTier / Tailscale / WireGuard / frp)


> 2026-09-17. First-phase design for cross-region overlay networking observability of personal infrastructure.
> Background: personal infrastructure is naturally scattered across the cloud, home, the office, and other
> regions; ZeroTier/Tailscale/WireGuard/frp are the lifeline that stitches them into a single network. This
> document only makes M1 (runtime read-only observability) design decisions and interface definitions; the
> management plane (controller API member authorization) is left to M2.

## Current State and Pain Points

| Pain point | Current state | Consequence |
|------|------|------|
| Nowhere to query overlay runtime state | Peer going offline, latency degradation, and stalled handshakes all require SSHing into each box and typing commands | Troubleshooting "the device at home is unreachable" is guesswork |
| Cockpit already probes, but far too shallow | `NetworkDetector` (`network-monitor`) only goes as far as WireGuard peer count and cloudflared at the process level | Virtual IP, latency, version, and last handshake are invisible |
| No unified view across regions | Every device has its own `zerotier-cli` / `tailscale status` | No way to compare "which region's nodes are offline" on one screen |
| The connection path carries no semantics | The Server only knows which IP an Agent came from, not its overlay identity | Distributed probing/troubleshooting cannot be explained per path |

## Architecture Overview

The same channel model as cron / logs: **synchronous RPC + pure pass-through on the server**. State for all
four tool families comes from the local CLI / local admin API (millisecond to sub-hundred-millisecond), so a
30s RPC timeout is more than enough.

```
┌─ server ───────────────────────────────┐         ┌─ agent (hosts overlay tools) ──────────┐
│ GET /api/agents/{id}/overlay/          │ ──RPC─▶ │ overlay provider (new)                 │
│   status  pass-through, no DB write    │ ◀─RPC── │ zerotier-cli -j / tailscale --json     │
│ Web /network aggregates across agents  │         │ wg show dump (strip keys) / frp admin  │
└────────────────────────────────────────┘         └────────────────────────────────────────┘
```

The cross-region view is realized by **frontend aggregation**: in a personal setup the number of agents
carrying the `overlay` capability is in single digits, so the page fetches each agent in turn and aggregates
in the frontend into a whole-network node table; the server persists nothing and keeps no aggregated state.

## Key Decisions

| # | Decision | Choice | Rationale |
|---|------|------|------|
| D1 | M1 tool scope | Read-only observability of all four families: ZeroTier, Tailscale, self-hosted WireGuard, frp | The user confirmed doing all of them; the four families cover the overwhelming majority of personal overlay shapes (centralized virtual networks ×2, self-hosted tunnels, port-mapping traversal) |
| D2 | Management plane | Not in M1 (member authorization and removal via the ZeroTier Central / Tailscale APIs are left to M2) | The management plane introduces API token configuration and a higher risk of misoperation; observability comes first to validate the data model |
| D3 | capability | Add a new `overlay`, coexisting with the existing `network-monitor` | The semantics of `network-monitor` are "monitoring feature"; leaving it alone avoids regressions, and a later version converges its WireGuard part into `overlay` |
| D4 | Detection condition | Any one of `zerotier-cli` / `tailscale` / `wg` / `frpc\|frps` passes LookPath → the `overlay` capability; each reader inside the provider then self-checks, and an absent tool returns `unavailable` | The gate lives in the detector, the granularity in the readers; a machine with two tools installed still registers only one provider |
| D5 | RPC method | A single method `overlay.status` returns snapshots of all tools at once | All four are read-only snapshots; in personal setups peers number from single digits to a few dozen, so one round trip saves the frontend many; total-size guard: peers truncated to 200 per tool |
| D6 | **Key security** | Output fields **constructed by allow-list**: the private key / preshared key fields of `wg show dump` never enter the response; ZeroTier/Tailscale/frp output naturally contains no keys | The wg dump text contains private keys and preshared keys, and an allow-list is more reliable than blacklisting them out |
| D7 | Tiered frp observability | Zero-config: LookPath + version + process liveness; once `COCKPIT_FRPC_ADMIN` / `COCKPIT_FRPS_ADMIN` (`host:port`) is configured, call the admin API for tunnel-level state — the two sides use different routes (verified on real frp 0.61.2): frpc `GET /api/status` (tunnel table counts), frps `GET /api/serverinfo` (sum of `proxyTypeCount`; frps has no `/api/status`, calling it 404s) | frp has no stateful CLI; the admin port is optional configuration, and observability cannot demand that users reconfigure frp |
| D8 | REST mounting | `GET /api/agents/{id}/overlay/status`, the serveAPI `/agents/` branch, JWT; browse-type requests are **not audited** (same criterion as logs/cron browsing) | Audit is only introduced once M2 gains mutating operations such as authorization/removal |
| D9 | Persistence | Not persisted — the CLI is the source of truth, the server purely forwards | No state-sync problem; the "whole-network view" is aggregated by the frontend fetching each agent in turn |
| D10 | CLI execution | 5s timeout per command (context), argv passed directly without a shell, output truncated at 1MB | tailscale status can reach hundreds of ms on a large network, so 5s is enough as a backstop; same discipline as logs |

## Data Model

```jsonc
// overlay.status response
{
  "tools": [
    {
      "tool": "zerotier",              // zerotier | tailscale | wireguard | frp
      "status": "ok",                  // ok | degraded | error | unavailable
      "version": "1.14.0",
      "error": "",                     // summary when status != ok/unavailable
      "networks": [                    // zerotier / tailscale: networks this host has joined
        { "id": "8056c2e21c", "name": "home", "status": "OK",
          "addresses": ["10.147.20.5"], "type": "private" }
      ],
      "peers": [                       // remote nodes
        { "id": "7f3d0a9b12", "name": "nas", "virtualIps": ["10.147.20.2"],
          "version": "1.14.0", "latencyMs": 23, "online": true,
          "endpoint": "1.2.3.4/9993", "relay": "", "role": "peer",
          "lastHandshake": "2026-09-17T07:00:12Z" }
      ],
      "interfaces": [                  // wireguard: per-interface dimension
        { "name": "wg0", "publicKey": "…=", "listenPort": 51820,
          "peerCount": 2, "peers": [ /* same structure as peers */ ] }
      ]
    }
  ]
}
```

Fields take the available subset per tool (the parser constructs by allow-list, see D6); `lastHandshake`
is uniformly RFC3339 UTC; `online` semantics vary by tool: ZeroTier uses the presence of an active path,
Tailscale uses `Online`, WireGuard uses last handshake < 3 minutes, frp uses admin API reachability.

## Agent-Side Design

### Detection and Registration

- `internal/agent/detector/overlay.go`: `OverlayDetector`, priority adjacent to NetworkDetector;
  any one of the four available → the `overlay` capability (metadata carries each availability flag);
- providers.go: `case "overlay"` registers `rpc.NewOverlayProvider(nil)` (readers are injected with
  the same `Commander` abstraction, so tests can inject fake output).

### RPC Method

| Method | Parameters | Returns | Description |
|------|------|------|------|
| `overlay.status` | `{}` | `{tools: [...]}` | Snapshots of all tools; a failure in one tool does not affect the rest (each degrades to error/unavailable on its own) |

### Per-Tool Parsing Notes

| Tool | Data source | Parsing | Security |
|------|--------|------|------|
| ZeroTier | `zerotier-cli -j listnetworks` / `-j listpeers` | JSON; from a peer's `paths[]`, take the entry that is both `active` and `preferred` as the endpoint | No key fields |
| Tailscale | `tailscale status --json --peers` (older versions that do not accept `--peers` fall back to `status --json`) | JSON; `Self` + `Peer{}`; `CurAddr` → endpoint, `Relay` → relay | No key fields |
| WireGuard | `wg show all dump` | Row-oriented table; the interface row supplies listen port / pubkey, the peer rows supply endpoint / allowed-ips / latest-handshake / transfer | **The private key field on the interface row and the preshared key field on peer rows are discarded** (allow-listed fields produce a separate output) |
| frp | `frpc -v` / `frps -v`, `pgrep`, admin API (D7: frpc `/api/status`, frps `/api/serverinfo`) | JSON; frps aggregates proxyTypeCount, frpc takes the tunnel table | The admin API response contains no token |

### Validation and Guards

- Entirely read-only, no write-back path, no symlink/path risk;
- 5s CLI timeout (D10); tool output truncated beyond 1MB;
- peers > 200 are truncated, with a truncation note appended to the tool-level `error`.

## Server-Side Design

### REST API (server/api_overlay.go, mounted on the serveAPI `/agents/` branch, JWT)

| Method | Path | Description |
|------|------|------|
| GET | `/api/agents/{id}/overlay/status` | Pure pass-through of `overlay.status`; 503 when the agent is offline; browse-type requests are not audited |

## Web UI Design

Add a `/network` "Overlay" page with a two-tier view:

- **Whole-network overview** (the cross-region view, the core value of the page): fetch the status of each
  online agent carrying the `overlay` capability one by one and aggregate in the frontend into a node
  table — one row = one remote node; columns = node name, owning agent, tool, virtual IP, latency, online
  state, version, last handshake; when the same node appears on multiple paths, merge by lowest latency and
  show a source badge;
- **Grouped by agent**: one collapsible panel per agent, holding tool cards (status/version/local networks)
  plus a peers detail table; offline agents show their last known state, labeled as such.

Components follow the cron/proxy page pattern: the agent selector filters by capability; the polling
interval follows the global refreshInterval setting.

## Not Doing (Later Items)

- The management plane (ZeroTier Central / Tailscale API member authorization, CMDB reconciliation to
  discover unmanaged devices) → M2;
- Agents reporting their own overlay identity to Register/heartbeat (connection-path semantics, per-path
  routing selection) → a separate design;
- P2P direct-connection optimization (when server and agent are on the same network, RPC does not relay
  through the server) → long term;
- Converging the legacy `network-monitor` capability → done (see D18, the capability has been deleted).

## M1 Checklist

1. `internal/agent/detector/overlay.go` + tests;
2. `internal/agent/rpc/overlay_provider.go` (four readers + parsers + Commander injection) + parser fixture
   tests (including assertions that wg keys are stripped);
3. The registration branch in `internal/agent/providers.go`;
4. `internal/server/api_overlay.go` + pass-through tests;
5. `web/src/pages/Network/` (overview + grouping) + route/menu;
6. `go test ./...`, `pnpm run build`, and `scripts/e2e-smoke.sh` show no regressions.

## References

- ZeroTier CLI JSON: `zerotier-cli -j help`; server-side API (M2) `api.zerotier.com/api/v1`
- Tailscale: `tailscale status --json`, API (M2) `api.tailscale.com`
- WireGuard dump format: `wg(8)`; frp admin API: the frp `Admin Server` documentation

---

# M2 Design: Cloud Management Plane + Identity Reporting + Legacy Capability Convergence

> 2026-09-19. After M1's read-only observability validated the data model and the forwarding chain, M2 fills
> in three pieces: **A. the cloud management plane** (ZeroTier Central / Tailscale API member authorization
> and removal, CMDB reconciliation), **B. agent overlay identity reporting** (carried at Register time, the
> connection-path semantics plus the matching key for CMDB reconciliation), **C. convergence of the legacy
> `network-monitor` capability** (done, see D18). A and B are two sides of the same coin: without B's identity
> reporting, A's "unmanaged device" reconciliation has no anchor.

## Architecture Overview

The management plane goes through the **server connecting straight to the cloud APIs** (the same outbound
path as DNS M1's Cloudflare and ACME); agents are not involved. Observability keeps using M1's existing
agent RPC, unchanged.

```
┌─ server ─────────────────────────────────────┐       ┌─ cloud control plane ───────┐
│ overlay cloud client (new, internal/overlay) │─HTTPS▶│ api.zerotier.com/api/v1     │
│ GET /api/overlay/cloud  list + reconcile     │       │ api.tailscale.com/api/v2    │
│ POST/DELETE mutations (authz/remove) + audit │       └─────────────────────────────┘
└──────────┬───────────────────────────────────┘
           │ Register (existing pipeline)              ┌─ agent ─────────────────────┐
           ▼                                           │ overlay capability metadata │
│ registry (capabilities fully stored)         │       │ + identity (new, M2-B)      │
│ /api/agents output carries metadata → web    │       │ zerotier-cli / tailscale    │
└──────────────────────────────────────────────┘       └─────────────────────────────┘
```

CMDB reconciliation is a **read-only derivation**: the cloud member list × the agent-reported identity,
joined in the frontend, nothing persisted, no aggregated state — the same philosophy as M1's "whole-network
view aggregated in the frontend".

## Key Decisions

| # | Decision | Choice | Rationale |
|---|------|------|------|
| D11 | Management-plane architecture | The server connects straight to the cloud APIs; agents are not involved | Management credentials are account-scoped (one token governs all network members), and relaying them through agents only widens the exposure surface and couples deployment; observability (M1) lives on the agent and management (M2) lives on the server, each closest to its source of truth |
| D12 | Loading cloud credentials | config.yaml `overlay.zerotier.api_token` / `overlay.tailscale.api_token`; the env vars `ZEROTIER_API_TOKEN` / `TAILSCALE_API_TOKEN` take precedence (secrets do not land in yaml, same discipline as DNS); Tailscale additionally has `overlay.tailscale.tailnet` (default `-`; some token shapes require an explicit tailnet name) | An exact replica of the DNS Cloudflare pattern; when unconfigured, a uniform 503 plus guidance naming which key is missing; tokens never enter responses/errors/audit |
| D13 | API surface | Read `GET /api/overlay/cloud` returns both providers in one call (an unconfigured section is `configured:false`, and a single provider failing degrades without turning the whole response into a 5xx); four mutation endpoints: `POST /api/overlay/cloud/zerotier/networks/{nwid}/members/{mid}` (body `{authorized:bool}`), `DELETE .../members/{mid}`, `POST /api/overlay/cloud/tailscale/devices/{devid}/authorize`, `DELETE .../devices/{devid}` | In a personal setup, fetching both providers at once saves a round trip; reads degrade, writes are precise; path organization matches DNS `/api/dns` |
| D14 | Mutation auditing | Authorization/deauthorization records `overlay_authz`, removal records `overlay_remove`; resourceID = the network/device plus the member ID; details record only booleans/names, never tokens | M1's D8 foreshadowed "auditing is introduced for mutating operations", now delivered; naming follows the cron_apply/proxy_apply style |
| D15 | Where identity is reported | The `overlay` capability's `metadata.identity` (the existing Register path, zero new protocol fields); scope is ZeroTier + Tailscale only | Only centralized virtual networks have a cloud management plane, so the CMDB reconciliation semantics only hold for those two (self-hosted WireGuard has no control plane, frp has no notion of members); the metadata map already reaches the registry and the /api/agents output in full at registration, so server/web need no storage changes |
| D16 | Identity extraction implementation | A new file `overlay_identity.go` inside the detector package does lightweight parsing (`zerotier-cli -j info` + `-j listnetworks`, `tailscale status --json`), allow-listed fields, 2s timeout per command, **failures are silently skipped** (an absent identity key ≠ a failed detection and does not block registration) | Do not port the whole parser set from the rpc package (that would force package dependency changes; identity needs very few fields); a detection on the registration path must be allowed to fail (same discipline as ddns/file's "lightweight stdlib implementation"); refresh timing = re-registration on reconnect, not carried by the heartbeat — membership changes rarely, and the authoritative runtime state is fetched live on every overlay.status |
| D17 | CMDB reconciliation | Matching keys: ZeroTier = member id ↔ the agent identity's node id (both 10-hex, naturally the same key); Tailscale = device id ↔ Self.ID; a tri-state annotation `managed` (a matching cockpit agent exists) / `unmanaged` (present in the cloud, absent from the panel), plus a count summary | member/device ids are assigned by the control plane and reported verbatim by the agent-side CLI, so there is no mapping ambiguity; `unmanaged` is the core selling point of this design — discovering unmanaged devices; "the agent reported it but the cloud does not have it" is usually a token-scope problem, so it is folded into the guidance text rather than made a separate state |
| D18 | `network-monitor` convergence | **Delete** NetworkDetector and `GetNetworkInterfaces` from `detector/network.go`: no server/web consumer of the capability (verified by grep), and nothing downstream consumes the metadata | M1's D3 foreshadowed this convergence, now delivered; shipping from the same repo in the same release means no compatibility baggage (precedent: service-design D9 renaming systemd→service); cloudflared tunnel observation is left for later, merged into overlay following frp's tiered observation pattern from D7, not in M2 |
| D19 | ID validation | A single-ended allow-list on the server: ZeroTier network id `^[0-9a-f]{16}$`, member id `^[0-9a-f]{10}$`, Tailscale device id `^[0-9]{1,20}$`; invalid values return 400 directly | The cloud APIs are outbound calls, and without validation the values could be injected into the request path; with no agent relay there is no need for rules on both ends, one end suffices |
| D20 | Cloud client implementation | A new `internal/overlay` package (ZeroTier/Tailscale clients + `NewXxxWithBase` httptest-injection variants), 15s timeout, member/device output constructed by allow-list (token/key-style fields naturally do not exist; raw JSON is defensively not passed through) | Isomorphic to the dns package; the WithBase variant is the fully-httptest test path already validated by DNS M2 |

## Cloud Data Model (Allow-List Construction)

```jsonc
// GET /api/overlay/cloud response
{
  "zerotier": {
    "configured": true,
    "error": "",                    // degraded summary when configured but the fetch fails
    "networks": [                   // Central API /network, read-only listing (management granularity is at the member level)
      { "id": "8056c2e21c", "name": "home", "members": [
        { "id": "7f3d0a9b12", "name": "nas", "authorized": true,
          "online": true, "ips": ["10.147.20.2"], "version": "1.14.0",
          "lastSeen": "2026-09-19T07:00:12Z", "managed": true }
      ]}
    ]
  },
  "tailscale": {
    "configured": false,            // unconfigured → the remaining fields are absent, the frontend renders a guidance card
    "tailnet": "-",
    "error": "",
    "devices": [
      { "id": "1234567890", "name": "nas.tailnet-name.ts.net",
        "addresses": ["100.64.0.2"], "user": "me@example.com",
        "os": "linux", "authorized": true, "online": true,
        "keyExpiry": "2027-01-01T00:00:00Z", "lastSeen": "…", "managed": true }
    ]
  }
}
```

`managed` is computed by the server at fetch time by reconciling against the agents' `metadata.identity` in
the registry (D17 matching keys); the web does zero derivation — the registry is in-process server data, so
the server computing it saves the frontend another /api/agents round trip; if the reconciliation fails (empty
registry) it is always `managed:false`, never a guess.

## Agent Side (M2-B)

### Identity Extraction (detector/overlay_identity.go)

| Tool | Command | Extracted fields (allow-list) |
|------|------|-------------------|
| ZeroTier | `zerotier-cli -j info` | `address` (= node id), `version` |
| ZeroTier | `zerotier-cli -j listnetworks` | Per network: `id`, `name`, `status`, `assignedAddresses` |
| Tailscale | `tailscale status --json` | `Self.ID`, `Self.HostName`, `Self.DNSName`, `Self.Tailscale IPs` |

- 2s timeout per command; if any one fails, that tool's identity is absent, which does not affect the other
  tool or the remaining detection;
- The result is written to the `overlay` capability's `metadata.identity`:
  `{zerotier: {nodeId, networks: [{id,name,status,addresses}]}, tailscale: {id, hostName, dnsName, addresses}}`;
- The web renders an identity chip (network name + local virtual address) in the per-agent grouped view.

### Not Doing

- Carrying identity in the heartbeat (D16: it changes rarely, refreshing on re-registration is enough);
- WireGuard/frp identity (D15: there is no cloud management plane to reconcile against).

## Server Side (M2-A)

### internal/overlay Cloud Client

| Method | ZeroTier Central (api.zerotier.com/api/v1) | Tailscale (api.tailscale.com/api/v2) |
|------|---------------------------------------------|----------------------------------------|
| List | `GET /network` + per-network `GET /network/{id}/member` | `GET /tailnet/{tailnet}/devices` |
| Authorize | `POST /network/{nwid}/member/{mid}` body `{authorized}` | `POST /device/{id}/authorize` |
| Remove | `DELETE /network/{nwid}/member/{mid}` | `DELETE /device/{id}` |

- Bearer token authentication; 15s timeout (same as the DNS client); member/device parsing is constructed
  by allow-list (D20);
- Both clients provide `NewXxxWithBase` variants to support end-to-end httptest testing;
- Network name: for ZT take `config.name` from the `/network` list item (the top-level id is taken the same
  way); the member name comes from `name` (the Central display name), falling back to the first six
  characters of the node id for display.

### REST API (server/api_overlay_cloud.go, JWT)

| Method | Path | Audit |
|------|------|------|
| GET | `/api/overlay/cloud` | No (browse-type, same as M1 D8) |
| POST | `/api/overlay/cloud/zerotier/networks/{nwid}/members/{mid}` | `overlay_authz` (details include the authorized boolean) |
| DELETE | `/api/overlay/cloud/zerotier/networks/{nwid}/members/{mid}` | `overlay_remove` |
| POST | `/api/overlay/cloud/tailscale/devices/{devid}/authorize` | `overlay_authz` |
| DELETE | `/api/overlay/cloud/tailscale/devices/{devid}` | `overlay_remove` |

- When the corresponding provider is unconfigured: reads return 503 with guidance text (naming which key is
  missing, each reporting its own like requireDNS); mutations likewise return 503 and are not forwarded to
  the cloud;
- Cloud 4xx/5xx map the HTTP status as-is and surface the cloud error summary (tokens stripped);
- After a successful mutation, the web partially refreshes `GET /api/overlay/cloud`.

## Web UI (/network Page Extension)

- At the top of the page, a **Segmented "Runtime Observability / Cloud Management"** control (observability
  by default; the M1 view is untouched);
- The **Cloud Management** view:
  - An unconfigured provider gets a guidance card (configuration method + the note that env takes
    precedence, same style as the DNS/acme guidance cards);
  - ZeroTier: a member table grouped by network (member name/ID/authorization Switch/online/virtual
    IP/version/last seen/"managed by the panel" badge + a Tooltip explaining it); row actions: an
    authorization Switch (immediate POST) + a removal Popconfirm (member id = node id; once the device joins
    again it can be re-authorized, so it is reversible enough to deserve only a Popconfirm);
  - Tailscale: a device table (name/addresses/owning user/OS/online/authorization state/keyExpiry expiry
    highlight/managed badge); row actions: an authorize button + a **removal Modal that requires typing the
    device name to confirm** (deleting a device requires logging in again, so it is more destructive than
    ZT; follow the stack's delete-confirmation pattern);
  - A reconciliation summary bar at the top: "n devices in the cloud, m unmanaged" (expanding to list the
    unmanaged ones);
- One addition to the **Runtime Observability** view: the per-agent group cards show an identity chip
  (from the capability metadata, reported by M2-B).

## M2 Checklist

1. `internal/agent/detector/overlay_identity.go` + extraction tests (injecting fake commands, silent
   failure, field allow-list);
2. Delete `detector/network.go` + clean up the related tests in step (M2-C, can be folded into a small
   standalone commit merged into feat(agent));
3. `internal/overlay` (two clients + WithBase) + end-to-end/degradation/allow-list httptest tests;
4. `internal/server/api_overlay_cloud.go` + endpoint tests (D19 validation/audit/unconfigured
   503/degradation/managed reconciliation);
5. The cloud management view in `web/src/pages/Network/` + the identity chip in the observability view;
6. `go test ./...` and `pnpm run build` show no regressions; check off the design-doc checklist + update the
   todo.md entries.

## M2 Implementation Delta Notes

- **Read semantics for an unconfigured provider**: the Server-side REST subsection says "reads return 503
  with guidance text", but the implementation follows D13's per-section semantics — `GET /api/overlay/cloud`
  is always 200, with an unconfigured section carrying `configured:false` and a failed section carrying an
  in-section `error` (the web renders the guidance card based on `configured`); 503 is used only for
  mutation operations (naming which env/yaml key is missing). D13 is the decision layer and the subsection is
  an implementation description; the decision wins and the original text is not rewritten.
- **Cloud error status mapping**: the design says "4xx/5xx map the HTTP status as-is", but the
  implementation uniformly maps **502** + a cloud error summary (truncated at 200 characters, tokens
  stripped), isomorphic to DNS M2's upstream error handling; the per-section `error` field carries the
  original summary text so status detail is not lost — an unreachable upstream is gateway semantics, and
  passing through 401/403 would instead mislead users into thinking it is an authentication problem with
  cockpit.
- **Stronger summary-bar semantics**: when m=0, "n devices in the cloud, m unmanaged" is shown as an
  info-level "all of them match the panel's Agent identities", positively confirming that the reconciliation
  chain is healthy rather than only reporting problems.

## References

- ZeroTier Central API v1: `https://api.zerotier.com/api/v1` (`GET /network`,
  `POST/DELETE /network/{id}/member/{memberId}`, Bearer token)
- Tailscale API v2: `https://api.tailscale.com/api/v2` (`GET /tailnet/{tailnet}/devices`,
  `POST /device/{id}/authorize`, `DELETE /device/{id}`, Bearer token; `tailnet=-` is the shorthand for the
  default tailnet supported by the API's generated documentation, and the implementation follows
  measurements made with a real token)

---

# M3 Design: Standalone Deployment Path — Network Join/Leave + daemon Service Management

> 2026-09-27. M1's observability and M2's cloud management plane have closed off both "looking" and "cloud
> authorization", but **two high-frequency local-side actions still require SSH**: ① "have this machine
> join/leave a network" ② "tailscaled / zerotier-one has crashed / is not installed / should not start at
> boot". M3 brings these two actions into the panel. The user decided on the **standalone deployment path**:
> the overlay daemon is still installed by the user on the host and managed by systemd; the cockpit agent is
> only the caller and the state manager, and never hosts the daemon process (see D21 for details).

## Architecture Overview

The same channel model as M1: **synchronous RPC + pure pass-through on the server + auditing**. The only
difference lies in the protection level of write operations:

```
┌─ server ───────────────────────────────┐          ┌─ agent ────────────────────────────────────┐
│ POST /api/agents/{id}/overlay/         │  ─RPC─▶  │ overlay.join / overlay.leave               │
│   networks/{netId}/join|leave          │  ◀RPC──  │   → zerotier-cli join|leave                │
│ GET  /api/agents/{id}/overlay/daemon   │  ─RPC─▶  │   → tailscale up|down                      │
│ POST /api/agents/{id}/overlay/service  │  ◀RPC─▶  │ overlay.daemon / overlay.service           │
│ RBAC overlay:admin + audit (3 actions) │          │   → systemctl (hard-coded unit allow-list) │
└────────────────────────────────────────┘          │ daemon belongs to systemd, agent only      │
                                                    │ manages, never hosts; argv direct, no shell│
                                                    └────────────────────────────────────────────┘
```

## Key Decisions

| # | Decision | Choice | Rationale |
|---|------|------|------|
| D21 | **Deployment shape: standalone deployment, no embedded tsnet** | The cockpit agent **does not pull in tsnet / a built-in tailscaled**; the overlay daemon is installed by the user on the host and managed by systemd; the agent only executes CLI and systemctl actions. The principle goes on the record: **the daemon belongs to systemd, the agent only manages and never hosts** | Embedding tsnet means the tailscaled lifecycle hangs off the cockpit agent process — any agent restart/upgrade drops the network, and a cockpit crash means losing contact (exactly the scenario where remote troubleshooting needs it alive); personal-infrastructure daemons already autostart year-round, so double hosting only enlarges the two-daemon conflict surface; standalone deployment also keeps ZeroTier viable (tsnet only solves Tailscale) |
| D22 | RPC method surface | Add `overlay.join` / `overlay.leave` (parameters `{tool, networkId}`) plus `overlay.daemon` (read) / `overlay.service` (write, parameters `{tool, action}`); a successful join/leave returns the **latest status snapshot + identity** | Write operations get methods of their own, separate from read-only `overlay.status`, which keeps RBAC/audit routing clean on the server; returning a snapshot on success saves the frontend one fetch (see D26) |
| D23 | ZeroTier join/leave | `zerotier-cli join\|leave <networkId>`, argv passed directly without a shell (D10 discipline), 5s timeout; the networkId is a **16-hex-digit** allow-list validated on **both the server and the agent**; a CLI reply with a `200` prefix counts as success (`101 already joined` also returns a snapshot) | ZeroTier's network ID shape is unique (16-hex), so a check at each end is the minimum injection discipline for a write; the 200/101 reply semantics are based on measured `zerotier-cli(1)` behaviour |
| D24 | Tailscale join/leave semantics (fixed after verifying actual CLI behaviour) | In the cockpit scenario a Tailscale "network" = a tailnet, and **there is no CLI action for joining by network ID**; join = first check `BackendState` from `tailscale status --json`, and only run `tailscale up` when it is `Running` (already logged in) — when not logged in, `up` is an interactive authentication flow (it prints an auth URL and waits for a browser), which is guaranteed to fail non-interactively, so **it is rejected and guidance text is returned**; leave = `tailscale down` (disconnects but keeps the login state, so up can bring it back), and **`logout` is never used** (clearing credentials means the next join needs interactive re-authentication, doubling the destructiveness); networkId passes `-` or a tailnet DNS suffix (e.g. `tailnet-name.ts.net`), compared EqualFold against the suffix derived from `Self.DNSName` with the first hostname label removed — on mismatch → 400 (when the DNSName has fewer than 3 labels no comparison is done; better to skip the comparison than to kill a legitimate request) | The difference in "can re-enter" across the three states `up/down/logout` is the core of this decision: down is reversible, logout is not (from the user's point of view); authkey auto-registration is out of scope — issuing credentials belongs to the install flow, and the panel only serves machines that are already installed |
| D25 | WireGuard / frp | join/leave and service uniformly return `unavailable` with a semantic explanation | WireGuard has no runtime join concept (configuration means `/etc/wireguard/*.conf` + `wg-quick`; adding and removing interfaces belongs to the file-management domain, and the panel does not edit config files); frp is port traversal, not an overlay network (no member/join semantics). Honest errors beat a forced mapping |
| D26 | Post-success state synchronization | The join/leave response carries the **latest `overlay.status` snapshot** (re-fetched locally by the agent) and the refreshed `identity`; after receiving the response the server **updates that agent's `metadata.identity` in place in the registry** — identity is reported through the response payload without touching the WebSocket protocol surface; at all other times the identity refresh timing is still re-registration on reconnect (D16 unchanged) | The WebSocket protocol only accepts **the first** message after connecting as a register (`websocket.go handleWebSocket`), and mid-session re-registration is not supported — the response payload is exactly the existing reverse channel, and low-frequency-but-state-jumping write operations such as join/leave borrow it to report identity, which is enough for CMDB reconciliation immediacy; the authoritative runtime state already lives in the live fetch on every `overlay.status`, and returning a snapshot means the frontend needs no second request |
| D27 | daemon/service management (M3.5) | `overlay.daemon` read: per tool return `{installed, unitExists, active, enabled, version, missingGuide}` — `LookPath` + version for the CLI/daemon, and for the systemd unit, existence via `systemctl cat <unit>` with state taken from `is-active` / `is-enabled`; `overlay.service` write: action ∈ `start\|stop\|enable\|disable`, with a **hard-coded unit name allow-list** `{tailscale→tailscaled.service, zerotier→zerotier-one.service}`, not reusing the generic service provider surface; on non-systemd platforms reads degrade (the unit fields are absent) and writes return unavailable; not installed → `missing` + install guidance text (one line each for apt/brew/the official script) | The allow-list holds only these two units: the overlay page must never become a way to touch arbitrary systemd units (the generic service provider exists for the /services page, whose permission resource differs); under the `the daemon belongs to systemd` principle the agent only issues systemctl commands and never touches process supervision (things like `tailscale up` pulling up netsd) |
| D28 | RBAC | Add an **`admin`** tier to the `overlay` resource (`resourceActions["overlay"] = [read, write, admin]`); all four write endpoints (join/leave/service) require `overlay:admin`; the **built-in operator role excludes `overlay:admin`** (same discipline as `users`/`roles`/`settings:admin`) — an overlay write can kick a host off the network, so it must be granted explicitly; the admin=full-set / operator=excluded-set assertions in `role_test.go` are updated accordingly | The blast radius grows tier by tier from observability (read) to cloud management (write), and the three tiers match exactly; an admin-tier grant affects only join/leave/service, while M2 cloud authorization stays at write |
| D29 | REST API + auditing | `POST /api/agents/{id}/overlay/networks/{netId}/join`, `.../leave` (body `{tool}`), `GET /api/agents/{id}/overlay/daemon`, `POST /api/agents/{id}/overlay/service` (body `{tool, action}`); audit actions `overlay_join` / `overlay_leave` / `service_toggle`, resourceID = `{agentID}/{tool}/{netId}` and `{agentID}/{unit}`; a server-side netId regex allow-list (ZT 16-hex / Tailscale `-` or a full tailnet DNS suffix, with an anchored regex so the alternation cannot degrade into a substring match), invalid → 400; agent offline → 503 | Path organization continues the M1 `/agents/{id}/overlay/` prefix; `GET daemon` is a browse-type request and is not audited (M1 D8 criterion); all three write actions leave audit traces (naming in the same style as the cloud section's `overlay_authz`/`overlay_remove`) |
| D30 | Web UI | In the runtime observability view (M1), on each tool card: **ZeroTier/Tailscale** get a "Join network" button (a Modal with a netId input — the ZT side hints 16-hex, the Tailscale side hints the tailnet name and that being logged in is required) and a "Leave" button (a Popconfirm whose text spells out the consequence: down is reversible); each tool also gets a **daemon status card** (installed/active/enabled/version badges + an install guidance Alert when `missing` + a start/stop/enable/disable button group); all operations use `PermGuard perm="overlay:admin"` | Actions sit right next to the observability cards — the decision inputs (current peers/networks) share the screen with the buttons; the "leave" copy for things like `tailscale down` emphasizes that the login state is kept and you can come back, narrowing the psychological expectation gap that leads to mistakes |

## Agent-Side Design (rpc/overlay_provider.go Extension)

### RPC Method Surface

| Method | Parameters | Returns | Gate |
|------|------|------|------|
| `overlay.join` | `{tool, networkId}` | `{status: <same as overlay.status>, identity: {…}}` | tool allow-list + validation on both ends; 5s |
| `overlay.leave` | `{tool, networkId}` | Same as above | Same as above |
| `overlay.daemon` | `{}` | `{tools: [{tool, installed, unitExists, active, enabled, version, missingGuide}]}` | Read-only |
| `overlay.service` | `{tool, action}` | `{tool, unit, action, result}` (a non-zero systemctl exit code → error) | unit name allow-list (D27); 5s |

### Per-Tool Command Semantics (argv Passed Directly)

| Tool | join | leave | Notes |
|------|------|-------|------|
| ZeroTier | `zerotier-cli join <16hex>` | `zerotier-cli leave <16hex>` | A `200`/`101` reply prefix means success |
| Tailscale | Requires `BackendState==Running` → `tailscale up`; otherwise an error + guidance to "log in on that machine with `tailscale up` first" | `tailscale down` | networkId is only used for auditing and consistency validation (D24) |
| WireGuard / frp | `unavailable` | `unavailable` | D25 |

### daemon Detection Details

| Item | Method | Notes |
|------|------|------|
| installed | CLI `LookPath` (zerotier-cli / tailscale) + daemon present (zerotier-one / tailscaled, same rule) | If either is missing → installed=false |
| version | Take the version string from the `<cli> version` output | Left empty on failure |
| unitExists | `systemctl cat <unit>` exit code | Skipped on non-systemd platforms (the field is absent) |
| active / enabled | `systemctl is-active / is-enabled <unit>` | Same as above |
| missingGuide | When installed=false, give a summary of the install commands | One line each for apt/brew/the official script, static strings |

## Server-Side Design

### REST API (`api_overlay.go` Extension, JWT + RBAC)

| Method | Path | Permission | Audit |
|------|------|------|------|
| GET | `/api/agents/{id}/overlay/daemon` | `overlay:read` | No (browse-type) |
| POST | `/api/agents/{id}/overlay/networks/{netId}/join` | `overlay:admin` | `overlay_join` |
| POST | `/api/agents/{id}/overlay/networks/{netId}/leave` | `overlay:admin` | `overlay_leave` |
| POST | `/api/agents/{id}/overlay/service` | `overlay:admin` | `service_toggle` |

- Successful join/leave: the server returns the new snapshot from the response verbatim (the web needs no
  second fetch);
- `requiredPerms` mounting: the `/overlay/networks/` and `/overlay/service` prefixes require
  `overlay:admin`, and the `/overlay/daemon` prefix requires `overlay:read` — following the
  `acme issue/deploy → acme:admin` special-case pattern (the existing `overlay/` entry in
  agentSubResources stays at write level; the admin prefix is checked first);
- Agent-side errors (the Tailscale not-logged-in guidance, CLI errors) are surfaced verbatim as 502 + a
  summary (the upstream-error criterion from the M2 implementation delta notes).

## Not Doing (Later Items)

- authkey interactive registration / ZeroTier network invite-token issuance (install-flow domain, D24);
- WireGuard interface file management (belongs to the file manager domain, D25);
- Viewing daemon logs (the logs provider already covers journald units);
- Refreshing the registry identity via mid-session re-registration (protocol limitation, D26).

## M3 Checklist

1. agent `overlay.join/leave/daemon/service` RPC + fake Commander injection tests (argv assertions,
   validation on both ends, the Tailscale pre-state, 200/101 replies, rejection of non-allow-listed units);
2. server `api_overlay.go` four endpoints + RBAC `overlay:admin` + three audit actions + endpoint tests
   (forwarding/validation/offline 503/audit/permission matrix);
3. `internal/storage/role.go` `resourceActions["overlay"] += admin`, operator exclusion + `role_test.go`
   assertion updates;
4. `web/src/pages/Network/` join/leave Modal + secondary confirmation + daemon status card (including the
   missing guidance);
5. This document (already landed) + todo.md and the capability matrix backfill;
6. Gates: `go vet`, `go test -race -short ./...`, web vitest + build all green.

### M3 Implementation Delta Notes (2026-09-27)

All six checklist items landed; the implementation phase made three corrections to the design (all written
back into the tables above):

1. **The tailnet name regex is anchored as a whole** (D24/D29): in the draft the alternation in `^-|xxx`
   was not wrapped as a group, so `^` only constrained the first branch and the second degraded into an
   unanchored substring match — any string containing a valid domain-shaped substring could slip past
   validation. Fixed to a fully anchored `^(-|…)$`; the netId criterion is correspondingly defined as `-`
   or a **full tailnet DNS suffix** (compared EqualFold under the same criterion as the extraction result of
   `Self.DNSName` with the first hostname label removed).
2. **daemon version probing is deferred**: when the CLI is not installed (LookPath fails), the
   `--version` probe is skipped instead of firing an execution that is guaranteed to fail against a missing
   binary.
3. The **D26 identity channel** is implemented per the revision above: identity is reported through the
   join/leave response payload, and the server's `Agent.UpdateOverlayIdentity` updates the registry in place
   with copy-on-write.

## References

- `zerotier-cli(1)`: the join/leave reply codes (200/101); `tailscale up|down|logout` semantics: the KB
  article "The tailscale CLI" (kb/1080) — `up` runs an interactive authentication flow when not logged in,
  `down` disconnects while keeping the login state, `logout` clears credentials;
- systemd unit names: the tailscale official install script writes `tailscaled.service`, the ZeroTier
  package writes `zerotier-one.service` (the basis for `systemctl cat` existence detection);
- Lifecycle-ownership principle: D21; the reason for isolating the service surface: D27.
