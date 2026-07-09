# Docker 部署

适合在服务器上直接运行 Cockpit Server。

## 快速开始

```bash
git clone https://github.com/cuihairu/cockpit.git
cd cockpit
cp deployments/docker/.env.example .env
vi .env
docker compose up -d --build
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
