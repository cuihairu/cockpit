# DNS 管理设计（M1：Cloudflare 集成；M2：DNSPod / 阿里云；M3：Domain 台账写联动；M4：批量导入/导出）

> 2026-09-15 立项。对应 todo「DNS 管理：Cloudflare API 集成，域名资源
> 联动记录增删改」。M2 立项 2026-09-19：DNSPod / 阿里云 provider。
> M3 立项 2026-09-27：Domain 台账写联动（D5 明写「留 M2」的写联动，
> 实际排到 M3）。M4 立项 2026-10-03：记录批量导入/导出。

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

- ~~DNSPod / 阿里云 DNS 等其他 provider~~（M2 已交付）；
- ~~与 Domain 资源的写联动~~（M3 已交付）；
- ~~批量导入/导出记录~~（M4 已交付）；
- DNS 变更历史；
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

- [x] internal/dns：`dnspod.go`（form client + name/MX/SRV 映射 + 分页）
- [x] internal/dns：`alidns.go`（RPC V1 签名 + 同套映射）+ `sign.go` 签名
      纯函数
- [x] server：requireDNS 按 provider 构造（dnspod/alidns 凭据缺失 503 各报
      各的键）；/api/dns/status 回显 provider
- [x] config：无新增键（复用 ACME D13 结构），Normalize 不动
- [x] web：引导卡三选一 + proxied 列 provider 分流 + tsc/build
- [x] 测试：两 client httptest 全操作（zones/records CRUD/分页/MX·SRV 拆装/
      错误码）、签名纯函数表驱动、server provider 分流与 503 文案、审计不回归
- [x] 文档收尾（本清单勾选）+ todo.md 同步

✅ M2 完成（2026-09-19）：dns 包 dnspod/alidns/sign 三文件 + server 分流 +
web 引导卡，dns 包 21 测试 + server 3 测试全绿。实现与设计的两处落地差异：
① DNSPod `Domain.List` 单页拉 400 条不翻页（D14 说的是 records 分页归一，
zones 与 M1 Cloudflare 同款单页策略）；② 阿里云 `DescribeDomainRecords`
无精确 type 过滤参数，`recordType` 在 client 侧过滤，`TotalPage` 仍按全集
自算（可能翻到空页，页码点击无害）。

**真机验收（剩余）**：DNSPod 免费版 TTL 下限与 CAA 支持、阿里云 enterprise
版差异、两家 SRV 编辑实测。

## M3：Domain 台账写联动（2026-09-27 设计）

### 痛点与范围

M1/M2 与 Domain 表的关系只有只读标注（D5：`in_cmdb`）：在面板里加了
`app.example.com` 的 A 记录，「资源 → 域名」里并不会多出这一行，探测与
证书管理依旧覆盖不到；zone 也要先去 inventory 手工登记。M3 把 D5 明写
「留 M2」的写联动落地：DNS 侧的域名资产变更同步进 Domain 表，并给出
provider 自动识别与反向对账语义。

### 决策

| # | 决策 | 内容 | 理由 / 备注 |
|---|------|------|------------|
| D18 | 联动范围与字段映射 | **zone 级显式登记/移除**（`POST`/`DELETE /api/dns/zones/{zid}/cmdb`）+ **解析记录 CRUD 成功后自动跟随**（仅 A/AAAA/CNAME 三类「解析出一个域名」的记录，MX/TXT/NS/SRV/CAA 是附属数据不是域名资产，不联动）；台账行字段就近取值：`Domain`=全名（provider 侧已归一，client 层 @↔zone 名，D14）、`Provider`=当前 `dns.provider`、`Status`="active"、`Labels`={source:"dns", zone_id, zone, record_id, type} | 三家 provider 都没有 zone 的创建/删除 API（DNS 管理只覆盖记录），所以「域名变更」在面板里就是登记/移除这个动作本身；A/AAAA/CNAME 口径对齐既有约定（DDNS 只写 A/AAAA、web proxied 仅这三类）；labels 让台账行可溯源到具体 zone/记录 |
| D19 | 确定性 ID 与 provider 识别 | 台账行 ID：zone 级 `dns-{zid}`、记录级 `dns-{zid}-{rid}`（对齐 acme-{id} 先例）——记录删除/改名时无需反查即可定位行；`Provider` 字段写当前配置名，三态（cloudflare/dnspod/alidns）识别联动对象由 zone 查找（ListZones 命中 zid）统一承担，provider 差异在 client 层已被吸收 | 确定性 ID 让 unlink 幂等（行不存在不报错）且记录更新时原地 upsert（改 name/type 不产生新行）；zid 在 dnspod/alidns 即域名本身（D14），ID 可读性反而更好 |
| D20 | 尽力而为 + 声明态保护 + 反向对账 | **provider 是事实源**：DNS 操作必须先成功，联动失败只记日志，不回滚、不改 API 应答（含审计：跟随动作不单独记，同一动作已有 dns_create/update/delete 留痕；登记/移除两个显式动作用新审计动作 `dns_cmdb_register`/`dns_cmdb_unregister`，resource=domain）；**声明态保护**：登记时同名行已存在（inventory/手工/历史 DNS 来源）一律 409，绝不静默改写别人的行；记录跟随遇到同名但 ID 不同的行 → 跳过联动只留日志；唯一索引是最终防线；移除登记只删 `labels.source=="dns"` 的行，且**一次清掉该 zone 的全部 DNS 来源行（zone 行 + 记录跟随行）**——这也是孤儿行的清理出口；**反向对账**：`GET /api/dns/zones` 响应新增 `orphans`（DNS 来源、所属 zone 已不在 provider 列表的行；归属口径：记录级行看 `labels.zone`，zone 级行看自身域名），只列出**永不自动删除**——inventory 声明或手工登记可能仍有效，由用户在面板逐条清理 | 面板外删 zone / 切换 provider 后的残留行不能悄悄消失（可能仍被探测/证书引用），但也不能假装它们有效——列出来让用户处置；zone 已从 provider 消失时移除端点不查 provider，照常可清；孤儿摘要带 `zone_id` 供前端定向清理 |

### API 面与 Web

- `POST /api/dns/zones/{zid}/cmdb` → 200 `{registered,id}` / 404（zone 不在
  provider）/ 409（同名行已存在）/ 502·503（凭据与上游同既有语义）；
- `DELETE /api/dns/zones/{zid}/cmdb` → 200 `{unregistered,records_removed}`
  / 404（该 zone 无任何 DNS 来源行）/ 409（zone 行存在但来源不是 dns）；
- `GET /api/dns/zones` 响应扩为 `{data:[...], orphans:[{id,domain,zone_id}]}`；
- 记录 CRUD 的联动对 web 透明（成功后台账行已跟随，无需新交互）；
- web `/dns` 记录管理 Tab：zone 选中后按 `in_cmdb` 显示「登记台账」或
  「移除登记」（Popconfirm 说明会连带清记录跟随行），两者受 `dns:write`
  管辖，失败文案（含 409）直接透出；`orphans` 非空时表格上方 Alert 逐条
  列出并给定向「清理」按钮（无 `zone_id` 的历史行提示去「资源 → 域名」
  手工处理）。

### 不做（M3 确认）

- 台账行 content/ttl 等 DNS 细节的镜像（Domain 表只表达「这个域名存在、
  归谁管」，值以 provider 为准）；
- 自动删除孤儿行（见 D20）；inventory 侧声明与 DNS 来源行的合并策略
  （保持「同名 409/跳过」的先到先得，不做字段级 merge）；
- 记录级联（zone 下全量记录的一次性登记）。

### M3 清单

- [x] server：`api_dns_cmdb.go`（登记/移除 + 记录跟随 + 孤儿对账）+
      api_dns 路由挂载与三处联动钩子
- [x] audit：`ResourceDomain` 资源类型 + `dns_cmdb_register`/
      `dns_cmdb_unregister` 动作
- [x] storage：无新增方法（复用 UpsertDomain/GetDomainByName/GetDomain/
      DeleteDomain/ListDomains），补唯一索引冲突与 GetDomainByName 大小写
      精确匹配的单测
- [x] dns 包：零改动（联动全部在 server 层，client 归一已够用）
- [x] web：api.ts 登记/移除方法 + getDNSZones 返回 `{zones,orphans}` +
      DNS 页按钮/Alert + DDNSPanel 适配
- [x] 测试：server 层登记/移除/三 provider 识别/跟随/失败不写库/同名不
      覆盖/孤儿对账与清理 + 错误分支注入（closed db、trigger 阻断
      INSERT·DELETE）；web 层 DNS 页 6 个联动用例
- [x] 文档收尾（本清单勾选）+ todo.md 同步

✅ M3 完成（2026-09-27）：api_dns_cmdb.go 全函数 100% 语句覆盖（经
TestDNS 口径 profile 核对）。与设计的两处实现差异：① 登记前用
`ListZones` 验 zone 存在（provider 是唯一事实源，面板不凭空登记），故
未知 zid 是 404 而不是直接落库；② 台账跟随读路径用
`GetDomainByName`（唯一索引精确匹配，大小写敏感——`Example.com` 与
`example.com` 是两行），孤儿对账用 `ListDomains` 单趟扫描（与 zones
的 in_cmdb 标注共用 provider 列表，不额外查询）。

**真机验收（剩余）**：真实 Cloudflare/DNSPod/阿里云账号下登记→台账出现
→改记录→台账跟随→删记录→台账消失→移除登记→整 zone 清干净的全链路；
以及面板外删 zone 后 orphans 提示与清理。

## M4：批量导入/导出（2026-10-03 设计）

### 痛点与范围

zone 迁移（换 provider / 换账号）、灾备留存、批量初始化子域名都是
「逐条手点」不可承受的操作。M4 补上便携的记录级导出与幂等导入：
导出产物是纯 JSON（RecordInput 形态），可人工审阅、可跨 provider
迁移；导入按「匹配键」做 create/update/skip 三分类，重导幂等。

### 决策

| # | 决策 | 内容 | 理由 / 备注 |
|---|------|------|------------|
| D21 | API 形态与保留字 | `GET /api/dns/zones/{zid}/records/export`、`POST /api/dns/zones/{zid}/records/import`；`export`/`import` 是 records 子路径的保留字，方法不符 405 | 三家 provider 的记录 ID 均为十六进制/数字串，与保留字无撞名面；rbac.go 零改动——`/api/dns` 前缀推导天然覆盖（export GET = dns:read、import POST = dns:write），测试断言钉死 |
| D22 | 导出 | 翻全量聚合（page 1..total_pages，与单页列表同一 client 归一），响应 `{zone_id, provider, count, records}`；records 为**便携 RecordInput 形态**（type/name/content/ttl/proxied，无 provider id/locked） | 导出文件可直接作导入输入（zone 归属由 URL 决定，文件里的 zone_id/provider/count 仅溯源信息）；GET 不经审计中间件（M1 起浏览不记），取内容审计先例（recordings cast）手动记 `dns_export`，details 只含 count |
| D23 | 导入匹配键与写判定 | 匹配键 = name 归一（去空白、去尾点、小写）+ type 大写 + content 去空白精确；命中且无差异 → skipped，命中有差异 → UpdateRecord（updated），未命中 → CreateRecord（created）。差异判定：**显式 TTL（>=60）须精确相等，auto（0/1）表示「provider 决定」不触发更新**；proxied 仅 cloudflare 比较（其余 provider 该字段恒 false，D15）。同请求内重复条目：create/update 成功后写回匹配索引，重复即 skip | auto 不触发更新让 CF 导出（ttl:1）迁到 DNSPod（读回默认 600）不会反复「更新」；proxied 的 provider 感知同 D15 口径 |
| D24 | 容错与上限 | 上限 500 条（超出 400）；**先整体校验**（逐条 ValidateInput，任一非法 400，错误带下标、最多列 5 条）再触上游；单条上游失败**不中断**，记入 `failed:[{index,name,error}]`；响应 `{total,created,updated,skipped,failed}`；审计 `dns_import`（resourceID={zid}/batch，details 只含计数不含记录内容） | 整体校验防半途而废（501 条里第 490 条非法不该先写 489 条）；单条失败继续对齐「导入是批量操作，一条坏不该废整批」 |
| D25 | 联动与权限复用 | created/updated 路径走 `linkDNSRecordDomain`（M3 D18-D20 语义原样：仅 A/AAAA/CNAME、失败只记日志不回滚）；RBAC/错误处理/503 引导全部复用既有基础设施 | 批量导入在台账侧等价于 N 次单条 CRUD，不引入第二套联动逻辑 |

### 不做（M4 确认）

- **删除/replace 模式**：导入只增改不删——批量删除的破坏面大（误传旧文件
  即清空 zone），需要删的走单条删除或 provider 控制台；「面板外删了记录
  想同步」用导出对比人工处置；
- 异步任务化：500 条上限内同步完成（上游串行写，最坏几十秒），不起
  任务轮询；
- DNS 变更历史（版本对比/回滚）——需要落库存快照，另行立项。

### M4 清单

- [x] server：api_dns.go 路由保留字分发 + export/import 两 handler +
      `fetchAllDNSRecords` 翻页聚合（与 import 现状比对共用）
- [x] audit：`dns_export`/`dns_import` 动作（details 只含计数）
- [x] web：api.ts 两方法 + types；DNS 页工具栏「导出」（Blob 下载
      `<zone>-records.json`）与「导入」Modal（本地读取 .json → 解析
      预览 → 确认 → 四分类结果摘要 + failed 明细）
- [x] AuditLogs：ACTION_MAP/RESOURCE_MAP 补 7 个 dns_* 动作标签与
      dns_record 资源标签（操作类型过滤下拉同步获得）
- [x] 测试：server——export 多页聚合/上游 502/未配置 503/方法 405、
      import 全四分类/校验 400/超限 400/坏 JSON/失败不中断/非 CF
      proxied 分支/审计与台账联动断言；RBAC requiredPerms 钉死断言；
      web——DNS 页导出下载/导入全流/坏文件拒绝/AuditLogs 标签

## 参考

- 内部：[probe-enhance-design.md](./probe-enhance-design.md)（server 出站
  探测先例）、notification 包（token 配置纪律）
- 外部：Cloudflare API v4（zones / dns_records）、DNSPod API（dnsapi.cn
  Domain.List / Record.*）、阿里云云解析 RPC API（alidns.aliyuncs.com
  2015-01-09，V1 签名）
