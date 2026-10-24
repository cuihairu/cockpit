# Firewall Observation Design (iptables/nftables read-only)


## Background

The firewall is the last blank spot in the read-only security-plane observation. "Look over the firewall state once" is a high-frequency inspection action for a personal homelab console: whether the default policy has been tampered with, whether there are inadvertently opened ports, and whether the ingress ports Cockpit itself depends on are still there. Same family as SMART/NAS/Overlay — observe first, alert second, write operations deferred (see decision-log D-2026-10-07-3).

Cockpit's current state:

- The agent already has a mature per-agent observation skeleton that can be replicated wholesale: capability probing (`internal/agent/detector/`), a whitelist provider (`internal/agent/rpc/smart_provider.go` as the reference), and a pure-forwarding server endpoint (`internal/server/api_smart.go` as the reference);
- A full-repo grep finds no iptables/nftables write path — cockpit has never created or modified firewall rules; this design observes only.

This task adds the data plane for M1: **ruleset snapshot read-only observation + a read-only three-state Web page**. It does not include the inspection loop and alerts (to be evaluated in a later batch), and no write operations.

## Decisions

- **D1 Create a new firewall capability, do not attach it to an existing detector**. The existing capabilities (hardware-monitor/openwrt/overlay, etc.) carry no semantic slot for firewall tool detection; create `internal/agent/detector/firewall.go`: `LookPath` probes `nft` and `iptables`; if either is found, report the `firewall` capability with metadata carrying the tool list (e.g. `{"nft": true, "iptables": true}`). If both are absent (typical for Windows/bare containers) → do not report the capability, the page shows a "no firewall tools" empty state, no error (same rule as smart D1).
- **D2 Backend determination is empirical, do not guess the distro**. The provider runs `nft list ruleset` successfully → backend=`nftables`; fails or nft absent but `iptables-save` succeeds → backend=`iptables`. The `nf_tables`/`legacy` wording inside `iptables --version` is only shown as metadata and does not participate in selection.
- **D3 RPC single method `firewall.status`**, read-only, no parameters, zero validation surface (same style as smart D2/overlay).
- **D4 Data-plane commands and timeouts**:
  - nftables: `nft -j list ruleset` (JSON, gets everything in one shot);
  - iptables: `iptables-save` (IPv4, must succeed) + `ip6tables-save` (IPv6, best-effort — if the command is missing or fails, only append an `error` explanation and do not sink the whole packet; the IPv4 data is returned as usual);
  - Timeouts: 10s per command, 30s for the whole RPC (same order of magnitude as smart D3);
  - Output size cap 4MB; on exceeding the cap, set `truncated: true`, but the truncation semantics differ by backend: nft's JSON cannot be safely half-parsed → on exceeding the cap **do not parse**, return only meta + a truncation note; iptables-save is line-based text → truncate on line boundaries (dropping the trailing half-line), and the already-captured portion parses as usual. The page clearly states "ruleset too large, showing only the first 4MB";
  - Reading-command failure → `available=false` + the `error` reason, unlike SMART's per-disk granularity — a firewall is a single set, there is no "partial success" (the only exception being the IPv6 best-effort case above: v4 is the body, v6 is only a supplement).
- **D5 Field whitelist, no raw JSON passthrough**. nft `-j`'s complete object tree is large and full of counter noise; the provider parses it into a flat rule list: per rule `handle` (nft) / line number (iptables), `text` (a human-readable summary), `packets`/`bytes` (carried when present). iptables-save is split line by line into the `*table`/`:chain policy`/rule-line triple. Never stuff the whole raw output to the frontend (same rule as smart D4).
- **D6 Summary stats and default policy in their own column**. Aggregate rule counts by `family×table×chain`; the `policy` (accept/drop) of the base chains such as input/forward/output is promoted to a page badge — "is the default policy accept" is the first thing you see during inspection.
- **D7 Cockpit's own labeling: reserved recognition, no empty M1 functionality**. Cockpit currently creates no firewall rules (no write path in the whole repo), so the M1 page has nothing to label. The reserved mechanism: when cockpit introduces rules in the future, they uniformly carry a comment marker (iptables `-m comment --comment "cockpit:xxx"`, nft `comment "cockpit:xxx"`), and the provider recognizes the `cockpit:` prefix to set `ownedByCockpit=true`, highlighted by the frontend. The mechanism is written into this decision now; the code is enabled when the first real rule exists.
- **D8 Permission reality**: `nft list ruleset`/`iptables-save` require root (CAP_NET_ADMIN/net_admin). The cockpit agent runs by default as a root systemd service; non-root deployments → `available=false` + a permission-error explanation, with a page hint (same rule as smart D6). OpenWrt nodes usually run root by default and ship nft (fw4), so the firewall capability and openwrt capability coexist without conflict, and the firewall page works equally on OpenWrt.
- **D9 REST pure forwarding, no persistence**: `GET /api/agents/{id}/firewall/status` passes through the RPC result; browsing endpoints are not audited (same rule as smart D7). M1 has no global config (no inspection interval), so there is no `/api/firewall/config`.
- **D10 Permission point**: rbac `agentSubResources` adds `"firewall/": "firewall"` (isomorphic to `"smart/": "smart"`); `firewall` is a single permission tier (read-only in M1, no read/write split needed).
- **D11 Frontend `/firewall` page (read-only three states)**:
  1. Agent offline or no firewall capability → empty-state guidance ("this host has no nftables/iptables tools detected");
  2. `available=false` (tools present but reading failed, typically no permissions) → an explanation state (`error` reason + a non-root deployment hint);
  3. Data present → an overview bar (backend type, total rule count, input/forward/output default-policy badges) + a Collapse expanding rule details by family×table→chain (`text` + counters).
  Menu name "firewall", `SafetyOutlined`, placed after "disk health". An Alert at the top of the page when `truncated=true`.
- **D12 Things we do not touch**: observation only, no rule create/delete/modify path; cloud security groups (AWS SG, etc.) are out of scope; "write operations (rule add/delete)" are explicitly left to later batches and require real-device acceptance discipline (decision-log D-2026-10-07-3's alternative option of doing writes directly was rejected).

## Data structure

`firewall.status` returns:

```json
{
  "available": true,
  "backend": "nftables",
  "backendVersion": "nft 1.0.9",
  "iptablesVariant": "",
  "totalRules": 42,
  "tables": [
    {
      "family": "inet",
      "name": "filter",
      "chains": [
        {
          "name": "input",
          "policy": "accept",
          "rules": [
            {
              "handle": 12,
              "text": "tcp dport 9000 accept",
              "packets": 1234,
              "bytes": 567890,
              "ownedByCockpit": false
            }
          ]
        }
      ]
    }
  ],
  "truncated": false,
  "error": ""
}
```

- iptables backend: `backend="iptables"`, `iptablesVariant` is `nf_tables`/`legacy` (from `iptables --version`, display only); iptables-save output is split per table, `family` fixed to `ipv4`/`ipv6` (corresponding to iptables-save / ip6tables-save); rule `handle` is omitted, and `text` is the rule line itself (the part after `-A CHAIN`).
- When `available=false`, tables is empty and `error` carries the reason; when all tools are missing, `backend=""`. `error` may still be non-empty when `available=true` (best-effort notes for IPv6 read failure, nft over-cap meta-only notes) — `error` is supplementary information, not a failure indicator; `available` governs.
- `totalRules` is provided for the overview bar and for later alert batches to compare baselines.

## Implementation split (M1, this task)

1. **docs**: this document;
2. **agent**: `detector/firewall.go` (capability probing) + `rpc/firewall_parse.go` (pure-function parsing of nft JSON / iptables-save text) + `rpc/firewall_provider.go` (`FirewallProvider`, Commander injectable) + `providers.go` registration + tests;
3. **server**: `api_firewall.go` (status pure forwarding) + route registration + rbac prefix + tests;
4. **web**: types + api method + `/firewall` page + menu/route + `tsc`/build passing;
5. **mobile**: not in M1 (the firewall observation page is left for later batches, same pattern as the M3 pages).

## Test points

- Parser: nft JSON (with counters / without counters, comment-prefix recognition, illegal JSON does not panic); iptables-save (multiple tables, `:chain POLICY` lines, empty tables); over-cap truncation logic (nft meta-only / iptables line boundaries);
- Provider: fake Commander injection; nft missing falls into the iptables branch; both commands failing → `available=false`; `--version` parsing for legacy/nf_tables; IPv6 best-effort (failure omitted without sinking v4);
- Detector: capability/metadata assertions for the four combinations of nft/iptables each present or both missing;
- Server: offline 503, agent error 502 passthrough, rbac `firewall/` prefix mapping;
- Frontend: three-state rendering (no capability / available=false / normal data), truncated Alert, `tsc --noEmit` + vite build.
