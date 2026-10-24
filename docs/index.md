---
layout: home

hero:
  name: Cockpit
  text: Personal hybrid infrastructure console
  tagline: Server + Agent for a lightweight resource view, monitoring and remote access
  actions:
    - theme: brand
      text: Quick start
      link: /guide/getting-started
    - theme: alt
      text: Architecture and boundaries
      link: /guide/architecture

features:
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" width="24" height="24"><rect x="3" y="7" width="18" height="13" rx="2"/><path d="M3 11h18"/><path d="M9 7V5a3 3 0 0 1 6 0v2"/></svg>'
    title: Unified resource view
    details: Manage Agents, compute instances, domains, certificates, services, gateways and storage through Inventory YAML and SQLite
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" width="24" height="24"><circle cx="12" cy="12" r="9"/><path d="M3 12h18"/><path d="M12 3a14 14 0 0 1 0 18"/><path d="M12 3a14 14 0 0 0 0 18"/></svg>'
    title: Agent-initiated connection
    details: Agents dial out to the Server over WebSocket, which suits nodes behind NAT and cross-region topologies
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" width="24" height="24"><path d="M4 20V10"/><path d="M10 20V4"/><path d="M16 20v-8"/><path d="M22 20H2"/></svg>'
    title: System metrics
    details: Agents report CPU, memory, disk, network and system info by heartbeat; the Server keeps history and snapshots
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" width="24" height="24"><rect x="4" y="10" width="16" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3"/><circle cx="12" cy="15.5" r="1.5"/></svg>'
    title: Auth and audit
    details: Admin bootstrap, JWT, TOTP, password reset, audit log and per-Agent secret
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" width="24" height="24"><rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8 20h8"/><path d="M12 16v4"/><path d="m8 8 2.5 2L8 12"/><path d="M13 12h4"/></svg>'
    title: Remote connection
    details: Terminal, VNC and desktop sessions use short-lived tickets, forwarded through Server and Agent to the target service
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" width="24" height="24"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M7 9l3 3-3 3"/><path d="M13 15h4"/></svg>'
    title: Unified Job execution
    details: 'One Job ledger across machines: run a command against any online Agent, with status, exit code and output written back on record'
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" width="24" height="24"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M12 4v16"/><path d="M6 9h3"/><path d="M15 15h3"/></svg>'
    title: Clear boundaries
    details: The Server owns the control plane and persistence; the Agent owns node-side collection, proxying and execution
  - icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" width="24" height="24"><path d="M14 4h4a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-4"/><path d="M10 8l4 4-4 4"/><path d="M14 12H3"/></svg>'
    title: Agent egress
    details: Documents the capability boundary of the Agent as an intranet egress point and the gap versus a full SD-WAN
---


Every page outside `docs/archive/` has an English twin; each Chinese page carries a language switcher. The
historical plans and specs under `docs/archive/` remain Chinese-only.

## Start here

- [Quick start](/guide/getting-started)
- [Architecture and boundaries](/guide/architecture)
- [Core concepts](/guide/concepts)
- [Protocol definitions](/guide/protocol)
- [Architecture decision log](/guide/decision-log)

## Guide

- [Introduction](/guide/introduction) ｜ [Acceptance checklist](/guide/acceptance-checklist)
- [Service management (systemd + Windows SCM + launchd)](/guide/service-design) ｜ [Service health probe and self-healing](/guide/service-health-design)
- [Backup management](/guide/backup-design) ｜ [Server backup and remote archiving](/guide/server-backup-design) ｜ [File manager](/guide/file-manager-design)
- [Job execution](/guide/jobs-design) ｜ [Workflow and asynchronous Job](/guide/workflow-design) ｜ [Cron management](/guide/cron-design)
- [Drift detection](/guide/drift-design) ｜ [Logs](/guide/logs-design)
- [DNS management](/guide/dns-design) ｜ [Domain binding](/guide/domain-binding-design)
- [Reverse proxy management](/guide/proxy-design) ｜ [Firewall (read-only observation)](/guide/firewall-design) ｜ [NAS management](/guide/nas-design)
- [ACME certificates](/guide/acme-design) ｜ [DDNS](/guide/ddns-design)
- [Disk health (SMART)](/guide/disk-health-design) ｜ [Recording and replay](/guide/recording-design)
- [Overlay networking (ZeroTier / Tailscale / WireGuard / frp)](/guide/overlay-design) ｜ [Stack deployment](/guide/stack-deploy-design)
- [Mobile plan: Flutter](/guide/mobile-design) ｜ [RBAC](/guide/rbac-design)
- [Vault and persona mapping](/guide/vault-design) ｜ [Persona to vault mapping](/guide/persona-vault-mapping)
- [Agent egress and SD-WAN](/guide/agent-egress-sdwan) ｜ [Reference projects](/guide/reference-projects)
- [OpenWrt Agent packaging](/guide/openwrt-agent-design) ｜ [Windows Agent packaging](/guide/windows-agent-design)
- [Probe enhancement](/guide/probe-enhance-design)

## Design, operations and research

- [agent-core three-layer architecture](/design/agent-core-architecture)
- [BMC / iBMC out-of-band management](/design/bmc-capabilities)
- [Hypervisor platform capabilities](/design/hypervisor-capabilities)
- [Service detect agent (healthprobe)](/design/service-detect-agent)
- [External remote session API contract](/design/external-session-api)
- [Docker deployment](/operations/deploy-docker)
- [Ops open-source survey](/research/ops-oss-survey)

## Root documents

- [WebSocket authentication boundary](/websocket-token-auth)
- [Agent ID generation policy](/agent-id)
- [Agent configuration reference](/agent-config)
- [guacd deployment guide](/guacd-deploy)
- [Remote desktop design: Apache Guacamole integration](/remote-desktop-guacamole-design)
- [Remote access integration design: unified VNC / SSH / RDP stack](/remote-access-integration-design)
- [UI theme and skin system](/ui-theme-design)
- [Documentation–source consistency audit (2026-10-08)](/audit-doc-consistency)