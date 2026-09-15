# DNS 管理设计（M1：Cloudflare 集成）

> 2026-09-15 立项。对应 todo「DNS 管理：Cloudflare API 集成，域名资源
> 联动记录增删改」。

## 痛点

域名是 CMDB 核心资源（探测按域名跑、证书按域名管），但 DNS 记录本身
只能在 Cloudflare 控制台里改：加一条 A 记录要开浏览器登录后台，面板里
「资源 → 域名」只是个只读台账。个人云场景高频操作（新增子域名指向、
切换 IP、加 TXT 验证）值得在面板内闭环。

## 架构

server 直连 Cloudflare API v4（与 probe 探测、通知渠道同款「server 出站
调外部服务」先例），不经 Agent：

```
Web /dns ──REST──▶ server api_dns ──HTTPS──▶ api.cloudflare.com/v4
                        │
                        └─读 Domain 表做只读联动标注（M1 不回写）
```

## 决策

| # | 决策 | 内容 | 理由 / 备注 |
|---|------|------|------------|
| D1 | Provider 边界 | M1 只实现 Cloudflare；`dns.Provider` 接口（ListZones/ListRecords/SaveRecord/DeleteRecord）+ 按 config 选择实现 | todo 明确 Cloudflare；接口先行让后续 DNSPod/阿里云只加实现不动上层 |
| D2 | token 配置 | config.yaml `dns.cloudflare.api_token`；env `CLOUDFLARE_API_TOKEN` 优先覆盖 | secret 不落 yaml 的惯例（env 覆盖）；未配置时 API 返回 503 + 引导文案 |
| D3 | 直连路径 | server 直连，http.Client 超时 15s，Bearer token 头 | probe/通知同款；Agent 不参与（DNS 是外部 SaaS，不是主机本地资源） |
| D4 | API 面 | `GET /api/dns/zones`（含 CMDB 已登记标注）；`GET /api/dns/zones/{zid}/records?type=&page=`；`POST .../records`；`PUT .../records/{rid}`；`DELETE .../records/{rid}` | 与资源管理页同风格 REST；zone 与 record 都是 Cloudflare 侧事实源，不落库 |
| D5 | 联动 | M1 只读：zone 列表标注该域名是否在 Domain 表（in_cmdb 字段） | 写联动（登记 Domain、provider 自动识别）留 M2；先做最小闭环 |
| D6 | 审计 | 记录创建/更新/删除记审计（action= dns_create/dns_update/dns_delete，resourceID=zone 名+记录名）；列表/查详情不记 | 变更记审计、浏览不记的既有纪律；token 不进审计 |
| D7 | token 安全 | 任何 API 响应与错误消息不含 token 值；配置探测端点只返回「已配置/未配置」布尔 | 通知渠道 token 同款纪律 |
| D8 | 记录校验 | type 白名单（A/AAAA/CNAME/MX/TXT/NS/SRV/CAA）；name/content 非空；ttl 空=1（auto）；双端同规则（server 校验 + Web 表单限制） | 防 Cloudflare 4xx 噪音；proxied 仅 Cloudflare 支持的类型有效（服务端忽略即可） |
| D9 | 分页 | per_page=50 + page 参数透传；M1 不做自动翻全量 | 记录数通常 < 50；Cloudflare result_info 已带回 total_pages 供前端翻页 |
| D10 | Web 入口 | 新页面 `/dns`「DNS 管理」，菜单列「漂移检测」后 | 独立功能条目独立页面；zone 下拉 + 记录 Table + 新建/编辑 Modal |

## 不做（后续版本）

- DNSPod / 阿里云 DNS 等其他 provider（接口已留）；
- 与 Domain 资源的写联动（自动登记/删除 Domain 行）；
- 批量导入/导出记录、DNS 变更历史；
- 经 Agent 的分布式解析检查（各地 resolver 生效探测）。

## M1 清单

- [x] internal/dns：Provider 接口 + Cloudflare client（zones/records CRUD、
      错误解包、分页）+ httptest mock 单测
- [x] config：`dns.cloudflare.api_token` + env 覆盖 + Normalize
- [x] server：`api_dns.go` 全套 REST + 双端校验 + 审计 + serveAPI 接入
- [x] web：`/dns` 页面（zone 选择 + 记录表 + CRUD Modal + 翻页）+ 路由菜单
- [x] 测试：client 全操作（mock Cloudflare）、server 转发与校验（503/
      400/502 路径）、审计落库
- [x] 文档收尾（本清单勾选）+ todo.md 同步

✅ M1 完成（2026-09-15）：dns 包 13 测试 + server 4 测试 + config 1 测试全绿。
实现与 D6 的一处落地差异：审计 resourceID 用 `zoneID/记录名`（创建/更新）
或 `zoneID/记录ID`（删除，此时记录名已不可得），不额外调 zones 接口换
zone 名——个人场景低频操作，ID 同样可追溯。入参归一化在 client 发出前
完成（type 大写、TTL=0 → 1 auto、去首尾空白），Cloudflare 收到的永远是
干净值。

## 参考

- 内部：[probe-enhance-design.md](./probe-enhance-design.md)（server 出站
  探测先例）、notification 包（token 配置纪律）
- 外部：Cloudflare API v4（zones / dns_records）
