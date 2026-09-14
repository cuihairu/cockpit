# P1 方案设计：拨测增强（探测间隔可配置 + 通知渠道集成）

> 2026-09-14。P1「拨测增强」首期设计。前置：自动健康探测已上线（`internal/probe`，
> 每 5 分钟一轮硬编码；`internal/alert` 每小时基于库内状态生成告警；`internal/notification`
> 仅支持 Herald 单渠道）。方案参考：Uptime Kuma（间隔 UI 可配、心跳式状态、恢复通知）、
> Gatus（多渠道通知器抽象）。本文只做设计决策与接口定义。

## 现状与痛点

| 痛点 | 现状代码 | 后果 |
|------|----------|------|
| 探测间隔硬编码 | `server.go` `probe.NewRunner(s.db, 5*time.Minute)` | 想加密切只能改代码重启 |
| 服务宕机通知滞后 | probe 只落库；`alertCheckLoop` 每小时扫库生成告警 | 宕机最长 1 小时后才通知 |
| 通知渠道单一 | `notification.Client` 只会 POST Herald `/api/v1/events` | 无 Herald 服务即无通知 |
| 前端假表单 | `AlertSettings.tsx` 三个阈值输入无 onFinish，从未生效 | 误导用户 |
| 告警重复 | `alert.Generator.createAlertIfNotExists` 注释自认「简化处理，直接创建」 | 每小时重复告警（本次不修，见「不做」） |

## 关键决策

| # | 决策 | 选择 | 理由 |
|---|------|------|------|
| D1 | 探测间隔存储 | DB `Setting` KV 表 + REST + UI，非 config.yaml | Uptime Kuma 式体验：运行时可改、立即生效；设置类配置走库与既有 Settings 页面心智一致 |
| D2 | 间隔生效方式 | Runner 内 `atomic.Int64` 存秒数，循环从 `Ticker` 改为每轮 `time.After(r.Interval())` | `Ticker` 不响应间隔变化；`After` 每轮重读原子值，SetInterval 后下一轮天然生效 |
| D3 | 通知渠道抽象 | `notification.Channel` 接口 + `Service` 扇出；Herald 降级为渠道之一 | ntfy/webhook/Telegram 与 Herald 同为「HTTP POST 一条消息」，一个接口全覆盖；逐渠道失败隔离 |
| D4 | 渠道配置位置 | config.yaml（`notification.ntfy[]/webhook[]/telegram[]`） | token/secret 属敏感凭据，随既有 Herald 配置走 yaml + 环境变量扩展；UI 只读展示状态 + 测试按钮，不做在线编辑 |
| D5 | 宕机即时通知 | probe 侧状态边沿检测：连续 N 次（默认 2）失败才算 down，恢复即发 up | 探测一轮已知结果，无需等小时级扫描；N=2 过滤瞬时网络抖动（Uptime Kuma retry 语义的最小实现） |
| D6 | 事件过滤 | 沿用 `cfg.Events`（`events.<name>.enabled`），Service 统一过滤 | 与 Herald 既有事件开关语义一致，多渠道共享同一份开关 |

## 通知渠道设计

### 统一载荷与接口

```go
// Notification 渠道无关的通知载荷
type Notification struct {
    EventType    string    // service.down / service.up / certificate.expired / ...
    Title, Message string
    Level        string    // info / warning / error
    ResourceType string    // service / certificate / domain / agent
    ResourceID   string
    Time         time.Time
}

type Channel interface {
    Name() string                                  // herald / ntfy / webhook / telegram
    Send(ctx context.Context, n *Notification) error
}
```

`Service`：`NewService(cfg)` 启动时按配置构建启用的渠道切片；`Send(ctx, n)` 先过
`cfg.Events` 事件开关，再并发扇出全部渠道，逐渠道记日志、失败不影响其他渠道；
`SendNonBlocking` 供告警/探测等异步场景。`TestNotification(ctx)` 返回逐渠道成功/失败明细。

### 渠道实现（全部纯 HTTP，无 SDK 依赖）

| 渠道 | 请求 | 配置字段 |
|------|------|----------|
| herald（既有） | `POST {base_url}/api/v1/events` `{type, labels}` | `base_url` / `timeout` |
| ntfy | `POST {server}/{topic}`，body=消息文本，Header `Title` / `Priority` / `Tags=warning,cockpit`，可选 `Authorization: Bearer {token}` | `server` / `topic` / `token?` / `priority?` |
| webhook | `POST {url}` JSON `{event_type, title, message, level, resource_type, resource_id, time}`，可选 Header `X-Cockpit-Secret: {secret}` 供接收方鉴权 | `url` / `secret?` |
| telegram | `POST https://api.telegram.org/bot{token}/sendMessage` `{chat_id, text, parse_mode:"HTML"}` | `bot_token` / `chat_id` |

每类渠道配置为**列表**（可发多个 topic / 多个 webhook / 多个 chat）。凭据字段在
任何 API 响应中只返回目标摘要（如 topic 名、host），不回显原文。

### 配置示例（config.yaml 扩展，向后兼容）

```yaml
notification:
  enabled: true
  herald:
    base_url: http://herald:8080
  ntfy:
    - server: https://ntfy.example.com
      topic: cockpit-alerts
      # token: ${NTFY_TOKEN}
  webhook:
    - url: https://example.com/hooks/cockpit
      # secret: ${WEBHOOK_SECRET}
  telegram:
    - bot_token: ${TG_BOT_TOKEN}
      chat_id: "123456789"
  events:
    service-down:  { type: service.down,  enabled: true }
    service-up:    { type: service.up,    enabled: true }
    cert-expired:  { type: certificate.expired, enabled: true }
```

## 探测间隔可配置

- **存储**：`storage.Setting{Key, Value}` 通用 KV 表（`probe.interval_seconds`，
  默认 300）；`GetSetting/SetSetting`（upsert）
- **范围校验**：`[30, 3600]` 秒。下限防自 DDoS（探测都是 server 主动出站，30s
  已是高频），上限 1h（再长不如关掉拨测）
- **API**（新 `api_probe.go`，`registerProbeAPI(mux)`，JWT 保护）：
  - `GET /api/probe/config` → `{interval_seconds}`
  - `PUT /api/probe/config` `{interval_seconds}` → 校验 → 落库 + `Runner.SetInterval` + 审计
- **Runner 改造**：`NewRunner(db, interval, notifier)`；`SetInterval(time.Duration)`
  原子更新；循环 `Ticker` → `time.After(Interval())`（间隔变化下一轮生效）
- **启动顺序**：`NewServer` 建 Runner（config 默认值）→ `Start` 时读 DB 覆盖

## 服务状态边沿通知（probe 即时告警）

- Runner 持有 `states map[resourceKey]*state{failCount, notified}`（仅探测循环
  goroutine 访问，无锁）；
- service 探测结果为 down：`failCount++`，首次达到阈值（2）→ 发 `service.down`
- 结果恢复 up 且 `notified` → 发 `service.up` 恢复通知
- 仅 service 类型做边沿通知：domain/cert 变化缓慢，`alert.Generator` 小时级扫描已覆盖；
  probe 通知**不写 alert 表**（避免与 Generator 重复），通知渠道按 `cfg.Events` 过滤
  （`service.up` 未启用则恢复不通知）

## Web UI（Settings 页改造）

`AlertSettings.tsx`（假表单）重做为「拨测与通知」卡片：

- **拨测**：探测间隔输入（30–3600 秒）+ 保存 → `PUT /api/probe/config`
- **通知渠道**：只读状态列表（类型 + 目标摘要 + 启用数，来源 `GET /api/notification/status`）；
  未配置渠道时提示 yaml 配置方法；「发送测试通知」按钮 → `POST /api/notification/test`，
  展示逐渠道结果
- 删除三个从未生效的阈值假表单（证书 7/30 天等阈值后续做成真配置时再回来）

## 不做（后续项）

- 通知渠道 UI 在线编辑（yaml + 环境变量已够个人场景；要做需先解决凭据加密存储）
- 心跳条式状态历史 UI（Uptime Kuma 式；需 probe 结果历史表，独立立项）
- `alert.Generator` 重复告警去重（`createAlertIfNotExists` 每次直接创建）——独立修复
- 告警阈值（磁盘 80% / 内存 85% / 证书 7/30 天）配置化

## 分期落地

### M1 —— 间隔可配置 + 多渠道通知（2026-09-14 完成）

- [x] notification：`Notification`/`Channel`/`Service` + ntfy/webhook/telegram 实现（herald 适配保留）
- [x] config：`ntfy[]/webhook[]/telegram[]` 配置结构 + 默认值 + 测试
- [x] probe：Runner 动态间隔（SetInterval + `time.After` 循环）+ 服务边沿通知（连续 2 次失败防抖）
- [x] storage：`Setting` KV 表
- [x] server：`api_probe.go`（GET/PUT config + notification/status + notification/test）+ 路由注册 + 审计（`probe` / `notification` 资源类型）
- [x] web：Settings「告警设置」Tab 重做为拨测与通知（间隔表单 + 渠道状态 + 测试通知 + 逐渠道结果）；删除三个从未生效的阈值假表单
- [x] 测试：notification 四渠道 httptest（含白名单过滤、失败隔离、凭据不泄漏）、probe 动态间隔夹紧与边沿通知、server API 全分支、storage KV

## 参考

- [Uptime Kuma](https://github.com/louislam/uptime-kuma)——间隔 UI 配置、retry 防抖、恢复通知
- [Gatus](https://github.com/TwiN/gatus)——多通知器抽象与告警事件命名
- 内部：[协议定义](./protocol.md)、`todo.md` P1 拨测增强条目、`internal/probe/runner.go`
