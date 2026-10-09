import axios from 'axios'
import { logger } from '@/utils/logger'

// 执行 Job API（docs/guide/jobs-design.md + workflow-design.md W1）：创建
// 异步——POST 立即返回 201 pending 视图，server 后台派发，终态经台账轮询
// 读取。pending 可取消（W3：派发前可撤）。

export type JobStatus = 'pending' | 'running' | 'success' | 'failed' | 'cancelled'

export interface JobParameters {
  /** agent.exec：待执行命令（shell 形态，≤16KB） */
  command?: string
  /** 执行超时秒（1-300，缺省 60） */
  timeout_s?: number
  [key: string]: unknown
}

export interface Job {
  id: string
  type: string
  target: string
  actor: string
  status: JobStatus
  parameters?: JobParameters
  /** 执行输出尾部（≤64KB，仅终态） */
  output?: string
  /** 非零退出码（仅终态） */
  exitCode?: number
  error?: string
  /** 所属 Workflow run（编排步骤 Job 才有） */
  workflowRunId?: string
  createdAt: string
  startedAt?: string
  finishedAt?: string
}

export interface JobCreateRequest {
  type: 'agent.exec'
  target: string
  parameters: JobParameters
}

export interface JobListFilters {
  status?: JobStatus
  target?: string
  type?: string
  workflow_run_id?: string
}

const jobsClient = axios.create({
  baseURL: '/api/jobs',
  timeout: 30000,
  headers: {
    'Content-Type': 'application/json',
  },
})

jobsClient.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => Promise.reject(error)
)

jobsClient.interceptors.response.use(
  (response) => response.data,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token')
      localStorage.removeItem('username')
      window.location.href = '/login'
    }
    logger.error('Jobs API Error:', error)
    return Promise.reject(error)
  }
)

/** 最近 Job 列表（倒序，server 固定 limit 50；可按状态/目标/类型/run 过滤） */
export async function listJobs(filters?: JobListFilters): Promise<Job[]> {
  const response = await jobsClient.get<unknown, { jobs: Job[] }>('', { params: filters })
  return response.jobs ?? []
}

/** 创建 Job（异步：返回 pending 视图，终态经台账轮询） */
export async function createJob(req: JobCreateRequest): Promise<Job> {
  return jobsClient.post<unknown, Job>('', {
    type: req.type,
    target: req.target,
    parameters: req.parameters,
  })
}

/** 单条详情 */
export async function getJob(id: string): Promise<Job> {
  return jobsClient.get<unknown, Job>(`/${encodeURIComponent(id)}`)
}

/** 取消 pending Job（已派发/终态返回 409） */
export async function cancelJob(id: string): Promise<Job> {
  return jobsClient.post<unknown, Job>(`/${encodeURIComponent(id)}/cancel`)
}
