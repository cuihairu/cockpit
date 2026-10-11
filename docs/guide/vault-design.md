# 密码箱抽象与可替换提供商（VaultProvider）设计

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
| ACME 账户/证书私钥 | DB **明文**列（仅 `json:"-"` 挡 API） | PKI 域，已知欠账（后续单独立项） |
| SSH 私钥目录 | `~/.ssh` 或 `COCKPIT_SSH_KEYS`（0600） | agent 侧，`getDefaultKey` RPC 回传 PEM |
| agent 注册 secret | env/CLI/plist/Windows 服务路径 | 部署面（installer 域） |
| inventory capability `config.api_token` | **明文落 DB**（`agents.capabilities` JSON） | 已知欠账（随 vault 第二波收敛） |
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
- **第二波（登记后续，不卡 M1）**：DNS/overlay/notification/SMTP token 构造时从 vault 解析（env-wins 之上加 vault 源）；inventory capability token 落库改 ref；ACME 私钥列加密。
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
- 配置：`vault.provider` 未知值启动报错；逐字段文档见 getting-started/config 参考。
