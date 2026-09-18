# ACME 证书自动签发设计

## 背景

Cockpit 已具备三个可复用基础，证书「签发 + 存储 + 续期」是它们合流的自然落点：

- **DNS 写路径**（`docs/guide/dns-design.md`）：server 直连 Cloudflare API，token 管理（config.yaml `dns.cloudflare.api_token` / env `CLOUDFLARE_API_TOKEN`）现成——DNS-01 challenge 的全部前提；
- **巡检循环模式**（drift → smart → ddns 三轮验证）：ticker + Setting 动态间隔 + 真去重告警；
- **证书观测**（`internal/cert` + `Certificate` 表）：probe 定期 TLS 探测回写到期/签发者，但**没有任何签发能力**，`Certificate` 是纯观测表。

反向代理功能（proxy-design.md）的「证书签发联动」被明确标注等待本能力；DDNS 场景（家庭宽带域名）签好解析后，下一步就是 HTTPS 证书。

## 决策

- **D1 库选型 lego v4**（`github.com/go-acme/lego/v4`，MIT，v4.35.2 活跃维护）：纯库嵌入（`lego.NewClient` + `Certificate.Obtain`），DNS-01 全流程封装（TXT 下发、传播等待、授权轮询、取证书链），100+ DNS provider 开箱即用，账户注册/EAB 支持。否决备选：`x/crypto/acme/autocert`——绑定 `http.Server` 的 TLS 配置场景，DNS-01 需自行编排（等于半个自研）；`certmagic`——定位是托管 web server 的 TLS 全生命周期（缓存/OCSP/SNI 路由），对「签发+存储+展示」的控制台场景过重；`acmez`——太底层，lego 正是建在这类原语上的封装。

- **D2 只做 DNS-01 challenge**：HTTP-01 / TLS-ALPN-01 要求 80/443 从公网可达——家庭宽带入站端口常被封（本项目的核心场景），且只有 DNS-01 支持泛域名（`*.example.com`）。DNS-01 只需 Zone.DNS Edit 权限，与 DNS 管理/DDNS **同一份 token** 即可，无需额外授权。

- **D3 架构：server 侧直连**：与 DNS 管理/probe/DDNS 同款出站路径，agent 不参与。签发与续期都是 server 动作——证书要存 server 的 SQLite（见 D5），且 Cloudflare token 只在 server（dns-design D2/D7）。

- **D4 DNS provider 与 CA 目录**：M1 用 lego 自带 `providers/dns/cloudflare`（`NewDNSProviderConfig` 直接吃 APIToken，复用 `s.dns` 同源 token；未配置 token 时创建/签发统一 503 + 引导文案，同 DNS 页模式）。CA 目录两档：`production`（Let's Encrypt 生产）与 `staging`（假证书，测试不触限频），per-证书可选，默认 staging（防误触生产限频，用户确认后切 production）。M2 扩展 DNSPod/阿里云时在 server 侧加 provider 工厂，lego 接口是现成扩展点。

- **D5 数据模型（SQLite 两表）**：
  - `AcmeAccount`（单行，ID=1）：`Email / PrivateKeyPEM（ECDSA P-256）/ RegistrationURI / CADirectory / CreatedAt`。首次签发时惰性注册（lego `Registration.Register`），账户 key 持久化——跨重启复用同一 ACME 账户（重复注册会触发账户数限频）。
  - `AcmeCert`：`Domains（JSON 数组，首个为 primary）/ PrimaryDomain（索引）/ CADirectory / Status（pending|issued|failed）/ CertificatePEM / IssuerPEM / PrivateKeyPEM / ExpiresAt / RenewBeforeDays（默认 30）/ AutoRenew / LastRenewAt / LastStatus（ok|failed|never）/ LastError / CheckedAt`（观测字段复刻 DDNS 风格）。
  - 证书与私钥存 SQLite：与面板数据同生命周期，server-backup（VACUUM INTO）自动覆盖；文件落盘留给 M2 部署联动按需导出。
- **D6 签发流程**：`POST /acme/certs/{id}/issue` 同步执行——校验 token/账户 → lego `Obtain{Domains, Bundle:false}`（证书链与 issuer 分开存）→ 回写 `Status=issued / ExpiresAt（解析叶子证书 NotAfter）/ LastStatus / LastRenewAt` → **联动观测表**：upsert `Certificate`（`DomainName=primary`，`Issuer`，`ExpiresAt`，`Status=valid`，`Labels["source"]="acme"`），probe 的 TLS 探测对该域名继续独立观测，两层互补。同步等待 DNS 传播通常几秒~几十秒，M1 可接受（前端按钮 loading）；异步任务化（stack 模式）留给签发+部署联动的 M2。

- **D7 续期巡检**：`acmeScanLoop` 复刻 drift/smart/ddns 模式；Setting `acme.scan_interval_seconds`，默认 3600，min 300，max 86400，`0 = 关闭`。每轮对每条 `AutoRenew && Status==issued` 证书判断 `ExpiresAt - now < RenewBeforeDays * 24h` → 重新 Obtain（全新证书，覆盖旧 PEM）。失败置 `Status=failed + LastError` + `alert.CheckACME` 真去重 warning（title 按主域名，同 CheckDDNS 构）。

- **D8 限频保护**：Let's Encrypt 生产限频（每注册域名每周 50 张、失败验证 5 次/小时/账户/主机名、重复证书 5 张/周）。M1 三道闸：a) 默认 staging（D4）；b) 签发失败节流——`LastRenewAt` 距今 < 1h 的证书巡检跳过重试（手动 issue 不节流，用户明确意图优先）；c) 传播 pre-check 由 lego 顺序 DNS 策略承担（查 authoritative NS，避免盲等固定时长）。

- **D9 安全与审计边界**：账户 key、证书私钥**永不进** API 响应/日志/审计；列表与详情只回元数据；PEM 内容仅经下载端点出去：`GET /acme/certs/{id}/download?part=cert|issuer|key`——`key` 强制记审计（`acme_download_key`），`cert/issuer` 不记。审计：`acme_create / acme_update / acme_delete / acme_issue / acme_download_key`；巡检自动续期不记审计（同 ddns D9 则，`LastRenewAt` 已记录轨迹）。删除配置仅删本地记录，**不向 CA 吊销**（吊销需要账户 key 且语义上证书可能已部署他处）。

- **D10 REST**（全部 `/api/acme` 前缀）：
  - `GET /api/acme/certs` 列表；`POST /api/acme/certs` 创建（`domains[]` 逐个过 `dns.ValidateInput` 同规则、`ca` ∈ {staging, production}、`autoRenew`、`renewBeforeDays` 7~90）；`PUT /api/acme/certs/{id}` 更新；`DELETE /api/acme/certs/{id}` 删除
  - `POST /api/acme/certs/{id}/issue` 立即签发/重签（同步，返回结果摘要）
  - `GET /api/acme/certs/{id}/download?part=cert|issuer|key`（`text/plain` 附件）
  - `GET/PUT /api/acme/account`（email 设置；PUT 时若账户已注册且 email 变更则更新注册）
  - `GET/PUT /api/acme/config` 全局巡检间隔（与 `/api/drift/config` 等同构；0 = 关闭）

- **D11 前端**：新页面 `/acme`「证书签发」（导航挂「资源」分组，紧跟「证书」观测项），页面结构照 DDNS Tab 模式：设置行（自动续期巡检开关 + 间隔分钟 + 新建按钮）+ 配置表（主域名+泛域名标记/CA 徽标（staging 橙色）/状态徽标（失败 Tooltip 错误）/到期时间+剩余天数/最后签发时间/操作（立即签发、下载下拉、编辑、删除））+ 新建/编辑 Modal（domains Tag 列表编辑、CA Select 默认 staging 带「测试用假证书」说明、自动续期 Switch、提前续期天数）+ email 账户设置（首次创建时内嵌在引导里）。未配置 token 时引导卡片（同 DNS 页文案，指出 ACME 与 DNS 管理共用 token）。

- **D12 校验面**：`domains[]` 非空、每个元素过与 DNS 记录名同规则的域名校验（允许 `*.` 前缀的泛域名，剥离后校验）；`renewBeforeDays` ∈ [7, 90]；`ca` 白名单；primary domain = domains[0]。token 未配置时 create/issue 返回 503（同 DNS 页 503 文案约定）。

## 测试策略

lego 完整 ACME 流程需真实 CA（官方测试用 Pebble 外部进程），M1 单测不跑真流程——把「签发」抽成 `AcmeIssuer` 接口（`Issue(cert *storage.AcmeCert) (issued *IssuedResult{CertificatePEM, IssuerPEM, PrivateKeyPEM, ExpiresAt}, err error)`），server 依赖接口、生产实现包 lego、测试注入 fake（与 fakeDNSProvider 同构）：

- storage：两表 CRUD 往返；
- API：校验（domains 空/坏域名/CA 白名单/renewBeforeDays 越界拒绝）、创建/更新/删除审计、download key 记审计且响应体为 PEM、issue 调用 issuer 并回写状态、token 未配置 503；
- 巡检：临期续期 / 未临期跳过 / failed 后 1h 节流 / 失败告警真去重；
- lego 集成（真签发）留给真实环境验收（staging 目录跑通一次）。

## 实现切分（M1，本任务）

1. **docs**：本文档；
2. **storage**：`acme.go`（AcmeAccount + AcmeCert CRUD）+ AutoMigrate + 测试；
3. **server**：`acme_issuer.go`（lego 封装：账户惰性注册、cloudflare provider 构造、Obtain）+ `api_acme.go`（REST + 审计）+ `acme_scan.go`（巡检）+ `alert.CheckACME` + api.go 分发 + 测试（fake issuer）；
4. **web**：/acme 页 + 路由/导航 + 类型 + api 方法 + build 通过。

## 验收清单（真实环境）

- [ ] staging 目录签发单域名证书成功，`/resources/certificates` 出现 `source=acme` 记录
- [ ] 下载 cert/key 的 PEM 内容与签发一致，key 下载产生审计
- [ ] 切 production 签发一张真证书，浏览器信任链验证通过
- [ ] 把 `renewBeforeDays` 调到 90，等巡检轮触发自动续期，ExpiresAt 更新
- [ ] 撤走 token 触发一次失败，告警只出现一条（真去重）
