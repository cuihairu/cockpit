# Cockpit

个人混合基础设施控制台，用于把分散在本地机房、云 VPS、NAT 后节点上的资源收敛到一个轻量 Server + Agent 控制面。

[![Test](https://github.com/cuihairu/cockpit/actions/workflows/test.yml/badge.svg)](https://github.com/cuihairu/cockpit/actions/workflows/test.yml)
[![Docs](https://github.com/cuihairu/cockpit/actions/workflows/docs.yml/badge.svg)](https://github.com/cuihairu/cockpit/actions/workflows/docs.yml)
[![codecov](https://codecov.io/gh/cuihairu/cockpit/branch/main/graph/badge.svg)](https://codecov.io/gh/cuihairu/cockpit)

## 当前能力

- Server 提供 Web UI、HTTP API、Agent WebSocket、认证、审计、告警和 SQLite 持久化。
- Agent 主动连接 Server，上报心跳、系统指标和能力信息。
- Inventory YAML 可通过 `cockpit sync` 同步为运行时资源视图。
- Web UI 支持资源、工作台、监控、设置、审计和远程连接入口。
- 远程终端、VNC、桌面连接使用短期 ticket，经 Server 和 Agent 转发到目标服务。

## 架构

```text
Browser Web UI
    |
    | HTTP API / WebSocket
    v
Cockpit Server (cmd/cockpit)
    |  \
    |   \ SQLite
    |
    | WebSocket /ws
    v
Cockpit Agent (cmd/cockpit-agent)
    |
    | local socket / TCP / platform API
    v
Managed targets
```

Server 是中心控制面，Agent 是节点侧执行面。Agent 主动连出，所以 NAT 后节点不需要暴露入站端口。

## 快速开始

构建二进制：

```bash
go build -o cockpit ./cmd/cockpit
go build -o cockpit-agent ./cmd/cockpit-agent
```

构建 Web UI：

```bash
cd web
pnpm install
pnpm build
cd ..
```

初始化并同步示例清单：

```bash
./cockpit init -example
./cockpit sync -config config/cockpit.yaml
```

启动 Server：

```bash
export ADMIN_PASSWORD='change-this-password'
./cockpit server -config config/cockpit.yaml
```

默认访问地址：

- Web UI: `http://127.0.0.1:9000`
- Health: `http://127.0.0.1:9000/health`

启动 Agent：

```bash
./cockpit-agent start -server ws://127.0.0.1:9000/ws -region home -zone datacenter
```

## 关键配置

默认配置路径优先级：

1. `-config` 指定路径
2. `./config/cockpit.yaml`
3. `./cockpit.yaml`
4. `/etc/cockpit/config.yaml`

生产环境至少设置：

```bash
export ADMIN_PASSWORD='use-a-strong-password'
export TOTP_ENCRYPTION_KEY="$(openssl rand -base64 32)"
export ALLOWED_ORIGINS="https://cockpit.example.com"
export PRODUCTION=true
```

对外部署时将 `server.host` 改为 `0.0.0.0`，并建议通过反向代理提供 HTTPS/WSS。

## 端到端冒烟脚本

仓库内置一个最小闭环验证脚本，用于本地一键验证 server/agent/inventory sync/资源 API 链路是否正常：

```bash
./scripts/e2e-smoke.sh
```

Windows PowerShell:

```powershell
pwsh ./scripts/e2e-smoke.ps1
```

脚本行为：

1. 在临时目录创建最小 `cockpit.yaml` + `inventory.yaml`
2. 构建 `cockpit` 与 `cockpit-agent` 二进制
3. 启动 server，等待 `/health` 返回 ok
4. 启动 agent，等待 `/api/agents` 出现在线记录
5. 用 `cockpit sync` 同步 inventory，并验证 `/api/resources/{compute-instances,domains,certificates,services,gateways,storages}` 返回非空结果

退出码 0 表示全链路正常；非 0 时会自动打印 `server.log` 末尾用于排查。
保留日志便于调试：`E2E_KEEP_LOGS=1 ./scripts/e2e-smoke.sh` 或 `pwsh ./scripts/e2e-smoke.ps1 -KeepLogs`。

## 文档

- [介绍](https://cuihairu.github.io/cockpit/guide/introduction)
- [快速开始](https://cuihairu.github.io/cockpit/guide/getting-started)
- [架构与边界](https://cuihairu.github.io/cockpit/guide/architecture)
- [协议与 API 边界](https://cuihairu.github.io/cockpit/guide/protocol)

## 许可证

Apache License 2.0
