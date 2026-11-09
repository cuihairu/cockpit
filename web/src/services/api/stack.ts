import type { StackView, StackInfo, StackDetail, StackCompose, StackComposeSaveResponse, StackTaskStartResponse, StackDeployment, StackTask, StackGitStatus, StackGitPullResponse } from '@/types'
import type { ApiCtx } from './core'

// stack 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface StackMethods {
    getStacks(): Promise<{ stacks: StackView[]; agentInfo?: Record<string, StackInfo> }>
    getStackDetail(agentId: string, name: string): Promise<StackDetail>
    getStackCompose(agentId: string, name: string): Promise<StackCompose>
    saveStackCompose(agentId: string, name: string, data: { compose: string; env?: string }): Promise<StackComposeSaveResponse>
    stackAction(agentId: string, name: string, action: 'up' | 'down' | 'restart' | 'pull'): Promise<StackTaskStartResponse>
    getStackHistory(agentId: string, name: string, limit?: number): Promise<{ deployments: StackDeployment[] }>
    getStackTask(agentId: string, taskId: string): Promise<StackTask>
    followStackTask(agentId: string, taskId: string, signal: AbortSignal): Promise<Response>
    getStackLogs(agentId: string, name: string, opts?: { service?: string; tail?: number }): Promise<{ logs: string }>
    deleteStack(agentId: string, name: string): Promise<StackTaskStartResponse>
    getStackGit(agentId: string): Promise<StackGitStatus>
    pullStackGit(agentId: string, data?: { remote?: string; branch?: string }): Promise<StackGitPullResponse>
}

export const stackMethods: StackMethods = {
  async getStacks(this: ApiCtx): Promise<{ stacks: StackView[]; agentInfo?: Record<string, StackInfo> }> {

    return this.client.get<unknown, { stacks: StackView[]; agentInfo?: Record<string, StackInfo> }>('/stacks')
  
  },

  async getStackDetail(this: ApiCtx, agentId: string, name: string): Promise<StackDetail> {

    return this.client.get<unknown, StackDetail>(`/stacks/agents/${agentId}/${name}`)
  
  },

  async getStackCompose(this: ApiCtx, agentId: string, name: string): Promise<StackCompose> {

    return this.client.get<unknown, StackCompose>(`/stacks/agents/${agentId}/${name}/compose`)
  
  },

  async saveStackCompose(this: ApiCtx, agentId: string, name: string, data: { compose: string; env?: string }): Promise<StackComposeSaveResponse> {

    return this.client.put<unknown, StackComposeSaveResponse>(
      `/stacks/agents/${agentId}/${name}/compose`,
      data,
    )
  
  },

  async stackAction(this: ApiCtx, agentId: string, name: string, action: 'up' | 'down' | 'restart' | 'pull'): Promise<StackTaskStartResponse> {

    return this.client.post<unknown, StackTaskStartResponse>(
      `/stacks/agents/${agentId}/${name}/${action}`,
    )
  
  },

  async getStackHistory(this: ApiCtx, agentId: string, name: string, limit = 50): Promise<{ deployments: StackDeployment[] }> {

    return this.client.get<unknown, { deployments: StackDeployment[] }>(
      `/stacks/agents/${agentId}/${name}/history`,
      { params: { limit } },
    )
  
  },

  async getStackTask(this: ApiCtx, agentId: string, taskId: string): Promise<StackTask> {

    return this.client.get<unknown, StackTask>(`/stacks/agents/${agentId}/tasks/${taskId}`)
  
  },

  async followStackTask(this: ApiCtx, agentId: string, taskId: string, signal: AbortSignal): Promise<Response> {

    const token = localStorage.getItem('token')
    return fetch(
      `/api/stacks/agents/${encodeURIComponent(agentId)}/tasks/${encodeURIComponent(taskId)}/follow`,
      {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
        body: JSON.stringify({}),
        signal,
      },
    )
  
  },

  async getStackLogs(this: ApiCtx, agentId: string, name: string, opts?: { service?: string; tail?: number }): Promise<{ logs: string }> {

    return this.client.get<unknown, { logs: string }>(`/stacks/agents/${agentId}/${name}/logs`, {
      params: { service: opts?.service, tail: opts?.tail ?? 200 },
    })
  
  },

  async deleteStack(this: ApiCtx, agentId: string, name: string): Promise<StackTaskStartResponse> {

    return this.client.delete<unknown, StackTaskStartResponse>(`/stacks/agents/${agentId}/${name}`)
  
  },

  async getStackGit(this: ApiCtx, agentId: string): Promise<StackGitStatus> {

    return this.client.get<unknown, StackGitStatus>(`/stacks/agents/${agentId}/git`)
  
  },

  async pullStackGit(this: ApiCtx, agentId: string, data?: { remote?: string; branch?: string }): Promise<StackGitPullResponse> {

    return this.client.post<unknown, StackGitPullResponse>(`/stacks/agents/${agentId}/git/pull`, data ?? {})
  
  }
}
