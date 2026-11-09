// 通用与 API 骨架类型
// 资源类型定义
export type ResourceType =
  | 'compute-instance'
  | 'container-service'
  | 'domain'
  | 'certificate'
  | 'service'
  | 'gateway'
  | 'ci-service'
  | 'storage'

// API 响应
export interface ApiResponse<T> {
  data?: T
  error?: string
  message?: string
}

// 分页参数
export interface PaginationParams {
  current: number
  pageSize: number
}

// 分页响应（与后端 api.go 返回格式一致）
export interface PaginatedResponse<T> {
  data: T[]
  total: number
  page: number
  pageSize: number
  totalPages: number
}

export interface StatusResponse {
  services: { running: number; down: number; unknown: number }
  domains: { valid: number; expiring: number }
  certificates: { valid: number; expiring: number }
  infrastructure: { total: number; online: number }
  /** Agent 过期判定阈值（分钟，0 = 服务端自动过期已关闭），见 D-2026-10-08-3 */
  agentExpireMinutes?: number
}
