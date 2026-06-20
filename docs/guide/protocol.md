# 协议与 API 边界

本文记录当前 Server、Agent、Web UI 之间的通信边界。

## 通信面

| 通信面 | 发起方 | 入口 | 认证 |
| --- | --- | --- | --- |
| Web UI HTTP API | Browser | `/api/*` | JWT Bearer；登录和密码重置等公开接口除外 |
| Agent 控制通道 | Agent | `/ws` | 首条 `register` 消息；可选 Agent secret |
| 远程终端 | Browser | `/api/remote/terminal` | 短期 ticket，经 `Sec-WebSocket-Protocol` 传递 |
| VNC 透传 | Browser | `/api/remote/vnc` | 短期 ticket，经 `Sec-WebSocket-Protocol` 传递 |
| RDP 桌面 | Browser | `/api/remote/desktop` | 短期 ticket，经 `Sec-WebSocket-Protocol` 传递 |

## Agent WebSocket

Agent 使用 WebSocket 主动连接 Server：

```text
cockpit-agent start -server ws://server:9000/ws
```

连接建立后，第一条消息必须是 `register`。如果第一条消息不是注册消息，Server 会关闭连接。

消息 envelope：

```json
{
  "id": "msg_...",
  "type": "register",
  "timestamp": 1710000000,
  "payload": {}
}
```

### register

方向：Agent -> Server

```json
{
  "type": "register",
  "payload": {
    "agentId": "agent-home-server01",
    "secret": "optional-agent-secret",
    "location": {
      "region": "home",
      "zone": "datacenter"
    },
    "hostname": "server01",
    "capabilities": [
      {
        "type": "docker-api",
        "endpoint": "unix:///var/run/docker.sock"
      }
    ],
    "virtualization": {
      "type": "kvm",
      "role": "guest"
    },
    "labels": {
      "env": "home"
    }
  }
}
```

Server 接受后返回同类型消息：

```json
{
  "type": "register",
  "payload": {
    "status": "accepted",
    "serverTime": 1710000000,
    "heartbeatInterval": 30
  }
}
```

注册约束：

- `agentId` 为空时 Server 可生成 ID，但 Agent 当前也会基于主机名生成默认 ID。
- 如果数据库中已有该 Agent 且配置了 secret，注册必须提供匹配 secret。
- Registry 中已有同 ID 活跃连接时，新连接会被拒绝。

### heartbeat

方向：Agent -> Server

Agent 每 30 秒上报一次心跳。心跳可包含系统信息，Server 会写入系统指标历史和快照。

```json
{
  "type": "heartbeat",
  "payload": {
    "agentId": "agent-home-server01",
    "status": "online",
    "systemInfo": {
      "hostname": "server01",
      "cpuUsage": 12.3,
      "memUsagePercent": 48.1,
      "diskUsagePercent": 61.5,
      "osName": "linux",
      "arch": "amd64"
    }
  }
}
```

Server 返回 `heartbeat` ACK，并复用请求消息 ID。

### rpc_request / rpc_response

方向：Server -> Agent -> Server

RPC payload 使用 `method` 和 `params`。方法名按 `<provider>.<action>` 解析；没有 provider 前缀时默认 provider 是 `system`。

```json
{
  "type": "rpc_request",
  "payload": {
    "method": "system.status",
    "params": {}
  }
}
```

响应必须复用请求 ID：

```json
{
  "type": "rpc_response",
  "payload": {
    "status": "success",
    "data": {}
  }
}
```

注意：SystemProvider 在 Agent 启动时始终注册；PVE、Docker、OpenWrt Provider 由 `Agent.setupProviders` 按检测到的 capability 和环境变量（如 `PVE_URL`/`PVE_TOKEN_ID`/`PVE_TOKEN_SECRET`、`DOCKER_HOST`、`OPENWRT_HOST`/`OPENWRT_USER`/`OPENWRT_PASS`）条件注册。凭据缺失时该 Provider 仅记录日志跳过，不影响其他 Provider 和基础心跳。

## 代理消息

代理消息用于 Server 通过在线 Agent 访问 Agent 本机或其网络可达的 TCP/UDP 目标。

| 类型 | 方向 | 用途 |
| --- | --- | --- |
| `proxy_new` | Server -> Agent | 要求 Agent 建立到目标的连接 |
| `proxy_data` | 双向 | 转发连接数据 |
| `proxy_close` | 双向 | 关闭连接 |
| `proxy_error` | Agent -> Server | 报告代理错误 |

关键字段：

- `proxyId`：代理配置或会话 ID。
- `connId`：单条连接 ID。
- `target`：Agent 侧要连接的 `host:port`。
- `data`：数据内容；JSON 编码时可能是 base64 字符串或字节数组。

## 远程连接 ticket

Browser 不直接把 JWT 放进远程 WebSocket URL。流程是：

1. Browser 携带 JWT 调 `POST /api/remote/tickets`。
2. Server 生成 5 分钟有效、单次使用 ticket。
3. Browser 建立 WebSocket，把 ticket 作为 `Sec-WebSocket-Protocol` 的第一个值。
4. Server 校验并消费 ticket，然后开始转发。

创建 ticket：

```http
POST /api/remote/tickets
Authorization: Bearer <jwt>
Content-Type: application/json
```

```json
{
  "agent_id": "agent-home-server01",
  "host": "127.0.0.1",
  "port": 22,
  "protocol": "ssh"
}
```

响应：

```json
{
  "ticket": "random-ticket-id",
  "expires_at": "2026-06-11T12:00:00Z"
}
```

随后连接：

```javascript
new WebSocket("ws://server:9000/api/remote/terminal", [ticket])
```

## HTTP API

公开接口：

- `GET /health`
- `POST /api/auth/login`
- `POST /api/auth/refresh`
- `POST /api/auth/totp/verify`
- `POST /api/auth/forgot-password`
- `POST /api/auth/reset-password`
- `POST /api/auth/verify-reset-code`

JWT 保护接口：

- `GET /api/status`
- `GET /api/me`
- `PUT /api/me/profile`
- `POST|PUT /api/me/password`
- `GET|PUT /api/settings`
- `GET /api/agents`
- `GET /api/agents/{id}`
- `GET|POST|DELETE /api/agents/{id}/secret`
- `GET /api/resources/{type}`
- `GET /api/resources/{type}/{id}`
- `GET|POST /api/users`（创建限管理员）
- `PUT|DELETE /api/users/{id}`
- `POST /api/users/{id}/password`
- `GET /api/alerts`
- `PUT /api/alerts/read-all`
- `GET|PUT|DELETE /api/alerts/{id}`（`PUT /api/alerts/{id}/read` 标记已读）
- `GET /api/metrics/snapshots`
- `GET /api/metrics/snapshot?agent_id=...`
- `GET /api/metrics/history?agent_id=...`
- `GET|POST|PUT|PATCH|DELETE /api/proxies`
- `GET /api/proxies/status`
- `POST /api/remote/tickets`
- `GET|POST /api/remote/sessions`
- `GET|DELETE /api/remote/sessions/{id}`
- `GET /api/admin/audit/logs`
- `GET /api/admin/audit/stats`

TOTP 设置接口（同样需要 JWT，与上面的公开 `totp/verify` 区分）：

- `POST /api/auth/totp/generate`
- `POST /api/auth/totp/enable`
- `POST /api/auth/totp/disable`

资源类型当前支持：

- `compute-instances`
- `domains`
- `certificates`
- `services`
- `gateways`
- `storages`
