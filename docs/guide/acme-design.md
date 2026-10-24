# ACME Automatic Certificate Issuance Design


## Background

Cockpit already has three reusable foundations, and certificate "issuance + storage + renewal" is the natural convergence point of the three:

- **DNS write path** (`docs/guide/dns-design.md`): the server calls the Cloudflare API directly; token management (config.yaml `dns.cloudflare.api_token` / env `CLOUDFLARE_API_TOKEN`) is ready — all the prerequisites for a DNS-01 challenge;
- **Inspection loop pattern** (validated over three rounds: drift → smart → ddns): ticker + Setting-based dynamic interval + genuinely deduplicated alerts;
- **Certificate observation** (`internal/cert` + `Certificate` table): probe periodically TLS-probes and writes back expiry/issuer, but there is **no issuance capability at all**; `Certificate` is a pure observation table.

The reverse proxy feature (proxy-design.md)'s "certificate issuance linkage" was explicitly marked as waiting for this capability; in the DDNS scenario (home broadband domains), once resolution is set up, the next step is an HTTPS certificate.

## Decisions

- **D1 Library choice: lego v4** (`github.com/go-acme/lego/v4`, MIT, actively maintained at v4.35.2): pure library embedding (`lego.NewClient` + `Certificate.Obtain`), the whole DNS-01 flow encapsulated (TXT publication, propagation waiting, authorization polling, certificate chain retrieval), 100+ DNS providers out of the box, account registration/EAB support. Rejected alternatives: `x/crypto/acme/autocert` — bound to `http.Server` TLS configuration scenarios, DNS-01 must be orchestrated yourself (amounting to half a custom development); `certmagic` — positioned for full TLS lifecycle of a managed web server (caching/OCSP/SNI routing), too heavy for a console "issue + store + display" scenario; `acmez` — too low-level, lego is precisely an encapsulation built on such primitives.

- **D2 DNS-01 challenge only**: HTTP-01 / TLS-ALPN-01 require 80/443 reachable from the public internet — home broadband inbound ports are often blocked (this project's core scenario), and only DNS-01 supports wildcards (`*.example.com`). DNS-01 needs only the Zone.DNS Edit permission, and works with the **same token** as DNS management/DDNS, no extra authorization needed.

- **D3 Architecture: server-side direct connection**: the same outbound path as DNS management/probe/DDNS; agents are not involved. Both issuance and renewal are server actions — certificates must be stored in the server's SQLite (see D5), and the Cloudflare token exists only on the server (dns-design D2/D7).

- **D4 DNS provider and CA directory**: M1 uses lego's built-in `providers/dns/cloudflare` (`NewDNSProviderConfig` takes the APIToken directly, reusing the same-source token from `s.dns`; when no token is configured, creation/issuance uniformly return 503 + guidance text, same as the DNS page pattern). CA directory in two tiers: `production` (Let's Encrypt production) and `staging` (fake certificates, testing without hitting rate limits), per-certificate optional, default staging (to prevent accidentally hitting production rate limits; switch to production after user confirmation). M2 extends DNSPod/Alibaba Cloud by adding a provider factory on the server side; the lego interface is a ready-made extension point.

- **D5 Data model (two SQLite tables)**:
  - `AcmeAccount` (single row, ID=1): `Email / PrivateKeyPEM (ECDSA P-256) / RegistrationURI / CADirectory / CreatedAt`. Lazily registered at first issuance (lego `Registration.Register`), with the account key persisted — so the same ACME account is reused across restarts (repeated registration would trip the account-count rate limit).
  - `AcmeCert`: `Domains (JSON array, the first is primary) / PrimaryDomain (indexed) / CADirectory / Status (pending|issued|failed) / CertificatePEM / IssuerPEM / PrivateKeyPEM / ExpiresAt / RenewBeforeDays (default 30) / AutoRenew / LastRenewAt / LastStatus (ok|failed|never) / LastError / CheckedAt` (observation fields replicate the DDNS style).
  - Certificates and private keys stored in SQLite: same lifetime as the panel data, covered automatically by server-backup (VACUUM INTO); file export is left to M2's deployment linkage on demand.
- **D6 Issuance flow**: `POST /acme/certs/{id}/issue` runs synchronously — validate token/account → lego `Obtain{Domains, Bundle:false}` (the certificate chain and issuer are stored separately) → write back `Status=issued / ExpiresAt (parse the leaf certificate's NotAfter) / LastStatus / LastRenewAt` → **link the observation table**: upsert `Certificate` (`DomainName=primary`, `Issuer`, `ExpiresAt`, `Status=valid`, `Labels["source"]="acme"`); probe's TLS probing continues to observe that domain independently, and the two layers complement each other. Synchronously waiting for DNS propagation is usually a few to a few tens of seconds, acceptable in M1 (frontend button loading); turning it into an async task (stack pattern) is left to M2's issuance + deployment linkage.

- **D7 Renewal inspection**: `acmeScanLoop` replicates the drift/smart/ddns pattern; Setting `acme.scan_interval_seconds`, default 3600, min 300, max 86400, `0 = disabled`. Scan scope is `AutoRenew` with `Status ∈ {issued, failed}`: for issued, judge `ExpiresAt - now < RenewBeforeDays * 24h` to renew early (a fresh Obtain, the new certificate overwriting the old PEM); for failed, retry after the D8 throttle (otherwise one failed certificate would be stuck forever); pending requires a manual first issuance by the user (the semantic expectation that creating ≠ issuing immediately). On failure, set `Status=failed + LastError` and raise a genuinely deduplicated warning via `alert.CheckACME` (title by primary domain, same structure as CheckDDNS).

- **D8 Rate-limit protection**: Let's Encrypt production limits (50 certificates per registered domain per week, 5 failed validations per hour per account/host name, 5 duplicate certificates per week). Three gates in M1: a) default staging (D4); b) issuance-failure throttling — certificates whose `LastRenewAt` is within 1h of now are skipped on inspection retry (manual issue is not throttled; an explicit user intent takes priority); c) the propagation pre-check is handled by lego's sequential DNS policy (querying authoritative NS, avoiding a blind fixed wait).

- **D9 Security and audit boundary**: the account key and certificate private keys **never enter** API responses/logs/audit; lists and details return metadata only; PEM content leaves only via the download endpoint: `GET /acme/certs/{id}/download?part=cert|issuer|key` — `key` is always audited (`acme_download_key`), `cert/issuer` are not. Audit: `acme_create / acme_update / acme_delete / acme_issue / acme_download_key`; inspection-driven automatic renewal is not audited (same rule as ddns D9, `LastRenewAt` already records the trail). Deleting a config only removes the local record and **does not revoke with the CA** (revocation requires the account key and semantically the certificate may already be deployed elsewhere).

- **D10 REST** (all under the `/api/acme` prefix):
  - `GET /api/acme/certs` list; `POST /api/acme/certs` create (`domains[]` each validated by `dns.ValidateInput` under the same rules, `ca` ∈ {staging, production}, `autoRenew`, `renewBeforeDays` 7~90); `PUT /api/acme/certs/{id}` update; `DELETE /api/acme/certs/{id}` delete
  - `POST /api/acme/certs/{id}/issue` issue/renew immediately (synchronous, returns a result summary)
  - `GET /api/acme/certs/{id}/download?part=cert|issuer|key` (text/plain attachment)
  - `GET/PUT /api/acme/account` (email settings; on PUT, if the account is already registered and the email changed, update the registration)
  - `GET/PUT /api/acme/config` global inspection interval (isomorphic with `/api/drift/config` etc.; 0 = disabled)

- **D11 Frontend**: new page `/acme` "certificate issuance" (navigation attached to the "resources" group, right after the "certificates" observation item), page structure following the DDNS tab pattern: a settings row (auto-renewal inspection switch + interval in minutes + a new button) + a config table (primary domain + wildcard marker / CA badge (staging in orange) / status badge (error tooltip on failure) / expiry time + days remaining / last issued time / actions (issue now, download dropdown, edit, delete)) + a new/edit Modal (domains Tag list editing, CA Select defaulting to staging with a "fake certificate for testing" note, auto-renewal Switch, renewal lead time in days) + email account settings (embedded in the onboarding flow at first creation). When no token is configured, a guidance card (same text as the DNS page, noting that ACME shares its token with DNS management).

- **D13 DNS provider extension: DNSPod / Alibaba Cloud (M2)**: lego providers are ready out of the box (verified in the v4.35.2 source: `dnspod.Config.LoginToken` (merged "ID,Token" format), `alidns.Config.APIKey/SecretKey`); only the provider factory and the config surface are missing on the server side.
  - **Configuration**: `dns.provider` ∈ {empty (=cloudflare, backwards compatible), dnspod, alidns}; credentials each go through the same dual channel — `dns.dnspod.login_token` (env `DNSPOD_LOGIN_TOKEN` takes priority), `dns.alidns.access_key` + `dns.alidns.secret_key` (env `ALIYUN_ACCESS_KEY` / `ALIYUN_ACCESS_KEY_SECRET` take priority, and the secret is not written to yaml by convention, same as cloudflare).
  - **issuer**: change `NewLegoIssuer`'s single-token closure to a `func() AcmeDNSConfig` config-snapshot closure (provider + all credentials); `Issue()` builds the corresponding lego provider from the snapshot, and an unknown provider / missing credentials each report their own key name (replacing Cloudflare's dedicated errAcmeNoToken message).
  - **Branching credentials-readiness semantics**: the DNS management page and DDNS remain Cloudflare-specific (`dns.status.configured` left as-is); the ACME perspective is independent — the `GET /api/acme/config` response gains `dns: {provider, configured}`, and the issue 503 pre-check plus inspection renewal judge by the ACME provider, with error text naming which key is missing.
  - **web**: the guidance card reads the acme config's `dns` field, and when unconfigured lists the configuration method for each of the three providers.
  - **Tests**: provider factory table-driven (all three ready / missing credentials / unknown); env overrides in config defaults; 503 pre-check dispatch by provider.
- **D12 Validation surface**: `domains[]` non-empty, each element passing the same domain validation as DNS record names (wildcards with a `*.` prefix allowed, validated after stripping it); `renewBeforeDays` ∈ [7, 90]; `ca` allowlist; primary domain = domains[0]. When no token is configured, create/issue return 503 (same 503 message convention as the DNS page).

- **D14 Post-issuance deployment push to the agent (M2, proxy site linkage)**: in proxy-design.md D7, an https site's `tlsCert/tlsKey` are absolute local paths on the agent, and M1 relies on "workbench - file upload" to move certificates by hand; certificates live in the server's SQLite, so after renewal the agent-side files do not self-update — the closed loop is to push the PEM to the agent after successful issuance, after which an nginx reload takes effect.
  - **Binding model (one-to-one; multi-target is now landed, see below)**: add `DeployAgentID / DeployCertPath / DeployKeyPath` to `AcmeCert` (empty = unbound) + `LastDeployAt` (Unix seconds, 0=never) + `LastDeployError` (empty = the most recent succeeded). Binding/unbinding goes through the existing `PUT /api/acme/certs/{id}` (`acmeCertInput` gains three optional fields; paths must be absolute, with the same rules on both sides).
  - **Channel**: reuse `file.write` (base64 single block + `truncate` overwrite; the 1MB block cap is generous for PEM). Private key on-disk permissions: add an optional `mode` parameter to `file.write`, allowlisted to `{0600, 0644}` (default 0644 behavior unchanged, backwards compatible) — certificate 0644, private key 0600. The only agent-side change.
  - **Path convention**: not enforced; web provides one-click fill-in of `/etc/cockpit/certs/<primary>.crt.pem` / `.key.pem`; the https form on the `/proxy` page hints at the convention, and nginx site paths reference it directly.
  - **Triggers**: (1) the success path of `runACMEIssue` triggers uniformly (shared by manual issue and inspection renewal); (2) manual immediate deployment `POST /api/acme/certs/{id}/deploy` (audit `acme_deploy`; 400 when unbound, 503 when the agent is offline). A push failure does not roll back the successful issuance state; `LastDeployError` is recorded (web red badge with tooltip), and no alert is raised — inspection scenarios avoid alert storms, and the undeployed state is visible on the page.
  - **server**: `deployAcmeCert(cert)` = validate binding → agent online → two `CallAgent file.write` calls (cert 0644 / key 0600, `truncate=true`) → write back `LastDeployAt/LastDeployError` → audit `acme_deploy` (details include the agentID and paths, no PEM).
  - **web**: add an "auto-deploy" section to the config Modal (online agent Select + two path inputs + one-click fill of the default paths); a "deploy" button in the actions column (available once bound); deployment status as a red badge + tooltip (time/error).
  - **Tests**: agent `file.write` mode allowlist (0600/0644 pass, others rejected, default unchanged); the deploy endpoint (400 unbound / 503 offline / success with two RPC round trips + audit); automatic push after `runACMEIssue` succeeds; PUT binding update.
  - **Multi-target deployment extension (implemented 2026-10-12, decision D-2026-10-12-3)**: the binding model is upgraded to one-to-many — new table `AcmeDeployTarget` (unique index on `(cert_id, agent_id)`, with `CertPath/KeyPath/LastDeployAt/LastDeployError`), and the five one-to-one columns above (`DeployAgentID/DeployCertPath/DeployKeyPath/LastDeployAt/LastDeployError`) are deprecated (columns retained only for rollback safety, no longer emitted in json), with a one-time idempotent backfill at migrate time (certificates with bindings get a target row of the same content and the old columns are cleared). The API input changes to `deployTargets[]` (≤8, agentId required, absolute paths, deduplicated per agent), and the view carries `deployTargets[]`; manual deployment endpoint semantics: 400 with no targets, 503 with no online target agent, **502 only when all targets fail** (partial success = 200, per-target results are in the target row's `LastDeployError`); `deployAcmeCert` does `file.write` per target (cert 0644 / key 0600, truncate) and writes back status per target; the audit `acme_deploy` details are per-target `{agentId, certPath, keyPath, ok, error}` (no PEM). Automatic deployment after a successful renewal walks all targets.


## Testing strategy

lego's full ACME flow requires a real CA (the official test setup uses the external Pebble process), so M1 unit tests do not run the real flow — extract "issuance" into an `AcmeIssuer` interface (`Issue(cert *storage.AcmeCert) (issued *IssuedResult{CertificatePEM, IssuerPEM, PrivateKeyPEM, ExpiresAt}, err error)`); the server depends on the interface, the production implementation wraps lego, and tests inject a fake (isomorphic with fakeDNSProvider):

- storage: CRUD round trips for the two tables;
- API: validation (empty domains / bad domain / CA allowlist / out-of-range renewBeforeDays rejected), create/update/delete audit, download key audited with a PEM response body, issue calls the issuer and writes back status, 503 when no token is configured;
- Inspection: early renewal / skipping when not near expiry / 1h throttle after failed / genuinely deduplicated failure alerts;
- The lego integration (real issuance) is left to real-environment acceptance (one run against the staging directory).

## Implementation split (M1, this task)

1. **docs**: this document;
2. **storage**: `acme.go` (AcmeAccount + AcmeCert CRUD) + AutoMigrate + tests;
3. **server**: `acme_issuer.go` (lego wrapper: lazy account registration, cloudflare provider construction, Obtain) + `api_acme.go` (REST + audit) + `acme_scan.go` (inspection) + `alert.CheckACME` + api.go dispatch + tests (fake issuer);
4. **web**: /acme page + route/navigation + types + api method + build passing.

## Acceptance checklist (real environment)

- [ ] Issuing a single-domain certificate against the staging directory succeeds, and a `source=acme` record appears in `/resources/certificates`
- [ ] Downloaded cert/key PEM content matches what was issued, and downloading the key produces an audit record
- [ ] Switch to production and issue a real certificate; the browser trust chain verifies
- [ ] Set `renewBeforeDays` to 90, wait for the inspection round to trigger automatic renewal, ExpiresAt updates
- [ ] Remove the token to trigger a failure; only one alert appears (genuine deduplication)
