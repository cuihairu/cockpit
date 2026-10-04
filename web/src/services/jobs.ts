import axios from 'axios'
import { logger } from '@/utils/logger'

// 执行 Job API（docs/guide/jobs-design.md）：创建即执行（同步 RPC 下发），
// 终态 success/failed 回写；running 为瞬态。最小闭环仅 agent.exec。

export type JobStatus = 'pending' | 'running' | 'success' | 'failed'

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
  createdAt: string
  startedAt?: string
  finishedAt?: string
}

export interface JobCreateRequest {
  type: 'agent.exec'
  target: string
  parameters: JobParameters
}

const jobsClient = axios.create({
  baseURL: '/api/jobs',
  timeout: 60000, // 创建即执行：最长 300s 命令经同步 RPC 等待，这里放宽到网关常见 60s
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

/** 最近 Job 列表（倒序，server 固定 limit 50） */
export async function listJobs(): Promise<Job[]> {
  const response = await jobsClient.get<unknown, { jobs: Job[] }>('')
  return response.jobs ?? []
}

/** 创建并执行 Job（返回终态视图；agent 侧最长等 timeout_s 秒） */
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
