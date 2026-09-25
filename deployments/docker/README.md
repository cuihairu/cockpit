# Docker 部署

适合在服务器上直接运行 Cockpit Server。

本目录有两套部署姿势：

| 方式 | 镜像来源 | 适用 |
| --- | --- | --- |
| 本目录 [`docker-compose.yml`](./docker-compose.yml) | 拉 `ghcr.io/cuihairu/cockpit` | 部署机只要有 Docker，不在部署机上编译 |
| 仓库根 [`docker-compose.yml`](../../docker-compose.yml) | 本地 `build: .` | 改代码自测 |

完整部署步骤（镜像 tag 策略、远控 guacd 启用、反向代理、备份回滚、排障）见
**[docs/operations/deploy-docker.md](../../docs/operations/deploy-docker.md)**。

## 快速开始

```bash
git clone https://github.com/cuihairu/cockpit.git
cd cockpit

# 方式一：本地构建（改代码用）
cp deployments/docker/.env.example .env
vi .env
docker compose up -d --build

# 方式二：拉官方镜像（部署用）
cp deployments/docker/.env.example deployments/docker/.env
vi deployments/docker/.env
docker compose -f deployments/docker/docker-compose.yml up -d
```

访问：

```text
http://<server-ip>:9000
```

## 必改配置

- `ADMIN_PASSWORD`：管理员初始密码，不能使用示例值。
- `JWT_SECRET`：JWT 签名密钥，必须使用长随机字符串。
- `TOTP_ENCRYPTION_KEY`：`PRODUCTION=true` 时必须设置，至少 32 个字符；启用 TOTP 后必须妥善保存，丢失后无法解密已有 TOTP 密钥。
- `ALLOWED_ORIGINS`：对外域名，例如 `https://cockpit.example.com`。

生成随机值示例：

```bash
openssl rand -base64 32
```

如果直接使用 `.env.example` 中的占位值，容器会拒绝启动。

## 常用命令

```bash
docker compose ps
docker compose logs -f cockpit-server
docker compose restart cockpit-server
docker compose pull
docker compose up -d --build
```

数据保存在 Docker volume `cockpit-data` 中，包含 SQLite 数据库 `/data/cockpit.db`。

升级镜像前建议备份该 volume 或导出 `/data/cockpit.db`。

## 远控（guacd，可选）

RDP / VNC / SSH 远控入口需要同网络的 guacd（`--profile guacd` 启用）：

```bash
GUACD_ADDR=guacd:4822 docker compose --profile guacd up -d
```

要点：guacd 端口不对外暴露（浏览器走 `/api/remote/guacamole`，Go 网关同网络
反代）；`guacd-recordings` 卷必须 server 与 guacd **同卷同路径**（guacd 写
`.guac`，Go 网关收走归档到 `/data/recordings`）；`GUACD_LOG_LEVEL` 别开 debug。
详见 [deployments/guacd/README.md](../guacd/README.md) 与
[docs/remote-access-integration-design.md](../../docs/remote-access-integration-design.md)。

## Agent 连接

Agent 连接地址：

```text
ws://<server-ip>:9000/ws
```

如果你在反向代理后提供 HTTPS，请使用：

```text
wss://cockpit.example.com/ws
```

## 反向代理

如果使用 Nginx/Caddy/Traefik，请确保 WebSocket `/ws` 能正常升级转发。
