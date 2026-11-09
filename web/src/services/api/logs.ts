import type { LogsStatus, LogsSources, LogsQuery, LogsQueryResult, LogsSearchQuery, LogsSearchResult } from '@/types'
import type { ApiCtx } from './core'

// logs 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface LogsMethods {
    getLogsStatus(agentId: string): Promise<LogsStatus>
    getLogsSources(agentId: string): Promise<LogsSources>
    queryLogs(agentId: string, query: LogsQuery): Promise<LogsQueryResult>
    searchLogs(payload: LogsSearchQuery): Promise<LogsSearchResult>
    followLogs(agentId: string, query: LogsQuery, signal: AbortSignal): Promise<Response>
}

export const logsMethods: LogsMethods = {
  async getLogsStatus(this: ApiCtx, agentId: string): Promise<LogsStatus> {

    return this.client.get<unknown, LogsStatus>(
      `/agents/${encodeURIComponent(agentId)}/logs/status`,
    )
  
  },

  async getLogsSources(this: ApiCtx, agentId: string): Promise<LogsSources> {

    return this.client.get<unknown, LogsSources>(
      `/agents/${encodeURIComponent(agentId)}/logs/sources`,
    )
  
  },

  async queryLogs(this: ApiCtx, agentId: string, query: LogsQuery): Promise<LogsQueryResult> {

    return this.client.post<unknown, LogsQueryResult>(
      `/agents/${encodeURIComponent(agentId)}/logs/query`,
      query,
    )
  
  },

  async searchLogs(this: ApiCtx, payload: LogsSearchQuery): Promise<LogsSearchResult> {

    return this.client.post<unknown, LogsSearchResult>('/logs/search', payload)
  
  },

  async followLogs(this: ApiCtx, agentId: string, query: LogsQuery, signal: AbortSignal): Promise<Response> {

    const token = localStorage.getItem('token')
    return fetch(`/api/agents/${encodeURIComponent(agentId)}/logs/follow`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify(query),
      signal,
    })
  
  }
}
