import type { OobRedirectApplyResult, OobRedirectSpec, OobRedirectListResult, OobRedirectSweepResult } from '@/types'
import type { ApiCtx } from './core'

// oob 域 API 方法（自 api.ts 按资源边界拆分；经原型增强挂回 ApiService）
export interface OobMethods {
    oobRedirectApply(input: {
    agentId: string
    oob: string
    spec: OobRedirectSpec
    confirmLabel: string
  }): Promise<OobRedirectApplyResult>
    oobRedirectRemove(input: {
    agentId: string
    oob: string
    name: string
    confirmName: string
  }): Promise<{ name: string }>
    oobRedirectList(agentId: string, oob: string): Promise<OobRedirectListResult>
    oobRedirectSweep(input: {
    agentId: string
    oob: string
    confirmOob: string
  }): Promise<OobRedirectSweepResult>
}

export const oobMethods: OobMethods = {
  async oobRedirectApply(this: ApiCtx, input: {
    agentId: string
    oob: string
    spec: OobRedirectSpec
    confirmLabel: string
  }): Promise<OobRedirectApplyResult> {

    return this.client.post<unknown, OobRedirectApplyResult>('/oob/redirect', {
      agent_id: input.agentId,
      oob: input.oob,
      spec: input.spec,
      confirm_label: input.confirmLabel,
    })
  
  },

  async oobRedirectRemove(this: ApiCtx, input: {
    agentId: string
    oob: string
    name: string
    confirmName: string
  }): Promise<{ name: string }> {

    return this.client.post<unknown, { name: string }>('/oob/redirect/remove', {
      agent_id: input.agentId,
      oob: input.oob,
      name: input.name,
      confirm_name: input.confirmName,
    })
  
  },

  async oobRedirectList(this: ApiCtx, agentId: string, oob: string): Promise<OobRedirectListResult> {

    return this.client.get<unknown, OobRedirectListResult>(
      `/oob/redirect?agent_id=${encodeURIComponent(agentId)}&oob=${encodeURIComponent(oob)}`,
    )
  
  },

  async oobRedirectSweep(this: ApiCtx, input: {
    agentId: string
    oob: string
    confirmOob: string
  }): Promise<OobRedirectSweepResult> {

    return this.client.post<unknown, OobRedirectSweepResult>('/oob/redirect/sweep', {
      agent_id: input.agentId,
      oob: input.oob,
      confirm_oob: input.confirmOob,
    })
  
  }
}
