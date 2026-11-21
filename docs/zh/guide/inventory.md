# Inventory 声明源

Inventory 文件是 Cockpit 的声明源：它描述系统应当知道的资产，Server 把这份声明 reconcile 进 SQLite。建立在它之上的运行时视图见 [运行时状态](/zh/guide/runtime)。

## 配置与状态

Cockpit 当前有两个主要数据面：

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

Inventory YAML 描述“希望系统知道哪些资产”。SQLite 保存 API 和 Web UI 查询所需的运行时视图，包括同步后的资源、用户、告警、审计、代理配置、系统指标和 Agent 在线状态。

需要注意：

- `cockpit sync` 是手动 inventory 同步入口。
- `inventory.watch: true` 的 Server 热加载复用同一套 Syncer，会同步 Agent、Domain、Certificate、ComputeInstance、Service、Gateway、Storage 等资源。
- `inventory.strict: true` 会在 Server 启动时严格校验 inventory；默认 `false` 时启动失败风险会降级为日志。
- Agent 心跳会上报在线状态和系统指标，但不会直接修改 inventory 文件。

## Inventory 文件

当前主路径是单文件 YAML：

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

顶层字段当前包括：

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

`resources` / `templates` 已在 schema 中保留，但同步到数据库的主路径是明确的资源字段，例如 `computeInstances`、`services`、`gateways`、`storages`。

资源上的 `region` / `zone` 字段可省略：为空不阻塞同步，落库统一标记 `unknown`（显式值优先；`services`/`gateways`/`storages` 的地域写进各自 labels，用户在 labels 里显式给的键不被空字段覆盖）。Agent 注册侧同语义——未提供时默认 `unknown`（D-2026-10-08-1）。

## 与历史设计的差异

仓库中有一些设计文档描述了更完整的 Git-first、目录拆分 inventory 和第三方集成愿景。当前实现已经具备 inventory 同步、Server/Agent 控制面、资源查询、监控、认证、审计、告警和远程连接，但仍应以本文和[架构与边界](/zh/guide/architecture)中描述的代码事实为准。
