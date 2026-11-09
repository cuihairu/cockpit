# Inventory Declaration Source

The Inventory file is the declaration source of Cockpit: it describes which assets the system should know about, and the Server reconciles that declaration into SQLite. The runtime view built on top of it is described in [Runtime State](/guide/runtime).

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

## Differences from Historical Designs

The repository contains some design documents describing a more complete Git-first, directory-split inventory and third-party integration vision. The current implementation already has inventory sync, the Server/Agent control plane, resource queries, monitoring, authentication, auditing, alerts, and remote connections, but the code facts described in this document and [Architecture and Boundaries](/guide/architecture) remain authoritative.
