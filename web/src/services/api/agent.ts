import type { Agent, AgentTagRef, AgentCleanupResult, ContainerInfo, ImageInfo } from '@/types'
import type { ApiCtx } from './core'

// agent 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface AgentMethods {
    getAgents(): Promise<Agent[]>
    getAgent(id: string): Promise<Agent>
    listAgentTags(): Promise<Array<AgentTagRef & { agentCount: number }>>
    createAgentTag(name: string, color: string): Promise<AgentTagRef>
    updateAgentTag(id: string, name: string, color: string): Promise<void>
    deleteAgentTag(id: string): Promise<void>
    setAgentTags(agentId: string, tagIds: string[]): Promise<void>
    cleanupAgents(body?: { thresholdMinutes?: number }): Promise<AgentCleanupResult>
    getContainers(agentId: string, all?: boolean): Promise<ContainerInfo[]>
    getImages(agentId: string): Promise<ImageInfo[]>
    startContainer(agentId: string, containerId: string): Promise<void>
    stopContainer(agentId: string, containerId: string, timeout?: number): Promise<void>
    restartContainer(agentId: string, containerId: string, timeout?: number): Promise<void>
    pauseContainer(agentId: string, containerId: string): Promise<void>
    unpauseContainer(agentId: string, containerId: string): Promise<void>
    removeContainer(agentId: string, containerId: string, opts?: { force?: boolean; volumes?: boolean }): Promise<void>
    getContainerLogs(agentId: string, containerId: string, opts?: { tail?: string; timestamps?: boolean }): Promise<string>
}

export const agentMethods: AgentMethods = {
  async getAgents(this: ApiCtx): Promise<Agent[]> {

    return this.client.get<unknown, Agent[]>('/agents')
  
  },

  async getAgent(this: ApiCtx, id: string): Promise<Agent> {

    return this.client.get<unknown, Agent>(`/agents/${id}`)
  
  },

  async listAgentTags(this: ApiCtx): Promise<Array<AgentTagRef & { agentCount: number }>> {

    return this.client.get('/agent-tags')
  
  },

  async createAgentTag(this: ApiCtx, name: string, color: string): Promise<AgentTagRef> {

    return this.client.post('/agent-tags', { name, color })
  
  },

  async updateAgentTag(this: ApiCtx, id: string, name: string, color: string): Promise<void> {

    return this.client.put(`/agent-tags/${id}`, { name, color })
  
  },

  async deleteAgentTag(this: ApiCtx, id: string): Promise<void> {

    return this.client.delete(`/agent-tags/${id}`)
  
  },

  async setAgentTags(this: ApiCtx, agentId: string, tagIds: string[]): Promise<void> {

    return this.client.put(`/agents/${agentId}/tags`, { tagIds })
  
  },

  async cleanupAgents(this: ApiCtx, body?: { thresholdMinutes?: number }): Promise<AgentCleanupResult> {

    return this.client.post('/agents/cleanup', body ?? {})
  
  },

  async getContainers(this: ApiCtx, agentId: string, all = true): Promise<ContainerInfo[]> {

    return this.client.get<unknown, ContainerInfo[]>(`/docker/agents/${agentId}/containers`, {
      params: { all },
    })
  
  },

  async getImages(this: ApiCtx, agentId: string): Promise<ImageInfo[]> {

    return this.client.get<unknown, ImageInfo[]>(`/docker/agents/${agentId}/images`)
  
  },

  async startContainer(this: ApiCtx, agentId: string, containerId: string): Promise<void> {

    await this.client.post(`/docker/agents/${agentId}/containers/${containerId}/start`)
  
  },

  async stopContainer(this: ApiCtx, agentId: string, containerId: string, timeout?: number): Promise<void> {

    await this.client.post(`/docker/agents/${agentId}/containers/${containerId}/stop`, undefined, {
      params: timeout !== undefined ? { timeout } : undefined,
    })
  
  },

  async restartContainer(this: ApiCtx, agentId: string, containerId: string, timeout?: number): Promise<void> {

    await this.client.post(`/docker/agents/${agentId}/containers/${containerId}/restart`, undefined, {
      params: timeout !== undefined ? { timeout } : undefined,
    })
  
  },

  async pauseContainer(this: ApiCtx, agentId: string, containerId: string): Promise<void> {

    await this.client.post(`/docker/agents/${agentId}/containers/${containerId}/pause`)
  
  },

  async unpauseContainer(this: ApiCtx, agentId: string, containerId: string): Promise<void> {

    await this.client.post(`/docker/agents/${agentId}/containers/${containerId}/unpause`)
  
  },

  async removeContainer(this: ApiCtx, agentId: string, containerId: string, opts?: { force?: boolean; volumes?: boolean }): Promise<void> {

    await this.client.delete(`/docker/agents/${agentId}/containers/${containerId}`, {
      params: opts,
    })
  
  },

  async getContainerLogs(this: ApiCtx, agentId: string, containerId: string, opts?: { tail?: string; timestamps?: boolean }): Promise<string> {

    return this.client.get<unknown, string>(
      `/docker/agents/${agentId}/containers/${containerId}/logs`,
      { params: { tail: opts?.tail ?? '100', timestamps: opts?.timestamps ?? false } },
    )
  
  }
}
