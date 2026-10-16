# External Remote Session Creation API Contract (memex integration, 2026-10-15)

> Language: [中文](./external-session-api.md) ｜ **English**

> Status: **Contract design, not implemented** (this file lands first; implementation follows the memex integration schedule).
> The three remote-access protocols (RDP/VNC/SSH via Guacamole, `telnet` via the
> agent channel) have all landed: `docs/remote-access-integration-design.md` D1-D8.
> memex handles group server authorization (who, authorized by whom, which machine,
> which permission) and needs to invoke a session actually initiated by this repo —
> this contract defines that entry point.

## Responsibility boundary (settled before discussion)

| Item | Owner |
|------|------|
| Authorization relationship storage (who can use which machine, with what permission) | **memex** (not in this repo, see E7) |
| Credential custody (passwords/keys/TOTP) | **persona** (this repo's vault consumes it read-only via `docs/guide/persona-vault-mapping.md`) |
| Credential delivery | **Not done** (credentials never traverse memex, see E2/E8) |
| Session initiation, tickets, egress policy, recording, audit | **This repo** (all reuse existing chains, zero new infrastructure) |

## Decisions

### E1 Placement: a standalone file, not merged into remote-access-integration-design.md

That file is the **review record of landed implementation** (D1-D8 all carry real acceptance runs), while the external interface is a **new design that is not implemented** — mixing the two states makes it impossible to tell ghost features from real ones when reviewing code against the docs. A standalone file + status header (not implemented) keeps the framing clean; once the implementation lands, strike the status header here and link back.

## Contract

### E2 Create session: `POST /api/external/remote/sessions`

**Request body (explicitly contains no credential fields)**:

```json
{
  "agent_id": "office-agent",
  "host": "db.internal",
  "port": 22,
  "protocol": "ssh",
  "credential_ref": "remote/memex/db.internal",
  "requested_by": "memex-user-17",
  "reason": "ops ticket #1042"
}
```

- `protocol` ∈ `ssh | rdp | vnc | telnet` (identical to the `handleRemoteSessionCreate` whitelist, `internal/server/api_remote.go:654-660`).
- **No `username`/`password`/`private_key`/`domain` fields** — credentials are named by `credential_ref` pointing at a vault entry (a local provider path or a persona entry UUID) and are injected server-side when the session is established (E8). memex never touches credentials.
- `requested_by` is the memex-side end-user identifier, used only for audit attribution and never as the authentication principal (the principal is E3's caller token).

**Returns 201**:

```json
{
  "session_id": "…",
  "protocol": "ssh",
  "channel": "guacamole",
  "status": "pending",
  "expires_at": "2026-10-15T12:05:00Z",
  "connect": {
    "ticket": "…",
    "ws_url": "wss://<server>/api/remote/guacamole?ticket=…",
    "ticket_ttl_seconds": 300
  }
}
```

- `ticket` is the existing one-time ticket (5 minutes, in-memory, burn-after-use, `internal/server/ticket.go`; the `?ticket=` query is the current input to `handleGuacamoleWebSocket`, `internal/server/api_guacamole.go:308`).
- `channel` selection per E6: `guacamole` (rdp/vnc/ssh) or `terminal` (`telnet` must take this path; when the memex client cannot speak the guacamole protocol, ssh can also downgrade to `terminal`, i.e. the same chain as D6 mobile — `wss://…/api/remote/terminal`, where the ticket is carried over the **WebSocket subprotocol** — `internal/server/api_remote.go:51-54` `protocols[0]` is the ticketID, which differs from the guacamole channel's `?ticket=` query).

**Errors** (fail-closed, all reusing existing semantics):

| Status | Scenario |
|------|------|
| 401 | caller token missing/wrong |
| 400 | field validation failed / protocol not in the whitelist |
| 403 | target not on the `remote_control` whitelist, or the egress policy denies it (`matchRemoteEgress`, `internal/server/api_remote.go:460`) |
| 404 | agent offline / no entry matching `credential_ref` / **caller config empty (the endpoint 404s by default, E3)** |
| 503 | guacd unreachable (guacamole channel only) |

### E3 Caller identity: named service tokens

The `remote_control` config gains a section (`config/cockpit.yaml:85` onward; struct `RemoteControlConfig`, `internal/config/config.go:140-156`):

```yaml
remote_control:
  external_callers:
    - name: memex
      token: "${COCKPIT_EXTERNAL_TOKEN_MEMEX}"   # secret not in yaml, injected via env
      allowed_protocols: ["ssh", "rdp"]          # optional, default all protocols
      allowed_agent_ids: ["office-agent"]        # optional, default unrestricted
```

- The auth header follows the webhook precedent: `X-Cockpit-Token` (constant-time comparison; precedent `internal/config/config.go:32-35`, `stacks.webhook_token`, decision D-2026-10-12-6). **`external_callers` unconfigured = endpoint 404, off by default** — the same red line as webhooks.
- Human web sessions continue to use JWT; service-to-service calls do not use a user JWT (no impersonating a person, no sharing session state). The caller's `name` plus `requested_by` together form the audit attribution.
- Caller-level `allowed_protocols`/`allowed_agent_ids` are **coarse guardrails** over memex's authorization relationship; they do not replace memex's fine-grained authorization (E7).

### E4 Lifecycle: query and terminate

- `GET /api/external/remote/sessions` — returns only sessions started by **this caller** (the ledger is filtered by caller name; the ledger base is `RemoteSessionManager`, `internal/server/api_remote.go:596-604` is the full view, the external entry adds filtering).
- `GET /api/external/remote/sessions/{id}` — single session status (pending/connected/closed/failed; ledger registration framing see batch nine D-2026-10-12-4).
- `DELETE /api/external/remote/sessions/{id}` — terminate: on hitting a live session, call `terminateLiveSession` (`internal/server/api_remote.go:675`) first and then close out the ledger, same as the existing manual DELETE semantics. **It can only delete sessions it started itself**; someone else's session returns 404 (existence is not disclosed).
- The 5-minute one-time ticket is unchanged; a session is not terminated because memex disconnects (the connection belongs to the memex client, the session belongs to the ledger).

### E5 Conclusion on audit and recording coverage

External callers reuse the existing chain, so **coverage holds with zero new audit mechanisms**; the attribution field is the only addition:

| Mechanism | Coverage | Evidence |
|------|------|------|
| Audit | `auditRemoteStart/End/Failure` identical across protocols; external sessions record `Username` as `memex:<requested_by>` (caller name prefix), credentials stay out of the details | `docs/remote-access-integration-design.md` D5; each failure path from `api_remote.go:423` |
| Egress policy | `matchRemoteEgress` anchored on agent_id; external calls and human operations share one policy (403 fail-closed) | `api_remote.go:460-461` |
| Recording | guacamole channel → guacd `.guac` (`collectGuacRecording`); terminal channel → asciinema character stream | `docs/guide/recording-design.md` D1; `remote-access-integration-design.md` D5 |
| Session ledger | Register/UpdateStatus/closed close out over the full chain (batch nine) | the existing POST creation from `internal/server/api_remote.go:642` |

Gaps (to be filled at implementation time, already covered by this contract): ① the `memex:` audit attribution prefix must be written into the ledger at session creation (`RemoteSessionRequest` gains an `initiator` field); ② caller-level protocol/agent guardrails must run before egress in the E2 validation chain.

### E6 Protocol channel selection (including mobile-chain differences)

- `rdp/vnc` → `channel: guacamole` (pixel stream; the memex client needs the guacamole protocol or embeds the web frontend directly).
- `ssh` → `channel: guacamole` (default, unified recording stack, D1/D3); when the request carries `"channel_override": "terminal"` it goes through the agent channel character stream — identical to D6 mobile (`/api/remote/terminal` + xterm stream). The absence of a guacamole client on the Flutter side is why D6 keeps a separate chain, and the same reasoning applies to the memex client.
- `telnet` → always `terminal` (D1: telnet does not enter guacd).
- The contract does not carry terminal content: once memex has the `ticket` + `ws_url` its client connects to the WS directly, and this repo neither forwards nor multiplexes the data stream to memex.

### E7 Boundaries of what is not done

1. **No credential delivery**: no field of this contract accepts or returns credentials; `credential_ref` only names an entry, and at runtime the server injects the ticket (`use_saved` semantics, `internal/server/api_remote.go:484` onward). The memex side never obtains a password/key.
2. **No authorization relationship storage**: who can use which machine with what permission is memex + persona's job; this repo only enforces its own two guardrails (the `remote_control` target whitelist/egress + E3 caller guardrails) and does not maintain memex's authorization matrix.
3. No session proxying/multiplexing, no new protocols, and no memex-side UI embedding specification (the ticket is the embedding gate, and its 5-minute one-time nature is the security boundary).

## Implementation anchors (for later scheduling, all existing components)

The `handleTicketCreate` request shape (`api_remote.go:399-411`), the egress check and vault injection order (`:460-500`: egress first, `use_saved` filled after), the session ledger (`:642-669`), the ticket (`internal/server/ticket.go`), the guacamole WS (`api_guacamole.go:306-308`), and the terminal WS (`api_remote.go:387`). New code is expected only in: the external route + caller auth middleware + `credential_ref` resolution + the ledger `initiator` field.