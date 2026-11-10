# Runtime State

Cockpit's runtime state is the SQLite-backed view queried by the API and Web UI, plus the live Agent connections registered on the Server. The declaration plane that feeds SQLite is described in [Inventory Declaration Source](/guide/inventory); security boundaries are summarized in [Security](/guide/security).

## Agent Capabilities

At startup the Agent runs capability detectors and reports the capabilities in the registration message:

```json
{
  "type": "docker-api",
  "endpoint": "unix:///var/run/docker.sock"
}
```

There are currently 19 capabilities in total, from two classes of origins. Only `file`, `ddns`, and `oob` are always reported; the remaining 16 are conditionally reported when their platform/environment conditions are met.

Probed (`internal/agent/detector/`, reported only when conditions are met):

| Capability | Detection basis |
| --- | --- |
| `openwrt` | `/etc/openwrt_release`, `ubus`, `opkg` |
| `pve-api` | `PVE_URL` or common PVE API addresses |
| `docker-api` | `DOCKER_HOST` or common Docker sockets |
| `hardware-monitor` | `smartctl`, temperature sensors, UPS tools |
| `firewall` | `nft` or `iptables` available (LookPath + `--version` verification) |
| `overlay` | Any of `zerotier-cli` / `tailscale` / `wg` / `frp` available |
| `remote-services` | Probing common remote-access ports on the local machine (SSH 22 / RDP 3389 / VNC 590x / Telnet 23 / FTP 21) |

Directly reported in code (`agent.go` `detectCapabilities`, reported when platform/environment conditions are met):

| Capability | Report condition |
| --- | --- |
| `backup` | Linux (standard-library archiving, zero external dependencies) |
| `file` | Always reported (standard library works on all platforms) |
| `nginx-proxy` | nginx executable and version detected |
| `traefik-proxy` | Traefik file-provider dynamic directory detected |
| `cron` | `crontab` command available |
| `logs` | At least one of `journalctl` or `docker` available |
| `drift` | Any of `nginx-proxy` / `traefik-proxy` / `cron` / `docker-api` present |
| `ddns` | Always reported (standard library outbound HTTP to probe the public IP) |
| `service` | One of Windows (SCM) / macOS (launchd) / systemd |
| `nas` | Software RAID (`/proc/mdstat`) or any storage tool present |
| `oob` | Always reported (out-of-band fallback action surface, WOL magic packet, `agent.go` `detectCapabilities`) |
| `virt-libvirt` | Local `virsh` entry present (`rpc.LibvirtAvailable()`), read-only; real driver probe deferred to the `virt.capabilities` RPC |

> Historical note: the old `network-monitor` capability (WireGuard peer counts and cloudflared
> process-level probing) was removed when overlay M1 landed (overlay-design.md D18); the server/web
> had no consumers.

The Agent also collects basic system metrics and reports them via heartbeats, including CPU, memory, disk, network, system version, architecture, load, and uptime.

## Server Registry

The in-memory Server Registry represents only the currently online connections:

- Agents that register successfully enter the Registry.
- Active connections with a duplicate Agent ID are rejected.
- RPC, proxy, remote terminal, and desktop sessions all rely on the Registry to find online Agents.
- After a Server restart the Registry is emptied and Agents need to reconnect and register.

An Agent record in SQLite represents history and resource associations, and is not the same as a "currently online connection".

## Remote Access

Remote access is not the Browser connecting directly to intranet targets; instead:

```text
Browser
  -> Server WebSocket
  -> Agent WebSocket control channel
  -> target host:port
```

Both terminal and desktop connections use `proxy_*` messages for TCP data forwarding: the terminal connects directly to the target; RDP/VNC/SSH desktops are relayed via Guacamole (the `guac:` prefix relays to guacd, see [Remote Desktop Guacamole Design](/remote-desktop-guacamole-design)).

Remote targets by default must match hostnames, IPs, or CIDRs explicitly configured in `remote_control.allowed_targets`; development/debugging environments can set `remote_control.allow_arbitrary_target: true` to skip that restriction. The current implementation does not derive remotely controllable targets from the inventory automatically.

RDP/VNC/SSH desktop protocols are terminated by guacd; the Agent only does TCP forwarding — the Agent has no special build-tag requirements, and the default build works on any platform.

The remote-connection Browser WebSocket uses short-lived ticket authentication. See [Protocols and API Boundary](/guide/protocol) for details.
