# Docker 部署指南

用官方发布的镜像（`ghcr.io/cuihairu/cockpit`）在服务器上跑 Cockpit Server。
面向「拿到一台服务器要把它跑起来」的部署者；改代码 locally 起服务见
[快速开始](../guide/getting-started.md)。

## 两种部署姿势先说清

| | 用途 | 镜像来源 | compose 文件 |
| --- | --- | --- | --- |
| 拉镜像部署（本文） | 生产/长期运行 | `ghcr.io/cuihairu/cockpit:<tag>` | `deployments/docker/docker-compose.yml` |
| 本地构建 | 改代码自测 | 本机 `docker build` | 仓库根 `docker-compose.yml` |

两份 compose 的服务定义同源（server + 可选 guacd），差别只在镜像来源。

## 前置要求

- Docker Engine 20.10+ 与 Compose V2（`docker compose version` 能跑）；
- 1 核 1G 内存起（远控 guacd 另算，建议再留 512M）；
- 对外 9000/tcp（或只绑回环、走反代）。

## 快速开始

```bash
# 1. 准备变量文件
cp deployments/docker/.env.example deployments/docker/.env
vi deployments/docker/.env        # 至少改 ADMIN_PASSWORD / JWT_SECRET / TOTP_ENCRYPTION_KEY

# 2. 起服务（锁 tag 更稳；留空则跟 latest）
cd deployments/docker
COCKPIT_IMAGE_TAG=latest docker compose up -d

# 3. 看状态
docker compose ps
curl -fsS http://127.0.0.1:9000/health
```

打开 `http://<服务器 IP>:9000`，用 `.env` 里的 `ADMIN_USERNAME` /
`ADMIN_PASSWORD` 登录。

生成随机值：

```bash
openssl rand -base64 32
```

## 镜像与 tag

镜像由 `.github/workflows/docker.yml`
在 GitHub-hosted runner 上用 buildx 构建并推到 GHCR。一次构建打多组 tag：

| tag | 含义 |
| --- | --- |
| `latest` | `main` 分支最新（每次合入 main 覆盖） |
| `main` | 同上，显式引用分支名 |
| `v1.2.3` / `1.2.3` / `1.2` / `1` | 打 `v*` tag 时的语义化版本系列 |
| `<短 sha>` | 每次构建的不可变快照，回滚用这个 |
| `pr-123` | PR 构建（不推送 registry，仅验证能构建） |

**部署建议锁 sha 或语义化版本 tag**（`COCKPIT_IMAGE_TAG=<sha>`），别长期跟
`latest`——`latest` 会随 main 前进，回滚时说不清回到哪一版。

```bash
docker compose pull && docker compose up -d
```

镜像只出 `linux/amd64`：Server 走 CGO（SQLite 绑死在 cgo），多架构要交叉
工具链链。需要 arm64 请用二进制发行包（`nightly.yml` / Releases）。

查看镜像里内置的版本号：

```bash
docker compose exec cockpit-server cockpit version
```

## 环境变量

完整清单见 `deployments/docker/.env.example`。

**必改**（占位值会让容器拒绝启动，`entrypoint.sh` 硬校验）：

| 变量 | 说明 |
| --- | --- |
| `ADMIN_PASSWORD` | 管理员初始口令，≥8 字符，不能是示例值 |
| `JWT_SECRET` | JWT 签名密钥，长随机串 |
| `TOTP_ENCRYPTION_KEY` | `PRODUCTION=true` 时必填，≥32 字符。**启用 TOTP 后必须备份**，丢了已有密钥解不开 |
| `ALLOWED_ORIGINS` | 对外域名，如 `https://cockpit.example.com`（反代部署必填） |

**可选**：

| 变量 | 说明 |
| --- | --- |
| `COCKPIT_IMAGE_TAG` | 镜像 tag，缺省 `latest` |
| `COCKPIT_HTTP_PORT` | 宿主机端口，缺省 9000 |
| `ADMIN_USERNAME` | 缺省 `admin` |
| `PRODUCTION` | 缺省 `true`（放开 Host/Origin 校验等开发态便利） |
| `BASE_URL` | 对外基址，邮件/通知里拼链接用 |
| `TZ` | 缺省 `Asia/Shanghai` |
| `GUACD_ADDR` | guacd 地址（启用远控时设 `guacd:4822`） |
| `GUACD_RECORDING_PATH` | guacd 录制目录，缺省 `/var/lib/guacamole` |
| `GUACD_LOG_LEVEL` | 缺省 `info`，**别开 `debug`**（会打印 connect 指令参数，含口令/私钥） |

## 远控栈（guacd，可选）

RDP / VNC / SSH 三个协议的远控入口需要同网络的 `guacd` 守护进程
（浏览器侧 `guacamole-common-js` + Go 网关反代，详见
[远控设计](../remote-desktop-guacamole-design.md) 与
[三协议统一栈](../remote-access-integration-design.md)）。

```bash
# .env 里加一行（同网络 DNS 名，不要写 127.0.0.1——那是 guacd 容器自己）
GUACD_ADDR=guacd:4822

docker compose --profile guacd up -d
```

要点：

- **端口不对外暴露**。guacd 不是 HTTP 服务，浏览器只走 Cockpit 的
  `/api/remote/guacamole` WebSocket，由 Go 网关同网络拨 guacd。
- **录制卷必须同卷同路径**。compose 已把 `guacd-recordings` 同时挂给两个
  容器（都在 `/var/lib/guacamole`）：guacd 按 `connect` 指令的
  `recording-path` 写 `<session>.guac`，Go 网关在会话结束时把同一路径的
  文件收走归档进 `/data/recordings`（`collectGuacRecording`）。两侧路径
  对不上，录制就永远收不到，`/recordings` 页面看不到桌面/终端会话。
  改了 `GUACD_RECORDING_PATH` 就要同步改两个容器的挂载点。
- guacd 不可达时 SSH 仍有退路：Workbench 的 SSH Tab 保留「内置终端（经
  Agent）」次入口（走 agent 通道 + xterm.js，不经 guacd）。

## 反向代理

反代必须放行两类 WebSocket 升级，否则 Agent 掉线、远控建不起来：

| 路径 | 用途 |
| --- | --- |
| `/ws` | Agent 长连接 |
| `/api/remote/guacamole` | Guacamole 隧道（RDP/VNC/SSH） |
| `/api/remote/terminal`、`/api/remote/desktop`、`/api/remote/vnc` | 自研/兜底远控通道 |

Nginx 片段：

```nginx
location ~ ^/api/remote/(terminal|desktop|vnc|guacamole) {
    proxy_pass http://127.0.0.1:9000;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_read_timeout 86400;   # 远控会话长时间静默，超时别设太小
}
```

仓库里有一份完整可用的站点配置：`deployments/nginx/cockpit.cuihairu.site.conf`。

HTTPS 下浏览器会用 `wss://`，Go 网关同源反代，无需额外配置。

## 数据与备份

数据全在 volume `cockpit-data` 挂载的 `/data`：

| 路径 | 内容 |
| --- | --- |
| `/data/cockpit.db` | SQLite 主库（账号、配置、审计、会话记录…） |
| `/data/inventory.yaml` | 采集清单 |
| `/data/recordings/` | 会话录制归档（终端 `.cast`、桌面/远控 `.guac`） |

备份与回滚：

```bash
docker run --rm -v cockpit-data:/data -v "$PWD":/backup debian:bookworm-slim \
  tar czf /backup/cockpit-data-$(date +%F).tar.gz -C /data .
```

升级前做一次。回滚 = 把 `COCKPIT_IMAGE_TAG` 改回旧 tag/sha 再
`docker compose up -d`（数据库向前兼容，不自动降级迁移）。

## 从源码构建

要在部署机本地构建（离线、无 registry 访问时）：

```bash
docker compose up -d --build          # 仓库根 compose，镜像名 cockpit:local
# 或单独构建
docker build -t cockpit:local --build-arg VERSION=$(git rev-parse --short HEAD) .
```

`--build-arg VERSION` 会注入二进制的 `cockpit version`（`main.version`）。
镜像内的 pnpm 版本已钉死（`pnpm@11.19.0`），与 lockfile 匹配。

## 排障

| 现象 | 排查 |
| --- | --- |
| 容器起不来，日志 `ADMIN_PASSWORD must be set...` | `.env` 里还是占位值；`entrypoint.sh` 故意拒绝弱口令启动 |
| `/health` 不通 | `docker compose logs cockpit-server`；确认 9000 未被占；`ALLOWED_ORIGINS` 与实际访问域名不符时静态资源会被浏览器拦 |
| 远控入口不显示 | `GUACD_ADDR` 没设或设错。留空 = 前端隐藏 RDP/VNC/SSH 入口（预期行为） |
| 远控点了连不上，日志 `dial guacd ... failed` | guacd 容器没起（`--profile guacd` 漏了）或 `GUACD_ADDR` 写成了 `127.0.0.1`（那是 guacd 容器内的回环，Go 网关拨不到） |
| `/recordings` 里没有远控会话录制 | guacd 录制卷没与 server 共挂（见「远控栈」一节） |
| 反代后远控握手失败 | 漏了 `/api/remote/guacamole` 的 Upgrade 头（见「反向代理」） |
| 终端/桌面会话中途断 | 反代 `proxy_read_timeout` 太小 |

## 相关文档

- [快速开始](../guide/getting-started.md) —— 二进制部署与本地起服务
- 部署资产说明 `deployments/docker/README.md` —— 变量与脚本的逐项解释
- [真机验收清单](../guide/acceptance-checklist.md) —— 上线后的功能验收
- [远控设计](../remote-desktop-guacamole-design.md) /
  [三协议统一栈](../remote-access-integration-design.md) —— guacd 接入细节
