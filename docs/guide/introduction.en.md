# Introduction

> Language: [中文](./introduction.md) ｜ **English**

Cockpit is a console for personal hybrid infrastructure. The current implementation focuses on three things:

- Establishing a unified resource view with Inventory YAML and SQLite.
- Managing NAT'd or geographically distributed nodes through Agents that connect outbound to the Server.
- Viewing resources, monitoring, alerts, and audits in the Web UI, and initiating remote terminal, VNC, RDP, and other connections.

## Current capabilities

The control-plane foundation is complete: the Server has a Web UI, HTTP API, Agent WebSocket intake, TOTP two-step verification, RBAC users and roles, and audit logs, with data in SQLite; Agents connect out to register, reporting heartbeats, system metrics, and capability tags, so NAT'd nodes need no inbound ports. `cockpit sync` syncs Inventory YAML into the runtime resource view — all six resource rows (compute instances, domains, certificates, services, gateways, storage) carry probe heartbeat bars and expiry alerts.

For remote operations, terminal / VNC / desktop go through short-lived ticket forwarding, with session recording archived and replayable; files can be browsed and transferred on agents; logs can be tailed on a single machine (journalctl / Docker) and searched federated across machines. Applications and containers are fully managed by Docker (containers, images, networks, volumes); Stacks deploy via compose with a template library and deployment history; restart/pull are operated locally. 

Backups cover both sides: on the agent side, scheduled packing, retention policies, and two-stage safe recovery, with artifacts pushed off-site via rclone and re-pushable; on the server side, `VACUUM INTO` backs up the panel database on schedule, likewise with off-site push. The Web side manages reverse proxy sites (nginx / Traefik dual backends, syntax self-check, rollback on failure); certificates use ACME issuance with automatic renewal (Cloudflare / DNSPod / Alibaba Cloud DNS-01), and issued artifacts are automatically deployed to configured paths on agents; DNS record management covers eight record types (A/AAAA/CNAME/MX/TXT/NS/SRV/CAA) across three providers, plus DDNS dynamic domains and a domain/certificate expiry ledger.

Host operations cover service management (systemd / Windows SCM / macOS launchd — three backends), Cron jobs, SMART disk inspection, NAS observation (DSM / TrueNAS / OMV, local mdadm / ZFS), and network observation and management (WireGuard / ZeroTier / Tailscale / frp runtime state, cloud enrollment, and joining/leaving networks and daemon service start/stop on this machine). Configuration drift detection compares what the panel dispatched against what is on disk; CMDB consistency compares declarations against actual reports; alerts go out through four channels: Herald / ntfy / webhook / Telegram.

## Applicable scenarios

Cockpit suits personal or small-scale environments:

- Mixed management of a home rack, a local rack, and cloud VPSes.
- Multiple nodes behind NAT that can only connect outbound.
- Wanting to describe hosts, services, domains, certificates, and storage in a single inventory.
- Wanting to see resource status, system metrics, and remote access entries in one Web UI.

Cockpit is currently not a large-scale multi-tenant CMDB, nor a Kubernetes control plane. It is more of a lightweight console for personal infrastructure.

## Architecture overview

```text
Browser
  |
  | /api, /api/remote/*
  v
Cockpit Server
  |-- SQLite
  |-- Web UI static files
  |-- Agent Registry
  |
  | /ws
  v
Cockpit Agent
  |-- local metrics
  |-- Docker / PVE / OpenWrt detection
  |-- TCP proxy / terminal / VNC / desktop target access
```

Core principles:

- The Web UI only talks to the Server.
- Agents connect outbound to the Server.
- The Server maintains the global runtime view.
- Agents handle node-side probing and execution.
- Inventory is the declarative input; SQLite is the runtime query surface.

See [architecture and boundaries](/guide/architecture) for the full boundary.

## Relationship to other tools

Cockpit does not require replacing existing consoles. The current code already has PVE, Docker, and OpenWrt clients and Agent capability detection; over the long term these capabilities can be orchestrated into a unified workbench.

For now, understand it as follows:

| Tool/platform | Cockpit's current relationship |
| --- | --- |
| PVE | Has an API client and Agent capability detection; control-plane integration still needs confirmation per specific feature |
| Docker | Has an API client, capability detection, and resource-view foundations |
| OpenWrt | Has client and capability-detection foundations |
| Remote terminal/VNC/RDP | Reaches intranet targets proxied through the Agent |
| Git | Inventory YAML can be managed by Git, but the current application still treats `cockpit sync` writing into SQLite as authoritative |

## Documentation reading path

1. [Getting started](/guide/getting-started)
2. [Core concepts](/guide/concepts)
3. [Architecture and boundaries](/guide/architecture)
4. [Agent egress and SD-WAN capability boundaries](/guide/agent-egress-sdwan)
5. [Protocol and API boundaries](/guide/protocol)
