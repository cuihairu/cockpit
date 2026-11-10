import type { StatusResponse } from '@/types'
import type { ApiCtx } from './core'
import type { Alert } from '../api'

// status 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface StatusMethods {
    getStatus(): Promise<StatusResponse>
    getAlerts(): Promise<{ data: Alert[] }>
    markAlertRead(id: string): Promise<void>
    markAllAlertsRead(): Promise<void>
}

export const statusMethods: StatusMethods = {
  async getStatus(this: ApiCtx): Promise<StatusResponse> {

    return this.client.get<unknown, StatusResponse>('/status')
  
  },

  async getAlerts(this: ApiCtx): Promise<{ data: Alert[] }> {

    return this.client.get<unknown, { data: Alert[] }>('/alerts')
  
  },

  async markAlertRead(this: ApiCtx, id: string): Promise<void> {

    return this.client.put(`/alerts/${id}/read`)
  
  },

  async markAllAlertsRead(this: ApiCtx): Promise<void> {

    return this.client.put('/alerts/read-all')
  
  }
}
