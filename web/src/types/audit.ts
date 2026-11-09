// 审计日志域类型
export interface AuditLog {
  id: number
  user_id: string
  username: string
  action: string
  resource: string
  resource_id: string
  details: string
  ip: string
  user_agent: string
  status: string
  created_at: string
}

export interface AuditLogStats {
  total_logs: number
  today_logs: number
  failed_logs: number
  by_action: Record<string, number>
  by_resource: Record<string, number>
  by_user: Record<string, number>
}

export interface AuditLogsPage {
  data: AuditLog[]
  pagination: { total: number }
}
