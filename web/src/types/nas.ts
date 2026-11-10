// ========== NAS 存储观测（见 nas-design.md） ==========

// 存储池：mdadm（/proc/mdstat）、ZFS（zpool）、LVM（vgs）、DSM/TrueNAS 厂商池
export interface NasPool {
  name: string
  kind: 'mdadm' | 'zfs' | 'lvm' | 'dsm' | 'truenas' | 'omv'
  state: 'healthy' | 'degraded' | 'resync' | 'failed' | 'unknown'
  totalGB?: number
  usedGB?: number
  devices?: string[]
  detail?: string
  host?: string // 来源设备：网络 NAS（M2）填 target 名，本地观测为空
}

// 挂载点容量（df 白名单：真实块设备且 ≥1GB；DSM 卷来自 load_info）
export interface NasMount {
  device: string
  mountPath: string
  fsType: string
  totalGB?: number
  usedGB?: number
  host?: string
}

// 网络共享（SMB testparm / NFS exportfs / DSM 共享文件夹标 smb）
export interface NasShare {
  protocol: 'smb' | 'nfs'
  name: string
  path: string
  comment?: string
  hosts?: string
  host?: string
}

// nas.status 返回（agent 本机只读观测，M1 源 = linux）
export interface NasStatus {
  available: boolean
  source: string
  pools: NasPool[]
  mounts: NasMount[]
  shares: NasShare[]
}

// 巡检配置（server 定时 nas.status + 池/容量告警；间隔 0 = 关闭）
export interface NasScanConfig {
  scan_interval_seconds: number
  min: number
  max: number
  default: number
  usage_warn_percent: number
  usageMin: number
  usageMax: number
}
