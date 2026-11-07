# Core Concepts and Source Glossary


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

## Glossary

Each entry states the abstract responsibility, then points to the code fact that owns it. Architecture sections for these layers are in [Architecture and Boundaries](/guide/architecture#source-layers).

| Term | English | Definition | Evidence |
| --- | --- | --- | --- |
| 进程外壳 | Process shell | Parses CLI commands, selects configuration, and maps startup arguments for Server/Agent/probe processes. | `cmd/cockpit/main.go:22`, `cmd/cockpit-agent/main.go:75` |
| 运行时组合 | Runtime composition | Owns process context, dependency construction, background task accounting, subsystem start/stop, and graceful DB shutdown. | `internal/server/server.go:126`, `internal/server/server.go:204`, `internal/server/server.go:411` |
| Agent 会话注册表 | Agent session registry | Holds currently connected Agent objects, capability snapshots, send queues, heartbeat presence, and pending RPC response channels; it is not durable inventory. | `internal/server/registry.go:11`, `internal/server/agent.go:15` |
| 控制协议 | Control protocol | Defines typed WebSocket envelopes for registration, heartbeat, RPC, proxy data, probe reports, liveness, and execution traces. | `internal/protocol/message.go:8`, `internal/protocol/codec.go:19` |
| Agent RPC 路由 | Agent RPC router | Parses `<provider>.<action>`, selects the registered provider, invokes one action, returns an ID-correlated response, and records real execution. | `internal/agent/rpc/router.go:70`, `internal/server/server.go:503` |
| 控制 API | Control API | Authenticates REST callers and translates requests into storage operations, Agent RPC, streaming tickets, or session management. | `internal/server/api.go:31`, `internal/server/server.go:283` |
| 远控会话 | Remote-control session | Tracks a browser-mediated live terminal/desktop path and converts it into `proxy_*` frames over the Agent control channel. | `internal/server/remote_session.go:43`, `internal/server/server.go:668` |
| 远控票据 | Remote-control ticket | Grants a five-minute, single-use bridge from a JWT-authenticated REST request to a browser WebSocket. | `internal/server/ticket.go:26`, `internal/server/ticket.go:41` |
| 代理数据面 | Proxy data plane | Bridges a listener or browser socket to an Agent-targeted TCP connection using `proxy_new`, `proxy_data`, and `proxy_close`; routes protocol-prefixed pseudo streams. | `internal/proxy/mgr/manager.go:26`, `internal/server/server.go:668` |
| 作业与编排 | Jobs and orchestration | Persists a Job or WorkflowRun, then asynchronously creates auditable Agent attempts, retries steps, resolves outputs, and freezes active definitions. | `internal/server/api_jobs.go:63`, `internal/server/api_workflow.go:484`, `internal/server/api_workflow.go:682` |
| 定时调度 | Scheduled execution | Reapplies stored intent through scanner loops or periodic runners without retaining request context. | `internal/server/server.go:293`, `internal/probe/runner.go:185`, `internal/server/backup_loop.go:101` |
| Inventory 声明源 | Inventory declaration source | Parses declarative YAML and reconciles it into SQLite through one Syncer used by CLI and hot reload. | `internal/inventory/schema.go:1`, `internal/sync/watcher.go:57`, `internal/sync/watcher.go:129` |
| 业务持久态 | Business persistence | Owns GORM models, automatic migration, and SQLite-backed CRUD for resources, users, jobs, workflows, metrics, and audit rows. | `internal/storage/storage.go:40`, `internal/storage/storage.go:102` |
| 秘密保管 | Secret vault | Separately unlocks and encrypts credential entries, keeping plaintext out of ordinary API responses and audit logs. | `internal/server/server.go:140`, `internal/vault/local.go:122`, `internal/vault/local.go:145` |
| Agent 能力探测 | Agent capability detection | Registers detectors, discovers node/platform/integration features, and reports capability metadata without binding credentials. | `internal/agent/detector/detector.go:10`, `internal/agent/providers.go:18` |
| 执行 Provider | Execution provider | Executes one local or locally reachable action type—files, jobs, Docker, PVE, services, logs, stacks, NAS, firewall, overlay—inside the Agent. | `internal/agent/provider/provider.go:4`, `internal/agent/rpc/router.go:70`, `internal/agent/providers.go:33` |
| 基础运行时 | Agent core runtime | Supplies reusable registration, heartbeat, reporting, reconnect, metrics, platform, and execution-trace primitives. | `core/register/register.go:1`, `core/heartbeat/heartbeat.go:1`, `core/report/upstream.go:1`, `core/platform/host.go:1` |
| 云集成 | Cloud integration | Builds vendor adapters from stored accounts, declares independent capabilities, and records asynchronous operations for discovery and lifecycle. | `internal/cloud/registry.go:12`, `internal/cloud/provider.go:13`, `internal/server/api_cloud.go:1` |
| DNS/证书集成 | DNS and certificate integration | Manages DNS records, ACME issuance, renewals, and deployment targets with provider-specific credentials. | `internal/dns/alidns.go:1`, `internal/server/acme_issuer.go:1`, `internal/server/acme_scan.go:1` |
| Overlay 集成 | Overlay integration | Reads or updates ZeroTier/Tailscale networks and reconciles local overlay identities. | `internal/overlay/zerotier.go:1`, `internal/overlay/tailscale.go:1`, `internal/agent/providers.go:214` |
| 通知扇出 | Notification fanout | Applies event whitelists, builds enabled channels, and performs short-timeout synchronous or process-scoped asynchronous delivery. | `internal/notification/service.go:33`, `internal/notification/service.go:68`, `internal/server/server.go:297` |
| SPA 交付 | SPA delivery | Serves hashed static assets with an index fallback from the same Server process that owns `/api` and `/ws`. | `internal/server/web.go:12`, `internal/server/server.go:389` |
| Web 客户端状态 | Web client state | Owns routes, permission-driven menus, JWT, presentation preferences, and browser session sockets; never becomes a system source of truth. | `web/src/App.tsx:120`, `web/src/App.tsx:583`, `web/src/services/api.ts:122` |
| Mobile 客户端状态 | Mobile client state | Stores non-sensitive settings in preferences and the auth token in secure storage, then uses the same `/api` runtime plane. | `mobile/lib/state/settings.dart:10`, `mobile/lib/state/settings.dart:117`, `mobile/lib/api/client.dart:38` |
