# 密码箱抽象与可替换提供商（VaultProvider）设计

> 语言：**中文** ｜ [English](./vault-design.en.md)

> 状态：**M1 设计定稿（2026-10-12），实现分波推进**。来源：todo 2026-10-11 用户令——persona 新升级已实现密码箱、可作网站密码箱使用；各项目密码箱应抽象接口、支持替换提供商。

## 1. 盘点（实现前先列全，todo 要求）

凭据进入系统的全部通道（2026-10-12 全仓盘点）：

| 通道 | 位置 | 现状 |
|---|---|---|
| SSH/RDP/VNC 连接凭据 | `remote_credentials` 表（password/private_key AES-GCM） | 已有密码箱雏形：`PUT /api/remote/vault/credentials` + 登录口令/TOTP 重验换 10min vault token（`api_vault.go`） |
| NAS targets | agent env `COCKPIT_NAS_TARGETS`（JSON 数组含 username+password） | 明文 env，凭据纪律靠约定（不进快照/日志/错误） |
| TOTP 种子 | `users.totp_secret`（AES-GCM） | auth 域，与连接凭据共用同一把 env 派生密钥 |
| 用户口令 / agent secret | bcrypt 哈希 | 非 vault 域（单向哈希，无解密需求） |
| DNS/overlay API token | yaml 字段 + env 覆盖（env-wins 约定「secret 不落文件」） | 服务端构造时读取 |
| notification（ntfy token/webhook secret/telegram bot_token） | yaml | 服务端发送时读取 |
| SMTP relay | env `SMTP_PASS` / yaml | 密码重置邮件 |
| ACME 账户/证书私钥 | DB 加密列（`serializer:acmeprivkey`，V6 第二波已收敛；`json:"-"` 挡 API） | PKI 域，已加密（2026-10-12） |
| SSH 私钥目录 | `~/.ssh` 或 `COCKPIT_SSH_KEYS`（0600） | agent 侧，`getDefaultKey` RPC 回传 PEM |
| agent 注册 secret | env/CLI/plist/Windows 服务路径 | 部署面（installer 域） |
| inventory capability `config.api_token` | DB 加密（`serializer:capconf` → `agents.capabilities` JSON 整体加密，V6 第二波已收敛） | 已加密（2026-10-12） |
| jenkins 类外部端点 | **不存在**（todo 预留） | 未来链接/webhook 型端点凭据走 vault ref |

加密基础设施现状：单一全局 AES-256-GCM 密钥 = `sha256(TOTP_ENCRYPTION_KEY)`（`storage/crypto.go`），无版本/无轮换/无 KDF（单轮 sha256）/无解锁仪式；`PRODUCTION=true` 时强制非弱密钥。

## 2. 决策

### V1 定位与边界

`internal/vault` = **服务端凭据保管抽象**。管：连接凭据（SSH/RDP/VNC）、外部端点凭据（jenkins 类，ref-by-path 预留）、NAS 目标凭据、未来 API token 类。**不管**：用户口令/agent secret（bcrypt 单向域）、JWT 签名密钥、TOTP 种子（auth 域，暂留原路径，格式兼容后续再并）、ACME 私钥（PKI 域，单独欠账）、agent 侧 SSH 私钥目录（部署面）。

### V2 Provider 接口

```go
type Entry struct {
    Path      string            // 唯一寻址：<域>/<名>，如 remote/<id>、nas/<name>
    Kind      string            // password / private_key / api_token / pair
    Username  string            // 可选配套用户名
    Value     string            // 明文，仅内存态
    Meta      map[string]string // 非敏感元数据（addr/type 等）
}
type Provider interface {
    Name() string
    Unlock(passphrase string) error // 幂等；已解锁时重验口令并刷新会话
    Lock()                          // 立即失效会话
    Read(path string) (*Entry, error)
    Write(path string, e *Entry) error
    List(prefix string) ([]EntryMeta, error) // 只回元数据，明文不出库
    Delete(path string) error
}
```

- 错误只带 path/原因类别，**永不含明文与口令**（红线，错误消息同日志纪律）。
- 审计统一在 vault 包发 `AuditLog`（action=`vault.read/write/delete/unlock`，details 只含 path+结果），实现不得自行落日志。

### V3 解锁与会话口径收敛（一处定义，各实现共用）

两层解锁，各司其职：

- **provider 级**：`Unlock/Lock` + TTL 自动上锁（`vault.Session`：TTL 默认无限=进程生命周期，可配）。本地实现 = 启动时从 key source 装载密钥；外部实现（persona 形态）= 真解锁握手 + 会话 TTL。
- **用户级**：既有 API 层 vault-token 门（登录口令/TOTP 重验 → 10min token → 写/删需 `X-Vault-Token`）**保持不变**——它已在用、已测，属 API 编排层，不属 provider。

KDF 只在 vault 包定义一处：env key 走 `sha256`（兼容存量密文）；主口令模式走 argon2id（V4 预留，M1 不实现交互式解锁——自托管无人值守重启优先）。

### V4 默认 local provider

- 存储：sqlite `vault_entries` 新表（`path` PK、`kind`、`username_enc`、`value_enc`、`meta` JSON、时间戳）。
- 密文格式：**与存量完全一致**（AES-256-GCM + base64，密钥 = `sha256(env key)`）→ 存量 remote_credentials/TOTP 行零迁移。
- 密文头版本前缀 `v1:`（新写入带版本，为轮换/换 KDF 预留 rewrap 路径；读时无前缀按 v0 旧格式解）。
- 主口令模式（`key_source: passphrase`）与外部提供商为 provider 注册表上的并列选项，接口不做特判。

### V5 密钥来源与红线

- 默认 key source：env `COCKPIT_VAULT_KEY`；未设时**回退 `TOTP_ENCRYPTION_KEY`**（同一派生口径，存量部署零改动）。
- `PRODUCTION=true` 强校验沿用（非空/≥32 字符/非默认前缀）。
- 红线：密钥不进仓库；解锁口令与明文凭据不落日志、不出现在错误消息；yaml 里密钥字段只允许 `${VAR}` 引用。

### V6 接入点改造分波

- **第一波（零协议改动）**：①remote-credential 加密/解密调用点改走 vault 收敛后的 crypto core（格式不变、零迁移）；②NAS targets：服务端 vault `nas/<name>` 条目为第一来源，agent 经 RPC 拉取，env 保留为回退与引导（`COCKPIT_NAS_TARGETS` 继续可用）。
- **第二波（已落地 2026-10-12，`internal/server/vault_secrets.go`）**：①DNS/overlay/notification/SMTP token 构造时经 `vault:<path>` 引用解析（env-wins 之上加 vault 源）；②inventory capability token 加密落库（`serializer:capconf`）；③ACME 私钥列加密（`serializer:acmeprivkey`）。
- jenkins 类：`links` URL 不嵌凭据，端点凭据走 vault ref（`external/<endpoint>`），第二波随链接功能立项。

### V7 注册与切换

`vault.provider: local | <外部>`；provider 注册表 `map[name]Factory`，未知 provider 启动即报错。每 provider 独立配置节 `vault.providers.<name>.*`。外部提供商（persona 形态）按同一 `Provider` 接口接入，vault 包不对其做类型特判。

### V8 兼容性

密文格式不变 → 存量 remote_credentials/TOTP 零迁移；`COCKPIT_NAS_TARGETS` 环境变量继续生效（vault 有 `nas/` 条目时优先）；不引入强制交互解锁，自托管重启语义不变。

## 3. 不变式

- 明文凭据只存在于：请求体内、provider 内存态、解密后立即使用的调用栈内；绝不进日志/审计/错误消息/HTTP 响应（列表只回元数据）。
- 审计必有 vault 读写删记录（含发起用户与结果），不含明文。
- provider 未知/未解锁 → 拒绝服务而非静默降级到明文。

## 4. 测试与验收

- vault 包单测：接口契约（解锁/上锁/TTL）、local provider CRUD、错误路径不带明文、审计钩子、v0/v1 密文兼容。
- remote-credential 既有测试全绿（格式兼容证明）。
- NAS RPC：agent 拉 vault targets 成功/失败回退 env。
- 配置：`vault.provider` 未知值启动报错；逐字段文档见下节。

## 5. 配置逐字段

### 5.1 配置文件（`vault:` 节，整节可省略 = 全默认）

| 字段 | 类型 | 默认值 | 含义 |
|------|------|--------|------|
| `vault.provider` | string | `local` | 密码箱提供商，经注册表解析（`internal/vault.Register`）；未知值**启动即报错**，不降级（V7）。切换 provider 只改此值，业务代码零改动 |
| `vault.session_ttl` | duration | `0` | provider 级解锁会话时长（`5m` 等 Go duration 写法）；`0` = 进程生命周期。local 实现随启动解锁，TTL 到期自动上锁后**需重启进程重新解锁**——自托管无人值守建议保持 `0`。用户级 vault token（10 分钟）在 API 层独立计时，不受此字段影响 |
| `vault.providers.persona.endpoint` | string | —（persona 选中时必填） | persona Connect API 基址（`persona connect serve`，如 `http://127.0.0.1:8747`）。仅在 `vault.provider: persona` 时实例化；缺失/非法启动即报错（fail-fast 不降级） |
| `vault.providers.persona.token` | string | —（必填） | persona Connect token（`pconn_…`，视同密码）；`PERSONA_CONNECT_TOKEN` env 优先，yaml 不落 secret。token 被拒（401）为显式错误，不按锁定处理 |
| `vault.providers.persona.timeout` | duration | `5s` | Connect API HTTP 超时；须为正 |
| `vault.providers.persona.session_ttl` | duration | `0`（回退 `vault.session_ttl`） | persona 会话时长；`Unlock` 走 health+items 双探握手，TTL 到期自动上锁后重跑 `Unlock` 即恢复 |

`vault.providers.*` 子树按已知字段**严格解码**：未知字段（含未接入的
provider 名）`Load` 即报错，防拼错键静默失效。`vault.providers` 节缺失或为
空值时跳过检查，local-only 部署的配置不产生空节。

```yaml
# 密码箱（provider 可替换；密钥只用 env 注入，yaml 中无任何密钥字段）
vault:
  provider: local
  session_ttl: 0
  # providers:            # 仅 provider 选中对应实现时生效（V7）
  #   persona:
  #     endpoint: http://127.0.0.1:8747
  #     timeout: 5s
  #     session_ttl: 30m   # token 用 PERSONA_CONNECT_TOKEN env 注入
```

#### vault: 引用（V6 第二波，已落地）

各凭据字段的值可写成 `vault:<path>` 显式引用，启动时经 provider 解析为条目 value
（只进内存 config；解析失败启动即终止，错误只含字段名与 path）：

```yaml
dns:
  cloudflare:
    api_token: "vault:dns/cloudflare"     # 条目 value=API token
notification:
  telegram:
    - bot_token: "vault:notify/telegram"
      chat_id: "123456"
```

- **优先级 env > yaml > vault**：env 覆盖（`CLOUDFLARE_API_TOKEN` 等）先于 yaml 生效，
  vault 引用只在 env 未覆盖该字段时存活；不做约定路径隐式回填（决策 D-2026-10-12-1）。
- **支持字段**：`dns.cloudflare.api_token`、`dns.dnspod.login_token`、
  `dns.alidns.access_key`/`secret_key`、`overlay.zerotier.api_token`、
  `overlay.tailscale.api_token`、`notification.ntfy[].token`、
  `notification.webhook[].secret`、`notification.telegram[].bot_token`、
  `email.smtp.password`。
- 条目口径：`value`=凭据本体；引用 path 无大小写/`..`（与条目 path 约定一致）。

### 5.2 环境变量（密钥注入，V5）

| 变量 | 必填 | 默认值 | 含义 |
|------|------|--------|------|
| `COCKPIT_VAULT_KEY` | 生产必填 | 回退 `TOTP_ENCRYPTION_KEY` | local provider 加密主密钥，`openssl rand -base64 32` 生成。未设置/过短（<32 字符）/命中内置弱默认前缀 → 启动 WARNING 并回落弱默认（仅供开发，日志可见，不阻断） |
| `TOTP_ENCRYPTION_KEY` | 生产必填（既有） | 内置弱默认 | 既有 TOTP 域密钥；作为 `COCKPIT_VAULT_KEY` 未设置时的回退。两域共用密钥链入口但存储格式独立（TOTP 走 v0 裸格式，vault 写入走 `v1:` 版本前缀） |
| `PERSONA_CONNECT_TOKEN` | `provider: persona` 时必填 | yaml 同名字段 | persona Connect token（`pconn_…`，视同密码）；env 优先于 yaml（惰性物化配置节，local-only 部署 yaml 不出现 persona 节）。token 永不进日志/错误消息/审计 |

设置优先级：`COCKPIT_VAULT_KEY` > `TOTP_ENCRYPTION_KEY` > 内置弱默认（告警）。

### 5.3 条目 path 约定（provider 无关）

| 前缀 | Kind | 字段 | 消费方 |
|------|------|------|--------|
| `nas/<name>` | `pair` | `username`；`value`=口令；`meta.type`（dsm/truenas/omv）、`meta.addr`（必须带协议）、`meta.insecure_tls`（`"true"` 自签） | server 经注册响应/nas.set_targets 下发 agent（V6）；变更即对在线 agent 补推 |
| `remote/...` | — | 远控凭据（既有 remote-credential 存储已收敛到 vault core，格式零迁移） | SSH/RDP/VNC 会话建立 |

path 规则：小写字母数字与 `/ _ . -`，不以 `/` 开头、不含 `..`（API 层校验，违规 400）。

### 5.4 同链路既有配置

- `COCKPIT_NAS_TARGETS`（agent env JSON）：继续生效（零迁移）；vault 存在 `nas/` 条目时**整体覆盖** env 引导值（vault 为第一来源，V8）。
- API 层 vault token：登录口令（+TOTP）重验签发、10 分钟、`X-Vault-Token` 头携带；条目写/删共用此门，列表只回元数据。

### 5.5 persona provider（只读接入，2026-10-13）

persona 密码箱经其 **HTTP Connect API**（`persona connect serve`，127.0.0.1 +
Bearer token）接入为第二 provider，实现 `internal/vault/persona.go`；逐操作
映射、错误分类与卡点见 [persona-vault-mapping.md](persona-vault-mapping.md)。

- **只读**：persona Connect API 无写入/删除端点（写入路径上游未拍板）——
  `Write`/`Delete` 会话门后显式 `ErrUnsupported`，不静默、不降级。条目只能在
  persona 侧维护，cockpit 只读消费。
- **Unlock 语义**：persona 无消费者解锁端点（凭据是 Connect token，vault 解锁
  是属主行为）——`Unlock(passphrase)` 的口令参数不参与，握手 =
  `GET /health`（存活）+ `GET /items`（token 有效且 vault 已解锁）双探；
  `Lock` 仅关本地会话门。
- **fail-closed**：连不上/超时/503 `vault_locked`/421/403/其他 5xx 一律包
  `ErrLocked` 拒绝服务；401 token 被拒为显式错误（处置是换 token，不等解锁）；
  404 → `ErrNotFound`。任何失败都不回退 local（V7 红线）。
- **条目映射**：`Entry.Path`=persona 条目 UUID；`Kind`=persona type slug
  （`password`/`api_key`/`totp`/`note`/`server_config`/`game_account`/
  `software_license` 直通）；`Value` 取 `data` 变体的秘密字段
  （`Password.password`、`ApiKey.api_key`、`TwoFactor.secret_key`、
  `SecureNote.note`、`ServerConfig.password`（可空）、`GameToken.secret_key`、
  `SoftwareLicense.license_key`）；`bank_card`/`certificate`/`identity` 无单一
  秘密字段，`Value` 空、`Meta.persona_data` 列字段名清单（非敏感）。
  `title`/`urls`/`updated_at` 进 `Meta.persona_*`（非敏感）。
- **存量兼容**：切换 provider 只改 `vault.provider`，local 的 `vault_entries`
  存量不动；切回 `local` 原条目完好（有测试佐证）。
