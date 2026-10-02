<p align="center">
  <img src="web/public/logo.svg" alt="Cockpit Logo" width="120" height="120">
</p>

<h1 align="center">Cockpit</h1>

个人混合基础设施控制台，用于把分散在本地机房、云 VPS、NAT 后节点上的资源收敛到一个轻量 Server + Agent 控制面。

[![Test](https://img.shields.io/github/actions/workflow/status/cuihairu/cockpit/test.yml?branch=main&logo=github&label=Test)](https://github.com/cuihairu/cockpit/actions/workflows/test.yml)
[![Docs](https://img.shields.io/github/actions/workflow/status/cuihairu/cockpit/docs.yml?branch=main&logo=github&label=Docs)](https://github.com/cuihairu/cockpit/actions/workflows/docs.yml)
[![codecov](https://codecov.io/gh/cuihairu/cockpit/branch/main/graph/badge.svg)](https://codecov.io/gh/cuihairu/cockpit)
[![Agent: Windows](https://img.shields.io/badge/platform-Windows-0078D4?logo=windows&logoColor=white)](https://cuihairu.github.io/cockpit/guide/getting-started)
[![Agent: Linux](https://img.shields.io/badge/platform-Linux-FCC624?logo=linux&logoColor=black)](https://cuihairu.github.io/cockpit/guide/getting-started)
[![Agent: macOS](https://img.shields.io/badge/platform-macOS-000000?logo=apple&logoColor=white)](https://cuihairu.github.io/cockpit/guide/getting-started)
[![Agent: x86_64](https://img.shields.io/badge/arch-x86__64-5B5B5B)](https://cuihairu.github.io/cockpit/guide/getting-started)
[![Agent: aarch64](https://img.shields.io/badge/arch-aarch64-5B5B5B)](https://cuihairu.github.io/cockpit/guide/getting-started)

## 核心特性

- **资源收敛**：Inventory YAML 同步为统一资源视图，拨测心跳与到期告警
- **NAT 友好**：Agent 主动连出注册，节点无需暴露入站端口
- **远程操作**：终端 / VNC / 桌面经短期 ticket 转发，支持会话录制
- **容器与应用**：Docker 全量管理 + Stacks 按 compose 部署
- **运维面**：反向代理站点、ACME 证书、DNS/DDNS、备份恢复、服务/Cron/SMART/NAS/组网观测
- **漂移与告警**：配置漂移检测，多渠道通知（Herald / ntfy / webhook / Telegram）

## 快速开始

> 前置：先有一台 Cockpit Server（Docker 一条起，部署机无需编译环境）：

```bash
cp deployments/docker/.env.example deployments/docker/.env && vi deployments/docker/.env
docker compose -f deployments/docker/docker-compose.yml up -d
```

### Linux

1. 下载并安装（自动检测架构，从每日构建匿名直链下载，装完自动验证）：

   ```bash
   curl -fsSL https://raw.githubusercontent.com/cuihairu/cockpit/main/install.sh | bash
   ```

2. 连接服务器（示例用本站域名，替换为你的 Cockpit 服务端地址）：

   ```bash
   cockpit-agent start -server wss://cockpit.cuihairu.site/ws -region home -zone datacenter
   ```

3. 验证安装：

   ```bash
   cockpit-agent --version
   ```

   验证成功预期输出：

   ```text
   Cockpit Agent v20261001
   ```

   （版本号为安装当日日期；agent 日志出现 `Registered as agent: ...` 即已连上服务器）

### macOS

1. 下载并安装（与 Linux 共用 `install.sh`，自动识别 Darwin 与 arm64/amd64）：

   ```bash
   curl -fsSL https://raw.githubusercontent.com/cuihairu/cockpit/main/install.sh | bash
   ```

2. 连接服务器：

   ```bash
   cockpit-agent start -server wss://cockpit.cuihairu.site/ws -region home -zone datacenter
   ```

3. 验证安装：

   ```bash
   cockpit-agent --version
   ```

   验证成功预期输出：

   ```text
   Cockpit Agent v20261001
   ```

### Windows（PowerShell 5.1+）

1. 下载并安装（免管理员，装到当前用户目录并写入用户 PATH）：

   ```powershell
   irm https://raw.githubusercontent.com/cuihairu/cockpit/main/install.ps1 | iex
   ```

2. 连接服务器（新开一个终端使 PATH 生效）：

   ```powershell
   cockpit-agent start -server wss://cockpit.cuihairu.site/ws -region home -zone datacenter
   ```

3. 验证安装：

   ```powershell
   cockpit-agent --version
   ```

   验证成功预期输出：

   ```text
   Cockpit Agent v20261001
   ```

## 文档

完整安装（一键安装细节 / 平台矩阵 / 源码构建 / OpenWrt ipk）、配置说明、架构与各功能设计文档均在文档站：

- [介绍](https://cuihairu.github.io/cockpit/guide/introduction)
- [快速开始](https://cuihairu.github.io/cockpit/guide/getting-started)
- [架构与边界](https://cuihairu.github.io/cockpit/guide/architecture)
- [协议与 API 边界](https://cuihairu.github.io/cockpit/guide/protocol)
- [Docker 部署](https://cuihairu.github.io/cockpit/operations/deploy-docker)
- Agent 手工部署与服务单元样例：[deployments/README.md](deployments/README.md)

## 许可证

Apache License 2.0
