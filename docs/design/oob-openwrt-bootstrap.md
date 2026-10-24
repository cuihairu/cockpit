---
title: OOB Fourth Line — OpenWrt Bootstrap Node (Liveness + WOL + Fallback Forwarding + Self-Start Reliability)
---

# OpenWrt Bootstrap Node Design


## Status

- Status: **Active (design landed, implementation slices pending; P0, todo.md:81 OOB
  fourth line, adjudicated by the user 2026-10-20)**.
- Source: `docs/review/oob-management-2026-10-20.md` §4 (user ruling: a LAN OpenWrt
  box that self-starts is key — gateway + liveness probing + the last remote-control
  path, **must self-start reliably**).
- Reuses the existing `internal/openwrt` ubus RPC client (system info / interfaces /
  routes / firewall zones and redirect reads, client.go:245-497).

## 1. Three Layers (boundaries vs existing surfaces)

| Layer | Content | Boundary |
| --- | --- | --- |
| Gateway (existing) | Inbound port / port forwarding / egress IP observation | Already covered by overlay and read-only OpenWrt observation; untouched here |
| Liveness layer (new) | Independent ARP/ICMP/TCP probing of configured subnets: even if the target's agent is dead or the OS is down, network-level reachability and MAC-presence judgments still hold; cross-validates agent heartbeats | healthprobe (service-detection agent) answers "is the service reachable"; this layer answers "is the host at the network layer" — no semantic overlap, packets and verdicts are implemented independently |
| Last-resort control (new) | WOL wake-up, OOB-traffic forwarding (for AMT/DASH machines), IP-KVM/serial forwarding; keeps one control path inside the LAN when the server is unreachable | Forwarding writes only manage fallback-scenario redirects; **host firewall rule writes are out of scope** (adjudicated as shelved until read-only acceptance closes) |

"Self-start" requirement: after power loss or watchdog timeout the node returns to
service automatically (watchdog + auto-start, no manual login recovery).

## 2. Placement

- Liveness, WOL and orchestration run **agent-side** (cockpit-agent deployed on the
  OpenWrt node, matching the existing nightly openwrt artifact line): it is directly
  attached to the LAN; probing and actions never detour through the server. The server
  dispatches actions and collects observations over the existing agent RPC surface
  (the server never dials OpenWrt directly — same discipline as the BMC surface).
- The three probe sources fail independently: ARP (neighbor table / active resolve),
  ICMP, TCP (configurable port list). Verdicts are three-state "present / absent /
  unknown" instead of booleans (same tri-state discipline as OOB: no answer ≠ does not exist).

## 3. Action Surface

| Action | Implementation | Discipline |
| --- | --- | --- |
| WOL | Magic-packet builder (pure function) + UDP broadcast sender (injected conn; tests never send for real) | Audit event records target MAC/subnet; actions are idempotent |
| Fallback forwarding orchestration | ubus firewall redirect write extension (existing client is read-only; writes via uci-commit-equivalent ubus calls) | **Double confirmation + audit** (same as bmc-capabilities §5 power-class): server-side second confirmation, agent-side only executes name-bearing instructions; rules carry TTL / manual removal, never silently permanent |
| OOB traffic forwarding | Same (16992/16993 redirects for AMT/DASH machines are a redirect subset) | Same |

## 4. Credential Surface

OpenWrt ubus RPC requires login: username/password are configured as vault `${VAR}`
references, resolved at runtime and delivered from server to agent over the encrypted
WS (same as the SSH agent-password precedent), used in memory only — never on disk,
never in logs or audit plaintext (same as bmc-capabilities §5.1).

## 5. Self-Start Reliability

- The OpenWrt-side cockpit-agent ships a **procd init script** (/etc/init.d/) that
  starts at boot with explicit `respawn` watchdog parameters (threshold / retries /
  cooldown); power-loss recovery needs no manual login.
- Packaging wiring goes through packaging/openwrt and the nightly artifact line
  (init script ships with the artifact).

## 6. Slice Plan (small steps: implement + tests green + English commit)

| Slice | Content | Gate |
| --- | --- | --- |
| Slice A (**landed 2026-10-20**, hash in todo.md:81) | WOL action surface: magic-packet pure function + UDP sender injection point + agent RPC method + server forwarding endpoint. Landed in f9210ab: internal/oob/wol (magic packet / MAC parse / subnet broadcast / injectable sender), agent `oob.wol` RPC (explicit ip:port wins, else broadcast address derived from the first broadcast-capable non-loopback subnet), POST /api/oob/wol endpoint (RBAC under inventory write; audit records the target MAC and actual send target). Remaining: Slice B liveness layer. | Go gate ✅ (full short suite + block-coverage gate + full -race green) |
| Slice B (**landed 2026-10-20**, hash in todo.md:81) | Liveness layer: ARP/ICMP/TCP probes (injection points each) + scheduling + three-state verdict reporting 〔Probe action surface landed: internal/oob/liveness three independent sources with tri-state verdicts (present/unknown; absent reserved — no answer ≠ does not exist; neighbor-table read, ping and TCP dial injection points, 100% package coverage, tests send no real packets; the loopback ICMP call tolerates host differences) + agent `oob.probe` RPC (ip required; ports/timeout in seconds optional) + POST /api/oob/probe forwarding endpoint (oob capability gate, RBAC under inventory write, audit oob_probe records target IP and port list, verdicts not audited); **scheduling + push reporting landed in 6fd7a77**: internal/oob/liveness.Scheduler (cycles never overlap, per-cycle concurrency capped at DefaultWorkers=16, Run returns immediately on an empty target list, Prober/Ticks injection points, 100% package coverage) + agent flags `-liveness-targets/-liveness-ports/-liveness-interval` (comma-separated IPs / ports 1..65535 / seconds, value-by-value errors) + new message type `liveness_report` pushed upstream (deliberately not merged with probe_report: network-layer presence and service-layer presence answer different questions, and mixing the streams would let consumers misread presence semantics) + server in-memory latest-state upsert (agentID→ip, idempotent SeenAt refresh) and GET /api/oob/liveness (agent_id/ip filters, seen_at/age_seconds/stale freshness markers, stable order by arrival time then IP, not persisted, not audited as a pure read, RBAC under inventory read; capacity naturally bounded by the target count)〕 | Go gate ✅ (full short suite + block-coverage gate 88.04%/100% effective + full -race 63 packages) |
| Slice C (Go side **landed 2026-10-20**, hash in todo.md:81) | Fallback forwarding orchestration: ubus firewall write extension + double confirmation + audit [landed in d545a93+d96d727: internal/oob/redirect pure logic layer (three disciplines: the cockpit_ prefix marks ownership / mandatory TTL / expiry encoded into the rule name; the ubus write contract is unverified on a real box and isolated in the pure payload builders, so a contract fix touches one layer) + openwrt client add/del/list firewall redirect + agent oob.redirect/.remove/.list RPCs (credentials only flow through params, never into error text) + three server endpoints (double confirmation confirm_label/confirm_name must match verbatim; credentials resolved per request from vault oob/{name}, plaintext only over the encrypted WS and in memory; audit records the spec plus the agent-returned rule name and expiry; agent refusals audited too); **expired-rule sweep landed in ea8adde**: agent `oob.redirect.sweep` (removes only cockpit-owned rules whose encoded expiry has passed; aborts on the first removal failure) + POST /api/oob/redirect/sweep (confirm_oob double confirmation) + audit oob_redirect_sweep; remaining: the web double-confirmation UI] | Go gate ✅ (full short suite + block-coverage gate 88.16%/100% effective + full -race 64 packages); web confirmation UI pending |
| Slice D | Self-start: procd init script + respawn config + artifact wiring | Go gate + on-machine acceptance |

On-machine acceptance (once an OpenWrt box is available): self-start after power loss,
liveness still reports while every agent is down, WOL wakes the target, fallback
forwarding path is reachable; untested machines never enter the support matrix.

## 7. Boundaries (honest list)

- No mesh networking data plane (meshlet boundary, ADR-013); the node only supplies
  its own basic info read-only.
- No host firewall rule writes (separate item adjudicated as shelved).
- Liveness never claims "service health": only network-layer presence; the service
  surface belongs to healthprobe.
- Forwarding orchestration does not promise carrier-grade NAT quality (throughput
  belongs to the gateway itself); usable fallback within TTL is the bar.
