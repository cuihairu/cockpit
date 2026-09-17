# DDNS（动态域名解析）设计

## 背景

家庭宽带公网 IP 会周期性变化，DDNS 让「域名 → 家庭出口 IP」保持同步，是在外访问 NAS/家庭服务的第一道依赖。Cockpit 已具备两个可复用基础：

- **DNS 写路径**（`docs/guide/dns-design.md`）：server 直连 Cloudflare API v4，`dns.Provider` 接口（ListRecords/Create/Update/Delete）与 token 管理（config.yaml / env，secret 不落盘）现成；
- **巡检循环模式**（drift → smart 两轮验证）：ticker + Setting 动态间隔 + 真去重告警。

缺的一块是「家里现在的公网 IP 是什么」——server 在云端，它看到的 WebSocket 对端地址不可靠（见 D2），IP 探测必须发生在家庭侧的 agent 上。

## 决策

- **D1 架构分工**：agent 只回答「本机所在网络的出口 IP」（新 RPC `ddns.ip`）；server 持有 DDNS 记录配置、跑巡检循环、调 Cloudflare 写记录。理由：DDNS 的目标 IP 在被观测网络内；而 DNS 写需要 token，token 只在 server（dns-design D2/D7）。
- **D2 不用连接对端地址**：server 从 WebSocket `RemoteAddr` 拿到的理论上是家庭出口 IP，但 agent 可能经代理/overlay 出站，该地址语义不可靠；agent 主动探测（出站 HTTP 访问回显服务）语义清晰——「这台机器看到的出口 IP」。
- **D3 IP 探测多源兜底**：回显服务国内可达性参差，按序尝试、取首个成功：
  - IPv4：`api.ipify.org` → `api-ipv4.ip.sb` → `ipv4.icanhazip.com` → `4.ipw.cn`
  - IPv6：`api6.ipify.org` → `api-ipv6.ip.sb` → `ipv6.icanhazip.com` → `6.ipw.cn`
  - 单源超时 5s、一族总超时 15s；响应 trim 后必须过 `net.ParseIP` 且地址族匹配才采纳。源列表做成包级变量，测试可注入（httptest 假源）。
- **D4 capability `ddns` 恒上报**：照 `file` capability 先例（标准库出站 HTTP，无外部依赖，无需 detector），在 `detectCapabilities` 里无条件追加；provider 提供 `ddns.ip` 单方法，返回 `{ipv4?, ipv6?}`（探测失败该族省略，不报错——DDNS 巡检按族取用）。是否作为 DDNS 的 IP 源由 server 侧配置绑定 agent 决定，与 capability 无关。
- **D5 配置表 `DDNSConfig`**（SQLite，复刻 BackupConfig 模式）：`ID / AgentID / ZoneID / ZoneName / RecordName（如 home.example.com）/ Type（A|AAAA）/ Enabled / LastIP / LastUpdatedAt / CheckedAt / LastStatus（ok|failed|never）/ LastError`。storage 层 CRUD 五函数。
- **D6 巡检循环**：`ddnsScanLoop` 复刻 drift/smart 模式；Setting 键 `ddns.scan_interval_seconds`，默认 300，min 60，max 86400，`0 = 关闭`。每轮：enabled 配置 → agent 在线 → `ddns.ip` 取对应族 IP → 每 zone 一次 `ListRecords(zone, type)` 找 name 匹配的记录 → 记录缺失则 `Create` → IP 变化则 `Update` → 回写表（LastIP/LastStatus/CheckedAt）。取不到 agent IP 只置 failed 状态，不动 DNS。
- **D7 API 配额保护**：仅当「IP 变化」或「记录缺失」才写 Cloudflare；每轮每 zone 至多一次 ListRecords；同轮内同 zone 的多条配置共享查询结果。
- **D8 告警**：巡检失败（agent 离线 / 探测不出 IP / Cloudflare 报错）→ `alert.Generator.CheckDDNS` 真去重 warning，title 按记录名（与 CheckDiskHealth 同构）。成功更新不告警（静默自愈），表内可见变更。
- **D9 审计边界**：用户操作（增删改 DDNS 配置）记审计 `ddns_create / ddns_update / ddns_delete`；巡检的自动 DNS 变更不记审计——它不是用户操作，且 `DDNSConfig.LastIP/LastUpdatedAt` 已完整记录变更轨迹（与 drift/smart 巡检不记审计同则）。
- **D10 REST**（全部 `/api/ddns` 前缀）：
  - `GET /api/ddns` 配置列表；`POST /api/ddns` 创建；`PUT /api/ddns/{id}` 更新；`DELETE /api/ddns/{id}` 删除（用户操作，记审计）
  - `POST /api/ddns/{id}/check` 立即同步检查单条（不等巡检周期），返回检查结果
  - 创建/更新校验：Type ∈ {A, AAAA}、RecordName 非空且过 `dns.ValidateInput` 同规则、AgentID 非空。zone 下拉复用已有 `GET /api/dns/zones`。
- **D11 前端**：DNS 页改 Tabs——「记录管理」（现有内容）+「DDNS」。DDNS Tab：配置表（记录名/类型/绑定主机/当前 IP/状态徽标/最后检查时间）+ 新建/编辑 Modal（agent 下拉在线优先、zone 下拉、记录名、类型、启用 Switch）+ 立即检查按钮 + 巡检间隔设置（与 smart/drift 页同模式）。
- **D12 安全**：探测到的 IP 必须过 `net.ParseIP` + 地址族匹配才进表、进 DNS；RecordName 复用 `dns.ValidateInput` 校验面；token 不出 server（D1 推论）。

## 数据结构

`ddns.ip` 返回：

```json
{ "ipv4": "203.0.113.7", "ipv6": "2001:db8::1" }
```

`DDNSConfig`（storage 表）与 REST JSON 一一对应，`LastStatus`：
- `never`：从未检查（新建后）
- `ok`：上轮检查成功（含「IP 无变化」的安静轮）
- `failed`：上轮失败（LastError 带原因）

## 实现切分（M1，本任务）

1. **docs**：本文档；
2. **agent**：`rpc/ddns_provider.go`（多源 IP 探测，Commander 不适用——出站 HTTP，http.Client 可注入）+ `detectCapabilities` 追加 ddns capability + `providers.go` 注册 + 测试（httptest 假源）；
3. **storage**：`ddns.go` 表 CRUD + AutoMigrate + 测试；
4. **server**：`api_ddns.go`（CRUD + check + 审计）+ `ddns_scan.go`（巡检循环）+ `alert.CheckDDNS` + api.go 分发 + 测试；
5. **web**：DNS 页 Tabs 改造 + DDNS Tab + 类型 + api 方法 + build 通过。

## 测试要点

- agent：多源顺序兜底（首个成功即停）、坏响应拒绝（非 IP / 族不匹配）、全源失败省略该族、IPv4/IPv6 各自独立；
- storage：CRUD 往返；
- server：配置校验（Type 白名单、空名拒绝）；巡检三态（IP 变化更新 / 无变化跳过写 / agent 失败置 failed + 告警去重）；check 端点；记录缺失时 Create；
- 前端：tsc + vite build。
