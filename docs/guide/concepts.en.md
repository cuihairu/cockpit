# Core Concepts

> Language: [中文](./concepts.md) ｜ **English**

## Resource Model

Cockpit uses Region, Zone, Agent, and resource objects to describe personal infrastructure:

```text
Region
  Zone
    Agent
      ComputeInstance / Service / Gateway / Storage

Domain
  Certificate
```

| Concept | Description |
| --- | --- |
| Region | A region or logical grouping, e.g. `home`, `jiangsu-huaian` |
| Zone | A data center, network segment, or availability zone within a Region |
| Agent | The execution agent running on a node, connects to the Server proactively |
| ComputeInstance | Compute instances such as VMs, containers, physical machines |
| Service | Services such as HTTP, TCP, databases |
| Domain | A domain name and its registration info |
| Certificate | TLS certificates, usually attached to a Domain |
| Gateway | Gateways such as OpenWrt, pfSense |
| Storage | Storage such as NFS, iSCSI, local disks, Ceph |

## Configuration and State

Cockpit currently has two main data planes:

```text
Inventory YAML
    |
    | cockpit sync
    v
SQLite
    ^
    | heartbeat / runtime update
    |
Agent
```

Inventory YAML describes "which assets we want the system to know about". SQLite holds the runtime view needed by the API and Web UI queries, including synced resources, users, alerts, audits, proxy configuration, system metrics, and Agent online status.

Notes:

- `cockpit sync` is the manual inventory sync entry point.
- Server hot-reload with `inventory.watch: true` reuses the same Syncer, syncing Agent, Domain, Certificate, ComputeInstance, Service, Gateway, Storage, and other resources.
- `inventory.strict: true` makes the Server strictly validate the inventory at startup; with the default `false`, startup-failure risks degrade to logs.
- Agent heartbeats report online status and system metrics, but never modify the inventory file directly.

## Inventory File

The current main path is a single-file YAML:

```yaml
version: v1

metadata:
  name: "My Home Lab"
  description: "Personal hybrid infrastructure"

regions:
  home:
    name: "Home"
    zones:
      datacenter:
        name: "Data Center"
        agents:
          server01:
            name: "Server 01"
            hostname: "server01.home.local"
            ip: "192.168.1.10"
            capabilities:
              - docker
              - system

domains:
  example-local:
    domain: "example.local"
    provider: "internal"

computeInstances:
  vm-web01:
    name: "Web Server VM"
    type: "vm"
    agent: "server01"
    region: "home"
    zone: "datacenter"
    cpu: 2
    memory: 2048
    disk: 40

services:
  web-service:
    name: "Web Service"
    type: "http"
    agent: "server01"
    url: "http://192.168.1.10:8080"
    interval: 60
```

Top-level fields currently include:

- `version`
- `metadata`
- `regions`
- `domains`
- `computeInstances`
- `services`
- `gateways`
- `storages`
- `resources`
- `templates`

`resources` / `templates` are reserved in the schema, but the main path synced to the database is the explicit resource fields such as `computeInstances`, `services`, `gateways`, `storages`.

The `region` / `zone` fields on a resource can be omitted: leaving them empty does not block syncing; the database marks them uniformly as `unknown` (explicit values take priority; for `services`/`gateways`/`storages` the region goes into their respective labels, and keys the user explicitly provides in labels are not overwritten by empty fields). Agent registration follows the same semantics — defaulting to `unknown` when not provided (D-2026-10-08-1).

## Agent Capabilities

At startup the Agent runs capability detectors and reports the capabilities in the registration message:

```json
{
  "type": "docker-api",
  "endpoint": "unix:///var/run/docker.sock"
}
```

There are currently 17 capabilities in total, from two classes of origins.

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

## Security Concepts

| Boundary | Current implementation |
| --- | --- |
| User authentication | JWT Bearer token |
| Admin initialization | `ADMIN_USERNAME` optional, `ADMIN_PASSWORD` required |
| Two-factor verification | TOTP, secret encrypted with `TOTP_ENCRYPTION_KEY` |
| Agent authentication | Agents with a configured secret must present a matching secret at registration |
| WebSocket Origin | Controlled by `ALLOWED_ORIGINS`; unset is fine for development but not for production |
| Remote connections | 5-minute, single-use ticket |

## Differences from Historical Designs

The repository contains some design documents describing a more complete Git-first, directory-split inventory and third-party integration vision. The current implementation already has inventory sync, the Server/Agent control plane, resource queries, monitoring, authentication, auditing, alerts, and remote connections, but the code facts described in this document and [Architecture and Boundaries](/guide/architecture) remain authoritative.
