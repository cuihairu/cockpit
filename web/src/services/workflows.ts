import axios from 'axios'
import { logger } from '@/utils/logger'

// Workflow 编排 API（docs/guide/workflow-design.md）：线性步骤链，每步即
// 一条真实 Job（agent.exec）。定义 CRUD + run 控制（run/取消）；run 状态机
// running → success | failed | cancelled。

export interface WorkflowStep {
  type: 'agent.exec'
  /** 单目标主机（与 targets 互斥，二选一） */
  target?: string
  /** 多目标扇出（M2b：逐台完整执行，≤20 台，与 target 互斥） */
  targets?: string[]
  parameters: {
    /** 步骤名，同一 workflow 内唯一（≤64） */
    name: string
    /** 待执行命令（shell 形态，≤16KB） */
    command: string
    /** 执行超时秒（1-300，缺省 60） */
    timeout_s?: number
    /** 失败是否继续后续步骤（缺省 false=失败即停） */
    continue_on_error?: boolean
    /** 失败重试次数（0-3，缺省 0；固定间隔 5s） */
    retry?: number
  }
}

export interface WorkflowDef {
  id: string
  name: string
  description?: string
  steps: WorkflowStep[]
  createdBy: string
  createdAt: string
  updatedAt: string
}

/** 扇出步单台结果（M2b F6）：Status 同 JobStatus 词表 */
export interface WorkflowRunStepTarget {
  agentId: string
  status: string
  jobId?: string
  attempts: number
  stopReason?: string
}

export interface WorkflowRunStep {
  name: string
  type: string
  target: string
  /** 扇出步逐台结果（M2b）；单目标步无此键 */
  targets?: WorkflowRunStepTarget[]
  status: string
  jobId?: string
  attempts: number
  stopReason?: string
  continueOnError?: boolean
}

export type WorkflowRunStatus = 'running' | 'success' | 'failed' | 'cancelled'

export interface WorkflowRun {
  id: string
  workflowId: string
  workflowName: string
  status: WorkflowRunStatus
  actor: string
  steps: WorkflowRunStep[]
  createdAt: string
  finishedAt?: string
}

export interface WorkflowRunListFilters {
  status?: WorkflowRunStatus
}

const workflowClient = axios.create({
  baseURL: '/api',
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json',
  },
})

workflowClient.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => Promise.reject(error)
)

workflowClient.interceptors.response.use(
  (response) => response.data,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token')
      localStorage.removeItem('username')
      window.location.href = '/login'
    }
    logger.error('Workflow API Error:', error)
    return Promise.reject(error)
  }
)

/** 定义列表 */
export async function listWorkflows(): Promise<WorkflowDef[]> {
  const response = await workflowClient.get<unknown, { workflows: WorkflowDef[] }>('/workflows')
  return response.workflows ?? []
}

/** 定义详情 */
export async function getWorkflow(id: string): Promise<WorkflowDef> {
  return workflowClient.get<unknown, WorkflowDef>(`/workflows/${encodeURIComponent(id)}`)
}

/** 创建定义（≤20 步，步骤名唯一） */
export async function createWorkflow(req: {
  name: string
  description?: string
  steps: WorkflowStep[]
}): Promise<WorkflowDef> {
  return workflowClient.post<unknown, WorkflowDef>('/workflows', req)
}

/** 更新定义（active run 在途时 409） */
export async function updateWorkflow(
  id: string,
  req: { name: string; description?: string; steps: WorkflowStep[] },
): Promise<WorkflowDef> {
  return workflowClient.put<unknown, WorkflowDef>(`/workflows/${encodeURIComponent(id)}`, req)
}

/** 删除定义（历史 run 台账保留） */
export async function deleteWorkflow(id: string): Promise<void> {
  await workflowClient.delete(`/workflows/${encodeURIComponent(id)}`)
}

/** 触发 run（W8：同定义重入 409）——返回 running 视图，终态经轮询读取 */
export async function runWorkflow(id: string): Promise<WorkflowRun> {
  return workflowClient.post<unknown, WorkflowRun>(`/workflows/${encodeURIComponent(id)}/run`)
}

/** 某定义的 run 台账（倒序） */
export async function listWorkflowRuns(
  id: string,
  filters?: WorkflowRunListFilters,
): Promise<WorkflowRun[]> {
  const response = await workflowClient.get<unknown, { runs: WorkflowRun[] }>(
    `/workflows/${encodeURIComponent(id)}/runs`,
    { params: filters },
  )
  return response.runs ?? []
}

/** run 详情（含步骤快照） */
export async function getWorkflowRun(runId: string): Promise<WorkflowRun> {
  return workflowClient.get<unknown, WorkflowRun>(`/workflow-runs/${encodeURIComponent(runId)}`)
}

/** 取消 run：停止推进后续步骤，当前在途步骤等它自然结束（W3） */
export async function cancelWorkflowRun(runId: string): Promise<WorkflowRun> {
  return workflowClient.post<unknown, WorkflowRun>(
    `/workflow-runs/${encodeURIComponent(runId)}/cancel`,
  )
}

/** run 是否终态（轮询停止条件） */
export function isRunTerminal(status: WorkflowRunStatus): boolean {
  return status !== 'running'
}

/** 步骤运行态是否终态 */
export function isStepTerminal(status: string): boolean {
  return status === 'success' || status === 'failed' || status === 'cancelled'
}
