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

---

# M2 方案设计：云端管理面 + 身份上报 + 旧能力收敛

> 2026-09-19。M1 只读观测验证了数据模型与转发链路后，M2 补齐三块：
> **A. 云管理面**（ZeroTier Central / Tailscale API 成员授权与除名，CMDB 对照）、
> **B. agent 虚拟网身份上报**（Register 时携带，连接路径语义 + CMDB 对照的匹配键）、
> **C. `network-monitor` 旧 capability 收敛**。A 与 B 互为表里：没有 B 的身份
> 上报，A 的「未纳管设备」对照没有锚点。

## 架构总览

管理面走 **server 直连云 API**（与 DNS M1 Cloudflare、ACME 同款出站路径），
agent 不参与；观测走 M1 既有 agent RPC，不变。

```
┌─ server ─────────────────────────────┐      ┌─ 云端控制面 ──────────────┐
│ overlay 云客户端（新，internal/overlay）│─HTTPS▶│ api.zerotier.com/api/v1   │
│ GET /api/overlay/cloud   列表+对照     │      │ api.tailscale.com/api/v2  │
│ POST/DELETE 变更（授权/除名）+ 审计     │      └───────────────────────────┘
└──────────┬───────────────────────────┘
           │ Register（既有管道）              ┌─ agent ───────────────────┐
           ▼                                 │ overlay capability metadata│
│ registry（capabilities 已全量存储）         │ + identity（新，M2-B）      │
│ /api/agents 既有输出带 metadata → web 对照  │ zerotier-cli / tailscale   │
└──────────────────────────────────────┘      └───────────────────────────┘
```

CMDB 对照是**只读推导**：云端成员列表 × agent 上报身份，前端 join，不落库、
无聚合状态——与 M1「全网视角前端聚合」同一哲学。

## 关键决策

| # | 决策 | 选择 | 理由 |
|---|------|------|------|
| D11 | 管理面架构 | server 直连云 API，agent 不参与 | 管理凭据是账号级（一份 token 管全网成员），经 agent 中转徒增泄露面与部署耦合；观测（M1）在 agent、管理（M2）在 server，各自贴近事实源 |
| D12 | 云凭据装载 | config.yaml `overlay.zerotier.api_token` / `overlay.tailscale.api_token`；env `ZEROTIER_API_TOKEN` / `TAILSCALE_API_TOKEN` 优先（secret 不落 yaml，同 DNS 纪律）；Tailscale 另有 `overlay.tailscale.tailnet`（默认 `-`，部分 token 形态要求显式 tailnet 名） | 完全复刻 DNS Cloudflare 模式；未配置统一 503 + 指名缺哪个键的引导文案；token 绝不进响应/错误/审计 |
| D13 | API 面 | 读 `GET /api/overlay/cloud` 一次返回两 provider（未配置段 `configured:false`，单 provider 失败降级不整体 5xx）；变更四端点：`POST /api/overlay/cloud/zerotier/networks/{nwid}/members/{mid}`（body `{authorized:bool}`）、`DELETE .../members/{mid}`、`POST /api/overlay/cloud/tailscale/devices/{devid}/authorize`、`DELETE .../devices/{devid}` | 个人场景两 provider 一次拉齐省往返；读降级写精确；与 DNS `/api/dns` 同款路径组织 |
| D14 | 变更审计 | 授权/取消授权记 `overlay_authz`、除名记 `overlay_remove`，resourceID=网络/设备与成员 ID，detail 只记布尔/名称，绝无 token | M1 D8 预告「变更操作引入审计」兑现；命名沿用 cron_apply/proxy_apply 风格 |
| D15 | 身份上报位置 | `overlay` capability `metadata.identity`（Register 既有管道，零新增协议字段）；范围仅 ZeroTier + Tailscale | 中心化虚拟网才有云管理面，CMDB 对照语义只在两者成立（WireGuard 自建无控制面、frp 无成员概念）；metadata map 已随注册全量到 registry 与 /api/agents 输出，server/web 零存储改动 |
| D16 | 身份提取实现 | detector 包内新文件 `overlay_identity.go` 轻量解析（`zerotier-cli -j info` + `-j listnetworks`、`tailscale status --json`），白名单字段，每命令 2s 超时，**失败静默跳过**（identity 键缺席 ≠ 探测失败，不阻塞注册） | 不搬 rpc 包整套解析器（避免包依赖调整，身份只需极少字段）；注册路径上的探测必须可失败（同 ddns/file「标准库轻实现」纪律）；刷新时机 = 重连重注册，心跳不携带——membership 低频变化，运行态权威在每次 overlay.status 实时拉取 |
| D17 | CMDB 对照 | 匹配键：ZeroTier = member id ↔ agent 身份 node id（同为 10 位 hex，天然同键）；Tailscale = device id ↔ Self.ID；三态标注 `managed`（有对应 cockpit agent）/ `unmanaged`（云端有、面板无） / 数量汇总 | member/device id 由控制面分配且 agent 侧 CLI 原样上报，无映射歧义；`unmanaged` 是本设计核心卖点——发现未纳管设备；「agent 报了但云端没有」多为 token 范围问题，归入引导文案不做独立状态 |
| D18 | `network-monitor` 收敛 | **删除** `detector/network.go` 的 NetworkDetector 与 `GetNetworkInterfaces`：capability 无 server/web 任何消费方（grep 验证），metadata 无下游 | M1 D3 预告收敛兑现；同仓同发无兼容包袱（service-design D9 重命名 systemd→service 先例）；cloudflared 隧道观测留后续按 frp D7 分级观测模式并入 overlay，M2 不做 |
| D19 | ID 校验 | server 单端白名单：ZeroTier network id `^[0-9a-f]{16}$`、member id `^[0-9a-f]{10}$`、Tailscale device id `^[0-9]{1,20}$`，非法直接 400 | 云 API 是出站调用，不校验会被当作请求路径注入；无 agent 转发故无双端规则，单端即可 |
| D20 | 云客户端实现 | `internal/overlay` 新包（ZeroTier/Tailscale 两 client + `NewXxxWithBase` httptest 注入变体），15s 超时，成员/设备输出白名单构造（token/密钥类字段天然不存在，防御性不透传原始 JSON） | 与 dns 包同构；WithBase 变体是 DNS M2 验证过的全 httptest 测试路径 |

## 云端数据模型（白名单构造）

```jsonc
// GET /api/overlay/cloud 响应
{
  "zerotier": {
    "configured": true,
    "error": "",                    // configured 但拉取失败时的降级摘要
    "networks": [                   // Central API /network，只读列出（管理粒度在成员）
      { "id": "8056c2e21c", "name": "home", "members": [
        { "id": "7f3d0a9b12", "name": "nas", "authorized": true,
          "online": true, "ips": ["10.147.20.2"], "version": "1.14.0",
          "lastSeen": "2026-09-19T07:00:12Z", "managed": true }
      ]}
    ]
  },
  "tailscale": {
    "configured": false,            // 未配置 → 其余字段缺席，前端出引导卡
    "tailnet": "-",
    "error": "",
    "devices": [
      { "id": "1234567890", "name": "nas.tailnet-name.ts.net",
        "addresses": ["100.64.0.2"], "user": "me@example.com",
        "os": "linux", "authorized": true, "online": true,
        "keyExpiry": "2027-01-01T00:00:00Z", "lastSeen": "…", "managed": true }
    ]
  }
}
```

`managed` 由 server 在拉取时与 registry 内 agent `metadata.identity` 对照计算
（D17 匹配键），web 零推导——registry 是 server 进程内数据，server 算比前端
再拉一遍 /api/agents 省一次往返；对照失败（registry 空）一律 `managed:false`
不猜测。

## Agent 侧（M2-B）

### identity 提取（detector/overlay_identity.go）

| 工具 | 命令 | 提取字段（白名单） |
|------|------|-------------------|
| ZeroTier | `zerotier-cli -j info` | `address`（= node id）、`version` |
| ZeroTier | `zerotier-cli -j listnetworks` | 每网 `id`、`name`、`status`、`assignedAddresses` |
| Tailscale | `tailscale status --json` | `Self.ID`、`Self.HostName`、`Self.DNSName`、`Self.Tailscale IPs` |

- 每命令 2s 超时；任一失败该工具 identity 缺席，不影响另一工具与其余探测；
- 输出结果写入 `overlay` capability `metadata.identity`：
  `{zerotier: {nodeId, networks: [{id,name,status,addresses}]}, tailscale: {id, hostName, dnsName, addresses}}`；
- web 在 agent 分组视图渲染身份 chip（网络名 + 本机虚拟地址）。

### 不做

- 心跳携带 identity（D16：低频变化，重注册刷新足够）；
- WireGuard/frp 身份（D15：无云管理面对照语义）。

## Server 侧（M2-A）

### internal/overlay 云客户端

| 方法 | ZeroTier Central（api.zerotier.com/api/v1） | Tailscale（api.tailscale.com/api/v2） |
|------|---------------------------------------------|----------------------------------------|
| 列表 | `GET /network` + 逐网 `GET /network/{id}/member` | `GET /tailnet/{tailnet}/devices` |
| 授权 | `POST /network/{nwid}/member/{mid}` body `{authorized}` | `POST /device/{id}/authorize` |
| 除名 | `DELETE /network/{nwid}/member/{mid}` | `DELETE /device/{id}` |

- Bearer token 认证；15s 超时（同 DNS client）；成员/设备解析白名单构造（D20）；
- 两 client 均 `NewXxxWithBase` 变体支撑 httptest 全流程测试；
- 网络名：ZT `/network` 列表项取 `config.name`（顶层 id 同取）；成员名取 `name`
  （Central 显示名），缺省回退 node id 前六位展示。

### REST API（server/api_overlay_cloud.go，JWT）

| 方法 | 路径 | 审计 |
|------|------|------|
| GET | `/api/overlay/cloud` | 否（浏览类，同 M1 D8） |
| POST | `/api/overlay/cloud/zerotier/networks/{nwid}/members/{mid}` | `overlay_authz`（detail 含 authorized 布尔） |
| DELETE | `/api/overlay/cloud/zerotier/networks/{nwid}/members/{mid}` | `overlay_remove` |
| POST | `/api/overlay/cloud/tailscale/devices/{devid}/authorize` | `overlay_authz` |
| DELETE | `/api/overlay/cloud/tailscale/devices/{devid}` | `overlay_remove` |

- 未配置对应 provider：读 503 带引导文案（指名缺哪个键，同 requireDNS 各报各的）；
  变更同样 503 不透传到云端；
- 云端 4xx/5xx 原样映射 HTTP 状态并透出云端错误摘要（脱 token）；
- 变更成功后 web 局部刷新 `GET /api/overlay/cloud`。

## Web UI（/network 页扩展）

- 页面顶部 **Segmented「运行态观测 / 云端管理」**（默认观测，M1 视图不动）；
- **云端管理**视图：
  - 未配置 provider 出引导卡（配置方法 + env 优先提示，同 DNS/acme 引导卡样式）；
  - ZeroTier：按 network 分组的成员表（成员名/ID/授权态 Switch/在线/虚拟 IP/
    版本/最后在线/「面板纳管」徽标 + Tooltip 解释）；行操作：授权 Switch
    （即时 POST）+ 除名 Popconfirm（member id = node id，设备重新 join 后
    可再授权，可逆性够 Popconfirm 级）；
  - Tailscale：设备表（名称/地址/归属 user/系统/在线/授权态/keyExpiry 过期高亮/
    纳管徽标）；行操作：authorize 按钮 + **除名 Modal 输入设备名确认**
    （设备删除需重新登录，破坏性高于 ZT，按 stack 删除确认模式）；
  - 顶部对照汇总条：「云端 n 台设备，m 台未纳管」（unmanaged 展开提示）；
- **运行态观测**视图补一处：agent 分组卡显示 identity chip（来自 capability
  metadata，M2-B 上报）。

## M2 清单

1. [`internal/agent/detector/overlay_identity.go` + 提取测试（注入假命令、失败静默、字段白名单）；]
2. [删除 `detector/network.go` + 相关测试同步清理（M2-C，独立小提交可并入 feat(agent)）；]
3. [`internal/overlay`（两 client + WithBase）+ httptest 全流程/降级/白名单测试；]
4. [`internal/server/api_overlay_cloud.go` + 端点测试（校验 D19/审计/未配置 503/降级/managed 对照）；]
5. [`web/src/pages/Network/` 云端管理视图 + 观测视图身份 chip；]
6. [`go test ./...`、`pnpm run build` 不回归；设计文档清单勾选 + todo.md 条目更新。]

## M2 落地差异补记

- **未配置 provider 的读语义**：Server 侧 REST 小节写「读 503 带引导文案」，实现按 D13
  段级语义——`GET /api/overlay/cloud` 恒 200，未配置段 `configured:false`、失败段段内
  `error`（引导卡由 web 依 `configured` 渲染）；503 仅用于变更操作（指名缺哪个 env/yaml
  键）。D13 是决策层、小节是实现描述，以决策为准，不回改原文。
- **云端错误状态映射**：设计写「4xx/5xx 原样映射 HTTP 状态」，实现统一映射 **502** +
  云端错误摘要（200 字符截断，脱 token），与 DNS M2 的上游错误处理同构；段级 `error`
  字段承载摘要原文，状态细节不丢——上游不可达属网关语义，透传 401/403 反而误导为
  用户对 cockpit 的认证问题。
- **汇总条语义补强**：「云端 n 台设备，m 台未纳管」在 m=0 时显示为 info 级「全部与
  面板 Agent 身份对上」，正向确认对照链路健康，而非仅报忧。

## 参考

- ZeroTier Central API v1：`https://api.zerotier.com/api/v1`（`GET /network`、
  `POST/DELETE /network/{id}/member/{memberId}`，Bearer token）
- Tailscale API v2：`https://api.tailscale.com/api/v2`（`GET /tailnet/{tailnet}/devices`、
  `POST /device/{id}/authorize`、`DELETE /device/{id}`，Bearer token；`tailnet=-`
  为 API 生成文档支持的默认 tailnet 简写，实现以真机 token 实测为准）
