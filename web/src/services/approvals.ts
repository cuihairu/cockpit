import axios from 'axios'
import { logger } from '@/utils/logger'

// 审批门 API（docs/guide/workflow-design.md M2d）：高危 Job 提交先落审批，
// 批准后才派发。等待态在 server 库里（重启不丢待批）；过期未决只能重新提交。

export type ApprovalStatus = 'pending' | 'approved' | 'rejected' | 'expired'

export interface Approval {
  id: string
  kind: string
  target: string
  actor: string
  status: ApprovalStatus
  /** 命令摘要（agent.exec 的 command 截断，列表视图免点开可辨） */
  summary?: string
  /** 完整入参（审批人须看到将执行什么） */
  parameters?: Record<string, unknown>
  expiresAt?: string
  decidedBy?: string
  decidedAt?: string
  /** 批准后派发出的 Job（对账用） */
  jobId?: string
  createdAt: string
}

const approvalsClient = axios.create({
  baseURL: '/api/approvals',
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json',
  },
})

approvalsClient.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => Promise.reject(error)
)

approvalsClient.interceptors.response.use(
  (response) => response.data,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token')
      localStorage.removeItem('username')
      window.location.href = '/login'
    }
    logger.error('Approvals API Error:', error)
    return Promise.reject(error)
  }
)

/** 审批台账（倒序；status 过滤，server 侧惰性清扫过期） */
export async function listApprovals(status?: ApprovalStatus): Promise<Approval[]> {
  const response = await approvalsClient.get<unknown, { approvals: Approval[] }>('', {
    params: status ? { status } : undefined,
  })
  return response.approvals ?? []
}

/** 批准并派发（重复决策/已过期 409） */
export async function approveApproval(id: string): Promise<Approval> {
  return approvalsClient.post<unknown, Approval>(`/${encodeURIComponent(id)}/approve`)
}

/** 驳回（终态，不派发） */
export async function rejectApproval(id: string): Promise<Approval> {
  return approvalsClient.post<unknown, Approval>(`/${encodeURIComponent(id)}/reject`)
}
