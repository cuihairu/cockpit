# 快速开始

## 环境要求

- Go 1.26.3 或兼容版本
- Node.js 与 pnpm，用于构建 Web UI
- Server 默认监听 `127.0.0.1:9000`

## 构建

在仓库根目录：

```bash
go build -o cockpit ./cmd/cockpit
go build -o cockpit-agent ./cmd/cockpit-agent
```

如需 Web UI：

```bash
cd web
pnpm install
pnpm build
cd ..
```

`cockpit init` 生成的配置默认使用 `server.static_dir: ./web/dist`。如果静态目录不存在，Server 根路径会返回 API 运行提示，而不是 Web UI。

## 初始化

```bash
./cockpit init -example
```

这会在当前目录创建：

```text
config/
  cockpit.yaml
data/
  cockpit.db
inventory/
  example.yaml
```

默认配置重点：

```yaml
server:
  host: 127.0.0.1
  port: 9000
  static_dir: ./web/dist

database:
  path: ./data/cockpit.db

inventory:
  path: ./inventory/example.yaml
  watch: false
```

## 同步 Inventory

```bash
./cockpit sync -config config/cockpit.yaml
```

也可以显式指定 inventory 和数据库：

```bash
./cockpit sync -inventory inventory/example.yaml -db data/cockpit.db
```

同步完成后查看数据库状态：

```bash
./cockpit status -db data/cockpit.db
```

## 启动 Server

Server 启动时必须提供管理员密码：

```bash
export ADMIN_PASSWORD='change-this-password'
./cockpit server -config config/cockpit.yaml
```

Windows PowerShell：

```powershell
$env:ADMIN_PASSWORD = 'change-this-password'
.\cockpit.exe server -config config\cockpit.yaml
```

启动后访问：

- Web UI: `http://127.0.0.1:9000`
- Health: `http://127.0.0.1:9000/health`

默认管理员用户名是 `admin`，可通过 `ADMIN_USERNAME` 修改。

## 启动 Agent

在被管理节点上运行：

```bash
./cockpit-agent start -server ws://127.0.0.1:9000/ws -region home -zone datacenter
```

常用参数：

```bash
./cockpit-agent start \
  -server wss://cockpit.example.com/ws \
  -id server01 \
  -secret YOUR_AGENT_SECRET \
  -region home \
  -zone datacenter \
  -labels env=home,role=hypervisor
```

要点：

- `-server` 必须指向 Server 的 Agent WebSocket 入口，通常以 `/ws` 结尾。
- `-secret` 可选但推荐；当 Server 中该 Agent 已配置 secret 后，后续注册必须提供。
- 未指定 `-region` / `-zone` 时，Agent 会尝试读取 `COCKPIT_REGION` / `COCKPIT_ZONE`，否则使用 `unknown`。

## 生产环境最小配置

生产环境至少应设置：

```bash
export ADMIN_PASSWORD='use-a-strong-password'
export TOTP_ENCRYPTION_KEY="$(openssl rand -base64 32)"
export ALLOWED_ORIGINS="https://cockpit.example.com"
export PRODUCTION=true
```

并将 Server 配置改为对外监听：

```yaml
server:
  host: 0.0.0.0
  port: 9000
```

建议通过反向代理终止 TLS，并让 Agent 使用 `wss://.../ws` 连接。

## 下一步

- [核心概念](/guide/concepts)
- [架构与边界](/guide/architecture)
- [协议与 API 边界](/guide/protocol)
