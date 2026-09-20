# 方案设计：服务域名绑定（Domain Binding）

> 2026-09-19。服务域名配置首期设计。需求原文：「可以配置指向服务的域名，这样
> agent 中可以方便使用域名来配置」。本文只做设计决策与接口定义。

## 现状与痛点

一个「域名对外提供服务」的事实分散在四个互不知情的模块里：

| 模块 | 现状 | 配的东西 |
|------|------|----------|
| DNS 管理 | `dns` provider 建 A/CNAME 记录 | 域名 → 服务器 IP |
| 反向代理 | traefik `ProxySite{ServerNames, Upstream}` | 域名 → host:port |
| 证书 | `cert` 监控按域名列表轮询到期时间 | 域名清单（手工维护，与上两处重复） |
| 探针 | `probe` 目标按 URL 配置 | 域名 URL（同样重复） |

| 痛点 | 后果 |
|------|------|
| 开一个新服务要跑 3-4 处配置 | 漏一处就是「解析没建/证书过期/探针没盯」三类经典事故 |
| 域名与服务的关系只存在于人脑 | 半年后不知道 blog.example.com 指向哪台机器哪个端口 |
| agent 侧配置只能手抄域名 | 换域名/加域名要逐台 agent 手改 |

## 架构总览

**DomainBinding 一等资源**：登记「域名 → agent 上的目标服务」，一次申请，
三个下游（DNS / 反代 / 证书监控）自动联动；agent 侧按需引用域名清单。

```
┌─ server ────────────────────────────────────────┐
│ DomainBinding{domain, agent, target, auto*}     │
│   apply() ──┬─▶ dns provider：A/CNAME 建改记录   │
│             ├─▶ agent RPC：traefik 站点下发      │
│             └─▶ cert monitor：自动纳入监控       │
│ GET /api/agents/{id}/domains  ← agent 按需拉取  │
└─────────────────────────────────────────────────┘
```

## 关键决策

| # | 决策 | 选择 | 理由 |
|---|------|------|------|
| D1 | 资源模型 | `DomainBinding{domain(唯一), agentID, target(host:port \| docker://服务名), enabled, autoDNS, autoProxy, autoCert}` | 三个 auto 开关解耦联动：只想登记关系不开自动化也合法 |
| D2 | target 语义 | 统一为「agent 可达地址」：host:port（host 可为 127.0.0.1/容器名/内网 IP）；`docker://name` 解析为该 agent 上 docker 网络内的服务名 | 反代上游就在 agent 本机网络，域名解析记录指向的是 agent 主机 IP，两者分层不混淆 |
| D3 | DNS 记录 | `autoDNS` 开时经 dns provider 写 A 记录（IP = agent 注册上报的主地址）；CNAME/多级域名首期不做 | agent 主地址已有注册链路，不新增配置；CNAME 场景等真实需求 |
| D4 | 反代下发 | `autoProxy` 开时复用既有 traefik `ApplySite`（ServerNames=[domain], Upstream=target），server 纯转发、agent 本地渲染+自检 | 完全复用 P1 反代通道，零新协议 |
| D5 | 证书联动 | `autoCert` 开时域名自动进入 cert 监控清单；反向：Binding 删除时提示（不自动删）监控项 | 监控宁可多盯不可漏盯；自动删除监控是危险默认 |
| D6 | 一致性检查 | Binding 列表页展示「漂移」：DNS 实际记录 ≠ 期望、站点未下发、监控缺失，逐项标记 | 登记了不等于生效了；漂移可见是自助排障第一步 |
| D7 | agent 引用 | `GET /api/agents/{id}/domains`（含 `/snippet` 配置片段生成）下发该 agent 的域名清单；probe 目标支持 `binding://` 引用形态，解析点在 server 探测链路 | 换域名只改 Binding，引用键（agent+target）稳定则探测自动跟随 |
| D8 | 删除语义 | 删 Binding 只删登记，不动已下发的 DNS/站点/监控项（提示手工清理） | 删除自动化连锁是危险操作；先可见后清理 |
| D9 | 权限 | 增删改需 `dns:write` + `proxy:write`（联动了谁就要谁的权限）；只读沿用 `dns:read` | 与 RBAC 设计（rbac-design.md）权限点清单衔接 |
| D10 | 存储 | 复用 settings/inventory 现有 SQLite，GORM 单表 `domain_bindings` | 无新依赖；量级（个位数到几十条）不值得独立存储 |

## API（server 侧，agent 零协议变更）

```
GET    /api/domains?agent=xxx     列表
GET    /api/domains/drift?agent=xxx  漂移检查（D6，实时逐条检查三路联动）
POST   /api/domains               登记/更新
DELETE /api/domains/{domain}      删登记（D8）
POST   /api/domains/{domain}/apply   执行联动（DNS/反代/证书按 auto* 开关）
GET    /api/agents/{id}/domains      该 agent 的域名清单（D7）
GET    /api/agents/{id}/domains/snippet  配置片段（D7，text/plain）
```

### D7 agent 引用细化

早期草案写「只读 RPC `domains.list`」——落地改为 **HTTP 端点**：现有
RPC 协议是 server→agent 单向（agent 响应），而域名清单的事实源在
server 库，「下发」的天然形态是 server 端点；agent 本身不消费清单
（agent 零协议变更保持），消费方是人 / 脚本 / 配置生成。

- **清单**：`GET /api/agents/{id}/domains`，数据源
  `ListDomainBindingsByAgent`，agent 必须已注册（404）
- **片段**：`GET /api/agents/{id}/domains/snippet` 返回 text/plain，
  含 env 行（`COCKPIT_AGENT_DOMAINS="…"`）与 nginx `server_name` 行
  两段，只取 enabled 绑定——curl 即得、agent 上的任意服务可 source
- **probe 引用**：`Service.URL` 支持 `binding://{agentID}/{target}`
  形态（如 `binding://a1/127.0.0.1:8080`、`binding://a1/docker://web`）。
  探测时 probe Runner 查 binding 表按 agent+target 解析出当前域名，
  以 `https://{domain}` 探测——引用键稳定，binding 换域名（删旧建新
  同 agent+target）后探测自动跟随，不再手抄。仅 http/https 类型可用；
  引用无匹配 / 绑定停用 / 类型不符 → 探测结果记 down 并报错（可见，
  不静默）

### D6 漂移检查细化

漂移 = **auto\* 开关承诺的状态**与实际状态的差距；没承诺（开关关）就没漂移。
实时按需检查（列表不内嵌——每条 DNS 检查要打 provider API、proxy 检查要打
agent RPC，内嵌会把列表拖到秒级），排障时主动触发。

- 检查范围：`enabled=false` 整条跳过；`autoDNS/autoProxy/autoCert` 关的
  路不检查（`checked=false`），开的路逐项标记
- DNS：期望 = agent 注册主地址（与 apply 同源）；实际 = provider 内该域名
  A 记录。记录缺失 `missing`、内容不符 `mismatch`；provider 未配置 /
  zone 不匹配 / agent 无地址 → `error`（无法判定，非漂移）
- Proxy：`site.get` 查 `bindingSiteName(domain)`；站点不存在 `missing`；
  存在但 upstream（docker:// 前缀归一后）或 serverNames 与登记不符
  `mismatch`；agent 不可达 / RPC 失败 → `error`
- Cert：inventory Domain 缺失 `missing`、被其他 agent 占 `foreign`；
  否则 ok
- 三态语义：`ok`（无漂移）/ `missing|mismatch|foreign`（确认漂移，附
  expected/actual）/ `error`（检查失败，不可判定）——前端红黄区分

## 分期

| 期 | 内容 | 验收 |
|----|------|------|
| P0 | Binding CRUD + apply 联动 DNS/反代/证书 + 漂移检查 | 登记一个域名到 apply 完成全自动，dashboard 可见 |
| P1 | agent `domains.list` RPC + probe 目标引用域名 + 一键生成安装/配置片段 | agent 侧配置不再手抄域名 |
| P2（按需） | CNAME/泛域名、批量 apply、cert 监控自动移除 | 按实际需求排期 |

## 实现状态

- 2026-09-20：P0 主体落地——storage 单表 + server 四端点（列表/登记/删除/apply
  三联动）+ apply 快照回写（`LastApplyStatus/LastError/AppliedAt`）。实现见
  `internal/storage/domain_binding.go`、`internal/server/api_domain_bindings.go`。
- 2026-09-20：D6 漂移检查落地——`GET /api/domains/drift` 实时逐条检查三路
  联动（语义见上节细化）。换绑 cert 监控项走 `UpdateDomainAgentID` 显式
  更新——GORM `Assign(struct)` 对已存在记录不落库，`UpsertDomain` 承担不了
  换绑语义（存量坑另记）。
- 2026-09-20：D7 agent 引用落地——`GET /api/agents/{id}/domains`（+
  `/snippet` 配置片段）与 probe `binding://` 引用解析（细化见上节）。
  未含：dashboard 页面（web UI 另行）、D9 权限点（沿用 admin 中间件现状）。

## 不做的事

- 不做 DNS 托管全量管理（zones/NS/SOA）——dns provider 已管记录，Binding 只管「服务视角」的那一条
- 不做内置 DNS server——域名解析始终落在用户已有 DNS 商
- 不做证书签发的自动触发链（ACME 签发仍由 acme 模块独立决策）——Binding 只保证「被监控」
