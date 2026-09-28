# 服务健康探针与自愈（Service Health）

> 2026-09-28 立项。目标场景：docker VM（192.168.5.5）的 cloudflared tunnel
> 反复「进程活着、连接僵死」——systemd 只看到 active，业务早已不通，目前
> 靠外部 cron 看门狗临时兜底。本设计把这类「systemd 无感知的僵死」的检测
> 与自愈做成 cockpit 内置能力：agent 周期探针（HTTP/TCP/systemd 三类）+
> 白名单内的 systemctl 自愈 + 审计与仪表盘。与 [service-design](./service-design.md)
> （三后端执行层）、[overlay-design](./overlay-design.md) M3/M3.5（daemon
> 服务管理）同属服务管理面，见下方架构章。
>
> 首个接入用例（cloudflared）的接入手册见「首个接入用例」一节；用户的 cron
> 看门狗作为兜底保留，cockpit 版稳定后退役。

## 架构：服务管理面分层

| 层 | 承载 | 职责 | 本文关系 |
|----|------|------|----------|
| 执行层 | `service.*`（service-design 三后端） | list/status/start/stop/restart/…、unit 文件 | 自愈动作**复用**其 restart 路径（同一校验/超时/argv 纪律） |
| 观测层 | `overlay.daemon`（M3.5 读）、`overlay.service`（M3.5 写） | 组网工具 daemon 的安装态/运行态观测与启停 | 同为「面向特定服务面」的封装，白名单纪律同源（D27） |
| 策略层 | **本文 `service.health.*`** | 探测（何时算不健康）+ 自愈（要不要动手）+ 留痕 | 不新增执行原语：判断与节流之上，动作全部落到执行层 |

策略层不另立 systemctl 调用通道——自愈 restart 走 `ServiceProvider.DoAction(unit, "restart")`
同一函数，unit 名白名单、30s 超时、stderr 摘要透传全部继承（D1）。

## 关键决策

| # | 决策 | 选择 | 理由 |
|---|------|------|------|
| D1 | 架构归属 | 挂在 `service` provider 上（方法 `service.health.config / health.status / health.now`），自愈经 `DoAction(unit,"restart")` | 用户语义「探针/自愈是服务管理的策略层」；复用执行层校验与超时，不出现第二条 systemctl 通道（同 M3.5 D27 的口子纪律，但权限资源同为 services 面） |
| D2 | 探测执行位置 | **agent 侧**周期探测（本机回环 127.0.0.1 直达业务端口） | 僵死检测必须贴着本机；server 不可达时探测与自愈照常，事件缓冲待补报（D8）。server 只做配置权威与结果归集 |
| D3 | 配置权威与下发 | server DB `Setting` KV（`service_health.config.<agentID>`）为唯一事实源；agent 只持运行副本。server 在「保存配置」与「agent 注册上线」两个时机经现有 `rpc_request` 通道全量推送 | 配置类走 Setting KV 与拨测 D1/SMART 间隔同心智；不新增协议消息类型；agent 丢弃本地状态换幂等 |
| D4 | 探针类型 | `http`（GET 状态码**精确等于** expectStatus，默认 200）/ `tcp`（DialTimeout 连通即活）/ `systemd`（`systemctl is-active` 输出 `active`）；标准库零依赖 | cloudflared 场景只需要 http（/ready）；tcp 兜「进程监听还在」类；systemd 兜「systemd 自己能看见的挂」。均只读不写 |
| D5 | 判定语义 | 连续 `failThreshold`（默认 3）次失败才判 down 并触发自愈；任一次成功即恢复（recovered），计数清零 | 过滤瞬时抖动（同拨测 D5 边沿语义的最小实现）；恢复不发「半个周期内又挂」的噪音事件 |
| D6 | 自愈白名单 | per-agent `whitelist[]`（*.service 名单）随配置下发；**双端校验**：server 保存时拒绝 `heal=true` 且目标不在白名单的探针（报错文案指引改 `heal=false` 只告警）；agent 执行前再校验一道，非白名单只告警不动手。`cloudflared.service` 为首个候选 | M3.5 D27「白名单绝不能成为动任意 unit 的口子」纪律的延伸：自愈的破坏半径（无人值守重启）高于人工点击，必须显式点名；双端各挡一道同 D23 写操作纪律 |
| D7 | 重启风暴退避 | unit 级滑动窗口台账：`backoffWindowSec`（默认 600）内最多 `maxRestartsInWindow`（默认 3）次 restart（含失败）；窗口耗尽只告警不动作，每窗口至多一条 `heal_backoff` 事件 | 用户要求「X 分钟内最多 Y 次」；台账按 unit 记（两个探针指向同一 unit 共享额度）；失败重启同样占额度——防对坏 unit 反复硬敲 |
| D8 | 事件回传 | 不引新协议消息：agent 事件环形缓冲（256 条，满丢最旧）；server 归集循环每 30s 轮询 `service.health.status` 且 `drain=true`（拉取并清空）；仪表盘 GET 同一 RPC 但 `drain=false` 只窥不取（防浏览器读吞掉归集循环还没落审计的事件）；最新状态快照缓存进 `service_health.state.<agentID>` 供离线灰态 | 协议零改动（agent 无 server 方向 RPC 原语）；drain/peek 分口后事件唯一消费者是归集循环（审计/告警路径不被读路径竞态）；30s 归集延迟对「分钟级自愈链路」无感；轮询节奏固定不配置（探针间隔才是用户旋钮） |
| D9 | 审计与告警 | 六类事件（probe_down/probe_recovered/heal_restarted/heal_failed/heal_blocked/heal_backoff）分流：自愈三态每条落审计 `service_health_heal / service_health_heal_failed / service_health_heal_blocked`，username=`cockpit-agent`，resourceID=`{agentID}/{unit}`，details 带探针结果摘要；down/blocked/backoff → 告警（title 含 probeId，未读去重，blocked/backoff 为 warning、down 为 error），recovered 不告警；六类全发通知（`service_health.down / .up / .healed / .heal_failed / .heal_blocked / .backoff`，白名单显式启用） | 用户要求「每次自愈落审计」；recovered 与 backoff 不进审计（噪音类不记审计口径），但 backoff 必须「告警+通知」可见——额度耗尽意味着自愈已停摆，用户必须知道 |
| D10 | RBAC | 查看 `services:read`，配置写入（PUT）与立即探测 `services:write` | 权限点清单已有 services 面（storage/role.go resourceActions），策略层不另立权限点；server 侧 PUT/check 用 `userHasPerm` 硬校验 |
| D11 | 平台边界 | 自愈仅 Linux + systemd（`DetectSystemd()`）；非 systemd 平台带 `heal=true` 的配置在 agent 侧拒绝（server 侧校验先行）。http/tcp 探针 v1 同此边界 | DoAction 的 systemctl 路径即平台边界；Windows SCM/macOS launchd 的自愈后续按需扩（模型已在，只差执行器） |
| D12 | 配置校验 | `id` 非空且 agent 内唯一（`[a-z0-9-]{1,64}`）；`intervalSec` 5–3600（默认 30）；`timeoutSec` 1–30（默认 5）；`failThreshold` 1–60（默认 3）；`backoffWindowSec` 60–86400、`maxRestartsInWindow` 1–10；unit 名 `*.service` 白名单正则（复用 serviceUnitNameRe）；http target 必须 `http(s)://` URL；tcp target `host:port`（端口 1–65535，不含 scheme/路径） | 双端同规则（server 保存时、agent 装载时各一道，非法整包拒绝——配置是原子的，不做部分应用）；数值上限防「配置出一条 1s 间隔的自愈风暴」 |
| D13 | agent 重启的已知边界 | agent 进程重启后连续失败计数清零（重新攒 threshold），事件缓冲清空 | 探针状态不落盘是有意取舍：agent 重启本身罕见，落盘引入「陈旧计数自愈」的新风险面；server 侧告警未读去重保证重攒期间不重复轰炸 |

## 数据模型

### 探针配置（server 存储 = 下发载荷，全量替换语义）

```json
{
  "probes": [
    {
      "id": "cloudflared-ready",
      "type": "http",
      "target": "http://127.0.0.1:20241/ready",
      "expectStatus": 200,
      "intervalSec": 30,
      "timeoutSec": 5,
      "failThreshold": 3,
      "heal": true,
      "unit": "cloudflared.service",
      "backoffWindowSec": 600,
      "maxRestartsInWindow": 3
    }
  ],
  "whitelist": ["cloudflared.service"]
}
```

`unit` 在 `heal=true` 时必填；`type=systemd` 时 target 即 unit 本名（unit 字段
缺省取 target）。探针 target 本身不受白名单约束（观测自由），受约束的只有
自愈动作（D6）。

### 运行态（agent 持有，server 轮询取回）

```json
{
  "states": {
    "cloudflared-ready": {
      "status": "fail",
      "lastCheck": 1759000000,
      "consecutiveFails": 3,
      "lastError": "HTTP 502 (expect 200)",
      "lastHeal": { "time": 1759000001, "unit": "cloudflared.service",
                    "result": "restarted", "detail": "exit 0", "durationMs": 812 }
    }
  },
  "events": [
    { "time": 1759000001, "probeId": "cloudflared-ready",
      "kind": "heal_restarted", "unit": "cloudflared.service",
      "detail": "consecutiveFails=3, restart exit 0" }
  ]
}
```

事件 `kind` ∈ `probe_down / probe_recovered / heal_restarted / heal_failed /
heal_blocked / heal_backoff`。`states` 全量快照、`events` 为 drain 语义
（取回即清空）。

## RPC 契约

| 方法 | 参数 | 结果 | 说明 |
|------|------|------|------|
| `service.health.config` | `{probes:[…], whitelist:[…]}` | `{applied: N, rejected: [...]}` | 全量替换；校验失败**整包拒绝**（D12）；装载即驱动引擎循环 |
| `service.health.status` | `{}` | `{states, events}` | events drain；无配置时 `{states:{}, events:[]}` |
| `service.health.now` | `{id}` | `{probe, status, lastError}` | 立即执行一次指定探针（计入连续失败计数与边沿判定，等同调度触发） |

## REST API（挂在 `/api/agents/{id}/health`）

| 路由 | 权限 | 审计 | 说明 |
|------|------|------|------|
| `GET /api/agents/{id}/health` | services:read | 否 | `{config, states, online}`——在线时实时拉取 agent 运行态，离线回落缓存灰态 |
| `PUT /api/agents/{id}/health` | services:write | `service_health_config` | 校验（D12 + D6 白名单）→ 落 Setting KV → 在线则推送 → 返回 `{applied}` |
| `POST /api/agents/{id}/health/probes/{probeId}/check` | services:write | 否 | 转发 `service.health.now`（诊断动作，读语义不记审计） |

agent 离线时 PUT 仍成功（配置已落库，注册上线时补推）——响应带 `pushed:false`
提示。

## 首个接入用例：docker VM（192.168.5.5）cloudflared

1. **部署 agent**：VM 上安装 `cockpit-agent`（systemd 服务，随包注册
   `service` capability）；确认 `/services` 页能看到该机且 `cloudflared.service`
   在列。
2. **选定探针**：cloudflared 自带 metrics/ready 端点（`--metrics
   127.0.0.1:20241` 时为 `http://127.0.0.1:20241/ready`，隧道连通才回 200）。
   **不要用 systemd 探针做主探针**——「进程活着连接僵死」恰是 systemd 判
   active 的盲区，http /ready 才能看见业务真相；systemd 探针可作为第二道
   兜底（进程真死时 systemd 反而更快知道）。
3. **配置探针**：面板 /services → 健康探针 → 新建：`id=cloudflared-ready`、
   `type=http`、`target=http://127.0.0.1:20241/ready`、`intervalSec=30`、
   `failThreshold=3`（≈90s 检出）、`heal=true`、`unit=cloudflared.service`。
4. **白名单**：同一页把 `cloudflared.service` 加入该机自愈白名单——保存前
   server 会拒绝「heal=true 但不在白名单」的配置（D6），这是有意的中途检查点。
5. **观察一个真实故障**：手动 `kill -STOP` cloudflared 模拟僵死（进程在、
   /ready 不应答），确认：~90s 内探针转 fail → 审计出现
   `service_health_heal`（username=cockpit-agent）→ 服务被 restart → /ready
   回 200 → recovered 事件入告警。
6. **退避观察**：连续人为制造 4 次故障，第 4 次（窗口内 3 次额度用尽）应
   只见 `heal_backoff` 告警、无 restart 审计——重启风暴被挡住。
7. **cron 看门狗退役**：保留兜底 1–2 周，比对期间审计中 cockpit 版自愈
   成功率，确认稳定后移除 cron 条目。

## 实现与测试清单

| 批次 | 内容 | 测试 |
|------|------|------|
| M1（agent） | `internal/agent/rpc/service_health.go`：引擎 + 三探针执行器 + 自愈（白名单/退避/事件环）+ provider 三动作接线 | 假 Commander/httptest 注入：三类探针判定、阈值边沿、恢复清零、白名单拦截、退避窗口、事件 drain、整包校验拒绝、非 systemd 平台 heal 拒绝 |
| M2（server） | `internal/server/api_service_health.go`（API + 校验 + 推送）、`internal/server/service_health_scan.go`（30s 归集循环 + 审计 + 告警 + 通知 + 状态缓存）、注册上线补推钩子、events 常量、RBAC 路由 | API 校验矩阵（非法 target/白名单拒绝）、离线 PUT 落库不推送、归集循环事件→审计/告警/通知、缓存灰态、上线补推 |
| M3（web） | /services 页「健康探针」面板：探针列表（状态徽标/连续失败/上次自愈）、编辑表单、白名单编辑、立即探测按钮 | vitest 组件测试 |

验收边界（留给真机）：真实 cloudflared 僵死注入（上面用例第 5/6 步）、
跨机多 agent 归集节奏、GB 级无关（本能力无大数据路径）。
