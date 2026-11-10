// ========== 服务健康探针与自愈（见 docs/guide/service-health-design.md） ==========

// 单条探针定义（server 存储 = 下发载荷；数值字段 0 视为缺省）
export interface HealthProbe {
  id: string
  type: 'http' | 'tcp' | 'systemd'
  target: string
  expectStatus?: number // http 专用，0 视为 200
  intervalSec?: number // 5-3600，默认 30
  timeoutSec?: number // 1-30，默认 5
  failThreshold?: number // 1-60，默认 3
  heal?: boolean // 自愈开关（目标必须在白名单）
  unit?: string // 自愈目标；systemd 型缺省取 target
  backoffWindowSec?: number // 60-86400，默认 600
  maxRestartsInWindow?: number // 1-10，默认 3
}

// 探针配置记录（含审计元数据）
export interface HealthConfigRecord {
  probes: HealthProbe[]
  whitelist: string[]
  updatedAt?: number
  updatedBy?: string
}

// 故障窗口（agent 侧 core/healthprobe.Window 镜像，P6 三语义）：startedAt/
// endedAt 为 RFC3339 串；endedAt 为零值串（0001-01-01T00:00:00Z）⟺ 仍在
// 故障中（time.Time 的 omitempty 无效，前后端按零值判开合）
export interface HealthWindow {
  target: string
  semantics?: 'liveness' | 'readiness' | 'heartbeat'
  startedAt: string
  endedAt?: string
  lastError?: string
}

// 最近一次自愈决策
export interface HealthHealRec {
  time: number
  unit: string
  result: string // restarted / failed / blocked / backoff
  detail: string
  durationMs: number
}

// 单探针运行态
export interface HealthStateEntry {
  status: string // ok / fail / unknown（最近一次探测结果，非判定态）
  semantics?: 'liveness' | 'readiness' | 'heartbeat' // 判定语义（P6）
  lastCheck?: number
  consecutiveFails?: number
  lastError?: string
  lastHeal?: HealthHealRec | null
  openWindow?: HealthWindow | null // 非空 ⟺ 判定期故障中
  recentWindows?: HealthWindow[] // 近期已关闭故障窗口（agent 侧环形上限 64）
}

// GET /agents/{id}/health 概览（在线实时拉取，离线回落缓存灰态）
export interface HealthOverview {
  config: HealthConfigRecord | null
  states: Record<string, HealthStateEntry>
  online: boolean
}

// PUT 保存返回（离线保存成功但 pushed=false，注册上线补推）
export interface HealthConfigSaveResult {
  applied: number
  pushed: boolean
}

// 立即探测返回（service.health.now 透传）
export interface HealthProbeCheckResult {
  probe: string
  status: string
  consecutiveFails?: number
  lastError?: string
}
