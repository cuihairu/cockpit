# 介绍

Cockpit 是一个面向个人混合基础设施的控制台。当前实现重点解决三件事：

- 用 Inventory YAML 和 SQLite 建立统一资源视图。
- 通过 Agent 主动连接 Server，管理 NAT 后或跨地域节点。
- 在 Web UI 中查看资源、监控、告警、审计，并发起远程终端、VNC、RDP 等连接。

## 当前能力

控制面底子是完整的：Server 有 Web UI、HTTP API、Agent WebSocket 接入、TOTP 两步验证、RBAC 用户与角色、审计日志，数据落 SQLite；Agent 主动连出注册，上报心跳、系统指标与能力标签，NAT 后的节点不开入站端口。`cockpit sync` 把 Inventory YAML 同步成运行时资源视图——计算实例、域名、证书、服务、网关、存储六类资源行都带拨测心跳条与到期告警。

远程操作方面，终端 / VNC / 桌面走短期 ticket 转发，会话录制归档可回放；agent 上能浏览与传输文件；日志既能在单机 tail（journalctl / Docker），也能跨机联邦检索。应用与容器由 Docker 全量管理（容器、镜像、网络、卷），Stacks 按 compose 部署，带模板库与部署历史，restart/pull 就近操作。

备份做在两侧：agent 侧定时打包、保留策略、双阶段安全恢复，产物经 rclone 推异地并可补传；server 侧用 `VACUUM INTO` 定时备份面板数据库，同样支持异地推送。Web 侧管反向代理站点（nginx / Traefik 双后端，语法自检、失败回滚），证书走 ACME 签发与自动续期（Cloudflare / DNSPod / 阿里云 DNS-01），签发产物自动部署到 agent 指定路径；DNS 记录管理覆盖三家厂商的 A/AAAA/CNAME/MX/TXT/NS/SRV/CAA 八类记录，另有 DDNS 动态域名与域名/证书到期台账。

主机运维覆盖服务管理（systemd / Windows SCM / macOS launchd 三后端）、Cron 定时任务、SMART 磁盘巡检、NAS 观测（DSM / TrueNAS / OMV，本地 mdadm / ZFS），以及组网观测与管理（WireGuard / ZeroTier / Tailscale / frp 运行态、云端纳管、本机加入/离开网络与 daemon 服务启停）。配置漂移检测做面板下发与磁盘现状的比对，CMDB 一致性做声明与实报的对照；告警经 Herald / ntfy / webhook / Telegram 四个渠道发出。

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
4. [Agent 出口与 SD-WAN 能力边界](/guide/agent-egress-sdwan)
5. [协议与 API 边界](/guide/protocol)
