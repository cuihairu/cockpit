import type { CronStatus, CronJobsResult, CronUserEntry, SystemdTimer, CronJob } from '@/types'
import type { ApiCtx } from './core'

// cron 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface CronMethods {
    cronUserQuery(user?: string): string
    getCronStatus(agentId: string, user?: string): Promise<CronStatus>
    getCronJobs(agentId: string, user?: string): Promise<CronJobsResult>
    getCronUsers(agentId: string): Promise<{ users: CronUserEntry[] }>
    getCronTimers(agentId: string): Promise<{ timers: SystemdTimer[] }>
    applyCronJob(agentId: string, job: CronJob, user?: string): Promise<{ name: string }>
    deleteCronJob(agentId: string, name: string, user?: string): Promise<void>
}

export const cronMethods: CronMethods = {
  cronUserQuery(this: ApiCtx, user?: string): string {

    return user ? `?user=${encodeURIComponent(user)}` : ''
  
  },

  async getCronStatus(this: ApiCtx, agentId: string, user?: string): Promise<CronStatus> {

    return this.client.get<unknown, CronStatus>(
      `/agents/${encodeURIComponent(agentId)}/cron/status${this.cronUserQuery(user)}`,
    )
  
  },

  async getCronJobs(this: ApiCtx, agentId: string, user?: string): Promise<CronJobsResult> {

    return this.client.get<unknown, CronJobsResult>(
      `/agents/${encodeURIComponent(agentId)}/cron/jobs${this.cronUserQuery(user)}`,
    )
  
  },

  async getCronUsers(this: ApiCtx, agentId: string): Promise<{ users: CronUserEntry[] }> {

    return this.client.get<unknown, { users: CronUserEntry[] }>(
      `/agents/${encodeURIComponent(agentId)}/cron/users`,
    )
  
  },

  async getCronTimers(this: ApiCtx, agentId: string): Promise<{ timers: SystemdTimer[] }> {

    return this.client.get<unknown, { timers: SystemdTimer[] }>(
      `/agents/${encodeURIComponent(agentId)}/cron/timers`,
    )
  
  },

  async applyCronJob(this: ApiCtx, agentId: string, job: CronJob, user?: string): Promise<{ name: string }> {

    return this.client.put<unknown, { name: string }>(
      `/agents/${encodeURIComponent(agentId)}/cron/jobs/${encodeURIComponent(job.name)}${this.cronUserQuery(user)}`,
      job,
    )
  
  },

  async deleteCronJob(this: ApiCtx, agentId: string, name: string, user?: string): Promise<void> {

    await this.client.delete(
      `/agents/${encodeURIComponent(agentId)}/cron/jobs/${encodeURIComponent(name)}${this.cronUserQuery(user)}`,
    )
  
  }
}
