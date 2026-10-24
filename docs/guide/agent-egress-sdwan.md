# Agent Egress and SD-WAN Capability Boundaries


This document records the actual capabilities of Cockpit's Agent as an intranet access egress, and the gap to full SD-WAN. Conclusions are based on the current code implementation.

## Conclusion

The current Agent already has the base capability of "accessing targets reachable in a certain online Agent's network through that Agent", but it is not SD-WAN. It is closer to application-layer remote control, port proxying, and a bastion-channel.

If the goal is "pick an Agent as an egress point to access SSH/RDP/VNC on some intranet machine", the existing architecture can support part of it. If the goal is "an entire subnet routed transparently through a certain Agent, joining like SD-WAN", it is not yet available.

## Capabilities Already Present

### Agent Connecting to Intranet TCP Targets

The Server can send a `proxy_new` message to an Agent; on receipt, the Agent connects to the target `host:port` and forwards data bidirectionally via `proxy_data`. As long as the network the Agent sits in can reach the target address, the Server can establish an application-layer channel through that Agent.

Key implementation:

- `internal/proxy/handler.go`: the Agent side connects to the target via `net.DialTimeout("tcp", target, ...)`.
- `internal/proxy/mgr/manager.go`: the Server side can listen on a local port and forward client connections to a specified Agent.
- `internal/server/api_remote.go`: the remote terminal entry point creates the proxy connection to the Agent via a ticket.

### Desktop Remote Control (RDP/VNC/SSH, Guacamole)

RDP/VNC/SSH desktops all go through the Guacamole tunnel: the browser's `GuacamoleModal` connects to `/api/remote/guacamole`, the Server converts guacamole-common-js commands into `proxy_new/proxy_data` with the `guac:` prefix for the Agent, the Agent only performs TCP relay to guacd, and the protocol is terminated by guacd (RDP via FreeRDP, VNC via libvncclient). No special build tags required; the in-house grdp and noVNC direct-connect paths have been removed (D-2026-10-07-1).

Key implementation:

- `internal/server/api_guacamole.go`: ticket validation, `guac:` command encode/decode, and byte relay.
- `web/src/components/GuacamoleModal/index.tsx`: guacamole-common-js rendering and input.

### Fixed-Port Proxy

ProxyManager supports listening on a port on the Server side and forwarding to a target service through a certain Agent. The current actual listen address is fixed to `127.0.0.1:<remotePort>`, which is a safe default avoiding accidental public exposure.

This suits local tooling or a fronting reverse proxy joining in; it does not mean automatically publishing intranet services to the public internet.

## Current Gaps

### Not Full SD-WAN

The following capabilities are not currently implemented:

- TUN/TAP virtual network interfaces
- L3 routing/forwarding
- Subnet routing table distribution
- NAT, iptables, or nftables management
- CIDR-level access control
- Real UDP proxy forwarding
- Multi-Agent path selection, link quality probing, and failover

The repository has WireGuard / ZeroTier / Tailscale capability detection (`internal/agent/detector/overlay.go`, reporting the `overlay` capability if any is available), but these are only capability detection, not tunnel establishment or routing management; Cloudflare Tunnel and routing-information probing are not implemented.

### No Full "Subnet Egress via Agent" Policy Model

The remote-control whitelist now supports hostnames, IPs, and CIDRs, and `remote_control.egress` already supports minimal egress restriction by `agent_id + allowed_targets + allowed_ports`. But it still does not derive allow-lists automatically from inventory, nor does it have multi-Agent routing selection, priority, failover, or dynamic path probing.

Key implementation:

- `internal/server/remote_audit.go`
- `remote_control.allowed_targets` in `config/cockpit.yaml`

### Web SSH Is Already a Complete SSH Client

The SSH path of the TerminalModal (browser xterm WebSocket): the Agent-side `internal/proxy/ssh_session.go` implements a complete SSH client (handshake, authentication, encryption, PTY allocation, shell), called by `internal/proxy/handler.go` at connection time; the Server only orchestrates tickets, auditing, and WebSocket relay, not touching the protocol stack.

Therefore the current Web SSH already has full protocol capability and is no longer a gap; the remaining work is unifying ACL, error feedback, and auditing across the two paths.

### UDP Proxy Not Actually Landed

**Rejected (D-2026-10-08-2)**: the API explicitly refuses `proxyType=udp` (returns 400), accepting only `tcp`. The storage layer still keeps the field but writes are fixed to `tcp`. A UDP proxy requires separate connection modeling, session timeouts, packet boundaries, and return paths; it is not in the current plan.

## Suggested Roadmap

### Phase 1: Agent Egress Gateway

Prioritize implementing "a certain Agent can serve as the remote-control egress for certain hosts or subnets"; do not start by developing a full SD-WAN in-house.

Suggested configuration model:

```yaml
remote_control:
  egress:
    - agent_id: office-agent
      allowed_cidrs:
        - 192.168.10.0/24
      allowed_ports:
        - 22
        - 3389
        - 5900
```

Already landed:

- `remote_control.allowed_targets` supports CIDR.
- `remote_control.egress` supports constraining targets and ports per Agent.
- Ticket creation validates whether `agent_id + host + port` matches the policy.
- Audit logs record the matched egress Agent, target IP, port, and protocol (`RemoteSessionDetails.Egress`).
- **Multi-policy matching rules are now defined (D-2026-10-12-2)**: when one Agent has multiple `egress[]` entries, matching is combined as "released if any single policy releases both target and port at the same time", independent of configuration order; no cross-policy assembly (a target released by policy A × a port released by policy B does not constitute a release). When an Agent does not appear in any policy, the refusal text points to `remote_control.egress` (add an Agent-level policy); when the target appears in a policy but the (target, port) combination is not covered by any policy, the refusal text points to `allowed_ports`.

Suggested follow-up:

- If transparent networking is needed, introduce a dedicated data plane.

This phase satisfies most of the "access intranet SSH/RDP/VNC through an Agent" requirement while keeping the implementation simple.

### Phase 2: Completing Protocol-Level Remote Control

Protocol-level remote control has already been unified via the Guacamole stack: RDP/VNC/SSH desktops are terminated by guacd, the browser does not implement a protocol stack itself; the Agent does not terminate protocols, only TCP relay, with no special tag requirements on build artifacts.

SSH has an additional built-in terminal fallback path: the Agent side uses `x/crypto/ssh` (`internal/proxy/ssh_session.go`, `ssh_provider.go` only supplies the default key) to complete handshake, authentication, and PTY forwarding; the Server is only responsible for orchestration, auditing, and WebSocket relay.

The remaining work is unifying ACL, error feedback, and auditing across the two paths, not building another protocol client.

### Phase 3: Port Publishing

If local tools need direct access, for example `ssh user@server -p 2222` to reach a machine behind an Agent, the ProxyManager should explicitly model the listen address and exposure policy.

Suggested new fields:

- `bindAddress`: defaults to `127.0.0.1`.
- `allowedSources`: an IP/CIDR allow-list of sources.
- `publicBind`: only listen on `0.0.0.0` when an admin explicitly permits it.

Do not simply change the current fixed `127.0.0.1` to `0.0.0.0`, which would widen the exposure surface.

### Phase 4: True SD-WAN Integration

If the goal is genuinely transparent L3 networking, it is recommended to integrate a mature solution rather than develop a full data plane inside Cockpit.

Possible directions:

- WireGuard: Cockpit handles configuration distribution, node registration, state monitoring, and auditing.
- Headscale/Tailscale: Cockpit acts as a console integrating nodes, ACLs, and service entries.
- Cloudflare Tunnel: suited for application-layer service publishing, not for full L3 subnet interconnect.

Cockpit is better suited as a control plane and ops entry point, with the data plane delegated to a mature tunnel or VPN implementation.

## Capability Matrix

| Goal | Current state | Notes |
| --- | --- | --- |
| Agent accessing a single TCP target reachable in its network | Basically available | Forwarded via `proxy_new` and `proxy_data` |
| VNC browser direct connect | Basically available | Via Guacamole (guacd terminates VNC), Agent only `guac:` TCP relay |
| RDP browser direct connect | Basically available | Via Guacamole (guacd terminates RDP), no special build tags |
| True browser SSH login | Basically available | Guacamole SSH desktop; Agent built-in `x/crypto/ssh` terminal fallback |
| Local SSH client via Server port forwarding | Architecturally supportable | Needs listen policy and a productized entry point |
| A subnet egressing via a certain Agent | Partially supported | CIDR allow-list and per-agent egress policy already supported, but still not transparent routing |
| True SD-WAN / transparent L3 networking | Not supported currently | Integrate WireGuard or Headscale/Tailscale |

## Design Principles

- KISS: do the Agent Egress Gateway first, solving remote control and ops access, rather than developing an L3 data plane in-house.
- YAGNI: only introduce WireGuard/Headscale or similar data planes when transparent networking is clearly needed.
- DRY: remote control, VNC, and port proxying should share the same target validation and auditing logic.
- SOLID: the Server handles policy, tickets, auditing, and orchestration; the Agent handles connecting to targets and protocol execution; do not make the Agent bear global policy decisions directly.
