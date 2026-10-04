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

面板把资源、执行、远控三条线收在一处。资源侧从 Inventory YAML 同步出统一资源视图，拨测心跳、到期告警、配置漂移检测都挂这份数据，告警发 Herald / ntfy / webhook / Telegram 四个渠道。执行侧有一本全机 Job 台账，对任意在线 agent 下发命令，状态、退出码、输出回写可查；Docker 容器全量管理，应用按 compose 文件以 Stacks 部署。远控侧终端、VNC、桌面走短期 ticket 转发并支持会话录制；反向代理站点、ACME 证书、DNS/DDNS、备份恢复、服务/Cron/SMART/NAS/组网观测也在同一面板操作。

Agent 经 WebSocket 主动连出注册，NAT 后的节点不用暴露任何入站端口。

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

### Windows

**图形化安装器（推荐）**：从 [nightly release](https://github.com/cuihairu/cockpit/releases/latest)
下载 `cockpit-agent-setup-nightly.exe`（同名 `.sha256` 为校验文件），双击运行：
装到 `%ProgramFiles%\Cockpit Agent`、开始菜单 + 桌面快捷方式（带 agent 图标）、
向导内填写 Server 地址、可勾选「注册 Windows 服务并开机自启」、控制面板标准
卸载条目。静默装机：

```powershell
.\cockpit-agent-setup-nightly.exe /VERYSILENT /SUPPRESSMSGBOXES /NORESTART /SERVER=wss://cockpit.cuihairu.site/ws
```

服务管理：`cockpit-agent service install|uninstall|start|stop|status`（服务名
`CockpitAgent`，日志 `%ProgramData%\CockpitAgent\agent.log`）。

**PowerShell 脚本路径**（免管理员，装到当前用户目录并写入用户 PATH）：

1. 下载并安装：

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

## 底座

本仓库基于开源组件构建，自己写的是控制面接线与各 provider 层：Server 与 Agent 用 Go，存储经 GORM 落 SQLite，实时通道跑在 gorilla/websocket 上；Web 端是 React 19 + Ant Design，终端渲染用 xterm.js；桌面与 VNC/SSH 远控接 Apache Guacamole（guacd 服务端 + guacamole-common-js 1.5.0 客户端），内置终端的 SSH 走 golang.org/x/crypto/ssh；证书签发用 go-acme/lego（DNS-01），移动端是 Flutter，文档站是 VitePress，Windows 安装器由 Inno Setup 打包。

## 许可证

Apache License 2.0
