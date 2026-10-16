# Password Vault Abstraction and Replaceable Providers (VaultProvider) Design

> Language: [中文](./vault-design.md) ｜ **English**

> Status: **M1 design finalized (2026-10-12), implementation advancing in waves**. Source: todo 2026-10-11 user directive — the persona upgrade has already implemented a password vault that can serve as a website vault; per-project vaults should abstract their interfaces and support replaceable providers.

## 1. Inventory (enumerate everything first, as the todo requires)

All channels through which credentials enter the system (repo-wide inventory 2026-10-12):

| Channel | Location | Current state |
|---|---|---|
| SSH/RDP/VNC connection credentials | `remote_credentials` table (password/private_key AES-GCM) | A vault prototype already exists: `PUT /api/remote/vault/credentials` + login-password/TOTP re-verification exchanging for a 10min vault token (`api_vault.go`) |
| NAS targets | agent env `COCKPIT_NAS_TARGETS` (JSON array containing username+password) | Plaintext env; credential discipline is by convention (never into snapshots/logs/errors) |
| TOTP seeds | `users.totp_secret` (AES-GCM) | auth domain, shares the same env-derived key with connection credentials |
| User passwords / agent secrets | bcrypt hashes | Not a vault domain (one-way hash, no decryption need) |
| DNS/overlay API tokens | yaml fields + env override (env-wins convention, "secrets don't land in files") | Read on the server side at construction time |
| notification (ntfy token/webhook secret/telegram bot_token) | yaml | Read on the server side at send time |
| SMTP relay | env `SMTP_PASS` / yaml | Password-reset emails |
| ACME account/certificate private keys | Encrypted DB columns (`serializer:acmeprivkey`, converged in the V6 second wave; `json:"-"` blocks the API) | PKI domain, already encrypted (2026-10-12) |
| SSH private key directory | `~/.ssh` or `COCKPIT_SSH_KEYS` (0600) | Agent side; `getDefaultKey` RPC returns PEM |
| Agent registration secrets | env/CLI/plist/Windows service paths | Deployment surface (installer domain) |
| inventory capability `config.api_token` | Encrypted in DB (`serializer:capconf` → `agents.capabilities` JSON encrypted as a whole, converged in the V6 second wave) | Already encrypted (2026-10-12) |
| jenkins-style external endpoints | **Does not exist** (reserved by todo) | Future link/webhook-style endpoint credentials go through a vault ref |

Current state of the encryption infrastructure: a single global AES-256-GCM key = `sha256(TOTP_ENCRYPTION_KEY)` (`storage/crypto.go`), no version, no rotation, no KDF (single sha256 round), no unlock ceremony; `PRODUCTION=true` enforces a non-weak key.

## 2. Decisions

### V1 Positioning and Boundaries

`internal/vault` = **server-side credential custody abstraction**. Governs: connection credentials (SSH/RDP/VNC), external endpoint credentials (jenkins-style, ref-by-path reserved), NAS target credentials, future API-token classes. **Does not govern**: user passwords/agent secrets (the bcrypt one-way domain), JWT signing keys, TOTP seeds (auth domain, left on the original path for now, format-compatible so they can be merged later), ACME private keys (PKI domain, a separate outstanding debt), the agent-side SSH private key directory (deployment surface).

### V2 Provider Interface

```go
type Entry struct {
    Path      string            // Unique addressing: <domain>/<name>, e.g. remote/<id>, nas/<name>
    Kind      string            // password / private_key / api_token / pair
    Username  string            // Optional companion username
    Value     string            // Plaintext, in-memory state only
    Meta      map[string]string // Non-sensitive metadata (addr/type etc.)
}
type Provider interface {
    Name() string
    Unlock(passphrase string) error // Idempotent; when already unlocked, re-verify the passphrase and refresh the session
    Lock()                          // Invalidate the session immediately
    Read(path string) (*Entry, error)
    Write(path string, e *Entry) error
    List(prefix string) ([]EntryMeta, error) // Metadata only; plaintext never leaves the store
    Delete(path string) error
}
```

- Errors carry only the path/reason category, **never plaintext or passphrases** (red line, error messages follow the logging discipline).
- Auditing is uniformly emitted by the vault package as `AuditLog` (action=`vault.read/write/delete/unlock`, details containing only path+result); implementations must not log on their own.

### V3 Converging Unlock and Session Semantics (defined once, shared by all implementations)

Two layers of unlock, each with its own job:

- **Provider level**: `Unlock/Lock` + automatic lock on TTL expiry (`vault.Session`: TTL defaults to unlimited = process lifetime, configurable). Local implementation = load the key from the key source at startup; external implementation (persona-shaped) = a real unlock handshake + session TTL.
- **User level**: the existing API-layer vault-token gate (login password/TOTP re-verification → 10min token → writes/deletes require `X-Vault-Token`) **stays unchanged** — it is already in use and tested, belongs to the API orchestration layer, not the provider.

The KDF is defined in one place only, in the vault package: the env key uses `sha256` (compatible with existing ciphertext); master-passphrase mode uses argon2id (reserved in V4; M1 does not implement interactive unlock — unattended restarts for self-hosting come first).

### V4 Default local Provider

- Storage: a new sqlite `vault_entries` table (`path` PK, `kind`, `username_enc`, `value_enc`, `meta` JSON, timestamps).
- Ciphertext format: **fully identical to existing** (AES-256-GCM + base64, key = `sha256(env key)`) → zero migration for existing remote_credentials/TOTP rows.
- Ciphertext header version prefix `v1:` (new writes carry the version, reserving a rewrap path for rotation/KDF changes; reads without a prefix are decoded as v0 old format).
- Master-passphrase mode (`key_source: passphrase`) and external providers are parallel options in the provider registry; the interface does not special-case them.

### V5 Key Sources and Red Lines

- Default key source: env `COCKPIT_VAULT_KEY`; when unset, **falls back to `TOTP_ENCRYPTION_KEY`** (the same derivation basis, zero changes for existing deployments).
- The `PRODUCTION=true` strict validation carries over (non-empty / ≥32 chars / not the default prefix).
- Red lines: keys never enter the repository; the unlock passphrase and plaintext credentials never go into logs or error messages; yaml key fields may only hold `${VAR}` references.

### V6 Integration-Point Rework by Wave

- **Wave 1 (zero protocol changes)**: ① the remote-credential encryption/decryption call sites switch to the vault's converged crypto core (format unchanged, zero migration); ② NAS targets: the server-side vault `nas/<name>` entry is the primary source, the agent pulls it via RPC, and env is retained as fallback and bootstrap (`COCKPIT_NAS_TARGETS` continues to work).
- **Wave 2 (landed 2026-10-12, `internal/server/vault_secrets.go`)**: ① DNS/overlay/notification/SMTP tokens resolve through `vault:<path>` references at construction time (adding a vault source on top of env-wins); ② inventory capability token encrypted into the DB (`serializer:capconf`); ③ ACME private key columns encrypted (`serializer:acmeprivkey`).
- jenkins-style: `links` URLs do not embed credentials; endpoint credentials go through a vault ref (`external/<endpoint>`), starting a project together with the links feature in the second wave.

### V7 Registration and Switching

`vault.provider: local | <external>`; provider registry `map[name]Factory`, an unknown provider is a startup error. Each provider has its own config section `vault.providers.<name>.*`. External providers (persona-shaped) plug in through the same `Provider` interface; the vault package does not type-check them.

### V8 Compatibility

Ciphertext format unchanged → zero migration for existing remote_credentials/TOTP; the `COCKPIT_NAS_TARGETS` env variable continues to work (takes priority when the vault has a `nas/` entry); no mandatory interactive unlock is introduced, self-hosted restart semantics unchanged.

## 3. Invariants

- Plaintext credentials exist only in: request bodies, provider in-memory state, and the call stack immediately after decryption; never in logs/audits/error messages/HTTP responses (list returns metadata only).
- Audits must include vault read/write/delete records (with the initiating user and the result), containing no plaintext.
- An unknown/unlocked provider → refuse service rather than silently degrading to plaintext.

## 4. Testing and Acceptance

- vault package unit tests: interface contracts (unlock/lock/TTL), local provider CRUD, error paths carrying no plaintext, audit hooks, v0/v1 ciphertext compatibility.
- All existing remote-credential tests green (proving format compatibility).
- NAS RPC: the agent's pull of vault targets succeeds / falls back to env on failure.
- Config: an unknown `vault.provider` value is a startup error; per-field documentation in the next section.

## 5. Configuration, Field by Field

### 5.1 Config File (the `vault:` section, the whole section may be omitted = all defaults)

| Field | Type | Default | Meaning |
|------|------|--------|------|
| `vault.provider` | string | `local` | The vault provider, resolved via the registry (`internal/vault.Register`); an unknown value is **a startup error**, no degradation (V7). Switching providers changes only this value, zero changes in business code |
| `vault.session_ttl` | duration | `0` | The provider-level unlock session duration (Go duration syntax such as `5m`); `0` = process lifetime. The local implementation unlocks at startup; after the TTL expires and it auto-locks, **the process must restart to re-unlock** — keep `0` for unattended self-hosting. The user-level vault token (10 minutes) is timed independently at the API layer and is unaffected by this field |
| `vault.providers.persona.endpoint` | string | — (required when persona is selected) | The persona Connect API base URL (`persona connect serve`, e.g. `http://127.0.0.1:8747`). Instantiated only when `vault.provider: persona`; missing/invalid is a startup error (fail-fast, no degradation) |
| `vault.providers.persona.token` | string | — (required) | The persona Connect token (`pconn_…`, treated as a password); the `PERSONA_CONNECT_TOKEN` env takes priority, secrets do not land in yaml. A rejected token (401) is an explicit error, not treated as locked |
| `vault.providers.persona.timeout` | duration | `5s` | The Connect API HTTP timeout; must be positive |
| `vault.providers.persona.session_ttl` | duration | `0` (falls back to `vault.session_ttl`) | The persona session duration; `Unlock` performs a health+items dual-probe handshake, and after the TTL expires and it auto-locks, re-running `Unlock` restores it |

The `vault.providers.*` subtree is **strictly decoded** against known fields: unknown fields (including un-integrated provider names) error at `Load`, preventing silently ineffective misspelled keys. If the `vault.providers` section is missing or empty, the check is skipped, so local-only deployments do not produce an empty section.

```yaml
# Password vault (provider swappable; secrets are injected only via env, no secret field anywhere in yaml)
vault:
  provider: local
  session_ttl: 0
  # providers:            # Takes effect only when provider selects the matching implementation (V7)
  #   persona:
  #     endpoint: http://127.0.0.1:8747
  #     timeout: 5s
  #     session_ttl: 30m   # token injected via the PERSONA_CONNECT_TOKEN env
```

#### vault: references (V6 wave 2, landed)

The value of each credential field may be written as an explicit `vault:<path>` reference, resolved at startup through the provider into the entry's value
(memory-only config; a resolution failure terminates startup, and the error contains only the field name and path):

```yaml
dns:
  cloudflare:
    api_token: "vault:dns/cloudflare"     # entry value=API token
notification:
  telegram:
    - bot_token: "vault:notify/telegram"
      chat_id: "123456"
```

- **Priority env > yaml > vault**: env overrides (such as `CLOUDFLARE_API_TOKEN`) apply before yaml,
  and a vault reference survives only when env does not override that field; no implicit backfill on a conventional path (decision D-2026-10-12-1).
- **Supported fields**: `dns.cloudflare.api_token`, `dns.dnspod.login_token`,
  `dns.alidns.access_key`/`secret_key`, `overlay.zerotier.api_token`,
  `overlay.tailscale.api_token`, `notification.ntfy[].token`,
  `notification.webhook[].secret`, `notification.telegram[].bot_token`,
  `email.smtp.password`.
- Entry conventions: `value`= the credential itself; the reference path has no case/`..` (consistent with the entry path convention).

### 5.2 Environment Variables (key injection, V5)

| Variable | Required | Default | Meaning |
|------|------|--------|------|
| `COCKPIT_VAULT_KEY` | Required in production | Falls back to `TOTP_ENCRYPTION_KEY` | The local provider's master encryption key, generated with `openssl rand -base64 32`. Unset / too short (<32 chars) / matching a built-in weak default prefix → startup WARNING and fallback to the weak default (development only, visible in logs, non-blocking) |
| `TOTP_ENCRYPTION_KEY` | Required in production (existing) | Built-in weak default | The existing TOTP domain key; serves as the fallback when `COCKPIT_VAULT_KEY` is unset. The two domains share the key-chain entry but store formats independently (TOTP uses the v0 bare format, vault writes use the `v1:` version prefix) |
| `PERSONA_CONNECT_TOKEN` | Required when `provider: persona` | The yaml field of the same name | The persona Connect token (`pconn_…`, treated as a password); env takes priority over yaml (the config section is materialized lazily, local-only deployments have no persona section in yaml). The token never enters logs/error messages/audits |

Setting priority: `COCKPIT_VAULT_KEY` > `TOTP_ENCRYPTION_KEY` > built-in weak default (with a warning).

### 5.3 Entry Path Conventions (provider-agnostic)

| Prefix | Kind | Fields | Consumer |
|------|------|------|--------|
| `nas/<name>` | `pair` | `username`; `value`= password; `meta.type` (dsm/truenas/omv), `meta.addr` (must carry a protocol), `meta.insecure_tls` (`"true"` for self-signed) | Server delivers to agents via the registration response/nas.set_targets (V6); changes are re-pushed to online agents immediately |
| `remote/...` | — | Remote-control credentials (the existing remote-credential storage has converged to the vault core, zero format migration) | SSH/RDP/VNC session establishment |

Path rules: lowercase alphanumerics and `/ _ . -`, no leading `/`, no `..` (validated at the API layer, violations → 400).

### 5.4 Existing Configuration on the Same Path

- `COCKPIT_NAS_TARGETS` (agent env JSON): continues to work (zero migration); when the vault has a `nas/` entry it **overrides the env bootstrap value as a whole** (the vault is the primary source, V8).
- API-layer vault token: issued on login-password (+TOTP) re-verification, 10 minutes, carried in the `X-Vault-Token` header; entry writes/deletes share this gate, list returns metadata only.

### 5.5 persona Provider (read-only integration, 2026-10-13)

The persona vault is integrated as a second provider through its **HTTP Connect API** (`persona connect serve`, 127.0.0.1 + Bearer token), implemented in `internal/vault/persona.go`; per-operation
mapping, error classification, and blockers are in [persona-vault-mapping.md](persona-vault-mapping.en.md).

- **Read-only**: the persona Connect API has no write/delete endpoints (the write path is not decided upstream) —
  `Write`/`Delete` return an explicit `ErrUnsupported` after the session gate, no silent behavior, no degradation. Entries can only be
  maintained on the persona side; cockpit consumes them read-only.
- **Unlock semantics**: persona has no consumer-unlock endpoint (the credential is the Connect token; vault unlock
  is an owner action) — the passphrase parameter of `Unlock(passphrase)` is not used; the handshake =
  `GET /health` (liveness) + `GET /items` (token valid and vault unlocked) dual probe;
  `Lock` only closes the local session gate.
- **fail-closed**: unreachable/timeout/503 `vault_locked`/421/403/other 5xx are all wrapped in
  `ErrLocked` and refuse service; a 401 rejected token is an explicit error (the remedy is to swap the token, not wait for an unlock);
  404 → `ErrNotFound`. No failure falls back to local (V7 red line).
- **Entry mapping**: `Entry.Path`= the persona entry UUID; `Kind`= the persona type slug
  (`password`/`api_key`/`totp`/`note`/`server_config`/`game_account`/
  `software_license` pass through); `Value` takes the secret field of the `data` variant
  (`Password.password`, `ApiKey.api_key`, `TwoFactor.secret_key`,
  `SecureNote.note`, `ServerConfig.password` (nullable), `GameToken.secret_key`,
  `SoftwareLicense.license_key`); `bank_card`/`certificate`/`identity` have no single
  secret field, so `Value` is empty and `Meta.persona_data` lists the field names (non-sensitive).
  `title`/`urls`/`updated_at` go into `Meta.persona_*` (non-sensitive).
- **Existing-data compatibility**: switching providers changes only `vault.provider`, and the existing local
  `vault_entries` data is untouched; switching back to `local` leaves the original entries intact (backed by tests).
