---
title: agent-core architecture refactor — the core/platform/plugin three layers and the build matrix (proposal pending approval)
---

# agent-core Architecture Refactor Brief

> Language: [中文](./agent-core-architecture.md) ｜ **English**

## Status

- Status: **P1–P5 landed (all ten `core/` packages exist and are wired up; re-verified 2026-10-15).** Implementation progress is authoritative per the annotations in the §7 batch table.
- Proposal framing (user directive, 2026-10-09):
  1. **Core extraction**: move generic capabilities into core (register/heartbeat/logging+metrics/config hot-reload/crash slot/retry/reporting/execution audit execlog/liveness in the system role). Core holds to hundred-line files, capabilities are prunable, and business code only writes plugins;
  2. **Platform differences layered**: platform-specific features (process supervision, paths, service integration on linux/windows/macos) go into a dedicated directory for each OS (the three layers core / platform adaptation / business plugins);
  3. **Goal**: the same core+plugins compile into platform-specialised agent artifacts (the CI matrix builds per platform).
- Rhythm benchmark: croupier `docs/design/agent-core-design.md` (core is a parts bin not a framework, prunable capabilities, the hundred-line discipline, batched extraction, same rollback strategy).
- Source material (authoritative on current state): [Architecture and boundaries](../guide/architecture.md) · [Windows Agent packaging design](../guide/windows-agent-design.md) (SCM service state finalized) · [OpenWrt Agent packaging design](../guide/openwrt-agent-design.md) (ipk/procd landed) · [Service health probe and self-healing design](../guide/service-health-design.md) (the ready-made seed of healthprobe).

## 1. Positioning: the three-layer architecture

**Core is not a framework, it is a parts bin.** No god-object assembly (the generic loops of register/heartbeat/report/execution-trail/metrics have already been split out this way — `internal/agent/agent.go` is now 696 lines); each capability is an independent small package that main imports as needed — capabilities that aren't imported do not participate in the build, which makes them "prunable on demand" for free.

```text
core/                 ① Generic capability layer: hundred-line files, zero business logic, zero platform branches
core/platform/        ② Platform adaptation layer: the only zone allowed to hold GOOS branches (process supervision/paths/service integration)
internal/agent/       ③ Plugin host + business plugins: Provider registry, capability detection, rpc Providers
```

Dependency direction (one-way, no backflow):

```text
cmd/cockpit-agent → internal/agent(plugins) → core/* → internal/protocol
                                    core/platform/* ← no other core package may import it
```

Three invariants:

1. **One protocol**: the single source of truth for the Server↔Agent wire types remains `internal/protocol`. Core packages may import it — this is the **only whitelisted exception** to the "core does not import internal/" principle (the protocol is shared by both sides, not an internal private good); if core ever needs to be fully independent, protocol is promoted to top-level in a separate batch.
2. **Platform branches in one place**: GOOS build tags and runtime platform detection may only appear in `core/platform/`. The scattered legacy platform files (`machineid_*`, `rpc/service_windows_scm.go`, `rpc/job_provider_unix.go`, etc.) are collected in batch P4; before collection they are registered in the §8 whitelist, and all new platform code goes straight into the platform layer.
3. **No library re-litigation**: gopsutil v3 (not upgraded to v4, revisit per batch), gorilla/websocket, protocol.Codec serialization — core only consolidates usage, it does not swap libraries.

## 2. Capability list table (each: existing / partial / new / current source)

| #  | Capability                     | Status         | Source and current state                                                                                                                     | core package (planned)        |
| -- | ------------------------ | ------------ | ------------------------------------------------------------------------------------------------------------------------------ | -------------------- |
| 1  | Registration and going live                 | **Existing**     | agent.go register first-packet chain + secret validation; the ID derives from the machine id (`machineid_{linux,darwin,windows,other}.go`)                    | `core/register`      |
| 2  | Heartbeat and liveness               | **Existing**     | 30s heartbeat + startedAt uptime + piggybacking on the open-services surface (servicesOnce background re-probe)                                                       | `core/heartbeat`     |
| 3  | Retry backoff                 | **Existing, hand-written** | the agent.go reconnect timer / concurrent-reconnect guard; scattered retry patterns inside the rpc providers — hoisted into one unified knob to prevent a fourth set                                        | `core/backoff`       |
| 4  | Structured logging               | **New (selection)** | currently stdlib `log.Printf`; logx settles on **slog** (stdlib, zero third-party deps), legacy call sites are not force-migrated — new code uses logx, migrating progressively per batch                     | `core/logx`          |
| 5  | Metrics collection                 | **Existing**     | collector.go: gopsutil v3 CPU/memory/disk/load/host, already 155 lines in parts shape, relocates in place                                                  | `core/metrics`       |
| 6  | Unified uplink                 | **Existing**     | protocol.Codec + outbound channel; metrics/services/registration/heartbeat all go through it — hoisted into a thin codec+channel wrapper                                      | `core/report`        |
| 7  | Config fetch and hot-reload           | **New**     | currently only startup flags (docs/agent-config.md), no fetch/hot-reload; generalise "version poll → fetch → hot-apply" (same framing as the server-side inventory.watch precedent) | `core/configsync`    |
| 8  | Execution audit (execlog)      | **Existing**     | `core/execlog`: ring + batched uplink (`execRecorder` + 30s batching), a single instrumentation point in the router handler; protocol `exec_log` message type registered (P5 landed)                     | `core/execlog`       |
| 9  | Liveness probe (system role)      | **Existing (landed in P6)** | `core/healthprobe`: Checker (HTTP/TCP) + Guard three-semantics (Liveness/Readiness/Heartbeat) debounced state machine + failure-window timeline; the service_health policy layer already switched (heal→liveness, alert-only→readiness, events on transition edges, windows uplink with state); Heartbeat semantics and "probe records modelled by the monitored system" wait for the system-role batch | `core/healthprobe`   |
| 10 | Crash collection                 | **Slot reserved**     | package skeleton + interface placeholders only, no implementation (the tiered scheme will be decided per research, following the croupier crash-capture-survey rhythm)                                       | `core/crash`         |
| 11 | Process supervision/service integration/paths    | **Existing, scattered per platform** | Windows SCM (service_windows.go 401 lines + svcargs), darwin launchd (207 lines), openwrt procd (ipk/init.d), linux systemd (install.sh) | `core/platform/<goos>` (§3) |

> The hoisting boundary (the first application of prunability): the 20+ Providers in `internal/agent/rpc/` **all stay on the plugin surface and do not enter core** — they are cockpit's business surface; any future small core-based agent naturally has neither these attack surfaces nor this size.

## 3. Platform adaptation layer (core/platform/)

The platform layer exposes exactly one contract; plugins and main only know the interface, never the platform:

```go
// core/platform/host.go (contract sketch, under a hundred lines)
type Host interface {
    Paths() Paths          // config/log/data dirs (linux /etc/cockpit,~/.config; windows ProgramData; darwin ~/Library)
    MachineID() string     // machine identifier derivation (collects machineid_*.go)
    Signals() []os.Signal  // signal/exit-code contract (aligns with the finalized in-process supervision semantics in windows-agent-design)
    Services() ServiceManager // service management surface hook (P7b): windows returns the SCM implementation;
                              // darwin/linux always return nil (launchd/systemd go through the
                              // business plugin channel, not a platform fact), so callers
                              // fall back to their own channel
}
```

| Platform     | Process supervision                     | Service integration                       | Current collection source                          |
| -------- | ---------------------------- | ------------------------------ | ------------------------------------- |
| linux    | systemd supervision (install.sh installs the unit) + signal-based graceful exit | install script; behaviour unchanged             | install.sh + the agent.go signal path        |
| windows  | SCM service state + **in-process supervision** (retry connection failure for 15s without exiting, SCM failure actions only as the backstop for hard crashes) | agent subcommands service install/uninstall/start/stop/status/run + Inno Setup installer + rsrc icon | cmd/cockpit-agent/service_windows.go, svcargs.go |
| darwin   | launchd plist                | service verbs (launchd implementation + stub) | rpc/service_launchd_darwin.go         |
| openwrt  | procd (init.d script)         | ipk package (install-and-run with opkg)     | openwrt-agent-design.md existing artifacts      |

**Build mechanism**: every file under `core/platform/{linux,windows,darwin}/` carries a `//go:build <goos>` tag; `core/platform/select_linux.go` (a three-line assembly: `import _` the matching platform package and register the implementation) is likewise one-of-three by GOOS tag. GOOS decides which implementation is installed, resolved at compile time with zero runtime branches.

**Exit semantics contract**: on any platform an agent is a "qualified supervised subject" out of the factory — any one of SIGTERM / SCM Stop / launchd TERM triggers a graceful exit (flush buffers, drop connections, exit code 0 = deliberately stopped); a crash or hard kill exits non-zero and is handed to the platform supervisor (systemd Restart / SCM failure actions / launchd KeepAlive / procd respawn) to relaunch per its own configuration. The platform layer guarantees this semantics is consistent across platforms, and business plugins are unaware of it.

## 4. Directory tree (planned)

```text
core/                        # ① Generic capability layer (same repo, same module, pruned at package granularity)
  register/                  #   Registration and going live (machine-id derivation lives in the platform layer, assembled here)
  heartbeat/                 #   Heartbeat and liveness (period/piggyback snapshot/reconnect suspension)
  backoff/                   #   Unified backoff knob (thin wrapper, ~60 lines)
  logx/                      #   slog assembly (stdlib, zero third-party deps)
  metrics/                   #   gopsutil v3 collector (can run standalone)
  report/                    #   Unified uplink: Codec wrapper + outbound channel + piggybacking
  configsync/                #   Version poll fetch + hot-apply
  execlog/                   #   Unified execution audit (local trail + uplink)
  healthprobe/               #   Liveness/Readiness/Heartbeat three-semantics probe + failure windows
  crash/                     #   Crash collection slot (skeleton + interface, not implemented)
  platform/                  # ② Platform adaptation layer (the only GOOS branch zone)
    host.go                  #   Host contract
    select_linux.go / select_windows.go / select_darwin.go
    linux/                   #   systemd/paths/signals (~/.ssh defaults, etc.)
    windows/                 #   Full SCM chain/svcargs/ProgramData paths
    darwin/                  #   launchd plist/paths
internal/agent/              # ③ Plugin host + business plugins (rpc Providers stay in place, Provider registry/capability detection unchanged)
internal/protocol/           #   One protocol (the only internal package core may import)
cmd/cockpit-agent/           #   main thin shell: core part assembly + platform layer wiring (116 lines today, thinner after)
```

- No separate Go module (in a single-repo single-maintainer rhythm a separate module is pure friction, same judgement as croupier).
- Core packages never depend on each other laterally (except leaf packages like `backoff`/`logx`); `platform/` is only imported by main and by packages that need platform facts such as register/heartbeat, and the direction is always `package → platform` — platform imports no core sibling package.
- Mapping for collecting legacy platform files (executed in batch P4): `machineid_*` → `platform/<goos>/machineid.go`; `localip.go` → unrelated to `core/platform`, stays in `core/netutil` or in place (P7a settled: **keep in place** — pure network facts with zero GOOS branches, consumed only on the registration path, not a platform fact, and not general enough to justify a core/netutil); `rpc/service_windows_scm.go` → `platform/windows/`; `rpc/service_launchd_darwin.go` → `platform/darwin/`; `rpc/job_provider_{unix,windows}.go`, `file_stat_{unix,windows}.go`, `backup_hook_{unix,other}.go` → stay in place as in-plugin platform files and are registered in the §8 whitelist; after P4 everything new goes into the platform layer.

## 5. Build matrix (platform-specialised artifacts)

Parts already in place are not re-litigated: the nightly `build-agent` 7 targets (linux amd64/arm64/arm, windows amd64/arm64, darwin amd64/arm64) + `build-openwrt` 5 ipk architectures (mipsel/mips/aarch64_generic/arm_cortex-a7_neon-vfpv4/x86_64) + Windows setup.exe (the Inno Setup inspection job). This time the "matrix" is promoted from the packaging layer to an **architectural promise**:

| Dimension       | Values                                                                 | Mechanism                                   |
| ---------- | -------------------------------------------------------------------- | -------------------------------------- |
| Target platform   | linux amd64/arm64/arm · windows amd64/arm64 · darwin amd64/arm64 · openwrt mips/mipsle/arm64 | the existing nightly matrix is the reference           |
| Platform layer     | selected and installed automatically at compile time by GOOS (§3 select_*.go)                              | build tags, zero configuration in the matrix                 |
| Plugin surface     | pruned at runtime by capability detection (unchanged today: docker/pve/openwrt/nginx…)   | detector + Provider registry             |
| Compile-time pruning | **slot reserved, not proposed**: build tags predefine plugin profiles (e.g. an openwrt minimal surface)      | raised as a separate batch when a real size need appears                   |
| Packaging form   | tar.gz (linux/darwin) · zip (windows) · setup.exe (windows) · ipk (openwrt) | existing packaging jobs untouched                      |
| Verification       | green build per target + Windows installer inspection job + probe real-machine regression              | existing CI + scripts/acceptance/          |

In one sentence: **the same core + platform layer + plugins, with the matrix specialising into 7+5 artifacts by GOOS**; all platform differences are absorbed into the compile-time platform layer and the packaging form, and the matrix entries themselves carry no platform if branches.

## 6. Verification framing (shared by every batch)

- **Zero behaviour change** is the hard gate: extraction = switching imports in place, the existing `go test ./...` stays green + the coverage gate (tool/coverage_check.sh) + real-machine probe regression (the scripts/acceptance/ suites) stay unred.
- The platform batch (P4) adds one line: the Windows installer job inspection + openwrt ipk installs + darwin launchd loads (all already covered by the nightly inspection surface).
- The web surface has no intersection with this refactor, so the web gate is not triggered by it.

## 7. Extraction batches (each an independent commit, CI green before the next)

| Batch  | Content                                                                                                                                 | Risk | Test strategy                                                       | Status |
| --- | ------------------------------------------------------------------------------------------------------------------------------------ | ---- | -------------------------------------------------------------- | ---- |
| P1  | `core/` skeleton + hoisting the `backoff`/`logx` thin shells (the agent.go reconnect backoff switches imports in place, zero behaviour change)                                                  | Low   | existing cases migrate with it + positive and negative backoff timing cases (injection-point convention unchanged)               | ✅ Landed |
| P2  | Hoisting `register`+`heartbeat`+`report` — **agent.go's 778 lines split into loops**: connect/reconnect/register/heartbeat each relocate, agent.go becomes an assembly shell                        | Medium   | existing agent_ws/agent_handler cases migrate; register-heartbeat-reconnect integration cases     | ✅ Landed |
| P3  | `metrics`+`execlog`+`healthprobe`: the collector relocates; execution audit lands (local trail + uplink); the service_health policy layer switches to the shared probe (three semantics + failure windows) | Medium   | collector cases migrate; execlog trail-before-uplink ordering cases; probe window open/close/recovery assertions | ✅ Landed |
| P4  | `platform/` collection (SCM/launchd/machineid/paths) + `configsync` + the `crash` slot; the platform-file whitelist drops to zero                                        | High   | full cross-platform CI matrix green + installer/ipk/launchd inspection; signal-exit semantics cases | ✅ Landed (configsync not done, see todo) |
| P5 | Protocol `exec_log` message + service-detect agent + probe uplink wiring | — | execlog trail-before-uplink + probe_report reporting cases | ✅ Landed |

- Orchestration with in-flight work: Workflow M1 acceptance and the probe line close out first, then P1 starts; no half-finished dependency is left between batches.
- **Rollback strategy**: in every batch `cmd/cockpit-agent` can still compile from the original internal paths (hoisting = an internal implementation switching imports), so halting any batch leaves no dangling references.

## 8. Known boundaries (honest list)

- **Dual-path transition period**: during P1–P4, core/ and internal/ coexist (the legacy platform file whitelist: `rpc/file_stat_*`, `rpc/job_provider_*`, `rpc/backup_hook_*`, `rpc/service_{windows,launchd}_{model,stub}.go` — after P4 the remainder is `machineid_*`, `rpc/service_windows_scm.go` and `rpc/service_launchd_darwin.go` collected into core/platform, while model/stub and in-plugin platform files stay in place per the §batch mapping).
- core depending on `internal/protocol` is the price of "one protocol"; if core ever needs to be reused outside this repo, protocol is promoted to top-level in a separate batch.
- Introducing slog in `logx` is a new selection (stdlib): legacy `log.Printf` is not force-migrated, new code uses logx, and a wholesale replacement is not a goal.
- The hundred-line discipline may break for assembly-dense packages like `platform/windows` (collating SCM's 401 lines): a break must be explained in the commit message by splitting layers (SCM interaction vs service verbs into separate files), not enforced as a dead number.
- Compile-time plugin pruning, crash collection, and configsync's server-side config source surface: this document only reserves slots for them, none is proposed.
- OpenWrt's musl/mips static build and the CGO=0 premise (the packaging decision in openwrt-agent-design.md) are hard constraints of the platform layer: no core package may introduce a CGO dependency, and new dependencies in packages like metrics must pass the static-build check.