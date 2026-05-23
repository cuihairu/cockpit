# Cockpit

> 个人混合基础设施控制台 —— Git-first 的 Homelab CMDB 与监控平台

[![Go](https://github.com/cuihairu/cockpit/actions/workflows/go.yml/badge.svg)](https://github.com/cuihairu/cockpit/actions/workflows/go.yml)
[![Docs](https://github.com/cuihairu/cockpit/actions/workflows/docs.yml/badge.svg)](https://github.com/cuihairu/cockpit/actions/workflows/docs.yml)
[![codecov](https://codecov.io/gh/cuihairu/cockpit/branch/main/graph/badge.svg)](https://codecov.io/gh/cuihairu/cockpit)

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://go.dev/)
[![Linux](https://img.shields.io/badge/Linux-FCC624?logo=linux&logoColor=black)](https://github.com/cuihairu/cockpit/releases)
[![macOS](https://img.shields.io/badge/macOS-000000?logo=apple)](https://github.com/cuihairu/cockpit/releases)
[![Windows](https://img.shields.io/badge/Windows-00A4EF?logo=windows)](https://github.com/cuihairu/cockpit/releases)
[![OpenWrt](https://img.shields.io/badge/OpenWrt-00B5E2?logo=openwrt)](https://github.com/cuihairu/cockpit/releases)

## 介绍

Cockpit 是一个针对个人混合基础设施的管理平台，提供：

- **统一资产视图**：物理机、PVE VM/LXC、Docker、域名、证书、CI/CD 等
- **跨地域监控**：支持异地机房、NAT 后设备的主动上报心跳
- **Git-first 配置**：所有配置存储在 Git，支持 diff/rollback
- **第三方集成**：同步到 Nezha、Homepage 等已有工具

## 架构

```
┌─────────────────────────────────────────────────────────────┐
│                    Cockpit Server                           │
│  ┌─────────────────────────────────────────────────────┐    │
│  │  WebSocket Server (Agent 连接)                      │    │
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
└─────────────────────────────────────────────────────────────┘
```

## 快速开始

### 安装 Server

```bash
# 克隆仓库
git clone https://github.com/cuihairu/cockpit.git
cd cockpit

# 初始化配置
./cockpit init

# 启动 Server
./cockpit server start
```

### 部署 Agent

```bash
# 在目标机器上
./cockpit-agent start --server wss://your-server.com:8080
```

## 文档

完整文档请访问：[cuihairu.github.io/cockpit](https://cuihairu.github.io/cockpit/)

## 许可证

Apache License 2.0
