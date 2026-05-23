# 介绍

## 什么是 Cockpit？

Cockpit 是一个针对**个人混合基础设施**的管理平台。如果你有以下场景，Cockpit 适合你：

- 多个地域的机房/服务器（本地机房 + 云 VPS）
- 多种虚拟化技术（PVE、LXC、Docker、KVM）
- 分散各地的 OpenWrt 路由器做内网穿透
- 多个域名和 SSL 证书需要管理
- GitHub Actions 等服务的运行状态需要监控

## 为什么需要 Cockpit？

### 痛点

| 痛点 | 说明 |
|------|------|
| **资产分散** | PVE 控制台、Portainer、各云厂商控制台、Nezha、Homepage... 访问入口分散 |
| **状态不可见** | 哪个服务挂了？哪个证书快过期？哪台 VPS 快到期？没有统一视图 |
| **配置散落** | Homepage/Nezha/Homelable 各存一份 SQLite，无版本控制 |
| **异地难管** | OpenWrt 分散各地、NAT 后，常规扫描探不到 |
| **关系不清晰** | 这个网站跑在哪个 Docker？哪个 VM？哪台 VPS？ |

### Cockpit 的解决方案

```
Git (配置真相源)
    ↓
Cockpit Server (统一视图 + 状态存储)
    ↓
各地 Agent (主动上报 + API 转发)
    ↓
第三方集成 (Nezha/Homepage)
```

## 与其他项目的关系

Cockpit **不是要替代**这些项目，而是**整合**它们：

| 项目 | Cockpit 如何使用 |
|------|------------------|
| **Homepage** | 从 Cockpit 配置生成 services.yaml |
| **Nezha** | Cockpit Agent 可作为 Nezha Agent，或对接 Nezha Dashboard |
| **Homelabel** | Cockpit 提供 JSON 导出，供 Homelabel 导入 |
| **PVE/Portainer** | Cockpit 只读展示，控制操作跳转到原平台 |

## 核心特性

### 1. Git-first 配置

```bash
inventory/
├── regions/          # 地域定义
├── zones/            # 可用区/机房
└── resources/        # 资产实例
    ├── compute-instances/
    ├── domains/
    ├── certificates/
    └── services/
```

所有配置存储在 Git，支持：
- `git diff` 查看变更
- `git rollback` 回滚历史
- `git pull/push` 多机同步

### 2. 跨地域监控

```
机房A (NAT后)          机房B (NAT后)          云VPS
    │                      │                    │
    └──────────────────────┼────────────────────┘
                           ↓
                    Cockpit Server (公网)
```

Agent 主动连接 Server，无需：
- 暴露内网端口
- 配置端口转发
- 担心防火墙规则

### 3. 资产关系建模

```yaml
# 服务 ← 部署于 → 计算实例
services/my-blog.yaml:
  computeRef: compute-vm-nginx

# 计算实例 ← 属于 → 宿主
compute-vm-nginx.yaml:
  hostRef: compute-pve-host-01

# 服务 ← 关联 → 域名
services/my-blog.yaml:
  domainRef: domain-example-com
  certificateRef: cert-example-com
```

一目了然的资产归属链。

## 架构概览

```
┌─────────────────────────────────────────────────────────────┐
│                    Cockpit Server                           │
│  ┌─────────────────────────────────────────────────────┐    │
│  │  WebSocket Server              │    │
│  │  Agent Registry (连接池管理)                        │    │
│  │  RPC Router (转发 API 调用)                         │    │
│  │  SQLite (配置 + 运行时状态)                         │    │
│  │  Web UI / API                                       │    │
│  └─────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
         ↑ WebSocket (Agent 主动连接)
         │
┌─────────────────────────────────────────────────────────────┐
│  各地 Agent（物理机/VM/容器，跨地域/NAT 均可）                │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │
│  │ 机房A Agent │  │ 机房B Agent │  │ OpenWrt Agent│         │
│  │ - PVE API   │  │ - PVE API   │  │ - 隧道拓扑   │         │
│  │ - Docker    │  │ - 硬件监控  │  │ - 路由信息   │         │
│  └─────────────┘  └─────────────┘  └─────────────┘          │
└─────────────────────────────────────────────────────────────┘
```

## 下一步

- [快速开始](/guide/getting-started) —— 5 分钟上手
- [核心概念](/guide/concepts) —— 了解资产模型
- [部署指南](/guide/deploy-server) —— 生产环境部署
