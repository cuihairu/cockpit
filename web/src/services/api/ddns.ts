import type { DDNSConfig, DDNSConfigInput, DDNSCheckResult, DDNSScanConfig } from '@/types'
import type { ApiCtx } from './core'

// ddns 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface DdnsMethods {
    getDDNSConfigs(): Promise<DDNSConfig[]>
    createDDNSConfig(input: DDNSConfigInput): Promise<DDNSConfig>
    updateDDNSConfig(id: number, input: DDNSConfigInput): Promise<DDNSConfig>
    deleteDDNSConfig(id: number): Promise<void>
    checkDDNSConfig(id: number): Promise<DDNSCheckResult>
    getDDNSScanConfig(): Promise<DDNSScanConfig>
    putDDNSScanConfig(scanIntervalSeconds: number): Promise<void>
}

export const ddnsMethods: DdnsMethods = {
  async getDDNSConfigs(this: ApiCtx): Promise<DDNSConfig[]> {

    return this.client.get<unknown, DDNSConfig[]>('/ddns')
  
  },

  async createDDNSConfig(this: ApiCtx, input: DDNSConfigInput): Promise<DDNSConfig> {

    return this.client.post<unknown, DDNSConfig>('/ddns', input)
  
  },

  async updateDDNSConfig(this: ApiCtx, id: number, input: DDNSConfigInput): Promise<DDNSConfig> {

    return this.client.put<unknown, DDNSConfig>(`/ddns/${id}`, input)
  
  },

  async deleteDDNSConfig(this: ApiCtx, id: number): Promise<void> {

    await this.client.delete(`/ddns/${id}`)
  
  },

  async checkDDNSConfig(this: ApiCtx, id: number): Promise<DDNSCheckResult> {

    return this.client.post<unknown, DDNSCheckResult>(`/ddns/${id}/check`)
  
  },

  async getDDNSScanConfig(this: ApiCtx): Promise<DDNSScanConfig> {

    return this.client.get<unknown, DDNSScanConfig>('/ddns/config')
  
  },

  async putDDNSScanConfig(this: ApiCtx, scanIntervalSeconds: number): Promise<void> {

    await this.client.put('/ddns/config', { scan_interval_seconds: scanIntervalSeconds })
  
  }
}
