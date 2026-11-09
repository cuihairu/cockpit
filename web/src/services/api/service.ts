import type { ServiceStatus, ServiceListResult, ServiceActionName, ServiceUnitFile, ServiceUnitFileSaveResult } from '@/types'
import type { ApiCtx } from './core'

// service 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface ServiceMethods {
    getServiceStatus(agentId: string): Promise<ServiceStatus>
    getAgentServices(agentId: string): Promise<ServiceListResult>
    serviceAction(agentId: string, unit: string, action: ServiceActionName): Promise<{ name: string; action: string }>
    serviceDaemonReload(agentId: string): Promise<{ reloaded: boolean }>
    getServiceUnitFile(agentId: string, unit: string): Promise<ServiceUnitFile>
    saveServiceUnitFile(agentId: string, unit: string, content: string): Promise<ServiceUnitFileSaveResult>
}

export const serviceMethods: ServiceMethods = {
  async getServiceStatus(this: ApiCtx, agentId: string): Promise<ServiceStatus> {

    return this.client.get<unknown, ServiceStatus>(`/agents/${encodeURIComponent(agentId)}/services/status`)
  
  },

  async getAgentServices(this: ApiCtx, agentId: string): Promise<ServiceListResult> {

    return this.client.get<unknown, ServiceListResult>(`/agents/${encodeURIComponent(agentId)}/services`)
  
  },

  async serviceAction(this: ApiCtx, agentId: string, unit: string, action: ServiceActionName): Promise<{ name: string; action: string }> {

    return this.client.post<unknown, { name: string; action: string }>(
      `/agents/${encodeURIComponent(agentId)}/services/${encodeURIComponent(unit)}/${action}`,
    )
  
  },

  async serviceDaemonReload(this: ApiCtx, agentId: string): Promise<{ reloaded: boolean }> {

    return this.client.post<unknown, { reloaded: boolean }>(
      `/agents/${encodeURIComponent(agentId)}/services/daemon-reload`,
    )
  
  },

  async getServiceUnitFile(this: ApiCtx, agentId: string, unit: string): Promise<ServiceUnitFile> {

    return this.client.get<unknown, ServiceUnitFile>(
      `/agents/${encodeURIComponent(agentId)}/services/${encodeURIComponent(unit)}/file`,
    )
  
  },

  async saveServiceUnitFile(this: ApiCtx, agentId: string, unit: string, content: string): Promise<ServiceUnitFileSaveResult> {

    return this.client.put<unknown, ServiceUnitFileSaveResult>(
      `/agents/${encodeURIComponent(agentId)}/services/${encodeURIComponent(unit)}/file`,
      { content },
    )
  
  }
}
