# Remote Access Three-Protocol Third-Party Integration Design: A Unified VNC / SSH / RDP Guacamole Stack


> Initiated 2026-09-25. Direction aligned with the other repository: the remote access integration
> of the three protocols VNC / SSH / RDP is unified on third-party solutions, no longer
> maintaining separate self-developed protocol chains. RDP / VNC already landed in
> [remote-desktop-guacamole-design](/remote-desktop-guacamole-design)
> (the Guacamole route); this document completes **SSH joining the same stack** and provides a
> selection overview for the three protocols. The old design's "not doing: SSH/telnet via
> Guacamole" item is **repealed** as of this document
> (see D7); the remaining decisions are unaffected.
>
> 2026-10-07 update: the old self-developed desktop chains (grdp / noVNC direct connections) have
> been deleted from the codebase entirely
> (D-2026-10-07-1); the comparison descriptions of the old chains in this document serve only as historical background.

## Current state (reviewed 2026-09-25)

| Protocol | Frontend entry | Channel | Protocol terminator (third-party library) | Status |
| --- | --- | --- | --- | --- |
| RDP | GuacamoleModal | `/api/remote/guacamole` → guacd | guacd (FreeRDP) + guacamole-common-js | Integrated |
| VNC | GuacamoleModal | Same as above (protocol=vnc) | guacd (libvncclient) + guacamole-common-js | Integrated |
| SSH | TerminalModal (xterm.js) | `/api/remote/terminal` → agent | `x/crypto/ssh` inside agent (a Go library, not a remote-access-specific stack) | Self-developed chain |
| Telnet | TerminalModal | Same as above | agent raw TCP | Unchanged |
| (fallback) | VNCModal (noVNC) / DesktopModal (grdp) | agent channel | noVNC / grdp | Kept for a coexistence period per the 2026-09-24 decision; **the components were deleted by D-2026-10-07-1** (this table is a historical snapshot) |

There is only one gap: **SSH does not ride the third-party remote-access stack**. For the
pain-point discussion of VNC/RDP (the maintenance cost of self-developed protocol chains,
carrying rendering/input/clipboard ourselves), see the "pain points" chapter of the old design
document; the same applies to SSH — and SSH is the only one of the three protocols still
sitting on a self-developed chain.

## Selection

### Client selection table for the three protocols

| Protocol | Selection | Underlying engine | Entry point |
| --- | --- | --- | --- |
| RDP | guacd's rdp plugin | FreeRDP | `/api/remote/guacamole` (protocol=rdp) |
| VNC | guacd's vnc plugin | libvncclient | `/api/remote/guacamole` (protocol=vnc) |
| SSH | guacd's ssh plugin | libssh2 | `/api/remote/guacamole` (protocol=ssh, new) |
| Browser rendering/input | guacamole-common-js | canvas / keyboard / clipboard | GuacamoleModal (all three protocols reuse the same component) |

**One-sentence rationale**: one guacd daemon covers all three protocols (plus optional
telnet), and on the browser side common-js is the single client that covers everything;
session/ticket/audit/egress policy stays in the Go backend (the boundary from old design
D4 is unchanged), and guacd is stateless with no concept of users.

### Why the alternatives are not used

| Option | Coverage | Reason for rejection |
| --- | --- | --- |
| **noVNC** (today's VNCModal) | VNC only (RFB) | Covers only VNC, leaving RDP/SSH unresolved; cannot share rendering/input/recording code with the RDP stack. Kept as a fallback, not extended further. |
| **x/crypto/ssh + xterm.js** (today's agent channel) | SSH only | This is a self-assembled "Go library + terminal emulator" chain, not a remote-access solution: host key (TOFU), keepalive, recording, and protocol evolution are all on us. Kept as the agent-side fallback, not the main entry. |
| Standalone terminal gateways such as **WebSSH / sshwifty / ttyd** | SSH only | One more service to deploy and one more account system; tickets/audit/egress policy would need a second integration pass. guacd is already in the deployment. |
| **server dialing crypto/ssh directly as a self-developed gateway** | SSH only | Equivalent to shifting the agent channel onto the server; recording and protocol maintenance would still be self-developed — the opposite of the "integrate third-party solutions" direction. |
| **Teleport** | SSH/K8s/DB | An enterprise-grade bundle (ships its own user store/certificate system/Proxy), too heavy for a personal homelab. |
| **Full Apache Guacamole** (including the Java web layer) | RDP/VNC/SSH/Telnet | Already rejected by old design D1: the Java layer overlaps in responsibility with Cockpit's Go backend (auth/session/audit all live on the Go side). We take only guacd + common-js. |
| **Myrtille / SparkView** | Mostly RDP | Commercial licensing or insufficient activity, incomplete protocol coverage. |

## Protocol and data flow (new SSH segment)

```
Browser                    Go gateway                      guacd                  Target
   │  WSS /api/remote/guacamole?ticket=…                     │                    │
   │  (Sec-WebSocket-Protocol fallback)                      │                    │
   │ ────────────────────────────────────────────────────────>│  TCP select "ssh"  │
   │                     parameter name list response <────── │                    │
   │                     size 1280x800 + connect ───────────> │ ── libssh2 ──────> │ :22
   │                     (username/password/private-key)     │    auth + PTY      │
   │ <───────────── instruction stream (terminal tiles/clipboard/sync) ───────── │ <───────────────── │
   │ ────────────── keyboard/mouse/clipboard instructions (key/mouse/clip) ───> │ ────────────────> │
```

- **Browser ↔ server segment**: WebSocket text frames carry the Guacamole instruction stream over exactly the same pipe as RDP/VNC; the gateway does not parse instruction content (except for constructing the handshake instructions).
- **server ↔ guacd segment**: `select ssh` → parameter list → `size` + `connect`. The pixel width/height of `size` is converted by guacd's ssh plugin into terminal columns/rows based on the default font metrics (the connect parameters carry no width/height — the official ssh parameter set has no such item).
- **guacd ↔ target segment**: libssh2 dials `hostname:port` and authenticates with the credentials carried in the connect instruction; terminal output is encoded as a Guacamole instruction stream (tiles) and pushed back.

### connect parameter mapping (D2)

| ticket parameter | guacd connect parameter | Description |
| --- | --- | --- |
| `username` | `username` | Passed through directly (upstream already handles it uniformly) |
| `password` | `password` | Passed through directly; password authentication |
| `private_key` | `private-key` | **PEM passed through as-is** (not base64); private key authentication, takes precedence over password |
| `domain` | Not sent | SSH has no concept of a domain (RDP-specific) |
| `width`/`height` | Not sent | Character terminal; size travels via the tunnel-layer `size` instruction (guacd converts to columns/rows itself) |

## Integration with the existing remote-access chain

Everything is reused, zero new infrastructure:

1. **Tickets**: the `private_key` field of `POST /api/remote/tickets` has long existed
   (used for agent-channel SSH private keys) and the Guacamole route reuses it directly; tickets remain one-shot, in-memory, valid for 5 minutes,
   and credentials never enter URLs/logs/audit.
2. **Egress policy**: `matchRemoteEgress` (allow-list / CIDR / per-agent), with agent_id as the policy anchor and audit attribution — semantically identical to RDP/VNC. guacd dials only the local loopback relay
   (`internal/server/guac_relay.go`); the real target is dialed by the agent from its own network position, so the agent sits on the data path (landed 2026-10-02, correcting old design D4's "guacd dials directly" assumption).
3. **Audit**: `auditRemoteStart/End` is the same across protocols; SSH sessions via guacd likewise record `Protocol: ssh`, and credentials (password/private key) never enter audit details.
4. **Recording**: guacd's ssh plugin likewise supports `recording-path/recording-name` → the `.guac` collection chain (`collectGuacRecording`, Format=guac) works as-is, and SSH session recordings play back on the same `/recordings` page as RDP/VNC.
5. **Permissions**: `terminal:write` (the RBAC `terminal` resource domain, `/api/remote/*` is merged into it uniformly, see `internal/server/rbac.go`); the entry button's
   PermGuard does not change when the channel switches.

## Decisions

- **D1 protocol whitelist extended with `ssh`**: `handleGuacamoleWebSocket` admits rdp/vnc/ssh; **telnet is not admitted** (the direction names only the three protocols; telnet has no encryption, so the agent channel stays as-is).
- **D2 connect parameter mapping**: see the table above; the private key PEM is passed through as-is (corrected during real-machine acceptance on
  2026-09-30: the settings.h comment "encoded as base64" contradicts the implementation —
  guac_user_parse_args_string → key_alloc does zero decoding, so base64 text fed into libssh2
  PEM_read always fails; A/B live tests showed raw text passing authentication in both formats while base64 failed across the board).
- **D3 frontend entry switch**: the web's default SSH entry switches from TerminalModal to
  GuacamoleModal (the Workbench SSH Tab and the Agents page route uniformly), consistent with
  RDP/VNC; the SSH Tab keeps the "built-in terminal (via Agent)" secondary entry as a fallback.
- **D4 agent channel downgraded to fallback, not deleted**: the same "coexistence period" discipline as the grdp route —
  TerminalModal is retained (telnet still goes through it; SSH has a way out when guacd is unreachable;
  mobile keeps using it).
- **D5 recording/audit/egress policy reuse**: see the previous section; zero new configuration keys.
- **D6 mobile untouched**: mobile's SSH terminal rides the independent `/api/remote/terminal` +
  xterm.dart chain and is not changed this time (the Flutter side has no guacamole client; to be
  evaluated separately later).
- **D7 old decision repealed**: the "not doing: SSH/telnet
  via Guacamole" item of remote-desktop-guacamole-design.md is repealed (telnet remains not done, SSH joins); the remaining "not doing"
  items (audio UI, file transfer, SFTP, etc.) are unchanged.

### Experience differences (recorded as-is)

guacd's ssh terminal is a **server-rendered tile stream** (bitmaps encoded as guacamole
instructions), not xterm.js's character grid: higher bandwidth usage, and font rendering is
decided by guacd. The gains of the unified stack (same component, same recording, same
deployment) outweigh this difference; for scenarios that demand high-fidelity terminal
rendering, use the "built-in terminal (via Agent)" fallback entry. This difference is the
root reason D3/D4 coexist — it is not a transitional state.

### D8: SSH size follows the window; reverse clipboard enabled for the terminal only

"Joining the same stack" does not mean "the three protocols behave identically" — two spots
diverge per protocol, both dictated by semantics rather than implementation laziness:

| | RDP / VNC (desktop) | SSH (terminal) |
| --- | --- | --- |
| `size` instruction semantics | "switch the remote desktop resolution" | "how many pixels the terminal occupies → guacd converts to columns/rows" |
| Who sends it | Toolbar resolution dropdown (explicit user choice) | `ResizeObserver` watching the display container + 150ms debounce, automatically following window/fullscreen resizing |
| Toolbar resolution dropdown | Shown | **Hidden** (size follows the window; fixed presets would fight it) |
| Reverse clipboard (browser → remote) | Not wired (keeps the existing forward-only behavior; zero behavior change on the desktop path) | Wired: Ctrl+V in the terminal area (`paste` event, carries its own data, no Clipboard permission needed) + the toolbar "Paste to remote" button (`navigator.clipboard.readText()`; on read failure the prompt says to use Ctrl+V) |

Why the desktop side does not send `size` automatically: that would make "the desktop as big
as the window" and, on fullscreen, override the resolution the user carefully chose —
inconsistent with existing behavior. Why the terminal side must send it automatically:
full-screen programs like `vim` / `top` redraw by columns/rows; without following they would
stay at the column/row count from connection time and leave a black band on the right.

The reverse clipboard uses `Client.createClipboardStream('text/plain')` +
`Guacamole.StringWriter` (the official common-js approach); the Go gateway is a byte pipe,
the `clipboard` instruction is passed through to guacd as-is, **zero backend changes**.
Plain text, single layer (rich formats: see "not doing").

### Implementation notes

- In `GuacamoleModal`, the `clear` of `useConnectionTimeout` was once destructured as
  `clearTimeout`, **shadowing the global `clearTimeout`** — the first version of the
  size-sync debounce was bitten by it (two consecutive resizes sent two `size`
  instructions). Renamed to `clearConnTimeout`.
  There are also real timers inside the component; never name the hook's clear `clearTimeout` again.
- The "Paste to remote" button of `RemoteToolbar` got an `aria-label` (it was icon-only
  with no accessible name, and tests could only locate it by button index).

## Testing and acceptance

**Unit tests (landed in this change)**:

- Go: the ssh branch of `guacConnectArgs` (username/password/private-key PEM passed through raw,
  no width/height/domain; private-key base64 was once a defect, corrected during real-machine acceptance
  on 2026-09-30); WS whitelist admits ssh and rejects telnet;
  full-chain (covGuacdServer) assertions on the ssh handshake instructions.
- web: the GuacamoleModal ssh form (username required, password/private key optional, no
  private_key sent when there is no private key); Workbench SSH routes to GuacamoleModal, and the built-in-terminal
  secondary entry still opens TerminalModal.
- web (completed under D8): ssh size sync (send `sendSize` with the last size after the
  container-resize debounce, skip on size 0, no ResizeObserver attached for RDP, effect cleanup
  on unmount);
  reverse clipboard (terminal paste / toolbar button, readText failure fallback); SSH hides
  the resolution dropdown while RDP keeps it; Agents page ssh routing assertions updated with D3.
- `web/src/test/setup.ts` adds an observable ResizeObserver stub (jsdom has no implementation),
  with trigger `window.__triggerResize(w, h)`.

**Real-machine acceptance (passed on 2026-09-30; see the "remote access three protocols" section of `docs/guide/acceptance-checklist.md`)**:

- Real guacd + real sshd: one pass each of password authentication / private key authentication;
- Terminal rendering (vim/top full-screen programs), `size` changes converting to columns/rows;
- SSH session `.guac` recording collection and `/recordings` playback;
- Bidirectional clipboard (browser ↔ sshd).

## Not doing (future versions)

- **telnet via guacd**: the direction names only the three protocols; telnet stays on the agent channel.
- **SFTP / Guacamole Filesystem drive**: overlaps with the file-manager route;
  keeps the old design's "not doing".
- **Deleting the agent SSH channel / TerminalModal / VNCModal / DesktopModal**:
  coexistence-period discipline; to be decided together after the Guacamole approach passes real-machine acceptance.
- **Switching mobile SSH to guacamole**: see D6.
- **guacd ssh-specific parameters** (font-name/color-scheme/timezone/backspace, etc.):
  start with the minimal parameter set and add on demand (parameters are append items of the connect instruction, zero migration).
