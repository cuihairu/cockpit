// ========== 磁盘健康（SMART，见 disk-health-design.md） ==========

// smart.status devices[] 元素（白名单字段，取不到的省略）
export interface SmartDevice {
  name: string
  model?: string
  serial?: string
  sizeBytes?: number
  health: 'passed' | 'failed' | 'unknown'
  temperatureC?: number
  powerOnHours?: number
  reallocatedSectors?: number
  pendingSectors?: number
  mediaErrors?: number
  percentUsed?: number
  error?: string
}

// smart.status 返回
export interface SmartStatus {
  available: boolean
  devices: SmartDevice[]
}

// 巡检配置（server 定时 smart.status + 磁盘告警；0 = 关闭）
export interface SmartScanConfig {
  scan_interval_seconds: number
  min: number
  max: number
  default: number
}
