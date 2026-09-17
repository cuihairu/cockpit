# 磁盘健康（SMART）设计

## 背景

对个人基础设施而言，磁盘是「坏了才发现」损失最大的部件：NAS 用户的第一痛点是数据盘静默损坏。SMART 指标（重映射扇区、待定扇区、介质错误）在盘真正死亡前通常有数周到数月的征兆窗口，及时告警可以换来从容迁移。

Cockpit 现状：

- `hardware-monitor` capability（`internal/agent/detector/hardware.go`）**已经探测 smartctl** 并在 metadata 置 `smart: true`，但没有对应的数据面 provider——探测是"有这个工具"，取不到读数；
- server 侧巡检循环（`internal/server/drift_scan.go`）与告警去重（`internal/alert/generator.go` 的 `CheckXxx` 族）模式成熟，可整体复刻。

本任务给 M1 补上数据面：**单盘健康只读观测 + 定时巡检 + 真去重告警 + 跨主机总览页**。

## 决策

- **D1 复用 hardware-monitor capability，不新建 detector**。`metadata.smart` 已由 `HardwareDetector` 探测（`LookPath` + `smartctl --version` 可运行）。agent 侧 provider 在 `providers.go` 新增 `case "hardware-monitor"` 注册；server 侧巡检按 `metadata.smart == true` 过滤（同一 capability 下只有温度/UPS 的主机自动跳过）。
- **D2 RPC 单方法 `smart.status`**，只读、无入参、校验面为零（与 `overlay.status` 同风格）。
- **D3 数据面两条命令**：
  - `lsblk --json --paths --noheadings -b -o NAME,TYPE,SIZE,MODEL,SERIAL` 发现物理盘（过滤 `TYPE == "disk"`，排除 loop/ram/分区）；`--paths` 让 NAME 直接是 `/dev/sda` 形式，无需拼接；
  - 每盘 `smartctl --json -H -A /dev/X` 读健康与属性。
  - 超时：lsblk 5s、单盘 smartctl 10s、整次 RPC 整体 30s；单盘失败不影响其余盘（与 overlay D5 同则）。
- **D4 字段白名单**。smartctl 的 JSON 输出很大，provider 只解析并输出以下字段，绝不透传完整 JSON：
  - `smart_status.passed` → 健康结论；
  - `temperature.current` → 温度（smartctl 7.x 对 ATA/NVMe 统一的顶层字段）；
  - `ata_smart_attributes.table[]` 中 id=5（Reallocated_Sector_Ct）、id=197（Current_Pending_Sector）、id=9（Power_On_Hours）的 `raw.value`（注意 smartctl 不同版本 raw.value 可能是数字或字符串，解析需兼容两形态）；
  - NVMe（`device.type == "nvme"`）：`nvme_smart_health_information_log` 的 `power_on_hours`、`media_errors`、`critical_warning`、`percentage_used`。
- **D5 健康三态**：`passed` / `failed`（`smart_status.passed == false`，即盘已报 FAILING_NOW）/ `unknown`（权限不足、盘不支持 SMART、smartctl 读数失败——`error` 字段带原因给前端展示）。`unknown` 不触发告警（环境问题不是盘的问题）。
- **D6 权限现实**：smartctl 读盘普遍需要 root。cockpit agent 默认以 root systemd 服务运行；非 root 部署时所有盘会标 `unknown` + 权限错误说明，功能不炸。文档与页面提示这一点。
- **D7 REST 纯转发不落库**：`GET /api/agents/{id}/smart/status` 透传 RPC 结果，浏览类不记审计（与 overlay D8/D9 同则）；巡检配置走全局路径 `GET/PUT /api/smart/config`（仿 `/api/drift/config`，巡检间隔非 per-agent）。
- **D8 巡检循环复刻 drift 模式**：1min ticker + lastScan 对比；Setting 键 `smart.scan_interval_seconds`，默认 3600，min 300，max 86400，`0 = 关闭`。范围 = `ListByCapability("hardware-monitor")` ∩ `metadata.smart` ∩ 在线。
- **D9 告警真去重**：`alert.Generator.CheckDiskHealth(agentID, hostname, issues)`，同主机一条告警（与 `CheckDriftScan` 同构）。分级：任一盘 `failed` → `error` 级；仅重映射/待定/介质扇区 > 0 → `warning` 级；`unknown` 只记日志。
- **D10 前端 `/disk`「磁盘健康」页**：跨 agent 总览一屏（主机、设备、型号、容量、温度、通电时长、重映射/待定/介质扇区、健康徽标），Collapse 按主机展开单机详情；页内巡检间隔设置（InputNumber + 保存，数据来自 `/api/smart/config`）。菜单名「磁盘健康」，`HddOutlined`，置于「组网观测」之后。

## 数据结构

`smart.status` 返回：

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

- `available: false` 表示 smartctl 缺失（与 metadata.smart 不符的环境，巡检与前端均跳过，不报错）；
- 数值字段取不到时省略（omitempty），前端展示 `—`；
- `serial` 用于跨巡检识别同一块盘（盘序号可能变），不做保密处理（本产品单管理员）。

## 实现切分（M1，本任务）

1. **docs**：本文档；
2. **agent**：`rpc/smart_parse.go`（纯函数解析 lsblk/smartctl JSON）+ `rpc/smart_provider.go`（`SmartProvider`，Commander 可注入）+ `providers.go` 注册 + 测试；
3. **server**：`api_smart.go`（status 转发 + config GET/PUT）+ `smart_scan.go`（巡检循环）+ `alert` 新增 `CheckDiskHealth` + 测试；
4. **web**：类型 + api 方法 + `/disk` 页 + 菜单/路由 + `tsc`/build 通过。

## 测试要点

- 解析器：ATA passed / failed、NVMe、`raw.value` 数字与字符串两形态、缺 `smart_status`（unknown）、全字段缺失不 panic；
- provider：fake Commander 注入；lsblk 空/坏 JSON；smartctl 缺失 → `available: false`；单盘 smartctl 失败 → 该盘 unknown、其余盘正常；
- server：Setting 边界校验；巡检过滤（无 smart 标志的 agent 跳过）；告警注入走真 DB（仿 drift 巡检测试）；
- 前端：`tsc --noEmit` + vite build。
