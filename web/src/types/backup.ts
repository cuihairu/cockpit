// ========== 备份管理 ==========

// 备份任务配置（备份在 Agent 侧本地执行，见 docs/guide/backup-design.md）
export interface BackupConfig {
  id: number
  agent_id: string
  name: string // 备份文件名前缀
  sources: string[] // 源路径列表
  sources_ok?: boolean // S10：损坏 sources JSON 时 false，视图显式暴露
  sources_err?: string
  dest_dir: string
  schedule: string // manual / daily@HH:mm / every:Nh
  retention: number // 保留份数，0 = 不清理
  remote_dest?: string // rclone 远端目标 remote:path，空 = 不启用异地（M2 D20）
  pre_hook?: string // 打包前执行的数据库热备命令，空 = 不执行（M3 D26）
  enabled: boolean
  last_run_at: number
  next_run_at: number // manual 恒为 0
  last_status: string // "" / running / success / failed
  created_at: number
}

// 备份配置创建/更新请求
export interface BackupConfigInput {
  agent_id: string
  name: string
  sources: string[]
  dest_dir: string
  schedule: string
  retention: number
  remote_dest?: string // 空 = 不启用异地
  pre_hook?: string // 打包前执行的数据库热备命令，空 = 不执行
  enabled: boolean
}

// 备份单次运行记录
export interface BackupRun {
  id: number
  configId: number
  taskId: string
  status: 'running' | 'success' | 'failed' | 'timeout'
  file: string
  size: number
  error?: string
  remoteStatus?: '' | 'ok' | 'failed' // 异地推送结果，空 = 未启用（M2 D20）
  remoteError?: string
  startedAt: number
  finishedAt: number
}

// Agent 备份目录下的产物文件
export interface BackupFile {
  name: string
  size: number
  mtime: number
}

// Agent 侧任务状态（restore 等，server 转发不落库）
export interface BackupTask {
  taskId: string
  action: 'backup' | 'restore'
  status: 'running' | 'success' | 'failed'
  error?: string
  file?: string
  size?: number
  startedAt?: number
  finishedAt?: number
  log?: string
}
