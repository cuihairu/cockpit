// ========== DDNS（动态域名解析，见 ddns-design.md） ==========

// 一条 DDNS 配置（server 落库；巡检时让绑定 agent 探测公网 IP 并写 Cloudflare）
export interface DDNSConfig {
  id: number
  agentId: string
  zoneId: string
  zoneName: string
  recordName: string // 完整记录名，如 home.example.com
  type: 'A' | 'AAAA'
  enabled: boolean
  lastIP: string
  lastStatus: 'never' | 'ok' | 'failed'
  lastError: string
  checkedAt: number // Unix 秒，0 = 从未
  createdAt: string
  updatedAt: string
}

// DDNS 配置创建/更新入参
export interface DDNSConfigInput {
  agentId: string
  zoneId: string
  zoneName: string
  recordName: string
  type: 'A' | 'AAAA'
  enabled: boolean
}

// 立即检查结果
export interface DDNSCheckResult {
  ip: string
  changed: boolean
  status: string
  error: string
}

// 巡检配置（server 定时同步；0 = 关闭）
export interface DDNSScanConfig {
  scan_interval_seconds: number
  min: number
  max: number
  default: number
}
