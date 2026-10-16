# DDNS (Dynamic Domain Resolution) Design

> Language: [中文](./ddns-design.md) ｜ **English**

## Background

Home broadband public IPs change periodically, and DDNS keeps "domain → home egress IP" in sync; it is the first dependency for accessing a NAS / home services from outside. Cockpit already has two reusable foundations:

- **DNS write path** (`docs/guide/dns-design.md`): the server calls the Cloudflare API v4 directly, and the `dns.Provider` interface (ListRecords/Create/Update/Delete) plus token management (config.yaml / env, secret never written to disk) are ready;
- **Inspection loop pattern** (validated over two rounds: drift → smart): ticker + Setting-based dynamic interval + genuinely deduplicated alerts.

The missing piece is "what is the home's current public IP" — the server is in the cloud, so the WebSocket peer address it sees is unreliable (see D2); IP probing must happen on the home-side agent.

## Decisions

- **D1 Architecture split**: the agent only answers "the egress IP of the network this machine is on" (new RPC `ddns.ip`); the server holds the DDNS record config, runs the inspection loop, and calls Cloudflare to write records. Rationale: the DDNS target IP is inside the observed network; DNS writes need a token, and the token lives only on the server (dns-design D2/D7).
- **D2 Do not use the connected peer address**: the `RemoteAddr` the server gets from the WebSocket is theoretically the home egress IP, but the agent may egress via a proxy/overlay, so that address's semantics are unreliable; the agent actively probing (outbound HTTP to an echo service) has clear semantics — "the egress IP this machine sees".
- **D3 IP probing with multi-source fallback**: echo services have uneven reachability in China, so try them in order and take the first success:
  - IPv4: `api.ipify.org` → `api-ipv4.ip.sb` → `ipv4.icanhazip.com` → `4.ipw.cn`
  - IPv6: `api6.ipify.org` → `api-ipv6.ip.sb` → `ipv6.icanhazip.com` → `6.ipw.cn`
  - Single-source timeout 5s, family total timeout 15s; the response, after trim, must pass `net.ParseIP` and match the address family to be accepted. The source list is a package-level variable, injectable in tests (fake sources via httptest).
- **D4 Capability `ddns` is always reported**: following the `file` capability precedent (stdlib outbound HTTP, no external dependency, no detector needed), appended unconditionally inside `detectCapabilities`; the provider offers the single method `ddns.ip`, returning `{ipv4?, ipv6?}` (on probe failure that family is omitted, no error — the DDNS inspection takes it per family). Whether it is used as an IP source for DDNS is decided by the server-side config binding an agent, and is unrelated to the capability.
- **D5 Config table `DDNSConfig`** (SQLite, replicating the BackupConfig pattern): `ID / AgentID / ZoneID / ZoneName / RecordName (e.g. home.example.com) / Type (A|AAAA) / Enabled / LastIP / LastUpdatedAt / CheckedAt / LastStatus (ok|failed|never) / LastError`. Five CRUD functions at the storage layer.
- **D6 Inspection loop**: `ddnsScanLoop` replicates the drift/smart pattern; Setting key `ddns.scan_interval_seconds`, default 300, min 60, max 86400, `0 = disabled`. Each round: enabled configs → agent online → `ddns.ip` for the corresponding family IP → one `ListRecords(zone, type)` per zone to find the record with a name match → Create if the record is missing → Update if the IP changed → write back to the table (LastIP/LastStatus/CheckedAt). Failing to get the agent IP only sets a failed state; DNS is untouched.
- **D7 API quota protection**: Cloudflare is written only when "the IP changed" or "the record is missing"; at most one ListRecords per zone per round; multiple configs for the same zone within a round share the query result.
- **D8 Alerts**: inspection failure (agent offline / IP not obtainable / Cloudflare error) → a genuinely deduplicated warning via `alert.Generator.CheckDDNS`, with the title by record name (isomorphic with CheckDiskHealth). Successful updates raise no alert (silent self-healing), and the change is visible in the table.
- **D9 Audit boundary**: user operations (creating/updating/deleting DDNS configs) are audited as `ddns_create / ddns_update / ddns_delete`; the automatic DNS changes of inspection are not audited — they are not user operations, and `DDNSConfig.LastIP/LastUpdatedAt` already fully records the change trail (same rule as drift/smart inspections not being audited).
- **D10 REST** (all under the `/api/ddns` prefix):
  - `GET /api/ddns` config list; `POST /api/ddns` create; `PUT /api/ddns/{id}` update; `DELETE /api/ddns/{id}` delete (user operation, audited)
  - `POST /api/ddns/{id}/check` immediately syncs and checks a single entry (without waiting for the inspection period), returning the check result
  - `GET/PUT /api/ddns/config` global inspection interval (isomorphic with `/api/drift/config` and `/api/smart/config`; 0 = disabled)
  - Create/update validation: Type ∈ {A, AAAA}, RecordName non-empty and validated by `dns.ValidateInput` under the same rules, AgentID non-empty. The zone dropdown reuses the existing `GET /api/dns/zones`.
- **D11 Frontend**: the DNS page changes to Tabs — "record management" (existing content) + "DDNS". DDNS tab: a config table (record name / type / bound host / current IP / status badge / last checked time) + a new/edit Modal (agent dropdown with online first, zone dropdown, record name, type, enable Switch) + a check-now button + an inspection interval setting (same pattern as the smart/drift pages).
- **D12 Security**: a probed IP must pass `net.ParseIP` + family match before entering the table or DNS; RecordName reuses the `dns.ValidateInput` validation surface; the token never leaves the server (a consequence of D1).

## Data structure

`ddns.ip` returns:

```json
{ "ipv4": "203.0.113.7", "ipv6": "2001:db8::1" }
```

`DDNSConfig` (storage table) corresponds one-to-one with the REST JSON; `LastStatus`:
- `never`: never checked (after creation)
- `ok`: the last check succeeded (including quiet rounds where the IP did not change)
- `failed`: the last check failed (LastError carries the reason)

## Implementation split (M1, this task)

1. **docs**: this document;
2. **agent**: `rpc/ddns_provider.go` (multi-source IP probing; Commander does not apply — this is outbound HTTP, so http.Client is injectable) + append the ddns capability in `detectCapabilities` + `providers.go` registration + tests (fake sources via httptest);
3. **storage**: `ddns.go` table CRUD + AutoMigrate + tests;
4. **server**: `api_ddns.go` (CRUD + check + audit) + `ddns_scan.go` (inspection loop) + `alert.CheckDDNS` + api.go dispatch + tests;
5. **web**: DNS page Tabs rework + DDNS tab + types + api method + build passing.

## Test points

- agent: multi-source ordered fallback (stop at first success), rejecting bad responses (non-IP / family mismatch), omitting a family when all sources fail, IPv4/IPv6 independent;
- storage: CRUD round trips;
- server: config validation (Type allowlist, empty-name rejection); the three inspection states (update on IP change / skip write when unchanged / failed on agent failure + deduplicated alert); the check endpoint; Create when the record is missing;
- frontend: tsc + vite build.
