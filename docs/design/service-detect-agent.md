---
title: 服务检测 agent——healthprobe 独立探测面（目标格式+状态机+故障窗口）
---

# 服务检测 agent 简档

## 状态

- 状态: **Active——B1-B6 全部落地（2026-10-10，B6 web 面板块见 Services 页 ProbePanel）**。
  用户令 2026-10-09 立项：基于 core/healthprobe 做正式探测 agent，先简档后写码。
- 定位：独立小 agent（探测专用产物）——同一份 core 零件 + 平台层 + 探测业务插件，三层架构纪律同
  [agent-core](agent-core-architecture.md)。standalone（无 server）只做本地状态；配置 server 后经
  register/heartbeat/report 全链上行。
- 与 P6 挂起项的关系：P6（rpc service_health 引擎切 Liveness/Readiness/Heartbeat 三语义）**维持挂起
  待拍板**；本 agent 的故障窗口是 croupier「system 角色探针」口径在**独立探测面**的落地，不改动既有
  service_health 引擎，两者不互斥也不互相阻塞。

## 1. 目标格式（YAML，yaml.v3 已在依赖）

```yaml
# /etc/cockpit-agent/probe.yaml（默认路径 = platform Paths().ConfigDir/probe.yaml，
# -config 覆盖；这是 platform Paths 契约的第一个消费方）
server: ws://127.0.0.1:9000/ws   # 可选；缺省纯本地模式
secret: ""                        # 可选，注册认证
interval: 30s                     # 全局默认间隔
timeout: 5s                       # 全局默认超时
threshold: 3                      # 全局默认连续失败判故障（防抖）
recovery: 2                       # 全局默认连续成功判恢复
targets:
  - name: blog-http
    type: http
    url: http://127.0.0.1:8080/health
    expect_status: 200            # http 专用，缺省 200
    interval: 15s                 # 覆盖全局
  - name: pg-tcp
    type: tcp
    addr: 10.0.0.5:5432
    timeout: 2s
    threshold: 2
```

- name 必填且唯一（重复启动报错）；type 仅 http|tcp（core/healthprobe 既有 Checker 一对一映射）。
- target 级 interval/timeout/threshold/recovery 覆盖全局默认；duration 字段用 time.ParseDuration。

## 2. 状态机（防抖）

每 target 一个独立状态机，三种状态：

```text
Unknown ──连续成功≥recovery──▶ Healthy ──连续失败≥threshold──▶ Faulty
   │                               ▲                              │
   └─────连续失败≥threshold────────┼──────────────────────────────┘
                                   └────────连续成功≥recovery────────┘
```

- **Unknown 是启动冷区**：首轮结果不立即定性（threshold=1 时首轮失败即 Faulty）。
- 连续计数：结果与当前倾向一致则累计、相异则清零重计（标准防抖）。
- 状态迁移是唯一"事件"：Healthy→Faulty 开故障窗口，Faulty→Healthy 关窗口；窗口内单次探测失败
  /成功只更新计数与 last_error，不产生事件。

## 3. 故障窗口数据结构（可回查）

```go
// core/healthprobe/window.go
type Window struct {
    Target    string    `json:"target"`
    StartedAt time.Time `json:"startedAt"`            // Healthy→Faulty 时刻
    EndedAt   time.Time `json:"endedAt,omitempty"`    // 零值 = 仍在故障中
    LastError string    `json:"lastError"`
}
```

- 每 target 一个环形保留区：最多 1 个进行中 + 64 个已关闭窗口（容量常量，防长跑膨胀）。
- 查询面：`Snapshot() → {target, state, since, lastChecked, lastError, open, recent []Window}`；
  全量 `Snapshots()` 供上行与状态文件。
- 「什么时间段服务不可用」= closed 窗口序列 [StartedAt, EndedAt]；「现在是否不可用」= open 窗口。

## 4. 输出面

1. **本地**：内存 Snapshot（`cockpit-probe-agent status` 子命令读）；可选 `-status-file` 落
   JSON（默认关；开启时建议放 Paths().DataDir）。
2. **上行**：协议新消息 `probe_report`（MessageType 字符串枚举追加），payload =
   `{targets:[{name,state,since,last_checked,last_error}], windows:[近期窗口]}`——状态迁移即发
   + 每 10×interval 全量兜底；复用 core/report Enqueue 队列与 core/register/heartbeat 注册链。
3. **server**：handler 收 probe_report 落库（快照表 + 窗口表），REST `GET /api/probe/targets`、
   `GET /api/probe/windows?target=&limit=` 供面板回查；挂现有鉴权中间件。
4. **herald 告警出口（留位不实现）**：`Alerter interface { Alert(ev Transition) }` + noop 默认
   实现——接口先行，推送通道等 herald 对接另批。

## 5. 三层落位

```text
core/healthprobe/            # ① 通用能力：既有 HTTP/TCP Checker + 新增 state.go（状态机）
                             #    + window.go（窗口环）——零业务零平台，可独立复用
internal/probeagent/         # ③ 业务插件：config.go（YAML 解析校验）、agent.go（调度器：
                             #    每 target ticker + Checker + 状态机 + 事件分发）、report.go
                             #    （上行+状态文件+Alerter 留位）
cmd/cockpit-probe-agent/     # main 薄壳：-config/-status-file flags，standalone/server 双模式
core/platform                # ② 平台适配：Paths 契约供默认配置/状态路径（首个消费方）；
                             #    Signals 契约供优雅退出（startcmd 同款）
```

依赖方向同 agent-core：cmd → internal/probeagent → core/* → internal/protocol（白名单例外）。

## 6. 批次计划（小步：实现+测试绿+英文 commit）

| 批 | 内容 | 门禁 |
| --- | --- | --- |
| B1 | 本简档 | docs |
| B2 | core/healthprobe +state.go+window.go（状态机/窗口环/事件回调）+单测 | Go 门禁 |
| B3 | internal/probeagent config+scheduler+snapshot+Alerter 留位 +单测 | Go 门禁 |
| B4 | cmd/cockpit-probe-agent main（双模式+platform 接线）+ **本地样例实测** | 构建+实测 |
| B5 | protocol probe_report + server handler/存储/REST | Go 门禁+server 测试 |
| B6 | web 面板块（接现有 health/资源面板） | web 全量门禁 TZ=UTC |

## 7. 边界（诚实清单）

- 告警只留 Alerter 接口，不实现任何推送通道（herald 对接另批）。
- server 存储用 sqlite（既有 AutoMigrate 面），不做独立时序库；窗口保留上限同本地环形容量，
  更久回查属未来需求另批。
- 探测从 agent 侧发起（黑盒探测），不做 server 侧分布式探测点位；多点位归属 croupier 口径另议。
- rpc service_health 引擎（既有服务自愈面）不动；两套探测语义（服务自愈 vs 目标探活）并存，
  面板上分区呈现。
