---
title: Hypervisor platform capabilities — hypervisor provider layer (unified VM lifecycle across PVE/vSphere/local QEMU-KVM)
---

# Hypervisor Platform Capability Design

> Language: [中文](./hypervisor-capabilities.md) ｜ **English**

## Status

- Status: **Active (user directive 2026-10-10 design follow-up 2: isomorphic with the BMC layer, entering the plugin mechanism)**.
- Positioning: hypervisor provider layer — unifying PVE / vSphere / local QEMU-KVM into **one set of VM lifecycle interfaces**, sharing the same provider mechanism as [BMC out-of-band management](bmc-capabilities.en.md) (capability probing + RPC forwarding + credential delivery + audit red lines), with differences only in platform client adaptation.
- Relationship with the BMC layer: **in-band vs out-of-band complementarity** — BMC manages hardware/power (controllable even with the host powered off); this layer manages VM lifecycle (when the platform is online); the panel presents them in separate sections, with no overlapping semantics (likewise non-overlapping with the service-detect agent's "is the service reachable").
- Zero code in this batch; implementation batches in §5.

## 1. Platforms and selection (research settled 2026-10-10: reference mature libraries directly, no in-house development)

| Platform | Selection | Notes |
| --- | --- | --- |
| PVE | **`github.com/bpg/proxmox-api`** (MPL-2.0, active) | A standalone client extracted from the bpg/terraform-provider-proxmox 2.3k★ family; REST+SSH; alternative Telmate/proxmox-api-go (veteran, listed officially in the wiki) |
| vSphere | **`github.com/vmware/govmomi`** (Apache-2, ~2.6k★ official-grade) | Ships the govc CLI + **vcsim simulator — CI uses vcsim, no real machine needed** |
| Local QEMU/KVM | **Reserved slot for libvirt Go bindings** | No selection settled; only a slot is reserved (in-band direct connection when the agent and hypervisor share the host) |

## 2. Unified VM lifecycle interface (platform-agnostic)

| Capability | Interface key | Destructive |
| --- | --- | --- |
| List | `vm.list` | No |
| Detail | `vm.get` | No |
| Create | `vm.create` | No (consumes resources; audited) |
| Start/stop | `vm.start` / `vm.stop` / `vm.reboot` | stop/reboot are destructive (audited) |
| Clone | `vm.clone` | Destructive (audited) |
| Delete | `vm.delete` | **High risk (double confirmation + audit)** |
| Console entry | `vm.console` | No (link/proxy surface; slot reserved in this batch) |
| Resource usage | `vm.metrics` | No (connects to the existing metrics surface) |
| Platform/node | `node.list` / `platform.info` | No |

## 3. Placement (provider mechanism isomorphic with BMC)

- **agent-side provider** (following the nginx/traefik/stacks/BMC precedent): capabilities `virt-pve`/`virt-vsphere`/`virt-libvirt` enter the registration metadata; server RPC methods `virt.*` are forwarded, and REST `/api/agents/{id}/virt/*` is pure forwarding with no persistence (the agent side is the source of truth, same as the proxy precedent).
- Platform clients are encapsulated in core (e.g. `core/virt`, pure clients with zero business logic); provider orchestration goes into `internal/agent` (plugin slot); the three platforms share one provider interface, with platform differences in the client adaptation layer.
- Credentials: PVE recommends **API tokens** (the PVEVMUser role with least privilege is better than a password); vSphere uses a service account; delivered over the encrypted WS for use-then-discard (same as the BMC §5 red line).
- CI strategy: vSphere via vcsim with no real machine; PVE via an interface stub (bpg/proxmox-api has no simulator); real-machine acceptance in a separate batch.

## 4. Security red lines (same as BMC)

1. **Credentials never in plaintext**: stored in the server credential vault (same AES-GCM model as the remote credential vault, with a `virt` type added); delivered over the encrypted WS for each operation, never written to the agent's disk and never in plaintext in logs/audit.
2. **TLS verified by default**: certificate validation is on by default; PVE self-signed certificates are common, so a per-target explicit `skip_verify` is limited to intranet self-signed cases (red-text warning in web).
3. **Destructive operations: double confirmation + audit**: `vm.delete`/`vm.stop` (forced)/`vm.clone` — second web confirmation (type the target name to confirm, same as the stack deletion precedent) + audit events `virt.vm.{delete,stop,clone,create}` (recording agent/platform/target/action/operator, no credentials) + execlog trail (depends on the P5 execlog pending item — MVP falls back on the existing audit surface first).
4. **Least-privilege separation of duties**: PVE API tokens use the PVEVMUser role; vSphere separates read-only roles from VM-management roles.

## 5. Batch plan (small steps: implementation + tests green + English commits)

| Batch | Content | Gate |
| --- | --- | --- |
| H-B1 | This brief | docs |
| H-B2 | PVE provider (list/get/start-stop + interface-stub tests) | Go gate |
| H-B3 | vSphere provider (full chain in CI via vcsim) | Go gate |
| H-B4 | Create/clone/delete + double confirmation + audit + credential-vault delivery | Go gate + web second confirmation |
| H-B5 | vm.metrics connected to the existing resource panel | Full web gate TZ=UTC |
| H-B6 | vm.console console entry | Go gate + manual acceptance |
| H-B7 | libvirt reserved slot wired in | Go gate |

## 6. Boundaries (honest list)

- No storage/network orchestration (PVE SDN, vSphere DVS) — bounded at VM lifecycle.
- Snapshot management and migration (HA/DRS) slots reserved; firmware/hardware surface belongs to the BMC layer.
- KVM direct-connection form only reserves a libvirt slot; no selection, nothing settled.
- No overlap with stack-deploy (docker compose container stacks): that manages containers, this layer manages VMs.
- Real-machine acceptance (PVE cluster/vCenter) in a separate batch; CI is based on vcsim + interface stubs.
