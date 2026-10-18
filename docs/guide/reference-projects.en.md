# Reference Project Comparison and Takeaways

> Language: [中文](./reference-projects.md) ｜ **English**

This document records comparative analyses of Cockpit against similar open-source projects, used for feature planning and design decisions. Compiled from public sources as of 2026-09; links in the sources section at the end.

Cockpit is positioned as a **personal hybrid infrastructure console**, with four main lines:

1. Git-first CMDB (inventory YAML → SQLite)
2. Agent-initiated outbound WebSocket (NAT friendly)
3. Runtime monitoring + alerts + health probing
4. Web console (terminal/desktop/VNC remote control + Docker management)

Seven benchmark projects were selected by capability domain. For each project we give pros and cons, distinguishing **directly adoptable** (concrete practices to copy) from **worth borrowing** (design ideas to absorb).

For the extended 2026-10 research (six monitoring dashboards + the liveness-collection route verdict, with live screenshots) see [Ops Open-Source Solutions Survey](/research/ops-oss-survey.en.md).

## Monitoring Domain: Beszel

[henrygd/beszel](https://github.com/henrygd/beszel) — MIT licensed, written in Go, hub + agent architecture, structurally identical to Cockpit's Server + Agent. As of 2026 it is the mainstream choice for homelab / small VPS cluster monitoring.

**Pros**

- Extremely lightweight: the agent takes only ~23MB RAM on each monitored server
- Single-binary deployment, up and running in minutes
- Docker container-level stats, historical data, alerts, and notification push all complete
- PocketBase foundation, simple to operate

**Cons**

- Monitoring only: no CMDB, no remote control, no management actions, no resource views
- Fixed data model, limited extensibility

**Worth borrowing**

- Agent resource usage is a hard metric: the Cockpit Agent should establish memory/CPU baseline measurements to avoid bloat
- Compact storage for historical metrics (Cockpit uses SQLite; its downsampling strategy is worth studying)
- The "works out of the box" deployment experience: one command to run, usable with default configuration

## Multi-Machine Container Management: Komodo

[Komodo (komo.do)](https://komo.do/docs/intro) — Core + Periphery architecture, fully identical in shape to Cockpit's Server + Agent, the most worthwhile multi-machine management reference to study.

**Pros**

- **Git-to-deploy workflow**: compose.yml lives in a Git repository; changes deploy automatically to target servers
- Build pipeline: image build and deployment integrated
- "Procedures": orchestrating common ops actions into reusable processes
- A single Core manages multiple Docker hosts

**Cons**

- Depends on MongoDB, heavy for a personal single-machine scenario
- Containers only: no CMDB, remote control, or desktop access

**Worth borrowing**

- A direct benchmark for roadmap P1 "app deployment": the pattern of pulling a Compose stack from Git + triggering up/down
- The "Procedures" concept: Cockpit already has the RPC path + probes, so orchestrating "restart stack + clean up images + health probe" into a reusable process is cheap

## Container Panels: Portainer / Dockge

[Portainer vs Dockge comparison](https://talos.tools/compare/portainer-vs-dockge), [OneUptime 2026 comparison](https://oneuptime.com/blog/post/2026-03-20-portainer-vs-dockge/view)

| | Portainer | Dockge |
| --- | --- | --- |
| Pros | Multi-host/Swarm/K8s, enterprise governance, mature ecosystem (~50MB memory) | compose-file-first philosophy, file as the source of truth, simple and intuitive for a single machine |
| Cons | Growing commercial heft, partially closed source | Single-machine limitation, weak multi-machine capability |

**Directly adoptable**

- The UI interaction blueprint of the Compose Stack: editor + status overview + up/down buttons + log entry completed on one screen. When Cockpit builds the P1 app deployment page it can follow this layout directly
- Dockge's compose-file-first: the YAML file itself is the configuration source, the UI is a view — consistent with Cockpit's Git-first philosophy

## Remote Access Gateway: Apache Guacamole

[Apache Guacamole](https://guacamole.apache.org/) — the clientless remote desktop gateway benchmark; the browser opens VNC/RDP/SSH directly.

**Pros**

- The guacd protocol translation layer decouples protocol handling from the web layer, a clean architecture
- Session recording, clipboard/file transfer channels
- Mature connection management and two-factor authentication

**Cons**

- Heavy Java stack, requires maintaining a separate guacd process
- Connections are statically configured, contrary to Cockpit's dynamic inventory philosophy

**Worth borrowing**

- **Session recording**: Cockpit audits currently record session start/end; terminal output recording could be added as an audit reinforcement
- File transfer channel: beyond roadmap P1's "file manager", a supplement for file ingress/egress on the remote-control side

## Access Security: Teleport

Teleport ([community comparison with Guacamole](https://www.reddit.com/r/selfhosted/comments/1im5ioy/how_to_connect_to_sshrdp_web_based_with_teleport/)) — an identity-centric access platform.

**Pros**

- Short-lived certificate access, RBAC, complete session recording
- Access request approval flow: high-risk operations require approval to proceed
- Session-granularity audit detail

**Cons**

- Enterprise-grade complexity and resource usage, too heavy for personal scenarios

**Directly adoptable**

- Cockpit's short-lived ticket + `Sec-WebSocket-Protocol` authentication chain follows the same thinking; **the design is validated as correct**

**Worth borrowing**

- Session-granularity audit detail (command-level recording is a long-term direction)
- High-risk switches like `allow_arbitrary_target` could be upgraded to a "one-time approval" mode

## CMDB: NetBox

[NetBox Labs: the source of truth philosophy](https://netboxlabs.com/blog/the-long-war-against-configuration-chaos/), [NetBox GitOps integration](https://netodata.io/netbox-integration-connecting-dcim-ipam-with-enterprise-infrastructure/) — the most mature declarative source of truth in the DCIM/IPAM space.

**Pros**

- The most mature "declarative source of truth" paradigm: API-first, separation of desired and actual state
- Complete GitOps integration ecosystem with Terraform / Ansible
- Fine-grained resource modeling: tags, custom fields, resource relations

**Cons**

- Python/Django stack, heavy DCIM/IPAM domain model, unsuitable for lightweight personal single-machine scenarios

**Worth borrowing**

- The inventory schema field modeling can be filled in against its resource model (tags, resource relations, custom fields)
- **Desired state vs actual state separation + drift views**: Cockpit's Syncer already follows this pattern; an API/UI could expose a highlighted view of "inventory declarations inconsistent with agent-reported reality" — this is the differentiating selling point of a Git-first CMDB

## Probing: Uptime Kuma

[Uptime Kuma](https://github.com/louislam/uptime-kuma) — the self-hosted probing/monitoring benchmark (GitHub is the only official source; uptimekuma.co is an unofficial fan site, not cited).

**Pros**

- 20-second probing intervals, multi-protocol (HTTP(s)/TCP/ping/DNS/keyword/Docker containers)
- 90+ notification channels (ntfy/Telegram/Discord/Slack/email, etc.)
- Public status pages, heartbeat-bar style history UI

**Cons**

- Node.js resident resource usage is on the high side
- Can only probe from its own location, **no distributed agent probing**
- No CMDB linkage; monitoring targets must be entered by hand

**Worth borrowing** (against Cockpit's `internal/probe`; "landed" items below were the original borrowing sources)

- ~~Configurable probing intervals~~ Landed: UI `probe.interval_seconds` setting / yaml `probe.interval` /
  `PUT /api/probe/config` (auto-clamped to [30s, 1h])
- ~~Notification channel integration: add ntfy/webhook/Telegram outputs~~ Landed: notification channels
  herald/ntfy/webhook/telegram, see the notification section of `config/cockpit.yaml`
- Heartbeat-bar status history UI, public status pages (still not built)
- **Cockpit's differentiating advantage**: probe results naturally link to inventory resources and can probe distributedly via the network the Agent sits in — something Uptime Kuma's architecture cannot do, and it should be strengthened head-on rather than avoided

## Summary Matrix

| Project | Overlap domain | Directly adoptable (copy the practice) | Worth borrowing (absorb the idea) | Not worth learning |
| --- | --- | --- | --- | --- |
| Beszel | Monitoring/agent | Compact historical-data storage | Agent lightness as a hard metric, deployment experience | —— |
| Komodo | Containers/agent | —— | Git-to-deploy, procedure orchestration | MongoDB dependency |
| Portainer/Dockge | Docker UI | Compose stack page interactions | compose-file-first | Enterprise governance layer |
| Guacamole | Remote control | —— | Session recording, file transfer | Java stack, static connection configuration |
| Teleport | Remote-control security | Short-lived tickets (validated) | Approval flows, session-granularity auditing | Enterprise-grade complexity |
| NetBox | CMDB | —— | Desired/actual state separation, drift views | Domain model weight |
| Uptime Kuma | Probing | Heartbeat-bar UI | Notification channels, status pages | Single-point probing model |

## Conclusions for Feature Planning

1. **P1 "App Deployment (Compose Stack)" starts first**: borrowing both Komodo's Git-to-deploy and Dockge's compose-file-first; Cockpit already has the `docker_provider` RPC path, the highest input-output ratio.
2. **Probe enhancements**: configurable intervals + notification channels, low cost, letting the landed health probing immediately produce real value.
3. **Session recording** (long-term): borrow from Guacamole/Teleport to reinforce remote-control auditing.
4. **Drift view** (long-term): borrow from NetBox to strengthen the Git-first CMDB differentiation.

## Sources

- [Beszel official site](https://www.beszel.dev/) / [henrygd/beszel](https://github.com/henrygd/beszel)
- [I tested 7 self-hosted monitoring tools (2026)](https://dev.to/vikasprogrammer/i-tested-7-self-hosted-monitoring-tools-on-a-3-vps-in-2026-heres-the-one-i-kept-aoa)
- [RamNode Beszel guide](https://www.ramnode.com/guides/series/monitoring/beszel)
- [Komodo docs](https://komo.do/docs/intro)
- [Komodo vs Portainer: Git-to-deploy](https://medium.com/@mariomarco08/komodo-vs-portainer-which-fits-a-git-to-deploy-workflow-715e8ef49bd3)
- [Bitdoze: five Docker management UIs tested](https://www.bitdoze.com/portainer-alternatives/)
- [Portainer vs Dockge (OneUptime, 2026)](https://oneuptime.com/blog/post/2026-03-20-portainer-vs-dockge/view)
- [Portainer vs Dockge (Talos.tools)](https://talos.tools/compare/portainer-vs-dockge)
- [Apache Guacamole official site](https://guacamole.apache.org/)
- [Teleport/Guacamole community comparison](https://www.reddit.com/r/selfhosted/comments/1im5ioy/how_to_connect_to_sshrdp_web_based_with_teleport/)
- [NetBox Labs: The Long War Against Configuration Chaos](https://netboxlabs.com/blog/the-long-war-against-configuration-chaos/)
- [NetBox GitOps integration](https://netodata.io/netbox-integration-connecting-dcim-ipam-with-enterprise-infrastructure/)
- [louislam/uptime-kuma](https://github.com/louislam/uptime-kuma) (official GitHub repository)
