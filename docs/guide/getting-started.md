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
  strict: false
```

`inventory.watch: true` 会让 Server 监听并热加载单个 inventory 文件。`inventory.strict: true` 会把启动时的 inventory 路径缺失、解析失败或初始同步失败视为 Server 启动错误；默认 `false` 时只记录日志，Server 继续启动。

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

### 一键安装（推荐）

三平台各一行命令（从每日构建 nightly release 匿名直链下载，自动检测
OS 与 CPU 架构，装完自动执行 `cockpit-agent --version` 验证；重跑即升级）：

Linux / macOS：

```bash
curl -fsSL https://raw.githubusercontent.com/cuihairu/cockpit/main/install.sh | bash
```

Windows（PowerShell 5.1+ / pwsh）：

```powershell
irm https://raw.githubusercontent.com/cuihairu/cockpit/main/install.ps1 | iex
```

可选注册开机自启服务（systemd / launchd / Windows 服务），连接信息随
`--with-service` 一并传入：

```bash
curl -fsSL https://raw.githubusercontent.com/cuihairu/cockpit/main/install.sh | \
  bash -s -- --with-service --server wss://cockpit.cuihairu.site/ws
```

```powershell
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/cuihairu/cockpit/main/install.ps1))) `
  -WithService -ServerUrl "wss://cockpit.cuihairu.site/ws"
```

支持矩阵：Linux amd64 / arm64 / armv7、macOS amd64 / arm64、Windows
amd64 / arm64（全部每日构建产物）；架构不认识时脚本会明确报错。
手动部署、服务参数与故障排查见 [deployments/README.md](https://github.com/cuihairu/cockpit/blob/main/deployments/README.md)。

### 手动启动

OpenWrt 路由器无需手工编译：nightly release 提供多架构
`.ipk` 安装包（opkg 即装、procd 托管、UCI 配置），安装步骤见
[OpenWrt Agent 打包设计](/guide/openwrt-agent-design#安装与升级)。

其余场景：下载对应平台 `cockpit-agent` 二进制（或本地编译），
在被管理节点上运行：

```bash
./cockpit-agent start -server wss://cockpit.cuihairu.site/ws -region home -zone datacenter
```

主二进制也提供兼容入口，参数与 `cockpit-agent start` 相同：

```bash
./cockpit agent -server wss://cockpit.cuihairu.site/ws -region home -zone datacenter
```

常用参数：

```bash
./cockpit-agent start \
  -server wss://cockpit.cuihairu.site/ws \
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
export ALLOWED_ORIGINS="https://cockpit.cuihairu.site"
export PRODUCTION=true
```

并将 Server 配置改为对外监听：

```yaml
server:
  host: 0.0.0.0
  port: 9000
```

建议通过反向代理终止 TLS，并让 Agent 使用 `wss://.../ws` 连接。

默认配置路径优先级：

1. `-config` 指定路径
2. `./config/cockpit.yaml`
3. `./cockpit.yaml`
4. `/etc/cockpit/config.yaml`

通知渠道（Herald / ntfy / webhook / Telegram）、DNS 与 ACME 凭据（Cloudflare / DNSPod / 阿里云）、组网云 token（ZeroTier / Tailscale）等完整键位见仓库 [`config/cockpit.yaml`](https://github.com/cuihairu/cockpit/blob/main/config/cockpit.yaml) 内注释；密钥类配置建议用环境变量注入（`CLOUDFLARE_API_TOKEN`、`DNSPOD_LOGIN_TOKEN`、`ALIYUN_ACCESS_KEY`、`ZEROTIER_API_TOKEN`、`TAILSCALE_API_TOKEN` 等）。

## 端到端冒烟

仓库内置最小闭环验证脚本（临时目录起 server + agent，sync inventory，校验资源 API 非空）：

```bash
./scripts/e2e-smoke.sh          # Linux/macOS
pwsh ./scripts/e2e-smoke.ps1    # Windows
```

退出码 0 表示全链路正常；非 0 时自动打印 `server.log` 末尾。保留日志调试：`E2E_KEEP_LOGS=1` / `-KeepLogs`。

## 不想编译？用 Docker

官方镜像由 CI 推到 `ghcr.io/cuihairu/cockpit`（多 tag：`latest` / `main` /
`v1.2.3` / 短 sha），部署机只要有 Docker 即可：

```bash
cp deployments/docker/.env.example deployments/docker/.env
vi deployments/docker/.env   # ADMIN_PASSWORD / JWT_SECRET 必填
docker compose -f deployments/docker/docker-compose.yml up -d
```

完整部署指南（tag 策略、远控 guacd、反向代理、备份回滚、排障）：
[Docker 部署](/operations/deploy-docker)。

## 下一步

- [核心概念](/guide/concepts)
- [架构与边界](/guide/architecture)
- [Agent 出口与 SD-WAN 能力边界](/guide/agent-egress-sdwan)
- [协议与 API 边界](/guide/protocol)
- [Docker 部署](/operations/deploy-docker)
