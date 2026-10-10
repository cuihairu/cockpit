[English](README.md) | [中文](README.zh.md)

<p align="center">
  <img src="web/public/logo.svg" alt="Cockpit Logo" width="120" height="120">
</p>

<h1 align="center">Cockpit</h1>

A personal hybrid infrastructure console that consolidates resources scattered across local machine rooms, cloud VPS instances, and nodes behind NAT into a single lightweight Server + Agent control plane.

[![Test](https://img.shields.io/github/actions/workflow/status/cuihairu/cockpit/test.yml?branch=main&logo=github&label=Test)](https://github.com/cuihairu/cockpit/actions/workflows/test.yml)
[![Docs](https://img.shields.io/github/actions/workflow/status/cuihairu/cockpit/docs.yml?branch=main&logo=github&label=Docs)](https://github.com/cuihairu/cockpit/actions/workflows/docs.yml)
[![codecov](https://codecov.io/gh/cuihairu/cockpit/branch/main/graph/badge.svg)](https://codecov.io/gh/cuihairu/cockpit)
[![Agent: Windows](https://img.shields.io/badge/platform-Windows-0078D4?logo=windows&logoColor=white)](https://cuihairu.github.io/cockpit/guide/getting-started)
[![Agent: Linux](https://img.shields.io/badge/platform-Linux-FCC624?logo=linux&logoColor=black)](https://cuihairu.github.io/cockpit/guide/getting-started)
[![Agent: macOS](https://img.shields.io/badge/platform-macOS-000000?logo=apple&logoColor=white)](https://cuihairu.github.io/cockpit/guide/getting-started)
[![Agent: x86_64](https://img.shields.io/badge/arch-x86__64-5B5B5B)](https://cuihairu.github.io/cockpit/guide/getting-started)
[![Agent: aarch64](https://img.shields.io/badge/arch-aarch64-5B5B5B)](https://cuihairu.github.io/cockpit/guide/getting-started)

## Features

The console brings resources, execution, and remote access together in one place. On the resource side, a unified inventory view is synced from Inventory YAML; probe heartbeats, expiration alerts, and configuration drift detection all run on this data, with alerts delivered through four channels: Herald, ntfy, webhook, and Telegram. On the execution side, a fleet-wide Job ledger dispatches commands to any online agent with trackable status, exit codes, and captured output; Docker containers get full lifecycle management, and applications deploy as Stacks from compose files. On the remote access side, terminal, VNC, and desktop sessions go through short-lived ticket forwarding with session recording; reverse proxy sites, ACME certificates, DNS/DDNS, backup and restore, and service/Cron/SMART/NAS/network observability are all operated from the same console.

Agents register by dialing out over WebSocket, so nodes behind NAT never need to expose any inbound ports.

## Quick Start

> Prerequisite: a running Cockpit Server first (a single Docker command; the deploy host needs no build toolchain):

```bash
cp deployments/docker/.env.example deployments/docker/.env && vi deployments/docker/.env
docker compose -f deployments/docker/docker-compose.yml up -d
```

### Linux

1. Download and install (architecture auto-detected, fetched anonymously from daily builds, verified automatically after install):

   ```bash
   curl -fsSL https://raw.githubusercontent.com/cuihairu/cockpit/main/install.sh | bash
   ```

2. Connect to the server (the example uses this site's domain; replace it with your own Cockpit server address):

   ```bash
   cockpit-agent start -server wss://cockpit.cuihairu.site/ws -region home -zone datacenter
   ```

3. Verify the installation:

   ```bash
   cockpit-agent --version
   ```

   Expected output on success:

   ```text
   Cockpit Agent v20261001
   ```

   (The version number is the date of installation; `Registered as agent: ...` in the agent log means the server connection is up.)

### macOS

1. Download and install (same `install.sh` as Linux; detects Darwin and arm64/amd64 automatically):

   ```bash
   curl -fsSL https://raw.githubusercontent.com/cuihairu/cockpit/main/install.sh | bash
   ```

2. Connect to the server:

   ```bash
   cockpit-agent start -server wss://cockpit.cuihairu.site/ws -region home -zone datacenter
   ```

3. Verify the installation:

   ```bash
   cockpit-agent --version
   ```

   Expected output on success:

   ```text
   Cockpit Agent v20261001
   ```

### Windows

**Graphical installer (recommended)**: download `cockpit-agent-setup-nightly.exe` from the [nightly release](https://github.com/cuihairu/cockpit/releases/latest) (the identically named `.sha256` file is for verification) and run it by double-click.
It installs to `%ProgramFiles%\Cockpit Agent`, creates Start menu and desktop shortcuts (with the agent icon), asks for the Server address in the wizard, offers an optional "Register Windows service with auto-start on boot" checkbox, and registers a standard uninstall entry in Control Panel. For silent installation:

```powershell
.\cockpit-agent-setup-nightly.exe /VERYSILENT /SUPPRESSMSGBOXES /NORESTART /SERVER=wss://cockpit.cuihairu.site/ws
```

Service management: `cockpit-agent service install|uninstall|start|stop|status` (service name
`CockpitAgent`, log at `%ProgramData%\CockpitAgent\agent.log`).

**PowerShell script path** (no administrator required; installs into the current user's directory and adds it to the user PATH):

1. Download and install:

   ```powershell
   irm https://raw.githubusercontent.com/cuihairu/cockpit/main/install.ps1 | iex
   ```

2. Connect to the server (open a new terminal so the PATH takes effect):

   ```powershell
   cockpit-agent start -server wss://cockpit.cuihairu.site/ws -region home -zone datacenter
   ```

3. Verify the installation:

   ```powershell
   cockpit-agent --version
   ```

   Expected output on success:

   ```text
   Cockpit Agent v20261001
   ```

## Documentation

Full installation details (one-line installer internals / platform matrix / building from source / OpenWrt ipk), configuration reference, architecture, and per-feature design docs live on the documentation site:

- [Introduction](https://cuihairu.github.io/cockpit/guide/introduction)
- [Quick Start](https://cuihairu.github.io/cockpit/guide/getting-started)
- [Architecture and Boundaries](https://cuihairu.github.io/cockpit/guide/architecture)
- [Protocol and API Boundaries](https://cuihairu.github.io/cockpit/guide/protocol)
- [Docker Deployment](https://cuihairu.github.io/cockpit/operations/deploy-docker)
- [Open-Source Ops Panels Survey](https://cuihairu.github.io/cockpit/research/ops-oss-survey)
- Agent manual deployment and service unit examples: [deployments/README.md](deployments/README.md)

## Foundations

This repository is built on open-source components; what is written here is the control plane wiring and the provider layers: Server and Agent are in Go, storage goes through GORM into SQLite, and the realtime channel runs on gorilla/websocket; the web frontend is React 19 + Ant Design with xterm.js for terminal rendering; desktop and VNC/SSH remote access are backed by Apache Guacamole (guacd server + guacamole-common-js 1.5.0 client), and the built-in terminal's SSH uses golang.org/x/crypto/ssh; certificate issuance uses go-acme/lego (DNS-01), the mobile app is Flutter, the docs site is VitePress, and the Windows installer is packaged with Inno Setup.

## License

Apache License 2.0
