# Cloud Provider Framework Design (P0 Slice 1)

> Status: design settled; implementation lands per slice. Source review (Chinese):
> `docs/review/chatgpt-2026-10-20-multicloud-positioning.md`. Boundary: no Kubernetes
> CRD full model (closed list); this design defines Go interfaces and an in-memory registry only.

## 1. Goals and Non-goals

**Goal**: add the foundation for a Cloud-API-driven resource control plane — pluggable
provider interfaces, capability declaration, asynchronous operation semantics, unified
resource and cost types. Framework and types land first; vendor adapters follow per slice.

**Non-goals** (excluded from this slice): no real cloud SDK integration (needs user
AccessKeys in later slices); no resource pool / reconciler / scheduling (P1); no budget
policy engine (P1); no Inventory migration (existing `ComputeInstance` untouched — cloud
resources are stored in a parallel model and linked via resource discovery).

## 2. First Vendors (decided by orchestrator 2026-10-20, overridable)

**Aliyun + Tencent Cloud**: direct domestic network path; the repo already ships Aliyun
DNS (`internal/dns/alidns.go` with a ready signing helper `sign.go`) and DNSPod
integration experience; accounts already owned by the user. AWS/Azure/GCP later on demand.

## 3. Interface Split (capability declaration + small interfaces)

```go
// internal/cloud
type Provider interface {
    Identity() ProviderIdentity
    Capabilities() []Capability
}
type ComputeProvider interface {
    ListInstances(ctx context.Context) ([]Instance, error)
    CreateInstance(ctx context.Context, spec InstanceSpec) (Operation, error)
    StartInstance(ctx context.Context, id string) (Operation, error)
    StopInstance(ctx context.Context, id string) (Operation, error)
    DeleteInstance(ctx context.Context, id string) (Operation, error)
    GetOperation(ctx context.Context, op Operation) (Operation, error)
}
type BillingProvider interface {
    QueryCosts(ctx context.Context, q CostQuery) ([]CostRecord, error)
}
type QuotaProvider interface {
    QueryQuotas(ctx context.Context, region string) ([]Quota, error)
}
```

**Declaration discipline**: the registry rejects a provider that declares `CapabilityCompute`
without implementing `ComputeProvider` (fail-closed, so callers never see a phantom capability).

## 4. Async Operation Semantics

A successful cloud API response does not mean the resource is ready (e.g. Tencent CVM
returns a RequestId and polling continues). Unified model:

```
Operation{ ID, Kind(create/start/stop/delete), NativeID, VendorRequestID,
           Status(pending|running|succeeded|failed), Error, UpdatedAt }
```

State machine: `pending → running → succeeded|failed`; terminal states reject further
driving. `VendorRequestID` is preserved verbatim (audit and vendor ticket evidence);
failures keep the vendor error text untranslated (no error swallowing).

## 5. Resource and Cost Model (new tables; ComputeInstance untouched)

- `cloud_account`: vendor, alias, credential reference (**vault reference only, `${VAR}`
  style — never plaintext**), permission scope, enabled flag.
- `cloud_instance`: account, native resource ID, name, region/zone, spec, runtime status,
  billing mode (on-demand/prepaid/spot), raw vendor status string.
- Resource state and billing state are **separate**: `stopped` does not imply billing
  stops; the model exposes `BillingContinues bool` filled by the adapter from facts.
- The three cost tiers (estimated / run-estimate / actual) land as the `CostRecord` type
  and `BillingProvider` interface only; bill collectors come with the adapter slices.

## 6. Credentials and Safety Boundary

Credentials live in the existing vault; config stores references only. Single-writer rule:
write operations against a cloud resource may only be issued through Cockpit's
`ComputeProvider`; if OpenTofu is ever introduced, write ownership must be adjudicated
first (P1 decision point — documented here, no mechanism in this slice).

## 7. Slice Plan

| Slice | Content | Gate |
|---|---|---|
| **1 (this)** | `internal/cloud`: interfaces, capability declaration + registry validation, Operation state machine, resource/cost types | `go build ./...`, `go test ./internal/cloud/...`, 100% package coverage per registry policy |
| **2 (done 2026-10-20)** | `cloud_accounts` / `cloud_instances` tables (GORM AutoMigrate runs inside Open, no separate migration script) + storage layer `internal/storage/cloud.go` (account upsert/list/delete, instance upsert/list/delete, all idempotent on their business unique keys) + credential red line enforced: only `${VAR}` references stored (form validation rejects plaintext and concatenation), resolved at runtime via env, fail-closed when unset | same + `go test ./internal/storage/...` |
| 3 | Aliyun adapter (Compute+Quota; reuse `sign.go`) + fake-server contract tests | same + vendor contract tests |
| 4 | Tencent Cloud adapter + two-vendor closed-loop acceptance (needs user AccessKeys) | on-machine acceptance checklist |

**External blockage (slices 3/4)**: real cloud AccessKeys are provided by the user
(configured through the vault; never in chat, never in the repo).
