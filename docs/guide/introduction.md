# 介绍

Cockpit 是一个面向个人混合基础设施的控制台。当前实现重点解决三件事：

- 用 Inventory YAML 和 SQLite 建立统一资源视图。
- 通过 Agent 主动连接 Server，管理 NAT 后或跨地域节点。
- 在 Web UI 中查看资源、监控、告警、审计，并发起远程终端、VNC、RDP 等连接。

## 当前能力

| 能力 | 当前状态 |
| --- | --- |
| Server 控制面 | 已实现 HTTP API、Web UI 静态资源、Agent WebSocket、SQLite 持久化 |
| Agent 执行面 | 已实现主动注册、心跳、系统指标采集、能力检测、代理和桌面消息处理 |
| Inventory 同步 | `cockpit sync` 支持单文件 `version: v1` YAML 同步到 SQLite |
| 资源视图 | 支持 Agent、计算实例、域名、证书、服务、网关、存储 |
| 认证 | 支持管理员初始化、JWT、TOTP、密码重置 |
| 远程连接 | 支持基于短期 ticket 的终端、VNC、桌面连接入口 |
| 审计与告警 | 支持登录/代理等审计记录，证书/域名/服务/Agent 告警检查 |

## 适用场景

Cockpit 适合个人或小规模环境：

- 家庭机房、本地机房和云 VPS 混合管理。
- 多个节点处在 NAT 后，只能主动连出。
- 希望用一份清单描述主机、服务、域名、证书和存储。
- 希望在一个 Web UI 中查看资源状态、系统指标和远程连接入口。

Cockpit 目前不是面向大规模多租户的 CMDB，也不是 Kubernetes 控制平面。它更像个人基础设施的轻量控制台。

## 架构概览

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

核心原则：

- Web UI 只访问 Server。
- Agent 主动连接 Server。
- Server 维护全局运行时视图。
- Agent 负责节点侧探测和执行。
- Inventory 是声明式输入，SQLite 是运行时查询面。

完整边界见 [架构与边界](/guide/architecture)。

## 与其他工具的关系

Cockpit 不要求替代已有控制台。当前代码中已经有 PVE、Docker、OpenWrt 客户端和 Agent 能力检测，长期可以把这些能力编排到统一工作台中。

当前应按以下方式理解：

| 工具/平台 | Cockpit 当前关系 |
| --- | --- |
| PVE | 有 API 客户端和 Agent capability 检测；控制面集成仍需按具体功能确认 |
| Docker | 有 API 客户端、能力检测和资源视图基础 |
| OpenWrt | 有客户端和能力检测基础 |
| 远程终端/VNC/RDP | 通过 Agent 代理访问内网目标 |
| Git | Inventory YAML 可以由 Git 管理，但当前应用仍以 `cockpit sync` 写入 SQLite 为准 |

## 文档阅读路径

1. [快速开始](/guide/getting-started)
2. [核心概念](/guide/concepts)
3. [架构与边界](/guide/architecture)
4. [协议与 API 边界](/guide/protocol)
