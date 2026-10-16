# DNS Management Design (M1: Cloudflare Integration; M2: DNSPod / Aliyun; M3: Domain Ledger Write Linkage; M4: Batch Import/Export)

> Language: [中文](./dns-design.md) ｜ **English**

> Project initiated 2026-09-15. Corresponds to the todo item "DNS management: Cloudflare API integration,
> domain resource linkage for record create/delete/update". M2 initiated 2026-09-19: DNSPod / Aliyun providers.
> M3 initiated 2026-09-27: Domain ledger write linkage (the write linkage that D5 explicitly deferred "to M2"
> actually lands in M3). M4 initiated 2026-10-03: batch record import/export.

## Pain Points

Domains are a core CMDB resource (probes run by domain, certificates are managed by domain), but DNS records
themselves can only be changed in the Cloudflare console: adding an A record means opening a browser and logging
into the dashboard, and the "Resources → Domains" panel is just a read-only ledger. High-frequency operations in
the personal-cloud scenario (adding a subdomain pointing, switching IPs, adding TXT verifications) deserve to be
closed-loop within the panel.

## Architecture

The server connects directly to the Cloudflare API v4 (same precedent as probe detection and notification
channels, "server makes outbound calls to external services"), not through the Agent:

```
Web /dns ──REST──▶ server api_dns ──HTTPS──▶ api.cloudflare.com/v4
                        │
                        └─读 Domain 表做只读联动标注（M1 不回写）
```

## Decisions

| # | Decision | Content | Rationale / Notes |
|---|------|------|------------|
| D1 | Provider boundary | M1 implements Cloudflare only; `dns.Provider` interface (ListZones/ListRecords/SaveRecord/DeleteRecord) + implementation selected by config | The todo explicitly names Cloudflare; interface-first means later DNSPod/Aliyun only add implementations without touching upper layers |
| D2 | token configuration | config.yaml `dns.cloudflare.api_token`; env `CLOUDFLARE_API_TOKEN` takes priority as an override | The convention of secrets not living in yaml (env override); when unconfigured the API returns 503 + guidance text |
| D3 | Direct connection path | Server connects directly, http.Client timeout 15s, Bearer token header | Same as probe/notifications; the Agent does not participate (DNS is an external SaaS, not a host-local resource) |
| D4 | API surface | `GET /api/dns/zones` (with CMDB-registered annotation); `GET /api/dns/zones/{zid}/records?type=&page=`; `POST .../records`; `PUT .../records/{rid}`; `DELETE .../records/{rid}` | Same REST style as the resource management page; zone and record are both Cloudflare-side sources of truth, not persisted |
| D5 | Linkage | M1 read-only: the zone list annotates whether the domain is in the Domain table (in_cmdb field) | Write linkage (registering a Domain, provider auto-detection) deferred to M2; build the minimal closed loop first |
| D6 | Audit | Record create/update/delete are audited (action= dns_create/dns_update/dns_delete, resourceID= zone name + record name); list/detail queries are not | The existing discipline of auditing changes, not browsing; the token never enters the audit |
| D7 | token security | No API response or error message contains the token value; the config-probe endpoint returns only a "configured/not configured" boolean | Same discipline as notification-channel tokens |
| D8 | Record validation | type whitelist (A/AAAA/CNAME/MX/TXT/NS/SRV/CAA); name/content non-empty; ttl empty=1 (auto); same rules on both ends (server validation + Web form constraints) | Prevents Cloudflare 4xx noise; proxied is only valid for types Cloudflare supports (the server can simply ignore it) |
| D9 | Pagination | per_page=50 + page parameter passed through; M1 does not auto-fetch all pages | Record counts are usually < 50; Cloudflare result_info already carries total_pages for frontend paging |
| D10 | Web entry | New page `/dns` "DNS Management", menu placed after "Drift Detection" | An independent feature entry gets its own page; zone dropdown + record Table + create/edit Modal |

## Not Doing (Future Versions)

- ~~Other providers such as DNSPod / Aliyun DNS~~ (delivered in M2);
- ~~Write linkage with Domain resources~~ (delivered in M3);
- ~~Batch import/export of records~~ (delivered in M4);
- DNS change history;
- Agent-based distributed resolution checks (resolver effectiveness probing from various regions).

## M1 Checklist

- [x] internal/dns: Provider interface + Cloudflare client (zones/records CRUD,
      error unwrapping, pagination) + httptest mock unit tests
- [x] config: `dns.cloudflare.api_token` + env override + Normalize
- [x] server: `api_dns.go` full REST suite + validation on both ends + audit + serveAPI integration
- [x] web: `/dns` page (zone selection + records table + CRUD Modal + pagination) + route menu
- [x] tests: client all operations (mock Cloudflare), server forwarding and validation (503/
      400/502 paths), audit persistence
- [x] Documentation wrap-up (checking off this list) + todo.md synced

M1 complete (2026-09-15): dns package 13 tests + server 4 tests + config 1 test all green.
One implementation difference from D6: the audit resourceID uses `zoneID/record name` (create/update)
or `zoneID/record ID` (delete, when the record name is no longer obtainable), without an extra zones API
call to swap in the zone name — a low-frequency operation in the personal scenario, the ID is equally
traceable. Input normalization happens in the client before sending (type uppercased, TTL=0 → 1 auto,
leading/trailing whitespace stripped), Cloudflare always receives clean values.

## M2: DNSPod / Aliyun Providers (designed 2026-09-19)

### Pain Points and Scope

Mainland-China domain hosting is dominated by DNSPod (Tencent Cloud) and Aliyun DNS; with only Cloudflare in
M1, these two classes of domains still require opening the console to change records. M2 lands the first item
of M1's "not doing" list (other providers, interface already reserved): two new client implementations, with
the upper-layer REST/validation/audit/web pages fully reused. Write linkage (Domain table write-back), batch
import, and distributed resolution checks remain not done.

### Decisions

| # | Decision | Content | Rationale / Notes |
|---|------|------|------------|
| D11 | Unified provider | Reuses the global `dns.provider` introduced by ACME D13 (empty=cloudflare for backward compatibility / dnspod / alidns) and the same set of credentials (`dns.dnspod.login_token` / `dns.alidns.access_key`+`secret_key`, env takes priority) | From M2 on, the DNS management page and ACME issuance share the same provider configuration, **eliminating D13's semantic fork** — one provider key governs both places, avoiding the mismatch of "issuance uses dnspod while record management points at cloudflare" |
| D12 | DNSPod client | Hand-written form client connecting directly to `https://dnsapi.cn/{Action}` (POST urlencoded: common params login_token/format=json + per-action params); actions: Domain.List, Record.List (offset/length pagination), Record.Create/Modify/Remove; success determined by `status.code == "1"` | Structurally identical to cloudflare.go (hand-written net/http, testable with httptest); nrdcg/dnspod-go in the dependency tree is too old and its abstraction doesn't fit, not introduced; error messages contain only status.code/message, the token never appears |
| D13 | Aliyun client | Hand-written RPC V1 signature client connecting directly to `https://alidns.aliyuncs.com/` (GET query: common params AccessKeyId/SignatureMethod=HMAC-SHA1/Timestamp(UTC)/SignatureNonce + Action=DescribeDomains / DescribeDomainRecords / AddDomainRecord / UpdateDomainRecord / DeleteDomainRecord; signature = params sorted and percent-encoded then HMAC-SHA1(secret+"&"), special characters !'()* and space encoded per Aliyun rules) | The alidns-20150109 SDK is already in the dependency tree but was brought in by lego (indirect); its tea stack makes injecting an httptest endpoint hard; a hand-written ~40-line signature buys full-chain testability and structural parity across the three clients — worth it. If things get more complex later, switch to the SDK |
| D14 | Concept mapping | Zone.ID: neither provider has an independent zone id, **use the domain itself** (DNSPod Domain.name / Aliyun DomainName); Zone.Status normalized to active; record name: both use **subdomain semantics** (www / @ for the root), the client layer converts to/from the full name (`@` ↔ zone name); pagination: DNSPod offset/length and Aliyun PageNumber/PageSize=50 both normalized to RecordsPage{Page,TotalPage} (TotalCount computed as ceil) | The dto and upper layers are unaware of provider differences; side effect of zid being the domain: the zid in the REST path is literally the domain, if anything more straightforward |
| D15 | Absorbing record field differences | proxied is Cloudflare-only: non-CF providers ignore it on input, always output false; MX: the first word of content is split out as priority (DNSPod `mx=` param / Aliyun Priority), reassembled on read into the `10 mail.x.com` form; SRV: content's four segments (priority weight port target) are split and reassembled, DNSPod mx=priority/value=weight port target, Aliyun four segments as separate params; TTL: when DNSPod receives 1 (auto) the ttl param is omitted and the API takes its default (the free-tier floor of 600 is backstopped by an API error) | MX/SRV splitting/reassembly is a hard difference between the two APIs, kept inside the client + table-driven tests; the remaining type whitelist/validation rules are fully identical to M1 D8 |
| D16 | API surface/audit unchanged | REST shape, validation on both ends, audit actions and resourceID rules, 503 guidance semantics all reused; only requireDNS constructs the client per provider; `GET /api/dns/status` response extended to `{provider, configured}` (provider is echoed even when unconfigured) | The frontend guidance card lists configuration methods per provider (same field semantics as ACME page M2 D13, copy aligned across both pages) |
| D17 | web provider awareness | The /dns guidance card picks one of three (cloudflare→api_token, dnspod→"ID,Token" combined format, alidns→access_key/secret_key, each listing config keys and env); the proxied column of the records table and edit modal shows only when provider=cloudflare; zone dropdown unchanged (name is the ID) | Pure display branching, no interaction differences; the ACME guidance card already has three-provider copy to reference |

### Not Doing (M2 reconfirmed)

- Tencent Cloud/Aliyun **console-level** features (DNS resolution tiers, line-based load balancing, batch
  tricks) — only the panel closed loop of record CRUD;
- Smart-line (record_line) editing: DNSPod Create/Modify pins `record_line=默认`; existing non-default-line
  records, when edited, fall back to the default line — M2 states this explicitly in the docs, and the edit
  box does not expose line concepts;
- Write linkage (Domain table write-back), batch import/export, Agent-based distributed resolution checks —
  maintaining M1's "not doing".

### M2 Checklist

- [x] internal/dns: `dnspod.go` (form client + name/MX/SRV mapping + pagination)
- [x] internal/dns: `alidns.go` (RPC V1 signature + the same mapping set) + `sign.go` pure
      signature function
- [x] server: requireDNS constructs per provider (dnspod/alidns missing credentials each report
      their own 503 key); /api/dns/status echoes the provider
- [x] config: no new keys (reuses the ACME D13 structure), Normalize untouched
- [x] web: guidance card three-way choice + proxied column provider branching + tsc/build
- [x] tests: both clients httptest all operations (zones/records CRUD/pagination/MX·SRV split-reassemble/
      error codes), signature pure function table-driven, server provider branching and 503 copy, audit no regression
- [x] Documentation wrap-up (checking off this list) + todo.md synced

M2 complete (2026-09-19): dns package dnspod/alidns/sign three files + server branching +
web guidance card, dns package 21 tests + server 3 tests all green. Two implementation differences from
the design: ① DNSPod `Domain.List` pulls 400 entries in a single page without paging (D14 spoke of record
pagination normalization; zones use the same single-page strategy as M1 Cloudflare); ② Aliyun
`DescribeDomainRecords` has no exact type-filter parameter, `recordType` is filtered on the client side, and
`TotalPage` is still self-computed over the full set (an empty page may be reached; clicking the page number
is harmless).

**Live acceptance (remaining)**: DNSPod free-tier TTL floor and CAA support, Aliyun enterprise-tier
differences, hands-on SRV editing for both providers.

## M3: Domain Ledger Write Linkage (designed 2026-09-27)

### Pain Points and Scope

The relationship of M1/M2 to the Domain table is read-only annotation only (D5: `in_cmdb`): after adding an
A record for `app.example.com` in the panel, no such row appears in "Resources → Domains", and probes and
certificate management still don't cover it; the zone also has to be manually registered in inventory first.
M3 lands the write linkage that D5 explicitly deferred "to M2": domain asset changes on the DNS side are
synced into the Domain table, together with provider auto-detection and reverse reconciliation semantics.

### Decisions

| # | Decision | Content | Rationale / Notes |
|---|------|------|------------|
| D18 | Linkage scope and field mapping | **Zone-level explicit register/unregister** (`POST`/`DELETE /api/dns/zones/{zid}/cmdb`) + **automatic following after successful record CRUD** (only the three types A/AAAA/CNAME that "resolve to a domain"; MX/TXT/NS/SRV/CAA are auxiliary data, not domain assets, no linkage); ledger row fields taken from the nearest source: `Domain`=full name (already normalized on the provider side, client layer @↔zone name, D14), `Provider`=current `dns.provider`, `Status`="active", `Labels`={source:"dns", zone_id, zone, record_id, type} | None of the three providers has a zone create/delete API (DNS management only covers records), so "domain changes" in the panel are exactly the register/unregister action itself; the A/AAAA/CNAME criterion aligns with the existing convention (DDNS only writes A/AAAA, web proxied only those three types); labels make ledger rows traceable to the specific zone/record |
| D19 | Deterministic IDs and provider detection | Ledger row IDs: zone-level `dns-{zid}`, record-level `dns-{zid}-{rid}` (aligned with the acme-{id} precedent) — when a record is deleted/renamed, the row can be located without reverse lookup; the `Provider` field records the currently configured name; three-state (cloudflare/dnspod/alidns) detection of the linkage target is borne uniformly by zone lookup (ListZones matching zid), provider differences already absorbed in the client layer | Deterministic IDs make unlink idempotent (a missing row is not an error) and record updates upsert in place (changing name/type produces no new row); zid is the domain itself for dnspod/alidns (D14), making the ID more readable if anything |
| D20 | Best effort + declared-state protection + reverse reconciliation | **The provider is the source of truth**: the DNS operation must succeed first; a linkage failure is only logged, no rollback, no change to the API response (including audit: the following action is not separately recorded, the same action already leaves a dns_create/update/delete trace; the register/unregister explicit actions use new audit actions `dns_cmdb_register`/`dns_cmdb_unregister`, resource=domain); **declared-state protection**: when registering, if a same-named row already exists (inventory/manual/historical DNS origin) it is always 409, never silently rewriting someone else's row; when record-following hits a same-named row with a different ID → skip the linkage, log only; the unique index is the final line of defense; unregister deletes only rows with `labels.source=="dns"`, and **clears all DNS-origin rows of that zone at once (zone row + record-following rows)** — this is also the cleanup exit for orphan rows; **reverse reconciliation**: `GET /api/dns/zones` response gains `orphans` (DNS-origin rows whose zone is no longer in the provider list; attribution criterion: record-level rows look at `labels.zone`, zone-level rows at their own domain), listed only, **never auto-deleted** — inventory declarations or manual registrations may still be valid, the user cleans them up one by one in the panel | Residual rows after deleting a zone outside the panel / switching providers must not silently disappear (they may still be referenced by probes/certificates), but neither can we pretend they are valid — list them for the user to dispose of; when the zone has vanished from the provider, the unregister endpoint does not query the provider and can still clean up as usual; the orphan summary carries `zone_id` for targeted frontend cleanup |

### API Surface and Web

- `POST /api/dns/zones/{zid}/cmdb` → 200 `{registered,id}` / 404 (zone not with the
  provider) / 409 (same-named row already exists) / 502·503 (credentials and upstream, same semantics as
  existing);
- `DELETE /api/dns/zones/{zid}/cmdb` → 200 `{unregistered,records_removed}`
  / 404 (no DNS-origin rows for this zone) / 409 (zone row exists but its origin is not dns);
- `GET /api/dns/zones` response extended to `{data:[...], orphans:[{id,domain,zone_id}]}`;
- The record CRUD linkage is transparent to the web (on success the ledger row has already followed, no new
  interaction needed);
- web `/dns` records-management Tab: after a zone is selected, show "Register in ledger" or
  "Unregister" based on `in_cmdb` (Popconfirm explains that record-following rows will be cleaned up along
  with it), both governed by `dns:write`, failure copy (including 409) surfaces directly; when `orphans` is
  non-empty, an Alert above the table lists them one by one with a targeted "Clean up" button (historical
  rows without `zone_id` prompt the user to handle them in "Resources → Domains" manually).

### Not Doing (M3 confirmed)

- Mirroring DNS details such as record content/ttl into ledger rows (the Domain table only expresses "this
  domain exists and who manages it"; values defer to the provider);
- Auto-deleting orphan rows (see D20); merging strategy between inventory declarations and DNS-origin rows
  (keeping the first-come-first-served "same name 409/skip", no field-level merge);
- Record cascading (one-shot registration of all records under a zone).

### M3 Checklist

- [x] server: `api_dns_cmdb.go` (register/unregister + record following + orphan reconciliation) +
      api_dns route mounting and three linkage hooks
- [x] audit: `ResourceDomain` resource type + `dns_cmdb_register`/
      `dns_cmdb_unregister` actions
- [x] storage: no new methods (reusing UpsertDomain/GetDomainByName/GetDomain/
      DeleteDomain/ListDomains), plus unit tests for unique-index conflicts and GetDomainByName exact
      case-sensitive matching
- [x] dns package: zero changes (all linkage lives in the server layer, client normalization suffices)
- [x] web: api.ts register/unregister methods + getDNSZones returns `{zones,orphans}` +
      DNS page buttons/Alert + DDNSPanel adaptation
- [x] tests: server-layer register/unregister/three-provider detection/following/failure-doesn't-write/
      same-name-no-overwrite/orphan reconciliation and cleanup + error-branch injection (closed db, trigger
      blocking INSERT·DELETE); web layer 6 linkage cases for the DNS page
- [x] Documentation wrap-up (checking off this list) + todo.md synced

M3 complete (2026-09-27): api_dns_cmdb.go all functions 100% statement coverage (verified
against the TestDNS profile). Two implementation differences from the design: ① before registering,
`ListZones` verifies the zone exists (the provider is the sole source of truth, the panel doesn't register
out of thin air), so an unknown zid is 404 rather than going straight to the database; ② the ledger
follow read path uses `GetDomainByName` (unique-index exact match, case-sensitive — `Example.com` and
`example.com` are two rows), and orphan reconciliation uses a single-pass `ListDomains` scan (sharing the
provider list with the zones in_cmdb annotation, no extra queries).

**Live acceptance (remaining)**: under real Cloudflare/DNSPod/Aliyun accounts, the full chain of
register → ledger row appears → edit record → ledger follows → delete record → ledger row
disappears → unregister → whole zone cleaned up; plus the orphans prompt and cleanup after deleting a
zone outside the panel.

## M4: Batch Import/Export (designed 2026-10-03)

### Pain Points and Scope

Zone migration (switching provider / account), disaster-recovery retention, and bulk subdomain initialization
are all operations where "clicking one by one" is unbearable. M4 adds portable record-level export and
idempotent import: the export artifact is pure JSON (RecordInput shape), human-reviewable and portable across
providers; import classifies each entry into create/update/skip by a "match key", and re-importing is
idempotent.

### Decisions

| # | Decision | Content | Rationale / Notes |
|---|------|------|------------|
| D21 | API shape and reserved words | `GET /api/dns/zones/{zid}/records/export`, `POST /api/dns/zones/{zid}/records/import`; `export`/`import` are reserved words of the records sub-path, method mismatch → 405 | All three providers' record IDs are hex/numeric strings, no collision surface with the reserved words; rbac.go zero changes — the `/api/dns` prefix derivation naturally covers them (export GET = dns:read, import POST = dns:write), pinned by test assertions |
| D22 | Export | Aggregates all pages (page 1..total_pages, same client normalization as the single-page listing), response `{zone_id, provider, count, records}`; records are in **portable RecordInput shape** (type/name/content/ttl/proxied, no provider id/locked) | The export file can directly serve as import input (zone ownership is decided by the URL; zone_id/provider/count in the file are provenance info only); GET does not go through the audit middleware (browsing not audited since M1), following the content-audit precedent (recordings cast) it manually records `dns_export`, details contain only the count |
| D23 | Import match key and write decision | Match key = normalized name (strip whitespace, strip trailing dot, lowercase) + uppercased type + content exact after whitespace stripping; hit with no difference → skipped, hit with a difference → UpdateRecord (updated), no hit → CreateRecord (created). Difference determination: **explicit TTL (>=60) must be exactly equal, auto (0/1) means "provider decides" and does not trigger an update**; proxied compared only for cloudflare (on other providers the field is always false, D15). Duplicate entries within the same request: after a create/update succeeds, the match index is written back, duplicates become skips | auto not triggering updates means a CF export (ttl:1) migrated to DNSPod (reads back the default 600) won't repeatedly "update"; proxied provider awareness follows the same criterion as D15 |
| D24 | Fault tolerance and limits | Limit of 500 entries (exceeding → 400); **validate the whole batch first** (ValidateInput per entry, any invalid → 400, errors carry the index, at most 5 listed) before touching upstream; a single upstream failure **does not abort**, recorded into `failed:[{index,name,error}]`; response `{total,created,updated,skipped,failed}`; audit `dns_import` (resourceID={zid}/batch, details contain only counts, no record contents) | Whole-batch validation prevents half-done work (an invalid entry at #490 of 501 shouldn't first write 489 entries); continuing past single failures aligns with "import is a batch operation, one bad entry shouldn't waste the whole batch" |
| D25 | Linkage and permission reuse | The created/updated paths go through `linkDNSRecordDomain` (M3 D18-D20 semantics as-is: only A/AAAA/CNAME, failures only logged, no rollback); RBAC/error handling/503 guidance all reuse existing infrastructure | A batch import is equivalent to N single CRUD operations on the ledger side, introducing no second set of linkage logic |

### Not Doing (M4 confirmed)

- **Delete/replace mode**: import only adds and updates, never deletes — the blast radius of batch deletion
  is large (uploading an old file by mistake empties the zone); deletions go through single-record deletion
  or the provider console; "a record was deleted outside the panel and I want to sync" is handled by
  exporting and comparing manually;
- Async taskification: within the 500-entry limit it completes synchronously (upstream writes serially,
  worst case tens of seconds), no task polling;
- DNS change history (version diff/rollback) — requires persisting snapshots, separate project.

### M4 Checklist

- [x] server: api_dns.go route reserved-word dispatch + export/import handlers +
      `fetchAllDNSRecords` pagination aggregation (shared with import's current-state comparison)
- [x] audit: `dns_export`/`dns_import` actions (details contain only counts)
- [x] web: api.ts two methods + types; DNS page toolbar "Export" (Blob download of
      `<zone>-records.json`) and "Import" Modal (local .json read → parse
      preview → confirm → four-way result summary + failed details)
- [x] AuditLogs: ACTION_MAP/RESOURCE_MAP add 7 dns_* action labels and the
      dns_record resource label (the operation-type filter dropdown gains them simultaneously)
- [x] tests: server — export multi-page aggregation/upstream 502/unconfigured 503/method 405,
      import all four classifications/validation 400/over-limit 400/bad JSON/failure-continues/non-CF
      proxied branch/audit and ledger linkage assertions; RBAC requiredPerms pinned assertions;
      web — DNS page export download/import full flow/bad-file rejection/AuditLogs labels

## References

- Internal: [probe-enhance-design.md](./probe-enhance-design.md) (server outbound
  probing precedent), notification package (token configuration discipline)
- External: Cloudflare API v4 (zones / dns_records), DNSPod API (dnsapi.cn
  Domain.List / Record.*), Aliyun DNS RPC API (alidns.aliyuncs.com
  2015-01-09, V1 signature)
