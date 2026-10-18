# Remote Desktop Design: Apache Guacamole Integration (guacd + guacamole-common-js)

> Language: [中文](./remote-desktop-guacamole-design.md) ｜ **English**

> 2026-09-23 the user approved this project. Previously the remote desktop went down a self-developed route (grdp pure-Go RDP client + self-developed canvas rendering / scancode input / clipboard); this design settles on **integrating Apache Guacamole** as the replacement. This document covers design only; implementation is confirmed separately.
>
> 2026-10-07 update: the Guacamole route has been implemented and landed; the old grdp / noVNC self-developed route has been deleted from the codebase entirely (D-2026-10-07-1), and descriptions of the old route in this document serve only as historical background.

## Pain Point: Why Replace the Self-Developed Route Now

Cockpit's remote desktop status quo is "three protocols, three stacks," and **not one of them is reliable**:

- **RDP** (self-developed grdp route): `internal/agent/rdp/session.go` terminates the RDP protocol with pure-Go `github.com/nakagami/grdp`, bitmap callbacks → `screen_update` dirty rects → self-developed `useCanvasRenderer` framebuffer compositing on the web side. The problem is not "can it run at all" but **protocol coverage breadth**: grdp's support for NLA/CredSSP, TLS/RDP security negotiation, and the RemoteFX/GFX graphics pipeline is far weaker than industrial-grade implementations like FreeRDP/xfreerdp. Connectivity to real Windows Server machines (especially those enforcing NLA) is questionable, and RDP is precisely the protocol where "cannot connect means zero value" — in office scenarios the Windows machines users want to reach enforce NLA by default; this is not an edge case.
- **VNC** (noVNC route): runs the RFB protocol in the browser; the agent only does binary TCP passthrough. This path is the **only one that works**, but it covers VNC only; and noVNC and grdp are two separate input/rendering/clipboard models, so the UI layer cannot converge.
- **SSH** (xterm.js route): just got working (see `internal/proxy/ssh_session.go`, agent side terminates the protocol with `crypto/ssh` + PTY). **SSH is a character terminal, not a graphical desktop**; going with xterm.js is correct and it should not be held hostage by the desktop solution.

The self-developed route also has hidden costs: rendering (framebuffer compositing, dirty rects, ResizeObserver frame merging), input (DOM events → scancodes → RDP virtual keys), clipboard (browser Clipboard API ↔ RDP cliprdr), resolution negotiation — **every one of these is reinventing protocol-detail wheels**, and each protocol added (e.g. SPICE in the future) means copying it all again.

Conclusion: hand desktop protocols (RDP/VNC) to a mature protocol engine, and Cockpit does only what it is good at — sessions, permissions, auditing, dynamic inventory.

## Architecture Decisions

### D1 Use only guacd + guacamole-common-js, **skip guacamole-web's Java layer**

This is the single most core cut of this design, and the reason is **avoiding two identity/permission/audit systems**:

The official Apache Guacamole distribution is three layers: `guacd` (C protocol daemon) + `guacamole-common-js` (browser JS library) + `guacamole-web` (Java servlet container application, including JDBC user/connection management). Most "Guacamole integration" failures fail precisely because the full `guacamole-web` was brought in — it ships its own user table, connection-config table, and permission table, so Cockpit's RBAC (`internal/auth` + `permAll` permission points) and Guacamole's user store become **two sources of truth requiring manual sync**, and auditing splits in half (Cockpit's `auditRemoteStart/End` + Guacamole's own connection history). This directly conflicts with this project's consistent "single source of truth" discipline (inventory-first, DB holds indexes and files hold content).

Looking at each of the three layers' roles separately makes clear which to keep:

- **guacd**: a pure protocol-translation daemon. It **has no concept of users** — give it an instruction like `RDP → host:3389` and it connects and translates RDP into a Guacamole instruction stream. Connection credentials (username/password) are passed in per connection and are not stored in guacd. This is exactly the "protocol engine" boundary we want: it terminates the RDP/VNC protocol for us but never touches our business model.
- **guacamole-common-js**: the browser-side JS library, implementing the Guacamole protocol's `Guacamole.Tunnel` (transport), `Guacamole.Display` (canvas rendering), `Guacamole.Keyboard`/`Guacamole.Mouse` (input), and `Guacamole.Client` (session state machine). It **also contains no user/connection management** — parameters (protocol/host/port/credentials) are supplied by the caller. This layer takes out the three mountains of self-developed rendering/input/clipboard for us.
- **guacamole-web**: Java servlet + JDBC, responsible for "who may connect to which machine." **Cockpit already has this layer**: RBAC permission points (`terminal:write` etc.) + the `matchRemoteEgress` egress policy + dynamic inventory (the agent-reported `remote-services` capability) + one-time tickets (`internal/server/ticket.go`). Bringing it in means bringing in a second system, and its "static connection config" model is the opposite of Cockpit's dynamic inventory philosophy (see the Guacamole downsides note in `reference-projects.md`).

The licensing argument holds the same way: Cockpit is **Apache License 2.0**, and guacd and guacamole-common-js are both Apache 2.0, so same-license integration carries zero contagion risk; guacamole-web is also Apache 2.0 (not an obstacle), but **the reason to skip it is architectural duplication, not licensing**.

### D2 The tunnel goes through a Go WS gateway reverse-proxying guacd; do not expose the guacd port

guacd speaks only the Guacamole protocol and listens on TCP `4822` by default. On the browser side, `guacamole-common-js`'s `Guacamole.Tunnel` has two implementations: `Guacamole.HTTPTunnel` (HTTP long-polling, going through `guacamole-web`'s servlet) and `Guacamole.WebSocketTunnel` (WebSocket, URL pointing at a server-side WS endpoint). **There is no "browser connects directly to guacd" path** — guacd is not an HTTP service, and an intranet port should not be exposed.

So a WS gateway is needed: browser WS ↔ Go backend ↔ guacd TCP. This position is exactly where the Cockpit server already sits (the existing `/api/remote/terminal` and `/api/remote/guacamole` are both "browser WS ↔ server ↔ agent WS / guacd TCP" two-hop pipelines; the server is already the remote-access gateway).

> **Implementation correction (2026-10-18)**: guacd does not dial the user's target directly. Per session, the server establishes egress forwarding through the agent and starts a relay listener bound to the local IP selected by the Server→guacd route; the hostname/port in the `connect` instruction is the **relay address**, and the real target is dialed by the agent from its own network position (`internal/server/guac_relay.go`, `api_guacamole.go`). The `GUACD_ADDR`, `GUAC_RELAY_BIND_ADDR`, `GUAC_RELAY_HOST` env vars are documented in [guacd-deploy](./guacd-deploy.md).
> The skeleton code below (skeleton 2) is kept in its design-time form.

The reasons to put it in Go rather than in guacamole-web are the same as D1: what the gateway does here is more than byte forwarding — there is also **session lifecycle, ticket validation, permission checks, audit persistence** — all capabilities Cockpit already has. guacamole-web's `GuacamoleWebSocketTunnelEndpoint` does the same thing, but with Java sessions + its permission tables, which lands back in two systems.

The concrete shape of the reverse proxy: between the Go backend and guacd **use the native Guacamole TCP protocol** (not guacd's WebSocket mode), because guacd's protocol is already a "instruction stream + base64 chunk" text protocol and TCP forwarding is the most direct; the Go side does bidirectional piping between WS text frames ↔ guacd's TCP stream and **does not parse instruction content** (except that during the handshake the `select`/`size`/`audio`/`video` parameter instructions need to be constructed by Go). This way protocol evolution (new instructions in Guacamole 1.5.x) is aligned between guacamole-common-js and guacd themselves, and the Go gateway does not follow along.

### D3 Reuse the existing ticket and 401 recursive-refresh chain

Existing ticket semantics (`internal/server/ticket.go`): `POST /api/remote/tickets` → one-time ticket, valid for 5 minutes, `ValidateTicket` consumes on use (`Consumed` flag). This was designed as the "WS handshake credential" — invalidated after one handshake, with a 5-minute leak window.

The Guacamole tunnel **reuses the same ticket-issuing endpoint and auditing**; the ticket remains the WS handshake credential (one-time semantics unchanged), and the only difference is that "after a successful handshake the server side holds the guacd TCP connection until the session ends" rather than "forwarding to the agent." This gives the frontend a consistent experience: `createRemoteTicket` → get ticket → connect WS; and gives backend auditing consistency: `auditRemoteStart` records `Protocol: "rdp"`/`"vnc"`, and at session end `auditRemoteEnd` records duration.

**Why the ticket stays one-time**: although the Guacamole tunnel is a long-lived connection, the act of "establishing the tunnel" is itself a one-time handshake. If the connection drops and a reconnect is needed, apply for a new ticket — consistent with the existing terminal/desktop reconnect behavior (the "reconnect" button in `TerminalModal` just calls `createRemoteTicket` again). Making the ticket a session-level long-lived credential would only widen the leak window, for zero benefit.

**Ticket transport over a dual channel** (alignment verified against common-js during implementation): guacamole-common-js's `WebSocketTunnel.connect(data)` is `new WebSocket(url + "?" + data, "guacamole")` — the **subprotocol is hard-coded as `"guacamole"`**, and `data` is appended into the URL query, so there is no way to carry the ticket via a custom subprotocol. Hence the gateway accepts the ticket on both paths:

- URL query `?ticket=` (**primary**): the frontend's `client.connect("ticket=" + ticket)` lands it in the query; the ticket is still a one-time 5-minute ticket, leak window unchanged;
- `Sec-WebSocket-Protocol[0]` (compatibility slot): a custom Tunnel or paths not going through common-js keep the same shape as terminal/desktop.

The query-form ticket does not enter audit/logs (WS paths are skipped by `AuditMiddleware` and covered by the dedicated remote-access audit; `auditRemoteStart` records only ticket.ID, not the URL).

**Tunnel-internal control instructions are terminated by the gateway** (protocol layering): `WebSocketTunnel` periodically sends `ping` (empty opcode = `Guacamole.Tunnel.INTERNAL_DATA_OPCODE`) and expects the server to "respond with an identical ping"; with no data for `receiveTimeout=15s` the tunnel is closed, and `unstableThreshold=1.5s` marks the connection unstable. On an idle desktop, failing to echo the ping inevitably drops the connection, so the gateway intercepts instructions with opcode length 0 in `wsToGuacd` and echoes them verbatim (it reads only the first segment's length prefix to determine whether the opcode is empty, without parsing argument content) and **never forwards them to guacd** (guacd does not know tunnel-layer instructions). At handshake the gateway additionally sends `0.,N.<sessionID>;` as the tunnel UUID (single-element internal instruction → `setUUID`). This interception layer is a necessary part of "copying the official Tunnel layering," not free improvisation — guacamole-web's WebSocketTunnelEndpoint layers the same way.

The 401 recursive-refresh chain copies the existing implementation, with precedents on both sides:

- web's 401 interception at `services/api.ts:138` + `refreshToken()` (`/auth/refresh`; the interceptor now sits at line 138, refreshToken at 165), and mobile's `api/client.dart:83-91` explicitly documents the **recursive-refresh guard**: requests with `extra['skipRefresh'] == true` (i.e. the refresh request itself) that receive a 401 do not trigger another refresh and go straight to `onUnauthorized` — otherwise a failed refresh would recurse infinitely.
- The Guacamole tunnel's ticket application goes through the same `remoteClient` (which already has the Bearer interceptor), with identical 401 behavior; the WS handshake itself carries no JWT (the ticket is the credential), so the WS path needs no refresh, and on reconnect `createRemoteTicket` naturally runs through the 401 refresh once.

The point of this "reuse" is **not inventing a second auth-failure semantics**: the behavior after a user's token expires (silent refresh → on failure, log out back to the login page) is the same across ordinary APIs, terminal, desktop, and the Guacamole tunnel.

### D4 Session/permission/audit in the Go backend; guacd is stateless

guacd is a **stateless protocol translator**: one `connect` instruction opens one TCP connection to the target, translates the instruction stream, and `disconnect` ends it. It does not know "who" is connecting — username/password are parameters of the `connect` instruction. This property determines the division of responsibilities:

- **Go backend**: ticket issuance (including the `matchRemoteEgress` egress policy check), `terminal:write` permission point check, `auditRemoteStart/End/Failure` persistence, `RemoteSessionManager` session registry (pending/connected/closed/failed), session recording toggle and retention policy.
- **guacd**: translates protocol only. Credentials are passed through the Go gateway into the `connect` instruction, **never written to guacd logs or guacd config** (guacd's `GUACD_LOG_LEVEL` should stay at `info` or below to keep instruction content out of logs).
- **guacamole-common-js**: rendering and input only. It has zero awareness of "who may connect."

This is consistent with the existing discipline in `remote_audit.go`: audit details **must not contain passwords** (the `auditRemoteStart` comment explicitly says "do not write password"), and the Guacamole path follows the same rule.

### D5 guacd deployed with docker-compose

guacd is a standalone C daemon; the official image is `guacamole/guacd` (the guacd-only image). Deploy with docker-compose rather than requiring guacd installed on the host, for these reasons:

- Consistent with Cockpit's stacks/Docker capability (users already have Docker hosts);
- guacd depends on FreeRDP/VNC client libraries (libguac-client-rdp etc.); building from source has a long dependency chain, and the image already has them compiled;
- Version upgrades (Guacamole 1.5.x → 1.6.x) touch only the image tag in compose;
- guacd has no persistent state, so the container is stateless and can be rebuilt at will.

The cost is one more container to manage — but that is exactly the value scenario of the stacks feature (see `stack-deploy-design.md`); Cockpit deploying its own guacd with its own tooling is self-consistent.

## Phase 1 Acceptance Scope

Phase 1's goal is **"can connect, can operate"** — no audio, no file transfer, no advanced clipboard formats (plain-text clipboard counts as basic I/O and is included):

1. **Browser RDP to Windows**
   - Target: the remote desktop service (`termsrv`) of Windows Pro/Enterprise/Server, supporting NLA (CredSSP) authentication.
   - **Win11 Home has no RDP server**: Windows Home does not provide the remote desktop **server** (it can only act as a client connecting to others). This is Microsoft's product-line segmentation, not a defect of this design. Therefore acceptance for Win11 Home machines goes through the **VNC fallback** — install a VNC server such as TightVNC/Vino, and Guacamole's VNC protocol engine connects to it. The target machines of the Phase 1 acceptance item "RDP to Windows" are Pro/Server; Home machines get a separate "VNC fallback" sub-item.
2. **VNC to Linux**: VNC servers of Linux desktops (TigerVNC/TightVNC/x11vnc), supporting VNC password authentication.
3. **iPad Safari touch usable**: iPadOS Safari touch events map to Guacamole mouse instructions (`Guacamole.Mouse` already handles touch → mouse); single-finger move/click and two-finger scroll are operable; after screen rotation the `size` instruction renegotiates the resolution.

**Not in Phase 1**: audio (see risk section), file transfer (`Guacamole.Filesystem`), rich clipboard formats (RTF/HTML), SFTP file-transfer stacking (Guacamole's RDP drive redirection).

## Option Comparison

| Option | Protocols Covered | License | Key Trade-offs |
|------|---------|--------|---------|
| **noVNC** | **VNC only** (RFB) | MPL 2.0 | Lightweight, runs RFB in the browser, already used by Cockpit. But **covers VNC only**; RDP would still need another stack; cannot share rendering/input/clipboard code with the RDP stack. It would be enough if we only did VNC, but Phase 1 explicitly requires RDP. |
| **RustDesk Web** | RustDesk's proprietary protocol (not RDP/VNC) | **AGPL 3.0** | Good performance (self-developed codecs), works out of the box. **Excluded for license contagion**: AGPL requires the whole work to be open-sourced when distributing derivatives; Cockpit is Apache 2.0, and integrating an AGPL component would bind the entire combined work to AGPL, conflicting with the project's license choice. Also, the RustDesk protocol is proprietary and **cannot connect to standard RDP/VNC servers** — it requires RustDesk installed on both ends, which does not fit the "connect to the user's existing Windows remote desktop" requirement. |
| **xterm.js** | Character terminal (SSH/telnet) | Apache 2.0 | Already used for SSH (`TerminalModal`), **but this is not a desktop solution**. xterm.js is a terminal emulator; it renders a character grid, not pixels; RDP/VNC graphics streams have nowhere to go. **SSH alone on xterm.js is right** and should not be held hostage by the desktop solution — Guacamole supports SSH too, but we have already built it ourselves (agent side `crypto/ssh` + PTY), and the replacement benefit is lower than the migration cost. |
| **Self-developed (grdp + canvas)** | RDP (partial) | MIT (grdp) | Status quo. **No**: protocol coverage weaker than FreeRDP (NLA/CredSSP/TLS negotiation/RemoteFX/GFX), real-Windows connectivity questionable; rendering/input/clipboard all self-maintained; recopy everything for each protocol added. Sunk cost is not a reason to keep investing. |
| **Guacamole (guacd + common-js)** | **RDP / VNC / SSH / Telnet** | Apache 2.0 | **Take it**: the protocol engine is the FreeRDP/VNC client libraries (industrial-grade coverage); rendering/input/clipboard are carried by common-js; same license as Cockpit, zero contagion; guacd is stateless with no user concept, a clean boundary with "business in the Go backend." The cost is one more guacd process + writing the Guacamole protocol tunnel gateway ourselves (but the gateway is exactly the seat Cockpit already occupies). |

**Why not Teleport** (noted in passing): Teleport is an identity-centric access platform, including short-lived certificates, approval flows, and session recording — more complete capabilities than Guacamole. But it **ships its own user/role/resource model** (the same "second system" problem as guacamole-web), and desktop protocol support (RDP) requires Teleport's desktop access, whose deployment shape is a whole cluster. Cockpit is a personal/small-team console; bringing in Teleport is using a cannon to swat a mosquito plus a model conflict. Guacamole's "protocol translation only" boundary fits better.

## Skeleton Code

### 1. guacd docker-compose

```yaml
# deployments/guacd/docker-compose.yml (per the repo's deployments/ convention)
services:
  guacd:
    image: guacamole/guacd:1.5.5
    container_name: cockpit-guacd
    restart: unless-stopped
    ports:
      # bind intranet/loopback only: the Go backend reverse-proxies to this port; not exposed externally
      - "127.0.0.1:4822:4822"
    volumes:
      - guacd-recordings:/var/lib/guacamole:rw   # session recording on disk
      - guacd-drive:/var/lib/guacamole/drive:rw  # for phase-two file transfer
    environment:
      # logs contain no instruction content; do not enable debug (it would print connect parameters incl. passwords)
      GUACD_LOG_LEVEL: info
      TZ: Asia/Shanghai
    # guacd is stateless and can be rebuilt at will; the recording volume is the only persistent surface
volumes:
  guacd-recordings:
  guacd-drive:
```

Key points: the port binds `127.0.0.1` (the Go gateway reverse-proxies on the same host; in cross-host deployment bind an intranet address and close it off with a firewall); `/var/lib/guacamole` is guacd's default recording directory (see the "session recording for free" part of the risk section); `GUACD_LOG_LEVEL` stays off debug (guacd debug prints `connect` instruction parameters, including passwords).

### 2. Go session interface (WS gateway reverse-proxying guacd)

```go
// internal/server/api_guacamole.go
package server

import (
	"bufio"
	"net"
	"strings"

	"github.com/gorilla/websocket"
)

// GuacamoleSession is one Guacamole desktop session: a browser WS ↔ guacd TCP two-hop pipeline.
// Responsibilities: ticket validation (at handshake) + egress policy + audit + bidirectional byte pipe.
// Does not parse Guacamole instruction content (except constructing connect/size at handshake).
type GuacamoleSession struct {
	ID       string
	UserID   string
	Username string
	Protocol string // rdp / vnc
	AgentID  string
	Host     string
	Port     int
	ClientWS *websocket.Conn
	guacd    net.Conn
	done     chan struct{}
}

// handleGuacamoleWebSocket is the browser ↔ server leg: WS text frames are Guacamole protocol instructions.
// The ticket goes over a dual channel: URL query ?ticket= (primary, since common-js hard-codes subprotocol
// "guacamole" and appends connect data to the query) / Sec-WebSocket-Protocol (compatibility slot).
func (s *Server) handleGuacamoleWebSocket(w http.ResponseWriter, r *http.Request) {
	// 1. Ticket: Sec-WebSocket-Protocol[0] → ValidateTicket (one-time consumption)
	// 2. Permissions: terminal:write + matchRemoteEgress(agentID, host, port)
	// 3. Dial guacd TCP (127.0.0.1:4822, 10s timeout) — implementation correction: see the D2 note
	//    above; in reality the server builds agent egress plus an interface-bound relay, and
	//    connect uses the relay address
	// 4. Upgrade WS (Subprotocols echo back the ticketID)
	// 5. Construct handshake instructions to send to guacd: size / audio / video / connect
	//    connect parameters: protocol=rdp|vnc, hostname, port, username, password…
	//    (credentials come from ticket.Params, same as handleTerminalWebSocket's proxy_new dispatch)
	// 6. auditRemoteStart (Protocol: rdp|vnc, Session: sessionID)
	// 7. Bidirectional pipe:
	//      WS → guacd: conn.Write(ws text frame content)
	//      guacd → WS: bufio.Scanner splits on Guacamole instruction boundaries → ws.WriteMessage
	// 8. Either side disconnects → sendGuacamoleClose → auditRemoteEnd (with duration)
}

// Guacamole instructions are a text protocol of "comma-separated length prefixes + semicolon terminator",
// e.g.: 4.size,1.1,4.1024,3.768;
// Pipeline forwarding can stream through with bufio.Reader; no parsing needed.
```

**Why the Go side does not parse instruction content** (aside from handshake construction and tunnel-internal instruction detection): the Guacamole protocol is a private protocol between guacamole-common-js and guacd, evolving with Guacamole releases. If the Go gateway parsed instructions (e.g. to "audit keystrokes"), it would have to change with protocol versions and would reintroduce a "input contains passwords" leak surface (see `recording-design.md` D4). **The gateway is a byte pipe only**; protocol evolution is aligned by the two Guacamole ends themselves; the only minimal parsing is reading the first segment's length prefix to determine whether the opcode is empty (tunnel-internal instructions, see D3); that layering is isomorphic to guacamole-web's WebSocketTunnelEndpoint.

### 3. Frontend guacamole-common-js integration skeleton

```ts
// web/src/components/GuacamoleModal/index.tsx
import Guacamole from 'guacamole-common-js'

/**
 * Guacamole desktop Modal: guacamole-common-js carries rendering/input/clipboard;
 * this component only handles "apply for ticket → establish tunnel → lifecycle".
 */
const GuacamoleModal: React.FC<Props> = ({ visible, onClose, agentId, host, port, protocol, username, password }) => {
  const clientRef = useRef<Guacamole.Client | null>(null)
  const displayRef = useRef<HTMLDivElement>(null)

  const connect = useCallback(async () => {
    // 1. Apply for a ticket (reuse createRemoteTicket; the 401 refresh chain is already built into remoteClient)
    const { ticket } = await createRemoteTicket({
      agentId, host, port,
      protocol: protocol as RemoteProtocol,
      username, password,
    })

    // 2. Tunnel: the ticket goes in the URL query (common-js's WebSocketTunnel hard-codes
    // subprotocol "guacamole" and appends connect(data) to the URL query, so the ticket
    // cannot go via Sec-WebSocket-Protocol — an upstream implementation constraint, see D3 "ticket dual channel")
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const tunnel = new Guacamole.WebSocketTunnel(
      `${wsProtocol}//${window.location.host}/api/remote/guacamole`,
    )
    // guacamole-common-js's WebSocketTunnel itself handles the Guacamole protocol's
    // chunk/base64 codec and ping keepalive — do not improvise here (see risk section)

    // 3. Client: Display/Keyboard/Mouse are all carried by common-js
    const client = new Guacamole.Client(tunnel)
    clientRef.current = client

    const display = client.getDisplay()
    displayRef.current?.appendChild(display.getElement())

    // Touch → mouse: common-js's Guacamole.Mouse already handles touch events
    const mouse = new Guacamole.Mouse(display.getElement())
    mouse.onmousedown = mouse.onmouseup = mouse.onmousemove = (state) => {
      client.sendMouseState(state)
    }
    const keyboard = new Guacamole.Keyboard(document)
    keyboard.onkeydown = (keysym) => client.sendKeyEvent(1, keysym)
    keyboard.onkeyup = (keysym) => client.sendKeyEvent(0, keysym)

    // 4. Clipboard (plain text)
    client.onclipboard = (streams, mimetype) => {
      if (mimetype !== 'text/plain') return
      const reader = new Guacamole.StringReader(streams[0])
      let text = ''
      reader.ondata = (chunk) => { text += chunk }
      reader.onend = () => navigator.clipboard.writeText(text)
    }

    // The data of connect(data) lands in the WS URL query; the gateway exchanges the ticket
    // for the real credentials, zero credentials on the browser side
    // (see "Why parameters are placed into the connect instruction by the server")
    client.connect(`ticket=${encodeURIComponent(ticket)}`)
  }, [agentId, host, port, protocol, username, password])

  useEffect(() => () => clientRef.current?.disconnect(), [])
  // ...
}
```

**Why parameters (credentials) are placed into the `connect` instruction by the server rather than the frontend**: guacamole-common-js's `client.connect(data)` can take a parameter string, and the server can also carry parameters in the `connect` instruction. **Choose the server** because the credentials are already in ticket.Params (stored by `handleTicketCreate`), so the frontend never needs to handle them — this removes one exposure of credentials in browser memory/on the network, and it is the same shape as `handleTerminalWebSocket`'s `proxy_new` credential dispatch (the frontend passes the ticket; the agent receives resolved credentials).

## Risks and Boundaries

### H.264/frame rate: accept against office scenarios; do not chase media-grade

Guacamole 1.5.x's RDP driver supports the GFX/AVC444 (H.264) hardware encoding path; the VNC side has no H.264 (the RFB protocol itself is pixel/pseudo-pixel encoding). **The acceptance bar is set at office scenarios**: text-editor scrolling is smooth, window dragging shows no obvious frame drops, input latency is acceptable (subjectively "not painful"), and **frame-rate numbers/bitrate are not the target**.

Rationale: Cockpit's remote-access scenario is "remote ops/incident response" (edit configs, read logs, restart services), not watching video/3D. Media-grade acceptance (e.g. stable 30fps, <100ms input latency) would funnel effort into tuning guacd's FreeRDP codec parameters, whose benefit is barely perceptible in office scenarios. In Phase 1, if the office scenario is smooth, that is a pass; if a specific target machine stutters, first check guacd's `disable-audio`/`color-depth`/`resize-method` parameters, then consider whether it is a network issue.

**Whether H.264 is enabled is not configured by us**: FreeRDP's AVC/H.264 requires target-Windows support (RemoteFX/GFX or AVC444), and guacd negotiates it automatically. We neither force it on nor forbid it — the negotiation result is the fact.

### Audio deferred to Phase 2

**(Corrected against source during implementation)** Audio streams use the Guacamole protocol's `audio,<streamIndex>,<mimetype>;` stream-creation instruction + the `blob` data stream, pushed **proactively by the server** (not a connect parameter), and pass automatically through the Go gateway's byte pipe — zero gateway changes; the client plays with `onaudio` or `Guacamole.AudioPlayer.getInstance(stream, mimetype)`, and the built-in `RawAudioPlayer` supports `audio/L8`/`audio/L16` (raw PCM). The design skeleton's phrasing of "the connect instruction's audio parameter" has been corrected per source verification.

Guacamole supports audio (RDP's `rdpsnd`; VNC has no audio), but:

1. Browser audio playback requires a user gesture to unlock (iOS Safari especially strict); mute/volume controls need UI wiring of our own;
2. guacd's audio codecs (e.g. RDP's Opus/MP3) must line up with the browser `Guacamole.Audio` playback chain;
3. In ops scenarios desktop audio has low value (unlike remote media).

**The benefit/cost ratio is low**, hence the explicit deferral. Architecturally there is no dead angle (source-verified): audio streams are already passed through inside the byte pipe; as long as `guacConnectArgs` does not pass `disable-audio` it is enabled by default; Phase 2 only adds the mute/volume-control UI (mute via `AudioContext.suspend()/resume()`, volume requires a custom AudioPlayer plugging in a GainNode), without touching the Phase 1 protocol path.

### Tunnel binary-frame chunk/base64 rules copied from the official Tunnel implementation

**This is the easiest place to outsmart oneself and crash**, hence the standalone emphasis:

In the Guacamole protocol's instruction stream, binary data (bitmaps, audio frames, clipboard files) is not stuffed directly into instructions but goes through the "**stream**" mechanism: the `blob` instruction carries a base64 chunk (with a stream index), and the `end` instruction closes it. `guacamole-common-js`'s `Guacamole.Tunnel` implementation contains `sendMessage`'s opcode assembly (length-prefix format `length.opcode,arg1,arg2...;`) and the base64 chunk splitting rules.

**Rules**:

1. The Go gateway does **not** reimplement chunk/base64 codec — the gateway is only a byte pipe between WS text frames ↔ the guacd TCP stream; the codecs on both ends are done by `guacamole-common-js` and guacd respectively;
2. If instructions must be constructed on the Go side in the future (e.g. the handshake-phase `connect`), **copy** the length-prefix format of [guacamole-common-js's `Guacamole.Tunnel.sendMessage`](https://github.com/apache/guacamole-client-commonjs) and the instruction parsing of [`guac-hawk-bit`](https://github.com/apache/guacamole-server) — **do not improvise** — the protocol is private; one wrong format detail means a silent stream cut with extremely high debugging cost (the instruction stream is opaque);
3. Version alignment: the guacamole-common-js version and the guacd version must match to the same Guacamole release (1.5.x pairs with 1.5.x). The protocol is private; cross-version compatibility is not guaranteed.

The root cause of this risk is "private protocol + no schema": the Guacamole protocol has no public IDL/schema; the format conventions exist only in the two ends' implementation code. Copying is faster and more correct than writing it ourselves.

### guacd session recording: audit replay for free

**This is an unexpected dividend of integrating Guacamole**, worth a separate section:

guacd natively supports recording sessions as **`.guac` format** (Guacamole session recording): pass `recording-path` + `recording-name` parameters in the `connect` instruction and recording happens automatically, with no need for us to parse/forward any pixel data. The recorded `.guac` files can be replayed/transcoded with the official `guacplay` (a tool shipped with guacamole-server) or `guacenc`.

Compare with Cockpit's status quo: the terminal side already has asciinema v2 recording (`recording-design.md`, M1 landed: `recording.go` + the `TerminalRecording` table + the `/recordings` replay page), but **desktop-side (RDP/VNC) recording is explicitly listed as "not doing"** (see recording-design.md D1/not-doing section: "VNC/RDP pixel-stream recording (WebM/image sequences; large volume, heavy replay)"). The reason it was rejected then was the **high cost of self-developed recording** — pixel-stream recording means writing our own format and our own player, with large volumes.

Under the Guacamole route this cost is **nearly zero**: guacd writes the `.guac` file, and we only need to:

1. Add `recording-path` (pointing at the guacd container volume) + `recording-name` (the session ID) parameters to the `connect` instruction;
2. After the session ends, collect the `.guac` file from the guacd volume into Cockpit's `data/recordings/` directory (same directory as asciinema `.cast`; the retention/cleanup logic in `recording.go` is reused directly);
3. Replay: Phase 2 builds a web player (parsing the `.guac` format), or start with the official `guacplay` for offline replay (CLI tool).

**This directly fills the "not doing" gap in `recording-design.md`** and aligns semantically with the existing `recording.enabled` / `recording.retention_days` settings. Phase 1 lands "recording to disk" first (free on the guacd side); the web player goes to Phase 2.

The cost: `.guac` is a Guacamole-proprietary format (same "private protocol" risk) and cannot be replayed outside the Guacamole ecosystem — but asciinema `.cast` is an open format replayable outside Cockpit; the two complement each other (terminal open format, desktop Guacamole format).

## Credential Storage Semantics: Credential Vault

> Raised by the user in real testing on 2026-10-02: having to hand-type the password/private key on every connection was unacceptable; they asked for a "vault" modeled on the Alibaba Cloud password box, with two explicit alignments — **server-side encrypted storage of each machine's credentials** + **re-verifying identity to view/manage credentials**. Already landed (`0d2f9da`): the Guacamole modal and the built-in terminal, the two entry points, are wired the same way; the management entry lives inside the connection modal, with no separate management page.

### Two Paths: Default No-Persistence vs. Explicit Save

- **Default path (unchanged)**: passwords/private keys go only into `ticket.Params` → the guacd `connect` instruction / the agent's SSH dialing; never written to the DB, never into audit — the existing discipline of D4 and `remote_audit.go`'s "do not write password" continues to hold;
- **Save path (explicit action)**: check "save credentials to the server" in the connection modal → pass second-factor verification before submitting → plaintext is handled only at the instant of submission; after encryption with `storage.Encrypt` (AES-256-GCM, key derived from `TOTP_ENCRYPTION_KEY`; `internal/storage/crypto.go` shares the same facility as TOTP) it lands in the `remote_credentials` table; the json tags of `password_enc`/`private_key_enc` are `-`, so ciphertext fields are never serialized — plaintext goes in but never out; the list API returns only username/domain/has_password/has_private_key metadata;
- **Isolation and overwrite**: isolated per logged-in user; `idx_remote_cred_target` (user_id+agent_id+host+port+protocol) is a five-column composite unique — saving again for the same target counts as an update, aligned with the same-target overwrite semantics of `services/desktop.ts` remembering usernames in localStorage.

### Connection Injection: Zero Credentials on the Browser Side, Unchanged

- **Connect with saved credentials**: the frontend sends only `use_saved: true`, carrying no credential fields — the server looks up and decrypts by (user, agent, host, port, protocol), filling in the missing username/password/private_key/domain of the request (explicitly passed values take precedence); if no saved credential exists → 400 `no saved credential for this target`;
- **Egress policy still goes first**: `matchRemoteEgress` runs before use_saved resolution (comment in `api_remote.go`: "egress validation still goes first") — the vault does not constitute a bypass around `remote_control.allowed_targets`;
- **Auditing**: `RemoteSessionDetails.AuthSource` records `saved` / `user` (empty = no credentials, e.g. VNC anonymous or the agent's default key); details still never write the password.

### Second-Factor Verification for Management Operations (Aligned with the Alibaba Cloud Password Box)

- `POST /api/remote/vault/verify`: login password (bcrypt comparison) or a TOTP one-time code (`ValidateTOTPCode`; when TOTP is enabled, either works) → issues a **10-minute multi-use** vault token (server-side in-memory map; expired entries are lazily cleaned at the next issuance);
- List/save/delete (`/api/remote/vault/credentials`) hang off `X-Vault-Token` validation; missing/expired returns 401 prompting re-verification; the token lives only in frontend memory (a module variable, not in localStorage) and expires when the page closes;
- **Layered convenience**: *connecting* with saved credentials needs only the login state (that is the vault's convenience point); **deleting saved credentials** always requires second-factor verification (a destructive management action; interaction is "verify first, cancellable").

### Wording

"Passwords are not persisted by default" refers to the default path; the wording after checking save is "stored encrypted with AES-GCM on the server; the browser never handles plaintext again." The two sentences correspond to the two paths above and must not be read as substitutes for each other; "never into logs" refers to the guacd and audit sides (D4); what lands in the DB is ciphertext the user explicitly asked to save.

## Not Doing (Future Versions)

- **Audio** (see risk section, Phase 2);
- **File transfer**: Guacamole's `Guacamole.Filesystem` + RDP drive redirection / VNC's SFTP stacking. Overlaps with Cockpit's `file-manager-design.md` route; file in/out is covered first by the file manager (agent-side RPC); in-desktop file transfer to be discussed as needed;
- **Rich clipboard formats** (RTF/HTML): Phase 1 is plain text;
- **Web `.guac` player**: Phase 1 uses the official `guacplay` for offline replay; the web player in Phase 2 (can reuse the player skeleton of the `/recordings` page);
- ~~**SSH/telnet over Guacamole**~~ (**2026-09-25: the SSH part voided**: the new direction of unifying all three protocols on a third-party stack has landed SSH access, see [remote-access-integration-design](./remote-access-integration-design.en.md); the experience difference (guacd server-side-rendered tiles vs. xterm.js character grid) is absorbed by the "built-in terminal (via Agent)" fallback entry; the agent channel is not deleted. **telnet stays not-doing** — unencrypted, legacy debugging use, keeps going through xterm.js): original rationale — SSH was already built by us (`internal/proxy/ssh_session.go`, agent side `crypto/ssh` + PTY + TOFU host key), and the xterm.js experience beats Guacamole's terminal emulation; same for telnet. **Desktop protocols (RDP/VNC) go through Guacamole; character terminals (SSH/telnet) go through xterm.js** — the two do not hold each other hostage;
- **Replacing the agent-side RDP (grdp)**: in Phase 1 the Guacamole route is **added** on the server side; the agent-side grdp is retained as a "degradation path when no guacd is deployed" or deprecated per the Phase 1 acceptance results. The two coexisting for long is inappropriate; the decision comes after Phase 1 acceptance;
- **A standalone vault management page**: list/delete/second-factor verification live inside the connection modal (see the "Credential Storage Semantics" section); a standalone management page under Settings to be discussed as needed.

## Milestones

**Phase 1: can connect, can operate** (the acceptance scope of this design)

1. guacd docker-compose lands (`deployments/guacd/docker-compose.yml`);
2. Go WS gateway reverse proxy (`internal/server/api_guacamole.go`) + ticket/audit/egress policy;
3. web `GuacamoleModal` (guacamole-common-js) replaces `DesktopModal`'s RDP/VNC entry (or a parallel entry behind a flag);
4. Acceptance: browser RDP to Windows Pro/Server (NLA), VNC to Linux, Win11 Home VNC fallback, iPad Safari touch usable.

**Phase 2: reinforcements**

- Audio;
- `.guac` web player + integration with the `/recordings` page;
- File transfer (as needed);
- Deciding the grdp route's fate.

## References

- [Apache Guacamole architecture](https://guacamole.apache.org/doc/gug/architecture.html)
- [guacamole-common-js API](https://guacamole.apache.org/doc/gug/guacamole-common-js.html)
- [Guacamole protocol (instruction format)](https://guacamole.apache.org/doc/gug/protocol-reference.html)
- [guacd parameters (recording-path etc.)](https://guacamole.apache.org/doc/gug/configuring-guacamole.html)
- In-project: `docs/guide/recording-design.md` (terminal recording, desktop recording gap), `docs/guide/reference-projects.md` (Guacamole reference notes), `internal/server/ticket.go` (ticket semantics), `internal/server/remote_audit.go` (remote-access audit), `web/src/services/api.ts` (401 refresh)
