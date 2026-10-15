# Cockpit

> Language: [中文](./index.md) ｜ **English**

A personal hybrid infrastructure console: one lightweight Server + Agent control plane that consolidates resources scattered across home server rooms, cloud VPS instances, and nodes behind NAT.

**Caveat on language coverage:** this page and the documents in the `docs/` root directory are translated. The `guide/`, `design/`, `operations/`, `research/` and `archive/` sections are currently Chinese-only — each Chinese page carries a language switcher once its English twin exists.

## What it does

- **Unified resource view** — manage Agents, compute instances, domains, certificates, services, gateways and storage through Inventory YAML and SQLite.
- **Agent-initiated connection** — the Agent dials out to the Server over WebSocket, which suits nodes behind NAT and cross-region topologies.
- **System metrics** — the Agent reports CPU, memory, disk, network and system info by heartbeat; the Server keeps history and snapshots.
- **Auth and audit** — admin bootstrap, JWT, TOTP, password reset, audit log and per-Agent secret.
- **Remote access** — terminal, VNC and desktop sessions use short-lived tickets and are forwarded through Server and Agent to the target service.
- **Unified Job execution** — a single Job ledger across machines: run a command against any online Agent, with status, exit code and output written back on record.
- **Clear boundaries** — the Server owns the control plane and persistence; the Agent owns node-side collection, proxying and execution.
- **Agent egress** — documents the capability boundary of the Agent as an intranet egress point and the gap versus a full SD-WAN.

## Start here

- [Quick start](/guide/getting-started)
- [Architecture and boundaries](/guide/architecture)
- [Protocol definitions](/guide/protocol)

## Root documents

- [WebSocket authentication boundary](./websocket-token-auth.en.md)
- [Agent ID generation policy](./agent-id.en.md)
- [Agent configuration reference](./agent-config.en.md)
- [guacd deployment guide](./guacd-deploy.en.md)
- [Remote desktop design: Apache Guacamole integration](./remote-desktop-guacamole-design.en.md)
- [Remote access integration design: unified VNC / SSH / RDP stack](./remote-access-integration-design.en.md)
- [UI theme and skin system](./ui-theme-design.en.md)
- [Documentation–source consistency audit (2026-10-08)](./audit-doc-consistency.en.md)
