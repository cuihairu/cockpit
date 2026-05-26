# WebSocket Token 认证优化方案

## 当前问题

目前 VNC/RDP/SSH 终端通过 URL 查询参数传递认证 token：

```
wss://host/api/remote/vnc?agent_id=xxx&host=xxx&port=xxx&token=xxx
```

**存在的问题：**
1. Token 可能被记录在服务器访问日志中
2. Token 可能被浏览器历史记录保存
3. 不符合最佳安全实践

## 推荐方案

### 方案 A: WebSocket 子协议认证（推荐）

使用 WebSocket 子协议传递 token，避免出现在 URL 中。

**前端实现：**
```typescript
const ws = new WebSocket(url, ['cockpit-token', token]);
```

**后端验证（Go 示例）：**
```go
func WebSocketUpgrade(w http.ResponseWriter, r *http.Request) {
    // 从子协议获取 token
    protocols := websocket.Subprotocols(r)
    if len(protocols) >= 2 && protocols[0] == 'cockpit-token' {
        token := protocols[1]
        // 验证 token
    }
    // 升级为 WebSocket
}
```

### 方案 B: 短期连接 Token

1. 前端先通过 POST 请求获取短期有效的连接 token（30秒）
2. 使用连接 token 建立 WebSocket 连接
3. 连接 token 使用后立即失效

**API 设计：**
```
POST /api/remote/token
{
  "agent_id": "xxx",
  "host": "xxx",
  "port": 3389,
  "type": "rdp"
}
Response:
{
  "connection_token": "jwt_short_lived",
  "expires_at": "2024-01-01T00:00:30Z"
}
```

### 方案 C: Cookie 认证

如果前端和后端同域，可以使用 HttpOnly Cookie 传递认证信息，WebSocket 会自动携带。

## 实施建议

1. **短期：** 方案 B 最容易实施，只需新增一个 API 端点
2. **中期：** 方案 A 是最佳实践，需要后端 WebSocket 升级逻辑修改
3. **长期：** 考虑方案 C 配合 CSRF 保护

## 兼容性

为保持向后兼容，建议同时支持 URL 参数和新的认证方式，逐步迁移。
