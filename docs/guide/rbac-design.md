# Design: Multi-Account and Fine-Grained Permission Control (RBAC)


> 2026-09-19. First-phase design for multi-account permission control. Original requirement: "account permission control, support multiple accounts".
> This document covers design decisions and interface definitions only; implementation is phased P0/P1.

## Current State and Pain Points

The multi-account base already exists: the `User` table (username/password/TOTP/department), a user CRUD API, and JWT carrying
`Role`. What's missing is the permission model — decisions rely entirely on scattered string comparisons:

| Pain point | Current state | Consequence |
|------|------|------|
| Only two role levels | The `Role` field hardcodes the two values `admin` / `user` | You cannot give an ops colleague "can operate but not manage users" |
| Decisions scattered | 10+ handlers hand-write `user.Role != "admin"` (`api.go` 394/703/802/844/850/893/911…) | New modules easily miss a check; auditing "who can do what" requires grepping by hand |
| No permission semantics | "admin" simultaneously means user management, certificate issuance, file read/write, terminal access | Cannot answer "can this account touch production files" |
| The user role is undefined | Non-admins can do almost nothing except edit their own profile, but this is an implicit convention | Frontend menus and backend decisions each go their own way |

## Architecture Overview

RBAC-lite: **user → role → permission points**. Permission decisions converge into a single server-side middleware;
the agent and websocket channels get zero changes (permissions are closed off at the server; the agent only accepts one-way calls from the server).

```
┌─ web ───────────────────────────────┐      ┌─ server ────────────────────────────────────────────┐
│ route guard + v-perm directive      │─JWT─▶│ auth middleware: verify signature → UserInfo{Role}  │
│                                     │      │   ↓                                                 │
│ /api/me returns                     │      │ requirePermission(perm): look up role table by Role │
│ permissions[]                       │      │   ↓ (local SQLite read, microseconds, no caching)   │
│ menus/buttons trimmed by permission │      │ handler: remove scattered Role != "admin"           │
└─────────────────────────────────────┘      └─────────────────────────────────────────────────────┘
```

## Key Decisions

| # | Decision | Choice | Rationale |
|---|------|------|------|
| D1 | Permission model | RBAC (role → set of permission points), no ABAC/ACL | The admin-plane user count is single digits; granting by role suffices; ABAC's attribute conditions have nowhere to source values |
| D2 | Permission point granularity | `<resource>:<action>`, action ∈ `read` / `write` / `admin` | Module level is enough (e.g. `files:write`, `acme:admin`); button-level authorization costs more than it returns |
| D3 | resource list | One-to-one with existing API modules: `inventory` `files` `logs` `terminal` `docker` `stack` `cron` `backup` `acme` `dns` `ddns` `proxy` `overlay` `drift` `nas` `alerts` `audit` `users` `roles` `settings`; reconciliation has since added `services` `smart` `recordings` `firewall` (firewall-design D10, read-only tier) `jobs` (jobs-design) `workflows` (workflow-design W9); `overlay` includes an `admin` tier (M3 D28, the join/leave/service host-side write operations). The approval gate `approvals` (workflow-design M2d) was registered with the 2026-10-15 documentation audit (previously the route was registered but the permission point was missing, causing endpoint 403s). The current full set takes `internal/storage/role.go` `resourceActions` as the single source of truth | The existing backend route prefixes are natural resources; no new taxonomy invented |
| D4 | Role storage | New `Role` table (`name` primary key, `permissions` JSON, `builtin` flag); `User.role` keeps storing the role name as a string | One GORM AutoMigrate table does it; User needs no foreign key — the role name is a soft reference |
| D5 | Where permissions are checked | Add `requirePermission(perm)` after the auth middleware; the `admin` permission point implies read/write for that resource | One choke point in the middleware chain; "admin implies read/write" frees the built-in admin role from enumerating every permission point |
| D6 | Query the DB per request | Validation queries the `roles` table directly, no in-memory cache | Admin-plane QPS is single digits; cache-invalidation logic (changing a role must kick online users) costs far more than the queries it saves |
| D7 | JWT content | Keep the status quo: the token carries only the role name | Role definition changes take effect **immediately** on issued tokens (a good property); stuffing permission points into the token would instead require handling "the token hasn't expired but permissions have narrowed" |
| D8 | Built-in roles | `admin` (full set incl. `users:admin`/`roles:admin`), `operator` (read+write on all modules, no users/roles/settings), `viewer` (read on all modules) | Covers 90% of scenarios; custom roles backstop the remaining 10% |
| D9 | Legacy migration | `role=admin` → admin; `role=user` → viewer; seed the built-in roles on first startup | Preserves current semantics (today's user has no write permissions anyway); relaxing is the admin manually promoting to operator |
| D10 | agent side | Zero changes | The agent has no user concept; RPCs are initiated by the server, so intercepting at the REST layer is equivalent to intercepting the RPC |
| D11 | websocket/terminal | Validate `terminal:write` when the connection is established; no re-check during the session | Revoking mid-terminal-session easily produces half-typed commands; reconnecting re-decides with the new permissions |
| D12 | Role management API | Only `roles:admin` (built-in admin): CRUD for custom roles; built-in roles are immutable and undeletable | Prevents self-lockout; custom role permission points must land within the D3 list (backend validation) |
| D13 | Self-protection | Cannot delete/demote the last valid admin; cannot modify your own role | The lesson of ordinary multi-tenant incidents — one validation saves one data-volume rescue |
| D14 | Audit | Role add/change, grant changes, and authorization denials (403) all enter the audit log | "Who elevated whom" must be traceable; 403 records help discover insufficient permission configuration |

## Permission Point List (D3 expanded)

> The complete permission point set takes `internal/storage/role.go` `resourceActions` as the single source of truth
> (the table below is an excerpt, including `services`/`smart`/`recordings`/`firewall`/`jobs` completed in P0/P1).

```
inventory:read/write    files:read/write       logs:read           terminal:write
docker:read/write       stack:read/write       cron:read/write     backup:read/write
acme:read/write/admin   dns:read/write         ddns:read/write     proxy:read/write
overlay:read/write/admin drift:read/write     nas:read/write     alerts:read/write
services:read/write     smart:read/write       recordings:read/write
firewall:read           jobs:read/write         workflows:read/write
approvals:read/write
audit:read              users:admin            roles:admin         settings:admin
```

Conventions: `<r>:write` implies `<r>:read`; `<r>:admin` implies write (e.g. acme's issuance/revocation/
CA-directory switching belong to `acme:admin`, while viewing the certificate list is `acme:read`).

## Phasing

| Phase | Content | Acceptance |
|----|------|------|
| P0 (backend) | Role table + built-in role seeding + `requirePermission` middleware + converging scattered handler checks + permission point list constants + D13/D14 validations | Every API has test coverage for the 200/403 matrix under the three built-in roles |
| P1 (frontend + management) | User/role management pages, `/api/me` returning permissions, route guards and menu pruning, the v-perm button directive | A viewer login sees no write-operation entry points at all |
| P2 (as needed) | Polishing the custom-role UI, permission templates, department-level grants (the Department field exists but does not participate in decisions for now) | Scheduled per actual demand |

## Implementation Status (P0 by commit)

| Commit | Content | Status |
|----|------|------|
| 1 | storage: the `Role` table (name primary key / permissions JSON / builtin) + permission point constants and built-in role definitions (admin/operator/viewer) + seeding (idempotent; built-in role permissions overwritten from code) + CRUD (custom role permission point whitelist validation; on delete, refuse built-ins, refuse referenced ones) + D9 legacy migration `role=user → viewer` | Done |
| 2 | server: the route permission table (path prefix × method → permission point, supporting AND semantics — domain-binding add/change/delete requires `dns:write`+`proxy:write`) + the `authorize` decision layer (after auth, before serveAPI dispatch; per-request DB query, missing role fails closed with 403) + implication rules (write⊇read, admin⊇write) + the three-role matrix tests. **Existing checks are not deleted** — authorize intercepts first, and the scattered `Role != "admin"` temporarily doubles as insurance | Done |
| 3 | Convergence: delete 10 scattered `Role != "admin"` occurrences (api.go ×7, api_proxy.go ×3); users/settings endpoints go through the permission table. Two additions on landing: creating/updating a user validates that the role name exists in the role table (a ghost role failing closed would lock the account; only `errors.Is(ErrNotFound)` gives 400, a DB fault passes through as 500); the default role for user creation changes user→viewer (after the D9 migration, user no longer exists). The condition exempting the old-password check on password change goes from the admin string to `userHasPerm(users:admin)` | Done |
| 4 | Role/user management APIs: `/api/roles` CRUD (only `roles:admin`) + D13 self-protection (the last valid admin cannot be deleted/demoted, cannot change your own role) + D14 403s into the audit. Three additions on landing: **"valid admin" = the role exists in the role table and covers `users:admin`** (ghost roles don't count); when D13's counting errors, refuse likewise (fail-safe; a DB fault makes no protective changes); an RBAC 403 short-circuits past the inner auth hook, so the audit layer performs optional authentication before the request (a valid Bearer is stuffed into ctx first) to guarantee recording "who" was denied — the inner auth's idempotent re-parsing is harmless | Done |
| P1 | web: user/role management pages, `/api/me` permissions, menu pruning (commits detailed in the table below) | Done (commits 5–8b, acceptance met: viewer has no write entry points anywhere) |

## Implementation Status (P1 by commit)

Acceptance criterion (the P1 row at design time): a viewer login sees no write-operation entry points at all.

| Commit | Content | Status |
|----|------|------|
| 5 | server: `/api/me` returns `permissions` (expanded by querying the role table; a ghost role → empty list, consistent with the decision layer's fail-closed) | Done |
| 6 | web permission infrastructure: UserContext pulls `/api/me` at startup and stores permissions (D7 semantics: dynamically pulled rather than frozen at login, so role changes take effect on refresh) + `usePerm`/`PermGuard` (the frontend replicates the write⊇read, admin⊇write implication rules) + permission-annotated menu pruning + route guards (no read permission → 403 page; backend RBAC remains authoritative, the guard is pure UX). Landing decisions: permissions are not stored in localStorage (re-pulled on refresh, avoiding stale decisions); unloaded/load-failure always fails closed with full pruning; menu-item perms align one-to-one with the GET semantics of the backend requiredPerms (monitor→inventory, disk→smart, domain-binding→dns:read); the settings menu is visible to everyone (incl. personal TOTP), Tab-level pruning in commit 8 | Done |
| 7 | web user/role management pages: the "Access Control" menu group (user management users:admin, role management roles:admin), user CRUD/change-role/change-password, role CRUD/permission-point matrix editing (built-in roles shown read-only). Landing decisions: the full set of valid permission points is derived from the built-in admin role's permissions (the seed is always the full set, zero backend changes); an inline Select in the user table changes the role (your own row disabled — D13 pre-blocked on the frontend); D13 backend-interception error messages surface via message | Done |
| 8a | web core ops module write-entry pruning: the write buttons and dangerous operations of DNS/Domains/Docker/Stacks/Backups/Proxy/Cron/Services wrapped in `PermGuard` (Docker container operations filtered by action — logs is a read). Landing decisions: Services follows the same pattern as Docker (top-level `usePerm` in the component + conditional rendering, the journal log button kept); the Cron enable switch uses a PermGuard disabled state as the backstop (a Switch without write permission shows disabled rather than disappearing, preserving state visibility); the DDNS/ServerBackup inspection/schedule config rows wrap only the save button (switch toggles are a local staging state; without a save entry nothing lands in the DB, coexisting with read-only display); Proxy's "Config" viewing is a read, kept | Done |
| 8b | web remaining module write-entry pruning: Acme (issue/deploy acme:admin)/Nas/Drift/Network/Disk/Recordings/Workbench (terminal:write remote-control entries, files:write file operations) + Settings page Tab-level (general/alerts settings:admin, the security Tab for everyone — personal TOTP). Landing decisions: the Acme issue/deploy buttons follow the backend's fixed acme:admin, while edit/delete/new/account-email are annotated separately as acme:write (a role with acme:write but not admin can manage configuration but cannot issue, aligned one-to-one with the backend's decisions); both the Workbench top ssh/rdp/vnc button group and the ConnectionPanel connect button, the dual entries, are wrapped in terminal:write; FileBrowser preview/download/search kept, while edit/permissions/rename/delete/new-directory/upload are conditionally rendered by canWrite; Settings filters the items array with usePerm (security/system info always present); the Network ZT authorize Switch uses a disabled-state backstop, and the leave/TS-authorize/TS-delete buttons are wrapped in PermGuard | Done |

Two implementation decisions (not fixed at design time, settled on landing):

- **Built-in role seed overwrite strategy**: an existing builtin role's permissions are overwritten from
  code (built-in roles are user-immutable — D12 — so version upgrades adjusting the built-in list must
  be able to take effect); custom roles are untouched. At steady state (roles already consistent, no
  legacy user rows) zero writes — a read-only DB reopen (the DB error-injection technique in server
  tests) does not trigger a write.
- **Custom role deletion**: refuse to delete while referenced by any user (a soft-reference deletion
  would fail-closed deny everything for those users, equivalent to locking their accounts).
- **Reachability of D13's user-side branch**: an operator reaching the user-management handler holds
  `users:admin` and is itself a valid admin; "delete/demote the last valid admin" excludes the target,
  leaving at least the operator — unreachable in production. The branch is retained as defense in depth
  for decision-layer changes (covered by direct test invocation). The role side (stripping a custom
  role's `users:admin`) is genuinely reachable: a `roles:admin` operator need not hold `users:admin`.

Implementation decisions added in commit 2:

- **Decision-layer shape**: a global `RBACMiddleware`, mounted inside AuditMiddleware (403s also enter
  the audit chain) and outside each auth hook, instead of wrapping requirePermission per handler. It
  parses the Bearer itself to get the role; a missing/bad token is passed through to the inner auth for
  a 401 (authentication precedes authorization). One mount point, and serveAPI plus the ten standalone
  registration points (docker/stacks/backups/probe/proxies/metrics/audit/remote/desktop/vnc) need zero
  changes; `/ws` (the agent channel, not a user request) is automatically exempt as it lacks the
  `/api/` prefix.
- **Sub-path merging**: probe/notification → `alerts` (probing config includes alert thresholds),
  metrics → `inventory`, proxies → `proxy`, server-backups → `backup`, remote/desktop/vnc
  → `terminal`.
- **Fixed actions**: audit/logs actions are all reads (export/search are also POST, always mapping to
  `:read`); terminal has only write (the GET list is likewise gated by `terminal:write`, D11
  semantics).
- **acme issue/deploy** (POST `.../issue`, `.../deploy`) is always `acme:admin`.
- **Unregistered /api/ paths pass through** (the handler's 404 backstop) — default-deny versus
  default-404 makes no security difference; a new endpoint forgotten in registration won't turn a 404
  into a 403 that reveals the path's existence.

## Things Not Being Done

- No casbin/ory-style permission framework — the dependency weight doesn't match this project's size
- No row-level permissions (a certain agent visible only to a certain department) — current deployments
  are personal/small-team self-hosted; revisit row-level isolation when a real need appears
- No agent protocol changes — permissions are the server's responsibility; the agent keeps the
  "execute on being called" trust model
