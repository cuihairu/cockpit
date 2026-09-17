# P1 方案设计：Overlay 组网观测（ZeroTier / Tailscale / WireGuard / frp）

> 2026-09-17。个人基础设施跨地域组网观测首期设计。背景：个人设施天然分散在
> 云上、家里、公司等多地域，ZeroTier/Tailscale/WireGuard/frp 是把它们连成一网
> 的命脉。本文只做 M1（运行态只读观测）设计决策与接口定义；管理面（控制器
> API 成员授权）留 M2。

## 现状与痛点

| 痛点 | 现状 | 后果 |
|------|------|------|
| 组网运行态无处可查 | peer 掉线、延迟劣化、握手停滞都要逐台 SSH 敲命令 | 「家里设备连不上」排查靠猜 |
| Cockpit 已有探测但太浅 | `NetworkDetector`（`network-monitor`）只到 WireGuard peer 数量与 cloudflared 进程级 | 看不到虚拟 IP、延迟、版本、最近握手 |
| 多地域无统一视角 | 每台设备各自一个 `zerotier-cli` / `tailscale status` | 无法一屏对比「哪个地域的节点不在线」 |
| 连接路径无语义 | Server 只知 Agent 来自哪个 IP，不知其虚拟网身份 | 分布式探测/排障无法按路径解释 |

## 架构总览

与 cron / logs 同一通道模型：**同步 RPC + server 纯转发**。四类工具状态都
来自本机 CLI / 本地 admin API（毫秒到百毫秒级），30s RPC 超时绰绰有余。

```
┌─ server ───────────────────────┐         ┌─ agent（组网工具所在主机）──────────┐
│ GET /api/agents/{id}/overlay/  │ ──RPC─▶ │ overlay provider（新）               │
│   status  纯转发不落库          │ ◀─RPC── │ zerotier-cli -j / tailscale --json  │
│ Web /network 前端跨 agent 聚合  │         │ wg show dump(剥离密钥) / frp admin  │
└────────────────────────────────┘         └─────────────────────────────────────┘
```

跨地域视角由**前端聚合**实现：个人场景带 `overlay` 能力的 agent 是个位数，
页面逐 agent 拉取后在前端聚合成全网节点表，server 不落库、无聚合状态。

## 关键决策

| # | 决策 | 选择 | 理由 |
|---|------|------|------|
| D1 | M1 工具范围 | ZeroTier、Tailscale、WireGuard 自建、frp 四类只读观测 | 用户确认全做；四类覆盖个人组网绝大多数形态（中心化虚拟网 ×2、自建隧道、端口映射穿透） |
| D2 | 管理面 | M1 不做（ZeroTier Central / Tailscale API 的成员授权、除名留 M2） | 管理面引入 API token 配置与更高的误操作风险；观测先行验证数据模型 |
| D3 | capability | 新增 `overlay`，与既有 `network-monitor` 并存 | `network-monitor` 语义是「监控特征」，不动它避免回归；后续版本再把其 WireGuard 部分收敛进 `overlay` |
| D4 | 探测条件 | `zerotier-cli` / `tailscale` / `wg` / `frpc|frps` 任一 LookPath 通过 → `overlay` capability；provider 内各 reader 再自检，缺席工具返回 `unavailable` | 门在 detector、粒度在 reader；一台装两个工具也只注册一个 provider |
| D5 | RPC 方法 | 单方法 `overlay.status` 一次返回全部工具快照 | 四类都是只读快照，个人场景 peers 个位数到几十，一次拉齐省去前端多次往返；总量保护：每工具 peers 截断 200 |
| D6 | **密钥安全** | 输出字段**白名单构造**：`wg show dump` 的 private key / preshared key 字段一律不进响应；ZeroTier/Tailscale/frp 输出天然无密钥 | wg dump 文本含私钥与预共享密钥，白名单比黑名单删除可靠 |
| D7 | frp 分级观测 | 零配置：LookPath + 版本 + 进程存活；`COCKPIT_FRPC_ADMIN` / `COCKPIT_FRPS_ADMIN`（`host:port`）配置后调 admin API `/api/status` 拿隧道级状态 | frp 无状态 CLI；admin 端口是可选配置，不能为观测强求改用户 frp 配置 |
| D8 | REST 挂载 | `GET /api/agents/{id}/overlay/status`，serveAPI `/agents/` 分支，JWT；浏览类**不记审计**（同 logs/cron 浏览口径） | M2 出现授权/除名等变更操作时才引入审计 |
| D9 | 持久化 | 不落库——CLI 即事实源，server 纯转发 | 无状态同步问题；「全网视角」由前端逐 agent 拉取聚合 |
| D10 | CLI 执行 | 每命令 5s 超时（context），argv 直传不经 shell，输出 1MB 截断 | tailscale status 在大网下可达百 ms 级，5s 兜底足够；同 logs 的纪律 |

## 数据模型

```jsonc
// overlay.status 响应
{
  "tools": [
    {
      "tool": "zerotier",              // zerotier | tailscale | wireguard | frp
      "status": "ok",                  // ok | degraded | error | unavailable
      "version": "1.14.0",
      "error": "",                     // status != ok/unavailable 时的摘要
      "networks": [                    // zerotier / tailscale：本机加入的网络
        { "id": "8056c2e21c", "name": "home", "status": "OK",
          "addresses": ["10.147.20.5"], "type": "private" }
      ],
      "peers": [                       // 对端节点
        { "id": "7f3d0a9b12", "name": "nas", "virtualIps": ["10.147.20.2"],
          "version": "1.14.0", "latencyMs": 23, "online": true,
          "endpoint": "1.2.3.4/9993", "relay": "", "role": "peer",
          "lastHandshake": "2026-09-17T07:00:12Z" }
      ],
      "interfaces": [                  // wireguard：接口维度
        { "name": "wg0", "publicKey": "…=", "listenPort": 51820,
          "peerCount": 2, "peers": [ /* 同 peers 结构 */ ] }
      ]
    }
  ]
}
```

字段按工具取可用子集（解析器白名单构造，见 D6）；`lastHandshake` 统一 RFC3339
UTC；`online` 语义按工具：ZeroTier 取 active path 存在、Tailscale 取 `Online`、
WireGuard 取最近握手 < 3 分钟、frp 取 admin API 连通。

## Agent 侧设计

### 探测与注册

- `internal/agent/detector/overlay.go`：`OverlayDetector`，优先级与 NetworkDetector 相邻；
  四类任一可用 → `overlay` capability（metadata 携带各自可用标志）；
- providers.go：`case "overlay"` 注册 `rpc.NewOverlayProvider(nil)`（reader 注入
  `Commander` 同款抽象，测试可注入假输出）。

### RPC 方法

| 方法 | 参数 | 返回 | 说明 |
|------|------|------|------|
| `overlay.status` | `{}` | `{tools: [...]}` | 全部工具快照；单工具失败不影响其余（各自降级为 error/unavailable） |

### 各工具解析要点

| 工具 | 数据源 | 解析 | 安全 |
|------|--------|------|------|
| ZeroTier | `zerotier-cli -j listnetworks` / `-j listpeers` | JSON；peers 的 `paths[]` 取 `active` 且 `preferred` 的一条为 endpoint | 无密钥字段 |
| Tailscale | `tailscale status --json --peers`（旧版不认 `--peers` 则回退 `status --json`） | JSON；`Self` + `Peer{}`；`CurAddr` → endpoint、`Relay` → relay | 无密钥字段 |
| WireGuard | `wg show all dump` | 行式表格；interface 行取 listen port / pubkey，peer 行取 endpoint / allowed-ips / latest-handshake / transfer | **interface 行 private key、peer 行 preshared key 字段丢弃**（白名单字段另构输出） |
| frp | `frpc -v` / `frps -v`、`pgrep`、admin API `/api/status`（D7） | JSON；frps 取 proxy 汇总，frpc 取隧道表 | admin API 响应不含 token |

### 校验与保护

- 全部只读，无写回路径，无 symlink/路径风险；
- CLI 超时 5s（D10）；工具输出超 1MB 截断；
- peers > 200 截断并在工具级 `error` 附截断提示。

## Server 侧设计

### REST API（server/api_overlay.go，挂 serveAPI `/agents/` 分支，JWT）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/agents/{id}/overlay/status` | 纯转发 `overlay.status`；agent 离线 503；浏览器类不记审计 |

## Web UI 设计

新增 `/network`「组网」页面，两层视图：

- **全网总览**（跨地域视角，页面价值核心）：逐个拉取带 `overlay` 能力的
  在线 agent 状态，前端聚合为节点表——每行 = 一台对端节点，列 = 节点名、
  所在 agent、工具、虚拟 IP、延迟、在线态、版本、最近握手；同节点多路径
  出现时按「延迟最低」合并并展示来源徽标；
- **按 agent 分组**：每 agent 一个折叠面板，工具卡片（状态/版本/本机网络）
  + peers 明细表；离线 agent 显示最后未知态并标注。

组件沿 cron/proxy 页模式：agent 选择按 capability 过滤；轮询间隔跟随
全局 refreshInterval 设置。

## 不做（后续项）

- 管理面（ZeroTier Central / Tailscale API 成员授权、CMDB 对照发现未纳管设备）→ M2；
- Agent 上报自身虚拟网身份到 Register/心跳（连接路径语义、按路径选路）→ 独立设计；
- P2P 直连优化（server 与 agent 同网时 RPC 不经 server 中转）→ 远期；
- `network-monitor` 旧 capability 的收敛合并 → 后续清理版本。

## M1 清单

1. `internal/agent/detector/overlay.go` + 测试；
2. `internal/agent/rpc/overlay_provider.go`（四 reader + 解析器 + Commander 注入）+ 解析器 fixture 测试（含 wg 密钥剥离断言）；
3. `internal/agent/providers.go` 注册分支；
4. `internal/server/api_overlay.go` + 转发测试；
5. `web/src/pages/Network/`（总览 + 分组）+ 路由/菜单；
6. `go test ./...`、`pnpm run build`、`scripts/e2e-smoke.sh` 不回归。

## 参考

- ZeroTier CLI JSON：`zerotier-cli -j help`，服务端 API（M2）`api.zerotier.com/api/v1`
- Tailscale：`tailscale status --json`，API（M2）`api.tailscale.com`
- WireGuard dump 格式：`wg(8)`；frp admin API：frp `Admin Server` 文档
