---
title: libvirt 绑定选型结论与启停片设计（项 28）
status: design-draft (2026-10-30, 已代决 docs/review/2026-10-30-decisions.md 项 28)
---

# libvirt 绑定选型结论与启停片设计（项 28）

> 依据：决策项 28（保留 agent 驻宿主路线，先做只读真机/API 验收；下一片只考虑目标白名单内的启停；绑定选型先写比较结论，暂沿用现有 virsh 适配）。
> 状态：**设计稿**（绑定比较结论已核实，§1）；只读真机/API 验收需隔离宿主/测试 VM（待用户）。
> 边界：Server 不直连宿主 libvirt；不一次做快照/克隆/迁移、不扩 Hyper-V/Firecracker；未核对存储条件前延后快照/克隆（决策明确不做）。

## 1. 绑定选型比较结论（CLI 适配 vs Go 绑定）

| 维度 | 现有 CLI 适配（`virsh -r`） | Go 绑定（如 libvirt-go） |
|---|---|---|
| 依赖 | 宿主有 `virsh` 即可（`internal/agent/rpc/libvirt_provider.go:34` LookPath 检测）；go.mod/go.sum 零新增依赖（核实：`grep -i libvirt`＝0） | 需 cgo＋系统 libvirt-dev 构建依赖，与宿主 libvirt 版本耦合 |
| 权限边界 | `-r` 只读连接——连接本身不能变更状态（`libvirt_provider.go:12-14` 注释明确）；写片可单独去 `-r` 并显式授权 | 直接持 socket 权限，读写由代码控制，边界靠自觉 |
| 超时/错误语义 | 进程级超时（`libvirtProbeTimeout=5s`/`libvirtListTimeout=10s`）；stderr 原文保留 | API 调用级错误，语义更结构化 |
| 测试 | 注入假 runner/PATH 桩，零 virsh 子进程（既有单测模式） | 需真实 daemon 或较重 mock |
| 覆盖 | 只读三动作（capabilities/vms.list/containers.list）足够当前片 | 全 API 面（未来快照/迁移或需） |

**结论**：**本阶段维持 CLI 适配，不引入 Go 绑定**（决策项 28「暂沿用现有 virsh 适配，不为尚未出现的需求立即替换」）。若未来快照/迁移片对结构化错误或细粒度控制有明确需求，再评估换绑——由该需求触发，不提前。

## 2. 启停片设计（下一片，只考虑目标白名单内的启停）

- **动作面**：`vm.start` / `vm.stop` / `vm.reboot`（对齐 `docs/design/hypervisor-capabilities.md` §2 的 `vm.start/vm.stop/vm.reboot` 接口键；PVE 同名先例 `pve/client.go:287-321`）。
- **白名单**：启停仅对显式白名单内目标开放（VM 名单或 tag），白名单外目标 fail-closed 拒绝——决策项 28「下一片只考虑目标白名单内的启停」。
- **权限**：去掉 `-r` 需独立权限与授权——写连接是独立授权目标，不是默认继承只读片；写动作需二次确认＋审计事件（`virt.vm.{start,stop,reboot}`，对齐 hypervisor-capabilities.md §4 红线 3）。
- **超时/取消**：写操作带超时与取消语义（`exec.CommandContext` 既有模式）；`virsh start/stop` 的退出码与 stderr 原文保留。
- **RPC 面**：经 agent RPC 转发，server REST `/api/agents/{id}/virt/*` 纯转发不持久化（agent 是事实源，hypervisor-capabilities.md §3 同构）。

## 3. 授权目标（本片拍板门）

- 启停白名单（哪些 VM）＋写连接权限范围（root/libvirt 组/受限策略）由用户拍板。
- 真机验收条件：带本地 libvirt socket 的隔离宿主＋测试 VM，先验默认 QEMU 与独立 LXC 连接（`libvirtLXCURI=lxc:///`，`libvirt_provider.go:26` 注释记录的 LXC driver 缺口已修），再验真实 HTTP→RPC 输出。

## 4. 待用户项

- **隔离宿主＋测试 VM**（真机/API 验收前置，决策项 28「提供带本地 libvirt socket 的隔离宿主和测试 VM」）。
- **启停白名单与写授权拍板**。
- PVE REST 测试不能代替本地 libvirt 验收（决策边界）。

## 5. 决策对照

本设计落实项 28 的「比较现有 CLI 适配与 Go 绑定所需依赖、超时/错误语义（§1）」「若维持 CLI，明确该阶段不引入新绑定（§1 结论）」「单列启停设计及授权目标（§2-3）」。真机验收待用户提供环境。
