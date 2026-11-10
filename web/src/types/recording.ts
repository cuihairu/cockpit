// 会话录制与服务端备份域类型
// 会话录制元数据（内容在 server 侧 .cast 文件，见 recording-design.md）
export interface TerminalRecording {
  id: number
  sessionId: string
  username: string
  agentId: string
  host: string
  port: number
  protocol: string
  /** 录制内容形态：cast（asciinema 终端流，默认）| guac（Guacamole 会话流） */
  format?: string
  startedAt: string
  durationMs: number // 0 = 进行中
  bytes: number
}

// 录制配置（recording M2 D19：开关/保留/异地归档目标；remote_dest 空 = 不推送）
export interface RecordingConfig {
  enabled: boolean
  retention_days: number // 0 = 永久
  remote_dest?: string // rclone remote:path
  rclone_available?: boolean // server 主机是否装了 rclone（实时探测）
  max_retention_days: number
}

// Server 自身数据库备份（VACUUM INTO 产物，目录即事实源，见 server-backup-design.md）
export interface ServerBackupFile {
  name: string
  size: number
  modTime: string
}

export interface ServerBackupConfig {
  interval_hours: number
  retention_days: number
  remote_dest?: string // rclone remote:path（空 = 不推送异地）
  rclone_available?: boolean // server 主机是否装了 rclone（实时探测）
  max_interval_hours: number
  max_retention_days: number
}
