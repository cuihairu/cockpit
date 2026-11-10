import type { ProbeConfig, ProbeResult, ProbeTargetSnapshot, ProbeWindow } from '@/types'
import type { ApiCtx } from './core'

// probe 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface ProbeMethods {
    getProbeConfig(): Promise<ProbeConfig>
    saveProbeConfig(config: ProbeConfig): Promise<ProbeConfig>
    getProbeHistory(resourceType: string, resourceId: string, limit?: number): Promise<{ results: ProbeResult[] }>
    getProbeTargets(): Promise<{ targets: ProbeTargetSnapshot[] }>
    getProbeWindows(params?: { agentId?: string; target?: string; limit?: number }): Promise<{ windows: ProbeWindow[] }>
}

export const probeMethods: ProbeMethods = {
  async getProbeConfig(this: ApiCtx): Promise<ProbeConfig> {

    return this.client.get<unknown, ProbeConfig>('/probe/config')
  
  },

  async saveProbeConfig(this: ApiCtx, config: ProbeConfig): Promise<ProbeConfig> {

    return this.client.put<unknown, ProbeConfig>('/probe/config', {
      interval_seconds: config.interval_seconds,
      fail_threshold: config.fail_threshold,
      disk_percent: config.disk_percent,
      memory_percent: config.memory_percent,
      cert_warn_days: config.cert_warn_days,
      cert_info_days: config.cert_info_days,
    })
  
  },

  async getProbeHistory(this: ApiCtx, resourceType: string, resourceId: string, limit = 50): Promise<{ results: ProbeResult[] }> {

    return this.client.get<unknown, { results: ProbeResult[] }>(
      `/probe/history?resource_type=${encodeURIComponent(resourceType)}&resource_id=${encodeURIComponent(resourceId)}&limit=${limit}`,
    )
  
  },

  async getProbeTargets(this: ApiCtx): Promise<{ targets: ProbeTargetSnapshot[] }> {

    return this.client.get<unknown, { targets: ProbeTargetSnapshot[] }>('/probe/targets')
  
  },

  async getProbeWindows(this: ApiCtx, params?: { agentId?: string; target?: string; limit?: number }): Promise<{ windows: ProbeWindow[] }> {

    const parts: string[] = []
    if (params?.agentId) parts.push(`agent_id=${encodeURIComponent(params.agentId)}`)
    if (params?.target) parts.push(`target=${encodeURIComponent(params.target)}`)
    if (params?.limit) parts.push(`limit=${params.limit}`)
    const qs = parts.length ? `?${parts.join('&')}` : ''
    return this.client.get<unknown, { windows: ProbeWindow[] }>(`/probe/windows${qs}`)
  
  }
}
