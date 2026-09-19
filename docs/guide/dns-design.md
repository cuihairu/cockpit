# DNS 管理设计（M1：Cloudflare 集成；M2：DNSPod / 阿里云）

> 2026-09-15 立项。对应 todo「DNS 管理：Cloudflare API 集成，域名资源
> 联动记录增删改」。M2 立项 2026-09-19：DNSPod / 阿里云 provider。

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

## M2：DNSPod / 阿里云 provider（2026-09-19 设计）

### 痛点与范围

国内域名的托管主流在 DNSPod（腾讯云）与阿里云解析，M1 只有 Cloudflare
时这两类域名仍要开控制台改记录。M2 把 M1「不做」清单第一项（其他
provider，接口已留）落地：新增两个 client 实现，上层 REST/校验/审计/
web 页面全部复用。写联动（Domain 表回写）、批量导入、分布式解析检查
继续不做。

### 决策

| # | 决策 | 内容 | 理由 / 备注 |
|---|------|------|------------|
| D11 | provider 统一 | 复用 ACME D13 引入的全局 `dns.provider`（空=cloudflare 向后兼容 / dnspod / alidns）与同一套凭据（`dns.dnspod.login_token` / `dns.alidns.access_key`+`secret_key`，env 优先） | M2 起 DNS 管理页与 ACME 签发共用同一 provider 配置，**消除 D13 的语义分叉**——一个 provider 键管两处，避免「签发用 dnspod、记录管理却指向 cloudflare」的错位 |
| D12 | DNSPod client | 手写 form 客户端直连 `https://dnsapi.cn/{Action}`（POST urlencoded：公共参数 login_token/format=json + 各 action 参数）；action：Domain.List、Record.List（offset/length 分页）、Record.Create/Modify/Remove；成功判定 `status.code == "1"` | 与 cloudflare.go 同构（net/http 手写、httptest 可测）；依赖树里的 nrdcg/dnspod-go 过老且抽象不对口，不引；错误消息只含 status.code/message，token 不出现 |
| D13 | 阿里云 client | 手写 RPC V1 签名客户端直连 `https://alidns.aliyuncs.com/`（GET query：公共参数 AccessKeyId/SignatureMethod=HMAC-SHA1/Timestamp(UTC)/SignatureNonce + Action=DescribeDomains / DescribeDomainRecords / AddDomainRecord / UpdateDomainRecord / DeleteDomainRecord；签名 = 参数排序 percentEncode 后 HMAC-SHA1(secret+"&")，特殊字符编码 !'()* 与空格按阿里规则） | alidns-20150109 SDK 已在依赖树但系 lego 带入（indirect），其 tea 栈难注入 httptest endpoint；手写签名 ~40 行换全链路可测性与三 client 同构，值得。如后续复杂化再换 SDK |
| D14 | 概念映射 | Zone.ID：两家都无独立 zone id，**用域名本身**（DNSPod Domain.name / 阿里云 DomainName）；Zone.Status 归一 active；记录 name：两家都是**子域语义**（www / @ 根域），client 层与全名互转（`@` ↔ zone 名）；分页：DNSPod offset/length 与阿里云 PageNumber/PageSize=50 都归一为 RecordsPage{Page,TotalPage}（TotalCount 自算 ceil） | dto 与上层不感知 provider 差异；zoneID 即域名的副作用：REST 路径里的 zid 直接是域名，语义反而更直白 |
| D15 | 记录字段差异吸收 | proxied 仅 Cloudflare：非 CF provider 输入忽略、输出恒 false；MX：content 首词优先级拆出（DNSPod `mx=` 参数 / 阿里云 Priority），读时拼回 `10 mail.x.com` 形态；SRV：content 四段（priority weight port target）拆装，DNSPod mx=priority/value=weight port target，阿里云四段分参数；TTL：DNSPod 传 1（auto）时省略 ttl 参数由 API 取默认（免费版下限 600 由 API 兜底报错） | MX/SRV 拆装是两家 API 的硬差异，client 内聚 + 表驱动测试；其余 type 白名单/校验规则与 M1 D8 完全一致 |
| D16 | API 面/审计不变 | REST 形态、双端校验、审计动作与 resourceID 规则、503 引导语义全部复用；仅 requireDNS 按 provider 构造 client；`GET /api/dns/status` 响应扩为 `{provider, configured}`（未配置时 provider 也回显） | 前端引导卡按 provider 列配置方法（ACME 页 M2 D13 同款字段语义，两页文案对齐） |
| D17 | web provider 感知 | /dns 页引导卡三选一（cloudflare→api_token、dnspod→"ID,Token" 合并格式、alidns→access_key/secret_key，均列 config 键与 env）；记录表与编辑 Modal 的 proxied 列仅 provider=cloudflare 显示；zone 下拉不变（名称即 ID） | 纯展示分流，无交互差异；ACME 引导卡已有三 provider 文案可参考 |

### 不做（M2 复确认）

- 腾讯云/阿里云**控制台级**功能（DNS 解析套餐、线路负载、批量玩法）——只做
  记录 CRUD 的面板闭环；
- 智能线路（record_line）编辑：DNSPod Create/Modify 固定 `record_line=默认`，
  现有非默认线路记录被编辑时会归到默认线路——M2 在文档明示，编辑框不展示
  线路概念；
- 写联动（Domain 表回写）、批量导入导出、经 Agent 分布式解析检查——维持 M1
  「不做」。

### M2 清单

- [ ] internal/dns：`dnspod.go`（form client + name/MX/SRV 映射 + 分页）
- [ ] internal/dns：`alidns.go`（RPC V1 签名 + 同套映射）+ `sign.go` 签名
      纯函数
- [ ] server：requireDNS 按 provider 构造（dnspod/alidns 凭据缺失 503 各报
      各的键）；/api/dns/status 回显 provider
- [ ] config：无新增键（复用 ACME D13 结构），Normalize 不动
- [ ] web：引导卡三选一 + proxied 列 provider 分流 + tsc/build
- [ ] 测试：两 client httptest 全操作（zones/records CRUD/分页/MX·SRV 拆装/
      错误码）、签名纯函数表驱动、server provider 分流与 503 文案、审计不回归
- [ ] 文档收尾（本清单勾选）+ todo.md 同步

**真机验收（剩余）**：DNSPod 免费版 TTL 下限与 CAA 支持、阿里云 enterprise
版差异、两家 SRV 编辑实测。

## 参考

- 内部：[probe-enhance-design.md](./probe-enhance-design.md)（server 出站
  探测先例）、notification 包（token 配置纪律）
- 外部：Cloudflare API v4（zones / dns_records）、DNSPod API（dnsapi.cn
  Domain.List / Record.*）、阿里云云解析 RPC API（alidns.aliyuncs.com
  2015-01-09，V1 签名）
