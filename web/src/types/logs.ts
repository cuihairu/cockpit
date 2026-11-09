// 远程日志查询（见 docs/guide/logs-design.md）
export interface LogsStatus {
  journalctl: boolean
  docker: boolean
}

export interface LogsSources {
  systemd: string[]
  docker: string[]
}

export interface LogsQuery {
  type: 'systemd' | 'docker'
  source: string
  tail: number
  since_minutes: number
  grep: string
}

export interface LogsQueryResult {
  lines: string
  truncated: boolean
}

// 跨机日志联邦检索（见 docs/guide/logs-design.md M3，D11-D16）
export interface LogsSearchQuery {
  type: 'systemd' | 'docker'
  source: string
  tail?: number
  since_minutes?: number
  grep?: string
  agents?: string[]
}

export interface LogsSearchItem {
  agentId: string
  hostname: string
  ok: boolean
  truncated?: boolean
  lines?: string
  error?: string
}

export interface LogsSearchSkipped {
  agentId: string
  reason: 'offline' | 'no-logs'
}

export interface LogsSearchResult {
  results: LogsSearchItem[]
  skipped: LogsSearchSkipped[]
}
