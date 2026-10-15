# WebSocket 认证边界

当前实现已经避免在远程连接 WebSocket URL 中直接携带 JWT。远程终端、VNC、桌面连接统一使用短期 ticket：terminal 入口仅经 `Sec-WebSocket-Protocol` 传递，guacamole 入口 URL query `?ticket=` 与子协议双通道（query 优先、子协议兜底）。

## 当前流程

1. Web UI 使用 JWT 调用 `POST /api/remote/tickets`。
2. Server 生成内存态 ticket：
   - 有效期 5 分钟。
   - 单次使用。
   - 绑定用户和连接参数。
3. Web UI 建立 WebSocket 连接，并把 ticket 作为子协议值。
4. Server 校验并消费 ticket，随后才升级连接并转发数据。

```typescript
const ticket = await createRemoteTicket(params)
const ws = new WebSocket('/api/remote/terminal', [ticket])
```

对应服务端入口：

- `/api/remote/terminal`
- `/api/remote/guacamole`

terminal 入口从 `Sec-WebSocket-Protocol` 读取 ticket；guacamole 入口先读 URL query `?ticket=`、缺失再回退子协议。两者都不把认证 token 放进通用审计。

## 设计边界

- JWT 只用于创建 ticket 的 HTTP API，不直接进入远程 WebSocket URL。
- ticket 存在 Server 内存中，Server 重启后失效。
- ticket 被成功验证后会立即标记为已消费，不能复用。
- ticket 参数中可以包含 `agent_id`、`host`、`port`、`protocol`、`username`、`password`、`domain`、`width`、`height` 等连接参数。

## 安全注意

生产环境还需要配合以下配置：

- 设置 `ALLOWED_ORIGINS`，限制 WebSocket Origin。
- 使用 HTTPS/WSS 终止 TLS。
- 避免在日志中打印 ticket 或远程连接密码。
- 对远程连接和代理创建操作保留审计记录。

## 历史说明

早期文档讨论过 URL query token、短期连接 token、Cookie 认证等方案。当前代码采用“短期 ticket + WebSocket 子协议”的组合方式，是本文档应遵循的实现基线。
