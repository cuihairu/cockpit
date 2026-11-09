import type { NasStatus, NasScanConfig } from '@/types'
import type { ApiCtx } from './core'

// nas 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface NasMethods {
    getNASStatus(agentId: string): Promise<NasStatus>
    getNASConfig(): Promise<NasScanConfig>
    putNASConfig(scanIntervalSeconds: number, usageWarnPercent?: number): Promise<void>
}

export const nasMethods: NasMethods = {
  async getNASStatus(this: ApiCtx, agentId: string): Promise<NasStatus> {

    return this.client.get<unknown, NasStatus>(`/agents/${encodeURIComponent(agentId)}/nas/status`)
  
  },

  async getNASConfig(this: ApiCtx): Promise<NasScanConfig> {

    return this.client.get<unknown, NasScanConfig>('/nas/config')
  
  },

  async putNASConfig(this: ApiCtx, scanIntervalSeconds: number, usageWarnPercent?: number): Promise<void> {

    await this.client.put('/nas/config', {
      scan_interval_seconds: scanIntervalSeconds,
      usage_warn_percent: usageWarnPercent,
    })
  
  }
}
