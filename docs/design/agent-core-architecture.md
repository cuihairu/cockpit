---
title: agent-core 架构重构——core/平台层/插件三层与构建矩阵（立项待批）
---

# agent-core 架构重构简档

## 状态

- 状态: **Proposed（架构文档先行，零代码；用户过目后按 §7 批次实施）**。
- 立项口径（用户令，2026-10-09）：
  1. **核心抽取**：通用能力收进 core（注册/心跳/日志指标/配置热更/崩溃留位/重试/上报/执行审计 execlog/探活 system 角色），core 守住百行级文件、能力可裁剪，业务只写插件；
  2. **平台差异分层**：不同平台特性（linux/windows/macos 各自的进程监管、路径、服务集成）放对应系统的独立目录（core / 平台适配层 / 业务插件 三层）；
  3. **目标**：同一份 core+插件编译出针对性的平台特化 agent 产物（CI 矩阵按平台构建）。
- 节奏对标：croupier `docs/design/agent-core-design.md`（core=零件箱不是框架、能力可裁、百行纪律、批次抽取、回退策略同款）。
- 底稿（现状权威）：[架构与边界](../guide/architecture.md) · [Windows Agent 打包设计](../guide/windows-agent-design.md)（SCM 服务态已定稿）· [OpenWrt Agent 打包设计](../guide/openwrt-agent-design.md)（ipk/procd 已落地）· [服务健康探针与自愈设计](../guide/service-health-design.md)（healthprobe 的现成种子）。

## 1. 定位：三层架构

**core 不是框架，是零件箱。** 不做 god-object 装配（现状 `internal/agent/agent.go` 778 行的大对象是本次要拆的债），每个能力一个独立小包，main 按需 import——没 import 的能力不参与编译，天然「按需可裁」。

```text
core/                 ① 通用能力层：百行级文件、零业务、零平台分支
core/platform/        ② 平台适配层：唯一允许 GOOS 分支的地带（进程监管/路径/服务集成）
internal/agent/       ③ 插件宿主 + 业务插件：Provider 注册表、能力检测、rpc Providers
```

依赖方向（单向，禁止倒灌）：

```text
cmd/cockpit-agent → internal/agent(插件) → core/* → internal/protocol
                                    core/platform/* ← core 其余包不得 import
```

三条不变量：

1. **协议一份**：Server↔Agent 的 wire 类型唯一来源仍是 `internal/protocol`。core 各包允许 import 它——这是「core 不 import internal/」原则的**唯一白名单例外**（协议是双方共有物，不是 internal 私产）；若未来 core 需要彻底独立，届时把 protocol 升 top-level 另批执行。
2. **平台分支一处**：GOOS 编译标签与运行时平台判定只准出现在 `core/platform/`。存量散落的平台文件（`machineid_*`、`rpc/service_windows_scm.go`、`rpc/job_provider_unix.go` 等）在 P4 批次收编，收编前按 §8 白名单登记，新增平台代码一律直接进平台层。
3. **库选型不翻案**：gopsutil v3（不升 v4，随批次再议）、gorilla/websocket、protocol.Codec 序列化——core 只收拢用法不换库。

## 2. 能力清单表（每项：已有/部分/新建/来源现状）

| #  | 能力                     | 状态         | 来源与现状                                                                                                                     | core 包（拟）        |
| -- | ------------------------ | ------------ | ------------------------------------------------------------------------------------------------------------------------------ | -------------------- |
| 1  | 注册上线                 | **已有**     | agent.go register 首包链 + secret 校验；ID 由 machine-id 派生（`machineid_{linux,darwin,windows,other}.go`）                    | `core/register`      |
| 2  | 心跳与存活               | **已有**     | 30s 心跳 + startedAt 运行时长 + 开放服务面捎带（servicesOnce 后台重探测）                                                       | `core/heartbeat`     |
| 3  | 重试退避                 | **已有，手写** | agent.go 重连计时器/防并发重连；rpc 各 provider 内散落重试写法——上收为统一旋钮杜绝第四套                                        | `core/backoff`       |
| 4  | 结构化日志               | **新建（选型）** | 现状 stdlib `log.Printf`；logx 以 **slog**（标准库，零三方依赖）落底，存量不强迁——新代码用 logx，随批次渐进                     | `core/logx`          |
| 5  | 指标采集                 | **已有**     | collector.go：gopsutil v3 CPU/内存/磁盘/负载/host，155 行已是零件形态，原地归位                                                  | `core/metrics`       |
| 6  | 统一上行                 | **已有**     | protocol.Codec + outbound channel；指标/服务面/注册/心跳全部走它——上收为编解码与通道薄封装                                      | `core/report`        |
| 7  | 配置拉取与热更           | **新建**     | 现状仅启动 flags（docs/agent-config.md），无拉取热更；泛化「版本号轮询→拉取→热生效」通用件（server 侧 inventory.watch 同口径先例） | `core/configsync`    |
| 8  | 执行审计（execlog）      | **新建**     | 现状执行审计只在 Server 侧 audit 表；agent 侧每次执行留痕（action/入参/结果/耗时）+ 上行，对齐 jobs 审计口径                     | `core/execlog`       |
| 9  | 探活（system 角色）      | **部分**     | rpc/service_health.go 已有服务探针+自愈策略层，但挂在 service provider 上非公共件；升为公共探针——Liveness/Readiness/Heartbeat 三语义 + 故障窗口时间线，探针记录按受监控 system 建模 | `core/healthprobe`   |
| 10 | 崩溃采集                 | **留位**     | 仅包骨架+接口占位不实现（分级方案届时按调研另批，对标 croupier crash-capture-survey 节奏）                                       | `core/crash`         |
| 11 | 进程监管/服务集成/路径    | **已有，平台分散** | Windows SCM（service_windows.go 401 行 + svcargs）、darwin launchd（207 行）、openwrt procd（ipk/init.d）、linux systemd（install.sh） | `core/platform/<goos>`（§3） |

> 上收边界（可裁的第一次应用）：`internal/agent/rpc/` 的 20+ Provider **全部留在插件面不进 core**——它们是 cockpit 的业务面；core 系未来任何小 agent 天然没有这些攻击面与体积。

## 3. 平台适配层（core/platform/）

平台层对外只暴露一个契约，插件与 main 只认接口不认平台：

```go
// core/platform/host.go（契约示意，百行内）
type Host interface {
    Paths() Paths          // 配置/日志/数据目录（linux /etc/cockpit,~/.config；windows ProgramData；darwin ~/Library）
    Service() Service      // 服务注册/注销/启停/状态/SCM-run 入口
    MachineID() string     // 机器标识派生（收编 machineid_*.go）
    Signals() Signals      // 信号/退出码约定（对齐 windows-agent-design 已定稿的进程内看护语义）
}
```

| 平台     | 进程监管                     | 服务集成                       | 现状收编来源                          |
| -------- | ---------------------------- | ------------------------------ | ------------------------------------- |
| linux    | systemd 托管（install.sh 装unit）+ 信号优雅退出 | 安装脚本；行为不变             | install.sh + agent.go 信号路径        |
| windows  | SCM 服务态 + **进程内看护**（连接失败 15s 重试不退进程，SCM failure actions 仅兜底硬崩） | agent 子命令 service install/uninstall/start/stop/status/run + Inno Setup 安装器 + rsrc 图标 | cmd/cockpit-agent/service_windows.go、svcargs.go |
| darwin   | launchd plist                | 服务动词（launchd 实现 + stub） | rpc/service_launchd_darwin.go         |
| openwrt  | procd（init.d 脚本）         | ipk 安装包（opkg 即装即用）     | openwrt-agent-design.md 既有产物      |

**构建机制**：`core/platform/{linux,windows,darwin}/` 目录内文件全量带 `//go:build <goos>` 标签；`core/platform/select_linux.go`（三行装配：`import _` 对应平台包并注册实现）同样按 GOOS 标签三选一。GOOS 决定装哪个实现，编译期完成、运行时零分支。

**退出语义契约**：任一平台的 agent 出厂即「合格被监管对象」——SIGTERM/SCM Stop/lanchd TERM 之一触发优雅退出（刷缓冲、断连接、退出码 0=主动停）；崩溃/硬杀退出码非 0，交给平台监管方（systemd Restart / SCM failure actions / launchd KeepAlive / procd respawn）按各自配置拉起。平台层保证这套语义在各平台一致，业务插件无感知。

## 4. 目录树（拟）

```text
core/                        # ① 通用能力层（同仓同 module，包级裁剪）
  register/                  #   注册上线（machine-id 派生在平台层，这里组装首包）
  heartbeat/                 #   心跳与存活（周期/捎带快照/重连挂起）
  backoff/                   #   统一退避旋钮（薄封装，~60 行）
  logx/                      #   slog 装配（标准库，零三方依赖）
  metrics/                   #   gopsutil v3 采集器（可独立运行）
  report/                    #   统一上行：Codec 封装 + outbound 通道 + 捎带
  configsync/                #   版本轮询拉取 + 热生效
  execlog/                   #   统一执行审计（本地留痕 + 上行）
  healthprobe/               #   Liveness/Readiness/Heartbeat 三语义探针 + 故障窗口
  crash/                     #   崩溃采集留位（骨架+接口，不实现）
  platform/                  # ② 平台适配层（唯一 GOOS 分支地带）
    host.go                  #   Host 契约
    select_linux.go / select_windows.go / select_darwin.go
    linux/                   #   systemd/路径/信号（~/.ssh 默认等）
    windows/                 #   SCM 全链/svcargs/ProgramData 路径
    darwin/                  #   launchd plist/路径
internal/agent/              # ③ 插件宿主 + 业务插件（rpc Providers 原地，Provider 注册表/能力检测不动）
internal/protocol/           # 协议一份（core 唯一可 import 的 internal 包）
cmd/cockpit-agent/           # main 薄壳：core 零件装配 + 平台层接线（112 行现状，更薄）
```

- 不建独立 Go module（单仓单人节奏下独立 module 是纯摩擦，同 croupier 判断）。
- core 各包互不横向依赖（`backoff`/`logx` 这类叶包除外）；`platform/` 只被 main 与 register/heartbeat 等需要平台事实的包 import，方向恒为 `包 → platform`，platform 不 import 任何 core 兄弟包。
- 存量平台文件收编映射（P4 批执行）：`machineid_*` → `platform/<goos>/machineid.go`；`localip.go` → `core/platform` 无关、留 `core/netutil` 或原地（P7a 已定：**原地保留**——纯网络事实零 GOOS 分支、仅注册路径消费，非平台事实不进平台层，通用度亦不足以立 core/netutil）；`rpc/service_windows_scm.go` → `platform/windows/`；`rpc/service_launchd_darwin.go` → `platform/darwin/`；`rpc/job_provider_{unix,windows}.go`、`file_stat_{unix,windows}.go`、`backup_hook_{unix,other}.go` → 就地保留为插件内平台文件并在 §8 白名单登记，P4 后新增一律进平台层。

## 5. 构建矩阵（平台特化产物）

现状已成立的部分不翻案：nightly `build-agent` 7 目标（linux amd64/arm64/arm、windows amd64/arm64、darwin amd64/arm64）+ `build-openwrt` 3 架 ipk + Windows setup.exe（Inno Setup 走查 job）。本次把「矩阵」从打包层升格为**架构承诺**：

| 维度       | 取值                                                                 | 机制                                   |
| ---------- | -------------------------------------------------------------------- | -------------------------------------- |
| 目标平台   | linux amd64/arm64/arm · windows amd64/arm64 · darwin amd64/arm64 · openwrt mips/mipsle/arm64 | 既有 nightly matrix 收编为准           |
| 平台层     | 随 GOOS 编译期自动选装（§3 select_*.go）                              | 编译标签，矩阵里零配置                 |
| 插件面     | 运行时按 capability 检测裁剪（现状不变：docker/pve/openwrt/nginx…）   | detector + Provider 注册表             |
| 编译期裁剪 | **留位不立项**：build tags 预置插件 profile（如 openwrt 极简面）      | 出现真实体积需求再批                   |
| 打包形态   | tar.gz（linux/darwin）· zip（windows）· setup.exe（windows）· ipk（openwrt） | 既有打包 job 不动                      |
| 验证       | 每目标构建绿 + Windows installer 走查 job + 探针真机回归              | 既有 CI + scripts/acceptance/          |

一句话：**同一份 core + 平台层 + 插件，矩阵按 GOOS 特化出 7+3 个产物**；平台差异全部消化在编译期平台层与打包形态，矩阵条目本身不携带平台 if 分支。

## 6. 验证口径（每批通用）

- **行为零变化**是硬门槛：抽取=原地切换 import，现有 `go test ./...` 全绿 + coverage 门禁（tool/coverage_check.sh）+ 真机探针回归（scripts/acceptance/ 各套）不红。
- 平台批次（P4）加一条：Windows installer job 走查 + openwrt ipk 可安装 + darwin launchd 加载（nightly 既有走查面）。
- web 面与本重构无交集，web 门禁不因本重构触发。

## 7. 抽取批次（每批独立提交，CI 绿进下批）

| 批  | 内容                                                                                                                                 | 风险 | 测试策略                                                       |
| --- | ------------------------------------------------------------------------------------------------------------------------------------ | ---- | -------------------------------------------------------------- |
| P1  | core/ 骨架 + `backoff`/`logx` 薄壳上收（agent.go 重连退避原地切 import，行为零变化）                                                  | 低   | 现有用例随迁 + 退避时序正反用例（注入点惯例不变）               |
| P2  | `register`+`heartbeat`+`report` 上收——**agent.go 778 行拆环**：连接/重连/注册/心跳各自归位，agent.go 降为装配壳                        | 中   | agent_ws/agent_handler 既有用例随迁；注册-心跳-重连集成用例     |
| P3  | `metrics`+`execlog`+`healthprobe`：collector 归位；执行审计落地（本地留痕+上行）；service_health 策略层切换到公共探针（三语义+故障窗口） | 中   | 采集器用例随迁；execlog 先落痕后上行的顺序用例；探针窗口开合/恢复断言 |
| P4  | `platform/` 收编（SCM/launchd/machineid/路径）+ `configsync` + `crash` 留位；平台文件白名单清零                                        | 高   | 跨平台 CI 矩阵全绿 + installer/ipk/launchd 走查；信号退出语义用例 |

- 与在途工作编排：Workflow M1 验收与探针线先行收尾，P1 在其后启动；批次间不留半成品依赖。
- **回退策略**：每批 `cmd/cockpit-agent` 仍可从 internal 原路径编译（上收=内部实现切换 import），任一批叫停不留悬空引用。

## 8. 已知边界（诚实清单）

- **双路径过渡期**：P1–P4 期间 core/ 与 internal/ 并存（存量平台文件白名单：`rpc/file_stat_*`、`rpc/job_provider_*`、`rpc/backup_hook_*`、`rpc/service_{windows,launchd}_{model,stub}.go`——P4 后余量（`machineid_*`、`rpc/service_windows_scm.go`、`rpc/service_launchd_darwin.go` 已收编 core/platform，模型/stub 与插件内平台文件按 §批次映射就地保留）。
- core 依赖 `internal/protocol` 是白名单例外（协议一份的代价）；若未来 core 要脱离本仓复用，protocol 升 top-level 另批执行。
- `logx` 引入 slog 是新选型（标准库）：存量 `log.Printf` 不强迁，新代码用 logx，全量替换不做目标。
- 百行纪律对 `platform/windows`（SCM 401 行收编）这类装配密集包可能破线：破线需在提交说明拆分层（SCM 交互与服务动词分文件），不当死数字硬拆。
- 编译期插件裁剪、crash 采集、configsync 的 server 侧配置源面：本文只留位，均不立项。
- OpenWrt 的 musl/mips 静态构建与 CGO=0 前提（openwrt-agent-design.md 拆包决策）是平台层的硬约束：core 各包不得引入 CGO 依赖，metrics 等三方面包新增依赖时须过静态编译检查。
