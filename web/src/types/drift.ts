// 防漂移检测（见 docs/guide/drift-design.md）
export interface DriftCheckItem {
  kind: 'nginx' | 'traefik' | 'cron' | 'stack'
  name: string
  status: 'ok' | 'drifted' | 'missing' | 'no_baseline' | 'error' | 'none'
  baseline_sha: string
  current_sha: string
}

export interface DriftCheckResult {
  items: DriftCheckItem[]
  checked_at: number
}

// 漂移 diff：单对象两侧全文（cron 侧为美化后 JSON；见 drift-design.md M3）
export interface DriftDiffResult {
  expected: string
  current: string
  baseline_updated_at: number
}

// 巡检配置（server 定时 drift.check + 漂移告警；0 = 关闭）
export interface DriftScanConfig {
  scan_interval_seconds: number
  min: number
  max: number
  default: number
}

// CMDB 一致性（M6，drift-design.md D28-D30）：inventory 声明 vs agent 实报
export type ConsistencyStatus = 'ok' | 'mismatch' | 'unregistered' | 'undeclared'

export interface ConsistencyFieldMismatch {
  field: 'hostname' | 'ip'
  declared: string
  actual: string
}

export interface ConsistencyAgentFacts {
  hostname?: string
  ip?: string
}

export interface AgentConsistency {
  id: string
  status: ConsistencyStatus
  declared?: ConsistencyAgentFacts
  actual?: ConsistencyAgentFacts
  mismatch?: ConsistencyFieldMismatch[]
}

export interface ConsistencyReport {
  agents: AgentConsistency[]
  summary: {
    total: number
    ok: number
    mismatch: number
    unregistered: number
    undeclared: number
  }
}
