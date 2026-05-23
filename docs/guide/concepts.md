# 核心概念

## 资产模型

Cockpit 将你的基础设施抽象为以下层级：

```
Region（地域/城市）
  └─ Zone（可用区/机房）
      └─ Resource（资源实例）
```

### Region（地域）

地理或逻辑位置，如 `local`（本地）、`jiangsu-huaian`（江苏淮安）、`tokyo`（东京）。

### Zone（可用区）

机房、数据中心、或更细粒度的物理位置。

### Resource（资源实例）

具体管理的资产，包括：

| 资源类型 | 说明 |
|----------|------|
| `ComputeInstance` | 计算实例：PVE VM/LXC、VPS、物理机 |
| `ContainerService` | 容器服务：Docker Stack/Service |
| `Domain` | 域名及注册信息 |
| `Certificate` | SSL/TLS 证书 |
| `Service` | Web 服务、API、数据库等 |
| `Gateway` | 网关/路由器（如 OpenWrt） |
| `CIService` | CI/CD 服务（如 GitHub Actions） |
| `Storage` | 存储设备（如 NAS） |

## 配置 vs 状态

```
┌─────────────────────────────────────────────────────────────┐
│                    配置层 (Config)                          │
│  inventory/*.yaml — Git 管理，版本控制                      │
└─────────────────────────────────────────────────────────────┘
                        ↓ sync
┌─────────────────────────────────────────────────────────────┐
│                    运行时层 (Runtime)                        │
│  SQLite — 查询、聚合、状态监控                               │
└─────────────────────────────────────────────────────────────┘
```

- **配置**：静态的、声明式的描述（在 Git 中）
- **状态**：动态的、实时变化的（在数据库中）

## 引用（Ref）关系

资源之间通过 **Ref** 建立关联：

```yaml
# 服务引用计算实例
services/my-blog.yaml:
  computeRef: compute-pve-vm-nginx

# 服务引用域名和证书
  domainRef: domain-example-com
  certificateRef: cert-example-com
```

引用格式：`{资源类型短名}-{唯一标识}`

## Agent 能力

Agent 通过 **能力声明** 告诉 Server 它能做什么：

```json
{
  "type": "pve-api",
  "endpoint": "https://192.168.1.10:8006",
  "version": "8.0"
}
```

常见能力类型：

| 能力类型 | 说明 |
|----------|------|
| `pve-api` | 可转发 PVE API 请求 |
| `docker-api` | 可转发 Docker API 请求 |
| `hardware-monitor` | 可采集硬件信息（SMART、温度） |
| `network-monitor` | 可采集网络信息（隧道、路由） |

## 消息类型

WebSocket 通信的消息类型：

| 方向 | 类型 | 说明 |
|------|------|------|
| Agent → Server | `register` | Agent 注册 |
| Agent → Server | `heartbeat` | 心跳上报 |
| Server → Agent | `rpc_request` | RPC 调用请求 |
| Agent → Server | `rpc_response` | RPC 调用响应 |
| 双向 | `error` | 错误信息 |
