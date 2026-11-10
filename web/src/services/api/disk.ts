import type { SmartStatus, SmartScanConfig } from '@/types'
import type { ApiCtx } from './core'

// disk 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface DiskMethods {
    getSmartStatus(agentId: string): Promise<SmartStatus>
    getSmartConfig(): Promise<SmartScanConfig>
    putSmartConfig(scanIntervalSeconds: number): Promise<void>
}

export const diskMethods: DiskMethods = {
  async getSmartStatus(this: ApiCtx, agentId: string): Promise<SmartStatus> {

    return this.client.get<unknown, SmartStatus>(`/agents/${encodeURIComponent(agentId)}/smart/status`)
  
  },

  async getSmartConfig(this: ApiCtx): Promise<SmartScanConfig> {

    return this.client.get<unknown, SmartScanConfig>('/smart/config')
  
  },

  async putSmartConfig(this: ApiCtx, scanIntervalSeconds: number): Promise<void> {

    await this.client.put('/smart/config', { scan_interval_seconds: scanIntervalSeconds })
  
  }
}
