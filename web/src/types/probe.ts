// 拨测配置（探测间隔可配置）
export interface ProbeConfig {
  interval_seconds: number
  fail_threshold: number
  disk_percent: number
  memory_percent: number
  cert_warn_days: number
  cert_info_days: number
  min_interval_seconds: number
  max_interval_seconds: number
  min_fail_threshold: number
  max_fail_threshold: number
  min_percent: number
  max_percent: number
}

// 拨测历史记录（心跳条数据源，见 docs/guide/probe-enhance-design.md M2）
export interface ProbeResult {
  id: string
  resourceType: string
  resourceId: string
  name: string
  status: string // up / down / degraded / active / valid / expiring / expired / error
  latencyMs: number
  message: string
  checkedAt: string
}

// 探针 agent（cockpit-probe-agent）目标观测快照（服务检测 agent B5/B6）：
// 每 agent+target 最新值，state 三态与 core/healthprobe 一致
export interface ProbeTargetSnapshot {
  id: string
  agentId: string
  target: string
  state: string // unknown（冷区）/ healthy / faulty
  since: string
  lastChecked: string
  lastError: string
  updatedAt: string
}

// 探针 agent 上报的故障窗口：endedAt 为 null 表示仍在故障中
export interface ProbeWindow {
  id: string
  agentId: string
  target: string
  startedAt: string
  endedAt: string | null
  lastError: string
  updatedAt: string
}
