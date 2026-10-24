# WebSocket Authentication Boundary


The current implementation already avoids carrying a JWT directly in the remote-connection WebSocket URL. Remote terminal, VNC and desktop connections all use a short-lived ticket: the terminal entry point carries it only through `Sec-WebSocket-Protocol`, while the guacamole entry point uses both the URL query `?ticket=` and the subprotocol as a two-channel fallback (query first, subprotocol as fallback).

## Current Flow

1. The Web UI calls `POST /api/remote/tickets` using a JWT.
2. The Server creates an in-memory ticket:
   - Valid for 5 minutes.
   - Single-use.
   - Bound to the user and the connection parameters.
3. The Web UI opens the WebSocket connection and passes the ticket as a subprotocol value.
4. The Server validates and consumes the ticket, and only then upgrades the connection and forwards data.

```typescript
const ticket = await createRemoteTicket(params)
const ws = new WebSocket('/api/remote/terminal', [ticket])
```

The corresponding server-side entry points:

- `/api/remote/terminal`
- `/api/remote/guacamole`

The terminal entry point reads the ticket from `Sec-WebSocket-Protocol`; the guacamole entry point reads the URL query `?ticket=` first and falls back to the subprotocol when it is absent. Neither puts the authentication token into the generic audit trail.

## Design Boundaries

- JWT is used only for the HTTP API that creates tickets; it never enters the remote WebSocket URL directly.
- Tickets live in Server memory and are invalidated when the Server restarts.
- A ticket is marked consumed immediately after successful validation and cannot be reused.
- The ticket parameters may include connection parameters such as `agent_id`, `host`, `port`, `protocol`, `username`, `password`, `domain`, `width` and `height`.

## Security Notes

Production deployments also need the following configuration:

- Set `ALLOWED_ORIGINS` to restrict WebSocket origins.
- Terminate TLS with HTTPS/WSS.
- Avoid printing tickets or remote-connection passwords in logs.
- Keep audit records for remote-connection and proxy-creation operations.

## Historical Note

Earlier documentation discussed URL query tokens, short-lived connection tokens and Cookie authentication. The current code uses the combination "short-lived ticket + WebSocket subprotocol"; that is the implementation baseline this document should follow.
