# Solution Design: Service Domain Binding


> 2026-09-19. First-phase design for service domain configuration. Original requirement: "It should be possible to configure a domain that points to a service, so that domains can be used conveniently in agent configurations." This document covers only design decisions and interface definitions.

## Current state and pain points

The fact that "a domain serves a service externally" is scattered across four modules that do not know about each other:

| Module | Current state | What it configures |
|------|------|----------|
| DNS management | the `dns` provider creates A/CNAME records | domain → server IP |
| Reverse proxy | traefik `ProxySite{ServerNames, Upstream}` | domain → host:port |
| Certificate | `cert` monitoring polls expiry times against a domain list | domain list (hand-maintained, duplicated with the two above) |
| Probe | `probe` targets are configured by URL | domain URL (also duplicated) |

| Pain point | Consequence |
|------|------|
| Standing up a new service requires configuring 3-4 places | Missing one place yields the classic three incident types: "resolution not created / certificate expired / probe not watching" |
| The domain-to-service relationship exists only in people's heads | Half a year later you no longer know which machine and which port blog.example.com points to |
| Agent-side configuration can only hand-copy domains | Changing/adding a domain means hand-editing each agent |

## Architecture overview

**DomainBinding as a first-class resource**: register "domain → target service on an agent", applied once, with three downstream systems (DNS / reverse proxy / certificate monitoring) automatically linked; the agent side references the domain list on demand.

```
┌─ server ────────────────────────────────────────┐
│ DomainBinding{domain, agent, target, auto*}     │
│   apply() ──┬─▶ dns provider: A/CNAME create/update │
│             ├─▶ agent RPC: dispatch traefik site  │
│             └─▶ cert monitor: auto-enroll          │
│ GET /api/agents/{id}/domains  ← agent pulls on demand │
└─────────────────────────────────────────────────┘
```

## Key decisions

| # | Decision | Choice | Rationale |
|---|------|------|------|
| D1 | Resource model | `DomainBinding{domain(unique), agentID, target(host:port \| docker://service-name), enabled, autoDNS, autoProxy, autoCert}` | The three auto switches decouple the linkage: registering only the relationship without enabling automation is also valid |
| D2 | target semantics | Unified as "agent-reachable address": host:port (host may be 127.0.0.1/container name/lan IP); `docker://name` resolves to a service name inside that agent's docker network | The reverse proxy upstream is in the agent's own machine network, and the domain resolution record points at the agent host IP; the two layers stay unconfused |
| D3 | DNS records | When `autoDNS` is on, write an A record via the dns provider (IP = the agent's registered primary address); CNAME/multi-level domains not in phase 1 | The agent's primary address already has a registration path, so no new config is added; wait for a real CNAME need |
| D4 | Proxy dispatch | When `autoProxy` is on, reuse the existing traefik `ApplySite` (ServerNames=[domain], Upstream=target); the server is a pure forwarder, and the agent renders + self-checks locally | Fully reuses the P1 reverse proxy channel, zero new protocol |
| D5 | Certificate linkage | When `autoCert` is on, the domain automatically enters the cert monitoring list; in reverse: when a Binding is deleted, prompt (do not auto-delete) the monitoring item | Monitoring would rather watch too much than miss something; auto-deleting monitoring is a dangerous default |
| D6 | Consistency check | The Binding list page shows "drift": actual DNS record ≠ expected, site not dispatched, monitoring missing, each item flagged | Registering does not mean it took effect; making drift visible is the first step toward self-service troubleshooting |
| D7 | agent reference | `GET /api/agents/{id}/domains` (including `/snippet` config snippet generation) delivers the domain list for that agent; probe targets support the `binding://` reference form, with resolution in the server's probe path | Changing a domain only means changing the Binding; as long as the reference key (agent+target) is stable, probing follows automatically |
| D8 | Deletion semantics | Deleting a Binding only removes the registration; it does not touch already-dispatched DNS/site/monitoring items (prompt to clean up manually) | Chained deletion automation is a dangerous operation; make it visible first, then clean up |
| D9 | Permissions | Create/update/delete require `dns:write` + `proxy:write` (whoever is linked needs their permission); read-only follows `dns:read` | Connects with the permission-point list in the RBAC design (rbac-design.md) |
| D10 | Storage | Reuse the existing settings/inventory SQLite, GORM single table `domain_bindings` | No new dependency; the scale (single digits to tens of rows) does not justify a separate store |

## API (server side, zero agent protocol changes)

```
GET    /api/domains?agent=xxx     list
GET    /api/domains/drift?agent=xxx  drift check (D6, live per-item check of the three-way linkage)
POST   /api/domains               register/update
DELETE /api/domains/{domain}      delete registration (D8)
POST   /api/domains/{domain}/apply   run the linkage (DNS/proxy/cert per the auto* switches)
GET    /api/agents/{id}/domains      domain list for that agent (D7)
GET    /api/agents/{id}/domains/snippet  config snippet (D7, text/plain)
```

### D7 agent reference, refined

The early draft said "read-only RPC `domains.list`" — the implementation changed this to an **HTTP endpoint**: the existing RPC protocol is server→agent one-way (the agent responds), and the source of truth for the domain list is the server's database, so the natural form of "delivery" is a server endpoint; the agent itself does not consume the list (the agent keeps zero protocol changes), and the consumers are people / scripts / config generation.

- **List**: `GET /api/agents/{id}/domains`, data source `ListDomainBindingsByAgent`; the agent must already be registered (404)
- **Snippet**: `GET /api/agents/{id}/domains/snippet` returns text/plain, containing two sections — an env line (`COCKPIT_AGENT_DOMAINS="…"`) and nginx `server_name` lines — taking only enabled bindings: curl it and you get it, and any service on the agent can source it
- **probe reference**: `Service.URL` supports the `binding://{agentID}/{target}` form (e.g. `binding://a1/127.0.0.1:8080`, `binding://a1/docker://web`). During probing, the probe Runner looks up the binding table and resolves the current domain by agent+target, then probes with `https://{domain}` — the reference key is stable, so after a binding changes domain (delete old, create new with the same agent+target) probing follows automatically, no more hand-copying. Available only for http/https types; reference with no match / binding disabled / type mismatch → the probe result is recorded as down with an error (visible, not silent)

### D6 drift check, refined

Drift = the gap between **the state promised by the auto\* switches** and the actual state; no promise (switch off) means no drift. Checked on demand in real time (not embedded in the list — each DNS check hits the provider API and each proxy check hits agent RPC, so embedding would drag the list out to the seconds scale); triggered manually during troubleshooting.

- Check scope: `enabled=false` skips the whole row; a path whose `autoDNS/autoProxy/autoCert` is off is not checked (`checked=false`), and a path that is on is flagged item by item
- DNS: expected = the agent's registered primary address (same source as apply); actual = the A record for that domain in the provider. Record missing → `missing`, content mismatch → `mismatch`; provider not configured / zone mismatch / agent has no address → `error` (indeterminable, not drift)
- Proxy: `site.get` looks up `bindingSiteName(domain)`; site absent → `missing`; present but upstream (after normalizing the docker:// prefix) or serverNames differs from the registration → `mismatch`; agent unreachable / RPC failure → `error`
- Cert: inventory Domain missing → `missing`, occupied by another agent → `foreign`; otherwise ok
- Three-state semantics: `ok` (no drift) / `missing|mismatch|foreign` (confirmed drift, with expected/actual) / `error` (check failed, indeterminable) — the frontend distinguishes red from yellow

## Phasing

| Phase | Content | Acceptance |
|----|------|------|
| P0 | Binding CRUD + apply with DNS/proxy/cert linkage + drift check | Registering one domain and completing apply is fully automatic, visible in the dashboard |
| P1 | agent `domains.list` RPC + probe targets referencing domains + one-click generation of install/config snippets | agent-side configuration no longer hand-copies domains |
| P2 (as needed) | CNAME/wildcard domains, batch apply, automatic removal of cert monitoring | Scheduled according to actual needs |

## Implementation status

- 2026-09-20: P0 core landed — storage single table + four server endpoints (list/register/delete/apply with the three-way linkage) + apply snapshot write-back (`LastApplyStatus/LastError/AppliedAt`). Implementation in `internal/storage/domain_binding.go`, `internal/server/api_domain_bindings.go`.
- 2026-09-20: D6 drift check landed — `GET /api/domains/drift` checks the three-way linkage live per item (semantics in the refinement section above). Re-binding a cert monitoring item uses an explicit `UpdateDomainAgentID` update — GORM `Assign(struct)` does not persist to an existing record, so `UpsertDomain` cannot carry the re-binding semantics (a pre-existing pitfall, recorded separately).
- 2026-09-20: D7 agent reference landed — `GET /api/agents/{id}/domains` (+ `/snippet` config snippet) and probe `binding://` reference resolution (refinement in the section above). Not included: the dashboard page (web UI separately), D9 permission points (follow the existing admin middleware).

## What we do not do

- Full DNS hosting management (zones/NS/SOA) — the dns provider already manages records, and Binding only manages that one entry from the "service perspective"
- No built-in DNS server — domain resolution always stays with the user's existing DNS provider
- No automatic trigger chain for certificate issuance (ACME issuance is still independently decided by the acme module) — Binding only guarantees "it is monitored"
