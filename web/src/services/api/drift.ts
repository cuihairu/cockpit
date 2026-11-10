import type { DriftCheckResult, DriftDiffResult, DriftScanConfig, ConsistencyReport } from '@/types'
import type { ApiCtx } from './core'

// drift 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface DriftMethods {
    checkDrift(agentId: string): Promise<DriftCheckResult>
    driftDiff(agentId: string, kind: string, name: string): Promise<DriftDiffResult>
    driftRecord(agentId: string, kind: string, name: string): Promise<void>
    getDriftConfig(): Promise<DriftScanConfig>
    putDriftConfig(scanIntervalSeconds: number): Promise<void>
    getInventoryConsistency(): Promise<ConsistencyReport>
}

export const driftMethods: DriftMethods = {
  async checkDrift(this: ApiCtx, agentId: string): Promise<DriftCheckResult> {

    return this.client.post<unknown, DriftCheckResult>(
      `/agents/${encodeURIComponent(agentId)}/drift/check`,
    )
  
  },

  async driftDiff(this: ApiCtx, agentId: string, kind: string, name: string): Promise<DriftDiffResult> {

    return this.client.post<unknown, DriftDiffResult>(
      `/agents/${encodeURIComponent(agentId)}/drift/diff`,
      { kind, name },
    )
  
  },

  async driftRecord(this: ApiCtx, agentId: string, kind: string, name: string): Promise<void> {

    await this.client.post(`/agents/${encodeURIComponent(agentId)}/drift/record`, {
      kind,
      name,
    })
  
  },

  async getDriftConfig(this: ApiCtx): Promise<DriftScanConfig> {

    return this.client.get<unknown, DriftScanConfig>('/drift/config')
  
  },

  async putDriftConfig(this: ApiCtx, scanIntervalSeconds: number): Promise<void> {

    await this.client.put('/drift/config', { scan_interval_seconds: scanIntervalSeconds })
  
  },

  async getInventoryConsistency(this: ApiCtx): Promise<ConsistencyReport> {

    return this.client.get<unknown, ConsistencyReport>('/inventory/consistency')
  
  }
}
