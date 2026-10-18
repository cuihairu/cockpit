# persona Vault → cockpit `vault.Provider` Interface Mapping

> Language: [中文](./persona-vault-mapping.md) ｜ **English**

> 2026-10-13. Basis document for the integration implementation (`internal/vault/persona.go`). All persona-side facts are taken from the persona upstream repo's source code and its `docs/EXTERNAL_INTEGRATION.md` (the 2026-10-12 empirically tested edition; that document lives in the persona repo, **not in this repo**), each with file:line references; line numbers may drift as persona upstream evolves — where they differ, the cited semantics are authoritative.

## 1. persona-side interface shape (source of truth)

persona's formal interface to external projects is the **HTTP Connect API** (`persona connect serve`, listening only on 127.0.0.1, token authentication). Five endpoints (`connect-server/src/lib.rs:90-96` `build_router`):

| Endpoint | Auth | Semantics |
|------|------|------|
| `GET /api/v1/connect/health` | No auth | Liveness probe, zero vault info (lib.rs:1072 note: unauthenticated but not exempt from the Host allowlist) |
| `GET /api/v1/connect/identities` | Bearer token | Identity list within scope |
| `GET /api/v1/connect/items` | Bearer token | Item metadata list; query filters `identity`/`type`/`title` (lib.rs:411-434 `ItemsQuery`) |
| `GET /api/v1/connect/items/{id}` | Bearer token | Single item with all fields (**decrypted** secrets); out-of-scope/archived/nonexistent all give the same-shape 404 (lib.rs:472-490) |
| `POST /api/v1/connect/items/{id}/totp` | Bearer token | Compute a TOTP code on the spot (unused by this integration in v1) |

Key semantics (all empirically verified / verifiable in source):

- **Consumers have no unlock/lock endpoint**. Vault unlocking is an owner action (CLI/desktop); the consumer credential is a **Connect token** (`pconn_…`, shown only once at issuance, only SHA-256 stored server-side, can carry `--expires-in` for expiry). Unknown/revoked/expired tokens all give the same-shape 401 `unauthorized` (`EXTERNAL_INTEGRATION.md` error-code table; auth resolution at `lib.rs:276-283`).
- **Read-only**: no write/delete endpoints (`EXTERNAL_INTEGRATION.md` open question: "the write path is not settled; the current interface is read-only"; corroborated by the routing table at lib.rs:92-96).
- **Locked means 503**: vault not unlocked → 503 `vault_locked`, does not consume quota (lib.rs:301-326, `locked_response`); you may back off and retry while waiting for the owner to unlock.
- **Three lines of defense**: the host portion of the Host header (port stripped) must be loopback (lib.rs:227-250, 421 `bad_host`); an `Origin` header present → 403 (browser anti-cross-origin); 120 req/min per token → 429 (lib.rs:19 doc comment).
- **Envelope**: success `{"ok":true,"data":…}`, error `{"ok":false,"error":{code,message}}` (lib.rs:144/155 `ok_json`/`err_json`).
- **Item model**: metadata `{id,title,type,urls[],updated_at}` (lib.rs:449-456 `item_meta_json`); details add `username` and `data` (lib.rs:476-487). `type` uses the scope vocabulary in snake_case (lib.rs:439-447 `item_type_slug`; `core/src/connect/token.rs:16-30` `ConnectItemType`, serde rename_all): `password / api_key / totp / note / bank_card / server_config / certificate / game_account / identity / software_license` (ssh-key/crypto-wallet/custom are **never authorizable**, token.rs:47). `data` is a serde externally-tagged enum (`core/src/models/credential.rs:459-474` `CredentialData`), keys are the variant names in PascalCase (`Password`/`ApiKey`/`TwoFactor`/`SecureNote`/`ServerConfig`/`GameToken`/`SoftwareLicense`/…); each variant's fields are in the same file: `PasswordCredentialData`(:282), `ApiKeyData`(:324), `ServerConfigData`(:347), `GameTokenData`(:390), `SecureNoteData`(:414), `SoftwareLicenseData`(:440).
- **Audit**: on the persona side every token validation/rejection leaves an audit record (`EXTERNAL_INTEGRATION.md` permission-boundary table); on the cockpit side operations are recorded into `audit_logs` via this repo's `AuditFunc` hook (V2).

## 2. Six-operation mapping

| cockpit `vault.Provider` (`internal/vault/vault.go:31-41`) | persona side | Implementation notes (`internal/vault/persona.go`) |
|---|---|---|
| `Name()` | — | Constant `"persona"` |
| `Unlock(passphrase)` | **No consumer unlock endpoint** — the credential is a Connect token injected via config; unlocking is an owner action | `passphrase` is not used (persona has no passphrase semantics; empty string is valid). Handshake = `GET /health` (alive) + `GET /items` (token valid + vault unlocked), both probes passing → `Session.Unlock()`. Idempotent: re-running the handshake while unlocked refreshes the TTL |
| `Lock()` | No corresponding endpoint | Local `Session.Lock()` only — subsequent operations are rejected at the local gate (ErrLocked); no network request is sent |
| `Read(path)` | `GET /items/{id}` | `path` = item UUID; `type` slug passes through to `Entry.Kind`; `username` → `Entry.Username`; `data.<variant>.<secret-field>` → `Entry.Value` (mapping in §3); `title`/`urls` → `Entry.Meta` (non-sensitive). 404 → `ErrNotFound` |
| `Write(path, e)` | **Does not exist** (read-only interface) | Explicit `ErrUnsupported` behind the session gate — not silently swallowed, no fallback to local |
| `List(prefix)` | `GET /items` | Metadata mapped directly; `prefix` filters client-side by `path` (UUID) prefix; the list has no `username` → `HasUsername=false`; `HasValue` = whether that type has a secret-field mapping (§3's seven types are true, `bank_card`/`certificate`/`identity` are false, consistent with Read behavior) |
| `Delete(path)` | **Does not exist** | Same as Write, explicit `ErrUnsupported` |

Error mapping (fail-closed, no fallback per V7):

| persona side | cockpit side |
|---|---|
| Connection failure / timeout / 421 / 403 / other 5xx | `fmt.Errorf("…: %w", ErrLocked)` — the caller refuses service per locked semantics, no fallback to local (V7 red line) |
| 503 `vault_locked` (owner has not unlocked) | Same as above, wrapping `ErrLocked` |
| 401 `unauthorized` (token invalid/revoked/expired) | Explicit token error (**not** wrapped in `ErrLocked` — the remediation is to swap the token, not wait for an unlock; a deterministic failure is not retried) |
| 429 `rate_limited` | Explicit rate-limit error (`rate limited (120 req/min per token)`; no automatic retry currently; the caller backs off as needed) |
| 404 `not_found` | `ErrNotFound` (decidable via errors.Is) |

Session policy: reuse `vault.Session` (`internal/vault/session.go:10-52`, auto-lock on TTL expiry) — the persona TTL comes from `vault.providers.persona.session_ttl`, falling back to the global `vault.session_ttl` if unset (both `0` = process lifetime). After TTL expiry operations return `ErrLocked`, requiring a fresh `Unlock` (re-run the double-probe handshake).

## 3. `Entry.Value` secret-field mapping (`data` PascalCase variant → secret field)

| `type` (slug) | `data` variant | Secret field (credential.rs line) | cockpit `Entry.Value` |
|---|---|---|---|
| `password` | `Password` | `password` (:283) | The value |
| `api_key` | `ApiKey` | `api_key` (:326; the optional `api_secret`/`token` fields are not taken in v1) | The value |
| `totp` | `TwoFactor` | `secret_key` (near :377, `TwoFactorData`) | The value (the TOTP shared secret) |
| `note` | `SecureNote` | `note` (:417) | The value |
| `server_config` | `ServerConfig` | `password` (:353, an `Option` that may be null → empty string) | The value (may be empty) |
| `game_account` | `GameToken` | `secret_key` (:394) | The value |
| `software_license` | `SoftwareLicense` | `license_key` (:442) | The value |
| `bank_card` / `certificate` / `identity` | Respective variants | No single secret field (card number/CVV/ID number are multi-field combinations, not stuffed into one value) | Empty string + `Meta.persona_data` listing the field names (non-sensitive) |

## 4. Blockers (what persona lacks, recorded as-is)

1. **No write/delete endpoints** (the biggest gap): under `vault.providers.persona`, `Write`/`Delete` are forever `ErrUnsupported`. When the persona provider is selected, cockpit's item-management APIs (PUT/DELETE `/api/remote/vault/entries`) have these two paths unavailable — persona items can only be maintained on the persona side; cockpit consumes them read-only. Once upstream settles the "write path", this provider just adds the implementation (the interface is unchanged).
2. **No consumer unlock/lock endpoints**: the `Unlock` "passphrase" parameter has no semantics (the token is the credential; the owner unlocks the vault); `Lock` can only lock the local session gate, not persona itself. The passphrase entered in the UI's "unlock vault" is decorative for the persona provider — a real mapping exists only after upstream defines consumer-side session semantics (e.g. exchanging the token for a short-lived session).
3. **The `use` verb has no HTTP endpoint** (a lib.rs test comment says "no consumption endpoint for now"): TOTP code generation via `POST /items/{id}/totp` works, but v1's `Entry` has no totp field, so it is not wired up; when cockpit later needs to display TOTP codes, this will be extended via Meta or a separate channel.
4. **List metadata does not indicate whether `username` exists**: `HasUsername` is always false (only the detail read has it), so list views cannot predict; once upstream metadata adds the field, this can be filled in.
5. **No proactive notification of token expiry**: persona returns 401 at expiry (same shape as invalid), and cockpit can only explicitly report "token rejected" on failure; it is recommended to set `--expires-in` per rotation period at issuance and to rotate manually before expiry (the documentation stance matches `EXTERNAL_INTEGRATION.md`'s consumer guidance).
