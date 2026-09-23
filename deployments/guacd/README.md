# guacd（Apache Guacamole 协议守护进程）

Guacamole 集成的协议引擎侧。见
[远程桌面设计](../../docs/remote-desktop-guacamole-design.md)。

## 它做什么（和不做什么）

`guacd` 是 Guacamole 的**协议翻译守护进程**（C 实现，Apache 2.0）：给它一条
`RDP → host:3389` 的指令，它去连并把 RDP/VNC 协议翻译成 Guacamole 指令流。

- **做**：RDP/VNC 协议终结（FreeRDP/VNC 客户端库）、指令流翻译、会话录制
  落盘（`.guac` 格式）
- **不做**：用户/权限/连接管理（那是 guacamole-web 的职责，我们跳过了——
  会话/权限/审计在 Cockpit server 的 Go 网关里）

## 部署

```bash
cd deployments/guacd
docker compose up -d
docker compose ps        # 应显示 healthy
```

端口默认绑 `127.0.0.1:4822`——**只给本机 Go 网关反代**，不对外暴露。
guacd 不是 HTTP 服务，浏览器永远走 Cockpit 的 `/api/remote/guacamole`
WebSocket 端点（由 Go 网关反代到此端口）。

跨机部署（guacd 与 Cockpit server 不同机）：

```bash
# .env
GUACD_BIND=192.168.5.20    # guacd 主机内网地址
GUACD_PORT=4822
```

并靠防火墙收口——只允许 Cockpit server 的 IP 访问 4822。

## 配置项（.env）

| 变量 | 默认 | 说明 |
|---|---|---|
| `GUACD_BIND` | `127.0.0.1` | 端口绑定地址（同机反代用回环；跨机用内网地址） |
| `GUACD_PORT` | `4822` | 端口（guacd 默认 4822，Go 网关侧 `GUACD_ADDR` 要对应） |
| `GUACD_LOG_LEVEL` | `info` | **别开 `debug`**——debug 会打印 `connect` 指令参数（含口令） |
| `TZ` | `Asia/Shanghai` | 时区 |

## 持久化卷

| 卷 | 路径 | 用途 |
|---|---|---|
| `guacd-recordings` | `/var/lib/guacamole` | **会话录制**（`.guac`）落盘目录。Go 网关在 `connect` 指令里传 `recording-path` + `recording-name`，guacd 自动录制——桌面审计回放的「白捡」红利，见设计风险章节 |
| `guacd-drive` | `/var/lib/guacamole/drive` | 阶段二文件传输（RDP 驱动器重定向）预留 |

guacd 本身无状态（连接参数每条 `connect` 指令传入，不落配置），容器可
随意重建；recording 卷是唯一持久化面。

## 版本对齐

`guacd` 版本要与前端 `guacamole-common-js` 版本匹配到**同一 Guacamole
发行版**（1.5.x 配 1.5.x）。Guacamole 协议是私有协议（无 schema），跨版本
不保证兼容——见设计风险章节「隧道二进制帧 chunk/base64 规则照抄官方
Tunnel 实现」。

升级只动本文件的 `image` tag，同时升级前端 `guacamole-common-js` 到对应版本。

## 验证连通

```bash
# 端口探活
nc -z 127.0.0.1 4822 && echo ok

# guacd 日志（应看到 "Listening on host 0.0.0.0, port 4822"）
docker compose logs guacd | tail
```

真实连接验证见设计文档「阶段 1 验收范围」（浏览器 RDP 连 Windows、
VNC 连 Linux、iPad Safari 触控；Win11 Home 无 RDP server 走 VNC 兜底）。
