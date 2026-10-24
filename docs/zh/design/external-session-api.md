# 外部新建远程会话接口契约（memex 接入，2026-10-15）


> 状态：**契约设计，未实现**（本文件先落档，实现随 memex 对接排期）。
> 远控三协议链路（RDP/VNC/SSH 走 Guacamole，`telnet` 走
> agent 通道）已全部落地：`docs/remote-access-integration-design.md` D1-D8。
> memex 做群内服务器授权（谁、被授权、哪台、什么权限），需要调用本仓真正
> 发起会话——本契约定义那个入口。

## 职责边界（先立后谈）

| 事项 | 归属 |
|------|------|
| 授权关系存储（谁能用哪台、什么权限） | **memex**（不进本仓，见 E7） |
| 凭据保管（口令/私钥/TOTP） | **persona**（本仓 vault 经 `docs/guide/persona-vault-mapping.md` 只读消费） |
| 凭据下发 | **不做**（凭据永不经过 memex，见 E2/E7） |
| 会话发起、票据、出口策略、录制、审计 | **本仓**（全部复用既有链路，零新增基础设施） |

## 决策

### E1 落档位置：独立文件，不并入 remote-access-integration-design.md

该文件是**已落地实现**的复核记录（D1-D8 均带实测验收），外部接口是
**未实现的新设计**——两类状态混排会让「以文档为准审码」时无法区分
幽灵功能与真实现。独立文件 + 状态头（未实现）保持口径干净；实现落地
后在本文件勾销状态头并回链。

## 契约

### E2 新建会话：`POST /api/external/remote/sessions`

**请求体（明确不含任何凭据字段）**：

```json
{
  "agent_id": "office-agent",
  "host": "db.internal",
  "port": 22,
  "protocol": "ssh",
  "credential_ref": "remote/memex/db.internal",
  "requested_by": "memex-user-17",
  "reason": "ops ticket #1042"
}
```

- `protocol` ∈ `ssh | rdp | vnc | telnet`（与 `handleRemoteSessionCreate` 白名单一致，
  `internal/server/api_remote.go:654-660`）。
- **无 `username`/`password`/`private_key`/`domain` 字段**——凭据由
  `credential_ref` 命名 vault 条目（local provider path 或 persona 条目
  UUID），会话建立时由服务端注入（E7 边界）。memex 全程接触不到凭据。
- `requested_by` 是 memex 侧终端用户标识，只进审计归属，不做鉴权主体
  （鉴权主体是 E3 的 caller token）。

**返回 201**：

```json
{
  "session_id": "…",
  "protocol": "ssh",
  "channel": "guacamole",
  "status": "pending",
  "expires_at": "2026-10-15T12:05:00Z",
  "connect": {
    "ticket": "…",
    "ws_url": "wss://<server>/api/remote/guacamole?ticket=…",
    "ticket_ttl_seconds": 300
  }
}
```

- `ticket` 为既有一次性票据（5 分钟、内存态、用后即焚，
  `internal/server/ticket.go`；`?ticket=` query 即
  `handleGuacamoleWebSocket` 现行入参，`internal/server/api_guacamole.go:308`）。
- `channel` 选择见 E6：`guacamole`（rdp/vnc/ssh）或 `terminal`
  （telnet 必走；memex 客户端说不了 guacamole 协议时 ssh 也可降级走
  `terminal`，即 D6 移动端同款链路 `wss://…/api/remote/terminal`，票据经
  **WebSocket 子协议**携带——`internal/server/api_remote.go:51-54`
  `protocols[0]` 即 ticketID，与 guacamole 通道的 `?ticket=` query 不同）。

**错误**（fail-closed，全部沿用既有语义）：

| 状态 | 场景 |
|------|------|
| 401 | caller token 缺失/错误 |
| 400 | 字段校验失败 / 协议不在白名单 |
| 403 | 目标不在 `remote_control` 白名单或 egress 策略拒绝（`matchRemoteEgress`，`internal/server/api_remote.go:460`） |
| 404 | agent 离线 / `credential_ref` 无对应条目 / **caller 配置为空（端点整体 404 默认关，E3）** |
| 503 | guacd 不可达（仅 guacamole 通道） |

### E3 调用方身份：命名服务 token

`remote_control` 配置扩一节（`config/cockpit.yaml:94` 起；结构体
`RemoteControlConfig`，`internal/config/config.go:140-156`）：

```yaml
remote_control:
  external_callers:
    - name: memex
      token: "${COCKPIT_EXTERNAL_TOKEN_MEMEX}"   # secret 不落 yaml，env 注入
      allowed_protocols: ["ssh", "rdp"]          # 可选，缺省全协议
      allowed_agent_ids: ["office-agent"]        # 可选，缺省不限
```

- 鉴权头沿用 webhook 先例：`X-Cockpit-Token`（常量时间比对；先例
  `internal/config/config.go:32-35`、`stacks.webhook_token`，决策
  D-2026-10-12-6）。**`external_callers` 未配置 = 端点 404，默认关**——
  与 webhook 同一红线。
- 人工 web 会话继续走 JWT；服务间调用不走用户 JWT（不冒充人、不共享
  会话态）。caller `name` + `requested_by` 共同构成审计归属。
- caller 级 `allowed_protocols`/`allowed_agent_ids` 是 memex 授权关系的
  **粗粒度护栏**，不替代 memex 的细粒度授权（E7）。

### E4 生命周期：查询与终止

- `GET /api/external/remote/sessions` —— 只返回**本 caller** 发起的会话
  （台账按 caller 名过滤；台账底座 `RemoteSessionManager`，
  `internal/server/api_remote.go:596-604` 为全量视图，外部入口加过滤）。
- `GET /api/external/remote/sessions/{id}` —— 单会话状态（pending/
  connected/closed/failed；台账登记口径见 续批九 D-2026-10-12-4）。
- `DELETE /api/external/remote/sessions/{id}` —— 终止：命中活会话先
  `terminateLiveSession`（`internal/server/api_remote.go:675`）后收口台账，
  同现有人工 DELETE 语义。**只能删自己发起的**，他方会话 404（不暴露存在性）。
- 票据 5 分钟一次性不变；会话不因 memex 断连而终止（连接归 memex 客户端，
  会话归台账）。

### E5 审计与录制覆盖结论

外部调用方走既有链路，**覆盖成立，零新增审计机制**；归属字段是唯一新增：

| 机制 | 覆盖 | 证据 |
|------|------|------|
| 审计 | `auditRemoteStart/End/Failure` 全协议同款；外部会话 `Username` 记 `memex:<requested_by>`（caller name 前缀），凭据不进详情 | `docs/remote-access-integration-design.md` D5；`api_remote.go:423` 起各失败路径 |
| 出口策略 | `matchRemoteEgress` agent_id 锚定，外部调用与人操同一套（403 fail-closed） | `api_remote.go:460-461` |
| 录制 | guacamole 通道 → guacd `.guac`（`collectGuacRecording`）；terminal 通道 → asciinema 字符流 | `docs/guide/recording-design.md` D1；`remote-access-integration-design.md` D5 |
| 会话台账 | Register/UpdateStatus/closed 全链路收口（续批九） | `internal/server/api_remote.go:642` 起既有 POST 创建 |

缺口（实现时补，均已含在契约内）：①审计归属前缀 `memex:` 需在会话创建时
写入台账（`RemoteSessionRequest` 加 `initiator` 字段）；②caller 级协议/
agent 护栏在 E2 校验链中先于 egress 执行。

### E6 协议通道选择（含移动端链路差异）

- `rdp/vnc` → `channel: guacamole`（像素流，memex 客户端需 guacamole 协议
  或直接嵌 web 前端）。
- `ssh` → `channel: guacamole`（默认，统一录制栈，D1/D3）；请求带
  `"channel_override": "terminal"` 时走 agent 通道字符流——即 D6 移动端
  同款（`/api/remote/terminal` + xterm 流）。Flutter 端无 guacamole 客户端
  是 D6 维持独立链路的原因，memex 客户端同理适用。
- `telnet` → 恒 `terminal`（D1：telnet 不进 guacd）。
- 契约不承载终端内容：memex 拿 `ticket` + `ws_url` 后由其客户端直连 WS，
  本仓不向 memex 转发/复用数据流。

### E7 不做的边界

1. **不做凭据下发**：契约任何字段不接收、不返回凭据；`credential_ref`
   只命名条目，运行时由服务端注入票据（`use_saved` 同语义，
   `internal/server/api_remote.go:484` 起）。memex 侧永远拿不到口令/私钥。
2. **不做授权关系存储**：谁能用哪台、什么权限是 memex + persona 的职责；
   本仓只执行自己的两道护栏（`remote_control` 目标白名单/egress + E3
   caller 护栏），不维护 memex 的授权矩阵。
3. 不做会话代理/多路复用、不做新协议、不做 memex 侧 UI 嵌入规范
   （票据即嵌入门票，5 分钟一次性是安全边界）。

## 实现锚点（供后续排期，全部为既有件）

`handleTicketCreate` 请求形状（`api_remote.go:399-411`）、egress 校验与
vault 注入顺序（`:460-500`：egress 先行、`use_saved` 后填）、会话台账
（`:642-669`）、票据（`internal/server/ticket.go`）、guacamole WS
（`api_guacamole.go:306-308`）、terminal WS（`api_remote.go:387`）。新代码
预期只在：外部路由 + caller 鉴权中间件 + `credential_ref` 解析 +
台账 `initiator` 字段。
