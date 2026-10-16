# Disk Health (SMART) Design

> Language: [中文](./disk-health-design.md) ｜ **English**

## Background

For personal infrastructure, disks are the part where "found out too late" costs the most: a NAS user's first pain point is silent data-disk failure. SMART metrics (reallocated sectors, pending sectors, media errors) usually give a warning window of weeks to months before a disk actually dies; timely alerts buy a calm migration.

Cockpit's current state:

- The `hardware-monitor` capability (`internal/agent/detector/hardware.go`) **already probes smartctl** and sets `smart: true` in metadata, but there is no corresponding data-plane provider — probing means "the tool exists", yet readings cannot be obtained;
- The server-side inspection loop (`internal/server/drift_scan.go`) and alert deduplication (the `CheckXxx` family in `internal/alert/generator.go`) patterns are mature and can be replicated wholesale.

This task adds the data plane for M1: **single-disk health read-only observation + scheduled inspection + genuinely deduplicated alerts + a cross-host overview page**.

## Decisions

- **D1 Reuse the hardware-monitor capability, no new detector**. `metadata.smart` is already probed by `HardwareDetector` (`LookPath` + `smartctl --version` runs). The agent-side provider registers `case "hardware-monitor"` in `providers.go`; the server-side inspection filters on `metadata.smart == true` (hosts with only temperature/UPS under the same capability are skipped automatically).
- **D2 RPC single method `smart.status`**, read-only, no parameters, zero validation surface (same style as `overlay.status`).
- **D3 Two data-plane commands**:
  - `lsblk --json --paths --noheadings -b -o NAME,TYPE,SIZE,MODEL,SERIAL` discovers physical disks (filter `TYPE == "disk"`, excluding loop/ram/partitions); `--paths` makes NAME directly `/dev/sda`-shaped, no concatenation needed;
  - Per disk, `smartctl --json -H -A /dev/X` reads health and attributes.
  - Timeouts: lsblk 5s, smartctl per disk 10s, whole RPC 30s; a single-disk failure does not affect the rest (same rule as overlay D5).
- **D4 Field whitelist**. smartctl's JSON output is large; the provider parses and emits only the following fields, never passing through the full JSON:
  - `smart_status.passed` → the health verdict;
  - `temperature.current` → temperature (smartctl 7.x's unified top-level field for ATA/NVMe);
  - From `ata_smart_attributes.table[]`: the `raw.value` of id=5 (Reallocated_Sector_Ct), id=197 (Current_Pending_Sector), and id=9 (Power_On_Hours) (note that raw.value may be a number or a string depending on smartctl version; parsing must tolerate both shapes);
  - NVMe (`device.type == "nvme"`): `power_on_hours`, `media_errors`, `critical_warning`, `percentage_used` from `nvme_smart_health_information_log`.
- **D5 Health three states**: `passed` / `failed` (`smart_status.passed == false`, i.e. the disk already reports FAILING_NOW) / `unknown` (insufficient permissions, disk does not support SMART, smartctl read failure — the `error` field carries the reason for frontend display). `unknown` raises no alert (an environmental problem is not a disk problem).
- **D6 Permission reality**: reading disks with smartctl generally requires root. The cockpit agent runs by default as a root systemd service; with a non-root deployment all disks are marked `unknown` plus a permission-error explanation, and the feature does not crash. Documentation and the page state this.
- **D7 REST pure forwarding, no persistence**: `GET /api/agents/{id}/smart/status` passes through the RPC result; browsing is not audited (same rule as overlay D8/D9); inspection config uses the global path `GET/PUT /api/smart/config` (following `/api/drift/config`, since the inspection interval is not per-agent).
- **D8 Inspection loop replicates the drift pattern**: 1min ticker + lastScan comparison; Setting key `smart.scan_interval_seconds`, default 3600, min 300, max 86400, `0 = disabled`. Scope = `ListByCapability("hardware-monitor")` ∩ `metadata.smart` ∩ online.
- **D9 Genuinely deduplicated alerts**: `alert.Generator.CheckDiskHealth(agentID, hostname, issues)`, one alert per host (isomorphic with `CheckDriftScan`). Grading: any disk `failed` → `error` level; only reallocated/pending/media sectors > 0 → `warning` level; `unknown` is only logged.
- **D10 Frontend `/disk` "disk health" page**: a cross-agent overview on one screen (host, device, model, capacity, temperature, power-on hours, reallocated/pending/media sectors, health badge), with a Collapse expanding per-host detail; in-page inspection interval setting (InputNumber + save, data from `/api/smart/config`). Menu name "disk health", `HddOutlined`, placed after "network observation".

## Data structure

`smart.status` returns:

```json
{
  "available": true,
  "devices": [
    {
      "name": "/dev/sda",
      "model": "Samsung SSD 870",
      "serial": "S6PXNZ0R...",
      "sizeBytes": 500107862016,
      "health": "passed",
      "temperatureC": 34,
      "powerOnHours": 12345,
      "reallocatedSectors": 0,
      "pendingSectors": 0,
      "mediaErrors": 0,
      "percentUsed": 3,
      "error": ""
    }
  ]
}
```

- `available: false` means smartctl is missing (environments that contradict metadata.smart; both inspection and frontend skip it without error);
- Numeric fields are omitted (omitempty) when unavailable, and the frontend shows `—`;
- `serial` identifies the same disk across inspections (disk ordering can change) and is not confidential (single-admin product).

## Implementation split (M1, this task)

1. **docs**: this document;
2. **agent**: `rpc/smart_parse.go` (pure-function parsing of lsblk/smartctl JSON) + `rpc/smart_provider.go` (`SmartProvider`, Commander injectable) + `providers.go` registration + tests;
3. **server**: `api_smart.go` (status forwarding + config GET/PUT) + `smart_scan.go` (inspection loop) + new `CheckDiskHealth` in `alert` + tests;
4. **web**: types + api method + `/disk` page + menu/route + `tsc`/build passing.

## Test points

- Parser: ATA passed / failed, NVMe, `raw.value` as number and as string, missing `smart_status` (unknown), all fields missing without panic;
- Provider: fake Commander injection; empty/bad lsblk JSON; smartctl missing → `available: false`; single-disk smartctl failure → that disk unknown, the rest normal;
- Server: Setting boundary validation; inspection filtering (agents without the smart flag are skipped); alert injection through the real DB (following the drift inspection tests);
- Frontend: `tsc --noEmit` + vite build.
