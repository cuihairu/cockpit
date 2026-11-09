# Core Concepts and Source Glossary

Cockpit's concepts are documented across four pages: this page holds the resource model and the source-linked glossary; runtime state, the inventory declaration source, and security boundaries each have their own page.

- [Runtime State](/guide/runtime) — data planes, Agent capability model, Server Registry, and remote access paths.
- [Inventory Declaration Source](/guide/inventory) — the Inventory YAML schema, sync semantics, and differences from historical designs.
- [Security](/guide/security) — authentication, secrets, and remote-connection boundaries.

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

## Glossary

Each entry states the abstract responsibility, then points to the code fact that owns it. Architecture sections for these layers are in [Architecture and Boundaries](/guide/architecture#source-layers).

| Term | English | Definition | Evidence |
| --- | --- | --- | --- |
| 进程外壳 | Process shell | Parses CLI commands, selects configuration, and maps startup arguments for Server/Agent/probe processes. | `cmd/cockpit/main.go:22`, `cmd/cockpit-agent/main.go:75` |
| 运行时组合 | Runtime composition | Owns process context, dependency construction, background task accounting, subsystem start/stop, and graceful DB shutdown. | `internal/server/server.go:126`, `internal/server/server.go:204`, `internal/server/server.go:411` |
| Agent 会话注册表 | Agent session registry | Holds currently connected Agent objects, capability snapshots, send queues, heartbeat presence, and pending RPC response channels; it is not durable inventory. | `internal/server/registry.go:11`, `internal/server/agent.go:15` |
| 控制协议 | Control protocol | Defines typed WebSocket envelopes for registration, heartbeat, RPC, proxy data, probe reports, liveness, and execution traces. | `internal/protocol/message.go:8`, `internal/protocol/codec.go:19` |
| Agent RPC 路由 | Agent RPC router | Parses `<provider>.<action>`, selects the registered provider, invokes one action, returns an ID-correlated response, and records real execution. | `internal/agent/rpc/router.go:70`, `internal/server/server.go:503` |
| 控制 API | Control API | Authenticates REST callers and translates requests into storage operations, Agent RPC, streaming tickets, or session management. | `internal/server/api.go:27`, `internal/server/server.go:283` |
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
