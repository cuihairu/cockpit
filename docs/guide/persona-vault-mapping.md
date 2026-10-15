# persona 密码箱 → cockpit `vault.Provider` 接口映射

> 2026-10-13 接入实现（`internal/vault/persona.go`）的依据文档。persona 侧事实
> 全部取自仓内源码与 `docs/EXTERNAL_INTEGRATION.md`（2026-10-12 实测版），
> 逐条带 file:line；行号随 persona 上游演进可能漂移，语义以引用处为准。

## 1. persona 侧接口形态（事实源）

persona 对外部项目的正式接口是 **HTTP Connect API**（`persona connect serve`，
只听 127.0.0.1，token 鉴权）。5 个端点（`connect-server/src/lib.rs:90-96`
`build_router`）：

| 端点 | 鉴权 | 语义 |
|------|------|------|
| `GET /api/v1/connect/health` | 免认证 | 存活探针，零 vault 信息（lib.rs:1072 注：免认证但不免 Host 白名单） |
| `GET /api/v1/connect/identities` | Bearer token | scope 内身份列表 |
| `GET /api/v1/connect/items` | Bearer token | 条目元数据列表；query 过滤 `identity`/`type`/`title`（lib.rs:411-434 `ItemsQuery`） |
| `GET /api/v1/connect/items/{id}` | Bearer token | 单条全字段（**解密后**秘密）；scope 外/归档/不存在同形 404（lib.rs:472-490） |
| `POST /api/v1/connect/items/{id}/totp` | Bearer token | 现算 TOTP 码（本接入 v1 未使用） |

关键语义（均实测/源码可查）：

- **消费者无解锁/上锁端点**。vault 解锁是属主行为（CLI/桌面），消费者凭据是
  **Connect token**（`pconn_…`，发放时只展示一次、服务端只存 SHA-256，可带
  `--expires-in` 到期）。token 未知/已吊销/已过期同形 401 `unauthorized`
  （`EXTERNAL_INTEGRATION.md` 错误码表；auth 解析 `lib.rs:276-283`）。
- **只读**：无写入/删除端点（`EXTERNAL_INTEGRATION.md` 开放问题：「写入路径
  未拍板，当前接口只读」；路由表 lib.rs:92-96 佐证）。
- **锁定即 503**：vault 未解锁 → 503 `vault_locked`，不消耗限额
  （lib.rs:301-326、`locked_response`）；可退避重试等属主解锁。
- **三防线**：Host 头 host 部分（去端口）必须 loopback（lib.rs:227-250，421
  `bad_host`）；带 `Origin` 头 → 403（浏览器防越）；每 token 120 req/min →
  429（lib.rs:19 文注）。
- **包络**：成功 `{"ok":true,"data":…}`、错误 `{"ok":false,"error":{code,message}}`
  （lib.rs:144/155 `ok_json`/`err_json`）。
- **条目模型**：元数据 `{id,title,type,urls[],updated_at}`（lib.rs:449-456
  `item_meta_json`）；详情多 `username` 与 `data`（lib.rs:476-487）。`type`
  用 scope 词汇表 snake_case（lib.rs:439-447 `item_type_slug`；
  `core/src/connect/token.rs:16-30` `ConnectItemType`，serde rename_all）：
  `password / api_key / totp / note / bank_card / server_config / certificate /
  game_account / identity / software_license`（ssh-key/crypto-wallet/custom
  **永不可授权**，token.rs:47）。`data` 为 serde 外部标签枚举
  （`core/src/models/credential.rs:459-474` `CredentialData`），键为变体名
  PascalCase（`Password`/`ApiKey`/`TwoFactor`/`SecureNote`/`ServerConfig`/
  `GameToken`/`SoftwareLicense`/…），各变体字段见同文件
  `PasswordCredentialData`(:282)、`ApiKeyData`(:324)、`ServerConfigData`(:347)、
  `GameTokenData`(:390)、`SecureNoteData`(:414)、`SoftwareLicenseData`(:440)。
- **审计**：persona 侧每次 token 校验/拒绝留审计（`EXTERNAL_INTEGRATION.md`
  权限边界表）；cockpit 侧操作由本仓 `AuditFunc` 钩子落 `audit_logs`（V2）。

## 2. 六操作映射

| cockpit `vault.Provider`（`internal/vault/vault.go:31-41`） | persona 侧 | 实现要点（`internal/vault/persona.go`） |
|---|---|---|
| `Name()` | — | 恒 `"persona"` |
| `Unlock(passphrase)` | **无消费者解锁端点**——凭据是配置注入的 Connect token，解锁是属主行为 | `passphrase` 不参与（persona 无口令语义，空串合法）。握手 = `GET /health`（存活）+ `GET /items`（token 有效 + vault 已解锁）双探通过 → `Session.Unlock()`。幂等：已解锁重跑同握手并刷新 TTL |
| `Lock()` | 无对应端点 | 仅本地 `Session.Lock()`——后续操作在本地门即拒（ErrLocked），不发网络请求 |
| `Read(path)` | `GET /items/{id}` | `path` = 条目 UUID；`type` slug 直通 `Entry.Kind`；`username` → `Entry.Username`；`data.<变体>.<秘密字段>` → `Entry.Value`（映射见 §3）；`title`/`urls` → `Entry.Meta`（非敏感）。404 → `ErrNotFound` |
| `Write(path, e)` | **不存在**（只读接口） | 会话门后显式 `ErrUnsupported`——不静默吞、不降级 local |
| `List(prefix)` | `GET /items` | 元数据直映射；`prefix` 按 `path`（UUID）前缀客户端过滤；列表无 `username` → `HasUsername=false`；`HasValue`=该 type 是否有秘密字段映射（§3 七类为 true，`bank_card`/`certificate`/`identity` 为 false，与 Read 行为一致） |
| `Delete(path)` | **不存在** | 同 Write，显式 `ErrUnsupported` |

错误映射（fail-closed，V7 不降级）：

| persona 侧 | cockpit 侧 |
|---|---|
| 连接失败 / 超时 / 421 / 403 / 其他 5xx | `fmt.Errorf("…: %w", ErrLocked)`——调用方按锁定语义拒绝服务，不降级 local（V7 红线） |
| 503 `vault_locked`（属主未解锁） | 同上，包 `ErrLocked` |
| 401 `unauthorized`（token 无效/吊销/过期） | 显式 token 错误（**不包** `ErrLocked`——处置动作是换 token 不是等解锁；确定性失败不重试） |
| 429 `rate_limited` | 显式限流错误（`rate limited (120 req/min per token)`；当前不自动重试，调用方按需退避） |
| 404 `not_found` | `ErrNotFound`（errors.Is 可判） |

会话口径：复用 `vault.Session`（`internal/vault/session.go:10-52`，TTL 到期
自动上锁）——persona TTL 取 `vault.providers.persona.session_ttl`，未设回退
全局 `vault.session_ttl`（两者 `0` = 进程生命周期）。TTL 到期后操作返回
`ErrLocked`，需重新 `Unlock`（重跑双探握手）。

## 3. `Entry.Value` 秘密字段映射（`data` PascalCase 变体 → 秘密字段）

| `type`（slug） | `data` 变体 | 秘密字段（credential.rs 行号） | cockpit `Entry.Value` |
|---|---|---|---|
| `password` | `Password` | `password`（:283） | 取值 |
| `api_key` | `ApiKey` | `api_key`（:326；`api_secret`/`token` 可选字段 v1 不取） | 取值 |
| `totp` | `TwoFactor` | `secret_key`（:377 附近 `TwoFactorData`） | 取值（TOTP 共享密钥） |
| `note` | `SecureNote` | `note`（:417） | 取值 |
| `server_config` | `ServerConfig` | `password`（:353，`Option` 可空 → 空串） | 取值（可空） |
| `game_account` | `GameToken` | `secret_key`（:394） | 取值 |
| `software_license` | `SoftwareLicense` | `license_key`（:442） | 取值 |
| `bank_card` / `certificate` / `identity` | 相应变体 | 无单一秘密字段（卡号/CVV/证件号为多字段组合，不塞单值） | 空串 + `Meta.persona_data` 列字段名清单（非敏感） |

## 4. 卡点（persona 侧缺什么，如实记录）

1. **无写入/删除端点**（最大缺口）：`vault.providers.persona` 下 `Write`/
   `Delete` 永远 `ErrUnsupported`。cockpit 侧条目管理 API（PUT/DELETE
   `/api/remote/vault/entries`）选 persona provider 时这两条路径不可用——
   persona 条目只能在 persona 侧维护，cockpit 只读消费。上游「写入路径未
   拍板」落地后本 provider 补实现即可（接口不变）。
2. **无消费者解锁/上锁端点**：`Unlock` 的「口令」参数无语义（token 即凭据，
   属主解锁 vault）；`Lock` 只能锁本地会话门，锁不住 persona 本体。UI 上
   「解锁密码箱」输入的口令对 persona provider 是摆设——需要上游定消费者
   侧会话语义（如 token 换短时会话）后才有真映射。
3. **`use` 动词无 HTTP 端点**（lib.rs 测试注「暂无消费端点」）：TOTP 出码走
   `POST /items/{id}/totp` 可用，但 v1 `Entry` 无 totp 字段，未接；后续 cockpit
   要展示 TOTP 码时经 Meta/独立通道扩。
4. **列表元数据不含 `username` 有无**：`HasUsername` 恒 false（读详情才有），
   列表视图无法预判；上游元数据加字段后可补。
5. **token 过期无主动通知**：persona 侧到期即 401（与无效同形），cockpit 只能
   在失败时显式报「token 被拒」；建议发放 token 时按轮换周期设 `--expires-in`
   并在到期前人工换发（文档口径同 `EXTERNAL_INTEGRATION.md` 消费者建议）。
