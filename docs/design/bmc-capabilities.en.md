---
title: BMC/iBMC out-of-band management — Redfish primary, IPMI fallback (capability list + vendor quirk table + security red lines)
---

# BMC/iBMC Out-of-Band Management Design

> Language: [中文](./bmc-capabilities.md) ｜ **English**

## Status

- Status: **Active (user directive 2026-10-10: package common BMC/iBMS system capabilities, documentation first, zero code)**.
- Positioning: the server **out-of-band** management plane — the agent dials the BMC controller directly over the management network (independent of the host OS, still manageable while powered off or crashed), completing the server management plane by pairing "in-band agent + out-of-band BMC".
- Relationship with existing batches: probing is initiated agent-side (black-box framing, same as the service-detect agent); execlog records for destructive power operations depend on P5 execlog (**suspended pending a decision**) — the MVP lands on the existing audit plane first and gains it automatically once P5 lands.
- Zero code in this batch; implementation batches in §7.

## 1. Protocol layer: standards first

| | Redfish (primary) | IPMI 2.0 LAN (fallback) |
| --- | --- | --- |
| Standard | DMTF Redfish (REST+JSON over HTTPS) | IPMI 2.0 RMCP+ (UDP 623) |
| Coverage | Mainstream servers after 2015 (iDRAC 8/9/10, iLO 4/5/6, iBMC V3+…) | Older machines, or BMCs with Redfish disabled |
| Auth | Basic or Session (`/SessionService/Sessions`, standard) | RMCP+ username/password (cipher suite negotiation, **cipher 0 forbidden**) |
| Transport | HTTP/TLS reuses existing infrastructure | UDP with self-managed sessions (wrapped by the library) |

**Dual-stack strategy**: per-target `protocol: redfish|ipmi` (explicit means no probing); the default `auto` — first GET `/redfish/v1` service root (2s timeout), if 200 with JSON then Redfish, otherwise fall back to IPMI. Both protocols expose the same set of capability interfaces (§2); the differences are confined inside the provider (§4).

## 2. Capability list (ordered by common use)

| # | Capability | Redfish standard | IPMI fallback | Batch |
| --- | --- | --- | --- | --- |
| C1 | Power state | `ComputerSystem.PowerState` (On/Off/PoweringOn…) | `chassis status` | B2 |
| C2 | Power control: on/off/graceful restart/forced restart | `Actions/#ComputerSystem.Reset`, `ResetType` ∈ On/ForceOff/GracefulRestart/ForceRestart | `chassis power on\|off\|soft\|cycle` | B4 (destructive) |
| C3 | Sensors: temperature/fan/voltage/power | `Thermal`/`Power` under `Chassis` (older) or `ThermalSubsystem`/`PowerSubsystem` (Redfish 1.9+) | SDR + streaming sensor readings | B3 |
| C4 | Hardware health rollup | `Status.Health` (System/Memory/Processor/Chassis) | Aggregation of sensor threshold states | B3 |
| C5 | SEL event log (query/filter by time/clear) | `Managers/{id}/LogServices/SEL/Entries` | `sel list/clear` | B5 (clear is destructive) |
| C6 | BMC/firmware/serial number info | `Manager`, `SoftwareInventory`, `SerialNumber/SKU` | `mc info`/device id | B2 |
| C7 | SOL serial console | Mostly OEM (`SerialInterface` rarely implemented) | **IPMI SOL** (activate/deactivate stream) | B6 |
| C8 | KVM entry/virtual media | **Link only, no implementation**: build the HTTPS URL per vendor rules and hand it to the frontend for redirect | — | B7 (link) |

The read-only surface (C1/C3/C4/C6) comes first; the destructive surface (C2, C5 clear) is deferred and follows the same §5 red lines.

## 3. Initiating side and placement

- **Agent-side provider** (same precedent as nginx/traefik/stacks): the agent reaches the BMC over the management network; the server never connects directly. The agent holds a BMC target list (address/credential reference/protocol), the capabilities `bmc-redfish`/`bmc-ipmi` enter the registration metadata, and the server forwards `bmc.*` RPC methods (server REST `/api/agents/{id}/bmc/*` is pure forwarding with no persistence, same as the proxy precedent — the agent side is the source of truth).
- The provider interface is the same set as generic Redfish (the capability key is the method surface): `bmc.power.status / bmc.power.set / bmc.sensors / bmc.health / bmc.sel.list / bmc.sel.clear / bmc.sol.open / bmc.info / bmc.console.link`.
- Three-layer discipline: protocol client adaptation (gofish/go-ipmi wrappers) goes into core (e.g. `core/bmc`, pure client, zero business logic); provider business orchestration goes into `internal/agent` (the plugin slot); the quirk table is data, not code (see §4).

## 4. Vendor quirk table (same capability, different property paths)

At runtime the quirk is selected automatically from the vendor identifier in the service root `Oem` section (`Oem.Hpe`/`Oem.Dell`/`Oem.Huawei`); unknown vendors take the pure-standard path (empty quirk), and a capability with no quirk reports "unsupported" rather than guessing.

**The quirk layer is not developed in-house**: the top-level wrapper bmclib/v2 ships built-in multi-vendor drivers (auto-fallback by probing the model, §6), so quirk placement **reuses its drivers first**; the table below is a capability mapping reference plus a self-supplemented fallback for what bmclib does not cover.

| Capability | Redfish standard (DMTF) | Dell iDRAC 9/10 | HPE iLO 5/6 | Huawei iBMC |
| --- | --- | --- | --- | --- |
| Temperature/fan | `Chassis/{id}/Thermal` (older) · `ThermalSubsystem/ThermalMetrics` (1.9+) | Standard Thermal works; precise whole-chassis power via OEM `/Oem/Dell/DellPower...` | OEM extended fan/temperature under `/Oem/Hpe` (some Thermal fields missing) | Standard Thermal works |
| Power | `Chassis/{id}/Power` or `PowerSubsystem/PowerMetrics` | OEM PowerMonitoring (watt-level precision) | `/Oem/Hpe` power section | Standard Power + OEM accelerated value |
| SEL | `LogServices/SEL/Entries` | SEL and **LC log** (`LogServices/Lclog`, more complete) coexist | IEL (Integrated Event Log, i.e. the standard LogService under a different name) | Standard SEL |
| Firmware inventory | `UpdateService/SoftwareInventory` (`FirmwareInventory` is obsolete) | OEM FirmwareInventory still hangs under UpdateService | OEM `/Oem/Hpe/UpdateService` | Standard SoftwareInventory |
| KVM link (C8 link only) | No standard | `https://{bmc}/console` (HTML5) | `https://{bmc}/html5` (iLO5) / `/console` (iLO6) | `https://{bmc}/#ovm` (HTML5) |
| SOL | Mostly OEM | IPMI SOL | IPMI SOL (VirtualSerialPort OEM as an alternative) | IPMI SOL |
| Session auth | `/SessionService/Sessions` standard, logout via `X-Auth-Token` | Standard | Standard | Standard |

Quirk implementation shape: **one override record per capability** (URI template / field name / result projection function key); the provider looks up the table by (capability key, vendor) and falls back to the standard path — the quirk table is data-driven, no per-vendor if/else chain in code.

## 5. Security red lines

1. **No plaintext credentials**: BMC credentials live in the server credential store (reusing the AES-GCM model of the remote credential vault, `protocol` extended with a `bmc` type); at each operation they are delivered to the agent over the encrypted WS channel (same precedent as SSH proxy passwords/keys), used in memory and discarded immediately, never written to disk and never appearing in plaintext in logs or audit details.
2. **TLS verification on by default**: Redfish certificate verification is on by default; per-target `skip_verify: true` is allowed only for intranet self-signed certificates (a red warning at the web config surface); IPMI mandates RMCP+ (cipher suite ≥1, plaintext cipher 0 forbidden).
3. **Double confirmation + audit for destructive operations**: C2 power-class and C5 clear — a second web confirmation (type the target name to confirm, same precedent as stack deletion) + audit events `bmc.power.{on,off,restart}` / `bmc.sel.clear` (recording agent/target/action/actor, no credentials) + execlog trail (depends on P5, audit first for the MVP).
4. Minimal IPMI account privileges (documentation recommendation: the probe-only role is Operator only; Admin is needed just for power operations).

## 6. Dependency selection (research settled 2026-10-10: reference mature libraries directly, no in-house development)

| Layer | Selection | Rationale |
| --- | --- | --- |
| **Top-level wrapper** | **`github.com/bmc-toolbox/bmclib/v2`** | Multi-vendor driver abstraction (Redfish/IPMI/SSH/vendor APIs), auto-fallback by probing the model — **the vendor quirk layer is built in** (§4 placement reuses it first, self-supplemented table only for gaps); the power/boot device/inventory/SEL/firmware capability surfaces are ready-made |
| Redfish protocol client | **`github.com/stmcginnis/gofish`** (BSD-3) | The DMTF standard, 45+ downstream consumers; the same client bmclib sits on — use it for direct mode (OEM paths with no bmclib driver) |
| Bare IPMI (when needed) | **`github.com/bougou/go-ipmi`** | Pure Go, no ipmitool binary dependency; remote (RMCP+) / in-band dual mode, ships a reference BMC server for CI stubs; full SOL/SEL/sensor coverage. Use it directly for SOL (C7) that bmclib does not cover |

**Note**: bmclib's **SSH driver is disabled by default and enabled on demand** — it consumes host SSH credentials (not BMC credentials), a separate credential surface, so it stays off by default; enable it per target when a vendor SSH API is genuinely needed. The thin wrapper layer = the provider plugin slot + the credential/TLS/audit red lines (§5); quirks and multi-vendor fallback are not developed in-house and upgrades follow upstream. (Archived alternatives: u-root ipmi targets in-band OpenIPMI drivers and is unsuitable for remote LAN; shelling out to ipmitool introduces an external binary dependency — both rejected.)

## 7. Batch plan (small steps: implement + green tests + English commit)

| Batch | Content | Gate |
| --- | --- | --- |
| BMC-B1 | This brief | docs |
| BMC-B2 | Provider skeleton + capability detection + C1/C6 read-only (introduce gofish/go-ipmi + reference stub tests) | Go gate |
| BMC-B3 | C3 sensors + C4 health (table-driven standard + quirk paths) | Go gate |
| BMC-B4 | C2 power control + double confirmation + audit + credential store delivery | Go gate + web double confirmation |
| BMC-B5 | C5 SEL query/clear | Go gate |
| BMC-B6 | C7 SOL (bridging the IPMI stream to the web terminal) | Go gate + manual acceptance |
| BMC-B7 | C8 KVM/virtual media link | Go gate |

## 8. Boundaries (honest list)

- No vendor lock-in; unknown OEM takes the pure-standard path, and a capability with no quirk explicitly reports "unsupported".
- Virtual media/KVM is link only with no implementation; firmware upgrade (pushing firmware packages) is **out of scope** — high risk, discussed separately.
- The "server connects to the BMC directly" shape is not built (initiated agent-side, management-network reachability is guaranteed by deployment); multi-probe placement follows the croupier framing, discussed separately.
- The SOL/SEL functional surface on old IPMI BMCs is only as good as go-ipmi's real measured coverage (a reference stub ≠ a real machine); real-machine acceptance is a separate batch.
- execlog depends on P5 (suspended pending a decision) — the MVP audit plane lands first, and power operations automatically gain execution trails once P5 lands.
- Semantics do not overlap with the service-detect agent (black-box liveness): that plane owns "is the service reachable", this plane owns "hardware and power".