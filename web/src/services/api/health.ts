import type { HealthOverview, HealthProbe, HealthConfigSaveResult, HealthProbeCheckResult } from '@/types'
import type { ApiCtx } from './core'

// health 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface HealthMethods {
    getAgentHealth(agentId: string): Promise<HealthOverview>
    saveAgentHealth(agentId: string, body: { probes: HealthProbe[]; whitelist: string[] }): Promise<HealthConfigSaveResult>
    checkAgentProbe(agentId: string, probeId: string): Promise<HealthProbeCheckResult>
}

export const healthMethods: HealthMethods = {
  async getAgentHealth(this: ApiCtx, agentId: string): Promise<HealthOverview> {

    return this.client.get<unknown, HealthOverview>(`/agents/${encodeURIComponent(agentId)}/health`)
  
  },

  async saveAgentHealth(this: ApiCtx, agentId: string, body: { probes: HealthProbe[]; whitelist: string[] }): Promise<HealthConfigSaveResult> {

    return this.client.put<unknown, HealthConfigSaveResult>(`/agents/${encodeURIComponent(agentId)}/health`, body)
  
  },

  async checkAgentProbe(this: ApiCtx, agentId: string, probeId: string): Promise<HealthProbeCheckResult> {

    return this.client.post<unknown, HealthProbeCheckResult>(
      `/agents/${encodeURIComponent(agentId)}/health/probes/${encodeURIComponent(probeId)}/check`,
    )
  
  }
}
